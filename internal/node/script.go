package node

import (
	"context"
	"sync"
	"time"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ScriptEngine 은 스크립트 실행 엔진 인터페이스이다.
// Lua 등의 실제 구현은 별도 SPEC(SPEC-SCRIPT-001)으로 이연된다.
type ScriptEngine interface {
	// Compile 은 스크립트 소스를 컴파일한다.
	Compile(source string) error
	// Execute 는 컴파일된 스크립트를 메시지에 대해 실행한다.
	Execute(ctx context.Context, msg message.Message) (message.Message, error)
	// Close 는 엔진 리소스를 해제한다.
	Close() error
}

// ScriptEngineFactory 는 노드별 ScriptEngine 인스턴스를 생성하는 팩토리이다.
// 단일 엔진을 모든 스크립트 노드에 공유하면 컴파일된 스크립트 ID 가 마지막
// 노드의 것으로 덮어쓰여 잘못된 스크립트가 실행될 수 있다. 따라서 노드별로
// 어댑터 인스턴스(자체 scriptID 보관)를 생성해야 한다.
//
// nodeID 는 디버깅 / 추적 용도로 전달된다.
type ScriptEngineFactory func(nodeID string) ScriptEngine

// ScriptNode 는 스크립트 기반으로 메시지를 처리하는 노드이다.
// ScriptEngine 인터페이스를 통해 다양한 스크립트 언어를 지원할 수 있다.
type ScriptNode struct {
	*BaseNode
	engine        ScriptEngine
	scriptSource  string
	scriptTimeout time.Duration
	mu            sync.RWMutex
}

// WithScriptEngine 은 ScriptNode에 ScriptEngine을 설정하는 옵션을 반환한다.
// 모든 스크립트 노드가 동일한 엔진 인스턴스를 공유하므로, 노드별 상태(컴파일된
// scriptID)를 가지는 엔진을 사용하면 안 된다. 노드별 인스턴스가 필요하면
// WithScriptEngineFactory 를 사용한다.
func WithScriptEngine(engine ScriptEngine) NodeOption {
	return func(b *BaseNode) {
		if b.config == nil {
			b.config = make(map[string]any)
		}
		b.config["_script_engine"] = engine
	}
}

// WithScriptEngineFactory 는 노드별로 ScriptEngine 인스턴스를 생성하는 팩토리
// 옵션이다. 동일한 옵션이 모든 스크립트 노드에 적용되더라도 NewScriptNode
// 가 def.ID 로 팩토리를 호출하여 노드별 어댑터 인스턴스를 생성한다.
//
// WithScriptEngine 과 동시에 사용 시 팩토리가 우선한다.
func WithScriptEngineFactory(factory ScriptEngineFactory) NodeOption {
	return func(b *BaseNode) {
		if b.config == nil {
			b.config = make(map[string]any)
		}
		b.config["_script_engine_factory"] = factory
	}
}

// WithScriptTimeout 은 ScriptNode의 스크립트 실행 타임아웃을 설정하는 옵션을 반환한다.
func WithScriptTimeout(d time.Duration) NodeOption {
	return func(b *BaseNode) {
		if b.config == nil {
			b.config = make(map[string]any)
		}
		b.config["_script_timeout"] = d
	}
}

// NewScriptNode 는 새로운 ScriptNode를 생성하는 팩토리 함수이다.
func NewScriptNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &ScriptNode{
		BaseNode:      base,
		scriptTimeout: 5 * time.Second,
	}

	// 옵션에서 engine과 timeout 추출
	if base.config != nil {
		// 팩토리가 있으면 노드별 인스턴스 생성을 우선한다.
		if f, ok := base.config["_script_engine_factory"]; ok {
			if factory, ok := f.(ScriptEngineFactory); ok && factory != nil {
				n.engine = factory(def.ID)
			}
		}
		// 팩토리가 없거나 nil 을 반환하면 공유 엔진을 사용한다.
		if n.engine == nil {
			if e, ok := base.config["_script_engine"]; ok {
				if engine, ok := e.(ScriptEngine); ok {
					n.engine = engine
				}
			}
		}
		if t, ok := base.config["_script_timeout"]; ok {
			if timeout, ok := t.(time.Duration); ok {
				n.scriptTimeout = timeout
			}
		}
		if s, ok := base.config["script"]; ok {
			if src, ok := s.(string); ok {
				n.scriptSource = src
			}
		}
	}

	return n, nil
}

// Init 은 ScriptNode를 초기화한다.
// 엔진이 설정되어 있으면 스크립트 소스를 컴파일한다.
func (n *ScriptNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	n.mu.RLock()
	engine := n.engine
	source := n.scriptSource
	n.mu.RUnlock()

	if engine != nil && source != "" {
		if err := engine.Compile(source); err != nil {
			return &NodeError{
				NodeID:   n.ID(),
				NodeType: n.Type(),
				Err:      ErrScriptCompileFailed,
			}
		}
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 스크립트를 실행하여 메시지를 처리한다.
// 엔진이 nil이면 메시지를 그대로 통과시킨다.
// 타임아웃 초과 시 ErrScriptTimeout, 실행 에러 시 ErrScriptExecutionFailed를 반환한다.
func (n *ScriptNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	engine := n.engine
	timeout := n.scriptTimeout
	n.mu.RUnlock()

	if engine == nil {
		return []message.Message{msg}, nil
	}

	// 타임아웃 컨텍스트 생성
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := engine.Execute(timeoutCtx, msg)
	if err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return nil, ErrScriptTimeout
		}
		return nil, ErrScriptExecutionFailed
	}
	return []message.Message{result}, nil
}

// Shutdown 은 ScriptNode를 종료하고 엔진 리소스를 해제한다.
func (n *ScriptNode) Shutdown(ctx context.Context) error {
	n.mu.RLock()
	engine := n.engine
	n.mu.RUnlock()

	if engine != nil {
		_ = engine.Close()
	}
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 ScriptNode의 설정을 적용한다.
// config에 "script" 키가 있으면 스크립트 소스를 업데이트하고 재컴파일한다.
func (n *ScriptNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	if src, ok := config["script"]; ok {
		if source, ok := src.(string); ok {
			n.mu.Lock()
			n.scriptSource = source
			engine := n.engine
			n.mu.Unlock()

			if engine != nil {
				if err := engine.Compile(source); err != nil {
					return ErrScriptCompileFailed
				}
			}
		}
	}
	return nil
}
