package xferr

import (
	"time"

	"github.com/xtra/xflow/pkg/message"
)

// DeadLetterMessage 는 처리 불가능하여 폐기된 메시지를 나타내는 인터페이스이다.
type DeadLetterMessage interface {
	// OriginalMessage 는 폐기된 원본 메시지를 반환한다.
	OriginalMessage() message.Message
	// Reason 는 메시지가 폐기된 사유를 반환한다.
	Reason() DropReason
	// SourceNodeID 는 메시지를 폐기한 노드의 ID를 반환한다.
	SourceNodeID() string
	// SourceWireID 는 메시지를 폐기한 와이어의 ID를 반환한다.
	SourceWireID() string
	// DropTimestamp 는 메시지가 폐기된 시각을 반환한다.
	DropTimestamp() time.Time
	// Context 는 폐기 관련 추가 컨텍스트 정보를 반환한다.
	Context() map[string]string
}

// DeadLetterOption 은 DeadLetterMessage 생성 시 적용할 옵션 함수 타입이다.
type DeadLetterOption func(*defaultDeadLetterMessage)

// WithDLSourceNodeID 는 데드레터 메시지의 소스 노드 ID를 설정한다.
func WithDLSourceNodeID(id string) DeadLetterOption {
	return func(dlm *defaultDeadLetterMessage) {
		dlm.sourceNodeID = id
	}
}

// WithDLSourceWireID 는 데드레터 메시지의 소스 와이어 ID를 설정한다.
func WithDLSourceWireID(id string) DeadLetterOption {
	return func(dlm *defaultDeadLetterMessage) {
		dlm.sourceWireID = id
	}
}

// WithDLContext 는 데드레터 메시지의 추가 컨텍스트를 설정한다.
// 제공된 맵은 방어적으로 복사된다.
func WithDLContext(ctx map[string]string) DeadLetterOption {
	return func(dlm *defaultDeadLetterMessage) {
		dlm.context = copyStringMap(ctx)
	}
}

// defaultDeadLetterMessage 는 DeadLetterMessage 인터페이스의 기본 구현체이다.
type defaultDeadLetterMessage struct {
	originalMessage message.Message
	reason          DropReason
	sourceNodeID    string
	sourceWireID    string
	dropTimestamp   time.Time
	context         map[string]string
}

// NewDeadLetterMessage 는 새로운 DeadLetterMessage 인스턴스를 생성한다.
// msg가 nil이면 ErrNilMessage, reason이 유효하지 않으면 ErrInvalidDropReason을 반환한다.
func NewDeadLetterMessage(msg message.Message, reason DropReason, opts ...DeadLetterOption) (DeadLetterMessage, error) {
	if msg == nil {
		return nil, ErrNilMessage
	}
	if !IsValidDropReason(reason) {
		return nil, ErrInvalidDropReason
	}

	dlm := &defaultDeadLetterMessage{
		originalMessage: msg,
		reason:          reason,
		sourceNodeID:    "",
		sourceWireID:    "",
		dropTimestamp:   time.Now(),
		context:         make(map[string]string),
	}

	for _, opt := range opts {
		opt(dlm)
	}

	return dlm, nil
}

func (dlm *defaultDeadLetterMessage) OriginalMessage() message.Message {
	return dlm.originalMessage
}

func (dlm *defaultDeadLetterMessage) Reason() DropReason {
	return dlm.reason
}

func (dlm *defaultDeadLetterMessage) SourceNodeID() string {
	return dlm.sourceNodeID
}

func (dlm *defaultDeadLetterMessage) SourceWireID() string {
	return dlm.sourceWireID
}

func (dlm *defaultDeadLetterMessage) DropTimestamp() time.Time {
	return dlm.dropTimestamp
}

// Context 는 폐기 관련 추가 컨텍스트를 방어적 복사본으로 반환한다.
func (dlm *defaultDeadLetterMessage) Context() map[string]string {
	return copyStringMap(dlm.context)
}

// copyStringMap 은 문자열 맵의 방어적 복사본을 생성한다.
func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
