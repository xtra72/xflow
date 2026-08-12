// Package chirpstack 는 ChirpStack LoRaWAN Network Server 가 발행하는 MQTT 업링크를
// 수신하여 measurement 당 1개 메시지로 fan-out 하는 수신 전용 에이전트를 제공한다
// (SPEC-CHIRPSTACK-001).
//
// 2계층 설계:
//   - 에이전트(본 패키지): MQTT 연결/구독/수신, 업링크 디코드, per-measurement
//     레코드 생성, 디바이스 자동 생성/메타데이터 노출.
//   - 노드(internal/node/chirpstack.go): 수신 전용 SourceNode. 에이전트가 emit 한
//     per-measurement 레코드를 소비해 flow message 로 빌드하고 device 그룹을 승격.
package chirpstack

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ChirpStackAgent 는 ChirpStack LoRaWAN 업링크를 수신하는 수신 전용 에이전트이다.
// 발행 경로(MessagePublisher)는 구현하지 않는다.
//
// M1 은 라이프사이클 스켈레톤이다. MQTT 트랜스포트(M2), 업링크 디코드/fan-out(M3),
// 디바이스 라이프사이클/Provider(M4)는 후속 마일스톤에서 추가된다.
type ChirpStackAgent struct {
	*lifecycle.BaseLifecycle

	agentConfig agent.AgentConfig

	// stopped 는 Stop() 이 호출되었음을 나타내는 가드 플래그이다 (mqtt_agent.go 이식).
	// Paho 의 백그라운드 재연결 goroutine 이 Stop 이후 재연결에 성공해도
	// subscribe() 가 stopped 상태이면 즉시 Disconnect 하여 세션 부활을 막는다(M2).
	stopped atomic.Bool

	stats  *agent.AgentStats
	logger *slog.Logger

	mu        sync.RWMutex
	startedAt time.Time
	createdAt time.Time
}

// 컴파일 타임 인터페이스 체크 (M1: agent.Agent 만; 선택 인터페이스는 M2 에서 추가).
var _ agent.Agent = (*ChirpStackAgent)(nil)

// NewChirpStackAgent 는 ChirpStackAgent 팩토리 함수이다.
//
// 에이전트 이름은 배포 내에서 고유해야 한다(REQ-M1-04): device_id 가
// (agentName, devEui) 로 키잉되므로, 이름 충돌은 조용히 덮어쓰지 않고
// ErrNameCollision 으로 거부한다.
func NewChirpStackAgent(config agent.AgentConfig) (agent.Agent, error) {
	if err := claimAgentName(config.Name, config.ID); err != nil {
		return nil, err
	}

	a := &ChirpStackAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("chirpstack")),
		stats:         agent.NewAgentStats(),
		logger:        agent.ResolveLogger(config),
		createdAt:     time.Now(),
	}

	if err := a.Init(config); err != nil {
		releaseAgentName(config.Name, config.ID)
		return nil, err
	}
	return a, nil
}

// Init 은 에이전트를 초기화한다.
//
// M1: 활성화 상태면 StateRunning 으로 전이한다(MQTT 연결은 M2). 비활성화 상태면
// List API 노출을 위해 생성만 하고 StateCreated 에 머문다.
func (a *ChirpStackAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("chirpstack init: %w", err)
	}

	// 재시작 경로에서 stopped 가드를 해제한다 (mqtt_agent.go 관례).
	a.stopped.Store(false)

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	// 비활성화 게이트: disabled 에이전트도 List API 노출을 위해 생성되나 연결하지
	// 않는다. lifecycle 은 StateCreated 에 머문다.
	if !config.IsEnabled() {
		a.logger.Info("chirpstack: 비활성화 상태로 생성됨 — 연결 건너뜀",
			"name", config.Name,
		)
		return nil
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("chirpstack init: %w", err)
	}
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("chirpstack init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()
	return nil
}

// Start 는 이미 Running 이면 no-op, Stopped/Created 이면 재-Init 한다.
func (a *ChirpStackAgent) Start(_ context.Context) error {
	switch a.CurrentState() {
	case lifecycle.StateRunning:
		return nil
	case lifecycle.StateCreated:
		a.mu.RLock()
		cfg := a.agentConfig
		a.mu.RUnlock()
		return a.Init(cfg)
	case lifecycle.StateStopped:
		if err := a.TransitionTo(lifecycle.StateCreated); err != nil {
			return fmt.Errorf("chirpstack start: reset to created: %w", err)
		}
		a.mu.RLock()
		cfg := a.agentConfig
		a.mu.RUnlock()
		return a.Init(cfg)
	default:
		return fmt.Errorf("chirpstack start: not in running state (current: %s)", a.CurrentState())
	}
}

// Stop 은 에이전트를 정지한다. idempotent: 이미 Stopped 면 전이를 건너뛴다.
func (a *ChirpStackAgent) Stop(_ context.Context) error {
	a.stopped.Store(true)

	if a.CurrentState() != lifecycle.StateStopped {
		if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
			return fmt.Errorf("chirpstack stop: %w", err)
		}
		if err := a.TransitionTo(lifecycle.StateStopped); err != nil {
			return fmt.Errorf("chirpstack stop: %w", err)
		}
	}

	a.mu.RLock()
	name, id := a.agentConfig.Name, a.agentConfig.ID
	a.mu.RUnlock()
	releaseAgentName(name, id)
	return nil
}

// Pause 는 Running -> Paused 전이한다.
func (a *ChirpStackAgent) Pause(_ context.Context) error {
	return a.TransitionTo(lifecycle.StatePaused)
}

// Resume 은 Paused -> Running 전이한다.
func (a *ChirpStackAgent) Resume(_ context.Context) error {
	return a.TransitionTo(lifecycle.StateRunning)
}

// Process 는 사용하지 않는다 (수신 전용, 발행 경로 없음).
func (a *ChirpStackAgent) Process(_ []byte) ([]byte, error) {
	return nil, nil
}

// Configure 는 에이전트 설정을 업데이트한다.
func (a *ChirpStackAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("chirpstack configure: %w", err)
	}
	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()
	return nil
}

// ID 는 에이전트 ID 를 반환한다.
func (a *ChirpStackAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *ChirpStackAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *ChirpStackAgent) Type() string {
	return "chirpstack"
}

// Health 는 에이전트의 건강 상태를 반환한다 (M1: 라이프사이클 상태 기반).
func (a *ChirpStackAgent) Health() agent.HealthStatus {
	now := time.Now()
	switch a.CurrentState() {
	case lifecycle.StateRunning:
		return agent.HealthStatus{Status: agent.HealthHealthy, LastCheck: now, Message: "chirpstack is running"}
	case lifecycle.StatePaused:
		return agent.HealthStatus{Status: agent.HealthDegraded, LastCheck: now, Message: "chirpstack is paused"}
	default:
		return agent.HealthStatus{Status: agent.HealthUnhealthy, LastCheck: now, Message: fmt.Sprintf("chirpstack is in %s state", a.CurrentState())}
	}
}

// Info 는 에이전트 정보 스냅샷을 반환한다.
func (a *ChirpStackAgent) Info() agent.AgentInfo {
	a.mu.RLock()
	cfg := a.agentConfig
	startedAt := a.startedAt
	createdAt := a.createdAt
	a.mu.RUnlock()

	state := a.CurrentState()
	var uptime time.Duration
	if state == lifecycle.StateRunning && !startedAt.IsZero() {
		uptime = time.Since(startedAt)
	}

	return agent.AgentInfo{
		ID:        cfg.ID,
		Name:      cfg.Name,
		Type:      "chirpstack",
		State:     state,
		Health:    a.Health(),
		Config:    cfg,
		Stats:     a.stats.Snapshot(),
		StartedAt: startedAt,
		Uptime:    uptime,
		CreatedAt: createdAt,
	}
}

// Stats 는 통계 스냅샷을 반환한다.
func (a *ChirpStackAgent) Stats() agent.StatsSnapshot {
	return a.stats.Snapshot()
}
