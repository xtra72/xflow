package device

import (
	"context"
	"sync"
)

// DeviceRegistry is the central registry for all devices across all agents.
type DeviceRegistry interface {
	// List returns devices matching the filter criteria.
	List(filter DeviceFilter) []Device

	// Get returns a specific device by its global ID.
	Get(id string) (Device, error)

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

	for agentName, provider := range r.providers {
		dev, err := provider.Device(id)
		if err != nil {
			continue
		}
		isOffline := r.offlineAgents[agentName]
		return r.wrapIfOffline(dev, isOffline), nil
	}
	return nil, ErrDeviceNotFound
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

func (r *inMemoryRegistry) SetMetadata(id string, metadata DeviceMetadata) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.deviceExists(id) {
		return ErrDeviceNotFound
	}
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

	for agentName, provider := range r.providers {
		dev, err := provider.Device(id)
		if err != nil {
			continue
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
	return nil, ErrDeviceNotFound
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
func (r *inMemoryRegistry) deviceExists(id string) bool {
	for _, provider := range r.providers {
		if _, err := provider.Device(id); err == nil {
			return true
		}
	}
	return false
}
