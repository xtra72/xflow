package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/fillpolicy"
)

// `previous` 채우기의 사용 기간 제한 파싱.
func TestParseSeriesFillPreviousLimit(t *testing.T) {
	t.Run("previous 가 아니면 제로값 — 다른 전략에는 뜻이 없다", func(t *testing.T) {
		got, err := parseSeriesFillPreviousLimit(influxSeriesQueryRequest{
			Fill:              dto.SeriesFillZero,
			FillPreviousMaxMs: 60_000,
		})
		require.NoError(t, err)
		assert.Equal(t, fillpolicy.Previous{}, got)
	})

	t.Run("미지정이면 무제한 — 종전 동작", func(t *testing.T) {
		got, err := parseSeriesFillPreviousLimit(influxSeriesQueryRequest{
			Fill: dto.SeriesFillPrevious,
		})
		require.NoError(t, err)
		assert.True(t, got.Unlimited())
	})

	t.Run("기간과 비움", func(t *testing.T) {
		got, err := parseSeriesFillPreviousLimit(influxSeriesQueryRequest{
			Fill:              dto.SeriesFillPrevious,
			FillPreviousMaxMs: 300_000,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(300_000), got.MaxMs)
		assert.Equal(t, fillpolicy.OverflowEmpty, got.Overflow)
	})

	t.Run("기간과 지정 값", func(t *testing.T) {
		got, err := parseSeriesFillPreviousLimit(influxSeriesQueryRequest{
			Fill:                      dto.SeriesFillPrevious,
			FillPreviousMaxMs:         300_000,
			FillPreviousOverflow:      dto.SeriesFillPreviousOverflowValue,
			FillPreviousOverflowValue: -1,
		})
		require.NoError(t, err)
		assert.Equal(t, fillpolicy.OverflowValue, got.Overflow)
		assert.Equal(t, -1.0, got.Value)
	})

	t.Run("음수 기간은 거부한다 — 0(무제한)과 뜻이 겹친다", func(t *testing.T) {
		_, err := parseSeriesFillPreviousLimit(influxSeriesQueryRequest{
			Fill:              dto.SeriesFillPrevious,
			FillPreviousMaxMs: -1,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fill_previous_max_ms")
	})

	t.Run("모르는 초과 처리는 거부한다 — 조용히 비우면 오답이다", func(t *testing.T) {
		_, err := parseSeriesFillPreviousLimit(influxSeriesQueryRequest{
			Fill:                 dto.SeriesFillPrevious,
			FillPreviousOverflow: "carry",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fill_previous_overflow")
	})
}

// spec 까지 실려 가는지 — 파싱만 맞고 전달이 빠지면 조용히 무제한이 된다.
func TestBuildSeriesQuerySpec_직전값_기간이_spec_에_실린다(t *testing.T) {
	spec, err := buildSeriesQuerySpec(influxSeriesQueryRequest{
		Measurement:               "cpu",
		Field:                     "usage",
		StartMs:                   0,
		EndMs:                     3_600_000,
		IntervalMs:                60_000,
		Aggregation:               dto.SeriesAggregationAverage,
		Fill:                      dto.SeriesFillPrevious,
		FillPreviousMaxMs:         300_000,
		FillPreviousOverflow:      dto.SeriesFillPreviousOverflowValue,
		FillPreviousOverflowValue: 0,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(300_000), spec.FillPreviousLimit.MaxMs)
	assert.Equal(t, fillpolicy.OverflowValue, spec.FillPreviousLimit.Overflow)
}
