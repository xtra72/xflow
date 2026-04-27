package flow

import (
	"testing"
)

// ---------------------------------------------------------------------------
// helper: 유효한 2-노드 Flow 생성 (WithNodes/WithWires 사용)
// ---------------------------------------------------------------------------

// makeValidFlow 는 nodeA(out) → nodeB(in) 연결이 있는 유효한 Flow를 반환한다.
func makeValidFlow(t *testing.T) (Flow, NodeDef, NodeDef) {
	t.Helper()

	nodeA := NewNodeDef("nodeA", "filter")
	nodeB := NewNodeDef("nodeB", "transform")
	wire := NewWire(nodeA.ID, "out", nodeB.ID, "in")

	f := NewFlow("valid-flow", WithNodes(nodeA, nodeB), WithWires(wire))
	return f, nodeA, nodeB
}

// findByCode 는 에러 목록에서 특정 코드를 가진 ValidationError를 찾는다.
func findByCode(errs []ValidationError, code string) *ValidationError {
	for i := range errs {
		if errs[i].Code == code {
			return &errs[i]
		}
	}
	return nil
}

// countBySeverity 는 에러 목록에서 특정 심각도의 에러 개수를 반환한다.
func countBySeverity(errs []ValidationError, sev ValidationSeverity) int {
	count := 0
	for _, e := range errs {
		if e.Severity == sev {
			count++
		}
	}
	return count
}

// ===========================================================================
// AC-30: 유효한 Flow는 빈 슬라이스를 반환한다
// ===========================================================================
func TestValidate_AC30_ValidFlowReturnsEmpty(t *testing.T) {
	f, _, _ := makeValidFlow(t)

	errs := Validate(f)

	if len(errs) != 0 {
		t.Errorf("유효한 Flow에서 에러 %d개 발생, want 0", len(errs))
		for _, e := range errs {
			t.Logf("  %s", e.Error())
		}
	}
}

// ===========================================================================
// AC-31: Wire orphan source
// ===========================================================================
func TestValidate_AC31_WireOrphanSource(t *testing.T) {
	node := NewNodeDef("nodeB", "filter")
	wire := NewWire("nonexistent-id", "out", node.ID, "in")
	f := NewFlow("test", WithNodes(node), WithWires(wire))

	errs := Validate(f)

	ve := findByCode(errs, "WIRE_ORPHAN_SOURCE")
	if ve == nil {
		t.Fatal("WIRE_ORPHAN_SOURCE 에러가 반환되지 않았다")
	}
	if ve.Severity != SeverityError {
		t.Errorf("Severity = %q, want %q", ve.Severity, SeverityError)
	}
	if ve.Path != "wires[0].source_node_id" {
		t.Errorf("Path = %q, want %q", ve.Path, "wires[0].source_node_id")
	}
}

// ===========================================================================
// AC-32: Wire orphan target
// ===========================================================================
func TestValidate_AC32_WireOrphanTarget(t *testing.T) {
	node := NewNodeDef("nodeA", "filter")
	wire := NewWire(node.ID, "out", "nonexistent-id", "in")
	f := NewFlow("test", WithNodes(node), WithWires(wire))

	errs := Validate(f)

	ve := findByCode(errs, "WIRE_ORPHAN_TARGET")
	if ve == nil {
		t.Fatal("WIRE_ORPHAN_TARGET 에러가 반환되지 않았다")
	}
	if ve.Severity != SeverityError {
		t.Errorf("Severity = %q, want %q", ve.Severity, SeverityError)
	}
	if ve.Path != "wires[0].target_node_id" {
		t.Errorf("Path = %q, want %q", ve.Path, "wires[0].target_node_id")
	}
}

// ===========================================================================
// AC-33: Duplicate wire
// ===========================================================================
func TestValidate_AC33_DuplicateWire(t *testing.T) {
	nodeA := NewNodeDef("nodeA", "filter")
	nodeB := NewNodeDef("nodeB", "transform")
	wire1 := NewWire(nodeA.ID, "out", nodeB.ID, "in")
	wire2 := NewWire(nodeA.ID, "out", nodeB.ID, "in")

	f := NewFlow("test", WithNodes(nodeA, nodeB), WithWires(wire1, wire2))

	errs := Validate(f)

	ve := findByCode(errs, "WIRE_DUPLICATE")
	if ve == nil {
		t.Fatal("WIRE_DUPLICATE 에러가 반환되지 않았다")
	}
	if ve.Severity != SeverityError {
		t.Errorf("Severity = %q, want %q", ve.Severity, SeverityError)
	}
}

// ===========================================================================
// AC-34: Self-reference wire (Warning)
// ===========================================================================
func TestValidate_AC34_SelfReferenceWire(t *testing.T) {
	nodeA := NewNodeDef("nodeA", "filter")
	wire := NewWire(nodeA.ID, "out", nodeA.ID, "in")

	f := NewFlow("test", WithNodes(nodeA), WithWires(wire))

	errs := Validate(f)

	ve := findByCode(errs, "WIRE_SELF_REFERENCE")
	if ve == nil {
		t.Fatal("WIRE_SELF_REFERENCE 에러가 반환되지 않았다")
	}
	if ve.Severity != SeverityWarning {
		t.Errorf("Severity = %q, want %q", ve.Severity, SeverityWarning)
	}
}

// ===========================================================================
// AC-35: Node ID/Name duplicates
// ===========================================================================
func TestValidate_AC35_DuplicateNodeID(t *testing.T) {
	node1 := NewNodeDef("nodeA", "filter")
	node2 := NewNodeDef("nodeB", "transform")

	// node2의 ID를 node1의 ID와 동일하게 설정
	node2.ID = node1.ID

	f := NewFlow("test", WithNodes(node1, node2))

	errs := Validate(f)

	ve := findByCode(errs, "NODE_DUPLICATE_ID")
	if ve == nil {
		t.Fatal("NODE_DUPLICATE_ID 에러가 반환되지 않았다")
	}
	if ve.Severity != SeverityError {
		t.Errorf("Severity = %q, want %q", ve.Severity, SeverityError)
	}
}

func TestValidate_AC35_DuplicateNodeName(t *testing.T) {
	node1 := NewNodeDef("sameName", "filter")
	node2 := NewNodeDef("sameName", "transform")

	f := NewFlow("test", WithNodes(node1, node2))

	errs := Validate(f)

	ve := findByCode(errs, "NODE_DUPLICATE_NAME")
	if ve == nil {
		t.Fatal("NODE_DUPLICATE_NAME 경고가 반환되지 않았다")
	}
	if ve.Severity != SeverityWarning {
		t.Errorf("Severity = %q, want %q", ve.Severity, SeverityWarning)
	}
}

// ===========================================================================
// AC-36: Bridge node without AgentRef
// ===========================================================================
func TestValidate_AC36_BridgeNodeWithoutAgentRef(t *testing.T) {
	node := NewNodeDef("bridgeNode", "bridge")
	// AgentRef는 nil (기본값)

	f := NewFlow("test", WithNodes(node))

	errs := Validate(f)

	ve := findByCode(errs, "NODE_BRIDGE_NO_AGENT")
	if ve == nil {
		t.Fatal("NODE_BRIDGE_NO_AGENT 에러가 반환되지 않았다")
	}
	if ve.Severity != SeverityError {
		t.Errorf("Severity = %q, want %q", ve.Severity, SeverityError)
	}
}

func TestValidate_BridgeNodeWithAgentRef_NoError(t *testing.T) {
	node := NewNodeDef("bridgeNode", "bridge", WithAgentRef(AgentRef{
		AgentID:   "agent-1",
		AgentName: "test-agent",
		Direction: BridgeIn,
	}))

	// 노드가 연결되지 않아 NODE_DISCONNECTED Warning은 발생할 수 있다.
	// 그러나 NODE_BRIDGE_NO_AGENT는 발생하면 안 된다.
	f := NewFlow("test", WithNodes(node))

	errs := Validate(f)

	ve := findByCode(errs, "NODE_BRIDGE_NO_AGENT")
	if ve != nil {
		t.Error("AgentRef가 있는 bridge 노드에서 NODE_BRIDGE_NO_AGENT가 반환되었다")
	}
}

// ===========================================================================
// AC-37: Buffer mode invalid size
// ===========================================================================
func TestValidate_AC37_BufferInvalidSize(t *testing.T) {
	nodeA := NewNodeDef("nodeA", "filter")
	nodeB := NewNodeDef("nodeB", "transform")
	wire := NewWire(nodeA.ID, "out", nodeB.ID, "in",
		WithWireMode(WireBuffer),
		WithBufferSize(0),
	)

	f := NewFlow("test", WithNodes(nodeA, nodeB), WithWires(wire))

	errs := Validate(f)

	ve := findByCode(errs, "WIRE_BUFFER_INVALID_SIZE")
	if ve == nil {
		t.Fatal("WIRE_BUFFER_INVALID_SIZE 에러가 반환되지 않았다")
	}
	if ve.Severity != SeverityError {
		t.Errorf("Severity = %q, want %q", ve.Severity, SeverityError)
	}
}

func TestValidate_BufferValidSize_NoError(t *testing.T) {
	nodeA := NewNodeDef("nodeA", "filter")
	nodeB := NewNodeDef("nodeB", "transform")
	wire := NewWire(nodeA.ID, "out", nodeB.ID, "in",
		WithWireMode(WireBuffer),
		WithBufferSize(10),
	)

	f := NewFlow("test", WithNodes(nodeA, nodeB), WithWires(wire))

	errs := Validate(f)

	ve := findByCode(errs, "WIRE_BUFFER_INVALID_SIZE")
	if ve != nil {
		t.Error("유효한 BufferSize에서 WIRE_BUFFER_INVALID_SIZE가 반환되었다")
	}
}

func TestValidate_BufferNegativeSize(t *testing.T) {
	nodeA := NewNodeDef("nodeA", "filter")
	nodeB := NewNodeDef("nodeB", "transform")
	wire := NewWire(nodeA.ID, "out", nodeB.ID, "in",
		WithWireMode(WireBuffer),
		WithBufferSize(-5),
	)

	f := NewFlow("test", WithNodes(nodeA, nodeB), WithWires(wire))

	errs := Validate(f)

	ve := findByCode(errs, "WIRE_BUFFER_INVALID_SIZE")
	if ve == nil {
		t.Fatal("음수 BufferSize에서 WIRE_BUFFER_INVALID_SIZE가 반환되지 않았다")
	}
}

// ===========================================================================
// AC-38: Disconnected node warning
// ===========================================================================
func TestValidate_AC38_DisconnectedNode(t *testing.T) {
	nodeA := NewNodeDef("nodeA", "filter")
	nodeB := NewNodeDef("nodeB", "transform")
	nodeC := NewNodeDef("nodeC", "logger")

	wire := NewWire(nodeA.ID, "out", nodeB.ID, "in")

	f := NewFlow("test", WithNodes(nodeA, nodeB, nodeC), WithWires(wire))

	errs := Validate(f)

	ve := findByCode(errs, "NODE_DISCONNECTED")
	if ve == nil {
		t.Fatal("NODE_DISCONNECTED 경고가 반환되지 않았다")
	}
	if ve.Severity != SeverityWarning {
		t.Errorf("Severity = %q, want %q", ve.Severity, SeverityWarning)
	}
}

// ===========================================================================
// AC-39: Multiple errors compound test
// ===========================================================================
func TestValidate_AC39_MultipleErrorsCompound(t *testing.T) {
	// 1) 중복 노드 ID
	node1 := NewNodeDef("nodeA", "filter")
	node2 := NewNodeDef("nodeB", "transform")
	node2.ID = node1.ID // 중복 ID

	// 2) orphan source wire
	node3 := NewNodeDef("nodeC", "logger")
	orphanWire := NewWire("nonexistent", "out", node3.ID, "in")

	// 3) nodeC는 orphanWire의 타겟이므로 연결됨,
	//    node1과 node2는 와이어 없음 → disconnected (Warning)

	f := NewFlow("test",
		WithNodes(node1, node2, node3),
		WithWires(orphanWire),
	)

	errs := Validate(f)

	// 최소 3개 이상의 에러
	if len(errs) < 3 {
		t.Errorf("에러 %d개 반환, 최소 3개 이상 필요", len(errs))
		for _, e := range errs {
			t.Logf("  %s", e.Error())
		}
	}

	// Error 심각도 최소 2개
	errorCount := countBySeverity(errs, SeverityError)
	if errorCount < 2 {
		t.Errorf("SeverityError 개수 = %d, 최소 2개 필요", errorCount)
	}

	// Warning 심각도 최소 1개
	warningCount := countBySeverity(errs, SeverityWarning)
	if warningCount < 1 {
		t.Errorf("SeverityWarning 개수 = %d, 최소 1개 필요", warningCount)
	}
}

// ===========================================================================
// 추가 테스트: Wire invalid source port
// ===========================================================================
func TestValidate_WireInvalidSourcePort(t *testing.T) {
	nodeA := NewNodeDef("nodeA", "filter")
	nodeB := NewNodeDef("nodeB", "transform")
	wire := NewWire(nodeA.ID, "nonexistent-port", nodeB.ID, "in")

	f := NewFlow("test", WithNodes(nodeA, nodeB), WithWires(wire))

	errs := Validate(f)

	ve := findByCode(errs, "WIRE_INVALID_SOURCE_PORT")
	if ve == nil {
		t.Fatal("WIRE_INVALID_SOURCE_PORT 에러가 반환되지 않았다")
	}
	if ve.Severity != SeverityError {
		t.Errorf("Severity = %q, want %q", ve.Severity, SeverityError)
	}
	if ve.Path != "wires[0].source_port" {
		t.Errorf("Path = %q, want %q", ve.Path, "wires[0].source_port")
	}
}

// ===========================================================================
// 추가 테스트: Wire invalid target port
// ===========================================================================
func TestValidate_WireInvalidTargetPort(t *testing.T) {
	nodeA := NewNodeDef("nodeA", "filter")
	nodeB := NewNodeDef("nodeB", "transform")
	wire := NewWire(nodeA.ID, "out", nodeB.ID, "nonexistent-port")

	f := NewFlow("test", WithNodes(nodeA, nodeB), WithWires(wire))

	errs := Validate(f)

	ve := findByCode(errs, "WIRE_INVALID_TARGET_PORT")
	if ve == nil {
		t.Fatal("WIRE_INVALID_TARGET_PORT 에러가 반환되지 않았다")
	}
	if ve.Severity != SeverityError {
		t.Errorf("Severity = %q, want %q", ve.Severity, SeverityError)
	}
	if ve.Path != "wires[0].target_port" {
		t.Errorf("Path = %q, want %q", ve.Path, "wires[0].target_port")
	}
}

// ===========================================================================
// 추가 테스트: Errors 포트를 통한 유효한 소스 포트 확인
// ===========================================================================
func TestValidate_WireFromErrorPort_NoError(t *testing.T) {
	nodeA := NewNodeDef("nodeA", "filter", WithErrorPort())
	nodeB := NewNodeDef("nodeB", "transform")
	wire := NewWire(nodeA.ID, "error", nodeB.ID, "in")

	f := NewFlow("test", WithNodes(nodeA, nodeB), WithWires(wire))

	errs := Validate(f)

	ve := findByCode(errs, "WIRE_INVALID_SOURCE_PORT")
	if ve != nil {
		t.Error("Errors 포트에서 나가는 유효한 Wire에서 WIRE_INVALID_SOURCE_PORT가 반환되었다")
	}
}

// ===========================================================================
// 추가 테스트: ValidationError.Error() 메서드 포맷 확인
// ===========================================================================
func TestValidationError_ErrorMethod(t *testing.T) {
	ve := ValidationError{
		Code:     "WIRE_ORPHAN_SOURCE",
		Severity: SeverityError,
		Message:  "source node not found",
		Path:     "wires[0].source_node_id",
	}

	got := ve.Error()
	want := "[error] WIRE_ORPHAN_SOURCE: source node not found (at wires[0].source_node_id)"

	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// ===========================================================================
// 추가 테스트: ValidationSeverity 상수 값 확인
// ===========================================================================
func TestValidationSeverity_Constants(t *testing.T) {
	if SeverityError != "error" {
		t.Errorf("SeverityError = %q, want %q", SeverityError, "error")
	}
	if SeverityWarning != "warning" {
		t.Errorf("SeverityWarning = %q, want %q", SeverityWarning, "warning")
	}
}
