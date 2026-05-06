// @SPEC:SPEC-UPDATE-001 v0.1.0
// downloader.go — HTTPS 다운로드 + 진행률 + 사전 디스크 검사 (M3).
//
// 보안 critical:
//   - HTTPS 전용 (http:// asset URL 거부; M1, M13)
//   - 임시 파일 권한 0600 (실행 불가; M3, M13)
//   - 원자적 rename (download → .tmp → rename to dest)
//   - context cancel 시 .tmp 즉시 정리
//
// 디자인:
//   - diskInspector 인터페이스로 디스크 검사 결정론화 (테스트 모킹 가능)
//   - production: realDiskInspector (syscall.Statfs_t)
//   - test: fakeDiskInspector (avail bytes 직접 지정)
//   - 사전 검사 임계값: asset.Size × 3 (백업 + 새 + 여유; SPEC M3)
//   - 진행률 콜백: 매 64KB 또는 마지막 호출 (전체 크기 도달 시)
package updater

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// diskInspector 는 임시 디렉토리의 가용 디스크 공간을 보고하는 추상화이다.
//
// production 구현 (statfs_unix.go) 은 syscall.Statfs_t 를 사용.
// 테스트는 fakeDiskInspector 로 결정론적 값 주입.
type diskInspector interface {
	AvailableBytes(path string) (uint64, error)
}

// Downloader 는 HTTPS 다운로드 + 진행률 보고 + 디스크 검사를 수행한다.
type Downloader struct {
	HTTPClient *http.Client
	disk       diskInspector
}

// NewDownloader 는 production 기본값으로 Downloader 를 생성한다.
//
// HTTPClient: 30분 타임아웃 (큰 바이너리 다운로드 대응).
// disk: realDiskInspector (운영체제별 statfs).
func NewDownloader() *Downloader {
	return &Downloader{
		HTTPClient: &http.Client{
			Timeout: 30 * time.Minute,
		},
		disk: realDisk{},
	}
}

// ProgressFunc 는 다운로드 진행 콜백 시그니처이다.
//
// downloaded: 누적 바이트 수
// total: 전체 바이트 수 (Content-Length 또는 asset.Size)
type ProgressFunc func(downloaded, total int64)

// Download 는 asset 을 destPath 로 다운로드한다 (원자적, HTTPS 전용).
//
// 흐름:
//  1. asset.DownloadURL 스킴 검증 (HTTPS only)
//  2. 사전 디스크 검사 (asset.Size × 3 보다 가용량이 작으면 거부)
//  3. ${destPath}.tmp 에 0600 권한으로 다운로드
//  4. 진행률 콜백 호출 (64KB boundary 마다)
//  5. 성공 시 atomic rename to destPath
//  6. 실패 / ctx cancel 시 .tmp 정리
//
// 반환:
//   - nil: 성공 (destPath 에 0600 파일 존재)
//   - ErrUpdateChannelInvalid: http:// 또는 malformed URL
//   - ErrUpdateInsufficientDiskSpace: 디스크 검사 실패 또는 inspector 자체 에러
//   - ErrUpdateDownloadFailed: 5xx, network error, truncated download 등
//   - ctx.Err(): cancel/timeout
func (d *Downloader) Download(ctx context.Context, asset ReleaseAsset, destPath string, progress ProgressFunc) error {
	// 1) URL 검증 (HTTPS 강제)
	if err := requireHTTPS(asset.DownloadURL); err != nil {
		return err
	}

	// 2) 사전 디스크 검사
	tmpDir := filepath.Dir(destPath)
	if err := d.checkDiskSpace(tmpDir, asset.Size); err != nil {
		return err
	}

	// 3) 임시 파일 생성 (0600)
	tmpPath := destPath + ".tmp"
	tmp, err := os.OpenFile(tmpPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("%w: open tmp file: %v", ErrUpdateDownloadFailed, err)
	}
	closed := false
	cleanup := func() {
		if !closed {
			_ = tmp.Close()
			closed = true
		}
		_ = os.Remove(tmpPath)
	}

	// 4) HTTP GET
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		cleanup()
		return fmt.Errorf("%w: build request: %v", ErrUpdateDownloadFailed, err)
	}
	req.Header.Set("User-Agent", "xflow-updater/1.0")

	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		cleanup()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: do request: %v", ErrUpdateDownloadFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		cleanup()
		return fmt.Errorf("%w: HTTP %d", ErrUpdateDownloadFailed, resp.StatusCode)
	}

	// 5) 스트리밍 복사 + 진행률
	total := asset.Size
	if resp.ContentLength > 0 {
		total = resp.ContentLength
	}
	if err := streamWithProgress(ctx, resp.Body, tmp, total, progress); err != nil {
		cleanup()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: stream: %v", ErrUpdateDownloadFailed, err)
	}

	// 6) flush + close + atomic rename
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("%w: fsync: %v", ErrUpdateDownloadFailed, err)
	}
	if err := tmp.Close(); err != nil {
		closed = true
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%w: close tmp: %v", ErrUpdateDownloadFailed, err)
	}
	closed = true

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%w: rename: %v", ErrUpdateDownloadFailed, err)
	}
	return nil
}

// DownloadManifest 는 checksum.txt + signature 파일을 다운로드하고 Manifest 를 구성한다.
//
// 인자:
//   - checksumAsset: checksum.txt asset (한 행씩 "<hex>  <name>" 형식)
//   - signatureAsset: <binaryName>.sig asset (Ed25519 raw 64 bytes)
//   - binaryName: checksum 파일에서 매칭할 바이너리 이름 (예: "xflowd-linux-amd64")
//   - version: 결과 Manifest 의 Version 필드
//
// checksum.txt 미발견 (binaryName 행 부재) → ErrUpdateInvalidInput.
// 다운로드 자체 에러 → ErrUpdateDownloadFailed (URL/서버) 또는 ErrUpdateChannelInvalid (HTTPS).
func (d *Downloader) DownloadManifest(
	ctx context.Context,
	checksumAsset, signatureAsset ReleaseAsset,
	binaryName string,
	version Version,
) (Manifest, error) {
	if err := requireHTTPS(checksumAsset.DownloadURL); err != nil {
		return Manifest{}, err
	}
	if err := requireHTTPS(signatureAsset.DownloadURL); err != nil {
		return Manifest{}, err
	}

	checksumBytes, err := d.fetchSmallFile(ctx, checksumAsset.DownloadURL, 1<<20) // 1MB cap
	if err != nil {
		return Manifest{}, err
	}
	sigBytes, err := d.fetchSmallFile(ctx, signatureAsset.DownloadURL, 1<<16) // 64KB cap
	if err != nil {
		return Manifest{}, err
	}

	// checksum 파일 파싱: "<hex>  <name>"
	var sumHex string
	scanner := bufio.NewScanner(strings.NewReader(string(checksumBytes)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// 형식: "<hex>" + 공백 + "<name>" (한 개 또는 두 개 공백)
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[len(fields)-1] == binaryName {
			sumHex = fields[0]
			break
		}
	}
	if sumHex == "" {
		return Manifest{}, fmt.Errorf("%w: no checksum entry for %q in checksum file",
			ErrUpdateInvalidInput, binaryName)
	}

	return Manifest{
		Version:   version,
		SHA256:    sumHex,
		Signature: sigBytes,
		// BinaryURL 은 caller 가 ReleaseAsset 으로부터 채워야 함 (Manifest 의 일관성)
	}, nil
}

// fetchSmallFile 은 작은 메타데이터 파일 (checksum, signature) 을 메모리에 로드한다.
//
// limit 이 초과되면 ErrUpdateDownloadFailed (DoS 방어).
func (d *Downloader) fetchSmallFile(ctx context.Context, urlStr string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrUpdateDownloadFailed, err)
	}
	req.Header.Set("User-Agent", "xflow-updater/1.0")

	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: do request: %v", ErrUpdateDownloadFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: HTTP %d", ErrUpdateDownloadFailed, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrUpdateDownloadFailed, err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%w: response exceeds %d bytes", ErrUpdateDownloadFailed, limit)
	}
	return body, nil
}

// requireHTTPS 는 URL 의 스킴이 https 가 아니면 ErrUpdateChannelInvalid 를 반환한다.
func requireHTTPS(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: parse URL: %v", ErrUpdateChannelInvalid, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%w: must use https:// scheme; got %s://", ErrUpdateChannelInvalid, u.Scheme)
	}
	return nil
}

// checkDiskSpace 는 사전 검사를 수행한다 (asset.Size × 3 미만이면 실패).
//
// SPEC M3: 백업 (1×) + 새 바이너리 (1×) + 여유 (1×) = 3× 임계값.
// inspector 자체 에러 (statfs 실패) 는 보수적으로 "공간 부족" 처리.
func (d *Downloader) checkDiskSpace(dir string, assetSize int64) error {
	if assetSize <= 0 {
		// asset.Size 가 미정인 경우 검사 생략 (서버가 size 를 안 줄 때)
		return nil
	}
	required := uint64(assetSize) * 3
	avail, err := d.disk.AvailableBytes(dir)
	if err != nil {
		// inspector 실패 → 안전하게 거부 (false negative 보다 false positive 우선)
		return fmt.Errorf("%w: disk inspect %q: %v", ErrUpdateInsufficientDiskSpace, dir, err)
	}
	if avail < required {
		return fmt.Errorf("%w: in %s: %d bytes available, %d required",
			ErrUpdateInsufficientDiskSpace, dir, avail, required)
	}
	return nil
}

// streamWithProgress 는 src → dst 를 buffer 로 복사하며 매 64KB 마다 progress 를 호출한다.
//
// progress 가 nil 이면 콜백 생략. 마지막 호출은 항상 written == total 시점에 수행.
func streamWithProgress(ctx context.Context, src io.Reader, dst io.Writer, total int64, progress ProgressFunc) error {
	const chunk = 64 * 1024
	buf := make([]byte, chunk)
	var written int64
	var lastReported int64

	for {
		// ctx 체크 (large file 다운로드 중 cancel 즉시 반응)
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, rerr := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			if progress != nil && written-lastReported >= chunk {
				progress(written, total)
				lastReported = written
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	// 마지막 진행 보고 (총량에 도달한 시점)
	if progress != nil && written != lastReported {
		progress(written, total)
	}
	return nil
}

// errReadFailed 는 프로덕션 코드에서 사용되지 않으나, errors.Is 호환 sentinel 을 위한 placeholder.
var _ = errors.New
