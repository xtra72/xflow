package tsdb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, 10000, cfg.MaxSeries, "MaxSeries 기본값")
	assert.Equal(t, 100000, cfg.MaxPointsPerSeries, "MaxPointsPerSeries 기본값")
	assert.Equal(t, int64(256*1024*1024), cfg.MaxMemoryBytes, "MaxMemoryBytes 기본값 (256MB)")
	assert.Equal(t, 24*time.Hour, cfg.MaxAge, "MaxAge 기본값")
	assert.Equal(t, 30*time.Second, cfg.EvictionInterval, "EvictionInterval 기본값")
	assert.Equal(t, 10000, cfg.MaxQueryPoints, "MaxQueryPoints 기본값")
	assert.Equal(t, 10*time.Second, cfg.QueryTimeout, "QueryTimeout 기본값")
}

func TestConfig_CustomValues(t *testing.T) {
	// 사용자 정의 값으로 Config 생성 확인
	cfg := Config{
		MaxSeries:          500,
		MaxPointsPerSeries: 1000,
		MaxMemoryBytes:     64 * 1024 * 1024,
		MaxAge:             1 * time.Hour,
		EvictionInterval:   10 * time.Second,
		MaxQueryPoints:     100,
		QueryTimeout:       5 * time.Second,
	}

	assert.Equal(t, 500, cfg.MaxSeries)
	assert.Equal(t, 1000, cfg.MaxPointsPerSeries)
	assert.Equal(t, int64(64*1024*1024), cfg.MaxMemoryBytes)
	assert.Equal(t, 1*time.Hour, cfg.MaxAge)
	assert.Equal(t, 10*time.Second, cfg.EvictionInterval)
	assert.Equal(t, 100, cfg.MaxQueryPoints)
	assert.Equal(t, 5*time.Second, cfg.QueryTimeout)
}
