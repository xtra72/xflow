package lg

import (
	"encoding/hex"
	"testing"
)

func TestIsExtendedAttr(t *testing.T) {
	tests := []struct {
		attr byte
		want bool
	}{
		{0x00, false}, // 0x0_ → 2B
		{0x0F, false},
		{0x10, true}, // 0x1_ → 3B
		{0x1A, true},
		{0x40, false}, // 0x4_ → 2B
		{0x41, false},
		{0x50, true}, // 0x5_ → 3B
		{0x5F, true},
		{0x80, false}, // 0x8_ → 2B
		{0x8F, false},
		{0x90, true}, // 0x9_ → 3B
		{0x9A, true},
		{0xC0, false}, // 0xC_ → 2B
		{0xC1, false},
		{0xD0, true}, // 0xD_ → 3B
		{0xDF, true},
		{0xB0, false}, // 0xB_ → 2B
		{0xE0, false}, // 0xE_ → 2B
		{0x30, false}, // 0x3_ → 2B
	}

	for _, tt := range tests {
		got := isExtendedAttr(tt.attr)
		if got != tt.want {
			t.Errorf("isExtendedAttr(0x%02X) = %v, want %v", tt.attr, got, tt.want)
		}
	}
}

func TestParseRegPairs_Basic(t *testing.T) {
	// 2바이트 쌍: [18 41] [18 88] [29 C0]
	payload, _ := hex.DecodeString("1841188829C0")
	pairs := parseRegPairs(payload)

	if len(pairs) != 3 {
		t.Fatalf("쌍 개수 = %d, want 3", len(pairs))
	}

	// 쌍 1: reg=0x18, attr=0x41 (전원 ON)
	if pairs[0].Reg != 0x18 || pairs[0].Attr != 0x41 || pairs[0].Len != 2 {
		t.Errorf("쌍 0: reg=0x%02X attr=0x%02X len=%d", pairs[0].Reg, pairs[0].Attr, pairs[0].Len)
	}
	// 쌍 2: reg=0x18, attr=0x88 (용량 8)
	if pairs[1].Reg != 0x18 || pairs[1].Attr != 0x88 || pairs[1].Len != 2 {
		t.Errorf("쌍 1: reg=0x%02X attr=0x%02X len=%d", pairs[1].Reg, pairs[1].Attr, pairs[1].Len)
	}
	// 쌍 3: reg=0x29, attr=0xC0
	if pairs[2].Reg != 0x29 || pairs[2].Attr != 0xC0 || pairs[2].Len != 2 {
		t.Errorf("쌍 2: reg=0x%02X attr=0x%02X len=%d", pairs[2].Reg, pairs[2].Attr, pairs[2].Len)
	}
}

func TestParseRegPairs_Extended(t *testing.T) {
	// 확장 3바이트: [18 41] [18 90 3C] [29 C0]
	// 전원 ON + 압축기 60Hz(확장) + 고정
	payload, _ := hex.DecodeString("1841189 03C29C0")
	// hex.DecodeString은 공백을 허용하지 않으므로 수정
	payload, _ = hex.DecodeString("184118903C29C0")
	pairs := parseRegPairs(payload)

	if len(pairs) != 3 {
		t.Fatalf("쌍 개수 = %d, want 3", len(pairs))
	}

	// 쌍 2: reg=0x18, attr=0x90, ext=0x3C (60Hz)
	if pairs[1].Reg != 0x18 || pairs[1].Attr != 0x90 || pairs[1].Ext != 0x3C || pairs[1].Len != 3 {
		t.Errorf("확장 쌍: reg=0x%02X attr=0x%02X ext=0x%02X len=%d",
			pairs[1].Reg, pairs[1].Attr, pairs[1].Ext, pairs[1].Len)
	}
}

func TestDecodePayload_PowerOn(t *testing.T) {
	// [18 41] → 전원 ON
	payload, _ := hex.DecodeString("1841")
	d := DecodePayload(payload)

	if d == nil {
		t.Fatal("DecodePayload returned nil")
	}
	if d.Power == nil || *d.Power != "ON" {
		t.Errorf("Power = %v, want ON", d.Power)
	}
}

func TestDecodePayload_PowerOff(t *testing.T) {
	// [18 40] → 전원 OFF
	payload, _ := hex.DecodeString("1840")
	d := DecodePayload(payload)

	if d.Power == nil || *d.Power != "OFF" {
		t.Errorf("Power = %v, want OFF", d.Power)
	}
}

func TestDecodePayload_PowerState(t *testing.T) {
	// [60 C1] → 전원 ON (응답)
	payload, _ := hex.DecodeString("60C1")
	d := DecodePayload(payload)
	if d.PowerState == nil || *d.PowerState != "ON" {
		t.Errorf("PowerState = %v, want ON", d.PowerState)
	}

	// [60 C0] → 전원 OFF (응답)
	payload, _ = hex.DecodeString("60C0")
	d = DecodePayload(payload)
	if d.PowerState == nil || *d.PowerState != "OFF" {
		t.Errorf("PowerState = %v, want OFF", d.PowerState)
	}
}

func TestDecodePayload_SetTemp(t *testing.T) {
	tests := []struct {
		hex  string
		want float64
		desc string
	}{
		{"6480", 15, "15°C (V=0)"},
		{"648A", 25, "25°C (V=A=10)"},
		{"648F", 30, "30°C (V=F=15)"},
		{"6485", 20, "20°C (V=5)"},
	}

	for _, tt := range tests {
		payload, _ := hex.DecodeString(tt.hex)
		d := DecodePayload(payload)
		if d.SetTempC == nil || *d.SetTempC != tt.want {
			t.Errorf("%s: SetTempC = %v, want %v", tt.desc, d.SetTempC, tt.want)
		}
	}
}

func TestDecodePayload_FanSpeedMode(t *testing.T) {
	tests := []struct {
		extByte string
		fan     string
		mode    string
		desc    string
	}{
		{"14", "low", "heat", "약풍/난방"},
		{"24", "mid", "heat", "중풍/난방"},
		{"34", "high", "heat", "강풍/난방"},
		{"44", "turbo", "heat", "초강/난방"},
		{"54", "auto", "heat", "자동/난방"},
		{"50", "auto", "cool", "자동/냉방"},
		{"11", "low", "dry", "약풍/제습"},
		{"42", "turbo", "fan", "초강/송풍"},
		{"53", "auto", "auto", "자동/자동"},
		{"20", "mid", "cool", "중풍/냉방"},
	}

	for _, tt := range tests {
		payload, _ := hex.DecodeString("6450" + tt.extByte)
		d := DecodePayload(payload)
		if d.FanSpeed == nil || *d.FanSpeed != tt.fan {
			t.Errorf("%s: FanSpeed = %v, want %s", tt.desc, d.FanSpeed, tt.fan)
		}
		if d.Mode == nil || *d.Mode != tt.mode {
			t.Errorf("%s: Mode = %v, want %s", tt.desc, d.Mode, tt.mode)
		}
	}
}

func TestDecodePayload_CompressorCap(t *testing.T) {
	// [18 88] → 용량 8
	payload, _ := hex.DecodeString("1888")
	d := DecodePayload(payload)

	if d.CompressorCap == nil || *d.CompressorCap != 8 {
		t.Errorf("CompressorCap = %v, want 8", d.CompressorCap)
	}
}

func TestDecodePayload_CompressorHz(t *testing.T) {
	// [18 90 3C] → 60Hz
	payload, _ := hex.DecodeString("18903C")
	d := DecodePayload(payload)

	if d.CompressorHz == nil || *d.CompressorHz != 60 {
		t.Errorf("CompressorHz = %v, want 60", d.CompressorHz)
	}
}

func TestDecodePayload_IndoorTemp(t *testing.T) {
	// [74 90 2C] → ext=0x2C=44, 44/2=22.0°C
	payload, _ := hex.DecodeString("74902C")
	d := DecodePayload(payload)

	if d.IndoorTempC == nil || *d.IndoorTempC != 22.0 {
		t.Errorf("IndoorTempC = %v, want 22.0", d.IndoorTempC)
	}
}

func TestDecodePayload_PipeTemp1(t *testing.T) {
	// [61 D0 64] → ext=0x64=100, 100/2=50.0°C
	payload, _ := hex.DecodeString("61D064")
	d := DecodePayload(payload)

	if d.PipeTemp1C == nil || *d.PipeTemp1C != 50.0 {
		t.Errorf("PipeTemp1C = %v, want 50.0", d.PipeTemp1C)
	}
}

func TestDecodePayload_ValveOpen(t *testing.T) {
	// [62 41] → 밸브 열림
	payload, _ := hex.DecodeString("6241")
	d := DecodePayload(payload)

	if d.ValveOpen == nil || *d.ValveOpen != true {
		t.Errorf("ValveOpen = %v, want true", d.ValveOpen)
	}

	// [62 40] → 밸브 닫힘
	payload, _ = hex.DecodeString("6240")
	d = DecodePayload(payload)

	if d.ValveOpen == nil || *d.ValveOpen != false {
		t.Errorf("ValveOpen = %v, want false", d.ValveOpen)
	}
}

func TestDecodePayload_FanSpeedResp(t *testing.T) {
	tests := []struct {
		hex  string
		want int
		desc string
	}{
		{"7101", 1, "약풍"},
		{"7102", 2, "중풍"},
		{"7105", 5, "자동"},
		{"7100", 0, "미설정"},
	}

	for _, tt := range tests {
		payload, _ := hex.DecodeString(tt.hex)
		d := DecodePayload(payload)
		if d.FanSpeedResp == nil || *d.FanSpeedResp != tt.want {
			t.Errorf("%s: FanSpeedResp = %v, want %d", tt.desc, d.FanSpeedResp, tt.want)
		}
	}
}

func TestDecodePayload_OutdoorActive(t *testing.T) {
	// [10 C1] → 활성
	payload, _ := hex.DecodeString("10C1")
	d := DecodePayload(payload)
	if d.OutdoorActive == nil || *d.OutdoorActive != true {
		t.Errorf("OutdoorActive = %v, want true", d.OutdoorActive)
	}

	// [10 C0] → 비활성
	payload, _ = hex.DecodeString("10C0")
	d = DecodePayload(payload)
	if d.OutdoorActive == nil || *d.OutdoorActive != false {
		t.Errorf("OutdoorActive = %v, want false", d.OutdoorActive)
	}
}

func TestDecodePayload_HeatDemand(t *testing.T) {
	// [11 01] → 난방 요구 ON
	payload, _ := hex.DecodeString("1101")
	d := DecodePayload(payload)
	if d.HeatDemand == nil || *d.HeatDemand != true {
		t.Errorf("HeatDemand = %v, want true", d.HeatDemand)
	}

	// [11 00] → 난방 요구 OFF
	payload, _ = hex.DecodeString("1100")
	d = DecodePayload(payload)
	if d.HeatDemand == nil || *d.HeatDemand != false {
		t.Errorf("HeatDemand = %v, want false", d.HeatDemand)
	}
}

func TestDecodePayload_CompressorRun(t *testing.T) {
	// [12 41] → 압축기 운전 ON
	payload, _ := hex.DecodeString("1241")
	d := DecodePayload(payload)
	if d.CompressorRun == nil || *d.CompressorRun != true {
		t.Errorf("CompressorRun = %v, want true", d.CompressorRun)
	}

	// [12 40] → 압축기 운전 OFF
	payload, _ = hex.DecodeString("1240")
	d = DecodePayload(payload)
	if d.CompressorRun == nil || *d.CompressorRun != false {
		t.Errorf("CompressorRun = %v, want false", d.CompressorRun)
	}
}

func TestDecodePayload_RefrigerantOn(t *testing.T) {
	// [1A C1] → 냉매 회로 운전 ON
	payload, _ := hex.DecodeString("1AC1")
	d := DecodePayload(payload)
	if d.RefrigerantOn == nil || *d.RefrigerantOn != true {
		t.Errorf("RefrigerantOn = %v, want true", d.RefrigerantOn)
	}

	// [1A C0] → 냉매 회로 운전 OFF
	payload, _ = hex.DecodeString("1AC0")
	d = DecodePayload(payload)
	if d.RefrigerantOn == nil || *d.RefrigerantOn != false {
		t.Errorf("RefrigerantOn = %v, want false", d.RefrigerantOn)
	}
}

func TestDecodePayload_OpMode(t *testing.T) {
	tests := []struct {
		hex  string
		want string
	}{
		{"13C0", "normal"},
		{"13C1", "heating"},
		{"13C3", "defrost"},
		{"13C6", "defrost-transition"},
	}

	for _, tt := range tests {
		payload, _ := hex.DecodeString(tt.hex)
		d := DecodePayload(payload)
		if d.OpMode == nil || *d.OpMode != tt.want {
			t.Errorf("OpMode(%s) = %v, want %s", tt.hex, d.OpMode, tt.want)
		}
	}
}

// TestDecodePayload_ControlCommand 는 프로토콜 분석 문서의 실제 제어 명령을 테스트한다.
// 문서 7.2: "전원 ON + 용량 8단계" = 18,41 18,88 29,c0
func TestDecodePayload_ControlCommand(t *testing.T) {
	payload, _ := hex.DecodeString("184118882 9C0")
	// hex.DecodeString 공백 불가
	payload, _ = hex.DecodeString("184118882 9C0")
	payload, _ = hex.DecodeString("184118882" + "9C0")
	// 올바른 hex
	payload, _ = hex.DecodeString("18411888" + "29C0")

	d := DecodePayload(payload)

	if d.Power == nil || *d.Power != "ON" {
		t.Errorf("Power = %v, want ON", d.Power)
	}
	if d.CompressorCap == nil || *d.CompressorCap != 8 {
		t.Errorf("CompressorCap = %v, want 8", d.CompressorCap)
	}
	if len(d.Pairs) != 3 {
		t.Errorf("Pairs 개수 = %d, want 3", len(d.Pairs))
	}
}

// TestDecodePayload_ExtendedControlCommand 는 확장 Hz 제어 명령을 테스트한다.
// 문서 7.2: "전원 ON + 60Hz(확장)" = 18,41 18,90,3C 29,c0
func TestDecodePayload_ExtendedControlCommand(t *testing.T) {
	payload, _ := hex.DecodeString("184118903C29C0")
	d := DecodePayload(payload)

	if d.Power == nil || *d.Power != "ON" {
		t.Errorf("Power = %v, want ON", d.Power)
	}
	if d.CompressorHz == nil || *d.CompressorHz != 60 {
		t.Errorf("CompressorHz = %v, want 60", d.CompressorHz)
	}
	if len(d.Pairs) != 3 {
		t.Errorf("Pairs 개수 = %d, want 3", len(d.Pairs))
	}
}

// TestDecodePayload_ModeChangeEvent 는 모드 변경 이벤트를 테스트한다.
// 문서 12.5: 난방→송풍 전환 시 payload 일부: 64,50,42 64,83 71,01
func TestDecodePayload_ModeChangeEvent(t *testing.T) {
	payload, _ := hex.DecodeString("6450426483" + "7101")
	d := DecodePayload(payload)

	if d.FanSpeed == nil || *d.FanSpeed != "turbo" {
		t.Errorf("FanSpeed = %v, want turbo", d.FanSpeed)
	}
	if d.Mode == nil || *d.Mode != "fan" {
		t.Errorf("Mode = %v, want fan", d.Mode)
	}
	// 64,83 → 설정온도 = 3+15 = 18°C
	if d.SetTempC == nil || *d.SetTempC != 18 {
		t.Errorf("SetTempC = %v, want 18", d.SetTempC)
	}
	// 71,01 → 풍속응답 1 (약풍)
	if d.FanSpeedResp == nil || *d.FanSpeedResp != 1 {
		t.Errorf("FanSpeedResp = %v, want 1", d.FanSpeedResp)
	}
}

func TestDecodePayload_Empty(t *testing.T) {
	d := DecodePayload(nil)
	if d != nil {
		t.Error("DecodePayload(nil) should return nil")
	}

	d = DecodePayload([]byte{})
	if d != nil {
		t.Error("DecodePayload([]) should return nil")
	}
}

func TestDecodePayload_SingleByte(t *testing.T) {
	// 1바이트 잔여는 무시
	payload := []byte{0x18}
	d := DecodePayload(payload)
	if d == nil {
		t.Fatal("DecodePayload returned nil")
	}
	if len(d.Pairs) != 0 {
		t.Errorf("Pairs 개수 = %d, want 0", len(d.Pairs))
	}
}

func TestHexByte(t *testing.T) {
	tests := []struct {
		b    byte
		want string
	}{
		{0x00, "00"},
		{0x18, "18"},
		{0x64, "64"},
		{0xFF, "ff"},
		{0xAB, "ab"},
	}

	for _, tt := range tests {
		got := hexByte(tt.b)
		if got != tt.want {
			t.Errorf("hexByte(0x%02X) = %q, want %q", tt.b, got, tt.want)
		}
	}
}

// TestDecodePayload_StatusRequestPlen26 은 프로토콜 예시 프레임의 페이로드를 테스트한다.
// 프레임 hex에서 추출: 18,40(OFF) + 18,80(용량0)
func TestDecodePayload_StatusRequestPlen26(t *testing.T) {
	payload, _ := hex.DecodeString("110010C018001AC01300134013C016001840188029C01DC0")
	d := DecodePayload(payload)

	if d == nil {
		t.Fatal("DecodePayload returned nil")
	}

	// 12개 쌍이어야 함
	if len(d.Pairs) != 12 {
		t.Errorf("Pairs 개수 = %d, want 12", len(d.Pairs))
	}

	// 0x10 C0 → 실외기 비활성
	if d.OutdoorActive == nil || *d.OutdoorActive != false {
		t.Errorf("OutdoorActive = %v, want false", d.OutdoorActive)
	}

	// 0x13 C0 → 일반 모드
	if d.OpMode == nil || *d.OpMode != "normal" {
		t.Errorf("OpMode = %v, want normal", d.OpMode)
	}

	// 0x18 40 → 전원 OFF
	if d.Power == nil || *d.Power != "OFF" {
		t.Errorf("Power = %v, want OFF", d.Power)
	}

	// 0x18 80 → 용량 0 (프레임 예시에서 0x80 = lo nibble 0)
	if d.CompressorCap == nil || *d.CompressorCap != 0 {
		t.Errorf("CompressorCap = %d, want 0", *d.CompressorCap)
	}
}

// TestDecodePayload_PipeTemp2 는 배관 온도 2 (0x62, 0x1_+ext) 해석을 테스트한다.
func TestDecodePayload_PipeTemp2(t *testing.T) {
	// [62 10 64] → ext=0x64=100, 100/2=50.0°C
	payload, _ := hex.DecodeString("621064")
	d := DecodePayload(payload)

	if d.PipeTemp2C == nil || *d.PipeTemp2C != 50.0 {
		t.Errorf("PipeTemp2C = %v, want 50.0", d.PipeTemp2C)
	}
}

// TestDecodePayload_FanMotorHz 는 실내기 팬모터 주파수 (0x62, 0xD0+ext) 해석을 테스트한다.
func TestDecodePayload_FanMotorHz(t *testing.T) {
	// [62 D0 5A] → ext=0x5A=90, 90Hz
	payload, _ := hex.DecodeString("62D05A")
	d := DecodePayload(payload)

	if d.FanMotorHz == nil || *d.FanMotorHz != 90 {
		t.Errorf("FanMotorHz = %v, want 90", d.FanMotorHz)
	}
}
