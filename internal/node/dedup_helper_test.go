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
		"device_id":       "rac-01",
		"trigger":      "poll",
		"last_seen_ms": int64(1234567890),
		"state": map[string]any{
			"power": "on",
		},
		"metadata": map[string]any{
			"device_type": "rac",
			"label":       "Living Room",
			"slot_num":    1,
		},
	}

	promotePayloadMetadata(msg, payload)

	if _, exists := payload["metadata"]; exists {
		t.Fatalf("payload['metadata'] 가 제거되어야 하지만 남아 있음: %v", payload["metadata"])
	}

	cases := map[string]string{
		"device_type": "rac",
		"label":       "Living Room",
		"slot_num":    "1",
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

	if _, ok := payload["device_id"]; !ok {
		t.Errorf("다른 payload 필드(dev_id)는 보존되어야 함")
	}
}

// TestPromotePayloadMetadata_NoMetadataKey_NoOp 는 payload 에 metadata 키가 없을 때
// no-op 으로 동작하는지 검증한다.
func TestPromotePayloadMetadata_NoMetadataKey_NoOp(t *testing.T) {
	msg := message.New()
	payload := map[string]any{
		"type":   "device_state",
		"device_id": "rac-01",
	}

	promotePayloadMetadata(msg, payload)

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

	promotePayloadMetadata(msg, payload)

	// payload['metadata'] 는 그대로 남아 있어야 함
	if v, ok := payload["metadata"]; !ok || v != "this is a string, not a map" {
		t.Errorf("metadata 가 map 이 아닐 때 payload 에서 제거하면 안 됨: got %v", v)
	}
	if len(msg.Metadata().All()) != 0 {
		t.Errorf("metadata 는 비어 있어야 함")
	}
}

// TestApplyDeviceStateMessageType_TriggerToMessageType 는 payload.trigger 가
// metadata.message_type="device_state.<trigger>" 로 변환되고 payload 에서
// trigger 가 제거되는지 검증한다 (v0.8.0).
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
				"device_id":  "rac-01",
				"trigger": tc.trigger,
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
