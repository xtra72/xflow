package modbusserver

import (
	"context"
	"log/slog"
	"time"

	modbus "github.com/xtra/xflow/internal/agent/modbus"
)

// ---------------------------------------------------------------------------
// Indirect 폴러 (REQ-MODBUS-010-04)
// ---------------------------------------------------------------------------
//
// devicePoller 는 indirect 백킹 디바이스마다 하나씩 존재하는 백그라운드 폴러이다.
// poll_interval 마다 backedStore 를 통해 upstream 의 선언된 영역을 조회하여 내부
// RegisterMap(backedStore.inner)을 갱신하고, 전 영역 성공 시 마지막 성공 시각을
// 기록한다(recordPollSuccess). 하나라도 실패하면 시각을 갱신하지 않아 staleness 가
// 자연히 자라며, timeout 초과 시 indirect 읽기가 0x0B 를 반환하게 한다(AC-09).
//
// 수명: context 취소로 종료되며 done 채널로 종료를 관측할 수 있어 goroutine 누수
// 검증이 가능하다. 시작=디바이스 생성 시(Start/add_device), 중지=제거/Stop 시로,
// 상위(agent.go, M5)가 소유한다.

// pollTarget 은 indirect 폴러가 매 주기 upstream 에서 조회할 단일 영역이다.
// fc 는 읽기 기능 코드(FC01~04), start/count 는 디바이스 주소 범위이다.
type pollTarget struct {
	fc    byte
	start uint16
	count uint16
}

// pollTargetsFromConfig 는 디바이스의 register_map 설정에서 폴링 대상 영역 목록을
// 구성한다. 각 영역(coils/discrete_inputs/holding/input)의 로컬 세그먼트를 대응
// 읽기 FC 로 매핑한다. 공유(IsShared) 세그먼트는 컨테이너가 서빙하므로 제외한다.
func pollTargetsFromConfig(cfg RegisterMapConfig) []pollTarget {
	var targets []pollTarget
	add := func(fc byte, segs []*RegisterAreaConfig) {
		for _, s := range segs {
			if s == nil || s.IsShared || s.Count == 0 {
				continue
			}
			targets = append(targets, pollTarget{fc: fc, start: s.StartAddress, count: s.Count})
		}
	}
	add(modbus.FC01ReadCoils, cfg.Coils)
	add(modbus.FC02ReadDiscreteInputs, cfg.DiscreteInputs)
	add(modbus.FC03ReadHoldingRegisters, cfg.HoldingRegisters)
	add(modbus.FC04ReadInputRegisters, cfg.InputRegisters)
	return targets
}

// devicePoller 는 단일 indirect 디바이스의 폴러 goroutine 핸들이다.
type devicePoller struct {
	store    *backedStore
	targets  []pollTarget
	interval time.Duration
	cancel   context.CancelFunc
	done     chan struct{}
	logger   *slog.Logger
}

// startPoller 는 폴러 goroutine 을 시작하고 핸들을 반환한다. ctx 는 상위(에이전트)
// 수명 컨텍스트이며, 반환된 핸들의 stop() 또는 ctx 취소로 폴러가 종료된다.
func startPoller(ctx context.Context, store *backedStore, targets []pollTarget, interval time.Duration, logger *slog.Logger) *devicePoller {
	pctx, cancel := context.WithCancel(ctx)
	p := &devicePoller{
		store:    store,
		targets:  targets,
		interval: interval,
		cancel:   cancel,
		done:     make(chan struct{}),
		logger:   logger,
	}
	go p.run(pctx)
	return p
}

// run 은 폴러 메인 루프이다. 매 tick 마다 pollOnce 를 수행하고, ctx 취소 시 종료한다.
// defer close(done) 로 종료를 관측 가능하게 한다(누수 검증).
func (p *devicePoller) run(ctx context.Context) {
	defer close(p.done)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.pollOnce(time.Now()); err != nil {
				// 폴 실패 시 lastOK 를 갱신하지 않아 stale 판정이 자연히 처리된다(AC-09).
				// 폴링은 계속 유지하여 upstream 복구 시 다음 주기에 갱신된다.
				if p.logger != nil {
					p.logger.Debug("modbus-gateway: upstream poll failed", "error", err)
				}
			}
		}
	}
}

// pollOnce 는 선언된 모든 영역을 upstream 에서 조회하여 내부 RegisterMap 을 갱신한다.
// 전 영역이 성공하면 recordPollSuccess 로 마지막 성공 시각을 기록한다(AC-07).
// 하나라도 실패하면 즉시 오류를 반환하며 시각을 갱신하지 않는다(staleness 성장).
// 폴링 대상이 없으면(비정상 설정) 성공을 기록하지 않는다.
func (p *devicePoller) pollOnce(now time.Time) error {
	if len(p.targets) == 0 {
		return nil
	}
	bs := p.store
	for _, tgt := range p.targets {
		switch tgt.fc {
		case modbus.FC01ReadCoils:
			vals, err := bs.upstreamReadBits(tgt.fc, tgt.start, tgt.count)
			if err != nil {
				return err
			}
			_, _ = bs.inner.WriteCoils(tgt.start, vals)
		case modbus.FC02ReadDiscreteInputs:
			vals, err := bs.upstreamReadBits(tgt.fc, tgt.start, tgt.count)
			if err != nil {
				return err
			}
			_, _ = bs.inner.WriteDiscreteInputs(tgt.start, vals)
		case modbus.FC03ReadHoldingRegisters:
			vals, err := bs.upstreamReadRegisters(tgt.fc, tgt.start, tgt.count)
			if err != nil {
				return err
			}
			_, _ = bs.inner.WriteHoldingRegisters(tgt.start, vals)
		case modbus.FC04ReadInputRegisters:
			vals, err := bs.upstreamReadRegisters(tgt.fc, tgt.start, tgt.count)
			if err != nil {
				return err
			}
			_, _ = bs.inner.WriteInputRegisters(tgt.start, vals)
		}
	}
	bs.recordPollSuccess(now)
	return nil
}

// stop 은 폴러 goroutine 에 종료를 요청하고 실제 종료를 대기한다(goroutine 누수 방지).
// 여러 번 호출해도 안전하다(cancel 은 멱등, done 은 이미 닫혀 있으면 즉시 반환).
func (p *devicePoller) stop() {
	p.cancel()
	<-p.done
}
