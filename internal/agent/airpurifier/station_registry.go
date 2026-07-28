package airpurifier

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/xtra/xflow/internal/storage"
)

// StationRegistryEntry 는 역사 레지스트리 항목 타입이다 (REQ-AIRPUR-001-02-09).
//
// 저장소(storage.StationRegistryEntry)와 동일 구조를 재사용하여 station→line 매핑을
// device_metadata 와 별개 저장소로 관리한다 (REQ-AIRPUR-001-02-12). 별칭이므로 B4/저장소
// 간 변환 없이 그대로 흐른다.
type StationRegistryEntry = storage.StationRegistryEntry

// StationRegistry 는 역사(station) → 호선(line) 매핑의 인메모리 캐시 + 조회 표면이다
// (REQ-AIRPUR-001-02-11). 저장소(repo)를 감싸며 자체 RWMutex 로 보호된다.
//
// 락 규율: 이 레지스트리의 락은 로스터 락(agent.mu) · pending 락과 완전히 분리된 자체
// 락이며 절대 중첩하지 않는다 (프로젝트 RWMutex 재진입 deadlock 트랩 회피). 쓰기 경로는
// 락 하에서 캐시만 갱신하고, 락 해제 후 저장소에 write-through 한다.
type StationRegistry struct {
	mu    sync.RWMutex
	cache map[string]StationRegistryEntry

	// repo 는 영속 저장소이다. station_registry_path 가 빈 값이면 nil(인메모리/시드 전용).
	repo storage.StationRegistryRepository
}

// newStationRegistry 는 저장소 경로와 설정 시드로부터 StationRegistry 를 구성한다.
//
// path 가 비어 있으면 저장소 없이 인메모리/시드 전용으로 동작한다. path 가 있으면
// StationRegistryFileRepository 를 생성해 영속 항목을 로드한 뒤 설정 시드를 오버레이한다.
// 병합 규칙: 시드를 먼저 깔고 영속 항목으로 덮어써(런타임 CRUD 가 최신 상태를 반영), 재시작
// 시 런타임 변경이 시드보다 우선한다.
func newStationRegistry(path string, seeds []StationSeed) (*StationRegistry, error) {
	r := &StationRegistry{
		cache: make(map[string]StationRegistryEntry),
	}

	// 설정 시드를 먼저 적재한다.
	for _, s := range seeds {
		if s.Station == "" {
			continue
		}
		r.cache[s.Station] = StationRegistryEntry{
			Station:     s.Station,
			Line:        s.Line,
			DisplayName: s.DisplayName,
			Order:       s.Order,
		}
	}

	if path == "" {
		return r, nil
	}

	repo, err := storage.NewStationRegistryFileRepository(path)
	if err != nil {
		return nil, fmt.Errorf("station registry: %w", err)
	}
	r.repo = repo

	// 영속 항목을 시드 위에 오버레이한다(런타임 CRUD 우선).
	persisted, err := repo.List(context.Background())
	if err != nil {
		return nil, fmt.Errorf("station registry: load persisted: %w", err)
	}
	for station, entry := range persisted {
		r.cache[station] = entry
	}

	return r, nil
}

// UpsertStation 은 station 엔트리를 추가/갱신하고 저장소에 write-through 한다.
func (r *StationRegistry) UpsertStation(entry StationRegistryEntry) error {
	if entry.Station == "" {
		return fmt.Errorf("%w: station is required", ErrStationNotFound)
	}

	r.mu.Lock()
	r.cache[entry.Station] = entry
	r.mu.Unlock()

	if r.repo != nil {
		return r.repo.Save(context.Background(), entry.Station, entry)
	}
	return nil
}

// RemoveStation 은 station 엔트리를 제거하고 저장소에 write-through 한다.
// 미등록 station 은 ErrStationNotFound 를 반환한다.
func (r *StationRegistry) RemoveStation(station string) error {
	r.mu.Lock()
	if _, ok := r.cache[station]; !ok {
		r.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrStationNotFound, station)
	}
	delete(r.cache, station)
	r.mu.Unlock()

	if r.repo != nil {
		return r.repo.Delete(context.Background(), station)
	}
	return nil
}

// GetStation 은 station 엔트리를 조회한다. 미등록 시 ErrStationNotFound.
func (r *StationRegistry) GetStation(station string) (StationRegistryEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.cache[station]
	if !ok {
		return StationRegistryEntry{}, fmt.Errorf("%w: %q", ErrStationNotFound, station)
	}
	return entry, nil
}

// ResolveLine 은 station→line 을 해석한다 (SSOT lookup). 미등록 시 ErrStationNotFound.
func (r *StationRegistry) ResolveLine(station string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.cache[station]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrStationNotFound, station)
	}
	return entry.Line, nil
}

// ListStations 는 전체 역사 목록을 Order 오름차순(동률 시 Station 사전순)으로 반환한다.
func (r *StationRegistry) ListStations() []StationRegistryEntry {
	r.mu.RLock()
	out := make([]StationRegistryEntry, 0, len(r.cache))
	for _, e := range r.cache {
		out = append(out, e)
	}
	r.mu.RUnlock()
	sortStationsByOrder(out)
	return out
}

// StationsByLine 은 특정 호선에 속한 역사 목록을 Order 오름차순으로 반환한다.
func (r *StationRegistry) StationsByLine(line string) []StationRegistryEntry {
	r.mu.RLock()
	out := make([]StationRegistryEntry, 0)
	for _, e := range r.cache {
		if e.Line == line {
			out = append(out, e)
		}
	}
	r.mu.RUnlock()
	sortStationsByOrder(out)
	return out
}

// sortStationsByOrder 는 Order 오름차순, 동률 시 Station 사전순으로 정렬한다(결정적 순서).
func sortStationsByOrder(entries []StationRegistryEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Order != entries[j].Order {
			return entries[i].Order < entries[j].Order
		}
		return entries[i].Station < entries[j].Station
	})
}

// ---------------------------------------------------------------------------
// Module 2B — 역사 레지스트리 런타임 CRUD 명령 핸들러 (REQ-AIRPUR-001-02-11)
// ---------------------------------------------------------------------------

// handleAddStation 는 add_station 명령을 처리한다 (station→line, 표시명, 정렬 upsert).
// 저장소 경로가 설정된 경우 write-through 로 영속화된다.
func (a *AirPurifierAgent) handleAddStation(req processRequest) ([]byte, error) {
	if req.Station == "" {
		return nil, fmt.Errorf("%w: add_station requires station", ErrInvalidCommand)
	}
	entry := StationRegistryEntry{
		Station:     req.Station,
		Line:        req.Line,
		DisplayName: req.DisplayName,
		Order:       req.Order,
	}
	if err := a.stations.UpsertStation(entry); err != nil {
		return nil, err
	}

	a.sendEvent("station_registered", map[string]any{"station": req.Station, "line": req.Line})

	return json.Marshal(map[string]any{
		"status":  "ok",
		"station": req.Station,
		"command": "add_station",
	})
}

// handleRemoveStation 는 remove_station 명령을 처리한다. 미등록 station 은 ErrStationNotFound.
func (a *AirPurifierAgent) handleRemoveStation(req processRequest) ([]byte, error) {
	if req.Station == "" {
		return nil, fmt.Errorf("%w: remove_station requires station", ErrInvalidCommand)
	}
	if err := a.stations.RemoveStation(req.Station); err != nil {
		return nil, err
	}

	a.sendEvent("station_unregistered", map[string]any{"station": req.Station})

	return json.Marshal(map[string]any{
		"status":  "ok",
		"station": req.Station,
		"command": "remove_station",
	})
}

// handleListStations 는 list_stations 명령을 처리한다. 전체 역사를 Order 순으로 반환한다.
func (a *AirPurifierAgent) handleListStations() ([]byte, error) {
	entries := a.stations.ListStations() // Order 오름차순 정렬된 값 복사본.
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]any{
			"station":      e.Station,
			"line":         e.Line,
			"display_name": e.DisplayName,
			"order":        e.Order,
		})
	}
	return json.Marshal(map[string]any{"status": "ok", "stations": out})
}

// ---------------------------------------------------------------------------
// 셀렉터 타겟 도출 시임 (B4 소비, REQ-AIRPUR-001-02-13)
// ---------------------------------------------------------------------------

// DevicesByStation 은 로스터에서 Station 이 일치하는 device_id 목록을 도출한다 (정렬).
// 로스터 락(mu) 하에 스냅샷한다. station 계층은 역사 레지스트리가 SSOT 이지만, 어떤
// 디바이스가 그 station 에 소속됐는지는 로스터 속성(Device.Station)에서 도출한다.
func (a *AirPurifierAgent) DevicesByStation(station string) []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var ids []string
	for id, dev := range a.devices {
		if dev.Station == station {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// DevicesByLine 은 line(호선) 셀렉터 대상 device_id 목록을 역사 레지스트리를 SSOT 로
// 도출한다 (REQ-AIRPUR-001-04-06). 반환:
//
//   - targets: line 에 속한 등록 station 들의 디바이스 union (device_id 정렬).
//   - excluded: Station 을 참조하지만 레지스트리에 미등록이라 대상에서 제외된 device_id
//     목록 (REQ-AIRPUR-001-02-13 집계 표기용). Station 이 빈 디바이스(위치 미지정)는
//     제외 목록에 포함하지 않는다(미등록 참조가 아니라 참조 자체가 없음).
//
// 락 규율: 먼저 역사 레지스트리(자체 락)에서 line 소속 station 집합과 전체 등록 station
// 집합을 스냅샷하고, 레지스트리 락 해제 후 로스터 락(mu)을 취득한다 — 락을 중첩하지 않는다.
func (a *AirPurifierAgent) DevicesByLine(line string) (targets, excluded []string) {
	// 1) 역사 레지스트리 스냅샷 (레지스트리 자체 락, 로스터 락과 분리).
	lineSet := make(map[string]struct{})
	for _, e := range a.stations.StationsByLine(line) {
		lineSet[e.Station] = struct{}{}
	}
	registeredSet := make(map[string]struct{})
	for _, e := range a.stations.ListStations() {
		registeredSet[e.Station] = struct{}{}
	}

	// 2) 로스터 스냅샷 (레지스트리 락 해제 후 로스터 락 취득 — 중첩 금지).
	a.mu.RLock()
	for id, dev := range a.devices {
		if dev.Station == "" {
			continue // 위치 미지정: 미등록 참조가 아님.
		}
		if _, inLine := lineSet[dev.Station]; inLine {
			targets = append(targets, id)
			continue
		}
		if _, registered := registeredSet[dev.Station]; !registered {
			excluded = append(excluded, id) // 미등록 station 참조 → 대상 제외 (표기 대상).
		}
		// 등록됐으나 다른 호선: 정상 비일치 — 대상도 제외 목록도 아님.
	}
	a.mu.RUnlock()

	sort.Strings(targets)
	sort.Strings(excluded)
	return targets, excluded
}
