// remote_secure_test.go 는 M6 TLS/wss 보안 전송 강제(require_secure)를 검증한다
// (@SPEC:SPEC-REMOTE-001 M6, REQ-F01).
//
// require_secure=true 이고 non-dev(production)이면, 평문 ws:// (client) 또는 TLS
// 미설정(server)을 거부한다. development 모드에서는 평문을 허용한다(로컬 개발 편의).
package config

import (
	"errors"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRequireSecure_RejectsPlaintextClient 는 production + require_secure 에서
// 평문 ws:// server_url 을 거부하는지 검증한다(REQ-F01).
func TestRequireSecure_RejectsPlaintextClient(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(WithConfigPaths(dir), WithDefaults(func(v *viper.Viper) {
		v.SetDefault("server.mode", "production")
		v.SetDefault("remote_management.mode", "client")
		v.SetDefault("remote_management.server_url", "ws://hub.example/api/remote/ws")
		v.SetDefault("remote_management.require_secure", true)
	}))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInsecureTransport),
		"production + require_secure + ws:// 는 ErrInsecureTransport 여야 함")
}

// TestRequireSecure_AllowsWSS 는 require_secure 에서 wss:// 가 허용되는지 확인한다.
func TestRequireSecure_AllowsWSS(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(WithConfigPaths(dir), WithDefaults(func(v *viper.Viper) {
		v.SetDefault("server.mode", "production")
		v.SetDefault("auth.jwt.secret", "prod-secret") // production 필수 필드.
		v.SetDefault("remote_management.mode", "client")
		v.SetDefault("remote_management.server_url", "wss://hub.example/api/remote/ws")
		v.SetDefault("remote_management.require_secure", true)
	}))
	require.NoError(t, err, "wss:// 는 require_secure 에서 허용되어야 함")
}

// TestRequireSecure_AllowsPlaintextInDev 는 development 모드에서는 require_secure
// 라도 평문을 허용하는지 확인한다(로컬 개발 편의 — "non-dev" 한정).
func TestRequireSecure_AllowsPlaintextInDev(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(WithConfigPaths(dir), WithDefaults(func(v *viper.Viper) {
		v.SetDefault("server.mode", "development")
		v.SetDefault("remote_management.mode", "client")
		v.SetDefault("remote_management.server_url", "ws://localhost:8080/api/remote/ws")
		v.SetDefault("remote_management.require_secure", true)
	}))
	require.NoError(t, err, "development 모드는 평문 ws:// 를 허용해야 함")
}

// TestRequireSecure_ServerRequiresTLS 는 production server 모드에서 require_secure
// 인데 TLS 미설정이면 거부하는지 검증한다(REQ-F01).
func TestRequireSecure_ServerRequiresTLS(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(WithConfigPaths(dir), WithDefaults(func(v *viper.Viper) {
		v.SetDefault("server.mode", "production")
		v.SetDefault("remote_management.mode", "server")
		v.SetDefault("remote_management.require_secure", true)
		v.SetDefault("remote_management.tls.enabled", false)
	}))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInsecureTransport),
		"production server + require_secure + TLS off 는 ErrInsecureTransport 여야 함")
}

// TestRequireSecure_DefaultFalse 는 require_secure 기본값이 false 이고 기존 동작이
// 회귀하지 않는지 확인한다(REQ-N03 — disabled/평문 기본 허용).
func TestRequireSecure_DefaultFalse(t *testing.T) {
	cfg, err := Load(WithConfigPaths(t.TempDir()))
	require.NoError(t, err)
	assert.False(t, cfg.RemoteManagement().RequireSecure, "require_secure 기본값은 false")

	// require_secure 미설정 시 production + 평문 client 도 통과(기존 동작 보존).
	_, err = Load(WithConfigPaths(t.TempDir()), WithDefaults(func(v *viper.Viper) {
		v.SetDefault("server.mode", "production")
		v.SetDefault("auth.jwt.secret", "prod-secret") // production 필수 필드.
		v.SetDefault("remote_management.mode", "client")
		v.SetDefault("remote_management.server_url", "ws://hub.example/api/remote/ws")
	}))
	require.NoError(t, err, "require_secure=false 면 평문도 허용(회귀 안전)")
}
