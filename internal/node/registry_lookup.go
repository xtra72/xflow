package node

// registry_lookup.go (message-slim-metadata / enrich) 는 노드 레이어에서
// agent / device 의 정규 부가 정보(type/name)를 id 로 조회하는 최소 read-only
// 룩업 인터페이스와 주입 옵션을 정의한다.
//
// 목적: slim-expand(WS egress 재수화)와 in-flow enrich 노드가 동일한 룩업 경로를
// 공유하도록, 노드 레이어에 단일 조회 계약을 둔다. 실제 데이터 소스는
// agent.Registry / device.DeviceRegistry 이며, cmd/xflowd 가 이를 감싼 함수형
// resolver 로 주입한다(inventory 노드의 WithDeviceRegistryFunc 패턴과 동일).
//
// 레이어링: 인터페이스는 순수 값 타입(RegistryMeta)만 노출하므로 소비 측이
// agent/device 구체 타입에 결합하지 않는다. internal/node 는 이미 internal/agent 를
// import 하지만, enrich/expression 경로는 이 최소 인터페이스만 사용해 결합을 줄인다.

// RegistryMeta 는 agent / device 의 정규 식별 정보 요약이다.
// 세 필드 모두 문자열이며, 조회 실패 시 소비 측은 zero-value 를 사용하지 않고
// (meta, false) 의 false 로 분기한다.
type RegistryMeta struct {
	// Type 은 agent/device 의 타입 문자열이다 (예: "serial", "HVACR.IDU").
	Type string
	// ID 는 정규 식별자(UUID)이다.
	ID string
	// Name 은 사용자 라벨 또는 기본 생성명이다.
	Name string
}

// AgentInfoLookup 은 agentID 로 agent 부가 정보를 조회하는 read-only 인터페이스이다.
type AgentInfoLookup interface {
	// LookupAgent 는 agentID 에 해당하는 RegistryMeta 를 반환한다.
	// 매칭이 없으면 (zero, false) 를 반환한다.
	LookupAgent(id string) (RegistryMeta, bool)
}

// DeviceInfoLookup 은 deviceID 로 device 부가 정보를 조회하는 read-only 인터페이스이다.
type DeviceInfoLookup interface {
	// LookupDevice 는 deviceID 에 해당하는 RegistryMeta 를 반환한다.
	// 매칭이 없으면 (zero, false) 를 반환한다.
	LookupDevice(id string) (RegistryMeta, bool)
}

// ---------------------------------------------------------------------------
// 함수형 어댑터 — 클로저를 인터페이스로 승격
// ---------------------------------------------------------------------------

// AgentLookupFunc 는 함수를 AgentInfoLookup 으로 승격하는 어댑터이다.
type AgentLookupFunc func(id string) (RegistryMeta, bool)

// LookupAgent 는 AgentInfoLookup 을 구현한다.
func (f AgentLookupFunc) LookupAgent(id string) (RegistryMeta, bool) {
	if f == nil {
		return RegistryMeta{}, false
	}
	return f(id)
}

// DeviceLookupFunc 는 함수를 DeviceInfoLookup 으로 승격하는 어댑터이다.
type DeviceLookupFunc func(id string) (RegistryMeta, bool)

// LookupDevice 는 DeviceInfoLookup 을 구현한다.
func (f DeviceLookupFunc) LookupDevice(id string) (RegistryMeta, bool) {
	if f == nil {
		return RegistryMeta{}, false
	}
	return f(id)
}

// ---------------------------------------------------------------------------
// 노드 옵션 주입 (inventory 의 WithDeviceRegistryFunc 패턴과 동일한 config-key 방식)
// ---------------------------------------------------------------------------

const (
	// enrichAgentLookupKey 는 NodeOption 으로 주입된 AgentInfoLookup 의 config 키이다.
	enrichAgentLookupKey = "_enrich_agent_lookup"
	// enrichDeviceLookupKey 는 NodeOption 으로 주입된 DeviceInfoLookup 의 config 키이다.
	enrichDeviceLookupKey = "_enrich_device_lookup"
)

// WithAgentInfoLookup 은 agent 부가 정보 룩업을 노드에 주입하는 옵션이다.
// enrich 노드 및 expression 빌트인(agentInfo)이 이를 사용한다.
func WithAgentInfoLookup(lookup AgentInfoLookup) NodeOption {
	return func(b *BaseNode) {
		if b.config == nil {
			b.config = make(map[string]any)
		}
		b.config[enrichAgentLookupKey] = lookup
	}
}

// WithDeviceInfoLookup 은 device 부가 정보 룩업을 노드에 주입하는 옵션이다.
// enrich 노드 및 expression 빌트인(deviceInfo)이 이를 사용한다.
func WithDeviceInfoLookup(lookup DeviceInfoLookup) NodeOption {
	return func(b *BaseNode) {
		if b.config == nil {
			b.config = make(map[string]any)
		}
		b.config[enrichDeviceLookupKey] = lookup
	}
}

// agentLookupFromConfig 는 BaseNode.config 에서 주입된 AgentInfoLookup 을 추출한다.
// 미주입이면 (nil, false).
func agentLookupFromConfig(cfg map[string]any) (AgentInfoLookup, bool) {
	if cfg == nil {
		return nil, false
	}
	v, ok := cfg[enrichAgentLookupKey]
	if !ok {
		return nil, false
	}
	l, ok := v.(AgentInfoLookup)
	return l, ok
}

// deviceLookupFromConfig 는 BaseNode.config 에서 주입된 DeviceInfoLookup 을 추출한다.
// 미주입이면 (nil, false).
func deviceLookupFromConfig(cfg map[string]any) (DeviceInfoLookup, bool) {
	if cfg == nil {
		return nil, false
	}
	v, ok := cfg[enrichDeviceLookupKey]
	if !ok {
		return nil, false
	}
	l, ok := v.(DeviceInfoLookup)
	return l, ok
}
