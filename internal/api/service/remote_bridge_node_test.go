// remote_bridge_node_test.go 는 P3 매니저 측 엔진 remote-bridge 노드(라이브 브리지 엔드포인트)를
// 검증한다(@SPEC:SPEC-SUBFLOW-001 v1.3 그룹 RB, REQ-SUBFLOW-RB01/RB05/RB06/RB07/RB09/RB12).
//
// 매니저 측 엔진은 단일 노드를 source 또는 processor 로만 취급하므로(입력 와이어가 있으면
// SourceCh 미드레인), 살아남은 remote:// flow-node 를 입력 forwarder(ProcessNode) + 출력
// emitter(MultiSourceNode) 쌍으로 재배선해 라이브 브리지에 바인딩한다(P2 bridge_runner 의
// 매니저 측 미러). 본 테스트는 재배선 + 노드 라이프사이클(start=OpenBridge, 입력→SendInput,
// Output→emit, stop=Close, offline status 노출)을 fake opener 위에서 검증한다.
package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
)

// TestRewireRemoteBridges_SurvivingNodeReplaced 는 살아남은 remote:// flow-node 가 fwd+emit
// 쌍으로 재배선되고, 부모 와이어가 그 쌍으로 이어지는지 검증한다(RB01/RB07).
func TestRewireRemoteBridges_SurvivingNodeReplaced(t *testing.T) {
	// upstream → remoteFN:in1, remoteFN:out1 → downstream.
	nodes := []flow.NodeDef{
		{ID: "up", Name: "up", Type: "transform"},
		{ID: "rfn", Name: "rfn", Type: flowNodeType,
			Config:  map[string]any{flowNodeFlowIDKey: "remote://node-1/flow-x"},
			Inputs:  []flow.Port{{ID: "in1", Name: "in1", Direction: flow.PortInput}},
			Outputs: []flow.Port{{ID: "out1", Name: "out1", Direction: flow.PortOutput}},
		},
		{ID: "down", Name: "down", Type: "transform"},
	}
	wires := []flow.Wire{
		{ID: "w1", SourceNodeID: "up", SourcePort: "out", TargetNodeID: "rfn", TargetPort: "in1"},
		{ID: "w2", SourceNodeID: "rfn", SourcePort: "out1", TargetNodeID: "down", TargetPort: "in"},
	}
	f := flow.RebuildFlow(flow.NewFlow("f1"), nodes, wires)

	opener := &fakeBridgeOpener{}
	tbl := newManagerBridgeTable()
	rewired, controllers, err := rewireRemoteBridges(f, opener, tbl, nil)
	require.NoError(t, err)
	require.Len(t, controllers, 1, "원격 flow-node 1개당 컨트롤러 1개(RB12)")

	// 결과 플로우에 remote flow-node 가 더 이상 남지 않아야 한다(fwd+emit 로 치환).
	for _, n := range rewired.Nodes() {
		assert.NotEqual(t, "rfn", n.ID, "원격 flow-node 는 치환되어야 함")
	}
	// 부모 와이어가 fwd/emit 노드로 이어져야 한다.
	var sawFwdTarget, sawEmitSource bool
	for _, w := range rewired.Wires() {
		if w.SourceNodeID == "up" && w.TargetPort == "in1" {
			sawFwdTarget = true
		}
		if w.TargetNodeID == "down" && w.SourcePort == "out1" {
			sawEmitSource = true
		}
	}
	assert.True(t, sawFwdTarget, "upstream 와이어는 입력 forwarder 로 재배선되어야 함")
	assert.True(t, sawEmitSource, "출력 emitter 와이어는 downstream 으로 이어져야 함")
}

// TestManagerBridgeController_Lifecycle 는 컨트롤러 라이프사이클(open→input→output→close)을
// fake opener 위에서 검증한다(RB05/RB07/RB09).
func TestManagerBridgeController_Lifecycle(t *testing.T) {
	opener := &fakeBridgeOpener{}
	ctrl := newManagerBridgeController("node-1", "flow-x",
		[]string{"in1"}, []string{"out1"}, opener, nil)

	// start = OpenBridge.
	require.NoError(t, ctrl.start(context.Background()))
	assert.Equal(t, "node-1", opener.lastInstanceID)
	assert.Equal(t, "flow-x", opener.lastRemoteFlowID)
	assert.Equal(t, []string{"in1"}, opener.lastInputPorts)
	assert.Equal(t, []string{"out1"}, opener.lastOutputPorts)

	// 입력 forwarder: 입력 메시지 → bridge.SendInput(port 매핑 — RB06).
	require.NoError(t, ctrl.forwardInput("in1", json.RawMessage(`{"v":1}`)))
	require.Len(t, opener.bridge.sent(), 1)
	assert.Equal(t, "in1", opener.bridge.sent()[0].Port)

	// 출력 emitter: bridge Output → emit 콜백으로 전달.
	got := make(chan BridgeOutput, 1)
	ctrl.onOutput = func(port string, data json.RawMessage) {
		got <- BridgeOutput{Port: port, Data: data}
	}
	opener.bridge.outputs <- BridgeOutput{Port: "out1", Data: json.RawMessage(`{"state":"on"}`)}
	select {
	case o := <-got:
		assert.Equal(t, "out1", o.Port)
		assert.JSONEq(t, `{"state":"on"}`, string(o.Data))
	case <-time.After(time.Second):
		t.Fatal("출력 emit 수신 타임아웃")
	}

	// stop = Close.
	ctrl.stop(context.Background())
	assert.True(t, opener.bridge.isClosed())
}

// TestManagerBridgeController_OfflineStatusSurfaced 는 offline status 가 컨트롤러 상태로
// 노출되는지 검증한다(RB09 — P4 web 노출 토대).
func TestManagerBridgeController_OfflineStatusSurfaced(t *testing.T) {
	opener := &fakeBridgeOpener{}
	ctrl := newManagerBridgeController("node-1", "flow-x",
		[]string{"in1"}, []string{"out1"}, opener, nil)
	require.NoError(t, ctrl.start(context.Background()))

	opener.bridge.status <- BridgeStatus{State: "offline", Detail: "node offline"}
	require.Eventually(t, func() bool {
		st := ctrl.lastStatus()
		return st.State == "offline"
	}, time.Second, 10*time.Millisecond, "offline status 가 노출되어야 함")

	ctrl.stop(context.Background())
}
