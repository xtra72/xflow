package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// newViperWithDefaults - 기본값이 설정된 Viper 인스턴스 생성 헬퍼
func newViperWithDefaults(t *testing.T) *viper.Viper {
	t.Helper()
	v := viper.New()
	SetDefaults(v)
	return v
}

// TestSetDefaults_Server - 서버 기본값 검증
func TestSetDefaults_Server(t *testing.T) {
	v := newViperWithDefaults(t)

	assert.Equal(t, 8080, v.GetInt("server.port"))
	assert.Equal(t, "0.0.0.0", v.GetString("server.host"))
	assert.Equal(t, "development", v.GetString("server.mode"))
	assert.False(t, v.GetBool("server.tls.enabled"))
	assert.True(t, v.GetBool("server.cors.enabled"))
	assert.Equal(t, []string{"*"}, v.GetStringSlice("server.cors.allowed_origins"))
	assert.True(t, v.GetBool("server.rate_limit.enabled"))
	assert.Equal(t, 100, v.GetInt("server.rate_limit.requests_per_second"))
}

// TestSetDefaults_Engine - 엔진 기본값 검증
func TestSetDefaults_Engine(t *testing.T) {
	v := newViperWithDefaults(t)

	assert.Equal(t, 1000, v.GetInt("engine.backpressure_threshold"))
	assert.Equal(t, 100, v.GetInt("engine.max_concurrent_flows"))
	assert.Equal(t, "parallel", v.GetString("engine.execution_policy"))
	assert.Equal(t, 0, v.GetInt("engine.wire_default_buffer"))
}

// TestSetDefaults_Storage - 스토리지 기본값 검증
func TestSetDefaults_Storage(t *testing.T) {
	v := newViperWithDefaults(t)

	assert.Equal(t, "sqlite", v.GetString("storage.type"))
	assert.Equal(t, "./data/xflow.db", v.GetString("storage.sqlite.path"))
	assert.Equal(t, 10, v.GetInt("storage.pool_size"))
}

// TestSetDefaults_Auth - 인증 기본값 검증
func TestSetDefaults_Auth(t *testing.T) {
	v := newViperWithDefaults(t)

	assert.Equal(t, "", v.GetString("auth.jwt.secret"))
	assert.Equal(t, "15m", v.GetString("auth.jwt.access_ttl"))
	assert.Equal(t, "168h", v.GetString("auth.jwt.refresh_ttl"))
	assert.True(t, v.GetBool("auth.api_key.enabled"))
}

// TestSetDefaults_Observe - 관측 기본값 검증
func TestSetDefaults_Observe(t *testing.T) {
	v := newViperWithDefaults(t)

	assert.Equal(t, "info", v.GetString("observe.default_level"))
	assert.True(t, v.GetBool("observe.metrics.enabled"))
	assert.False(t, v.GetBool("observe.trace.enabled"))
	assert.Equal(t, "json", v.GetString("observe.format"))
	assert.Equal(t, "stdout", v.GetString("observe.output"))
}

// TestSetDefaults_Script - 스크립트 기본값 검증
func TestSetDefaults_Script(t *testing.T) {
	v := newViperWithDefaults(t)

	assert.Equal(t, "5s", v.GetString("script.timeout"))
	assert.Equal(t, 10, v.GetInt("script.vm_pool_size"))
	assert.True(t, v.GetBool("script.sandbox.enabled"))
	assert.Equal(t, 64, v.GetInt("script.sandbox.max_memory_mb"))
	assert.Equal(t, 5000, v.GetInt("script.sandbox.max_execution_ms"))
}

// TestSetDefaults_Plugin - 플러그인 기본값 검증
func TestSetDefaults_Plugin(t *testing.T) {
	v := newViperWithDefaults(t)

	assert.Equal(t, "./plugins", v.GetString("plugin.directory"))
	assert.True(t, v.GetBool("plugin.wasm.enabled"))
	assert.True(t, v.GetBool("plugin.go.enabled"))
}
