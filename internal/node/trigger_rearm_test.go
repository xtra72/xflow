package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// SPEC-TRIGGER-PANEL-001 M1 — TriggerNode.Configure live 재무장 테스트
//
// §A 시나리오(acceptance.md)를 Go testify 로 검증한다. 모든 테스트는 mock timer
// 를 주입하여 결정적(deterministic)으로 발화·취소·세대 동작을 관찰한다.
// ---------------------------------------------------------------------------

// intervalSchedulesConfig 는 interval 스케줄 셋만 담은 재설정용 config 를 만든다.
// 런타임 ReconfigureNode 경로를 미러하여 _timer_agent 를 포함하지 않는다
// (n.timer 는 Init 에서 resolve 된 값이 유지된다).
func intervalSchedulesConfig(values ...string) map[string]any {
	scheds := make([]any, 0, len(values))
	for _, v := range values {
		scheds = append(scheds, map[string]any{"type": "interval", "value": v})
	}
	return map[string]any{"schedules": scheds}
}

// fireTrigger 는 지정된 timerID 의 핸들러를 동기 호출한다(발화 시뮬레이션).
func fireTrigger(timer *mockTimer, timerID string) {
	timer.triggerHandler(timerID, system.TimerTrigger{
		TimerID:   system.TimerID(timerID),
		TriggerAt: time.Now(),
		TickCount: 1,
	})
}

// A-1 — running 노드 live 재등록 (schedule 교체) · REQ-01-01
func TestTriggerRearm_LiveReconfigure_SwapSchedule(t *testing.T) {
	timer := newMockTimer()
	node, timer := initTriggerWithMock(t, intervalSchedulesConfig("1s"), timer)
	defer node.Shutdown(context.Background())
	tn := node.(*TriggerNode)

	// Init 직후: A(interval 1s) 하나만 등록되어 있어야 한다.
	require.Len(t, tn.timerEntries, 1)
	require.Equal(t, 1*time.Second, timer.getCalls()[0].Interval)

	// live Configure 로 A → B(interval 5s) 교체.
	require.NoError(t, node.Configure(intervalSchedulesConfig("5s")))

	// A 타이머는 취소되고 B 가 새로 등록되며, timerEntries 는 정확히 B 하나만 포함.
	assert.True(t, timer.isCancelled("trigger-1-interval-0"), "재무장 시 기존 A 타이머는 취소되어야 한다")
	require.Len(t, tn.timerEntries, 1, "재무장 후 timerEntries 는 B 하나만 포함해야 한다")

	calls := timer.getCalls()
	require.Len(t, calls, 2, "Init(A) + 재무장(B) 으로 SetInterval 호출은 2회여야 한다")
	assert.Equal(t, "SetInterval", calls[1].Method)
	assert.Equal(t, 5*time.Second, calls[1].Interval, "재등록된 타이머는 B(5s) 여야 한다")

	// 재무장 세대가 1 증가했고, 남은 엔트리는 새 세대(1)를 캡처했다.
	assert.Equal(t, uint64(1), tn.rearmGen.Load())
	assert.Equal(t, uint64(1), tn.timerEntries[0].gen)
}

// A-2 — no-double-fire / no-orphan (generation 토큰) · REQ-01-02, RD-6
func TestTriggerRearm_GenerationToken_DropsStaleFire(t *testing.T) {
	timer := newMockTimer()
	// A 는 per-schedule payload {source:"A"}, 재설정 후 B 는 {source:"B"} 를 갖는다.
	initCfg := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s", "payload": map[string]any{"source": "A"}},
		},
	}
	node, timer := initTriggerWithMock(t, initCfg, timer)
	defer node.Shutdown(context.Background())
	tn := node.(*TriggerNode)

	// 세대 G0 = 0 확인, A 의 in-flight 핸들러를 캡처(취소 전에 이미 디스패치된 발화 시뮬레이션).
	require.Equal(t, uint64(0), tn.rearmGen.Load())
	staleHandlerA := timer.getCalls()[0].Handler
	require.NotNil(t, staleHandlerA)

	// live 재무장 A → B: 세대가 G1 = 1 로 증가한다.
	reCfg := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "5s", "payload": map[string]any{"source": "B"}},
		},
	}
	require.NoError(t, node.Configure(reCfg))
	require.Equal(t, uint64(1), tn.rearmGen.Load())

	// cancel→re-register 창에서 도착한 A 의 stale in-flight 발화(캡처 세대 G0):
	// 세대 불일치(0 != 1)로 drop 되어야 한다 — 중복 발화 없음.
	staleHandlerA(system.TimerTrigger{
		TimerID:   "trigger-1-interval-0",
		TriggerAt: time.Now(),
		TickCount: 99,
	})
	staleMsgs := drainSourceCh(node, 80*time.Millisecond)
	assert.Empty(t, staleMsgs, "stale G0 발화는 세대 불일치로 폐기되어야 한다(orphan/double-fire 없음)")

	// B(세대 G1)는 각 주기마다 정확히 1회 발화한다.
	fireTrigger(timer, "trigger-1-interval-0")
	bMsgs := drainSourceCh(node, 80*time.Millisecond)
	require.Len(t, bMsgs, 1, "B 는 유효 세대이므로 발화 시 정확히 1회 메시지를 생성해야 한다")
	src, ok := bMsgs[0].Payload().Get("source")
	require.True(t, ok)
	assert.Equal(t, "B", src, "발화된 메시지는 재무장 이후의 B 페이로드여야 한다")
}

// A-3 — 비-Running 상태(Created/Paused/Stopped)는 타이머 재등록 안 함 · REQ-01-03
func TestTriggerRearm_NonRunning_NoReregister(t *testing.T) {
	t.Run("Created", func(t *testing.T) {
		timer := newMockTimer()
		cfg := intervalSchedulesConfig("1s")
		cfg["_timer_agent"] = timer
		def := newTriggerDef(cfg)
		node, err := NewTriggerNode(def)
		require.NoError(t, err)
		tn := node.(*TriggerNode)

		// Init 이전(Created) Configure 는 파싱만 하고 등록/취소하지 않는다.
		require.NoError(t, node.Configure(intervalSchedulesConfig("5s")))
		assert.Empty(t, timer.getCalls(), "Created 상태 Configure 는 타이머를 등록하지 않아야 한다")
		require.Len(t, tn.schedules, 1)
		assert.Equal(t, "5s", tn.schedules[0].Value, "n.schedules 는 갱신되어야 한다")
		assert.Equal(t, uint64(0), tn.rearmGen.Load(), "재무장 세대는 증가하지 않아야 한다")
	})

	t.Run("Paused", func(t *testing.T) {
		timer := newMockTimer()
		node, timer := initTriggerWithMock(t, intervalSchedulesConfig("1s"), timer)
		defer node.Shutdown(context.Background())
		tn := node.(*TriggerNode)
		require.NoError(t, tn.Pause(context.Background()))

		require.NoError(t, node.Configure(intervalSchedulesConfig("5s")))
		assert.False(t, timer.isCancelled("trigger-1-interval-0"), "Paused 상태에서는 기존 타이머가 취소되지 않아야 한다")
		assert.Len(t, timer.getCalls(), 1, "Paused 상태 Configure 는 재등록하지 않아야 한다(Init 의 1회만)")
		require.Len(t, tn.schedules, 1)
		assert.Equal(t, "5s", tn.schedules[0].Value, "n.schedules 는 갱신되어야 한다")
	})

	t.Run("Stopped", func(t *testing.T) {
		timer := newMockTimer()
		node, timer := initTriggerWithMock(t, intervalSchedulesConfig("1s"), timer)
		require.NoError(t, node.Shutdown(context.Background()))
		tn := node.(*TriggerNode)

		require.NoError(t, node.Configure(intervalSchedulesConfig("5s")))
		assert.Len(t, timer.getCalls(), 1, "Stopped 상태 Configure 는 재등록하지 않아야 한다(Init 의 1회만)")
		require.Len(t, tn.schedules, 1)
		assert.Equal(t, "5s", tn.schedules[0].Value, "n.schedules 는 갱신되어야 한다")
	})
}

// A-4 — Init 경로 불변 (최초 등록 동작·엔트리 수가 기존과 동일) · REQ-01-04
//
// 이 코드베이스의 배포/부팅 경로는 항상 Configure(파싱 전용)→Init(등록) 순서다
// (NewBaseNode 는 def.Config 를 n.config 로 옮기지 않으므로 Init 단독 경로는 없다).
// 최초 Configure 는 Created 상태여서 재무장하지 않고(게이트 false), Init 이 등록을
// 담당한다 — 재무장 로직 추가가 Init 의 최초 1회 등록 동작을 바꾸지 않음을 검증한다.
func TestTriggerRearm_InitPath_Unchanged(t *testing.T) {
	timer := newMockTimer()
	cfg := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s"},
			map[string]any{"type": "cron", "value": "*/5 * * * *"},
		},
	}
	node, timer := initTriggerWithMock(t, cfg, timer) // Configure → Init (엔진 배포 패턴)
	defer node.Shutdown(context.Background())
	tn := node.(*TriggerNode)

	// 최초 등록 동작·엔트리 수가 기존과 동일해야 한다(스케줄 2개 → 등록 2회).
	require.Len(t, tn.timerEntries, 2, "Init 은 스케줄 수만큼 최초 1회 등록해야 한다")
	assert.Len(t, timer.getCalls(), 2, "재무장 로직 추가에도 Init 등록은 정확히 2회여야 한다(이중 등록 없음)")
	assert.Equal(t, uint64(0), tn.rearmGen.Load(), "Init 경로는 재무장 세대를 증가시키지 않는다")
	for _, e := range tn.timerEntries {
		assert.Equal(t, uint64(0), e.gen, "Init 등록 엔트리는 세대 0 을 캡처해야 한다")
	}
	assert.True(t, tn.started.Load(), "Init 성공 후 started 는 true 여야 한다")
	assert.Equal(t, lifecycle.StateRunning, tn.CurrentState())
}

// 초기 Configure+Init 시퀀스가 이중 등록하지 않음을 명시적으로 검증 · REQ-01-04
func TestTriggerRearm_ConfigureThenInit_NoDoubleRegister(t *testing.T) {
	timer := newMockTimer()
	// initTriggerWithMock 은 Configure(config) → Init(ctx) 순으로 호출한다(엔진 패턴).
	node, timer := initTriggerWithMock(t, intervalSchedulesConfig("1s"), timer)
	defer node.Shutdown(context.Background())
	tn := node.(*TriggerNode)

	assert.Len(t, timer.getCalls(), 1, "Configure(파싱만)+Init(등록) 시퀀스는 정확히 1회만 등록해야 한다")
	require.Len(t, tn.timerEntries, 1)

	// 발화도 정확히 1회만 메시지를 생성한다(중복 등록 시 여러 개가 생성될 것).
	fireTrigger(timer, "trigger-1-interval-0")
	msgs := drainSourceCh(node, 80*time.Millisecond)
	assert.Len(t, msgs, 1)
}

// A-5 — 빈 스케줄 live Configure (IDLE 확정) · REQ-01-05, RD-7
func TestTriggerRearm_EmptySchedules_ValidIdle(t *testing.T) {
	timer := newMockTimer()
	node, timer := initTriggerWithMock(t, intervalSchedulesConfig("1s"), timer)
	defer node.Shutdown(context.Background())
	tn := node.(*TriggerNode)

	// 빈 스케줄 live Configure: 오류가 아니며 모든 타이머를 취소한다.
	err := node.Configure(map[string]any{"schedules": []any{}})
	require.NoError(t, err, "빈 스케줄 live Configure 는 오류가 아니어야 한다")

	assert.True(t, timer.isCancelled("trigger-1-interval-0"), "빈 스케줄 재무장은 모든 타이머를 취소해야 한다")
	assert.Empty(t, tn.timerEntries, "재무장 후 timerEntries 는 비어야 한다")
	assert.Equal(t, lifecycle.StateRunning, tn.CurrentState(), "노드는 발화 없는 유효 IDLE 상태로 Running 을 유지해야 한다")
	assert.Equal(t, uint64(1), tn.rearmGen.Load(), "재무장 세대는 증가한다(이전 타이머의 stale 발화 무효화)")
}

// A-6 — 재등록 실패 롤백 · REQ-01-06
func TestTriggerRearm_RegisterFailure_Rollback(t *testing.T) {
	timer := newMockTimer()
	node, timer := initTriggerWithMock(t, intervalSchedulesConfig("1s"), timer)
	defer node.Shutdown(context.Background())
	tn := node.(*TriggerNode)

	// 첫 스케줄은 유효, 둘째 스케줄은 interval value 가 non-string 이라 registerSchedules 실패.
	badCfg := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "2s"},
			map[string]any{"type": "interval", "value": 123}, // non-string → 실패
		},
	}
	err := node.Configure(badCfg)
	require.Error(t, err, "잘못된 스케줄로 재등록이 실패해야 한다")
	assert.ErrorIs(t, err, ErrTriggerInvalidScheduleValue)

	// 부분 등록 타이머가 롤백되어 timerEntries 는 비고, lifecycle 은 Error 로 전이하지 않는다.
	assert.Empty(t, tn.timerEntries, "실패 시 부분 등록 타이머가 cancelAllTimers 로 롤백되어야 한다")
	assert.True(t, timer.isCancelled("trigger-1-interval-0"), "롤백은 부분 등록된 새 타이머를 취소해야 한다")
	assert.Equal(t, lifecycle.StateRunning, tn.CurrentState(), "재등록 실패는 lifecycle 을 Error 로 전이시키지 않아야 한다")
}

// A-7 — payload 폴백 보존 (재등록 후에도 buildMessage 순서 동일) · REQ-01-07
func TestTriggerRearm_PayloadFallback_Preserved(t *testing.T) {
	timer := newMockTimer()
	node, timer := initTriggerWithMock(t, intervalSchedulesConfig("1s"), timer)
	defer node.Shutdown(context.Background())

	// 재무장: S1 은 per-schedule payload, S2 는 노드 레벨 payload 로 폴백.
	reCfg := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "2s", "payload": map[string]any{"p": "s1"}},
			map[string]any{"type": "interval", "value": "3s"}, // per-schedule payload 없음 → 노드 레벨 폴백
		},
		"payload": map[string]any{"p": "node"},
	}
	require.NoError(t, node.Configure(reCfg))

	// S1 발화 → per-schedule payload 우선.
	fireTrigger(timer, "trigger-1-interval-0")
	s1 := drainSourceCh(node, 80*time.Millisecond)
	require.Len(t, s1, 1)
	v, ok := s1[0].Payload().Get("p")
	require.True(t, ok)
	assert.Equal(t, "s1", v, "per-schedule payload 가 우선되어야 한다")

	// S2 발화 → per-schedule payload 없으므로 노드 레벨 payload 로 폴백.
	fireTrigger(timer, "trigger-1-interval-1")
	s2 := drainSourceCh(node, 80*time.Millisecond)
	require.Len(t, s2, 1)
	v, ok = s2[0].Payload().Get("p")
	require.True(t, ok)
	assert.Equal(t, "node", v, "per-schedule payload 없으면 노드 레벨 payload 로 폴백해야 한다")
}

// 재무장 vs 발화 동시성 스트레스 (go test -race 대상) · RD-6 동시성 규율
func TestTriggerRearm_ConcurrentRearmAndFire(t *testing.T) {
	timer := newMockTimer()
	node, timer := initTriggerWithMock(t, intervalSchedulesConfig("1s"), timer)
	defer node.Shutdown(context.Background())

	done := make(chan struct{})

	// 발화 고루틴: rearmGen 을 반복적으로 읽는 핸들러를 계속 호출한다.
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				fireTrigger(timer, "trigger-1-interval-0")
			}
		}
	}()

	// 재무장 고루틴(메인): Configure 로 rearmGen 을 반복 증가시킨다.
	for i := 0; i < 50; i++ {
		require.NoError(t, node.Configure(intervalSchedulesConfig("1s")))
	}
	close(done)

	// 채널을 비워 발화 고루틴이 블록되지 않게 한다.
	_ = drainSourceCh(node, 50*time.Millisecond)
	assert.GreaterOrEqual(t, node.(*TriggerNode).rearmGen.Load(), uint64(50))
}
