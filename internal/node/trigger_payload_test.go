package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/pkg/message"
)

// ===========================================================================
// Module 7 (v1.2.0): per-schedule payload (스케줄별 페이로드)
// ===========================================================================

// fireInterval 은 interval-0 타이머 핸들러를 발화시키고 수신 메시지를 반환한다.
func fireInterval(t *testing.T, node Node, timer *mockTimer, tickCount int64) message.Message {
	t.Helper()
	timer.triggerHandler("trigger-1-interval-0", system.TimerTrigger{
		TimerID:   "trigger-1-interval-0",
		TriggerAt: time.Now(),
		TickCount: tickCount,
	})
	msgs := drainSourceCh(node, 100*time.Millisecond)
	require.Len(t, msgs, 1)
	return msgs[0]
}

func TestTriggerNode_PerSchedulePayload_Priority(t *testing.T) {
	// AC-NODE-004-41: 스케줄 항목 페이로드가 노드 레벨보다 우선
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{
				"type":    "interval",
				"value":   "50ms",
				"payload": map[string]any{"src": "schedule"},
			},
		},
		"payload": map[string]any{"src": "node"},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	msg := fireInterval(t, node, timer, 1)
	v, ok := msg.Payload().Get("src")
	require.True(t, ok)
	assert.Equal(t, "schedule", v, "스케줄 항목 페이로드가 우선되어야 한다")
}

func TestTriggerNode_PerSchedulePayload_FallbackToNode(t *testing.T) {
	// AC-NODE-004-42: 스케줄 항목 페이로드 없으면 노드 레벨로 폴백
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "50ms"},
		},
		"payload": map[string]any{"src": "node"},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	msg := fireInterval(t, node, timer, 1)
	v, ok := msg.Payload().Get("src")
	require.True(t, ok)
	assert.Equal(t, "node", v, "노드 레벨 페이로드로 폴백되어야 한다")
}

func TestTriggerNode_PerSchedulePayload_FallbackToDefault(t *testing.T) {
	// AC-NODE-004-42: 스케줄·노드 레벨 페이로드 모두 없으면 기본 페이로드
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "50ms"},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	msg := fireInterval(t, node, timer, 1)
	_, ok := msg.Payload().Get("trigger_time")
	assert.True(t, ok, "기본 페이로드로 폴백되어 trigger_time 이 있어야 한다")
}

func TestTriggerNode_PerScheduleTemplate_Priority(t *testing.T) {
	// AC-NODE-004-43: 스케줄 항목 payload_template 우선 치환
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{
				"type":             "interval",
				"value":            "50ms",
				"payload_template": map[string]any{"c": "$.tick_count"},
			},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	msg := fireInterval(t, node, timer, 2)
	v, ok := msg.Payload().Get("c")
	require.True(t, ok)
	assert.Equal(t, int64(2), v, "스케줄 항목 템플릿이 치환되어야 한다")
}

func TestTriggerNode_BackwardCompat_TimesWithNodePayload(t *testing.T) {
	// AC-NODE-004-44: 하위 호환 - 기존 times + 노드 레벨 payload 동작 불변
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "times", "value": []any{"09:00", "18:00"}},
		},
		"payload": map[string]any{"t": 1},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	calls := timer.getCalls()
	require.Len(t, calls, 2)
	assert.Equal(t, "0 9 * * *", calls[0].CronExpr)
	assert.Equal(t, "0 18 * * *", calls[1].CronExpr)

	// 트리거 시 노드 레벨 payload 가 사용되어야 한다 (v1.1.0 동일)
	timer.triggerHandler(calls[0].ID, system.TimerTrigger{
		TimerID:   system.TimerID(calls[0].ID),
		TriggerAt: time.Now(),
		TickCount: 1,
	})
	msgs := drainSourceCh(node, 100*time.Millisecond)
	require.Len(t, msgs, 1)
	v, ok := msgs[0].Payload().Get("t")
	require.True(t, ok)
	assert.Equal(t, 1, v)
}
