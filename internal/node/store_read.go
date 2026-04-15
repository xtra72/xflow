package node

import (
	"context"
	"fmt"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// StoreReadNode 는 키-값 저장소에서 데이터를 조회하는 노드이다.
// 메시지의 payload에서 key_template을 해석하여 키를 생성하고,
// Store에서 값을 조회하여 메시지 payload의 output_key에 추가한 뒤
// 메시지를 다음 노드로 전달한다.
//
// Store 인스턴스는 agent_ref로 지정된 Store 에이전트에서 가져온다.
// Init 시 AgentResolver를 통해 에이전트를 찾고, storeProvider 인터페이스로
// Store 인스턴스에 접근한다.
// HistoryReader 는 히스토리 조회 기능을 제공하는 인터페이스이다.
type HistoryReader interface {
	GetHistory(ctx context.Context, key string) ([]any, error)
}

// MetadataReader 는 Store 엔트리의 메타데이터를 조회하는 인터페이스이다.
// GetMetadata 는 count, created_at, updated_at, oldest_at 등의 메타데이터를
// map[string]any 형태로 반환한다.
type MetadataReader interface {
	GetMetadata(ctx context.Context, key string) (map[string]any, error)
}

type StoreReadNode struct {
	*BaseNode
	store          StoreReader
	resolver       AgentResolver    // AgentResolver (생성 시 옵션에서 추출)
	agentRef       *flow.AgentRef   // Store 에이전트 참조
	keyTemplate    string           // 키 템플릿 (예: "{device}:{metric}")
	namespace      string           // Store 네임스페이스
	outputKey       string           // 조회된 값을 저장할 payload 키 (기본값: "store_value")
	includeHistory  bool             // 히스토리를 함께 조회할지 여부 (기본값: false)
	includeMetadata bool             // 메타데이터를 함께 조회할지 여부 (기본값: false)
}

// NewStoreReadNode 는 새로운 StoreReadNode를 생성하는 팩토리 함수이다.
func NewStoreReadNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &StoreReadNode{
		BaseNode:  base,
		namespace: "default",
		outputKey: "store_value",
		agentRef:  def.AgentRef,
	}
	// WithAgentResolver 옵션으로 주입된 resolver를 필드에 저장
	if r, ok := base.config["_agent_resolver"]; ok {
		if resolver, ok := r.(AgentResolver); ok {
			n.resolver = resolver
		}
	}
	return n, nil
}

// Init 은 StoreReadNode를 초기화하고 AgentResolver로 Store 에이전트를 해석한다.
func (n *StoreReadNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	// AgentResolver를 통해 Store 에이전트 해석
	if err := n.resolveStore(ctx); err != nil {
		return fmt.Errorf("store-read init: %w", err)
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// resolveStore 는 AgentResolver를 사용하여 Store 에이전트를 찾고 StoreReader를 추출한다.
func (n *StoreReadNode) resolveStore(ctx context.Context) error {
	// config["_store"]로 직접 주입된 경우 (테스트용 하위 호환성)
	if s, ok := n.config["_store"]; ok {
		if reader, ok := s.(StoreReader); ok {
			n.store = reader
			return nil
		}
	}

	// AgentRef가 없으면 store 미설정 상태로 진행 (Process에서 ErrStoreNotConfigured 반환)
	if n.agentRef == nil {
		return nil
	}

	// resolver 확인
	if n.resolver == nil {
		return fmt.Errorf("agent resolver not configured")
	}

	// 에이전트 해석
	transport, err := n.resolver.ResolveAgent(ctx, *n.agentRef)
	if err != nil {
		return fmt.Errorf("failed to resolve store agent %q: %w", n.agentRef.AgentName, err)
	}

	// AgentAccessor로 원본 에이전트 추출
	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return fmt.Errorf("store agent transport does not support AgentAccessor")
	}

	// storeProvider 인터페이스로 네임스페이스별 Store 인스턴스 추출
	provider, ok := accessor.UnderlyingAgent().(storeProvider)
	if !ok {
		return fmt.Errorf("agent %q does not implement storeProvider", n.agentRef.AgentName)
	}

	instance := provider.NodeStoreForNamespace(n.namespace)
	if instance == nil {
		return fmt.Errorf("store agent %q returned nil store for namespace %q", n.agentRef.AgentName, n.namespace)
	}

	// NodeStoreAdapter → StoreReader 타입 단언
	reader, ok := instance.(StoreReader)
	if !ok {
		return fmt.Errorf("store agent %q returned incompatible type for StoreReader", n.agentRef.AgentName)
	}
	n.store = reader

	return nil
}

// Shutdown 은 StoreReadNode를 종료한다.
func (n *StoreReadNode) Shutdown(ctx context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 StoreReadNode의 설정을 적용한다.
//
// 지원하는 설정 키:
//   - "key_template": string - 키 템플릿 ({field} 형식 플레이스홀더)
//   - "namespace": string - Store 네임스페이스 (기본값: "default")
//   - "output_key": string - 조회된 값을 저장할 payload 키 (기본값: "store_value")
//   - "include_history": bool - 히스토리를 함께 조회할지 여부 (기본값: false)
//   - "include_metadata": bool - 메타데이터를 함께 조회할지 여부 (기본값: false)
func (n *StoreReadNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	if v, ok := config["key_template"]; ok {
		if s, ok := v.(string); ok {
			n.keyTemplate = s
		}
	}

	if v, ok := config["namespace"]; ok {
		if s, ok := v.(string); ok {
			n.namespace = s
		}
	}

	if v, ok := config["output_key"]; ok {
		if s, ok := v.(string); ok {
			n.outputKey = s
		}
	}

	if v, ok := config["include_history"]; ok {
		if b, ok := v.(bool); ok {
			n.includeHistory = b
		}
	}

	if v, ok := config["include_metadata"]; ok {
		if b, ok := v.(bool); ok {
			n.includeMetadata = b
		}
	}

	return nil
}

// Process 는 Store에서 값을 조회하여 메시지 payload에 추가하고 반환한다.
// 키가 존재하지 않으면 값을 추가하지 않고 원본 메시지를 그대로 반환한다.
// include_history가 true이고 store가 HistoryReader를 구현하면 히스토리도 함께 조회한다.
// include_metadata가 true이고 store가 MetadataReader를 구현하면 메타데이터도 함께 조회한다.
func (n *StoreReadNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	if n.store == nil {
		return nil, ErrStoreNotConfigured
	}

	// 키 해석
	key, err := resolveKeyTemplate(n.keyTemplate, msg.Payload())
	if err != nil {
		return nil, fmt.Errorf("store-read: %w", err)
	}

	// Store에서 조회
	value, found, err := n.store.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("store-read: %w", err)
	}

	// 키가 존재하면 payload에 추가
	if found {
		msg.Payload().Set(n.outputKey, value)

		// include_history가 true이고 store가 HistoryReader를 구현하면 히스토리도 추가
		if n.includeHistory {
			if hr, ok := n.store.(HistoryReader); ok {
				history, histErr := hr.GetHistory(ctx, key)
				if histErr == nil {
					msg.Payload().Set("history", history)
				}
			}
		}

		// include_metadata가 true이고 store가 MetadataReader를 구현하면 메타데이터도 추가
		if n.includeMetadata {
			if mr, ok := n.store.(MetadataReader); ok {
				meta, metaErr := mr.GetMetadata(ctx, key)
				if metaErr == nil {
					for k, v := range meta {
						msg.Payload().Set(k, v)
					}
				}
			}
		}
	}

	// pass-through: 메시지를 그대로 반환
	return []message.Message{msg}, nil
}
