package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent/system"
)

// ===========================================================================
// Module 7 (v1.2.0): monthly 스케줄 타입
// ===========================================================================

func TestTriggerNode_Monthly_SpecificDay(t *testing.T) {
	// AC-NODE-004-37: monthly 특정 일자 cron 등록
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "monthly", "day": 15, "times": []any{"08:30"}},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	calls := timer.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "SetCron", calls[0].Method)
	assert.Equal(t, "30 8 15 * *", calls[0].CronExpr)

	timer.triggerHandler(calls[0].ID, system.TimerTrigger{
		TimerID:   system.TimerID(calls[0].ID),
		TriggerAt: time.Now(),
		TickCount: 1,
	})
	msgs := drainSourceCh(node, 100*time.Millisecond)
	require.Len(t, msgs, 1)
	v, ok := msgs[0].Metadata().Get("trigger.schedule_type")
	assert.True(t, ok)
	assert.Equal(t, "monthly", v)
}

func TestTriggerNode_Monthly_First(t *testing.T) {
	// AC-NODE-004-38: monthly "first" 일자 cron 등록 (first = 1일)
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "monthly", "day": "first", "times": []any{"00:00"}},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	calls := timer.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "0 0 1 * *", calls[0].CronExpr)
}

func TestTriggerNode_Monthly_NonExistentIntegerDay(t *testing.T) {
	// AC-NODE-004-40: monthly 존재하지 않는 정수 일자 (표준 cron, 클램핑 없음)
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "monthly", "day": 31, "times": []any{"10:00"}},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	calls := timer.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "0 10 31 * *", calls[0].CronExpr)
}

func TestTriggerNode_Monthly_InvalidDay(t *testing.T) {
	// day 값이 범위 밖(0, 32) 또는 알 수 없는 문자열이면 ErrTriggerInvalidScheduleValue
	cases := []any{0, 32, "someday"}
	for _, day := range cases {
		config := map[string]any{
			"schedules": []any{
				map[string]any{"type": "monthly", "day": day, "times": []any{"10:00"}},
			},
			"_timer_agent": newMockTimer(),
		}
		def := newTriggerDef(config)
		node, err := NewTriggerNode(def)
		require.NoError(t, err)
		require.NoError(t, node.Configure(config))
		err = node.Init(context.Background())
		assert.ErrorIs(t, err, ErrTriggerInvalidScheduleValue, "day=%v 는 거부되어야 한다", day)
	}
}

// TestTriggerNode_Monthly_LastDayGate 는 monthly "last" 월말 emit 게이트를
// 주입된 고정 clock 으로 검증한다. (AC-NODE-004-39)
func TestTriggerNode_Monthly_LastDayGate(t *testing.T) {
	// 먼저 등록되는 cron 표현식이 매일 cron 인지 확인
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "monthly", "day": "last", "times": []any{"23:59"}},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	calls := timer.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "59 23 * * *", calls[0].CronExpr,
		"last 는 매일 cron 으로 등록되어야 한다 (마지막 날 직접 표현 불가)")

	timerID := calls[0].ID

	cases := []struct {
		name     string
		now      time.Time
		wantEmit bool
	}{
		{"2월 마지막 날 (28일)", time.Date(2026, 2, 28, 23, 59, 0, 0, time.UTC), true},
		{"2월 월말 아님 (27일)", time.Date(2026, 2, 27, 23, 59, 0, 0, time.UTC), false},
		{"윤년 2월 마지막 날 (29일)", time.Date(2028, 2, 29, 23, 59, 0, 0, time.UTC), true},
		{"30일 달 마지막 날 (4월 30일)", time.Date(2026, 4, 30, 23, 59, 0, 0, time.UTC), true},
		{"31일 달 마지막 날 (1월 31일)", time.Date(2026, 1, 31, 23, 59, 0, 0, time.UTC), true},
	}

	tn := node.(*TriggerNode)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixed := tc.now
			tn.setNowFunc(func() time.Time { return fixed })

			timer.triggerHandler(timerID, system.TimerTrigger{
				TimerID:   system.TimerID(timerID),
				TriggerAt: fixed,
				TickCount: 1,
			})

			msgs := drainSourceCh(node, 80*time.Millisecond)
			if tc.wantEmit {
				assert.Len(t, msgs, 1, "월 마지막 날에는 메시지가 emit 되어야 한다")
			} else {
				assert.Empty(t, msgs, "월말이 아니면 메시지가 emit 되지 않아야 한다 (게이트)")
			}
		})
	}
}
