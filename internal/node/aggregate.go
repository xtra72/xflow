package node

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// WindowType 은 집계 윈도우 타입을 나타내는 문자열 타입이다.
type WindowType string

const (
	// WindowCount 는 메시지 개수 기반 윈도우를 나타낸다.
	WindowCount WindowType = "count"
	// WindowTime 은 시간 기반 윈도우를 나타낸다.
	WindowTime WindowType = "time"
)

// AggregateFn 은 집계 함수 타입을 나타내는 문자열 타입이다.
type AggregateFn string

const (
	// AggregateSum 은 합계 집계 함수를 나타낸다.
	AggregateSum AggregateFn = "sum"
	// AggregateAvg 는 평균 집계 함수를 나타낸다.
	AggregateAvg AggregateFn = "avg"
	// AggregateCount 는 개수 집계 함수를 나타낸다.
	AggregateCount AggregateFn = "count"
	// AggregateMin 은 최솟값 집계 함수를 나타낸다.
	AggregateMin AggregateFn = "min"
	// AggregateMax 는 최댓값 집계 함수를 나타낸다.
	AggregateMax AggregateFn = "max"
	// AggregateFirst 는 첫 번째 값 집계 함수를 나타낸다.
	AggregateFirst AggregateFn = "first"
	// AggregateLast 는 마지막 값 집계 함수를 나타낸다.
	AggregateLast AggregateFn = "last"
)

// AggregateNode 는 다중 메시지를 집계하는 노드이다.
// 카운트 기반 또는 시간 기반 윈도우로 메시지를 버퍼링한 후 집계 함수를 실행한다.
type AggregateNode struct {
	*BaseNode
	windowType      WindowType
	windowSize      int              // 카운트 윈도우 크기
	windowDur       time.Duration    // 타임 윈도우 지속시간
	windowSizeRaw   any              // 출력용 원본 윈도우 크기 값
	aggregateFns    []AggregateFn    // 집계 함수 목록 (다중 함수 지원)
	fields          []string         // 집계 대상 필드 목록 (다중 필드 지원)
	buffer          []message.Message
	mu              sync.Mutex
	timer           *time.Timer
	lastFlushResult []message.Message // 타이머 플러시 결과 저장
}

// NewAggregateNode 는 새로운 AggregateNode를 생성하는 팩토리 함수이다.
func NewAggregateNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &AggregateNode{
		BaseNode: base,
	}
	return n, nil
}

// Init 은 AggregateNode를 초기화한다.
// 타임 윈도우가 설정되어 있으면 주기적 타이머를 시작한다.
func (n *AggregateNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	// 타임 윈도우인 경우 타이머 시작
	n.mu.Lock()
	if n.windowType == WindowTime && n.windowDur > 0 {
		n.startTimer()
	}
	n.mu.Unlock()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// startTimer 는 타임 윈도우 타이머를 시작한다.
// 호출자가 mu 잠금을 보유해야 한다.
func (n *AggregateNode) startTimer() {
	n.timer = time.AfterFunc(n.windowDur, func() {
		n.mu.Lock()
		defer n.mu.Unlock()

		if len(n.buffer) > 0 {
			result := n.executeAggregate(n.buffer)
			n.lastFlushResult = result
			n.buffer = nil
		}

		// 타이머 재시작
		if n.timer != nil {
			n.startTimer()
		}
	})
}

// Process 는 메시지를 버퍼에 추가하고, 윈도우 조건이 충족되면 집계 결과를 반환한다.
func (n *AggregateNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.buffer = append(n.buffer, msg)

	// 카운트 윈도우: 버퍼가 윈도우 크기에 도달하면 집계
	if n.windowType == WindowCount && len(n.buffer) >= n.windowSize {
		result := n.executeAggregate(n.buffer)
		n.buffer = nil
		return result, nil
	}

	// 타임 윈도우: Process에서는 버퍼에만 추가하고 빈 결과 반환
	return []message.Message{}, nil
}

// executeAggregate 는 버퍼의 메시지들에 대해 집계 함수를 실행한다.
// 호출자가 mu 잠금을 보유해야 한다.
// 다중 함수 또는 다중 필드인 경우 stats 매트릭스를 생성하고,
// 단일 함수+단일 필드인 경우 기존 result+count 포맷을 유지한다.
func (n *AggregateNode) executeAggregate(buf []message.Message) []message.Message {
	if len(buf) == 0 {
		return []message.Message{}
	}

	isMultiMode := len(n.aggregateFns) > 1 || len(n.fields) > 1

	// 집계 함수 이름 목록 (메타데이터용)
	fnNames := make([]string, len(n.aggregateFns))
	for i, fn := range n.aggregateFns {
		fnNames[i] = string(fn)
	}

	// 윈도우 크기 출력값 결정
	var windowSizeOutput any
	if n.windowSizeRaw != nil {
		windowSizeOutput = n.windowSizeRaw
	} else if n.windowType == WindowCount {
		windowSizeOutput = n.windowSize
	} else {
		windowSizeOutput = n.windowDur.String()
	}

	if isMultiMode {
		return n.executeMultiAggregate(buf, fnNames, windowSizeOutput)
	}
	return n.executeSingleAggregate(buf, fnNames, windowSizeOutput)
}

// executeSingleAggregate 는 단일 함수+단일 필드 모드의 집계를 실행한다.
// 기존 호환성을 위해 result+count 포맷을 유지하며, stats도 추가한다.
func (n *AggregateNode) executeSingleAggregate(buf []message.Message, fnNames []string, windowSizeOutput any) []message.Message {
	fn := n.aggregateFns[0]
	field := n.fields[0]

	var resultMsg message.Message

	switch fn {
	case AggregateFirst:
		// 첫 번째 메시지의 페이로드 사용
		first := buf[0]
		resultMsg = message.New(
			message.WithPayload(first.Payload().Clone()),
			message.WithMetadata("_aggregate_fn", fnNames[0]),
			message.WithMetadata("_window_type", string(n.windowType)),
			message.WithMetadata("_fields", field),
		)
		resultMsg.Payload().Set("count", len(buf))

		// stats 추가 (first의 경우 해당 필드의 첫 번째 값)
		firstVal := computeAggregate(fn, nil, buf, field)
		stats := map[string]any{
			field: map[string]any{
				string(fn): firstVal,
			},
		}
		resultMsg.Payload().Set("stats", stats)

	case AggregateLast:
		// 마지막 메시지의 페이로드 사용
		last := buf[len(buf)-1]
		resultMsg = message.New(
			message.WithPayload(last.Payload().Clone()),
			message.WithMetadata("_aggregate_fn", fnNames[0]),
			message.WithMetadata("_window_type", string(n.windowType)),
			message.WithMetadata("_fields", field),
		)
		resultMsg.Payload().Set("count", len(buf))

		// stats 추가
		lastVal := computeAggregate(fn, nil, buf, field)
		stats := map[string]any{
			field: map[string]any{
				string(fn): lastVal,
			},
		}
		resultMsg.Payload().Set("stats", stats)

	case AggregateCount:
		resultMsg = message.New(
			message.WithMetadata("_aggregate_fn", fnNames[0]),
			message.WithMetadata("_window_type", string(n.windowType)),
			message.WithMetadata("_fields", field),
		)
		resultMsg.Payload().Set("result", len(buf))
		resultMsg.Payload().Set("count", len(buf))

		// stats 추가 (count는 해당 필드의 유효한 숫자 값 개수)
		values := extractNumericValuesForField(buf, field)
		stats := map[string]any{
			field: map[string]any{
				string(fn): len(values),
			},
		}
		resultMsg.Payload().Set("stats", stats)

	default:
		// sum, avg, min, max - 숫자 기반 집계
		values := extractNumericValuesForField(buf, field)
		result := computeAggregate(fn, values, buf, field)

		resultMsg = message.New(
			message.WithMetadata("_aggregate_fn", fnNames[0]),
			message.WithMetadata("_window_type", string(n.windowType)),
			message.WithMetadata("_fields", field),
		)
		resultMsg.Payload().Set("result", result)
		resultMsg.Payload().Set("count", len(buf))

		// stats 추가
		stats := map[string]any{
			field: map[string]any{
				string(fn): result,
			},
		}
		resultMsg.Payload().Set("stats", stats)
	}

	resultMsg.Payload().Set("window_count", len(buf))
	resultMsg.Payload().Set("window_type", string(n.windowType))
	resultMsg.Payload().Set("window_size", windowSizeOutput)

	return []message.Message{resultMsg}
}

// executeMultiAggregate 는 다중 함수 또는 다중 필드 모드의 집계를 실행한다.
// stats 매트릭스: map[field]map[fn]value 형태의 결과를 생성한다.
func (n *AggregateNode) executeMultiAggregate(buf []message.Message, fnNames []string, windowSizeOutput any) []message.Message {
	stats := make(map[string]any)

	for _, field := range n.fields {
		values := extractNumericValuesForField(buf, field)
		fieldStats := make(map[string]any)

		for _, fn := range n.aggregateFns {
			fieldStats[string(fn)] = computeAggregate(fn, values, buf, field)
		}

		stats[field] = fieldStats
	}

	resultMsg := message.New(
		message.WithMetadata("_aggregate_fn", strings.Join(fnNames, ",")),
		message.WithMetadata("_window_type", string(n.windowType)),
		message.WithMetadata("_fields", strings.Join(n.fields, ",")),
	)
	resultMsg.Payload().Set("stats", stats)
	resultMsg.Payload().Set("window_count", len(buf))
	resultMsg.Payload().Set("window_type", string(n.windowType))
	resultMsg.Payload().Set("window_size", windowSizeOutput)

	return []message.Message{resultMsg}
}

// extractNumericValuesForField 는 메시지 버퍼에서 지정된 필드의 숫자 값을 추출한다.
// 비숫자 값이나 필드가 없는 메시지는 건너뛴다.
func extractNumericValuesForField(buf []message.Message, field string) []float64 {
	var values []float64
	for _, msg := range buf {
		v, ok := msg.Payload().Get(field)
		if !ok {
			continue
		}
		if f, ok := toFloat64(v); ok {
			values = append(values, f)
		}
	}
	return values
}

// computeAggregate 는 지정된 집계 함수로 값을 계산한다.
// values는 해당 필드의 숫자 값 슬라이스이다.
// buf와 field는 first/last 함수에서 원본 메시지 접근에 사용된다.
func computeAggregate(fn AggregateFn, values []float64, buf []message.Message, field string) any {
	switch fn {
	case AggregateSum:
		var sum float64
		for _, v := range values {
			sum += v
		}
		return sum

	case AggregateAvg:
		if len(values) == 0 {
			return float64(0)
		}
		var sum float64
		for _, v := range values {
			sum += v
		}
		return sum / float64(len(values))

	case AggregateMin:
		if len(values) == 0 {
			return float64(0)
		}
		min := math.MaxFloat64
		for _, v := range values {
			if v < min {
				min = v
			}
		}
		return min

	case AggregateMax:
		if len(values) == 0 {
			return float64(0)
		}
		max := -math.MaxFloat64
		for _, v := range values {
			if v > max {
				max = v
			}
		}
		return max

	case AggregateCount:
		return len(values)

	case AggregateFirst:
		// 버퍼에서 해당 필드의 첫 번째 숫자 값을 반환한다
		for _, msg := range buf {
			v, ok := msg.Payload().Get(field)
			if !ok {
				continue
			}
			if f, ok := toFloat64(v); ok {
				return f
			}
		}
		return float64(0)

	case AggregateLast:
		// 버퍼에서 해당 필드의 마지막 숫자 값을 반환한다
		for i := len(buf) - 1; i >= 0; i-- {
			v, ok := buf[i].Payload().Get(field)
			if !ok {
				continue
			}
			if f, ok := toFloat64(v); ok {
				return f
			}
		}
		return float64(0)

	default:
		return float64(0)
	}
}

// toFloat64 는 다양한 숫자 타입을 float64로 변환한다.
func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int8:
		return float64(val), true
	case int16:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint:
		return float64(val), true
	case uint8:
		return float64(val), true
	case uint16:
		return float64(val), true
	case uint32:
		return float64(val), true
	case uint64:
		return float64(val), true
	default:
		return 0, false
	}
}

// Shutdown 은 AggregateNode를 종료한다.
// 타이머를 정지하고 잔여 버퍼를 플러시한다.
func (n *AggregateNode) Shutdown(ctx context.Context) error {
	n.mu.Lock()
	// 타이머 정지
	if n.timer != nil {
		n.timer.Stop()
		n.timer = nil
	}

	// 잔여 버퍼 플러시
	if len(n.buffer) > 0 {
		result := n.executeAggregate(n.buffer)
		n.lastFlushResult = result
		n.buffer = nil
	}
	n.mu.Unlock()

	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// validAggregateFns 는 유효한 집계 함수 이름을 정의하는 맵이다.
var validAggregateFns = map[AggregateFn]bool{
	AggregateSum:   true,
	AggregateAvg:   true,
	AggregateCount: true,
	AggregateMin:   true,
	AggregateMax:   true,
	AggregateFirst: true,
	AggregateLast:  true,
}

// Configure 는 AggregateNode의 설정을 적용한다.
// "window_type", "window_size", "aggregate_fn", "fields", "field" 키를 사용한다.
func (n *AggregateNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	// 윈도우 타입 추출
	if wt, ok := config["window_type"]; ok {
		if wtStr, ok := wt.(string); ok {
			switch WindowType(wtStr) {
			case WindowCount, WindowTime:
				n.windowType = WindowType(wtStr)
			default:
				return fmt.Errorf("%w: unknown window type %q", ErrAggregateWindowInvalid, wtStr)
			}
		}
	}

	// 윈도우 크기 추출 및 원본 값 저장
	if ws, ok := config["window_size"]; ok {
		n.windowSizeRaw = ws
		switch n.windowType {
		case WindowCount:
			if size, ok := ws.(int); ok {
				if size <= 0 {
					return fmt.Errorf("%w: window size must be > 0", ErrAggregateWindowInvalid)
				}
				n.windowSize = size
			}
		case WindowTime:
			if durStr, ok := ws.(string); ok {
				dur, err := time.ParseDuration(durStr)
				if err != nil {
					return fmt.Errorf("%w: invalid duration %q: %v", ErrAggregateWindowInvalid, durStr, err)
				}
				if dur <= 0 {
					return fmt.Errorf("%w: window duration must be > 0", ErrAggregateWindowInvalid)
				}
				n.windowDur = dur
			}
		}
	}

	// 집계 함수 추출 (string 또는 []any 지원)
	if af, ok := config["aggregate_fn"]; ok {
		switch v := af.(type) {
		case string:
			fn := AggregateFn(v)
			if !validAggregateFns[fn] {
				return fmt.Errorf("%w: unknown function %q", ErrAggregateFnInvalid, v)
			}
			n.aggregateFns = []AggregateFn{fn}
		case []any:
			if len(v) == 0 {
				return fmt.Errorf("%w: function list must not be empty", ErrAggregateFnInvalid)
			}
			fns := make([]AggregateFn, 0, len(v))
			for _, item := range v {
				fnStr, ok := item.(string)
				if !ok {
					return fmt.Errorf("%w: function must be a string", ErrAggregateFnInvalid)
				}
				fn := AggregateFn(fnStr)
				if !validAggregateFns[fn] {
					return fmt.Errorf("%w: unknown function %q", ErrAggregateFnInvalid, fnStr)
				}
				fns = append(fns, fn)
			}
			n.aggregateFns = fns
		}
	}

	// fields 파싱 (우선순위: fields > field > 기본값 "value")
	if fs, ok := config["fields"]; ok {
		switch v := fs.(type) {
		case []any:
			if len(v) == 0 {
				return fmt.Errorf("%w: fields list must not be empty", ErrAggregateFieldInvalid)
			}
			fields := make([]string, 0, len(v))
			for _, item := range v {
				fStr, ok := item.(string)
				if !ok {
					return fmt.Errorf("%w: field must be a string", ErrAggregateFieldInvalid)
				}
				fields = append(fields, fStr)
			}
			n.fields = fields
		}
	} else if f, ok := config["field"]; ok {
		if fStr, ok := f.(string); ok {
			n.fields = []string{fStr}
		}
	}

	// fields 기본값 설정
	if len(n.fields) == 0 {
		n.fields = []string{"value"}
	}

	return nil
}
