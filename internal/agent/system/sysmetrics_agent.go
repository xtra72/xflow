package system

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// 시스템 모니터링 에이전트.
//
// 호스트의 CPU·메모리·스토리지·디스크 I/O·네트워크를 주기적으로 표본 수집해,
// **표본 하나를 메시지 하나로** 묶어 내보낸다. 짝이 되는 `sysmetrics-in` 노드가
// 레코드를 flow 메시지로 옮긴다.
//
// 측정값마다 쪼개 보내면(fan-out) 메시지가 폭증한다 — 마운트 2개 + 장치 2개 +
// 인터페이스 3개면 5초마다 40여 건이다. 대신 값들을 payload 의 평탄한 키로 묶는다.
// storage-write 의 `fields` 모드가 정확히 이 형태를 기대한다.

// SysMetricsAgent 는 시스템 리소스 표본을 주기적으로 방출하는 에이전트이다.
type SysMetricsAgent struct {
	*agent.BaseAgent

	mu          sync.RWMutex
	agentConfig agent.AgentConfig
	cfg         SysMetricsConfig
	logger      *slog.Logger
	// stats 는 방출 통계이다. BaseAgent 내부 통계는 비공개라 별도로 든다
	// (chirpstack 에이전트와 같은 방식).
	stats *agent.AgentStats

	// history 는 표본 이력 버퍼다. 대시보드 패널이 `get_history` 로 질의한다.
	//
	// 스냅샷과 나란히 두는 이유: 스냅샷은 "지금 값"(타일·게이지)이고 이력은 "구간"
	// (차트)이다. 두 화면이 서로 다른 것을 필요로 하므로 한쪽으로 합치지 않는다.
	history *sysMetricsHistory

	// snapshot 은 마지막으로 뜬 표본이다. 대시보드 패널이 `State()` 로 읽는다.
	// 방출과 독립으로 갱신되므로 플로우를 구성하지 않아도 값이 채워진다
	// (sysmetrics_state.go 참조). 표본 이전에는 nil 이다.
	snapshot *SysMetricsSample

	// recvCh 는 표본 JSON 을 노드로 넘기는 통로이다.
	recvCh chan []byte
	// done 은 Stop 신호이다. Start 마다 새로 만든다.
	done chan struct{}
	// wg 는 표본 루프 종료를 기다린다.
	wg sync.WaitGroup
}

// 인터페이스 준수 확인.
var (
	_ agent.Agent           = (*SysMetricsAgent)(nil)
	_ agent.MessageReceiver = (*SysMetricsAgent)(nil)
)

// recvBufferSize 는 레코드 버퍼 크기이다.
//
// 표본 하나가 메시지 하나이므로 크지 않아도 된다. 노드가 잠깐 멈춰도 몇 분치는
// 버티도록 여유를 둔다.
const recvBufferSize = 256

// NewSysMetricsAgent 는 설정으로 에이전트를 만든다.
func NewSysMetricsAgent(config agent.AgentConfig) (*SysMetricsAgent, error) {
	a := &SysMetricsAgent{
		BaseAgent: agent.NewBaseAgent(),
		stats:     agent.NewAgentStats(),
		history:   newSysMetricsHistory(),
	}
	if err := a.Init(config); err != nil {
		return nil, err
	}
	return a, nil
}

// Init 은 설정을 검증하고 상태를 초기화한다.
func (a *SysMetricsAgent) Init(config agent.AgentConfig) error {
	cfg, err := parseSysMetricsConfig(config)
	if err != nil {
		return fmt.Errorf("sysmetrics: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.cfg = cfg
	a.logger = config.Logger
	if a.logger == nil {
		a.logger = slog.Default()
	}
	a.recvCh = make(chan []byte, recvBufferSize)
	a.mu.Unlock()

	return a.BaseAgent.Init(config)
}

// Configure 는 설정을 교체한다. 실행 중이면 다음 표본부터 반영된다.
//
// 파싱을 먼저 하고 그 다음에 갈아 끼운다 — 해석할 수 없는 설정으로 절반만 갱신된
// 상태를 만들지 않는다.
//
// `BaseAgent.Configure` 도 함께 부른다. 조회 응답의 config 는 `Info()` 가 내는 값이고
// (`agentToHandlerInfo`), 그 기본 구현은 BaseAgent 가 들고 있는 config 를 돌려준다.
// 여기서 자기 필드만 갈아 끼우면 수집은 새 설정으로 도는데 화면은 옛 값을 보여 준다 —
// 사용자에게는 "인터페이스를 골랐는데 적용이 안 됨" 으로 보이고, 데이터는 실제로
// 걸러지고 있어 추적하기 어렵다.
func (a *SysMetricsAgent) Configure(config agent.AgentConfig) error {
	cfg, err := parseSysMetricsConfig(config)
	if err != nil {
		return fmt.Errorf("sysmetrics: %w", err)
	}

	// 검증과 조회용 config 갱신은 BaseAgent 의 몫이다. 여기서 다시 적으면 두 곳이
	// 어긋난다.
	if err := a.BaseAgent.Configure(config); err != nil {
		return err
	}

	a.mu.Lock()
	a.agentConfig = config
	a.cfg = cfg
	a.mu.Unlock()

	return nil
}

// Start 는 표본 루프를 시작한다.
//
// 루프 기동 여부는 lifecycle 상태가 아니라 `done` 채널로 판정한다. `BaseAgent.Init`
// 이 이미 Running 으로 전이시키므로(Created → Initializing → Running), 상태만 보고
// 되돌아가면 생성 직후 Start 에서 루프가 영영 걸리지 않는다.
func (a *SysMetricsAgent) Start(ctx context.Context) error {
	if a.CurrentState() == lifecycle.StateStopped {
		// Stop 이 닫아 둔 done 을 그대로 두고 다시 돌면 루프가 즉시 끝난다.
		if err := a.TransitionTo(lifecycle.StateCreated); err != nil {
			return fmt.Errorf("sysmetrics start: reset to created: %w", err)
		}
		a.mu.RLock()
		cfg := a.agentConfig
		a.mu.RUnlock()
		if err := a.Init(cfg); err != nil {
			return err
		}
	}

	if a.CurrentState() != lifecycle.StateRunning {
		if err := a.BaseAgent.Start(ctx); err != nil {
			return err
		}
	}

	a.mu.Lock()
	if a.done != nil {
		// 이미 돌고 있다 — 두 번째 루프를 띄우면 표본이 두 배로 나간다.
		a.mu.Unlock()
		return nil
	}
	a.done = make(chan struct{})
	done := a.done
	interval := a.cfg.Interval
	a.mu.Unlock()

	// CPU 사용률 예열. gopsutil 의 논블로킹 모드는 직전 호출과의 차이로 계산하므로,
	// 기준점 없이 첫 표본을 뜨면 0 이 나온다.
	if _, err := collectCPU(0); err != nil {
		a.logger.Debug("sysmetrics: CPU 예열 실패", "error", err)
	}

	a.wg.Add(1)
	go a.sampleLoop(done, interval)

	return nil
}

// Stop 은 표본 루프를 멈춘다.
func (a *SysMetricsAgent) Stop(ctx context.Context) error {
	a.mu.Lock()
	done := a.done
	a.done = nil
	a.mu.Unlock()

	if done != nil {
		close(done)
	}
	a.wg.Wait()

	return a.BaseAgent.Stop(ctx)
}

// sampleLoop 는 주기마다 표본을 떠서 채널에 넣는다.
func (a *SysMetricsAgent) sampleLoop(done <-chan struct{}, interval time.Duration) {
	defer a.wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			a.emitSample(done)
		}
	}
}

// emitSample 은 표본 하나를 떠서 일괄 레코드로 묶어 채널에 넣는다.
func (a *SysMetricsAgent) emitSample(done <-chan struct{}) {
	a.mu.RLock()
	cfg := a.cfg
	ch := a.recvCh
	a.mu.RUnlock()

	sample, errs := collectSample(cfg, time.Now().UnixMilli())
	for _, err := range errs {
		// 지표 하나가 실패해도 표본은 보낸다 — 디스크 권한 문제로 CPU 관측까지
		// 끊기면 관측 도구로서 쓸모가 없다.
		a.logger.Warn("sysmetrics: 지표 수집 실패", "error", err)
	}

	// 스냅샷을 먼저 갱신한다. 이 순서가 계약이다 — 아래 방출은 채널이 가득 차면
	// 표본을 버리는데, 플로우를 구성하지 않은 사용자에게 채널은 늘 가득 찬 상태다.
	// 저장을 방출 뒤로 옮기면 그런 사용자의 대시보드 패널이 영영 비게 된다.
	a.storeSnapshot(sample)

	payload, err := json.Marshal(buildBatch(sample))
	if err != nil {
		a.logger.Error("sysmetrics: 표본 직렬화 실패", "error", err)
		return
	}

	select {
	case ch <- payload:
		a.stats.IncrExternalMessagesReceived()
	case <-done:
	default:
		// 노드가 소비하지 못해 버퍼가 찼다. 오래된 것을 밀어내지 않고 이번 표본을
		// 버린다 — 시계열은 순서가 뒤집히는 편이 더 나쁘다.
		a.logger.Warn("sysmetrics: 버퍼가 가득 차 표본을 버렸습니다")
	}
}

// ReceiveMessage 는 표본 채널에서 메시지를 가져온다 (agent.MessageReceiver).
func (a *SysMetricsAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	a.mu.RLock()
	ch := a.recvCh
	done := a.done
	a.mu.RUnlock()

	select {
	case data := <-ch:
		a.stats.IncrInternalMessagesSent()
		return data, nil
	case <-done:
		return nil, fmt.Errorf("sysmetrics: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Type 은 에이전트 타입을 반환한다.
func (a *SysMetricsAgent) Type() string {
	return SysMetricsAgentType
}
