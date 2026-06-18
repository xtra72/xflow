// remote_version.go 는 원격 관리 서버의 "버전 관리"(Phase 1) REST 핸들러를 제공한다
// (@SPEC:SPEC-REMOTE-001 버전 관리).
//
// 제공 기능:
//   - GET  /remote/nodes/{instance_id}/version-history — 노드 버전 변경 이력(최신순)
//   - GET  /remote/target-version                      — 관리자 지정 목표 버전 조회
//   - PUT  /remote/target-version                      — 관리자 지정 목표 버전 설정
//
// 목표 버전은 SettingsRepository(전역 KV)에 JSON 으로 영속하며, /remote/nodes 응답의
// outdated 플래그 계산에 사용한다(노드 버전 < 목표 버전이면 outdated). 모든 엔드포인트는
// admin 권한을 강제한다(REQ-F04).
package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/updater"
)

// targetVersionSettingKey 는 SettingsRepository 에 목표 버전을 저장하는 전역 키이다.
const targetVersionSettingKey = "remote.target_version"

// targetVersionDoc 는 목표 버전 설정 값의 JSON 스키마이다(SettingsRepository 는 불투명
// JSON 문자열을 저장하므로 향후 확장(채널 등)을 위해 객체로 감싼다).
type targetVersionDoc struct {
	Version string `json:"version"`
}

// nodeVersionHistoryDTO 는 버전 이력 한 줄의 응답 표현이다.
type nodeVersionHistoryDTO struct {
	Version   string `json:"version"`
	ChangedAt int64  `json:"changed_at"` // epoch ms
}

// targetVersion 은 현재 설정된 목표 버전을 반환한다. settings 미구성/미설정/파싱 실패
// 시 빈 문자열을 반환한다(outdated 계산이 자연 비활성).
func (h *RemoteAdminHandler) targetVersion(ctx context.Context) string {
	if h.settings == nil {
		return ""
	}
	raw, err := h.settings.GetSetting(ctx, targetVersionSettingKey)
	if err != nil || raw == "" {
		return ""
	}
	var doc targetVersionDoc
	if json.Unmarshal([]byte(raw), &doc) != nil {
		return ""
	}
	return doc.Version
}

// VersionHistory 는 노드의 버전 변경 이력을 최신순으로 반환한다.
// GET /remote/nodes/{instance_id}/version-history?limit=N
func (h *RemoteAdminHandler) VersionHistory(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	id := ctx.Param("instance_id")
	limit := parsePositiveInt(ctx.Query("limit"), 100)
	hist, err := h.svc.NodeVersionHistory(ctx.Context(), id, limit)
	if err != nil {
		return mapRemoteAdminError(err)
	}
	out := make([]nodeVersionHistoryDTO, 0, len(hist))
	for _, e := range hist {
		out = append(out, nodeVersionHistoryDTO{Version: e.Version, ChangedAt: e.ChangedAt})
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(out))
}

// GetTargetVersion 은 현재 목표 버전을 반환한다. GET /remote/target-version
func (h *RemoteAdminHandler) GetTargetVersion(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(targetVersionDoc{Version: h.targetVersion(ctx.Context())}))
}

// PutTargetVersion 은 관리자가 목표 버전을 설정한다. PUT /remote/target-version {version}
// 빈 문자열은 목표 버전 해제(outdated 비활성)를 의미한다. 비어 있지 않으면 semver 여야 한다.
func (h *RemoteAdminHandler) PutTargetVersion(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	if h.settings == nil {
		return api.ErrInternalServer.WithMessage("설정 저장소가 구성되지 않았습니다")
	}
	var req targetVersionDoc
	if err := ctx.Bind(&req); err != nil {
		return api.ErrBadRequest.WithMessage("요청 본문 파싱 실패")
	}
	if req.Version != "" && !updater.Version(req.Version).IsValid() {
		return api.ErrBadRequest.WithMessage("version 은 vMAJOR.MINOR.PATCH 형식이어야 합니다")
	}
	payload, err := json.Marshal(targetVersionDoc{Version: req.Version})
	if err != nil {
		return api.ErrInternalServer.WithMessage(err.Error())
	}
	if err := h.settings.SetSetting(ctx.Context(), targetVersionSettingKey, string(payload)); err != nil {
		return api.ErrInternalServer.WithMessage(err.Error())
	}
	h.logger.Info("원격 목표 버전 설정", "target_version", req.Version, "actor", ctx.UserID())
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(targetVersionDoc{Version: req.Version}))
}

// isOutdated 는 노드 버전이 목표 버전보다 낮은지(semver) 판정한다.
// 목표 미설정/빈 버전/비-semver 는 false(보수적 — 알 수 없으면 구버전 아님으로 표시).
func isOutdated(nodeVersion, target string) bool {
	if target == "" || nodeVersion == "" {
		return false
	}
	nv := updater.Version(nodeVersion)
	tv := updater.Version(target)
	if !nv.IsValid() || !tv.IsValid() {
		return false
	}
	return nv.Compare(tv) < 0
}
