package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestObserve_IDStyle_Default - observe.id_style 미지정 시 기본값 "both" 반환.
func TestObserve_IDStyle_Default(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "both", cfg.Observe().IDStyle)
}

// TestObserve_IDStyle_FromFile - YAML 파일의 observe.id_style 이 반영되는지 검증.
func TestObserve_IDStyle_FromFile(t *testing.T) {
	tests := []struct {
		name  string
		style string
	}{
		{"name 모드", "name"},
		{"id 모드", "id"},
		{"both 모드", "both"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := "observe:\n  id_style: \"" + tt.style + "\"\n"
			path := writeTestYAML(t, yaml)
			cfg, err := Load(WithConfigFile(path))
			require.NoError(t, err)
			assert.Equal(t, tt.style, cfg.Observe().IDStyle)
		})
	}
}

// TestObserve_IDStyle_EmptyFallsBackToBoth - 빈 문자열은 "both" 로 fallback.
func TestObserve_IDStyle_EmptyFallsBackToBoth(t *testing.T) {
	yaml := "observe:\n  id_style: \"\"\n"
	path := writeTestYAML(t, yaml)
	cfg, err := Load(WithConfigFile(path))
	require.NoError(t, err)
	assert.Equal(t, "both", cfg.Observe().IDStyle)
}

// TestValidate_IDStyle - 유효/무효 id_style 값에 대한 로드 시점 검증.
func TestValidate_IDStyle(t *testing.T) {
	valid := []string{"name", "id", "both", ""}
	for _, s := range valid {
		v := newValidViper(t)
		v.Set("observe.id_style", s)
		assert.NoError(t, Validate(v), "id_style=%q 는 유효해야 합니다", s)
	}

	invalid := []string{"NAME", "uuid", "garbage", "names"}
	for _, s := range invalid {
		v := newValidViper(t)
		v.Set("observe.id_style", s)
		err := Validate(v)
		require.Error(t, err, "id_style=%q 는 에러를 반환해야 합니다", s)
		assert.True(t, errors.Is(err, ErrInvalidIDStyle),
			"에러는 ErrInvalidIDStyle 을 포함해야 합니다, 실제: %v", err)
	}
}

// TestSet_IDStyle_Runtime - observe.id_style 이 런타임에 변경 가능(mutable)하고
// 유효성 검증이 적용되는지 검증.
func TestSet_IDStyle_Runtime(t *testing.T) {
	assert.True(t, IsMutable("observe.id_style"),
		"observe.id_style 는 런타임 변경 가능해야 합니다")

	cfg, err := Load()
	require.NoError(t, err)

	// 유효 값 변경 성공.
	require.NoError(t, cfg.Set("observe.id_style", "name"))
	assert.Equal(t, "name", cfg.Observe().IDStyle)

	// 무효 값 변경 실패 (기존 값 유지).
	err = cfg.Set("observe.id_style", "garbage")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidIDStyle))
	assert.Equal(t, "name", cfg.Observe().IDStyle, "무효 값은 반영되지 않아야 합니다")
}
