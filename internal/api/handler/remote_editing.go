// remote_editing.go 는 M7 원격 자원 편집 REST API(승인·온라인 노드의 플로우/에이전트
// FULL CRUD)를 제공한다(@SPEC:SPEC-REMOTE-001 M7, 그룹 I, REQ-I01~I06/I11/I12, E08/A4).
//
// 모든 엔드포인트는 admin 권한을 강제한다(REQ-F04). 편집은 서버 단독 영속을 절대 하지
// 않는다(A4/E08): 그룹 D 명령(Dispatch)으로 (온라인) 노드에 전파하고, 노드 어댑터
// 적용 결과를 받은 후에만 미러 캐시를 갱신한다.
//
// 라우트(api/v1 그룹 하위, remote_admin.go 의 GET 목록과 공존):
//
//	POST   /remote/nodes/{instance_id}/flows             — 플로우 생성(REQ-I01) → 201
//	PATCH  /remote/nodes/{instance_id}/flows/{flow_id}   — 플로우 수정(REQ-I02) → 200
//	DELETE /remote/nodes/{instance_id}/flows/{flow_id}   — 플로우 삭제(REQ-I03) → 204
//	POST   /remote/nodes/{instance_id}/agents            — 에이전트 생성(REQ-I04) → 201
//	PATCH  /remote/nodes/{instance_id}/agents/{agent_id} — 에이전트 수정(REQ-I04) → 200
//	DELETE /remote/nodes/{instance_id}/agents/{agent_id} — 에이전트 삭제(REQ-I04) → 204
//
// 실패 의미(REQ-I11): 오프라인/미관리 → 503, 명령 타임아웃 → 504, 노드 적용 실패 →
// 502(remote_admin.go 의 mapRemoteCommandError 재사용). 어떤 실패에서도 미러 캐시는
// 갱신되지 않는다(성공 결과 수신 시에만 — REQ-E08).
//
// 감사(REQ-I12): 누가(actor)/언제/어느 노드/도메인·액션/결과는 Dispatch 경로의
// recordCommandAudit 가 영속한다(중복 방지 — 명령 감사는 서버 측 1곳). 핸들러는 actor
// 를 ContextWithActor 로 전달하고 자원 id 를 구조화 로그로 남긴다. 시크릿은 기록하지
// 않는다(REQ-F06).
package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/remote"
	"github.com/xtra/xflow/internal/storage"
)

// RemoteEditService 는 원격 편집 오케스트레이션을 추상화한다(*remote.Server 가 만족).
//
// 핸들러는 Dispatch(명령 전파 + 게이팅/타임아웃/실패/감사) → 성공 시 미러 mutator
// (UpsertMirror/DeleteMirror) 순으로 호출한다. IsResourceExposed 는 update/delete 의
// 노출 범위(REQ-I05) 사전 게이트이다.
type RemoteEditService interface {
	// Dispatch 는 승인+온라인 노드에 명령을 전파하고 결과를 기다린다(REQ-D01/D05~D08).
	// 미관리/오프라인 → remote.ErrNodeNotManaged/ErrNoConn, 타임아웃 → ErrCommandTimeout,
	// 노드 적용 실패 → ErrCommandFailed. actor 는 ctx(ContextWithActor)로 전달된다.
	Dispatch(ctx context.Context, instanceID, domain, action string, args json.RawMessage) (json.RawMessage, error)
	// IsResourceExposed 는 kind(flow|agent) 자원 id 가 노드의 노출 범위 내(미러 존재)
	// 인지 반환한다(REQ-I05/E07).
	IsResourceExposed(ctx context.Context, instanceID, kind, id string) (bool, error)
	// UpsertMirror 는 명령 성공 후 단일 미러 행을 추가/갱신한다(REQ-E08). item.Definition
	// 은 호출자가 redaction(F06)한 정의여야 한다.
	UpsertMirror(ctx context.Context, kind string, item storage.MirroredResource) error
	// DeleteMirror 는 명령 성공 후 단일 미러 행을 제거한다(REQ-E08).
	DeleteMirror(ctx context.Context, instanceID, kind, id string) error
}

// RemoteEditHandler 는 원격 플로우/에이전트 편집 엔드포인트를 처리한다.
type RemoteEditHandler struct {
	svc    RemoteEditService
	logger *slog.Logger
}

// NewRemoteEditHandler 는 RemoteEditHandler 를 생성한다.
func NewRemoteEditHandler(svc RemoteEditService, logger *slog.Logger) *RemoteEditHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &RemoteEditHandler{svc: svc, logger: logger}
}

// RegisterRoutes 는 원격 편집 라우트를 그룹에 등록한다(remote_admin 의 GET 목록과 공존).
func (h *RemoteEditHandler) RegisterRoutes(g *api.RouteGroup) {
	g.POST("/remote/nodes/{instance_id}/flows", h.CreateFlow)
	g.PATCH("/remote/nodes/{instance_id}/flows/{flow_id}", h.UpdateFlow)
	g.DELETE("/remote/nodes/{instance_id}/flows/{flow_id}", h.DeleteFlow)
	g.POST("/remote/nodes/{instance_id}/agents", h.CreateAgent)
	g.PATCH("/remote/nodes/{instance_id}/agents/{agent_id}", h.UpdateAgent)
	g.DELETE("/remote/nodes/{instance_id}/agents/{agent_id}", h.DeleteAgent)
}

// nodeResult 는 노드 어댑터(FlowServiceAdapter/AgentServiceAdapter)가 반환하는 결과의
// 공통 최소 형태이다(node-assigned id + name/status). create 의 채번 id 추출과 update
// 의 미러 행 구성에 사용한다. Config 는 미러에 쓰기 전 redaction 한다(REQ-F06).
type nodeResult struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Status string         `json:"status"`
	Config map[string]any `json:"config"`
}

// --- 플로우 -----------------------------------------------------------------

// CreateFlow 는 노드에 새 플로우를 생성한다(REQ-I01). POST .../flows
//
// 노드 어댑터 Create 가 ID 를 채번·반환한다(node-assigned — OPEN QUESTION 8). 신규
// 자원은 자동 노출되지 않으므로(opt-in 보존 — OPEN QUESTION 9) 미러를 강제로 채우지
// 않는다(노드가 노출 설정을 갱신해 push 할 때 미러에 나타난다).
func (h *RemoteEditHandler) CreateFlow(ctx api.Context) error {
	return h.create(ctx, remote.DomainFlow)
}

// UpdateFlow 는 노드의 기존 플로우를 수정한다(REQ-I02). PATCH .../flows/{flow_id}
func (h *RemoteEditHandler) UpdateFlow(ctx api.Context) error {
	return h.update(ctx, remote.DomainFlow, ctx.Param("flow_id"))
}

// DeleteFlow 는 노드의 플로우를 삭제한다(REQ-I03). DELETE .../flows/{flow_id}
func (h *RemoteEditHandler) DeleteFlow(ctx api.Context) error {
	return h.delete(ctx, remote.DomainFlow, ctx.Param("flow_id"))
}

// --- 에이전트 -----------------------------------------------------------------

// CreateAgent 는 노드에 새 에이전트를 생성한다(REQ-I04). POST .../agents
func (h *RemoteEditHandler) CreateAgent(ctx api.Context) error {
	return h.create(ctx, remote.DomainAgent)
}

// UpdateAgent 는 노드의 기존 에이전트를 수정한다(REQ-I04). PATCH .../agents/{agent_id}
func (h *RemoteEditHandler) UpdateAgent(ctx api.Context) error {
	return h.update(ctx, remote.DomainAgent, ctx.Param("agent_id"))
}

// DeleteAgent 는 노드의 에이전트를 삭제한다(REQ-I04). DELETE .../agents/{agent_id}
func (h *RemoteEditHandler) DeleteAgent(ctx api.Context) error {
	return h.delete(ctx, remote.DomainAgent, ctx.Param("agent_id"))
}

// --- 공통 오케스트레이션 -------------------------------------------------------

// create 는 생성 명령을 전파하고 노드 채번 결과를 201 로 반환한다(REQ-I01/I04).
// 본문은 자원 정의 JSON 이며 그대로 명령 args 로 전달한다(어댑터가 dto 로 디코드 — A5).
func (h *RemoteEditHandler) create(ctx api.Context, domain string) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	instanceID := ctx.Param("instance_id")
	var args json.RawMessage
	if err := ctx.Bind(&args); err != nil {
		return api.ErrBadRequest.WithMessage("자원 정의 본문은 필수입니다")
	}
	if len(args) == 0 {
		return api.ErrBadRequest.WithMessage("자원 정의 본문은 필수입니다")
	}

	h.logger.Info("원격 자원 생성", "instance_id", instanceID, "domain", domain, "actor", ctx.UserID())

	result, dispErr := h.dispatch(ctx, instanceID, domain, remote.ActionCreate, args)
	if dispErr != nil {
		return mapRemoteCommandError(dispErr)
	}
	// 생성 성공 — 노드 채번 id 를 응답한다. 미러는 강제로 채우지 않는다(opt-in 보존).
	return ctx.JSON(http.StatusCreated, dto.NewSuccessResponse(decodeNodeResult(result)))
}

// update 는 수정 명령을 전파하고 성공 시 미러 행을 upsert 한다(REQ-I02/I04/E08).
//
// 노출 범위(REQ-I05): 대상 자원이 노드의 미러(=노출 범위)에 없으면 404 로 거부하고
// 명령을 디스패치하지 않는다. 본문에 id 를 주입해 명령 args 로 전달한다.
func (h *RemoteEditHandler) update(ctx api.Context, domain, resourceID string) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	if resourceID == "" {
		return api.ErrBadRequest.WithMessage("자원 id 는 필수입니다")
	}
	instanceID := ctx.Param("instance_id")

	// 노출 범위 게이트(REQ-I05/E07): 범위 밖이면 디스패치하지 않고 404.
	if err := h.requireExposed(ctx, instanceID, domain, resourceID); err != nil {
		return err
	}

	// 갱신 정의(JSON 객체)를 디코드하고 path 의 자원 id 를 주입한다(어댑터 update 가
	// id + 갱신 정의를 함께 디코드 — REQ-I06). 마스킹/미변경 시크릿 필드는 본문에서
	// 생략되어 있으며, 노드가 기존값으로 backfill 한다(REQ-I07 — client/apply 측 병합).
	body := map[string]any{}
	if err := ctx.Bind(&body); err != nil {
		return api.ErrBadRequest.WithMessage("갱신 정의 본문이 올바르지 않습니다")
	}
	body["id"] = resourceID
	args, err := json.Marshal(body)
	if err != nil {
		return api.ErrBadRequest.WithMessage(err.Error())
	}

	h.logger.Info("원격 자원 수정",
		"instance_id", instanceID, "domain", domain, "resource_id", resourceID, "actor", ctx.UserID())

	result, dispErr := h.dispatch(ctx, instanceID, domain, remote.ActionUpdate, args)
	if dispErr != nil {
		return mapRemoteCommandError(dispErr)
	}

	// 성공 — 노드 반환 정의로 미러 행을 갱신한다(REQ-E08). Config 는 redaction(F06).
	nr := decodeNodeResult(result)
	if mirrorErr := h.svc.UpsertMirror(ctx.Context(), kindFor(domain), mirrorRowFromResult(instanceID, domain, resourceID, nr)); mirrorErr != nil {
		h.logger.Warn("원격 수정 후 미러 갱신 실패",
			"instance_id", instanceID, "domain", domain, "resource_id", resourceID, "error", mirrorErr)
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(nr))
}

// delete 는 삭제 명령을 전파하고 성공 시 미러 행을 제거한다(REQ-I03/I04/E08).
func (h *RemoteEditHandler) delete(ctx api.Context, domain, resourceID string) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	if resourceID == "" {
		return api.ErrBadRequest.WithMessage("자원 id 는 필수입니다")
	}
	instanceID := ctx.Param("instance_id")

	if err := h.requireExposed(ctx, instanceID, domain, resourceID); err != nil {
		return err
	}

	args, _ := json.Marshal(map[string]string{"id": resourceID})

	h.logger.Info("원격 자원 삭제",
		"instance_id", instanceID, "domain", domain, "resource_id", resourceID, "actor", ctx.UserID())

	if _, dispErr := h.dispatch(ctx, instanceID, domain, remote.ActionDelete, args); dispErr != nil {
		return mapRemoteCommandError(dispErr)
	}

	// 성공 — 미러 행을 제거한다(REQ-E08).
	if mirrorErr := h.svc.DeleteMirror(ctx.Context(), instanceID, kindFor(domain), resourceID); mirrorErr != nil {
		h.logger.Warn("원격 삭제 후 미러 정리 실패",
			"instance_id", instanceID, "domain", domain, "resource_id", resourceID, "error", mirrorErr)
	}
	return ctx.NoContent(http.StatusNoContent)
}

// requireExposed 는 update/delete 대상 자원이 노드의 노출 범위 내인지 강제한다(REQ-I05).
// 범위 밖이면 404 를 반환한다(명령 디스패치 없음).
func (h *RemoteEditHandler) requireExposed(ctx api.Context, instanceID, domain, resourceID string) error {
	exposed, err := h.svc.IsResourceExposed(ctx.Context(), instanceID, kindFor(domain), resourceID)
	if err != nil {
		return api.ErrInternalServer.WithMessage(err.Error())
	}
	if !exposed {
		return api.ErrNotFound.WithMessage("대상 자원이 노드의 노출 범위에 없습니다")
	}
	return nil
}

// dispatch 는 actor 를 ctx 에 실어 명령을 전파한다(REQ-I12 who — 감사는 Dispatch 가 영속).
func (h *RemoteEditHandler) dispatch(ctx api.Context, instanceID, domain, action string, args json.RawMessage) (json.RawMessage, error) {
	dispatchCtx := remote.ContextWithActor(ctx.Context(), ctx.UserID())
	return h.svc.Dispatch(dispatchCtx, instanceID, domain, action, args)
}

// kindFor 는 도메인 문자열을 미러 kind 로 매핑한다(동일 문자열 — flow/agent).
func kindFor(domain string) string { return domain }

// decodeNodeResult 는 노드 결과 JSON 을 nodeResult 로 디코드한다(실패는 빈 결과).
func decodeNodeResult(result json.RawMessage) nodeResult {
	var nr nodeResult
	if len(result) > 0 {
		_ = json.Unmarshal(result, &nr)
	}
	return nr
}

// mirrorRowFromResult 는 노드 반환 결과로 미러 행을 구성한다(update upsert — REQ-E08).
//
// Config 는 노드가 반환한 라이브 정의이므로 시크릿을 포함할 수 있다. 미러에 쓰기 전
// RedactSensitiveConfig(F06 SoT)로 시크릿을 제거하여, 미러 캐시에 시크릿이 영속되지
// 않도록 한다(REQ-F06 — 미러는 항상 redacted).
func mirrorRowFromResult(instanceID, domain, resourceID string, nr nodeResult) storage.MirroredResource {
	def := json.RawMessage("{}")
	if nr.Config != nil {
		if data, err := json.Marshal(RedactSensitiveConfig(nr.Config)); err == nil {
			def = data
		}
	}
	id := nr.ID
	if id == "" {
		id = resourceID
	}
	return storage.MirroredResource{
		ID:               id,
		SourceInstanceID: instanceID,
		Name:             nr.Name,
		Kind:             kindFor(domain),
		Status:           nr.Status,
		Definition:       string(def),
	}
}
