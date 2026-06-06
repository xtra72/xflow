package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"

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

// Router 는 라우트 등록과 요청 디스패칭을 관리한다.
type Router struct {
	mux         *http.ServeMux
	middlewares []MiddlewareFunc
	prefix      string
	routeCount  int
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
	r.addRoute("GET", path, handler, mw...)
}

// POST 는 POST 라우트를 등록한다.
func (r *Router) POST(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	r.addRoute("POST", path, handler, mw...)
}

// PUT 는 PUT 라우트를 등록한다.
func (r *Router) PUT(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	r.addRoute("PUT", path, handler, mw...)
}

// PATCH 는 PATCH 라우트를 등록한다(부분 갱신 — @SPEC:SPEC-REMOTE-001 M7 원격 자원 수정).
func (r *Router) PATCH(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	r.addRoute("PATCH", path, handler, mw...)
}

// DELETE 는 DELETE 라우트를 등록한다.
func (r *Router) DELETE(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	r.addRoute("DELETE", path, handler, mw...)
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

// addRoute 는 메서드, 경로, 핸들러를 ServeMux에 등록한다.
func (r *Router) addRoute(method, path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	// 미들웨어 체인 구성: 글로벌 -> 라우트별
	chain := make([]MiddlewareFunc, 0, len(r.middlewares)+len(mw))
	chain = append(chain, r.middlewares...)
	chain = append(chain, mw...)

	wrapped := applyMiddleware(handler, chain)
	pattern := method + " " + r.prefix + path

	r.mux.HandleFunc(pattern, func(w http.ResponseWriter, req *http.Request) {
		ctx := newHTTPContext(w, req)
		if err := wrapped(ctx); err != nil {
			handleError(ctx, err)
		}
	})
	r.routeCount++
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

// GET 는 GET 라우트를 그룹에 등록한다.
func (g *RouteGroup) GET(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	g.addRoute("GET", path, handler, mw...)
}

// POST 는 POST 라우트를 그룹에 등록한다.
func (g *RouteGroup) POST(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	g.addRoute("POST", path, handler, mw...)
}

// PUT 는 PUT 라우트를 그룹에 등록한다.
func (g *RouteGroup) PUT(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	g.addRoute("PUT", path, handler, mw...)
}

// PATCH 는 PATCH 라우트를 그룹에 등록한다(부분 갱신 — @SPEC:SPEC-REMOTE-001 M7).
func (g *RouteGroup) PATCH(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	g.addRoute("PATCH", path, handler, mw...)
}

// DELETE 는 DELETE 라우트를 그룹에 등록한다.
func (g *RouteGroup) DELETE(path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	g.addRoute("DELETE", path, handler, mw...)
}

// addRoute 는 그룹의 라우트를 등록한다.
func (g *RouteGroup) addRoute(method, path string, handler HandlerFunc, mw ...MiddlewareFunc) {
	// 미들웨어 체인: 글로벌 -> 그룹 -> 라우트별
	chain := make([]MiddlewareFunc, 0, len(g.router.middlewares)+len(g.middlewares)+len(mw))
	chain = append(chain, g.router.middlewares...)
	chain = append(chain, g.middlewares...)
	chain = append(chain, mw...)

	wrapped := applyMiddleware(handler, chain)
	pattern := method + " " + g.prefix + path

	g.router.mux.HandleFunc(pattern, func(w http.ResponseWriter, req *http.Request) {
		ctx := newHTTPContext(w, req)
		if err := wrapped(ctx); err != nil {
			handleError(ctx, err)
		}
	})
	g.router.routeCount++
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
	if ctx.written {
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
type httpContext struct {
	w       http.ResponseWriter
	r       *http.Request
	written bool
}

// newHTTPContext 는 새 httpContext를 생성한다.
func newHTTPContext(w http.ResponseWriter, r *http.Request) *httpContext {
	return &httpContext{
		w: w,
		r: r,
	}
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
func (c *httpContext) JSON(code int, v any) error {
	if c.written {
		return nil
	}
	c.written = true
	c.w.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.w.WriteHeader(code)
	if v == nil {
		return nil
	}
	return json.NewEncoder(c.w).Encode(v)
}

// NoContent 는 빈 응답을 작성한다.
func (c *httpContext) NoContent(code int) error {
	if c.written {
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
func (c *httpContext) SetHeader(key, value string) {
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
	return c.w
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
