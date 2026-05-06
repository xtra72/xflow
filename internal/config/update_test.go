// @SPEC:SPEC-UPDATE-001 v0.1.0
// update_test.go — UpdateSettings 변환 + viper 통합 테스트.
package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/updater"
)

// TestDefaultUpdateSettings_Values 는 안전 기본값을 검증한다.
//
// SPEC M11: Enabled=false (opt-in), AutoApply=false, InsecureSkipVerify=false.
func TestDefaultUpdateSettings_Values(t *testing.T) {
	s := DefaultUpdateSettings()

	assert.False(t, s.Enabled, "기본값은 비활성 opt-in")
	assert.Equal(t, string(updater.ChannelStable), s.Channel, "기본 채널 stable")
	assert.Equal(t, 24*time.Hour, s.CheckInterval)
	assert.False(t, s.AutoApply)
	assert.False(t, s.NotifyOnly)
	assert.Equal(t, "https://api.github.com/repos/xtra72/xflow", s.UpdateURL)
	assert.Equal(t, "", s.PublicKeyPath)
	assert.Equal(t, 30*time.Second, s.DrainTimeout)
	assert.Equal(t, 5*time.Second, s.HealthCheckTimeout)
	assert.False(t, s.InsecureSkipVerify, "TLS 검증 활성")
}

// TestUpdateSettings_ToUpdater_HappyPath 는 정상 입력의 변환 결과를 검증한다.
func TestUpdateSettings_ToUpdater_HappyPath(t *testing.T) {
	s := UpdateSettings{
		Enabled:            true,
		Channel:            "beta",
		CheckInterval:      6 * time.Hour,
		AutoApply:          true,
		NotifyOnly:         false,
		UpdateURL:          "https://api.github.com/repos/example/xflow",
		PublicKeyPath:      "/etc/xflow/pubkey.pem",
		DrainTimeout:       45 * time.Second,
		HealthCheckTimeout: 10 * time.Second,
		InsecureSkipVerify: false,
	}

	cfg, err := s.ToUpdater()
	require.NoError(t, err)

	assert.True(t, cfg.Enabled)
	assert.Equal(t, updater.ChannelBeta, cfg.Channel)
	assert.Equal(t, 6*time.Hour, cfg.CheckInterval)
	assert.True(t, cfg.AutoApply)
	assert.Equal(t, "https://api.github.com/repos/example/xflow", cfg.UpdateURL)
	assert.Equal(t, "/etc/xflow/pubkey.pem", cfg.PublicKeyPath)
	assert.Equal(t, 45*time.Second, cfg.DrainTimeout)
	assert.Equal(t, 10*time.Second, cfg.HealthCheckTimeout)
	assert.False(t, cfg.InsecureSkipVerify)
}

// TestUpdateSettings_ToUpdater_InvalidChannel 는 enum 외 채널 거부를 검증한다.
func TestUpdateSettings_ToUpdater_InvalidChannel(t *testing.T) {
	s := DefaultUpdateSettings()
	s.Channel = "experimental"

	_, err := s.ToUpdater()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid update.channel")
	assert.Contains(t, err.Error(), "experimental")
}

// TestUpdateSettings_ToUpdater_HTTPSchemeRejected 는 http:// 스킴 거부를 검증한다.
//
// SPEC M1, M13: HTTPS 강제.
func TestUpdateSettings_ToUpdater_HTTPSchemeRejected(t *testing.T) {
	s := DefaultUpdateSettings()
	s.UpdateURL = "http://api.github.com/repos/xtra72/xflow"

	_, err := s.ToUpdater()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "https://")
}

// TestUpdateSettings_ToUpdater_MalformedURL 는 잘못된 URL 거부를 검증한다.
func TestUpdateSettings_ToUpdater_MalformedURL(t *testing.T) {
	s := DefaultUpdateSettings()
	s.UpdateURL = "://not-a-url"

	_, err := s.ToUpdater()
	require.Error(t, err)
}

// TestUpdateSettings_ToUpdater_EmptyChannelUsesDefault 는 빈 채널이 기본값을 유지함을 검증한다.
func TestUpdateSettings_ToUpdater_EmptyChannelUsesDefault(t *testing.T) {
	s := UpdateSettings{Channel: ""}
	cfg, err := s.ToUpdater()
	require.NoError(t, err)
	// updater.DefaultConfig().Channel == ChannelStable
	assert.Equal(t, updater.ChannelStable, cfg.Channel)
}

// TestUpdateSettings_ToUpdater_ZeroDurationsKeepDefaults 는 0 값이 기본값으로 대체됨을 검증한다.
func TestUpdateSettings_ToUpdater_ZeroDurationsKeepDefaults(t *testing.T) {
	s := UpdateSettings{
		Channel:            "stable",
		UpdateURL:          "https://example.com/api",
		CheckInterval:      0,
		DrainTimeout:       0,
		HealthCheckTimeout: 0,
	}
	cfg, err := s.ToUpdater()
	require.NoError(t, err)

	// updater.DefaultConfig() 의 값이 보존되어야 함
	def := updater.DefaultConfig()
	assert.Equal(t, def.CheckInterval, cfg.CheckInterval)
	assert.Equal(t, def.DrainTimeout, cfg.DrainTimeout)
	assert.Equal(t, def.HealthCheckTimeout, cfg.HealthCheckTimeout)
}

// TestConfig_Update_Defaults 는 Load() 가 update 섹션 기본값을 노출함을 검증한다.
func TestConfig_Update_Defaults(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	u := cfg.Update()
	assert.False(t, u.Enabled, "기본값은 비활성")
	assert.Equal(t, "stable", u.Channel)
	assert.Equal(t, 24*time.Hour, u.CheckInterval)
	assert.False(t, u.AutoApply)
	assert.Equal(t, "https://api.github.com/repos/xtra72/xflow", u.UpdateURL)
	assert.Equal(t, 30*time.Second, u.DrainTimeout)
	assert.Equal(t, 5*time.Second, u.HealthCheckTimeout)
	assert.False(t, u.InsecureSkipVerify)
}

// TestConfig_Update_FromYAML_RoundTrip 는 yaml 입력 → UpdateSettings → ToUpdater 흐름을 검증한다.
func TestConfig_Update_FromYAML_RoundTrip(t *testing.T) {
	yaml := `
update:
  enabled: true
  channel: beta
  check_interval: 6h
  auto_apply: true
  notify_only: false
  update_url: https://api.github.com/repos/example/xflow
  public_key_path: /etc/xflow/pubkey.pem
  drain_timeout: 45s
  health_check_timeout: 10s
  insecure_skip_verify: false
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0644))

	cfg, err := Load(WithConfigFile(path))
	require.NoError(t, err)

	u := cfg.Update()
	assert.True(t, u.Enabled)
	assert.Equal(t, "beta", u.Channel)
	assert.Equal(t, 6*time.Hour, u.CheckInterval)
	assert.True(t, u.AutoApply)
	assert.Equal(t, "https://api.github.com/repos/example/xflow", u.UpdateURL)
	assert.Equal(t, "/etc/xflow/pubkey.pem", u.PublicKeyPath)
	assert.Equal(t, 45*time.Second, u.DrainTimeout)
	assert.Equal(t, 10*time.Second, u.HealthCheckTimeout)

	// updater.UpdateConfig 변환 검증
	uc, err := u.ToUpdater()
	require.NoError(t, err)
	assert.Equal(t, updater.ChannelBeta, uc.Channel)
	assert.Equal(t, 6*time.Hour, uc.CheckInterval)
}
