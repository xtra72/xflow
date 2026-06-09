// server_bridge_opener_close_propagation_test.go 는 serverFlowBridge 래퍼가 하부
// ServerBridge 채널 close(노드 드롭 — teardownNodeBridges)를 자신의 다운스트림
// Outputs/Status 채널로 전파하는지 검증한다(@SPEC:SPEC-SUBFLOW-001 v1.3 그룹 RB,
// REQ-SUBFLOW-RB05/RB07/RB09 회귀 — self-healing 트리거 버그).
//
// 버그(재현): 노드 PROGRAM 재시작 시 remote.Server.teardownNodeBridges 가 하부
// ServerBridge 의 Outputs/Status 채널을 CLOSE 한다(Close() 호출 없이 — 노드가 그냥
// 사라짐). serverFlowBridge 의 두 펌프는 `for range src` 이므로 종료하지만, 래퍼의
// b.outputs/b.status 는 Close() 안에서만 닫힌다 → 다운스트림은 영구히 열린 채 멈춘다.
// 그 결과 managerBridgeController 의 pumpOutputs/pumpStatus 가 !ok 를 못 보고 영구
// 블록 → genWG.Wait() 가 안 풀려 self-healing supervisor 가 깨어나지 않는다.
//
// 수정 후: 하부 채널 close → 두 펌프 종료 → watcher 가 finish() 로 다운스트림 채널을
// close → 컨트롤러 펌프가 !ok 로 종료 → supervisor 가 재개설.
package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/remote"
)

// recvClosedWithin 는 ch 가 timeout 안에 CLOSED 상태가 되는지(수신이 !ok 를 반환)
// 검사한다. 값이 흘러오면 폐기하고 계속 기다린다(close 신호만 본다).
func recvClosedWithin[T any](ch <-chan T, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return true
			}
			// 값 수신 — 폐기하고 close 를 계속 기다린다.
		case <-deadline:
			return false
		}
	}
}

// TestServerFlowBridge_UnderlyingCloseClosesDownstream 는 노드 드롭(하부 Outputs+Status
// 채널 close)을 Close() 호출 없이 모사하고, 래퍼의 Outputs()/Status() 가 닫히는지
// 검증한다. 수정 전에는 다운스트림이 열린 채 멈춰 FAIL 한다.
func TestServerFlowBridge_UnderlyingCloseClosesDownstream(t *testing.T) {
	api := &fakeServerBridgeAPI{}
	opener := newServerBridgeOpener(api, nil)

	bridge, err := opener.OpenBridge(context.Background(), "node-1", "flow-x",
		[]string{"in1"}, []string{"out1"})
	require.NoError(t, err)
	require.NotNil(t, bridge)

	// 정상 출력 1건이 흐르는지 먼저 확인(펌프가 살아 있음).
	api.bridge.outputs <- remote.BridgeOutputPayload{Port: "out1", Data: json.RawMessage(`{"n":1}`)}
	select {
	case out := <-bridge.Outputs():
		assert.Equal(t, "out1", out.Port)
	case <-time.After(time.Second):
		t.Fatal("초기 출력 수신 타임아웃")
	}

	// 노드 드롭 모사: teardownNodeBridges 미러 — Close() 호출 없이 하부 채널만 close.
	api.bridge.dropNode()

	// 래퍼의 다운스트림 채널이 close 되어야 한다(컨트롤러 펌프가 !ok 를 보도록).
	assert.True(t, recvClosedWithin(bridge.Outputs(), 2*time.Second),
		"하부 close 시 래퍼 Outputs() 가 닫혀야 함(self-healing 트리거)")
	assert.True(t, recvClosedWithin(bridge.Status(), 2*time.Second),
		"하부 close 시 래퍼 Status() 가 닫혀야 함")
}

// TestServerFlowBridge_CloseIsIdempotentAfterUnderlyingClose 는 하부 채널 close 로
// 이미 다운스트림이 닫힌 뒤 명시적 Close() 를 호출해도 panic/이중 close 없이 멱등으로
// 동작하고, 채널이 닫힌 상태를 유지하는지 검증한다(회귀 — 이중 close 방어).
func TestServerFlowBridge_CloseIsIdempotentAfterUnderlyingClose(t *testing.T) {
	api := &fakeServerBridgeAPI{}
	opener := newServerBridgeOpener(api, nil)

	bridge, err := opener.OpenBridge(context.Background(), "node-1", "flow-x",
		[]string{"in1"}, []string{"out1"})
	require.NoError(t, err)

	// 노드 드롭 모사(Close() 없이 하부 채널 close).
	api.bridge.dropNode()

	require.True(t, recvClosedWithin(bridge.Outputs(), 2*time.Second),
		"하부 close 시 래퍼 Outputs() 가 닫혀야 함")

	// 명시적 Close() 를 여러 번 호출해도 panic 없이 멱등.
	require.NotPanics(t, func() {
		require.NoError(t, bridge.Close())
		require.NoError(t, bridge.Close())
	})

	// 채널은 여전히 닫힌 상태.
	assert.True(t, recvClosedWithin(bridge.Outputs(), time.Second),
		"Close 후에도 Outputs() 는 닫힌 상태 유지")
	assert.True(t, recvClosedWithin(bridge.Status(), time.Second),
		"Close 후에도 Status() 는 닫힌 상태 유지")
}

// TestServerFlowBridge_CloseStillClosesDownstream 는 명시적 Close()(노드 드롭 없이)가
// 여전히 하부 핸들을 닫고 다운스트림 채널을 닫는지 검증한다(기존 동작 회귀 보호).
func TestServerFlowBridge_CloseStillClosesDownstream(t *testing.T) {
	api := &fakeServerBridgeAPI{}
	opener := newServerBridgeOpener(api, nil)

	bridge, err := opener.OpenBridge(context.Background(), "node-1", "flow-x",
		[]string{"in1"}, []string{"out1"})
	require.NoError(t, err)

	require.NoError(t, bridge.Close())
	assert.True(t, api.bridge.closed, "Close 는 하부 핸들을 닫아야 함")
	assert.True(t, recvClosedWithin(bridge.Outputs(), 2*time.Second),
		"Close 후 Outputs() 는 닫혀야 함")
	assert.True(t, recvClosedWithin(bridge.Status(), 2*time.Second),
		"Close 후 Status() 는 닫혀야 함")
}
