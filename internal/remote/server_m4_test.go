// server_m4_test.go 는 M4 서버 측 인벤토리 수신/캐시/목록을 검증한다
// (@SPEC:SPEC-REMOTE-001 M4, REQ-E03/E04/E05/E06/E08).
//
// fakeConn / memManagedNodeRepo 는 server_test.go / registration_test.go 의 것을
// 재사용한다. 미러 저장소는 실제 sqlite 구현(임시 파일)을 사용한다.
package remote

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/storage"
)

func newServerMirror(t *testing.T) storage.MirrorRepository {
	t.Helper()
	repo, err := storage.NewMirrorSQLiteRepository(context.Background(), filepath.Join(t.TempDir(), "mirror.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

// startServerConn 은 서버를 시작하고 hello 로 식별된 노드 연결을 만든다.
func startServerConn(t *testing.T, srv *Server, instanceID string) (*fakeConn, context.CancelFunc) {
	t.Helper()
	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	hello, _ := NewHelloMessage(HelloPayload{InstanceID: instanceID, Hostname: "h", Version: "v"})
	conn.inject(t, hello)
	require.Eventually(t, func() bool {
		_, ok := srv.NodeState(instanceID)
		return ok
	}, time.Second, 5*time.Millisecond)
	return conn, cancel
}

// TestServer_ReceivesSnapshotUpdatesCache 는 inventory_snapshot 수신이 미러 캐시를
// 갱신하는지 검증한다(REQ-E01/E03/E04).
func TestServer_ReceivesSnapshotUpdatesCache(t *testing.T) {
	mirror := newServerMirror(t)
	srv := NewServer(ServerConfig{Mirror: mirror}, nil)
	conn, cancel := startServerConn(t, srv, "node-S1")
	defer cancel()

	snap, _ := NewInventorySnapshotMessage(InventorySnapshotPayload{
		InstanceID: "node-S1",
		Flows:      []InventoryItem{{ID: "f1", Name: "flowA", Kind: KindFlow}},
		Agents:     []InventoryItem{{ID: "a1", Name: "agentA", Kind: KindAgent}},
	})
	conn.inject(t, snap)

	require.Eventually(t, func() bool {
		flows, _ := mirror.ListFlows(context.Background(), "node-S1")
		return len(flows) == 1
	}, time.Second, 5*time.Millisecond)

	flows, _ := mirror.ListFlows(context.Background(), "node-S1")
	assert.Equal(t, "node-S1", flows[0].SourceInstanceID, "출처 노드 태깅(REQ-E04)")
	agents, _ := mirror.ListAgents(context.Background(), "node-S1")
	assert.Len(t, agents, 1)
}

// TestServer_ReceivesDeltaUpdatesCache 는 inventory_delta 가 미러 캐시에 op 를
// 적용하는지 검증한다(REQ-E02/E03).
func TestServer_ReceivesDeltaUpdatesCache(t *testing.T) {
	mirror := newServerMirror(t)
	srv := NewServer(ServerConfig{Mirror: mirror}, nil)
	conn, cancel := startServerConn(t, srv, "node-S2")
	defer cancel()

	// add
	add, _ := NewInventoryDeltaMessage(InventoryDeltaPayload{
		InstanceID: "node-S2", Op: OpAdd, Kind: KindFlow,
		Item: InventoryItem{ID: "f1", Name: "flowA", Kind: KindFlow},
	})
	conn.inject(t, add)
	require.Eventually(t, func() bool {
		f, _ := mirror.ListFlows(context.Background(), "node-S2")
		return len(f) == 1
	}, time.Second, 5*time.Millisecond)

	// remove
	rm, _ := NewInventoryDeltaMessage(InventoryDeltaPayload{
		InstanceID: "node-S2", Op: OpRemove, Kind: KindFlow,
		Item: InventoryItem{ID: "f1"},
	})
	conn.inject(t, rm)
	require.Eventually(t, func() bool {
		f, _ := mirror.ListFlows(context.Background(), "node-S2")
		return len(f) == 0
	}, time.Second, 5*time.Millisecond)
}

// TestServer_SnapshotUsesConnInstanceID 는 페이로드가 다른 instance_id 를 주장해도
// 연결의 인증 instance_id 로 태깅됨을 검증한다(노드 스푸핑 방지).
func TestServer_SnapshotUsesConnInstanceID(t *testing.T) {
	mirror := newServerMirror(t)
	srv := NewServer(ServerConfig{Mirror: mirror}, nil)
	conn, cancel := startServerConn(t, srv, "node-real")
	defer cancel()

	// 페이로드는 "node-spoof" 를 주장하지만 연결은 node-real.
	snap, _ := NewInventorySnapshotMessage(InventorySnapshotPayload{
		InstanceID: "node-spoof",
		Flows:      []InventoryItem{{ID: "f1", Name: "x", Kind: KindFlow}},
	})
	conn.inject(t, snap)

	require.Eventually(t, func() bool {
		f, _ := mirror.ListFlows(context.Background(), "node-real")
		return len(f) == 1
	}, time.Second, 5*time.Millisecond)

	spoofed, _ := mirror.ListFlows(context.Background(), "node-spoof")
	assert.Empty(t, spoofed, "스푸핑 instance_id 로는 태깅되지 않아야 함")
}

// TestServer_AggregatedListTagsByNodeWithOnline 는 통합 목록이 출처 노드 + online
// 플래그를 포함하는지 검증한다(REQ-E05/E06).
func TestServer_AggregatedListTagsByNodeWithOnline(t *testing.T) {
	mirror := newServerMirror(t)
	srv := NewServer(ServerConfig{Mirror: mirror}, nil)
	ctx := context.Background()

	conn1, cancel1 := startServerConn(t, srv, "node-A")
	defer cancel1()
	snapA, _ := NewInventorySnapshotMessage(InventorySnapshotPayload{
		InstanceID: "node-A", Flows: []InventoryItem{{ID: "f1", Name: "a", Kind: KindFlow}},
	})
	conn1.inject(t, snapA)

	// node-B 의 미러를 직접 저장하고(연결 없음 → 오프라인), online=false 를 검증.
	require.NoError(t, mirror.ReplaceFlows(ctx, "node-B", []storage.MirroredResource{
		{ID: "f2", SourceInstanceID: "node-B", Name: "b", Kind: KindFlow},
	}))

	require.Eventually(t, func() bool {
		all, _ := srv.ListAllMirroredFlows(ctx)
		return len(all) == 2
	}, time.Second, 5*time.Millisecond)

	all, err := srv.ListAllMirroredFlows(ctx)
	require.NoError(t, err)
	byNode := map[string]MirroredResourceView{}
	for _, v := range all {
		byNode[v.SourceInstanceID] = v
	}
	assert.True(t, byNode["node-A"].Online, "node-A 는 연결되어 online")
	assert.False(t, byNode["node-B"].Online, "node-B 는 미연결 → offline last-known(REQ-E06)")
}

// TestServer_ListMirroredByNode_UnknownNode 는 알 수 없는 노드 조회가
// ErrManagedNodeNotFound 를 반환하는지 검증한다(핸들러 404 토대 — REQ-E05).
func TestServer_ListMirroredByNode_UnknownNode(t *testing.T) {
	mirror := newServerMirror(t)
	repo := newMemManagedNodeRepo()
	srv := NewServer(ServerConfig{Mirror: mirror, Repo: repo}, nil)

	_, err := srv.ListMirroredFlows(context.Background(), "ghost")
	assert.ErrorIs(t, err, storage.ErrManagedNodeNotFound)
}

// TestServer_DeleteNodeCleansMirror 는 노드 삭제 시 그 노드 미러가 정리되고 다른
// 노드는 보존됨을 검증한다(orphan 방지).
func TestServer_DeleteNodeCleansMirror(t *testing.T) {
	mirror := newServerMirror(t)
	repo := newMemManagedNodeRepo()
	srv := NewServer(ServerConfig{Mirror: mirror, Repo: repo}, nil)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{InstanceID: "node-A", Status: RegStatusApproved}))
	require.NoError(t, mirror.ReplaceFlows(ctx, "node-A", []storage.MirroredResource{{ID: "f1", SourceInstanceID: "node-A", Kind: KindFlow}}))
	require.NoError(t, mirror.ReplaceFlows(ctx, "node-B", []storage.MirroredResource{{ID: "f1", SourceInstanceID: "node-B", Kind: KindFlow}}))

	require.NoError(t, srv.DeleteNode(ctx, "node-A"))

	a, _ := mirror.ListFlows(ctx, "node-A")
	b, _ := mirror.ListFlows(ctx, "node-B")
	assert.Empty(t, a, "삭제된 노드 미러는 정리되어야 함")
	assert.Len(t, b, 1, "다른 노드 미러는 보존되어야 함")
}

// TestServer_NilMirrorIgnoresInventory 는 mirror 미구성 시 인벤토리 메시지를 안전히
// 무시하는지 검증한다(하위 호환).
func TestServer_NilMirrorIgnoresInventory(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil) // Mirror 미구성.
	conn, cancel := startServerConn(t, srv, "node-nm")
	defer cancel()

	snap, _ := NewInventorySnapshotMessage(InventorySnapshotPayload{
		InstanceID: "node-nm", Flows: []InventoryItem{{ID: "f1", Kind: KindFlow}},
	})
	conn.inject(t, snap)

	// 패닉/크래시 없이 노드는 여전히 online 유지.
	require.Eventually(t, func() bool {
		st, ok := srv.NodeState("node-nm")
		return ok && st.Online
	}, time.Second, 5*time.Millisecond)
}

// TestServer_MirroredAgentsAndDevicesListing 는 agent/device 노드별 + 통합 목록
// 메서드를 검증한다(REQ-E05/E06).
func TestServer_MirroredAgentsAndDevicesListing(t *testing.T) {
	mirror := newServerMirror(t)
	repo := newMemManagedNodeRepo()
	srv := NewServer(ServerConfig{Mirror: mirror, Repo: repo}, nil)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{InstanceID: "node-A", Status: RegStatusApproved}))
	require.NoError(t, mirror.ReplaceAgents(ctx, "node-A", []storage.MirroredResource{{ID: "a1", SourceInstanceID: "node-A", Kind: KindAgent}}))
	require.NoError(t, mirror.ReplaceDevices(ctx, "node-A", []storage.MirroredResource{{ID: "d1", SourceInstanceID: "node-A", Kind: KindDevice}}))

	agents, err := srv.ListMirroredAgents(ctx, "node-A")
	require.NoError(t, err)
	require.Len(t, agents, 1)
	assert.False(t, agents[0].Online, "미연결 → offline")

	devices, err := srv.ListMirroredDevices(ctx, "node-A")
	require.NoError(t, err)
	require.Len(t, devices, 1)

	allA, err := srv.ListAllMirroredAgents(ctx)
	require.NoError(t, err)
	assert.Len(t, allA, 1)
	allD, err := srv.ListAllMirroredDevices(ctx)
	require.NoError(t, err)
	assert.Len(t, allD, 1)
}

// TestServer_MirrorListWithoutMirror 는 mirror 미구성 시 목록이 빈 슬라이스를
// 반환함을 검증한다(하위 호환).
func TestServer_MirrorListWithoutMirror(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil) // Mirror 미구성.
	ctx := context.Background()

	flows, err := srv.ListMirroredFlows(ctx, "any")
	require.NoError(t, err)
	assert.Empty(t, flows)

	af, err := srv.ListAllMirroredFlows(ctx)
	require.NoError(t, err)
	assert.Empty(t, af)
	aa, err := srv.ListAllMirroredAgents(ctx)
	require.NoError(t, err)
	assert.Empty(t, aa)
	ad, err := srv.ListAllMirroredDevices(ctx)
	require.NoError(t, err)
	assert.Empty(t, ad)
}

// TestServer_DeleteNodeWithoutRepo 는 repo 미구성 시 DeleteNode 가 ErrNoRepo 를
// 반환함을 검증한다.
func TestServer_DeleteNodeWithoutRepo(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil)
	assert.ErrorIs(t, srv.DeleteNode(context.Background(), "x"), ErrNoRepo)
}

// TestServer_DeltaUnknownOp 는 알 수 없는 op 가 안전히 무시됨을 검증한다.
func TestServer_DeltaUnknownOp(t *testing.T) {
	mirror := newServerMirror(t)
	srv := NewServer(ServerConfig{Mirror: mirror}, nil)
	conn, cancel := startServerConn(t, srv, "node-op")
	defer cancel()

	bad, _ := NewInventoryDeltaMessage(InventoryDeltaPayload{
		InstanceID: "node-op", Op: "bogus", Kind: KindFlow, Item: InventoryItem{ID: "f1"},
	})
	conn.inject(t, bad)

	// 노드는 여전히 online(패닉 없음), 캐시는 비어 있음.
	require.Eventually(t, func() bool {
		st, ok := srv.NodeState("node-op")
		return ok && st.Online
	}, time.Second, 5*time.Millisecond)
	f, _ := mirror.ListFlows(context.Background(), "node-op")
	assert.Empty(t, f)
}

// TestServer_EnsureNodeExistsInMemory 는 repo 미구성 시 in-memory 노드 존재로
// 검증이 동작함을 확인한다(REQ-E05).
func TestServer_EnsureNodeExistsInMemory(t *testing.T) {
	mirror := newServerMirror(t)
	srv := NewServer(ServerConfig{Mirror: mirror}, nil) // repo 없음.
	conn, cancel := startServerConn(t, srv, "node-mem")
	defer cancel()
	_ = conn

	// in-memory 에 노드가 있으므로 목록 조회 성공(빈 결과).
	flows, err := srv.ListMirroredFlows(context.Background(), "node-mem")
	require.NoError(t, err)
	assert.Empty(t, flows)

	// 미존재 노드는 404.
	_, err = srv.ListMirroredFlows(context.Background(), "ghost")
	assert.ErrorIs(t, err, storage.ErrManagedNodeNotFound)
}

// TestServer_MirroredViewCarriesSourceAndOnline 는 뷰가 출처 노드 + online 플래그를
// 운반하는지 검증한다(핸들러 DTO 변환 토대 — REQ-E04/E05/E06). storage.MirroredResource
// 자체는 JSON 태그가 없으므로(저장소 모델), 응답 직렬화는 핸들러 DTO 가 담당한다.
func TestServer_MirroredViewCarriesSourceAndOnline(t *testing.T) {
	v := MirroredResourceView{
		MirroredResource: storage.MirroredResource{ID: "1", SourceInstanceID: "n", Kind: KindFlow},
		Online:           true,
	}
	assert.Equal(t, "n", v.SourceInstanceID)
	assert.True(t, v.Online)
	// 직렬화 자체가 패닉 없이 동작하는지(핸들러 변환 가정) 확인.
	_, err := json.Marshal(v)
	require.NoError(t, err)
}
