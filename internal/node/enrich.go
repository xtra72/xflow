package node

import (
	"context"
	"fmt"
	"strings"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// enrich.go (message-slim-metadata / enrich) 는 slim 된 메시지의 agent / device
// 그룹을 레지스트리 룩업(C)으로 in-flow 재수화하는 enrich 노드를 구현한다.
//
// slim egress 는 wire/스토리지에서 agent/device 그룹을 id-only 로 축소한다. enrich
// 노드는 그 반대 방향의 in-flow 보강을 제공한다: id 를 소스로 레지스트리에서
// type/name 을 조회하여 (a) 메타데이터 그룹을 재수화하거나 (b) payload 키에
// {type,id,name} 객체를 기록한다. 두 출력은 독립적으로/동시에 활성화할 수 있다.
//
// 설계 원칙(backward-compat):
//   - enrich 노드가 없는 플로우는 동작이 전혀 바뀌지 않는다.
//   - 룩업 실패/누락 id 는 에러가 아니라 passthrough(원본 그대로 통과)이며 메시지를
//     변형하지 않는다(no-op). 이는 스트림 중단을 막기 위한 기본 정책이다.
//   - id 는 항상 보존한다(룩업이 다른 id 를 돌려줘도 소스 id 우선).

// enrichSource 는 enrich 대상 그룹 종류이다.
type enrichSource string

const (
	enrichSourceAgent  enrichSource = "agent"
	enrichSourceDevice enrichSource = "device"
)

// 컴파일 타임 인터페이스 체크.
var _ Node = (*EnrichNode)(nil)

var (
	// ErrEnrichInvalidSource 는 source 가 누락 또는 agent/device 외 값일 때 반환된다.
	ErrEnrichInvalidSource = fmt.Errorf("enrich: %w: invalid source (must be one of: agent, device)", ErrInvalidConfig)
	// ErrEnrichNoOutput 는 to_metadata / to_payload 가 모두 비활성일 때 반환된다
	// (아무 것도 하지 않는 노드를 방지).
	ErrEnrichNoOutput = fmt.Errorf("enrich: %w: at least one output required (to_metadata or to_payload)", ErrInvalidConfig)
	// ErrEnrichInvalidConfig 는 개별 config 필드 타입이 잘못되었을 때 반환된다.
	ErrEnrichInvalidConfig = fmt.Errorf("enrich: %w: invalid config", ErrInvalidConfig)
)

// EnrichNode 는 id 소스로 agent/device 레지스트리를 조회하여 메시지를 보강하는
// 처리 노드이다.
type EnrichNode struct {
	*BaseNode

	// 설정 (factory 이후 immutable)
	source enrichSource
	// idSource 는 id 를 얻는 템플릿/JSONPath 이다 (예: "$.metadata.agent.id",
	// "$.payload.device_id"). resolveTemplateExpr 로 해석한다.
	idSource string
	// toMetadata 가 true 면 해당 그룹(agent/device)을 type/name 으로 재수화한다.
	toMetadata bool
	// toPayloadKey 가 비어있지 않으면 {type,id,name} 객체를 그 payload 키에 기록한다.
	toPayloadKey string

	// 의존성 룩업 (NodeOption 으로 주입, Init 에서 검증)
	agentLookup  AgentInfoLookup
	deviceLookup DeviceInfoLookup
}

// NewEnrichNode 는 NodeDef 와 옵션으로부터 enrich 노드를 생성한다.
//
// config 키:
//   - source (string, required): "agent" | "device".
//   - id_source (string, optional): id 를 얻는 템플릿. 미지정 시 기본값은
//     source 에 따라 "$.metadata.agent.id" 또는 "$.metadata.device.id".
//   - to_metadata (bool, optional, default false): true 면 메타데이터 그룹 재수화.
//   - to_payload (string, optional): 비어있지 않으면 그 payload 키에
//     {type,id,name} 객체 기록.
//
// to_metadata / to_payload 는 조합 가능하며, 둘 다 비활성이면 ErrEnrichNoOutput.
//
// 의존성(AgentInfoLookup / DeviceInfoLookup)은 NodeOption 으로 주입한다.
func NewEnrichNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	cfg := def.Config

	source, err := extractEnrichSource(cfg)
	if err != nil {
		return nil, err
	}

	idSource, err := extractEnrichIDSource(cfg, source)
	if err != nil {
		return nil, err
	}

	toMetadata, err := extractEnrichBool(cfg, "to_metadata")
	if err != nil {
		return nil, err
	}

	toPayloadKey, err := extractEnrichString(cfg, "to_payload")
	if err != nil {
		return nil, err
	}

	if !toMetadata && toPayloadKey == "" {
		return nil, ErrEnrichNoOutput
	}

	n := &EnrichNode{
		BaseNode:     base,
		source:       source,
		idSource:     idSource,
		toMetadata:   toMetadata,
		toPayloadKey: toPayloadKey,
	}

	// 주입된 룩업 추출(있으면). 없으면 Init 에서 해당 source 검증 실패.
	if l, ok := agentLookupFromConfig(base.config); ok {
		n.agentLookup = l
	}
	if l, ok := deviceLookupFromConfig(base.config); ok {
		n.deviceLookup = l
	}

	return n, nil
}

// Init 은 선택된 source 에 필요한 룩업이 주입되었는지 확인하고 running 으로 전이한다.
func (n *EnrichNode) Init(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	switch n.source {
	case enrichSourceAgent:
		if n.agentLookup == nil {
			return fmt.Errorf("enrich: %w: agent lookup not configured (use WithAgentInfoLookup)", ErrNodeNotInitialized)
		}
	case enrichSourceDevice:
		if n.deviceLookup == nil {
			return fmt.Errorf("enrich: %w: device lookup not configured (use WithDeviceInfoLookup)", ErrNodeNotInitialized)
		}
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Shutdown 은 노드를 정지 전이한다.
func (n *EnrichNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Process 는 메시지를 보강하여 통과시킨다.
//
// 동작:
//  1. id_source 로 id 를 해석한다. 실패/빈 값이면 passthrough(원본 그대로).
//  2. 레지스트리에서 조회한다. 실패(not-found)면 passthrough.
//  3. to_metadata 면 그룹 재수화, to_payload 면 payload 키 기록. id 는 항상 보존.
//
// enrich 는 in-place 로 동일 메시지를 반환한다(내부 흐름의 다른 노드와 동일한
// 통과 규약). 룩업 실패 시 아무 변형도 하지 않는다.
func (n *EnrichNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	if msg == nil {
		return nil, nil
	}

	id := n.resolveID(msg)
	if id == "" {
		// id 를 얻지 못하면 보강 불가 → 그대로 통과(에러 아님).
		return []message.Message{msg}, nil
	}

	meta, ok := n.lookup(id)
	if !ok {
		// 레지스트리에 없음 → 그대로 통과(변형 없음).
		return []message.Message{msg}, nil
	}

	// id 우선 보존: 소스 id 를 신뢰한다.
	meta.ID = id

	if n.toMetadata {
		n.applyToMetadata(msg, meta)
	}
	if n.toPayloadKey != "" {
		n.applyToPayload(msg, meta)
	}

	return []message.Message{msg}, nil
}

// resolveID 는 id_source 템플릿으로 메시지에서 id 문자열을 해석한다.
// 해석 실패 또는 비문자열/빈 값이면 "" 를 반환한다.
func (n *EnrichNode) resolveID(msg message.Message) string {
	v, err := resolveTemplateExpr(n.idSource, msg)
	if err != nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		// 문자열이 아니면 표준 문자열화(숫자 id 등 방어적 처리).
		if v == nil {
			return ""
		}
		s = fmt.Sprintf("%v", v)
	}
	return strings.TrimSpace(s)
}

// lookup 은 source 에 맞는 룩업을 수행한다.
func (n *EnrichNode) lookup(id string) (RegistryMeta, bool) {
	switch n.source {
	case enrichSourceAgent:
		if n.agentLookup == nil {
			return RegistryMeta{}, false
		}
		return n.agentLookup.LookupAgent(id)
	case enrichSourceDevice:
		if n.deviceLookup == nil {
			return RegistryMeta{}, false
		}
		return n.deviceLookup.LookupDevice(id)
	default:
		return RegistryMeta{}, false
	}
}

// applyToMetadata 는 source 그룹(agent/device)을 type/id/name 으로 재수화한다.
// 비어있지 않은 필드만 설정하며, 기존 그룹 위에 병합한다(id 보존).
func (n *EnrichNode) applyToMetadata(msg message.Message, meta RegistryMeta) {
	group, _ := msg.Metadata().GetGroup(string(n.source))
	if group == nil {
		group = make(map[string]string, 3)
	}
	if meta.Type != "" {
		group["type"] = meta.Type
	}
	if meta.ID != "" {
		group["id"] = meta.ID
	}
	if meta.Name != "" {
		group["name"] = meta.Name
	}
	msg.Metadata().SetGroup(string(n.source), group)
}

// applyToPayload 는 {type,id,name} 객체를 to_payload 키에 기록한다.
// 비어있지 않은 필드만 포함한다.
func (n *EnrichNode) applyToPayload(msg message.Message, meta RegistryMeta) {
	obj := make(map[string]any, 3)
	if meta.Type != "" {
		obj["type"] = meta.Type
	}
	if meta.ID != "" {
		obj["id"] = meta.ID
	}
	if meta.Name != "" {
		obj["name"] = meta.Name
	}
	msg.Payload().Set(n.toPayloadKey, obj)
}

// ---------------------------------------------------------------------------
// config 추출 헬퍼
// ---------------------------------------------------------------------------

// extractEnrichSource 는 source 를 검증하여 반환한다(필수).
func extractEnrichSource(cfg map[string]any) (enrichSource, error) {
	if cfg == nil {
		return "", ErrEnrichInvalidSource
	}
	raw, ok := cfg["source"]
	if !ok {
		return "", ErrEnrichInvalidSource
	}
	s, ok := raw.(string)
	if !ok {
		return "", ErrEnrichInvalidSource
	}
	switch enrichSource(strings.TrimSpace(s)) {
	case enrichSourceAgent:
		return enrichSourceAgent, nil
	case enrichSourceDevice:
		return enrichSourceDevice, nil
	default:
		return "", ErrEnrichInvalidSource
	}
}

// extractEnrichIDSource 는 id_source 를 반환한다. 미지정이면 source 기반 기본값.
func extractEnrichIDSource(cfg map[string]any, source enrichSource) (string, error) {
	raw, ok := cfg["id_source"]
	if !ok {
		return defaultEnrichIDSource(source), nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%w: id_source must be a string", ErrEnrichInvalidConfig)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultEnrichIDSource(source), nil
	}
	return s, nil
}

// defaultEnrichIDSource 는 source 에 대한 기본 id 템플릿을 반환한다.
func defaultEnrichIDSource(source enrichSource) string {
	switch source {
	case enrichSourceAgent:
		return "$.metadata.agent.id"
	case enrichSourceDevice:
		return "$.metadata.device.id"
	default:
		return ""
	}
}

// extractEnrichBool 은 지정 키의 bool 값을 반환한다(미지정 시 false).
func extractEnrichBool(cfg map[string]any, key string) (bool, error) {
	raw, ok := cfg[key]
	if !ok {
		return false, nil
	}
	b, ok := raw.(bool)
	if !ok {
		return false, fmt.Errorf("%w: %s must be a bool", ErrEnrichInvalidConfig, key)
	}
	return b, nil
}

// extractEnrichString 은 지정 키의 string 값을 trim 하여 반환한다(미지정 시 "").
func extractEnrichString(cfg map[string]any, key string) (string, error) {
	raw, ok := cfg[key]
	if !ok {
		return "", nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%w: %s must be a string", ErrEnrichInvalidConfig, key)
	}
	return strings.TrimSpace(s), nil
}
