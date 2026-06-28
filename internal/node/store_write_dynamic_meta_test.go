// store_write_dynamic_meta_test.go 는 store-write 노드의 동적 metric_type / tags
// 해석 기능을 검증한다 (리터럴 또는 `$.` 경로).
//
// 커버리지:
//   - metric_type 리터럴 / $.payload / $.metadata 경로 해석
//   - metric_type 해석 실패·정규식 위반 시 생략 (계속 진행)
//   - metric_type 리터럴 정규식 위반 시 Configure 거부
//   - metric_type 미설정 폴백 (PRESERVE)
//   - tags 리터럴 + $. 경로 혼재 해석
//   - tags 부분 해석 실패 시 해당 태그만 생략
//   - tags 숫자/불리언 경로 값 문자열화
//   - data_type + metric_type + tags 조합
//
// 이 파일은 store_write_meta_test.go 의 mockMetaStore / metaCall / newMockMetaStore 를 재사용한다.
package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// newConfiguredStoreWriteNode 는 주어진 config 로 Configure + Init 까지 마친
// StoreWriteNode 를 반환하는 테스트 헬퍼이다. _store 는 호출자가 config 에 포함한다.
func newConfiguredStoreWriteNode(t *testing.T, config map[string]any) *StoreWriteNode {
	t.Helper()
	def := flow.NodeDef{ID: "sw-dyn", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)
	require.NoError(t, n.Configure(config))
	require.NoError(t, n.Init(context.Background()))
	return n.(*StoreWriteNode)
}

// --- metric_type: 리터럴 ---

func TestStoreWriteNode_MetricType_Literal(t *testing.T) {
	store := newMockMetaStore()
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       store,
		"key_template": "k",
		"value_key":    "$.payload.v",
		"metric_type":  "temperature",
	})

	payload := message.NewPayload(map[string]any{"v": float64(21.5)})
	_, err := n.Process(context.Background(), message.New(message.WithPayload(payload)))
	require.NoError(t, err)

	require.Len(t, store.metaCalls, 1)
	assert.Equal(t, "temperature", store.metaCalls[0].opts.MetricType)
}

// --- metric_type: $.payload 경로 해석 ---

func TestStoreWriteNode_MetricType_PayloadPath(t *testing.T) {
	store := newMockMetaStore()
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       store,
		"key_template": "k",
		"value_key":    "$.payload.v",
		"metric_type":  "$.payload.metric",
	})

	payload := message.NewPayload(map[string]any{
		"v":      float64(1),
		"metric": "humidity",
	})
	_, err := n.Process(context.Background(), message.New(message.WithPayload(payload)))
	require.NoError(t, err)

	require.Len(t, store.metaCalls, 1)
	assert.Equal(t, "humidity", store.metaCalls[0].opts.MetricType)
}

// --- metric_type: $.metadata 경로 해석 ---

func TestStoreWriteNode_MetricType_MetadataPath(t *testing.T) {
	store := newMockMetaStore()
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       store,
		"key_template": "k",
		"value_key":    "$.payload.v",
		"metric_type":  "$.metadata.metric",
	})

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"v": float64(1)})))
	msg.Metadata().Set("metric", "pressure")

	_, err := n.Process(context.Background(), msg)
	require.NoError(t, err)

	require.Len(t, store.metaCalls, 1)
	assert.Equal(t, "pressure", store.metaCalls[0].opts.MetricType)
}

// --- metric_type: $. 경로 해석 실패 → metric 생략 (계속 진행) ---

func TestStoreWriteNode_MetricType_ResolveFail_Skipped(t *testing.T) {
	store := newMockMetaStore()
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       store,
		"key_template": "k",
		"value_key":    "$.payload.v",
		"metric_type":  "$.payload.missing",
	})

	payload := message.NewPayload(map[string]any{"v": float64(1)})
	_, err := n.Process(context.Background(), message.New(message.WithPayload(payload)))
	// 해석 실패는 에러가 아니라 metric 생략 → Process 는 성공해야 한다.
	require.NoError(t, err)

	// metric/tags/data_type 모두 없으므로 일반 Set 폴백 경로가 사용된다.
	assert.Empty(t, store.metaCalls)
	assert.Equal(t, 1, store.setCalls)
}

// --- metric_type: $. 경로 해석 결과가 정규식 위반 → 생략 ---

func TestStoreWriteNode_MetricType_InvalidResolved_Skipped(t *testing.T) {
	store := newMockMetaStore()
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       store,
		"key_template": "k",
		"value_key":    "$.payload.v",
		"metric_type":  "$.payload.metric",
	})

	// 공백 포함 → ^[a-zA-Z0-9_-]+$ 위반.
	payload := message.NewPayload(map[string]any{
		"v":      float64(1),
		"metric": "bad metric!",
	})
	_, err := n.Process(context.Background(), message.New(message.WithPayload(payload)))
	require.NoError(t, err)

	// 위반으로 metric 생략 → 다른 메타도 없으므로 일반 Set 폴백.
	assert.Empty(t, store.metaCalls)
	assert.Equal(t, 1, store.setCalls)
}

// --- metric_type: 리터럴 정규식 위반 → Configure 에서 거부 ---

func TestStoreWriteNode_MetricType_LiteralInvalid_ConfigError(t *testing.T) {
	def := flow.NodeDef{ID: "sw-dyn", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"_store":       newMockMetaStore(),
		"key_template": "k",
		"metric_type":  "bad metric!",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metric_type")
}

// --- metric_type: 미설정 → 폴백 (기존 동작 보존) ---

func TestStoreWriteNode_MetricType_Unset_Fallback(t *testing.T) {
	store := newMockMetaStore()
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       store,
		"key_template": "k",
		"value_key":    "$.payload.v",
	})

	payload := message.NewPayload(map[string]any{"v": float64(1)})
	_, err := n.Process(context.Background(), message.New(message.WithPayload(payload)))
	require.NoError(t, err)

	assert.Empty(t, store.metaCalls)
	assert.Equal(t, 1, store.setCalls)
}

// --- tags: 리터럴 + $. 경로 혼재 해석 ---

func TestStoreWriteNode_Tags_LiteralAndPathMixed(t *testing.T) {
	store := newMockMetaStore()
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       store,
		"key_template": "k",
		"value_key":    "$.payload.v",
		"tags": map[string]any{
			"unit":     "celsius",           // 리터럴
			"device":   "$.metadata.dev_id", // metadata 경로
			"location": "$.payload.loc",     // payload 경로
		},
	})

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"v":   float64(1),
		"loc": "room-a",
	})))
	msg.Metadata().Set("dev_id", "sensor-7")

	_, err := n.Process(context.Background(), msg)
	require.NoError(t, err)

	require.Len(t, store.metaCalls, 1)
	assert.Equal(t, map[string]string{
		"unit":     "celsius",
		"device":   "sensor-7",
		"location": "room-a",
	}, store.metaCalls[0].opts.Tags)
}

// --- tags: $. 경로 해석 실패 → 해당 태그만 생략, 나머지 유지 ---

func TestStoreWriteNode_Tags_PartialResolveFail(t *testing.T) {
	store := newMockMetaStore()
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       store,
		"key_template": "k",
		"value_key":    "$.payload.v",
		"tags": map[string]any{
			"unit":   "celsius",           // 리터럴 (유지)
			"device": "$.payload.missing", // 해석 실패 (생략)
		},
	})

	payload := message.NewPayload(map[string]any{"v": float64(1)})
	_, err := n.Process(context.Background(), message.New(message.WithPayload(payload)))
	require.NoError(t, err)

	require.Len(t, store.metaCalls, 1)
	assert.Equal(t, map[string]string{"unit": "celsius"}, store.metaCalls[0].opts.Tags)
}

// --- tags: 숫자/불리언 경로 값도 문자열로 변환 ---

func TestStoreWriteNode_Tags_NonStringPathValuesStringified(t *testing.T) {
	store := newMockMetaStore()
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       store,
		"key_template": "k",
		"value_key":    "$.payload.v",
		"tags": map[string]any{
			"floor":  "$.payload.floor",  // 숫자
			"active": "$.payload.active", // 불리언
		},
	})

	payload := message.NewPayload(map[string]any{
		"v":      float64(1),
		"floor":  float64(3),
		"active": true,
	})
	_, err := n.Process(context.Background(), message.New(message.WithPayload(payload)))
	require.NoError(t, err)

	require.Len(t, store.metaCalls, 1)
	tags := store.metaCalls[0].opts.Tags
	assert.Equal(t, "3", tags["floor"])
	assert.Equal(t, "true", tags["active"])
}

// --- 조합: data_type + metric_type + tags 함께 ---

func TestStoreWriteNode_DataTypeMetricTags_Combined(t *testing.T) {
	store := newMockMetaStore()
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       store,
		"key_template": "k",
		"value_key":    "$.payload.v",
		"data_type":    "float",
		"metric_type":  "$.payload.metric",
		"tags": map[string]any{
			"unit": "celsius",
		},
	})

	payload := message.NewPayload(map[string]any{
		"v":      float64(22.2),
		"metric": "temperature",
	})
	_, err := n.Process(context.Background(), message.New(message.WithPayload(payload)))
	require.NoError(t, err)

	require.Len(t, store.metaCalls, 1)
	opts := store.metaCalls[0].opts
	assert.Equal(t, "float", opts.DataType)
	assert.Equal(t, "temperature", opts.MetricType)
	assert.Equal(t, map[string]string{"unit": "celsius"}, opts.Tags)
}
