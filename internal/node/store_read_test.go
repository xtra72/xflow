package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

func TestStoreReadNode_Configure(t *testing.T) {
	def := flow.NodeDef{ID: "sr1", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_template": "{device}:{metric}",
		"namespace":    "devices",
		"output_key":   "cached_value",
	})
	require.NoError(t, err)

	sr := n.(*StoreReadNode)
	assert.Equal(t, "{device}:{metric}", sr.keyTemplate)
	assert.Equal(t, "devices", sr.namespace)
	assert.Equal(t, "cached_value", sr.outputKey)
}

func TestStoreReadNode_Process_BasicGet(t *testing.T) {
	def := flow.NodeDef{ID: "sr2", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	// 미리 데이터 설정
	require.NoError(t, store.Set(context.Background(), "my_key", float64(42)))

	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "my_key",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{"existing": "data"})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 기본 output_key인 "store_value"에 값이 추가되었는지 확인
	v, ok := results[0].Payload().Get("store_value")
	assert.True(t, ok)
	assert.Equal(t, float64(42), v)

	// 기존 payload가 유지되는지 확인
	ev, ok := results[0].Payload().Get("existing")
	assert.True(t, ok)
	assert.Equal(t, "data", ev)
}

func TestStoreReadNode_Process_KeyTemplate(t *testing.T) {
	def := flow.NodeDef{ID: "sr3", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "A:temp", float64(25.5)))

	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "{location}:{type}",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"location": "A",
		"type":     "temp",
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	v, ok := results[0].Payload().Get("store_value")
	assert.True(t, ok)
	assert.Equal(t, float64(25.5), v)
}

func TestStoreReadNode_Process_KeyNotFound(t *testing.T) {
	def := flow.NodeDef{ID: "sr4", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()

	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "nonexistent",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{"data": "test"})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 키가 없으면 output_key에 nil이 설정되지 않고 pass-through
	_, ok := results[0].Payload().Get("store_value")
	assert.False(t, ok)

	// 기존 데이터는 유지
	v, ok := results[0].Payload().Get("data")
	assert.True(t, ok)
	assert.Equal(t, "test", v)
}

func TestStoreReadNode_Process_OutputKey(t *testing.T) {
	def := flow.NodeDef{ID: "sr5", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "k1", "cached_data"))

	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "k1",
		"output_key":   "my_output",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// custom output_key "my_output"에 값이 설정되었는지 확인
	v, ok := results[0].Payload().Get("my_output")
	assert.True(t, ok)
	assert.Equal(t, "cached_data", v)
}

func TestStoreReadNode_Process_PassThrough(t *testing.T) {
	def := flow.NodeDef{ID: "sr6", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "key", "value"))

	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "key",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{"original": "payload"})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 원본 메시지 ID가 동일한지 확인
	assert.Equal(t, msg.ID(), results[0].ID())

	// 기존 데이터가 유지되는지 확인
	v, ok := results[0].Payload().Get("original")
	assert.True(t, ok)
	assert.Equal(t, "payload", v)

	// store에서 읽은 값도 추가되었는지 확인
	sv, ok := results[0].Payload().Get("store_value")
	assert.True(t, ok)
	assert.Equal(t, "value", sv)
}

func TestStoreReadNode_Configure_IncludeHistory(t *testing.T) {
	def := flow.NodeDef{ID: "sr-hist-cfg", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_template":    "test",
		"include_history": true,
	})
	require.NoError(t, err)

	sr := n.(*StoreReadNode)
	assert.True(t, sr.includeHistory, "include_history가 true로 설정되어야 한다")
}

func TestStoreReadNode_Process_IncludeHistory(t *testing.T) {
	def := flow.NodeDef{ID: "sr-hist-on", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "key1", "current-value"))

	err = n.Configure(map[string]any{
		"_store":          store,
		"key_template":    "key1",
		"include_history": true,
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 값이 설정되어야 한다
	v, ok := results[0].Payload().Get("store_value")
	assert.True(t, ok)
	assert.Equal(t, "current-value", v)

	// 히스토리도 포함되어야 한다
	hist, ok := results[0].Payload().Get("history")
	assert.True(t, ok, "include_history=true일 때 history가 payload에 포함되어야 한다")
	histSlice, ok := hist.([]any)
	require.True(t, ok)
	assert.Len(t, histSlice, 2)
	assert.Equal(t, "prev-value-2", histSlice[0])
	assert.Equal(t, "prev-value-1", histSlice[1])
}

func TestStoreReadNode_Process_IncludeHistoryDisabled(t *testing.T) {
	def := flow.NodeDef{ID: "sr-hist-off", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "key1", "current-value"))

	// include_history를 설정하지 않으면 기본값 false
	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "key1",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 값은 설정되어야 한다
	v, ok := results[0].Payload().Get("store_value")
	assert.True(t, ok)
	assert.Equal(t, "current-value", v)

	// 히스토리는 포함되지 않아야 한다
	_, ok = results[0].Payload().Get("history")
	assert.False(t, ok, "include_history=false(기본값)일 때 history가 payload에 포함되지 않아야 한다")
}

func TestStoreReadNode_Configure_IncludeMetadata(t *testing.T) {
	def := flow.NodeDef{ID: "sr-meta-cfg", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_template":     "test",
		"include_metadata": true,
	})
	require.NoError(t, err)

	sr := n.(*StoreReadNode)
	assert.True(t, sr.includeMetadata, "include_metadata가 true로 설정되어야 한다")
}

func TestStoreReadNode_Process_IncludeMetadata(t *testing.T) {
	def := flow.NodeDef{ID: "sr-meta-on", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "key1", "current-value"))

	err = n.Configure(map[string]any{
		"_store":           store,
		"key_template":     "key1",
		"include_metadata": true,
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 값이 설정되어야 한다
	v, ok := results[0].Payload().Get("store_value")
	assert.True(t, ok)
	assert.Equal(t, "current-value", v)

	// 메타데이터 필드들이 포함되어야 한다
	count, ok := results[0].Payload().Get("store_count")
	assert.True(t, ok, "include_metadata=true일 때 store_count가 payload에 포함되어야 한다")
	assert.Equal(t, 3, count)

	createdAt, ok := results[0].Payload().Get("store_created_at")
	assert.True(t, ok, "include_metadata=true일 때 store_created_at이 payload에 포함되어야 한다")
	assert.NotNil(t, createdAt)

	updatedAt, ok := results[0].Payload().Get("store_updated_at")
	assert.True(t, ok, "include_metadata=true일 때 store_updated_at이 payload에 포함되어야 한다")
	assert.NotNil(t, updatedAt)

	oldestAt, ok := results[0].Payload().Get("store_oldest_at")
	assert.True(t, ok, "include_metadata=true일 때 store_oldest_at이 payload에 포함되어야 한다")
	assert.NotNil(t, oldestAt)
}

func TestStoreReadNode_Process_IncludeMetadataDisabled(t *testing.T) {
	def := flow.NodeDef{ID: "sr-meta-off", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "key1", "current-value"))

	// include_metadata를 설정하지 않으면 기본값 false
	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "key1",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 값은 설정되어야 한다
	v, ok := results[0].Payload().Get("store_value")
	assert.True(t, ok)
	assert.Equal(t, "current-value", v)

	// 메타데이터는 포함되지 않아야 한다
	_, ok = results[0].Payload().Get("store_count")
	assert.False(t, ok, "include_metadata=false(기본값)일 때 store_count가 payload에 포함되지 않아야 한다")

	_, ok = results[0].Payload().Get("store_created_at")
	assert.False(t, ok, "include_metadata=false(기본값)일 때 store_created_at이 payload에 포함되지 않아야 한다")
}

func TestStoreReadNode_Process_IncludeMetadataAndHistory(t *testing.T) {
	def := flow.NodeDef{ID: "sr-meta-hist", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "key1", "current-value"))

	err = n.Configure(map[string]any{
		"_store":           store,
		"key_template":     "key1",
		"include_history":  true,
		"include_metadata": true,
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 값이 설정되어야 한다
	v, ok := results[0].Payload().Get("store_value")
	assert.True(t, ok)
	assert.Equal(t, "current-value", v)

	// 히스토리가 포함되어야 한다
	hist, ok := results[0].Payload().Get("history")
	assert.True(t, ok, "include_history=true일 때 history가 payload에 포함되어야 한다")
	histSlice, ok := hist.([]any)
	require.True(t, ok)
	assert.Len(t, histSlice, 2)

	// 메타데이터 필드도 포함되어야 한다
	count, ok := results[0].Payload().Get("store_count")
	assert.True(t, ok, "include_metadata=true일 때 store_count가 payload에 포함되어야 한다")
	assert.Equal(t, 3, count)

	_, ok = results[0].Payload().Get("store_created_at")
	assert.True(t, ok, "include_metadata=true일 때 store_created_at이 payload에 포함되어야 한다")
}

func TestStoreReadNode_Process_IncludeMetadataKeyNotFound(t *testing.T) {
	def := flow.NodeDef{ID: "sr-meta-nf", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()

	err = n.Configure(map[string]any{
		"_store":           store,
		"key_template":     "nonexistent",
		"include_metadata": true,
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 키가 존재하지 않으면 메타데이터도 포함되지 않아야 한다
	_, ok := results[0].Payload().Get("store_value")
	assert.False(t, ok)

	_, ok = results[0].Payload().Get("store_count")
	assert.False(t, ok, "키가 존재하지 않으면 메타데이터도 포함되지 않아야 한다")
}

func TestStoreReadNode_Process_NoStore(t *testing.T) {
	def := flow.NodeDef{ID: "sr7", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

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
