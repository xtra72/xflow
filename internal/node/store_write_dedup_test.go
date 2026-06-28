package node

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// countingStore 는 dead-band(미세변화 억제) 테스트용 StoreWriter 구현체이다.
// mockStoreWriter 는 키별 map 으로 덮어쓰기 때문에 "몇 번 기록됐는지"를 셀 수 없다.
// countingStore 는 모든 쓰기를 append 로 기록하여 억제 여부를 검증할 수 있게 한다.
// StoreMetaWriter 도 구현하여 metric_type/tags 가 있는 메타 경로도 동일하게 집계한다.
type countingStore struct {
	mu     sync.Mutex
	writes []storeWriteRecord
}

type storeWriteRecord struct {
	key      string
	value    any
	metric   string
	dataType string
}

func newCountingStore() *countingStore {
	return &countingStore{}
}

func (c *countingStore) Set(_ context.Context, key string, value any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes = append(c.writes, storeWriteRecord{key: key, value: value})
	return nil
}

func (c *countingStore) SetWithTTL(_ context.Context, key string, value any, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes = append(c.writes, storeWriteRecord{key: key, value: value})
	return nil
}

func (c *countingStore) SetWithMeta(_ context.Context, key string, value any, opts system.StoreWriteMeta) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes = append(c.writes, storeWriteRecord{key: key, value: value, metric: opts.MetricType, dataType: opts.DataType})
	return nil
}

func (c *countingStore) countForKey(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, w := range c.writes {
		if w.key == key {
			n++
		}
	}
	return n
}

// countForMetric 은 특정 metric_type 으로 기록된 횟수를 센다.
func (c *countingStore) countForMetric(metric string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, w := range c.writes {
		if w.metric == metric {
			n++
		}
	}
	return n
}

// lastForMetric 은 특정 metric_type 의 마지막 기록을 반환한다.
func (c *countingStore) lastForMetric(metric string) (storeWriteRecord, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.writes) - 1; i >= 0; i-- {
		if c.writes[i].metric == metric {
			return c.writes[i], true
		}
	}
	return storeWriteRecord{}, false
}

func (c *countingStore) total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.writes)
}

// newDedupNode 는 key_template="k", value_key="$.payload.v" 로 설정된
// dead-band 테스트용 store-write 노드와 카운팅 스토어를 생성한다.
func newDedupNode(t *testing.T, extra map[string]any) (*StoreWriteNode, *countingStore) {
	t.Helper()
	def := flow.NodeDef{ID: "sw-dedup", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newCountingStore()
	cfg := map[string]any{
		"_store":       store,
		"key_template": "k",
		"value_key":    "$.payload.v",
	}
	for k, v := range extra {
		cfg[k] = v
	}
	require.NoError(t, n.Configure(cfg))
	require.NoError(t, n.Init(context.Background()))
	return n.(*StoreWriteNode), store
}

// procAt 는 payload.v=value, timestamp=ts 인 메시지를 노드에 흘려보내고
// pass-through 결과가 항상 1건임을 검증한다 (억제 여부와 무관).
func procAt(t *testing.T, n *StoreWriteNode, value any, ts time.Time) {
	t.Helper()
	p := message.NewPayload(map[string]any{"v": value})
	msg := message.New(message.WithPayload(p), message.WithTimestamp(ts))
	res, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, res, 1, "store-write 는 억제 여부와 무관하게 메시지를 pass-through 해야 한다")
}

func TestStoreWriteNode_Configure_Dedup(t *testing.T) {
	n, _ := newDedupNode(t, map[string]any{
		"min_interval":       "1m",
		"min_change":         0.5,
		"min_change_percent": 10.0,
	})
	assert.Equal(t, time.Minute, n.minInterval)
	assert.True(t, n.hasMinChange)
	assert.Equal(t, 0.5, n.minChange)
	assert.True(t, n.hasMinChangePercent)
	assert.Equal(t, 10.0, n.minChangePercent)
}

func TestStoreWriteNode_Configure_Dedup_Invalid(t *testing.T) {
	def := flow.NodeDef{ID: "sw-bad", Type: "store-write"}

	cases := []struct {
		name string
		cfg  map[string]any
	}{
		{"bad_interval", map[string]any{"key_template": "k", "min_interval": "nope"}},
		{"neg_change", map[string]any{"key_template": "k", "min_change": -1.0}},
		{"neg_percent", map[string]any{"key_template": "k", "min_change_percent": -5.0}},
		{"non_number_change", map[string]any{"key_template": "k", "min_change": "x"}},
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

// 기능 미설정 시(min_interval 없음) 동일 값이어도 항상 저장 (완전 하위호환).
func TestStoreWriteNode_Dedup_DisabledByDefault(t *testing.T) {
	n, store := newDedupNode(t, nil)
	base := time.UnixMilli(1_000_000)
	procAt(t, n, 20.0, base)
	procAt(t, n, 20.0, base.Add(time.Second))
	procAt(t, n, 20.0, base.Add(2*time.Second))
	assert.Equal(t, 3, store.countForKey("k"))
}

// 절대값 dead-band: 구간 내 작은 변화는 억제, 누적 변화가 임계 초과하면 저장.
func TestStoreWriteNode_Dedup_AbsoluteThreshold(t *testing.T) {
	n, store := newDedupNode(t, map[string]any{
		"min_interval": "1m",
		"min_change":   0.5,
	})
	base := time.UnixMilli(1_000_000)
	procAt(t, n, 20.0, base)                     // store (1) last=20.0
	procAt(t, n, 20.3, base.Add(10*time.Second)) // delta 0.3 <= 0.5 → suppress
	procAt(t, n, 20.6, base.Add(20*time.Second)) // delta 0.6 > 0.5 (vs 20.0) → store (2) last=20.6
	procAt(t, n, 21.0, base.Add(25*time.Second)) // delta 0.4 <= 0.5 (vs 20.6) → suppress
	assert.Equal(t, 2, store.countForKey("k"))
}

// heartbeat: min_interval 경과 시 변화가 없어도 강제 저장.
func TestStoreWriteNode_Dedup_HeartbeatAfterInterval(t *testing.T) {
	n, store := newDedupNode(t, map[string]any{
		"min_interval": "1m",
		"min_change":   100.0, // 사실상 변화로는 절대 트리거되지 않음
	})
	base := time.UnixMilli(1_000_000)
	procAt(t, n, 20.0, base)                     // store (1) last_at=base
	procAt(t, n, 20.0, base.Add(30*time.Second)) // 구간 내 + 변화 없음 → suppress
	procAt(t, n, 20.0, base.Add(90*time.Second)) // base 로부터 90s >= 60s → store (2)
	assert.Equal(t, 2, store.countForKey("k"))
}

// 퍼센트 dead-band.
func TestStoreWriteNode_Dedup_PercentThreshold(t *testing.T) {
	n, store := newDedupNode(t, map[string]any{
		"min_interval":       "1m",
		"min_change_percent": 10.0,
	})
	base := time.UnixMilli(1_000_000)
	procAt(t, n, 100.0, base)                     // store (1) last=100
	procAt(t, n, 105.0, base.Add(10*time.Second)) // 5% <= 10 → suppress
	procAt(t, n, 115.0, base.Add(20*time.Second)) // 15% > 10 (vs 100) → store (2)
	assert.Equal(t, 2, store.countForKey("k"))
}

// 절대값 + 퍼센트 동시: 둘 중 하나라도 초과하면 저장.
func TestStoreWriteNode_Dedup_BothThresholds(t *testing.T) {
	n, store := newDedupNode(t, map[string]any{
		"min_interval":       "1m",
		"min_change":         1.0,
		"min_change_percent": 50.0,
	})
	base := time.UnixMilli(1_000_000)
	procAt(t, n, 100.0, base)                     // store (1) last=100
	procAt(t, n, 100.5, base.Add(10*time.Second)) // delta 0.5<=1, 0.5%<=50 → suppress
	procAt(t, n, 102.0, base.Add(20*time.Second)) // delta 2>1 → store (2) last=102
	procAt(t, n, 102.5, base.Add(30*time.Second)) // delta 0.5<=1, 0.49%<=50 → suppress
	assert.Equal(t, 2, store.countForKey("k"))
}

// 비숫자 값: 직전 저장값과 다르면 저장, 같으면 구간 내 억제 + heartbeat.
func TestStoreWriteNode_Dedup_NonNumeric(t *testing.T) {
	n, store := newDedupNode(t, map[string]any{
		"min_interval": "1m",
	})
	base := time.UnixMilli(1_000_000)
	procAt(t, n, "on", base)                      // store (1)
	procAt(t, n, "on", base.Add(10*time.Second))  // 동일 + 구간 내 → suppress
	procAt(t, n, "off", base.Add(20*time.Second)) // 다름 → store (2) last="off"
	procAt(t, n, "off", base.Add(30*time.Second)) // 동일 + 구간 내 → suppress
	procAt(t, n, "off", base.Add(90*time.Second)) // base+20s 기준 70s >= 60s → heartbeat store (3)
	assert.Equal(t, 3, store.countForKey("k"))
}

// json.Number(UseNumber 디코딩 결과)도 숫자로 취급되어 절대값 dead-band 가 적용된다.
func TestStoreWriteNode_Dedup_JSONNumber(t *testing.T) {
	n, store := newDedupNode(t, map[string]any{
		"min_interval": "1m",
		"min_change":   0.5,
	})
	base := time.UnixMilli(1_000_000)
	procAt(t, n, json.Number("20.0"), base)                     // store (1) last=20.0
	procAt(t, n, json.Number("20.3"), base.Add(10*time.Second)) // 0.3 <= 0.5 → suppress
	procAt(t, n, json.Number("20.6"), base.Add(20*time.Second)) // 0.6 > 0.5 → store (2)
	assert.Equal(t, 2, store.countForKey("k"))
}

// 시리즈 독립성: 키별로 dead-band 상태가 분리되어야 한다 (요청 #3 분류 기준).
func TestStoreWriteNode_Dedup_PerSeriesIndependence(t *testing.T) {
	def := flow.NodeDef{ID: "sw-multi", Type: "store-write"}
	n, err := NewStoreWriteNode(def)
	require.NoError(t, err)

	store := newCountingStore()
	require.NoError(t, n.Configure(map[string]any{
		"_store":       store,
		"min_interval": "1m",
		"min_change":   0.5,
		"key_mappings": map[string]any{
			"a": "$.payload.va",
			"b": "$.payload.vb",
		},
	}))
	require.NoError(t, n.Init(context.Background()))

	base := time.UnixMilli(1_000_000)
	send := func(va, vb float64, ts time.Time) {
		p := message.NewPayload(map[string]any{"va": va, "vb": vb})
		msg := message.New(message.WithPayload(p), message.WithTimestamp(ts))
		_, perr := n.Process(context.Background(), msg)
		require.NoError(t, perr)
	}

	send(20.0, 50.0, base)                     // a store, b store
	send(20.2, 60.0, base.Add(10*time.Second)) // a suppress(0.2<=0.5), b store(10>0.5)
	assert.Equal(t, 1, store.countForKey("a"))
	assert.Equal(t, 2, store.countForKey("b"))
}

// seriesSignature: key/metric/tags 조합이 다르면 서로 다른 시그니처여야 한다.
func TestSeriesSignature_Distinct(t *testing.T) {
	s1 := seriesSignature("k", "temp", map[string]string{"loc": "a"})
	s2 := seriesSignature("k", "humidity", map[string]string{"loc": "a"}) // metric 다름
	s3 := seriesSignature("k", "temp", map[string]string{"loc": "b"})     // tag 다름
	s4 := seriesSignature("k2", "temp", map[string]string{"loc": "a"})    // key 다름
	s5 := seriesSignature("k", "temp", map[string]string{"loc": "a"})     // s1 과 동일

	assert.Equal(t, s1, s5)
	assert.NotEqual(t, s1, s2)
	assert.NotEqual(t, s1, s3)
	assert.NotEqual(t, s1, s4)

	// 태그 순서가 달라도 동일 시그니처 (정렬 보장).
	a := seriesSignature("k", "m", map[string]string{"x": "1", "y": "2"})
	b := seriesSignature("k", "m", map[string]string{"y": "2", "x": "1"})
	assert.Equal(t, a, b)
}
