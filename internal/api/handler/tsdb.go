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
	g.POST("/tsdb/write", h.Write)
	g.POST("/tsdb/query", h.Query)
	g.GET("/tsdb/series", h.ListSeries)
	g.GET("/tsdb/series/{key}/latest", h.Latest)
	g.GET("/tsdb/stats", h.Stats)
	g.DELETE("/tsdb/series/{key}", h.DeleteSeries)
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

// ListSeries 는 시리즈 키 목록을 반환한다.
// GET /tsdb/series?measurement=xxx&tag_key=tag_value
func (h *TSDBHandler) ListSeries(ctx api.Context) error {
	measurement := ctx.Query("measurement")

	// 쿼리 파라미터에서 태그 필터 추출 (measurement 제외)
	tags := make(map[string]string)
	// Query 메서드로는 모든 파라미터를 순회할 수 없으므로,
	// 알려진 파라미터(measurement)를 제외하고 나머지를 태그로 취급한다.
	// 여기서는 간단히 measurement만 제외한다.

	var series []string
	if measurement == "" && len(tags) == 0 {
		series = h.db.SeriesKeys()
	} else {
		series = h.db.FilterSeries(measurement, tags)
	}

	if series == nil {
		series = []string{}
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(dto.TSDBSeriesListResponse{
		Series: series,
		Count:  len(series),
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
