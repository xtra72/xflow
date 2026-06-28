package tsdb

import (
	"context"
	"math"
	"strings"
	"time"
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
	Limit          int               // 최대 반환 포인트 수 (0이면 config 기본값)
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

		// 범위 쿼리로 원본 포인트 가져오기
		rawPoints := s.QueryRange(q.Start, q.End)
		scannedPoints := len(rawPoints)

		if scannedPoints == 0 {
			continue
		}

		var resultPoints []DataPoint

		if q.Aggregation == "" {
			// raw 반환
			resultPoints = rawPoints
		} else if q.BucketInterval > 0 {
			// 다운샘플링. Fill 이 설정되면 [Start,End] 전 구간 버킷을 생성하고 빈 버킷을 채운다.
			if q.Fill == FillNone {
				resultPoints = downsample(rawPoints, q.Field, q.Aggregation, q.BucketInterval)
			} else {
				resultPoints = downsampleFilled(rawPoints, q.Field, q.Aggregation, q.BucketInterval, q.Start, q.End, q.Fill)
			}
		} else {
			// 전체 범위 집계
			val, err := aggregate(rawPoints, q.Field, q.Aggregation)
			if err != nil {
				continue
			}
			fieldName := q.Field
			if fieldName == "" {
				fieldName = "value"
			}
			resultPoints = []DataPoint{
				{
					Timestamp: rawPoints[0].Timestamp,
					Fields:    map[string]any{fieldName: val},
				},
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

// aggregate 는 포인트 목록에 집계 함수를 적용한다.
func aggregate(points []DataPoint, field string, fn AggregateFunc) (float64, error) {
	if len(points) == 0 {
		return 0, ErrEmptyResult
	}

	// 필드 값 추출
	values := make([]float64, 0, len(points))
	for _, p := range points {
		if field == "" {
			// 필드가 지정되지 않으면 첫 번째 숫자 필드를 사용
			for _, v := range p.Fields {
				if fv, ok := toFloat64(v); ok {
					values = append(values, fv)
					break
				}
			}
		} else {
			v, ok := p.Fields[field]
			if !ok {
				continue
			}
			fv, ok := toFloat64(v)
			if !ok {
				continue
			}
			values = append(values, fv)
		}
	}

	if len(values) == 0 {
		return 0, ErrEmptyResult
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
	if len(points) == 0 || bucketInterval <= 0 {
		return nil
	}

	// 시작 시간을 버킷 간격에 맞춰 정렬
	bucketStart := points[0].Timestamp.Truncate(bucketInterval)

	var result []DataPoint
	var bucket []DataPoint

	for _, p := range points {
		// 현재 포인트가 현재 버킷에 속하는지 확인
		if p.Timestamp.Before(bucketStart.Add(bucketInterval)) {
			bucket = append(bucket, p)
		} else {
			// 현재 버킷 집계
			if len(bucket) > 0 {
				if dp := aggregateBucket(bucket, field, fn, bucketStart); dp != nil {
					result = append(result, *dp)
				}
			}

			// 다음 버킷으로 이동 (빈 버킷 건너뜀)
			bucketStart = p.Timestamp.Truncate(bucketInterval)
			bucket = []DataPoint{p}
		}
	}

	// 마지막 버킷 처리
	if len(bucket) > 0 {
		if dp := aggregateBucket(bucket, field, fn, bucketStart); dp != nil {
			result = append(result, *dp)
		}
	}

	return result
}

// aggregateBucket 는 단일 버킷의 포인트를 집계하여 DataPoint를 반환한다.
func aggregateBucket(bucket []DataPoint, field string, fn AggregateFunc, bucketStart time.Time) *DataPoint {
	val, err := aggregate(bucket, field, fn)
	if err != nil {
		return nil
	}

	fieldName := field
	if fieldName == "" {
		fieldName = "value"
	}

	return &DataPoint{
		Timestamp: bucketStart,
		Fields:    map[string]any{fieldName: val},
	}
}

// downsampleFilled 는 [start,end] 전 구간에 대해 버킷을 생성하고, 값이 없는 빈 버킷을
// fill 전략으로 채운다. 빈 버킷이 결과에 포함되므로 시간축이 균일해진다.
//
//	previous: 직전 비어있지 않은 값으로 carry-forward (선두 빈 버킷은 null)
//	avg:      전/후 비어있지 않은 값의 평균 (한쪽만 있으면 그 값, 둘 다 없으면 null)
//	zero:     0
//	null:     null 값 (비우기 — 버킷은 존재, 값은 비어 있음)
func downsampleFilled(points []DataPoint, field string, fn AggregateFunc, interval time.Duration, start, end time.Time, fill FillStrategy) []DataPoint {
	if interval <= 0 || !start.Before(end) {
		return nil
	}

	fieldName := field
	if fieldName == "" {
		fieldName = "value"
	}

	// 1) 포인트를 버킷(truncated timestamp)별로 모아 집계한다.
	buckets := make(map[int64][]DataPoint)
	for _, p := range points {
		k := p.Timestamp.Truncate(interval).UnixNano()
		buckets[k] = append(buckets[k], p)
	}
	aggVal := make(map[int64]float64, len(buckets))
	for k, bkt := range buckets {
		if v, err := aggregate(bkt, field, fn); err == nil {
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
	for i, b := range starts {
		if v, ok := aggVal[b.UnixNano()]; ok {
			result = append(result, DataPoint{Timestamp: b, Fields: map[string]any{fieldName: v}})
			vv := v
			prevVal = &vv
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
				filled = *prevVal
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
