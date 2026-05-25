// Package node - inventory_test.go: SPEC-INVENTORY-001 인수 테스트
//
// 본 파일은 Inventory 노드 (devices/agents/nodes/flows 인벤토리 스냅샷 emit)의
// TDD 단위 테스트를 제공한다. Phase 1~4 의 acceptance.md 시나리오와 1:1 매핑된다.
package node

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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

func (d *fakeDevice) ID() string                      { return d.id }
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

// ---------------------------------------------------------------------------
// Phase 1 RED — Factory 검증 (M1, M2 일부)
// ---------------------------------------------------------------------------

// AC1.3: source 누락 시 ErrInventoryInvalidSource
func TestInventoryNode_FactoryWithMissingSource_ReturnsError(t *testing.T) {
	def := flow.NodeDef{
		ID:     "n1",
		Name:   "inv",
		Type:   "inventory",
		Config: map[string]any{},
	}
	n, err := NewInventoryNode(def)
	if n != nil {
		t.Fatalf("expected nil node when source missing, got %v", n)
	}
	if !errors.Is(err, ErrInventoryInvalidSource) {
		t.Fatalf("expected ErrInventoryInvalidSource, got %v", err)
	}
}

// AC1.4: 알 수 없는 source enum
func TestInventoryNode_FactoryWithInvalidSource_ReturnsError(t *testing.T) {
	def := flow.NodeDef{
		ID:     "n1",
		Type:   "inventory",
		Config: map[string]any{"source": "unknown_kind"},
	}
	_, err := NewInventoryNode(def)
	if !errors.Is(err, ErrInventoryInvalidSource) {
		t.Fatalf("expected ErrInventoryInvalidSource, got %v", err)
	}
}

// AC1.5: 4종 source 모두 팩토리 통과
func TestInventoryNode_FactoryAllValidSources_ReturnsNode(t *testing.T) {
	sources := []string{
		InventorySourceDevices, InventorySourceAgents,
		InventorySourceNodes, InventorySourceFlows,
	}
	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			def := flow.NodeDef{
				ID:     "n-" + src,
				Type:   "inventory",
				Config: map[string]any{"source": src},
			}
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

// AC2.1: 기본 emit_shape 는 array
func TestInventoryNode_DefaultEmitShape_IsArray(t *testing.T) {
	def := flow.NodeDef{
		ID:     "n1",
		Type:   "inventory",
		Config: map[string]any{"source": "devices"},
	}
	n, err := NewInventoryNode(def)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	inv := n.(*InventoryNode)
	if inv.emitShape != InventoryShapeArray {
		t.Fatalf("expected default emit_shape=array, got %s", inv.emitShape)
	}
}

// AC2.7: 잘못된 emit_shape
func TestInventoryNode_FactoryWithInvalidEmitShape_ReturnsError(t *testing.T) {
	def := flow.NodeDef{
		ID:   "n1",
		Type: "inventory",
		Config: map[string]any{
			"source":     "devices",
			"emit_shape": "batch",
		},
	}
	_, err := NewInventoryNode(def)
	if !errors.Is(err, ErrInventoryInvalidEmitShape) {
		t.Fatalf("expected ErrInventoryInvalidEmitShape, got %v", err)
	}
}

// AC1.2: 기본 포트 정의 확인 (in, out, error)
func TestInventoryNode_DefaultPorts(t *testing.T) {
	def := flow.NodeDef{
		ID:   "n1",
		Type: "inventory",
		Config: map[string]any{
			"source": "devices",
		},
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
// Phase 1 RED — devices/array shape (M2, M3, M5 일부)
// ---------------------------------------------------------------------------

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

// AC2.2: devices/array - 단일 메시지 emit
func TestInventoryNode_DevicesArrayShape_EmitsSingleMessage(t *testing.T) {
	reg := newFakeDeviceRegistry(
		makeDevice("ag1:0.0.16", "Indoor A", "lgcnp", "ag1", true),
		makeDevice("ag1:0.0.17", "Indoor B", "lgcnp", "ag1", true),
	)
	n := newInventoryNode(t,
		map[string]any{"source": "devices", "emit_shape": "array"},
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
		t.Fatalf("expected 1 message in array mode, got %d", len(out))
	}

	payload := out[0].Payload()
	if v, _ := payload.Get("source"); v != "devices" {
		t.Fatalf("payload.source mismatch: %v", v)
	}
	if v, _ := payload.Get("count"); v != 2 {
		t.Fatalf("payload.count mismatch: %v", v)
	}
	items, ok := payload.Get("items")
	if !ok {
		t.Fatalf("payload.items missing")
	}
	itemsList, ok := items.([]map[string]any)
	if !ok {
		t.Fatalf("payload.items must be []map[string]any, got %T", items)
	}
	if len(itemsList) != 2 {
		t.Fatalf("items length mismatch: %d", len(itemsList))
	}

	// metadata 확인
	if v, _ := out[0].Metadata().Get("inventory.source"); v != "devices" {
		t.Fatalf("metadata.inventory.source mismatch: %s", v)
	}
	if v, _ := out[0].Metadata().Get("inventory.count"); v != "2" {
		t.Fatalf("metadata.inventory.count mismatch: %s", v)
	}
}

// AC2.3: devices/array - 빈 레지스트리에서 count=0
func TestInventoryNode_DevicesArrayShape_EmptyRegistry_EmitsCountZero(t *testing.T) {
	reg := newFakeDeviceRegistry()
	n := newInventoryNode(t,
		map[string]any{"source": "devices", "emit_shape": "array"},
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
	if v, _ := out[0].Payload().Get("count"); v != 0 {
		t.Fatalf("count=0 expected, got %v", v)
	}
	items, _ := out[0].Payload().Get("items")
	il, ok := items.([]map[string]any)
	if !ok {
		t.Fatalf("items must be slice, got %T", items)
	}
	if il == nil || len(il) != 0 {
		t.Fatalf("items must be empty non-nil slice, got %v", il)
	}
}

// AC3.1: filter 가 DeviceRegistry.List 에 전달됨
func TestInventoryNode_Devices_WithFilter_AppliesDeviceFilter(t *testing.T) {
	d1 := makeDevice("ag1:0.0.16", "A", "lgcnp", "ag1", true)
	d2 := makeDevice("ag1:0.0.17", "B", "modbus", "ag1", true)
	reg := newFakeDeviceRegistry(d1, d2)

	onlineTrue := true
	n := newInventoryNode(t,
		map[string]any{
			"source":     "devices",
			"emit_shape": "array",
			"filter": map[string]any{
				"protocol": "lgcnp",
				"online":   true,
			},
		},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	_, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}

	// fake registry 의 lastFilter 확인
	if reg.lastFilter.Protocol != "lgcnp" {
		t.Fatalf("filter.protocol expected lgcnp, got %s", reg.lastFilter.Protocol)
	}
	if reg.lastFilter.Online == nil || *reg.lastFilter.Online != onlineTrue {
		t.Fatalf("filter.online expected *true, got %v", reg.lastFilter.Online)
	}
}

// AC3.2: filter 누락 시 zero-value 필터 사용 (모든 디바이스 반환)
func TestInventoryNode_Devices_WithoutFilter_UsesEmptyFilter(t *testing.T) {
	d1 := makeDevice("ag1:0.0.16", "A", "lgcnp", "ag1", true)
	d2 := makeDevice("ag1:0.0.17", "B", "modbus", "ag1", false)
	reg := newFakeDeviceRegistry(d1, d2)

	n := newInventoryNode(t,
		map[string]any{"source": "devices", "emit_shape": "array"},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if v, _ := out[0].Payload().Get("count"); v != 2 {
		t.Fatalf("expected all 2 devices, got %v", v)
	}
	f := reg.lastFilter
	if f.Protocol != "" || f.AgentName != "" || f.Type != "" ||
		f.Online != nil || f.Group != "" || len(f.Tags) != 0 {
		t.Fatalf("expected zero-value filter, got %+v", f)
	}
}

// AC4.1: devices source 인데 DeviceRegistry 미주입 시 Init 에러
func TestInventoryNode_DevicesSource_WithoutRegistryOption_InitError(t *testing.T) {
	n := newInventoryNode(t,
		map[string]any{"source": "devices"},
		// no WithDeviceRegistryFunc
	)
	err := n.Init(context.Background())
	if !errors.Is(err, ErrInventoryDeviceRegistryNotAvailable) {
		t.Fatalf("expected ErrInventoryDeviceRegistryNotAvailable, got %v", err)
	}
}

// AC5.3: devices 항목 스키마 (include_metadata=true 기본)
func TestInventoryNode_DevicesArrayShape_ItemSchema_FullMetadata(t *testing.T) {
	d := makeDevice("ag1:0.0.16", "Indoor A", "lgcnp", "ag1", true)
	reg := newFakeDeviceRegistry(d)
	n := newInventoryNode(t,
		map[string]any{"source": "devices", "emit_shape": "array"},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	itemsRaw, _ := out[0].Payload().Get("items")
	items := itemsRaw.([]map[string]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0]
	if item["id"] != "ag1:0.0.16" {
		t.Fatalf("id mismatch: %v", item["id"])
	}
	if item["protocol"] != "lgcnp" {
		t.Fatalf("protocol mismatch: %v", item["protocol"])
	}
	if item["online"] != true {
		t.Fatalf("online mismatch: %v", item["online"])
	}
	caps, ok := item["capabilities"].([]string)
	if !ok || len(caps) != 2 {
		t.Fatalf("capabilities mismatch: %v", item["capabilities"])
	}
	if _, hasMeta := item["metadata"]; !hasMeta {
		t.Fatalf("expected metadata key when include_metadata=true")
	}
	if _, hasState := item["state"]; !hasState {
		t.Fatalf("expected state key when include_metadata=true")
	}
	// last_seen 은 RFC3339 string
	if _, ok := item["last_seen"].(string); !ok {
		t.Fatalf("last_seen expected string (RFC3339), got %T", item["last_seen"])
	}
}

// AC5.4: include_metadata=false → metadata/state 생략
func TestInventoryNode_DevicesArrayShape_ItemSchema_NoMetadata(t *testing.T) {
	d := makeDevice("ag1:0.0.16", "Indoor A", "lgcnp", "ag1", true)
	reg := newFakeDeviceRegistry(d)
	n := newInventoryNode(t,
		map[string]any{
			"source":           "devices",
			"emit_shape":       "array",
			"include_metadata": false,
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
	itemsRaw, _ := out[0].Payload().Get("items")
	items := itemsRaw.([]map[string]any)
	item := items[0]
	if _, hasMeta := item["metadata"]; hasMeta {
		t.Fatalf("metadata must be omitted when include_metadata=false")
	}
	if _, hasState := item["state"]; hasState {
		t.Fatalf("state must be omitted when include_metadata=false")
	}
	// 핵심 필드는 여전히 존재해야 함
	if item["id"] != "ag1:0.0.16" {
		t.Fatalf("id must remain")
	}
}

// ---------------------------------------------------------------------------
// Phase 2 RED — per_item, agents/nodes/flows source (M2 per_item, M5 항목 스키마)
// ---------------------------------------------------------------------------

// AC2.4: per_item - N개 메시지 emit
func TestInventoryNode_DevicesPerItem_EmitsNMessages(t *testing.T) {
	devs := []device.Device{
		makeDevice("ag1:0.0.16", "A", "lgcnp", "ag1", true),
		makeDevice("ag1:0.0.17", "B", "lgcnp", "ag1", true),
		makeDevice("ag1:0.0.18", "C", "lgcnp", "ag1", true),
	}
	reg := newFakeDeviceRegistry(devs...)
	n := newInventoryNode(t,
		map[string]any{"source": "devices", "emit_shape": "per_item"},
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
		t.Fatalf("expected 3 per_item messages, got %d", len(out))
	}

	// per_item payload 는 단일 item 객체 (wrapper 없음): id 키가 직접 노출
	for i, m := range out {
		if v, ok := m.Payload().Get("id"); !ok || v == nil {
			t.Fatalf("per_item[%d] missing payload.id", i)
		}
		if _, ok := m.Payload().Get("count"); ok {
			t.Fatalf("per_item[%d] must NOT have wrapper key 'count'", i)
		}
		if _, ok := m.Payload().Get("items"); ok {
			t.Fatalf("per_item[%d] must NOT have wrapper key 'items'", i)
		}
	}
}

// AC2.5: per_item, 빈 레지스트리 → 0개 메시지
func TestInventoryNode_DevicesPerItem_EmptyRegistry_EmitsZeroMessages(t *testing.T) {
	reg := newFakeDeviceRegistry()
	n := newInventoryNode(t,
		map[string]any{"source": "devices", "emit_shape": "per_item"},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected 0 messages for empty registry in per_item mode, got %d", len(out))
	}
}

// AC2.6: per_item - inventory.index/total metadata 정확
func TestInventoryNode_DevicesPerItem_IndexMetadataCorrect(t *testing.T) {
	devs := []device.Device{
		makeDevice("ag1:0.0.16", "A", "lgcnp", "ag1", true),
		makeDevice("ag1:0.0.17", "B", "lgcnp", "ag1", true),
		makeDevice("ag1:0.0.18", "C", "lgcnp", "ag1", true),
	}
	reg := newFakeDeviceRegistry(devs...)
	n := newInventoryNode(t,
		map[string]any{"source": "devices", "emit_shape": "per_item"},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}

	for i, m := range out {
		idx, _ := m.Metadata().Get("inventory.index")
		if idx != strconv.Itoa(i) {
			t.Fatalf("message[%d].inventory.index expected %d, got %s", i, i, idx)
		}
		if v, _ := m.Metadata().Get("inventory.total"); v != "3" {
			t.Fatalf("message[%d].inventory.total expected 3, got %s", i, v)
		}
		if v, _ := m.Metadata().Get("inventory.count"); v != "3" {
			t.Fatalf("message[%d].inventory.count expected 3, got %s", i, v)
		}
	}

	// array 모드 출력에는 inventory.index/total 가 없어야 한다 → 별도 검증.
}

// AC5.9: array 모드는 index/total metadata 없음
func TestInventoryNode_DevicesArrayShape_HasNoIndexTotalMetadata(t *testing.T) {
	reg := newFakeDeviceRegistry(makeDevice("d1", "A", "x", "ag", true))
	n := newInventoryNode(t,
		map[string]any{"source": "devices", "emit_shape": "array"},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, _ := n.Process(context.Background(), message.New())
	if _, ok := out[0].Metadata().Get("inventory.index"); ok {
		t.Fatalf("array mode must NOT have inventory.index")
	}
	if _, ok := out[0].Metadata().Get("inventory.total"); ok {
		t.Fatalf("array mode must NOT have inventory.total")
	}
}

// AC5.10: per_item 모드 — 입력 metadata 보존 (얕은 복사)
func TestInventoryNode_PerItem_PreservesInputMetadata(t *testing.T) {
	devs := []device.Device{
		makeDevice("d1", "A", "lgcnp", "ag", true),
		makeDevice("d2", "B", "lgcnp", "ag", true),
	}
	reg := newFakeDeviceRegistry(devs...)
	n := newInventoryNode(t,
		map[string]any{"source": "devices", "emit_shape": "per_item"},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}

	// 입력 메시지에 trigger 컨텍스트 metadata 부착
	inMsg := message.New(
		message.WithMetadata("trigger.schedule_id", "sched-1"),
		message.WithMetadata("trigger.tick_count", "42"),
	)
	out, err := n.Process(context.Background(), inMsg)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(out))
	}
	for i, m := range out {
		if v, _ := m.Metadata().Get("trigger.schedule_id"); v != "sched-1" {
			t.Fatalf("message[%d] trigger.schedule_id lost: got %q", i, v)
		}
		if v, _ := m.Metadata().Get("trigger.tick_count"); v != "42" {
			t.Fatalf("message[%d] trigger.tick_count lost: got %q", i, v)
		}
		// inventory.* 도 함께 존재
		if v, _ := m.Metadata().Get("inventory.source"); v != "devices" {
			t.Fatalf("message[%d] inventory.source missing", i)
		}
	}
}

// AC5.11: 입력의 inventory.* 키는 노드 설정으로 덮어쓰기
func TestInventoryNode_InputInventoryKeys_Overwritten(t *testing.T) {
	reg := newFakeDeviceRegistry(makeDevice("d1", "A", "x", "ag", true))
	n := newInventoryNode(t,
		map[string]any{"source": "devices", "emit_shape": "array"},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	inMsg := message.New(message.WithMetadata("inventory.source", "should_be_overwritten"))
	out, _ := n.Process(context.Background(), inMsg)
	if v, _ := out[0].Metadata().Get("inventory.source"); v != "devices" {
		t.Fatalf("inventory.source must be overwritten by node config, got %s", v)
	}
}

// ---------------------------------------------------------------------------
// Phase 2 - agents source
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

// AC5.5: agents 항목 스키마
func TestInventoryNode_AgentsArray_EmitsAgentList(t *testing.T) {
	mgr := &fakeAgentManager{
		agents: []agent.Agent{
			&fakeAgent{id: "a1", name: "serial-agent", agType: "serial", state: lifecycle.StateRunning},
			&fakeAgent{id: "a2", name: "mqtt-agent", agType: "mqtt", state: lifecycle.StateStopped},
		},
	}
	n := newInventoryNode(t,
		map[string]any{"source": "agents", "emit_shape": "array"},
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
		t.Fatalf("expected 1 array message, got %d", len(out))
	}
	if v, _ := out[0].Payload().Get("count"); v != 2 {
		t.Fatalf("count expected 2, got %v", v)
	}
	itemsRaw, _ := out[0].Payload().Get("items")
	items := itemsRaw.([]map[string]any)
	if items[0]["id"] != "a1" || items[0]["name"] != "serial-agent" {
		t.Fatalf("first item mismatch: %+v", items[0])
	}
	if items[0]["state"] != string(lifecycle.StateRunning) {
		t.Fatalf("state must be lifecycle string, got %v", items[0]["state"])
	}
	if _, hasInfo := items[0]["info"]; !hasInfo {
		t.Fatalf("info key expected with include_metadata=true")
	}
}

// AC4.2: agents source 인데 AgentManager 미주입 시 Init 에러
func TestInventoryNode_AgentsSource_WithoutManagerOption_InitError(t *testing.T) {
	n := newInventoryNode(t,
		map[string]any{"source": "agents"},
	)
	err := n.Init(context.Background())
	if !errors.Is(err, ErrInventoryAgentManagerNotAvailable) {
		t.Fatalf("expected ErrInventoryAgentManagerNotAvailable, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Phase 2 - flows / nodes source
// ---------------------------------------------------------------------------

type fakeFlowRegistry struct {
	flows []FlowSummary
}

func (f *fakeFlowRegistry) FlowSummaries() []FlowSummary { return f.flows }

// AC5.6: flows 항목 스키마
func TestInventoryNode_FlowsArray_EmitsFlowSummary(t *testing.T) {
	freg := &fakeFlowRegistry{
		flows: []FlowSummary{
			{ID: "f1", Name: "my-flow", State: "running", NodeCount: 5, WireCount: 4, Extra: map[string]any{"uptime_seconds": 120}},
		},
	}
	n := newInventoryNode(t,
		map[string]any{"source": "flows", "emit_shape": "array"},
		WithFlowRegistryFunc(func() FlowRegistry { return freg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, _ := n.Process(context.Background(), message.New())
	itemsRaw, _ := out[0].Payload().Get("items")
	items := itemsRaw.([]map[string]any)
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
		t.Fatalf("extra expected when include_metadata=true")
	}
}

// AC4.3: flows source 인데 FlowRegistry 미주입 시 Init 에러
func TestInventoryNode_FlowsSource_WithoutRegistryOption_InitError(t *testing.T) {
	n := newInventoryNode(t,
		map[string]any{"source": "flows"},
	)
	err := n.Init(context.Background())
	if !errors.Is(err, ErrInventoryFlowRegistryNotAvailable) {
		t.Fatalf("expected ErrInventoryFlowRegistryNotAvailable, got %v", err)
	}
}

// AC5.7: nodes 항목 스키마
func TestInventoryNode_NodesArray_EmitsNodeTypeMeta(t *testing.T) {
	// 실제 Registry (빌트인 + inventory) 사용은 Phase 5 등록 후에만 가능.
	// 여기서는 별도 Registry 인스턴스에 inventory 를 추가 등록한 뒤 검증.
	reg := NewRegistry()
	// inventory 가 registerBuiltins 에 추가되지 않은 단계여도 별도 등록 가능
	if !reg.Has("inventory") {
		_ = reg.RegisterWithMeta("inventory", NewInventoryNode, NodeTypeMeta{
			Type:        "inventory",
			Category:    "processing",
			Description: "test",
			Source:      "builtin",
		})
	}
	n := newInventoryNode(t,
		map[string]any{"source": "nodes", "emit_shape": "array"},
		WithNodeRegistryFunc(func() *Registry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, _ := n.Process(context.Background(), message.New())
	itemsRaw, _ := out[0].Payload().Get("items")
	items := itemsRaw.([]map[string]any)
	if len(items) == 0 {
		t.Fatalf("expected at least one node type")
	}
	// 항목은 type/category/description/origin 키를 가져야 함
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

// AC4.4: nodes source 인데 NodeRegistry 미주입 시 Init 에러
func TestInventoryNode_NodesSource_WithoutNodeRegistryOption_InitError(t *testing.T) {
	n := newInventoryNode(t,
		map[string]any{"source": "nodes"},
	)
	err := n.Init(context.Background())
	if !errors.Is(err, ErrInventoryNodeRegistryNotAvailable) {
		t.Fatalf("expected ErrInventoryNodeRegistryNotAvailable, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Phase 3 - Filter 검증 강화
// ---------------------------------------------------------------------------

// AC3.5: filter.tags 가 비-string 슬라이스 → ErrInventoryInvalidFilter
func TestInventoryNode_FilterWithInvalidTagsType_FactoryError(t *testing.T) {
	def := flow.NodeDef{
		ID:   "n1",
		Type: "inventory",
		Config: map[string]any{
			"source": "devices",
			"filter": map[string]any{
				"tags": []any{1, 2, 3},
			},
		},
	}
	_, err := NewInventoryNode(def)
	if !errors.Is(err, ErrInventoryInvalidFilter) {
		t.Fatalf("expected ErrInventoryInvalidFilter, got %v", err)
	}
}

// AC3.6: filter.online 의 타입 검증 (string 이지 bool 아님)
func TestInventoryNode_FilterWithInvalidOnlineType_FactoryError(t *testing.T) {
	def := flow.NodeDef{
		ID:   "n1",
		Type: "inventory",
		Config: map[string]any{
			"source": "devices",
			"filter": map[string]any{
				"online": "yes",
			},
		},
	}
	_, err := NewInventoryNode(def)
	if !errors.Is(err, ErrInventoryInvalidFilter) {
		t.Fatalf("expected ErrInventoryInvalidFilter, got %v", err)
	}
}

// AC3.3: DeviceFilter 의 모든 필드 매핑
func TestInventoryNode_FilterAllFieldsMapping(t *testing.T) {
	reg := newFakeDeviceRegistry()
	n := newInventoryNode(t,
		map[string]any{
			"source": "devices",
			"filter": map[string]any{
				"protocol":   "modbus",
				"agent_name": "ag1",
				"type":       "HVACR.IDU",
				"online":     false,
				"group":      "prod",
				"tags":       []any{"critical", "v2"},
			},
		},
		WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }),
	)
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	_, _ = n.Process(context.Background(), message.New())

	f := reg.lastFilter
	if f.Protocol != "modbus" {
		t.Fatalf("Protocol: %s", f.Protocol)
	}
	if f.AgentName != "ag1" {
		t.Fatalf("AgentName: %s", f.AgentName)
	}
	if f.Type != "HVACR.IDU" {
		t.Fatalf("Type: %s", f.Type)
	}
	if f.Online == nil || *f.Online != false {
		t.Fatalf("Online: %v", f.Online)
	}
	if f.Group != "prod" {
		t.Fatalf("Group: %s", f.Group)
	}
	if len(f.Tags) != 2 || f.Tags[0] != "critical" || f.Tags[1] != "v2" {
		t.Fatalf("Tags: %v", f.Tags)
	}
}

// AC3.4: 비-device source 에서 filter 무시 + 경고 (팩토리는 성공)
func TestInventoryNode_FilterOnNonDeviceSource_LoggedAndIgnored(t *testing.T) {
	mgr := &fakeAgentManager{
		agents: []agent.Agent{
			&fakeAgent{id: "a1", name: "agent-1", agType: "serial", state: lifecycle.StateRunning},
		},
	}
	def := flow.NodeDef{
		ID:   "n1",
		Type: "inventory",
		Config: map[string]any{
			"source": "agents",
			"filter": map[string]any{"protocol": "lgcnp"},
		},
	}
	n, err := NewInventoryNode(def, WithAgentManagerFunc(func() agent.Manager { return mgr }))
	if err != nil {
		t.Fatalf("factory must succeed even with filter on non-device source, got %v", err)
	}
	if err := n.(*InventoryNode).Configure(def.Config); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if err := n.(*InventoryNode).Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := n.(*InventoryNode).Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("agents array mode should emit 1 message regardless of filter, got %d", len(out))
	}
	if v, _ := out[0].Payload().Get("count"); v != 1 {
		t.Fatalf("filter must be ignored, expected count=1, got %v", v)
	}
}

// Shutdown 은 idempotent 하며 stateless read-only 노드이므로 추가 정리가 없다.
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
	// 두 번째 호출은 no-op (already Stopped)
	if err := n.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown 2nd: %v", err)
	}
}

// AC4.6: 모든 옵션 주입 시 4종 source 모두 정상
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
				t.Fatalf("array mode expected 1 message, got %d", len(out))
			}
			if v, _ := out[0].Payload().Get("source"); v != src {
				t.Fatalf("payload.source mismatch: %v", v)
			}
		})
	}
}
