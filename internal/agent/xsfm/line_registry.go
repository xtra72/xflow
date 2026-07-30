package xsfm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// ---------------------------------------------------------------------------
// M1 — 라인 1급 엔티티 + 라인 레지스트리 (신설 비침습 가산 레이어)
// (SPEC-XSFM-LINE-001 REQ-01-01..08, RD-1/RD-7)
// ---------------------------------------------------------------------------
//
// 라인 레지스트리는 GroupRegistry/StationRegistry 가 그러했듯 별도 레이어로 신설된다.
// 기존 station→line 파생·fan-out·그룹(GROUP-001) 동작의 구조를 바꾸지 않으며 무회귀여야
// 한다(비침습 가산 레이어, §1.3). 라인 레지스트리는 라인 엔티티의 존재·코드·표시명·정렬만
// 관장하며 멤버 device_id 를 저장하지 않는다 — line:<code> 멤버는 항상 DevicesByLine 로
// 파생한다(REQ-01-08, 파생 vs 명시 분리).
//
// 락 규율(REQ-07-01): 라인 레지스트리는 로스터 락(agent.mu)·pending 락·station 레지스트리
// 락·group 레지스트리 락과 완전히 분리된 자체 RWMutex 를 가지며 절대 중첩하지 않는다
// (프로젝트 RWMutex 재진입 deadlock 트랩 회피, HVAC 에이전트 교훈). 쓰기 경로는 락 하에서
// 캐시만 갱신하고 스냅샷을 뜬 뒤 락을 해제하고 저장소에 write-through 한다(파일 I/O 를 락
// 구간 안에서 수행하지 않는다).

// Line 은 라인 1급 엔티티이다 (SPEC-XSFM-LINE-001 §4.1, REQ-01-01).
//
// Code 는 고유 식별자(group id 의 line:<code> 로 사용), Name 은 표시명, Order 는 UI 정렬
// 키(오름차순, RD-7, 기본값=생성 순번)이다.
type Line struct {
	Code  string `json:"code"`            // 고유 식별자(코드).
	Name  string `json:"name"`            // 표시명.
	Order int    `json:"order,omitempty"` // UI 정렬 키(오름차순).
}

// LineRegistry 는 라인 엔티티의 인메모리 캐시 + 영속 표면이다(신설 레이어).
// GroupRegistry 패턴을 미러한다: 자체 RWMutex + 캐시(code→Line) + write-through 영속.
type LineRegistry struct {
	mu    sync.RWMutex
	cache map[string]Line // code → Line.

	// store 는 영속 저장소이다. dir 이 빈 값이면 nil(인메모리 전용).
	store *lineRegistryStore
}

// newLineRegistry 는 저장소 디렉터리로부터 LineRegistry 를 구성한다.
//
// dir 이 비어 있으면 저장소 없이 인메모리 전용으로 동작한다(단위 테스트/영속 비활성). dir 이
// 있으면 {dir}/line_registry.json 저장소를 열고 영속된 라인을 캐시에 복원한다(device_registry.json
// / group_registry.json 과 파일명이 달라 같은 dir 을 공유해도 충돌하지 않는다).
func newLineRegistry(dir string) (*LineRegistry, error) {
	r := &LineRegistry{cache: make(map[string]Line)}
	if dir == "" {
		return r, nil
	}
	store, err := newLineRegistryStore(dir)
	if err != nil {
		return nil, fmt.Errorf("line registry: %w", err)
	}
	r.store = store

	persisted, err := store.load()
	if err != nil {
		return nil, fmt.Errorf("line registry: load persisted: %w", err)
	}
	for code, l := range persisted {
		l.Code = code // 키를 권위로(방어적).
		r.cache[code] = l
	}
	return r, nil
}

// UpsertLine 은 라인을 추가/갱신하고 저장소에 write-through 한다(REQ-01-03, upsert 의미).
//
// @MX:WARN: 자체 RWMutex 하에서 캐시만 갱신하고 스냅샷을 뜬 뒤 락을 해제하고 persist 를
// 호출할 것 — 파일 I/O 를 락 구간 안에서 수행하지 말 것.
// @MX:REASON: 락을 파일 I/O 에 걸쳐 잡으면 동시 라인 조회가 장시간 블록되고, 타 레지스트리
// 락과의 잠금 순서가 얽혀 교착 위험이 생긴다(REQ-07-01 락 규율).
func (r *LineRegistry) UpsertLine(l Line) error {
	if l.Code == "" {
		return fmt.Errorf("%w: line code is required", ErrLineNotFound)
	}
	r.mu.Lock()
	r.cache[l.Code] = l
	snapshot := r.snapshotLocked()
	r.mu.Unlock()

	return r.persist(snapshot)
}

// RemoveLine 은 라인을 제거하고 저장소에 write-through 한다(REQ-01-05). 미등록 라인은
// ErrLineNotFound 를 반환한다. 참조 검사(ErrLineInUse)는 에이전트 레이어(handleRemoveLine)가
// station 레지스트리를 SSOT 로 담당한다(레지스트리 간 락 중첩 금지).
func (r *LineRegistry) RemoveLine(code string) error {
	r.mu.Lock()
	if _, ok := r.cache[code]; !ok {
		r.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrLineNotFound, code)
	}
	delete(r.cache, code)
	snapshot := r.snapshotLocked()
	r.mu.Unlock()

	return r.persist(snapshot)
}

// GetLine 은 라인의 값 복사본을 조회한다. 미등록 시 ErrLineNotFound.
func (r *LineRegistry) GetLine(code string) (Line, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	l, ok := r.cache[code]
	if !ok {
		return Line{}, fmt.Errorf("%w: %q", ErrLineNotFound, code)
	}
	return l, nil
}

// ListLines 는 전체 라인을 Order 오름차순(동률 시 Code 사전순)으로 반환한다(REQ-01-06).
func (r *LineRegistry) ListLines() []Line {
	r.mu.RLock()
	out := make([]Line, 0, len(r.cache))
	for _, l := range r.cache {
		out = append(out, l)
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Code < out[j].Code
	})
	return out
}

// nextOrderLocked 는 현재 캐시의 최대 Order + 1 을 반환한다(생성 순번 기본값, RD-7).
// 락을 보유한 채 호출한다.
func (r *LineRegistry) nextOrderLocked() int {
	max := 0
	for _, l := range r.cache {
		if l.Order > max {
			max = l.Order
		}
	}
	return max + 1
}

// ensureLine 은 code 가 없으면 Line{Code, Name, Order=다음순번} 로 생성하고, 있으면 무시한다
// (REQ-05-01 ensure-create, 멱등). 마이그레이션 전용. 생성 시에만 write-through 한다.
func (r *LineRegistry) ensureLine(code, name string) {
	if code == "" {
		return
	}
	r.mu.Lock()
	if _, ok := r.cache[code]; ok {
		r.mu.Unlock()
		return // 이미 존재 — 멱등.
	}
	r.cache[code] = Line{Code: code, Name: name, Order: r.nextOrderLocked()}
	snapshot := r.snapshotLocked()
	r.mu.Unlock()

	_ = r.persist(snapshot) // 마이그레이션 영속 실패는 시동을 막지 않는다(인메모리 상태 유효).
}

// snapshotLocked 는 캐시의 복사 스냅샷을 만든다(영속용). 락을 보유한 채 호출한다.
func (r *LineRegistry) snapshotLocked() map[string]Line {
	out := make(map[string]Line, len(r.cache))
	for code, l := range r.cache {
		out[code] = l
	}
	return out
}

// persist 는 스냅샷을 저장소에 atomic 하게 기록한다(저장소 미설정이면 no-op).
func (r *LineRegistry) persist(snapshot map[string]Line) error {
	if r.store == nil {
		return nil
	}
	return r.store.save(snapshot)
}

// ---------------------------------------------------------------------------
// lineRegistryStore — 라인 엔티티의 파일 기반 영속 저장소
// (groupRegistryStore 패턴 미러: 단일 JSON 파일 + tmp+rename atomic write)
// ---------------------------------------------------------------------------

// lineRegistryStore 는 라인 레지스트리의 파일 기반 영속 저장소이다(REQ-07-02).
// {dir}/line_registry.json 에 저장하며 자체 mu + atomic write 를 사용한다.
type lineRegistryStore struct {
	filePath string
	mu       sync.Mutex
}

// newLineRegistryStore 는 파일 기반 라인 저장소를 생성한다. 파일은 {dir}/line_registry.json.
func newLineRegistryStore(dir string) (*lineRegistryStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create line registry directory: %w", err)
	}
	return &lineRegistryStore{filePath: filepath.Join(dir, "line_registry.json")}, nil
}

// load 는 저장된 라인을 code → Line 맵으로 읽어들인다. 파일이 없으면 빈 맵 + nil.
func (s *lineRegistryStore) load() (map[string]Line, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Line{}, nil
		}
		return nil, fmt.Errorf("read line registry: %w", err)
	}
	if len(data) == 0 {
		return map[string]Line{}, nil
	}
	var out map[string]Line
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal line registry: %w", err)
	}
	return out, nil
}

// save 는 라인 스냅샷을 JSON 파일에 atomic 하게 기록한다(tmp write + rename).
func (s *lineRegistryStore) save(lines map[string]Line) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(lines, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal line registry: %w", err)
	}

	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp line registry file: %w", err)
	}
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp line registry file: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// M1 — 라인 레지스트리 런타임 CRUD 명령 핸들러 (add_line/remove_line/list_lines)
// ---------------------------------------------------------------------------

// handleAddLine 은 add_line 명령을 처리한다(REQ-01-03/04). code(필수, RD-6 포맷 검증) 로 라인을
// upsert 하고 영속화한다. name 이 비면 code 를 표시명으로 쓴다. 이미 존재하면 표시명/정렬을
// 갱신하는 upsert 의미이다(중복 신규 생성 아님).
func (a *XSFMAgent) handleAddLine(req processRequest) ([]byte, error) {
	if req.Code == "" {
		return nil, fmt.Errorf("%w: add_line requires code", ErrInvalidCommand)
	}
	if !validCode(req.Code) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidCode, req.Code)
	}
	name := req.Name
	if name == "" {
		name = req.Code
	}
	order := req.Order
	if order == 0 {
		// 신규 라인은 생성 순번을 기본 Order 로 부여한다(RD-7). 기존 라인 upsert 는 아래
		// UpsertLine 이 값을 그대로 반영하므로, order 미지정 재지정은 다음 순번으로 재계산되지
		// 않도록 기존 Order 를 보존한다.
		if existing, err := a.lines.GetLine(req.Code); err == nil {
			order = existing.Order
		} else {
			order = a.nextLineOrder()
		}
	}
	if err := a.lines.UpsertLine(Line{Code: req.Code, Name: name, Order: order}); err != nil {
		return nil, err
	}

	a.sendEvent("line_registered", map[string]any{"code": req.Code, "name": name})

	return json.Marshal(map[string]any{
		"status":  "ok",
		"code":    req.Code,
		"command": "add_line",
	})
}

// nextLineOrder 는 라인 레지스트리 락 하에서 다음 생성 순번을 계산한다(락 중첩 없음).
func (a *XSFMAgent) nextLineOrder() int {
	a.lines.mu.Lock()
	defer a.lines.mu.Unlock()
	return a.lines.nextOrderLocked()
}

// handleRemoveLine 은 remove_line 명령을 처리한다(REQ-01-05/05a, RD-5). 해당 라인 코드를
// 참조하는 역사가 하나라도 있으면 ErrLineInUse 로 거부하고, 참조가 없을 때만 제거+영속한다.
// 미등록 라인은 ErrLineNotFound.
//
// 락 규율: station 레지스트리 참조 검사(자체 락)를 먼저 수행해 해제한 뒤 라인 레지스트리를
// 제거한다 — 두 레지스트리 락을 중첩하지 않는다.
func (a *XSFMAgent) handleRemoveLine(req processRequest) ([]byte, error) {
	if req.Code == "" {
		return nil, fmt.Errorf("%w: remove_line requires code", ErrInvalidCommand)
	}
	// RD-5: 참조 중인 라인은 거부. station 레지스트리(자체 락)를 SSOT 로 참조 여부를 판정한다.
	if refs := a.stations.StationsByLine(req.Code); len(refs) > 0 {
		return nil, fmt.Errorf("%w: %q (%d station(s))", ErrLineInUse, req.Code, len(refs))
	}
	if err := a.lines.RemoveLine(req.Code); err != nil {
		return nil, err
	}

	a.sendEvent("line_unregistered", map[string]any{"code": req.Code})

	return json.Marshal(map[string]any{
		"status":  "ok",
		"code":    req.Code,
		"command": "remove_line",
	})
}

// ---------------------------------------------------------------------------
// M2 — 로드 마이그레이션 (라인 ensure-create) + 라인 해석 헬퍼
// ---------------------------------------------------------------------------

// migrateLinesFromStations 는 로드(Init) 시 station 레지스트리의 모든 엔트리 Line 값 집합을
// 순회하여, 라인 레지스트리에 없는 코드는 Line{Code:line, Name:line, Order:다음순번} 로
// ensure-create 한다(REQ-05-01, §4.4.1). 비파괴·멱등(이미 존재하면 무시). 라인은 slugify
// 하지 않고 기존 문자열을 code 로 그대로 승격한다(레거시 라인 코드 보존).
//
// 락 규율: station 레지스트리(자체 락)에서 라인 문자열을 스냅샷한 뒤 해제하고, 그다음 라인
// 레지스트리에 ensure-create 한다 — 두 레지스트리 락을 중첩하지 않는다.
func (a *XSFMAgent) migrateLinesFromStations() {
	if a.lines == nil || a.stations == nil {
		return
	}
	// 결정적 순서(Order 오름차순 정렬된 ListStations)로 순회해 Order 부여를 재현 가능하게 한다.
	seen := make(map[string]struct{})
	var lineCodes []string
	for _, e := range a.stations.ListStations() {
		if e.Line == "" {
			continue
		}
		if _, ok := seen[e.Line]; ok {
			continue
		}
		seen[e.Line] = struct{}{}
		lineCodes = append(lineCodes, e.Line)
	}
	for _, code := range lineCodes {
		a.lines.ensureLine(code, code) // name = code(레거시 승격 기본값).
	}
}

// resolveLineFor 는 station→line 코드를 해석한다(composeName 라인 세그먼트 주입용).
// station 레지스트리(자체 락)만 취득하며, 반드시 로스터 락을 보유하지 않은 상태에서 호출한다
// (레지스트리 간 락 중첩 금지, REQ-07-01). 미해석(미등록 station·빈 라인·레지스트리 부재)이면
// "" 를 반환해 호출부가 라인 세그먼트를 생략(3-세그먼트)하도록 한다(RD-4).
func (a *XSFMAgent) resolveLineFor(station string) string {
	if a.stations == nil || station == "" {
		return ""
	}
	line, err := a.stations.ResolveLine(station)
	if err != nil {
		return ""
	}
	return line
}

// handleListLines 는 list_lines 명령을 처리한다(REQ-01-06). 라인을 Order 오름차순으로 반환한다.
func (a *XSFMAgent) handleListLines() ([]byte, error) {
	lines := a.lines.ListLines()
	out := make([]map[string]any, 0, len(lines))
	for _, l := range lines {
		out = append(out, map[string]any{
			"code":  l.Code,
			"name":  l.Name,
			"order": l.Order,
		})
	}
	return json.Marshal(map[string]any{"status": "ok", "lines": out})
}
