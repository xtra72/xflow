package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/xtra/xflow/internal/api/dto"
)

// HandlerFunc 는 API 핸들러 함수 시그니처이다.
type HandlerFunc func(ctx Context) error

// MiddlewareFunc 는 HandlerFunc를 감싸서 새로운 HandlerFunc를 반환한다.
type MiddlewareFunc func(next HandlerFunc) HandlerFunc

// Context 는 핸들러를 위한 요청 컨텍스트를 추상화한다.
type Context interface {
	// Param 은 경로 파라미터를 반환한다 (예: {id}).
	Param(name string) string
	// Query 는 쿼리 파라미터를 반환한다.
	Query(name string) string
	// QueryDefault 는 기본값을 가진 쿼리 파라미터를 반환한다.
	QueryDefault(name, def string) string
	// QueryValues 는 동일 이름으로 여러 번 주어진 쿼리 파라미터의 모든 값을 반환한다.
	// 값이 없으면 빈 슬라이스를 반환한다.
	// 예: ?tag=a:1&tag=b:2 → QueryValues("tag") = []string{"a:1", "b:2"}
	QueryValues(name string) []string
	// Bind 는 JSON 본문을 구조체에 바인딩한다.
	Bind(v any) error
	// JSON 은 JSON 응답을 작성한다.
	JSON(code int, v any) error
	// NoContent 는 빈 응답을 작성한다.
	NoContent(code int) error
	// RequestID 는 X-Request-ID를 반환한다.
	RequestID() string
	// UserID 는 인증 컨텍스트에서 사용자 ID를 반환한다.
	UserID() string
	// UserRole 은 인증 컨텍스트에서 사용자 역할을 반환한다.
	UserRole() string
	// Context 는 기본 context.Context를 반환한다.
	Context() context.Context
	// SetHeader 는 응답 헤더를 설정한다.
	SetHeader(key, value string)
	// GetHeader 는 요청 헤더를 반환한다.
	GetHeader(key string) string
	// RealIP 는 클라이언트 IP를 반환한다 (X-Forwarded-For 폴백).
	RealIP() string
	// Method 는 HTTP 메서드를 반환한다.
	Method() string
	// Path 는 요청 경로를 반환한다.
	Path() string
}

// RouteRecord 는 등록된 라우트 1건의 메타데이터이다.
//
// @SPEC:SPEC-AUTH-005 (M5)
// MiddlewareFunc 는 익명 클로저라 부착된 미들웨어가 어떤 권한을 요구하는지 사후에
// 들여다볼 수 없다. 권한 부착 누락(= 보안 구멍)을 자동 검증하려면(acceptance.md AC-08)
// 등록 시점에 권한 키를 함께 기록해야 한다.
type RouteRecord struct {
	// Method 는 HTTP 메서드이다.
	Method string
	// Pattern 은 그룹 prefix 를 포함한 전체 경로이다 (예: /api/v1/agents/{id}).
	Pattern string
	// Permission 은 라우트가 요구하는 권한 키이다. 빈 문자열이면 미부착이다.
	Permission string
}

// Router 는 라우트 등록과 요청 디스패칭을 관리한다.
type Router struct {
	mux         *http.ServeMux
	middlewares []MiddlewareFunc
	prefix      string
	routeCount  int
	routes      []RouteRecord
}

// NewRouter 는 새 Router를 생성한다.
func NewRouter() *Router {
	return &Router{
		mux: http.NewServeMux(),
	}
}

// Use 는 라우터에 글로벌 미들웨어를 추가한다.
func (r *Router) Use(mw ...MiddlewareFunc) {
	r.middlewares = append(r.middlewares, mw...)
}

// Group 은 주어진 접두사와 선택적 미들웨어를 가진 하위 그룹을 생성한다.
func (r *Router) Group(prefix string, mw ...MiddlewareFunc) *RouteGroup {
	return &RouteGroup{
		router:      r,
		prefix:      r.prefix + prefix,
		middlewares: mw,
	}
}

// GET 는 GET 라우트를 등록한다.
func (r *Router) GET(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	r.addRoute("GET", path, "", handler, mw...)
}

// POST 는 POST 라우트를 등록한다.
func (r *Router) POST(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	r.addRoute("POST", path, "", handler, mw...)
}

// PUT 는 PUT 라우트를 등록한다.
func (r *Router) PUT(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	r.addRoute("PUT", path, "", handler, mw...)
}

// PATCH 는 PATCH 라우트를 등록한다(부분 갱신 — @SPEC:SPEC-REMOTE-001 M7 원격 자원 수정).
func (r *Router) PATCH(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	r.addRoute("PATCH", path, "", handler, mw...)
}

// DELETE 는 DELETE 라우트를 등록한다.
func (r *Router) DELETE(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	r.addRoute("DELETE", path, "", handler, mw...)
}

// HandleFunc 는 raw http.HandlerFunc를 직접 등록한다.
// Context 래퍼를 바이패스하므로 WebSocket 업그레이드 등
// raw HTTP 접근이 필요한 경우에 사용한다.
func (r *Router) HandleFunc(pattern string, handler http.HandlerFunc) {
	r.mux.HandleFunc(pattern, handler)
	r.routeCount++
}

// Handler 는 http.Handler (기본 ServeMux)를 반환한다.
func (r *Router) Handler() http.Handler {
	return r.mux
}

// RouteCount 는 등록된 라우트 수를 반환한다.
func (r *Router) RouteCount() int {
	return r.routeCount
}

// Routes 는 등록된 라우트 메타데이터 목록의 복사본을 반환한다.
//
// @SPEC:SPEC-AUTH-005 (M5, acceptance.md AC-08)
// 권한 부착 커버리지 검사가 본 목록을 전수 대조한다. RegisterRawHandler /
// Router.HandleFunc 로 등록되는 raw 핸들러는 Context 래퍼와 미들웨어 체인을
// 통째로 우회하므로(자체 JWT 검증) 본 목록에 포함되지 않는다.
func (r *Router) Routes() []RouteRecord {
	return append([]RouteRecord(nil), r.routes...)
}

// addRoute 는 메서드, 경로, 핸들러를 ServeMux에 등록한다.
// permission 이 비어있지 않으면 RouteRecord 에 함께 기록된다 (권한 미들웨어 부착은
// 호출자 책임 — RouteGroup 의 *Perm 계열 메서드가 수행한다).
func (r *Router) addRoute(method, path, permission string, handler HandlerFunc, mw ...MiddlewareFunc) {
	// 미들웨어 체인 구성: 글로벌 -> 라우트별
	chain := make([]MiddlewareFunc, 0, len(r.middlewares)+len(mw))
	chain = append(chain, r.middlewares...)
	chain = append(chain, mw...)

	wrapped := applyMiddleware(handler, chain)
	fullPath := r.prefix + path
	pattern := method + " " + fullPath

	r.mux.HandleFunc(pattern, func(w http.ResponseWriter, req *http.Request) {
		ctx := newHTTPContext(w, req)
		if err := wrapped(ctx); err != nil {
			handleError(ctx, err)
		}
	})
	r.routeCount++
	r.routes = append(r.routes, RouteRecord{Method: method, Pattern: fullPath, Permission: permission})
}

// RouteGroup 은 공유 접두사와 미들웨어를 가진 라우트 그룹이다.
type RouteGroup struct {
	router      *Router
	prefix      string
	middlewares []MiddlewareFunc
}

// Use 는 그룹에 미들웨어를 추가한다.
func (g *RouteGroup) Use(mw ...MiddlewareFunc) {
	g.middlewares = append(g.middlewares, mw...)
}

// Group 은 하위 그룹을 생성한다.
func (g *RouteGroup) Group(prefix string, mw ...MiddlewareFunc) *RouteGroup {
	return &RouteGroup{
		router:      g.router,
		prefix:      g.prefix + prefix,
		middlewares: append(g.middlewares, mw...),
	}
}

// GET 는 GET 라우트를 그룹에 등록한다 (권한 미부착 — 공개/인증 전용 라우트).
func (g *RouteGroup) GET(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	g.addRoute("GET", path, "", handler, mw...)
}

// POST 는 POST 라우트를 그룹에 등록한다 (권한 미부착).
func (g *RouteGroup) POST(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	g.addRoute("POST", path, "", handler, mw...)
}

// PUT 는 PUT 라우트를 그룹에 등록한다 (권한 미부착).
func (g *RouteGroup) PUT(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	g.addRoute("PUT", path, "", handler, mw...)
}

// PATCH 는 PATCH 라우트를 그룹에 등록한다(부분 갱신 — @SPEC:SPEC-REMOTE-001 M7).
func (g *RouteGroup) PATCH(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	g.addRoute("PATCH", path, "", handler, mw...)
}

// DELETE 는 DELETE 라우트를 그룹에 등록한다 (권한 미부착).
func (g *RouteGroup) DELETE(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	g.addRoute("DELETE", path, "", handler, mw...)
}

// @SPEC:SPEC-AUTH-005 (M5) — 권한 인지 라우트 등록.
//
// *Perm 계열은 두 가지를 한 번에 수행한다.
//  1. RequirePermission(perm) 을 라우트별 미들웨어로 부착한다 (실제 인가 강제).
//  2. 권한 키를 RouteRecord 에 기록한다 (커버리지 검사 대상, AC-08).
//
// 두 동작을 분리하면 한쪽만 갱신되는 drift 가 생기므로 단일 호출로 묶는다.
// 권한이 필요 없는 라우트는 기존 GET/POST/... 를 그대로 사용하고, 커버리지 검사의
// 명시적 allowlist 에 사유와 함께 등재한다.

// GETPerm 은 권한을 요구하는 GET 라우트를 등록한다.
func (g *RouteGroup) GETPerm(path, permission string, handler HandlerFunc, mw ...MiddlewareFunc) {
	g.addPermRoute("GET", path, permission, handler, mw...)
}

// POSTPerm 은 권한을 요구하는 POST 라우트를 등록한다.
func (g *RouteGroup) POSTPerm(path, permission string, handler HandlerFunc, mw ...MiddlewareFunc) {
	g.addPermRoute("POST", path, permission, handler, mw...)
}

// PUTPerm 은 권한을 요구하는 PUT 라우트를 등록한다.
func (g *RouteGroup) PUTPerm(path, permission string, handler HandlerFunc, mw ...MiddlewareFunc) {
	g.addPermRoute("PUT", path, permission, handler, mw...)
}

// PATCHPerm 은 권한을 요구하는 PATCH 라우트를 등록한다.
func (g *RouteGroup) PATCHPerm(path, permission string, handler HandlerFunc, mw ...MiddlewareFunc) {
	g.addPermRoute("PATCH", path, permission, handler, mw...)
}

// DELETEPerm 은 권한을 요구하는 DELETE 라우트를 등록한다.
func (g *RouteGroup) DELETEPerm(path, permission string, handler HandlerFunc, mw ...MiddlewareFunc) {
	g.addPermRoute("DELETE", path, permission, handler, mw...)
}

// addPermRoute 는 권한 미들웨어를 부착하고 권한 키를 기록한다.
// 권한 미들웨어는 라우트별 미들웨어의 가장 앞에 두어 핸들러 진입 전에 검사되게 한다.
func (g *RouteGroup) addPermRoute(method, path, permission string, handler HandlerFunc, mw ...MiddlewareFunc) {
	withPerm := make([]MiddlewareFunc, 0, len(mw)+1)
	withPerm = append(withPerm, RequirePermission(permission))
	withPerm = append(withPerm, mw...)
	g.addRoute(method, path, permission, handler, withPerm...)
}

// addRoute 는 그룹의 라우트를 등록한다.
func (g *RouteGroup) addRoute(method, path, permission string, handler HandlerFunc, mw ...MiddlewareFunc) {
	// 미들웨어 체인: 글로벌 -> 그룹 -> 라우트별
	chain := make([]MiddlewareFunc, 0, len(g.router.middlewares)+len(g.middlewares)+len(mw))
	chain = append(chain, g.router.middlewares...)
	chain = append(chain, g.middlewares...)
	chain = append(chain, mw...)

	wrapped := applyMiddleware(handler, chain)
	fullPath := g.prefix + path
	pattern := method + " " + fullPath

	g.router.mux.HandleFunc(pattern, func(w http.ResponseWriter, req *http.Request) {
		ctx := newHTTPContext(w, req)
		if err := wrapped(ctx); err != nil {
			handleError(ctx, err)
		}
	})
	g.router.routeCount++
	g.router.routes = append(g.router.routes, RouteRecord{
		Method:     method,
		Pattern:    fullPath,
		Permission: permission,
	})
}

// applyMiddleware 는 핸들러에 미들웨어 체인을 적용한다.
// 미들웨어는 순서대로 적용되어, 첫 번째 미들웨어가 가장 바깥쪽에 위치한다.
func applyMiddleware(handler HandlerFunc, middlewares []MiddlewareFunc) HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// handleError 는 핸들러 에러를 적절한 JSON 응답으로 변환한다.
func handleError(ctx *httpContext, err error) {
	if ctx.isWritten() {
		return
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		apiErr = ErrInternalServer.WithMessage(err.Error())
	}

	// 500 에러는 서버 로그에 원본 에러를 기록한다
	if apiErr.HTTPCode >= 500 {
		slog.Error("API error",
			"code", apiErr.Code,
			"message", apiErr.Message,
			"method", ctx.r.Method,
			"path", ctx.r.URL.Path,
			"error", err.Error(),
		)
	}

	resp := dto.NewErrorResponse(apiErr.Code, apiErr.Message, apiErr.Details)

	// 에러 응답 작성 시도 (이미 written이면 무시)
	_ = ctx.JSON(apiErr.HTTPCode, resp)
}

// httpContext 는 Context의 net/http 구현이다.
//
// 동시성 주의: Timeout 미들웨어는 핸들러를 별도 goroutine 에서 실행한다.
// 타임아웃/취소 시 메인 경로가 먼저 반환되어 net/http 가 핸들러를 완료 처리하는데,
// 이때 백그라운드 핸들러 goroutine 이 뒤늦게 ResponseWriter 에 접근하면
// "Header called after Handler finished" 패닉이 발생한다.
// 따라서 written/w/finished 접근을 mu 로 동기화하고, finished 이후의 응답 write 를
// no-op 으로 차단한다.
type httpContext struct {
	mu      sync.Mutex
	w       http.ResponseWriter
	r       *http.Request
	written bool
	// finished 는 메인 핸들러 경로가 종료(타임아웃/취소)되어 net/http 가
	// 핸들러를 완료 처리했음을 의미한다. true 이면 ResponseWriter 접근이 금지된다.
	finished bool
}

// newHTTPContext 는 새 httpContext를 생성한다.
func newHTTPContext(w http.ResponseWriter, r *http.Request) *httpContext {
	return &httpContext{
		w: w,
		r: r,
	}
}

// finish 는 메인 핸들러 경로가 종료되었음을 표시한다.
// 호출 이후 JSON/NoContent/SetHeader 등 ResponseWriter 쓰기 메서드는 no-op 이 된다.
// Timeout 미들웨어가 백그라운드 goroutine 의 뒤늦은 write 를 차단하기 위해 사용한다.
func (c *httpContext) finish() {
	c.mu.Lock()
	c.finished = true
	c.mu.Unlock()
}

// isWritten 은 응답이 이미 작성되었는지 동기화하여 반환한다.
func (c *httpContext) isWritten() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.written
}

// setWriter 는 기본 ResponseWriter 를 교체한다 (Logger/Compress 미들웨어용).
// 백그라운드 goroutine 의 w 읽기와의 data race 를 막기 위해 mu 로 보호한다.
func (c *httpContext) setWriter(w http.ResponseWriter) {
	c.mu.Lock()
	c.w = w
	c.mu.Unlock()
}

// getWriter 는 기본 ResponseWriter 를 동기화하여 반환한다 (미들웨어용).
func (c *httpContext) getWriter() http.ResponseWriter {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.w
}

// Param 은 경로 파라미터를 반환한다 (Go 1.22+ PathValue 사용).
func (c *httpContext) Param(name string) string {
	return c.r.PathValue(name)
}

// Query 는 쿼리 파라미터를 반환한다.
func (c *httpContext) Query(name string) string {
	return c.r.URL.Query().Get(name)
}

// QueryDefault 는 기본값을 가진 쿼리 파라미터를 반환한다.
func (c *httpContext) QueryDefault(name, def string) string {
	v := c.r.URL.Query().Get(name)
	if v == "" {
		return def
	}
	return v
}

// QueryValues 는 동일 이름 쿼리 파라미터의 모든 값을 반환한다.
// 값이 없으면 빈 슬라이스를 반환한다.
func (c *httpContext) QueryValues(name string) []string {
	vs := c.r.URL.Query()[name]
	if vs == nil {
		return []string{}
	}
	return vs
}

// Bind 는 JSON 요청 본문을 구조체에 디코딩한다.
func (c *httpContext) Bind(v any) error {
	if c.r.Body == nil {
		return ErrBadRequest.WithMessage("request body is empty")
	}
	decoder := json.NewDecoder(c.r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		return ErrBadRequest.WithMessage("invalid request body: " + err.Error())
	}
	return nil
}

// JSON 은 JSON 응답을 작성한다.
//
// 동시성: finished(메인 핸들러 종료) 또는 written(이미 응답함) 이면 no-op 으로
// 안전 반환한다. body 인코딩은 lock 밖에서 수행하여 lock 보유 시간을 최소화하고,
// finished/written 가드 및 헤더/WriteHeader/Write 는 lock 안에서 원자적으로 수행하여
// 타임아웃 경로(finish)와의 경합으로 인한 "Header called after Handler finished"
// 패닉을 방지한다.
func (c *httpContext) JSON(code int, v any) error {
	// body 인코딩은 ResponseWriter 접근과 무관하므로 lock 밖에서 수행한다.
	var buf bytes.Buffer
	if v != nil {
		if err := json.NewEncoder(&buf).Encode(v); err != nil {
			return err
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.finished || c.written {
		return nil
	}
	c.written = true
	c.w.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.w.WriteHeader(code)
	if v == nil {
		return nil
	}
	_, err := c.w.Write(buf.Bytes())
	return err
}

// NoContent 는 빈 응답을 작성한다.
func (c *httpContext) NoContent(code int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.finished || c.written {
		return nil
	}
	c.written = true
	c.w.WriteHeader(code)
	return nil
}

// RequestID 는 컨텍스트에서 요청 ID를 반환한다.
func (c *httpContext) RequestID() string {
	if id, ok := c.r.Context().Value(ctxKeyRequestID).(string); ok {
		return id
	}
	return ""
}

// UserID 는 컨텍스트에서 사용자 ID를 반환한다.
func (c *httpContext) UserID() string {
	if id, ok := c.r.Context().Value(ctxKeyUserID).(string); ok {
		return id
	}
	return ""
}

// UserRole 은 컨텍스트에서 사용자 역할을 반환한다.
func (c *httpContext) UserRole() string {
	if role, ok := c.r.Context().Value(ctxKeyUserRole).(string); ok {
		return role
	}
	return ""
}

// Context 는 기본 context.Context를 반환한다.
func (c *httpContext) Context() context.Context {
	return c.r.Context()
}

// SetHeader 는 응답 헤더를 설정한다.
//
// 동시성: finished(메인 핸들러 종료) 또는 written(헤더가 이미 커밋됨) 이면
// no-op 으로 안전 반환한다. 이미 응답한 뒤의 헤더 변경은 효과가 없고,
// finished 이후의 접근은 패닉을 유발하기 때문이다.
func (c *httpContext) SetHeader(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.finished || c.written {
		return
	}
	c.w.Header().Set(key, value)
}

// GetHeader 는 요청 헤더를 반환한다.
func (c *httpContext) GetHeader(key string) string {
	return c.r.Header.Get(key)
}

// RealIP 는 클라이언트 IP 주소를 반환한다.
// X-Forwarded-For -> X-Real-IP -> RemoteAddr 순서로 확인한다.
func (c *httpContext) RealIP() string {
	if xff := c.r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For 의 첫 번째 IP를 사용한다
		if idx := strings.IndexByte(xff, ','); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	if xri := c.r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// RemoteAddr 에서 호스트 부분만 추출한다
	host, _, err := net.SplitHostPort(c.r.RemoteAddr)
	if err != nil {
		return c.r.RemoteAddr
	}
	return host
}

// Method 는 HTTP 메서드를 반환한다.
func (c *httpContext) Method() string {
	return c.r.Method
}

// Path 는 요청 경로를 반환한다.
func (c *httpContext) Path() string {
	return c.r.URL.Path
}

// setRequest 는 요청을 업데이트한다 (미들웨어에서 컨텍스트 추가 시 사용).
func (c *httpContext) setRequest(r *http.Request) {
	c.r = r
}

// responseWriter 는 기본 http.ResponseWriter를 반환한다 (미들웨어에서 사용).
func (c *httpContext) responseWriter() http.ResponseWriter {
	return c.getWriter()
}

// request 는 기본 *http.Request를 반환한다 (미들웨어에서 사용).
func (c *httpContext) request() *http.Request {
	return c.r
}

// Request 는 기본 *http.Request 를 반환한다 (핸들러에서 raw body 접근 시 사용).
//
// @spec SPEC-DASHBOARD-001 v0.2.0 (M-7) — payload 크기 한도 검증 (256KB) 을 위해
// 핸들러가 io.LimitReader 로 body 를 직접 읽어야 한다.
//
// 본 메서드는 Context 인터페이스에 포함되지 않으므로 핸들러가 interface assertion
// 으로 접근한다 (api.Context 의 호환성 보존).
func (c *httpContext) Request() *http.Request {
	return c.r
}

// generateRequestID 는 UUID v4 요청 ID를 생성한다.
func generateRequestID() string {
	return uuid.New().String()
}
