package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// SPEC-SUBFLOW-001 v1.3 그룹 RB — 매니저 측 브리지 계약(P3 소비 인터페이스)
// ---------------------------------------------------------------------------
//
// 본 테스트는 P1 이 정의하는 FlowBridgeOpener/FlowBridge 인터페이스 계약을 컴파일 타임
// 및 동작으로 검증한다(REQ-SUBFLOW-RB07). 실제 구현(remote.Server bridge 메시지 위)은
// P3 에서 cmd/xflowd 와이어링 계층이 주입한다 — 여기서는 계약만 확인한다.

// fakeBridge 는 FlowBridge 계약을 만족하는 테스트 더블이다.
type fakeBridge struct {
	sent    []BridgeInputMsg
	outputs chan BridgeOutput
	status  chan BridgeStatus
	closed  bool
}

func newFakeBridge() *fakeBridge {
	return &fakeBridge{
		outputs: make(chan BridgeOutput, 4),
		status:  make(chan BridgeStatus, 4),
	}
}

func (b *fakeBridge) SendInput(port string, data json.RawMessage) error {
	b.sent = append(b.sent, BridgeInputMsg{Port: port, Data: data})
	return nil
}
func (b *fakeBridge) Outputs() <-chan BridgeOutput { return b.outputs }
func (b *fakeBridge) Status() <-chan BridgeStatus  { return b.status }
func (b *fakeBridge) Close() error {
	b.closed = true
	close(b.outputs)
	close(b.status)
	return nil
}

// fakeBridgeOpener 는 FlowBridgeOpener 계약을 만족하는 테스트 더블이다.
type fakeBridgeOpener struct {
	lastInstanceID   string
	lastRemoteFlowID string
	lastInputPorts   []string
	lastOutputPorts  []string
	bridge           *fakeBridge
}

func (o *fakeBridgeOpener) OpenBridge(_ context.Context, instanceID, remoteFlowID string, inputPorts, outputPorts []string) (FlowBridge, error) {
	o.lastInstanceID = instanceID
	o.lastRemoteFlowID = remoteFlowID
	o.lastInputPorts = inputPorts
	o.lastOutputPorts = outputPorts
	o.bridge = newFakeBridge()
	return o.bridge, nil
}

// 컴파일 타임 계약 검증: 더블이 인터페이스를 만족해야 한다.
var (
	_ FlowBridge       = (*fakeBridge)(nil)
	_ FlowBridgeOpener = (*fakeBridgeOpener)(nil)
)

// TestFlowBridgeContract 는 매니저 측 브리지 계약의 메시지 흐름(open → send input →
// receive output/status → close)을 검증한다(REQ-SUBFLOW-RB07).
func TestFlowBridgeContract(t *testing.T) {
	opener := &fakeBridgeOpener{}

	// open: 매니저가 (instance_id, remote_flow_id, 경계 포트)로 브리지를 연다.
	bridge, err := opener.OpenBridge(context.Background(), "node-1", "flow-x",
		[]string{"in1"}, []string{"out1"})
	require.NoError(t, err)
	require.NotNil(t, bridge)
	assert.Equal(t, "node-1", opener.lastInstanceID)
	assert.Equal(t, "flow-x", opener.lastRemoteFlowID)
	assert.Equal(t, []string{"in1"}, opener.lastInputPorts)
	assert.Equal(t, []string{"out1"}, opener.lastOutputPorts)

	// send input: 로컬 입력 핸들 → 원격 입력 경계 포트(WRITE).
	require.NoError(t, bridge.SendInput("in1", json.RawMessage(`{"v":1}`)))
	require.Len(t, opener.bridge.sent, 1)
	assert.Equal(t, "in1", opener.bridge.sent[0].Port)

	// receive output: 원격 출력 경계 포트 → 로컬 출력 핸들(READ).
	opener.bridge.outputs <- BridgeOutput{Port: "out1", Data: json.RawMessage(`{"state":"on"}`)}
	out := <-bridge.Outputs()
	assert.Equal(t, "out1", out.Port)
	assert.JSONEq(t, `{"state":"on"}`, string(out.Data))

	// receive status: 라이프사이클/헬스.
	opener.bridge.status <- BridgeStatus{State: "running"}
	st := <-bridge.Status()
	assert.Equal(t, "running", st.State)

	// close: teardown.
	require.NoError(t, bridge.Close())
	assert.True(t, opener.bridge.closed)
}
