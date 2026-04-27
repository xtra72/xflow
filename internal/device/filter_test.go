package device

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// newTestDevice creates a mockDevice with the given parameters for filter testing.
func newTestDevice(id, name string, dt DeviceType, protocol, agentName string, online bool, meta DeviceMetadata) *mockDevice {
	return &mockDevice{
		id:         id,
		name:       name,
		deviceType: dt,
		protocol:   protocol,
		agentName:  agentName,
		online:     online,
		lastSeen:   time.Now(),
		state: DeviceState{
			Online: online,
		},
		metadata: meta,
	}
}

func TestDeviceFilterMatches(t *testing.T) {
	onlineDevice := newTestDevice(
		"nasa-agent:20.01.00",
		"Indoor Unit 1",
		DeviceTypeIndoor,
		"nasa",
		"nasa-agent",
		true,
		DeviceMetadata{
			Tags:     []string{"hvac", "lobby"},
			Location: "1F Lobby",
			Group:    "1F",
			Labels:   map[string]string{"zone": "public"},
		},
	)

	offlineDevice := newTestDevice(
		"modbus-agent:sensor-01",
		"Temperature Sensor",
		DeviceTypeSensor,
		"modbus",
		"modbus-agent",
		false,
		DeviceMetadata{
			Tags:  []string{"sensor", "outdoor"},
			Group: "outdoor",
		},
	)

	controllerDevice := newTestDevice(
		"nasa-agent:00.01.00",
		"Main Controller",
		DeviceTypeController,
		"nasa",
		"nasa-agent",
		true,
		DeviceMetadata{
			Tags:  []string{"controller"},
			Group: "1F",
		},
	)

	boolTrue := true
	boolFalse := false

	tests := []struct {
		name    string
		filter  DeviceFilter
		device  Device
		matches bool
	}{
		{
			name:    "empty filter matches any device",
			filter:  DeviceFilter{},
			device:  onlineDevice,
			matches: true,
		},
		{
			name:    "empty filter matches offline device",
			filter:  DeviceFilter{},
			device:  offlineDevice,
			matches: true,
		},
		{
			name:    "protocol filter matches",
			filter:  DeviceFilter{Protocol: "nasa"},
			device:  onlineDevice,
			matches: true,
		},
		{
			name:    "protocol filter does not match",
			filter:  DeviceFilter{Protocol: "modbus"},
			device:  onlineDevice,
			matches: false,
		},
		{
			name:    "agent name filter matches",
			filter:  DeviceFilter{AgentName: "nasa-agent"},
			device:  onlineDevice,
			matches: true,
		},
		{
			name:    "agent name filter does not match",
			filter:  DeviceFilter{AgentName: "modbus-agent"},
			device:  onlineDevice,
			matches: false,
		},
		{
			name:    "type filter matches",
			filter:  DeviceFilter{Type: "indoor"},
			device:  onlineDevice,
			matches: true,
		},
		{
			name:    "type filter does not match",
			filter:  DeviceFilter{Type: "sensor"},
			device:  onlineDevice,
			matches: false,
		},
		{
			name:    "online filter true matches online device",
			filter:  DeviceFilter{Online: &boolTrue},
			device:  onlineDevice,
			matches: true,
		},
		{
			name:    "online filter true does not match offline device",
			filter:  DeviceFilter{Online: &boolTrue},
			device:  offlineDevice,
			matches: false,
		},
		{
			name:    "online filter false matches offline device",
			filter:  DeviceFilter{Online: &boolFalse},
			device:  offlineDevice,
			matches: true,
		},
		{
			name:    "online filter false does not match online device",
			filter:  DeviceFilter{Online: &boolFalse},
			device:  onlineDevice,
			matches: false,
		},
		{
			name:    "nil online filter matches any device",
			filter:  DeviceFilter{Online: nil},
			device:  offlineDevice,
			matches: true,
		},
		{
			name:    "tags filter matches when device has all tags",
			filter:  DeviceFilter{Tags: []string{"hvac"}},
			device:  onlineDevice,
			matches: true,
		},
		{
			name:    "tags filter matches when device has multiple required tags",
			filter:  DeviceFilter{Tags: []string{"hvac", "lobby"}},
			device:  onlineDevice,
			matches: true,
		},
		{
			name:    "tags filter does not match when device is missing a tag",
			filter:  DeviceFilter{Tags: []string{"hvac", "missing"}},
			device:  onlineDevice,
			matches: false,
		},
		{
			name:    "tags filter with empty slice matches any device",
			filter:  DeviceFilter{Tags: []string{}},
			device:  offlineDevice,
			matches: true,
		},
		{
			name:    "group filter matches",
			filter:  DeviceFilter{Group: "1F"},
			device:  onlineDevice,
			matches: true,
		},
		{
			name:    "group filter does not match",
			filter:  DeviceFilter{Group: "2F"},
			device:  onlineDevice,
			matches: false,
		},
		{
			name: "multiple filters AND'd together - all match",
			filter: DeviceFilter{
				Protocol:  "nasa",
				AgentName: "nasa-agent",
				Type:      "indoor",
				Online:    &boolTrue,
				Tags:      []string{"hvac"},
				Group:     "1F",
			},
			device:  onlineDevice,
			matches: true,
		},
		{
			name: "multiple filters AND'd together - one does not match",
			filter: DeviceFilter{
				Protocol:  "nasa",
				AgentName: "nasa-agent",
				Type:      "indoor",
				Online:    &boolTrue,
				Tags:      []string{"hvac"},
				Group:     "2F", // does not match
			},
			device:  onlineDevice,
			matches: false,
		},
		{
			name:    "controller type filter matches controller device",
			filter:  DeviceFilter{Type: "controller"},
			device:  controllerDevice,
			matches: true,
		},
		{
			name: "protocol and agent combined filter",
			filter: DeviceFilter{
				Protocol:  "modbus",
				AgentName: "modbus-agent",
			},
			device:  offlineDevice,
			matches: true,
		},
		{
			name: "tags filter does not match device with no tags",
			filter: DeviceFilter{
				Tags: []string{"nonexistent"},
			},
			device: newTestDevice(
				"test:no-tags",
				"No Tags Device",
				DeviceTypeUnknown,
				"test",
				"test-agent",
				true,
				DeviceMetadata{},
			),
			matches: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.Matches(tt.device)
			assert.Equal(t, tt.matches, result)
		})
	}
}
