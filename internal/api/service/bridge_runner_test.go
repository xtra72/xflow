// bridge_runner_test.go 는 노드 측 라이브 브리지 실행 어댑터(BridgeFlowRunnerAdapter)의
// 자동배포·경계 포트 tap/inject·refcount 소유(재사용/정지)를 검증한다
// (@SPEC:SPEC-SUBFLOW-001 그룹 RB, P2, REQ-SUBFLOW-RB05/RB06/RB07, OQ-RB1/RB5).
package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
)

// newBridgeTestEnv 는 tap 노드 타입을 등록한 엔진 + 어댑터 + runner 를 구성한다.
func newBridgeTestEnv(t *testing.T) (*engine.Engine, *FlowServiceAdapter, *BridgeFlowRunnerAdapter) {
	t.Helper()
	reg := node.NewRegistry()
	require.NoError(t, RegisterBridgeTapNodes(reg))
	eng := engine.NewEngine(engine.WithNodeRegistry(reg))
	repo := newTestRepo(t)
	adapter := NewFlowServiceAdapter(eng, repo, nil)
	runner := NewBridgeFlowRunnerAdapter(adapter, eng, nil)
	return eng, adapter, runner
}

// saveBoundaryFlow 는 입력 경계 포트(in1) → passthrough 노드 → 출력 경계 포트(out1)인
// 플로우를 저장하고 id 를 반환한다. passthrough 노드는 입력을 그대로 출력으로 흘린다.
func saveBoundaryFlow(t *testing.T, adapter *FlowServiceAdapter) string {
	t.Helper()
	f := flow.NewFlow("bridge-target",
		flow.WithFlowInputPorts(flow.Port{ID: "p-in", Name: "in1"}),
		flow.WithFlowOutputPorts(flow.Port{ID: "p-out", Name: "out1"}),
		flow.WithNodes(flow.NodeDef{
			ID:   "passthrough-1",
			Type: "transform",
			Inputs: []flow.Port{
				{ID: "in", Name: "in", Direction: flow.PortInput},
			},
			Outputs: []flow.Port{
				{ID: "out", Name: "out", Direction: flow.PortOutput},
			},
		}),
		flow.WithWires(
			flow.NewWire(flow.FlowInputBoundaryID, "in1", "passthrough-1", "in"),
			flow.NewWire("passthrough-1", "out", flow.FlowOutputBoundaryID, "out1"),
		),
	)
	require.NoError(t, adapter.repo.Save(context.Background(), f))
	return f.ID()
}

// TestBridgeRunner_OpenReportsBoundaryPorts 는 OpenBridge 가 참조 플로우를 실행하고
// 실제 입출력 경계 포트 이름을 보고함을 검증한다(REQ-SUBFLOW-RB06).
func TestBridgeRunner_OpenReportsBoundaryPorts(t *testing.T) {
	eng, adapter, runner := newBridgeTestEnv(t)
	flowID := saveBoundaryFlow(t, adapter)
	ctx := context.Background()

	handle, err := runner.OpenBridge(ctx, flowID, func(string, json.RawMessage) {})
	require.NoError(t, err)
	defer func() { _ = handle.Close(ctx) }()

	assert.ElementsMatch(t, []string{"in1"}, handle.InputPorts())
	assert.ElementsMatch(t, []string{"out1"}, handle.OutputPorts())

	// 참조 플로우가 running 이어야 한다(자동배포 — OQ-RB1/RB5).
	status, statErr := eng.GetFlowStatus(flowID)
	require.NoError(t, statErr)
	assert.Equal(t, flow.FlowRunning, status.State)
}

// TestBridgeRunner_InjectAndDrain 는 입력 경계 주입 → passthrough → 출력 경계 →
// onOutput 콜백 end-to-end 를 검증한다(REQ-SUBFLOW-RB07 양방향).
func TestBridgeRunner_InjectAndDrain(t *testing.T) {
	_, adapter, runner := newBridgeTestEnv(t)
	flowID := saveBoundaryFlow(t, adapter)
	ctx := context.Background()

	var mu sync.Mutex
	var got []string
	handle, err := runner.OpenBridge(ctx, flowID, func(port string, data json.RawMessage) {
		mu.Lock()
		got = append(got, port+":"+string(data))
		mu.Unlock()
	})
	require.NoError(t, err)
	defer func() { _ = handle.Close(ctx) }()

	require.NoError(t, handle.Inject(ctx, "in1", json.RawMessage(`{"value":42}`)))

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) >= 1
	}, 2*time.Second, 10*time.Millisecond, "출력 경계 메시지가 onOutput 으로 도달해야 함")

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, got[0], "out1:")
}

// TestBridgeRunner_ReuseRunningFlow 는 이미 실행 중인 플로우를 두 번째 open 이 재사용함을
// 검증한다(refcount — OQ-RB1/RB5). 첫 브리지 Close 가 두 번째를 정지시키지 않아야 한다.
func TestBridgeRunner_ReuseRunningFlow(t *testing.T) {
	eng, adapter, runner := newBridgeTestEnv(t)
	flowID := saveBoundaryFlow(t, adapter)
	ctx := context.Background()

	h1, err := runner.OpenBridge(ctx, flowID, func(string, json.RawMessage) {})
	require.NoError(t, err)
	h2, err := runner.OpenBridge(ctx, flowID, func(string, json.RawMessage) {})
	require.NoError(t, err)

	// 첫 브리지 close — 두 번째가 아직 쓰므로 플로우는 running 유지.
	require.NoError(t, h1.Close(ctx))
	status, statErr := eng.GetFlowStatus(flowID)
	require.NoError(t, statErr)
	assert.Equal(t, flow.FlowRunning, status.State, "다른 브리지가 쓰는 동안 정지하면 안 됨")

	// 마지막 브리지 close — auto-start 했고 더 쓰는 브리지가 없으므로 정지.
	require.NoError(t, h2.Close(ctx))
	status2, _ := eng.GetFlowStatus(flowID)
	assert.NotEqual(t, flow.FlowRunning, status2.State, "마지막 브리지 close 시 정지해야 함")
}

// TestBridgeRunner_FlowNotFound 는 없는 플로우 open 이 오류를 반환함을 검증한다.
func TestBridgeRunner_FlowNotFound(t *testing.T) {
	_, _, runner := newBridgeTestEnv(t)
	_, err := runner.OpenBridge(context.Background(), "does-not-exist", func(string, json.RawMessage) {})
	assert.Error(t, err)
}
