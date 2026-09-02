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

	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/auth"
	"github.com/xtra/xflow/internal/config"
)

// 미들웨어를 위한 컨텍스트 키
type contextKey string

const (
	ctxKeyRequestID contextKey = "request_id"
	ctxKeyUserID    contextKey = "user_id"
	ctxKeyUserRole  contextKey = "user_role"
)

// ContextKeyUserRole 은 user_role 컨텍스트 키를 반환한다 (테스트용).
//
// 운영 코드는 ctx.UserRole() 을 사용해야 하며, 본 함수는 테스트에서
// http.Request.Context() 에 role 을 직접 주입할 때만 사용한다.
//
// @spec SPEC-UPDATE-002 v0.1.0 (M7) — admin 권한 필수 핸들러 테스트 지원.
func ContextKeyUserRole() any {
	return ctxKeyUserRole
}

// ContextKeyUserID 는 user_id 컨텍스트 키를 반환한다 (테스트용).
//
// 운영 코드는 ctx.UserID() 를 사용해야 하며, 본 함수는 테스트에서 JWT 미들웨어를
// 우회하여 username 을 컨텍스트에 직접 주입할 때만 사용한다.
//
// @spec SPEC-DASHBOARD-001 v0.2.0 (M-7) — /api/dashboards/mine 핸들러 테스트 지원.
func ContextKeyUserID() any {
	return ctxKeyUserID
}

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
			originalWriter := hctx.getWriter()
			srw := &statusRecorderWriter{ResponseWriter: originalWriter, statusCode: http.StatusOK}
			hctx.setWriter(srw)

			err := next(hctx)

			duration := time.Since(start)
			hctx.setWriter(originalWriter)

			logger.Debug("HTTP 요청",
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
				// 핸들러가 정상 완료(또는 자체 에러 반환)된 경우.
				return err
			case <-timeoutCtx.Done():
				// 타임아웃 또는 취소(클라이언트 끊김)로 메인 경로가 먼저 종료된다.
				// 백그라운드 핸들러 goroutine 은 여전히 실행 중일 수 있으므로,
				// finished 로 마킹하여 이후의 hctx.JSON()/Write() 를 no-op 으로 차단한다.
				// 이로써 "Header called after Handler finished" 패닉을 방지한다.
				//
				// deadline 초과 시: 백그라운드가 아직 응답하지 않았다면 408 을 직접 쓴다
				// (written 가드로 중복 write 차단). finished 마킹 전에 써야 하므로
				// JSON() 을 먼저 호출한 뒤 finish() 한다.
				// 취소(클라이언트 끊김) 시: 쓸 대상이 없으므로 응답하지 않는다.
				if timeoutCtx.Err() == context.DeadlineExceeded {
					_ = hctx.JSON(ErrRequestTimeout.HTTPCode, dto.NewErrorResponse(
						ErrRequestTimeout.Code, ErrRequestTimeout.Message, nil))
				}
				hctx.finish()
				// 응답을 직접 처리했으므로 nil 을 반환하여 handleError 의 중복 렌더를 막는다.
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

			preGzipWriter := hctx.getWriter()
			gw, err := gzip.NewWriterLevel(preGzipWriter, gzip.DefaultCompression)
			if err != nil {
				return next(hctx)
			}

			grw := &gzipResponseWriter{
				ResponseWriter: preGzipWriter,
				writer:         gw,
			}
			hctx.setWriter(grw)
			hctx.SetHeader("Content-Encoding", "gzip")
			// Content-Length는 압축 후 달라지므로 삭제
			grw.Header().Del("Content-Length")

			handlerErr := next(hctx)

			if handlerErr != nil {
				// 에러 경로: gzip 래핑을 되돌려서 handleError 가 비압축 응답을 쓸 수 있게 한다.
				// gw.Close() 를 호출하지 않아 gzip 바이트가 기록되지 않는다.
				hctx.setWriter(preGzipWriter)
				preGzipWriter.Header().Del("Content-Encoding")
				return handlerErr
			}

			// 성공 경로: gzip 데이터를 플러시한다
			_ = gw.Close()
			return nil
		}
	}
}

// Auth 는 JWT 인증 미들웨어이다.
// basic_auth.enabled=true 이면 JWT 토큰을 검증하고, UserID와 UserRole을 컨텍스트에 설정한다.
// basic_auth.enabled=false 이면 패스스루한다.
// /health, /ready, /api/v1/auth/login, /api/v1/auth/refresh 는 인증을 건너뛴다.
func Auth(enabled bool, jwtSvc *auth.JWTService) MiddlewareFunc {
	// 인증이 비활성화이면 패스스루
	if !enabled || jwtSvc == nil {
		return func(next HandlerFunc) HandlerFunc {
			return func(ctx Context) error {
				return next(ctx)
			}
		}
	}

	// 인증 면제 경로
	exemptPaths := map[string]bool{
		"/health":              true,
		"/ready":               true,
		"/api/v1/auth/login":   true,
		"/api/v1/auth/refresh": true,
		"/api/v1/auth/status":  true,
	}

	return func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			hctx, ok := ctx.(*httpContext)
			if !ok {
				return next(ctx)
			}

			path := hctx.Path()

			// 면제 경로 확인
			if exemptPaths[path] {
				return next(hctx)
			}

			// Authorization 헤더에서 Bearer 토큰 추출
			authHeader := hctx.GetHeader("Authorization")
			if authHeader == "" {
				return ErrUnauthorized.WithMessage("Authorization 헤더가 필요합니다")
			}

			if !strings.HasPrefix(authHeader, "Bearer ") {
				return ErrUnauthorized.WithMessage("Bearer 토큰 형식이 필요합니다")
			}

			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			if tokenString == "" {
				return ErrUnauthorized.WithMessage("토큰이 비어있습니다")
			}

			// 블랙리스트 확인
			if jwtSvc.IsBlacklisted(tokenString) {
				return ErrUnauthorized.WithMessage("만료된 토큰입니다")
			}

			// JWT 토큰 검증
			claims, err := jwtSvc.ValidateToken(tokenString)
			if err != nil {
				return ErrUnauthorized.WithMessage("유효하지 않은 토큰입니다")
			}

			// 컨텍스트에 사용자 정보 저장
			newCtx := context.WithValue(hctx.r.Context(), ctxKeyUserID, claims.Username)
			newCtx = context.WithValue(newCtx, ctxKeyUserRole, claims.Role)
			hctx.setRequest(hctx.r.WithContext(newCtx))

			return next(hctx)
		}
	}
}

// @SPEC:SPEC-AUTH-005 (M4) — 인가 강제.
//
// 기존 RequireRole 은 인자를 버리는 패스스루라 인가를 전혀 강제하지 못했다
// (spec.md §1.2.1). 호출자가 없었으므로 실제 검사로 대체하는 대신 제거하고,
// 라우트별 권한 키를 요구하는 RequirePermission 으로 일원화한다.

// Authorizer 는 요청 주체가 특정 권한 키를 보유했는지 판정한다.
//
// username 은 JWT 의 주체이고 role 은 토큰 클레임의 역할이다. 구현은 username 으로
// 현재 역할을 다시 조회하여 강등이 기존 토큰에도 즉시 반영되게 할 수 있다
// (acceptance.md AC-05). 조회 대상이 없으면 role 을 그대로 사용한다.
//
// internal/auth.PermissionCache 가 본 인터페이스를 만족한다. api 패키지가 인가 구현을
// 직접 알지 않도록 인터페이스로 주입받는다.
type Authorizer interface {
	HasPermission(ctx context.Context, username, role, permission string) (bool, error)
}

// authzConfig 는 요청 컨텍스트로 전달되는 인가 설정이다.
//
// 핸들러의 RegisterRoutes 는 Server 참조가 없으므로 RequirePermission 이 생성 시점에
// 활성화 플래그를 알 수 없다. 서버 조립부가 글로벌 미들웨어(Authorization)로 본 설정을
// 컨텍스트에 주입하고, RequirePermission 은 요청 시점에 이를 읽는다.
type authzConfig struct {
	enabled    bool
	authorizer Authorizer
	logger     *slog.Logger
}

// ctxKeyAuthz 는 authzConfig 컨텍스트 키이다.
const ctxKeyAuthz contextKey = "authz"

// Authorization 은 인가 설정을 요청 컨텍스트에 주입하는 글로벌 미들웨어이다.
//
// Auth 와 동일한 enabled 플래그를 공유한다 (spec.md §2.5 S1). Auth 직후에 등록하여
// 컨텍스트의 역할 정보가 이미 채워진 상태에서 RequirePermission 이 동작하게 한다.
func Authorization(enabled bool, authorizer Authorizer, logger *slog.Logger) MiddlewareFunc {
	if logger == nil {
		logger = slog.Default()
	}
	cfg := &authzConfig{enabled: enabled, authorizer: authorizer, logger: logger}

	return func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			hctx, ok := ctx.(*httpContext)
			if !ok {
				return next(ctx)
			}
			newCtx := context.WithValue(hctx.r.Context(), ctxKeyAuthz, cfg)
			hctx.setRequest(hctx.r.WithContext(newCtx))
			return next(hctx)
		}
	}
}

// RequirePermission 은 라우트가 요구하는 권한 키를 요청자의 역할이 보유했는지 검사한다.
//
// 동작 (spec.md §2.3 E1, §2.5 S1):
//   - 인가 설정이 컨텍스트에 없거나 비활성(basic_auth.enabled=false)이면 패스스루한다.
//     인증 없이 쓰던 기존 배포가 전부 403 이 되는 회귀를 막기 위함이다.
//   - 권한이 없으면 403 으로 거부하고, 구조화 로그에 username/permission/method/path 를
//     남긴다. 응답 본문에는 어떤 권한이 부족한지 노출하지 않는다.
//   - 토큰의 역할이 삭제된 역할을 가리키면 403 이다 (500 아님).
func RequirePermission(permission string) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			hctx, ok := ctx.(*httpContext)
			if !ok {
				return next(ctx)
			}

			cfg, _ := hctx.r.Context().Value(ctxKeyAuthz).(*authzConfig)
			if cfg == nil || !cfg.enabled || cfg.authorizer == nil {
				// S1: 인증 비활성 상태에서는 권한 검사를 수행하지 않는다.
				return next(hctx)
			}

			allowed, err := cfg.authorizer.HasPermission(
				hctx.Context(), hctx.UserID(), hctx.UserRole(), permission)
			if err != nil {
				cfg.logger.Error("인가 검사 실패",
					slog.String("username", hctx.UserID()),
					slog.String("permission", permission),
					slog.String("method", hctx.Method()),
					slog.String("path", hctx.Path()),
					slog.String("error", err.Error()),
				)
				return ErrInternalServer
			}
			if !allowed {
				cfg.logger.Warn("권한 거부",
					slog.String("username", hctx.UserID()),
					slog.String("permission", permission),
					slog.String("method", hctx.Method()),
					slog.String("path", hctx.Path()),
				)
				// 부족한 권한 키를 응답에 노출하지 않는다 (spec.md §5).
				return ErrForbidden
			}

			return next(hctx)
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
