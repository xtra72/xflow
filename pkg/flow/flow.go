package flow

import (
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// ErrorPolicy
// ---------------------------------------------------------------------------

// ErrorPolicy 는 Flow 실행 중 에러 처리 방식을 나타내는 문자열 타입이다.
type ErrorPolicy string

const (
	// ErrorPropagate 는 에러를 상위로 전파하는 기본 정책이다.
	ErrorPropagate ErrorPolicy = "propagate"

	// ErrorIgnore 는 에러를 무시하고 계속 실행하는 정책이다.
	ErrorIgnore ErrorPolicy = "ignore"

	// ErrorStop 은 에러 발생 시 Flow 실행을 중지하는 정책이다.
	ErrorStop ErrorPolicy = "stop"
)

// ---------------------------------------------------------------------------
// FlowConfig
// ---------------------------------------------------------------------------

// FlowConfig 는 Flow의 실행 설정을 정의하는 구조체이다.
type FlowConfig struct {
	TrackHistory   bool        `json:"track_history"`
	MaxHistorySize int         `json:"max_history_size"`
	ErrorHandling  ErrorPolicy `json:"error_handling"`
}

// ---------------------------------------------------------------------------
// Flow 인터페이스
// ---------------------------------------------------------------------------

// Flow 는 플로우의 읽기/쓰기 작업을 정의하는 핵심 인터페이스이다.
type Flow interface {
	// ID 는 Flow의 고유 식별자를 반환한다.
	ID() string
	// Name 은 Flow의 이름을 반환한다.
	Name() string
	// Description 은 Flow의 설명을 반환한다.
	Description() string
	// State 는 Flow의 현재 생명주기 상태를 반환한다.
	State() FlowState
	// Nodes 는 Flow에 등록된 모든 노드의 방어적 복사본을 반환한다.
	Nodes() []NodeDef
	// Node 는 ID 또는 이름으로 노드를 조회한다. ID를 먼저 검색한 후 이름으로 검색한다.
	Node(idOrName string) (NodeDef, bool)
	// Wires 는 Flow에 등록된 모든 와이어의 방어적 복사본을 반환한다.
	Wires() []Wire
	// Wire 는 ID로 와이어를 조회한다.
	Wire(id string) (Wire, bool)
	// Config 는 현재 FlowConfig를 반환한다.
	Config() FlowConfig
	// Metadata 는 메타데이터의 방어적 복사본을 반환한다.
	Metadata() map[string]string
	// CreatedAt 은 Flow 생성 시각을 반환한다.
	CreatedAt() time.Time
	// UpdatedAt 은 Flow의 마지막 수정 시각을 반환한다.
	UpdatedAt() time.Time

	// AddNode 는 Flow에 새로운 노드를 추가한다.
	// 중복 ID이면 ErrDuplicateNodeID, 중복 이름이면 ErrDuplicateNodeName을 반환한다.
	AddNode(node NodeDef) error
	// RemoveNode 는 ID 또는 이름으로 노드를 제거한다. 연결된 Wire도 함께 제거된다.
	// 노드를 찾을 수 없으면 ErrNodeNotFound를 반환한다.
	RemoveNode(idOrName string) error
	// AddWire 는 Flow에 새로운 와이어를 추가한다.
	// 소스/타겟 노드 및 포트의 유효성을 검증한다.
	AddWire(wire Wire) error
	// RemoveWire 는 ID로 와이어를 제거한다.
	// 와이어를 찾을 수 없으면 ErrWireNotFound를 반환한다.
	RemoveWire(id string) error
	// SetDescription 은 Flow의 설명을 변경한다.
	SetDescription(desc string)
	// SetConfig 는 Flow의 설정을 변경한다.
	SetConfig(config FlowConfig)
	// SetMetadata 는 메타데이터에 키-값 쌍을 설정한다.
	SetMetadata(key, value string)
	// RemoveMetadata 는 지정된 키의 메타데이터를 제거한다.
	RemoveMetadata(key string)
	// SetState 는 Flow의 상태를 변경한다.
	// 유효하지 않은 전이이면 ErrInvalidStateTransition을 반환한다.
	SetState(state FlowState) error
}

// ---------------------------------------------------------------------------
// defaultFlow (비공개 구현체)
// ---------------------------------------------------------------------------

// defaultFlow 는 Flow 인터페이스의 기본 구현체이다.
type defaultFlow struct {
	id          string
	name        string
	description string
	state       FlowState
	nodes       []NodeDef
	wires       []Wire
	config      FlowConfig
	metadata    map[string]string
	createdAt   time.Time
	updatedAt   time.Time
}

// ---------------------------------------------------------------------------
// flowConfig (빌더용 비공개 구조체)
// ---------------------------------------------------------------------------

// flowConfig 는 NewFlow 팩토리에서 옵션을 수집하기 위한 비공개 구조체이다.
type flowConfig struct {
	description string
	config      FlowConfig
	metadata    map[string]string
	nodes       []NodeDef
	wires       []Wire
}

// ---------------------------------------------------------------------------
// FlowOption
// ---------------------------------------------------------------------------

// FlowOption 은 NewFlow 팩토리 함수에 전달되는 옵션 함수 타입이다.
type FlowOption func(*flowConfig)

// ---------------------------------------------------------------------------
// NewFlow 팩토리
// ---------------------------------------------------------------------------

// NewFlow 는 지정된 이름으로 새로운 Flow를 생성한다.
// 기본 상태는 FlowStored이며, ID는 UUID로 자동 생성된다.
// 기본 FlowConfig는 MaxHistorySize: 100, ErrorHandling: ErrorPropagate이다.
// 추가 설정은 FlowOption 함수를 통해 적용할 수 있다.
func NewFlow(name string, opts ...FlowOption) Flow {
	cfg := &flowConfig{
		config: FlowConfig{
			MaxHistorySize: 100,
			ErrorHandling:  ErrorPropagate,
		},
		metadata: make(map[string]string),
	}

	for _, opt := range opts {
		opt(cfg)
	}

	now := time.Now()

	return &defaultFlow{
		id:          uuid.New().String(),
		name:        name,
		description: cfg.description,
		state:       FlowStored,
		nodes:       cfg.nodes,
		wires:       cfg.wires,
		config:      cfg.config,
		metadata:    cfg.metadata,
		createdAt:   now,
		updatedAt:   now,
	}
}

// ---------------------------------------------------------------------------
// FlowOption 함수들
// ---------------------------------------------------------------------------

// WithDescription 은 Flow의 설명을 설정하는 FlowOption이다.
func WithDescription(desc string) FlowOption {
	return func(c *flowConfig) {
		c.description = desc
	}
}

// WithFlowConfig 는 Flow의 설정을 지정하는 FlowOption이다.
func WithFlowConfig(config FlowConfig) FlowOption {
	return func(c *flowConfig) {
		c.config = config
	}
}

// WithFlowMetadata 는 Flow 메타데이터에 키-값 쌍을 추가하는 FlowOption이다.
func WithFlowMetadata(key, value string) FlowOption {
	return func(c *flowConfig) {
		if c.metadata == nil {
			c.metadata = make(map[string]string)
		}
		c.metadata[key] = value
	}
}

// WithNodes 는 Flow에 초기 노드 목록을 설정하는 FlowOption이다.
func WithNodes(nodes ...NodeDef) FlowOption {
	return func(c *flowConfig) {
		c.nodes = append(c.nodes, nodes...)
	}
}

// WithWires 는 Flow에 초기 와이어 목록을 설정하는 FlowOption이다.
func WithWires(wires ...Wire) FlowOption {
	return func(c *flowConfig) {
		c.wires = append(c.wires, wires...)
	}
}

// ---------------------------------------------------------------------------
// 읽기 메서드 구현
// ---------------------------------------------------------------------------

func (f *defaultFlow) ID() string          { return f.id }
func (f *defaultFlow) Name() string        { return f.name }
func (f *defaultFlow) Description() string { return f.description }
func (f *defaultFlow) State() FlowState    { return f.state }

// Nodes 는 등록된 노드의 방어적 복사본을 반환한다.
func (f *defaultFlow) Nodes() []NodeDef {
	if f.nodes == nil {
		return []NodeDef{}
	}
	copied := make([]NodeDef, len(f.nodes))
	copy(copied, f.nodes)
	return copied
}

// Node 는 ID 또는 이름으로 노드를 조회한다.
// ID를 먼저 검색한 후 이름으로 검색한다.
func (f *defaultFlow) Node(idOrName string) (NodeDef, bool) {
	// ID로 먼저 검색
	for _, n := range f.nodes {
		if n.ID == idOrName {
			return n, true
		}
	}
	// Name으로 검색
	for _, n := range f.nodes {
		if n.Name == idOrName {
			return n, true
		}
	}
	return NodeDef{}, false
}

// Wires 는 등록된 와이어의 방어적 복사본을 반환한다.
func (f *defaultFlow) Wires() []Wire {
	if f.wires == nil {
		return []Wire{}
	}
	copied := make([]Wire, len(f.wires))
	copy(copied, f.wires)
	return copied
}

// Wire 는 ID로 와이어를 조회한다.
func (f *defaultFlow) Wire(id string) (Wire, bool) {
	for _, w := range f.wires {
		if w.ID == id {
			return w, true
		}
	}
	return Wire{}, false
}

// Config 는 현재 FlowConfig를 반환한다.
func (f *defaultFlow) Config() FlowConfig {
	return f.config
}

// Metadata 는 메타데이터의 방어적 복사본을 반환한다.
func (f *defaultFlow) Metadata() map[string]string {
	copied := make(map[string]string, len(f.metadata))
	for k, v := range f.metadata {
		copied[k] = v
	}
	return copied
}

// CreatedAt 은 Flow 생성 시각을 반환한다.
func (f *defaultFlow) CreatedAt() time.Time { return f.createdAt }

// UpdatedAt 은 Flow의 마지막 수정 시각을 반환한다.
func (f *defaultFlow) UpdatedAt() time.Time { return f.updatedAt }

// ---------------------------------------------------------------------------
// 변이(Mutation) 메서드 구현
// ---------------------------------------------------------------------------

// AddNode 는 Flow에 새로운 노드를 추가한다.
// 중복 ID이면 ErrDuplicateNodeID, 중복 이름이면 ErrDuplicateNodeName을 반환한다.
func (f *defaultFlow) AddNode(node NodeDef) error {
	for _, existing := range f.nodes {
		if existing.ID == node.ID {
			return ErrDuplicateNodeID
		}
	}
	for _, existing := range f.nodes {
		if existing.Name == node.Name {
			return ErrDuplicateNodeName
		}
	}

	f.nodes = append(f.nodes, node)
	f.updatedAt = time.Now()
	return nil
}

// RemoveNode 는 ID 또는 이름으로 노드를 제거한다.
// 노드에 연결된 모든 Wire도 함께 제거된다.
// 노드를 찾을 수 없으면 ErrNodeNotFound를 반환한다.
func (f *defaultFlow) RemoveNode(idOrName string) error {
	idx := -1
	var nodeID string

	// ID로 먼저 검색
	for i, n := range f.nodes {
		if n.ID == idOrName {
			idx = i
			nodeID = n.ID
			break
		}
	}
	// Name으로 검색
	if idx == -1 {
		for i, n := range f.nodes {
			if n.Name == idOrName {
				idx = i
				nodeID = n.ID
				break
			}
		}
	}

	if idx == -1 {
		return ErrNodeNotFound
	}

	// 연결된 Wire 제거
	filtered := f.wires[:0]
	for _, w := range f.wires {
		if w.SourceNodeID != nodeID && w.TargetNodeID != nodeID {
			filtered = append(filtered, w)
		}
	}
	f.wires = filtered

	// 노드 제거 (순서 유지)
	f.nodes = append(f.nodes[:idx], f.nodes[idx+1:]...)
	f.updatedAt = time.Now()
	return nil
}

// AddWire 는 Flow에 새로운 와이어를 추가한다.
// 소스/타겟 노드와 포트의 존재 여부를 검증한다.
func (f *defaultFlow) AddWire(wire Wire) error {
	// 중복 Wire ID 검사
	for _, existing := range f.wires {
		if existing.ID == wire.ID {
			return ErrDuplicateWireID
		}
	}

	// 소스 노드 검증
	srcNode, srcFound := f.findNodeByID(wire.SourceNodeID)
	if !srcFound {
		return ErrInvalidWireSource
	}

	// 타겟 노드 검증
	tgtNode, tgtFound := f.findNodeByID(wire.TargetNodeID)
	if !tgtFound {
		return ErrInvalidWireTarget
	}

	// 소스 포트 검증: Outputs 또는 ErrorPort에서 Name 매칭
	if !f.hasOutputPort(srcNode, wire.SourcePort) {
		return ErrInvalidWireSource
	}

	// 타겟 포트 검증: Inputs에서 Name 매칭
	if !f.hasInputPort(tgtNode, wire.TargetPort) {
		return ErrInvalidWireTarget
	}

	f.wires = append(f.wires, wire)
	f.updatedAt = time.Now()
	return nil
}

// RemoveWire 는 ID로 와이어를 제거한다.
// 와이어를 찾을 수 없으면 ErrWireNotFound를 반환한다.
func (f *defaultFlow) RemoveWire(id string) error {
	for i, w := range f.wires {
		if w.ID == id {
			f.wires = append(f.wires[:i], f.wires[i+1:]...)
			f.updatedAt = time.Now()
			return nil
		}
	}
	return ErrWireNotFound
}

// SetDescription 은 Flow의 설명을 변경한다.
func (f *defaultFlow) SetDescription(desc string) {
	f.description = desc
	f.updatedAt = time.Now()
}

// SetConfig 는 Flow의 설정을 변경한다.
func (f *defaultFlow) SetConfig(config FlowConfig) {
	f.config = config
	f.updatedAt = time.Now()
}

// SetMetadata 는 메타데이터에 키-값 쌍을 설정한다.
func (f *defaultFlow) SetMetadata(key, value string) {
	if f.metadata == nil {
		f.metadata = make(map[string]string)
	}
	f.metadata[key] = value
	f.updatedAt = time.Now()
}

// RemoveMetadata 는 지정된 키의 메타데이터를 제거한다.
func (f *defaultFlow) RemoveMetadata(key string) {
	delete(f.metadata, key)
	f.updatedAt = time.Now()
}

// SetState 는 Flow의 상태를 변경한다.
// IsValidTransition을 사용하여 전이 유효성을 검증한다.
// 유효하지 않은 전이이면 ErrInvalidStateTransition을 반환한다.
func (f *defaultFlow) SetState(state FlowState) error {
	if !IsValidTransition(f.state, state) {
		return ErrInvalidStateTransition
	}
	f.state = state
	f.updatedAt = time.Now()
	return nil
}

// ---------------------------------------------------------------------------
// 비공개 헬퍼 메서드
// ---------------------------------------------------------------------------

// findNodeByID 는 ID로 노드를 찾는 비공개 헬퍼 메서드이다.
func (f *defaultFlow) findNodeByID(id string) (NodeDef, bool) {
	for _, n := range f.nodes {
		if n.ID == id {
			return n, true
		}
	}
	return NodeDef{}, false
}

// hasOutputPort 는 노드의 Outputs 또는 ErrorPort에서 지정된 이름의 포트가 존재하는지 확인한다.
func (f *defaultFlow) hasOutputPort(node NodeDef, portName string) bool {
	for _, p := range node.Outputs {
		if p.Name == portName {
			return true
		}
	}
	if node.ErrorPort != nil && node.ErrorPort.Name == portName {
		return true
	}
	return false
}

// hasInputPort 는 노드의 Inputs에서 지정된 이름의 포트가 존재하는지 확인한다.
func (f *defaultFlow) hasInputPort(node NodeDef, portName string) bool {
	for _, p := range node.Inputs {
		if p.Name == portName {
			return true
		}
	}
	return false
}
