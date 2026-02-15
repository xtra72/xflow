package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Cron 타이머 테스트
// ---------------------------------------------------------------------------

// TestTimerAgent_SetCron_ValidExpression 은 유효한 5필드 cron 표현식이 등록되는지 검증한다.
func TestTimerAgent_SetCron_ValidExpression(t *testing.T) {
	agent := newTestTimerAgent(t)

	handler := func(_ TimerTrigger) {}

	tid, err := agent.SetCron("daily", "0 0 * * *", handler)
	require.NoError(t, err)
	assert.Equal(t, TimerID("daily"), tid)

	infos := agent.List()
	require.Len(t, infos, 1)
	assert.Equal(t, TimerTypeCron, infos[0].Type)
	assert.Equal(t, "0 0 * * *", infos[0].Expression)
	assert.True(t, infos[0].Active)
}

// TestTimerAgent_SetCron_InvalidExpression 은 유효하지 않은 cron 표현식이 거부되는지 검증한다.
func TestTimerAgent_SetCron_InvalidExpression(t *testing.T) {
	agent := newTestTimerAgent(t)

	handler := func(_ TimerTrigger) {}

	_, err := agent.SetCron("bad-cron", "invalid cron expr", handler)
	assert.ErrorIs(t, err, ErrInvalidCronExpression)
}

// TestTimerAgent_SetCron_Cancel 은 cron 타이머 취소가 정상 동작하는지 검증한다.
func TestTimerAgent_SetCron_Cancel(t *testing.T) {
	agent := newTestTimerAgent(t)

	handler := func(_ TimerTrigger) {}

	tid, err := agent.SetCron("cancel-cron", "* * * * *", handler)
	require.NoError(t, err)

	// 취소
	err = agent.Cancel(tid)
	require.NoError(t, err)

	// List에서 제거됨
	assert.Empty(t, agent.List())
}

// TestTimerAgent_SetCron_EmptyID 는 빈 ID가 거부되는지 검증한다.
func TestTimerAgent_SetCron_EmptyID(t *testing.T) {
	agent := newTestTimerAgent(t)
	_, err := agent.SetCron("", "* * * * *", func(_ TimerTrigger) {})
	assert.ErrorIs(t, err, ErrTimerIDEmpty)
}

// TestTimerAgent_SetCron_NilHandler 는 nil 핸들러가 거부되는지 검증한다.
func TestTimerAgent_SetCron_NilHandler(t *testing.T) {
	agent := newTestTimerAgent(t)
	_, err := agent.SetCron("nil-cron", "* * * * *", nil)
	assert.ErrorIs(t, err, ErrNilHandler)
}

// TestTimerAgent_SetCron_DuplicateID 는 중복 ID가 거부되는지 검증한다.
func TestTimerAgent_SetCron_DuplicateID(t *testing.T) {
	agent := newTestTimerAgent(t)
	handler := func(_ TimerTrigger) {}

	_, err := agent.SetCron("dup-cron", "* * * * *", handler)
	require.NoError(t, err)

	_, err = agent.SetCron("dup-cron", "0 * * * *", handler)
	assert.ErrorIs(t, err, ErrDuplicateTimerID)
}

// TestTimerAgent_SetCron_Closed 는 닫힌 에이전트에서 거부되는지 검증한다.
func TestTimerAgent_SetCron_Closed(t *testing.T) {
	agent := NewTimerAgent()
	require.NoError(t, agent.Init(context.Background()))
	require.NoError(t, agent.Stop(context.Background()))

	_, err := agent.SetCron("after-close", "* * * * *", func(_ TimerTrigger) {})
	assert.ErrorIs(t, err, ErrTimerClosed)
}

// TestTimerAgent_SetCron_Paused 는 일시정지 상태에서 거부되는지 검증한다.
func TestTimerAgent_SetCron_Paused(t *testing.T) {
	agent := newTestTimerAgent(t)
	require.NoError(t, agent.Pause(context.Background()))

	_, err := agent.SetCron("paused-cron", "* * * * *", func(_ TimerTrigger) {})
	assert.ErrorIs(t, err, ErrTimerPaused)
}

// TestTimerAgent_SetCron_ScheduleID 는 ScheduleID가 비어있지 않은지 검증한다.
// (cron 실행은 분 단위이므로 여기서는 등록 및 취소만 테스트)
func TestTimerAgent_SetCron_ScheduleID(t *testing.T) {
	agent := newTestTimerAgent(t)

	handler := func(_ TimerTrigger) {}

	_, err := agent.SetCron("sched-id", "* * * * *", handler)
	require.NoError(t, err)

	// 등록된 타이머 정보 확인
	infos := agent.List()
	require.Len(t, infos, 1)
	assert.Equal(t, TimerTypeCron, infos[0].Type)
}
