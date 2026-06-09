package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/pkg/flow"
)

// ---------------------------------------------------------------------------
// 테스트 헬퍼 — fake RemoteFlowFetcher
// ---------------------------------------------------------------------------

// fakeRemoteFetcher 는 원격 서브플로우 fetch 를 흉내내는 더블이다.
// flows[instanceID+"/"+flowID] 에 등록된 정의 바이트를 반환하고, 미등록이면 err 를 반환한다.
type fakeRemoteFetcher struct {
	flows map[string][]byte
	err   error // 설정 시 모든 fetch 가 이 에러를 반환한다(오프라인/누락 시뮬레이션).
	calls int
}

func newFakeRemoteFetcher() *fakeRemoteFetcher {
	return &fakeRemoteFetcher{flows: make(map[string][]byte)}
}

func (f *fakeRemoteFetcher) put(instanceID, flowID string, data []byte) {
	f.flows[instanceID+"/"+flowID] = data
}

func (f *fakeRemoteFetcher) FetchRemoteFlow(_ context.Context, instanceID, flowID string) ([]byte, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	data, ok := f.flows[instanceID+"/"+flowID]
	if !ok {
		return nil, errors.New("fake remote: flow not found")
	}
	return data, nil
}

var _ RemoteFlowFetcher = (*fakeRemoteFetcher)(nil)

// remoteFlowGetBytes 는 query 프록시 flow/get 응답(= handler.FlowInfo 마샬)을 흉내내는
// 바이트를 만든다. Config 에는 flowToReactFlowConfig 와 동일한 React Flow 모양
// (nodes[data], edges, 최상위 inputs/outputs)을 담는다.
func remoteFlowGetBytes(t *testing.T, id, name string, reactConfig map[string]any) []byte {
	t.Helper()
	info := handler.FlowInfo{
		ID:     id,
		Name:   name,
		Status: "stored",
		Config: reactConfig,
	}
	data, err := json.Marshal(info)
	require.NoError(t, err)
	return data
}

// reactFlowConfigPassthrough 는 input "X" → A(transform) → output "Y" 를 가진
// self-contained 원격 플로우의 React Flow config 를 만든다(flowToReactFlowConfig 모양).
func reactFlowConfigPassthrough() map[string]any {
	return map[string]any{
		"nodes": []map[string]any{
			{
				"id":       "A",
				"type":     "custom",
				"position": map[string]any{"x": 0, "y": 0},
				"data": map[string]any{
					"label":    "노드A",
					"nodeType": "transform",
					"ports": []map[string]any{
						{"name": "in", "direction": "input"},
						{"name": "out", "direction": "output"},
					},
				},
			},
		},
		"edges": []map[string]any{
			{"id": "bi", "source": flow.FlowInputBoundaryID, "target": "A", "sourceHandle": "X", "targetHandle": "in"},
			{"id": "bo", "source": "A", "target": flow.FlowOutputBoundaryID, "sourceHandle": "out", "targetHandle": "Y"},
		},
		"inputs":  []map[string]any{{"id": "X", "name": "X", "direction": "input"}},
		"outputs": []map[string]any{{"id": "Y", "name": "Y", "direction": "output"}},
	}
}

// reactFlowConfigWithFlowNode 는 내부에 flow-node 를 포함하는(중첩) React Flow config 를
// 만든다(REQ-SUBFLOW-R08 중첩 거부 검증용).
func reactFlowConfigWithFlowNode() map[string]any {
	return map[string]any{
		"nodes": []map[string]any{
			{
				"id":       "nested",
				"type":     "custom",
				"position": map[string]any{"x": 0, "y": 0},
				"data": map[string]any{
					"label":    "중첩서브",
					"nodeType": flowNodeType,
					"flow_id":  "some-local-flow",
					"ports":    []map[string]any{},
				},
			},
		},
		"edges":   []map[string]any{},
		"inputs":  []map[string]any{{"id": "X", "name": "X", "direction": "input"}},
		"outputs": []map[string]any{{"id": "Y", "name": "Y", "direction": "output"}},
	}
}

// remoteFlowNode 는 원격 참조(remote://) flow-node 를 만든다.
func remoteFlowNode(id, name, instanceID, remoteFlowID string, inputs, outputs []flow.Port) flow.NodeDef {
	return flowNode(id, name, "remote://"+instanceID+"/"+remoteFlowID, inputs, outputs)
}

// ===========================================================================
// remote:// 파싱 단위 테스트
// ===========================================================================

func TestParseRemoteFlowRef(t *testing.T) {
	tests := []struct {
		name       string
		flowID     string
		wantRemote bool
		wantInst   string
		wantFlow   string
		wantErr    bool
	}{
		{name: "bare id 는 로컬", flowID: "flow-abc123", wantRemote: false},
		{name: "정규형 원격", flowID: "remote://node-7f3a/flow-abc123", wantRemote: true, wantInst: "node-7f3a", wantFlow: "flow-abc123"},
		{name: "flow id 에 슬래시 포함", flowID: "remote://node-1/a/b/c", wantRemote: true, wantInst: "node-1", wantFlow: "a/b/c"},
		{name: "스킴만 → 에러", flowID: "remote://", wantRemote: true, wantErr: true},
		{name: "instance 만, 슬래시 없음 → 에러", flowID: "remote://node-1", wantRemote: true, wantErr: true},
		{name: "flow id 비어있음 → 에러", flowID: "remote://node-1/", wantRemote: true, wantErr: true},
		{name: "instance 비어있음 → 에러", flowID: "remote:///flow-x", wantRemote: true, wantErr: true},
		{name: "빈 문자열은 로컬(빈)", flowID: "", wantRemote: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inst, fid, isRemote, err := parseRemoteFlowRef(tc.flowID)
			assert.Equal(t, tc.wantRemote, isRemote, "isRemote")
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.wantRemote {
				assert.Equal(t, tc.wantInst, inst)
				assert.Equal(t, tc.wantFlow, fid)
			}
		})
	}
}

// ===========================================================================
// 원격 확장 — 인라인 확장(로컬과 동일 규칙)
// ===========================================================================

// 원격 참조 flow-node 가 로컬 서브플로우처럼 네임스페이스 인라인 확장되는지 검증한다
// (REQ-SUBFLOW-R02/R04).
func TestExpandSubflowsWithFetcher_원격참조_인라인확장(t *testing.T) {
	repo := newFakeFlowRepo()
	fetcher := newFakeRemoteFetcher()
	fetcher.put("node-1", "flow-x",
		remoteFlowGetBytes(t, "flow-x", "원격플로우", reactFlowConfigPassthrough()))

	// 부모 B: src → flow-node(remote://node-1/flow-x) → sink
	src := passthroughNode("src", "소스")
	sink := passthroughNode("sink", "싱크")
	fn := remoteFlowNode("fn1", "원격서브", "node-1", "flow-x",
		[]flow.Port{inPort("X")}, []flow.Port{outPort("Y")})
	parent := flow.NewFlow("B",
		flow.WithNodes(src, fn, sink),
		flow.WithWires(
			wire("p_in", "src", "out", "fn1", "X"),
			wire("p_out", "fn1", "Y", "sink", "in"),
		),
	)

	got, err := ExpandSubflowsWithFetcher(context.Background(), parent, repo, fetcher)
	require.NoError(t, err)

	assert.Equal(t, 1, fetcher.calls, "원격 정의는 fetch 되어야 한다")

	// flow-node 타입이 남으면 안 된다.
	for _, n := range got.Nodes() {
		assert.NotEqual(t, flowNodeType, n.Type)
	}
	// 네임스페이스 노드 존재: subflow_fn1_A
	_, ok := nodeByID(got.Nodes(), "subflow_fn1_A")
	assert.True(t, ok, "원격 서브플로우 내부 노드가 네임스페이스로 확장되어야 한다")

	// 재배선: src.out → subflow_fn1_A.in, subflow_fn1_A.out → sink.in
	assert.True(t, hasWire(got.Wires(), "src", "out", "subflow_fn1_A", "in"),
		"입력 경계가 내부 소비자로 재배선되어야 한다")
	assert.True(t, hasWire(got.Wires(), "subflow_fn1_A", "out", "sink", "in"),
		"출력 경계가 부모 하류로 재배선되어야 한다")
}

// nil fetcher + 원격 참조 → 배포 거부(REQ-SUBFLOW-R07).
func TestExpandSubflowsWithFetcher_해석기부재_거부(t *testing.T) {
	repo := newFakeFlowRepo()
	fn := remoteFlowNode("fn1", "원격서브", "node-1", "flow-x",
		[]flow.Port{inPort("X")}, []flow.Port{outPort("Y")})
	parent := flow.NewFlow("B", flow.WithNodes(fn))

	// nil fetcher.
	_, err := ExpandSubflowsWithFetcher(context.Background(), parent, repo, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "서버 모드")

	// 기존 3-인자 ExpandSubflows(=nil fetcher) 도 동일하게 거부해야 한다.
	_, err2 := ExpandSubflows(context.Background(), parent, repo)
	require.Error(t, err2)
}

// fetcher 에러(오프라인/누락) → 배포 거부, stale/empty 무음 사용 금지(REQ-SUBFLOW-R06).
func TestExpandSubflowsWithFetcher_fetch실패_배포거부(t *testing.T) {
	repo := newFakeFlowRepo()
	fetcher := newFakeRemoteFetcher()
	fetcher.err = errors.New("remote: target node is not managed (not approved or offline)")

	fn := remoteFlowNode("fn1", "원격서브", "node-1", "flow-x",
		[]flow.Port{inPort("X")}, []flow.Port{outPort("Y")})
	parent := flow.NewFlow("B", flow.WithNodes(fn))

	got, err := ExpandSubflowsWithFetcher(context.Background(), parent, repo, fetcher)
	require.Error(t, err)
	assert.Nil(t, got, "실패 시 stale/empty 플로우를 반환하면 안 된다")
	// 에러에 어느 flow-node/어느 flow_id 인지 식별 정보가 있어야 한다(실행 가능한 에러).
	assert.Contains(t, err.Error(), "fn1")
	assert.Contains(t, err.Error(), "node-1")
}

// 중첩 원격 참조(fetch 된 정의가 내부에 flow-node 포함) → 거부(REQ-SUBFLOW-R08).
func TestExpandSubflowsWithFetcher_중첩flow노드_거부(t *testing.T) {
	repo := newFakeFlowRepo()
	fetcher := newFakeRemoteFetcher()
	fetcher.put("node-1", "flow-nested",
		remoteFlowGetBytes(t, "flow-nested", "중첩원격", reactFlowConfigWithFlowNode()))

	fn := remoteFlowNode("fn1", "원격서브", "node-1", "flow-nested",
		[]flow.Port{inPort("X")}, []flow.Port{outPort("Y")})
	parent := flow.NewFlow("B", flow.WithNodes(fn))

	_, err := ExpandSubflowsWithFetcher(context.Background(), parent, repo, fetcher)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "중첩")
}

// bare id 로컬 참조는 fetcher 가 있어도 repo.Get 으로 해석되어야 한다(하위 호환, 회귀 0).
func TestExpandSubflowsWithFetcher_로컬참조_repoGet사용(t *testing.T) {
	repo := newFakeFlowRepo()
	fetcher := newFakeRemoteFetcher() // 비어 있음 — 호출되면 안 된다.

	// 로컬 참조 플로우 F.
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

	got, err := ExpandSubflowsWithFetcher(context.Background(), parent, repo, fetcher)
	require.NoError(t, err)
	assert.Equal(t, 0, fetcher.calls, "로컬 bare id 참조는 fetcher 를 호출하면 안 된다")
	_, ok := nodeByID(got.Nodes(), "subflow_fn1_A")
	assert.True(t, ok)
}

// 원격 정의 역직렬화 단위 테스트: flow/get 응답(FlowInfo 바이트) → flow.Flow.
func TestDeserializeRemoteFlow(t *testing.T) {
	data := remoteFlowGetBytes(t, "flow-x", "원격플로우", reactFlowConfigPassthrough())
	f, err := deserializeRemoteFlow(data, "node-1", "flow-x")
	require.NoError(t, err)
	require.NotNil(t, f)

	// 내부 노드 A 가 transform 타입으로 복원되어야 한다.
	a, ok := nodeByID(f.Nodes(), "A")
	require.True(t, ok)
	assert.Equal(t, "transform", a.Type)

	// 플로우 레벨 포트(inputs/outputs)가 복원되어야 한다.
	require.Len(t, f.Inputs(), 1)
	require.Len(t, f.Outputs(), 1)
	assert.Equal(t, "X", f.Inputs()[0].Name)
	assert.Equal(t, "Y", f.Outputs()[0].Name)

	// 경계 와이어가 보존되어야 한다(instantiateSubflow 가 inMap/outMap 구성에 사용).
	hasBoundaryIn := false
	hasBoundaryOut := false
	for _, w := range f.Wires() {
		if w.SourceNodeID == flow.FlowInputBoundaryID {
			hasBoundaryIn = true
		}
		if w.TargetNodeID == flow.FlowOutputBoundaryID {
			hasBoundaryOut = true
		}
	}
	assert.True(t, hasBoundaryIn, "입력 경계 와이어가 보존되어야 한다")
	assert.True(t, hasBoundaryOut, "출력 경계 와이어가 보존되어야 한다")
}

// ===========================================================================
// Reproduction Test: Remote Subflow Output Port Rewiring
// ===========================================================================

// remoteFlowConfigWithBoundaryOutput creates a React Flow config that mimics
// the real "Serial" flow: [serial-in, tcp-out, serial-out, tcp-in] with
// a boundary output wire from serial-in to __flow_output__.
// This matches the real scenario described in the issue.
func remoteFlowConfigWithBoundaryOutput() map[string]any {
	return map[string]any{
		"nodes": []map[string]any{
			{
				"id":       "serial-in",
				"type":     "custom",
				"position": map[string]any{"x": 0, "y": 0},
				"data": map[string]any{
					"label":    "Serial Input",
					"nodeType": "custom", // device/transport node
					"ports": []map[string]any{
						{"name": "out", "direction": "output"},
					},
				},
			},
			{
				"id":       "tcp-out",
				"type":     "custom",
				"position": map[string]any{"x": 100, "y": 0},
				"data": map[string]any{
					"label":    "TCP Output",
					"nodeType": "custom",
					"ports": []map[string]any{
						{"name": "in", "direction": "input"},
					},
				},
			},
			{
				"id":       "serial-out",
				"type":     "custom",
				"position": map[string]any{"x": 0, "y": 100},
				"data": map[string]any{
					"label":    "Serial Output",
					"nodeType": "custom",
					"ports": []map[string]any{
						{"name": "in", "direction": "input"},
					},
				},
			},
			{
				"id":       "tcp-in",
				"type":     "custom",
				"position": map[string]any{"x": 100, "y": 100},
				"data": map[string]any{
					"label":    "TCP Input",
					"nodeType": "custom",
					"ports": []map[string]any{
						{"name": "out", "direction": "output"},
					},
				},
			},
		},
		"edges": []map[string]any{
			// BOUNDARY OUTPUT: serial-in.out → flow output port "out1"
			{"id": "bo", "source": "serial-in", "target": flow.FlowOutputBoundaryID, "sourceHandle": "out", "targetHandle": "out1"},
			// Internal wires (not involved in boundary)
			{"id": "internal1", "source": "serial-in", "target": "tcp-out", "sourceHandle": "out", "targetHandle": "in"},
			{"id": "internal2", "source": "tcp-in", "target": "serial-out", "sourceHandle": "out", "targetHandle": "in"},
		},
		"inputs":  []map[string]any{}, // No input ports
		"outputs": []map[string]any{{"id": "out1", "name": "out1", "direction": "output"}},
	}
}

// TestExpandSubflowsWithFetcher_RemoteOutputPortRewiring_H1Check is the reproduction test
// for hypothesis H1 (expansion bug). It verifies that a remote flow's boundary output wire
// is correctly rewired to the parent's downstream node after expansion.
//
// Setup:
//   - Remote flow: [serial-in, tcp-out, serial-out, tcp-in] with boundary output
//     serial-in.out → __flow_output__ (port name 'out1')
//   - Local parent: src → flow-node(remote://node-1/serial) → sink
//
// Expected behavior (H1 NOT present):
//   - After expansion, the flow-node's output port 'out1' should be rewired
//     to the internal producer (serial-in)
//   - Parent output wire: flow-node('out1') → sink('in') should become:
//     serial-in('out') → sink('in') via namespaced rewiring
//   - Result: src.out → subflow_<fn_id>_serial-in.out → sink.in
//
// If H1 is present (expansion bug):
//   - The boundary output wire is lost (outMap['out1'] is empty)
//   - The parent output wire is dropped as a DanglingWire
//   - The expanded flow has NO wire connecting the remote source to the parent sink
//   - This reproduces the "no output" symptom
func TestExpandSubflowsWithFetcher_RemoteOutputPortRewiring_H1Check(t *testing.T) {
	repo := newFakeFlowRepo()
	fetcher := newFakeRemoteFetcher()

	// Register the remote flow with boundary output config
	remoteConfig := remoteFlowConfigWithBoundaryOutput()
	fetcher.put("node-1", "serial",
		remoteFlowGetBytes(t, "serial", "Remote Serial Flow", remoteConfig))

	// Local parent flow: src → flow-node(remote://node-1/serial) → sink
	src := passthroughNode("src", "Local Source")
	sink := passthroughNode("sink", "Local Sink")
	// Flow-node with output port 'out1' matching the remote flow's output port
	remoteFlowNode := remoteFlowNode("fn", "Remote Subflow", "node-1", "serial",
		[]flow.Port{}, // no inputs
		[]flow.Port{outPort("out1")})
	parent := flow.NewFlow("parent",
		flow.WithNodes(src, remoteFlowNode, sink),
		flow.WithWires(
			wire("p_in", "src", "out", "fn", "out1"),  // src → flow-node input
			wire("p_out", "fn", "out1", "sink", "in"), // flow-node output → sink
		),
	)

	// Expand with fetcher
	expanded, err := ExpandSubflowsWithFetcher(context.Background(), parent, repo, fetcher)
	require.NoError(t, err, "expansion should succeed")
	require.NotNil(t, expanded)

	// --- H1 Check: Output Port Rewiring ---
	// After expansion, the remote flow-node's output port 'out1' should be
	// connected to the internal producer (serial-in).

	t.Logf("Expanded flow nodes: %d", len(expanded.Nodes()))
	for _, n := range expanded.Nodes() {
		t.Logf("  - %s (type=%s)", n.ID, n.Type)
	}

	t.Logf("Expanded flow wires: %d", len(expanded.Wires()))
	for _, w := range expanded.Wires() {
		t.Logf("  - %s: %s.%s -> %s.%s", w.ID, w.SourceNodeID, w.SourcePort, w.TargetNodeID, w.TargetPort)
	}

	// Expected wires after expansion:
	// 1. src.out → subflow_fn_serial-in.in (passthrough input, should NOT exist since no input ports)
	// 2. subflow_fn_serial-in.out → sink.in (parent output wire rewired to internal producer)

	// Key assertion: Check if the parent output wire was correctly rewired
	// to the internal producer (serial-in), NOT dropped as dangling.
	hasOutputWireToSink := false
	for _, w := range expanded.Wires() {
		// After expansion, the output wire should have:
		// - Source: the namespaced serial-in node
		// - SourcePort: "out"
		// - Target: sink
		// - TargetPort: "in"
		if strings.HasPrefix(w.SourceNodeID, "subflow_fn_") && w.SourceNodeID == "subflow_fn_serial-in" &&
			w.SourcePort == "out" && w.TargetNodeID == "sink" && w.TargetPort == "in" {
			hasOutputWireToSink = true
			t.Logf("SUCCESS: Found correctly rewired output wire: %s", w.ID)
			break
		}
	}

	if !hasOutputWireToSink {
		t.Log("FAILURE (H1 detected): Output wire was NOT correctly rewired to internal producer")
		t.Log("This indicates the remote flow's boundary output was lost during deserialization")
		t.Log("or the outMap was not correctly populated during instantiateSubflow")
	}

	assert.True(t, hasOutputWireToSink,
		"remote subflow output port should be rewired to internal producer (serial-in),\n"+
			"expected wire: subflow_fn_serial-in.out → sink.in")

	// Also verify flow-node type is gone
	for _, n := range expanded.Nodes() {
		assert.NotEqual(t, flowNodeType, n.Type,
			"expanded flow should not contain flow-node types")
	}
}

// TestExpandSubflowsWithFetcher_LocalVsRemoteComparison compares local and remote
// subflow expansion to detect divergence. If local works but remote doesn't,
// it confirms H1 (deserialization bug in remote path).
func TestExpandSubflowsWithFetcher_LocalVsRemoteComparison(t *testing.T) {
	// --- Setup local reference flow ---
	localRef := flow.NewFlow("serial",
		flow.WithNodes(
			flow.NodeDef{
				ID:      "serial-in",
				Name:    "Serial Input",
				Type:    "custom",
				Outputs: []flow.Port{outPort("out")},
			},
			flow.NodeDef{
				ID:     "tcp-out",
				Name:   "TCP Output",
				Type:   "custom",
				Inputs: []flow.Port{inPort("in")},
			},
			flow.NodeDef{
				ID:     "serial-out",
				Name:   "Serial Output",
				Type:   "custom",
				Inputs: []flow.Port{inPort("in")},
			},
			flow.NodeDef{
				ID:      "tcp-in",
				Name:    "TCP Input",
				Type:    "custom",
				Outputs: []flow.Port{outPort("out")},
			},
		),
		flow.WithWires(
			// Boundary output: serial-in → flow output (out1)
			wire("bo", "serial-in", "out", flow.FlowOutputBoundaryID, "out1"),
			// Internal wires
			wire("internal1", "serial-in", "out", "tcp-out", "in"),
			wire("internal2", "tcp-in", "out", "serial-out", "in"),
		),
		flow.WithFlowOutputPorts(outPort("out1")),
	)

	// --- Setup local parent ---
	localRepo := newFakeFlowRepo()
	localRepo.put(localRef)

	localSrc := passthroughNode("src", "Source")
	localSink := passthroughNode("sink", "Sink")
	localFlowNode := flowNode("fn", "Local Subflow", localRef.ID(),
		[]flow.Port{}, []flow.Port{outPort("out1")})
	localParent := flow.NewFlow("parent",
		flow.WithNodes(localSrc, localFlowNode, localSink),
		flow.WithWires(
			wire("p_out", "fn", "out1", "sink", "in"),
		),
	)

	// Expand local
	localExpanded, err := ExpandSubflows(context.Background(), localParent, localRepo)
	require.NoError(t, err)

	// --- Setup remote reference flow (same structure) ---
	remoteRepo := newFakeFlowRepo()
	remoteFetcher := newFakeRemoteFetcher()
	remoteFetcher.put("node-1", "serial",
		remoteFlowGetBytes(t, "serial", "Remote Serial Flow",
			remoteFlowConfigWithBoundaryOutput()))

	// --- Setup remote parent (same structure as local) ---
	remoteSrc := passthroughNode("src", "Source")
	remoteSink := passthroughNode("sink", "Sink")
	remoteFlowNode := remoteFlowNode("fn", "Remote Subflow", "node-1", "serial",
		[]flow.Port{}, []flow.Port{outPort("out1")})
	remoteParent := flow.NewFlow("parent",
		flow.WithNodes(remoteSrc, remoteFlowNode, remoteSink),
		flow.WithWires(
			wire("p_out", "fn", "out1", "sink", "in"),
		),
	)

	// Expand remote
	remoteExpanded, err := ExpandSubflowsWithFetcher(context.Background(), remoteParent, remoteRepo, remoteFetcher)
	require.NoError(t, err)

	// --- Compare ---
	localWireCount := len(localExpanded.Wires())
	remoteWireCount := len(remoteExpanded.Wires())

	t.Logf("Local expansion: %d wires, %d nodes", localWireCount, len(localExpanded.Nodes()))
	t.Logf("Remote expansion: %d wires, %d nodes", remoteWireCount, len(remoteExpanded.Nodes()))

	// The critical check: both should have the parent output wire rewired
	// to the internal producer (serial-in).
	localHasOutput := hasWire(localExpanded.Wires(), "subflow_fn_serial-in", "out", "sink", "in")
	remoteHasOutput := hasWire(remoteExpanded.Wires(), "subflow_fn_serial-in", "out", "sink", "in")

	t.Logf("Local has output wire (serial-in→sink): %v", localHasOutput)
	t.Logf("Remote has output wire (serial-in→sink): %v", remoteHasOutput)

	if localHasOutput && !remoteHasOutput {
		t.Error("CRITICAL DIVERGENCE DETECTED (H1 confirmed):\n" +
			"Local expansion preserves output wire, but remote expansion drops it.\n" +
			"This indicates a deserialization or boundary handling bug in the remote path.")
	}

	assert.Equal(t, localHasOutput, remoteHasOutput,
		"local and remote subflow expansion should behave identically for the same logical flow")
}
