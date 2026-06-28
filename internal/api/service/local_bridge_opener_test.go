// local_bridge_opener_test.go 는 로컬 in-process FlowBridgeOpener(shared 모드)의 단위 동작을
// 검증한다(@SPEC:SPEC-SUBFLOW-002 SH02/SH04/MR02/MR03/L01/L02/L06/L07).
package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/pkg/flow"
)

// fakeStatusProvider 는 engineFlowStatusProvider 테스트 더블이다.
type fakeStatusProvider struct {
	state flow.FlowState
	err   error
}

func (f *fakeStatusProvider) GetFlowStatus(_ string) (engine.FlowStatus, error) {
	if f.err != nil {
		return engine.FlowStatus{}, f.err
	}
	return engine.FlowStatus{State: f.state}, nil
}

// newConnectedOpener 는 running 상태 + 등록된 공유 경계 컨트롤러를 갖춘 opener 와 컨트롤러를
// 반환한다(테스트 헬퍼).
func newConnectedOpener(t *testing.T, flowID string, inPorts, outPorts []string) (*localBridgeOpener, *sharedBoundaryController) {
	t.Helper()
	table := newSharedBoundaryRegistry()
	ctrl := newSharedBoundaryController(flowID)
	ctrl.inputPorts = inPorts
	ctrl.outputPorts = outPorts
	for _, p := range inPorts {
		ctrl.ensureInputChan(p)
	}
	table.register(flowID, ctrl)
	op := newLocalBridgeOpener(&fakeStatusProvider{state: flow.FlowRunning}, table, nil)
	return op, ctrl
}

// TestLocalBridgeOpener_미실행_오프라인 은 참조 플로우가 running 이 아니면 ErrSharedBridgeOffline
// (transient)을 반환하는지 검증한다(L01 자동시작 없음, L02 오프라인 대기).
func TestLocalBridgeOpener_미실행_오프라인(t *testing.T) {
	t.Parallel()
	table := newSharedBoundaryRegistry()
	op := newLocalBridgeOpener(&fakeStatusProvider{state: flow.FlowStopped}, table, nil)

	_, err := op.OpenBridge(context.Background(), "", "flow-x", []string{"in1"}, []string{"out1"})
	assert.ErrorIs(t, err, ErrSharedBridgeOffline, "미실행 참조 플로우는 오프라인이어야 함")
}

// TestLocalBridgeOpener_컨트롤러없음_오프라인 은 running 이어도 공유 경계 컨트롤러가 없으면
// 오프라인을 반환하는지 검증한다(배포 순서 경합 — 컨트롤러 도착 전).
func TestLocalBridgeOpener_컨트롤러없음_오프라인(t *testing.T) {
	t.Parallel()
	table := newSharedBoundaryRegistry()
	op := newLocalBridgeOpener(&fakeStatusProvider{state: flow.FlowRunning}, table, nil)

	_, err := op.OpenBridge(context.Background(), "", "flow-x", []string{"in1"}, []string{"out1"})
	assert.ErrorIs(t, err, ErrSharedBridgeOffline)
}

// TestLocalBridgeOpener_연결_양방향 은 연결 후 입력 주입(fan-in)과 출력 수신(fan-out)이
// 동작하는지 검증한다(SH02/SH03/MR02/MR03).
func TestLocalBridgeOpener_연결_양방향(t *testing.T) {
	t.Parallel()
	op, ctrl := newConnectedOpener(t, "flow-x", []string{"in1"}, []string{"out1"})

	bridge, err := op.OpenBridge(context.Background(), "", "flow-x", []string{"in1"}, []string{"out1"})
	require.NoError(t, err)
	require.NotNil(t, bridge)

	// 입력: SendInput → 컨트롤러 입력 채널(fan-in).
	require.NoError(t, bridge.SendInput("in1", json.RawMessage(`{"v":1}`)))
	select {
	case <-ctrl.ensureInputChan("in1"):
		// 메시지가 입력 경계 채널에 도착.
	case <-time.After(time.Second):
		t.Fatal("입력 주입이 경계 채널에 도착하지 않음")
	}

	// 출력: 컨트롤러 emitOutput → 브리지 Outputs(fan-out).
	ctrl.emitOutput("out1", json.RawMessage(`{"state":"on"}`))
	select {
	case o := <-bridge.Outputs():
		assert.Equal(t, "out1", o.Port)
		assert.JSONEq(t, `{"state":"on"}`, string(o.Data))
	case <-time.After(time.Second):
		t.Fatal("출력 fan-out 수신 타임아웃")
	}

	// 연결 직후 status 는 running.
	select {
	case st := <-bridge.Status():
		assert.Equal(t, "running", st.State)
	case <-time.After(time.Second):
		t.Fatal("running status 타임아웃")
	}
}

// TestLocalBridgeOpener_다중브리지_fanout 은 같은 컨트롤러에 attach 된 2개 브리지가 출력을
// 모두 받는지 검증한다(MR01/MR03 — 단일 인스턴스 fan-out).
func TestLocalBridgeOpener_다중브리지_fanout(t *testing.T) {
	t.Parallel()
	op, ctrl := newConnectedOpener(t, "flow-x", []string{"in1"}, []string{"out1"})

	b1, err := op.OpenBridge(context.Background(), "", "flow-x", []string{"in1"}, []string{"out1"})
	require.NoError(t, err)
	b2, err := op.OpenBridge(context.Background(), "", "flow-x", []string{"in1"}, []string{"out1"})
	require.NoError(t, err)

	ctrl.emitOutput("out1", json.RawMessage(`{"n":7}`))

	for i, b := range []FlowBridge{b1, b2} {
		select {
		case o := <-b.Outputs():
			assert.JSONEq(t, `{"n":7}`, string(o.Data), "브리지 %d", i)
		case <-time.After(time.Second):
			t.Fatalf("브리지 %d 출력 fan-out 타임아웃", i)
		}
	}
}

// TestLocalBridgeOpener_Close_참조플로우_불변 은 브리지 Close 가 subscriber 만 제거하고 참조
// 플로우/컨트롤러에는 영향을 주지 않는지 검증한다(L06/L07 — 참조 카운팅·자동 정지 없음).
func TestLocalBridgeOpener_Close_참조플로우_불변(t *testing.T) {
	t.Parallel()
	op, ctrl := newConnectedOpener(t, "flow-x", []string{"in1"}, []string{"out1"})

	b1, err := op.OpenBridge(context.Background(), "", "flow-x", []string{"in1"}, []string{"out1"})
	require.NoError(t, err)
	b2, err := op.OpenBridge(context.Background(), "", "flow-x", []string{"in1"}, []string{"out1"})
	require.NoError(t, err)

	require.NoError(t, b1.Close())
	// 컨트롤러는 닫히지 않아야 한다(참조 플로우 인스턴스 불변).
	assert.False(t, ctrl.isClosed(), "브리지 Close 가 참조 플로우 컨트롤러를 닫으면 안 됨")

	// 남은 브리지 b2 는 여전히 출력을 받아야 한다(teardown 격리 — L07).
	ctrl.emitOutput("out1", json.RawMessage(`{"alive":true}`))
	select {
	case o := <-b2.Outputs():
		assert.JSONEq(t, `{"alive":true}`, string(o.Data))
	case <-time.After(time.Second):
		t.Fatal("teardown 격리 실패: 남은 브리지가 출력을 못 받음")
	}

	// b1 Close 는 멱등.
	require.NoError(t, b1.Close())
}

// TestLocalBridgeOpener_컨트롤러닫힘_오프라인 은 컨트롤러가 shutdown 된 뒤 open 하면 오프라인을
// 반환하는지 검증한다(참조 플로우 정지 — L04).
func TestLocalBridgeOpener_컨트롤러닫힘_오프라인(t *testing.T) {
	t.Parallel()
	op, ctrl := newConnectedOpener(t, "flow-x", []string{"in1"}, []string{"out1"})
	ctrl.shutdown()

	_, err := op.OpenBridge(context.Background(), "", "flow-x", []string{"in1"}, []string{"out1"})
	assert.ErrorIs(t, err, ErrSharedBridgeOffline)
}
