package tsdb

import "errors"

// 센티널 에러 정의
var (
	// ErrSeriesNotFound 는 시리즈를 찾을 수 없을 때 반환된다.
	ErrSeriesNotFound = errors.New("tsdb: 시리즈를 찾을 수 없음")

	// ErrInvalidMeasurement 는 measurement 이름이 비어있을 때 반환된다.
	ErrInvalidMeasurement = errors.New("tsdb: measurement 이름이 비어있음")

	// ErrInvalidField 는 필드 맵이 비어있거나 nil일 때 반환된다.
	ErrInvalidField = errors.New("tsdb: 필드 맵이 비어있거나 nil")

	// ErrQueryTimeout 는 쿼리 타임아웃이 발생했을 때 반환된다.
	ErrQueryTimeout = errors.New("tsdb: 쿼리 타임아웃")

	// ErrMaxPointsExceeded 는 최대 반환 포인트 수를 초과했을 때 반환된다.
	ErrMaxPointsExceeded = errors.New("tsdb: 최대 반환 포인트 수 초과")

	// ErrTSDBClosed 는 TSDB가 이미 닫혔을 때 반환된다.
	ErrTSDBClosed = errors.New("tsdb: TSDB가 이미 닫힘")

	// ErrMaxSeriesExceeded 는 최대 시리즈 수를 초과했을 때 반환된다.
	ErrMaxSeriesExceeded = errors.New("tsdb: 최대 시리즈 수 초과")

	// ErrInvalidAggregation 는 알 수 없는 집계 함수일 때 반환된다.
	ErrInvalidAggregation = errors.New("tsdb: 알 수 없는 집계 함수")

	// ErrEmptyResult 는 집계 대상 데이터가 비어있을 때 반환된다.
	ErrEmptyResult = errors.New("tsdb: 집계 대상 데이터가 비어있음")
)
