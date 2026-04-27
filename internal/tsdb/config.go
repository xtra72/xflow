package tsdb

import "time"

// Config 는 TSDB의 설정을 정의한다.
type Config struct {
	MaxSeries          int           // 최대 시리즈 수 (기본: 10000)
	MaxPointsPerSeries int           // 시리즈당 최대 포인트 수 (기본: 100000)
	MaxMemoryBytes     int64         // 최대 메모리 사용량 (기본: 256MB)
	MaxAge             time.Duration // 데이터 최대 보존 기간 (기본: 24h)
	EvictionInterval   time.Duration // 퇴거 검사 주기 (기본: 30s)
	MaxQueryPoints     int           // 쿼리 최대 반환 포인트 수 (기본: 10000)
	QueryTimeout       time.Duration // 쿼리 타임아웃 (기본: 10s)
}

// DefaultConfig 는 기본 설정 값을 반환한다.
func DefaultConfig() Config {
	return Config{
		MaxSeries:          10000,
		MaxPointsPerSeries: 100000,
		MaxMemoryBytes:     256 * 1024 * 1024, // 256MB
		MaxAge:             24 * time.Hour,
		EvictionInterval:   30 * time.Second,
		MaxQueryPoints:     10000,
		QueryTimeout:       10 * time.Second,
	}
}
