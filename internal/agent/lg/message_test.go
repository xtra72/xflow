package lg

import (
	"math"
	"testing"
)

// ---------------------------------------------------------------------------
// EncodeTargetTemp / DecodeTargetTemp
// ---------------------------------------------------------------------------

func TestEncodeTargetTemp(t *testing.T) {
	tests := []struct {
		name    string
		celsius int
		want    byte
	}{
		{name: "25 degrees -> 10", celsius: 25, want: 10},
		{name: "16 degrees -> 1", celsius: 16, want: 1},
		{name: "30 degrees -> 15", celsius: 30, want: 15},
		{name: "15 degrees -> 0", celsius: 15, want: 0},
		{name: "20 degrees -> 5", celsius: 20, want: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodeTargetTemp(tt.celsius)
			if got != tt.want {
				t.Errorf("EncodeTargetTemp(%d) = %d, want %d", tt.celsius, got, tt.want)
			}
		})
	}
}

func TestDecodeTargetTemp(t *testing.T) {
	tests := []struct {
		name string
		raw  byte
		want int
	}{
		{name: "raw lower nibble 10 -> 25", raw: 10, want: 25},
		{name: "raw lower nibble 1 -> 16", raw: 1, want: 16},
		{name: "raw lower nibble 15 -> 30", raw: 15, want: 30},
		{name: "raw lower nibble 0 -> 15", raw: 0, want: 15},
		{name: "raw with upper nibble set - only lower nibble used", raw: 0xFA, want: int(0x0A) + 15}, // lower nibble = 0x0A = 10, 10+15=25
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeTargetTemp(tt.raw)
			if got != tt.want {
				t.Errorf("DecodeTargetTemp(0x%02X) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}

func TestTargetTempRoundTrip(t *testing.T) {
	// Round-trip: EncodeTargetTemp -> DecodeTargetTemp should return original value
	// for the valid range 16-30.
	for celsius := 16; celsius <= 30; celsius++ {
		encoded := EncodeTargetTemp(celsius)
		decoded := DecodeTargetTemp(encoded)
		if decoded != celsius {
			t.Errorf("round-trip failed for %d: encoded=%d, decoded=%d", celsius, encoded, decoded)
		}
	}
}

// ---------------------------------------------------------------------------
// DecodeMeasuredTemp
// ---------------------------------------------------------------------------

func TestDecodeMeasuredTemp(t *testing.T) {
	tests := []struct {
		name string
		raw  byte
		want float32
	}{
		{
			name: "raw 132 -> (192-132)/3.0 = 20.0",
			raw:  132,
			want: 20.0,
		},
		{
			name: "raw 192 -> 0.0",
			raw:  192,
			want: 0.0,
		},
		{
			name: "raw 0 -> 64.0",
			raw:  0,
			want: 64.0,
		},
		{
			name: "raw 162 -> (192-162)/3.0 = 10.0",
			raw:  162,
			want: 10.0,
		},
		{
			name: "raw 117 -> (192-117)/3.0 = 25.0",
			raw:  117,
			want: 25.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeMeasuredTemp(tt.raw)
			if math.Abs(float64(got-tt.want)) > 0.01 {
				t.Errorf("DecodeMeasuredTemp(%d) = %f, want %f", tt.raw, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// EncodeModeCombo / DecodeModeCombo
// ---------------------------------------------------------------------------

func TestEncodeModeCombo(t *testing.T) {
	tests := []struct {
		name      string
		mode      byte
		fan       byte
		swingAuto bool
		want      byte
	}{
		{
			name:      "cool/low/no swing",
			mode:      ModeCool, // 0
			fan:       FanLow,  // 1
			swingAuto: false,
			want:      (0 << 5) | (1 << 2) | 0x00, // 0x04
		},
		{
			name:      "heat/high/swing auto",
			mode:      ModeHeat, // 4
			fan:       FanHigh, // 3
			swingAuto: true,
			want:      (4 << 5) | (3 << 2) | 0x02, // 0x8E
		},
		{
			name:      "auto/auto/no swing",
			mode:      ModeAuto, // 3
			fan:       FanAuto, // 4
			swingAuto: false,
			want:      (3 << 5) | (4 << 2), // 0x70
		},
		{
			name:      "dry/medium/swing auto",
			mode:      ModeDry,    // 1
			fan:       FanMedium, // 2
			swingAuto: true,
			want:      (1 << 5) | (2 << 2) | 0x02, // 0x2A
		},
		{
			name:      "fan/turbo/no swing",
			mode:      ModeFan,   // 2
			fan:       FanTurbo, // 6
			swingAuto: false,
			want:      (2 << 5) | (6 << 2), // 0x58
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodeModeCombo(tt.mode, tt.fan, tt.swingAuto)
			if got != tt.want {
				t.Errorf("EncodeModeCombo(%d, %d, %v) = 0x%02X, want 0x%02X",
					tt.mode, tt.fan, tt.swingAuto, got, tt.want)
			}
		})
	}
}

func TestDecodeModeCombo(t *testing.T) {
	tests := []struct {
		name          string
		combo         byte
		wantMode      byte
		wantFan       byte
		wantSwingAuto bool
	}{
		{
			name:          "cool/low/no swing",
			combo:         (0 << 5) | (1 << 2),
			wantMode:      ModeCool,
			wantFan:       FanLow,
			wantSwingAuto: false,
		},
		{
			name:          "heat/high/swing auto",
			combo:         (4 << 5) | (3 << 2) | 0x02,
			wantMode:      ModeHeat,
			wantFan:       FanHigh,
			wantSwingAuto: true,
		},
		{
			name:          "auto/auto/no swing",
			combo:         (3 << 5) | (4 << 2),
			wantMode:      ModeAuto,
			wantFan:       FanAuto,
			wantSwingAuto: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, fan, swingAuto := DecodeModeCombo(tt.combo)
			if mode != tt.wantMode {
				t.Errorf("DecodeModeCombo(0x%02X) mode = %d, want %d", tt.combo, mode, tt.wantMode)
			}
			if fan != tt.wantFan {
				t.Errorf("DecodeModeCombo(0x%02X) fan = %d, want %d", tt.combo, fan, tt.wantFan)
			}
			if swingAuto != tt.wantSwingAuto {
				t.Errorf("DecodeModeCombo(0x%02X) swingAuto = %v, want %v", tt.combo, swingAuto, tt.wantSwingAuto)
			}
		})
	}
}

func TestModeComboRoundTrip(t *testing.T) {
	modes := []byte{ModeCool, ModeDry, ModeFan, ModeAuto, ModeHeat}
	fans := []byte{FanNoChange, FanLow, FanMedium, FanHigh, FanAuto, FanSlow, FanTurbo}
	swings := []bool{false, true}

	for _, mode := range modes {
		for _, fan := range fans {
			for _, swing := range swings {
				combo := EncodeModeCombo(mode, fan, swing)
				gotMode, gotFan, gotSwing := DecodeModeCombo(combo)
				if gotMode != mode || gotFan != fan || gotSwing != swing {
					t.Errorf("round-trip failed for mode=%d, fan=%d, swing=%v: got mode=%d, fan=%d, swing=%v",
						mode, fan, swing, gotMode, gotFan, gotSwing)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// ZoneToGroupUnit / GroupUnitToZone
// ---------------------------------------------------------------------------

func TestZoneToGroupUnit(t *testing.T) {
	tests := []struct {
		name      string
		zone      byte
		wantGroup int
		wantUnit  int
	}{
		{name: "zone 0x10 -> group=1, unit=0", zone: 0x10, wantGroup: 1, wantUnit: 0},
		{name: "zone 0x00 -> group=0, unit=0", zone: 0x00, wantGroup: 0, wantUnit: 0},
		{name: "zone 0xFF -> group=15, unit=15", zone: 0xFF, wantGroup: 15, wantUnit: 15},
		{name: "zone 0x23 -> group=2, unit=3", zone: 0x23, wantGroup: 2, wantUnit: 3},
		{name: "zone 0xA5 -> group=10, unit=5", zone: 0xA5, wantGroup: 10, wantUnit: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group, unit := ZoneToGroupUnit(tt.zone)
			if group != tt.wantGroup {
				t.Errorf("ZoneToGroupUnit(0x%02X) group = %d, want %d", tt.zone, group, tt.wantGroup)
			}
			if unit != tt.wantUnit {
				t.Errorf("ZoneToGroupUnit(0x%02X) unit = %d, want %d", tt.zone, unit, tt.wantUnit)
			}
		})
	}
}

func TestGroupUnitToZone(t *testing.T) {
	tests := []struct {
		name  string
		group int
		unit  int
		want  byte
	}{
		{name: "group=1, unit=0 -> 0x10", group: 1, unit: 0, want: 0x10},
		{name: "group=0, unit=0 -> 0x00", group: 0, unit: 0, want: 0x00},
		{name: "group=15, unit=15 -> 0xFF", group: 15, unit: 15, want: 0xFF},
		{name: "group=2, unit=3 -> 0x23", group: 2, unit: 3, want: 0x23},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GroupUnitToZone(tt.group, tt.unit)
			if got != tt.want {
				t.Errorf("GroupUnitToZone(%d, %d) = 0x%02X, want 0x%02X", tt.group, tt.unit, got, tt.want)
			}
		})
	}
}

func TestZoneGroupUnitRoundTrip(t *testing.T) {
	for group := 0; group <= 15; group++ {
		for unit := 0; unit <= 15; unit++ {
			zone := GroupUnitToZone(group, unit)
			gotGroup, gotUnit := ZoneToGroupUnit(zone)
			if gotGroup != group || gotUnit != unit {
				t.Errorf("round-trip failed for group=%d, unit=%d: zone=0x%02X, got group=%d, unit=%d",
					group, unit, zone, gotGroup, gotUnit)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// String maps consistency
// ---------------------------------------------------------------------------

func TestStringToModeAndModeToString_Consistency(t *testing.T) {
	// Every entry in StringToMode should have a reverse in ModeToString
	for str, modeByte := range StringToMode {
		reverseStr, ok := ModeToString[modeByte]
		if !ok {
			t.Errorf("StringToMode[%q] = 0x%02X, but ModeToString[0x%02X] does not exist", str, modeByte, modeByte)
			continue
		}
		if reverseStr != str {
			t.Errorf("StringToMode[%q] = 0x%02X, but ModeToString[0x%02X] = %q", str, modeByte, modeByte, reverseStr)
		}
	}

	// Every entry in ModeToString should have a reverse in StringToMode
	for modeByte, str := range ModeToString {
		reverseByte, ok := StringToMode[str]
		if !ok {
			t.Errorf("ModeToString[0x%02X] = %q, but StringToMode[%q] does not exist", modeByte, str, str)
			continue
		}
		if reverseByte != modeByte {
			t.Errorf("ModeToString[0x%02X] = %q, but StringToMode[%q] = 0x%02X", modeByte, str, str, reverseByte)
		}
	}
}

func TestStringToFanSpeedAndFanSpeedToString_Consistency(t *testing.T) {
	// 하위 호환 별칭 (정방향 매핑만 존재, 역방향 불일치 허용)
	aliases := map[string]bool{"slow": true}

	// Every entry in StringToFanSpeed should have a reverse in FanSpeedToString
	for str, fanByte := range StringToFanSpeed {
		reverseStr, ok := FanSpeedToString[fanByte]
		if !ok {
			t.Errorf("StringToFanSpeed[%q] = 0x%02X, but FanSpeedToString[0x%02X] does not exist", str, fanByte, fanByte)
			continue
		}
		if reverseStr != str && !aliases[str] {
			t.Errorf("StringToFanSpeed[%q] = 0x%02X, but FanSpeedToString[0x%02X] = %q", str, fanByte, fanByte, reverseStr)
		}
	}
}

func TestModeMapCompleteness(t *testing.T) {
	// All defined mode constants should be in the maps
	allModes := map[string]byte{
		"cool": ModeCool,
		"dry":  ModeDry,
		"fan":  ModeFan,
		"auto": ModeAuto,
		"heat": ModeHeat,
	}

	if len(StringToMode) != len(allModes) {
		t.Errorf("StringToMode has %d entries, want %d", len(StringToMode), len(allModes))
	}

	for name, expected := range allModes {
		got, ok := StringToMode[name]
		if !ok {
			t.Errorf("StringToMode missing entry for %q", name)
		} else if got != expected {
			t.Errorf("StringToMode[%q] = %d, want %d", name, got, expected)
		}
	}
}
