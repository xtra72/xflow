package api

import (
	"compress/gzip"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/config"
)

// 미들웨어를 위한 컨텍스트 키
type contextKey string

const (
	ctxKeyRequestID contextKey = "request_id"
	ctxKeyUserID    contextKey = "user_id"
	ctxKeyUserRole  contextKey = "user_role"
)

// RequestID 는 UUID v4 요청 ID를 생성하고 X-Request-ID 헤더를 설정한다.
func RequestID() MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			hctx, ok := ctx.(*httpContext)
			if !ok {
				return next(ctx)
			}

			// 기존 요청 ID가 있으면 사용, 없으면 생성
			requestID := hctx.GetHeader("X-Request-ID")
			if requestID == "" {
				requestID = generateRequestID()
			}

			// 응답 헤더에 설정
			hctx.SetHeader("X-Request-ID", requestID)

			// 컨텍스트에 요청 ID 저장
			newCtx := context.WithValue(hctx.r.Context(), ctxKeyRequestID, requestID)
			hctx.setRequest(hctx.r.WithContext(newCtx))

			return next(hctx)
		}
	}
}

// Logger 는 slog를 사용하여 요청/응답을 로깅한다.
// method, path, status code, duration, request_id를 로깅한다.
func Logger(logger *slog.Logger) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			hctx, ok := ctx.(*httpContext)
			if !ok {
				return next(ctx)
			}

			start := time.Now()
			srw := &statusRecorderWriter{ResponseWriter: hctx.w, statusCode: http.StatusOK}
			originalWriter := hctx.w
			hctx.w = srw

			err := next(hctx)

			duration := time.Since(start)
			hctx.w = originalWriter

			logger.Info("HTTP 요청",
				slog.String("method", hctx.Method()),
				slog.String("path", hctx.Path()),
				slog.Int("status", srw.statusCode),
				slog.String("duration", duration.String()),
				slog.String("request_id", hctx.RequestID()),
				slog.String("client_ip", hctx.RealIP()),
			)

			return err
		}
	}
}

// Recovery 는 패닉으로부터 복구하고 500 에러를 표준 형식으로 반환한다.
func Recovery(logger *slog.Logger) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) (retErr error) {
			hctx, ok := ctx.(*httpContext)
			if !ok {
				return next(ctx)
			}

			defer func() {
				if r := recover(); r != nil {
					logger.Error("패닉 복구",
						slog.Any("panic", r),
						slog.String("request_id", hctx.RequestID()),
						slog.String("method", hctx.Method()),
						slog.String("path", hctx.Path()),
					)
					retErr = ErrInternalServer
				}
			}()

			return next(hctx)
		}
	}
}

// CORS 는 설정에 따라 CORS 헤더를 적용한다.
func CORS(cfg config.CORSConfig) MiddlewareFunc {
	allowedOriginsSet := make(map[string]bool, len(cfg.AllowedOrigins))
	allowAll := false
	for _, origin := range cfg.AllowedOrigins {
		if origin == "*" {
			allowAll = true
		}
		allowedOriginsSet[origin] = true
	}

	return func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			hctx, ok := ctx.(*httpContext)
			if !ok {
				return next(ctx)
			}

			origin := hctx.GetHeader("Origin")
			if origin == "" {
				return next(hctx)
			}

			// 오리진 확인
			if allowAll || allowedOriginsSet[origin] {
				hctx.SetHeader("Access-Control-Allow-Origin", origin)
				hctx.SetHeader("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				hctx.SetHeader("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
				hctx.SetHeader("Access-Control-Max-Age", "86400")
				hctx.SetHeader("Vary", "Origin")
			}

			// OPTIONS 프리플라이트 요청 처리
			if hctx.Method() == http.MethodOptions {
				return hctx.NoContent(http.StatusNoContent)
			}

			return next(hctx)
		}
	}
}

// RateLimit 는 토큰 버킷을 사용하여 클라이언트별 속도 제한을 구현한다.
// 초과 시 Retry-After 헤더와 함께 429를 반환한다.
func RateLimit(cfg config.RateLimitConfig) MiddlewareFunc {
	limiter := newRateLimiter(cfg.RequestsPerSecond)

	return func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			hctx, ok := ctx.(*httpContext)
			if !ok {
				return next(ctx)
			}

			clientIP := hctx.RealIP()
			if !limiter.allow(clientIP) {
				hctx.SetHeader("Retry-After", "1")
				return ErrRateLimitExceeded
			}

			return next(hctx)
		}
	}
}

// Timeout 은 요청 처리 타임아웃을 설정한다.
// 초과 시 408을 반환한다.
func Timeout(d time.Duration) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			hctx, ok := ctx.(*httpContext)
			if !ok {
				return next(ctx)
			}

			timeoutCtx, cancel := context.WithTimeout(hctx.r.Context(), d)
			defer cancel()

			hctx.setRequest(hctx.r.WithContext(timeoutCtx))

			// 핸들러를 별도 고루틴에서 실행
			done := make(chan error, 1)
			go func() {
				done <- next(hctx)
			}()

			select {
			case err := <-done:
				return err
			case <-timeoutCtx.Done():
				if timeoutCtx.Err() == context.DeadlineExceeded {
					return ErrRequestTimeout
				}
				return nil
			}
		}
	}
}

// Compress 는 클라이언트가 Accept-Encoding: gzip을 보낼 때 gzip 압축을 적용한다.
func Compress() MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			hctx, ok := ctx.(*httpContext)
			if !ok {
				return next(ctx)
			}

			if !strings.Contains(hctx.GetHeader("Accept-Encoding"), "gzip") {
				return next(hctx)
			}

			gw, err := gzip.NewWriterLevel(hctx.w, gzip.DefaultCompression)
			if err != nil {
				return next(hctx)
			}

			grw := &gzipResponseWriter{
				ResponseWriter: hctx.w,
				writer:         gw,
			}
			hctx.w = grw
			hctx.SetHeader("Content-Encoding", "gzip")
			// Content-Length는 압축 후 달라지므로 삭제
			hctx.w.Header().Del("Content-Length")

			handlerErr := next(hctx)
			// gzip writer를 닫아야 Flush가 됨
			_ = gw.Close()

			return handlerErr
		}
	}
}

// Auth 는 JWT/API 키 유효성 검사를 위한 플레이스홀더 미들웨어이다.
// P1에서는 단순 패스스루이다. 전체 구현은 SPEC-AUTH-001에서 한다.
// 미래: 토큰을 유효성 검사하고, UserID와 UserRole을 컨텍스트에 설정한다.
func Auth() MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			return next(ctx)
		}
	}
}

// RequireRole 은 사용자가 필요한 역할을 가지고 있는지 확인한다.
// P1에서는 단순 패스스루이다.
func RequireRole(_ ...string) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			return next(ctx)
		}
	}
}

// rateLimiter 는 클라이언트별 토큰 버킷 속도 제한기이다.
type rateLimiter struct {
	clients sync.Map // map[string]*clientBucket
	rate    int      // 초당 요청 수
}

// clientBucket 은 클라이언트별 토큰 버킷이다.
type clientBucket struct {
	tokens    float64
	lastCheck time.Time
	mu        sync.Mutex
}

// newRateLimiter 는 새 rateLimiter를 생성한다.
func newRateLimiter(requestsPerSecond int) *rateLimiter {
	if requestsPerSecond <= 0 {
		requestsPerSecond = 10 // 기본값
	}
	return &rateLimiter{
		rate: requestsPerSecond,
	}
}

// allow 는 주어진 클라이언트 IP에 대해 요청을 허용할지 결정한다.
func (rl *rateLimiter) allow(clientIP string) bool {
	val, _ := rl.clients.LoadOrStore(clientIP, &clientBucket{
		tokens:    float64(rl.rate),
		lastCheck: time.Now(),
	})
	bucket := val.(*clientBucket)

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(bucket.lastCheck).Seconds()
	bucket.lastCheck = now

	// 토큰 리필
	bucket.tokens += elapsed * float64(rl.rate)
	if bucket.tokens > float64(rl.rate) {
		bucket.tokens = float64(rl.rate)
	}

	if bucket.tokens < 1 {
		return false
	}

	bucket.tokens--
	return true
}

// statusRecorderWriter 는 응답 상태 코드를 기록하는 ResponseWriter 래퍼이다.
type statusRecorderWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

// WriteHeader 는 상태 코드를 기록하고 기본 ResponseWriter에 전달한다.
func (w *statusRecorderWriter) WriteHeader(code int) {
	if !w.written {
		w.statusCode = code
		w.written = true
	}
	w.ResponseWriter.WriteHeader(code)
}

// Write 는 기본 ResponseWriter에 데이터를 쓴다.
func (w *statusRecorderWriter) Write(b []byte) (int, error) {
	if !w.written {
		w.statusCode = http.StatusOK
		w.written = true
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap 은 기본 ResponseWriter를 반환한다 (http.ResponseController 호환).
func (w *statusRecorderWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// gzipResponseWriter 는 gzip 압축을 적용하는 ResponseWriter 래퍼이다.
type gzipResponseWriter struct {
	http.ResponseWriter
	writer io.Writer
}

// Write 는 gzip writer에 데이터를 쓴다.
func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.writer.Write(b)
}

// WriteHeader 는 기본 ResponseWriter에 상태 코드를 쓴다.
func (w *gzipResponseWriter) WriteHeader(code int) {
	w.ResponseWriter.WriteHeader(code)
}

// Unwrap 은 기본 ResponseWriter를 반환한다 (http.ResponseController 호환).
func (w *gzipResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

