// dashboard_budget_test.go 는 캔버스 요소 상한 설정과 거기서 유도되는 페이로드 예산을
// 검증한다 (@SPEC:SPEC-CANVAS-007 §결정 14).
//
// 이 시험이 지키려는 성질 넷:
//  1. 기본값은 오늘 값 그대로다 — 설정을 쓰지 않는 설치는 달라지지 않는다.
//  2. 설정 파일의 값이 실제로 읽힌다(기본값 위에 얹힌다).
//  3. **바닥** — 작게 설정해도 오늘 동작하던 256KB 아래로 내려가지 않는다.
//  4. **천장** — 오타 하나로 서버가 무제한 본문을 받게 되지 않는다.
package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDashboard_Defaults 는 설정이 없을 때 오늘 값이 그대로 나오는지 본다.
//
// 1024 는 프론트엔드가 컴파일 상수로 들고 있던 그 수이고, 256KB 는 핸들러가 상수로
// 들고 있던 그 수다. 둘 다 달라지지 않아야 이 변경이 **기존 설치에 무해**하다.
func TestDashboard_Defaults(t *testing.T) {
	cfg := newViperConfigForTest(t, nil)

	dash := cfg.Dashboard()
	assert.Equal(t, 1024, dash.MaxCanvasElements, "기본 요소 상한은 오늘의 1024")
	assert.EqualValues(t, 1024*1024, dash.PayloadBudgetBytes, "1024 × 1KiB = 1MiB")
	assert.GreaterOrEqual(t, dash.PayloadBudgetBytes, MinDashboardPayloadBytes,
		"기본 예산이 바닥 아래로 내려가면 기존 대시보드 저장이 깨진다")
}

// TestDashboard_ConfigFileOverride 는 **설정 파일**의 값이 읽히는지 본다.
//
// viper.Set 이 아니라 YAML 을 실제로 지나게 하는 것이 요점이다 — 사용자가 손으로 쓰는
// 것은 파일이고, 키 이름이 틀리면 Set 시험은 통과하되 파일은 무시된다.
func TestDashboard_ConfigFileOverride(t *testing.T) {
	path := writeTestYAML(t, `
dashboard:
  max_canvas_elements: 2048
`)
	cfg, err := Load(WithConfigFile(path))
	require.NoError(t, err)

	dash := cfg.Dashboard()
	assert.Equal(t, 2048, dash.MaxCanvasElements, "파일의 값이 기본값을 덮는다")
	assert.EqualValues(t, 2048*1024, dash.PayloadBudgetBytes, "예산이 그 수를 따라간다")
}

// TestDeriveDashboardPayloadBytes 는 유도 함수의 구간 전체를 못박는다.
//
// **바닥과 천장을 같은 표에서 재는 이유**: 둘은 한 함수의 두 끝이고, 한쪽만 고치면
// 나머지가 조용히 어긋난다.
func TestDeriveDashboardPayloadBytes(t *testing.T) {
	tests := []struct {
		name     string
		elements int
		want     int64
	}{
		// --- 바닥 ---
		{"0(미설정) → 기본값의 예산", 0, 1024 * 1024},
		{"음수(오타) → 기본값의 예산", -1, 1024 * 1024},
		{"1 → 바닥", 1, MinDashboardPayloadBytes},
		{"255 → 바닥(아직 256KB 에 못 미친다)", 255, MinDashboardPayloadBytes},
		{"256 → 바닥과 정확히 같다(256 × 1KiB = 256KB)", 256, MinDashboardPayloadBytes},

		// --- 중간 ---
		{"257 → 바닥을 벗어나 비례하기 시작한다", 257, 257 * 1024},
		{"기본 1024", 1024, 1024 * 1024},
		{"2048", 2048, 2048 * 1024},

		// --- 천장 ---
		{"8192 → 천장과 정확히 같다", 8192, MaxDashboardPayloadBytes},
		{"8193 → 천장에서 잘린다", 8193, MaxDashboardPayloadBytes},
		{"오타 1억 → 천장에서 잘린다", 100_000_000, MaxDashboardPayloadBytes},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DeriveDashboardPayloadBytes(tt.elements))
		})
	}
}

// TestDeriveDashboardPayloadBytes_AlwaysBounded 는 **어떤 정수가 들어와도** 결과가
// 구간 안임을 본다. DoS 방어물이라는 성질이 이 변경을 넘어 살아남는지를 재는 자리다.
func TestDeriveDashboardPayloadBytes_AlwaysBounded(t *testing.T) {
	inputs := []int{
		-1 << 31, -99999, -1, 0, 1, 7, 1024, 65535,
		1 << 20, 1 << 28, 1<<31 - 1,
	}
	for _, n := range inputs {
		got := DeriveDashboardPayloadBytes(n)
		assert.GreaterOrEqual(t, got, MinDashboardPayloadBytes, "입력 %d 에서 바닥을 뚫었다", n)
		assert.LessOrEqual(t, got, MaxDashboardPayloadBytes, "입력 %d 에서 천장을 뚫었다", n)
	}
}

// TestClampMaxCanvasElements 는 요소 수 자체의 죄기를 본다.
//
// 예산 천장과 요소 천장이 **같은 자리에서 만나는지**가 이 시험의 요점이다 — 어긋나면
// "확보한 자리를 쓸 수 없다" 또는 "받아 놓고 저장은 실패한다" 가 된다.
func TestClampMaxCanvasElements(t *testing.T) {
	assert.Equal(t, DefaultMaxCanvasElements, ClampMaxCanvasElements(0), "0 → 기본값")
	assert.Equal(t, DefaultMaxCanvasElements, ClampMaxCanvasElements(-5), "음수 → 기본값")
	assert.Equal(t, 1, ClampMaxCanvasElements(1))
	assert.Equal(t, 4096, ClampMaxCanvasElements(4096))
	assert.EqualValues(t, MaxCanvasElementsCeiling, ClampMaxCanvasElements(1<<30), "천장에서 잘린다")

	// 두 천장이 만나는 자리.
	assert.Equal(t, MaxDashboardPayloadBytes,
		DeriveDashboardPayloadBytes(int(MaxCanvasElementsCeiling)),
		"요소 천장에서 예산이 정확히 천장에 닿아야 한다")
}

// TestDashboard_ClampedViaConfig 는 죄기가 **설정 경로**에서도 걸리는지 본다.
// 순수 함수만 시험하면 접근자가 죄기를 부르지 않는 회귀를 놓친다.
func TestDashboard_ClampedViaConfig(t *testing.T) {
	t.Run("천장 초과", func(t *testing.T) {
		cfg := newViperConfigForTest(t, map[string]any{
			"dashboard.max_canvas_elements": 999_999,
		})
		dash := cfg.Dashboard()
		assert.EqualValues(t, MaxCanvasElementsCeiling, dash.MaxCanvasElements)
		assert.Equal(t, MaxDashboardPayloadBytes, dash.PayloadBudgetBytes)
	})

	t.Run("작게 설정해도 예산은 바닥을 지킨다", func(t *testing.T) {
		cfg := newViperConfigForTest(t, map[string]any{
			"dashboard.max_canvas_elements": 16,
		})
		dash := cfg.Dashboard()
		assert.Equal(t, 16, dash.MaxCanvasElements, "요소 수는 그대로 죈 값")
		assert.Equal(t, MinDashboardPayloadBytes, dash.PayloadBudgetBytes,
			"요소를 죄었다고 기존 대시보드 저장이 깨져서는 안 된다")
	})
}
