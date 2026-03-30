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
}

// defaultConfig 는 기본 설정 값을 반환한다.
func defaultConfig() storeConfig {
	return storeConfig{
		backend:        "volatile",
		defaultTTL:     0,
		scanInterval:   1 * time.Second,
		maxKeyLength:   MaxKeyLength,
		maxHistorySize: 0,
		historyTTL:     0,
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
