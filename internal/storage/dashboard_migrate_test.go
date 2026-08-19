package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @SPEC:SPEC-DASHBOARD-004 (M3, spec.md §2.4, plan.md M3 검증 표)
// 본 파일은 구 스냅샷 → 대시보드 1급 엔티티 1회성 이관을 검증한다.
//
// 이관은 롤백 불가 지점이므로 plan.md 의 검증 표 전 항목을 자동화 테스트로 고정한다:
// 원본 보존 / 장수 일치 / 소유자 정합 / 멱등성 / 빈 상태 유지 / 사전 백업.

// ---------------------------------------------------------------------------
// 픽스처
// ---------------------------------------------------------------------------

// newLegacyDB 는 구 스키마(users + 레거시 dashboards)만 가진 DB 를 연다.
// 반환값은 (*sql.DB, dbPath) 이다.
func newLegacyDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "legacy-migrate.db")

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err, "sql.Open 실패")
	t.Cleanup(func() { db.Close() })

	_, err = db.ExecContext(ctx, "PRAGMA journal_mode=WAL")
	require.NoError(t, err, "WAL 모드 설정 실패")
	require.NoError(t, migrateUsersSchema(ctx, db), "users 스키마 생성 실패")
	require.NoError(t, seedLegacyDashboardTable(ctx, db, dashboardsTable), "레거시 스키마 생성 실패")
	return db, dbPath
}

// insertUser 는 users 행 1개를 추가한다 (id 는 AUTOINCREMENT — 호출 순서 = id 순서).
func insertUser(t *testing.T, db *sql.DB, username, role string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO users(username, password_hash, role, created_at, updated_at) VALUES (?, 'x', ?, 1, 1)`,
		username, role)
	require.NoErrorf(t, err, "사용자 %q 삽입 실패", username)
}

// insertLegacySnapshot 은 레거시 스냅샷 1행을 추가한다.
func insertLegacySnapshot(t *testing.T, db *sql.DB, table, scope, owner string, updatedAt int64, payload string) {
	t.Helper()
	var dbOwner any
	if owner != "" {
		dbOwner = owner
	}
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO `+table+`(scope, owner, version, updated_at, payload) VALUES (?, ?, 1, ?, ?)`,
		scope, dbOwner, updatedAt, payload)
	require.NoErrorf(t, err, "%s 스냅샷 삽입 실패 (scope=%s owner=%s)", table, scope, owner)
}

// legacyPayload 는 구 payload JSON 을 만든다.
func legacyPayload(activeID string, deviceGridLayout string, pageIDs ...string) string {
	pages := make([]string, 0, len(pageIDs))
	for i, id := range pageIDs {
		pages = append(pages, fmt.Sprintf(
			`{"id":%q,"name":"페이지 %s","isDefault":%t,"panels":[{"id":"p%d","type":"flows","title":"t","config":{"k":%d}}],"layout":[{"i":"p%d","x":0,"y":0,"w":2,"h":2}]}`,
			id, id, i == 0, i, i, i))
	}
	if deviceGridLayout == "" {
		deviceGridLayout = "{}"
	}
	return fmt.Sprintf(
		`{"dashboardPages":[%s],"activeDashboardId":%q,"dashboardGridCols":10,"dashboardShowGridLines":true,"dashboardRefreshInterval":30,"deviceGridLayout":%s}`,
		joinComma(pages), activeID, deviceGridLayout)
}

func joinComma(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}

// countRows 는 단일 테이블의 행 수를 센다.
func countRows(t *testing.T, db *sql.DB, table string) int64 {
	t.Helper()
	var n int64
	require.NoErrorf(t, db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM `+table).Scan(&n), "%s 행 수 조회 실패", table)
	return n
}

// markerCount 는 이관 마커 행 수를 센다.
func markerCount(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM schema_markers WHERE key = ?`, dashboardEntityMarkerKey).Scan(&n))
	return n
}

// seedAC01Fixture 는 AC-01 픽스처를 구성한다.
// scope='global' 1행(3장) + scope='user' 2행(각 2장) = 대시보드 7장.
func seedAC01Fixture(t *testing.T, db *sql.DB) {
	t.Helper()
	insertUser(t, db, "root", "admin")
	insertUser(t, db, "alice", "editor")
	insertUser(t, db, "bob", "viewer")

	insertLegacySnapshot(t, db, "dashboards", "global", "", 1000, legacyPayload("g1", "", "g1", "g2", "g3"))
	insertLegacySnapshot(t, db, "dashboards", "user", "alice", 2000,
		legacyPayload("a2", `{"dev-1":{"i":"dev-1","x":0,"y":0,"w":1,"h":1}}`, "a1", "a2"))
	insertLegacySnapshot(t, db, "dashboards", "user", "bob", 3000, legacyPayload("b1", "", "b1", "b2"))
}

// ---------------------------------------------------------------------------
// AC-01 — 멱등성 / 원본 보존 / 장수 일치
// ---------------------------------------------------------------------------

// TestMigrateDashboardEntities_Idempotent 는 AC-01 을 검증한다.
// 3회 연속 실행해도 대시보드 행 수가 불변이고 마커는 정확히 1행이다.
func TestMigrateDashboardEntities_Idempotent(t *testing.T) {
	ctx := context.Background()
	db, _ := newLegacyDB(t)
	seedAC01Fixture(t, db)

	before := countRows(t, db, "dashboards")
	require.EqualValues(t, 3, before, "구 스냅샷은 3행이어야 한다")

	for i := 1; i <= 3; i++ {
		require.NoErrorf(t, migrateDashboardEntities(ctx, db), "%d회차 이관 실패", i)

		assert.EqualValuesf(t, 7, countRows(t, db, "dashboards"),
			"%d회차 후 대시보드는 7장이어야 한다 (3 + 2 + 2)", i)
		assert.EqualValuesf(t, 3, countRows(t, db, dashboardSnapshotsV1Table),
			"%d회차 후에도 원본 스냅샷 3행이 그대로 남아야 한다", i)
		assert.EqualValuesf(t, 1, markerCount(t, db), "%d회차 후 마커는 정확히 1행", i)
	}
}

// TestMigrateDashboardEntities_PreservesLegacyTable 는 원본 보존을 명시적으로
// 검증한다 — 이관 전후 dashboard_snapshots_v1 의 행 수와 payload 가 동일하다.
func TestMigrateDashboardEntities_PreservesLegacyTable(t *testing.T) {
	ctx := context.Background()
	db, _ := newLegacyDB(t)
	seedAC01Fixture(t, db)

	type row struct{ scope, owner, payload string }
	readAll := func(table string) []row {
		rows, err := db.QueryContext(ctx,
			`SELECT scope, COALESCE(owner,''), payload FROM `+table+` ORDER BY id`)
		require.NoError(t, err)
		defer rows.Close()
		var out []row
		for rows.Next() {
			var r row
			require.NoError(t, rows.Scan(&r.scope, &r.owner, &r.payload))
			out = append(out, r)
		}
		return out
	}

	before := readAll("dashboards")
	require.NoError(t, migrateDashboardEntities(ctx, db))
	after := readAll(dashboardSnapshotsV1Table)

	assert.Equal(t, before, after, "개명된 원본 테이블의 내용은 이관 전과 완전히 동일해야 한다")
}

// TestMigrateDashboardEntities_DashboardCountMatchesPageSum 는 장수 일치를
// 검증한다 — 신규 대시보드 행 수 == 모든 스냅샷의 dashboardPages 길이 합.
func TestMigrateDashboardEntities_DashboardCountMatchesPageSum(t *testing.T) {
	ctx := context.Background()
	db, _ := newLegacyDB(t)
	insertUser(t, db, "root", "admin")

	// 4 + 1 + 0 = 5장
	insertLegacySnapshot(t, db, "dashboards", "global", "", 1000, legacyPayload("g1", "", "g1", "g2", "g3", "g4"))
	insertLegacySnapshot(t, db, "dashboards", "user", "alice", 2000, legacyPayload("a1", "", "a1"))
	insertLegacySnapshot(t, db, "dashboards", "user", "bob", 3000, legacyPayload("", ""))

	wantSum := int64(0)
	rows, err := db.QueryContext(ctx, `SELECT payload FROM dashboards`)
	require.NoError(t, err)
	for rows.Next() {
		var raw []byte
		require.NoError(t, rows.Scan(&raw))
		var p legacyDashboardSnapshotPayload
		require.NoError(t, json.Unmarshal(raw, &p))
		wantSum += int64(len(p.DashboardPages))
	}
	rows.Close()
	require.EqualValues(t, 5, wantSum, "픽스처의 페이지 합은 5여야 한다")

	require.NoError(t, migrateDashboardEntities(ctx, db))
	assert.Equal(t, wantSum, countRows(t, db, "dashboards"), "대시보드 장수는 페이지 합과 같아야 한다")
}

// TestMigrateDashboardEntities_OwnerFidelity 는 소유자 정합을 검증한다.
// scope='user' 출처 행의 owner 는 원본 owner 와 일치하고 visibility 는 private,
// scope='global' 출처는 최초 admin 소유의 shared 다.
func TestMigrateDashboardEntities_OwnerFidelity(t *testing.T) {
	ctx := context.Background()
	db, _ := newLegacyDB(t)
	seedAC01Fixture(t, db)

	require.NoError(t, migrateDashboardEntities(ctx, db))

	type meta struct{ owner, visibility string }
	got := map[string]meta{}
	rows, err := db.QueryContext(ctx, `SELECT uid, owner, visibility FROM dashboards`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var uid string
		var m meta
		require.NoError(t, rows.Scan(&uid, &m.owner, &m.visibility))
		got[uid] = m
	}

	want := map[string]meta{
		"g1": {"root", "shared"},
		"g2": {"root", "shared"},
		"g3": {"root", "shared"},
		"a1": {"alice", "private"},
		"a2": {"alice", "private"},
		"b1": {"bob", "private"},
		"b2": {"bob", "private"},
	}
	assert.Equal(t, want, got, "소유자·공개범위가 출처 스코프와 정확히 대응해야 한다")
}

// TestMigrateDashboardEntities_InheritsNameDefaultAndSortOrder 는 name /
// is_default / sort_order 승계를 검증한다.
func TestMigrateDashboardEntities_InheritsNameDefaultAndSortOrder(t *testing.T) {
	ctx := context.Background()
	db, _ := newLegacyDB(t)
	insertUser(t, db, "root", "admin")
	insertLegacySnapshot(t, db, "dashboards", "user", "alice", 2000, legacyPayload("a1", "", "a1", "a2", "a3"))

	require.NoError(t, migrateDashboardEntities(ctx, db))

	for i, uid := range []string{"a1", "a2", "a3"} {
		var name string
		var isDefault, sortOrder, version int64
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT name, is_default, sort_order, version FROM dashboards WHERE uid = ?`, uid).
			Scan(&name, &isDefault, &sortOrder, &version))
		assert.Equalf(t, "페이지 "+uid, name, "%s 의 이름이 승계되어야 한다", uid)
		assert.EqualValuesf(t, i, sortOrder, "%s 의 sort_order 는 배열 인덱스여야 한다", uid)
		assert.EqualValuesf(t, 1, version, "%s 의 초기 version 은 1", uid)
		if i == 0 {
			assert.EqualValues(t, 1, isDefault, "첫 페이지의 isDefault 가 승계되어야 한다")
		} else {
			assert.EqualValuesf(t, 0, isDefault, "%s 는 기본 대시보드가 아니다", uid)
		}
	}
}

// ---------------------------------------------------------------------------
// AC-02 — 이관 후 비운 상태 유지
// ---------------------------------------------------------------------------

// TestMigrateDashboardEntities_EmptyAfterMigrationStaysEmpty 는 AC-02 를 검증한다.
// 관리자가 전 대시보드를 삭제한 뒤 재기동해도 재생성되지 않는다.
//
// 이 시나리오가 "행 수로 판정하면 안 되는" 이유다 — 행 수 판정이면 삭제한
// 대시보드가 재기동마다 되살아난다.
func TestMigrateDashboardEntities_EmptyAfterMigrationStaysEmpty(t *testing.T) {
	ctx := context.Background()
	db, _ := newLegacyDB(t)
	seedAC01Fixture(t, db)

	require.NoError(t, migrateDashboardEntities(ctx, db))
	require.EqualValues(t, 7, countRows(t, db, "dashboards"))

	_, err := db.ExecContext(ctx, `DELETE FROM dashboards`)
	require.NoError(t, err, "관리자의 전량 삭제")

	for i := 1; i <= 2; i++ {
		require.NoErrorf(t, migrateDashboardEntities(ctx, db), "%d회차 재기동 실패", i)
		assert.EqualValuesf(t, 0, countRows(t, db, "dashboards"),
			"%d회차 재기동 후에도 비어 있어야 한다 (재생성 금지)", i)
	}
	assert.EqualValues(t, 3, countRows(t, db, dashboardSnapshotsV1Table), "원본은 여전히 3행")
	assert.EqualValues(t, 1, markerCount(t, db), "마커는 1행 유지")
}

// ---------------------------------------------------------------------------
// uid 충돌
// ---------------------------------------------------------------------------

// TestMigrateDashboardEntities_UIDCollisionSuffixesPersonal 는 전역·개인 스냅샷이
// 같은 페이지 id 를 쓸 때 개인 쪽에 "-<username>" 접미사가 붙고, 그 사용자의
// active_dashboard_uid 도 같은 트랜잭션에서 보정되는지 검증한다 (spec.md §2.4).
func TestMigrateDashboardEntities_UIDCollisionSuffixesPersonal(t *testing.T) {
	ctx := context.Background()
	db, _ := newLegacyDB(t)
	insertUser(t, db, "root", "admin")
	insertUser(t, db, "alice", "editor")

	insertLegacySnapshot(t, db, "dashboards", "global", "", 1000, legacyPayload("default", "", "default"))
	insertLegacySnapshot(t, db, "dashboards", "user", "alice", 2000, legacyPayload("default", "", "default", "extra"))

	require.NoError(t, migrateDashboardEntities(ctx, db))

	// 전역이 원본 uid 를 유지하고 개인 쪽이 접미사를 받는다.
	var globalOwner, globalVis string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT owner, visibility FROM dashboards WHERE uid = 'default'`).Scan(&globalOwner, &globalVis))
	assert.Equal(t, "root", globalOwner, "원본 uid 는 전역 대시보드가 유지한다")
	assert.Equal(t, "shared", globalVis)

	var personalOwner string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT owner FROM dashboards WHERE uid = 'default-alice'`).Scan(&personalOwner),
		"개인 대시보드는 '-<username>' 접미사를 받아야 한다")
	assert.Equal(t, "alice", personalOwner)

	// 충돌하지 않은 개인 대시보드는 접미사를 받지 않는다.
	assert.EqualValues(t, 1, countRowsWhere(t, db, "dashboards", `uid = 'extra'`),
		"충돌하지 않은 uid 는 그대로 유지된다")

	// active_dashboard_uid 가 접미사 붙은 uid 로 보정된다.
	var activeUID string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT active_dashboard_uid FROM dashboard_user_state WHERE username = 'alice'`).Scan(&activeUID))
	assert.Equal(t, "default-alice", activeUID,
		"충돌 보정된 uid 를 활성 대시보드 참조도 함께 따라가야 한다")
}

// TestMigrateDashboardEntities_ReservedUIDAvoided 는 예약어(shared/mine/state)를
// 페이지 id 로 쓰던 구 데이터가 uid 를 그대로 가져가지 않음을 검증한다
// (라우트 리터럴과 충돌 — spec.md §2.1).
func TestMigrateDashboardEntities_ReservedUIDAvoided(t *testing.T) {
	ctx := context.Background()
	db, _ := newLegacyDB(t)
	insertUser(t, db, "root", "admin")
	insertLegacySnapshot(t, db, "dashboards", "user", "alice", 2000, legacyPayload("shared", "", "shared", "mine", "state"))

	require.NoError(t, migrateDashboardEntities(ctx, db))

	assert.EqualValues(t, 3, countRows(t, db, "dashboards"), "3장 모두 보존되어야 한다")
	for _, reserved := range dashboardReservedUIDs {
		assert.EqualValuesf(t, 0, countRowsWhere(t, db, "dashboards", `uid = '`+reserved+`'`),
			"예약어 %q 는 uid 로 발급되면 안 된다", reserved)
	}
	assert.EqualValues(t, 1, countRowsWhere(t, db, "dashboards", `uid = 'shared-alice'`))
}

// countRowsWhere 는 조건에 맞는 행 수를 센다.
func countRowsWhere(t *testing.T, db *sql.DB, table, where string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM `+table+` WHERE `+where).Scan(&n))
	return n
}

// ---------------------------------------------------------------------------
// AC-14 — 사용자 UI 상태 분리
// ---------------------------------------------------------------------------

// TestMigrateDashboardEntities_UserStateSeparated 는 AC-14 를 검증한다.
// activeDashboardId / deviceGridLayout 이 사용자별 행으로 보존되고, 전역 스냅샷의
// 두 값은 이관되지 않는다.
func TestMigrateDashboardEntities_UserStateSeparated(t *testing.T) {
	ctx := context.Background()
	db, _ := newLegacyDB(t)
	insertUser(t, db, "root", "admin")

	globalLayout := `{"g-dev":{"i":"g-dev","x":9,"y":9,"w":9,"h":9}}`
	aliceLayout := `{"d1":{"i":"d1","x":0,"y":0,"w":1,"h":1},"d2":{"i":"d2","x":1,"y":0,"w":1,"h":1},"d3":{"i":"d3","x":2,"y":0,"w":1,"h":1}}`

	insertLegacySnapshot(t, db, "dashboards", "global", "", 1000, legacyPayload("gX", globalLayout, "gX"))
	insertLegacySnapshot(t, db, "dashboards", "user", "alice", 2000, legacyPayload("a1", aliceLayout, "a1", "a2"))

	require.NoError(t, migrateDashboardEntities(ctx, db))

	// 사용자 행이 생성되고 두 값이 보존된다.
	var activeUID, layout string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT active_dashboard_uid, device_grid_layout FROM dashboard_user_state WHERE username = 'alice'`).
		Scan(&activeUID, &layout))
	assert.Equal(t, "a1", activeUID)
	assert.JSONEq(t, aliceLayout, layout, "deviceGridLayout 3개 항목이 그대로 보존되어야 한다")

	// 전역 스냅샷 출처의 UI 상태는 이관되지 않는다 — 어느 사용자의 것인지
	// 정의되지 않기 때문이다(spec.md §2.4).
	assert.EqualValues(t, 1, countRows(t, db, "dashboard_user_state"),
		"UI 상태 행은 개인 스냅샷 수만큼만 생성된다")
	assert.EqualValues(t, 0, countRowsWhere(t, db, "dashboard_user_state",
		`device_grid_layout LIKE '%g-dev%' OR active_dashboard_uid = 'gX'`),
		"전역 스냅샷의 activeDashboardId / deviceGridLayout 은 버려져야 한다")
}

// ---------------------------------------------------------------------------
// payload 그리드 설정 복사
// ---------------------------------------------------------------------------

// TestMigrateDashboardEntities_CopiesGridSettings 는 출처 스냅샷의 그리드 설정
// 3종이 각 대시보드 payload 로 복사되는지 검증한다 (spec.md §2.4).
//
// 구 키 이름(dashboardGridCols 등)이 신규 payload 에서 접두사 없는 이름
// (gridCols 등)으로 바뀐다 (spec.md §2.1).
func TestMigrateDashboardEntities_CopiesGridSettings(t *testing.T) {
	ctx := context.Background()
	db, _ := newLegacyDB(t)
	insertUser(t, db, "root", "admin")

	insertLegacySnapshot(t, db, "dashboards", "user", "alice", 2000, `{
		"dashboardPages":[
			{"id":"a1","name":"A1","isDefault":true,"panels":[{"id":"p1","type":"flows","title":"t","config":{"visibleColumns":["name"]}}],"layout":[{"i":"p1","x":0,"y":0,"w":2,"h":2}]},
			{"id":"a2","name":"A2","isDefault":false,"panels":[],"layout":[]}
		],
		"activeDashboardId":"a1",
		"dashboardGridCols":12,
		"dashboardShowGridLines":false,
		"dashboardRefreshInterval":45,
		"deviceGridLayout":{}
	}`)

	require.NoError(t, migrateDashboardEntities(ctx, db))

	for _, uid := range []string{"a1", "a2"} {
		var payload string
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT payload FROM dashboards WHERE uid = ?`, uid).Scan(&payload))

		var got map[string]json.RawMessage
		require.NoErrorf(t, json.Unmarshal([]byte(payload), &got), "%s payload 파싱 실패", uid)

		assert.JSONEqf(t, `12`, string(got["gridCols"]), "%s 의 gridCols 는 출처 값이어야 한다", uid)
		assert.JSONEqf(t, `false`, string(got["showGridLines"]), "%s 의 showGridLines 는 출처 값이어야 한다", uid)
		assert.JSONEqf(t, `45`, string(got["refreshInterval"]), "%s 의 refreshInterval 은 출처 값이어야 한다", uid)
		require.Containsf(t, got, "panels", "%s payload 에 panels 가 있어야 한다", uid)
		require.Containsf(t, got, "layout", "%s payload 에 layout 이 있어야 한다", uid)
		assert.NotContainsf(t, got, "dashboardPages", "%s payload 에 구 배열이 남으면 안 된다", uid)
	}

	// 패널 세부 설정이 재직렬화로 유실되지 않는다.
	var a1 string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT payload FROM dashboards WHERE uid = 'a1'`).Scan(&a1))
	assert.Contains(t, a1, `"visibleColumns"`, "패널 config 의 알 수 없는 필드가 보존되어야 한다")
}

// ---------------------------------------------------------------------------
// 최초 admin 결정 / admin 부재 폴백
// ---------------------------------------------------------------------------

// TestMigrateDashboardEntities_FirstAdminIsLowestUserID 는 "최초 admin 사용자" 가
// users.id 오름차순 첫 행으로 **결정적으로** 정해지는지 검증한다.
//
// 사전순과 id 순이 어긋나도록 픽스처를 구성한다 — SQLite 가 우연히 돌려주는
// 순서에 기대면 같은 DB 에서도 결과가 달라져 이관이 재현 불가능해진다.
func TestMigrateDashboardEntities_FirstAdminIsLowestUserID(t *testing.T) {
	ctx := context.Background()
	db, _ := newLegacyDB(t)

	insertUser(t, db, "zed", "viewer")    // id=1, admin 아님
	insertUser(t, db, "mallory", "admin") // id=2, 최초 admin
	insertUser(t, db, "alice", "admin")   // id=3, 사전순으로는 첫 번째지만 나중에 생성됨

	insertLegacySnapshot(t, db, "dashboards", "global", "", 1000, legacyPayload("g1", "", "g1"))

	require.NoError(t, migrateDashboardEntities(ctx, db))

	var owner string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT owner FROM dashboards WHERE uid = 'g1'`).Scan(&owner))
	assert.Equal(t, "mallory", owner,
		"users.id 오름차순 첫 admin 이어야 한다 (사전순 아님)")
}

// TestMigrateDashboardEntities_NoAdminFallsBackToEmptyOwner 는 admin 사용자가
// 하나도 없을 때 부팅을 실패시키지 않고 빈 소유자로 이관하는지 검증한다
// (acceptance.md 엣지 케이스 + spec.md §2.10 인증 비활성 소유자 표기).
func TestMigrateDashboardEntities_NoAdminFallsBackToEmptyOwner(t *testing.T) {
	ctx := context.Background()
	db, _ := newLegacyDB(t)

	insertUser(t, db, "alice", "editor")
	insertUser(t, db, "bob", "viewer")
	insertLegacySnapshot(t, db, "dashboards", "global", "", 1000, legacyPayload("g1", "", "g1"))

	require.NoError(t, migrateDashboardEntities(ctx, db), "admin 부재가 부팅을 막으면 안 된다")

	var owner, visibility string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT owner, visibility FROM dashboards WHERE uid = 'g1'`).Scan(&owner, &visibility))
	assert.Equal(t, "", owner, "admin 이 없으면 소유자는 빈 문자열")
	assert.Equal(t, "shared", visibility, "공개범위는 shared 로 유지되어 계속 조회 가능해야 한다")
}

// TestMigrateDashboardEntities_NoUsersTable 는 users 테이블 자체가 없어도
// 이관이 실패하지 않음을 검증한다.
func TestMigrateDashboardEntities_NoUsersTable(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "nousers.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, seedLegacyDashboardTable(ctx, db, dashboardsTable))
	insertLegacySnapshot(t, db, "dashboards", "global", "", 1000, legacyPayload("g1", "", "g1"))

	require.NoError(t, migrateDashboardEntities(ctx, db))

	var owner string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT owner FROM dashboards WHERE uid = 'g1'`).Scan(&owner))
	assert.Equal(t, "", owner)
}

// ---------------------------------------------------------------------------
// 신규 설치
// ---------------------------------------------------------------------------

// TestMigrateDashboardEntities_FreshInstall 는 구 dashboards 테이블이 아예 없는
// 신규 설치에서 오류 없이 신규 테이블을 만들고 마커만 기록함을 검증한다.
func TestMigrateDashboardEntities_FreshInstall(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "fresh.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	require.NoError(t, migrateDashboardEntities(ctx, db), "신규 설치에서 오류가 없어야 한다")

	for _, table := range []string{"dashboards", "dashboard_acl", "dashboard_user_state", "schema_markers"} {
		exists, err := tableExists(ctx, db, table)
		require.NoError(t, err)
		assert.Truef(t, exists, "%s 테이블이 생성되어야 한다", table)
	}
	assert.EqualValues(t, 0, countRows(t, db, "dashboards"), "이관할 대시보드가 없다")
	assert.EqualValues(t, 1, markerCount(t, db), "마커는 기록된다")

	// 보존 원본 테이블은 만들어지지 않는다 (개명할 구 테이블이 없었다).
	archived, err := tableExists(ctx, db, dashboardSnapshotsV1Table)
	require.NoError(t, err)
	assert.False(t, archived, "신규 설치에는 보존 원본이 존재하지 않는다")
}

// ---------------------------------------------------------------------------
// 트랜잭션 롤백
// ---------------------------------------------------------------------------

// TestMigrateDashboardEntities_TransactionRollback 는 4단계 중간 실패 시
// 신규 dashboards 가 비고, 원본이 온전하며, 마커가 기록되지 않아 다음 기동에서
// 재시도됨을 검증한다.
//
// 실패 주입 방법: 신규 dashboards 자리에 기본값 없는 NOT NULL 컬럼을 가진 테이블을
// 미리 만들어 둔다. CREATE TABLE IF NOT EXISTS 는 no-op 이 되고 INSERT 가 실패한다.
func TestMigrateDashboardEntities_TransactionRollback(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "rollback.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	require.NoError(t, migrateUsersSchema(ctx, db))
	insertUser(t, db, "root", "admin")

	// 이미 개명이 끝난 상태를 직접 구성한다 (구 테이블은 보존 원본 이름으로 존재).
	require.NoError(t, seedLegacyDashboardTable(ctx, db, dashboardSnapshotsV1Table))
	insertLegacySnapshot(t, db, dashboardSnapshotsV1Table, "global", "", 1000, legacyPayload("g1", "", "g1", "g2"))
	insertLegacySnapshot(t, db, dashboardSnapshotsV1Table, "user", "alice", 2000, legacyPayload("a1", "", "a1"))

	// INSERT 를 반드시 실패시키는 dashboards 테이블.
	_, err = db.ExecContext(ctx, `CREATE TABLE dashboards (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		uid        TEXT    NOT NULL UNIQUE,
		name       TEXT    NOT NULL,
		owner      TEXT    NOT NULL,
		visibility TEXT    NOT NULL,
		is_default INTEGER NOT NULL DEFAULT 0,
		sort_order INTEGER NOT NULL DEFAULT 0,
		version    INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		payload    TEXT    NOT NULL,
		must_be_set TEXT   NOT NULL
	)`)
	require.NoError(t, err)

	err = migrateDashboardEntities(ctx, db)
	require.Error(t, err, "이관은 실패해야 한다")

	assert.EqualValues(t, 0, countRows(t, db, "dashboards"), "부분 삽입이 남으면 안 된다")
	assert.EqualValues(t, 2, countRows(t, db, dashboardSnapshotsV1Table), "원본은 온전해야 한다")
	assert.EqualValues(t, 0, countRows(t, db, "dashboard_user_state"), "UI 상태도 롤백되어야 한다")
	assert.EqualValues(t, 0, markerCount(t, db), "마커가 없어야 다음 기동에서 재시도된다")

	// 원인을 제거하면 다음 기동에서 정상 이관된다.
	_, err = db.ExecContext(ctx, `DROP TABLE dashboards`)
	require.NoError(t, err)
	require.NoError(t, migrateDashboardEntities(ctx, db), "재시도는 성공해야 한다")
	assert.EqualValues(t, 3, countRows(t, db, "dashboards"), "재시도 후 3장 (2 + 1)")
	assert.EqualValues(t, 1, markerCount(t, db))
}

// TestMigrateDashboardEntities_InvalidPayloadAborts 는 해석 불가능한 payload 를
// 만나면 조용히 건너뛰지 않고 이관 전체를 중단함을 검증한다.
//
// 건너뛰면 그 사용자의 대시보드가 사라진 채 마커가 기록되어 영영 복구되지 않는다.
func TestMigrateDashboardEntities_InvalidPayloadAborts(t *testing.T) {
	ctx := context.Background()
	db, _ := newLegacyDB(t)
	insertUser(t, db, "root", "admin")
	insertLegacySnapshot(t, db, "dashboards", "user", "alice", 2000, `{"dashboardPages": NOT-JSON`)

	err := migrateDashboardEntities(ctx, db)
	require.Error(t, err, "손상된 payload 는 조용히 무시되면 안 된다")
	assert.EqualValues(t, 0, markerCount(t, db), "마커가 없어야 재시도 가능하다")
	assert.EqualValues(t, 1, countRows(t, db, dashboardSnapshotsV1Table), "원본은 온전하다")
}

// ---------------------------------------------------------------------------
// 사전 백업 (VACUUM INTO)
// ---------------------------------------------------------------------------

// TestMigrateDashboardEntities_CreatesBackup 는 이관 전에 VACUUM INTO 백업 파일이
// 생성되고, 그 백업이 구 스냅샷을 담고 있음을 검증한다.
func TestMigrateDashboardEntities_CreatesBackup(t *testing.T) {
	ctx := context.Background()
	db, dbPath := newLegacyDB(t)
	seedAC01Fixture(t, db)

	backupPath := dbPath + dashboardBackupSuffix
	_, err := os.Stat(backupPath)
	require.ErrorIs(t, err, os.ErrNotExist, "이관 전에는 백업이 없다")

	require.NoError(t, migrateDashboardEntities(ctx, db))

	info, err := os.Stat(backupPath)
	require.NoError(t, err, "백업 파일이 생성되어야 한다")
	require.True(t, info.Mode().IsRegular())
	assert.Greater(t, info.Size(), int64(0), "백업이 비어 있으면 안 된다")

	// 백업은 이관 이전 상태(구 스키마 dashboards 3행)를 담는다.
	backup, err := sql.Open("sqlite", backupPath)
	require.NoError(t, err)
	defer backup.Close()

	legacy, err := hasLegacyDashboardSchema(ctx, backup)
	require.NoError(t, err)
	assert.True(t, legacy, "백업의 dashboards 는 이관 이전 구 스키마여야 한다")
	assert.EqualValues(t, 3, countRows(t, backup, "dashboards"), "백업에 구 스냅샷 3행이 있어야 한다")
}

// TestMigrateDashboardEntities_DoesNotOverwriteExistingBackup 는 이미 백업 파일이
// 있으면 덮어쓰지 않음을 검증한다 — 이전 시도의 백업이 더 원본에 가깝다.
func TestMigrateDashboardEntities_DoesNotOverwriteExistingBackup(t *testing.T) {
	ctx := context.Background()
	db, dbPath := newLegacyDB(t)
	seedAC01Fixture(t, db)

	backupPath := dbPath + dashboardBackupSuffix
	sentinel := []byte("이전 시도의 백업 — 덮어쓰면 안 된다")
	require.NoError(t, os.WriteFile(backupPath, sentinel, 0o600))

	require.NoError(t, migrateDashboardEntities(ctx, db), "기존 백업이 있어도 이관은 진행된다")

	got, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	assert.Equal(t, sentinel, got, "기존 백업이 그대로 남아 있어야 한다")
	assert.EqualValues(t, 7, countRows(t, db, "dashboards"), "이관 자체는 정상 수행된다")
}

// TestMigrateDashboardEntities_BackupFailureAborts 는 백업을 만들 수 없으면
// 이관을 수행하지 않음을 검증한다 — 복구 수단 없이 롤백 불가 변경을 실행하지 않는다.
//
// 실패 주입: 백업 경로에 디렉토리를 만들어 파일을 쓸 수 없게 한다.
func TestMigrateDashboardEntities_BackupFailureAborts(t *testing.T) {
	ctx := context.Background()
	db, dbPath := newLegacyDB(t)
	seedAC01Fixture(t, db)

	require.NoError(t, os.Mkdir(dbPath+dashboardBackupSuffix, 0o755))

	err := migrateDashboardEntities(ctx, db)
	require.Error(t, err, "백업 실패 시 이관을 중단해야 한다")
	assert.Contains(t, err.Error(), "backup")

	// 백업이 개명보다 앞서므로, 실패 시 DB 는 손대지 않은 이관 이전 상태 그대로다.
	legacy, lerr := hasLegacyDashboardSchema(ctx, db)
	require.NoError(t, lerr)
	assert.True(t, legacy, "구 스키마가 그대로 남아 있어야 한다")
	assert.EqualValues(t, 3, countRows(t, db, "dashboards"), "구 스냅샷 3행이 온전하다")

	archived, aerr := tableExists(ctx, db, dashboardSnapshotsV1Table)
	require.NoError(t, aerr)
	assert.False(t, archived, "개명이 일어나지 않아야 한다")
	assert.EqualValues(t, 0, markerCount(t, db), "마커가 없어야 다음 기동에서 재시도된다")
}

// ---------------------------------------------------------------------------
// 보존 원본 보호 / 부팅 경로 배선
// ---------------------------------------------------------------------------

// TestMigrateDashboardEntities_RefusesWhenBothTablesExist 는 구 dashboards 와
// 보존 원본이 동시에 존재하는 비정상 상태에서 개명을 강행하지 않음을 검증한다.
func TestMigrateDashboardEntities_RefusesWhenBothTablesExist(t *testing.T) {
	ctx := context.Background()
	db, _ := newLegacyDB(t)
	insertLegacySnapshot(t, db, "dashboards", "global", "", 1000, legacyPayload("g1", "", "g1"))
	require.NoError(t, seedLegacyDashboardTable(ctx, db, dashboardSnapshotsV1Table))
	insertLegacySnapshot(t, db, dashboardSnapshotsV1Table, "global", "", 500, legacyPayload("old", "", "old"))

	err := migrateDashboardEntities(ctx, db)
	require.Error(t, err, "어느 쪽이 원본인지 판단할 수 없으므로 멈춰야 한다")

	assert.EqualValues(t, 1, countRows(t, db, "dashboards"), "구 테이블은 그대로")
	assert.EqualValues(t, 1, countRows(t, db, dashboardSnapshotsV1Table), "보존 원본도 그대로")
}

// TestOpenSQLiteDB_MigratesDashboardEntities 는 부팅 경로(OpenSQLiteDB)가 이관을
// 수행하고, 재기동해도 멱등함을 검증한다.
func TestOpenSQLiteDB_MigratesDashboardEntities(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "boot.db")

	// 1) 구 스키마 DB 를 만든다.
	seed, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	require.NoError(t, migrateUsersSchema(ctx, seed))
	require.NoError(t, seedLegacyDashboardTable(ctx, seed, dashboardsTable))
	insertUser(t, seed, "root", "admin")
	insertLegacySnapshot(t, seed, "dashboards", "global", "", 1000, legacyPayload("g1", "", "g1", "g2"))
	insertLegacySnapshot(t, seed, "dashboards", "user", "alice", 2000, legacyPayload("a1", "", "a1"))
	require.NoError(t, seed.Close())

	// 2) 서버 기동 경로.
	db, err := OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err, "구 스키마 DB 에서 부팅이 성공해야 한다")
	t.Cleanup(func() { db.Close() })

	assert.EqualValues(t, 3, countRows(t, db, "dashboards"), "2 + 1 = 3장")
	assert.EqualValues(t, 2, countRows(t, db, dashboardSnapshotsV1Table), "원본 2행 보존")
	assert.EqualValues(t, 1, markerCount(t, db))

	// 3) 재기동 2회 — 멱등.
	for i := 1; i <= 2; i++ {
		reopened, err := OpenSQLiteDB(ctx, dbPath)
		require.NoErrorf(t, err, "%d회차 재기동 실패", i)
		assert.EqualValuesf(t, 3, countRows(t, reopened, "dashboards"), "%d회차 재기동 후 장수 불변", i)
		assert.EqualValuesf(t, 1, markerCount(t, reopened), "%d회차 재기동 후 마커 1행", i)
		require.NoError(t, reopened.Close())
	}

	// 4) 보존 원본은 어떤 부팅 경로에서도 변하지 않는다.
	//
	// @SPEC:SPEC-DASHBOARD-004 (M4) — 레거시 스냅샷 저장소가 제거되었으므로 이제
	// 보존 원본에 쓰는 코드 경로 자체가 없다. 그래도 부팅을 반복해도 행 수가 그대로
	// 인지 고정한다 — 보존 원본은 이관이 잘못되었을 때의 유일한 복구 원본이다.
	reopened, err := OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { reopened.Close() })
	assert.EqualValues(t, 2, countRows(t, reopened, dashboardSnapshotsV1Table),
		"보존 원본(dashboard_snapshots_v1)은 변경되면 안 된다")

	entityRepo, err := NewDashboardEntitySQLiteRepository(ctx, reopened)
	require.NoError(t, err, "신규 저장소 생성이 부팅을 막으면 안 된다")
	items, err := entityRepo.List(ctx, false)
	require.NoError(t, err)
	assert.Len(t, items, 3, "이관된 대시보드는 신규 저장소로 조회된다")
}

// ---------------------------------------------------------------------------
// 순수 헬퍼 단위 테스트
// ---------------------------------------------------------------------------

// TestAllocateDashboardUID 는 uid 확정 규칙을 검증한다.
// 롤백 불가 지점의 충돌 처리이므로 분기 전부를 직접 확인한다.
func TestAllocateDashboardUID(t *testing.T) {
	t.Run("빈 집합에서는 원본 id 를 그대로 쓴다", func(t *testing.T) {
		used := map[string]struct{}{}
		assert.Equal(t, "d1", allocateDashboardUID("d1", "", used))
		assert.Contains(t, used, "d1", "확정한 uid 는 선점 집합에 등록된다")
	})

	t.Run("충돌 시 접미사를 붙인다", func(t *testing.T) {
		used := map[string]struct{}{"d1": {}}
		assert.Equal(t, "d1-alice", allocateDashboardUID("d1", "alice", used))
	})

	t.Run("접미사까지 충돌하면 일련번호를 붙인다", func(t *testing.T) {
		used := map[string]struct{}{"d1": {}, "d1-alice": {}}
		assert.Equal(t, "d1-alice-2", allocateDashboardUID("d1", "alice", used))
		assert.Equal(t, "d1-alice-3", allocateDashboardUID("d1", "alice", used))
	})

	t.Run("접미사가 없으면 일련번호만 붙인다", func(t *testing.T) {
		used := map[string]struct{}{"d1": {}}
		assert.Equal(t, "d1-2", allocateDashboardUID("d1", "", used))
	})

	t.Run("원본 id 가 비어 있으면 대체 기준을 쓴다", func(t *testing.T) {
		used := map[string]struct{}{}
		assert.Equal(t, "dashboard", allocateDashboardUID("", "", used),
			"빈 uid 는 라우팅이 불가능하므로 발급하지 않는다")
	})

	t.Run("예약어는 선점되어 있어 회피된다", func(t *testing.T) {
		used := map[string]struct{}{}
		for _, r := range dashboardReservedUIDs {
			used[r] = struct{}{}
		}
		assert.Equal(t, "shared-bob", allocateDashboardUID("shared", "bob", used))
	})
}

// TestBuildEntityPayload_OmitsAbsentGridSettings 는 원본에 없던 그리드 설정을
// 이관이 지어내지 않음을 검증한다.
func TestBuildEntityPayload_OmitsAbsentGridSettings(t *testing.T) {
	page := legacyDashboardPage{ID: "d1"}
	src := legacyDashboardSnapshotPayload{
		GridCols:      json.RawMessage(`null`),
		ShowGridLines: nil,
	}

	raw, err := buildEntityPayload(page, src)
	require.NoError(t, err)

	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &got))

	assert.JSONEq(t, `[]`, string(got["panels"]), "panels 가 없으면 빈 배열로 정규화된다")
	assert.JSONEq(t, `[]`, string(got["layout"]), "layout 이 없으면 빈 배열로 정규화된다")
	assert.NotContains(t, got, "gridCols", "JSON null 은 키를 생략한다")
	assert.NotContains(t, got, "showGridLines", "값이 없으면 키를 생략한다")
	assert.NotContains(t, got, "refreshInterval", "값이 없으면 키를 생략한다")
}

// TestEmptyObjectIfNull 는 device_grid_layout 기본값 정규화를 검증한다.
func TestEmptyObjectIfNull(t *testing.T) {
	assert.JSONEq(t, `{}`, string(emptyObjectIfNull(nil)))
	assert.JSONEq(t, `{}`, string(emptyObjectIfNull(json.RawMessage(`null`))))
	assert.JSONEq(t, `{"a":1}`, string(emptyObjectIfNull(json.RawMessage(`{"a":1}`))))
}
