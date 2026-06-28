package node

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	// WindowSliding 은 슬라이딩 윈도우를 나타낸다.
	// 윈도우 크기 내의 메시지를 유지하며, slide_interval 주기로 집계를 실행한다.
	WindowSliding WindowType = "sliding"
)

// timestampedMessage 는 메시지에 수신 시간을 추가한 래퍼 구조체이다.
// 슬라이딩 윈도우에서 메시지 만료 판단에 사용된다.
type timestampedMessage struct {
	msg        message.Message
	receivedAt time.Time
}

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
	windowSize      int           // 카운트 윈도우 크기
	windowDur       time.Duration // 타임 윈도우 지속시간
	windowSizeRaw   any           // 출력용 원본 윈도우 크기 값
	aggregateFns    []AggregateFn // 집계 함수 목록 (다중 함수 지원)
	fields          []string      // 집계 대상 필드 목록 (다중 필드 지원)
	buffer          []message.Message
	mu              sync.Mutex
	timer           *time.Timer
	lastFlushResult []message.Message // 타이머 플러시 결과 저장

	// SPEC-AGG-002: 그룹별 파티셔닝 필드
	groupByKeys  []string                     // 그룹 분류 기준 필드명 목록 (nil = 비그룹 모드)
	maxGroups    int                          // 최대 허용 그룹 수 (기본값: 100)
	groupBuffers map[string][]message.Message // 그룹별 독립 버퍼 맵

	// SPEC-AGG-002 M6: 슬라이딩 윈도우 필드
	slideDuration  time.Duration                   // slide_interval 파싱 결과
	tsBuffer       []timestampedMessage            // sliding 비그룹 모드 타임스탬프 버퍼
	groupTsBuffers map[string][]timestampedMessage // sliding 그룹별 타임스탬프 버퍼
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
	// 슬라이딩 윈도우인 경우 슬라이딩 타이머 시작
	if n.windowType == WindowSliding && n.slideDuration > 0 {
		n.startSlidingTimer()
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

		// SPEC-AGG-002: 그룹 모드와 비그룹 모드 분기
		if len(n.groupByKeys) > 0 {
			// 그룹 모드: 모든 비어있지 않은 그룹 플러시
			results := n.flushAllGroups()
			if len(results) > 0 {
				n.lastFlushResult = results
			}
		} else {
			// 비그룹 모드: 기존 단일 버퍼 플러시
			if len(n.buffer) > 0 {
				result := n.executeAggregate(n.buffer)
				n.lastFlushResult = result
				n.buffer = nil
			}
		}

		// 타이머 재시작
		if n.timer != nil {
			n.startTimer()
		}
	})
}

// startSlidingTimer 는 슬라이딩 윈도우 타이머를 시작한다.
// slideDuration 간격으로 slidingFlush 또는 slidingFlushAllGroups를 호출한다.
// 호출자가 mu 잠금을 보유해야 한다.
func (n *AggregateNode) startSlidingTimer() {
	n.timer = time.AfterFunc(n.slideDuration, func() {
		n.mu.Lock()
		defer n.mu.Unlock()

		if len(n.groupByKeys) > 0 {
			n.slidingFlushAllGroups()
		} else {
			n.slidingFlush()
		}

		// 타이머 재시작
		if n.timer != nil {
			n.startSlidingTimer()
		}
	})
}

// Process 는 메시지를 버퍼에 추가하고, 윈도우 조건이 충족되면 집계 결과를 반환한다.
func (n *AggregateNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// SPEC-AGG-002 M6: 슬라이딩 윈도우 모드 분기
	if n.windowType == WindowSliding {
		if len(n.groupByKeys) > 0 {
			return n.processSlidingGroupMode(msg), nil
		}
		n.tsBuffer = append(n.tsBuffer, timestampedMessage{msg: msg, receivedAt: time.Now()})
		return []message.Message{}, nil
	}

	// SPEC-AGG-002: 그룹 모드 분기
	if len(n.groupByKeys) > 0 {
		return n.processGroupMode(msg), nil
	}

	// 비그룹 모드: 기존 단일 버퍼 로직 (SPEC-AGG-001)
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

// processGroupMode 는 그룹 모드에서 메시지를 처리한다.
// 호출자가 mu 잠금을 보유해야 한다.
func (n *AggregateNode) processGroupMode(msg message.Message) []message.Message {
	groupKey := n.extractGroupKey(msg)

	// 그룹 존재 여부 확인
	_, exists := n.groupBuffers[groupKey]
	if !exists {
		// 신규 그룹: maxGroups 제한 확인
		if len(n.groupBuffers) >= n.maxGroups {
			log.Printf("[WARN] aggregate: max_groups(%d) 초과, 메시지 드롭 (group_key=%s)", n.maxGroups, groupKey)
			return []message.Message{}
		}
		n.groupBuffers[groupKey] = nil
	}

	// 그룹 버퍼에 메시지 추가
	n.groupBuffers[groupKey] = append(n.groupBuffers[groupKey], msg)

	// count 윈도우: 해당 그룹 버퍼 크기 확인
	if n.windowType == WindowCount && len(n.groupBuffers[groupKey]) >= n.windowSize {
		buf := n.groupBuffers[groupKey]
		results := n.flushGroup(groupKey, buf)
		n.groupBuffers[groupKey] = buf[:0]
		return results
	}

	// time 윈도우: 버퍼에 추가만 (타이머가 flushAllGroups 호출)
	return []message.Message{}
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
	case json.Number:
		// UseNumber() 디코딩 시 숫자는 string 밑바탕의 json.Number 로 들어온다.
		// Float64() 변환에 성공하면 숫자로 취급한다.
		f, err := val.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// toStringKey 는 단일 필드 값을 그룹 키 문자열로 변환한다.
// nil → "_unknown", string → 그대로, 기타 → fmt.Sprintf("%v", value)
func toStringKey(value any) string {
	if value == nil {
		return "_unknown"
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", value)
}

// extractGroupKey 는 메시지에서 그룹 키를 추출한다.
// 단일 키: 해당 필드 값을 문자열로 변환
// 복합 키: 각 필드 값을 "|"로 결합
// 호출자가 mu 잠금을 보유해야 한다.
func (n *AggregateNode) extractGroupKey(msg message.Message) string {
	if len(n.groupByKeys) == 1 {
		rawValue, _ := msg.Payload().Get(n.groupByKeys[0])
		return toStringKey(rawValue)
	}

	// 복합 키 모드
	parts := make([]string, len(n.groupByKeys))
	for i, key := range n.groupByKeys {
		rawValue, _ := msg.Payload().Get(key)
		parts[i] = toStringKey(rawValue)
	}
	return strings.Join(parts, "|")
}

// parseGroupValues 는 복합 키 결합 문자열을 필드명→값 맵으로 분해한다.
func (n *AggregateNode) parseGroupValues(compositeKey string) map[string]string {
	parts := strings.Split(compositeKey, "|")
	result := make(map[string]string, len(n.groupByKeys))
	for i, key := range n.groupByKeys {
		if i < len(parts) {
			result[key] = parts[i]
		}
	}
	return result
}

// flushGroup 은 단일 그룹 버퍼를 플러시하고 그룹 메타데이터를 추가한다.
// 기존 executeAggregate 로직을 재사용한다.
// 호출자가 mu 잠금을 보유해야 한다.
func (n *AggregateNode) flushGroup(groupKey string, buf []message.Message) []message.Message {
	results := n.executeAggregate(buf)

	for _, r := range results {
		// 페이로드에 그룹 정보 추가
		if len(n.groupByKeys) == 1 {
			// 단일 키 모드
			r.Payload().Set("group_key", n.groupByKeys[0])
			r.Payload().Set("group_value", groupKey)
		} else {
			// 복합 키 모드
			r.Payload().Set("group_keys", n.groupByKeys)
			r.Payload().Set("group_value", groupKey)
			r.Payload().Set("group_values", n.parseGroupValues(groupKey))
		}

		// 메타데이터에 그룹 정보 추가
		r.Metadata().Set("_group_keys", strings.Join(n.groupByKeys, ","))
		r.Metadata().Set("_group_value", groupKey)
	}

	return results
}

// flushAllGroups 는 비어있지 않은 모든 그룹 버퍼를 플러시한다.
// time 윈도우 타이머 만료 시 호출된다.
// 호출자가 mu 잠금을 보유해야 한다.
func (n *AggregateNode) flushAllGroups() []message.Message {
	var allResults []message.Message
	for key, buf := range n.groupBuffers {
		if len(buf) > 0 {
			results := n.flushGroup(key, buf)
			allResults = append(allResults, results...)
			n.groupBuffers[key] = buf[:0]
		}
	}
	return allResults
}

// processSlidingGroupMode 는 슬라이딩 윈도우 그룹 모드에서 메시지를 처리한다.
// 호출자가 mu 잠금을 보유해야 한다.
func (n *AggregateNode) processSlidingGroupMode(msg message.Message) []message.Message {
	groupKey := n.extractGroupKey(msg)

	// 그룹 존재 여부 확인
	_, exists := n.groupTsBuffers[groupKey]
	if !exists {
		// 신규 그룹: maxGroups 제한 확인
		if len(n.groupTsBuffers) >= n.maxGroups {
			log.Printf("[WARN] aggregate: max_groups(%d) 초과, 메시지 드롭 (group_key=%s)", n.maxGroups, groupKey)
			return []message.Message{}
		}
		n.groupTsBuffers[groupKey] = nil
	}

	n.groupTsBuffers[groupKey] = append(n.groupTsBuffers[groupKey], timestampedMessage{msg: msg, receivedAt: time.Now()})
	return []message.Message{}
}

// slidingFlush 는 비그룹 모드에서 슬라이딩 윈도우 플러시를 수행한다.
// 윈도우 크기 밖의 오래된 메시지를 제거하고, 남은 메시지에 대해 집계를 실행한다.
// 호출자가 mu 잠금을 보유해야 한다.
func (n *AggregateNode) slidingFlush() {
	now := time.Now()
	cutoff := now.Add(-n.windowDur)

	// 오래된 메시지 제거 (eviction)
	validIdx := 0
	for _, ts := range n.tsBuffer {
		if !ts.receivedAt.Before(cutoff) {
			n.tsBuffer[validIdx] = ts
			validIdx++
		}
	}
	n.tsBuffer = n.tsBuffer[:validIdx]

	if len(n.tsBuffer) == 0 {
		return // 빈 윈도우 스킵
	}

	// 메시지 추출
	msgs := make([]message.Message, len(n.tsBuffer))
	for i, ts := range n.tsBuffer {
		msgs[i] = ts.msg
	}

	results := n.executeAggregate(msgs)

	// window_start/window_end 추가
	for _, r := range results {
		r.Payload().Set("window_start", cutoff.Format(time.RFC3339))
		r.Payload().Set("window_end", now.Format(time.RFC3339))
	}

	n.lastFlushResult = results
}

// slidingFlushAllGroups 는 그룹 모드에서 슬라이딩 윈도우 플러시를 수행한다.
// 모든 그룹의 오래된 메시지를 제거하고, 비어있지 않은 그룹에 대해 집계를 실행한다.
// 호출자가 mu 잠금을 보유해야 한다.
func (n *AggregateNode) slidingFlushAllGroups() {
	now := time.Now()
	cutoff := now.Add(-n.windowDur)

	var allResults []message.Message

	for key, tsBuf := range n.groupTsBuffers {
		// 그룹별 오래된 메시지 제거
		validIdx := 0
		for _, ts := range tsBuf {
			if !ts.receivedAt.Before(cutoff) {
				tsBuf[validIdx] = ts
				validIdx++
			}
		}
		n.groupTsBuffers[key] = tsBuf[:validIdx]

		if len(n.groupTsBuffers[key]) == 0 {
			continue // 빈 그룹 스킵
		}

		// 메시지 추출
		msgs := make([]message.Message, len(n.groupTsBuffers[key]))
		for i, ts := range n.groupTsBuffers[key] {
			msgs[i] = ts.msg
		}

		results := n.flushGroup(key, msgs)

		// window_start/window_end 추가
		for _, r := range results {
			r.Payload().Set("window_start", cutoff.Format(time.RFC3339))
			r.Payload().Set("window_end", now.Format(time.RFC3339))
		}

		allResults = append(allResults, results...)
	}

	if len(allResults) > 0 {
		n.lastFlushResult = allResults
	}
}

// Info 는 AggregateNode의 현재 집계 상태 정보를 반환한다.
// 버퍼 크기, 윈도우 설정, 그룹 정보, 현재 버퍼의 부분 집계 결과를 포함한다.
func (n *AggregateNode) Info() map[string]any {
	n.mu.Lock()
	defer n.mu.Unlock()

	info := map[string]any{
		"window_type": string(n.windowType),
	}

	// 윈도우 크기
	if n.windowType == WindowCount {
		info["window_size"] = n.windowSize
	} else if n.windowDur > 0 {
		info["window_size"] = n.windowDur.String()
	}

	// 집계 함수 목록
	fns := make([]string, len(n.aggregateFns))
	for i, fn := range n.aggregateFns {
		fns[i] = string(fn)
	}
	info["aggregate_functions"] = fns
	info["fields"] = n.fields

	// 슬라이딩 윈도우 추가 정보
	if n.windowType == WindowSliding && n.slideDuration > 0 {
		info["slide_interval"] = n.slideDuration.String()
	}

	// 그룹 모드 정보
	if len(n.groupByKeys) > 0 {
		info["group_by"] = n.groupByKeys
		info["max_groups"] = n.maxGroups
	}

	// 현재 버퍼 상태 및 부분 집계
	if len(n.groupByKeys) > 0 {
		info["buffer_size"], info["group_count"], info["current_stats"] = n.groupBufferStats()
	} else {
		info["buffer_size"], info["current_stats"] = n.singleBufferStats()
	}

	return info
}

// singleBufferStats 는 비그룹 모드의 버퍼 통계를 반환한다.
// 호출자가 mu 잠금을 보유해야 한다.
func (n *AggregateNode) singleBufferStats() (int, map[string]any) {
	var buf []message.Message

	if n.windowType == WindowSliding {
		// 슬라이딩 윈도우: 타임스탬프 버퍼에서 메시지 추출
		buf = make([]message.Message, len(n.tsBuffer))
		for i, ts := range n.tsBuffer {
			buf[i] = ts.msg
		}
	} else {
		buf = n.buffer
	}

	if len(buf) == 0 {
		return 0, nil
	}

	stats := n.computeBufferStats(buf)
	return len(buf), stats
}

// groupBufferStats 는 그룹 모드의 버퍼 통계를 반환한다.
// 총 버퍼 크기, 그룹 수, 그룹별 통계를 반환한다.
// 호출자가 mu 잠금을 보유해야 한다.
func (n *AggregateNode) groupBufferStats() (int, int, map[string]any) {
	totalSize := 0
	groups := make(map[string]any)

	if n.windowType == WindowSliding {
		for key, tsBuf := range n.groupTsBuffers {
			if len(tsBuf) == 0 {
				continue
			}
			totalSize += len(tsBuf)
			msgs := make([]message.Message, len(tsBuf))
			for i, ts := range tsBuf {
				msgs[i] = ts.msg
			}
			groups[key] = map[string]any{
				"buffer_size": len(tsBuf),
				"stats":       n.computeBufferStats(msgs),
			}
		}
	} else {
		for key, buf := range n.groupBuffers {
			if len(buf) == 0 {
				continue
			}
			totalSize += len(buf)
			groups[key] = map[string]any{
				"buffer_size": len(buf),
				"stats":       n.computeBufferStats(buf),
			}
		}
	}

	if len(groups) == 0 {
		return 0, 0, nil
	}
	return totalSize, len(groups), groups
}

// computeBufferStats 는 버퍼 내 메시지에 대해 현재 부분 집계 결과를 계산한다.
// 호출자가 mu 잠금을 보유해야 한다.
func (n *AggregateNode) computeBufferStats(buf []message.Message) map[string]any {
	if len(buf) == 0 {
		return nil
	}

	stats := make(map[string]any)
	for _, field := range n.fields {
		values := extractNumericValuesForField(buf, field)
		fieldStats := make(map[string]any)
		for _, fn := range n.aggregateFns {
			fieldStats[string(fn)] = computeAggregate(fn, values, buf, field)
		}
		stats[field] = fieldStats
	}
	return stats
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

	// SPEC-AGG-002 M7: 슬라이딩 윈도우 잔여 버퍼 플러시
	if n.windowType == WindowSliding {
		if len(n.groupByKeys) > 0 {
			// 그룹 모드: 모든 그룹의 잔여 타임스탬프 버퍼 플러시
			var allResults []message.Message
			for key, tsBuf := range n.groupTsBuffers {
				if len(tsBuf) == 0 {
					continue
				}
				msgs := make([]message.Message, len(tsBuf))
				for i, ts := range tsBuf {
					msgs[i] = ts.msg
				}
				results := n.flushGroup(key, msgs)
				allResults = append(allResults, results...)
				n.groupTsBuffers[key] = nil
			}
			if len(allResults) > 0 {
				n.lastFlushResult = allResults
			}
		} else {
			// 비그룹 모드: 잔여 타임스탬프 버퍼 플러시
			if len(n.tsBuffer) > 0 {
				msgs := make([]message.Message, len(n.tsBuffer))
				for i, ts := range n.tsBuffer {
					msgs[i] = ts.msg
				}
				result := n.executeAggregate(msgs)
				n.lastFlushResult = result
				n.tsBuffer = nil
			}
		}
	} else if len(n.groupByKeys) > 0 {
		// SPEC-AGG-002: 그룹 모드 잔여 버퍼 플러시
		results := n.flushAllGroups()
		if len(results) > 0 {
			n.lastFlushResult = results
		}
	} else {
		// 비그룹 모드: 기존 잔여 버퍼 플러시
		if len(n.buffer) > 0 {
			result := n.executeAggregate(n.buffer)
			n.lastFlushResult = result
			n.buffer = nil
		}
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
			case WindowCount, WindowTime, WindowSliding:
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
		case WindowTime, WindowSliding:
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

	// SPEC-AGG-002: group_by 파싱
	if gb, ok := config["group_by"]; ok {
		switch v := gb.(type) {
		case string:
			if v == "" {
				return fmt.Errorf("%w", ErrAggregateGroupByInvalid)
			}
			n.groupByKeys = []string{v}
		case []any:
			if len(v) == 0 {
				return fmt.Errorf("%w", ErrAggregateGroupByInvalid)
			}
			keys := make([]string, 0, len(v))
			for _, item := range v {
				s, ok := item.(string)
				if !ok {
					return fmt.Errorf("%w", ErrAggregateGroupByInvalid)
				}
				if s == "" {
					return fmt.Errorf("%w", ErrAggregateGroupByInvalid)
				}
				keys = append(keys, s)
			}
			n.groupByKeys = keys
		default:
			return fmt.Errorf("%w", ErrAggregateGroupByInvalid)
		}
	}

	// SPEC-AGG-002: max_groups 파싱
	if mg, ok := config["max_groups"]; ok {
		switch v := mg.(type) {
		case int:
			if v <= 0 {
				return fmt.Errorf("%w", ErrAggregateMaxGroupsInvalid)
			}
			n.maxGroups = v
		case float64:
			iv := int(v)
			if iv <= 0 {
				return fmt.Errorf("%w", ErrAggregateMaxGroupsInvalid)
			}
			n.maxGroups = iv
		default:
			return fmt.Errorf("%w", ErrAggregateMaxGroupsInvalid)
		}
	}

	// SPEC-AGG-002: group_by 설정 시 그룹 버퍼 초기화 + maxGroups 기본값
	if len(n.groupByKeys) > 0 {
		n.groupBuffers = make(map[string][]message.Message)
		if n.maxGroups == 0 {
			n.maxGroups = 100
		}
	}

	// SPEC-AGG-002 M6: 슬라이딩 윈도우 slide_interval 파싱 및 버퍼 초기화
	if n.windowType == WindowSliding {
		if si, ok := config["slide_interval"]; ok {
			if siStr, ok := si.(string); ok {
				dur, err := time.ParseDuration(siStr)
				if err != nil {
					return fmt.Errorf("%w: %v", ErrAggregateSlideIntervalParse, err)
				}
				if dur <= 0 {
					return fmt.Errorf("%w: must be > 0", ErrAggregateSlideIntervalParse)
				}
				if dur > n.windowDur {
					return fmt.Errorf("%w", ErrAggregateSlideIntervalInvalid)
				}
				n.slideDuration = dur
			} else {
				return fmt.Errorf("%w: slide_interval must be a string", ErrAggregateSlideIntervalParse)
			}
		} else {
			// 기본값: windowDur / 10
			n.slideDuration = n.windowDur / 10
		}

		// 슬라이딩 버퍼 초기화
		if len(n.groupByKeys) > 0 {
			n.groupTsBuffers = make(map[string][]timestampedMessage)
		} else {
			n.tsBuffer = []timestampedMessage{}
		}
	}

	return nil
}
