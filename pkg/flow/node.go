package flow

import "github.com/google/uuid"

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

// NodeDef 는 플로우 내 노드의 정적 정의를 나타내는 구조체이다.
type NodeDef struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Config    map[string]any    `json:"config,omitempty"`
	Inputs    []Port            `json:"inputs"`
	Outputs   []Port            `json:"outputs"`
	Errors    []Port            `json:"errors,omitempty"`
	AgentRef  *AgentRef         `json:"agent_ref,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
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
