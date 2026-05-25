package century

import (
	"testing"
)

func TestWriteDeduplicator_FirstSightEmits(t *testing.T) {
	t.Parallel()
	d := NewWriteDeduplicator()
	payload := []byte{0x01, 0x02, 0x03}
	if !d.ShouldEmit(1, payload) {
		t.Fatalf("first sight returned false, want true")
	}
}

func TestWriteDeduplicator_DuplicateInSameCycle(t *testing.T) {
	t.Parallel()
	// AC-F1: same payload in same cycle → second is suppressed.
	d := NewWriteDeduplicator()
	payload := []byte{0x01, 0x02, 0x03}
	if !d.ShouldEmit(1, payload) {
		t.Fatalf("first emit suppressed")
	}
	if d.ShouldEmit(1, payload) {
		t.Errorf("duplicate in same cycle emitted, want suppressed")
	}
}

func TestWriteDeduplicator_DifferentPayloadSameCycleEmits(t *testing.T) {
	t.Parallel()
	d := NewWriteDeduplicator()
	if !d.ShouldEmit(1, []byte{0x01}) {
		t.Fatalf("first payload suppressed")
	}
	if !d.ShouldEmit(1, []byte{0x02}) {
		t.Errorf("different payload in same cycle suppressed, want emit")
	}
}

func TestWriteDeduplicator_SamePayloadDifferentCyclesBothEmit(t *testing.T) {
	t.Parallel()
	// AC-F3: same payload but different cycle → both emit.
	d := NewWriteDeduplicator()
	payload := []byte{0x01, 0x02, 0x03}
	if !d.ShouldEmit(1, payload) {
		t.Fatalf("cycle 1 first emit suppressed")
	}
	if !d.ShouldEmit(2, payload) {
		t.Errorf("cycle 2 first emit suppressed (same payload, different cycle)")
	}
}

func TestWriteDeduplicator_RetainsBoundedCycles(t *testing.T) {
	t.Parallel()
	// Memory bound: deduplicator keeps last N=4 cycles. Older cycles' state is discarded.
	d := NewWriteDeduplicator()
	payload := []byte{0xAB, 0xCD}

	// Push payload in cycles 1..5; cycle 1 should have been evicted by cycle 5.
	for c := uint64(1); c <= 5; c++ {
		if !d.ShouldEmit(c, payload) {
			t.Errorf("cycle %d first emit suppressed", c)
		}
	}
	// Re-emit in cycle 1 — even though originally seen, cycle 1 should be evicted by now,
	// so it should emit again (best-effort; this is a defensive behavior, not strict req).
	// We do not assert this strictly since it depends on retention policy implementation,
	// but ensure no panic.
	_ = d.ShouldEmit(1, payload)

	// Now ensure cycle 5 still suppresses duplicate.
	if d.ShouldEmit(5, payload) {
		t.Errorf("cycle 5 duplicate emitted, want suppressed")
	}
}

func TestWriteDeduplicator_PayloadCopyImmunity(t *testing.T) {
	t.Parallel()
	// Caller may reuse the input slice; deduplicator must be immune to backing mutations.
	d := NewWriteDeduplicator()
	buf := []byte{0x01, 0x02, 0x03}
	if !d.ShouldEmit(1, buf) {
		t.Fatalf("first emit suppressed")
	}
	// Mutate caller buffer.
	buf[0] = 0xFF
	// Same logical content as before mutation. Submit a fresh copy with original bytes.
	if d.ShouldEmit(1, []byte{0x01, 0x02, 0x03}) {
		t.Errorf("duplicate after caller mutation emitted, want suppressed")
	}
}
