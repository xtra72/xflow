package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/pkg/flow"
)

// ---------------------------------------------------------------------------
// SPEC-SUBFLOW-001 v1.3 그룹 RB — 참조 종류별 분기(reference-kind branch)
// ---------------------------------------------------------------------------
//
// v1.3 SUPERSEDE: v1.2 의 "원격 참조 = 배포 시 fetch + 매니저 인라인 확장"은 폐기되었다
// (device/secret 무동작 한계 — §1.2 결정 5). 원격 참조 flow-node 는 이제 라이브 브리지로
// 동작하므로, ExpandSubflows 는 원격 참조를 확장하지 않고 flow-node 를 LIVE NODE 로
// 그대로 남긴다(REQ-SUBFLOW-RB01/RB07). LOCAL bare-id 참조는 기존대로 인라인 확장한다
// (불변, regression-0 — 결정 1/그룹 D).
//
// 아래 테스트는 NEW 동작을 검증한다(retired: fetcher 기반 인라인 확장 테스트).

// remoteFlowNode 는 원격 참조(remote://) flow-node 를 만든다.
func remoteFlowNode(id, name, instanceID, remoteFlowID string, inputs, outputs []flow.Port) flow.NodeDef {
	return flowNode(id, name, "remote://"+instanceID+"/"+remoteFlowID, inputs, outputs)
}

// ===========================================================================
// remote:// 파싱 단위 테스트 (REQ-SUBFLOW-R01 — v1.3 보존)
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
// 참조 종류별 분기: 원격 = 라이브 노드 유지(미확장) / 로컬 = 인라인 확장
// ===========================================================================

// 원격 참조 flow-node 는 확장되지 않고 LIVE NODE 로 그대로 남아야 한다
// (REQ-SUBFLOW-RB01/RB07). flow_id 의 remote:// 참조와 입출력 포트가 보존되어,
// P3 엔진이 이를 브리지 엔드포인트로 실행할 수 있어야 한다.
func TestExpandSubflows_원격참조_미확장_라이브노드유지(t *testing.T) {
	repo := newFakeFlowRepo()

	// 부모: src → flow-node(remote://node-1/flow-x) → sink
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

	got, err := ExpandSubflows(context.Background(), parent, repo)
	require.NoError(t, err)

	// 원격 flow-node 는 출력에 그대로 살아 있어야 한다(미확장).
	survivor, ok := nodeByID(got.Nodes(), "fn1")
	require.True(t, ok, "원격 참조 flow-node 는 확장되지 않고 그대로 남아야 한다")
	assert.Equal(t, flowNodeType, survivor.Type, "타입은 flow-node 로 유지")

	// 참조(remote:// ref)가 보존되어야 한다 — P3 가 instance_id/remote_flow_id 를 분해한다.
	assert.Equal(t, "remote://node-1/flow-x", survivor.Config[flowNodeFlowIDKey],
		"원격 참조 flow_id 가 보존되어야 한다")

	// 입출력 포트가 보존되어야 한다 — P3 경계 포트 매핑(이름 기반, RB06)에 사용된다.
	require.Len(t, survivor.Inputs, 1)
	require.Len(t, survivor.Outputs, 1)
	assert.Equal(t, "X", survivor.Inputs[0].Name)
	assert.Equal(t, "Y", survivor.Outputs[0].Name)

	// 네임스페이스 확장 노드(subflow_fn1_*)가 생기면 안 된다(미확장).
	for _, n := range got.Nodes() {
		assert.NotContains(t, n.ID, "subflow_fn1_", "원격 참조는 네임스페이스 확장되지 않아야 한다")
	}

	// 부모 와이어가 살아남은 flow-node 핸들을 그대로 가리켜야 한다(재배선 없음).
	assert.True(t, hasWire(got.Wires(), "src", "out", "fn1", "X"),
		"입력 와이어가 살아남은 flow-node 입력 핸들을 가리켜야 한다")
	assert.True(t, hasWire(got.Wires(), "fn1", "Y", "sink", "in"),
		"출력 와이어가 살아남은 flow-node 출력 핸들에서 출발해야 한다")
}

// 로컬 bare-id 참조는 기존대로 인라인 확장되어야 한다(불변, regression-0).
func TestExpandSubflows_로컬참조_인라인확장_불변(t *testing.T) {
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

	src := passthroughNode("src", "소스")
	sink := passthroughNode("sink", "싱크")
	fn := flowNode("fn1", "로컬서브", ref.ID(), []flow.Port{inPort("X")}, []flow.Port{outPort("Y")})
	parent := flow.NewFlow("B",
		flow.WithNodes(src, fn, sink),
		flow.WithWires(
			wire("p_in", "src", "out", "fn1", "X"),
			wire("p_out", "fn1", "Y", "sink", "in"),
		),
	)

	got, err := ExpandSubflows(context.Background(), parent, repo)
	require.NoError(t, err)

	// flow-node 타입이 남으면 안 된다(확장됨).
	for _, n := range got.Nodes() {
		assert.NotEqual(t, flowNodeType, n.Type, "로컬 참조는 확장되어 flow-node 가 남지 않아야 한다")
	}
	// 네임스페이스 노드 존재.
	_, ok := nodeByID(got.Nodes(), "subflow_fn1_A")
	assert.True(t, ok, "로컬 서브플로우 내부 노드가 네임스페이스로 확장되어야 한다")
	// 재배선.
	assert.True(t, hasWire(got.Wires(), "src", "out", "subflow_fn1_A", "in"))
	assert.True(t, hasWire(got.Wires(), "subflow_fn1_A", "out", "sink", "in"))
}

// 로컬 + 원격 flow-node 가 한 부모에 공존하면, 로컬은 확장되고 원격은 살아남아야 한다
// (REQ-SUBFLOW-RB01 — 두 모드 공존).
func TestExpandSubflows_로컬과원격_공존_각각처리(t *testing.T) {
	repo := newFakeFlowRepo()

	localRef := flow.NewFlow("L",
		flow.WithNodes(passthroughNode("A", "노드A")),
		flow.WithWires(
			wire("bi", flow.FlowInputBoundaryID, "X", "A", "in"),
			wire("bo", "A", "out", flow.FlowOutputBoundaryID, "Y"),
		),
		flow.WithFlowInputPorts(inPort("X")),
		flow.WithFlowOutputPorts(outPort("Y")),
	)
	repo.put(localRef)

	src := passthroughNode("src", "소스")
	mid := passthroughNode("mid", "중간")
	sink := passthroughNode("sink", "싱크")
	localFN := flowNode("local1", "로컬서브", localRef.ID(), []flow.Port{inPort("X")}, []flow.Port{outPort("Y")})
	remoteFN := remoteFlowNode("remote1", "원격서브", "node-9", "flow-r",
		[]flow.Port{inPort("X")}, []flow.Port{outPort("Y")})

	parent := flow.NewFlow("B",
		flow.WithNodes(src, localFN, mid, remoteFN, sink),
		flow.WithWires(
			wire("w1", "src", "out", "local1", "X"),  // src → 로컬 flow-node
			wire("w2", "local1", "Y", "mid", "in"),   // 로컬 flow-node → mid
			wire("w3", "mid", "out", "remote1", "X"), // mid → 원격 flow-node
			wire("w4", "remote1", "Y", "sink", "in"), // 원격 flow-node → sink
		),
	)

	got, err := ExpandSubflows(context.Background(), parent, repo)
	require.NoError(t, err)

	// 로컬은 확장: subflow_local1_A 존재, local1 flow-node 사라짐.
	_, localExpanded := nodeByID(got.Nodes(), "subflow_local1_A")
	assert.True(t, localExpanded, "로컬 flow-node 는 확장되어야 한다")
	_, localSurvives := nodeByID(got.Nodes(), "local1")
	assert.False(t, localSurvives, "로컬 flow-node 는 확장 후 사라져야 한다")

	// 원격은 생존: remote1 flow-node 그대로, 참조/포트 보존.
	remoteSurvivor, remoteSurvives := nodeByID(got.Nodes(), "remote1")
	require.True(t, remoteSurvives, "원격 flow-node 는 살아남아야 한다")
	assert.Equal(t, flowNodeType, remoteSurvivor.Type)
	assert.Equal(t, "remote://node-9/flow-r", remoteSurvivor.Config[flowNodeFlowIDKey])
	require.Len(t, remoteSurvivor.Inputs, 1)
	require.Len(t, remoteSurvivor.Outputs, 1)

	// 로컬 측 재배선: src.out → subflow_local1_A.in, subflow_local1_A.out → mid.in.
	assert.True(t, hasWire(got.Wires(), "src", "out", "subflow_local1_A", "in"))
	assert.True(t, hasWire(got.Wires(), "subflow_local1_A", "out", "mid", "in"))

	// 원격 경계 와이어는 살아남은 flow-node 를 그대로 가리켜야 한다.
	assert.True(t, hasWire(got.Wires(), "mid", "out", "remote1", "X"),
		"mid → 원격 flow-node 입력 와이어 보존")
	assert.True(t, hasWire(got.Wires(), "remote1", "Y", "sink", "in"),
		"원격 flow-node → sink 출력 와이어 보존")
}

// 원격 참조만 있고 로컬 참조가 없는 플로우도 거부 없이 통과해야 한다(원격은 생존).
func TestExpandSubflows_원격참조전용_거부없음(t *testing.T) {
	repo := newFakeFlowRepo()
	fn := remoteFlowNode("fn1", "원격서브", "node-1", "flow-x",
		[]flow.Port{inPort("X")}, []flow.Port{outPort("Y")})
	parent := flow.NewFlow("B", flow.WithNodes(fn))

	got, err := ExpandSubflows(context.Background(), parent, repo)
	require.NoError(t, err, "원격 참조 전용 플로우는 배포 거부되지 않아야 한다(브리지로 처리)")
	survivor, ok := nodeByID(got.Nodes(), "fn1")
	require.True(t, ok)
	assert.Equal(t, flowNodeType, survivor.Type)
}

// 잘못된 remote:// 형식은 배포 거부(명확한 에러).
func TestExpandSubflows_원격참조_형식오류_거부(t *testing.T) {
	repo := newFakeFlowRepo()
	fn := flowNode("fn1", "잘못된원격", "remote://node-1", nil, nil) // 슬래시 누락
	parent := flow.NewFlow("B", flow.WithNodes(fn))

	_, err := ExpandSubflows(context.Background(), parent, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fn1")
}
