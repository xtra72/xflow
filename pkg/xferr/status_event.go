package xferr

import (
	"time"

	"github.com/xtra/xflow/pkg/lifecycle"
)

// StatusEvent 는 컴포넌트의 상태 변경 이벤트를 나타내는 인터페이스이다.
type StatusEvent interface {
	// ComponentType 는 컴포넌트의 유형을 반환한다.
	ComponentType() ComponentType
	// ComponentID 는 컴포넌트의 고유 식별자를 반환한다.
	ComponentID() string
	// PreviousState 는 이전 상태를 반환한다.
	PreviousState() lifecycle.State
	// NewState 는 새로운 상태를 반환한다.
	NewState() lifecycle.State
	// Timestamp 는 상태 변경 시각을 반환한다.
	Timestamp() time.Time
	// Info 는 상태 변경 관련 추가 정보를 반환한다.
	Info() map[string]string
}

// StatusEventOption 은 StatusEvent 생성 시 적용할 옵션 함수 타입이다.
type StatusEventOption func(*defaultStatusEvent)

// WithInfo 는 상태 변경 관련 추가 정보를 설정한다.
// 제공된 맵은 방어적으로 복사된다.
func WithInfo(info map[string]string) StatusEventOption {
	return func(se *defaultStatusEvent) {
		se.info = copyStringMap(info)
	}
}

// defaultStatusEvent 는 StatusEvent 인터페이스의 기본 구현체이다.
type defaultStatusEvent struct {
	componentType ComponentType
	componentID   string
	previousState lifecycle.State
	newState      lifecycle.State
	timestamp     time.Time
	info          map[string]string
}

// NewStatusEvent 는 새로운 StatusEvent 인스턴스를 생성한다.
// prevState와 newState가 동일하면 ErrSameStateTransition,
// compType이 유효하지 않으면 ErrInvalidComponentType을 반환한다.
func NewStatusEvent(compType ComponentType, compID string, prevState, newState lifecycle.State, opts ...StatusEventOption) (StatusEvent, error) {
	if prevState == newState {
		return nil, ErrSameStateTransition
	}
	if !IsValidComponentType(compType) {
		return nil, ErrInvalidComponentType
	}

	se := &defaultStatusEvent{
		componentType: compType,
		componentID:   compID,
		previousState: prevState,
		newState:      newState,
		timestamp:     time.Now(),
		info:          make(map[string]string),
	}

	for _, opt := range opts {
		opt(se)
	}

	return se, nil
}

func (se *defaultStatusEvent) ComponentType() ComponentType {
	return se.componentType
}

func (se *defaultStatusEvent) ComponentID() string {
	return se.componentID
}

func (se *defaultStatusEvent) PreviousState() lifecycle.State {
	return se.previousState
}

func (se *defaultStatusEvent) NewState() lifecycle.State {
	return se.newState
}

func (se *defaultStatusEvent) Timestamp() time.Time {
	return se.timestamp
}

// Info 는 상태 변경 관련 추가 정보를 방어적 복사본으로 반환한다.
func (se *defaultStatusEvent) Info() map[string]string {
	return copyStringMap(se.info)
}
