package node

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// TestStoreReadNode_Configure 는 기본 설정 필드가 정상 반영되는지 검증한다.
func TestStoreReadNode_Configure(t *testing.T) {
	def := flow.NodeDef{ID: "sr1", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_template": "{device}:{field}",
		"namespace":    "devices",
		"output_key":   "cached_value",
	})
	require.NoError(t, err)

	sr := n.(*StoreReadNode)
	assert.Equal(t, "{device}:{field}", sr.keyTemplate)
	assert.Equal(t, "devices", sr.namespace)
	assert.Equal(t, "cached_value", sr.outputKey)
	assert.Equal(t, ReadModeLatest, sr.readMode, "기본 read_mode 는 latest 이어야 한다")
}

// TestStoreReadNode_Process_LatestArray 는 latest 모드가 단일 엔트리 배열을 반환하는지 검증한다.
func TestStoreReadNode_Process_LatestArray(t *testing.T) {
	def := flow.NodeDef{ID: "sr2", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
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

	raw, ok := results[0].Payload().Get("store_value")
	require.True(t, ok, "output_key 에 배열이 기록되어야 한다")
	entries, ok := raw.([]map[string]any)
	require.True(t, ok, "output_key 는 []map[string]any 타입이어야 한다")
	require.Len(t, entries, 1, "latest 모드는 길이 1 배열이어야 한다")
	assert.Equal(t, float64(42), entries[0]["value"])
	ts, hasTS := entries[0]["timestamp"]
	require.True(t, hasTS, "각 엔트리는 timestamp 필드를 가져야 한다")
	_, isInt64 := ts.(int64)
	assert.True(t, isInt64, "timestamp 는 epoch ms int64 여야 한다")

	// 기존 payload 유지 확인
	ev, ok := results[0].Payload().Get("existing")
	assert.True(t, ok)
	assert.Equal(t, "data", ev)
}

// TestStoreReadNode_Process_KeyTemplate 는 동적 키 해석이 동작하는지 검증한다.
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

	raw, ok := results[0].Payload().Get("store_value")
	require.True(t, ok)
	entries := raw.([]map[string]any)
	require.Len(t, entries, 1)
	assert.Equal(t, float64(25.5), entries[0]["value"])
}

// TestStoreReadNode_Process_KeyNotFound 는 키가 없을 때 빈 배열이 기록되는지 검증한다.
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

	raw, ok := results[0].Payload().Get("store_value")
	require.True(t, ok, "키가 없어도 output_key 에는 빈 배열이 기록되어야 한다")
	entries := raw.([]map[string]any)
	assert.Empty(t, entries)

	v, ok := results[0].Payload().Get("data")
	assert.True(t, ok)
	assert.Equal(t, "test", v)
}

// TestStoreReadNode_Process_OutputKey 는 output_key 설정이 동작하는지 검증한다.
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

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)

	raw, ok := results[0].Payload().Get("my_output")
	require.True(t, ok)
	entries := raw.([]map[string]any)
	require.Len(t, entries, 1)
	assert.Equal(t, "cached_data", entries[0]["value"])
}

// TestStoreReadNode_Configure_LastN_RequiresCount 는 last_n 모드가 count 를 요구하는지 검증한다.
func TestStoreReadNode_Configure_LastN_RequiresCount(t *testing.T) {
	def := flow.NodeDef{ID: "sr-lastn-nocount", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_template": "k",
		"read_mode":    "last_n",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "count > 0")
}

// TestStoreReadNode_Configure_Duration 은 duration 문자열이 해석되는지 검증한다.
func TestStoreReadNode_Configure_Duration(t *testing.T) {
	def := flow.NodeDef{ID: "sr-dur", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_template": "k",
		"read_mode":    "duration",
		"duration":     "5m",
	})
	require.NoError(t, err)

	sr := n.(*StoreReadNode)
	assert.Equal(t, 5*time.Minute, sr.duration)
}

// TestStoreReadNode_Configure_Duration_Invalid 는 잘못된 duration 문자열에 에러를 반환하는지 검증한다.
func TestStoreReadNode_Configure_Duration_Invalid(t *testing.T) {
	def := flow.NodeDef{ID: "sr-dur-bad", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_template": "k",
		"read_mode":    "duration",
		"duration":     "not-a-duration",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid duration")
}

// TestStoreReadNode_Configure_TimeRange_Requires 는 time_range 모드가 from/to 를 요구하는지 검증한다.
func TestStoreReadNode_Configure_TimeRange_Requires(t *testing.T) {
	def := flow.NodeDef{ID: "sr-tr", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_template": "k",
		"read_mode":    "time_range",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "from and to")
}

// TestStoreReadNode_Configure_UnknownMode 는 알 수 없는 모드에 에러를 반환하는지 검증한다.
func TestStoreReadNode_Configure_UnknownMode(t *testing.T) {
	def := flow.NodeDef{ID: "sr-unknown", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_template": "k",
		"read_mode":    "bogus",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown read_mode")
}

// TestStoreReadNode_Process_LastN 은 last_n 모드에서 배열 전체가 반환되는지 검증한다.
func TestStoreReadNode_Process_LastN(t *testing.T) {
	def := flow.NodeDef{ID: "sr-lastn", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "k", "c"))
	now := time.Now()
	store.setMockHistory("k", []map[string]any{
		{"value": "c", "timestamp": now.UnixMilli()},
		{"value": "b", "timestamp": now.Add(-time.Minute).UnixMilli()},
		{"value": "a", "timestamp": now.Add(-2 * time.Minute).UnixMilli()},
	})

	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "k",
		"read_mode":    "last_n",
		"count":        3,
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)

	raw, ok := results[0].Payload().Get("store_value")
	require.True(t, ok)
	entries := raw.([]map[string]any)
	require.Len(t, entries, 3)
	assert.Equal(t, "c", entries[0]["value"])
	assert.Equal(t, "b", entries[1]["value"])
	assert.Equal(t, "a", entries[2]["value"])
}

// TestStoreReadNode_Process_TimeRange_DynamicRef 는 time_range 모드에서 {field} 동적 참조가 동작하는지 검증한다.
func TestStoreReadNode_Process_TimeRange_DynamicRef(t *testing.T) {
	def := flow.NodeDef{ID: "sr-tr-dyn", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "k", "cur"))
	// mock 은 필터를 적용하지 않으므로, 이 테스트는 동적 참조가 에러 없이 해석되는지만 검증한다.
	store.setMockHistory("k", []map[string]any{
		{"value": "cur", "timestamp": time.Now().UnixMilli()},
	})

	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "k",
		"read_mode":    "time_range",
		"from":         "{from_ts}",
		"to":           "{to_ts}",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	from := time.Now().Add(-time.Hour).UnixMilli()
	to := time.Now().UnixMilli()
	payload := message.NewPayload(map[string]any{
		"from_ts": from,
		"to_ts":   to,
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	raw, ok := results[0].Payload().Get("store_value")
	require.True(t, ok)
	_, ok = raw.([]map[string]any)
	assert.True(t, ok, "time_range 모드도 동일한 배열 형식으로 반환되어야 한다")
}

// TestStoreReadNode_Process_TimeRange_EpochLiteral 는 time_range 모드에서 epoch ms 리터럴이 파싱되는지 검증한다.
func TestStoreReadNode_Process_TimeRange_EpochLiteral(t *testing.T) {
	def := flow.NodeDef{ID: "sr-tr-epoch", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "k", "cur"))

	fromMs := time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC).UnixMilli()
	toMs := time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC).UnixMilli()

	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "k",
		"read_mode":    "time_range",
		"from":         strconv.FormatInt(fromMs, 10),
		"to":           strconv.FormatInt(toMs, 10),
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{})))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)
}

// TestStoreReadNode_Process_TimeRange_RFC3339Fallback 는 RFC3339 리터럴이 호환성 폴백으로 동작하는지 검증한다.
func TestStoreReadNode_Process_TimeRange_RFC3339Fallback(t *testing.T) {
	def := flow.NodeDef{ID: "sr-tr-lit", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "k", "cur"))

	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "k",
		"read_mode":    "time_range",
		"from":         "2026-04-16T00:00:00Z",
		"to":           "2026-04-17T00:00:00Z",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{})))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)
}

// TestStoreReadNode_Process_SinceN 은 since_n 모드가 정상 동작하는지 검증한다.
func TestStoreReadNode_Process_SinceN(t *testing.T) {
	def := flow.NodeDef{ID: "sr-since", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "k", "cur"))

	sinceMs := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "k",
		"read_mode":    "since_n",
		"count":        5,
		"since":        strconv.FormatInt(sinceMs, 10),
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	raw, ok := results[0].Payload().Get("store_value")
	require.True(t, ok)
	_, ok = raw.([]map[string]any)
	assert.True(t, ok)
}

// TestStoreReadNode_Process_Duration 은 duration 모드가 정상 동작하는지 검증한다.
func TestStoreReadNode_Process_Duration(t *testing.T) {
	def := flow.NodeDef{ID: "sr-dur-run", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "k", "cur"))

	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "k",
		"read_mode":    "duration",
		"duration":     "10m",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	raw, ok := results[0].Payload().Get("store_value")
	require.True(t, ok)
	_, ok = raw.([]map[string]any)
	assert.True(t, ok)
}

// TestStoreReadNode_Configure_IncludeMetadata 는 include_metadata 필드가 반영되는지 검증한다.
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
	assert.True(t, sr.includeMetadata)
}

// TestStoreReadNode_Process_IncludeMetadata 는 include_metadata=true 시 메타 필드가 payload 에 추가되는지 검증한다.
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

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)

	count, ok := results[0].Payload().Get("store_count")
	assert.True(t, ok)
	assert.Equal(t, 3, count)

	_, ok = results[0].Payload().Get("store_created_at")
	assert.True(t, ok)
}

// TestStoreReadNode_Process_IncludeMetadataDisabled 는 기본값에서 메타가 추가되지 않음을 검증한다.
func TestStoreReadNode_Process_IncludeMetadataDisabled(t *testing.T) {
	def := flow.NodeDef{ID: "sr-meta-off", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "key1", "current-value"))

	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "key1",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)

	_, ok := results[0].Payload().Get("store_count")
	assert.False(t, ok)
}

// TestStoreReadNode_Init_NoAgentRef 는 AgentRef 미설정 시 Init 단계에서 즉시 실패해야 함을 검증한다.
func TestStoreReadNode_Init_NoAgentRef(t *testing.T) {
	def := flow.NodeDef{ID: "sr7", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_template": "test",
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent_ref is required")
}

// TestStoreReadNode_Process_DynamicTimeField_EpochInt64 는 동적 참조 값이 int64(epoch ms)일 때 해석되는지 검증한다.
func TestStoreReadNode_Process_DynamicTimeField_EpochInt64(t *testing.T) {
	def := flow.NodeDef{ID: "sr-dyn-i64", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "k", "cur"))

	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "k",
		"read_mode":    "since_n",
		"count":        3,
		"since":        "{since_ts}",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"since_ts": time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC).UnixMilli(),
	})
	msg := message.New(message.WithPayload(payload))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)
}

// TestStoreReadNode_Process_DynamicTimeField_Float64 는 JSON 디코딩된 float64 (epoch ms)가 해석되는지 검증한다.
func TestStoreReadNode_Process_DynamicTimeField_Float64(t *testing.T) {
	def := flow.NodeDef{ID: "sr-dyn-f64", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "k", "cur"))

	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "k",
		"read_mode":    "since_n",
		"count":        3,
		"since":        "{since_ts}",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"since_ts": float64(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC).UnixMilli()),
	})
	msg := message.New(message.WithPayload(payload))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)
}

// TestStoreReadNode_Process_DynamicTimeField_Missing 는 payload 에 없는 필드 참조 시 에러를 반환하는지 검증한다.
func TestStoreReadNode_Process_DynamicTimeField_Missing(t *testing.T) {
	def := flow.NodeDef{ID: "sr-dyn-missing", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "k", "cur"))

	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "k",
		"read_mode":    "since_n",
		"count":        3,
		"since":        "{missing_field}",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{})))

	_, err = n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing_field")
}

// ---- entries_field 배치 읽기 ----

// TestStoreReadNode_EntriesField_BatchRead 는 배열 요소별로 키를 해석하여
// 다중 키를 한 번에 읽는 기능을 검증한다.
func TestStoreReadNode_EntriesField_BatchRead(t *testing.T) {
	def := flow.NodeDef{ID: "sr-batch", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "device.room1.temp", float64(23.5)))
	require.NoError(t, store.Set(context.Background(), "device.room2.temp", float64(24.1)))
	require.NoError(t, store.Set(context.Background(), "device.room3.temp", float64(22.8)))

	err = n.Configure(map[string]any{
		"_store":        store,
		"key_template":  "device.{item}.temp",
		"entries_field": "rooms",
		"entries_var":   "item",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"rooms": []any{"room1", "room2", "room3"},
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	raw, ok := results[0].Payload().Get("store_value")
	require.True(t, ok)
	batch, ok := raw.(map[string]any)
	require.True(t, ok, "entries_field 모드는 map[string]any 를 반환해야 한다")
	require.Len(t, batch, 3)

	// SPEC-NODE-001 v1.5.0: 결과 map 키는 resolved store 키 사용.
	for _, room := range []string{"room1", "room2", "room3"} {
		fullKey := "device." + room + ".temp"
		entries, ok := batch[fullKey].([]map[string]any)
		require.True(t, ok, "각 항목은 []map[string]any 이어야 한다: %s", fullKey)
		require.Len(t, entries, 1)
	}
	r1 := batch["device.room1.temp"].([]map[string]any)
	assert.Equal(t, float64(23.5), r1[0]["value"])
	r3 := batch["device.room3.temp"].([]map[string]any)
	assert.Equal(t, float64(22.8), r3[0]["value"])
}

// TestStoreReadNode_EntriesField_EmptyArray 는 빈 배열이면 빈 map 을 반환하는지 검증한다.
func TestStoreReadNode_EntriesField_EmptyArray(t *testing.T) {
	def := flow.NodeDef{ID: "sr-empty", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()

	err = n.Configure(map[string]any{
		"_store":        store,
		"key_template":  "device.{item}.temp",
		"entries_field": "rooms",
		"entries_var":   "item",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"rooms": []any{},
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	raw, ok := results[0].Payload().Get("store_value")
	require.True(t, ok)
	batch, ok := raw.(map[string]any)
	require.True(t, ok)
	assert.Empty(t, batch)
}

// TestStoreReadNode_EntriesField_MissingField 는 entries_field 가 payload 에 없으면 에러인지 검증한다.
func TestStoreReadNode_EntriesField_MissingField(t *testing.T) {
	def := flow.NodeDef{ID: "sr-miss", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()

	err = n.Configure(map[string]any{
		"_store":        store,
		"key_template":  "device.{item}.temp",
		"entries_field": "rooms",
		"entries_var":   "item",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	// rooms 필드 없음
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{})))

	_, err = n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entries_field")
}

// TestStoreReadNode_EntriesField_NoVarLeakInPayload 는 배치 처리 후 임시 변수가 payload 에 남지 않는지 검증한다.
func TestStoreReadNode_EntriesField_NoVarLeakInPayload(t *testing.T) {
	def := flow.NodeDef{ID: "sr-leak", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "k.A", float64(1)))

	// entries_var 미지정 → 기본값 "item" 사용
	err = n.Configure(map[string]any{
		"_store":        store,
		"key_template":  "k.{item}",
		"entries_field": "ids",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"ids": []any{"A"},
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)

	// 임시 변수 "item" 이 payload 에 남아있으면 안 됨
	_, leaked := results[0].Payload().Get("item")
	assert.False(t, leaked, "임시 변수 item 이 payload 에 남아있으면 안 된다")
}

// TestStoreReadNode_EntriesField_TypedSlice 는 []string 등 구체 타입 슬라이스를 처리하는지 검증한다.
func TestStoreReadNode_EntriesField_TypedSlice(t *testing.T) {
	def := flow.NodeDef{ID: "sr-typed", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "dev.X.val", float64(10)))
	require.NoError(t, store.Set(context.Background(), "dev.Y.val", float64(20)))

	err = n.Configure(map[string]any{
		"_store":        store,
		"key_template":  "dev.{id}.val",
		"entries_field": "device_ids",
		"entries_var":   "id",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	// []string 타입 ([]any 아님)
	payload := message.NewPayload(map[string]any{
		"device_ids": []string{"X", "Y"},
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	raw, ok := results[0].Payload().Get("store_value")
	require.True(t, ok)
	batch, ok := raw.(map[string]any)
	require.True(t, ok)
	require.Len(t, batch, 2)
	// SPEC-NODE-001 v1.5.0: resolved key 사용.
	assert.Contains(t, batch, "dev.X.val")
	assert.Contains(t, batch, "dev.Y.val")
}

// TestStoreReadNode_EntriesField_PrimitiveStrings 는 string 요소가 다른 슬라이스
// 타입([]any) 으로 들어와도 정상 처리되는지 검증한다.
func TestStoreReadNode_EntriesField_PrimitiveStrings(t *testing.T) {
	def := flow.NodeDef{ID: "sr-prim", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "device.A.temp", float64(20)))
	require.NoError(t, store.Set(context.Background(), "device.B.temp", float64(30)))

	err = n.Configure(map[string]any{
		"_store":        store,
		"key_template":  "device.{item}.temp",
		"entries_field": "devices",
		"entries_var":   "item",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"devices": []any{"A", "B"},
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)

	raw, _ := results[0].Payload().Get("store_value")
	batch := raw.(map[string]any)
	assert.Len(t, batch, 2)
	// SPEC-NODE-001 v1.5.0: resolved key 사용.
	assert.Contains(t, batch, "device.A.temp")
	assert.Contains(t, batch, "device.B.temp")
}

// TestStoreReadNode_EntriesField_ObjectElements 는 배열 요소가 객체일 때
// {entries_var.field} dot notation 으로 nested 접근하는지 검증한다.
// SPEC-NODE-001 v1.5.0 신규 동작.
func TestStoreReadNode_EntriesField_ObjectElements(t *testing.T) {
	def := flow.NodeDef{ID: "sr-obj", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "device.room1.temp", float64(23.5)))
	require.NoError(t, store.Set(context.Background(), "device.room2.temp", float64(24.1)))

	err = n.Configure(map[string]any{
		"_store":        store,
		"key_template":  "device.{item.id}.temp",
		"entries_field": "devices",
		"entries_var":   "item",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"devices": []any{
			map[string]any{"id": "room1", "name": "거실"},
			map[string]any{"id": "room2", "name": "안방"},
		},
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)

	raw, _ := results[0].Payload().Get("store_value")
	batch := raw.(map[string]any)
	require.Len(t, batch, 2)
	assert.Contains(t, batch, "device.room1.temp")
	assert.Contains(t, batch, "device.room2.temp")

	r1 := batch["device.room1.temp"].([]map[string]any)
	assert.Equal(t, float64(23.5), r1[0]["value"])
	r2 := batch["device.room2.temp"].([]map[string]any)
	assert.Equal(t, float64(24.1), r2[0]["value"])

	// 임시 변수 item 이 leak 되지 않아야 함
	_, leaked := results[0].Payload().Get("item")
	assert.False(t, leaked)
}

// TestStoreReadNode_EntriesField_ObjectNestedField 는 깊은 nested 객체 필드
// ({item.location.zone}) 도 처리하는지 검증한다.
func TestStoreReadNode_EntriesField_ObjectNestedField(t *testing.T) {
	def := flow.NodeDef{ID: "sr-deep", Type: "store-read"}
	n, err := NewStoreReadNode(def)
	require.NoError(t, err)

	store := newMockStore()
	require.NoError(t, store.Set(context.Background(), "zone.north.temp", float64(18.0)))

	err = n.Configure(map[string]any{
		"_store":        store,
		"key_template":  "zone.{item.location.zone}.temp",
		"entries_field": "devices",
		"entries_var":   "item",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"devices": []any{
			map[string]any{
				"id": "d1",
				"location": map[string]any{
					"zone": "north",
				},
			},
		},
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)

	raw, _ := results[0].Payload().Get("store_value")
	batch := raw.(map[string]any)
	assert.Contains(t, batch, "zone.north.temp")
}
