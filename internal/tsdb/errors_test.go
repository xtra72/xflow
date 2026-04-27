package tsdb

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{
			name: "ErrSeriesNotFound 메시지 확인",
			err:  ErrSeriesNotFound,
			msg:  "tsdb: 시리즈를 찾을 수 없음",
		},
		{
			name: "ErrInvalidMeasurement 메시지 확인",
			err:  ErrInvalidMeasurement,
			msg:  "tsdb: measurement 이름이 비어있음",
		},
		{
			name: "ErrInvalidField 메시지 확인",
			err:  ErrInvalidField,
			msg:  "tsdb: 필드 맵이 비어있거나 nil",
		},
		{
			name: "ErrQueryTimeout 메시지 확인",
			err:  ErrQueryTimeout,
			msg:  "tsdb: 쿼리 타임아웃",
		},
		{
			name: "ErrMaxPointsExceeded 메시지 확인",
			err:  ErrMaxPointsExceeded,
			msg:  "tsdb: 최대 반환 포인트 수 초과",
		},
		{
			name: "ErrTSDBClosed 메시지 확인",
			err:  ErrTSDBClosed,
			msg:  "tsdb: TSDB가 이미 닫힘",
		},
		{
			name: "ErrMaxSeriesExceeded 메시지 확인",
			err:  ErrMaxSeriesExceeded,
			msg:  "tsdb: 최대 시리즈 수 초과",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.msg, tt.err.Error())
		})
	}
}

func TestErrorsIs(t *testing.T) {
	// errors.Is로 센티널 에러 매칭 확인
	tests := []struct {
		name   string
		err    error
		target error
	}{
		{"ErrSeriesNotFound 매칭", ErrSeriesNotFound, ErrSeriesNotFound},
		{"ErrInvalidMeasurement 매칭", ErrInvalidMeasurement, ErrInvalidMeasurement},
		{"ErrInvalidField 매칭", ErrInvalidField, ErrInvalidField},
		{"ErrQueryTimeout 매칭", ErrQueryTimeout, ErrQueryTimeout},
		{"ErrMaxPointsExceeded 매칭", ErrMaxPointsExceeded, ErrMaxPointsExceeded},
		{"ErrTSDBClosed 매칭", ErrTSDBClosed, ErrTSDBClosed},
		{"ErrMaxSeriesExceeded 매칭", ErrMaxSeriesExceeded, ErrMaxSeriesExceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, errors.Is(tt.err, tt.target))
		})
	}
}

func TestWrappedErrorsIs(t *testing.T) {
	// fmt.Errorf로 래핑된 에러도 errors.Is로 매칭되는지 확인
	wrapped := fmt.Errorf("추가 컨텍스트: %w", ErrSeriesNotFound)
	assert.True(t, errors.Is(wrapped, ErrSeriesNotFound))
	assert.False(t, errors.Is(wrapped, ErrTSDBClosed))
}

func TestErrorsAreDistinct(t *testing.T) {
	// 모든 에러가 서로 다른지 확인
	allErrors := []error{
		ErrSeriesNotFound,
		ErrInvalidMeasurement,
		ErrInvalidField,
		ErrQueryTimeout,
		ErrMaxPointsExceeded,
		ErrTSDBClosed,
		ErrMaxSeriesExceeded,
	}

	for i := 0; i < len(allErrors); i++ {
		for j := i + 1; j < len(allErrors); j++ {
			assert.False(t, errors.Is(allErrors[i], allErrors[j]),
				"에러 %v 와 %v 는 서로 달라야 함", allErrors[i], allErrors[j])
		}
	}
}
