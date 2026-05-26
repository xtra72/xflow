// Package node - inventory.go: SPEC-INVENTORY-001 Inventory 노드 구현
//
// Inventory 노드는 in-process 디바이스/에이전트/노드/플로우 레지스트리의
// 스냅샷을 메시지로 emit 한다. 4종 source × 2종 emit_shape 매트릭스를 지원하며
// devices source 에 대해서는 기존 device.DeviceFilter 재사용 형태로 필터링한다.
//
// 의존성은 NodeOption 4종 (WithDeviceRegistryFunc, WithAgentManagerFunc,
// WithFlowRegistryFunc, WithNodeRegistryFunc) 으로 함수형 resolver 패턴으로
// 주입한다. 함수형 resolver 는 cmd/xflowd/main.go 에서 engine 자기 참조 등
// 순환 초기화 순서 문제를 회피하기 위한 채택이다 (plan.md 결정 (c3)).
//
// 본 노드는 어떠한 레지스트리도 변경하지 않는다 (read-only). List/Get 계열
// 메서드만 호출하며, 변경 메서드는 호출하지 않는다.
//
// 버전 이력:
//   - v0.1.0 (2026-05-25): 최초 구현 — 4종 source × 2종 shape 매트릭스.
//   - v0.2.0 (2026-05-25): devices source 의 payload 에 `device_uuid` 필드 추가.
//     agent.ResolveDeviceID(ctx, agentName, localID) 로 글로벌 UUID 를 조회하여
//     composite key (id) 와 함께 노출. UUID 가 없으면 device_uuid 키 자체를 생략
//     (graceful degradation). 사용 사례: 에이전트 rename 에도 안정적인 시계열
//     tag 키 / MQTT topic 식별자.
//   - v0.3.0 (2026-05-26): SPEC-DEVICE-IDENTITY-001 Phase B § B-T8 정규화 —
//     `device_uuid` 키를 `uid` 로 정규화 (`device_uuid` alias 함께 emit).
//   - v1.0 (2026-05-26): SPEC-DEVICE-IDENTITY-001 Phase D § D-T18 — `device_uuid`
//     호환 alias 완전 제거. `uid` 만 emit 한다 (greenfield xflowd v1.0).
package node

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// Source / Shape enum 상수
// ---------------------------------------------------------------------------

const (
	// InventorySourceDevices 는 디바이스 레지스트리 스냅샷을 emit 하는 source 값이다.
	InventorySourceDevices = "devices"
	// InventorySourceAgents 는 에이전트 매니저 스냅샷을 emit 하는 source 값이다.
	InventorySourceAgents = "agents"
	// InventorySourceNodes 는 노드 레지스트리 스냅샷을 emit 하는 source 값이다.
	InventorySourceNodes = "nodes"
	// InventorySourceFlows 는 플로우 레지스트리 스냅샷을 emit 하는 source 값이다.
	InventorySourceFlows = "flows"

	// InventoryShapeArray 는 단일 메시지에 배열을 담는 emit 형태이다 (기본).
	InventoryShapeArray = "array"
	// InventoryShapePerItem 는 항목별 N 개 메시지로 fan-out 하는 emit 형태이다.
	InventoryShapePerItem = "per_item"

	// inventoryMetaSource 는 출력 메타데이터에 추가되는 source 키 이름이다.
	inventoryMetaSource = "inventory.source"
	// inventoryMetaCount 는 출력 메타데이터에 추가되는 전체 항목 수 키 이름이다.
	inventoryMetaCount = "inventory.count"
	// inventoryMetaIndex 는 per_item 출력 메타데이터의 0-based 인덱스 키이다.
	inventoryMetaIndex = "inventory.index"
	// inventoryMetaTotal 는 per_item 출력 메타데이터의 전체 항목 수 키이다 (count alias).
	inventoryMetaTotal = "inventory.total"

	// inventoryDeviceRegistryFnKey 는 NodeOption 으로 주입된 DeviceRegistry resolver 의 config 키이다.
	inventoryDeviceRegistryFnKey = "_inventory_device_registry_fn"
	// inventoryAgentManagerFnKey 는 NodeOption 으로 주입된 AgentManager resolver 의 config 키이다.
	inventoryAgentManagerFnKey = "_inventory_agent_manager_fn"
	// inventoryFlowRegistryFnKey 는 NodeOption 으로 주입된 FlowRegistry resolver 의 config 키이다.
	inventoryFlowRegistryFnKey = "_inventory_flow_registry_fn"
	// inventoryNodeRegistryFnKey 는 NodeOption 으로 주입된 *Registry resolver 의 config 키이다.
	inventoryNodeRegistryFnKey = "_inventory_node_registry_fn"
)

// ---------------------------------------------------------------------------
// Sentinel 에러
// ---------------------------------------------------------------------------

var (
	// ErrInventoryInvalidSource 는 source 가 누락 또는 4종 enum 외 값일 때 반환된다.
	ErrInventoryInvalidSource = fmt.Errorf("inventory: %w: invalid source (must be one of: devices, agents, nodes, flows)", ErrInvalidConfig)
	// ErrInventoryInvalidEmitShape 는 emit_shape 가 2종 enum 외 값일 때 반환된다.
	ErrInventoryInvalidEmitShape = fmt.Errorf("inventory: %w: invalid emit_shape (must be one of: array, per_item)", ErrInvalidConfig)
	// ErrInventoryInvalidFilter 는 filter 의 필드 타입이 device.DeviceFilter 와 호환되지 않을 때 반환된다.
	ErrInventoryInvalidFilter = fmt.Errorf("inventory: %w: invalid filter", ErrInvalidConfig)

	// ErrInventoryDeviceRegistryNotAvailable 는 source=devices 인데 DeviceRegistry 가 주입되지 않았을 때 Init 에서 반환된다.
	ErrInventoryDeviceRegistryNotAvailable = fmt.Errorf("inventory: %w: device registry not configured (use WithDeviceRegistryFunc)", ErrNodeNotInitialized)
	// ErrInventoryAgentManagerNotAvailable 는 source=agents 인데 AgentManager 가 주입되지 않았을 때 Init 에서 반환된다.
	ErrInventoryAgentManagerNotAvailable = fmt.Errorf("inventory: %w: agent manager not configured (use WithAgentManagerFunc)", ErrNodeNotInitialized)
	// ErrInventoryFlowRegistryNotAvailable 는 source=flows 인데 FlowRegistry 가 주입되지 않았을 때 Init 에서 반환된다.
	ErrInventoryFlowRegistryNotAvailable = fmt.Errorf("inventory: %w: flow registry not configured (use WithFlowRegistryFunc)", ErrNodeNotInitialized)
	// ErrInventoryNodeRegistryNotAvailable 는 source=nodes 인데 NodeRegistry 가 주입되지 않았을 때 Init 에서 반환된다.
	ErrInventoryNodeRegistryNotAvailable = fmt.Errorf("inventory: %w: node registry not configured (use WithNodeRegistryFunc)", ErrNodeNotInitialized)
)

// ---------------------------------------------------------------------------
// FlowSummary / FlowRegistry — engine 패키지 cycle 회피용 작은 인터페이스
// ---------------------------------------------------------------------------

// FlowSummary 는 inventory 노드가 flow 목록을 표현할 때 사용하는 작은 DTO 이다.
// engine.Engine 이 (e *Engine) FlowSummaries() []node.FlowSummary 어댑터로 이를 만족한다.
// 본 타입은 engine 패키지 → node 패키지 단방향 의존성을 유지하기 위한 분리이다.
type FlowSummary struct {
	ID        string
	Name      string
	State     string
	NodeCount int
	WireCount int
	Extra     map[string]any
}

// FlowRegistry 는 inventory 노드가 flow 목록을 조회하는 최소 인터페이스이다.
// engine.Engine 이 이를 만족한다 (Phase 5 에서 어댑터 메서드 추가).
type FlowRegistry interface {
	// FlowSummaries 는 배포된 모든 플로우의 요약 정보를 반환한다.
	FlowSummaries() []FlowSummary
}

// ---------------------------------------------------------------------------
// NodeOption - 함수형 resolver 4종
// ---------------------------------------------------------------------------

// WithDeviceRegistryFunc 는 inventory 노드에 DeviceRegistry resolver 를 주입하는 옵션이다.
// 함수형 resolver 는 노드 인스턴스화 시점이 의존성 생성 이후임을 보장하여
// 초기화 순서 문제를 회피한다. fn 은 Init 시점에 1회 호출된다.
func WithDeviceRegistryFunc(fn func() device.DeviceRegistry) NodeOption {
	return func(b *BaseNode) {
		if b.config == nil {
			b.config = make(map[string]any)
		}
		b.config[inventoryDeviceRegistryFnKey] = fn
	}
}

// WithAgentManagerFunc 는 inventory 노드에 agent.Manager resolver 를 주입하는 옵션이다.
func WithAgentManagerFunc(fn func() agent.Manager) NodeOption {
	return func(b *BaseNode) {
		if b.config == nil {
			b.config = make(map[string]any)
		}
		b.config[inventoryAgentManagerFnKey] = fn
	}
}

// WithFlowRegistryFunc 는 inventory 노드에 FlowRegistry resolver 를 주입하는 옵션이다.
// engine.Engine 이 FlowSummaries() 메서드로 이를 만족한다.
func WithFlowRegistryFunc(fn func() FlowRegistry) NodeOption {
	return func(b *BaseNode) {
		if b.config == nil {
			b.config = make(map[string]any)
		}
		b.config[inventoryFlowRegistryFnKey] = fn
	}
}

// WithNodeRegistryFunc 는 inventory 노드에 *Registry resolver 를 주입하는 옵션이다.
// 자기 참조 형태이며, 함수형 resolver 패턴으로 초기화 순서 문제를 회피한다.
func WithNodeRegistryFunc(fn func() *Registry) NodeOption {
	return func(b *BaseNode) {
		if b.config == nil {
			b.config = make(map[string]any)
		}
		b.config[inventoryNodeRegistryFnKey] = fn
	}
}

// ---------------------------------------------------------------------------
// InventoryNode 구조체
// ---------------------------------------------------------------------------

// InventoryNode 는 in-process 인벤토리 스냅샷을 emit 하는 처리 노드이다.
// 4종 source (devices/agents/nodes/flows) × 2종 emit_shape (array/per_item) 매트릭스를 지원한다.
type InventoryNode struct {
	*BaseNode

	// 노드 설정 (immutable after factory)
	source          string
	emitShape       string
	includeMetadata bool

	// 필터 (devices source 한정)
	filter          device.DeviceFilter
	filterSpecified bool

	// 의존성 resolver (NodeOption 으로 주입, Init 에서 검증)
	deviceRegistryFn func() device.DeviceRegistry
	agentManagerFn   func() agent.Manager
	flowRegistryFn   func() FlowRegistry
	nodeRegistryFn   func() *Registry

	// 비-device source 에서 filter 가 무시되었음을 1회만 로깅하기 위한 가드
	filterIgnoredWarnOnce sync.Once
}

// 컴파일 타임 인터페이스 체크
var _ Node = (*InventoryNode)(nil)

// ---------------------------------------------------------------------------
// Factory
// ---------------------------------------------------------------------------

// NewInventoryNode 는 NodeDef 와 옵션으로부터 inventory 노드를 생성한다.
//
// config 키:
//   - source (string, required): "devices" | "agents" | "nodes" | "flows"
//   - emit_shape (string, default "array"): "array" | "per_item"
//   - include_metadata (bool, default true): metadata/state/info 등 풍부 필드 포함 여부
//   - filter (object, devices 한정): protocol/agent_name/type/online/group/tags
//
// 의존성은 NodeOption 4종 (WithDeviceRegistryFunc 등) 으로 주입한다.
// Init 시 선택된 source 에 필요한 resolver 가 없으면 sentinel 에러를 반환한다.
func NewInventoryNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)

	// config 정규화 (factory 시점에 validate)
	cfg := def.Config

	// source 검증 (필수)
	source, err := extractInventorySource(cfg)
	if err != nil {
		return nil, err
	}

	// emit_shape 검증 (default array)
	emitShape, err := extractInventoryEmitShape(cfg)
	if err != nil {
		return nil, err
	}

	// include_metadata (default true)
	includeMetadata := true
	if v, ok := cfg["include_metadata"]; ok {
		if b, ok := v.(bool); ok {
			includeMetadata = b
		}
	}

	// filter 파싱 (devices 한정, 비-devices 일 때는 파싱하되 적용은 안 함)
	var deviceFilter device.DeviceFilter
	var filterSpecified bool
	if rawFilter, ok := cfg["filter"]; ok && rawFilter != nil {
		// 빈 map 이면 무시
		if m, ok := rawFilter.(map[string]any); ok && len(m) > 0 {
			parsed, err := parseInventoryDeviceFilter(m)
			if err != nil {
				return nil, err
			}
			deviceFilter = parsed
			filterSpecified = true
		}
	}

	n := &InventoryNode{
		BaseNode:        base,
		source:          source,
		emitShape:       emitShape,
		includeMetadata: includeMetadata,
		filter:          deviceFilter,
		filterSpecified: filterSpecified,
	}

	// base.config 에서 NodeOption 으로 주입된 resolver 들을 추출
	if base.config != nil {
		if fn, ok := base.config[inventoryDeviceRegistryFnKey].(func() device.DeviceRegistry); ok {
			n.deviceRegistryFn = fn
		}
		if fn, ok := base.config[inventoryAgentManagerFnKey].(func() agent.Manager); ok {
			n.agentManagerFn = fn
		}
		if fn, ok := base.config[inventoryFlowRegistryFnKey].(func() FlowRegistry); ok {
			n.flowRegistryFn = fn
		}
		if fn, ok := base.config[inventoryNodeRegistryFnKey].(func() *Registry); ok {
			n.nodeRegistryFn = fn
		}
	}

	// 비-device source 에서 filter 가 명시된 경우 경고 (1회)
	if filterSpecified && source != InventorySourceDevices {
		n.filterIgnoredWarnOnce.Do(func() {
			slog.Warn("inventory: filter ignored for non-device source",
				"node", n.ID(), "source", source)
		})
	}

	return n, nil
}

// extractInventorySource 는 config 에서 source 키를 추출하고 유효성을 검증한다.
func extractInventorySource(cfg map[string]any) (string, error) {
	raw, ok := cfg["source"]
	if !ok {
		return "", fmt.Errorf("%w: missing 'source' field", ErrInventoryInvalidSource)
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%w: 'source' must be string, got %T", ErrInventoryInvalidSource, raw)
	}
	switch s {
	case InventorySourceDevices, InventorySourceAgents,
		InventorySourceNodes, InventorySourceFlows:
		return s, nil
	default:
		return "", fmt.Errorf("%w: unknown source %q", ErrInventoryInvalidSource, s)
	}
}

// extractInventoryEmitShape 는 config 에서 emit_shape 키를 추출한다. 미지정 시 기본 "array".
func extractInventoryEmitShape(cfg map[string]any) (string, error) {
	raw, ok := cfg["emit_shape"]
	if !ok {
		return InventoryShapeArray, nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%w: 'emit_shape' must be string, got %T", ErrInventoryInvalidEmitShape, raw)
	}
	switch s {
	case InventoryShapeArray, InventoryShapePerItem:
		return s, nil
	default:
		return "", fmt.Errorf("%w: unknown emit_shape %q", ErrInventoryInvalidEmitShape, s)
	}
}

// parseInventoryDeviceFilter 는 yaml/json 표현의 filter map 을 device.DeviceFilter 로 변환한다.
// 타입 불일치 시 ErrInventoryInvalidFilter 를 반환한다.
func parseInventoryDeviceFilter(raw map[string]any) (device.DeviceFilter, error) {
	var f device.DeviceFilter

	if v, ok := raw["protocol"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return f, fmt.Errorf("%w: filter.protocol must be string, got %T", ErrInventoryInvalidFilter, v)
		}
		f.Protocol = s
	}
	if v, ok := raw["agent_name"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return f, fmt.Errorf("%w: filter.agent_name must be string, got %T", ErrInventoryInvalidFilter, v)
		}
		f.AgentName = s
	}
	if v, ok := raw["type"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return f, fmt.Errorf("%w: filter.type must be string, got %T", ErrInventoryInvalidFilter, v)
		}
		f.Type = s
	}
	if v, ok := raw["online"]; ok && v != nil {
		b, ok := v.(bool)
		if !ok {
			return f, fmt.Errorf("%w: filter.online must be bool, got %T", ErrInventoryInvalidFilter, v)
		}
		f.Online = &b
	}
	if v, ok := raw["group"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return f, fmt.Errorf("%w: filter.group must be string, got %T", ErrInventoryInvalidFilter, v)
		}
		f.Group = s
	}
	if v, ok := raw["tags"]; ok && v != nil {
		tags, err := coerceStringSlice(v)
		if err != nil {
			return f, fmt.Errorf("%w: filter.tags %s", ErrInventoryInvalidFilter, err.Error())
		}
		f.Tags = tags
	}

	return f, nil
}

// coerceStringSlice 는 임의의 슬라이스를 []string 으로 변환한다.
// yaml/json 역직렬화 결과는 []any 가 흔하므로 element 타입을 엄격히 검증한다.
func coerceStringSlice(v any) ([]string, error) {
	switch s := v.(type) {
	case []string:
		return s, nil
	case []any:
		out := make([]string, 0, len(s))
		for i, el := range s {
			str, ok := el.(string)
			if !ok {
				return nil, fmt.Errorf("element[%d] must be string, got %T", i, el)
			}
			out = append(out, str)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("must be string slice, got %T", v)
	}
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// Init 은 노드를 초기화한다. 선택된 source 에 필요한 의존성 resolver 가
// 주입되어 있는지 검증하고, 없으면 sentinel 에러를 반환한다.
func (n *InventoryNode) Init(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	// source 별 필수 의존성 검증
	switch n.source {
	case InventorySourceDevices:
		if n.deviceRegistryFn == nil {
			return ErrInventoryDeviceRegistryNotAvailable
		}
	case InventorySourceAgents:
		if n.agentManagerFn == nil {
			return ErrInventoryAgentManagerNotAvailable
		}
	case InventorySourceFlows:
		if n.flowRegistryFn == nil {
			return ErrInventoryFlowRegistryNotAvailable
		}
	case InventorySourceNodes:
		if n.nodeRegistryFn == nil {
			return ErrInventoryNodeRegistryNotAvailable
		}
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Shutdown 은 노드를 종료한다. inventory 노드는 stateless read-only 이므로
// 별도의 정리 작업이 없다.
func (n *InventoryNode) Shutdown(_ context.Context) error {
	state := n.BaseNode.CurrentState()
	if state == lifecycle.StateStopped {
		return nil
	}
	if err := n.BaseNode.TransitionTo(lifecycle.StateStopping); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateStopped)
}

// ---------------------------------------------------------------------------
// Process - 핵심 로직
// ---------------------------------------------------------------------------

// Process 는 입력 메시지(트리거) 1개당 선택된 source 의 스냅샷을 emit 한다.
// emit_shape 에 따라 단일 array 메시지 또는 항목별 N개 메시지로 fan-out 한다.
// 입력 메시지의 payload 는 무시되며, metadata 는 출력에 얕은 복사로 보존된다
// (단 inventory.* 키는 노드 설정 값으로 덮어쓴다).
//
// v0.2.0: source=devices 인 경우 ctx 는 agent.ResolveDeviceID 호출에 사용되어
// 각 디바이스의 글로벌 UUID (device_uuid) 를 조회하는 데 쓰인다. 다른 source 에는
// ctx 가 사용되지 않는다.
func (n *InventoryNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	items, err := n.collectItems(ctx)
	if err != nil {
		return nil, err
	}

	inputMeta := snapshotInputMetadata(msg)
	total := len(items)

	if n.logger != nil {
		n.logger.Info("inventory: emitted snapshot",
			"source", n.source, "shape", n.emitShape, "count", total)
	}

	switch n.emitShape {
	case InventoryShapeArray:
		out := buildInventoryArrayMessage(n.source, items, inputMeta)
		return []message.Message{out}, nil

	case InventoryShapePerItem:
		if total == 0 {
			return nil, nil
		}
		results := make([]message.Message, 0, total)
		for i, item := range items {
			results = append(results, buildInventoryPerItemMessage(n.source, item, i, total, inputMeta))
		}
		return results, nil

	default:
		// 팩토리에서 검증되었으므로 이론적으로 도달 불가
		return nil, fmt.Errorf("%w: unexpected emit_shape %q", ErrInventoryInvalidEmitShape, n.emitShape)
	}
}

// collectItems 는 source 에 따른 항목 슬라이스를 수집한다.
// 각 source 별 resolver 함수를 호출하여 in-process 객체에서 데이터를 가져온다.
//
// v0.2.0: ctx 는 devices source 에서 agent.ResolveDeviceID 호출에 사용된다.
// 다른 source 에서는 사용되지 않는다.
func (n *InventoryNode) collectItems(ctx context.Context) ([]map[string]any, error) {
	switch n.source {
	case InventorySourceDevices:
		reg := n.deviceRegistryFn()
		if reg == nil {
			return nil, ErrInventoryDeviceRegistryNotAvailable
		}
		devices := reg.List(n.filter)
		items := make([]map[string]any, 0, len(devices))
		for _, d := range devices {
			items = append(items, deviceToItem(ctx, d, n.includeMetadata))
		}
		return items, nil

	case InventorySourceAgents:
		mgr := n.agentManagerFn()
		if mgr == nil {
			return nil, ErrInventoryAgentManagerNotAvailable
		}
		agents := mgr.List()
		items := make([]map[string]any, 0, len(agents))
		for _, a := range agents {
			items = append(items, agentToItem(a, n.includeMetadata))
		}
		return items, nil

	case InventorySourceFlows:
		reg := n.flowRegistryFn()
		if reg == nil {
			return nil, ErrInventoryFlowRegistryNotAvailable
		}
		flows := reg.FlowSummaries()
		items := make([]map[string]any, 0, len(flows))
		for _, f := range flows {
			items = append(items, flowSummaryToItem(f, n.includeMetadata))
		}
		return items, nil

	case InventorySourceNodes:
		reg := n.nodeRegistryFn()
		if reg == nil {
			return nil, ErrInventoryNodeRegistryNotAvailable
		}
		metas := reg.AllTypeMeta()
		items := make([]map[string]any, 0, len(metas))
		for _, m := range metas {
			items = append(items, nodeTypeMetaToItem(m))
		}
		return items, nil

	default:
		return nil, fmt.Errorf("%w: unexpected source %q", ErrInventoryInvalidSource, n.source)
	}
}

// ---------------------------------------------------------------------------
// Item serializers - source 별 항목 객체 생성
// ---------------------------------------------------------------------------

// deviceToItem 은 device.Device 를 inventory 항목 map 으로 직렬화한다.
// includeMeta=false 일 때 metadata/state 키를 생략하고 핵심 식별 필드만 노출한다.
//
// v0.2.0: agent.ResolveDeviceID(ctx, agentName, localID) 를 호출하여 글로벌 UUID
// (device_uuid) 를 함께 노출한다. UUID 가 비어 있으면 (저장소 미설정 / 매핑 없음 /
// 에러) device_uuid 키 자체를 생략하여 downstream 이 키 존재 여부로 graceful
// degradation 을 판단할 수 있게 한다. localID 는 composite id ("agent_name:local_id")
// 에서 "agent_name:" 접두사를 제거하여 추출한다.
func deviceToItem(ctx context.Context, d device.Device, includeMeta bool) map[string]any {
	item := map[string]any{
		"id":           d.ID(),
		"name":         d.Name(),
		"type":         string(d.Type()),
		"protocol":     d.Protocol(),
		"agent_name":   d.AgentName(),
		"online":       d.Online(),
		"last_seen":    formatRFC3339(d.LastSeen()),
		"source":       d.Source(),
		"capabilities": stringSliceOrEmpty(d.Capabilities()),
	}

	// uid (UUID) — 글로벌 식별자 (SPEC-DEVICE-IDENTITY-001 Phase D § D-T18).
	// SPEC-INVENTORY-001 v1.0: device_uuid 호환 alias 제거 — uid 만 emit.
	// UUID 가 없으면 키 자체를 생략 (graceful degradation).
	if uuid := resolveDeviceUUID(ctx, d); uuid != "" {
		item["uid"] = uuid
	}

	if !includeMeta {
		return item
	}

	// metadata 객체
	md := d.Metadata()
	metaObj := map[string]any{
		"name":     md.Name,
		"tags":     stringSliceOrEmpty(md.Tags),
		"location": md.Location,
		"group":    md.Group,
		"labels":   stringMapOrEmpty(md.Labels),
	}
	if md.Pinned != nil {
		metaObj["pinned"] = *md.Pinned
	} else {
		metaObj["pinned"] = false
	}
	item["metadata"] = metaObj

	// state 객체
	st := d.State()
	item["state"] = map[string]any{
		"online":      st.Online,
		"ready":       st.Ready,
		"last_seen":   formatRFC3339(st.LastSeen),
		"error_count": st.ErrorCount,
		"properties":  anyMapOrEmpty(st.Properties),
	}

	return item
}

// agentToItem 은 agent.Agent 를 inventory 항목 map 으로 직렬화한다.
// includeMeta=true 일 때 AgentInfo/StatsSnapshot 의 안전한 일부 필드를 첨부한다.
func agentToItem(a agent.Agent, includeMeta bool) map[string]any {
	info := a.Info()
	item := map[string]any{
		"id":    a.ID(),
		"name":  a.Name(),
		"type":  a.Type(),
		"state": string(info.State),
	}
	if !includeMeta {
		return item
	}

	// info 객체 — config 등 민감 정보는 생략하고 안전한 필드만 노출
	item["info"] = map[string]any{
		"id":         info.ID,
		"name":       info.Name,
		"type":       info.Type,
		"state":      string(info.State),
		"started_at": formatRFC3339(info.StartedAt),
		"created_at": formatRFC3339(info.CreatedAt),
		"uptime_ms":  info.Uptime.Milliseconds(),
	}

	// stats 객체 - 비용이 큰 노드 참조 통계는 NodeRefs 길이만 노출
	stats := info.Stats
	item["stats"] = map[string]any{
		"messages_received": stats.MessagesReceived,
		"messages_sent":     stats.MessagesSent,
		"messages_errored":  stats.MessagesErrored,
		"bytes_read":        stats.BytesRead,
		"bytes_written":     stats.BytesWritten,
		"restart_count":     stats.RestartCount,
		"dropped_messages":  stats.DroppedMessages,
		"last_activity_at":  formatRFC3339(stats.LastActivityAt),
		"node_refs_count":   len(stats.NodeRefs),
	}

	return item
}

// flowSummaryToItem 은 FlowSummary 를 inventory 항목 map 으로 직렬화한다.
func flowSummaryToItem(f FlowSummary, includeMeta bool) map[string]any {
	item := map[string]any{
		"id":         f.ID,
		"name":       f.Name,
		"state":      f.State,
		"node_count": f.NodeCount,
		"wire_count": f.WireCount,
	}
	if !includeMeta {
		return item
	}
	item["extra"] = anyMapOrEmpty(f.Extra)
	return item
}

// nodeTypeMetaToItem 은 NodeTypeMeta 를 inventory 항목 map 으로 직렬화한다.
// include_metadata 영향 없이 항상 동일 4개 필드를 노출한다 (이미 메타데이터 자체).
func nodeTypeMetaToItem(m NodeTypeMeta) map[string]any {
	return map[string]any{
		"type":        m.Type,
		"category":    m.Category,
		"description": m.Description,
		"origin":      m.Source,
	}
}

// ---------------------------------------------------------------------------
// Message builders - array / per_item 출력 메시지 생성
// ---------------------------------------------------------------------------

// buildInventoryArrayMessage 는 array shape 의 단일 출력 메시지를 생성한다.
// 입력 metadata 는 얕은 복사로 보존하되 inventory.* 키는 노드 설정 값으로 덮어쓴다.
func buildInventoryArrayMessage(source string, items []map[string]any, inputMeta map[string]string) message.Message {
	out := message.New()
	out.Payload().Set("source", source)
	out.Payload().Set("count", len(items))
	out.Payload().Set("items", items)

	applyMetadata(out, inputMeta, map[string]string{
		inventoryMetaSource: source,
		inventoryMetaCount:  strconv.Itoa(len(items)),
	})
	return out
}

// buildInventoryPerItemMessage 는 per_item shape 의 단일 항목 메시지를 생성한다.
// payload 는 item map 자체를 키-값으로 풀어서 노출 (wrapper 없음).
func buildInventoryPerItemMessage(source string, item map[string]any, index, total int, inputMeta map[string]string) message.Message {
	out := message.New()
	for k, v := range item {
		out.Payload().Set(k, v)
	}

	applyMetadata(out, inputMeta, map[string]string{
		inventoryMetaSource: source,
		inventoryMetaCount:  strconv.Itoa(total),
		inventoryMetaIndex:  strconv.Itoa(index),
		inventoryMetaTotal:  strconv.Itoa(total),
	})
	return out
}

// applyMetadata 는 입력 metadata 의 얕은 복사 후 inventory.* override 키들을 덮어쓴다.
// 결정 (d3) 의 정책 구현이다 — 입력 trigger 의 컨텍스트 (schedule_id 등)는
// downstream 에서 활용 가능하도록 보존된다.
func applyMetadata(msg message.Message, inputMeta map[string]string, overrides map[string]string) {
	md := msg.Metadata()
	for k, v := range inputMeta {
		md.Set(k, v)
	}
	for k, v := range overrides {
		md.Set(k, v)
	}
}

// snapshotInputMetadata 는 입력 메시지의 metadata 를 얕은 복사한다.
// nil-safe: msg.Metadata() 가 nil 이거나 빈 map 이면 빈 map 을 반환한다.
func snapshotInputMetadata(msg message.Message) map[string]string {
	if msg == nil {
		return map[string]string{}
	}
	md := msg.Metadata()
	if md == nil {
		return map[string]string{}
	}
	return md.All()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resolveDeviceUUID 는 디바이스의 글로벌 UUID (device_uuid) 를 조회한다 (v0.2.0).
//
// SPEC-DEVICE-IDENTITY-001 Phase A (A-AC5): Device.UID() 가 인터페이스에
// 추가되었으므로, 어댑터가 직접 책임지는 UID() 결과를 그대로 사용한다.
// 이전에는 composite id ("agent:local_id") 에서 prefix 를 제거해 localID 를
// 재추출했으나, 이 방식은 Century 같이 composite 의 localID 형식이 emit
// 경로의 ResolveDeviceID 호출 형식과 다른 경우 (예: "3b" vs "0x3B") 서로
// 다른 UUID 를 반환하는 결함이 있었다. 어댑터의 UID() 는 emit 경로와
// 정확히 같은 unitID 형식을 사용하므로 본 SPEC 의 핵심 invariant
// (emit / inventory / REST 의 uid 가 동일 UUID) 가 자연스럽게 보장된다.
//
// 호환 정렬: SPEC-INVENTORY-001 v0.2.0 의 device_uuid 필드는 본 SPEC 의 uid
// 와 항상 동일 값을 가진다. Phase B 에서 키 자체를 uid 로 정규화할 예정.
//
// UID 가 비어 있으면 (DeviceIDRepository 미설정 / 매핑 부재 / 에러) device_uuid
// 키 자체를 생략한다 (graceful degradation).
//
// ctx 인자는 인터페이스 호환을 위해 보존하되, 현재 d.UID() 는 자체적으로
// context.Background() 를 사용하므로 사용되지 않는다 (블랭크 처리).
func resolveDeviceUUID(_ context.Context, d device.Device) string {
	return d.UID()
}

// formatRFC3339 는 time.Time 을 RFC3339 문자열로 직렬화한다. zero time 은 빈 문자열을 반환한다.
func formatRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// stringSliceOrEmpty 는 nil 슬라이스를 빈 슬라이스로 정규화한다 (JSON downstream 친화).
func stringSliceOrEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// stringMapOrEmpty 는 nil map 을 빈 map 으로 정규화한다.
func stringMapOrEmpty(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// anyMapOrEmpty 는 nil map 을 빈 map 으로 정규화한다.
func anyMapOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
