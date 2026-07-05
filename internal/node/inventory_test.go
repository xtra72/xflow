// Package node - inventory_test.go: inventory 노드 재설계 단위 테스트
//
// 본 파일은 재설계된 Inventory 노드 (devices/agents/nodes/flows 스냅샷 emit)의
// 단위 테스트를 제공한다. 신규 동작(조건식 필터, fields 투영, max_items 청킹,
// 새 metadata, payload.items)은 reproduction-first 로 먼저 작성되었다.
//
// 구 동작(emit_shape / include_metadata / filter struct)은 breaking 재설계로
// 제거되었으므로 관련 테스트도 함께 제거되었다.
package node

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// Test fakes - DeviceRegistry, AgentManager, FlowRegistry, *node.Registry
// ---------------------------------------------------------------------------

// fakeDevice 는 device.Device 인터페이스의 최소 구현체이다.
type fakeDevice struct {
	id           string
	name         string
	uid          string // SPEC-DEVICE-IDENTITY-001 Phase A: UUID v4 (may be empty)
	devType      device.DeviceType
	protocol     string
	agentName    string
	online       bool
	lastSeen     time.Time
	state        device.DeviceState
	metadata     device.DeviceMetadata
	deviceSource string
	capabilities []string
}

func (d *fakeDevice) ID() string { return d.id }

// UID 는 production 어댑터 패턴 (agent.ResolveDeviceID 를 통한 UUID 조회) 을
// 재현한다. d.uid 가 명시적으로 채워져 있으면 그 값을 우선 사용한다.
func (d *fakeDevice) UID() string {
	if d.uid != "" {
		return d.uid
	}
	if d.agentName == "" {
		return ""
	}
	prefix := d.agentName + ":"
	if len(d.id) <= len(prefix) || d.id[:len(prefix)] != prefix {
		return ""
	}
	localID := d.id[len(prefix):]
	return agent.ResolveDeviceID(context.Background(), d.agentName, localID)
}
func (d *fakeDevice) Name() string                    { return d.name }
func (d *fakeDevice) Type() device.DeviceType         { return d.devType }
func (d *fakeDevice) Protocol() string                { return d.protocol }
func (d *fakeDevice) AgentName() string               { return d.agentName }
func (d *fakeDevice) Online() bool                    { return d.online }
func (d *fakeDevice) LastSeen() time.Time             { return d.lastSeen }
func (d *fakeDevice) State() device.DeviceState       { return d.state }
func (d *fakeDevice) Metadata() device.DeviceMetadata { return d.metadata }
func (d *fakeDevice) Source() string                  { return d.deviceSource }
func (d *fakeDevice) Capabilities() []string          { return d.capabilities }

// fakeDeviceRegistry 는 device.DeviceRegistry 의 테스트용 구현이다.
// 호출 인자 추적을 위해 lastFilter / listCallCount 를 노출한다.
type fakeDeviceRegistry struct {
	mu            sync.Mutex
	devices       []device.Device
	lastFilter    device.DeviceFilter
	listCallCount int
}

func newFakeDeviceRegistry(devs ...device.Device) *fakeDeviceRegistry {
	return &fakeDeviceRegistry{devices: devs}
}

func (r *fakeDeviceRegistry) List(filter device.DeviceFilter) []device.Device {
	r.mu.Lock()
	r.lastFilter = filter
	r.listCallCount++
	r.mu.Unlock()

	var result []device.Device
	for _, d := range r.devices {
		if filter.Matches(d) {
			result = append(result, d)
		}
	}
	return result
}

func (r *fakeDeviceRegistry) Get(id string) (device.Device, error) {
	for _, d := range r.devices {
		if d.ID() == id {
			return d, nil
		}
	}
	return nil, device.ErrDeviceNotFound
}

func (r *fakeDeviceRegistry) Count() int {
	return len(r.devices)
}

func (r *fakeDeviceRegistry) RegisterProvider(string, device.DeviceProvider)  {}
func (r *fakeDeviceRegistry) UnregisterProvider(string)                       {}
func (r *fakeDeviceRegistry) SetMetadata(string, device.DeviceMetadata) error { return nil }
func (r *fakeDeviceRegistry) GetMetadata(string) (device.DeviceMetadata, error) {
	return device.DeviceMetadata{}, nil
}
func (r *fakeDeviceRegistry) Execute(context.Context, string, string, map[string]any) (map[string]any, error) {
	return nil, nil
}

// GetByUID 는 SPEC-DEVICE-IDENTITY-001 Phase B 의 1급 lookup 경로 (fake).
func (r *fakeDeviceRegistry) GetByUID(uid string) (device.Device, error) {
	if uid == "" {
		return nil, device.ErrDeviceNotFound
	}
	for _, d := range r.devices {
		if d.UID() == uid {
			return d, nil
		}
	}
	return nil, device.ErrDeviceNotFound
}

// GetByAgentName 은 (agent, name) lookup 경로 (fake).
func (r *fakeDeviceRegistry) GetByAgentName(agent, name string) (device.Device, error) {
	if agent == "" || name == "" {
		return nil, device.ErrDeviceNotFound
	}
	for _, d := range r.devices {
		if d.AgentName() == agent && d.Name() == name {
			return d, nil
		}
	}
	return nil, device.ErrDeviceNotFound
}

// ResolveDevice 는 참조 형식 자동 dispatch (fake).
func (r *fakeDeviceRegistry) ResolveDevice(ref string) (device.Device, device.DeviceRefKind, error) {
	kind := device.ClassifyDeviceRef(ref)
	switch kind {
	case device.DeviceRefUUID:
		d, err := r.GetByUID(ref)
		return d, kind, err
	case device.DeviceRefAgentName:
		agent, name, ok := device.SplitAgentName(ref)
		if !ok {
			return nil, kind, device.ErrDeviceNotFound
		}
		d, err := r.GetByAgentName(agent, name)
		return d, kind, err
	case device.DeviceRefComposite:
		d, err := r.Get(ref)
		return d, kind, err
	default:
		return nil, kind, device.ErrDeviceNotFound
	}
}

// makeDevice 는 표준 fakeDevice 를 생성한다.
func makeDevice(id, name, protocol, agentName string, online bool) *fakeDevice {
	now := time.Date(2026, 5, 25, 10, 30, 0, 0, time.UTC)
	return &fakeDevice{
		id:           id,
		name:         name,
		devType:      device.DeviceTypeIndoor,
		protocol:     protocol,
		agentName:    agentName,
		online:       online,
		lastSeen:     now,
		deviceSource: "auto",
		capabilities: []string{"status", "control"},
		state: device.DeviceState{
			Online:     online,
			Ready:      true,
			LastSeen:   now,
			ErrorCount: 0,
			Properties: map[string]any{"temperature": 22.5},
		},
		metadata: device.DeviceMetadata{
			Name:     name,
			Tags:     []string{"critical", "production"},
			Location: "Floor 1",
			Group:    "production",
			Labels:   map[string]string{"zone": "north"},
		},
	}
}

// newInventoryNode 는 테스트 헬퍼: 팩토리 + Configure 를 수행한다.
func newInventoryNode(t *testing.T, cfg map[string]any, opts ...NodeOption) *InventoryNode {
	t.Helper()
	def := flow.NodeDef{
		ID:     "inv-1",
		Name:   "inv",
		Type:   "inventory",
		Config: cfg,
	}
	n, err := NewInventoryNode(def, opts...)
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}
	if err := n.Configure(cfg); err != nil {
		t.Fatalf("configure error: %v", err)
	}
	return n.(*InventoryNode)
}

// getItems 는 출력 메시지의 payload.items 를 []map[string]any 로 추출한다.
func getItems(t *testing.T, m message.Message) []map[string]any {
	t.Helper()
	raw, ok := m.Payload().Get("items")
	if !ok {
		t.Fatalf("payload.items missing")
	}
	items, ok := raw.([]map[string]any)
	if !ok {
		t.Fatalf("payload.items must be []map[string]any, got %T", raw)
	}
	return items
}

// metaStr 는 출력 메시지의 metadata 문자열 키를 읽는다.
func metaStr(t *testing.T, m message.Message, key string) string {
	t.Helper()
	v, _ := m.Metadata().Get(key)
	return v
}

// ---------------------------------------------------------------------------
// Factory - source 검증
// ---------------------------------------------------------------------------

func TestInventoryNode_FactoryWithMissingSource_ReturnsError(t *testing.T) {
	def := flow.NodeDef{ID: "n1", Type: "inventory", Config: map[string]any{}}
	n, err := NewInventoryNode(def)
	if n != nil {
		t.Fatalf("expected nil node when source missing, got %v", n)
	}
	if !errors.Is(err, ErrInventoryInvalidSource) {
		t.Fatalf("expected ErrInventoryInvalidSource, got %v", err)
	}
}

func TestInventoryNode_FactoryWithInvalidSource_ReturnsError(t *testing.T) {
	def := flow.NodeDef{ID: "n1", Type: "inventory", Config: map[string]any{"source": "unknown_kind"}}
	_, err := NewInventoryNode(def)
	if !errors.Is(err, ErrInventoryInvalidSource) {
		t.Fatalf("expected ErrInventoryInvalidSource, got %v", err)
	}
}

func TestInventoryNode_FactoryAllValidSources_ReturnsNode(t *testing.T) {
	sources := []string{
		InventorySourceDevices, InventorySourceAgents,
		InventorySourceNodes, InventorySourceFlows,
	}
	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			def := flow.NodeDef{ID: "n-" + src, Type: "inventory", Config: map[string]any{"source": src}}
			n, err := NewInventoryNode(def)
			if err != nil {
				t.Fatalf("source=%s: unexpected error: %v", src, err)
			}
			inv, ok := n.(*InventoryNode)
			if !ok {
				t.Fatalf("source=%s: expected *InventoryNode, got %T", src, n)
			}
			if inv.source != src {
				t.Fatalf("source=%s: internal source mismatch: %s", src, inv.source)
			}
		})
	}
}

func TestInventoryNode_DefaultPorts(t *testing.T) {
	def := flow.NodeDef{
		ID:      "n1",
		Type:    "inventory",
		Config:  map[string]any{"source": "devices"},
		Inputs:  []flow.Port{{ID: "p1", Name: "in", Direction: flow.PortInput}},
		Outputs: []flow.Port{{ID: "p2", Name: "out", Direction: flow.PortOutput}},
	}
	n, err := NewInventoryNode(def)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	ports := n.Ports()
	if len(ports) < 3 {
		t.Fatalf("expected at least 3 ports (in/out/error), got %d", len(ports))
	}
	var hasIn, hasOut, hasError bool
	for _, p := range ports {
		switch p.Name {
		case "in":
			hasIn = true
		case "out":
			hasOut = true
		case "_error":
			hasError = true
		}
	}
	if !hasIn || !hasOut || !hasError {
		t.Fatalf("missing ports: in=%v out=%v error=%v", hasIn, hasOut, hasError)
	}
}

// ---------------------------------------------------------------------------
// Source 의존성 미주입 시 Init 에러
// ---------------------------------------------------------------------------

func TestInventoryNode_DevicesSource_WithoutRegistryOption_InitError(t *testing.T) {
	n := newInventoryNode(t, map[string]any{"source": "devices"})
	if err := n.Init(context.Background()); !errors.Is(err, ErrInventoryDeviceRegistryNotAvailable) {
		t.Fatalf("expected ErrInventoryDeviceRegistryNotAvailable, got %v", err)
	}
}

func TestInventoryNode_AgentsSource_WithoutManagerOption_InitError(t *testing.T) {
	n := newInventoryNode(t, map[string]any{"source": "agents"})
	if err := n.Init(context.Background()); !errors.Is(err, ErrInventoryAgentManagerNotAvailable) {
		t.Fatalf("expected ErrInventoryAgentManagerNotAvailable, got %v", err)
	}
}

func TestInventoryNode_FlowsSource_WithoutRegistryOption_InitError(t *testing.T) {
	n := newInventoryNode(t, map[string]any{"source": "flows"})
	if err := n.Init(context.Background()); !errors.Is(err, ErrInventoryFlowRegistryNotAvailable) {
		t.Fatalf("expected ErrInventoryFlowRegistryNotAvailable, got %v", err)
	}
}

func TestInventoryNode_NodesSource_WithoutNodeRegistryOption_InitError(t *testing.T) {
	n := newInventoryNode(t, map[string]any{"source": "nodes"})
	if err := n.Init(context.Background()); !errors.Is(err, ErrInventoryNodeRegistryNotAvailable) {
		t.Fatalf("expected ErrInventoryNodeRegistryNotAvailable, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// devices source - 기본 emit (payload.items + 새 metadata)
// ---------------------------------------------------------------------------

func TestInventoryNode_Devices_EmitsSingleMessageWithItems(t *testing.T) {
	reg := newFakeDeviceRegistry(
		makeDevice("ag1:0.0.16", "Indoor A", "lg_icp01", "ag1", true),
		makeDevice("ag1:0.0.17", "Indoor B", "lg_icp01", "ag1", true),
	)
	n := newInventoryNode(t,
		map[string]any{"source": "devices"},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init error: %v", err)
	}

	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out))
	}

	items := getItems(t, out[0])
	if len(items) != 2 {
		t.Fatalf("items length mismatch: %d", len(items))
	}

	// 구 payload 키 (source/count) 는 제거되었다 — items 키만 존재.
	if _, ok := out[0].Payload().Get("source"); ok {
		t.Fatalf("payload.source must be removed")
	}
	if _, ok := out[0].Payload().Get("count"); ok {
		t.Fatalf("payload.count must be removed")
	}

	// 새 metadata: type(단수형)/total_count/offset/count
	if got := metaStr(t, out[0], "type"); got != "device" {
		t.Fatalf("metadata.type expected 'device', got %q", got)
	}
	if got := metaStr(t, out[0], "total_count"); got != "2" {
		t.Fatalf("metadata.total_count expected 2, got %q", got)
	}
	if got := metaStr(t, out[0], "offset"); got != "0" {
		t.Fatalf("metadata.offset expected 0, got %q", got)
	}
	if got := metaStr(t, out[0], "count"); got != "2" {
		t.Fatalf("metadata.count expected 2, got %q", got)
	}
}

// 빈 레지스트리 → 빈 메시지 1개 (트리거당 1메시지 보장)
func TestInventoryNode_Devices_EmptyRegistry_EmitsSingleEmptyMessage(t *testing.T) {
	reg := newFakeDeviceRegistry()
	n := newInventoryNode(t,
		map[string]any{"source": "devices"},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected exactly 1 empty message, got %d", len(out))
	}
	items := getItems(t, out[0])
	if items == nil || len(items) != 0 {
		t.Fatalf("items must be empty non-nil slice, got %v", items)
	}
	if got := metaStr(t, out[0], "total_count"); got != "0" {
		t.Fatalf("total_count expected 0, got %q", got)
	}
	if got := metaStr(t, out[0], "offset"); got != "0" {
		t.Fatalf("offset expected 0, got %q", got)
	}
	if got := metaStr(t, out[0], "count"); got != "0" {
		t.Fatalf("count expected 0, got %q", got)
	}
}

// devices source 는 항상 빈 device.DeviceFilter 로 List 를 호출한다 (조건식으로 통일).
func TestInventoryNode_Devices_AlwaysUsesEmptyDeviceFilter(t *testing.T) {
	d1 := makeDevice("ag1:0.0.16", "A", "lg_icp01", "ag1", true)
	d2 := makeDevice("ag1:0.0.17", "B", "modbus", "ag1", false)
	reg := newFakeDeviceRegistry(d1, d2)

	n := newInventoryNode(t,
		map[string]any{"source": "devices"},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if got := metaStr(t, out[0], "total_count"); got != "2" {
		t.Fatalf("expected all 2 devices, got total_count=%q", got)
	}
	f := reg.lastFilter
	if f.Protocol != "" || f.AgentName != "" || f.Type != "" ||
		f.Online != nil || f.Group != "" || len(f.Tags) != 0 {
		t.Fatalf("expected zero-value filter, got %+v", f)
	}
}

// 항목 스키마는 항상 rich (metadata/state 포함)
func TestInventoryNode_Devices_ItemSchema_AlwaysRich(t *testing.T) {
	d := makeDevice("ag1:0.0.16", "Indoor A", "lg_icp01", "ag1", true)
	reg := newFakeDeviceRegistry(d)
	n := newInventoryNode(t,
		map[string]any{"source": "devices"},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	items := getItems(t, out[0])
	item := items[0]
	if item["id"] != "ag1:0.0.16" {
		t.Fatalf("id mismatch: %v", item["id"])
	}
	if item["protocol"] != "lg_icp01" {
		t.Fatalf("protocol mismatch: %v", item["protocol"])
	}
	if item["online"] != true {
		t.Fatalf("online mismatch: %v", item["online"])
	}
	if _, hasMeta := item["metadata"]; !hasMeta {
		t.Fatalf("expected metadata key (always rich)")
	}
	if _, hasState := item["state"]; !hasState {
		t.Fatalf("expected state key (always rich)")
	}
	if _, ok := item["last_seen"].(string); !ok {
		t.Fatalf("last_seen expected RFC3339 string, got %T", item["last_seen"])
	}
}

// Type() 은 항상 "inventory.event"
func TestInventoryNode_Emit_SetsInventoryEventType(t *testing.T) {
	reg := newFakeDeviceRegistry(makeDevice("d1", "A", "lg_icp01", "ag", true))
	n := newInventoryNode(t,
		map[string]any{"source": "devices"},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if got := out[0].Type(); got != "inventory.event" {
		t.Fatalf("msg.Type() mismatch: got %q, want %q", got, "inventory.event")
	}
	if _, ok := out[0].Metadata().Get("message_type"); ok {
		t.Fatalf("metadata.message_type 키는 부재해야 한다")
	}
}

// 입력 메시지의 metadata 는 출력에 복사되지 않는다.
func TestInventoryNode_DoesNotCopyInputMetadata(t *testing.T) {
	reg := newFakeDeviceRegistry(makeDevice("d1", "A", "x", "ag", true))
	n := newInventoryNode(t,
		map[string]any{"source": "devices"},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	inMsg := message.New(
		message.WithMetadata("trigger.schedule_id", "sched-1"),
		message.WithMetadata("inventory.source", "should_not_appear"),
	)
	out, err := n.Process(context.Background(), inMsg)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if _, ok := out[0].Metadata().Get("trigger.schedule_id"); ok {
		t.Fatalf("input metadata trigger.schedule_id must NOT be copied")
	}
	if _, ok := out[0].Metadata().Get("inventory.source"); ok {
		t.Fatalf("legacy inventory.source key must NOT exist")
	}
	// 새 metadata 만 존재
	if got := metaStr(t, out[0], "type"); got != "device" {
		t.Fatalf("metadata.type expected 'device', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// metadata.type 단수형 매핑 (모든 source)
// ---------------------------------------------------------------------------

func TestInventoryNode_MetadataType_SingularPerSource(t *testing.T) {
	devReg := newFakeDeviceRegistry(makeDevice("d1", "A", "x", "ag", true))
	agtMgr := &fakeAgentManager{agents: []agent.Agent{&fakeAgent{id: "a1", name: "ag1", agType: "serial", state: lifecycle.StateRunning}}}
	flowReg := &fakeFlowRegistry{flows: []FlowSummary{{ID: "f1", Name: "flow-1", State: "running"}}}
	nodeReg := NewRegistry()

	opts := []NodeOption{
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return devReg }),
		WithAgentManagerFunc(func() agent.Manager { return agtMgr }),
		WithFlowRegistryFunc(func() FlowRegistry { return flowReg }),
		WithNodeRegistryFunc(func() *Registry { return nodeReg }),
	}

	cases := map[string]string{
		"devices": "device",
		"agents":  "agent",
		"nodes":   "node",
		"flows":   "flow",
	}
	for src, wantType := range cases {
		t.Run(src, func(t *testing.T) {
			n := newInventoryNode(t, map[string]any{"source": src}, opts...)
			if err := n.Init(context.Background()); err != nil {
				t.Fatalf("init: %v", err)
			}
			out, err := n.Process(context.Background(), message.New())
			if err != nil {
				t.Fatalf("process: %v", err)
			}
			if len(out) != 1 {
				t.Fatalf("expected 1 message, got %d", len(out))
			}
			if got := metaStr(t, out[0], "type"); got != wantType {
				t.Fatalf("metadata.type expected %q, got %q", wantType, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// condition - 조건식 필터 (모든 source)
// ---------------------------------------------------------------------------

func TestInventoryNode_Condition_FiltersDevices(t *testing.T) {
	reg := newFakeDeviceRegistry(
		makeDevice("d1", "A", "lg_icp01", "ag", true),
		makeDevice("d2", "B", "modbus", "ag", false),
		makeDevice("d3", "C", "lg_icp01", "ag", true),
	)
	n := newInventoryNode(t,
		map[string]any{
			"source":    "devices",
			"condition": "$.payload.online == true",
		},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	items := getItems(t, out[0])
	if len(items) != 2 {
		t.Fatalf("condition online==true expected 2 items, got %d", len(items))
	}
	for _, it := range items {
		if it["online"] != true {
			t.Fatalf("filtered item must be online, got %v", it["online"])
		}
	}
	if got := metaStr(t, out[0], "total_count"); got != "2" {
		t.Fatalf("total_count must reflect post-filter count 2, got %q", got)
	}
}

func TestInventoryNode_Condition_AppliesToNonDeviceSource(t *testing.T) {
	mgr := &fakeAgentManager{
		agents: []agent.Agent{
			&fakeAgent{id: "a1", name: "serial-agent", agType: "serial", state: lifecycle.StateRunning},
			&fakeAgent{id: "a2", name: "mqtt-agent", agType: "mqtt", state: lifecycle.StateStopped},
		},
	}
	n := newInventoryNode(t,
		map[string]any{
			"source":    "agents",
			"condition": "$.payload.type == 'serial'",
		},
		WithAgentManagerFunc(func() agent.Manager { return mgr }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	items := getItems(t, out[0])
	if len(items) != 1 {
		t.Fatalf("condition type=='serial' expected 1 agent, got %d", len(items))
	}
	if items[0]["name"] != "serial-agent" {
		t.Fatalf("expected serial-agent, got %v", items[0]["name"])
	}
}

// 조건식이 모든 항목을 제거하면 빈 메시지 1개를 방출한다.
func TestInventoryNode_Condition_AllFilteredOut_EmitsEmptyMessage(t *testing.T) {
	reg := newFakeDeviceRegistry(
		makeDevice("d1", "A", "x", "ag", false),
		makeDevice("d2", "B", "x", "ag", false),
	)
	n := newInventoryNode(t,
		map[string]any{
			"source":    "devices",
			"condition": "$.payload.online == true",
		},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 empty message, got %d", len(out))
	}
	items := getItems(t, out[0])
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
	if got := metaStr(t, out[0], "total_count"); got != "0" {
		t.Fatalf("total_count expected 0, got %q", got)
	}
}

// 조건식 컴파일 실패 → NewInventoryNode 에서 에러
func TestInventoryNode_Condition_CompileError_FactoryError(t *testing.T) {
	def := flow.NodeDef{
		ID:   "n1",
		Type: "inventory",
		Config: map[string]any{
			"source":    "devices",
			"condition": "this is not a valid (( expression",
		},
	}
	_, err := NewInventoryNode(def)
	if err == nil {
		t.Fatalf("expected compile error for invalid condition, got nil")
	}
}

// 빈 condition 문자열 → 필터 없음 (전체 통과)
func TestInventoryNode_Condition_Empty_NoFilter(t *testing.T) {
	reg := newFakeDeviceRegistry(
		makeDevice("d1", "A", "x", "ag", true),
		makeDevice("d2", "B", "x", "ag", false),
	)
	n := newInventoryNode(t,
		map[string]any{"source": "devices", "condition": ""},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	items := getItems(t, out[0])
	if len(items) != 2 {
		t.Fatalf("empty condition must pass all, got %d", len(items))
	}
}

// ---------------------------------------------------------------------------
// fields - 화이트리스트 투영
// ---------------------------------------------------------------------------

func TestInventoryNode_Fields_CommaSeparatedString(t *testing.T) {
	reg := newFakeDeviceRegistry(makeDevice("d1", "A", "lg_icp01", "ag", true))
	n := newInventoryNode(t,
		map[string]any{
			"source": "devices",
			"fields": " id , name ",
		},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	items := getItems(t, out[0])
	item := items[0]
	if len(item) != 2 {
		t.Fatalf("projection expected 2 keys (id,name), got %d: %v", len(item), item)
	}
	if item["id"] != "d1" || item["name"] != "A" {
		t.Fatalf("projected values mismatch: %v", item)
	}
	if _, ok := item["protocol"]; ok {
		t.Fatalf("non-whitelisted key 'protocol' must be omitted")
	}
}

func TestInventoryNode_Fields_SliceForm(t *testing.T) {
	reg := newFakeDeviceRegistry(makeDevice("d1", "A", "lg_icp01", "ag", true))
	n := newInventoryNode(t,
		map[string]any{
			"source": "devices",
			"fields": []any{"id", "protocol"},
		},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	item := getItems(t, out[0])[0]
	if len(item) != 2 {
		t.Fatalf("projection expected 2 keys, got %d: %v", len(item), item)
	}
	if item["id"] != "d1" || item["protocol"] != "lg_icp01" {
		t.Fatalf("projected values mismatch: %v", item)
	}
}

// fields 에 존재하지 않는 키는 그냥 생략 (에러 아님)
func TestInventoryNode_Fields_MissingKeySkipped(t *testing.T) {
	reg := newFakeDeviceRegistry(makeDevice("d1", "A", "x", "ag", true))
	n := newInventoryNode(t,
		map[string]any{
			"source": "devices",
			"fields": "id,nonexistent_key",
		},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	item := getItems(t, out[0])[0]
	if len(item) != 1 {
		t.Fatalf("missing key must be skipped, expected 1 key, got %d: %v", len(item), item)
	}
	if item["id"] != "d1" {
		t.Fatalf("id must remain: %v", item)
	}
}

// 빈 fields 는 전체 필드 유지
func TestInventoryNode_Fields_EmptyKeepsAll(t *testing.T) {
	reg := newFakeDeviceRegistry(makeDevice("d1", "A", "x", "ag", true))
	n := newInventoryNode(t,
		map[string]any{"source": "devices", "fields": "  ,  "},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	item := getItems(t, out[0])[0]
	if _, ok := item["metadata"]; !ok {
		t.Fatalf("empty fields must keep all keys (metadata present)")
	}
	if item["id"] != "d1" {
		t.Fatalf("id must remain")
	}
}

// ---------------------------------------------------------------------------
// max_items - 청킹
// ---------------------------------------------------------------------------

func TestInventoryNode_MaxItems_ChunksMessages(t *testing.T) {
	devs := []device.Device{
		makeDevice("d1", "A", "x", "ag", true),
		makeDevice("d2", "B", "x", "ag", true),
		makeDevice("d3", "C", "x", "ag", true),
		makeDevice("d4", "D", "x", "ag", true),
		makeDevice("d5", "E", "x", "ag", true),
	}
	reg := newFakeDeviceRegistry(devs...)
	n := newInventoryNode(t,
		map[string]any{"source": "devices", "max_items": 2},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("5 items max_items=2 expected 3 messages, got %d", len(out))
	}
	wantCounts := []string{"2", "2", "1"}
	wantOffsets := []string{"0", "2", "4"}
	for i, m := range out {
		if got := metaStr(t, m, "total_count"); got != "5" {
			t.Fatalf("msg[%d] total_count expected 5, got %q", i, got)
		}
		if got := metaStr(t, m, "offset"); got != wantOffsets[i] {
			t.Fatalf("msg[%d] offset expected %s, got %q", i, wantOffsets[i], got)
		}
		if got := metaStr(t, m, "count"); got != wantCounts[i] {
			t.Fatalf("msg[%d] count expected %s, got %q", i, wantCounts[i], got)
		}
		items := getItems(t, m)
		cnt, _ := strconv.Atoi(wantCounts[i])
		if len(items) != cnt {
			t.Fatalf("msg[%d] items length expected %d, got %d", i, cnt, len(items))
		}
	}
}

// max_items 가 number(float64)로 들어와도 파싱
func TestInventoryNode_MaxItems_FloatParsing(t *testing.T) {
	devs := []device.Device{
		makeDevice("d1", "A", "x", "ag", true),
		makeDevice("d2", "B", "x", "ag", true),
		makeDevice("d3", "C", "x", "ag", true),
	}
	reg := newFakeDeviceRegistry(devs...)
	n := newInventoryNode(t,
		map[string]any{"source": "devices", "max_items": float64(2)},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("3 items max_items=2 expected 2 messages, got %d", len(out))
	}
}

// max_items<=0 → 무제한 (1개 메시지)
func TestInventoryNode_MaxItems_ZeroUnlimited(t *testing.T) {
	devs := []device.Device{
		makeDevice("d1", "A", "x", "ag", true),
		makeDevice("d2", "B", "x", "ag", true),
		makeDevice("d3", "C", "x", "ag", true),
	}
	reg := newFakeDeviceRegistry(devs...)
	n := newInventoryNode(t,
		map[string]any{"source": "devices", "max_items": 0},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("max_items=0 expected 1 message, got %d", len(out))
	}
	if got := metaStr(t, out[0], "count"); got != "3" {
		t.Fatalf("count expected 3, got %q", got)
	}
}

// max_items 가 항목 수보다 클 때 → 1개 메시지 전체
func TestInventoryNode_MaxItems_LargerThanTotal(t *testing.T) {
	reg := newFakeDeviceRegistry(
		makeDevice("d1", "A", "x", "ag", true),
		makeDevice("d2", "B", "x", "ag", true),
	)
	n := newInventoryNode(t,
		map[string]any{"source": "devices", "max_items": 100},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out))
	}
	if got := metaStr(t, out[0], "count"); got != "2" {
		t.Fatalf("count expected 2, got %q", got)
	}
}

// condition + fields + max_items 조합 동작 (순서: condition → fields → chunk)
func TestInventoryNode_CombinedConditionFieldsMaxItems(t *testing.T) {
	reg := newFakeDeviceRegistry(
		makeDevice("d1", "A", "x", "ag", true),
		makeDevice("d2", "B", "x", "ag", false),
		makeDevice("d3", "C", "x", "ag", true),
		makeDevice("d4", "D", "x", "ag", true),
	)
	n := newInventoryNode(t,
		map[string]any{
			"source":    "devices",
			"condition": "$.payload.online == true",
			"fields":    "id",
			"max_items": 2,
		},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	// online==true 3개 → max_items=2 → 2 메시지 (2,1)
	if len(out) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(out))
	}
	for i, m := range out {
		if got := metaStr(t, m, "total_count"); got != "3" {
			t.Fatalf("msg[%d] total_count expected 3, got %q", i, got)
		}
		for _, it := range getItems(t, m) {
			if len(it) != 1 {
				t.Fatalf("msg[%d] item must be projected to 1 key (id), got %v", i, it)
			}
			if _, ok := it["id"]; !ok {
				t.Fatalf("msg[%d] item must have id key", i)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// agents / flows / nodes 항목 스키마
// ---------------------------------------------------------------------------

// fakeAgent implements agent.Agent for tests.
type fakeAgent struct {
	id     string
	name   string
	agType string
	state  lifecycle.State
	stats  agent.StatsSnapshot
}

func (a *fakeAgent) Init(agent.AgentConfig) error      { return nil }
func (a *fakeAgent) Start(context.Context) error       { return nil }
func (a *fakeAgent) Stop(context.Context) error        { return nil }
func (a *fakeAgent) Pause(context.Context) error       { return nil }
func (a *fakeAgent) Resume(context.Context) error      { return nil }
func (a *fakeAgent) Health() agent.HealthStatus        { return agent.HealthStatus{} }
func (a *fakeAgent) Process([]byte) ([]byte, error)    { return nil, nil }
func (a *fakeAgent) Configure(agent.AgentConfig) error { return nil }
func (a *fakeAgent) ID() string                        { return a.id }
func (a *fakeAgent) Name() string                      { return a.name }
func (a *fakeAgent) Type() string                      { return a.agType }
func (a *fakeAgent) Info() agent.AgentInfo {
	return agent.AgentInfo{
		ID:    a.id,
		Name:  a.name,
		Type:  a.agType,
		State: a.state,
		Stats: a.stats,
	}
}
func (a *fakeAgent) Stats() agent.StatsSnapshot { return a.stats }

type fakeAgentManager struct {
	agents []agent.Agent
}

func (m *fakeAgentManager) Create(agent.AgentConfig) (agent.Agent, error) { return nil, nil }
func (m *fakeAgentManager) Start(context.Context, string) error           { return nil }
func (m *fakeAgentManager) Stop(context.Context, string) error            { return nil }
func (m *fakeAgentManager) Restart(context.Context, string) error         { return nil }
func (m *fakeAgentManager) Delete(string) error                           { return nil }
func (m *fakeAgentManager) Get(id string) (agent.Agent, error) {
	for _, a := range m.agents {
		if a.ID() == id {
			return a, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (m *fakeAgentManager) List() []agent.Agent                   { return m.agents }
func (m *fakeAgentManager) Shutdown(context.Context) error        { return nil }
func (m *fakeAgentManager) Summary() agent.ManagerSummary         { return agent.ManagerSummary{} }
func (m *fakeAgentManager) SetAgentLogLevel(string, string) error { return nil }

func TestInventoryNode_Agents_EmitsAgentList(t *testing.T) {
	mgr := &fakeAgentManager{
		agents: []agent.Agent{
			&fakeAgent{id: "a1", name: "serial-agent", agType: "serial", state: lifecycle.StateRunning},
			&fakeAgent{id: "a2", name: "mqtt-agent", agType: "mqtt", state: lifecycle.StateStopped},
		},
	}
	n := newInventoryNode(t,
		map[string]any{"source": "agents"},
		WithAgentManagerFunc(func() agent.Manager { return mgr }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out))
	}
	if got := metaStr(t, out[0], "total_count"); got != "2" {
		t.Fatalf("total_count expected 2, got %q", got)
	}
	items := getItems(t, out[0])
	if items[0]["id"] != "a1" || items[0]["name"] != "serial-agent" {
		t.Fatalf("first item mismatch: %+v", items[0])
	}
	if items[0]["state"] != string(lifecycle.StateRunning) {
		t.Fatalf("state must be lifecycle string, got %v", items[0]["state"])
	}
	if _, hasInfo := items[0]["info"]; !hasInfo {
		t.Fatalf("info key expected (always rich)")
	}
}

type fakeFlowRegistry struct {
	flows []FlowSummary
}

func (f *fakeFlowRegistry) FlowSummaries() []FlowSummary { return f.flows }

func TestInventoryNode_Flows_EmitsFlowSummary(t *testing.T) {
	freg := &fakeFlowRegistry{
		flows: []FlowSummary{
			{ID: "f1", Name: "my-flow", State: "running", NodeCount: 5, WireCount: 4, Extra: map[string]any{"uptime_seconds": 120}},
		},
	}
	n := newInventoryNode(t,
		map[string]any{"source": "flows"},
		WithFlowRegistryFunc(func() FlowRegistry { return freg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, _ := n.Process(context.Background(), message.New())
	items := getItems(t, out[0])
	if items[0]["id"] != "f1" || items[0]["name"] != "my-flow" {
		t.Fatalf("flow item mismatch: %+v", items[0])
	}
	if items[0]["node_count"] != 5 {
		t.Fatalf("node_count mismatch: %v", items[0]["node_count"])
	}
	if items[0]["wire_count"] != 4 {
		t.Fatalf("wire_count mismatch: %v", items[0]["wire_count"])
	}
	if _, hasExtra := items[0]["extra"]; !hasExtra {
		t.Fatalf("extra expected (always rich)")
	}
}

func TestInventoryNode_Nodes_EmitsNodeTypeMeta(t *testing.T) {
	reg := NewRegistry()
	if !reg.Has("inventory") {
		_ = reg.RegisterWithMeta("inventory", NewInventoryNode, NodeTypeMeta{
			Type:        "inventory",
			Category:    "processing",
			Description: "test",
			Source:      "builtin",
		})
	}
	n := newInventoryNode(t,
		map[string]any{"source": "nodes"},
		WithNodeRegistryFunc(func() *Registry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, _ := n.Process(context.Background(), message.New())
	items := getItems(t, out[0])
	if len(items) == 0 {
		t.Fatalf("expected at least one node type")
	}
	var foundInventory bool
	for _, it := range items {
		if it["type"] == "inventory" {
			foundInventory = true
			if it["category"] != "processing" {
				t.Fatalf("inventory category mismatch: %v", it["category"])
			}
			if it["origin"] == nil {
				t.Fatalf("origin key required")
			}
		}
	}
	if !foundInventory {
		t.Fatalf("inventory node must be in nodes inventory")
	}
}

// ---------------------------------------------------------------------------
// Registry / read-only / concurrency / shutdown
// ---------------------------------------------------------------------------

func TestInventoryNode_RegisteredAsBuiltin(t *testing.T) {
	reg := NewRegistry()
	if !reg.Has("inventory") {
		t.Fatalf("inventory must be registered as builtin")
	}
	meta, ok := reg.TypeMeta("inventory")
	if !ok {
		t.Fatalf("TypeMeta(inventory) must return ok=true")
	}
	if meta.Type != "inventory" {
		t.Fatalf("Type expected 'inventory', got %s", meta.Type)
	}
	if meta.Category != "processing" {
		t.Fatalf("Category expected 'processing', got %s", meta.Category)
	}
	if meta.Source != "builtin" {
		t.Fatalf("Source expected 'builtin', got %s", meta.Source)
	}
	if !strings.Contains(meta.Description, "인벤토리") && !strings.Contains(meta.Description, "스냅샷") {
		t.Fatalf("description must contain '인벤토리' or '스냅샷', got %s", meta.Description)
	}
}

func TestInventoryNode_ReadOnly_NoMutation(t *testing.T) {
	reg := newFakeDeviceRegistry(
		makeDevice("d1", "A", "x", "ag", true),
		makeDevice("d2", "B", "x", "ag", true),
	)
	n := newInventoryNode(t,
		map[string]any{"source": "devices"},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	initialCount := reg.Count()
	for i := 0; i < 1000; i++ {
		if _, err := n.Process(context.Background(), message.New()); err != nil {
			t.Fatalf("process[%d]: %v", i, err)
		}
	}
	if reg.Count() != initialCount {
		t.Fatalf("registry count changed: %d to %d", initialCount, reg.Count())
	}
	if reg.listCallCount != 1000 {
		t.Fatalf("List call count expected 1000, got %d", reg.listCallCount)
	}
}

func TestInventoryNode_ConcurrentProcess_RaceSafe(t *testing.T) {
	reg := newFakeDeviceRegistry(
		makeDevice("d1", "A", "x", "ag", true),
		makeDevice("d2", "B", "x", "ag", true),
		makeDevice("d3", "C", "x", "ag", true),
	)
	// max_items=1 → 항목당 1메시지 (3개) fan-out 으로 동시성 부하 검증
	n := newInventoryNode(t,
		map[string]any{"source": "devices", "max_items": 1},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}

	var wg sync.WaitGroup
	const goroutines = 10
	const iterations = 100
	errCh := make(chan error, goroutines*iterations)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				out, err := n.Process(context.Background(), message.New())
				if err != nil {
					errCh <- err
					return
				}
				if len(out) != 3 {
					errCh <- fmt.Errorf("expected 3 messages, got %d", len(out))
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

func TestInventoryNode_Shutdown_Idempotent(t *testing.T) {
	reg := newFakeDeviceRegistry()
	n := newInventoryNode(t,
		map[string]any{"source": "devices"},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := n.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown 1st: %v", err)
	}
	if err := n.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown 2nd: %v", err)
	}
}

func TestInventoryNode_AllSources_WithAllOptionsInjected_ProcessSucceeds(t *testing.T) {
	devReg := newFakeDeviceRegistry(makeDevice("d1", "A", "x", "ag", true))
	agtMgr := &fakeAgentManager{agents: []agent.Agent{&fakeAgent{id: "a1", name: "ag1", agType: "serial", state: lifecycle.StateRunning}}}
	flowReg := &fakeFlowRegistry{flows: []FlowSummary{{ID: "f1", Name: "flow-1", State: "running"}}}
	nodeReg := NewRegistry()

	opts := []NodeOption{
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return devReg }),
		WithAgentManagerFunc(func() agent.Manager { return agtMgr }),
		WithFlowRegistryFunc(func() FlowRegistry { return flowReg }),
		WithNodeRegistryFunc(func() *Registry { return nodeReg }),
	}

	for _, src := range []string{"devices", "agents", "flows", "nodes"} {
		t.Run(src, func(t *testing.T) {
			n := newInventoryNode(t, map[string]any{"source": src}, opts...)
			if err := n.Init(context.Background()); err != nil {
				t.Fatalf("init: %v", err)
			}
			out, err := n.Process(context.Background(), message.New())
			if err != nil {
				t.Fatalf("process: %v", err)
			}
			if len(out) != 1 {
				t.Fatalf("expected 1 message, got %d", len(out))
			}
			if _, ok := out[0].Payload().Get("items"); !ok {
				t.Fatalf("payload.items missing for source %s", src)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// device_uuid 테스트 헬퍼 (production UID resolve 경로 재현)
// ---------------------------------------------------------------------------

// fakeDeviceIDRepo 는 agent.DeviceIDRepository 의 테스트용 구현이다.
type fakeDeviceIDRepo struct {
	mu       sync.Mutex
	mapping  map[string]string // key: "agentName/unitID" → UUID
	getCalls int
}

func newFakeDeviceIDRepo(mapping map[string]string) *fakeDeviceIDRepo {
	return &fakeDeviceIDRepo{mapping: mapping}
}

func (r *fakeDeviceIDRepo) GetOrCreate(_ context.Context, agentName, unitID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getCalls++
	key := agentName + "/" + unitID
	if uuid, ok := r.mapping[key]; ok {
		return uuid, nil
	}
	return "", nil
}

func (r *fakeDeviceIDRepo) Get(_ context.Context, agentName, unitID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := agentName + "/" + unitID
	if uuid, ok := r.mapping[key]; ok {
		return uuid, nil
	}
	return "", nil
}

// withDeviceIDRepo 는 테스트 동안 패키지-레벨 DeviceIDRepository 를 임시 주입한다.
func withDeviceIDRepo(t *testing.T, repo agent.DeviceIDRepository) {
	t.Helper()
	prev := agent.GetDeviceIDRepository()
	agent.SetDeviceIDRepository(repo)
	t.Cleanup(func() {
		agent.SetDeviceIDRepository(prev)
	})
}
