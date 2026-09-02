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

	// ErrKeyExists 는 Rename 대상 키가 이미 존재해 덮어쓰기가 거부될 때 반환된다.
	ErrKeyExists = errors.New("store: destination key already exists")

	// @spec SPEC-STORE-003
	// ErrKeyNotAllowed 는 registration_type=manual 모드에서
	// 정적 키 목록에 없는 키를 쓰려고 할 때 반환된다.
	ErrKeyNotAllowed = errors.New("store: key not allowed (registration_type=manual and key not in static keys)")

	// @spec SPEC-STORE-003
	// ErrDuplicateStaticKey 는 설정 로드 시 `keys` 목록에 동일한 key 가
	// 두 번 이상 정의되어 있을 때 반환된다.
	ErrDuplicateStaticKey = errors.New("store: duplicate static key definition")

	// @spec SPEC-STORE-003
	// ErrInvalidTagKey 는 태그의 key 가 허용 패턴(^[a-zA-Z0-9_-]+$)을 위반할 때 반환된다.
	ErrInvalidTagKey = errors.New("store: invalid tag key (must match ^[a-zA-Z0-9_-]+$)")

	// @spec SPEC-STORE-003 v0.3.0
	// ErrTypeMismatch 는 쓰기 값의 Go 타입이 등록된 data_type 과 일치하지 않을 때 반환된다.
	ErrTypeMismatch = errors.New("store: value type does not match registered data_type")

	// @spec SPEC-STORE-003 v0.3.0
	// ErrUnsupportedValueType 은 nil 또는 추론 불가능한 타입(채널, 함수 등)이 auto 모드에서 쓰여질 때 반환된다.
	ErrUnsupportedValueType = errors.New("store: unsupported value type for auto data_type inference")

	// @spec SPEC-STORE-003 v0.3.0
	// ErrInvalidDataType 은 yaml 의 data_type 값이 6종 enum (int, float, string, boolean, bytes, json) 외이거나, manual 모드에서 누락되었을 때 반환된다.
	ErrInvalidDataType = errors.New("store: invalid or missing data_type (must be one of: int, float, string, boolean, bytes, json)")

	// @spec SPEC-STORE-003 v0.3.0
	// ErrInvalidField 은 yaml 의 field 이 정규식 ^[a-zA-Z0-9_-]+$ 를 위반할 때 반환된다.
	ErrInvalidField = errors.New("store: invalid field (must match ^[a-zA-Z0-9_-]+$)")

	// @spec SPEC-STORE-004
	// ErrInvalidSeriesKey 는 인코딩된 시리즈 키를 DecodeSeriesKey 로 역직렬화할 때
	// 인코딩 프레임 구분자(`|`)나 태그 KV 구분자(`=`)가 부족하여 분해에 실패한 경우 반환된다.
	ErrInvalidSeriesKey = errors.New("store: invalid encoded series key")
)
