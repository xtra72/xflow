package modbus

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/device/adapter"
)

// ModbusDeviceProvider implements device.DeviceProvider for the Modbus agent.
// It converts ModbusDevice instances to the unified Device interface using adapters.
type ModbusDeviceProvider struct {
	agent *ModbusAgent
}

// Compile-time interface check.
var _ device.DeviceProvider = (*ModbusDeviceProvider)(nil)

// NewModbusDeviceProvider creates a DeviceProvider wrapping a ModbusAgent.
func NewModbusDeviceProvider(agent *ModbusAgent) *ModbusDeviceProvider {
	return &ModbusDeviceProvider{agent: agent}
}

// Devices returns all devices managed by this agent as unified Device instances.
func (p *ModbusDeviceProvider) Devices() []device.Device {
	p.agent.mu.RLock()
	devices := make([]*ModbusDevice, len(p.agent.devices))
	copy(devices, p.agent.devices)
	caches := p.agent.caches
	agentName := p.agent.agentConfig.Name
	hasWritable := p.agent.config.EnableWriteEvents
	p.agent.mu.RUnlock()

	result := make([]device.Device, 0, len(devices))
	for _, dev := range devices {
		info := modbusDeviceToInfo(dev, caches)

		if hasWritable {
			executor := newModbusExecutor(p.agent, dev.config.ID)
			result = append(result, adapter.NewControllableModbusDevice(agentName, info, executor))
		} else {
			result = append(result, adapter.NewModbusDevice(agentName, info))
		}
	}
	return result
}

// Device returns a specific device by its global ID ("agentName:deviceID").
func (p *ModbusDeviceProvider) Device(id string) (device.Device, error) {
	p.agent.mu.RLock()
	agentName := p.agent.agentConfig.Name
	caches := p.agent.caches
	hasWritable := p.agent.config.EnableWriteEvents
	p.agent.mu.RUnlock()

	prefix := agentName + ":"
	if len(id) <= len(prefix) || id[:len(prefix)] != prefix {
		return nil, device.ErrDeviceNotFound
	}
	deviceID := id[len(prefix):]

	p.agent.mu.RLock()
	defer p.agent.mu.RUnlock()

	for _, dev := range p.agent.devices {
		if dev.config.ID == deviceID {
			info := modbusDeviceToInfo(dev, caches)
			if hasWritable {
				executor := newModbusExecutor(p.agent, dev.config.ID)
				return adapter.NewControllableModbusDevice(agentName, info, executor), nil
			}
			return adapter.NewModbusDevice(agentName, info), nil
		}
	}
	return nil, device.ErrDeviceNotFound
}

// modbusDeviceToInfo converts a ModbusDevice to adapter.ModbusDeviceInfo.
func modbusDeviceToInfo(dev *ModbusDevice, caches map[string]*RegisterCache) adapter.ModbusDeviceInfo {
	dev.mu.RLock()
	online := dev.online
	dev.mu.RUnlock()

	info := adapter.ModbusDeviceInfo{
		DeviceID: dev.config.ID,
		Host:     dev.config.Host,
		Port:     dev.config.Port,
		UnitID:   dev.config.UnitID,
		Online:   online,
		LastSeen: time.Now(), // Modbus uses polling, so last seen is approximate
		Writable: true,
	}

	// Extract register group names
	groups := make([]string, 0, len(dev.config.RegisterGroups))
	for _, rg := range dev.config.RegisterGroups {
		if rg.Name != "" {
			groups = append(groups, rg.Name)
		}
	}
	info.RegisterGroups = groups

	// Extract cached register values
	if cache, ok := caches[dev.config.ID]; ok {
		info.CacheData = cache.GetSnapshot()
	}

	return info
}

// newModbusExecutor creates a CommandExecutor that delegates to ModbusAgent.Process.
func newModbusExecutor(agent *ModbusAgent, deviceID string) adapter.CommandExecutor {
	return func(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
		req := processRequest{
			Command:  command,
			DeviceID: deviceID,
			Params:   params,
		}

		data, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("modbus executor: marshal request: %w", err)
		}

		resp, err := agent.Process(data)
		if err != nil {
			return nil, err
		}

		if resp == nil {
			return nil, nil
		}

		var result map[string]any
		if err := json.Unmarshal(resp, &result); err != nil {
			return nil, fmt.Errorf("modbus executor: unmarshal response: %w", err)
		}

		return result, nil
	}
}
