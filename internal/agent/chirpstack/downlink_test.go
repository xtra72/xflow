package chirpstack

import (
	"encoding/json"
	"testing"
)

// TestBuildDownlinkTopic 는 ChirpStack 다운링크 명령 토픽 구성을 검증한다 (REQ-M2-03).
func TestBuildDownlinkTopic(t *testing.T) {
	const (
		appID  = "96b4d719-f23f-40aa-9f94-a0f2d0354342"
		devEui = "24e124141d180806"
	)
	want := "application/" + appID + "/device/" + devEui + "/command/down"
	if got := BuildDownlinkTopic(appID, devEui); got != want {
		t.Errorf("BuildDownlinkTopic = %q, want %q", got, want)
	}
}

// TestBuildDownlinkPayload 는 다운링크 페이로드 JSON 계약을 검증한다 (REQ-M2-03).
//
// 계약: {"devEui","confirmed","fPort","data"} — data 는 base64(StdEncoding).
func TestBuildDownlinkPayload(t *testing.T) {
	tests := []struct {
		name      string
		devEui    string
		confirmed bool
		fPort     uint8
		data      []byte
		wantData  string
	}{
		{
			name:     "reboot TLV",
			devEui:   "24e124141d180806",
			fPort:    85,
			data:     []byte{0xff, 0x10, 0xff},
			wantData: "/xD/",
		},
		{
			name:      "confirmed=true 통과 + 4바이트 TLV",
			devEui:    "24e124141d180806",
			confirmed: true,
			fPort:     85,
			data:      []byte{0xff, 0x03, 0xb0, 0x04},
			wantData:  "/wOwBA==",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := BuildDownlinkPayload(tt.devEui, tt.confirmed, tt.fPort, tt.data)
			if err != nil {
				t.Fatalf("BuildDownlinkPayload: %v", err)
			}

			var got struct {
				DevEui    string `json:"devEui"`
				Confirmed bool   `json:"confirmed"`
				FPort     uint8  `json:"fPort"`
				Data      string `json:"data"`
			}
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("페이로드가 유효한 JSON 이 아니다: %v (%s)", err, raw)
			}
			if got.DevEui != tt.devEui {
				t.Errorf("devEui = %q, want %q", got.DevEui, tt.devEui)
			}
			if got.Confirmed != tt.confirmed {
				t.Errorf("confirmed = %v, want %v", got.Confirmed, tt.confirmed)
			}
			if got.FPort != tt.fPort {
				t.Errorf("fPort = %d, want %d", got.FPort, tt.fPort)
			}
			if got.Data != tt.wantData {
				t.Errorf("data = %q, want %q", got.Data, tt.wantData)
			}
		})
	}
}

// TestBuildDownlinkPayload_Rejections 는 빈 devEui / 빈 data 를 거부하는지 검증한다.
func TestBuildDownlinkPayload_Rejections(t *testing.T) {
	if _, err := BuildDownlinkPayload("", false, 85, []byte{0xff}); err == nil {
		t.Error("빈 devEui 는 거부되어야 한다")
	}
	if _, err := BuildDownlinkPayload("dev", false, 85, nil); err == nil {
		t.Error("빈 data 는 거부되어야 한다")
	}
}

// TestDownlinkTarget_CachedAfterUplink 는 최초 업링크가 applicationId 를 캐시한 뒤에만
// 다운링크 대상이 조회되는지 검증한다 (REQ-M2-05, AC-3b, R5 순서 제약).
func TestDownlinkTarget_CachedAfterUplink(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "dltarget-cs")

	const devEui = "24e124141d180806"

	// 업링크 이전: 미캐시 → ok=false.
	if _, _, ok := a.DownlinkTarget(devEui); ok {
		t.Fatal("업링크 이전에는 applicationId 가 캐시되지 않아야 한다")
	}

	// 실측 픽스처 업링크 1건 수신.
	a.handleUplink(loadRawUplink(t), "application/x/device/y/event/up")

	appID, profile, ok := a.DownlinkTarget(devEui)
	if !ok {
		t.Fatal("업링크 이후에는 다운링크 대상이 조회되어야 한다")
	}
	if appID != "96b4d719-f23f-40aa-9f94-a0f2d0354342" {
		t.Errorf("applicationID = %q", appID)
	}
	if profile != "WS301" {
		t.Errorf("deviceProfileName = %q, want WS301", profile)
	}

	// 빈 devEui 는 항상 미조회.
	if _, _, ok := a.DownlinkTarget(""); ok {
		t.Error("빈 devEui 는 ok=false 여야 한다")
	}
	// 알 수 없는 devEui 도 미조회.
	if _, _, ok := a.DownlinkTarget("0000000000000000"); ok {
		t.Error("미지 devEui 는 ok=false 여야 한다")
	}
}
