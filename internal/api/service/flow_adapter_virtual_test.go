package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/pkg/flow"
)

// TestConvertReactFlowEdgesToWires_VirtualAndName 는 edge→wire 변환에서
// virtual(bool) 과 name(string) 이 그대로 보존되는지 검증한다
// (SPEC-LINK-001 REQ-LINK-002, AC-B2).
func TestConvertReactFlowEdgesToWires_VirtualAndName(t *testing.T) {
	def := map[string]any{
		"edges": []any{
			map[string]any{
				"id":           "e1",
				"source":       "a",
				"target":       "b",
				"sourceHandle": "out",
				"targetHandle": "in",
				"name":         "sensor",
				"virtual":      true,
			},
		},
	}

	convertReactFlowEdgesToWires(def)

	wiresRaw, ok := def["wires"].([]any)
	require.True(t, ok, "wires 가 생성되어야 한다")
	require.Len(t, wiresRaw, 1)

	w0 := wiresRaw[0].(map[string]any)
	assert.Equal(t, true, w0["virtual"], "virtual 값이 보존되어야 한다")
	assert.Equal(t, "sensor", w0["name"], "name 값이 보존되어야 한다")
}

// TestConvertReactFlowEdgesToWires_VirtualAbsentDefaultsFalse 는 edge 에 virtual
// 키가 없으면 변환 결과에도 virtual 키가 강제로 들어가지 않고(기본 false 해석)
// name 은 정상 통과되는지 검증한다 (REQ-LINK-002, REQ-LINK-031).
func TestConvertReactFlowEdgesToWires_VirtualAbsentDefaultsFalse(t *testing.T) {
	def := map[string]any{
		"edges": []any{
			map[string]any{
				"id":           "e1",
				"source":       "a",
				"target":       "b",
				"sourceHandle": "out",
				"targetHandle": "in",
				"name":         "plain",
			},
		},
	}

	convertReactFlowEdgesToWires(def)

	wiresRaw, ok := def["wires"].([]any)
	require.True(t, ok)
	require.Len(t, wiresRaw, 1)

	w0 := wiresRaw[0].(map[string]any)
	// virtual 키는 명시되지 않았으므로 변환 결과에도 없어야 한다(역직렬화 시 기본 false).
	_, hasVirtual := w0["virtual"]
	assert.False(t, hasVirtual, "edge 에 virtual 이 없으면 wire 에도 virtual 키가 없어야 한다")
	assert.Equal(t, "plain", w0["name"])
}

// TestConvertReactFlowEdgesToWires_VirtualDoesNotChangeModePolicy 는 virtual
// 통과가 기존 mode/buffer_size 정책에 영향을 주지 않는지 회귀 검증한다
// (REQ-LINK-002, AC-B2).
func TestConvertReactFlowEdgesToWires_VirtualDoesNotChangeModePolicy(t *testing.T) {
	def := map[string]any{
		"edges": []any{
			map[string]any{
				"id":           "e1",
				"source":       "a",
				"target":       "b",
				"sourceHandle": "out",
				"targetHandle": "in",
				"virtual":      true,
			},
		},
	}

	convertReactFlowEdgesToWires(def)

	w0 := def["wires"].([]any)[0].(map[string]any)
	// buffer_size 키가 없었으므로 기본 큐 정책(buffer_size=100, mode="buffer")이 유지되어야 한다.
	assert.Equal(t, 100, w0["buffer_size"], "기본 큐 buffer_size 정책 유지")
	assert.Equal(t, "buffer", w0["mode"], "기본 큐 mode 정책 유지")
	assert.Equal(t, true, w0["virtual"], "virtual 도 함께 보존")
}

// TestFlowToReactFlowConfig_VirtualAndNamePreserved 는 역방향(wire→edge) 직렬화에서
// virtual/name 이 React Flow edge 메타로 보존되는지 검증한다(REQ-LINK-003, AC-04 저장-로드).
func TestFlowToReactFlowConfig_VirtualAndNamePreserved(t *testing.T) {
	adapter := NewFlowServiceAdapter(newTestEngine(), newTestRepo(t), nil)

	wire := flow.NewWire("a", "out", "b", "in", flow.WithWireVirtual(true))
	wire.Name = "sensor"

	f := flow.NewFlow("test-flow",
		flow.WithNodes(
			flow.NewNodeDef("a", "trigger"),
			flow.NewNodeDef("b", "output"),
		),
		flow.WithWires(wire),
	)

	result := adapter.flowToReactFlowConfig(f)

	edgesRaw, ok := result["edges"].([]map[string]any)
	require.True(t, ok, "edges 가 생성되어야 한다")
	require.Len(t, edgesRaw, 1)

	e0 := edgesRaw[0]
	assert.Equal(t, true, e0["virtual"], "역방향 직렬화에서 virtual 이 보존되어야 한다")
	assert.Equal(t, "sensor", e0["name"], "역방향 직렬화에서 name 이 보존되어야 한다")
}

// TestFlowToReactFlowConfig_VirtualFalsePreserved 는 virtual=false 와이어가
// edge 메타에 virtual:false 로 직렬화되는지(하위 호환) 검증한다(REQ-LINK-025).
func TestFlowToReactFlowConfig_VirtualFalsePreserved(t *testing.T) {
	adapter := NewFlowServiceAdapter(newTestEngine(), newTestRepo(t), nil)

	wire := flow.NewWire("a", "out", "b", "in") // 기본 Virtual=false

	f := flow.NewFlow("test-flow",
		flow.WithNodes(
			flow.NewNodeDef("a", "trigger"),
			flow.NewNodeDef("b", "output"),
		),
		flow.WithWires(wire),
	)

	result := adapter.flowToReactFlowConfig(f)

	e0 := result["edges"].([]map[string]any)[0]
	assert.Equal(t, false, e0["virtual"], "virtual=false 와이어는 virtual:false 로 직렬화되어야 한다")
}

// TestVirtualRoundTrip_EdgeToWireToEdge 는 edge→wire→edge 전체 라운드트립에서
// virtual/name 이 보존되는지 검증한다(REQ-LINK-003/004, AC-04).
func TestVirtualRoundTrip_EdgeToWireToEdge(t *testing.T) {
	adapter := NewFlowServiceAdapter(newTestEngine(), newTestRepo(t), nil)

	// 1) 프론트 정의(React Flow) → 정규화(wire 변환).
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
				"name":         "sensor",
				"virtual":      true,
			},
		},
	}

	// 2) 정의 → flow.Flow (저장 단계 직렬화 경로).
	f, err := adapter.flowFromDefinition("test-flow", "", def)
	require.NoError(t, err)

	wires := f.Wires()
	require.Len(t, wires, 1)
	assert.True(t, wires[0].Virtual, "flow.Wire 로 변환 후 Virtual=true 보존")
	assert.Equal(t, "sensor", wires[0].Name, "flow.Wire 로 변환 후 Name 보존")

	// 3) flow.Flow → React Flow (로드 단계 역직렬화 경로).
	result := adapter.flowToReactFlowConfig(f)
	e0 := result["edges"].([]map[string]any)[0]
	assert.Equal(t, true, e0["virtual"], "라운드트립 후 edge virtual 보존")
	assert.Equal(t, "sensor", e0["name"], "라운드트립 후 edge name 보존")
}
