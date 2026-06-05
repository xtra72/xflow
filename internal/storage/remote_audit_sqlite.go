// remote_audit_sqlite.go 는 RemoteAuditRepository 의 SQLite 구현이다
// (@SPEC:SPEC-REMOTE-001 M6, spec §4.6 REQ-F05/F06).
//
// ManagedNodeSQLiteRepository/MirrorSQLiteRepository 패턴을 준용한다: WAL 모드,
// IF NOT EXISTS 멱등 마이그레이션, 기존 xflow.db 에 remote_audit 테이블을 멱등
// 추가. append-only 이므로 update/delete SQL 은 제공하지 않는다(감사 무결성).
//
// 인덱스: (instance_id, ts) 복합 인덱스로 노드별 최신순 조회를 가속한다.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // Pure Go SQLite 드라이버 등록
)

// 컴파일 타임 인터페이스 구현 검증.
var _ RemoteAuditRepository = (*RemoteAuditSQLiteRepository)(nil)

// sqliteDSN 은 modernc.org/sqlite DSN 에 WAL + busy_timeout pragma 를 부착한다
// (@SPEC:SPEC-REMOTE-001 M6, REQ-N01). DSN 방식은 풀의 모든 연결에 pragma 가
// 적용되므로, 다수 노드 동시 쓰기 경합 시 SQLITE_BUSY 대신 대기·재시도하게 한다
// (ExecContext PRAGMA 는 한 연결에만 적용되어 경합 시 재발).
//
// 입력 경로는 file: URI 로 감싸 쿼리 파라미터를 안전하게 부착한다.
func sqliteDSN(dbPath string) string {
	return "file:" + dbPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
}

// RemoteAuditSQLiteRepository 는 SQLite 기반 RemoteAuditRepository 구현이다.
type RemoteAuditSQLiteRepository struct {
	db *sql.DB
}

// NewRemoteAuditSQLiteRepository 는 SQLite DB 를 열고 remote_audit 테이블을 멱등하게
// 생성한 후 저장소를 반환한다. WAL 모드를 활성화한다.
func NewRemoteAuditSQLiteRepository(ctx context.Context, dbPath string) (*RemoteAuditSQLiteRepository, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	// DSN 에 WAL + busy_timeout pragma 를 실어 모든 풀 연결에 적용한다(다중 노드 동시
	// 감사 기록 경합 흡수 — @SPEC:SPEC-REMOTE-001 M6, REQ-N01).
	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := migrateRemoteAuditSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return &RemoteAuditSQLiteRepository{db: db}, nil
}

// migrateRemoteAuditSchema 는 remote_audit 테이블과 (instance_id, ts) 인덱스를
// 멱등하게 생성한다(spec §4.6).
//
// 컬럼은 비밀이 아닌 메타데이터만 보유한다(REQ-F06): actor/action/domain/
// command_action/result/reason/ts. 명령 인자·토큰·페이로드 컬럼은 의도적으로 없다.
func migrateRemoteAuditSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS remote_audit (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		instance_id    TEXT    NOT NULL DEFAULT '',
		actor          TEXT    NOT NULL DEFAULT '',
		action         TEXT    NOT NULL DEFAULT '',
		domain         TEXT    NOT NULL DEFAULT '',
		command_action TEXT    NOT NULL DEFAULT '',
		result         TEXT    NOT NULL DEFAULT '',
		reason         TEXT    NOT NULL DEFAULT '',
		ts             INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		return fmt.Errorf("create remote_audit table: %w", err)
	}
	if _, err := db.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_remote_audit_instance_ts ON remote_audit (instance_id, ts)`); err != nil {
		return fmt.Errorf("create remote_audit index: %w", err)
	}
	return nil
}

// Append 는 감사 레코드를 추가한다(추가 전용 — REQ-F05).
func (r *RemoteAuditSQLiteRepository) Append(ctx context.Context, rec RemoteAuditRecord) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO remote_audit (instance_id, actor, action, domain, command_action, result, reason, ts)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, rec.InstanceID, rec.Actor, rec.Action, rec.Domain, rec.CommandAction, rec.Result, rec.Reason, rec.Timestamp)
	if err != nil {
		return fmt.Errorf("append remote audit: %w", err)
	}
	return nil
}

// List 는 감사 레코드를 최신순(ts 내림차순, 동률은 id 내림차순)으로 반환한다.
// instanceID 가 비어 있지 않으면 해당 노드로 필터한다. limit<=0 이면 100 으로 보정한다.
func (r *RemoteAuditSQLiteRepository) List(ctx context.Context, instanceID string, limit, offset int) ([]RemoteAuditRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var (
		rows *sql.Rows
		err  error
	)
	const cols = `id, instance_id, actor, action, domain, command_action, result, reason, ts`
	if instanceID != "" {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+cols+` FROM remote_audit WHERE instance_id = ?
				ORDER BY ts DESC, id DESC LIMIT ? OFFSET ?`, instanceID, limit, offset)
	} else {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+cols+` FROM remote_audit
				ORDER BY ts DESC, id DESC LIMIT ? OFFSET ?`, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("list remote audit: %w", err)
	}
	defer rows.Close()

	var out []RemoteAuditRecord
	for rows.Next() {
		var rec RemoteAuditRecord
		if err := rows.Scan(&rec.ID, &rec.InstanceID, &rec.Actor, &rec.Action,
			&rec.Domain, &rec.CommandAction, &rec.Result, &rec.Reason, &rec.Timestamp); err != nil {
			return nil, fmt.Errorf("scan remote audit row: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate remote audit rows: %w", err)
	}
	return out, nil
}

// Close 는 데이터베이스 연결을 닫는다.
func (r *RemoteAuditSQLiteRepository) Close() error {
	return r.db.Close()
}
