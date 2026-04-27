package tsdb

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMemoryAtomicOperations(t *testing.T) {
	// 각 테스트 전 메모리 리셋
	ResetMemoryUsage()

	t.Run("초기값은 0", func(t *testing.T) {
		ResetMemoryUsage()
		assert.Equal(t, int64(0), CurrentMemoryUsage())
	})

	t.Run("AddMemory", func(t *testing.T) {
		ResetMemoryUsage()
		AddMemory(100)
		assert.Equal(t, int64(100), CurrentMemoryUsage())

		AddMemory(50)
		assert.Equal(t, int64(150), CurrentMemoryUsage())
	})

	t.Run("SubMemory", func(t *testing.T) {
		ResetMemoryUsage()
		AddMemory(200)
		SubMemory(50)
		assert.Equal(t, int64(150), CurrentMemoryUsage())
	})

	t.Run("ResetMemoryUsage", func(t *testing.T) {
		ResetMemoryUsage()
		AddMemory(999)
		ResetMemoryUsage()
		assert.Equal(t, int64(0), CurrentMemoryUsage())
	})
}

func TestMemory_ConcurrentAccess(t *testing.T) {
	// 동시 접근 안전성 테스트
	ResetMemoryUsage()

	var wg sync.WaitGroup
	goroutines := 100
	perGoroutine := int64(10)

	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			AddMemory(perGoroutine)
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(goroutines)*perGoroutine, CurrentMemoryUsage())
}

func TestEstimatePointSize(t *testing.T) {
	tests := []struct {
		name     string
		dp       DataPoint
		expected int64
	}{
		{
			name: "단일 float64 필드",
			dp: DataPoint{
				Timestamp: time.Now(),
				Fields:    map[string]any{"v": float64(1.0)},
			},
			// DataPoint.EstimateSize()와 동일한 결과여야 함
			expected: 24 + 8 + 16 + 1 + 8, // 57
		},
		{
			name: "빈 필드",
			dp: DataPoint{
				Timestamp: time.Now(),
				Fields:    map[string]any{},
			},
			expected: 32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimatePointSize(tt.dp)
			assert.Equal(t, tt.expected, got)
			// DataPoint.EstimateSize()와 동일한 결과인지 확인
			assert.Equal(t, tt.dp.EstimateSize(), got)
		})
	}
}
