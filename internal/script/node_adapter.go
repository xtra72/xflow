// node_adapter.go (v0.18.23) 는 DefaultScriptEngine 을 node.ScriptEngine
// 인터페이스로 감싸는 어댑터를 제공한다.
//
// 두 인터페이스의 차이:
//   - node.ScriptEngine: Compile(source) / Execute(ctx, msg) / Close()
//     — 인스턴스 1개당 컴파일된 script 1개 가정 (per-node 사용).
//   - DefaultScriptEngine: Compile(ctx, ScriptSource) → scriptID,
//     Execute(ctx, scriptID, input) — 여러 script 를 관리 (scriptID 키).
//
// 어댑터는 per-node 인스턴스로 생성되며, 자체 scriptID 를 보관해 양 인터페이스
// 의 의미 차이를 흡수한다.

package script

import (
	"context"
	"fmt"
	"sync"

	"github.com/xtra/xflow/pkg/message"
)

// NodeEngineAdapter 는 DefaultScriptEngine 을 node.ScriptEngine 인터페이스
// 형태로 감싼다. 한 인스턴스가 한 node 의 스크립트 1개를 관리한다.
type NodeEngineAdapter struct {
	engine   *DefaultScriptEngine
	nodeName string

	mu       sync.RWMutex
	scriptID string
}

// NewNodeEngineAdapter 는 어댑터 인스턴스를 생성한다.
// engine 은 공유 인스턴스, nodeName 은 디버깅 / scriptID 생성용.
func NewNodeEngineAdapter(engine *DefaultScriptEngine, nodeName string) *NodeEngineAdapter {
	return &NodeEngineAdapter{
		engine:   engine,
		nodeName: nodeName,
	}
}

// Compile 은 source 를 컴파일하고 결과 scriptID 를 보관한다.
// 동일 어댑터에 이전 script 가 있으면 캐시에서 제거.
func (a *NodeEngineAdapter) Compile(source string) error {
	if a.engine == nil {
		return fmt.Errorf("script: engine not initialized")
	}
	if source == "" {
		return fmt.Errorf("script: empty source")
	}

	ctx := context.Background()
	sid, err := a.engine.Compile(ctx, ScriptSource{
		Type:    SourceInline,
		Content: source,
		Name:    a.nodeName,
	})
	if err != nil {
		return fmt.Errorf("script: compile %q: %w", a.nodeName, err)
	}

	a.mu.Lock()
	if a.scriptID != "" && a.scriptID != sid {
		_ = a.engine.Remove(a.scriptID)
	}
	a.scriptID = sid
	a.mu.Unlock()
	return nil
}

// Execute 는 컴파일된 script 를 msg 에 대해 실행하고 변환된 message 를 반환한다.
// script 가 nil 또는 nil 반환 시 입력 msg 그대로 통과.
// script 가 테이블 반환 시 payload/metadata 를 추출해 새 message 빌드.
func (a *NodeEngineAdapter) Execute(ctx context.Context, msg message.Message) (message.Message, error) {
	a.mu.RLock()
	sid := a.scriptID
	a.mu.RUnlock()

	if sid == "" {
		// Compile 전 — pass-through.
		return msg, nil
	}

	result, err := a.engine.Execute(ctx, sid, msg)
	if err != nil {
		return nil, fmt.Errorf("script: execute %q: %w", a.nodeName, err)
	}

	// nil 반환 → pass-through.
	if result == nil {
		return msg, nil
	}

	// Lua 테이블이면 FromLuaValue 가 map[string]any 로 변환했을 것.
	if m, ok := result.(map[string]any); ok {
		return mapToMessage(m, msg), nil
	}

	// 알 수 없는 타입 → pass-through (안전한 fallback).
	return msg, nil
}

// Close 는 캐시에서 script 를 제거한다.
func (a *NodeEngineAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.scriptID != "" && a.engine != nil {
		_ = a.engine.Remove(a.scriptID)
		a.scriptID = ""
	}
	return nil
}

// mapToMessage 는 Lua 테이블에서 추출된 map[string]any 를 message.Message
// 로 변환한다. payload / metadata 키에서 값을 추출.
//
// 누락된 키는 원본 msg 의 값을 보존 (부분 갱신 지원).
func mapToMessage(m map[string]any, original message.Message) message.Message {
	opts := []message.Option{}

	if rawPayload, ok := m["payload"]; ok {
		if payloadMap, ok := rawPayload.(map[string]any); ok {
			opts = append(opts, message.WithPayload(message.NewPayload(payloadMap)))
		}
	} else if original != nil {
		// payload 누락 시 원본 보존.
		opts = append(opts, message.WithPayload(original.Payload()))
	}

	if rawMeta, ok := m["metadata"]; ok {
		if metaMap, ok := rawMeta.(map[string]any); ok {
			for k, v := range metaMap {
				if s, ok := v.(string); ok {
					opts = append(opts, message.WithMetadata(k, s))
				} else {
					opts = append(opts, message.WithMetadata(k, fmt.Sprintf("%v", v)))
				}
			}
		}
	} else if original != nil {
		for k, v := range original.Metadata().All() {
			opts = append(opts, message.WithMetadata(k, v))
		}
	}

	return message.New(opts...)
}
