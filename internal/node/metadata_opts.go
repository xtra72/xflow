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
//   - name        — 사용자 라벨 (없으면 자동 생성된 기본명)
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
//
// v0.19.0 (P3, 2026-06-09): Agent / Device 그룹 emit 플래그 추가.
//   - Agent: 모든 에이전트 생성 메시지에 agent:{type,id,[name]} 그룹 emit.
//   - Device: HVACR 계열 디바이스 노드의 device:{type,id} 그룹 emit.
//
// 중요: Agent / Device 는 기타 flat 필드(NodeID 등 default OFF)와 달리 기본값이
// ON 이다. 사용자가 그룹 메타데이터를 기본으로 원하기 때문. zero-value 의 bool 은
// false 이므로 기본 ON 은 PARSE 시점에서 처리한다 (parseEmitMetadata / DefaultEmitOptions
// 가 명시 비활성화 없으면 true 로 설정). 구조체 zero-value 를 직접 쓰는 단위 테스트는
// 필요 시 Agent/Device 를 명시한다.
type MetadataEmitOptions struct {
	NodeID     bool `json:"node_id"`
	DeviceType bool `json:"device_type"`
	Name       bool `json:"name"`
	NodeSource bool `json:"node_source"`
	// Agent 는 agent:{type,id,[name]} 그룹 emit 여부. 기본 ON (parse 시 default true).
	Agent bool `json:"agent"`
	// Device 는 device:{type,id} 그룹 emit 여부. 기본 ON (parse 시 default true).
	Device bool `json:"device"`
	// Detail 은 agent / device 그룹의 "상세 정보" emit 여부. 기본 ON (parse 시 default true).
	//
	// OFF 면 두 그룹은 id 하나로 축소된다 (agent.id / device.id 만; name / type /
	// dev_eui 제거). Agent / Device 와 마찬가지로 zero-value 가 false 이므로 기본 ON 은
	// PARSE 시점(parseEmitMetadata / DefaultEmitOptions)에서 강제한다.
	//
	// 적용 범위: 축소는 chirpstack 노드 로컬이다(reduceChirpStackIdentityGroups).
	// 공유 헬퍼(emitAgentGroup / mergeDeviceGroup / promoteDevIDWithUUID)는 이 값을
	// 읽지 않으므로 다른 노드 타입의 출력은 영향을 받지 않는다.
	Detail bool `json:"detail"`
}

// DefaultEmitOptions 는 P3 기본 emit 정책을 반환한다: Agent / Device 그룹과 Detail 은
// ON, 그 외 flat 옵션(NodeID/DeviceType/Name/NodeSource)은 OFF.
//
// 사용처: emit-options 를 별도 config 로 파싱하지 않는 노드(serial_io / mqtt /
// modbus / tcp_io 등)가 그룹 emit 기본값을 얻기 위해 사용. parseEmitMetadata 도
// 동일한 기본값을 적용한다.
//
// Detail 을 여기서 true 로 두는 것이 중요하다 — zero-value(false)로 새면 위 노드들의
// metadata 가 의도치 않게 축소된 것으로 해석될 수 있다.
func DefaultEmitOptions() MetadataEmitOptions {
	return MetadataEmitOptions{Agent: true, Device: true, Detail: true}
}

// IsAllowed 는 주어진 metadata key 가 현재 옵션에서 허용되는지 반환한다.
// device_id 는 필수 시스템 키로 항상 허용 (true).
// node_id / device_type / name / node_source 는 해당 옵션 플래그에 따라 결정.
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
	case "name":
		return o.Name
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

// SetAgentGroupIfAllowed 는 Agent 옵션이 ON 일 때 agent 그룹을 설정한다.
//
// 그룹 형태: agent: {type, id, [name]}. name 은 비어있지 않을 때만 포함한다.
// type, id 가 둘 다 비어있으면 그룹을 만들지 않는다 (의미 없는 빈 그룹 방지).
//
// setGroup 은 message.Metadata().SetGroup 시그니처: func(key string, fields map[string]string).
func (o MetadataEmitOptions) SetAgentGroupIfAllowed(setGroup func(string, map[string]string), agentType, agentID, agentName string) {
	if !o.Agent {
		return
	}
	if agentType == "" && agentID == "" {
		return
	}
	fields := map[string]string{
		"type": agentType,
		"id":   agentID,
	}
	if agentName != "" {
		fields["name"] = agentName
	}
	setGroup("agent", fields)
}

// SetDeviceGroupIfAllowed 는 Device 옵션이 ON 일 때 device 그룹을 설정한다.
//
// 그룹 형태: device: {type, id, name}. 각 필드는 비어있지 않을 때만 포함한다.
// type, id, name 이 모두 비어있으면 그룹을 만들지 않는다 (의미 없는 빈 그룹 방지).
//
// 주의: SetGroup 은 전체 치환이므로, 호출 측이 type / id / name 을 따로(다른
// 소스에서) 알게 되는 HVACR promote 경로에서는 mergeDeviceGroup 헬퍼로 누적
// 병합한다. 이 메서드는 모든 필드를 한 번에 알고 있는 단순 케이스용이다.
func (o MetadataEmitOptions) SetDeviceGroupIfAllowed(setGroup func(string, map[string]string), deviceType, deviceID, deviceName string) {
	if !o.Device {
		return
	}
	if deviceType == "" && deviceID == "" && deviceName == "" {
		return
	}
	fields := make(map[string]string, 3)
	if deviceType != "" {
		fields["type"] = deviceType
	}
	if deviceID != "" {
		fields["id"] = deviceID
	}
	if deviceName != "" {
		fields["name"] = deviceName
	}
	setGroup("device", fields)
}

// parseEmitMetadata 는 노드 config map 에서 옵션을 파싱한다 (두 형식 모두 지원).
//
// 1) 중첩 형식: config["emit_metadata"] = map[string]any{"device_type": true, ...}
// 2) 평탄 형식: config["emit_device_type"] = true, config["emit_name"] = true, ...
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
	// P3: Agent / Device 그룹은 기본 ON. 명시 비활성화(false) 가 없으면 true.
	// bool zero-value 가 false 이므로 여기서 기본값을 강제한다. 기존 flat 옵션
	// (NodeID/DeviceType/Name/NodeSource) 은 default OFF 정책 유지 — 손대지 않는다.
	//
	// Detail(상세 정보) 도 동일한 default-true 취급을 받는다. 기본값을 ON 으로 두어야
	// 기존 배포의 metadata 모양이 그대로 유지된다 (absent = 오늘과 동일).
	out.Agent = true
	out.Device = true
	out.Detail = true

	if raw, ok := config["emit_metadata"]; ok {
		if m, ok := raw.(map[string]any); ok {
			if v, ok := m["node_id"].(bool); ok {
				out.NodeID = v
			}
			if v, ok := m["device_type"].(bool); ok {
				out.DeviceType = v
			}
			if v, ok := m["name"].(bool); ok {
				out.Name = v
			}
			if v, ok := m["node_source"].(bool); ok {
				out.NodeSource = v
			}
			if v, ok := m["agent"].(bool); ok {
				out.Agent = v
			}
			if v, ok := m["device"].(bool); ok {
				out.Device = v
			}
			if v, ok := m["detail"].(bool); ok {
				out.Detail = v
			}
		}
	}
	if v, ok := config["emit_node_id"].(bool); ok {
		out.NodeID = v
	}
	if v, ok := config["emit_device_type"].(bool); ok {
		out.DeviceType = v
	}
	if v, ok := config["emit_name"].(bool); ok {
		out.Name = v
	}
	if v, ok := config["emit_node_source"].(bool); ok {
		out.NodeSource = v
	}
	if v, ok := config["emit_agent"].(bool); ok {
		out.Agent = v
	}
	if v, ok := config["emit_device"].(bool); ok {
		out.Device = v
	}
	if v, ok := config["emit_detail"].(bool); ok {
		out.Detail = v
	}
}
