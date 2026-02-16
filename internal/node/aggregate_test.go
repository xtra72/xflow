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
	assert.Equal(t, AggregateSum, an.aggregateFn)
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
	assert.Equal(t, AggregateAvg, an.aggregateFn)
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
	an.aggregateFn = AggregateSum

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
	an.aggregateFn = AggregateAvg

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
	an.aggregateFn = AggregateCount

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
	an.aggregateFn = AggregateMin

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
	an.aggregateFn = AggregateMax

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
	an.aggregateFn = AggregateFirst

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
	an.aggregateFn = AggregateLast

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
	an.aggregateFn = AggregateSum

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
	an.aggregateFn = AggregateSum

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
	an.aggregateFn = AggregateSum

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
	an.aggregateFn = AggregateSum

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

// --- 동시성 테스트 ---

// TestAggregateNode_동시성안전_Process 는 Process가 동시성 안전한지 확인한다.
func TestAggregateNode_동시성안전_Process(t *testing.T) {
	def := flow.NewNodeDef("agg-conc", "aggregate")
	node, _ := NewAggregateNode(def)
	an := node.(*AggregateNode)

	an.windowType = WindowCount
	an.windowSize = 100
	an.aggregateFn = AggregateCount

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
			an.aggregateFn = tt.fn

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
