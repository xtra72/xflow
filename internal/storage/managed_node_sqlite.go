// managed_node_sqlite.go 는 ManagedNodeRepository 의 SQLite 구현이다
// (@SPEC:SPEC-REMOTE-001 M2, spec §5.4 managed_nodes).
//
// FlowRepository/SQLiteRepository 패턴을 준용한다: WAL 모드, IF NOT EXISTS 멱등
// 마이그레이션. 기존 xflow.db 에 managed_nodes 테이블을 멱등하게 추가하므로
// 기존 DB(다른 테이블 보유)에서도 안전하다.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite 드라이버 등록
)

// 컴파일 타임 인터페이스 구현 검증.
var _ ManagedNodeRepository = (*ManagedNodeSQLiteRepository)(nil)

// ManagedNodeSQLiteRepository 는 SQLite 기반 ManagedNodeRepository 구현이다.
type ManagedNodeSQLiteRepository struct {
	db *sql.DB
}

// NewManagedNodeSQLiteRepository 는 SQLite DB 를 열고 managed_nodes 테이블을
// 멱등하게 생성한 후 저장소를 반환한다.
//
// dbPath 의 부모 디렉토리가 없으면 자동 생성한다. WAL 모드를 활성화한다.
func NewManagedNodeSQLiteRepository(ctx context.Context, dbPath string) (*ManagedNodeSQLiteRepository, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	// DSN 에 WAL + busy_timeout pragma 를 실어 모든 풀 연결에 적용한다(다수 노드
	// online/last_seen 갱신 경합 흡수 — @SPEC:SPEC-REMOTE-001 M6, REQ-N01).
	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := migrateManagedNodesSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return &ManagedNodeSQLiteRepository{db: db}, nil
}

// migrateManagedNodesSchema 는 managed_nodes 테이블을 멱등하게 생성하고, v1.4(M9)
// 그룹·BASIC 시스템 정보 컬럼을 멱등하게 추가한다(spec §5.4, REQ-K01/K08).
//
// status 는 CHECK 제약으로 상태 머신 값만 허용한다(pending/approved/rejected/revoked).
// 시각 컬럼은 epoch milliseconds(int64) 이다.
//
// 마이그레이션 안전성(REQ-K09 하위 호환): 기존 DB(group_name/os/arch/started_at 컬럼이
// 없는 v1.3 이하)에서도 ALTER TABLE ADD COLUMN IF NOT EXISTS 로 신규 컬럼을 멱등하게
// 추가한다. 기본값은 빈 문자열/0 이므로 기존 행은 회귀 없이 동작한다.
func migrateManagedNodesSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS managed_nodes (
		instance_id TEXT    PRIMARY KEY,
		hostname    TEXT    NOT NULL DEFAULT '',
		version     TEXT    NOT NULL DEFAULT '',
		status      TEXT    NOT NULL DEFAULT 'pending'
		            CHECK (status IN ('pending', 'approved', 'rejected', 'revoked')),
		token_id    TEXT    NOT NULL DEFAULT '',
		last_seen   INTEGER NOT NULL DEFAULT 0,
		online      INTEGER NOT NULL DEFAULT 0,
		created_at  INTEGER NOT NULL,
		updated_at  INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create managed_nodes table: %w", err)
	}

	// v1.4(M9) 그룹·시스템 정보 컬럼을 멱등하게 추가한다(REQ-K01/K08/K09). modernc.org/
	// sqlite 는 ADD COLUMN IF NOT EXISTS 를 지원하지 않으므로, 기존 컬럼 집합을 조회해
	// 부재한 컬럼만 ALTER TABLE ADD COLUMN 한다(기존 DB 안전 — 중복 추가 방지).
	if err := addManagedNodeColumns(ctx, db); err != nil {
		return err
	}
	return nil
}

// addManagedNodeColumns 는 managed_nodes 에 부재한 v1.4 컬럼을 멱등하게 추가한다.
//
// PRAGMA table_info 로 현재 컬럼 집합을 읽고, 없는 컬럼만 ALTER TABLE ADD COLUMN 한다.
// 이미 모든 컬럼이 있으면 no-op 이므로 기존 DB·신규 DB 모두에서 안전하다(REQ-K09).
func addManagedNodeColumns(ctx context.Context, db *sql.DB) error {
	existing, err := managedNodeColumns(ctx, db)
	if err != nil {
		return err
	}
	// 컬럼명 → DDL 정의(추가 시 사용). 모두 NOT NULL DEFAULT 로 기존 행 안전.
	additions := []struct{ name, ddl string }{
		{"group_name", "group_name TEXT NOT NULL DEFAULT ''"},
		{"os", "os TEXT NOT NULL DEFAULT ''"},
		{"arch", "arch TEXT NOT NULL DEFAULT ''"},
		{"started_at", "started_at INTEGER NOT NULL DEFAULT 0"},
		// v1.6(M11, 그룹 M) 노드 해상도 컬럼(REQ-M01/M02). 기존 행은 0(미보고 — REQ-M03).
		{"display_width", "display_width INTEGER NOT NULL DEFAULT 0"},
		{"display_height", "display_height INTEGER NOT NULL DEFAULT 0"},
		// v1.6(M11 확장) 노드 해상도 서버-측 오버라이드 컬럼(관리자 전용). 기존 행은 0
		// (오버라이드 없음 → effective=노드 보고값). group_name 과 동일하게 admin-owned.
		{"display_override_width", "display_override_width INTEGER NOT NULL DEFAULT 0"},
		{"display_override_height", "display_override_height INTEGER NOT NULL DEFAULT 0"},
	}
	for _, col := range additions {
		if _, ok := existing[col.name]; ok {
			continue // 이미 존재 — 멱등.
		}
		stmt := fmt.Sprintf(`ALTER TABLE managed_nodes ADD COLUMN %s`, col.ddl)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("add managed_nodes column %s: %w", col.name, err)
		}
	}
	return nil
}

// managedNodeColumns 는 managed_nodes 의 현재 컬럼명 집합을 반환한다(PRAGMA table_info).
func managedNodeColumns(ctx context.Context, db *sql.DB) (map[string]struct{}, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(managed_nodes)`)
	if err != nil {
		return nil, fmt.Errorf("read managed_nodes columns: %w", err)
	}
	defer rows.Close()

	cols := make(map[string]struct{})
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notNull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			return nil, fmt.Errorf("scan managed_nodes column: %w", err)
		}
		cols[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate managed_nodes columns: %w", err)
	}
	return cols, nil
}

// Upsert 는 노드를 저장한다. 동일 instance_id 가 있으면 등록/상태/생존 필드를 갱신
// 하되 created_at 은 보존한다(REQ-E06 last-known).
//
// v1.4(M9) 관리자/시스템 소유 필드 보존(REQ-K02/K08): group_name(관리자 전용)·os·arch·
// started_at(시스템 정보)은 ON CONFLICT 갱신 집합에서 제외되어 register/heartbeat upsert
// 가 이를 덮어쓰지 않는다. 즉 register upsert 는 관리자가 배정한 group_name 을 절대
// clobber 하지 않으며(REQ-K02), 시스템 정보는 SetSystemInfo 가 제공된 필드만 갱신한다
// (REQ-K08). 신규 INSERT 시에는 struct 의 group_name/os/arch/started_at 값을 그대로
// 기록한다(첫 register 가 시스템 정보를 운반하는 경우 반영).
//
// v1.6(M11 확장) 해상도 오버라이드 보존: display_override_width/display_override_height
// (관리자 전용)도 ON CONFLICT 갱신 집합에서 제외되어 register/heartbeat upsert 가 이를
// clobber 하지 않는다(group_name 패턴 일관). 오버라이드는 SetNodeDisplayOverride 로만
// 변경한다. 신규 INSERT 시에는 struct 의 오버라이드 값을 그대로 기록한다.
func (r *ManagedNodeSQLiteRepository) Upsert(ctx context.Context, node ManagedNode) error {
	now := time.Now().UnixMilli()
	createdAt := node.CreatedAt
	if createdAt == 0 {
		createdAt = now
	}
	status := node.Status
	if status == "" {
		status = "pending"
	}
	online := 0
	if node.Online {
		online = 1
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO managed_nodes (instance_id, hostname, version, status, token_id, last_seen, online, group_name, os, arch, started_at, display_width, display_height, display_override_width, display_override_height, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(instance_id) DO UPDATE SET
			hostname   = excluded.hostname,
			version    = excluded.version,
			status     = excluded.status,
			token_id   = excluded.token_id,
			last_seen  = excluded.last_seen,
			online     = excluded.online,
			updated_at = excluded.updated_at
	`, node.InstanceID, node.Hostname, node.Version, status, node.TokenID,
		node.LastSeen, online, node.GroupName, node.OS, node.Arch, node.StartedAt,
		node.DisplayWidth, node.DisplayHeight, node.DisplayOverrideWidth, node.DisplayOverrideHeight,
		createdAt, now)
	if err != nil {
		return fmt.Errorf("upsert managed node: %w", err)
	}
	return nil
}

// Get 은 instance_id 로 노드를 조회한다. 없으면 ErrManagedNodeNotFound.
func (r *ManagedNodeSQLiteRepository) Get(ctx context.Context, instanceID string) (ManagedNode, error) {
	var (
		node   ManagedNode
		online int
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT instance_id, hostname, version, status, token_id, last_seen, online, group_name, os, arch, started_at, display_width, display_height, display_override_width, display_override_height, created_at, updated_at
		FROM managed_nodes WHERE instance_id = ?
	`, instanceID).Scan(&node.InstanceID, &node.Hostname, &node.Version, &node.Status,
		&node.TokenID, &node.LastSeen, &online, &node.GroupName, &node.OS, &node.Arch,
		&node.StartedAt, &node.DisplayWidth, &node.DisplayHeight,
		&node.DisplayOverrideWidth, &node.DisplayOverrideHeight, &node.CreatedAt, &node.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ManagedNode{}, ErrManagedNodeNotFound
	}
	if err != nil {
		return ManagedNode{}, fmt.Errorf("get managed node: %w", err)
	}
	node.Online = online != 0
	return node, nil
}

// List 는 모든 관리 노드를 created_at 순서로 반환한다.
func (r *ManagedNodeSQLiteRepository) List(ctx context.Context) ([]ManagedNode, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT instance_id, hostname, version, status, token_id, last_seen, online, group_name, os, arch, started_at, display_width, display_height, display_override_width, display_override_height, created_at, updated_at
		FROM managed_nodes ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("list managed nodes: %w", err)
	}
	defer rows.Close()

	var out []ManagedNode
	for rows.Next() {
		var (
			node   ManagedNode
			online int
		)
		if err := rows.Scan(&node.InstanceID, &node.Hostname, &node.Version, &node.Status,
			&node.TokenID, &node.LastSeen, &online, &node.GroupName, &node.OS, &node.Arch,
			&node.StartedAt, &node.DisplayWidth, &node.DisplayHeight,
			&node.DisplayOverrideWidth, &node.DisplayOverrideHeight, &node.CreatedAt, &node.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan managed node row: %w", err)
		}
		node.Online = online != 0
		out = append(out, node)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate managed node rows: %w", err)
	}
	return out, nil
}

// UpdateStatus 는 등록 상태를 전이한다. 없으면 ErrManagedNodeNotFound.
func (r *ManagedNodeSQLiteRepository) UpdateStatus(ctx context.Context, instanceID, status string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE managed_nodes SET status = ?, updated_at = ? WHERE instance_id = ?
	`, status, time.Now().UnixMilli(), instanceID)
	return checkAffected(res, err, "update managed node status")
}

// SetToken 은 노드 토큰 식별자를 저장한다. 없으면 ErrManagedNodeNotFound.
func (r *ManagedNodeSQLiteRepository) SetToken(ctx context.Context, instanceID, tokenID string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE managed_nodes SET token_id = ?, updated_at = ? WHERE instance_id = ?
	`, tokenID, time.Now().UnixMilli(), instanceID)
	return checkAffected(res, err, "set managed node token")
}

// SetOnline 은 online 상태와 last_seen 을 갱신한다(행 삭제 없음 — REQ-E06).
func (r *ManagedNodeSQLiteRepository) SetOnline(ctx context.Context, instanceID string, online bool, lastSeenMs int64) error {
	onlineInt := 0
	if online {
		onlineInt = 1
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE managed_nodes SET online = ?, last_seen = ?, updated_at = ? WHERE instance_id = ?
	`, onlineInt, lastSeenMs, time.Now().UnixMilli(), instanceID)
	return checkAffected(res, err, "set managed node online")
}

// SetNodeGroup 은 노드의 단일 그룹 라벨을 배정/변경/해제한다(REQ-K02). groupName 이
// 빈 문자열이면 그룹을 해제하여 "전체" 버킷으로 환원한다(REQ-K05). 그룹은 서버 운영
// 메타데이터이므로 노드로 명령을 전파하지 않는다(A13 — DB 갱신만). 없으면 ErrManagedNodeNotFound.
func (r *ManagedNodeSQLiteRepository) SetNodeGroup(ctx context.Context, instanceID, groupName string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE managed_nodes SET group_name = ?, updated_at = ? WHERE instance_id = ?
	`, groupName, time.Now().UnixMilli(), instanceID)
	return checkAffected(res, err, "set managed node group")
}

// RenameGroup 은 oldName 그룹의 모든 노드 group_name 을 newName 으로 일괄 변경한다.
// 영향받은 노드 수를 반환한다(0 이면 해당 그룹 없음). 노드 보고가 group_name 을 덮어쓰지
// 않으므로(Upsert ON CONFLICT 제외) 관리자 변경이 안정적으로 유지된다.
func (r *ManagedNodeSQLiteRepository) RenameGroup(ctx context.Context, oldName, newName string) (int, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE managed_nodes SET group_name = ?, updated_at = ? WHERE group_name = ?
	`, newName, time.Now().UnixMilli(), oldName)
	if err != nil {
		return 0, fmt.Errorf("rename node group: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// DeleteGroup 은 groupName 그룹의 모든 노드를 "전체" 버킷(group_name="")으로 이동한다.
// 영향받은 노드 수를 반환한다(0 이면 해당 그룹 없음). 노드 행은 삭제하지 않는다(REQ-K05).
func (r *ManagedNodeSQLiteRepository) DeleteGroup(ctx context.Context, groupName string) (int, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE managed_nodes SET group_name = '', updated_at = ? WHERE group_name = ?
	`, time.Now().UnixMilli(), groupName)
	if err != nil {
		return 0, fmt.Errorf("delete node group: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ListGroups 는 현재 사용 중인 distinct 그룹 라벨과 노드 수를 반환한다(REQ-K03).
//
// 응답은 항상 가상 "전체" 버킷(GroupName="")의 노드 수를 먼저 포함하고(그룹 미지정
// 노드), 그 다음 비어 있지 않은 distinct 그룹을 그룹명 오름차순으로 나열한다. 빈 그룹
// (구성원 0)은 GROUP BY 결과에 나타나지 않아 자동으로 사라진다(REQ-K05).
func (r *ManagedNodeSQLiteRepository) ListGroups(ctx context.Context) ([]NodeGroupCount, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT group_name, COUNT(*) AS node_count
		FROM managed_nodes
		GROUP BY group_name
	`)
	if err != nil {
		return nil, fmt.Errorf("list node groups: %w", err)
	}
	defer rows.Close()

	ungrouped := 0
	named := make([]NodeGroupCount, 0)
	for rows.Next() {
		var gc NodeGroupCount
		if err := rows.Scan(&gc.GroupName, &gc.NodeCount); err != nil {
			return nil, fmt.Errorf("scan node group row: %w", err)
		}
		if gc.GroupName == "" {
			ungrouped = gc.NodeCount
			continue
		}
		named = append(named, gc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate node group rows: %w", err)
	}

	// 그룹명 오름차순 정렬(distinct 그룹). "전체" 버킷을 맨 앞에 둔다(REQ-K03 — 항상 포함).
	sort.Slice(named, func(i, j int) bool { return named[i].GroupName < named[j].GroupName })
	out := make([]NodeGroupCount, 0, len(named)+1)
	out = append(out, NodeGroupCount{GroupName: "", NodeCount: ungrouped})
	out = append(out, named...)
	return out, nil
}

// SetSystemInfo 는 노드가 보고한 BASIC 시스템 정보(os/arch/started_at) + 노드 해상도
// (display_width/display_height)를 저장한다(REQ-K08/M01/M02).
//
// 제공된 필드만 갱신하고 미제공(빈 문자열/0) 필드는 기존값을 보존한다(REQ-K09/M03 하위
// 호환 — heartbeat 가 일부만 보내거나 구버전 노드가 생략). COALESCE/NULLIF 로 제공 시에만
// 덮어쓴다. group_name 은 절대 건드리지 않는다(관리자 전용). 없으면 ErrManagedNodeNotFound.
func (r *ManagedNodeSQLiteRepository) SetSystemInfo(ctx context.Context, instanceID, osName, arch string, startedAtMs int64, displayWidth, displayHeight int) error {
	// NULLIF(?, '') 가 빈 문자열을 NULL 로 만들고, COALESCE 가 NULL 일 때 기존값을 유지한다.
	// started_at/display_* 는 0 을 미제공으로 간주한다(NULLIF(?, 0)) — preserve-on-omit.
	res, err := r.db.ExecContext(ctx, `
		UPDATE managed_nodes SET
			os             = COALESCE(NULLIF(?, ''), os),
			arch           = COALESCE(NULLIF(?, ''), arch),
			started_at     = COALESCE(NULLIF(?, 0), started_at),
			display_width  = COALESCE(NULLIF(?, 0), display_width),
			display_height = COALESCE(NULLIF(?, 0), display_height),
			updated_at     = ?
		WHERE instance_id = ?
	`, osName, arch, startedAtMs, displayWidth, displayHeight, time.Now().UnixMilli(), instanceID)
	return checkAffected(res, err, "set managed node system info")
}

// SetNodeDisplayOverride 는 관리자가 서버에서 노드 해상도를 강제하는 오버라이드를
// 설정/해제한다(v1.6 M11 확장, OQ-M1 보조 override). group_name 과 동일하게 관리자 전용
// 메타데이터이며 노드로 명령을 전파하지 않는다(A13 일관 — DB 갱신만).
//
// width<=0 또는 height<=0 이면 오버라이드를 0,0 으로 해제한다(effective 해상도가 노드
// 보고값으로 폴백). 둘 다 양수일 때만 그 값을 저장한다(부분 오버라이드는 의미가 없으므로
// 검증 단계에서 막거나 여기서 해제로 정규화). 노드 보고 해상도(display_width/height)·
// group_name 은 절대 건드리지 않는다. 없으면 ErrManagedNodeNotFound.
func (r *ManagedNodeSQLiteRepository) SetNodeDisplayOverride(ctx context.Context, instanceID string, width, height int) error {
	if width <= 0 || height <= 0 {
		// 비양수 → 해제(0,0). 부분값(한쪽만 양수)도 무효 오버라이드이므로 함께 해제한다.
		width, height = 0, 0
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE managed_nodes SET
			display_override_width  = ?,
			display_override_height = ?,
			updated_at              = ?
		WHERE instance_id = ?
	`, width, height, time.Now().UnixMilli(), instanceID)
	return checkAffected(res, err, "set managed node display override")
}

// Delete 는 instance_id 로 노드를 삭제한다. 없으면 ErrManagedNodeNotFound.
func (r *ManagedNodeSQLiteRepository) Delete(ctx context.Context, instanceID string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM managed_nodes WHERE instance_id = ?`, instanceID)
	return checkAffected(res, err, "delete managed node")
}

// Close 는 데이터베이스 연결을 닫는다.
func (r *ManagedNodeSQLiteRepository) Close() error {
	return r.db.Close()
}

// checkAffected 는 ExecContext 결과를 검사하여 영향받은 행이 없으면
// ErrManagedNodeNotFound 를 반환한다.
func checkAffected(res sql.Result, execErr error, op string) error {
	if execErr != nil {
		return fmt.Errorf("%s: %w", op, execErr)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s rows affected: %w", op, err)
	}
	if affected == 0 {
		return ErrManagedNodeNotFound
	}
	return nil
}
