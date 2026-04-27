package system

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/xtra/xflow/pkg/message"
)

// LoggerBridgeHandler 는 Bridge Node를 통한 메시지 기반 Logger 접근을 처리한다.
// 수신된 메시지의 메타데이터에서 연산 종류를 판별하고, LoggerAgent에 위임한 뒤 응답 메시지를 반환한다.
type LoggerBridgeHandler struct {
	agent *LoggerAgent
}

// NewLoggerBridgeHandler 는 LoggerAgent를 사용하는 LoggerBridgeHandler를 생성한다.
func NewLoggerBridgeHandler(agent *LoggerAgent) *LoggerBridgeHandler {
	return &LoggerBridgeHandler{agent: agent}
}

// HandleMessage 는 Bridge Node에서 수신한 메시지를 처리하고 응답 메시지를 반환한다.
//
// 메시지 메타데이터 필드:
//   - logger.operation: "write", "set_level", "set_level_pattern", "get_level", "subscribe", "unsubscribe", "components"
//   - logger.component: 대상 컴포넌트 이름
//   - logger.level: 로그 레벨 문자열 ("debug", "info", "warn", "error")
//   - logger.pattern: 패턴 매칭용 문자열 (set_level_pattern 연산)
//   - logger.subscription_id: 구독 ID (unsubscribe 연산)
//
// 응답 메시지:
//   - Metadata "logger.status": "ok" 또는 "error"
//   - Metadata "logger.error": 에러 메시지 (status가 "error"일 때)
//   - Payload: 연산 결과 (get_level→level, subscribe→subscription_id, components→components)
func (h *LoggerBridgeHandler) HandleMessage(ctx context.Context, msg message.Message) (message.Message, error) {
	operation, _ := msg.Metadata().Get("logger.operation")

	switch operation {
	case "write":
		return h.handleWrite(ctx, msg)
	case "set_level":
		return h.handleSetLevel(ctx, msg)
	case "set_level_pattern":
		return h.handleSetLevelPattern(ctx, msg)
	case "get_level":
		return h.handleGetLevel(ctx, msg)
	case "subscribe":
		return h.handleSubscribe(ctx, msg)
	case "unsubscribe":
		return h.handleUnsubscribe(ctx, msg)
	case "components":
		return h.handleComponents(ctx)
	default:
		return h.errorResponse(fmt.Sprintf("지원하지 않는 연산: %s", operation)), nil
	}
}

// handleWrite 는 write 연산을 처리하고 응답 메시지를 반환한다.
func (h *LoggerBridgeHandler) handleWrite(ctx context.Context, msg message.Message) (message.Message, error) {
	component, _ := msg.Metadata().Get("logger.component")
	levelStr, _ := msg.Metadata().Get("logger.level")
	logMsg, _ := msg.Payload().Get("message")

	level, err := parseLogLevel(levelStr)
	if err != nil {
		return h.errorResponse(err.Error()), nil
	}

	msgStr := ""
	if logMsg != nil {
		msgStr = fmt.Sprintf("%v", logMsg)
	}

	if err := h.agent.WriteLog(ctx, component, level, msgStr); err != nil {
		return h.errorResponse(err.Error()), nil
	}

	return h.okResponse(), nil
}

// handleSetLevel 은 set_level 연산을 처리하고 응답 메시지를 반환한다.
func (h *LoggerBridgeHandler) handleSetLevel(ctx context.Context, msg message.Message) (message.Message, error) {
	component, _ := msg.Metadata().Get("logger.component")
	levelStr, _ := msg.Metadata().Get("logger.level")

	level, err := parseLogLevel(levelStr)
	if err != nil {
		return h.errorResponse(err.Error()), nil
	}

	if err := h.agent.SetLevel(ctx, component, level); err != nil {
		return h.errorResponse(err.Error()), nil
	}

	return h.okResponse(), nil
}

// handleSetLevelPattern 은 set_level_pattern 연산을 처리하고 응답 메시지를 반환한다.
func (h *LoggerBridgeHandler) handleSetLevelPattern(ctx context.Context, msg message.Message) (message.Message, error) {
	pattern, _ := msg.Metadata().Get("logger.pattern")
	levelStr, _ := msg.Metadata().Get("logger.level")

	level, err := parseLogLevel(levelStr)
	if err != nil {
		return h.errorResponse(err.Error()), nil
	}

	count, err := h.agent.SetLevelByPattern(ctx, pattern, level)
	if err != nil {
		return h.errorResponse(err.Error()), nil
	}

	resp := h.okResponse()
	resp.Payload().Set("count", count)
	return resp, nil
}

// handleGetLevel 은 get_level 연산을 처리하고 응답 메시지를 반환한다.
func (h *LoggerBridgeHandler) handleGetLevel(ctx context.Context, msg message.Message) (message.Message, error) {
	component, _ := msg.Metadata().Get("logger.component")

	level, err := h.agent.GetLevel(ctx, component)
	if err != nil {
		return h.errorResponse(err.Error()), nil
	}

	resp := h.okResponse()
	resp.Payload().Set("level", level.String())
	return resp, nil
}

// handleSubscribe 는 subscribe 연산을 처리하고 응답 메시지를 반환한다.
// Bridge에서의 구독은 io.Discard를 사용한다 (실제 스트림은 별도 채널로 전달).
func (h *LoggerBridgeHandler) handleSubscribe(ctx context.Context, msg message.Message) (message.Message, error) {
	component, _ := msg.Metadata().Get("logger.component")

	subID, err := h.agent.Subscribe(ctx, component, io.Discard)
	if err != nil {
		return h.errorResponse(err.Error()), nil
	}

	resp := h.okResponse()
	resp.Payload().Set("subscription_id", subID)
	return resp, nil
}

// handleUnsubscribe 는 unsubscribe 연산을 처리하고 응답 메시지를 반환한다.
func (h *LoggerBridgeHandler) handleUnsubscribe(ctx context.Context, msg message.Message) (message.Message, error) {
	subID, _ := msg.Metadata().Get("logger.subscription_id")

	if err := h.agent.Unsubscribe(ctx, subID); err != nil {
		return h.errorResponse(err.Error()), nil
	}

	return h.okResponse(), nil
}

// handleComponents 는 components 연산을 처리하고 응답 메시지를 반환한다.
func (h *LoggerBridgeHandler) handleComponents(ctx context.Context) (message.Message, error) {
	comps, err := h.agent.Components(ctx)
	if err != nil {
		return h.errorResponse(err.Error()), nil
	}

	if comps == nil {
		comps = []string{}
	}

	resp := h.okResponse()
	resp.Payload().Set("components", comps)
	return resp, nil
}

// okResponse 는 성공 상태의 응답 메시지를 생성한다.
func (h *LoggerBridgeHandler) okResponse() message.Message {
	resp := message.New()
	resp.Metadata().Set("logger.status", "ok")
	return resp
}

// errorResponse 는 에러 상태의 응답 메시지를 생성한다.
func (h *LoggerBridgeHandler) errorResponse(errMsg string) message.Message {
	resp := message.New()
	resp.Metadata().Set("logger.status", "error")
	resp.Metadata().Set("logger.error", errMsg)
	return resp
}

// parseLogLevel 은 문자열을 slog.Level로 변환한다.
// 유효하지 않은 레벨 문자열이면 ErrInvalidLevel을 반환한다.
func parseLogLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, ErrInvalidLevel
	}
}
