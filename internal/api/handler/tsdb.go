package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/ws"
	"github.com/xtra/xflow/internal/tsdb"
)

// TSDBHandler 는 TSDB REST API 요청을 처리하는 핸들러이다.
type TSDBHandler struct {
	db     tsdb.TSDB
	hub    *ws.Hub // WebSocket 허브 (구독 관리, nil 허용)
	logger *slog.Logger
}

// TSDBHandlerOption 은 TSDBHandler 의 선택적 설정 함수이다.
type TSDBHandlerOption func(*TSDBHandler)

// WithHub 는 TSDBHandler 에 WebSocket Hub 를 설정한다.
func WithHub(hub *ws.Hub) TSDBHandlerOption {
	return func(h *TSDBHandler) {
		h.hub = hub
	}
}

// NewTSDBHandler 는 새 TSDBHandler를 생성한다.
func NewTSDBHandler(db tsdb.TSDB, logger *slog.Logger, opts ...TSDBHandlerOption) *TSDBHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &TSDBHandler{
		db:     db,
		logger: logger,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// RegisterRoutes 는 주어진 라우트 그룹에 TSDB 라우트를 등록한다.
//
// Routes:
//
//	POST   /tsdb/write              -> Write
//	POST   /tsdb/query              -> Query
//	GET    /tsdb/series             -> ListSeries
//	GET    /tsdb/series/{key}/latest -> Latest
//	GET    /tsdb/stats              -> Stats
//	DELETE /tsdb/series/{key}       -> DeleteSeries
func (h *TSDBHandler) RegisterRoutes(g *api.RouteGroup) {
	// @SPEC:SPEC-AUTH-005 (M5) — 카탈로그에 tsdb 리소스가 없어 시계열 데이터 저장소로
	// 가장 가까운 store.* 로 매핑한다 (read/update 2종이므로 삭제도 update).
	g.POSTPerm("/tsdb/write", "store.update", h.Write)
	g.POSTPerm("/tsdb/query", "store.read", h.Query)
	g.GETPerm("/tsdb/series", "store.read", h.ListSeries)
	g.GETPerm("/tsdb/series/{key}/latest", "store.read", h.Latest)
	g.GETPerm("/tsdb/stats", "store.read", h.Stats)
	g.DELETEPerm("/tsdb/series/{key}", "store.update", h.DeleteSeries)
}

// Write 는 TSDB에 데이터 포인트를 기록한다.
// POST /tsdb/write
func (h *TSDBHandler) Write(ctx api.Context) error {
	var req dto.TSDBWriteRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	if len(req.Points) == 0 {
		return api.ErrBadRequest.WithMessage("points array is required and must not be empty")
	}

	// DTO -> tsdb.WriteRequest 변환
	points := make([]tsdb.WriteRequest, 0, len(req.Points))
	for i, p := range req.Points {
		if p.Measurement == "" {
			return api.ErrBadRequest.WithMessage(
				fmt.Sprintf("points[%d].measurement is required", i))
		}
		if len(p.Fields) == 0 {
			return api.ErrBadRequest.WithMessage(
				fmt.Sprintf("points[%d].fields is required", i))
		}

		wr := tsdb.WriteRequest{
			Measurement: p.Measurement,
			Tags:        p.Tags,
			Fields:      p.Fields,
		}

		if p.Timestamp != nil {
			ts, err := parseTimestamp(*p.Timestamp)
			if err != nil {
				return api.ErrBadRequest.WithMessage(
					fmt.Sprintf("points[%d].timestamp: %s", i, err.Error()))
			}
			wr.Timestamp = ts
		}

		points = append(points, wr)
	}

	if err := h.db.WriteBatch(points); err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusCreated, dto.NewSuccessResponse(dto.TSDBWriteResponse{
		Written: len(points),
	}))
}

// Query 는 TSDB 쿼리를 실행한다.
// POST /tsdb/query
func (h *TSDBHandler) Query(ctx api.Context) error {
	var req dto.TSDBQueryRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	q := tsdb.Query{
		SeriesKey:   req.SeriesKey,
		Measurement: req.Measurement,
		TagFilters:  req.Tags,
		Field:       req.Field,
		Limit:       req.Limit,
	}

	// 집계 함수 변환
	if req.Aggregation != "" {
		q.Aggregation = tsdb.AggregateFunc(req.Aggregation)
	}

	// 시작 시간 파싱
	if req.Start != nil {
		t, err := time.Parse(time.RFC3339Nano, *req.Start)
		if err != nil {
			return api.ErrBadRequest.WithMessage("invalid start time: " + err.Error())
		}
		q.Start = t
	}

	// 종료 시간 파싱
	if req.End != nil {
		t, err := time.Parse(time.RFC3339Nano, *req.End)
		if err != nil {
			return api.ErrBadRequest.WithMessage("invalid end time: " + err.Error())
		}
		q.End = t
	}

	// 버킷 간격 파싱
	if req.Bucket != "" {
		d, err := time.ParseDuration(req.Bucket)
		if err != nil {
			return api.ErrBadRequest.WithMessage("invalid bucket duration: " + err.Error())
		}
		q.BucketInterval = d
	}

	// 빈 버킷 채우기 전략 (버킷 다운샘플링 시에만 의미)
	switch tsdb.FillStrategy(req.Fill) {
	case tsdb.FillNone, tsdb.FillNull, tsdb.FillZero, tsdb.FillPrevious, tsdb.FillAvg:
		q.Fill = tsdb.FillStrategy(req.Fill)
	default:
		return api.ErrBadRequest.WithMessage("invalid fill strategy: " + req.Fill + " (allowed: null, zero, previous, avg)")
	}

	// 버킷 수 상한 검사 — 반드시 Execute 전에 한다.
	// Execute 안의 상한(ErrMaxPointsExceeded)은 다운샘플링 *결과*를 보므로,
	// 그 시점엔 이미 구간 전체의 원본 포인트를 읽어 복사한 뒤다. 과대 요청은
	// 스캔이 시작되기 전에 끊어야 비용이 발생하지 않는다.
	if err := validateTSDBBucketCount(q); err != nil {
		return api.ErrBadRequest.WithMessage(err.Error())
	}

	results, err := h.db.Execute(q)
	if err != nil {
		return api.MapDomainError(err)
	}

	// tsdb.QueryResult -> dto 변환
	resp := dto.TSDBQueryResponse{
		Results: make([]dto.TSDBSeriesResult, 0, len(results)),
	}

	for _, r := range results {
		sr := dto.TSDBSeriesResult{
			SeriesKey: r.SeriesKey,
			Points:    make([]dto.TSDBDataPoint, 0, len(r.Points)),
			Stats: &dto.TSDBQueryStats{
				ScannedPoints:  r.Stats.ScannedPoints,
				ReturnedPoints: r.Stats.ReturnedPoints,
				ExecutionTime:  r.Stats.ExecutionTime.String(),
			},
		}

		for _, dp := range r.Points {
			sr.Points = append(sr.Points, dto.TSDBDataPoint{
				Timestamp: dp.Timestamp.Format(time.RFC3339Nano),
				Fields:    dp.Fields,
			})
		}

		resp.Results = append(resp.Results, sr)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(resp))
}

// 페이지네이션 관련 상수 (SPEC-WEB-005)
// @spec SPEC-WEB-005
const (
	tsdbSeriesDefaultPageSize = 25  // size 미지정 또는 0 일 때 사용하는 기본 페이지 크기
	tsdbSeriesMaxPageSize     = 100 // size 파라미터 상한 — 초과 시 이 값으로 클램프
)

// validateTSDBBucketCount 는 조회 구간과 버킷 간격으로 만들어질 버킷 수가
// 상한을 넘는지 검사한다.
//
// Store(InfluxDB) 경로의 validateSeriesBucketCount 와 같은 판정을 내장 TSDB
// 경로에도 적용해 두 경로를 대칭으로 만든다. 상한 상수(maxAggregationBuckets)도
// 공유한다 — 값을 복제하면 한쪽만 조정되어 조용히 어긋난다.
//
// 구간이 열려 있거나(start·end 미지정) 버킷 간격이 없으면(원본 반환) 버킷 수를
// 셀 수 없으므로 통과시킨다. 이 두 경우의 방어는 스캔 상한 소관이다.
func validateTSDBBucketCount(q tsdb.Query) error {
	if q.BucketInterval <= 0 || q.Start.IsZero() || q.End.IsZero() {
		return nil
	}
	span := q.End.Sub(q.Start)
	if span <= 0 {
		return nil
	}

	numBuckets := int64(span / q.BucketInterval)
	if span%q.BucketInterval != 0 {
		numBuckets++
	}
	if numBuckets > maxAggregationBuckets {
		return fmt.Errorf(
			"too many buckets: bucket_count=%d exceeds max=%d "+
				"(start=%s, end=%s, bucket=%s); "+
				"increase bucket or shrink the time window",
			numBuckets, maxAggregationBuckets,
			q.Start.Format(time.RFC3339Nano), q.End.Format(time.RFC3339Nano),
			q.BucketInterval)
	}
	return nil
}

// ListSeries 는 시리즈 키 목록을 반환한다.
//
// GET /tsdb/series?measurement=xxx&tag_key=tag_value
//
// SPEC-WEB-005: optional 페이지네이션 파라미터 추가
//   - page:     1 이상 정수. 기본 1. 0 또는 음수는 1 로 클램프.
//   - size:     페이지 크기. 기본 25. 0 이면 기본값. 100 초과 시 100 으로 클램프.
//   - agent_id: 선택. 향후 멀티 인스턴스 TSDB 라우팅용 식별자. 현재는 파싱만 하고
//     싱글톤 인스턴스로 라우팅한다.
//
// 하위 호환성: page 와 size 가 모두 부재하면 기존 응답 포맷 ({series, count}) 을
// 그대로 유지하며 pagination 필드는 JSON 에 포함되지 않는다 (omitempty).
// 둘 중 하나라도 존재하면 페이지네이션이 활성화되어 pagination 메타가 포함된다.
//
// @spec SPEC-WEB-005
func (h *TSDBHandler) ListSeries(ctx api.Context) error {
	measurement := ctx.Query("measurement")

	// 쿼리 파라미터에서 태그 필터 추출 (measurement 제외)
	tags := make(map[string]string)
	// Query 메서드로는 모든 파라미터를 순회할 수 없으므로,
	// 알려진 파라미터(measurement)를 제외하고 나머지를 태그로 취급한다.
	// 여기서는 간단히 measurement만 제외한다.

	// --- SPEC-WEB-005: 페이지네이션 파라미터 파싱 ---
	pageStr := ctx.Query("page")
	sizeStr := ctx.Query("size")
	// agent_id 는 현재 싱글톤 라우팅이므로 파싱만 하고 무시한다.
	// TODO(SPEC-WEB-005): 향후 멀티 인스턴스 TSDB 확장 시 레지스트리에서
	//                    agent_id 로 특정 인스턴스를 조회하여 라우팅한다.
	_ = ctx.Query("agent_id")

	paginationRequested := pageStr != "" || sizeStr != ""

	var (
		page int
		size int
	)
	if paginationRequested {
		// page 파싱 및 클램프
		if pageStr != "" {
			parsed, err := strconv.Atoi(pageStr)
			if err != nil {
				return api.ErrBadRequest.WithMessage("invalid page parameter: must be a positive integer")
			}
			page = parsed
		} else {
			page = 1
		}
		if page < 1 {
			page = 1
		}

		// size 파싱, 기본값 적용, 상한 클램프
		if sizeStr != "" {
			parsed, err := strconv.Atoi(sizeStr)
			if err != nil {
				return api.ErrBadRequest.WithMessage("invalid size parameter: must be a positive integer")
			}
			size = parsed
		}
		if size <= 0 {
			size = tsdbSeriesDefaultPageSize
		}
		if size > tsdbSeriesMaxPageSize {
			size = tsdbSeriesMaxPageSize
		}
	}

	// --- 기존 시리즈 조회 로직 (변경 없음) ---
	var series []string
	if measurement == "" && len(tags) == 0 {
		series = h.db.SeriesKeys()
	} else {
		series = h.db.FilterSeries(measurement, tags)
	}

	if series == nil {
		series = []string{}
	}

	// 페이지네이션 미요청: 기존 응답 포맷 그대로 반환
	if !paginationRequested {
		return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(dto.TSDBSeriesListResponse{
			Series: series,
			Count:  len(series),
		}))
	}

	// --- 페이지네이션 적용 ---
	total := len(series)
	totalPages := 0
	if size > 0 && total > 0 {
		totalPages = (total + size - 1) / size // ceil(total / size)
	}

	offset := (page - 1) * size
	end := offset + size

	var pageSeries []string
	switch {
	case offset >= total:
		// 오프셋이 범위를 초과하면 빈 페이지 반환
		pageSeries = []string{}
	case end > total:
		pageSeries = series[offset:total]
	default:
		pageSeries = series[offset:end]
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(dto.TSDBSeriesListResponse{
		Series: pageSeries,
		Count:  len(pageSeries),
		Pagination: &dto.PaginationMeta{
			Page:       page,
			Size:       size,
			Total:      int64(total),
			TotalPages: totalPages,
		},
	}))
}

// Latest 는 시리즈의 최신 데이터 포인트를 반환한다.
// GET /tsdb/series/{key}/latest?n=10
func (h *TSDBHandler) Latest(ctx api.Context) error {
	rawKey := ctx.Param("key")
	if rawKey == "" {
		return api.ErrBadRequest.WithMessage("series key is required")
	}

	// URL 디코딩 (시리즈 키에 쉼표, 등호 포함)
	key, err := url.PathUnescape(rawKey)
	if err != nil {
		return api.ErrBadRequest.WithMessage("invalid series key encoding")
	}

	n := 1
	if nStr := ctx.Query("n"); nStr != "" {
		parsed, err := strconv.Atoi(nStr)
		if err != nil || parsed < 1 {
			return api.ErrBadRequest.WithMessage("n must be a positive integer")
		}
		n = parsed
	}

	points, err := h.db.Latest(key, n)
	if err != nil {
		return api.MapDomainError(err)
	}

	// tsdb.DataPoint -> dto 변환
	dtoPoints := make([]dto.TSDBDataPoint, 0, len(points))
	for _, dp := range points {
		dtoPoints = append(dtoPoints, dto.TSDBDataPoint{
			Timestamp: dp.Timestamp.Format(time.RFC3339Nano),
			Fields:    dp.Fields,
		})
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(dtoPoints))
}

// Stats 는 TSDB 통계를 반환한다.
// GET /tsdb/stats
func (h *TSDBHandler) Stats(ctx api.Context) error {
	stats := h.db.Stats()

	resp := dto.TSDBStatsResponse{
		SeriesCount: stats.SeriesCount,
		TotalPoints: stats.TotalPoints,
		MemoryBytes: stats.MemoryBytes,
		MemoryHuman: formatBytes(stats.MemoryBytes),
	}

	if !stats.OldestPoint.IsZero() {
		resp.OldestPoint = stats.OldestPoint.Format(time.RFC3339Nano)
	}
	if !stats.NewestPoint.IsZero() {
		resp.NewestPoint = stats.NewestPoint.Format(time.RFC3339Nano)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(resp))
}

// DeleteSeries 는 시리즈를 삭제한다.
// DELETE /tsdb/series/{key}
func (h *TSDBHandler) DeleteSeries(ctx api.Context) error {
	rawKey := ctx.Param("key")
	if rawKey == "" {
		return api.ErrBadRequest.WithMessage("series key is required")
	}

	key, err := url.PathUnescape(rawKey)
	if err != nil {
		return api.ErrBadRequest.WithMessage("invalid series key encoding")
	}

	if err := h.db.DeleteSeries(key); err != nil {
		return api.MapDomainError(err)
	}

	return ctx.NoContent(http.StatusNoContent)
}

// parseTimestamp 는 RFC3339Nano 또는 unix 나노초 문자열을 time.Time으로 파싱한다.
func parseTimestamp(s string) (time.Time, error) {
	// RFC3339Nano 먼저 시도
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}

	// unix 나노초 시도
	nanos, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timestamp format: use RFC3339Nano or unix nanoseconds")
	}
	return time.Unix(0, nanos), nil
}

// formatBytes 는 바이트 수를 사람이 읽을 수 있는 형식으로 변환한다.
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
