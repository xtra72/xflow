package node

import "encoding/json"

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
