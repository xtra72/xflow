package system

import (
	"context"
	"fmt"
	"time"

	"github.com/xtra/xflow/pkg/message"
)

// BridgeHandler 는 Bridge Node를 통한 메시지 기반 Store 접근을 처리한다.
// 수신된 메시지의 메타데이터에서 연산 종류를 판별하고, StoreAgent에 위임한 뒤 응답 메시지를 반환한다.
type BridgeHandler struct {
	agent *StoreAgent
}

// NewBridgeHandler 는 StoreAgent를 사용하는 BridgeHandler를 생성한다.
func NewBridgeHandler(agent *StoreAgent) *BridgeHandler {
	return &BridgeHandler{agent: agent}
}

// HandleMessage 는 Bridge Node에서 수신한 메시지를 처리하고 응답 메시지를 반환한다.
//
// 메시지 메타데이터 필드:
//   - store.operation: "get", "set", "delete", "has", "keys", "clear"
//   - store.key: 대상 키
//   - store.ttl: TTL 기간 문자열 (선택, set 연산용)
//   - store.namespace: 네임스페이스 (선택, 기본값 "default")
//   - store.pattern: keys 연산용 패턴
//
// 응답 메시지:
//   - Metadata "store.status": "ok" 또는 "error"
//   - Metadata "store.error": 에러 메시지 (status가 "error"일 때)
//   - Payload: 연산 결과 (get→value, has→exists, keys→keys)
func (h *BridgeHandler) HandleMessage(ctx context.Context, msg message.Message) (message.Message, error) {
	operation, _ := msg.Metadata().Get("store.operation")
	key, _ := msg.Metadata().Get("store.key")
	namespace, _ := msg.Metadata().Get("store.namespace")

	if namespace == "" {
		namespace = "default"
	}

	// 네임스페이스에 해당하는 Store를 획득한다
	store := h.agent.ForNamespace(namespace)

	switch operation {
	case "get":
		return h.handleGet(ctx, store, key)
	case "set":
		return h.handleSet(ctx, store, msg, key)
	case "delete":
		return h.handleDelete(ctx, store, key)
	case "has":
		return h.handleHas(ctx, store, key)
	case "keys":
		return h.handleKeys(ctx, store, msg)
	case "clear":
		return h.handleClear(ctx, store)
	default:
		return h.errorResponse(fmt.Sprintf("지원하지 않는 연산: %s", operation)), nil
	}
}

// handleGet 은 Get 연산을 처리하고 응답 메시지를 반환한다.
func (h *BridgeHandler) handleGet(ctx context.Context, store Store, key string) (message.Message, error) {
	entry, err := store.Get(ctx, key)
	if err != nil {
		return h.errorResponse(err.Error()), nil
	}

	resp := message.New()
	resp.Metadata().Set("store.status", "ok")
	resp.Payload().Set("value", entry.Value)
	return resp, nil
}

// handleSet 은 Set 연산을 처리하고 응답 메시지를 반환한다.
// store.ttl 메타데이터가 있으면 SetWithTTL을, 없으면 Set을 호출한다.
func (h *BridgeHandler) handleSet(ctx context.Context, store Store, msg message.Message, key string) (message.Message, error) {
	value, _ := msg.Payload().Get("value")

	ttlStr, hasTTL := msg.Metadata().Get("store.ttl")

	if hasTTL && ttlStr != "" {
		ttl, err := time.ParseDuration(ttlStr)
		if err != nil {
			return h.errorResponse(fmt.Sprintf("유효하지 않은 TTL: %s", ttlStr)), nil
		}
		if err := store.SetWithTTL(ctx, key, value, ttl); err != nil {
			return h.errorResponse(err.Error()), nil
		}
	} else {
		if err := store.Set(ctx, key, value); err != nil {
			return h.errorResponse(err.Error()), nil
		}
	}

	resp := message.New()
	resp.Metadata().Set("store.status", "ok")
	return resp, nil
}

// handleDelete 는 Delete 연산을 처리하고 응답 메시지를 반환한다.
func (h *BridgeHandler) handleDelete(ctx context.Context, store Store, key string) (message.Message, error) {
	if err := store.Delete(ctx, key); err != nil {
		return h.errorResponse(err.Error()), nil
	}

	resp := message.New()
	resp.Metadata().Set("store.status", "ok")
	return resp, nil
}

// handleHas 는 Has 연산을 처리하고 응답 메시지를 반환한다.
func (h *BridgeHandler) handleHas(ctx context.Context, store Store, key string) (message.Message, error) {
	exists, err := store.Has(ctx, key)
	if err != nil {
		return h.errorResponse(err.Error()), nil
	}

	resp := message.New()
	resp.Metadata().Set("store.status", "ok")
	resp.Payload().Set("exists", exists)
	return resp, nil
}

// handleKeys 는 Keys 연산을 처리하고 응답 메시지를 반환한다.
func (h *BridgeHandler) handleKeys(ctx context.Context, store Store, msg message.Message) (message.Message, error) {
	pattern, _ := msg.Metadata().Get("store.pattern")
	if pattern == "" {
		pattern = "*"
	}

	keys, err := store.Keys(ctx, pattern)
	if err != nil {
		return h.errorResponse(err.Error()), nil
	}

	// nil 키는 빈 슬라이스로 변환
	if keys == nil {
		keys = []string{}
	}

	resp := message.New()
	resp.Metadata().Set("store.status", "ok")
	resp.Payload().Set("keys", keys)
	return resp, nil
}

// handleClear 는 Clear 연산을 처리하고 응답 메시지를 반환한다.
func (h *BridgeHandler) handleClear(ctx context.Context, store Store) (message.Message, error) {
	if err := store.Clear(ctx); err != nil {
		return h.errorResponse(err.Error()), nil
	}

	resp := message.New()
	resp.Metadata().Set("store.status", "ok")
	return resp, nil
}

// errorResponse 는 에러 상태의 응답 메시지를 생성한다.
func (h *BridgeHandler) errorResponse(errMsg string) message.Message {
	resp := message.New()
	resp.Metadata().Set("store.status", "error")
	resp.Metadata().Set("store.error", errMsg)
	return resp
}
