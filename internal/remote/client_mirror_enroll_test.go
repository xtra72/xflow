// client_mirror_enroll_test.go 는 첫 가입(enrollment) 세션에서 노드가 세션 도중
// 승인(register_ack approved → node_token)되었을 때, 같은 세션에서 인벤토리 미러가
// 시작되어 inventory_snapshot 을 push 하는지 검증한다
// (@SPEC:SPEC-REMOTE-001 M4, REQ-E01 — 첫 세션 미러 시작 회귀 가드).
//
// 회귀 배경: 과거 runSession 은 세션 시작 시점의 hasToken() 으로만 미러를 시작했다.
// 첫 enrollment 세션은 토큰 없이 dial→register 하고, 토큰은 동일 세션의 read loop
// 에서 register_ack 로 뒤늦게 도착하므로, 그 세션에서는 미러가 영원히 시작되지 않아
// 서버 미러가 빈 상태로 고착되었다(소켓은 ESTABLISHED, last_seen 고정).
//
// clientFakeConn / fakeInventorySource / readMirrorMsg 등은 기존 테스트 하네스를
// 재사용한다(client_test.go, client_m4_test.go).
package remote

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClient_StartsMirrorOnMidSessionApproval 는 토큰 없이 시작한 첫 세션에서
// 세션 도중 approved register_ack(node_token)가 도착하면, 같은 세션에서
// inventory_snapshot 이 push 되는지 검증한다(REQ-E01).
//
// 수정 전: 미러가 시작되지 않아 스냅샷이 오지 않으므로 FAIL.
// 수정 후: 중도 승인 시 미러가 시작되어 스냅샷이 도착하므로 PASS.
func TestClient_StartsMirrorOnMidSessionApproval(t *testing.T) {
	src := &fakeInventorySource{
		flows:   []InventoryItem{invItem("f1", "flowA", KindFlow, nil)},
		agents:  []InventoryItem{invItem("a1", "agentA", KindAgent, nil)},
		devices: []InventoryItem{invItem("d1", "devA", KindDevice, nil)},
	}

	conn := newClientFakeConn()
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) { return conn, nil })

	// DataDir 없음 → 시작 시 토큰 미보유 → 첫 메시지는 register(enrollment 경로).
	cli := NewClient(ClientConfig{
		ServerURL:             "wss://example/api/remote/ws",
		InstanceID:            "node-enroll",
		HeartbeatInterval:     time.Hour,
		InventoryPollInterval: time.Hour, // 스냅샷만으로 판정(델타 poll 간섭 배제).
		Inventory:             src,
		Exposure:              ExposureSummary{Flows: ExposeAll, Agents: ExposeAll, Devices: ExposeAll},
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cli.Start(ctx)
	defer cli.Stop()

	// 1) 첫 메시지는 register(토큰 미보유).
	typ, _ := readMirrorMsg(t, conn)
	require.Equal(t, TypeRegister, typ, "토큰 미보유 → 첫 메시지는 register 여야 함")

	// 2) 서버가 같은 세션에서 approved register_ack(node_token)를 보낸다(중도 승인).
	ack, err := NewRegisterAckMessage(RegisterAckPayload{
		Status:    RegStatusApproved,
		NodeToken: "enroll-token-xyz",
	})
	require.NoError(t, err)
	ackData, err := ack.Encode()
	require.NoError(t, err)
	conn.toClient <- ackData

	// 3) 같은 세션에서 inventory_snapshot 이 push 되어야 한다(수정 전에는 오지 않음).
	typ, payload := readMirrorMsg(t, conn)
	require.Equal(t, TypeInventorySnapshot, typ,
		"중도 승인 후 같은 세션에서 inventory_snapshot 이 push 되어야 함")

	var snap InventorySnapshotPayload
	require.NoError(t, json.Unmarshal(payload, &snap))
	assert.Equal(t, "node-enroll", snap.InstanceID)
	require.Len(t, snap.Flows, 1)
	assert.Equal(t, "flowA", snap.Flows[0].Name)
	require.Len(t, snap.Agents, 1)
	require.Len(t, snap.Devices, 1)
}

// TestClient_NoMirrorWhenPendingThenSessionEnds 는 미승인(pending) 노드가 승인
// 신호 없이 세션이 끝나면, 대기 중이던 미러 고루틴이 누수 없이 정리되고 스냅샷을
// 보내지 않음을 검증한다(goroutine leak 방지 + 미승인 미러 비활성).
func TestClient_NoMirrorWhenPendingThenSessionEnds(t *testing.T) {
	src := &fakeInventorySource{flows: []InventoryItem{invItem("f1", "flowA", KindFlow, nil)}}

	conn := newClientFakeConn()
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) { return conn, nil })

	cli := NewClient(ClientConfig{
		ServerURL:             "wss://example/api/remote/ws",
		InstanceID:            "node-pending",
		HeartbeatInterval:     time.Hour,
		InventoryPollInterval: 10 * time.Millisecond,
		Inventory:             src,
		Exposure:              ExposureSummary{Flows: ExposeAll},
		// DataDir 없음 → 미승인 유지.
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	cli.Start(ctx)

	// 첫 메시지는 register. 이후 pending(승인 신호 없음).
	typ, _ := readMirrorMsg(t, conn)
	require.Equal(t, TypeRegister, typ)

	// 서버 → pending ack(토큰 없음). 미러는 시작되면 안 된다.
	ack, _ := NewRegisterAckMessage(RegisterAckPayload{Status: RegStatusPending})
	ackData, _ := ack.Encode()
	conn.toClient <- ackData

	// 스냅샷이 오지 않아야 한다.
	select {
	case data := <-conn.fromClient:
		msg, _ := DecodeMessage(data)
		assert.NotEqual(t, TypeInventorySnapshot, msg.Type, "pending 노드는 스냅샷을 보내면 안 됨")
	case <-time.After(60 * time.Millisecond):
		// 기대 경로: 미러 메시지 없음.
	}

	// 세션 종료 → 대기 중이던 미러 고루틴이 정리되어야 한다(Stop 이 wg.Wait 로 검증).
	cancel()
	cli.Stop()
}
