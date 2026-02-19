package node

import (
	"context"
	"sync"

	"github.com/xtra/xflow/internal/observe"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// Node 는 Engine이 소비하는 노드 계약이다.
// 모든 구체 노드 타입(FilterNode, TransformNode 등)이 이 인터페이스를 구현한다.
type Node interface {
	// ID 는 노드의 고유 식별자를 반환한다.
	ID() string
	// Name 은 노드의 이름을 반환한다.
	Name() string
	// Type 은 노드의 타입을 반환한다.
	Type() string
	// Init 은 노드를 초기화한다.
	Init(ctx context.Context) error
	// Process 는 메시지를 처리하고 결과 메시지 목록을 반환한다.
	Process(ctx context.Context, msg message.Message) ([]message.Message, error)
	// Shutdown 은 노드를 종료한다.
	Shutdown(ctx context.Context) error
	// Configure 는 노드 설정을 적용한다.
	Configure(config map[string]any) error
	// Ports 는 노드의 모든 포트 목록을 반환한다.
	Ports() []NodePort
}

// SourceNode 는 자체적으로 메시지를 생성하는 노드의 선택적 인터페이스이다.
// BridgeIn 모드의 BridgeNode처럼 외부 소스에서 메시지를 수신하는 노드가 구현한다.
// 엔진은 입력 와이어가 없는 노드에 대해 이 인터페이스를 확인하고,
// SourceCh에서 메시지를 읽어 출력 와이어로 전달한다.
type SourceNode interface {
	SourceCh() <-chan message.Message
}

// NodePort 는 런타임 포트 정보를 나타내는 구조체이다.
// pkg/flow.Port(정적 정의)와 구분되며, 연결 상태(Connected) 필드를 추가로 포함한다.
type NodePort struct {
	ID        string             // 포트 고유 식별자
	Name      string             // 포트 이름
	Direction flow.PortDirection // 포트 방향 (input, output, error)
	Connected bool               // 연결 상태
}

// NodeOption 은 BaseNode 생성 시 적용할 수 있는 옵션 함수 타입이다.
type NodeOption func(*BaseNode)

// WithLogger 는 BaseNode에 ComponentLogger를 설정하는 옵션을 반환한다.
func WithLogger(logger observe.ComponentLogger) NodeOption {
	return func(b *BaseNode) {
		b.logger = logger
	}
}

// WithMetrics 는 BaseNode에 MetricsCollector를 설정하는 옵션을 반환한다.
func WithMetrics(metrics observe.MetricsCollector) NodeOption {
	return func(b *BaseNode) {
		b.metrics = metrics
	}
}

// BaseNode 는 모든 노드의 기반 구조체이다.
// BaseLifecycle을 포인터로 임베딩하여 공통 상태 관리를 재사용하고,
// 포트 관리, 설정 관리, 로깅/메트릭 기능을 제공한다.
type BaseNode struct {
	*lifecycle.BaseLifecycle // 생명주기 상태 관리 임베딩

	id       string // 노드 고유 식별자
	name     string // 노드 이름
	nodeType string // 노드 타입

	config map[string]any // 노드 설정
	mu     sync.RWMutex   // 설정 및 포트 접근 보호

	inputs    map[string]*NodePort // 입력 포트 맵 (이름 -> 포트)
	outputs   map[string]*NodePort // 출력 포트 맵 (이름 -> 포트)
	errorPort *NodePort            // 에러 포트

	logger  observe.ComponentLogger  // 로거 (nil 가능)
	metrics observe.MetricsCollector // 메트릭 수집기 (nil 가능)
}

// NewBaseNode 는 flow.NodeDef와 옵션을 기반으로 새로운 BaseNode를 생성한다.
// NodeDef의 입력/출력 포트를 NodePort로 변환하고,
// ErrorPort가 nil이면 기본 "_error" 포트를 생성한다.
func NewBaseNode(def flow.NodeDef, opts ...NodeOption) *BaseNode {
	b := &BaseNode{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("node")),
		id:            def.ID,
		name:          def.Name,
		nodeType:      def.Type,
		inputs:        make(map[string]*NodePort),
		outputs:       make(map[string]*NodePort),
	}

	// 입력 포트 초기화
	for _, p := range def.Inputs {
		b.inputs[p.Name] = &NodePort{
			ID:        p.ID,
			Name:      p.Name,
			Direction: p.Direction,
		}
	}

	// 출력 포트 초기화
	for _, p := range def.Outputs {
		b.outputs[p.Name] = &NodePort{
			ID:        p.ID,
			Name:      p.Name,
			Direction: p.Direction,
		}
	}

	// 에러 포트 초기화
	if def.ErrorPort != nil {
		b.errorPort = &NodePort{
			ID:        def.ErrorPort.ID,
			Name:      def.ErrorPort.Name,
			Direction: def.ErrorPort.Direction,
		}
	} else {
		// 기본 에러 포트 생성
		b.errorPort = &NodePort{
			ID:        "_error",
			Name:      "_error",
			Direction: flow.PortError,
		}
	}

	// 옵션 적용
	for _, opt := range opts {
		opt(b)
	}

	return b
}

// ID 는 노드의 고유 식별자를 반환한다.
func (b *BaseNode) ID() string {
	return b.id
}

// Name 은 노드의 이름을 반환한다.
func (b *BaseNode) Name() string {
	return b.name
}

// Type 은 노드의 타입을 반환한다.
func (b *BaseNode) Type() string {
	return b.nodeType
}

// Configure 는 노드 설정을 적용한다. nil config는 에러를 반환한다.
func (b *BaseNode) Configure(config map[string]any) error {
	if config == nil {
		return ErrInvalidConfig
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.config = make(map[string]any, len(config))
	for k, v := range config {
		b.config[k] = v
	}
	return nil
}

// GetConfig 는 현재 설정의 복사본을 반환한다.
func (b *BaseNode) GetConfig() map[string]any {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.config == nil {
		return nil
	}
	cp := make(map[string]any, len(b.config))
	for k, v := range b.config {
		cp[k] = v
	}
	return cp
}

// Ports 는 모든 포트(입력 + 출력 + 에러)의 목록을 반환한다.
func (b *BaseNode) Ports() []NodePort {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var ports []NodePort
	for _, p := range b.inputs {
		ports = append(ports, *p)
	}
	for _, p := range b.outputs {
		ports = append(ports, *p)
	}
	if b.errorPort != nil {
		ports = append(ports, *b.errorPort)
	}
	return ports
}

// GetPort 는 이름으로 포트를 조회한다. 입력, 출력, 에러 포트를 모두 검색한다.
func (b *BaseNode) GetPort(name string) (*NodePort, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if p, ok := b.inputs[name]; ok {
		return p, true
	}
	if p, ok := b.outputs[name]; ok {
		return p, true
	}
	if b.errorPort != nil && b.errorPort.Name == name {
		return b.errorPort, true
	}
	return nil, false
}

// GetOutputPort 는 이름으로 출력 포트를 조회한다.
func (b *BaseNode) GetOutputPort(name string) (*NodePort, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	p, ok := b.outputs[name]
	return p, ok
}

// GetErrorPort 는 에러 포트를 반환한다.
func (b *BaseNode) GetErrorPort() *NodePort {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.errorPort
}

// Logger 는 설정된 ComponentLogger를 반환한다. 설정되지 않았으면 nil이다.
func (b *BaseNode) Logger() observe.ComponentLogger {
	return b.logger
}

// MetricsCollector 는 설정된 MetricsCollector를 반환한다. 설정되지 않았으면 nil이다.
func (b *BaseNode) MetricsCollector() observe.MetricsCollector {
	return b.metrics
}
