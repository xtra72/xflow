package century

import (
	"sync"
	"testing"
	"time"
)

func TestNewIcp01Device_Defaults(t *testing.T) {
	t.Parallel()
	now := time.Unix(1737216000, 0)
	d := NewIcp01Device(0x3B, "auto", now)
	if d.SubDevID != 0x3B {
		t.Errorf("SubDevID = 0x%02X, want 0x3B", d.SubDevID)
	}
	if d.Label != "indoor-3b" {
		t.Errorf("Label = %q, want indoor-3b", d.Label)
	}
	if d.Source != "auto" {
		t.Errorf("Source = %q, want auto", d.Source)
	}
	if !d.Online {
		t.Errorf("Online = false, want true (initial)")
	}
	if !d.LastSeen.Equal(now) {
		t.Errorf("LastSeen = %v, want %v", d.LastSeen, now)
	}
	if d.State == nil {
		t.Fatalf("State = nil, want allocated")
	}
}

func TestIcp01Device_Touch(t *testing.T) {
	t.Parallel()
	start := time.Unix(1737216000, 0)
	d := NewIcp01Device(0x3B, "auto", start)
	d.Online = false

	later := start.Add(1 * time.Second)
	d.Touch(later)

	if !d.Online {
		t.Errorf("Online = false after Touch, want true")
	}
	if !d.LastSeen.Equal(later) {
		t.Errorf("LastSeen = %v, want %v", d.LastSeen, later)
	}
}

func TestIcp01Device_IsStale(t *testing.T) {
	t.Parallel()
	start := time.Unix(1737216000, 0)
	d := NewIcp01Device(0x3B, "auto", start)

	cases := []struct {
		name    string
		now     time.Time
		timeout time.Duration
		want    bool
	}{
		{"fresh", start.Add(10 * time.Millisecond), 200 * time.Millisecond, false},
		{"exactly at boundary", start.Add(200 * time.Millisecond), 200 * time.Millisecond, false},
		{"stale by 1ms", start.Add(201 * time.Millisecond), 200 * time.Millisecond, true},
		{"stale by 1s", start.Add(1 * time.Second), 200 * time.Millisecond, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := d.IsStale(tc.now, tc.timeout); got != tc.want {
				t.Errorf("IsStale(%v,%v) = %v, want %v", tc.now, tc.timeout, got, tc.want)
			}
		})
	}
}

func TestIcp01Device_Update_RoutesByType(t *testing.T) {
	t.Parallel()
	now := time.Unix(1737216000, 0)
	d := NewIcp01Device(0x3B, "auto", now)

	reg02 := &Reg02Decoded{SubDevID: 0x3B, Register: 0x02, TimestampMs: 1, Mode: NewModeField(0x01)}
	reg03 := &Reg03Decoded{SubDevID: 0x3B, Register: 0x03, TimestampMs: 2}
	reg04r := &Reg04ReadDecoded{SubDevID: 0x3B, Register: 0x04, TimestampMs: 3}
	reg04w := &Reg04WriteDecoded{SubDevID: 0x3B, Register: 0x04, TimestampMs: 4, ObservationMode: "passive"}
	ack := &ACKDecoded{TimestampMs: 5}

	d.Update(reg02, now.Add(1*time.Millisecond))
	d.Update(reg03, now.Add(2*time.Millisecond))
	d.Update(reg04r, now.Add(3*time.Millisecond))
	d.Update(reg04w, now.Add(4*time.Millisecond))
	d.Update(ack, now.Add(5*time.Millisecond))

	state := d.State
	if state.Reg02 != reg02 {
		t.Errorf("State.Reg02 = %v, want %v", state.Reg02, reg02)
	}
	if state.Reg03 != reg03 {
		t.Errorf("State.Reg03 = %v, want %v", state.Reg03, reg03)
	}
	if state.Reg04Read != reg04r {
		t.Errorf("State.Reg04Read = %v, want %v", state.Reg04Read, reg04r)
	}
	if state.Reg04Write != reg04w {
		t.Errorf("State.Reg04Write = %v, want %v", state.Reg04Write, reg04w)
	}
	if !state.LastFrameAt.Equal(now.Add(5 * time.Millisecond)) {
		t.Errorf("LastFrameAt = %v, want %v", state.LastFrameAt, now.Add(5*time.Millisecond))
	}
}

func TestIcp01Device_UpdateThenStaleStillKeepsLastKnown(t *testing.T) {
	t.Parallel()
	// AC-B2: "디바이스의 마지막 알려진 status 는 보존되어야 한다 (stale 표시)."
	now := time.Unix(1737216000, 0)
	d := NewIcp01Device(0x3B, "auto", now)
	reg02 := &Reg02Decoded{SubDevID: 0x3B, Register: 0x02, Mode: NewModeField(0x01)}
	d.Update(reg02, now)

	// Simulate offline transition externally; state should still hold last known reg02.
	d.Online = false
	if d.State.Reg02 != reg02 {
		t.Fatalf("State.Reg02 cleared after offline; want preserved")
	}
}

func TestIcp01Device_ConcurrentUpdate(t *testing.T) {
	t.Parallel()
	now := time.Now()
	d := NewIcp01Device(0x3B, "auto", now)

	const workers = 8
	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				reg02 := &Reg02Decoded{SubDevID: 0x3B, Register: 0x02, Mode: NewModeField(0x01)}
				d.Update(reg02, time.Now())
				_ = d.IsStale(time.Now(), time.Second)
			}
		}()
	}
	wg.Wait()
}
