package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/node"
)

func TestNewNodeServiceAdapter(t *testing.T) {
	r := node.NewRegistry()

	t.Run("nil logger 시 기본 로거 사용", func(t *testing.T) {
		a := NewNodeServiceAdapter(r, nil)
		require.NotNil(t, a)
		assert.NotNil(t, a.logger)
	})
}

func TestNodeServiceAdapter_ListNodeTypes(t *testing.T) {
	r := node.NewRegistry()
	a := NewNodeServiceAdapter(r, nil)

	types, err := a.ListNodeTypes(context.Background())
	require.NoError(t, err)
	// 31개 빌트인 노드 타입
	assert.Len(t, types, 31)

	// 정렬 확인 (AllTypeMeta가 정렬된 결과를 반환)
	for i := 1; i < len(types); i++ {
		assert.True(t, types[i-1].Type < types[i].Type,
			"타입이 정렬되어 있어야 한다: %s < %s", types[i-1].Type, types[i].Type)
	}

	// 필수 필드 확인
	for _, nt := range types {
		assert.NotEmpty(t, nt.Type)
		assert.NotEmpty(t, nt.Category)
		assert.NotEmpty(t, nt.Description)
		assert.Equal(t, "builtin", nt.Source)
	}
}

func TestNodeServiceAdapter_GetNodeType_Found(t *testing.T) {
	r := node.NewRegistry()
	a := NewNodeServiceAdapter(r, nil)

	info, err := a.GetNodeType(context.Background(), "filter")
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "filter", info.Type)
	assert.Equal(t, "processing", info.Category)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, "builtin", info.Source)
}

func TestNodeServiceAdapter_GetNodeType_NotFound(t *testing.T) {
	r := node.NewRegistry()
	a := NewNodeServiceAdapter(r, nil)

	info, err := a.GetNodeType(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, node.ErrNodeTypeNotFound)
	assert.Nil(t, info)
}
