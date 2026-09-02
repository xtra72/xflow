package node

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent/system"
)

// ===========================================================================
// Module 7 (v1.2.0): weekly 스케줄 타입
// ===========================================================================

// cronExprs 는 mock timer 의 SetCron 호출에서 cron 표현식만 추출한다.
func cronExprs(calls []mockTimerCall) []string {
	out := make([]string, 0, len(calls))
	for _, c := range calls {
		if c.Method == "SetCron" {
			out = append(out, c.CronExpr)
		}
	}
	return out
}

func TestTriggerNode_Weekly_MultiDayTimeCombination(t *testing.T) {
	// AC-NODE-004-35: weekly 다중 요일×시각 조합 cron 등록
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{
				"type":  "weekly",
				"days":  []any{"mon", "wed", "fri"},
				"times": []any{"09:00", "18:00"},
			},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	calls := timer.getCalls()
	require.Len(t, calls, 6, "3요일 × 2시각 = 6개 cron 이 등록되어야 한다")

	got := cronExprs(calls)
	sort.Strings(got)
	want := []string{
		"0 18 * * 1", "0 18 * * 3", "0 18 * * 5",
		"0 9 * * 1", "0 9 * * 3", "0 9 * * 5",
	}
	sort.Strings(want)
	assert.Equal(t, want, got)

	// 임의의 핸들러를 호출하면 weekly 메타데이터를 가진 메시지가 생성되어야 한다
	timer.triggerHandler(calls[0].ID, system.TimerTrigger{
		TimerID:   system.TimerID(calls[0].ID),
		TriggerAt: time.Now(),
		TickCount: 1,
	})
	msgs := drainSourceCh(node, 100*time.Millisecond)
	require.Len(t, msgs, 1)
	v, ok := msgs[0].Metadata().Get("trigger.schedule_type")
	assert.True(t, ok)
	assert.Equal(t, "weekly", v)
}

func TestTriggerNode_Weekly_NumericDayTokens(t *testing.T) {
	// AC-NODE-004-36: 정수 요일 토큰 (0=일, 6=토)
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{
				"type":  "weekly",
				"days":  []any{0, 6},
				"times": []any{"10:00"},
			},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	got := cronExprs(timer.getCalls())
	sort.Strings(got)
	want := []string{"0 10 * * 0", "0 10 * * 6"}
	sort.Strings(want)
	assert.Equal(t, want, got)
}

func TestTriggerNode_Weekly_InvalidDayToken(t *testing.T) {
	// AC-NODE-004-36: 잘못된 요일 토큰은 ErrTriggerInvalidScheduleValue
	config := map[string]any{
		"schedules": []any{
			map[string]any{
				"type":  "weekly",
				"days":  []any{"funday"},
				"times": []any{"10:00"},
			},
		},
		"_timer_agent": newMockTimer(),
	}
	def := newTriggerDef(config)
	node, err := NewTriggerNode(def)
	require.NoError(t, err)
	require.NoError(t, node.Configure(config))
	err = node.Init(context.Background())
	assert.ErrorIs(t, err, ErrTriggerInvalidScheduleValue)
}

func TestTriggerNode_Weekly_InvalidTime(t *testing.T) {
	// AC-NODE-004-36: 잘못된 시각은 ErrTriggerInvalidScheduleValue
	config := map[string]any{
		"schedules": []any{
			map[string]any{
				"type":  "weekly",
				"days":  []any{"mon"},
				"times": []any{"25:61"},
			},
		},
		"_timer_agent": newMockTimer(),
	}
	def := newTriggerDef(config)
	node, err := NewTriggerNode(def)
	require.NoError(t, err)
	require.NoError(t, node.Configure(config))
	err = node.Init(context.Background())
	assert.ErrorIs(t, err, ErrTriggerInvalidScheduleValue)
}
