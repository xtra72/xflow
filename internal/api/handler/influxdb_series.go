// @spec SPEC-TSDB-002 §2.6 (U6) · §2.8 (U8) · §2.9 (U9)
package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
)

// influxSeriesQueryRequest 는 구조화 시리즈 질의의 요청 바디다(§2.6).
//
// 정의는 dto 패키지가 갖는다. 핸들러 안에서는 기존 요청 타입들
// (influxDBQueryRequest · storeQueryRequest)과 같은 이름 규약으로 쓴다.
type influxSeriesQueryRequest = dto.InfluxSeriesQueryRequest

// metricLabelKey 는 시리즈 라벨의 예약 키다. Store 가 이미 쓰는 규약이며
// (store_query.go 의 시리즈 라벨 조립, 프런트의 METRIC_LABEL_KEY),
// TSDB 소스도 같은 키를 써야 클라이언트의 그룹화·표시 이름 경로가 그대로
// 재사용된다(§2.6 · §4.3).
const metricLabelKey = "__field__"

// influxSeriesQueryer 는 구조화 시리즈 질의를 제공하는 에이전트 계약이다.
// *system.InfluxDBAgent 가 이를 만족한다. 잘못된 타입의 에이전트가 지정되면
// 400 으로 거부할 수 있다.
//
// 능력(interface) 이 아니라 이 좁은 계약으로 판정하는 이유는 쓰기 경로의 선례와
// 같다 — Store 와 InfluxDB 에이전트가 둘 다 Process([]byte) 를 가지므로 넓은
// 능력 단언은 양쪽에 성립한다.
type influxSeriesQueryer interface {
	QuerySeriesBuckets(ctx context.Context, spec system.SeriesQuerySpec) ([]system.SeriesBucket, error)
}

// InfluxDBSeriesHandler 는 패널이 쓰는 구조화 시리즈 질의 라우트를 제공한다.
//
// 라우트:
//
//	POST /influxdb/{agent_name}/series/query
//
// 기존 POST /influxdb/{agent_name}/query(원문 통과)와는 별개 경로다. 목적이
// 다르다 — 원문 통과는 사용자가 쿼리를 직접 쓰는 도구이고, 이쪽은 패널이 쓰는
// 계약이다. 원문 통과 경로의 계약은 본 SPEC 에서 변경되지 않는다(§2.16 #15).
type InfluxDBSeriesHandler struct {
	agents AgentLookup
	logger *slog.Logger
}

// NewInfluxDBSeriesHandler 는 새 InfluxDBSeriesHandler 를 생성한다.
func NewInfluxDBSeriesHandler(agents AgentLookup, logger *slog.Logger) *InfluxDBSeriesHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &InfluxDBSeriesHandler{agents: agents, logger: logger}
}

// RegisterRoutes 는 구조화 시리즈 질의 라우트를 등록한다.
func (h *InfluxDBSeriesHandler) RegisterRoutes(g *api.RouteGroup) {
	// @SPEC:SPEC-AUTH-005 (M5) — 카탈로그에 influxdb 리소스가 없어 store.* 로
	// 매핑한다. 시리즈 질의는 조회이므로 read 이다.
	g.POSTPerm("/influxdb/{agent_name}/series/query", "store.read", h.QuerySeries)
}

// QuerySeries 는 구조화 시리즈 질의를 실행하고 평탄 응답을 반환한다.
//
// 응답은 기존 chartQueryResponse 를 그대로 재사용한다 — 신규 응답 타입을 만들지
// 않는다(§2.16 #23 · §4.3). 시리즈 구분은 entries[].labels 가 담는다.
func (h *InfluxDBSeriesHandler) QuerySeries(ctx api.Context) error {
	agentName := ctx.Param("agent_name")
	if agentName == "" {
		return api.ErrBadRequest.WithMessage("agent_name is required")
	}

	var req influxSeriesQueryRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	spec, err := buildSeriesQuerySpec(req)
	if err != nil {
		return api.ErrBadRequest.WithMessage(err.Error())
	}

	ag := findAgentByName(h.agents, agentName)
	if ag == nil {
		return api.ErrNotFound.
			WithMessage("agent_not_found: " + agentName).
			WithDetails(map[string]string{"error": "agent_not_found"})
	}

	seriesAgent, ok := ag.(influxSeriesQueryer)
	if !ok {
		return api.ErrBadRequest.
			WithMessage("not_an_influxdb_agent: " + agentName).
			WithDetails(map[string]string{"error": "not_an_influxdb_agent"})
	}

	// 쿼리 타임아웃은 원문 통과 경로와 같은 값을 쓴다.
	qctx, cancel := context.WithTimeout(ctx.Context(), defaultInfluxQueryTimeout)
	defer cancel()

	buckets, err := seriesAgent.QuerySeriesBuckets(qctx, spec)
	if err != nil {
		return mapInfluxSeriesError(err)
	}

	entries := buildInfluxSeriesEntries(buckets, req)
	resp := chartQueryResponse{
		Entries:   entries,
		Count:     len(entries),
		Truncated: false,
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(resp))
}

// buildSeriesQuerySpec 는 요청을 검증하고 도메인 spec 으로 변환한다.
//
// §2.6 의 검증 표 중 400 을 반환하는 7 종을 전부 여기서 소진한다. 실행 직전이
// 아니라 에이전트 조회 전에 검증하는 이유는, 잘못된 요청의 원인이 에이전트
// 존재 여부에 따라 달라지면 클라이언트가 고칠 곳을 찾기 어려워지기 때문이다.
func buildSeriesQuerySpec(req influxSeriesQueryRequest) (system.SeriesQuerySpec, error) {
	var spec system.SeriesQuerySpec

	if req.Measurement == "" {
		return spec, errors.New("measurement is required")
	}
	if req.Field == "" {
		// 폴백을 두지 않는다. "첫 번째 숫자 필드" 류의 추정은 조용한 오답이다(§2.16 #4).
		return spec, errors.New("field is required")
	}
	if req.IntervalMs <= 0 {
		return spec, errors.New("interval_ms must be > 0")
	}
	if req.EndMs <= req.StartMs {
		return spec, errors.New("end_ms must be greater than start_ms")
	}

	aggregation, err := parseSeriesAggregation(req.Aggregation)
	if err != nil {
		return spec, err
	}
	fill, err := parseSeriesFill(req.Fill)
	if err != nil {
		return spec, err
	}
	if err := validateSeriesBucketCount(req); err != nil {
		return spec, err
	}

	spec = system.SeriesQuerySpec{
		Bucket:      req.Bucket,
		Measurement: req.Measurement,
		Field:       req.Field,
		Tags:        req.Tags,
		StartMs:     req.StartMs,
		EndMs:       req.EndMs,
		IntervalMs:  req.IntervalMs,
		Aggregation: aggregation,
		Fill:        fill,
	}
	if err := spec.ValidateIdentifiers(); err != nil {
		return system.SeriesQuerySpec{}, err
	}
	return spec, nil
}

// parseSeriesAggregation 는 요청 어휘를 도메인 집계 연산으로 옮긴다.
//
// 5 종을 명시적으로 나열하고 default 에서 거부한다. map 리터럴이었다면 항목을
// 빠뜨려도 컴파일되고, 그 결과는 조용한 400 이 아니라 조용한 오답이다(§4.6).
func parseSeriesAggregation(v string) (system.SeriesAggregation, error) {
	switch v {
	case dto.SeriesAggregationMin:
		return system.SeriesAggMin, nil
	case dto.SeriesAggregationMax:
		return system.SeriesAggMax, nil
	case dto.SeriesAggregationAverage:
		return system.SeriesAggAverage, nil
	case dto.SeriesAggregationFirst:
		return system.SeriesAggFirst, nil
	case dto.SeriesAggregationLast:
		return system.SeriesAggLast, nil
	default:
		return system.SeriesAggMin, fmt.Errorf(
			"invalid aggregation: %q (expected one of: %s, %s, %s, %s, %s)",
			v,
			dto.SeriesAggregationMin, dto.SeriesAggregationMax, dto.SeriesAggregationAverage,
			dto.SeriesAggregationFirst, dto.SeriesAggregationLast)
	}
}

// parseSeriesFill 는 요청 어휘를 도메인 fill 전략으로 옮긴다.
//
// avg 는 프런트 계약에 남아 있는 값이지만 InfluxDB 양쪽 모두 대응물이 없다.
// 조용히 다른 전략으로 대체하지 않고 400 으로 거부한다(§2.16 #17).
func parseSeriesFill(v string) (system.SeriesFill, error) {
	switch v {
	case dto.SeriesFillNone:
		return system.SeriesFillNone, nil
	case dto.SeriesFillNull:
		return system.SeriesFillNull, nil
	case dto.SeriesFillZero:
		return system.SeriesFillZero, nil
	case dto.SeriesFillPrevious:
		return system.SeriesFillPrevious, nil
	case dto.SeriesFillAvg:
		return system.SeriesFillNone, fmt.Errorf(
			"invalid fill: %q is not supported by influxdb backends (expected one of: \"\", %s, %s, %s)",
			v, dto.SeriesFillNull, dto.SeriesFillZero, dto.SeriesFillPrevious)
	default:
		return system.SeriesFillNone, fmt.Errorf(
			"invalid fill: %q (expected one of: \"\", %s, %s, %s)",
			v, dto.SeriesFillNull, dto.SeriesFillZero, dto.SeriesFillPrevious)
	}
}

// validateSeriesBucketCount 는 버킷 수 상한을 강제한다(§2.9).
//
// 상한은 Store 와 같은 상수(maxAggregationBuckets) 를 쓴다. 값을 복제하면
// 소스를 갈아탄 사용자가 "왜 이건 되고 저건 안 되지"를 겪는다.
//
// 거부 메시지는 초과한 축과 실제 값 · 상한을 모두 담는다. "too many buckets" 만
// 으로는 시간창을 줄여야 할지 인터벌을 늘려야 할지 알 수 없다.
func validateSeriesBucketCount(req influxSeriesQueryRequest) error {
	span := req.EndMs - req.StartMs
	numBuckets := span / req.IntervalMs
	if span%req.IntervalMs != 0 {
		numBuckets++
	}
	if numBuckets > maxAggregationBuckets {
		return fmt.Errorf(
			"too many buckets: bucket_count=%d exceeds max=%d "+
				"(start_ms=%d, end_ms=%d, interval_ms=%d); "+
				"increase interval_ms or shrink the time window",
			numBuckets, maxAggregationBuckets,
			req.StartMs, req.EndMs, req.IntervalMs)
	}
	return nil
}

// mapInfluxSeriesError 는 에이전트 오류를 HTTP 오류로 매핑한다.
//
// 타임아웃/취소는 408 — 원문 통과 경로(influxdb_query.go)의 규약을 그대로
// 따른다. 이스케이프 불가 식별자와 미지원 fill 은 이미 요청 검증에서 400 으로
// 걸러지지만, 에이전트가 같은 판정을 다시 내릴 수 있으므로 여기서도 400 으로
// 매핑해 500 으로 새는 것을 막는다.
func mapInfluxSeriesError(err error) *api.APIError {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return api.ErrRequestTimeout.WithMessage(err.Error())
	case errors.Is(err, system.ErrUnescapableIdentifier),
		errors.Is(err, system.ErrUnsupportedSeriesFill):
		return api.ErrBadRequest.WithMessage(err.Error())
	default:
		return api.ErrInternalServer.WithMessage(err.Error())
	}
}

// buildInfluxSeriesEntries 는 버킷을 표준 평탄 엔트리로 조립한다(§2.6).
//
//   - timestamp = 버킷 시작 시각(epoch ms). 끝이 아니다(§2.8).
//   - labels = {"__field__": field, ...tags} — Store 와 동일한 예약 라벨 규약.
//
// measurement 를 labels 에 넣지 않는 이유는 요청 1건이 시리즈 1개를 처리하기
// 때문이다 — measurement 는 요청 축에 있고, 클라이언트는 요청 순서로 컬럼을
// 배치한다(§4.3).
func buildInfluxSeriesEntries(buckets []system.SeriesBucket, req influxSeriesQueryRequest) []chartQueryEntry {
	out := make([]chartQueryEntry, 0, len(buckets))
	for _, b := range buckets {
		labels := make(map[string]string, len(req.Tags)+1)
		labels[metricLabelKey] = req.Field
		for k, v := range req.Tags {
			labels[k] = v
		}
		out = append(out, chartQueryEntry{
			Timestamp: b.StartMs,
			Value:     b.Value,
			Labels:    labels,
		})
	}
	return out
}
