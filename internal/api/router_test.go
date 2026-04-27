package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRouter(t *testing.T) {
	r := NewRouter()
	require.NotNil(t, r)
	assert.NotNil(t, r.mux)
	assert.Empty(t, r.middlewares)
	assert.Equal(t, 0, r.RouteCount())
}

func TestRouter_Use(t *testing.T) {
	r := NewRouter()
	assert.Empty(t, r.middlewares)

	mw := func(next HandlerFunc) HandlerFunc { return next }
	r.Use(mw)
	assert.Len(t, r.middlewares, 1)

	r.Use(mw, mw)
	assert.Len(t, r.middlewares, 3)
}

func TestRouter_GET(t *testing.T) {
	r := NewRouter()
	handler := func(ctx Context) error {
		return ctx.JSON(http.StatusOK, map[string]string{"method": "GET"})
	}

	r.GET("/test", handler)
	assert.Equal(t, 1, r.RouteCount())

	// HTTP 요청 테스트
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

	var body map[string]string
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "GET", body["method"])
}

func TestRouter_POST(t *testing.T) {
	r := NewRouter()
	handler := func(ctx Context) error {
		return ctx.JSON(http.StatusCreated, map[string]string{"method": "POST"})
	}

	r.POST("/test", handler)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestRouter_PUT(t *testing.T) {
	r := NewRouter()
	handler := func(ctx Context) error {
		return ctx.JSON(http.StatusOK, map[string]string{"method": "PUT"})
	}

	r.PUT("/test", handler)

	req := httptest.NewRequest(http.MethodPut, "/test", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRouter_DELETE(t *testing.T) {
	r := NewRouter()
	handler := func(ctx Context) error {
		return ctx.NoContent(http.StatusNoContent)
	}

	r.DELETE("/test", handler)

	req := httptest.NewRequest(http.MethodDelete, "/test", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestRouter_PathParams(t *testing.T) {
	r := NewRouter()
	handler := func(ctx Context) error {
		id := ctx.Param("id")
		return ctx.JSON(http.StatusOK, map[string]string{"id": id})
	}

	r.GET("/items/{id}", handler)

	req := httptest.NewRequest(http.MethodGet, "/items/abc123", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "abc123", body["id"])
}

func TestRouter_QueryParams(t *testing.T) {
	r := NewRouter()
	handler := func(ctx Context) error {
		name := ctx.Query("name")
		page := ctx.QueryDefault("page", "1")
		missing := ctx.QueryDefault("missing", "default")
		return ctx.JSON(http.StatusOK, map[string]string{
			"name":    name,
			"page":    page,
			"missing": missing,
		})
	}

	r.GET("/search", handler)

	req := httptest.NewRequest(http.MethodGet, "/search?name=test&page=2", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "test", body["name"])
	assert.Equal(t, "2", body["page"])
	assert.Equal(t, "default", body["missing"])
}

func TestRouter_Group(t *testing.T) {
	r := NewRouter()
	api := r.Group("/api/v1")

	api.GET("/users", func(ctx Context) error {
		return ctx.JSON(http.StatusOK, map[string]string{"resource": "users"})
	})

	api.POST("/users", func(ctx Context) error {
		return ctx.JSON(http.StatusCreated, map[string]string{"action": "create"})
	})

	assert.Equal(t, 2, r.RouteCount())

	// GET /api/v1/users
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "users", body["resource"])

	// POST /api/v1/users
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users", nil)
	rec = httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestRouter_NestedGroup(t *testing.T) {
	r := NewRouter()
	api := r.Group("/api")
	v1 := api.Group("/v1")
	v1.GET("/items", func(ctx Context) error {
		return ctx.JSON(http.StatusOK, map[string]string{"nested": "true"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/items", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRouter_GroupWithMiddleware(t *testing.T) {
	r := NewRouter()

	// 글로벌 미들웨어: 헤더 추가
	r.Use(func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			ctx.SetHeader("X-Global", "true")
			return next(ctx)
		}
	})

	// 그룹 미들웨어: 헤더 추가
	api := r.Group("/api", func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			ctx.SetHeader("X-Group", "true")
			return next(ctx)
		}
	})

	api.GET("/test", func(ctx Context) error {
		return ctx.JSON(http.StatusOK, map[string]string{"ok": "true"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "true", rec.Header().Get("X-Global"))
	assert.Equal(t, "true", rec.Header().Get("X-Group"))
}

func TestRouter_MiddlewareChainOrder(t *testing.T) {
	r := NewRouter()
	var order []string

	r.Use(func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			order = append(order, "global")
			return next(ctx)
		}
	})

	group := r.Group("/api", func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			order = append(order, "group")
			return next(ctx)
		}
	})

	routeMW := func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			order = append(order, "route")
			return next(ctx)
		}
	}

	group.GET("/test", func(ctx Context) error {
		order = append(order, "handler")
		return ctx.NoContent(http.StatusOK)
	}, routeMW)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"global", "group", "route", "handler"}, order)
}

func TestRouter_HandlerError(t *testing.T) {
	r := NewRouter()
	r.GET("/error", func(ctx Context) error {
		return ErrNotFound.WithMessage("item not found")
	})

	req := httptest.NewRequest(http.MethodGet, "/error", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

	var body map[string]any
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, false, body["success"])
}

func TestRouter_HandlerGenericError(t *testing.T) {
	r := NewRouter()
	r.GET("/error", func(ctx Context) error {
		return io.EOF // 비 APIError
	})

	req := httptest.NewRequest(http.MethodGet, "/error", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	// 비 APIError는 500으로 매핑된다
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestRouter_Handler(t *testing.T) {
	r := NewRouter()
	h := r.Handler()
	require.NotNil(t, h)
	assert.IsType(t, &http.ServeMux{}, h)
}

func TestRouteGroup_Use(t *testing.T) {
	r := NewRouter()
	g := r.Group("/api")
	assert.Empty(t, g.middlewares)

	mw := func(next HandlerFunc) HandlerFunc { return next }
	g.Use(mw)
	assert.Len(t, g.middlewares, 1)
}

func TestRouteGroup_SubGroup(t *testing.T) {
	r := NewRouter()
	api := r.Group("/api")
	v1 := api.Group("/v1")

	assert.Equal(t, "/api/v1", v1.prefix)
}

func TestRouteGroup_PUT(t *testing.T) {
	r := NewRouter()
	g := r.Group("/api")
	g.PUT("/items/{id}", func(ctx Context) error {
		return ctx.JSON(http.StatusOK, map[string]string{"id": ctx.Param("id")})
	})

	req := httptest.NewRequest(http.MethodPut, "/api/items/42", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "42", body["id"])
}

func TestRouteGroup_DELETE(t *testing.T) {
	r := NewRouter()
	g := r.Group("/api")
	g.DELETE("/items/{id}", func(ctx Context) error {
		return ctx.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/items/42", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

// --- httpContext 테스트 ---

func TestHTTPContext_Param(t *testing.T) {
	r := NewRouter()
	var capturedID string

	r.GET("/items/{id}", func(ctx Context) error {
		capturedID = ctx.Param("id")
		return ctx.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/items/test-id-123", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, "test-id-123", capturedID)
}

func TestHTTPContext_Query(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		key      string
		expected string
	}{
		{
			name:     "existing query param",
			url:      "/test?name=value",
			key:      "name",
			expected: "value",
		},
		{
			name:     "missing query param returns empty",
			url:      "/test?name=value",
			key:      "missing",
			expected: "",
		},
		{
			name:     "empty query param",
			url:      "/test?name=",
			key:      "name",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter()
			var result string
			r.GET("/test", func(ctx Context) error {
				result = ctx.Query(tt.key)
				return ctx.NoContent(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			r.Handler().ServeHTTP(rec, req)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHTTPContext_QueryDefault(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		key      string
		def      string
		expected string
	}{
		{
			name:     "existing param returns value",
			url:      "/test?page=5",
			key:      "page",
			def:      "1",
			expected: "5",
		},
		{
			name:     "missing param returns default",
			url:      "/test",
			key:      "page",
			def:      "1",
			expected: "1",
		},
		{
			name:     "empty param returns default",
			url:      "/test?page=",
			key:      "page",
			def:      "1",
			expected: "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter()
			var result string
			r.GET("/test", func(ctx Context) error {
				result = ctx.QueryDefault(tt.key, tt.def)
				return ctx.NoContent(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			r.Handler().ServeHTTP(rec, req)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHTTPContext_Bind(t *testing.T) {
	type testBody struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	t.Run("valid JSON body", func(t *testing.T) {
		r := NewRouter()
		var bound testBody

		r.POST("/test", func(ctx Context) error {
			if err := ctx.Bind(&bound); err != nil {
				return err
			}
			return ctx.NoContent(http.StatusOK)
		})

		body := `{"name":"test","age":25}`
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "test", bound.Name)
		assert.Equal(t, 25, bound.Age)
	})

	t.Run("invalid JSON body", func(t *testing.T) {
		r := NewRouter()
		r.POST("/test", func(ctx Context) error {
			var v testBody
			return ctx.Bind(&v)
		})

		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("invalid json"))
		rec := httptest.NewRecorder()
		r.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("nil body", func(t *testing.T) {
		r := NewRouter()
		r.POST("/test", func(ctx Context) error {
			var v testBody
			return ctx.Bind(&v)
		})

		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.Body = nil
		rec := httptest.NewRecorder()
		r.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("unknown fields rejected", func(t *testing.T) {
		r := NewRouter()
		r.POST("/test", func(ctx Context) error {
			var v testBody
			return ctx.Bind(&v)
		})

		body := `{"name":"test","unknown_field":"value"}`
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
		rec := httptest.NewRecorder()
		r.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestHTTPContext_JSON(t *testing.T) {
	t.Run("writes JSON response", func(t *testing.T) {
		r := NewRouter()
		r.GET("/test", func(ctx Context) error {
			return ctx.JSON(http.StatusOK, map[string]string{"key": "value"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		r.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

		var body map[string]string
		err := json.NewDecoder(rec.Body).Decode(&body)
		require.NoError(t, err)
		assert.Equal(t, "value", body["key"])
	})

	t.Run("nil value writes no body", func(t *testing.T) {
		r := NewRouter()
		r.GET("/test", func(ctx Context) error {
			return ctx.JSON(http.StatusOK, nil)
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		r.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Empty(t, rec.Body.String())
	})

	t.Run("double write is no-op", func(t *testing.T) {
		r := NewRouter()
		r.GET("/test", func(ctx Context) error {
			_ = ctx.JSON(http.StatusOK, map[string]string{"first": "true"})
			// 두 번째 JSON 호출은 무시된다
			return ctx.JSON(http.StatusInternalServerError, map[string]string{"second": "true"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		r.Handler().ServeHTTP(rec, req)

		// 첫 번째 응답만 반영된다
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestHTTPContext_NoContent(t *testing.T) {
	r := NewRouter()
	r.DELETE("/test", func(ctx Context) error {
		return ctx.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodDelete, "/test", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.String())
}

func TestHTTPContext_SetHeader_GetHeader(t *testing.T) {
	r := NewRouter()
	var gotHeader string

	r.GET("/test", func(ctx Context) error {
		gotHeader = ctx.GetHeader("X-Custom")
		ctx.SetHeader("X-Response", "set")
		return ctx.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Custom", "input")
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, "input", gotHeader)
	assert.Equal(t, "set", rec.Header().Get("X-Response"))
}

func TestHTTPContext_RealIP(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected string
	}{
		{
			name:     "X-Forwarded-For single IP",
			headers:  map[string]string{"X-Forwarded-For": "1.2.3.4"},
			expected: "1.2.3.4",
		},
		{
			name:     "X-Forwarded-For multiple IPs (first used)",
			headers:  map[string]string{"X-Forwarded-For": "1.2.3.4, 5.6.7.8"},
			expected: "1.2.3.4",
		},
		{
			name:     "X-Real-IP fallback",
			headers:  map[string]string{"X-Real-IP": "9.8.7.6"},
			expected: "9.8.7.6",
		},
		{
			name:     "X-Forwarded-For takes priority over X-Real-IP",
			headers:  map[string]string{"X-Forwarded-For": "1.2.3.4", "X-Real-IP": "9.8.7.6"},
			expected: "1.2.3.4",
		},
		{
			name:     "RemoteAddr fallback",
			headers:  map[string]string{},
			expected: "192.0.2.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter()
			var realIP string

			r.GET("/test", func(ctx Context) error {
				realIP = ctx.RealIP()
				return ctx.NoContent(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			r.Handler().ServeHTTP(rec, req)

			assert.Equal(t, tt.expected, realIP)
		})
	}
}

func TestHTTPContext_MethodAndPath(t *testing.T) {
	r := NewRouter()
	var method, path string

	r.POST("/api/test", func(ctx Context) error {
		method = ctx.Method()
		path = ctx.Path()
		return ctx.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, "POST", method)
	assert.Equal(t, "/api/test", path)
}

func TestHTTPContext_Context(t *testing.T) {
	r := NewRouter()
	var hasContext bool

	r.GET("/test", func(ctx Context) error {
		hasContext = ctx.Context() != nil
		return ctx.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.True(t, hasContext)
}

func TestHTTPContext_RequestIDFromContext(t *testing.T) {
	r := NewRouter()
	var requestID string

	// 컨텍스트에 요청 ID가 없을 때 빈 문자열 반환
	r.GET("/test", func(ctx Context) error {
		requestID = ctx.RequestID()
		return ctx.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, "", requestID)
}

func TestHTTPContext_UserIDAndRoleFromContext(t *testing.T) {
	r := NewRouter()
	var userID, userRole string

	// 컨텍스트에 사용자 정보가 없을 때 빈 문자열 반환
	r.GET("/test", func(ctx Context) error {
		userID = ctx.UserID()
		userRole = ctx.UserRole()
		return ctx.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	assert.Equal(t, "", userID)
	assert.Equal(t, "", userRole)
}

func TestApplyMiddleware(t *testing.T) {
	var order []int

	mw1 := func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			order = append(order, 1)
			return next(ctx)
		}
	}
	mw2 := func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			order = append(order, 2)
			return next(ctx)
		}
	}
	handler := func(ctx Context) error {
		order = append(order, 3)
		return nil
	}

	wrapped := applyMiddleware(handler, []MiddlewareFunc{mw1, mw2})

	// httpContext 생성
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	ctx := newHTTPContext(rec, req)

	err := wrapped(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, order)
}

func TestApplyMiddleware_Empty(t *testing.T) {
	called := false
	handler := func(ctx Context) error {
		called = true
		return nil
	}

	wrapped := applyMiddleware(handler, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	ctx := newHTTPContext(rec, req)

	err := wrapped(ctx)
	assert.NoError(t, err)
	assert.True(t, called)
}

func TestHandleError_APIError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := newHTTPContext(rec, req)

	handleError(ctx, ErrNotFound.WithMessage("resource not found"))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
}

func TestHandleError_NonAPIError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := newHTTPContext(rec, req)

	handleError(ctx, io.EOF)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandleError_AlreadyWritten(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := newHTTPContext(rec, req)
	ctx.written = true

	// 이미 응답이 쓰여진 경우 handleError는 아무것도 하지 않는다
	handleError(ctx, ErrNotFound)
	// 응답 본문이 비어있어야 한다
	assert.Empty(t, rec.Body.String())
}

func TestGenerateRequestID(t *testing.T) {
	id := generateRequestID()
	assert.NotEmpty(t, id)

	// UUID v4 형식 확인 (8-4-4-4-12)
	parts := strings.Split(id, "-")
	assert.Len(t, parts, 5)
	assert.Len(t, parts[0], 8)
	assert.Len(t, parts[1], 4)
	assert.Len(t, parts[2], 4)
	assert.Len(t, parts[3], 4)
	assert.Len(t, parts[4], 12)

	// 두 ID는 서로 달라야 한다
	id2 := generateRequestID()
	assert.NotEqual(t, id, id2)
}

func TestHTTPContext_BindWithLargeBody(t *testing.T) {
	r := NewRouter()
	r.POST("/test", func(ctx Context) error {
		var v map[string]any
		return ctx.Bind(&v)
	})

	// 큰 JSON 본문
	data := make(map[string]string)
	for i := 0; i < 100; i++ {
		data[strings.Repeat("k", 10)] = strings.Repeat("v", 100)
	}
	body, _ := json.Marshal(data)

	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	// DisallowUnknownFields가 없으면 통과해야 함
	// map[string]any로 바인딩하므로 모든 필드를 허용한다
	assert.Equal(t, http.StatusOK, rec.Code)
}
