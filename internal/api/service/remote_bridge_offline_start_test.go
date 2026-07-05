// remote_bridge_offline_start_test.go 는 매니저 측 라이브 브리지 컨트롤러의 OFFLINE-AT-BOOT
// 시작 허용을 검증한다(@SPEC:SPEC-SUBFLOW-001 v1.3 그룹 RB, REQ-SUBFLOW-RB05/RB09 회귀).
//
// 버그(재현): 매니저 부팅 시 remote:// flow-node 를 가진 플로우가 auto_start 로 배포될 때,
// 원격 NODE 는 아직 연결되지 않은 상태이다(노드는 server.Start 가 WS 엔드포인트를 올린
// 뒤에야 dial-in 한다 — auto_start 보다 나중). 따라서 컨트롤러의 초기 OpenBridge 는 transient
// 오류(node not managed / offline / open timeout)로 실패한다. 기존 start() 는 이 오류를 그대로
// 반환 → flow-node deploy 실패 → auto_start 실패. remote:// 플로우는 노드가 일시적으로 오프라인
// 이어도 배포되고 노드가 도착하면 연결되어야 한다(self-heal 감독자의 본래 취지 — 739816c/bfd195e).
//
// 수정: start() 는 PERMANENT 오설정(opener==nil)만 deploy 를 실패시킨다. 그 외 초기 OpenBridge
// 오류(transient)는 deploy 를 실패시키지 않고, c.bridge 를 nil 로 둔 채 감독자를 "아직 열지
// 못함" 모드로 시작해 노드가 온라인+관리됨이 되는 즉시 브리지를 연다. 본 테스트는 첫 OpenBridge
// 가 transient 오류를 반환한 뒤 나중 시도에 성공하는 opener 로, start() 가 NIL 을 반환하고
// (deploy 성공), 초기 c.bridge 가 nil(forwardInput 무오류 드롭)이며, 감독자가 나중 시도에
// 브리지를 열어 푸시된 출력이 onOutput 에 도달함을 검증한다.
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

// offlineThenOnlineOpener 는 처음 failFor 번의 OpenBridge 호출에 transient 오류를 반환하고
// 그 이후에는 fakeBridge 를 반환하는 테스트 더블이다(부팅 시 노드 오프라인 → 나중 온라인 모사).
// reconnectOpener 와 달리 "초기 open 도 실패할 수 있음"을 모델링한다(call==1 부터 실패 가능).
type offlineThenOnlineOpener struct {
	mu       sync.Mutex
	calls    int
	failFor  int           // 처음 failFor 번(call<=failFor)은 transient 오류 반환
	bridges  []*fakeBridge // 성공적으로 반환한 브리지 이력
	openErr  error         // transient 실패 시 반환할 오류
	openedCh chan struct{} // 각 OpenBridge 호출마다 1 신호(테스트 동기화)
}

func newOfflineThenOnlineOpener(failFor int) *offlineThenOnlineOpener {
	return &offlineThenOnlineOpener{
		failFor:  failFor,
		openErr:  errors.New("node not managed (offline at boot)"),
		openedCh: make(chan struct{}, 64),
	}
}

func (o *offlineThenOnlineOpener) OpenBridge(_ context.Context, _, _ string, _, _ []string) (FlowBridge, error) {
	o.mu.Lock()
	o.calls++
	call := o.calls
	o.mu.Unlock()

	select {
	case o.openedCh <- struct{}{}:
	default:
	}

	if call <= o.failFor {
		return nil, o.openErr
	}
	b := newFakeBridge()
	o.mu.Lock()
	o.bridges = append(o.bridges, b)
	o.mu.Unlock()
	return b, nil
}

func (o *offlineThenOnlineOpener) callCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.calls
}

func (o *offlineThenOnlineOpener) bridgeAt(i int) *fakeBridge {
	o.mu.Lock()
	defer o.mu.Unlock()
	if i < 0 || i >= len(o.bridges) {
		return nil
	}
	return o.bridges[i]
}

// TestManagerBridgeController_StartTolerantOfOfflineNode 는 부팅 시 노드가 오프라인이어서 초기
// OpenBridge 가 transient 로 실패해도 start() 가 NIL 을 반환하고(deploy 성공), 초기 c.bridge 가
// nil 이며(forwardInput 무오류 드롭), 감독자가 나중에 브리지를 열어 출력이 onOutput 에 도달함을
// 검증한다(Fix 2 핵심 — 본 버그의 root cause).
func TestManagerBridgeController_StartTolerantOfOfflineNode(t *testing.T) {
	opener := newOfflineThenOnlineOpener(3) // 처음 3번(초기 + 재시도 2)은 실패 → 노드 오프라인 모사
	ctrl := newManagerBridgeController("node-1", "flow-x",
		[]string{"in1"}, []string{"out1"}, opener, nil)
	// 타이트한(ms) 백오프 주입 — 빠르게 수렴.
	ctrl.backoff = func(attempt int) time.Duration { return time.Millisecond }

	// onOutput 콜백을 한 번만 바인딩한다(emitter 노드 미러).
	got := make(chan BridgeOutput, 8)
	ctrl.setOnOutput(func(port string, data json.RawMessage) {
		got <- BridgeOutput{Port: port, Data: data}
	})

	// 초기 open 이 transient 로 실패해도 start() 는 NIL 을 반환해야 한다(deploy 성공).
	require.NoError(t, ctrl.start(context.Background()),
		"노드 오프라인 시 초기 open transient 실패는 deploy 를 실패시키면 안 됨")

	// 초기에는 c.bridge 가 nil 이어야 한다 → forwardInput 은 무오류로 드롭한다(브리지 미가용).
	require.NoError(t, ctrl.forwardInput("in1", json.RawMessage(`{"early":true}`)),
		"브리지 미가용 시 forwardInput 은 조용히 드롭해야 함")

	// 감독자가 나중 시도에 브리지를 열어야 한다(처음 3번 실패 후 성공).
	require.Eventually(t, func() bool {
		return opener.bridgeAt(0) != nil
	}, 3*time.Second, 5*time.Millisecond, "노드 도착 시 브리지가 열려야 함")
	assert.GreaterOrEqual(t, opener.callCount(), 4, "초기1 + 실패2 + 성공1 = 최소 4회 OpenBridge")

	bridge := opener.bridgeAt(0)
	require.NotNil(t, bridge)

	// 노드 온라인 후 푸시된 출력이 같은 onOutput 콜백으로 전달되어야 한다.
	bridge.outputs <- BridgeOutput{Port: "out1", Data: json.RawMessage(`{"n":1}`)}
	select {
	case o := <-got:
		assert.JSONEq(t, `{"n":1}`, string(o.Data))
	case <-time.After(2 * time.Second):
		t.Fatal("노드 온라인 후 출력 수신 타임아웃")
	}

	// 브리지 개설 후 forwardInput 은 라이브 브리지로 향해야 한다.
	require.NoError(t, ctrl.forwardInput("in1", json.RawMessage(`{"in":true}`)))
	require.Eventually(t, func() bool {
		return len(bridge.sent()) == 1
	}, time.Second, 5*time.Millisecond, "개설 후 forwardInput 은 라이브 브리지로 향해야 함")
	assert.Equal(t, "in1", bridge.sent()[0].Port)

	// stop() 은 깔끔히 종료해야 한다.
	ctrl.stop(context.Background())
	assert.True(t, bridge.isClosed(), "stop 은 현재 브리지를 닫아야 함")
}

// TestManagerBridgeController_StartNilOpenerStillFails 는 opener==nil(PERMANENT 오설정 —
// server 모드 요구)이 여전히 ErrRemoteBridgeUnavailable 을 반환해 deploy 를 실패시킴을
// 검증한다(Fix 2 — transient 와 permanent 구분, 회귀 0).
func TestManagerBridgeController_StartNilOpenerStillFails(t *testing.T) {
	ctrl := newManagerBridgeController("node-1", "flow-x",
		[]string{"in1"}, []string{"out1"}, nil, nil)
	err := ctrl.start(context.Background())
	require.Error(t, err, "opener==nil 은 deploy 를 실패시켜야 함(server 모드 요구)")
	assert.ErrorIs(t, err, ErrRemoteBridgeUnavailable)
}

// TestManagerBridgeController_StopDuringInitialOpenRetry 는 초기 open-retry 루프(노드가 끝내
// 오지 않음)에 stuck 된 컨트롤러를 stop() 이 깔끔히 취소함을 검증한다(고루틴 누수 0, 추가
// OpenBridge 없음 — Fix 2 stop() 취소). -race 로 실행.
func TestManagerBridgeController_StopDuringInitialOpenRetry(t *testing.T) {
	// 영구 실패 opener(노드가 끝내 오지 않음 모사) — start() 는 무한 재시도 루프에 진입한다.
	opener := newOfflineThenOnlineOpener(1 << 30)
	ctrl := newManagerBridgeController("node-1", "flow-x",
		[]string{"in1"}, []string{"out1"}, opener, nil)
	ctrl.backoff = func(attempt int) time.Duration { return time.Millisecond }

	// 초기 open transient 실패 → deploy 성공(start 는 nil), 감독자는 재시도 루프에 진입.
	require.NoError(t, ctrl.start(context.Background()))

	// 재시도 루프가 OpenBridge 를 적어도 한 번 호출할 때까지 대기(루프 진입 확인).
	require.Eventually(t, func() bool {
		return opener.callCount() >= 2
	}, 2*time.Second, 5*time.Millisecond, "감독자가 초기 open-retry 루프에 진입해야 함")

	// stop() 은 초기 open-retry 루프에 stuck 된 감독자를 취소하고 회수해야 한다(블로킹 무).
	done := make(chan struct{})
	go func() {
		ctrl.stop(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("stop() 이 초기 open-retry 루프를 취소하지 못하고 블로킹됨(고루틴 누수)")
	}

	// stop 이후 추가 OpenBridge 호출이 없어야 한다(재시도 루프가 종료됨).
	callsAfterStop := opener.callCount()
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, callsAfterStop, opener.callCount(), "stop 이후 추가 OpenBridge 가 없어야 함")
}
