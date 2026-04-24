package system

import "time"

// StoreOption 은 StoreAgent 생성 시 적용할 수 있는 옵션 함수 타입이다.
type StoreOption func(*storeConfig)

// storeConfig 는 StoreAgent의 내부 설정을 담는 구조체이다.
type storeConfig struct {
	backend        string          // "volatile" 또는 "persistent"
	defaultTTL     time.Duration   // 기본 TTL (0이면 만료 없음)
	scanInterval   time.Duration   // TTL 스캔 주기 (기본 1초)
	repository     StoreRepository // 영속 저장소 (persistent 백엔드 시)
	maxKeyLength   int             // 최대 키 길이 (기본 512)
	maxHistorySize int             // 값 변경 히스토리 최대 보관 수 (0이면 비활성)
	historyTTL     time.Duration   // 히스토리 항목 최대 보관 시간 (0이면 무제한)

	// @spec SPEC-STORE-003
	// allowDynamicKeys 가 false 이면 staticKeys 에 없는 키 쓰기 요청이 거부된다.
	// 기본값은 true (기존 동작 보존: 모든 키 허용).
	allowDynamicKeys bool
	// @spec SPEC-STORE-003
	// staticKeys 는 정적으로 선언된 사용자 키 → 태그(map[string]string) 매핑이다.
	// 키는 네임스페이스 접두사를 포함하지 않은 '사용자 관점' 키이다.
	// nil 또는 빈 맵이면 정적 키 정의가 없는 것으로 간주된다.
	staticKeys map[string]map[string]string
}

// defaultConfig 는 기본 설정 값을 반환한다.
func defaultConfig() storeConfig {
	return storeConfig{
		backend:          "volatile",
		defaultTTL:       0,
		scanInterval:     1 * time.Second,
		maxKeyLength:     MaxKeyLength,
		maxHistorySize:   0,
		historyTTL:       0,
		allowDynamicKeys: true, // @spec SPEC-STORE-003: 기본값 true (하위호환).
		staticKeys:       nil,
	}
}

// WithBackend 는 저장소 백엔드를 설정하는 옵션을 반환한다.
// "volatile" 또는 "persistent" 를 지정할 수 있다.
func WithBackend(backend string) StoreOption {
	return func(c *storeConfig) {
		c.backend = backend
	}
}

// WithDefaultTTL 은 기본 TTL을 설정하는 옵션을 반환한다.
// 0이면 만료 없음을 의미한다.
func WithDefaultTTL(ttl time.Duration) StoreOption {
	return func(c *storeConfig) {
		c.defaultTTL = ttl
	}
}

// WithScanInterval 은 TTL 만료 스캔 주기를 설정하는 옵션을 반환한다.
func WithScanInterval(interval time.Duration) StoreOption {
	return func(c *storeConfig) {
		c.scanInterval = interval
	}
}

// WithRepository 는 영속 저장소 백엔드를 설정하는 옵션을 반환한다.
func WithRepository(repo StoreRepository) StoreOption {
	return func(c *storeConfig) {
		c.repository = repo
	}
}

// WithMaxKeyLength 는 최대 키 길이를 설정하는 옵션을 반환한다.
func WithMaxKeyLength(length int) StoreOption {
	return func(c *storeConfig) {
		c.maxKeyLength = length
	}
}

// WithMaxHistorySize 는 값 변경 히스토리의 최대 보관 수를 설정하는 옵션을 반환한다.
// 0이면 히스토리를 기록하지 않는다 (기본값).
func WithMaxHistorySize(n int) StoreOption {
	return func(c *storeConfig) {
		c.maxHistorySize = n
	}
}

// WithHistoryTTL 은 히스토리 항목의 최대 보관 시간을 설정하는 옵션을 반환한다.
// 0이면 시간 기반 트리밍을 하지 않는다 (기본값).
func WithHistoryTTL(d time.Duration) StoreOption {
	return func(c *storeConfig) {
		c.historyTTL = d
	}
}

// WithAllowDynamicKeys 는 동적 키 허용 여부를 설정하는 옵션을 반환한다.
// true(기본)이면 정적 키 목록에 없는 키도 자유롭게 쓸 수 있다.
// false(strict 모드)이면 정적 키 목록에 없는 키 쓰기가 ErrKeyNotAllowed 로 거부된다.
//
// @spec SPEC-STORE-003
func WithAllowDynamicKeys(allow bool) StoreOption {
	return func(c *storeConfig) {
		c.allowDynamicKeys = allow
	}
}

// WithStaticKeys 는 정적 키 → 태그 매핑을 설정하는 옵션을 반환한다.
// keys 는 사용자 관점 키(네임스페이스 접두사 제외)를 기준으로 한다.
// nil 이거나 빈 맵이면 정적 키 정의가 없는 상태가 된다.
//
// @spec SPEC-STORE-003
func WithStaticKeys(keys map[string]map[string]string) StoreOption {
	return func(c *storeConfig) {
		c.staticKeys = keys
	}
}
