package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/ws"
	"github.com/xtra/xflow/internal/tsdb"
)

// --- Mock TSDB ---

type mockTSDB struct {
	writeBatchFn   func(points []tsdb.WriteRequest) error
	executeFn      func(q tsdb.Query) ([]tsdb.QueryResult, error)
	seriesKeysFn   func() []string
	filterSeriesFn func(measurement string, tags map[string]string) []string
	latestFn       func(key string, n int) ([]tsdb.DataPoint, error)
	statsFn        func() tsdb.Stats
	deleteSeriesFn func(key string) error
	subscribeFn    func(keys []string, ch chan tsdb.DataPoint) func()
}

func (m *mockTSDB) Write(measurement string, tags map[string]string, fields map[string]any) error {
	return nil
}

func (m *mockTSDB) WriteBatch(points []tsdb.WriteRequest) error {
	if m.writeBatchFn != nil {
		return m.writeBatchFn(points)
	}
	return nil
}

func (m *mockTSDB) QueryRange(seriesKey string, start, end time.Time) ([]tsdb.DataPoint, error) {
	return nil, nil
}

func (m *mockTSDB) Latest(key string, n int) ([]tsdb.DataPoint, error) {
	if m.latestFn != nil {
		return m.latestFn(key, n)
	}
	return nil, nil
}

func (m *mockTSDB) Execute(q tsdb.Query) ([]tsdb.QueryResult, error) {
	if m.executeFn != nil {
		return m.executeFn(q)
	}
	return nil, nil
}

func (m *mockTSDB) FilterSeries(measurement string, tags map[string]string) []string {
	if m.filterSeriesFn != nil {
		return m.filterSeriesFn(measurement, tags)
	}
	return nil
}

func (m *mockTSDB) SeriesKeys() []string {
	if m.seriesKeysFn != nil {
		return m.seriesKeysFn()
	}
	return nil
}

func (m *mockTSDB) DeleteSeries(key string) error {
	if m.deleteSeriesFn != nil {
		return m.deleteSeriesFn(key)
	}
	return nil
}

func (m *mockTSDB) Stats() tsdb.Stats {
	if m.statsFn != nil {
		return m.statsFn()
	}
	return tsdb.Stats{}
}

func (m *mockTSDB) Subscribe(keys []string, ch chan tsdb.DataPoint) func() {
	if m.subscribeFn != nil {
		return m.subscribeFn(keys, ch)
	}
	return func() {}
}

func (m *mockTSDB) Close() error {
	return nil
}

// --- 테스트 헬퍼 ---

func setupTSDBRouter(mock *mockTSDB) *api.Router {
	router := api.NewRouter()
	h := NewTSDBHandler(mock, nil)
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router
}

func decodeTSDBJSON(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	err := json.NewDecoder(rec.Body).Decode(v)
	require.NoError(t, err)
}

// --- RegisterRoutes 테스트 ---

func TestTSDBHandler_RegisterRoutes(t *testing.T) {
	mock := &mockTSDB{}
	router := api.NewRouter()
	h := NewTSDBHandler(mock, nil)
	g := router.Group("/api/v1")

	before := router.RouteCount()
	h.RegisterRoutes(g)
	after := router.RouteCount()

	// 6개의 라우트가 등록되어야 한다
	assert.Equal(t, 6, after-before)
}

// --- Write 테스트 ---

func TestTSDBHandler_Write(t *testing.T) {
	t.Run("유효한 쓰기", func(t *testing.T) {
		var calledPoints []tsdb.WriteRequest
		mock := &mockTSDB{
			writeBatchFn: func(points []tsdb.WriteRequest) error {
				calledPoints = points
				return nil
			},
		}
		router := setupTSDBRouter(mock)

		body := `{"points":[{"measurement":"cpu","tags":{"host":"server1"},"fields":{"usage":85.5}}]}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tsdb/write", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)
		require.Len(t, calledPoints, 1)
		assert.Equal(t, "cpu", calledPoints[0].Measurement)
		assert.Equal(t, "server1", calledPoints[0].Tags["host"])

		var resp dto.APIResponse[dto.TSDBWriteResponse]
		decodeTSDBJSON(t, rec, &resp)
		assert.True(t, resp.Success)
		assert.Equal(t, 1, resp.Data.Written)
	})

	t.Run("타임스탬프 포함 쓰기", func(t *testing.T) {
		var calledPoints []tsdb.WriteRequest
		mock := &mockTSDB{
			writeBatchFn: func(points []tsdb.WriteRequest) error {
				calledPoints = points
				return nil
			},
		}
		router := setupTSDBRouter(mock)

		ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC).Format(time.RFC3339Nano)
		body := `{"points":[{"measurement":"temp","fields":{"value":22.5},"timestamp":"` + ts + `"}]}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tsdb/write", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)
		require.Len(t, calledPoints, 1)
		assert.False(t, calledPoints[0].Timestamp.IsZero())
	})

	t.Run("빈 포인트 배열", func(t *testing.T) {
		mock := &mockTSDB{}
		router := setupTSDBRouter(mock)

		body := `{"points":[]}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tsdb/write", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("measurement 누락", func(t *testing.T) {
		mock := &mockTSDB{}
		router := setupTSDBRouter(mock)

		body := `{"points":[{"fields":{"value":1}}]}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tsdb/write", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("fields 누락", func(t *testing.T) {
		mock := &mockTSDB{}
		router := setupTSDBRouter(mock)

		body := `{"points":[{"measurement":"cpu"}]}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tsdb/write", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// --- Query 테스트 ---

func TestTSDBHandler_Query(t *testing.T) {
	now := time.Now().UTC()

	t.Run("집계 쿼리", func(t *testing.T) {
		mock := &mockTSDB{
			executeFn: func(q tsdb.Query) ([]tsdb.QueryResult, error) {
				assert.Equal(t, tsdb.AggAvg, q.Aggregation)
				assert.Equal(t, "temperature", q.Field)
				return []tsdb.QueryResult{
					{
						SeriesKey: "temp,room=living",
						Points: []tsdb.DataPoint{
							{Timestamp: now, Fields: map[string]any{"temperature": 23.5}},
						},
						Stats: tsdb.QueryStats{
							ScannedPoints:  100,
							ReturnedPoints: 1,
							ExecutionTime:  5 * time.Millisecond,
						},
					},
				}, nil
			},
		}
		router := setupTSDBRouter(mock)

		body := `{"measurement":"temp","aggregation":"avg","field":"temperature"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tsdb/query", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp dto.APIResponse[dto.TSDBQueryResponse]
		decodeTSDBJSON(t, rec, &resp)
		assert.True(t, resp.Success)
		require.Len(t, resp.Data.Results, 1)
		assert.Equal(t, "temp,room=living", resp.Data.Results[0].SeriesKey)
		require.Len(t, resp.Data.Results[0].Points, 1)
		assert.Equal(t, 100, resp.Data.Results[0].Stats.ScannedPoints)
	})

	t.Run("시리즈 키 쿼리", func(t *testing.T) {
		mock := &mockTSDB{
			executeFn: func(q tsdb.Query) ([]tsdb.QueryResult, error) {
				assert.Equal(t, "cpu,host=a", q.SeriesKey)
				return []tsdb.QueryResult{
					{
						SeriesKey: "cpu,host=a",
						Points: []tsdb.DataPoint{
							{Timestamp: now, Fields: map[string]any{"usage": 80.0}},
						},
						Stats: tsdb.QueryStats{ScannedPoints: 10, ReturnedPoints: 1},
					},
				}, nil
			},
		}
		router := setupTSDBRouter(mock)

		body := `{"series_key":"cpu,host=a"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tsdb/query", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("시간 범위 쿼리", func(t *testing.T) {
		startStr := now.Add(-time.Hour).Format(time.RFC3339Nano)
		endStr := now.Format(time.RFC3339Nano)
		mock := &mockTSDB{
			executeFn: func(q tsdb.Query) ([]tsdb.QueryResult, error) {
				assert.False(t, q.Start.IsZero())
				assert.False(t, q.End.IsZero())
				return nil, nil
			},
		}
		router := setupTSDBRouter(mock)

		body := `{"series_key":"cpu","start":"` + startStr + `","end":"` + endStr + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tsdb/query", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

// --- ListSeries 테스트 ---

func TestTSDBHandler_ListSeries(t *testing.T) {
	t.Run("필터 없음", func(t *testing.T) {
		mock := &mockTSDB{
			seriesKeysFn: func() []string {
				return []string{"cpu,host=a", "mem,host=a", "temp,room=living"}
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series", nil)
		rec := httptest.NewRecorder()

		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp dto.APIResponse[dto.TSDBSeriesListResponse]
		decodeTSDBJSON(t, rec, &resp)
		assert.True(t, resp.Success)
		assert.Equal(t, 3, resp.Data.Count)
		assert.Len(t, resp.Data.Series, 3)
	})

	t.Run("measurement 필터", func(t *testing.T) {
		mock := &mockTSDB{
			filterSeriesFn: func(measurement string, tags map[string]string) []string {
				assert.Equal(t, "cpu", measurement)
				return []string{"cpu,host=a", "cpu,host=b"}
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series?measurement=cpu", nil)
		rec := httptest.NewRecorder()

		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp dto.APIResponse[dto.TSDBSeriesListResponse]
		decodeTSDBJSON(t, rec, &resp)
		assert.True(t, resp.Success)
		assert.Equal(t, 2, resp.Data.Count)
	})

	t.Run("시리즈 없음", func(t *testing.T) {
		mock := &mockTSDB{
			seriesKeysFn: func() []string {
				return nil
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series", nil)
		rec := httptest.NewRecorder()

		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp dto.APIResponse[dto.TSDBSeriesListResponse]
		decodeTSDBJSON(t, rec, &resp)
		assert.True(t, resp.Success)
		assert.Equal(t, 0, resp.Data.Count)
		assert.NotNil(t, resp.Data.Series) // nil이 아닌 빈 배열이어야 함
	})
}

// --- Latest 테스트 ---

func TestTSDBHandler_Latest(t *testing.T) {
	now := time.Now().UTC()

	t.Run("기본 n=1", func(t *testing.T) {
		mock := &mockTSDB{
			latestFn: func(key string, n int) ([]tsdb.DataPoint, error) {
				assert.Equal(t, "cpu,host=a", key)
				assert.Equal(t, 1, n)
				return []tsdb.DataPoint{
					{Timestamp: now, Fields: map[string]any{"usage": 85.0}},
				}, nil
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series/cpu%2Chost%3Da/latest", nil)
		rec := httptest.NewRecorder()

		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp dto.APIResponse[[]dto.TSDBDataPoint]
		decodeTSDBJSON(t, rec, &resp)
		assert.True(t, resp.Success)
		require.Len(t, resp.Data, 1)
	})

	t.Run("커스텀 n=5", func(t *testing.T) {
		mock := &mockTSDB{
			latestFn: func(key string, n int) ([]tsdb.DataPoint, error) {
				assert.Equal(t, 5, n)
				points := make([]tsdb.DataPoint, 5)
				for i := range points {
					points[i] = tsdb.DataPoint{
						Timestamp: now.Add(time.Duration(-i) * time.Minute),
						Fields:    map[string]any{"usage": float64(80 + i)},
					}
				}
				return points, nil
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series/cpu%2Chost%3Da/latest?n=5", nil)
		rec := httptest.NewRecorder()

		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp dto.APIResponse[[]dto.TSDBDataPoint]
		decodeTSDBJSON(t, rec, &resp)
		assert.True(t, resp.Success)
		assert.Len(t, resp.Data, 5)
	})

	t.Run("시리즈를 찾을 수 없음", func(t *testing.T) {
		mock := &mockTSDB{
			latestFn: func(key string, n int) ([]tsdb.DataPoint, error) {
				return nil, tsdb.ErrSeriesNotFound
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series/nonexistent/latest", nil)
		rec := httptest.NewRecorder()

		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

// --- Stats 테스트 ---

func TestTSDBHandler_Stats(t *testing.T) {
	oldest := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newest := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	mock := &mockTSDB{
		statsFn: func() tsdb.Stats {
			return tsdb.Stats{
				SeriesCount: 42,
				TotalPoints: 100000,
				MemoryBytes: 268435456, // 256 MB
				OldestPoint: oldest,
				NewestPoint: newest,
			}
		},
	}
	router := setupTSDBRouter(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/stats", nil)
	rec := httptest.NewRecorder()

	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp dto.APIResponse[dto.TSDBStatsResponse]
	decodeTSDBJSON(t, rec, &resp)
	assert.True(t, resp.Success)
	assert.Equal(t, 42, resp.Data.SeriesCount)
	assert.Equal(t, int64(100000), resp.Data.TotalPoints)
	assert.Equal(t, int64(268435456), resp.Data.MemoryBytes)
	assert.Equal(t, "256.0 MB", resp.Data.MemoryHuman)
	assert.NotEmpty(t, resp.Data.OldestPoint)
	assert.NotEmpty(t, resp.Data.NewestPoint)
}

// --- DeleteSeries 테스트 ---

func TestTSDBHandler_DeleteSeries(t *testing.T) {
	t.Run("성공", func(t *testing.T) {
		var deletedKey string
		mock := &mockTSDB{
			deleteSeriesFn: func(key string) error {
				deletedKey = key
				return nil
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/tsdb/series/cpu%2Chost%3Da", nil)
		rec := httptest.NewRecorder()

		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Equal(t, "cpu,host=a", deletedKey)
	})

	t.Run("시리즈를 찾을 수 없음", func(t *testing.T) {
		mock := &mockTSDB{
			deleteSeriesFn: func(key string) error {
				return tsdb.ErrSeriesNotFound
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/tsdb/series/nonexistent", nil)
		rec := httptest.NewRecorder()

		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

// --- NewTSDBHandler 테스트 ---

func TestNewTSDBHandler(t *testing.T) {
	t.Run("기본 생성", func(t *testing.T) {
		mock := &mockTSDB{}
		h := NewTSDBHandler(mock, nil)
		assert.NotNil(t, h)
		assert.NotNil(t, h.logger)
		assert.Nil(t, h.hub)
	})

	t.Run("WithHub 옵션", func(t *testing.T) {
		mock := &mockTSDB{}
		hub := &ws.Hub{}
		h := NewTSDBHandler(mock, nil, WithHub(hub))
		assert.NotNil(t, h.hub)
	})
}

// --- formatBytes 테스트 ---

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1048576, "1.0 MB"},
		{268435456, "256.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, formatBytes(tt.bytes))
		})
	}
}

// --- parseTimestamp 테스트 ---

func TestParseTimestamp(t *testing.T) {
	t.Run("RFC3339Nano", func(t *testing.T) {
		ts := "2024-01-15T10:30:00.123456789Z"
		result, err := parseTimestamp(ts)
		require.NoError(t, err)
		assert.Equal(t, 2024, result.Year())
		assert.Equal(t, time.January, result.Month())
	})

	t.Run("unix 나노초", func(t *testing.T) {
		ts := "1705312200000000000" // 2024-01-15T10:30:00Z
		result, err := parseTimestamp(ts)
		require.NoError(t, err)
		assert.False(t, result.IsZero())
	})

	t.Run("잘못된 형식", func(t *testing.T) {
		_, err := parseTimestamp("not-a-timestamp")
		assert.Error(t, err)
	})
}
