package system

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 테스트 헬퍼
// ---------------------------------------------------------------------------

// newTestBridgeHandler 는 초기화된 StoreAgent와 BridgeHandler를 생성한다.
func newTestBridgeHandler(t *testing.T) *BridgeHandler {
	t.Helper()
	agent := NewStoreAgent()
	err := agent.Init(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = agent.Stop(context.Background())
	})
	return NewBridgeHandler(agent)
}

// makeStoreMsg 는 Store 연산을 위한 메시지를 생성한다.
func makeStoreMsg(operation, key string, opts ...func(message.Message)) message.Message {
	msg := message.New()
	msg.Metadata().Set("store.operation", operation)
	if key != "" {
		msg.Metadata().Set("store.key", key)
	}
	for _, opt := range opts {
		opt(msg)
	}
	return msg
}

// withPayloadValue 는 메시지 페이로드에 "value" 키로 값을 설정하는 옵션이다.
func withPayloadValue(value any) func(message.Message) {
	return func(msg message.Message) {
		msg.Payload().Set("value", value)
	}
}

// withTTL 은 메시지 메타데이터에 store.ttl을 설정하는 옵션이다.
func withTTL(ttl string) func(message.Message) {
	return func(msg message.Message) {
		msg.Metadata().Set("store.ttl", ttl)
	}
}

// withNamespace 는 메시지 메타데이터에 store.namespace를 설정하는 옵션이다.
func withNamespace(ns string) func(message.Message) {
	return func(msg message.Message) {
		msg.Metadata().Set("store.namespace", ns)
	}
}

// withPattern 은 메시지 메타데이터에 store.pattern을 설정하는 옵션이다.
func withPattern(pattern string) func(message.Message) {
	return func(msg message.Message) {
		msg.Metadata().Set("store.pattern", pattern)
	}
}

// ---------------------------------------------------------------------------
// Get 연산 테스트
// ---------------------------------------------------------------------------

// TestBridgeHandler_GetExistingKey 는 존재하는 키에 대한 Get 연산을 검증한다.
func TestBridgeHandler_GetExistingKey(t *testing.T) {
	ctx := context.Background()
	handler := newTestBridgeHandler(t)

	// 먼저 값을 저장한다
	setMsg := makeStoreMsg("set", "greeting", withPayloadValue("hello"))
	_, err := handler.HandleMessage(ctx, setMsg)
	require.NoError(t, err)

	// Get 연산
	getMsg := makeStoreMsg("get", "greeting")
	resp, err := handler.HandleMessage(ctx, getMsg)
	require.NoError(t, err)

	// 응답 검증
	status, ok := resp.Metadata().Get("store.status")
	require.True(t, ok)
	assert.Equal(t, "ok", status)

	val, ok := resp.Payload().Get("value")
	require.True(t, ok)
	assert.Equal(t, "hello", val)
}

// TestBridgeHandler_GetMissingKey 는 존재하지 않는 키에 대한 Get 연산이 에러 상태를 반환하는지 검증한다.
func TestBridgeHandler_GetMissingKey(t *testing.T) {
	ctx := context.Background()
	handler := newTestBridgeHandler(t)

	getMsg := makeStoreMsg("get", "nonexistent")
	resp, err := handler.HandleMessage(ctx, getMsg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("store.status")
	require.True(t, ok)
	assert.Equal(t, "error", status)

	errMsg, ok := resp.Metadata().Get("store.error")
	require.True(t, ok)
	assert.Contains(t, errMsg, "not found")
}

// ---------------------------------------------------------------------------
// Set 연산 테스트
// ---------------------------------------------------------------------------

// TestBridgeHandler_SetOperation 은 Set 연산으로 값을 저장하는지 검증한다.
func TestBridgeHandler_SetOperation(t *testing.T) {
	ctx := context.Background()
	handler := newTestBridgeHandler(t)

	setMsg := makeStoreMsg("set", "key1", withPayloadValue("value1"))
	resp, err := handler.HandleMessage(ctx, setMsg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("store.status")
	require.True(t, ok)
	assert.Equal(t, "ok", status)

	// 저장된 값 확인
	getMsg := makeStoreMsg("get", "key1")
	resp, err = handler.HandleMessage(ctx, getMsg)
	require.NoError(t, err)

	val, ok := resp.Payload().Get("value")
	require.True(t, ok)
	assert.Equal(t, "value1", val)
}

// ---------------------------------------------------------------------------
// Set with TTL 연산 테스트
// ---------------------------------------------------------------------------

// TestBridgeHandler_SetWithTTL 은 TTL이 포함된 Set 연산을 검증한다.
func TestBridgeHandler_SetWithTTL(t *testing.T) {
	ctx := context.Background()
	handler := newTestBridgeHandler(t)

	setMsg := makeStoreMsg("set", "ttl-key",
		withPayloadValue("ttl-value"),
		withTTL("10s"),
	)
	resp, err := handler.HandleMessage(ctx, setMsg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("store.status")
	require.True(t, ok)
	assert.Equal(t, "ok", status)

	// 값이 저장되었는지 확인
	getMsg := makeStoreMsg("get", "ttl-key")
	resp, err = handler.HandleMessage(ctx, getMsg)
	require.NoError(t, err)

	val, ok := resp.Payload().Get("value")
	require.True(t, ok)
	assert.Equal(t, "ttl-value", val)
}

// ---------------------------------------------------------------------------
// Delete 연산 테스트
// ---------------------------------------------------------------------------

// TestBridgeHandler_DeleteOperation 은 Delete 연산을 검증한다.
func TestBridgeHandler_DeleteOperation(t *testing.T) {
	ctx := context.Background()
	handler := newTestBridgeHandler(t)

	// 먼저 저장
	setMsg := makeStoreMsg("set", "del-key", withPayloadValue("value"))
	_, err := handler.HandleMessage(ctx, setMsg)
	require.NoError(t, err)

	// 삭제
	delMsg := makeStoreMsg("delete", "del-key")
	resp, err := handler.HandleMessage(ctx, delMsg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("store.status")
	require.True(t, ok)
	assert.Equal(t, "ok", status)

	// 삭제 확인
	getMsg := makeStoreMsg("get", "del-key")
	resp, err = handler.HandleMessage(ctx, getMsg)
	require.NoError(t, err)

	status, _ = resp.Metadata().Get("store.status")
	assert.Equal(t, "error", status)
}

// ---------------------------------------------------------------------------
// Has 연산 테스트
// ---------------------------------------------------------------------------

// TestBridgeHandler_HasOperation 은 Has 연산을 검증한다 (존재/미존재 키).
func TestBridgeHandler_HasOperation(t *testing.T) {
	ctx := context.Background()
	handler := newTestBridgeHandler(t)

	// 존재하지 않는 키
	hasMsg := makeStoreMsg("has", "no-key")
	resp, err := handler.HandleMessage(ctx, hasMsg)
	require.NoError(t, err)

	status, _ := resp.Metadata().Get("store.status")
	assert.Equal(t, "ok", status)

	val, ok := resp.Payload().Get("exists")
	require.True(t, ok)
	assert.Equal(t, false, val)

	// 키 저장 후 확인
	setMsg := makeStoreMsg("set", "yes-key", withPayloadValue("value"))
	_, err = handler.HandleMessage(ctx, setMsg)
	require.NoError(t, err)

	hasMsg = makeStoreMsg("has", "yes-key")
	resp, err = handler.HandleMessage(ctx, hasMsg)
	require.NoError(t, err)

	val, ok = resp.Payload().Get("exists")
	require.True(t, ok)
	assert.Equal(t, true, val)
}

// ---------------------------------------------------------------------------
// Keys 연산 테스트
// ---------------------------------------------------------------------------

// TestBridgeHandler_KeysOperation 은 Keys 연산과 패턴 필터링을 검증한다.
func TestBridgeHandler_KeysOperation(t *testing.T) {
	ctx := context.Background()
	handler := newTestBridgeHandler(t)

	// 여러 키 저장
	for _, k := range []string{"user:1", "user:2", "session:1"} {
		setMsg := makeStoreMsg("set", k, withPayloadValue("v"))
		_, err := handler.HandleMessage(ctx, setMsg)
		require.NoError(t, err)
	}

	// 패턴으로 키 조회
	keysMsg := makeStoreMsg("keys", "", withPattern("user:*"))
	resp, err := handler.HandleMessage(ctx, keysMsg)
	require.NoError(t, err)

	status, _ := resp.Metadata().Get("store.status")
	assert.Equal(t, "ok", status)

	val, ok := resp.Payload().Get("keys")
	require.True(t, ok)

	keys, ok := val.([]string)
	require.True(t, ok)
	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "user:1")
	assert.Contains(t, keys, "user:2")
}

// ---------------------------------------------------------------------------
// Clear 연산 테스트
// ---------------------------------------------------------------------------

// TestBridgeHandler_ClearOperation 은 Clear 연산을 검증한다.
func TestBridgeHandler_ClearOperation(t *testing.T) {
	ctx := context.Background()
	handler := newTestBridgeHandler(t)

	// 키 저장
	setMsg := makeStoreMsg("set", "a", withPayloadValue("1"))
	_, err := handler.HandleMessage(ctx, setMsg)
	require.NoError(t, err)

	// 클리어
	clearMsg := makeStoreMsg("clear", "")
	resp, err := handler.HandleMessage(ctx, clearMsg)
	require.NoError(t, err)

	status, _ := resp.Metadata().Get("store.status")
	assert.Equal(t, "ok", status)

	// 키가 없어야 한다
	keysMsg := makeStoreMsg("keys", "", withPattern("*"))
	resp, err = handler.HandleMessage(ctx, keysMsg)
	require.NoError(t, err)

	val, ok := resp.Payload().Get("keys")
	require.True(t, ok)

	keys, ok := val.([]string)
	require.True(t, ok)
	assert.Empty(t, keys)
}

// ---------------------------------------------------------------------------
// 유효하지 않은 연산 테스트
// ---------------------------------------------------------------------------

// TestBridgeHandler_InvalidOperation 은 유효하지 않은 연산에 대해 에러 상태를 반환하는지 검증한다.
func TestBridgeHandler_InvalidOperation(t *testing.T) {
	ctx := context.Background()
	handler := newTestBridgeHandler(t)

	invalidMsg := makeStoreMsg("invalid-op", "key")
	resp, err := handler.HandleMessage(ctx, invalidMsg)
	require.NoError(t, err)

	status, ok := resp.Metadata().Get("store.status")
	require.True(t, ok)
	assert.Equal(t, "error", status)

	errMsg, ok := resp.Metadata().Get("store.error")
	require.True(t, ok)
	assert.Contains(t, errMsg, "invalid-op")
}

// ---------------------------------------------------------------------------
// 네임스페이스 라우팅 테스트
// ---------------------------------------------------------------------------

// TestBridgeHandler_NamespaceRouting 은 store.namespace 메타데이터에 따라 네임스페이스가 적용되는지 검증한다.
func TestBridgeHandler_NamespaceRouting(t *testing.T) {
	ctx := context.Background()
	handler := newTestBridgeHandler(t)

	// ns-a 네임스페이스에 저장
	setMsg := makeStoreMsg("set", "key1",
		withPayloadValue("value-a"),
		withNamespace("ns-a"),
	)
	_, err := handler.HandleMessage(ctx, setMsg)
	require.NoError(t, err)

	// ns-b 네임스페이스에 저장
	setMsg = makeStoreMsg("set", "key1",
		withPayloadValue("value-b"),
		withNamespace("ns-b"),
	)
	_, err = handler.HandleMessage(ctx, setMsg)
	require.NoError(t, err)

	// ns-a에서 조회 → "value-a"
	getMsg := makeStoreMsg("get", "key1", withNamespace("ns-a"))
	resp, err := handler.HandleMessage(ctx, getMsg)
	require.NoError(t, err)

	status, _ := resp.Metadata().Get("store.status")
	assert.Equal(t, "ok", status)

	val, ok := resp.Payload().Get("value")
	require.True(t, ok)
	assert.Equal(t, "value-a", val)

	// ns-b에서 조회 → "value-b"
	getMsg = makeStoreMsg("get", "key1", withNamespace("ns-b"))
	resp, err = handler.HandleMessage(ctx, getMsg)
	require.NoError(t, err)

	val, ok = resp.Payload().Get("value")
	require.True(t, ok)
	assert.Equal(t, "value-b", val)
}

// ---------------------------------------------------------------------------
// TTL 만료 후 Get 테스트
// ---------------------------------------------------------------------------

// TestBridgeHandler_SetWithTTLExpiration 은 TTL 만료 후 Get이 에러를 반환하는지 검증한다.
func TestBridgeHandler_SetWithTTLExpiration(t *testing.T) {
	ctx := context.Background()
	handler := newTestBridgeHandler(t)

	// 매우 짧은 TTL로 저장
	setMsg := makeStoreMsg("set", "expire-key",
		withPayloadValue("expire-value"),
		withTTL("1ms"),
	)
	_, err := handler.HandleMessage(ctx, setMsg)
	require.NoError(t, err)

	// 만료 대기
	time.Sleep(10 * time.Millisecond)

	// 만료 후 Get
	getMsg := makeStoreMsg("get", "expire-key")
	resp, err := handler.HandleMessage(ctx, getMsg)
	require.NoError(t, err)

	status, _ := resp.Metadata().Get("store.status")
	assert.Equal(t, "error", status)
}
