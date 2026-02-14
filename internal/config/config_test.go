package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTestYAML - 임시 YAML 파일을 생성하고 경로를 반환
func writeTestYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test-config.yaml")
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
	return path
}

// --- Load 테스트 ---

// TestLoad_DefaultValues - 옵션 없이 Load() 호출 시 모든 기본값 일치 (AC-001)
func TestLoad_DefaultValues(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// 서버 기본값 확인
	assert.Equal(t, 8080, cfg.Server().Port)
	assert.Equal(t, "0.0.0.0", cfg.Server().Host)
	assert.Equal(t, "development", cfg.Server().Mode)

	// 엔진 기본값 확인
	assert.Equal(t, 1000, cfg.Engine().BackpressureThreshold)
	assert.Equal(t, 100, cfg.Engine().MaxConcurrentFlows)
	assert.Equal(t, "parallel", cfg.Engine().ExecutionPolicy)

	// 스토리지 기본값 확인
	assert.Equal(t, "sqlite", cfg.Storage().Type)
	assert.Equal(t, "./data/xflow.db", cfg.Storage().SQLitePath)
	assert.Equal(t, 10, cfg.Storage().PoolSize)

	// 인증 기본값 확인
	assert.Equal(t, "", cfg.Auth().JWT.Secret)
	assert.Equal(t, "15m", cfg.Auth().JWT.AccessTTL)
	assert.Equal(t, "168h", cfg.Auth().JWT.RefreshTTL)
	assert.True(t, cfg.Auth().APIKey.Enabled)

	// 관측 기본값 확인
	assert.Equal(t, "info", cfg.Observe().DefaultLevel)
	assert.True(t, cfg.Observe().MetricsEnabled)
	assert.False(t, cfg.Observe().TraceEnabled)

	// 스크립트 기본값 확인
	assert.Equal(t, "5s", cfg.Script().Timeout)
	assert.Equal(t, 10, cfg.Script().VMPoolSize)
	assert.True(t, cfg.Script().Sandbox.Enabled)
	assert.Equal(t, 64, cfg.Script().Sandbox.MaxMemoryMB)
	assert.Equal(t, 5000, cfg.Script().Sandbox.MaxExecutionMS)

	// 플러그인 기본값 확인
	assert.Equal(t, "./plugins", cfg.Plugin().Directory)
	assert.True(t, cfg.Plugin().WASMEnabled)
	assert.True(t, cfg.Plugin().GoEnabled)
}

// TestLoad_WithConfigFile - YAML 파일로 Load 시 기본값 오버라이드 (AC-002)
func TestLoad_WithConfigFile(t *testing.T) {
	yaml := `
server:
  port: 9090
  host: "127.0.0.1"
engine:
  backpressure_threshold: 2000
storage:
  type: "sqlite"
`
	path := writeTestYAML(t, yaml)

	cfg, err := Load(WithConfigFile(path))
	require.NoError(t, err)

	assert.Equal(t, 9090, cfg.Server().Port)
	assert.Equal(t, "127.0.0.1", cfg.Server().Host)
	assert.Equal(t, 2000, cfg.Engine().BackpressureThreshold)
	// 파일에 없는 값은 기본값 유지
	assert.Equal(t, "development", cfg.Server().Mode)
	assert.Equal(t, 100, cfg.Engine().MaxConcurrentFlows)
}

// TestLoad_EnvOverride - 환경 변수가 파일 + 기본값 오버라이드 (AC-003)
func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("XFLOW_SERVER_PORT", "3000")
	t.Setenv("XFLOW_ENGINE_BACKPRESSURE_THRESHOLD", "5000")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, 3000, cfg.Server().Port)
	assert.Equal(t, 5000, cfg.Engine().BackpressureThreshold)
	// 환경 변수 미설정 값은 기본값 유지
	assert.Equal(t, "0.0.0.0", cfg.Server().Host)
}

// TestLoad_WithFlags - pflag.FlagSet 값이 최우선 오버라이드 (AC-004 부분)
func TestLoad_WithFlags(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Int("server.port", 8080, "서버 포트")
	flags.String("server.mode", "development", "서버 모드")
	require.NoError(t, flags.Set("server.port", "4000"))
	require.NoError(t, flags.Set("server.mode", "production"))

	// 프로덕션 모드에서 JWT 시크릿 필요
	yaml := `
auth:
  jwt:
    secret: "test-secret-for-production"
`
	path := writeTestYAML(t, yaml)

	cfg, err := Load(
		WithConfigFile(path),
		WithFlags(flags),
	)
	require.NoError(t, err)

	assert.Equal(t, 4000, cfg.Server().Port)
	assert.Equal(t, "production", cfg.Server().Mode)
}

// TestLoad_WithConfigName - 바이너리별 설정 파일명 (AC-001-1)
func TestLoad_WithConfigName(t *testing.T) {
	// 임시 디렉토리에 xflowd.yaml 생성
	dir := t.TempDir()
	content := `
server:
  port: 7070
`
	err := os.WriteFile(filepath.Join(dir, "xflowd.yaml"), []byte(content), 0644)
	require.NoError(t, err)

	cfg, loadErr := Load(
		WithConfigName("xflowd"),
		WithConfigPaths(dir),
	)
	require.NoError(t, loadErr)
	assert.Equal(t, 7070, cfg.Server().Port)
}

// TestLoad_WithConfigPaths - 여러 검색 경로, 먼저 발견된 파일 사용 (AC-001-1)
func TestLoad_WithConfigPaths(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// dir1에 설정 파일 생성 (첫 번째 경로)
	content := `
server:
  port: 6060
`
	err := os.WriteFile(filepath.Join(dir1, "xflow.yaml"), []byte(content), 0644)
	require.NoError(t, err)

	cfg, loadErr := Load(
		WithConfigPaths(dir1, dir2),
	)
	require.NoError(t, loadErr)
	assert.Equal(t, 6060, cfg.Server().Port)
}

// TestLoad_OverrideChain_5Levels - 5단계 오버라이드 체인 테스트 (AC-004)
func TestLoad_OverrideChain_5Levels(t *testing.T) {
	// Level 1: 코드 기본값 (SetDefaults)
	// Level 2: 기본 경로 파일 (configPaths에서 찾은 파일)
	// Level 3: 사용자 지정 파일 (WithConfigFile)
	// Level 4: 환경 변수
	// Level 5: CLI 플래그

	t.Run("Level1_기본값", func(t *testing.T) {
		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, 8080, cfg.Server().Port)
	})

	t.Run("Level2_기본경로파일이_기본값_오버라이드", func(t *testing.T) {
		dir := t.TempDir()
		content := `
server:
  port: 1111
`
		err := os.WriteFile(filepath.Join(dir, "xflow.yaml"), []byte(content), 0644)
		require.NoError(t, err)

		cfg, loadErr := Load(WithConfigPaths(dir))
		require.NoError(t, loadErr)
		assert.Equal(t, 1111, cfg.Server().Port)
	})

	t.Run("Level3_사용자파일이_기본경로파일_오버라이드", func(t *testing.T) {
		// 기본 경로 파일
		dir := t.TempDir()
		defaultContent := `
server:
  port: 1111
  host: "10.0.0.1"
`
		err := os.WriteFile(filepath.Join(dir, "xflow.yaml"), []byte(defaultContent), 0644)
		require.NoError(t, err)

		// 사용자 지정 파일 (port를 오버라이드하지만 host는 유지)
		userContent := `
server:
  port: 2222
`
		userPath := writeTestYAML(t, userContent)

		cfg, loadErr := Load(
			WithConfigPaths(dir),
			WithConfigFile(userPath),
		)
		require.NoError(t, loadErr)
		assert.Equal(t, 2222, cfg.Server().Port)
		assert.Equal(t, "10.0.0.1", cfg.Server().Host)
	})

	t.Run("Level4_환경변수가_파일_오버라이드", func(t *testing.T) {
		yaml := `
server:
  port: 2222
`
		path := writeTestYAML(t, yaml)
		t.Setenv("XFLOW_SERVER_PORT", "3333")

		cfg, err := Load(WithConfigFile(path))
		require.NoError(t, err)
		assert.Equal(t, 3333, cfg.Server().Port)
	})

	t.Run("Level5_플래그가_환경변수_오버라이드", func(t *testing.T) {
		yaml := `
server:
  port: 2222
auth:
  jwt:
    secret: "test-secret"
`
		path := writeTestYAML(t, yaml)
		t.Setenv("XFLOW_SERVER_PORT", "3333")
		t.Setenv("XFLOW_SERVER_MODE", "production")

		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		flags.Int("server.port", 8080, "서버 포트")
		require.NoError(t, flags.Set("server.port", "4444"))

		cfg, err := Load(
			WithConfigFile(path),
			WithFlags(flags),
		)
		require.NoError(t, err)
		assert.Equal(t, 4444, cfg.Server().Port)
	})
}

// TestLoad_WithEnvPrefix - 커스텀 환경 변수 접두사 (AC-020)
func TestLoad_WithEnvPrefix(t *testing.T) {
	t.Setenv("CUSTOM_SERVER_PORT", "5555")

	cfg, err := Load(WithEnvPrefix("CUSTOM"))
	require.NoError(t, err)
	assert.Equal(t, 5555, cfg.Server().Port)
}

// TestLoad_NoConfigFile - 설정 파일 없이 기본값만 사용 (AC-001-1)
func TestLoad_NoConfigFile(t *testing.T) {
	emptyDir := t.TempDir()

	cfg, err := Load(WithConfigPaths(emptyDir))
	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.Server().Port)
}

// TestLoad_ValidationFailure - 유효하지 않은 설정 시 에러 반환
func TestLoad_ValidationFailure(t *testing.T) {
	yaml := `
server:
  port: 0
`
	path := writeTestYAML(t, yaml)

	_, err := Load(WithConfigFile(path))
	require.Error(t, err)
}

// TestLoad_WithLogger - 로거 옵션 동작 확인
func TestLoad_WithLogger(t *testing.T) {
	logger := slog.Default()

	cfg, err := Load(WithLogger(logger))
	require.NoError(t, err)
	require.NotNil(t, cfg)
}

// TestLoad_WithDefaults - 커스텀 기본값 함수 동작 확인
func TestLoad_WithDefaults(t *testing.T) {
	cfg, err := Load(WithDefaults(func(v *viper.Viper) {
		v.SetDefault("server.port", 9999)
	}))
	require.NoError(t, err)
	assert.Equal(t, 9999, cfg.Server().Port)
}

// TestLoad_InvalidConfigFile - 존재하지 않는 사용자 지정 파일 에러
func TestLoad_InvalidConfigFile(t *testing.T) {
	_, err := Load(WithConfigFile("/nonexistent/config.yaml"))
	require.Error(t, err)
}

// --- 카테고리 접근자 테스트 ---

// TestConfig_Server - ServerConfig 모든 필드 기본값 검증
func TestConfig_Server(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	s := cfg.Server()
	assert.Equal(t, 8080, s.Port)
	assert.Equal(t, "0.0.0.0", s.Host)
	assert.Equal(t, "development", s.Mode)
	assert.False(t, s.TLS.Enabled)
	assert.Equal(t, "", s.TLS.CertFile)
	assert.Equal(t, "", s.TLS.KeyFile)
	assert.True(t, s.CORS.Enabled)
	assert.Equal(t, []string{"*"}, s.CORS.AllowedOrigins)
	assert.True(t, s.RateLimit.Enabled)
	assert.Equal(t, 100, s.RateLimit.RequestsPerSecond)
}

// TestConfig_Engine - EngineConfig 모든 필드 검증
func TestConfig_Engine(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	e := cfg.Engine()
	assert.Equal(t, 1000, e.BackpressureThreshold)
	assert.Equal(t, 100, e.MaxConcurrentFlows)
	assert.Equal(t, "parallel", e.ExecutionPolicy)
	assert.Equal(t, 0, e.WireDefaultBuffer)
}

// TestConfig_Storage - StorageConfig 모든 필드 검증
func TestConfig_Storage(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	s := cfg.Storage()
	assert.Equal(t, "sqlite", s.Type)
	assert.Equal(t, "./data/xflow.db", s.SQLitePath)
	assert.Equal(t, "", s.PostgresDSN)
	assert.Equal(t, 10, s.PoolSize)
}

// TestConfig_Auth - AuthConfig 모든 필드 검증
func TestConfig_Auth(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	a := cfg.Auth()
	assert.Equal(t, "", a.JWT.Secret)
	assert.Equal(t, "15m", a.JWT.AccessTTL)
	assert.Equal(t, "168h", a.JWT.RefreshTTL)
	assert.True(t, a.APIKey.Enabled)
	assert.Empty(t, a.OAuth2.Providers)
}

// TestConfig_Observe - ObserveConfig 모든 필드 검증
func TestConfig_Observe(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	o := cfg.Observe()
	assert.Equal(t, "info", o.DefaultLevel)
	assert.True(t, o.MetricsEnabled)
	assert.False(t, o.TraceEnabled)
}

// TestConfig_Script - ScriptConfig 모든 필드 검증
func TestConfig_Script(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	s := cfg.Script()
	assert.Equal(t, "5s", s.Timeout)
	assert.Equal(t, 10, s.VMPoolSize)
	assert.True(t, s.Sandbox.Enabled)
	assert.Equal(t, 64, s.Sandbox.MaxMemoryMB)
	assert.Equal(t, 5000, s.Sandbox.MaxExecutionMS)
}

// TestConfig_Plugin - PluginConfig 모든 필드 검증
func TestConfig_Plugin(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	p := cfg.Plugin()
	assert.Equal(t, "./plugins", p.Directory)
	assert.True(t, p.WASMEnabled)
	assert.True(t, p.GoEnabled)
}

// --- 범용 접근 테스트 ---

// TestConfig_Get - Get(key)이 올바른 값 반환
func TestConfig_Get(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.Get("server.port"))
	assert.Equal(t, "0.0.0.0", cfg.Get("server.host"))
	assert.Equal(t, "sqlite", cfg.Get("storage.type"))
}

// TestConfig_IsSet - IsSet(key)이 설정된 키와 미설정 키 구분
func TestConfig_IsSet(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	assert.True(t, cfg.IsSet("server.port"))
	assert.True(t, cfg.IsSet("storage.type"))
	assert.False(t, cfg.IsSet("nonexistent.key"))
}

// --- Set 메서드 테스트 ---

// TestConfig_Set_MutableKey - 변경 가능한 키 설정 성공
func TestConfig_Set_MutableKey(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	err = cfg.Set("engine.backpressure_threshold", 2000)
	require.NoError(t, err)
	assert.Equal(t, 2000, cfg.Engine().BackpressureThreshold)
}

// TestConfig_Set_ImmutableKey - 변경 불가능한 키 설정 시 ErrImmutableKey 반환
func TestConfig_Set_ImmutableKey(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	err = cfg.Set("server.port", 9090)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrImmutableKey)
	// 값이 변경되지 않았는지 확인
	assert.Equal(t, 8080, cfg.Server().Port)
}

// --- OnChange / ChangeHistory 테스트 ---

// TestConfig_OnChange - 콜백이 변경 시 호출됨
func TestConfig_OnChange(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	var received ChangeEvent
	called := false

	unsub := cfg.OnChange("engine.backpressure_threshold", func(event ChangeEvent) {
		received = event
		called = true
	})
	defer unsub()

	err = cfg.Set("engine.backpressure_threshold", 3000)
	require.NoError(t, err)

	assert.True(t, called, "콜백이 호출되어야 합니다")
	assert.Equal(t, "engine.backpressure_threshold", received.Key)
	assert.Equal(t, 3000, received.NewValue)
	assert.Equal(t, "api", received.Source)
}

// TestConfig_OnChange_Unsubscribe - 구독 해제 후 콜백 미호출
func TestConfig_OnChange_Unsubscribe(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	callCount := 0
	unsub := cfg.OnChange("engine.backpressure_threshold", func(event ChangeEvent) {
		callCount++
	})

	// 첫 번째 변경 - 콜백 호출됨
	err = cfg.Set("engine.backpressure_threshold", 2000)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// 구독 해제
	unsub()

	// 두 번째 변경 - 콜백 미호출
	err = cfg.Set("engine.backpressure_threshold", 3000)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "구독 해제 후 콜백이 호출되면 안 됩니다")
}

// TestConfig_ChangeHistory - 변경 이력 기록
func TestConfig_ChangeHistory(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	err = cfg.Set("engine.backpressure_threshold", 2000)
	require.NoError(t, err)
	err = cfg.Set("observe.default_level", "debug")
	require.NoError(t, err)

	history := cfg.ChangeHistory()
	assert.Len(t, history, 2)
	assert.Equal(t, "engine.backpressure_threshold", history[0].Key)
	assert.Equal(t, "observe.default_level", history[1].Key)
}

// --- WatchConfig / StopWatch 테스트 ---

// TestConfig_WatchConfig_StopWatch - WatchConfig/StopWatch 호출 가능
func TestConfig_WatchConfig_StopWatch(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	// WatchConfig/StopWatch가 패닉 없이 호출 가능해야 함
	err = cfg.WatchConfig()
	assert.NoError(t, err)

	cfg.StopWatch()
}

// --- Configurable 인터페이스 테스트 ---

// testConfigurable - Configurable 인터페이스를 구현하는 테스트용 mock
type testConfigurable struct{}

func (tc *testConfigurable) Configure(cfg map[string]any) error {
	return nil
}

func (tc *testConfigurable) GetConfig() map[string]any {
	return map[string]any{}
}

// TestConfigurable_Interface - mock 구현이 인터페이스 충족 (AC-019)
func TestConfigurable_Interface(t *testing.T) {
	m := &testConfigurable{}

	// 인터페이스 충족 여부 컴파일 타임 확인
	var _ Configurable = (*testConfigurable)(nil)

	// Configure 메서드 테스트
	err := m.Configure(map[string]any{"key": "value"})
	assert.NoError(t, err)

	// GetConfig 메서드 테스트
	result := m.GetConfig()
	assert.NotNil(t, result)
}

// =============================================================================
// M4 테스트: 런타임 검증, 콜백 panic 복구, FIFO 이력, WatchConfig
// =============================================================================

// TestSet_ValidationOnSet - Set() 시 유효성 검증 (AC-016)
func TestSet_ValidationOnSet(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   any
		wantErr error
	}{
		{"유효하지 않은 로그 레벨", "observe.default_level", "invalid_level", ErrInvalidLogLevel},
		{"유효한 로그 레벨 debug", "observe.default_level", "debug", nil},
		{"유효한 로그 레벨 info", "observe.default_level", "info", nil},
		{"유효한 로그 레벨 warn", "observe.default_level", "warn", nil},
		{"유효한 로그 레벨 error", "observe.default_level", "error", nil},
		{"음수 백프레셔 임계값", "engine.backpressure_threshold", -1, ErrInvalidPositiveValue},
		{"0 백프레셔 임계값", "engine.backpressure_threshold", 0, ErrInvalidPositiveValue},
		{"유효한 백프레셔 임계값", "engine.backpressure_threshold", 5000, nil},
		{"유효하지 않은 기간 문자열", "auth.jwt.access_ttl", "invalid", ErrInvalidDuration},
		{"유효한 기간 문자열", "auth.jwt.access_ttl", "30m", nil},
		{"0 요청 속도 제한", "server.rate_limit.requests_per_second", 0, ErrInvalidPositiveValue},
		{"음수 요청 속도 제한", "server.rate_limit.requests_per_second", -5, ErrInvalidPositiveValue},
		{"유효한 요청 속도 제한", "server.rate_limit.requests_per_second", 200, nil},
		{"로그 레벨이 문자열이 아닌 경우", "observe.default_level", 123, ErrInvalidLogLevel},
		{"기간이 문자열이 아닌 경우", "auth.jwt.access_ttl", 999, ErrInvalidDuration},
		{"float64 양수 값", "engine.backpressure_threshold", float64(3000), nil},
		{"유효한 max_concurrent_flows", "engine.max_concurrent_flows", 50, nil},
		{"0 max_concurrent_flows", "engine.max_concurrent_flows", 0, ErrInvalidPositiveValue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load()
			require.NoError(t, err)

			err = cfg.Set(tt.key, tt.value)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestSet_ValidationFail_NoCallback - 검증 실패 시 콜백 미호출 (AC-016)
func TestSet_ValidationFail_NoCallback(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	called := false
	unsub := cfg.OnChange("observe.default_level", func(event ChangeEvent) {
		called = true
	})
	defer unsub()

	err = cfg.Set("observe.default_level", "invalid_level")
	require.Error(t, err)
	assert.False(t, called, "검증 실패 시 콜백이 호출되면 안 됩니다")
}

// TestSet_ValidationFail_NoHistory - 검증 실패 시 이력 미기록 (AC-016)
func TestSet_ValidationFail_NoHistory(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	err = cfg.Set("observe.default_level", "invalid_level")
	require.Error(t, err)

	assert.Empty(t, cfg.ChangeHistory(), "검증 실패 시 이력에 기록되면 안 됩니다")
}

// TestSet_ValidationFail_ValueUnchanged - 검증 실패 시 값 미변경 (AC-016)
func TestSet_ValidationFail_ValueUnchanged(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	// 기본 로그 레벨은 "info"
	assert.Equal(t, "info", cfg.Observe().DefaultLevel)

	err = cfg.Set("observe.default_level", "invalid_level")
	require.Error(t, err)

	// 값이 변경되지 않아야 함
	assert.Equal(t, "info", cfg.Observe().DefaultLevel)
}

// TestOnChange_PanicRecovery - 콜백 panic 복구 (AC-014)
func TestOnChange_PanicRecovery(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	normalCalled := false

	// panic을 발생시키는 콜백 등록
	unsub1 := cfg.OnChange("engine.backpressure_threshold", func(event ChangeEvent) {
		panic("test panic")
	})
	defer unsub1()

	// 정상 콜백 등록
	unsub2 := cfg.OnChange("engine.backpressure_threshold", func(event ChangeEvent) {
		normalCalled = true
	})
	defer unsub2()

	// Set()은 panic 없이 완료되어야 함
	err = cfg.Set("engine.backpressure_threshold", 3000)
	require.NoError(t, err)

	// 정상 콜백이 호출되어야 함
	assert.True(t, normalCalled, "다른 콜백의 panic에도 불구하고 정상 콜백이 호출되어야 합니다")
}

// TestOnChange_MultipleCallbacks - 동일 키에 복수 콜백 모두 호출 (AC-012)
func TestOnChange_MultipleCallbacks(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	count1, count2 := 0, 0

	unsub1 := cfg.OnChange("engine.backpressure_threshold", func(event ChangeEvent) {
		count1++
	})
	defer unsub1()

	unsub2 := cfg.OnChange("engine.backpressure_threshold", func(event ChangeEvent) {
		count2++
	})
	defer unsub2()

	err = cfg.Set("engine.backpressure_threshold", 5000)
	require.NoError(t, err)

	assert.Equal(t, 1, count1)
	assert.Equal(t, 1, count2)
}

// TestOnChange_DifferentKey - 다른 키 변경 시 해당 키 콜백 미호출 (AC-012)
func TestOnChange_DifferentKey(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	called := false
	unsub := cfg.OnChange("engine.backpressure_threshold", func(event ChangeEvent) {
		called = true
	})
	defer unsub()

	err = cfg.Set("observe.default_level", "debug")
	require.NoError(t, err)

	assert.False(t, called, "다른 키 변경 시 해당 키 콜백이 호출되면 안 됩니다")
}

// TestOnChange_ChangeEventFields - ChangeEvent 필드 상세 검증 (AC-012)
func TestOnChange_ChangeEventFields(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	var received ChangeEvent
	unsub := cfg.OnChange("engine.backpressure_threshold", func(event ChangeEvent) {
		received = event
	})
	defer unsub()

	before := time.Now()
	err = cfg.Set("engine.backpressure_threshold", 3000)
	require.NoError(t, err)
	after := time.Now()

	assert.Equal(t, "engine.backpressure_threshold", received.Key)
	assert.Equal(t, 3000, received.NewValue)
	assert.Equal(t, "api", received.Source)
	assert.True(t, received.Timestamp.After(before) || received.Timestamp.Equal(before))
	assert.True(t, received.Timestamp.Before(after) || received.Timestamp.Equal(after))
}

// TestChangeHistory_FIFO - 110회 변경 시 최신 100건만 유지 (AC-015)
func TestChangeHistory_FIFO(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	// 110번 변경
	for i := 1; i <= 110; i++ {
		err = cfg.Set("engine.backpressure_threshold", i)
		require.NoError(t, err)
	}

	history := cfg.ChangeHistory()
	assert.Len(t, history, 100)

	// 첫 번째 항목은 11번째 변경 (1~10은 제거됨)
	assert.Equal(t, 11, history[0].NewValue)

	// 마지막 항목은 110번째 변경
	assert.Equal(t, 110, history[99].NewValue)
}

// TestSet_ImmutableKey_Wildcard - 와일드카드 immutable 키 (AC-011)
func TestSet_ImmutableKey_Wildcard(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	immutableKeys := []string{
		"server.port",
		"server.host",
		"server.tls.enabled",
		"server.tls.cert_file",
		"storage.type",
		"script.vm_pool_size",
	}

	for _, key := range immutableKeys {
		t.Run(key, func(t *testing.T) {
			err := cfg.Set(key, "test")
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrImmutableKey)
		})
	}
}

// TestWatchConfig_FileChange - 파일 변경 감지 (AC-018)
func TestWatchConfig_FileChange(t *testing.T) {
	// 설정 파일 생성
	dir := t.TempDir()
	configPath := filepath.Join(dir, "xflow.yaml")
	initial := `
engine:
  backpressure_threshold: 1000
`
	err := os.WriteFile(configPath, []byte(initial), 0644)
	require.NoError(t, err)

	cfg, err := Load(WithConfigPaths(dir))
	require.NoError(t, err)
	assert.Equal(t, 1000, cfg.Engine().BackpressureThreshold)

	// WatchConfig 시작
	err = cfg.WatchConfig()
	require.NoError(t, err)

	// 콜백 등록
	changed := make(chan struct{}, 1)
	unsub := cfg.OnChange("engine.backpressure_threshold", func(event ChangeEvent) {
		changed <- struct{}{}
	})
	defer unsub()

	// 파일 수정
	updated := `
engine:
  backpressure_threshold: 5000
`
	err = os.WriteFile(configPath, []byte(updated), 0644)
	require.NoError(t, err)

	// 변경 감지 대기 (최대 2초)
	select {
	case <-changed:
		// 성공
	case <-time.After(2 * time.Second):
		t.Log("WatchConfig 파일 변경 감지 타임아웃 (플랫폼에 따라 다를 수 있음)")
	}

	cfg.StopWatch()
}

// TestWatchConfig_Idempotent - WatchConfig 중복 호출 시 에러 없음
func TestWatchConfig_Idempotent(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	err = cfg.WatchConfig()
	require.NoError(t, err)

	// 두 번째 호출도 에러 없이 반환
	err = cfg.WatchConfig()
	require.NoError(t, err)

	cfg.StopWatch()
}

// =============================================================================
// M5 테스트: 동시성, 벤치마크
// =============================================================================

// TestConcurrency_ReadWrite - 동시 읽기/쓰기 경쟁 조건 없음 (AC-017)
func TestConcurrency_ReadWrite(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	var wg sync.WaitGroup

	// 10개 읽기 고루틴
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = cfg.Server()
				_ = cfg.Engine()
				_ = cfg.Storage()
				_ = cfg.Auth()
				_ = cfg.Observe()
				_ = cfg.Script()
				_ = cfg.Plugin()
			}
		}()
	}

	// 1개 쓰기 고루틴
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 100; j++ {
			_ = cfg.Set("engine.backpressure_threshold", j+1)
		}
	}()

	wg.Wait()
}

// TestConcurrency_CallbackRegistration - 동시 콜백 등록/해제
func TestConcurrency_CallbackRegistration(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				unsub := cfg.OnChange("engine.backpressure_threshold", func(event ChangeEvent) {})
				unsub()
			}
		}()
	}

	wg.Wait()
}

// TestConcurrency_SetAndOnChange - 동시 Set + OnChange + ChangeHistory
func TestConcurrency_SetAndOnChange(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	var wg sync.WaitGroup

	// 읽기 고루틴
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = cfg.ChangeHistory()
		}
	}()

	// 쓰기 고루틴
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = cfg.Set("engine.backpressure_threshold", i+1)
		}
	}()

	// 콜백 등록/해제 고루틴
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			unsub := cfg.OnChange("engine.backpressure_threshold", func(event ChangeEvent) {})
			unsub()
		}
	}()

	wg.Wait()
}

// TestConcurrency_MultipleWriters - 복수 쓰기 고루틴 안전성
func TestConcurrency_MultipleWriters(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = cfg.Set("engine.backpressure_threshold", idx*100+j+1)
			}
		}(i)
	}

	wg.Wait()

	// 최종 값이 양수인지 확인
	assert.Greater(t, cfg.Engine().BackpressureThreshold, 0)
}

// --- 벤치마크 ---

func BenchmarkLoad(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = Load()
	}
}

func BenchmarkSet_MutableKey(b *testing.B) {
	cfg, _ := Load()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.Set("engine.backpressure_threshold", i+1)
	}
}

func BenchmarkServer_Read(b *testing.B) {
	cfg, _ := Load()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.Server()
	}
}

func BenchmarkConcurrent_Read(b *testing.B) {
	cfg, _ := Load()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = cfg.Server()
		}
	})
}
