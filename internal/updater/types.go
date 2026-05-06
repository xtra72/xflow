// Package updater 는 SPEC-UPDATE-001 자동 업데이트 메커니즘을 구현한다.
//
// 보안 critical: HTTPS 전용, Ed25519 서명 검증 + SHA256 체크섬, 다운그레이드 방지,
// 공개키 핀닝 (빌드 변수로 임베드).
//
// 본 패키지는 의존성 없이 self-contained 로 설계되어 xflowd / xflow-agent / xflow CLI
// 모두에서 재사용 가능하다.
//
// @SPEC:SPEC-UPDATE-001 v0.1.0
package updater

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Channel 은 업데이트 채널을 나타내는 enum 이다.
//
// SPEC M1: 허용 값은 stable, beta, nightly 만이며, 기본값은 stable.
type Channel string

const (
	// ChannelStable 은 안정 릴리즈 채널 (releases/latest 사용).
	ChannelStable Channel = "stable"
	// ChannelBeta 는 베타 prerelease 채널.
	ChannelBeta Channel = "beta"
	// ChannelNightly 는 nightly prerelease 채널.
	ChannelNightly Channel = "nightly"
)

// IsValid 는 채널 값이 허용된 enum 멤버인지 확인한다.
func (c Channel) IsValid() bool {
	switch c {
	case ChannelStable, ChannelBeta, ChannelNightly:
		return true
	default:
		return false
	}
}

// Version 은 semver 형식 (v 접두사 필수) 의 버전 문자열이다.
//
// 형식: vMAJOR.MINOR.PATCH 또는 vMAJOR.MINOR.PATCH-PRERELEASE
// 예: v0.4.0, v1.0.0-beta.2, v1.0.0-nightly.20260505
type Version string

// versionRegex 는 v 접두사 + 3-part numeric semver + optional prerelease 패턴을 매칭한다.
// prerelease 부분은 영문/숫자/점/하이픈 허용.
var versionRegex = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)(?:-([\w][\w.-]*))?$`)

// IsValid 는 버전 문자열이 SPEC 형식 (v 접두사 + 3-part semver) 을 따르는지 확인한다.
func (v Version) IsValid() bool {
	return versionRegex.MatchString(string(v))
}

// String 은 Version 의 문자열 표현을 반환한다 (round-trip 보장).
func (v Version) String() string {
	return string(v)
}

// Compare 는 semver 시맨틱으로 두 버전을 비교한다.
//
// 반환값:
//   - -1: v < other
//   - 0: v == other (또는 둘 중 하나라도 invalid)
//   - +1: v > other
//
// 비교 규칙 (semver 표준):
//  1. major.minor.patch 를 숫자로 비교
//  2. 동일하면 prerelease 비교: prerelease 있는 쪽이 작음
//  3. 둘 다 prerelease 면 lexicographic 비교
//
// SPEC: "release tag 가 semver 형식 아니면 비교 실패 → skip" → invalid 입력은 0 반환.
func (v Version) Compare(other Version) int {
	a := versionRegex.FindStringSubmatch(string(v))
	b := versionRegex.FindStringSubmatch(string(other))
	if a == nil || b == nil {
		return 0 // 비교 불가
	}

	// major / minor / patch 숫자 비교
	for i := 1; i <= 3; i++ {
		ai, _ := strconv.Atoi(a[i])
		bi, _ := strconv.Atoi(b[i])
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}

	// numeric 부분 동일 → prerelease 비교
	aPre, bPre := a[4], b[4]
	switch {
	case aPre == "" && bPre == "":
		return 0
	case aPre == "" && bPre != "":
		// release > prerelease (semver: 1.0.0 > 1.0.0-beta)
		return 1
	case aPre != "" && bPre == "":
		return -1
	default:
		// 둘 다 prerelease → 단순 lexicographic
		// (정밀한 semver 비교는 추후 확장; SPEC M2 는 채널별 정렬에 충분)
		if aPre < bPre {
			return -1
		}
		if aPre > bPre {
			return 1
		}
		return 0
	}
}

// ReleaseAsset 은 release 에 첨부된 단일 바이너리 asset 정보이다.
type ReleaseAsset struct {
	// Name 은 asset 파일명 (예: xflowd-linux-amd64).
	Name string `json:"name"`
	// DownloadURL 은 HTTPS 다운로드 URL.
	DownloadURL string `json:"download_url"`
	// Size 는 바이트 단위 파일 크기 (사전 디스크 검사 + 진행률 계산용).
	Size int64 `json:"size"`
}

// ReleaseInfo 는 채널에서 가져온 단일 release 메타데이터이다.
//
// SPEC M2: GitHub Releases API 응답을 정규화한 형태.
type ReleaseInfo struct {
	Version     Version        `json:"version"`
	Channel     Channel        `json:"channel"`
	PublishedAt time.Time      `json:"published_at"`
	Assets      []ReleaseAsset `json:"assets"`
	// ReleaseNotesURL 은 운영자가 변경사항을 확인할 GitHub release 페이지 URL (M2 응답에 포함).
	ReleaseNotesURL string `json:"release_notes_url,omitempty"`
}

// Manifest 는 다운로드 + 검증에 필요한 메타데이터의 집합이다.
//
// SPEC M3-M4: 검증 단계에서 SHA256 + Ed25519 서명을 사용한다.
type Manifest struct {
	// Version 은 대상 릴리즈 버전.
	Version Version `json:"version"`
	// SHA256 은 hex 인코딩된 64자 SHA256 체크섬.
	SHA256 string `json:"sha256"`
	// Signature 는 64-byte Ed25519 raw signature.
	Signature []byte `json:"signature"`
	// BinaryURL 은 바이너리 자체의 HTTPS 다운로드 URL.
	BinaryURL string `json:"binary_url"`
}

// UpdateConfig 는 운영자 yaml 설정 (SPEC M11).
//
// 안전 기본값: Enabled=false (명시적 opt-in), AutoApply=false, InsecureSkipVerify=false.
type UpdateConfig struct {
	Enabled            bool          `yaml:"enabled" json:"enabled"`
	Channel            Channel       `yaml:"channel" json:"channel"`
	CheckInterval      time.Duration `yaml:"check_interval" json:"check_interval"`
	AutoApply          bool          `yaml:"auto_apply" json:"auto_apply"`
	NotifyOnly         bool          `yaml:"notify_only" json:"notify_only"`
	UpdateURL          string        `yaml:"update_url" json:"update_url"`
	PublicKeyPath      string        `yaml:"public_key_path" json:"public_key_path"`
	DrainTimeout       time.Duration `yaml:"drain_timeout" json:"drain_timeout"`
	HealthCheckTimeout time.Duration `yaml:"health_check_timeout" json:"health_check_timeout"`
	InsecureSkipVerify bool          `yaml:"insecure_skip_verify" json:"insecure_skip_verify"`
}

// DefaultConfig 는 SPEC M11 기본값을 반환한다.
//
// 보안: Enabled=false 가 기본 (운영자가 명시적으로 활성화해야 자동 동작 시작).
func DefaultConfig() UpdateConfig {
	return UpdateConfig{
		Enabled:            false, // 명시적 opt-in 정책
		Channel:            ChannelStable,
		CheckInterval:      24 * time.Hour,
		AutoApply:          false, // 알림만이 기본
		NotifyOnly:         false,
		DrainTimeout:       30 * time.Second,
		HealthCheckTimeout: 5 * time.Second,
		InsecureSkipVerify: false, // TLS 검증 활성
	}
}

// AssetName 은 OS/arch 조합으로 표준 asset 파일명을 생성한다.
//
// 예: AssetName("xflowd", "linux", "amd64") = "xflowd-linux-amd64"
//
// SPEC M3 asset 매칭에 사용.
func AssetName(binary, goos, goarch string) string {
	var b strings.Builder
	b.WriteString(binary)
	b.WriteByte('-')
	b.WriteString(goos)
	b.WriteByte('-')
	b.WriteString(goarch)
	return b.String()
}
