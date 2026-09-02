package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/tsdb"
)

// 내장 TSDB 조회(/tsdb/query)의 버킷 수 상한 방어.
//
// Store(InfluxDB) 경로에는 이미 사전 검증이 있었지만 내장 TSDB 경로에는 없어,
// 과대 요청이 구간 전체를 스캔·복사한 뒤에야 거부됐다. 여기서는 그 거부가
// Execute 에 닿기 *전에* 일어나는지를 본다 — executeFn 이 호출되면 실패다.
func TestTSDBHandler_Query_BucketCountGuard(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// 요청 본문을 만든다. bucket 은 Go duration 문법이다.
	body := func(end time.Time, bucket string) string {
		return fmt.Sprintf(
			`{"series_key":"temp,room=living","start":%q,"end":%q,"bucket":%q,"aggregation":"avg","field":"temperature"}`,
			start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano), bucket)
	}

	post := func(t *testing.T, mock *mockTSDB, reqBody string) *httptest.ResponseRecorder {
		t.Helper()
		router := setupTSDBRouter(mock)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tsdb/query", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)
		return rec
	}

	t.Run("상한 초과는 스캔 전에 400 으로 거부된다", func(t *testing.T) {
		var executed bool
		mock := &mockTSDB{
			executeFn: func(tsdb.Query) ([]tsdb.QueryResult, error) {
				executed = true
				return nil, nil
			},
		}

		// 1초 버킷 × (상한+1) 구간.
		end := start.Add(time.Duration(maxAggregationBuckets+1) * time.Second)
		rec := post(t, mock, body(end, "1s"))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.False(t, executed, "Execute 가 호출되면 스캔 비용을 이미 치른 것이다")

		msg := rec.Body.String()
		assert.Contains(t, msg, fmt.Sprintf("bucket_count=%d", maxAggregationBuckets+1))
		assert.Contains(t, msg, fmt.Sprintf("max=%d", maxAggregationBuckets))
	})

	t.Run("상한 경계는 통과한다", func(t *testing.T) {
		var executed bool
		mock := &mockTSDB{
			executeFn: func(tsdb.Query) ([]tsdb.QueryResult, error) {
				executed = true
				return nil, nil
			},
		}

		end := start.Add(time.Duration(maxAggregationBuckets) * time.Second)
		rec := post(t, mock, body(end, "1s"))

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.True(t, executed)
	})

	t.Run("나머지 버킷은 올림으로 센다", func(t *testing.T) {
		mock := &mockTSDB{}

		// 상한만큼의 온전한 버킷 + 1ms → 올림하면 상한+1.
		end := start.Add(time.Duration(maxAggregationBuckets)*time.Second + time.Millisecond)
		rec := post(t, mock, body(end, "1s"))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), fmt.Sprintf("bucket_count=%d", maxAggregationBuckets+1))
	})

	t.Run("버킷 간격이 없으면(원본 반환) 검사하지 않는다", func(t *testing.T) {
		var executed bool
		mock := &mockTSDB{
			executeFn: func(tsdb.Query) ([]tsdb.QueryResult, error) {
				executed = true
				return nil, nil
			},
		}

		end := start.Add(time.Duration(maxAggregationBuckets+1) * time.Second)
		reqBody := fmt.Sprintf(
			`{"series_key":"temp,room=living","start":%q,"end":%q}`,
			start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano))
		rec := post(t, mock, reqBody)

		// 버킷 수를 셀 수 없는 요청이므로 통과시킨다(방어는 스캔 상한 소관).
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.True(t, executed)
	})

	t.Run("구간이 열려 있으면 검사하지 않는다", func(t *testing.T) {
		var executed bool
		mock := &mockTSDB{
			executeFn: func(tsdb.Query) ([]tsdb.QueryResult, error) {
				executed = true
				return nil, nil
			},
		}

		// start 만 주고 end 를 생략 — 구간 길이를 알 수 없다.
		reqBody := fmt.Sprintf(
			`{"series_key":"temp,room=living","start":%q,"bucket":"1s","aggregation":"avg","field":"temperature"}`,
			start.Format(time.RFC3339Nano))
		rec := post(t, mock, reqBody)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.True(t, executed)
	})
}

// 상한 상수가 Store 경로와 공유되는지 — 값이 복제되면 한쪽만 조정되어 어긋난다.
func TestTSDBBucketGuard_SharesStoreLimit(t *testing.T) {
	q := tsdb.Query{
		Start:          time.Unix(0, 0),
		End:            time.Unix(0, 0).Add(time.Duration(maxAggregationBuckets) * time.Second),
		BucketInterval: time.Second,
	}
	require.NoError(t, validateTSDBBucketCount(q))

	q.End = q.End.Add(time.Second)
	require.Error(t, validateTSDBBucketCount(q))
}
