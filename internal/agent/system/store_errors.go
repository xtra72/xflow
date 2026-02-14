package system

import "errors"

// Store 에러 정의 - 저장소 연산에서 발생할 수 있는 센티넬 에러들이다.
var (
	// ErrKeyNotFound 는 요청한 키가 저장소에 존재하지 않을 때 반환된다.
	ErrKeyNotFound = errors.New("store: key not found")

	// ErrNamespaceNotAllowed 는 허용되지 않은 네임스페이스에 접근할 때 반환된다.
	ErrNamespaceNotAllowed = errors.New("store: namespace not allowed")

	// ErrStoreClosed 는 닫힌 저장소에 연산을 시도할 때 반환된다.
	ErrStoreClosed = errors.New("store: store is closed")

	// ErrStorePaused 는 일시정지된 저장소에 쓰기 연산을 시도할 때 반환된다.
	ErrStorePaused = errors.New("store: store is paused (write operations disabled)")

	// ErrNilValue 는 nil 값을 저장하려 할 때 반환된다.
	ErrNilValue = errors.New("store: nil value not allowed")

	// ErrInvalidTTL 은 유효하지 않은 TTL(음수 등)을 지정했을 때 반환된다.
	ErrInvalidTTL = errors.New("store: invalid TTL (must be >= 0)")

	// ErrNotSerializable 는 직렬화할 수 없는 값을 저장하려 할 때 반환된다.
	ErrNotSerializable = errors.New("store: value is not serializable")

	// ErrKeyTooLong 은 키 길이가 최대 허용 길이를 초과할 때 반환된다.
	ErrKeyTooLong = errors.New("store: key exceeds maximum length")
)
