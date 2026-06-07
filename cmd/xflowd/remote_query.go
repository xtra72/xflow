// remote_query.go 는 M8(그룹 J) 원격 관리 클라이언트의 READ/QUERY 프록시 + 스트림
// 소스 어댑터를 정의한다(@SPEC:SPEC-REMOTE-001 M8, spec §5.5/§5.10, REQ-J01/J04/J06/J08).
//
// internal/remote 는 import cycle 회피를 위해 중립 remote.QuerySource /
// remote.StreamSource 인터페이스만 정의한다. 본 파일이 그 인터페이스를 로컬 read
// 어댑터(FlowServiceAdapter 의 FlowStatus/ListFlowNodes/GetFlowNode/GetFlow,
// AgentServiceAdapter 의 AgentStats/GetAgent, device 레지스트리의 Get→State/Commands/
// Metadata)에 바인딩한다(A10 — 노드의 기존 로컬 read 핸들러 재실행).
//
// READ-ONLY(REQ-J03): query-action 은 read 매핑만 수행한다. 변경 의미 action 은 remote
// client 가 allowlist 로 거부하므로 본 브리지에 도달하지 않는다.
//
// redaction(REQ-J06): 본 브리지는 원본 JSON 을 반환하고, remote client 가
// ClientConfig.QueryRedactor(newQueryRedactor)로 전송 전 마스킹한다. 리댁터는
// secret_fields SoT(handler.RedactSensitiveConfig)를 임의 JSON 형태에 재귀 적용한다.
//
// 미가용 데이터(스토어/토픽/세션/시리즈/로그 등 어댑터로 cheaply 노출 불가)는 패닉
// 대신 remote.ErrQueryActionUnsupported 를 반환한다(서버가 502 node-error 로 매핑 —
// REQ-J07). 향후 해당 read 핸들러가 어댑터로 노출되면 매핑을 추가한다(seam).
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/remote"
)

// queryFlowReader 는 flow read query-action 에 필요한 FlowServiceAdapter 의 좁은
// 인터페이스이다(*service.FlowServiceAdapter 가 만족).
type queryFlowReader interface {
	FlowStatus(ctx context.Context, id string) (*handler.FlowStatusInfo, error)
	ListFlowNodes(ctx context.Context, flowID string) ([]handler.FlowNodeInfo, error)
	GetFlowNode(ctx context.Context, flowID, nodeID string) (*handler.FlowNodeInfo, error)
	GetFlow(ctx context.Context, id string) (*handler.FlowInfo, error)
}

// queryAgentReader 는 agent read query-action 에 필요한 AgentServiceAdapter 의 좁은
// 인터페이스이다.
//
// ExecAgent 는 에이전트에 read 전용 명령(list_connections 등)을 전달해 결과 JSON 을
// 받는다. agent/sessions 가 로컬 SessionsTab 과 동일하게 list_connections 결과를
// 반환하는 데 사용된다(REQ-J04). 변경 의미 명령은 query allowlist 밖이므로 도달하지
// 않는다(READ-ONLY — REQ-J03).
type queryAgentReader interface {
	AgentStats(ctx context.Context, id string) (*handler.AgentStatsInfo, error)
	GetAgent(ctx context.Context, id string, detail string) (*handler.AgentInfo, error)
	ExecAgent(ctx context.Context, id string, data []byte) (json.RawMessage, error)
}

// queryDeviceReader 는 device read query-action 에 필요한 레지스트리의 좁은
// 인터페이스이다(*device.Registry 의 Get/List 를 만족).
//
// List 는 agent/devices 가 에이전트 이름으로 연결 디바이스를 조회하는 데 사용된다
// (로컬 useDevices({agent}) 와 동일 소스 — REQ-J04).
type queryDeviceReader interface {
	Get(id string) (device.Device, error)
	List(filter device.DeviceFilter) []device.Device
}

// queryStoreReader 는 agent/store query-action 에 필요한 store keys 조회 인터페이스이다
// (GET /store/{agent}/keys 와 동일 소스 — StoreQueryHandler.ListKeys 매핑).
//
// agentName 으로 store 시스템 에이전트를 찾아 StaticKeysSnapshot 을 응답 형상으로
// 변환한다(구현은 remote_agent_data.go 의 agentManagerStoreReader).
type queryStoreReader interface {
	StoreKeys(ctx context.Context, agentName string) (*handler.StoreKeysListResponse, error)
}

// querySeriesReader 는 agent/series query-action 에 필요한 시리즈 키 목록 조회
// 인터페이스이다(GET /tsdb/series 와 동일 소스 — TSDBAgent.TSDB().SeriesKeys 매핑).
type querySeriesReader interface {
	SeriesList(ctx context.Context, agentName string) ([]string, error)
}

// remoteQuerySource 는 로컬 read 어댑터를 remote.QuerySource 로 어댑트한다(REQ-J01/J04).
type remoteQuerySource struct {
	flows   queryFlowReader
	agents  queryAgentReader
	devices queryDeviceReader
	store   queryStoreReader
	series  querySeriesReader
}

var _ remote.QuerySource = (*remoteQuerySource)(nil)

// newRemoteQuerySource 는 read 어댑터를 바인딩한 query 소스를 생성한다.
func newRemoteQuerySource(flows queryFlowReader, agents queryAgentReader, devices queryDeviceReader, store queryStoreReader, series querySeriesReader) *remoteQuerySource {
	return &remoteQuerySource{flows: flows, agents: agents, devices: devices, store: store, series: series}
}

// Query 는 domain/queryAction 을 로컬 read 핸들러로 매핑하여 결과 JSON 을 반환한다
// (REQ-J04 매핑 표). 미가용 action 은 remote.ErrQueryActionUnsupported 를 반환한다.
func (s *remoteQuerySource) Query(ctx context.Context, domain, action string, args json.RawMessage) (json.RawMessage, error) {
	switch domain {
	case remote.DomainFlow:
		return s.queryFlow(ctx, action, args)
	case remote.DomainAgent:
		return s.queryAgent(ctx, action, args)
	case remote.DomainDevice:
		return s.queryDevice(ctx, action, args)
	default:
		return nil, fmt.Errorf("%w: domain %q", remote.ErrQueryActionUnsupported, domain)
	}
}

func (s *remoteQuerySource) queryFlow(ctx context.Context, action string, args json.RawMessage) (json.RawMessage, error) {
	switch action {
	case remote.QueryActionStatus:
		id, err := queryID(args)
		if err != nil {
			return nil, err
		}
		info, err := s.flows.FlowStatus(ctx, id)
		if err != nil {
			return nil, err
		}
		return marshalQuery(info)
	case remote.QueryActionNodes:
		id, err := queryID(args)
		if err != nil {
			return nil, err
		}
		nodes, err := s.flows.ListFlowNodes(ctx, id)
		if err != nil {
			return nil, err
		}
		return marshalQuery(map[string]any{"nodes": nodes})
	case remote.QueryActionNode:
		var p flowNodeArgs
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("flow node args: %w", err)
		}
		if p.ID == "" || p.NodeID == "" {
			return nil, fmt.Errorf("flow node: id 와 node_id 는 필수입니다")
		}
		node, err := s.flows.GetFlowNode(ctx, p.ID, p.NodeID)
		if err != nil {
			return nil, err
		}
		return marshalQuery(node)
	case remote.QueryActionGet:
		id, err := queryID(args)
		if err != nil {
			return nil, err
		}
		info, err := s.flows.GetFlow(ctx, id)
		if err != nil {
			return nil, err
		}
		return marshalQuery(info)
	case remote.QueryActionList, remote.QueryActionLogs:
		// list 는 그룹 E 미러 우선(서버 측), logs 는 어댑터로 노출되지 않음 → 미지원.
		return nil, fmt.Errorf("%w: flow/%s", remote.ErrQueryActionUnsupported, action)
	default:
		return nil, fmt.Errorf("%w: flow/%s", remote.ErrQueryActionUnsupported, action)
	}
}

func (s *remoteQuerySource) queryAgent(ctx context.Context, action string, args json.RawMessage) (json.RawMessage, error) {
	switch action {
	case remote.QueryActionStats:
		id, err := queryID(args)
		if err != nil {
			return nil, err
		}
		info, err := s.agents.AgentStats(ctx, id)
		if err != nil {
			return nil, err
		}
		return marshalQuery(info)
	case remote.QueryActionGet, remote.QueryActionConfig, remote.QueryActionTopics:
		id, err := queryID(args)
		if err != nil {
			return nil, err
		}
		// get/config/topics 는 모두 GetAgent(detail=full) 의 동일 소스를 사용한다.
		//   - config: get 의 Config 섹션과 동일 소스(full 상세).
		//   - topics: 토픽 데이터는 AgentInfo.State 에 존재한다(로컬 TopicsTab 이
		//     agent.state.{subscribed_topics,pub_topics,…} 를 읽음). 동일 full 페이로드를
		//     반환하여 로컬 패널과 IDENTICAL 형상을 보장한다(REQ-J04).
		info, err := s.agents.GetAgent(ctx, id, "full")
		if err != nil {
			return nil, err
		}
		return marshalQuery(info)
	case remote.QueryActionDevices:
		return s.queryAgentDevices(ctx, args)
	case remote.QueryActionSessions:
		return s.queryAgentSessions(ctx, args)
	case remote.QueryActionStore:
		return s.queryAgentStore(ctx, args)
	case remote.QueryActionSeries:
		return s.queryAgentSeries(ctx, args)
	case remote.QueryActionList:
		// list 는 그룹 E 미러 우선(서버 측). 라이브 보강이 필요해지면 추가한다(seam).
		return nil, fmt.Errorf("%w: agent/list(미러 우선)", remote.ErrQueryActionUnsupported)
	default:
		return nil, fmt.Errorf("%w: agent/%s", remote.ErrQueryActionUnsupported, action)
	}
}

// queryAgentDevices 는 agent/devices 를 디바이스 레지스트리 List(AgentName) 로 매핑한다
// (로컬 useDevices({agent}) 와 동일 소스 — REQ-J04). 응답은 {data:[…]} 형상으로
// 로컬 useDevices 가 소비하는 envelope 과 동일하다.
func (s *remoteQuerySource) queryAgentDevices(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	id, err := queryID(args)
	if err != nil {
		return nil, err
	}
	// 디바이스 필터는 agent NAME 기반이므로 먼저 id→name 을 해소한다.
	info, err := s.agents.GetAgent(ctx, id, "")
	if err != nil {
		return nil, err
	}
	devs := s.devices.List(device.DeviceFilter{AgentName: info.Name})
	items := make([]map[string]any, 0, len(devs))
	for _, d := range devs {
		items = append(items, deviceListItem(d))
	}
	return marshalQuery(map[string]any{"data": items})
}

// queryAgentSessions 는 agent/sessions 를 read 전용 list_connections 명령으로 매핑한다
// (로컬 SessionsTab 과 동일 소스 — REQ-J04). 결과는 그대로 반환하며(로컬 패널이
// connections 를 언랩), redaction 은 client 가 전송 전 수행한다(REQ-J06).
func (s *remoteQuerySource) queryAgentSessions(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	id, err := queryID(args)
	if err != nil {
		return nil, err
	}
	cmd := json.RawMessage(`{"command":"list_connections"}`)
	res, err := s.agents.ExecAgent(ctx, id, cmd)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return json.RawMessage("{}"), nil
	}
	return res, nil
}

// queryAgentStore 는 agent/store 를 store 시스템 에이전트의 keys snapshot 으로 매핑한다
// (GET /store/{agent}/keys 와 동일 형상 — REQ-J04). store 는 agent NAME 기반이므로
// 먼저 id→name 을 해소한다.
func (s *remoteQuerySource) queryAgentStore(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	id, err := queryID(args)
	if err != nil {
		return nil, err
	}
	info, err := s.agents.GetAgent(ctx, id, "")
	if err != nil {
		return nil, err
	}
	resp, err := s.store.StoreKeys(ctx, info.Name)
	if err != nil {
		return nil, err
	}
	return marshalQuery(resp)
}

// queryAgentSeries 는 agent/series 를 TSDB 시리즈 키 목록으로 매핑한다(GET /tsdb/series
// 와 동일 형상 {series:[…], count} — REQ-J04). series 는 agent NAME 기반이므로 먼저
// id→name 을 해소한다.
func (s *remoteQuerySource) queryAgentSeries(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	id, err := queryID(args)
	if err != nil {
		return nil, err
	}
	info, err := s.agents.GetAgent(ctx, id, "")
	if err != nil {
		return nil, err
	}
	keys, err := s.series.SeriesList(ctx, info.Name)
	if err != nil {
		return nil, err
	}
	if keys == nil {
		keys = []string{}
	}
	return marshalQuery(map[string]any{"series": keys, "count": len(keys)})
}

// deviceListItem 은 디바이스를 로컬 DeviceResponse(GET /devices) 와 동일 키 집합의
// map 으로 변환한다(REQ-J04 — 로컬 useDevices 가 소비하는 형상). 레지스트리
// 메타데이터 병합은 query 경로에서 생략한다(목록 표시에는 기본 필드로 충분).
func deviceListItem(d device.Device) map[string]any {
	return map[string]any{
		"id":           d.ID(),
		"name":         d.Name(),
		"type":         string(d.Type()),
		"protocol":     d.Protocol(),
		"agent_name":   d.AgentName(),
		"online":       d.Online(),
		"last_seen":    d.LastSeen(),
		"source":       d.Source(),
		"capabilities": d.Capabilities(),
	}
}

func (s *remoteQuerySource) queryDevice(_ context.Context, action string, args json.RawMessage) (json.RawMessage, error) {
	id, err := queryID(args)
	if err != nil {
		return nil, err
	}
	dev, err := s.devices.Get(id)
	if err != nil {
		return nil, err
	}
	switch action {
	case remote.QueryActionGet:
		return marshalQuery(map[string]any{
			"id":         dev.ID(),
			"name":       dev.Name(),
			"protocol":   dev.Protocol(),
			"type":       string(dev.Type()),
			"agent_name": dev.AgentName(),
			"online":     dev.Online(),
			"state":      dev.State(),
			"metadata":   dev.Metadata(),
		})
	case remote.QueryActionState:
		return marshalQuery(dev.State())
	case remote.QueryActionMetadata:
		return marshalQuery(dev.Metadata())
	case remote.QueryActionCommands:
		// 명령 스펙은 ControllableDevice 만 보유한다. 비제어 디바이스는 빈 목록.
		if cd, ok := dev.(device.ControllableDevice); ok {
			return marshalQuery(map[string]any{"commands": cd.Commands()})
		}
		return marshalQuery(map[string]any{"commands": []any{}})
	case remote.QueryActionList:
		return nil, fmt.Errorf("%w: device/list(미러 우선)", remote.ErrQueryActionUnsupported)
	default:
		return nil, fmt.Errorf("%w: device/%s", remote.ErrQueryActionUnsupported, action)
	}
}

// flowNodeArgs 는 flow.node query-action 의 인자({id, node_id})이다.
type flowNodeArgs struct {
	ID     string `json:"id"`
	NodeID string `json:"node_id"`
}

// queryArgs 는 단일 id 기반 query-action 의 공통 인자이다.
type queryArgs struct {
	ID string `json:"id"`
}

// queryID 는 {"id": "..."} args 에서 ID 를 추출한다. 누락 시 오류.
func queryID(args json.RawMessage) (string, error) {
	var p queryArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("decode query id args: %w", err)
	}
	if p.ID == "" {
		return "", fmt.Errorf("query: id 는 필수입니다")
	}
	return p.ID, nil
}

// marshalQuery 는 read 결과를 json.RawMessage 로 직렬화한다. nil 은 빈 객체로 처리한다.
func marshalQuery(v any) (json.RawMessage, error) {
	if v == nil {
		return json.RawMessage("{}"), nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal query result: %w", err)
	}
	return data, nil
}

// queryRedactor 는 임의 JSON 본문에 secret_fields redaction 정책을 재귀 적용하는
// remote.QueryRedactor 구현이다(REQ-J06 — secret_fields SoT 재사용).
type queryRedactor struct{}

var _ remote.QueryRedactor = (*queryRedactor)(nil)

// newQueryRedactor 는 query/stream 응답용 리댁터를 생성한다.
func newQueryRedactor() remote.QueryRedactor {
	return &queryRedactor{}
}

// Redact 는 JSON 을 디코드하여 시크릿 필드를 재귀 제거한 뒤 재직렬화한다(REQ-J06).
//
//   - 객체({...}): handler.RedactSensitiveConfig(재귀 — 중첩 맵/배열 포함).
//   - 배열([...]): 각 요소를 재귀 redaction.
//   - 스칼라/디코드 불가: 그대로 통과(graceful — 비시크릿 데이터).
func (r *queryRedactor) Redact(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return data // 디코드 불가 → 보수적으로 원본 통과.
	}
	out, err := json.Marshal(redactJSONValue(v))
	if err != nil {
		return data
	}
	return out
}

// redactJSONValue 는 임의 JSON 값(map/slice/scalar)에 시크릿 redaction 을 재귀 적용한다.
// handler.RedactSensitiveConfig 와 동일 정책(SoT 재사용)을 map 에 적용하고, 배열은
// 요소별로 재귀한다.
func redactJSONValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return handler.RedactSensitiveConfig(t)
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = redactJSONValue(item)
		}
		return out
	default:
		return v
	}
}
