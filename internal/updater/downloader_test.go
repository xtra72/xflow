// @SPEC:SPEC-UPDATE-001 v0.1.0
// downloader_test.go — Phase B 단위 테스트: HTTPS 다운로더 + 진행률 + 디스크 검사.
//
// 테스트 전략:
//   - httptest.NewTLSServer 로 HTTPS 모방
//   - diskInspector 인터페이스 모킹으로 디스크 부족 시나리오 결정론적 검증
//   - HTTPS 강제 (http:// asset URL 거부, M1/M13)
//   - 임시 파일 0600 권한 + 원자적 rename + 진행률 콜백 검증
package updater

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
)

// fakeDiskInspector 는 결정론적 디스크 가용량을 반환한다 (테스트 전용).
type fakeDiskInspector struct {
	avail uint64
	err   error
}

func (f *fakeDiskInspector) AvailableBytes(_ string) (uint64, error) {
	return f.avail, f.err
}

// helperRandomBytes 는 결정론적 SHA256 검증을 위해 임의 바이트를 생성한다.
func helperRandomBytes(t *testing.T, n int) []byte {
	t.Helper()
	buf := make([]byte, n)
	_, err := rand.Read(buf)
	require.NoError(t, err)
	return buf
}

// TestNewDownloader_Default 은 기본 Downloader 생성 (실제 디스크 inspector 사용).
func TestNewDownloader_Default(t *testing.T) {
	d := NewDownloader()
	require.NotNil(t, d)
	require.NotNil(t, d.HTTPClient)
}

// TestDownloader_HappyPath_DownloadsToDest 는 정상 다운로드 흐름을 검증.
func TestDownloader_HappyPath_DownloadsToDest(t *testing.T) {
	payload := helperRandomBytes(t, 4096)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", intToStr(int64(len(payload))))
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	d := NewDownloader()
	d.HTTPClient = srv.Client()
	d.disk = &fakeDiskInspector{avail: 1 << 30} // 1GB 여유

	dest := filepath.Join(t.TempDir(), "binary")
	asset := ReleaseAsset{
		Name:        "xflowd-linux-amd64",
		DownloadURL: srv.URL + "/binary",
		Size:        int64(len(payload)),
	}

	err := d.Download(context.Background(), asset, dest, nil)
	require.NoError(t, err)

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, payload, got)
}

// TestDownloader_HTTPSchemeRejected 는 http:// asset URL 을 거부한다 (M1, M13).
func TestDownloader_HTTPSchemeRejected(t *testing.T) {
	d := NewDownloader()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	asset := ReleaseAsset{
		Name:        "xflowd-linux-amd64",
		DownloadURL: "http://example.com/binary",
		Size:        100,
	}
	err := d.Download(context.Background(), asset, filepath.Join(t.TempDir(), "out"), nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateChannelInvalid),
		"expected ErrUpdateChannelInvalid for http:// URL, got %v", err)
}

// TestDownloader_MalformedURL_Rejected 는 파싱 실패 URL 을 거부.
func TestDownloader_MalformedURL_Rejected(t *testing.T) {
	d := NewDownloader()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	asset := ReleaseAsset{
		Name:        "xflowd-linux-amd64",
		DownloadURL: "://broken",
		Size:        100,
	}
	err := d.Download(context.Background(), asset, filepath.Join(t.TempDir(), "out"), nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateChannelInvalid))
}

// TestDownloader_DiskSpaceCheck_Insufficient 는 사전 디스크 검사 실패를 검증 (M3 / Scenario 6).
func TestDownloader_DiskSpaceCheck_Insufficient(t *testing.T) {
	d := NewDownloader()
	// asset.Size 의 3배 미만으로 가용량 설정 (SPEC: 백업+새+여유 = 3배)
	d.disk = &fakeDiskInspector{avail: 1024} // 1KB only

	asset := ReleaseAsset{
		Name:        "xflowd-linux-amd64",
		DownloadURL: "https://example.com/binary",
		Size:        10 * 1024 * 1024, // 10MB
	}
	err := d.Download(context.Background(), asset, filepath.Join(t.TempDir(), "out"), nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInsufficientDiskSpace))
}

// TestDownloader_DiskInspectorError_Wrapped 는 inspector 자체 에러를 ErrUpdate.. 에 wrapping.
func TestDownloader_DiskInspectorError_Wrapped(t *testing.T) {
	d := NewDownloader()
	d.disk = &fakeDiskInspector{err: errors.New("statfs failed")}

	asset := ReleaseAsset{
		Name:        "xflowd-linux-amd64",
		DownloadURL: "https://example.com/binary",
		Size:        100,
	}
	err := d.Download(context.Background(), asset, filepath.Join(t.TempDir(), "out"), nil)
	require.Error(t, err)
	// inspector 실패 → ErrUpdateInsufficientDiskSpace 로 보수적 처리
	assert.True(t, errors.Is(err, ErrUpdateInsufficientDiskSpace))
}

// TestDownloader_ProgressCallback_Invoked 는 진행률 콜백이 호출됨을 검증.
func TestDownloader_ProgressCallback_Invoked(t *testing.T) {
	payload := helperRandomBytes(t, 256*1024) // 256KB → 64KB 마다 호출 = 4+회
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", intToStr(int64(len(payload))))
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	d := NewDownloader()
	d.HTTPClient = srv.Client()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	dest := filepath.Join(t.TempDir(), "binary")
	asset := ReleaseAsset{DownloadURL: srv.URL + "/bin", Size: int64(len(payload))}

	calls := 0
	var lastDownloaded, lastTotal int64
	err := d.Download(context.Background(), asset, dest, func(downloaded, total int64) {
		calls++
		lastDownloaded = downloaded
		lastTotal = total
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, calls, 2, "progress callback should fire at least twice")
	assert.Equal(t, int64(len(payload)), lastDownloaded, "final downloaded must equal total")
	assert.Equal(t, int64(len(payload)), lastTotal)
}

// TestDownloader_FilePermissions_0600 는 다운로드된 파일 권한을 검증 (M3, M13).
func TestDownloader_FilePermissions_0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions not enforced on windows")
	}
	payload := helperRandomBytes(t, 1024)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	d := NewDownloader()
	d.HTTPClient = srv.Client()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	dest := filepath.Join(t.TempDir(), "binary")
	asset := ReleaseAsset{DownloadURL: srv.URL + "/bin", Size: int64(len(payload))}

	require.NoError(t, d.Download(context.Background(), asset, dest, nil))
	stat, err := os.Stat(dest)
	require.NoError(t, err)
	// 0600 = owner rw only
	assert.Equal(t, os.FileMode(0o600), stat.Mode().Perm(),
		"file must be 0600 (got %o)", stat.Mode().Perm())
}

// TestDownloader_AtomicRename_TmpRemoved 는 성공 시 .tmp 파일이 남지 않음을 검증.
func TestDownloader_AtomicRename_TmpRemoved(t *testing.T) {
	payload := helperRandomBytes(t, 1024)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	d := NewDownloader()
	d.HTTPClient = srv.Client()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	dest := filepath.Join(t.TempDir(), "binary")
	asset := ReleaseAsset{DownloadURL: srv.URL + "/bin", Size: int64(len(payload))}

	require.NoError(t, d.Download(context.Background(), asset, dest, nil))
	_, err := os.Stat(dest + ".tmp")
	assert.True(t, os.IsNotExist(err), "tmp file must be removed (atomic rename complete)")
}

// TestDownloader_5xxError_PropagatesError 는 서버 에러를 ErrUpdateDownloadFailed 로 변환.
func TestDownloader_5xxError_PropagatesError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := NewDownloader()
	d.HTTPClient = srv.Client()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	dest := filepath.Join(t.TempDir(), "binary")
	asset := ReleaseAsset{DownloadURL: srv.URL + "/bin", Size: 100}

	err := d.Download(context.Background(), asset, dest, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateDownloadFailed))
	// 임시 파일도 정리됐는지 확인
	_, statErr := os.Stat(dest + ".tmp")
	assert.True(t, os.IsNotExist(statErr), "tmp file must be cleaned up on error")
}

// TestDownloader_ContextCancellation_RemovesTempFile 는 ctx 취소 시 임시 파일 정리.
func TestDownloader_ContextCancellation_RemovesTempFile(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 천천히 응답해서 cancel 이 먼저 도달하도록
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Length", "10000")
		_, _ = w.Write([]byte("partial"))
		if flusher != nil {
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()

	d := NewDownloader()
	d.HTTPClient = srv.Client()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	dest := filepath.Join(t.TempDir(), "binary")
	asset := ReleaseAsset{DownloadURL: srv.URL + "/bin", Size: 10000}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := d.Download(ctx, asset, dest, nil)
	require.Error(t, err)
	_, statErr := os.Stat(dest + ".tmp")
	assert.True(t, os.IsNotExist(statErr), "tmp must be removed on cancellation")
	_, destErr := os.Stat(dest)
	assert.True(t, os.IsNotExist(destErr), "dest must not exist on cancellation")
}

// TestDownloader_RedirectFollowed 는 302 리다이렉트를 따라가는지 검증 (GitHub assets 패턴).
func TestDownloader_RedirectFollowed(t *testing.T) {
	payload := helperRandomBytes(t, 1024)
	final := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer final.Close()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+"/redirected", http.StatusFound)
	}))
	defer srv.Close()

	d := NewDownloader()
	// 두 server 모두 same self-signed CA 가 아니므로, 둘 다 받아들이는 client 필요.
	// 간단히 각 서버 cert pool 합치는 대신 InsecureSkipVerify 로 테스트 단계 우회.
	d.HTTPClient = newInsecureTestClient()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	dest := filepath.Join(t.TempDir(), "binary")
	asset := ReleaseAsset{DownloadURL: srv.URL + "/bin", Size: int64(len(payload))}

	require.NoError(t, d.Download(context.Background(), asset, dest, nil))
	got, _ := os.ReadFile(dest)
	assert.Equal(t, payload, got)
}

// TestDownloader_DownloadManifest_ParsesChecksum 은 checksum.txt + .sig 다운로드 흐름.
//
// 다운로드된 checksum.txt 형식: "<sha256-hex>  xflowd-linux-amd64\n"
func TestDownloader_DownloadManifest_ParsesChecksum(t *testing.T) {
	binPayload := helperRandomBytes(t, 1024)
	binSum := sha256.Sum256(binPayload)
	binSumHex := hex.EncodeToString(binSum[:])
	sig := helperRandomBytes(t, 64) // ed25519 signature length

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/checksum.txt"):
			_, _ = w.Write([]byte(binSumHex + "  xflowd-linux-amd64\n"))
		case strings.HasSuffix(r.URL.Path, "/sig"):
			_, _ = w.Write(sig)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := NewDownloader()
	d.HTTPClient = srv.Client()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	checksumAsset := ReleaseAsset{Name: "checksum.txt", DownloadURL: srv.URL + "/checksum.txt", Size: 200}
	sigAsset := ReleaseAsset{Name: "xflowd-linux-amd64.sig", DownloadURL: srv.URL + "/sig", Size: 64}

	man, err := d.DownloadManifest(context.Background(), checksumAsset, sigAsset, "xflowd-linux-amd64", "v0.4.0")
	require.NoError(t, err)
	assert.Equal(t, Version("v0.4.0"), man.Version)
	assert.Equal(t, binSumHex, man.SHA256)
	assert.Equal(t, sig, man.Signature)
}

// TestDownloader_DownloadManifest_ChecksumMissingForBinary 는 해당 binary 항목이 없으면 에러.
func TestDownloader_DownloadManifest_ChecksumMissingForBinary(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/checksum.txt"):
			_, _ = w.Write([]byte("aabbcc  some-other-binary\n"))
		case strings.HasSuffix(r.URL.Path, "/sig"):
			_, _ = w.Write(make([]byte, 64))
		}
	}))
	defer srv.Close()

	d := NewDownloader()
	d.HTTPClient = srv.Client()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	checksumAsset := ReleaseAsset{DownloadURL: srv.URL + "/checksum.txt", Size: 200}
	sigAsset := ReleaseAsset{DownloadURL: srv.URL + "/sig", Size: 64}

	_, err := d.DownloadManifest(context.Background(), checksumAsset, sigAsset, "xflowd-linux-amd64", "v0.4.0")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
}

// newInsecureTestClient 는 다중 httptest.TLSServer 사이를 따라가기 위한 client (테스트 only).
func newInsecureTestClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = insecureTLSConfig()
	return &http.Client{Transport: tr, Timeout: 10 * time.Second}
}

// helperReadAll 은 의도적으로 io.ReadAll 호출 위치를 명시화.
func helperReadAll(t *testing.T, r io.Reader) []byte {
	t.Helper()
	b, err := io.ReadAll(r)
	require.NoError(t, err)
	return b
}
