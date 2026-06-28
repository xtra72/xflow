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

// newSnapshotNode 는 한 메시지에서 power/temperature 두 메트릭을 기록하며 각 메트릭에
// dead-band(min_interval/min_change)를 건 store-write 노드를 만든다.
// snapshot=true 면 message_type="device_state.report" 메시지를 스냅샷으로 식별해 dead-band 를 우회한다.
func newSnapshotNode(t *testing.T, snapshot bool) (*StoreWriteNode, *countingStore) {
	t.Helper()
	def := flow.NodeDef{ID: "sw-snap", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newCountingStore()
	cfg := map[string]any{
		"_store":       store,
		"key_template": "dev",
		"tags":         map[string]any{"message_type": "$.payload.message_type"},
		"metrics": []any{
			map[string]any{
				"metric_type":  "power",
				"value_key":    "$.payload.power",
				"min_interval": "1m",
				"min_change":   100.0,
			},
			map[string]any{
				"metric_type":  "temperature",
				"value_key":    "$.payload.temperature",
				"min_interval": "1m",
				"min_change":   100.0,
			},
		},
	}
	if snapshot {
		cfg["snapshot_value_path"] = "$.payload.message_type"
		cfg["snapshot_values"] = []any{"device_state.report"}
	}
	require.NoError(t, n.Configure(cfg))
	require.NoError(t, n.Init(context.Background()))
	return n.(*StoreWriteNode), store
}

// procState 는 power/temperature/message_type 페이로드 메시지를 흘려보낸다.
func procState(t *testing.T, n *StoreWriteNode, power any, temp float64, mt string, ts time.Time) {
	t.Helper()
	p := message.NewPayload(map[string]any{
		"power":        power,
		"temperature":  temp,
		"message_type": mt,
	})
	msg := message.New(message.WithPayload(p), message.WithTimestamp(ts))
	res, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, res, 1)
}

// 버그 재현: 스냅샷 우회가 없으면 device_state.report 라도 dead-band 가 시리즈별로 적용되어
// 자주 바뀌는 power 만 매번 저장되고 값이 안정적인 temperature 는 억제된다 → 카운트 불일치.
func TestStoreWriteNode_Snapshot_BugRepro_UnequalCountsWithoutBypass(t *testing.T) {
	n, store := newSnapshotNode(t, false)
	base := time.UnixMilli(1_000_000)
	// 3회 report: power 토글(false/true/false), temperature 18.0 고정. 모두 min_interval 이내.
	procState(t, n, false, 18.0, "device_state.report", base)
	procState(t, n, true, 18.0, "device_state.report", base.Add(time.Second))
	procState(t, n, false, 18.0, "device_state.report", base.Add(2*time.Second))

	// power: 매번 값이 바뀌어 유의미 변화 → 3회 저장.
	assert.Equal(t, 3, store.countForMetric("power"))
	// temperature: 값 고정 + min_interval 이내 → 최초 1회만 저장 (버그: report 인데도 억제됨).
	assert.Equal(t, 1, store.countForMetric("temperature"))
}

// 수정: 스냅샷(device_state.report) 메시지는 dead-band 를 우회하여 모든 메트릭을 매번 저장한다.
// → power 와 temperature 카운트가 report 횟수와 동일하게 일치한다.
func TestStoreWriteNode_Snapshot_BypassEqualizesCounts(t *testing.T) {
	n, store := newSnapshotNode(t, true)
	base := time.UnixMilli(1_000_000)
	procState(t, n, false, 18.0, "device_state.report", base)
	procState(t, n, true, 18.0, "device_state.report", base.Add(time.Second))
	procState(t, n, false, 18.0, "device_state.report", base.Add(2*time.Second))

	assert.Equal(t, 3, store.countForMetric("power"))
	assert.Equal(t, 3, store.countForMetric("temperature"))
}

// 스냅샷이 아닌 메시지(device_state.change)는 기존 dead-band 가 그대로 적용된다.
func TestStoreWriteNode_Snapshot_NonSnapshotStillDeadbanded(t *testing.T) {
	n, store := newSnapshotNode(t, true)
	base := time.UnixMilli(1_000_000)
	// change 메시지: temperature 고정 → min_interval 이내면 최초 1회만 저장.
	procState(t, n, false, 18.0, "device_state.change", base)
	procState(t, n, false, 18.0, "device_state.change", base.Add(time.Second))
	procState(t, n, false, 18.0, "device_state.change", base.Add(2*time.Second))

	assert.Equal(t, 1, store.countForMetric("temperature"))
	// power 도 값 고정(false)이라 억제 → 1회.
	assert.Equal(t, 1, store.countForMetric("power"))
}

// 스냅샷은 min_interval 과 무관하게 매 메시지 저장한다(무제한).
func TestStoreWriteNode_Snapshot_IgnoresMinInterval(t *testing.T) {
	n, store := newSnapshotNode(t, true)
	base := time.UnixMilli(1_000_000)
	// min_interval=1m 보다 훨씬 짧은 간격으로 5회 report, 값 완전 고정.
	for i := 0; i < 5; i++ {
		procState(t, n, true, 22.0, "device_state.report", base.Add(time.Duration(i)*time.Millisecond))
	}
	assert.Equal(t, 5, store.countForMetric("power"))
	assert.Equal(t, 5, store.countForMetric("temperature"))
}

// 설정 검증: snapshot_value_path 와 snapshot_values 는 함께 지정해야 한다.
func TestStoreWriteNode_Snapshot_ConfigValidation(t *testing.T) {
	def := flow.NodeDef{ID: "sw-snap-bad", Type: "store-write"}
	cases := []struct {
		name string
		cfg  map[string]any
	}{
		{
			"path_without_values",
			map[string]any{"key_template": "k", "snapshot_value_path": "$.payload.message_type"},
		},
		{
			"values_without_path",
			map[string]any{"key_template": "k", "snapshot_values": []any{"device_state.report"}},
		},
		{
			"path_not_dollar",
			map[string]any{"key_template": "k", "snapshot_value_path": "message_type", "snapshot_values": []any{"x"}},
		},
		{
			"empty_values",
			map[string]any{"key_template": "k", "snapshot_value_path": "$.payload.message_type", "snapshot_values": []any{}},
		},
		{
			"non_string_value",
			map[string]any{"key_template": "k", "snapshot_value_path": "$.payload.message_type", "snapshot_values": []any{123}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, err := NewStoreWriteNode(def)
			require.NoError(t, err)
			err = n.Configure(tc.cfg)
			require.Error(t, err)
		})
	}
}

// 정상 설정은 통과하고 필드가 채워진다.
func TestStoreWriteNode_Snapshot_ConfigValid(t *testing.T) {
	n, _ := newSnapshotNode(t, true)
	assert.Equal(t, "$.payload.message_type", n.snapshotPath)
	assert.True(t, n.snapshotValues["device_state.report"])
}
