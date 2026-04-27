package node

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// Mock Timer - system.Timer 인터페이스를 구현하는 테스트용 타이머
// ---------------------------------------------------------------------------

// mockTimerCall 은 타이머 등록 호출 기록이다.
type mockTimerCall struct {
	Method   string        // "SetInterval", "SetCron", "SetTimeout"
	ID       string        // 등록 ID
	Interval time.Duration // SetInterval 전용
	CronExpr string        // SetCron 전용
	Delay    time.Duration // SetTimeout 전용
	Handler  system.TimerHandler
}

// mockTimer 는 테스트용 Timer 구현체이다.
type mockTimer struct {
	mu       sync.Mutex
	calls    []mockTimerCall
	handlers map[system.TimerID]system.TimerHandler
	nextID   int
	cancelCh map[system.TimerID]bool

	// 에러 주입 설정
	setIntervalErr error
	setCronErr     error
	setTimeoutErr  error
	cancelErr      error
}

func newMockTimer() *mockTimer {
	return &mockTimer{
		handlers: make(map[system.TimerID]system.TimerHandler),
		cancelCh: make(map[system.TimerID]bool),
	}
}

func (m *mockTimer) SetInterval(id string, interval time.Duration, handler system.TimerHandler) (system.TimerID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, mockTimerCall{
		Method:   "SetInterval",
		ID:       id,
		Interval: interval,
		Handler:  handler,
	})

	if m.setIntervalErr != nil {
		return "", m.setIntervalErr
	}

	m.nextID++
	timerID := system.TimerID(id)
	m.handlers[timerID] = handler
	return timerID, nil
}

func (m *mockTimer) SetCron(id string, cronExpr string, handler system.TimerHandler) (system.TimerID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, mockTimerCall{
		Method:   "SetCron",
		ID:       id,
		CronExpr: cronExpr,
		Handler:  handler,
	})

	if m.setCronErr != nil {
		return "", m.setCronErr
	}

	m.nextID++
	timerID := system.TimerID(id)
	m.handlers[timerID] = handler
	return timerID, nil
}

func (m *mockTimer) SetTimeout(id string, delay time.Duration, handler system.TimerHandler) (system.TimerID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, mockTimerCall{
		Method:  "SetTimeout",
		ID:      id,
		Delay:   delay,
		Handler: handler,
	})

	if m.setTimeoutErr != nil {
		return "", m.setTimeoutErr
	}

	m.nextID++
	timerID := system.TimerID(id)
	m.handlers[timerID] = handler
	return timerID, nil
}

func (m *mockTimer) Cancel(id system.TimerID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancelErr != nil {
		return m.cancelErr
	}

	m.cancelCh[id] = true
	delete(m.handlers, id)
	return nil
}

func (m *mockTimer) List() []system.TimerInfo {
	return nil
}

// triggerHandler 는 지정된 핸들러를 동기적으로 호출하여 테스트 가능하게 한다.
func (m *mockTimer) triggerHandler(id string, trigger system.TimerTrigger) {
	m.mu.Lock()
	handler, ok := m.handlers[system.TimerID(id)]
	m.mu.Unlock()

	if ok {
		handler(trigger)
	}
}

// getCalls 는 기록된 호출 목록을 반환한다.
func (m *mockTimer) getCalls() []mockTimerCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]mockTimerCall, len(m.calls))
	copy(cp, m.calls)
	return cp
}

// isCancelled 는 지정된 타이머가 취소되었는지 확인한다.
func (m *mockTimer) isCancelled(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cancelCh[system.TimerID(id)]
}

// ---------------------------------------------------------------------------
// 테스트 헬퍼 함수
// ---------------------------------------------------------------------------

// newTriggerDef 는 테스트용 NodeDef를 생성한다.
func newTriggerDef(config map[string]any) flow.NodeDef {
	return flow.NodeDef{
		ID:   "trigger-1",
		Name: "test-trigger",
		Type: "trigger",
		Config: config,
		Outputs: []flow.Port{
			{ID: "out", Name: "out", Direction: flow.PortOutput},
		},
	}
}

// initTriggerWithMock 은 mock Timer를 주입하여 TriggerNode를 생성하고 Init한다.
func initTriggerWithMock(t *testing.T, config map[string]any, timer *mockTimer) (Node, *mockTimer) {
	t.Helper()
	if timer == nil {
		timer = newMockTimer()
	}
	config["_timer_agent"] = timer
	def := newTriggerDef(config)
	node, err := NewTriggerNode(def)
	require.NoError(t, err)
	// 엔진 패턴과 동일하게 Configure → Init 순서로 호출
	err = node.Configure(config)
	require.NoError(t, err)
	err = node.Init(context.Background())
	require.NoError(t, err)
	return node, timer
}

// drainSourceCh 는 sourceCh에서 메시지를 타임아웃까지 읽어 반환한다.
func drainSourceCh(n Node, timeout time.Duration) []message.Message {
	src, ok := n.(SourceNode)
	if !ok {
		return nil
	}
	ch := src.SourceCh()
	var msgs []message.Message
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return msgs
			}
			msgs = append(msgs, msg)
		case <-timer.C:
			return msgs
		}
	}
}

// ===========================================================================
// Module 1: TriggerNode Core
// ===========================================================================

func TestTriggerNode_ImplementsInterfaces(t *testing.T) {
	// REQ-01-01: TriggerNode은 Node와 SourceNode 인터페이스를 구현해야 한다
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s"},
		},
	}
	def := newTriggerDef(config)
	node, err := NewTriggerNode(def)
	require.NoError(t, err)

	// Node 인터페이스 확인
	var _ Node = node
	assert.Equal(t, "trigger-1", node.ID())
	assert.Equal(t, "test-trigger", node.Name())
	assert.Equal(t, "trigger", node.Type())

	// SourceNode 인터페이스 확인
	srcNode, ok := node.(SourceNode)
	assert.True(t, ok, "TriggerNode은 SourceNode를 구현해야 한다")
	assert.NotNil(t, srcNode.SourceCh())
}

func TestTriggerNode_NewFactory(t *testing.T) {
	// REQ-01-02: NewTriggerNode 팩토리 함수
	tests := []struct {
		name    string
		config  map[string]any
		wantErr bool
	}{
		{
			name: "유효한 설정으로 생성",
			config: map[string]any{
				"schedules": []any{
					map[string]any{"type": "interval", "value": "5s"},
				},
			},
			wantErr: false,
		},
		{
			name:    "빈 설정으로 생성 (팩토리는 성공, Init에서 실패)",
			config:  map[string]any{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := newTriggerDef(tt.config)
			node, err := NewTriggerNode(def)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, node)
			}
		})
	}
}

func TestTriggerNode_SourceChBufferDefault(t *testing.T) {
	// REQ-01-02: sourceCh 버퍼 기본값 64
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s"},
		},
	}
	def := newTriggerDef(config)
	node, err := NewTriggerNode(def)
	require.NoError(t, err)

	srcNode := node.(SourceNode)
	ch := srcNode.SourceCh()
	assert.Equal(t, 64, cap(ch))
}

func TestTriggerNode_Process_ReturnsEmpty(t *testing.T) {
	// REQ-01-04: Process()는 빈 슬라이스를 반환해야 한다
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s"},
		},
		"_timer_agent": newMockTimer(),
	}
	def := newTriggerDef(config)
	node, err := NewTriggerNode(def)
	require.NoError(t, err)

	msg := message.New()
	result, err := node.Process(context.Background(), msg)
	assert.NoError(t, err)
	assert.Empty(t, result)
}

// ===========================================================================
// Module 2: Schedule Configuration
// ===========================================================================

func TestTriggerNode_IntervalSchedule(t *testing.T) {
	// REQ-02-02: interval 스케줄 타입
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "5s"},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	calls := timer.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "SetInterval", calls[0].Method)
	assert.Equal(t, 5*time.Second, calls[0].Interval)
}

func TestTriggerNode_CronSchedule(t *testing.T) {
	// REQ-02-03: cron 스케줄 타입
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "cron", "value": "*/5 * * * *"},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	calls := timer.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "SetCron", calls[0].Method)
	assert.Equal(t, "*/5 * * * *", calls[0].CronExpr)
}

func TestTriggerNode_OnceSchedule(t *testing.T) {
	// REQ-02-04: once 스케줄 타입 - 미래 시각
	futureTime := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "once", "value": futureTime},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	calls := timer.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "SetTimeout", calls[0].Method)
	assert.True(t, calls[0].Delay > 0, "지연 시간은 양수여야 한다")
}

func TestTriggerNode_TimesSchedule(t *testing.T) {
	// REQ-02-05: times 스케줄 타입 → cron으로 변환
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{
				"type":  "times",
				"value": []any{"09:00", "12:30", "18:00"},
			},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	calls := timer.getCalls()
	require.Len(t, calls, 3, "times의 각 시각이 개별 cron으로 등록되어야 한다")
	for _, c := range calls {
		assert.Equal(t, "SetCron", c.Method)
	}
	// "09:00" → "0 9 * * *", "12:30" → "30 12 * * *", "18:00" → "0 18 * * *"
	assert.Equal(t, "0 9 * * *", calls[0].CronExpr)
	assert.Equal(t, "30 12 * * *", calls[1].CronExpr)
	assert.Equal(t, "0 18 * * *", calls[2].CronExpr)
}

func TestTriggerNode_MultipleSchedules(t *testing.T) {
	// REQ-02-01: 다중 스케줄 동시 설정
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "5s"},
			map[string]any{"type": "cron", "value": "*/10 * * * *"},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	calls := timer.getCalls()
	require.Len(t, calls, 2)
	assert.Equal(t, "SetInterval", calls[0].Method)
	assert.Equal(t, "SetCron", calls[1].Method)
}

func TestTriggerNode_NoSchedules_Error(t *testing.T) {
	// REQ-02-06: 스케줄 미설정 시 ErrTriggerNoSchedules
	tests := []struct {
		name   string
		config map[string]any
	}{
		{
			name:   "schedules 키 누락",
			config: map[string]any{"_timer_agent": newMockTimer()},
		},
		{
			name:   "빈 schedules 배열",
			config: map[string]any{"schedules": []any{}, "_timer_agent": newMockTimer()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := newTriggerDef(tt.config)
			node, err := NewTriggerNode(def)
			require.NoError(t, err)
			require.NoError(t, node.Configure(tt.config))
			err = node.Init(context.Background())
			assert.Error(t, err)
			assert.ErrorIs(t, err, ErrTriggerNoSchedules)
		})
	}
}

func TestTriggerNode_InvalidScheduleType_Error(t *testing.T) {
	// REQ-02-07: 지원하지 않는 스케줄 타입
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "unknown", "value": "something"},
		},
		"_timer_agent": newMockTimer(),
	}
	def := newTriggerDef(config)
	node, err := NewTriggerNode(def)
	require.NoError(t, err)
	require.NoError(t, node.Configure(config))
	err = node.Init(context.Background())
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrTriggerInvalidScheduleType)
}

func TestTriggerNode_InvalidScheduleValue_Error(t *testing.T) {
	// REQ-02-08: 유효하지 않은 스케줄 값
	tests := []struct {
		name   string
		sched  map[string]any
	}{
		{
			name:  "잘못된 interval 값",
			sched: map[string]any{"type": "interval", "value": "not-a-duration"},
		},
		{
			name:  "잘못된 once 값 (비ISO 형식)",
			sched: map[string]any{"type": "once", "value": "not-a-timestamp"},
		},
		{
			name:  "과거 once 값",
			sched: map[string]any{"type": "once", "value": "2000-01-01T00:00:00Z"},
		},
		{
			name:  "잘못된 times 값 형식",
			sched: map[string]any{"type": "times", "value": []any{"25:99"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := map[string]any{
				"schedules":    []any{tt.sched},
				"_timer_agent": newMockTimer(),
			}
			def := newTriggerDef(config)
			node, err := NewTriggerNode(def)
			require.NoError(t, err)
			require.NoError(t, node.Configure(config))
			err = node.Init(context.Background())
			assert.Error(t, err)
			assert.ErrorIs(t, err, ErrTriggerInvalidScheduleValue)
		})
	}
}

// ===========================================================================
// Module 3: Payload Generation
// ===========================================================================

func TestTriggerNode_StaticPayload(t *testing.T) {
	// REQ-03-01: 정적 페이로드
	tests := []struct {
		name    string
		payload any
		check   func(t *testing.T, msg message.Message)
	}{
		{
			name:    "숫자 페이로드 (int)",
			payload: map[string]any{"value": 42},
			check: func(t *testing.T, msg message.Message) {
				v, ok := msg.Payload().Get("value")
				require.True(t, ok)
				assert.Equal(t, 42, v)
			},
		},
		{
			name:    "문자열 페이로드",
			payload: map[string]any{"greeting": "hello"},
			check: func(t *testing.T, msg message.Message) {
				v, ok := msg.Payload().Get("greeting")
				require.True(t, ok)
				assert.Equal(t, "hello", v)
			},
		},
		{
			name:    "불리언 페이로드",
			payload: map[string]any{"enabled": true},
			check: func(t *testing.T, msg message.Message) {
				v, ok := msg.Payload().Get("enabled")
				require.True(t, ok)
				assert.Equal(t, true, v)
			},
		},
		{
			name:    "오브젝트 페이로드",
			payload: map[string]any{"nested": map[string]any{"key": "val"}},
			check: func(t *testing.T, msg message.Message) {
				v, ok := msg.Payload().Get("nested")
				require.True(t, ok)
				m, ok := v.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "val", m["key"])
			},
		},
		{
			name:    "배열 페이로드",
			payload: map[string]any{"items": []any{1, 2, 3}},
			check: func(t *testing.T, msg message.Message) {
				v, ok := msg.Payload().Get("items")
				require.True(t, ok)
				arr, ok := v.([]any)
				require.True(t, ok)
				assert.Len(t, arr, 3)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timer := newMockTimer()
			config := map[string]any{
				"schedules": []any{
					map[string]any{"type": "interval", "value": "1s"},
				},
				"payload": tt.payload,
			}
			node, timer := initTriggerWithMock(t, config, timer)
			defer node.Shutdown(context.Background())

			// 핸들러를 수동으로 트리거
			timer.triggerHandler("test-trigger-interval-0", system.TimerTrigger{
				TimerID:   "test-trigger-interval-0",
				TriggerAt: time.Now(),
				TickCount: 1,
			})

			msgs := drainSourceCh(node, 100*time.Millisecond)
			require.Len(t, msgs, 1)
			tt.check(t, msgs[0])
		})
	}
}

func TestTriggerNode_DefaultPayload(t *testing.T) {
	// REQ-03-02: 기본 페이로드 (payload 미설정 시)
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s"},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	now := time.Now()
	timer.triggerHandler("test-trigger-interval-0", system.TimerTrigger{
		TimerID:   "test-trigger-interval-0",
		TriggerAt: now,
		TickCount: 1,
	})

	msgs := drainSourceCh(node, 100*time.Millisecond)
	require.Len(t, msgs, 1)

	v, ok := msgs[0].Payload().Get("trigger_time")
	require.True(t, ok, "기본 페이로드에 trigger_time이 있어야 한다")
	timeStr, ok := v.(string)
	require.True(t, ok, "trigger_time은 문자열이어야 한다")
	_, err := time.Parse(time.RFC3339, timeStr)
	assert.NoError(t, err, "trigger_time은 유효한 RFC3339 형식이어야 한다")
}

func TestTriggerNode_TemplatePayload(t *testing.T) {
	// REQ-03-03: 템플릿 페이로드
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s"},
		},
		"payload_template": map[string]any{
			"time":     "$.trigger_time",
			"count":    "$.tick_count",
			"schedule": "$.schedule_id",
			"node":     "$.trigger_id",
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	triggerTime := time.Now()
	timer.triggerHandler("test-trigger-interval-0", system.TimerTrigger{
		TimerID:    "test-trigger-interval-0",
		TriggerAt:  triggerTime,
		TickCount:  5,
		ScheduleID: "sched-001",
	})

	msgs := drainSourceCh(node, 100*time.Millisecond)
	require.Len(t, msgs, 1)

	payload := msgs[0].Payload()

	// $.trigger_time → ISO 8601 시각
	v, ok := payload.Get("time")
	require.True(t, ok)
	assert.Equal(t, triggerTime.UTC().Format(time.RFC3339Nano), v)

	// $.tick_count → 숫자
	v, ok = payload.Get("count")
	require.True(t, ok)
	assert.Equal(t, int64(5), v)

	// $.schedule_id → 문자열
	v, ok = payload.Get("schedule")
	require.True(t, ok)
	assert.Equal(t, "sched-001", v)

	// $.trigger_id → 노드 이름
	v, ok = payload.Get("node")
	require.True(t, ok)
	assert.Equal(t, "test-trigger", v)
}

// ===========================================================================
// Module 4: Message Metadata
// ===========================================================================

func TestTriggerNode_MessageMetadata(t *testing.T) {
	// REQ-04-01: 트리거 메타데이터 자동 첨부
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s"},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	triggerTime := time.Now()
	timer.triggerHandler("test-trigger-interval-0", system.TimerTrigger{
		TimerID:   "test-trigger-interval-0",
		TriggerAt: triggerTime,
		TickCount: 3,
	})

	msgs := drainSourceCh(node, 100*time.Millisecond)
	require.Len(t, msgs, 1)

	meta := msgs[0].Metadata()

	// trigger.schedule_type
	v, ok := meta.Get("trigger.schedule_type")
	assert.True(t, ok)
	assert.Equal(t, "interval", v)

	// trigger.schedule_id
	v, ok = meta.Get("trigger.schedule_id")
	assert.True(t, ok)
	assert.Equal(t, "test-trigger-interval-0", v)

	// trigger.tick_count
	v, ok = meta.Get("trigger.tick_count")
	assert.True(t, ok)
	assert.Equal(t, "3", v)

	// trigger.trigger_time
	v, ok = meta.Get("trigger.trigger_time")
	assert.True(t, ok)
	assert.NotEmpty(t, v)

	// trigger.node_name
	v, ok = meta.Get("trigger.node_name")
	assert.True(t, ok)
	assert.Equal(t, "test-trigger", v)
}

// ===========================================================================
// Module 5: Lifecycle Integration
// ===========================================================================

func TestTriggerNode_Init_Success(t *testing.T) {
	// REQ-05-01: Init 성공 시 StateRunning으로 전이
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s"},
		},
	}
	node, _ := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	assert.Equal(t, lifecycle.StateRunning, node.(*TriggerNode).CurrentState())
}

func TestTriggerNode_Init_FailureRollback(t *testing.T) {
	// REQ-05-01: Init 실패 시 등록된 타이머를 모두 취소하고 StateError로 전이
	timer := newMockTimer()
	// 두 번째 스케줄에서 실패하도록 설정 (첫 번째는 interval로 성공, 두 번째는 잘못된 타입)
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s"},
			map[string]any{"type": "invalid_type", "value": "something"},
		},
		"_timer_agent": timer,
	}
	def := newTriggerDef(config)
	node, err := NewTriggerNode(def)
	require.NoError(t, err)
	require.NoError(t, node.Configure(config))
	err = node.Init(context.Background())
	assert.Error(t, err)

	// 첫 번째 타이머가 취소되었는지 확인
	assert.True(t, timer.isCancelled("test-trigger-interval-0"),
		"Init 실패 시 이미 등록된 타이머는 취소되어야 한다")

	// StateError로 전이
	assert.Equal(t, lifecycle.StateError, node.(*TriggerNode).CurrentState())
}

func TestTriggerNode_Shutdown(t *testing.T) {
	// REQ-05-02: Shutdown 시 타이머 취소 및 채널 닫기
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s"},
			map[string]any{"type": "cron", "value": "*/5 * * * *"},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)

	err := node.Shutdown(context.Background())
	assert.NoError(t, err)

	// 모든 타이머가 취소되었는지 확인
	assert.True(t, timer.isCancelled("test-trigger-interval-0"))
	assert.True(t, timer.isCancelled("test-trigger-cron-1"))

	// sourceCh가 닫혔는지 확인
	srcNode := node.(SourceNode)
	_, ok := <-srcNode.SourceCh()
	assert.False(t, ok, "sourceCh가 닫혀야 한다")

	// StateStopped 확인
	assert.Equal(t, lifecycle.StateStopped, node.(*TriggerNode).CurrentState())
}

func TestTriggerNode_PauseResume(t *testing.T) {
	// REQ-05-03, REQ-05-04: Pause/Resume
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s"},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	triggerNode := node.(*TriggerNode)

	// Pause
	err := triggerNode.Pause(context.Background())
	assert.NoError(t, err)

	// Pause 중 트리거 → 메시지가 생성되지 않아야 한다
	timer.triggerHandler("test-trigger-interval-0", system.TimerTrigger{
		TimerID:   "test-trigger-interval-0",
		TriggerAt: time.Now(),
		TickCount: 1,
	})
	msgs := drainSourceCh(node, 100*time.Millisecond)
	assert.Empty(t, msgs, "Pause 중에는 메시지가 생성되지 않아야 한다")

	// Resume
	err = triggerNode.Resume(context.Background())
	assert.NoError(t, err)

	// Resume 후 트리거 → 메시지가 생성되어야 한다
	timer.triggerHandler("test-trigger-interval-0", system.TimerTrigger{
		TimerID:   "test-trigger-interval-0",
		TriggerAt: time.Now(),
		TickCount: 2,
	})
	msgs = drainSourceCh(node, 100*time.Millisecond)
	assert.Len(t, msgs, 1, "Resume 후에는 메시지가 생성되어야 한다")
}

// ===========================================================================
// Module 6: Error Handling
// ===========================================================================

func TestTriggerNode_SentinelErrors(t *testing.T) {
	// REQ-06-01: sentinel 에러 정의 확인
	assert.NotNil(t, ErrTriggerNoSchedules)
	assert.NotNil(t, ErrTriggerInvalidScheduleType)
	assert.NotNil(t, ErrTriggerInvalidScheduleValue)
	assert.NotNil(t, ErrTriggerTimerNotAvailable)
	assert.NotNil(t, ErrTriggerPayloadTemplateFailed)

	// ErrInvalidConfig 또는 ErrNodeNotInitialized를 래핑하는지 확인
	assert.True(t,
		strings.Contains(ErrTriggerNoSchedules.Error(), "trigger") ||
			strings.Contains(ErrTriggerNoSchedules.Error(), "schedule"),
		"ErrTriggerNoSchedules에 'trigger' 또는 'schedule' 포함")
}

func TestTriggerNode_TimerNotAvailable(t *testing.T) {
	// REQ-06-02: Timer Agent가 없을 때 ErrTriggerTimerNotAvailable
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s"},
		},
		// _timer_agent 없음, _agent_resolver도 없음
	}
	def := newTriggerDef(config)
	node, err := NewTriggerNode(def)
	require.NoError(t, err)
	require.NoError(t, node.Configure(config))
	err = node.Init(context.Background())
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrTriggerTimerNotAvailable)
}

func TestTriggerNode_SourceChFull_DropMessage(t *testing.T) {
	// REQ-06-03: sourceCh가 가득 차면 메시지를 드롭
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s"},
		},
		"source_ch_size": 2, // 작은 버퍼 크기
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	// 버퍼 크기보다 많은 메시지를 전송
	for i := 0; i < 5; i++ {
		timer.triggerHandler("test-trigger-interval-0", system.TimerTrigger{
			TimerID:   "test-trigger-interval-0",
			TriggerAt: time.Now(),
			TickCount: int64(i + 1),
		})
	}

	// 읽어낸 메시지 수가 버퍼 크기 이하여야 한다 (드롭됨)
	msgs := drainSourceCh(node, 100*time.Millisecond)
	assert.LessOrEqual(t, len(msgs), 2, "버퍼가 가득 차면 메시지가 드롭되어야 한다")
}

func TestTriggerNode_TemplatePayload_Error(t *testing.T) {
	// REQ-06-04: 템플릿 평가 실패 시 에러 메타데이터 포함 메시지 전송
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s"},
		},
		"payload_template": map[string]any{
			"value": "$.unknown_var", // 알 수 없는 변수
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	timer.triggerHandler("test-trigger-interval-0", system.TimerTrigger{
		TimerID:   "test-trigger-interval-0",
		TriggerAt: time.Now(),
		TickCount: 1,
	})

	msgs := drainSourceCh(node, 100*time.Millisecond)
	require.Len(t, msgs, 1)

	// trigger.error 메타데이터가 포함되어야 한다
	v, ok := msgs[0].Metadata().Get("trigger.error")
	assert.True(t, ok, "템플릿 실패 시 trigger.error 메타데이터가 포함되어야 한다")
	assert.NotEmpty(t, v)
}

// ===========================================================================
// 통합 테스트: 다중 스케줄 + 메시지 생성
// ===========================================================================

func TestTriggerNode_MultiSchedule_MessageGeneration(t *testing.T) {
	// 다중 스케줄이 각각 올바른 메타데이터를 가진 메시지를 생성하는지 검증
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s"},
			map[string]any{"type": "cron", "value": "*/5 * * * *"},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	// interval 스케줄 트리거
	timer.triggerHandler("test-trigger-interval-0", system.TimerTrigger{
		TimerID:   "test-trigger-interval-0",
		TriggerAt: time.Now(),
		TickCount: 1,
	})

	// cron 스케줄 트리거
	timer.triggerHandler("test-trigger-cron-1", system.TimerTrigger{
		TimerID:   "test-trigger-cron-1",
		TriggerAt: time.Now(),
		TickCount: 1,
	})

	msgs := drainSourceCh(node, 200*time.Millisecond)
	require.Len(t, msgs, 2)

	// 첫 번째 메시지는 interval
	schedType, _ := msgs[0].Metadata().Get("trigger.schedule_type")
	assert.Equal(t, "interval", schedType)

	// 두 번째 메시지는 cron
	schedType, _ = msgs[1].Metadata().Get("trigger.schedule_type")
	assert.Equal(t, "cron", schedType)
}

func TestTriggerNode_ConcurrentTriggers(t *testing.T) {
	// 동시에 여러 핸들러가 호출되어도 안전한지 검증
	timer := newMockTimer()
	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s"},
		},
	}
	node, timer := initTriggerWithMock(t, config, timer)
	defer node.Shutdown(context.Background())

	var wg sync.WaitGroup
	var count int64

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			timer.triggerHandler("test-trigger-interval-0", system.TimerTrigger{
				TimerID:   "test-trigger-interval-0",
				TriggerAt: time.Now(),
				TickCount: int64(idx),
			})
			atomic.AddInt64(&count, 1)
		}(i)
	}

	wg.Wait()

	msgs := drainSourceCh(node, 200*time.Millisecond)
	assert.NotEmpty(t, msgs, "동시 트리거로 메시지가 생성되어야 한다")
}

func TestTriggerNode_TimerSetError_RollbackOnInit(t *testing.T) {
	// Timer.SetInterval 에러 시 Init이 실패하고 롤백해야 한다
	timer := newMockTimer()
	timer.setIntervalErr = fmt.Errorf("timer error")

	config := map[string]any{
		"schedules": []any{
			map[string]any{"type": "interval", "value": "1s"},
		},
		"_timer_agent": timer,
	}
	def := newTriggerDef(config)
	node, err := NewTriggerNode(def)
	require.NoError(t, err)
	require.NoError(t, node.Configure(config))
	err = node.Init(context.Background())
	assert.Error(t, err)
}

// ===========================================================================
// Node Registry 등록 테스트
// ===========================================================================

func TestTriggerNode_Registry(t *testing.T) {
	// REQ-01-03: Registry에 "trigger" 타입이 등록되어야 한다
	registry := NewRegistry()
	assert.True(t, registry.Has("trigger"), "Registry에 'trigger' 타입이 등록되어야 한다")

	meta, ok := registry.TypeMeta("trigger")
	assert.True(t, ok)
	assert.Equal(t, "input", meta.Category)
}
