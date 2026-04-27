package tsdb

import "time"

// DataPoint 는 시계열 데이터의 단일 측정값이다.
type DataPoint struct {
	Timestamp time.Time      // 나노초 정밀도
	Fields    map[string]any // float64, int64, string, bool 지원
}

// EstimateSize 는 DataPoint의 추정 메모리 크기(바이트)를 반환한다.
// timestamp(24) + map overhead(8) + per-field (key string len + value size)
func (dp DataPoint) EstimateSize() int64 {
	// time.Time 구조체 크기 (wall + ext + loc 포인터)
	var size int64 = 24
	// 맵 오버헤드
	size += 8

	for k, v := range dp.Fields {
		// 키 문자열 길이 (string header 16 + 실제 데이터)
		size += 16 + int64(len(k))
		// 값 크기 추정
		size += estimateFieldValueSize(v)
	}

	return size
}

// estimateFieldValueSize 는 필드 값의 추정 크기를 반환한다.
func estimateFieldValueSize(v any) int64 {
	switch val := v.(type) {
	case float64:
		return 8
	case int64:
		return 8
	case bool:
		return 1
	case string:
		return int64(len(val))
	default:
		// 알 수 없는 타입은 8바이트로 추정
		return 8
	}
}

// Validate 는 DataPoint의 유효성을 검증한다.
// Fields가 nil이거나 비어있으면 ErrInvalidField를 반환한다.
func (dp DataPoint) Validate() error {
	if dp.Fields == nil || len(dp.Fields) == 0 {
		return ErrInvalidField
	}
	return nil
}
