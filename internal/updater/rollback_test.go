// @SPEC:SPEC-UPDATE-001 v0.1.0
// rollback_test.go — Phase C 롤백 테스트.
//
// SPEC M7 (롤백) 검증:
//   - 백업 존재 확인 (CanRollback)
//   - 백업 메타데이터 (BackupInfo)
//   - 원자적 복원 (Restore)
//   - 백업 부재 시 ErrUpdateRollbackFailed
package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- helpers ----

// setupBinaryWithBackup 은 main + .previous 백업이 모두 있는 디스크 상태를 시뮬레이션한다.
func setupBinaryWithBackup(t *testing.T) (binPath string, mainContent, backupContent []byte) {
	t.Helper()
	tmpDir := t.TempDir()
	binPath = filepath.Join(tmpDir, "xflowd-test")
	mainContent = []byte("CURRENT-V0.4.0-CONTENT")
	backupContent = []byte("PREVIOUS-V0.3.0-CONTENT")
	require.NoError(t, os.WriteFile(binPath, mainContent, 0o755))
	require.NoError(t, os.WriteFile(binPath+".previous", backupContent, 0o755))
	return binPath, mainContent, backupContent
}

// setupBinaryNoBackup 은 메인 바이너리만 있고 백업이 없는 상태를 시뮬레이션한다.
func setupBinaryNoBackup(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "xflowd-test")
	require.NoError(t, os.WriteFile(binPath, []byte("ONLY-CURRENT"), 0o755))
	return binPath
}

// ---- CanRollback ----

// TestRollback_CanRollback_TrueWhenBackupExists 검증.
func TestRollback_CanRollback_TrueWhenBackupExists(t *testing.T) {
	binPath, _, _ := setupBinaryWithBackup(t)
	r := NewRollback(binPath)
	assert.True(t, r.CanRollback(),
		"CanRollback should return true when .previous exists")
}

// TestRollback_CanRollback_FalseWhenNoBackup 검증.
func TestRollback_CanRollback_FalseWhenNoBackup(t *testing.T) {
	binPath := setupBinaryNoBackup(t)
	r := NewRollback(binPath)
	assert.False(t, r.CanRollback(),
		"CanRollback should return false when no .previous file exists")
}

// TestRollback_CanRollback_FalseWhenBackupIsDir 는 .previous 가 디렉토리면 거부.
func TestRollback_CanRollback_FalseWhenBackupIsDir(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "xflowd-test")
	require.NoError(t, os.WriteFile(binPath, []byte("MAIN"), 0o755))
	// .previous 가 디렉토리 (잘못된 상태)
	require.NoError(t, os.Mkdir(binPath+".previous", 0o755))

	r := NewRollback(binPath)
	assert.False(t, r.CanRollback(),
		"CanRollback should reject directory as backup")
}

// ---- BackupInfo ----

// TestRollback_BackupInfo_ReturnsBasicMetadata 검증.
func TestRollback_BackupInfo_ReturnsBasicMetadata(t *testing.T) {
	binPath, _, backupContent := setupBinaryWithBackup(t)

	r := NewRollback(binPath)
	info, err := r.BackupInfo()
	require.NoError(t, err)
	assert.Equal(t, binPath+".previous", info.Path)
	assert.Equal(t, int64(len(backupContent)), info.Size)
	assert.False(t, info.ModTime.IsZero())
	assert.False(t, info.ModTime.After(time.Now().Add(time.Second)))
}

// TestRollback_BackupInfo_NoBackup_ReturnsErr 검증.
func TestRollback_BackupInfo_NoBackup_ReturnsErr(t *testing.T) {
	binPath := setupBinaryNoBackup(t)
	r := NewRollback(binPath)
	_, err := r.BackupInfo()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateRollbackFailed))
}

// ---- Restore: Happy path ----

// TestRollback_Restore_HappyPath 는 정상 복원 흐름을 검증한다.
func TestRollback_Restore_HappyPath(t *testing.T) {
	binPath, _, backupContent := setupBinaryWithBackup(t)

	r := NewRollback(binPath)
	result, err := r.Restore(context.Background())
	require.NoError(t, err)
	assert.Equal(t, binPath+".previous", result.RestoredFrom)
	assert.Equal(t, binPath, result.RestoredTo)
	assert.False(t, result.RestoredAt.IsZero())

	// 메인 바이너리 내용이 백업 내용으로 교체됨
	got, err := os.ReadFile(binPath)
	require.NoError(t, err)
	assert.Equal(t, backupContent, got,
		"main binary should now contain backup content after restore")
}

// TestRollback_Restore_RemovesBackupAfterSuccess 는 복원 후 백업 정리를 검증한다.
func TestRollback_Restore_RemovesBackupAfterSuccess(t *testing.T) {
	binPath, _, _ := setupBinaryWithBackup(t)

	r := NewRollback(binPath)
	_, err := r.Restore(context.Background())
	require.NoError(t, err)

	// 복원 후 백업 파일은 정리되어야 함 (재롤백 방지)
	_, err = os.Stat(binPath + ".previous")
	assert.True(t, os.IsNotExist(err),
		"backup file should be removed after successful restore")
}

// TestRollback_Restore_PreservesPermissions_0755 검증.
func TestRollback_Restore_PreservesPermissions_0755(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on Windows")
	}
	binPath, _, _ := setupBinaryWithBackup(t)

	r := NewRollback(binPath)
	_, err := r.Restore(context.Background())
	require.NoError(t, err)

	info, err := os.Stat(binPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm(),
		"restored binary should have 0755 permission")
}

// ---- Restore: 에러 ----

// TestRollback_Restore_NoBackup_ErrUpdateRollbackFailed 는 백업 없을 때 명시적 실패.
func TestRollback_Restore_NoBackup_ErrUpdateRollbackFailed(t *testing.T) {
	binPath := setupBinaryNoBackup(t)

	r := NewRollback(binPath)
	_, err := r.Restore(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateRollbackFailed),
		"Restore without backup must return ErrUpdateRollbackFailed")
}

// TestRollback_Restore_BinaryPathNotWritable 는 쓰기 불가 환경 처리.
func TestRollback_Restore_BinaryPathNotWritable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("requires unprivileged unix to test write permission")
	}

	tmpDir := t.TempDir()
	roDir := filepath.Join(tmpDir, "readonly")
	require.NoError(t, os.Mkdir(roDir, 0o755))

	binPath := filepath.Join(roDir, "xflowd-test")
	require.NoError(t, os.WriteFile(binPath, []byte("MAIN"), 0o755))
	require.NoError(t, os.WriteFile(binPath+".previous", []byte("BACKUP"), 0o755))

	// 디렉토리를 read-only 로 전환 → rename 불가
	require.NoError(t, os.Chmod(roDir, 0o555))
	defer func() {
		_ = os.Chmod(roDir, 0o755)
	}()

	r := NewRollback(binPath)
	_, err := r.Restore(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateRollbackFailed))
}

// TestRollback_BackupPath_DerivedFromBinaryPath 는 backup 경로 규칙을 검증한다.
func TestRollback_BackupPath_DerivedFromBinaryPath(t *testing.T) {
	r := NewRollback("/usr/local/bin/xflowd")
	require.NotNil(t, r)

	// 내부 backupPath 가 .previous suffix 를 따르는지 간접 검증:
	// 백업 없는 환경에서 BackupInfo 호출 시 에러 메시지에 .previous 포함.
	_, err := r.BackupInfo()
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".previous")
}

// TestRollback_Restore_ContextCancellation 는 cancelled context fail-fast 를 검증한다.
func TestRollback_Restore_ContextCancellation(t *testing.T) {
	binPath, _, _ := setupBinaryWithBackup(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := NewRollback(binPath)
	_, err := r.Restore(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateRollbackFailed))
}

// TestRollback_BackupInfo_NonRegularFile 는 .previous 가 디렉토리일 때 에러 처리.
func TestRollback_BackupInfo_NonRegularFile(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "xflowd-test")
	require.NoError(t, os.WriteFile(binPath, []byte("MAIN"), 0o755))

	// .previous 를 디렉토리로 생성 → 비정상 상태
	// 단, CanRollback 이 먼저 false 를 반환하므로 BackupInfo 가 stat 단계에서
	// IsRegular() 검사로 실패해야 한다.
	require.NoError(t, os.Mkdir(binPath+".previous", 0o755))

	r := NewRollback(binPath)
	_, err := r.BackupInfo()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateRollbackFailed))
	assert.Contains(t, err.Error(), "not a regular file")
}
