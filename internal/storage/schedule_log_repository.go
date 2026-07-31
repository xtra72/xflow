// schedule_log_repository.go 는 설비 제어 예약(스케줄) 실행 로그 저장소 인터페이스를
// 정의한다(@SPEC:SPEC-SCHEDULE-VIEW-001 M1, spec §4.1).
//
// 스케줄 로그는 예약 규칙의 발화(fire)와 실행 결과(result)를 각각 append-only 로
// 기록한다. fire↔result 는 CorrelationID(schedule_id + ":" + trigger_time)로 조인한다.
// 갱신/삭제 API 는 노출하지 않으며(감사 무결성), 시크릿(명령 인자/토큰/페이로드)은
// 저장하지 않는다(Reason 은 비밀-아님 분류 텍스트만 담는다).
package storage

import (
	"context"
	"errors"
	"path/filepath"
)

// 스케줄 로그 레코드 종류 상수.
const (
	// ScheduleLogRecordKindFire 는 예약 규칙 발화 이벤트이다(결과 이전).
	ScheduleLogRecordKindFire = "fire"
	// ScheduleLogRecordKindResult 는 예약 실행 결과 이벤트이다(발화 이후).
	ScheduleLogRecordKindResult = "result"
)

// 스케줄 실행 집계 결과 상수.
const (
	// ScheduleLogResultOK 는 실행 성공이다.
	ScheduleLogResultOK = "ok"
	// ScheduleLogResultError 는 실행 실패이다.
	ScheduleLogResultError = "error"
)

// ErrScheduleLogClosed 는 닫힌 저장소에 쓰기를 시도할 때 반환된다.
var ErrScheduleLogClosed = errors.New("schedule log repository closed")

// ScheduleLogRecord 는 단일 스케줄 실행 로그 레코드이다(spec §4.1).
//
// fire 이벤트와 result 이벤트는 별도의 append 로 기록되며, 동일한 CorrelationID 로
// 조인된다. 한 규칙이 여러 대상으로 fan-out 하더라도 result 는 여전히 1 레코드이며,
// 대상별 ok/error 는 Targets(JSON 배열)에 임베드한다.
//
// Timestamp/TriggerTime 은 epoch milliseconds(int64) 이다(프로젝트 규약).
type ScheduleLogRecord struct {
	ID              int64  // 자동 증가 PK(조회 시 채워짐)
	CorrelationID   string // 조인 키 = schedule_id + ":" + trigger_time (fire↔result 연결)
	RecordKind      string // fire | result
	ScheduleID      string // 발화한 스케줄 id
	RuleName        string // 규칙 표시 이름(비어 있을 수 있음)
	DeclaredAgentID string // 스케줄 설정상 선언된 대상 에이전트
	ActorAgentID    string // 실제 실행 에이전트(fire 이벤트에서는 빈 값)
	TriggerTime     int64  // 발화 시각(epoch ms)
	Target          string // 셀렉터 kind=value(예: group_id=station:0150). result 이벤트.
	Action          string // 제어 명령(set_power/set_fan_speed/set_multiple). result 이벤트.
	Result          string // 집계 결과 ok | error(result 이벤트. fire 에서는 빈 값)
	Targets         string // 대상별 ok/error 임베드, JSON 배열 [{target,result,reason}]
	Reason          string // 비밀-아님 사유/오류 분류(예: members=[..] ok=3/4)
	Timestamp       int64  // 레코드 기록 시각(epoch ms)
}

// ScheduleLogFilter 는 스케줄 로그 조회 필터이다(spec §4.1, RD-6).
//
// AgentID 는 DeclaredAgentID 또는 ActorAgentID 중 하나라도 일치하면 매칭한다
// (선언된 대상 또는 실제 실행자 관점 조회 — RD-6).
type ScheduleLogFilter struct {
	ScheduleID string // 빈 값 = 전체
	RuleName   string // 빈 값 = 전체
	AgentID    string // 빈 값 = 전체. DeclaredAgentID 또는 ActorAgentID 와 매칭(RD-6)
}

// ScheduleLogRepository 는 스케줄 실행 로그의 영속 저장소이다(spec §4.1).
//
// append-only: 갱신/삭제 API 를 노출하지 않는다(fire 와 result 는 각각 별도의 추가).
// 보존은 무제한이며 프루닝하지 않는다(RD-11). 조회는 스케줄/규칙/에이전트 필터 +
// 페이지네이션을 지원하며 최신순(timestamp 내림차순)으로 반환한다.
type ScheduleLogRepository interface {
	// Append 는 로그 레코드를 추가한다(추가 전용). fire 와 result 는 각각 별도의
	// Append 호출로 기록되며 갱신/삭제 API 는 없다.
	Append(ctx context.Context, rec ScheduleLogRecord) error
	// List 는 로그 레코드를 최신순(timestamp 내림차순)으로 반환한다. 필터 조건에
	// 따라 스케줄/규칙/에이전트로 좁힌다. limit/offset 으로 페이지네이션한다.
	List(ctx context.Context, f ScheduleLogFilter, limit, offset int) ([]ScheduleLogRecord, error)
	// Count 는 필터에 매칭되는 전체 레코드 수를 반환한다(페이지네이션 total 용).
	// List 와 동일한 필터 의미(AgentID 는 declared 또는 actor 매칭)를 사용하되
	// limit/offset 은 적용하지 않는다.
	Count(ctx context.Context, f ScheduleLogFilter) (int, error)
	// Clear 는 저장된 모든 스케줄 로그를 삭제한다(수동 전체 초기화). append-only 예외 —
	// 개별 삭제는 여전히 없고, 전량 초기화만 허용한다.
	Clear(ctx context.Context) error
	// Close 는 저장소 리소스를 정리한다.
	Close() error
}

// NewScheduleLogRepository 는 storage type 에 따라 ScheduleLogRepository 구현을
// 생성한다(스케줄 로그 저장소 백엔드 선택 — 시작 설정으로 결정).
//
// 디스패치:
//   - "sqlite"/"database"/"db"/"" → SQLite(영속, 기본)
//   - "memory"/"mem"              → 인메모리(비영속, 재시작 시 소실) — path 무시
//   - "file"/"jsonl"              → JSON-Lines 파일(append-only) — path 에서 .jsonl 유도
func NewScheduleLogRepository(ctx context.Context, storageType, sqlitePath string) (ScheduleLogRepository, error) {
	switch storageType {
	case "memory", "mem":
		return NewScheduleLogMemoryRepository(), nil
	case "file", "jsonl":
		return NewScheduleLogJSONLRepository(scheduleLogJSONLPath(sqlitePath))
	case "sqlite", "database", "db", "":
		return NewScheduleLogSQLiteRepository(ctx, sqlitePath)
	default:
		return NewScheduleLogSQLiteRepository(ctx, sqlitePath)
	}
}

// scheduleLogJSONLPath 는 sqlite DB 경로에서 형제(sibling) JSONL 파일 경로를 유도한다.
// 예: "./data/xflow.db" → "./data/schedule_log.jsonl". 빈 경로면 현재 디렉터리 기준.
func scheduleLogJSONLPath(sqlitePath string) string {
	dir := filepath.Dir(sqlitePath)
	if sqlitePath == "" {
		dir = "."
	}
	return filepath.Join(dir, "schedule_log.jsonl")
}
