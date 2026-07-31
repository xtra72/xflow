// schedule_log_sqlite.go 는 ScheduleLogRepository 의 SQLite 구현이다
// (@SPEC:SPEC-SCHEDULE-VIEW-001 M1, spec §4.1).
//
// RemoteAuditSQLiteRepository 패턴을 준용한다: WAL 모드, IF NOT EXISTS 멱등
// 마이그레이션, 기존 xflow.db 에 schedule_log 테이블을 멱등 추가. append-only 이므로
// update/delete SQL 은 제공하지 않으며, 보존은 무제한이다(RD-11 — 프루닝 없음).
//
// 인덱스: correlation_id(fire↔result 조인), schedule_id/actor_agent_id/
// declared_agent_id(필터), timestamp DESC(최신순 조회) 를 가속한다.
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
var _ ScheduleLogRepository = (*ScheduleLogSQLiteRepository)(nil)

// ScheduleLogSQLiteRepository 는 SQLite 기반 ScheduleLogRepository 구현이다.
type ScheduleLogSQLiteRepository struct {
	db *sql.DB
}

// NewScheduleLogSQLiteRepository 는 SQLite DB 를 열고 schedule_log 테이블을 멱등하게
// 생성한 후 저장소를 반환한다. WAL 모드를 활성화한다(sqliteDSN 재사용).
func NewScheduleLogSQLiteRepository(ctx context.Context, dbPath string) (*ScheduleLogSQLiteRepository, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	// DSN 에 WAL + busy_timeout pragma 를 실어 모든 풀 연결에 적용한다(동시 기록 경합
	// 흡수 — remote_audit 저장소와 동일한 sqliteDSN 헬퍼 재사용).
	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := migrateScheduleLogSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return &ScheduleLogSQLiteRepository{db: db}, nil
}

// migrateScheduleLogSchema 는 schedule_log 테이블과 조회 인덱스를 멱등하게 생성한다
// (spec §4.1).
//
// 모든 컬럼은 ScheduleLogRecord 필드를 snake_case 로 매핑한다. 시크릿(명령 인자/
// 토큰/페이로드) 컬럼은 의도적으로 없다(reason 은 비밀-아님 분류만 담는다).
func migrateScheduleLogSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schedule_log (
		id                INTEGER PRIMARY KEY AUTOINCREMENT,
		correlation_id    TEXT    NOT NULL DEFAULT '',
		record_kind       TEXT    NOT NULL DEFAULT '',
		schedule_id       TEXT    NOT NULL DEFAULT '',
		rule_name         TEXT    NOT NULL DEFAULT '',
		declared_agent_id TEXT    NOT NULL DEFAULT '',
		actor_agent_id    TEXT    NOT NULL DEFAULT '',
		trigger_time      INTEGER NOT NULL DEFAULT 0,
		target            TEXT    NOT NULL DEFAULT '',
		action            TEXT    NOT NULL DEFAULT '',
		result            TEXT    NOT NULL DEFAULT '',
		targets           TEXT    NOT NULL DEFAULT '',
		reason            TEXT    NOT NULL DEFAULT '',
		timestamp         INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		return fmt.Errorf("create schedule_log table: %w", err)
	}

	// 조회 인덱스: 조인 키·필터 컬럼·최신순 정렬을 각각 가속한다.
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_schedule_log_corr ON schedule_log (correlation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_schedule_log_schedule ON schedule_log (schedule_id)`,
		`CREATE INDEX IF NOT EXISTS idx_schedule_log_actor ON schedule_log (actor_agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_schedule_log_declared ON schedule_log (declared_agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_schedule_log_ts ON schedule_log (timestamp DESC)`,
	}
	for _, stmt := range indexes {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create schedule_log index: %w", err)
		}
	}
	return nil
}

// Append 는 로그 레코드를 추가한다(추가 전용). fire 와 result 는 각각 별도의
// Append 호출로 기록된다.
func (r *ScheduleLogSQLiteRepository) Append(ctx context.Context, rec ScheduleLogRecord) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO schedule_log (
			correlation_id, record_kind, schedule_id, rule_name, declared_agent_id,
			actor_agent_id, trigger_time, target, action, result, targets, reason, timestamp
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, rec.CorrelationID, rec.RecordKind, rec.ScheduleID, rec.RuleName, rec.DeclaredAgentID,
		rec.ActorAgentID, rec.TriggerTime, rec.Target, rec.Action, rec.Result, rec.Targets, rec.Reason, rec.Timestamp)
	if err != nil {
		return fmt.Errorf("append schedule log: %w", err)
	}
	return nil
}

// scheduleLogWhere 는 필터 조건을 SQL WHERE 절과 인자 슬라이스로 구성한다(빈 값은
// 무시). AgentID 필터는 DeclaredAgentID 또는 ActorAgentID 중 하나라도 일치하면
// 매칭한다(RD-6). List/Count 가 동일 WHERE 의미를 공유하도록 여기에 단일화한다.
func scheduleLogWhere(f ScheduleLogFilter) (where string, args []any) {
	appendCond := func(cond string, values ...any) {
		if where == "" {
			where = " WHERE " + cond
		} else {
			where += " AND " + cond
		}
		args = append(args, values...)
	}
	if f.ScheduleID != "" {
		appendCond("schedule_id = ?", f.ScheduleID)
	}
	if f.RuleName != "" {
		appendCond("rule_name = ?", f.RuleName)
	}
	if f.AgentID != "" {
		// 선언된 대상 또는 실제 실행자 중 하나라도 일치하면 매칭(RD-6).
		appendCond("(declared_agent_id = ? OR actor_agent_id = ?)", f.AgentID, f.AgentID)
	}
	return where, args
}

// List 는 로그 레코드를 최신순(timestamp 내림차순, 동률은 id 내림차순)으로 반환한다.
// 필터 조건에 따라 WHERE 절을 동적으로 구성한다. AgentID 필터는 DeclaredAgentID 또는
// ActorAgentID 중 하나라도 일치하면 매칭한다(RD-6). limit<=0 이면 100 으로 보정한다.
func (r *ScheduleLogSQLiteRepository) List(ctx context.Context, f ScheduleLogFilter, limit, offset int) ([]ScheduleLogRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	where, args := scheduleLogWhere(f)

	const cols = `id, correlation_id, record_kind, schedule_id, rule_name, declared_agent_id,
		actor_agent_id, trigger_time, target, action, result, targets, reason, timestamp`
	args = append(args, limit, offset)
	query := `SELECT ` + cols + ` FROM schedule_log` + where +
		` ORDER BY timestamp DESC, id DESC LIMIT ? OFFSET ?`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list schedule log: %w", err)
	}
	defer rows.Close()

	var out []ScheduleLogRecord
	for rows.Next() {
		var rec ScheduleLogRecord
		if err := rows.Scan(&rec.ID, &rec.CorrelationID, &rec.RecordKind, &rec.ScheduleID,
			&rec.RuleName, &rec.DeclaredAgentID, &rec.ActorAgentID, &rec.TriggerTime,
			&rec.Target, &rec.Action, &rec.Result, &rec.Targets, &rec.Reason, &rec.Timestamp); err != nil {
			return nil, fmt.Errorf("scan schedule log row: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schedule log rows: %w", err)
	}
	return out, nil
}

// Count 는 필터에 매칭되는 전체 레코드 수를 반환한다(페이지네이션 total 용). List 와
// 동일한 WHERE 절(AgentID 는 declared 또는 actor 매칭 — RD-6)을 사용하되 limit/offset 은
// 적용하지 않는다.
func (r *ScheduleLogSQLiteRepository) Count(ctx context.Context, f ScheduleLogFilter) (int, error) {
	where, args := scheduleLogWhere(f)
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schedule_log`+where, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count schedule log: %w", err)
	}
	return n, nil
}

// Clear 는 저장된 모든 스케줄 로그를 삭제한다(수동 전체 초기화). append-only 예외 —
// 개별 삭제 SQL 은 여전히 없고, 전량 DELETE 만 허용한다. 테이블은 유지한다.
func (r *ScheduleLogSQLiteRepository) Clear(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM schedule_log`); err != nil {
		return fmt.Errorf("clear schedule log: %w", err)
	}
	return nil
}

// Close 는 데이터베이스 연결을 닫는다.
func (r *ScheduleLogSQLiteRepository) Close() error {
	return r.db.Close()
}
