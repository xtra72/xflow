// device_history_test.go 는 DeviceHistory() 설정 접근자의 기본값/보정/override
// 동작을 검증한다.
package config

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newViperConfigForTest 는 기본값이 설정된 viper 기반 Config 를 생성한다(override 적용).
func newViperConfigForTest(t *testing.T, overrides map[string]any) Config {
	t.Helper()
	v := viper.New()
	SetDefaults(v)
	for k, val := range overrides {
		v.Set(k, val)
	}
	return &viperConfig{v: v, maxHistory: 100}
}

func TestDeviceHistory_Defaults(t *testing.T) {
	cfg := newViperConfigForTest(t, nil)

	dh := cfg.DeviceHistory()
	assert.True(t, dh.Enabled, "기본 활성")
	assert.Equal(t, 10*time.Second, dh.Interval, "기본 주기 10s")
	assert.Equal(t, 100, dh.MaxEntries, "기본 보관 100개")
}

func TestDeviceHistory_Overrides(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]any
		want      DeviceHistoryConfig
	}{
		{
			name: "명시 override 적용",
			overrides: map[string]any{
				"device_history.enabled":     false,
				"device_history.interval":    "30s",
				"device_history.max_entries": 200,
			},
			want: DeviceHistoryConfig{Enabled: false, Interval: 30 * time.Second, MaxEntries: 200},
		},
		{
			name: "interval 0/무효 → 기본 10s 보정",
			overrides: map[string]any{
				"device_history.interval": "0s",
			},
			want: DeviceHistoryConfig{Enabled: true, Interval: 10 * time.Second, MaxEntries: 100},
		},
		{
			name: "max_entries 0 → 기본 100 보정",
			overrides: map[string]any{
				"device_history.max_entries": 0,
			},
			want: DeviceHistoryConfig{Enabled: true, Interval: 10 * time.Second, MaxEntries: 100},
		},
		{
			name: "max_entries 음수 → 기본 100 보정",
			overrides: map[string]any{
				"device_history.max_entries": -5,
			},
			want: DeviceHistoryConfig{Enabled: true, Interval: 10 * time.Second, MaxEntries: 100},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newViperConfigForTest(t, tt.overrides)
			got := cfg.DeviceHistory()
			require.Equal(t, tt.want, got)
		})
	}
}
