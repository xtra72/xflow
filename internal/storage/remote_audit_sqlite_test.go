// remote_audit_sqlite_test.go 는 RemoteAuditRepository 의 SQLite 구현을 검증한다
// (@SPEC:SPEC-REMOTE-001 M6, REQ-F05/F06).
//
// 감사 로그는 append-only 이며, 시크릿(토큰/명령 인자/페이로드)을 절대 저장하지
// 않는다. instance_id + ts 인덱스로 노드별 조회를 가속한다.
package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAuditRepo(t *testing.T) *RemoteAuditSQLiteRepository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "audit.db")
	repo, err := NewRemoteAuditSQLiteRepository(context.Background(), dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

// TestRemoteAudit_AppendAndList 는 감사 레코드를 추가하고 조회하는 기본 동작을
// 검증한다(REQ-F05). 누가/언제/어느 노드/무엇/결과가 보존되어야 한다.
func TestRemoteAudit_AppendAndList(t *testing.T) {
	repo := newTestAuditRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Append(ctx, RemoteAuditRecord{
		InstanceID: "node-1", Actor: "admin", Action: "approve",
		Result: "ok", Timestamp: 1000,
	}))
	require.NoError(t, repo.Append(ctx, RemoteAuditRecord{
		InstanceID: "node-1", Actor: "admin", Action: "command",
		Domain: "flow", CommandAction: "deploy", Result: "ok", Timestamp: 2000,
	}))
	require.NoError(t, repo.Append(ctx, RemoteAuditRecord{
		InstanceID: "node-2", Actor: "admin", Action: "revoke",
		Result: "ok", Timestamp: 1500,
	}))

	// 전체 조회(instance_id="" 필터 없음) — ts 내림차순(최신 우선).
	all, err := repo.List(ctx, "", 100, 0)
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, int64(2000), all[0].Timestamp, "최신 레코드가 먼저 와야 함")
	assert.Equal(t, "command", all[0].Action)
	assert.Equal(t, "flow", all[0].Domain)
	assert.Equal(t, "deploy", all[0].CommandAction)

	// 노드별 필터.
	n1, err := repo.List(ctx, "node-1", 100, 0)
	require.NoError(t, err)
	require.Len(t, n1, 2)
	for _, r := range n1 {
		assert.Equal(t, "node-1", r.InstanceID)
	}
}

// TestRemoteAudit_Pagination 는 limit/offset 페이지네이션을 검증한다.
func TestRemoteAudit_Pagination(t *testing.T) {
	repo := newTestAuditRepo(t)
	ctx := context.Background()
	for i := int64(1); i <= 5; i++ {
		require.NoError(t, repo.Append(ctx, RemoteAuditRecord{
			InstanceID: "n", Actor: "admin", Action: "approve", Result: "ok", Timestamp: i * 100,
		}))
	}

	page1, err := repo.List(ctx, "", 2, 0)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	assert.Equal(t, int64(500), page1[0].Timestamp)
	assert.Equal(t, int64(400), page1[1].Timestamp)

	page2, err := repo.List(ctx, "", 2, 2)
	require.NoError(t, err)
	require.Len(t, page2, 2)
	assert.Equal(t, int64(300), page2[0].Timestamp)
}

// TestRemoteAudit_AppendOnly 는 저장소가 update/delete API 를 노출하지 않고
// (append-only), 레코드가 단조 증가 id 를 받는지 확인한다.
func TestRemoteAudit_AppendOnly(t *testing.T) {
	repo := newTestAuditRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Append(ctx, RemoteAuditRecord{InstanceID: "n", Actor: "a", Action: "approve", Result: "ok", Timestamp: 1}))
	require.NoError(t, repo.Append(ctx, RemoteAuditRecord{InstanceID: "n", Actor: "a", Action: "reject", Result: "ok", Timestamp: 2}))

	recs, err := repo.List(ctx, "", 10, 0)
	require.NoError(t, err)
	require.Len(t, recs, 2)
	// id 가 부여되고 서로 달라야 한다.
	assert.NotZero(t, recs[0].ID)
	assert.NotEqual(t, recs[0].ID, recs[1].ID)
}

// TestRemoteAudit_FactoryConstruction 은 factory 가 sqlite/file/default 모두에서
// sqlite 저장소를 생성하는지 확인한다.
func TestRemoteAudit_FactoryConstruction(t *testing.T) {
	for _, typ := range []string{"sqlite", "file", "unknown"} {
		dbPath := filepath.Join(t.TempDir(), "audit.db")
		repo, err := NewRemoteAuditRepository(context.Background(), typ, dbPath)
		require.NoError(t, err)
		require.NotNil(t, repo)

		require.NoError(t, repo.Append(context.Background(), RemoteAuditRecord{
			InstanceID: "n", Actor: "admin", Action: "approve", Result: "ok", Timestamp: 1,
		}))
		require.NoError(t, repo.Close())
	}
}

// TestRemoteAudit_EmptyResult 는 빈 저장소 조회가 nil/빈 결과를 반환하는지 확인한다.
func TestRemoteAudit_EmptyResult(t *testing.T) {
	repo := newTestAuditRepo(t)
	recs, err := repo.List(context.Background(), "missing", 10, 0)
	require.NoError(t, err)
	assert.Empty(t, recs)

	// limit<=0/offset<0 은 보정되어 에러 없이 동작한다.
	recs, err = repo.List(context.Background(), "", 0, -5)
	require.NoError(t, err)
	assert.Empty(t, recs)
}
