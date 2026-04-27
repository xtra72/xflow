package tsdb

import "sync/atomic"

// globalMemoryUsage 는 전역 메모리 사용량을 추적하는 원자적 카운터이다.
var globalMemoryUsage atomic.Int64

// AddMemory 는 전역 메모리 사용량에 바이트를 추가한다.
func AddMemory(bytes int64) {
	globalMemoryUsage.Add(bytes)
}

// SubMemory 는 전역 메모리 사용량에서 바이트를 차감한다.
func SubMemory(bytes int64) {
	globalMemoryUsage.Add(-bytes)
}

// CurrentMemoryUsage 는 현재 전역 메모리 사용량을 반환한다.
func CurrentMemoryUsage() int64 {
	return globalMemoryUsage.Load()
}

// ResetMemoryUsage 는 전역 메모리 사용량을 0으로 초기화한다. (테스트용)
func ResetMemoryUsage() {
	globalMemoryUsage.Store(0)
}

// EstimatePointSize 는 DataPoint의 추정 메모리 크기를 계산한다.
func EstimatePointSize(dp DataPoint) int64 {
	return dp.EstimateSize()
}
