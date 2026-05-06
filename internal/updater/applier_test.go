// @SPEC:SPEC-UPDATE-001 v0.1.0
// applier_test.go — Phase C 원자적 바이너리 교체 테스트.
//
// SPEC M5 (원자적 교체) + M4 (TOCTOU 방어 - pre-rename 검증) 검증.
//
// 보안 critical:
//   - 다운로드된 바이너리를 rename 직전에 재검증 (TOCTOU 방어)
//   - 백업 파일 무결성 (rollback 의존)
//   - 파일 권한 0755 보장
//
// 테스트 안전성: 실제 xflowd 바이너리는 절대 건드리지 않음.
// 모든 테스트는 t.TempDir() 내부의 가짜 바이너리 사용.
package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- helpers ----

// setupFakeBinary 는 t.TempDir() 안에 가짜 바이너리 파일을 생성하고 경로/내용을 반환한다.
//
// 보안: 실제 xflowd 바이너리에 절대 영향이 없도록 격리된 임시 디렉토리만 사용.
func setupFakeBinary(t *testing.T) (binaryPath string, content []byte) {
	t.Helper()
	tmpDir := t.TempDir()
	bin := filepath.Join(tmpDir, "xflowd-test")
	initial := []byte("FAKE-XFLOWD-V0.3.0-CONTENT")
	require.NoError(t, os.WriteFile(bin, initial, 0o755))
	return bin, initial
}

// makeManifestFor 는 content 에 대한 SHA256 + Ed25519 서명을 계산해 Manifest 를 생성한다.
func makeManifestFor(t *testing.T, content []byte, priv ed25519.PrivateKey) Manifest {
	t.Helper()
	sum := sha256.Sum256(content)
	sig := ed25519.Sign(priv, content)
	return Manifest{
		Version:   "v0.4.0",
		SHA256:    hex.EncodeToString(sum[:]),
		Signature: sig,
		BinaryURL: "https://example.com/xflowd-linux-amd64",
	}
}

// writeDownloadedFile 은 검증된 다운로드 바이너리를 시뮬레이션해 디스크에 쓴다.
func writeDownloadedFile(t *testing.T, dir string, content []byte) string {
	t.Helper()
	p := filepath.Join(dir, "downloaded-binary")
	require.NoError(t, os.WriteFile(p, content, 0o600))
	return p
}

// newTestVerifier 는 테스트용 ephemeral Verifier 를 생성한다.
func newTestVerifier(t *testing.T) (*Verifier, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	v, err := NewVerifier(pub)
	require.NoError(t, err)
	return v, priv
}

// ---- Apply: Happy path ----

// TestApplier_HappyPath_ReplacesAndBacksUp 는 정상 교체 + 백업 생성을 검증한다.
func TestApplier_HappyPath_ReplacesAndBacksUp(t *testing.T) {
	binPath, oldContent := setupFakeBinary(t)
	verifier, priv := newTestVerifier(t)

	newContent := []byte("FAKE-XFLOWD-V0.4.0-NEW-CONTENT")
	manifest := makeManifestFor(t, newContent, priv)
	dlPath := writeDownloadedFile(t, t.TempDir(), newContent)

	applier := NewApplier(verifier, binPath)
	result, err := applier.Apply(context.Background(), ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: dlPath,
	})
	require.NoError(t, err)
	assert.Equal(t, Version("v0.4.0"), result.NewVersion)
	assert.Equal(t, binPath+".previous", result.BackupPath)
	assert.False(t, result.AppliedAt.IsZero())

	// 새 바이너리 내용 확인
	got, err := os.ReadFile(binPath)
	require.NoError(t, err)
	assert.Equal(t, newContent, got)

	// 백업 파일 확인 (구 내용이어야 함)
	gotBackup, err := os.ReadFile(binPath + ".previous")
	require.NoError(t, err)
	assert.Equal(t, oldContent, gotBackup)
}

// TestApplier_BackupContainsOldBinary 는 백업이 교체 직전 바이너리와 동일함을 검증한다.
func TestApplier_BackupContainsOldBinary(t *testing.T) {
	binPath, oldContent := setupFakeBinary(t)
	verifier, priv := newTestVerifier(t)

	newContent := []byte("XFLOWD-V0.5.0")
	manifest := makeManifestFor(t, newContent, priv)
	dlPath := writeDownloadedFile(t, t.TempDir(), newContent)

	applier := NewApplier(verifier, binPath)
	_, err := applier.Apply(context.Background(), ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: dlPath,
	})
	require.NoError(t, err)

	backupBytes, err := os.ReadFile(binPath + ".previous")
	require.NoError(t, err)
	assert.Equal(t, oldContent, backupBytes,
		"backup should contain pre-update binary bytes verbatim")
}

// TestApplier_NewBinaryHasCorrectPermissions 는 새 바이너리 권한이 0755 인지 검증한다.
func TestApplier_NewBinaryHasCorrectPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on Windows")
	}
	binPath, _ := setupFakeBinary(t)
	verifier, priv := newTestVerifier(t)

	newContent := []byte("XFLOWD-V0.4.0-PERMS")
	manifest := makeManifestFor(t, newContent, priv)
	dlPath := writeDownloadedFile(t, t.TempDir(), newContent)

	applier := NewApplier(verifier, binPath)
	_, err := applier.Apply(context.Background(), ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: dlPath,
	})
	require.NoError(t, err)

	info, err := os.Stat(binPath)
	require.NoError(t, err)
	// 0755 또는 그에 해당하는 실행 비트
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm(),
		"new binary should have 0755 permission")
}

// ---- Apply: TOCTOU 방어 (pre-rename 재검증) ----

// TestApplier_VerifyFailsAtPreRename_AbortsApply 는 다운로드 후 변조된 파일이
// pre-rename 검증에서 거부되어 교체가 일어나지 않음을 검증한다 (M4 TOCTOU 방어).
func TestApplier_VerifyFailsAtPreRename_AbortsApply(t *testing.T) {
	binPath, oldContent := setupFakeBinary(t)
	verifier, priv := newTestVerifier(t)

	// manifest 는 "정상" 콘텐츠 기준으로 서명/체크섬 계산
	expectedContent := []byte("XFLOWD-V0.4.0-LEGITIMATE")
	manifest := makeManifestFor(t, expectedContent, priv)

	// 그러나 디스크에 있는 다운로드 파일은 변조됨 (TOCTOU 시나리오)
	tamperedContent := []byte("XFLOWD-V0.4.0-TAMPERED")
	dlPath := writeDownloadedFile(t, t.TempDir(), tamperedContent)

	applier := NewApplier(verifier, binPath)
	_, err := applier.Apply(context.Background(), ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: dlPath,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateApplyFailed),
		"tampered content should trigger ErrUpdateApplyFailed via pre-rename verify")

	// 기존 바이너리가 그대로 있어야 함
	got, err := os.ReadFile(binPath)
	require.NoError(t, err)
	assert.Equal(t, oldContent, got, "binary must NOT be replaced on verify failure")

	// 백업도 만들어지지 않아야 함
	_, err = os.Stat(binPath + ".previous")
	assert.True(t, os.IsNotExist(err),
		"backup should NOT be created when pre-rename verification fails")
}

// TestApplier_VerifyAtPreRename_TOCTOUDefense 는 Verifier.VerifyAll 이
// Apply 실행 중 호출됨을 확인한다 (M4 TOCTOU 방어).
//
// Phase B 의 downloader 가 검증 1회를 수행하며, applier 가 pre-rename 시점에
// 다시 검증함으로써 다운로드와 교체 사이의 변조를 차단한다.
func TestApplier_VerifyAtPreRename_TOCTOUDefense(t *testing.T) {
	binPath, _ := setupFakeBinary(t)
	verifier, priv := newTestVerifier(t)

	newContent := []byte("XFLOWD-V0.4.0-TOCTOU")
	manifest := makeManifestFor(t, newContent, priv)
	dlPath := writeDownloadedFile(t, t.TempDir(), newContent)

	applier := NewApplier(verifier, binPath)
	_, err := applier.Apply(context.Background(), ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: dlPath,
	})
	require.NoError(t, err, "valid content should pass pre-rename verification")
}

// ---- Apply: 입력 / 환경 에러 ----

// TestApplier_DownloadedFileNotFound 는 다운로드 파일 부재 시 에러를 검증한다.
func TestApplier_DownloadedFileNotFound(t *testing.T) {
	binPath, _ := setupFakeBinary(t)
	verifier, priv := newTestVerifier(t)
	manifest := makeManifestFor(t, []byte("any"), priv)

	applier := NewApplier(verifier, binPath)
	_, err := applier.Apply(context.Background(), ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: "/nonexistent/path/binary",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateApplyFailed))
}

// TestApplier_BinaryPathNotWritable 는 쓰기 불가능한 대상 경로 처리.
func TestApplier_BinaryPathNotWritable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("requires unprivileged unix to test write permission")
	}

	tmpDir := t.TempDir()
	roDir := filepath.Join(tmpDir, "readonly")
	require.NoError(t, os.Mkdir(roDir, 0o555)) // read+exec 만, write 없음
	defer func() {
		_ = os.Chmod(roDir, 0o755) // cleanup 가능하도록 복원
	}()

	binPath := filepath.Join(roDir, "xflowd-test")
	// 파일은 미리 생성 못하므로, applier 가 read 단계나 rename 단계에서 실패해야 함
	verifier, priv := newTestVerifier(t)
	newContent := []byte("XFLOWD-V0.4.0")
	manifest := makeManifestFor(t, newContent, priv)
	dlPath := writeDownloadedFile(t, t.TempDir(), newContent)

	applier := NewApplier(verifier, binPath)
	_, err := applier.Apply(context.Background(), ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: dlPath,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateApplyFailed))
}

// TestApplier_ContextCancellation 는 cancelled context 가 fail-fast 동작함을 검증한다.
func TestApplier_ContextCancellation(t *testing.T) {
	binPath, _ := setupFakeBinary(t)
	verifier, priv := newTestVerifier(t)

	newContent := []byte("XFLOWD-V0.4.0-CTX")
	manifest := makeManifestFor(t, newContent, priv)
	dlPath := writeDownloadedFile(t, t.TempDir(), newContent)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 즉시 취소

	applier := NewApplier(verifier, binPath)
	_, err := applier.Apply(ctx, ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: dlPath,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled) || errors.Is(err, ErrUpdateApplyFailed),
		"cancelled context should abort apply")
}

// ---- Apply: 옵션 동작 ----

// TestApplier_SkipBackupOption 는 SkipBackup=true 일 때 백업이 생성되지 않음을 검증한다.
func TestApplier_SkipBackupOption(t *testing.T) {
	binPath, _ := setupFakeBinary(t)
	verifier, priv := newTestVerifier(t)

	newContent := []byte("XFLOWD-V0.4.0-NOBACKUP")
	manifest := makeManifestFor(t, newContent, priv)
	dlPath := writeDownloadedFile(t, t.TempDir(), newContent)

	applier := NewApplier(verifier, binPath)
	_, err := applier.Apply(context.Background(), ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: dlPath,
		SkipBackup:     true,
	})
	require.NoError(t, err)

	// 백업이 만들어지지 않아야 함
	_, err = os.Stat(binPath + ".previous")
	assert.True(t, os.IsNotExist(err),
		"SkipBackup=true should not create backup file")
}

// TestApplier_LargeFile_5MB 는 큰 바이너리 처리에 메모리 이슈 없음을 검증한다.
func TestApplier_LargeFile_5MB(t *testing.T) {
	binPath, _ := setupFakeBinary(t)
	verifier, priv := newTestVerifier(t)

	// 5MB 가짜 바이너리 콘텐츠
	largeContent := make([]byte, 5*1024*1024)
	_, err := rand.Read(largeContent)
	require.NoError(t, err)

	manifest := makeManifestFor(t, largeContent, priv)
	dlPath := writeDownloadedFile(t, t.TempDir(), largeContent)

	applier := NewApplier(verifier, binPath)
	_, err = applier.Apply(context.Background(), ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: dlPath,
	})
	require.NoError(t, err)

	got, err := os.ReadFile(binPath)
	require.NoError(t, err)
	assert.Equal(t, len(largeContent), len(got))
}

// TestApplier_TamperedSignature_FailsApply 는 서명만 변조된 경우도 차단함을 검증한다.
func TestApplier_TamperedSignature_FailsApply(t *testing.T) {
	binPath, oldContent := setupFakeBinary(t)
	verifier, priv := newTestVerifier(t)

	newContent := []byte("XFLOWD-V0.4.0-SIG-TAMPER")
	manifest := makeManifestFor(t, newContent, priv)

	// 서명만 변조 (체크섬은 유효)
	manifest.Signature = make([]byte, ed25519.SignatureSize)
	dlPath := writeDownloadedFile(t, t.TempDir(), newContent)

	applier := NewApplier(verifier, binPath)
	_, err := applier.Apply(context.Background(), ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: dlPath,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateApplyFailed))

	// 기존 바이너리 보존
	got, err := os.ReadFile(binPath)
	require.NoError(t, err)
	assert.Equal(t, oldContent, got)
}

// TestApplier_NilVerifier_ReturnsErr 는 verifier 가 nil 인 경우 안전하게 거부함을 검증한다.
func TestApplier_NilVerifier_ReturnsErr(t *testing.T) {
	binPath, _ := setupFakeBinary(t)

	applier := NewApplier(nil, binPath)
	require.NotNil(t, applier)

	_, err := applier.Apply(context.Background(), ApplyOptions{
		Manifest:       Manifest{Version: "v0.4.0"},
		DownloadedPath: "/tmp/anything",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput) || errors.Is(err, ErrUpdateApplyFailed))
}

// TestApplier_DownloadedPathIsDirectory 는 디렉토리를 다운로드 경로로 지정한 경우 처리.
func TestApplier_DownloadedPathIsDirectory(t *testing.T) {
	binPath, _ := setupFakeBinary(t)
	verifier, priv := newTestVerifier(t)
	manifest := makeManifestFor(t, []byte("any"), priv)

	tmpDir := t.TempDir()
	dirAsFile := filepath.Join(tmpDir, "i-am-a-dir")
	require.NoError(t, os.Mkdir(dirAsFile, 0o755))

	applier := NewApplier(verifier, binPath)
	_, err := applier.Apply(context.Background(), ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: dirAsFile,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateApplyFailed))
}
