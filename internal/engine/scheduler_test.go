package engine

import (
	"errors"
	"testing"

	"github.com/xtra/xflow/pkg/flow"
)

// newTestFlow 는 테스트용 Flow를 생성하는 헬퍼이다.
func newTestFlow(name string, nodes []flow.NodeDef, wires []flow.Wire) flow.Flow {
	return flow.NewFlow(name,
		flow.WithNodes(nodes...),
		flow.WithWires(wires...),
	)
}

func TestDAGScheduler_LinearChain(t *testing.T) {
	// A -> B -> C 선형 체인의 토폴로지 정렬을 검증한다.
	nodes := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
		flow.NewNodeDef("B", "transform"),
		flow.NewNodeDef("C", "transform"),
	}

	wires := []flow.Wire{
		flow.NewWire(nodes[0].ID, "out", nodes[1].ID, "in"),
		flow.NewWire(nodes[1].ID, "out", nodes[2].ID, "in"),
	}

	f := newTestFlow("linear", nodes, wires)

	scheduler := NewDAGScheduler()
	plan, err := scheduler.Plan(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Order에서 A가 B보다 앞에, B가 C보다 앞에 있어야 한다.
	orderIndex := make(map[string]int)
	for i, id := range plan.Order {
		orderIndex[id] = i
	}

	if orderIndex[nodes[0].ID] >= orderIndex[nodes[1].ID] {
		t.Errorf("A should come before B in topological order")
	}
	if orderIndex[nodes[1].ID] >= orderIndex[nodes[2].ID] {
		t.Errorf("B should come before C in topological order")
	}

	// Levels: [[A], [B], [C]]
	if len(plan.Levels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(plan.Levels))
	}
	if len(plan.Levels[0]) != 1 || plan.Levels[0][0] != nodes[0].ID {
		t.Errorf("level 0 should contain only A")
	}
	if len(plan.Levels[1]) != 1 || plan.Levels[1][0] != nodes[1].ID {
		t.Errorf("level 1 should contain only B")
	}
	if len(plan.Levels[2]) != 1 || plan.Levels[2][0] != nodes[2].ID {
		t.Errorf("level 2 should contain only C")
	}
}

func TestDAGScheduler_Diamond(t *testing.T) {
	// A -> B, A -> C, B -> D, C -> D 다이아몬드 패턴을 검증한다.
	nodes := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
		flow.NewNodeDef("B", "transform"),
		flow.NewNodeDef("C", "transform"),
		flow.NewNodeDef("D", "transform"),
	}

	wires := []flow.Wire{
		flow.NewWire(nodes[0].ID, "out", nodes[1].ID, "in"),
		flow.NewWire(nodes[0].ID, "out", nodes[2].ID, "in"),
		flow.NewWire(nodes[1].ID, "out", nodes[3].ID, "in"),
		flow.NewWire(nodes[2].ID, "out", nodes[3].ID, "in"),
	}

	f := newTestFlow("diamond", nodes, wires)

	scheduler := NewDAGScheduler()
	plan, err := scheduler.Plan(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	orderIndex := make(map[string]int)
	for i, id := range plan.Order {
		orderIndex[id] = i
	}

	// A는 B, C보다 앞에 있어야 한다.
	if orderIndex[nodes[0].ID] >= orderIndex[nodes[1].ID] {
		t.Errorf("A should come before B")
	}
	if orderIndex[nodes[0].ID] >= orderIndex[nodes[2].ID] {
		t.Errorf("A should come before C")
	}
	// B, C는 D보다 앞에 있어야 한다.
	if orderIndex[nodes[1].ID] >= orderIndex[nodes[3].ID] {
		t.Errorf("B should come before D")
	}
	if orderIndex[nodes[2].ID] >= orderIndex[nodes[3].ID] {
		t.Errorf("C should come before D")
	}

	// Levels: [[A], [B, C], [D]]
	if len(plan.Levels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(plan.Levels))
	}
	if len(plan.Levels[0]) != 1 {
		t.Errorf("level 0 should have 1 node, got %d", len(plan.Levels[0]))
	}
	if len(plan.Levels[1]) != 2 {
		t.Errorf("level 1 should have 2 nodes, got %d", len(plan.Levels[1]))
	}
	if len(plan.Levels[2]) != 1 {
		t.Errorf("level 2 should have 1 node, got %d", len(plan.Levels[2]))
	}
}

func TestDAGScheduler_ParallelPaths(t *testing.T) {
	// A -> B, C -> D (독립된 두 경로)를 검증한다.
	nodes := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
		flow.NewNodeDef("B", "transform"),
		flow.NewNodeDef("C", "transform"),
		flow.NewNodeDef("D", "transform"),
	}

	wires := []flow.Wire{
		flow.NewWire(nodes[0].ID, "out", nodes[1].ID, "in"),
		flow.NewWire(nodes[2].ID, "out", nodes[3].ID, "in"),
	}

	f := newTestFlow("parallel", nodes, wires)

	scheduler := NewDAGScheduler()
	plan, err := scheduler.Plan(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Level 0에 A와 C 둘 다 있어야 한다.
	if len(plan.Levels) != 2 {
		t.Fatalf("expected 2 levels, got %d", len(plan.Levels))
	}
	if len(plan.Levels[0]) != 2 {
		t.Errorf("level 0 should have 2 source nodes, got %d", len(plan.Levels[0]))
	}
	if len(plan.Levels[1]) != 2 {
		t.Errorf("level 1 should have 2 target nodes, got %d", len(plan.Levels[1]))
	}

	// 전체 4개 노드가 Order에 있어야 한다.
	if len(plan.Order) != 4 {
		t.Errorf("expected 4 nodes in order, got %d", len(plan.Order))
	}
}

func TestDAGScheduler_CycleDetected(t *testing.T) {
	// A -> B -> A 순환 그래프에서 ErrCycleDetected를 반환해야 한다.
	nodes := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
		flow.NewNodeDef("B", "transform"),
	}

	wires := []flow.Wire{
		flow.NewWire(nodes[0].ID, "out", nodes[1].ID, "in"),
		flow.NewWire(nodes[1].ID, "out", nodes[0].ID, "in"),
	}

	f := newTestFlow("cycle", nodes, wires)

	scheduler := NewDAGScheduler()
	_, err := scheduler.Plan(f)
	if err == nil {
		t.Fatal("expected error for cycle, got nil")
	}
	if !errors.Is(err, ErrCycleDetected) {
		t.Errorf("expected ErrCycleDetected, got %v", err)
	}
}

func TestDAGScheduler_SingleNode(t *testing.T) {
	// 단일 노드(와이어 없음)에서도 올바르게 동작해야 한다.
	nodes := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
	}

	f := newTestFlow("single", nodes, nil)

	scheduler := NewDAGScheduler()
	plan, err := scheduler.Plan(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Levels) != 1 {
		t.Fatalf("expected 1 level, got %d", len(plan.Levels))
	}
	if len(plan.Order) != 1 {
		t.Fatalf("expected 1 node in order, got %d", len(plan.Order))
	}
	if plan.Order[0] != nodes[0].ID {
		t.Errorf("expected node A in order, got %q", plan.Order[0])
	}
}

func TestDAGScheduler_EmptyFlow(t *testing.T) {
	// 노드가 없는 Flow에서도 에러 없이 빈 결과를 반환해야 한다.
	f := newTestFlow("empty", nil, nil)

	scheduler := NewDAGScheduler()
	plan, err := scheduler.Plan(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Levels) != 0 {
		t.Errorf("expected 0 levels, got %d", len(plan.Levels))
	}
	if len(plan.Order) != 0 {
		t.Errorf("expected 0 nodes in order, got %d", len(plan.Order))
	}
}

func TestSchedulerInterface(t *testing.T) {
	// DAGScheduler가 Scheduler 인터페이스를 구현하는지 검증한다.
	var _ Scheduler = (*DAGScheduler)(nil)
}
