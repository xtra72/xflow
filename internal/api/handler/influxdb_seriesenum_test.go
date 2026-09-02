// @spec SPEC-TSDB-003 §2.2 (U2) · §2.7 (U7) · §2.8 (U8) — 시리즈 열거 라우트 D5.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/api"
)

// fakeInfluxEnumeratorAgent 는 influxSeriesEnumerator 를 구현하는 테스트용 페이크다.
//
// influxSchemaDiscoverer 도 influxManager 도 구현하지 않는다 — 열거가 **별도**
// 인터페이스로 분리되어 있는지를 이 타입의 존재가 확인한다(plan.md §4 위험 R5).
// 프로덕션의 *system.InfluxDBAgent 는 셋 모두를 만족한다.
type fakeInfluxEnumeratorAgent struct {
	*fakeAgentCommon
	enumerateFn func(ctx context.Context, spec system.SeriesEnumSpec) (system.SeriesEnumResult, error)
	gotSpec     system.SeriesEnumSpec
	calls       int
}

func (f *fakeInfluxEnumeratorAgent) EnumerateSeries(
	ctx context.Context, spec system.SeriesEnumSpec,
) (system.SeriesEnumResult, error) {
	f.calls++
	f.gotSpec = spec
	if f.enumerateFn != nil {
		return f.enumerateFn(ctx, spec)
	}
	return system.SeriesEnumResult{}, nil
}

func newInfluxEnumeratorFake(name string) *fakeInfluxEnumeratorAgent {
	return &fakeInfluxEnumeratorAgent{fakeAgentCommon: newFakeAgent("i1", name, "influxdb")}
}

// enumGet 은 열거 GET 요청 헬퍼다.
func enumGet(t *testing.T, agents []agent.Agent, target string) *httptest.ResponseRecorder {
	t.Helper()
	router := setupInfluxManagementRouter(t, agents...)
	return mgmtReq(t, router, http.MethodGet, target, "")
}

// enumSeriesDTO 는 응답 series 항목의 디코딩 형상이다.
type enumSeriesDTO struct {
	Tags   map[string]string `json:"tags"`
	Fields []string          `json:"fields"`
}

// enumResponseDTO 는 열거 응답의 디코딩 형상이다(§2.2 의 5키).
type enumResponseDTO struct {
	Series     []enumSeriesDTO `json:"series"`
	FieldExact bool            `json:"field_exact"`
	Count      int             `json:"count"`
	Truncated  bool            `json:"truncated"`
	Window     struct {
		StartMs int64 `json:"start_ms"`
		EndMs   int64 `json:"end_ms"`
	} `json:"window"`
}

func decodeEnumResponse(t *testing.T, rec *httptest.ResponseRecorder) enumResponseDTO {
	t.Helper()
	var resp struct {
		Success bool            `json:"success"`
		Data    enumResponseDTO `json:"data"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	return resp.Data
}

// enumSeries 는 태그 1벌 · 필드 목록으로 시리즈 1개를 만든다.
func enumSeries(tags map[string]string, fields ...string) system.EnumeratedSeries {
	return system.EnumeratedSeries{Tags: tags, Fields: fields}
}

// ===== 라우트 등록 (AC-01) =====

// TestInfluxSeriesEnum_라우트_등록 은 D5 가 추가로 1 종 등록됨을 확인한다.
func TestInfluxSeriesEnum_라우트_등록(t *testing.T) {
	t.Parallel()
	router := api.NewRouter()
	h := NewInfluxDBManagementHandler(&fakeAgentLookup{}, nil)
	before := router.RouteCount()
	h.RegisterRoutes(router.Group("/api/v1"))
	assert.Equal(t, 10, router.RouteCount()-before, "관리 6 종 + 디스커버리 3 종 + 열거 1 종")

	seen := false
	for _, r := range router.Routes() {
		if r.Pattern != "/api/v1/influxdb/{agent_name}/series" {
			continue
		}
		seen = true
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "store.read", r.Permission, "열거는 읽기 전용이다")
	}
	assert.True(t, seen, "D5 라우트가 등록되지 않았다")
}

// TestInfluxSeriesEnum_접두사_충돌없음 은 OQ1 의 가정을 **라우터에 대고** 검증한다.
//
// GET /influxdb/{agent}/series 는 기존 POST /influxdb/{agent}/series/query 와
// 경로 접두사를 공유한다. 메서드가 달라 충돌하지 않는다는 것이 OQ1 의 확정이며,
// 그 확정을 가정으로 두지 않고 두 핸들러를 같은 라우터에 등록해 확인한다.
func TestInfluxSeriesEnum_접두사_충돌없음(t *testing.T) {
	t.Parallel()
	f := newInfluxEnumeratorFake("metrics")
	lookup := &fakeAgentLookup{agents: []agent.Agent{f}}

	router := api.NewRouter()
	g := router.Group("/api/v1")
	require.NotPanics(t, func() {
		NewInfluxDBManagementHandler(lookup, nil).RegisterRoutes(g)
		NewInfluxDBSeriesHandler(lookup, nil).RegisterRoutes(g)
	}, "같은 접두사의 두 라우트 등록이 ServeMux 에서 충돌했다")

	// GET /series 는 열거 핸들러로 간다(200).
	rec := mgmtReq(t, router, http.MethodGet, "/api/v1/influxdb/metrics/series?measurement=cpu", "")
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assert.Equal(t, 1, f.calls, "GET /series 가 열거 핸들러에 도달하지 않았다")

	// POST /series/query 는 질의 핸들러로 간다 — 열거 핸들러를 부르지 않는다.
	rec = mgmtReq(t, router, http.MethodPost, "/api/v1/influxdb/metrics/series/query", `{"measurement":"cpu"}`)
	assert.NotEqual(t, http.StatusNotFound, rec.Code, "POST /series/query 가 사라졌다")
	assert.Equal(t, 1, f.calls, "POST /series/query 가 열거 핸들러로 잘못 라우팅되었다")
}

// ===== 응답 형상 (AC-02) =====

// TestInfluxSeriesEnum_응답형상 은 §2.2 의 5 개 최상위 키를 확인한다.
func TestInfluxSeriesEnum_응답형상(t *testing.T) {
	f := newInfluxEnumeratorFake("metrics")
	f.enumerateFn = func(_ context.Context, _ system.SeriesEnumSpec) (system.SeriesEnumResult, error) {
		return system.SeriesEnumResult{
			Series: []system.EnumeratedSeries{
				enumSeries(map[string]string{"host": "a", "region": "kr"}, "idle", "usage"),
				enumSeries(map[string]string{"host": "b", "region": "kr"}, "usage"),
			},
			FieldExact: true,
		}, nil
	}

	rec := enumGet(t, []agent.Agent{f},
		"/api/v1/influxdb/metrics/series?measurement=cpu&start_ms=1700000000000&end_ms=1702592000000")
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	// 봉투 안 data 의 최상위 키가 5 개 전부 존재하는지 raw 로 확인한다.
	var raw struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	for _, k := range []string{"series", "field_exact", "count", "truncated", "window"} {
		assert.Contains(t, raw.Data, k, "응답에 %q 키가 없다", k)
	}

	rec = enumGet(t, []agent.Agent{f},
		"/api/v1/influxdb/metrics/series?measurement=cpu&start_ms=1700000000000&end_ms=1702592000000")
	got := decodeEnumResponse(t, rec)
	assert.Equal(t, 2, got.Count)
	assert.False(t, got.Truncated)
	assert.True(t, got.FieldExact)
	require.Len(t, got.Series, 2)
	assert.Equal(t, map[string]string{"host": "a", "region": "kr"}, got.Series[0].Tags)
	assert.Equal(t, []string{"idle", "usage"}, got.Series[0].Fields)
	assert.Equal(t, int64(1700000000000), got.Window.StartMs)
	assert.Equal(t, int64(1702592000000), got.Window.EndMs)
}

// TestInfluxSeriesEnum_빈결과 는 nil 결과가 null 이 아니라 빈 배열로 나감을
// 확인한다 — 클라이언트가 length 를 바로 읽을 수 있어야 한다(D2~D4 와 같은 규약).
func TestInfluxSeriesEnum_빈결과(t *testing.T) {
	f := newInfluxEnumeratorFake("metrics")
	rec := enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series?measurement=cpu")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"series":[]`)

	got := decodeEnumResponse(t, rec)
	assert.Equal(t, 0, got.Count)
	assert.False(t, got.Truncated)
}

// TestInfluxSeriesEnum_태그없는시리즈 는 태그 0 개 measurement 가 {} 로 나감을
// 확인한다(§2.2 — "키 없는 시리즈는 {}"). null 이면 프런트가 태그 맵을 못 읽는다.
func TestInfluxSeriesEnum_태그없는시리즈(t *testing.T) {
	f := newInfluxEnumeratorFake("metrics")
	f.enumerateFn = func(_ context.Context, _ system.SeriesEnumSpec) (system.SeriesEnumResult, error) {
		return system.SeriesEnumResult{
			Series:     []system.EnumeratedSeries{{Tags: nil, Fields: []string{"beat"}}},
			FieldExact: true,
		}, nil
	}

	rec := enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series?measurement=heartbeat")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"tags":{}`)
	got := decodeEnumResponse(t, rec)
	assert.Equal(t, 1, got.Count)
}

// ===== 파라미터 파싱 (AC-03 · AC-05) =====

func TestInfluxSeriesEnum_measurement누락_400(t *testing.T) {
	f := newInfluxEnumeratorFake("metrics")
	rec := enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "measurement is required")
	assert.Zero(t, f.calls, "measurement 없이 열거를 실행해서는 안 된다")
}

// TestInfluxSeriesEnum_태그필터_파싱 은 tags 가 k=v,k=v 로 파싱됨을 확인한다(AC-05).
func TestInfluxSeriesEnum_태그필터_파싱(t *testing.T) {
	f := newInfluxEnumeratorFake("metrics")
	rec := enumGet(t, []agent.Agent{f},
		"/api/v1/influxdb/metrics/series?measurement=cpu&tags=region%3Dkr%2Chost%3Da")
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assert.Equal(t, map[string]string{"region": "kr", "host": "a"}, f.gotSpec.Tags)
}

// TestInfluxSeriesEnum_태그필터_등호포함값 은 값에 등호가 있어도 첫 등호에서만
// 쪼갬을 확인한다. 태그 값에 등호가 정당하게 들어갈 수 있다.
func TestInfluxSeriesEnum_태그필터_등호포함값(t *testing.T) {
	f := newInfluxEnumeratorFake("metrics")
	rec := enumGet(t, []agent.Agent{f},
		"/api/v1/influxdb/metrics/series?measurement=cpu&tags=k%3Da%3Db")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, map[string]string{"k": "a=b"}, f.gotSpec.Tags)
}

// TestInfluxSeriesEnum_태그필터_파싱불가_400 은 등호 없는 항목 · 빈 키 · 빈 항목을
// 거부함을 확인한다(§2.2 오류표 "tags 파싱 불가").
func TestInfluxSeriesEnum_태그필터_파싱불가_400(t *testing.T) {
	cases := map[string]string{
		"등호없음":  "region",
		"빈키":    "%3Dkr",
		"후행쉼표":  "region%3Dkr%2C",
		"빈항목중간": "a%3D1%2C%2Cb%3D2",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			f := newInfluxEnumeratorFake("metrics")
			rec := enumGet(t, []agent.Agent{f},
				"/api/v1/influxdb/metrics/series?measurement=cpu&tags="+raw)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Zero(t, f.calls, "파싱 실패 시 열거를 실행해서는 안 된다")
		})
	}
}

// TestInfluxSeriesEnum_숫자파싱불가_400 은 start_ms · end_ms · limit 의 숫자
// 파싱 실패를 400 으로 거부함을 확인한다. 사용자가 고칠 수 있는 입력 오류를
// 조용히 기본값으로 대체하면 사용자가 다른 창을 보고 있음을 알 수 없다.
func TestInfluxSeriesEnum_숫자파싱불가_400(t *testing.T) {
	for _, q := range []string{"start_ms=abc", "end_ms=abc", "limit=abc"} {
		t.Run(q, func(t *testing.T) {
			f := newInfluxEnumeratorFake("metrics")
			rec := enumGet(t, []agent.Agent{f},
				"/api/v1/influxdb/metrics/series?measurement=cpu&"+q)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Zero(t, f.calls)
		})
	}
}

// TestInfluxSeriesEnum_bucket전달 은 bucket 을 그대로 넘김을 확인한다.
// 빈 값이면 빈 값 그대로 넘겨 에이전트가 기본값을 채우게 한다(D2~D4 와 같은 규약).
func TestInfluxSeriesEnum_bucket전달(t *testing.T) {
	f := newInfluxEnumeratorFake("metrics")
	rec := enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series?measurement=cpu&bucket=b1")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "b1", f.gotSpec.Bucket)
	assert.Equal(t, "cpu", f.gotSpec.Measurement)

	f2 := newInfluxEnumeratorFake("metrics")
	rec = enumGet(t, []agent.Agent{f2}, "/api/v1/influxdb/metrics/series?measurement=cpu")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, f2.gotSpec.Bucket)
}

// ===== 탐색 창 (AC-24 · AC-25) =====

// TestInfluxSeriesEnum_기본창_30일 은 창 미지정 시 now-30d ~ now 가 적용되고
// 응답 window 에 드러남을 확인한다(§2.8).
func TestInfluxSeriesEnum_기본창_30일(t *testing.T) {
	f := newInfluxEnumeratorFake("metrics")
	before := time.Now().UnixMilli()
	rec := enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series?measurement=cpu")
	after := time.Now().UnixMilli()
	require.Equal(t, http.StatusOK, rec.Code)

	got := decodeEnumResponse(t, rec)
	const day30 = int64(30 * 24 * 60 * 60 * 1000)
	assert.GreaterOrEqual(t, got.Window.EndMs, before)
	assert.LessOrEqual(t, got.Window.EndMs, after)
	assert.Equal(t, got.Window.EndMs-day30, got.Window.StartMs, "기본 폭이 30일이 아니다")

	// 실제로 사용한 창이 에이전트에 그대로 전달된다.
	assert.Equal(t, got.Window.StartMs, f.gotSpec.StartMs)
	assert.Equal(t, got.Window.EndMs, f.gotSpec.EndMs)
}

// TestInfluxSeriesEnum_지정창_반영 은 지정한 창이 그대로 쓰이고 응답에 되비침을
// 확인한다.
func TestInfluxSeriesEnum_지정창_반영(t *testing.T) {
	f := newInfluxEnumeratorFake("metrics")
	rec := enumGet(t, []agent.Agent{f},
		"/api/v1/influxdb/metrics/series?measurement=cpu&start_ms=1700000000000&end_ms=1702592000000")
	require.Equal(t, http.StatusOK, rec.Code)

	got := decodeEnumResponse(t, rec)
	assert.Equal(t, int64(1700000000000), got.Window.StartMs)
	assert.Equal(t, int64(1702592000000), got.Window.EndMs)
	assert.Equal(t, int64(1700000000000), f.gotSpec.StartMs)
	assert.Equal(t, int64(1702592000000), f.gotSpec.EndMs)
}

// TestInfluxSeriesEnum_start만지정 은 end 만 기본값(now)으로 채워짐을 확인한다.
func TestInfluxSeriesEnum_start만지정(t *testing.T) {
	f := newInfluxEnumeratorFake("metrics")
	before := time.Now().UnixMilli()
	rec := enumGet(t, []agent.Agent{f},
		"/api/v1/influxdb/metrics/series?measurement=cpu&start_ms=1700000000000")
	require.Equal(t, http.StatusOK, rec.Code)

	got := decodeEnumResponse(t, rec)
	assert.Equal(t, int64(1700000000000), got.Window.StartMs)
	assert.GreaterOrEqual(t, got.Window.EndMs, before)
}

// ===== 오류 매핑 (AC-04) =====

func TestInfluxSeriesEnum_오류매핑(t *testing.T) {
	t.Run("에이전트없음_404", func(t *testing.T) {
		rec := enumGet(t, nil, "/api/v1/influxdb/ghost/series?measurement=cpu")
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "agent_not_found")
	})

	t.Run("잘못된타입_400", func(t *testing.T) {
		storeFake := &fakeStoreAgent{fakeAgentCommon: newFakeAgent("s1", "wrong", "store")}
		rec := enumGet(t, []agent.Agent{storeFake}, "/api/v1/influxdb/wrong/series?measurement=cpu")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "not_an_influxdb_agent")
	})

	t.Run("이스케이프불가_400", func(t *testing.T) {
		f := newInfluxEnumeratorFake("metrics")
		// 제어 문자가 든 measurement 는 질의 전에 거부된다 — 네트워크를 타지 않는다.
		rec := enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series?measurement=cp%00u")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Zero(t, f.calls, "이스케이프 불가 식별자로 질의를 실행했다")
	})

	t.Run("이스케이프불가_태그값_400", func(t *testing.T) {
		f := newInfluxEnumeratorFake("metrics")
		rec := enumGet(t, []agent.Agent{f},
			"/api/v1/influxdb/metrics/series?measurement=cpu&tags=host%3Da%0Ab")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Zero(t, f.calls)
	})

	t.Run("창역전_400", func(t *testing.T) {
		f := newInfluxEnumeratorFake("metrics")
		rec := enumGet(t, []agent.Agent{f},
			"/api/v1/influxdb/metrics/series?measurement=cpu&start_ms=200&end_ms=100")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "end_ms")
		assert.Zero(t, f.calls)
	})

	t.Run("창동일_400", func(t *testing.T) {
		f := newInfluxEnumeratorFake("metrics")
		rec := enumGet(t, []agent.Agent{f},
			"/api/v1/influxdb/metrics/series?measurement=cpu&start_ms=100&end_ms=100")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Zero(t, f.calls)
	})

	t.Run("타임아웃_408", func(t *testing.T) {
		f := newInfluxEnumeratorFake("metrics")
		f.enumerateFn = func(_ context.Context, _ system.SeriesEnumSpec) (system.SeriesEnumResult, error) {
			return system.SeriesEnumResult{}, context.DeadlineExceeded
		}
		rec := enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series?measurement=cpu")
		assert.Equal(t, http.StatusRequestTimeout, rec.Code)
	})

	t.Run("업스트림오류_502", func(t *testing.T) {
		f := newInfluxEnumeratorFake("metrics")
		f.enumerateFn = func(_ context.Context, _ system.SeriesEnumSpec) (system.SeriesEnumResult, error) {
			return system.SeriesEnumResult{}, errors.New("upstream down")
		}
		rec := enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series?measurement=cpu")
		assert.Equal(t, http.StatusBadGateway, rec.Code)
	})

	t.Run("에이전트가_이스케이프불가_400", func(t *testing.T) {
		// 핸들러 선검사를 통과했더라도 에이전트가 sentinel 을 돌려주면 400 이다
		// — mapInfluxDiscoveryError 재사용(M4.6).
		f := newInfluxEnumeratorFake("metrics")
		f.enumerateFn = func(_ context.Context, _ system.SeriesEnumSpec) (system.SeriesEnumResult, error) {
			return system.SeriesEnumResult{}, system.ErrUnescapableIdentifier
		}
		rec := enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series?measurement=cpu")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// ===== 정렬 · 상한 · 절단 (AC-06 · AC-20 · AC-22) =====

// TestInfluxSeriesEnum_결정적정렬 은 입력 순서를 뒤집어도 같은 출력임을 확인한다
// (§2.2 · UB1-16).
//
// 접기 함수가 이미 정렬해 주지만 핸들러가 **어느 1,000 개를 남길지** 를 정하는
// 지점이므로(§2.7 — 태그 집합 상한은 핸들러가 강제한다) 순서의 책임도 여기 있다.
func TestInfluxSeriesEnum_결정적정렬(t *testing.T) {
	forward := []system.EnumeratedSeries{
		enumSeries(map[string]string{"host": "a"}, "usage"),
		enumSeries(map[string]string{"host": "b"}, "usage"),
		enumSeries(map[string]string{"host": "c"}, "usage"),
	}
	reversed := []system.EnumeratedSeries{forward[2], forward[1], forward[0]}

	run := func(in []system.EnumeratedSeries) []enumSeriesDTO {
		f := newInfluxEnumeratorFake("metrics")
		f.enumerateFn = func(_ context.Context, _ system.SeriesEnumSpec) (system.SeriesEnumResult, error) {
			return system.SeriesEnumResult{Series: in, FieldExact: true}, nil
		}
		rec := enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series?measurement=cpu")
		require.Equal(t, http.StatusOK, rec.Code)
		return decodeEnumResponse(t, rec).Series
	}

	got := run(forward)
	require.Len(t, got, 3)
	assert.Equal(t, "a", got[0].Tags["host"])
	assert.Equal(t, "c", got[2].Tags["host"])
	assert.Equal(t, got, run(reversed), "입력 순서가 출력에 새었다")
}

// makeEnumSeries 는 host=sNNNN 태그를 가진 시리즈 n 개를 만든다.
func makeEnumSeries(n int) []system.EnumeratedSeries {
	out := make([]system.EnumeratedSeries, 0, n)
	for i := range n {
		out = append(out, enumSeries(map[string]string{"host": fmt.Sprintf("s%04d", i)}, "usage"))
	}
	return out
}

// TestInfluxSeriesEnum_태그집합상한 은 반환 태그 집합 1,000 상한을 확인한다(§2.7 · AC-20).
func TestInfluxSeriesEnum_태그집합상한(t *testing.T) {
	f := newInfluxEnumeratorFake("metrics")
	f.enumerateFn = func(_ context.Context, _ system.SeriesEnumSpec) (system.SeriesEnumResult, error) {
		return system.SeriesEnumResult{Series: makeEnumSeries(1200), FieldExact: true}, nil
	}

	rec := enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series?measurement=cpu")
	require.Equal(t, http.StatusOK, rec.Code)

	got := decodeEnumResponse(t, rec)
	assert.Equal(t, 1000, got.Count)
	assert.Len(t, got.Series, 1000)
	assert.True(t, got.Truncated, "상한에 걸렸는데 truncated 가 거짓이다")
	assert.Equal(t, "s0000", got.Series[0].Tags["host"])
	assert.Equal(t, "s0999", got.Series[999].Tags["host"], "상위 N 이 아니다")
}

// TestInfluxSeriesEnum_상한이하_절단없음 은 정확히 상한만큼일 때 truncated 가
// 거짓임을 확인한다 — 경계에서 거짓 양성을 내면 사용자가 없는 문제를 좁힌다.
func TestInfluxSeriesEnum_상한이하_절단없음(t *testing.T) {
	f := newInfluxEnumeratorFake("metrics")
	f.enumerateFn = func(_ context.Context, _ system.SeriesEnumSpec) (system.SeriesEnumResult, error) {
		return system.SeriesEnumResult{Series: makeEnumSeries(1000), FieldExact: true}, nil
	}
	rec := enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series?measurement=cpu")
	require.Equal(t, http.StatusOK, rec.Code)

	got := decodeEnumResponse(t, rec)
	assert.Equal(t, 1000, got.Count)
	assert.False(t, got.Truncated)
}

// TestInfluxSeriesEnum_limit파라미터 는 limit 이 반환 태그 집합 수를 좁힘을
// 확인한다(§2.2 — limit 은 태그 집합 축이다).
func TestInfluxSeriesEnum_limit파라미터(t *testing.T) {
	newFake := func() *fakeInfluxEnumeratorAgent {
		f := newInfluxEnumeratorFake("metrics")
		f.enumerateFn = func(_ context.Context, _ system.SeriesEnumSpec) (system.SeriesEnumResult, error) {
			return system.SeriesEnumResult{Series: makeEnumSeries(5), FieldExact: true}, nil
		}
		return f
	}

	f := newFake()
	rec := enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series?measurement=cpu&limit=2")
	require.Equal(t, http.StatusOK, rec.Code)
	got := decodeEnumResponse(t, rec)
	assert.Equal(t, 2, got.Count)
	assert.True(t, got.Truncated)
	assert.Equal(t, "s0000", got.Series[0].Tags["host"])

	// 서버 상한을 넘는 limit 은 서버 상한으로 절삭된다 — 클라이언트가 상한을
	// 올릴 수 없어야 한다(§2.7 "상한은 서버에 둔다").
	f = newFake()
	rec = enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series?measurement=cpu&limit=99999")
	require.Equal(t, http.StatusOK, rec.Code)
	got = decodeEnumResponse(t, rec)
	assert.Equal(t, 5, got.Count)
	assert.False(t, got.Truncated)

	// 0 · 음수는 미지정과 같다.
	for _, v := range []string{"0", "-1"} {
		f = newFake()
		rec = enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series?measurement=cpu&limit="+v)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, 5, decodeEnumResponse(t, rec).Count, "limit=%s", v)
	}
}

// TestInfluxSeriesEnum_절단_결정적 은 같은 집합을 다르게 섞어 두 번 보내도 같은
// 1,000 개가 나옴을 확인한다(OQ9 · AC-22).
//
// 폴링마다 다른 부분집합이 잘리면 사용자가 고른 시리즈가 목록에서 사라졌다
// 나타났다 한다.
func TestInfluxSeriesEnum_절단_결정적(t *testing.T) {
	base := makeEnumSeries(1200)

	run := func(seed int64) []enumSeriesDTO {
		shuffled := append([]system.EnumeratedSeries(nil), base...)
		r := rand.New(rand.NewSource(seed))
		r.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

		f := newInfluxEnumeratorFake("metrics")
		f.enumerateFn = func(_ context.Context, _ system.SeriesEnumSpec) (system.SeriesEnumResult, error) {
			return system.SeriesEnumResult{Series: shuffled, FieldExact: true}, nil
		}
		rec := enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series?measurement=cpu")
		require.Equal(t, http.StatusOK, rec.Code)
		return decodeEnumResponse(t, rec).Series
	}

	first, second := run(1), run(2)
	require.Len(t, first, 1000)
	assert.Equal(t, first, second, "절단된 부분집합이 입력 순서에 따라 달라졌다")
}

// TestInfluxSeriesEnum_정렬_구분자충돌 은 태그 값에 구분자(, = \)가 들어가도
// 서로 다른 두 시리즈가 같은 정렬 키로 뭉개지지 않음을 확인한다.
func TestInfluxSeriesEnum_정렬_구분자충돌(t *testing.T) {
	in := []system.EnumeratedSeries{
		enumSeries(map[string]string{"a": "b", "c": "d"}, "usage"),
		enumSeries(map[string]string{"a": "b,c=d"}, "usage"),
	}
	f := newInfluxEnumeratorFake("metrics")
	f.enumerateFn = func(_ context.Context, _ system.SeriesEnumSpec) (system.SeriesEnumResult, error) {
		return system.SeriesEnumResult{Series: in, FieldExact: true}, nil
	}
	rec := enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series?measurement=cpu")
	require.Equal(t, http.StatusOK, rec.Code)

	got := decodeEnumResponse(t, rec)
	assert.Equal(t, 2, got.Count, "구분자 충돌로 시리즈가 합쳐졌다")
}

// ===== field_exact 통과 (AC-15 대응 · §2.6) =====

// TestInfluxSeriesEnum_FieldExact_통과 는 핸들러가 백엔드 버전으로 분기하지 않고
// 에이전트가 보고한 값을 그대로 통과시킴을 확인한다(§2.6 — 정본은 서버 응답이다).
func TestInfluxSeriesEnum_FieldExact_통과(t *testing.T) {
	for _, want := range []bool{true, false} {
		t.Run(fmt.Sprintf("%t", want), func(t *testing.T) {
			f := newInfluxEnumeratorFake("metrics")
			f.enumerateFn = func(_ context.Context, _ system.SeriesEnumSpec) (system.SeriesEnumResult, error) {
				return system.SeriesEnumResult{
					Series:     []system.EnumeratedSeries{enumSeries(map[string]string{"host": "a"}, "usage")},
					FieldExact: want,
				}, nil
			}
			rec := enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series?measurement=cpu")
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, want, decodeEnumResponse(t, rec).FieldExact)
		})
	}
}

// TestInfluxSeriesEnum_백엔드_무분기 는 핸들러가 버전 문자열로 분기하지 않음을
// 소스에서 확인한다. 분기가 생기면 백엔드가 늘 때마다 핸들러가 깨진다.
func TestInfluxSeriesEnum_백엔드_무분기(t *testing.T) {
	t.Parallel()
	src := readHandlerSource(t, "influxdb_seriesenum.go")
	for _, forbidden := range []string{`"v2"`, `"v3"`, "influxV2Client", "influxV3Client"} {
		assert.NotContains(t, src, forbidden, "핸들러가 백엔드 버전으로 분기한다")
	}
}

// ===== 캐시 금지 (AC-27 대응 · UB1-5) =====

// TestInfluxSeriesEnum_캐시없음 은 같은 요청을 두 번 보내면 에이전트도 두 번
// 불림을 확인한다(§2.9). 스키마는 쓰기에 따라 변한다.
func TestInfluxSeriesEnum_캐시없음(t *testing.T) {
	f := newInfluxEnumeratorFake("metrics")
	router := setupInfluxManagementRouter(t, f)
	for range 2 {
		rec := mgmtReq(t, router, http.MethodGet, "/api/v1/influxdb/metrics/series?measurement=cpu", "")
		require.Equal(t, http.StatusOK, rec.Code)
	}
	assert.Equal(t, 2, f.calls, "열거 응답이 캐시되었다")
}

// ===== 정적 경계 (AC-31 / UB1-12) =====

// TestInfluxSeriesEnum_tsdbtags_미import 는 API 계층이 마이그레이션 패키지를
// import 하지 않음을 확인한다(SPEC-TSDB-002 AC-31 승계 · UB1-12).
func TestInfluxSeriesEnum_tsdbtags_미import(t *testing.T) {
	t.Parallel()
	for _, f := range []string{"influxdb_seriesenum.go", "influxdb_management.go"} {
		assert.NotContains(t, readHandlerSource(t, f), "migrate/tsdbtags", "파일=%s", f)
	}
}

// readHandlerSource 는 handler 패키지의 소스 파일을 읽는다.
// 테스트가 자기 패키지 디렉터리에서 실행되므로 상대 경로로 충분하다.
func readHandlerSource(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	require.NoError(t, err)
	return string(b)
}

// §2.7 은 "두 상한 중 어느 하나라도 걸리면 truncated" 를 요구한다. 태그 집합
// 상한은 핸들러가 관측하지만 원시 행 상한은 열거 계층만 관측할 수 있다.
// 행이 잘리면 태그 집합 수가 상한 아래여도 목록은 불완전하므로, 그 신호를
// 무시하면 사용자가 잘린 목록을 전부라고 믿게 된다.
func TestInfluxSeriesEnum_원시행상한_절단신호(t *testing.T) {
	f := newInfluxEnumeratorFake("metrics")
	f.enumerateFn = func(_ context.Context, _ system.SeriesEnumSpec) (system.SeriesEnumResult, error) {
		// 태그 집합은 3개뿐 — 태그 집합 상한(1,000)에는 한참 못 미친다.
		return system.SeriesEnumResult{
			Series:      makeEnumSeries(3),
			FieldExact:  true,
			RowLimitHit: true,
		}, nil
	}
	rec := enumGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/series?measurement=cpu")
	require.Equal(t, http.StatusOK, rec.Code)

	got := decodeEnumResponse(t, rec)
	assert.Equal(t, 3, got.Count, "태그 집합 상한에는 걸리지 않았다")
	assert.True(t, got.Truncated, "원시 행이 잘렸으므로 목록은 불완전하다")
}
