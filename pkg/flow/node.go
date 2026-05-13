package flow

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// PortDirection 은 포트의 데이터 흐름 방향을 나타내는 문자열 타입이다.
type PortDirection string

const (
	// PortInput 은 노드로 데이터가 들어오는 입력 포트를 나타낸다.
	PortInput PortDirection = "input"

	// PortOutput 은 노드에서 데이터가 나가는 출력 포트를 나타낸다.
	PortOutput PortDirection = "output"

	// PortError 는 노드의 에러 출력 포트를 나타낸다.
	PortError PortDirection = "error"
)

// Port 는 노드의 입출력 연결 지점을 정의하는 구조체이다.
type Port struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Direction PortDirection `json:"direction"`
}

// BridgeDirection 은 에이전트와 플로우 간의 데이터 흐름 방향을 나타내는 문자열 타입이다.
type BridgeDirection string

const (
	// BridgeIn 은 에이전트에서 플로우로 데이터가 들어오는 방향을 나타낸다.
	BridgeIn BridgeDirection = "in"

	// BridgeOut 은 플로우에서 에이전트로 데이터가 나가는 방향을 나타낸다.
	BridgeOut BridgeDirection = "out"

	// BridgeInOut 은 에이전트와 플로우 간 양방향 데이터 흐름을 나타낸다.
	BridgeInOut BridgeDirection = "inout"

	// BridgeRequestReply 는 요청/응답 패턴의 데이터 흐름을 나타낸다.
	BridgeRequestReply BridgeDirection = "request_reply"
)

// AgentRef 는 노드가 참조하는 에이전트 정보를 정의하는 구조체이다.
type AgentRef struct {
	AgentID   string          `json:"agent_id"`
	AgentName string          `json:"agent_name"`
	Direction BridgeDirection `json:"direction"`
}

// UnmarshalJSON 은 AgentRef 의 관대 역직렬화를 지원한다.
//
// 표준 형식: {"agent_id": "...", "agent_name": "...", "direction": "..."}
// 호환 형식: "<agent_name>" — 외부 도구 또는 client DynamicForm 의 flat 형식.
//
//	이 경우 string 을 AgentName 에 매핑한다 (AgentID 는 비워둠 — 서버 측 cascade
//	로직이 이름 기반 매칭으로 ID 를 채울 수 있다).
//
// 빈 객체 {} 또는 JSON null 은 nil-safe 하게 zero 값으로 역직렬화한다.
//
// SPEC: flow round-trip 결함 hotfix (2026-05-13)
func (ar *AgentRef) UnmarshalJSON(data []byte) error {
	// 1) 표준 형식 시도 (object): 별칭 타입을 사용해 무한 재귀 방지
	type agentRefAlias AgentRef
	var aux agentRefAlias
	if err := json.Unmarshal(data, &aux); err == nil {
		*ar = AgentRef(aux)
		return nil
	}
	// 2) 호환 형식 fallback (bare string)
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		ar.AgentID = ""
		ar.AgentName = s
		ar.Direction = ""
		return nil
	}
	return fmt.Errorf("flow.AgentRef: 지원하지 않는 JSON 형식 (object 또는 string 만 허용): %s", string(data))
}

// NodeDef 는 플로우 내 노드의 정적 정의를 나타내는 구조체이다.
type NodeDef struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Enabled  *bool             `json:"enabled,omitempty"`
	Config   map[string]any    `json:"config,omitempty"`
	Inputs   []Port            `json:"inputs"`
	Outputs  []Port            `json:"outputs"`
	Errors   []Port            `json:"errors,omitempty"`
	AgentRef *AgentRef         `json:"agent_ref,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// nodeDefStandardFields 는 NodeDef 의 표준 JSON 필드 키 집합이다.
// UnmarshalJSON 이 unknown field 를 Config 로 자동 수집할 때 사용된다.
var nodeDefStandardFields = map[string]bool{
	"id":        true,
	"name":      true,
	"type":      true,
	"enabled":   true,
	"config":    true,
	"inputs":    true,
	"outputs":   true,
	"errors":    true,
	"agent_ref": true,
	"metadata":  true,
	// layout 은 React Flow 렌더링 전용 메타데이터로, 노드 도메인 속성이 아니다.
	// Config 로 흡수하지 않고 무시한다 (export 가 별도 키로 보존하므로 round-trip OK).
	"layout": true,
}

// UnmarshalJSON 은 NodeDef 의 관대 역직렬화를 지원한다.
//
// 표준 필드 (id, name, type, enabled, config, inputs, outputs, errors, agent_ref, metadata)
// 외의 모든 unknown field 는 자동으로 Config 맵에 수집된다. 이는 다음 시나리오를
// 지원한다:
//
//  1. 외부 도구 또는 flat 형식 JSON 의 노드 속성 (category, poll_command, condition,
//     expression 등) 을 NodeDef.Config 로 복원한다.
//  2. 기존 flattenNodeData 가 만든 broken export JSON 의 round-trip 복구.
//  3. 미래 도입될 신규 필드의 forward-compatibility.
//
// 정책: 명시적 "config" 객체가 있고 unknown field 와 키가 충돌하면, 명시 값이 우선한다
// (사용자 의도 보존). layout 키는 렌더링 전용 메타이므로 Config 로 흡수하지 않는다.
//
// SPEC: flow import data-loss hotfix (2026-05-13)
func (n *NodeDef) UnmarshalJSON(data []byte) error {
	// 1) 모든 키를 일단 raw map 으로 받아 표준/비표준 으로 분리
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	standardPayload := make(map[string]json.RawMessage, len(raw))
	extraFields := make(map[string]any)

	for k, v := range raw {
		if nodeDefStandardFields[k] {
			standardPayload[k] = v
			continue
		}
		// unknown field 디코드 (any 타입으로 — string/number/bool/array/object 모두 허용)
		var val any
		if err := json.Unmarshal(v, &val); err != nil {
			return fmt.Errorf("flow.NodeDef.%s: %w", k, err)
		}
		extraFields[k] = val
	}

	// 2) 표준 필드만 별칭 타입으로 unmarshal (재귀 방지)
	type aliasNodeDef NodeDef
	var aux aliasNodeDef
	if len(standardPayload) > 0 {
		standardJSON, err := json.Marshal(standardPayload)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(standardJSON, &aux); err != nil {
			return err
		}
	}
	*n = NodeDef(aux)

	// 3) extra fields 를 Config 에 병합 (명시 값 우선)
	if len(extraFields) > 0 {
		if n.Config == nil {
			n.Config = make(map[string]any, len(extraFields))
		}
		for k, v := range extraFields {
			if _, exists := n.Config[k]; exists {
				continue // 명시적 config 값이 이미 있으면 덮어쓰지 않음
			}
			n.Config[k] = v
		}
	}
	return nil
}

// IsEnabled 는 노드의 활성화 상태를 반환한다.
// Enabled 가 nil 이면 기본값 true (활성화)를 반환한다.
func (n NodeDef) IsEnabled() bool {
	if n.Enabled == nil {
		return true
	}
	return *n.Enabled
}

// NodeOption 은 NewNodeDef 팩토리 함수에 전달되는 옵션 함수 타입이다.
type NodeOption func(*NodeDef)

// NewNodeDef 는 지정된 이름과 타입으로 새로운 NodeDef를 생성한다.
// 기본 입력 포트("in")와 출력 포트("out")가 자동으로 생성된다.
// 추가 설정은 NodeOption 함수를 통해 적용할 수 있다.
func NewNodeDef(name, nodeType string, opts ...NodeOption) NodeDef {
	node := NodeDef{
		ID:   uuid.New().String(),
		Name: name,
		Type: nodeType,
		Inputs: []Port{
			{
				ID:        uuid.New().String(),
				Name:      "in",
				Direction: PortInput,
			},
		},
		Outputs: []Port{
			{
				ID:        uuid.New().String(),
				Name:      "out",
				Direction: PortOutput,
			},
		},
	}

	for _, opt := range opts {
		opt(&node)
	}

	return node
}

// WithInputPorts 는 기본 입력 포트를 지정된 포트 목록으로 대체하는 NodeOption이다.
func WithInputPorts(ports ...Port) NodeOption {
	return func(n *NodeDef) {
		n.Inputs = ports
	}
}

// WithOutputPorts 는 기본 출력 포트를 지정된 포트 목록으로 대체하는 NodeOption이다.
func WithOutputPorts(ports ...Port) NodeOption {
	return func(n *NodeDef) {
		n.Outputs = ports
	}
}

// WithErrorPort 는 에러 포트를 생성하여 노드에 추가하는 NodeOption이다.
func WithErrorPort() NodeOption {
	return func(n *NodeDef) {
		n.Errors = []Port{
			{
				ID:        uuid.New().String(),
				Name:      "error",
				Direction: PortError,
			},
		}
	}
}

// WithBridgePorts 는 브릿지 direction에 따라 입출력 포트를 설정하는 NodeOption이다.
// BridgeIn: 출력 포트만 (에이전트→플로우), BridgeOut: 입력 포트만 (플로우→에이전트),
// BridgeInOut/BridgeRequestReply: 양방향 포트.
func WithBridgePorts(direction BridgeDirection) NodeOption {
	return func(n *NodeDef) {
		switch direction {
		case BridgeIn:
			n.Inputs = nil
			n.Outputs = []Port{
				{
					ID:        uuid.New().String(),
					Name:      "out",
					Direction: PortOutput,
				},
			}
		case BridgeOut:
			n.Inputs = []Port{
				{
					ID:        uuid.New().String(),
					Name:      "in",
					Direction: PortInput,
				},
			}
			n.Outputs = nil
		case BridgeInOut, BridgeRequestReply:
			n.Inputs = []Port{
				{
					ID:        uuid.New().String(),
					Name:      "in",
					Direction: PortInput,
				},
			}
			n.Outputs = []Port{
				{
					ID:        uuid.New().String(),
					Name:      "out",
					Direction: PortOutput,
				},
			}
		}
	}
}

// WithNodeConfig 는 노드 설정에 키-값 쌍을 추가하는 NodeOption이다.
// Config 맵이 nil이면 자동으로 초기화한다.
func WithNodeConfig(key string, value any) NodeOption {
	return func(n *NodeDef) {
		if n.Config == nil {
			n.Config = make(map[string]any)
		}
		n.Config[key] = value
	}
}

// WithAgentRef 는 에이전트 참조 정보를 노드에 설정하는 NodeOption이다.
func WithAgentRef(ref AgentRef) NodeOption {
	return func(n *NodeDef) {
		n.AgentRef = &ref
	}
}

// WithEnabled 는 노드의 활성화 상태를 설정하는 NodeOption이다.
func WithEnabled(enabled bool) NodeOption {
	return func(n *NodeDef) {
		n.Enabled = &enabled
	}
}

// WithNodeMetadata 는 노드 메타데이터에 키-값 쌍을 추가하는 NodeOption이다.
// Metadata 맵이 nil이면 자동으로 초기화한다.
func WithNodeMetadata(key, value string) NodeOption {
	return func(n *NodeDef) {
		if n.Metadata == nil {
			n.Metadata = make(map[string]string)
		}
		n.Metadata[key] = value
	}
}
