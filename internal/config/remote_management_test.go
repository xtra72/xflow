package config

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTempFile 는 빈 파일을 생성한다(TLS 검증 테스트용 cert/key 스텁).
func writeTempFile(path string) error {
	return os.WriteFile(path, []byte("stub"), 0o600)
}

// TestRemoteManagement_Defaults 는 remote_management 기본값을 검증한다
// (REQ-REMOTE-A01/A04/A05, N03 — 기본 disabled 회귀 안전).
func TestRemoteManagement_Defaults(t *testing.T) {
	cfg, err := Load(WithConfigPaths(t.TempDir()))
	require.NoError(t, err)

	rm := cfg.RemoteManagement()
	assert.Equal(t, "disabled", rm.Mode, "기본 모드는 disabled 여야 함(회귀 안전)")
	assert.Equal(t, "", rm.ServerURL)
	assert.Equal(t, "", rm.InstanceID)
	assert.True(t, rm.AutoRegister, "auto_register 기본값은 true")
	assert.Equal(t, 30*time.Second, rm.HeartbeatInterval, "heartbeat 안전 기본값 30s")
	assert.Equal(t, "", rm.BootstrapSecret)
	assert.False(t, rm.TLS.Enabled)
}

// TestRemoteManagement_Accessor 는 설정값이 accessor 로 올바르게 노출되는지
// 검증한다(REQ-REMOTE-A01/A02/A04/A05).
func TestRemoteManagement_Accessor(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(WithConfigPaths(dir), WithDefaults(func(v *viper.Viper) {
		v.SetDefault("remote_management.mode", "client")
		v.SetDefault("remote_management.server_url", "wss://hub.example/api/remote/ws")
		v.SetDefault("remote_management.instance_id", "node-xyz")
		v.SetDefault("remote_management.auto_register", false)
		v.SetDefault("remote_management.heartbeat_interval", "45s")
		v.SetDefault("remote_management.bootstrap_secret", "s3cr3t")
		v.SetDefault("remote_management.exposure.flows", "all")
		v.SetDefault("remote_management.exposure.agents", "none")
		v.SetDefault("remote_management.exposure.devices", "all")
		v.SetDefault("remote_management.tls.enabled", false)
	}))
	require.NoError(t, err)

	rm := cfg.RemoteManagement()
	assert.Equal(t, "client", rm.Mode)
	assert.Equal(t, "wss://hub.example/api/remote/ws", rm.ServerURL)
	assert.Equal(t, "node-xyz", rm.InstanceID)
	assert.False(t, rm.AutoRegister)
	assert.Equal(t, 45*time.Second, rm.HeartbeatInterval)
	assert.Equal(t, "s3cr3t", rm.BootstrapSecret)
	assert.Equal(t, "all", rm.Exposure.Flows)
	assert.Equal(t, "none", rm.Exposure.Agents)
	assert.Equal(t, "all", rm.Exposure.Devices)
}

// TestRemoteManagement_ValidationClientRequiresServerURL 는 client 모드에서
// server_url 이 비면 검증 오류가 나는지 확인한다(REQ-REMOTE-A02).
func TestRemoteManagement_ValidationClientRequiresServerURL(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(WithConfigPaths(dir), WithDefaults(func(v *viper.Viper) {
		v.SetDefault("remote_management.mode", "client")
		v.SetDefault("remote_management.server_url", "")
	}))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRequiredField),
		"client 모드 + 빈 server_url 은 ErrRequiredField 여야 함")
}

// TestRemoteManagement_ValidationInvalidMode 는 잘못된 모드 값을 거부하는지
// 검증한다(REQ-REMOTE-A01).
func TestRemoteManagement_ValidationInvalidMode(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(WithConfigPaths(dir), WithDefaults(func(v *viper.Viper) {
		v.SetDefault("remote_management.mode", "bogus")
	}))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidRemoteMode),
		"알 수 없는 모드는 ErrInvalidRemoteMode 여야 함")
}

// TestRemoteManagement_ValidModesAccepted 는 server/client/disabled 가 모두
// 허용되는지 검증한다(client 는 server_url 필요).
func TestRemoteManagement_ValidModesAccepted(t *testing.T) {
	tests := []struct {
		mode      string
		serverURL string
	}{
		{"disabled", ""},
		{"server", ""},
		{"client", "wss://hub/api/remote/ws"},
	}
	for _, tc := range tests {
		t.Run(tc.mode, func(t *testing.T) {
			dir := t.TempDir()
			_, err := Load(WithConfigPaths(dir), WithDefaults(func(v *viper.Viper) {
				v.SetDefault("remote_management.mode", tc.mode)
				v.SetDefault("remote_management.server_url", tc.serverURL)
			}))
			assert.NoError(t, err)
		})
	}
}

// TestRemoteManagement_HeartbeatIntervalFallback 는 미설정 시 안전 기본값으로
// 대체되는지 검증한다(REQ-REMOTE-A05).
func TestRemoteManagement_HeartbeatIntervalFallback(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(WithConfigPaths(dir), WithDefaults(func(v *viper.Viper) {
		v.SetDefault("remote_management.heartbeat_interval", "")
	}))
	require.NoError(t, err)
	rm := cfg.RemoteManagement()
	assert.Equal(t, 30*time.Second, rm.HeartbeatInterval,
		"빈 heartbeat_interval 은 안전 기본값으로 대체되어야 함")
}

// TestRemoteManagement_TLSMissingFilesRejected 는 TLS 활성화 시 cert/key 파일이
// 없으면 검증 오류가 나는지 확인한다(REQ-REMOTE-F01).
func TestRemoteManagement_TLSMissingFilesRejected(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(WithConfigPaths(dir), WithDefaults(func(v *viper.Viper) {
		v.SetDefault("remote_management.mode", "server")
		v.SetDefault("remote_management.tls.enabled", true)
		v.SetDefault("remote_management.tls.cert_file", "/nonexistent/cert.pem")
		v.SetDefault("remote_management.tls.key_file", "/nonexistent/key.pem")
	}))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrFileNotFound),
		"TLS 활성화 + 부재 파일은 ErrFileNotFound 여야 함")
}

// TestRemoteManagement_TLSValidFilesAccepted 는 존재하는 cert/key 파일이면
// 통과하는지 검증한다.
func TestRemoteManagement_TLSValidFilesAccepted(t *testing.T) {
	dir := t.TempDir()
	certPath := dir + "/cert.pem"
	keyPath := dir + "/key.pem"
	require.NoError(t, writeTempFile(certPath))
	require.NoError(t, writeTempFile(keyPath))

	_, err := Load(WithConfigPaths(dir), WithDefaults(func(v *viper.Viper) {
		v.SetDefault("remote_management.mode", "server")
		v.SetDefault("remote_management.tls.enabled", true)
		v.SetDefault("remote_management.tls.cert_file", certPath)
		v.SetDefault("remote_management.tls.key_file", keyPath)
	}))
	assert.NoError(t, err)
}

// TestRemoteManagement_OnChangeFires 는 mode 변경 시 OnChange 콜백이 호출되는지
// 검증한다(REQ-REMOTE-A06 핫리로드 seam).
func TestRemoteManagement_OnChangeFires(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(WithConfigPaths(dir))
	require.NoError(t, err)

	fired := make(chan ChangeEvent, 1)
	unsub := cfg.OnChange("remote_management.mode", func(e ChangeEvent) {
		fired <- e
	})
	defer unsub()

	require.NoError(t, cfg.Set("remote_management.mode", "server"))

	select {
	case e := <-fired:
		assert.Equal(t, "remote_management.mode", e.Key)
		assert.Equal(t, "server", e.NewValue)
	case <-time.After(time.Second):
		t.Fatal("OnChange 콜백이 호출되지 않음")
	}
}

// TestRemoteManagement_SetModeRejectsInvalid 는 런타임 Set 이 잘못된 모드를
// 거부하는지 검증한다(REQ-REMOTE-A01 + A06).
func TestRemoteManagement_SetModeRejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(WithConfigPaths(dir))
	require.NoError(t, err)

	err = cfg.Set("remote_management.mode", "nonsense")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidRemoteMode))
}
