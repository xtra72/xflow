package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// newMetricsNode 는 key_template + metrics 배열로 설정된 store-write 노드와 카운팅 스토어를 만든다.
func newMetricsNode(t *testing.T, metrics []any, extra map[string]any) (*StoreWriteNode, *countingStore) {
	t.Helper()
	def := flow.NodeDef{ID: "sw-metrics", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newCountingStore()
	cfg := map[string]any{
		"_store":       store,
		"key_template": "dev1",
		"metrics":      metrics,
	}
	for k, v := range extra {
		cfg[k] = v
	}
	require.NoError(t, n.Configure(cfg))
	require.NoError(t, n.Init(context.Background()))
	return n.(*StoreWriteNode), store
}

// sendMetrics 는 payload 맵 + timestamp 메시지를 노드에 흘려보낸다.
func sendMetrics(t *testing.T, n *StoreWriteNode, payload map[string]any, ts time.Time) {
	t.Helper()
	p := message.NewPayload(payload)
	msg := message.New(message.WithPayload(p), message.WithTimestamp(ts))
	_, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
}

// 다중 메트릭: 한 메시지가 같은 키에 metric_type 별 별도 시리즈로 기록된다.
func TestStoreWriteNode_Metrics_MultiWrite(t *testing.T) {
	n, store := newMetricsNode(t, []any{
		map[string]any{"metric_type": "temperature", "value_key": "$.payload.temp", "data_type": "float"},
		map[string]any{"metric_type": "humidity", "value_key": "$.payload.hum", "data_type": "float"},
	}, nil)

	base := time.UnixMilli(1_000_000)
	sendMetrics(t, n, map[string]any{"temp": 21.5, "hum": 55.0}, base)

	assert.Equal(t, 2, store.countForKey("dev1"))
	assert.Equal(t, 1, store.countForMetric("temperature"))
	assert.Equal(t, 1, store.countForMetric("humidity"))

	tw, ok := store.lastForMetric("temperature")
	require.True(t, ok)
	assert.Equal(t, 21.5, tw.value)
	assert.Equal(t, "float", tw.dataType)

	hw, ok := store.lastForMetric("humidity")
	require.True(t, ok)
	assert.Equal(t, 55.0, hw.value)
}

// 메트릭별 dead-band 가 독립적으로 동작한다.
func TestStoreWriteNode_Metrics_PerMetricDeadband(t *testing.T) {
	n, store := newMetricsNode(t, []any{
		map[string]any{"metric_type": "temperature", "value_key": "$.payload.temp", "min_interval": "1m", "min_change": 0.5},
		map[string]any{"metric_type": "humidity", "value_key": "$.payload.hum", "min_interval": "1m", "min_change": 5.0},
	}, nil)

	base := time.UnixMilli(1_000_000)
	// t0: 둘 다 최초 저장.
	sendMetrics(t, n, map[string]any{"temp": 20.0, "hum": 50.0}, base)
	// t0+10s: temp +0.3(<=0.5 → 억제), hum +3(<=5 → 억제).
	sendMetrics(t, n, map[string]any{"temp": 20.3, "hum": 53.0}, base.Add(10*time.Second))
	// t0+20s: temp +0.6 vs 20.0(>0.5 → 저장), hum +3 vs 50(<=5 → 억제).
	sendMetrics(t, n, map[string]any{"temp": 20.6, "hum": 53.0}, base.Add(20*time.Second))

	assert.Equal(t, 2, store.countForMetric("temperature"))
	assert.Equal(t, 1, store.countForMetric("humidity"))
}

// value_key 미지정 시 기본값 $.payload.value 를 읽는다.
func TestStoreWriteNode_Metrics_DefaultValueKey(t *testing.T) {
	n, store := newMetricsNode(t, []any{
		map[string]any{"metric_type": "power"},
	}, nil)

	base := time.UnixMilli(1_000_000)
	sendMetrics(t, n, map[string]any{"value": 42.0, "other": 7.0}, base)

	w, ok := store.lastForMetric("power")
	require.True(t, ok)
	assert.Equal(t, 42.0, w.value)
}

// metrics 는 key_template 이 없으면 거부된다.
func TestStoreWriteNode_Metrics_RequiresKeyTemplate(t *testing.T) {
	def := flow.NodeDef{ID: "sw-bad", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)
	err = n.Configure(map[string]any{
		"key_mappings": map[string]any{"k": "$.payload.v"},
		"metrics": []any{
			map[string]any{"metric_type": "temperature"},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key_template")
}

// metrics 항목의 잘못된 설정은 거부된다.
func TestStoreWriteNode_Metrics_InvalidSpec(t *testing.T) {
	def := flow.NodeDef{ID: "sw-bad2", Type: "store-write"}
	cases := []struct {
		name   string
		metric map[string]any
	}{
		{"bad_data_type", map[string]any{"metric_type": "t", "data_type": "decimal"}},
		{"bad_value_key", map[string]any{"metric_type": "t", "value_key": "payload.temp"}},
		{"bad_metric_type", map[string]any{"metric_type": "has space"}},
		{"neg_min_change", map[string]any{"metric_type": "t", "min_change": -1.0}},
		{"bad_min_interval", map[string]any{"metric_type": "t", "min_interval": "nope"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, err := NewStoreWriteNode(def)
			require.NoError(t, err)
			err = n.Configure(map[string]any{
				"key_template": "k",
				"metrics":      []any{tc.metric},
			})
			require.Error(t, err)
		})
	}
}
