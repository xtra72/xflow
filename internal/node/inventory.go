// Package node - inventory.go: Inventory 노드 (재설계)
//
// Inventory 노드는 in-process 디바이스/에이전트/노드/플로우 레지스트리의
// 스냅샷을 메시지로 emit 한다. 4종 source 를 지원하며, 항목 단위의 조건식
// 필터(condition)·필드 화이트리스트 투영(fields)·메시지당 항목 수 청킹
// (max_items) 을 제공한다.
//
// 의존성은 NodeOption 4종 (WithDeviceRegistryFunc, WithAgentManagerFunc,
// WithFlowRegistryFunc, WithNodeRegistryFunc) 으로 함수형 resolver 패턴으로
// 주입한다. 함수형 resolver 는 cmd/xflowd/main.go 에서 engine 자기 참조 등
// 순환 초기화 순서 문제를 회피하기 위한 채택이다.
//
// 본 노드는 어떠한 레지스트리도 변경하지 않는다 (read-only). List/Get 계열
// 메서드만 호출하며, 변경 메서드는 호출하지 않는다.
//
// 재설계(breaking): 구 키 emit_shape / include_metadata / filter(struct) 는
// 제거되었다. 항목 직렬화는 항상 풍부한(rich) 필드를 노출하며, 필터링은
// filter 노드와 동일한 compileCondition 조건식으로 모든 source 에 적용된다.
// 출력 payload 는 항상 "items" 키 아래 청크 배열을 담고, metadata 는
// type/total_count/offset/count 4키만 설정한다 (입력 메타 복사 없음).
package node

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// Source enum 상수
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

	// inventoryMetaType 은 출력 메타데이터의 source 단수형 키 이름이다.
	inventoryMetaType = "type"
	// inventoryMetaTotalCount 는 출력 메타데이터의 전체(필터 후) 항목 수 키 이름이다.
	inventoryMetaTotalCount = "total_count"
	// inventoryMetaOffset 은 출력 메타데이터의 청크 첫 항목 0-based 인덱스 키 이름이다.
	inventoryMetaOffset = "offset"
	// inventoryMetaCount 는 출력 메타데이터의 이 메시지 항목 수 키 이름이다.
	inventoryMetaCount = "count"

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
	// ErrInventoryInvalidConfig 는 fields/max_items 등 설정 타입이 잘못되었을 때 반환된다.
	ErrInventoryInvalidConfig = fmt.Errorf("inventory: %w: invalid config", ErrInvalidConfig)

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
// engine.Engine 이 이를 만족한다.
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
// 4종 source (devices/agents/nodes/flows) 를 지원하며, 항목 단위의 조건식
// 필터·필드 투영·청킹을 제공한다.
type InventoryNode struct {
	*BaseNode

	// 노드 설정 (immutable after factory)
	source   string
	fields   []string        // 필드 화이트리스트 (빈 슬라이스면 전체 필드)
	cond     FilterCondition // 항목 조건식 필터 (nil 이면 필터 없음)
	maxItems int             // 메시지당 최대 항목 수 (<=0 이면 무제한)

	// 의존성 resolver (NodeOption 으로 주입, Init 에서 검증)
	deviceRegistryFn func() device.DeviceRegistry
	agentManagerFn   func() agent.Manager
	flowRegistryFn   func() FlowRegistry
	nodeRegistryFn   func() *Registry
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
//   - fields (string | []any | []string, optional): 포함할 항목 필드 화이트리스트.
//     쉼표 구분 string 또는 슬라이스 모두 허용 (공백 trim, 빈 항목 제거).
//     비었거나 미지정이면 전체 필드. 항목에 없는 키는 그냥 생략한다 (에러 아님).
//   - condition (string, optional): 항목 조건식 필터. filter 노드와 동일한
//     compileCondition 으로 컴파일하며, 각 항목을 payload 로 감싸 평가한다.
//     모든 source 에 적용된다. 컴파일 실패 시 에러를 반환한다.
//   - max_items (number, optional, default 0): 메시지당 최대 항목 수.
//     <=0 이면 무제한 (전체 1개 메시지), >0 이면 N개씩 분할한다.
//
// 의존성은 NodeOption 4종 (WithDeviceRegistryFunc 등) 으로 주입한다.
// Init 시 선택된 source 에 필요한 resolver 가 없으면 sentinel 에러를 반환한다.
func NewInventoryNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	cfg := def.Config

	// source 검증 (필수)
	source, err := extractInventorySource(cfg)
	if err != nil {
		return nil, err
	}

	// fields 파싱 (선택)
	fields, err := parseInventoryFields(cfg)
	if err != nil {
		return nil, err
	}

	// condition 컴파일 (선택, filter 노드 정책과 동일)
	cond, err := compileInventoryCondition(cfg)
	if err != nil {
		return nil, err
	}

	// max_items 파싱 (선택, default 0)
	maxItems, err := parseInventoryMaxItems(cfg)
	if err != nil {
		return nil, err
	}

	n := &InventoryNode{
		BaseNode: base,
		source:   source,
		fields:   fields,
		cond:     cond,
		maxItems: maxItems,
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

// parseInventoryFields 는 config 의 fields 키를 []string 으로 파싱한다.
// 쉼표 구분 string 또는 []any/[]string 을 허용하며, 각 항목을 trim 하고
// 빈 항목은 제거한다. 미지정/빈 결과는 nil (전체 필드)을 반환한다.
func parseInventoryFields(cfg map[string]any) ([]string, error) {
	raw, ok := cfg["fields"]
	if !ok || raw == nil {
		return nil, nil
	}

	var parts []string
	switch v := raw.(type) {
	case string:
		parts = strings.Split(v, ",")
	case []string:
		parts = v
	case []any:
		parts = make([]string, 0, len(v))
		for i, el := range v {
			s, ok := el.(string)
			if !ok {
				return nil, fmt.Errorf("%w: fields[%d] must be string, got %T", ErrInventoryInvalidConfig, i, el)
			}
			parts = append(parts, s)
		}
	default:
		return nil, fmt.Errorf("%w: 'fields' must be string or string slice, got %T", ErrInventoryInvalidConfig, raw)
	}

	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// compileInventoryCondition 은 config 의 condition 키를 FilterCondition 으로 컴파일한다.
// filter 노드와 동일하게 compileCondition 을 재사용한다. 미지정/빈 문자열이면
// nil (필터 없음)을 반환한다. 타입 불일치/컴파일 실패 시 에러를 반환한다.
func compileInventoryCondition(cfg map[string]any) (FilterCondition, error) {
	raw, ok := cfg["condition"]
	if !ok || raw == nil {
		return nil, nil
	}
	expr, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("%w: 'condition' must be string, got %T", ErrInventoryInvalidConfig, raw)
	}
	if strings.TrimSpace(expr) == "" {
		return nil, nil
	}
	cond, err := compileCondition(expr)
	if err != nil {
		return nil, fmt.Errorf("inventory configure: %w", err)
	}
	return cond, nil
}

// parseInventoryMaxItems 는 config 의 max_items 키를 int 로 파싱한다.
// JSON number (float64) 또는 int 를 허용한다. 미지정이면 0 (무제한)을 반환한다.
func parseInventoryMaxItems(cfg map[string]any) (int, error) {
	raw, ok := cfg["max_items"]
	if !ok || raw == nil {
		return 0, nil
	}
	switch v := raw.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case float32:
		return int(v), nil
	default:
		return 0, fmt.Errorf("%w: 'max_items' must be a number, got %T", ErrInventoryInvalidConfig, raw)
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
// 처리 순서는 (1) 수집 → (2) condition 필터 → (3) fields 투영 → (4) max_items
// 청킹 이다. 입력 메시지의 payload/metadata 는 모두 무시된다.
//
// 청킹 경계:
//   - total==0: 빈 메시지 1개 (items=[], total_count=0, offset=0, count=0).
//     트리거당 1메시지 보장을 위해 무방출이 아니라 빈 메시지를 방출한다.
//   - max_items<=0: 전체 항목으로 메시지 1개 (offset=0, count=total).
//   - max_items>0: offset = 0, max, 2max, … 각 청크마다 메시지 1개.
//
// ctx 는 devices source 에서 deviceToItem 의 UUID resolve 호출에 사용된다.
func (n *InventoryNode) Process(ctx context.Context, _ message.Message) ([]message.Message, error) {
	items, err := n.collectItems(ctx)
	if err != nil {
		return nil, err
	}

	// (2) condition 필터 적용 (있으면)
	if n.cond != nil {
		items = n.applyCondition(items)
	}

	// (3) fields 투영 적용 (있으면)
	if len(n.fields) > 0 {
		items = n.applyFieldProjection(items)
	}

	total := len(items)

	if n.logger != nil {
		n.logger.Debug("inventory: emitted snapshot",
			"source", n.source, "total", total, "max_items", n.maxItems)
	}

	typeLabel := singularSource(n.source)

	// (4) 청킹 → 메시지 생성
	// total==0 또는 max_items<=0 이면 단일 메시지.
	if total == 0 || n.maxItems <= 0 {
		out := buildInventoryMessage(typeLabel, items, total, 0)
		return []message.Message{out}, nil
	}

	results := make([]message.Message, 0, (total+n.maxItems-1)/n.maxItems)
	for offset := 0; offset < total; offset += n.maxItems {
		end := offset + n.maxItems
		if end > total {
			end = total
		}
		chunk := items[offset:end]
		results = append(results, buildInventoryMessage(typeLabel, chunk, total, offset))
	}
	return results, nil
}

// applyCondition 은 각 항목을 message payload 로 감싸 조건식을 평가하고,
// 통과한 항목만 남긴다. filter 노드와 동일한 평가 경로를 재사용한다
// ($.payload.X 가 항목의 키 X 를 가리킨다).
func (n *InventoryNode) applyCondition(items []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		msg := message.New(message.WithPayload(message.NewPayload(item)))
		if n.cond(msg) {
			out = append(out, item)
		}
	}
	return out
}

// applyFieldProjection 은 각 항목을 fields 화이트리스트 키들로만 재구성한다.
// 항목에 없는 키는 그냥 생략한다 (에러 아님).
func (n *InventoryNode) applyFieldProjection(items []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		projected := make(map[string]any, len(n.fields))
		for _, key := range n.fields {
			if v, ok := item[key]; ok {
				projected[key] = v
			}
		}
		out = append(out, projected)
	}
	return out
}

// collectItems 는 source 에 따른 항목 슬라이스를 수집한다.
// 각 source 별 resolver 함수를 호출하여 in-process 객체에서 데이터를 가져온다.
// devices source 는 빈 device.DeviceFilter{} 로 List 를 호출하며, 필터링은
// 상위의 condition 조건식으로 통일된다.
//
// ctx 는 devices source 에서 deviceToItem 의 UUID resolve 호출에 사용된다.
func (n *InventoryNode) collectItems(ctx context.Context) ([]map[string]any, error) {
	switch n.source {
	case InventorySourceDevices:
		reg := n.deviceRegistryFn()
		if reg == nil {
			return nil, ErrInventoryDeviceRegistryNotAvailable
		}
		devices := reg.List(device.DeviceFilter{})
		items := make([]map[string]any, 0, len(devices))
		for _, d := range devices {
			items = append(items, deviceToItem(ctx, d))
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
			items = append(items, agentToItem(a))
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
			items = append(items, flowSummaryToItem(f))
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

// singularSource 는 source 의 복수형을 metadata.type 의 단수형으로 변환한다.
func singularSource(source string) string {
	switch source {
	case InventorySourceDevices:
		return "device"
	case InventorySourceAgents:
		return "agent"
	case InventorySourceNodes:
		return "node"
	case InventorySourceFlows:
		return "flow"
	default:
		return source
	}
}

// ---------------------------------------------------------------------------
// Item serializers - source 별 항목 객체 생성 (항상 rich)
// ---------------------------------------------------------------------------

// deviceToItem 은 device.Device 를 inventory 항목 map 으로 직렬화한다.
// metadata/state 등 풍부 필드를 항상 포함한다.
//
// SPEC-DEVICE-IDENTITY-001 Phase D (v1.0): Device.ID() 자체가 UUID 를 반환하므로
// payload 의 "id" 키 (= d.ID() = UUID) 만 노출한다. ctx 는 향후 UUID resolve
// 확장 지점으로 유지한다.
func deviceToItem(_ context.Context, d device.Device) map[string]any {
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
// AgentInfo/StatsSnapshot 의 안전한 일부 필드를 항상 첨부한다.
func agentToItem(a agent.Agent) map[string]any {
	info := a.Info()
	item := map[string]any{
		"id":    a.ID(),
		"name":  a.Name(),
		"type":  a.Type(),
		"state": string(info.State),
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
func flowSummaryToItem(f FlowSummary) map[string]any {
	return map[string]any{
		"id":         f.ID,
		"name":       f.Name,
		"state":      f.State,
		"node_count": f.NodeCount,
		"wire_count": f.WireCount,
		"extra":      anyMapOrEmpty(f.Extra),
	}
}

// nodeTypeMetaToItem 은 NodeTypeMeta 를 inventory 항목 map 으로 직렬화한다.
func nodeTypeMetaToItem(m NodeTypeMeta) map[string]any {
	return map[string]any{
		"type":        m.Type,
		"category":    m.Category,
		"description": m.Description,
		"origin":      m.Source,
	}
}

// ---------------------------------------------------------------------------
// Message builder - 단일 청크 출력 메시지 생성
// ---------------------------------------------------------------------------

// buildInventoryMessage 는 청크 항목을 담은 단일 출력 메시지를 생성한다.
// payload 는 "items" 키 아래 청크 배열을 담고, metadata 는 type/total_count/
// offset/count 4키만 설정한다 (입력 메타 복사 없음).
// SPEC-MESSAGE-TYPE-001 § T1: 1급 Type() 으로 "inventory.event" 설정.
func buildInventoryMessage(typeLabel string, chunk []map[string]any, total, offset int) message.Message {
	out := message.New()
	out.SetType("inventory.event")
	out.Payload().Set("items", chunk)

	md := out.Metadata()
	md.Set(inventoryMetaType, typeLabel)
	md.Set(inventoryMetaTotalCount, strconv.Itoa(total))
	md.Set(inventoryMetaOffset, strconv.Itoa(offset))
	md.Set(inventoryMetaCount, strconv.Itoa(len(chunk)))
	return out
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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
