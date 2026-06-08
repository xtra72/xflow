// remote_display_override_test.go 는 v1.6(M11 확장) 노드 해상도 서버-측 오버라이드
// REST API(PUT/DELETE /remote/nodes/{instance_id}/display)의 권한·검증·동작과,
// NodeDetail 의 effective/override/reported 노출을 검증한다
// (@SPEC:SPEC-REMOTE-001 M11, OQ-M1 보조 override, REQ-K06/F04 admin 게이팅 일관).
package handler

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/remote"
	"github.com/xtra/xflow/internal/storage"
)

// 본 파일은 remote_grouping_test.go 의 fakeGrouping(overrides 필드 + Set/Clear
// DisplayOverride 메서드 보강)과 doGrouping 헬퍼에 의존한다(동일 패키지).

// TestRemoteDisplay_SetOverride 는 admin 이 오버라이드를 설정하는지 검증한다.
func TestRemoteDisplay_SetOverride(t *testing.T) {
	svc := newFakeGrouping()
	svc.groups["n1"] = ""

	rec := doGrouping(t, svc, "admin", http.MethodPut, "/api/v1/remote/nodes/n1/display", `{"width":1920,"height":1080}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, [2]int{1920, 1080}, svc.overrides["n1"])
}

// TestRemoteDisplay_SetOverrideRequiresAdmin 는 비-admin 거부(403)를 검증한다(REQ-K06/F04).
func TestRemoteDisplay_SetOverrideRequiresAdmin(t *testing.T) {
	svc := newFakeGrouping()
	svc.groups["n1"] = ""

	rec := doGrouping(t, svc, "node", http.MethodPut, "/api/v1/remote/nodes/n1/display", `{"width":1920,"height":1080}`)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	_, set := svc.overrides["n1"]
	assert.False(t, set, "거부된 요청은 오버라이드를 변경하지 않아야 함")
}

// TestRemoteDisplay_SetOverrideViewerRejected 는 viewer 거부(403)를 검증한다.
func TestRemoteDisplay_SetOverrideViewerRejected(t *testing.T) {
	svc := newFakeGrouping()
	svc.groups["n1"] = ""
	rec := doGrouping(t, svc, "viewer", http.MethodPut, "/api/v1/remote/nodes/n1/display", `{"width":1920,"height":1080}`)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestRemoteDisplay_SetOverrideNonPositiveWidth 는 width<=0 이 400 으로 거부되는지
// 검증한다(검증 — 양수만 허용; 해제는 DELETE 로).
func TestRemoteDisplay_SetOverrideNonPositiveWidth(t *testing.T) {
	svc := newFakeGrouping()
	svc.groups["n1"] = ""
	rec := doGrouping(t, svc, "admin", http.MethodPut, "/api/v1/remote/nodes/n1/display", `{"width":0,"height":1080}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	_, set := svc.overrides["n1"]
	assert.False(t, set, "검증 실패 요청은 오버라이드를 설정하지 않아야 함")
}

// TestRemoteDisplay_SetOverrideNonPositiveHeight 는 height<=0 이 400 으로 거부되는지 검증한다.
func TestRemoteDisplay_SetOverrideNonPositiveHeight(t *testing.T) {
	svc := newFakeGrouping()
	svc.groups["n1"] = ""
	rec := doGrouping(t, svc, "admin", http.MethodPut, "/api/v1/remote/nodes/n1/display", `{"width":1920,"height":-5}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestRemoteDisplay_SetOverrideBadBody 는 잘못된 JSON 본문이 400 으로 매핑되는지 검증한다.
func TestRemoteDisplay_SetOverrideBadBody(t *testing.T) {
	svc := newFakeGrouping()
	svc.groups["n1"] = ""
	rec := doGrouping(t, svc, "admin", http.MethodPut, "/api/v1/remote/nodes/n1/display", `{not json`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestRemoteDisplay_SetOverrideUnknownNode 는 미존재 노드 404 를 검증한다.
func TestRemoteDisplay_SetOverrideUnknownNode(t *testing.T) {
	svc := newFakeGrouping()
	rec := doGrouping(t, svc, "admin", http.MethodPut, "/api/v1/remote/nodes/missing/display", `{"width":1920,"height":1080}`)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestRemoteDisplay_ClearOverride 는 DELETE 가 오버라이드를 해제(204)하는지 검증한다.
func TestRemoteDisplay_ClearOverride(t *testing.T) {
	svc := newFakeGrouping()
	svc.groups["n1"] = ""
	svc.overrides["n1"] = [2]int{1920, 1080}

	rec := doGrouping(t, svc, "admin", http.MethodDelete, "/api/v1/remote/nodes/n1/display", "")
	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, [2]int{0, 0}, svc.overrides["n1"], "해제 시 0,0 으로 환원")
}

// TestRemoteDisplay_ClearOverrideRequiresAdmin 는 비-admin 해제 거부(403)를 검증한다.
func TestRemoteDisplay_ClearOverrideRequiresAdmin(t *testing.T) {
	svc := newFakeGrouping()
	svc.groups["n1"] = ""
	svc.overrides["n1"] = [2]int{1920, 1080}
	rec := doGrouping(t, svc, "node", http.MethodDelete, "/api/v1/remote/nodes/n1/display", "")
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, [2]int{1920, 1080}, svc.overrides["n1"])
}

// TestRemoteDisplay_ClearOverrideUnknownNode 는 미존재 노드 해제 404 를 검증한다.
func TestRemoteDisplay_ClearOverrideUnknownNode(t *testing.T) {
	svc := newFakeGrouping()
	rec := doGrouping(t, svc, "admin", http.MethodDelete, "/api/v1/remote/nodes/missing/display", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestRemoteDisplay_NodeDetailEffectiveOverride 는 오버라이드가 설정된 경우
// NodeDetailDTO 의 display_width/height(effective)가 오버라이드를 반영하고,
// display_override_* 와 display_reported_* 가 각각 노출되는지 검증한다(프론트엔드 소스 표시).
func TestRemoteDisplay_NodeDetailEffectiveOverride(t *testing.T) {
	svc := newFakeGrouping()
	svc.detail = remote.NodeDetail{
		Node: storage.ManagedNode{
			InstanceID: "n1", Status: "approved",
			DisplayWidth: 1366, DisplayHeight: 768, // 노드 보고(reported)
			DisplayOverrideWidth: 1920, DisplayOverrideHeight: 1080, // 관리자 오버라이드
		},
		// EffectiveWidth/Height 는 remote.NodeDetail 이 산출(override 우선).
		EffectiveWidth:  1920,
		EffectiveHeight: 1080,
		Online:          true,
	}

	rec := doGrouping(t, svc, "admin", http.MethodGet, "/api/v1/remote/nodes/n1", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data NodeDetailDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	d := resp.Data
	// effective(display_width/height) = override.
	assert.Equal(t, 1920, d.DisplayWidth, "effective display_width 는 override 를 반영해야 함")
	assert.Equal(t, 1080, d.DisplayHeight, "effective display_height 는 override 를 반영해야 함")
	// override 노출.
	assert.Equal(t, 1920, d.DisplayOverrideWidth)
	assert.Equal(t, 1080, d.DisplayOverrideHeight)
	// reported 노출(소스 표시용).
	assert.Equal(t, 1366, d.DisplayReportedWidth)
	assert.Equal(t, 768, d.DisplayReportedHeight)
}

// TestRemoteDisplay_NodeDetailEffectiveReported 는 오버라이드 미설정(0,0) 시
// effective 가 노드 보고값(reported)이 되는지 검증한다(하위 호환).
func TestRemoteDisplay_NodeDetailEffectiveReported(t *testing.T) {
	svc := newFakeGrouping()
	svc.detail = remote.NodeDetail{
		Node: storage.ManagedNode{
			InstanceID: "n1", Status: "approved",
			DisplayWidth: 1366, DisplayHeight: 768, // 노드 보고
			// override 없음(0,0).
		},
		EffectiveWidth:  1366,
		EffectiveHeight: 768,
		Online:          true,
	}

	rec := doGrouping(t, svc, "admin", http.MethodGet, "/api/v1/remote/nodes/n1", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data NodeDetailDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	d := resp.Data
	assert.Equal(t, 1366, d.DisplayWidth, "오버라이드 없으면 effective=reported")
	assert.Equal(t, 768, d.DisplayHeight)
	assert.Equal(t, 0, d.DisplayOverrideWidth, "오버라이드 없으면 override=0")
	assert.Equal(t, 0, d.DisplayOverrideHeight)
	assert.Equal(t, 1366, d.DisplayReportedWidth)
	assert.Equal(t, 768, d.DisplayReportedHeight)
}
