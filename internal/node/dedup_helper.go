package node

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/message"
)

// applyDeviceStateMessageType 는 HVAC device_state 메시지의 type 을 계층형 값
// ("device_state.<sub_type>") 으로 설정한다 (v0.8.0, v0.12.0: msg.SetType 사용).
//
// 동작:
//   - payload 의 "trigger" 키 (string) 를 sub_type 으로 사용
//   - payload 에서 "trigger" 키 제거 (이제 msg.Type 에 인코딩됨)
//   - trigger 가 없거나 빈 문자열이면 defaultSubType 사용
//   - 최종 msg.Type() = "device_state." + sub_type
//
// 예시:
//   - trigger="change" → msg.Type="device_state.change"
//   - trigger 없음, defaultSubType="poll" → msg.Type="device_state.poll"
//   - Process 응답 등 trigger 무관 경로는 직접 "device_state.response" 등을 호출자가 지정
//
// v0.12.0: 이전엔 metadata.message_type 에 설정 → 이제 msg top-level Type 로 promote.
func applyDeviceStateMessageType(msg message.Message, payload map[string]any, defaultSubType string) {
	subType := defaultSubType
	if raw, ok := payload["trigger"]; ok {
		if s, ok := raw.(string); ok && s != "" {
			subType = s
		}
		delete(payload, "trigger")
	}
	if subType == "" {
		return
	}
	msg.SetType("device_state." + subType)
}

// emitAgentGroup 는 에이전트 생성 메시지에 agent:{type,id,[name]} 그룹을 emit 한다 (P3).
//
// nil-guard: a 또는 msg 가 nil 이면 no-op. opts.Agent 가 OFF 면 SetAgentGroupIfAllowed
// 내부에서 no-op. type/id 가 둘 다 비어있어도 no-op.
//
// 사용처: 에이전트를 래핑하고 메시지를 생성하는 모든 노드(HVACR 계열 + serial/mqtt/
// modbus/tcp 등) 의 outgoing 메시지 빌드 직후 호출.
func emitAgentGroup(msg message.Message, a agent.Agent, opts MetadataEmitOptions) {
	if msg == nil || a == nil {
		return
	}
	opts.SetAgentGroupIfAllowed(msg.Metadata().SetGroup, a.Type(), a.ID(), a.Name())
}

// mergeDeviceGroup 는 device 그룹에 (type, id) 중 비어있지 않은 필드를 누적 병합한다 (P3).
//
// device 의 type 과 id 는 서로 다른 소스(payload.metadata.device_type / 등록 DeviceInfo /
// payload.device_id 해석 UUID)에서 따로 들어오므로, SetGroup 의 전체 치환으로 인한
// clobber 를 막기 위해 기존 그룹을 읽어 빈 필드만 갱신한다.
//
// opts.Device 가 OFF 면 no-op. type/id 둘 다 비어있어도 no-op.
func mergeDeviceGroup(msg message.Message, opts MetadataEmitOptions, deviceType, deviceID string) {
	if !opts.Device {
		return
	}
	if deviceType == "" && deviceID == "" {
		return
	}
	fields, _ := msg.Metadata().GetGroup("device")
	if fields == nil {
		fields = make(map[string]string, 2)
	}
	if deviceType != "" {
		fields["type"] = deviceType
	}
	if deviceID != "" {
		fields["id"] = deviceID
	}
	msg.Metadata().SetGroup("device", fields)
}

// promoteDevIDToMetadata 는 payload 의 device_id 키를 metadata 로 이동한다 (v0.18.26, P3).
//
// 동작:
//   - payload["unit_id"] (프로토콜 식별자) 가 있으면 항상 payload 에서 제거.
//     v0.18.26 부터 unit_id 는 출력 metadata 로 노출하지 않는다 (의미가 프로토콜
//     해석에 한정되며, 어드레싱은 노드 config 의 input 필드로 분리).
//   - payload["device_id"] (글로벌 UUID) 를:
//     1. flat metadata.device_id 로 promote (P3: MQTT 토픽 템플릿
//     $.metadata.device_id 소비자가 flat 키를 사용하므로 보존).
//     2. device:{...} 그룹의 id 로도 병합 (opts.Device ON 일 때).
//   - device_id 가 없으면 device_id 에 한해 no-op.
//
// P3 보존 근거: resolveTemplateExpr (store_write.go) 는 nested metadata path 를
// 지원하지 않아 ($.metadata.device.id 불가) flat device_id 가 필요하다.
func promoteDevIDToMetadata(msg message.Message, payload map[string]any, opts MetadataEmitOptions) {
	if _, ok := payload["unit_id"]; ok {
		delete(payload, "unit_id")
	}
	deviceID := stringifyPayloadKey(payload, "device_id")
	// flat 보존 (기존 동작 + MQTT 템플릿 호환).
	promotePayloadKeyToMetadata(msg, payload, "device_id")
	// device 그룹의 id 로도 병합.
	mergeDeviceGroup(msg, opts, "", deviceID)
}

// stringifyPayloadKey 는 payload[key] 를 string 으로 (제거 없이) 읽어 반환한다.
// 키가 없으면 "" 반환.
func stringifyPayloadKey(payload map[string]any, key string) string {
	raw, ok := payload[key]
	if !ok {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", raw)
}

// promoteDevIDWithUUID 는 promoteDevIDToMetadata 의 확장: agentName 이 비어있지 않으면
// payload["unit_id"] 를 키로
//
//  1. 글로벌 UUID 를 조회해 payload["device_id"] 에 주입 (v0.18.7)
//  2. 등록된 DeviceInfo (device_type / name) 가 있으면 opts 가 허용한 필드만
//     metadata 에 직접 주입 (v0.18.7, v0.18.8 에서 opts 도입)
//
// 한 뒤 device_id 를 metadata 로 promote 하고 unit_id 는 payload 에서 제거한다.
//
// opts: MetadataEmitOptions — DeviceType / Name 등 옵션 필드 토글.
// zero-value 시 device_type / name 은 emit 되지 않음 (default minimal).
//
// 주의: name metadata 의 값 소스는 DeviceInfo.Label (디바이스 라벨 값) 이며,
// 출력 metadata 키 이름만 "name" 으로 노출한다.
//
// v0.18.26: unit_id 출력 토글 제거. unit_id 는 항상 payload 에서 제거되고
// metadata 에는 노출되지 않는다.
func promoteDevIDWithUUID(msg message.Message, payload map[string]any, agentName string, opts MetadataEmitOptions) {
	if agentName != "" {
		if rawUnitID, ok := payload["unit_id"]; ok {
			unitIDStr := fmt.Sprintf("%v", rawUnitID)
			if uuid := agent.ResolveDeviceID(context.Background(), agentName, unitIDStr); uuid != "" {
				payload["device_id"] = uuid
			}
			if opts.DeviceType || opts.Name || opts.Device {
				if info, ok := agent.GetDeviceInfo(agentName, unitIDStr); ok {
					// P3: device_type 은 device 그룹의 type 으로 병합 (opts.Device).
					// flat device_type 도 보존 (opts.DeviceType, MQTT 템플릿 호환).
					if info.DeviceType != "" {
						if opts.DeviceType {
							msg.Metadata().Set("device_type", info.DeviceType)
						}
						mergeDeviceGroup(msg, opts, info.DeviceType, "")
					}
					// name(디바이스 라벨) 은 flat 으로만 유지 (그룹 스펙 외, 기존 동작).
					if opts.Name && info.Label != "" {
						msg.Metadata().Set("name", info.Label)
					}
				}
			}
		}
	}
	promoteDevIDToMetadata(msg, payload, opts)
}

// promotePayloadKeyToMetadata 는 payload[key] 를 metadata[key] 로 옮긴다 (string 변환 포함).
func promotePayloadKeyToMetadata(msg message.Message, payload map[string]any, key string) {
	raw, ok := payload[key]
	if !ok {
		return
	}
	if s, ok := raw.(string); ok {
		msg.Metadata().Set(key, s)
	} else {
		msg.Metadata().Set(key, fmt.Sprintf("%v", raw))
	}
	delete(payload, key)
}

// applyPowerOffFilter 는 power=false 일 때 신뢰할 수 없는 상태 필드
// (current_temperature, mode, fan_speed) 를 payload 에서 제거한다 (v0.18.0).
//
// 동작:
//   - enabled=false 면 no-op
//   - payload["power"] 가 bool 이 아니거나 true 면 no-op
//   - payload["power"] == false 이면 current_temperature / mode / fan_speed 키 제거
//
// 의도: HVAC 디바이스가 OFF 상태일 때 emit 되는 mode=0 / fan_speed=0 / current_temperature
// (마지막 측정값) 가 downstream consumer (대시보드, InfluxDB) 를 혼동시키지 않도록.
// target_temperature, online, dev 식별자 등 OFF 에서도 의미있는 필드는 보존.
func applyPowerOffFilter(payload map[string]any, enabled bool) {
	if !enabled {
		return
	}
	power, ok := payload["power"].(bool)
	if !ok || power {
		return
	}
	delete(payload, "current_temperature")
	delete(payload, "mode")
	delete(payload, "fan_speed")
}

// flattenStateToPayload 는 payload 의 nested "state" 객체를 payload 루트로
// 평탄화한다 (v0.13.0).
//
// 동작:
//   - payload["state"] 가 map[string]any 이면 그 안의 키들을 payload 루트로 이동
//   - payload 에서 "state" 키 제거
//   - state 의 키가 payload 루트의 기존 키와 충돌하면 state 값으로 덮어씀
//     (HVAC schema 상 충돌이 발생할 일이 없는 구조)
//   - state 가 없거나 map 이 아니면 no-op
//
// 의도: msg.Type 이 이미 "device_state.X" 라 schema 가 device state 임이 명시되어
// payload 가 곧 state. state wrapper 는 prefix 의 중복.
func flattenStateToPayload(payload map[string]any) {
	raw, ok := payload["state"]
	if !ok {
		return
	}
	stateMap, ok := raw.(map[string]any)
	if !ok {
		return
	}
	for k, v := range stateMap {
		payload[k] = v
	}
	delete(payload, "state")
}

// promoteLastSeenToTimestamp 는 payload 의 "last_seen_ms" (int64 epoch ms) 를
// message.Timestamp 로 promote 한다 (v0.12.0).
//
// 동작:
//   - payload["last_seen_ms"] 가 정수 타입이면 time.UnixMilli(value) 로 변환하여
//     msg.SetTimestamp 호출
//   - payload 에서 "last_seen_ms" 키 제거
//   - 없거나 정수가 아니면 no-op (msg.Timestamp 는 기본 time.Now() 유지)
//
// JSON unmarshal 시 숫자는 float64 가 되므로 둘 다 처리한다.
func promoteLastSeenToTimestamp(msg message.Message, payload map[string]any) {
	raw, ok := payload["last_seen_ms"]
	if !ok {
		return
	}
	var ms int64
	switch v := raw.(type) {
	case int64:
		ms = v
	case int:
		ms = int64(v)
	case float64:
		ms = int64(v)
	default:
		return
	}
	msg.SetTimestamp(time.UnixMilli(ms))
	delete(payload, "last_seen_ms")
}

// promotePayloadMetadata 는 payload map 의 "metadata" 키 (nested object) 를
// 메시지 metadata 로 이동한다 (v0.7.14, v0.18.8 에서 opts 도입).
//
// 사용 의도: HVAC status 노드의 emit schema 는 payload 내부에 metadata 그룹
// (`{device_type, label, slot_num, ...}`) 을 포함한다. 이 정보는 의미상
// 메시지 metadata 에 속하므로, 노드가 message 로 빌드할 때 promote 한다.
//
//   - payload 의 "metadata" 키가 map[string]any 가 아니면 no-op
//   - opts 가 허용한 key 만 promote (device_type / label / slot_num 은 토글 가능,
//     그 외는 forward-compat 차원에서 기본 허용)
//   - metadata 값은 string 으로 변환 (message metadata 는 string-only)
//   - promote 후 payload 에서 "metadata" 키 제거 (허용 여부와 무관 — payload 에서는 항상 제거)
//
// opts: MetadataEmitOptions zero-value 시 device_type / label / slot_num 모두 skip
// (default minimal).
func promotePayloadMetadata(msg message.Message, payload map[string]any, opts MetadataEmitOptions) {
	rawMeta, ok := payload["metadata"]
	if !ok {
		return
	}
	m, ok := rawMeta.(map[string]any)
	if !ok {
		return
	}
	for k, v := range m {
		sv := fmt.Sprintf("%v", v)
		// P3: device_type 은 device 그룹의 type 으로도 병합한다 (opts.Device).
		// flat device_type 은 기존대로 opts.DeviceType 토글에 따라 유지 (MQTT 템플릿
		// $.metadata.device_type 호환). 그룹/플랫은 각각 독립 토글.
		if k == "device_type" {
			mergeDeviceGroup(msg, opts, sv, "")
		}
		if !opts.IsAllowed(k) {
			continue
		}
		msg.Metadata().Set(k, sv)
	}
	delete(payload, "metadata")
}

// normalizeForDedup 는 HVAC status 노드의 pollSingle 응답에서 휘발성 필드
// (last_seen_ms 등) 를 제거한 정규화 bytes 를 반환한다 (v0.7.8).
//
// 목적: byte-equal dedup 비교 시 매 frame 마다 변하는 timestamp 가 비교를
// 무효화하지 않도록 한다. 출력 자체는 원본 그대로 emit 한다.
//
// 처리 대상:
//   - top-level "last_seen_ms"
//   - devices[].last_seen_ms (get_all 응답)
//   - device.last_seen_ms (get_state 응답)
//
// JSON parse 실패 시 원본 resp 를 그대로 반환 (안전한 fallback).
func normalizeForDedup(resp []byte) []byte {
	var m map[string]any
	if err := json.Unmarshal(resp, &m); err != nil {
		return resp
	}
	delete(m, "last_seen_ms")

	// get_all: devices 배열의 각 device 에서 last_seen_ms 제거.
	if devs, ok := m["devices"].([]any); ok {
		for _, d := range devs {
			if dm, ok := d.(map[string]any); ok {
				delete(dm, "last_seen_ms")
			}
		}
	}
	// get_state: 단일 device 객체.
	if d, ok := m["device"].(map[string]any); ok {
		delete(d, "last_seen_ms")
	}

	out, err := json.Marshal(m)
	if err != nil {
		return resp
	}
	return out
}
