// dashboard_asset_test.go — /api/dashboard-assets 핸들러 통합 테스트.
//
// 검증 범위:
//   - 업로드 → id 반환, 같은 내용 재업로드는 같은 id (내용 주소화 멱등)
//   - 조회 → data-URL 왕복 (업로드한 바이트가 그대로 복원)
//   - 미인증 401 / 없는 id 404 / 손상 data-URL 400 / 비이미지 MIME 400
//   - snapshot 상한(256KB)과 **다른 축**임을 확인 — 256KB 초과 이미지도 업로드된다.
//     이것이 자산 분리의 존재 이유다(대시보드 저장 실패의 직접 원인이었다).

package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/storage"
)

// setupAssetRouter 는 DashboardAssetHandler 가 등록된 라우터를 반환한다.
func setupAssetRouter(t *testing.T) *api.Router {
	t.Helper()
	ctx := context.Background()
	db, err := storage.OpenSQLiteDB(ctx, filepath.Join(t.TempDir(), "assets.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	repo, err := storage.NewDashboardAssetSQLiteRepository(ctx, db)
	require.NoError(t, err)

	router := api.NewRouter()
	h := NewDashboardAssetHandler(repo, nil)
	h.RegisterRoutes(router.Group("/api/v1"))
	return router
}

/** 바이트를 base64 data-URL 로 만든다. */
func pngDataURL(data []byte) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
}

// uploadAsset 은 data-URL 을 업로드하고 응답 recorder 를 반환한다.
func uploadAsset(t *testing.T, router *api.Router, username, dataURL string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"data_url": dataURL})
	require.NoError(t, err)
	return requestWithAuth(t, router, http.MethodPost, "/api/v1/dashboard-assets",
		username, "editor", bytes.NewReader(body), "")
}

// decodeAssetData 는 envelope 의 data 를 map 으로 디코딩한다.
func decodeAssetData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var env dto.APIResponse[map[string]any]
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&env), "응답 디코딩 실패: %s", rec.Body.String())
	require.True(t, env.Success, "envelope.success 는 true 여야 함: %s", rec.Body.String())
	return env.Data
}

func TestDashboardAsset_Upload_ReturnsContentAddressedID(t *testing.T) {
	router := setupAssetRouter(t)
	rec := uploadAsset(t, router, "alice", pngDataURL([]byte("floor-plan-bytes")))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	data := decodeAssetData(t, rec)
	id, _ := data["id"].(string)
	assert.Len(t, id, 64, "id 는 SHA-256 hex(64자)")
	assert.Equal(t, "image/png", data["mime"])
}

func TestDashboardAsset_Upload_SameContentSameID(t *testing.T) {
	router := setupAssetRouter(t)
	url := pngDataURL([]byte("identical"))

	first := decodeAssetData(t, uploadAsset(t, router, "alice", url))
	// 다른 사용자가 같은 이미지를 올려도 같은 id — 중복 저장이 없다.
	second := decodeAssetData(t, uploadAsset(t, router, "bob", url))
	assert.Equal(t, first["id"], second["id"])
}

func TestDashboardAsset_Get_RoundTripsBytes(t *testing.T) {
	router := setupAssetRouter(t)
	original := []byte{0x89, 0x50, 0x4e, 0x47, 0x00, 0xff, 0x10}
	uploaded := decodeAssetData(t, uploadAsset(t, router, "alice", pngDataURL(original)))
	id := uploaded["id"].(string)

	rec := requestWithAuth(t, router, http.MethodGet, "/api/v1/dashboard-assets/"+id,
		"alice", "editor", nil, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	got := decodeAssetData(t, rec)
	assert.Equal(t, pngDataURL(original), got["data_url"], "업로드한 바이트가 그대로 복원되어야 한다")
	assert.Equal(t, "image/png", got["mime"])
}

func TestDashboardAsset_Get_NotFound(t *testing.T) {
	router := setupAssetRouter(t)
	rec := requestWithAuth(t, router, http.MethodGet,
		"/api/v1/dashboard-assets/0000000000000000000000000000000000000000000000000000000000000000",
		"alice", "editor", nil, "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDashboardAsset_Unauthenticated_401(t *testing.T) {
	router := setupAssetRouter(t)
	// username="" → UserID() 가 "" → 401.
	body, _ := json.Marshal(map[string]string{"data_url": pngDataURL([]byte("x"))})
	rec := requestWithAuth(t, router, http.MethodPost, "/api/v1/dashboard-assets",
		"", "", bytes.NewReader(body), "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = requestWithAuth(t, router, http.MethodGet, "/api/v1/dashboard-assets/abc", "", "", nil, "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestDashboardAsset_Upload_RejectsMalformedDataURL(t *testing.T) {
	router := setupAssetRouter(t)
	for name, url := range map[string]string{
		"data 접두사 없음": "image/png;base64,AAAA",
		"콤마 없음":       "data:image/png;base64",
		"base64 아님":   "data:image/png,AAAA",
		"MIME 없음":     "data:;base64,AAAA",
		"base64 손상":   "data:image/png;base64,!!!!",
		"빈 payload":   "data:image/png;base64,",
	} {
		t.Run(name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"data_url": url})
			rec := requestWithAuth(t, router, http.MethodPost, "/api/v1/dashboard-assets",
				"alice", "editor", bytes.NewReader(body), "")
			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		})
	}
}

func TestDashboardAsset_Upload_RejectsNonImageMIME(t *testing.T) {
	router := setupAssetRouter(t)
	// 임의 바이너리 보관소로 전용되지 않도록 이미지 MIME 만 허용한다.
	url := "data:application/zip;base64," + base64.StdEncoding.EncodeToString([]byte("PK"))
	body, _ := json.Marshal(map[string]string{"data_url": url})
	rec := requestWithAuth(t, router, http.MethodPost, "/api/v1/dashboard-assets",
		"alice", "editor", bytes.NewReader(body), "")
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestDashboardAsset_Upload_AcceptsImageLargerThanSnapshotLimit(t *testing.T) {
	// 자산 분리의 존재 이유: snapshot 상한(256KB)을 넘는 도면 사진도 자산으로는 저장된다.
	// 이것을 config 에 data-URL 로 박았을 때 대시보드 저장 전체가 413 으로 실패했다.
	router := setupAssetRouter(t)
	big := bytes.Repeat([]byte{0x41}, maxDashboardPayloadBytes+1024)
	rec := uploadAsset(t, router, "alice", pngDataURL(big))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	data := decodeAssetData(t, rec)
	assert.Equal(t, float64(len(big)), data["size"], fmt.Sprintf("size 는 원본 바이트 수: %v", data))
}
