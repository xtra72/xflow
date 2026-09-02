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

// ===========================================================================
// Module 3 (v1.3.0 개정) + Module 8: 통합 페이로드 엔진
//   - 통째 변수 네이티브 타입 치환 / interpolation / $$ 이스케이프
//   - config 하위호환 라우팅(비-map 스칼라/배열 → {"value": v} 래핑)
//   - 미지 변수 에러+null / 중첩 맵 미재귀 / 구 static 마이그레이션 동일 출력
// ===========================================================================

// fireNodePayload 는 노드 레벨 payload 를 가진 interval trigger 를 발화시켜 메시지를 반환한다.
func fireNodePayload(t *testing.T, payload any, tickCount int64) message.Message {
	t.Helper()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "50ms"},
		},
		"payload": payload,
	}
	node, timer := initTriggerWithMock(t, config, newMockTimer())
	t.Cleanup(func() { _ = node.Shutdown(context.Background()) })
	return fireInterval(t, node, timer, tickCount)
}

func TestTriggerNode_Unified_ScalarNumberWrap(t *testing.T) {
	// AC-NODE-004-13: 비-map 숫자 스칼라 → {"value": 42} 래핑
	msg := fireNodePayload(t, 42, 1)
	v, ok := msg.Payload().Get("value")
	require.True(t, ok, "스칼라는 value 키로 래핑되어야 한다")
	assert.Equal(t, 42, v, "value 가 네이티브 숫자 42 여야 한다")
}

func TestTriggerNode_Unified_ScalarStringWrap(t *testing.T) {
	// AC-NODE-004-14: 문자열 스칼라 → {"value": "hello"} 래핑 (리터럴)
	msg := fireNodePayload(t, "hello", 1)
	v, ok := msg.Payload().Get("value")
	require.True(t, ok)
	assert.Equal(t, "hello", v, "$. / $$ 없으므로 리터럴로 통과해야 한다")
}

func TestTriggerNode_Unified_MapDirectEval(t *testing.T) {
	// AC-NODE-004-15: map 은 래핑 없이 직접 평가 (비문자열 패스스루 + 리터럴)
	msg := fireNodePayload(t, map[string]any{"temperature": 25.5, "status": "active"}, 1)
	temp, ok := msg.Payload().Get("temperature")
	require.True(t, ok)
	assert.Equal(t, 25.5, temp, "비문자열 값은 패스스루되어야 한다")
	status, ok := msg.Payload().Get("status")
	require.True(t, ok)
	assert.Equal(t, "active", status, "$. / $$ 없으므로 리터럴")
	_, wrapped := msg.Payload().Get("value")
	assert.False(t, wrapped, "map 은 value 로 래핑되지 않아야 한다")
}

func TestTriggerNode_Unified_ArrayScalarWrap(t *testing.T) {
	// AC-NODE-004-16: 배열 스칼라 → {"value": [1,2,3]} 래핑 (비문자열 패스스루)
	msg := fireNodePayload(t, []any{1, 2, 3}, 1)
	v, ok := msg.Payload().Get("value")
	require.True(t, ok)
	arr, ok := v.([]any)
	require.True(t, ok, "배열은 비문자열 패스스루되어야 한다")
	assert.Equal(t, []any{1, 2, 3}, arr)
}

func TestTriggerNode_Unified_NestedMapNoRecurseAndDeepCopy(t *testing.T) {
	// AC-NODE-004-18: 중첩 맵 미재귀 + 메시지 간 깊은 복사 격리
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "50ms"},
		},
		"payload": map[string]any{"data": map[string]any{"value": 1}},
	}
	node, timer := initTriggerWithMock(t, config, newMockTimer())
	defer node.Shutdown(context.Background())

	msg1 := fireInterval(t, node, timer, 1)
	msg2 := fireInterval(t, node, timer, 2)

	// 중첩 맵은 재귀 평가되지 않고 리터럴 패스스루
	d1, ok := msg1.Payload().Get("data")
	require.True(t, ok)
	m1, ok := d1.(map[string]any)
	require.True(t, ok, "중첩 맵은 재귀 평가되지 않고 map 그대로여야 한다")
	assert.Equal(t, 1, m1["value"])

	// 첫 메시지의 중첩 맵을 변경해도 두 번째 메시지에 영향 없어야 한다 (깊은 복사)
	m1["value"] = 999
	d2, ok := msg2.Payload().Get("data")
	require.True(t, ok)
	m2 := d2.(map[string]any)
	assert.Equal(t, 1, m2["value"], "메시지 간 페이로드는 깊은 복사로 독립적이어야 한다")
}

func TestTriggerNode_Unified_WholeValueTypedSubstitution(t *testing.T) {
	// AC-NODE-004-19, AC-NODE-004-48: 통째 변수 네이티브 타입 치환
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "50ms"},
		},
		"payload": map[string]any{"time": "$.trigger_time", "count": "$.tick_count"},
	}
	node, timer := initTriggerWithMock(t, config, newMockTimer())
	defer node.Shutdown(context.Background())

	msg := fireInterval(t, node, timer, 3)
	timeV, ok := msg.Payload().Get("time")
	require.True(t, ok)
	_, isStr := timeV.(string)
	assert.True(t, isStr, "trigger_time 통째 치환은 문자열 ISO 시각이어야 한다")

	countV, ok := msg.Payload().Get("count")
	require.True(t, ok)
	assert.Equal(t, int64(3), countV, "tick_count 통째 치환은 네이티브 숫자 3이어야 한다 (문자열 아님)")
}

func TestTriggerNode_Unified_UnknownVarErrorAndNull(t *testing.T) {
	// AC-NODE-004-20: 미지 변수 → 에러 기록 + 키 null + trigger.error 메타데이터
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "50ms"},
		},
		"payload": map[string]any{"bad": "$.unknown_var"},
	}
	node, timer := initTriggerWithMock(t, config, newMockTimer())
	defer node.Shutdown(context.Background())

	msg := fireInterval(t, node, timer, 1)
	v, ok := msg.Payload().Get("bad")
	require.True(t, ok, "bad 키는 존재해야 한다")
	assert.Nil(t, v, "미지 변수 키의 값은 null 이어야 한다")

	errMeta, ok := msg.Metadata().Get("trigger.error")
	assert.True(t, ok, "trigger.error 메타데이터가 포함되어야 한다")
	assert.NotEmpty(t, errMeta)
}

func TestTriggerNode_Unified_PartialUnknownVar(t *testing.T) {
	// REQ-06-04: 일부 키만 미지 변수 에러 시, 정상 키는 그대로 평가되고 문제 키만 null
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "50ms"},
		},
		"payload": map[string]any{"ok": "$.tick_count", "bad": "$.nope"},
	}
	node, timer := initTriggerWithMock(t, config, newMockTimer())
	defer node.Shutdown(context.Background())

	msg := fireInterval(t, node, timer, 5)
	okV, ok := msg.Payload().Get("ok")
	require.True(t, ok)
	assert.Equal(t, int64(5), okV, "정상 키는 그대로 평가되어야 한다")
	badV, ok := msg.Payload().Get("bad")
	require.True(t, ok)
	assert.Nil(t, badV, "미지 변수 키만 null 이어야 한다")
}

func TestTriggerNode_Unified_DollarEscape(t *testing.T) {
	// AC-NODE-004-46: $$ → 리터럴 $
	msg := fireNodePayload(t, map[string]any{"price": "$$100", "label": "cost is $$"}, 1)
	price, ok := msg.Payload().Get("price")
	require.True(t, ok)
	assert.Equal(t, "$100", price, "$$ 는 리터럴 $ 로 치환되어야 한다")
	label, ok := msg.Payload().Get("label")
	require.True(t, ok)
	assert.Equal(t, "cost is $", label)
}

func TestTriggerNode_Unified_LiteralPlusVarInterpolation(t *testing.T) {
	// AC-NODE-004-47, AC-NODE-004-48: 리터럴 + 변수 혼합 interpolation
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "50ms"},
		},
		"payload": map[string]any{
			"cmd": "open",
			"at":  "$.trigger_time",
			"msg": "count=$.tick_count now",
		},
	}
	node, timer := initTriggerWithMock(t, config, newMockTimer())
	defer node.Shutdown(context.Background())

	msg := fireInterval(t, node, timer, 7)
	cmd, ok := msg.Payload().Get("cmd")
	require.True(t, ok)
	assert.Equal(t, "open", cmd, "리터럴은 그대로여야 한다")

	at, ok := msg.Payload().Get("at")
	require.True(t, ok)
	_, isStr := at.(string)
	assert.True(t, isStr, "통째 변수 치환은 문자열 ISO 시각이어야 한다")

	m, ok := msg.Payload().Get("msg")
	require.True(t, ok)
	assert.Equal(t, "count=7 now", m, "통째 일치 아닌 경우 문자열 형태로 interpolation 되어야 한다")
}

func TestTriggerNode_Unified_MigrationStaticMapSameOutput(t *testing.T) {
	// AC-NODE-004-49: 구 static 맵 동일 출력 (리터럴 통과 + 비문자열 패스스루)
	msg := fireNodePayload(t, map[string]any{"cmd": "open", "level": 3}, 1)
	cmd, ok := msg.Payload().Get("cmd")
	require.True(t, ok)
	assert.Equal(t, "open", cmd)
	level, ok := msg.Payload().Get("level")
	require.True(t, ok)
	assert.Equal(t, 3, level, "비문자열은 패스스루되어야 한다")
}

func TestTriggerNode_Unified_MigrationStaticScalarWrap(t *testing.T) {
	// AC-NODE-004-50: 구 static 스칼라 5 → {"value": 5} 래핑
	msg := fireNodePayload(t, 5, 1)
	v, ok := msg.Payload().Get("value")
	require.True(t, ok)
	assert.Equal(t, 5, v)
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
