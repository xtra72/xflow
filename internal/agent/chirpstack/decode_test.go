package chirpstack

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

// loadRawUplink 는 testdata/packet.json 의 첫 JSON 객체(xflow 메시지 래퍼)에서
// payload(원시 ChirpStack 업링크)를 추출해 반환한다.
//
// 실제 배포에서 ChirpStack MQTT integration 은 원시 업링크(top-level deviceInfo/
// object/time/rxInfo)를 발행한다. 픽스처는 이를 xflow 메시지로 래핑해 캡처한
// 것이므로, 테스트는 .payload 를 꺼내 원시 업링크를 얻는다.
func loadRawUplink(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/packet.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	var wrapper struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := dec.Decode(&wrapper); err != nil {
		t.Fatalf("decode fixture wrapper: %v", err)
	}
	if len(wrapper.Payload) == 0 {
		t.Fatal("fixture wrapper has no payload")
	}
	return wrapper.Payload
}

// TestDecodeUplink_Fixture 는 실측 WS301 업링크 디코드를 검증한다 (REQ-M3-01).
func TestDecodeUplink_Fixture(t *testing.T) {
	up, err := decodeUplink(loadRawUplink(t))
	if err != nil {
		t.Fatalf("decodeUplink: %v", err)
	}

	if up.DeviceInfo.DevEui != "24e124141d180806" {
		t.Errorf("devEui = %q", up.DeviceInfo.DevEui)
	}
	if up.DeviceInfo.DeviceName != "WS301-180806" {
		t.Errorf("deviceName = %q", up.DeviceInfo.DeviceName)
	}
	if up.DeviceInfo.DeviceProfileName != "WS301" {
		t.Errorf("deviceProfileName = %q", up.DeviceInfo.DeviceProfileName)
	}
	if got := up.DeviceInfo.Tags["location"]; got != "실습실" {
		t.Errorf("tags.location = %q, want 실습실", got)
	}
	if up.DeviceInfo.Tags["point"] != "앞문" || up.DeviceInfo.Tags["spot"] != "앞문" {
		t.Errorf("tags point/spot = %q/%q, want 앞문/앞문", up.DeviceInfo.Tags["point"], up.DeviceInfo.Tags["spot"])
	}
	if v, ok := up.Object["magnet_status"]; !ok || v != "close" {
		t.Errorf("object.magnet_status = %v, want close", v)
	}
	if up.Time == "" {
		t.Error("time should be present")
	}
}

// TestDecodeUplink_MissingDevEui 는 devEui 누락 시 방어적으로 에러를 반환하는지
// 검증한다 (device_id 키잉 불가).
func TestDecodeUplink_MissingDevEui(t *testing.T) {
	_, err := decodeUplink([]byte(`{"object":{"x":1}}`))
	if err == nil {
		t.Error("expected error for missing devEui, got nil")
	}
}
