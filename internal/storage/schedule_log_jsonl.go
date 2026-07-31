// schedule_log_jsonl.go 는 ScheduleLogRepository 의 JSON-Lines 파일 구현이다
// (@SPEC:SPEC-SCHEDULE-VIEW-001 — 스케줄 로그 저장소 백엔드 선택).
//
// append-only 파일: 한 줄에 JSON 인코딩된 ScheduleLogRecord 하나를 기록한다. sqlite 대비
// 의존성 없이(순수 stdlib) 사람이 읽을 수 있는 영속 로그를 남기려는 용도. List/Count 는
// 전체 파일을 읽어 언마샬·필터·정렬·페이지네이션한다(파일 크기가 큰 운영은 sqlite 권장).
// 필터 의미는 sqlite/memory 와 동일하다(AgentID 는 declared 또는 actor 매칭 — RD-6).
//
// 손상 방어: 언마샬 실패한 줄은 건너뛴다(전체 읽기를 실패시키지 않음). ID 는 append 시
// 파일의 현재 줄 수 + 1 로 부여한다(단조 증가, 조회 tie-break 용).
package storage

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// 컴파일 타임 인터페이스 구현 검증.
var _ ScheduleLogRepository = (*ScheduleLogJSONLRepository)(nil)

// ScheduleLogJSONLRepository 는 JSON-Lines 파일 기반 저장소이다. 뮤텍스로 동시 접근을
// 보호한다(파일 append/읽기/truncate 직렬화).
type ScheduleLogJSONLRepository struct {
	mu     sync.Mutex
	path   string
	nextID int64
	closed bool
}

// NewScheduleLogJSONLRepository 는 JSONL 저장소를 생성한다. 부모 디렉터리를 멱등 생성하고
// (sqlite 구현과 동일한 dir-create 패턴), 기존 파일이 있으면 줄 수로 nextID 를 초기화한다.
func NewScheduleLogJSONLRepository(path string) (*ScheduleLogJSONLRepository, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create schedule log directory: %w", err)
	}
	r := &ScheduleLogJSONLRepository{path: path}
	// 기존 파일의 줄 수로 nextID 를 복원한다(재시작 후 ID 단조성 보존). 파일 부재는 정상.
	recs, err := r.readAll()
	if err != nil {
		return nil, err
	}
	var maxID int64
	for _, rec := range recs {
		if rec.ID > maxID {
			maxID = rec.ID
		}
	}
	r.nextID = maxID
	return r, nil
}

// Append 는 레코드 한 줄을 파일 끝에 추가한다(추가 전용). ID 를 자동 증가로 부여한다.
func (r *ScheduleLogJSONLRepository) Append(_ context.Context, rec ScheduleLogRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrScheduleLogClosed
	}
	r.nextID++
	rec.ID = r.nextID

	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal schedule log record: %w", err)
	}

	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open schedule log file: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write schedule log line: %w", err)
	}
	return nil
}

// List 는 파일 전체를 읽어 필터·최신순 정렬·페이지네이션을 적용해 반환한다. 필터 의미는
// sqlite/memory 와 동일하다(AgentID 는 declared 또는 actor 매칭 — RD-6). limit<=0 이면
// 100 으로 보정하고 offset<0 이면 0 으로 보정한다.
func (r *ScheduleLogJSONLRepository) List(_ context.Context, f ScheduleLogFilter, limit, offset int) ([]ScheduleLogRecord, error) {
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

	all, err := r.readAll()
	if err != nil {
		return nil, err
	}
	filtered := filterScheduleLogRecords(all, f)
	sortScheduleLogRecordsNewestFirst(filtered)

	if offset >= len(filtered) {
		return []ScheduleLogRecord{}, nil
	}
	filtered = filtered[offset:]
	if limit < len(filtered) {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

// Count 는 필터에 매칭되는 전체 레코드 수를 반환한다(페이지네이션 total 용).
func (r *ScheduleLogJSONLRepository) Count(_ context.Context, f ScheduleLogFilter) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, ErrScheduleLogClosed
	}
	all, err := r.readAll()
	if err != nil {
		return 0, err
	}
	return len(filterScheduleLogRecords(all, f)), nil
}

// Clear 는 파일을 truncate 하여 모든 스케줄 로그를 삭제한다(수동 전체 초기화). 파일이
// 없으면 no-op 성공이다.
func (r *ScheduleLogJSONLRepository) Clear(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrScheduleLogClosed
	}
	if err := os.Truncate(r.path, 0); err != nil {
		if os.IsNotExist(err) {
			return nil // 파일 미생성 → 지울 것 없음(멱등)
		}
		return fmt.Errorf("truncate schedule log file: %w", err)
	}
	return nil
}

// Close 는 저장소를 닫는다. 파일 핸들은 각 연산에서 열고 닫으므로 상태 플래그만 세운다.
func (r *ScheduleLogJSONLRepository) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}

// readAll 은 파일의 모든 줄을 언마샬해 레코드 슬라이스로 반환한다. 파일 부재는 빈
// 슬라이스로 취급하고, 손상(언마샬 실패)된 줄은 건너뛴다(전체 읽기를 실패시키지 않음).
// 호출자가 뮤텍스를 보유한 상태에서 호출해야 한다.
func (r *ScheduleLogJSONLRepository) readAll() ([]ScheduleLogRecord, error) {
	f, err := os.Open(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open schedule log file: %w", err)
	}
	defer f.Close()

	var out []ScheduleLogRecord
	scanner := bufio.NewScanner(f)
	// 긴 줄(대용량 targets JSON) 대비 버퍼 상한을 넉넉히(최대 1MB/줄) 늘린다.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec ScheduleLogRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue // 손상된 줄은 건너뛴다(방어적 읽기)
		}
		out = append(out, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan schedule log file: %w", err)
	}
	return out, nil
}
