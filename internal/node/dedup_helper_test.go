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
		"dev_id":       "rac-01",
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

	if _, ok := payload["dev_id"]; !ok {
		t.Errorf("다른 payload 필드(dev_id)는 보존되어야 함")
	}
}

// TestPromotePayloadMetadata_NoMetadataKey_NoOp 는 payload 에 metadata 키가 없을 때
// no-op 으로 동작하는지 검증한다.
func TestPromotePayloadMetadata_NoMetadataKey_NoOp(t *testing.T) {
	msg := message.New()
	payload := map[string]any{
		"type":   "device_state",
		"dev_id": "rac-01",
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
