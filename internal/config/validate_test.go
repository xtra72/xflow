package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newValidViper - 기본값이 설정된 유효한 Viper 인스턴스 생성 헬퍼
func newValidViper(t *testing.T) *viper.Viper {
	t.Helper()
	v := viper.New()
	SetDefaults(v)
	return v
}

// TestValidate_ValidDefaults - 기본값은 항상 유효해야 합니다 (AC-005)
func TestValidate_ValidDefaults(t *testing.T) {
	v := newValidViper(t)
	err := Validate(v)
	assert.NoError(t, err, "기본값은 유효성 검증을 통과해야 합니다")
}

// TestValidate_InvalidPort - 유효하지 않은 포트 검증 (AC-006)
func TestValidate_InvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"포트 0", 0},
		{"포트 70000", 70000},
		{"음수 포트", -1},
		{"포트 65536", 65536},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newValidViper(t)
			v.Set("server.port", tt.port)

			err := Validate(v)
			require.Error(t, err, "포트 %d는 에러를 반환해야 합니다", tt.port)
			assert.True(t, errors.Is(err, ErrInvalidPort),
				"에러는 ErrInvalidPort를 포함해야 합니다, 실제: %v", err)
		})
	}
}

// TestValidate_ValidPort - 유효한 포트 검증
func TestValidate_ValidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"최소 포트", 1},
		{"기본 포트", 8080},
		{"최대 포트", 65535},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newValidViper(t)
			v.Set("server.port", tt.port)

			err := Validate(v)
			assert.NoError(t, err, "포트 %d는 유효해야 합니다", tt.port)
		})
	}
}

// TestValidate_InvalidStorageType - 유효하지 않은 스토리지 타입 검증
func TestValidate_InvalidStorageType(t *testing.T) {
	v := newValidViper(t)
	v.Set("storage.type", "mongodb")

	err := Validate(v)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidStorageType),
		"에러는 ErrInvalidStorageType을 포함해야 합니다, 실제: %v", err)
}

// TestValidate_InvalidLogLevel - 유효하지 않은 로그 레벨 검증
func TestValidate_InvalidLogLevel(t *testing.T) {
	v := newValidViper(t)
	v.Set("observe.default_level", "verbose")

	err := Validate(v)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidLogLevel),
		"에러는 ErrInvalidLogLevel을 포함해야 합니다, 실제: %v", err)
}

// TestValidate_MultipleErrors - 여러 유효성 검증 에러 집계 (AC-007)
func TestValidate_MultipleErrors(t *testing.T) {
	v := newValidViper(t)
	v.Set("server.port", 0)
	v.Set("storage.type", "mongodb")
	v.Set("observe.default_level", "verbose")

	err := Validate(v)
	require.Error(t, err)

	var ve *ValidationErrors
	require.True(t, errors.As(err, &ve), "에러는 *ValidationErrors 타입이어야 합니다")
	assert.GreaterOrEqual(t, len(ve.Errors()), 3,
		"최소 3개의 유효성 검증 에러가 있어야 합니다, 실제: %d", len(ve.Errors()))
}

// TestValidate_TLS_CertNotFound - TLS 활성화 시 인증서 파일 없음 검증 (AC-008)
func TestValidate_TLS_CertNotFound(t *testing.T) {
	v := newValidViper(t)
	v.Set("server.tls.enabled", true)
	v.Set("server.tls.cert_file", "/nonexistent/cert.pem")
	v.Set("server.tls.key_file", "/nonexistent/key.pem")

	err := Validate(v)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrFileNotFound),
		"에러는 ErrFileNotFound를 포함해야 합니다, 실제: %v", err)
}

// TestValidate_TLS_Disabled - TLS 비활성화 시 인증서 검사 생략 검증
func TestValidate_TLS_Disabled(t *testing.T) {
	v := newValidViper(t)
	v.Set("server.tls.enabled", false)
	// 인증서 파일을 설정하지 않아도 에러 없어야 함

	err := Validate(v)
	assert.NoError(t, err, "TLS 비활성화 시 인증서 검사를 건너뛰어야 합니다")
}

// TestValidate_TLS_ValidCerts - TLS 활성화 + 유효한 인증서 파일 검증
func TestValidate_TLS_ValidCerts(t *testing.T) {
	// 임시 인증서 파일 생성
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")
	require.NoError(t, os.WriteFile(certFile, []byte("cert"), 0o644))
	require.NoError(t, os.WriteFile(keyFile, []byte("key"), 0o644))

	v := newValidViper(t)
	v.Set("server.tls.enabled", true)
	v.Set("server.tls.cert_file", certFile)
	v.Set("server.tls.key_file", keyFile)

	err := Validate(v)
	assert.NoError(t, err, "유효한 인증서 파일이 있으면 에러가 없어야 합니다")
}

// TestValidate_ProductionJWT - 프로덕션 모드에서 JWT 시크릿 필수 (AC-009)
func TestValidate_ProductionJWT(t *testing.T) {
	v := newValidViper(t)
	v.Set("server.mode", "production")
	v.Set("auth.jwt.secret", "")

	err := Validate(v)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRequiredField),
		"에러는 ErrRequiredField를 포함해야 합니다, 실제: %v", err)
}

// TestValidate_DevelopmentJWT - 개발 모드에서 JWT 시크릿 비필수
func TestValidate_DevelopmentJWT(t *testing.T) {
	v := newValidViper(t)
	v.Set("server.mode", "development")
	v.Set("auth.jwt.secret", "")

	err := Validate(v)
	assert.NoError(t, err, "개발 모드에서는 빈 JWT 시크릿이 허용되어야 합니다")
}

// TestValidate_PositiveValues - 양수 필수 값 검증
func TestValidate_PositiveValues(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"백프레셔 임계값", "engine.backpressure_threshold"},
		{"최대 동시 플로우", "engine.max_concurrent_flows"},
		{"VM 풀 크기", "script.vm_pool_size"},
		{"풀 크기", "storage.pool_size"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newValidViper(t)
			v.Set(tt.key, 0)

			err := Validate(v)
			require.Error(t, err, "%s=0은 에러를 반환해야 합니다", tt.key)
			assert.True(t, errors.Is(err, ErrInvalidPositiveValue),
				"에러는 ErrInvalidPositiveValue를 포함해야 합니다, 실제: %v", err)
		})
	}
}

// TestValidate_InvalidDuration - 유효하지 않은 기간 문자열 검증
func TestValidate_InvalidDuration(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"JWT 액세스 TTL", "auth.jwt.access_ttl"},
		{"JWT 리프레시 TTL", "auth.jwt.refresh_ttl"},
		{"스크립트 타임아웃", "script.timeout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newValidViper(t)
			v.Set(tt.key, "invalid")

			err := Validate(v)
			require.Error(t, err, "%s=\"invalid\"는 에러를 반환해야 합니다", tt.key)
			assert.True(t, errors.Is(err, ErrInvalidDuration),
				"에러는 ErrInvalidDuration을 포함해야 합니다, 실제: %v", err)
		})
	}
}

// TestValidate_LogFormat - observe.format 유효성 검증
func TestValidate_LogFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{"json은 유효", "json", false},
		{"text는 유효", "text", false},
		{"xml은 유효하지 않음", "xml", true},
		{"yaml은 유효하지 않음", "yaml", true},
		{"빈 문자열은 유효하지 않음", "", true},
		{"대문자 JSON은 유효하지 않음", "JSON", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newValidViper(t)
			v.Set("observe.format", tt.format)
			err := Validate(v)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidLogFormat)
			} else {
				if err != nil {
					assert.NotErrorIs(t, err, ErrInvalidLogFormat)
				}
			}
		})
	}
}

// TestValidate_LogOutput - observe.output 유효성 검증
func TestValidate_LogOutput(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantErr bool
	}{
		{"stdout는 유효", "stdout", false},
		{"절대 경로는 유효", "/var/log/xflow.log", false},
		{"상대 경로는 유효", "./logs/xflow.log", false},
		{"stdout+경로는 유효", "stdout+/var/log/xflow.log", false},
		{"stdout+상대경로는 유효", "stdout+./logs/xflow.log", false},
		{"빈 문자열은 유효하지 않음", "", true},
		{"+만 있으면 유효하지 않음", "+", true},
		{"stdout+ 빈 경로는 유효하지 않음", "stdout+", true},
		{"+경로는 유효하지 않음", "+/path", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newValidViper(t)
			v.Set("observe.output", tt.output)
			err := Validate(v)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidLogOutput)
			} else {
				if err != nil {
					assert.NotErrorIs(t, err, ErrInvalidLogOutput)
				}
			}
		})
	}
}

// TestValidate_PostgresDSN - PostgreSQL 타입 선택 시 DSN 필수 검증
func TestValidate_PostgresDSN(t *testing.T) {
	v := newValidViper(t)
	v.Set("storage.type", "postgres")
	v.Set("storage.postgres.dsn", "")

	err := Validate(v)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRequiredField),
		"에러는 ErrRequiredField를 포함해야 합니다, 실제: %v", err)
}

// TestValidate_PostgresDSN_Valid - PostgreSQL + 유효한 DSN 검증
func TestValidate_PostgresDSN_Valid(t *testing.T) {
	v := newValidViper(t)
	v.Set("storage.type", "postgres")
	v.Set("storage.postgres.dsn", "postgres://user:pass@localhost:5432/xflow")

	err := Validate(v)
	assert.NoError(t, err, "유효한 PostgreSQL DSN이 있으면 에러가 없어야 합니다")
}
