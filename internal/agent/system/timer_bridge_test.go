package system

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 테스트 헬퍼
// ---------------------------------------------------------------------------

// newTestTimerBridgeHandler 는 초기화된 TimerAgent와 TimerBridgeHandler를 생성한다.
func newTestTimerBridgeHandler(t *testing.T) *TimerBridgeHandler {
	t.Helper()
	agent := NewTimerAgent()
	err := agent.Init(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = agent.Stop(context.Background())
	})
	return NewTimerBridgeHandler(agent)
}

// makeTimerMsg 는 타이머 연산을 위한 메시지를 생성한다.
func makeTimerMsg(operation string, opts ...func(message.Message)) message.Message {
	msg := message.New()
	msg.Metadata().Set("timer.operation", operation)
	for _, opt := range opts {
		opt(msg)
	}
	return msg
}

// withTimerID 는 메시지 메타데이터에 timer.id를 설정하는 옵션이다.
func withTimerID(id string) func(message.Message) {
	return func(msg message.Message) {
		msg.Metadata().Set("timer.id", id)
	}
}

// withTimerInterval 은 메시지 메타데이터에 timer.interval을 설정하는 옵션이다.
func withTimerInterval(interval string) func(message.Message) {
	return func(msg message.Message) {
		msg.Metadata().Set("timer.interval", interval)
	}
}

// withTimerCron 은 메시지 메타데이터에 timer.cron을 설정하는 옵션이다.
func withTimerCron(cronExpr string) func(message.Message) {
	return func(msg message.Message) {
		msg.Metadata().Set("timer.cron", cronExpr)
	}
}

// withTimerDelay 는 메시지 메타데이터에 timer.delay를 설정하는 옵션이다.
func withTimerDelay(delay string) func(message.Message) {
	return func(msg message.Message) {
		msg.Metadata().Set("timer.delay", delay)
	}
}

// ---------------------------------------------------------------------------
// set_interval 연산 테스트
// ---------------------------------------------------------------------------

// TestTimerBridgeHandler_SetInterval 은 set_interval 연산이 타이머를 등록하는지 검증한다.
func TestTimerBridgeHandler_SetInterval(t *testing.T) {
	ctx := context.Background()
	handler := newTestTimerBridgeHandler(t)

	msg := makeTimerMsg("set_interval",
		withTimerID("bridge-interval"),
		withTimerInterval("200ms"),
	)

	resp, err := handler.HandleMessage(ctx, msg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("timer.status")
	require.True(t, ok)
	assert.Equal(t, "ok", status)

	// 등록된 타이머 확인
	listMsg := makeTimerMsg("list")
	listResp, err := handler.HandleMessage(ctx, listMsg)
	require.NoError(t, err)

	timersVal, ok := listResp.Payload().Get("timers")
	require.True(t, ok)

	timers, ok := timersVal.([]TimerInfo)
	require.True(t, ok)
	assert.Len(t, timers, 1)
	assert.Equal(t, TimerID("bridge-interval"), timers[0].ID)
}

// ---------------------------------------------------------------------------
// set_cron 연산 테스트
// ---------------------------------------------------------------------------

// TestTimerBridgeHandler_SetCron 은 set_cron 연산이 타이머를 등록하는지 검증한다.
func TestTimerBridgeHandler_SetCron(t *testing.T) {
	ctx := context.Background()
	handler := newTestTimerBridgeHandler(t)

	msg := makeTimerMsg("set_cron",
		withTimerID("bridge-cron"),
		withTimerCron("0 0 * * *"),
	)

	resp, err := handler.HandleMessage(ctx, msg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("timer.status")
	require.True(t, ok)
	assert.Equal(t, "ok", status)
}

// ---------------------------------------------------------------------------
// set_timeout 연산 테스트
// ---------------------------------------------------------------------------

// TestTimerBridgeHandler_SetTimeout 은 set_timeout 연산이 타이머를 등록하는지 검증한다.
func TestTimerBridgeHandler_SetTimeout(t *testing.T) {
	ctx := context.Background()
	handler := newTestTimerBridgeHandler(t)

	msg := makeTimerMsg("set_timeout",
		withTimerID("bridge-timeout"),
		withTimerDelay("5s"),
	)

	resp, err := handler.HandleMessage(ctx, msg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("timer.status")
	require.True(t, ok)
	assert.Equal(t, "ok", status)
}

// ---------------------------------------------------------------------------
// list 연산 테스트
// ---------------------------------------------------------------------------

// TestTimerBridgeHandler_List 은 list 연산이 타이머 정보를 반환하는지 검증한다.
func TestTimerBridgeHandler_List(t *testing.T) {
	ctx := context.Background()
	handler := newTestTimerBridgeHandler(t)

	// 타이머 등록
	_, err := handler.HandleMessage(ctx, makeTimerMsg("set_interval",
		withTimerID("list-1"),
		withTimerInterval("200ms"),
	))
	require.NoError(t, err)

	_, err = handler.HandleMessage(ctx, makeTimerMsg("set_cron",
		withTimerID("list-2"),
		withTimerCron("* * * * *"),
	))
	require.NoError(t, err)

	// list 조회
	listResp, err := handler.HandleMessage(ctx, makeTimerMsg("list"))
	require.NoError(t, err)

	status, ok := listResp.Metadata().Get("timer.status")
	require.True(t, ok)
	assert.Equal(t, "ok", status)

	timersVal, ok := listResp.Payload().Get("timers")
	require.True(t, ok)

	timers, ok := timersVal.([]TimerInfo)
	require.True(t, ok)
	assert.Len(t, timers, 2)
}

// TestTimerBridgeHandler_ListEmpty 는 빈 목록이 반환되는지 검증한다.
func TestTimerBridgeHandler_ListEmpty(t *testing.T) {
	ctx := context.Background()
	handler := newTestTimerBridgeHandler(t)

	listResp, err := handler.HandleMessage(ctx, makeTimerMsg("list"))
	require.NoError(t, err)

	status, ok := listResp.Metadata().Get("timer.status")
	require.True(t, ok)
	assert.Equal(t, "ok", status)

	timersVal, ok := listResp.Payload().Get("timers")
	require.True(t, ok)

	timers, ok := timersVal.([]TimerInfo)
	require.True(t, ok)
	assert.Empty(t, timers)
}

// ---------------------------------------------------------------------------
// cancel 연산 테스트
// ---------------------------------------------------------------------------

// TestTimerBridgeHandler_Cancel 은 cancel 연산이 타이머를 취소하는지 검증한다.
func TestTimerBridgeHandler_Cancel(t *testing.T) {
	ctx := context.Background()
	handler := newTestTimerBridgeHandler(t)

	// 타이머 등록
	_, err := handler.HandleMessage(ctx, makeTimerMsg("set_interval",
		withTimerID("cancel-bridge"),
		withTimerInterval("200ms"),
	))
	require.NoError(t, err)

	// cancel 연산
	cancelResp, err := handler.HandleMessage(ctx, makeTimerMsg("cancel",
		withTimerID("cancel-bridge"),
	))
	require.NoError(t, err)

	status, ok := cancelResp.Metadata().Get("timer.status")
	require.True(t, ok)
	assert.Equal(t, "ok", status)

	// list 확인 - 비어야 한다
	listResp, err := handler.HandleMessage(ctx, makeTimerMsg("list"))
	require.NoError(t, err)

	timersVal, _ := listResp.Payload().Get("timers")
	timers := timersVal.([]TimerInfo)
	assert.Empty(t, timers)
}

// TestTimerBridgeHandler_CancelNotFound 는 존재하지 않는 타이머 취소 시 에러를 반환하는지 검증한다.
func TestTimerBridgeHandler_CancelNotFound(t *testing.T) {
	ctx := context.Background()
	handler := newTestTimerBridgeHandler(t)

	cancelResp, err := handler.HandleMessage(ctx, makeTimerMsg("cancel",
		withTimerID("nonexistent"),
	))
	require.NoError(t, err)

	status, ok := cancelResp.Metadata().Get("timer.status")
	require.True(t, ok)
	assert.Equal(t, "error", status)

	errMsg, ok := cancelResp.Metadata().Get("timer.error")
	require.True(t, ok)
	assert.Contains(t, errMsg, "not found")
}

// ---------------------------------------------------------------------------
// 유효하지 않은 연산 테스트
// ---------------------------------------------------------------------------

// TestTimerBridgeHandler_InvalidOperation 은 유효하지 않은 연산에 대해 에러 상태를 반환하는지 검증한다.
func TestTimerBridgeHandler_InvalidOperation(t *testing.T) {
	ctx := context.Background()
	handler := newTestTimerBridgeHandler(t)

	resp, err := handler.HandleMessage(ctx, makeTimerMsg("invalid-op"))
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("timer.status")
	require.True(t, ok)
	assert.Equal(t, "error", status)

	errMsg, ok := resp.Metadata().Get("timer.error")
	require.True(t, ok)
	assert.Contains(t, errMsg, "invalid-op")
}

// ---------------------------------------------------------------------------
// 에러 케이스 테스트
// ---------------------------------------------------------------------------

// TestTimerBridgeHandler_SetInterval_InvalidInterval 은 유효하지 않은 인터벌이 에러를 반환하는지 검증한다.
func TestTimerBridgeHandler_SetInterval_InvalidInterval(t *testing.T) {
	ctx := context.Background()
	handler := newTestTimerBridgeHandler(t)

	resp, err := handler.HandleMessage(ctx, makeTimerMsg("set_interval",
		withTimerID("bad-interval"),
		withTimerInterval("not-a-duration"),
	))
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("timer.status")
	require.True(t, ok)
	assert.Equal(t, "error", status)
}

// TestTimerBridgeHandler_SetCron_InvalidExpression 은 유효하지 않은 cron 표현식이 에러를 반환하는지 검증한다.
func TestTimerBridgeHandler_SetCron_InvalidExpression(t *testing.T) {
	ctx := context.Background()
	handler := newTestTimerBridgeHandler(t)

	resp, err := handler.HandleMessage(ctx, makeTimerMsg("set_cron",
		withTimerID("bad-cron"),
		withTimerCron("invalid cron"),
	))
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("timer.status")
	require.True(t, ok)
	assert.Equal(t, "error", status)
}

// TestTimerBridgeHandler_SetTimeout_InvalidDelay 는 유효하지 않은 지연이 에러를 반환하는지 검증한다.
func TestTimerBridgeHandler_SetTimeout_InvalidDelay(t *testing.T) {
	ctx := context.Background()
	handler := newTestTimerBridgeHandler(t)

	resp, err := handler.HandleMessage(ctx, makeTimerMsg("set_timeout",
		withTimerID("bad-delay"),
		withTimerDelay("not-a-duration"),
	))
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("timer.status")
	require.True(t, ok)
	assert.Equal(t, "error", status)
}

// TestTimerBridgeHandler_SetTimeout_ZeroDelay 는 0 지연이 에러를 반환하는지 검증한다.
func TestTimerBridgeHandler_SetTimeout_ZeroDelay(t *testing.T) {
	ctx := context.Background()
	handler := newTestTimerBridgeHandler(t)

	resp, err := handler.HandleMessage(ctx, makeTimerMsg("set_timeout",
		withTimerID("zero-delay"),
		withTimerDelay("0s"),
	))
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("timer.status")
	require.True(t, ok)
	assert.Equal(t, "error", status)
}

// ---------------------------------------------------------------------------
// set_interval 으로 등록 후 실행 후 list 의 FireCount 확인
// ---------------------------------------------------------------------------

// TestTimerBridgeHandler_SetInterval_CheckFireCount 는 인터벌 타이머 실행 후 FireCount를 확인한다.
func TestTimerBridgeHandler_SetInterval_CheckFireCount(t *testing.T) {
	ctx := context.Background()
	handler := newTestTimerBridgeHandler(t)

	_, err := handler.HandleMessage(ctx, makeTimerMsg("set_interval",
		withTimerID("fire-count"),
		withTimerInterval("100ms"),
	))
	require.NoError(t, err)

	// 실행 대기
	time.Sleep(350 * time.Millisecond)

	// list 조회
	listResp, err := handler.HandleMessage(ctx, makeTimerMsg("list"))
	require.NoError(t, err)

	timersVal, ok := listResp.Payload().Get("timers")
	require.True(t, ok)

	timers := timersVal.([]TimerInfo)
	require.Len(t, timers, 1)
	assert.GreaterOrEqual(t, timers[0].FireCount, int64(2),
		"최소 2회 이상 실행되어야 한다")
}
