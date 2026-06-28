// remote_mode_test.go 는 원격 관리 모드 조회 엔드포인트(GET /remote/mode)를 검증한다
// (@SPEC:SPEC-REMOTE-001). 모든 모드(server/client/disabled)에서 등록되며 주입된
// 모드 문자열을 JSON 으로 반환한다(admin 권한 불요 — 로그인 사용자 누구나 조회 가능).
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// doModeRequest 는 RemoteModeHandler 를 /api/v1 그룹에 등록하고 요청을 실행한다.
// role 은 admin 권한 불요를 검증하기 위해 비워둘 수 있다.
func doModeRequest(t *testing.T, mode, role string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewRemoteModeHandler(mode)
	router, rec, req := newAdminRequest(http.MethodGet, "/api/v1/remote/mode", role)
	h.RegisterRoutes(router.Group("/api/v1"))
	router.Handler().ServeHTTP(rec, req)
	return rec
}

// TestRemoteMode_ReturnsInjectedMode 는 주입된 모드를 JSON 200 으로 반환하는지
// 검증한다(server/client/disabled).
func TestRemoteMode_ReturnsInjectedMode(t *testing.T) {
	cases := []struct {
		name string
		mode string
		want string
	}{
		{"server", "server", "server"},
		{"client", "client", "client"},
		{"disabled", "disabled", "disabled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doModeRequest(t, tc.mode, "admin")
			require.Equal(t, http.StatusOK, rec.Code)

			var resp struct {
				Data struct {
					Mode string `json:"mode"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tc.want, resp.Data.Mode)
		})
	}
}

// TestRemoteMode_EmptyModeReportsDisabled 는 빈 모드(미설정)를 "disabled" 로
// 보고하는지 검증한다(기본값 안전).
func TestRemoteMode_EmptyModeReportsDisabled(t *testing.T) {
	rec := doModeRequest(t, "", "admin")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data struct {
			Mode string `json:"mode"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "disabled", resp.Data.Mode)
}

// TestRemoteMode_NoAdminRequired 는 admin 권한 없이도(role 미주입) 모드를 조회할
// 수 있는지 검증한다 — 표준 /api/v1 인증만 거치며 admin-role 강제는 없다.
func TestRemoteMode_NoAdminRequired(t *testing.T) {
	rec := doModeRequest(t, "server", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data struct {
			Mode string `json:"mode"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "server", resp.Data.Mode)
}
