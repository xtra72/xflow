package lg

import (
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// TestIcp02SequenceManager_ObserveAndNext
// ObserveFrame 으로 관찰한 시퀀스의 다음 값을 반환해야 한다.
// ---------------------------------------------------------------------------

func TestIcp02SequenceManager_ObserveAndNext(t *testing.T) {
	sm := NewIcp02SequenceManager()
	cmd := [2]byte{0x02, 0x01}

	// 관찰 전: 기본값 0 + 1 = 1
	if got := sm.NextSEQ0(cmd); got != 0x01 {
		t.Errorf("before observe: want 0x01, got 0x%02X", got)
	}
	if got := sm.NextSEQ1(); got != 0x01 {
		t.Errorf("before observe seq1: want 0x01, got 0x%02X", got)
	}

	// 컨트롤러 프레임 관찰: SEQ0=0x15, SEQ1=0xA3
	sm.ObserveFrame(cmd, 0x15, 0xA3)

	// 관찰 후: 0x15 + 1 = 0x16, 0xA3 + 1 = 0xA4
	if got := sm.NextSEQ0(cmd); got != 0x16 {
		t.Errorf("after observe seq0: want 0x16, got 0x%02X", got)
	}
	if got := sm.NextSEQ1(); got != 0xA4 {
		t.Errorf("after observe seq1: want 0xA4, got 0x%02X", got)
	}
}

// ---------------------------------------------------------------------------
// TestIcp02SequenceManager_Synced
// ObserveFrame 호출 전후로 Synced 값이 변경되어야 한다.
// ---------------------------------------------------------------------------

func TestIcp02SequenceManager_Synced(t *testing.T) {
	sm := NewIcp02SequenceManager()

	if sm.Synced() {
		t.Error("want Synced()=false before observe")
	}

	sm.ObserveFrame([2]byte{0x02, 0x04}, 0x10, 0x20)

	if !sm.Synced() {
		t.Error("want Synced()=true after observe")
	}
}

// ---------------------------------------------------------------------------
// TestIcp02SequenceManager_SEQ0_Independent
// SEQ0 는 CMD 타입별로 독립적인 카운터를 유지해야 한다.
// ---------------------------------------------------------------------------

func TestIcp02SequenceManager_SEQ0_Independent(t *testing.T) {
	sm := NewIcp02SequenceManager()

	cmd1 := [2]byte{0x02, 0x01}
	cmd2 := [2]byte{0x06, 0x04}

	sm.ObserveFrame(cmd1, 0x05, 0x10)
	sm.ObserveFrame(cmd2, 0x20, 0x10)

	if got := sm.NextSEQ0(cmd1); got != 0x06 {
		t.Errorf("cmd1: want 0x06, got 0x%02X", got)
	}
	if got := sm.NextSEQ0(cmd2); got != 0x21 {
		t.Errorf("cmd2: want 0x21, got 0x%02X", got)
	}
}

// ---------------------------------------------------------------------------
// TestIcp02SequenceManager_Wrap
// 0xFF → 0x00 으로 래핑되어야 한다.
// ---------------------------------------------------------------------------

func TestIcp02SequenceManager_Wrap(t *testing.T) {
	sm := NewIcp02SequenceManager()
	cmd := [2]byte{0x02, 0x01}

	// SEQ0=0xFF 관찰 → NextSEQ0 = 0x00 (wrap)
	sm.ObserveFrame(cmd, 0xFF, 0xFF)
	if got := sm.NextSEQ0(cmd); got != 0x00 {
		t.Errorf("seq0 wrap: want 0x00, got 0x%02X", got)
	}
	if got := sm.NextSEQ1(); got != 0x00 {
		t.Errorf("seq1 wrap: want 0x00, got 0x%02X", got)
	}
}

// ---------------------------------------------------------------------------
// TestIcp02SequenceManager_Reset
// Reset 호출 후 모든 카운터가 초기화되어야 한다.
// ---------------------------------------------------------------------------

func TestIcp02SequenceManager_Reset(t *testing.T) {
	sm := NewIcp02SequenceManager()
	cmd := [2]byte{0x02, 0x01}

	sm.ObserveFrame(cmd, 0x50, 0x80)

	sm.Reset()

	if sm.Synced() {
		t.Error("want Synced()=false after reset")
	}
	// 리셋 후 기본값 0 + 1 = 1
	if got := sm.NextSEQ0(cmd); got != 0x01 {
		t.Errorf("seq0 after reset: want 0x01, got 0x%02X", got)
	}
	if got := sm.NextSEQ1(); got != 0x01 {
		t.Errorf("seq1 after reset: want 0x01, got 0x%02X", got)
	}
}

// ---------------------------------------------------------------------------
// TestIcp02SequenceManager_AllocSEQ0
// AllocSEQ0 는 매 호출마다 카운터를 전진시켜야 한다.
// ---------------------------------------------------------------------------

func TestIcp02SequenceManager_AllocSEQ0(t *testing.T) {
	sm := NewIcp02SequenceManager()
	cmd := [2]byte{0x02, 0x01}

	sm.ObserveFrame(cmd, 0x10, 0x50)

	// 첫 번째 Alloc: 0x10 + 1 = 0x11
	if got := sm.AllocSEQ0(cmd); got != 0x11 {
		t.Errorf("alloc1: want 0x11, got 0x%02X", got)
	}
	// 두 번째 Alloc: 0x11 + 1 = 0x12 (카운터 전진)
	if got := sm.AllocSEQ0(cmd); got != 0x12 {
		t.Errorf("alloc2: want 0x12, got 0x%02X", got)
	}
	// NextSEQ0 는 관찰값 기준 (seq0Map): 관찰값 0x10+1 = 0x11 (AllocSEQ0 와 독립)
	if got := sm.NextSEQ0(cmd); got != 0x11 {
		t.Errorf("next after alloc: want 0x11, got 0x%02X", got)
	}
}

// ---------------------------------------------------------------------------
// TestIcp02SequenceManager_AllocSEQ1
// AllocSEQ1 는 매 호출마다 전역 카운터를 전진시켜야 한다.
// ---------------------------------------------------------------------------

func TestIcp02SequenceManager_AllocSEQ1(t *testing.T) {
	sm := NewIcp02SequenceManager()

	sm.ObserveFrame([2]byte{0x02, 0x01}, 0x00, 0xA0)

	if got := sm.AllocSEQ1(); got != 0xA1 {
		t.Errorf("alloc1: want 0xA1, got 0x%02X", got)
	}
	if got := sm.AllocSEQ1(); got != 0xA2 {
		t.Errorf("alloc2: want 0xA2, got 0x%02X", got)
	}
}

// ---------------------------------------------------------------------------
// TestIcp02SequenceManager_AllocSEQ0_Wrap
// AllocSEQ0 는 0xFF 에서 0x00 으로 순환해야 한다.
// ---------------------------------------------------------------------------

func TestIcp02SequenceManager_AllocSEQ0_Wrap(t *testing.T) {
	sm := NewIcp02SequenceManager()
	cmd := [2]byte{0x02, 0x01}

	sm.ObserveFrame(cmd, 0xFE, 0x00)

	if got := sm.AllocSEQ0(cmd); got != 0xFF {
		t.Errorf("alloc1: want 0xFF, got 0x%02X", got)
	}
	if got := sm.AllocSEQ0(cmd); got != 0x00 {
		t.Errorf("alloc2 wrap: want 0x00, got 0x%02X", got)
	}
	if got := sm.AllocSEQ0(cmd); got != 0x01 {
		t.Errorf("alloc3: want 0x01, got 0x%02X", got)
	}
}

// ---------------------------------------------------------------------------
// TestIcp02SequenceManager_AllocSEQ0_ObserveRace
// AllocSEQ0 호출 사이에 ObserveFrame 이 끼어들어도 값이 뒤로 가지 않아야 한다.
// ---------------------------------------------------------------------------

func TestIcp02SequenceManager_AllocSEQ0_ObserveRace(t *testing.T) {
	sm := NewIcp02SequenceManager()
	cmd := [2]byte{0x02, 0x01}

	sm.ObserveFrame(cmd, 0xA4, 0x50)

	// 첫 번째 할당: 0xA5
	if got := sm.AllocSEQ0(cmd); got != 0xA5 {
		t.Errorf("alloc1: want 0xA5, got 0x%02X", got)
	}

	// 실제 컨트롤러가 0xA4 를 다시 관찰 (할당값 0xA5 보다 뒤)
	sm.ObserveFrame(cmd, 0xA4, 0x50)

	// 두 번째 할당: 0xA5 가 아닌 0xA6 이어야 함 (high-water mark 보호)
	if got := sm.AllocSEQ0(cmd); got != 0xA6 {
		t.Errorf("alloc2 after observe: want 0xA6, got 0x%02X", got)
	}

	// 실제 컨트롤러가 0xA7 로 전진 (할당값 0xA6 보다 앞)
	sm.ObserveFrame(cmd, 0xA7, 0x50)

	// 세 번째 할당: 관찰값 기반 0xA8
	if got := sm.AllocSEQ0(cmd); got != 0xA8 {
		t.Errorf("alloc3 after advance: want 0xA8, got 0x%02X", got)
	}
}

// ---------------------------------------------------------------------------
// TestIcp02SequenceManager_Concurrent
// 100개 고루틴에서 동시 호출해도 panic 없이 동작해야 한다. (-race 플래그 검증)
// ---------------------------------------------------------------------------

func TestIcp02SequenceManager_Concurrent(t *testing.T) {
	sm := NewIcp02SequenceManager()
	cmd := [2]byte{0x03, 0x01}

	var wg sync.WaitGroup
	const goroutines = 100

	// ObserveFrame + NextSEQ0 + NextSEQ1 동시 접근
	wg.Add(goroutines * 3)
	for i := 0; i < goroutines; i++ {
		go func(n byte) {
			defer wg.Done()
			sm.ObserveFrame(cmd, n, n)
		}(byte(i))
		go func() {
			defer wg.Done()
			sm.NextSEQ0(cmd)
		}()
		go func() {
			defer wg.Done()
			sm.NextSEQ1()
		}()
	}
	wg.Wait()
}
