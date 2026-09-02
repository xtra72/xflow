package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @SPEC:SPEC-DASHBOARD-004 (M2)
// 본 파일은 신규 대시보드 엔티티 스키마와 저장소 3종
// (dashboards / dashboard_acl / dashboard_user_state) 을 검증한다.

// openEntityDB 는 신규 스키마 전용 raw *sql.DB 를 연다.
//
// OpenSQLiteDB 를 쓰지 않는 이유: 그 경로는 레거시 migrateDashboardSchema 를 먼저
// 호출해 구 스키마의 dashboards 를 만들어 버린다. 신규 스키마는 M3 이관 이후에만
// 공존할 수 있으므로 테스트는 빈 DB 에서 시작한다.
func openEntityDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "dashboard-entity-test.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err, "sql.Open 실패")
	t.Cleanup(func() { db.Close() })

	_, err = db.ExecContext(context.Background(), "PRAGMA journal_mode=WAL")
	require.NoError(t, err, "WAL 모드 설정 실패")
	return db
}

// setupEntityRepo 는 신규 대시보드 저장소 3종을 함께 만든다.
func setupEntityRepo(t *testing.T) (*sql.DB, *DashboardEntitySQLiteRepository, *DashboardACLSQLiteRepository, *DashboardUserStateSQLiteRepository) {
	t.Helper()
	ctx := context.Background()
	db := openEntityDB(t)

	repo, err := NewDashboardEntitySQLiteRepository(ctx, db)
	require.NoError(t, err, "NewDashboardEntitySQLiteRepository 실패")
	aclRepo, err := NewDashboardACLSQLiteRepository(ctx, db)
	require.NoError(t, err, "NewDashboardACLSQLiteRepository 실패")
	stateRepo, err := NewDashboardUserStateSQLiteRepository(ctx, db)
	require.NoError(t, err, "NewDashboardUserStateSQLiteRepository 실패")
	return db, repo, aclRepo, stateRepo
}

// newDashboard 는 테스트용 Dashboard 값을 만든다.
func newDashboard(uid, owner, visibility string) Dashboard {
	return Dashboard{
		UID:        uid,
		Name:       "대시보드 " + uid,
		Owner:      owner,
		Visibility: visibility,
		Payload:    []byte(`{"panels":[],"layout":[]}`),
	}
}

// ---------------------------------------------------------------------------
// 스키마: 멱등성 / 구 스키마 가드 / schema_markers
// ---------------------------------------------------------------------------

// TestMigrateDashboardSchemaV2_Idempotent 는 3회 연속 호출해도 오류가 없고
// 테이블이 중복 생성되지 않음을 검증한다.
func TestMigrateDashboardSchemaV2_Idempotent(t *testing.T) {
	ctx := context.Background()
	db := openEntityDB(t)

	for i := 1; i <= 3; i++ {
		require.NoErrorf(t, migrateDashboardSchemaV2(ctx, db), "%d회차 마이그레이션 실패", i)
	}

	for _, table := range []string{"dashboards", "dashboard_acl", "dashboard_user_state", "schema_markers"} {
		var n int64
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n))
		assert.EqualValuesf(t, 1, n, "%s 테이블이 정확히 1개여야 한다", table)
	}

	// 인덱스도 중복 생성되지 않는다.
	for _, idx := range []string{"dashboards_owner_idx", "dashboards_visibility_idx", "dashboard_acl_subject_idx"} {
		var n int64
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, idx).Scan(&n))
		assert.EqualValuesf(t, 1, n, "%s 인덱스가 정확히 1개여야 한다", idx)
	}
}

// TestMigrateDashboardSchemaV2_RejectsLegacySchema 는 구 스키마 위에서 호출하면
// 조용히 성공하지 않고 오류가 나는지 검증한다.
//
// CREATE TABLE IF NOT EXISTS 는 구 스키마의 dashboards 에 대해 no-op 이므로,
// 가드가 없으면 신규 컬럼이 없는 채로 성공한 것처럼 보이고 이후 모든 질의가
// 런타임에 깨진다.
func TestMigrateDashboardSchemaV2_RejectsLegacySchema(t *testing.T) {
	ctx := context.Background()
	db := openEntityDB(t)

	require.NoError(t, seedLegacyDashboardSchema(ctx, db), "레거시 스키마 생성 실패")

	err := migrateDashboardSchemaV2(ctx, db)
	require.Error(t, err, "구 스키마 위에서는 오류를 반환해야 한다")
	assert.Contains(t, err.Error(), "legacy")

	// 이관(RENAME) 이후에는 통과한다 — M3 절차와 동일한 순서.
	_, err = db.ExecContext(ctx, `ALTER TABLE dashboards RENAME TO dashboard_snapshots_v1`)
	require.NoError(t, err)
	assert.NoError(t, migrateDashboardSchemaV2(ctx, db), "RENAME 이후에는 성공해야 한다")
}

// TestSchemaMarkers_InsertAndRead 는 마커 행의 삽입·조회와 PK 중복 차단을 검증한다.
func TestSchemaMarkers_InsertAndRead(t *testing.T) {
	ctx := context.Background()
	db := openEntityDB(t)
	require.NoError(t, migrateDashboardSchemaV2(ctx, db))

	// 초기 상태: 마커 없음 = "이관 안 됨".
	var n int64
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_markers WHERE key='dashboard_entity_migrated'`).Scan(&n))
	assert.EqualValues(t, 0, n, "초기 상태에는 마커가 없다")

	now := time.Now().UnixMilli()
	_, err := db.ExecContext(ctx,
		`INSERT INTO schema_markers(key, value, applied_at) VALUES (?, ?, ?)`,
		"dashboard_entity_migrated", "1", now)
	require.NoError(t, err, "마커 삽입 실패")

	var key, value string
	var appliedAt int64
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT key, value, applied_at FROM schema_markers WHERE key = ?`,
		"dashboard_entity_migrated").Scan(&key, &value, &appliedAt))
	assert.Equal(t, "dashboard_entity_migrated", key)
	assert.Equal(t, "1", value)
	assert.Equal(t, now, appliedAt)

	// key 는 PRIMARY KEY 이므로 중복 삽입은 거부된다 (이관 2회 기록 방지).
	_, err = db.ExecContext(ctx,
		`INSERT INTO schema_markers(key, value, applied_at) VALUES (?, ?, ?)`,
		"dashboard_entity_migrated", "2", now)
	assert.Error(t, err, "동일 key 중복 삽입은 거부되어야 한다")
}

// ---------------------------------------------------------------------------
// dashboards CRUD
// ---------------------------------------------------------------------------

// TestDashboardEntity_CreateGetDelete 는 uid 기반 CRUD 기본 흐름을 검증한다.
func TestDashboardEntity_CreateGetDelete(t *testing.T) {
	ctx := context.Background()
	_, repo, _, _ := setupEntityRepo(t)

	// 없는 uid 조회는 NotFound.
	_, err := repo.Get(ctx, "D1")
	assert.ErrorIs(t, err, ErrDashboardNotFound)

	before := time.Now().UnixMilli()
	created, err := repo.Create(ctx, newDashboard("D1", "edi", "private"))
	require.NoError(t, err, "생성 실패")

	assert.NotZero(t, created.ID, "ID 는 서버가 부여한다")
	assert.EqualValues(t, 1, created.Version, "생성 시 version 은 1")
	assert.GreaterOrEqual(t, created.CreatedAt, before)
	assert.Equal(t, created.CreatedAt, created.UpdatedAt, "생성 시 두 시각은 같다")

	got, err := repo.Get(ctx, "D1")
	require.NoError(t, err)
	assert.Equal(t, "D1", got.UID)
	assert.Equal(t, "대시보드 D1", got.Name)
	assert.Equal(t, "edi", got.Owner)
	assert.Equal(t, "private", got.Visibility)
	assert.False(t, got.IsDefault)
	assert.EqualValues(t, 0, got.SortOrder)
	assert.JSONEq(t, `{"panels":[],"layout":[]}`, string(got.Payload))

	require.NoError(t, repo.Delete(ctx, "D1"))
	_, err = repo.Get(ctx, "D1")
	assert.ErrorIs(t, err, ErrDashboardNotFound, "삭제 후에는 NotFound")

	// 삭제는 멱등하다.
	assert.NoError(t, repo.Delete(ctx, "D1"), "없는 uid 삭제는 멱등")
}

// TestDashboardEntity_Create_UIDUnique 는 uid UNIQUE 제약 위반을 검증한다.
func TestDashboardEntity_Create_UIDUnique(t *testing.T) {
	ctx := context.Background()
	db, repo, _, _ := setupEntityRepo(t)

	_, err := repo.Create(ctx, newDashboard("D1", "edi", "private"))
	require.NoError(t, err)

	// 소유자가 달라도 uid 는 전역 유일하다.
	_, err = repo.Create(ctx, newDashboard("D1", "root", "shared"))
	assert.ErrorIs(t, err, ErrDashboardUIDExists, "중복 uid 는 ErrDashboardUIDExists")

	var n int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM dashboards`).Scan(&n))
	assert.EqualValues(t, 1, n, "중복 생성 시도로 행이 늘어나지 않는다")

	// 저장소를 우회한 직접 INSERT 도 UNIQUE 제약이 막는다 (최종 방어선).
	_, err = db.ExecContext(ctx, `
		INSERT INTO dashboards(uid, name, owner, visibility, is_default, sort_order, version, created_at, updated_at, payload)
		VALUES ('D1', 'dup', 'root', 'shared', 0, 0, 1, 0, 0, '{}')`)
	assert.Error(t, err, "UNIQUE 제약이 직접 INSERT 도 거부해야 한다")
}

// TestDashboardEntity_List 는 정렬 순서와 payload 포함 여부를 검증한다.
func TestDashboardEntity_List(t *testing.T) {
	ctx := context.Background()
	_, repo, _, _ := setupEntityRepo(t)

	assert.Empty(t, mustList(t, repo, false), "빈 저장소의 목록은 비어 있다")

	// sort_order 는 Create 가 소유자별로 부여하므로(입력값 무시), 정렬 검증에 필요한
	// 값은 Update 로 명시한다 — Update 는 여전히 SortOrder 를 받는다.
	for _, d := range []Dashboard{
		newDashboard("D3", "edi", "shared"),
		newDashboard("D1", "edi", "private"),
		newDashboard("D2", "root", "acl"),
	} {
		_, err := repo.Create(ctx, d)
		require.NoError(t, err)
	}
	for uid, order := range map[string]int64{"D1": 1, "D2": 1, "D3": 2} {
		o := order
		_, err := repo.Update(ctx, uid, DashboardUpdate{SortOrder: &o}, -1)
		require.NoError(t, err)
	}

	// sort_order → uid 순.
	list := mustList(t, repo, false)
	require.Len(t, list, 3)
	assert.Equal(t, []string{"D1", "D2", "D3"}, []string{list[0].UID, list[1].UID, list[2].UID})
	for _, d := range list {
		assert.Nil(t, d.Payload, "includePayload=false 이면 payload 를 읽지 않는다")
	}

	withPayload := mustList(t, repo, true)
	require.Len(t, withPayload, 3)
	for _, d := range withPayload {
		assert.JSONEq(t, `{"panels":[],"layout":[]}`, string(d.Payload), "includePayload=true 이면 payload 를 담는다")
	}
}

// mustList 는 List 호출 헬퍼이다.
func mustList(t *testing.T, repo *DashboardEntitySQLiteRepository, includePayload bool) []Dashboard {
	t.Helper()
	list, err := repo.List(context.Background(), includePayload)
	require.NoError(t, err, "목록 조회 실패")
	return list
}

// TestDashboardEntity_Update_PartialAndVersion 은 부분 갱신과 서버 부여 version 을
// 검증한다.
func TestDashboardEntity_Update_PartialAndVersion(t *testing.T) {
	ctx := context.Background()
	_, repo, _, _ := setupEntityRepo(t)

	created, err := repo.Create(ctx, newDashboard("D1", "edi", "private"))
	require.NoError(t, err)

	// payload 만 갱신 — 이름·공개범위는 보존된다.
	name := "이름변경"
	updated, err := repo.Update(ctx, "D1", DashboardUpdate{Payload: []byte(`{"panels":[1]}`)}, -1)
	require.NoError(t, err)
	assert.EqualValues(t, 2, updated.Version, "version 은 서버가 1 증가시킨다")
	assert.Equal(t, "대시보드 D1", updated.Name, "미지정 필드는 보존된다")
	assert.JSONEq(t, `{"panels":[1]}`, string(updated.Payload))
	assert.GreaterOrEqual(t, updated.UpdatedAt, created.UpdatedAt)

	// 메타만 갱신 — payload 는 보존된다.
	vis := "shared"
	isDefault := true
	sortOrder := int64(7)
	owner := "root"
	updated, err = repo.Update(ctx, "D1", DashboardUpdate{
		Name:       &name,
		Visibility: &vis,
		IsDefault:  &isDefault,
		SortOrder:  &sortOrder,
		Owner:      &owner,
	}, -1)
	require.NoError(t, err)
	assert.EqualValues(t, 3, updated.Version)
	assert.Equal(t, name, updated.Name)
	assert.Equal(t, "shared", updated.Visibility)
	assert.True(t, updated.IsDefault)
	assert.EqualValues(t, 7, updated.SortOrder)
	assert.Equal(t, "root", updated.Owner)
	assert.JSONEq(t, `{"panels":[1]}`, string(updated.Payload), "payload 는 보존된다")

	// 없는 uid 갱신은 NotFound.
	_, err = repo.Update(ctx, "nope", DashboardUpdate{Payload: []byte(`{}`)}, -1)
	assert.ErrorIs(t, err, ErrDashboardNotFound)
}

// TestDashboardEntity_Update_IfMatchConflict 는 낙관적 동시성을 검증한다
// (acceptance.md AC-11).
func TestDashboardEntity_Update_IfMatchConflict(t *testing.T) {
	ctx := context.Background()
	_, repo, _, _ := setupEntityRepo(t)

	_, err := repo.Create(ctx, newDashboard("D7", "root", "private"))
	require.NoError(t, err) // version = 1

	// 불일치 → 409 상당, 서버 상태 불변.
	_, err = repo.Update(ctx, "D7", DashboardUpdate{Payload: []byte(`{"x":1}`)}, 0)
	assert.ErrorIs(t, err, ErrDashboardVersionMismatch)

	after, err := repo.Get(ctx, "D7")
	require.NoError(t, err)
	assert.EqualValues(t, 1, after.Version, "충돌 시 version 이 변하지 않는다")
	assert.JSONEq(t, `{"panels":[],"layout":[]}`, string(after.Payload), "충돌 시 payload 도 변하지 않는다")

	// 일치 → 저장되고 version 증가.
	ok, err := repo.Update(ctx, "D7", DashboardUpdate{Payload: []byte(`{"x":1}`)}, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 2, ok.Version)

	// 헤더 없음(-1) → 무조건 저장.
	ok, err = repo.Update(ctx, "D7", DashboardUpdate{Payload: []byte(`{"x":2}`)}, -1)
	require.NoError(t, err)
	assert.EqualValues(t, 3, ok.Version)
}

// TestDashboardEntity_Delete_RemovesACL 은 대시보드 삭제 시 ACL 이 함께 사라짐을
// 검증한다 (acceptance.md AC-08 — 고아 ACL 행 0개).
func TestDashboardEntity_Delete_RemovesACL(t *testing.T) {
	ctx := context.Background()
	db, repo, aclRepo, _ := setupEntityRepo(t)

	created, err := repo.Create(ctx, newDashboard("D5", "root", "acl"))
	require.NoError(t, err)
	require.NoError(t, aclRepo.Replace(ctx, created.ID, []DashboardACLEntry{
		{Subject: "user:edi", Level: "view", GrantedBy: "root"},
		{Subject: "role:operator", Level: "edit", GrantedBy: "root"},
	}))

	require.NoError(t, repo.Delete(ctx, "D5"))

	var orphans int64
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM dashboard_acl WHERE dashboard_id NOT IN (SELECT id FROM dashboards)`).Scan(&orphans))
	assert.EqualValues(t, 0, orphans, "고아 ACL 행이 남으면 안 된다")
}

// ---------------------------------------------------------------------------
// dashboard_acl
// ---------------------------------------------------------------------------

// TestDashboardACL_ReplaceIsFullReplacement 는 Replace 가 부분 갱신이 아니라
// 전량 치환임을 검증한다 (spec.md §4.5, acceptance.md AC-06).
func TestDashboardACL_ReplaceIsFullReplacement(t *testing.T) {
	ctx := context.Background()
	_, repo, aclRepo, _ := setupEntityRepo(t)

	d, err := repo.Create(ctx, newDashboard("D2", "edi", "acl"))
	require.NoError(t, err)

	before := time.Now().UnixMilli()
	require.NoError(t, aclRepo.Replace(ctx, d.ID, []DashboardACLEntry{
		{Subject: "user:ops", Level: "view", GrantedBy: "edi"},
		{Subject: "role:operator", Level: "edit", GrantedBy: "edi"},
	}))

	entries, err := aclRepo.ListByDashboard(ctx, d.ID)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	// subject 사전순: role:operator < user:ops
	assert.Equal(t, "role:operator", entries[0].Subject)
	assert.Equal(t, "edit", entries[0].Level)
	assert.Equal(t, "user:ops", entries[1].Subject)
	assert.Equal(t, "view", entries[1].Level)
	assert.Equal(t, "edi", entries[1].GrantedBy)
	assert.GreaterOrEqual(t, entries[1].GrantedAt, before, "granted_at 은 서버 시각")

	// 치환: 기존 2행이 사라지고 새 1행만 남는다.
	require.NoError(t, aclRepo.Replace(ctx, d.ID, []DashboardACLEntry{
		{Subject: "user:vie", Level: "edit", GrantedBy: "root"},
	}))
	entries, err = aclRepo.ListByDashboard(ctx, d.ID)
	require.NoError(t, err)
	require.Len(t, entries, 1, "이전 행이 남아 있으면 전량 치환이 아니다")
	assert.Equal(t, "user:vie", entries[0].Subject)
	assert.Equal(t, "edit", entries[0].Level)
	assert.Equal(t, "root", entries[0].GrantedBy)

	// 빈 배열 치환 = 전량 삭제.
	require.NoError(t, aclRepo.Replace(ctx, d.ID, nil))
	entries, err = aclRepo.ListByDashboard(ctx, d.ID)
	require.NoError(t, err)
	assert.Empty(t, entries, "빈 배열 치환은 전량 삭제와 같다")
}

// TestDashboardACL_ReplaceRejectsInvalidLevel 은 CHECK 제약이 미지원 레벨을
// 거부하고, 실패 시 기존 ACL 이 보존됨을 검증한다.
func TestDashboardACL_ReplaceRejectsInvalidLevel(t *testing.T) {
	ctx := context.Background()
	_, repo, aclRepo, _ := setupEntityRepo(t)

	d, err := repo.Create(ctx, newDashboard("D9", "root", "acl"))
	require.NoError(t, err)
	require.NoError(t, aclRepo.Replace(ctx, d.ID, []DashboardACLEntry{
		{Subject: "user:edi", Level: "view", GrantedBy: "root"},
	}))

	err = aclRepo.Replace(ctx, d.ID, []DashboardACLEntry{
		{Subject: "user:edi", Level: "manage", GrantedBy: "root"},
	})
	require.Error(t, err, "미지원 레벨은 CHECK 제약이 거부한다")

	// 트랜잭션 롤백으로 기존 ACL 이 보존된다 (부분 반영 없음).
	entries, err := aclRepo.ListByDashboard(ctx, d.ID)
	require.NoError(t, err)
	require.Len(t, entries, 1, "실패 시 ACL 이 전혀 변경되지 않아야 한다")
	assert.Equal(t, "view", entries[0].Level)
}

// TestDashboardACL_ListBySubjects 는 user: 와 role: 두 subject 형태를 모두
// 매치하는지 검증한다.
func TestDashboardACL_ListBySubjects(t *testing.T) {
	ctx := context.Background()
	_, repo, aclRepo, _ := setupEntityRepo(t)

	d1, err := repo.Create(ctx, newDashboard("D1", "edi", "acl"))
	require.NoError(t, err)
	d2, err := repo.Create(ctx, newDashboard("D2", "edi", "acl"))
	require.NoError(t, err)
	d3, err := repo.Create(ctx, newDashboard("D3", "edi", "acl"))
	require.NoError(t, err)

	require.NoError(t, aclRepo.Replace(ctx, d1.ID, []DashboardACLEntry{
		{Subject: "user:ops", Level: "view", GrantedBy: "edi"},
	}))
	require.NoError(t, aclRepo.Replace(ctx, d2.ID, []DashboardACLEntry{
		{Subject: "role:operator", Level: "edit", GrantedBy: "edi"},
	}))
	require.NoError(t, aclRepo.Replace(ctx, d3.ID, []DashboardACLEntry{
		{Subject: "user:someone-else", Level: "edit", GrantedBy: "edi"},
	}))

	// 두 형태를 한 번의 질의로 가져온다 (행당 추가 질의 없음).
	entries, err := aclRepo.ListBySubjects(ctx, []string{"user:ops", "role:operator"})
	require.NoError(t, err)
	require.Len(t, entries, 2, "user: 와 role: 형태가 모두 매치되어야 한다")

	byDashboard := map[int64]DashboardACLEntry{}
	for _, e := range entries {
		byDashboard[e.DashboardID] = e
	}
	assert.Equal(t, "user:ops", byDashboard[d1.ID].Subject)
	assert.Equal(t, "view", byDashboard[d1.ID].Level)
	assert.Equal(t, "role:operator", byDashboard[d2.ID].Subject)
	assert.Equal(t, "edit", byDashboard[d2.ID].Level)
	assert.NotContains(t, byDashboard, d3.ID, "매치되지 않는 대시보드는 포함되지 않는다")

	// 빈 입력은 빈 결과 (전량 조회로 퇴화하지 않는다).
	entries, err = aclRepo.ListBySubjects(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, entries, "subject 가 없으면 매치도 없다")
}

// TestDashboardACL_DeleteByDashboard 는 대시보드 단위 ACL 삭제를 검증한다.
func TestDashboardACL_DeleteByDashboard(t *testing.T) {
	ctx := context.Background()
	_, repo, aclRepo, _ := setupEntityRepo(t)

	d, err := repo.Create(ctx, newDashboard("D1", "edi", "acl"))
	require.NoError(t, err)
	require.NoError(t, aclRepo.Replace(ctx, d.ID, []DashboardACLEntry{
		{Subject: "user:ops", Level: "view", GrantedBy: "edi"},
	}))

	require.NoError(t, aclRepo.DeleteByDashboard(ctx, d.ID))
	entries, err := aclRepo.ListByDashboard(ctx, d.ID)
	require.NoError(t, err)
	assert.Empty(t, entries)

	// 멱등하다.
	assert.NoError(t, aclRepo.DeleteByDashboard(ctx, d.ID))
}

// ---------------------------------------------------------------------------
// dashboard_user_state
// ---------------------------------------------------------------------------

// TestDashboardUserState_RoundTrip 은 Get/Put 왕복과 서버 부여 version 을
// 검증한다 (acceptance.md AC-14).
func TestDashboardUserState_RoundTrip(t *testing.T) {
	ctx := context.Background()
	_, _, _, stateRepo := setupEntityRepo(t)

	_, err := stateRepo.Get(ctx, "root")
	assert.ErrorIs(t, err, ErrDashboardUserStateNotFound, "저장 전에는 NotFound")

	before := time.Now().UnixMilli()
	put, err := stateRepo.Put(ctx, DashboardUserState{
		Username:           "root",
		ActiveDashboardUID: "D1",
		DeviceGridLayout:   []byte(`{"dev-1":{"x":0,"y":0}}`),
	}, -1)
	require.NoError(t, err)
	assert.EqualValues(t, 1, put.Version, "최초 저장 시 version 은 1")
	assert.GreaterOrEqual(t, put.UpdatedAt, before)

	got, err := stateRepo.Get(ctx, "root")
	require.NoError(t, err)
	assert.Equal(t, "root", got.Username)
	assert.Equal(t, "D1", got.ActiveDashboardUID)
	assert.JSONEq(t, `{"dev-1":{"x":0,"y":0}}`, string(got.DeviceGridLayout))
	assert.EqualValues(t, 1, got.Version)

	// 갱신 시 version 증가.
	put, err = stateRepo.Put(ctx, DashboardUserState{
		Username:           "root",
		ActiveDashboardUID: "D2",
	}, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 2, put.Version)
	assert.JSONEq(t, `{}`, string(put.DeviceGridLayout), "빈 레이아웃은 빈 JSON 객체로 정규화된다")

	// 사용자별로 분리된다.
	_, err = stateRepo.Get(ctx, "edi")
	assert.ErrorIs(t, err, ErrDashboardUserStateNotFound)
}

// TestDashboardUserState_IfMatchConflict 는 상태 저장의 낙관적 동시성을 검증한다.
func TestDashboardUserState_IfMatchConflict(t *testing.T) {
	ctx := context.Background()
	_, _, _, stateRepo := setupEntityRepo(t)

	_, err := stateRepo.Put(ctx, DashboardUserState{Username: "root", ActiveDashboardUID: "D1"}, 0)
	require.NoError(t, err, "행이 없으면 현재 version 은 0 으로 간주된다")

	_, err = stateRepo.Put(ctx, DashboardUserState{Username: "root", ActiveDashboardUID: "D2"}, 0)
	assert.ErrorIs(t, err, ErrDashboardVersionMismatch)

	got, err := stateRepo.Get(ctx, "root")
	require.NoError(t, err)
	assert.Equal(t, "D1", got.ActiveDashboardUID, "충돌 시 상태가 변하지 않는다")
	assert.EqualValues(t, 1, got.Version)
}

// ---------------------------------------------------------------------------
// 생성자 방어
// ---------------------------------------------------------------------------

// TestEntityRepositories_NilDB 는 nil *sql.DB 주입을 거부하는지 검증한다.
func TestEntityRepositories_NilDB(t *testing.T) {
	ctx := context.Background()

	_, err := NewDashboardEntitySQLiteRepository(ctx, nil)
	assert.Error(t, err)
	_, err = NewDashboardACLSQLiteRepository(ctx, nil)
	assert.Error(t, err)
	_, err = NewDashboardUserStateSQLiteRepository(ctx, nil)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// sort_order 소유자별 시퀀스 (SPEC-DASHBOARD-004 M4)
// ---------------------------------------------------------------------------

// TestDashboardEntity_Create_AssignsPerOwnerSortOrder 는 생성 시 sort_order 가
// 소유자별로 증가하고, 소유자끼리는 독립임을 검증한다.
//
// uid 는 서버가 UUID 로 발급하므로, sort_order 가 전부 0 이면 목록 순서가 uid 순으로
// 떨어져 새로 만든 대시보드끼리 임의로 뒤섞인다 — 사용자에게 보이는 결함이다.
func TestDashboardEntity_Create_AssignsPerOwnerSortOrder(t *testing.T) {
	ctx := context.Background()
	_, repo, _, _ := setupEntityRepo(t)

	t.Run("같은 소유자는 증가한다", func(t *testing.T) {
		first, err := repo.Create(ctx, newDashboard("A1", "edi", "private"))
		require.NoError(t, err)
		second, err := repo.Create(ctx, newDashboard("A2", "edi", "private"))
		require.NoError(t, err)
		third, err := repo.Create(ctx, newDashboard("A3", "edi", "private"))
		require.NoError(t, err)

		assert.EqualValues(t, 0, first.SortOrder, "그 소유자의 첫 대시보드는 0")
		assert.EqualValues(t, 1, second.SortOrder)
		assert.EqualValues(t, 2, third.SortOrder)

		// 반환값뿐 아니라 저장된 행도 같아야 한다.
		stored, err := repo.Get(ctx, "A2")
		require.NoError(t, err)
		assert.EqualValues(t, 1, stored.SortOrder)
	})

	t.Run("소유자끼리는 독립이다", func(t *testing.T) {
		// edi 는 이미 0,1,2 를 썼다. root 는 자기 시퀀스를 0 부터 시작한다.
		rootFirst, err := repo.Create(ctx, newDashboard("B1", "root", "shared"))
		require.NoError(t, err)
		assert.EqualValues(t, 0, rootFirst.SortOrder,
			"다른 소유자의 값이 시퀀스에 섞이면 안 된다")

		rootSecond, err := repo.Create(ctx, newDashboard("B2", "root", "shared"))
		require.NoError(t, err)
		assert.EqualValues(t, 1, rootSecond.SortOrder)

		// 그 사이에 edi 가 하나 더 만들어도 edi 시퀀스만 이어진다.
		ediNext, err := repo.Create(ctx, newDashboard("A4", "edi", "private"))
		require.NoError(t, err)
		assert.EqualValues(t, 3, ediNext.SortOrder)
	})

	t.Run("입력의 SortOrder 는 무시된다", func(t *testing.T) {
		d := newDashboard("C1", "ops", "private")
		d.SortOrder = 99
		created, err := repo.Create(ctx, d)
		require.NoError(t, err)
		assert.EqualValues(t, 0, created.SortOrder, "서버가 부여한다")
	})
}

// TestDashboardEntity_Create_ContinuesAfterMigratedSortOrder 는 이관으로 이미
// sort_order 를 가진 소유자의 다음 생성이 그 최댓값 뒤에서 이어짐을 검증한다.
//
// M3 이관(insertMigratedDashboards)은 dashboardPages 배열 인덱스를 sort_order 로
// 넣으므로 이관 직후 사용자는 0..M-1 을 갖는다. 새 대시보드가 0 부터 다시 시작하면
// 이관된 대시보드와 순서가 충돌한다.
func TestDashboardEntity_Create_ContinuesAfterMigratedSortOrder(t *testing.T) {
	ctx := context.Background()
	db, repo, _, _ := setupEntityRepo(t)

	// 이관이 남긴 것과 같은 모양의 행을 직접 넣는다 (0,1,2 = 배열 인덱스).
	for i, uid := range []string{"M0", "M1", "M2"} {
		_, err := db.ExecContext(ctx, `
			INSERT INTO dashboards(uid, name, owner, visibility, is_default, sort_order, version, created_at, updated_at, payload)
			VALUES (?, ?, 'alice', 'private', 0, ?, 1, 1000, 1000, '{"panels":[],"layout":[]}')
		`, uid, "이관된 "+uid, int64(i))
		require.NoError(t, err)
	}

	created, err := repo.Create(ctx, newDashboard("NEW", "alice", "private"))
	require.NoError(t, err)
	assert.EqualValues(t, 3, created.SortOrder,
		"이관된 최댓값(2) 다음에서 이어져야 한다")

	// 다른 소유자는 여전히 0 부터 시작한다.
	other, err := repo.Create(ctx, newDashboard("OTHER", "bob", "private"))
	require.NoError(t, err)
	assert.EqualValues(t, 0, other.SortOrder)

	// 목록은 alice 의 이관 3장 뒤에 새 대시보드가 온다.
	list := mustList(t, repo, false)
	var aliceOrder []string
	for _, d := range list {
		if d.Owner == "alice" {
			aliceOrder = append(aliceOrder, d.UID)
		}
	}
	assert.Equal(t, []string{"M0", "M1", "M2", "NEW"}, aliceOrder)
}

// ---------------------------------------------------------------------------
// is_default 소유자별 정규화 (SPEC-DASHBOARD-004, acceptance.md 엣지 케이스)
// ---------------------------------------------------------------------------

// isDefaultUIDs 는 owner 소유 대시보드 중 is_default 인 uid 목록을 반환한다.
func isDefaultUIDs(t *testing.T, repo *DashboardEntitySQLiteRepository, owner string) []string {
	t.Helper()
	var out []string
	for _, d := range mustList(t, repo, false) {
		if d.Owner == owner && d.IsDefault {
			out = append(out, d.UID)
		}
	}
	return out
}

// TestDashboardEntity_Update_NormalizesIsDefaultPerOwner 는 기본 대시보드 지정이
// 같은 소유자의 기존 기본을 해제하고, 다른 소유자에게는 영향을 주지 않음을 검증한다.
//
// acceptance.md: "is_default 가 2장 이상에 설정됨 → 마지막 것만 유지하고 나머지는
// 0으로 정규화". 정규화 범위가 소유자인 이유는 Update 의 주석 참조 — 전역이면 한
// 사용자의 지정이 다른 사용자의 기본을 조용히 해제한다.
func TestDashboardEntity_Update_NormalizesIsDefaultPerOwner(t *testing.T) {
	ctx := context.Background()
	_, repo, _, _ := setupEntityRepo(t)

	for _, d := range []Dashboard{
		newDashboard("E1", "edi", "private"),
		newDashboard("E2", "edi", "private"),
		newDashboard("E3", "edi", "private"),
		newDashboard("R1", "root", "private"),
	} {
		_, err := repo.Create(ctx, d)
		require.NoError(t, err)
	}

	yes := true
	setDefault := func(uid string) *Dashboard {
		t.Helper()
		d, err := repo.Update(ctx, uid, DashboardUpdate{IsDefault: &yes}, -1)
		require.NoError(t, err)
		return d
	}

	// 다른 소유자(root)가 먼저 자기 기본을 지정해 둔다.
	setDefault("R1")
	require.Equal(t, []string{"R1"}, isDefaultUIDs(t, repo, "root"))

	// edi 의 첫 지정.
	first := setDefault("E1")
	assert.True(t, first.IsDefault)
	assert.Equal(t, []string{"E1"}, isDefaultUIDs(t, repo, "edi"))

	// 두 번째 지정이 첫 번째를 해제한다 — "마지막 것만 유지".
	second := setDefault("E2")
	assert.True(t, second.IsDefault)
	assert.Equal(t, []string{"E2"}, isDefaultUIDs(t, repo, "edi"),
		"같은 소유자의 이전 기본은 해제되어야 한다")

	// 세 번째도 마찬가지.
	setDefault("E3")
	assert.Equal(t, []string{"E3"}, isDefaultUIDs(t, repo, "edi"))

	// 다른 소유자의 기본은 그대로다.
	assert.Equal(t, []string{"R1"}, isDefaultUIDs(t, repo, "root"),
		"타 소유자의 기본 대시보드가 해제되면 안 된다")

	// 해제된 행의 version 은 오르지 않는다 — 무관한 클라이언트의 If-Match 를
	// 깨뜨리지 않기 위함이다(Update 주석 참조).
	e1, err := repo.Get(ctx, "E1")
	require.NoError(t, err)
	assert.EqualValues(t, 2, e1.Version, "E1 은 자기 지정(1→2) 이후 더 오르지 않는다")
	assert.False(t, e1.IsDefault)
}

// TestDashboardEntity_Update_IsDefaultFalseDoesNotNormalize 는 기본 해제(false)가
// 다른 행을 건드리지 않음을 검증한다. 정규화는 "올릴 때" 만 필요하다.
func TestDashboardEntity_Update_IsDefaultFalseDoesNotNormalize(t *testing.T) {
	ctx := context.Background()
	_, repo, _, _ := setupEntityRepo(t)

	for _, uid := range []string{"F1", "F2"} {
		_, err := repo.Create(ctx, newDashboard(uid, "edi", "private"))
		require.NoError(t, err)
	}
	yes, no := true, false
	_, err := repo.Update(ctx, "F1", DashboardUpdate{IsDefault: &yes}, -1)
	require.NoError(t, err)

	// F2 를 명시적으로 false 로 두어도 F1 의 기본이 유지된다.
	_, err = repo.Update(ctx, "F2", DashboardUpdate{IsDefault: &no}, -1)
	require.NoError(t, err)
	assert.Equal(t, []string{"F1"}, isDefaultUIDs(t, repo, "edi"))
}

// TestDashboardEntity_Update_NormalizesAgainstNewOwner 는 소유권 이전과 기본 지정이
// 함께 오면 **새 소유자** 기준으로 정규화됨을 검증한다.
func TestDashboardEntity_Update_NormalizesAgainstNewOwner(t *testing.T) {
	ctx := context.Background()
	_, repo, _, _ := setupEntityRepo(t)

	for _, d := range []Dashboard{
		newDashboard("G1", "root", "private"), // root 의 기존 기본
		newDashboard("G2", "edi", "private"),  // edi 의 기존 기본
		newDashboard("G3", "edi", "private"),  // 이전 대상
	} {
		_, err := repo.Create(ctx, d)
		require.NoError(t, err)
	}
	yes := true
	_, err := repo.Update(ctx, "G1", DashboardUpdate{IsDefault: &yes}, -1)
	require.NoError(t, err)
	_, err = repo.Update(ctx, "G2", DashboardUpdate{IsDefault: &yes}, -1)
	require.NoError(t, err)

	// G3 을 root 에게 넘기면서 동시에 기본으로 지정한다.
	newOwner := "root"
	_, err = repo.Update(ctx, "G3", DashboardUpdate{Owner: &newOwner, IsDefault: &yes}, -1)
	require.NoError(t, err)

	assert.Equal(t, []string{"G3"}, isDefaultUIDs(t, repo, "root"),
		"새 소유자(root) 쪽의 기존 기본이 해제되어야 한다")
	assert.Equal(t, []string{"G2"}, isDefaultUIDs(t, repo, "edi"),
		"옛 소유자(edi) 의 기본은 건드리지 않는다")
}
