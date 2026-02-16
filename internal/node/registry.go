package node

import (
	"sort"
	"sync"

	"github.com/xtra/xflow/pkg/flow"
)

// NodeFactory 는 NodeDef로부터 Node를 생성하는 팩토리 함수 타입이다.
type NodeFactory func(def flow.NodeDef, opts ...NodeOption) (Node, error)

// RegistryOption 은 Registry 생성 시 적용할 수 있는 옵션 함수 타입이다.
type RegistryOption func(*Registry)

// WithoutBuiltins 는 빌트인 노드 타입을 등록하지 않는 옵션을 반환한다.
// 테스트나 커스텀 레지스트리를 만들 때 유용하다.
func WithoutBuiltins() RegistryOption {
	return func(r *Registry) {
		r.skipBuiltins = true
	}
}

// Registry 는 노드 타입별 팩토리를 관리하는 레지스트리이다.
// 빌트인 노드 타입(filter, transform, switch, bridge, script, catch)을 자동 등록한다.
type Registry struct {
	mu           sync.RWMutex
	factories    map[string]NodeFactory
	skipBuiltins bool
}

// NewRegistry 는 새로운 Registry를 생성한다.
// WithoutBuiltins 옵션이 없으면 6개의 빌트인 노드 타입이 자동 등록된다.
func NewRegistry(opts ...RegistryOption) *Registry {
	r := &Registry{
		factories: make(map[string]NodeFactory),
	}

	for _, opt := range opts {
		opt(r)
	}

	if !r.skipBuiltins {
		r.registerBuiltins()
	}

	return r
}

// registerBuiltins 는 빌트인 노드 팩토리를 등록한다.
func (r *Registry) registerBuiltins() {
	r.factories["filter"] = NewFilterNode
	r.factories["transform"] = NewTransformNode
	r.factories["switch"] = NewSwitchNode
	r.factories["bridge"] = NewBridgeNode
	r.factories["script"] = NewScriptNode
	r.factories["catch"] = NewCatchNode
}

// Register 는 새로운 노드 타입과 팩토리를 등록한다.
// 이미 등록된 타입이면 ErrNodeTypeAlreadyRegistered를 반환한다.
func (r *Registry) Register(typeName string, factory NodeFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[typeName]; exists {
		return ErrNodeTypeAlreadyRegistered
	}
	r.factories[typeName] = factory
	return nil
}

// Create 는 NodeDef의 Type에 해당하는 팩토리를 찾아 노드를 생성한다.
// 등록되지 않은 타입이면 ErrNodeTypeNotFound를 반환한다.
func (r *Registry) Create(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	r.mu.RLock()
	factory, exists := r.factories[def.Type]
	r.mu.RUnlock()

	if !exists {
		return nil, ErrNodeTypeNotFound
	}
	return factory(def, opts...)
}

// Types 는 등록된 모든 노드 타입의 정렬된 목록을 반환한다.
func (r *Registry) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.factories))
	for t := range r.factories {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

// Has 는 지정된 타입이 등록되어 있는지 확인한다.
func (r *Registry) Has(typeName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.factories[typeName]
	return exists
}
