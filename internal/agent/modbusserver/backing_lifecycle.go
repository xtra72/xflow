package modbusserver

import (
	"context"
	"log/slog"

	modbus "github.com/xtra/xflow/internal/agent/modbus"
)

// upstreamTransportFactory 는 백킹 upstream 트랜스포트 생성 함수 타입이다.
// 기본값은 newUpstreamTransport 이며, 테스트에서 fake 트랜스포트를 주입하기 위한 seam 이다.
type upstreamTransportFactory func(cfg *BackingConfig, logger *slog.Logger) (modbus.ModbusTransport, error)

// ---------------------------------------------------------------------------
// 백킹 런타임 수명 배선 (REQ-MODBUS-010-02/04/05)
// ---------------------------------------------------------------------------
//
// deviceBacking 은 단일 백킹 디바이스의 런타임 상태(upstream 트랜스포트 + indirect
// 폴러)를 묶는다. 에이전트가 a.mu 보호 하에 a.backings[unitID] 로 소유하며, Stop /
// remove_device 시 폴러 종료 + 트랜스포트 Close 로 정리한다(goroutine 누수 방지).
type deviceBacking struct {
	transport modbus.ModbusTransport
	poller    *devicePoller // indirect 만 존재; direct 는 nil
}

// setupBacking 은 백킹 설정에 따라 upstream 트랜스포트를 구성·연결하고 backedStore 로
// 감싼 RequestHandler 를 만든다. indirect 면 폴러도 시작한다. 반환된 RequestHandler 는
// 디바이스의 ReqHandler 로 연결되고, deviceBacking 은 a.backings 에 등록되어야 한다(호출자 책임).
//
// 주의: 이 메서드는 a.mu 를 잡지 않는다. transport.Connect(TCP dial)가 블로킹될 수 있어
// 락을 잡은 채 호출하면 다른 Process/Stop 을 지연시키기 때문이다. 호출자는 a.mu 밖에서
// 호출하고, 반환 후 맵 등록 시에만 짧게 a.mu 를 잡는다.
//
// inner 는 디바이스 자체 RegisterMap(저장/서빙 소스), rmCfg 는 폴 대상 영역 산출용
// register_map 설정, bc 는 검증된 BackingConfig 이다. ctx 는 폴러 수명을 묶는 에이전트
// 컨텍스트이다.
func (a *ModbusServerAgent) setupBacking(ctx context.Context, inner *RegisterMap, rmCfg RegisterMapConfig, bc *BackingConfig) (*RequestHandler, *deviceBacking, error) {
	factory := a.newTransport
	if factory == nil {
		factory = newUpstreamTransport
	}
	transport, err := factory(bc, a.logger)
	if err != nil {
		return nil, nil, err
	}

	// 연결 수립. 실패해도 게이트웨이 전체를 죽이지 않는다 — 백킹 디바이스는 도달 불가
	// 상태에서 0x0B 를 반환하며, indirect 폴러는 다음 주기에 재연결/갱신을 시도한다.
	if cerr := transport.Connect(ctx); cerr != nil && a.logger != nil {
		a.logger.Warn("modbus-gateway: upstream connect failed",
			"transport", bc.Transport, "unit_id", bc.UnitID, "error", cerr)
	}

	bs := newBackedStore(inner, transport, bc)
	handler := newRequestHandlerWithStore(bs, a.logger)

	backing := &deviceBacking{transport: transport}
	if bs.mode == backingIndirect {
		backing.poller = startPoller(ctx, bs, pollTargetsFromConfig(rmCfg), bc.PollInterval, a.logger)
	}

	return handler, backing, nil
}

// startBackings 는 설정 시점 백킹 디바이스(cfg.Backing != nil)에 대해 upstream 연결 +
// (indirect) 폴러 시작을 수행하고, 디바이스의 ReqHandler 를 backedStore 기반으로 교체한다.
// 에이전트 Start 에서 리스너 기동 이전에 호출되므로 ReqHandler 교체에 동시 접근이 없다.
func (a *ModbusServerAgent) startBackings(ctx context.Context) error {
	for i := range a.config.Devices {
		cfg := a.config.Devices[i]
		if cfg.Backing == nil {
			continue
		}
		dev := a.deviceManager.GetDevice(cfg.UnitID)
		if dev == nil {
			// zero-device / role=sub 등 대상이 아직 없으면 건너뛴다.
			continue
		}

		a.mu.RLock()
		_, exists := a.backings[cfg.UnitID]
		a.mu.RUnlock()
		if exists {
			// 재기동 대비 멱등: 이미 배선되어 있으면 중복 배선하지 않는다.
			continue
		}

		handler, backing, err := a.setupBacking(ctx, dev.RegisterMap, cfg.RegisterMap, cfg.Backing)
		if err != nil {
			return err
		}
		dev.ReqHandler = handler

		a.mu.Lock()
		a.backings[cfg.UnitID] = backing
		a.mu.Unlock()
	}
	return nil
}

// stopBackings 는 모든 백킹 폴러를 종료하고 upstream 트랜스포트를 Close 한다(Stop 경로).
// a.mu 를 짧게 잡아 맵을 스왑한 뒤, 락 밖에서 폴러 종료(블로킹)를 대기한다.
func (a *ModbusServerAgent) stopBackings() {
	a.mu.Lock()
	backings := a.backings
	a.backings = make(map[byte]*deviceBacking)
	a.mu.Unlock()

	for _, b := range backings {
		teardownBacking(b)
	}
}

// teardownBacking 은 단일 백킹의 폴러 종료 + 트랜스포트 Close 를 수행한다.
// 폴러 stop() 은 goroutine 종료를 대기하므로 a.mu 를 잡지 않은 상태에서 호출해야 한다.
func teardownBacking(b *deviceBacking) {
	if b == nil {
		return
	}
	if b.poller != nil {
		b.poller.stop()
	}
	if b.transport != nil {
		_ = b.transport.Close()
	}
}
