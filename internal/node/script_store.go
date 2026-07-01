package node

import (
	"context"
	"fmt"

	"github.com/xtra/xflow/internal/script"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// script_store.go (message-slim-metadata / Follow-up A) 는 스크립트 노드가
// 네임스페이스 스코프 스토어를 해석하여, 실행별로 xflow.store 에 바인딩할 수 있도록
// 하는 헬퍼를 제공한다.
//
// 해석 경로는 store-read/store-write 노드와 동일하다:
//   AgentResolver.ResolveAgent(agent_ref) → AgentAccessor → storeProvider →
//   NodeStoreForNamespace(namespace) → *system.NodeStoreAdapter.
//
// 그 결과 어댑터를 script.StoreAccessor 로 감싸(get/set/delete/has), 스크립트
// 엔진이 이번 실행에 한해 xflow.store 를 이 네임스페이스 스토어에 연결한다.

// scriptEngineWithStore 는 실행별 스토어 바인딩을 지원하는 스크립트 엔진의 선택적
// 인터페이스이다. script.NodeEngineAdapter 가 구조적으로 이를 만족한다(같은
// script.StoreAccessor 타입을 사용하므로 구조적 만족이 성립).
type scriptEngineWithStore interface {
	ExecuteWithStore(ctx context.Context, msg message.Message, store script.StoreAccessor) (message.Message, error)
}

// nodeStoreScriptAccessor 는 node 계층의 네임스페이스 스토어(*system.NodeStoreAdapter
// 를 만족하는 StoreReader+StoreWriter)를 script.StoreAccessor 로 어댑트한다.
//
// 시그니처 차이 해소:
//   - StoreReader.Get 은 (any, bool, error) → script.StoreAccessor.Get 은 (any, error).
//     키 없음(found=false)은 (nil, nil) 로 매핑한다.
//   - Delete 는 StoreDeleter(있는 경우)로 위임한다.
type nodeStoreScriptAccessor struct {
	reader  StoreReader
	writer  StoreWriter
	deleter StoreDeleter
}

// StoreDeleter 는 키 삭제를 지원하는 선택적 인터페이스이다.
// *system.NodeStoreAdapter 가 이를 구현한다(Delete 추가됨).
type StoreDeleter interface {
	Delete(ctx context.Context, key string) error
}

// Get 은 script.StoreAccessor.Get 을 구현한다. 키가 없으면 (nil, nil).
func (a *nodeStoreScriptAccessor) Get(ctx context.Context, key string) (any, error) {
	if a.reader == nil {
		return nil, nil
	}
	v, found, err := a.reader.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return v, nil
}

// Set 은 script.StoreAccessor.Set 을 구현한다.
func (a *nodeStoreScriptAccessor) Set(ctx context.Context, key string, value any) error {
	if a.writer == nil {
		return fmt.Errorf("script store: writer not available")
	}
	return a.writer.Set(ctx, key, value)
}

// Delete 은 script.StoreAccessor.Delete 를 구현한다.
func (a *nodeStoreScriptAccessor) Delete(ctx context.Context, key string) error {
	if a.deleter == nil {
		return fmt.Errorf("script store: delete not supported")
	}
	return a.deleter.Delete(ctx, key)
}

// Has 는 script.StoreAccessor.Has 를 구현한다.
func (a *nodeStoreScriptAccessor) Has(ctx context.Context, key string) (bool, error) {
	if a.reader == nil {
		return false, nil
	}
	return a.reader.Has(ctx, key)
}

// resolveNamespacedScriptStore 는 agent_ref/namespace 로 네임스페이스 스토어를
// 해석하여 script.StoreAccessor 로 반환한다.
//
// agentRef 또는 resolver 가 없으면 (nil, nil) 을 반환한다 — 스토어 미구성 노드는
// 정상 시나리오이며, 이 경우 xflow.store 는 nil-safe 로 동작한다.
// 해석 도중 실제 오류(에이전트 없음/타입 불일치 등)는 error 로 반환한다.
func resolveNamespacedScriptStore(ctx context.Context, resolver AgentResolver, agentRef *flow.AgentRef, namespace string) (script.StoreAccessor, error) {
	if agentRef == nil || resolver == nil {
		// 스토어 미구성 — graceful (바인딩 없음).
		return nil, nil
	}
	if namespace == "" {
		namespace = "default"
	}

	transport, err := resolver.ResolveAgent(ctx, *agentRef)
	if err != nil {
		return nil, fmt.Errorf("script store: resolve agent %q: %w", agentRef.AgentName, err)
	}
	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return nil, fmt.Errorf("script store: agent %q transport does not support AgentAccessor", agentRef.AgentName)
	}
	provider, ok := accessor.UnderlyingAgent().(storeProvider)
	if !ok {
		return nil, fmt.Errorf("script store: agent %q does not implement storeProvider", agentRef.AgentName)
	}
	instance := provider.NodeStoreForNamespace(namespace)
	if instance == nil {
		return nil, fmt.Errorf("script store: agent %q returned nil store for namespace %q", agentRef.AgentName, namespace)
	}

	reader, _ := instance.(StoreReader)
	writer, _ := instance.(StoreWriter)
	deleter, _ := instance.(StoreDeleter)
	if reader == nil && writer == nil {
		return nil, fmt.Errorf("script store: agent %q store is neither reader nor writer", agentRef.AgentName)
	}

	return &nodeStoreScriptAccessor{reader: reader, writer: writer, deleter: deleter}, nil
}
