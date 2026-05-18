package century

import (
	"sync"
	"time"
)

// CycleTracker 는 Century polling cycle 의 경계를 휴리스틱으로 감지한다 (REQ-CENTURY-027).
//
// 경계 판정 신호 우선순위 (SPEC §5.6):
//  1. 1차 신호: 마지막 reg 0x04 read response (slave→master FC=0x06, register=0x02
//     prefix 가 0x04) 직후를 cycle 경계점으로 마킹. 다음 frame 부터 새 cycle.
//  2. 2차 신호 (fallback): inter-frame idle 이 cycleIdleTimeout 을 strict 하게 초과하면
//     새 cycle 로 간주한다.
//
// CycleTracker 는 concurrent-safe 하지만, 일반적으로 captureLoop 단일 goroutine 에서만 사용된다.
type CycleTracker struct {
	idleTimeout time.Duration

	mu              sync.Mutex
	lastFrameAt     time.Time
	currentCycle    uint64
	boundaryPending bool // 이전 frame 이 reg 0x04 응답이었음 — 다음 frame 은 새 cycle 시작
	hasLastFrame    bool // 첫 frame 여부 판정용
}

// NewCycleTracker 는 주어진 idleTimeout 으로 새 CycleTracker 를 생성한다.
//
// idleTimeout 은 strict 비교에 사용되므로, 정확히 idleTimeout 만큼의 gap 은 같은 cycle 로 분류된다
// (REQ-CENTURY-027 의 "초과" 의미와 일관).
func NewCycleTracker(idleTimeout time.Duration) *CycleTracker {
	return &CycleTracker{idleTimeout: idleTimeout}
}

// OnFrame 은 새 frame 을 관측했음을 알리고, 그 frame 이 속하는 cycle id 를 반환한다.
//
// 첫 호출은 cycle 1 부터 시작한다 (0 은 "초기화 전" 의미로 예약).
// 두 가지 조건 중 하나가 충족되면 새 cycle 로 전이한다:
//   - 직전 frame 이 reg 0x04 read response 였다 (1차 신호, AC-F3)
//   - idle 이 idleTimeout 을 strict 하게 초과한다 (2차 신호, AC-F4)
//
// 호출 후 frame 이 reg 0x04 read response 이면 boundaryPending 을 set 하여 다음 호출에서
// 새 cycle 로 전이시킨다.
func (c *CycleTracker) OnFrame(f *Frame, now time.Time) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch {
	case !c.hasLastFrame:
		// 첫 frame.
		c.currentCycle = 1
		c.hasLastFrame = true
	case c.boundaryPending:
		// 이전 frame 이 reg 0x04 응답 마커였다 — 새 cycle.
		c.currentCycle++
	case now.Sub(c.lastFrameAt) > c.idleTimeout:
		// idle 초과 (2차 fallback).
		c.currentCycle++
	}

	c.lastFrameAt = now
	c.boundaryPending = isReg04ReadResponse(f)
	return c.currentCycle
}

// CurrentCycle 은 마지막으로 OnFrame 이 반환한 cycle id 를 반환한다.
// 아직 어떤 frame 도 관측되지 않았다면 0 을 반환한다.
func (c *CycleTracker) CurrentCycle() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentCycle
}

// isReg04ReadResponse 는 frame 이 reg 0x04 read response (FC=0x06, register=0x04, slave→master,
// data 길이 ≥ 14B) 인지 검사한다.
//
// ACK (payload_length=1) 와 reg 0x02/0x03 응답, 그리고 reg 0x04 write 는 false 를 반환한다.
func isReg04ReadResponse(f *Frame) bool {
	if f == nil {
		return false
	}
	if f.FunctionCode != FCResponse {
		return false
	}
	if f.IsACK() {
		return false
	}
	reg, ok := f.Register()
	if !ok {
		return false
	}
	return reg == 0x04
}
