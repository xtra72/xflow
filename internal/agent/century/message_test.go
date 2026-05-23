package century

import (
	"encoding/json"
	"testing"
)

// TestConfirmationStatus_String pins the String() output of every enum value
// against the lowercase wire form required by REQ-CENTURY-020.
//
// (REQ-CENTURY-020, AC-E1)
func TestConfirmationStatus_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status ConfirmationStatus
		want   string
	}{
		{Confirmed, "confirmed"},
		{Inferred, "inferred"},
		{Unknown, "unknown"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			if got := tc.status.String(); got != tc.want {
				t.Fatalf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestConfirmationStatus_JSON ensures the enum marshals to a JSON string in
// lowercase and round-trips through Unmarshal (REQ-CENTURY-020, AC-E1).
func TestConfirmationStatus_JSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status ConfirmationStatus
		want   string
	}{
		{Confirmed, `"confirmed"`},
		{Inferred, `"inferred"`},
		{Unknown, `"unknown"`},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			got, err := json.Marshal(tc.status)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("Marshal = %s, want %s", got, tc.want)
			}

			var round ConfirmationStatus
			if err := json.Unmarshal(got, &round); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if round != tc.status {
				t.Errorf("round-trip = %v, want %v", round, tc.status)
			}
		})
	}
}

// TestModeCode_String covers the two confirmed mode codes plus the fallback
// "mode_unknown_<hex>" rendering for additive enum evolution (REQ-CENTURY-021,
// AC-E2).
func TestModeCode_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  byte
		want string
	}{
		{0x00, "off"},
		{0x01, "cool"},
		{0x02, "mode_unknown_02"},
		{0x05, "mode_unknown_05"},
		{0xFF, "mode_unknown_ff"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			if got := ModeCode(tc.raw).String(); got != tc.want {
				t.Fatalf("ModeCode(0x%02X).String() = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestModeCode_AdditiveEnum verifies that the IsConfirmed() helper reports
// true only for the two ground-truth codes; every other byte must be flagged
// as unconfirmed so downstream code can route it through the
// confirmation_status="unknown" path without changing existing semantics
// (REQ-CENTURY-021, AC-E2).
func TestModeCode_AdditiveEnum(t *testing.T) {
	t.Parallel()

	confirmed := map[byte]bool{0x00: true, 0x01: true}
	for i := 0; i <= 0xFF; i++ {
		raw := byte(i)
		got := ModeCode(raw).IsConfirmed()
		want := confirmed[raw]
		if got != want {
			t.Fatalf("ModeCode(0x%02X).IsConfirmed() = %v, want %v", raw, got, want)
		}
	}
}

// TestDecodedEvent_JSON exercises a minimal end-to-end JSON marshal of a
// decoded reg 0x02 event, asserting snake_case field names that match the
// SPEC §4.4 example payload.
func TestDecodedEvent_JSON(t *testing.T) {
	t.Parallel()

	evt := &Reg02Decoded{
		SubDevID:    0x3B,
		Register:    0x02,
		TimestampMs: 1737216000123,
		Direction:   DirectionSlaveToMaster,
		Mode: ModeField{
			Value:              "cool",
			Raw:                1,
			ConfirmationStatus: Confirmed,
		},
	}
	got, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// Inspect a handful of canonical snake_case keys.
	for _, key := range []string{
		`"device_id":59`,
		`"register":2`,
		`"timestamp_ms":1737216000123`,
		`"direction":"slave_to_master"`,
		`"mode":{`,
		`"value":"cool"`,
		`"status":"confirmed"`,
	} {
		if !contains(got, key) {
			t.Errorf("JSON %s missing key %s", got, key)
		}
	}
}

func contains(haystack []byte, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if string(haystack[i:i+len(needle)]) == needle {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// v0.3.0 (M7) — CenturyDeviceStateEvent JSON / Snapshot tests (REQ-CENTURY-033)
// ---------------------------------------------------------------------------

// TestCenturyDeviceStateEvent_JSONSnakeCase pins the snake_case JSON wire format
// of the v0.3.0 device-centric event (REQ-CENTURY-033, AC-H2).
func TestCenturyDeviceStateEvent_JSONSnakeCase(t *testing.T) {
	t.Parallel()

	evapA := float32(8.5)
	evapB := float32(8.0)
	snap := CenturyDeviceStateSnapshot{
		Power:       true,
		Mode:        "cool",
		ModeRaw:     0x01,
		FanSpeed:    17,
		TargetTemp:  25.0,
		CurrentTemp: 25.2,
		Online:      true,
		TempEvapAC:  &evapA, // v0.5.1: Reg03 증발기 온도 → state 그룹에 노출
		TempEvapBC:  &evapB,
	}
	ev := NewDeviceStateEvent(snap, 0x3B, "indoor-3b", 1715985000000, TriggerChange, "")
	if ev.SubDevID != "0x3B" {
		t.Errorf("SubDevID = %q, want 0x3B (uppercase 2-digit hex)", ev.SubDevID)
	}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(b)
	// v0.5.0 통합 schema — timestamp_ms 제거, label 은 metadata.label 로 이동.
	// v0.5.1 — temp_evap_a_c / temp_evap_b_c 도 state 그룹 안에 포함 (Reg03 수신 시).
	// v0.9.0 — payload.type 제거 (metadata.message_type 이 schema 식별 역할).
	for _, key := range []string{
		`"device_id":"0x3B"`,
		`"last_seen_ms":1715985000000`,
		`"online":true`,
		`"power":true`,
		`"mode":1`,
		`"fan_speed":1`,
		`"target_temperature":25`,
		`"current_temperature":25.2`,
		`"temp_evap_a_c":8.5`,
		`"temp_evap_b_c":8`,
		`"trigger":"change"`,
		`"metadata":{"label":"indoor-3b","device_type":"HVACR.IDU"}`,
	} {
		if !contains([]byte(got), key) {
			t.Errorf("JSON missing key %q in %s", key, got)
		}
	}
	// timestamp_ms 는 제거되어야 한다.
	if contains([]byte(got), `"timestamp_ms"`) {
		t.Errorf("v0.5.0: timestamp_ms 가 출력에 남아있음: %s", got)
	}
	// label 은 top-level 이 아닌 metadata 안에 있어야 한다.
	if contains([]byte(got), `"label":"indoor-3b","timestamp`) || contains([]byte(got), `"label":"indoor-3b","last_seen`) {
		t.Errorf("v0.5.0: label 이 top-level 로 남아있음: %s", got)
	}
}

// TestCenturyDeviceStateEvent_ReportTrigger covers the periodic-report variant
// of the trigger field (REQ-CENTURY-035, v0.6.0 rename: keepalive→report).
func TestCenturyDeviceStateEvent_ReportTrigger(t *testing.T) {
	t.Parallel()
	snap := CenturyDeviceStateSnapshot{Mode: "off", Online: true}
	ev := NewDeviceStateEvent(snap, 0x3B, "indoor-3b", 999, TriggerReport, "")
	if ev.Trigger != TriggerReport {
		t.Errorf("Trigger = %q, want %q", ev.Trigger, TriggerReport)
	}
	b, _ := json.Marshal(ev)
	if !contains(b, `"trigger":"report"`) {
		t.Errorf("JSON missing trigger=report: %s", b)
	}
}

// TestBuildDeviceStateSnapshot_FromReg02Only covers AC-H2 — only reg02 received.
// current_temp_c / evap_*_c are 0.0 (missing-register fallback per A14).
func TestBuildDeviceStateSnapshot_FromReg02Only(t *testing.T) {
	t.Parallel()
	state := &CenturyDeviceState{
		Reg02: &Reg02Decoded{
			SubDevID:  0x3B,
			Register:  0x02,
			Mode:      NewModeField(0x01),
			Fan:       FieldU8{Value: 17, ConfirmationStatus: Confirmed},
			SetpointC: FieldFloat32{Value: 25.0, Raw: 250, ConfirmationStatus: Confirmed},
		},
	}
	snap := BuildDeviceStateSnapshot(state, true)
	if !snap.Power {
		t.Errorf("Power = false, want true (mode=cooling)")
	}
	if snap.Mode != "cool" {
		t.Errorf("Mode = %q, want cool", snap.Mode)
	}
	if snap.ModeRaw != 0x01 {
		t.Errorf("ModeRaw = 0x%02X, want 0x01", snap.ModeRaw)
	}
	if snap.FanSpeed != 17 {
		t.Errorf("FanSpeed = %d, want 17", snap.FanSpeed)
	}
	if snap.TargetTemp != 25.0 {
		t.Errorf("TargetTemp = %v, want 25.0", snap.TargetTemp)
	}
	if snap.CurrentTemp != 0.0 {
		t.Errorf("CurrentTemp = %v, want 0.0 (reg04 not received)", snap.CurrentTemp)
	}
}

// TestBuildDeviceStateSnapshot_PowerFromModeOff covers REQ-CENTURY-033:
// power=false when mode=0x00 (off).
func TestBuildDeviceStateSnapshot_PowerFromModeOff(t *testing.T) {
	t.Parallel()
	state := &CenturyDeviceState{
		Reg02: &Reg02Decoded{Mode: NewModeField(0x00)},
	}
	snap := BuildDeviceStateSnapshot(state, true)
	if snap.Power {
		t.Errorf("Power = true, want false (mode=off)")
	}
	if snap.Mode != "off" {
		t.Errorf("Mode = %q, want off", snap.Mode)
	}
}

// TestBuildDeviceStateSnapshot_AllRegistersReceived covers AC-H3: full state with reg02+03+04.
func TestBuildDeviceStateSnapshot_AllRegistersReceived(t *testing.T) {
	t.Parallel()
	state := &CenturyDeviceState{
		Reg02: &Reg02Decoded{
			Mode:      NewModeField(0x01),
			Fan:       FieldU8{Value: 17},
			SetpointC: FieldFloat32{Value: 25.0},
		},
		Reg03: &Reg03Decoded{
			TempEvapAC: FieldFloat32{Value: 9.0},
			TempEvapBC: FieldFloat32{Value: 8.5},
		},
		Reg04Read: &Reg04ReadDecoded{
			TempAC: FieldFloat32{Value: 25.2},
		},
	}
	snap := BuildDeviceStateSnapshot(state, true)
	if snap.CurrentTemp != 25.2 {
		t.Errorf("CurrentTemp = %v, want 25.2", snap.CurrentTemp)
	}
	// v0.3.1: evap 필드는 snapshot 에서 제거됨 — register-decoded Reg03Decoded 로만 노출.
}

// TestBuildDeviceStateSnapshot_NilState covers the edge case where State is nil.
func TestBuildDeviceStateSnapshot_NilState(t *testing.T) {
	t.Parallel()
	snap := BuildDeviceStateSnapshot(nil, false)
	if snap.Power {
		t.Errorf("Power = true, want false (nil state)")
	}
	if snap.Mode != "off" {
		t.Errorf("Mode = %q, want off (nil state)", snap.Mode)
	}
	if snap.Online {
		t.Errorf("Online = true, want false")
	}
}

// TestCenturyDeviceStateSnapshot_Equals pins the change-detection comparison:
// 5 core fields only; evap and online are NOT compared by Equals (per A15 + design).
func TestCenturyDeviceStateSnapshot_Equals(t *testing.T) {
	t.Parallel()
	base := CenturyDeviceStateSnapshot{
		Power: true, Mode: "cool", ModeRaw: 0x01,
		FanSpeed: 17, TargetTemp: 25.0, CurrentTemp: 25.2, Online: true,
	}
	// Same core → equal.
	same := base
	if !base.Equals(same) {
		t.Errorf("identical snapshots not equal")
	}
	// Online-only change → still equal (online is tracked separately).
	onlineDiff := base
	onlineDiff.Online = false
	if !base.Equals(onlineDiff) {
		t.Errorf("online-only diff should still be equal under Equals()")
	}
	// Any core change → not equal.
	for _, mut := range []func(*CenturyDeviceStateSnapshot){
		func(s *CenturyDeviceStateSnapshot) { s.Power = false },
		func(s *CenturyDeviceStateSnapshot) { s.ModeRaw = 0x02 },
		func(s *CenturyDeviceStateSnapshot) { s.FanSpeed = 18 },
		func(s *CenturyDeviceStateSnapshot) { s.TargetTemp = 26.0 },
		func(s *CenturyDeviceStateSnapshot) { s.CurrentTemp = 25.3 },
	} {
		diff := base
		mut(&diff)
		if base.Equals(diff) {
			t.Errorf("core change not detected: %+v vs %+v", base, diff)
		}
	}
}
