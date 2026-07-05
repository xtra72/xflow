// remote_inventory_test.go 는 인벤토리 소스 어댑터(redaction + 페이지네이션 + 중립
// 변환)를 검증한다(@SPEC:SPEC-REMOTE-001 M4, REQ-E01/E04, F06).
package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/remote"
)

// fakeFlowLister 는 페이지네이션을 모사하는 flow 목록 소스이다.
//
// ListFlows 는 목록 엔드포인트(요약)를 모사하며 빈 Config 를 반환할 수 있다
// (flowStatusToInfo 의 하드코딩된 빈 정의를 재현). GetFlow 는 단건 엔드포인트를
// 모사하며 전체 React Flow 정의(nodes/edges)를 반환한다.
type fakeFlowLister struct {
	flows []handler.FlowInfo
	// fullByID 는 GetFlow 가 반환할 전체 정의(Config 포함)이다. 비어 있으면
	// ListFlows 가 반환한 항목을 그대로 반환한다.
	fullByID map[string]handler.FlowInfo
	// getErrByID 에 등록된 ID 는 GetFlow 호출 시 에러를 반환한다(부분 실패 모사).
	getErrByID map[string]error
}

func (f *fakeFlowLister) ListFlows(_ context.Context, opts dto.ListOptions) ([]handler.FlowInfo, int64, error) {
	total := int64(len(f.flows))
	start := opts.Offset()
	if start >= len(f.flows) {
		return []handler.FlowInfo{}, total, nil
	}
	end := start + opts.Size
	if end > len(f.flows) {
		end = len(f.flows)
	}
	return f.flows[start:end], total, nil
}

func (f *fakeFlowLister) GetFlow(_ context.Context, id string) (*handler.FlowInfo, error) {
	if f.getErrByID != nil {
		if err, ok := f.getErrByID[id]; ok {
			return nil, err
		}
	}
	if f.fullByID != nil {
		if info, ok := f.fullByID[id]; ok {
			return &info, nil
		}
	}
	for i := range f.flows {
		if f.flows[i].ID == id {
			info := f.flows[i]
			return &info, nil
		}
	}
	return nil, errFlowNotFoundStub
}

// errFlowNotFoundStub 는 테스트 스텁 전용 not-found 에러이다.
var errFlowNotFoundStub = stubError("flow not found")

type stubError string

func (e stubError) Error() string { return string(e) }

type fakeAgentLister struct {
	agents []handler.AgentInfo
}

func (f *fakeAgentLister) ListAgents(_ context.Context, opts dto.ListOptions) ([]handler.AgentInfo, int64, error) {
	total := int64(len(f.agents))
	start := opts.Offset()
	if start >= len(f.agents) {
		return []handler.AgentInfo{}, total, nil
	}
	end := start + opts.Size
	if end > len(f.agents) {
		end = len(f.agents)
	}
	return f.agents[start:end], total, nil
}

type fakeDeviceLister struct {
	devices []device.Device
}

func (f *fakeDeviceLister) List(_ device.DeviceFilter) []device.Device { return f.devices }

// TestInventorySource_FlowRedaction 는 flow 정의의 시크릿이 제거됨을 검증한다(F06).
func TestInventorySource_FlowRedaction(t *testing.T) {
	src := newRemoteInventorySource(
		&fakeFlowLister{flows: []handler.FlowInfo{
			{ID: "f1", Name: "flowA", Status: "running", UpdatedAt: "2026-06-05T10:00:00Z",
				Config: map[string]any{"host": "h", "password": "supersecret", "token": "abc"}},
		}},
		&fakeAgentLister{},
		&fakeDeviceLister{},
	)

	items, err := src.ListFlows(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 1)
	def := string(items[0].Definition)
	assert.NotContains(t, def, "supersecret", "password 값이 제거되어야 함(F06)")
	assert.NotContains(t, def, "password")
	assert.NotContains(t, def, "token")
	assert.Contains(t, def, "host", "비시크릿 키는 보존")
	assert.Equal(t, remote.KindFlow, items[0].Kind)
	assert.Greater(t, items[0].UpdatedAt, int64(0), "RFC3339 → epoch ms 변환")
}

// TestInventorySource_FlowFullDefinition 는 플로우 미러가 목록 요약(빈 Config)이 아니라
// 단건 조회(GetFlow)의 전체 React Flow 정의(nodes/edges)를 담는지 검증한다.
// 회귀 방지: flowStatusToInfo 가 하드코딩하는 빈 정의가 미러로 새어 나가면 원격 에디터에
// 빈 캔버스가 표시된다.
func TestInventorySource_FlowFullDefinition(t *testing.T) {
	src := newRemoteInventorySource(
		&fakeFlowLister{
			// 목록 엔드포인트는 빈 Config 를 반환(flowStatusToInfo 재현).
			flows: []handler.FlowInfo{
				{ID: "f1", Name: "flowA", Status: "running",
					Config: map[string]any{
						"nodes": []map[string]any{},
						"edges": []map[string]any{},
					}},
			},
			// 단건 엔드포인트는 전체 정의를 반환.
			fullByID: map[string]handler.FlowInfo{
				"f1": {ID: "f1", Name: "flowA", Status: "running",
					Config: map[string]any{
						"nodes": []map[string]any{
							{"id": "n1", "type": "custom"},
						},
						"edges": []map[string]any{
							{"id": "e1", "source": "n1", "target": "n1"},
						},
					}},
			},
		},
		&fakeAgentLister{},
		&fakeDeviceLister{},
	)

	items, err := src.ListFlows(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 1)
	def := string(items[0].Definition)
	assert.Contains(t, def, "\"n1\"", "전체 정의의 노드가 포함되어야 함")
	assert.Contains(t, def, "\"e1\"", "전체 정의의 엣지가 포함되어야 함")
}

// TestInventorySource_FlowFullDefinitionRedaction 는 단건 전체 정의에도 redaction 이
// 적용됨을 검증한다(F06). 시크릿은 단건 Config 의 노드 설정에 있을 수 있다.
func TestInventorySource_FlowFullDefinitionRedaction(t *testing.T) {
	src := newRemoteInventorySource(
		&fakeFlowLister{
			flows: []handler.FlowInfo{
				{ID: "f1", Name: "flowA", Status: "running",
					Config: map[string]any{"nodes": []map[string]any{}, "edges": []map[string]any{}}},
			},
			fullByID: map[string]handler.FlowInfo{
				"f1": {ID: "f1", Name: "flowA", Status: "running",
					Config: map[string]any{
						"host":     "h",
						"password": "supersecret",
						"token":    "abc",
						"nodes":    []map[string]any{{"id": "n1"}},
					}},
			},
		},
		&fakeAgentLister{},
		&fakeDeviceLister{},
	)

	items, err := src.ListFlows(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 1)
	def := string(items[0].Definition)
	assert.NotContains(t, def, "supersecret", "전체 정의에도 password 값 제거(F06)")
	assert.NotContains(t, def, "password")
	assert.NotContains(t, def, "token")
	assert.Contains(t, def, "host", "비시크릿 키는 보존")
	assert.Contains(t, def, "\"n1\"", "전체 정의의 노드 보존")
}

// TestInventorySource_FlowGetFlowErrorGraceful 는 단건 조회가 실패해도 전체 스냅샷이
// 중단되지 않고, 해당 플로우는 목록 요약 메타데이터로라도 포함됨을 검증한다.
func TestInventorySource_FlowGetFlowErrorGraceful(t *testing.T) {
	src := newRemoteInventorySource(
		&fakeFlowLister{
			flows: []handler.FlowInfo{
				{ID: "f1", Name: "flowA", Status: "running",
					Config: map[string]any{"nodes": []map[string]any{}, "edges": []map[string]any{}}},
				{ID: "f2", Name: "flowB", Status: "stored",
					Config: map[string]any{"nodes": []map[string]any{}, "edges": []map[string]any{}}},
			},
			fullByID: map[string]handler.FlowInfo{
				"f2": {ID: "f2", Name: "flowB", Status: "stored",
					Config: map[string]any{"nodes": []map[string]any{{"id": "n2"}}}},
			},
			// f1 단건 조회는 실패 → 목록 요약으로 폴백, 스냅샷은 계속 진행.
			getErrByID: map[string]error{"f1": errFlowNotFoundStub},
		},
		&fakeAgentLister{},
		&fakeDeviceLister{},
	)

	items, err := src.ListFlows(context.Background())
	require.NoError(t, err, "단건 실패가 전체 스냅샷을 중단시키면 안 됨")
	require.Len(t, items, 2, "실패한 플로우도 메타데이터로 포함되어야 함")

	byID := map[string]remote.InventoryItem{}
	for _, it := range items {
		byID[it.ID] = it
	}
	require.Contains(t, byID, "f1")
	require.Contains(t, byID, "f2")
	assert.Equal(t, "flowA", byID["f1"].Name, "실패한 플로우도 목록 메타데이터 유지")
	assert.Contains(t, string(byID["f2"].Definition), "\"n2\"", "정상 플로우는 전체 정의 포함")
}

// TestInventorySource_AgentRedaction 는 agent config 의 시크릿이 제거됨을 검증한다(F06).
func TestInventorySource_AgentRedaction(t *testing.T) {
	src := newRemoteInventorySource(
		&fakeFlowLister{},
		&fakeAgentLister{agents: []handler.AgentInfo{
			{ID: "a1", Name: "agentA", Type: "mqtt", Status: "running",
				Config: map[string]any{"broker": "tcp://x", "password": "p", "api_key": "k"}},
		}},
		&fakeDeviceLister{},
	)

	items, err := src.ListAgents(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 1)
	def := string(items[0].Definition)
	assert.NotContains(t, def, "password")
	assert.NotContains(t, def, "api_key")
	assert.Contains(t, def, "broker")
	assert.Equal(t, remote.KindAgent, items[0].Kind)
}

// TestInventorySource_Pagination 는 페이지 크기를 초과하는 인벤토리를 전부 수집하는지
// 검증한다(대규모 인벤토리 대응).
func TestInventorySource_Pagination(t *testing.T) {
	var flows []handler.FlowInfo
	for i := 0; i < 250; i++ { // inventoryPageSize(100) 초과.
		flows = append(flows, handler.FlowInfo{ID: strings.Repeat("x", 0) + itoa(i), Name: "f" + itoa(i)})
	}
	src := newRemoteInventorySource(&fakeFlowLister{flows: flows}, &fakeAgentLister{}, &fakeDeviceLister{})

	items, err := src.ListFlows(context.Background())
	require.NoError(t, err)
	assert.Len(t, items, 250, "페이지네이션으로 전체를 수집해야 함")
}

// TestInventorySource_Devices 는 디바이스가 중립 항목으로 변환되는지 검증한다(REQ-E04).
func TestInventorySource_Devices(t *testing.T) {
	src := newRemoteInventorySource(
		&fakeFlowLister{},
		&fakeAgentLister{},
		&fakeDeviceLister{devices: []device.Device{newStubDevice("d1", "Living Room", true)}},
	)
	items, err := src.ListDevices(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "d1", items[0].ID)
	assert.Equal(t, "Living Room", items[0].Name)
	assert.Equal(t, remote.KindDevice, items[0].Kind)
	assert.Equal(t, "online", items[0].Status)
}

// TestParseRFC3339Millis 는 시각 변환 헬퍼를 검증한다.
func TestParseRFC3339Millis(t *testing.T) {
	assert.Equal(t, int64(0), parseRFC3339Millis(""))
	assert.Equal(t, int64(0), parseRFC3339Millis("not-a-time"))
	got := parseRFC3339Millis("2026-06-05T00:00:00Z")
	assert.Greater(t, got, int64(0))
}

// --- 테스트용 stub device ---

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

type stubDevice struct {
	id     string
	name   string
	online bool
}

func newStubDevice(id, name string, online bool) device.Device {
	return &stubDevice{id: id, name: name, online: online}
}

func (d *stubDevice) ID() string                { return d.id }
func (d *stubDevice) UID() string               { return d.id }
func (d *stubDevice) Name() string              { return d.name }
func (d *stubDevice) Type() device.DeviceType   { return device.DeviceTypeSensor }
func (d *stubDevice) Protocol() string          { return "nasa" }
func (d *stubDevice) AgentName() string         { return "agent1" }
func (d *stubDevice) Online() bool              { return d.online }
func (d *stubDevice) LastSeen() time.Time       { return time.Unix(1700000000, 0) }
func (d *stubDevice) State() device.DeviceState { return device.DeviceState{Online: d.online} }
func (d *stubDevice) Metadata() device.DeviceMetadata {
	return device.DeviceMetadata{Name: d.name}
}
func (d *stubDevice) Source() string         { return "auto" }
func (d *stubDevice) Capabilities() []string { return nil }
