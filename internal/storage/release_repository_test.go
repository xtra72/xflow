// release_repository_test.go 는 서버 호스팅 프로그램 이미지 저장소를 검증한다.
package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestReleaseRepo 는 임시 SQLite DB + releases 디렉토리로 저장소를 만든다.
func newTestReleaseRepo(t *testing.T) (*ReleaseRepository, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "xflow.db")
	db, err := OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	releasesDir := filepath.Join(dir, "releases")
	repo, err := NewReleaseRepository(db, releasesDir)
	require.NoError(t, err)
	return repo, releasesDir
}

// putBinary 는 (version,os,arch) 에 주어진 바이너리/서명 바이트로 asset 을 올린다.
func putBinary(t *testing.T, repo *ReleaseRepository, version, goos, arch string, bin, sig []byte) ReleaseAssetRecord {
	t.Helper()
	var sigReader io.Reader
	if sig != nil {
		sigReader = strings.NewReader(string(sig))
	}
	rec, err := repo.PutAsset(context.Background(), version, goos, arch,
		strings.NewReader(string(bin)), sigReader, 1000)
	require.NoError(t, err)
	return rec
}

func TestUpsertRelease_ValidatesSemverAndChannel(t *testing.T) {
	repo, _ := newTestReleaseRepo(t)
	ctx := context.Background()

	// 유효 semver + 채널.
	require.NoError(t, repo.UpsertRelease(ctx, "v1.2.3", "stable", "first", 1000))

	// 비-semver 거부.
	err := repo.UpsertRelease(ctx, "1.2.3", "stable", "", 1000)
	require.ErrorIs(t, err, ErrInvalidReleaseVersion)

	// 잘못된 채널 거부.
	err = repo.UpsertRelease(ctx, "v1.2.4", "edge", "", 1000)
	require.ErrorIs(t, err, ErrInvalidReleaseChannel)

	// 빈 채널은 stable 로 기본 처리.
	require.NoError(t, repo.UpsertRelease(ctx, "v1.2.5", "", "", 1000))
	rec, err := repo.GetRelease(ctx, "v1.2.5")
	require.NoError(t, err)
	assert.Equal(t, "stable", rec.Channel)
}

func TestPutAsset_ComputesSHA256AndStoresFiles(t *testing.T) {
	repo, releasesDir := newTestReleaseRepo(t)

	bin := []byte("fake-xflowd-binary-bytes")
	sig := []byte("ed25519-signature-bytes")
	rec := putBinary(t, repo, "v1.0.0", "linux", "amd64", bin, sig)

	// SHA256 정확성.
	want := sha256.Sum256(bin)
	assert.Equal(t, hex.EncodeToString(want[:]), rec.SHA256)
	assert.Equal(t, int64(len(bin)), rec.Size)
	assert.True(t, rec.HasSig)
	assert.Equal(t, "xflowd-linux-amd64", rec.Filename)

	// 디스크에 바이너리 + .sig 가 0600 으로 기록되었는지.
	binPath := filepath.Join(releasesDir, "v1.0.0", "xflowd-linux-amd64")
	sigPath := binPath + ".sig"
	gotBin, err := os.ReadFile(binPath)
	require.NoError(t, err)
	assert.Equal(t, bin, gotBin)
	gotSig, err := os.ReadFile(sigPath)
	require.NoError(t, err)
	assert.Equal(t, sig, gotSig)

	info, err := os.Stat(binPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestPutAsset_AutoCreatesReleaseRow(t *testing.T) {
	repo, _ := newTestReleaseRepo(t)
	ctx := context.Background()

	// 릴리즈 행을 먼저 만들지 않고 asset 만 올린다.
	putBinary(t, repo, "v2.0.0", "linux", "arm64", []byte("bin"), []byte("sig"))

	rec, err := repo.GetRelease(ctx, "v2.0.0")
	require.NoError(t, err)
	assert.Equal(t, "stable", rec.Channel)
	require.Len(t, rec.Assets, 1)
	assert.Equal(t, "arm64", rec.Assets[0].Arch)
}

// TestLatestVersionByArch_NewerVersionMissingArch 는 신규 버전이 일부 아키텍처를 누락하면
// 그 슬롯은 더 낮은(이전) 버전이 차지하고, 채널 필터·빈 저장소 경계를 검증한다.
func TestLatestVersionByArch_NewerVersionMissingArch(t *testing.T) {
	repo, _ := newTestReleaseRepo(t)
	ctx := context.Background()

	// v1.0.0(stable): amd64 + arm64 둘 다 보유.
	putBinary(t, repo, "v1.0.0", "linux", "amd64", []byte("a1"), nil)
	putBinary(t, repo, "v1.0.0", "linux", "arm64", []byte("a2"), nil)
	// v2.0.0(stable): amd64 만 보유(arm64 누락).
	putBinary(t, repo, "v2.0.0", "linux", "amd64", []byte("b1"), nil)
	// v1.5.0(beta): arm 만 보유(채널 필터 검증용).
	require.NoError(t, repo.UpsertRelease(ctx, "v1.5.0", "beta", "", 1000))
	putBinary(t, repo, "v1.5.0", "linux", "arm", []byte("c1"), nil)

	// stable 채널: amd64 는 최신 v2.0.0, arm64 는 v2.0.0 누락이라 v1.0.0 으로 폴백.
	// beta-only arm 슬롯은 stable 필터에서 제외된다.
	stable, err := repo.LatestVersionByArch(ctx, "stable")
	require.NoError(t, err)
	assert.Equal(t, "v2.0.0", stable["linux/amd64"])
	assert.Equal(t, "v1.0.0", stable["linux/arm64"])
	_, hasArm := stable["linux/arm"]
	assert.False(t, hasArm, "beta-only arm 슬롯은 stable 필터에서 제외")

	// 전 채널(빈 channel): beta arm 슬롯도 포함된다.
	all, err := repo.LatestVersionByArch(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, "v2.0.0", all["linux/amd64"])
	assert.Equal(t, "v1.0.0", all["linux/arm64"])
	assert.Equal(t, "v1.5.0", all["linux/arm"])
}

// TestLatestVersionByArch_EmptyStore 는 빈 저장소에서 빈 맵을 반환하는지 검증한다.
func TestLatestVersionByArch_EmptyStore(t *testing.T) {
	repo, _ := newTestReleaseRepo(t)
	m, err := repo.LatestVersionByArch(context.Background(), "stable")
	require.NoError(t, err)
	assert.Empty(t, m)
}

func TestListReleases_SortedSemverDesc(t *testing.T) {
	repo, _ := newTestReleaseRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.UpsertRelease(ctx, "v1.0.0", "stable", "", 1000))
	require.NoError(t, repo.UpsertRelease(ctx, "v1.2.0", "stable", "", 1000))
	require.NoError(t, repo.UpsertRelease(ctx, "v1.10.0", "beta", "", 1000))
	require.NoError(t, repo.UpsertRelease(ctx, "v1.1.5", "stable", "", 1000))

	recs, err := repo.ListReleases(ctx)
	require.NoError(t, err)
	got := make([]string, len(recs))
	for i, r := range recs {
		got[i] = r.Version
	}
	// semver 내림차순: 1.10.0 > 1.2.0 > 1.1.5 > 1.0.0.
	assert.Equal(t, []string{"v1.10.0", "v1.2.0", "v1.1.5", "v1.0.0"}, got)
}

func TestResolveLatestStable_IgnoresPrerelease(t *testing.T) {
	repo, _ := newTestReleaseRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.UpsertRelease(ctx, "v1.0.0", "stable", "", 1000))
	require.NoError(t, repo.UpsertRelease(ctx, "v2.0.0-beta.1", "beta", "", 1000))
	require.NoError(t, repo.UpsertRelease(ctx, "v1.5.0", "stable", "", 1000))

	rec, ok, err := repo.ResolveLatestStable(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	// 최고 stable 은 1.5.0 (2.0.0-beta 는 beta 채널이므로 제외).
	assert.Equal(t, "v1.5.0", rec.Version)

	// stable 이 전혀 없으면 ok=false.
	repo2, _ := newTestReleaseRepo(t)
	require.NoError(t, repo2.UpsertRelease(ctx, "v3.0.0-nightly.1", "nightly", "", 1000))
	_, ok2, err := repo2.ResolveLatestStable(ctx)
	require.NoError(t, err)
	assert.False(t, ok2)
}

func TestOpenAsset_StreamsBytesAndRejectsTraversal(t *testing.T) {
	repo, _ := newTestReleaseRepo(t)
	ctx := context.Background()

	bin := []byte("streamed-content")
	putBinary(t, repo, "v1.0.0", "linux", "amd64", bin, []byte("sig"))

	// 바이너리 스트리밍.
	rc, size, err := repo.OpenAsset(ctx, "v1.0.0", "xflowd-linux-amd64")
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()
	assert.Equal(t, int64(len(bin)), size)
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, bin, got)

	// .sig 스트리밍.
	rcSig, _, err := repo.OpenAsset(ctx, "v1.0.0", "xflowd-linux-amd64.sig")
	require.NoError(t, err)
	_ = rcSig.Close()

	// 경로 순회 거부.
	for _, bad := range []string{"../secret", "a/b", "..", "foo/../bar"} {
		_, _, perr := repo.OpenAsset(ctx, "v1.0.0", bad)
		require.ErrorIs(t, perr, ErrInvalidAssetFilename, "filename %q 는 거부되어야 함", bad)
	}
	// version 의 경로 순회도 거부.
	_, _, verr := repo.OpenAsset(ctx, "../etc", "xflowd-linux-amd64")
	require.ErrorIs(t, verr, ErrInvalidAssetFilename)

	// 미존재 파일.
	_, _, nerr := repo.OpenAsset(ctx, "v1.0.0", "xflowd-windows-amd64")
	require.ErrorIs(t, nerr, ErrReleaseAssetNotFound)
}

func TestChecksumFile_BinariesOnlyDeterministic(t *testing.T) {
	repo, _ := newTestReleaseRepo(t)
	ctx := context.Background()

	binAmd := []byte("amd64-binary")
	binArm := []byte("arm64-binary")
	putBinary(t, repo, "v1.0.0", "linux", "amd64", binAmd, []byte("sig1"))
	putBinary(t, repo, "v1.0.0", "linux", "arm64", binArm, []byte("sig2"))

	data, err := repo.ChecksumFile(ctx, "v1.0.0")
	require.NoError(t, err)

	sumAmd := sha256.Sum256(binAmd)
	sumArm := sha256.Sum256(binArm)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	require.Len(t, lines, 2, "바이너리 2개만 — .sig 라인 미포함")

	// 각 라인은 `{hex}  {filename}` (2칸 공백) 형식이고 .sig 는 포함하지 않는다.
	joined := string(data)
	assert.Contains(t, joined, hex.EncodeToString(sumAmd[:])+"  xflowd-linux-amd64\n")
	assert.Contains(t, joined, hex.EncodeToString(sumArm[:])+"  xflowd-linux-arm64\n")
	assert.NotContains(t, joined, ".sig")

	// 결정적: filename 오름차순(amd64 < arm64).
	assert.True(t, strings.Index(joined, "amd64") < strings.Index(joined, "arm64"))

	// 미존재 버전.
	_, nerr := repo.ChecksumFile(ctx, "v9.9.9")
	require.ErrorIs(t, nerr, ErrReleaseNotFound)
}

func TestDeleteAsset_RemovesRowAndFiles(t *testing.T) {
	repo, releasesDir := newTestReleaseRepo(t)
	ctx := context.Background()

	putBinary(t, repo, "v1.0.0", "linux", "amd64", []byte("bin"), []byte("sig"))
	binPath := filepath.Join(releasesDir, "v1.0.0", "xflowd-linux-amd64")
	require.FileExists(t, binPath)

	require.NoError(t, repo.DeleteAsset(ctx, "v1.0.0", "linux", "amd64"))

	// 파일이 사라지고, asset 행도 사라졌는지(릴리즈 행은 유지).
	assert.NoFileExists(t, binPath)
	assert.NoFileExists(t, binPath+".sig")
	rec, err := repo.GetRelease(ctx, "v1.0.0")
	require.NoError(t, err)
	assert.Empty(t, rec.Assets)

	// 멱등: 재삭제 무해.
	require.NoError(t, repo.DeleteAsset(ctx, "v1.0.0", "linux", "amd64"))
}

func TestDeleteRelease_RemovesRowsAndDir(t *testing.T) {
	repo, releasesDir := newTestReleaseRepo(t)
	ctx := context.Background()

	putBinary(t, repo, "v1.0.0", "linux", "amd64", []byte("bin"), []byte("sig"))
	putBinary(t, repo, "v1.0.0", "linux", "arm64", []byte("bin2"), []byte("sig2"))
	versionDir := filepath.Join(releasesDir, "v1.0.0")
	require.DirExists(t, versionDir)

	require.NoError(t, repo.DeleteRelease(ctx, "v1.0.0"))

	assert.NoDirExists(t, versionDir)
	_, err := repo.GetRelease(ctx, "v1.0.0")
	require.ErrorIs(t, err, ErrReleaseNotFound)

	// 멱등: 미존재 삭제 무해.
	require.NoError(t, repo.DeleteRelease(ctx, "v1.0.0"))
}

func TestPutAsset_WithoutSignature(t *testing.T) {
	repo, releasesDir := newTestReleaseRepo(t)

	rec := putBinary(t, repo, "v1.0.0", "darwin", "arm64", []byte("bin"), nil)
	assert.False(t, rec.HasSig)
	assert.NoFileExists(t, filepath.Join(releasesDir, "v1.0.0", "xflowd-darwin-arm64.sig"))
}
