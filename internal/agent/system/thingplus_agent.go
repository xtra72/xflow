package system

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ThingsBoard DEVICE MQTT API 토픽 상수 (v1/devices/me/*).
//
// 이 에이전트는 단일 액세스 토큰으로 하나의 디바이스("me")를 인증하는
// Device API 를 사용한다. Gateway API (v1/gateway/*) 와 달리 하위 디바이스
// connect/disconnect 개념이 없으며, 텔레메트리는 곧바로 발행한다.
const (
	// topicDeviceTelemetry 는 텔레메트리 업링크 토픽이다 (Device API).
	// 페이로드: {"ts":<epoch_ms>,"values":{"<unit>":{<kv>}}}
	topicDeviceTelemetry = "v1/devices/me/telemetry"
	// topicDeviceAttributes 는 클라이언트 속성 업링크 및 공유 속성 다운링크 토픽이다 (Device API).
	// 업링크(클라이언트 속성) / 다운링크(공유 속성) 모두 flat {"key":value} 형식이다.
	topicDeviceAttributes = "v1/devices/me/attributes"

	// topicDeviceRPCRequestPrefix 는 서버→디바이스 RPC 요청(다운링크) 토픽 접두사이다.
	// 실제 토픽은 v1/devices/me/rpc/request/{requestId} 이며, {requestId}(정수)는
	// 페이로드가 아닌 토픽에 담긴다. 페이로드는 {"method":..,"params":..} 형식이다.
	topicDeviceRPCRequestPrefix = "v1/devices/me/rpc/request/"
	// topicDeviceRPCRequestSub 는 RPC 요청 구독 필터이다(+ 와일드카드로 모든 requestId 수신).
	topicDeviceRPCRequestSub = "v1/devices/me/rpc/request/+"
	// topicDeviceRPCResponsePrefix 는 디바이스→서버 RPC 응답(업링크) 토픽 접두사이다.
	// 응답은 v1/devices/me/rpc/response/{requestId} 로 발행하며, requestId 는 요청
	// 토픽에서 받은 값을 그대로 사용한다. 본문은 응답 바디(예: {"result":...})이다.
	topicDeviceRPCResponsePrefix = "v1/devices/me/rpc/response/"
)

// ThingsBoard Gateway MQTT API 토픽 상수 (v1/gateway/*).
//
// api_mode="gateway" 일 때 활성화된다. 게이트웨이 API 는 하나의 연결로 다수의 하위
// 디바이스를 다중화(connect/disconnect)하며, 텔레메트리/속성/RPC 를 NAME 으로 감싼다.
// api_mode="device"(기본) 에서는 이 토픽들을 사용하지 않고 Device API(v1/devices/me/*)
// 를 사용한다.
const (
	// topicGatewayConnect 는 하위 디바이스를 게이트웨이에 연결(및 서버 측 자동 생성)하는 토픽이다.
	topicGatewayConnect = "v1/gateway/connect"
	// topicGatewayDisconnect 는 하위 디바이스 연결을 해제하는 토픽이다.
	topicGatewayDisconnect = "v1/gateway/disconnect"
	// topicGatewayTelemetry 는 게이트웨이 텔레메트리 업링크 토픽이다 (gateway 모드).
	// 페이로드: {"<NAME>":[{"ts":<epoch_ms>,"values":{<kv>}}]}
	topicGatewayTelemetry = "v1/gateway/telemetry"
	// topicGatewayAttributes 는 게이트웨이 클라이언트 속성 업링크 / 공유 속성 다운링크 토픽이다 (gateway 모드).
	topicGatewayAttributes = "v1/gateway/attributes"
	// topicGatewayRPC 는 게이트웨이 RPC 다운링크 및 RPC 응답 업링크 토픽이다 (gateway 모드).
	topicGatewayRPC = "v1/gateway/rpc"
)

// ThingplusConfig 는 thingplus-gateway 에이전트 설정이다.
type ThingplusConfig struct {
	// APIMode 는 ThingsBoard MQTT API 모드이다: "device" | "gateway".
	//   - "device"(기본): Device API(v1/devices/me/*). 단일 액세스 토큰으로 하나의
	//     디바이스("me")를 인증한다. sub-device connect 개념이 없어 텔레메트리를 곧바로 발행한다.
	//   - "gateway": Gateway API(v1/gateway/*). 하나의 연결로 다수의 하위 디바이스를
	//     connect/disconnect 로 다중화하며, 텔레메트리/속성/RPC 를 NAME 으로 감싼다.
	// 알 수 없거나 빈 값은 "device" 로 보정한다.
	APIMode string `json:"api_mode"`

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

	// ClientID 는 MQTT 클라이언트 식별자이다.
	// 비우면 "xflow-thingplus-<uuid>" 형태로 자동 생성되어 게이트웨이 인스턴스마다 고유해진다.
	// 고정 값을 지정하면 여러 인스턴스가 동일 client_id를 공유해 브로커가 한쪽을 끊는
	// (EOF flapping) 충돌이 발생할 수 있으므로 인스턴스마다 고유해야 한다.
	ClientID string `json:"client_id"`

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

	// LogMessages 는 송/수신(TX/RX) 프레임을 hex 로 INFO 로그 출력할지 여부이다
	// (기본값 false, opt-in 진단용). socket/serial 계열의 log_messages 옵션과 동형이며,
	// 운영 환경에서는 로그 폭주·민감 데이터 노출 우려로 비활성을 권장한다.
	LogMessages bool `json:"log_messages"`
}

// mqttPublisher 는 디바이스 상태 머신이 발행에 필요로 하는 최소 인터페이스이다.
// 실제 *mqtt.Client가 이를 만족하며, 테스트에서는 fake 발행자를 주입한다.
type mqttPublisher interface {
	Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token
	IsConnected() bool
}

// mqttSubscriber 는 다운링크 토픽 구독에 필요한 최소 인터페이스이다.
// 실제 mqtt.Client 가 이를 만족하며, 테스트에서는 fake 구독자를 주입한다.
type mqttSubscriber interface {
	Subscribe(topic string, qos byte, callback mqtt.MessageHandler) mqtt.Token
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
	// stopped 는 Stop() 이 호출되었음을 나타내는 가드 플래그이다.
	// Paho 는 SetAutoReconnect(true)+SetConnectRetry(true) 로 백그라운드 재연결
	// goroutine 을 유지하며, Disconnect() 만으로는 flapping 중에 이 재연결 루프가
	// 확실히 취소되지 않아 Stop 이후에도 재연결에 성공할 수 있다. 재연결이 성공하면
	// onConnect 가 다운링크 토픽을 재구독하고 디바이스를 재connect 하여 세션이 부활한다.
	// 이를 방지하기 위해 Stop() 에서 stopped=true 로 설정하고, onConnect 가 stopped
	// 상태이면 즉시 Disconnect 후 재구독 없이 반환하여 부활을 무력화한다.
	// Init()/재시작 경로에서 stopped=false 로 리셋되어 정상 재시작이 가능하다.
	stopped   atomic.Bool
	stats     *agent.AgentStats
	logger    *slog.Logger
	mu        sync.RWMutex
	startedAt time.Time
	createdAt time.Time

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

	// rpcResponsePublisher 는 Device API RPC 응답 발행자 override 이다(테스트 주입용).
	// nil 이면 실제 client(uplinkPublisher)를 사용한다. 프로덕션 경로에서는 항상 nil 이다.
	rpcResponsePublisher mqttPublisher
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
	_ agent.TransportChecker        = (*ThingplusGatewayAgent)(nil)
)

// parseThingplusConfig 는 AgentConfig에서 ThingplusConfig를 파싱한다.
// 기본값: Port=1883(TLS 시 8883), DeviceNamePath="$.device", QoS=1,
// KeepAliveSec=60, AutoReconnect=true, BufferSize=256.
func parseThingplusConfig(cfg agent.AgentConfig) ThingplusConfig {
	tc := ThingplusConfig{
		APIMode:           "device",
		Broker:            "localhost",
		Port:              1883,
		DeviceNamePath:    "$.device",
		QoS:               1,
		KeepAliveSec:      60,
		AutoReconnect:     true,
		BufferSize:        256,
		ConnectTimeoutSec: 10,
		// client_id 기본값은 인스턴스마다 고유하게 자동 생성한다.
		// 결정적(config.ID 기반) 값을 쓰면 동일 config에서 두 번 생성될 때
		// 동일 client_id로 충돌하여 브로커가 EOF로 한쪽을 끊는 flapping이 발생한다.
		ClientID: "xflow-thingplus-" + uuid.New().String(),
	}

	opts := cfg.Transport.Options
	if opts == nil {
		return tc
	}

	// api_mode: "gateway" 또는 "device" 만 허용하며, 그 외/빈 값은 기본 "device" 를 유지한다.
	if v, ok := opts["api_mode"].(string); ok && (v == "gateway" || v == "device") {
		tc.APIMode = v
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
	if v, ok := opts["client_id"].(string); ok && v != "" {
		tc.ClientID = v
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
	// log_messages (기본값: false) — 송/수신(TX/RX) 프레임을 hex 로 INFO 로그 출력.
	// Web UI select/toggle 는 bool 또는 string 으로 전달될 수 있으므로 관대하게 파싱한다.
	if v, ok := opts["log_messages"]; ok {
		tc.LogMessages = toBool(v)
	}

	return tc
}

// toBool 은 bool 또는 string 값을 bool 로 변환한다.
// JSON/YAML 파싱에서는 bool 로, Web UI toggle 필드에서는 string("true"/"1")으로 전달될 수
// 있으므로 두 타입을 모두 처리하며, 인식되지 않는 값은 관대하게 false 로 본다
// (socket/serial 계열 toBool 관례와 동형).
func toBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return b == "true" || b == "1"
	default:
		return false
	}
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

	// client_id 자동 생성 여부 확인 및 로깅.
	// 사용자가 명시하지 않은 경우에만 자동 생성되었음을 로깅한다 (mqtt_agent 패턴).
	// access_token은 로그에 노출하지 않는다.
	userSetClientID := false
	if opts := config.Transport.Options; opts != nil {
		if v, ok := opts["client_id"].(string); ok && v != "" {
			userSetClientID = true
		}
	}
	if !userSetClientID {
		slog.Info("thingplus: client_id 자동 생성됨",
			"client_id", tc.ClientID,
		)
	}

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

	// 재시작 경로(Start: Stopped→Created→Init)에서 stopped 가드를 해제하여
	// 정상적으로 재연결/재구독이 가능하게 한다. 비활성화 경로에서도 항상 리셋하여
	// 이후 enable→Start 재-Init 시 stopped 가드가 남아 연결을 막지 않게 한다.
	a.stopped.Store(false)

	// agentConfig 를 먼저 저장한다. 비활성화 게이트에서 조기 반환하더라도, 이후
	// enable→Start 경로가 a.agentConfig 를 읽어 Init 을 재호출할 수 있어야 하기 때문이다.
	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	// 비활성화 게이트 (SPEC-AGENT-005): 데몬 부팅 시 restoreAgents 는 List API 노출을
	// 위해 disabled 에이전트도 Create(=Init) 한다. 이때 게이트웨이 브로커에 연결하면 안
	// 되므로, Connect() 와 StateRunning 전이를 건너뛴다. lifecycle 은 Initializing→Stopped
	// 가 invalid 하므로(Created→Initializing 만 허용) 전이 없이 StateCreated 에 머문다.
	// 결과: Running 아님, 미연결(client 미생성이므로 TransportConnected()==false),
	// 등록됨(List API 노출 가능). 이후 enable→Start 가 Init 을 재호출해 연결한다.
	if !config.IsEnabled() {
		a.logger.Info("thingplus: 비활성화 상태로 생성됨 — 연결 건너뜀",
			"broker", thingplusBrokerURL(a.cfg),
			"client_id", a.cfg.ClientID,
		)
		return nil
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("thingplus init: %w", err)
	}

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
		SetClientID(a.cfg.ClientID).
		SetUsername(a.cfg.AccessToken).
		SetPassword("").
		SetKeepAlive(time.Duration(a.cfg.KeepAliveSec) * time.Second).
		SetAutoReconnect(a.cfg.AutoReconnect).
		SetCleanSession(true).
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
	// stopped-guard: Stop() 이후 Paho 의 connect-retry/auto-reconnect goroutine 이
	// 살아남아 재연결에 성공하면 onConnect 가 다시 호출된다. 이때 다운링크 재구독 및
	// 디바이스 재connect 를 수행하면 세션이 부활하고 중복 세션이 게이트웨이를 EOF 로
	// kick 하는 상황이 지속되므로, stopped 상태이면 즉시 연결을 끊고 반환한다.
	// (Disconnect 만으로는 flapping 중 재연결 루프가 확실히 취소되지 않는 Paho 특성 대응.)
	if a.stopped.Load() {
		a.logger.Info("thingplus: Stop 이후 재연결 감지 — 재구독/재connect 없이 즉시 종료(세션 부활 방지)")
		c.Disconnect(0)
		return
	}

	// 콜백 goroutine 에서 실행되므로 런타임 Configure() 와의 레이스를 방지하기 위해 스냅샷한다.
	cfg := a.snapshotConfig()
	a.logger.Info("thingplus: 게이트웨이 브로커에 연결됨",
		"broker", thingplusBrokerURL(cfg),
		"access_token", maskSecret(cfg.AccessToken),
	)

	// 다운링크 토픽 구독 (모드별 토픽).
	a.subscribeDownlink(c)

	// gateway 모드에서는 이전에 connected 였던 하위 디바이스를 재connect 한다
	// (v1/gateway/connect 재발행, REQ-map-reconnect). device 모드는 sub-device connect
	// 개념이 없으므로 재connect 하지 않는다.
	if cfg.APIMode == "gateway" {
		a.reconnectKnownDevices(c)
	}

	// 재연결 후 버퍼링된 업링크를 flush 한다 (REQ-up-nolost / AC-05).
	a.flushUplinkBuffer(c)
}

// subscribeDownlink 는 현재 api_mode 에 맞는 다운링크 토픽을 구독한다.
//
// device 모드(기본):
//   - v1/devices/me/attributes         : 공유 속성 변경 push
//   - v1/devices/me/rpc/request/+       : 서버→디바이스 RPC 요청(+ 로 모든 requestId 수신)
//
// gateway 모드:
//   - v1/gateway/rpc                    : 게이트웨이 RPC 요청/응답
//   - v1/gateway/attributes             : 게이트웨이 공유 속성 변경 push
//
// c 는 mqttSubscriber 인터페이스로 받아 테스트에서 fake 구독자 주입이 가능하다
// (실제 mqtt.Client 가 이를 만족한다).
func (a *ThingplusGatewayAgent) subscribeDownlink(c mqttSubscriber) {
	cfg := a.snapshotConfig()
	qos := cfg.QoS
	var topics []string
	if cfg.APIMode == "gateway" {
		topics = []string{topicGatewayRPC, topicGatewayAttributes}
	} else {
		topics = []string{topicDeviceAttributes, topicDeviceRPCRequestSub}
	}
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
	// RX 로그 gate: 브로커로부터의 유일한 수신 진입점이므로 여기서 한 번만 로깅한다.
	logThingplusFrame(a.logger, a.snapshotConfig().LogMessages, "RX", msg.Topic(), data)
	a.routeDownlink(msg.Topic(), data)
}

// routeDownlink 는 토픽/페이로드로부터 플로우 메시지 바이트를 조립하여 recvCh 에 방출한다.
// downlinkHandler 와 통합 테스트가 공통으로 사용하는 순수(브로커 비의존) 라우팅 경로이다.
func (a *ThingplusGatewayAgent) routeDownlink(topic string, data []byte) {
	var out []byte

	// Device API RPC 요청: 토픽 v1/devices/me/rpc/request/{requestId} (동적 suffix).
	// switch(정적 토픽) 이전에 접두사로 먼저 판별한다.
	if strings.HasPrefix(topic, topicDeviceRPCRequestPrefix) {
		requestID := strings.TrimPrefix(topic, topicDeviceRPCRequestPrefix)
		if built, err := a.buildDeviceRPCRequestMessage(requestID, data); err == nil {
			out = built
		} else {
			a.logger.Warn("thingplus: device RPC 요청 다운링크 파싱 실패, 원시 fallback",
				"request_id", requestID,
				"error", err,
			)
		}
		if out == nil {
			out = data
		}
		a.emitDownlink(topic, out)
		return
	}

	switch topic {
	case topicDeviceAttributes:
		// Device API 공유 속성 다운링크 (현재 경로).
		if built, err := a.buildDeviceSharedAttrDownlinkMessage(data); err == nil {
			out = built
		} else {
			a.logger.Warn("thingplus: device 공유 속성 다운링크 파싱 실패, 원시 fallback", "error", err)
		}
	case topicGatewayRPC:
		// 게이트웨이 RPC 다운링크 (gateway 모드). id 는 payload 에 담긴다.
		if built, err := a.buildRPCDownlinkMessage(data); err == nil {
			out = built
		} else {
			a.logger.Warn("thingplus: RPC 다운링크 파싱 실패, 원시 fallback", "error", err)
		}
	case topicGatewayAttributes:
		// 게이트웨이 공유 속성 다운링크 (gateway 모드).
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

// buildDeviceSharedAttrDownlinkMessage 는 Device API 공유 속성 다운링크 페이로드를
// 파싱하여 Type "thingplus.attr.update" 플로우 메시지 JSON 을 조립한다.
//
// Device API 공유 속성은 "me" 를 대상으로 하므로 페이로드에 device NAME 이 없다.
// 따라서 device/device_id 메타데이터는 확보 가능한 범위에서만 채운다(현재는 비어 있음).
// 속성 데이터는 payload["data"] 에 담아 기존 방출 계약(thingplus.attr.update)을 보존한다.
func (a *ThingplusGatewayAgent) buildDeviceSharedAttrDownlinkMessage(data []byte) ([]byte, error) {
	attrs, err := parseDeviceSharedAttributes(data)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"device":    "",
		"device_id": "",
		"data":      attrs,
	}
	return a.buildFlowMessageJSON("thingplus.attr.update", "", "", payload)
}

// buildDeviceRPCRequestMessage 는 Device API RPC 요청 다운링크를 파싱하여
// Type "thingplus.rpc.request" 플로우 메시지 JSON 을 조립한다.
//
// requestID 는 토픽 v1/devices/me/rpc/request/{requestId} 의 suffix 이다. 방출 페이로드
// 의 "id" 에는 이 토픽 값을 문자열 원본 그대로 담아, 플로우가 echo 한 응답이 정확히
// v1/devices/me/rpc/response/{id} 로 라우팅되도록 보장한다(id-in-topic 계약).
// 페이로드는 {"method":..,"params":..} 이며 requestId 는 페이로드에 없다.
func (a *ThingplusGatewayAgent) buildDeviceRPCRequestMessage(requestID string, data []byte) ([]byte, error) {
	method, params, err := parseDeviceRPCRequest(data)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		// id 는 토픽 suffix 문자열을 그대로 보존한다(응답 토픽 라우팅 정확성 보장).
		"id":     requestID,
		"method": method,
		"params": params,
	}
	// Device API RPC 는 "me" 대상이므로 device/device_id 메타데이터는 비운다.
	return a.buildFlowMessageJSON("thingplus.rpc.request", "", "", payload)
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

	logThingplusFrame(a.logger, a.snapshotConfig().LogMessages, "TX", topicGatewayConnect, payload)
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

	logThingplusFrame(a.logger, a.snapshotConfig().LogMessages, "TX", topicGatewayDisconnect, payload)
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

	logThingplusFrame(a.logger, a.snapshotConfig().LogMessages, "TX", topic, payload)
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
	case lifecycle.StateCreated:
		// 비활성화 상태로 생성된 에이전트(Init 비활성화 게이트가 StateCreated 에 남김)를
		// 시작하는 경로이다. 이미 Created 이므로 리셋 없이 Init() 을 재호출한다.
		// enable 이후라면 agentConfig.IsEnabled()==true 이므로 Init 이 연결하며,
		// 여전히 비활성화라면 게이트가 다시 조기 반환하여 연결하지 않는다(no-op).
		a.mu.RLock()
		cfg := a.agentConfig
		a.mu.RUnlock()
		return a.Init(cfg)
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
// idempotent: 이미 Stopped 상태에서 재호출되어도 에러 없이 항상 Disconnect 하여
// 연결 끊김을 보장한다 (manager 가 desync 감지 시 강제 disconnect 목적으로 재호출).
func (a *ThingplusGatewayAgent) Stop(_ context.Context) error {
	// stopped 가드를 먼저 설정한다. Stop 이후 Paho 가 재연결에 성공해 onConnect 가
	// 호출되더라도, 가드가 즉시 Disconnect 하고 재구독/재connect 하지 않아 세션 부활을 막는다.
	a.stopped.Store(true)

	// 이미 Stopped 상태이면 lifecycle 전이(Stopped→Stopping 은 invalid)를 건너뛴다.
	// 그래도 아래에서 Disconnect 는 항상 호출하여 연결 끊김을 보장한다.
	transition := a.CurrentState() != lifecycle.StateStopped
	if transition {
		if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
			return fmt.Errorf("thingplus stop: %w", err)
		}
	}

	// Disconnect 는 IsConnected() 여부와 무관하게 항상 호출한다.
	// Paho 는 SetAutoReconnect(true)+SetConnectRetry(true) 로 백그라운드 재연결
	// goroutine 을 유지하는데, 이 재연결 machinery 를 취소하는 유일한 방법이 Disconnect() 이다.
	// flapping(현재 연결 끊김) 중에 Stop 이 호출되면 IsConnected()==false 이므로 예전에는
	// Disconnect 를 건너뛰어 auto-reconnect 가 살아남아 계속 재연결했고, 그 중복 세션이
	// thingplus-gateway 를 EOF 로 kick 하는 상황을 지속시켰다. 이를 방지하기 위해
	// 연결 상태와 무관하게 Disconnect 를 호출한다.
	if a.client != nil {
		a.client.Disconnect(250)
	}

	// ReceiveMessage 대기자에게 종료 시그널 (중복 Stop 호출 시 panic 방지)
	a.doneOnce.Do(func() { close(a.done) })

	// 버퍼에 남은 메시지 드레인
	for {
		select {
		case <-a.recvCh:
		default:
			// idempotent: 이미 Stopped 이면 전이를 건너뛴다(Stopped→Stopped 는 invalid).
			if transition {
				if err := a.TransitionTo(lifecycle.StateStopped); err != nil {
					return fmt.Errorf("thingplus stop: %w", err)
				}
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

// TransportConnected 는 실제 Paho 클라이언트의 연결 여부를 반환한다.
// agent.TransportChecker 인터페이스 구현. 이를 통해 API 의 connected 필드가
// 라이프사이클 상태가 아닌 실제 브로커 연결 상태를 반영하고, manager 가
// State/트랜스포트 desync 를 감지할 수 있다.
func (a *ThingplusGatewayAgent) TransportConnected() bool {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()
	return client != nil && client.IsConnected()
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

	// RPC 응답 분기: Type 이 thingplus.rpc.response 이면 모드에 맞는 발행자로 라우팅한다.
	//   - device 모드: requestId 가 토픽에 담기고 v1/devices/me/rpc/response/{id} 로 발행("me" 게이팅 없음).
	//   - gateway 모드: id 가 payload 에 담기고 device connected 게이팅 후 v1/gateway/rpc 로 발행.
	if msgType == "thingplus.rpc.response" {
		return nil, a.routeRPCResponse(payload)
	}

	// RPC 응답 분기(Type 미지정 fallback): payload 에 rpc id 가 있으면 RPC 응답으로 처리한다.
	if looksLikeRPCReply(payload) {
		return nil, a.routeRPCResponse(payload)
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

// logThingplusFrame 은 log_messages 옵션이 켜져 있을 때 송/수신 프레임을 hex 로 INFO 로그한다.
//
// enabled 가 false 이거나 logger 가 nil 이면 no-op 이다 (opt-in 진단용).
// dir 은 "TX"(송신) 또는 "RX"(수신), topic 은 대상 MQTT 토픽이다.
//
// socket 계열(tcp/udp)의 logPacket 과 동일한 형식(len + hex)을 사용하여 진단 로그를
// 통일한다. socket.logPacket 은 패키지 private 이라 재사용할 수 없어 로컬로 복제한다.
func logThingplusFrame(logger *slog.Logger, enabled bool, dir, topic string, data []byte) {
	if !enabled || logger == nil {
		return
	}
	logger.Info("thingplus: "+dir, "topic", topic, "len", len(data), "hex", hex.EncodeToString(data))
}

// isGatewayMode 는 현재 설정이 Gateway API 모드(api_mode="gateway")인지 반환한다.
// 그 외(기본 "device" 포함)는 false 이다. 런타임 Configure() 레이스 방지를 위해
// snapshotConfig() 로 스냅샷한 값을 사용한다.
func (a *ThingplusGatewayAgent) isGatewayMode() bool {
	return a.snapshotConfig().APIMode == "gateway"
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

// routeRPCResponse 는 플로우로부터 온 RPC 응답을 현재 api_mode 에 맞는 발행 경로로 라우팅한다.
//   - gateway 모드: handleRPCReply → device connected 게이팅 후 v1/gateway/rpc 로 발행.
//   - device 모드(기본): handleDeviceRPCResponse → v1/devices/me/rpc/response/{id} 로 발행.
func (a *ThingplusGatewayAgent) routeRPCResponse(payload map[string]any) error {
	pub := a.rpcResponsePublisherOrClient()
	if a.isGatewayMode() {
		return a.handleRPCReply(pub, payload)
	}
	return a.handleDeviceRPCResponse(pub, payload)
}

// rpcResponsePublisherOrClient 는 RPC 응답 발행에 사용할 mqttPublisher 를
// 반환한다. 테스트 주입(rpcResponsePublisher)이 있으면 그것을, 없으면 실제 client 를 쓴다.
func (a *ThingplusGatewayAgent) rpcResponsePublisherOrClient() mqttPublisher {
	a.mu.RLock()
	override := a.rpcResponsePublisher
	a.mu.RUnlock()
	if override != nil {
		return override
	}
	return a.uplinkPublisher()
}

// extractDeviceRPCResponseID 는 응답 페이로드에서 requestId(응답 토픽 suffix)를 추출한다.
//
// Device API 에서 requestId 는 요청 토픽에서 전달되어 방출 페이로드의 "id" 로 담겼고,
// 플로우가 이를 echo 하므로 문자열로 보존된다. 다만 플로우 구현에 따라 숫자로 올 수도
// 있어 문자열/숫자 모두 수용한다("rpc_id" 도 fallback 으로 허용).
func extractDeviceRPCResponseID(payload map[string]any) (string, bool) {
	for _, key := range []string{"id", "rpc_id"} {
		v, ok := payload[key]
		if !ok {
			continue
		}
		switch n := v.(type) {
		case string:
			if n != "" {
				return n, true
			}
		case float64:
			// JSON 숫자는 float64 로 언마샬된다. 정수 형태로 문자열화한다.
			return fmt.Sprintf("%d", int64(n)), true
		case int:
			return fmt.Sprintf("%d", n), true
		case int64:
			return fmt.Sprintf("%d", n), true
		}
	}
	return "", false
}

// extractDeviceRPCResponseBody 는 응답 발행 본문을 추출한다.
//
// 규칙: payload.data(맵) 또는 payload.result 가 있으면 그것을 우선 사용하고, 없으면
// 상관/식별 키(id/rpc_id/device/device_id)를 제외한 나머지를 본문으로 사용한다.
// requestId 는 토픽에 담기므로 본문에 포함되지 않는다.
func extractDeviceRPCResponseBody(payload map[string]any) map[string]any {
	if d, ok := payload["data"].(map[string]any); ok {
		return d
	}
	if r, ok := payload["result"]; ok {
		// result 는 임의 값일 수 있으므로 {"result":<v>} 형태로 감싼다.
		return map[string]any{"result": r}
	}
	body := make(map[string]any, len(payload))
	for k, v := range payload {
		switch k {
		case "id", "rpc_id", "device", "device_id":
			continue
		default:
			body[k] = v
		}
	}
	return body
}

// handleDeviceRPCResponse 는 플로우로부터 온 Device API RPC 응답을
// v1/devices/me/rpc/response/{id} 로 발행한다.
//
// requestId(id)는 방출된 요청 페이로드에서 echo 되어 payload["id"] 에 담긴다. 이를
// 토픽 suffix 로 사용하여 서버가 원래 요청과 상관(correlate)할 수 있게 한다. 응답 본문은
// extractDeviceRPCResponseBody 로 추출하며, id 등 식별 키는 본문에서 제외된다.
//
// Device API 에는 "me" connect 게이팅이 없으므로 브로커 연결 시 곧바로 발행한다.
// pub 이 nil 또는 미연결이면 발행할 수 없어 에러를 반환한다.
func (a *ThingplusGatewayAgent) handleDeviceRPCResponse(pub mqttPublisher, payload map[string]any) error {
	id, ok := extractDeviceRPCResponseID(payload)
	if !ok {
		return fmt.Errorf("thingplus device rpc response: requestId(id) 를 찾을 수 없음")
	}

	if pub == nil || !pub.IsConnected() {
		return fmt.Errorf("thingplus device rpc response: 브로커에 연결되어 있지 않음")
	}

	body := extractDeviceRPCResponseBody(payload)
	out, err := buildDeviceRPCResponse(body)
	if err != nil {
		return err
	}

	topic := topicDeviceRPCResponsePrefix + id
	logThingplusFrame(a.logger, a.snapshotConfig().LogMessages, "TX", topic, out)
	token := pub.Publish(topic, a.snapshotConfig().QoS, false, out)
	token.Wait()
	if token.Error() != nil {
		a.stats.IncrExternalMessagesErrored()
		return fmt.Errorf("thingplus device rpc response: 발행 실패: %w", token.Error())
	}
	a.uplinkCount.Add(1)
	a.stats.IncrExternalMessagesSent()
	a.logger.Info("thingplus: device RPC 응답 발행 완료", "topic", topic)
	return nil
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

	logThingplusFrame(a.logger, a.snapshotConfig().LogMessages, "TX", topicGatewayRPC, out)
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
	// Device API 에서는 sub-device connect 개념이 없으나, NAME↔device_id 매핑은
	// 관찰(observability) 목적으로 계속 유지한다(무해).
	deviceID := a.mapping.resolveDeviceID(context.Background(), a.Name(), name)
	a.devices.ensure(name)
	a.devices.setDeviceID(name, deviceID)

	// 텔레메트리 values 및 클라이언트 속성 분리.
	values, attrs := splitUplinkPayload(payload, cfg.DeviceNamePath)

	connected := pub != nil && pub.IsConnected()

	if cfg.APIMode == "gateway" {
		return a.handleUplinkGateway(pub, name, values, attrs, connected, cfg.QoS)
	}
	return a.handleUplinkDevice(pub, name, values, attrs, connected, cfg.QoS)
}

// handleUplinkDevice 는 Device API(v1/devices/me/*) 업링크를 처리한다.
//
// sub-device gateway connect 가 없으므로 auto-connect 를 수행하지 않고 텔레메트리를 곧바로
// 발행한다. 연결 끊김 시에는 무손실 버퍼링하고 재연결 시 flush 한다(REQ-up-nolost 유지).
func (a *ThingplusGatewayAgent) handleUplinkDevice(pub mqttPublisher, name string, values, attrs map[string]any, connected bool, qos byte) error {
	// 텔레메트리 발행 (값이 있을 때). Device API 형식:
	// {"ts":<UnixMilli>,"values":{"<name>":{<kv>}}}. ts 는 현재 시각으로 채운다.
	if len(values) > 0 {
		ts := time.Now()
		telemetry, err := buildDeviceTelemetry(name, &ts, values)
		if err != nil {
			return err
		}
		if err := a.publishOrBuffer(pub, topicDeviceTelemetry, telemetry, connected, qos); err != nil {
			return err
		}
	}

	// 클라이언트 속성 발행 (있을 때). Device API 형식: flat {"<k>":<v>} 를
	// v1/devices/me/attributes 로 발행한다.
	if len(attrs) > 0 {
		clientAttr, err := buildDeviceClientAttributes(attrs)
		if err != nil {
			return err
		}
		if err := a.publishOrBuffer(pub, topicDeviceAttributes, clientAttr, connected, qos); err != nil {
			return err
		}
	}

	return nil
}

// handleUplinkGateway 는 Gateway API(v1/gateway/*) 업링크를 처리한다.
//
// 흐름: 연결 상태이고 디바이스가 아직 connected 가 아니면 auto-connect(connectDevice) 하여
// 텔레메트리 발행 전에 게이팅(A6/A7)한다. 그 후 텔레메트리({"<NAME>":[{ts,values}]})를
// v1/gateway/telemetry 로, 클라이언트 속성({"<NAME>":{...}})을 v1/gateway/attributes 로
// 발행한다. 브로커 연결이 끊겨 있으면 무손실 버퍼링하고 재연결 시 flush 한다.
func (a *ThingplusGatewayAgent) handleUplinkGateway(pub mqttPublisher, name string, values, attrs map[string]any, connected bool, qos byte) error {
	// auto-connect: 연결 상태에서 디바이스가 아직 connected 가 아니면 connect 한다.
	// connect 발행이 실패하면 디바이스는 connected 로 전이하지 않으며, 아래 게이팅으로
	// 텔레메트리는 버퍼링된다(무손실).
	if connected {
		if e, ok := a.devices.get(name); !ok || e.state != deviceConnected {
			if err := a.connectDevice(pub, name); err != nil {
				a.logger.Warn("thingplus: gateway auto-connect 실패 — 텔레메트리 버퍼링",
					"device", name,
					"error", err,
				)
			}
		}
	}

	// A6/A7 게이팅: connected(브로커) + 디바이스 connected 일 때만 즉시 발행하고,
	// 그렇지 않으면 무손실 버퍼링한다.
	e, ok := a.devices.get(name)
	deviceReady := connected && ok && e.state == deviceConnected

	// 텔레메트리 발행 (값이 있을 때). Gateway 형식: {"<NAME>":[{"ts":<ms>,"values":{...}}]}.
	if len(values) > 0 {
		ts := time.Now()
		telemetry, err := buildTelemetry(name, &ts, values)
		if err != nil {
			return err
		}
		if err := a.publishOrBuffer(pub, topicGatewayTelemetry, telemetry, deviceReady, qos); err != nil {
			return err
		}
	}

	// 클라이언트 속성 발행 (있을 때). Gateway 형식: {"<NAME>":{"<k>":<v>}}.
	if len(attrs) > 0 {
		clientAttr, err := buildClientAttributes(name, attrs)
		if err != nil {
			return err
		}
		if err := a.publishOrBuffer(pub, topicGatewayAttributes, clientAttr, deviceReady, qos); err != nil {
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
		logThingplusFrame(a.logger, a.snapshotConfig().LogMessages, "TX", topic, payload)
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
	logMessages := a.snapshotConfig().LogMessages
	for _, it := range items {
		logThingplusFrame(a.logger, logMessages, "TX", it.topic, it.payload)
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
		"api_mode":              cfg.APIMode,
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
