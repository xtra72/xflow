package system

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 테스트 헬퍼
// ---------------------------------------------------------------------------

// newTestLoggerBridgeHandler 는 초기화된 LoggerAgent와 LoggerBridgeHandler를 생성한다.
func newTestLoggerBridgeHandler(t *testing.T) *LoggerBridgeHandler {
	t.Helper()
	agent := newTestLoggerAgent(t,
		WithLogWriter(io.Discard),
		WithLogDefaultLevel(slog.LevelDebug),
	)
	return NewLoggerBridgeHandler(agent)
}

// makeLoggerMsg 는 Logger 연산을 위한 메시지를 생성한다.
func makeLoggerMsg(operation string, opts ...func(message.Message)) message.Message {
	msg := message.New()
	msg.Metadata().Set("logger.operation", operation)
	for _, opt := range opts {
		opt(msg)
	}
	return msg
}

// withLogComponent 는 메시지 메타데이터에 logger.component를 설정하는 옵션이다.
func withLogComponent(component string) func(message.Message) {
	return func(msg message.Message) {
		msg.Metadata().Set("logger.component", component)
	}
}

// withLogLevel 은 메시지 메타데이터에 logger.level을 설정하는 옵션이다.
func withLogLevel(level string) func(message.Message) {
	return func(msg message.Message) {
		msg.Metadata().Set("logger.level", level)
	}
}

// withLogMessage 는 메시지 페이로드에 "message"를 설정하는 옵션이다.
func withLogMessage(logMsg string) func(message.Message) {
	return func(msg message.Message) {
		msg.Payload().Set("message", logMsg)
	}
}

// withLogPattern 은 메시지 메타데이터에 logger.pattern을 설정하는 옵션이다.
func withLogPattern(pattern string) func(message.Message) {
	return func(msg message.Message) {
		msg.Metadata().Set("logger.pattern", pattern)
	}
}

// withSubscriptionID 는 메시지 메타데이터에 logger.subscription_id를 설정하는 옵션이다.
func withSubscriptionID(id string) func(message.Message) {
	return func(msg message.Message) {
		msg.Metadata().Set("logger.subscription_id", id)
	}
}

// ---------------------------------------------------------------------------
// write 연산 테스트
// ---------------------------------------------------------------------------

func TestLoggerBridgeHandler_Write_Success(t *testing.T) {
	ctx := context.Background()
	handler := newTestLoggerBridgeHandler(t)

	msg := makeLoggerMsg("write",
		withLogComponent("test-comp"),
		withLogLevel("info"),
		withLogMessage("hello world"),
	)

	resp, err := handler.HandleMessage(ctx, msg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("logger.status")
	require.True(t, ok)
	assert.Equal(t, "ok", status)
}

func TestLoggerBridgeHandler_Write_AllLevels(t *testing.T) {
	ctx := context.Background()
	handler := newTestLoggerBridgeHandler(t)

	levels := []string{"debug", "info", "warn", "error"}
	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			msg := makeLoggerMsg("write",
				withLogComponent("comp"),
				withLogLevel(level),
				withLogMessage("test msg"),
			)

			resp, err := handler.HandleMessage(ctx, msg)
			require.NoError(t, err)

			status, ok := resp.Metadata().Get("logger.status")
			require.True(t, ok)
			assert.Equal(t, "ok", status)
		})
	}
}

func TestLoggerBridgeHandler_Write_InvalidLevel(t *testing.T) {
	ctx := context.Background()
	handler := newTestLoggerBridgeHandler(t)

	msg := makeLoggerMsg("write",
		withLogComponent("comp"),
		withLogLevel("invalid"),
		withLogMessage("test"),
	)

	resp, err := handler.HandleMessage(ctx, msg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("logger.status")
	require.True(t, ok)
	assert.Equal(t, "error", status)
}

// ---------------------------------------------------------------------------
// set_level 연산 테스트
// ---------------------------------------------------------------------------

func TestLoggerBridgeHandler_SetLevel_Success(t *testing.T) {
	ctx := context.Background()
	handler := newTestLoggerBridgeHandler(t)

	msg := makeLoggerMsg("set_level",
		withLogComponent("comp"),
		withLogLevel("debug"),
	)

	resp, err := handler.HandleMessage(ctx, msg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("logger.status")
	require.True(t, ok)
	assert.Equal(t, "ok", status)
}

func TestLoggerBridgeHandler_SetLevel_InvalidLevel(t *testing.T) {
	ctx := context.Background()
	handler := newTestLoggerBridgeHandler(t)

	msg := makeLoggerMsg("set_level",
		withLogComponent("comp"),
		withLogLevel("invalid"),
	)

	resp, err := handler.HandleMessage(ctx, msg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("logger.status")
	require.True(t, ok)
	assert.Equal(t, "error", status)
}

// ---------------------------------------------------------------------------
// set_level_pattern 연산 테스트
// ---------------------------------------------------------------------------

func TestLoggerBridgeHandler_SetLevelPattern_Success(t *testing.T) {
	ctx := context.Background()
	handler := newTestLoggerBridgeHandler(t)

	// 먼저 컴포넌트를 등록
	writeMsg := makeLoggerMsg("write",
		withLogComponent("agent.timer"),
		withLogLevel("info"),
		withLogMessage("init"),
	)
	_, err := handler.HandleMessage(ctx, writeMsg)
	require.NoError(t, err)

	// 패턴으로 레벨 설정
	msg := makeLoggerMsg("set_level_pattern",
		withLogPattern("agent.*"),
		withLogLevel("debug"),
	)

	resp, err := handler.HandleMessage(ctx, msg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("logger.status")
	require.True(t, ok)
	assert.Equal(t, "ok", status)
}

// ---------------------------------------------------------------------------
// get_level 연산 테스트
// ---------------------------------------------------------------------------

func TestLoggerBridgeHandler_GetLevel_Success(t *testing.T) {
	ctx := context.Background()
	handler := newTestLoggerBridgeHandler(t)

	msg := makeLoggerMsg("get_level",
		withLogComponent("comp"),
	)

	resp, err := handler.HandleMessage(ctx, msg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("logger.status")
	require.True(t, ok)
	assert.Equal(t, "ok", status)

	// 레벨 값이 페이로드에 포함되어야 한다
	level, ok := resp.Payload().Get("level")
	require.True(t, ok)
	assert.NotNil(t, level)
}

// ---------------------------------------------------------------------------
// subscribe 연산 테스트
// ---------------------------------------------------------------------------

func TestLoggerBridgeHandler_Subscribe_Success(t *testing.T) {
	ctx := context.Background()
	handler := newTestLoggerBridgeHandler(t)

	msg := makeLoggerMsg("subscribe",
		withLogComponent("comp"),
	)

	resp, err := handler.HandleMessage(ctx, msg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("logger.status")
	require.True(t, ok)
	assert.Equal(t, "ok", status)

	subID, ok := resp.Payload().Get("subscription_id")
	require.True(t, ok)
	assert.NotEmpty(t, subID)
}

// ---------------------------------------------------------------------------
// unsubscribe 연산 테스트
// ---------------------------------------------------------------------------

func TestLoggerBridgeHandler_Unsubscribe_Success(t *testing.T) {
	ctx := context.Background()
	handler := newTestLoggerBridgeHandler(t)

	// 먼저 구독
	subMsg := makeLoggerMsg("subscribe",
		withLogComponent("comp"),
	)
	subResp, err := handler.HandleMessage(ctx, subMsg)
	require.NoError(t, err)

	subID, ok := subResp.Payload().Get("subscription_id")
	require.True(t, ok)

	// 구독 해제
	unsubMsg := makeLoggerMsg("unsubscribe",
		withSubscriptionID(subID.(string)),
	)
	resp, err := handler.HandleMessage(ctx, unsubMsg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("logger.status")
	require.True(t, ok)
	assert.Equal(t, "ok", status)
}

func TestLoggerBridgeHandler_Unsubscribe_NotFound(t *testing.T) {
	ctx := context.Background()
	handler := newTestLoggerBridgeHandler(t)

	msg := makeLoggerMsg("unsubscribe",
		withSubscriptionID("nonexistent-id"),
	)

	resp, err := handler.HandleMessage(ctx, msg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("logger.status")
	require.True(t, ok)
	assert.Equal(t, "error", status)
}

// ---------------------------------------------------------------------------
// components 연산 테스트
// ---------------------------------------------------------------------------

func TestLoggerBridgeHandler_Components_Success(t *testing.T) {
	ctx := context.Background()
	handler := newTestLoggerBridgeHandler(t)

	// 컴포넌트를 등록
	writeMsg := makeLoggerMsg("write",
		withLogComponent("comp-a"),
		withLogLevel("info"),
		withLogMessage("init"),
	)
	_, err := handler.HandleMessage(ctx, writeMsg)
	require.NoError(t, err)

	// components 조회
	msg := makeLoggerMsg("components")
	resp, err := handler.HandleMessage(ctx, msg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("logger.status")
	require.True(t, ok)
	assert.Equal(t, "ok", status)

	comps, ok := resp.Payload().Get("components")
	require.True(t, ok)
	assert.NotNil(t, comps)
}

// ---------------------------------------------------------------------------
// 유효하지 않은 연산 테스트
// ---------------------------------------------------------------------------

func TestLoggerBridgeHandler_InvalidOperation(t *testing.T) {
	ctx := context.Background()
	handler := newTestLoggerBridgeHandler(t)

	msg := makeLoggerMsg("invalid_op")
	resp, err := handler.HandleMessage(ctx, msg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("logger.status")
	require.True(t, ok)
	assert.Equal(t, "error", status)

	errMsg, ok := resp.Metadata().Get("logger.error")
	require.True(t, ok)
	assert.Contains(t, errMsg, "invalid_op")
}

// ---------------------------------------------------------------------------
// parseLogLevel 테스트
// ---------------------------------------------------------------------------

func TestParseLogLevel_ValidLevels(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"DEBUG", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{"WARN", slog.LevelWarn},
		{"ERROR", slog.LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level, err := parseLogLevel(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, level)
		})
	}
}

func TestParseLogLevel_InvalidLevel(t *testing.T) {
	_, err := parseLogLevel("invalid")
	assert.ErrorIs(t, err, ErrInvalidLevel)
}

func TestParseLogLevel_EmptyString(t *testing.T) {
	_, err := parseLogLevel("")
	assert.ErrorIs(t, err, ErrInvalidLevel)
}
