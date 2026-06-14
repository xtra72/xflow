// store_write_key_mappings_test.go 는 store-write 노드의 다중 키(key_mappings) 기록
// 기능을 검증한다.
//
// 커버리지:
//   - Configure: key_mappings 파싱 + value 규칙 검증($. 경로/빈값) + 잘못된 value 거부
//   - Configure: key_template/key_mappings 둘 다 없으면 거부 (키 없음)
//   - Process: key_mappings 다중 키 기록
//   - Process: key_template + key_mappings 동시 기록 (1+개)
//   - Process: 키 템플릿 보간({...})
//   - Process: value 빈값 = 전체 payload 저장
//   - Process: 공유 메타(data_type/metric_type/tags)를 모든 키에 동일 적용
//   - Process: 키 해석 실패 시 에러
//
// 이 파일은 store_write_test.go 의 mockStoreWriter / newMockStore 와
// store_write_meta_test.go 의 mockMetaStore / metaCall / newMockMetaStore 를 재사용한다.
package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// --- Configure: key_mappings 파싱/검증 ---

// key_mappings 가 map[string]string 으로 정규화되는지 검증한다.
func TestStoreWriteNode_Configure_KeyMappings(t *testing.T) {
	def := flow.NodeDef{ID: "km1", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_mappings": map[string]any{
			"{$.metadata.dev}:temp": "$.payload.temperature",
			"{$.metadata.dev}:hum":  "$.payload.humidity",
		},
	})
	require.NoError(t, err)

	sw := n.(*StoreWriteNode)
	require.Len(t, sw.keyMappings, 2)
	assert.Equal(t, "$.payload.temperature", sw.keyMappings["{$.metadata.dev}:temp"])
	assert.Equal(t, "$.payload.humidity", sw.keyMappings["{$.metadata.dev}:hum"])
}

// value 가 $. 경로도 빈 문자열도 아니면 거부한다 (value_key 와 동일 규칙).
func TestStoreWriteNode_Configure_KeyMappings_InvalidValue(t *testing.T) {
	def := flow.NodeDef{ID: "km2", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_mappings": map[string]any{
			"k1": "payload.temperature", // $. prefix 없음 → 거부
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a $.-path")
}

// value 가 문자열이 아니면 거부한다.
func TestStoreWriteNode_Configure_KeyMappings_NonStringValue(t *testing.T) {
	def := flow.NodeDef{ID: "km3", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"key_mappings": map[string]any{
			"k1": 123, // 문자열 아님 → 거부
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

// key_template 과 key_mappings 가 둘 다 없으면 기록할 키가 없으므로 거부한다.
func TestStoreWriteNode_Configure_NoKeySource(t *testing.T) {
	def := flow.NodeDef{ID: "km4", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"value_key": "$.payload.v", // 키 소스 없음
		"data_type": "float",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key_template 또는 key_mappings")
}

// --- Process: 다중 키 기록 ---

// key_mappings 만으로 여러 (키,값)을 한 메시지에서 기록하는지 검증한다.
func TestStoreWriteNode_Process_KeyMappings_MultiWrite(t *testing.T) {
	def := flow.NodeDef{ID: "km5", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockStore()
	err = n.Configure(map[string]any{
		"_store": store,
		"key_mappings": map[string]any{
			"sensor:temp": "$.payload.temperature",
			"sensor:hum":  "$.payload.humidity",
		},
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"temperature": float64(21.5),
		"humidity":    float64(60),
	})
	results, err := n.Process(context.Background(), message.New(message.WithPayload(payload)))
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 두 키가 각각 올바른 값으로 기록되었는지 확인.
	tv, ok, _ := store.Get(context.Background(), "sensor:temp")
	assert.True(t, ok)
	assert.Equal(t, float64(21.5), tv)

	hv, ok, _ := store.Get(context.Background(), "sensor:hum")
	assert.True(t, ok)
	assert.Equal(t, float64(60), hv)
}

// key_template + key_mappings 동시 지정 시 총 (1 + len(key_mappings))개를 기록하는지 검증한다.
func TestStoreWriteNode_Process_KeyTemplateAndKeyMappings(t *testing.T) {
	def := flow.NodeDef{ID: "km6", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockStore()
	err = n.Configure(map[string]any{
		"_store":       store,
		"key_template": "main",
		"value_key":    "$.payload.temperature",
		"key_mappings": map[string]any{
			"extra:hum": "$.payload.humidity",
		},
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"temperature": float64(21.5),
		"humidity":    float64(60),
	})
	_, err = n.Process(context.Background(), message.New(message.WithPayload(payload)))
	require.NoError(t, err)

	// 단일 키 (main) + key_mappings 키 (extra:hum) 모두 기록.
	mv, ok, _ := store.Get(context.Background(), "main")
	assert.True(t, ok)
	assert.Equal(t, float64(21.5), mv)

	hv, ok, _ := store.Get(context.Background(), "extra:hum")
	assert.True(t, ok)
	assert.Equal(t, float64(60), hv)
}

// 키 템플릿 {...} 보간이 key_mappings 키에도 적용되는지 검증한다.
func TestStoreWriteNode_Process_KeyMappings_KeyInterpolation(t *testing.T) {
	def := flow.NodeDef{ID: "km7", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockStore()
	err = n.Configure(map[string]any{
		"_store": store,
		"key_mappings": map[string]any{
			"{$.metadata.dev}:temp": "$.payload.temperature",
		},
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": float64(19.0),
	})))
	msg.Metadata().Set("dev", "idu-7")

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	// 키가 "idu-7:temp"로 보간되었는지 확인.
	v, ok, _ := store.Get(context.Background(), "idu-7:temp")
	assert.True(t, ok)
	assert.Equal(t, float64(19.0), v)
}

// key_mappings value 가 빈 문자열이면 전체 payload 를 저장하는지 검증한다.
func TestStoreWriteNode_Process_KeyMappings_EmptyValueWholePayload(t *testing.T) {
	def := flow.NodeDef{ID: "km8", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockStore()
	err = n.Configure(map[string]any{
		"_store": store,
		"key_mappings": map[string]any{
			"snapshot": "", // 빈 값 → 전체 payload
		},
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"a": float64(1),
		"b": float64(2),
	})
	_, err = n.Process(context.Background(), message.New(message.WithPayload(payload)))
	require.NoError(t, err)

	v, ok, _ := store.Get(context.Background(), "snapshot")
	require.True(t, ok)
	m, isMap := v.(map[string]any)
	require.True(t, isMap)
	assert.Equal(t, float64(1), m["a"])
	assert.Equal(t, float64(2), m["b"])
}

// 키 해석 실패(존재하지 않는 보간 필드) 시 에러를 반환하는지 검증한다.
func TestStoreWriteNode_Process_KeyMappings_KeyResolveError(t *testing.T) {
	def := flow.NodeDef{ID: "km9", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockStore()
	err = n.Configure(map[string]any{
		"_store": store,
		"key_mappings": map[string]any{
			"{$.metadata.missing}:temp": "$.payload.temperature",
		},
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{"temperature": float64(1)})
	_, err = n.Process(context.Background(), message.New(message.WithPayload(payload)))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key_mappings")
}

// --- Process: 공유 메타 적용 ---

// data_type/metric_type/tags 공유 메타가 key_mappings 의 모든 키에 동일 적용되는지 검증한다.
func TestStoreWriteNode_Process_KeyMappings_SharedMeta(t *testing.T) {
	def := flow.NodeDef{ID: "km10", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newMockMetaStore()
	err = n.Configure(map[string]any{
		"_store":      store,
		"data_type":   "float",
		"metric_type": "temperature",
		"tags": map[string]any{
			"room": "kitchen",
		},
		"key_mappings": map[string]any{
			"sensor:temp": "$.payload.temperature",
			"sensor:hum":  "$.payload.humidity",
		},
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"temperature": float64(21.5),
		"humidity":    float64(60),
	})
	_, err = n.Process(context.Background(), message.New(message.WithPayload(payload)))
	require.NoError(t, err)

	// 두 키 모두 SetWithMeta 로 기록되었고, 동일한 공유 메타가 적용됐는지 확인.
	require.Len(t, store.metaCalls, 2)
	for _, c := range store.metaCalls {
		assert.Equal(t, "float", c.opts.DataType)
		assert.Equal(t, "temperature", c.opts.MetricType)
		assert.Equal(t, map[string]string{"room": "kitchen"}, c.opts.Tags)
	}

	// 두 키가 모두 기록되었는지 확인 (순서 무관).
	keys := map[string]bool{}
	for _, c := range store.metaCalls {
		keys[c.key] = true
	}
	assert.True(t, keys["sensor:temp"])
	assert.True(t, keys["sensor:hum"])
}
