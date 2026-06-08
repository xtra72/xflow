// managed_node_display_test.go 는 v1.6(M11, 그룹 M)의 노드 해상도(display_width/
// display_height) 저장·마이그레이션·preserve-on-omit 를 검증한다
// (@SPEC:SPEC-REMOTE-001 M11, REQ-M01~M03). M9 시스템 정보 패턴을 그대로 따른다.
package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestManagedNode_DisplayStoredAndPreserved 는 SetSystemInfo 가 display_width/
// display_height 를 제공 시 갱신, 미제공(0) 시 기존값 보존하는지 검증한다
// (REQ-M01/M03 — 하위 호환 preserve-on-omit).
func TestManagedNode_DisplayStoredAndPreserved(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "n", Status: "approved"}))

	// 최초 보고(register): 해상도 저장.
	require.NoError(t, repo.SetSystemInfo(ctx, "n", "linux", "arm64", 5000, 1920, 1080))
	got, err := repo.Get(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, 1920, got.DisplayWidth)
	assert.Equal(t, 1080, got.DisplayHeight)

	// heartbeat 가 해상도 생략(0) → 기존값 보존(REQ-M03).
	require.NoError(t, repo.SetSystemInfo(ctx, "n", "linux", "arm64", 6000, 0, 0))
	got, _ = repo.Get(ctx, "n")
	assert.Equal(t, 1920, got.DisplayWidth, "미제공 display_width 는 기존값 보존")
	assert.Equal(t, 1080, got.DisplayHeight, "미제공 display_height 는 기존값 보존")

	// 운영자가 해상도 변경 후 재기동 register → 갱신(제공 시 덮어씀).
	require.NoError(t, repo.SetSystemInfo(ctx, "n", "linux", "arm64", 7000, 2560, 1440))
	got, _ = repo.Get(ctx, "n")
	assert.Equal(t, 2560, got.DisplayWidth, "제공된 display_width 는 갱신되어야 함")
	assert.Equal(t, 1440, got.DisplayHeight, "제공된 display_height 는 갱신되어야 함")
}

// TestManagedNode_DisplayDefaultZero 는 해상도 미보고(구버전 노드) 시 0(미보고)으로
// 처리되는지 검증한다(REQ-M03 하위 호환).
func TestManagedNode_DisplayDefaultZero(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "n", Status: "pending"}))

	got, err := repo.Get(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, 0, got.DisplayWidth, "미보고 노드는 display_width 0")
	assert.Equal(t, 0, got.DisplayHeight, "미보고 노드는 display_height 0")
}

// TestManagedNode_DisplayInsertedOnUpsert 는 첫 INSERT 가 struct 의 해상도 값을
// 기록하는지 검증한다(register 가 해상도를 운반하는 경우).
func TestManagedNode_DisplayInsertedOnUpsert(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, ManagedNode{
		InstanceID: "n", Status: "pending",
		DisplayWidth: 1366, DisplayHeight: 768,
	}))

	got, err := repo.Get(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, 1366, got.DisplayWidth)
	assert.Equal(t, 768, got.DisplayHeight)
}

// TestManagedNode_DisplayPreservedOnRegisterUpsert 는 register 재upsert(해상도 0)가
// 기존 저장된 해상도를 clobber 하지 않는지 검증한다(REQ-M03 — preserve admin/system
// fields on register upsert, M9 os/arch 패턴 일관).
func TestManagedNode_DisplayPreservedOnRegisterUpsert(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "n", Status: "approved"}))
	require.NoError(t, repo.SetSystemInfo(ctx, "n", "linux", "arm64", 1000, 1920, 1080))

	// register upsert 는 해상도를 운반하지 않을 수 있다(0) — 기존값 보존되어야 한다.
	require.NoError(t, repo.Upsert(ctx, ManagedNode{
		InstanceID: "n", Status: "approved", Hostname: "host2",
	}))
	got, err := repo.Get(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, "host2", got.Hostname, "메타는 갱신되어야 함")
	assert.Equal(t, 1920, got.DisplayWidth, "register upsert 는 저장된 display_width 를 보존해야 함")
	assert.Equal(t, 1080, got.DisplayHeight, "register upsert 는 저장된 display_height 를 보존해야 함")
}

// TestManagedNode_DisplayListCarriesFields 는 List 가 해상도 필드를 운반하는지 검증한다.
func TestManagedNode_DisplayListCarriesFields(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, ManagedNode{
		InstanceID: "n", Status: "approved",
		DisplayWidth: 800, DisplayHeight: 600,
	}))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, 800, list[0].DisplayWidth)
	assert.Equal(t, 600, list[0].DisplayHeight)
}

// TestManagedNode_DisplayMigrationOnExistingDB 는 display 컬럼이 없는 기존 DB 에서
// 마이그레이션이 멱등하게 컬럼을 추가하는지 검증한다(REQ-M03 하위 호환 마이그레이션).
func TestManagedNode_DisplayMigrationOnExistingDB(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "existing-display.db")

	// v1.5 이하 스키마(display 컬럼 없음) 를 직접 만든다.
	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE TABLE managed_nodes (
		instance_id TEXT PRIMARY KEY,
		hostname    TEXT NOT NULL DEFAULT '',
		version     TEXT NOT NULL DEFAULT '',
		status      TEXT NOT NULL DEFAULT 'pending',
		token_id    TEXT NOT NULL DEFAULT '',
		last_seen   INTEGER NOT NULL DEFAULT 0,
		online      INTEGER NOT NULL DEFAULT 0,
		group_name  TEXT NOT NULL DEFAULT '',
		os          TEXT NOT NULL DEFAULT '',
		arch        TEXT NOT NULL DEFAULT '',
		started_at  INTEGER NOT NULL DEFAULT 0,
		created_at  INTEGER NOT NULL DEFAULT 0,
		updated_at  INTEGER NOT NULL DEFAULT 0
	)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO managed_nodes (instance_id, created_at, updated_at) VALUES ('old', 1, 1)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// 신규 코드로 동일 DB 를 연다 → display 컬럼 멱등 추가.
	repo, err := NewManagedNodeSQLiteRepository(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })

	got, err := repo.Get(ctx, "old")
	require.NoError(t, err)
	assert.Equal(t, 0, got.DisplayWidth, "마이그레이션된 기존 행은 display_width 0")
	assert.Equal(t, 0, got.DisplayHeight, "마이그레이션된 기존 행은 display_height 0")

	// 신규 컬럼에 쓰기 가능해야 한다.
	require.NoError(t, repo.SetSystemInfo(ctx, "old", "linux", "amd64", 1, 1920, 1080))
	got, _ = repo.Get(ctx, "old")
	assert.Equal(t, 1920, got.DisplayWidth)
	assert.Equal(t, 1080, got.DisplayHeight)

	// 재오픈 시에도 멱등(중복 ADD COLUMN 없음).
	repo2, err := NewManagedNodeSQLiteRepository(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo2.Close() })
}
