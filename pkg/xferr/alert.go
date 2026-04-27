package xferr

import "time"

// AlertThresholdType 는 경고 임계값의 유형을 나타내는 문자열 타입이다.
type AlertThresholdType string

const (
	// AlertThresholdRate 는 비율 기반 임계값을 나타낸다.
	AlertThresholdRate AlertThresholdType = "rate"
	// AlertThresholdCount 는 카운트 기반 임계값을 나타낸다.
	AlertThresholdCount AlertThresholdType = "count"
)

// AlertThreshold 는 경고 발생 임계값 설정을 나타내는 구조체이다.
type AlertThreshold struct {
	// Name 은 임계값의 이름이다.
	Name string
	// ThresholdType 는 임계값의 유형(rate 또는 count)이다.
	ThresholdType AlertThresholdType
	// Value 는 임계값이다.
	Value float64
	// WindowDuration 은 임계값 측정 윈도우 기간이다.
	WindowDuration time.Duration
	// Scope 는 임계값이 적용되는 범위이다.
	Scope string
}

// AlertCallback 은 경고 임계값이 초과되었을 때 호출되는 콜백 함수 타입이다.
type AlertCallback func(threshold AlertThreshold, currentValue float64)

// AlertConfig 는 경고 설정을 나타내는 구조체이다.
type AlertConfig struct {
	// Thresholds 는 경고 임계값 목록이다.
	Thresholds []AlertThreshold
	// Callback 은 임계값 초과 시 호출되는 콜백 함수이다.
	Callback AlertCallback
	// Enabled 는 경고 기능의 활성화 여부이다.
	Enabled bool
}
