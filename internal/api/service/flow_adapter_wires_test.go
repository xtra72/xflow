package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNormalizeReactFlowDefinition_FlatNodesWithEdges_ConvertsToWires 는 노드가
// `data` 필드 없이 flat 구조로 들어오더라도 React Flow 스타일 `edges` 가
// XFlow 표준 `wires` 로 변환되는지 확인한다. (data-loss hotfix W)
func TestNormalizeReactFlowDefinition_FlatNodesWithEdges_ConvertsToWires(t *testing.T) {
	def := map[string]any{
		"nodes": []any{
			map[string]any{"id": "a", "type": "trigger"},
			map[string]any{"id": "b", "type": "output"},
		},
		"edges": []any{
			map[string]any{
				"id":           "e1",
				"source":       "a",
				"target":       "b",
				"sourceHandle": "out",
				"targetHandle": "in",
			},
		},
	}

	got := normalizeReactFlowDefinition(def)

	// edges 키는 wires 로 치환되어 사라져야 한다.
	_, hasEdges := got["edges"]
	assert.False(t, hasEdges, "edges 키는 wires 변환 후 제거되어야 한다")

	wiresRaw, ok := got["wires"].([]any)
	require.True(t, ok, "wires 가 []any 슬라이스여야 한다")
	require.Len(t, wiresRaw, 1, "wires 는 1건이어야 한다")

	w0 := wiresRaw[0].(map[string]any)
	assert.Equal(t, "e1", w0["id"])
	assert.Equal(t, "a", w0["source_node_id"])
	assert.Equal(t, "b", w0["target_node_id"])
	assert.Equal(t, "out", w0["source_port"])
	assert.Equal(t, "in", w0["target_port"])

	// React Flow 원본 키는 wires 항목에 남으면 안 된다.
	_, hasSource := w0["source"]
	_, hasTarget := w0["target"]
	_, hasSH := w0["sourceHandle"]
	_, hasTH := w0["targetHandle"]
	assert.False(t, hasSource, "wires 항목에 source 키가 남으면 안 된다")
	assert.False(t, hasTarget, "wires 항목에 target 키가 남으면 안 된다")
	assert.False(t, hasSH, "wires 항목에 sourceHandle 키가 남으면 안 된다")
	assert.False(t, hasTH, "wires 항목에 targetHandle 키가 남으면 안 된다")
}

// TestNormalizeReactFlowDefinition_ReactFlowNodesWithEdges_StillConverts 는
// 기존 React Flow 형식(노드에 data 필드 존재)에서도 wires 변환이 정상 동작하는지
// 회귀 확인한다.
func TestNormalizeReactFlowDefinition_ReactFlowNodesWithEdges_StillConverts(t *testing.T) {
	def := map[string]any{
		"nodes": []any{
			map[string]any{
				"id":   "a",
				"type": "custom",
				"data": map[string]any{
					"nodeType": "trigger",
					"label":    "Start",
				},
			},
			map[string]any{
				"id":   "b",
				"type": "custom",
				"data": map[string]any{
					"nodeType": "output",
					"label":    "End",
				},
			},
		},
		"edges": []any{
			map[string]any{
				"id":           "e1",
				"source":       "a",
				"target":       "b",
				"sourceHandle": "out",
				"targetHandle": "in",
			},
		},
	}

	got := normalizeReactFlowDefinition(def)

	_, hasEdges := got["edges"]
	assert.False(t, hasEdges, "edges 키가 제거되어야 한다")

	wiresRaw, ok := got["wires"].([]any)
	require.True(t, ok)
	require.Len(t, wiresRaw, 1)

	w0 := wiresRaw[0].(map[string]any)
	assert.Equal(t, "e1", w0["id"])
	assert.Equal(t, "a", w0["source_node_id"])
	assert.Equal(t, "b", w0["target_node_id"])
	assert.Equal(t, "out", w0["source_port"])
	assert.Equal(t, "in", w0["target_port"])

	// React Flow 노드는 정규화 후에도 nodes 가 존재해야 한다.
	nodes, ok := got["nodes"].([]any)
	require.True(t, ok)
	require.Len(t, nodes, 2)
}

// TestNormalizeReactFlowDefinition_CanonicalWires_Idempotent 는 이미 표준
// `wires` 형식으로 들어온 정의가 변경 없이 보존되는지 확인한다.
func TestNormalizeReactFlowDefinition_CanonicalWires_Idempotent(t *testing.T) {
	def := map[string]any{
		"nodes": []any{
			map[string]any{"id": "a", "type": "trigger"},
			map[string]any{"id": "b", "type": "output"},
		},
		"wires": []any{
			map[string]any{
				"id":             "w1",
				"source_node_id": "a",
				"target_node_id": "b",
				"source_port":    "out",
				"target_port":    "in",
			},
		},
	}

	got := normalizeReactFlowDefinition(def)

	_, hasEdges := got["edges"]
	assert.False(t, hasEdges, "edges 키가 등장하면 안 된다")

	wiresRaw, ok := got["wires"].([]any)
	require.True(t, ok)
	require.Len(t, wiresRaw, 1, "wires 항목 개수가 유지되어야 한다")

	w0 := wiresRaw[0].(map[string]any)
	assert.Equal(t, "w1", w0["id"])
	assert.Equal(t, "a", w0["source_node_id"])
	assert.Equal(t, "b", w0["target_node_id"])
	assert.Equal(t, "out", w0["source_port"])
	assert.Equal(t, "in", w0["target_port"])

	// 변환 후에도 React Flow 키가 추가되면 안 된다.
	_, hasSource := w0["source"]
	_, hasSH := w0["sourceHandle"]
	assert.False(t, hasSource, "source 키가 추가되면 안 된다")
	assert.False(t, hasSH, "sourceHandle 키가 추가되면 안 된다")
}

// TestNormalizeReactFlowDefinition_NoEdgesNoWires_NoOp 는 edges/wires 가 모두
// 없는 입력에서 함수가 무동작(no-op)으로 동작하는지 확인한다.
func TestNormalizeReactFlowDefinition_NoEdgesNoWires_NoOp(t *testing.T) {
	def := map[string]any{
		"nodes": []any{
			map[string]any{"id": "a", "type": "trigger"},
		},
	}

	got := normalizeReactFlowDefinition(def)

	_, hasEdges := got["edges"]
	_, hasWires := got["wires"]
	assert.False(t, hasEdges, "입력에 없던 edges 키가 생성되면 안 된다")
	assert.False(t, hasWires, "입력에 없던 wires 키가 생성되면 안 된다")

	nodes, ok := got["nodes"].([]any)
	require.True(t, ok)
	require.Len(t, nodes, 1)
}
