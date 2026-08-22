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

// influxSchemaDiscoverer 는 InfluxDB 에이전트가 제공해야 하는 스키마 디스커버리
// 계약이다. *system.InfluxDBAgent 가 이 인터페이스를 만족한다.
//
// @spec SPEC-TSDB-002 §2.10 (U10)
//
// influxManager 와 합치지 않는 이유는 권한 축이 다르기 때문이다 — 디스커버리는
// 읽기 전용(store.read)이고 관리 조작은 쓰기(store.update)를 포함한다. 하나로
// 합치면 읽기 전용 라우트가 쓰기 능력을 갖춘 에이전트만 받게 된다.
type influxSchemaDiscoverer interface {
	ListTagKeys(ctx context.Context, bucket, measurement string) ([]string, error)
	ListTagValues(ctx context.Context, bucket, measurement, tagKey string) ([]string, error)
	ListFieldKeys(ctx context.Context, bucket, measurement string) ([]string, error)
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
//
// 스키마 디스커버리 라우트(§2.10 D2~D4, 전부 읽기 전용):
//
//	GET    /influxdb/{agent_name}/tag-keys?measurement=&bucket=
//	GET    /influxdb/{agent_name}/tag-values?measurement=&tag_key=&bucket=
//	GET    /influxdb/{agent_name}/field-keys?measurement=&bucket=
//
// D1(measurement 목록)은 위 /measurements 라우트가 겸한다 — 신규 라우트를 만들지
// 않는다. 같은 목록을 두 경로가 돌려주면 어느 쪽이 정본인지 알 수 없게 된다.
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

	// @spec SPEC-TSDB-002 §2.10 (U10) — 스키마 디스커버리 D2~D4.
	// 전부 조회이므로 store.read 이다(D1 /measurements 와 같은 권한).
	g.GETPerm("/influxdb/{agent_name}/tag-keys", "store.read", h.ListTagKeys)
	g.GETPerm("/influxdb/{agent_name}/tag-values", "store.read", h.ListTagValues)
	g.GETPerm("/influxdb/{agent_name}/field-keys", "store.read", h.ListFieldKeys)
}

// resolveDiscoverer 는 agent_name 으로 에이전트를 조회하고 influxSchemaDiscoverer
// 로 검증한다. 판정 실패의 처분은 resolveManager 와 같다(404 / 400).
func (h *InfluxDBManagementHandler) resolveDiscoverer(ctx api.Context) (influxSchemaDiscoverer, *api.APIError) {
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

	disc, ok := ag.(influxSchemaDiscoverer)
	if !ok {
		return nil, api.ErrBadRequest.
			WithMessage("not_an_influxdb_agent: " + agentName).
			WithDetails(map[string]string{"error": "not_an_influxdb_agent"})
	}
	return disc, nil
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

// --- 스키마 디스커버리 (@spec SPEC-TSDB-002 §2.10 D2~D4) ---
//
// 세 핸들러 모두 읽기 전용이며 응답을 캐시하지 않는다(UB1-12). 스키마는 쓰기에
// 따라 변하므로, 오래된 목록에서 고른 시리즈는 조회 시 빈 결과가 된다.

// mapInfluxDiscoveryError 는 디스커버리 오류를 HTTP 오류로 매핑한다.
//
// 이스케이프 불가 식별자는 400 이다 — 사용자가 고칠 수 있는 입력 오류를 502 로
// 보내면 클라이언트가 업스트림 장애로 오독한다. 나머지는 관리 조작과 같은
// 매핑(501 / 408 / 502)을 쓴다.
func mapInfluxDiscoveryError(err error) *api.APIError {
	if errors.Is(err, system.ErrUnescapableIdentifier) {
		return api.ErrBadRequest.WithMessage(err.Error())
	}
	return mapInfluxManagementError(err)
}

// ListTagKeys 는 GET /influxdb/{agent_name}/tag-keys?measurement=&bucket= 를 처리한다(D2).
func (h *InfluxDBManagementHandler) ListTagKeys(ctx api.Context) error {
	disc, apiErr := h.resolveDiscoverer(ctx)
	if apiErr != nil {
		return apiErr
	}

	measurement := ctx.Query("measurement")
	if measurement == "" {
		return api.ErrBadRequest.WithMessage("measurement is required")
	}

	mctx, cancel := context.WithTimeout(ctx.Context(), defaultInfluxManagementTimeout)
	defer cancel()

	keys, err := disc.ListTagKeys(mctx, ctx.Query("bucket"), measurement)
	if err != nil {
		return mapInfluxDiscoveryError(err)
	}
	if keys == nil {
		keys = []string{}
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"tag_keys": keys,
		"count":    len(keys),
	}))
}

// ListTagValues 는 GET /influxdb/{agent_name}/tag-values?measurement=&tag_key=&bucket=
// 를 처리한다(D3).
func (h *InfluxDBManagementHandler) ListTagValues(ctx api.Context) error {
	disc, apiErr := h.resolveDiscoverer(ctx)
	if apiErr != nil {
		return apiErr
	}

	measurement := ctx.Query("measurement")
	if measurement == "" {
		return api.ErrBadRequest.WithMessage("measurement is required")
	}
	tagKey := ctx.Query("tag_key")
	if tagKey == "" {
		// 태그 키 없이 값 목록을 돌려주면 어느 키의 값인지 알 수 없다. 폴백을
		// 두지 않고 거부한다.
		return api.ErrBadRequest.WithMessage("tag_key is required")
	}

	mctx, cancel := context.WithTimeout(ctx.Context(), defaultInfluxManagementTimeout)
	defer cancel()

	values, err := disc.ListTagValues(mctx, ctx.Query("bucket"), measurement, tagKey)
	if err != nil {
		return mapInfluxDiscoveryError(err)
	}
	if values == nil {
		values = []string{}
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"tag_values": values,
		"count":      len(values),
	}))
}

// ListFieldKeys 는 GET /influxdb/{agent_name}/field-keys?measurement=&bucket= 를 처리한다(D4).
func (h *InfluxDBManagementHandler) ListFieldKeys(ctx api.Context) error {
	disc, apiErr := h.resolveDiscoverer(ctx)
	if apiErr != nil {
		return apiErr
	}

	measurement := ctx.Query("measurement")
	if measurement == "" {
		return api.ErrBadRequest.WithMessage("measurement is required")
	}

	mctx, cancel := context.WithTimeout(ctx.Context(), defaultInfluxManagementTimeout)
	defer cancel()

	fields, err := disc.ListFieldKeys(mctx, ctx.Query("bucket"), measurement)
	if err != nil {
		return mapInfluxDiscoveryError(err)
	}
	if fields == nil {
		fields = []string{}
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"field_keys": fields,
		"count":      len(fields),
	}))
}
