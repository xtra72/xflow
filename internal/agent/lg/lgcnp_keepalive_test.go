package lg

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestLGCNPAgent_ShouldEmitIDU_Keepalive 는 StateReportInterval 설정 시
// 동일 state 라도 interval 경과 후 emit 강제 (v0.18.18).
func TestLGCNPAgent_ShouldEmitIDU_Keepalive(t *testing.T) {
	t.Parallel()

	a := &LGCNPAgent{
		dedupMu:       sync.Mutex{},
		lastIDUEmit:   make(map[int][]byte),
		lastIDUParsed: make(map[int]LGCNPIDUParsed),
		lastIDUEmitAt: make(map[int]time.Time),
		lgcnpConfig: LGCNPConfig{
			StateReportInterval: 30 * time.Second,
		},
	}

	state := &LGCNPIDUParsed{
		Power:       true,
		TargetTemp:  25.0,
		CurrentTemp: 24.0,
		Mode:        1,
		FanSpeed:    3,
	}

	// 첫 emit → true.
	emit, _ := a.shouldEmitIDU(1, state)
	assert.True(t, emit, "첫 emit 은 true")

	// 동일 state 즉시 반복 → false (interval 미경과).
	emit, reason := a.shouldEmitIDU(1, state)
	assert.False(t, emit, "interval 미경과: dedup")
	assert.Equal(t, "identical", reason)

	// 30s 경과 시뮬레이션 — lastIDUEmitAt 을 과거로 조정.
	a.dedupMu.Lock()
	a.lastIDUEmitAt[1] = time.Now().Add(-31 * time.Second)
	a.dedupMu.Unlock()

	// 동일 state 여도 interval 경과 → true (keepalive).
	emit, _ = a.shouldEmitIDU(1, state)
	assert.True(t, emit, "interval 경과: keepalive emit 강제")
}

// TestLGCNPAgent_ShouldEmitIDU_KeepaliveDisabled 는 StateReportInterval=0
// 일 때 기존 dedup 동작 유지 (v0.18.18 regression 보호).
func TestLGCNPAgent_ShouldEmitIDU_KeepaliveDisabled(t *testing.T) {
	t.Parallel()

	a := &LGCNPAgent{
		dedupMu:       sync.Mutex{},
		lastIDUEmit:   make(map[int][]byte),
		lastIDUParsed: make(map[int]LGCNPIDUParsed),
		lastIDUEmitAt: make(map[int]time.Time),
		lgcnpConfig: LGCNPConfig{
			StateReportInterval: 0, // 비활성
		},
	}

	state := &LGCNPIDUParsed{Power: true, TargetTemp: 25.0}

	emit, _ := a.shouldEmitIDU(1, state)
	assert.True(t, emit, "첫 emit")

	// 어떤 시간 경과여도 dedup 유지.
	a.dedupMu.Lock()
	a.lastIDUEmitAt[1] = time.Now().Add(-24 * time.Hour)
	a.dedupMu.Unlock()

	emit, reason := a.shouldEmitIDU(1, state)
	assert.False(t, emit, "interval=0: 무조건 dedup")
	assert.Equal(t, "identical", reason)
}

// TestLGCNPAgent_ShouldEmitODU_Keepalive 는 ODU 도 동일하게 keepalive 동작.
func TestLGCNPAgent_ShouldEmitODU_Keepalive(t *testing.T) {
	t.Parallel()

	a := &LGCNPAgent{
		dedupMu: sync.Mutex{},
		lgcnpConfig: LGCNPConfig{
			StateReportInterval: 30 * time.Second,
		},
	}

	outdoor := 25.5
	state := &LGCNPODUParsed{OutdoorTemp: &outdoor}

	emit, _ := a.shouldEmitODU(state)
	assert.True(t, emit, "첫 emit")

	emit, reason := a.shouldEmitODU(state)
	assert.False(t, emit, "interval 미경과: dedup")
	assert.Equal(t, "identical", reason)

	a.dedupMu.Lock()
	a.lastODUEmitAt = time.Now().Add(-31 * time.Second)
	a.dedupMu.Unlock()

	emit, _ = a.shouldEmitODU(state)
	assert.True(t, emit, "interval 경과: keepalive emit 강제")
}
