package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseResolution 는 "WIDTHxHEIGHT" 문자열 파싱을 검증한다
// (@SPEC:SPEC-REMOTE-001 M11, REQ-M01). 유효 → (w,h), 무효/빈값 → (0,0).
func TestParseResolution(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		wantW int
		wantH int
	}{
		{"표준 해상도", "1920x1080", 1920, 1080},
		{"소문자 x", "1280x720", 1280, 720},
		{"대문자 X", "1024X768", 1024, 768},
		{"공백 허용", " 800 x 600 ", 800, 600},
		{"빈 문자열", "", 0, 0},
		{"형식 오류 — 구분자 없음", "1920", 0, 0},
		{"형식 오류 — 비정수", "abcxdef", 0, 0},
		{"형식 오류 — 음수", "-1920x1080", 0, 0},
		{"형식 오류 — 0 폭", "0x1080", 0, 0},
		{"형식 오류 — 0 높이", "1920x0", 0, 0},
		{"형식 오류 — 부동소수", "1920.5x1080", 0, 0},
		{"형식 오류 — 추가 토큰", "1920x1080x1", 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, h := ParseResolution(tc.in)
			assert.Equal(t, tc.wantW, w, "width")
			assert.Equal(t, tc.wantH, h, "height")
		})
	}
}

// TestRemoteManagement_DisplayDefaults 는 display 미설정 시 0(미보고)으로
// 처리되는지 검증한다(REQ-M03 하위 호환).
func TestRemoteManagement_DisplayDefaults(t *testing.T) {
	cfg, err := Load(WithConfigPaths(t.TempDir()))
	require.NoError(t, err)

	rm := cfg.RemoteManagement()
	assert.Equal(t, 0, rm.Display.Width, "기본 display width 0(미보고)")
	assert.Equal(t, 0, rm.Display.Height, "기본 display height 0(미보고)")
}

// TestRemoteManagement_DisplayResolutionString 는 display.resolution "WxH"
// 문자열이 width/height 로 파싱되는지 검증한다(REQ-M01).
func TestRemoteManagement_DisplayResolutionString(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(WithConfigPaths(dir), WithDefaults(func(v *viper.Viper) {
		v.SetDefault("remote_management.display.resolution", "1920x1080")
	}))
	require.NoError(t, err)

	rm := cfg.RemoteManagement()
	assert.Equal(t, 1920, rm.Display.Width)
	assert.Equal(t, 1080, rm.Display.Height)
}

// TestRemoteManagement_DisplayWidthHeight 는 resolution 미설정 시 명시적
// width/height 가 사용되는지 검증한다(REQ-M01).
func TestRemoteManagement_DisplayWidthHeight(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(WithConfigPaths(dir), WithDefaults(func(v *viper.Viper) {
		v.SetDefault("remote_management.display.width", 1280)
		v.SetDefault("remote_management.display.height", 720)
	}))
	require.NoError(t, err)

	rm := cfg.RemoteManagement()
	assert.Equal(t, 1280, rm.Display.Width)
	assert.Equal(t, 720, rm.Display.Height)
}

// TestRemoteManagement_DisplayResolutionPrecedence 는 resolution 이 유효하면
// width/height 보다 우선하는지 검증한다(REQ-M01 — resolution SoT).
func TestRemoteManagement_DisplayResolutionPrecedence(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(WithConfigPaths(dir), WithDefaults(func(v *viper.Viper) {
		v.SetDefault("remote_management.display.resolution", "2560x1440")
		v.SetDefault("remote_management.display.width", 1280)
		v.SetDefault("remote_management.display.height", 720)
	}))
	require.NoError(t, err)

	rm := cfg.RemoteManagement()
	assert.Equal(t, 2560, rm.Display.Width, "유효 resolution 이 width 보다 우선")
	assert.Equal(t, 1440, rm.Display.Height, "유효 resolution 이 height 보다 우선")
}

// TestRemoteManagement_DisplayInvalidResolutionFallsBackToWidthHeight 는 잘못된
// resolution 형식이면 width/height 로 폴백하는지 검증한다(REQ-M01/M03 안전 처리).
func TestRemoteManagement_DisplayInvalidResolutionFallsBackToWidthHeight(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(WithConfigPaths(dir), WithDefaults(func(v *viper.Viper) {
		v.SetDefault("remote_management.display.resolution", "bogus")
		v.SetDefault("remote_management.display.width", 1366)
		v.SetDefault("remote_management.display.height", 768)
	}))
	require.NoError(t, err)

	rm := cfg.RemoteManagement()
	assert.Equal(t, 1366, rm.Display.Width, "무효 resolution → width 폴백")
	assert.Equal(t, 768, rm.Display.Height, "무효 resolution → height 폴백")
}

// TestRemoteManagement_DisplayInvalidWidthHeightZeroed 는 음수/0 width/height 가
// 0(미보고)으로 안전 처리되는지 검증한다(REQ-M03).
func TestRemoteManagement_DisplayInvalidWidthHeightZeroed(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(WithConfigPaths(dir), WithDefaults(func(v *viper.Viper) {
		v.SetDefault("remote_management.display.width", -5)
		v.SetDefault("remote_management.display.height", 0)
	}))
	require.NoError(t, err)

	rm := cfg.RemoteManagement()
	assert.Equal(t, 0, rm.Display.Width, "음수 width → 0(미보고)")
	assert.Equal(t, 0, rm.Display.Height, "0 height → 0(미보고)")
}
