package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
)

// influxManager 는 InfluxDB 에이전트가 제공해야 하는 bucket/measurement 관리 계약이다.
// *system.InfluxDBAgent 가 이 인터페이스를 만족한다. 잘못된 타입의 에이전트가
// 관리 엔드포인트에 지정되면 400 으로 거부한다.
type influxManager interface {
	ListBuckets(ctx context.Context) ([]system.BucketInfo, error)
	CreateBucket(ctx context.Context, name string, retentionSeconds int64) (system.BucketInfo, error)
	DeleteBucket(ctx context.Context, bucketRef string) error
	TruncateBucket(ctx context.Context, bucketRef string) error
	ListMeasurements(ctx context.Context, bucket string) ([]string, error)
	DeleteMeasurement(ctx context.Context, bucket, measurement string) error
}

// defaultInfluxManagementTimeout 은 관리 조작 HTTP 요청의 기본 타임아웃이다.
// 대용량 truncate/delete 는 서버 측 삭제 작업이 길어질 수 있어 쿼리보다 넉넉하게 잡는다.
var defaultInfluxManagementTimeout = 60 * time.Second

// InfluxDBManagementHandler 는 InfluxDB bucket/measurement 관리 라우트를 제공한다.
//
// 라우트:
//
//	GET    /influxdb/{agent_name}/buckets
//	POST   /influxdb/{agent_name}/buckets
//	DELETE /influxdb/{agent_name}/buckets/{bucket}
//	POST   /influxdb/{agent_name}/buckets/{bucket}/truncate
//	GET    /influxdb/{agent_name}/measurements?bucket=
//	DELETE /influxdb/{agent_name}/measurements/{name}?bucket=
type InfluxDBManagementHandler struct {
	agents AgentLookup
	logger *slog.Logger
}

// NewInfluxDBManagementHandler 는 새 InfluxDBManagementHandler 를 생성한다.
func NewInfluxDBManagementHandler(agents AgentLookup, logger *slog.Logger) *InfluxDBManagementHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &InfluxDBManagementHandler{agents: agents, logger: logger}
}

// RegisterRoutes 는 InfluxDB 관리 라우트를 등록한다.
func (h *InfluxDBManagementHandler) RegisterRoutes(g *api.RouteGroup) {
	// @SPEC:SPEC-AUTH-005 (M5) — 카탈로그에 influxdb 리소스가 없어 store.* 로 매핑한다.
	// store 는 read/update 2종이므로 bucket/measurement 삭제도 update 이다.
	g.GETPerm("/influxdb/{agent_name}/buckets", "store.read", h.ListBuckets)
	g.POSTPerm("/influxdb/{agent_name}/buckets", "store.update", h.CreateBucket)
	g.DELETEPerm("/influxdb/{agent_name}/buckets/{bucket}", "store.update", h.DeleteBucket)
	g.POSTPerm("/influxdb/{agent_name}/buckets/{bucket}/truncate", "store.update", h.TruncateBucket)
	g.GETPerm("/influxdb/{agent_name}/measurements", "store.read", h.ListMeasurements)
	g.DELETEPerm("/influxdb/{agent_name}/measurements/{name}", "store.update", h.DeleteMeasurement)
}

// resolveManager 는 agent_name 으로 에이전트를 조회하고 influxManager 로 검증한다.
//   - 에이전트 없음: 404
//   - influxManager 미구현(잘못된 타입): 400
func (h *InfluxDBManagementHandler) resolveManager(ctx api.Context) (influxManager, *api.APIError) {
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

	mgr, ok := ag.(influxManager)
	if !ok {
		return nil, api.ErrBadRequest.
			WithMessage("not_an_influxdb_agent: " + agentName).
			WithDetails(map[string]string{"error": "not_an_influxdb_agent"})
	}
	return mgr, nil
}

// mapInfluxManagementError 는 관리 조작 에러를 적절한 HTTP 에러로 매핑한다.
//   - ErrManagementNotSupported → 501 Not Implemented
//   - context 타임아웃/취소 → 408 Request Timeout
//   - 그 외 → 502 Bad Gateway (업스트림 InfluxDB 조작 실패)
func mapInfluxManagementError(err error) *api.APIError {
	switch {
	case errors.Is(err, system.ErrManagementNotSupported):
		return api.ErrNotImplemented.
			WithMessage(err.Error()).
			WithDetails(map[string]string{"error": "management_not_supported"})
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return api.ErrRequestTimeout.WithMessage(err.Error())
	default:
		return api.ErrBadGateway.WithMessage(err.Error())
	}
}

// bucketInfoDTO 는 BucketInfo 의 HTTP 응답 표현이다.
type bucketInfoDTO struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	OrgID            string `json:"org_id"`
	RetentionSeconds int64  `json:"retention_seconds"`
}

func toBucketInfoDTO(b system.BucketInfo) bucketInfoDTO {
	return bucketInfoDTO{
		ID:               b.ID,
		Name:             b.Name,
		OrgID:            b.OrgID,
		RetentionSeconds: b.RetentionSeconds,
	}
}

// createBucketRequest 는 버킷 생성 요청 바디이다.
type createBucketRequest struct {
	Name             string `json:"name"`
	RetentionSeconds int64  `json:"retentionSeconds"`
}

// ListBuckets 는 GET /influxdb/{agent_name}/buckets 를 처리한다.
func (h *InfluxDBManagementHandler) ListBuckets(ctx api.Context) error {
	mgr, apiErr := h.resolveManager(ctx)
	if apiErr != nil {
		return apiErr
	}

	mctx, cancel := context.WithTimeout(ctx.Context(), defaultInfluxManagementTimeout)
	defer cancel()

	buckets, err := mgr.ListBuckets(mctx)
	if err != nil {
		return mapInfluxManagementError(err)
	}

	items := make([]bucketInfoDTO, 0, len(buckets))
	for _, b := range buckets {
		items = append(items, toBucketInfoDTO(b))
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"buckets": items,
		"count":   len(items),
	}))
}

// CreateBucket 은 POST /influxdb/{agent_name}/buckets 를 처리한다.
func (h *InfluxDBManagementHandler) CreateBucket(ctx api.Context) error {
	mgr, apiErr := h.resolveManager(ctx)
	if apiErr != nil {
		return apiErr
	}

	var req createBucketRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	if req.Name == "" {
		return api.ErrBadRequest.WithMessage("name is required")
	}
	if req.RetentionSeconds < 0 {
		return api.ErrBadRequest.WithMessage("retentionSeconds must be >= 0")
	}

	mctx, cancel := context.WithTimeout(ctx.Context(), defaultInfluxManagementTimeout)
	defer cancel()

	info, err := mgr.CreateBucket(mctx, req.Name, req.RetentionSeconds)
	if err != nil {
		return mapInfluxManagementError(err)
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toBucketInfoDTO(info)))
}

// DeleteBucket 은 DELETE /influxdb/{agent_name}/buckets/{bucket} 를 처리한다.
func (h *InfluxDBManagementHandler) DeleteBucket(ctx api.Context) error {
	mgr, apiErr := h.resolveManager(ctx)
	if apiErr != nil {
		return apiErr
	}

	bucket := ctx.Param("bucket")
	if bucket == "" {
		return api.ErrBadRequest.WithMessage("bucket is required")
	}

	mctx, cancel := context.WithTimeout(ctx.Context(), defaultInfluxManagementTimeout)
	defer cancel()

	if err := mgr.DeleteBucket(mctx, bucket); err != nil {
		return mapInfluxManagementError(err)
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"bucket":  bucket,
		"deleted": true,
	}))
}

// TruncateBucket 은 POST /influxdb/{agent_name}/buckets/{bucket}/truncate 를 처리한다.
func (h *InfluxDBManagementHandler) TruncateBucket(ctx api.Context) error {
	mgr, apiErr := h.resolveManager(ctx)
	if apiErr != nil {
		return apiErr
	}

	bucket := ctx.Param("bucket")
	if bucket == "" {
		return api.ErrBadRequest.WithMessage("bucket is required")
	}

	mctx, cancel := context.WithTimeout(ctx.Context(), defaultInfluxManagementTimeout)
	defer cancel()

	if err := mgr.TruncateBucket(mctx, bucket); err != nil {
		return mapInfluxManagementError(err)
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"bucket":    bucket,
		"truncated": true,
	}))
}

// ListMeasurements 는 GET /influxdb/{agent_name}/measurements?bucket= 를 처리한다.
// bucket 쿼리 파라미터를 생략하면 에이전트 설정의 기본 버킷을 사용한다.
func (h *InfluxDBManagementHandler) ListMeasurements(ctx api.Context) error {
	mgr, apiErr := h.resolveManager(ctx)
	if apiErr != nil {
		return apiErr
	}

	bucket := ctx.Query("bucket")

	mctx, cancel := context.WithTimeout(ctx.Context(), defaultInfluxManagementTimeout)
	defer cancel()

	measurements, err := mgr.ListMeasurements(mctx, bucket)
	if err != nil {
		return mapInfluxManagementError(err)
	}
	if measurements == nil {
		measurements = []string{}
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"measurements": measurements,
		"count":        len(measurements),
	}))
}

// DeleteMeasurement 는 DELETE /influxdb/{agent_name}/measurements/{name}?bucket= 를 처리한다.
// bucket 쿼리 파라미터를 생략하면 에이전트 설정의 기본 버킷을 사용한다.
func (h *InfluxDBManagementHandler) DeleteMeasurement(ctx api.Context) error {
	mgr, apiErr := h.resolveManager(ctx)
	if apiErr != nil {
		return apiErr
	}

	measurement := ctx.Param("name")
	if measurement == "" {
		return api.ErrBadRequest.WithMessage("measurement name is required")
	}
	bucket := ctx.Query("bucket")

	mctx, cancel := context.WithTimeout(ctx.Context(), defaultInfluxManagementTimeout)
	defer cancel()

	if err := mgr.DeleteMeasurement(mctx, bucket, measurement); err != nil {
		return mapInfluxManagementError(err)
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"measurement": measurement,
		"bucket":      bucket,
		"deleted":     true,
	}))
}
