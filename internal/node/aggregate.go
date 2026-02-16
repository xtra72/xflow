package node

import (
	"context"
	"fmt"
	"math"
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
	windowType     WindowType
	windowSize     int           // 카운트 윈도우 크기
	windowDur      time.Duration // 타임 윈도우 지속시간
	aggregateFn    AggregateFn
	buffer         []message.Message
	mu             sync.Mutex
	timer          *time.Timer
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
func (n *AggregateNode) executeAggregate(buf []message.Message) []message.Message {
	if len(buf) == 0 {
		return []message.Message{}
	}

	var resultMsg message.Message

	switch n.aggregateFn {
	case AggregateFirst:
		// 첫 번째 메시지의 페이로드 사용
		first := buf[0]
		resultMsg = message.New(
			message.WithPayload(first.Payload().Clone()),
			message.WithMetadata("_aggregate_fn", string(n.aggregateFn)),
			message.WithMetadata("_window_type", string(n.windowType)),
		)
		resultMsg.Payload().Set("count", len(buf))

	case AggregateLast:
		// 마지막 메시지의 페이로드 사용
		last := buf[len(buf)-1]
		resultMsg = message.New(
			message.WithPayload(last.Payload().Clone()),
			message.WithMetadata("_aggregate_fn", string(n.aggregateFn)),
			message.WithMetadata("_window_type", string(n.windowType)),
		)
		resultMsg.Payload().Set("count", len(buf))

	case AggregateCount:
		resultMsg = message.New(
			message.WithMetadata("_aggregate_fn", string(n.aggregateFn)),
			message.WithMetadata("_window_type", string(n.windowType)),
		)
		resultMsg.Payload().Set("result", len(buf))
		resultMsg.Payload().Set("count", len(buf))

	default:
		// sum, avg, min, max - 숫자 기반 집계
		values := n.extractNumericValues(buf)
		var result float64

		switch n.aggregateFn {
		case AggregateSum:
			for _, v := range values {
				result += v
			}
		case AggregateAvg:
			if len(values) > 0 {
				var sum float64
				for _, v := range values {
					sum += v
				}
				result = sum / float64(len(values))
			}
		case AggregateMin:
			if len(values) > 0 {
				result = math.MaxFloat64
				for _, v := range values {
					if v < result {
						result = v
					}
				}
			}
		case AggregateMax:
			if len(values) > 0 {
				result = -math.MaxFloat64
				for _, v := range values {
					if v > result {
						result = v
					}
				}
			}
		}

		resultMsg = message.New(
			message.WithMetadata("_aggregate_fn", string(n.aggregateFn)),
			message.WithMetadata("_window_type", string(n.windowType)),
		)
		resultMsg.Payload().Set("result", result)
		resultMsg.Payload().Set("count", len(buf))
	}

	return []message.Message{resultMsg}
}

// extractNumericValues 는 메시지 버퍼에서 숫자 값을 추출한다.
// 페이로드의 "value" 키에서 숫자를 추출하며, 비숫자 값은 건너뛴다.
func (n *AggregateNode) extractNumericValues(buf []message.Message) []float64 {
	var values []float64
	for _, msg := range buf {
		v, ok := msg.Payload().Get("value")
		if !ok {
			continue
		}
		if f, ok := toFloat64(v); ok {
			values = append(values, f)
		}
	}
	return values
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

// Configure 는 AggregateNode의 설정을 적용한다.
// "window_type", "window_size", "aggregate_fn" 키를 사용한다.
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

	// 윈도우 크기 추출
	if ws, ok := config["window_size"]; ok {
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

	// 집계 함수 추출
	if af, ok := config["aggregate_fn"]; ok {
		if afStr, ok := af.(string); ok {
			n.aggregateFn = AggregateFn(afStr)
		}
	}

	return nil
}
