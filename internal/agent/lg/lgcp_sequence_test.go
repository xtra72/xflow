package lg

import (
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// TestLGCPSequenceManager_NextSEQ0_Independent (AC-003)
// SEQ0 는 CMD 타입별로 독립적인 카운터를 유지해야 한다.
// ---------------------------------------------------------------------------

func TestLGCPSequenceManager_NextSEQ0_Independent(t *testing.T) {
	sm := NewLGCPSequenceManager()

	cmd1 := [2]byte{0x02, 0x01}
	cmd2 := [2]byte{0x06, 0x04}

	// CMD1 에 대해 3회 호출 → 0x00, 0x01, 0x02
	if got := sm.NextSEQ0(cmd1); got != 0x00 {
		t.Errorf("cmd1 call 1: want 0x00, got 0x%02X", got)
	}
	if got := sm.NextSEQ0(cmd1); got != 0x01 {
		t.Errorf("cmd1 call 2: want 0x01, got 0x%02X", got)
	}
	if got := sm.NextSEQ0(cmd1); got != 0x02 {
		t.Errorf("cmd1 call 3: want 0x02, got 0x%02X", got)
	}

	// CMD2 는 독립 카운터이므로 0x00 부터 시작
	if got := sm.NextSEQ0(cmd2); got != 0x00 {
		t.Errorf("cmd2 call 1: want 0x00, got 0x%02X", got)
	}
}

// ---------------------------------------------------------------------------
// TestLGCPSequenceManager_NextSEQ1_Global (AC-003)
// SEQ1 은 전역 프레임 카운터로, 호출마다 순차 증가해야 한다.
// ---------------------------------------------------------------------------

func TestLGCPSequenceManager_NextSEQ1_Global(t *testing.T) {
	sm := NewLGCPSequenceManager()

	for i := byte(0); i < 5; i++ {
		got := sm.NextSEQ1()
		if got != i {
			t.Errorf("call %d: want 0x%02X, got 0x%02X", i+1, i, got)
		}
	}
}

// ---------------------------------------------------------------------------
// TestLGCPSequenceManager_Wrap (AC-004)
// 0xFF → 0x00 으로 래핑되어야 한다.
// ---------------------------------------------------------------------------

func TestLGCPSequenceManager_Wrap(t *testing.T) {
	sm := NewLGCPSequenceManager()

	// SEQ1 을 0xFF 까지 전진시킨다
	for i := 0; i < 0xFF; i++ {
		sm.NextSEQ1()
	}

	// 현재 값이 0xFF 이어야 한다
	got := sm.NextSEQ1()
	if got != 0xFF {
		t.Errorf("before wrap: want 0xFF, got 0x%02X", got)
	}

	// 다음 호출은 0x00 (wrap)
	got = sm.NextSEQ1()
	if got != 0x00 {
		t.Errorf("after wrap: want 0x00, got 0x%02X", got)
	}

	// SEQ0 래핑도 검증
	cmd := [2]byte{0x01, 0x01}
	for i := 0; i < 0xFF; i++ {
		sm.NextSEQ0(cmd)
	}
	got = sm.NextSEQ0(cmd)
	if got != 0xFF {
		t.Errorf("seq0 before wrap: want 0xFF, got 0x%02X", got)
	}
	got = sm.NextSEQ0(cmd)
	if got != 0x00 {
		t.Errorf("seq0 after wrap: want 0x00, got 0x%02X", got)
	}
}

// ---------------------------------------------------------------------------
// TestLGCPSequenceManager_Reset
// Reset 호출 후 모든 카운터가 0x00 으로 초기화되어야 한다.
// ---------------------------------------------------------------------------

func TestLGCPSequenceManager_Reset(t *testing.T) {
	sm := NewLGCPSequenceManager()

	cmd := [2]byte{0x02, 0x01}

	// 카운터를 증가시킨다
	sm.NextSEQ0(cmd)
	sm.NextSEQ0(cmd)
	sm.NextSEQ1()
	sm.NextSEQ1()
	sm.NextSEQ1()

	// Reset
	sm.Reset()

	// SEQ0 는 0x00 부터 다시 시작
	if got := sm.NextSEQ0(cmd); got != 0x00 {
		t.Errorf("seq0 after reset: want 0x00, got 0x%02X", got)
	}

	// SEQ1 도 0x00 부터 다시 시작
	if got := sm.NextSEQ1(); got != 0x00 {
		t.Errorf("seq1 after reset: want 0x00, got 0x%02X", got)
	}
}

// ---------------------------------------------------------------------------
// TestLGCPSequenceManager_Concurrent
// 100개 고루틴에서 동시 호출해도 panic 없이 동작해야 한다. (-race 플래그 검증)
// ---------------------------------------------------------------------------

func TestLGCPSequenceManager_Concurrent(t *testing.T) {
	sm := NewLGCPSequenceManager()

	var wg sync.WaitGroup
	const goroutines = 100

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			sm.NextSEQ1()
		}()
	}
	wg.Wait()

	// SEQ0 동시 접근도 검증
	cmd := [2]byte{0x03, 0x01}
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			sm.NextSEQ0(cmd)
		}()
	}
	wg.Wait()
}
