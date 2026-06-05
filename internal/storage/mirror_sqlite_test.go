package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMirrorRepo(t *testing.T) *MirrorSQLiteRepository {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mirror.db")
	repo, err := NewMirrorSQLiteRepository(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func res(id, node, name, status, def string) MirroredResource {
	return MirroredResource{ID: id, SourceInstanceID: node, Name: name, Status: status, Definition: def, UpdatedAt: 1000}
}

// TestMirror_ReplaceAndList 는 snapshot 교체 후 노드별 조회가 동작하고 모든 행에
// source_instance_id 가 태깅되는지 검증한다(REQ-E03/E04).
func TestMirror_ReplaceAndList(t *testing.T) {
	repo := newMirrorRepo(t)
	ctx := context.Background()

	items := []MirroredResource{
		res("f1", "node-A", "flow1", "running", `{"a":1}`),
		res("f2", "node-A", "flow2", "stopped", `{"b":2}`),
	}
	require.NoError(t, repo.ReplaceFlows(ctx, "node-A", items))

	got, err := repo.ListFlows(ctx, "node-A")
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, m := range got {
		assert.Equal(t, "node-A", m.SourceInstanceID, "모든 행은 출처 노드로 태깅되어야 함(REQ-E04)")
		assert.Equal(t, "flow", m.Kind)
	}
}

// TestMirror_ReplaceIsPerNode 는 한 노드 replace 가 다른 노드 행에 영향을 주지
// 않음을 검증한다(노드별 격리).
func TestMirror_ReplaceIsPerNode(t *testing.T) {
	repo := newMirrorRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.ReplaceFlows(ctx, "node-A", []MirroredResource{res("f1", "node-A", "a", "", "{}")}))
	require.NoError(t, repo.ReplaceFlows(ctx, "node-B", []MirroredResource{res("f1", "node-B", "b", "", "{}")}))

	// node-A 재교체로 비우기 → node-B 는 보존.
	require.NoError(t, repo.ReplaceFlows(ctx, "node-A", nil))

	a, _ := repo.ListFlows(ctx, "node-A")
	b, _ := repo.ListFlows(ctx, "node-B")
	assert.Empty(t, a)
	assert.Len(t, b, 1)
}

// TestMirror_DeltaUpsertAndDelete 는 delta add/update/remove 적용을 검증한다(REQ-E02).
func TestMirror_DeltaUpsertAndDelete(t *testing.T) {
	repo := newMirrorRepo(t)
	ctx := context.Background()

	// add
	require.NoError(t, repo.UpsertResource(ctx, "agent", res("a1", "node-A", "agent1", "online", `{"x":1}`)))
	got, _ := repo.ListAgents(ctx, "node-A")
	require.Len(t, got, 1)
	assert.Equal(t, "agent1", got[0].Name)

	// update (동일 PK)
	require.NoError(t, repo.UpsertResource(ctx, "agent", res("a1", "node-A", "agent1-renamed", "offline", `{"x":2}`)))
	got, _ = repo.ListAgents(ctx, "node-A")
	require.Len(t, got, 1)
	assert.Equal(t, "agent1-renamed", got[0].Name)
	assert.Equal(t, "offline", got[0].Status)

	// remove
	require.NoError(t, repo.DeleteResource(ctx, "node-A", "agent", "a1"))
	got, _ = repo.ListAgents(ctx, "node-A")
	assert.Empty(t, got)
}

// TestMirror_DeleteResourceIsIdempotent 는 중복 remove 가 안전함을 검증한다(REQ-E02
// 중복 델타 내성).
func TestMirror_DeleteResourceIsIdempotent(t *testing.T) {
	repo := newMirrorRepo(t)
	ctx := context.Background()
	assert.NoError(t, repo.DeleteResource(ctx, "node-A", "flow", "missing"))
	assert.NoError(t, repo.DeleteResource(ctx, "node-A", "flow", "missing"))
}

// TestMirror_RowsRetainedAfterReplaceNoNodeState 는 미러 행이 노드 오프라인과
// 무관하게 보존됨을 검증한다(last-known — REQ-E06). MirrorRepository 는 online 상태를
// 보유하지 않으므로 행은 명시적 삭제 전까지 보존된다.
func TestMirror_RowsRetainedAfterReplaceNoNodeState(t *testing.T) {
	repo := newMirrorRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.ReplaceDevices(ctx, "node-A", []MirroredResource{res("d1", "node-A", "dev", "", "{}")}))

	// 오프라인 전이는 managed_nodes 의 책임이며 미러 행에는 영향이 없다 → 여전히 조회됨.
	got, _ := repo.ListDevices(ctx, "node-A")
	assert.Len(t, got, 1, "오프라인이어도 미러 행은 보존되어야 함(last-known)")
}

// TestMirror_ListAllTagsBySource 는 통합 목록이 출처 노드 태그를 포함하는지 검증한다
// (REQ-E05).
func TestMirror_ListAllTagsBySource(t *testing.T) {
	repo := newMirrorRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.ReplaceFlows(ctx, "node-A", []MirroredResource{res("f1", "node-A", "a", "", "{}")}))
	require.NoError(t, repo.ReplaceFlows(ctx, "node-B", []MirroredResource{res("f1", "node-B", "b", "", "{}")}))

	all, err := repo.ListAllFlows(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)
	sources := map[string]bool{}
	for _, m := range all {
		sources[m.SourceInstanceID] = true
	}
	assert.True(t, sources["node-A"])
	assert.True(t, sources["node-B"])
}

// TestMirror_DeleteByNode 는 노드 삭제 시 모든 kind 미러가 정리됨을 검증한다(orphan
// 방지). 다른 노드는 보존된다.
func TestMirror_DeleteByNode(t *testing.T) {
	repo := newMirrorRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.ReplaceFlows(ctx, "node-A", []MirroredResource{res("f1", "node-A", "a", "", "{}")}))
	require.NoError(t, repo.ReplaceAgents(ctx, "node-A", []MirroredResource{res("a1", "node-A", "ag", "", "{}")}))
	require.NoError(t, repo.ReplaceDevices(ctx, "node-A", []MirroredResource{res("d1", "node-A", "dv", "", "{}")}))
	require.NoError(t, repo.ReplaceFlows(ctx, "node-B", []MirroredResource{res("f1", "node-B", "b", "", "{}")}))

	require.NoError(t, repo.DeleteByNode(ctx, "node-A"))

	f, _ := repo.ListFlows(ctx, "node-A")
	a, _ := repo.ListAgents(ctx, "node-A")
	d, _ := repo.ListDevices(ctx, "node-A")
	assert.Empty(t, f)
	assert.Empty(t, a)
	assert.Empty(t, d)

	bf, _ := repo.ListFlows(ctx, "node-B")
	assert.Len(t, bf, 1, "다른 노드 미러는 보존되어야 함")
}

// TestMirror_UnknownKind 는 미지원 kind 가 에러를 반환하는지 검증한다.
func TestMirror_UnknownKind(t *testing.T) {
	repo := newMirrorRepo(t)
	ctx := context.Background()
	assert.Error(t, repo.UpsertResource(ctx, "bogus", res("x", "n", "", "", "{}")))
	assert.Error(t, repo.DeleteResource(ctx, "n", "bogus", "x"))
}

// TestMirror_FactoryAndClose 는 팩토리 생성과 Close 를 검증한다(sqlite/file/default
// 분기 모두 sqlite 구현으로 귀결).
func TestMirror_FactoryAndClose(t *testing.T) {
	for _, typ := range []string{"sqlite", "file", "unknown"} {
		path := filepath.Join(t.TempDir(), "mirror.db")
		repo, err := NewMirrorRepository(context.Background(), typ, path)
		require.NoError(t, err, "type=%s", typ)
		assert.NoError(t, repo.Close())
	}
}

// TestMirror_ListAllAgentsAndDevices 는 통합 agent/device 조회를 검증한다(REQ-E05).
func TestMirror_ListAllAgentsAndDevices(t *testing.T) {
	repo := newMirrorRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.ReplaceAgents(ctx, "node-A", []MirroredResource{res("a1", "node-A", "ag", "", "{}")}))
	require.NoError(t, repo.ReplaceDevices(ctx, "node-A", []MirroredResource{res("d1", "node-A", "dv", "", "{}")}))
	require.NoError(t, repo.ReplaceAgents(ctx, "node-B", []MirroredResource{res("a2", "node-B", "ag2", "", "{}")}))

	agents, err := repo.ListAllAgents(ctx)
	require.NoError(t, err)
	assert.Len(t, agents, 2)
	for _, a := range agents {
		assert.Equal(t, "agent", a.Kind)
	}

	devices, err := repo.ListAllDevices(ctx)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Equal(t, "device", devices[0].Kind)
}

// TestMirror_UpsertAgentDevice 는 agent/device delta upsert 를 검증한다(REQ-E02).
func TestMirror_UpsertAgentDevice(t *testing.T) {
	repo := newMirrorRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.UpsertResource(ctx, "device", res("d1", "node-A", "dev", "online", "{}")))
	d, err := repo.ListDevices(ctx, "node-A")
	require.NoError(t, err)
	require.Len(t, d, 1)
	assert.Equal(t, "device", d[0].Kind)
}

// TestMirror_UpsertDefaultUpdatedAt 는 UpdatedAt=0 입력 시 현재 시각이 채워지는지
// 검증한다.
func TestMirror_UpsertDefaultUpdatedAt(t *testing.T) {
	repo := newMirrorRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.UpsertResource(ctx, "flow", MirroredResource{ID: "f1", SourceInstanceID: "node-A", Kind: "flow"}))
	f, _ := repo.ListFlows(ctx, "node-A")
	require.Len(t, f, 1)
	assert.Greater(t, f[0].UpdatedAt, int64(0), "UpdatedAt=0 입력은 현재 시각으로 채워져야 함")
}

// TestMirror_ReplaceDefaultUpdatedAt 는 snapshot 항목의 UpdatedAt=0 도 채워지는지
// 검증한다.
func TestMirror_ReplaceDefaultUpdatedAt(t *testing.T) {
	repo := newMirrorRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.ReplaceFlows(ctx, "node-A", []MirroredResource{{ID: "f1", SourceInstanceID: "node-A", Kind: "flow"}}))
	f, _ := repo.ListFlows(ctx, "node-A")
	require.Len(t, f, 1)
	assert.Greater(t, f[0].UpdatedAt, int64(0))
}
