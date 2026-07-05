// bridge_runner_restart_test.go 는 참조 플로우(subflow)를 노드에서 RESTART 했을 때 노드 측
// 라이브 브리지 tap 이 보존되어 메시지 전달이 끊기지 않음을 검증한다
// (@SPEC:SPEC-SUBFLOW-001 그룹 RB, 재시작 투명성 — bridge tap 재바인딩).
//
// 회귀 시나리오: 매니저 플로우가 remote:// flow-node 로 참조 플로우의 라이브 브리지를
// 열어둔 상태에서, 사용자가 노드의 참조 플로우를 재시작하면(Stop→Undeploy→DeployFlow→Start)
// DeployFlow 가 저장소의 UNTAPPED 정의를 다시 읽어 배포하므로 tap 노드가 사라지고 출력이
// 끊긴다. 수정 후에는 활성 컨트롤러가 있으면 동일 tapID 로 재배선하여 출력/입력이 모두
// 재시작 너머로 보존되어야 한다.
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

// TestBridgeRunner_RestartPreservesTap 는 OpenBridge 로 라이브 브리지가 열린 참조 플로우를
// RestartFlow 로 재시작해도 동일 onOutput subscriber 가 계속 출력을 수신하고, 입력 주입도
// 재시작 너머로 전달됨을 검증한다(재시작 투명성).
func TestBridgeRunner_RestartPreservesTap(t *testing.T) {
	eng, adapter, runner := newBridgeTestEnv(t)
	// 노드 측 (재)배포가 tap 인지하도록 tap 소스를 주입한다(server_bridge_opener 의
	// SetRemoteBridgeOpener 와 동일 와이어링 — main.go client-mode).
	adapter.SetBridgeTapSource(runner)

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

	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(got)
	}

	// 재시작 전: 주입 → passthrough → 출력 경계 → onOutput 도달.
	require.NoError(t, handle.Inject(ctx, "in1", json.RawMessage(`{"value":1}`)))
	require.Eventually(t, func() bool { return count() >= 1 }, 2*time.Second, 10*time.Millisecond,
		"재시작 전 출력 경계 메시지가 onOutput 으로 도달해야 함")
	before := count()

	// 사용자가 노드에서 참조 플로우를 재시작(Stop→Undeploy→DeployFlow→Start).
	require.NoError(t, adapter.RestartFlow(ctx, flowID))

	// 재시작 후 플로우는 다시 running 이어야 한다.
	status, statErr := eng.GetFlowStatus(flowID)
	require.NoError(t, statErr)
	require.Equal(t, flow.FlowRunning, status.State)

	// 핵심: 재시작 후에도 동일 subscriber 가 출력을 계속 수신해야 한다(tap 재바인딩).
	require.NoError(t, handle.Inject(ctx, "in1", json.RawMessage(`{"value":2}`)))
	require.Eventually(t, func() bool { return count() > before }, 2*time.Second, 10*time.Millisecond,
		"재시작 후에도 동일 브리지 subscriber 가 출력을 수신해야 함(tap 보존)")

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, got[len(got)-1], "out1:", "재시작 후 출력 포트가 보존되어야 함")
}
