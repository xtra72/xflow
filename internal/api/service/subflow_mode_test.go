// subflow_mode_test.go 는 flow-node mode(shared|instance) 정규화와 배포 분기(ExpandSubflows)를
// 검증한다(@SPEC:SPEC-SUBFLOW-002 그룹 M/SH/IN/MG, REQ-SUBFLOW2-M02/M03/SH01/IN01/IN03/MG01/MG02).
package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
)

// flowNodeShared 는 mode=shared(또는 미지정 — 인자로 제어) LOCAL flow-node 를 만든다.
func flowNodeShared(id, name, refFlowID string, inputs, outputs []flow.Port, explicit bool) flow.NodeDef {
	cfg := map[string]any{flowNodeFlowIDKey: refFlowID}
	if explicit {
		cfg[flowNodeModeKey] = flowModeShared
	}
	return flow.NodeDef{
		ID:      id,
		Name:    name,
		Type:    flowNodeType,
		Config:  cfg,
		Inputs:  inputs,
		Outputs: outputs,
	}
}

// TestNormalizeFlowNodeMode 는 mode 정규화 규칙을 검증한다(M02 미지정→shared, M03 결정성).
func TestNormalizeFlowNodeMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  map[string]any
		want string
	}{
		{"nil config → shared", nil, flowModeShared},
		{"미지정 → shared(M02 breaking)", map[string]any{flowNodeFlowIDKey: "x"}, flowModeShared},
		{"빈 문자열 → shared", map[string]any{flowNodeModeKey: ""}, flowModeShared},
		{"shared 명시", map[string]any{flowNodeModeKey: "shared"}, flowModeShared},
		{"instance 명시", map[string]any{flowNodeModeKey: "instance"}, flowModeInstance},
		{"대문자 INSTANCE → instance", map[string]any{flowNodeModeKey: "INSTANCE"}, flowModeInstance},
		{"공백 포함 ' instance ' → instance", map[string]any{flowNodeModeKey: " instance "}, flowModeInstance},
		{"알 수 없는 값 → shared(M03)", map[string]any{flowNodeModeKey: "bogus"}, flowModeShared},
		{"비문자열 → shared", map[string]any{flowNodeModeKey: 42}, flowModeShared},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, normalizeFlowNodeMode(tc.cfg))
		})
	}
}

// sharedRefFlow 는 입력 X / 출력 Y 경계 포트를 가진 참조 플로우를 만든다(테스트 헬퍼).
func sharedRefFlow(t *testing.T, id string) flow.Flow {
	t.Helper()
	return flow.NewFlow(id,
		flow.WithNodes(passthroughNode("A", "노드A")),
		flow.WithWires(
			wire("bi", flow.FlowInputBoundaryID, "X", "A", "in"),
			wire("bo", "A", "out", flow.FlowOutputBoundaryID, "Y"),
		),
		flow.WithFlowInputPorts(inPort("X")),
		flow.WithFlowOutputPorts(outPort("Y")),
	)
}

// TestExpandSubflows_shared_미확장_라이브노드보존 은 mode=shared LOCAL flow-node 가 인라인
// 확장되지 않고 LIVE NODE 로 보존되는지 검증한다(SH01).
func TestExpandSubflows_shared_미확장_라이브노드보존(t *testing.T) {
	t.Parallel()
	repo := newFakeFlowRepo()
	ref := sharedRefFlow(t, "F")
	repo.put(ref)

	fn := flowNodeShared("fn1", "공유서브", ref.ID(), []flow.Port{inPort("X")}, []flow.Port{outPort("Y")}, true)
	parent := flow.NewFlow("B",
		flow.WithNodes(passthroughNode("src", "소스"), fn, passthroughNode("sink", "싱크")),
		flow.WithWires(
			wire("p_in", "src", "out", "fn1", "X"),
			wire("p_out", "fn1", "Y", "sink", "in"),
		),
	)

	got, err := ExpandSubflows(context.Background(), parent, repo)
	require.NoError(t, err)

	// flow-node 가 LIVE NODE 로 살아남아야 한다(미확장).
	survivor, ok := nodeByID(got.Nodes(), "fn1")
	require.True(t, ok, "shared flow-node 는 미확장 LIVE NODE 로 보존되어야 한다(SH01)")
	assert.Equal(t, flowNodeType, survivor.Type)

	// 네임스페이스 노드가 생성되지 않아야 한다(복사본 없음).
	_, expanded := nodeByID(got.Nodes(), "subflow_fn1_A")
	assert.False(t, expanded, "shared 는 인라인 확장하지 않아야 한다(복사본 없음)")
}

// TestExpandSubflows_미지정_shared로_동작 은 mode 미지정 LOCAL flow-node 가 shared 로
// 동작(미확장)하는지 검증한다(MG01 — breaking 마이그레이션).
func TestExpandSubflows_미지정_shared로_동작(t *testing.T) {
	t.Parallel()
	repo := newFakeFlowRepo()
	ref := sharedRefFlow(t, "F")
	repo.put(ref)

	// explicit=false → config 에 mode 키 없음(SPEC-SUBFLOW-001 시절 저장 정의 모사).
	fn := flowNodeShared("fn1", "레거시", ref.ID(), []flow.Port{inPort("X")}, []flow.Port{outPort("Y")}, false)
	parent := flow.NewFlow("B",
		flow.WithNodes(passthroughNode("src", "소스"), fn, passthroughNode("sink", "싱크")),
		flow.WithWires(
			wire("p_in", "src", "out", "fn1", "X"),
			wire("p_out", "fn1", "Y", "sink", "in"),
		),
	)

	got, err := ExpandSubflows(context.Background(), parent, repo)
	require.NoError(t, err)

	survivor, ok := nodeByID(got.Nodes(), "fn1")
	require.True(t, ok, "mode 미지정은 shared 로 동작하여 미확장 보존되어야 한다(MG01)")
	assert.Equal(t, flowNodeType, survivor.Type)
	_, expanded := nodeByID(got.Nodes(), "subflow_fn1_A")
	assert.False(t, expanded, "미지정(=shared)은 인라인 확장하지 않아야 한다")
}

// TestExpandSubflows_혼합모드_각각처리 은 한 부모의 shared/instance flow-node 가 각각 분기
// 처리되는지 검증한다(IN03 — shared 미확장, instance 확장, 상호 비간섭).
func TestExpandSubflows_혼합모드_각각처리(t *testing.T) {
	t.Parallel()
	repo := newFakeFlowRepo()
	sharedRef := sharedRefFlow(t, "S")
	instanceRef := sharedRefFlow(t, "I")
	repo.put(sharedRef)
	repo.put(instanceRef)

	sharedFN := flowNodeShared("sh1", "공유", sharedRef.ID(), []flow.Port{inPort("X")}, []flow.Port{outPort("Y")}, true)
	instanceFN := flowNode("in1", "인스턴스", instanceRef.ID(), []flow.Port{inPort("X")}, []flow.Port{outPort("Y")})

	parent := flow.NewFlow("B",
		flow.WithNodes(passthroughNode("src", "소스"), sharedFN, instanceFN, passthroughNode("sink", "싱크")),
		flow.WithWires(
			wire("w1", "src", "out", "sh1", "X"),
			wire("w2", "sh1", "Y", "in1", "X"),
			wire("w3", "in1", "Y", "sink", "in"),
		),
	)

	got, err := ExpandSubflows(context.Background(), parent, repo)
	require.NoError(t, err)

	// shared 는 보존.
	_, sharedSurvives := nodeByID(got.Nodes(), "sh1")
	assert.True(t, sharedSurvives, "shared flow-node 는 미확장 보존(SH01)")

	// instance 는 확장(복사본 생성, flow-node 소멸).
	_, instanceExpanded := nodeByID(got.Nodes(), "subflow_in1_A")
	assert.True(t, instanceExpanded, "instance flow-node 는 인라인 확장(IN01)")
	_, instanceSurvives := nodeByID(got.Nodes(), "in1")
	assert.False(t, instanceSurvives, "instance flow-node 는 확장 후 소멸")
}
