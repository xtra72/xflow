package flow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBoundary_상수규약 은 경계 센티넬 ID 상수가 규약대로 정의되었는지 확인한다.
func TestBoundary_상수규약(t *testing.T) {
	assert.Equal(t, "__flow_input__", FlowInputBoundaryID)
	assert.Equal(t, "__flow_output__", FlowOutputBoundaryID)
}

// TestBoundary_IsBoundaryWire 는 경계 와이어 판별 규약을 검증한다.
func TestBoundary_IsBoundaryWire(t *testing.T) {
	// 입력 경계: 플로우 입력 포트 → 내부 노드
	inWire := NewWire(FlowInputBoundaryID, "in1", "node-a", "in")
	assert.True(t, IsBoundaryWire(inWire))

	// 출력 경계: 내부 노드 → 플로우 출력 포트
	outWire := NewWire("node-b", "out", FlowOutputBoundaryID, "out1")
	assert.True(t, IsBoundaryWire(outWire))

	// 일반 와이어: 두 내부 노드 연결
	normalWire := NewWire("node-a", "out", "node-b", "in")
	assert.False(t, IsBoundaryWire(normalWire))
}

// TestBoundary_StripBoundaryWires_경계만제거 는 경계 와이어만 제거하고 일반 와이어와
// 그 외 모든 필드(노드/포트/메타데이터/ID)를 보존하는지 확인한다(REQ-SUBFLOW-F01).
func TestBoundary_StripBoundaryWires_경계만제거(t *testing.T) {
	inWire := NewWire(FlowInputBoundaryID, "in1", "node-a", "in")
	normalWire := NewWire("node-a", "out", "node-b", "in")
	outWire := NewWire("node-b", "out", FlowOutputBoundaryID, "out1")

	f := NewFlow("boundary-flow",
		WithNodes(
			NodeDef{ID: "node-a", Type: "transform"},
			NodeDef{ID: "node-b", Type: "transform"},
		),
		WithWires(inWire, normalWire, outWire),
		WithFlowInputPorts(Port{ID: "p-in", Name: "in1"}),
		WithFlowOutputPorts(Port{ID: "p-out", Name: "out1"}),
	)

	stripped := StripBoundaryWires(f)

	// 경계 와이어 2개는 제거되고 일반 와이어 1개만 남아야 한다.
	wires := stripped.Wires()
	require.Len(t, wires, 1)
	assert.Equal(t, normalWire.ID, wires[0].ID)

	// 노드/포트/ID 는 보존되어야 한다.
	assert.Equal(t, f.ID(), stripped.ID())
	assert.Len(t, stripped.Nodes(), 2)
	assert.Len(t, stripped.Inputs(), 1)
	assert.Len(t, stripped.Outputs(), 1)

	// 원본은 변경되지 않아야 한다(경계 와이어 3개 모두 보존).
	assert.Len(t, f.Wires(), 3)
}

// TestBoundary_StripBoundaryWires_경계없음_무변경 은 경계 와이어가 없으면 모든 와이어가
// 그대로 보존되는지 확인한다.
func TestBoundary_StripBoundaryWires_경계없음_무변경(t *testing.T) {
	w1 := NewWire("node-a", "out", "node-b", "in")
	w2 := NewWire("node-b", "out", "node-c", "in")

	f := NewFlow("plain-flow",
		WithNodes(
			NodeDef{ID: "node-a", Type: "transform"},
			NodeDef{ID: "node-b", Type: "transform"},
			NodeDef{ID: "node-c", Type: "transform"},
		),
		WithWires(w1, w2),
	)

	stripped := StripBoundaryWires(f)
	assert.Len(t, stripped.Wires(), 2)
}
