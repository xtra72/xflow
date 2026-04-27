package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/node"
)

// --- Mock NodeRegistry ---

type mockNodeRegistry struct {
	listNodeTypesFn func(ctx context.Context) ([]NodeTypeInfo, error)
	getNodeTypeFn   func(ctx context.Context, typeName string) (*NodeTypeInfo, error)
}

func (m *mockNodeRegistry) ListNodeTypes(ctx context.Context) ([]NodeTypeInfo, error) {
	if m.listNodeTypesFn != nil {
		return m.listNodeTypesFn(ctx)
	}
	return nil, nil
}

func (m *mockNodeRegistry) GetNodeType(ctx context.Context, typeName string) (*NodeTypeInfo, error) {
	if m.getNodeTypeFn != nil {
		return m.getNodeTypeFn(ctx, typeName)
	}
	return nil, nil
}

// --- 헬퍼 ---

func setupNodeRouter(mock *mockNodeRegistry) *api.Router {
	router := api.NewRouter()
	h := NewNodeHandler(mock, nil)
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router
}

// --- NewNodeHandler 테스트 ---

func TestNewNodeHandler(t *testing.T) {
	mock := &mockNodeRegistry{}

	t.Run("nil logger 시 기본 로거 사용", func(t *testing.T) {
		h := NewNodeHandler(mock, nil)
		require.NotNil(t, h)
		assert.NotNil(t, h.logger)
		assert.Equal(t, mock, h.registry)
	})
}

// --- RegisterRoutes 테스트 ---

func TestNodeHandler_RegisterRoutes(t *testing.T) {
	router := setupNodeRouter(&mockNodeRegistry{})
	// 2개 라우트: GET /nodes, GET /nodes/{type}
	assert.Equal(t, 2, router.RouteCount())
}

// --- List 테스트 ---

func TestNodeHandler_List(t *testing.T) {
	tests := []struct {
		name         string
		mock         *mockNodeRegistry
		expectedCode int
		expectedLen  int
	}{
		{
			name: "성공: 빈 목록",
			mock: &mockNodeRegistry{
				listNodeTypesFn: func(_ context.Context) ([]NodeTypeInfo, error) {
					return []NodeTypeInfo{}, nil
				},
			},
			expectedCode: http.StatusOK,
			expectedLen:  0,
		},
		{
			name: "성공: 노드 타입 목록",
			mock: &mockNodeRegistry{
				listNodeTypesFn: func(_ context.Context) ([]NodeTypeInfo, error) {
					return []NodeTypeInfo{
						{Type: "filter", Category: "processing", Description: "필터", Source: "builtin"},
						{Type: "transform", Category: "processing", Description: "변환", Source: "builtin"},
					}, nil
				},
			},
			expectedCode: http.StatusOK,
			expectedLen:  2,
		},
		{
			name: "에러: 내부 오류",
			mock: &mockNodeRegistry{
				listNodeTypesFn: func(_ context.Context) ([]NodeTypeInfo, error) {
					return nil, errors.New("internal error")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupNodeRouter(tt.mock)
			rec := doRequest(t, router, http.MethodGet, "/api/v1/nodes", nil)
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusOK {
				var resp dto.APIResponse[[]NodeTypeInfo]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				assert.Len(t, resp.Data, tt.expectedLen)
			}
		})
	}
}

// --- Get 테스트 ---

func TestNodeHandler_Get(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		mock         *mockNodeRegistry
		expectedCode int
	}{
		{
			name: "성공: 노드 타입 조회",
			url:  "/api/v1/nodes/filter",
			mock: &mockNodeRegistry{
				getNodeTypeFn: func(_ context.Context, typeName string) (*NodeTypeInfo, error) {
					assert.Equal(t, "filter", typeName)
					return &NodeTypeInfo{
						Type:        "filter",
						Category:    "processing",
						Description: "조건에 따라 메시지를 필터링",
						Source:      "builtin",
					}, nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "에러: 미등록 타입 404",
			url:  "/api/v1/nodes/unknown",
			mock: &mockNodeRegistry{
				getNodeTypeFn: func(_ context.Context, _ string) (*NodeTypeInfo, error) {
					return nil, node.ErrNodeTypeNotFound
				},
			},
			expectedCode: http.StatusNotFound,
		},
		{
			name: "에러: 내부 오류",
			url:  "/api/v1/nodes/error",
			mock: &mockNodeRegistry{
				getNodeTypeFn: func(_ context.Context, _ string) (*NodeTypeInfo, error) {
					return nil, errors.New("internal error")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupNodeRouter(tt.mock)
			rec := doRequest(t, router, http.MethodGet, tt.url, nil)
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusOK {
				var resp dto.APIResponse[NodeTypeInfo]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				assert.Equal(t, "filter", resp.Data.Type)
			}
		})
	}
}
