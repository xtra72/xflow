package service

import (
	"context"
	"encoding/json"
	"errors"
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
