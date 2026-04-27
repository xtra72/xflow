package lifecycle

import "time"

// StateChangeEvent 는 상태 변경 시 콜백에 전달되는 이벤트 정보이다.
type StateChangeEvent struct {
	// Component 는 상태가 변경된 컴포넌트의 이름이다.
	Component string

	// From 은 변경 전 상태이다.
	From State

	// To 는 변경 후 상태이다.
	To State

	// Timestamp 는 상태 변경이 발생한 시각이다.
	Timestamp time.Time

	// Error 는 상태 변경과 관련된 에러 정보이다 (없으면 nil).
	Error error
}

// StateChangeCallback 은 상태 변경 시 호출되는 콜백 함수 타입이다.
type StateChangeCallback func(event StateChangeEvent)

// UnsubscribeFunc 는 콜백 구독을 해제하는 함수 타입이다.
type UnsubscribeFunc func()
