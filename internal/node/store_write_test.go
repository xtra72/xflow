package node

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// mockStoreWriter 는 테스트용 StoreWriter + StoreReader 구현체이다.
type mockStoreWriter struct {
	mu   sync.Mutex
	data map[string]mockStoreEntry
}

type mockStoreEntry struct {
	value any
	ttl   time.Duration
}

func newMockStore() *mockStoreWriter {
	return &mockStoreWriter{data: make(map[string]mockStoreEntry)}
}

func (m *mockStoreWriter) Set(_ context.Context, key string, value any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = mockStoreEntry{value: value}
	return nil
}

func (m *mockStoreWriter) SetWithTTL(_ context.Context, key string, value any, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = mockStoreEntry{value: value, ttl: ttl}
	return nil
}

func (m *mockStoreWriter) Get(_ context.Context, key string) (any, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.data[key]
	if !ok {
		return nil, false, nil
	}
	return entry.value, true, nil
}

func (m *mockStoreWriter) Has(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[key]
	return ok, nil
}

func (m *mockStoreWriter) GetHistory(_ context.Context, key string) ([]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	// 테스트용: 간단한 히스토리를 반환한다
	return []any{"prev-value-2", "prev-value-1"}, nil
}

// GetMetadata 는 MetadataReader 인터페이스를 구현한다.
// 테스트용 고정 메타데이터를 반환한다.
func (m *mockStoreWriter) GetMetadata(_ context.Context, key string) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	now := time.Now()
	return map[string]any{
		"store_count":      3,
		"store_created_at": now.Add(-1 * time.Hour),
		"store_updated_at": now,
		"store_oldest_at":  now.Add(-2 * time.Hour),
	}, nil
}

// --- store-write 테스트 ---

func TestStoreWriteNode_Configure(t *testing.T) {
	def := flow.NodeDef{ID: "sw1", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_template": "{location}:{sensor}",
		"value_key":    "temperature",
		"namespace":    "sensors",
		"ttl":          "5m",
	})
	require.NoError(t, err)

	sw := n.(*StoreWriteNode)
	assert.Equal(t, "{location}:{sensor}", sw.keyTemplate)
	assert.Equal(t, "temperature", sw.valueKey)
	assert.Equal(t, "sensors", sw.namespace)
	assert.Equal(t, 5*time.Minute, sw.ttl)
}

func TestStoreWriteNode_Process_BasicSet(t *testing.T) {
	def := flow.NodeDef{ID: "sw2", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockStore()
	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "my_key",
		"value_key":    "data",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"data": float64(42),
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// Store에 값이 기록되었는지 확인
	val, ok, getErr := store.Get(context.Background(), "my_key")
	require.NoError(t, getErr)
	assert.True(t, ok)
	assert.Equal(t, float64(42), val)
}

func TestStoreWriteNode_Process_KeyTemplate(t *testing.T) {
	def := flow.NodeDef{ID: "sw3", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockStore()
	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "{location}:{point}:{type}",
		"value_key":    "value",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"location": "A",
		"point":    "1",
		"type":     "temp",
		"value":    float64(25.5),
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 키가 "A:1:temp"로 해석되었는지 확인
	val, ok, getErr := store.Get(context.Background(), "A:1:temp")
	require.NoError(t, getErr)
	assert.True(t, ok)
	assert.Equal(t, float64(25.5), val)
}

func TestStoreWriteNode_Process_WithTTL(t *testing.T) {
	def := flow.NodeDef{ID: "sw4", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockStore()
	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "ttl_key",
		"value_key":    "data",
		"ttl":          "1h",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"data": "hello",
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// TTL이 올바르게 전달되었는지 확인
	store.mu.Lock()
	entry, ok := store.data["ttl_key"]
	store.mu.Unlock()
	assert.True(t, ok)
	assert.Equal(t, "hello", entry.value)
	assert.Equal(t, 1*time.Hour, entry.ttl)
}

func TestStoreWriteNode_Process_ValueKey(t *testing.T) {
	def := flow.NodeDef{ID: "sw5", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockStore()
	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "specific_key",
		"value_key":    "temperature",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"temperature": float64(36.6),
		"humidity":    float64(55.0),
		"extra":       "ignored",
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// temperature 값만 저장되었는지 확인
	val, ok, getErr := store.Get(context.Background(), "specific_key")
	require.NoError(t, getErr)
	assert.True(t, ok)
	assert.Equal(t, float64(36.6), val)
}

func TestStoreWriteNode_Process_WholePayload(t *testing.T) {
	def := flow.NodeDef{ID: "sw6", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockStore()
	// value_key를 설정하지 않으면 전체 payload를 저장
	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "whole_key",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"a": float64(1),
		"b": float64(2),
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 전체 payload가 map으로 저장되었는지 확인
	val, ok, getErr := store.Get(context.Background(), "whole_key")
	require.NoError(t, getErr)
	assert.True(t, ok)
	m, isMap := val.(map[string]any)
	require.True(t, isMap)
	assert.Equal(t, float64(1), m["a"])
	assert.Equal(t, float64(2), m["b"])
}

func TestStoreWriteNode_Process_PassThrough(t *testing.T) {
	def := flow.NodeDef{ID: "sw7", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockStore()
	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "pt_key",
		"value_key":    "data",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"data": float64(99),
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 반환된 메시지가 원본과 동일한지 확인 (pass-through)
	assert.Equal(t, msg.ID(), results[0].ID())
	v, ok := results[0].Payload().Get("data")
	assert.True(t, ok)
	assert.Equal(t, float64(99), v)
}

func TestStoreWriteNode_Process_NoStore(t *testing.T) {
	def := flow.NodeDef{ID: "sw8", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	// Store를 주입하지 않고 Configure
	err = n.Configure(map[string]any{
		"key_template": "test",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{"data": "test"})
	msg := message.New(message.WithPayload(payload))

	_, err = n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrStoreNotConfigured)
}

func TestStoreWriteNode_Process_MissingKeyField(t *testing.T) {
	def := flow.NodeDef{ID: "sw9", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockStore()
	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "{location}:{missing_field}",
		"value_key":    "data",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"location": "A",
		"data":     float64(10),
	})
	msg := message.New(message.WithPayload(payload))

	_, err = n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing_field")
}
