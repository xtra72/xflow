package device

import (
	"context"
	"sync"
)

// DeviceRegistry is the central registry for all devices across all agents.
type DeviceRegistry interface {
	// List returns devices matching the filter criteria.
	List(filter DeviceFilter) []Device

	// Get returns a specific device by its global ID (composite "agent:local_id").
	//
	// Note: Phase B (SPEC-DEVICE-IDENTITY-001) introduces GetByUID and
	// GetByAgentName as 1급 lookup paths. Existing Get(id) semantics is
	// preserved unchanged for backward compatibility.
	Get(id string) (Device, error)

	// GetByUID 는 Device.UID() == uid 인 디바이스를 검색한다 (Phase B).
	// UUID 가 빈 문자열이거나 매칭 없으면 ErrDeviceNotFound.
	//
	// SPEC-DEVICE-IDENTITY-001 § M4 — UUID resolver 의 표준 진입점.
	GetByUID(uid string) (Device, error)

	// GetByAgentName 은 (agent, name) 쌍으로 디바이스를 검색한다 (Phase B).
	// 어느 한 쪽이 빈 문자열이거나 매칭 없으면 ErrDeviceNotFound.
	//
	// SPEC-DEVICE-IDENTITY-001 § M4 — name 기반 명시 resolver 의 표준 진입점.
	GetByAgentName(agent, name string) (Device, error)

	// ResolveDevice 는 참조 문자열의 형식을 자동 판단하여 디바이스를 검색한다.
	// UUID v4 또는 "agent/name" 만 허용한다.
	//
	// SPEC-DEVICE-IDENTITY-001 Phase D (xflowd v1.0 — D-T2): composite
	// ("agent:local_id") 형식은 더 이상 지원하지 않는다. ClassifyDeviceRef 가
	// DeviceRefUnknown 으로 분류 → ErrDeviceNotFound 반환.
	//
	// 두 번째 반환값 kind 는 매칭에 사용된 형식이다 (호출자가 진단/로깅에 활용).
	ResolveDevice(ref string) (Device, DeviceRefKind, error)

	// Count returns the total number of registered devices.
	Count() int

	// RegisterProvider registers a device provider for an agent.
	// If a provider is already registered for the agent, it is replaced
	// and any offline status is cleared.
	RegisterProvider(agentName string, provider DeviceProvider)

	// UnregisterProvider marks all devices from the agent as offline.
	// The provider is not removed; devices still appear in List but show Online() == false.
	UnregisterProvider(agentName string)

	// SetMetadata sets user-defined metadata for a device.
	// Returns ErrDeviceNotFound if the device does not exist in any provider.
	SetMetadata(id string, metadata DeviceMetadata) error

	// GetMetadata returns user-defined metadata for a device.
	// Returns empty DeviceMetadata if no metadata has been set (not an error).
	GetMetadata(id string) (DeviceMetadata, error)

	// Execute runs a command on a controllable device.
	// Returns ErrDeviceNotFound if the device does not exist,
	// ErrAgentStopped if the owning agent is offline,
	// ErrNotControllable if the device does not implement ControllableDevice.
	Execute(ctx context.Context, id string, command string, params map[string]any) (map[string]any, error)
}

// offlineDeviceWrapper wraps a Device and overrides Online() to return false.
// This is used for devices from agents that have been unregistered (marked offline).
type offlineDeviceWrapper struct {
	Device
}

func (w *offlineDeviceWrapper) Online() bool {
	return false
}

func (w *offlineDeviceWrapper) State() DeviceState {
	state := w.Device.State()
	state.Online = false
	return state
}

// HistoryComparisonProperties / HistoryEventTimeMs 는 내부 Device 의 선택적 구현을
// 그대로 위임한다.
//
// 위임이 필요한 이유: 이 래퍼는 **인터페이스를 임베드**하므로 승격되는 메서드 집합이
// device.Device 로 한정된다. 위임하지 않으면 에이전트가 오프라인으로 표시되는 순간
// 프로바이더의 이력 힌트(비교 표면/이벤트 시각)가 타입 단언에서 조용히 사라져,
// 같은 디바이스가 온라인일 때와 다른 이력 동작을 보인다.
//
// 내부 Device 가 해당 인터페이스를 구현하지 않으면 "의견 없음"(false)을 반환하며,
// 레코더는 미구현 프로바이더와 동일한 폴백 경로를 탄다.
func (w *offlineDeviceWrapper) HistoryComparisonProperties() (map[string]any, bool) {
	if hc, ok := w.Device.(HistoryComparable); ok {
		return hc.HistoryComparisonProperties()
	}
	return nil, false
}

func (w *offlineDeviceWrapper) HistoryEventTimeMs() (int64, bool) {
	if he, ok := w.Device.(HistoryEventTimed); ok {
		return he.HistoryEventTimeMs()
	}
	return 0, false
}

// inMemoryRegistry is the default in-memory implementation of DeviceRegistry.
// It uses a pull model: List() and Get() query providers directly each time.
type inMemoryRegistry struct {
	mu            sync.RWMutex
	providers     map[string]DeviceProvider // agent name -> provider
	offlineAgents map[string]bool           // agent name -> true if offline
	metadata      map[string]DeviceMetadata // device ID -> user-defined metadata
}

// NewRegistry creates a new in-memory DeviceRegistry.
func NewRegistry() DeviceRegistry {
	return &inMemoryRegistry{
		providers:     make(map[string]DeviceProvider),
		offlineAgents: make(map[string]bool),
		metadata:      make(map[string]DeviceMetadata),
	}
}

func (r *inMemoryRegistry) RegisterProvider(agentName string, provider DeviceProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers[agentName] = provider
	delete(r.offlineAgents, agentName)
}

func (r *inMemoryRegistry) UnregisterProvider(agentName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.offlineAgents[agentName] = true
}

func (r *inMemoryRegistry) List(filter DeviceFilter) []Device {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Device
	for agentName, provider := range r.providers {
		isOffline := r.offlineAgents[agentName]
		for _, dev := range provider.Devices() {
			d := r.wrapIfOffline(dev, isOffline)
			if filter.Matches(d) {
				result = append(result, d)
			}
		}
	}
	return result
}

func (r *inMemoryRegistry) Get(id string) (Device, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	dev, agentName, found := r.findByID(id)
	if !found {
		return nil, ErrDeviceNotFound
	}
	return r.wrapIfOffline(dev, r.offlineAgents[agentName]), nil
}

// findByID 는 모든 provider 의 Devices() 를 순회하며 Device.ID() == id 인
// 디바이스를 찾는다. SPEC-DEVICE-IDENTITY-001 Phase D § D-T1 이후 ID 는 UUID
// 이므로, 각 provider 의 Device(id) 가 여전히 composite format ("agentName:address")
// 을 기대하더라도 registry 차원에서 UUID lookup 이 동작하도록 한다.
//
// Must be called with lock held.
func (r *inMemoryRegistry) findByID(id string) (Device, string, bool) {
	if id == "" {
		return nil, "", false
	}
	for agentName, provider := range r.providers {
		for _, d := range provider.Devices() {
			if d.ID() == id {
				return d, agentName, true
			}
		}
	}
	return nil, "", false
}

func (r *inMemoryRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, provider := range r.providers {
		count += len(provider.Devices())
	}
	return count
}

// SetMetadata 는 device 의 사용자 정의 메타데이터를 저장한다.
//
// 디바이스 존재 여부는 검증하지 않는다 (2026-05-27 변경): auto-discovered
// 디바이스는 서버 부팅 시점엔 아직 발견되지 않을 수 있으므로, 영속 저장소에서
// 메타데이터를 pre-load 할 때 ErrDeviceNotFound 로 실패하던 race condition
// 회피. 디바이스 미존재 시점에 metadata 만 미리 등록되어도 추후 디바이스가
// 발견되면 GetMetadata 로 정상 조회된다.
//
// API 핸들러 (PUT /devices/{id}/metadata) 는 URL 의 {id} 가 실재 디바이스인지
// 별도 검증할 수 있다 (현재는 명시 검증하지 않음 — 인증된 사용자 신뢰).
func (r *inMemoryRegistry) SetMetadata(id string, metadata DeviceMetadata) error {
	if id == "" {
		return ErrDeviceNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metadata[id] = metadata
	return nil
}

func (r *inMemoryRegistry) GetMetadata(id string) (DeviceMetadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	meta, ok := r.metadata[id]
	if !ok {
		return DeviceMetadata{}, nil
	}
	return meta, nil
}

func (r *inMemoryRegistry) Execute(ctx context.Context, id string, command string, params map[string]any) (map[string]any, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	dev, agentName, found := r.findByID(id)
	if !found {
		return nil, ErrDeviceNotFound
	}

	// Check if the agent is offline.
	if r.offlineAgents[agentName] {
		return nil, ErrAgentStopped
	}

	// Check if the device is controllable.
	controllable, ok := dev.(ControllableDevice)
	if !ok {
		return nil, ErrNotControllable
	}

	return controllable.Execute(ctx, command, params)
}

// wrapIfOffline wraps a device with offlineDeviceWrapper if the agent is offline.
func (r *inMemoryRegistry) wrapIfOffline(dev Device, isOffline bool) Device {
	if isOffline {
		return &offlineDeviceWrapper{Device: dev}
	}
	return dev
}

// deviceExists checks if a device with the given ID exists in any provider.
// Must be called with lock held.
//
// SPEC-DEVICE-IDENTITY-001 Phase D § D-T1: ID 가 UUID 이므로 provider 의
// composite-key 기반 Device(id) 대신 findByID 로 통일.
func (r *inMemoryRegistry) deviceExists(id string) bool {
	_, _, found := r.findByID(id)
	return found
}
