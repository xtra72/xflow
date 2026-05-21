package node

import (
	"encoding/json"
	"fmt"

	"github.com/xtra/xflow/pkg/message"
)

// applyDeviceStateMessageType 는 HVAC device_state 메시지의 metadata.message_type 을
// 계층형 값 ("device_state.<sub_type>") 으로 설정한다 (v0.8.0 breaking change).
//
// 동작:
//   - payload 의 "trigger" 키 (string) 를 sub_type 으로 사용
//   - payload 에서 "trigger" 키 제거 (이제 metadata 에 인코딩됨)
//   - trigger 가 없거나 빈 문자열이면 defaultSubType 사용
//   - 최종 message_type = "device_state." + sub_type
//
// 예시:
//   - trigger="change" → message_type="device_state.change"
//   - trigger 없음, defaultSubType="poll" → message_type="device_state.poll"
//   - Process 응답 등 trigger 무관 경로는 직접 "device_state.response" 등을 호출자가 지정
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
	msg.Metadata().Set("message_type", "device_state."+subType)
}

// promotePayloadMetadata 는 payload map 의 "metadata" 키 (nested object) 를
// 메시지 metadata 로 이동한다 (v0.7.14).
//
// 사용 의도: HVAC status 노드의 emit schema 는 payload 내부에 metadata 그룹
// (`{device_type, label, slot_num, ...}`) 을 포함한다. 이 정보는 의미상
// 메시지 metadata 에 속하므로, 노드가 message 로 빌드할 때 promote 한다.
//
//   - payload 의 "metadata" 키가 map[string]any 가 아니면 no-op
//   - metadata 값은 string 으로 변환 (message metadata 는 string-only)
//   - promote 후 payload 에서 "metadata" 키 제거
func promotePayloadMetadata(msg message.Message, payload map[string]any) {
	rawMeta, ok := payload["metadata"]
	if !ok {
		return
	}
	m, ok := rawMeta.(map[string]any)
	if !ok {
		return
	}
	for k, v := range m {
		msg.Metadata().Set(k, fmt.Sprintf("%v", v))
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
