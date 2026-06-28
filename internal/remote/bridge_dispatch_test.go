// bridge_dispatch_test.go 는 P3 서버 측 라이브 브리지 라우팅/상관/teardown 을 검증한다
// (@SPEC:SPEC-SUBFLOW-001 v1.3 그룹 RB, REQ-SUBFLOW-RB05/RB06/RB07/RB09/RB12).
//
// 검증(command/query/stream 서버 테스트를 미러):
//   - OpenBridge: bridge_id 부여 + bridge_open 전송 + bridge_open_ack 대기/상관(RB05/RB06).
//   - OpenBridge 타임아웃: ack 미수신 시 오류(RB05).
//   - bridge_output / bridge_status 를 bridge_id 로 소유 브리지에 라우팅(RB07/RB09).
//   - SendBridgeInput → bridge_input(WRITE — RB07). CloseBridge → bridge_close(RB09).
//   - 노드 세션 드롭 시 해당 노드 브리지 teardown + 최종 offline status(RB09).
//
// fakeConn / approvedNodeConn 은 server_test.go / dispatch_test.go 에 정의된 것을 재사용한다.
package remote

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readDispatchedBridgeOpen 은 서버가 conn 으로 보낸 다음 bridge_open 을 읽어 디코드한다.
func readDispatchedBridgeOpen(t *testing.T, conn *fakeConn) BridgeOpenPayload {
	t.Helper()
	select {
	case data := <-conn.outgoing:
		msg, err := DecodeMessage(data)
		require.NoError(t, err)
		require.Equal(t, TypeBridgeOpen, msg.Type)
		var op BridgeOpenPayload
		require.NoError(t, json.Unmarshal(msg.Payload, &op))
		return op
	case <-time.After(time.Second):
		t.Fatal("bridge_open 디스패치 수신 타임아웃")
		return BridgeOpenPayload{}
	}
}

// openBridgeAsync 는 OpenBridge 를 고루틴으로 호출하고 결과를 채널로 반환한다.
func openBridgeAsync(srv *Server, instanceID, flowID string) (<-chan *ServerBridge, <-chan error) {
	bch := make(chan *ServerBridge, 1)
	ech := make(chan error, 1)
	go func() {
		b, err := srv.OpenBridge(context.Background(), instanceID, flowID,
			[]string{"in1"}, []string{"out1"})
		bch <- b
		ech <- err
	}()
	return bch, ech
}

// TestServerBridge_OpenAckCorrelation 은 OpenBridge 가 bridge_open 을 보내고 매칭되는
// bridge_open_ack 로 상관되어 핸들을 활성화하는지 검증한다(RB05/RB06).
func TestServerBridge_OpenAckCorrelation(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-b1")
	defer cancel()

	bch, ech := openBridgeAsync(srv, "node-b1", "flow-x")

	op := readDispatchedBridgeOpen(t, conn)
	assert.Equal(t, "node-b1", op.InstanceID)
	assert.Equal(t, "flow-x", op.RemoteFlowID)
	require.NotEmpty(t, op.BridgeID, "브리지는 고유 bridge_id 를 가져야 함")

	ack, _ := NewBridgeOpenAckMessage(BridgeOpenAckPayload{
		BridgeID:    op.BridgeID,
		OK:          true,
		InputPorts:  []string{"in1"},
		OutputPorts: []string{"out1", "out2"},
	})
	conn.inject(t, ack)

	require.NoError(t, <-ech)
	b := <-bch
	require.NotNil(t, b)
	assert.Equal(t, op.BridgeID, b.BridgeID())
	assert.Equal(t, []string{"in1"}, b.InputPorts())
	assert.Equal(t, []string{"out1", "out2"}, b.OutputPorts())
}

// TestServerBridge_OpenAckFailure 는 ack.ok=false 시 OpenBridge 가 오류를 반환하는지
// 검증한다(RB05 — 502 계열 매핑 입력).
func TestServerBridge_OpenAckFailure(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-b1f")
	defer cancel()

	_, ech := openBridgeAsync(srv, "node-b1f", "flow-x")
	op := readDispatchedBridgeOpen(t, conn)

	ack, _ := NewBridgeOpenAckMessage(BridgeOpenAckPayload{
		BridgeID: op.BridgeID, OK: false, Error: "flow not in exposure scope",
	})
	conn.inject(t, ack)

	err := <-ech
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flow not in exposure scope")
}

// TestServerBridge_OpenTimeout 은 ack 미수신 시 OpenBridge 가 타임아웃 오류를 반환하는지
// 검증한다(RB05).
func TestServerBridge_OpenTimeout(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-b2")
	defer cancel()
	srv.bridgeOpenTimeout = 150 * time.Millisecond

	_, ech := openBridgeAsync(srv, "node-b2", "flow-x")
	_ = readDispatchedBridgeOpen(t, conn) // ack 를 보내지 않는다.

	select {
	case err := <-ech:
		require.ErrorIs(t, err, ErrBridgeOpenTimeout)
	case <-time.After(time.Second):
		t.Fatal("OpenBridge 타임아웃 미발생")
	}
}

// TestServerBridge_NotManaged 는 미승인/오프라인 노드 OpenBridge 가 거절되는지 검증한다
// (RB08 게이팅 전제 — command 디스패치와 동일).
func TestServerBridge_NotManaged(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil)
	_, err := srv.OpenBridge(context.Background(), "ghost", "flow-x", nil, nil)
	require.ErrorIs(t, err, ErrNodeNotManaged)
}

// openedBridge 는 OpenBridge → ack 까지 진행해 활성 브리지를 반환한다(헬퍼).
func openedBridge(t *testing.T, srv *Server, conn *fakeConn, instanceID, flowID string) *ServerBridge {
	t.Helper()
	bch, ech := openBridgeAsync(srv, instanceID, flowID)
	op := readDispatchedBridgeOpen(t, conn)
	ack, _ := NewBridgeOpenAckMessage(BridgeOpenAckPayload{
		BridgeID: op.BridgeID, OK: true, InputPorts: []string{"in1"}, OutputPorts: []string{"out1"},
	})
	conn.inject(t, ack)
	require.NoError(t, <-ech)
	return <-bch
}

// TestServerBridge_OutputRoutedByBridgeID 는 bridge_output 이 소유 브리지의 Outputs
// 채널로 라우팅되는지 검증한다(RB07).
func TestServerBridge_OutputRoutedByBridgeID(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-b3")
	defer cancel()
	b := openedBridge(t, srv, conn, "node-b3", "flow-x")
	defer b.Close()

	out, _ := NewBridgeOutputMessage(BridgeOutputPayload{
		BridgeID: b.BridgeID(), Port: "out1", Data: json.RawMessage(`{"state":"on"}`),
	})
	conn.inject(t, out)

	select {
	case got := <-b.Outputs():
		assert.Equal(t, "out1", got.Port)
		assert.JSONEq(t, `{"state":"on"}`, string(got.Data))
	case <-time.After(time.Second):
		t.Fatal("bridge_output 라우팅 수신 타임아웃")
	}
}

// TestServerBridge_StatusRoutedByBridgeID 는 bridge_status 가 소유 브리지의 Status
// 채널로 라우팅되는지 검증한다(RB09).
func TestServerBridge_StatusRoutedByBridgeID(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-b4")
	defer cancel()
	b := openedBridge(t, srv, conn, "node-b4", "flow-x")
	defer b.Close()

	st, _ := NewBridgeStatusMessage(BridgeStatusPayload{
		BridgeID: b.BridgeID(), Status: BridgeStateError, Error: "boom",
	})
	conn.inject(t, st)

	select {
	case got := <-b.Status():
		assert.Equal(t, BridgeStateError, got.Status)
		assert.Equal(t, "boom", got.Error)
	case <-time.After(time.Second):
		t.Fatal("bridge_status 라우팅 수신 타임아웃")
	}
}

// TestServerBridge_SendInputAndClose 는 SendBridgeInput → bridge_input(WRITE),
// CloseBridge → bridge_close 가 노드로 전송되는지 검증한다(RB07/RB09).
func TestServerBridge_SendInputAndClose(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-b5")
	defer cancel()
	b := openedBridge(t, srv, conn, "node-b5", "flow-x")

	require.NoError(t, b.SendInput("in1", json.RawMessage(`{"v":1}`)))
	select {
	case data := <-conn.outgoing:
		msg, err := DecodeMessage(data)
		require.NoError(t, err)
		require.Equal(t, TypeBridgeInput, msg.Type)
		var in BridgeInputPayload
		require.NoError(t, json.Unmarshal(msg.Payload, &in))
		assert.Equal(t, b.BridgeID(), in.BridgeID)
		assert.Equal(t, "in1", in.Port)
		assert.JSONEq(t, `{"v":1}`, string(in.Data))
	case <-time.After(time.Second):
		t.Fatal("bridge_input 전송 수신 타임아웃")
	}

	require.NoError(t, b.Close())
	select {
	case data := <-conn.outgoing:
		msg, err := DecodeMessage(data)
		require.NoError(t, err)
		require.Equal(t, TypeBridgeClose, msg.Type)
		var cl BridgeClosePayload
		require.NoError(t, json.Unmarshal(msg.Payload, &cl))
		assert.Equal(t, b.BridgeID(), cl.BridgeID)
	case <-time.After(time.Second):
		t.Fatal("bridge_close 전송 수신 타임아웃")
	}

	// Close 후 Outputs/Status 채널은 닫혀야 한다(소비자 종료 감지).
	_, ok := <-b.Outputs()
	assert.False(t, ok, "Close 후 Outputs 채널은 닫혀야 함")
}

// TestServerBridge_NodeDropTeardown 은 노드 세션 드롭 시 해당 노드의 브리지가
// teardown 되고 최종 offline status 가 방출되는지 검증한다(RB09).
func TestServerBridge_NodeDropTeardown(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-b6")
	defer cancel()
	b := openedBridge(t, srv, conn, "node-b6", "flow-x")

	// 세션 드롭(연결 종료) → HandleConnection 의 defer 가 teardownNodeBridges 호출.
	_ = conn.Close()

	// 최종 offline status 가 방출되어야 한다.
	select {
	case got := <-b.Status():
		assert.Equal(t, BridgeStateOffline, got.Status)
	case <-time.After(time.Second):
		t.Fatal("노드 드롭 시 offline status 미방출")
	}
	// Outputs 채널은 닫혀야 한다(teardown).
	require.Eventually(t, func() bool {
		select {
		case _, ok := <-b.Outputs():
			return !ok
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond, "노드 드롭 후 Outputs 채널은 닫혀야 함")
}
