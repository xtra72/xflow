// @SPEC:SPEC-UPDATE-001 v0.1.0
// types_test.go — Phase A 단위 테스트: Version, Channel, ReleaseInfo, UpdateConfig 검증
package updater

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVersion_IsValid 는 semver(v 접두사) 형식 검증을 다룬다.
func TestVersion_IsValid(t *testing.T) {
	tests := []struct {
		name string
		v    Version
		want bool
	}{
		{"valid simple", "v0.4.0", true},
		{"valid larger", "v1.2.3", true},
		{"valid double-digit", "v10.20.30", true},
		{"valid with prerelease", "v1.0.0-beta.2", true},
		{"valid nightly", "v1.0.0-nightly.20260505", true},
		{"missing v prefix", "0.4.0", false},
		{"missing patch", "v0.4", false},
		{"non-numeric", "vfoo", false},
		{"empty", "", false},
		{"plain text", "foo", false},
		{"trailing junk", "v1.2.3xyz", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.v.IsValid(), "IsValid(%q)", tc.v)
		})
	}
}

// TestVersion_String 는 round-trip 보장을 확인한다.
func TestVersion_String(t *testing.T) {
	v := Version("v0.4.0")
	assert.Equal(t, "v0.4.0", v.String())
}

// TestVersion_Compare 는 semver 비교 시맨틱을 검증한다 (string 비교 함정 회피).
func TestVersion_Compare(t *testing.T) {
	tests := []struct {
		name string
		a, b Version
		want int
	}{
		{"equal", "v0.4.0", "v0.4.0", 0},
		{"a less major", "v0.4.0", "v1.0.0", -1},
		{"a greater major", "v1.0.0", "v0.4.0", 1},
		{"a less minor", "v0.3.0", "v0.4.0", -1},
		{"a greater minor", "v0.5.0", "v0.4.0", 1},
		{"a less patch", "v0.4.0", "v0.4.1", -1},
		{"a greater patch", "v0.4.2", "v0.4.1", 1},
		{"string-trap minor", "v0.9.0", "v0.10.0", -1}, // v0.10.0 이 더 큼
		{"string-trap patch", "v1.0.9", "v1.0.10", -1},
		{"large versions", "v1.0.0", "v0.99.99", 1},
		{"prerelease vs release", "v1.0.0-beta.1", "v1.0.0", -1}, // semver: prerelease < release
		{"release vs prerelease", "v1.0.0", "v1.0.0-rc.1", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.a.Compare(tc.b)
			assert.Equal(t, tc.want, got, "Compare(%q, %q)", tc.a, tc.b)
		})
	}
}

// TestVersion_Compare_PrereleaseVsPrerelease 는 양쪽 모두 prerelease 일 때
// lexicographic 비교를 검증한다 (semver 표준의 단순화 형태).
func TestVersion_Compare_PrereleaseVsPrerelease(t *testing.T) {
	tests := []struct {
		name string
		a, b Version
		want int
	}{
		{"alpha < beta", "v1.0.0-alpha.1", "v1.0.0-beta.1", -1},
		{"beta > alpha", "v1.0.0-beta.1", "v1.0.0-alpha.1", 1},
		{"equal prerelease", "v1.0.0-rc.1", "v1.0.0-rc.1", 0},
		{"beta.1 < beta.2", "v1.0.0-beta.1", "v1.0.0-beta.2", -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.a.Compare(tc.b)
			assert.Equal(t, tc.want, got, "Compare(%q, %q)", tc.a, tc.b)
		})
	}
}

// TestVersion_Compare_InvalidVersion 는 잘못된 입력 처리(둘 중 하나라도 invalid)를 검증한다.
func TestVersion_Compare_InvalidVersion(t *testing.T) {
	// 잘못된 버전 비교는 0 (비교 불가) 반환 — SPEC: "release tag가 semver 형식 아니면 비교 실패 → skip"
	got := Version("foo").Compare(Version("v1.0.0"))
	assert.Equal(t, 0, got)
	got = Version("v1.0.0").Compare(Version("foo"))
	assert.Equal(t, 0, got)
	got = Version("foo").Compare(Version("bar"))
	assert.Equal(t, 0, got)
}

// TestChannel_IsValid 는 enum 멤버십을 검증한다.
func TestChannel_IsValid(t *testing.T) {
	tests := []struct {
		c    Channel
		want bool
	}{
		{ChannelStable, true},
		{ChannelBeta, true},
		{ChannelNightly, true},
		{Channel("foo"), false},
		{Channel(""), false},
		{Channel("STABLE"), false}, // case-sensitive
	}
	for _, tc := range tests {
		t.Run(string(tc.c), func(t *testing.T) {
			assert.Equal(t, tc.want, tc.c.IsValid())
		})
	}
}

// TestReleaseInfo_JSON_RoundTrip 는 JSON 직렬화/역직렬화를 검증한다.
func TestReleaseInfo_JSON_RoundTrip(t *testing.T) {
	original := ReleaseInfo{
		Version:     Version("v0.4.0"),
		Channel:     ChannelStable,
		PublishedAt: time.Date(2026, 5, 5, 8, 0, 0, 0, time.UTC),
		Assets: []ReleaseAsset{
			{Name: "xflowd-linux-amd64", DownloadURL: "https://example.com/bin", Size: 25165824},
		},
	}
	data, err := json.Marshal(&original)
	require.NoError(t, err)

	var decoded ReleaseInfo
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, original.Version, decoded.Version)
	assert.Equal(t, original.Channel, decoded.Channel)
	assert.True(t, original.PublishedAt.Equal(decoded.PublishedAt))
	require.Len(t, decoded.Assets, 1)
	assert.Equal(t, "xflowd-linux-amd64", decoded.Assets[0].Name)
	assert.Equal(t, int64(25165824), decoded.Assets[0].Size)
}

// TestManifest_Construct 는 manifest 구조체 기본 생성을 검증한다.
func TestManifest_Construct(t *testing.T) {
	m := Manifest{
		Version:   Version("v0.4.0"),
		SHA256:    "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		Signature: []byte("sig-bytes"),
		BinaryURL: "https://example.com/bin",
	}
	assert.Equal(t, Version("v0.4.0"), m.Version)
	assert.Equal(t, 64, len(m.SHA256)) // SHA256 hex = 64 chars
	assert.NotEmpty(t, m.Signature)
}

// TestAssetName 은 OS/arch 조합 asset 명 생성을 검증한다 (SPEC M3 asset 매칭).
func TestAssetName(t *testing.T) {
	tests := []struct {
		binary, goos, goarch, want string
	}{
		{"xflowd", "linux", "amd64", "xflowd-linux-amd64"},
		{"xflowd", "darwin", "arm64", "xflowd-darwin-arm64"},
		{"xflowd", "linux", "arm64", "xflowd-linux-arm64"},
		{"xflow-agent", "darwin", "amd64", "xflow-agent-darwin-amd64"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := AssetName(tc.binary, tc.goos, tc.goarch)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestUpdateConfig_Defaults 는 안전 기본값(opt-in 정책)을 검증한다.
func TestUpdateConfig_Defaults(t *testing.T) {
	cfg := DefaultConfig()

	// 보안: 기본은 비활성 (운영자 명시적 opt-in 필요)
	assert.False(t, cfg.Enabled, "기본 비활성")

	// 채널: stable
	assert.Equal(t, ChannelStable, cfg.Channel)

	// 주기: 24시간
	assert.Equal(t, 24*time.Hour, cfg.CheckInterval)

	// 자동 적용: false (알림만 기본)
	assert.False(t, cfg.AutoApply)

	// drain timeout: 30초 (SPEC M6)
	assert.Equal(t, 30*time.Second, cfg.DrainTimeout)

	// health check timeout: 5초 (SPEC M6)
	assert.Equal(t, 5*time.Second, cfg.HealthCheckTimeout)

	// 보안: 기본 TLS 검증 활성
	assert.False(t, cfg.InsecureSkipVerify)
}
