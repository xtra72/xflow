package lg

import (
	"bytes"
	"errors"
	"testing"
)

func TestEncodePowerPayload_On(t *testing.T) {
	got := encodePowerPayload(true, 0, 0x00)
	// 10,C1 = 실외기 활성, 13,C0 = normal, 18,41 = 전원 ON, 18,80 = 용량 0, 29,C0 = 고정
	want := []byte{0x10, 0xC1, 0x13, 0xC0, 0x18, 0x41, 0x18, 0x80, 0x29, 0xC0}
	if !bytes.Equal(got, want) {
		t.Errorf("power ON: got %X, want %X", got, want)
	}
}

func TestEncodePowerPayload_Off(t *testing.T) {
	got := encodePowerPayload(false, 0, 0x00)
	// 10,C0 = 실외기 비활성, 13,C0 = normal, 18,40 = 전원 OFF, 18,80 = 용량 0, 29,C0 = 고정
	want := []byte{0x10, 0xC0, 0x13, 0xC0, 0x18, 0x40, 0x18, 0x80, 0x29, 0xC0}
	if !bytes.Equal(got, want) {
		t.Errorf("power OFF: got %X, want %X", got, want)
	}
}

func TestEncodePowerPayload_OnWithCompressor(t *testing.T) {
	got := encodePowerPayload(true, 10, 0x01) // heating mode
	want := []byte{0x10, 0xC1, 0x13, 0xC1, 0x18, 0x41, 0x18, 0x8A, 0x29, 0xC0}
	if !bytes.Equal(got, want) {
		t.Errorf("power ON comp=10 heating: got %X, want %X", got, want)
	}
}

func TestEncodeTemperaturePayload_Normal(t *testing.T) {
	got, err := encodeTemperaturePayload(25.0)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x64, 0x8A}
	if !bytes.Equal(got, want) {
		t.Errorf("temp 25: got %X, want %X", got, want)
	}
}

func TestEncodeTemperaturePayload_Min(t *testing.T) {
	got, err := encodeTemperaturePayload(15.0)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x64, 0x80}
	if !bytes.Equal(got, want) {
		t.Errorf("temp 15: got %X, want %X", got, want)
	}
}

func TestEncodeTemperaturePayload_Max(t *testing.T) {
	got, err := encodeTemperaturePayload(30.0)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x64, 0x8F}
	if !bytes.Equal(got, want) {
		t.Errorf("temp 30: got %X, want %X", got, want)
	}
}

func TestEncodeTemperaturePayload_OutOfRange(t *testing.T) {
	_, err := encodeTemperaturePayload(31.0)
	if err == nil {
		t.Fatal("expected error for temp 31")
	}
	if !errors.Is(err, ErrHvacr02TemperatureOutOfRange) {
		t.Errorf("wrong error type: %v", err)
	}

	_, err = encodeTemperaturePayload(14.0)
	if err == nil {
		t.Fatal("expected error for temp 14")
	}
}

func TestEncodeTemperaturePayload_Decimal(t *testing.T) {
	got, err := encodeTemperaturePayload(25.7)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x64, 0x8A} // 소수점 버림 → 25
	if !bytes.Equal(got, want) {
		t.Errorf("temp 25.7: got %X, want %X", got, want)
	}
}

func TestEncodeFanModePayload(t *testing.T) {
	tests := []struct {
		fan, mode int
		want      []byte
	}{
		{3, 0, []byte{0x64, 0x50, 0x30}}, // high + cooling
		{5, 4, []byte{0x64, 0x50, 0x54}}, // auto + heating
		{2, 1, []byte{0x64, 0x50, 0x21}}, // medium + dehumidify
		{1, 3, []byte{0x64, 0x50, 0x13}}, // low + auto
	}
	for _, tt := range tests {
		got := encodeFanModePayload(tt.fan, tt.mode)
		if !bytes.Equal(got, tt.want) {
			t.Errorf("fan=%d mode=%d: got %X, want %X", tt.fan, tt.mode, got, tt.want)
		}
	}
}

func TestModeCodeToOpMode(t *testing.T) {
	tests := []struct {
		modeCode int
		want     byte
	}{
		{0, 0x00}, // cooling → normal
		{1, 0x00}, // dehumidify → normal
		{2, 0x00}, // fan → normal
		{3, 0x00}, // auto → normal
		{4, 0x01}, // heating
	}
	for _, tt := range tests {
		got := modeCodeToOpMode(tt.modeCode)
		if got != tt.want {
			t.Errorf("modeCodeToOpMode(%d): got 0x%02X, want 0x%02X", tt.modeCode, got, tt.want)
		}
	}
}

func TestLookupFanSpeedCode(t *testing.T) {
	for name, want := range hvacr02FanSpeedCodes {
		got, err := lookupFanSpeedCode(name)
		if err != nil {
			t.Errorf("lookupFanSpeedCode(%q): %v", name, err)
		}
		if got != want {
			t.Errorf("lookupFanSpeedCode(%q): got %d, want %d", name, got, want)
		}
	}
	_, err := lookupFanSpeedCode("supersonic")
	if !errors.Is(err, ErrHvacr02InvalidFanSpeed) {
		t.Errorf("invalid fan_speed: got %v, want ErrHvacr02InvalidFanSpeed", err)
	}
}

func TestLookupModeCode(t *testing.T) {
	for name, want := range hvacr02ModeCodes {
		got, err := lookupModeCode(name)
		if err != nil {
			t.Errorf("lookupModeCode(%q): %v", name, err)
		}
		if got != want {
			t.Errorf("lookupModeCode(%q): got %d, want %d", name, got, want)
		}
	}
	_, err := lookupModeCode("turbo")
	if !errors.Is(err, ErrHvacr02InvalidMode) {
		t.Errorf("invalid mode: got %v, want ErrHvacr02InvalidMode", err)
	}
}

func TestAppendPayloadCRC(t *testing.T) {
	// 실제 캡처 데이터로 검증: CRC 입력 = CMD(2B) + SEQ0(1B) + PLEN(1B) + register_data
	tests := []struct {
		name string
		cmd  [2]byte
		seq0 byte
		data []byte
		want []byte // data + CRC(2B)
	}{
		{
			"ctrl frame seq0=AF",
			[2]byte{0x02, 0x01}, 0xAF,
			[]byte{0x18, 0x41, 0x18, 0x8A, 0x29, 0xC0},
			[]byte{0x18, 0x41, 0x18, 0x8A, 0x29, 0xC0, 0x52, 0x1D},
		},
		{
			"ctrl frame seq0=B2",
			[2]byte{0x02, 0x01}, 0xB2,
			[]byte{0x18, 0x41, 0x18, 0x89, 0x29, 0xC0},
			[]byte{0x18, 0x41, 0x18, 0x89, 0x29, 0xC0, 0x60, 0x9D},
		},
		{
			"power OFF seq0=4B",
			[2]byte{0x02, 0x01}, 0x4B,
			[]byte{0x18, 0x40, 0x18, 0x80, 0x29, 0xC0},
			[]byte{0x18, 0x40, 0x18, 0x80, 0x29, 0xC0, 0x40, 0xD9},
		},
		{
			"unit->ctrl temp change seq0=58",
			[2]byte{0x02, 0x01}, 0x58,
			[]byte{0x64, 0x50, 0x14, 0x71, 0x01},
			[]byte{0x64, 0x50, 0x14, 0x71, 0x01, 0x61, 0xC4},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendPayloadCRC(tt.cmd, tt.seq0, tt.data)
			if !bytes.Equal(got, tt.want) {
				t.Errorf("got %X, want %X", got, tt.want)
			}
		})
	}
}

func TestBuildControlPayload_Multiple(t *testing.T) {
	params := map[string]interface{}{
		"power":       true,
		"target_temperature": float64(24),
		"fan_speed":   "medium",
		"mode":        "cooling",
	}
	got, err := buildControlPayload(params, hvacr02DefaultFanCode, hvacr02DefaultModeCode)
	if err != nil {
		t.Fatal(err)
	}

	// 전원 ON: 10 C1 13 C0 18 41 18 80 29 C0 (cooling → opMode=0x00)
	// 온도 24: 64 89 (0x80 | 9)
	// 풍량+모드: 64 50 20 (medium=2, cooling=0)
	want := []byte{0x10, 0xC1, 0x13, 0xC0, 0x18, 0x41, 0x18, 0x80, 0x29, 0xC0, 0x64, 0x89, 0x64, 0x50, 0x20}
	if !bytes.Equal(got, want) {
		t.Errorf("multiple: got %X, want %X", got, want)
	}
}

func TestBuildControlPayload_OnlyTemperature(t *testing.T) {
	params := map[string]interface{}{
		"target_temperature": float64(20),
	}
	got, err := buildControlPayload(params, hvacr02DefaultFanCode, hvacr02DefaultModeCode)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x64, 0x85} // 0x80 | 5
	if !bytes.Equal(got, want) {
		t.Errorf("only temp: got %X, want %X", got, want)
	}
}

func TestBuildControlPayload_OnlyPower(t *testing.T) {
	params := map[string]interface{}{
		"power": false,
	}
	got, err := buildControlPayload(params, hvacr02DefaultFanCode, hvacr02DefaultModeCode)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x10, 0xC0, 0x13, 0xC0, 0x18, 0x40, 0x18, 0x80, 0x29, 0xC0}
	if !bytes.Equal(got, want) {
		t.Errorf("only power off: got %X, want %X", got, want)
	}
}

// ---------------------------------------------------------------------------
// 서모스탯 사칭 모드 페이로드 테스트
// ---------------------------------------------------------------------------

func TestEncodeThermostatPowerPayload_On(t *testing.T) {
	// Power ON: 62,41 + 64,50,XY(fan+mode) + 64,8V(temp)
	// fan=2(medium), mode=0(cooling), temp=25°C → offset=10 → 0x8A
	got := encodeThermostatPowerPayload(true, 2, 0, 25.0)
	want := []byte{0x62, 0x41, 0x64, 0x50, 0x20, 0x64, 0x8A}
	if !bytes.Equal(got, want) {
		t.Errorf("thermostat power ON: got %X, want %X", got, want)
	}
}

func TestEncodeThermostatPowerPayload_Off(t *testing.T) {
	// Power OFF: 62,40 만 전송
	got := encodeThermostatPowerPayload(false, 2, 0, 25.0)
	want := []byte{0x62, 0x40}
	if !bytes.Equal(got, want) {
		t.Errorf("thermostat power OFF: got %X, want %X", got, want)
	}
}

func TestEncodeThermostatPowerPayload_OnAutoHeating(t *testing.T) {
	// fan=5(auto), mode=4(heating), temp=30°C → offset=15 → 0x8F
	got := encodeThermostatPowerPayload(true, 5, 4, 30.0)
	want := []byte{0x62, 0x41, 0x64, 0x50, 0x54, 0x64, 0x8F}
	if !bytes.Equal(got, want) {
		t.Errorf("thermostat power ON auto/heating: got %X, want %X", got, want)
	}
}

func TestEncodeThermostatTempPayload(t *testing.T) {
	tests := []struct {
		tempC float64
		want  []byte
	}{
		{15.0, []byte{0x64, 0x80}},
		{25.0, []byte{0x64, 0x8A}},
		{30.0, []byte{0x64, 0x8F}},
	}
	for _, tt := range tests {
		got, err := encodeThermostatTempPayload(tt.tempC)
		if err != nil {
			t.Fatalf("temp %.0f: %v", tt.tempC, err)
		}
		if !bytes.Equal(got, tt.want) {
			t.Errorf("temp %.0f: got %X, want %X", tt.tempC, got, tt.want)
		}
	}
}

func TestEncodeThermostatTempPayload_OutOfRange(t *testing.T) {
	_, err := encodeThermostatTempPayload(31.0)
	if !errors.Is(err, ErrHvacr02TemperatureOutOfRange) {
		t.Errorf("temp 31: got %v, want ErrHvacr02TemperatureOutOfRange", err)
	}
}

func TestEncodeThermostatFanModePayload(t *testing.T) {
	// 캡처 데이터와 비교: 64,50,20 = medium/cooling
	got := encodeThermostatFanModePayload(2, 0)
	want := []byte{0x64, 0x50, 0x20}
	if !bytes.Equal(got, want) {
		t.Errorf("thermostat fan+mode: got %X, want %X", got, want)
	}
}

func TestBuildThermostatPayload_Multiple(t *testing.T) {
	params := map[string]interface{}{
		"power":       true,
		"target_temperature": float64(24),
		"fan_speed":   "medium",
		"mode":        "cooling",
	}
	got, err := buildThermostatPayload(params, hvacr02DefaultFanCode, hvacr02DefaultModeCode, 25.0)
	if err != nil {
		t.Fatal(err)
	}
	// power ON: 62,41 + 64,50,50(auto/cooling-기본) + 64,89(24°C)
	// → but fan_speed/mode 파라미터로 오버라이드되므로:
	// power ON: 62,41 + 64,50,50(auto/cooling) + 64,89(24°C-currentTemp로 전원 인코딩에 사용)
	// temp: 64,89 (24°C)
	// fan+mode: 64,50,20 (medium/cooling)
	// 실제: power 파트는 currentTemp(25)를 사용 → 62,41,64,50,50,64,8A
	//       temp 파트는 params의 24 → 64,89
	//       fan+mode 파트는 medium/cooling → 64,50,20
	want := []byte{0x62, 0x41, 0x64, 0x50, 0x50, 0x64, 0x8A, 0x64, 0x89, 0x64, 0x50, 0x20}
	if !bytes.Equal(got, want) {
		t.Errorf("thermostat multiple: got %X, want %X", got, want)
	}
}

func TestBuildThermostatPayload_OnlyPower(t *testing.T) {
	params := map[string]interface{}{
		"power": false,
	}
	got, err := buildThermostatPayload(params, hvacr02DefaultFanCode, hvacr02DefaultModeCode, 24.0)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x62, 0x40}
	if !bytes.Equal(got, want) {
		t.Errorf("thermostat only power off: got %X, want %X", got, want)
	}
}
