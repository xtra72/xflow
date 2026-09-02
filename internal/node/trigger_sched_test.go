package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// SPEC-TRIGGER-SCHED-001 M1 — 스케줄 규칙 메타 확장 + 발화 게이팅 테스트
//
// Module 01 (REQ-SCHED-01-01~07) 및 AC-1/AC-2/AC-3 을 testify 로 검증한다.
// 모든 발화 테스트는 mock timer + 고정 clock(setNowFunc)으로 결정적으로 동작한다.
// ---------------------------------------------------------------------------

// localDate 는 서버 로컬 시간대의 특정 시각을 만든다(유효기간 판정은 서버 로컬 기준).
func localDate(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.Local)
}

// singleIntervalCfg 는 interval "1s" 스케줄 하나에 규칙 메타(meta)를 병합한 config 를 만든다.
func singleIntervalCfg(meta map[string]any) map[string]any {
	sched := map[string]any{"type": "interval", "value": "1s"}
	for k, v := range meta {
		sched[k] = v
	}
	return map[string]any{"schedules": []any{sched}}
}

// initSchedNode 는 mock timer 로 노드를 Init 하고, 고정 clock 을 주입해 반환한다.
func initSchedNode(t *testing.T, cfg map[string]any, now time.Time) (*TriggerNode, *mockTimer) {
	t.Helper()
	node, timer := initTriggerWithMock(t, cfg, newMockTimer())
	tn := node.(*TriggerNode)
	tn.setNowFunc(func() time.Time { return now })
	return tn, timer
}

// ---------------------------------------------------------------------------
// withinValidity 순수 함수 단위 테스트 (RD-8 의미론)
// ---------------------------------------------------------------------------

func TestWithinValidity(t *testing.T) {
	loc := time.Local
	mk := func(y int, mo time.Month, d, h, mi int) time.Time {
		return time.Date(y, mo, d, h, mi, 0, 0, loc)
	}

	cases := []struct {
		name string
		now  time.Time
		from string
		to   string
		want bool
	}{
		{"both empty → unbounded", mk(2026, 7, 21, 12, 0), "", "", true},
		{"before from", mk(2026, 7, 19, 23, 59), "2026-07-20", "2026-07-22", false},
		{"within range", mk(2026, 7, 21, 12, 0), "2026-07-20", "2026-07-22", true},
		{"after to", mk(2026, 7, 23, 0, 0), "2026-07-20", "2026-07-22", false},
		{"from boundary 00:00 inclusive", mk(2026, 7, 20, 0, 0), "2026-07-20", "2026-07-22", true},
		{"to boundary 23:59 inclusive", mk(2026, 7, 22, 23, 59), "2026-07-20", "2026-07-22", true},
		{"day before from 23:59 excluded", mk(2026, 7, 19, 23, 59), "2026-07-20", "", false},
		{"day after to 00:00 excluded", mk(2026, 7, 23, 0, 0), "", "2026-07-22", false},
		{"empty to → unbounded future", mk(2030, 1, 1, 0, 0), "2026-07-20", "", true},
		{"empty from → unbounded past", mk(2000, 1, 1, 0, 0), "", "2026-07-22", true},
		{"invalid from → unbounded past", mk(2000, 1, 1, 0, 0), "not-a-date", "2026-07-22", true},
		{"invalid to → unbounded future", mk(2030, 1, 1, 0, 0), "2026-07-20", "garbage", true},
		{"both invalid → unbounded", mk(2026, 7, 21, 12, 0), "x", "y", true},
		{"same-day from==to inclusive", mk(2026, 7, 21, 15, 0), "2026-07-21", "2026-07-21", true},
		{"RFC3339 datetime bound normalized to date", mk(2026, 7, 22, 23, 59), "2026-07-20T08:00:00Z", "2026-07-22T09:00:00Z", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, withinValidity(c.now, c.from, c.to))
		})
	}
}

// ---------------------------------------------------------------------------
// AC-1 — 비활성 규칙은 발화하지 않는다 (REQ-SCHED-01-03)
// ---------------------------------------------------------------------------

func TestTriggerSched_Disabled_NoEmit_TimerStaysRegistered(t *testing.T) {
	// enabled=false, 유효기간이 현재를 포함하더라도 발화하지 않아야 한다.
	cfg := singleIntervalCfg(map[string]any{
		"enabled":    false,
		"valid_from": "2026-07-01",
		"valid_to":   "2026-12-31",
	})
	tn, timer := initSchedNode(t, cfg, localDate(2026, 7, 21, 12, 0))
	defer tn.Shutdown(context.Background())

	// 타이머는 등록된 채 유지된다(취소하지 않음).
	require.Len(t, tn.timerEntries, 1, "비활성이어도 타이머 엔트리는 등록 유지되어야 한다")
	assert.False(t, tn.timerEntries[0].enabled)

	fireTrigger(timer, "trigger-1-interval-0")
	msgs := drainSourceCh(tn, 80*time.Millisecond)
	assert.Empty(t, msgs, "비활성 규칙 발화는 메시지를 방출하지 않아야 한다")

	// 타이머는 여전히 등록되어 있고 다음 틱에 재검사된다.
	assert.False(t, timer.isCancelled("trigger-1-interval-0"), "비활성은 타이머를 취소하지 않아야 한다")
	require.Len(t, tn.timerEntries, 1)
}

func TestTriggerSched_Disabled_OtherActiveRuleUnaffected(t *testing.T) {
	// [0] 비활성, [1] 활성 → [1] 만 발화한다 (AC-1 And).
	cfg := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s", "enabled": false},
			map[string]any{"type": "interval", "value": "1s", "enabled": true},
		},
	}
	tn, timer := initSchedNode(t, cfg, localDate(2026, 7, 21, 12, 0))
	defer tn.Shutdown(context.Background())

	fireTrigger(timer, "trigger-1-interval-0") // 비활성
	assert.Empty(t, drainSourceCh(tn, 60*time.Millisecond), "비활성 규칙은 발화하지 않아야 한다")

	fireTrigger(timer, "trigger-1-interval-1") // 활성
	assert.Len(t, drainSourceCh(tn, 60*time.Millisecond), 1, "활성 규칙은 정상 발화해야 한다")
}

// ---------------------------------------------------------------------------
// AC-2 — 규칙은 유효기간 내에서만 발화한다 (REQ-SCHED-01-04)
// ---------------------------------------------------------------------------

func TestTriggerSched_ValidityWindow_Emit(t *testing.T) {
	meta := map[string]any{"valid_from": "2026-07-20", "valid_to": "2026-07-22", "enabled": true}

	cases := []struct {
		name     string
		now      time.Time
		wantEmit bool
	}{
		{"before window (07-19)", localDate(2026, 7, 19, 12, 0), false},
		{"within window (07-21)", localDate(2026, 7, 21, 12, 0), true},
		{"after window (07-23)", localDate(2026, 7, 23, 12, 0), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tn, timer := initSchedNode(t, singleIntervalCfg(meta), c.now)
			defer tn.Shutdown(context.Background())
			fireTrigger(timer, "trigger-1-interval-0")
			msgs := drainSourceCh(tn, 80*time.Millisecond)
			if c.wantEmit {
				assert.Len(t, msgs, 1, "유효기간 내에서는 발화해야 한다")
			} else {
				assert.Empty(t, msgs, "유효기간 밖에서는 발화하지 않아야 한다")
			}
		})
	}
}

func TestTriggerSched_UnboundedBounds_Emit(t *testing.T) {
	// valid_to 빈 값(무기한): 먼 미래에도 발화한다(하한만 존재).
	t.Run("empty valid_to → unbounded future", func(t *testing.T) {
		cfg := singleIntervalCfg(map[string]any{"valid_from": "2026-07-20"})
		tn, timer := initSchedNode(t, cfg, localDate(2030, 1, 1, 0, 0))
		defer tn.Shutdown(context.Background())
		fireTrigger(timer, "trigger-1-interval-0")
		assert.Len(t, drainSourceCh(tn, 80*time.Millisecond), 1, "valid_to 빈 값이면 상한 없이 발화해야 한다")
	})

	// valid_from 빈 값(하한 무제한): 상한 이내이면 과거에도 발화한다.
	t.Run("empty valid_from → unbounded past", func(t *testing.T) {
		cfg := singleIntervalCfg(map[string]any{"valid_to": "2026-07-22"})
		tn, timer := initSchedNode(t, cfg, localDate(2000, 1, 1, 0, 0))
		defer tn.Shutdown(context.Background())
		fireTrigger(timer, "trigger-1-interval-0")
		assert.Len(t, drainSourceCh(tn, 80*time.Millisecond), 1, "valid_from 빈 값이면 하한 없이 발화해야 한다")
	})
}

// ---------------------------------------------------------------------------
// AC-3 — 유효기간 경계값 (양끝 inclusive) (REQ-SCHED-01-04)
// ---------------------------------------------------------------------------

func TestTriggerSched_BoundaryInclusive(t *testing.T) {
	meta := map[string]any{"valid_from": "2026-07-20", "valid_to": "2026-07-22"}

	cases := []struct {
		name     string
		now      time.Time
		wantEmit bool
	}{
		{"lower boundary 07-20 00:00 (inclusive)", localDate(2026, 7, 20, 0, 0), true},
		{"upper boundary 07-22 23:59 (inclusive)", localDate(2026, 7, 22, 23, 59), true},
		{"just before 07-19 23:59 (excluded)", localDate(2026, 7, 19, 23, 59), false},
		{"just after 07-23 00:00 (excluded)", localDate(2026, 7, 23, 0, 0), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tn, timer := initSchedNode(t, singleIntervalCfg(meta), c.now)
			defer tn.Shutdown(context.Background())
			fireTrigger(timer, "trigger-1-interval-0")
			msgs := drainSourceCh(tn, 80*time.Millisecond)
			assert.Equal(t, c.wantEmit, len(msgs) == 1)
		})
	}
}

// ---------------------------------------------------------------------------
// REQ-SCHED-01-02 — enabled 기본 true (하위 호환, 무회귀)
// ---------------------------------------------------------------------------

func TestTriggerSched_EnabledDefaultTrue_WhenFieldAbsent(t *testing.T) {
	// 기존 스케줄 형태(확장 필드 부재) → enabled 기본 true, 유효기간 무제한 → 발화.
	cfg := map[string]any{"schedules": []any{map[string]any{"type": "interval", "value": "1s"}}}
	tn, timer := initSchedNode(t, cfg, localDate(2026, 7, 21, 12, 0))
	defer tn.Shutdown(context.Background())

	require.Len(t, tn.timerEntries, 1)
	assert.True(t, tn.timerEntries[0].enabled, "enabled 필드 부재 시 엔트리는 활성(true)으로 해소되어야 한다")
	assert.Nil(t, tn.schedules[0].Enabled, "config 부재 시 TriggerSchedule.Enabled 는 nil(부재) 이어야 한다")

	fireTrigger(timer, "trigger-1-interval-0")
	assert.Len(t, drainSourceCh(tn, 80*time.Millisecond), 1, "기존 스케줄은 무회귀로 발화해야 한다")
}

func TestTriggerSched_EnabledExplicitFalse_ParsedAsPointer(t *testing.T) {
	cfg := singleIntervalCfg(map[string]any{"enabled": false})
	tn, _ := initSchedNode(t, cfg, localDate(2026, 7, 21, 12, 0))
	defer tn.Shutdown(context.Background())

	require.NotNil(t, tn.schedules[0].Enabled, "명시 enabled 는 포인터로 파싱되어 부재와 구분되어야 한다")
	assert.False(t, *tn.schedules[0].Enabled)
	assert.False(t, tn.timerEntries[0].enabled)
}

// ---------------------------------------------------------------------------
// REQ-SCHED-01-01/06 — name/priority 파싱 + 캐리 + pass-through 메타
// ---------------------------------------------------------------------------

func TestTriggerSched_NamePriorityParsedAndCarried(t *testing.T) {
	cfg := singleIntervalCfg(map[string]any{
		"name":       "대합실 야간 소등",
		"priority":   3,
		"valid_from": "",
		"valid_to":   "",
	})
	tn, timer := initSchedNode(t, cfg, localDate(2026, 7, 21, 12, 0))
	defer tn.Shutdown(context.Background())

	// 스케줄/엔트리에 캡처되었는지 확인.
	assert.Equal(t, "대합실 야간 소등", tn.schedules[0].Name)
	assert.Equal(t, 3, tn.schedules[0].Priority)
	assert.Equal(t, "대합실 야간 소등", tn.timerEntries[0].name)
	assert.Equal(t, 3, tn.timerEntries[0].priority)

	// 방출 메시지 메타데이터에 pass-through 되는지 확인.
	fireTrigger(timer, "trigger-1-interval-0")
	msgs := drainSourceCh(tn, 80*time.Millisecond)
	require.Len(t, msgs, 1)
	meta := msgs[0].Metadata()
	name, ok := meta.Get("trigger.rule_name")
	require.True(t, ok, "name 은 방출 메시지 메타에 통과 전달되어야 한다")
	assert.Equal(t, "대합실 야간 소등", name)
	prio, ok := meta.Get("trigger.priority")
	require.True(t, ok, "priority 는 방출 메시지 메타에 통과 전달되어야 한다")
	assert.Equal(t, "3", prio)
}

func TestTriggerSched_PriorityAcceptsNumericForms(t *testing.T) {
	// JSON 디코딩(float64) / Go 리터럴(int) / 문자열 정수 모두 수용.
	assert.Equal(t, 2, anyToInt(float64(2)))
	assert.Equal(t, 5, anyToInt(5))
	assert.Equal(t, 7, anyToInt(int64(7)))
	assert.Equal(t, 4, anyToInt(" 4 "))
	assert.Equal(t, 0, anyToInt("nan"))
	assert.Equal(t, 0, anyToInt(nil))
	assert.Equal(t, 0, anyToInt(true))
}

func TestTriggerSched_NoMetaOnDefaultSchedule(t *testing.T) {
	// 무회귀: name="" priority=0 인 기존 스케줄은 새 메타 키를 추가하지 않는다.
	cfg := map[string]any{"schedules": []any{map[string]any{"type": "interval", "value": "1s"}}}
	tn, timer := initSchedNode(t, cfg, localDate(2026, 7, 21, 12, 0))
	defer tn.Shutdown(context.Background())

	fireTrigger(timer, "trigger-1-interval-0")
	msgs := drainSourceCh(tn, 80*time.Millisecond)
	require.Len(t, msgs, 1)
	_, hasName := msgs[0].Metadata().Get("trigger.rule_name")
	_, hasPrio := msgs[0].Metadata().Get("trigger.priority")
	assert.False(t, hasName, "이름 없는 기존 스케줄은 rule_name 메타를 추가하지 않아야 한다")
	assert.False(t, hasPrio, "priority 0 인 기존 스케줄은 priority 메타를 추가하지 않아야 한다")
}

// ---------------------------------------------------------------------------
// REQ-SCHED-01-07 — live Configure 로 enabled 토글 시 다음 틱에 재게이팅
// ---------------------------------------------------------------------------

func TestTriggerSched_LiveConfigure_ToggleEnabledRegates(t *testing.T) {
	cfg := singleIntervalCfg(map[string]any{"enabled": true})
	tn, timer := initSchedNode(t, cfg, localDate(2026, 7, 21, 12, 0))
	defer tn.Shutdown(context.Background())

	// 초기(활성): 발화한다.
	fireTrigger(timer, "trigger-1-interval-0")
	require.Len(t, drainSourceCh(tn, 80*time.Millisecond), 1, "활성 상태에서는 발화해야 한다")

	// live Configure 로 enabled=false 재무장.
	require.NoError(t, tn.Configure(singleIntervalCfg(map[string]any{"enabled": false})))
	// 재무장 후 새 clock 을 다시 주입한다(Configure 는 nowFunc 를 건드리지 않지만 명시).
	tn.setNowFunc(func() time.Time { return localDate(2026, 7, 21, 12, 0) })

	// 다음 틱: 비활성으로 재게이팅되어 발화하지 않는다.
	fireTrigger(timer, "trigger-1-interval-0")
	assert.Empty(t, drainSourceCh(tn, 80*time.Millisecond), "토글 후에는 비활성으로 재게이팅되어 발화하지 않아야 한다")
	assert.False(t, tn.timerEntries[0].enabled)

	// 다시 활성화하면 발화가 재개된다.
	require.NoError(t, tn.Configure(singleIntervalCfg(map[string]any{"enabled": true})))
	tn.setNowFunc(func() time.Time { return localDate(2026, 7, 21, 12, 0) })
	fireTrigger(timer, "trigger-1-interval-0")
	assert.Len(t, drainSourceCh(tn, 80*time.Millisecond), 1, "재활성화 후에는 발화가 재개되어야 한다")
}

// ---------------------------------------------------------------------------
// 파싱 견고성 — 잘못된/빈 날짜 문자열은 오류가 아니라 무제한 처리 (RD-8)
// ---------------------------------------------------------------------------

func TestTriggerSched_InvalidDates_TreatedAsUnbounded(t *testing.T) {
	cfg := singleIntervalCfg(map[string]any{"valid_from": "not-a-date", "valid_to": "??"})
	tn, timer := initSchedNode(t, cfg, localDate(2026, 7, 21, 12, 0))
	defer tn.Shutdown(context.Background())

	// 잘못된 날짜는 스케줄을 실패시키지 않고 등록되며, 양방향 무제한으로 항상 발화한다.
	require.Len(t, tn.timerEntries, 1, "잘못된 날짜 문자열이어도 스케줄은 정상 등록되어야 한다")
	fireTrigger(timer, "trigger-1-interval-0")
	assert.Len(t, drainSourceCh(tn, 80*time.Millisecond), 1, "잘못된 날짜는 무제한으로 처리되어 발화해야 한다")
}
