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

// ---------------------------------------------------------------------------
// SPEC-SUBFLOW-001 v1.3 그룹 RB — 구체 FlowBridgeOpener/FlowBridge (remote.Server 위, P3)
// ---------------------------------------------------------------------------
//
// ServerBridgeOpener 는 service.FlowBridgeOpener 를 remote.Server 의 bridge API 위에 구현한다.
// 본 테스트는 fakeServerBridgeAPI(remote.Server 의 bridge 표면을 모사) 위에서 open →
// sendinput → outputs → status → close 매핑 + 매니저 측 포트별 버퍼링을 검증한다.

// fakeServerBridge 는 serverBridgeHandle 계약을 만족하는 테스트 더블이다.
//
// 실제 remote.ServerBridge 처럼 채널 close 는 멱등이다(closeOnce). 노드 드롭은
// teardownNodeBridges 가 채널을 닫는 것으로 모사하므로, 이후 Close() 가 다시 호출되어도
// 이중 close panic 이 나지 않아야 한다(실제 ServerBridge.teardown 의 closeOnce 미러).
type fakeServerBridge struct {
	id      string
	inPorts []string
	outPort []string
	sent    []struct {
		port string
		data json.RawMessage
	}
	outputs   chan remote.BridgeOutputPayload
	status    chan remote.BridgeStatusPayload
	closed    bool
	closeOnce sync.Once
}

// dropNode 는 노드 세션 드롭(teardownNodeBridges)을 모사한다: Close() 호출 없이 채널만
// 닫는다(멱등). 실제 운영에서 노드가 사라질 때의 경로다.
func (b *fakeServerBridge) dropNode() {
	b.closeOnce.Do(func() {
		close(b.outputs)
		close(b.status)
	})
}

func newFakeServerBridge(id string, in, out []string) *fakeServerBridge {
	return &fakeServerBridge{
		id:      id,
		inPorts: in,
		outPort: out,
		outputs: make(chan remote.BridgeOutputPayload, 8),
		status:  make(chan remote.BridgeStatusPayload, 8),
	}
}

func (b *fakeServerBridge) BridgeID() string      { return b.id }
func (b *fakeServerBridge) InputPorts() []string  { return b.inPorts }
func (b *fakeServerBridge) OutputPorts() []string { return b.outPort }
func (b *fakeServerBridge) SendInput(port string, data json.RawMessage) error {
	b.sent = append(b.sent, struct {
		port string
		data json.RawMessage
	}{port, data})
	return nil
}
func (b *fakeServerBridge) Outputs() <-chan remote.BridgeOutputPayload { return b.outputs }
func (b *fakeServerBridge) Status() <-chan remote.BridgeStatusPayload  { return b.status }
func (b *fakeServerBridge) Close() error {
	b.closed = true
	b.closeOnce.Do(func() { // 멱등 — 노드 드롭(dropNode)이 먼저 닫았어도 안전(실제 ServerBridge 미러).
		close(b.outputs)
		close(b.status)
	})
	return nil
}

// fakeServerBridgeAPI 는 serverBridgeAPI(remote.Server 의 OpenBridge 표면)를 모사한다.
type fakeServerBridgeAPI struct {
	lastInstanceID   string
	lastRemoteFlowID string
	lastInputPorts   []string
	lastOutputPorts  []string
	bridge           *fakeServerBridge
}

func (a *fakeServerBridgeAPI) OpenBridge(_ context.Context, instanceID, remoteFlowID string, inputPorts, outputPorts []string) (serverBridgeHandle, error) {
	a.lastInstanceID = instanceID
	a.lastRemoteFlowID = remoteFlowID
	a.lastInputPorts = inputPorts
	a.lastOutputPorts = outputPorts
	a.bridge = newFakeServerBridge("b-1", []string{"in1"}, []string{"out1"})
	return a.bridge, nil
}

// TestServerBridgeOpener_OpenSendOutputsClose 는 구체 어댑터가 open → sendinput → outputs/
// status → close 를 remote bridge API 로 매핑하는지 검증한다(REQ-SUBFLOW-RB07).
func TestServerBridgeOpener_OpenSendOutputsClose(t *testing.T) {
	api := &fakeServerBridgeAPI{}
	opener := newServerBridgeOpener(api, nil)

	bridge, err := opener.OpenBridge(context.Background(), "node-1", "flow-x",
		[]string{"in1"}, []string{"out1"})
	require.NoError(t, err)
	require.NotNil(t, bridge)
	assert.Equal(t, "node-1", api.lastInstanceID)
	assert.Equal(t, "flow-x", api.lastRemoteFlowID)

	// SendInput → 하부 bridge 로 전달.
	require.NoError(t, bridge.SendInput("in1", json.RawMessage(`{"v":1}`)))
	require.Len(t, api.bridge.sent, 1)
	assert.Equal(t, "in1", api.bridge.sent[0].port)

	// 하부 bridge_output → 어댑터 Outputs(매니저 측 버퍼 경유).
	api.bridge.outputs <- remote.BridgeOutputPayload{Port: "out1", Data: json.RawMessage(`{"state":"on"}`)}
	select {
	case out := <-bridge.Outputs():
		assert.Equal(t, "out1", out.Port)
		assert.JSONEq(t, `{"state":"on"}`, string(out.Data))
	case <-time.After(time.Second):
		t.Fatal("Outputs 매핑 수신 타임아웃")
	}

	// 하부 bridge_status → 어댑터 Status.
	api.bridge.status <- remote.BridgeStatusPayload{Status: "running"}
	select {
	case st := <-bridge.Status():
		assert.Equal(t, "running", st.State)
	case <-time.After(time.Second):
		t.Fatal("Status 매핑 수신 타임아웃")
	}

	// Close → 하부 bridge.Close + 어댑터 채널 close.
	require.NoError(t, bridge.Close())
	assert.True(t, api.bridge.closed)
	require.Eventually(t, func() bool {
		select {
		case _, ok := <-bridge.Outputs():
			return !ok
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond, "Close 후 Outputs 는 닫혀야 함")
}

// 컴파일 타임: 어댑터가 service.FlowBridgeOpener/FlowBridge 를 만족해야 한다.
var (
	_ FlowBridgeOpener = (*serverBridgeOpener)(nil)
)
