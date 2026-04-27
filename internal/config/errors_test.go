package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestErrVariables_Distinct - 11개의 센티널 에러가 모두 고유한지 검증
func TestErrVariables_Distinct(t *testing.T) {
	sentinels := []error{
		ErrInvalidPort,
		ErrFileNotFound,
		ErrRequiredField,
		ErrInvalidStorageType,
		ErrInvalidLogLevel,
		ErrInvalidPositiveValue,
		ErrInvalidDuration,
		ErrImmutableKey,
		ErrConfigNotLoaded,
		ErrInvalidLogFormat,
		ErrInvalidLogOutput,
	}

	for i := 0; i < len(sentinels); i++ {
		for j := i + 1; j < len(sentinels); j++ {
			assert.NotEqual(t, sentinels[i].Error(), sentinels[j].Error(),
				"에러 %d와 %d가 동일합니다", i, j)
		}
	}
}

// TestErrVariables_ErrorsIs - 각 에러가 errors.Is()와 호환되는지 검증
func TestErrVariables_ErrorsIs(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"ErrInvalidPort", ErrInvalidPort},
		{"ErrFileNotFound", ErrFileNotFound},
		{"ErrRequiredField", ErrRequiredField},
		{"ErrInvalidStorageType", ErrInvalidStorageType},
		{"ErrInvalidLogLevel", ErrInvalidLogLevel},
		{"ErrInvalidPositiveValue", ErrInvalidPositiveValue},
		{"ErrInvalidDuration", ErrInvalidDuration},
		{"ErrImmutableKey", ErrImmutableKey},
		{"ErrConfigNotLoaded", ErrConfigNotLoaded},
		{"ErrInvalidLogFormat", ErrInvalidLogFormat},
		{"ErrInvalidLogOutput", ErrInvalidLogOutput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, errors.Is(tt.err, tt.err),
				"%s가 errors.Is()와 호환되지 않습니다", tt.name)
		})
	}
}

// TestValidationErrors_Empty - 빈 ValidationErrors의 HasErrors() 동작 검증
func TestValidationErrors_Empty(t *testing.T) {
	ve := &ValidationErrors{}
	assert.False(t, ve.HasErrors(), "빈 ValidationErrors의 HasErrors()는 false여야 합니다")
	assert.Empty(t, ve.Errors(), "빈 ValidationErrors의 Errors()는 비어있어야 합니다")
}

// TestValidationErrors_Add - 에러 추가 후 HasErrors(), Errors() 동작 검증
func TestValidationErrors_Add(t *testing.T) {
	ve := &ValidationErrors{}

	err1 := errors.New("에러 1")
	err2 := errors.New("에러 2")

	ve.Add(err1)
	require.True(t, ve.HasErrors(), "에러 추가 후 HasErrors()는 true여야 합니다")
	assert.Len(t, ve.Errors(), 1, "에러 1개 추가 후 Errors() 길이는 1이어야 합니다")

	ve.Add(err2)
	assert.Len(t, ve.Errors(), 2, "에러 2개 추가 후 Errors() 길이는 2여야 합니다")
	assert.Contains(t, ve.Errors(), err1, "Errors()에 첫 번째 에러가 포함되어야 합니다")
	assert.Contains(t, ve.Errors(), err2, "Errors()에 두 번째 에러가 포함되어야 합니다")
}

// TestValidationErrors_Error - Error() 메서드의 줄바꿈 포맷 검증
func TestValidationErrors_Error(t *testing.T) {
	ve := &ValidationErrors{}
	ve.Add(errors.New("첫 번째 에러"))
	ve.Add(errors.New("두 번째 에러"))
	ve.Add(errors.New("세 번째 에러"))

	result := ve.Error()
	assert.Contains(t, result, "첫 번째 에러")
	assert.Contains(t, result, "두 번째 에러")
	assert.Contains(t, result, "세 번째 에러")
	assert.Equal(t, "첫 번째 에러\n두 번째 에러\n세 번째 에러", result,
		"Error()는 줄바꿈으로 에러를 연결해야 합니다")
}

// TestValidationErrors_ErrorInterface - error 인터페이스 구현 검증
func TestValidationErrors_ErrorInterface(t *testing.T) {
	var err error = &ValidationErrors{}
	assert.NotNil(t, err, "ValidationErrors는 error 인터페이스를 구현해야 합니다")
}
