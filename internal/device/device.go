// Package device provides a unified, protocol-agnostic device model and interfaces
// for managing IoT devices across multiple protocol agents.
package device

import "time"

// DeviceType represents the type of device.
type DeviceType string

const (
	// DeviceTypeIndoor represents an indoor HVAC unit (v0.18.3: "HVACR.IDU").
	DeviceTypeIndoor DeviceType = "HVACR.IDU"

	// DeviceTypeOutdoor represents an outdoor HVAC unit (v0.18.3: "HVACR.ODU").
	DeviceTypeOutdoor DeviceType = "HVACR.ODU"

	// DeviceTypeController represents a controller device.
	DeviceTypeController DeviceType = "controller"

	// DeviceTypeSensor represents a sensor device.
	DeviceTypeSensor DeviceType = "sensor"

	// DeviceTypeActuator represents an actuator device.
	DeviceTypeActuator DeviceType = "actuator"

	// DeviceTypeUnknown represents a device with an unknown type.
	DeviceTypeUnknown DeviceType = "unknown"
)

// Device is the unified interface for all protocol-agnostic devices.
// Every protocol-specific device (NASA, Modbus, MQTT, etc.) must be
// adaptable to this interface.
type Device interface {
	// ID returns the globally unique, immutable UUID v4 for this device.
	//
	// SPEC-DEVICE-IDENTITY-001 Phase D (xflowd v1.0 — D-T1, Breaking):
	// ID() returns the same UUID as UID(). The legacy composite key
	// ("agent_name:local_id") has been fully removed. Callers that need
	// the human-readable agent/name pair should use AgentName() + Name()
	// directly.
	//
	// The UUID is resolved via agent.ResolveDeviceID(ctx, agentName, localID)
	// and is guaranteed stable across:
	//   - agent renames (UUID stays the same regardless of agent rename)
	//   - process restarts (DeviceIDRepository is persistent)
	//   - device cache rebuilds (idempotent GetOrCreate)
	//
	// Returns an empty string only when DeviceIDRepository is not configured
	// or the (agentName, localID) mapping cannot be resolved.
	ID() string

	// UID returns the globally unique, immutable UUID v4 for this device.
	//
	// SPEC-DEVICE-IDENTITY-001 Phase D (xflowd v1.0 — D-T1): UID() and ID()
	// now return the same UUID. UID() is retained as the explicit, semantically
	// unambiguous accessor for new code.
	//
	// The UUID is resolved via agent.ResolveDeviceID(ctx, agentName, localID)
	// and is guaranteed stable across:
	//   - agent renames (composite ID changes, UUID stays the same)
	//   - process restarts (DeviceIDRepository is persistent)
	//   - device cache rebuilds (idempotent GetOrCreate)
	//
	// Returns an empty string only when DeviceIDRepository is not configured
	// or the mapping cannot be resolved. Callers MUST treat the empty string
	// as "UID unavailable" and omit any downstream "uid" field (graceful
	// degradation contract).
	//
	// See SPEC-DEVICE-IDENTITY-001 § M1 for the full identity contract.
	UID() string

	// Name returns the user-defined name for this device.
	Name() string

	// Type returns the device type (indoor, outdoor, controller, sensor, actuator).
	Type() DeviceType

	// Protocol returns the protocol name (nasa, modbus, mqtt, etc.).
	Protocol() string

	// AgentName returns the name of the owning agent.
	AgentName() string

	// Online returns whether the device is currently online.
	Online() bool

	// LastSeen returns the last communication time.
	LastSeen() time.Time

	// State returns the current device state.
	State() DeviceState

	// Metadata returns user-defined metadata for this device.
	Metadata() DeviceMetadata

	// Source returns the device origin: "config" (from config file) or "auto" (discovered).
	Source() string

	// Capabilities returns the list of supported capabilities.
	Capabilities() []string
}

// DeviceState holds the current state of a device. It separates common fields
// from protocol-specific properties stored in the Properties map.
type DeviceState struct {
	Online     bool           `json:"online"`
	Ready      bool           `json:"ready"`
	LastSeen   time.Time      `json:"last_seen"`
	ErrorCount int            `json:"error_count"`
	Properties map[string]any `json:"properties"` // Protocol-specific properties
}

// DeviceMetadata holds user-defined metadata such as name, tags, location, and labels.
type DeviceMetadata struct {
	Name     string            `json:"name"` // 사용자 정의 디바이스 이름
	Tags     []string          `json:"tags"`
	Location string            `json:"location"`
	Group    string            `json:"group"`
	Labels   map[string]string `json:"labels"`
	Pinned   *bool             `json:"pinned,omitempty"` // 고정 설치 여부 (nil = 미설정)

	// 소유 정보 — 고정 설치 디바이스를 재시작 후 **다시 발견되기 전에** 복원하기 위해
	// 저장한다.
	//
	// 메타데이터는 디바이스 UUID 로만 키잉되어 있어, 어느 에이전트의 어떤 로컬
	// 식별자인지 알 방법이 없었다. 그래서 부팅 시 복원 경로는 살아 있는 디바이스를
	// 열거해 UUID→로컬 ID 맵을 만들었는데, 그러면 **이미 발견된 디바이스만** 복원할
	// 수 있어 순환이 된다. LoRaWAN 처럼 업링크가 와야 발견되는 디바이스는 그때까지
	// 사라진 채로 남는다.
	//
	// 저장 시점(사용자가 메타데이터를 고칠 때)에는 디바이스가 살아 있으므로 소유
	// 정보를 알 수 있다. 그것을 함께 적어 두면 다음 부팅에서 참조만으로 복원된다.
	//
	// 두 필드 모두 omitempty — 이 필드가 생기기 전에 저장된 파일은 값이 비어 있고,
	// 그 경우 종전 경로(살아 있는 디바이스 열거)로 폴백한다.
	AgentName string `json:"agent_name,omitempty"` // 소유 에이전트 이름
	LocalID   string `json:"local_id,omitempty"`   // 에이전트 내부 식별자(주소/DevEUI 등)

	// 데이터 갱신 시간 제한(초). 마지막 갱신 후 이 시간이 지나면 화면에서 그 값을
	// **오래된 것으로 표시**한다.
	//
	// 에이전트의 offline 임계와는 다른 축이다. offline 임계는 "디바이스가 살아 있는가"
	// 를 판정해 online 플래그를 만들고, 이 값은 "지금 보이는 값을 현재 값으로 믿어도
	// 되는가"를 말한다. 보고 주기가 제각각인 디바이스를 한 화면에 놓으면 하나의 임계로는
	// 둘 다 맞출 수 없어, 디바이스마다 따로 잡는다.
	//
	// nil = 미설정. 이때는 아무 표시도 하지 않는다 — 지금까지의 화면이 그대로 산다.
	StaleAfterSec *int `json:"stale_after_sec,omitempty"`
}

// DeviceProvider is implemented by agents that manage devices.
// Each protocol agent (NASA, Modbus, etc.) implements this interface
// to expose its devices to the central registry.
type DeviceProvider interface {
	// Devices returns all devices managed by this provider.
	Devices() []Device

	// Device returns a specific device by its ID.
	Device(id string) (Device, error)
}
