package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- NewClient tests ---

// TestNewClient - 클라이언트 생성 및 필드 초기화 검증
func TestNewClient(t *testing.T) {
	c := NewClient("http://localhost:8080", "test-token", 10*time.Second, true)

	require.NotNil(t, c, "클라이언트가 nil 이면 안됩니다")
	assert.Equal(t, "http://localhost:8080", c.baseURL,
		"baseURL 이 올바르게 설정되어야 합니다")
	assert.Equal(t, "test-token", c.token,
		"token 이 올바르게 설정되어야 합니다")
	assert.Equal(t, true, c.verbose,
		"verbose 가 올바르게 설정되어야 합니다")
	assert.NotNil(t, c.httpClient,
		"httpClient 가 nil 이면 안됩니다")
	assert.Equal(t, 10*time.Second, c.httpClient.Timeout,
		"timeout 이 올바르게 설정되어야 합니다")
}

// TestNewClient_DefaultTimeout - 타임아웃 0 입력 시 기본값 30초 적용 검증
func TestNewClient_DefaultTimeout(t *testing.T) {
	c := NewClient("http://localhost", "", 0, false)

	assert.Equal(t, 30*time.Second, c.httpClient.Timeout,
		"타임아웃이 0 이면 기본값 30초가 적용되어야 합니다")
}

// --- Authorization header tests ---

// TestClient_AuthHeader - Authorization 헤더 자동 주입 검증
func TestClient_AuthHeader(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		writeSuccessResponse(w, "ok")
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "my-secret-token", 5*time.Second, false)
	var result string
	err := c.Get("/test", &result)
	require.NoError(t, err)
	assert.Equal(t, "Bearer my-secret-token", receivedAuth,
		"Authorization 헤더가 Bearer 형식으로 설정되어야 합니다")
}

// TestClient_NoAuthHeader - 토큰이 비어있을 때 Authorization 헤더 미설정 검증
func TestClient_NoAuthHeader(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		writeSuccessResponse(w, "ok")
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", 5*time.Second, false)
	var result string
	err := c.Get("/test", &result)
	require.NoError(t, err)
	assert.Empty(t, receivedAuth,
		"토큰이 비어있으면 Authorization 헤더가 설정되지 않아야 합니다")
}

// TestClient_ContentTypeHeaders - Content-Type 및 Accept 헤더 검증
func TestClient_ContentTypeHeaders(t *testing.T) {
	var contentType, accept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		accept = r.Header.Get("Accept")
		writeSuccessResponse(w, nil)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", 5*time.Second, false)
	err := c.Post("/test", map[string]string{"key": "value"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "application/json", contentType,
		"Content-Type 이 application/json 이어야 합니다")
	assert.Equal(t, "application/json", accept,
		"Accept 가 application/json 이어야 합니다")
}

// --- HTTP method tests ---

// TestClient_Get - GET 요청 및 응답 디코딩 검증
func TestClient_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "HTTP 메서드가 GET 이어야 합니다")
		assert.Equal(t, "/api/workflows", r.URL.Path, "경로가 올바라야 합니다")
		writeSuccessResponse(w, map[string]string{"id": "wf-1", "name": "테스트"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", 5*time.Second, false)
	var result map[string]string
	err := c.Get("/api/workflows", &result)
	require.NoError(t, err, "GET 요청 에러가 없어야 합니다")
	assert.Equal(t, "wf-1", result["id"])
	assert.Equal(t, "테스트", result["name"])
}

// TestClient_Post - POST 요청 및 본문 전송 검증
func TestClient_Post(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method, "HTTP 메서드가 POST 여야 합니다")

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var reqBody map[string]string
		err = json.Unmarshal(body, &reqBody)
		require.NoError(t, err)
		assert.Equal(t, "새 워크플로우", reqBody["name"],
			"요청 본문이 올바르게 전송되어야 합니다")

		writeSuccessResponse(w, map[string]string{"id": "wf-new"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", 5*time.Second, false)
	var result map[string]string
	err := c.Post("/api/workflows", map[string]string{"name": "새 워크플로우"}, &result)
	require.NoError(t, err, "POST 요청 에러가 없어야 합니다")
	assert.Equal(t, "wf-new", result["id"])
}

// TestClient_Put - PUT 요청 검증
func TestClient_Put(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method, "HTTP 메서드가 PUT 이어야 합니다")
		writeSuccessResponse(w, map[string]string{"updated": "true"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", 5*time.Second, false)
	var result map[string]string
	err := c.Put("/api/workflows/wf-1", map[string]string{"name": "수정됨"}, &result)
	require.NoError(t, err, "PUT 요청 에러가 없어야 합니다")
	assert.Equal(t, "true", result["updated"])
}

// TestClient_Delete - DELETE 요청 검증
func TestClient_Delete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method, "HTTP 메서드가 DELETE 여야 합니다")
		writeSuccessResponse(w, nil)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", 5*time.Second, false)
	err := c.Delete("/api/workflows/wf-1", nil)
	require.NoError(t, err, "DELETE 요청 에러가 없어야 합니다")
}

// --- Raw response tests ---

// TestClient_GetRaw - GetRaw 의 원시 바이트 응답 검증
func TestClient_GetRaw(t *testing.T) {
	expectedBody := []byte(`{"raw":"data"}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(expectedBody)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", 5*time.Second, false)
	raw, err := c.GetRaw("/export")
	require.NoError(t, err, "GetRaw 에러가 없어야 합니다")
	assert.Equal(t, expectedBody, raw,
		"원시 바이트가 그대로 반환되어야 합니다")
}

// TestClient_PostRaw - PostRaw 의 원시 바이트 응답 검증
func TestClient_PostRaw(t *testing.T) {
	expectedBody := []byte(`{"result":"created"}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(expectedBody)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", 5*time.Second, false)
	raw, err := c.PostRaw("/import", map[string]string{"data": "value"})
	require.NoError(t, err, "PostRaw 에러가 없어야 합니다")
	assert.Equal(t, expectedBody, raw)
}

// --- Error mapping tests ---

// TestClient_ErrorMapping_401 - 401 응답의 인증 실패 에러 매핑 검증
func TestClient_ErrorMapping_401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   map[string]string{"code": "UNAUTHORIZED", "message": "토큰 만료"},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "expired-token", 5*time.Second, false)
	var result any
	err := c.Get("/protected", &result)
	require.Error(t, err, "401 응답은 에러를 반환해야 합니다")

	cliErr, ok := err.(*CLIError)
	require.True(t, ok, "에러가 CLIError 타입이어야 합니다")
	assert.Equal(t, "인증에 실패했습니다. 토큰을 확인해주세요", cliErr.Message)
}

// TestClient_ErrorMapping_403 - 403 응답의 권한 거부 에러 매핑 검증
func TestClient_ErrorMapping_403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   map[string]string{"code": "FORBIDDEN", "message": "권한 없음"},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "token", 5*time.Second, false)
	err := c.Get("/admin", nil)
	require.Error(t, err)

	cliErr, ok := err.(*CLIError)
	require.True(t, ok)
	assert.Equal(t, "이 작업을 수행할 권한이 없습니다", cliErr.Message)
}

// TestClient_ErrorMapping_404 - 404 응답의 에러 메시지 매핑 검증
func TestClient_ErrorMapping_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   map[string]string{"code": "NOT_FOUND", "message": "워크플로우를 찾을 수 없습니다"},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", 5*time.Second, false)
	err := c.Get("/api/workflows/missing", nil)
	require.Error(t, err)

	cliErr, ok := err.(*CLIError)
	require.True(t, ok)
	assert.Contains(t, cliErr.Message, "워크플로우를 찾을 수 없습니다")
}

// TestClient_ErrorMapping_500 - 500 응답의 에러 매핑 검증
func TestClient_ErrorMapping_500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   map[string]string{"code": "INTERNAL", "message": "내부 서버 에러"},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", 5*time.Second, false)
	err := c.Get("/api/broken", nil)
	require.Error(t, err)

	cliErr, ok := err.(*CLIError)
	require.True(t, ok)
	assert.Contains(t, cliErr.Message, "내부 서버 에러")
}

// --- Ping tests ---

// TestClient_Ping_Success - 서버 정상 연결 시 Ping 성공 검증
func TestClient_Ping_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/health", r.URL.Path, "Ping 은 /health 경로로 요청해야 합니다")
		writeSuccessResponse(w, map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", 5*time.Second, false)
	err := c.Ping()
	assert.NoError(t, err, "정상 서버에 대한 Ping 은 에러가 없어야 합니다")
}

// TestClient_Ping_Failure - 서버 연결 불가 시 Ping 실패 검증
func TestClient_Ping_Failure(t *testing.T) {
	c := NewClient("http://localhost:1", "", 1*time.Second, false)
	err := c.Ping()
	require.Error(t, err, "연결 불가 서버에 대한 Ping 은 에러를 반환해야 합니다")

	cliErr, ok := err.(*CLIError)
	require.True(t, ok, "에러가 CLIError 타입이어야 합니다")
	assert.Contains(t, cliErr.Message, "서버에 연결할 수 없습니다",
		"서버 연결 실패 메시지가 포함되어야 합니다")
}

// --- Timeout test ---

// TestClient_Timeout - 요청 타임아웃 처리 검증
func TestClient_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		writeSuccessResponse(w, nil)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", 50*time.Millisecond, false)
	err := c.Get("/slow", nil)
	require.Error(t, err, "타임아웃이 발생하면 에러를 반환해야 합니다")
}

// --- Verbose logging test ---

// TestClient_VerboseLogging - verbose 모드에서 로깅이 동작하는지 검증
func TestClient_VerboseLogging(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSuccessResponse(w, nil)
	}))
	defer srv.Close()

	// verbose=true should not cause any errors (just logs to stderr)
	c := NewClient(srv.URL, "", 5*time.Second, true)
	err := c.Get("/test", nil)
	assert.NoError(t, err, "verbose 모드에서 에러가 발생하면 안됩니다")
}

// TestClient_Get_NilResult - result 가 nil 인 GET 요청 검증
func TestClient_Get_NilResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSuccessResponse(w, map[string]string{"id": "test"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", 5*time.Second, false)
	err := c.Get("/api/test", nil)
	assert.NoError(t, err, "result 가 nil 이어도 에러가 없어야 합니다")
}

// TestClient_Post_NilBody - body 가 nil 인 POST 요청 검증
func TestClient_Post_NilBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		writeSuccessResponse(w, nil)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", 5*time.Second, false)
	err := c.Post("/api/action", nil, nil)
	assert.NoError(t, err, "nil body POST 요청에 에러가 없어야 합니다")
}

// --- Helper functions ---

// writeSuccessResponse writes a standard success API response.
func writeSuccessResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"data":    data,
	})
}
