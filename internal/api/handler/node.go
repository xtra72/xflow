package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
)

// NodeRegistry 는 노드 타입 조회를 위한 인터페이스이다.
type NodeRegistry interface {
	ListNodeTypes(ctx context.Context) ([]NodeTypeInfo, error)
	GetNodeType(ctx context.Context, typeName string) (*NodeTypeInfo, error)
}

// NodeTypeInfo 는 노드 타입 정보를 나타낸다.
type NodeTypeInfo struct {
	Type        string `json:"type"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

// NodeHandler 는 노드 타입 관련 API 엔드포인트를 처리한다.
type NodeHandler struct {
	registry NodeRegistry
	logger   *slog.Logger
}

// NewNodeHandler 는 새 NodeHandler를 생성한다.
func NewNodeHandler(registry NodeRegistry, logger *slog.Logger) *NodeHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &NodeHandler{
		registry: registry,
		logger:   logger,
	}
}

// RegisterRoutes 는 노드 타입 관련 라우트를 등록한다.
func (h *NodeHandler) RegisterRoutes(g *api.RouteGroup) {
	// @SPEC:SPEC-AUTH-005 (M5) — node 는 카탈로그 조회 전용이라 read 만 존재한다.
	g.GETPerm("/nodes", "node.read", h.List)
	g.GETPerm("/nodes/{type}", "node.read", h.Get)
}

// List 는 등록된 모든 노드 타입 목록을 반환한다.
// GET /nodes
func (h *NodeHandler) List(ctx api.Context) error {
	types, err := h.registry.ListNodeTypes(ctx.Context())
	if err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(types))
}

// Get 은 특정 노드 타입의 상세 정보를 반환한다.
// GET /nodes/{type}
func (h *NodeHandler) Get(ctx api.Context) error {
	typeName := ctx.Param("type")
	if typeName == "" {
		return api.ErrBadRequest.WithMessage("node type is required")
	}

	info, err := h.registry.GetNodeType(ctx.Context(), typeName)
	if err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(info))
}
