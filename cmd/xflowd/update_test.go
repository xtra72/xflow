// @SPEC:SPEC-UPDATE-001 v0.1.0
// update_test.go — `xflowd update` 서브커맨드 트리 동작 검증.
//
// 테스트 전략:
//   - cobra.Command 의 SetArgs/SetOut/SetErr 패턴으로 인터랙션 격리
//   - updateDeps 의존성 주입으로 네트워크/디스크/restart 부수효과 차단
//   - 실제 binary 교체 테스트는 t.TempDir() 의 더미 바이너리만 대상으로 함
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/updater"
)

// helperConfigPath 는 update.* 키만 포함하는 임시 yaml 설정 파일을 생성한다.
func helperConfigPath(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "xflowd.yaml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	return p
}

// helperBinaryPath 는 dummy binary 파일 (롤백 테스트용) 을 생성한다.
func helperBinaryPath(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "xflowd")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o755))
	return p
}

// helperBackupBinary 는 .previous 백업 파일을 dummy binary 옆에 생성한다.
func helperBackupBinary(t *testing.T, binaryPath, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(binaryPath+".previous", []byte(content), 0o755))
}

// stubChecker 는 테스트용 Checker 동작을 모사하는 헬퍼이다.
// 실제 *updater.Checker 와 호환되도록 httptest 서버를 띄우는 방식.
func stubChecker(t *testing.T, latestTag string, prerelease bool) *updater.Checker {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assetName := updater.AssetName("xflowd", runtime.GOOS, runtime.GOARCH)
		release := map[string]any{
			"tag_name":     latestTag,
			"name":         latestTag,
			"published_at": time.Now().Format(time.RFC3339),
			"prerelease":   prerelease,
			"html_url":     "https://example.com/release/" + latestTag,
			"assets": []map[string]any{
				{
					"name":                 assetName,
					"browser_download_url": "https://example.com/" + assetName,
					"size":                 1024,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(release)
	}))
	t.Cleanup(server.Close)

	checker, err := updater.NewChecker(server.URL, updater.ChannelStable)
	require.NoError(t, err)
	checker.HTTPClient = server.Client() // TLS 인증서 신뢰
	return checker
}

// --- Help / Tree Tests ---

// TestUpdateCmd_Help_NoArgs 는 인자 없이 호출 시 도움말이 표시되고 exit 0 임을 검증한다.
func TestUpdateCmd_Help_NoArgs(t *testing.T) {
	cmd := newUpdateCmd(defaultUpdateDeps())
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "update")
	assert.Contains(t, out, "check")
	assert.Contains(t, out, "apply")
	assert.Contains(t, out, "rollback")
	assert.Contains(t, out, "channel")
}

// TestUpdateCmd_HasAllSubcommands 는 모든 sub-subcommand 가 등록되어 있음을 검증한다.
func TestUpdateCmd_HasAllSubcommands(t *testing.T) {
	cmd := newUpdateCmd(defaultUpdateDeps())
	subNames := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, want := range []string{"check", "apply", "status", "rollback", "channel"} {
		assert.True(t, subNames[want], "subcommand %q must exist", want)
	}
}

// --- check Tests ---

// TestUpdateCheck_HappyPath 는 새 버전이 발견된 경우 exit 0 + "업데이트 가능" 출력을 검증한다.
func TestUpdateCheck_HappyPath(t *testing.T) {
	cfgPath := helperConfigPath(t, `
update:
  enabled: true
  channel: stable
  update_url: https://example.com/api
`)

	deps := defaultUpdateDeps()
	deps.NewChecker = func(_ string, _ updater.Channel) (*updater.Checker, error) {
		return stubChecker(t, "v9.9.9", false), nil
	}
	deps.CurrentVersion = func() string { return "v0.0.1" }

	cmd := newUpdateCmd(deps)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"check", "--config", cfgPath})

	err := cmd.Execute()
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "v0.0.1")
	assert.Contains(t, out, "v9.9.9")
}

// TestUpdateCheck_NoUpdate 는 동일 버전인 경우에도 exit 0 임을 검증한다.
func TestUpdateCheck_NoUpdate(t *testing.T) {
	cfgPath := helperConfigPath(t, `
update:
  enabled: true
  channel: stable
  update_url: https://example.com/api
`)

	deps := defaultUpdateDeps()
	deps.NewChecker = func(_ string, _ updater.Channel) (*updater.Checker, error) {
		return stubChecker(t, "v0.0.1", false), nil
	}
	deps.CurrentVersion = func() string { return "v0.0.1" }

	cmd := newUpdateCmd(deps)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"check", "--config", cfgPath})

	err := cmd.Execute()
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "v0.0.1")
}

// TestUpdateCheck_InvalidConfig 는 update_url 이 http:// 인 경우 에러를 반환함을 검증한다.
func TestUpdateCheck_InvalidConfig(t *testing.T) {
	cfgPath := helperConfigPath(t, `
update:
  enabled: true
  channel: stable
  update_url: http://insecure.example.com/api
`)

	deps := defaultUpdateDeps()
	cmd := newUpdateCmd(deps)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"check", "--config", cfgPath})

	err := cmd.Execute()
	require.Error(t, err)
}

// --- apply Tests ---

// TestUpdateApply_AlreadyUpToDate 는 동일 버전일 때 적용을 건너뜀을 검증한다.
func TestUpdateApply_AlreadyUpToDate(t *testing.T) {
	cfgPath := helperConfigPath(t, `
update:
  enabled: true
  channel: stable
  update_url: https://example.com/api
`)

	deps := defaultUpdateDeps()
	deps.NewChecker = func(_ string, _ updater.Channel) (*updater.Checker, error) {
		return stubChecker(t, "v0.0.1", false), nil
	}
	deps.CurrentVersion = func() string { return "v0.0.1" }

	cmd := newUpdateCmd(deps)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"apply", "--config", cfgPath, "--yes"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(buf.String()), "up to date")
}

// TestUpdateApply_DowngradeWithoutForce_Refused 는 --force 없는 다운그레이드 거부를 검증한다.
func TestUpdateApply_DowngradeWithoutForce_Refused(t *testing.T) {
	cfgPath := helperConfigPath(t, `
update:
  enabled: true
  channel: stable
  update_url: https://example.com/api
`)

	deps := defaultUpdateDeps()
	deps.NewChecker = func(_ string, _ updater.Channel) (*updater.Checker, error) {
		// 새 버전으로 v0.0.1 만 노출 (현재가 v9.9.9 이므로 자동 다운그레이드)
		return stubChecker(t, "v0.0.1", false), nil
	}
	deps.CurrentVersion = func() string { return "v9.9.9" }

	cmd := newUpdateCmd(deps)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	// 명시적 다운그레이드 시도 (--version v0.0.1 는 현재 v9.9.9 보다 낮음)
	cmd.SetArgs([]string{"apply", "--config", cfgPath, "--version", "v0.0.1", "--yes"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "downgrade")
}

// TestUpdateApply_DowngradeWithForce_Allowed 는 --force 다운그레이드 시 다운로드 단계까지 진입함을 검증한다.
//
// 본 테스트는 실제 바이너리 교체는 시도하지 않는다 (Downloader 가 fake URL 로 5xx 받음).
func TestUpdateApply_DowngradeWithForce_Allowed(t *testing.T) {
	cfgPath := helperConfigPath(t, `
update:
  enabled: true
  channel: stable
  update_url: https://example.com/api
`)

	deps := defaultUpdateDeps()
	deps.NewChecker = func(_ string, _ updater.Channel) (*updater.Checker, error) {
		return stubChecker(t, "v0.0.1", false), nil
	}
	deps.CurrentVersion = func() string { return "v9.9.9" }
	// Apply 단계까지 가지 않도록 binary path 를 test-only sentinel 로 설정
	deps.BinaryPath = func() (string, error) {
		return helperBinaryPath(t, "fake-bin"), nil
	}

	cmd := newUpdateCmd(deps)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"apply", "--config", cfgPath, "--version", "v0.0.1", "--force", "--yes"})

	err := cmd.Execute()
	// --force 가 다운그레이드 게이트는 통과시켰으므로, 후속 다운로드/검증 단계에서는 실패할 수 있다.
	// 핵심은 ErrDowngradeRequiresForce 가 *발생하지 않았다* 는 것.
	if err != nil {
		assert.NotContains(t, err.Error(), "downgrade")
		assert.NotContains(t, err.Error(), "force flag")
	}
}

// --- status Tests ---

// TestUpdateStatus_OutputsJSON 는 status 가 현재 버전과 채널 정보를 JSON 으로 출력함을 검증한다.
func TestUpdateStatus_OutputsJSON(t *testing.T) {
	cfgPath := helperConfigPath(t, `
update:
  enabled: false
  channel: beta
  update_url: https://example.com/api
`)

	deps := defaultUpdateDeps()
	deps.CurrentVersion = func() string { return "v1.2.3" }

	cmd := newUpdateCmd(deps)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"status", "--config", cfgPath, "--json"})

	err := cmd.Execute()
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &payload))
	assert.Equal(t, "v1.2.3", payload["current_version"])
	assert.Equal(t, "beta", payload["channel"])
	assert.Equal(t, false, payload["enabled"])
}

// TestUpdateStatus_TableFormat 는 기본 (table) 형식으로 출력함을 검증한다.
func TestUpdateStatus_TableFormat(t *testing.T) {
	cfgPath := helperConfigPath(t, `
update:
  enabled: true
  channel: stable
  update_url: https://example.com/api
`)

	deps := defaultUpdateDeps()
	deps.CurrentVersion = func() string { return "v1.2.3" }

	cmd := newUpdateCmd(deps)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"status", "--config", cfgPath})

	err := cmd.Execute()
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "v1.2.3")
	assert.Contains(t, out, "stable")
}

// --- rollback Tests ---

// TestUpdateRollback_NoBackup_Exits1 는 백업이 없으면 에러를 반환함을 검증한다.
func TestUpdateRollback_NoBackup_Exits1(t *testing.T) {
	cfgPath := helperConfigPath(t, `update: {enabled: false}`)

	deps := defaultUpdateDeps()
	binPath := helperBinaryPath(t, "current")
	// 백업 파일은 일부러 만들지 않는다.
	deps.BinaryPath = func() (string, error) { return binPath, nil }

	cmd := newUpdateCmd(deps)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"rollback", "--config", cfgPath, "--yes"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "backup")
}

// TestUpdateRollback_WithBackup_Restores 는 백업이 있으면 복원이 성공함을 검증한다.
func TestUpdateRollback_WithBackup_Restores(t *testing.T) {
	cfgPath := helperConfigPath(t, `update: {enabled: false}`)

	binPath := helperBinaryPath(t, "current-version-content")
	helperBackupBinary(t, binPath, "previous-version-content")

	deps := defaultUpdateDeps()
	deps.BinaryPath = func() (string, error) { return binPath, nil }

	cmd := newUpdateCmd(deps)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"rollback", "--config", cfgPath, "--yes"})

	err := cmd.Execute()
	require.NoError(t, err, "rollback output: %s", buf.String())

	// 메인 바이너리가 백업 내용으로 교체되었는지 확인
	restored, err := os.ReadFile(binPath)
	require.NoError(t, err)
	assert.Equal(t, "previous-version-content", string(restored))

	// 백업이 정리되었는지 확인 (M7 loop 방지)
	_, err = os.Stat(binPath + ".previous")
	assert.True(t, os.IsNotExist(err), "백업은 복원 후 삭제되어야 함")
}

// --- channel Tests ---

// TestUpdateChannel_NoArg_PrintsCurrent 는 인자 없이 호출 시 현재 채널을 출력함을 검증한다.
func TestUpdateChannel_NoArg_PrintsCurrent(t *testing.T) {
	cfgPath := helperConfigPath(t, `
update:
  channel: nightly
`)

	deps := defaultUpdateDeps()
	cmd := newUpdateCmd(deps)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"channel", "--config", cfgPath})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "nightly")
}

// TestUpdateChannel_SetBeta_UpdatesConfig 는 새 채널 인자가 설정 파일에 기록됨을 검증한다.
func TestUpdateChannel_SetBeta_UpdatesConfig(t *testing.T) {
	cfgPath := helperConfigPath(t, `
update:
  enabled: true
  channel: stable
  update_url: https://example.com/api
`)

	deps := defaultUpdateDeps()
	cmd := newUpdateCmd(deps)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"channel", "beta", "--config", cfgPath})

	err := cmd.Execute()
	require.NoError(t, err, "stderr: %s", buf.String())

	// 변경된 설정 파일 재로드 검증
	body, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.Contains(t, string(body), "beta")
	assert.NotContains(t, string(body), "channel: stable")
}

// TestUpdateChannel_InvalidName_Rejects 는 enum 외 채널 거부를 검증한다.
func TestUpdateChannel_InvalidName_Rejects(t *testing.T) {
	cfgPath := helperConfigPath(t, `
update:
  channel: stable
`)

	deps := defaultUpdateDeps()
	cmd := newUpdateCmd(deps)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"channel", "experimental", "--config", cfgPath})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "channel")
}

// --- internal helpers (defaults injection roundtrip) ---

// TestDefaultUpdateDeps_NotNil 는 production 의존성 셋이 nil 함수 없이 채워짐을 검증한다.
func TestDefaultUpdateDeps_NotNil(t *testing.T) {
	d := defaultUpdateDeps()
	assert.NotNil(t, d.NewChecker)
	assert.NotNil(t, d.NewDownloader)
	assert.NotNil(t, d.NewVerifier)
	assert.NotNil(t, d.NewApplier)
	assert.NotNil(t, d.NewRollback)
	assert.NotNil(t, d.LoadKey)
	assert.NotNil(t, d.BinaryPath)
	assert.NotNil(t, d.CurrentVersion)
	assert.NotNil(t, d.Now)
	assert.NotNil(t, d.Stdin)
}

// TestConfirm_YesFlag_SkipsPrompt 는 --yes 플래그 시 prompt 가 자동 통과됨을 검증한다.
func TestConfirm_YesFlag_SkipsPrompt(t *testing.T) {
	// Stdin 이 비어 있어도 --yes 가 true 면 confirm() 은 true 반환해야 한다.
	stdinReader := strings.NewReader("")
	yes, err := confirm(io.NopCloser(stdinReader), io.Discard, "테스트?", true)
	require.NoError(t, err)
	assert.True(t, yes)
}

// TestConfirm_NoInput_ReturnsFalse 는 stdin 이 비어 있을 때 (EOF) 거부됨을 검증한다.
func TestConfirm_NoInput_ReturnsFalse(t *testing.T) {
	stdinReader := strings.NewReader("\n")
	yes, err := confirm(io.NopCloser(stdinReader), io.Discard, "테스트?", false)
	require.NoError(t, err)
	assert.False(t, yes)
}

// TestConfirm_YesInput_ReturnsTrue 는 "y\n" 입력 시 true 반환을 검증한다.
func TestConfirm_YesInput_ReturnsTrue(t *testing.T) {
	stdinReader := strings.NewReader("y\n")
	yes, err := confirm(io.NopCloser(stdinReader), io.Discard, "테스트?", false)
	require.NoError(t, err)
	assert.True(t, yes)
}

// --- writeChannelToConfig 검증 (channel 명령 핵심 로직) ---

func TestWriteChannelToConfig_PreservesOtherKeys(t *testing.T) {
	cfgPath := helperConfigPath(t, `
update:
  enabled: true
  channel: stable
  check_interval: 6h
server:
  port: 9999
`)

	require.NoError(t, writeChannelToConfig(cfgPath, "beta"))

	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	body := string(data)

	assert.Contains(t, body, "channel: beta")
	assert.Contains(t, body, "enabled: true")
	assert.Contains(t, body, "check_interval: 6h")
	assert.Contains(t, body, "9999")
}

// --- helper tests ---

func TestReadChecksumFor_HappyPath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "checksum.txt")
	body := `aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  xflowd-linux-amd64
bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  xflowd-darwin-arm64
cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc *xflowd-windows-amd64.exe
`
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))

	hash, err := readChecksumFor(p, "xflowd-darwin-arm64")
	require.NoError(t, err)
	assert.Equal(t, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", hash)

	// "*" prefix (binary mode in sha256sum) 처리 검증
	hash2, err := readChecksumFor(p, "xflowd-windows-amd64.exe")
	require.NoError(t, err)
	assert.Equal(t, "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", hash2)
}

func TestReadChecksumFor_NotFound(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "checksum.txt")
	require.NoError(t, os.WriteFile(p, []byte("aaaa  other-binary\n"), 0o600))

	_, err := readChecksumFor(p, "xflowd-linux-amd64")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestReadChecksumFor_FileMissing(t *testing.T) {
	_, err := readChecksumFor("/nonexistent/checksum.txt", "x")
	require.Error(t, err)
}

func TestDecodeSignature_HexInput(t *testing.T) {
	// 64-byte raw → 128-char hex 인코딩
	raw := bytes.Repeat([]byte{0xAB}, 64)
	hexStr := []byte("abababababababababababababababababababababababababababababababab" +
		"abababababababababababababababababababababababababababababababab")

	out := decodeSignature(hexStr)
	assert.Equal(t, raw, out)
}

func TestDecodeSignature_RawInput(t *testing.T) {
	raw := bytes.Repeat([]byte{0xCD}, 64)
	out := decodeSignature(raw)
	assert.Equal(t, raw, out, "raw bytes should pass through unchanged")
}

func TestWriteChannelToConfig_CreatesUpdateSection(t *testing.T) {
	cfgPath := helperConfigPath(t, `
server:
  port: 8080
`)

	require.NoError(t, writeChannelToConfig(cfgPath, "nightly"))

	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	body := string(data)

	assert.Contains(t, body, "channel: nightly")
}
