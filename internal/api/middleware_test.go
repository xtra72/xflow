package api

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/config"
)

func TestRequestID_GeneratesUUID(t *testing.T) {
	r := NewRouter()
	r.Use(RequestID())
	var requestID string

	r.GET("/test", func(ctx Context) error {
		requestID = ctx.RequestID()
		return ctx.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, requestID)

	// UUID v4 형식 확인 (8-4-4-4-12)
	parts := strings.Split(requestID, "-")
	assert.Len(t, parts, 5)

	// 응답 헤더에도 설정되어야 한다
	assert.Equal(t, requestID, rec.Header().Get("X-Request-ID"))
}

func TestRequestID_ReusesExisting(t *testing.T) {
	r := NewRouter()
	r.Use(RequestID())
	var requestID string

	r.GET("/test", func(ctx Context) error {
		requestID = ctx.RequestID()
		return ctx.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", "existing-id-123")
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, "existing-id-123", requestID)
	assert.Equal(t, "existing-id-123", rec.Header().Get("X-Request-ID"))
}

func TestRequestID_UniquenessPerRequest(t *testing.T) {
	r := NewRouter()
	r.Use(RequestID())
	var ids []string
	var mu sync.Mutex

	r.GET("/test", func(ctx Context) error {
		mu.Lock()
		ids = append(ids, ctx.RequestID())
		mu.Unlock()
		return ctx.NoContent(http.StatusOK)
	})

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		r.Handler().ServeHTTP(rec, req)
	}

	// 모든 ID가 고유해야 한다
	assert.Len(t, ids, 10)
	unique := make(map[string]bool)
	for _, id := range ids {
		unique[id] = true
	}
	assert.Len(t, unique, 10)
}

func TestLogger_LogsRequest(t *testing.T) {
	r := NewRouter()
	r.Use(Logger(slog.Default()))

	r.GET("/test", func(ctx Context) error {
		return ctx.JSON(http.StatusOK, map[string]string{"ok": "true"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	// 로거가 패닉 없이 실행되어야 한다
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestLogger_RecordsStatusCode(t *testing.T) {
	r := NewRouter()
	r.Use(Logger(slog.Default()))

	r.GET("/test", func(ctx Context) error {
		return ctx.JSON(http.StatusCreated, map[string]string{"created": "true"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestRecovery_RecoversPanic(t *testing.T) {
	r := NewRouter()
	r.Use(Recovery(slog.Default()))

	r.GET("/panic", func(ctx Context) error {
		panic("test panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	// 패닉이 복구되어 500 응답이 반환되어야 한다
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestRecovery_PassesThrough(t *testing.T) {
	r := NewRouter()
	r.Use(Recovery(slog.Default()))

	r.GET("/ok", func(ctx Context) error {
		return ctx.JSON(http.StatusOK, map[string]string{"ok": "true"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRecovery_NilPanic(t *testing.T) {
	r := NewRouter()
	r.Use(Recovery(slog.Default()))

	r.GET("/nil-panic", func(ctx Context) error {
		panic(nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/nil-panic", nil)
	rec := httptest.NewRecorder()

	// Go 1.21+ 에서 panic(nil)은 *runtime.PanicNilError를 발생시킨다
	// Recovery 미들웨어가 이를 처리해야 한다
	r.Handler().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestCORS_AllowedOrigin(t *testing.T) {
	r := NewRouter()
	r.Use(CORS(config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"http://example.com"},
	}))

	r.GET("/test", func(ctx Context) error {
		return ctx.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "http://example.com", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.NotEmpty(t, rec.Header().Get("Access-Control-Allow-Methods"))
	assert.NotEmpty(t, rec.Header().Get("Access-Control-Allow-Headers"))
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	r := NewRouter()
	r.Use(CORS(config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"http://example.com"},
	}))

	r.GET("/test", func(ctx Context) error {
		return ctx.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://evil.com")
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_WildcardOrigin(t *testing.T) {
	r := NewRouter()
	r.Use(CORS(config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
	}))

	r.GET("/test", func(ctx Context) error {
		return ctx.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://any-origin.com")
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, "http://any-origin.com", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_PreflightRequest(t *testing.T) {
	r := NewRouter()
	r.Use(CORS(config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
	}))

	// OPTIONS 핸들러를 등록하지 않아도 CORS 미들웨어가 처리해야 한다
	r.GET("/test", func(ctx Context) error {
		return ctx.NoContent(http.StatusOK)
	})

	// OPTIONS 라우트 직접 등록 (Go 1.22+ ServeMux에서 필요)
	r.mux.HandleFunc("OPTIONS /test", func(w http.ResponseWriter, req *http.Request) {
		ctx := newHTTPContext(w, req)
		chain := applyMiddleware(func(c Context) error {
			return c.NoContent(http.StatusOK)
		}, r.middlewares)
		if err := chain(ctx); err != nil {
			handleError(ctx, err)
		}
	})

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "http://example.com", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_NoOriginHeader(t *testing.T) {
	r := NewRouter()
	r.Use(CORS(config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
	}))

	r.GET("/test", func(ctx Context) error {
		return ctx.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// Origin 헤더 없음
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	// Origin이 없으면 CORS 헤더가 설정되지 않아야 한다
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestRateLimit_AllowsRequests(t *testing.T) {
	r := NewRouter()
	r.Use(RateLimit(config.RateLimitConfig{
		Enabled:           true,
		RequestsPerSecond: 100,
	}))

	r.GET("/test", func(ctx Context) error {
		return ctx.NoContent(http.StatusOK)
	})

	// 첫 번째 요청은 허용되어야 한다
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRateLimit_ExceedsLimit(t *testing.T) {
	r := NewRouter()
	r.Use(RateLimit(config.RateLimitConfig{
		Enabled:           true,
		RequestsPerSecond: 2,
	}))

	r.GET("/test", func(ctx Context) error {
		return ctx.NoContent(http.StatusOK)
	})

	// 초당 2개 제한으로, 빠르게 보내면 초과해야 한다
	var rateLimited bool
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()
		r.Handler().ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			rateLimited = true
			assert.NotEmpty(t, rec.Header().Get("Retry-After"))
			break
		}
	}
	assert.True(t, rateLimited, "속도 제한이 트리거되어야 한다")
}

func TestRateLimit_DifferentClients(t *testing.T) {
	r := NewRouter()
	r.Use(RateLimit(config.RateLimitConfig{
		Enabled:           true,
		RequestsPerSecond: 1,
	}))

	r.GET("/test", func(ctx Context) error {
		return ctx.NoContent(http.StatusOK)
	})

	// 첫 번째 클라이언트
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "192.168.1.1:12345"
	rec1 := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)

	// 두 번째 클라이언트 (다른 IP)
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "192.168.1.2:12345"
	rec2 := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)
}

func TestRateLimit_ConcurrentAccess(t *testing.T) {
	r := NewRouter()
	r.Use(RateLimit(config.RateLimitConfig{
		Enabled:           true,
		RequestsPerSecond: 100,
	}))

	r.GET("/test", func(ctx Context) error {
		return ctx.NoContent(http.StatusOK)
	})

	// 동시 요청 테스트
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = "192.168.1.1:12345"
			rec := httptest.NewRecorder()
			r.Handler().ServeHTTP(rec, req)
			// 패닉 없이 응답해야 한다
			assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusTooManyRequests)
		}(i)
	}
	wg.Wait()
}

func TestTimeout_PassesThrough(t *testing.T) {
	r := NewRouter()
	r.Use(Timeout(5 * time.Second))

	r.GET("/test", func(ctx Context) error {
		return ctx.JSON(http.StatusOK, map[string]string{"ok": "true"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestTimeout_ExceedsLimit(t *testing.T) {
	r := NewRouter()
	r.Use(Timeout(50 * time.Millisecond))

	r.GET("/slow", func(ctx Context) error {
		select {
		case <-ctx.Context().Done():
			return nil
		case <-time.After(5 * time.Second):
			return ctx.JSON(http.StatusOK, map[string]string{"ok": "true"})
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestTimeout, rec.Code)
}

func TestCompress_GzipResponse(t *testing.T) {
	r := NewRouter()
	r.Use(Compress())

	r.GET("/test", func(ctx Context) error {
		return ctx.JSON(http.StatusOK, map[string]string{"data": "compressed response content"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))

	// gzip 디코딩 확인
	reader, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	require.NoError(t, err)

	var body map[string]string
	err = json.Unmarshal(decompressed, &body)
	require.NoError(t, err)
	assert.Equal(t, "compressed response content", body["data"])
}

func TestCompress_ErrorPath_NoGzip(t *testing.T) {
	// 핸들러가 에러를 반환하면 gzip 압축 없이 plain JSON 에러 응답이 와야 한다.
	r := NewRouter()
	r.Use(Compress())

	r.GET("/test", func(ctx Context) error {
		return ErrNotFound.WithMessage("flow not found")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	// 에러 경로에서는 Content-Encoding: gzip 이 없어야 한다
	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// plain JSON 으로 디코딩 가능해야 한다
	var body map[string]any
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, false, body["success"])
}

func TestCompress_NoGzipWithoutHeader(t *testing.T) {
	r := NewRouter()
	r.Use(Compress())

	r.GET("/test", func(ctx Context) error {
		return ctx.JSON(http.StatusOK, map[string]string{"data": "plain"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// Accept-Encoding 헤더 없음
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Header().Get("Content-Encoding"))

	// 일반 JSON으로 디코딩 가능해야 한다
	var body map[string]string
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "plain", body["data"])
}

func TestAuth_PassThrough(t *testing.T) {
	r := NewRouter()
	r.Use(Auth(false, nil))

	r.GET("/test", func(ctx Context) error {
		return ctx.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequireRole_PassThrough(t *testing.T) {
	r := NewRouter()
	r.Use(RequireRole("admin", "user"))

	r.GET("/test", func(ctx Context) error {
		return ctx.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// --- rateLimiter 단위 테스트 ---

func TestNewRateLimiter(t *testing.T) {
	tests := []struct {
		name     string
		rate     int
		expected int
	}{
		{"positive rate", 10, 10},
		{"zero rate uses default", 0, 10},
		{"negative rate uses default", -1, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := newRateLimiter(tt.rate)
			assert.Equal(t, tt.expected, rl.rate)
		})
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	rl := newRateLimiter(5)

	// 첫 5개 요청은 허용
	for i := 0; i < 5; i++ {
		assert.True(t, rl.allow("client1"), "요청 %d는 허용되어야 한다", i)
	}

	// 6번째 요청은 거부
	assert.False(t, rl.allow("client1"), "6번째 요청은 거부되어야 한다")
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	rl := newRateLimiter(10)

	// 모든 토큰 소진
	for i := 0; i < 10; i++ {
		rl.allow("client1")
	}
	assert.False(t, rl.allow("client1"))

	// 시간 경과 후 토큰 리필
	time.Sleep(200 * time.Millisecond)
	assert.True(t, rl.allow("client1"), "시간이 지나면 토큰이 리필되어야 한다")
}

func TestRateLimiter_DifferentClients(t *testing.T) {
	rl := newRateLimiter(1)

	assert.True(t, rl.allow("client1"))
	assert.False(t, rl.allow("client1"))

	// 다른 클라이언트는 독립적
	assert.True(t, rl.allow("client2"))
}

// --- statusRecorderWriter 테스트 ---

func TestStatusRecorderWriter_DefaultStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	srw := &statusRecorderWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	assert.Equal(t, http.StatusOK, srw.statusCode)
}

func TestStatusRecorderWriter_WriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	srw := &statusRecorderWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	srw.WriteHeader(http.StatusCreated)
	assert.Equal(t, http.StatusCreated, srw.statusCode)
	assert.True(t, srw.written)
}

func TestStatusRecorderWriter_Write(t *testing.T) {
	rec := httptest.NewRecorder()
	srw := &statusRecorderWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	n, err := srw.Write([]byte("hello"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.True(t, srw.written)
}

func TestStatusRecorderWriter_Unwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	srw := &statusRecorderWriter{ResponseWriter: rec}

	unwrapped := srw.Unwrap()
	assert.Equal(t, rec, unwrapped)
}

func TestStatusRecorderWriter_DoubleWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	srw := &statusRecorderWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	srw.WriteHeader(http.StatusCreated)
	srw.WriteHeader(http.StatusNotFound) // 두 번째 호출은 statusCode를 변경하지 않아야 한다

	assert.Equal(t, http.StatusCreated, srw.statusCode)
}

// --- gzipResponseWriter 테스트 ---

func TestGzipResponseWriter_Write(t *testing.T) {
	rec := httptest.NewRecorder()
	gw, err := gzip.NewWriterLevel(rec, gzip.DefaultCompression)
	require.NoError(t, err)

	grw := &gzipResponseWriter{
		ResponseWriter: rec,
		writer:         gw,
	}

	n, err := grw.Write([]byte("hello"))
	assert.NoError(t, err)
	assert.Greater(t, n, 0)

	_ = gw.Close()

	// gzip으로 압축된 데이터를 읽기
	reader, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	defer reader.Close()

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

func TestGzipResponseWriter_Unwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	grw := &gzipResponseWriter{
		ResponseWriter: rec,
		writer:         rec,
	}

	unwrapped := grw.Unwrap()
	assert.Equal(t, rec, unwrapped)
}

// --- 컨텍스트 키 테스트 ---

func TestContextKeys(t *testing.T) {
	// 컨텍스트 키가 서로 다른지 확인
	assert.NotEqual(t, ctxKeyRequestID, ctxKeyUserID)
	assert.NotEqual(t, ctxKeyRequestID, ctxKeyUserRole)
	assert.NotEqual(t, ctxKeyUserID, ctxKeyUserRole)
}

func TestContextKeys_InContext(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxKeyRequestID, "req-123")
	ctx = context.WithValue(ctx, ctxKeyUserID, "user-456")
	ctx = context.WithValue(ctx, ctxKeyUserRole, "admin")

	assert.Equal(t, "req-123", ctx.Value(ctxKeyRequestID))
	assert.Equal(t, "user-456", ctx.Value(ctxKeyUserID))
	assert.Equal(t, "admin", ctx.Value(ctxKeyUserRole))
}

// --- 미들웨어 체인 통합 테스트 ---

func TestMiddlewareChain_FullStack(t *testing.T) {
	r := NewRouter()
	r.Use(
		Recovery(slog.Default()),
		RequestID(),
		Logger(slog.Default()),
		Auth(false, nil),
	)

	r.GET("/test", func(ctx Context) error {
		assert.NotEmpty(t, ctx.RequestID())
		return ctx.JSON(http.StatusOK, map[string]string{"ok": "true"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))
}

func TestMiddlewareChain_RecoveryWithRequestID(t *testing.T) {
	r := NewRouter()
	r.Use(
		Recovery(slog.Default()),
		RequestID(),
	)

	r.GET("/panic", func(ctx Context) error {
		panic("intentional panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	// RequestID가 Recovery 후에 있어도 패닉 전에 헤더가 설정되었을 수 있다
}

func TestMiddlewareChain_CORSWithRateLimit(t *testing.T) {
	r := NewRouter()
	r.Use(
		CORS(config.CORSConfig{
			Enabled:        true,
			AllowedOrigins: []string{"http://example.com"},
		}),
		RateLimit(config.RateLimitConfig{
			Enabled:           true,
			RequestsPerSecond: 100,
		}),
	)

	r.GET("/test", func(ctx Context) error {
		return ctx.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "http://example.com", rec.Header().Get("Access-Control-Allow-Origin"))
}
