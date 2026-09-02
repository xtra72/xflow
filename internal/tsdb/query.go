package tsdb

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/fillpolicy"
)

// AggregateFunc 는 집계 함수 타입이다.
type AggregateFunc string

const (
	AggMin   AggregateFunc = "min"
	AggMax   AggregateFunc = "max"
	AggAvg   AggregateFunc = "avg"
	AggSum   AggregateFunc = "sum"
	AggCount AggregateFunc = "count"
	AggFirst AggregateFunc = "first"
	AggLast  AggregateFunc = "last"
)

// FillStrategy 는 버킷 다운샘플링 시 값이 없는(빈) 버킷을 채우는 방법이다.
type FillStrategy string

const (
	FillNone     FillStrategy = ""         // 빈 버킷 생략 (기본, 하위호환)
	FillNull     FillStrategy = "null"     // 빈 버킷을 null 값으로 표시 (비우기)
	FillZero     FillStrategy = "zero"     // 0 으로 채움
	FillPrevious FillStrategy = "previous" // 직전 비어있지 않은 값으로 carry-forward
	FillAvg      FillStrategy = "avg"      // 전/후 비어있지 않은 값의 평균 (양쪽 없으면 한쪽, 둘 다 없으면 null)
)

// Query 는 시계열 쿼리를 정의한다.
type Query struct {
	SeriesKey      string            // 특정 시리즈 키 (빈 문자열이면 필터 사용)
	Measurement    string            // measurement 기반 필터
	TagFilters     map[string]string // 태그 키-값 필터
	Start          time.Time         // 시작 시간
	End            time.Time         // 종료 시간
	Field          string            // 집계할 필드 이름 (빈 문자열이면 모든 필드)
	Aggregation    AggregateFunc     // 집계 함수 (빈 문자열이면 raw 반환)
	BucketInterval time.Duration     // 다운샘플링 버킷 간격 (0이면 전체 범위)
	Fill           FillStrategy      // 빈 버킷 채우기 전략 (BucketInterval>0 일 때만 의미. ""=빈 버킷 생략)
	// FillPrevious 채우기의 사용 기간 제한. 제로값이면 종전대로 무제한 이어 쓴다.
	// 다른 채우기 전략에서는 읽지 않는다.
	FillPreviousLimit fillpolicy.Previous
	Limit             int // 최대 반환 포인트 수 (0이면 config 기본값)
}

// QueryResult 는 쿼리 결과를 정의한다.
type QueryResult struct {
	SeriesKey string      // 원본 시리즈 키
	Points    []DataPoint // raw 또는 집계된 포인트
	Stats     QueryStats
}

// QueryStats 는 쿼리 통계를 정의한다.
type QueryStats struct {
	ScannedPoints  int           // 스캔한 원본 포인트 수
	ReturnedPoints int           // 반환된 포인트 수
	ExecutionTime  time.Duration // 실행 시간
}

// Execute 는 쿼리를 실행한다.
func (db *defaultTSDB) Execute(q Query) ([]QueryResult, error) {
	if db.closed.Load() {
		return nil, ErrTSDBClosed
	}

	startTime := time.Now()

	// 타임아웃 컨텍스트 설정
	timeout := db.config.QueryTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 대상 시리즈 키 결정
	var targetKeys []string
	if q.SeriesKey != "" {
		targetKeys = []string{q.SeriesKey}
	} else {
		targetKeys = db.FilterSeries(q.Measurement, q.TagFilters)
	}

	// 최대 반환 포인트 수 결정
	maxPoints := q.Limit
	if maxPoints <= 0 {
		maxPoints = db.config.MaxQueryPoints
	}

	var results []QueryResult
	totalReturnedPoints := 0

	for _, key := range targetKeys {
		// 타임아웃 체크
		select {
		case <-ctx.Done():
			return nil, ErrQueryTimeout
		default:
		}

		s, err := db.getSeries(key)
		if err != nil {
			continue // 시리즈를 찾을 수 없으면 건너뜀
		}

		var resultPoints []DataPoint
		var scannedPoints int

		if q.Aggregation == "" {
			// raw 반환 — 호출자에게 원본 전체를 넘기므로 복사가 필요하다.
			rawPoints := s.QueryRange(q.Start, q.End)
			scannedPoints = len(rawPoints)
			if scannedPoints == 0 {
				continue
			}
			resultPoints = rawPoints
		} else {
			// 집계 경로는 (시각, 값) 두 가지만 쓴다. DataPoint 를 복사하지 않고
			// 필요한 스칼라만 뽑아 온다 — 구간 크기에 비례하던 포인트당 맵 할당이
			// 사라지고, 락 보유 시간도 그만큼 짧아진다.
			scalars, scanned, firstTS := s.QueryRangeScalar(q.Start, q.End, q.Field)
			scannedPoints = scanned
			if scannedPoints == 0 {
				continue
			}

			if q.BucketInterval > 0 {
				// 다운샘플링. Fill 이 설정되면 [Start,End] 전 구간 버킷을 생성하고 빈 버킷을 채운다.
				if q.Fill == FillNone {
					resultPoints = downsampleScalars(scalars, q.Field, q.Aggregation, q.BucketInterval)
				} else {
					resultPoints = downsampleFilledScalars(scalars, q.Field, q.Aggregation, q.BucketInterval, q.Start, q.End, q.Fill, q.FillPreviousLimit)
				}
			} else {
				// 전체 범위 집계. 결과 시각은 구간 첫 **원본** 포인트의 시각이다
				// (값이 없는 선두 포인트도 시각을 정한다 — 기존 규약 유지).
				val, err := aggregateScalars(scalars, q.Aggregation)
				if err != nil {
					continue
				}
				resultPoints = []DataPoint{
					{
						Timestamp: firstTS,
						Fields:    map[string]any{resultFieldName(q.Field): val},
					},
				}
			}
		}

		// 포인트 수 제한 체크
		if maxPoints > 0 && totalReturnedPoints+len(resultPoints) > maxPoints {
			return nil, ErrMaxPointsExceeded
		}

		totalReturnedPoints += len(resultPoints)

		results = append(results, QueryResult{
			SeriesKey: key,
			Points:    resultPoints,
			Stats: QueryStats{
				ScannedPoints:  scannedPoints,
				ReturnedPoints: len(resultPoints),
				ExecutionTime:  time.Since(startTime),
			},
		})
	}

	return results, nil
}

// FilterSeries 는 measurement와 태그 필터로 시리즈 키를 검색한다.
func (db *defaultTSDB) FilterSeries(measurement string, tags map[string]string) []string {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var matched []string
	for key := range db.series {
		m, t := parseSeriesKey(key)

		// measurement 필터
		if measurement != "" && m != measurement {
			continue
		}

		// 태그 필터 (모든 태그 키-값이 일치해야 함)
		if len(tags) > 0 {
			match := true
			for tk, tv := range tags {
				if t[tk] != tv {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		matched = append(matched, key)
	}

	return matched
}

// parseSeriesKey 는 시리즈 키를 measurement와 태그로 파싱한다.
// 키 형식: "measurement,key1=val1,key2=val2"
func parseSeriesKey(key string) (measurement string, tags map[string]string) {
	tags = make(map[string]string)

	parts := strings.SplitN(key, ",", 2)
	measurement = parts[0]

	if len(parts) < 2 {
		return measurement, tags
	}

	tagParts := strings.Split(parts[1], ",")
	for _, tp := range tagParts {
		kv := strings.SplitN(tp, "=", 2)
		if len(kv) == 2 {
			tags[kv[0]] = kv[1]
		}
	}

	return measurement, tags
}

// scalarPoint 는 집계 경로가 실제로 쓰는 최소 표현이다(시각 + 값 하나).
//
// 집계는 DataPoint 의 Fields 맵 전체가 아니라 대상 필드 하나만 읽는다. 원본을
// 통째로 복사하는 대신 이 형태로 뽑아 두면 포인트마다 맵을 할당하지 않는다.
type scalarPoint struct {
	ts time.Time
	v  float64
}

// resultFieldName 은 집계 결과가 실릴 필드 이름을 정한다.
// 필드 미지정(첫 숫자 필드 자동 선택)일 때의 관례 이름은 "value" 다.
func resultFieldName(field string) string {
	if field == "" {
		return "value"
	}
	return field
}

// appendScalar 는 DataPoint 에서 field 값을 뽑아 dst 에 덧붙인다.
// 값이 없거나 숫자로 볼 수 없으면 dst 를 그대로 돌려준다.
//
// 필드 선택 규칙의 단일 출처다 — Series.QueryRangeScalar 와 extractScalars 가
// 모두 이 함수를 쓴다. 규칙이 두 곳에 복제되면 한쪽만 바뀌어 조용히 어긋난다.
func appendScalar(dst []scalarPoint, dp DataPoint, field string) []scalarPoint {
	if field == "" {
		// 필드가 지정되지 않으면 첫 번째 숫자 필드를 사용
		for _, v := range dp.Fields {
			if fv, ok := toFloat64(v); ok {
				return append(dst, scalarPoint{ts: dp.Timestamp, v: fv})
			}
		}
		return dst
	}
	v, ok := dp.Fields[field]
	if !ok {
		return dst
	}
	fv, ok := toFloat64(v)
	if !ok {
		return dst
	}
	return append(dst, scalarPoint{ts: dp.Timestamp, v: fv})
}

// extractScalars 는 DataPoint 목록에서 field 값을 뽑는다.
// DataPoint 를 받는 기존 함수들이 스칼라 구현에 위임할 때 쓴다.
func extractScalars(points []DataPoint, field string) []scalarPoint {
	if len(points) == 0 {
		return nil
	}
	out := make([]scalarPoint, 0, len(points))
	for _, p := range points {
		out = appendScalar(out, p, field)
	}
	return out
}

// aggregate 는 포인트 목록에 집계 함수를 적용한다.
func aggregate(points []DataPoint, field string, fn AggregateFunc) (float64, error) {
	return aggregateScalars(extractScalars(points, field), fn)
}

// aggregateScalars 는 이미 뽑아 둔 스칼라 목록에 집계 함수를 적용한다.
func aggregateScalars(pts []scalarPoint, fn AggregateFunc) (float64, error) {
	if len(pts) == 0 {
		return 0, ErrEmptyResult
	}

	values := make([]float64, len(pts))
	for i, p := range pts {
		values[i] = p.v
	}

	switch fn {
	case AggMin:
		result := values[0]
		for _, v := range values[1:] {
			if v < result {
				result = v
			}
		}
		return result, nil

	case AggMax:
		result := values[0]
		for _, v := range values[1:] {
			if v > result {
				result = v
			}
		}
		return result, nil

	case AggAvg:
		sum := 0.0
		for _, v := range values {
			sum += v
		}
		return sum / float64(len(values)), nil

	case AggSum:
		sum := 0.0
		for _, v := range values {
			sum += v
		}
		return sum, nil

	case AggCount:
		return float64(len(values)), nil

	case AggFirst:
		return values[0], nil

	case AggLast:
		return values[len(values)-1], nil

	default:
		return 0, ErrInvalidAggregation
	}
}

// downsample 는 포인트를 버킷 간격으로 다운샘플링한다.
func downsample(points []DataPoint, field string, fn AggregateFunc, bucketInterval time.Duration) []DataPoint {
	return downsampleScalars(extractScalars(points, field), field, fn, bucketInterval)
}

// downsampleScalars 는 이미 뽑아 둔 스칼라를 버킷 간격으로 다운샘플링한다.
// 빈 버킷은 결과에서 생략된다(균일 시간축이 필요하면 downsampleFilledScalars).
func downsampleScalars(pts []scalarPoint, field string, fn AggregateFunc, bucketInterval time.Duration) []DataPoint {
	if len(pts) == 0 || bucketInterval <= 0 {
		return nil
	}
	fieldName := resultFieldName(field)

	// 시작 시간을 버킷 간격에 맞춰 정렬
	bucketStart := pts[0].ts.Truncate(bucketInterval)

	var result []DataPoint
	var bucket []scalarPoint

	flush := func() {
		if len(bucket) == 0 {
			return
		}
		val, err := aggregateScalars(bucket, fn)
		if err != nil {
			return
		}
		result = append(result, DataPoint{
			Timestamp: bucketStart,
			Fields:    map[string]any{fieldName: val},
		})
	}

	for _, p := range pts {
		// 현재 포인트가 현재 버킷에 속하는지 확인
		if p.ts.Before(bucketStart.Add(bucketInterval)) {
			bucket = append(bucket, p)
			continue
		}
		flush()
		// 다음 버킷으로 이동 (빈 버킷 건너뜀)
		bucketStart = p.ts.Truncate(bucketInterval)
		bucket = []scalarPoint{p}
	}

	// 마지막 버킷 처리
	flush()

	return result
}

// downsampleFilled 는 [start,end] 전 구간에 대해 버킷을 생성하고, 값이 없는 빈 버킷을
// fill 전략으로 채운다. 빈 버킷이 결과에 포함되므로 시간축이 균일해진다.
//
//	previous: 직전 비어있지 않은 값으로 carry-forward (선두 빈 버킷은 null)
//	avg:      전/후 비어있지 않은 값의 평균 (한쪽만 있으면 그 값, 둘 다 없으면 null)
//	zero:     0
//	null:     null 값 (비우기 — 버킷은 존재, 값은 비어 있음)
func downsampleFilled(points []DataPoint, field string, fn AggregateFunc, interval time.Duration, start, end time.Time, fill FillStrategy) []DataPoint {
	return downsampleFilledScalars(
		extractScalars(points, field), field, fn, interval, start, end, fill, fillpolicy.Previous{})
}

// downsampleFilledScalars 는 downsampleFilled 의 스칼라 구현이다.
//
// prevLimit 는 `previous` 채우기의 사용 기간 제한이다. 제로값이면 종전대로
// 제한 없이 이어 쓴다.
func downsampleFilledScalars(pts []scalarPoint, field string, fn AggregateFunc, interval time.Duration, start, end time.Time, fill FillStrategy, prevLimit fillpolicy.Previous) []DataPoint {
	if interval <= 0 || !start.Before(end) {
		return nil
	}

	fieldName := resultFieldName(field)

	// 1) 포인트를 버킷(truncated timestamp)별로 모아 집계한다.
	buckets := make(map[int64][]scalarPoint)
	for _, p := range pts {
		k := p.ts.Truncate(interval).UnixNano()
		buckets[k] = append(buckets[k], p)
	}
	aggVal := make(map[int64]float64, len(buckets))
	for k, bkt := range buckets {
		if v, err := aggregateScalars(bkt, fn); err == nil {
			aggVal[k] = v
		}
	}

	// 2) [start,end] 의 버킷 시작 목록을 만든다.
	var starts []time.Time
	for b := start.Truncate(interval); b.Before(end); b = b.Add(interval) {
		starts = append(starts, b)
	}
	n := len(starts)
	if n == 0 {
		return nil
	}

	// 3) avg 용으로 각 위치의 "다음(이후) 비어있지 않은 값"을 미리 계산한다.
	nextVal := make([]*float64, n)
	var nv *float64
	for i := n - 1; i >= 0; i-- {
		if v, ok := aggVal[starts[i].UnixNano()]; ok {
			vv := v
			nv = &vv
		}
		nextVal[i] = nv
	}

	// 4) 버킷을 순회하며 값/채움을 적용한다.
	result := make([]DataPoint, 0, n)
	var prevVal *float64
	// 직전값을 연속 몇 번째로 채우는 중인지. 실측이 나오면 0 으로 되돌린다 —
	// 사용 기간은 "마지막 실측 이후" 를 세는 것이지 누적이 아니다.
	var prevRun int64
	intervalMs := interval.Milliseconds()
	for i, b := range starts {
		if v, ok := aggVal[b.UnixNano()]; ok {
			result = append(result, DataPoint{Timestamp: b, Fields: map[string]any{fieldName: v}})
			vv := v
			prevVal = &vv
			prevRun = 0
			continue
		}
		var filled any // nil = null
		switch fill {
		case FillNull:
			filled = nil
		case FillZero:
			filled = float64(0)
		case FillPrevious:
			if prevVal != nil {
				prevRun++
				if v, ok := prevLimit.FillAt(*prevVal, prevRun, intervalMs); ok {
					filled = v
				}
				// ok=false 면 filled 는 nil 그대로 — 기간을 넘겨 비운다.
			}
		case FillAvg:
			switch {
			case prevVal != nil && nextVal[i] != nil:
				filled = (*prevVal + *nextVal[i]) / 2
			case prevVal != nil:
				filled = *prevVal
			case nextVal[i] != nil:
				filled = *nextVal[i]
			}
		default:
			// 알 수 없는 전략은 null 로 안전 처리
			filled = nil
		}
		result = append(result, DataPoint{Timestamp: b, Fields: map[string]any{fieldName: filled}})
	}

	return result
}

// toFloat64 는 any 타입을 float64로 변환한다.
func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int64:
		return float64(val), true
	case int:
		return float64(val), true
	case float32:
		return float64(val), true
	case int32:
		return float64(val), true
	case uint64:
		return float64(val), true
	default:
		return math.NaN(), false
	}
}
