package node

import (
	"context"
	"fmt"
	"strings"
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
// 스코프:
//   - payload_fields 는 최상위(top-level) 키만 대상으로 한다 (dot path 미지원).
//   - metadata_fields 는 메타데이터 nested group 을 인식한다. 각 엔트리는 다음 형식이다:
//   - `key` (점 없음): top-level string 키 OR group 키 전체.
//     예) `device` → device 그룹 전체 보존, `node_id` → top-level string 보존.
//   - `group.field` (점 1개): group 안의 단일 필드.
//     예) `device.id` → device 그룹에서 id 필드만 남김(나머지 device 필드 제거).
//   - `a.b.c` (점 2개 이상): group 은 정확히 한 단계 깊이이므로 무효 — 무시된다(에러 관대).
//     결합 규칙: 같은 그룹에 전체 키(`device`)와 필드 키(`device.id`)가 함께 있으면
//     전체 키가 우선한다(그룹 전체 보존). `device.id` 와 `device.name` 만 있으면
//     device 그룹은 {id, name} 으로 축소된다.
//
// message-level 에서 id 와 timestamp 는 구조적(structural) 필드이므로 항상 보존되며
// 드랍할 수 없다. 실질적으로 message_filter 로 비울 수 있는 것은 type 뿐이다.
//
// 동작 변경 주의: metadata_filter 가 켜진 상태에서, 과거에는 메타데이터 group 값이
// 필터에 보이지 않아 항상 통과했다. 이제 group 도 string 키와 동일하게 화이트리스트
// 대상이 된다 — metadata_fields 에 나열되지 않은 group 은 제거된다.
type SelectFieldNode struct {
	*BaseNode

	onMissing onMissingMode

	// dropToPort 가 true 면 drop 결정 시 메시지를 폐기하지 않고 "drop" 포트로 emit 한다.
	// on_missing == drop 일 때만 의미가 있다 (ignore/fill 에서는 드랍이 없어 무시됨).
	dropToPort bool

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

	// drop_to_port: bool / "true"|"false" 문자열 모두 허용 (configBool, Web UI 호환). 기본 false.
	dropToPort, _ := configBool(config, "drop_to_port")

	payloadFields := parseStringMap(config["payload_fields"])
	metadataFields := parseStringMap(config["metadata_fields"])
	messageFields := parseStringSet(config["message_fields"])

	// 원자적 설정 적용
	n.mu.Lock()
	n.onMissing = onMissing
	n.dropToPort = dropToPort
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
//     누락되면 메시지를 드랍한다. drop_to_port 가 false 면 (nil, nil) 로 폐기하고,
//     true 면 원본(미변형) 메시지를 "drop" 포트로 emit 한다 (emitDrop 참고).
func (n *SelectFieldNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	onMissing := n.onMissing
	dropToPort := n.dropToPort
	payloadFilter := n.payloadFilter
	payloadFields := n.payloadFields
	metadataFilter := n.metadataFilter
	metadataFields := n.metadataFields
	messageFilter := n.messageFilter
	messageFields := n.messageFields
	n.mu.RUnlock()

	// --- 드랍 판정 (drop 정책) ---
	// 새 페이로드/메타데이터를 적용하기 전에 누락 여부를 먼저 확인하여,
	// drop 이면 부분 변형 없이 즉시 처리한다. drop_to_port 가 켜져 있으면
	// 원본(미변형) 메시지를 "drop" 포트로 emit 하고, 아니면 폐기한다.
	if onMissing == onMissingDrop {
		if payloadFilter && hasMissing(msg.Payload().ToMap(), payloadFields) {
			return n.emitDrop(msg, dropToPort)
		}
		if metadataFilter {
			topLevel, perGroup := parseMetadataFields(metadataFields)
			if metadataHasMissing(msg.Metadata().Raw(), topLevel, perGroup) {
				return n.emitDrop(msg, dropToPort)
			}
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
		topLevel, perGroup := parseMetadataFields(metadataFields)
		rebuildMetadata(msg.Metadata(), topLevel, perGroup, onMissing)
	}

	// --- message-level (type) 재구성 ---
	// id 와 timestamp 는 항상 보존. type 만 message_filter 로 비울 수 있다.
	if messageFilter && !messageFields["type"] {
		msg.SetType("")
	}

	return []message.Message{msg}, nil
}

// emitDrop 은 drop 결정 시의 출력을 결정한다.
//   - dropToPort 가 true 면 원본(미변형) 메시지의 Clone 에 _target_port="drop" 를
//     설정하여 "drop" 포트로 emit 한다 (filter.go 의 reject 포트 패턴 미러링).
//   - false 면 (nil, nil) 을 반환하여 메시지를 폐기한다 (기존 동작).
//
// 호출 시점이 페이로드/메타데이터 변형 이전이므로, Clone 된 메시지는 원본 그대로이다.
func (n *SelectFieldNode) emitDrop(msg message.Message, dropToPort bool) ([]message.Message, error) {
	if !dropToPort {
		return nil, nil
	}
	out := msg.Clone()
	out.Metadata().Set("_target_port", "drop")
	return []message.Message{out}, nil
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

// parseMetadataFields 는 metadata_fields 화이트리스트를 두 구조로 분해한다.
//   - topLevel:  점 없는 키 → fill 기본값. top-level string 키 또는 group 전체 키.
//   - perGroup:  group 키 → (field 이름 → fill 기본값). `group.field` 형식.
//
// `a.b.c` 처럼 점이 2개 이상인 엔트리는 무효(group 은 한 단계 깊이)이므로 무시한다.
func parseMetadataFields(fields map[string]string) (topLevel map[string]string, perGroup map[string]map[string]string) {
	topLevel = make(map[string]string)
	perGroup = make(map[string]map[string]string)
	for key, fillVal := range fields {
		group, field, hasDot := strings.Cut(key, ".")
		if !hasDot {
			// 점 없음 → top-level 키 (string 또는 group 전체).
			topLevel[key] = fillVal
			continue
		}
		if group == "" || field == "" || strings.Contains(field, ".") {
			// 빈 그룹/필드 또는 한 단계 초과(a.b.c) → 무효, 무시.
			continue
		}
		fset := perGroup[group]
		if fset == nil {
			fset = make(map[string]string)
			perGroup[group] = fset
		}
		fset[field] = fillVal
	}
	return topLevel, perGroup
}

// rebuildMetadata 는 화이트리스트(topLevel/perGroup)에 따라 메타데이터를 재구성한다.
//
// 현재 메타데이터를 Raw() 로 순회하며 각 top-level 키를 다음과 같이 처리한다:
//   - string 값: topLevel 에 선택되었으면 유지, 아니면 제거.
//   - group 값:
//   - topLevel 에 전체 선택(whole)되었으면 그룹 전체 보존.
//   - perGroup 에 필드 선택이 있으면 선택 필드만으로 그룹 재구성
//     (subset 이 비면 SetGroup 의 no-op delete 로 그룹 제거).
//   - 둘 다 아니면 그룹 제거.
//
// on_missing == fill 일 때 누락된 top-level string 키와 group 필드를 기본값으로 채운다.
// (group 자체가 없으면 group 을 생성한다.)
func rebuildMetadata(md message.Metadata, topLevel map[string]string, perGroup map[string]map[string]string, mode onMissingMode) {
	raw := md.Raw()

	for k, v := range raw {
		switch val := v.(type) {
		case map[string]string:
			// group 값.
			if _, whole := topLevel[k]; whole {
				// 전체 선택 → 그룹 보존(전체 키 우선; perGroup 무시).
				continue
			}
			fieldSel, ok := perGroup[k]
			if !ok {
				// 선택되지 않은 그룹 → 제거.
				md.Remove(k)
				continue
			}
			// 선택 필드만으로 그룹 재구성.
			subset := make(map[string]string, len(fieldSel))
			for field, fillVal := range fieldSel {
				if fv, exists := val[field]; exists {
					subset[field] = fv
				} else if mode == onMissingFill {
					subset[field] = fillVal
				}
				// ignore/drop: 건너뜀.
			}
			// subset 이 비면 SetGroup 이 키를 제거한다(no-op delete).
			md.SetGroup(k, subset)
		default:
			// string 값.
			if _, sel := topLevel[k]; !sel {
				md.Remove(k)
			}
			// 선택된 string 은 그대로 둔다.
		}
	}

	if mode != onMissingFill {
		return
	}

	// fill: 누락된 top-level string 키 채움.
	// (부재 키는 string/group 구분이 불가능하므로 string fill 로 처리한다 —
	//  기존 top-level string fill 동작과 동일.)
	for k, fillVal := range topLevel {
		if _, exists := raw[k]; !exists {
			md.Set(k, fillVal)
		}
	}

	// fill: 그룹 자체가 부재인 perGroup 선택은 그룹을 생성하여 필드를 채운다.
	// (그룹이 존재하지만 필드가 누락된 경우는 위 루프에서 이미 채워졌다.)
	for group, fieldSel := range perGroup {
		if _, whole := topLevel[group]; whole {
			continue // 전체 키 우선.
		}
		if _, exists := raw[group]; exists {
			continue // 위 루프에서 처리됨.
		}
		subset := make(map[string]string, len(fieldSel))
		for field, fillVal := range fieldSel {
			subset[field] = fillVal
		}
		md.SetGroup(group, subset)
	}
}

// metadataHasMissing 은 화이트리스트(topLevel/perGroup) 중 raw 메타데이터에
// 존재하지 않는 항목이 하나라도 있으면 true 를 반환한다 (drop 정책 판정용).
//
// 누락 판정:
//   - topLevel 키: raw 에 해당 키(string 또는 group)가 없으면 누락.
//   - perGroup `group.field`: group 이 없거나, group 에 해당 field 가 없으면 누락.
//     (단, 같은 group 이 topLevel 에 전체 선택되어 있으면 perGroup 판정은 건너뛴다.)
func metadataHasMissing(raw map[string]any, topLevel map[string]string, perGroup map[string]map[string]string) bool {
	for key := range topLevel {
		if _, ok := raw[key]; !ok {
			return true
		}
	}
	for group, fields := range perGroup {
		if _, whole := topLevel[group]; whole {
			continue // 전체 키 우선 — perGroup 판정 생략.
		}
		g, ok := raw[group].(map[string]string)
		if !ok {
			// 그룹 부재 또는 group 이 아님(string) → 누락.
			return true
		}
		for field := range fields {
			if _, ok := g[field]; !ok {
				return true
			}
		}
	}
	return false
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
