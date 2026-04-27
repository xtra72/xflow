package flow

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// helper: 테스트용 노드 두 개(nodeA, nodeB)와 유효한 Wire 생성
// ---------------------------------------------------------------------------
func setupTwoNodesAndWire(t *testing.T) (Flow, NodeDef, NodeDef, Wire) {
	t.Helper()

	nodeA := NewNodeDef("nodeA", "filter")
	nodeB := NewNodeDef("nodeB", "transform")

	f := NewFlow("test-flow")

	if err := f.AddNode(nodeA); err != nil {
		t.Fatalf("AddNode(nodeA) 실패: %v", err)
	}
	if err := f.AddNode(nodeB); err != nil {
		t.Fatalf("AddNode(nodeB) 실패: %v", err)
	}

	// nodeA.Outputs[0].Name == "out", nodeB.Inputs[0].Name == "in"
	wire := NewWire(nodeA.ID, "out", nodeB.ID, "in")

	return f, nodeA, nodeB, wire
}

// ===========================================================================
// AC-01: NewFlow 기본 생성
// ===========================================================================
func TestNewFlow_Basic(t *testing.T) {
	before := time.Now()
	f := NewFlow("sensor-pipeline")
	after := time.Now()

	// ID는 유효한 UUID
	if !isValidUUID(f.ID()) {
		t.Errorf("ID가 유효한 UUID가 아닙니다: %q", f.ID())
	}

	// Name
	if f.Name() != "sensor-pipeline" {
		t.Errorf("Name = %q, want %q", f.Name(), "sensor-pipeline")
	}

	// State 초기값
	if f.State() != FlowStored {
		t.Errorf("State = %q, want %q", f.State(), FlowStored)
	}

	// 빈 Nodes/Wires
	if len(f.Nodes()) != 0 {
		t.Errorf("Nodes 길이 = %d, want 0", len(f.Nodes()))
	}
	if len(f.Wires()) != 0 {
		t.Errorf("Wires 길이 = %d, want 0", len(f.Wires()))
	}

	// CreatedAt이 현재 시각 근처
	if f.CreatedAt().Before(before) || f.CreatedAt().After(after) {
		t.Errorf("CreatedAt = %v, 범위 [%v, %v] 밖", f.CreatedAt(), before, after)
	}

	// UpdatedAt == CreatedAt (초기)
	if !f.UpdatedAt().Equal(f.CreatedAt()) {
		t.Error("초기 UpdatedAt은 CreatedAt과 같아야 합니다")
	}

	// Description 초기값 빈 문자열
	if f.Description() != "" {
		t.Errorf("Description = %q, want %q", f.Description(), "")
	}
}

// ===========================================================================
// AC-02: NewFlow 옵션 적용
// ===========================================================================
func TestNewFlow_WithOptions(t *testing.T) {
	f := NewFlow("test",
		WithDescription("테스트 설명"),
		WithFlowMetadata("key", "val"),
	)

	if f.Description() != "테스트 설명" {
		t.Errorf("Description = %q, want %q", f.Description(), "테스트 설명")
	}

	md := f.Metadata()
	if md["key"] != "val" {
		t.Errorf("Metadata[key] = %q, want %q", md["key"], "val")
	}
}

// ===========================================================================
// 기본 FlowConfig 값 검증
// ===========================================================================
func TestNewFlow_DefaultConfig(t *testing.T) {
	f := NewFlow("cfg-test")
	cfg := f.Config()

	if cfg.MaxHistorySize != 100 {
		t.Errorf("MaxHistorySize = %d, want 100", cfg.MaxHistorySize)
	}
	if cfg.ErrorHandling != ErrorPropagate {
		t.Errorf("ErrorHandling = %q, want %q", cfg.ErrorHandling, ErrorPropagate)
	}
	if cfg.TrackHistory != false {
		t.Error("TrackHistory 기본값은 false여야 합니다")
	}
}

// ===========================================================================
// WithFlowConfig 옵션 적용
// ===========================================================================
func TestNewFlow_WithFlowConfig(t *testing.T) {
	custom := FlowConfig{
		TrackHistory:   true,
		MaxHistorySize: 50,
		ErrorHandling:  ErrorStop,
	}
	f := NewFlow("cfg", WithFlowConfig(custom))
	cfg := f.Config()

	if cfg.TrackHistory != true {
		t.Error("TrackHistory = false, want true")
	}
	if cfg.MaxHistorySize != 50 {
		t.Errorf("MaxHistorySize = %d, want 50", cfg.MaxHistorySize)
	}
	if cfg.ErrorHandling != ErrorStop {
		t.Errorf("ErrorHandling = %q, want %q", cfg.ErrorHandling, ErrorStop)
	}
}

// ===========================================================================
// WithNodes 옵션 적용
// ===========================================================================
func TestNewFlow_WithNodes(t *testing.T) {
	n1 := NewNodeDef("n1", "filter")
	n2 := NewNodeDef("n2", "transform")

	f := NewFlow("with-nodes", WithNodes(n1, n2))

	if len(f.Nodes()) != 2 {
		t.Fatalf("Nodes 길이 = %d, want 2", len(f.Nodes()))
	}
	if f.Nodes()[0].Name != "n1" {
		t.Errorf("Nodes[0].Name = %q, want %q", f.Nodes()[0].Name, "n1")
	}
	if f.Nodes()[1].Name != "n2" {
		t.Errorf("Nodes[1].Name = %q, want %q", f.Nodes()[1].Name, "n2")
	}
}

// ===========================================================================
// WithWires 옵션 적용
// ===========================================================================
func TestNewFlow_WithWires(t *testing.T) {
	w1 := NewWire("src1", "out", "dst1", "in")
	w2 := NewWire("src2", "out", "dst2", "in")

	f := NewFlow("with-wires", WithWires(w1, w2))

	if len(f.Wires()) != 2 {
		t.Fatalf("Wires 길이 = %d, want 2", len(f.Wires()))
	}
}

// ===========================================================================
// AC-03: AddNode 성공
// ===========================================================================
func TestAddNode_Success(t *testing.T) {
	f := NewFlow("add-node")
	node := NewNodeDef("filter-1", "filter")

	err := f.AddNode(node)
	if err != nil {
		t.Fatalf("AddNode 실패: %v", err)
	}

	if len(f.Nodes()) != 1 {
		t.Fatalf("Nodes 길이 = %d, want 1", len(f.Nodes()))
	}

	// Node("filter-1") 조회 성공
	got, ok := f.Node("filter-1")
	if !ok {
		t.Fatal("Node(\"filter-1\") = false, want true")
	}
	if got.Name != "filter-1" {
		t.Errorf("Node.Name = %q, want %q", got.Name, "filter-1")
	}
}

// ===========================================================================
// AC-04: AddNode 중복 ID 에러
// ===========================================================================
func TestAddNode_DuplicateID(t *testing.T) {
	f := NewFlow("dup-id")
	node := NewNodeDef("node-a", "filter")
	_ = f.AddNode(node)

	// 같은 ID의 다른 노드 생성
	dup := NodeDef{
		ID:      node.ID,
		Name:    "different-name",
		Type:    "transform",
		Inputs:  []Port{{ID: "p1", Name: "in", Direction: PortInput}},
		Outputs: []Port{{ID: "p2", Name: "out", Direction: PortOutput}},
	}

	err := f.AddNode(dup)
	if err != ErrDuplicateNodeID {
		t.Errorf("err = %v, want ErrDuplicateNodeID", err)
	}

	// 길이 변경 없음
	if len(f.Nodes()) != 1 {
		t.Errorf("Nodes 길이 = %d, want 1", len(f.Nodes()))
	}
}

// ===========================================================================
// AC-05: AddNode 중복 Name 허용 (와이어는 ID로 연결되므로 이름 중복은 허용)
// ===========================================================================
func TestAddNode_DuplicateName(t *testing.T) {
	f := NewFlow("dup-name")
	node := NewNodeDef("same-name", "filter")
	_ = f.AddNode(node)

	// 다른 ID, 같은 Name → 허용되어야 함
	dup := NewNodeDef("same-name", "transform")

	err := f.AddNode(dup)
	if err != nil {
		t.Errorf("중복 이름은 허용되어야 하지만 에러 발생: %v", err)
	}
	if len(f.Nodes()) != 2 {
		t.Errorf("노드 수 = %d, want 2", len(f.Nodes()))
	}
}

// ===========================================================================
// AC-06: RemoveNode 연결된 Wire도 함께 제거
// ===========================================================================
func TestRemoveNode_CascadesWireRemoval(t *testing.T) {
	f, nodeA, nodeB, wire := setupTwoNodesAndWire(t)

	// Wire 추가
	if err := f.AddWire(wire); err != nil {
		t.Fatalf("AddWire 실패: %v", err)
	}
	if len(f.Wires()) != 1 {
		t.Fatalf("Wires 길이 = %d, want 1", len(f.Wires()))
	}

	// nodeA 제거 -> Wire도 제거되어야 함
	if err := f.RemoveNode(nodeA.ID); err != nil {
		t.Fatalf("RemoveNode 실패: %v", err)
	}

	if len(f.Nodes()) != 1 {
		t.Errorf("RemoveNode 후 Nodes 길이 = %d, want 1", len(f.Nodes()))
	}
	if len(f.Wires()) != 0 {
		t.Errorf("RemoveNode 후 Wires 길이 = %d, want 0 (cascade)", len(f.Wires()))
	}

	// 남은 노드 확인
	_, ok := f.Node(nodeB.Name)
	if !ok {
		t.Error("nodeB가 삭제되면 안 됩니다")
	}
}

// ===========================================================================
// RemoveNode: 존재하지 않는 노드
// ===========================================================================
func TestRemoveNode_NotFound(t *testing.T) {
	f := NewFlow("rm-notfound")

	err := f.RemoveNode("nonexistent")
	if err != ErrNodeNotFound {
		t.Errorf("err = %v, want ErrNodeNotFound", err)
	}
}

// ===========================================================================
// RemoveNode: 이름으로 제거
// ===========================================================================
func TestRemoveNode_ByName(t *testing.T) {
	f := NewFlow("rm-by-name")
	node := NewNodeDef("my-node", "filter")
	_ = f.AddNode(node)

	err := f.RemoveNode("my-node")
	if err != nil {
		t.Fatalf("RemoveNode(name) 실패: %v", err)
	}
	if len(f.Nodes()) != 0 {
		t.Errorf("Nodes 길이 = %d, want 0", len(f.Nodes()))
	}
}

// ===========================================================================
// AC-07: AddWire 소스 노드 유효하지 않음
// ===========================================================================
func TestAddWire_InvalidSourceNode(t *testing.T) {
	f := NewFlow("wire-bad-src")
	nodeB := NewNodeDef("nodeB", "transform")
	_ = f.AddNode(nodeB)

	wire := NewWire("nonexistent-node", "out", nodeB.ID, "in")

	err := f.AddWire(wire)
	if err != ErrInvalidWireSource {
		t.Errorf("err = %v, want ErrInvalidWireSource", err)
	}
}

// ===========================================================================
// AddWire 성공
// ===========================================================================
func TestAddWire_Success(t *testing.T) {
	f, _, _, wire := setupTwoNodesAndWire(t)

	err := f.AddWire(wire)
	if err != nil {
		t.Fatalf("AddWire 실패: %v", err)
	}

	if len(f.Wires()) != 1 {
		t.Fatalf("Wires 길이 = %d, want 1", len(f.Wires()))
	}
}

// ===========================================================================
// AddWire 소스 포트 유효하지 않음
// ===========================================================================
func TestAddWire_InvalidSourcePort(t *testing.T) {
	f := NewFlow("wire-bad-src-port")
	nodeA := NewNodeDef("nodeA", "filter")
	nodeB := NewNodeDef("nodeB", "transform")
	_ = f.AddNode(nodeA)
	_ = f.AddNode(nodeB)

	wire := NewWire(nodeA.ID, "nonexistent-port", nodeB.ID, "in")

	err := f.AddWire(wire)
	if err != ErrInvalidWireSource {
		t.Errorf("err = %v, want ErrInvalidWireSource", err)
	}
}

// ===========================================================================
// AddWire 타겟 노드 유효하지 않음
// ===========================================================================
func TestAddWire_InvalidTargetNode(t *testing.T) {
	f := NewFlow("wire-bad-tgt")
	nodeA := NewNodeDef("nodeA", "filter")
	_ = f.AddNode(nodeA)

	wire := NewWire(nodeA.ID, "out", "nonexistent-node", "in")

	err := f.AddWire(wire)
	if err != ErrInvalidWireTarget {
		t.Errorf("err = %v, want ErrInvalidWireTarget", err)
	}
}

// ===========================================================================
// AddWire 타겟 포트 유효하지 않음
// ===========================================================================
func TestAddWire_InvalidTargetPort(t *testing.T) {
	f := NewFlow("wire-bad-tgt-port")
	nodeA := NewNodeDef("nodeA", "filter")
	nodeB := NewNodeDef("nodeB", "transform")
	_ = f.AddNode(nodeA)
	_ = f.AddNode(nodeB)

	wire := NewWire(nodeA.ID, "out", nodeB.ID, "nonexistent-port")

	err := f.AddWire(wire)
	if err != ErrInvalidWireTarget {
		t.Errorf("err = %v, want ErrInvalidWireTarget", err)
	}
}

// ===========================================================================
// AddWire 중복 Wire ID
// ===========================================================================
func TestAddWire_DuplicateWireID(t *testing.T) {
	f, nodeA, nodeB, wire := setupTwoNodesAndWire(t)

	_ = f.AddWire(wire)

	// 같은 ID의 Wire 생성
	dup := Wire{
		ID:           wire.ID,
		SourceNodeID: nodeA.ID,
		SourcePort:   "out",
		TargetNodeID: nodeB.ID,
		TargetPort:   "in",
		Mode:         WireBypass,
	}

	err := f.AddWire(dup)
	if err != ErrDuplicateWireID {
		t.Errorf("err = %v, want ErrDuplicateWireID", err)
	}
}

// ===========================================================================
// AddWire: Errors 포트를 소스 포트로 사용 가능
// ===========================================================================
func TestAddWire_ErrorPortAsSource(t *testing.T) {
	f := NewFlow("wire-error-port")
	nodeA := NewNodeDef("nodeA", "filter", WithErrorPort())
	nodeB := NewNodeDef("nodeB", "transform")
	_ = f.AddNode(nodeA)
	_ = f.AddNode(nodeB)

	wire := NewWire(nodeA.ID, "error", nodeB.ID, "in")

	err := f.AddWire(wire)
	if err != nil {
		t.Fatalf("Errors 포트를 소스로 사용한 AddWire 실패: %v", err)
	}
}

// ===========================================================================
// RemoveWire 성공
// ===========================================================================
func TestRemoveWire_Success(t *testing.T) {
	f, _, _, wire := setupTwoNodesAndWire(t)
	_ = f.AddWire(wire)

	err := f.RemoveWire(wire.ID)
	if err != nil {
		t.Fatalf("RemoveWire 실패: %v", err)
	}
	if len(f.Wires()) != 0 {
		t.Errorf("Wires 길이 = %d, want 0", len(f.Wires()))
	}
}

// ===========================================================================
// RemoveWire: 존재하지 않는 Wire
// ===========================================================================
func TestRemoveWire_NotFound(t *testing.T) {
	f := NewFlow("rm-wire-nf")

	err := f.RemoveWire("nonexistent")
	if err != ErrWireNotFound {
		t.Errorf("err = %v, want ErrWireNotFound", err)
	}
}

// ===========================================================================
// AC-13: Nodes() 방어적 복사
// ===========================================================================
func TestNodes_DefensiveCopy(t *testing.T) {
	f := NewFlow("defensive")
	node := NewNodeDef("original", "filter")
	_ = f.AddNode(node)

	// 반환된 슬라이스를 수정
	nodes := f.Nodes()
	nodes[0].Name = "modified"

	// 원본에 영향 없음
	got, ok := f.Node("original")
	if !ok {
		t.Fatal("원본 노드가 사라졌습니다 - 방어적 복사 실패")
	}
	if got.Name != "original" {
		t.Errorf("Node.Name = %q, want %q (방어적 복사 실패)", got.Name, "original")
	}
}

// ===========================================================================
// Wires() 방어적 복사
// ===========================================================================
func TestWires_DefensiveCopy(t *testing.T) {
	f, _, _, wire := setupTwoNodesAndWire(t)
	_ = f.AddWire(wire)

	wires := f.Wires()
	wires[0].SourcePort = "modified"

	// 원본에 영향 없음
	got, ok := f.Wire(wire.ID)
	if !ok {
		t.Fatal("Wire를 찾을 수 없습니다")
	}
	if got.SourcePort != "out" {
		t.Errorf("SourcePort = %q, want %q (방어적 복사 실패)", got.SourcePort, "out")
	}
}

// ===========================================================================
// Metadata() 방어적 복사
// ===========================================================================
func TestMetadata_DefensiveCopy(t *testing.T) {
	f := NewFlow("meta-copy", WithFlowMetadata("env", "prod"))

	md := f.Metadata()
	md["env"] = "dev"

	// 원본에 영향 없음
	if f.Metadata()["env"] != "prod" {
		t.Error("Metadata 방어적 복사 실패")
	}
}

// ===========================================================================
// AC-19: Node 조회 - ID 우선, Name 그 다음, 존재하지 않으면 false
// ===========================================================================
func TestNode_Lookup(t *testing.T) {
	f := NewFlow("lookup")
	node := NewNodeDef("my-node", "filter")
	_ = f.AddNode(node)

	// ID로 조회
	got, ok := f.Node(node.ID)
	if !ok {
		t.Fatal("ID로 조회 실패")
	}
	if got.ID != node.ID {
		t.Errorf("Node.ID = %q, want %q", got.ID, node.ID)
	}

	// Name으로 조회
	got2, ok2 := f.Node("my-node")
	if !ok2 {
		t.Fatal("Name으로 조회 실패")
	}
	if got2.Name != "my-node" {
		t.Errorf("Node.Name = %q, want %q", got2.Name, "my-node")
	}

	// 존재하지 않는 노드
	_, ok3 := f.Node("nonexistent")
	if ok3 {
		t.Error("존재하지 않는 노드 조회가 true를 반환했습니다")
	}
}

// ===========================================================================
// Wire() ID로 조회
// ===========================================================================
func TestWire_Lookup(t *testing.T) {
	f, _, _, wire := setupTwoNodesAndWire(t)
	_ = f.AddWire(wire)

	got, ok := f.Wire(wire.ID)
	if !ok {
		t.Fatal("Wire ID로 조회 실패")
	}
	if got.ID != wire.ID {
		t.Errorf("Wire.ID = %q, want %q", got.ID, wire.ID)
	}

	// 존재하지 않는 Wire
	_, ok2 := f.Wire("nonexistent")
	if ok2 {
		t.Error("존재하지 않는 Wire 조회가 true를 반환했습니다")
	}
}

// ===========================================================================
// SetState: 유효한 전이 (Stored -> Loaded)
// ===========================================================================
func TestSetState_ValidTransition(t *testing.T) {
	f := NewFlow("state-test")
	initialUpdatedAt := f.UpdatedAt()

	// 약간의 시간차를 위해 대기
	time.Sleep(time.Millisecond)

	err := f.SetState(FlowLoaded)
	if err != nil {
		t.Fatalf("SetState(Loaded) 실패: %v", err)
	}
	if f.State() != FlowLoaded {
		t.Errorf("State = %q, want %q", f.State(), FlowLoaded)
	}

	// UpdatedAt이 변경되었는지 확인
	if !f.UpdatedAt().After(initialUpdatedAt) {
		t.Error("SetState 후 UpdatedAt이 갱신되지 않았습니다")
	}
}

// ===========================================================================
// SetState: 유효하지 않은 전이 (Stored -> Running)
// ===========================================================================
func TestSetState_InvalidTransition(t *testing.T) {
	f := NewFlow("state-invalid")

	err := f.SetState(FlowRunning)
	if err != ErrInvalidStateTransition {
		t.Errorf("err = %v, want ErrInvalidStateTransition", err)
	}

	// 상태 변경 없음
	if f.State() != FlowStored {
		t.Errorf("상태가 변경되었습니다: %q, want %q", f.State(), FlowStored)
	}
}

// ===========================================================================
// SetDescription
// ===========================================================================
func TestSetDescription(t *testing.T) {
	f := NewFlow("desc-test")

	f.SetDescription("새로운 설명")

	if f.Description() != "새로운 설명" {
		t.Errorf("Description = %q, want %q", f.Description(), "새로운 설명")
	}
}

// ===========================================================================
// SetConfig
// ===========================================================================
func TestSetConfig(t *testing.T) {
	f := NewFlow("config-test")

	newCfg := FlowConfig{
		TrackHistory:   true,
		MaxHistorySize: 200,
		ErrorHandling:  ErrorIgnore,
	}
	f.SetConfig(newCfg)

	cfg := f.Config()
	if cfg.TrackHistory != true {
		t.Error("TrackHistory = false, want true")
	}
	if cfg.MaxHistorySize != 200 {
		t.Errorf("MaxHistorySize = %d, want 200", cfg.MaxHistorySize)
	}
	if cfg.ErrorHandling != ErrorIgnore {
		t.Errorf("ErrorHandling = %q, want %q", cfg.ErrorHandling, ErrorIgnore)
	}
}

// ===========================================================================
// SetMetadata / RemoveMetadata
// ===========================================================================
func TestSetMetadata_And_RemoveMetadata(t *testing.T) {
	f := NewFlow("meta-test")

	f.SetMetadata("author", "xtra")
	if f.Metadata()["author"] != "xtra" {
		t.Errorf("Metadata[author] = %q, want %q", f.Metadata()["author"], "xtra")
	}

	f.SetMetadata("version", "1.0")
	if f.Metadata()["version"] != "1.0" {
		t.Errorf("Metadata[version] = %q, want %q", f.Metadata()["version"], "1.0")
	}

	// RemoveMetadata
	f.RemoveMetadata("author")
	if _, exists := f.Metadata()["author"]; exists {
		t.Error("author 메타데이터가 삭제되지 않았습니다")
	}

	// version은 남아있어야 함
	if f.Metadata()["version"] != "1.0" {
		t.Error("version 메타데이터가 사라졌습니다")
	}
}

// ===========================================================================
// AddNode UpdatedAt 갱신 확인
// ===========================================================================
func TestAddNode_UpdatesTimestamp(t *testing.T) {
	f := NewFlow("ts-test")
	initialUpdatedAt := f.UpdatedAt()

	time.Sleep(time.Millisecond)

	node := NewNodeDef("n1", "filter")
	_ = f.AddNode(node)

	if !f.UpdatedAt().After(initialUpdatedAt) {
		t.Error("AddNode 후 UpdatedAt이 갱신되지 않았습니다")
	}
}

// ===========================================================================
// RemoveNode UpdatedAt 갱신 확인
// ===========================================================================
func TestRemoveNode_UpdatesTimestamp(t *testing.T) {
	f := NewFlow("ts-rm")
	node := NewNodeDef("n1", "filter")
	_ = f.AddNode(node)

	initialUpdatedAt := f.UpdatedAt()
	time.Sleep(time.Millisecond)

	_ = f.RemoveNode("n1")

	if !f.UpdatedAt().After(initialUpdatedAt) {
		t.Error("RemoveNode 후 UpdatedAt이 갱신되지 않았습니다")
	}
}

// ===========================================================================
// AddWire UpdatedAt 갱신 확인
// ===========================================================================
func TestAddWire_UpdatesTimestamp(t *testing.T) {
	f, _, _, wire := setupTwoNodesAndWire(t)

	initialUpdatedAt := f.UpdatedAt()
	time.Sleep(time.Millisecond)

	_ = f.AddWire(wire)

	if !f.UpdatedAt().After(initialUpdatedAt) {
		t.Error("AddWire 후 UpdatedAt이 갱신되지 않았습니다")
	}
}

// ===========================================================================
// RemoveWire UpdatedAt 갱신 확인
// ===========================================================================
func TestRemoveWire_UpdatesTimestamp(t *testing.T) {
	f, _, _, wire := setupTwoNodesAndWire(t)
	_ = f.AddWire(wire)

	initialUpdatedAt := f.UpdatedAt()
	time.Sleep(time.Millisecond)

	_ = f.RemoveWire(wire.ID)

	if !f.UpdatedAt().After(initialUpdatedAt) {
		t.Error("RemoveWire 후 UpdatedAt이 갱신되지 않았습니다")
	}
}

// ===========================================================================
// ErrorPolicy 상수 값 검증
// ===========================================================================
func TestErrorPolicy_Constants(t *testing.T) {
	tests := []struct {
		name string
		got  ErrorPolicy
		want string
	}{
		{"ErrorPropagate", ErrorPropagate, "propagate"},
		{"ErrorIgnore", ErrorIgnore, "ignore"},
		{"ErrorStop", ErrorStop, "stop"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

// ===========================================================================
// RemoveNode: 타겟 노드 제거 시 연결된 Wire도 제거
// ===========================================================================
func TestRemoveNode_TargetCascadesWireRemoval(t *testing.T) {
	f, _, nodeB, wire := setupTwoNodesAndWire(t)
	_ = f.AddWire(wire)

	// nodeB(타겟) 제거 -> Wire도 제거되어야 함
	if err := f.RemoveNode(nodeB.ID); err != nil {
		t.Fatalf("RemoveNode 실패: %v", err)
	}

	if len(f.Wires()) != 0 {
		t.Errorf("타겟 노드 제거 후 Wires 길이 = %d, want 0", len(f.Wires()))
	}
}

// ===========================================================================
// Config() 반환값 검증
// ===========================================================================
func TestConfig_Returns(t *testing.T) {
	f := NewFlow("cfg-return")
	cfg := f.Config()

	// 기본값 확인
	if cfg.TrackHistory != false {
		t.Error("기본 TrackHistory는 false여야 합니다")
	}
}

// ===========================================================================
// CreatedAt / UpdatedAt 기본 동작
// ===========================================================================
func TestCreatedAt_UpdatedAt(t *testing.T) {
	f := NewFlow("time-test")

	if f.CreatedAt().IsZero() {
		t.Error("CreatedAt이 zero입니다")
	}
	if f.UpdatedAt().IsZero() {
		t.Error("UpdatedAt이 zero입니다")
	}
}
