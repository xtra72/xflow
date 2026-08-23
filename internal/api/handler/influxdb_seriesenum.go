// @spec SPEC-TSDB-003 §2.2 (U2) · §2.7 (U7) · §2.8 (U8)
//
// 시리즈 열거 라우트 D5 다. 디스커버리 D1~D4(influxdb_management.go)의 다섯 번째
// 항목이며 같은 핸들러에 등록한다 — resolveDiscoverer 와 같은 에이전트 해석,
// mapInfluxDiscoveryError 와 같은 오류 매핑, 같은 타임아웃을 그대로 쓴다.
//
// **이 핸들러는 백엔드 버전을 모른다.** v2 는 Flux 그룹 키로, v3 는 InfluxQL
// GROUP BY * 로 열거하며 접기 규칙도 다르지만(§2.4 · §2.5), 그 차이는 전부
// system.InfluxSeriesEnumerator 뒤에 있다. 핸들러가 버전으로 분기하면 백엔드가
// 늘 때마다 HTTP 계층이 깨진다.
//
// **핸들러가 소유하는 것은 두 가지다.**
//
//  1. 반환 태그 집합 상한 1,000 과 절단 신호(§2.7). 원시 행 상한 20,000 은
//     생성된 쿼리가 이미 강제한다 — 이쪽은 DOM 행 수를, 저쪽은 백엔드 계산량을
//     막는다.
//  2. 탐색 창 기본값 적용과 **실제로 사용한 창**의 회신(§2.8).
package handler

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
)

// influxSeriesEnumerator 는 InfluxDB 에이전트가 제공해야 하는 시리즈 열거 계약이다.
// *system.InfluxDBAgent 가 이 인터페이스를 만족한다.
//
// influxSchemaDiscoverer 와 합치지 않는다(plan.md §4 위험 R5). 넓히면 그
// 인터페이스를 만족하던 모든 구현(테스트 모의 포함)이 동시에 깨진다.
type influxSeriesEnumerator interface {
	EnumerateSeries(ctx context.Context, spec system.SeriesEnumSpec) (system.SeriesEnumResult, error)
}

// maxInfluxSeriesEnumTagSets 는 **반환 태그 집합** 수 상한이다(§2.7 · OQ4).
//
// 근거는 선택 표가 전 행을 렌더한다는 사실이다(SeriesSelectTable — 가상화 없음).
// 수천 행을 반환하면 검색·facet 필터가 있어도 DOM 이 먼저 무너진다. 정밀한
// 측정치가 아니라 정성적 상한이며, 가상화 도입은 본 SPEC 의 범위 밖이다.
//
// 상한을 **서버**에 두는 이유: 클라이언트 상한은 InfluxDB 가 계산을 마친 뒤에야
// 작동하므로 백엔드 부하를 막지 못한다.
const maxInfluxSeriesEnumTagSets = 1000

// resolveEnumerator 는 agent_name 으로 에이전트를 조회하고 influxSeriesEnumerator
// 로 검증한다. 판정 실패의 처분은 resolveDiscoverer 와 같다(404 / 400).
func (h *InfluxDBManagementHandler) resolveEnumerator(ctx api.Context) (influxSeriesEnumerator, *api.APIError) {
	agentName := ctx.Param("agent_name")
	if agentName == "" {
		return nil, api.ErrBadRequest.WithMessage("agent_name is required")
	}

	ag := findAgentByName(h.agents, agentName)
	if ag == nil {
		return nil, api.ErrNotFound.
			WithMessage("agent_not_found: " + agentName).
			WithDetails(map[string]string{"error": "agent_not_found"})
	}

	enum, ok := ag.(influxSeriesEnumerator)
	if !ok {
		return nil, api.ErrBadRequest.
			WithMessage("not_an_influxdb_agent: " + agentName).
			WithDetails(map[string]string{"error": "not_an_influxdb_agent"})
	}
	return enum, nil
}

// parseSeriesEnumTagFilter 는 `k1=v1,k2=v2` 형식의 사전 필터를 파싱한다(§2.2).
//
// 엄격하게 거부한다 — 파싱할 수 없는 필터를 조용히 무시하면 사용자가 좁혔다고
// 믿는 목록이 실제로는 좁혀지지 않은 전체 목록이 된다. 그 오해는 절단이 걸린
// 상태에서 특히 비싸다(§2.12).
//
// 값에 등호가 있는 경우는 첫 등호에서만 쪼갠다 — 태그 값에 등호가 정당하게
// 들어갈 수 있고, 거부하면 고를 수 없는 시리즈가 생긴다.
func parseSeriesEnumTagFilter(raw string) (map[string]string, error) {
	if raw == "" {
		return nil, nil
	}
	out := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		k, v, ok := strings.Cut(pair, "=")
		if !ok || k == "" {
			return nil, api.ErrBadRequest.
				WithMessage("tags must be a comma-separated list of key=value pairs")
		}
		out[k] = v
	}
	return out, nil
}

// parseSeriesEnumInt 는 선택적 정수 질의 파라미터를 파싱한다.
// 미지정은 0 이며, 파싱 불가는 400 이다 — 사용자가 고칠 수 있는 입력 오류를
// 조용히 기본값으로 대체하면 사용자가 의도한 것과 다른 창을 보게 된다.
func parseSeriesEnumInt(ctx api.Context, name string) (int64, *api.APIError) {
	raw := ctx.Query(name)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, api.ErrBadRequest.WithMessage(name + " must be an integer")
	}
	return n, nil
}

// ListSeries 는 GET /influxdb/{agent_name}/series 를 처리한다(D5 · §2.2).
//
// 질의 파라미터: measurement(필수) · bucket · start_ms · end_ms · tags · limit.
//
// 검증 순서가 형상을 정한다 — 식별자·창 검증이 전부 끝난 뒤에야 에이전트를
// 부른다. 사용자가 고칠 수 있는 입력 오류는 네트워크를 타기 전에 400 이 되어야
// 하고, 그래야 업스트림 장애(502)와 구분된다.
func (h *InfluxDBManagementHandler) ListSeries(ctx api.Context) error {
	enum, apiErr := h.resolveEnumerator(ctx)
	if apiErr != nil {
		return apiErr
	}

	measurement := ctx.Query("measurement")
	if measurement == "" {
		return api.ErrBadRequest.WithMessage("measurement is required")
	}

	tags, err := parseSeriesEnumTagFilter(ctx.Query("tags"))
	if err != nil {
		return err
	}

	startMs, apiErr := parseSeriesEnumInt(ctx, "start_ms")
	if apiErr != nil {
		return apiErr
	}
	endMs, apiErr := parseSeriesEnumInt(ctx, "end_ms")
	if apiErr != nil {
		return apiErr
	}
	limit, apiErr := parseSeriesEnumInt(ctx, "limit")
	if apiErr != nil {
		return apiErr
	}

	// 창 기본값을 **여기서** 채운다. 응답의 window 는 서버가 실제로 사용한 창이어야
	// 하므로(§2.8) 채운 결과를 spec 과 응답이 함께 쓴다. time.Now() 를 인자로
	// 넘기는 것은 ResolveSeriesEnumWindow 의 규약이다 — 그 함수는 시계를 읽지 않는다.
	startMs, endMs = system.ResolveSeriesEnumWindow(startMs, endMs, time.Now())
	if endMs <= startMs {
		return api.ErrBadRequest.WithMessage("end_ms must be greater than start_ms")
	}

	spec := system.SeriesEnumSpec{
		Bucket:      ctx.Query("bucket"),
		Measurement: measurement,
		Tags:        tags,
		StartMs:     startMs,
		EndMs:       endMs,
		// RowLimit 은 비운다 — 원시 행 상한의 기본값(20,000)이 적용된다. limit
		// 질의 파라미터는 **반환 태그 집합** 축이므로(§2.2) 이 축과 섞지 않는다.
	}
	// 이스케이프 불가 식별자는 질의 전에 400 으로 거부한다. 생성 함수도 같은
	// 검증을 하지만 그쪽 오류는 에이전트를 거쳐 오므로 업스트림 실패와 구분되지
	// 않는다.
	if err := spec.ValidateIdentifiers(); err != nil {
		return mapInfluxDiscoveryError(err)
	}

	mctx, cancel := context.WithTimeout(ctx.Context(), defaultInfluxManagementTimeout)
	defer cancel()

	result, err := enum.EnumerateSeries(mctx, spec)
	if err != nil {
		return mapInfluxDiscoveryError(err)
	}

	series, tagSetCapHit := capSeriesEnumResult(result.Series, limit)
	// §2.7 — 두 상한 중 **어느 하나라도** 걸리면 truncated 다. 태그 집합 상한은
	// 여기서 관측되고, 원시 행 상한은 열거 계층만 관측할 수 있어 결과에 실려 온다.
	// 원시 행이 잘리면 태그 집합 수가 상한 아래여도 목록은 불완전하다.
	truncated := tagSetCapHit || result.RowLimitHit
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(dto.InfluxSeriesEnumResponse{
		Series:     series,
		FieldExact: result.FieldExact,
		Count:      len(series),
		Truncated:  truncated,
		Window:     dto.InfluxSeriesEnumWindow{StartMs: startMs, EndMs: endMs},
	}))
}

// capSeriesEnumResult 는 정렬 → 상한 적용 → DTO 변환을 수행한다(§2.7 · OQ9).
//
// limit 은 요청이 준 태그 집합 상한이며 [1, maxInfluxSeriesEnumTagSets] 로
// 접힌다. 클라이언트가 서버 상한을 올릴 수 없어야 한다.
//
// **정렬을 여기서 한 번 더 하는 이유.** 접기 함수가 이미 태그 직렬화 오름차순을
// 보장하지만, "어느 N 개를 남길지"를 정하는 지점은 상한을 강제하는 이 함수다.
// 남길 부분집합의 결정성을 상류 계약에만 맡기면, 그 계약이 흔들릴 때 사용자가
// 고른 시리즈가 폴링마다 목록에서 사라졌다 나타났다 한다(UB1-16).
func capSeriesEnumResult(in []system.EnumeratedSeries, limit int64) ([]dto.InfluxEnumeratedSeries, bool) {
	limitN := maxInfluxSeriesEnumTagSets
	if limit > 0 && limit < int64(limitN) {
		limitN = int(limit)
	}

	// 정렬 키를 미리 계산한다 — 비교 함수 안에서 직렬화하면 같은 태그 집합을
	// O(log n) 번 다시 만든다.
	type keyed struct {
		key    string
		series system.EnumeratedSeries
	}
	items := make([]keyed, 0, len(in))
	for _, s := range in {
		items = append(items, keyed{key: serializeEnumTagSet(s.Tags), series: s})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].key < items[j].key })

	truncated := len(items) > limitN
	if truncated {
		items = items[:limitN]
	}

	out := make([]dto.InfluxEnumeratedSeries, 0, len(items))
	for _, it := range items {
		tags := it.series.Tags
		if tags == nil {
			// 키 없는 시리즈는 {} 다 — null 이면 프런트가 분기 없이 읽을 수 없다.
			tags = map[string]string{}
		}
		fields := it.series.Fields
		if fields == nil {
			fields = []string{}
		}
		out = append(out, dto.InfluxEnumeratedSeries{Tags: tags, Fields: fields})
	}
	return out, truncated
}

// seriesEnumTagSetEscaper 는 직렬화의 구분자 충돌을 막는다.
// 이스케이프가 없으면 {a: "b,c=d"} 와 {a: "b", c: "d"} 가 같은 키가 되어 서로
// 다른 두 시리즈의 정렬 순서가 입력 순서에 의존하게 된다.
var seriesEnumTagSetEscaper = strings.NewReplacer(`\`, `\\`, `,`, `\,`, `=`, `\=`)

// serializeEnumTagSet 은 태그 집합을 정렬 가능한 단일 문자열로 만든다(§2.2).
//
// agent 계층에도 같은 뜻의 미공개 함수가 있으나 그쪽을 넓히지 않고 복제한다 —
// 열거 구현 파일은 본 회차의 변경 대상이 아니며, 정렬 키를 얻자고 내부 함수를
// 공개 표면으로 끌어올리면 그 함수가 계약이 된다. 규칙(키 사전순 · `\` `,` `=`
// 이스케이프)은 §2.2 가 정본이므로 양쪽이 같은 문서를 따른다.
func serializeEnumTagSet(tags map[string]string) string {
	if len(tags) == 0 {
		return ""
	}
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts,
			seriesEnumTagSetEscaper.Replace(k)+"="+seriesEnumTagSetEscaper.Replace(tags[k]))
	}
	return strings.Join(parts, ",")
}
