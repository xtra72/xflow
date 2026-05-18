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
		{0x01, "cooling"},
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
			Value:              "cooling",
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
		`"sub_dev_id":59`,
		`"register":2`,
		`"timestamp_ms":1737216000123`,
		`"direction":"slave_to_master"`,
		`"mode":{`,
		`"value":"cooling"`,
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
