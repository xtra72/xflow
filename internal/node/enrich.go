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
// 그룹을 레지스트리 룩업으로 in-flow 재수화하는 enrich 노드를 구현한다.
//
// slim egress 는 wire/스토리지에서 agent/device 그룹을 id-only 로 축소한다. enrich
// 노드는 그 반대 방향의 in-flow 보강을 제공한다: id 를 소스로 레지스트리에서
// type/name 을 조회하여 (a) 메타데이터 그룹을 재수화하거나 (b) payload 키에
// {type,id,name} 객체를 기록한다.
//
// 멀티 소스(feature/enrich-multi-source): agent 와 device 는 독립된 중첩 config
// 블록으로 설정하며, 한 enrich 노드에서 동시에 둘 다 보강할 수 있다. 각 소스는
// 서로 독립적으로 실행된다(예: device 는 성공하고 agent 는 id 누락으로 스킵).
//
// 설계 원칙(backward-compat):
//   - enrich 노드가 없는 플로우는 동작이 전혀 바뀌지 않는다.
//   - 룩업 실패/누락 id 는 에러가 아니라 passthrough(원본 그대로 통과)이며 메시지를
//     변형하지 않는다(no-op). 이는 스트림 중단을 막기 위한 기본 정책이다.
//   - id 는 항상 보존한다(룩업이 다른 id 를 돌려줘도 소스 id 우선).
//   - to_metadata 인 소스는 조회 성공 여부와 무관하게 _slimKeep 마커를 설정한다
//     (egress 슬림이 기존 그룹을 되돌려 지우지 않도록).

// enrich 그룹 이름 상수(메타데이터 그룹 키 = 소스 이름).
const (
	enrichGroupAgent  = "agent"
	enrichGroupDevice = "device"
)

// 컴파일 타임 인터페이스 체크.
var _ Node = (*EnrichNode)(nil)

var (
	// ErrEnrichNoOutput 는 활성(active) 소스가 하나도 없을 때 반환된다
	// (아무 것도 하지 않는 노드를 방지).
	ErrEnrichNoOutput = fmt.Errorf("enrich: %w: at least one active source required (agent or device with to_metadata or to_payload)", ErrInvalidConfig)
	// ErrEnrichInvalidConfig 는 config 블록/필드 타입이 잘못되었을 때 반환된다.
	ErrEnrichInvalidConfig = fmt.Errorf("enrich: %w: invalid config", ErrInvalidConfig)
)

// enrichSpec 은 한 소스(agent 또는 device)의 활성 설정이다.
// 이 값이 존재하면(nil 이 아니면) 해당 소스는 활성 상태이다.
type enrichSpec struct {
	// idSource 는 id 를 얻는 템플릿/JSONPath 이다 (예: "$.metadata.agent.id").
	idSource string
	// toMetadata 가 true 면 해당 그룹을 type/name 으로 재수화한다.
	toMetadata bool
	// toPayloadKey 가 비어있지 않으면 {type,id,name} 객체를 그 payload 키에 기록한다.
	toPayloadKey string
}

// EnrichNode 는 agent/device 레지스트리를 조회하여 메시지를 보강하는 처리 노드이다.
// agent 와 device 는 독립 스펙으로 보관하며, 활성 스펙만 non-nil 이다.
type EnrichNode struct {
	*BaseNode

	// 소스별 활성 스펙 (nil 이면 비활성)
	agentSpec  *enrichSpec
	deviceSpec *enrichSpec

	// 의존성 룩업 (NodeOption 으로 주입, Init 에서 활성 소스에 대해 검증)
	agentLookup  AgentInfoLookup
	deviceLookup DeviceInfoLookup
}

// NewEnrichNode 는 NodeDef 와 옵션으로부터 enrich 노드를 생성한다.
//
// config 스키마(중첩 블록 — 신규 형식):
//
//	agent:                              # 선택 블록
//	  enabled: true                     # 선택 bool (블록 존재 시 기본 true)
//	  id_source: "$.metadata.agent.id"  # 선택 (기본 "$.metadata.agent.id")
//	  to_metadata: true                 # 선택 bool
//	  to_payload: "agent_info"          # 선택 string (payload 키)
//	device:                             # 선택 블록 (agent 와 동일 필드)
//	  ...
//
// 소스 활성 조건: 블록이 존재하고 AND enabled != false AND (to_metadata==true OR
// to_payload != ""). 활성 소스가 하나도 없으면 ErrEnrichNoOutput.
//
// 블록/필드 타입 오류(예: agent 가 map 이 아님, id_source 가 string 이 아님) →
// ErrEnrichInvalidConfig.
//
// 의존성(AgentInfoLookup / DeviceInfoLookup)은 NodeOption 으로 주입한다.
func NewEnrichNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	cfg := def.Config

	agentSpec, err := parseEnrichBlock(cfg, enrichGroupAgent)
	if err != nil {
		return nil, err
	}
	deviceSpec, err := parseEnrichBlock(cfg, enrichGroupDevice)
	if err != nil {
		return nil, err
	}

	if agentSpec == nil && deviceSpec == nil {
		return nil, ErrEnrichNoOutput
	}

	n := &EnrichNode{
		BaseNode:   base,
		agentSpec:  agentSpec,
		deviceSpec: deviceSpec,
	}

	// 주입된 룩업 추출(있으면). 없으면 Init 에서 활성 소스에 대해 검증 실패.
	if l, ok := agentLookupFromConfig(base.config); ok {
		n.agentLookup = l
	}
	if l, ok := deviceLookupFromConfig(base.config); ok {
		n.deviceLookup = l
	}

	return n, nil
}

// Init 은 활성 소스에 필요한 룩업이 주입되었는지 확인하고 running 으로 전이한다.
func (n *EnrichNode) Init(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if n.agentSpec != nil && n.agentLookup == nil {
		return fmt.Errorf("enrich: %w: agent lookup not configured (use WithAgentInfoLookup)", ErrNodeNotInitialized)
	}
	if n.deviceSpec != nil && n.deviceLookup == nil {
		return fmt.Errorf("enrich: %w: device lookup not configured (use WithDeviceInfoLookup)", ErrNodeNotInitialized)
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Shutdown 은 노드를 정지 전이한다.
func (n *EnrichNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Process 는 활성 각 소스(agent, device)를 독립적으로 보강하여 메시지를 통과시킨다.
// 두 소스는 서로 영향을 주지 않는다(한쪽 성공/한쪽 스킵 가능). 메시지를 에러로
// 만들지 않으며, in-place 로 동일 메시지를 반환한다.
func (n *EnrichNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	if msg == nil {
		return nil, nil
	}

	if n.agentSpec != nil {
		n.processSource(msg, enrichGroupAgent, n.agentSpec, n.lookupAgent)
	}
	if n.deviceSpec != nil {
		n.processSource(msg, enrichGroupDevice, n.deviceSpec, n.lookupDevice)
	}

	return []message.Message{msg}, nil
}

// processSource 는 한 소스(groupName)를 spec 에 따라 보강한다.
//
// 동작:
//  1. spec.toMetadata 이면 즉시 _slimKeep 마커를 설정한다(조회 성공 여부와 무관).
//  2. id_source 로 id 를 해석한다. 실패/빈 값이면 종료(마커는 이미 설정됨).
//  3. 레지스트리에서 조회한다. 실패(not-found)면 종료(마커 설정, fill 만 생략).
//  4. 조회 성공 시: to_metadata 면 그룹 재수화, to_payload 면 payload 키 기록. id 보존.
func (n *EnrichNode) processSource(msg message.Message, groupName string, spec *enrichSpec, lookup func(string) (RegistryMeta, bool)) {
	// to_metadata 이면 마커를 즉시 설정하여 조회 실패 시에도 기존 그룹이 보존되도록 한다
	// (marker 는 idempotent 이므로 중복 설정은 no-op).
	if spec.toMetadata {
		addSlimKeep(msg.Metadata(), groupName)
	}

	id := resolveEnrichID(msg, spec.idSource)
	if id == "" {
		return // id 없음 → 보강 불가(마커는 이미 설정됨).
	}

	meta, ok := lookup(id)
	if !ok {
		return // 레지스트리에 없음 → fill 생략(마커는 이미 설정됨).
	}

	// id 우선 보존: 소스 id 를 신뢰한다.
	meta.ID = id

	if spec.toMetadata {
		applyEnrichToMetadata(msg, groupName, meta)
	}
	if spec.toPayloadKey != "" {
		applyEnrichToPayload(msg, spec.toPayloadKey, meta)
	}
}

// lookupAgent / lookupDevice 는 소스별 룩업 어댑터이다(nil 룩업 방어 포함).
func (n *EnrichNode) lookupAgent(id string) (RegistryMeta, bool) {
	if n.agentLookup == nil {
		return RegistryMeta{}, false
	}
	return n.agentLookup.LookupAgent(id)
}

func (n *EnrichNode) lookupDevice(id string) (RegistryMeta, bool) {
	if n.deviceLookup == nil {
		return RegistryMeta{}, false
	}
	return n.deviceLookup.LookupDevice(id)
}

// resolveEnrichID 는 id_source 템플릿으로 메시지에서 id 문자열을 해석한다.
// 해석 실패 또는 비문자열/빈 값이면 "" 를 반환한다.
func resolveEnrichID(msg message.Message, idSource string) string {
	v, err := resolveTemplateExpr(idSource, msg)
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

// applyEnrichToMetadata 는 groupName 그룹(agent/device)을 type/id/name 으로 재수화한다.
// 비어있지 않은 필드만 설정하며, 기존 그룹 위에 병합한다(id 보존). 재수화한 그룹이
// egress 슬림에서 되돌려 지워지지 않도록 _slimKeep 마커에 그룹 이름을 누적한다.
func applyEnrichToMetadata(msg message.Message, groupName string, meta RegistryMeta) {
	group, _ := msg.Metadata().GetGroup(groupName)
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
	msg.Metadata().SetGroup(groupName, group)

	// 보존 마커에 이 그룹 이름을 누적(merge/de-dupe, 기존 값 clobber 금지).
	addSlimKeep(msg.Metadata(), groupName)
}

// addSlimKeep 은 _slimKeep 마커(쉼표 구분 그룹 이름 목록)에 groupName 을 추가한다.
// 이미 존재하면 no-op(중복 방지). 기존 목록을 보존하며 append 한다.
func addSlimKeep(md message.Metadata, groupName string) {
	existing, _ := md.Get(message.MetaKeySlimKeep)
	if existing == "" {
		md.Set(message.MetaKeySlimKeep, groupName)
		return
	}
	for _, name := range strings.Split(existing, ",") {
		if strings.TrimSpace(name) == groupName {
			return // 이미 포함 — no-op.
		}
	}
	md.Set(message.MetaKeySlimKeep, existing+","+groupName)
}

// applyEnrichToPayload 는 {type,id,name} 객체를 payloadKey 에 기록한다.
// 비어있지 않은 필드만 포함한다.
func applyEnrichToPayload(msg message.Message, payloadKey string, meta RegistryMeta) {
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
	msg.Payload().Set(payloadKey, obj)
}

// ---------------------------------------------------------------------------
// config 블록 파싱
// ---------------------------------------------------------------------------

// parseEnrichBlock 은 groupName("agent"/"device") 중첩 블록을 파싱하여 활성 스펙을
// 반환한다. 블록이 없거나 비활성(enabled=false 또는 출력 없음)이면 (nil, nil).
//
// 활성 조건: 블록 존재 AND enabled != false AND (to_metadata==true OR to_payload != "").
// 블록이 map 이 아니거나 필드 타입이 틀리면 ErrEnrichInvalidConfig.
func parseEnrichBlock(cfg map[string]any, groupName string) (*enrichSpec, error) {
	if cfg == nil {
		return nil, nil
	}
	raw, ok := cfg[groupName]
	if !ok {
		return nil, nil // 블록 없음 → 비활성.
	}
	block, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s must be a map", ErrEnrichInvalidConfig, groupName)
	}

	// enabled (기본 true when 블록 존재)
	enabled := true
	if v, present := block["enabled"]; present {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("%w: %s.enabled must be a bool", ErrEnrichInvalidConfig, groupName)
		}
		enabled = b
	}

	// id_source (기본 소스별 기본값)
	idSource := defaultEnrichIDSource(groupName)
	if v, present := block["id_source"]; present {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%w: %s.id_source must be a string", ErrEnrichInvalidConfig, groupName)
		}
		if s = strings.TrimSpace(s); s != "" {
			idSource = s
		}
	}

	// to_metadata
	toMetadata := false
	if v, present := block["to_metadata"]; present {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("%w: %s.to_metadata must be a bool", ErrEnrichInvalidConfig, groupName)
		}
		toMetadata = b
	}

	// to_payload
	toPayloadKey := ""
	if v, present := block["to_payload"]; present {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%w: %s.to_payload must be a string", ErrEnrichInvalidConfig, groupName)
		}
		toPayloadKey = strings.TrimSpace(s)
	}

	// 활성 판정: enabled 이고 출력이 하나 이상.
	if !enabled || (!toMetadata && toPayloadKey == "") {
		return nil, nil
	}

	return &enrichSpec{
		idSource:     idSource,
		toMetadata:   toMetadata,
		toPayloadKey: toPayloadKey,
	}, nil
}

// defaultEnrichIDSource 는 groupName 에 대한 기본 id 템플릿을 반환한다.
func defaultEnrichIDSource(groupName string) string {
	switch groupName {
	case enrichGroupAgent:
		return "$.metadata.agent.id"
	case enrichGroupDevice:
		return "$.metadata.device.id"
	default:
		return ""
	}
}
