package system

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ThingsBoard Gateway MQTT API 토픽 상수 (v1/gateway/*).
const (
	// topicGatewayConnect 는 하위 디바이스를 게이트웨이에 연결(및 서버 측 자동 생성)하는 토픽이다.
	topicGatewayConnect = "v1/gateway/connect"
	// topicGatewayDisconnect 는 하위 디바이스 연결을 해제하는 토픽이다.
	topicGatewayDisconnect = "v1/gateway/disconnect"
	// topicGatewayTelemetry 는 텔레메트리 업링크 토픽이다 (M3).
	topicGatewayTelemetry = "v1/gateway/telemetry"
	// topicGatewayAttributes 는 클라이언트 속성 업링크 / 공유 속성 다운링크 토픽이다.
	topicGatewayAttributes = "v1/gateway/attributes"
	// topicGatewayRPC 는 RPC 다운링크 및 RPC 응답 업링크 토픽이다 (M4).
	topicGatewayRPC = "v1/gateway/rpc"
)

// ThingplusConfig 는 thingplus-gateway 에이전트 설정이다.
type ThingplusConfig struct {
	// Broker 는 MQTT 브로커 주소이다 (예: "localhost" 또는 "tcp://localhost:1883").
	Broker string `json:"broker"`

	// Port 는 브로커 포트이다 (기본 1883, TLS 시 8883).
	Port int `json:"port"`

	// TLS 는 TLS 사용 여부이다.
	TLS bool `json:"tls"`

	// CACert 는 TLS 검증에 사용할 CA 인증서 경로 또는 PEM 문자열이다.
	CACert string `json:"ca_cert"`

	// AccessToken 은 게이트웨이 access token이다. MQTT username으로 전달되며 민감 정보이다.
	// 평문으로 로그/State에 노출하지 않는다 (REQ-core-nocred-log).
	AccessToken string `json:"access_token"`

	// DeviceNamePath 는 인입 메시지에서 디바이스 NAME을 추출하는 JSONPath이다 (기본 "$.device").
	DeviceNamePath string `json:"device_name_path"`

	// QoS 는 메시지 전달 보증 레벨이다 (0, 1, 2).
	QoS byte `json:"qos"`

	// KeepAliveSec 는 연결 유지 간격(초)이다.
	KeepAliveSec int `json:"keep_alive_sec"`

	// AutoReconnect 는 자동 재연결 활성화 여부이다.
	AutoReconnect bool `json:"auto_reconnect"`

	// BufferSize 는 다운링크 수신 버퍼 크기이다.
	BufferSize int `json:"buffer_size"`

	// ConnectTimeoutSec 는 연결 타임아웃(초)이다.
	ConnectTimeoutSec int `json:"connect_timeout_sec"`
}

// mqttPublisher 는 디바이스 상태 머신이 발행에 필요로 하는 최소 인터페이스이다.
// 실제 *mqtt.Client가 이를 만족하며, 테스트에서는 fake 발행자를 주입한다.
type mqttPublisher interface {
	Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token
	IsConnected() bool
}

// deviceState 는 게이트웨이 하위 디바이스의 연결 상태를 나타낸다.
type deviceState int

const (
	// deviceDisconnected 는 디바이스가 게이트웨이에 연결되지 않은 상태이다.
	deviceDisconnected deviceState = iota
	// deviceConnecting 는 connect 발행 후 PUBACK 미수신 상태이다 (A7 게이팅 대상).
	deviceConnecting
	// deviceConnected 는 connect PUBACK 수신이 완료된 상태이다.
	deviceConnected
)

// deviceEntry 는 개별 디바이스의 상태 및 메타데이터이다.
type deviceEntry struct {
	name        string
	state       deviceState
	deviceID    string
	connectedAt time.Time
}

// deviceStateMap 는 NAME → 디바이스 상태 맵을 스레드 세이프하게 관리한다.
type deviceStateMap struct {
	mu      sync.RWMutex
	devices map[string]*deviceEntry
}

// newDeviceStateMap 는 빈 deviceStateMap를 생성한다.
func newDeviceStateMap() *deviceStateMap {
	return &deviceStateMap{
		devices: make(map[string]*deviceEntry),
	}
}

// ensure 는 NAME에 해당하는 항목을 반환하며, 없으면 disconnected 상태로 생성한다.
func (m *deviceStateMap) ensure(name string) *deviceEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.devices[name]
	if !ok {
		e = &deviceEntry{name: name, state: deviceDisconnected}
		m.devices[name] = e
	}
	return e
}

// get 은 NAME에 해당하는 항목의 복사본을 반환한다.
func (m *deviceStateMap) get(name string) (deviceEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.devices[name]
	if !ok {
		return deviceEntry{}, false
	}
	return *e, true
}

// setState 는 NAME의 상태를 전이한다. 항목이 없으면 생성한다.
func (m *deviceStateMap) setState(name string, state deviceState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.devices[name]
	if !ok {
		e = &deviceEntry{name: name}
		m.devices[name] = e
	}
	e.state = state
	if state == deviceConnected {
		e.connectedAt = time.Now()
	}
}

// setDeviceID 는 NAME 항목에 device_id를 기록한다.
func (m *deviceStateMap) setDeviceID(name, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.devices[name]; ok {
		e.deviceID = id
	}
}

// remove 는 NAME 항목을 제거한다.
func (m *deviceStateMap) remove(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.devices, name)
}

// count 는 알려진 디바이스 수를 반환한다.
func (m *deviceStateMap) count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.devices)
}

// connectedNames 는 connected 상태인 디바이스 NAME 목록을 반환한다.
func (m *deviceStateMap) connectedNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.devices))
	for name, e := range m.devices {
		if e.state == deviceConnected {
			names = append(names, name)
		}
	}
	return names
}

// snapshot 은 모든 디바이스 항목의 읽기 전용 복사본을 반환한다.
func (m *deviceStateMap) snapshot() []deviceEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]deviceEntry, 0, len(m.devices))
	for _, e := range m.devices {
		out = append(out, *e)
	}
	return out
}

// ThingplusGatewayAgent 는 ThingsBoard Gateway MQTT API를 프록시하는 시스템 에이전트이다.
// 단일 MQTT 연결을 다수의 논리 디바이스에 프록시하며, 업링크(텔레메트리/속성)와
// 다운링크(RPC/공유 속성)를 양방향으로 중계하고 NAME↔device_id 매핑을 관리한다.
//
// mqtt_agent.go의 MQTTAgent를 참조 모델로 하여 *lifecycle.BaseLifecycle을 임베딩하고
// Agent 인터페이스를 직접 구현한다.
type ThingplusGatewayAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig agent.AgentConfig
	cfg         ThingplusConfig
	client      mqtt.Client
	devices     *deviceStateMap // NAME → 디바이스 상태 (disconnected/connecting/connected)
	mapping     *nameIDMap      // NAME ↔ device_id 양방향 매핑
	recvCh      chan []byte     // 다운링크 수신 버퍼 (mqtt_agent recvCh 패턴)
	done        chan struct{}
	doneOnce    sync.Once
	stats       *agent.AgentStats
	logger      *slog.Logger
	mu          sync.RWMutex
	startedAt   time.Time
	createdAt   time.Time

	// 업링크/다운링크 관찰 카운터 (State() 노출용)
	uplinkCount   atomic.Int64
	downlinkCount atomic.Int64

	// upBuf 는 브로커 연결 끊김 시 업링크(텔레메트리/속성)를 무손실로 보관하는
	// 경계가 있는 버퍼이다 (REQ-up-nolost / AC-05). 재연결 시 onConnect 에서 flush 한다.
	upBuf        *boundedUplinkBuffer
	upBufDropped atomic.Int64 // 버퍼 초과로 관찰된 드롭 수 (silent drop 금지, 메트릭 노출)

	// pendingRPC 는 다운링크 RPC (device,id) → device NAME 상관 맵이다 (응답 발행 시 사용).
	pendingRPC *pendingRPCMap
	// pendingRPCEvicted 는 상한 초과로 evict 된 상관 항목 수이다(관찰 가능한 드롭, State 노출).
	pendingRPCEvicted atomic.Int64
}

// bufferedUplink 는 재연결 시 재발행할 업링크 항목이다.
type bufferedUplink struct {
	topic   string
	qos     byte
	payload []byte
}

// boundedUplinkBuffer 는 경계가 있는 업링크 버퍼이다.
// 용량 초과 시 조용히 폐기하지 않고 (add 가 false 반환) 호출자가 관찰 가능하게 처리한다.
type boundedUplinkBuffer struct {
	mu    sync.Mutex
	items []bufferedUplink
	cap   int
}

// newBoundedUplinkBuffer 는 지정 용량의 업링크 버퍼를 생성한다. cap<1 이면 1 로 보정한다.
func newBoundedUplinkBuffer(capacity int) *boundedUplinkBuffer {
	if capacity < 1 {
		capacity = 1
	}
	return &boundedUplinkBuffer{cap: capacity}
}

// add 는 항목을 버퍼에 추가한다. 용량이 가득 차면 false 를 반환한다 (드롭 금지 신호).
func (b *boundedUplinkBuffer) add(item bufferedUplink) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.items) >= b.cap {
		return false
	}
	b.items = append(b.items, item)
	return true
}

// drain 은 버퍼의 모든 항목을 반환하고 버퍼를 비운다 (flush 용).
func (b *boundedUplinkBuffer) drain() []bufferedUplink {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.items
	b.items = nil
	return out
}

// len 은 현재 버퍼에 대기 중인 항목 수를 반환한다.
func (b *boundedUplinkBuffer) len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.items)
}

// pendingRPCDefaultCap 은 pendingRPC 맵의 기본 상한이다. cfg.BufferSize 가 없거나
// 유효하지 않을 때 사용한다. 상한 초과 시 가장 오래된 항목을 제거(evict)한다.
const pendingRPCDefaultCap = 1024

// pendingRPCKey 는 (device NAME, RPC id) 복합 키이다.
//
// id 만으로 키잉하면 서로 다른 디바이스가 동일한 RPC id 를 사용할 때 충돌하여
// 응답이 오배송될 수 있으므로(REQ-dn-name-resolve), device 를 포함한 복합 키를 사용한다.
type pendingRPCKey struct {
	device string
	id     int
}

// pendingRPCEntry 는 상관 항목이다. seq 는 삽입 순서로, 상한 초과 시
// 가장 오래된(가장 작은 seq) 항목을 evict 하는 데 사용한다.
type pendingRPCEntry struct {
	name string
	seq  uint64
}

// pendingRPCMap 은 다운링크 RPC (device,id) → device NAME 상관 맵이다 (응답 발행 시 사용).
//
// 상한(cap)을 두어 무한 증가를 방지한다. 플로우가 응답하지 않아 항목이 회수되지 않아도
// 상한 초과 시 가장 오래된 항목이 evict 되어 메모리 누수를 방지한다.
type pendingRPCMap struct {
	mu      sync.Mutex
	byKey   map[pendingRPCKey]pendingRPCEntry
	cap     int
	nextSeq uint64
}

// newPendingRPCMap 은 지정 상한의 pendingRPCMap 을 생성한다. capacity<1 이면
// pendingRPCDefaultCap 으로 보정한다.
func newPendingRPCMap(capacity int) *pendingRPCMap {
	if capacity < 1 {
		capacity = pendingRPCDefaultCap
	}
	return &pendingRPCMap{
		byKey: make(map[pendingRPCKey]pendingRPCEntry),
		cap:   capacity,
	}
}

// put 은 (device, id) → NAME 상관을 기록한다. 상한 초과 시 가장 오래된 항목을 evict 하고
// evict 된 NAME 과 true 를 반환한다(관찰 가능한 드롭 신호). evict 가 없으면 ("", false).
func (m *pendingRPCMap) put(device string, id int, name string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := pendingRPCKey{device: device, id: id}
	var evictedName string
	var evicted bool

	// 새 키이고 상한에 도달했으면 가장 오래된 항목을 evict 한다.
	if _, exists := m.byKey[key]; !exists && len(m.byKey) >= m.cap {
		var oldestKey pendingRPCKey
		var oldestSeq uint64 = ^uint64(0)
		found := false
		for k, e := range m.byKey {
			if e.seq < oldestSeq {
				oldestSeq = e.seq
				oldestKey = k
				found = true
			}
		}
		if found {
			evictedName = m.byKey[oldestKey].name
			evicted = true
			delete(m.byKey, oldestKey)
		}
	}

	m.byKey[key] = pendingRPCEntry{name: name, seq: m.nextSeq}
	m.nextSeq++
	return evictedName, evicted
}

// take 는 (device, id) 에 해당하는 NAME 을 반환하고 상관을 제거한다.
func (m *pendingRPCMap) take(device string, id int) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := pendingRPCKey{device: device, id: id}
	e, ok := m.byKey[key]
	if ok {
		delete(m.byKey, key)
	}
	return e.name, ok
}

// takeByID 는 device 정보 없이 id 만으로 상관을 조회한다(payload 에 device 가 없을 때 fallback).
// 동일 id 가 여러 디바이스에 존재하면 안전하게 라우팅할 수 없으므로 ("", false) 를 반환한다.
// 정확히 하나만 존재하면 해당 NAME 을 반환하고 제거한다.
func (m *pendingRPCMap) takeByID(id int) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var matchKey pendingRPCKey
	var matchName string
	count := 0
	for k, e := range m.byKey {
		if k.id == id {
			matchKey = k
			matchName = e.name
			count++
			if count > 1 {
				break
			}
		}
	}
	if count != 1 {
		return "", false
	}
	delete(m.byKey, matchKey)
	return matchName, true
}

// len 은 현재 상관 항목 수를 반환한다.
func (m *pendingRPCMap) len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.byKey)
}

// 컴파일 타임 인터페이스 체크
var (
	_ agent.Agent                   = (*ThingplusGatewayAgent)(nil)
	_ agent.MessageReceiver         = (*ThingplusGatewayAgent)(nil)
	_ agent.MessagePublisher        = (*ThingplusGatewayAgent)(nil)
	_ agent.StatefulAgent           = (*ThingplusGatewayAgent)(nil)
	_ agent.SubscriberAgent         = (*ThingplusGatewayAgent)(nil)
	_ agent.BufferInfoProvider      = (*ThingplusGatewayAgent)(nil)
	_ agent.ConnectionStatsProvider = (*ThingplusGatewayAgent)(nil)
)

// parseThingplusConfig 는 AgentConfig에서 ThingplusConfig를 파싱한다.
// 기본값: Port=1883(TLS 시 8883), DeviceNamePath="$.device", QoS=1,
// KeepAliveSec=60, AutoReconnect=true, BufferSize=256.
func parseThingplusConfig(cfg agent.AgentConfig) ThingplusConfig {
	tc := ThingplusConfig{
		Broker:            "localhost",
		Port:              1883,
		DeviceNamePath:    "$.device",
		QoS:               1,
		KeepAliveSec:      60,
		AutoReconnect:     true,
		BufferSize:        256,
		ConnectTimeoutSec: 10,
	}

	opts := cfg.Transport.Options
	if opts == nil {
		return tc
	}

	if v, ok := opts["broker"].(string); ok && v != "" {
		tc.Broker = v
	}
	// TLS 를 먼저 파싱하여 포트 기본값 결정에 반영한다.
	if v, ok := opts["tls"].(bool); ok {
		tc.TLS = v
	}
	// TLS 활성 시 포트 기본값을 8883으로 조정한다 (명시적 port가 없을 때).
	if tc.TLS {
		tc.Port = 8883
	}
	if v, ok := opts["port"]; ok {
		if n := toInt(v); n > 0 {
			tc.Port = n
		}
	}
	if v, ok := opts["ca_cert"].(string); ok {
		tc.CACert = v
	}
	if v, ok := opts["access_token"].(string); ok {
		tc.AccessToken = v
	}
	if v, ok := opts["device_name_path"].(string); ok && v != "" {
		tc.DeviceNamePath = v
	}
	if v, ok := opts["qos"]; ok {
		tc.QoS = byte(toInt(v))
	}
	if v, ok := opts["keep_alive_sec"]; ok {
		tc.KeepAliveSec = toInt(v)
	}
	if v, ok := opts["auto_reconnect"].(bool); ok {
		tc.AutoReconnect = v
	}
	if v, ok := opts["buffer_size"]; ok {
		if n := toInt(v); n > 0 {
			tc.BufferSize = n
		}
	}
	if v, ok := opts["connect_timeout_sec"]; ok {
		if n := toInt(v); n > 0 {
			tc.ConnectTimeoutSec = n
		}
	}

	return tc
}

// thingplusBrokerURL 은 설정으로부터 Paho 브로커 URL을 구성한다.
// broker에 이미 스킴("://")이 포함되어 있으면 그대로 사용하고,
// 아니면 TLS 여부에 따라 "ssl://" 또는 "tcp://" 스킴과 포트를 결합한다.
func thingplusBrokerURL(cfg ThingplusConfig) string {
	if strings.Contains(cfg.Broker, "://") {
		return cfg.Broker
	}
	scheme := "tcp"
	if cfg.TLS {
		scheme = "ssl"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, cfg.Broker, cfg.Port)
}

// maskSecret 은 민감한 자격 증명을 로그/State 노출용으로 마스킹한다.
// 원본 평문을 절대 포함하지 않으며, 값이 있으면 길이만 표기한다.
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	return fmt.Sprintf("****(len=%d)", len(s))
}

// NewThingplusGatewayAgent 는 ThingplusGatewayAgent 팩토리 함수이다.
func NewThingplusGatewayAgent(config agent.AgentConfig) (agent.Agent, error) {
	tc := parseThingplusConfig(config)

	a := &ThingplusGatewayAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("thingplus-gateway")),
		cfg:           tc,
		devices:       newDeviceStateMap(),
		mapping:       newNameIDMap(),
		recvCh:        make(chan []byte, tc.BufferSize),
		done:          make(chan struct{}),
		stats:         agent.NewAgentStats(),
		logger:        agent.ResolveLogger(config),
		createdAt:     time.Now(),
		upBuf:         newBoundedUplinkBuffer(tc.BufferSize),
		pendingRPC:    newPendingRPCMap(tc.BufferSize),
	}

	if err := a.Init(config); err != nil {
		return nil, err
	}

	return a, nil
}

// Init 은 에이전트를 초기화하고 게이트웨이 브로커에 연결한다.
func (a *ThingplusGatewayAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("thingplus init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("thingplus init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	// TLS 설정 구성 (활성 시 CA 로드)
	tlsConfig, err := a.buildTLSConfig()
	if err != nil {
		_ = a.TransitionTo(lifecycle.StateError)
		return fmt.Errorf("thingplus init: tls 구성 실패: %w", err)
	}

	brokerURL := thingplusBrokerURL(a.cfg)

	// MQTT 클라이언트 옵션 구성.
	// access token은 MQTT username으로 전달하고 password는 비운다 (A10).
	opts := mqtt.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID("xflow-thingplus-" + config.ID).
		SetUsername(a.cfg.AccessToken).
		SetPassword("").
		SetKeepAlive(time.Duration(a.cfg.KeepAliveSec) * time.Second).
		SetAutoReconnect(a.cfg.AutoReconnect).
		SetConnectTimeout(time.Duration(a.cfg.ConnectTimeoutSec) * time.Second).
		SetOrderMatters(false)

	if tlsConfig != nil {
		opts.SetTLSConfig(tlsConfig)
	}

	// AutoReconnect 시 초기 연결도 백그라운드에서 재시도한다 (mqtt_agent 패턴).
	// 죽은 브로커가 플로우 시작을 막지 않도록 한다.
	if a.cfg.AutoReconnect {
		opts.SetConnectRetry(true)
		opts.SetConnectRetryInterval(time.Duration(a.cfg.ConnectTimeoutSec) * time.Second)
	}

	opts.SetOnConnectHandler(a.onConnect)
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		a.logger.Warn("thingplus: 연결 끊김",
			"broker", brokerURL,
			"error", err,
		)
	})
	opts.SetDefaultPublishHandler(a.downlinkHandler)

	a.client = mqtt.NewClient(opts)
	token := a.client.Connect()
	connected := token.WaitTimeout(time.Duration(a.cfg.ConnectTimeoutSec)*time.Second) && token.Error() == nil
	if !connected {
		if a.cfg.AutoReconnect {
			// 초기 연결 실패를 치명적으로 보지 않고 Running(degraded)으로 진입한다.
			a.logger.Warn("thingplus: 초기 연결 실패 — 백그라운드 재연결로 진행(degraded)",
				"broker", brokerURL,
				"access_token", maskSecret(a.cfg.AccessToken),
				"error", token.Error(),
			)
		} else {
			_ = a.TransitionTo(lifecycle.StateError)
			if token.Error() != nil {
				return fmt.Errorf("thingplus init: 연결 실패: %w", token.Error())
			}
			return fmt.Errorf("thingplus init: 연결 타임아웃 (%s)", brokerURL)
		}
	}

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("thingplus init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	return nil
}

// buildTLSConfig 는 CACert로부터 *tls.Config를 구성한다. TLS 미사용 시 nil을 반환한다.
// CACert는 파일 경로 또는 PEM 문자열을 모두 허용한다.
func (a *ThingplusGatewayAgent) buildTLSConfig() (*tls.Config, error) {
	if !a.cfg.TLS {
		return nil, nil
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if a.cfg.CACert == "" {
		return tlsConfig, nil
	}

	pemData := []byte(a.cfg.CACert)
	// PEM 헤더가 없으면 파일 경로로 간주하여 로드한다.
	if !strings.Contains(a.cfg.CACert, "-----BEGIN") {
		data, err := os.ReadFile(a.cfg.CACert)
		if err != nil {
			return nil, fmt.Errorf("ca_cert 읽기 실패: %w", err)
		}
		pemData = data
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, fmt.Errorf("ca_cert PEM 파싱 실패")
	}
	tlsConfig.RootCAs = pool
	return tlsConfig, nil
}

// onConnect 는 브로커 연결(및 재연결) 성공 시 호출된다.
// 다운링크 토픽을 구독하고, 이전에 연결되어 있던 알려진 디바이스들을 재connect한다.
func (a *ThingplusGatewayAgent) onConnect(c mqtt.Client) {
	// 콜백 goroutine 에서 실행되므로 런타임 Configure() 와의 레이스를 방지하기 위해 스냅샷한다.
	cfg := a.snapshotConfig()
	a.logger.Info("thingplus: 게이트웨이 브로커에 연결됨",
		"broker", thingplusBrokerURL(cfg),
		"access_token", maskSecret(cfg.AccessToken),
	)

	// 다운링크 토픽 구독 (RPC, 공유 속성).
	a.subscribeDownlink(c)

	// 재연결 시 이전에 connected 였던 디바이스들을 다시 connect한다 (REQ-map-reconnect).
	a.reconnectKnownDevices(c)

	// 재연결 후 버퍼링된 업링크를 flush 한다 (REQ-up-nolost / AC-05).
	a.flushUplinkBuffer(c)
}

// subscribeDownlink 는 게이트웨이 다운링크 토픽(RPC, 공유 속성)을 구독한다.
// 현재 핸들러는 수신 로깅 및 카운팅만 수행하며, 전체 파싱/방출은 M4에서 구현한다.
func (a *ThingplusGatewayAgent) subscribeDownlink(c mqtt.Client) {
	qos := a.snapshotConfig().QoS
	topics := []string{topicGatewayRPC, topicGatewayAttributes}
	for _, topic := range topics {
		token := c.Subscribe(topic, qos, a.downlinkHandler)
		token.Wait()
		if token.Error() != nil {
			a.logger.Error("thingplus: 다운링크 토픽 구독 실패",
				"topic", topic,
				"error", token.Error(),
			)
			continue
		}
		a.logger.Info("thingplus: 다운링크 토픽 구독 완료",
			"topic", topic,
			"qos", qos,
		)
	}
}

// downlinkHandler 는 다운링크 메시지 수신 콜백이다 (M4).
// 토픽별로 라우팅하여 RPC 요청과 공유 속성 변경을 Type() 부여 플로우 메시지로 방출한다.
//   - topicGatewayRPC        → parseRPCRequest → Type "thingplus.rpc.request" 방출
//   - topicGatewayAttributes → parseSharedAttributes → Type "thingplus.attr.update" 방출
//
// 인식되지 않는 토픽은 원시 페이로드 fallback 으로 recvCh 에 그대로 전달한다.
func (a *ThingplusGatewayAgent) downlinkHandler(_ mqtt.Client, msg mqtt.Message) {
	data := make([]byte, len(msg.Payload()))
	copy(data, msg.Payload())
	a.routeDownlink(msg.Topic(), data)
}

// routeDownlink 는 토픽/페이로드로부터 플로우 메시지 바이트를 조립하여 recvCh 에 방출한다.
// downlinkHandler 와 통합 테스트가 공통으로 사용하는 순수(브로커 비의존) 라우팅 경로이다.
func (a *ThingplusGatewayAgent) routeDownlink(topic string, data []byte) {
	var out []byte

	switch topic {
	case topicGatewayRPC:
		if built, err := a.buildRPCDownlinkMessage(data); err == nil {
			out = built
		} else {
			a.logger.Warn("thingplus: RPC 다운링크 파싱 실패, 원시 fallback", "error", err)
		}
	case topicGatewayAttributes:
		if built, err := a.buildSharedAttrDownlinkMessage(data); err == nil {
			out = built
		} else {
			a.logger.Warn("thingplus: 공유 속성 다운링크 파싱 실패, 원시 fallback", "error", err)
		}
	}

	// 인식 실패 또는 미지원 토픽 → 원시 페이로드 fallback (동작 지속).
	if out == nil {
		out = data
	}

	a.emitDownlink(topic, out)
}

// emitDownlink 는 방출용 바이트를 recvCh 에 넣고 다운링크 통계를 갱신한다.
// 버퍼가 가득 차면 관찰 가능한 방식(에러 카운트 + 경고)으로 처리한다.
func (a *ThingplusGatewayAgent) emitDownlink(topic string, out []byte) {
	select {
	case a.recvCh <- out:
		a.downlinkCount.Add(1)
		a.stats.IncrExternalMessagesReceived()
		a.stats.AddBytesRead(int64(len(out)))
		a.stats.UpdateLastActivity()
	default:
		a.stats.IncrExternalMessagesErrored()
		a.logger.Warn("thingplus: 다운링크 버퍼 가득 참, 메시지 드롭",
			"topic", topic,
		)
	}
}

// buildRPCDownlinkMessage 는 RPC 다운링크 페이로드를 파싱하여 Type "thingplus.rpc.request"
// 플로우 메시지 JSON 을 조립한다 (REQ-dn-rpc-recv / REQ-dn-name-resolve / AC-02).
//
// NAME 을 device_id 로 역해석하여 방출 페이로드에 device/device_id/id/method/params 를 담고,
// 응답 상관을 위해 pendingRPC 에 (id → NAME) 을 기록한다.
func (a *ThingplusGatewayAgent) buildRPCDownlinkMessage(data []byte) ([]byte, error) {
	req, err := parseRPCRequest(data)
	if err != nil {
		return nil, err
	}

	deviceID := a.mapping.resolveDeviceID(context.Background(), a.Name(), req.Device)
	a.devices.setDeviceID(req.Device, deviceID)

	// 응답 발행 시 NAME 을 복원할 수 있도록 (device,id) 복합 키로 상관 기록.
	// 상한 초과 시 가장 오래된 항목이 evict 되며, silent drop 금지 원칙에 따라 관찰 가능하게 처리한다.
	if evictedName, evicted := a.pendingRPC.put(req.Device, req.Data.ID, req.Device); evicted {
		a.pendingRPCEvicted.Add(1)
		a.logger.Warn("thingplus: pendingRPC 상한 초과 — 가장 오래된 상관 evict",
			"evicted_device", evictedName,
			"evicted_total", a.pendingRPCEvicted.Load(),
		)
	}

	payload := map[string]any{
		"device":    req.Device,
		"device_id": deviceID,
		"id":        req.Data.ID,
		"method":    req.Data.Method,
		"params":    req.Data.Params,
	}
	return a.buildFlowMessageJSON("thingplus.rpc.request", req.Device, deviceID, payload)
}

// buildSharedAttrDownlinkMessage 는 공유 속성 다운링크 페이로드를 파싱하여
// Type "thingplus.attr.update" 플로우 메시지 JSON 을 조립한다 (REQ-dn-shared / AC-04).
func (a *ThingplusGatewayAgent) buildSharedAttrDownlinkMessage(data []byte) ([]byte, error) {
	msg, err := parseSharedAttributes(data)
	if err != nil {
		return nil, err
	}

	deviceID := a.mapping.resolveDeviceID(context.Background(), a.Name(), msg.Device)
	a.devices.setDeviceID(msg.Device, deviceID)

	payload := map[string]any{
		"device":    msg.Device,
		"device_id": deviceID,
		"data":      msg.Data,
	}
	return a.buildFlowMessageJSON("thingplus.attr.update", msg.Device, deviceID, payload)
}

// buildFlowMessageJSON 은 지정 Type 과 페이로드로 message.Message 를 조립하여
// MarshalJSON 바이트를 반환한다.
//
// DESIGN DECISION (bridge.go 근거): Bridge In 기본 경로(resolver.convertPayload)는
// recvCh 바이트를 payload 맵으로만 파싱하며 message Type 을 보존하지 않고,
// startReceiveLoop 는 Type 이 비어 있으면 "event" 로 강제한다. 따라서 Type 을 보존하려면
// 에이전트가 message.New(WithType(...)) 로 조립한 message.Message 를 MarshalJSON 하여
// recvCh 에 넣고, 소비 측이 FromJSON 으로 복원해야 한다. 이 함수가 그 방출 표현을 만든다.
func (a *ThingplusGatewayAgent) buildFlowMessageJSON(msgType, name, deviceID string, payload map[string]any) ([]byte, error) {
	m := message.New(
		message.WithType(msgType),
		message.WithPayload(message.NewPayload(payload)),
		message.WithMetadata("device", name),
		message.WithMetadata("device_id", deviceID),
	)
	return m.MarshalJSON()
}

// connectDevice 는 하위 디바이스를 게이트웨이에 연결한다 (REQ-map-connect).
//
// v1/gateway/connect에 {"device":"<NAME>"}를 발행하고, connect PUBACK 수신
// (token.Wait 완료)을 connected 전이 조건으로 사용한다 (A7 게이팅). 발행 실패 시
// 디바이스는 connecting 상태로 남으며 에러를 반환한다.
func (a *ThingplusGatewayAgent) connectDevice(pub mqttPublisher, name string) error {
	a.devices.ensure(name)
	a.devices.setState(name, deviceConnecting)

	payload, err := json.Marshal(map[string]any{"device": name})
	if err != nil {
		return fmt.Errorf("thingplus connect: 페이로드 조립 실패: %w", err)
	}

	token := pub.Publish(topicGatewayConnect, a.snapshotConfig().QoS, false, payload)
	token.Wait()
	if token.Error() != nil {
		a.stats.IncrExternalMessagesErrored()
		return fmt.Errorf("thingplus connect: %q 발행 실패: %w", name, token.Error())
	}

	// PUBACK 수신 완료 → connected 전이
	a.devices.setState(name, deviceConnected)
	a.uplinkCount.Add(1)
	a.stats.IncrExternalMessagesSent()
	a.logger.Info("thingplus: 디바이스 connect 완료",
		"device", name,
	)
	return nil
}

// disconnectDevice 는 하위 디바이스 연결을 해제한다 (REQ-map-disconnect).
// v1/gateway/disconnect에 {"device":"<NAME>"}를 발행하고 상태 맵에서 제거한다.
func (a *ThingplusGatewayAgent) disconnectDevice(pub mqttPublisher, name string) error {
	payload, err := json.Marshal(map[string]any{"device": name})
	if err != nil {
		return fmt.Errorf("thingplus disconnect: 페이로드 조립 실패: %w", err)
	}

	token := pub.Publish(topicGatewayDisconnect, a.snapshotConfig().QoS, false, payload)
	token.Wait()
	if token.Error() != nil {
		a.stats.IncrExternalMessagesErrored()
		return fmt.Errorf("thingplus disconnect: %q 발행 실패: %w", name, token.Error())
	}

	a.devices.remove(name)
	a.uplinkCount.Add(1)
	a.stats.IncrExternalMessagesSent()
	a.logger.Info("thingplus: 디바이스 disconnect 완료",
		"device", name,
	)
	return nil
}

// reconnectKnownDevices 는 브로커 재연결 시 이전에 connected 였던 모든 디바이스를
// 다시 connect한다 (REQ-map-reconnect / REQ-core-reconnect).
func (a *ThingplusGatewayAgent) reconnectKnownDevices(pub mqttPublisher) {
	names := a.devices.connectedNames()
	for _, name := range names {
		if err := a.connectDevice(pub, name); err != nil {
			a.logger.Error("thingplus: 재연결 시 디바이스 재connect 실패",
				"device", name,
				"error", err,
			)
		}
	}
}

// ReceiveMessage 는 다운링크 수신 채널에서 메시지를 가져온다.
// agent.MessageReceiver 인터페이스 구현.
func (a *ThingplusGatewayAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case data := <-a.recvCh:
		return data, nil
	case <-a.done:
		return nil, fmt.Errorf("thingplus: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// PublishMessage 는 게이트웨이 브로커에 메시지를 발행한다.
// agent.MessagePublisher 인터페이스 구현.
func (a *ThingplusGatewayAgent) PublishMessage(topic string, qos byte, retained bool, payload []byte) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("thingplus: 브로커에 연결되어 있지 않음")
	}
	if topic == "" {
		return fmt.Errorf("thingplus: 발행 토픽이 지정되지 않음")
	}

	token := client.Publish(topic, qos, retained, payload)
	token.Wait()
	if token.Error() != nil {
		a.stats.IncrExternalMessagesErrored()
		return fmt.Errorf("thingplus: 발행 실패: %w", token.Error())
	}

	a.uplinkCount.Add(1)
	a.stats.IncrExternalMessagesSent()
	return nil
}

// Subscribe 는 동적으로 토픽을 구독한다.
// agent.SubscriberAgent 인터페이스 구현.
func (a *ThingplusGatewayAgent) Subscribe(_ context.Context, topics []string) error {
	a.mu.RLock()
	client := a.client
	qos := a.cfg.QoS
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("thingplus subscribe: 브로커에 연결되지 않음")
	}
	for _, topic := range topics {
		token := client.Subscribe(topic, qos, a.downlinkHandler)
		token.Wait()
		if token.Error() != nil {
			return fmt.Errorf("thingplus subscribe: 토픽 %q 구독 실패: %w", topic, token.Error())
		}
	}
	return nil
}

// Unsubscribe 는 동적으로 토픽 구독을 해제한다.
// agent.SubscriberAgent 인터페이스 구현.
func (a *ThingplusGatewayAgent) Unsubscribe(_ context.Context, topics []string) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("thingplus unsubscribe: 브로커에 연결되지 않음")
	}
	token := client.Unsubscribe(topics...)
	token.Wait()
	if token.Error() != nil {
		return fmt.Errorf("thingplus unsubscribe: %w", token.Error())
	}
	return nil
}

// Start 는 이미 Running 상태이면 no-op이다.
// Stopped 상태이면 Created로 리셋 후 Init()을 재호출하여 재연결한다.
func (a *ThingplusGatewayAgent) Start(_ context.Context) error {
	switch a.CurrentState() {
	case lifecycle.StateRunning:
		return nil
	case lifecycle.StateStopped:
		if err := a.TransitionTo(lifecycle.StateCreated); err != nil {
			return fmt.Errorf("thingplus start: reset to created: %w", err)
		}
		a.mu.RLock()
		cfg := a.agentConfig
		a.mu.RUnlock()
		return a.Init(cfg)
	default:
		return fmt.Errorf("thingplus start: not in running state (current: %s)", a.CurrentState())
	}
}

// Stop 은 게이트웨이 연결을 종료한다.
func (a *ThingplusGatewayAgent) Stop(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("thingplus stop: %w", err)
	}

	if a.client != nil && a.client.IsConnected() {
		a.client.Disconnect(250)
	}

	// ReceiveMessage 대기자에게 종료 시그널 (중복 Stop 호출 시 panic 방지)
	a.doneOnce.Do(func() { close(a.done) })

	// 버퍼에 남은 메시지 드레인
	for {
		select {
		case <-a.recvCh:
		default:
			if err := a.TransitionTo(lifecycle.StateStopped); err != nil {
				return fmt.Errorf("thingplus stop: %w", err)
			}
			return nil
		}
	}
}

// Pause 는 Running -> Paused 전이한다.
func (a *ThingplusGatewayAgent) Pause(_ context.Context) error {
	return a.TransitionTo(lifecycle.StatePaused)
}

// Resume 은 Paused -> Running 전이한다.
func (a *ThingplusGatewayAgent) Resume(_ context.Context) error {
	return a.TransitionTo(lifecycle.StateRunning)
}

// Health 는 에이전트의 건강 상태를 반환한다.
func (a *ThingplusGatewayAgent) Health() agent.HealthStatus {
	now := time.Now()
	state := a.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		if a.client != nil && a.client.IsConnected() {
			return agent.HealthStatus{
				Status:    agent.HealthHealthy,
				LastCheck: now,
				Message:   "thingplus gateway is running and connected",
			}
		}
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "thingplus gateway is running but disconnected (reconnecting)",
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "thingplus gateway is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("thingplus gateway is in %s state", state),
		}
	}
}

// Process 는 인입 플로우 메시지(플로우 → 게이트웨이)의 단일 진입점이다 (M3/M4).
//
// DESIGN DECISION (bridge.go 근거): BridgeOut 경로에서 어댑터가 없으면 Bridge 는
// transport.Send → agent.Process(data) 로 원시 바이트를 전달한다(resolver.go Send).
// 따라서 Process 를 인입 진입점으로 사용하면 어댑터에 의존하지 않고 업링크/RPC 응답을
// 모두 처리할 수 있다. (thingplus 어댑터도 추가로 제공하나, 상태 로직은 Process 에 둔다.)
//
// 메시지 형태로 분기한다:
//   - RPC 응답(type "thingplus.rpc.response" 또는 payload 에 rpc id 존재) → buildRPCResponse
//     후 topicGatewayRPC 로 발행 (연결된 디바이스에 한함, A6/A7).
//   - 그 외 → 업링크 텔레메트리/속성으로 처리 (auto-connect 후 발행, 연결 끊김 시 버퍼링).
//
// 반환 바이트는 없다(nil). 오류는 관찰 목적으로 반환한다.
func (a *ThingplusGatewayAgent) Process(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}

	// message.Message JSON 이면 우선 복원하여 Type/metadata/payload 를 활용한다.
	// 아니면 raw JSON payload 로 간주하여 map 을 직접 파싱한다(메타데이터 없음).
	msgType, msg, payload, err := decodeInbound(data)
	if err != nil {
		return nil, fmt.Errorf("thingplus process: 인입 메시지 파싱 실패: %w", err)
	}

	// RPC 응답 분기: type 이 rpc.response 이거나 payload 에 rpc id 가 있으면 응답으로 처리.
	// RPC 응답은 payload["device"] 기반이므로 metadata 를 사용하지 않는다.
	if msgType == "thingplus.rpc.response" || looksLikeRPCReply(payload) {
		return nil, a.handleRPCReply(a.uplinkPublisher(), payload)
	}

	// 그 외는 업링크(텔레메트리/속성)로 처리.
	// 발행자는 실제 클라이언트(nil 가능)를 주입한다. nil 이면 미연결로 간주하여 버퍼링한다.
	// msg 를 전달하여 메타데이터 스코프 device_name_path 추출을 지원한다(msg==nil 이면 payload 스코프만).
	return nil, a.handleUplinkFromMessage(a.uplinkPublisher(), msg, payload)
}

// uplinkPublisher 는 업링크 발행에 사용할 mqttPublisher 를 반환한다.
// 실제 *mqtt.Client 를 반환하며, client 가 nil 이면 nil 을 반환한다(미연결로 처리).
func (a *ThingplusGatewayAgent) uplinkPublisher() mqttPublisher {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()
	if client == nil {
		return nil
	}
	return client
}

// snapshotConfig 는 런타임 Configure() 와의 데이터 레이스를 방지하기 위해
// 현재 cfg 의 값 복사본을 짧은 RLock 하에 반환한다 (mqtt_agent.go 의 agentConfig
// 스냅샷 패턴 참조). ThingplusConfig 는 포인터/맵/슬라이스가 없는 평탄한 값 구조체이므로
// 얕은 복사가 곧 완전한 안전 복사이다.
//
// 주의(재귀 RLock deadlock 트랩): 이 함수는 a.mu 를 즉시 해제하므로, 호출자는 반환된
// 로컬 복사본만 사용하며 발행/토큰 대기 등 블로킹 연산 동안 a.mu 를 보유하지 않는다.
func (a *ThingplusGatewayAgent) snapshotConfig() ThingplusConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

// decodeInbound 는 인입 바이트를 (msgType, msg, payload map) 으로 복원한다.
//
// 우선 message.Message JSON(FromJSON) 을 시도하여 성공하면 top-level Type, 복원된 메시지(msg),
// 그리고 payload 맵을 반환한다. 복원된 msg 는 메타데이터(그룹 포함)를 보존하므로
// 메타데이터 스코프 device_name_path 추출에 사용된다.
//
// FromJSON 이 실패하면 raw JSON 객체로 간주하여 payload 맵으로 사용하며, 이 경우 msg 는 nil 이다
// (메타데이터 없음 → 메타데이터 스코프 경로는 해석 불가).
func decodeInbound(data []byte) (string, message.Message, map[string]any, error) {
	if m, err := message.FromJSON(data); err == nil {
		return m.Type(), m, m.Payload().ToMap(), nil
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", nil, nil, err
	}
	return "", nil, raw, nil
}

// looksLikeRPCReply 는 payload 가 RPC 응답 형태(rpc id 를 보유)인지 판별한다.
// {"id":<n>,...} 또는 {"rpc_id":<n>,...} 형태를 응답으로 본다.
func looksLikeRPCReply(payload map[string]any) bool {
	if _, ok := payload["rpc_id"]; ok {
		return true
	}
	// "id" 키가 있고 "method"/"temperature" 등 업링크성 키가 없으면 응답으로 간주.
	if _, ok := payload["id"]; ok {
		if _, hasData := payload["data"]; hasData {
			return true
		}
	}
	return false
}

// handleRPCReply 는 플로우로부터 온 RPC 응답을 게이트웨이 RPC 토픽으로 발행한다
// (REQ-dn-rpc-reply / AC-02). 디바이스가 connected 상태일 때만 발행한다 (A6/A7).
//
// pub 을 주입받아 테스트에서 fakePublisher 로 검증 가능하다. pub 이 nil 또는 미연결이면
// 발행할 수 없으므로 에러를 반환한다.
func (a *ThingplusGatewayAgent) handleRPCReply(pub mqttPublisher, payload map[string]any) error {
	id, ok := toIntKey(payload, "id", "rpc_id")
	if !ok {
		return fmt.Errorf("thingplus rpc reply: rpc id 를 찾을 수 없음")
	}

	// device NAME 해석: payload.device → pendingRPC 상관 → 실패 시 에러.
	name := stringKey(payload, "device")
	if name == "" {
		// device 미지정 시 id 만으로 상관 조회(fallback). 동일 id 가 여러 디바이스에
		// 존재하면 안전하게 라우팅할 수 없으므로 실패한다(오배송 방지).
		if n, ok := a.pendingRPC.takeByID(id); ok {
			name = n
		}
	} else {
		a.pendingRPC.take(name, id) // (device,id) 상관 정리(있으면).
	}
	if name == "" {
		return fmt.Errorf("thingplus rpc reply: device NAME 을 확인할 수 없음 (id=%d)", id)
	}

	// A6/A7: connected 상태일 때만 발행 게이팅.
	if e, ok := a.devices.get(name); !ok || e.state != deviceConnected {
		return fmt.Errorf("thingplus rpc reply: 디바이스 %q 가 connected 상태가 아님 (게이팅)", name)
	}

	if pub == nil || !pub.IsConnected() {
		return fmt.Errorf("thingplus rpc reply: 브로커에 연결되어 있지 않음")
	}

	// 응답 data 추출 (payload.data 우선, 없으면 id/device/rpc_id 를 제외한 나머지).
	respData := extractRPCReplyData(payload)

	out, err := buildRPCResponse(name, id, respData)
	if err != nil {
		return err
	}

	token := pub.Publish(topicGatewayRPC, a.snapshotConfig().QoS, false, out)
	token.Wait()
	if token.Error() != nil {
		a.stats.IncrExternalMessagesErrored()
		return fmt.Errorf("thingplus rpc reply: 발행 실패: %w", token.Error())
	}
	a.uplinkCount.Add(1)
	a.stats.IncrExternalMessagesSent()
	return nil
}

// extractRPCReplyData 는 RPC 응답 페이로드에서 발행할 data 맵을 추출한다.
// payload.data 가 맵이면 그대로 사용하고, 없으면 상관/식별 키를 제외한 나머지를 사용한다.
func extractRPCReplyData(payload map[string]any) map[string]any {
	if d, ok := payload["data"].(map[string]any); ok {
		return d
	}
	data := make(map[string]any, len(payload))
	for k, v := range payload {
		switch k {
		case "id", "rpc_id", "device", "device_id":
			continue
		default:
			data[k] = v
		}
	}
	return data
}

// handleUplink 는 인입 텔레메트리/속성을 게이트웨이로 발행한다 (REQ-up-*).
//
// 흐름: device NAME 추출 → device_id 해석/기록 → 미연결 시 auto-connect(연결 시) →
// 텔레메트리(+클라이언트 속성) 조립 → 발행. 브로커 연결이 끊겨 있으면 upBuf 에 버퍼링한다.
//
// pub 이 nil 이거나 IsConnected()==false 이면 미연결로 간주하여 무손실 버퍼링한다.
// 발행자를 주입받으므로 테스트에서 fakePublisher 로 검증 가능하다.
//
// 이 오버로드는 메타데이터 없이(payload 스코프 device_name_path 만) 처리한다.
// message.Message 로 인입되어 메타데이터 스코프 경로를 지원하려면
// handleUplinkFromMessage 를 사용한다(Process 경로).
func (a *ThingplusGatewayAgent) handleUplink(pub mqttPublisher, payload map[string]any) error {
	return a.handleUplinkFromMessage(pub, nil, payload)
}

// handleUplinkFromMessage 는 handleUplink 의 메타데이터 인지(metadata-aware) 코어이다.
//
// msg 가 non-nil 이면 메타데이터 스코프 device_name_path
// ("$.metadata.device.name", "$.metadata.device_id" 등)를 지원한다.
// msg 가 nil 이면(raw JSON 인입) payload 스코프 경로("$.device", "$.payload.dev" 등)만
// 해석 가능하며 메타데이터 스코프 경로는 명확한 에러를 반환한다.
func (a *ThingplusGatewayAgent) handleUplinkFromMessage(pub mqttPublisher, msg message.Message, payload map[string]any) error {
	// 런타임 Configure() 와의 레이스 방지: 필요한 설정 필드를 함수 진입 시 한 번 스냅샷한다.
	cfg := a.snapshotConfig()

	// device NAME 추출 (cfg.DeviceNamePath, 기본 "$.device").
	// 경로 스코프(metadata/payload/bare)에 따라 메시지 또는 payload 에서 해석한다.
	name, err := resolveDeviceNameFromMessage(msg, payload, cfg.DeviceNamePath)
	if err != nil {
		return err
	}

	// device_id 해석 및 기록 (REQ-map-resolve / REQ-map-fallback).
	deviceID := a.mapping.resolveDeviceID(context.Background(), a.Name(), name)
	a.devices.ensure(name)
	a.devices.setDeviceID(name, deviceID)

	// 텔레메트리 values 및 클라이언트 속성 분리.
	values, attrs := splitUplinkPayload(payload, cfg.DeviceNamePath)

	connected := pub != nil && pub.IsConnected()

	// A6/A7: 미연결 디바이스는 텔레메트리 발행 전 auto-connect 한다(브로커 연결 시).
	if connected {
		if e, ok := a.devices.get(name); !ok || e.state != deviceConnected {
			if err := a.connectDevice(pub, name); err != nil {
				return fmt.Errorf("thingplus uplink: auto-connect 실패: %w", err)
			}
		}
	}

	// 텔레메트리 발행 (값이 있을 때).
	if len(values) > 0 {
		ts := time.Now()
		telemetry, err := buildTelemetry(name, &ts, values)
		if err != nil {
			return err
		}
		if err := a.publishOrBuffer(pub, topicGatewayTelemetry, telemetry, connected, cfg.QoS); err != nil {
			return err
		}
	}

	// 클라이언트 속성 발행 (있을 때, REQ-up-attributes).
	if len(attrs) > 0 {
		clientAttr, err := buildClientAttributes(name, attrs)
		if err != nil {
			return err
		}
		if err := a.publishOrBuffer(pub, topicGatewayAttributes, clientAttr, connected, cfg.QoS); err != nil {
			return err
		}
	}

	return nil
}

// publishOrBuffer 는 연결 상태이면 즉시 발행하고, 끊겨 있으면 upBuf 에 버퍼링한다
// (REQ-up-nolost / AC-05). 버퍼가 가득 차면 조용히 폐기하지 않고 카운터+경고로 관찰한다.
//
// qos 는 호출자가 스냅샷한 값을 전달받아 사용한다 (런타임 Configure() 레이스 방지).
func (a *ThingplusGatewayAgent) publishOrBuffer(pub mqttPublisher, topic string, payload []byte, connected bool, qos byte) error {
	if connected {
		token := pub.Publish(topic, qos, false, payload)
		token.Wait()
		if token.Error() != nil {
			a.stats.IncrExternalMessagesErrored()
			return fmt.Errorf("thingplus uplink: 발행 실패: %w", token.Error())
		}
		a.uplinkCount.Add(1)
		a.stats.IncrExternalMessagesSent()
		return nil
	}

	if a.upBuf.add(bufferedUplink{topic: topic, qos: qos, payload: payload}) {
		a.logger.Info("thingplus: 연결 끊김 — 업링크 버퍼링(무손실)",
			"topic", topic,
			"buffered", a.upBuf.len(),
		)
		return nil
	}

	// 버퍼 초과: silent drop 금지. 관찰 가능하게 처리.
	a.upBufDropped.Add(1)
	a.stats.IncrExternalMessagesErrored()
	a.logger.Warn("thingplus: 업링크 버퍼 초과 — 관찰 가능한 드롭",
		"topic", topic,
		"dropped_total", a.upBufDropped.Load(),
	)
	return fmt.Errorf("thingplus uplink: 버퍼 초과 (topic=%s)", topic)
}

// flushUplinkBuffer 는 브로커 재연결 시 버퍼에 쌓인 업링크를 재발행한다
// (REQ-up-nolost / AC-05). onConnect 에서 호출된다.
func (a *ThingplusGatewayAgent) flushUplinkBuffer(pub mqttPublisher) {
	items := a.upBuf.drain()
	for _, it := range items {
		token := pub.Publish(it.topic, it.qos, false, it.payload)
		token.Wait()
		if token.Error() != nil {
			a.stats.IncrExternalMessagesErrored()
			a.logger.Error("thingplus: 버퍼 flush 발행 실패",
				"topic", it.topic,
				"error", token.Error(),
			)
			continue
		}
		a.uplinkCount.Add(1)
		a.stats.IncrExternalMessagesSent()
	}
	if len(items) > 0 {
		a.logger.Info("thingplus: 재연결 후 업링크 버퍼 flush 완료",
			"flushed", len(items),
		)
	}
}

// splitUplinkPayload 는 인입 페이로드를 텔레메트리 values 와 클라이언트 속성으로 분리한다.
//
// 규칙:
//   - device 식별 키(cfg.DeviceNamePath 의 최상위 키, 기본 "device")는 제외.
//   - "attributes" 객체가 있으면 클라이언트 속성으로 분리.
//   - "values" 객체가 있으면 텔레메트리로 사용하고, 없으면 나머지 최상위 키를 텔레메트리로 본다.
func splitUplinkPayload(payload map[string]any, namePath string) (values map[string]any, attrs map[string]any) {
	deviceKey := topLevelKey(namePath)

	if a, ok := payload["attributes"].(map[string]any); ok {
		attrs = a
	}

	if v, ok := payload["values"].(map[string]any); ok {
		values = v
		return values, attrs
	}

	// values 명시가 없으면 식별/속성 키를 제외한 나머지를 텔레메트리로 본다.
	values = make(map[string]any)
	for k, v := range payload {
		switch k {
		case deviceKey, "attributes", "values", "device_id":
			continue
		default:
			values[k] = v
		}
	}
	return values, attrs
}

// topLevelKey 는 JSONPath("$.device" 또는 "$.metadata.device_id")에서 최상위 키를 추출한다.
// 파싱 불가 시 "device" 를 반환한다.
func topLevelKey(path string) string {
	p := strings.TrimPrefix(path, "$.")
	if p == "" {
		return "device"
	}
	if i := strings.IndexByte(p, '.'); i >= 0 {
		p = p[:i]
	}
	return p
}

// stringKey 는 payload 에서 문자열 값을 안전하게 추출한다. 없으면 "" 를 반환한다.
func stringKey(payload map[string]any, key string) string {
	if v, ok := payload[key].(string); ok {
		return v
	}
	return ""
}

// toIntKey 는 payload 에서 지정 키들 중 첫 번째로 발견되는 정수 값을 추출한다.
// JSON 숫자는 float64 로 언마샬되므로 float64 도 처리한다.
func toIntKey(payload map[string]any, keys ...string) (int, bool) {
	for _, k := range keys {
		v, ok := payload[k]
		if !ok {
			continue
		}
		switch n := v.(type) {
		case float64:
			return int(n), true
		case int:
			return n, true
		case int64:
			return int(n), true
		}
	}
	return 0, false
}

// Configure 는 에이전트 설정을 업데이트한다.
func (a *ThingplusGatewayAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("thingplus configure: %w", err)
	}
	a.mu.Lock()
	a.agentConfig = config
	a.cfg = parseThingplusConfig(config)
	a.mu.Unlock()
	return nil
}

// ID 는 에이전트 ID를 반환한다.
func (a *ThingplusGatewayAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *ThingplusGatewayAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *ThingplusGatewayAgent) Type() string {
	return "thingplus-gateway"
}

// Info 는 에이전트 정보 스냅샷을 반환한다.
func (a *ThingplusGatewayAgent) Info() agent.AgentInfo {
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
		Type:      "thingplus-gateway",
		State:     state,
		Health:    a.Health(),
		Config:    cfg,
		Stats:     a.stats.Snapshot(),
		StartedAt: startedAt,
		Uptime:    uptime,
		CreatedAt: createdAt,
	}
}

// BufferInfo 는 다운링크 수신 버퍼의 대기 개수와 용량을 반환한다.
// agent.BufferInfoProvider 인터페이스 구현.
func (a *ThingplusGatewayAgent) BufferInfo() (int, int) {
	return len(a.recvCh), cap(a.recvCh)
}

// Stats 는 통계 스냅샷을 반환한다.
func (a *ThingplusGatewayAgent) Stats() agent.StatsSnapshot {
	s := a.stats.Snapshot()
	s.MsgBufferPending, s.MsgBufferCapacity = a.BufferInfo()
	return s
}

// ConnectionStats 는 디바이스별 연결 통계를 반환한다.
// agent.ConnectionStatsProvider 인터페이스 구현. 각 알려진 디바이스를 하나의
// 연결 단위로 노출한다.
func (a *ThingplusGatewayAgent) ConnectionStats() []agent.ConnectionStats {
	entries := a.devices.snapshot()
	out := make([]agent.ConnectionStats, 0, len(entries))
	for _, e := range entries {
		out = append(out, agent.ConnectionStats{
			ID:          e.name,
			ConnectedAt: e.connectedAt,
		})
	}
	return out
}

// State 는 에이전트의 런타임 상태를 반환한다.
// agent.StatefulAgent 인터페이스 구현. access token은 마스킹하여 노출한다
// (REQ-core-nocred-log / AC-06).
func (a *ThingplusGatewayAgent) State() map[string]any {
	// 런타임 Configure() 및 재연결과의 레이스 방지: client 와 cfg 를 한 번의 RLock 하에 스냅샷한다.
	a.mu.RLock()
	client := a.client
	cfg := a.cfg
	a.mu.RUnlock()

	connected := client != nil && client.IsConnected()
	pending, capacity := a.BufferInfo()

	return map[string]any{
		"broker":                thingplusBrokerURL(cfg),
		"tls":                   cfg.TLS,
		"connected":             connected,
		"device_count":          a.devices.count(),
		"device_name_path":      cfg.DeviceNamePath,
		"qos":                   cfg.QoS,
		"uplink_count":          a.uplinkCount.Load(),
		"downlink_count":        a.downlinkCount.Load(),
		"buffer_pending":        pending,
		"buffer_capacity":       capacity,
		"uplink_buffer_len":     a.upBuf.len(),
		"uplink_buffer_dropped": a.upBufDropped.Load(),
		"pending_rpc_evicted":   a.pendingRPCEvicted.Load(),
		"access_token_masked":   maskSecret(cfg.AccessToken),
	}
}
