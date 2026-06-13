// store_write_meta_test.go 는 store-write 노드의 data_type / tags 지정 기능을 검증한다.
//
// 커버리지:
//   - Configure: data_type enum 검증, tags 정규화/검증
//   - Process: data_type/tags 설정 시 StoreMetaWriter.SetWithMeta 사용
//   - Process: data_type/tags 미설정 시 기존 Set/SetWithTTL 폴백 (PRESERVE)
//   - Process: store 가 StoreMetaWriter 미구현 시 폴백 (PRESERVE)
//   - 동적 키(key_template) + data_type + tags 조합
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

// mockMetaStore 는 StoreWriter + StoreMetaWriter 를 구현하는 테스트 더블이다.
// SetWithMeta 호출 인자를 캡처하여 노드가 메타 경로를 올바르게 사용하는지 검증한다.
type mockMetaStore struct {
	mu sync.Mutex
	// setCalls / setTTLCalls 는 일반 쓰기 경로 호출 여부를 추적한다 (폴백 검증용).
	setCalls    int
	setTTLCalls int
	// metaCalls 는 SetWithMeta 호출을 기록한다.
	metaCalls []metaCall
}

type metaCall struct {
	key   string
	value any
	opts  StoreWriteMeta
}

func newMockMetaStore() *mockMetaStore {
	return &mockMetaStore{}
}

func (m *mockMetaStore) Set(_ context.Context, _ string, _ any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setCalls++
	return nil
}

func (m *mockMetaStore) SetWithTTL(_ context.Context, _ string, _ any, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setTTLCalls++
	return nil
}

func (m *mockMetaStore) SetWithMeta(_ context.Context, key string, value any, opts StoreWriteMeta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metaCalls = append(m.metaCalls, metaCall{key: key, value: value, opts: opts})
	return nil
}

// --- Configure: data_type / tags 파싱 ---

func TestStoreWriteNode_Configure_DataTypeAndTags(t *testing.T) {
	def := flow.NodeDef{ID: "swm1", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_template": "k",
		"data_type":    "float",
		"tags": map[string]any{
			"room":  "kitchen",
			"floor": "2",
		},
	})
	require.NoError(t, err)

	sw := n.(*StoreWriteNode)
	assert.Equal(t, "float", sw.dataType)
	assert.Equal(t, "kitchen", sw.tags["room"])
	assert.Equal(t, "2", sw.tags["floor"])
}

func TestStoreWriteNode_Configure_InvalidDataType(t *testing.T) {
	def := flow.NodeDef{ID: "swm2", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_template": "k",
		"data_type":    "decimal", // 6종 enum 외
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid data_type")
}

func TestStoreWriteNode_Configure_InvalidTagKey(t *testing.T) {
	def := flow.NodeDef{ID: "swm3", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_template": "k",
		"tags":         map[string]any{"bad key!": "v"}, // 공백/특수문자 위반
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tag key")
}

func TestStoreWriteNode_Configure_NonStringTagValue(t *testing.T) {
	def := flow.NodeDef{ID: "swm4", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_template": "k",
		"tags":         map[string]any{"room": 123}, // value 가 문자열 아님
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

// --- Process: 메타 경로 ---

// data_type 지정 시 SetWithMeta 가 올바른 opts 로 호출되는지 검증한다.
func TestStoreWriteNode_Process_DataType_UsesSetWithMeta(t *testing.T) {
	def := flow.NodeDef{ID: "swm5", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockMetaStore()
	require.NoError(t, n.Configure(map[string]any{
		"_store":       store,
		"key_template": "temp",
		"value_key":    "$.payload.t",
		"data_type":    "float",
	}))
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{"t": 21.5})
	msg := message.New(message.WithPayload(payload))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	require.Len(t, store.metaCalls, 1, "SetWithMeta 가 호출되어야 한다")
	assert.Equal(t, 0, store.setCalls, "일반 Set 은 호출되지 않아야 한다")
	call := store.metaCalls[0]
	assert.Equal(t, "temp", call.key)
	assert.Equal(t, 21.5, call.value)
	assert.Equal(t, "float", call.opts.DataType)
	assert.Empty(t, call.opts.Tags)
}

// tags 지정 시 SetWithMeta 가 태그를 전달하는지 검증한다.
func TestStoreWriteNode_Process_Tags_UsesSetWithMeta(t *testing.T) {
	def := flow.NodeDef{ID: "swm6", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockMetaStore()
	require.NoError(t, n.Configure(map[string]any{
		"_store":       store,
		"key_template": "hum",
		"value_key":    "$.payload.h",
		"tags":         map[string]any{"room": "bath"},
	}))
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{"h": float64(55)})
	msg := message.New(message.WithPayload(payload))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	require.Len(t, store.metaCalls, 1)
	assert.Equal(t, "bath", store.metaCalls[0].opts.Tags["room"])
}

// data_type + tags + TTL 동시 지정 + 동적 키(key_template) 조합을 검증한다.
func TestStoreWriteNode_Process_DynamicKey_DataType_Tags_TTL(t *testing.T) {
	def := flow.NodeDef{ID: "swm7", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockMetaStore()
	require.NoError(t, n.Configure(map[string]any{
		"_store":       store,
		"key_template": "{loc}:{sensor}",
		"value_key":    "$.payload.v",
		"data_type":    "int",
		"tags":         map[string]any{"unit": "ppm"},
		"ttl":          "10m",
	}))
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"loc":    "indoor",
		"sensor": "co2",
		"v":      420,
	})
	msg := message.New(message.WithPayload(payload))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	require.Len(t, store.metaCalls, 1)
	call := store.metaCalls[0]
	assert.Equal(t, "indoor:co2", call.key, "동적 키가 보간되어야 한다")
	assert.Equal(t, 420, call.value)
	assert.Equal(t, "int", call.opts.DataType)
	assert.Equal(t, "ppm", call.opts.Tags["unit"])
	assert.Equal(t, 10*time.Minute, call.opts.TTL)
}

// --- PRESERVE: 폴백 동작 ---

// data_type/tags 미설정 시 SetWithMeta 가 아닌 기존 Set 경로를 쓰는지 검증한다.
func TestStoreWriteNode_Process_NoMeta_FallsBackToSet(t *testing.T) {
	def := flow.NodeDef{ID: "swm8", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockMetaStore()
	require.NoError(t, n.Configure(map[string]any{
		"_store":       store,
		"key_template": "k",
		"value_key":    "$.payload.v",
	}))
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{"v": float64(1)})
	msg := message.New(message.WithPayload(payload))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	assert.Empty(t, store.metaCalls, "메타 미설정 시 SetWithMeta 를 쓰지 않아야 한다")
	assert.Equal(t, 1, store.setCalls, "일반 Set 으로 폴백해야 한다")
}

// data_type/tags 미설정 + TTL 설정 시 SetWithTTL 폴백을 검증한다.
func TestStoreWriteNode_Process_NoMeta_TTL_FallsBackToSetWithTTL(t *testing.T) {
	def := flow.NodeDef{ID: "swm9", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockMetaStore()
	require.NoError(t, n.Configure(map[string]any{
		"_store":       store,
		"key_template": "k",
		"value_key":    "$.payload.v",
		"ttl":          "1m",
	}))
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{"v": float64(1)})
	msg := message.New(message.WithPayload(payload))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	assert.Empty(t, store.metaCalls)
	assert.Equal(t, 1, store.setTTLCalls, "TTL 설정 시 SetWithTTL 로 폴백해야 한다")
}

// store 가 StoreMetaWriter 를 구현하지 않으면(기존 mockStoreWriter) data_type 이 설정돼도
// 기존 Set 경로로 폴백하는지 검증한다 (하위 호환 — PRESERVE).
func TestStoreWriteNode_Process_NonMetaStore_FallsBack(t *testing.T) {
	def := flow.NodeDef{ID: "swm10", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockStore() // StoreMetaWriter 미구현
	require.NoError(t, n.Configure(map[string]any{
		"_store":       store,
		"key_template": "k",
		"value_key":    "$.payload.v",
		"data_type":    "float", // 설정됐지만 store 가 메타를 지원하지 않음
	}))
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{"v": float64(7)})
	msg := message.New(message.WithPayload(payload))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	// 일반 Set 으로 값이 기록되어야 한다.
	val, ok, getErr := store.Get(context.Background(), "k")
	require.NoError(t, getErr)
	assert.True(t, ok)
	assert.Equal(t, float64(7), val)
}
