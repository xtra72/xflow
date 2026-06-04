package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/pkg/flow"
)

// ===========================================================================
// SPEC-SUBFLOW-001 Milestone 1: 어댑터 플로우 레벨 포트 변환 테스트
//
// 정의 최상위 inputs/outputs 배열 ↔ Flow.Inputs()/Outputs() 양방향 변환과
// 저장→로드 round-trip, export 보존, 하위호환을 검증한다(그룹 A 백엔드/영속).
// ===========================================================================

// REQ-SUBFLOW-A01/A07: 정의 최상위 inputs/outputs 가 Flow 에 올바른 방향/id 로 파싱된다.
// id 가 누락된 포트는 UUID 가 생성되어야 한다.
func TestFlowFromDefinition_ParsesFlowPorts(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	definition := map[string]any{
		"name":  "ports-flow",
		"nodes": []any{},
		"wires": []any{},
		"inputs": []any{
			map[string]any{"id": "in-1", "name": "in1"},
			map[string]any{"name": "in2"}, // id 누락 → 생성 기대
		},
		"outputs": []any{
			map[string]any{"id": "out-1", "name": "out1"},
		},
	}

	f, err := adapter.flowFromDefinition("ports-flow", "", definition)
	require.NoError(t, err)

	in := f.Inputs()
	require.Len(t, in, 2)
	assert.Equal(t, "in-1", in[0].ID)
	assert.Equal(t, "in1", in[0].Name)
	assert.Equal(t, flow.PortInput, in[0].Direction)
	assert.Equal(t, "in2", in[1].Name)
	assert.NotEmpty(t, in[1].ID, "id 누락 포트는 UUID 가 생성되어야 한다")

	out := f.Outputs()
	require.Len(t, out, 1)
	assert.Equal(t, "out-1", out[0].ID)
	assert.Equal(t, flow.PortOutput, out[0].Direction)
}

// REQ-SUBFLOW-F02: inputs/outputs 가 없는 기존 정의는 빈 슬라이스로 파싱된다(회귀 0).
func TestFlowFromDefinition_NoPorts_BackwardCompat(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	definition := map[string]any{
		"name":  "legacy-flow",
		"nodes": []any{},
		"wires": []any{},
	}

	f, err := adapter.flowFromDefinition("legacy-flow", "", definition)
	require.NoError(t, err)
	assert.Empty(t, f.Inputs())
	assert.Empty(t, f.Outputs())
}

// REQ-SUBFLOW-A07: flowToReactFlowConfig 가 inputs/outputs 를 정의 최상위로 다시 방출한다.
func TestFlowToReactFlowConfig_EmitsFlowPorts(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	f := flow.NewFlow("emit-flow",
		flow.WithFlowInputPorts(flow.Port{ID: "in-1", Name: "in1"}),
		flow.WithFlowOutputPorts(flow.Port{ID: "out-1", Name: "out1"}),
	)

	cfg := adapter.flowToReactFlowConfig(f)

	inputs, ok := cfg["inputs"].([]map[string]any)
	require.True(t, ok, "config 최상위에 inputs 가 []map[string]any 로 존재해야 함")
	require.Len(t, inputs, 1)
	assert.Equal(t, "in-1", inputs[0]["id"])
	assert.Equal(t, "in1", inputs[0]["name"])

	outputs, ok := cfg["outputs"].([]map[string]any)
	require.True(t, ok, "config 최상위에 outputs 가 존재해야 함")
	require.Len(t, outputs, 1)
	assert.Equal(t, "out-1", outputs[0]["id"])
}

// flowToReactFlowConfig: 포트가 없으면 inputs/outputs 는 빈 배열로 방출된다.
func TestFlowToReactFlowConfig_NoPorts_EmptyArrays(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	f := flow.NewFlow("noport-flow")
	cfg := adapter.flowToReactFlowConfig(f)

	inputs, ok := cfg["inputs"].([]map[string]any)
	require.True(t, ok, "inputs 키는 항상 존재해야 함")
	assert.Len(t, inputs, 0)
	outputs, ok := cfg["outputs"].([]map[string]any)
	require.True(t, ok, "outputs 키는 항상 존재해야 함")
	assert.Len(t, outputs, 0)
}

// REQ-SUBFLOW-A05: 정의 → Flow → 정의(config) 전체 round-trip 에서 포트가 보존된다.
func TestFlowPort_FullRoundTrip(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	definition := map[string]any{
		"name":  "rt-flow",
		"nodes": []any{},
		"wires": []any{},
		"inputs": []any{
			map[string]any{"id": "in-1", "name": "in1"},
		},
		"outputs": []any{
			map[string]any{"id": "out-1", "name": "out1"},
			map[string]any{"id": "out-2", "name": "out2"},
		},
	}

	f, err := adapter.flowFromDefinition("rt-flow", "", definition)
	require.NoError(t, err)

	cfg := adapter.flowToReactFlowConfig(f)

	// config 를 다시 정의로 사용해 두 번째 Flow 를 만든다.
	roundTripDef := map[string]any{
		"name":    "rt-flow",
		"nodes":   cfg["nodes"],
		"edges":   cfg["edges"],
		"inputs":  toAnySlice(cfg["inputs"].([]map[string]any)),
		"outputs": toAnySlice(cfg["outputs"].([]map[string]any)),
	}
	f2, err := adapter.flowFromDefinition("rt-flow", "", roundTripDef)
	require.NoError(t, err)

	require.Len(t, f2.Inputs(), 1)
	assert.Equal(t, "in-1", f2.Inputs()[0].ID)
	assert.Equal(t, "in1", f2.Inputs()[0].Name)
	assert.Equal(t, flow.PortInput, f2.Inputs()[0].Direction)

	require.Len(t, f2.Outputs(), 2)
	assert.Equal(t, "out-2", f2.Outputs()[1].ID)
	assert.Equal(t, flow.PortOutput, f2.Outputs()[1].Direction)
}

// REQ-SUBFLOW-A05/A06: CreateFlow → GetFlow 저장소 round-trip 에서 포트가 보존된다.
// (export 경로 동일 — flowToReactFlowConfig 가 export 와 동일 직렬화를 수행하므로 보존됨)
func TestCreateFlow_GetFlow_PreservesPorts(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)
	ctx := context.Background()

	created, err := adapter.CreateFlow(ctx, &dto.FlowCreateRequest{
		Name: "persist-flow",
		Definition: map[string]any{
			"name":  "persist-flow",
			"nodes": []any{},
			"wires": []any{},
			"inputs": []any{
				map[string]any{"id": "in-1", "name": "in1"},
			},
			"outputs": []any{
				map[string]any{"id": "out-1", "name": "out1"},
			},
		},
	})
	require.NoError(t, err)

	// 생성 응답 config 에 포트가 포함되어야 한다.
	createdInputs, ok := created.Config["inputs"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, createdInputs, 1)
	assert.Equal(t, "in-1", createdInputs[0]["id"])

	// 저장소 재조회 후에도 포트가 보존되어야 한다(REQ-SUBFLOW-A05).
	reread, err := adapter.GetFlow(ctx, created.ID)
	require.NoError(t, err)
	rereadInputs, ok := reread.Config["inputs"].([]map[string]any)
	require.True(t, ok, "재조회 config 에 inputs 가 존재해야 함")
	require.Len(t, rereadInputs, 1)
	assert.Equal(t, "in-1", rereadInputs[0]["id"])
	assert.Equal(t, "in1", rereadInputs[0]["name"])

	rereadOutputs, ok := reread.Config["outputs"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, rereadOutputs, 1)
	assert.Equal(t, "out-1", rereadOutputs[0]["id"])
}

// toAnySlice 는 []map[string]any 를 []any 로 변환하는 테스트 헬퍼이다.
func toAnySlice(in []map[string]any) []any {
	out := make([]any, len(in))
	for i, m := range in {
		out[i] = m
	}
	return out
}
