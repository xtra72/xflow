// shared_boundary_multiport_test.go 는 다중 출력/입력 경계 포트를 가진 shared 참조 플로우의
// 포트별 라우팅을 검증한다(@SPEC:SPEC-SUBFLOW-002 그룹 SH/MR — 다중 출력 포트 보강).
//
// 배경(M3 한계 보강):
//
//	M3 e2e 는 단일 출력 경계 포트(Y)로만 검증했다. 참조 플로우가 출력 경계 포트를 여러 개
//	(out1/out2) 가질 때, 내부의 서로 다른 노드가 각 포트로 방출하면 각 메시지가 "올바른"
//	출력 경계 포트로만 전달되어야 한다(교차·누락·중복 없음). 본 테스트는 그 포트별 fan-out
//	정확성을 종단 검증한다(엔진 + 공유 경계 탭 + 컨트롤러).
//
// 다중 출력 포트 라우팅 메커니즘:
//
//	출력 경계 와이어 `producer → __flow_output__:outN` 는 공유 경계 출력 탭으로 재배선된다.
//	엔진의 mergeInputWires 는 노드의 여러 입력 와이어를 단일 채널로 머지하므로, 단일 출력 탭
//	노드가 다중 입력 포트(out1/out2)를 가지면 Process 는 메시지가 어느 포트로 도착했는지
//	알 수 없다(머지로 포트 정보 소실 → 폴백으로 첫 포트에 합쳐짐). 본 테스트는 이 라우팅이
//	포트별로 정확함을 단언하여, 출력 포트별 분기 보장을 회귀 방지한다.
package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
)

// makeMultiOutRefFlow 는 입력 경계 포트 IN 1개 + 출력 경계 포트 out1/out2 2개를 가진 참조
// 플로우를 생성·저장한다. 내부 switch 노드가 payload.k 값으로 out1(k==1)/out2(k==2)로
// 분기하여, 서로 다른 출력 경계 포트로 서로 다른 메시지가 방출되게 한다.
//
//	__flow_input__:IN → sw:in
//	sw:out1 → __flow_output__:out1
//	sw:out2 → __flow_output__:out2
func makeMultiOutRefFlow(t *testing.T, adapter *FlowServiceAdapter, id string) {
	t.Helper()
	sw := flow.NodeDef{
		ID:   "sw",
		Name: "분기",
		Type: "switch",
		Config: map[string]any{
			"routes": []any{
				map[string]any{"name": "out1", "condition": "$.payload.k == 1"},
				map[string]any{"name": "out2", "condition": "$.payload.k == 2"},
			},
		},
		Inputs:  []flow.Port{inPort("in")},
		Outputs: []flow.Port{outPort("out1"), outPort("out2")},
	}
	ref := flow.NewFlowWithID(id, id,
		flow.WithNodes(sw),
		flow.WithWires(
			wire("bi", flow.FlowInputBoundaryID, "IN", "sw", "in"),
			wire("bo1", "sw", "out1", flow.FlowOutputBoundaryID, "out1"),
			wire("bo2", "sw", "out2", flow.FlowOutputBoundaryID, "out2"),
		),
		flow.WithFlowInputPorts(inPort("IN")),
		flow.WithFlowOutputPorts(outPort("out1"), outPort("out2")),
	)
	require.NoError(t, adapter.repo.Save(context.Background(), ref))
}

// TestSharedE2E_다중출력포트_포트별라우팅 은 참조 플로우가 2개의 출력 경계 포트(out1/out2)를
// 가질 때, 각 입력이 올바른 출력 경계 포트로만 돌아오는지(교차·누락 없음) 검증한다.
//
// 변경 전(버그): 출력 탭이 단일 노드 + 다중 입력 포트라 엔진 머지로 도착 포트 정보를 잃고
// 폴백으로 정렬상 첫 포트(out1)로 합쳐져, k==2 입력이 out2 대신 out1 으로 오거나 out2 가
// 누락된다 → 본 테스트 실패.
// 변경 후(수정): 출력 경계 포트별 탭 노드로 분리되어 포트별 정확 라우팅 → 통과.
func TestSharedE2E_다중출력포트_포트별라우팅(t *testing.T) {
	adapter, eng := newSharedTestEnv(t)
	ctx := context.Background()

	makeMultiOutRefFlow(t, adapter, "RM")
	require.NoError(t, adapter.DeployFlow(ctx, "RM"))
	require.NoError(t, eng.StartFlow(ctx, "RM"))
	t.Cleanup(func() { _ = eng.StopFlow(ctx, "RM"); _ = adapter.UndeployFlow(ctx, "RM") })

	// 공유 경계 컨트롤러 등록 + 출력 포트 2개 확인.
	sbCtrl := requireSharedCtrl(t, "RM")
	require.ElementsMatch(t, []string{"out1", "out2"}, sbCtrl.outputPorts,
		"참조 플로우 출력 경계 포트 2개(out1/out2)가 인식되어야 함")
	require.ElementsMatch(t, []string{"IN"}, sbCtrl.inputPorts)

	// 부모 P 배포·시작(shared flow-node → RM, 입력 IN / 출력 out1,out2).
	deployAndStartParentWithSharedPorts(t, adapter, eng, "PM", "RM",
		[]flow.Port{inPort("IN")}, []flow.Port{outPort("out1"), outPort("out2")})
	t.Cleanup(func() { _ = eng.StopFlow(ctx, "PM"); _ = adapter.UndeployFlow(ctx, "PM") })

	ctrl := firstController(t, adapter, "PM")
	requireBridgeConnected(t, ctrl)

	// 포트별 수집기: 도착 포트 → 마지막 페이로드.
	var mu sync.Mutex
	seen := map[string][]string{}
	ctrl.setOnOutput(func(p string, d json.RawMessage) {
		mu.Lock()
		seen[p] = append(seen[p], string(d))
		mu.Unlock()
	})

	// k==1 입력 → out1 으로만 돌아와야 한다.
	requireOutputOnPort(t, ctrl, &mu, seen, "IN", `{"k":1}`, "out1", "out2",
		"k==1 입력은 out1 경계 포트로만 방출되어야 함(교차·누락 없음)")

	// k==2 입력 → out2 로만 돌아와야 한다.
	requireOutputOnPort(t, ctrl, &mu, seen, "IN", `{"k":2}`, "out2", "out1",
		"k==2 입력은 out2 경계 포트로만 방출되어야 함(교차·누락 없음)")
}

// requireOutputOnPort 는 inputPort 로 payload 를 주입한 뒤, wantPort 에서 메시지가 도착하고
// forbiddenPort 에는 (해당 주입으로 인한) 메시지가 도착하지 않음을 검증한다. 입력 채널이
// 엔진에 의해 비동기 드레인되므로 Eventually 로 wantPort 도달을 기다린다. 도달 후 동일 주입에
// 대한 forbiddenPort 오염이 없는지 확인한다.
func requireOutputOnPort(t *testing.T, ctrl *managerBridgeController, mu *sync.Mutex, seen map[string][]string, inputPort, payload, wantPort, forbiddenPort, msg string) {
	t.Helper()

	snapshotLen := func(port string) int {
		mu.Lock()
		defer mu.Unlock()
		return len(seen[port])
	}
	baseWant := snapshotLen(wantPort)
	baseForbidden := snapshotLen(forbiddenPort)

	require.Eventually(t, func() bool {
		_ = ctrl.forwardInput(inputPort, json.RawMessage(payload))
		return snapshotLen(wantPort) > baseWant
	}, 5*time.Second, 50*time.Millisecond, msg)

	// 안정화 대기 후 forbiddenPort 오염(교차) 검사. 단 주입을 여러 번 했을 수 있으므로
	// 정확한 1:1 보다는 "원하는 포트에는 도착, 금지 포트에는 이번 주입 페이로드가 없음"을
	// 검사한다.
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	assert.Greater(t, len(seen[wantPort]), baseWant, "%s: wantPort=%s 도달", msg, wantPort)
	for i := baseForbidden; i < len(seen[forbiddenPort]); i++ {
		assert.NotContains(t, seen[forbiddenPort][i], jsonKMarker(payload),
			"%s: forbiddenPort=%s 로 교차 방출됨(payload=%s)", msg, forbiddenPort, payload)
	}
}

// jsonKMarker 는 페이로드에서 교차 검출용 식별 문자열을 만든다(단순 substring 비교).
// {"k":1} → `"k":1`.
func jsonKMarker(payload string) string {
	// {"k":1} → k":1 (공백 없는 컴팩트 형태 가정)
	return payload[1 : len(payload)-1]
}

// makeMultiInRefFlow 는 입력 경계 포트 in1/in2 2개 + 출력 경계 포트 out1/out2 2개를 가진
// 참조 플로우를 생성·저장한다. in1 으로 들어온 메시지는 out1 로, in2 는 out2 로 직결
// (passthrough)되어, 부모 in1→ref in1→out1, in2→ref in2→out2 매핑 정확성을 검증한다.
//
//	__flow_input__:in1 → relay1:in ; relay1:out → __flow_output__:out1
//	__flow_input__:in2 → relay2:in ; relay2:out → __flow_output__:out2
func makeMultiInRefFlow(t *testing.T, adapter *FlowServiceAdapter, id string) {
	t.Helper()
	ref := flow.NewFlowWithID(id, id,
		flow.WithNodes(passthroughNode("relay1", "릴레이1"), passthroughNode("relay2", "릴레이2")),
		flow.WithWires(
			wire("bi1", flow.FlowInputBoundaryID, "in1", "relay1", "in"),
			wire("bo1", "relay1", "out", flow.FlowOutputBoundaryID, "out1"),
			wire("bi2", flow.FlowInputBoundaryID, "in2", "relay2", "in"),
			wire("bo2", "relay2", "out", flow.FlowOutputBoundaryID, "out2"),
		),
		flow.WithFlowInputPorts(inPort("in1"), inPort("in2")),
		flow.WithFlowOutputPorts(outPort("out1"), outPort("out2")),
	)
	require.NoError(t, adapter.repo.Save(context.Background(), ref))
}

// TestSharedE2E_다중입력포트_포트별라우팅 은 참조 플로우가 2개의 입력 경계 포트(in1/in2)를
// 가질 때, 부모가 각 입력 포트로 주입한 메시지가 올바른 내부 경로를 거쳐 대응 출력 포트로만
// 돌아오는지(in1→out1, in2→out2; 교차·누락 없음) 검증한다.
//
// 입력 측은 단일 MultiSourceNode 가 포트별 source 채널을 노출하고 엔진이 출력 와이어를
// SourcePort 로 그룹핑하므로 이미 포트별로 정확히 라우팅된다. 본 테스트는 그 보장을 종단
// 회귀 방지한다(다중 출력 포트 수정이 입력 측을 깨지 않음도 함께 확인).
func TestSharedE2E_다중입력포트_포트별라우팅(t *testing.T) {
	adapter, eng := newSharedTestEnv(t)
	ctx := context.Background()

	makeMultiInRefFlow(t, adapter, "RI")
	require.NoError(t, adapter.DeployFlow(ctx, "RI"))
	require.NoError(t, eng.StartFlow(ctx, "RI"))
	t.Cleanup(func() { _ = eng.StopFlow(ctx, "RI"); _ = adapter.UndeployFlow(ctx, "RI") })

	sbCtrl := requireSharedCtrl(t, "RI")
	require.ElementsMatch(t, []string{"in1", "in2"}, sbCtrl.inputPorts,
		"참조 플로우 입력 경계 포트 2개(in1/in2)가 인식되어야 함")
	require.ElementsMatch(t, []string{"out1", "out2"}, sbCtrl.outputPorts)

	deployAndStartParentWithSharedPorts(t, adapter, eng, "PI", "RI",
		[]flow.Port{inPort("in1"), inPort("in2")}, []flow.Port{outPort("out1"), outPort("out2")})
	t.Cleanup(func() { _ = eng.StopFlow(ctx, "PI"); _ = adapter.UndeployFlow(ctx, "PI") })

	ctrl := firstController(t, adapter, "PI")
	requireBridgeConnected(t, ctrl)

	var mu sync.Mutex
	seen := map[string][]string{}
	ctrl.setOnOutput(func(p string, d json.RawMessage) {
		mu.Lock()
		seen[p] = append(seen[p], string(d))
		mu.Unlock()
	})

	// in1 주입 → out1 으로만, in2 오염 없음.
	requireOutputOnPort(t, ctrl, &mu, seen, "in1", `{"m":1}`, "out1", "out2",
		"in1 주입은 out1 으로만 돌아와야 함(in1→out1 매핑)")
	// in2 주입 → out2 로만, out1 오염 없음.
	requireOutputOnPort(t, ctrl, &mu, seen, "in2", `{"m":2}`, "out2", "out1",
		"in2 주입은 out2 로만 돌아와야 함(in2→out2 매핑)")
}
