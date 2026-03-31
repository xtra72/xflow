package lg

import (
	"bytes"
	"errors"
	"testing"
)

func TestEncodePowerPayload_On(t *testing.T) {
	got := encodePowerPayload(true, 0)
	want := []byte{0x18, 0x41, 0x18, 0x80, 0x29, 0xC0}
	if !bytes.Equal(got, want) {
		t.Errorf("power ON: got %X, want %X", got, want)
	}
}

func TestEncodePowerPayload_Off(t *testing.T) {
	got := encodePowerPayload(false, 0)
	want := []byte{0x18, 0x40, 0x18, 0x80, 0x29, 0xC0}
	if !bytes.Equal(got, want) {
		t.Errorf("power OFF: got %X, want %X", got, want)
	}
}

func TestEncodePowerPayload_OnWithCompressor(t *testing.T) {
	got := encodePowerPayload(true, 10)
	want := []byte{0x18, 0x41, 0x18, 0x8A, 0x29, 0xC0}
	if !bytes.Equal(got, want) {
		t.Errorf("power ON comp=10: got %X, want %X", got, want)
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
	if !errors.Is(err, ErrLGCPTemperatureOutOfRange) {
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

func TestLookupFanSpeedCode(t *testing.T) {
	for name, want := range lgcpFanSpeedCodes {
		got, err := lookupFanSpeedCode(name)
		if err != nil {
			t.Errorf("lookupFanSpeedCode(%q): %v", name, err)
		}
		if got != want {
			t.Errorf("lookupFanSpeedCode(%q): got %d, want %d", name, got, want)
		}
	}
	_, err := lookupFanSpeedCode("supersonic")
	if !errors.Is(err, ErrLGCPInvalidFanSpeed) {
		t.Errorf("invalid fan_speed: got %v, want ErrLGCPInvalidFanSpeed", err)
	}
}

func TestLookupModeCode(t *testing.T) {
	for name, want := range lgcpModeCodes {
		got, err := lookupModeCode(name)
		if err != nil {
			t.Errorf("lookupModeCode(%q): %v", name, err)
		}
		if got != want {
			t.Errorf("lookupModeCode(%q): got %d, want %d", name, got, want)
		}
	}
	_, err := lookupModeCode("turbo")
	if !errors.Is(err, ErrLGCPInvalidMode) {
		t.Errorf("invalid mode: got %v, want ErrLGCPInvalidMode", err)
	}
}

func TestBuildControlPayload_Multiple(t *testing.T) {
	params := map[string]interface{}{
		"power":       true,
		"temperature": float64(24),
		"fan_speed":   "medium",
		"mode":        "cooling",
	}
	got, err := buildControlPayload(params, lgcpDefaultFanCode, lgcpDefaultModeCode)
	if err != nil {
		t.Fatal(err)
	}

	// 전원 ON: 18 41 18 80 29 C0
	// 온도 24: 64 89 (0x80 | 9)
	// 풍량+모드: 64 50 20 (medium=2, cooling=0)
	want := []byte{0x18, 0x41, 0x18, 0x80, 0x29, 0xC0, 0x64, 0x89, 0x64, 0x50, 0x20}
	if !bytes.Equal(got, want) {
		t.Errorf("multiple: got %X, want %X", got, want)
	}
}

func TestBuildControlPayload_OnlyTemperature(t *testing.T) {
	params := map[string]interface{}{
		"temperature": float64(20),
	}
	got, err := buildControlPayload(params, lgcpDefaultFanCode, lgcpDefaultModeCode)
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
	got, err := buildControlPayload(params, lgcpDefaultFanCode, lgcpDefaultModeCode)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x18, 0x40, 0x18, 0x80, 0x29, 0xC0}
	if !bytes.Equal(got, want) {
		t.Errorf("only power off: got %X, want %X", got, want)
	}
}
