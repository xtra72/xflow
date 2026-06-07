// mirror_sqlite.go 는 MirrorRepository 의 SQLite 구현이다
// (@SPEC:SPEC-REMOTE-001 M4, spec §5.4 mirrored_flows/agents/devices).
//
// SQLiteRepository/ManagedNodeSQLiteRepository 패턴을 준용한다: WAL 모드, IF NOT
// EXISTS 멱등 마이그레이션. 세 미러 테이블은 동일 스키마(id, source_instance_id,
// name, status, definition, updated_at)를 공유하며, source_instance_id 인덱스로
// 노드별/전체 조회를 가속한다.
//
// 출처 태깅(REQ-E04): 모든 행이 source_instance_id 를 보유한다. PK 는 (source_instance_id,
// id) 복합키로, 동일 노드 내 자원 ID 유일성을 보장하고 노드 간 ID 충돌을 허용한다.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite 드라이버 등록
)

// 컴파일 타임 인터페이스 구현 검증.
var _ MirrorRepository = (*MirrorSQLiteRepository)(nil)

// mirrorKindTable 은 kind → 테이블명 매핑이다.
var mirrorKindTable = map[string]string{
	"flow":   "mirrored_flows",
	"agent":  "mirrored_agents",
	"device": "mirrored_devices",
}

// MirrorSQLiteRepository 는 SQLite 기반 MirrorRepository 구현이다.
type MirrorSQLiteRepository struct {
	db *sql.DB
}

// NewMirrorSQLiteRepository 는 SQLite DB 를 열고 미러 테이블을 멱등하게 생성한 후
// 저장소를 반환한다.
func NewMirrorSQLiteRepository(ctx context.Context, dbPath string) (*MirrorSQLiteRepository, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	// DSN 에 pragma 를 실어 모든 풀 연결에 WAL + busy_timeout 이 적용되게 한다.
	// 다수 노드가 동시에 미러를 push 하면 쓰기가 경합하므로(SQLITE_BUSY), 즉시 실패
	// 대신 대기·재시도하게 한다(@SPEC:SPEC-REMOTE-001 M6, REQ-N01 — 다중 노드 확장성).
	// PRAGMA 를 ExecContext 로 한 번만 실행하면 풀의 한 연결에만 적용되어 경합 시
	// SQLITE_BUSY 가 재발하므로, DSN(_pragma) 방식으로 연결마다 적용한다.
	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := migrateMirrorSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return &MirrorSQLiteRepository{db: db}, nil
}

// migrateMirrorSchema 는 세 미러 테이블을 멱등하게 생성한다(spec §5.4).
func migrateMirrorSchema(ctx context.Context, db *sql.DB) error {
	for _, table := range []string{"mirrored_flows", "mirrored_agents", "mirrored_devices"} {
		stmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			source_instance_id TEXT    NOT NULL,
			id                 TEXT    NOT NULL,
			name               TEXT    NOT NULL DEFAULT '',
			status             TEXT    NOT NULL DEFAULT '',
			definition         TEXT    NOT NULL DEFAULT '',
			updated_at         INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (source_instance_id, id)
		)`, table)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create %s table: %w", table, err)
		}
		idx := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_source ON %s (source_instance_id)`, table, table)
		if _, err := db.ExecContext(ctx, idx); err != nil {
			return fmt.Errorf("create %s source index: %w", table, err)
		}
	}
	return nil
}

// tableFor 는 kind 에 대응하는 테이블명을 반환한다. 미지원 kind 는 에러.
func tableFor(kind string) (string, error) {
	table, ok := mirrorKindTable[kind]
	if !ok {
		return "", fmt.Errorf("mirror: unknown kind %q", kind)
	}
	return table, nil
}

// replaceKind 는 한 노드의 kind 미러 전체를 트랜잭션으로 교체한다(snapshot — REQ-E03).
func (r *MirrorSQLiteRepository) replaceKind(ctx context.Context, table, instanceID string, items []MirroredResource) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // commit 성공 시 no-op.

	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM %s WHERE source_instance_id = ?`, table), instanceID); err != nil {
		return fmt.Errorf("clear %s for node: %w", table, err)
	}

	insert := fmt.Sprintf(`INSERT INTO %s (source_instance_id, id, name, status, definition, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, table)
	for _, it := range items {
		updatedAt := it.UpdatedAt
		if updatedAt == 0 {
			updatedAt = time.Now().UnixMilli()
		}
		if _, err := tx.ExecContext(ctx, insert,
			instanceID, it.ID, it.Name, it.Status, it.Definition, updatedAt); err != nil {
			return fmt.Errorf("insert %s row: %w", table, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace tx: %w", err)
	}
	return nil
}

// ReplaceFlows 는 한 노드의 flow 미러 전체를 교체한다(REQ-E03).
func (r *MirrorSQLiteRepository) ReplaceFlows(ctx context.Context, instanceID string, items []MirroredResource) error {
	return r.replaceKind(ctx, "mirrored_flows", instanceID, items)
}

// ReplaceAgents 는 한 노드의 agent 미러 전체를 교체한다(REQ-E03).
func (r *MirrorSQLiteRepository) ReplaceAgents(ctx context.Context, instanceID string, items []MirroredResource) error {
	return r.replaceKind(ctx, "mirrored_agents", instanceID, items)
}

// ReplaceDevices 는 한 노드의 device 미러 전체를 교체한다(REQ-E03).
func (r *MirrorSQLiteRepository) ReplaceDevices(ctx context.Context, instanceID string, items []MirroredResource) error {
	return r.replaceKind(ctx, "mirrored_devices", instanceID, items)
}

// UpsertResource 는 단일 미러 행을 추가/갱신한다(delta add/update — REQ-E02/E03).
func (r *MirrorSQLiteRepository) UpsertResource(ctx context.Context, kind string, item MirroredResource) error {
	table, err := tableFor(kind)
	if err != nil {
		return err
	}
	updatedAt := item.UpdatedAt
	if updatedAt == 0 {
		updatedAt = time.Now().UnixMilli()
	}
	stmt := fmt.Sprintf(`INSERT INTO %s (source_instance_id, id, name, status, definition, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_instance_id, id) DO UPDATE SET
			name       = excluded.name,
			status     = excluded.status,
			definition = excluded.definition,
			updated_at = excluded.updated_at`, table)
	if _, err := r.db.ExecContext(ctx, stmt,
		item.SourceInstanceID, item.ID, item.Name, item.Status, item.Definition, updatedAt); err != nil {
		return fmt.Errorf("upsert %s row: %w", table, err)
	}
	return nil
}

// DeleteResource 는 노드+kind+id 로 단일 미러 행을 삭제한다(delta remove — REQ-E02).
// 행이 없어도 에러가 아니다(멱등 — 중복 remove 안전).
func (r *MirrorSQLiteRepository) DeleteResource(ctx context.Context, instanceID, kind, id string) error {
	table, err := tableFor(kind)
	if err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM %s WHERE source_instance_id = ? AND id = ?`, table),
		instanceID, id); err != nil {
		return fmt.Errorf("delete %s row: %w", table, err)
	}
	return nil
}

// listByNode 는 한 노드의 kind 미러를 id 순서로 반환한다.
func (r *MirrorSQLiteRepository) listByNode(ctx context.Context, table, kind, instanceID string) ([]MirroredResource, error) {
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT source_instance_id, id, name, status, definition, updated_at
			FROM %s WHERE source_instance_id = ? ORDER BY id`, table), instanceID)
	if err != nil {
		return nil, fmt.Errorf("list %s by node: %w", table, err)
	}
	defer rows.Close()
	return scanMirrorRows(rows, kind)
}

// listAll 은 전 노드의 kind 미러를 (source_instance_id, id) 순서로 반환한다.
func (r *MirrorSQLiteRepository) listAll(ctx context.Context, table, kind string) ([]MirroredResource, error) {
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT source_instance_id, id, name, status, definition, updated_at
			FROM %s ORDER BY source_instance_id, id`, table))
	if err != nil {
		return nil, fmt.Errorf("list all %s: %w", table, err)
	}
	defer rows.Close()
	return scanMirrorRows(rows, kind)
}

// scanMirrorRows 는 행 집합을 MirroredResource 슬라이스로 스캔한다(kind 주입).
func scanMirrorRows(rows *sql.Rows, kind string) ([]MirroredResource, error) {
	var out []MirroredResource
	for rows.Next() {
		var m MirroredResource
		if err := rows.Scan(&m.SourceInstanceID, &m.ID, &m.Name, &m.Status, &m.Definition, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan mirror row: %w", err)
		}
		m.Kind = kind
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mirror rows: %w", err)
	}
	return out, nil
}

// ListFlows 는 한 노드의 flow 미러를 반환한다(REQ-E05).
func (r *MirrorSQLiteRepository) ListFlows(ctx context.Context, instanceID string) ([]MirroredResource, error) {
	return r.listByNode(ctx, "mirrored_flows", "flow", instanceID)
}

// ListAgents 는 한 노드의 agent 미러를 반환한다(REQ-E05).
func (r *MirrorSQLiteRepository) ListAgents(ctx context.Context, instanceID string) ([]MirroredResource, error) {
	return r.listByNode(ctx, "mirrored_agents", "agent", instanceID)
}

// ListDevices 는 한 노드의 device 미러를 반환한다(REQ-E05).
func (r *MirrorSQLiteRepository) ListDevices(ctx context.Context, instanceID string) ([]MirroredResource, error) {
	return r.listByNode(ctx, "mirrored_devices", "device", instanceID)
}

// ListAllFlows 는 전 노드의 flow 미러를 출처 태그와 함께 반환한다(REQ-E05).
func (r *MirrorSQLiteRepository) ListAllFlows(ctx context.Context) ([]MirroredResource, error) {
	return r.listAll(ctx, "mirrored_flows", "flow")
}

// ListAllAgents 는 전 노드의 agent 미러를 반환한다(REQ-E05).
func (r *MirrorSQLiteRepository) ListAllAgents(ctx context.Context) ([]MirroredResource, error) {
	return r.listAll(ctx, "mirrored_agents", "agent")
}

// ListAllDevices 는 전 노드의 device 미러를 반환한다(REQ-E05).
func (r *MirrorSQLiteRepository) ListAllDevices(ctx context.Context) ([]MirroredResource, error) {
	return r.listAll(ctx, "mirrored_devices", "device")
}

// NodeSummary 는 한 노드의 운영 요약을 미러 데이터에서 파생해 반환한다(v1.4 M9,
// REQ-K10/A15). 세 미러 테이블의 status 별 카운트를 GROUP BY 로 집계한다. 미러 행은
// 노드 오프라인 시에도 보존되므로 last-known 요약을 제공한다(REQ-E06). 빈 노드는
// 모든 카운트가 0 인 요약을 반환한다(에러 아님).
func (r *MirrorSQLiteRepository) NodeSummary(ctx context.Context, instanceID string) (NodeOperationalSummary, error) {
	flowCounts, err := r.statusCounts(ctx, "mirrored_flows", instanceID)
	if err != nil {
		return NodeOperationalSummary{}, err
	}
	agentCounts, err := r.statusCounts(ctx, "mirrored_agents", instanceID)
	if err != nil {
		return NodeOperationalSummary{}, err
	}
	deviceCounts, err := r.statusCounts(ctx, "mirrored_devices", instanceID)
	if err != nil {
		return NodeOperationalSummary{}, err
	}

	return NodeOperationalSummary{
		Flows: FlowSummary{
			Total:   sumCounts(flowCounts),
			Running: flowCounts["running"],
			Stopped: flowCounts["stopped"],
		},
		Agents: AgentSummary{
			Total:     sumCounts(agentCounts),
			Connected: agentCounts["connected"],
		},
		Devices: DeviceSummary{
			Total:  sumCounts(deviceCounts),
			Online: deviceCounts["online"],
		},
	}, nil
}

// statusCounts 는 한 노드의 kind 테이블에서 status 별 행 수를 집계해 반환한다(REQ-K10).
func (r *MirrorSQLiteRepository) statusCounts(ctx context.Context, table, instanceID string) (map[string]int, error) {
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT status, COUNT(*) FROM %s WHERE source_instance_id = ? GROUP BY status`, table),
		instanceID)
	if err != nil {
		return nil, fmt.Errorf("status counts %s: %w", table, err)
	}
	defer rows.Close()

	out := make(map[string]int)
	for rows.Next() {
		var (
			status string
			count  int
		)
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scan status count %s: %w", table, err)
		}
		out[status] += count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate status counts %s: %w", table, err)
	}
	return out, nil
}

// sumCounts 는 status 카운트 맵의 총합(노드별 자원 총 수)을 반환한다.
func sumCounts(counts map[string]int) int {
	total := 0
	for _, c := range counts {
		total += c
	}
	return total
}

// DeleteByNode 는 한 노드의 모든 미러 행을 삭제한다(노드 삭제 시 orphan 정리). 멱등.
func (r *MirrorSQLiteRepository) DeleteByNode(ctx context.Context, instanceID string) error {
	for _, table := range []string{"mirrored_flows", "mirrored_agents", "mirrored_devices"} {
		if _, err := r.db.ExecContext(ctx,
			fmt.Sprintf(`DELETE FROM %s WHERE source_instance_id = ?`, table), instanceID); err != nil {
			return fmt.Errorf("delete %s by node: %w", table, err)
		}
	}
	return nil
}

// Close 는 데이터베이스 연결을 닫는다.
func (r *MirrorSQLiteRepository) Close() error {
	return r.db.Close()
}
