package lg

import (
	"math"
	"testing"
)

func TestUpdateFromResponse_FullState(t *testing.T) {
	// Build a response with known values
	combo := EncodeModeCombo(ModeCool, FanHigh, true)
	targetTempRaw := EncodeTargetTemp(25)
	roomTempRaw := byte(132) // -> 20.0

	resp := &LGAPResponse{
		Status:      0x00,
		FlagsEcho:   FlagPower | FlagPlasma, // power on, plasma on, lock off
		Zone:        0x10,
		Error:       0x05, // some error code
		ModeCombo:   combo,
		TargetTemp:  targetTempRaw,
		RoomTemp:    roomTempRaw,
		PipeInTemp:  147, // (192-147)/3.0 = 15.0
		PipeOutTemp: 162, // (192-162)/3.0 = 10.0
		ZoneLoad:    204,
		ZonePower:   0x01, // stopped
		DesignLoad:  100,
		ODULoad:     80,
	}

	state := &LGAPDeviceState{}
	state.UpdateFromResponse(resp)

	// Power: FlagPower is set
	if !state.Power {
		t.Errorf("Power = %v, want true", state.Power)
	}

	// Locked: FlagLock is NOT set
	if state.Locked {
		t.Errorf("Locked = %v, want false", state.Locked)
	}

	// Plasma: FlagPlasma is set
	if !state.Plasma {
		t.Errorf("Plasma = %v, want true", state.Plasma)
	}

	// Mode: ModeCool -> "cool"
	if state.Mode != "cool" {
		t.Errorf("Mode = %q, want %q", state.Mode, "cool")
	}

	// FanSpeed: FanHigh -> "high"
	if state.FanSpeed != "high" {
		t.Errorf("FanSpeed = %q, want %q", state.FanSpeed, "high")
	}

	// SwingAuto
	if !state.SwingAuto {
		t.Errorf("SwingAuto = %v, want true", state.SwingAuto)
	}

	// TargetTemp: 25
	if state.TargetTemp != 25 {
		t.Errorf("TargetTemp = %d, want %d", state.TargetTemp, 25)
	}

	// RoomTemp: 20.0
	if math.Abs(float64(state.RoomTemp-20.0)) > 0.01 {
		t.Errorf("RoomTemp = %f, want 20.0", state.RoomTemp)
	}

	// PipeInTemp: 15.0
	if math.Abs(float64(state.PipeInTemp-15.0)) > 0.01 {
		t.Errorf("PipeInTemp = %f, want 15.0", state.PipeInTemp)
	}

	// PipeOutTemp: 10.0
	if math.Abs(float64(state.PipeOutTemp-10.0)) > 0.01 {
		t.Errorf("PipeOutTemp = %f, want 10.0", state.PipeOutTemp)
	}

	// Loads
	if state.ZoneLoad != 204 {
		t.Errorf("ZoneLoad = %d, want %d", state.ZoneLoad, 204)
	}
	if state.ZonePower != 0x01 {
		t.Errorf("ZonePower = 0x%02X, want 0x01", state.ZonePower)
	}
	if state.DesignLoad != 100 {
		t.Errorf("DesignLoad = %d, want %d", state.DesignLoad, 100)
	}
	if state.ODULoad != 80 {
		t.Errorf("ODULoad = %d, want %d", state.ODULoad, 80)
	}

	// ErrorCode
	if state.ErrorCode != 0x05 {
		t.Errorf("ErrorCode = 0x%02X, want 0x05", state.ErrorCode)
	}
}

func TestUpdateFromResponse_PowerOff(t *testing.T) {
	resp := &LGAPResponse{
		FlagsEcho: 0x00, // no flags set -> power off
		ModeCombo: EncodeModeCombo(ModeAuto, FanAuto, false),
		TargetTemp: EncodeTargetTemp(22),
		RoomTemp:   132,
	}

	state := &LGAPDeviceState{}
	state.UpdateFromResponse(resp)

	if state.Power {
		t.Errorf("Power = %v, want false (no FlagPower set)", state.Power)
	}
	if state.Locked {
		t.Errorf("Locked = %v, want false (no FlagLock set)", state.Locked)
	}
	if state.Plasma {
		t.Errorf("Plasma = %v, want false (no FlagPlasma set)", state.Plasma)
	}
}

func TestUpdateFromResponse_LockFlag(t *testing.T) {
	resp := &LGAPResponse{
		FlagsEcho: FlagLock, // only lock flag
		ModeCombo: EncodeModeCombo(ModeCool, FanLow, false),
		TargetTemp: EncodeTargetTemp(20),
		RoomTemp:   132,
	}

	state := &LGAPDeviceState{}
	state.UpdateFromResponse(resp)

	if state.Power {
		t.Errorf("Power = %v, want false", state.Power)
	}
	if !state.Locked {
		t.Errorf("Locked = %v, want true", state.Locked)
	}
	if state.Plasma {
		t.Errorf("Plasma = %v, want false", state.Plasma)
	}
}

func TestUpdateFromResponse_AllModes(t *testing.T) {
	modes := []struct {
		mode     byte
		wantName string
	}{
		{ModeCool, "cool"},
		{ModeDry, "dry"},
		{ModeFan, "fan"},
		{ModeAuto, "auto"},
		{ModeHeat, "heat"},
	}

	for _, tt := range modes {
		t.Run(tt.wantName, func(t *testing.T) {
			resp := &LGAPResponse{
				ModeCombo:  EncodeModeCombo(tt.mode, FanAuto, false),
				TargetTemp: EncodeTargetTemp(22),
				RoomTemp:   132,
			}

			state := &LGAPDeviceState{}
			state.UpdateFromResponse(resp)

			if state.Mode != tt.wantName {
				t.Errorf("Mode = %q, want %q", state.Mode, tt.wantName)
			}
		})
	}
}

func TestUpdateFromResponse_AllFanSpeeds(t *testing.T) {
	fans := []struct {
		fan      byte
		wantName string
	}{
		{FanNoChange, "no_change"},
		{FanLow, "low"},
		{FanMedium, "medium"},
		{FanHigh, "high"},
		{FanAuto, "auto"},
		{FanSlow, "slow"},
		{FanTurbo, "turbo"},
	}

	for _, tt := range fans {
		t.Run(tt.wantName, func(t *testing.T) {
			resp := &LGAPResponse{
				ModeCombo:  EncodeModeCombo(ModeCool, tt.fan, false),
				TargetTemp: EncodeTargetTemp(22),
				RoomTemp:   132,
			}

			state := &LGAPDeviceState{}
			state.UpdateFromResponse(resp)

			if state.FanSpeed != tt.wantName {
				t.Errorf("FanSpeed = %q, want %q", state.FanSpeed, tt.wantName)
			}
		})
	}
}

func TestUpdateFromResponse_ErrorCodePropagation(t *testing.T) {
	tests := []struct {
		name      string
		errorCode byte
	}{
		{name: "no error", errorCode: 0x00},
		{name: "error 0x01", errorCode: 0x01},
		{name: "error 0xFF", errorCode: 0xFF},
		{name: "error 0x42", errorCode: 0x42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &LGAPResponse{
				Error:      tt.errorCode,
				ModeCombo:  EncodeModeCombo(ModeCool, FanAuto, false),
				TargetTemp: EncodeTargetTemp(22),
				RoomTemp:   132,
			}

			state := &LGAPDeviceState{}
			state.UpdateFromResponse(resp)

			if state.ErrorCode != tt.errorCode {
				t.Errorf("ErrorCode = 0x%02X, want 0x%02X", state.ErrorCode, tt.errorCode)
			}
		})
	}
}

func TestUpdateFromResponse_TemperatureDecoding(t *testing.T) {
	tests := []struct {
		name           string
		targetTempRaw  byte
		roomTempRaw    byte
		wantTargetTemp int
		wantRoomTemp   float32
	}{
		{
			name:           "25 degrees room 20.0",
			targetTempRaw:  EncodeTargetTemp(25),
			roomTempRaw:    132,
			wantTargetTemp: 25,
			wantRoomTemp:   20.0,
		},
		{
			name:           "16 degrees room 30.0",
			targetTempRaw:  EncodeTargetTemp(16),
			roomTempRaw:    102, // (192-102)/3.0 = 30.0
			wantTargetTemp: 16,
			wantRoomTemp:   30.0,
		},
		{
			name:           "30 degrees room 10.0",
			targetTempRaw:  EncodeTargetTemp(30),
			roomTempRaw:    162, // (192-162)/3.0 = 10.0
			wantTargetTemp: 30,
			wantRoomTemp:   10.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &LGAPResponse{
				ModeCombo:  EncodeModeCombo(ModeCool, FanAuto, false),
				TargetTemp: tt.targetTempRaw,
				RoomTemp:   tt.roomTempRaw,
			}

			state := &LGAPDeviceState{}
			state.UpdateFromResponse(resp)

			if state.TargetTemp != tt.wantTargetTemp {
				t.Errorf("TargetTemp = %d, want %d", state.TargetTemp, tt.wantTargetTemp)
			}
			if math.Abs(float64(state.RoomTemp-tt.wantRoomTemp)) > 0.01 {
				t.Errorf("RoomTemp = %f, want %f", state.RoomTemp, tt.wantRoomTemp)
			}
		})
	}
}

func TestStateForJSON(t *testing.T) {
	state := &LGAPDeviceState{
		Power:      true,
		Mode:       "cool",
		FanSpeed:   "high",
		TargetTemp: 25,
		RoomTemp:   20.0,
	}

	result := state.StateForJSON()
	if result == nil {
		t.Fatal("StateForJSON() returned nil")
	}

	// StateForJSON returns the state itself
	got, ok := result.(*LGAPDeviceState)
	if !ok {
		t.Fatalf("StateForJSON() returned type %T, want *LGAPDeviceState", result)
	}
	if got != state {
		t.Errorf("StateForJSON() returned different pointer")
	}
}
