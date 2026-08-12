package chirpstack

import (
	"encoding/hex"
	"testing"
)

// TestEncodeDownlink_WS301Golden 은 WS301 코덱의 golden-vector 인코딩을 검증한다
// (REQ-M2-01/02, AC-1).
//
// 기대 바이트는 Milesight WS301 User Guide V1.4 Ch.6 Downlink Commands(p.31-32)
// 및 공식 Milesight-IoT encoder 에서 유도한다. reboot / set_report_interval 은
// 유저 가이드 대조로 확인된 값이고, query_device_status 는 공식 encoder 에만 존재한다
// (codec_ws301.go 의 @MX:DEBT 참조).
func TestEncodeDownlink_WS301Golden(t *testing.T) {
	tests := []struct {
		name      string
		cmd       DownlinkCommand
		wantHex   string
		wantPort  uint8
		wantConfd bool
	}{
		{
			name:     "reboot",
			cmd:      DownlinkCommand{DevEui: "24e124141d180806", Name: "reboot"},
			wantHex:  "ff10ff",
			wantPort: 85,
		},
		{
			name: "set_report_interval 1200s (0x04b0 리틀엔디언)",
			cmd: DownlinkCommand{
				DevEui: "24e124141d180806",
				Name:   "set_report_interval",
				Params: map[string]any{"interval": 1200},
			},
			wantHex:  "ff03b004",
			wantPort: 85,
		},
		{
			name: "set_report_interval JSON float64 입력",
			cmd: DownlinkCommand{
				DevEui: "24e124141d180806",
				Name:   "set_report_interval",
				Params: map[string]any{"interval": float64(60)},
			},
			wantHex:  "ff033c00",
			wantPort: 85,
		},
		{
			name: "set_report_interval 상한 64800s (0xfd20 리틀엔디언)",
			cmd: DownlinkCommand{
				DevEui: "24e124141d180806",
				Name:   "set_report_interval",
				Params: map[string]any{"interval": 64800},
			},
			wantHex:  "ff0320fd",
			wantPort: 85,
		},
		{
			name:     "query_device_status",
			cmd:      DownlinkCommand{DevEui: "24e124141d180806", Name: "query_device_status"},
			wantHex:  "ff28ff",
			wantPort: 85,
		},
		{
			name: "confirmed 통과 (AC-1b)",
			cmd: DownlinkCommand{
				DevEui:    "24e124141d180806",
				Name:      "reboot",
				Confirmed: true,
			},
			wantHex:   "ff10ff",
			wantPort:  85,
			wantConfd: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fPort, data, confirmed, err := EncodeDownlink(ws301ProfileName, tt.cmd)
			if err != nil {
				t.Fatalf("EncodeDownlink: %v", err)
			}
			if fPort != tt.wantPort {
				t.Errorf("fPort = %d, want %d", fPort, tt.wantPort)
			}
			if got := hex.EncodeToString(data); got != tt.wantHex {
				t.Errorf("data = %s, want %s", got, tt.wantHex)
			}
			if confirmed != tt.wantConfd {
				t.Errorf("confirmed = %v, want %v", confirmed, tt.wantConfd)
			}
		})
	}
}

// TestEncodeDownlink_Rejections 는 미등록 프로파일 / 미지 command / 범위 밖 파라미터가
// 에러로 거부되는지 검증한다 (REQ-M2-04, AC-3) — 일반 passthrough 다운링크 금지.
func TestEncodeDownlink_Rejections(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		cmd     DownlinkCommand
	}{
		{
			name:    "미등록 deviceProfile",
			profile: "EM300-TH",
			cmd:     DownlinkCommand{DevEui: "dev", Name: "reboot"},
		},
		{
			name:    "빈 deviceProfile",
			profile: "",
			cmd:     DownlinkCommand{DevEui: "dev", Name: "reboot"},
		},
		{
			name:    "코덱이 모르는 command",
			profile: ws301ProfileName,
			cmd:     DownlinkCommand{DevEui: "dev", Name: "open_buzzer"},
		},
		{
			name:    "빈 command",
			profile: ws301ProfileName,
			cmd:     DownlinkCommand{DevEui: "dev", Name: ""},
		},
		{
			name:    "interval 파라미터 누락",
			profile: ws301ProfileName,
			cmd:     DownlinkCommand{DevEui: "dev", Name: "set_report_interval"},
		},
		{
			name:    "interval 하한 미만(59s)",
			profile: ws301ProfileName,
			cmd: DownlinkCommand{
				DevEui: "dev", Name: "set_report_interval",
				Params: map[string]any{"interval": 59},
			},
		},
		{
			name:    "interval 상한 초과(64801s)",
			profile: ws301ProfileName,
			cmd: DownlinkCommand{
				DevEui: "dev", Name: "set_report_interval",
				Params: map[string]any{"interval": 64801},
			},
		},
		{
			name:    "interval 비숫자 타입",
			profile: ws301ProfileName,
			cmd: DownlinkCommand{
				DevEui: "dev", Name: "set_report_interval",
				Params: map[string]any{"interval": "1200"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, data, _, err := EncodeDownlink(tt.profile, tt.cmd)
			if err == nil {
				t.Fatal("거부되어야 하는 입력이 인코딩되었다 (generic passthrough 금지)")
			}
			if len(data) != 0 {
				t.Errorf("거부 시 data 는 비어야 한다, got %x", data)
			}
		})
	}
}

// TestDownlinkCodecRegistry 는 레지스트리 등록/조회가 deviceProfile 별로 동작하고
// 확장 가능한지 검증한다 (REQ-M2-01).
func TestDownlinkCodecRegistry(t *testing.T) {
	if _, ok := LookupDownlinkCodec(ws301ProfileName); !ok {
		t.Fatalf("v1 seed 코덱 %q 가 등록되어 있어야 한다", ws301ProfileName)
	}
	if _, ok := LookupDownlinkCodec("NO-SUCH-PROFILE"); ok {
		t.Error("미등록 프로파일 조회는 ok=false 여야 한다")
	}

	// 확장점 검증: 임시 코덱 등록 후 조회.
	const testProfile = "TEST-PROFILE-002"
	RegisterDownlinkCodec(testProfile, downlinkCodecFunc(func(cmd DownlinkCommand) (uint8, []byte, bool, error) {
		return 7, []byte{0x01}, cmd.Confirmed, nil
	}))
	t.Cleanup(func() { unregisterDownlinkCodecForTest(testProfile) })

	fPort, data, confirmed, err := EncodeDownlink(testProfile, DownlinkCommand{Name: "x", Confirmed: true})
	if err != nil {
		t.Fatalf("EncodeDownlink(확장 코덱): %v", err)
	}
	if fPort != 7 || len(data) != 1 || data[0] != 0x01 || !confirmed {
		t.Errorf("확장 코덱 결과 = (%d, %x, %v)", fPort, data, confirmed)
	}
}

// TestRegisterDownlinkCodec_IgnoresInvalid 는 빈 프로파일명/nil 코덱 등록이 무시되는지
// 검증한다(레지스트리 오염 방지).
func TestRegisterDownlinkCodec_IgnoresInvalid(t *testing.T) {
	RegisterDownlinkCodec("", ws301Codec{})
	if _, ok := LookupDownlinkCodec(""); ok {
		t.Error("빈 프로파일명은 등록되지 않아야 한다")
	}

	RegisterDownlinkCodec("NIL-CODEC-PROFILE", nil)
	if _, ok := LookupDownlinkCodec("NIL-CODEC-PROFILE"); ok {
		t.Error("nil 코덱은 등록되지 않아야 한다")
	}
}

// TestIntParam_NumericTypes 는 params 정수 추출이 Go 정수/JSON 실수 표현을 모두
// 수용하고 비정수·비숫자를 거부하는지 검증한다.
func TestIntParam_NumericTypes(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		want   int
		wantOK bool
	}{
		{name: "int", value: 600, want: 600, wantOK: true},
		{name: "int32", value: int32(600), want: 600, wantOK: true},
		{name: "int64", value: int64(600), want: 600, wantOK: true},
		{name: "float64 정수값", value: float64(600), want: 600, wantOK: true},
		{name: "float32 정수값", value: float32(600), want: 600, wantOK: true},
		{name: "float64 소수값 거부", value: 600.5},
		{name: "float32 소수값 거부", value: float32(600.5)},
		{name: "문자열 거부", value: "600"},
		{name: "bool 거부", value: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := intParam(map[string]any{"interval": tt.value}, "interval")
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("value = %d, want %d", got, tt.want)
			}
		})
	}

	// 키 부재.
	if _, ok := intParam(map[string]any{}, "interval"); ok {
		t.Error("키 부재는 ok=false 여야 한다")
	}
	// nil 맵.
	if _, ok := intParam(nil, "interval"); ok {
		t.Error("nil params 는 ok=false 여야 한다")
	}
}

// downlinkCodecFunc 는 테스트에서 함수를 DownlinkCodec 으로 어댑팅한다.
type downlinkCodecFunc func(DownlinkCommand) (uint8, []byte, bool, error)

func (f downlinkCodecFunc) Encode(cmd DownlinkCommand) (uint8, []byte, bool, error) {
	return f(cmd)
}
