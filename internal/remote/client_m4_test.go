// client_m4_test.go 는 M4 클라이언트 측 인벤토리 미러 송신을 검증한다
// (@SPEC:SPEC-REMOTE-001 M4, REQ-E01/E02/E07, A04/A07, F06).
//
// clientFakeConn 은 client_test.go 의 것을 재사용한다.
package remote

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeInventorySource 는 InventorySource 의 테스트 구현이다. mutex 로 보호되어
// 테스트가 런타임에 자원을 변경하면 다음 poll 이 반영한다.
type fakeInventorySource struct {
	mu      sync.Mutex
	flows   []InventoryItem
	agents  []InventoryItem
	devices []InventoryItem
}

func (s *fakeInventorySource) ListFlows(_ context.Context) ([]InventoryItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]InventoryItem(nil), s.flows...), nil
}
func (s *fakeInventorySource) ListAgents(_ context.Context) ([]InventoryItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]InventoryItem(nil), s.agents...), nil
}
func (s *fakeInventorySource) ListDevices(_ context.Context) ([]InventoryItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]InventoryItem(nil), s.devices...), nil
}
func (s *fakeInventorySource) setFlows(items ...InventoryItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flows = items
}

// startMirrorClient 는 승인 상태 + Inventory 소스를 가진 클라이언트를 시작한다.
func startMirrorClient(t *testing.T, src InventorySource, exp ExposureSummary, poll time.Duration) (*Client, *clientFakeConn, context.CancelFunc) {
	t.Helper()
	conn := newClientFakeConn()
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) { return conn, nil })

	dir := t.TempDir()
	require.NoError(t, SaveNodeToken(dir, "node-token-m4"))

	cli := NewClient(ClientConfig{
		ServerURL:             "wss://example/api/remote/ws",
		InstanceID:            "node-m4",
		HeartbeatInterval:     time.Hour,
		InventoryPollInterval: poll,
		DataDir:               dir,
		Inventory:             src,
		Exposure:              exp,
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	cli.Start(ctx)

	// 토큰 보유 → 첫 메시지는 hello. 소비한다.
	select {
	case <-conn.fromClient:
	case <-time.After(time.Second):
		t.Fatal("hello 송신 타임아웃")
	}
	return cli, conn, cancel
}

// readMirrorMsg 는 클라이언트가 보낸 다음 메시지를 읽고 디코드한다(heartbeat 스킵).
func readMirrorMsg(t *testing.T, conn *clientFakeConn) (string, []byte) {
	t.Helper()
	for {
		select {
		case data := <-conn.fromClient:
			msg, err := DecodeMessage(data)
			require.NoError(t, err)
			if msg.Type == TypeHeartbeat {
				continue
			}
			return msg.Type, msg.Payload
		case <-time.After(2 * time.Second):
			t.Fatal("미러 메시지 수신 타임아웃")
			return "", nil
		}
	}
}

func invItem(id, name, kind string, def map[string]any) InventoryItem {
	raw, _ := json.Marshal(def)
	return InventoryItem{ID: id, Name: name, Kind: kind, Definition: raw}
}

// TestClient_PushesSnapshotOnConnect 는 접속 시 노출 범위로 필터된 스냅샷을 push
// 하는지 검증한다(REQ-E01/E07).
func TestClient_PushesSnapshotOnConnect(t *testing.T) {
	src := &fakeInventorySource{
		flows:   []InventoryItem{invItem("f1", "flowA", KindFlow, nil), invItem("f2", "flowB", KindFlow, nil)},
		agents:  []InventoryItem{invItem("a1", "agentA", KindAgent, nil)},
		devices: []InventoryItem{invItem("d1", "devA", KindDevice, nil)},
	}
	// flows: 명시 목록(flowA 만), agents: all, devices: none.
	_, conn, cancel := startMirrorClient(t, src, ExposureSummary{Flows: "flowA", Agents: ExposeAll, Devices: ExposeNone}, time.Hour)
	defer cancel()

	typ, payload := readMirrorMsg(t, conn)
	require.Equal(t, TypeInventorySnapshot, typ)

	var snap InventorySnapshotPayload
	require.NoError(t, json.Unmarshal(payload, &snap))
	assert.Equal(t, "node-m4", snap.InstanceID)
	require.Len(t, snap.Flows, 1, "노출 목록(flowA)만 포함되어야 함")
	assert.Equal(t, "flowA", snap.Flows[0].Name)
	assert.Len(t, snap.Agents, 1, "agents=all → 전부")
	assert.Empty(t, snap.Devices, "devices=none → 비노출")
}

// TestClient_RedactionPreservedInSnapshot 는 소스가 redaction 한 Definition 이
// 그대로 송신되어 시크릿이 페이로드에 없음을 검증한다(REQ-F06).
//
// 미러 엔진은 소스가 제공한 Definition 을 변형하지 않으므로, 소스가 시크릿을 제거한
// 정의를 주면 페이로드에도 시크릿이 없다. 본 테스트는 그 계약(소스 redaction → 송신
// 보존)을 검증한다.
func TestClient_RedactionPreservedInSnapshot(t *testing.T) {
	// 소스가 이미 redaction 한 정의(password 키 없음).
	src := &fakeInventorySource{
		flows: []InventoryItem{invItem("f1", "flowA", KindFlow, map[string]any{"host": "h", "port": 1})},
	}
	_, conn, cancel := startMirrorClient(t, src, ExposureSummary{Flows: ExposeAll}, time.Hour)
	defer cancel()

	_, payload := readMirrorMsg(t, conn)
	assert.NotContains(t, string(payload), "password", "redacted 정의에 시크릿 키가 없어야 함")
	assert.NotContains(t, string(payload), "secret")
}

// TestClient_EmitsDeltaOnChange 는 자원 변경 시 델타가 push 되는지 검증한다(REQ-E02).
func TestClient_EmitsDeltaOnChange(t *testing.T) {
	src := &fakeInventorySource{flows: []InventoryItem{invItem("f1", "flowA", KindFlow, nil)}}
	_, conn, cancel := startMirrorClient(t, src, ExposureSummary{Flows: ExposeAll}, 15*time.Millisecond)
	defer cancel()

	// 초기 스냅샷 소비.
	typ, _ := readMirrorMsg(t, conn)
	require.Equal(t, TypeInventorySnapshot, typ)

	// 자원 추가 → 다음 poll 이 add 델타를 보내야 함.
	src.setFlows(invItem("f1", "flowA", KindFlow, nil), invItem("f2", "flowB", KindFlow, nil))

	typ, payload := readMirrorMsg(t, conn)
	require.Equal(t, TypeInventoryDelta, typ)
	var d InventoryDeltaPayload
	require.NoError(t, json.Unmarshal(payload, &d))
	assert.Equal(t, OpAdd, d.Op)
	assert.Equal(t, "f2", d.Item.ID)
	assert.Equal(t, "node-m4", d.InstanceID)
}

// TestClient_ExposureChangeEmitsRemove 는 노출 축소(A07) 시 remove 델타가 push
// 되는지 검증한다(REQ-A07 — 노출 해제 자원 제거 신호).
func TestClient_ExposureChangeEmitsRemove(t *testing.T) {
	src := &fakeInventorySource{flows: []InventoryItem{
		invItem("f1", "flowA", KindFlow, nil),
		invItem("f2", "flowB", KindFlow, nil),
	}}
	cli, conn, cancel := startMirrorClient(t, src, ExposureSummary{Flows: ExposeAll}, time.Hour)
	defer cancel()

	// 초기 스냅샷(2개) 소비.
	typ, payload := readMirrorMsg(t, conn)
	require.Equal(t, TypeInventorySnapshot, typ)
	var snap InventorySnapshotPayload
	require.NoError(t, json.Unmarshal(payload, &snap))
	require.Len(t, snap.Flows, 2)

	// 노출을 flowA 만으로 축소 → flowB 는 remove 로 신호되어야 함.
	cli.UpdateExposure(ExposureSummary{Flows: "flowA"})

	typ, payload = readMirrorMsg(t, conn)
	require.Equal(t, TypeInventoryDelta, typ)
	var d InventoryDeltaPayload
	require.NoError(t, json.Unmarshal(payload, &d))
	assert.Equal(t, OpRemove, d.Op)
	assert.Equal(t, "f2", d.Item.ID)
}

// failFirstSource 는 첫 N 회 List 호출만 실패하고 이후 정상 반환하는 소스이다
// (스냅샷 실패 → 이후 poll 복구를 결정적으로 검증).
type failFirstSource struct {
	mu        sync.Mutex
	failsLeft int
	flows     []InventoryItem
}

func (s *failFirstSource) maybeFail() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failsLeft > 0 {
		s.failsLeft--
		return true
	}
	return false
}
func (s *failFirstSource) ListFlows(_ context.Context) ([]InventoryItem, error) {
	if s.maybeFail() {
		return nil, assertErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]InventoryItem(nil), s.flows...), nil
}
func (s *failFirstSource) ListAgents(_ context.Context) ([]InventoryItem, error) { return nil, nil }
func (s *failFirstSource) ListDevices(_ context.Context) ([]InventoryItem, error) {
	return nil, nil
}

// TestClient_SnapshotErrorKeepsLoop 는 스냅샷 빌드 실패 시에도 미러 루프가 유지되어
// 다음 poll 에 복구함을 검증한다(견고성). 첫 ListFlows(=스냅샷)만 실패시킨다.
func TestClient_SnapshotErrorKeepsLoop(t *testing.T) {
	src := &failFirstSource{failsLeft: 1, flows: []InventoryItem{invItem("f1", "flowA", KindFlow, nil)}}
	_, conn, cancel := startMirrorClient(t, src, ExposureSummary{Flows: ExposeAll}, 15*time.Millisecond)
	defer cancel()

	// 첫 스냅샷은 실패(메시지 없음). 이후 poll 의 diff(baseline={} vs [f1])가 add 델타.
	typ, payload := readMirrorMsg(t, conn)
	require.Equal(t, TypeInventoryDelta, typ, "스냅샷 실패 후에도 루프가 유지되어 델타를 보내야 함")
	var d InventoryDeltaPayload
	require.NoError(t, json.Unmarshal(payload, &d))
	assert.Equal(t, OpAdd, d.Op)
}

var assertErr = assertError("inventory source failure")

type assertError string

func (e assertError) Error() string { return string(e) }

// TestClient_UpdateExposureCoalesces 는 활성 루프 없이도 UpdateExposure 가 패닉 없이
// 신호를 흡수하고, 현재 노출을 갱신함을 검증한다(A07).
func TestClient_UpdateExposureCoalesces(t *testing.T) {
	cli := NewClient(ClientConfig{
		ServerURL:  "wss://x",
		InstanceID: "n",
		Exposure:   ExposureSummary{Flows: ExposeNone},
	}, DialerFunc(func(_ context.Context, _ string) (Conn, error) { return newClientFakeConn(), nil }))

	cli.UpdateExposure(ExposureSummary{Flows: ExposeAll})
	cli.UpdateExposure(ExposureSummary{Flows: "f1"}) // coalesce(채널 버퍼 1).
	assert.Equal(t, "f1", cli.currentExposure().Flows)
}

// TestClient_NoMirrorWithoutToken 는 미승인(토큰 미보유) 노드가 미러링하지 않음을
// 검증한다(REQ-C06/F03 정신 — 미승인 노드는 미러링 비활성).
func TestClient_NoMirrorWithoutToken(t *testing.T) {
	src := &fakeInventorySource{flows: []InventoryItem{invItem("f1", "flowA", KindFlow, nil)}}
	conn := newClientFakeConn()
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) { return conn, nil })

	cli := NewClient(ClientConfig{
		ServerURL:             "wss://example/api/remote/ws",
		InstanceID:            "node-noauth",
		HeartbeatInterval:     time.Hour,
		InventoryPollInterval: 10 * time.Millisecond,
		Inventory:             src,
		Exposure:              ExposureSummary{Flows: ExposeAll},
		// DataDir 없음 → 토큰 미보유 → 미승인.
	}, dialer)
	ctx, cancel := context.WithCancel(context.Background())
	cli.Start(ctx)
	defer cli.Stop()
	defer cancel()

	// 첫 메시지는 register(미승인). 그 이후 inventory_snapshot 이 와서는 안 된다.
	typ, _ := readMirrorMsg(t, conn)
	require.Equal(t, TypeRegister, typ)

	select {
	case data := <-conn.fromClient:
		msg, _ := DecodeMessage(data)
		assert.NotEqual(t, TypeInventorySnapshot, msg.Type, "미승인 노드는 스냅샷을 보내면 안 됨")
	case <-time.After(60 * time.Millisecond):
		// 기대 경로: 미러 메시지 없음.
	}
}
