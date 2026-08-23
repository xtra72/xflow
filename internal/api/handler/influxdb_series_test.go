// @spec SPEC-TSDB-002 §2.6 (U6) · §2.8 (U8) · §2.9 (U9)
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/api"
)

// fakeInfluxSeriesAgent 는 influxSeriesQueryer 를 구현하는 테스트용 페이크이다.
type fakeInfluxSeriesAgent struct {
	*fakeAgentCommon
	seriesFn func(ctx context.Context, spec system.SeriesQuerySpec) ([]system.SeriesBucket, error)
	// captured 는 마지막으로 전달받은 spec 이다.
	captured system.SeriesQuerySpec
}

func (f *fakeInfluxSeriesAgent) QuerySeriesBuckets(
	ctx context.Context, spec system.SeriesQuerySpec,
) ([]system.SeriesBucket, error) {
	f.captured = spec
	if f.seriesFn != nil {
		return f.seriesFn(ctx, spec)
	}
	return nil, nil
}

func setupInfluxSeriesRouter(t *testing.T, agents ...agent.Agent) *api.Router {
	t.Helper()
	router := api.NewRouter()
	h := NewInfluxDBSeriesHandler(&fakeAgentLookup{agents: agents}, nil)
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router
}

// postSeriesQuery 는 구조화 질의 요청을 보내고 recorder 를 돌려준다.
func postSeriesQuery(t *testing.T, router *api.Router, agentName, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/influxdb/"+agentName+"/series/query",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	return rec
}

// validSeriesBody 는 기본 유효 요청이다. 개별 테스트가 필드를 덮어쓴다.
func validSeriesBody(overrides map[string]any) string {
	body := map[string]any{
		"bucket":      "metrics",
		"measurement": "cpu",
		"field":       "usage",
		"tags":        map[string]string{"host": "a"},
		"start_ms":    1_700_000_000_000,
		"end_ms":      1_700_003_600_000,
		"interval_ms": 60_000,
		"aggregation": "average",
	}
	for k, v := range overrides {
		if v == nil {
			delete(body, k)
			continue
		}
		body[k] = v
	}
	b, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestInfluxDBSeriesHandler_RegisterRoutes(t *testing.T) {
	t.Parallel()
	router := api.NewRouter()
	h := NewInfluxDBSeriesHandler(&fakeAgentLookup{}, nil)
	g := router.Group("/api/v1")
	before := router.RouteCount()
	h.RegisterRoutes(g)
	assert.Equal(t, 1, router.RouteCount()-before)
}

// --- AC-18: 라벨 규약 + 빈 결과 ---

func TestInfluxSeriesQuery_EntryLabelsUseFieldReservedKey(t *testing.T) {
	t.Parallel()
	agentFake := &fakeInfluxSeriesAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		seriesFn: func(_ context.Context, _ system.SeriesQuerySpec) ([]system.SeriesBucket, error) {
			return []system.SeriesBucket{
				{StartMs: 1_700_000_000_000, Value: 12.5},
				{StartMs: 1_700_000_060_000, Value: 13.5},
			}, nil
		},
	}
	router := setupInfluxSeriesRouter(t, agentFake)

	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(map[string]any{
		"tags": map[string]string{"host": "a", "region": "kr"},
	}))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 2)
	assert.Equal(t, 2, resp.Data.Count)
	for _, e := range resp.Data.Entries {
		// 예약 라벨은 요청의 field 값이며, 나머지 키는 요청 태그 그대로다.
		assert.Equal(t, "usage", e.Labels["__field__"])
		assert.Equal(t, "a", e.Labels["host"])
		assert.Equal(t, "kr", e.Labels["region"])
		assert.Len(t, e.Labels, 3)
		// measurement 는 요청 축에 있으므로 라벨에 넣지 않는다(§4.3).
		assert.NotContains(t, e.Labels, "_measurement")
	}
}

func TestInfluxSeriesQuery_EmptyResultReturnsEmptyEntries(t *testing.T) {
	t.Parallel()
	agentFake := &fakeInfluxSeriesAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		seriesFn: func(_ context.Context, _ system.SeriesQuerySpec) ([]system.SeriesBucket, error) {
			return nil, nil
		},
	}
	router := setupInfluxSeriesRouter(t, agentFake)

	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(nil))
	// 빈 결과는 오류가 아니다 — 빈 차트로 표시되어야 한다(§2.14).
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	// decodeQueryResponse 가 버퍼를 소진하므로 원문을 먼저 읽는다.
	assert.Contains(t, rec.Body.String(), `"entries":[]`, "null 이 아니라 빈 배열이어야 한다")

	resp := decodeQueryResponse(t, rec)
	assert.Empty(t, resp.Data.Entries)
	assert.Equal(t, 0, resp.Data.Count)
	assert.False(t, resp.Data.Truncated)
}

// --- AC-26: 엔트리 타임스탬프가 버킷 시작 시각이다 ---

func TestInfluxSeriesEntry_TimestampIsBucketStart(t *testing.T) {
	t.Parallel()
	const interval = int64(60_000)
	// 요청 start_ms 는 인터벌 경계와 어긋나 있다. 버킷은 사용자 시작 시각이 아니라
	// epoch 0 기준 벽시계 경계에 정렬되므로 첫 버킷 시작은 그보다 앞이다(§2.8).
	start := system.SeriesBucketStartMs(1_700_000_000_000, interval)
	agentFake := &fakeInfluxSeriesAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		seriesFn: func(_ context.Context, spec system.SeriesQuerySpec) ([]system.SeriesBucket, error) {
			return []system.SeriesBucket{
				{StartMs: system.SeriesBucketStartMs(start+13_456, spec.IntervalMs), Value: 1.0},
				{StartMs: system.SeriesBucketStartMs(start+interval+59_999, spec.IntervalMs), Value: 2.0},
			}, nil
		},
	}
	router := setupInfluxSeriesRouter(t, agentFake)

	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(nil))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 2)
	// 버킷 끝이었다면 각각 한 인터벌만큼 뒤로 밀렸을 것이다.
	assert.Equal(t, start, resp.Data.Entries[0].Timestamp)
	assert.Equal(t, start+interval, resp.Data.Entries[1].Timestamp)
	for _, e := range resp.Data.Entries {
		assert.Zero(t, e.Timestamp%interval, "버킷 시작은 인터벌 배수여야 한다")
	}
}

// --- AC-19: 요청 검증 7 종 ---

func TestInfluxSeriesQuery_Validation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		overrides map[string]any
		wantMsg   string
	}{
		{
			name:      "measurement 빈 문자열",
			overrides: map[string]any{"measurement": ""},
			wantMsg:   "measurement is required",
		},
		{
			name:      "field 빈 문자열",
			overrides: map[string]any{"field": ""},
			wantMsg:   "field is required",
		},
		{
			name:      "interval_ms <= 0",
			overrides: map[string]any{"interval_ms": 0},
			// encoding/json 이 '>' 를 \u003e 로 이스케이프하므로 그 앞까지만 대조한다.
			wantMsg: "interval_ms must be",
		},
		{
			name:      "end_ms <= start_ms",
			overrides: map[string]any{"end_ms": 1_700_000_000_000},
			wantMsg:   "end_ms must be greater than start_ms",
		},
		{
			name:      "aggregation 미지의 값",
			overrides: map[string]any{"aggregation": "median"},
			wantMsg:   "invalid aggregation",
		},
		{
			name:      "fill avg 는 대응물이 없다",
			overrides: map[string]any{"fill": "avg"},
			wantMsg:   "invalid fill",
		},
		{
			name:      "이스케이프 불가 식별자",
			overrides: map[string]any{"measurement": "cpu\nDROP"},
			wantMsg:   "cannot be safely escaped",
		},
	}
	require.Len(t, cases, 7, "§2.6 검증 표의 400 조건 7 종 전수")

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agentFake := &fakeInfluxSeriesAgent{
				fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
				seriesFn: func(_ context.Context, _ system.SeriesQuerySpec) ([]system.SeriesBucket, error) {
					t.Fatal("검증 실패 요청이 에이전트까지 도달해서는 안 된다")
					return nil, nil
				},
			}
			router := setupInfluxSeriesRouter(t, agentFake)

			rec := postSeriesQuery(t, router, "metrics", validSeriesBody(tc.overrides))
			require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
			assert.Contains(t, rec.Body.String(), tc.wantMsg)
		})
	}
}

func TestInfluxSeriesQuery_ValidFillValuesAreAccepted(t *testing.T) {
	t.Parallel()
	for _, fill := range []string{"", "null", "zero", "previous"} {
		t.Run("fill="+fill, func(t *testing.T) {
			agentFake := &fakeInfluxSeriesAgent{
				fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
			}
			router := setupInfluxSeriesRouter(t, agentFake)
			rec := postSeriesQuery(t, router, "metrics", validSeriesBody(map[string]any{"fill": fill}))
			assert.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
		})
	}
}

func TestInfluxSeriesQuery_AggregationVocabularyIsMappedToDomainEnum(t *testing.T) {
	t.Parallel()
	cases := map[string]system.SeriesAggregation{
		"min":     system.SeriesAggMin,
		"max":     system.SeriesAggMax,
		"average": system.SeriesAggAverage,
		"first":   system.SeriesAggFirst,
		"last":    system.SeriesAggLast,
	}
	for wire, want := range cases {
		t.Run(wire, func(t *testing.T) {
			agentFake := &fakeInfluxSeriesAgent{
				fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
			}
			router := setupInfluxSeriesRouter(t, agentFake)
			rec := postSeriesQuery(t, router, "metrics", validSeriesBody(map[string]any{"aggregation": wire}))
			require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
			assert.Equal(t, want, agentFake.captured.Aggregation)
		})
	}
}

// --- AC-20: 에이전트 오류 매핑 ---

func TestInfluxSeriesQuery_AgentNotFound(t *testing.T) {
	t.Parallel()
	router := setupInfluxSeriesRouter(t)
	rec := postSeriesQuery(t, router, "missing", validSeriesBody(nil))
	require.Equal(t, http.StatusNotFound, rec.Code, "body=%s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), "agent_not_found")
}

func TestInfluxSeriesQuery_NotAnInfluxAgent(t *testing.T) {
	t.Parallel()
	// QuerySeriesBuckets 를 갖지 않는 에이전트 — 능력이 아니라 계약으로 판정한다.
	router := setupInfluxSeriesRouter(t, newFakeAgent("s1", "metrics", "store"))
	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(nil))
	require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), "not_an_influxdb_agent")
}

func TestInfluxSeriesQuery_Timeout(t *testing.T) {
	t.Parallel()
	agentFake := &fakeInfluxSeriesAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		seriesFn: func(_ context.Context, _ system.SeriesQuerySpec) ([]system.SeriesBucket, error) {
			return nil, fmt.Errorf("influxdb flux series query: %w", context.DeadlineExceeded)
		},
	}
	router := setupInfluxSeriesRouter(t, agentFake)
	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(nil))
	assert.Equal(t, http.StatusRequestTimeout, rec.Code, "body=%s", rec.Body.String())
}

func TestInfluxSeriesQuery_UpstreamFailureIsNotSilentlyEmpty(t *testing.T) {
	t.Parallel()
	agentFake := &fakeInfluxSeriesAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		seriesFn: func(_ context.Context, _ system.SeriesQuerySpec) ([]system.SeriesBucket, error) {
			return nil, errors.New("upstream exploded")
		},
	}
	router := setupInfluxSeriesRouter(t, agentFake)
	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(nil))
	// 빈 결과와 전체 실패를 같은 글리프로 표시하면 단선을 정상으로 오독한다(§2.14).
	assert.Equal(t, http.StatusInternalServerError, rec.Code, "body=%s", rec.Body.String())
}

// --- AC-28: 버킷 수 가드 (서버) ---

func TestInfluxSeriesQuery_BucketLimit(t *testing.T) {
	t.Parallel()
	const interval = int64(1_000)
	start := int64(1_700_000_000_000)
	// 정확히 상한 + 1 개의 버킷을 만든다.
	end := start + interval*int64(maxAggregationBuckets+1)

	agentFake := &fakeInfluxSeriesAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		seriesFn: func(_ context.Context, _ system.SeriesQuerySpec) ([]system.SeriesBucket, error) {
			t.Fatal("상한 초과 요청이 에이전트까지 도달해서는 안 된다")
			return nil, nil
		},
	}
	router := setupInfluxSeriesRouter(t, agentFake)

	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(map[string]any{
		"start_ms":    start,
		"end_ms":      end,
		"interval_ms": interval,
	}))
	require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())

	body := rec.Body.String()
	// 초과한 축 · 실제 값 · 상한이 모두 있어야 사용자가 시간창을 줄일지 인터벌을
	// 늘릴지 판단할 수 있다(§2.9).
	assert.Contains(t, body, "too many buckets")
	assert.Contains(t, body, fmt.Sprintf("bucket_count=%d", maxAggregationBuckets+1))
	assert.Contains(t, body, fmt.Sprintf("max=%d", maxAggregationBuckets))
	assert.Contains(t, body, fmt.Sprintf("interval_ms=%d", interval))
}

func TestInfluxSeriesQuery_BucketLimit_AtBoundaryIsAccepted(t *testing.T) {
	t.Parallel()
	const interval = int64(1_000)
	start := int64(1_700_000_000_000)
	end := start + interval*int64(maxAggregationBuckets)

	agentFake := &fakeInfluxSeriesAgent{fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb")}
	router := setupInfluxSeriesRouter(t, agentFake)

	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(map[string]any{
		"start_ms":    start,
		"end_ms":      end,
		"interval_ms": interval,
	}))
	assert.Equal(t, http.StatusOK, rec.Code, "정확히 상한이면 통과해야 한다: body=%s", rec.Body.String())
}

// --- 요청 → spec 전달 ---

func TestInfluxSeriesQuery_SpecCarriesRequestAxesVerbatim(t *testing.T) {
	t.Parallel()
	agentFake := &fakeInfluxSeriesAgent{fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb")}
	router := setupInfluxSeriesRouter(t, agentFake)

	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(map[string]any{
		"bucket": "other-bucket",
		"fill":   "previous",
	}))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	got := agentFake.captured
	assert.Equal(t, "other-bucket", got.Bucket)
	assert.Equal(t, "cpu", got.Measurement)
	assert.Equal(t, "usage", got.Field)
	assert.Equal(t, map[string]string{"host": "a"}, got.Tags)
	assert.Equal(t, int64(1_700_000_000_000), got.StartMs)
	assert.Equal(t, int64(1_700_003_600_000), got.EndMs)
	assert.Equal(t, int64(60_000), got.IntervalMs)
	assert.Equal(t, system.SeriesAggAverage, got.Aggregation)
	assert.Equal(t, system.SeriesFillPrevious, got.Fill)
}

func TestInfluxSeriesQuery_UnknownFillIsRejected(t *testing.T) {
	t.Parallel()
	agentFake := &fakeInfluxSeriesAgent{fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb")}
	router := setupInfluxSeriesRouter(t, agentFake)
	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(map[string]any{"fill": "bogus"}))
	require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), "invalid fill")
}

// 요청 검증을 통과했더라도 에이전트가 같은 판정을 내릴 수 있다. 그 경우가
// 500 으로 새지 않고 400 으로 남는지 확인한다.
func TestInfluxSeriesQuery_AgentSideRejectionStaysBadRequest(t *testing.T) {
	t.Parallel()
	cases := map[string]error{
		"이스케이프 불가 식별자": fmt.Errorf("wrapped: %w", system.ErrUnescapableIdentifier),
		"미지원 fill":     fmt.Errorf("wrapped: %w", system.ErrUnsupportedSeriesFill),
	}
	for name, agentErr := range cases {
		t.Run(name, func(t *testing.T) {
			agentFake := &fakeInfluxSeriesAgent{
				fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
				seriesFn: func(_ context.Context, _ system.SeriesQuerySpec) ([]system.SeriesBucket, error) {
					return nil, agentErr
				},
			}
			router := setupInfluxSeriesRouter(t, agentFake)
			rec := postSeriesQuery(t, router, "metrics", validSeriesBody(nil))
			assert.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
		})
	}
}

func TestInfluxSeriesQuery_MalformedBodyIsRejected(t *testing.T) {
	t.Parallel()
	agentFake := &fakeInfluxSeriesAgent{fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb")}
	router := setupInfluxSeriesRouter(t, agentFake)
	rec := postSeriesQuery(t, router, "metrics", `{"measurement":`)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
}

// 나누어떨어지지 않는 구간은 마지막 부분 버킷까지 세야 한다. 내림하면 상한
// 바로 위의 요청이 통과한다.
func TestInfluxSeriesQuery_BucketCountRoundsUpPartialBucket(t *testing.T) {
	t.Parallel()
	const interval = int64(1_000)
	start := int64(1_700_000_000_000)
	// 상한 개의 완전 버킷 + 1ms → 올림하면 상한 + 1 이 되어 거부되어야 한다.
	end := start + interval*int64(maxAggregationBuckets) + 1

	agentFake := &fakeInfluxSeriesAgent{fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb")}
	router := setupInfluxSeriesRouter(t, agentFake)
	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(map[string]any{
		"start_ms":    start,
		"end_ms":      end,
		"interval_ms": interval,
	}))
	require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), fmt.Sprintf("bucket_count=%d", maxAggregationBuckets+1))
}

// ===== group by (SPEC-TSDB-004 M4) =====

// TestInfluxSeriesQuery_GroupBy_LabelsComeFromResult 는 §2.6 U6 을 고정한다(AC-09).
//
// 라벨은 **요청이 아니라 결과**에서 만들어져야 한다. 요청 태그를 그대로 복사하면
// 모든 그룹이 같은 라벨을 달고 클라이언트에서 한 시리즈로 접힌다 — 그룹이 나뉜
// 것처럼 보이지만 그래프는 하나다.
func TestInfluxSeriesQuery_GroupBy_LabelsComeFromResult(t *testing.T) {
	t.Parallel()
	agentFake := &fakeInfluxSeriesAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		seriesFn: func(_ context.Context, _ system.SeriesQuerySpec) ([]system.SeriesBucket, error) {
			return []system.SeriesBucket{
				{StartMs: 1_700_000_000_000, Value: 1.0, Tags: map[string]string{"host": "a"}},
				{StartMs: 1_700_000_000_000, Value: 2.0, Tags: map[string]string{"host": "b"}},
			}, nil
		},
	}
	router := setupInfluxSeriesRouter(t, agentFake)

	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(map[string]any{
		"tags":     map[string]string{"region": "kr"},
		"group_by": []string{"host"},
	}))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 2)

	// __field__ 는 모든 그룹에서 상수다 — 이것이 깨지면 클라이언트가
	// measurement 혼재로 오인한다.
	for _, e := range resp.Data.Entries {
		assert.Equal(t, "usage", e.Labels["__field__"])
		assert.Equal(t, "kr", e.Labels["region"], "사전 필터 태그는 유지된다")
	}
	// 그룹마다 host 가 다르다.
	assert.Equal(t, "a", resp.Data.Entries[0].Labels["host"])
	assert.Equal(t, "b", resp.Data.Entries[1].Labels["host"])
}

// TestInfluxSeriesQuery_GroupBy_ResultTagOverridesRequestTag 는 AC-10 을 고정한다.
//
// 요청에 없던 키가 응답에 나타나야 하고, 요청과 결과가 같은 키를 가지면
// **결과 값이 이긴다**. 요청 값이 이기면 그룹 구분이 사라진다.
func TestInfluxSeriesQuery_GroupBy_ResultTagOverridesRequestTag(t *testing.T) {
	t.Parallel()
	agentFake := &fakeInfluxSeriesAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		seriesFn: func(_ context.Context, _ system.SeriesQuerySpec) ([]system.SeriesBucket, error) {
			return []system.SeriesBucket{
				// 결과가 요청에 없던 rack 을 들고 온다.
				{StartMs: 1_700_000_000_000, Value: 1.0, Tags: map[string]string{"rack": "r1"}},
			}, nil
		},
	}
	router := setupInfluxSeriesRouter(t, agentFake)

	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(map[string]any{
		"tags":     nil,
		"group_by": []string{"rack"},
	}))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 1)
	assert.Equal(t, "r1", resp.Data.Entries[0].Labels["rack"],
		"요청에 없던 그룹 키가 응답 라벨에 나타나야 한다")
}

// TestInfluxSeriesQuery_GroupBy_없으면라벨이이전과같다 는 §2.9 U9 를 고정한다.
// 버킷에 Tags 가 없으면 라벨 구성이 본 축 도입 이전과 정확히 같아야 한다.
func TestInfluxSeriesQuery_GroupBy_없으면라벨이이전과같다(t *testing.T) {
	t.Parallel()
	agentFake := &fakeInfluxSeriesAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		seriesFn: func(_ context.Context, _ system.SeriesQuerySpec) ([]system.SeriesBucket, error) {
			return []system.SeriesBucket{{StartMs: 1_700_000_000_000, Value: 1.0}}, nil
		},
	}
	router := setupInfluxSeriesRouter(t, agentFake)

	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(map[string]any{
		"tags": map[string]string{"host": "a"},
	}))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 1)
	assert.Equal(t, map[string]string{"__field__": "usage", "host": "a"},
		resp.Data.Entries[0].Labels)
}

// TestInfluxSeriesQuery_GroupBy_SpecCarriesGroupAxis 는 요청의 group_by 가
// 도메인 spec 으로 그대로 전달되는지 고정한다. 전달이 끊기면 백엔드는 조용히
// 정확 일치 모드로 질의하고 사용자는 "group by 가 무시된다" 고 인지한다.
func TestInfluxSeriesQuery_GroupBy_SpecCarriesGroupAxis(t *testing.T) {
	t.Parallel()
	var got system.SeriesQuerySpec
	agentFake := &fakeInfluxSeriesAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		seriesFn: func(_ context.Context, spec system.SeriesQuerySpec) ([]system.SeriesBucket, error) {
			got = spec
			return nil, nil
		},
	}
	router := setupInfluxSeriesRouter(t, agentFake)

	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(map[string]any{
		"tags":     nil, // 기본 tags(host)를 지운다 — host 는 그룹 축으로 쓴다
		"group_by": []string{"rack", "host"},
	}))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assert.Equal(t, []string{"rack", "host"}, got.GroupBy,
		"입력 순서 그대로 전달된다 — 정렬은 쿼리 생성기의 몫이다")
}

// TestInfluxSeriesQuery_GroupBy_ConflictWithTagFilterIsRejected 는 UB1-3 을
// 고정한다(AC-14). 에이전트에 도달하기 **전에** 400 이어야 한다 — §2.6 의
// "400 은 에이전트 조회 전에 전부 결정된다" 원칙이다.
func TestInfluxSeriesQuery_GroupBy_ConflictWithTagFilterIsRejected(t *testing.T) {
	t.Parallel()
	called := false
	agentFake := &fakeInfluxSeriesAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		seriesFn: func(_ context.Context, _ system.SeriesQuerySpec) ([]system.SeriesBucket, error) {
			called = true
			return nil, nil
		},
	}
	router := setupInfluxSeriesRouter(t, agentFake)

	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(map[string]any{
		"tags":     map[string]string{"host": "a", "region": "kr"},
		"group_by": []string{"host"},
	}))
	require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), "host", "충돌한 키를 알려 줘야 한다")
	assert.False(t, called, "질의가 실행되면 안 된다")
}

// TestInfluxSeriesQuery_GroupBy_NoCapNoTruncation 은 §2.7 을 고정한다(AC-11).
//
// 그룹 수가 많아도 잘리지 않고 truncated 가 켜지지 않아야 한다. 상한을 나중에
// 슬쩍 들여오면 차트가 부분 집합을 완전한 그림처럼 그리게 되므로 여기서 막는다.
func TestInfluxSeriesQuery_GroupBy_NoCapNoTruncation(t *testing.T) {
	t.Parallel()
	const groups = 200
	agentFake := &fakeInfluxSeriesAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		seriesFn: func(_ context.Context, _ system.SeriesQuerySpec) ([]system.SeriesBucket, error) {
			out := make([]system.SeriesBucket, 0, groups)
			for i := 0; i < groups; i++ {
				out = append(out, system.SeriesBucket{
					StartMs: 1_700_000_000_000,
					Value:   float64(i),
					Tags:    map[string]string{"host": fmt.Sprintf("h%03d", i)},
				})
			}
			return out, nil
		},
	}
	router := setupInfluxSeriesRouter(t, agentFake)

	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(map[string]any{
		"tags":     nil,
		"group_by": []string{"host"},
	}))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	resp := decodeQueryResponse(t, rec)
	assert.Len(t, resp.Data.Entries, groups, "그룹이 잘리면 안 된다")
	assert.Equal(t, groups, resp.Data.Count)
	assert.False(t, resp.Data.Truncated, "group by 는 truncated 를 켜지 않는다")
}

// TestInfluxSeriesQuery_GroupFilter_SpecCarriesPageSelection 은 페이지 선택이
// 도메인 spec 까지 전달되는지 고정한다(SPEC-TSDB-004 §2.7.1). 전달이 끊기면
// 서버는 전 그룹을 계산하고 페이지네이션이 무효가 된다.
func TestInfluxSeriesQuery_GroupFilter_SpecCarriesPageSelection(t *testing.T) {
	t.Parallel()
	var got system.SeriesQuerySpec
	agentFake := &fakeInfluxSeriesAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		seriesFn: func(_ context.Context, spec system.SeriesQuerySpec) ([]system.SeriesBucket, error) {
			got = spec
			return nil, nil
		},
	}
	router := setupInfluxSeriesRouter(t, agentFake)

	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(map[string]any{
		"tags":         nil,
		"group_by":     []string{"host"},
		"group_filter": []map[string]string{{"host": "a"}, {"host": "b"}},
	}))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assert.Equal(t, []map[string]string{{"host": "a"}, {"host": "b"}}, got.GroupFilter)
}

// TestInfluxSeriesQuery_GroupFilter_EmptyComboIsRejected 는 §2.7.1 을 고정한다.
// 빈 조합은 "모든 그룹" 이 되어 페이지 선택을 조용히 무효화하므로 400 이다.
func TestInfluxSeriesQuery_GroupFilter_EmptyComboIsRejected(t *testing.T) {
	t.Parallel()
	called := false
	agentFake := &fakeInfluxSeriesAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		seriesFn: func(_ context.Context, _ system.SeriesQuerySpec) ([]system.SeriesBucket, error) {
			called = true
			return nil, nil
		},
	}
	router := setupInfluxSeriesRouter(t, agentFake)

	rec := postSeriesQuery(t, router, "metrics", validSeriesBody(map[string]any{
		"tags":         nil,
		"group_by":     []string{"host"},
		"group_filter": []map[string]string{{"host": "a"}, {}},
	}))
	require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
	assert.False(t, called, "질의가 실행되면 안 된다")
}
