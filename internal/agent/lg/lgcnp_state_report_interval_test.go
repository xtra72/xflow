package lg

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseLGCNPConfig_StateReportInterval_VariousInputs 는 v0.18.20 의
// state_report_interval 입력 형식 관용 처리를 검증한다 (Web UI 호환).
//
// 허용 입력:
//   - string "30s" / "1m" (Go duration)
//   - string "30" (단위 없는 숫자 → 초)
//   - number 30 (JSON number → 초)
//   - 빈 문자열 / 누락 → 0 (비활성)
func TestParseLGCNPConfig_StateReportInterval_VariousInputs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  any
		want time.Duration
	}{
		{"duration string 30s", "30s", 30 * time.Second},
		{"duration string 1m", "1m", time.Minute},
		{"duration string 5m", "5m", 5 * time.Minute},
		{"numeric string 30", "30", 30 * time.Second},
		{"numeric string 60", "60", time.Minute},
		{"int 30", 30, 30 * time.Second},
		{"int64 60", int64(60), time.Minute},
		{"float64 1.5", 1.5, 1500 * time.Millisecond},
		{"empty string", "", 0},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := parseLGCNPConfig(map[string]any{
				"serial_port":           "/dev/ttyTEST",
				"state_report_interval": tc.raw,
			})
			require.NoError(t, err, "raw=%v 파싱 성공해야 함", tc.raw)
			assert.Equal(t, tc.want, cfg.StateReportInterval,
				"raw=%v → expected %v, got %v", tc.raw, tc.want, cfg.StateReportInterval)
		})
	}
}

// TestParseLGCNPConfig_StateReportInterval_Missing 는 옵션 누락 시 기본 0.
func TestParseLGCNPConfig_StateReportInterval_Missing(t *testing.T) {
	t.Parallel()

	cfg, err := parseLGCNPConfig(map[string]any{
		"serial_port": "/dev/ttyTEST",
	})
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), cfg.StateReportInterval, "옵션 누락: 기본 0 (비활성)")
}

// TestParseLGCNPConfig_StateReportInterval_Invalid 는 파싱 불가능한 문자열
// 시 에러 반환.
func TestParseLGCNPConfig_StateReportInterval_Invalid(t *testing.T) {
	t.Parallel()

	_, err := parseLGCNPConfig(map[string]any{
		"serial_port":           "/dev/ttyTEST",
		"state_report_interval": "abc",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "state_report_interval")
}
