// @SPEC:SPEC-DASHBOARD-004 (M4, spec.md §2.9 E3, §4.5, §2.13 UB1 #5)
// dashboard_acl.go — 대시보드 권한 부여(ACL) REST 핸들러.
//
// ACL 은 **전량 치환**만 제공한다. 항목 단위 POST/DELETE 를 두면 클라이언트가
// "화면에 보이는 목록" 과 "서버 상태" 의 차이를 계산해야 하고, 두 관리자가 동시에
// 편집할 때 중간 상태가 커밋된다(spec.md §4.5).
//
// 검증은 전부 통과하거나 전부 거부한다 — 일부만 저장되면 "누가 권한을 갖고 있는가"
// 를 요청 하나로 결정할 수 없게 된다(spec.md §2.9).

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/dashboardacl"
	"github.com/xtra/xflow/internal/storage"
)

// GetACL 은 대시보드의 ACL 을 반환한다 (grant 인가 필요).
// GET /api/v1/dashboards/{uid}/acl
func (h *DashboardHandler) GetACL(ctx api.Context) error {
	_, d, access, err := h.load(ctx)
	if err != nil {
		return err
	}
	if !access.View {
		return forbiddenDashboard()
	}
	if !access.Grant {
		return forbiddenDashboard()
	}
	if h.acl == nil {
		return ctx.JSON(http.StatusOK, dto.NewSuccessResponse([]dto.DashboardACLEntry{}))
	}

	rows, err := h.acl.ListByDashboard(ctx.Context(), d.ID)
	if err != nil {
		h.logger.Error("대시보드 ACL 조회 실패", "uid", d.UID, "error", err)
		return api.ErrInternalServer.WithMessage("대시보드 권한 조회 실패")
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toACLDTO(rows)))
}

// PutACL 은 대시보드의 ACL 을 요청 내용으로 전량 치환한다 (grant 인가 필요).
// PUT /api/v1/dashboards/{uid}/acl
//
// 하나라도 유효하지 않으면 400 이며 ACL 은 전혀 변경되지 않는다(spec.md §2.9).
func (h *DashboardHandler) PutACL(ctx api.Context) error {
	_, d, access, err := h.load(ctx)
	if err != nil {
		return err
	}
	if !access.View {
		return forbiddenDashboard()
	}
	if !access.Grant {
		return forbiddenDashboard()
	}
	if h.acl == nil {
		return api.ErrInternalServer.WithMessage("대시보드 권한 저장소가 구성되지 않았습니다")
	}

	body, err := h.readBody(ctx)
	if err != nil {
		return err
	}
	entries, err := decodeACLRequest(body)
	if err != nil {
		return err
	}
	if err := h.validateACLEntries(ctx.Context(), d.Owner, entries); err != nil {
		return err
	}

	grantedBy := ctx.UserID()
	rows := make([]storage.DashboardACLEntry, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, storage.DashboardACLEntry{
			DashboardID: d.ID,
			Subject:     e.Subject,
			Level:       e.Level,
			GrantedBy:   grantedBy,
		})
	}

	if err := h.acl.Replace(ctx.Context(), d.ID, rows); err != nil {
		h.logger.Error("대시보드 ACL 저장 실패", "uid", d.UID, "error", err)
		return api.ErrInternalServer.WithMessage("대시보드 권한 저장 실패")
	}

	saved, err := h.acl.ListByDashboard(ctx.Context(), d.ID)
	if err != nil {
		h.logger.Error("대시보드 ACL 재조회 실패", "uid", d.UID, "error", err)
		return api.ErrInternalServer.WithMessage("대시보드 권한 조회 실패")
	}

	h.logger.Info("대시보드 권한 변경", "uid", d.UID, "entries", len(saved), "actor", grantedBy)
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toACLDTO(saved)))
}

// decodeACLRequest 는 최상위 배열과 `{"entries":[...]}` 객체 형식을 모두 받는다.
//
// acceptance.md AC-17 은 최상위 배열을 보내고, 객체 형식은 확장 여지를 남긴다.
// 두 형식을 모두 받는 편이 클라이언트 구현체를 하나로 강제하는 것보다 안전하다.
func decodeACLRequest(body []byte) ([]dto.DashboardACLEntry, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}

	if strings.HasPrefix(trimmed, "[") {
		var entries []dto.DashboardACLEntry
		if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
			return nil, api.ErrBadRequest.WithMessage("invalid request body: " + err.Error())
		}
		return entries, nil
	}

	var req dto.DashboardACLReplaceRequest
	if err := json.Unmarshal([]byte(trimmed), &req); err != nil {
		return nil, api.ErrBadRequest.WithMessage("invalid request body: " + err.Error())
	}
	return req.Entries, nil
}

// validateACLEntries 는 subject 형식·실재, level, 중복, 소유자 등재를 검증한다
// (acceptance.md AC-17).
//
// 소유자 자신을 등재·제거하는 것은 400 이다. 소유권이 항상 ACL 보다 우선하므로
// 무의미하며, "나를 뺐다" 는 오해를 만든다(spec.md §2.13 UB1 #5).
func (h *DashboardHandler) validateACLEntries(ctx context.Context, owner string, entries []dto.DashboardACLEntry) error {
	seen := make(map[string]struct{}, len(entries))

	for _, e := range entries {
		subject := e.Subject

		if !dashboardacl.IsValidLevel(e.Level) {
			return api.ErrBadRequest.WithMessage("level 은 view 또는 edit 이어야 합니다: " + e.Level)
		}
		if _, dup := seen[subject]; dup {
			return api.ErrBadRequest.WithMessage("중복된 subject 입니다: " + subject)
		}
		seen[subject] = struct{}{}

		switch {
		case strings.HasPrefix(subject, dashboardacl.SubjectPrefixUser):
			username := strings.TrimPrefix(subject, dashboardacl.SubjectPrefixUser)
			if username == "" {
				return api.ErrBadRequest.WithMessage("subject 의 사용자 이름이 비어 있습니다")
			}
			if owner != "" && username == owner {
				return api.ErrBadRequest.WithMessage(
					"소유자는 ACL 에 등재할 수 없습니다 — 소유권이 항상 우선합니다")
			}
			ok, err := h.userExists(ctx, username)
			if err != nil {
				return err
			}
			if !ok {
				return api.ErrBadRequest.WithMessage("존재하지 않는 사용자입니다: " + username)
			}

		case strings.HasPrefix(subject, dashboardacl.SubjectPrefixRole):
			role := strings.TrimPrefix(subject, dashboardacl.SubjectPrefixRole)
			if role == "" {
				return api.ErrBadRequest.WithMessage("subject 의 역할 이름이 비어 있습니다")
			}
			ok, err := h.roleExists(ctx, role)
			if err != nil {
				return err
			}
			if !ok {
				return api.ErrBadRequest.WithMessage("존재하지 않는 역할입니다: " + role)
			}

		default:
			return api.ErrBadRequest.WithMessage(
				"subject 는 user:<username> 또는 role:<rolename> 형식이어야 합니다: " + subject)
		}
	}
	return nil
}

// userExists 는 사용자 실재 여부를 판정한다. DB 미주입 시 검증을 생략한다.
func (h *DashboardHandler) userExists(ctx context.Context, username string) (bool, error) {
	if h.db == nil {
		return true, nil
	}
	_, err := storage.GetUserByUsername(ctx, h.db, username)
	if errors.Is(err, storage.ErrUserNotFound) {
		return false, nil
	}
	if err != nil {
		h.logger.Error("사용자 조회 실패", "username", username, "error", err)
		return false, api.ErrInternalServer.WithMessage("사용자 조회 실패")
	}
	return true, nil
}

// roleExists 는 역할 실재 여부를 판정한다. DB 미주입 시 검증을 생략한다.
func (h *DashboardHandler) roleExists(ctx context.Context, role string) (bool, error) {
	if h.db == nil {
		return true, nil
	}
	_, err := storage.GetRoleByName(ctx, h.db, role)
	if errors.Is(err, storage.ErrRoleNotFound) {
		return false, nil
	}
	if err != nil {
		h.logger.Error("역할 조회 실패", "role", role, "error", err)
		return false, api.ErrInternalServer.WithMessage("역할 조회 실패")
	}
	return true, nil
}

// toACLDTO 는 저장소 ACL 행을 응답 DTO 로 변환한다.
// 항목이 없어도 null 이 아닌 빈 배열로 직렬화한다 (클라이언트 분기 단순화).
func toACLDTO(rows []storage.DashboardACLEntry) []dto.DashboardACLEntry {
	out := make([]dto.DashboardACLEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.DashboardACLEntry{
			Subject:   r.Subject,
			Level:     r.Level,
			GrantedBy: r.GrantedBy,
			GrantedAt: r.GrantedAt,
		})
	}
	return out
}
