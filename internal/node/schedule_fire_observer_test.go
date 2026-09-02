package node

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent/system"
)

// ---------------------------------------------------------------------------
// SPEC-SCHEDULE-VIEW-001 M2 — trigger 측 발화 관측자 + agent_id 메타 테스트
// ---------------------------------------------------------------------------

// captureObserver 는 node.ScheduleFireObserver 의 테스트용 캡처 관측자이다.
type captureObserver struct {
	mu    sync.Mutex
	calls []ScheduleFireContext
}

func (o *captureObserver) OnScheduleFire(ctx ScheduleFireContext) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls = append(o.calls, ctx)
}

func (o *captureObserver) snapshot() []ScheduleFireContext {
	o.mu.Lock()
	defer o.mu.Unlock()
	cp := make([]ScheduleFireContext, len(o.calls))
	copy(cp, o.calls)
	return cp
}

// AC-13 특성화(무회귀): agent_id 없는 스케줄은 이전과 동일한 메타 집합을 방출하며
// trigger.agent_id 키를 추가하지 않는다. name/priority 무메타 특성화(trigger_sched_test)와 동일 결계.
func TestScheduleFire_NoAgentIDMeta_OnDefaultSchedule(t *testing.T) {
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s"},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	fireTrigger(timer, "trigger-1-interval-0")
	msgs := drainSourceCh(node, 80*time.Millisecond)
	require.Len(t, msgs, 1)

	_, hasAgent := msgs[0].Metadata().Get("trigger.agent_id")
	assert.False(t, hasAgent, "agent_id 없는 스케줄은 trigger.agent_id 메타를 추가하지 않아야 한다(AC-13)")

	// 기존 메타 집합은 그대로 유지된다(무회귀 스팟 체크).
	for _, key := range []string{"trigger.schedule_type", "trigger.schedule_id", "trigger.trigger_time", "trigger.node_name"} {
		_, ok := msgs[0].Metadata().Get(key)
		assert.True(t, ok, "기존 메타 %s 는 유지되어야 한다", key)
	}
}

// agent_id 가 선언된 스케줄은 trigger.agent_id 메타를 방출한다(파싱·엔트리 캡처·pass-through).
func TestScheduleFire_AgentIDParsedAndCarried(t *testing.T) {
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s", "agent_id": "hvac-1", "name": "야간 소등"},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	tn := node.(*TriggerNode)
	defer node.Shutdown(context.Background())

	assert.Equal(t, "hvac-1", tn.schedules[0].AgentID, "config agent_id 파싱")
	assert.Equal(t, "hvac-1", tn.timerEntries[0].agentID, "엔트리 캡처")

	fireTrigger(timer, "trigger-1-interval-0")
	msgs := drainSourceCh(node, 80*time.Millisecond)
	require.Len(t, msgs, 1)
	v, ok := msgs[0].Metadata().Get("trigger.agent_id")
	require.True(t, ok, "agent_id 는 방출 메시지 메타에 통과 전달되어야 한다")
	assert.Equal(t, "hvac-1", v)
}

// AC-15 fire-only: 발화마다 관측자가 호출되며 correlation_id/선언 대상/발화 시각을 전달한다.
// 다운스트림 제어 유무와 무관하게 fire 이벤트는 항상 포착된다.
func TestScheduleFire_ObserverCalledOnEveryFire(t *testing.T) {
	obs := &captureObserver{}
	SetScheduleFireObserver(obs)
	defer SetScheduleFireObserver(nil)

	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s", "agent_id": "hvac-1", "name": "야간 소등"},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	triggerAt := time.Now()
	timer.triggerHandler("trigger-1-interval-0", system.TimerTrigger{
		TimerID:   "trigger-1-interval-0",
		TriggerAt: triggerAt,
		TickCount: 1,
	})

	calls := obs.snapshot()
	require.Len(t, calls, 1, "발화마다 관측자가 1회 호출되어야 한다(AC-15)")
	c := calls[0]
	assert.Equal(t, "trigger-1-interval-0", c.ScheduleID)
	assert.Equal(t, "hvac-1", c.DeclaredAgentID)
	assert.Equal(t, "야간 소등", c.RuleName)
	assert.Equal(t, triggerAt.UnixMilli(), c.TriggerTime, "발화 시각 epoch ms")
	// correlation_id = schedule_id + ":" + trigger_ms (result 이벤트와 동일 공식, AC-16).
	want := "trigger-1-interval-0:" + strconv.FormatInt(triggerAt.UnixMilli(), 10)
	assert.Equal(t, want, c.CorrelationID)
}

// 관측자 미설정(nil)에서도 발화는 crash 없이 정상 동작한다(graceful no-op).
func TestScheduleFire_NilObserverGracefulNoOp(t *testing.T) {
	SetScheduleFireObserver(nil) // 명시적 미설정
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{map[string]any{"type": "interval", "value": "1s", "agent_id": "hvac-1"}},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	fireTrigger(timer, "trigger-1-interval-0")
	msgs := drainSourceCh(node, 80*time.Millisecond)
	require.Len(t, msgs, 1, "관측자 미설정에서도 발화는 정상 방출되어야 한다")
}

// correlation_id 일관성 검증: fire 측(epoch ms)과 result 측(RFC3339 메타 파싱)이 동일 키를 만든다.
// buildXsfmControlCommand 가 실은 trigger.trigger_time(RFC3339)을 파싱한 ms 가 fire 측 ms 와 같음을
// 확인한다(AC-16 조인 정합성).
func TestScheduleFire_CorrelationConsistentWithMetaTriggerTime(t *testing.T) {
	obs := &captureObserver{}
	SetScheduleFireObserver(obs)
	defer SetScheduleFireObserver(nil)

	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{map[string]any{
			"type": "interval", "value": "1s", "agent_id": "hvac-1",
			// 제어 키를 담은 per-schedule payload — buildXsfmControlCommand 가 set_power 로 추론한다.
			"payload": map[string]any{"power": true},
		}},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	triggerAt := time.Now()
	timer.triggerHandler("trigger-1-interval-0", system.TimerTrigger{
		TimerID:   "trigger-1-interval-0",
		TriggerAt: triggerAt,
		TickCount: 1,
	})

	msgs := drainSourceCh(node, 80*time.Millisecond)
	require.Len(t, msgs, 1)

	// result 측 재현: _correlation 을 추출하고 RFC3339 trigger_time 을 ms 로 파싱.
	cmdBytes, err := buildXsfmControlCommand(msgs[0], "dev-1", "node-1")
	require.NoError(t, err)
	var cmd map[string]any
	require.NoError(t, json.Unmarshal(cmdBytes, &cmd))
	corr, ok := cmd["_correlation"].(map[string]any)
	require.True(t, ok, "예약 제어는 _correlation 블록을 실어야 한다")

	rfc3339 := corr["trigger_time"].(string)
	parsed, perr := time.Parse(time.RFC3339Nano, rfc3339)
	require.NoError(t, perr)
	resultMs := parsed.UnixMilli()

	fire := obs.snapshot()[0]
	assert.Equal(t, fire.TriggerTime, resultMs, "fire 측 epoch ms 와 result 측 파싱 ms 가 일치해야 조인된다(AC-16)")
}

// AC-6 특성화(무회귀): 트리거 상관 메타가 없는(수동) 제어 메시지는 _correlation 블록을 실지 않는다.
func TestBuildXsfmControlCommand_NoCorrelation_WhenManual(t *testing.T) {
	t.Parallel()
	// 수동 제어: trigger.* 메타 없음, 제어 키만.
	msg := newXsfmMsg(map[string]any{"device_id": "01", "power": true}, nil)
	out, err := buildXsfmControlCommand(msg, "01", "node-1")
	require.NoError(t, err)
	var cmd map[string]any
	require.NoError(t, json.Unmarshal(out, &cmd))
	_, has := cmd["_correlation"]
	assert.False(t, has, "수동 제어는 _correlation 블록을 실지 않아야 한다(AC-6)")
}

// 예약 제어: schedule_id + trigger_time 메타가 있으면 _correlation 4필드를 모두 실는다.
func TestBuildXsfmControlCommand_CorrelationFields(t *testing.T) {
	t.Parallel()
	meta := map[string]string{
		"trigger.schedule_id":  "sched-9",
		"trigger.trigger_time": "2026-07-31T12:00:00Z",
		"trigger.rule_name":    "야간 소등",
		"trigger.agent_id":     "hvac-1",
	}
	msg := newXsfmMsg(map[string]any{"device_id": "01", "power": true}, meta)
	out, err := buildXsfmControlCommand(msg, "01", "node-1")
	require.NoError(t, err)
	var cmd map[string]any
	require.NoError(t, json.Unmarshal(out, &cmd))
	corr, ok := cmd["_correlation"].(map[string]any)
	require.True(t, ok, "예약 제어는 _correlation 블록을 실어야 한다")
	assert.Equal(t, "sched-9", corr["schedule_id"])
	assert.Equal(t, "야간 소등", corr["rule_name"])
	assert.Equal(t, "hvac-1", corr["declared_agent_id"])
	assert.Equal(t, "2026-07-31T12:00:00Z", corr["trigger_time"])
}

// schedule_id 만 있고 trigger_time 이 없으면 상관을 실지 않는다(둘 다 필요, 전면 생략).
func TestBuildXsfmControlCommand_NoCorrelation_WhenPartialMeta(t *testing.T) {
	t.Parallel()
	meta := map[string]string{"trigger.schedule_id": "sched-9"} // trigger_time 없음
	msg := newXsfmMsg(map[string]any{"device_id": "01", "power": true}, meta)
	out, err := buildXsfmControlCommand(msg, "01", "node-1")
	require.NoError(t, err)
	var cmd map[string]any
	require.NoError(t, json.Unmarshal(out, &cmd))
	_, has := cmd["_correlation"]
	assert.False(t, has, "schedule_id 만으로는 _correlation 을 실지 않아야 한다")
}
