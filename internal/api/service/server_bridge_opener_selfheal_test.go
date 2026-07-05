// server_bridge_opener_selfheal_test.go 는 REAL serverFlowBridge 래퍼를 managerBridgeController
// 에 물려 end-to-end self-healing 트리거를 검증한다(@SPEC:SPEC-SUBFLOW-001 v1.3 그룹 RB,
// REQ-SUBFLOW-RB05/RB07/RB09 — 본 버그의 증상에 직접 대응).
//
// 기존 remote_bridge_reconnect_test.go 는 fakeBridge(FlowBridge 직접 구현)를 쓰므로 래퍼
// 버그를 노출하지 못한다(fakeBridge.Close() 가 채널을 닫음 → 펌프 종료). 실제 운영에서는
// 노드 드롭 시 Close() 가 호출되지 않고 하부 ServerBridge 채널만 닫히는데, 래퍼가 그것을
// 다운스트림으로 전파하지 못해 컨트롤러 펌프가 영구 블록한다. 본 테스트는 그 실제 경로
// (serverFlowBridge 래퍼)를 그대로 사용해 재현/회귀한다.
package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/remote"
)

// selfHealServerAPI 는 호출마다 새 fakeServerBridge 를 반환하는 serverBridgeAPI 더블이다.
// serverBridgeOpener 가 이를 REAL serverFlowBridge 로 래핑하므로, 컨트롤러는 실제 래퍼를
// 통해 동작한다(reconnect_test 의 fakeBridge 직접 경로와 다름 — 래퍼 버그 노출).
type selfHealServerAPI struct {
	mu       sync.Mutex
	calls    int
	failFor  int // 재개설(call>=2) 중 처음 failFor 번은 에러 반환(백오프 운동)
	bridges  []*fakeServerBridge
	openedCh chan struct{}
}

func newSelfHealServerAPI(failFor int) *selfHealServerAPI {
	return &selfHealServerAPI{failFor: failFor, openedCh: make(chan struct{}, 64)}
}

func (a *selfHealServerAPI) OpenBridge(_ context.Context, _, _ string, in, out []string) (serverBridgeHandle, error) {
	a.mu.Lock()
	a.calls++
	call := a.calls
	a.mu.Unlock()

	select {
	case a.openedCh <- struct{}{}:
	default:
	}

	if call >= 2 && (call-1) <= a.failFor {
		return nil, assertOfflineErr
	}
	b := newFakeServerBridge("b", in, out)
	a.mu.Lock()
	a.bridges = append(a.bridges, b)
	a.mu.Unlock()
	return b, nil
}

func (a *selfHealServerAPI) callCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

func (a *selfHealServerAPI) bridgeAt(i int) *fakeServerBridge {
	a.mu.Lock()
	defer a.mu.Unlock()
	if i < 0 || i >= len(a.bridges) {
		return nil
	}
	return a.bridges[i]
}

var assertOfflineErr = &offlineErr{}

type offlineErr struct{}

func (*offlineErr) Error() string { return "node not managed (offline)" }

// TestManagerBridgeController_SelfHealWithRealWrapper 는 REAL serverFlowBridge 래퍼를 통해
// 노드 드롭(하부 ServerBridge 채널 close, Close() 미호출)이 컨트롤러 self-heal 을 트리거하는지
// end-to-end 로 검증한다. 수정 전에는 래퍼가 다운스트림을 닫지 못해 재개설이 일어나지 않아
// FAIL 한다(본 버그의 직접 증상).
func TestManagerBridgeController_SelfHealWithRealWrapper(t *testing.T) {
	api := newSelfHealServerAPI(2) // 재개설 첫 2번 실패 → 백오프 운동
	opener := newServerBridgeOpener(api, nil)

	ctrl := newManagerBridgeController("node-1", "flow-x",
		[]string{"in1"}, []string{"out1"}, opener, nil)
	ctrl.backoff = func(attempt int) time.Duration { return time.Millisecond }

	// onOutput 콜백을 한 번만 바인딩(emitter 노드 미러). 재연결 후에도 동일 콜백으로 흘러야 함.
	got := make(chan BridgeOutput, 8)
	ctrl.setOnOutput(func(port string, data json.RawMessage) {
		got <- BridgeOutput{Port: port, Data: data}
	})

	require.NoError(t, ctrl.start(context.Background()))
	require.Equal(t, 1, api.callCount(), "초기 open 은 1회")

	bridge1 := api.bridgeAt(0)
	require.NotNil(t, bridge1)

	// bridge#1 출력 → onOutput 수신(REAL 래퍼 경유).
	bridge1.outputs <- remote.BridgeOutputPayload{Port: "out1", Data: json.RawMessage(`{"n":1}`)}
	select {
	case o := <-got:
		assert.JSONEq(t, `{"n":1}`, string(o.Data))
	case <-time.After(2 * time.Second):
		t.Fatal("bridge#1 출력 수신 타임아웃")
	}

	// 노드 드롭 모사(teardownNodeBridges 미러): Close() 호출 없이 하부 채널만 close.
	// 이것이 정확히 운영 경로다 — 컨트롤러 stop 은 호출되지 않는다.
	bridge1.dropNode()

	// 컨트롤러는 self-heal 로 재개설해야 한다: OpenBridge 가 다시 호출되고(처음 2번 실패 후
	// 성공) bridge#2 가 생긴다. 수정 전에는 래퍼가 다운스트림을 닫지 못해 여기서 멈춘다.
	require.Eventually(t, func() bool {
		return api.bridgeAt(1) != nil
	}, 4*time.Second, 5*time.Millisecond, "노드 드롭 후 브리지가 재개설되어야 함(self-heal)")
	assert.GreaterOrEqual(t, api.callCount(), 4, "재개설은 초기1 + 실패2 + 성공1 = 최소 4회")

	bridge2 := api.bridgeAt(1)
	require.NotNil(t, bridge2)
	require.NotSame(t, bridge1, bridge2)

	// 재연결 후 bridge#2 출력 → 같은 onOutput 콜백으로 전달(투명 재개).
	bridge2.outputs <- remote.BridgeOutputPayload{Port: "out1", Data: json.RawMessage(`{"n":2}`)}
	select {
	case o := <-got:
		assert.JSONEq(t, `{"n":2}`, string(o.Data))
	case <-time.After(2 * time.Second):
		t.Fatal("재연결 후 bridge#2 출력 수신 타임아웃(self-heal 실패)")
	}

	// forwardInput 은 재연결 후 bridge#2 를 타겟해야 한다(SendInput 은 동일 고루틴에서
	// 동기 실행되므로 직후 sent 를 직접 읽어도 race-free).
	require.NoError(t, ctrl.forwardInput("in1", json.RawMessage(`{"in":true}`)))
	require.Len(t, bridge2.sent, 1, "forwardInput 은 재연결된 bridge#2 로 향해야 함")
	assert.Equal(t, "in1", bridge2.sent[0].port)

	// stop() 은 추가 재개설 없이 깔끔히 종료.
	callsBeforeStop := api.callCount()
	ctrl.stop(context.Background())
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, callsBeforeStop, api.callCount(), "stop 이후 추가 OpenBridge 가 없어야 함")
}
