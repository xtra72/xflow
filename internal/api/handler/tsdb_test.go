package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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

// --- Characterization 테스트 (DDD - PRESERVE) ---
//
// 목적: SPEC-WEB-005 페이지네이션 도입 시 기존 호출부가 영향받지 않음을 보장한다.
// page/size 파라미터가 없는 레거시 호출 경로는 응답 JSON 구조 {"series": [...], "count": N}
// 를 정확히 유지해야 하며, pagination 필드는 JSON 에 **존재하지 않아야** 한다.
// (null 이 아니라 키 자체가 누락되어야 함 → DTO 의 omitempty 로 보장)
// @spec SPEC-WEB-005

// TestTSDBHandler_ListSeries_Characterization 은 SPEC-WEB-005 변경 전/후에 걸쳐
// 기존 ListSeries 응답 포맷이 변하지 않음을 검증하는 characterization 테스트이다.
func TestTSDBHandler_ListSeries_Characterization(t *testing.T) {
	t.Run("레거시 호출: pagination 필드가 JSON 에 부재해야 한다", func(t *testing.T) {
		mock := &mockTSDB{
			seriesKeysFn: func() []string {
				return []string{"cpu,host=a", "mem,host=a"}
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series", nil)
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		// Raw JSON 파싱으로 필드 존재 여부 정확히 검증
		var raw map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))

		data, ok := raw["data"].(map[string]any)
		require.True(t, ok, "data 필드가 객체여야 한다")

		// 기존 필드는 반드시 존재
		_, hasSeries := data["series"]
		_, hasCount := data["count"]
		assert.True(t, hasSeries, "series 필드가 존재해야 한다")
		assert.True(t, hasCount, "count 필드가 존재해야 한다")

		// pagination 필드는 반드시 부재
		_, hasPagination := data["pagination"]
		assert.False(t, hasPagination,
			"레거시 호출에서는 pagination 필드가 JSON 에 부재해야 한다 (omitempty)")
	})

	t.Run("빈 결과: 빈 배열과 count=0, pagination 부재", func(t *testing.T) {
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

		var raw map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
		data := raw["data"].(map[string]any)

		// 빈 배열이어야 하며 null 이면 안 됨
		series, ok := data["series"].([]any)
		require.True(t, ok, "series 는 배열이어야 한다 (null 불가)")
		assert.Empty(t, series)
		assert.EqualValues(t, 0, data["count"])

		_, hasPagination := data["pagination"]
		assert.False(t, hasPagination)
	})

	t.Run("measurement 필터 호출도 기존 포맷 유지", func(t *testing.T) {
		mock := &mockTSDB{
			filterSeriesFn: func(measurement string, tags map[string]string) []string {
				return []string{"cpu,host=a", "cpu,host=b"}
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series?measurement=cpu", nil)
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var raw map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
		data := raw["data"].(map[string]any)

		_, hasPagination := data["pagination"]
		assert.False(t, hasPagination, "measurement 필터만 있는 호출도 pagination 이 부재해야 한다")
	})
}

// --- 페이지네이션 TDD 테스트 (SPEC-WEB-005 Task 4) ---
// @spec SPEC-WEB-005

// makeSeriesKeys 는 N 개의 테스트 시리즈 키를 생성하는 헬퍼이다.
func makeSeriesKeys(n int) []string {
	keys := make([]string, n)
	for i := 0; i < n; i++ {
		// 시리즈 키 포맷: temp,room=001 ... temp,room=NNN
		// i+1 을 써서 1-based 로 직관적 순번 유지
		keys[i] = "temp,room=" + strconv.Itoa(i+1)
	}
	return keys
}

func TestTSDBHandler_ListSeries_Pagination(t *testing.T) {
	t.Run("page=1, size=10 — 첫 10개 반환", func(t *testing.T) {
		mock := &mockTSDB{
			seriesKeysFn: func() []string {
				return makeSeriesKeys(30)
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series?page=1&size=10", nil)
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp dto.APIResponse[dto.TSDBSeriesListResponse]
		decodeTSDBJSON(t, rec, &resp)

		require.True(t, resp.Success)
		assert.Len(t, resp.Data.Series, 10)
		assert.Equal(t, 10, resp.Data.Count)
		require.NotNil(t, resp.Data.Pagination)
		assert.Equal(t, 1, resp.Data.Pagination.Page)
		assert.Equal(t, 10, resp.Data.Pagination.Size)
		assert.EqualValues(t, 30, resp.Data.Pagination.Total)
		assert.Equal(t, 3, resp.Data.Pagination.TotalPages)

		// 첫 페이지는 1번 키부터 시작
		assert.Equal(t, "temp,room=1", resp.Data.Series[0])
		assert.Equal(t, "temp,room=10", resp.Data.Series[9])
	})

	t.Run("page=2, size=10 with total=30 → items 11..20 반환", func(t *testing.T) {
		mock := &mockTSDB{
			seriesKeysFn: func() []string {
				return makeSeriesKeys(30)
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series?page=2&size=10", nil)
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp dto.APIResponse[dto.TSDBSeriesListResponse]
		decodeTSDBJSON(t, rec, &resp)

		require.NotNil(t, resp.Data.Pagination)
		assert.Equal(t, 2, resp.Data.Pagination.Page)
		assert.Len(t, resp.Data.Series, 10)
		assert.Equal(t, "temp,room=11", resp.Data.Series[0])
		assert.Equal(t, "temp,room=20", resp.Data.Series[9])
	})

	t.Run("다양한 size 값 — 테이블 주도 테스트", func(t *testing.T) {
		cases := []struct {
			name         string
			total        int
			size         int
			expectedLen  int
			expectedSize int
		}{
			{"size=10", 50, 10, 10, 10},
			{"size=25", 50, 25, 25, 25},
			{"size=50", 50, 50, 50, 50},
			{"size=100", 200, 100, 100, 100},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				total := tc.total
				mock := &mockTSDB{
					seriesKeysFn: func() []string {
						return makeSeriesKeys(total)
					},
				}
				router := setupTSDBRouter(mock)

				url := "/api/v1/tsdb/series?page=1&size=" + strconv.Itoa(tc.size)
				req := httptest.NewRequest(http.MethodGet, url, nil)
				rec := httptest.NewRecorder()
				router.Handler().ServeHTTP(rec, req)

				assert.Equal(t, http.StatusOK, rec.Code)

				var resp dto.APIResponse[dto.TSDBSeriesListResponse]
				decodeTSDBJSON(t, rec, &resp)

				assert.Len(t, resp.Data.Series, tc.expectedLen)
				require.NotNil(t, resp.Data.Pagination)
				assert.Equal(t, tc.expectedSize, resp.Data.Pagination.Size)
			})
		}
	})

	t.Run("size=101 — 100 으로 캡핑 (Permissive)", func(t *testing.T) {
		mock := &mockTSDB{
			seriesKeysFn: func() []string {
				return makeSeriesKeys(200)
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series?page=1&size=101", nil)
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp dto.APIResponse[dto.TSDBSeriesListResponse]
		decodeTSDBJSON(t, rec, &resp)

		require.NotNil(t, resp.Data.Pagination)
		assert.Equal(t, 100, resp.Data.Pagination.Size, "size 상한 100 으로 클램프되어야 한다")
		assert.Len(t, resp.Data.Series, 100)
	})

	t.Run("size=0 — 기본값 25 로 보정", func(t *testing.T) {
		// 설계 결정: size=0 은 400 이 아니라 기본값(25)으로 보정한다.
		// 사용자가 명시적으로 페이지네이션을 요청(page 파라미터 존재)한 경우에만 해당.
		mock := &mockTSDB{
			seriesKeysFn: func() []string {
				return makeSeriesKeys(100)
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series?page=1&size=0", nil)
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp dto.APIResponse[dto.TSDBSeriesListResponse]
		decodeTSDBJSON(t, rec, &resp)

		require.NotNil(t, resp.Data.Pagination)
		assert.Equal(t, 25, resp.Data.Pagination.Size, "size=0 은 기본값 25 로 보정되어야 한다")
	})

	t.Run("page=0 또는 음수 — 1 로 클램프", func(t *testing.T) {
		mock := &mockTSDB{
			seriesKeysFn: func() []string {
				return makeSeriesKeys(30)
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series?page=0&size=10", nil)
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp dto.APIResponse[dto.TSDBSeriesListResponse]
		decodeTSDBJSON(t, rec, &resp)

		require.NotNil(t, resp.Data.Pagination)
		assert.Equal(t, 1, resp.Data.Pagination.Page, "page=0 은 1 로 클램프되어야 한다")
	})

	t.Run("페이지 초과 — 빈 시리즈, pagination.page 는 요청값 반영", func(t *testing.T) {
		mock := &mockTSDB{
			seriesKeysFn: func() []string {
				return makeSeriesKeys(10)
			},
		}
		router := setupTSDBRouter(mock)

		// 10 개 밖에 없는데 page=5, size=10 요청 → 오프셋이 범위 밖
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series?page=5&size=10", nil)
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp dto.APIResponse[dto.TSDBSeriesListResponse]
		decodeTSDBJSON(t, rec, &resp)

		assert.Empty(t, resp.Data.Series, "오프셋을 초과하면 빈 배열")
		assert.Equal(t, 0, resp.Data.Count)
		require.NotNil(t, resp.Data.Pagination)
		assert.Equal(t, 5, resp.Data.Pagination.Page, "요청 페이지 번호를 그대로 에코")
		assert.EqualValues(t, 10, resp.Data.Pagination.Total)
		assert.Equal(t, 1, resp.Data.Pagination.TotalPages)
	})

	t.Run("빈 결과 + 페이지네이션 — total=0, total_pages=0", func(t *testing.T) {
		mock := &mockTSDB{
			seriesKeysFn: func() []string {
				return []string{}
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series?page=1&size=25", nil)
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp dto.APIResponse[dto.TSDBSeriesListResponse]
		decodeTSDBJSON(t, rec, &resp)

		assert.NotNil(t, resp.Data.Series)
		assert.Empty(t, resp.Data.Series)
		assert.Equal(t, 0, resp.Data.Count)
		require.NotNil(t, resp.Data.Pagination)
		assert.Equal(t, 1, resp.Data.Pagination.Page)
		assert.Equal(t, 25, resp.Data.Pagination.Size)
		assert.EqualValues(t, 0, resp.Data.Pagination.Total)
		assert.Equal(t, 0, resp.Data.Pagination.TotalPages, "total=0 이면 total_pages 도 0")
	})

	t.Run("비숫자 page — 400 Bad Request", func(t *testing.T) {
		mock := &mockTSDB{
			seriesKeysFn: func() []string {
				return makeSeriesKeys(10)
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series?page=abc&size=10", nil)
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code,
			"비숫자 page 파라미터는 400 을 반환해야 한다")
	})

	t.Run("비숫자 size — 400 Bad Request", func(t *testing.T) {
		mock := &mockTSDB{
			seriesKeysFn: func() []string {
				return makeSeriesKeys(10)
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series?page=1&size=xyz", nil)
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code,
			"비숫자 size 파라미터는 400 을 반환해야 한다")
	})

	t.Run("agent_id 파라미터 — 파싱만 하고 싱글톤으로 라우팅", func(t *testing.T) {
		// 현재 구현은 agent_id 를 무시하고 기본 TSDB 인스턴스로 라우팅한다.
		// 파라미터가 존재해도 에러를 내지 않아야 한다 (향후 멀티 인스턴스 확장 대비).
		mock := &mockTSDB{
			seriesKeysFn: func() []string {
				return []string{"temp,room=1"}
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/tsdb/series?page=1&size=25&agent_id=tsdb-primary", nil)
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp dto.APIResponse[dto.TSDBSeriesListResponse]
		decodeTSDBJSON(t, rec, &resp)
		assert.True(t, resp.Success)
		assert.Len(t, resp.Data.Series, 1)
	})

	t.Run("size 만 있어도 페이지네이션 활성화 (page 기본값 1)", func(t *testing.T) {
		mock := &mockTSDB{
			seriesKeysFn: func() []string {
				return makeSeriesKeys(30)
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series?size=10", nil)
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp dto.APIResponse[dto.TSDBSeriesListResponse]
		decodeTSDBJSON(t, rec, &resp)

		require.NotNil(t, resp.Data.Pagination, "size 만 있어도 pagination 이 포함되어야 한다")
		assert.Equal(t, 1, resp.Data.Pagination.Page)
		assert.Equal(t, 10, resp.Data.Pagination.Size)
	})

	t.Run("page 만 있어도 페이지네이션 활성화 (size 기본값 25)", func(t *testing.T) {
		mock := &mockTSDB{
			seriesKeysFn: func() []string {
				return makeSeriesKeys(100)
			},
		}
		router := setupTSDBRouter(mock)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tsdb/series?page=2", nil)
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp dto.APIResponse[dto.TSDBSeriesListResponse]
		decodeTSDBJSON(t, rec, &resp)

		require.NotNil(t, resp.Data.Pagination)
		assert.Equal(t, 2, resp.Data.Pagination.Page)
		assert.Equal(t, 25, resp.Data.Pagination.Size, "size 미지정 시 기본값 25")
	})
}
