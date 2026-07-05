// shared_boundary_test.go 는 공유 경계 컨트롤러/탭의 단위 동작을 검증한다(@SPEC:SPEC-SUBFLOW-002
// MR02/MR03/N03/L04).
package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
)

// TestSharedBoundaryController_inject_fanin 은 입력 주입이 입력 경계 채널로 합류되는지(fan-in)
// 검증한다(MR02).
func TestSharedBoundaryController_inject_fanin(t *testing.T) {
	t.Parallel()
	c := newSharedBoundaryController("R")
	c.ensureInputChan("X")

	require.NoError(t, c.inject(context.Background(), "X", json.RawMessage(`{"v":1}`)))
	assert.Len(t, c.ensureInputChan("X"), 1, "주입 메시지가 입력 경계 채널에 합류해야 함")
}

// TestSharedBoundaryController_inject_미지의포트_오류 는 알 수 없는 포트 주입이 오류를 반환하는지
// 검증한다.
func TestSharedBoundaryController_inject_미지의포트_오류(t *testing.T) {
	t.Parallel()
	c := newSharedBoundaryController("R")
	err := c.inject(context.Background(), "nope", json.RawMessage(`{}`))
	assert.Error(t, err)
}

// TestSharedBoundaryController_inject_닫힘_오류 는 shutdown 후 주입이 오류를 반환하는지 검증한다.
func TestSharedBoundaryController_inject_닫힘_오류(t *testing.T) {
	t.Parallel()
	c := newSharedBoundaryController("R")
	c.ensureInputChan("X")
	c.shutdown()
	err := c.inject(context.Background(), "X", json.RawMessage(`{}`))
	assert.Error(t, err)
}

// TestSharedBoundaryController_inject_유계_oldestdrop 은 채널이 가득 차면 oldest-drop 으로
// 유계를 유지하는지 검증한다(N03 — 메모리 폭증 방지).
func TestSharedBoundaryController_inject_유계_oldestdrop(t *testing.T) {
	t.Parallel()
	c := newSharedBoundaryController("R")
	ch := c.ensureInputChan("X")

	// 버퍼(64)를 가득 채운다.
	for i := 0; i < sharedBoundaryInputChanBuffer; i++ {
		require.NoError(t, c.inject(context.Background(), "X", json.RawMessage(`{"n":0}`)))
	}
	assert.Len(t, ch, sharedBoundaryInputChanBuffer, "버퍼가 가득 차야 함")

	// 추가 주입: oldest-drop 후 재시도 → 길이는 유계 유지(폭증하지 않음).
	require.NoError(t, c.inject(context.Background(), "X", json.RawMessage(`{"n":1}`)))
	assert.LessOrEqual(t, len(ch), sharedBoundaryInputChanBuffer, "유계 버퍼 유지(N03)")
}

// TestSharedBoundaryController_emitOutput_fanout 은 출력이 모든 subscriber 로 fan-out 되는지
// 검증한다(MR03).
func TestSharedBoundaryController_emitOutput_fanout(t *testing.T) {
	t.Parallel()
	c := newSharedBoundaryController("R")

	got1 := make(chan json.RawMessage, 1)
	got2 := make(chan json.RawMessage, 1)
	s1 := c.addSubscriber(func(_ string, d json.RawMessage) { got1 <- d }, nil)
	s2 := c.addSubscriber(func(_ string, d json.RawMessage) { got2 <- d }, nil)
	require.NotNil(t, s1)
	require.NotNil(t, s2)

	c.emitOutput("Y", json.RawMessage(`{"out":1}`))
	assert.JSONEq(t, `{"out":1}`, string(<-got1))
	assert.JSONEq(t, `{"out":1}`, string(<-got2))

	// removeSubscriber 후에는 더 이상 받지 않는다(L06 — 참조 카운팅 없이 단순 제거).
	c.removeSubscriber(s1)
	c.emitOutput("Y", json.RawMessage(`{"out":2}`))
	assert.JSONEq(t, `{"out":2}`, string(<-got2))
	assert.Len(t, got1, 0, "제거된 subscriber 는 더 이상 받지 않아야 함")
}

// TestSharedBoundaryController_shutdown_onClose 는 shutdown 이 각 subscriber 의 onClose 를
// 호출하는지 검증한다(L04 — self-heal 트리거).
func TestSharedBoundaryController_shutdown_onClose(t *testing.T) {
	t.Parallel()
	c := newSharedBoundaryController("R")
	closed := make(chan struct{}, 1)
	require.NotNil(t, c.addSubscriber(func(string, json.RawMessage) {}, func() { closed <- struct{}{} }))

	c.shutdown()
	select {
	case <-closed:
	default:
		t.Fatal("shutdown 은 subscriber 의 onClose 를 호출해야 함(L04)")
	}
	assert.True(t, c.isClosed())
}

// TestSharedBoundaryRegistry_unregister_조건부 는 unregister 가 현재 등록된 컨트롤러와 동일할
// 때만 삭제하는지 검증한다(재배포 경합 방어).
func TestSharedBoundaryRegistry_unregister_조건부(t *testing.T) {
	t.Parallel()
	reg := newSharedBoundaryRegistry()
	c1 := newSharedBoundaryController("R")
	c2 := newSharedBoundaryController("R")
	reg.register("R", c1)

	// 다른 컨트롤러(c2)로 unregister 시도 → 삭제되지 않아야 한다.
	reg.unregister("R", c2)
	_, ok := reg.lookup("R")
	assert.True(t, ok, "다른 컨트롤러로의 unregister 는 무시되어야 함")

	// 동일 컨트롤러(c1)로 unregister → 삭제.
	reg.unregister("R", c1)
	_, ok = reg.lookup("R")
	assert.False(t, ok)
}

// TestInstallSharedBoundaryTaps_경계없음_무변경 은 경계 와이어가 없는 플로우는 탭 설치 없이
// 무변경(nil 컨트롤러)으로 반환되는지 검증한다(잎 플로우 — 회귀 0).
func TestInstallSharedBoundaryTaps_경계없음_무변경(t *testing.T) {
	t.Parallel()
	f := flow.RebuildFlow(flow.NewFlow("leaf"),
		[]flow.NodeDef{{ID: "n1", Type: "transform"}}, nil)

	out, ctrl := installSharedBoundaryTaps(f, "leaf")
	assert.Nil(t, ctrl, "경계 와이어 없는 플로우는 컨트롤러를 만들지 않아야 함")
	assert.Equal(t, f, out, "무변경 반환")
}

// TestInstallSharedBoundaryTaps_경계있음_탭설치 는 경계 와이어가 있는 플로우가 공유 경계 탭으로
// 재배선되고 컨트롤러가 입출력 포트와 함께 반환되는지 검증한다.
func TestInstallSharedBoundaryTaps_경계있음_탭설치(t *testing.T) {
	t.Parallel()
	f := flow.RebuildFlow(flow.NewFlow("R"),
		[]flow.NodeDef{{ID: "relay", Type: "transform",
			Inputs:  []flow.Port{{ID: "in", Name: "in", Direction: flow.PortInput}},
			Outputs: []flow.Port{{ID: "out", Name: "out", Direction: flow.PortOutput}},
		}},
		[]flow.Wire{
			{ID: "bi", SourceNodeID: flow.FlowInputBoundaryID, SourcePort: "X", TargetNodeID: "relay", TargetPort: "in"},
			{ID: "bo", SourceNodeID: "relay", SourcePort: "out", TargetNodeID: flow.FlowOutputBoundaryID, TargetPort: "Y"},
		})

	out, ctrl := installSharedBoundaryTaps(f, "R")
	defer globalSharedBoundaryTable.unregister("R", ctrl)
	require.NotNil(t, ctrl)
	assert.Equal(t, []string{"X"}, ctrl.inputPorts)
	assert.Equal(t, []string{"Y"}, ctrl.outputPorts)

	// 탭 노드가 추가되어야 한다. 출력 탭은 출력 경계 포트별 전용 노드(포트별 정확 라우팅)로
	// 생성되므로, 포트 Y 전용 노드 ID 로 확인한다.
	_, hasIn := nodeByID(out.Nodes(), sharedBoundaryInputTapNodeID)
	_, hasOut := nodeByID(out.Nodes(), sharedBoundaryOutputTapNodeIDForPort("Y"))
	assert.True(t, hasIn, "입력 탭 노드 추가")
	assert.True(t, hasOut, "출력 탭 노드(포트 Y 전용) 추가")

	// 경계 센티넬 와이어가 더 이상 남지 않아야 한다(탭으로 재배선).
	for _, w := range out.Wires() {
		assert.False(t, flow.IsBoundaryWire(w), "경계 와이어는 탭으로 재배선되어야 함")
	}
}
