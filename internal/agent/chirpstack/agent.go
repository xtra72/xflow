// Package chirpstack 는 ChirpStack LoRaWAN Network Server 가 발행하는 MQTT 업링크를
// 수신하여 measurement 당 1개 메시지로 fan-out 하는 에이전트를 제공한다
// (SPEC-CHIRPSTACK-001), 그리고 LoRaWAN 다운링크(제어) 발행 경로를 제공한다
// (SPEC-CHIRPSTACK-002).
//
// SPEC-CHIRPSTACK-001 은 본 에이전트를 "수신 전용"으로 정의하며 발행 경로를 의도적으로
// 배제했다. SPEC-CHIRPSTACK-002 REQ-M1-02 가 그 배제를 명시적으로 역전한다 — 수신 계약
// (REQ-FROZEN-A)은 그대로 보존되며, 발행은 추가된 능력이다(회귀 아님).
//
// 2계층 설계:
//   - 에이전트(본 패키지): MQTT 연결/구독/수신, 업링크 디코드, per-measurement
//     레코드 생성, 디바이스 자동 생성/메타데이터 노출, 다운링크 발행 프리미티브
//     (publish.go) + deviceProfile 별 다운링크 코덱(codec.go).
//   - 노드(internal/node/chirpstack.go): 수신 전용 SourceNode. 에이전트가 emit 한
//     per-measurement 레코드를 소비해 flow message 로 빌드하고 device 그룹을 승격.
//   - 노드(internal/node/chirpstack_control.go): typed command 를 코덱으로 인코딩해
//     다운링크 토픽으로 발행하는 제어 노드.
package chirpstack

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ChirpStackAgent 는 ChirpStack LoRaWAN 업링크를 수신하고, 다운링크(제어) 명령을
// 발행하는 에이전트이다.
//
// SPEC-CHIRPSTACK-001 은 "발행 경로(MessagePublisher)는 구현하지 않는다" 로 발행을
// 의도적으로 배제했으나, SPEC-CHIRPSTACK-002 REQ-M1-02 가 이 배제를 명시적으로
// 역전하여 PublishMessage(publish.go)를 구현한다. 수신 계약은 변경되지 않는다.
type ChirpStackAgent struct {
	*lifecycle.BaseLifecycle

	agentConfig agent.AgentConfig
	csConfig    ChirpStackConfig

	client   mqtt.Client
	recvCh   chan []byte
	done     chan struct{}
	doneOnce sync.Once

	// stopped 는 Stop() 이 호출되었음을 나타내는 가드 플래그이다 (mqtt_agent.go 이식).
	// Paho 의 백그라운드 재연결 goroutine 이 Stop 이후 재연결에 성공해도
	// subscribe() 가 stopped 상태이면 즉시 Disconnect 하여 세션 부활을 막는다.
	stopped atomic.Bool

	subscribedTopics []string
	topicsMu         sync.RWMutex

	// devices 는 devEui 키 자동 생성 디바이스 로스터이다 (M4, REQ-M4-01).
	devices   map[string]*deviceState
	devicesMu sync.RWMutex

	// comm 은 devEui 키 comm-state 추적 맵이다 (M5, REQ-M5-02/03). 로스터(devices)와
	// 분리해 comm-state watchdog 이 online/last-seen 을 독립적으로 관리한다.
	comm   map[string]*commEntry
	commMu sync.Mutex

	// watchdog goroutine 수명 제어 (REQ-M6-03: context 취소로 종료 보장).
	wdCancel  context.CancelFunc
	wdWg      sync.WaitGroup
	wdStarted bool // a.mu 보호

	stats  *agent.AgentStats
	logger *slog.Logger

	mu        sync.RWMutex
	startedAt time.Time
	createdAt time.Time
}

// 컴파일 타임 인터페이스 체크.
//
// MessagePublisher 는 SPEC-CHIRPSTACK-002 REQ-M1-02 로 추가되었다 — SPEC-CHIRPSTACK-001
// 의 발행 경로 배제를 역전한 기계적 신호이며, 회귀가 아니다(publish.go 구현).
var (
	_ agent.Agent            = (*ChirpStackAgent)(nil)
	_ agent.MessageReceiver  = (*ChirpStackAgent)(nil)
	_ agent.TransportChecker = (*ChirpStackAgent)(nil)
	_ agent.StatefulAgent    = (*ChirpStackAgent)(nil)
	_ agent.MessagePublisher = (*ChirpStackAgent)(nil)
)

// NewChirpStackAgent 는 ChirpStackAgent 팩토리 함수이다.
//
// 에이전트 이름은 배포 내에서 고유해야 한다(REQ-M1-04): device_id 가
// (agentName, devEui) 로 키잉되므로, 이름 충돌은 조용히 덮어쓰지 않고
// ErrNameCollision 으로 거부한다.
func NewChirpStackAgent(config agent.AgentConfig) (agent.Agent, error) {
	if err := claimAgentName(config.Name, config.ID); err != nil {
		return nil, err
	}

	cc := parseChirpStackConfig(config)

	a := &ChirpStackAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("chirpstack")),
		csConfig:      cc,
		recvCh:        make(chan []byte, cc.BufferSize),
		done:          make(chan struct{}),
		devices:       make(map[string]*deviceState),
		comm:          make(map[string]*commEntry),
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

// Init 은 에이전트를 초기화하고, 활성화 상태이면 MQTT 브로커에 연결/구독한다.
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
			"broker", a.csConfig.Broker,
			"client_id", a.csConfig.ClientID,
		)
		return nil
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("chirpstack init: %w", err)
	}
	if err := a.connect(); err != nil {
		return err
	}
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("chirpstack init: %w", err)
	}

	// comm-state watchdog 시작 (활성화 + emit_comm_state 시에만) (M5, REQ-M5-03/04).
	a.startCommWatchdog()

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()
	return nil
}

// connect 는 MQTT 클라이언트를 구성하고 브로커에 연결한다 (mqtt_agent.go Init 이식).
// AutoReconnect 시 초기 연결 실패를 치명적으로 보지 않고 degraded 로 진행한다
// (죽은 브로커가 플로우 시작을 막지 않게 함).
func (a *ChirpStackAgent) connect() error {
	opts := mqtt.NewClientOptions().
		AddBroker(a.csConfig.Broker).
		SetClientID(a.csConfig.ClientID).
		SetKeepAlive(time.Duration(a.csConfig.KeepAliveSec) * time.Second).
		SetAutoReconnect(a.csConfig.AutoReconnect).
		SetCleanSession(a.csConfig.CleanSession).
		SetConnectTimeout(time.Duration(a.csConfig.ConnectTimeoutSec) * time.Second).
		SetOrderMatters(false)

	if a.csConfig.AutoReconnect {
		opts.SetConnectRetry(true)
		opts.SetConnectRetryInterval(time.Duration(a.csConfig.ConnectTimeoutSec) * time.Second)
	}
	if a.csConfig.Username != "" {
		opts.SetUsername(a.csConfig.Username)
	}
	if a.csConfig.Password != "" {
		opts.SetPassword(a.csConfig.Password)
	}

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		a.logger.Info("chirpstack: 브로커에 연결됨",
			"broker", a.csConfig.Broker,
			"client_id", a.csConfig.ClientID,
		)
		a.subscribe(c)
	})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		a.logger.Warn("chirpstack: 연결 끊김", "broker", a.csConfig.Broker, "error", err)
	})
	opts.SetDefaultPublishHandler(a.messageHandler)

	// a.client 는 노드 goroutine 의 발행 경로(PublishMessage)와 TransportConnected 가
	// a.mu.RLock 으로 읽으므로, 대입도 a.mu 하에서 수행한다 (SPEC-CHIRPSTACK-002:
	// 발행 경로 추가로 넓어진 데이터 레이스 차단 — 동작 보존 최소 수정).
	client := mqtt.NewClient(opts)
	a.mu.Lock()
	a.client = client
	a.mu.Unlock()

	token := client.Connect()
	connected := token.WaitTimeout(time.Duration(a.csConfig.ConnectTimeoutSec)*time.Second) && token.Error() == nil
	if !connected {
		if a.csConfig.AutoReconnect {
			a.logger.Warn("chirpstack: 초기 연결 실패 — 백그라운드 재연결로 진행(degraded)",
				"broker", a.csConfig.Broker, "error", token.Error(),
			)
			return nil
		}
		_ = a.TransitionTo(lifecycle.StateError)
		if token.Error() != nil {
			return fmt.Errorf("chirpstack init: 연결 실패: %w", token.Error())
		}
		return fmt.Errorf("chirpstack init: 연결 타임아웃 (%s)", a.csConfig.Broker)
	}
	return nil
}

// subscribe 는 설정 토픽을 구독한다. stopped 상태이면 즉시 Disconnect 하여
// Stop 이후 재연결에 의한 세션 부활을 막는다 (mqtt_agent.go stopped-guard 이식).
func (a *ChirpStackAgent) subscribe(c mqtt.Client) {
	if a.stopped.Load() {
		a.logger.Info("chirpstack: Stop 이후 재연결 감지 — 재구독 없이 즉시 종료(세션 부활 방지)")
		c.Disconnect(0)
		return
	}

	a.topicsMu.Lock()
	if len(a.subscribedTopics) == 0 && len(a.csConfig.Topics) > 0 {
		a.subscribedTopics = make([]string, len(a.csConfig.Topics))
		copy(a.subscribedTopics, a.csConfig.Topics)
	}
	topics := make([]string, len(a.subscribedTopics))
	copy(topics, a.subscribedTopics)
	a.topicsMu.Unlock()

	for _, topic := range topics {
		token := c.Subscribe(topic, a.csConfig.QoS, nil)
		token.Wait()
		if token.Error() != nil {
			a.logger.Error("chirpstack: 토픽 구독 실패", "topic", topic, "error", token.Error())
		} else {
			a.logger.Info("chirpstack: 토픽 구독 완료", "topic", topic, "qos", a.csConfig.QoS)
		}
	}
}

// messageHandler 는 MQTT 업링크 수신 콜백이다. 업링크를 디코드/fan-out 한다.
func (a *ChirpStackAgent) messageHandler(_ mqtt.Client, msg mqtt.Message) {
	a.handleUplink(msg.Payload(), msg.Topic())
}

// handleUplink 는 원시 업링크를 디코드하여 measurement 당 1개 레코드로 fan-out 하고
// 각 레코드(JSON)를 수신 채널에 넣는다 (REQ-M3-01/02/05).
//
// 노드는 이 레코드를 소비해 flow message 로 빌드하고 device 그룹을 승격한다.
func (a *ChirpStackAgent) handleUplink(raw []byte, topic string) {
	up, err := decodeUplink(raw)
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		a.logger.Warn("chirpstack: 업링크 디코드 실패", "topic", topic, "error", err)
		return
	}

	// 디바이스 자동 생성/갱신 + UID 발급 + 런타임 info 등록 (M4, REQ-M4-01/02/04).
	a.upsertDevice(up)

	// comm-state fold: 활성화 시 last-seen 갱신 + online 전이 change emit (M5, REQ-M5-02).
	if a.csConfig.EmitCommState {
		a.onUplinkCommState(up)
	}

	records := buildMeasurementRecords(up, a.logger)
	for i := range records {
		b, err := json.Marshal(records[i])
		if err != nil {
			a.stats.IncrExternalMessagesErrored()
			a.logger.Warn("chirpstack: 레코드 직렬화 실패", "measurement", records[i].Measurement, "error", err)
			continue
		}
		a.enqueue(b, topic)
	}
}

// enqueue 는 바이트를 수신 채널에 넣고 통계를 갱신한다. 버퍼가 가득 차면 드롭한다.
func (a *ChirpStackAgent) enqueue(data []byte, topic string) {
	select {
	case a.recvCh <- data:
		a.stats.IncrExternalMessagesReceived()
		a.stats.AddBytesRead(int64(len(data)))
		a.stats.UpdateLastActivity()
	default:
		a.stats.IncrExternalMessagesErrored()
		a.logger.Warn("chirpstack: 버퍼 가득 참, 메시지 드롭", "topic", topic)
	}
}

// ReceiveMessage 는 수신 채널에서 메시지를 가져온다 (agent.MessageReceiver).
func (a *ChirpStackAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case data := <-a.recvCh:
		return data, nil
	case <-a.done:
		return nil, fmt.Errorf("chirpstack: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Start 는 이미 Running 이면 no-op, Stopped/Created 이면 재-Init 하여 재연결한다.
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

// Stop 은 MQTT 구독을 해제하고 연결을 종료한다. idempotent 하며, stopped 가드로
// Stop 이후 Paho 재연결에 의한 세션 부활을 막는다.
func (a *ChirpStackAgent) Stop(_ context.Context) error {
	a.stopped.Store(true)

	// comm-state watchdog goroutine 종료 (context 취소 + 대기) (M5, REQ-M6-03).
	a.stopCommWatchdog()

	// Created(비활성화 생성) / Stopped 는 Stopping 전이가 invalid 하므로 건너뛴다.
	// 그래도 done close / 버퍼 드레인 / 이름 해제는 항상 수행한다.
	state := a.CurrentState()
	transition := state != lifecycle.StateStopped && state != lifecycle.StateCreated
	if transition {
		if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
			return fmt.Errorf("chirpstack stop: %w", err)
		}
	}

	if a.client != nil {
		if a.client.IsConnected() {
			for _, topic := range a.csConfig.Topics {
				token := a.client.Unsubscribe(topic)
				token.Wait()
			}
		}
		a.client.Disconnect(250)
	}

	a.doneOnce.Do(func() { close(a.done) })

	// 버퍼 드레인.
	for {
		select {
		case <-a.recvCh:
		default:
			if transition {
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
	}
}

// Pause 는 Running -> Paused 전이한다.
func (a *ChirpStackAgent) Pause(_ context.Context) error {
	return a.TransitionTo(lifecycle.StatePaused)
}

// Resume 은 Paused -> Running 전이한다.
func (a *ChirpStackAgent) Resume(_ context.Context) error {
	return a.TransitionTo(lifecycle.StateRunning)
}

// Process 는 사용하지 않는다. 다운링크 발행은 PublishMessage 인터페이스를 사용한다
// (SPEC-CHIRPSTACK-002 REQ-M1-02 — SPEC-CHIRPSTACK-001 의 "발행 경로 없음" 배제 역전).
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

// TransportConnected 는 실제 Paho 클라이언트의 연결 여부를 반환한다
// (agent.TransportChecker).
func (a *ChirpStackAgent) TransportConnected() bool {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()
	return client != nil && client.IsConnected()
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

// Health 는 에이전트의 건강 상태를 반환한다.
func (a *ChirpStackAgent) Health() agent.HealthStatus {
	now := time.Now()
	switch a.CurrentState() {
	case lifecycle.StateRunning:
		if a.TransportConnected() {
			return agent.HealthStatus{Status: agent.HealthHealthy, LastCheck: now, Message: "chirpstack is running and connected"}
		}
		return agent.HealthStatus{Status: agent.HealthDegraded, LastCheck: now, Message: "chirpstack is running but disconnected (reconnecting)"}
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

// State 는 타입별 런타임 상태를 반환한다 (agent.StatefulAgent).
func (a *ChirpStackAgent) State() map[string]any {
	a.topicsMu.RLock()
	topics := make([]string, len(a.subscribedTopics))
	copy(topics, a.subscribedTopics)
	a.topicsMu.RUnlock()
	if len(topics) == 0 {
		topics = append(topics, a.csConfig.Topics...)
	}

	return map[string]any{
		"broker":    a.csConfig.Broker,
		"client_id": a.csConfig.ClientID,
		"connected": a.TransportConnected(),
		"qos":       a.csConfig.QoS,
		"topics":    topics,
	}
}
