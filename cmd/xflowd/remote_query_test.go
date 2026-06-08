// remote_query_test.go 는 M8(그룹 J) 노드 측 query 브리지(로컬 read 매핑 + redaction)
// 와 스트림 브리지를 검증한다(@SPEC:SPEC-REMOTE-001 M8, REQ-J04/J06/J07).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/remote"
)

// --- query read 어댑터 fake ---

type fakeQueryFlowReader struct {
	status    *handler.FlowStatusInfo
	nodes     []handler.FlowNodeInfo
	node      *handler.FlowNodeInfo
	getInfo   *handler.FlowInfo
	statusErr error

	// list 는 ListFlows 가 반환할 페이지(들)이다. listErr 가 설정되면 오류를 반환한다.
	list    []handler.FlowInfo
	listErr error
}

func (f *fakeQueryFlowReader) FlowStatus(_ context.Context, _ string) (*handler.FlowStatusInfo, error) {
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	return f.status, nil
}
func (f *fakeQueryFlowReader) ListFlowNodes(_ context.Context, _ string) ([]handler.FlowNodeInfo, error) {
	return f.nodes, nil
}
func (f *fakeQueryFlowReader) GetFlowNode(_ context.Context, _, _ string) (*handler.FlowNodeInfo, error) {
	return f.node, nil
}
func (f *fakeQueryFlowReader) GetFlow(_ context.Context, _ string) (*handler.FlowInfo, error) {
	return f.getInfo, nil
}

// ListFlows 는 단일 페이지에 전체 목록을 반환한다(total == len → 1 페이지로 종료).
func (f *fakeQueryFlowReader) ListFlows(_ context.Context, opts dto.ListOptions) ([]handler.FlowInfo, int64, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	if opts.Page > 1 {
		return []handler.FlowInfo{}, int64(len(f.list)), nil
	}
	return f.list, int64(len(f.list)), nil
}

type fakeQueryAgentReader struct {
	stats    *handler.AgentStatsInfo
	getInfo  *handler.AgentInfo
	execData json.RawMessage // ExecAgent 가 반환할 raw JSON (sessions 등).
	execErr  error

	// list 는 ListAgents 가 반환할 페이지(들)이다. listErr 가 설정되면 오류를 반환한다.
	list    []handler.AgentInfo
	listErr error
}

func (f *fakeQueryAgentReader) AgentStats(_ context.Context, _ string) (*handler.AgentStatsInfo, error) {
	return f.stats, nil
}
func (f *fakeQueryAgentReader) GetAgent(_ context.Context, _ string, _ string) (*handler.AgentInfo, error) {
	return f.getInfo, nil
}
func (f *fakeQueryAgentReader) ExecAgent(_ context.Context, _ string, _ []byte) (json.RawMessage, error) {
	if f.execErr != nil {
		return nil, f.execErr
	}
	return f.execData, nil
}

// ListAgents 는 단일 페이지에 전체 목록을 반환한다(total == len → 1 페이지로 종료).
func (f *fakeQueryAgentReader) ListAgents(_ context.Context, opts dto.ListOptions) ([]handler.AgentInfo, int64, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	if opts.Page > 1 {
		return []handler.AgentInfo{}, int64(len(f.list)), nil
	}
	return f.list, int64(len(f.list)), nil
}

type fakeQueryDeviceReader struct {
	dev  device.Device
	list []device.Device // List 가 반환할 디바이스 목록(agent/devices 용).
	err  error
}

func (f *fakeQueryDeviceReader) Get(id string) (device.Device, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.dev, nil
}
func (f *fakeQueryDeviceReader) List(_ device.DeviceFilter) []device.Device {
	return f.list
}

// fakeQueryStoreReader 는 store keys snapshot 을 반환하는 store reader fake 이다.
type fakeQueryStoreReader struct {
	resp *handler.StoreKeysListResponse
	err  error
}

func (f *fakeQueryStoreReader) StoreKeys(_ context.Context, _ string) (*handler.StoreKeysListResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

// fakeQuerySeriesReader 는 series 키 목록을 반환하는 series reader fake 이다.
type fakeQuerySeriesReader struct {
	keys []string
	err  error
}

func (f *fakeQuerySeriesReader) SeriesList(_ context.Context, _ string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.keys, nil
}

// TestQueryBridge_FlowStatus 는 flow.status query-action 이 FlowStatus read 핸들러로
// 매핑되어 JSON 을 반환하는지 검증한다(REQ-J04 매핑).
func TestQueryBridge_FlowStatus(t *testing.T) {
	flows := &fakeQueryFlowReader{status: &handler.FlowStatusInfo{ID: "f1", Status: "running"}}
	bridge := newTestQuerySource(flows, &fakeQueryAgentReader{}, &fakeQueryDeviceReader{})

	data, err := bridge.Query(context.Background(), remote.DomainFlow, remote.QueryActionStatus, json.RawMessage(`{"id":"f1"}`))
	require.NoError(t, err)
	assert.Contains(t, string(data), "running")
}

// TestQueryBridge_AgentStats 는 agent.stats query-action 이 AgentStats 로 매핑되는지
// 검증한다.
func TestQueryBridge_AgentStats(t *testing.T) {
	agents := &fakeQueryAgentReader{stats: &handler.AgentStatsInfo{ID: "a1"}}
	bridge := newTestQuerySource(&fakeQueryFlowReader{}, agents, &fakeQueryDeviceReader{})

	data, err := bridge.Query(context.Background(), remote.DomainAgent, remote.QueryActionStats, json.RawMessage(`{"id":"a1"}`))
	require.NoError(t, err)
	assert.Contains(t, string(data), "a1")
}

// TestQueryBridge_DeviceState 는 device.state query-action 이 device State() 로
// 매핑되는지 검증한다.
func TestQueryBridge_DeviceState(t *testing.T) {
	dev := &queryMockDevice{id: "d1", state: device.DeviceState{Online: true}}
	devices := &fakeQueryDeviceReader{dev: dev}
	bridge := newTestQuerySource(&fakeQueryFlowReader{}, &fakeQueryAgentReader{}, devices)

	data, err := bridge.Query(context.Background(), remote.DomainDevice, remote.QueryActionState, json.RawMessage(`{"id":"d1"}`))
	require.NoError(t, err)
	assert.Contains(t, string(data), "online")
}

// TestQueryBridge_UnsupportedAction 는 로컬 backing 이 없는 query-action 이 패닉 없이
// ErrQueryActionUnsupported 를 반환하는지 검증한다(REQ-J07 — node-error).
//
// flow/logs 는 데몬에 플로우/노드 로그 read 엔드포인트가 없으므로(monitor.go 는 로그
// *레벨* 관리만 제공) 영구 미지원이다.
func TestQueryBridge_UnsupportedAction(t *testing.T) {
	bridge := newTestQuerySource(&fakeQueryFlowReader{}, &fakeQueryAgentReader{}, &fakeQueryDeviceReader{})

	// flow.logs 는 본 노드에서 데이터 미가용 → 미지원 오류.
	_, err := bridge.Query(context.Background(), remote.DomainFlow, remote.QueryActionLogs, json.RawMessage(`{"id":"f1"}`))
	require.Error(t, err)
	assert.ErrorIs(t, err, remote.ErrQueryActionUnsupported)
}

// TestQueryBridge_DeviceNotFound 는 device 미존재 시 오류를 반환하는지 검증한다.
func TestQueryBridge_DeviceNotFound(t *testing.T) {
	devices := &fakeQueryDeviceReader{err: errors.New("device not found")}
	bridge := newTestQuerySource(&fakeQueryFlowReader{}, &fakeQueryAgentReader{}, devices)

	_, err := bridge.Query(context.Background(), remote.DomainDevice, remote.QueryActionState, json.RawMessage(`{"id":"dX"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- M8 신규 agent 상세 패널 query-action 매핑(REQ-J04 FULL 커버리지) ---

// TestQueryBridge_AgentTopics 는 agent.topics 가 GetAgent(full) 의 state 를 반환하는지
// 검증한다(로컬 TopicsTab 이 agent.state.{subscribed_topics,…} 를 읽음).
func TestQueryBridge_AgentTopics(t *testing.T) {
	agents := &fakeQueryAgentReader{getInfo: &handler.AgentInfo{
		ID:   "a1",
		Name: "mqtt1",
		State: map[string]any{
			"subscribed_topics": []any{map[string]any{"topic": "sensors/#"}},
			"pub_topics":        []any{},
		},
	}}
	bridge := newTestQuerySource(&fakeQueryFlowReader{}, agents, &fakeQueryDeviceReader{})

	data, err := bridge.Query(context.Background(), remote.DomainAgent, remote.QueryActionTopics, json.RawMessage(`{"id":"a1"}`))
	require.NoError(t, err)
	assert.Contains(t, string(data), "subscribed_topics")
	assert.Contains(t, string(data), "sensors/#")
}

// TestQueryBridge_AgentDevices 는 agent.devices 가 디바이스 레지스트리 List 결과를
// {data:[…]} 로 반환하는지 검증한다(로컬 useDevices 와 동일 형상).
func TestQueryBridge_AgentDevices(t *testing.T) {
	agents := &fakeQueryAgentReader{getInfo: &handler.AgentInfo{ID: "a1", Name: "modbus1"}}
	dev := &queryMockDevice{id: "d1", state: device.DeviceState{Online: true}}
	devices := &fakeQueryDeviceReader{list: []device.Device{dev}}
	bridge := newTestQuerySource(&fakeQueryFlowReader{}, agents, devices)

	data, err := bridge.Query(context.Background(), remote.DomainAgent, remote.QueryActionDevices, json.RawMessage(`{"id":"a1"}`))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"data"`)
	assert.Contains(t, string(data), "dev-d1")
}

// TestQueryBridge_AgentSessions 는 agent.sessions 가 ExecAgent(list_connections) 결과를
// 그대로 반환하는지 검증한다(로컬 SessionsTab 이 connections 를 언랩).
func TestQueryBridge_AgentSessions(t *testing.T) {
	agents := &fakeQueryAgentReader{
		execData: json.RawMessage(`{"connections":[{"remote_addr":"10.0.0.1:5000"}]}`),
	}
	bridge := newTestQuerySource(&fakeQueryFlowReader{}, agents, &fakeQueryDeviceReader{})

	data, err := bridge.Query(context.Background(), remote.DomainAgent, remote.QueryActionSessions, json.RawMessage(`{"id":"a1"}`))
	require.NoError(t, err)
	assert.Contains(t, string(data), "connections")
	assert.Contains(t, string(data), "10.0.0.1:5000")
}

// TestQueryBridge_AgentStore 는 agent.store 가 store keys snapshot 을 {count, keys:[…]}
// 로 반환하는지 검증한다(GET /store/{name}/keys 와 동일 형상).
func TestQueryBridge_AgentStore(t *testing.T) {
	agents := &fakeQueryAgentReader{getInfo: &handler.AgentInfo{ID: "a1", Name: "store1"}}
	store := &fakeQueryStoreReader{resp: &handler.StoreKeysListResponse{
		Count: 1,
		Keys: []handler.StoreKeyResponse{
			{Key: "temp", Registration: "manual", DataType: "float", MetricType: "temperature", Tags: map[string]string{}},
		},
	}}
	bridge := newRemoteQuerySource(&fakeQueryFlowReader{}, agents, &fakeQueryDeviceReader{}, store, &fakeQuerySeriesReader{})

	data, err := bridge.Query(context.Background(), remote.DomainAgent, remote.QueryActionStore, json.RawMessage(`{"id":"a1"}`))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"count":1`)
	assert.Contains(t, string(data), "temp")
}

// TestQueryBridge_AgentSeries 는 agent.series 가 series 키 목록을 {series:[…], count}
// 로 반환하는지 검증한다(GET /tsdb/series 와 동일 형상).
func TestQueryBridge_AgentSeries(t *testing.T) {
	agents := &fakeQueryAgentReader{getInfo: &handler.AgentInfo{ID: "a1", Name: "tsdb1"}}
	series := &fakeQuerySeriesReader{keys: []string{"cpu", "mem"}}
	bridge := newRemoteQuerySource(&fakeQueryFlowReader{}, agents, &fakeQueryDeviceReader{}, &fakeQueryStoreReader{}, series)

	data, err := bridge.Query(context.Background(), remote.DomainAgent, remote.QueryActionSeries, json.RawMessage(`{"id":"a1"}`))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"series"`)
	assert.Contains(t, string(data), "cpu")
	assert.Contains(t, string(data), `"count":2`)
}

// --- M8 보강: agent/flow/device list query-action (라이브 목록 — REQ-J04) ---

// TestQueryBridge_AgentList 는 agent.list query-action 이 ListAgents 의 전체 목록을
// runtime 필드(connected/uptime/stats)와 함께 {data:[…]} 로 반환하는지 검증한다.
// 미러 요약과 달리 노드의 라이브 로컬 목록을 그대로 반환한다(REQ-J04 보강).
func TestQueryBridge_AgentList(t *testing.T) {
	connected := true
	agents := &fakeQueryAgentReader{list: []handler.AgentInfo{
		{
			ID: "a1", Name: "mqtt1", Type: "mqtt", Status: "running", Enabled: true,
			Connected: &connected,
			Uptime:    "1m30s",
			Stats:     &handler.AgentStatsResponse{MessagesIn: 10, MessagesOut: 5},
		},
	}}
	bridge := newTestQuerySource(&fakeQueryFlowReader{}, agents, &fakeQueryDeviceReader{})

	data, err := bridge.Query(context.Background(), remote.DomainAgent, remote.QueryActionList, nil)
	require.NoError(t, err)
	s := string(data)
	assert.Contains(t, s, `"data"`)
	assert.Contains(t, s, "mqtt1")
	assert.Contains(t, s, `"connected":true`, "라이브 connected 필드 포함")
	assert.Contains(t, s, "1m30s", "라이브 uptime 필드 포함")
	assert.Contains(t, s, `"messages_in":10`, "라이브 stats 필드 포함")
}

// TestQueryBridge_AgentListRedaction 는 agent.list 결과의 시크릿 config 가 리댁터로
// 마스킹되는지 검증한다(REQ-J06 — config 가 시크릿을 운반할 수 있음).
func TestQueryBridge_AgentListRedaction(t *testing.T) {
	agents := &fakeQueryAgentReader{list: []handler.AgentInfo{
		{ID: "a1", Name: "mqtt1", Config: map[string]any{"host": "h", "password": "hunter2"}},
	}}
	bridge := newTestQuerySource(&fakeQueryFlowReader{}, agents, &fakeQueryDeviceReader{})

	data, err := bridge.Query(context.Background(), remote.DomainAgent, remote.QueryActionList, nil)
	require.NoError(t, err)
	// 브리지는 raw 를 반환하고 리댁터가 전송 전 마스킹한다(REQ-J06).
	out := newQueryRedactor().Redact(data)
	assert.NotContains(t, string(out), "hunter2")
	assert.NotContains(t, string(out), "password")
	assert.Contains(t, string(out), "mqtt1")
}

// TestQueryBridge_FlowList 는 flow.list query-action 이 ListFlows 목록을 {data:[…]} 로
// 반환하는지 검증한다(status/node_count/uptime 요약 필드 포함 — REQ-J04 보강).
func TestQueryBridge_FlowList(t *testing.T) {
	flows := &fakeQueryFlowReader{list: []handler.FlowInfo{
		{ID: "f1", Name: "flow-one", Status: "running", NodeCount: 3, Uptime: "2m"},
	}}
	bridge := newTestQuerySource(flows, &fakeQueryAgentReader{}, &fakeQueryDeviceReader{})

	data, err := bridge.Query(context.Background(), remote.DomainFlow, remote.QueryActionList, nil)
	require.NoError(t, err)
	s := string(data)
	assert.Contains(t, s, `"data"`)
	assert.Contains(t, s, "flow-one")
	assert.Contains(t, s, `"status":"running"`)
	assert.Contains(t, s, `"node_count":3`)
	assert.Contains(t, s, "2m")
}

// TestQueryBridge_DeviceList 는 device.list query-action 이 레지스트리 List 전체를
// {data:[…]} 로 반환하는지 검증한다(REQ-J04 보강).
func TestQueryBridge_DeviceList(t *testing.T) {
	dev := &queryMockDevice{id: "d1", state: device.DeviceState{Online: true}}
	devices := &fakeQueryDeviceReader{list: []device.Device{dev}}
	bridge := newTestQuerySource(&fakeQueryFlowReader{}, &fakeQueryAgentReader{}, devices)

	data, err := bridge.Query(context.Background(), remote.DomainDevice, remote.QueryActionList, nil)
	require.NoError(t, err)
	s := string(data)
	assert.Contains(t, s, `"data"`)
	assert.Contains(t, s, "dev-d1")
	assert.Contains(t, s, `"online":true`)
}

// TestQueryBridge_ListError 는 목록 소스 오류가 패닉 없이 전파되는지 검증한다(REQ-J07).
func TestQueryBridge_ListError(t *testing.T) {
	agents := &fakeQueryAgentReader{listErr: errors.New("list boom")}
	bridge := newTestQuerySource(&fakeQueryFlowReader{}, agents, &fakeQueryDeviceReader{})

	_, err := bridge.Query(context.Background(), remote.DomainAgent, remote.QueryActionList, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

// TestQueryBridge_AgentStoreRedaction 는 store 응답에 섞인 시크릿이 리댁터로 마스킹
// 되는지 검증한다(REQ-J06 — 전송 전 redaction).
func TestQueryBridge_AgentStoreRedaction(t *testing.T) {
	redactor := newQueryRedactor()
	// store 키 값에 시크릿 필드가 섞인 경우(예: 자격증명 노출)도 마스킹되어야 한다.
	in := json.RawMessage(`{"count":1,"keys":[{"key":"k","password":"hunter2"}]}`)
	out := redactor.Redact(in)
	assert.NotContains(t, string(out), "hunter2")
	assert.NotContains(t, string(out), "password")
	assert.Contains(t, string(out), `"key":"k"`)
}

// TestQueryRedactor_MasksSecrets 는 JSON 리댁터가 시크릿 필드를 제거하는지 검증한다
// (REQ-J06 — secret_fields SoT 재사용).
func TestQueryRedactor_MasksSecrets(t *testing.T) {
	redactor := newQueryRedactor()
	in := json.RawMessage(`{"name":"a1","config":{"password":"hunter2","host":"h"}}`)
	out := redactor.Redact(in)

	assert.NotContains(t, string(out), "hunter2", "시크릿 값은 제거되어야 함")
	assert.NotContains(t, string(out), "password", "시크릿 키는 제거되어야 함")
	assert.Contains(t, string(out), "h", "비시크릿 값은 보존")
}

// TestQueryRedactor_NonObjectPassThrough 는 비-객체 JSON(배열/스칼라)이 그대로
// 통과하는지 검증한다(graceful).
func TestQueryRedactor_NonObjectPassThrough(t *testing.T) {
	redactor := newQueryRedactor()
	arr := json.RawMessage(`[{"password":"x"},{"a":1}]`)
	out := redactor.Redact(arr)
	// 배열 내부 객체도 재귀 redaction 되어야 한다.
	assert.NotContains(t, string(out), "password")
	assert.Contains(t, string(out), "\"a\":1")

	scalar := json.RawMessage(`42`)
	assert.JSONEq(t, "42", string(redactor.Redact(scalar)))
}

// --- 스트림 브리지 ---

// TestStreamBridge_DeviceStatePolls 는 device.state 스트림이 폴링으로 갱신을
// 흘리는지 검증한다(REQ-J08).
func TestStreamBridge_DeviceStatePolls(t *testing.T) {
	dev := &queryMockDevice{id: "d1", state: device.DeviceState{Online: true}}
	devices := &fakeQueryDeviceReader{dev: dev}
	bridge := newTestStreamSource(&fakeQueryAgentReader{}, devices, &fakeQuerySeriesReader{}, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub, err := bridge.Subscribe(ctx, remote.DomainDevice, remote.StreamActionState, json.RawMessage(`{"id":"d1"}`))
	require.NoError(t, err)
	defer sub.Close()

	select {
	case v := <-sub.Updates():
		assert.Contains(t, string(v), "online")
	case <-time.After(time.Second):
		t.Fatal("device.state 스트림 갱신 타임아웃")
	}
}

// TestStreamBridge_AgentSeriesPolls 는 agent.series 스트림이 series 키 목록을 폴링으로
// 흘리는지 검증한다(REQ-J08 — TSDB/series 라이브 소스).
func TestStreamBridge_AgentSeriesPolls(t *testing.T) {
	agents := &fakeQueryAgentReader{getInfo: &handler.AgentInfo{ID: "a1", Name: "tsdb1"}}
	series := &fakeQuerySeriesReader{keys: []string{"cpu", "mem"}}
	bridge := newRemoteStreamSource(agents, &fakeQueryDeviceReader{}, series, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub, err := bridge.Subscribe(ctx, remote.DomainAgent, remote.StreamActionSeries, json.RawMessage(`{"id":"a1"}`))
	require.NoError(t, err)
	defer sub.Close()

	select {
	case v := <-sub.Updates():
		assert.Contains(t, string(v), "cpu")
		assert.Contains(t, string(v), `"series"`)
	case <-time.After(time.Second):
		t.Fatal("agent.series 스트림 갱신 타임아웃")
	}
}

// TestStreamBridge_UnsupportedAction 는 비스트림/미가용 action 이 미지원 오류를
// 반환하는지 검증한다. device 도메인의 stats 는 스트림 소스가 없어 미지원이다.
func TestStreamBridge_UnsupportedAction(t *testing.T) {
	bridge := newTestStreamSource(&fakeQueryAgentReader{}, &fakeQueryDeviceReader{}, &fakeQuerySeriesReader{}, 10*time.Millisecond)
	_, err := bridge.Subscribe(context.Background(), remote.DomainDevice, remote.StreamActionStats, json.RawMessage(`{"id":"d1"}`))
	require.Error(t, err)
	assert.ErrorIs(t, err, remote.ErrQueryActionUnsupported)
}

// newTestQuerySource 는 store/series reader 를 빈 fake 로 채운 query 소스 헬퍼이다
// (store/series 를 사용하지 않는 기존 테스트 편의).
func newTestQuerySource(flows queryFlowReader, agents queryAgentReader, devices queryDeviceReader) *remoteQuerySource {
	return newRemoteQuerySource(flows, agents, devices, &fakeQueryStoreReader{}, &fakeQuerySeriesReader{})
}

// newTestStreamSource 는 series reader 를 명시 주입하는 stream 소스 헬퍼이다.
func newTestStreamSource(agents queryAgentReader, devices queryDeviceReader, series querySeriesReader, interval time.Duration) *remoteStreamSource {
	return newRemoteStreamSource(agents, devices, series, interval)
}

// queryMockDevice 는 device.Device 의 최소 테스트 구현이다(state/commands/metadata).
type queryMockDevice struct {
	id    string
	state device.DeviceState
	cmds  []device.CommandSpec
}

func (d *queryMockDevice) ID() string                { return d.id }
func (d *queryMockDevice) UID() string               { return d.id }
func (d *queryMockDevice) Name() string              { return "dev-" + d.id }
func (d *queryMockDevice) Type() device.DeviceType   { return device.DeviceType("sensor") }
func (d *queryMockDevice) Protocol() string          { return "mock" }
func (d *queryMockDevice) AgentName() string         { return "agent-x" }
func (d *queryMockDevice) Online() bool              { return d.state.Online }
func (d *queryMockDevice) LastSeen() time.Time       { return time.Now() }
func (d *queryMockDevice) State() device.DeviceState { return d.state }
func (d *queryMockDevice) Metadata() device.DeviceMetadata {
	return device.DeviceMetadata{Name: "dev-" + d.id}
}
func (d *queryMockDevice) Source() string         { return "config" }
func (d *queryMockDevice) Capabilities() []string { return nil }
