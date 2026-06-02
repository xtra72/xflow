package node

import (
	"context"
	"fmt"
	"sync"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// onMissingMode 는 화이트리스트 필드가 메시지에 누락되었을 때의 처리 정책이다.
type onMissingMode string

const (
	// onMissingIgnore 는 누락된 필드를 건너뛴다 (출력에 포함하지 않음). 기본값.
	onMissingIgnore onMissingMode = "ignore"
	// onMissingDrop 은 활성화된 필터 그룹의 화이트리스트 필드가 하나라도 누락되면
	// 메시지 전체를 드랍한다 (아무것도 emit 하지 않음).
	onMissingDrop onMissingMode = "drop"
	// onMissingFill 은 누락된 필드를 설정된 fill 기본값으로 채워 포함한다.
	onMissingFill onMissingMode = "fill"
)

// SelectFieldNode 는 메시지에서 지정한 필드만 남기는(화이트리스트) 처리 노드이다.
// payload / metadata / message 세 그룹에 대해 각각 필터 on/off 토글을 가지며,
// 누락된 필드는 on_missing 정책(ignore/drop/fill)에 따라 처리한다.
//
// 스코프: payload_fields 와 metadata_fields 는 최상위(top-level) 키만 대상으로 한다.
// (dot path 미지원 — top-level 키가 기준선.)
// message-level 에서 id 와 timestamp 는 구조적(structural) 필드이므로 항상 보존되며
// 드랍할 수 없다. 실질적으로 message_filter 로 비울 수 있는 것은 type 뿐이다.
type SelectFieldNode struct {
	*BaseNode

	onMissing onMissingMode

	payloadFilter bool
	payloadFields map[string]string // 유지할 payload 키 → fill 기본값 (fill 모드에서만 사용)

	metadataFilter bool
	metadataFields map[string]string // 유지할 metadata 키 → fill 기본값 (fill 모드에서만 사용)

	messageFilter bool
	messageFields map[string]bool // 유지할 message-level 필드 집합 ({id,type,timestamp} 부분집합)

	mu sync.RWMutex // 설정 보호 뮤텍스
}

// 인터페이스 컴파일 체크
var _ Node = (*SelectFieldNode)(nil)

// NewSelectFieldNode 는 새로운 SelectFieldNode를 생성하는 팩토리 함수이다.
func NewSelectFieldNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &SelectFieldNode{
		BaseNode:  base,
		onMissing: onMissingIgnore, // 기본값
	}
	return n, nil
}

// Init 은 SelectFieldNode를 초기화한다.
func (n *SelectFieldNode) Init(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Shutdown 은 SelectFieldNode를 종료한다.
func (n *SelectFieldNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 SelectFieldNode의 설정을 적용한다.
// 모든 키 파싱은 관대하게(lenient) 처리한다:
//   - *_fields 맵: map[string]any / map[string]string 모두 허용.
//   - message_fields: []any / []string 모두 허용.
//
// 알 수 없는 on_missing 값은 기본값(ignore)으로 폴백한다.
func (n *SelectFieldNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	// on_missing 정책 (기본 ignore)
	onMissing := onMissingIgnore
	if v, ok := config["on_missing"]; ok {
		if s, ok := v.(string); ok {
			switch onMissingMode(s) {
			case onMissingDrop:
				onMissing = onMissingDrop
			case onMissingFill:
				onMissing = onMissingFill
			default:
				onMissing = onMissingIgnore
			}
		}
	}

	payloadFilter, _ := config["payload_filter"].(bool)
	metadataFilter, _ := config["metadata_filter"].(bool)
	messageFilter, _ := config["message_filter"].(bool)

	payloadFields := parseStringMap(config["payload_fields"])
	metadataFields := parseStringMap(config["metadata_fields"])
	messageFields := parseStringSet(config["message_fields"])

	// 원자적 설정 적용
	n.mu.Lock()
	n.onMissing = onMissing
	n.payloadFilter = payloadFilter
	n.payloadFields = payloadFields
	n.metadataFilter = metadataFilter
	n.metadataFields = metadataFields
	n.messageFilter = messageFilter
	n.messageFields = messageFields
	n.mu.Unlock()

	return nil
}

// Process 는 화이트리스트 정책에 따라 출력 메시지를 구성한다.
//
// 동작:
//   - id 와 timestamp 는 구조적 필드이므로 항상 원본을 보존한다. message 패키지가
//     ID 를 명시적으로 설정하는 옵션을 제공하지 않으므로, 입력 메시지를 in-place 로
//     재구성하여 id/timestamp 를 자연스럽게 보존한다 (mapping 노드와 동일 패턴).
//   - payload: payload_filter 가 true 면 payload_fields 에 명시된 키만 유지하고,
//     누락 시 on_missing 정책을 따른다. false 면 원본 payload 를 그대로 둔다.
//   - metadata: 동일 로직을 metadata 키에 적용한다.
//   - message-level: message_filter 가 true 이고 type 이 message_fields 에 없으면
//     출력 type 을 ""(빈 문자열)로 비운다. 그 외에는 원본 type 을 유지한다.
//   - on_missing == drop 이고 활성화된 필터 그룹의 화이트리스트 필드가 하나라도
//     누락되면 (nil, nil) 을 반환하여 메시지를 드랍한다 (emit 없음).
func (n *SelectFieldNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	onMissing := n.onMissing
	payloadFilter := n.payloadFilter
	payloadFields := n.payloadFields
	metadataFilter := n.metadataFilter
	metadataFields := n.metadataFields
	messageFilter := n.messageFilter
	messageFields := n.messageFields
	n.mu.RUnlock()

	// --- 드랍 판정 (drop 정책) ---
	// 새 페이로드/메타데이터를 적용하기 전에 누락 여부를 먼저 확인하여,
	// drop 이면 부분 변형 없이 즉시 (nil, nil) 을 반환한다.
	if onMissing == onMissingDrop {
		if payloadFilter && hasMissing(msg.Payload().ToMap(), payloadFields) {
			return nil, nil
		}
		if metadataFilter && hasMissing(metadataToAnyMap(msg), metadataFields) {
			return nil, nil
		}
	}

	// --- payload 재구성 ---
	if payloadFilter {
		srcPayload := msg.Payload().ToMap()
		filtered := selectKeys(srcPayload, payloadFields, onMissing)
		p := msg.Payload()
		// 화이트리스트에 없는 기존 키 제거.
		for _, k := range p.Keys() {
			if _, keep := filtered[k]; !keep {
				p.Delete(k)
			}
		}
		// 유지/채움 키 적용.
		for k, v := range filtered {
			p.Set(k, v)
		}
	}

	// --- metadata 재구성 ---
	if metadataFilter {
		srcMeta := metadataToAnyMap(msg)
		filtered := selectKeys(srcMeta, metadataFields, onMissing)
		md := msg.Metadata()
		// 화이트리스트에 없는 기존 키 제거.
		for k := range srcMeta {
			if _, keep := filtered[k]; !keep {
				md.Remove(k)
			}
		}
		// 유지/채움 키 적용 (metadata 값은 문자열).
		for k, v := range filtered {
			md.Set(k, anyToString(v))
		}
	}

	// --- message-level (type) 재구성 ---
	// id 와 timestamp 는 항상 보존. type 만 message_filter 로 비울 수 있다.
	if messageFilter && !messageFields["type"] {
		msg.SetType("")
	}

	return []message.Message{msg}, nil
}

// hasMissing 은 fields 화이트리스트 중 src 에 존재하지 않는 키가 하나라도 있으면 true 를 반환한다.
func hasMissing(src map[string]any, fields map[string]string) bool {
	for key := range fields {
		if _, ok := src[key]; !ok {
			return true
		}
	}
	return false
}

// metadataToAnyMap 은 메시지 메타데이터를 map[string]any 로 변환한다.
func metadataToAnyMap(msg message.Message) map[string]any {
	all := msg.Metadata().All()
	out := make(map[string]any, len(all))
	for k, v := range all {
		out[k] = v
	}
	return out
}

// anyToString 은 metadata 값(any)을 문자열로 정규화한다.
func anyToString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// selectKeys 는 src 맵에서 fields 화이트리스트에 해당하는 키만 추출한 새 맵을 반환한다.
// 누락된 키는 on_missing 정책에 따라 처리한다:
//   - ignore: 건너뜀.
//   - fill:   설정된 fill 기본값으로 채움.
//   - drop:   드랍 판정은 호출 전에 hasMissing 으로 처리하므로, 여기서는 ignore 처럼
//     건너뛴다 (실제로는 drop 일 때 이 경로가 도달하지 않음).
func selectKeys(src map[string]any, fields map[string]string, mode onMissingMode) map[string]any {
	out := make(map[string]any, len(fields))
	for key, fillVal := range fields {
		if v, ok := src[key]; ok {
			// 존재하는 키 → 그대로 유지.
			out[key] = v
			continue
		}
		// 누락된 키 → 정책 적용.
		if mode == onMissingFill {
			out[key] = fillVal
		}
		// ignore/drop: 건너뜀.
	}
	return out
}

// parseStringMap 은 *_fields 설정을 map[string]string 으로 관대하게 변환한다.
// map[string]string / map[string]any 모두 허용하며, 값은 문자열로 정규화한다
// (fill 기본값으로 사용). nil 이거나 다른 타입이면 빈 맵을 반환한다.
func parseStringMap(v any) map[string]string {
	result := make(map[string]string)
	switch m := v.(type) {
	case map[string]string:
		for k, val := range m {
			result[k] = val
		}
	case map[string]any:
		for k, val := range m {
			if s, ok := val.(string); ok {
				result[k] = s
			} else if val == nil {
				result[k] = ""
			} else {
				result[k] = fmt.Sprintf("%v", val)
			}
		}
	}
	return result
}

// parseStringSet 은 message_fields 설정을 문자열 집합으로 관대하게 변환한다.
// []string / []any 모두 허용한다. nil 이거나 다른 타입이면 빈 집합을 반환한다.
func parseStringSet(v any) map[string]bool {
	result := make(map[string]bool)
	switch arr := v.(type) {
	case []string:
		for _, s := range arr {
			result[s] = true
		}
	case []any:
		for _, item := range arr {
			if s, ok := item.(string); ok {
				result[s] = true
			}
		}
	}
	return result
}
