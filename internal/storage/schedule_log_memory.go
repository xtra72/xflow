// schedule_log_memory.go 는 ScheduleLogRepository 의 인메모리 구현이다
// (@SPEC:SPEC-SCHEDULE-VIEW-001 — 스케줄 로그 저장소 백엔드 선택).
//
// 비영속: 레코드는 프로세스 메모리에만 보관되며 재시작 시 소실된다(의도된 동작 —
// 시크릿-없는 감사 로그를 디스크에 남기지 않으려는 배포/테스트 용도). List 필터·정렬·
// 페이지네이션 의미는 sqlite 구현과 동일하다(ScheduleID/RuleName 정확 일치, AgentID 는
// declared 또는 actor 매칭 — RD-6, timestamp DESC · id DESC 최신순).
package storage

import (
	"context"
	"sort"
	"sync"
)

// 컴파일 타임 인터페이스 구현 검증.
var _ ScheduleLogRepository = (*ScheduleLogMemoryRepository)(nil)

// ScheduleLogMemoryRepository 는 뮤텍스로 보호되는 슬라이스 기반 인메모리 저장소이다.
type ScheduleLogMemoryRepository struct {
	mu      sync.Mutex
	records []ScheduleLogRecord
	nextID  int64
	closed  bool
}

// NewScheduleLogMemoryRepository 는 빈 인메모리 스케줄 로그 저장소를 생성한다.
func NewScheduleLogMemoryRepository() *ScheduleLogMemoryRepository {
	return &ScheduleLogMemoryRepository{}
}

// Append 는 로그 레코드를 추가한다(추가 전용). ID 를 자동 증가로 채운다. 닫힌 뒤에는
// ErrScheduleLogClosed 를 반환한다.
func (r *ScheduleLogMemoryRepository) Append(_ context.Context, rec ScheduleLogRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrScheduleLogClosed
	}
	r.nextID++
	rec.ID = r.nextID
	r.records = append(r.records, rec)
	return nil
}

// List 는 필터·최신순 정렬·페이지네이션을 적용해 레코드를 반환한다. 필터 의미는
// sqlite 구현과 동일하다(AgentID 는 declared 또는 actor 매칭 — RD-6). limit<=0 이면
// 100 으로 보정하고 offset<0 이면 0 으로 보정한다.
func (r *ScheduleLogMemoryRepository) List(_ context.Context, f ScheduleLogFilter, limit, offset int) ([]ScheduleLogRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, ErrScheduleLogClosed
	}
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	filtered := filterScheduleLogRecords(r.records, f)
	sortScheduleLogRecordsNewestFirst(filtered)

	if offset >= len(filtered) {
		return []ScheduleLogRecord{}, nil
	}
	filtered = filtered[offset:]
	if limit < len(filtered) {
		filtered = filtered[:limit]
	}
	// 방어적 복사: 내부 슬라이스 참조를 노출하지 않는다.
	out := make([]ScheduleLogRecord, len(filtered))
	copy(out, filtered)
	return out, nil
}

// Count 는 필터에 매칭되는 전체 레코드 수를 반환한다(페이지네이션 total 용).
func (r *ScheduleLogMemoryRepository) Count(_ context.Context, f ScheduleLogFilter) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, ErrScheduleLogClosed
	}
	return len(filterScheduleLogRecords(r.records, f)), nil
}

// Clear 는 저장된 모든 스케줄 로그를 삭제한다(수동 전체 초기화). ID 카운터는 유지한다
// (기록 순서 단조성 보존).
func (r *ScheduleLogMemoryRepository) Clear(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrScheduleLogClosed
	}
	r.records = nil
	return nil
}

// Close 는 저장소를 닫는다(no-op 리소스 정리 — 이후 쓰기/조회는 ErrScheduleLogClosed).
func (r *ScheduleLogMemoryRepository) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}

// filterScheduleLogRecords 는 필터에 매칭되는 레코드만 새 슬라이스로 반환한다.
// ScheduleID/RuleName 은 정확 일치, AgentID 는 declared 또는 actor 매칭(RD-6).
func filterScheduleLogRecords(records []ScheduleLogRecord, f ScheduleLogFilter) []ScheduleLogRecord {
	out := make([]ScheduleLogRecord, 0, len(records))
	for _, rec := range records {
		if f.ScheduleID != "" && rec.ScheduleID != f.ScheduleID {
			continue
		}
		if f.RuleName != "" && rec.RuleName != f.RuleName {
			continue
		}
		if f.AgentID != "" && rec.DeclaredAgentID != f.AgentID && rec.ActorAgentID != f.AgentID {
			continue
		}
		out = append(out, rec)
	}
	return out
}

// sortScheduleLogRecordsNewestFirst 는 최신순(timestamp 내림차순, 동률은 id 내림차순)
// 으로 in-place 정렬한다(sqlite ORDER BY timestamp DESC, id DESC 와 동일).
func sortScheduleLogRecordsNewestFirst(records []ScheduleLogRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Timestamp != records[j].Timestamp {
			return records[i].Timestamp > records[j].Timestamp
		}
		return records[i].ID > records[j].ID
	})
}
