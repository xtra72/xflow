// remote_bridge_reconnect_test.go 는 매니저 측 라이브 브리지 컨트롤러의 SELF-HEALING(자동
// 재연결)을 검증한다(@SPEC:SPEC-SUBFLOW-001 v1.3 그룹 RB, REQ-SUBFLOW-RB05/RB07/RB09 회귀).
//
// 버그(재현): 원격 NODE PROGRAM 재시작 시 노드 WS 는 재연결되지만(매니저가 hello 로 online
// 재기록), 라이브 서브플로우 BRIDGE 출력은 영구히 멈춘다. 원인 — 매니저 측 브리지는 로컬
// 플로우 배포 시 단 한 번(start) OpenBridge 로 열린다. 노드 드롭 시 remote.Server 가
// teardownNodeBridges 로 최종 offline status 를 방출하고 ServerBridge 의 Outputs/Status
// 채널을 close 하면, 컨트롤러의 pumpOutputs/pumpStatus 가 !ok 로 RETURN 한다. 로컬 flow-node
// 는 여전히 배포 상태(stop 미호출)이므로 c.closed==false 인데, 매니저는 브리지를 재개설하지
// 않는다 → 출력이 영구히 죽는다.
//
// 수정: 컨트롤러가 의도적 stop 없이 브리지가 죽으면(펌프 종료/offline) 재개설 루프에 진입한다.
// 본 테스트는 노드 드롭을 모사(offline status 방출 + bridge#1 Outputs/Status 채널 close —
// teardownNodeBridges 미러)하고, 컨트롤러가 OpenBridge 를 다시 호출하며(백오프 동안 몇 번
// 실패 후 bridge#2 반환), 재연결 후 같은 onOutput 콜백으로 출력이 전달되고 forwardInput 이
// bridge#2 를 타겟하는지 검증한다. stop() 은 추가 OpenBridge 없이 깔끔히 종료(고루틴 누수 0).
package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reconnectOpener 는 호출마다 별도 fakeBridge 를 반환하고, 처음 failBefore 번은 에러를 반환해
// 백오프 재시도를 강제하는 테스트 더블이다(self-healing 검증용 — 동시 접근 안전).
type reconnectOpener struct {
	mu       sync.Mutex
	calls    int
	failFor  int           // 1번 이후(재개설) 호출 중 처음 failFor 번은 에러를 반환
	bridges  []*fakeBridge // 성공적으로 반환한 브리지 이력(인덱스 = 성공 순번)
	openErr  error         // 실패 시 반환할 에러
	openedCh chan struct{} // 각 OpenBridge 호출마다(성공/실패 무관) 1 신호(테스트 동기화용)
}

func newReconnectOpener(failFor int) *reconnectOpener {
	return &reconnectOpener{
		failFor:  failFor,
		openErr:  errors.New("node not managed (offline)"),
		openedCh: make(chan struct{}, 64),
	}
}

func (o *reconnectOpener) OpenBridge(_ context.Context, _, _ string, _, _ []string) (FlowBridge, error) {
	o.mu.Lock()
	o.calls++
	call := o.calls
	o.mu.Unlock()

	// 테스트 동기화: 호출이 발생했음을 비블로킹 신호.
	select {
	case o.openedCh <- struct{}{}:
	default:
	}

	// 첫 호출(call==1)은 항상 성공(초기 open — deploy-time 게이팅). 재개설(call>=2) 중
	// 처음 failFor 번은 실패시켜 백오프 재시도를 운동시킨다.
	if call >= 2 && (call-1) <= o.failFor {
		return nil, o.openErr
	}
	b := newFakeBridge()
	o.mu.Lock()
	o.bridges = append(o.bridges, b)
	o.mu.Unlock()
	return b, nil
}

func (o *reconnectOpener) callCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.calls
}

func (o *reconnectOpener) bridgeAt(i int) *fakeBridge {
	o.mu.Lock()
	defer o.mu.Unlock()
	if i < 0 || i >= len(o.bridges) {
		return nil
	}
	return o.bridges[i]
}

// TestManagerBridgeController_SelfHealOnNodeDrop 는 노드 드롭(펌프 종료) 후 컨트롤러가 자동
// 재개설하고, 재연결 후 같은 onOutput 콜백으로 출력이 흐르며 forwardInput 이 새 브리지를
// 타겟하는지 검증한다(self-healing 회귀 — 본 버그의 핵심).
func TestManagerBridgeController_SelfHealOnNodeDrop(t *testing.T) {
	opener := newReconnectOpener(2) // 재개설 첫 2번 실패 → 백오프 운동
	ctrl := newManagerBridgeController("node-1", "flow-x",
		[]string{"in1"}, []string{"out1"}, opener, nil)
	// 테스트는 타이트한(ms) 백오프를 주입해 빠르게 수렴시킨다.
	ctrl.backoff = func(attempt int) time.Duration { return time.Millisecond }

	// onOutput 콜백을 한 번만 바인딩한다(emitter 노드 미러). 재연결 후에도 동일 콜백으로
	// 출력이 흘러야 한다.
	got := make(chan BridgeOutput, 8)
	ctrl.setOnOutput(func(port string, data json.RawMessage) {
		got <- BridgeOutput{Port: port, Data: data}
	})

	// 초기 open(deploy-time) 은 동기적으로 성공해야 한다.
	require.NoError(t, ctrl.start(context.Background()))
	require.Equal(t, 1, opener.callCount(), "초기 open 은 1회")

	bridge1 := opener.bridgeAt(0)
	require.NotNil(t, bridge1)

	// bridge#1 에서 출력 → 콜백 수신.
	bridge1.outputs <- BridgeOutput{Port: "out1", Data: json.RawMessage(`{"n":1}`)}
	select {
	case o := <-got:
		assert.JSONEq(t, `{"n":1}`, string(o.Data))
	case <-time.After(2 * time.Second):
		t.Fatal("bridge#1 출력 수신 타임아웃")
	}

	// 노드 드롭 모사(teardownNodeBridges 미러): 최종 offline status 방출 후 채널 close.
	// fakeBridge.Close() 가 outputs/status 를 close 하므로, 그것으로 채널 종료를 모사한다.
	bridge1.status <- BridgeStatus{State: "offline", Detail: "node offline"}
	require.NoError(t, bridge1.Close())

	// 컨트롤러는 재개설을 시도해야 한다: OpenBridge 가 다시 호출되고(처음 2번 실패 후 성공),
	// 결국 성공 브리지(bridge#2)가 생긴다.
	require.Eventually(t, func() bool {
		return opener.bridgeAt(1) != nil
	}, 3*time.Second, 5*time.Millisecond, "노드 드롭 후 브리지가 재개설되어야 함")
	assert.GreaterOrEqual(t, opener.callCount(), 4, "재개설은 초기1 + 실패2 + 성공1 = 최소 4회 OpenBridge")

	bridge2 := opener.bridgeAt(1)
	require.NotNil(t, bridge2)
	require.NotSame(t, bridge1, bridge2, "재개설은 새 브리지여야 함")

	// 재연결 후 bridge#2 출력 → 같은 onOutput 콜백으로 전달되어야 한다(투명 재개).
	bridge2.outputs <- BridgeOutput{Port: "out1", Data: json.RawMessage(`{"n":2}`)}
	select {
	case o := <-got:
		assert.JSONEq(t, `{"n":2}`, string(o.Data))
	case <-time.After(2 * time.Second):
		t.Fatal("재연결 후 bridge#2 출력 수신 타임아웃")
	}

	// forwardInput 은 재연결 후 bridge#2 를 타겟해야 한다.
	require.NoError(t, ctrl.forwardInput("in1", json.RawMessage(`{"in":true}`)))
	require.Eventually(t, func() bool {
		return len(bridge2.sent()) == 1
	}, time.Second, 5*time.Millisecond, "forwardInput 은 재연결된 bridge#2 로 향해야 함")
	assert.Equal(t, "in1", bridge2.sent()[0].Port)

	// stop() 은 추가 재개설 없이 깔끔히 종료해야 한다.
	callsBeforeStop := opener.callCount()
	ctrl.stop(context.Background())
	assert.True(t, bridge2.isClosed(), "stop 은 현재 브리지를 닫아야 함")

	// stop 이후 추가 OpenBridge 호출이 없어야 한다(재개설 루프가 종료됨).
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, callsBeforeStop, opener.callCount(), "stop 이후 추가 OpenBridge 가 없어야 함")
}

// TestManagerBridgeController_StopDoesNotReopen 는 의도적 stop 이 재개설을 유발하지 않음을
// 검증한다(stop 과 node-drop 구분 — 정상 종료가 reopen 을 트리거하면 안 됨).
func TestManagerBridgeController_StopDoesNotReopen(t *testing.T) {
	opener := newReconnectOpener(0)
	ctrl := newManagerBridgeController("node-1", "flow-x",
		[]string{"in1"}, []string{"out1"}, opener, nil)
	ctrl.backoff = func(attempt int) time.Duration { return time.Millisecond }
	require.NoError(t, ctrl.start(context.Background()))
	require.Equal(t, 1, opener.callCount())

	// 정상 stop: 브리지를 닫고 펌프를 종료한다. 이때 채널 close 가 발생하지만 c.closed==true
	// 이므로 재개설 루프에 진입하면 안 된다.
	ctrl.stop(context.Background())

	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, 1, opener.callCount(), "정상 stop 은 재개설하면 안 됨")
}
