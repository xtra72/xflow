// metadata_opts.go (v0.18.26) 는 HVAC 노드의 metadata 옵션 emit 정책을 정의한다.
//
// 출력 metadata 정책:
//
// 필수 필드 (항상 emit):
//   - device_id  — 글로벌 고유 UUID (프로토콜 식별자 unit_id 와는 별개)
//
// 옵션 필드 (default OFF, 노드 config 에서 토글):
//   - node_id    — 메시지를 emit 한 노드 UUID
//   - device_type — "HVACR.IDU" / "HVACR.ODU"
//   - label       — 사용자 라벨 (없으면 자동 생성된 기본명)
//   - node_source — emit 경로 식별 ("poll_bulk", "device_state", "poll" 등)
//
// 제거된 필드 (v0.18.26):
//   - unit_id   — 프로토콜 해석 시에만 의미 있는 식별자. 출력 metadata 에는
//     포함하지 않는다. 프로토콜 타깃 지정용 어드레싱은 노드 config 의
//     group_id / unit_id 입력 필드로 처리한다.
//   - slot_num  — Samsung NASA / LG ICP-01 의 슬롯 번호. 의미가 모호하고
//     사실상 unit_id 의 부분 표현이라 emit 하지 않는다.
//
// 기본값 정책: minimal — device_id 만 emit, 그 외 OFF. 사용자가 Web UI 에서
// 명시적으로 활성화한 경우에만 추가 emit.

package node

import "fmt"

// MetadataEmitOptions 는 노드 emit 시 metadata 에 포함할 옵션 필드를 제어한다.
//
// JSON 직렬화 시 snake_case 사용. 모든 필드는 기본 false — 즉 default 동작은
// device_id 만 emit 하는 minimal mode.
//
// v0.18.26 (2026-05-28): UnitID / SlotNum 필드 삭제. 두 키는 프로토콜 해석
// 단계에서만 의미가 있고 출력 metadata 로 노출할 가치가 없다고 판정. 어드레싱
// (프로토콜 타깃 지정) 은 노드 config 의 group_id / unit_id 입력 필드로 분리.
type MetadataEmitOptions struct {
	NodeID     bool `json:"node_id"`
	DeviceType bool `json:"device_type"`
	Label      bool `json:"label"`
	NodeSource bool `json:"node_source"`
}

// IsAllowed 는 주어진 metadata key 가 현재 옵션에서 허용되는지 반환한다.
// device_id 는 필수 시스템 키로 항상 허용 (true).
// node_id / device_type / label / node_source 는 해당 옵션 플래그에 따라 결정.
// unit_id / slot_num 는 항상 거부 (v0.18.26 부터 출력 metadata 에서 제거).
// 그 외 key 는 forward-compat 차원에서 기본 허용 (true).
func (o MetadataEmitOptions) IsAllowed(key string) bool {
	switch key {
	case "unit_id", "slot_num":
		return false
	case "node_id":
		return o.NodeID
	case "device_type":
		return o.DeviceType
	case "label":
		return o.Label
	case "node_source":
		return o.NodeSource
	}
	return true
}

// SetIfAllowed 는 옵션이 허용된 경우에만 metadata 에 (key, value) 를 설정한다.
// value 는 fmt.Sprintf 로 string 변환 (message metadata 는 string-only).
func (o MetadataEmitOptions) SetIfAllowed(setter func(string, string), key string, value any) {
	if !o.IsAllowed(key) {
		return
	}
	if value == nil {
		return
	}
	if s, ok := value.(string); ok {
		if s == "" {
			return
		}
		setter(key, s)
		return
	}
	setter(key, fmt.Sprintf("%v", value))
}

// parseEmitMetadata 는 노드 config map 에서 옵션을 파싱한다 (두 형식 모두 지원).
//
// 1) 중첩 형식: config["emit_metadata"] = map[string]any{"device_type": true, ...}
// 2) 평탄 형식: config["emit_device_type"] = true, config["emit_label"] = true, ...
//
// 두 형식 모두 동일 옵션에 매핑되며, 중첩이 우선 적용 후 평탄이 덮어쓴다.
// 누락된 키는 변경되지 않음 (out 의 기존 값 보존).
//
// Web UI 호환을 위해 평탄 형식도 수용 — form serializer 가 중첩 객체를
// 자연스럽게 표현하지 못하는 경우 fallback.
//
// v0.18.26: emit_unit_id / emit_slot_num 키는 silently 무시 (forward-compat).
// 기존 설정 파일에서 해당 키가 남아 있어도 에러 없이 진행한다.
func parseEmitMetadata(config map[string]any, out *MetadataEmitOptions) {
	if raw, ok := config["emit_metadata"]; ok {
		if m, ok := raw.(map[string]any); ok {
			if v, ok := m["node_id"].(bool); ok {
				out.NodeID = v
			}
			if v, ok := m["device_type"].(bool); ok {
				out.DeviceType = v
			}
			if v, ok := m["label"].(bool); ok {
				out.Label = v
			}
			if v, ok := m["node_source"].(bool); ok {
				out.NodeSource = v
			}
		}
	}
	if v, ok := config["emit_node_id"].(bool); ok {
		out.NodeID = v
	}
	if v, ok := config["emit_device_type"].(bool); ok {
		out.DeviceType = v
	}
	if v, ok := config["emit_label"].(bool); ok {
		out.Label = v
	}
	if v, ok := config["emit_node_source"].(bool); ok {
		out.NodeSource = v
	}
}
