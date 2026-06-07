// managed_node_group_test.go 는 v1.4(M9, 그룹 K)의 노드 그룹핑 + BASIC 시스템 정보
// 저장소 동작을 검증한다(@SPEC:SPEC-REMOTE-001 M9, REQ-K01~K05/K08/K09).
//
// 검증 범위:
//   - 기존 DB(v1.3 스키마)에 group_name/os/arch/started_at 컬럼을 멱등하게 추가(REQ-K09).
//   - SetNodeGroup 배정/변경/해제("전체" 환원, REQ-K02/K05).
//   - ListGroups distinct + 카운트(항상 "전체" 포함, 빈 그룹 자동 소멸, REQ-K03/K05).
//   - register upsert 가 관리자 배정 group_name 을 clobber 하지 않음(REQ-K02 핵심).
//   - SetSystemInfo 제공 시 갱신, 미제공 시 기존값 보존(REQ-K08/K09 하위 호환).
//   - Get/List 가 신규 필드를 운반(REQ-K08).
package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestManagedNode_MigrationAddsColumnsIdempotent 는 group_name/os/arch/started_at
// 컬럼이 없는 기존(v1.3) DB 에 멱등하게 컬럼을 추가하는지 검증한다(REQ-K09).
//
// 먼저 신규 컬럼이 없는 구버전 managed_nodes 스키마를 손수 만들고 행을 1개 넣은 뒤,
// 동일 경로로 repo 를 열어(마이그레이션 수행) 신규 컬럼이 추가되고 기존 행이 빈값/0
// 기본값으로 읽히는지(회귀 0), 그리고 두 번째 open(이미 컬럼 존재)도 안전한지 본다.
func TestManagedNode_MigrationAddsColumnsIdempotent(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "legacy.db")

	// 1) 신규 컬럼이 없는 구버전 스키마를 손수 생성하고 행을 삽입한다(v1.3 모사).
	rawDB, err := sql.Open("sqlite", sqliteDSN(dbPath))
	require.NoError(t, err)
	_, err = rawDB.ExecContext(ctx, `CREATE TABLE managed_nodes (
		instance_id TEXT PRIMARY KEY,
		hostname    TEXT NOT NULL DEFAULT '',
		version     TEXT NOT NULL DEFAULT '',
		status      TEXT NOT NULL DEFAULT 'pending',
		token_id    TEXT NOT NULL DEFAULT '',
		last_seen   INTEGER NOT NULL DEFAULT 0,
		online      INTEGER NOT NULL DEFAULT 0,
		created_at  INTEGER NOT NULL DEFAULT 0,
		updated_at  INTEGER NOT NULL DEFAULT 0
	)`)
	require.NoError(t, err)
	_, err = rawDB.ExecContext(ctx,
		`INSERT INTO managed_nodes (instance_id, hostname, status, created_at, updated_at)
		 VALUES ('legacy-1', 'old-host', 'approved', 100, 100)`)
	require.NoError(t, err)
	require.NoError(t, rawDB.Close())

	// 2) repo 를 열면 마이그레이션이 신규 컬럼을 멱등하게 추가한다.
	repo, err := NewManagedNodeSQLiteRepository(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })

	// 기존 행이 신규 필드 기본값(빈값/0)으로 읽혀야 한다(회귀 0 — REQ-K09).
	got, err := repo.Get(ctx, "legacy-1")
	require.NoError(t, err)
	assert.Equal(t, "old-host", got.Hostname)
	assert.Equal(t, "approved", got.Status)
	assert.Empty(t, got.GroupName, "구버전 행의 group_name 은 빈값(전체)이어야 함")
	assert.Empty(t, got.OS)
	assert.Empty(t, got.Arch)
	assert.Equal(t, int64(0), got.StartedAt)

	// 3) 두 번째 open(이미 컬럼 존재)도 에러 없이 멱등해야 한다.
	repo2, err := NewManagedNodeSQLiteRepository(ctx, dbPath)
	require.NoError(t, err)
	require.NoError(t, repo2.Close())
}

// TestManagedNode_SetNodeGroupAndClear 는 그룹 배정·변경·해제("전체" 환원)를 검증한다
// (REQ-K02/K05).
func TestManagedNode_SetNodeGroupAndClear(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "n", Status: "approved"}))

	// 배정.
	require.NoError(t, repo.SetNodeGroup(ctx, "n", "prod"))
	got, err := repo.Get(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, "prod", got.GroupName)

	// 변경(재라벨링).
	require.NoError(t, repo.SetNodeGroup(ctx, "n", "staging"))
	got, _ = repo.Get(ctx, "n")
	assert.Equal(t, "staging", got.GroupName)

	// 해제(빈 문자열 → "전체" 환원).
	require.NoError(t, repo.SetNodeGroup(ctx, "n", ""))
	got, _ = repo.Get(ctx, "n")
	assert.Empty(t, got.GroupName, "그룹 해제 시 빈값으로 환원되어야 함")
}

// TestManagedNode_SetNodeGroupNotFound 는 미존재 노드 그룹 배정이 에러를 반환하는지
// 검증한다.
func TestManagedNode_SetNodeGroupNotFound(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	err := repo.SetNodeGroup(context.Background(), "missing", "g")
	assert.ErrorIs(t, err, ErrManagedNodeNotFound)
}

// TestManagedNode_ListGroups 는 distinct 그룹 + 카운트(항상 "전체" 포함, 빈 그룹
// 소멸)를 검증한다(REQ-K03/K05).
func TestManagedNode_ListGroups(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()

	// prod x2, staging x1, 미지정(전체) x2.
	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "a", Status: "approved"}))
	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "b", Status: "approved"}))
	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "c", Status: "approved"}))
	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "d", Status: "approved"}))
	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "e", Status: "approved"}))
	require.NoError(t, repo.SetNodeGroup(ctx, "a", "prod"))
	require.NoError(t, repo.SetNodeGroup(ctx, "b", "prod"))
	require.NoError(t, repo.SetNodeGroup(ctx, "c", "staging"))
	// d, e 는 미지정(전체).

	groups, err := repo.ListGroups(ctx)
	require.NoError(t, err)

	counts := map[string]int{}
	for _, g := range groups {
		counts[g.GroupName] = g.NodeCount
	}
	assert.Equal(t, 2, counts[""], "'전체'(빈 라벨) 버킷은 미지정 2개여야 함")
	assert.Equal(t, 2, counts["prod"])
	assert.Equal(t, 1, counts["staging"])

	// "전체"(빈 라벨)가 항상 맨 앞에 포함되어야 한다(REQ-K03).
	require.NotEmpty(t, groups)
	assert.Equal(t, "", groups[0].GroupName, "첫 항목은 '전체' 버킷이어야 함")

	// 빈 그룹 자동 소멸(REQ-K05): staging 노드를 다른 그룹으로 옮기면 staging 이 사라짐.
	require.NoError(t, repo.SetNodeGroup(ctx, "c", "prod"))
	groups, err = repo.ListGroups(ctx)
	require.NoError(t, err)
	for _, g := range groups {
		assert.NotEqual(t, "staging", g.GroupName, "구성원 0 그룹은 distinct 목록에서 사라져야 함")
	}
}

// TestManagedNode_RegisterUpsertPreservesGroup 는 register 경로의 Upsert 가 관리자
// 배정 group_name 을 clobber 하지 않는지 검증한다(REQ-K02 핵심 — group_name 은 관리자 소유).
func TestManagedNode_RegisterUpsertPreservesGroup(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()

	// 노드 최초 등록 + 관리자 그룹 배정.
	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "n", Hostname: "h1", Status: "approved", Online: true}))
	require.NoError(t, repo.SetNodeGroup(ctx, "n", "prod"))

	// 노드 재접속 register → Upsert(상태/메타 갱신, group_name 은 struct 에서 빈값).
	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "n", Hostname: "h2", Status: "approved", Online: true}))

	got, err := repo.Get(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, "h2", got.Hostname, "register 는 메타를 갱신해야 함")
	assert.Equal(t, "prod", got.GroupName, "register upsert 는 관리자 배정 group_name 을 보존해야 함")
}

// TestManagedNode_SystemInfoStoredAndPreserved 는 SetSystemInfo 가 제공 시 갱신, 미제공
// 시 기존값 보존하는지 검증한다(REQ-K08/K09 하위 호환).
func TestManagedNode_SystemInfoStoredAndPreserved(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "n", Status: "approved"}))

	// 최초 보고(register): os/arch/started_at 저장.
	require.NoError(t, repo.SetSystemInfo(ctx, "n", "linux", "arm64", 5000))
	got, err := repo.Get(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, "linux", got.OS)
	assert.Equal(t, "arm64", got.Arch)
	assert.Equal(t, int64(5000), got.StartedAt)

	// heartbeat 가 일부 필드 생략(빈값/0) → 기존값 보존(REQ-K09).
	require.NoError(t, repo.SetSystemInfo(ctx, "n", "", "", 0))
	got, _ = repo.Get(ctx, "n")
	assert.Equal(t, "linux", got.OS, "미제공 os 는 기존값 보존")
	assert.Equal(t, "arm64", got.Arch, "미제공 arch 는 기존값 보존")
	assert.Equal(t, int64(5000), got.StartedAt, "미제공 started_at 은 기존값 보존")

	// 재기동 register → started_at 갱신(제공 시 덮어씀).
	require.NoError(t, repo.SetSystemInfo(ctx, "n", "linux", "arm64", 9000))
	got, _ = repo.Get(ctx, "n")
	assert.Equal(t, int64(9000), got.StartedAt, "제공된 started_at 은 갱신되어야 함")
}

// TestManagedNode_SetSystemInfoNotFound 는 미존재 노드 시스템 정보 설정이 에러를
// 반환하는지 검증한다.
func TestManagedNode_SetSystemInfoNotFound(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	err := repo.SetSystemInfo(context.Background(), "missing", "linux", "amd64", 1)
	assert.ErrorIs(t, err, ErrManagedNodeNotFound)
}

// TestManagedNode_SystemInfoDoesNotClobberGroup 는 SetSystemInfo 가 group_name 을
// 건드리지 않는지 검증한다(관심사 분리 — 관리자 그룹 vs 시스템 정보).
func TestManagedNode_SystemInfoDoesNotClobberGroup(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "n", Status: "approved"}))
	require.NoError(t, repo.SetNodeGroup(ctx, "n", "prod"))

	require.NoError(t, repo.SetSystemInfo(ctx, "n", "darwin", "arm64", 1234))
	got, err := repo.Get(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, "prod", got.GroupName, "시스템 정보 갱신은 group_name 을 건드리지 않아야 함")
	assert.Equal(t, "darwin", got.OS)
}

// TestManagedNode_GroupMethodsAfterClose 는 Close 후 신규 메서드가 에러를 반환하는지
// 검증한다(에러 경로).
func TestManagedNode_GroupMethodsAfterClose(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "closed-group.db")
	repo, err := NewManagedNodeSQLiteRepository(context.Background(), dbPath)
	require.NoError(t, err)
	require.NoError(t, repo.Close())

	ctx := context.Background()
	assert.Error(t, repo.SetNodeGroup(ctx, "x", "g"))
	assert.Error(t, repo.SetSystemInfo(ctx, "x", "linux", "amd64", 1))
	_, listErr := repo.ListGroups(ctx)
	assert.Error(t, listErr)
}

// TestManagedNode_ListCarriesNewFields 는 List 가 신규 필드를 운반하는지 검증한다.
func TestManagedNode_ListCarriesNewFields(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, ManagedNode{
		InstanceID: "n", Status: "approved",
		GroupName: "prod", OS: "linux", Arch: "amd64", StartedAt: 7000,
	}))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "prod", list[0].GroupName)
	assert.Equal(t, "linux", list[0].OS)
	assert.Equal(t, "amd64", list[0].Arch)
	assert.Equal(t, int64(7000), list[0].StartedAt)
}
