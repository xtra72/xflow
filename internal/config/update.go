// @SPEC:SPEC-UPDATE-001 v0.1.0
// update.go — xflowd 자동 업데이트 설정 (config.update.* 섹션).
//
// 본 파일은 운영자가 yaml/env/cli 로 지정하는 update 설정을
// updater.UpdateConfig (런타임 표현) 으로 변환한다.
//
// 디자인 결정:
//   - config 패키지는 updater 를 import 하지만 그 역은 금지 (단방향 의존)
//   - 모든 시간 필드는 time.Duration (yaml: "30s", "24h" 형식)
//   - 안전 기본값: Enabled=false (명시적 opt-in), AutoApply=false, InsecureSkipVerify=false
//   - update_url 은 https:// 강제 (HTTP 입력 시 ToUpdater 가 거부)
//   - channel 은 stable/beta/nightly 3종만 허용 (updater.Channel.IsValid 위임)
package config

import (
	"fmt"
	"net/url"
	"time"

	"github.com/xtra/xflow/internal/updater"
)

// UpdateSettings 는 xflowd config 의 update 섹션을 표현한다.
//
// 이 구조체는 viper 가 디코딩한 raw 값을 보유하며, ToUpdater() 호출 시점에
// updater.UpdateConfig 로 변환되어 검증된다.
type UpdateSettings struct {
	// Enabled 는 자동 업데이트 모듈 자체의 활성/비활성 마스터 스위치 (기본 false).
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Channel 은 stable/beta/nightly 중 하나 (기본 stable).
	Channel string `yaml:"channel" json:"channel"`

	// CheckInterval 은 자동 체크 주기 (기본 24h).
	CheckInterval time.Duration `yaml:"check_interval" json:"check_interval"`

	// AutoApply 는 새 버전 발견 시 즉시 적용 여부 (기본 false → notify only).
	AutoApply bool `yaml:"auto_apply" json:"auto_apply"`

	// NotifyOnly 는 자동 적용 없이 알림만 발행 (AutoApply=false 와 의미 중복; SPEC 정의 보존).
	NotifyOnly bool `yaml:"notify_only" json:"notify_only"`

	// UpdateURL 은 GitHub Releases API 베이스 URL (https:// 강제).
	UpdateURL string `yaml:"update_url" json:"update_url"`

	// PublicKeyPath 는 Ed25519 공개키 파일 경로 (PEM 또는 hex).
	// 빈 문자열이면 빌드 시 임베드된 핀닝 키 사용 (Phase A keys.go 참조).
	PublicKeyPath string `yaml:"public_key_path" json:"public_key_path"`

	// DrainTimeout 은 restart 전 in-flight 요청 처리 대기 시간 (기본 30s).
	DrainTimeout time.Duration `yaml:"drain_timeout" json:"drain_timeout"`

	// HealthCheckTimeout 은 단일 health check HTTP GET 타임아웃 (기본 5s).
	HealthCheckTimeout time.Duration `yaml:"health_check_timeout" json:"health_check_timeout"`

	// InsecureSkipVerify 는 TLS 인증서 검증 건너뛰기 (기본 false; 테스트 전용).
	InsecureSkipVerify bool `yaml:"insecure_skip_verify" json:"insecure_skip_verify"`
}

// DefaultUpdateSettings 는 SPEC M11 기본값으로 UpdateSettings 를 반환한다.
//
// 보안 기본값: Enabled=false (운영자가 명시적으로 활성화해야 동작).
func DefaultUpdateSettings() UpdateSettings {
	return UpdateSettings{
		Enabled:            false,
		Channel:            string(updater.ChannelStable),
		CheckInterval:      24 * time.Hour,
		AutoApply:          false,
		NotifyOnly:         false,
		UpdateURL:          "https://api.github.com/repos/xtra72/xflow",
		PublicKeyPath:      "",
		DrainTimeout:       30 * time.Second,
		HealthCheckTimeout: 5 * time.Second,
		InsecureSkipVerify: false,
	}
}

// ToUpdater 는 UpdateSettings 를 updater.UpdateConfig 로 변환한다.
//
// 검증 항목:
//   - Channel: updater.Channel.IsValid (stable/beta/nightly)
//   - UpdateURL: 비어있지 않으면 url.Parse 통과 + Scheme == "https"
//   - CheckInterval / DrainTimeout / HealthCheckTimeout: 0 이하면 기본값으로 대체
//
// 빈 값/0 값은 DefaultConfig() 의 값으로 메워진다 (partial override 지원).
func (s UpdateSettings) ToUpdater() (updater.UpdateConfig, error) {
	cfg := updater.DefaultConfig()

	cfg.Enabled = s.Enabled
	cfg.AutoApply = s.AutoApply
	cfg.NotifyOnly = s.NotifyOnly
	cfg.InsecureSkipVerify = s.InsecureSkipVerify

	if s.Channel != "" {
		ch := updater.Channel(s.Channel)
		if !ch.IsValid() {
			return updater.UpdateConfig{}, fmt.Errorf("config: invalid update.channel %q (allowed: stable, beta, nightly)", s.Channel)
		}
		cfg.Channel = ch
	}

	if s.UpdateURL != "" {
		u, err := url.Parse(s.UpdateURL)
		if err != nil {
			return updater.UpdateConfig{}, fmt.Errorf("config: parse update.update_url %q: %w", s.UpdateURL, err)
		}
		if u.Scheme != "https" {
			return updater.UpdateConfig{}, fmt.Errorf("config: update.update_url must use https:// scheme; got %q", s.UpdateURL)
		}
		cfg.UpdateURL = s.UpdateURL
	}

	if s.PublicKeyPath != "" {
		cfg.PublicKeyPath = s.PublicKeyPath
	}

	if s.CheckInterval > 0 {
		cfg.CheckInterval = s.CheckInterval
	}
	if s.DrainTimeout > 0 {
		cfg.DrainTimeout = s.DrainTimeout
	}
	if s.HealthCheckTimeout > 0 {
		cfg.HealthCheckTimeout = s.HealthCheckTimeout
	}

	return cfg, nil
}
