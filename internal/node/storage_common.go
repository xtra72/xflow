package node

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/xtra/xflow/pkg/message"
)

// 이 파일은 storage-write 계열 노드(및 store-read 등 인접 노드)가 공유하는
// 백엔드 중립 헬퍼를 모아 둔다. 과거에는 store_write.go / influxdb_write.go 에
// 각각 흩어져 있었으나, 두 노드를 storage-write 하나로 통합하면서 백엔드에
// 의존하지 않는 부분만 이곳으로 옮겼다.
//
// 포함 범위:
//   - 값 경로 해석: resolveKeyTemplate / resolveTemplateExpr / lookupPayloadPath
//   - 값 문자열화: valueToString / scalarFieldValue
//   - 이름 검증 패턴: tagKeyPattern / fieldPattern

// tagKeyPattern 은 태그 key 로 허용되는 문자 패턴이다 (system 계층과 동일).
var tagKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// fieldPattern 은 field 으로 허용되는 문자 패턴이다 (system 계층의 fieldPattern 과 동일).
// 값 이름(name)이 리터럴일 때 Configure 시점에, `$.` 경로일 때 해석 결과를 Process 시점에 검증한다.
var fieldPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// resolveKeyTemplate 는 {expr} 플레이스홀더를 메시지 값으로 치환한다.
//
// 지원 문법:
//   - {field}                — payload 의 field (legacy, backward compatible)
//   - {$.payload.field}      — payload 의 field (명시적)
//   - {$.payload.a.b.c}      — payload 의 중첩 경로 (map[string]any traversal)
//   - {$.metadata.field}     — metadata 의 field
//
// 예: "{$.metadata.dev_id}:{$.payload.state.mode}" +
//
//	metadata{dev_id:"idu-1"} + payload{state:{mode:1}}
//	→ "idu-1:1"
func resolveKeyTemplate(template string, msg message.Message) (string, error) {
	result := template
	for {
		start := strings.Index(result, "{")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}")
		if end == -1 {
			break
		}
		end += start

		expr := result[start+1 : end]
		v, err := resolveTemplateExpr(expr, msg)
		if err != nil {
			return "", err
		}
		result = result[:start] + fmt.Sprintf("%v", v) + result[end+1:]
	}
	return result, nil
}

// resolveTemplateExpr 는 단일 {expr} 식을 해석한다 (v0.7.9).
// expr 가 "$." prefix 면 JSONPath-like 경로, 그 외는 payload 직접 필드.
//
// v0.13.0 확장: 메시지 top-level 필드 ($.id, $.type, $.timestamp) 지원.
//   - $.id        → msg.ID() (string)
//   - $.type      → msg.Type() (string)
//   - $.timestamp → msg.Timestamp().UnixMilli() (int64 epoch ms)
//   - $.payload.X / $.payload.x.y → payload JSONPath
//   - $.metadata.X → metadata 단일 키
func resolveTemplateExpr(expr string, msg message.Message) (any, error) {
	if !strings.HasPrefix(expr, "$.") {
		// Legacy: payload 직접 필드.
		// SPEC-NODE-001 v1.5.0: dot notation 지원 — {item.id} 또는 {item.nested.field}
		// 형태로 nested 객체 traverse. 단일 segment 는 기존 flat lookup 동작 유지.
		parts := strings.Split(expr, ".")
		if len(parts) == 1 {
			v, ok := msg.Payload().Get(expr)
			if !ok {
				return nil, fmt.Errorf("key template field %q not found in payload", expr)
			}
			return v, nil
		}
		return lookupPayloadPath(msg.Payload(), parts)
	}

	parts := strings.Split(expr[2:], ".")
	// Top-level 단일 segment 처리 ($.id, $.type, $.timestamp) (v0.13.0)
	if len(parts) == 1 {
		switch parts[0] {
		case "id":
			return msg.ID(), nil
		case "type":
			return msg.Type(), nil
		case "timestamp":
			return msg.Timestamp().UnixMilli(), nil
		case "payload", "metadata":
			// payload/metadata 는 sub-path 가 필수.
			return nil, fmt.Errorf("invalid key template path %q (expected $.payload.field or $.metadata.field)", expr)
		default:
			return nil, fmt.Errorf("unknown key template root %q (expected $.payload, $.metadata, $.id, $.type, $.timestamp)", parts[0])
		}
	}
	switch parts[0] {
	case "payload":
		return lookupPayloadPath(msg.Payload(), parts[1:])
	case "metadata":
		switch len(parts) {
		case 2:
			// $.metadata.{key} — flat 메타데이터 단일 키.
			v, ok := msg.Metadata().Get(parts[1])
			if !ok {
				return nil, fmt.Errorf("metadata key %q not found", parts[1])
			}
			return v, nil
		case 3:
			// $.metadata.{group}.{field} — 그룹(device/agent 등) 의 필드.
			// 예: $.metadata.device.id / $.metadata.device.type / $.metadata.agent.type.
			group, ok := msg.Metadata().GetGroup(parts[1])
			if !ok {
				return nil, fmt.Errorf("metadata group %q not found", parts[1])
			}
			v, ok := group[parts[2]]
			if !ok {
				return nil, fmt.Errorf("metadata group field %q.%q not found", parts[1], parts[2])
			}
			return v, nil
		default:
			// 그룹은 한 단계 깊이만 지원한다.
			return nil, fmt.Errorf("metadata path %q: too deep (groups are one level)", expr)
		}
	default:
		return nil, fmt.Errorf("unknown key template root %q (expected $.payload, $.metadata, $.id, $.type, $.timestamp)", parts[0])
	}
}

// lookupPayloadPath 는 payload 의 점-구분 경로를 따라간다 (v0.7.9).
// 첫 segment 는 payload.Get 으로 가져오고, 이후 segment 는 map[string]any 로 traverse.
func lookupPayloadPath(payload message.Payload, path []string) (any, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("empty payload path")
	}
	v, ok := payload.Get(path[0])
	if !ok {
		return nil, fmt.Errorf("payload key %q not found", path[0])
	}
	for i := 1; i < len(path); i++ {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("payload path %q: %q is not a nested object", strings.Join(path, "."), path[i-1])
		}
		v, ok = m[path[i]]
		if !ok {
			return nil, fmt.Errorf("payload path key %q not found", path[i])
		}
	}
	return v, nil
}

// isCompositeValue 는 값이 스칼라 필드/태그로 쓸 수 없는 복합 값
// (map / slice 등 구조형) 인지 보고한다. 스칼라(string/number/bool/nil)는 false.
func isCompositeValue(v any) bool {
	switch v.(type) {
	case nil, bool, string,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return false
	default:
		// map[string]any, []any, 구조체 등 → 오브젝트.
		return true
	}
}

// valueToString 은 값을 문자열로 변환한다(태그/시리즈 키/메트릭 이름 용 — 항상 문자열).
// 오브젝트는 JSON 으로, 스칼라는 fmt 로 변환한다. nil 은 빈 문자열.
func valueToString(v any) string {
	if v == nil {
		return ""
	}
	if isCompositeValue(v) {
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
	}
	return fmt.Sprintf("%v", v)
}

// scalarFieldValue 는 측정값으로 사용할 값을 반환한다. 스칼라는 타입을 보존하고
// (백엔드가 int/float/bool/string 을 구분하므로), 복합 값은 스칼라 필드로 쓸 수
// 없으므로 JSON 문자열로 변환한다.
func scalarFieldValue(v any) any {
	if isCompositeValue(v) {
		return valueToString(v)
	}
	return v
}
