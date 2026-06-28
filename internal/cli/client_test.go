package cli

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

// --- TLS / insecure tests ---

// TestClient_WithInsecure_SkipsTLSVerification - WithInsecure(true) 적용 시
// 자체 서명 인증서 검증을 건너뛰어 요청이 성공하는지 검증한다.
// 동시에 WithInsecure 없이는 x509 검증 실패로 에러가 발생하는지 검증한다.
func TestClient_WithInsecure_SkipsTLSVerification(t *testing.T) {
	// httptest.NewTLSServer 는 자체 서명 인증서를 사용한다.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSuccessResponse(w, map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	// 1) WithInsecure 미적용: 기본 트랜스포트는 자체 서명 인증서를 신뢰하지 않으므로
	//    TLS/cert 에러가 발생해야 한다.
	secureClient := NewClient(srv.URL, "", 5*time.Second, false)
	var secureResult map[string]string
	secureErr := secureClient.Get("/test", &secureResult)
	require.Error(t, secureErr,
		"WithInsecure 없이 자체 서명 HTTPS 서버 요청은 TLS 에러를 반환해야 합니다")

	cliErr, ok := secureErr.(*CLIError)
	require.True(t, ok, "에러가 CLIError 타입이어야 합니다")
	assert.Contains(t, cliErr.Hint, "--insecure",
		"TLS 검증 실패 에러의 힌트는 --insecure 사용을 안내해야 합니다")

	// 2) WithInsecure(true) 적용: 인증서 검증을 건너뛰므로 요청이 성공해야 한다.
	insecureClient := NewClient(srv.URL, "", 5*time.Second, false, WithInsecure(true))
	var insecureResult map[string]string
	insecureErr := insecureClient.Get("/test", &insecureResult)
	require.NoError(t, insecureErr,
		"WithInsecure(true) 적용 시 자체 서명 HTTPS 서버 요청이 성공해야 합니다")
	assert.Equal(t, "ok", insecureResult["status"])
}

// TestClient_WithInsecure_False_KeepsDefaultTransport - WithInsecure(false) 는
// 기본 트랜스포트를 유지하여 평문 HTTP 서버 통신에 영향이 없는지 검증한다.
func TestClient_WithInsecure_False_KeepsDefaultTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSuccessResponse(w, map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", 5*time.Second, false, WithInsecure(false))
	var result map[string]string
	err := c.Get("/test", &result)
	require.NoError(t, err,
		"WithInsecure(false) 는 평문 HTTP 통신에 영향을 주지 않아야 합니다")
	assert.Equal(t, "ok", result["status"])
}

// TestClient_WrapConnectionError_TLSHint - wrapConnectionError 가 x509 인증서
// 검증 실패 에러에 대해 --insecure 힌트를 포함한 CLIError 를 생성하는지 검증한다.
func TestClient_WrapConnectionError_TLSHint(t *testing.T) {
	c := NewClient("https://localhost:8443", "", 5*time.Second, false)
	synthetic := errors.New("Get \"https://localhost:8443/health\": x509: certificate signed by unknown authority")

	err := c.wrapConnectionError(synthetic)
	cliErr, ok := err.(*CLIError)
	require.True(t, ok, "에러가 CLIError 타입이어야 합니다")
	assert.Contains(t, cliErr.Message, "TLS 인증서",
		"TLS 검증 실패 메시지가 포함되어야 합니다")
	assert.Contains(t, cliErr.Hint, "--insecure",
		"힌트가 --insecure 옵션을 안내해야 합니다")
	assert.Equal(t, synthetic, cliErr.Cause,
		"원인 에러가 보존되어야 합니다")
	assert.Equal(t, 1, cliErr.ExitCode,
		"종료 코드는 1 이어야 합니다")
}

// TestClient_WrapConnectionError_NonTLS - wrapConnectionError 가 일반 연결 에러에
// 대해 기존 서버 연결 실패 메시지를 유지하는지 검증한다.
func TestClient_WrapConnectionError_NonTLS(t *testing.T) {
	c := NewClient("http://localhost:8080", "", 5*time.Second, false)
	synthetic := errors.New("dial tcp 127.0.0.1:8080: connect: connection refused")

	err := c.wrapConnectionError(synthetic)
	cliErr, ok := err.(*CLIError)
	require.True(t, ok, "에러가 CLIError 타입이어야 합니다")
	assert.Contains(t, cliErr.Message, "서버에 연결할 수 없습니다",
		"일반 연결 에러는 기존 메시지를 유지해야 합니다")
	assert.NotContains(t, cliErr.Hint, "--insecure",
		"일반 연결 에러 힌트에는 --insecure 안내가 없어야 합니다")
}

// TestClient_WrapConnectionError_TLSPrefix - "tls:" 프리픽스 에러도 TLS 에러로
// 판별하여 --insecure 힌트를 제공하는지 검증한다.
func TestClient_WrapConnectionError_TLSPrefix(t *testing.T) {
	c := NewClient("https://localhost:8443", "", 5*time.Second, false)
	synthetic := errors.New("remote error: tls: handshake failure")

	err := c.wrapConnectionError(synthetic)
	cliErr, ok := err.(*CLIError)
	require.True(t, ok)
	assert.Contains(t, cliErr.Hint, "--insecure",
		"tls: 프리픽스 에러도 --insecure 힌트를 제공해야 합니다")
}

// TestIsTLSError_Nil - nil 에러는 TLS 에러가 아님을 검증한다.
func TestIsTLSError_Nil(t *testing.T) {
	assert.False(t, isTLSError(nil),
		"nil 에러는 TLS 에러로 판별되지 않아야 합니다")
}

// TestWithInsecure_SetsTransport - WithInsecure(true) 가 InsecureSkipVerify 트랜스포트를
// 설정하고, false 는 기본(nil) 트랜스포트를 유지하는지 검증한다.
func TestWithInsecure_SetsTransport(t *testing.T) {
	insecure := NewClient("https://localhost", "", 5*time.Second, false, WithInsecure(true))
	transport, ok := insecure.httpClient.Transport.(*http.Transport)
	require.True(t, ok, "insecure 클라이언트는 *http.Transport 를 가져야 합니다")
	require.NotNil(t, transport.TLSClientConfig,
		"TLSClientConfig 가 설정되어야 합니다")
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify,
		"InsecureSkipVerify 가 true 여야 합니다")

	secure := NewClient("https://localhost", "", 5*time.Second, false, WithInsecure(false))
	assert.Nil(t, secure.httpClient.Transport,
		"WithInsecure(false) 는 기본 트랜스포트(nil)를 유지해야 합니다")
}

// --- HTTP -> HTTPS auto-upgrade tests ---

// TestClient_AutoUpgrade_HTTPToHTTPS_WithInsecure - http:// 로 HTTPS 서버에 요청 시
// 서버가 반환하는 400 "Client sent an HTTP request to an HTTPS server" 본문을 감지하여
// 동일 호스트/포트로 https:// 재시도가 투명하게 수행되고 성공하는지 검증한다.
// 또한 업그레이드가 클라이언트 수명 동안 유지되어 baseURL 이 https:// 로 변경되는지 확인한다.
func TestClient_AutoUpgrade_HTTPToHTTPS_WithInsecure(t *testing.T) {
	// httptest.NewTLSServer 는 자체 서명 인증서를 사용한다.
	// Go 의 TLS 서버는 평문 HTTP 요청에 대해 자동으로 400 +
	// "Client sent an HTTP request to an HTTPS server" 본문을 응답한다.
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSuccessResponse(w, map[string]bool{"ok": true})
	}))
	defer ts.Close()

	// TLS 서버의 포트로 향하는 http:// URL 을 만든다.
	httpURL := strings.Replace(ts.URL, "https://", "http://", 1)

	// 자체 서명 인증서이므로 insecure 로 검증을 건너뛴다.
	c := NewClient(httpURL, "", 5*time.Second, false, WithInsecure(true))

	var result struct {
		OK bool `json:"ok"`
	}
	err := c.Get("/health", &result)
	require.NoError(t, err,
		"http:// 요청은 https:// 로 투명하게 재시도되어 성공해야 합니다")
	assert.True(t, result.OK,
		"재시도된 https:// 응답의 데이터가 올바르게 디코딩되어야 합니다")
	assert.True(t, strings.HasPrefix(c.baseURL, "https://"),
		"업그레이드가 유지되어 baseURL 이 https:// 로 변경되어야 합니다")
}

// TestClient_AutoUpgrade_WithoutInsecure_FailsWithTLSHint - WithInsecure 없이
// 동일 시나리오에서는 업그레이드는 수행되지만 자체 서명 인증서 검증 실패로
// --insecure 힌트를 포함한 에러가 반환되는지 검증한다.
func TestClient_AutoUpgrade_WithoutInsecure_FailsWithTLSHint(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSuccessResponse(w, map[string]bool{"ok": true})
	}))
	defer ts.Close()

	httpURL := strings.Replace(ts.URL, "https://", "http://", 1)

	c := NewClient(httpURL, "", 5*time.Second, false)

	var result struct {
		OK bool `json:"ok"`
	}
	err := c.Get("/health", &result)
	require.Error(t, err,
		"WithInsecure 없이는 https:// 재시도에서 인증서 검증 실패로 에러가 발생해야 합니다")

	cliErr, ok := err.(*CLIError)
	require.True(t, ok, "에러가 CLIError 타입이어야 합니다")
	assert.Contains(t, cliErr.Hint, "--insecure",
		"인증서 검증 실패 에러의 힌트는 --insecure 사용을 안내해야 합니다")
}

// TestClient_AutoUpgrade_Normal400NotUpgraded - 일반 400 응답(HTTPS 불일치 마커가
// 없는 경우)은 업그레이드되지 않으며, 소비한 본문이 복원되어 호출자가 정상적으로
// 읽고 에러로 매핑할 수 있는지 검증한다.
func TestClient_AutoUpgrade_Normal400NotUpgraded(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   map[string]string{"code": "BAD_REQUEST", "message": "잘못된 요청입니다"},
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "", 5*time.Second, false)

	var result any
	err := c.Get("/api/invalid", &result)
	require.Error(t, err, "일반 400 응답은 에러를 반환해야 합니다")

	cliErr, ok := err.(*CLIError)
	require.True(t, ok, "에러가 CLIError 타입이어야 합니다")
	assert.Contains(t, cliErr.Message, "잘못된 요청입니다",
		"본문이 복원되어 호출자가 에러 메시지를 읽을 수 있어야 합니다")
	assert.Equal(t, ts.URL, c.baseURL,
		"일반 400 은 업그레이드를 트리거하지 않아야 합니다")
}

// TestClient_AutoUpgrade_400BodyReadError_FallsBack - http:// 400 응답의 본문 읽기가
// 실패하면(예: Content-Length 보다 짧은 본문 후 연결 단절) 업그레이드 판단을 포기하고
// 원본 응답을 안전하게 반환하는지 검증한다. 호출자는 빈 본문으로 인해 에러로 매핑된다.
func TestClient_AutoUpgrade_400BodyReadError_FallsBack(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 실제 본문보다 큰 Content-Length 를 선언한 뒤 일부만 쓰고 연결을 끊어
		// 클라이언트의 io.ReadAll 이 unexpected EOF 로 실패하도록 유도한다.
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("short"))
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, hijErr := hj.Hijack()
			if hijErr == nil {
				_ = conn.Close()
			}
		}
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "", 5*time.Second, false)

	var result any
	err := c.Get("/api/truncated", &result)
	require.Error(t, err,
		"본문 읽기 실패 후 빈 본문으로 400 이 반환되어 에러로 매핑되어야 합니다")
	assert.Equal(t, ts.URL, c.baseURL,
		"본문 읽기 실패 시 업그레이드는 수행되지 않아야 합니다")
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
