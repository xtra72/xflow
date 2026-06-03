package node

import (
	"testing"

	"github.com/xtra/xflow/pkg/message"
)

// TestPromotePayloadMetadata_NestedMetadataPromoted 는 payload 의 nested metadata
// 그룹이 message metadata 로 promote 되고 payload 에서 제거되는지 검증한다 (v0.7.14).
func TestPromotePayloadMetadata_NestedMetadataPromoted(t *testing.T) {
	msg := message.New()
	payload := map[string]any{
		"type":         "device_state",
		"device_id":    "rac-01",
		"trigger":      "poll",
		"last_seen_ms": int64(1234567890),
		"state": map[string]any{
			"power": "on",
		},
		"metadata": map[string]any{
			"device_type": "rac",
			"name":        "Living Room",
			"slot_num":    1,
		},
	}

	// v0.18.26: UnitID / SlotNum 필드 삭제. 남은 옵션 (NodeID / DeviceType /
	// Name / NodeSource) 만 ON.
	promotePayloadMetadata(msg, payload, MetadataEmitOptions{NodeID: true, DeviceType: true, Name: true, NodeSource: true})

	if _, exists := payload["metadata"]; exists {
		t.Fatalf("payload['metadata'] 가 제거되어야 하지만 남아 있음: %v", payload["metadata"])
	}

	// v0.18.26: slot_num 은 출력 metadata 에서 제거.
	cases := map[string]string{
		"device_type": "rac",
		"name":        "Living Room",
	}
	for k, want := range cases {
		got, ok := msg.Metadata().Get(k)
		if !ok {
			t.Errorf("metadata[%q] 가 promote 되지 않음", k)
			continue
		}
		if got != want {
			t.Errorf("metadata[%q] = %q; want %q", k, got, want)
		}
	}

	// slot_num 은 v0.18.26 부터 metadata 로 promote 되지 않아야 한다.
	if _, ok := msg.Metadata().Get("slot_num"); ok {
		t.Errorf("slot_num 은 v0.18.26 부터 output metadata 에서 제거되어야 함")
	}

	if _, ok := payload["device_id"]; !ok {
		t.Errorf("다른 payload 필드(dev_id)는 보존되어야 함")
	}
}

// TestPromotePayloadMetadata_NoMetadataKey_NoOp 는 payload 에 metadata 키가 없을 때
// no-op 으로 동작하는지 검증한다.
func TestPromotePayloadMetadata_NoMetadataKey_NoOp(t *testing.T) {
	msg := message.New()
	payload := map[string]any{
		"type":      "device_state",
		"device_id": "rac-01",
	}

	promotePayloadMetadata(msg, payload, MetadataEmitOptions{})

	if len(payload) != 2 {
		t.Errorf("payload 가 변경되면 안 됨: got %v", payload)
	}
	// metadata().All() 은 비어 있어야 함
	all := msg.Metadata().All()
	if len(all) != 0 {
		t.Errorf("metadata 는 비어 있어야 함: got %v", all)
	}
}

// TestPromotePayloadMetadata_NotAMap_NoPromote 는 metadata 값이 map 이 아닌 경우
// promote 하지 않고 그대로 두는지 검증한다.
func TestPromotePayloadMetadata_NotAMap_NoPromote(t *testing.T) {
	msg := message.New()
	payload := map[string]any{
		"metadata": "this is a string, not a map",
	}

	promotePayloadMetadata(msg, payload, MetadataEmitOptions{})

	// payload['metadata'] 는 그대로 남아 있어야 함
	if v, ok := payload["metadata"]; !ok || v != "this is a string, not a map" {
		t.Errorf("metadata 가 map 이 아닐 때 payload 에서 제거하면 안 됨: got %v", v)
	}
	if len(msg.Metadata().All()) != 0 {
		t.Errorf("metadata 는 비어 있어야 함")
	}
}

// TestApplyDeviceStateMessageType_TriggerToMessageType 는 payload.trigger 가
// msg.Type()="device_state.<trigger>" 로 변환되고 payload 에서
// trigger 가 제거되는지 검증한다 (v0.8.0, v0.12.0+ 1급 채널).
func TestApplyDeviceStateMessageType_TriggerToMessageType(t *testing.T) {
	cases := []struct {
		trigger     string
		wantMsgType string
	}{
		{"change", "device_state.change"},
		{"report", "device_state.report"},
		{"keepalive", "device_state.keepalive"},
		{"init", "device_state.init"},
		{"poll", "device_state.poll"},
	}
	for _, tc := range cases {
		t.Run(tc.trigger, func(t *testing.T) {
			msg := message.New()
			payload := map[string]any{
				"device_id": "rac-01",
				"trigger":   tc.trigger,
			}
			applyDeviceStateMessageType(msg, payload, "fallback")

			mt := msg.Type()
			if mt != tc.wantMsgType {
				t.Errorf("msg.Type() = %q; want %q", mt, tc.wantMsgType)
			}
			if _, exists := payload["trigger"]; exists {
				t.Error("trigger 는 payload 에서 제거되어야 함")
			}
		})
	}
}

// TestApplyDeviceStateMessageType_FallbackWhenNoTrigger 는 trigger 가 없을 때
// defaultSubType 이 사용되는지 검증한다.
func TestApplyDeviceStateMessageType_FallbackWhenNoTrigger(t *testing.T) {
	msg := message.New()
	payload := map[string]any{"device_id": "rac-01"}

	applyDeviceStateMessageType(msg, payload, "poll")

	if mt := msg.Type(); mt != "device_state.poll" {
		t.Errorf("msg.Type() = %q; want %q", mt, "device_state.poll")
	}
}

// TestApplyDeviceStateMessageType_EmptyDefaultNoOp 는 trigger 가 없고
// defaultSubType 도 빈 경우 msg.Type 이 설정되지 않는지 검증한다.
func TestApplyDeviceStateMessageType_EmptyDefaultNoOp(t *testing.T) {
	msg := message.New()
	payload := map[string]any{"device_id": "rac-01"}

	applyDeviceStateMessageType(msg, payload, "")

	if mt := msg.Type(); mt != "" {
		t.Errorf("defaultSubType 이 비어 있고 trigger 도 없으면 msg.Type() 이 빈 문자열이어야 함: got %q", mt)
	}
}

// TestApplyDeviceStateMessageType_TriggerOverridesDefault 는 trigger 가 있으면
// defaultSubType 이 무시되는지 검증한다.
func TestApplyDeviceStateMessageType_TriggerOverridesDefault(t *testing.T) {
	msg := message.New()
	payload := map[string]any{"trigger": "change"}

	applyDeviceStateMessageType(msg, payload, "poll")

	if mt := msg.Type(); mt != "device_state.change" {
		t.Errorf("trigger 가 있으면 우선해야 함: got %q; want %q", mt, "device_state.change")
	}
}

// TestApplyPowerOffFilter_DisabledIsNoOp 는 enabled=false 일 때 어떠한 키도
// 제거되지 않는지 검증한다 (v0.18.0).
func TestApplyPowerOffFilter_DisabledIsNoOp(t *testing.T) {
	payload := map[string]any{
		"power":               false,
		"current_temperature": 25.0,
		"mode":                "cool",
		"fan_speed":           3,
		"target_temperature":  22.0,
	}

	applyPowerOffFilter(payload, false)

	for _, key := range []string{"power", "current_temperature", "mode", "fan_speed", "target_temperature"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("enabled=false 이면 %q 가 유지되어야 함", key)
		}
	}
}

// TestApplyPowerOffFilter_PowerOnIsNoOp 는 power=true 일 때 어떠한 키도
// 제거되지 않는지 검증한다 (v0.18.0).
func TestApplyPowerOffFilter_PowerOnIsNoOp(t *testing.T) {
	payload := map[string]any{
		"power":               true,
		"current_temperature": 25.0,
		"mode":                "cool",
		"fan_speed":           3,
	}

	applyPowerOffFilter(payload, true)

	for _, key := range []string{"power", "current_temperature", "mode", "fan_speed"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("power=true 이면 %q 가 유지되어야 함", key)
		}
	}
}

// TestApplyPowerOffFilter_PowerOffRemovesUnreliableState 는 power=false 일 때
// current_temperature, mode, fan_speed 만 제거되고 다른 필드는 유지되는지 검증한다 (v0.18.0).
func TestApplyPowerOffFilter_PowerOffRemovesUnreliableState(t *testing.T) {
	payload := map[string]any{
		"power":               false,
		"current_temperature": 25.0,
		"mode":                "cool",
		"fan_speed":           3,
		"target_temperature":  22.0,
		"online":              true,
	}

	applyPowerOffFilter(payload, true)

	// 제거되어야 하는 키.
	for _, key := range []string{"current_temperature", "mode", "fan_speed"} {
		if _, ok := payload[key]; ok {
			t.Errorf("power=false / enabled=true 일 때 %q 가 제거되어야 함", key)
		}
	}
	// 유지되어야 하는 키 (OFF 에서도 의미있는 필드).
	for _, key := range []string{"power", "target_temperature", "online"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("power=false 라도 %q 는 유지되어야 함", key)
		}
	}
}

// TestApplyPowerOffFilter_PowerNotBoolIsNoOp 는 power 가 bool 이 아니면
// 어떠한 키도 제거되지 않는지 검증한다 (v0.18.0, 보수적 fallback).
func TestApplyPowerOffFilter_PowerNotBoolIsNoOp(t *testing.T) {
	payload := map[string]any{
		"power":               "off", // string, 보수적으로 무시
		"current_temperature": 25.0,
		"mode":                "cool",
	}

	applyPowerOffFilter(payload, true)

	for _, key := range []string{"power", "current_temperature", "mode"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("power 가 bool 이 아니면 %q 가 유지되어야 함", key)
		}
	}
}

// TestApplyPowerOffFilter_NoPowerKeyIsNoOp 는 power 키가 없으면 어떠한 키도
// 제거되지 않는지 검증한다 (v0.18.0).
func TestApplyPowerOffFilter_NoPowerKeyIsNoOp(t *testing.T) {
	payload := map[string]any{
		"current_temperature": 25.0,
		"mode":                "cool",
	}

	applyPowerOffFilter(payload, true)

	for _, key := range []string{"current_temperature", "mode"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("power 키가 없으면 %q 가 유지되어야 함", key)
		}
	}
}
