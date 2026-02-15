package xferr

import "errors"

// 센티널 에러 변수 정의
var (
	// ErrNilMessage 는 nil 메시지가 전달되었을 때 반환된다.
	ErrNilMessage = errors.New("xferr: nil message")

	// ErrNilError 는 nil 에러가 전달되었을 때 반환된다.
	ErrNilError = errors.New("xferr: nil error")

	// ErrNoReceiver 는 등록된 수신자가 없을 때 반환된다.
	ErrNoReceiver = errors.New("xferr: no receiver registered")

	// ErrAlertThresholdExceeded 는 경고 임계값이 초과되었을 때 반환된다.
	ErrAlertThresholdExceeded = errors.New("xferr: alert threshold exceeded")

	// ErrInvalidSeverity 는 유효하지 않은 심각도가 전달되었을 때 반환된다.
	ErrInvalidSeverity = errors.New("xferr: invalid severity")

	// ErrInvalidCategory 는 유효하지 않은 카테고리가 전달되었을 때 반환된다.
	ErrInvalidCategory = errors.New("xferr: invalid category")

	// ErrInvalidDropReason 는 유효하지 않은 폐기 사유가 전달되었을 때 반환된다.
	ErrInvalidDropReason = errors.New("xferr: invalid drop reason")

	// ErrInvalidComponentType 는 유효하지 않은 컴포넌트 타입이 전달되었을 때 반환된다.
	ErrInvalidComponentType = errors.New("xferr: invalid component type")

	// ErrSameStateTransition 은 동일한 상태로의 전이가 시도되었을 때 반환된다.
	ErrSameStateTransition = errors.New("xferr: same state transition")
)
