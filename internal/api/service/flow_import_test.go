package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeReactFlowDef 는 React Flow 스타일의 고정 id 정의를 만든다.
func makeReactFlowDef() map[string]any {
	return map[string]any{
		"name": "test-flow",
		"nodes": []any{
			map[string]any{
				"id":   "node-1",
				"type": "custom",
				"data": map[string]any{"label": "A"},
			},
			map[string]any{
				"id":   "node-2",
				"type": "custom",
				"data": map[string]any{"label": "B"},
			},
		},
		"edges": []any{
			map[string]any{
				"id":           "edge-1",
				"source":       "node-1",
				"target":       "node-2",
				"sourceHandle": "out",
				"targetHandle": "in",
			},
		},
	}
}

// collectNodeIDs 는 정의에서 노드 id 목록을 추출한다.
func collectNodeIDs(def map[string]any) []string {
	var ids []string
	for _, n := range def["nodes"].([]any) {
		ids = append(ids, n.(map[string]any)["id"].(string))
	}
	return ids
}

func TestRegenerateDefinitionIDs_AssignsFreshNodeIDs(t *testing.T) {
	t.Parallel()

	def := makeReactFlowDef()
	out := RegenerateDefinitionIDs(def)

	ids := collectNodeIDs(out)
	require.Len(t, ids, 2)

	// 고정 id 가 모두 교체되어야 한다
	assert.NotEqual(t, "node-1", ids[0])
	assert.NotEqual(t, "node-2", ids[1])
	// 서로 달라야 한다
	assert.NotEqual(t, ids[0], ids[1])
	// UUID 형식(36자, 하이픈 4개)인지 간단 확인
	assert.Len(t, ids[0], 36)
	assert.Len(t, ids[1], 36)
}

func TestRegenerateDefinitionIDs_RewritesEdgeRefs(t *testing.T) {
	t.Parallel()

	def := makeReactFlowDef()
	out := RegenerateDefinitionIDs(def)

	nodes := out["nodes"].([]any)
	newID1 := nodes[0].(map[string]any)["id"].(string)
	newID2 := nodes[1].(map[string]any)["id"].(string)

	edges := out["edges"].([]any)
	require.Len(t, edges, 1)
	edge := edges[0].(map[string]any)

	// source/target 가 새 노드 id 로 재작성되어야 한다 (같은 src→dst 연결 유지)
	assert.Equal(t, newID1, edge["source"])
	assert.Equal(t, newID2, edge["target"])

	// edge id 도 새 UUID 로 교체되어야 한다
	assert.NotEqual(t, "edge-1", edge["id"])
	assert.Len(t, edge["id"].(string), 36)

	// 포트 핸들은 그대로 유지
	assert.Equal(t, "out", edge["sourceHandle"])
	assert.Equal(t, "in", edge["targetHandle"])
}

func TestRegenerateDefinitionIDs_TwiceYieldsDistinctIDs(t *testing.T) {
	t.Parallel()

	out1 := RegenerateDefinitionIDs(makeReactFlowDef())
	out2 := RegenerateDefinitionIDs(makeReactFlowDef())

	ids1 := collectNodeIDs(out1)
	ids2 := collectNodeIDs(out2)

	// 두 번의 import 가 서로 다른 id 를 만들어야 한다 (충돌 방지)
	for _, a := range ids1 {
		for _, b := range ids2 {
			assert.NotEqual(t, a, b, "두 import 의 노드 id 가 충돌해서는 안 된다")
		}
	}
}

func TestRegenerateDefinitionIDs_NoDanglingReferences(t *testing.T) {
	t.Parallel()

	out := RegenerateDefinitionIDs(makeReactFlowDef())

	// 새 노드 id 집합
	nodeIDs := make(map[string]bool)
	for _, id := range collectNodeIDs(out) {
		nodeIDs[id] = true
	}

	// 모든 엣지 source/target 가 존재하는 노드를 가리켜야 한다
	for _, e := range out["edges"].([]any) {
		edge := e.(map[string]any)
		assert.True(t, nodeIDs[edge["source"].(string)], "source 가 존재하는 노드를 가리켜야 한다")
		assert.True(t, nodeIDs[edge["target"].(string)], "target 가 존재하는 노드를 가리켜야 한다")
	}
}

func TestRegenerateDefinitionIDs_DoesNotMutateInput(t *testing.T) {
	t.Parallel()

	def := makeReactFlowDef()
	_ = RegenerateDefinitionIDs(def)

	// 원본 정의의 id 는 보존되어야 한다 (복사본 반환)
	assert.Equal(t, "node-1", def["nodes"].([]any)[0].(map[string]any)["id"])
	assert.Equal(t, "node-2", def["nodes"].([]any)[1].(map[string]any)["id"])
	assert.Equal(t, "edge-1", def["edges"].([]any)[0].(map[string]any)["id"])
	assert.Equal(t, "node-1", def["edges"].([]any)[0].(map[string]any)["source"])
}

func TestRegenerateDefinitionIDs_XFlowWiresFormat(t *testing.T) {
	t.Parallel()

	// XFlow 표준 wires 포맷 (source_node_id/target_node_id) 도 지원해야 한다
	def := map[string]any{
		"nodes": []any{
			map[string]any{"id": "n1", "name": "A"},
			map[string]any{"id": "n2", "name": "B"},
		},
		"wires": []any{
			map[string]any{
				"id":             "w1",
				"source_node_id": "n1",
				"target_node_id": "n2",
			},
		},
	}
	out := RegenerateDefinitionIDs(def)

	nodes := out["nodes"].([]any)
	newID1 := nodes[0].(map[string]any)["id"].(string)
	newID2 := nodes[1].(map[string]any)["id"].(string)

	wire := out["wires"].([]any)[0].(map[string]any)
	assert.Equal(t, newID1, wire["source_node_id"])
	assert.Equal(t, newID2, wire["target_node_id"])
	assert.NotEqual(t, "w1", wire["id"])
}

func TestRegenerateDefinitionIDs_NilAndEmpty(t *testing.T) {
	t.Parallel()

	assert.Nil(t, RegenerateDefinitionIDs(nil))

	// 노드/엣지 없는 정의도 안전하게 처리
	out := RegenerateDefinitionIDs(map[string]any{"name": "empty"})
	assert.Equal(t, "empty", out["name"])
}

func TestRegenerateDefinitionIDs_PreservesUnmappedEdgeRefs(t *testing.T) {
	t.Parallel()

	// source 가 존재하지 않는 노드를 가리키는 경우(비정상 입력): 매핑 불가 시
	// 원본 참조를 보존한다(임의 변경 금지). edge id 는 그래도 새로 발급한다.
	def := map[string]any{
		"nodes": []any{
			map[string]any{"id": "node-1"},
		},
		"edges": []any{
			map[string]any{"id": "edge-1", "source": "node-1", "target": "ghost"},
		},
	}
	out := RegenerateDefinitionIDs(def)

	newID := out["nodes"].([]any)[0].(map[string]any)["id"].(string)
	edge := out["edges"].([]any)[0].(map[string]any)
	assert.Equal(t, newID, edge["source"])
	assert.Equal(t, "ghost", edge["target"], "매핑 불가 참조는 보존")
}
