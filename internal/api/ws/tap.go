package ws

import (
	"sync"

	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/pkg/message"
)

// 컴파일 타임 인터페이스 만족 보장.
var (
	_ engine.OutputObserver = (*TapObserver)(nil)
	_ messageBroadcaster    = (*Hub)(nil)
)

// TypeNodeOutput 는 노드 출력 tap 메시지 타입이다.
// 와이어 연결 없이 임의의 노드 출력 메시지를 에디터로 스트리밍하는 데 사용된다.
const TypeNodeOutput = "node.output"

// NodeOutputPayload 는 node.output WebSocket 메시지의 페이로드이다.
//
// Message 는 브리지 봉투(messageToJSON) 와 동일한 whole-message 맵 형태로,
// DebugPanel 이 기존 debug.message 와 일관되게 렌더링할 수 있도록 한다.
// 포함 필드: id, type, timestamp(epoch ms int64),
// payload(map), metadata(map, nested group 보존을 위해 Raw()).
type NodeOutputPayload struct {
	FlowID  string         `json:"flow_id"`
	NodeID  string         `json:"node_id"`
	Port    string         `json:"port"`
	Message map[string]any `json:"message"`
}

// messageBroadcaster 는 TapObserver 가 의존하는 최소 브로드캐스트 인터페이스이다.
// *Hub 가 이를 만족한다. 테스트에서는 카운팅 fake 로 대체할 수 있다.
type messageBroadcaster interface {
	BroadcastMessage(msgType string, payload any) error
}

// tapKey 는 (flowID, nodeID) 조합의 맵 키이다.
type tapKey struct {
	flowID string
	nodeID string
}

// TapRegistry 는 현재 tap(관측) 중인 (flowID, nodeID) 집합을 관리하는
// 동시성 안전 레지스트리이다. 런타임 전용이며 재시작 시 초기화된다
// (관측은 일시적/임시이므로 영속화하지 않는다).
type TapRegistry struct {
	mu     sync.RWMutex
	tapped map[tapKey]struct{}
}

// NewTapRegistry 는 빈 TapRegistry 를 생성한다.
func NewTapRegistry() *TapRegistry {
	return &TapRegistry{
		tapped: make(map[tapKey]struct{}),
	}
}

// SetTap 은 지정된 노드의 tap 상태를 설정한다.
// enabled=true 면 관측 대상에 추가하고, false 면 제거한다.
func (r *TapRegistry) SetTap(flowID, nodeID string, enabled bool) {
	key := tapKey{flowID: flowID, nodeID: nodeID}
	r.mu.Lock()
	defer r.mu.Unlock()
	if enabled {
		r.tapped[key] = struct{}{}
	} else {
		delete(r.tapped, key)
	}
}

// IsTapped 는 지정된 노드가 현재 tap 중인지 반환한다.
// 핫 패스에서 호출되므로 RLock + 맵 조회만 수행한다.
func (r *TapRegistry) IsTapped(flowID, nodeID string) bool {
	key := tapKey{flowID: flowID, nodeID: nodeID}
	r.mu.RLock()
	_, ok := r.tapped[key]
	r.mu.RUnlock()
	return ok
}

// TappedNodes 는 지정된 플로우에서 현재 tap 중인 노드 ID 목록을 반환한다.
// 에디터가 새로고침 시 tap 상태를 복원하는 데 사용된다. 순서는 보장되지 않는다.
func (r *TapRegistry) TappedNodes(flowID string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []string
	for key := range r.tapped {
		if key.flowID == flowID {
			out = append(out, key.nodeID)
		}
	}
	return out
}

// TapObserver 는 engine.OutputObserver 를 구현하여 tap 된 노드의 출력 메시지를
// WebSocket 으로 브로드캐스트한다. tap 되지 않은 노드는 fast-path 로 즉시 반환하며
// 직렬화/할당을 발생시키지 않는다.
type TapObserver struct {
	registry    *TapRegistry
	broadcaster messageBroadcaster
	// expander 는 외부 경계 egress 시 agent / device 그룹을 type/name 으로
	// 역-수화하는 opt-in 훅이다(message-slim-metadata). nil(기본)이면 슬림화만
	// 수행한다 — 즉 wire 에는 agent={id} / device={id} 만 나간다. 설정 시 expand=true
	// 동작(레지스트리에서 type/name 재수화)을 한다.
	expander *message.GroupExpander
}

// NewTapObserver 는 주어진 레지스트리와 브로드캐스터로 TapObserver 를 생성한다.
// 기본 egress 정책은 슬림(agent/device 그룹 id-only).
func NewTapObserver(registry *TapRegistry, broadcaster messageBroadcaster) *TapObserver {
	return &TapObserver{
		registry:    registry,
		broadcaster: broadcaster,
	}
}

// WithExpander 는 egress 시 그룹 역-수화를 활성화한다(expand opt-in). exp 가 nil 이면
// 슬림 기본 동작으로 되돌린다. 같은 인스턴스를 반환하여 체이닝을 허용한다.
func (o *TapObserver) WithExpander(exp *message.GroupExpander) *TapObserver {
	if o == nil {
		return o
	}
	o.expander = exp
	return o
}

// OnNodeOutput 은 노드가 출력 포트로 메시지를 방출할 때 엔진에 의해 호출된다.
//
// Fast-path: tap 되지 않은 노드면 직렬화/할당 없이 즉시 반환한다.
// tap 된 경우에만 node.output 페이로드를 구성하여 논블로킹 브로드캐스트한다.
// 옵저버 내부 패닉은 복구하여 엔진 고루틴을 보호한다 (tap 이 엔진을 죽이지 않음).
func (o *TapObserver) OnNodeOutput(flowID, nodeID, port string, msg message.Message) {
	// Fast-path: 관측 대상이 아니면 즉시 반환 (직렬화/할당 없음).
	if o == nil || o.registry == nil || !o.registry.IsTapped(flowID, nodeID) {
		return
	}
	if msg == nil || o.broadcaster == nil {
		return
	}

	// tap 콜백 내부에서의 패닉이 엔진 고루틴을 죽이지 않도록 격리한다.
	defer func() {
		_ = recover()
	}()

	payload := NodeOutputPayload{
		FlowID:  flowID,
		NodeID:  nodeID,
		Port:    port,
		Message: buildNodeOutputMessage(msg, o.expander),
	}

	// 논블로킹 브로드캐스트: Hub.Broadcast 는 버퍼링되어 느린 소비자에 대해
	// 드롭한다 (debug.message 와 동일). 반환 에러는 무시한다.
	_ = o.broadcaster.BroadcastMessage(TypeNodeOutput, payload)
}

// buildNodeOutputMessage 는 메시지를 브리지 봉투(messageToJSON)와 동일한
// whole-message 맵으로 구성한다. timestamp 는 epoch ms (int64) 단일 필드로
// 노출한다(중복되는 human-readable time 필드는 폐기 — timestamp 로 통일).
//
// metadata (message-slim-metadata): 외부 경계 egress 이므로 agent / device 그룹을
// 기본 슬림화(id-only)한다. exp 가 주어지면(expand opt-in) 레지스트리에서 type/name 을
// 역-수화한다. 내부 메시지 흐름은 영향받지 않으며 여기서는 Raw() 복사본 위에서만 동작한다.
func buildNodeOutputMessage(msg message.Message, exp *message.GroupExpander) map[string]any {
	return map[string]any{
		"id":        msg.ID(),
		"type":      msg.Type(),
		"timestamp": msg.Timestamp().UnixMilli(),
		"payload":   msg.Payload().ToMap(),
		"metadata":  EgressMetadata(msg, exp),
	}
}

// EgressMetadata 는 외부 클라이언트로 나가는 메시지 메타데이터를 정책에 따라 변환한다.
//
//   - exp == nil (기본): agent / device 그룹을 id-only 로 슬림화한다.
//   - exp != nil (expand opt-in): 그룹을 레지스트리 조회로 type/name 까지 역-수화한다.
//
// 두 경우 모두 msg.Metadata().Raw() 의 복사본 위에서 동작하므로 원본 메시지는
// 변형되지 않는다(내부 흐름 무영향).
func EgressMetadata(msg message.Message, exp *message.GroupExpander) map[string]any {
	raw := msg.Metadata().Raw()
	if exp != nil {
		return message.ExpandGroupsByID(raw, *exp)
	}
	return message.SlimGroupsToID(raw)
}
