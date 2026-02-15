package xferr

import (
	"time"

	"github.com/xtra/xflow/pkg/message"
)

// ErrorMessage 는 에러가 발생한 메시지를 래핑하는 인터페이스이다.
type ErrorMessage interface {
	// OriginalMessage 는 에러가 발생한 원본 메시지를 반환한다.
	OriginalMessage() message.Message
	// Error 는 발생한 에러를 반환한다.
	Error() error
	// SourceNodeID 는 에러가 발생한 노드의 ID를 반환한다.
	SourceNodeID() string
	// ErrorTimestamp 는 에러 발생 시각을 반환한다.
	ErrorTimestamp() time.Time
	// Severity 는 에러의 심각도를 반환한다.
	Severity() ErrorSeverity
	// Category 는 에러의 분류 카테고리를 반환한다.
	Category() ErrorCategory
	// StackContext 는 에러 발생 위치의 컨텍스트 정보를 반환한다.
	StackContext() string
}

// ErrorMessageOption 은 ErrorMessage 생성 시 적용할 옵션 함수 타입이다.
type ErrorMessageOption func(*defaultErrorMessage)

// WithSeverity 는 에러의 심각도를 설정한다.
func WithSeverity(s ErrorSeverity) ErrorMessageOption {
	return func(em *defaultErrorMessage) {
		em.severity = s
	}
}

// WithCategory 는 에러의 분류 카테고리를 설정한다.
func WithCategory(c ErrorCategory) ErrorMessageOption {
	return func(em *defaultErrorMessage) {
		em.category = c
	}
}

// WithStackContext 는 에러 발생 위치의 컨텍스트 정보를 설정한다.
func WithStackContext(ctx string) ErrorMessageOption {
	return func(em *defaultErrorMessage) {
		em.stackContext = ctx
	}
}

// defaultErrorMessage 는 ErrorMessage 인터페이스의 기본 구현체이다.
type defaultErrorMessage struct {
	originalMessage message.Message
	err             error
	sourceNodeID    string
	errorTimestamp  time.Time
	severity        ErrorSeverity
	category        ErrorCategory
	stackContext    string
}

// NewErrorMessage 는 새로운 ErrorMessage 인스턴스를 생성한다.
// msg가 nil이면 ErrNilMessage, err가 nil이면 ErrNilError를 반환한다.
func NewErrorMessage(msg message.Message, err error, sourceNodeID string, opts ...ErrorMessageOption) (ErrorMessage, error) {
	if msg == nil {
		return nil, ErrNilMessage
	}
	if err == nil {
		return nil, ErrNilError
	}

	em := &defaultErrorMessage{
		originalMessage: msg,
		err:             err,
		sourceNodeID:    sourceNodeID,
		errorTimestamp:  time.Now(),
		severity:        SeverityError,
		category:        CategoryProcessing,
		stackContext:    "",
	}

	for _, opt := range opts {
		opt(em)
	}

	return em, nil
}

func (em *defaultErrorMessage) OriginalMessage() message.Message {
	return em.originalMessage
}

func (em *defaultErrorMessage) Error() error {
	return em.err
}

func (em *defaultErrorMessage) SourceNodeID() string {
	return em.sourceNodeID
}

func (em *defaultErrorMessage) ErrorTimestamp() time.Time {
	return em.errorTimestamp
}

func (em *defaultErrorMessage) Severity() ErrorSeverity {
	return em.severity
}

func (em *defaultErrorMessage) Category() ErrorCategory {
	return em.category
}

func (em *defaultErrorMessage) StackContext() string {
	return em.stackContext
}
