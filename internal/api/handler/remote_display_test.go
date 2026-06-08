// remote_display_test.go 는 v1.6(M11, 그룹 M) NodeDetail 의 노드 해상도
// (display_width/display_height) 노출을 검증한다(@SPEC:SPEC-REMOTE-001 M11, REQ-M02/M03).
//
// v1.6 M11 확장(서버 오버라이드): DTO 의 display_width/height 는 이제 EFFECTIVE 해상도
// (오버라이드>보고값)를 의미하며 remote.NodeDetail.EffectiveWidth/Height 로부터 매핑된다.
// 본 파일의 fake 는 오버라이드가 없는 경우 effective=보고값이 되도록 EffectiveWidth/Height
// 를 노드 보고값과 동일하게 설정한다(실서버 remote.Server.NodeDetail 파생과 일관).
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

// TestRemoteGrouping_NodeDetailExposesDisplayResolution 는 노드 상세 응답이 해상도를
// 노출하는지 검증한다(REQ-M02 — 관리자 뷰가 고정 캔버스 크기에 소비).
func TestRemoteGrouping_NodeDetailExposesDisplayResolution(t *testing.T) {
	svc := newFakeGrouping()
	svc.detail = remote.NodeDetail{
		Node: storage.ManagedNode{
			InstanceID: "n1", Hostname: "host", Status: "approved",
			DisplayWidth: 1920, DisplayHeight: 1080,
		},
		// 오버라이드 없음 → effective=보고값(실서버 파생 일관).
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
	assert.Equal(t, 1920, resp.Data.DisplayWidth)
	assert.Equal(t, 1080, resp.Data.DisplayHeight)
}

// TestRemoteGrouping_NodeDetailUnreportedDisplayIsZero 는 미보고 노드의 해상도가
// 0 으로 노출되는지 검증한다(REQ-M03 — 프론트엔드가 폴백 트리거).
func TestRemoteGrouping_NodeDetailUnreportedDisplayIsZero(t *testing.T) {
	svc := newFakeGrouping()
	svc.detail = remote.NodeDetail{
		Node:   storage.ManagedNode{InstanceID: "n1", Status: "approved"},
		Online: false,
	}

	rec := doGrouping(t, svc, "admin", http.MethodGet, "/api/v1/remote/nodes/n1", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data NodeDetailDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Data.DisplayWidth, "미보고 → 0(프론트 폴백)")
	assert.Equal(t, 0, resp.Data.DisplayHeight, "미보고 → 0(프론트 폴백)")
}
