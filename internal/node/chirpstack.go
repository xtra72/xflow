package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// chirpStackDefaultBufferSize 는 노드 source 채널의 기본 버퍼 크기이다.
const chirpStackDefaultBufferSize = 256

// ChirpStackInNode 는 ChirpStack 에이전트가 fan-out 한 per-measurement 레코드를
// 소비하여 flow message 로 방출하는 수신 전용 SourceNode 이다
// (SPEC-CHIRPSTACK-001, REQ-M3-06, REQ-FROZEN-01/02).
//
// mqtt-subscriber 패턴을 미러링한다: agent_ref 로 에이전트를 resolve 하고
// MessageReceiver 로 레코드를 수신, receiveLoop 로 sourceCh 에 전달한다. 기존 Lua
// script+split 파이프라인을 대체하며 store/influx 소비자에 직접 연결된다.
type ChirpStackInNode struct {
	*BaseNode

	agentRef     string
	emitMetadata MetadataEmitOptions

	resolver  AgentResolver
	transport AgentTransport
	agent     agent.Agent
	receiver  agent.MessageReceiver

	// agentName 은 initAgent 에서 1회 pre-capture 한다 — 방출 경로에서 매번
	// a.Name() 을 호출하면 HVAC 락 함정(재귀 RLock)에 노출되기 때문이다.
	// 에이전트 재시작 시 인스턴스가 교체되므로 Reinit 에서 재캡처한다.
	agentName string

	sourceCh chan message.Message
	stopCh   chan struct{}
	stopOnce sync.Once
	mu       sync.RWMutex
}

var (
	_ Node       = (*ChirpStackInNode)(nil)
	_ SourceNode = (*ChirpStackInNode)(nil)
	// AgentReinitializer 미구현은 엔진의 노드 재초기화 루프에서 조용히 건너뛰어져
	// 에이전트 재시작 후 노드가 영구 무음이 된다. 컴파일 타임에 고정한다.
	_ AgentReinitializer = (*ChirpStackInNode)(nil)
)

// NewChirpStackInNode 는 ChirpStackInNode 팩토리 함수이다.
func NewChirpStackInNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &ChirpStackInNode{
		BaseNode:     base,
		emitMetadata: DefaultEmitOptions(),
		sourceCh:     make(chan message.Message, chirpStackDefaultBufferSize),
		stopCh:       make(chan struct{}),
	}
	n.resolver = extractResolverFromConfig(base)
	return n, nil
}

// Configure 는 노드 설정을 적용한다. agent_ref(필수)를 검증한다.
func (n *ChirpStackInNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	var ref string
	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok {
			ref = s
		}
	}
	if ref == "" {
		return fmt.Errorf("chirpstack-in: agent_ref 는 필수입니다")
	}

	var emit MetadataEmitOptions
	parseEmitMetadata(config, &emit)

	n.mu.Lock()
	n.agentRef = ref
	n.emitMetadata = emit
	n.mu.Unlock()

	if r := extractResolverFromConfig(n.BaseNode); r != nil {
		n.resolver = r
	}
	return nil
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다 (AgentReinitializer /
// flow validate 의 agent_ref 필수 검증 대상).
func (n *ChirpStackInNode) AgentRef() flow.AgentRef {
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// initAgent 는 AgentResolver 로 에이전트를 resolve 하고 MessageReceiver 를 확인한다.
//
// agent / receiver / agentName 은 Reinit 이 런타임에 교체하므로(수신 루프가 동시에
// 읽음) 반드시 n.mu 하에서 기록한다.
func (n *ChirpStackInNode) initAgent(ctx context.Context) error {
	if n.resolver == nil {
		return fmt.Errorf("chirpstack-in: AgentResolver 미주입")
	}
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()

	transport, err := n.resolver.ResolveAgent(ctx, flow.AgentRef{AgentID: ref, AgentName: ref})
	if err != nil {
		return fmt.Errorf("chirpstack-in: agent resolve 실패: %w", err)
	}

	var resolved agent.Agent
	if accessor, ok := transport.(AgentAccessor); ok {
		resolved = accessor.UnderlyingAgent()
	}
	recv, ok := resolved.(agent.MessageReceiver)
	if !ok {
		return fmt.Errorf("chirpstack-in: 에이전트가 MessageReceiver 를 구현하지 않음")
	}

	// HVAC 락 함정 회피: Name() 은 n.mu 를 잡기 전에 1회 호출한다.
	agentName := resolved.Name()

	n.mu.Lock()
	n.transport = transport
	n.agent = resolved
	n.receiver = recv
	n.agentName = agentName
	n.mu.Unlock()
	return nil
}

// Init 은 노드를 초기화하고 수신 루프를 시작한다.
func (n *ChirpStackInNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if err := n.initAgent(ctx); err != nil {
		return err
	}
	go n.receiveLoop()
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// receiveLoop 는 에이전트로부터 per-measurement 레코드를 수신하여 message 로 빌드해
// sourceCh 에 전달한다.
//
// agent / receiver / agentName / stopCh 는 루프 시작 시 1회 캡처한다. 특히 stopCh 은
// 반드시 "이 루프 세대(generation)"의 채널이어야 한다 — Reinit 이 stopCh 을 새 채널로
// 교체하므로, 매 반복마다 n.stopCh 을 다시 읽으면 구 루프가 새(열린) 채널을 보고
// 종료하지 못해 누수 + 이중 소비가 발생한다.
func (n *ChirpStackInNode) receiveLoop() {
	n.mu.RLock()
	a := n.agent
	recv := n.receiver
	opts := n.emitMetadata
	agentName := n.agentName
	stopCh := n.stopCh
	n.mu.RUnlock()

	var throttle receiveFailureThrottle

	for {
		select {
		case <-stopCh:
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), chirpStackReceiveTimeout)
		data, err := recv.ReceiveMessage(ctx)
		cancel()
		if err != nil || data == nil {
			// 무음 실패 방지: 에이전트 교체 등으로 수신이 영구 실패해도 진단 로그가
			// 전혀 남지 않던 결함을 rate-limited Warn 으로 보완한다. 정상 유휴 경로인
			// 5초 타임아웃은 실패로 계상하지 않는다.
			if count, emit := throttle.note(err, time.Now()); emit {
				if lg := n.Logger(); lg != nil {
					lg.Warn("chirpstack-in: 수신 실패 지속",
						"node_id", n.ID(),
						"consecutive_failures", count,
						"error", err,
					)
				}
			}
			continue
		}
		throttle.reset()

		msg, ok := buildChirpStackMessage(data, n.ID(), a, agentName, opts)
		if !ok {
			continue
		}

		select {
		case n.sourceCh <- msg:
		case <-stopCh:
			return
		}
	}
}

// chirpStackReceiveTimeout 은 1회 수신 대기 상한이다. 초과 시 반환되는
// context.DeadlineExceeded 는 "유휴"이며 실패가 아니다.
const chirpStackReceiveTimeout = 5 * time.Second

// chirpStackReceiveWarnInterval 은 수신 실패 경고의 최소 방출 간격이다.
// 실패 시 수신이 즉시 반환되어 루프가 빠르게 회전할 수 있으므로 반드시 rate-limit 한다.
const chirpStackReceiveWarnInterval = 5 * time.Second

// receiveFailureThrottle 는 receiveLoop 의 수신 실패 경고를 rate-limit 한다.
// 단일 receiveLoop goroutine 에서만 접근하므로 락이 필요 없다.
type receiveFailureThrottle struct {
	consecutive int
	lastWarnAt  time.Time
}

// note 는 수신 결과를 1건 기록하고 (연속 실패 횟수, 경고 방출 여부) 를 반환한다.
//
//   - context.DeadlineExceeded 는 정상 유휴 경로이므로 실패로 계상하지 않고 경고도
//     내지 않는다(그대로 두면 5초마다 로그가 범람한다).
//   - 그 외 실패는 첫 발생 시 즉시 1회, 이후에는 chirpStackReceiveWarnInterval 마다
//     1회만 경고한다.
func (t *receiveFailureThrottle) note(err error, now time.Time) (int, bool) {
	if err != nil && errors.Is(err, context.DeadlineExceeded) {
		return 0, false
	}
	t.consecutive++
	if t.lastWarnAt.IsZero() || now.Sub(t.lastWarnAt) >= chirpStackReceiveWarnInterval {
		t.lastWarnAt = now
		return t.consecutive, true
	}
	return t.consecutive, false
}

// reset 은 수신 성공 시 연속 실패 카운터를 초기화한다.
func (t *receiveFailureThrottle) reset() {
	t.consecutive = 0
}

// buildChirpStackMessage 는 에이전트가 emit 한 per-measurement 레코드(JSON)를 flow
// message 로 빌드한다 (REQ-FROZEN-01/02, REQ-M3-02/03/04).
//
// 다운스트림 계약(보존): $.type=event, $.timestamp(업링크 time),
// $.payload.value, $.metadata.measurement, $.metadata.tags.*, 그리고 노드가
// unit_id 를 승격한 $.metadata.device.{id,name}.
func buildChirpStackMessage(data []byte, nodeID string, a agent.Agent, agentName string, opts MetadataEmitOptions) (message.Message, bool) {
	// 판별자 peek: record=="device_state" 면 comm-state fold 메시지로 빌드한다
	// (REQ-FROZEN-03). measurementRecord 는 record 를 비워 두므로 event 로 취급.
	var disc struct {
		Record string `json:"record"`
	}
	_ = json.Unmarshal(data, &disc)
	switch disc.Record {
	case "device_state":
		return buildChirpStackDeviceStateMessage(data, nodeID, a, agentName, opts)
	case chirpStackRecordCombined:
		return buildChirpStackCombinedMessage(data, nodeID, a, agentName, opts)
	case chirpStackRecordMeasurementReport:
		return buildChirpStackMeasurementReportMessage(data, nodeID, a, agentName, opts)
	}

	var rec struct {
		Measurement string                 `json:"measurement"`
		Value       any                    `json:"value"`
		UnitID      string                 `json:"unit_id"`
		TimeMs      int64                  `json:"time_ms"`
		Tags        map[string]string      `json:"tags"`
		Radio       *chirpStackRadioRecord `json:"radio"`
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, false
	}

	msg := message.New()
	msg.SetType("event")
	if rec.TimeMs > 0 {
		msg.SetTimestamp(time.UnixMilli(rec.TimeMs))
	}
	if rec.Measurement != "" {
		msg.Metadata().Set("measurement", rec.Measurement)
	}
	// tags verbatim pass-through (REQ-FROZEN-04) — 키 매핑 없이 그룹으로 전달.
	if len(rec.Tags) > 0 {
		msg.Metadata().SetGroup("tags", rec.Tags)
	}
	if opts.NodeID {
		msg.Metadata().Set("node_id", nodeID)
	}
	emitAgentGroup(msg, a, opts)

	// payload map 을 구성하고 device 그룹을 승격한다 (unit_id → device.{id,name,type}).
	// promoteDevIDWithUUID 는 payload 에서 unit_id 를 제거하고 device 그룹을 채운다.
	payload := make(map[string]any, 2)
	if rec.Value != nil {
		payload["value"] = rec.Value
	}
	if rec.UnitID != "" {
		payload["unit_id"] = rec.UnitID
	}
	setChirpStackRadio(payload, rec.Radio)
	promoteDevIDWithUUID(msg, payload, agentName, opts)
	// unit_id 는 chirpstack 에서 곧 devEui 이다 — 승격이 payload 에서 지운 값을
	// device 그룹에 디바이스 정보(dev_eui)로 되살린다.
	setChirpStackDevEui(msg, opts, rec.UnitID)
	// 모든 그룹 조립 이후 마지막에 축소한다 (detail OFF 일 때만 동작).
	reduceChirpStackIdentityGroups(msg, opts)

	for k, v := range payload {
		msg.Payload().Set(k, v)
	}
	return msg, true
}

// setChirpStackDevEui 는 device 그룹에 chirpstack 고유 키 dev_eui 를 덧붙인다.
//
// 왜 필요한가: 노드는 레코드의 unit_id(=devEui)를 promoteDevIDWithUUID 로 소비해
// device 그룹의 id(UUID)로 바꾸고 payload 에서 제거한다. 그 결과 devEui 자체는
// 메시지 어디에서도 읽을 수 없다. device_id 는 UUID 여야 하는 플랫폼 불변식이므로
// (SPEC-DEVICE-IDENTITY-001 Phase D), devEui 는 id 를 대체하는 대신 별도 키로 싣는다.
//
// 왜 여기(chirpstack 로컬)인가: mergeDeviceGroup / promoteDevIDWithUUID 는 7개 노드
// 파일 31개 호출부가 공유하는 전역 규약이라 시그니처/동작을 넓히면 blast radius 가
// 그만큼 커진다. 본 헬퍼는 공유 헬퍼를 전혀 건드리지 않고 공개 metadata API 만으로
// 같은 그룹에 키 하나를 더한다 — 다른 프로듀서(HVACR/modbus/serial…)의 출력은
// 한 바이트도 변하지 않는다.
//
// 호출 순서(중요): 반드시 promoteDevIDWithUUID 이후에 호출한다. 그 헬퍼는 내부에서
// mergeDeviceGroup 을 여러 번 호출하지만, mergeDeviceGroup 은 기존 그룹을 읽어
// 병합하므로(전체 치환 아님) 이후 호출이 있어도 dev_eui 는 보존된다. 그럼에도
// 승격 이후로 고정해 두어 순서 의존을 남기지 않는다.
//
// opts.Device 게이팅: device 그룹 emit 자체가 꺼져 있으면 no-op 이다. 게이팅하지
// 않으면 device 그룹이 없어야 할 설정에서 dev_eui 하나 때문에 그룹이 생겨 모양이
// 바뀐다 (mergeDeviceGroup 과 동일한 규율).
func setChirpStackDevEui(msg message.Message, opts MetadataEmitOptions, devEui string) {
	if !opts.Device || devEui == "" {
		return
	}
	fields, _ := msg.Metadata().GetGroup("device")
	if fields == nil {
		fields = make(map[string]string, 4)
	}
	fields["dev_eui"] = devEui
	msg.Metadata().SetGroup("device", fields)
}

// reduceChirpStackIdentityGroups 는 "상세 정보(detail)" 토글이 OFF 일 때 agent /
// device 그룹을 각각 id 하나로 축소한다 (name / type / dev_eui 제거).
//
// 왜 필요한가: 두 그룹은 노드가 조립하므로(에이전트는 {measurement,value,unit_id,
// time_ms,tags} 만 방출) 축소 지점도 노드여야 한다. 다운스트림(store/influx)이 식별자만
// 필요한 배포에서 매 메시지마다 name/type/dev_eui 를 싣는 비용을 없앤다.
//
// 왜 여기(chirpstack 로컬)인가: setChirpStackDevEui 와 동일한 규율이다. 공유 헬퍼
// (emitAgentGroup / SetAgentGroupIfAllowed / mergeDeviceGroup / promoteDevIDWithUUID)
// 는 12개 노드 파일 수십 개 호출부가 공유하는 전역 규약이라 동작을 넓히면 blast radius 가
// 그만큼 커진다. 본 헬퍼는 공유 헬퍼를 전혀 건드리지 않고 공개 metadata API 만으로
// 이미 조립된 그룹을 다시 쓴다 — 다른 프로듀서(HVACR/modbus/serial…)의 출력은
// 한 바이트도 변하지 않는다.
//
// 호출 순서(중요): 반드시 모든 그룹 조립(emitAgentGroup / promoteDevIDWithUUID /
// setChirpStackDevEui) 이후 마지막에 호출한다. 이후에 그룹을 다시 채우면 축소가 무효화된다.
//
// 축소 대상은 agent / device 그룹뿐이다. measurement / tags / timestamp / payload 는
// frozen 계약(SPEC-CHIRPSTACK-002 REQ-FROZEN-A, SPEC-CHIRPSTACK-001 REQ-FROZEN-02)
// 이므로 건드리지 않는다.
//
// 그룹 토글과의 상호작용: opts.Agent / opts.Device 가 OFF 면 애초에 그룹이 없으므로
// reduceMetadataGroupToID 가 no-op 이다 — 빈 그룹을 새로 만들지 않는다.
func reduceChirpStackIdentityGroups(msg message.Message, opts MetadataEmitOptions) {
	if msg == nil || opts.Detail {
		return
	}
	reduceMetadataGroupToID(msg, "agent")
	reduceMetadataGroupToID(msg, "device")
}

// reduceMetadataGroupToID 는 metadata 그룹을 id 키 하나만 남기고 축소한다.
//
// 정책:
//   - 그룹이 없으면 no-op (빈 그룹을 만들지 않는다).
//   - id 가 있으면 {id} 로 치환.
//   - id 가 없으면(UUID 미해석 디바이스 등) 그룹 전체를 제거한다. detail OFF 의 계약은
//     "id 만 emit" 인데 id 가 없으면 emit 할 것이 없다. {} 나 {dev_eui} 같은 잔여
//     그룹을 남기면 계약이 깨지고 다운스트림이 식별자 없는 그룹을 파싱하게 된다.
func reduceMetadataGroupToID(msg message.Message, key string) {
	fields, ok := msg.Metadata().GetGroup(key)
	if !ok || len(fields) == 0 {
		return
	}
	id := fields["id"]
	if id == "" {
		msg.Metadata().Remove(key)
		return
	}
	msg.Metadata().SetGroup(key, map[string]string{"id": id})
}

// chirpStackRecordCombined 는 combined(측정치 통합) 레코드의 판별자 값이다
// (에이전트: recordKindMeasurements). measurement_emit_mode="combined" opt-in 경로에서만
// 나타나며, 기본(per_measurement) 경로의 레코드는 record 를 비워 두므로 영향이 없다.
const chirpStackRecordCombined = "measurements"

// buildChirpStackCombinedMessage 는 combined 레코드(JSON)를 flow message 로 빌드한다
// (measurement_emit_mode="combined").
//
// per-measurement 경로와의 유일한 차이는 payload 모양이다:
//   - payload = 모든 measurement 를 최상위 flat 키로 편 map (payload.value 없음).
//   - metadata.measurement 없음 (단일 measurement 로 특정되지 않으므로).
//
// 그 외 계약은 per-measurement 경로와 동일하다: type="event", timestamp(UnixMilli),
// metadata.tags(verbatim), metadata.device.*(unit_id 승격), agent 그룹.
func buildChirpStackCombinedMessage(data []byte, nodeID string, a agent.Agent, agentName string, opts MetadataEmitOptions) (message.Message, bool) {
	var rec struct {
		Values map[string]any         `json:"values"`
		UnitID string                 `json:"unit_id"`
		TimeMs int64                  `json:"time_ms"`
		Tags   map[string]string      `json:"tags"`
		Radio  *chirpStackRadioRecord `json:"radio"`
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, false
	}

	msg := message.New()
	msg.SetType("event")
	if rec.TimeMs > 0 {
		msg.SetTimestamp(time.UnixMilli(rec.TimeMs))
	}
	// tags verbatim pass-through (REQ-FROZEN-04) — per-measurement 경로와 동일.
	if len(rec.Tags) > 0 {
		msg.Metadata().SetGroup("tags", rec.Tags)
	}
	if opts.NodeID {
		msg.Metadata().Set("node_id", nodeID)
	}
	emitAgentGroup(msg, a, opts)

	// payload = measurement flat 키 + unit_id(승격 후 제거).
	payload := make(map[string]any, len(rec.Values)+1)
	for k, v := range rec.Values {
		payload[k] = v
	}
	if rec.UnitID != "" {
		payload["unit_id"] = rec.UnitID
	}
	// radio 는 Values 를 편 **뒤에** 별도 키로 얹는다. 레코드에서 이미 Values 와
	// 분리되어 있으므로, "rssi" 라는 이름의 센서가 있어도 payload["rssi"] 는 그
	// 센서 값이고 무선 품질은 payload["radio"] 아래에 있어 서로 덮어쓰지 않는다.
	setChirpStackRadio(payload, rec.Radio)
	promoteDevIDWithUUID(msg, payload, agentName, opts)
	setChirpStackDevEui(msg, opts, rec.UnitID) // per-measurement 경로와 동일.
	reduceChirpStackIdentityGroups(msg, opts)  // per-measurement 경로와 동일.

	for k, v := range payload {
		msg.Payload().Set(k, v)
	}
	return msg, true
}

// ─────────────────────────────────────────────────────────────────────────────
// 무선 품질(radio) — event 경로 공유 헬퍼 (에이전트 emit_radio opt-in).
// ─────────────────────────────────────────────────────────────────────────────

// chirpStackRadioGateway 는 event 레코드 radio.gateways[] 항목의 노드측 표현이다.
//
// channel 은 수신 게이트웨이의 concentrator IF 인덱스이며 **주파수가 아니다**.
// 값 타입 uint32 이므로 proto3 가 생략한 channel:0 이 0 으로 보존된다("미상" 아님).
type chirpStackRadioGateway struct {
	GatewayID string  `json:"gateway_id"`
	RSSI      int     `json:"rssi"`
	SNR       float64 `json:"snr"`
	Channel   uint32  `json:"channel"`
}

// chirpStackRadioRecord 는 event 레코드의 radio 그룹이다 (에이전트: radioGroup).
type chirpStackRadioRecord struct {
	Gateways []chirpStackRadioGateway `json:"gateways"`
	Count    int                      `json:"count"`
}

// setChirpStackRadio 는 무선 품질 그룹을 payload 의 radio 키에 얹는다.
//
// # metadata 가 아니라 payload 인 이유 (요청 shape 로부터의 유일한 이탈)
//
// 원 설계는 $.metadata.radio 였으나 **메타데이터의 값 계약이 그것을 허용하지 않는다**:
// message.Metadata 는 값으로 string 또는 map[string]string **둘만** 저장한다
// (pkg/message/metadata.go 의 "값 계약" 주석). 게이트웨이 배열은 물론이고 숫자
// rssi/snr/channel 조차 문자열로 접지 않고서는 담을 수 없다. 문자열로 접으면
// (a) 다운스트림이 숫자를 쓰려면 매번 재파싱해야 하고, (b) 배열은 JSON 문자열을
// 통째로 문자열 필드에 넣는 이중 인코딩이 되어 계약이라 부를 수 없는 모양이 된다.
//
// payload 는 map[string]any 라 중첩/숫자를 손실 없이 담는다. 게다가 이 에이전트의
// 기존 관례와도 일치한다 — device_state 경로는 이미 rssi/snr/gateway_id 를
// payload.state 아래에 싣는다(buildChirpStackDeviceStateMessage).
//
// **원 요구의 실질은 보존된다.** "flat payload 에 넣지 말라" 는 요구의 근거는
// "rssi 라는 이름의 센서와 충돌하면 안 된다" 였고, radio 라는 단일 키 아래로
// 네임스페이스하면 그 충돌은 구조적으로 불가능하다: combined 모드에서 센서 이름은
// payload 최상위 flat 키가 되고 무선 품질은 payload["radio"] 안에만 존재한다.
// (payload 최상위에 radio 라는 이름의 센서가 있는 경우에만 충돌하며, 이는 rssi 와
// 달리 LoRaWAN 디코더가 만들어 내는 이름이 아니다.)
//
// nil(=에이전트가 emit_radio 를 끔, 또는 rxInfo 부재) 이면 **키를 만들지 않는다** —
// 빈 객체를 흘리지 않는다. 기본 설정에서 payload 가 오늘과 바이트 동일해지는 지점이다.
func setChirpStackRadio(payload map[string]any, radio *chirpStackRadioRecord) {
	if payload == nil || radio == nil || len(radio.Gateways) == 0 {
		return
	}
	// 에이전트가 gateway_id 오름차순으로 정렬해 보내므로 순서를 그대로 보존한다
	// (노드가 재정렬하면 두 표면의 순서 규약이 갈라진다).
	gws := make([]any, 0, len(radio.Gateways))
	for _, g := range radio.Gateways {
		gws = append(gws, map[string]any{
			"gateway_id": g.GatewayID,
			"rssi":       g.RSSI,
			"snr":        g.SNR,
			"channel":    g.Channel,
		})
	}
	payload["radio"] = map[string]any{
		"gateways": gws,
		"count":    radio.Count,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 주기 집계 리포트 (measurement_report → msg.Type "measurement.report").
// ─────────────────────────────────────────────────────────────────────────────

// chirpStackRecordMeasurementReport 는 주기 집계 리포트 레코드의 판별자 값이다
// (에이전트: recordKindMeasurementReport).
//
// device_state 의 report **트리거**와 이름이 겹쳐 보이지만 전혀 다른 것이다:
// device_state.report 는 마지막 스냅샷의 재방출이고, measurement.report 는 윈도
// 구간의 min/max/avg/count 집계이다. 그래서 판별자와 메시지 타입을 모두 분리했고,
// 기존 세 분기(device_state / measurements / 빈 문자열=event)는 건드리지 않는다.
const chirpStackRecordMeasurementReport = "measurement_report"

// chirpStackMeasurementReportType 은 집계 리포트 메시지의 타입이다.
const chirpStackMeasurementReportType = "measurement.report"

// chirpStackAggRecord 는 집계 통계 1건이다 (에이전트: aggView).
//
// Min/Max/Avg 가 포인터인 이유는 "비숫자 스칼라라 숫자 통계가 없음"과 "값이 0"을
// 구분하기 위해서이다. 키가 없으면 nil 이 되고, 노드는 그 키를 payload 에 만들지
// 않는다 — 0 을 지어내지 않는다.
type chirpStackAggRecord struct {
	Min   *float64 `json:"min"`
	Max   *float64 `json:"max"`
	Avg   *float64 `json:"avg"`
	Count int64    `json:"count"`
}

// toMap 은 집계 통계를 payload 표현으로 바꾼다. 부재한 숫자 통계는 키를 만들지 않는다.
func (s chirpStackAggRecord) toMap() map[string]any {
	m := make(map[string]any, 4)
	m["count"] = s.Count
	if s.Min != nil {
		m["min"] = *s.Min
	}
	if s.Max != nil {
		m["max"] = *s.Max
	}
	if s.Avg != nil {
		m["avg"] = *s.Avg
	}
	return m
}

// chirpStackRadioAggRecord 는 리포트의 게이트웨이별 무선 품질 집계 1건이다.
type chirpStackRadioAggRecord struct {
	GatewayID string              `json:"gateway_id"`
	RSSI      chirpStackAggRecord `json:"rssi"`
	SNR       chirpStackAggRecord `json:"snr"`
}

// buildChirpStackMeasurementReportMessage 는 주기 집계 리포트 레코드(JSON)를 flow
// message 로 빌드한다.
//
// # 방출되는 읽기 경로 (outputDesc)
//
// 공통:
//
//	$.type                      = "measurement.report"
//	$.timestamp                 = 윈도 종료 시각 (UnixMilli(time_ms))
//	$.payload.uplinks           = 윈도 구간 업링크 수신 건수 (int64)
//	$.payload.gateways          = 윈도 구간 관측된 서로 다른 게이트웨이 수 (int)
//	$.payload.window_start_ms   = 윈도 시작 (int64 UnixMilli)
//	$.payload.window_end_ms     = 윈도 종료 (int64 UnixMilli)
//	$.payload.radio[]           = [{gateway_id, rssi:{min,max,avg,count}, snr:{...}}]
//	                              gateway_id 오름차순 (에이전트 정렬 순서 보존)
//	$.metadata.device.{id,name,type,dev_eui}  = unit_id(devEui) 승격 결과
//	$.metadata.agent.*          = emitAgentGroup (기존 규약)
//	$.metadata.tags.*           = 태그 verbatim (기존 규약)
//	$.metadata.node_id          = opts.NodeID 시
//
// report_emit_mode = per_measurement (기본):
//
//	$.metadata.measurement      = measurement 이름
//	$.payload.count             = 해당 measurement 의 윈도 내 샘플 수 (int64)
//	$.payload.{min,max,avg}     = 숫자 통계 (float64). 비숫자 스칼라만 관측된
//	                              measurement 는 이 세 키가 **없다**.
//
// report_emit_mode = combined:
//
//	$.payload.measurements.<이름>.{count,min,max,avg}
//	(metadata.measurement 없음 — 단일 measurement 로 특정되지 않는다)
//
// # payload 모양 선택 근거
//
// per_measurement 가 통계를 payload 최상위 flat 키로 펴는 것은 event 경로의
// per_measurement 가 $.payload.value 를 최상위에 두는 것과 같은 논리이다 — 그 모드의
// 존재 이유가 "메시지 1건 = measurement 1개" 이기 때문이다.
//
// combined 은 flat 이 아니라 measurements 아래로 네임스페이스한다. event 의 combined
// 은 flat 이지만 리포트에는 uplinks/gateways/window_* 라는 고정 키가 함께 있어서,
// flat 으로 펴면 그 이름을 가진 센서가 서로 덮어쓴다. 네임스페이스가 그 충돌을
// 구조적으로 제거한다.
//
// # radio 모양이 event 와 다른 이유
//
// event 의 $.payload.radio 는 {gateways:[...], count:N} 객체이고 리포트의
// $.payload.radio 는 배열이다. 리포트는 게이트웨이 수를 이미 $.payload.gateways 로
// 싣고 있어 count 를 한 번 더 담으면 같은 사실이 두 곳에 존재하게 된다. 두 표면은
// 애초에 서로 다른 $.type 이므로 같은 스키마로 소비되지 않는다.
func buildChirpStackMeasurementReportMessage(data []byte, nodeID string, a agent.Agent, agentName string, opts MetadataEmitOptions) (message.Message, bool) {
	var rec struct {
		Measurement   string                         `json:"measurement"`
		Stats         *chirpStackAggRecord           `json:"stats"`
		Measurements  map[string]chirpStackAggRecord `json:"measurements"`
		Radio         []chirpStackRadioAggRecord     `json:"radio"`
		Uplinks       int64                          `json:"uplinks"`
		Gateways      int                            `json:"gateways"`
		UnitID        string                         `json:"unit_id"`
		TimeMs        int64                          `json:"time_ms"`
		WindowStartMs int64                          `json:"window_start_ms"`
		WindowEndMs   int64                          `json:"window_end_ms"`
		Tags          map[string]string              `json:"tags"`
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, false
	}

	msg := message.New()
	msg.SetType(chirpStackMeasurementReportType)
	if rec.TimeMs > 0 {
		msg.SetTimestamp(time.UnixMilli(rec.TimeMs))
	}
	// per_measurement 모드에서만 measurement 가 특정된다 (event 경로와 동일 규약).
	if rec.Measurement != "" {
		msg.Metadata().Set("measurement", rec.Measurement)
	}
	// tags verbatim pass-through — event 경로와 동일.
	if len(rec.Tags) > 0 {
		msg.Metadata().SetGroup("tags", rec.Tags)
	}
	if opts.NodeID {
		msg.Metadata().Set("node_id", nodeID)
	}
	emitAgentGroup(msg, a, opts)

	payload := make(map[string]any, 8)
	payload["uplinks"] = rec.Uplinks
	payload["gateways"] = rec.Gateways
	payload["window_start_ms"] = rec.WindowStartMs
	payload["window_end_ms"] = rec.WindowEndMs

	if len(rec.Measurements) > 0 {
		values := make(map[string]any, len(rec.Measurements))
		for k, s := range rec.Measurements {
			values[k] = s.toMap()
		}
		payload["measurements"] = values
	} else if rec.Stats != nil {
		for k, v := range rec.Stats.toMap() {
			payload[k] = v
		}
	}

	if len(rec.Radio) > 0 {
		radio := make([]any, 0, len(rec.Radio))
		for _, g := range rec.Radio {
			radio = append(radio, map[string]any{
				"gateway_id": g.GatewayID,
				"rssi":       g.RSSI.toMap(),
				"snr":        g.SNR.toMap(),
			})
		}
		payload["radio"] = radio
	}

	if rec.UnitID != "" {
		payload["unit_id"] = rec.UnitID
	}
	promoteDevIDWithUUID(msg, payload, agentName, opts)
	setChirpStackDevEui(msg, opts, rec.UnitID) // event 경로와 동일.
	reduceChirpStackIdentityGroups(msg, opts)  // event 경로와 동일.

	for k, v := range payload {
		msg.Payload().Set(k, v)
	}
	return msg, true
}

// buildChirpStackDeviceStateMessage 는 comm-state fold 레코드(JSON)를 flow message 로
// 빌드한다 (REQ-FROZEN-03). 별도 device_connection 타입 없이 device_state 스트림으로
// 접힌다.
//
// 계약:
//   - msg.Type = "device_state.<trigger>" (applyDeviceStateMessageType 승격).
//   - payload.state = {online, rssi, snr, gateway_id, last_seen_ms}.
//   - payload.last_seen_ms = int64 UnixMilli (top-level).
//   - unit_id(=devEui) → device 그룹(UUID/name/type) 승격 (event 경로와 동일).
func buildChirpStackDeviceStateMessage(data []byte, nodeID string, a agent.Agent, agentName string, opts MetadataEmitOptions) (message.Message, bool) {
	var rec struct {
		Trigger    string `json:"trigger"`
		UnitID     string `json:"unit_id"`
		TimeMs     int64  `json:"time_ms"`
		LastSeenMs int64  `json:"last_seen_ms"`
		State      struct {
			Online     bool    `json:"online"`
			RSSI       int     `json:"rssi"`
			SNR        float64 `json:"snr"`
			GatewayID  string  `json:"gateway_id"`
			LastSeenMs int64   `json:"last_seen_ms"`
		} `json:"state"`
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, false
	}

	msg := message.New()
	if rec.TimeMs > 0 {
		msg.SetTimestamp(time.UnixMilli(rec.TimeMs))
	}

	payload := map[string]any{
		"trigger":      rec.Trigger, // applyDeviceStateMessageType 가 소비 → msg.Type 승격.
		"unit_id":      rec.UnitID,  // promoteDevIDWithUUID 가 device 그룹으로 승격.
		"last_seen_ms": rec.LastSeenMs,
		"state": map[string]any{
			"online":       rec.State.Online,
			"rssi":         rec.State.RSSI,
			"snr":          rec.State.SNR,
			"gateway_id":   rec.State.GatewayID,
			"last_seen_ms": rec.State.LastSeenMs,
		},
	}

	// trigger → msg.Type("device_state.<trigger>"); payload 에서 trigger 제거.
	applyDeviceStateMessageType(msg, payload, commTriggerReport)
	if opts.NodeID {
		msg.Metadata().Set("node_id", nodeID)
	}
	emitAgentGroup(msg, a, opts)
	promoteDevIDWithUUID(msg, payload, agentName, opts)
	setChirpStackDevEui(msg, opts, rec.UnitID) // event 경로와 동일.
	reduceChirpStackIdentityGroups(msg, opts)  // event 경로와 동일.

	for k, v := range payload {
		msg.Payload().Set(k, v)
	}
	return msg, true
}

// commTriggerReport 는 trigger 미지정 시 device_state 메시지의 기본 sub-type 이다.
const commTriggerReport = "report"

// Process 는 SourceNode 이므로 입력 메시지를 그대로 통과시킨다.
func (n *ChirpStackInNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	return []message.Message{msg}, nil
}

// SourceCh 는 수신된 메시지를 전달하는 채널을 반환한다.
func (n *ChirpStackInNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// Shutdown 은 노드를 종료한다.
func (n *ChirpStackInNode) Shutdown(_ context.Context) error {
	n.stopCurrentLoop()
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// stopCurrentLoop 은 현재 세대의 receiveLoop 에 종료를 신호한다.
// stopCh / stopOnce 는 Reinit 이 교체하므로 반드시 n.mu 하에서 다룬다.
func (n *ChirpStackInNode) stopCurrentLoop() {
	n.mu.Lock()
	n.stopOnce.Do(func() {
		close(n.stopCh)
	})
	n.mu.Unlock()
}

// Reinit 은 에이전트 재시작(새 인스턴스로 교체) 후 노드를 재초기화한다
// (MQTTSubNode.Reinit 관용구 미러링).
//
// 이 노드는 수신 goroutine 을 소유하므로 순서가 중요하다:
// 기존 receiveLoop 종료 → agent/transport/receiver/agentName 재해석 →
// stopCh / stopOnce 재생성 → 새 receiveLoop 기동.
// stopCh 을 재생성하지 않으면 새 루프가 첫 select 에서 즉시 종료된다(구 stopCh 은
// 이미 close 된 상태).
//
// 본 메서드가 없으면 Engine.ReinitNodesForAgent 가 이 노드를 조용히 건너뛰어,
// 에이전트 재시작 후 노드가 구 인스턴스의 닫힌 채널만 바라보며 영구 무음이 된다.
func (n *ChirpStackInNode) Reinit(ctx context.Context) error {
	// 1. 기존 수신 루프 종료 (구 루프는 자신이 캡처한 stopCh 을 관찰한다).
	n.stopCurrentLoop()

	// 2. agent / transport / receiver / agentName 재해석.
	//    실패 시 새 루프를 띄우지 않고 에러를 전파한다(엔진이 로그로 기록).
	if err := n.initAgent(ctx); err != nil {
		return err
	}

	// 3. 새 세대의 stopCh / stopOnce 로 교체.
	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.stopOnce = sync.Once{}
	n.mu.Unlock()

	// 4. 새 receiveLoop 기동.
	go n.receiveLoop()
	return nil
}
