package century

import (
	"testing"
	"time"
)

// frameSlaveResponse는 reg N에 대한 슬레이브 응답 frame을 합성하여 cycle tracker 테스트에 사용한다.
func frameSlaveResponse(reg byte) *Frame {
	return &Frame{
		Src: AddrSlave, Dst: AddrMaster,
		FunctionCode: FCResponse,
		Payload:      []byte{0x3B, 0x00, reg, 0x00}, // sub_dev_id, reserved2, register, data...
	}
}

// frameMasterWrite는 마스터의 reg 0x04 write frame을 합성한다.
func frameMasterWrite() *Frame {
	return &Frame{
		Src: AddrMaster, Dst: AddrSlave,
		FunctionCode: FCWrite,
		Payload:      []byte{0x3B, 0x00, 0x04, 0x00},
	}
}

func TestCycleTracker_FirstFrameStartsCycle(t *testing.T) {
	t.Parallel()
	ct := NewCycleTracker(100 * time.Millisecond)
	t0 := time.Unix(0, 0)
	id := ct.OnFrame(frameMasterWrite(), t0)
	if id == 0 {
		t.Fatalf("first cycle id = 0, want >= 1")
	}
}

func TestCycleTracker_Reg04ResponseAdvancesCycle(t *testing.T) {
	t.Parallel()
	// AC-F3: reg 0x04 read response 후 다음 frame 은 새 cycle 에 속해야 한다.
	ct := NewCycleTracker(100 * time.Millisecond)
	base := time.Unix(0, 0)

	c1 := ct.OnFrame(frameMasterWrite(), base.Add(0))                                                        // 1. WRITE
	c2 := ct.OnFrame(&Frame{FunctionCode: FCResponse, Payload: []byte{0x00}}, base.Add(10*time.Millisecond)) // 2. ACK (1B)
	c3 := ct.OnFrame(frameSlaveResponse(0x02), base.Add(20*time.Millisecond))                                // 3. reg 0x02 response
	c4 := ct.OnFrame(frameSlaveResponse(0x03), base.Add(30*time.Millisecond))                                // 4. reg 0x03 response
	c5 := ct.OnFrame(frameSlaveResponse(0x04), base.Add(40*time.Millisecond))                                // 5. reg 0x04 response — cycle boundary marker
	c6 := ct.OnFrame(frameMasterWrite(), base.Add(50*time.Millisecond))                                      // 6. WRITE — new cycle

	if c1 != c2 || c2 != c3 || c3 != c4 || c4 != c5 {
		t.Errorf("frames 1..5 should share cycle id, got [%d,%d,%d,%d,%d]", c1, c2, c3, c4, c5)
	}
	if c6 == c5 {
		t.Errorf("frame 6 (after reg 0x04 marker) should be a new cycle, got same id %d", c6)
	}
	if c6 != c5+1 {
		t.Errorf("frame 6 cycle id = %d, want %d (monotonic)", c6, c5+1)
	}
}

func TestCycleTracker_IdleFallbackAdvancesCycle(t *testing.T) {
	t.Parallel()
	// AC-F4: 100ms idle 초과 시 새 cycle.
	ct := NewCycleTracker(100 * time.Millisecond)
	base := time.Unix(0, 0)

	c1 := ct.OnFrame(frameMasterWrite(), base)
	c2 := ct.OnFrame(frameMasterWrite(), base.Add(150*time.Millisecond)) // 150ms gap > 100ms → new cycle.
	if c2 == c1 {
		t.Errorf("frame after idle > 100ms should be new cycle, got same id %d", c2)
	}
	if c2 != c1+1 {
		t.Errorf("idle cycle id = %d, want %d", c2, c1+1)
	}
}

func TestCycleTracker_IdleUnderThresholdSameCycle(t *testing.T) {
	t.Parallel()
	// AC-F4 lower half: 50ms idle stays in same cycle.
	ct := NewCycleTracker(100 * time.Millisecond)
	base := time.Unix(0, 0)

	c1 := ct.OnFrame(frameMasterWrite(), base)
	c2 := ct.OnFrame(frameMasterWrite(), base.Add(50*time.Millisecond))
	if c1 != c2 {
		t.Errorf("frame after idle 50ms (< 100ms) should stay in same cycle, got [%d,%d]", c1, c2)
	}
}

func TestCycleTracker_IdleAtExactThresholdSameCycle(t *testing.T) {
	t.Parallel()
	// Boundary: exactly 100ms is not a new cycle (strict >).
	ct := NewCycleTracker(100 * time.Millisecond)
	base := time.Unix(0, 0)
	c1 := ct.OnFrame(frameMasterWrite(), base)
	c2 := ct.OnFrame(frameMasterWrite(), base.Add(100*time.Millisecond))
	if c1 != c2 {
		t.Errorf("frame at exactly 100ms idle should stay in same cycle, got [%d,%d]", c1, c2)
	}
}

func TestCycleTracker_NonReg04ResponseDoesNotAdvance(t *testing.T) {
	t.Parallel()
	// reg 0x02 or 0x03 response should NOT mark a cycle boundary.
	ct := NewCycleTracker(100 * time.Millisecond)
	base := time.Unix(0, 0)
	c1 := ct.OnFrame(frameSlaveResponse(0x02), base)
	c2 := ct.OnFrame(frameSlaveResponse(0x03), base.Add(10*time.Millisecond))
	c3 := ct.OnFrame(frameMasterWrite(), base.Add(20*time.Millisecond))
	if c1 != c2 || c2 != c3 {
		t.Errorf("non-marker frames should share cycle id, got [%d,%d,%d]", c1, c2, c3)
	}
}

func TestCycleTracker_CurrentCycleReturnsLatest(t *testing.T) {
	t.Parallel()
	ct := NewCycleTracker(100 * time.Millisecond)
	base := time.Unix(0, 0)
	_ = ct.OnFrame(frameMasterWrite(), base)
	got := ct.OnFrame(frameSlaveResponse(0x04), base.Add(5*time.Millisecond))
	if ct.CurrentCycle() != got {
		t.Errorf("CurrentCycle = %d, want %d", ct.CurrentCycle(), got)
	}
}
