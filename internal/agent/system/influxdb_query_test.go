package system

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ExecuteFluxQuery / ExecuteInfluxQLQuery 는 SPEC-CHART-001 M3 에서 추가된
// 쿼리 API 이다. 이 파일은 RED 단계의 실패하는 테스트를 정의한다.

// TestInfluxDBAgent_ExecuteFluxQuery_단일행_성공 는 Flux 쿼리가 단일 행 결과를
// 반환할 때 Time/Value/Labels 로 올바르게 매핑되는지 검증한다.
func TestInfluxDBAgent_ExecuteFluxQuery_단일행_성공(t *testing.T) {
	t.Parallel()
	expectedTime := time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC)
	mock := &mockInfluxClient{
		queryFunc: func(_ context.Context, query, lang string) ([]map[string]any, error) {
			assert.Equal(t, "flux", lang)
			assert.Contains(t, query, "range")
			return []map[string]any{
				{
					"_time":        expectedTime,
					"_value":       42.5,
					"_measurement": "temperature",
					"_field":       "value",
					"room":         "A",
				},
			}, nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	results, err := a.ExecuteFluxQuery(context.Background(), `from(bucket:"x") |> range(start:-1h)`)
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.Equal(t, expectedTime, r.Time)
	assert.Equal(t, 42.5, r.Value)
	// _time 과 _value 는 Labels 에 포함되지 않는다. 나머지(_measurement/_field/tag)는 포함된다.
	assert.Equal(t, "A", r.Labels["room"])
	assert.Equal(t, "temperature", r.Labels["_measurement"])
	assert.Equal(t, "value", r.Labels["_field"])
	_, hasTime := r.Labels["_time"]
	assert.False(t, hasTime, "_time should not be in labels")
	_, hasValue := r.Labels["_value"]
	assert.False(t, hasValue, "_value should not be in labels")
}

// TestInfluxDBAgent_ExecuteFluxQuery_다중행_순서보존 는 여러 행 결과의 순서가
// 그대로 보존되는지 확인한다.
func TestInfluxDBAgent_ExecuteFluxQuery_다중행_순서보존(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Minute)
	t3 := t1.Add(2 * time.Minute)
	mock := &mockInfluxClient{
		queryFunc: func(_ context.Context, _, _ string) ([]map[string]any, error) {
			return []map[string]any{
				{"_time": t1, "_value": 1.0, "host": "a"},
				{"_time": t2, "_value": 2.0, "host": "b"},
				{"_time": t3, "_value": 3.0, "host": "c"},
			}, nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	results, err := a.ExecuteFluxQuery(context.Background(), "from(bucket:\"x\")")
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, t1, results[0].Time)
	assert.Equal(t, 1.0, results[0].Value)
	assert.Equal(t, "a", results[0].Labels["host"])
	assert.Equal(t, t2, results[1].Time)
	assert.Equal(t, "b", results[1].Labels["host"])
	assert.Equal(t, t3, results[2].Time)
	assert.Equal(t, "c", results[2].Labels["host"])
}

// TestInfluxDBAgent_ExecuteFluxQuery_빈결과 는 빈 결과가 올바르게 처리되는지
// 확인한다 (nil 이 아닌 빈 슬라이스).
func TestInfluxDBAgent_ExecuteFluxQuery_빈결과(t *testing.T) {
	t.Parallel()
	mock := &mockInfluxClient{
		queryFunc: func(_ context.Context, _, _ string) ([]map[string]any, error) {
			return []map[string]any{}, nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	results, err := a.ExecuteFluxQuery(context.Background(), "from(bucket:\"x\")")
	require.NoError(t, err)
	assert.NotNil(t, results)
	assert.Len(t, results, 0)
}

// TestInfluxDBAgent_ExecuteFluxQuery_에러전파 는 client 에러가 래핑되어
// 반환되는지 확인한다.
func TestInfluxDBAgent_ExecuteFluxQuery_에러전파(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("bucket not found")
	mock := &mockInfluxClient{
		queryFunc: func(_ context.Context, _, _ string) ([]map[string]any, error) {
			return nil, sentinel
		},
	}
	a := newTestInfluxDBAgent(mock)

	_, err := a.ExecuteFluxQuery(context.Background(), "bad")
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// TestInfluxDBAgent_ExecuteFluxQuery_시간없음_허용 은 _time 필드가 없는 경우
// Time 이 zero value 로 남고 모든 필드가 Labels 에 포함되는지 확인한다.
func TestInfluxDBAgent_ExecuteFluxQuery_시간없음_허용(t *testing.T) {
	t.Parallel()
	mock := &mockInfluxClient{
		queryFunc: func(_ context.Context, _, _ string) ([]map[string]any, error) {
			return []map[string]any{
				{"_value": 100, "host": "a"},
			}, nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	results, err := a.ExecuteFluxQuery(context.Background(), "from(bucket:\"x\")")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Time.IsZero())
	assert.Equal(t, 100, results[0].Value)
	assert.Equal(t, "a", results[0].Labels["host"])
}

// TestInfluxDBAgent_ExecuteInfluxQLQuery_lang_지정 은 InfluxQL 메서드가
// client.Query 에 "influxql" 언어를 전달하는지 확인한다.
func TestInfluxDBAgent_ExecuteInfluxQLQuery_lang_지정(t *testing.T) {
	t.Parallel()
	var capturedLang string
	mock := &mockInfluxClient{
		queryFunc: func(_ context.Context, _ string, lang string) ([]map[string]any, error) {
			capturedLang = lang
			return []map[string]any{{"_time": time.Now(), "_value": 1}}, nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	_, err := a.ExecuteInfluxQLQuery(context.Background(), "SELECT * FROM x")
	require.NoError(t, err)
	assert.Equal(t, "influxql", capturedLang)
}

// TestInfluxDBAgent_ExecuteQuery_라벨은_문자열화 는 _time, _value 를 제외한
// 모든 값이 문자열로 변환되어 Labels 에 들어가는지 확인한다 (숫자 tag 포함).
func TestInfluxDBAgent_ExecuteQuery_라벨은_문자열화(t *testing.T) {
	t.Parallel()
	mock := &mockInfluxClient{
		queryFunc: func(_ context.Context, _, _ string) ([]map[string]any, error) {
			return []map[string]any{
				{"_time": time.Now(), "_value": 1.0, "priority": 3, "active": true, "name": "x"},
			}, nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	results, err := a.ExecuteFluxQuery(context.Background(), "q")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "3", results[0].Labels["priority"])
	assert.Equal(t, "true", results[0].Labels["active"])
	assert.Equal(t, "x", results[0].Labels["name"])
}
