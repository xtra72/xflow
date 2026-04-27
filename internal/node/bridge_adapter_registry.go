package node

import (
	"sync"

	"github.com/xtra/xflow/pkg/message"
)

// AdapterRegistry 는 에이전트 타입별 BridgeAdapter 인스턴스를 관리하는 레지스트리이다.
// 스레드 안전하며, 전역 레지스트리와 인스턴스별 레지스트리를 모두 지원한다.
type AdapterRegistry struct {
	mu       sync.RWMutex
	adapters map[string]BridgeAdapter
}

// NewAdapterRegistry 는 새로운 AdapterRegistry 인스턴스를 반환한다.
func NewAdapterRegistry() *AdapterRegistry {
	return &AdapterRegistry{
		adapters: make(map[string]BridgeAdapter),
	}
}

// globalAdapterRegistry 는 패키지 수준의 전역 어댑터 레지스트리이다.
var globalAdapterRegistry = &AdapterRegistry{
	adapters: make(map[string]BridgeAdapter),
}

// RegisterAdapter 는 전역 레지스트리에 에이전트 타입별 어댑터를 등록한다.
func RegisterAdapter(agentType string, adapter BridgeAdapter) {
	globalAdapterRegistry.RegisterAdapter(agentType, adapter)
}

// GetAdapter 는 전역 레지스트리에서 에이전트 타입에 해당하는 어댑터를 조회한다.
func GetAdapter(agentType string) (BridgeAdapter, bool) {
	return globalAdapterRegistry.GetAdapter(agentType)
}

// RegisterAdapter 는 에이전트 타입별 어댑터를 레지스트리에 등록한다.
// 이미 등록된 타입이 있으면 덮어쓴다.
func (r *AdapterRegistry) RegisterAdapter(agentType string, adapter BridgeAdapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[agentType] = adapter
}

// GetAdapter 는 에이전트 타입에 해당하는 어댑터를 반환한다.
// 등록되지 않은 타입이면 (nil, false)를 반환한다.
func (r *AdapterRegistry) GetAdapter(agentType string) (BridgeAdapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	adapter, ok := r.adapters[agentType]
	return adapter, ok
}

// DefaultAdapter 는 기존 DefaultTransformer를 래핑하여 BridgeAdapter 인터페이스를 구현하는 기본 어댑터이다.
// 프로토콜별 메타데이터 변환 없이 기존 동작을 유지한다.
type DefaultAdapter struct {
	transformer *DefaultTransformer
}

// NewDefaultAdapter 는 새로운 DefaultAdapter 인스턴스를 반환한다.
func NewDefaultAdapter() *DefaultAdapter {
	return &DefaultAdapter{
		transformer: NewDefaultTransformer(),
	}
}

// Validate 는 항상 nil을 반환한다 (프로토콜별 검증 없음).
func (a *DefaultAdapter) Validate(_ BridgeConfig) error {
	return nil
}

// DefaultConfig 는 빈 BridgeConfig를 반환한다.
func (a *DefaultAdapter) DefaultConfig() BridgeConfig {
	return BridgeConfig{}
}

// TransformToFlow 는 DefaultTransformer.AgentToFlow에 위임하여 변환한다.
// AgentMeta는 무시된다.
func (a *DefaultAdapter) TransformToFlow(data []byte, _ AgentMeta) (message.Message, error) {
	return a.transformer.AgentToFlow(data)
}

// TransformToAgent 는 DefaultTransformer.FlowToAgent에 위임하여 변환한다.
// 빈 AgentMeta를 반환한다.
func (a *DefaultAdapter) TransformToAgent(msg message.Message) ([]byte, AgentMeta, error) {
	data, err := a.transformer.FlowToAgent(msg)
	return data, AgentMeta{}, err
}

// HandleControl 은 nil을 반환한다 (기본 어댑터는 제어 메시지 처리 없음).
func (a *DefaultAdapter) HandleControl(_ message.Message) error {
	return nil
}

// AdapterTransformerBridge 는 BridgeAdapter를 BridgeTransformer 인터페이스로 래핑한다.
// BridgeNode가 기존 transformer 필드를 통해 어댑터를 사용할 수 있도록 한다.
type AdapterTransformerBridge struct {
	adapter BridgeAdapter
}

// NewAdapterTransformerBridge 는 주어진 BridgeAdapter를 래핑하는 새로운 AdapterTransformerBridge를 반환한다.
func NewAdapterTransformerBridge(adapter BridgeAdapter) *AdapterTransformerBridge {
	return &AdapterTransformerBridge{adapter: adapter}
}

// AgentToFlow 는 어댑터의 TransformToFlow에 위임한다.
// 빈 AgentMeta로 호출한다.
func (b *AdapterTransformerBridge) AgentToFlow(data []byte) (message.Message, error) {
	return b.adapter.TransformToFlow(data, AgentMeta{})
}

// FlowToAgent 는 어댑터의 TransformToAgent에 위임한다.
// 반환된 AgentMeta는 무시한다.
func (b *AdapterTransformerBridge) FlowToAgent(msg message.Message) ([]byte, error) {
	data, _, err := b.adapter.TransformToAgent(msg)
	return data, err
}
