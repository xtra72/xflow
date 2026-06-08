// grouping_test.go 는 v1.4(M9, 그룹 K) 서버 측 노드 그룹핑 + BASIC 시스템 정보 수신
// + uptime 파생 + 노드별 운영 요약(미러 파생)을 검증한다(@SPEC:SPEC-REMOTE-001 M9,
// REQ-K02~K05/K07/K08/K10).
//
// 검증 범위:
//   - SetNodeGroup/ClearNodeGroup/ListGroups(서버 메서드, 노드 명령 비전파 — A13).
//   - register 가 시스템 정보를 저장(REQ-K07/K08), heartbeat 가 갱신(미보고는 보존 — K09).
//   - uptime = now - started_at 파생, started_at==0 미보고 시 미표시(REQ-K08).
//   - NodeSummary/NodeDetail 미러 파생 요약(오프라인 last-known — REQ-K10/E06).
package remote

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/storage"
)

// newGroupingServer 는 repo + mirror + tokenIssuer 가 주입된 서버를 생성한다(M9 검증용).
func newGroupingServer(t *testing.T) (*Server, *memManagedNodeRepo, storage.MirrorRepository) {
	t.Helper()
	repo := newMemManagedNodeRepo()
	mirror := newServerMirror(t)
	srv := NewServer(ServerConfig{
		Repo:        repo,
		Mirror:      mirror,
		TokenIssuer: newFakeTokenIssuer(),
	}, nil)
	return srv, repo, mirror
}

// TestServer_SetAndClearNodeGroup 는 그룹 배정/해제가 repo 에 반영되고 노드로 명령을
// 전파하지 않는지(서버 메서드 직접 호출 — A13) 검증한다(REQ-K02).
func TestServer_SetAndClearNodeGroup(t *testing.T) {
	srv, repo, _ := newGroupingServer(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{InstanceID: "n", Status: RegStatusApproved}))

	require.NoError(t, srv.SetNodeGroup(ctx, "n", "prod"))
	got, _ := repo.Get(ctx, "n")
	assert.Equal(t, "prod", got.GroupName)

	require.NoError(t, srv.ClearNodeGroup(ctx, "n"))
	got, _ = repo.Get(ctx, "n")
	assert.Empty(t, got.GroupName, "그룹 해제 시 '전체'로 환원")
}

// TestServer_SetNodeGroupNotFound 는 미존재 노드 그룹 배정 시 ErrManagedNodeNotFound.
func TestServer_SetNodeGroupNotFound(t *testing.T) {
	srv, _, _ := newGroupingServer(t)
	err := srv.SetNodeGroup(context.Background(), "missing", "g")
	assert.ErrorIs(t, err, storage.ErrManagedNodeNotFound)
}

// TestServer_ListGroups 는 distinct 그룹 + "전체" 버킷 카운트를 반환하는지 검증한다(REQ-K03).
func TestServer_ListGroups(t *testing.T) {
	srv, repo, _ := newGroupingServer(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{InstanceID: "a", Status: RegStatusApproved}))
	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{InstanceID: "b", Status: RegStatusApproved}))
	require.NoError(t, srv.SetNodeGroup(ctx, "a", "prod"))
	// b 는 미지정(전체).

	groups, err := srv.ListGroups(ctx)
	require.NoError(t, err)
	counts := map[string]int{}
	for _, g := range groups {
		counts[g.GroupName] = g.NodeCount
	}
	assert.Equal(t, 1, counts[""], "전체 버킷 1개")
	assert.Equal(t, 1, counts["prod"])
}

// TestServer_ListGroupsNoRepo 는 repo 미구성(M1 모드)에서 빈 목록을 반환하는지 검증한다.
func TestServer_ListGroupsNoRepo(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil)
	groups, err := srv.ListGroups(context.Background())
	require.NoError(t, err)
	assert.Empty(t, groups)
}

// TestServer_RegisterStoresSystemInfo 는 register 페이로드의 BASIC 시스템 정보가
// managed_nodes 에 저장되는지 검증한다(REQ-K07/K08).
func TestServer_RegisterStoresSystemInfo(t *testing.T) {
	srv, repo, _ := newGroupingServer(t)
	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{
		InstanceID: "n", Hostname: "h", Version: "1.2.3",
		OS: "linux", Arch: "arm64", StartedAt: 5000,
	})
	_ = readAck(t, conn)

	require.Eventually(t, func() bool {
		got, err := repo.Get(context.Background(), "n")
		return err == nil && got.OS == "linux"
	}, time.Second, 10*time.Millisecond)

	got, _ := repo.Get(context.Background(), "n")
	assert.Equal(t, "linux", got.OS)
	assert.Equal(t, "arm64", got.Arch)
	assert.Equal(t, int64(5000), got.StartedAt)
}

// TestServer_HeartbeatUpdatesSystemInfoPreservesOmitted 는 heartbeat 가 제공된 시스템
// 정보를 갱신하되, 후속 heartbeat 가 일부를 생략하면 기존값을 보존하는지 검증한다
// (REQ-K08/K09 하위 호환).
func TestServer_HeartbeatUpdatesSystemInfoPreservesOmitted(t *testing.T) {
	srv, repo, _ := newGroupingServer(t)
	ctx := context.Background()
	// approved 노드를 미리 등록(토큰 보유 재접속 경로 모사).
	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{InstanceID: "n", Status: RegStatusApproved, TokenID: "jti"}))

	conn := newFakeConn()
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = srv.HandleConnectionAuth(connCtx, conn, "n") }()

	// 첫 heartbeat: 전체 시스템 정보.
	hb1, _ := NewHeartbeatMessageWithInfo("n", "linux", "amd64", "1.0.0", 7000, 0, 0)
	conn.inject(t, hb1)
	require.Eventually(t, func() bool {
		got, err := repo.Get(ctx, "n")
		return err == nil && got.StartedAt == 7000
	}, time.Second, 10*time.Millisecond)

	// 둘째 heartbeat: 시스템 정보 생략(구버전 노드 모사) → 기존값 보존(REQ-K09).
	hb2, _ := NewHeartbeatMessage("n")
	conn.inject(t, hb2)
	// 약간 대기 후 보존 확인.
	time.Sleep(50 * time.Millisecond)
	got, _ := repo.Get(ctx, "n")
	assert.Equal(t, "linux", got.OS, "생략된 os 는 기존값 보존")
	assert.Equal(t, "amd64", got.Arch, "생략된 arch 는 기존값 보존")
	assert.Equal(t, int64(7000), got.StartedAt, "생략된 started_at 은 기존값 보존")
}

// TestServer_NodeSummaryFromMirror 는 노드별 운영 요약이 미러에서 파생되는지 검증한다
// (REQ-K10/A15).
func TestServer_NodeSummaryFromMirror(t *testing.T) {
	srv, repo, mirror := newGroupingServer(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{InstanceID: "n", Status: RegStatusApproved}))
	require.NoError(t, mirror.ReplaceFlows(ctx, "n", []storage.MirroredResource{
		{ID: "f1", SourceInstanceID: "n", Kind: "flow", Status: "running"},
		{ID: "f2", SourceInstanceID: "n", Kind: "flow", Status: "stopped"},
	}))
	require.NoError(t, mirror.ReplaceDevices(ctx, "n", []storage.MirroredResource{
		{ID: "d1", SourceInstanceID: "n", Kind: "device", Status: "online"},
	}))

	sum, err := srv.NodeSummary(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, 2, sum.Flows.Total)
	assert.Equal(t, 1, sum.Flows.Running)
	assert.Equal(t, 1, sum.Flows.Stopped)
	assert.Equal(t, 1, sum.Devices.Total)
	assert.Equal(t, 1, sum.Devices.Online)
}

// TestServer_NodeSummaryUnknownNode 는 미존재 노드 요약이 ErrManagedNodeNotFound 인지 검증한다.
func TestServer_NodeSummaryUnknownNode(t *testing.T) {
	srv, _, _ := newGroupingServer(t)
	_, err := srv.NodeSummary(context.Background(), "missing")
	assert.ErrorIs(t, err, storage.ErrManagedNodeNotFound)
}

// TestServer_NodeDetailWithUptime 는 노드 상세가 메타+시스템 정보+uptime+운영 요약을
// 결합하고, uptime 이 started_at 으로 파생되는지 검증한다(REQ-K08/K10).
func TestServer_NodeDetailWithUptime(t *testing.T) {
	srv, repo, mirror := newGroupingServer(t)
	ctx := context.Background()
	startedAt := time.Now().Add(-1 * time.Hour).UnixMilli()
	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{
		InstanceID: "n", Hostname: "host", Version: "1.0.0", Status: RegStatusApproved,
		OS: "linux", Arch: "arm64", StartedAt: startedAt,
	}))
	require.NoError(t, mirror.ReplaceAgents(ctx, "n", []storage.MirroredResource{
		{ID: "a1", SourceInstanceID: "n", Kind: "agent", Status: "connected"},
	}))

	detail, err := srv.NodeDetail(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, "host", detail.Node.Hostname)
	assert.Equal(t, "linux", detail.Node.OS)
	assert.True(t, detail.HasUptime, "started_at>0 이면 uptime 표시 가능")
	assert.Greater(t, detail.UptimeMs, int64(0), "uptime 은 양수여야 함")
	assert.InDelta(t, time.Hour.Milliseconds(), detail.UptimeMs, float64(time.Minute.Milliseconds()))
	assert.Equal(t, 1, detail.Summary.Agents.Total)
	assert.Equal(t, 1, detail.Summary.Agents.Connected)
}

// TestServer_NodeDetailNoUptimeWhenStartedAtZero 는 started_at==0(미보고)일 때 uptime
// 이 미표시(-1, HasUptime=false)인지 검증한다(REQ-K08/K09 하위 호환).
func TestServer_NodeDetailNoUptimeWhenStartedAtZero(t *testing.T) {
	srv, repo, _ := newGroupingServer(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{InstanceID: "n", Status: RegStatusApproved}))

	detail, err := srv.NodeDetail(ctx, "n")
	require.NoError(t, err)
	assert.False(t, detail.HasUptime, "started_at==0 이면 uptime 미표시")
	assert.Equal(t, int64(-1), detail.UptimeMs)
}

// TestServer_NodeDetailUnknownNode 는 미존재 노드 상세가 ErrManagedNodeNotFound 인지 검증한다.
func TestServer_NodeDetailUnknownNode(t *testing.T) {
	srv, _, _ := newGroupingServer(t)
	_, err := srv.NodeDetail(context.Background(), "missing")
	assert.ErrorIs(t, err, storage.ErrManagedNodeNotFound)
}

// TestServer_NodeSummaryOfflineLastKnown 는 노드 오프라인 시에도 last-known 미러로
// 요약을 제공하는지 검증한다(REQ-E06/A15). 미러 행은 online 과 무관하게 보존된다.
func TestServer_NodeSummaryOfflineLastKnown(t *testing.T) {
	srv, repo, mirror := newGroupingServer(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{InstanceID: "n", Status: RegStatusApproved, Online: false}))
	require.NoError(t, mirror.ReplaceFlows(ctx, "n", []storage.MirroredResource{
		{ID: "f1", SourceInstanceID: "n", Kind: "flow", Status: "running"},
	}))

	detail, err := srv.NodeDetail(ctx, "n")
	require.NoError(t, err)
	assert.False(t, detail.Online, "오프라인 노드")
	assert.Equal(t, 1, detail.Summary.Flows.Total, "오프라인이어도 last-known 미러로 요약 제공")
}

// TestServer_HandleHeartbeatNoRepo 는 repo 미구성(M1 모드)에서 heartbeat 시스템 정보
// 처리가 안전하게 no-op 인지 검증한다(패닉/에러 없음).
func TestServer_HandleHeartbeatNoRepo(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil)
	// repo 미구성 — 디코드 가능한 payload 라도 저장 경로는 no-op.
	hb, _ := NewHeartbeatMessageWithInfo("n", "linux", "amd64", "1.0", 100, 0, 0)
	srv.handleHeartbeat(context.Background(), "n", hb.Payload)
}

// TestServer_HandleHeartbeatDecodeFail 는 손상된 heartbeat payload 가 안전하게
// 무시되는지 검증한다(에러 없음).
func TestServer_HandleHeartbeatDecodeFail(t *testing.T) {
	srv, _, _ := newGroupingServer(t)
	srv.handleHeartbeat(context.Background(), "n", []byte(`{not json`))
}

// TestServer_StoreSystemInfoAllEmptyNoOp 는 모든 필드가 비어 있으면(구버전 노드) 저장이
// no-op 으로 회귀를 피하는지 검증한다(REQ-K09).
func TestServer_StoreSystemInfoAllEmptyNoOp(t *testing.T) {
	srv, repo, _ := newGroupingServer(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{
		InstanceID: "n", Status: RegStatusApproved, OS: "linux", Arch: "arm64", StartedAt: 1000,
	}))

	// 빈 보고 → 기존값 보존(no-op).
	srv.storeSystemInfo(ctx, "n", "", "", 0, 0, 0)
	got, _ := repo.Get(ctx, "n")
	assert.Equal(t, "linux", got.OS)
	assert.Equal(t, int64(1000), got.StartedAt)
}

// TestDeriveUptime 는 uptime 파생 경계(미보고/시계 역전/정상)를 검증한다(REQ-K08).
func TestDeriveUptime(t *testing.T) {
	// 미보고(started_at<=0) → (-1, false).
	up, ok := deriveUptime(0, 1000)
	assert.Equal(t, int64(-1), up)
	assert.False(t, ok)

	// 정상 → (now - started, true).
	up, ok = deriveUptime(1000, 5000)
	assert.Equal(t, int64(4000), up)
	assert.True(t, ok)

	// 시계 역전(now<started) → 0 클램프.
	up, ok = deriveUptime(5000, 1000)
	assert.Equal(t, int64(0), up)
	assert.True(t, ok)
}
