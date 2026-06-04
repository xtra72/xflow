package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/internal/storage"
	"github.com/xtra/xflow/pkg/flow"
)

// ---------------------------------------------------------------------------
// 테스트 헬퍼 — fake in-memory FlowRepository
// ---------------------------------------------------------------------------

// fakeFlowRepo 는 ExpandSubflows 테스트용 메모리 저장소이다.
// 참조 플로우를 id 로 등록·조회하며, 동시 접근 안전을 위해 뮤텍스로 보호한다.
type fakeFlowRepo struct {
	mu    sync.RWMutex
	flows map[string]flow.Flow
}

func newFakeFlowRepo() *fakeFlowRepo {
	return &fakeFlowRepo{flows: make(map[string]flow.Flow)}
}

func (r *fakeFlowRepo) put(f flow.Flow) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flows[f.ID()] = f
}

func (r *fakeFlowRepo) Save(_ context.Context, f flow.Flow) error {
	r.put(f)
	return nil
}

func (r *fakeFlowRepo) Get(_ context.Context, id string) (flow.Flow, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.flows[id]
	if !ok {
		return nil, storage.ErrFlowNotFound
	}
	return f, nil
}

func (r *fakeFlowRepo) List(_ context.Context) ([]flow.Flow, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]flow.Flow, 0, len(r.flows))
	for _, f := range r.flows {
		out = append(out, f)
	}
	return out, nil
}

func (r *fakeFlowRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.flows[id]; !ok {
		return storage.ErrFlowNotFound
	}
	delete(r.flows, id)
	return nil
}

func (r *fakeFlowRepo) Close() error { return nil }

var _ storage.FlowRepository = (*fakeFlowRepo)(nil)

// ---------------------------------------------------------------------------
// 테스트 헬퍼 — 플로우/노드/와이어 빌더
// ---------------------------------------------------------------------------

// inPort/outPort 는 이름으로 입출력 포트를 만든다.
func inPort(name string) flow.Port { return flow.Port{ID: name, Name: name, Direction: flow.PortInput} }
func outPort(name string) flow.Port {
	return flow.Port{ID: name, Name: name, Direction: flow.PortOutput}
}

// passthroughNode 는 입력 "in" → 출력 "out" 을 가진 transform 노드를 만든다.
// transform 은 변환 함수가 없으면 메시지를 그대로 통과시킨다(identity).
func passthroughNode(id, name string) flow.NodeDef {
	return flow.NodeDef{
		ID:      id,
		Name:    name,
		Type:    "transform",
		Inputs:  []flow.Port{inPort("in")},
		Outputs: []flow.Port{outPort("out")},
	}
}

// flowNode 는 다른 플로우를 참조하는 flow-node 노드를 만든다.
// 입출력 핸들은 참조 플로우의 포트 이름과 일치시킨다(테스트 단순화).
func flowNode(id, name, refFlowID string, inputs, outputs []flow.Port) flow.NodeDef {
	return flow.NodeDef{
		ID:      id,
		Name:    name,
		Type:    flowNodeType,
		Config:  map[string]any{flowNodeFlowIDKey: refFlowID},
		Inputs:  inputs,
		Outputs: outputs,
	}
}

// wire 는 단순 와이어를 만든다(기본 bypass).
func wire(id, srcNode, srcPort, tgtNode, tgtPort string) flow.Wire {
	return flow.Wire{
		ID:           id,
		Type:         flow.WireSimple,
		SourceNodeID: srcNode,
		SourcePort:   srcPort,
		TargetNodeID: tgtNode,
		TargetPort:   tgtPort,
		Mode:         flow.WireBypass,
	}
}

// nodeByID 는 확장 결과에서 ID 로 노드를 찾는다.
func nodeByID(nodes []flow.NodeDef, id string) (flow.NodeDef, bool) {
	for _, n := range nodes {
		if n.ID == id {
			return n, true
		}
	}
	return flow.NodeDef{}, false
}

// ===========================================================================
// 단위 테스트 — 네임스페이싱
// ===========================================================================

// 확장된 노드 ID 가 subflow_<flowNodeID>_<origNode> 규칙을 따르는지 검증한다.
func TestExpandSubflows_네임스페이싱_규칙(t *testing.T) {
	repo := newFakeFlowRepo()

	// 참조 플로우 F: input "X" → A(transform) → output "Y"
	fA := passthroughNode("A", "노드A")
	ref := flow.NewFlow("F",
		flow.WithNodes(fA),
		flow.WithWires(
			wire("bi", flow.FlowInputBoundaryID, "X", "A", "in"),
			wire("bo", "A", "out", flow.FlowOutputBoundaryID, "Y"),
		),
		flow.WithFlowInputPorts(inPort("X")),
		flow.WithFlowOutputPorts(outPort("Y")),
	)
	repo.put(ref)

	// 부모 B: src → flow-node(F) → sink
	src := passthroughNode("src", "소스")
	sink := passthroughNode("sink", "싱크")
	fn := flowNode("fn1", "서브", ref.ID(), []flow.Port{inPort("X")}, []flow.Port{outPort("Y")})
	parent := flow.NewFlow("B",
		flow.WithNodes(src, fn, sink),
		flow.WithWires(
			wire("p_in", "src", "out", "fn1", "X"),
			wire("p_out", "fn1", "Y", "sink", "in"),
		),
	)

	got, err := ExpandSubflows(context.Background(), parent, repo)
	require.NoError(t, err)

	// flow-node 타입 노드가 없어야 한다.
	for _, n := range got.Nodes() {
		assert.NotEqual(t, flowNodeType, n.Type, "확장 후 flow-node 타입이 남으면 안 된다")
	}

	// 네임스페이스 노드 존재: subflow_fn1_A
	expandedID := "subflow_fn1_A"
	nd, ok := nodeByID(got.Nodes(), expandedID)
	assert.True(t, ok, "네임스페이스 노드 %q 가 존재해야 한다", expandedID)
	assert.Equal(t, "transform", nd.Type)
	assert.Equal(t, "노드A", nd.Name, "이름은 보존되어야 한다")

	// 부모의 일반 노드는 그대로 보존.
	_, hasSrc := nodeByID(got.Nodes(), "src")
	_, hasSink := nodeByID(got.Nodes(), "sink")
	assert.True(t, hasSrc && hasSink)

	// INPUT 재배선: src.out → subflow_fn1_A.in 가 존재해야 한다.
	assert.True(t, hasWire(got.Wires(), "src", "out", "subflow_fn1_A", "in"),
		"입력 재배선 와이어 src→subflow_fn1_A 가 있어야 한다")
	// OUTPUT 재배선: subflow_fn1_A.out → sink.in 가 존재해야 한다.
	assert.True(t, hasWire(got.Wires(), "subflow_fn1_A", "out", "sink", "in"),
		"출력 재배선 와이어 subflow_fn1_A→sink 가 있어야 한다")

	// 부모 ID 보존(평탄화 결과는 부모 정체성 유지).
	assert.Equal(t, parent.ID(), got.ID())
}

// hasWire 는 주어진 (src,srcPort,tgt,tgtPort) 와이어가 존재하는지 확인한다.
func hasWire(wires []flow.Wire, src, srcPort, tgt, tgtPort string) bool {
	for _, w := range wires {
		if w.SourceNodeID == src && w.SourcePort == srcPort &&
			w.TargetNodeID == tgt && w.TargetPort == tgtPort {
			return true
		}
	}
	return false
}

// 동일 부모 내 두 flow-node 가 같은 플로우를 참조해도 격리(독립 인스턴스)되는지 검증한다.
func TestExpandSubflows_동일플로우_이중참조_격리(t *testing.T) {
	repo := newFakeFlowRepo()
	fA := passthroughNode("A", "노드A")
	ref := flow.NewFlow("F",
		flow.WithNodes(fA),
		flow.WithWires(
			wire("bi", flow.FlowInputBoundaryID, "X", "A", "in"),
			wire("bo", "A", "out", flow.FlowOutputBoundaryID, "Y"),
		),
		flow.WithFlowInputPorts(inPort("X")),
		flow.WithFlowOutputPorts(outPort("Y")),
	)
	repo.put(ref)

	src := passthroughNode("src", "소스")
	fn1 := flowNode("fn1", "서브1", ref.ID(), []flow.Port{inPort("X")}, []flow.Port{outPort("Y")})
	fn2 := flowNode("fn2", "서브2", ref.ID(), []flow.Port{inPort("X")}, []flow.Port{outPort("Y")})
	sink := passthroughNode("sink", "싱크")
	parent := flow.NewFlow("B",
		flow.WithNodes(src, fn1, fn2, sink),
		flow.WithWires(
			wire("w1", "src", "out", "fn1", "X"),
			wire("w2", "fn1", "Y", "fn2", "X"),
			wire("w3", "fn2", "Y", "sink", "in"),
		),
	)

	got, err := ExpandSubflows(context.Background(), parent, repo)
	require.NoError(t, err)

	// 두 개의 서로 다른 접두사 인스턴스가 존재해야 한다.
	_, ok1 := nodeByID(got.Nodes(), "subflow_fn1_A")
	_, ok2 := nodeByID(got.Nodes(), "subflow_fn2_A")
	assert.True(t, ok1, "fn1 인스턴스 노드 존재")
	assert.True(t, ok2, "fn2 인스턴스 노드 존재")
	assert.NotEqual(t, "subflow_fn1_A", "subflow_fn2_A")

	// 라우팅 체인: src → fn1.A → fn2.A → sink (재배선으로 직접 연결).
	assert.True(t, hasWire(got.Wires(), "src", "out", "subflow_fn1_A", "in"))
	assert.True(t, hasWire(got.Wires(), "subflow_fn1_A", "out", "subflow_fn2_A", "in"),
		"fn1 인스턴스 출력이 fn2 인스턴스 입력으로 직접 재배선되어야 한다")
	assert.True(t, hasWire(got.Wires(), "subflow_fn2_A", "out", "sink", "in"))

	// 와이어 ID 유일성 검증.
	seen := make(map[string]bool)
	for _, w := range got.Wires() {
		assert.False(t, seen[w.ID], "와이어 ID 중복: %s", w.ID)
		seen[w.ID] = true
	}
}

// 중첩(F 가 flow-node(G) 를 포함)이 누적 접두사로 완전히 해소되는지 검증한다.
func TestExpandSubflows_중첩_재귀해소(t *testing.T) {
	repo := newFakeFlowRepo()

	// G: input "GX" → C → output "GY"
	gC := passthroughNode("C", "노드C")
	g := flow.NewFlow("G",
		flow.WithNodes(gC),
		flow.WithWires(
			wire("gbi", flow.FlowInputBoundaryID, "GX", "C", "in"),
			wire("gbo", "C", "out", flow.FlowOutputBoundaryID, "GY"),
		),
		flow.WithFlowInputPorts(inPort("GX")),
		flow.WithFlowOutputPorts(outPort("GY")),
	)
	repo.put(g)

	// F: input "X" → flow-node(G) → output "Y"
	fnG := flowNode("gn", "지서브", g.ID(), []flow.Port{inPort("GX")}, []flow.Port{outPort("GY")})
	f := flow.NewFlow("F",
		flow.WithNodes(fnG),
		flow.WithWires(
			wire("fbi", flow.FlowInputBoundaryID, "X", "gn", "GX"),
			wire("fbo", "gn", "GY", flow.FlowOutputBoundaryID, "Y"),
		),
		flow.WithFlowInputPorts(inPort("X")),
		flow.WithFlowOutputPorts(outPort("Y")),
	)
	repo.put(f)

	// B: src → flow-node(F) → sink
	src := passthroughNode("src", "소스")
	sink := passthroughNode("sink", "싱크")
	fnF := flowNode("fn1", "서브", f.ID(), []flow.Port{inPort("X")}, []flow.Port{outPort("Y")})
	parent := flow.NewFlow("B",
		flow.WithNodes(src, fnF, sink),
		flow.WithWires(
			wire("p_in", "src", "out", "fn1", "X"),
			wire("p_out", "fn1", "Y", "sink", "in"),
		),
	)

	got, err := ExpandSubflows(context.Background(), parent, repo)
	require.NoError(t, err)

	// 누적 접두사: subflow_fn1_subflow_gn_C
	nestedID := "subflow_fn1_subflow_gn_C"
	_, ok := nodeByID(got.Nodes(), nestedID)
	assert.True(t, ok, "중첩 누적 접두사 노드 %q 가 존재해야 한다", nestedID)

	// flow-node 타입이 전혀 남지 않아야 한다.
	for _, n := range got.Nodes() {
		assert.NotEqual(t, flowNodeType, n.Type)
	}

	// 라우팅: src → nested C → sink 직접 연결.
	assert.True(t, hasWire(got.Wires(), "src", "out", nestedID, "in"))
	assert.True(t, hasWire(got.Wires(), nestedID, "out", "sink", "in"))
}

// 참조 플로우를 수정한 뒤 재확장하면 변경이 반영되는지(항상 최신) 검증한다.
func TestExpandSubflows_항상최신_반영(t *testing.T) {
	repo := newFakeFlowRepo()
	ref := flow.NewFlow("F",
		flow.WithNodes(passthroughNode("A", "노드A")),
		flow.WithWires(
			wire("bi", flow.FlowInputBoundaryID, "X", "A", "in"),
			wire("bo", "A", "out", flow.FlowOutputBoundaryID, "Y"),
		),
		flow.WithFlowInputPorts(inPort("X")),
		flow.WithFlowOutputPorts(outPort("Y")),
	)
	repo.put(ref)

	fn := flowNode("fn1", "서브", ref.ID(), []flow.Port{inPort("X")}, []flow.Port{outPort("Y")})
	parent := flow.NewFlow("B",
		flow.WithNodes(passthroughNode("src", "소스"), fn),
		flow.WithWires(wire("p_in", "src", "out", "fn1", "X")),
	)

	got1, err := ExpandSubflows(context.Background(), parent, repo)
	require.NoError(t, err)
	_, hasA := nodeByID(got1.Nodes(), "subflow_fn1_A")
	assert.True(t, hasA)

	// 참조 플로우 F 의 내부 노드를 A → B2 로 교체하여 같은 id 로 재저장.
	refV2 := flow.NewFlowWithID(ref.ID(), "F",
		flow.WithNodes(passthroughNode("B2", "노드B2")),
		flow.WithWires(
			wire("bi", flow.FlowInputBoundaryID, "X", "B2", "in"),
			wire("bo", "B2", "out", flow.FlowOutputBoundaryID, "Y"),
		),
		flow.WithFlowInputPorts(inPort("X")),
		flow.WithFlowOutputPorts(outPort("Y")),
	)
	repo.put(refV2)

	got2, err := ExpandSubflows(context.Background(), parent, repo)
	require.NoError(t, err)
	_, hasOld := nodeByID(got2.Nodes(), "subflow_fn1_A")
	_, hasNew := nodeByID(got2.Nodes(), "subflow_fn1_B2")
	assert.False(t, hasOld, "이전 정의(A)는 더 이상 확장되지 않아야 한다")
	assert.True(t, hasNew, "변경된 정의(B2)가 반영되어야 한다")
}

// 참조 포트가 제거되어 매칭 경계 와이어가 없으면 부모 와이어가 dangling 으로 드롭되는지 검증한다.
func TestExpandSubflows_dangling_드롭_무크래시(t *testing.T) {
	repo := newFakeFlowRepo()
	// F 는 입력 포트 "X" 만 가지고 출력 포트 "Y" 의 경계 와이어가 없다(출력 포트 제거 시나리오).
	ref := flow.NewFlow("F",
		flow.WithNodes(passthroughNode("A", "노드A")),
		flow.WithWires(
			wire("bi", flow.FlowInputBoundaryID, "X", "A", "in"),
			// 출력 경계 와이어 없음 → flow-node 의 Y 핸들에 매칭되는 내부 경계가 없다.
		),
		flow.WithFlowInputPorts(inPort("X")),
	)
	repo.put(ref)

	fn := flowNode("fn1", "서브", ref.ID(), []flow.Port{inPort("X")}, []flow.Port{outPort("Y")})
	parent := flow.NewFlow("B",
		flow.WithNodes(passthroughNode("src", "소스"), fn, passthroughNode("sink", "싱크")),
		flow.WithWires(
			wire("p_in", "src", "out", "fn1", "X"),
			wire("p_out", "fn1", "Y", "sink", "in"), // 이 와이어가 dangling 으로 드롭되어야 한다.
		),
	)

	got, err := ExpandSubflows(context.Background(), parent, repo)
	require.NoError(t, err, "dangling 은 에러가 아니라 드롭+경고여야 한다")

	// 입력 재배선은 정상 동작.
	assert.True(t, hasWire(got.Wires(), "src", "out", "subflow_fn1_A", "in"))
	// 출력 측은 매칭 경계가 없으므로 sink 로 가는 재배선이 없어야 한다.
	assert.False(t, hasWire(got.Wires(), "subflow_fn1_A", "out", "sink", "in"),
		"매칭 경계가 없으면 출력 재배선이 생성되면 안 된다")
	// 원본 dangling 부모 와이어(fn1→sink)도 결과에 남으면 안 된다.
	assert.False(t, hasWire(got.Wires(), "fn1", "Y", "sink", "in"))
}

// 멀티-팬아웃 입력과 다중 내부 소비자의 카르테시안 재배선을 검증한다.
func TestExpandSubflows_경계재배선_카르테시안(t *testing.T) {
	repo := newFakeFlowRepo()
	// F: 입력 "X" 가 두 내부 노드(A, B2)로 분기. 각각 출력 "Y" 로 수렴.
	ref := flow.NewFlow("F",
		flow.WithNodes(passthroughNode("A", "A"), passthroughNode("B2", "B2")),
		flow.WithWires(
			wire("bi1", flow.FlowInputBoundaryID, "X", "A", "in"),
			wire("bi2", flow.FlowInputBoundaryID, "X", "B2", "in"),
			wire("bo1", "A", "out", flow.FlowOutputBoundaryID, "Y"),
			wire("bo2", "B2", "out", flow.FlowOutputBoundaryID, "Y"),
		),
		flow.WithFlowInputPorts(inPort("X")),
		flow.WithFlowOutputPorts(outPort("Y")),
	)
	repo.put(ref)

	// 부모: 두 소스(s1, s2)가 모두 fn1.X 로 팬인. fn1.Y 가 sink 로.
	fn := flowNode("fn1", "서브", ref.ID(), []flow.Port{inPort("X")}, []flow.Port{outPort("Y")})
	parent := flow.NewFlow("B",
		flow.WithNodes(passthroughNode("s1", "s1"), passthroughNode("s2", "s2"), fn, passthroughNode("sink", "싱크")),
		flow.WithWires(
			wire("p1", "s1", "out", "fn1", "X"),
			wire("p2", "s2", "out", "fn1", "X"),
			wire("po", "fn1", "Y", "sink", "in"),
		),
	)

	got, err := ExpandSubflows(context.Background(), parent, repo)
	require.NoError(t, err)

	// 입력 카르테시안: {s1,s2} × {A,B2} = 4 재배선.
	assert.True(t, hasWire(got.Wires(), "s1", "out", "subflow_fn1_A", "in"))
	assert.True(t, hasWire(got.Wires(), "s1", "out", "subflow_fn1_B2", "in"))
	assert.True(t, hasWire(got.Wires(), "s2", "out", "subflow_fn1_A", "in"))
	assert.True(t, hasWire(got.Wires(), "s2", "out", "subflow_fn1_B2", "in"))
	// 출력 카르테시안: {A,B2} × {sink} = 2 재배선.
	assert.True(t, hasWire(got.Wires(), "subflow_fn1_A", "out", "sink", "in"))
	assert.True(t, hasWire(got.Wires(), "subflow_fn1_B2", "out", "sink", "in"))
}

// 깊이 상한 초과 시 에러를 반환하는지 검증한다.
func TestExpandSubflows_깊이상한_초과_에러(t *testing.T) {
	repo := newFakeFlowRepo()

	// depth+1 단계로 self 가 아닌 체인을 만든다: L0 → L1 → ... → L9 (각 단계 flow-node 참조).
	// maxSubflowExpandDepth=8 이므로 9단계 중첩은 초과한다.
	const chain = 10
	var prevID string
	for i := chain - 1; i >= 0; i-- {
		id := "L" + string(rune('0'+i))
		var nodes []flow.NodeDef
		var wires []flow.Wire
		if i == chain-1 {
			// 말단: 단순 passthrough.
			nodes = []flow.NodeDef{passthroughNode("leaf", "leaf")}
			wires = []flow.Wire{
				wire("bi", flow.FlowInputBoundaryID, "X", "leaf", "in"),
				wire("bo", "leaf", "out", flow.FlowOutputBoundaryID, "Y"),
			}
		} else {
			fn := flowNode("fn", "child", "ref-"+prevID, []flow.Port{inPort("X")}, []flow.Port{outPort("Y")})
			nodes = []flow.NodeDef{fn}
			wires = []flow.Wire{
				wire("bi", flow.FlowInputBoundaryID, "X", "fn", "X"),
				wire("bo", "fn", "Y", flow.FlowOutputBoundaryID, "Y"),
			}
		}
		f := flow.NewFlowWithID("ref-"+id, id,
			flow.WithNodes(nodes...),
			flow.WithWires(wires...),
			flow.WithFlowInputPorts(inPort("X")),
			flow.WithFlowOutputPorts(outPort("Y")),
		)
		repo.put(f)
		prevID = id
	}

	// 루트 부모: flow-node(ref-L0)
	root := flow.NewFlow("root",
		flow.WithNodes(flowNode("rn", "root-sub", "ref-L0", []flow.Port{inPort("X")}, []flow.Port{outPort("Y")})),
	)

	_, err := ExpandSubflows(context.Background(), root, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "깊이 초과")
}

// 누락 참조(repo 에 없는 flow_id)는 에러여야 한다(REQ-SUBFLOW-F03).
func TestExpandSubflows_누락참조_에러(t *testing.T) {
	repo := newFakeFlowRepo()
	parent := flow.NewFlow("B",
		flow.WithNodes(flowNode("fn1", "서브", "does-not-exist", []flow.Port{inPort("X")}, []flow.Port{outPort("Y")})),
	)
	_, err := ExpandSubflows(context.Background(), parent, repo)
	require.Error(t, err)
}

// flow-node 가 없는 기존 플로우는 변경 없이 그대로 반환되는지(회귀 0) 검증한다.
func TestExpandSubflows_flow노드없음_무변경(t *testing.T) {
	repo := newFakeFlowRepo()
	parent := flow.NewFlow("B",
		flow.WithNodes(passthroughNode("a", "a"), passthroughNode("b", "b")),
		flow.WithWires(wire("w", "a", "out", "b", "in")),
	)
	got, err := ExpandSubflows(context.Background(), parent, repo)
	require.NoError(t, err)
	assert.Equal(t, parent.ID(), got.ID())
	assert.Len(t, got.Nodes(), 2)
	assert.True(t, hasWire(got.Wires(), "a", "out", "b", "in"))
}

// ===========================================================================
// 통합 테스트 — 실제 엔진을 통한 end-to-end 메시지 라우팅
// ===========================================================================

// captureSink 는 output 노드가 emit 한 디버그 메시지를 수집하는 DebugSink 더블이다.
type captureSink struct {
	mu   sync.Mutex
	msgs []string
}

func (s *captureSink) SendDebug(_ string, msg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, msg)
	return nil
}

func (s *captureSink) all() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.msgs))
	copy(out, s.msgs)
	return out
}

// 실제 엔진으로 확장된 플로우를 배포·실행하여, 메시지가 서브플로우 내부 노드를 거쳐
// 부모의 collector(output) 로 라우팅되는지 end-to-end 로 검증한다.
//
// 구성:
//
//	F = [input "X" → A(transform passthrough) → output "Y"]
//	B = [trigger(소스, 고정 payload) → flow-node(F) handle X..Y → output(collector, DebugSink)]
//
// trigger 가 고정 payload(marker)를 주기적으로 생성 → 서브플로우 A 통과 →
// collector 가 DebugSink 로 emit → marker 문자열이 캡처되면 라우팅 성공.
func TestExpandSubflows_엔진_end2end_라우팅(t *testing.T) {
	repo := newFakeFlowRepo()

	// 참조 플로우 F.
	ref := flow.NewFlow("F",
		flow.WithNodes(passthroughNode("A", "통과노드A")),
		flow.WithWires(
			wire("bi", flow.FlowInputBoundaryID, "X", "A", "in"),
			wire("bo", "A", "out", flow.FlowOutputBoundaryID, "Y"),
		),
		flow.WithFlowInputPorts(inPort("X")),
		flow.WithFlowOutputPorts(outPort("Y")),
	)
	repo.put(ref)

	// 실제 timer agent (trigger 소스 구동용).
	timerAgent := system.NewTimerAgent()
	require.NoError(t, timerAgent.Init(context.Background()))
	defer timerAgent.Stop(context.Background())

	const marker = "subflow-routed-42"

	// 부모 B: trigger → flow-node(F) → output(collector).
	triggerNode := flow.NodeDef{
		ID:   "trig",
		Name: "소스트리거",
		Type: "trigger",
		Config: map[string]any{
			"schedules":    []any{map[string]any{"type": "interval", "value": "100ms"}},
			"payload":      map[string]any{"marker": marker},
			"_timer_agent": timerAgent,
		},
		Outputs: []flow.Port{outPort("out")},
	}
	collector := flow.NodeDef{
		ID:     "collector",
		Name:   "수집기",
		Type:   "output",
		Config: map[string]any{"output": "editor", "format": "json"},
		Inputs: []flow.Port{inPort("in")},
	}
	fn := flowNode("fn1", "서브", ref.ID(), []flow.Port{inPort("X")}, []flow.Port{outPort("Y")})
	parent := flow.NewFlowWithID("parent-e2e", "B",
		flow.WithNodes(triggerNode, fn, collector),
		flow.WithWires(
			wire("p_in", "trig", "out", "fn1", "X"),
			wire("p_out", "fn1", "Y", "collector", "in"),
		),
	)
	repo.put(parent)

	// 확장.
	expanded, err := ExpandSubflows(context.Background(), parent, repo)
	require.NoError(t, err)

	// 확장 결과에 flow-node 가 없고, 서브플로우 내부 노드가 존재해야 한다.
	_, hasInternal := nodeByID(expanded.Nodes(), "subflow_fn1_A")
	require.True(t, hasInternal)

	// 경계 와이어 제거(단독 배포 전처리와 동일 — 확장 후 남은 자기 경계 없음).
	expanded = flow.StripBoundaryWires(expanded)

	// 실제 엔진으로 배포·시작.
	sink := &captureSink{}
	eng := engine.NewEngine(
		engine.WithNodeRegistry(node.NewRegistry()),
		engine.WithDebugSink(sink),
	)
	require.NoError(t, eng.DeployFlow(context.Background(), expanded))
	require.NoError(t, eng.StartFlow(context.Background(), expanded.ID()))
	defer func() {
		_ = eng.StopFlow(context.Background(), expanded.ID())
		_ = eng.UndeployFlow(context.Background(), expanded.ID())
	}()

	// marker 가 collector 출력에 나타날 때까지 대기(최대 3초; interval 100ms).
	deadline := time.Now().Add(3 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		for _, m := range sink.all() {
			if strings.Contains(m, marker) {
				found = true
				break
			}
		}
		if found {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	assert.True(t, found, "marker %q 가 서브플로우를 거쳐 collector 로 라우팅되어야 한다 (captured=%v)", marker, sink.all())
}
