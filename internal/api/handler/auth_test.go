package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/auth"
	"github.com/xtra/xflow/internal/storage"
)

// testAuthSetup 은 인증 테스트를 위한 공통 설정을 생성한다.
type testAuthSetup struct {
	handler     *AuthHandler
	credentials *auth.CredentialsManager
	jwtSvc      *auth.JWTService
	router      *api.Router
}

// newTestAuthSetup 은 SQLite 백엔드 (SPEC-DASHBOARD-001 v0.2.0) 로 테스트 환경을
// 구성한다. admin 과 viewer 두 사용자를 미리 직접 삽입한다.
func newTestAuthSetup(t *testing.T) *testAuthSetup {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "auth-test.db")
	db, err := storage.OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	hash, err := auth.HashPassword("password123")
	require.NoError(t, err)
	require.NoError(t, storage.InsertUser(ctx, db, "admin", hash, "admin", 0, 0))
	require.NoError(t, storage.InsertUser(ctx, db, "viewer", hash, "viewer", 0, 0))

	cm := auth.NewCredentialsManager(db, "")
	require.NoError(t, cm.Load())

	jwtSvc, err := auth.NewJWTService("test-secret-for-handler-tests", "15m", "168h")
	require.NoError(t, err)

	h := NewAuthHandler(cm, jwtSvc, nil)

	router := api.NewRouter()
	router.Use(api.Auth(true, jwtSvc))
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)

	return &testAuthSetup{
		handler:     h,
		credentials: cm,
		jwtSvc:      jwtSvc,
		router:      router,
	}
}

func doAuthRequest(router *api.Router, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}

	req := httptest.NewRequest(method, path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	w := httptest.NewRecorder()
	router.Handler().ServeHTTP(w, req)
	return w
}

func parseResponse[T any](t *testing.T, w *httptest.ResponseRecorder) dto.APIResponse[T] {
	t.Helper()
	var resp dto.APIResponse[T]
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

func TestAuthHandler_Login(t *testing.T) {
	setup := newTestAuthSetup(t)

	tests := []struct {
		name       string
		body       dto.LoginRequest
		wantStatus int
		wantOK     bool
	}{
		{
			name:       "성공적인 로그인",
			body:       dto.LoginRequest{Username: "admin", Password: "password123"},
			wantStatus: http.StatusOK,
			wantOK:     true,
		},
		{
			name:       "잘못된 비밀번호",
			body:       dto.LoginRequest{Username: "admin", Password: "wrong"},
			wantStatus: http.StatusUnauthorized,
			wantOK:     false,
		},
		{
			name:       "존재하지 않는 사용자",
			body:       dto.LoginRequest{Username: "nobody", Password: "password123"},
			wantStatus: http.StatusUnauthorized,
			wantOK:     false,
		},
		{
			name:       "빈 사용자명",
			body:       dto.LoginRequest{Username: "", Password: "password123"},
			wantStatus: http.StatusBadRequest,
			wantOK:     false,
		},
		{
			name:       "빈 비밀번호",
			body:       dto.LoginRequest{Username: "admin", Password: ""},
			wantStatus: http.StatusBadRequest,
			wantOK:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := doAuthRequest(setup.router, "POST", "/api/v1/auth/login", tc.body, nil)
			assert.Equal(t, tc.wantStatus, w.Code)

			if tc.wantOK {
				// SPEC-AUTH-004 U1: 응답이 {user, tokens} 중첩 구조여야 한다.
				resp := parseResponse[dto.LoginResponse](t, w)
				assert.True(t, resp.Success)
				assert.Equal(t, tc.body.Username, resp.Data.User.Username)
				assert.NotEmpty(t, resp.Data.User.Role)
				assert.NotEmpty(t, resp.Data.Tokens.AccessToken)
				assert.NotEmpty(t, resp.Data.Tokens.RefreshToken)
				assert.Equal(t, "Bearer", resp.Data.Tokens.TokenType)
				assert.Greater(t, resp.Data.Tokens.ExpiresAt, int64(0))
			}
		})
	}
}

// TestLogin_ResponseShape 는 SPEC-AUTH-004 U1 회귀 방지 테스트이다.
// LoginResponse 의 JSON 직렬화 결과가 정확히 {user, tokens} 최상위 키만 가져야 하며,
// tokens 객체는 정확히 4개의 키 (access_token, refresh_token, expires_at, token_type) 만 가져야 한다.
func TestLogin_ResponseShape(t *testing.T) {
	resp := dto.LoginResponse{
		User: dto.UserInfoResponse{
			Username: "admin",
			Role:     "admin",
		},
		Tokens: dto.TokenPair{
			AccessToken:  "test-access-token",
			RefreshToken: "test-refresh-token",
			ExpiresAt:    1234567890,
			TokenType:    "Bearer",
		},
	}

	raw, err := json.Marshal(resp)
	require.NoError(t, err)

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &top))

	// 최상위 키는 정확히 user, tokens 두 개여야 한다.
	assert.Len(t, top, 2, "LoginResponse 의 최상위 키는 정확히 2개여야 한다")
	assert.Contains(t, top, "user")
	assert.Contains(t, top, "tokens")

	// tokens 는 객체이며 정확히 4개의 키를 가져야 한다.
	var tokens map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(top["tokens"], &tokens))
	assert.Len(t, tokens, 4, "tokens 객체의 키는 정확히 4개여야 한다")
	assert.Contains(t, tokens, "access_token")
	assert.Contains(t, tokens, "refresh_token")
	assert.Contains(t, tokens, "expires_at")
	assert.Contains(t, tokens, "token_type")
}

// TestRefresh_ResponseShape 는 SPEC-AUTH-004 O1 회귀 방지 테스트이다.
// RefreshResponse 는 LoginResponse 와 의도적으로 다른 flat 구조를 유지해야 하며,
// 최상위에 user 또는 tokens 키가 있어서는 안 된다.
func TestRefresh_ResponseShape(t *testing.T) {
	resp := dto.RefreshResponse{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		ExpiresAt:    1234567890,
		TokenType:    "Bearer",
	}

	raw, err := json.Marshal(resp)
	require.NoError(t, err)

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &top))

	// 최상위 키는 정확히 flat 4개여야 한다 (login 과의 비대칭 보존).
	assert.Len(t, top, 4, "RefreshResponse 의 최상위 키는 정확히 4개여야 한다")
	assert.Contains(t, top, "access_token")
	assert.Contains(t, top, "refresh_token")
	assert.Contains(t, top, "expires_at")
	assert.Contains(t, top, "token_type")

	// login 응답과 같은 중첩 키가 있어서는 안 된다.
	assert.NotContains(t, top, "user", "RefreshResponse 에 user 키가 있으면 O1 위반")
	assert.NotContains(t, top, "tokens", "RefreshResponse 에 tokens 키가 있으면 O1 위반")
}

func TestAuthHandler_Me(t *testing.T) {
	setup := newTestAuthSetup(t)

	// 먼저 로그인하여 토큰 획득
	w := doAuthRequest(setup.router, "POST", "/api/v1/auth/login",
		dto.LoginRequest{Username: "admin", Password: "password123"}, nil)
	require.Equal(t, http.StatusOK, w.Code)

	loginResp := parseResponse[dto.LoginResponse](t, w)
	token := loginResp.Data.Tokens.AccessToken

	t.Run("인증된 사용자 정보 조회", func(t *testing.T) {
		w := doAuthRequest(setup.router, "GET", "/api/v1/auth/me", nil,
			map[string]string{"Authorization": "Bearer " + token})
		assert.Equal(t, http.StatusOK, w.Code)

		resp := parseResponse[dto.UserInfoResponse](t, w)
		assert.True(t, resp.Success)
		assert.Equal(t, "admin", resp.Data.Username)
		assert.Equal(t, "admin", resp.Data.Role)
	})

	t.Run("인증 없이 접근", func(t *testing.T) {
		w := doAuthRequest(setup.router, "GET", "/api/v1/auth/me", nil, nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("잘못된 토큰", func(t *testing.T) {
		w := doAuthRequest(setup.router, "GET", "/api/v1/auth/me", nil,
			map[string]string{"Authorization": "Bearer invalid-token"})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestAuthHandler_Logout(t *testing.T) {
	setup := newTestAuthSetup(t)

	// 로그인
	w := doAuthRequest(setup.router, "POST", "/api/v1/auth/login",
		dto.LoginRequest{Username: "admin", Password: "password123"}, nil)
	require.Equal(t, http.StatusOK, w.Code)

	loginResp := parseResponse[dto.LoginResponse](t, w)
	token := loginResp.Data.Tokens.AccessToken

	// 로그아웃
	w = doAuthRequest(setup.router, "POST", "/api/v1/auth/logout", nil,
		map[string]string{"Authorization": "Bearer " + token})
	assert.Equal(t, http.StatusOK, w.Code)

	// 로그아웃 후 같은 토큰으로 접근 불가
	w = doAuthRequest(setup.router, "GET", "/api/v1/auth/me", nil,
		map[string]string{"Authorization": "Bearer " + token})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthHandler_Refresh(t *testing.T) {
	setup := newTestAuthSetup(t)

	// 로그인
	w := doAuthRequest(setup.router, "POST", "/api/v1/auth/login",
		dto.LoginRequest{Username: "admin", Password: "password123"}, nil)
	require.Equal(t, http.StatusOK, w.Code)

	loginResp := parseResponse[dto.LoginResponse](t, w)
	refreshToken := loginResp.Data.Tokens.RefreshToken

	t.Run("성공적인 토큰 갱신", func(t *testing.T) {
		w := doAuthRequest(setup.router, "POST", "/api/v1/auth/refresh",
			dto.RefreshRequest{RefreshToken: refreshToken}, nil)
		assert.Equal(t, http.StatusOK, w.Code)

		resp := parseResponse[dto.RefreshResponse](t, w)
		assert.True(t, resp.Success)
		assert.NotEmpty(t, resp.Data.AccessToken)
		assert.NotEmpty(t, resp.Data.RefreshToken)
		assert.Equal(t, "Bearer", resp.Data.TokenType)
	})

	t.Run("이미 사용된 리프레시 토큰 (토큰 회전)", func(t *testing.T) {
		// 위에서 사용한 리프레시 토큰은 블랙리스트에 추가됨
		w := doAuthRequest(setup.router, "POST", "/api/v1/auth/refresh",
			dto.RefreshRequest{RefreshToken: refreshToken}, nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("빈 리프레시 토큰", func(t *testing.T) {
		w := doAuthRequest(setup.router, "POST", "/api/v1/auth/refresh",
			dto.RefreshRequest{RefreshToken: ""}, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("잘못된 리프레시 토큰", func(t *testing.T) {
		w := doAuthRequest(setup.router, "POST", "/api/v1/auth/refresh",
			dto.RefreshRequest{RefreshToken: "invalid-refresh-token"}, nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestAuthHandler_ChangePassword(t *testing.T) {
	setup := newTestAuthSetup(t)

	// 로그인
	w := doAuthRequest(setup.router, "POST", "/api/v1/auth/login",
		dto.LoginRequest{Username: "admin", Password: "password123"}, nil)
	require.Equal(t, http.StatusOK, w.Code)

	loginResp := parseResponse[dto.LoginResponse](t, w)
	token := loginResp.Data.Tokens.AccessToken

	t.Run("성공적인 비밀번호 변경", func(t *testing.T) {
		w := doAuthRequest(setup.router, "PUT", "/api/v1/auth/password",
			dto.ChangePasswordRequest{
				CurrentPassword: "password123",
				NewPassword:     "newpassword456",
			},
			map[string]string{"Authorization": "Bearer " + token})
		assert.Equal(t, http.StatusOK, w.Code)

		// 새 비밀번호로 로그인
		w = doAuthRequest(setup.router, "POST", "/api/v1/auth/login",
			dto.LoginRequest{Username: "admin", Password: "newpassword456"}, nil)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("잘못된 현재 비밀번호", func(t *testing.T) {
		w := doAuthRequest(setup.router, "PUT", "/api/v1/auth/password",
			dto.ChangePasswordRequest{
				CurrentPassword: "wrongpassword",
				NewPassword:     "newpassword789",
			},
			map[string]string{"Authorization": "Bearer " + token})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("인증 없이 접근", func(t *testing.T) {
		w := doAuthRequest(setup.router, "PUT", "/api/v1/auth/password",
			dto.ChangePasswordRequest{
				CurrentPassword: "newpassword456",
				NewPassword:     "another",
			}, nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		want       string
	}{
		{"Bearer 토큰", "Bearer abc123", "abc123"},
		{"빈 헤더", "", ""},
		{"Bearer 없음", "Basic abc123", ""},
		{"Bearer만", "Bearer ", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractBearerToken(tc.authHeader)
			assert.Equal(t, tc.want, got)
		})
	}
}
