package system

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// MQTTSubscriberConfig 는 MQTT Subscriber 에이전트의 설정이다.
type MQTTSubscriberConfig struct {
	// Broker 는 MQTT 브로커 주소이다 (예: "tcp://localhost:1883").
	Broker string `json:"broker"`

	// ClientID 는 MQTT 클라이언트 식별자이다.
	ClientID string `json:"client_id"`

	// Username 은 MQTT 인증 사용자명이다.
	Username string `json:"username"`

	// Password 는 MQTT 인증 비밀번호이다.
	Password string `json:"password"`

	// Topics 는 구독할 토픽 목록이다.
	Topics []string `json:"topics"`

	// QoS 는 메시지 전달 보증 레벨이다 (0, 1, 2).
	QoS byte `json:"qos"`

	// KeepAliveSec 는 연결 유지 간격(초)이다.
	KeepAliveSec int `json:"keep_alive_sec"`

	// AutoReconnect 는 자동 재연결 활성화 여부이다.
	AutoReconnect bool `json:"auto_reconnect"`

	// CleanSession 은 클린 세션 사용 여부이다.
	CleanSession bool `json:"clean_session"`

	// BufferSize 는 수신 버퍼 크기이다.
	BufferSize int `json:"buffer_size"`

	// ConnectTimeoutSec 는 연결 타임아웃(초)이다.
	ConnectTimeoutSec int `json:"connect_timeout_sec"`
}

// parseMQTTSubscriberConfig 는 AgentConfig에서 MQTTSubscriberConfig를 파싱한다.
func parseMQTTSubscriberConfig(cfg agent.AgentConfig) MQTTSubscriberConfig {
	mc := MQTTSubscriberConfig{
		Broker:            "tcp://localhost:1883",
		ClientID:          "xflow-mqtt-001",
		QoS:               1,
		KeepAliveSec:      60,
		AutoReconnect:     true,
		CleanSession:      true,
		BufferSize:        256,
		ConnectTimeoutSec: 10,
	}

	opts := cfg.Transport.Options
	if opts == nil {
		return mc
	}

	if v, ok := opts["broker"].(string); ok && v != "" {
		mc.Broker = v
	}
	if v, ok := opts["client_id"].(string); ok && v != "" {
		mc.ClientID = v
	}
	if v, ok := opts["username"].(string); ok {
		mc.Username = v
	}
	if v, ok := opts["password"].(string); ok {
		mc.Password = v
	}
	if v, ok := opts["topics"]; ok {
		mc.Topics = toStringSlice(v)
	}
	if v, ok := opts["qos"]; ok {
		mc.QoS = byte(toInt(v))
	}
	if v, ok := opts["keep_alive_sec"]; ok {
		mc.KeepAliveSec = toInt(v)
	}
	if v, ok := opts["auto_reconnect"].(bool); ok {
		mc.AutoReconnect = v
	}
	if v, ok := opts["clean_session"].(bool); ok {
		mc.CleanSession = v
	}
	if v, ok := opts["buffer_size"]; ok {
		mc.BufferSize = toInt(v)
	}
	if v, ok := opts["connect_timeout_sec"]; ok {
		mc.ConnectTimeoutSec = toInt(v)
	}

	return mc
}

// MQTTSubscriberAgent 는 MQTT 브로커에서 메시지를 구독하는 에이전트이다.
// agent.Agent 와 agent.MessageReceiver 인터페이스를 구현한다.
type MQTTSubscriberAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig agent.AgentConfig
	mqttConfig  MQTTSubscriberConfig
	client      mqtt.Client
	recvCh      chan []byte
	done        chan struct{}
	stats       *agent.AgentStats
	logger      *slog.Logger
	mu          sync.RWMutex
	startedAt   time.Time
	createdAt   time.Time
}

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*MQTTSubscriberAgent)(nil)
var _ agent.MessageReceiver = (*MQTTSubscriberAgent)(nil)

// NewMQTTSubscriberAgent 는 MQTTSubscriberAgent 팩토리 함수이다.
func NewMQTTSubscriberAgent(config agent.AgentConfig) (agent.Agent, error) {
	mc := parseMQTTSubscriberConfig(config)

	a := &MQTTSubscriberAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("mqtt-subscriber")),
		mqttConfig:    mc,
		recvCh:        make(chan []byte, mc.BufferSize),
		done:          make(chan struct{}),
		stats:         agent.NewAgentStats(),
		logger:        slog.Default(),
		createdAt:     time.Now(),
	}

	if err := a.Init(config); err != nil {
		return nil, err
	}

	return a, nil
}

// Init 은 에이전트를 초기화하고 MQTT 브로커에 연결한다.
func (a *MQTTSubscriberAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("mqtt-subscriber init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("mqtt-subscriber init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	// MQTT 클라이언트 옵션 구성
	opts := mqtt.NewClientOptions().
		AddBroker(a.mqttConfig.Broker).
		SetClientID(a.mqttConfig.ClientID).
		SetKeepAlive(time.Duration(a.mqttConfig.KeepAliveSec) * time.Second).
		SetAutoReconnect(a.mqttConfig.AutoReconnect).
		SetCleanSession(a.mqttConfig.CleanSession).
		SetConnectTimeout(time.Duration(a.mqttConfig.ConnectTimeoutSec) * time.Second).
		SetOrderMatters(false)

	if a.mqttConfig.Username != "" {
		opts.SetUsername(a.mqttConfig.Username)
	}
	if a.mqttConfig.Password != "" {
		opts.SetPassword(a.mqttConfig.Password)
	}

	// 연결 성공 시 토픽 구독 (재연결 시에도 자동 재구독)
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		a.logger.Info("mqtt-subscriber: 브로커에 연결됨",
			"broker", a.mqttConfig.Broker,
			"client_id", a.mqttConfig.ClientID,
		)
		a.subscribe(c)
	})

	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		a.logger.Warn("mqtt-subscriber: 연결 끊김",
			"broker", a.mqttConfig.Broker,
			"error", err,
		)
	})

	// 메시지 수신 기본 핸들러
	opts.SetDefaultPublishHandler(a.messageHandler)

	// MQTT 클라이언트 생성 및 연결
	a.client = mqtt.NewClient(opts)
	token := a.client.Connect()
	if !token.WaitTimeout(time.Duration(a.mqttConfig.ConnectTimeoutSec) * time.Second) {
		_ = a.TransitionTo(lifecycle.StateError)
		return fmt.Errorf("mqtt-subscriber init: 연결 타임아웃 (%s)", a.mqttConfig.Broker)
	}
	if token.Error() != nil {
		_ = a.TransitionTo(lifecycle.StateError)
		return fmt.Errorf("mqtt-subscriber init: 연결 실패: %w", token.Error())
	}

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("mqtt-subscriber init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	return nil
}

// subscribe 는 설정된 토픽들을 구독한다.
func (a *MQTTSubscriberAgent) subscribe(c mqtt.Client) {
	for _, topic := range a.mqttConfig.Topics {
		token := c.Subscribe(topic, a.mqttConfig.QoS, nil)
		token.Wait()
		if token.Error() != nil {
			a.logger.Error("mqtt-subscriber: 토픽 구독 실패",
				"topic", topic,
				"error", token.Error(),
			)
		} else {
			a.logger.Info("mqtt-subscriber: 토픽 구독 완료",
				"topic", topic,
				"qos", a.mqttConfig.QoS,
			)
		}
	}
}

// messageHandler 는 MQTT 메시지 수신 콜백이다.
func (a *MQTTSubscriberAgent) messageHandler(_ mqtt.Client, msg mqtt.Message) {
	data := make([]byte, len(msg.Payload()))
	copy(data, msg.Payload())

	select {
	case a.recvCh <- data:
		a.stats.IncrMessagesReceived()
		a.stats.AddBytesRead(int64(len(data)))
		a.stats.UpdateLastActivity()
	default:
		a.stats.IncrMessagesErrored()
		a.logger.Warn("mqtt-subscriber: 버퍼 가득 참, 메시지 드롭",
			"topic", msg.Topic(),
		)
	}
}

// ReceiveMessage 는 수신 채널에서 메시지를 가져온다.
// agent.MessageReceiver 인터페이스 구현.
func (a *MQTTSubscriberAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case data := <-a.recvCh:
		return data, nil
	case <-a.done:
		return nil, fmt.Errorf("mqtt-subscriber: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Start 는 이미 Running 상태이면 no-op이다.
func (a *MQTTSubscriberAgent) Start(_ context.Context) error {
	if a.CurrentState() == lifecycle.StateRunning {
		return nil
	}
	return fmt.Errorf("mqtt-subscriber start: not in running state (current: %s)", a.CurrentState())
}

// Stop 은 MQTT 구독을 해제하고, 클라이언트 연결을 종료한다.
func (a *MQTTSubscriberAgent) Stop(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("mqtt-subscriber stop: %w", err)
	}

	// 1. 토픽 구독 해제
	if a.client != nil && a.client.IsConnected() {
		for _, topic := range a.mqttConfig.Topics {
			token := a.client.Unsubscribe(topic)
			token.Wait()
		}
		// 250ms 대기 후 연결 종료
		a.client.Disconnect(250)
	}

	// 2. ReceiveMessage 대기자에게 종료 시그널
	close(a.done)

	// 3. 버퍼에 남은 메시지 드레인
	for {
		select {
		case <-a.recvCh:
			// 드레인
		default:
			if err := a.TransitionTo(lifecycle.StateStopped); err != nil {
				return fmt.Errorf("mqtt-subscriber stop: %w", err)
			}
			return nil
		}
	}
}

// Pause 는 Running -> Paused 전이한다.
func (a *MQTTSubscriberAgent) Pause(_ context.Context) error {
	return a.TransitionTo(lifecycle.StatePaused)
}

// Resume 은 Paused -> Running 전이한다.
func (a *MQTTSubscriberAgent) Resume(_ context.Context) error {
	return a.TransitionTo(lifecycle.StateRunning)
}

// Health 는 에이전트의 건강 상태를 반환한다.
func (a *MQTTSubscriberAgent) Health() agent.HealthStatus {
	now := time.Now()
	state := a.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		if a.client != nil && a.client.IsConnected() {
			return agent.HealthStatus{
				Status:    agent.HealthHealthy,
				LastCheck: now,
				Message:   "mqtt-subscriber is running and connected",
			}
		}
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "mqtt-subscriber is running but disconnected (reconnecting)",
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "mqtt-subscriber is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("mqtt-subscriber is in %s state", state),
		}
	}
}

// Process 는 MQTT Subscriber에서는 사용하지 않는다 (수신 전용).
func (a *MQTTSubscriberAgent) Process(_ []byte) ([]byte, error) {
	return nil, nil
}

// Configure 는 에이전트 설정을 업데이트한다.
func (a *MQTTSubscriberAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("mqtt-subscriber configure: %w", err)
	}
	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()
	return nil
}

// ID 는 에이전트 ID를 반환한다.
func (a *MQTTSubscriberAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *MQTTSubscriberAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *MQTTSubscriberAgent) Type() string {
	return "mqtt"
}

// Info 는 에이전트 정보 스냅샷을 반환한다.
func (a *MQTTSubscriberAgent) Info() agent.AgentInfo {
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
		Type:      "mqtt",
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
func (a *MQTTSubscriberAgent) Stats() agent.StatsSnapshot {
	return a.stats.Snapshot()
}

// toStringSlice 는 인터페이스 값을 []string으로 변환한다.
func toStringSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		result := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	default:
		return nil
	}
}
