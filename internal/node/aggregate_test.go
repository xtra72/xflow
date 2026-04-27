package node

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// --- AggregateNode 인터페이스 준수 ---

var _ Node = (*AggregateNode)(nil)

// --- NewAggregateNode 테스트 ---

// TestNewAggregateNode_정상생성 은 AggregateNode가 올바르게 생성되는지 확인한다.
func TestNewAggregateNode_정상생성(t *testing.T) {
	def := flow.NewNodeDef("agg-1", "aggregate")
	node, err := NewAggregateNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "agg-1", node.Name())
	assert.Equal(t, "aggregate", node.Type())
}

// --- Init 테스트 ---

// TestAggregateNode_Init_상태전이 는 Init 호출 시 Running 상태로 전이하는지 확인한다.
func TestAggregateNode_Init_상태전이(t *testing.T) {
	def := flow.NewNodeDef("agg-init", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, an.CurrentState())
}

// --- Configure 테스트 ---

// TestAggregateNode_Configure_카운트윈도우 는 count 윈도우 설정이 올바르게 적용되는지 확인한다.
func TestAggregateNode_Configure_카운트윈도우(t *testing.T) {
	def := flow.NewNodeDef("agg-cfg-count", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  3,
		"aggregate_fn": "sum",
	})
	require.NoError(t, err)
	assert.Equal(t, WindowCount, an.windowType)
	assert.Equal(t, 3, an.windowSize)
	assert.Equal(t, []AggregateFn{AggregateSum}, an.aggregateFns)
}

// TestAggregateNode_Configure_타임윈도우 는 time 윈도우 설정이 올바르게 적용되는지 확인한다.
func TestAggregateNode_Configure_타임윈도우(t *testing.T) {
	def := flow.NewNodeDef("agg-cfg-time", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "time",
		"window_size":  "50ms",
		"aggregate_fn": "avg",
	})
	require.NoError(t, err)
	assert.Equal(t, WindowTime, an.windowType)
	assert.Equal(t, 50*time.Millisecond, an.windowDur)
	assert.Equal(t, []AggregateFn{AggregateAvg}, an.aggregateFns)
}

// TestAggregateNode_Configure_nil에러 는 nil config 시 에러를 반환하는지 확인한다.
func TestAggregateNode_Configure_nil에러(t *testing.T) {
	def := flow.NewNodeDef("agg-cfg-nil", "aggregate")
	node, _ := NewAggregateNode(def)

	err := node.Configure(nil)
	assert.Error(t, err)
}

// --- Process 카운트 윈도우 테스트 ---

// TestAggregateNode_Process_카운트윈도우_Sum 은 카운트 윈도우에서 sum 집계가 올바르게 동작하는지 확인한다.
func TestAggregateNode_Process_카운트윈도우_Sum(t *testing.T) {
	def := flow.NewNodeDef("agg-count-sum", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 3
	an.aggregateFns = []AggregateFn{AggregateSum}
	an.fields = []string{"value"}

	ctx := context.Background()

	// 첫 번째 메시지 - 버퍼에 추가, 출력 없음
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 10.0})))
	results, err := an.Process(ctx, msg1)
	require.NoError(t, err)
	assert.Empty(t, results)

	// 두 번째 메시지 - 버퍼에 추가, 출력 없음
	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 20.0})))
	results, err = an.Process(ctx, msg2)
	require.NoError(t, err)
	assert.Empty(t, results)

	// 세 번째 메시지 - 윈도우 완성, 집계 결과 출력
	msg3 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 30.0})))
	results, err = an.Process(ctx, msg3)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 결과 검증
	resultVal, ok := results[0].Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 60.0, resultVal)

	countVal, ok := results[0].Payload().Get("count")
	require.True(t, ok)
	assert.Equal(t, 3, countVal)

	fnMeta, ok := results[0].Metadata().Get("_aggregate_fn")
	require.True(t, ok)
	assert.Equal(t, "sum", fnMeta)

	wtMeta, ok := results[0].Metadata().Get("_window_type")
	require.True(t, ok)
	assert.Equal(t, "count", wtMeta)
}

// TestAggregateNode_Process_카운트윈도우_Avg 는 avg 집계 함수를 확인한다.
func TestAggregateNode_Process_카운트윈도우_Avg(t *testing.T) {
	def := flow.NewNodeDef("agg-count-avg", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 2
	an.aggregateFns = []AggregateFn{AggregateAvg}
	an.fields = []string{"value"}

	ctx := context.Background()
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 10.0})))
	_, _ = an.Process(ctx, msg1)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 30.0})))
	results, err := an.Process(ctx, msg2)
	require.NoError(t, err)
	require.Len(t, results, 1)

	resultVal, ok := results[0].Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 20.0, resultVal)
}

// TestAggregateNode_Process_카운트윈도우_Count 는 count 집계 함수를 확인한다.
func TestAggregateNode_Process_카운트윈도우_Count(t *testing.T) {
	def := flow.NewNodeDef("agg-count-count", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 3
	an.aggregateFns = []AggregateFn{AggregateCount}
	an.fields = []string{"value"}

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		msg := message.New()
		_, _ = an.Process(ctx, msg)
	}

	msg := message.New()
	results, err := an.Process(ctx, msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	resultVal, ok := results[0].Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 3, resultVal)
}

// TestAggregateNode_Process_카운트윈도우_Min 은 min 집계 함수를 확인한다.
func TestAggregateNode_Process_카운트윈도우_Min(t *testing.T) {
	def := flow.NewNodeDef("agg-count-min", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 3
	an.aggregateFns = []AggregateFn{AggregateMin}
	an.fields = []string{"value"}

	ctx := context.Background()
	values := []float64{30.0, 10.0, 20.0}
	var results []message.Message
	for _, v := range values {
		msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": v})))
		var err error
		results, err = an.Process(ctx, msg)
		require.NoError(t, err)
	}

	require.Len(t, results, 1)
	resultVal, ok := results[0].Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 10.0, resultVal)
}

// TestAggregateNode_Process_카운트윈도우_Max 는 max 집계 함수를 확인한다.
func TestAggregateNode_Process_카운트윈도우_Max(t *testing.T) {
	def := flow.NewNodeDef("agg-count-max", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 3
	an.aggregateFns = []AggregateFn{AggregateMax}
	an.fields = []string{"value"}

	ctx := context.Background()
	values := []float64{10.0, 30.0, 20.0}
	var results []message.Message
	for _, v := range values {
		msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": v})))
		var err error
		results, err = an.Process(ctx, msg)
		require.NoError(t, err)
	}

	require.Len(t, results, 1)
	resultVal, ok := results[0].Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 30.0, resultVal)
}

// TestAggregateNode_Process_카운트윈도우_First 는 first 집계 함수를 확인한다.
func TestAggregateNode_Process_카운트윈도우_First(t *testing.T) {
	def := flow.NewNodeDef("agg-count-first", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 2
	an.aggregateFns = []AggregateFn{AggregateFirst}
	an.fields = []string{"value"}

	ctx := context.Background()

	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"key": "first-val"})))
	_, _ = an.Process(ctx, msg1)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"key": "second-val"})))
	results, err := an.Process(ctx, msg2)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// first 는 첫 번째 메시지의 페이로드를 사용한다
	keyVal, ok := results[0].Payload().Get("key")
	require.True(t, ok)
	assert.Equal(t, "first-val", keyVal)
}

// TestAggregateNode_Process_카운트윈도우_Last 는 last 집계 함수를 확인한다.
func TestAggregateNode_Process_카운트윈도우_Last(t *testing.T) {
	def := flow.NewNodeDef("agg-count-last", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 2
	an.aggregateFns = []AggregateFn{AggregateLast}
	an.fields = []string{"value"}

	ctx := context.Background()

	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"key": "first-val"})))
	_, _ = an.Process(ctx, msg1)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"key": "last-val"})))
	results, err := an.Process(ctx, msg2)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// last 는 마지막 메시지의 페이로드를 사용한다
	keyVal, ok := results[0].Payload().Get("key")
	require.True(t, ok)
	assert.Equal(t, "last-val", keyVal)
}

// TestAggregateNode_Process_비숫자값_스킵 은 sum 집계 시 비숫자 값을 건너뛰는지 확인한다.
func TestAggregateNode_Process_비숫자값_스킵(t *testing.T) {
	def := flow.NewNodeDef("agg-skip-nan", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 3
	an.aggregateFns = []AggregateFn{AggregateSum}
	an.fields = []string{"value"}

	ctx := context.Background()

	// 숫자 값
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 10.0})))
	_, _ = an.Process(ctx, msg1)

	// 비숫자 값 (스킵)
	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": "not-a-number"})))
	_, _ = an.Process(ctx, msg2)

	// 숫자 값
	msg3 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 20.0})))
	results, err := an.Process(ctx, msg3)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 비숫자는 스킵되므로 10 + 20 = 30
	resultVal, ok := results[0].Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 30.0, resultVal)
}

// TestAggregateNode_Process_정수값_숫자변환 은 int 타입 값도 float64로 변환하여 집계하는지 확인한다.
func TestAggregateNode_Process_정수값_숫자변환(t *testing.T) {
	def := flow.NewNodeDef("agg-int-conv", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 2
	an.aggregateFns = []AggregateFn{AggregateSum}
	an.fields = []string{"value"}

	ctx := context.Background()

	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 10})))
	_, _ = an.Process(ctx, msg1)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 20})))
	results, err := an.Process(ctx, msg2)
	require.NoError(t, err)
	require.Len(t, results, 1)

	resultVal, ok := results[0].Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 30.0, resultVal)
}

// --- Process 타임 윈도우 테스트 ---

// TestAggregateNode_Process_타임윈도우_자동플러시 는 타임 윈도우에서 시간이 지나면 버퍼가 플러시되는지 확인한다.
func TestAggregateNode_Process_타임윈도우_자동플러시(t *testing.T) {
	def := flow.NewNodeDef("agg-time-flush", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowTime
	an.windowDur = 30 * time.Millisecond
	an.aggregateFns = []AggregateFn{AggregateSum}
	an.fields = []string{"value"}

	// Init으로 타이머 시작
	err := an.Init(context.Background())
	require.NoError(t, err)

	ctx := context.Background()

	// 메시지 추가
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 10.0})))
	_, err = an.Process(ctx, msg1)
	require.NoError(t, err)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 20.0})))
	_, err = an.Process(ctx, msg2)
	require.NoError(t, err)

	// 타이머 만료 대기
	time.Sleep(60 * time.Millisecond)

	// flushResult를 확인
	an.mu.Lock()
	result := an.lastFlushResult
	an.mu.Unlock()

	require.NotNil(t, result, "타이머 만료 후 플러시 결과가 있어야 한다")
	require.Len(t, result, 1)

	resultVal, ok := result[0].Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 30.0, resultVal)

	// Shutdown으로 정리
	_ = an.Shutdown(context.Background())
}

// --- Shutdown 테스트 ---

// TestAggregateNode_Shutdown_상태전이 는 Shutdown 시 Stopping 상태로 전이하는지 확인한다.
func TestAggregateNode_Shutdown_상태전이(t *testing.T) {
	def := flow.NewNodeDef("agg-shut", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	_ = an.Init(context.Background())
	err := an.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, an.CurrentState())
}

// TestAggregateNode_Shutdown_잔여버퍼플러시 는 Shutdown 시 버퍼에 남은 메시지를 플러시하는지 확인한다.
func TestAggregateNode_Shutdown_잔여버퍼플러시(t *testing.T) {
	def := flow.NewNodeDef("agg-shut-flush", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 10 // 큰 윈도우 사이즈
	an.aggregateFns = []AggregateFn{AggregateSum}
	an.fields = []string{"value"}

	_ = an.Init(context.Background())

	ctx := context.Background()

	// 윈도우 완성 전에 메시지 추가
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 5.0})))
	_, _ = an.Process(ctx, msg1)
	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 15.0})))
	_, _ = an.Process(ctx, msg2)

	// Shutdown으로 잔여 버퍼 플러시
	err := an.Shutdown(context.Background())
	require.NoError(t, err)

	// 플러시 결과 확인
	an.mu.Lock()
	result := an.lastFlushResult
	an.mu.Unlock()

	require.NotNil(t, result)
	require.Len(t, result, 1)

	resultVal, ok := result[0].Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 20.0, resultVal)
}

// --- 유효하지 않은 윈도우 테스트 ---

// TestAggregateNode_Configure_유효하지않은윈도우사이즈 는 windowSize <= 0일 때 에러를 반환하는지 확인한다.
func TestAggregateNode_Configure_유효하지않은윈도우사이즈(t *testing.T) {
	def := flow.NewNodeDef("agg-invalid-size", "aggregate")
	node, _ := NewAggregateNode(def)

	err := node.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  0,
		"aggregate_fn": "sum",
	})
	assert.ErrorIs(t, err, ErrAggregateWindowInvalid)
}

// TestAggregateNode_Configure_유효하지않은윈도우타입 은 잘못된 window_type 시 에러를 반환하는지 확인한다.
func TestAggregateNode_Configure_유효하지않은윈도우타입(t *testing.T) {
	def := flow.NewNodeDef("agg-invalid-type", "aggregate")
	node, _ := NewAggregateNode(def)

	err := node.Configure(map[string]any{
		"window_type":  "invalid",
		"window_size":  3,
		"aggregate_fn": "sum",
	})
	assert.ErrorIs(t, err, ErrAggregateWindowInvalid)
}

// --- Configure 다중 함수/필드 파싱 테스트 ---

// TestAggregateNode_Configure_다중집계함수 는 aggregate_fn이 리스트로 전달될 때 올바르게 파싱되는지 확인한다.
func TestAggregateNode_Configure_다중집계함수(t *testing.T) {
	def := flow.NewNodeDef("agg-cfg-multi-fn", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  5,
		"aggregate_fn": []any{"sum", "avg", "min", "max"},
	})
	require.NoError(t, err)
	assert.Equal(t, []AggregateFn{AggregateSum, AggregateAvg, AggregateMin, AggregateMax}, an.aggregateFns)
}

// TestAggregateNode_Configure_빈집계함수리스트 는 빈 aggregate_fn 리스트가 에러를 반환하는지 확인한다.
func TestAggregateNode_Configure_빈집계함수리스트(t *testing.T) {
	def := flow.NewNodeDef("agg-cfg-empty-fn", "aggregate")
	node, _ := NewAggregateNode(def)

	err := node.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  5,
		"aggregate_fn": []any{},
	})
	assert.ErrorIs(t, err, ErrAggregateFnInvalid)
}

// TestAggregateNode_Configure_유효하지않은집계함수 는 알 수 없는 함수 이름이 에러를 반환하는지 확인한다.
func TestAggregateNode_Configure_유효하지않은집계함수(t *testing.T) {
	def := flow.NewNodeDef("agg-cfg-invalid-fn", "aggregate")
	node, _ := NewAggregateNode(def)

	err := node.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  5,
		"aggregate_fn": "unknown_fn",
	})
	assert.ErrorIs(t, err, ErrAggregateFnInvalid)
}

// TestAggregateNode_Configure_리스트내_유효하지않은함수 는 리스트 내 알 수 없는 함수가 에러를 반환하는지 확인한다.
func TestAggregateNode_Configure_리스트내_유효하지않은함수(t *testing.T) {
	def := flow.NewNodeDef("agg-cfg-list-invalid", "aggregate")
	node, _ := NewAggregateNode(def)

	err := node.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  5,
		"aggregate_fn": []any{"sum", "invalid"},
	})
	assert.ErrorIs(t, err, ErrAggregateFnInvalid)
}

// TestAggregateNode_Configure_다중필드 는 fields 리스트가 올바르게 파싱되는지 확인한다.
func TestAggregateNode_Configure_다중필드(t *testing.T) {
	def := flow.NewNodeDef("agg-cfg-multi-fields", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  5,
		"aggregate_fn": "sum",
		"fields":       []any{"temperature", "humidity"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"temperature", "humidity"}, an.fields)
}

// TestAggregateNode_Configure_단일필드_field키 는 field 키로 단일 필드가 파싱되는지 확인한다.
func TestAggregateNode_Configure_단일필드_field키(t *testing.T) {
	def := flow.NewNodeDef("agg-cfg-single-field", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  5,
		"aggregate_fn": "sum",
		"field":        "temperature",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"temperature"}, an.fields)
}

// TestAggregateNode_Configure_필드미설정_기본값 은 field/fields 미설정 시 기본값 "value"가 사용되는지 확인한다.
func TestAggregateNode_Configure_필드미설정_기본값(t *testing.T) {
	def := flow.NewNodeDef("agg-cfg-default-field", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  5,
		"aggregate_fn": "sum",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"value"}, an.fields)
}

// TestAggregateNode_Configure_빈필드리스트 는 빈 fields 리스트가 에러를 반환하는지 확인한다.
func TestAggregateNode_Configure_빈필드리스트(t *testing.T) {
	def := flow.NewNodeDef("agg-cfg-empty-fields", "aggregate")
	node, _ := NewAggregateNode(def)

	err := node.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  5,
		"aggregate_fn": "sum",
		"fields":       []any{},
	})
	assert.ErrorIs(t, err, ErrAggregateFieldInvalid)
}

// TestAggregateNode_Configure_fields우선순위 는 fields가 field보다 우선하는지 확인한다.
func TestAggregateNode_Configure_fields우선순위(t *testing.T) {
	def := flow.NewNodeDef("agg-cfg-fields-priority", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  5,
		"aggregate_fn": "sum",
		"fields":       []any{"temperature", "humidity"},
		"field":        "pressure",
	})
	require.NoError(t, err)
	// fields가 field보다 우선한다
	assert.Equal(t, []string{"temperature", "humidity"}, an.fields)
}

// TestAggregateNode_Configure_windowSizeRaw저장 은 windowSizeRaw가 올바르게 저장되는지 확인한다.
func TestAggregateNode_Configure_windowSizeRaw저장(t *testing.T) {
	def := flow.NewNodeDef("agg-cfg-ws-raw", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	// 카운트 윈도우
	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  10,
		"aggregate_fn": "sum",
	})
	require.NoError(t, err)
	assert.Equal(t, 10, an.windowSizeRaw)

	// 타임 윈도우
	def2 := flow.NewNodeDef("agg-cfg-ws-raw2", "aggregate")
	node2, _ := NewAggregateNode(def2)
	an2 := node2.(*AggregateNode)

	err = an2.Configure(map[string]any{
		"window_type":  "time",
		"window_size":  "30s",
		"aggregate_fn": "avg",
	})
	require.NoError(t, err)
	assert.Equal(t, "30s", an2.windowSizeRaw)
}

// TestAggregateNode_Configure_모든유효한함수 는 모든 유효한 집계 함수가 올바르게 파싱되는지 확인한다.
func TestAggregateNode_Configure_모든유효한함수(t *testing.T) {
	def := flow.NewNodeDef("agg-cfg-all-fns", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  5,
		"aggregate_fn": []any{"sum", "avg", "min", "max", "count", "first", "last"},
	})
	require.NoError(t, err)
	assert.Len(t, an.aggregateFns, 7)
}

// --- 동시성 테스트 ---

// TestAggregateNode_동시성안전_Process 는 Process가 동시성 안전한지 확인한다.
func TestAggregateNode_동시성안전_Process(t *testing.T) {
	def := flow.NewNodeDef("agg-conc", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 100
	an.aggregateFns = []AggregateFn{AggregateCount}
	an.fields = []string{"value"}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg := message.New()
			_, _ = an.Process(context.Background(), msg)
		}()
	}
	wg.Wait()
}

// TestAggregateNode_동시성안전_다중모드_Process 는 다중 모드에서 Process가 동시성 안전한지 확인한다.
func TestAggregateNode_동시성안전_다중모드_Process(t *testing.T) {
	def := flow.NewNodeDef("agg-conc-multi", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 100
	an.aggregateFns = []AggregateFn{AggregateSum, AggregateAvg, AggregateCount}
	an.fields = []string{"temperature", "humidity"}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(val float64) {
			defer wg.Done()
			msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
				"temperature": val,
				"humidity":    val * 2,
			})))
			_, _ = an.Process(context.Background(), msg)
		}(float64(i))
	}
	wg.Wait()
}

// TestAggregateNode_Process_타임윈도우_다중모드_자동플러시 는 타임 윈도우 다중 모드에서 자동 플러시가 올바르게 동작하는지 확인한다.
func TestAggregateNode_Process_타임윈도우_다중모드_자동플러시(t *testing.T) {
	def := flow.NewNodeDef("agg-time-multi-flush", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowTime
	an.windowDur = 30 * time.Millisecond
	an.aggregateFns = []AggregateFn{AggregateAvg, AggregateMin, AggregateMax}
	an.fields = []string{"temperature", "humidity"}
	an.windowSizeRaw = "30ms"

	// Init으로 타이머 시작
	err := an.Init(context.Background())
	require.NoError(t, err)

	ctx := context.Background()

	// 메시지 추가
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 20.0,
		"humidity":    60.0,
	})))
	_, err = an.Process(ctx, msg1)
	require.NoError(t, err)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 30.0,
		"humidity":    80.0,
	})))
	_, err = an.Process(ctx, msg2)
	require.NoError(t, err)

	// 타이머 만료 대기
	time.Sleep(60 * time.Millisecond)

	// flushResult를 확인
	an.mu.Lock()
	result := an.lastFlushResult
	an.mu.Unlock()

	require.NotNil(t, result, "타이머 만료 후 플러시 결과가 있어야 한다")
	require.Len(t, result, 1)

	// stats 매트릭스 검증
	statsRaw, ok := result[0].Payload().Get("stats")
	require.True(t, ok)
	stats, ok := statsRaw.(map[string]any)
	require.True(t, ok)

	tempStats, ok := stats["temperature"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 25.0, tempStats["avg"])
	assert.Equal(t, 20.0, tempStats["min"])
	assert.Equal(t, 30.0, tempStats["max"])

	humStats, ok := stats["humidity"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 70.0, humStats["avg"])
	assert.Equal(t, 60.0, humStats["min"])
	assert.Equal(t, 80.0, humStats["max"])

	// window_size 검증
	ws, ok := result[0].Payload().Get("window_size")
	require.True(t, ok)
	assert.Equal(t, "30ms", ws)

	// Shutdown으로 정리
	_ = an.Shutdown(context.Background())
}

// TestAggregateNode_Shutdown_다중모드_잔여버퍼플러시 는 다중 모드에서 Shutdown 시 잔여 버퍼를 올바르게 플러시하는지 확인한다.
func TestAggregateNode_Shutdown_다중모드_잔여버퍼플러시(t *testing.T) {
	def := flow.NewNodeDef("agg-shut-multi-flush", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 10 // 큰 윈도우 사이즈
	an.aggregateFns = []AggregateFn{AggregateSum, AggregateAvg}
	an.fields = []string{"temperature"}
	an.windowSizeRaw = 10

	_ = an.Init(context.Background())

	ctx := context.Background()

	// 윈도우 완성 전에 메시지 추가
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"temperature": 10.0})))
	_, _ = an.Process(ctx, msg1)
	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"temperature": 30.0})))
	_, _ = an.Process(ctx, msg2)

	// Shutdown으로 잔여 버퍼 플러시
	err := an.Shutdown(context.Background())
	require.NoError(t, err)

	// 플러시 결과 확인
	an.mu.Lock()
	result := an.lastFlushResult
	an.mu.Unlock()

	require.NotNil(t, result)
	require.Len(t, result, 1)

	// 다중 함수 모드이므로 stats 매트릭스 확인
	statsRaw, ok := result[0].Payload().Get("stats")
	require.True(t, ok)
	stats := statsRaw.(map[string]any)
	tempStats := stats["temperature"].(map[string]any)

	assert.Equal(t, 40.0, tempStats["sum"])
	assert.Equal(t, 20.0, tempStats["avg"])
}

// --- 테이블 드리븐 집계 함수 테스트 ---

// TestAggregateNode_Process_집계함수테이블 은 모든 집계 함수를 테이블 드리븐 테스트로 검증한다.
func TestAggregateNode_Process_집계함수테이블(t *testing.T) {
	tests := []struct {
		name       string
		fn         AggregateFn
		values     []float64
		wantResult any
	}{
		{"sum 합산", AggregateSum, []float64{1, 2, 3}, 6.0},
		{"avg 평균", AggregateAvg, []float64{10, 20, 30}, 20.0},
		{"min 최솟값", AggregateMin, []float64{5, 1, 9}, 1.0},
		{"max 최댓값", AggregateMax, []float64{5, 1, 9}, 9.0},
		{"count 개수", AggregateCount, []float64{1, 2, 3}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := flow.NewNodeDef("agg-table-"+tt.name, "aggregate")
			node, _ := NewAggregateNode(def)
			an := node.(*AggregateNode)

			an.windowType = WindowCount
			an.windowSize = len(tt.values)
			an.aggregateFns = []AggregateFn{tt.fn}
			an.fields = []string{"value"}

			ctx := context.Background()
			var results []message.Message
			for _, v := range tt.values {
				msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": v})))
				var err error
				results, err = an.Process(ctx, msg)
				require.NoError(t, err)
			}

			require.Len(t, results, 1)
			resultVal, ok := results[0].Payload().Get("result")
			require.True(t, ok)
			assert.Equal(t, tt.wantResult, resultVal)
		})
	}
}

// --- 다중 필드/다중 함수 테스트 ---

// TestAggregateNode_Process_다중함수_단일필드 는 다중 함수 모드에서 stats 매트릭스가 올바르게 생성되는지 확인한다.
func TestAggregateNode_Process_다중함수_단일필드(t *testing.T) {
	def := flow.NewNodeDef("agg-multi-fn", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 3
	an.aggregateFns = []AggregateFn{AggregateSum, AggregateAvg, AggregateMin, AggregateMax, AggregateCount}
	an.fields = []string{"value"}
	an.windowSizeRaw = 3

	ctx := context.Background()
	values := []float64{10.0, 20.0, 30.0}
	var results []message.Message
	for _, v := range values {
		msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": v})))
		var err error
		results, err = an.Process(ctx, msg)
		require.NoError(t, err)
	}

	require.Len(t, results, 1)

	// stats 매트릭스 검증
	statsRaw, ok := results[0].Payload().Get("stats")
	require.True(t, ok)
	stats, ok := statsRaw.(map[string]any)
	require.True(t, ok)

	valueStats, ok := stats["value"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, 60.0, valueStats["sum"])
	assert.Equal(t, 20.0, valueStats["avg"])
	assert.Equal(t, 10.0, valueStats["min"])
	assert.Equal(t, 30.0, valueStats["max"])
	assert.Equal(t, 3, valueStats["count"])

	// window_count, window_type, window_size 검증
	wc, ok := results[0].Payload().Get("window_count")
	require.True(t, ok)
	assert.Equal(t, 3, wc)

	wt, ok := results[0].Payload().Get("window_type")
	require.True(t, ok)
	assert.Equal(t, "count", wt)

	ws, ok := results[0].Payload().Get("window_size")
	require.True(t, ok)
	assert.Equal(t, 3, ws)

	// 다중 모드에서는 result 키가 없어야 한다
	_, hasResult := results[0].Payload().Get("result")
	assert.False(t, hasResult, "다중 모드에서는 result 키가 없어야 한다")

	// 메타데이터 검증
	fnMeta, ok := results[0].Metadata().Get("_aggregate_fn")
	require.True(t, ok)
	assert.Equal(t, "sum,avg,min,max,count", fnMeta)

	fieldsMeta, ok := results[0].Metadata().Get("_fields")
	require.True(t, ok)
	assert.Equal(t, "value", fieldsMeta)
}

// TestAggregateNode_Process_다중필드_다중함수 는 다중 필드+다중 함수 모드에서 stats 매트릭스가 올바르게 생성되는지 확인한다.
func TestAggregateNode_Process_다중필드_다중함수(t *testing.T) {
	def := flow.NewNodeDef("agg-multi-field-fn", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 3
	an.aggregateFns = []AggregateFn{AggregateAvg, AggregateMin, AggregateMax, AggregateCount}
	an.fields = []string{"temperature", "humidity"}
	an.windowSizeRaw = 3

	ctx := context.Background()

	// 3개의 메시지 전송
	msgs := []map[string]any{
		{"temperature": 20.0, "humidity": 60.0},
		{"temperature": 25.0, "humidity": 65.0},
		{"temperature": 30.0, "humidity": 70.0},
	}

	var results []message.Message
	for _, data := range msgs {
		msg := message.New(message.WithPayload(message.NewPayload(data)))
		var err error
		results, err = an.Process(ctx, msg)
		require.NoError(t, err)
	}

	require.Len(t, results, 1)

	// stats 매트릭스 검증
	statsRaw, ok := results[0].Payload().Get("stats")
	require.True(t, ok)
	stats, ok := statsRaw.(map[string]any)
	require.True(t, ok)

	// temperature 필드 검증
	tempStats, ok := stats["temperature"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 25.0, tempStats["avg"])
	assert.Equal(t, 20.0, tempStats["min"])
	assert.Equal(t, 30.0, tempStats["max"])
	assert.Equal(t, 3, tempStats["count"])

	// humidity 필드 검증
	humStats, ok := stats["humidity"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 65.0, humStats["avg"])
	assert.Equal(t, 60.0, humStats["min"])
	assert.Equal(t, 70.0, humStats["max"])
	assert.Equal(t, 3, humStats["count"])

	// 메타데이터 검증
	fieldsMeta, ok := results[0].Metadata().Get("_fields")
	require.True(t, ok)
	assert.Equal(t, "temperature,humidity", fieldsMeta)
}

// TestAggregateNode_Process_다중필드_결측값_스킵 은 다중 필드 모드에서 특정 필드가 없는 메시지를 건너뛰는지 확인한다.
func TestAggregateNode_Process_다중필드_결측값_스킵(t *testing.T) {
	def := flow.NewNodeDef("agg-multi-missing", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 3
	an.aggregateFns = []AggregateFn{AggregateSum, AggregateCount}
	an.fields = []string{"temperature", "humidity"}
	an.windowSizeRaw = 3

	ctx := context.Background()

	// humidity가 없는 메시지 포함
	msgs := []map[string]any{
		{"temperature": 20.0, "humidity": 60.0},
		{"temperature": 25.0},                      // humidity 없음
		{"temperature": 30.0, "humidity": 70.0},
	}

	var results []message.Message
	for _, data := range msgs {
		msg := message.New(message.WithPayload(message.NewPayload(data)))
		var err error
		results, err = an.Process(ctx, msg)
		require.NoError(t, err)
	}

	require.Len(t, results, 1)

	statsRaw, ok := results[0].Payload().Get("stats")
	require.True(t, ok)
	stats, ok := statsRaw.(map[string]any)
	require.True(t, ok)

	// temperature: 3개 모두 존재
	tempStats, ok := stats["temperature"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 75.0, tempStats["sum"])
	assert.Equal(t, 3, tempStats["count"])

	// humidity: 2개만 존재 (결측값 스킵)
	humStats, ok := stats["humidity"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 130.0, humStats["sum"])
	assert.Equal(t, 2, humStats["count"])
}

// TestAggregateNode_Process_다중필드_first_last 는 다중 필드 모드에서 first/last가 올바르게 동작하는지 확인한다.
func TestAggregateNode_Process_다중필드_first_last(t *testing.T) {
	def := flow.NewNodeDef("agg-multi-first-last", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 3
	an.aggregateFns = []AggregateFn{AggregateFirst, AggregateLast}
	an.fields = []string{"temperature", "humidity"}
	an.windowSizeRaw = 3

	ctx := context.Background()

	msgs := []map[string]any{
		{"temperature": 20.0, "humidity": 60.0},
		{"temperature": 25.0, "humidity": 65.0},
		{"temperature": 30.0, "humidity": 70.0},
	}

	var results []message.Message
	for _, data := range msgs {
		msg := message.New(message.WithPayload(message.NewPayload(data)))
		var err error
		results, err = an.Process(ctx, msg)
		require.NoError(t, err)
	}

	require.Len(t, results, 1)

	statsRaw, ok := results[0].Payload().Get("stats")
	require.True(t, ok)
	stats, ok := statsRaw.(map[string]any)
	require.True(t, ok)

	tempStats, ok := stats["temperature"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 20.0, tempStats["first"])
	assert.Equal(t, 30.0, tempStats["last"])

	humStats, ok := stats["humidity"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 60.0, humStats["first"])
	assert.Equal(t, 70.0, humStats["last"])
}

// TestAggregateNode_Process_단일모드_stats포함 은 단일 모드에서도 stats가 포함되는지 확인한다.
func TestAggregateNode_Process_단일모드_stats포함(t *testing.T) {
	def := flow.NewNodeDef("agg-single-stats", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 3
	an.aggregateFns = []AggregateFn{AggregateSum}
	an.fields = []string{"value"}
	an.windowSizeRaw = 3

	ctx := context.Background()
	values := []float64{10.0, 20.0, 30.0}
	var results []message.Message
	for _, v := range values {
		msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": v})))
		var err error
		results, err = an.Process(ctx, msg)
		require.NoError(t, err)
	}

	require.Len(t, results, 1)

	// 기존 호환 필드 확인
	resultVal, ok := results[0].Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 60.0, resultVal)

	countVal, ok := results[0].Payload().Get("count")
	require.True(t, ok)
	assert.Equal(t, 3, countVal)

	// 새 필드 확인
	statsRaw, ok := results[0].Payload().Get("stats")
	require.True(t, ok)
	stats, ok := statsRaw.(map[string]any)
	require.True(t, ok)

	valueStats, ok := stats["value"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 60.0, valueStats["sum"])

	// window_count, window_type, window_size 확인
	wc, ok := results[0].Payload().Get("window_count")
	require.True(t, ok)
	assert.Equal(t, 3, wc)

	// 메타데이터 확인
	fieldsMeta, ok := results[0].Metadata().Get("_fields")
	require.True(t, ok)
	assert.Equal(t, "value", fieldsMeta)
}

// TestAggregateNode_Process_단일모드_count_stats 은 단일 모드 count에서 stats가 올바르게 포함되는지 확인한다.
func TestAggregateNode_Process_단일모드_count_stats(t *testing.T) {
	def := flow.NewNodeDef("agg-single-count-stats", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 3
	an.aggregateFns = []AggregateFn{AggregateCount}
	an.fields = []string{"value"}
	an.windowSizeRaw = 3

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": float64(i)})))
		_, _ = an.Process(ctx, msg)
	}

	an.mu.Lock()
	// 버퍼가 비었으므로 (이미 플러시됨) 새로운 메시지 3개 추가
	an.mu.Unlock()

	// 다시 3개 추가
	var results []message.Message
	for i := 0; i < 3; i++ {
		msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": float64(i + 10)})))
		var err error
		results, err = an.Process(ctx, msg)
		require.NoError(t, err)
	}

	require.Len(t, results, 1)

	// 기존 호환: result = 3 (메시지 개수)
	resultVal, ok := results[0].Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 3, resultVal)

	// stats.value.count = 3 (유효한 숫자 값 개수)
	statsRaw, ok := results[0].Payload().Get("stats")
	require.True(t, ok)
	stats := statsRaw.(map[string]any)
	valueStats := stats["value"].(map[string]any)
	assert.Equal(t, 3, valueStats["count"])
}

// TestAggregateNode_Process_Configure경유_다중모드 는 Configure를 통한 다중 모드 설정이 올바르게 동작하는지 확인한다.
func TestAggregateNode_Process_Configure경유_다중모드(t *testing.T) {
	def := flow.NewNodeDef("agg-cfg-multi", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  2,
		"aggregate_fn": []any{"avg", "min", "max"},
		"fields":       []any{"temperature", "humidity"},
	})
	require.NoError(t, err)

	ctx := context.Background()

	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 20.0,
		"humidity":    60.0,
	})))
	_, err = an.Process(ctx, msg1)
	require.NoError(t, err)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 30.0,
		"humidity":    80.0,
	})))
	results, err := an.Process(ctx, msg2)
	require.NoError(t, err)
	require.Len(t, results, 1)

	statsRaw, ok := results[0].Payload().Get("stats")
	require.True(t, ok)
	stats := statsRaw.(map[string]any)

	tempStats := stats["temperature"].(map[string]any)
	assert.Equal(t, 25.0, tempStats["avg"])
	assert.Equal(t, 20.0, tempStats["min"])
	assert.Equal(t, 30.0, tempStats["max"])

	humStats := stats["humidity"].(map[string]any)
	assert.Equal(t, 70.0, humStats["avg"])
	assert.Equal(t, 60.0, humStats["min"])
	assert.Equal(t, 80.0, humStats["max"])

	// window_size가 windowSizeRaw에서 온 값인지 확인
	ws, ok := results[0].Payload().Get("window_size")
	require.True(t, ok)
	assert.Equal(t, 2, ws)
}

// ==========================================================================
// SPEC-AGG-002: Group-By 파티셔닝 테스트 (M1-M4)
// ==========================================================================

// --- M1: group_by Configure + 그룹 버퍼 구조 테스트 ---

// TestAggregateNode_Configure_GroupBy_단일키파싱 은 group_by 단일 문자열 파싱을 확인한다.
func TestAggregateNode_Configure_GroupBy_단일키파싱(t *testing.T) {
	def := flow.NewNodeDef("agg-gb-single", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  3,
		"aggregate_fn": "avg",
		"group_by":     "location",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"location"}, an.groupByKeys)
	assert.NotNil(t, an.groupBuffers)
	assert.Equal(t, 100, an.maxGroups) // 기본값
}

// TestAggregateNode_Configure_GroupBy_복합키파싱 은 group_by 문자열 배열(복합 키) 파싱을 확인한다.
func TestAggregateNode_Configure_GroupBy_복합키파싱(t *testing.T) {
	def := flow.NewNodeDef("agg-gb-composite", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  3,
		"aggregate_fn": "avg",
		"group_by":     []any{"location", "device_id"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"location", "device_id"}, an.groupByKeys)
	assert.NotNil(t, an.groupBuffers)
}

// TestAggregateNode_Configure_GroupBy_빈문자열에러 는 group_by 빈 문자열이 에러를 반환하는지 확인한다.
func TestAggregateNode_Configure_GroupBy_빈문자열에러(t *testing.T) {
	def := flow.NewNodeDef("agg-gb-empty", "aggregate")
	node, _ := NewAggregateNode(def)

	err := node.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  3,
		"aggregate_fn": "avg",
		"group_by":     "",
	})
	assert.ErrorIs(t, err, ErrAggregateGroupByInvalid)
}

// TestAggregateNode_Configure_GroupBy_빈배열에러 는 group_by 빈 배열이 에러를 반환하는지 확인한다.
func TestAggregateNode_Configure_GroupBy_빈배열에러(t *testing.T) {
	def := flow.NewNodeDef("agg-gb-empty-arr", "aggregate")
	node, _ := NewAggregateNode(def)

	err := node.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  3,
		"aggregate_fn": "avg",
		"group_by":     []any{},
	})
	assert.ErrorIs(t, err, ErrAggregateGroupByInvalid)
}

// TestAggregateNode_Configure_GroupBy_배열내빈문자열에러 는 group_by 배열 내 빈 문자열이 에러를 반환하는지 확인한다.
func TestAggregateNode_Configure_GroupBy_배열내빈문자열에러(t *testing.T) {
	def := flow.NewNodeDef("agg-gb-arr-empty-str", "aggregate")
	node, _ := NewAggregateNode(def)

	err := node.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  3,
		"aggregate_fn": "avg",
		"group_by":     []any{"location", ""},
	})
	assert.ErrorIs(t, err, ErrAggregateGroupByInvalid)
}

// TestAggregateNode_Configure_GroupBy_미설정_하위호환 은 group_by 미설정 시 기존 동작과 동일한지 확인한다.
func TestAggregateNode_Configure_GroupBy_미설정_하위호환(t *testing.T) {
	def := flow.NewNodeDef("agg-gb-compat", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  3,
		"aggregate_fn": "avg",
	})
	require.NoError(t, err)
	assert.Nil(t, an.groupByKeys)
	assert.Nil(t, an.groupBuffers)
}

// TestAggregateNode_Configure_MaxGroups_양의정수 는 max_groups 양의 정수 파싱을 확인한다.
func TestAggregateNode_Configure_MaxGroups_양의정수(t *testing.T) {
	def := flow.NewNodeDef("agg-mg-valid", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  3,
		"aggregate_fn": "avg",
		"group_by":     "location",
		"max_groups":   50,
	})
	require.NoError(t, err)
	assert.Equal(t, 50, an.maxGroups)
}

// TestAggregateNode_Configure_MaxGroups_기본값 은 max_groups 미설정 시 기본값(100)을 확인한다.
func TestAggregateNode_Configure_MaxGroups_기본값(t *testing.T) {
	def := flow.NewNodeDef("agg-mg-default", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  3,
		"aggregate_fn": "avg",
		"group_by":     "location",
	})
	require.NoError(t, err)
	assert.Equal(t, 100, an.maxGroups)
}

// TestAggregateNode_Configure_MaxGroups_0이하에러 는 max_groups 0 이하가 에러를 반환하는지 확인한다.
func TestAggregateNode_Configure_MaxGroups_0이하에러(t *testing.T) {
	def := flow.NewNodeDef("agg-mg-invalid", "aggregate")
	node, _ := NewAggregateNode(def)

	err := node.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  3,
		"aggregate_fn": "avg",
		"group_by":     "location",
		"max_groups":   0,
	})
	assert.ErrorIs(t, err, ErrAggregateMaxGroupsInvalid)

	// 음수 테스트
	def2 := flow.NewNodeDef("agg-mg-neg", "aggregate")
	node2, _ := NewAggregateNode(def2)

	err = node2.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  3,
		"aggregate_fn": "avg",
		"group_by":     "location",
		"max_groups":   -5,
	})
	assert.ErrorIs(t, err, ErrAggregateMaxGroupsInvalid)
}

// --- M2: 그룹별 집계 로직 테스트 ---

// TestAggregateNode_Process_그룹_카운트윈도우_단일키 는 단일 키 그룹 모드에서 count 윈도우가 그룹별 독립 플러시되는지 확인한다.
func TestAggregateNode_Process_그룹_카운트윈도우_단일키(t *testing.T) {
	def := flow.NewNodeDef("agg-grp-count-single", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  2,
		"aggregate_fn": "avg",
		"field":        "temperature",
		"group_by":     "location",
	})
	require.NoError(t, err)

	ctx := context.Background()

	// 그룹 A: seoul 메시지 1
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "seoul",
		"temperature": 20.0,
	})))
	results, err := an.Process(ctx, msg1)
	require.NoError(t, err)
	assert.Empty(t, results)

	// 그룹 B: busan 메시지 1
	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "busan",
		"temperature": 25.0,
	})))
	results, err = an.Process(ctx, msg2)
	require.NoError(t, err)
	assert.Empty(t, results)

	// 그룹 A: seoul 메시지 2 → 윈도우 완성, 플러시
	msg3 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "seoul",
		"temperature": 30.0,
	})))
	results, err = an.Process(ctx, msg3)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 결과 검증: seoul 그룹만 플러시
	resultVal, ok := results[0].Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 25.0, resultVal) // avg(20, 30) = 25

	groupKey, ok := results[0].Payload().Get("group_key")
	require.True(t, ok)
	assert.Equal(t, "location", groupKey)

	groupValue, ok := results[0].Payload().Get("group_value")
	require.True(t, ok)
	assert.Equal(t, "seoul", groupValue)

	// busan 그룹은 영향받지 않음 (아직 버퍼에 1개)
	an.mu.Lock()
	assert.Len(t, an.groupBuffers["busan"], 1)
	an.mu.Unlock()
}

// TestAggregateNode_Process_그룹_카운트윈도우_복합키 는 복합 키 그룹 모드에서 count 윈도우 플러시를 확인한다.
func TestAggregateNode_Process_그룹_카운트윈도우_복합키(t *testing.T) {
	def := flow.NewNodeDef("agg-grp-count-comp", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  2,
		"aggregate_fn": "avg",
		"field":        "temperature",
		"group_by":     []any{"location", "device_id"},
	})
	require.NoError(t, err)

	ctx := context.Background()

	// 그룹 "factory-A|sensor-001"
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "factory-A",
		"device_id":   "sensor-001",
		"temperature": 20.0,
	})))
	results, _ := an.Process(ctx, msg1)
	assert.Empty(t, results)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "factory-A",
		"device_id":   "sensor-001",
		"temperature": 30.0,
	})))
	results, err = an.Process(ctx, msg2)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 복합 키 출력 검증
	groupKeys, ok := results[0].Payload().Get("group_keys")
	require.True(t, ok)
	assert.Equal(t, []string{"location", "device_id"}, groupKeys)

	groupValue, ok := results[0].Payload().Get("group_value")
	require.True(t, ok)
	assert.Equal(t, "factory-A|sensor-001", groupValue)

	groupValues, ok := results[0].Payload().Get("group_values")
	require.True(t, ok)
	gv, ok := groupValues.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "factory-A", gv["location"])
	assert.Equal(t, "sensor-001", gv["device_id"])

	// 메타데이터 검증
	gkMeta, ok := results[0].Metadata().Get("_group_keys")
	require.True(t, ok)
	assert.Equal(t, "location,device_id", gkMeta)

	gvMeta, ok := results[0].Metadata().Get("_group_value")
	require.True(t, ok)
	assert.Equal(t, "factory-A|sensor-001", gvMeta)
}

// TestAggregateNode_Process_그룹_타임윈도우_다중그룹플러시 는 time 윈도우에서 다중 그룹이 동시에 플러시되는지 확인한다.
func TestAggregateNode_Process_그룹_타임윈도우_다중그룹플러시(t *testing.T) {
	def := flow.NewNodeDef("agg-grp-time-multi", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "time",
		"window_size":  "30ms",
		"aggregate_fn": "avg",
		"field":        "temperature",
		"group_by":     "location",
	})
	require.NoError(t, err)

	// Init으로 타이머 시작
	err = an.Init(context.Background())
	require.NoError(t, err)

	ctx := context.Background()

	// 그룹 A: seoul
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "seoul",
		"temperature": 20.0,
	})))
	_, _ = an.Process(ctx, msg1)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "seoul",
		"temperature": 30.0,
	})))
	_, _ = an.Process(ctx, msg2)

	// 그룹 B: busan
	msg3 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "busan",
		"temperature": 15.0,
	})))
	_, _ = an.Process(ctx, msg3)

	// 타이머 만료 대기
	time.Sleep(60 * time.Millisecond)

	an.mu.Lock()
	result := an.lastFlushResult
	an.mu.Unlock()

	require.NotNil(t, result, "타이머 만료 후 플러시 결과가 있어야 한다")
	require.Len(t, result, 2, "2개 그룹의 결과가 있어야 한다")

	// 결과를 group_value로 분류
	resultMap := make(map[string]message.Message)
	for _, r := range result {
		gv, ok := r.Payload().Get("group_value")
		require.True(t, ok)
		resultMap[gv.(string)] = r
	}

	// seoul 그룹 검증
	seoulResult, ok := resultMap["seoul"]
	require.True(t, ok)
	seoulVal, ok := seoulResult.Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 25.0, seoulVal) // avg(20, 30) = 25

	// busan 그룹 검증
	busanResult, ok := resultMap["busan"]
	require.True(t, ok)
	busanVal, ok := busanResult.Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 15.0, busanVal) // avg(15) = 15

	_ = an.Shutdown(context.Background())
}

// TestAggregateNode_Process_그룹_타임윈도우_빈그룹스킵 은 time 윈도우에서 빈 그룹이 스킵되는지 확인한다.
func TestAggregateNode_Process_그룹_타임윈도우_빈그룹스킵(t *testing.T) {
	def := flow.NewNodeDef("agg-grp-time-empty", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "time",
		"window_size":  "30ms",
		"aggregate_fn": "avg",
		"field":        "temperature",
		"group_by":     "location",
	})
	require.NoError(t, err)

	// Init으로 타이머 시작
	err = an.Init(context.Background())
	require.NoError(t, err)

	ctx := context.Background()

	// count 윈도우로 그룹 A를 플러시해 빈 그룹 생성
	// 대신 time 윈도우이므로 그룹 A 메시지 추가 후 타이머로 플러시
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "seoul",
		"temperature": 20.0,
	})))
	_, _ = an.Process(ctx, msg1)

	// 첫 번째 타이머 만료 대기 - seoul 플러시됨
	time.Sleep(60 * time.Millisecond)

	an.mu.Lock()
	result1 := an.lastFlushResult
	an.mu.Unlock()

	require.Len(t, result1, 1)

	// 두 번째 타이머 만료 대기 - seoul 버퍼가 비었으므로 스킵되어야 함
	time.Sleep(60 * time.Millisecond)

	an.mu.Lock()
	result2 := an.lastFlushResult
	an.mu.Unlock()

	// lastFlushResult는 첫 번째 플러시 결과가 유지됨 (빈 그룹은 결과를 생성하지 않으므로)
	assert.Equal(t, result1, result2)

	_ = an.Shutdown(context.Background())
}

// --- M3: 출력 형식 + 그룹화된 stats 매트릭스 테스트 ---

// TestAggregateNode_Process_그룹_다중필드_다중함수_단일키 는 group_by + 다중 필드 + 다중 함수 조합을 확인한다.
func TestAggregateNode_Process_그룹_다중필드_다중함수_단일키(t *testing.T) {
	def := flow.NewNodeDef("agg-grp-multi-single", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  2,
		"aggregate_fn": []any{"avg", "min", "max"},
		"fields":       []any{"temperature", "humidity"},
		"group_by":     "location",
	})
	require.NoError(t, err)

	ctx := context.Background()

	// 그룹 seoul: 2개 메시지
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "seoul",
		"temperature": 20.0,
		"humidity":    60.0,
	})))
	_, _ = an.Process(ctx, msg1)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "seoul",
		"temperature": 30.0,
		"humidity":    80.0,
	})))
	results, err := an.Process(ctx, msg2)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 출력 구조 검증
	groupKey, ok := results[0].Payload().Get("group_key")
	require.True(t, ok)
	assert.Equal(t, "location", groupKey)

	groupValue, ok := results[0].Payload().Get("group_value")
	require.True(t, ok)
	assert.Equal(t, "seoul", groupValue)

	// stats 매트릭스 검증
	statsRaw, ok := results[0].Payload().Get("stats")
	require.True(t, ok)
	stats, ok := statsRaw.(map[string]any)
	require.True(t, ok)

	tempStats, ok := stats["temperature"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 25.0, tempStats["avg"])
	assert.Equal(t, 20.0, tempStats["min"])
	assert.Equal(t, 30.0, tempStats["max"])

	humStats, ok := stats["humidity"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 70.0, humStats["avg"])
	assert.Equal(t, 60.0, humStats["min"])
	assert.Equal(t, 80.0, humStats["max"])

	// window_count, window_type, window_size 검증
	wc, ok := results[0].Payload().Get("window_count")
	require.True(t, ok)
	assert.Equal(t, 2, wc)

	wt, ok := results[0].Payload().Get("window_type")
	require.True(t, ok)
	assert.Equal(t, "count", wt)

	ws, ok := results[0].Payload().Get("window_size")
	require.True(t, ok)
	assert.Equal(t, 2, ws)

	// 메타데이터 검증
	gkMeta, ok := results[0].Metadata().Get("_group_keys")
	require.True(t, ok)
	assert.Equal(t, "location", gkMeta)

	gvMeta, ok := results[0].Metadata().Get("_group_value")
	require.True(t, ok)
	assert.Equal(t, "seoul", gvMeta)
}

// TestAggregateNode_Process_그룹_다중필드_다중함수_복합키 는 복합 키 + 다중 필드 + 다중 함수 조합을 확인한다.
func TestAggregateNode_Process_그룹_다중필드_다중함수_복합키(t *testing.T) {
	def := flow.NewNodeDef("agg-grp-multi-comp", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  2,
		"aggregate_fn": []any{"avg", "min", "max"},
		"fields":       []any{"temperature", "humidity"},
		"group_by":     []any{"location", "device_id"},
	})
	require.NoError(t, err)

	ctx := context.Background()

	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "factory-A",
		"device_id":   "sensor-001",
		"temperature": 20.0,
		"humidity":    60.0,
	})))
	_, _ = an.Process(ctx, msg1)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "factory-A",
		"device_id":   "sensor-001",
		"temperature": 30.0,
		"humidity":    80.0,
	})))
	results, err := an.Process(ctx, msg2)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 복합 키 출력 검증
	groupKeys, ok := results[0].Payload().Get("group_keys")
	require.True(t, ok)
	assert.Equal(t, []string{"location", "device_id"}, groupKeys)

	groupValue, ok := results[0].Payload().Get("group_value")
	require.True(t, ok)
	assert.Equal(t, "factory-A|sensor-001", groupValue)

	groupValues, ok := results[0].Payload().Get("group_values")
	require.True(t, ok)
	gv := groupValues.(map[string]string)
	assert.Equal(t, "factory-A", gv["location"])
	assert.Equal(t, "sensor-001", gv["device_id"])

	// stats 매트릭스 검증
	statsRaw, ok := results[0].Payload().Get("stats")
	require.True(t, ok)
	stats := statsRaw.(map[string]any)

	tempStats := stats["temperature"].(map[string]any)
	assert.Equal(t, 25.0, tempStats["avg"])
	assert.Equal(t, 20.0, tempStats["min"])
	assert.Equal(t, 30.0, tempStats["max"])

	humStats := stats["humidity"].(map[string]any)
	assert.Equal(t, 70.0, humStats["avg"])
	assert.Equal(t, 60.0, humStats["min"])
	assert.Equal(t, 80.0, humStats["max"])
}

// TestAggregateNode_Process_비그룹모드_하위호환_출력형식 은 group_by 미설정 시 기존 출력 형식이 유지되는지 확인한다.
func TestAggregateNode_Process_비그룹모드_하위호환_출력형식(t *testing.T) {
	def := flow.NewNodeDef("agg-compat-output", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  2,
		"aggregate_fn": "avg",
		"field":        "temperature",
	})
	require.NoError(t, err)

	ctx := context.Background()

	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"temperature": 20.0})))
	_, _ = an.Process(ctx, msg1)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"temperature": 30.0})))
	results, err := an.Process(ctx, msg2)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 기존 호환 필드 확인 (SPEC-AGG-001)
	resultVal, ok := results[0].Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 25.0, resultVal)

	countVal, ok := results[0].Payload().Get("count")
	require.True(t, ok)
	assert.Equal(t, 2, countVal)

	// group_key, group_value 가 없어야 한다
	_, hasGroupKey := results[0].Payload().Get("group_key")
	assert.False(t, hasGroupKey, "비그룹 모드에서는 group_key가 없어야 한다")

	_, hasGroupValue := results[0].Payload().Get("group_value")
	assert.False(t, hasGroupValue, "비그룹 모드에서는 group_value가 없어야 한다")
}

// --- M4: 하위 호환 + 엣지 케이스 테스트 ---

// TestAggregateNode_Process_그룹_MaxGroups초과_드롭 은 max_groups 초과 시 메시지를 드롭하는지 확인한다.
func TestAggregateNode_Process_그룹_MaxGroups초과_드롭(t *testing.T) {
	def := flow.NewNodeDef("agg-grp-maxgroups", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  10,
		"aggregate_fn": "avg",
		"field":        "temperature",
		"group_by":     "location",
		"max_groups":   2,
	})
	require.NoError(t, err)

	ctx := context.Background()

	// 그룹 1: seoul
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "seoul",
		"temperature": 20.0,
	})))
	_, _ = an.Process(ctx, msg1)

	// 그룹 2: busan
	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "busan",
		"temperature": 25.0,
	})))
	_, _ = an.Process(ctx, msg2)

	// 그룹 3: jeju → max_groups 초과, 드롭
	msg3 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "jeju",
		"temperature": 30.0,
	})))
	results, err := an.Process(ctx, msg3)
	require.NoError(t, err)
	assert.Empty(t, results)

	// jeju 그룹이 생성되지 않았는지 확인
	an.mu.Lock()
	_, exists := an.groupBuffers["jeju"]
	assert.False(t, exists, "max_groups 초과 시 새 그룹이 생성되지 않아야 한다")

	// 기존 그룹은 정상 유지
	assert.Len(t, an.groupBuffers["seoul"], 1)
	assert.Len(t, an.groupBuffers["busan"], 1)
	an.mu.Unlock()

	// 기존 그룹 메시지는 정상 처리
	msg4 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "seoul",
		"temperature": 22.0,
	})))
	_, err = an.Process(ctx, msg4)
	require.NoError(t, err)

	an.mu.Lock()
	assert.Len(t, an.groupBuffers["seoul"], 2)
	an.mu.Unlock()
}

// TestAggregateNode_Process_그룹_Unknown_단일키 는 단일 키 모드에서 nil 값이 _unknown 그룹에 배정되는지 확인한다.
func TestAggregateNode_Process_그룹_Unknown_단일키(t *testing.T) {
	def := flow.NewNodeDef("agg-grp-unknown-single", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  2,
		"aggregate_fn": "avg",
		"field":        "temperature",
		"group_by":     "location",
	})
	require.NoError(t, err)

	ctx := context.Background()

	// location 필드가 없는 메시지
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 20.0,
	})))
	_, _ = an.Process(ctx, msg1)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 30.0,
	})))
	results, err := an.Process(ctx, msg2)
	require.NoError(t, err)
	require.Len(t, results, 1)

	groupValue, ok := results[0].Payload().Get("group_value")
	require.True(t, ok)
	assert.Equal(t, "_unknown", groupValue)
}

// TestAggregateNode_Process_그룹_Unknown_복합키_부분누락 은 복합 키에서 일부 필드 누락 시 해당 필드만 _unknown으로 대체되는지 확인한다.
func TestAggregateNode_Process_그룹_Unknown_복합키_부분누락(t *testing.T) {
	def := flow.NewNodeDef("agg-grp-unknown-partial", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  2,
		"aggregate_fn": "avg",
		"field":        "temperature",
		"group_by":     []any{"location", "device_id"},
	})
	require.NoError(t, err)

	ctx := context.Background()

	// device_id가 없는 메시지
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "factory-A",
		"temperature": 20.0,
	})))
	_, _ = an.Process(ctx, msg1)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "factory-A",
		"temperature": 30.0,
	})))
	results, err := an.Process(ctx, msg2)
	require.NoError(t, err)
	require.Len(t, results, 1)

	groupValue, ok := results[0].Payload().Get("group_value")
	require.True(t, ok)
	assert.Equal(t, "factory-A|_unknown", groupValue)

	groupValues, ok := results[0].Payload().Get("group_values")
	require.True(t, ok)
	gv := groupValues.(map[string]string)
	assert.Equal(t, "factory-A", gv["location"])
	assert.Equal(t, "_unknown", gv["device_id"])
}

// TestAggregateNode_Process_그룹_Unknown_복합키_전체누락 은 복합 키에서 모든 필드 누락 시 _unknown|_unknown이 되는지 확인한다.
func TestAggregateNode_Process_그룹_Unknown_복합키_전체누락(t *testing.T) {
	def := flow.NewNodeDef("agg-grp-unknown-all", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  2,
		"aggregate_fn": "avg",
		"field":        "temperature",
		"group_by":     []any{"location", "device_id"},
	})
	require.NoError(t, err)

	ctx := context.Background()

	// 두 필드 모두 없는 메시지
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 20.0,
	})))
	_, _ = an.Process(ctx, msg1)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 30.0,
	})))
	results, err := an.Process(ctx, msg2)
	require.NoError(t, err)
	require.Len(t, results, 1)

	groupValue, ok := results[0].Payload().Get("group_value")
	require.True(t, ok)
	assert.Equal(t, "_unknown|_unknown", groupValue)
}

// TestAggregateNode_Process_그룹_타입변환 은 그룹 키 값이 비문자열 타입일 때 변환이 올바르게 되는지 확인한다.
func TestAggregateNode_Process_그룹_타입변환(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{"float64", 25.5, "25.5"},
		{"int", 42, "42"},
		{"bool_true", true, "true"},
		{"bool_false", false, "false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := flow.NewNodeDef("agg-grp-type-"+tt.name, "aggregate")
			node, _ := NewAggregateNode(def)
			an := node.(*AggregateNode)

			err := an.Configure(map[string]any{
				"window_type":  "count",
				"window_size":  2,
				"aggregate_fn": "avg",
				"field":        "temperature",
				"group_by":     "zone",
			})
			require.NoError(t, err)

			ctx := context.Background()

			msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
				"zone":        tt.value,
				"temperature": 20.0,
			})))
			_, _ = an.Process(ctx, msg1)

			msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
				"zone":        tt.value,
				"temperature": 30.0,
			})))
			results, err := an.Process(ctx, msg2)
			require.NoError(t, err)
			require.Len(t, results, 1)

			groupValue, ok := results[0].Payload().Get("group_value")
			require.True(t, ok)
			assert.Equal(t, tt.expected, groupValue)
		})
	}
}

// TestAggregateNode_Process_그룹_플러시후_버퍼초기화_재사용 은 플러시 후 그룹 버퍼가 초기화되고 재사용되는지 확인한다.
func TestAggregateNode_Process_그룹_플러시후_버퍼초기화_재사용(t *testing.T) {
	def := flow.NewNodeDef("agg-grp-flush-reuse", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  2,
		"aggregate_fn": "avg",
		"field":        "temperature",
		"group_by":     "location",
	})
	require.NoError(t, err)

	ctx := context.Background()

	// 첫 번째 윈도우
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "seoul",
		"temperature": 20.0,
	})))
	_, _ = an.Process(ctx, msg1)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "seoul",
		"temperature": 30.0,
	})))
	results1, _ := an.Process(ctx, msg2)
	require.Len(t, results1, 1)

	// 플러시 후 그룹 버퍼가 비어있는지 확인
	an.mu.Lock()
	assert.Len(t, an.groupBuffers["seoul"], 0)
	_, exists := an.groupBuffers["seoul"]
	assert.True(t, exists, "플러시 후에도 그룹 키는 맵에 남아있어야 한다")
	an.mu.Unlock()

	// 두 번째 윈도우 - 같은 그룹에 다시 메시지 추가
	msg3 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "seoul",
		"temperature": 40.0,
	})))
	_, _ = an.Process(ctx, msg3)

	msg4 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "seoul",
		"temperature": 50.0,
	})))
	results2, _ := an.Process(ctx, msg4)
	require.Len(t, results2, 1)

	resultVal, ok := results2[0].Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 45.0, resultVal) // avg(40, 50) = 45
}

// TestAggregateNode_Shutdown_그룹_잔여버퍼플러시 는 Shutdown 시 모든 그룹의 잔여 버퍼가 플러시되는지 확인한다.
func TestAggregateNode_Shutdown_그룹_잔여버퍼플러시(t *testing.T) {
	def := flow.NewNodeDef("agg-grp-shut-flush", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  10, // 큰 윈도우 사이즈
		"aggregate_fn": "avg",
		"field":        "temperature",
		"group_by":     "location",
	})
	require.NoError(t, err)

	_ = an.Init(context.Background())

	ctx := context.Background()

	// 여러 그룹에 메시지 추가 (윈도우 완성 안 됨)
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "seoul",
		"temperature": 20.0,
	})))
	_, _ = an.Process(ctx, msg1)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "busan",
		"temperature": 25.0,
	})))
	_, _ = an.Process(ctx, msg2)

	msg3 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "seoul",
		"temperature": 30.0,
	})))
	_, _ = an.Process(ctx, msg3)

	// Shutdown 으로 잔여 버퍼 플러시
	err = an.Shutdown(context.Background())
	require.NoError(t, err)

	an.mu.Lock()
	result := an.lastFlushResult
	an.mu.Unlock()

	require.NotNil(t, result)
	require.Len(t, result, 2, "2개 그룹의 결과가 있어야 한다")

	// 결과를 group_value로 분류
	resultMap := make(map[string]message.Message)
	for _, r := range result {
		gv, ok := r.Payload().Get("group_value")
		require.True(t, ok)
		resultMap[gv.(string)] = r
	}

	// seoul: avg(20, 30) = 25
	seoulResult, ok := resultMap["seoul"]
	require.True(t, ok)
	seoulVal, ok := seoulResult.Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 25.0, seoulVal)

	// busan: avg(25) = 25
	busanResult, ok := resultMap["busan"]
	require.True(t, ok)
	busanVal, ok := busanResult.Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 25.0, busanVal)
}

// TestAggregateNode_동시성안전_그룹모드_Process 는 그룹 모드에서 Process가 동시성 안전한지 확인한다.
func TestAggregateNode_동시성안전_그룹모드_Process(t *testing.T) {
	def := flow.NewNodeDef("agg-grp-conc", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  100,
		"aggregate_fn": "sum",
		"field":        "temperature",
		"group_by":     "location",
		"max_groups":   10,
	})
	require.NoError(t, err)

	locations := []string{"seoul", "busan", "jeju", "incheon", "daegu"}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			loc := locations[idx%len(locations)]
			msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
				"location":    loc,
				"temperature": float64(idx),
			})))
			_, _ = an.Process(context.Background(), msg)
		}(i)
	}
	wg.Wait()

	// 레이스 조건 없이 완료되면 성공
	an.mu.Lock()
	totalMsgs := 0
	for _, buf := range an.groupBuffers {
		totalMsgs += len(buf)
	}
	an.mu.Unlock()
	assert.Equal(t, 50, totalMsgs, "모든 메시지가 그룹 버퍼에 추가되어야 한다")
}

// TestAggregateNode_동시성안전_그룹모드_타임윈도우 는 그룹 모드 타임 윈도우에서 동시성이 안전한지 확인한다.
func TestAggregateNode_동시성안전_그룹모드_타임윈도우(t *testing.T) {
	def := flow.NewNodeDef("agg-grp-conc-time", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "time",
		"window_size":  "20ms",
		"aggregate_fn": "sum",
		"field":        "temperature",
		"group_by":     "location",
	})
	require.NoError(t, err)

	_ = an.Init(context.Background())

	locations := []string{"seoul", "busan", "jeju"}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			loc := locations[idx%len(locations)]
			msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
				"location":    loc,
				"temperature": float64(idx),
			})))
			_, _ = an.Process(context.Background(), msg)
		}(i)
	}
	wg.Wait()

	// 타이머 만료 대기
	time.Sleep(50 * time.Millisecond)

	_ = an.Shutdown(context.Background())
	// 레이스 조건 없이 완료되면 성공
}

// TestAggregateNode_Process_그룹_카운트윈도우_그룹A플러시_그룹B미영향 은 그룹 A가 플러시될 때 그룹 B가 영향받지 않는지 확인한다.
func TestAggregateNode_Process_그룹_카운트윈도우_그룹A플러시_그룹B미영향(t *testing.T) {
	def := flow.NewNodeDef("agg-grp-independent", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  2,
		"aggregate_fn": "sum",
		"field":        "temperature",
		"group_by":     "location",
	})
	require.NoError(t, err)

	ctx := context.Background()

	// 그룹 A (seoul) 메시지 2개 → 플러시
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"location": "seoul", "temperature": 10.0})))
	_, _ = an.Process(ctx, msg1)

	// 그룹 B (busan) 메시지 1개
	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"location": "busan", "temperature": 20.0})))
	_, _ = an.Process(ctx, msg2)

	// 그룹 A 두 번째 메시지 → 플러시
	msg3 := message.New(message.WithPayload(message.NewPayload(map[string]any{"location": "seoul", "temperature": 30.0})))
	results, err := an.Process(ctx, msg3)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// seoul 결과
	resultVal, ok := results[0].Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 40.0, resultVal) // sum(10, 30) = 40

	// busan은 여전히 버퍼에 1개
	an.mu.Lock()
	assert.Len(t, an.groupBuffers["busan"], 1)
	assert.Len(t, an.groupBuffers["seoul"], 0) // 플러시됨
	an.mu.Unlock()
}

// ==========================================================================
// SPEC-AGG-002 M6-M7: 슬라이딩 윈도우 테스트
// ==========================================================================

// --- M6: Configure 슬라이딩 윈도우 테스트 ---

// TestAggregateNode_Configure_슬라이딩윈도우_기본설정 은 슬라이딩 윈도우 기본 설정이 올바르게 적용되는지 확인한다.
func TestAggregateNode_Configure_슬라이딩윈도우_기본설정(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-basic", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":    "sliding",
		"window_size":    "1s",
		"slide_interval": "100ms",
		"aggregate_fn":   "avg",
		"field":          "temperature",
	})
	require.NoError(t, err)
	assert.Equal(t, WindowSliding, an.windowType)
	assert.Equal(t, 1*time.Second, an.windowDur)
	assert.Equal(t, 100*time.Millisecond, an.slideDuration)
	assert.NotNil(t, an.tsBuffer)
	assert.Nil(t, an.groupTsBuffers)
}

// TestAggregateNode_Configure_슬라이딩윈도우_슬라이드간격_기본값 은 slide_interval 미설정 시 기본값(windowDur/10)을 확인한다.
func TestAggregateNode_Configure_슬라이딩윈도우_슬라이드간격_기본값(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-default-si", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "sliding",
		"window_size":  "1s",
		"aggregate_fn": "avg",
	})
	require.NoError(t, err)
	assert.Equal(t, WindowSliding, an.windowType)
	assert.Equal(t, 100*time.Millisecond, an.slideDuration) // 1s / 10 = 100ms
}

// TestAggregateNode_Configure_슬라이딩윈도우_슬라이드간격_초과에러 는 slide_interval > window_size 시 에러를 반환하는지 확인한다.
func TestAggregateNode_Configure_슬라이딩윈도우_슬라이드간격_초과에러(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-si-exceed", "aggregate")
	node, _ := NewAggregateNode(def)

	err := node.Configure(map[string]any{
		"window_type":    "sliding",
		"window_size":    "1s",
		"slide_interval": "2s",
		"aggregate_fn":   "avg",
	})
	assert.ErrorIs(t, err, ErrAggregateSlideIntervalInvalid)
}

// TestAggregateNode_Configure_슬라이딩윈도우_슬라이드간격_파싱에러 는 유효하지 않은 slide_interval이 에러를 반환하는지 확인한다.
func TestAggregateNode_Configure_슬라이딩윈도우_슬라이드간격_파싱에러(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-si-parse", "aggregate")
	node, _ := NewAggregateNode(def)

	err := node.Configure(map[string]any{
		"window_type":    "sliding",
		"window_size":    "1s",
		"slide_interval": "invalid",
		"aggregate_fn":   "avg",
	})
	assert.ErrorIs(t, err, ErrAggregateSlideIntervalParse)
}

// TestAggregateNode_Configure_슬라이딩윈도우_슬라이드간격_비문자열에러 는 slide_interval이 문자열이 아닐 때 에러를 반환하는지 확인한다.
func TestAggregateNode_Configure_슬라이딩윈도우_슬라이드간격_비문자열에러(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-si-non-str", "aggregate")
	node, _ := NewAggregateNode(def)

	err := node.Configure(map[string]any{
		"window_type":    "sliding",
		"window_size":    "1s",
		"slide_interval": 100,
		"aggregate_fn":   "avg",
	})
	assert.ErrorIs(t, err, ErrAggregateSlideIntervalParse)
}

// TestAggregateNode_Configure_슬라이딩윈도우_슬라이드간격_0이하에러 는 slide_interval <= 0일 때 에러를 반환하는지 확인한다.
func TestAggregateNode_Configure_슬라이딩윈도우_슬라이드간격_0이하에러(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-si-zero", "aggregate")
	node, _ := NewAggregateNode(def)

	err := node.Configure(map[string]any{
		"window_type":    "sliding",
		"window_size":    "1s",
		"slide_interval": "0s",
		"aggregate_fn":   "avg",
	})
	assert.ErrorIs(t, err, ErrAggregateSlideIntervalParse)
}

// TestAggregateNode_Configure_슬라이딩윈도우_그룹모드_버퍼초기화 는 슬라이딩 윈도우 그룹 모드에서 groupTsBuffers가 초기화되는지 확인한다.
func TestAggregateNode_Configure_슬라이딩윈도우_그룹모드_버퍼초기화(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-grp-init", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":    "sliding",
		"window_size":    "1s",
		"slide_interval": "100ms",
		"aggregate_fn":   "avg",
		"group_by":       "location",
	})
	require.NoError(t, err)
	assert.NotNil(t, an.groupTsBuffers)
	assert.Nil(t, an.tsBuffer) // 그룹 모드이므로 tsBuffer는 nil
}

// --- M6: Process + Flush 슬라이딩 윈도우 테스트 ---

// TestAggregateNode_슬라이딩윈도우_메시지타임스탬프저장 은 슬라이딩 윈도우에서 메시지가 타임스탬프와 함께 저장되는지 확인한다.
func TestAggregateNode_슬라이딩윈도우_메시지타임스탬프저장(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-ts-store", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":    "sliding",
		"window_size":    "1s",
		"slide_interval": "100ms",
		"aggregate_fn":   "sum",
		"field":          "value",
	})
	require.NoError(t, err)

	ctx := context.Background()
	beforeProcess := time.Now()

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 10.0})))
	results, err := an.Process(ctx, msg)
	require.NoError(t, err)
	assert.Empty(t, results) // 슬라이딩 윈도우: Process는 항상 빈 결과

	afterProcess := time.Now()

	an.mu.Lock()
	require.Len(t, an.tsBuffer, 1)
	assert.False(t, an.tsBuffer[0].receivedAt.Before(beforeProcess))
	assert.False(t, an.tsBuffer[0].receivedAt.After(afterProcess))
	an.mu.Unlock()
}

// TestAggregateNode_슬라이딩윈도우_Eviction후_윈도우내메시지만집계 는 eviction 후 윈도우 내 메시지만 집계되는지 확인한다.
func TestAggregateNode_슬라이딩윈도우_Eviction후_윈도우내메시지만집계(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-eviction", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowSliding
	an.windowDur = 200 * time.Millisecond
	an.slideDuration = 50 * time.Millisecond
	an.aggregateFns = []AggregateFn{AggregateSum}
	an.fields = []string{"value"}
	an.tsBuffer = []timestampedMessage{}

	now := time.Now()

	// 오래된 메시지 (윈도우 밖): 300ms 전
	oldMsg := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 100.0})))
	an.tsBuffer = append(an.tsBuffer, timestampedMessage{msg: oldMsg, receivedAt: now.Add(-300 * time.Millisecond)})

	// 윈도우 내 메시지: 100ms 전
	recentMsg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 10.0})))
	an.tsBuffer = append(an.tsBuffer, timestampedMessage{msg: recentMsg1, receivedAt: now.Add(-100 * time.Millisecond)})

	// 윈도우 내 메시지: 50ms 전
	recentMsg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 20.0})))
	an.tsBuffer = append(an.tsBuffer, timestampedMessage{msg: recentMsg2, receivedAt: now.Add(-50 * time.Millisecond)})

	// 직접 slidingFlush 호출
	an.mu.Lock()
	an.slidingFlush()
	result := an.lastFlushResult
	an.mu.Unlock()

	require.NotNil(t, result)
	require.Len(t, result, 1)

	// 윈도우 내 메시지만 집계: 10 + 20 = 30 (100.0은 eviction)
	resultVal, ok := result[0].Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 30.0, resultVal)

	// eviction 후 버퍼에 2개만 남음
	an.mu.Lock()
	assert.Len(t, an.tsBuffer, 2)
	an.mu.Unlock()
}

// TestAggregateNode_슬라이딩윈도우_빈윈도우_스킵 은 빈 윈도우에서 플러시가 스킵되는지 확인한다.
func TestAggregateNode_슬라이딩윈도우_빈윈도우_스킵(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-empty-skip", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowSliding
	an.windowDur = 100 * time.Millisecond
	an.slideDuration = 50 * time.Millisecond
	an.aggregateFns = []AggregateFn{AggregateSum}
	an.fields = []string{"value"}
	an.tsBuffer = []timestampedMessage{}

	// 빈 버퍼로 slidingFlush 호출
	an.mu.Lock()
	an.slidingFlush()
	result := an.lastFlushResult
	an.mu.Unlock()

	// 빈 윈도우이므로 결과 없음
	assert.Nil(t, result)
}

// TestAggregateNode_슬라이딩윈도우_출력_윈도우시간포함 은 슬라이딩 윈도우 출력에 window_start/window_end가 포함되는지 확인한다.
func TestAggregateNode_슬라이딩윈도우_출력_윈도우시간포함(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-window-time", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowSliding
	an.windowDur = 1 * time.Second
	an.slideDuration = 100 * time.Millisecond
	an.aggregateFns = []AggregateFn{AggregateSum}
	an.fields = []string{"value"}
	an.tsBuffer = []timestampedMessage{}

	// 윈도우 내 메시지 추가
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 42.0})))
	an.tsBuffer = append(an.tsBuffer, timestampedMessage{msg: msg, receivedAt: time.Now()})

	an.mu.Lock()
	an.slidingFlush()
	result := an.lastFlushResult
	an.mu.Unlock()

	require.NotNil(t, result)
	require.Len(t, result, 1)

	// window_start, window_end 검증
	wsRaw, ok := result[0].Payload().Get("window_start")
	require.True(t, ok)
	ws, ok := wsRaw.(string)
	require.True(t, ok)
	_, err := time.Parse(time.RFC3339, ws)
	assert.NoError(t, err, "window_start가 RFC3339 형식이어야 한다")

	weRaw, ok := result[0].Payload().Get("window_end")
	require.True(t, ok)
	we, ok := weRaw.(string)
	require.True(t, ok)
	_, err = time.Parse(time.RFC3339, we)
	assert.NoError(t, err, "window_end가 RFC3339 형식이어야 한다")

	// window_end > window_start
	startTime, _ := time.Parse(time.RFC3339, ws)
	endTime, _ := time.Parse(time.RFC3339, we)
	assert.True(t, endTime.After(startTime) || endTime.Equal(startTime), "window_end >= window_start")
}

// --- M6: 그룹 모드 슬라이딩 윈도우 테스트 ---

// TestAggregateNode_슬라이딩윈도우_그룹모드_독립윈도우 는 그룹 모드에서 각 그룹이 독립적인 슬라이딩 윈도우를 가지는지 확인한다.
func TestAggregateNode_슬라이딩윈도우_그룹모드_독립윈도우(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-grp-ind", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":    "sliding",
		"window_size":    "500ms",
		"slide_interval": "100ms",
		"aggregate_fn":   "avg",
		"field":          "temperature",
		"group_by":       "location",
	})
	require.NoError(t, err)

	ctx := context.Background()

	// 그룹 A: seoul
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "seoul",
		"temperature": 20.0,
	})))
	results, err := an.Process(ctx, msg1)
	require.NoError(t, err)
	assert.Empty(t, results)

	// 그룹 B: busan
	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"location":    "busan",
		"temperature": 30.0,
	})))
	results, err = an.Process(ctx, msg2)
	require.NoError(t, err)
	assert.Empty(t, results)

	// 각 그룹에 독립적으로 메시지가 저장되는지 확인
	an.mu.Lock()
	assert.Len(t, an.groupTsBuffers["seoul"], 1)
	assert.Len(t, an.groupTsBuffers["busan"], 1)
	an.mu.Unlock()
}

// TestAggregateNode_슬라이딩윈도우_그룹모드_Eviction 은 그룹 모드에서 그룹별 eviction이 올바르게 동작하는지 확인한다.
func TestAggregateNode_슬라이딩윈도우_그룹모드_Eviction(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-grp-evict", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowSliding
	an.windowDur = 200 * time.Millisecond
	an.slideDuration = 50 * time.Millisecond
	an.aggregateFns = []AggregateFn{AggregateSum}
	an.fields = []string{"value"}
	an.groupByKeys = []string{"location"}
	an.maxGroups = 100
	an.groupTsBuffers = make(map[string][]timestampedMessage)

	now := time.Now()

	// 그룹 seoul: 1개 오래된(eviction 대상), 1개 최신
	an.groupTsBuffers["seoul"] = []timestampedMessage{
		{msg: message.New(message.WithPayload(message.NewPayload(map[string]any{"location": "seoul", "value": 100.0}))), receivedAt: now.Add(-500 * time.Millisecond)},
		{msg: message.New(message.WithPayload(message.NewPayload(map[string]any{"location": "seoul", "value": 10.0}))), receivedAt: now.Add(-50 * time.Millisecond)},
	}

	// 그룹 busan: 전부 최신
	an.groupTsBuffers["busan"] = []timestampedMessage{
		{msg: message.New(message.WithPayload(message.NewPayload(map[string]any{"location": "busan", "value": 20.0}))), receivedAt: now.Add(-100 * time.Millisecond)},
		{msg: message.New(message.WithPayload(message.NewPayload(map[string]any{"location": "busan", "value": 30.0}))), receivedAt: now.Add(-50 * time.Millisecond)},
	}

	an.mu.Lock()
	an.slidingFlushAllGroups()
	result := an.lastFlushResult
	an.mu.Unlock()

	require.NotNil(t, result)
	require.Len(t, result, 2)

	// 결과를 group_value로 분류
	resultMap := make(map[string]message.Message)
	for _, r := range result {
		gv, ok := r.Payload().Get("group_value")
		require.True(t, ok)
		resultMap[gv.(string)] = r
	}

	// seoul: 오래된 메시지 eviction 후 10.0만 남음
	seoulResult, ok := resultMap["seoul"]
	require.True(t, ok)
	seoulVal, ok := seoulResult.Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 10.0, seoulVal)

	// busan: 전부 윈도우 내, sum(20, 30) = 50
	busanResult, ok := resultMap["busan"]
	require.True(t, ok)
	busanVal, ok := busanResult.Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 50.0, busanVal)

	// 각 그룹에 window_start/window_end 확인
	_, ok = seoulResult.Payload().Get("window_start")
	assert.True(t, ok)
	_, ok = seoulResult.Payload().Get("window_end")
	assert.True(t, ok)
}

// TestAggregateNode_슬라이딩윈도우_그룹모드_MaxGroups초과_드롭 은 슬라이딩 그룹 모드에서 max_groups 초과 시 메시지가 드롭되는지 확인한다.
func TestAggregateNode_슬라이딩윈도우_그룹모드_MaxGroups초과_드롭(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-grp-maxg", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":    "sliding",
		"window_size":    "1s",
		"slide_interval": "100ms",
		"aggregate_fn":   "sum",
		"field":          "value",
		"group_by":       "location",
		"max_groups":     2,
	})
	require.NoError(t, err)

	ctx := context.Background()

	// 그룹 1: seoul
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"location": "seoul", "value": 10.0})))
	_, _ = an.Process(ctx, msg1)

	// 그룹 2: busan
	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"location": "busan", "value": 20.0})))
	_, _ = an.Process(ctx, msg2)

	// 그룹 3: jeju → max_groups 초과, 드롭
	msg3 := message.New(message.WithPayload(message.NewPayload(map[string]any{"location": "jeju", "value": 30.0})))
	results, err := an.Process(ctx, msg3)
	require.NoError(t, err)
	assert.Empty(t, results)

	an.mu.Lock()
	_, exists := an.groupTsBuffers["jeju"]
	assert.False(t, exists)
	assert.Len(t, an.groupTsBuffers["seoul"], 1)
	assert.Len(t, an.groupTsBuffers["busan"], 1)
	an.mu.Unlock()
}

// --- M7: 슬라이딩 윈도우 통합 테스트 ---

// TestAggregateNode_슬라이딩윈도우_Shutdown_잔여버퍼플러시 는 Shutdown 시 슬라이딩 윈도우 잔여 버퍼가 플러시되는지 확인한다.
func TestAggregateNode_슬라이딩윈도우_Shutdown_잔여버퍼플러시(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-shut-flush", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":    "sliding",
		"window_size":    "10s",
		"slide_interval": "1s",
		"aggregate_fn":   "sum",
		"field":          "value",
	})
	require.NoError(t, err)

	err = an.Init(context.Background())
	require.NoError(t, err)

	ctx := context.Background()

	// 메시지 추가
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 10.0})))
	_, _ = an.Process(ctx, msg1)
	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 20.0})))
	_, _ = an.Process(ctx, msg2)

	// Shutdown으로 잔여 버퍼 플러시
	err = an.Shutdown(context.Background())
	require.NoError(t, err)

	an.mu.Lock()
	result := an.lastFlushResult
	an.mu.Unlock()

	require.NotNil(t, result)
	require.Len(t, result, 1)

	resultVal, ok := result[0].Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 30.0, resultVal)
}

// TestAggregateNode_슬라이딩윈도우_Shutdown_그룹모드_잔여버퍼플러시 는 그룹 모드에서 Shutdown 시 잔여 버퍼가 플러시되는지 확인한다.
func TestAggregateNode_슬라이딩윈도우_Shutdown_그룹모드_잔여버퍼플러시(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-grp-shut", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":    "sliding",
		"window_size":    "10s",
		"slide_interval": "1s",
		"aggregate_fn":   "avg",
		"field":          "temperature",
		"group_by":       "location",
	})
	require.NoError(t, err)

	err = an.Init(context.Background())
	require.NoError(t, err)

	ctx := context.Background()

	// 여러 그룹에 메시지 추가
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"location": "seoul", "temperature": 20.0})))
	_, _ = an.Process(ctx, msg1)
	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"location": "busan", "temperature": 30.0})))
	_, _ = an.Process(ctx, msg2)
	msg3 := message.New(message.WithPayload(message.NewPayload(map[string]any{"location": "seoul", "temperature": 40.0})))
	_, _ = an.Process(ctx, msg3)

	// Shutdown
	err = an.Shutdown(context.Background())
	require.NoError(t, err)

	an.mu.Lock()
	result := an.lastFlushResult
	an.mu.Unlock()

	require.NotNil(t, result)
	require.Len(t, result, 2)

	// 결과를 group_value로 분류
	resultMap := make(map[string]message.Message)
	for _, r := range result {
		gv, ok := r.Payload().Get("group_value")
		require.True(t, ok)
		resultMap[gv.(string)] = r
	}

	// seoul: avg(20, 40) = 30
	seoulResult, ok := resultMap["seoul"]
	require.True(t, ok)
	seoulVal, ok := seoulResult.Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 30.0, seoulVal)

	// busan: avg(30) = 30
	busanResult, ok := resultMap["busan"]
	require.True(t, ok)
	busanVal, ok := busanResult.Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 30.0, busanVal)
}

// TestAggregateNode_슬라이딩윈도우_타이머_자동플러시 는 슬라이딩 윈도우 타이머가 주기적으로 플러시하는지 확인한다.
func TestAggregateNode_슬라이딩윈도우_타이머_자동플러시(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-timer-flush", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":    "sliding",
		"window_size":    "500ms",
		"slide_interval": "50ms",
		"aggregate_fn":   "sum",
		"field":          "value",
	})
	require.NoError(t, err)

	err = an.Init(context.Background())
	require.NoError(t, err)

	ctx := context.Background()

	// 메시지 추가
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 10.0})))
	_, _ = an.Process(ctx, msg1)
	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 20.0})))
	_, _ = an.Process(ctx, msg2)

	// 타이머(50ms) 만료 대기
	time.Sleep(100 * time.Millisecond)

	an.mu.Lock()
	result := an.lastFlushResult
	an.mu.Unlock()

	require.NotNil(t, result, "타이머 만료 후 플러시 결과가 있어야 한다")
	require.Len(t, result, 1)

	resultVal, ok := result[0].Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, 30.0, resultVal) // sum(10, 20) = 30

	// window_start/window_end 확인
	_, ok = result[0].Payload().Get("window_start")
	assert.True(t, ok)
	_, ok = result[0].Payload().Get("window_end")
	assert.True(t, ok)

	_ = an.Shutdown(context.Background())
}

// TestAggregateNode_슬라이딩윈도우_동시성_Race 는 슬라이딩 윈도우에서 동시성이 안전한지 확인한다.
func TestAggregateNode_슬라이딩윈도우_동시성_Race(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-race", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":    "sliding",
		"window_size":    "200ms",
		"slide_interval": "30ms",
		"aggregate_fn":   "sum",
		"field":          "value",
	})
	require.NoError(t, err)

	err = an.Init(context.Background())
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(val float64) {
			defer wg.Done()
			msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": val})))
			_, _ = an.Process(context.Background(), msg)
		}(float64(i))
	}
	wg.Wait()

	// 타이머 만료 대기
	time.Sleep(100 * time.Millisecond)

	_ = an.Shutdown(context.Background())
	// 레이스 조건 없이 완료되면 성공
}

// TestAggregateNode_슬라이딩윈도우_그룹모드_동시성_Race 는 슬라이딩 윈도우 그룹 모드에서 동시성이 안전한지 확인한다.
func TestAggregateNode_슬라이딩윈도우_그룹모드_동시성_Race(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-grp-race", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":    "sliding",
		"window_size":    "200ms",
		"slide_interval": "30ms",
		"aggregate_fn":   "sum",
		"field":          "value",
		"group_by":       "location",
		"max_groups":     10,
	})
	require.NoError(t, err)

	err = an.Init(context.Background())
	require.NoError(t, err)

	locations := []string{"seoul", "busan", "jeju", "incheon"}
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			loc := locations[idx%len(locations)]
			msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
				"location": loc,
				"value":    float64(idx),
			})))
			_, _ = an.Process(context.Background(), msg)
		}(i)
	}
	wg.Wait()

	// 타이머 만료 대기
	time.Sleep(100 * time.Millisecond)

	_ = an.Shutdown(context.Background())
	// 레이스 조건 없이 완료되면 성공
}

// TestAggregateNode_슬라이딩윈도우_기존윈도우_회귀없음 은 슬라이딩 윈도우 추가 후 기존 count/time 윈도우가 정상 동작하는지 확인한다.
func TestAggregateNode_슬라이딩윈도우_기존윈도우_회귀없음(t *testing.T) {
	// count 윈도우 회귀 테스트
	t.Run("count윈도우", func(t *testing.T) {
		def := flow.NewNodeDef("agg-sliding-compat-count", "aggregate")
		node, _ := NewAggregateNode(def)
		an := node.(*AggregateNode)

		err := an.Configure(map[string]any{
			"window_type":  "count",
			"window_size":  2,
			"aggregate_fn": "sum",
			"field":        "value",
		})
		require.NoError(t, err)

		ctx := context.Background()
		msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 10.0})))
		results, _ := an.Process(ctx, msg1)
		assert.Empty(t, results)

		msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 20.0})))
		results, err = an.Process(ctx, msg2)
		require.NoError(t, err)
		require.Len(t, results, 1)

		resultVal, ok := results[0].Payload().Get("result")
		require.True(t, ok)
		assert.Equal(t, 30.0, resultVal)
	})

	// time 윈도우 회귀 테스트
	t.Run("time윈도우", func(t *testing.T) {
		def := flow.NewNodeDef("agg-sliding-compat-time", "aggregate")
		node, _ := NewAggregateNode(def)
		an := node.(*AggregateNode)

		err := an.Configure(map[string]any{
			"window_type":  "time",
			"window_size":  "30ms",
			"aggregate_fn": "sum",
			"field":        "value",
		})
		require.NoError(t, err)

		err = an.Init(context.Background())
		require.NoError(t, err)

		ctx := context.Background()
		msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 10.0})))
		_, _ = an.Process(ctx, msg1)
		msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 20.0})))
		_, _ = an.Process(ctx, msg2)

		// 타이머 만료 대기
		time.Sleep(60 * time.Millisecond)

		an.mu.Lock()
		result := an.lastFlushResult
		an.mu.Unlock()

		require.NotNil(t, result)
		require.Len(t, result, 1)

		resultVal, ok := result[0].Payload().Get("result")
		require.True(t, ok)
		assert.Equal(t, 30.0, resultVal)

		_ = an.Shutdown(context.Background())
	})

	// count + group_by 회귀 테스트
	t.Run("count_group_by윈도우", func(t *testing.T) {
		def := flow.NewNodeDef("agg-sliding-compat-grp", "aggregate")
		node, _ := NewAggregateNode(def)
		an := node.(*AggregateNode)

		err := an.Configure(map[string]any{
			"window_type":  "count",
			"window_size":  2,
			"aggregate_fn": "avg",
			"field":        "temperature",
			"group_by":     "location",
		})
		require.NoError(t, err)

		ctx := context.Background()

		msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"location": "seoul", "temperature": 20.0})))
		_, _ = an.Process(ctx, msg1)
		msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"location": "seoul", "temperature": 30.0})))
		results, err := an.Process(ctx, msg2)
		require.NoError(t, err)
		require.Len(t, results, 1)

		resultVal, ok := results[0].Payload().Get("result")
		require.True(t, ok)
		assert.Equal(t, 25.0, resultVal)

		groupValue, ok := results[0].Payload().Get("group_value")
		require.True(t, ok)
		assert.Equal(t, "seoul", groupValue)
	})
}

// TestAggregateNode_슬라이딩윈도우_Eviction_모든메시지만료 는 모든 메시지가 만료된 경우 빈 윈도우로 처리되는지 확인한다.
func TestAggregateNode_슬라이딩윈도우_Eviction_모든메시지만료(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-all-expired", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowSliding
	an.windowDur = 100 * time.Millisecond
	an.slideDuration = 50 * time.Millisecond
	an.aggregateFns = []AggregateFn{AggregateSum}
	an.fields = []string{"value"}
	an.tsBuffer = []timestampedMessage{}

	now := time.Now()

	// 모든 메시지가 윈도우 밖 (오래됨)
	an.tsBuffer = append(an.tsBuffer, timestampedMessage{
		msg:        message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 100.0}))),
		receivedAt: now.Add(-500 * time.Millisecond),
	})
	an.tsBuffer = append(an.tsBuffer, timestampedMessage{
		msg:        message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 200.0}))),
		receivedAt: now.Add(-300 * time.Millisecond),
	})

	// 기존 lastFlushResult 설정 (이전 결과)
	an.lastFlushResult = []message.Message{message.New()}

	an.mu.Lock()
	an.slidingFlush()
	result := an.lastFlushResult
	an.mu.Unlock()

	// 모든 메시지가 eviction되었으므로 lastFlushResult는 이전 값 유지
	assert.Len(t, result, 1) // 이전 결과 유지

	// 버퍼가 비어있어야 함
	an.mu.Lock()
	assert.Len(t, an.tsBuffer, 0)
	an.mu.Unlock()
}

// TestAggregateNode_슬라이딩윈도우_슬라이드간격_윈도우크기동일 은 slide_interval == window_size일 때 정상 동작하는지 확인한다.
func TestAggregateNode_슬라이딩윈도우_슬라이드간격_윈도우크기동일(t *testing.T) {
	def := flow.NewNodeDef("agg-sliding-si-eq-ws", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":    "sliding",
		"window_size":    "100ms",
		"slide_interval": "100ms",
		"aggregate_fn":   "sum",
		"field":          "value",
	})
	require.NoError(t, err)
	assert.Equal(t, 100*time.Millisecond, an.slideDuration)
	assert.Equal(t, 100*time.Millisecond, an.windowDur)
}

// --- Info 테스트 ---

// TestAggregateNode_Info_빈버퍼 는 버퍼가 비어있을 때 Info()가 올바른 메타데이터를 반환하는지 확인한다.
func TestAggregateNode_Info_빈버퍼(t *testing.T) {
	def := flow.NewNodeDef("agg-info-empty", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  5,
		"aggregate_fn": "sum",
		"field":        "temperature",
	})
	require.NoError(t, err)

	info := an.Info()
	assert.Equal(t, "count", info["window_type"])
	assert.Equal(t, 5, info["window_size"])
	assert.Equal(t, []string{"sum"}, info["aggregate_functions"])
	assert.Equal(t, []string{"temperature"}, info["fields"])
	assert.Equal(t, 0, info["buffer_size"])
	assert.Nil(t, info["current_stats"])
	// 비그룹 모드이므로 그룹 관련 키 없음
	assert.Nil(t, info["group_by"])
}

// TestAggregateNode_Info_카운트윈도우_부분집계 는 카운트 윈도우에서 부분 집계 결과를 확인한다.
func TestAggregateNode_Info_카운트윈도우_부분집계(t *testing.T) {
	def := flow.NewNodeDef("agg-info-partial", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  5,
		"aggregate_fn": []any{"sum", "avg"},
		"fields":       []any{"temperature"},
	})
	require.NoError(t, err)
	require.NoError(t, an.Init(context.Background()))

	// 3개 메시지 투입 (윈도우 5개 미달 → 버퍼에 잔류)
	for _, temp := range []float64{20.0, 30.0, 40.0} {
		msg := message.New()
		msg.Payload().Set("temperature", temp)
		_, err := an.Process(context.Background(), msg)
		require.NoError(t, err)
	}

	info := an.Info()
	assert.Equal(t, 3, info["buffer_size"])
	assert.Equal(t, []string{"sum", "avg"}, info["aggregate_functions"])

	stats, ok := info["current_stats"].(map[string]any)
	require.True(t, ok)
	tempStats, ok := stats["temperature"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 90.0, tempStats["sum"])
	assert.Equal(t, 30.0, tempStats["avg"])
}

// TestAggregateNode_Info_그룹모드 는 그룹 모드에서 그룹별 통계를 확인한다.
func TestAggregateNode_Info_그룹모드(t *testing.T) {
	def := flow.NewNodeDef("agg-info-group", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  10,
		"aggregate_fn": "sum",
		"field":        "value",
		"group_by":     "sensor_id",
		"max_groups":   50,
	})
	require.NoError(t, err)
	require.NoError(t, an.Init(context.Background()))

	// 2개 그룹에 메시지 투입
	for _, item := range []struct {
		sensorID string
		value    float64
	}{
		{"s1", 10.0}, {"s1", 20.0}, {"s2", 100.0},
	} {
		msg := message.New()
		msg.Payload().Set("sensor_id", item.sensorID)
		msg.Payload().Set("value", item.value)
		_, err := an.Process(context.Background(), msg)
		require.NoError(t, err)
	}

	info := an.Info()
	assert.Equal(t, 3, info["buffer_size"])
	assert.Equal(t, 2, info["group_count"])
	assert.Equal(t, []string{"sensor_id"}, info["group_by"])
	assert.Equal(t, 50, info["max_groups"])

	stats, ok := info["current_stats"].(map[string]any)
	require.True(t, ok)

	// s1 그룹: sum=30
	s1, ok := stats["s1"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 2, s1["buffer_size"])
	s1Stats := s1["stats"].(map[string]any)
	s1Value := s1Stats["value"].(map[string]any)
	assert.Equal(t, 30.0, s1Value["sum"])

	// s2 그룹: sum=100
	s2, ok := stats["s2"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 1, s2["buffer_size"])
	s2Stats := s2["stats"].(map[string]any)
	s2Value := s2Stats["value"].(map[string]any)
	assert.Equal(t, 100.0, s2Value["sum"])
}

// TestAggregateNode_Info_타임윈도우 는 타임 윈도우 설정에서 window_size가 duration 문자열로 반환되는지 확인한다.
func TestAggregateNode_Info_타임윈도우(t *testing.T) {
	def := flow.NewNodeDef("agg-info-time", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "time",
		"window_size":  "30s",
		"aggregate_fn": "avg",
		"field":        "humidity",
	})
	require.NoError(t, err)

	info := an.Info()
	assert.Equal(t, "time", info["window_type"])
	assert.Equal(t, "30s", info["window_size"])
}

// TestAggregateNode_Info_슬라이딩윈도우 는 슬라이딩 윈도우 설정에서 slide_interval이 포함되는지 확인한다.
func TestAggregateNode_Info_슬라이딩윈도우(t *testing.T) {
	def := flow.NewNodeDef("agg-info-sliding", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":    "sliding",
		"window_size":    "1m",
		"slide_interval": "10s",
		"aggregate_fn":   "avg",
		"field":          "value",
	})
	require.NoError(t, err)

	info := an.Info()
	assert.Equal(t, "sliding", info["window_type"])
	assert.Equal(t, "1m0s", info["window_size"])
	assert.Equal(t, "10s", info["slide_interval"])
}

// TestAggregateNode_Info_다중필드_다중함수 는 다중 필드+다중 함수 모드에서 모든 조합이 계산되는지 확인한다.
func TestAggregateNode_Info_다중필드_다중함수(t *testing.T) {
	def := flow.NewNodeDef("agg-info-multi", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	err := an.Configure(map[string]any{
		"window_type":  "count",
		"window_size":  100,
		"aggregate_fn": []any{"sum", "min", "max"},
		"fields":       []any{"temp", "humidity"},
	})
	require.NoError(t, err)
	require.NoError(t, an.Init(context.Background()))

	// 2개 메시지 투입
	for _, vals := range [][2]float64{{25.0, 60.0}, {35.0, 40.0}} {
		msg := message.New()
		msg.Payload().Set("temp", vals[0])
		msg.Payload().Set("humidity", vals[1])
		_, err := an.Process(context.Background(), msg)
		require.NoError(t, err)
	}

	info := an.Info()
	assert.Equal(t, 2, info["buffer_size"])
	assert.Equal(t, []string{"sum", "min", "max"}, info["aggregate_functions"])
	assert.Equal(t, []string{"temp", "humidity"}, info["fields"])

	stats, ok := info["current_stats"].(map[string]any)
	require.True(t, ok)

	// temp: sum=60, min=25, max=35
	tempStats := stats["temp"].(map[string]any)
	assert.Equal(t, 60.0, tempStats["sum"])
	assert.Equal(t, 25.0, tempStats["min"])
	assert.Equal(t, 35.0, tempStats["max"])

	// humidity: sum=100, min=40, max=60
	humStats := stats["humidity"].(map[string]any)
	assert.Equal(t, 100.0, humStats["sum"])
	assert.Equal(t, 40.0, humStats["min"])
	assert.Equal(t, 60.0, humStats["max"])
}
