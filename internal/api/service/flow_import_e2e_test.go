package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/pkg/flow"
)

// importDef 는 고정 id 를 가진 React Flow 정의를 만든다(매 호출마다 동일 구조).
func importDef() map[string]any {
	return map[string]any{
		"nodes": []any{
			map[string]any{
				"id":   "node-1",
				"type": "custom",
				"data": map[string]any{"label": "src", "nodeType": "src"},
			},
			map[string]any{
				"id":   "node-2",
				"type": "custom",
				"data": map[string]any{"label": "dst", "nodeType": "dst"},
			},
		},
		"edges": []any{
			map[string]any{
				"id":     "edge-1",
				"source": "node-1",
				"target": "node-2",
			},
		},
	}
}

// findWireBySrcDst 는 src→dst 연결을 가진 와이어를 찾는다.
func nodeIDSet(f flow.Flow) map[string]bool {
	set := make(map[string]bool)
	for _, n := range f.Nodes() {
		set[n.ID] = true
	}
	return set
}

// TestCreateFlow_ImportMode_RegeneratesIDsAcrossImports 는 동일 정의를 두 번
// import 하면 노드/와이어 ID 가 서로 충돌하지 않음을 검증한다. (requirement 2)
func TestCreateFlow_ImportMode_RegeneratesIDsAcrossImports(t *testing.T) {
	eng := newTestEngine()
	repo := newTestRepo(t)
	adapter := NewFlowServiceAdapter(eng, repo, nil)
	ctx := context.Background()

	info1, err := adapter.CreateFlow(ctx, &dto.FlowCreateRequest{
		Name:          "import-flow-1",
		Definition:    importDef(),
		RegenerateIDs: true,
	})
	require.NoError(t, err)

	info2, err := adapter.CreateFlow(ctx, &dto.FlowCreateRequest{
		Name:          "import-flow-2",
		Definition:    importDef(),
		RegenerateIDs: true,
	})
	require.NoError(t, err)

	f1, err := repo.Get(ctx, info1.ID)
	require.NoError(t, err)
	f2, err := repo.Get(ctx, info2.ID)
	require.NoError(t, err)

	// 1) 노드 ID 가 고정 값("node-1"/"node-2")이 아니어야 한다
	for _, n := range f1.Nodes() {
		assert.NotEqual(t, "node-1", n.ID)
		assert.NotEqual(t, "node-2", n.ID)
		assert.Len(t, n.ID, 36, "UUID 형식이어야 한다")
	}

	// 2) 두 import 의 노드 ID 가 서로 충돌하지 않아야 한다
	ids1 := nodeIDSet(f1)
	ids2 := nodeIDSet(f2)
	for id := range ids1 {
		assert.False(t, ids2[id], "두 import 의 노드 ID 가 충돌해서는 안 된다: %s", id)
	}

	// 3) 와이어가 같은 src→dst 노드 쌍을 연결하고 dangling 참조가 없어야 한다
	require.Len(t, f1.Wires(), 1)
	w1 := f1.Wires()[0]
	assert.True(t, ids1[w1.SourceNodeID], "source 가 존재하는 노드를 가리켜야 한다")
	assert.True(t, ids1[w1.TargetNodeID], "target 가 존재하는 노드를 가리켜야 한다")
	assert.NotEqual(t, "edge-1", w1.ID, "와이어 ID 도 재생성되어야 한다")

	// 4) 두 import 의 와이어 ID 도 충돌하지 않아야 한다
	require.Len(t, f2.Wires(), 1)
	assert.NotEqual(t, w1.ID, f2.Wires()[0].ID)
}

// TestCreateFlow_NormalMode_PreservesIDs 는 RegenerateIDs 미설정 시(일반 저장)
// 입력 노드 ID 가 보존됨을 검증한다. (CONSTRAINT: 일반 저장은 regen 금지)
func TestCreateFlow_NormalMode_PreservesIDs(t *testing.T) {
	eng := newTestEngine()
	repo := newTestRepo(t)
	adapter := NewFlowServiceAdapter(eng, repo, nil)
	ctx := context.Background()

	info, err := adapter.CreateFlow(ctx, &dto.FlowCreateRequest{
		Name:       "normal-flow",
		Definition: importDef(),
		// RegenerateIDs 미설정 → false
	})
	require.NoError(t, err)

	f, err := repo.Get(ctx, info.ID)
	require.NoError(t, err)

	ids := nodeIDSet(f)
	assert.True(t, ids["node-1"], "일반 저장 시 노드 ID 가 보존되어야 한다")
	assert.True(t, ids["node-2"], "일반 저장 시 노드 ID 가 보존되어야 한다")

	require.Len(t, f.Wires(), 1)
	w := f.Wires()[0]
	assert.Equal(t, "node-1", w.SourceNodeID)
	assert.Equal(t, "node-2", w.TargetNodeID)
}
