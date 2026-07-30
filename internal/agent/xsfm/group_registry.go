package xsfm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// M1 — 그룹 1급 개념: 그룹 엔티티 + 그룹 레지스트리 (신설 비침습 레이어)
// (SPEC-XSFM-GROUP-001 REQ-01-01..07, RD-2/RD-3)
// ---------------------------------------------------------------------------
//
// 그룹 레지스트리는 기존 station→line 레지스트리(station_registry.go)·디바이스 로스터
// (device.go/persist.go)·fan-out(control.go/group.go)의 구조를 바꾸지 않고 추가되는 별도
// 레이어이다(REQ-01-02, NF-02 무회귀). 커스텀 그룹의 멤버십만 명시적으로 저장·영속하며,
// 기본 그룹(line/station)은 저장하지 않고 조회 시점에 파생한다(REQ-01-03/04, 파생 vs 명시 분리).
//
// 락 규율(REQ-01-07): 그룹 레지스트리는 로스터 락(agent.mu)·pending 락·station 레지스트리
// 락과 완전히 분리된 자체 RWMutex 를 가지며 절대 중첩하지 않는다(프로젝트 RWMutex 재진입
// deadlock 트랩 회피, HVAC 에이전트 교훈). 쓰기 경로는 락 하에서 캐시만 갱신하고 스냅샷을
// 뜬 뒤 락을 해제하고 저장소에 write-through 한다(락을 파일 I/O 에 걸쳐 잡지 않는다).

// 그룹 타입 상수와 id 타입 접두사 (RD-2: 타입 접두사 인코딩).
const (
	groupTypeCustom  = "custom"
	groupTypeStation = "station"
	groupTypeLine    = "line"

	groupPrefixCustom  = "custom:"
	groupPrefixStation = "station:"
	groupPrefixLine    = "line:"
)

// Group 은 그룹 엔티티이다 (SPEC-XSFM-GROUP-001 §4.1, REQ-01-01).
//
// ID 는 타입 접두사 인코딩(RD-2): "station:<code>" | "line:<code>" | "custom:<name|uuid>".
// 접두사만으로 Type 을 판별할 수 있어 기본 그룹과 커스텀 그룹의 id 충돌을 원천 차단한다.
// Members 는 device_id 목록이며 type=custom 에서만 저장·권위이고, line/station 은 조회 시
// DevicesByLine/DevicesByStation 로 파생한다(저장하지 않음).
type Group struct {
	ID      string   `json:"id"`                // 타입 접두사 인코딩 식별자.
	Name    string   `json:"name"`              // 표시 이름.
	Type    string   `json:"type"`              // "line" | "station" | "custom".
	Ref     string   `json:"ref,omitempty"`     // 파생 그룹의 원천 참조(station 코드/line id). custom 은 "".
	Members []string `json:"members,omitempty"` // device_id 목록 — custom 에서만 저장.
}

// groupTypeFromID 는 그룹 id 의 타입 접두사로부터 타입을 판별한다 (RD-2). 접두사가 없는
// 레거시 id 는 "" 를 반환한다(호출부가 레거시 primary-group 경로로 폴백).
func groupTypeFromID(id string) string {
	switch {
	case strings.HasPrefix(id, groupPrefixCustom):
		return groupTypeCustom
	case strings.HasPrefix(id, groupPrefixStation):
		return groupTypeStation
	case strings.HasPrefix(id, groupPrefixLine):
		return groupTypeLine
	default:
		return ""
	}
}

// customIDName 은 "custom:<name>" 에서 name 부분을 반환한다. RD-2 규약대로 "custom:" 접두사만
// 분리하고 나머지는 원문 보존한다(콜론 포함 커스텀 name 안전).
func customIDName(id string) string { return strings.TrimPrefix(id, groupPrefixCustom) }

// customIDFor 는 name 으로부터 커스텀 그룹 id 를 만든다("custom:" + name).
func customIDFor(name string) string { return groupPrefixCustom + name }

// GroupRegistry 는 커스텀 그룹 멤버십의 인메모리 캐시 + 영속 표면이다(신설 레이어).
// StationRegistry 패턴을 미러한다: 자체 RWMutex + 캐시 + write-through 영속(atomic write).
// 기본 그룹(line/station)은 저장하지 않으므로 캐시에는 커스텀 그룹만 담긴다.
type GroupRegistry struct {
	mu    sync.RWMutex
	cache map[string]Group // id → Group (custom 그룹만).

	// store 는 영속 저장소이다. dir 이 빈 값이면 nil(인메모리 전용).
	store *groupRegistryStore
}

// newGroupRegistry 는 저장소 디렉터리로부터 GroupRegistry 를 구성한다.
//
// dir 이 비어 있으면 저장소 없이 인메모리 전용으로 동작한다(단위 테스트/영속 비활성). dir 이
// 있으면 {dir}/group_registry.json 저장소를 열고 영속된 커스텀 그룹을 캐시에 복원한다. 기본
// 그룹은 저장하지 않으므로 로드 대상이 아니다(재시작 시 station/line 레지스트리로 재파생).
func newGroupRegistry(dir string) (*GroupRegistry, error) {
	r := &GroupRegistry{cache: make(map[string]Group)}
	if dir == "" {
		return r, nil
	}
	store, err := newGroupRegistryStore(dir)
	if err != nil {
		return nil, fmt.Errorf("group registry: %w", err)
	}
	r.store = store

	persisted, err := store.load()
	if err != nil {
		return nil, fmt.Errorf("group registry: load persisted: %w", err)
	}
	for id, g := range persisted {
		if groupTypeFromID(id) != groupTypeCustom {
			continue // 방어적: 기본 그룹은 저장 대상이 아니므로 무시.
		}
		g.Type = groupTypeCustom
		r.cache[id] = normalizeGroup(g)
	}
	return r, nil
}

// UpsertGroup 은 커스텀 그룹을 추가/갱신하고 저장소에 write-through 한다(REQ-01-06).
// 기본 그룹(type≠custom 또는 custom: 접두사 아님)은 ErrGroupNotCustom 으로 거부한다.
//
// @MX:WARN: 자체 RWMutex 하에서 캐시만 갱신하고 스냅샷을 뜬 뒤 락을 해제하고 persist 를
// 호출할 것 — 파일 I/O 를 락 구간 안에서 수행하지 말 것.
// @MX:REASON: 락을 파일 I/O 에 걸쳐 잡으면 동시 그룹 조회/fan-out 이 장시간 블록되고,
// 저장소 락(store.mu)과 레지스트리 락(mu)의 잠금 순서가 얽혀 교착 위험이 생긴다(REQ-01-07).
func (r *GroupRegistry) UpsertGroup(g Group) error {
	if groupTypeFromID(g.ID) != groupTypeCustom {
		return fmt.Errorf("%w: %q", ErrGroupNotCustom, g.ID)
	}
	g.Type = groupTypeCustom
	g.Ref = ""
	g = normalizeGroup(g)

	r.mu.Lock()
	r.cache[g.ID] = g
	snapshot := r.snapshotLocked()
	r.mu.Unlock()

	return r.persist(snapshot)
}

// RemoveGroup 은 커스텀 그룹을 제거하고 저장소에 write-through 한다. 미등록 그룹은
// ErrGroupNotFound, 기본 그룹은 ErrGroupNotCustom 을 반환한다(REQ-03-05/06).
func (r *GroupRegistry) RemoveGroup(id string) error {
	if groupTypeFromID(id) != groupTypeCustom {
		return fmt.Errorf("%w: %q", ErrGroupNotCustom, id)
	}
	r.mu.Lock()
	if _, ok := r.cache[id]; !ok {
		r.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrGroupNotFound, id)
	}
	delete(r.cache, id)
	snapshot := r.snapshotLocked()
	r.mu.Unlock()

	return r.persist(snapshot)
}

// GetGroup 은 커스텀 그룹의 값 복사본을 조회한다. 미등록 시 ErrGroupNotFound.
func (r *GroupRegistry) GetGroup(id string) (Group, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.cache[id]
	if !ok {
		return Group{}, fmt.Errorf("%w: %q", ErrGroupNotFound, id)
	}
	return cloneGroup(g), nil
}

// ListGroups 는 전체 커스텀 그룹의 값 복사본을 결정적 순서(id 사전순)로 반환한다.
// 기본 그룹(line/station)은 여기 포함되지 않으며 상위 list_groups 핸들러가 파생해 합친다.
func (r *GroupRegistry) ListGroups() []Group {
	r.mu.RLock()
	out := make([]Group, 0, len(r.cache))
	for _, g := range r.cache {
		out = append(out, cloneGroup(g))
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// AddMember 는 커스텀 그룹에 device_id 멤버를 추가하고 write-through 한다(중복은 무시).
// 미등록 그룹은 ErrGroupNotFound. 로스터 존재 여부는 검사하지 않는다(RD-3: 조회 시 필터).
func (r *GroupRegistry) AddMember(id, deviceID string) error {
	if groupTypeFromID(id) != groupTypeCustom {
		return fmt.Errorf("%w: %q", ErrGroupNotCustom, id)
	}
	r.mu.Lock()
	g, ok := r.cache[id]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrGroupNotFound, id)
	}
	g.Members = appendIfAbsent(g.Members, deviceID)
	r.cache[id] = g
	snapshot := r.snapshotLocked()
	r.mu.Unlock()

	return r.persist(snapshot)
}

// RemoveMember 는 커스텀 그룹에서 device_id 멤버를 제거하고 write-through 한다.
// 미등록 그룹은 ErrGroupNotFound(멤버 부재는 no-op, 에러 아님 — 비침습).
func (r *GroupRegistry) RemoveMember(id, deviceID string) error {
	if groupTypeFromID(id) != groupTypeCustom {
		return fmt.Errorf("%w: %q", ErrGroupNotCustom, id)
	}
	r.mu.Lock()
	g, ok := r.cache[id]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrGroupNotFound, id)
	}
	g.Members = removeString(g.Members, deviceID)
	r.cache[id] = g
	snapshot := r.snapshotLocked()
	r.mu.Unlock()

	return r.persist(snapshot)
}

// membersOf 는 커스텀 그룹의 멤버 복사본과 존재 여부를 반환한다(GroupMembers custom 분기용).
// 로스터 대조 필터는 호출부(agent 레이어)가 락 해제 후 별도로 수행한다(락 중첩 금지).
func (r *GroupRegistry) membersOf(id string) ([]string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.cache[id]
	if !ok {
		return nil, false
	}
	return append([]string(nil), g.Members...), true
}

// exists 는 커스텀 그룹의 존재 여부를 반환한다(fan-out 미등록 그룹 거부 검사용, REQ-05-05).
func (r *GroupRegistry) exists(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.cache[id]
	return ok
}

// importLegacyMemberships 는 마이그레이션(REQ-02-04)에서 여러 레거시 group_id 를 한 번에
// 커스텀 그룹으로 보장 생성하고 멤버를 편입한다. 락을 한 번만 잡고 한 번만 영속하여 시동
// 시 write 폭주를 피한다(개별 UpsertGroup/AddMember 반복 대비 효율).
func (r *GroupRegistry) importLegacyMemberships(pairs []legacyMembership) {
	if len(pairs) == 0 {
		return
	}
	r.mu.Lock()
	for _, p := range pairs {
		g, ok := r.cache[p.groupID]
		if !ok {
			g = Group{ID: p.groupID, Name: p.name, Type: groupTypeCustom}
		}
		g.Members = appendIfAbsent(g.Members, p.deviceID)
		r.cache[p.groupID] = g
	}
	snapshot := r.snapshotLocked()
	r.mu.Unlock()

	if err := r.persist(snapshot); err != nil {
		// 마이그레이션 영속 실패는 시동을 막지 않는다(best-effort, 인메모리 상태는 유효).
		_ = err
	}
}

// legacyMembership 은 마이그레이션 편입 단위이다(그룹 id + 표시명 + device_id).
type legacyMembership struct {
	groupID  string
	name     string
	deviceID string
}

// snapshotLocked 는 캐시의 깊은 복사 스냅샷을 만든다(영속용). 락을 보유한 채 호출한다.
func (r *GroupRegistry) snapshotLocked() map[string]Group {
	out := make(map[string]Group, len(r.cache))
	for id, g := range r.cache {
		out[id] = cloneGroup(g)
	}
	return out
}

// persist 는 스냅샷을 저장소에 atomic 하게 기록한다(저장소 미설정이면 no-op).
func (r *GroupRegistry) persist(snapshot map[string]Group) error {
	if r.store == nil {
		return nil
	}
	return r.store.save(snapshot)
}

// normalizeGroup 은 그룹의 Members 를 정렬·중복제거한 새 슬라이스로 정규화한다(결정적 순서).
func normalizeGroup(g Group) Group {
	g.Members = dedupSortedStrings(g.Members)
	return g
}

// cloneGroup 은 Group 의 값 복사본을 반환한다(Members 슬라이스 깊은 복사 — 캐시 backing 공유 방지).
func cloneGroup(g Group) Group {
	c := g
	c.Members = append([]string(nil), g.Members...)
	return c
}

// appendIfAbsent 는 s 에 v 가 없으면 추가한 새 슬라이스를, 있으면 정렬된 새 슬라이스를 반환한다
// (항상 새 슬라이스 — in-place 변경/backing 공유 금지).
func appendIfAbsent(s []string, v string) []string {
	for _, e := range s {
		if e == v {
			return dedupSortedStrings(s)
		}
	}
	out := append(append([]string(nil), s...), v)
	return dedupSortedStrings(out)
}

// removeString 은 s 에서 v 를 제거한 새 슬라이스를 반환한다.
func removeString(s []string, v string) []string {
	out := make([]string, 0, len(s))
	for _, e := range s {
		if e != v {
			out = append(out, e)
		}
	}
	return out
}

// dedupSortedStrings 는 정렬·중복제거한 새 슬라이스를 반환한다(빈 입력은 nil).
func dedupSortedStrings(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	cp := append([]string(nil), s...)
	sort.Strings(cp)
	out := cp[:0]
	var prev string
	for i, e := range cp {
		if i == 0 || e != prev {
			out = append(out, e)
		}
		prev = e
	}
	return out
}

// ---------------------------------------------------------------------------
// groupRegistryStore — 커스텀 그룹의 파일 기반 영속 저장소
// (deviceRegistryStore 패턴 미러: 단일 JSON 파일 + tmp+rename atomic write)
// ---------------------------------------------------------------------------

// groupRegistryStore 는 커스텀 그룹 레지스트리의 파일 기반 영속 저장소이다(REQ-01-06/NF-01).
// device_registry.json 과 동일 dir 에 group_registry.json 으로 저장하며(파일명이 달라 충돌
// 없음), deviceRegistryStore 와 동일하게 자체 mu + atomic write 를 사용한다.
type groupRegistryStore struct {
	filePath string
	mu       sync.Mutex
}

// newGroupRegistryStore 는 파일 기반 그룹 저장소를 생성한다. 파일은 {dir}/group_registry.json.
func newGroupRegistryStore(dir string) (*groupRegistryStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create group registry directory: %w", err)
	}
	return &groupRegistryStore{filePath: filepath.Join(dir, "group_registry.json")}, nil
}

// load 는 저장된 커스텀 그룹을 id → Group 맵으로 읽어들인다. 파일이 없으면 빈 맵 + nil.
func (s *groupRegistryStore) load() (map[string]Group, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Group{}, nil
		}
		return nil, fmt.Errorf("read group registry: %w", err)
	}
	if len(data) == 0 {
		return map[string]Group{}, nil
	}
	var out map[string]Group
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal group registry: %w", err)
	}
	return out, nil
}

// save 는 커스텀 그룹 스냅샷을 JSON 파일에 atomic 하게 기록한다(tmp write + rename).
func (s *groupRegistryStore) save(groups map[string]Group) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(groups, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal group registry: %w", err)
	}

	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp group registry file: %w", err)
	}
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp group registry file: %w", err)
	}
	return nil
}
