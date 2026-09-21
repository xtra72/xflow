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
	"strings"

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

// List 는 조건에 맞는 감사 레코드와 필터 적용 전체 건수를 반환한다
// (@SPEC:SPEC-REMOTE-LOG-001).
//
// 정렬·필터를 SQL 로 내리는 이유는 화면이 받아 온 쪽 안에서만 정렬하면 그 결과가
// 전체를 대표하지 않기 때문이다. 정렬 기준은 화이트리스트를 지나며, 그 밖의 값은
// 기본(ts)으로 떨어진다.
//
// 2차 정렬로 항상 id 를 붙인다. 같은 밀리초에 여러 사건이 생기면(연결 직후 인벤토리
// 수신 등) 순서가 요청마다 달라져, 쪽을 넘길 때 같은 줄이 두 번 보이거나 한 줄이
// 통째로 건너뛰어진다.
func (r *RemoteAuditSQLiteRepository) List(ctx context.Context, q RemoteAuditQuery) ([]RemoteAuditRecord, int64, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}

	where, args := auditWhere(q)
	order := auditOrder(q)

	var total int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM remote_audit`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count remote audit: %w", err)
	}

	const cols = `id, instance_id, actor, action, domain, command_action, result, reason, ts`
	pageArgs := append(append([]any(nil), args...), limit, offset)
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+cols+` FROM remote_audit`+where+` ORDER BY `+order+` LIMIT ? OFFSET ?`,
		pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list remote audit: %w", err)
	}
	defer rows.Close()

	var out []RemoteAuditRecord
	for rows.Next() {
		var rec RemoteAuditRecord
		if err := rows.Scan(&rec.ID, &rec.InstanceID, &rec.Actor, &rec.Action,
			&rec.Domain, &rec.CommandAction, &rec.Result, &rec.Reason, &rec.Timestamp); err != nil {
			return nil, 0, fmt.Errorf("scan remote audit row: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate remote audit rows: %w", err)
	}
	return out, total, nil
}

// auditWhere 는 필터 절과 인자를 만든다. 빈 필드는 거르지 않는다.
func auditWhere(q RemoteAuditQuery) (string, []any) {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if q.InstanceID != "" {
		clauses = append(clauses, "instance_id = ?")
		args = append(args, q.InstanceID)
	}
	if q.Action != "" {
		clauses = append(clauses, "action = ?")
		args = append(args, q.Action)
	}
	if q.Actor != "" {
		clauses = append(clauses, "actor = ?")
		args = append(args, q.Actor)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// auditOrder 는 ORDER BY 절을 만든다. 정렬 기준은 화이트리스트를 지난 컬럼명만 쓴다.
func auditOrder(q RemoteAuditQuery) string {
	field := AuditSortTime
	switch q.SortField {
	case AuditSortInstance, AuditSortActor, AuditSortAction:
		field = q.SortField
	}
	dir := "DESC"
	if q.SortAsc {
		dir = "ASC"
	}
	// id 2차 정렬로 동률의 순서를 고정한다(쪽 넘김 안정성).
	return field + " " + dir + ", id " + dir
}

// DeleteOlderThan 은 beforeMs 보다 오래된 감사 레코드를 지우고 지운 건수를 반환한다
// (@SPEC:SPEC-REMOTE-LOG-001 보존 정책).
//
// beforeMs 가 0 이하면 아무것도 지우지 않는다 — "보존 기간 미설정" 을 "전부 삭제" 로
// 읽으면 한 번의 설정 실수가 감사 기록을 통째로 날린다.
func (r *RemoteAuditSQLiteRepository) DeleteOlderThan(ctx context.Context, beforeMs int64) (int64, error) {
	if beforeMs <= 0 {
		return 0, nil
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM remote_audit WHERE ts < ?`, beforeMs)
	if err != nil {
		return 0, fmt.Errorf("delete old remote audit records: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, nil // 건수를 못 읽어도 삭제 자체는 성공했다.
	}
	return affected, nil
}

// Close 는 데이터베이스 연결을 닫는다.
func (r *RemoteAuditSQLiteRepository) Close() error {
	return r.db.Close()
}
