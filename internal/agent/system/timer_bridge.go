package system

import (
	"context"
	"fmt"
	"time"

	"github.com/xtra/xflow/pkg/message"
)

// TimerBridgeHandler 는 Bridge Node를 통한 메시지 기반 Timer 접근을 처리한다.
// 수신된 메시지의 메타데이터에서 연산 종류를 판별하고, TimerAgent에 위임한 뒤 응답 메시지를 반환한다.
type TimerBridgeHandler struct {
	agent *TimerAgent
}

// NewTimerBridgeHandler 는 TimerAgent를 사용하는 TimerBridgeHandler를 생성한다.
func NewTimerBridgeHandler(agent *TimerAgent) *TimerBridgeHandler {
	return &TimerBridgeHandler{agent: agent}
}

// HandleMessage 는 Bridge Node에서 수신한 메시지를 처리하고 응답 메시지를 반환한다.
//
// 메시지 메타데이터 필드:
//   - timer.operation: "set_interval", "set_cron", "set_timeout", "cancel", "list"
//   - timer.id: 타이머 ID
//   - timer.interval: 인터벌 문자열 (set_interval용)
//   - timer.cron: cron 표현식 (set_cron용)
//   - timer.delay: 지연 시간 문자열 (set_timeout용)
//
// 응답 메시지:
//   - Metadata "timer.status": "ok" 또는 "error"
//   - Metadata "timer.error": 에러 메시지 (status가 "error"일 때)
//   - Payload "timers": TimerInfo 슬라이스 (list 연산용)
func (h *TimerBridgeHandler) HandleMessage(ctx context.Context, msg message.Message) (message.Message, error) {
	operation, _ := msg.Metadata().Get("timer.operation")

	switch operation {
	case "set_interval":
		return h.handleSetInterval(ctx, msg)
	case "set_cron":
		return h.handleSetCron(ctx, msg)
	case "set_timeout":
		return h.handleSetTimeout(ctx, msg)
	case "cancel":
		return h.handleCancel(ctx, msg)
	case "list":
		return h.handleList()
	default:
		return h.errorResponse(fmt.Sprintf("지원하지 않는 연산: %s", operation)), nil
	}
}

// handleSetInterval 은 set_interval 연산을 처리한다.
// Bridge를 통한 등록은 no-op 핸들러를 사용한다 (트리거 정보는 List로 조회 가능).
func (h *TimerBridgeHandler) handleSetInterval(_ context.Context, msg message.Message) (message.Message, error) {
	id, _ := msg.Metadata().Get("timer.id")
	intervalStr, _ := msg.Metadata().Get("timer.interval")

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		return h.errorResponse(fmt.Sprintf("유효하지 않은 인터벌: %s", intervalStr)), nil
	}

	// Bridge를 통한 등록은 no-op 핸들러를 사용한다
	_, err = h.agent.SetInterval(id, interval, func(_ TimerTrigger) {})
	if err != nil {
		return h.errorResponse(err.Error()), nil
	}

	resp := message.New()
	resp.Metadata().Set("timer.status", "ok")
	return resp, nil
}

// handleSetCron 은 set_cron 연산을 처리한다.
func (h *TimerBridgeHandler) handleSetCron(_ context.Context, msg message.Message) (message.Message, error) {
	id, _ := msg.Metadata().Get("timer.id")
	cronExpr, _ := msg.Metadata().Get("timer.cron")

	_, err := h.agent.SetCron(id, cronExpr, func(_ TimerTrigger) {})
	if err != nil {
		return h.errorResponse(err.Error()), nil
	}

	resp := message.New()
	resp.Metadata().Set("timer.status", "ok")
	return resp, nil
}

// handleSetTimeout 은 set_timeout 연산을 처리한다.
func (h *TimerBridgeHandler) handleSetTimeout(_ context.Context, msg message.Message) (message.Message, error) {
	id, _ := msg.Metadata().Get("timer.id")
	delayStr, _ := msg.Metadata().Get("timer.delay")

	delay, err := time.ParseDuration(delayStr)
	if err != nil {
		return h.errorResponse(fmt.Sprintf("유효하지 않은 지연 시간: %s", delayStr)), nil
	}

	_, err = h.agent.SetTimeout(id, delay, func(_ TimerTrigger) {})
	if err != nil {
		return h.errorResponse(err.Error()), nil
	}

	resp := message.New()
	resp.Metadata().Set("timer.status", "ok")
	return resp, nil
}

// handleCancel 은 cancel 연산을 처리한다.
func (h *TimerBridgeHandler) handleCancel(_ context.Context, msg message.Message) (message.Message, error) {
	id, _ := msg.Metadata().Get("timer.id")

	err := h.agent.Cancel(TimerID(id))
	if err != nil {
		return h.errorResponse(err.Error()), nil
	}

	resp := message.New()
	resp.Metadata().Set("timer.status", "ok")
	return resp, nil
}

// handleList 는 list 연산을 처리한다.
func (h *TimerBridgeHandler) handleList() (message.Message, error) {
	infos := h.agent.List()

	resp := message.New()
	resp.Metadata().Set("timer.status", "ok")
	resp.Payload().Set("timers", infos)
	return resp, nil
}

// errorResponse 는 에러 상태의 응답 메시지를 생성한다.
func (h *TimerBridgeHandler) errorResponse(errMsg string) message.Message {
	resp := message.New()
	resp.Metadata().Set("timer.status", "error")
	resp.Metadata().Set("timer.error", errMsg)
	return resp
}
