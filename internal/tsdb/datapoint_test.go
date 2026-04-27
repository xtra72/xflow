package tsdb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataPoint_EstimateSize(t *testing.T) {
	tests := []struct {
		name     string
		dp       DataPoint
		expected int64
	}{
		{
			name: "빈 필드 맵",
			dp: DataPoint{
				Timestamp: time.Now(),
				Fields:    map[string]any{},
			},
			// 24 (timestamp) + 8 (map overhead)
			expected: 32,
		},
		{
			name: "float64 필드 하나",
			dp: DataPoint{
				Timestamp: time.Now(),
				Fields:    map[string]any{"temp": float64(25.5)},
			},
			// 24 + 8 + (16 + 4 + 8) = 60
			expected: 60,
		},
		{
			name: "int64 필드 하나",
			dp: DataPoint{
				Timestamp: time.Now(),
				Fields:    map[string]any{"count": int64(100)},
			},
			// 24 + 8 + (16 + 5 + 8) = 61
			expected: 61,
		},
		{
			name: "bool 필드 하나",
			dp: DataPoint{
				Timestamp: time.Now(),
				Fields:    map[string]any{"active": true},
			},
			// 24 + 8 + (16 + 6 + 1) = 55
			expected: 55,
		},
		{
			name: "string 필드 하나",
			dp: DataPoint{
				Timestamp: time.Now(),
				Fields:    map[string]any{"status": "running"},
			},
			// 24 + 8 + (16 + 6 + 7) = 61
			expected: 61,
		},
		{
			name: "복합 필드",
			dp: DataPoint{
				Timestamp: time.Now(),
				Fields: map[string]any{
					"temp":   float64(25.5),  // 16 + 4 + 8 = 28
					"count":  int64(100),     // 16 + 5 + 8 = 29
					"active": true,           // 16 + 6 + 1 = 23
					"status": "ok",           // 16 + 6 + 2 = 24
				},
			},
			// 24 + 8 + 28 + 29 + 23 + 24 = 136
			expected: 136,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dp.EstimateSize()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestDataPoint_EstimateSize_UnknownType(t *testing.T) {
	// 알 수 없는 타입은 8바이트로 추정
	dp := DataPoint{
		Timestamp: time.Now(),
		Fields:    map[string]any{"data": []byte{1, 2, 3}},
	}
	size := dp.EstimateSize()
	// 24 + 8 + (16 + 4 + 8) = 60
	assert.Equal(t, int64(60), size)
}

func TestDataPoint_Validate(t *testing.T) {
	tests := []struct {
		name    string
		dp      DataPoint
		wantErr error
	}{
		{
			name: "유효한 데이터포인트",
			dp: DataPoint{
				Timestamp: time.Now(),
				Fields:    map[string]any{"temp": float64(25.5)},
			},
			wantErr: nil,
		},
		{
			name: "nil 필드",
			dp: DataPoint{
				Timestamp: time.Now(),
				Fields:    nil,
			},
			wantErr: ErrInvalidField,
		},
		{
			name: "빈 필드 맵",
			dp: DataPoint{
				Timestamp: time.Now(),
				Fields:    map[string]any{},
			},
			wantErr: ErrInvalidField,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.dp.Validate()
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
