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
	all, _, err := repo.List(ctx, RemoteAuditQuery{Limit: 100})
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, int64(2000), all[0].Timestamp, "최신 레코드가 먼저 와야 함")
	assert.Equal(t, "command", all[0].Action)
	assert.Equal(t, "flow", all[0].Domain)
	assert.Equal(t, "deploy", all[0].CommandAction)

	// 노드별 필터.
	n1, _, err := repo.List(ctx, RemoteAuditQuery{InstanceID: "node-1", Limit: 100})
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

	page1, _, err := repo.List(ctx, RemoteAuditQuery{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page1, 2)
	assert.Equal(t, int64(500), page1[0].Timestamp)
	assert.Equal(t, int64(400), page1[1].Timestamp)

	page2, _, err := repo.List(ctx, RemoteAuditQuery{Limit: 2, Offset: 2})
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

	recs, _, err := repo.List(ctx, RemoteAuditQuery{Limit: 10})
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
	recs, _, err := repo.List(context.Background(), RemoteAuditQuery{InstanceID: "missing", Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, recs)

	// limit<=0/offset<0 은 보정되어 에러 없이 동작한다.
	recs, _, err = repo.List(context.Background(), RemoteAuditQuery{Limit: 0, Offset: -5})
	require.NoError(t, err)
	assert.Empty(t, recs)
}

// --- 정렬·필터·전체 건수 (@SPEC:SPEC-REMOTE-LOG-001) ---
//
// 화면이 받아 온 쪽 안에서만 정렬하면 그 결과가 전체를 대표하지 않는다. 그래서
// 정렬·필터를 SQL 로 내렸고, 아래 시험이 그 계약을 고정한다.

/** 조회용 레코드를 만든다. ts 는 인자 그대로 쓴다(정렬 검증에 필요). */
func auditRec(instance, actor, action string, ts int64) RemoteAuditRecord {
	return RemoteAuditRecord{
		InstanceID: instance, Actor: actor, Action: action,
		Result: AuditResultOK, Timestamp: ts,
	}
}

func TestRemoteAudit_FilterByActionAndActor(t *testing.T) {
	ctx := context.Background()
	repo := newTestAuditRepo(t)

	require.NoError(t, repo.Append(ctx, auditRec("n1", "admin", AuditActionConnect, 100)))
	require.NoError(t, repo.Append(ctx, auditRec("n1", "bob", AuditActionAccess, 200)))
	require.NoError(t, repo.Append(ctx, auditRec("n2", "admin", AuditActionAccess, 300)))

	byAction, total, err := repo.List(ctx, RemoteAuditQuery{Action: AuditActionAccess, Limit: 10})
	require.NoError(t, err)
	require.Len(t, byAction, 2)
	require.Equal(t, int64(2), total, "전체 건수는 필터를 적용한 값이어야 한다")

	byActor, total, err := repo.List(ctx, RemoteAuditQuery{Actor: "admin", Limit: 10})
	require.NoError(t, err)
	require.Len(t, byActor, 2)
	require.Equal(t, int64(2), total)

	both, total, err := repo.List(ctx,
		RemoteAuditQuery{Actor: "admin", Action: AuditActionAccess, Limit: 10})
	require.NoError(t, err)
	require.Len(t, both, 1)
	require.Equal(t, int64(1), total)
	require.Equal(t, "n2", both[0].InstanceID)
}

func TestRemoteAudit_SortFieldAndDirection(t *testing.T) {
	ctx := context.Background()
	repo := newTestAuditRepo(t)

	require.NoError(t, repo.Append(ctx, auditRec("n-c", "carol", AuditActionConnect, 100)))
	require.NoError(t, repo.Append(ctx, auditRec("n-a", "alice", AuditActionAccess, 200)))
	require.NoError(t, repo.Append(ctx, auditRec("n-b", "bob", AuditActionDelete, 300)))

	// 기본: 최신순.
	latest, _, err := repo.List(ctx, RemoteAuditQuery{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, int64(300), latest[0].Timestamp)

	// 시각 오름차순.
	oldest, _, err := repo.List(ctx, RemoteAuditQuery{Limit: 10, SortAsc: true})
	require.NoError(t, err)
	require.Equal(t, int64(100), oldest[0].Timestamp)

	// 수행자 오름차순.
	byActor, _, err := repo.List(ctx,
		RemoteAuditQuery{Limit: 10, SortField: AuditSortActor, SortAsc: true})
	require.NoError(t, err)
	require.Equal(t, "alice", byActor[0].Actor)

	// 노드 내림차순.
	byNode, _, err := repo.List(ctx, RemoteAuditQuery{Limit: 10, SortField: AuditSortInstance})
	require.NoError(t, err)
	require.Equal(t, "n-c", byNode[0].InstanceID)
}

func TestRemoteAudit_UnknownSortFieldFallsBackToTime(t *testing.T) {
	ctx := context.Background()
	repo := newTestAuditRepo(t)
	require.NoError(t, repo.Append(ctx, auditRec("n1", "a", AuditActionConnect, 100)))
	require.NoError(t, repo.Append(ctx, auditRec("n2", "b", AuditActionConnect, 200)))

	// 화이트리스트 밖(주입 시도 포함)은 기본 정렬로 떨어진다 — 목록이 비거나 터지지 않는다.
	recs, total, err := repo.List(ctx,
		RemoteAuditQuery{Limit: 10, SortField: "ts; DROP TABLE remote_audit--"})
	require.NoError(t, err)
	require.Len(t, recs, 2)
	require.Equal(t, int64(2), total)
	require.Equal(t, int64(200), recs[0].Timestamp, "기본은 최신순이다")
}

func TestRemoteAudit_TotalIgnoresPaging(t *testing.T) {
	ctx := context.Background()
	repo := newTestAuditRepo(t)
	for i := 0; i < 5; i++ {
		require.NoError(t, repo.Append(ctx, auditRec("n1", "a", AuditActionConnect, int64(i+1))))
	}

	page, total, err := repo.List(ctx, RemoteAuditQuery{Limit: 2, Offset: 2})
	require.NoError(t, err)
	require.Len(t, page, 2, "쪽은 요청한 크기만큼")
	require.Equal(t, int64(5), total, "전체 건수는 쪽 크기와 무관해야 한다 — 쪽 수 계산의 근거다")
}
