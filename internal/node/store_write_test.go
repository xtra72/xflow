package node

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// mockStoreWriter 는 테스트용 StoreWriter + StoreReader 구현체이다.
type mockStoreWriter struct {
	mu   sync.Mutex
	data map[string]mockStoreEntry
	// mockHistory 는 QueryHistory 호출 시 반환할 시계열 엔트리이다.
	// 설정되지 않으면 현재값(data[key])만 포함한 단일 엔트리 슬라이스를 반환한다.
	mockHistory map[string][]map[string]any
}

type mockStoreEntry struct {
	value any
	ttl   time.Duration
}

func newMockStore() *mockStoreWriter {
	return &mockStoreWriter{
		data:        make(map[string]mockStoreEntry),
		mockHistory: make(map[string][]map[string]any),
	}
}

// setMockHistory 는 QueryHistory 가 반환할 시계열 엔트리를 설정한다 (테스트 헬퍼).
func (m *mockStoreWriter) setMockHistory(key string, entries []map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mockHistory[key] = entries
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

// QueryHistory 는 HistoryQueryReader 인터페이스를 구현한다.
// mockHistory 가 설정되어 있으면 해당 값을 그대로 반환하고,
// 없으면 data 에 저장된 현재값 1건을 포함한 단일 엔트리를 반환한다.
// system.HistoryQuery 의 Validate 를 호출하지만, 필터링은 적용하지 않는다 (테스트는 모드별 직접 검증).
func (m *mockStoreWriter) QueryHistory(_ context.Context, key string, q system.HistoryQuery) ([]map[string]any, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if hist, ok := m.mockHistory[key]; ok {
		// 반환 슬라이스를 복사하여 외부 변형을 방지한다.
		out := make([]map[string]any, len(hist))
		for i, e := range hist {
			cp := make(map[string]any, len(e))
			for k, v := range e {
				cp[k] = v
			}
			out[i] = cp
		}
		return out, nil
	}

	entry, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	return []map[string]any{
		{"value": entry.value, "timestamp": time.Now().UnixMilli()},
	}, nil
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
		"value_key":    "$.payload.temperature",
		"namespace":    "sensors",
		"ttl":          "5m",
	})
	require.NoError(t, err)

	sw := n.(*StoreWriteNode)
	assert.Equal(t, "{location}:{sensor}", sw.keyTemplate)
	assert.Equal(t, "$.payload.temperature", sw.valueKey)
	assert.Equal(t, "sensors", sw.namespace)
	assert.Equal(t, 5*time.Minute, sw.ttl)
}

// data_type: "auto" 는 유효값으로 수용된다(노드 레벨 + metrics 항목 양쪽).
// "auto" 는 쓰기 값 타입을 추론해 키별로 고정하는 sentinel 이다(가변 타입 단일 노드 지원).
func TestStoreWriteNode_Configure_DataTypeAuto(t *testing.T) {
	def := flow.NodeDef{ID: "sw-auto", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_template": "{$.metadata.device.name}-{$.metadata.measurement}",
		"value_key":    "$.payload.value",
		"data_type":    "auto",
		"metrics": []any{
			map[string]any{"metric_type": "$.metadata.measurement", "value_key": "$.payload.value", "data_type": "auto"},
		},
	})
	require.NoError(t, err)

	sw := n.(*StoreWriteNode)
	assert.Equal(t, "auto", sw.dataType)
	require.Len(t, sw.metrics, 1)
	assert.Equal(t, "auto", sw.metrics[0].dataType)
}

// 6종 enum 도 "auto" 도 아닌 data_type 은 여전히 거부된다.
func TestStoreWriteNode_Configure_DataTypeInvalid(t *testing.T) {
	def := flow.NodeDef{ID: "sw-bad", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_template": "k",
		"data_type":    "number",
	})
	require.Error(t, err)
}

func TestStoreWriteNode_Process_BasicSet(t *testing.T) {
	def := flow.NodeDef{ID: "sw2", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockStore()
	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "my_key",
		"value_key":    "$.payload.data",
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
		"value_key":    "$.payload.value",
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
		"value_key":    "$.payload.data",
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
		"value_key":    "$.payload.temperature",
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

// TestStoreWriteNode_Process_ValueKey_PayloadPath 는 v0.7.10 의 value_key 에
// $.payload.<path> JSONPath 구문이 적용되는지 검증한다.
func TestStoreWriteNode_Process_ValueKey_PayloadPath(t *testing.T) {
	def := flow.NodeDef{ID: "sw-vp", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockStore()
	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "k",
		"value_key":    "$.payload.state.current_temperature",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"state": map[string]any{
			"current_temperature": float64(23.5),
			"mode":                1,
		},
	})
	msg := message.New(message.WithPayload(payload))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	val, ok, getErr := store.Get(context.Background(), "k")
	require.NoError(t, getErr)
	assert.True(t, ok)
	assert.Equal(t, float64(23.5), val)
}

// TestStoreWriteNode_Process_ValueKey_MetadataPath 는 value_key 에
// $.metadata.<field> 가 적용되는지 검증한다.
func TestStoreWriteNode_Process_ValueKey_MetadataPath(t *testing.T) {
	def := flow.NodeDef{ID: "sw-vm", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockStore()
	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "k",
		"value_key":    "$.metadata.device_id",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	msg := message.New()
	msg.Metadata().Set("device_id", "idu-1")

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	val, ok, getErr := store.Get(context.Background(), "k")
	require.NoError(t, getErr)
	assert.True(t, ok)
	assert.Equal(t, "idu-1", val)
}

// TestStoreWriteNode_Process_ValueKey_NotFound 는 잘못된 경로 시 에러
// (legacy / JSONPath 모두).
func TestStoreWriteNode_Process_ValueKey_NotFound(t *testing.T) {
	def := flow.NodeDef{ID: "sw-vn", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockStore()
	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "k",
		"value_key":    "$.payload.missing.path",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	msg := message.New()
	_, err = n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "value_key")
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
		"value_key":    "$.payload.data",
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

// TestStoreWriteNode_Init_NoAgentRef 는 AgentRef 미설정 시 Init 단계에서 즉시
// 실패해야 함을 검증한다 (fail-fast 정책). 이전에는 Init 이 성공한 뒤 Process
// 시점에 ErrStoreNotConfigured 를 반환하는 lazy-fail 패턴이었으나,
// 디버깅을 어렵게 만들고 잘못된 플로우가 Running 상태로 진입하는 문제가 있어
// 다른 스토리지 노드(influxdb/tsdb)와 동일한 fail-fast 패턴으로 통일되었다.
func TestStoreWriteNode_Init_NoAgentRef(t *testing.T) {
	def := flow.NodeDef{ID: "sw8", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	// Store를 주입하지 않고 Configure
	err = n.Configure(map[string]any{
		"key_template": "test",
	})
	require.NoError(t, err)

	// Init 이 즉시 에러를 반환해야 한다.
	err = n.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent_ref is required")
}

func TestStoreWriteNode_Process_MissingKeyField(t *testing.T) {
	def := flow.NodeDef{ID: "sw9", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockStore()
	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "{location}:{missing_field}",
		"value_key":    "$.payload.data",
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
