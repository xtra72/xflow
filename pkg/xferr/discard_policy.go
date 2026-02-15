package xferr

import (
	"context"
	"fmt"
)

// Logger 는 로깅을 위한 인터페이스이다.
type Logger interface {
	// Error 는 에러 수준의 로그를 기록한다.
	Error(msg string, args ...any)
	// Warn 은 경고 수준의 로그를 기록한다.
	Warn(msg string, args ...any)
	// Info 는 정보 수준의 로그를 기록한다.
	Info(msg string, args ...any)
}

// Metrics 는 메트릭 수집을 위한 인터페이스이다.
type Metrics interface {
	// IncrCounter 는 지정된 카운터를 1 증가시킨다.
	IncrCounter(name string, labels map[string]string)
}

// DiscardPolicy 는 수신자가 없는 에러, 데드레터, 상태 이벤트를 처리하는 정책 인터페이스이다.
type DiscardPolicy interface {
	// HandleError 는 수신자 없는 에러 메시지를 처리한다.
	HandleError(ctx context.Context, errMsg ErrorMessage)
	// HandleDeadLetter 는 수신자 없는 데드레터 메시지를 처리한다.
	HandleDeadLetter(ctx context.Context, dlMsg DeadLetterMessage)
	// HandleStatus 는 수신자 없는 상태 이벤트를 처리한다.
	HandleStatus(ctx context.Context, evt StatusEvent)
}

// === LogAndDiscardPolicy ===

// logAndDiscardPolicy 는 로깅 후 메트릭을 증가시키고 폐기하는 정책이다.
type logAndDiscardPolicy struct {
	logger  Logger
	metrics Metrics
}

// NewLogAndDiscardPolicy 는 로깅 후 폐기하는 정책을 생성한다.
func NewLogAndDiscardPolicy(logger Logger, metrics Metrics) DiscardPolicy {
	return &logAndDiscardPolicy{
		logger:  logger,
		metrics: metrics,
	}
}

func (p *logAndDiscardPolicy) HandleError(ctx context.Context, errMsg ErrorMessage) {
	p.logger.Error(fmt.Sprintf("에러 메시지 폐기: source=%s severity=%s category=%s err=%v",
		errMsg.SourceNodeID(), errMsg.Severity(), errMsg.Category(), errMsg.Error()))
	p.metrics.IncrCounter("xferr.error.discarded", map[string]string{
		"severity": string(errMsg.Severity()),
		"category": string(errMsg.Category()),
	})
}

func (p *logAndDiscardPolicy) HandleDeadLetter(ctx context.Context, dlMsg DeadLetterMessage) {
	p.logger.Warn(fmt.Sprintf("데드레터 메시지 폐기: reason=%s source_node=%s",
		dlMsg.Reason(), dlMsg.SourceNodeID()))
	p.metrics.IncrCounter("xferr.deadletter.discarded", map[string]string{
		"reason": string(dlMsg.Reason()),
	})
}

func (p *logAndDiscardPolicy) HandleStatus(ctx context.Context, evt StatusEvent) {
	p.logger.Info(fmt.Sprintf("상태 이벤트 폐기: component=%s/%s %s->%s",
		evt.ComponentType(), evt.ComponentID(), evt.PreviousState(), evt.NewState()))
	p.metrics.IncrCounter("xferr.status.discarded", map[string]string{
		"component_type": string(evt.ComponentType()),
	})
}

// === SilentDiscardPolicy ===

// silentDiscardPolicy 는 로깅 없이 메트릭만 증가시키고 폐기하는 정책이다.
type silentDiscardPolicy struct {
	metrics Metrics
}

// NewSilentDiscardPolicy 는 로깅 없이 메트릭만 기록하는 폐기 정책을 생성한다.
func NewSilentDiscardPolicy(metrics Metrics) DiscardPolicy {
	return &silentDiscardPolicy{
		metrics: metrics,
	}
}

func (p *silentDiscardPolicy) HandleError(ctx context.Context, errMsg ErrorMessage) {
	p.metrics.IncrCounter("xferr.error.discarded", map[string]string{
		"severity": string(errMsg.Severity()),
		"category": string(errMsg.Category()),
	})
}

func (p *silentDiscardPolicy) HandleDeadLetter(ctx context.Context, dlMsg DeadLetterMessage) {
	p.metrics.IncrCounter("xferr.deadletter.discarded", map[string]string{
		"reason": string(dlMsg.Reason()),
	})
}

func (p *silentDiscardPolicy) HandleStatus(ctx context.Context, evt StatusEvent) {
	p.metrics.IncrCounter("xferr.status.discarded", map[string]string{
		"component_type": string(evt.ComponentType()),
	})
}

// === PanicOnCriticalPolicy ===

// panicOnCriticalPolicy 는 치명적 에러 시 패닉을 발생시키고, 그 외에는 LogAndDiscard와 동일하게 동작하는 정책이다.
type panicOnCriticalPolicy struct {
	logger  Logger
	metrics Metrics
}

// NewPanicOnCriticalPolicy 는 치명적 에러 시 패닉을 발생시키는 정책을 생성한다.
func NewPanicOnCriticalPolicy(logger Logger, metrics Metrics) DiscardPolicy {
	return &panicOnCriticalPolicy{
		logger:  logger,
		metrics: metrics,
	}
}

func (p *panicOnCriticalPolicy) HandleError(ctx context.Context, errMsg ErrorMessage) {
	if errMsg.Severity() == SeverityCritical {
		panic(fmt.Sprintf("xferr: 치명적 에러 발생: source=%s err=%v",
			errMsg.SourceNodeID(), errMsg.Error()))
	}

	// 비치명적 에러는 LogAndDiscard와 동일하게 처리
	p.logger.Error(fmt.Sprintf("에러 메시지 폐기: source=%s severity=%s category=%s err=%v",
		errMsg.SourceNodeID(), errMsg.Severity(), errMsg.Category(), errMsg.Error()))
	p.metrics.IncrCounter("xferr.error.discarded", map[string]string{
		"severity": string(errMsg.Severity()),
		"category": string(errMsg.Category()),
	})
}

func (p *panicOnCriticalPolicy) HandleDeadLetter(ctx context.Context, dlMsg DeadLetterMessage) {
	p.logger.Warn(fmt.Sprintf("데드레터 메시지 폐기: reason=%s source_node=%s",
		dlMsg.Reason(), dlMsg.SourceNodeID()))
	p.metrics.IncrCounter("xferr.deadletter.discarded", map[string]string{
		"reason": string(dlMsg.Reason()),
	})
}

func (p *panicOnCriticalPolicy) HandleStatus(ctx context.Context, evt StatusEvent) {
	p.logger.Info(fmt.Sprintf("상태 이벤트 폐기: component=%s/%s %s->%s",
		evt.ComponentType(), evt.ComponentID(), evt.PreviousState(), evt.NewState()))
	p.metrics.IncrCounter("xferr.status.discarded", map[string]string{
		"component_type": string(evt.ComponentType()),
	})
}
