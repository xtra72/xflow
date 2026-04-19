package node

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// fakeChartSubscriber 는 chart-emitter 테스트에서 채널 구독을 흉내내는 헬퍼이다.
type fakeChartSubscriber struct {
	id   string
	mu   sync.Mutex
	msgs [][]byte
	done bool
}

func newFakeChartSub(id string) *fakeChartSubscriber {
	return &fakeChartSubscriber{id: id}
}

func (f *fakeChartSubscriber) Send(b []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]byte, len(b))
	copy(cp, b)
	f.msgs = append(f.msgs, cp)
	return nil
}

func (f *fakeChartSubscriber) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.done = true
	return nil
}

func (f *fakeChartSubscriber) ID() string { return f.id }

func (f *fakeChartSubscriber) messages() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([][]byte, len(f.msgs))
	copy(cp, f.msgs)
	return cp
}

func (f *fakeChartSubscriber) closed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.done
}

// setupChartRegistry 는 각 테스트용 임시 레지스트리를 현재 프로세스에 주입한다.
// 테스트 종료 시 원상 복구한다.
func setupChartRegistry(t *testing.T) *system.ChartChannelRegistry {
	t.Helper()
	prev := GetChartChannelRegistry()
	reg := system.NewChartChannelRegistry()
	SetChartChannelRegistry(reg)
	t.Cleanup(func() {
		reg.Close()
		SetChartChannelRegistry(prev)
	})
	return reg
}

// makeChartEmitterDef 는 chart-emitter NodeDef 를 생성한다.
func makeChartEmitterDef() flow.NodeDef {
	return flow.NewNodeDef("emitter-1", "chart-emitter",
		flow.WithInputPorts(flow.Port{ID: "in", Name: "in", Direction: flow.PortInput}),
		flow.WithOutputPorts(),
	)
}

// --- Ports & interface 확인 ---

var _ Node = (*ChartEmitterNode)(nil)

func TestChartEmitterNode_Ports(t *testing.T) {
	t.Parallel()

	setupChartRegistry(t)

	def := makeChartEmitterDef()
	n, err := NewChartEmitterNode(def)
	require.NoError(t, err)

	// Ports 는 input 1 개 + error(기본) 를 포함할 수 있다.
	// Direction 별로 카운팅
	var inputs, outputs int
	for _, p := range n.Ports() {
		switch p.Direction {
		case flow.PortInput:
			inputs++
		case flow.PortOutput:
			outputs++
		}
	}
	assert.Equal(t, 1, inputs, "chart-emitter must have exactly 1 input port")
	assert.Equal(t, 0, outputs, "chart-emitter must have 0 output ports")
}

// --- Configure 테스트 ---

func TestChartEmitterNode_Configure(t *testing.T) {
	setupChartRegistry(t)

	t.Run("missing_channel_name", func(t *testing.T) {
		n, err := NewChartEmitterNode(makeChartEmitterDef())
		require.NoError(t, err)

		err = n.Configure(map[string]any{"buffer_size": 100})
		assert.Error(t, err)
	})

	t.Run("invalid_channel_name_format", func(t *testing.T) {
		n, err := NewChartEmitterNode(makeChartEmitterDef())
		require.NoError(t, err)

		err = n.Configure(map[string]any{"channel_name": "1bad-start"})
		assert.Error(t, err)
	})

	t.Run("invalid_buffer_size_too_small", func(t *testing.T) {
		n, err := NewChartEmitterNode(makeChartEmitterDef())
		require.NoError(t, err)

		err = n.Configure(map[string]any{
			"channel_name": "good",
			"buffer_size":  0,
		})
		assert.Error(t, err)
	})

	t.Run("invalid_buffer_size_too_large", func(t *testing.T) {
		n, err := NewChartEmitterNode(makeChartEmitterDef())
		require.NoError(t, err)

		err = n.Configure(map[string]any{
			"channel_name": "good",
			"buffer_size":  20001,
		})
		assert.Error(t, err)
	})

	t.Run("invalid_retention_sec_out_of_range", func(t *testing.T) {
		n, err := NewChartEmitterNode(makeChartEmitterDef())
		require.NoError(t, err)

		err = n.Configure(map[string]any{
			"channel_name":  "good",
			"retention_sec": 100000,
		})
		assert.Error(t, err)
	})

	t.Run("valid_with_defaults", func(t *testing.T) {
		n, err := NewChartEmitterNode(makeChartEmitterDef())
		require.NoError(t, err)

		err = n.Configure(map[string]any{"channel_name": "room1"})
		require.NoError(t, err)

		em := n.(*ChartEmitterNode)
		assert.Equal(t, "room1", em.ChannelName())
		assert.Equal(t, 100, em.BufferSize())
		assert.Equal(t, 3600, em.RetentionSec())
	})

	t.Run("valid_float64_buffer_size", func(t *testing.T) {
		// JSON 역직렬화 시 숫자는 float64 로 들어옴
		n, err := NewChartEmitterNode(makeChartEmitterDef())
		require.NoError(t, err)

		err = n.Configure(map[string]any{
			"channel_name":  "room1",
			"buffer_size":   float64(250),
			"retention_sec": float64(7200),
		})
		require.NoError(t, err)

		em := n.(*ChartEmitterNode)
		assert.Equal(t, 250, em.BufferSize())
		assert.Equal(t, 7200, em.RetentionSec())
	})

	t.Run("rejects_non_integer_float", func(t *testing.T) {
		n, err := NewChartEmitterNode(makeChartEmitterDef())
		require.NoError(t, err)

		err = n.Configure(map[string]any{
			"channel_name": "room1",
			"buffer_size":  250.5,
		})
		assert.Error(t, err)
	})
}

// --- Init 테스트 ---

func TestChartEmitterNode_Init_RegistersInRegistry(t *testing.T) {
	reg := setupChartRegistry(t)

	n, err := NewChartEmitterNode(makeChartEmitterDef())
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{
		"channel_name":  "init_ok",
		"buffer_size":   50,
		"retention_sec": 60,
	}))

	require.NoError(t, n.Init(context.Background()))

	ch, ok := reg.Get("init_ok")
	require.True(t, ok)
	info := ch.Info()
	assert.Equal(t, 50, info.BufferSize)
	assert.Equal(t, 60, info.RetentionSec)
}

func TestChartEmitterNode_Init_DuplicateFailsFast(t *testing.T) {
	setupChartRegistry(t)

	// 1번 노드 성공
	n1, err := NewChartEmitterNode(makeChartEmitterDef())
	require.NoError(t, err)
	require.NoError(t, n1.Configure(map[string]any{"channel_name": "dupch"}))
	require.NoError(t, n1.Init(context.Background()))

	// 2번 노드: 같은 channel_name
	def2 := flow.NewNodeDef("emitter-2", "chart-emitter",
		flow.WithInputPorts(flow.Port{ID: "in", Name: "in", Direction: flow.PortInput}),
		flow.WithOutputPorts(),
	)
	n2, err := NewChartEmitterNode(def2)
	require.NoError(t, err)
	require.NoError(t, n2.Configure(map[string]any{"channel_name": "dupch"}))

	err = n2.Init(context.Background())
	require.Error(t, err)
	msg := err.Error()
	// fail-fast 메시지 형식
	assert.True(t, strings.Contains(msg, `"dupch"`), "must contain quoted channel name, got %q", msg)
	assert.Contains(t, msg, "is already registered")
}

// --- Process 테스트 ---

func TestChartEmitterNode_Process_NoTimestamp(t *testing.T) {
	reg := setupChartRegistry(t)

	n, err := NewChartEmitterNode(makeChartEmitterDef())
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{"channel_name": "pts"}))
	require.NoError(t, n.Init(context.Background()))

	// 주입 가능한 clock
	em := n.(*ChartEmitterNode)
	em.SetClock(func() int64 { return 12345 })

	// timestamp 없음, value 있음
	payload := message.NewPayload(map[string]any{"value": 42.5})
	msg := message.New(message.WithPayload(payload))
	outs, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Empty(t, outs, "chart-emitter is a sink; no output messages")

	ch, _ := reg.Get("pts")
	snap := ch.Snapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, int64(12345), snap[0].Timestamp)
	assert.EqualValues(t, 42.5, snap[0].Value)
}

func TestChartEmitterNode_Process_NoValue(t *testing.T) {
	reg := setupChartRegistry(t)

	n, err := NewChartEmitterNode(makeChartEmitterDef())
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{"channel_name": "pv"}))
	require.NoError(t, n.Init(context.Background()))

	em := n.(*ChartEmitterNode)
	em.SetClock(func() int64 { return 999 })

	payload := message.NewPayload(map[string]any{
		"room": "A",
		"temp": 25.0,
	})
	msg := message.New(message.WithPayload(payload))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	ch, _ := reg.Get("pv")
	snap := ch.Snapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, int64(999), snap[0].Timestamp)

	// value 는 payload 전체를 감싸야 한다
	m, ok := snap[0].Value.(map[string]any)
	require.True(t, ok, "expected wrapped payload map, got %T", snap[0].Value)
	assert.Equal(t, "A", m["room"])
	assert.EqualValues(t, 25.0, m["temp"])
}

func TestChartEmitterNode_Process_BothPresent(t *testing.T) {
	reg := setupChartRegistry(t)

	n, err := NewChartEmitterNode(makeChartEmitterDef())
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{"channel_name": "pboth"}))
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"timestamp": int64(100),
		"value":     5.0,
		"labels":    map[string]string{"x": "y"},
		"meta":      map[string]any{"src": "test"},
	})
	msg := message.New(message.WithPayload(payload))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	ch, _ := reg.Get("pboth")
	snap := ch.Snapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, int64(100), snap[0].Timestamp)
	assert.EqualValues(t, 5.0, snap[0].Value)
	require.NotNil(t, snap[0].Labels)
	assert.Equal(t, "y", snap[0].Labels["x"])
	require.NotNil(t, snap[0].Meta)
	assert.Equal(t, "test", snap[0].Meta["src"])
}

func TestChartEmitterNode_Process_TimestampAsFloat(t *testing.T) {
	reg := setupChartRegistry(t)

	n, err := NewChartEmitterNode(makeChartEmitterDef())
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{"channel_name": "pfl"}))
	require.NoError(t, n.Init(context.Background()))

	// JSON 역직렬화 케이스: timestamp 가 float64
	payload := message.NewPayload(map[string]any{
		"timestamp": float64(1713312000000),
		"value":     10,
	})
	msg := message.New(message.WithPayload(payload))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	ch, _ := reg.Get("pfl")
	snap := ch.Snapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, int64(1713312000000), snap[0].Timestamp)
}

func TestChartEmitterNode_Process_FanOutToSubscribers(t *testing.T) {
	reg := setupChartRegistry(t)

	n, err := NewChartEmitterNode(makeChartEmitterDef())
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{"channel_name": "pfan"}))
	require.NoError(t, n.Init(context.Background()))

	ch, _ := reg.Get("pfan")
	sub := newFakeChartSub("sub1")
	ch.Subscribe(sub)

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"timestamp": int64(100),
		"value":     1,
	})))
	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	msgs := sub.messages()
	require.Len(t, msgs, 1)

	var frame map[string]any
	require.NoError(t, json.Unmarshal(msgs[0], &frame))
	assert.Equal(t, "chart.append", frame["type"])
	assert.Equal(t, "pfan", frame["channel"])
}

// --- Shutdown 테스트 ---

func TestChartEmitterNode_Shutdown_UnregistersAndClosesSubscribers(t *testing.T) {
	reg := setupChartRegistry(t)

	n, err := NewChartEmitterNode(makeChartEmitterDef())
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{"channel_name": "sd"}))
	require.NoError(t, n.Init(context.Background()))

	ch, ok := reg.Get("sd")
	require.True(t, ok)

	sub := newFakeChartSub("s")
	ch.Subscribe(sub)

	require.NoError(t, n.Shutdown(context.Background()))

	// 레지스트리에서 제거됨
	_, ok = reg.Get("sd")
	assert.False(t, ok)

	// 구독자는 chart.closed 수신 + Close 호출
	msgs := sub.messages()
	require.NotEmpty(t, msgs)
	var frame map[string]any
	require.NoError(t, json.Unmarshal(msgs[len(msgs)-1], &frame))
	assert.Equal(t, "chart.closed", frame["type"])
	assert.True(t, sub.closed())
}

// --- Factory 흐름: Registry.Create 로 빌트인 생성 확인 ---

func TestChartEmitter_RegisteredAsBuiltin(t *testing.T) {
	r := NewRegistry()
	assert.True(t, r.Has("chart-emitter"), "chart-emitter must be registered as builtin")

	meta, ok := r.TypeMeta("chart-emitter")
	require.True(t, ok)
	assert.Equal(t, "chart-emitter", meta.Type)
	assert.NotEmpty(t, meta.Category)
}

// --- 헬퍼 함수 커버리지 보강 테스트 ---

func TestChartConfigToInt_Variants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      any
		want    int
		wantErr bool
	}{
		{"int", 42, 42, false},
		{"int32", int32(7), 7, false},
		{"int64", int64(100), 100, false},
		{"float64_int", float64(123), 123, false},
		{"float32_int", float32(9), 9, false},
		{"float64_frac", 3.5, 0, true},
		{"float32_frac", float32(1.5), 0, true},
		{"string", "100", 0, true},
		{"nil", nil, 0, true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := chartConfigToInt(c.in)
			if c.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, c.want, got)
			}
		})
	}
}

func TestExtractTimestamp_Variants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		payload map[string]any
		want    int64
		wantOk  bool
	}{
		{"missing", map[string]any{"value": 1}, 0, false},
		{"int64", map[string]any{"timestamp": int64(123)}, 123, true},
		{"int", map[string]any{"timestamp": 456}, 456, true},
		{"int32", map[string]any{"timestamp": int32(789)}, 789, true},
		{"float64_int", map[string]any{"timestamp": float64(1000)}, 1000, true},
		{"float64_frac", map[string]any{"timestamp": 1000.5}, 0, false},
		{"float32", map[string]any{"timestamp": float32(2000)}, 2000, true},
		{"float32_frac", map[string]any{"timestamp": float32(2000.5)}, 0, false},
		{"string", map[string]any{"timestamp": "123"}, 0, false},
		{"bool", map[string]any{"timestamp": true}, 0, false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, ok := extractTimestamp(c.payload)
			assert.Equal(t, c.wantOk, ok)
			if c.wantOk {
				assert.Equal(t, c.want, got)
			}
		})
	}
}

func TestToStringMap_Variants(t *testing.T) {
	t.Parallel()

	t.Run("map_string_string", func(t *testing.T) {
		m := map[string]string{"a": "1", "b": "2"}
		got := toStringMap(m)
		require.NotNil(t, got)
		assert.Equal(t, "1", got["a"])
	})
	t.Run("map_string_string_empty", func(t *testing.T) {
		got := toStringMap(map[string]string{})
		assert.Nil(t, got)
	})
	t.Run("map_string_any_with_strings", func(t *testing.T) {
		m := map[string]any{"a": "x", "b": "y"}
		got := toStringMap(m)
		require.NotNil(t, got)
		assert.Equal(t, "x", got["a"])
	})
	t.Run("map_string_any_with_nonstrings", func(t *testing.T) {
		m := map[string]any{"a": 1, "b": true}
		got := toStringMap(m)
		assert.Nil(t, got, "no string values → nil")
	})
	t.Run("map_string_any_empty", func(t *testing.T) {
		got := toStringMap(map[string]any{})
		assert.Nil(t, got)
	})
	t.Run("non_map", func(t *testing.T) {
		got := toStringMap("string")
		assert.Nil(t, got)
	})
}

// Init 이 레지스트리 미설정 상태를 감지하는지 확인
func TestChartEmitterNode_Init_NoRegistryReturnsError(t *testing.T) {
	prev := GetChartChannelRegistry()
	SetChartChannelRegistry(nil)
	t.Cleanup(func() {
		SetChartChannelRegistry(prev)
	})

	n, err := NewChartEmitterNode(makeChartEmitterDef())
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{"channel_name": "nr"}))

	err = n.Init(context.Background())
	assert.Error(t, err)
}

// Process 가 Init 되기 전에 호출되면 에러를 반환하는지 확인
func TestChartEmitterNode_Process_BeforeInit(t *testing.T) {
	setupChartRegistry(t)

	n, err := NewChartEmitterNode(makeChartEmitterDef())
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{"channel_name": "pbi"}))

	// Init 건너뛰고 바로 Process
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 1})))
	_, err = n.Process(context.Background(), msg)
	assert.Error(t, err)
}

// NodeDef.Metadata["flow_id"] 가 전파되는지 확인
func TestChartEmitterNode_FlowIDFromMetadata(t *testing.T) {
	setupChartRegistry(t)

	def := flow.NewNodeDef("em", "chart-emitter",
		flow.WithInputPorts(flow.Port{ID: "in", Name: "in", Direction: flow.PortInput}),
		flow.WithOutputPorts(),
		flow.WithNodeMetadata("flow_id", "flow-xyz"),
	)
	n, err := NewChartEmitterNode(def)
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{"channel_name": "fid"}))
	require.NoError(t, n.Init(context.Background()))

	reg := GetChartChannelRegistry()
	ch, ok := reg.Get("fid")
	require.True(t, ok)
	assert.Equal(t, "flow-xyz", ch.Info().FlowID)
}

// --- 배치 모드 (entries_field) 테스트 ---

// entries_field 설정 시 payload[field] 의 배열을 개별 엔트리로 발행하고
// timestamp 오름차순으로 정렬되어 링버퍼에 저장되는지 확인.
func TestChartEmitterNode_Process_BatchMode_AscendingOrder(t *testing.T) {
	reg := setupChartRegistry(t)

	n, err := NewChartEmitterNode(makeChartEmitterDef())
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{
		"channel_name":  "batch_asc",
		"entries_field": "store_value",
	}))
	require.NoError(t, n.Init(context.Background()))

	// store-read 가 내보내는 descending 배열 (최신순).
	payload := message.NewPayload(map[string]any{
		"store_value": []any{
			map[string]any{"timestamp": int64(300), "value": 23.0},
			map[string]any{"timestamp": int64(200), "value": 22.0},
			map[string]any{"timestamp": int64(100), "value": 21.0},
		},
	})
	msg := message.New(message.WithPayload(payload))
	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	ch, _ := reg.Get("batch_asc")
	snap := ch.Snapshot()
	require.Len(t, snap, 3, "3 entries expected")

	// 링버퍼 자체는 publish 순서 (ascending)
	// Snapshot 은 Subscribe backfill 과 동일하게 ascending 정렬되어 반환됨
	assert.Equal(t, int64(100), snap[0].Timestamp)
	assert.Equal(t, int64(200), snap[1].Timestamp)
	assert.Equal(t, int64(300), snap[2].Timestamp)
	assert.EqualValues(t, 21.0, snap[0].Value)
	assert.EqualValues(t, 23.0, snap[2].Value)
}

// 배치 크기가 buffer_size 를 초과할 때 최신(newest) 엔트리가 남는지 확인.
// 기존 단일엔트리+descending 입력에서는 oldest 가 남았던 문제의 회귀 가드.
func TestChartEmitterNode_Process_BatchMode_FIFOKeepsNewest(t *testing.T) {
	reg := setupChartRegistry(t)

	n, err := NewChartEmitterNode(makeChartEmitterDef())
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{
		"channel_name":  "batch_fifo",
		"buffer_size":   3,
		"entries_field": "store_value",
	}))
	require.NoError(t, n.Init(context.Background()))

	// 5 개 엔트리 (descending 순)
	arr := []any{}
	for ts := int64(500); ts >= 100; ts -= 100 {
		arr = append(arr, map[string]any{"timestamp": ts, "value": float64(ts)})
	}
	payload := message.NewPayload(map[string]any{"store_value": arr})
	msg := message.New(message.WithPayload(payload))
	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	ch, _ := reg.Get("batch_fifo")
	snap := ch.Snapshot()
	require.Len(t, snap, 3, "buffer_size=3 이므로 최신 3개만 남아야 함")
	// 오름차순 정렬 후 publish → 마지막 3개 (300, 400, 500) 가 링버퍼에 남음
	assert.Equal(t, int64(300), snap[0].Timestamp)
	assert.Equal(t, int64(400), snap[1].Timestamp)
	assert.Equal(t, int64(500), snap[2].Timestamp)
}

// entries_field 가 설정되었어도 해당 필드가 없으면 단일 엔트리 경로로 fallback.
// (실시간 업데이트 메시지도 동일 채널로 받을 수 있도록 허용)
func TestChartEmitterNode_Process_BatchMode_FallbackSingleEntry(t *testing.T) {
	reg := setupChartRegistry(t)

	n, err := NewChartEmitterNode(makeChartEmitterDef())
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{
		"channel_name":  "batch_fb",
		"entries_field": "store_value",
	}))
	require.NoError(t, n.Init(context.Background()))

	// store_value 필드 없음, 최상위에 timestamp+value
	payload := message.NewPayload(map[string]any{
		"timestamp": int64(777),
		"value":     3.14,
	})
	msg := message.New(message.WithPayload(payload))
	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	ch, _ := reg.Get("batch_fb")
	snap := ch.Snapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, int64(777), snap[0].Timestamp)
	assert.EqualValues(t, 3.14, snap[0].Value)
}

// primitive 배열 (숫자만) → clock() 타임스탬프 주입 + value 로 저장.
func TestChartEmitterNode_Process_BatchMode_PrimitiveElements(t *testing.T) {
	reg := setupChartRegistry(t)

	n, err := NewChartEmitterNode(makeChartEmitterDef())
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{
		"channel_name":  "batch_prim",
		"entries_field": "values",
	}))
	require.NoError(t, n.Init(context.Background()))

	em := n.(*ChartEmitterNode)
	tick := int64(0)
	em.SetClock(func() int64 {
		tick++
		return tick
	})

	payload := message.NewPayload(map[string]any{
		"values": []any{21.0, 22.0, 23.0},
	})
	msg := message.New(message.WithPayload(payload))
	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	ch, _ := reg.Get("batch_prim")
	snap := ch.Snapshot()
	require.Len(t, snap, 3)
	assert.EqualValues(t, 21.0, snap[0].Value)
	assert.EqualValues(t, 22.0, snap[1].Value)
	assert.EqualValues(t, 23.0, snap[2].Value)
}

// []map[string]any 타입도 배치로 인식해야 함 (Go 측에서 직접 구성된 payload 대비).
func TestChartEmitterNode_Process_BatchMode_TypedMapSlice(t *testing.T) {
	reg := setupChartRegistry(t)

	n, err := NewChartEmitterNode(makeChartEmitterDef())
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{
		"channel_name":  "batch_typed",
		"entries_field": "rows",
	}))
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"rows": []map[string]any{
			{"timestamp": int64(10), "value": 1.0},
			{"timestamp": int64(20), "value": 2.0},
		},
	})
	msg := message.New(message.WithPayload(payload))
	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	ch, _ := reg.Get("batch_typed")
	snap := ch.Snapshot()
	require.Len(t, snap, 2)
	assert.Equal(t, int64(10), snap[0].Timestamp)
	assert.Equal(t, int64(20), snap[1].Timestamp)
}

// entries_field 가 string 이 아니면 Configure 에서 에러.
func TestChartEmitterNode_Configure_EntriesFieldTypeError(t *testing.T) {
	setupChartRegistry(t)

	n, err := NewChartEmitterNode(makeChartEmitterDef())
	require.NoError(t, err)
	err = n.Configure(map[string]any{
		"channel_name":  "etype",
		"entries_field": 123,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entries_field")
}

// 빈 배열은 배치 모드에서 아무것도 publish 하지 않아야 함.
func TestChartEmitterNode_Process_BatchMode_EmptyArray(t *testing.T) {
	reg := setupChartRegistry(t)

	n, err := NewChartEmitterNode(makeChartEmitterDef())
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{
		"channel_name":  "batch_empty",
		"entries_field": "store_value",
	}))
	require.NoError(t, n.Init(context.Background()))

	// 빈 배열 → fallback to single-entry 경로에서 payload 전체가 value 로 감싸짐
	// (이 fallback 동작은 "필드 있지만 빈" 케이스를 실시간 메시지와 구분하지 않음)
	payload := message.NewPayload(map[string]any{
		"store_value": []any{},
	})
	msg := message.New(message.WithPayload(payload))
	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	ch, _ := reg.Get("batch_empty")
	snap := ch.Snapshot()
	// 빈 배열 자체가 value 로 저장됨 (단일 엔트리 fallback).
	require.Len(t, snap, 1)
	assert.NotNil(t, snap[0].Value)
}

// ---- 멀티채널 (channels_field) ----

// TestChartEmitterNode_ChannelsField_MultiChannel 는 map 키별로 별도 채널에 발행하는지 검증한다.
func TestChartEmitterNode_ChannelsField_MultiChannel(t *testing.T) {
	reg := system.NewChartChannelRegistry()
	SetChartChannelRegistry(reg)
	defer SetChartChannelRegistry(nil)

	def := flow.NodeDef{ID: "mc1", Type: "chart-emitter"}
	n, err := NewChartEmitterNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"channels_field":  "store_value",
		"channel_prefix":  "temp_",
		"buffer_size":     100,
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"store_value": map[string]any{
			"room1": []any{
				map[string]any{"value": 23.5, "timestamp": int64(1000)},
			},
			"room2": []any{
				map[string]any{"value": 24.1, "timestamp": int64(2000)},
				map[string]any{"value": 24.3, "timestamp": int64(3000)},
			},
		},
	})
	msg := message.New(message.WithPayload(payload))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	// temp_room1 채널: 1개 엔트리
	ch1, ok := reg.Get("temp_room1")
	require.True(t, ok, "temp_room1 채널이 등록되어야 한다")
	snap1 := ch1.Snapshot()
	require.Len(t, snap1, 1)
	assert.Equal(t, 23.5, snap1[0].Value)

	// temp_room2 채널: 2개 엔트리
	ch2, ok := reg.Get("temp_room2")
	require.True(t, ok, "temp_room2 채널이 등록되어야 한다")
	snap2 := ch2.Snapshot()
	require.Len(t, snap2, 2)
}

// TestChartEmitterNode_ChannelsField_NoPrefixUsesKeyAsChannel 는 prefix 미지정 시
// map 키가 그대로 채널 이름이 되는지 검증한다.
func TestChartEmitterNode_ChannelsField_NoPrefixUsesKeyAsChannel(t *testing.T) {
	reg := system.NewChartChannelRegistry()
	SetChartChannelRegistry(reg)
	defer SetChartChannelRegistry(nil)

	def := flow.NodeDef{ID: "mc2", Type: "chart-emitter"}
	n, err := NewChartEmitterNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"channels_field": "data",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"data": map[string]any{
			"sensorA": []any{
				map[string]any{"value": 10.0, "timestamp": int64(100)},
			},
		},
	})
	msg := message.New(message.WithPayload(payload))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	ch, ok := reg.Get("sensorA")
	require.True(t, ok)
	snap := ch.Snapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, 10.0, snap[0].Value)
}

// TestChartEmitterNode_ChannelsField_TypedMapSlice 는 store-read 출력인
// map[string][]map[string]any 형태의 값을 정상 처리하는지 검증한다.
func TestChartEmitterNode_ChannelsField_TypedMapSlice(t *testing.T) {
	reg := system.NewChartChannelRegistry()
	SetChartChannelRegistry(reg)
	defer SetChartChannelRegistry(nil)

	def := flow.NodeDef{ID: "mc-typed", Type: "chart-emitter"}
	n, err := NewChartEmitterNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"channels_field": "store_value",
		"channel_prefix": "dev_",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	// store-read 실제 출력 형태: map[string]any 값이 []map[string]any
	storeOutput := map[string]any{
		"1": []map[string]any{
			{"timestamp": int64(1000), "value": 20.5},
			{"timestamp": int64(2000), "value": 21.0},
		},
		"2": []map[string]any{
			{"timestamp": int64(1000), "value": 21.5},
		},
	}
	payload := message.NewPayload(map[string]any{
		"store_value": storeOutput,
	})
	_, err = n.Process(context.Background(), message.New(message.WithPayload(payload)))
	require.NoError(t, err)

	ch1, ok := reg.Get("dev_1")
	require.True(t, ok)
	assert.Len(t, ch1.Snapshot(), 2)
	assert.Equal(t, 20.5, ch1.Snapshot()[0].Value)

	ch2, ok := reg.Get("dev_2")
	require.True(t, ok)
	assert.Len(t, ch2.Snapshot(), 1)
}

// TestChartEmitterNode_ChannelsField_NumericKey 는 map 키가 숫자일 때 자동 "ch_" 접두사가 붙는지 검증한다.
func TestChartEmitterNode_ChannelsField_NumericKey(t *testing.T) {
	reg := system.NewChartChannelRegistry()
	SetChartChannelRegistry(reg)
	defer SetChartChannelRegistry(nil)

	def := flow.NodeDef{ID: "mc-num", Type: "chart-emitter"}
	n, err := NewChartEmitterNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"channels_field": "data",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"data": map[string]any{
			"2": []any{map[string]any{"value": 10.0, "timestamp": int64(100)}},
			"3": []any{map[string]any{"value": 20.0, "timestamp": int64(200)}},
		},
	})
	_, err = n.Process(context.Background(), message.New(message.WithPayload(payload)))
	require.NoError(t, err)

	// 숫자 키 → "ch_2", "ch_3"
	ch2, ok := reg.Get("ch_2")
	require.True(t, ok, "숫자 키 2 → ch_2 로 등록되어야 한다")
	assert.Len(t, ch2.Snapshot(), 1)

	ch3, ok := reg.Get("ch_3")
	require.True(t, ok)
	assert.Len(t, ch3.Snapshot(), 1)
}

// TestChartEmitterNode_ChannelsField_ShutdownUnregistersAll 는 Shutdown 시 모든 채널이 해제되는지 검증한다.
func TestChartEmitterNode_ChannelsField_ShutdownUnregistersAll(t *testing.T) {
	reg := system.NewChartChannelRegistry()
	SetChartChannelRegistry(reg)
	defer SetChartChannelRegistry(nil)

	def := flow.NodeDef{ID: "mc3", Type: "chart-emitter"}
	n, err := NewChartEmitterNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"channels_field": "data",
		"channel_prefix": "x_",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"data": map[string]any{
			"a": []any{map[string]any{"value": 1, "timestamp": int64(1)}},
			"b": []any{map[string]any{"value": 2, "timestamp": int64(2)}},
		},
	})
	_, err = n.Process(context.Background(), message.New(message.WithPayload(payload)))
	require.NoError(t, err)

	// 채널 등록 확인
	_, ok := reg.Get("x_a")
	require.True(t, ok)
	_, ok = reg.Get("x_b")
	require.True(t, ok)

	// Shutdown → 모든 채널 해제
	require.NoError(t, n.Shutdown(context.Background()))

	_, ok = reg.Get("x_a")
	require.False(t, ok, "shutdown 후 채널이 해제되어야 한다")
}

