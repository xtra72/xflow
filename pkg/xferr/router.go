package xferr

import (
	"context"
	"sync"
)

// ErrorReceiver 는 에러 메시지를 수신하는 인터페이스이다.
type ErrorReceiver interface {
	// ReceiveError 는 에러 메시지를 수신하여 처리한다.
	ReceiveError(ctx context.Context, errMsg ErrorMessage) error
}

// DeadLetterReceiver 는 데드레터 메시지를 수신하는 인터페이스이다.
type DeadLetterReceiver interface {
	// ReceiveDeadLetter 는 데드레터 메시지를 수신하여 처리한다.
	ReceiveDeadLetter(ctx context.Context, dlMsg DeadLetterMessage) error
}

// StatusReceiver 는 상태 이벤트를 수신하는 인터페이스이다.
type StatusReceiver interface {
	// ReceiveStatus 는 상태 이벤트를 수신하여 처리한다.
	ReceiveStatus(ctx context.Context, evt StatusEvent) error
}

// ErrorRouter 는 에러, 데드레터, 상태 이벤트를 라우팅하는 인터페이스이다.
type ErrorRouter interface {
	// RouteError 는 에러 메시지를 등록된 수신자에게 라우팅한다.
	RouteError(ctx context.Context, errMsg ErrorMessage) error
	// RouteDeadLetter 는 데드레터 메시지를 등록된 수신자에게 라우팅한다.
	RouteDeadLetter(ctx context.Context, dlMsg DeadLetterMessage) error
	// RouteStatus 는 상태 이벤트를 등록된 수신자에게 라우팅한다.
	RouteStatus(ctx context.Context, evt StatusEvent) error
	// RegisterErrorReceiver 는 지정된 스코프에 에러 수신자를 등록한다.
	RegisterErrorReceiver(scope string, receiver ErrorReceiver)
	// RegisterDeadLetterReceiver 는 데드레터 수신자를 등록한다.
	RegisterDeadLetterReceiver(receiver DeadLetterReceiver)
	// RegisterStatusReceiver 는 상태 수신자를 등록한다.
	RegisterStatusReceiver(receiver StatusReceiver)
}

// DefaultErrorRouter 는 ErrorRouter 인터페이스의 기본 구현체이다.
type DefaultErrorRouter struct {
	mu                  sync.RWMutex
	errorReceivers      map[string][]ErrorReceiver
	deadLetterReceivers []DeadLetterReceiver
	statusReceivers     []StatusReceiver
	policy              DiscardPolicy
}

// NewDefaultErrorRouter 는 새로운 DefaultErrorRouter 인스턴스를 생성한다.
func NewDefaultErrorRouter(policy DiscardPolicy) *DefaultErrorRouter {
	return &DefaultErrorRouter{
		errorReceivers:      make(map[string][]ErrorReceiver),
		deadLetterReceivers: make([]DeadLetterReceiver, 0),
		statusReceivers:     make([]StatusReceiver, 0),
		policy:              policy,
	}
}

// RegisterErrorReceiver 는 지정된 스코프에 에러 수신자를 등록한다.
// scope가 "*"이면 모든 에러를 수신한다.
func (r *DefaultErrorRouter) RegisterErrorReceiver(scope string, receiver ErrorReceiver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errorReceivers[scope] = append(r.errorReceivers[scope], receiver)
}

// RegisterDeadLetterReceiver 는 데드레터 수신자를 등록한다.
func (r *DefaultErrorRouter) RegisterDeadLetterReceiver(receiver DeadLetterReceiver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deadLetterReceivers = append(r.deadLetterReceivers, receiver)
}

// RegisterStatusReceiver 는 상태 수신자를 등록한다.
func (r *DefaultErrorRouter) RegisterStatusReceiver(receiver StatusReceiver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statusReceivers = append(r.statusReceivers, receiver)
}

// RouteError 는 에러 메시지를 매칭되는 수신자에게 팬아웃으로 전달한다.
// 정확한 스코프 매칭 후 와일드카드("*") 매칭을 수행한다.
// 매칭되는 수신자가 없으면 DiscardPolicy로 처리한다.
func (r *DefaultErrorRouter) RouteError(ctx context.Context, errMsg ErrorMessage) error {
	r.mu.RLock()
	// 매칭되는 수신자 수집
	var matched []ErrorReceiver
	sourceNodeID := errMsg.SourceNodeID()

	// 정확한 매칭
	if receivers, ok := r.errorReceivers[sourceNodeID]; ok {
		matched = append(matched, receivers...)
	}

	// 와일드카드 매칭
	if wildcardReceivers, ok := r.errorReceivers["*"]; ok {
		matched = append(matched, wildcardReceivers...)
	}
	r.mu.RUnlock()

	// 매칭된 수신자가 없으면 폐기 정책 적용
	if len(matched) == 0 {
		r.policy.HandleError(ctx, errMsg)
		return nil
	}

	// 모든 매칭 수신자에게 팬아웃
	for _, receiver := range matched {
		if err := receiver.ReceiveError(ctx, errMsg); err != nil {
			return err
		}
	}

	return nil
}

// RouteDeadLetter 는 데드레터 메시지를 모든 등록된 수신자에게 전달한다.
// 수신자가 없으면 DiscardPolicy로 처리한다.
func (r *DefaultErrorRouter) RouteDeadLetter(ctx context.Context, dlMsg DeadLetterMessage) error {
	r.mu.RLock()
	receivers := make([]DeadLetterReceiver, len(r.deadLetterReceivers))
	copy(receivers, r.deadLetterReceivers)
	r.mu.RUnlock()

	if len(receivers) == 0 {
		r.policy.HandleDeadLetter(ctx, dlMsg)
		return nil
	}

	for _, receiver := range receivers {
		if err := receiver.ReceiveDeadLetter(ctx, dlMsg); err != nil {
			return err
		}
	}

	return nil
}

// RouteStatus 는 상태 이벤트를 모든 등록된 수신자에게 전달한다.
// 수신자가 없으면 DiscardPolicy로 처리한다.
func (r *DefaultErrorRouter) RouteStatus(ctx context.Context, evt StatusEvent) error {
	r.mu.RLock()
	receivers := make([]StatusReceiver, len(r.statusReceivers))
	copy(receivers, r.statusReceivers)
	r.mu.RUnlock()

	if len(receivers) == 0 {
		r.policy.HandleStatus(ctx, evt)
		return nil
	}

	for _, receiver := range receivers {
		if err := receiver.ReceiveStatus(ctx, evt); err != nil {
			return err
		}
	}

	return nil
}
