// Package storage 의 device_id_repository.go 는 (agentName, unitID) → UUID
// 매핑을 영속화한다 (v0.18.6 — SPEC-CENTURY-HVACR-001 / SPEC-SAMSUNG-HVACR-001 등 5 HVAC).
//
// 목적: 각 HVAC 에이전트의 디바이스에 글로벌 고유 UUID 를 부여하여, 프로토콜
// 식별자 (unit_id: idu-1, 0x3B 등) 와 분리한다. 재시작 후에도 동일한 device_id
// 가 유지되어 다운스트림 (대시보드 / TSDB / 알람) 의 추적이 안정적.
//
// 키 형식: "<agentName>:<unitID>" (예: "lg_hvacr01-bus1:idu-1", "century_hvacr01:0x3b")
// 값: UUID v4 문자열 (예: "550e8400-e29b-41d4-a716-446655440000")

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"

	"github.com/xtra/xflow/internal/agent"
)

// DeviceIDRepository 는 (agentName, unitID) → device_id (UUID) 매핑을 제공한다.
type DeviceIDRepository interface {
	// GetOrCreate 는 (agentName, unitID) 의 UUID 를 반환한다. 존재하지 않으면
	// 새 UUID v4 를 생성·영속하고 반환한다.
	GetOrCreate(ctx context.Context, agentName, unitID string) (string, error)

	// Get 은 (agentName, unitID) 의 UUID 를 반환한다. 존재하지 않으면 빈 문자열.
	Get(ctx context.Context, agentName, unitID string) (string, error)

	// Set 은 (agentName, unitID) 에 지정 deviceID 를 등록·영속한다.
	// 지정 deviceID 가 이미 다른 (agentName, unitID) 에 배정돼 있으면
	// agent.ErrDeviceIDConflict(래핑)를 반환한다. 같은 키에 같은 값 재지정은 idempotent.
	Set(ctx context.Context, agentName, unitID, deviceID string) error

	// Delete 는 (agentName, unitID) 매핑을 제거한다.
	Delete(ctx context.Context, agentName, unitID string) error

	// List 는 모든 매핑을 반환한다 ("<agentName>:<unitID>" → UUID).
	List(ctx context.Context) (map[string]string, error)

	// Close 는 리소스를 정리한다.
	Close() error
}

// DeviceIDFileRepository 는 단일 JSON 파일에 매핑을 저장한다.
// 메모리 캐시 + 갱신 시 파일 전체 재기록 (atomic write 패턴).
type DeviceIDFileRepository struct {
	filePath string
	mu       sync.RWMutex
	cache    map[string]string
}

var _ DeviceIDRepository = (*DeviceIDFileRepository)(nil)

// NewDeviceIDFileRepository 는 파일 기반 저장소를 생성한다.
// 파일 경로: {dir}/device_ids.json. dir 가 없으면 생성한다.
// 기존 파일이 있으면 cache 로 로드, 없으면 빈 cache 로 시작.
func NewDeviceIDFileRepository(dir string) (*DeviceIDFileRepository, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create device-id directory: %w", err)
	}
	r := &DeviceIDFileRepository{
		filePath: filepath.Join(dir, "device_ids.json"),
		cache:    make(map[string]string),
	}
	if err := r.loadFromFile(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load device ids: %w", err)
	}
	return r, nil
}

// deviceIDKey 는 (agentName, unitID) 을 저장소 키로 결합한다.
func deviceIDKey(agentName, unitID string) string {
	return agentName + ":" + unitID
}

func (r *DeviceIDFileRepository) GetOrCreate(_ context.Context, agentName, unitID string) (string, error) {
	if agentName == "" || unitID == "" {
		return "", fmt.Errorf("device-id repository: empty agentName or unitID")
	}
	key := deviceIDKey(agentName, unitID)

	r.mu.RLock()
	if v, ok := r.cache[key]; ok {
		r.mu.RUnlock()
		return v, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	// double-check 패턴: 다른 고루틴이 race 로 먼저 추가했을 수 있음.
	if v, ok := r.cache[key]; ok {
		return v, nil
	}
	id := uuid.New().String()
	r.cache[key] = id
	if err := r.saveToFile(); err != nil {
		// 저장 실패 — 메모리 캐시에서도 제거 (다음 호출에서 재시도하도록).
		delete(r.cache, key)
		return "", fmt.Errorf("persist device id: %w", err)
	}
	return id, nil
}

func (r *DeviceIDFileRepository) Set(_ context.Context, agentName, unitID, deviceID string) error {
	if agentName == "" || unitID == "" || deviceID == "" {
		return fmt.Errorf("device-id repository: empty agentName, unitID, or deviceID for Set")
	}
	key := deviceIDKey(agentName, unitID)

	r.mu.Lock()
	defer r.mu.Unlock()

	// idempotent: 같은 키에 이미 같은 값이면 no-op.
	if existing, ok := r.cache[key]; ok && existing == deviceID {
		return nil
	}
	// 유일성: 지정 deviceID 가 다른 키에 배정돼 있으면 충돌.
	if err := ensureDeviceIDUnique(r.cache, key, deviceID); err != nil {
		return err
	}
	prior, had := r.cache[key]
	r.cache[key] = deviceID
	if err := r.saveToFile(); err != nil {
		// 저장 실패 — 이전 상태로 롤백 (다음 시도에서 재시도하도록).
		if had {
			r.cache[key] = prior
		} else {
			delete(r.cache, key)
		}
		return fmt.Errorf("persist device id (Set): %w", err)
	}
	return nil
}

func (r *DeviceIDFileRepository) Get(_ context.Context, agentName, unitID string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cache[deviceIDKey(agentName, unitID)], nil
}

func (r *DeviceIDFileRepository) Delete(_ context.Context, agentName, unitID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := deviceIDKey(agentName, unitID)
	if _, ok := r.cache[key]; !ok {
		return nil
	}
	delete(r.cache, key)
	return r.saveToFile()
}

func (r *DeviceIDFileRepository) List(_ context.Context) (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.cache))
	for k, v := range r.cache {
		out[k] = v
	}
	return out, nil
}

func (r *DeviceIDFileRepository) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveToFile()
}

// loadFromFile 는 파일에서 cache 로 로드한다. 파일이 없으면 os.IsNotExist 에러.
func (r *DeviceIDFileRepository) loadFromFile() error {
	data, err := os.ReadFile(r.filePath)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	m := make(map[string]string)
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("unmarshal device ids: %w", err)
	}
	r.cache = m
	return nil
}

// saveToFile 는 cache 를 파일에 atomic 하게 기록한다 (tmp + rename).
// 호출자는 mu lock 을 보유해야 한다.
func (r *DeviceIDFileRepository) saveToFile() error {
	data, err := json.MarshalIndent(r.cache, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal device ids: %w", err)
	}
	tmp := r.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write tmp file: %w", err)
	}
	if err := os.Rename(tmp, r.filePath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename tmp file: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// In-memory implementation (tests / 영속 불필요 환경)
// ---------------------------------------------------------------------------

// DeviceIDMemoryRepository 는 in-memory only 구현이다. 테스트 / dev 환경 용도.
type DeviceIDMemoryRepository struct {
	mu    sync.RWMutex
	cache map[string]string
}

var _ DeviceIDRepository = (*DeviceIDMemoryRepository)(nil)

// NewDeviceIDMemoryRepository 는 메모리 기반 저장소를 생성한다.
func NewDeviceIDMemoryRepository() *DeviceIDMemoryRepository {
	return &DeviceIDMemoryRepository{cache: make(map[string]string)}
}

func (r *DeviceIDMemoryRepository) GetOrCreate(_ context.Context, agentName, unitID string) (string, error) {
	if agentName == "" || unitID == "" {
		return "", fmt.Errorf("device-id repository: empty agentName or unitID")
	}
	key := deviceIDKey(agentName, unitID)
	r.mu.RLock()
	if v, ok := r.cache[key]; ok {
		r.mu.RUnlock()
		return v, nil
	}
	r.mu.RUnlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.cache[key]; ok {
		return v, nil
	}
	id := uuid.New().String()
	r.cache[key] = id
	return id, nil
}

func (r *DeviceIDMemoryRepository) Set(_ context.Context, agentName, unitID, deviceID string) error {
	if agentName == "" || unitID == "" || deviceID == "" {
		return fmt.Errorf("device-id repository: empty agentName, unitID, or deviceID for Set")
	}
	key := deviceIDKey(agentName, unitID)

	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.cache[key]; ok && existing == deviceID {
		return nil // idempotent
	}
	if err := ensureDeviceIDUnique(r.cache, key, deviceID); err != nil {
		return err
	}
	r.cache[key] = deviceID
	return nil
}

func (r *DeviceIDMemoryRepository) Get(_ context.Context, agentName, unitID string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cache[deviceIDKey(agentName, unitID)], nil
}

// ensureDeviceIDUnique 는 deviceID 가 targetKey 이외의 키에 이미 배정돼 있으면
// agent.ErrDeviceIDConflict(래핑)를 반환한다. 호출자는 cache 잠금을 보유해야 한다.
//
// 목적: 지정 device_id 의 전역 유일성 보장(중복 device_id 방지). 같은 키(targetKey)에
// 대한 재지정은 충돌이 아니다(호출자가 idempotent 처리 후 호출).
func ensureDeviceIDUnique(cache map[string]string, targetKey, deviceID string) error {
	for k, v := range cache {
		if k != targetKey && v == deviceID {
			return fmt.Errorf("%w: %q already assigned to %q", agent.ErrDeviceIDConflict, deviceID, k)
		}
	}
	return nil
}

func (r *DeviceIDMemoryRepository) Delete(_ context.Context, agentName, unitID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cache, deviceIDKey(agentName, unitID))
	return nil
}

func (r *DeviceIDMemoryRepository) List(_ context.Context) (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.cache))
	for k, v := range r.cache {
		out[k] = v
	}
	return out, nil
}

func (r *DeviceIDMemoryRepository) Close() error { return nil }
