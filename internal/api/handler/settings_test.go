// settings_test.go 는 전역 설정 API 핸들러(GET/PUT /settings/{key})의 통합 테스트이다.
//
// 검증 범위:
//   - PUT happy path: 유효 JSON 저장 → 200, 이후 GET 200 + 동일 value
//   - GET 없는 key → 404
//   - PUT 잘못된 JSON → 400
//   - PUT 256KB 초과 → 413
//   - 키 단위 격리(서로 다른 key 는 독립)
//   - repo 에러 → 500
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/storage"
)

// --- Mock SettingsRepository ---

type mockSettingsRepo struct {
	store   map[string]string
	getErr  error
	setErr  error
	getCall int
	setCall int
}

func newMockSettingsRepo() *mockSettingsRepo {
	return &mockSettingsRepo{store: make(map[string]string)}
}

func (m *mockSettingsRepo) GetSetting(_ context.Context, key string) (string, error) {
	m.getCall++
	if m.getErr != nil {
		return "", m.getErr
	}
	v, ok := m.store[key]
	if !ok {
		return "", storage.ErrSettingNotFound
	}
	return v, nil
}

func (m *mockSettingsRepo) SetSetting(_ context.Context, key, value string) error {
	m.setCall++
	if m.setErr != nil {
		return m.setErr
	}
	m.store[key] = value
	return nil
}

// setupSettingsRouter 는 SettingsHandler 가 등록된 라우터를 생성한다.
func setupSettingsRouter(repo SettingsRepository) *api.Router {
	router := api.NewRouter()
	h := NewSettingsHandler(repo, nil)
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router
}

func doSettingsRequest(router *api.Router, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var reader *bytes.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	router.Handler().ServeHTTP(rec, req)
	return rec
}

func TestNewSettingsHandler_NilLogger(t *testing.T) {
	h := NewSettingsHandler(newMockSettingsRepo(), nil)
	require.NotNil(t, h)
	assert.NotNil(t, h.logger)
}

func TestSettingsHandler_PutThenGet(t *testing.T) {
	repo := newMockSettingsRepo()
	router := setupSettingsRouter(repo)

	value := `{"columns":["name","online"]}`

	// PUT
	putRec := doSettingsRequest(router, http.MethodPut, "/api/v1/settings/device-list-columns", value)
	require.Equal(t, http.StatusOK, putRec.Code, putRec.Body.String())

	var putResp dto.APIResponse[SettingsResponse]
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &putResp))
	assert.Equal(t, "device-list-columns", putResp.Data.Key)
	assert.JSONEq(t, value, string(putResp.Data.Value))

	// GET
	getRec := doSettingsRequest(router, http.MethodGet, "/api/v1/settings/device-list-columns", "")
	require.Equal(t, http.StatusOK, getRec.Code, getRec.Body.String())

	var getResp dto.APIResponse[SettingsResponse]
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	assert.Equal(t, "device-list-columns", getResp.Data.Key)
	assert.JSONEq(t, value, string(getResp.Data.Value))
}

func TestSettingsHandler_Get(t *testing.T) {
	tests := []struct {
		name     string
		seed     map[string]string
		repoErr  error
		key      string
		wantCode int
	}{
		{
			name:     "존재하는 key → 200",
			seed:     map[string]string{"k": `{"a":1}`},
			key:      "k",
			wantCode: http.StatusOK,
		},
		{
			name:     "없는 key → 404",
			seed:     map[string]string{},
			key:      "missing",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "repo 에러 → 500",
			repoErr:  assertAnError(),
			key:      "k",
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockSettingsRepo()
			if tt.seed != nil {
				repo.store = tt.seed
			}
			repo.getErr = tt.repoErr
			router := setupSettingsRouter(repo)

			rec := doSettingsRequest(router, http.MethodGet, "/api/v1/settings/"+tt.key, "")
			assert.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
		})
	}
}

func TestSettingsHandler_Put(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		setErr   error
		wantCode int
	}{
		{
			name:     "유효 JSON 객체 → 200",
			body:     `{"x":true}`,
			wantCode: http.StatusOK,
		},
		{
			name:     "유효 JSON 배열 → 200",
			body:     `[1,2,3]`,
			wantCode: http.StatusOK,
		},
		{
			name:     "잘못된 JSON → 400",
			body:     `{not json`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "빈 body → 400 (유효 JSON 아님)",
			body:     "",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "repo set 에러 → 500",
			body:     `{"x":1}`,
			setErr:   assertAnError(),
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockSettingsRepo()
			repo.setErr = tt.setErr
			router := setupSettingsRouter(repo)

			rec := doSettingsRequest(router, http.MethodPut, "/api/v1/settings/k", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
		})
	}
}

func TestSettingsHandler_PutTooLarge(t *testing.T) {
	repo := newMockSettingsRepo()
	router := setupSettingsRouter(repo)

	// 256KB 초과 JSON 문자열 생성: {"v":"<aaaa...>"}
	big := strings.Repeat("a", maxSettingsValueBytes+100)
	body := `{"v":"` + big + `"}`

	rec := doSettingsRequest(router, http.MethodPut, "/api/v1/settings/k", body)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
	assert.Zero(t, repo.setCall, "한도 초과 시 저장이 호출되지 않아야 한다")
}

func TestSettingsHandler_KeyIsolation(t *testing.T) {
	repo := newMockSettingsRepo()
	router := setupSettingsRouter(repo)

	require.Equal(t, http.StatusOK,
		doSettingsRequest(router, http.MethodPut, "/api/v1/settings/a", `{"k":"A"}`).Code)
	require.Equal(t, http.StatusOK,
		doSettingsRequest(router, http.MethodPut, "/api/v1/settings/b", `{"k":"B"}`).Code)

	recA := doSettingsRequest(router, http.MethodGet, "/api/v1/settings/a", "")
	recB := doSettingsRequest(router, http.MethodGet, "/api/v1/settings/b", "")

	var respA, respB dto.APIResponse[SettingsResponse]
	require.NoError(t, json.Unmarshal(recA.Body.Bytes(), &respA))
	require.NoError(t, json.Unmarshal(recB.Body.Bytes(), &respB))
	assert.JSONEq(t, `{"k":"A"}`, string(respA.Data.Value))
	assert.JSONEq(t, `{"k":"B"}`, string(respB.Data.Value))
}

// assertAnError 는 테스트용 임의 에러를 반환한다.
func assertAnError() error {
	return errTestSentinel
}

var errTestSentinel = errorString("boom")

type errorString string

func (e errorString) Error() string { return string(e) }
