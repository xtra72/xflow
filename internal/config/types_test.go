package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestServerConfig_ZeroValue - ServerConfig 제로값 검증
func TestServerConfig_ZeroValue(t *testing.T) {
	cfg := ServerConfig{}
	assert.Equal(t, 0, cfg.Port)
	assert.Equal(t, "", cfg.Host)
	assert.Equal(t, "", cfg.Mode)
	assert.False(t, cfg.TLS.Enabled)
	assert.False(t, cfg.CORS.Enabled)
	assert.False(t, cfg.RateLimit.Enabled)
}

// TestTLSConfig_Fields - TLSConfig 필드 설정 검증
func TestTLSConfig_Fields(t *testing.T) {
	cfg := TLSConfig{
		Enabled:  true,
		CertFile: "/path/to/cert.pem",
		KeyFile:  "/path/to/key.pem",
	}
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "/path/to/cert.pem", cfg.CertFile)
	assert.Equal(t, "/path/to/key.pem", cfg.KeyFile)
}

// TestCORSConfig_Fields - CORSConfig 필드 설정 검증
func TestCORSConfig_Fields(t *testing.T) {
	cfg := CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"http://localhost:3000", "https://example.com"},
	}
	assert.True(t, cfg.Enabled)
	assert.Len(t, cfg.AllowedOrigins, 2)
	assert.Contains(t, cfg.AllowedOrigins, "http://localhost:3000")
}

// TestRateLimitConfig_Fields - RateLimitConfig 필드 설정 검증
func TestRateLimitConfig_Fields(t *testing.T) {
	cfg := RateLimitConfig{
		Enabled:           true,
		RequestsPerSecond: 100,
	}
	assert.True(t, cfg.Enabled)
	assert.Equal(t, 100, cfg.RequestsPerSecond)
}

// TestEngineConfig_Fields - EngineConfig 필드 설정 검증
func TestEngineConfig_Fields(t *testing.T) {
	cfg := EngineConfig{
		BackpressureThreshold: 1000,
		MaxConcurrentFlows:    100,
		ExecutionPolicy:       "parallel",
		WireDefaultBuffer:     0,
	}
	assert.Equal(t, 1000, cfg.BackpressureThreshold)
	assert.Equal(t, 100, cfg.MaxConcurrentFlows)
	assert.Equal(t, "parallel", cfg.ExecutionPolicy)
	assert.Equal(t, 0, cfg.WireDefaultBuffer)
}

// TestStorageConfig_Fields - StorageConfig 필드 설정 검증
func TestStorageConfig_Fields(t *testing.T) {
	cfg := StorageConfig{
		Type:        "sqlite",
		SQLitePath:  "./data/xflow.db",
		PostgresDSN: "",
		PoolSize:    10,
	}
	assert.Equal(t, "sqlite", cfg.Type)
	assert.Equal(t, "./data/xflow.db", cfg.SQLitePath)
	assert.Equal(t, "", cfg.PostgresDSN)
	assert.Equal(t, 10, cfg.PoolSize)
}

// TestAuthConfig_Fields - AuthConfig 및 하위 구조체 필드 설정 검증
func TestAuthConfig_Fields(t *testing.T) {
	cfg := AuthConfig{
		JWT: JWTConfig{
			Secret:     "my-secret",
			AccessTTL:  "15m",
			RefreshTTL: "168h",
		},
		APIKey: APIKeyConfig{
			Enabled: true,
		},
		OAuth2: OAuth2Config{
			Providers: []string{"google", "github"},
		},
	}
	assert.Equal(t, "my-secret", cfg.JWT.Secret)
	assert.Equal(t, "15m", cfg.JWT.AccessTTL)
	assert.Equal(t, "168h", cfg.JWT.RefreshTTL)
	assert.True(t, cfg.APIKey.Enabled)
	assert.Len(t, cfg.OAuth2.Providers, 2)
}

// TestObserveConfig_Fields - ObserveConfig 필드 설정 검증
func TestObserveConfig_Fields(t *testing.T) {
	cfg := ObserveConfig{
		DefaultLevel:   "info",
		MetricsEnabled: true,
		TraceEnabled:   false,
		Format:         "json",
		Output:         "stdout",
	}
	assert.Equal(t, "info", cfg.DefaultLevel)
	assert.True(t, cfg.MetricsEnabled)
	assert.False(t, cfg.TraceEnabled)
	assert.Equal(t, "json", cfg.Format)
	assert.Equal(t, "stdout", cfg.Output)
}

// TestScriptConfig_Fields - ScriptConfig 및 SandboxConfig 필드 설정 검증
func TestScriptConfig_Fields(t *testing.T) {
	cfg := ScriptConfig{
		Timeout:    "5s",
		VMPoolSize: 10,
		Sandbox: SandboxConfig{
			Enabled:        true,
			MaxMemoryMB:    64,
			MaxExecutionMS: 5000,
		},
	}
	assert.Equal(t, "5s", cfg.Timeout)
	assert.Equal(t, 10, cfg.VMPoolSize)
	assert.True(t, cfg.Sandbox.Enabled)
	assert.Equal(t, 64, cfg.Sandbox.MaxMemoryMB)
	assert.Equal(t, 5000, cfg.Sandbox.MaxExecutionMS)
}

// TestPluginConfig_Fields - PluginConfig 필드 설정 검증
func TestPluginConfig_Fields(t *testing.T) {
	cfg := PluginConfig{
		Directory:   "./plugins",
		WASMEnabled: true,
		GoEnabled:   true,
	}
	assert.Equal(t, "./plugins", cfg.Directory)
	assert.True(t, cfg.WASMEnabled)
	assert.True(t, cfg.GoEnabled)
}
