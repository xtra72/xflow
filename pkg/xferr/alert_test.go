package xferr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// === AlertThresholdType 테스트 ===

// TestAlertThresholdType_Constants 는 AlertThresholdType 상수 값을 검증한다.
func TestAlertThresholdType_Constants(t *testing.T) {
	assert.Equal(t, AlertThresholdType("rate"), AlertThresholdRate)
	assert.Equal(t, AlertThresholdType("count"), AlertThresholdCount)
}

// === AlertThreshold 테스트 ===

// TestAlertThreshold_Construction 은 AlertThreshold 구조체 생성을 검증한다.
func TestAlertThreshold_Construction(t *testing.T) {
	threshold := AlertThreshold{
		Name:           "high-error-rate",
		ThresholdType:  AlertThresholdRate,
		Value:          0.95,
		WindowDuration: 5 * time.Minute,
		Scope:          "node-1",
	}

	assert.Equal(t, "high-error-rate", threshold.Name)
	assert.Equal(t, AlertThresholdRate, threshold.ThresholdType)
	assert.Equal(t, 0.95, threshold.Value)
	assert.Equal(t, 5*time.Minute, threshold.WindowDuration)
	assert.Equal(t, "node-1", threshold.Scope)
}

// TestAlertThreshold_ZeroValue 는 AlertThreshold의 제로 값을 검증한다.
func TestAlertThreshold_ZeroValue(t *testing.T) {
	var threshold AlertThreshold

	assert.Empty(t, threshold.Name, "기본 Name은 빈 문자열이어야 한다")
	assert.Empty(t, string(threshold.ThresholdType), "기본 ThresholdType은 빈 문자열이어야 한다")
	assert.Zero(t, threshold.Value, "기본 Value는 0이어야 한다")
	assert.Zero(t, threshold.WindowDuration, "기본 WindowDuration은 0이어야 한다")
	assert.Empty(t, threshold.Scope, "기본 Scope는 빈 문자열이어야 한다")
}

// === AlertCallback 테스트 ===

// TestAlertCallback_Invocation 은 AlertCallback 함수 호출을 검증한다.
func TestAlertCallback_Invocation(t *testing.T) {
	var calledThreshold AlertThreshold
	var calledValue float64

	callback := AlertCallback(func(threshold AlertThreshold, currentValue float64) {
		calledThreshold = threshold
		calledValue = currentValue
	})

	testThreshold := AlertThreshold{
		Name:          "test",
		ThresholdType: AlertThresholdCount,
		Value:         100,
	}

	callback(testThreshold, 150.0)

	assert.Equal(t, testThreshold, calledThreshold, "콜백에 전달된 임계값이 일치해야 한다")
	assert.Equal(t, 150.0, calledValue, "콜백에 전달된 현재 값이 일치해야 한다")
}

// === AlertConfig 테스트 ===

// TestAlertConfig_Construction 은 AlertConfig 구조체 생성을 검증한다.
func TestAlertConfig_Construction(t *testing.T) {
	callbackCalled := false
	callback := AlertCallback(func(threshold AlertThreshold, currentValue float64) {
		callbackCalled = true
	})

	config := AlertConfig{
		Thresholds: []AlertThreshold{
			{Name: "rate-threshold", ThresholdType: AlertThresholdRate, Value: 0.9, WindowDuration: time.Minute},
			{Name: "count-threshold", ThresholdType: AlertThresholdCount, Value: 100, WindowDuration: 5 * time.Minute},
		},
		Callback: callback,
		Enabled:  true,
	}

	assert.Len(t, config.Thresholds, 2, "임계값이 2개여야 한다")
	assert.True(t, config.Enabled, "활성화 상태여야 한다")
	assert.NotNil(t, config.Callback, "콜백이 nil이 아니어야 한다")

	// 콜백 호출 확인
	config.Callback(config.Thresholds[0], 0.95)
	assert.True(t, callbackCalled, "콜백이 호출되어야 한다")
}

// TestAlertConfig_ZeroValue 는 AlertConfig의 제로 값을 검증한다.
func TestAlertConfig_ZeroValue(t *testing.T) {
	var config AlertConfig

	assert.Nil(t, config.Thresholds, "기본 Thresholds는 nil이어야 한다")
	assert.Nil(t, config.Callback, "기본 Callback은 nil이어야 한다")
	assert.False(t, config.Enabled, "기본 Enabled는 false여야 한다")
}

// TestAlertConfig_Disabled 는 비활성화된 AlertConfig를 검증한다.
func TestAlertConfig_Disabled(t *testing.T) {
	config := AlertConfig{
		Thresholds: []AlertThreshold{
			{Name: "test", ThresholdType: AlertThresholdRate, Value: 0.5},
		},
		Enabled: false,
	}

	assert.False(t, config.Enabled, "비활성화 상태여야 한다")
	assert.Len(t, config.Thresholds, 1, "임계값은 설정되어 있어야 한다")
}
