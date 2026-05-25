package system

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// MQTTConfig 는 MQTT 에이전트의 설정이다.
type MQTTConfig struct {
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

	// MaxPubTopics 는 발행 토픽 통계의 최대 추적 수이다.
	// 초과 시 가장 오래된 토픽이 제거된다. 기본값 100.
	MaxPubTopics int `json:"max_pub_topics"`
}

// topicStat 는 개별 토픽의 메시지 통계이다.
type topicStat struct {
	Count     int64     `json:"count"`
	Bytes     int64     `json:"bytes"`
	UpdatedAt time.Time `json:"updated_at"`
}

// parseMQTTConfig 는 AgentConfig에서 MQTTConfig를 파싱한다.
func parseMQTTConfig(cfg agent.AgentConfig) MQTTConfig {
	mc := MQTTConfig{
		Broker:            "tcp://localhost:1883",
		ClientID:          "xflow-" + uuid.New().String(),
		QoS:               1,
		KeepAliveSec:      60,
		AutoReconnect:     true,
		CleanSession:      true,
		BufferSize:        256,
		ConnectTimeoutSec: 10,
		MaxPubTopics:      100,
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
	if v, ok := opts["max_pub_topics"]; ok {
		if n := toInt(v); n > 0 {
			mc.MaxPubTopics = n
		}
	}

	return mc
}

// MQTTAgent 는 MQTT 브로커와 연동하는 에이전트이다.
// 구독(MessageReceiver, SubscriberAgent)과 발행(MessagePublisher)을 모두 지원한다.
type MQTTAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig      agent.AgentConfig
	mqttConfig       MQTTConfig
	client           mqtt.Client
	recvCh           chan []byte
	done             chan struct{}
	doneOnce         sync.Once
	stats            *agent.AgentStats
	logger           *slog.Logger
	mu               sync.RWMutex
	startedAt        time.Time
	createdAt        time.Time
	subscribedTopics []string              // 현재 구독 중인 토픽 목록
	topicsMu         sync.RWMutex          // subscribedTopics 보호용
	subTopicStats    map[string]*topicStat // 구독 토픽별 수신 통계
	pubTopicStats    map[string]*topicStat // 발행 토픽별 송신 통계
	topicStatsMu     sync.RWMutex          // subTopicStats, pubTopicStats 보호용
}

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*MQTTAgent)(nil)
var _ agent.MessageReceiver = (*MQTTAgent)(nil)
var _ agent.SubscriberAgent = (*MQTTAgent)(nil)
var _ agent.StatefulAgent = (*MQTTAgent)(nil)
var _ agent.BufferInfoProvider = (*MQTTAgent)(nil)
var _ agent.MessagePublisher = (*MQTTAgent)(nil)

// NewMQTTAgent 는 MQTTAgent 팩토리 함수이다.
func NewMQTTAgent(config agent.AgentConfig) (agent.Agent, error) {
	mc := parseMQTTConfig(config)

	// client_id 자동 생성 여부 확인 및 로깅
	userSetClientID := false
	if opts := config.Transport.Options; opts != nil {
		if v, ok := opts["client_id"].(string); ok && v != "" {
			userSetClientID = true
		}
	}
	if !userSetClientID {
		slog.Info("mqtt: client_id 자동 생성됨",
			"client_id", mc.ClientID,
		)
	}

	a := &MQTTAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("mqtt-client")),
		mqttConfig:    mc,
		recvCh:        make(chan []byte, mc.BufferSize),
		done:          make(chan struct{}),
		stats:         agent.NewAgentStats(),
		logger:        agent.ResolveLogger(config),
		createdAt:     time.Now(),
		subTopicStats: make(map[string]*topicStat),
		pubTopicStats: make(map[string]*topicStat),
	}

	if err := a.Init(config); err != nil {
		return nil, err
	}

	return a, nil
}

// Init 은 에이전트를 초기화하고 MQTT 브로커에 연결한다.
func (a *MQTTAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("mqtt init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("mqtt init: %w", err)
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
		a.logger.Info("mqtt: 브로커에 연결됨",
			"broker", a.mqttConfig.Broker,
			"client_id", a.mqttConfig.ClientID,
		)
		a.subscribe(c)
	})

	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		a.logger.Warn("mqtt: 연결 끊김",
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
		return fmt.Errorf("mqtt init: 연결 타임아웃 (%s)", a.mqttConfig.Broker)
	}
	if token.Error() != nil {
		_ = a.TransitionTo(lifecycle.StateError)
		return fmt.Errorf("mqtt init: 연결 실패: %w", token.Error())
	}

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("mqtt init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	return nil
}

// subscribe 는 현재 구독 중인 모든 토픽을 구독한다.
// 초기 연결 시에는 설정 토픽으로 subscribedTopics를 초기화하고,
// 재연결 시에는 subscribedTopics의 모든 토픽(Bridge가 추가한 토픽 포함)을 복원한다.
func (a *MQTTAgent) subscribe(c mqtt.Client) {
	a.topicsMu.Lock()
	if len(a.subscribedTopics) == 0 && len(a.mqttConfig.Topics) > 0 {
		// 초기 연결: 설정 토픽으로 초기화
		a.subscribedTopics = make([]string, len(a.mqttConfig.Topics))
		copy(a.subscribedTopics, a.mqttConfig.Topics)
	}
	// 재연결 시에도 모든 구독 토픽(Bridge 추가 포함)을 복원
	topics := make([]string, len(a.subscribedTopics))
	copy(topics, a.subscribedTopics)
	a.topicsMu.Unlock()

	for _, topic := range topics {
		token := c.Subscribe(topic, a.mqttConfig.QoS, nil)
		token.Wait()
		if token.Error() != nil {
			a.logger.Error("mqtt: 토픽 구독 실패",
				"topic", topic,
				"error", token.Error(),
			)
		} else {
			a.logger.Info("mqtt: 토픽 구독 완료",
				"topic", topic,
				"qos", a.mqttConfig.QoS,
			)
		}
	}
}

// Subscribe 는 동적으로 토픽을 구독한다.
// agent.SubscriberAgent 인터페이스 구현.
func (a *MQTTAgent) Subscribe(_ context.Context, topics []string) error {
	// 토픽을 먼저 저장한다. 브로커 미연결 시에도 OnConnectHandler가
	// subscribedTopics를 자동 구독하므로 연결 후 자동으로 구독된다.
	a.topicsMu.Lock()
	a.subscribedTopics = append(a.subscribedTopics, topics...)
	a.topicsMu.Unlock()

	// 브로커에 연결된 상태면 즉시 구독한다.
	if a.client != nil && a.client.IsConnected() {
		for _, topic := range topics {
			token := a.client.Subscribe(topic, a.mqttConfig.QoS, nil)
			token.Wait()
			if token.Error() != nil {
				return fmt.Errorf("mqtt subscribe: 토픽 %q 구독 실패: %w", topic, token.Error())
			}
			a.logger.Info("mqtt: 동적 토픽 구독 완료",
				"topic", topic,
				"qos", a.mqttConfig.QoS,
			)
		}
	} else {
		a.logger.Info("mqtt: 브로커 미연결 상태, 토픽 등록 완료 (연결 시 자동 구독)",
			"topics", topics,
		)
	}

	return nil
}

// Unsubscribe 는 동적으로 토픽 구독을 해제한다.
// agent.SubscriberAgent 인터페이스 구현.
func (a *MQTTAgent) Unsubscribe(_ context.Context, topics []string) error {
	if a.client == nil || !a.client.IsConnected() {
		return fmt.Errorf("mqtt unsubscribe: 브로커에 연결되지 않음")
	}

	token := a.client.Unsubscribe(topics...)
	token.Wait()
	if token.Error() != nil {
		return fmt.Errorf("mqtt unsubscribe: %w", token.Error())
	}

	a.topicsMu.Lock()
	a.subscribedTopics = removeTopics(a.subscribedTopics, topics)
	a.topicsMu.Unlock()

	for _, topic := range topics {
		a.logger.Info("mqtt: 토픽 구독 해제 완료",
			"topic", topic,
		)
	}

	return nil
}

// removeTopics 는 목록에서 지정된 토픽들을 제거한다.
func removeTopics(list []string, toRemove []string) []string {
	removeSet := make(map[string]bool, len(toRemove))
	for _, t := range toRemove {
		removeSet[t] = true
	}
	result := make([]string, 0, len(list))
	for _, t := range list {
		if !removeSet[t] {
			result = append(result, t)
		}
	}
	return result
}

// messageHandler 는 MQTT 메시지 수신 콜백이다.
func (a *MQTTAgent) messageHandler(_ mqtt.Client, msg mqtt.Message) {
	data := make([]byte, len(msg.Payload()))
	copy(data, msg.Payload())

	select {
	case a.recvCh <- data:
		a.stats.IncrExternalMessagesReceived()
		a.stats.AddBytesRead(int64(len(data)))
		a.stats.UpdateLastActivity()
		a.recordSubTopicStat(msg.Topic(), int64(len(data)))
	default:
		a.stats.IncrExternalMessagesErrored()
		a.logger.Warn("mqtt: 버퍼 가득 참, 메시지 드롭",
			"topic", msg.Topic(),
		)
	}
}

// ReceiveMessage 는 수신 채널에서 메시지를 가져온다.
// agent.MessageReceiver 인터페이스 구현.
func (a *MQTTAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case data := <-a.recvCh:
		return data, nil
	case <-a.done:
		return nil, fmt.Errorf("mqtt: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Start 는 이미 Running 상태이면 no-op이다.
// Stopped 상태이면 Created로 리셋 후 Init()을 재호출하여 재연결한다.
func (a *MQTTAgent) Start(_ context.Context) error {
	switch a.CurrentState() {
	case lifecycle.StateRunning:
		return nil
	case lifecycle.StateStopped:
		// Stopped → Created → Init() 재호출
		if err := a.TransitionTo(lifecycle.StateCreated); err != nil {
			return fmt.Errorf("mqtt start: reset to created: %w", err)
		}
		a.mu.RLock()
		cfg := a.agentConfig
		a.mu.RUnlock()
		return a.Init(cfg)
	default:
		return fmt.Errorf("mqtt start: not in running state (current: %s)", a.CurrentState())
	}
}

// Stop 은 MQTT 구독을 해제하고, 클라이언트 연결을 종료한다.
func (a *MQTTAgent) Stop(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("mqtt stop: %w", err)
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

	// 2. ReceiveMessage 대기자에게 종료 시그널 (중복 Stop 호출 시 panic 방지)
	a.doneOnce.Do(func() { close(a.done) })

	// 3. 버퍼에 남은 메시지 드레인
	for {
		select {
		case <-a.recvCh:
			// 드레인
		default:
			if err := a.TransitionTo(lifecycle.StateStopped); err != nil {
				return fmt.Errorf("mqtt stop: %w", err)
			}
			return nil
		}
	}
}

// Pause 는 Running -> Paused 전이한다.
func (a *MQTTAgent) Pause(_ context.Context) error {
	return a.TransitionTo(lifecycle.StatePaused)
}

// Resume 은 Paused -> Running 전이한다.
func (a *MQTTAgent) Resume(_ context.Context) error {
	return a.TransitionTo(lifecycle.StateRunning)
}

// Health 는 에이전트의 건강 상태를 반환한다.
func (a *MQTTAgent) Health() agent.HealthStatus {
	now := time.Now()
	state := a.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		if a.client != nil && a.client.IsConnected() {
			return agent.HealthStatus{
				Status:    agent.HealthHealthy,
				LastCheck: now,
				Message:   "mqtt is running and connected",
			}
		}
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "mqtt is running but disconnected (reconnecting)",
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "mqtt is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("mqtt is in %s state", state),
		}
	}
}

// Process 는 사용하지 않는다. 발행은 PublishMessage 인터페이스를 사용한다.
func (a *MQTTAgent) Process(_ []byte) ([]byte, error) {
	return nil, nil
}

// PublishMessage 는 MQTT 브로커에 메시지를 발행한다.
// BridgeOut 방향에서 어댑터가 변환한 토픽/QoS/Retained 정보와 함께 페이로드를 전송한다.
func (a *MQTTAgent) PublishMessage(topic string, qos byte, retained bool, payload []byte) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("mqtt: 브로커에 연결되어 있지 않음")
	}

	if topic == "" {
		return fmt.Errorf("mqtt: 발행 토픽이 지정되지 않음")
	}

	token := client.Publish(topic, qos, retained, payload)
	token.Wait()
	if token.Error() != nil {
		a.stats.IncrExternalMessagesErrored()
		return fmt.Errorf("mqtt: 발행 실패: %w", token.Error())
	}

	a.stats.IncrExternalMessagesSent()
	a.recordPubTopicStat(topic, int64(len(payload)))
	a.logger.Debug("mqtt: 메시지 발행 완료",
		"topic", topic,
		"qos", qos,
		"retained", retained,
		"bytes", len(payload),
		"payload", formatMQTTPayloadForLog(payload),
	)

	return nil
}

// formatMQTTPayloadForLog 은 payload 를 debug log 용 문자열로 변환한다.
// 텍스트 (UTF-8, JSON 등) 면 string 으로, 그 외 바이너리는 hex 로 표기.
// 1024 바이트 초과 시 잘라내고 truncation 표시.
func formatMQTTPayloadForLog(payload []byte) string {
	const maxLen = 1024
	if isPrintableText(payload) {
		s := string(payload)
		if len(s) > maxLen {
			return s[:maxLen] + "...(truncated)"
		}
		return s
	}
	// 바이너리: hex.
	if len(payload) > maxLen/2 {
		return hex.EncodeToString(payload[:maxLen/2]) + "...(truncated)"
	}
	return hex.EncodeToString(payload)
}

// isPrintableText 는 byte slice 가 printable UTF-8 텍스트인지 판정한다.
// 제어 문자 (\t, \n, \r 외) 가 포함되면 false.
func isPrintableText(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	for _, c := range b {
		if c < 0x20 && c != '\t' && c != '\n' && c != '\r' {
			return false
		}
		if c == 0x7f {
			return false
		}
	}
	return true
}

// Configure 는 에이전트 설정을 업데이트한다.
func (a *MQTTAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("mqtt configure: %w", err)
	}
	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()
	return nil
}

// ID 는 에이전트 ID를 반환한다.
func (a *MQTTAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *MQTTAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *MQTTAgent) Type() string {
	return "mqtt-client"
}

// Info 는 에이전트 정보 스냅샷을 반환한다.
func (a *MQTTAgent) Info() agent.AgentInfo {
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
		Type:      "mqtt-client",
		State:     state,
		Health:    a.Health(),
		Config:    cfg,
		Stats:     a.stats.Snapshot(),
		StartedAt: startedAt,
		Uptime:    uptime,
		CreatedAt: createdAt,
	}
}

// BufferInfo returns the pending and capacity of the receive buffer.
func (a *MQTTAgent) BufferInfo() (int, int) {
	return len(a.recvCh), cap(a.recvCh)
}

// Stats 는 통계 스냅샷을 반환한다.
func (a *MQTTAgent) Stats() agent.StatsSnapshot {
	s := a.stats.Snapshot()
	s.MsgBufferPending, s.MsgBufferCapacity = a.BufferInfo()
	return s
}

// recordSubTopicStat 는 구독 토픽의 수신 통계를 기록한다.
func (a *MQTTAgent) recordSubTopicStat(topic string, bytes int64) {
	a.topicStatsMu.Lock()
	defer a.topicStatsMu.Unlock()

	ts, ok := a.subTopicStats[topic]
	if !ok {
		ts = &topicStat{}
		a.subTopicStats[topic] = ts
	}
	ts.Count++
	ts.Bytes += bytes
	ts.UpdatedAt = time.Now()
}

// recordPubTopicStat 는 발행 토픽의 송신 통계를 기록한다.
// MaxPubTopics 를 초과하면 가장 오래된 토픽을 제거한다.
func (a *MQTTAgent) recordPubTopicStat(topic string, bytes int64) {
	a.topicStatsMu.Lock()
	defer a.topicStatsMu.Unlock()

	ts, ok := a.pubTopicStats[topic]
	if !ok {
		// LRU 퇴출: 최대 수 초과 시 가장 오래된 토픽 제거
		if len(a.pubTopicStats) >= a.mqttConfig.MaxPubTopics {
			var oldestKey string
			var oldestTime time.Time
			for k, v := range a.pubTopicStats {
				if oldestKey == "" || v.UpdatedAt.Before(oldestTime) {
					oldestKey = k
					oldestTime = v.UpdatedAt
				}
			}
			if oldestKey != "" {
				delete(a.pubTopicStats, oldestKey)
			}
		}
		ts = &topicStat{}
		a.pubTopicStats[topic] = ts
	}
	ts.Count++
	ts.Bytes += bytes
	ts.UpdatedAt = time.Now()
}

// topicStatSnapshot 는 토픽 통계의 직렬화 가능한 스냅샷이다.
type topicStatSnapshot struct {
	Topic     string `json:"topic"`
	Count     int64  `json:"count"`
	Bytes     int64  `json:"bytes"`
	UpdatedAt string `json:"updated_at"`
}

// mqttTopicMatch 는 MQTT 토픽 패턴과 실제 토픽의 매칭 여부를 반환한다.
// '+' 는 단일 레벨, '#' 는 나머지 모든 레벨과 매칭된다.
func mqttTopicMatch(pattern, topic string) bool {
	pParts := strings.Split(pattern, "/")
	tParts := strings.Split(topic, "/")

	for i, p := range pParts {
		if p == "#" {
			return true // '#'는 나머지 전부 매칭
		}
		if i >= len(tParts) {
			return false
		}
		if p != "+" && p != tParts[i] {
			return false
		}
	}
	return len(pParts) == len(tParts)
}

// State 는 MQTT 에이전트의 런타임 상태를 반환한다.
// 브로커 연결 정보, 구독/발행 토픽 목록과 토픽별 통계를 포함한다.
// agent.StatefulAgent 인터페이스 구현.
func (a *MQTTAgent) State() map[string]any {
	a.topicsMu.RLock()
	topics := make([]string, len(a.subscribedTopics))
	copy(topics, a.subscribedTopics)
	a.topicsMu.RUnlock()

	// 브로커에 아직 연결되지 않은 경우 설정된 토픽 목록을 사용
	if len(topics) == 0 {
		topics = make([]string, len(a.mqttConfig.Topics))
		copy(topics, a.mqttConfig.Topics)
	}

	connected := a.client != nil && a.client.IsConnected()

	// 구독 토픽별 트리 구조 (구독 패턴 → 수신 토픽 매칭)
	a.topicStatsMu.RLock()

	type subscribedEntry struct {
		Topic          string              `json:"topic"`
		QoS            byte                `json:"qos"`
		TotalCount     int64               `json:"total_count"`
		TotalBytes     int64               `json:"total_bytes"`
		ReceivedTopics []topicStatSnapshot `json:"received_topics"`
	}

	// 매칭되지 않은 수신 토픽을 추적
	unmatchedRecv := make(map[string]bool, len(a.subTopicStats))
	for t := range a.subTopicStats {
		unmatchedRecv[t] = true
	}

	subscribedTopics := make([]subscribedEntry, 0, len(topics))
	for _, pattern := range topics {
		entry := subscribedEntry{Topic: pattern, QoS: a.mqttConfig.QoS}

		for t, ts := range a.subTopicStats {
			if mqttTopicMatch(pattern, t) {
				entry.ReceivedTopics = append(entry.ReceivedTopics, topicStatSnapshot{
					Topic:     t,
					Count:     ts.Count,
					Bytes:     ts.Bytes,
					UpdatedAt: ts.UpdatedAt.Format(time.RFC3339),
				})
				entry.TotalCount += ts.Count
				entry.TotalBytes += ts.Bytes
				delete(unmatchedRecv, t)
			}
		}

		if entry.ReceivedTopics == nil {
			entry.ReceivedTopics = []topicStatSnapshot{}
		}
		subscribedTopics = append(subscribedTopics, entry)
	}

	// 매칭되지 않은 수신 토픽 (구독 패턴 없이 수신된 토픽)
	unmatchedTopics := make([]topicStatSnapshot, 0)
	for t := range unmatchedRecv {
		ts := a.subTopicStats[t]
		unmatchedTopics = append(unmatchedTopics, topicStatSnapshot{
			Topic:     t,
			Count:     ts.Count,
			Bytes:     ts.Bytes,
			UpdatedAt: ts.UpdatedAt.Format(time.RFC3339),
		})
	}

	// 발행 토픽별 통계
	pubStats := make([]topicStatSnapshot, 0, len(a.pubTopicStats))
	for t, ts := range a.pubTopicStats {
		pubStats = append(pubStats, topicStatSnapshot{
			Topic:     t,
			Count:     ts.Count,
			Bytes:     ts.Bytes,
			UpdatedAt: ts.UpdatedAt.Format(time.RFC3339),
		})
	}
	a.topicStatsMu.RUnlock()

	return map[string]any{
		"broker":            a.mqttConfig.Broker,
		"client_id":         a.mqttConfig.ClientID,
		"connected":         connected,
		"qos":               a.mqttConfig.QoS,
		"topics":            topics,
		"topic_count":       len(topics),
		"subscribed_topics": subscribedTopics,
		"unmatched_topics":  unmatchedTopics,
		"pub_topics":        pubStats,
		"max_pub_topics":    a.mqttConfig.MaxPubTopics,
	}
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
