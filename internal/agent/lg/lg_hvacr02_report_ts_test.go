// lg_hvacr02_report_ts_test.go — report 타임스탬프가 발행 시각(now)인지 검증.
//
// 배경: 노드는 last_seen_ms 를 메시지 타임스탬프로 사용한다. 이전에는 모든 trigger 가
// dev.LastSeen(마지막 프레임 시각)을 썼기 때문에, OFF 처럼 조용한 디바이스의 주기 report
// 가 모두 "마지막 프레임 시각"으로 찍혀 store 에 같은 시각으로 쌓였다("보고 주기 안 맞음").
// report 는 발행 시점(now), change 는 프레임 시각(LastSeen)을 쓰도록 수정.
package lg

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lastRecentTimestamp(t *testing.T, a *Hvacr02Agent) time.Time {
	t.Helper()
	a.recentMu.RLock()
	defer a.recentMu.RUnlock()
	idx := (a.recentIdx - 1 + hvacr02RecentBufferSize) % hvacr02RecentBufferSize
	return a.recentFrames[idx].Timestamp
}

// 조용한 디바이스(LastSeen 과거)의 주기 report 는 발행 시각(now)으로 찍혀야 한다.
func TestReportTimestamp_IsEmitTimeNotStaleLastSeen(t *testing.T) {
	a := newControllerMergeTestAgent(t)
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("OFF")}, time.Now())
	dev := a.devices["44550066"]
	require.NotNil(t, dev)

	// 마지막 프레임 시각을 10분 전으로 (조용한 디바이스 모사).
	old := time.Now().Add(-10 * time.Minute)
	a.mu.Lock()
	dev.LastSeen = old
	a.mu.Unlock()

	// 주기 report 발행.
	a.mu.Lock()
	a.emitDeviceStateLocked(dev, "report")
	a.mu.Unlock()

	ts := lastRecentTimestamp(t, a)
	assert.WithinDuration(t, time.Now(), ts, 5*time.Second,
		"report 타임스탬프는 발행 시각(now)이어야 한다")
	assert.True(t, ts.After(old.Add(time.Minute)),
		"report 타임스탬프가 과거 LastSeen 으로 찍히면 안 된다")
}

// change 는 이벤트(프레임) 시각(LastSeen)으로 찍혀야 한다.
func TestChangeTimestamp_IsFrameTime(t *testing.T) {
	a := newControllerMergeTestAgent(t)
	// 최초 관측 (OFF, report).
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("OFF")}, time.Now())

	// 특정 프레임 시각에 변경(OFF→ON) 발생.
	frameTime := time.Now().Add(-3 * time.Minute)
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("ON"), IndoorTempC: f64Ptr(24.0)}, frameTime)

	ts := lastRecentTimestamp(t, a)
	assert.WithinDuration(t, frameTime, ts, time.Second,
		"change 타임스탬프는 이벤트(프레임) 시각이어야 한다")
}
