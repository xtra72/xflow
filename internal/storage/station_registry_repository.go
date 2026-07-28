package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// StationRegistryEntry 는 역사(station) → 호선(line) 매핑 레지스트리의 영속 항목이다
// (REQ-AIRPUR-001-02-09). device_metadata 와 별개 저장소로 관리되며(REQ-AIRPUR-001-02-12),
// line 은 오직 이 레지스트리에만 SSOT 로 존재한다.
type StationRegistryEntry struct {
	// Station 은 역사 식별자(레지스트리 기본 키, 디바이스 Station 이 참조)이다.
	Station string `json:"station"`
	// Line 은 소속 호선 식별자 — station 의 상위 집계 레벨이다.
	Line string `json:"line"`
	// DisplayName 은 역사 표시명(UI/대시보드 소비용)이다.
	DisplayName string `json:"display_name"`
	// Order 는 라인맵 상 역사 정렬 위치(호선 내 순서)이다.
	Order int `json:"order"`
}

// StationRegistryRepository 는 역사 레지스트리 영속화 인터페이스이다.
type StationRegistryRepository interface {
	// Save 는 station 엔트리를 upsert 한다.
	Save(ctx context.Context, station string, entry StationRegistryEntry) error

	// Get 은 station 엔트리를 조회한다. 없으면 zero 값 + nil 에러를 반환한다.
	Get(ctx context.Context, station string) (StationRegistryEntry, error)

	// Delete 는 station 엔트리를 제거한다.
	Delete(ctx context.Context, station string) error

	// List 는 저장된 전체 엔트리를 station -> entry 맵으로 반환한다.
	List(ctx context.Context) (map[string]StationRegistryEntry, error)

	// Close 는 리소스를 해제한다.
	Close() error
}

// StationRegistryFileRepository 는 StationRegistryRepository 의 파일 기반 구현이다.
// device_metadata 패턴을 미러한다: 단일 JSON 파일(atomic write) + 인메모리 캐시.
type StationRegistryFileRepository struct {
	filePath string
	mu       sync.RWMutex
	cache    map[string]StationRegistryEntry // 인메모리 캐시
}

// 컴파일 타임 인터페이스 체크.
var _ StationRegistryRepository = (*StationRegistryFileRepository)(nil)

// NewStationRegistryFileRepository 는 파일 기반 역사 레지스트리 저장소를 생성한다.
// 파일은 {dir}/station_registry.json 에 저장되며 디렉터리가 없으면 생성된다.
func NewStationRegistryFileRepository(dir string) (*StationRegistryFileRepository, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create station registry directory: %w", err)
	}

	r := &StationRegistryFileRepository{
		filePath: filepath.Join(dir, "station_registry.json"),
		cache:    make(map[string]StationRegistryEntry),
	}

	// 기존 레지스트리를 파일에서 로드한다.
	if err := r.loadFromFile(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load station registry: %w", err)
	}

	return r, nil
}

// Save 는 station 엔트리를 upsert 한다.
func (r *StationRegistryFileRepository) Save(_ context.Context, station string, entry StationRegistryEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cache[station] = entry
	return r.saveToFile()
}

// Get 은 station 엔트리를 조회한다. 없으면 zero 값 + nil 에러를 반환한다.
func (r *StationRegistryFileRepository) Get(_ context.Context, station string) (StationRegistryEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.cache[station]
	if !ok {
		return StationRegistryEntry{}, nil
	}
	return entry, nil
}

// Delete 는 station 엔트리를 제거한다.
func (r *StationRegistryFileRepository) Delete(_ context.Context, station string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.cache, station)
	return r.saveToFile()
}

// List 는 저장된 전체 엔트리를 반환한다.
func (r *StationRegistryFileRepository) List(_ context.Context) (map[string]StationRegistryEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]StationRegistryEntry, len(r.cache))
	for k, v := range r.cache {
		result[k] = v
	}
	return result, nil
}

// Close 는 리소스를 해제한다. 파일 기반 저장소에서는 no-op 이다.
func (r *StationRegistryFileRepository) Close() error {
	return nil
}

// loadFromFile 은 JSON 파일에서 캐시로 레지스트리를 읽어들인다.
func (r *StationRegistryFileRepository) loadFromFile() error {
	data, err := os.ReadFile(r.filePath)
	if err != nil {
		return err
	}

	if len(data) == 0 {
		return nil
	}

	return json.Unmarshal(data, &r.cache)
}

// saveToFile 은 캐시를 JSON 파일에 atomic 하게 기록한다(tmp write + rename).
func (r *StationRegistryFileRepository) saveToFile() error {
	data, err := json.MarshalIndent(r.cache, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal station registry: %w", err)
	}

	tmpPath := r.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp station registry file: %w", err)
	}

	if err := os.Rename(tmpPath, r.filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp station registry file: %w", err)
	}

	return nil
}
