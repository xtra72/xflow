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
