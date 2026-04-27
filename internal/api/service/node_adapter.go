package service

import (
	"context"
	"log/slog"

	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/node"
)

// NodeServiceAdapter 는 handler.NodeRegistry 인터페이스를 구현하여
// node.Registry 와 연결하는 서비스 어댑터이다.
type NodeServiceAdapter struct {
	registry *node.Registry
	logger   *slog.Logger
}

// NewNodeServiceAdapter 는 새 NodeServiceAdapter 를 생성한다.
func NewNodeServiceAdapter(registry *node.Registry, logger *slog.Logger) *NodeServiceAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &NodeServiceAdapter{
		registry: registry,
		logger:   logger,
	}
}

// ListNodeTypes 는 등록된 모든 노드 타입 정보를 반환한다.
func (a *NodeServiceAdapter) ListNodeTypes(_ context.Context) ([]handler.NodeTypeInfo, error) {
	metas := a.registry.AllTypeMeta()
	result := make([]handler.NodeTypeInfo, len(metas))
	for i, m := range metas {
		result[i] = handler.NodeTypeInfo{
			Type:        m.Type,
			Category:    m.Category,
			Description: m.Description,
			Source:      m.Source,
		}
	}
	return result, nil
}

// GetNodeType 는 특정 노드 타입의 정보를 반환한다.
// 등록되지 않은 타입이면 node.ErrNodeTypeNotFound를 반환한다.
func (a *NodeServiceAdapter) GetNodeType(_ context.Context, typeName string) (*handler.NodeTypeInfo, error) {
	meta, ok := a.registry.TypeMeta(typeName)
	if !ok {
		return nil, node.ErrNodeTypeNotFound
	}
	return &handler.NodeTypeInfo{
		Type:        meta.Type,
		Category:    meta.Category,
		Description: meta.Description,
		Source:      meta.Source,
	}, nil
}
