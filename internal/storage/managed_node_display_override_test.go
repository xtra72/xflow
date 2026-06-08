// managed_node_display_override_test.go 는 v1.6(M11 확장)의 노드 해상도 서버-측
// 오버라이드(display_override_width/display_override_height) 저장·해제·마이그레이션·
// register upsert 보존을 검증한다(@SPEC:SPEC-REMOTE-001 M11, OQ-M1 보조 override).
//
// 오버라이드는 관리자가 노드 config/재시작 없이 서버에서 노드 해상도를 강제하는 값이며,
// group_name 과 동일하게 관리자 소유(admin-owned)이므로 register/heartbeat upsert 가
// 절대 clobber 하지 않아야 한다(M9 group_name 보존 패턴 일관).
package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestManagedNode_DisplayOverrideSetAndClear 는 SetNodeDisplayOverride 가 양수
// width/height 를 저장하고, width<=0 또는 height<=0 이 오버라이드를 0,0 으로
// 해제(clear)하는지 검증한다.
func TestManagedNode_DisplayOverrideSetAndClear(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "n", Status: "approved"}))

	// 최초엔 오버라이드 없음(0,0).
	got, err := repo.Get(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, 0, got.DisplayOverrideWidth)
	assert.Equal(t, 0, got.DisplayOverrideHeight)

	// 관리자가 오버라이드 설정.
	require.NoError(t, repo.SetNodeDisplayOverride(ctx, "n", 1920, 1080))
	got, err = repo.Get(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, 1920, got.DisplayOverrideWidth)
	assert.Equal(t, 1080, got.DisplayOverrideHeight)

	// width<=0 → 해제(0,0).
	require.NoError(t, repo.SetNodeDisplayOverride(ctx, "n", 0, 1080))
	got, _ = repo.Get(ctx, "n")
	assert.Equal(t, 0, got.DisplayOverrideWidth, "width<=0 은 오버라이드 해제")
	assert.Equal(t, 0, got.DisplayOverrideHeight)

	// 다시 설정 후 height<=0 → 해제.
	require.NoError(t, repo.SetNodeDisplayOverride(ctx, "n", 2560, 1440))
	require.NoError(t, repo.SetNodeDisplayOverride(ctx, "n", 2560, -1))
	got, _ = repo.Get(ctx, "n")
	assert.Equal(t, 0, got.DisplayOverrideWidth, "height<=0 은 오버라이드 해제")
	assert.Equal(t, 0, got.DisplayOverrideHeight)
}

// TestManagedNode_DisplayOverrideUnknownNode 는 미존재 노드에 SetNodeDisplayOverride
// 시 ErrManagedNodeNotFound 를 반환하는지 검증한다.
func TestManagedNode_DisplayOverrideUnknownNode(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()
	err := repo.SetNodeDisplayOverride(ctx, "missing", 1920, 1080)
	assert.ErrorIs(t, err, ErrManagedNodeNotFound)
}

// TestManagedNode_DisplayOverridePreservedOnRegisterUpsert 는 register 재upsert 가
// 관리자가 설정한 오버라이드를 clobber 하지 않는지 검증한다(admin-owned 보존 —
// M9 group_name 패턴 일관). SetSystemInfo(노드 보고)도 오버라이드를 건드리지 않는다.
func TestManagedNode_DisplayOverridePreservedOnRegisterUpsert(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "n", Status: "approved"}))
	require.NoError(t, repo.SetNodeDisplayOverride(ctx, "n", 1920, 1080))

	// register upsert(오버라이드 미운반) — 보존되어야 한다.
	require.NoError(t, repo.Upsert(ctx, ManagedNode{
		InstanceID: "n", Status: "approved", Hostname: "host2",
		DisplayWidth: 1366, DisplayHeight: 768,
	}))
	got, err := repo.Get(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, "host2", got.Hostname, "메타는 갱신되어야 함")
	assert.Equal(t, 1920, got.DisplayOverrideWidth, "register upsert 는 오버라이드를 보존해야 함")
	assert.Equal(t, 1080, got.DisplayOverrideHeight, "register upsert 는 오버라이드를 보존해야 함")

	// 노드 보고(SetSystemInfo)도 오버라이드를 건드리지 않는다.
	require.NoError(t, repo.SetSystemInfo(ctx, "n", "linux", "arm64", 1000, 800, 600))
	got, _ = repo.Get(ctx, "n")
	assert.Equal(t, 1920, got.DisplayOverrideWidth, "SetSystemInfo 는 오버라이드를 건드리지 않아야 함")
	assert.Equal(t, 1080, got.DisplayOverrideHeight)
	assert.Equal(t, 800, got.DisplayWidth, "노드 보고 해상도는 별도로 갱신")
	assert.Equal(t, 600, got.DisplayHeight)
}

// TestManagedNode_DisplayOverrideListCarriesFields 는 List 가 오버라이드 필드를
// 운반하는지 검증한다.
func TestManagedNode_DisplayOverrideListCarriesFields(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "n", Status: "approved"}))
	require.NoError(t, repo.SetNodeDisplayOverride(ctx, "n", 1024, 768))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, 1024, list[0].DisplayOverrideWidth)
	assert.Equal(t, 768, list[0].DisplayOverrideHeight)
}

// TestManagedNode_DisplayOverrideInsertedOnUpsert 는 첫 INSERT 가 struct 의 오버라이드
// 값을 기록하는지 검증한다(완전성 — INSERT 컬럼 집합 포함).
func TestManagedNode_DisplayOverrideInsertedOnUpsert(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, ManagedNode{
		InstanceID: "n", Status: "pending",
		DisplayOverrideWidth: 3840, DisplayOverrideHeight: 2160,
	}))

	got, err := repo.Get(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, 3840, got.DisplayOverrideWidth)
	assert.Equal(t, 2160, got.DisplayOverrideHeight)
}

// TestManagedNode_DisplayOverrideMigrationOnExistingDB 는 오버라이드 컬럼이 없는
// 기존 DB(v1.6 M11.1 이하 — display_width/height 만 보유)에서 마이그레이션이 멱등하게
// 컬럼을 추가하는지 검증한다(하위 호환 마이그레이션).
func TestManagedNode_DisplayOverrideMigrationOnExistingDB(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "existing-display-override.db")

	// M11.1 스키마(display_width/height 보유, override 컬럼 없음) 를 직접 만든다.
	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE TABLE managed_nodes (
		instance_id    TEXT PRIMARY KEY,
		hostname       TEXT NOT NULL DEFAULT '',
		version        TEXT NOT NULL DEFAULT '',
		status         TEXT NOT NULL DEFAULT 'pending',
		token_id       TEXT NOT NULL DEFAULT '',
		last_seen      INTEGER NOT NULL DEFAULT 0,
		online         INTEGER NOT NULL DEFAULT 0,
		group_name     TEXT NOT NULL DEFAULT '',
		os             TEXT NOT NULL DEFAULT '',
		arch           TEXT NOT NULL DEFAULT '',
		started_at     INTEGER NOT NULL DEFAULT 0,
		display_width  INTEGER NOT NULL DEFAULT 0,
		display_height INTEGER NOT NULL DEFAULT 0,
		created_at     INTEGER NOT NULL DEFAULT 0,
		updated_at     INTEGER NOT NULL DEFAULT 0
	)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO managed_nodes (instance_id, created_at, updated_at) VALUES ('old', 1, 1)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// 신규 코드로 동일 DB 를 연다 → override 컬럼 멱등 추가.
	repo, err := NewManagedNodeSQLiteRepository(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })

	got, err := repo.Get(ctx, "old")
	require.NoError(t, err)
	assert.Equal(t, 0, got.DisplayOverrideWidth, "마이그레이션된 기존 행은 override 0")
	assert.Equal(t, 0, got.DisplayOverrideHeight)

	// 신규 컬럼에 쓰기 가능해야 한다.
	require.NoError(t, repo.SetNodeDisplayOverride(ctx, "old", 1920, 1080))
	got, _ = repo.Get(ctx, "old")
	assert.Equal(t, 1920, got.DisplayOverrideWidth)
	assert.Equal(t, 1080, got.DisplayOverrideHeight)

	// 재오픈 시에도 멱등(중복 ADD COLUMN 없음).
	repo2, err := NewManagedNodeSQLiteRepository(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo2.Close() })
	got, err = repo2.Get(ctx, "old")
	require.NoError(t, err)
	assert.Equal(t, 1920, got.DisplayOverrideWidth, "재오픈 후에도 오버라이드 유지")
}
