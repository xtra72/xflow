package century

import (
	"sync"
	"testing"
	"time"
)

func makeRec(seq uint64) CapturedFrame {
	return CapturedFrame{
		Seq:        seq,
		ReceivedAt: time.Unix(1737216000, int64(seq)),
		Raw:        []byte{byte(seq)},
	}
}

func TestFrameRingBuffer_PushUntilCapacity(t *testing.T) {
	t.Parallel()
	rb := NewFrameRingBuffer(4)
	for i := uint64(1); i <= 4; i++ {
		rb.Push(makeRec(i))
	}
	if got := rb.Len(); got != 4 {
		t.Fatalf("Len = %d, want 4", got)
	}
	if drops := rb.DropCount(); drops != 0 {
		t.Errorf("DropCount = %d, want 0", drops)
	}
	if total := rb.TotalPushed(); total != 4 {
		t.Errorf("TotalPushed = %d, want 4", total)
	}
}

func TestFrameRingBuffer_OverflowEvictsOldest(t *testing.T) {
	t.Parallel()
	// AC-B3: ring_buffer_size=4, 5 frames push → oldest evicted, drop counter +1.
	rb := NewFrameRingBuffer(4)
	for i := uint64(1); i <= 5; i++ {
		rb.Push(makeRec(i))
	}
	if got := rb.Len(); got != 4 {
		t.Fatalf("Len = %d, want 4 (capacity)", got)
	}
	if drops := rb.DropCount(); drops != 1 {
		t.Errorf("DropCount = %d, want 1", drops)
	}
	if total := rb.TotalPushed(); total != 5 {
		t.Errorf("TotalPushed = %d, want 5", total)
	}

	// GetRecent must return [2, 3, 4, 5] (oldest evicted).
	recent := rb.GetRecent(10)
	if len(recent) != 4 {
		t.Fatalf("len(GetRecent) = %d, want 4", len(recent))
	}
	gotSeqs := make([]uint64, 0, 4)
	for _, r := range recent {
		gotSeqs = append(gotSeqs, r.Seq)
	}
	wantSeqs := []uint64{2, 3, 4, 5}
	for i := range wantSeqs {
		if gotSeqs[i] != wantSeqs[i] {
			t.Errorf("GetRecent[%d].Seq = %d, want %d", i, gotSeqs[i], wantSeqs[i])
		}
	}
}

func TestFrameRingBuffer_GetRecentLimit(t *testing.T) {
	t.Parallel()
	rb := NewFrameRingBuffer(8)
	for i := uint64(1); i <= 5; i++ {
		rb.Push(makeRec(i))
	}
	got := rb.GetRecent(3)
	if len(got) != 3 {
		t.Fatalf("len(GetRecent(3)) = %d, want 3", len(got))
	}
	// Expect last 3 in insertion order: [3, 4, 5].
	if got[0].Seq != 3 || got[1].Seq != 4 || got[2].Seq != 5 {
		t.Errorf("GetRecent seqs = [%d,%d,%d], want [3,4,5]", got[0].Seq, got[1].Seq, got[2].Seq)
	}
}

func TestFrameRingBuffer_Drain(t *testing.T) {
	t.Parallel()
	rb := NewFrameRingBuffer(4)
	for i := uint64(1); i <= 3; i++ {
		rb.Push(makeRec(i))
	}
	got := rb.Drain()
	if len(got) != 3 {
		t.Fatalf("len(Drain) = %d, want 3", len(got))
	}
	if rb.Len() != 0 {
		t.Errorf("Len after drain = %d, want 0", rb.Len())
	}
	if again := rb.Drain(); len(again) != 0 {
		t.Errorf("second Drain returned %d items, want 0", len(again))
	}
}

func TestFrameRingBuffer_ConcurrentPush(t *testing.T) {
	t.Parallel()
	rb := NewFrameRingBuffer(64)
	const workers = 8
	const iterations = 100
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		w := w
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				rb.Push(makeRec(uint64(w*iterations + i + 1)))
			}
		}()
	}
	wg.Wait()
	// Total = workers*iterations = 800; capacity 64 ⇒ drops = 800 - 64.
	if got := rb.TotalPushed(); got != workers*iterations {
		t.Errorf("TotalPushed = %d, want %d", got, workers*iterations)
	}
	if got := rb.DropCount(); got != workers*iterations-64 {
		t.Errorf("DropCount = %d, want %d", got, workers*iterations-64)
	}
	if got := rb.Len(); got != 64 {
		t.Errorf("Len = %d, want 64", got)
	}
}
