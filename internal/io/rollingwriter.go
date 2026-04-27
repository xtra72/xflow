package xflowio

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// defaultMaxSize 는 로그 파일의 기본 최대 크기 (10MB).
const defaultMaxSize int64 = 10 * 1024 * 1024

// RollingWriterConfig 는 롤링 파일 라이터의 설정을 담는 구조체이다.
type RollingWriterConfig struct {
	// FilePath 는 로그 파일 경로이다. (필수)
	FilePath string
	// MaxSize 는 파일 최대 크기(바이트). 기본값: 10MB.
	MaxSize int64
	// MaxAge 는 백업 파일 최대 보관 일수. 0이면 무제한.
	MaxAge int
	// MaxBackups 는 백업 파일 최대 개수. 0이면 무제한.
	MaxBackups int
	// Compress 는 백업 파일을 gzip 압축할지 여부이다.
	Compress bool
}

// RollingWriter 는 크기/기간 기반 롤링 파일 라이터이다.
// io.WriteCloser 를 구현하며 스레드 안전하다.
type RollingWriter struct {
	cfg  RollingWriterConfig
	file *os.File
	size int64
	mu   sync.Mutex
}

// 컴파일 타임 인터페이스 체크
var _ io.WriteCloser = (*RollingWriter)(nil)

// NewRollingWriter 는 새로운 RollingWriter 를 생성한다.
// FilePath 가 비어있으면 에러를 반환한다.
// 부모 디렉터리가 없으면 자동으로 생성한다.
func NewRollingWriter(cfg RollingWriterConfig) (*RollingWriter, error) {
	if cfg.FilePath == "" {
		return nil, fmt.Errorf("rollingwriter: FilePath 는 필수입니다")
	}

	if cfg.MaxSize <= 0 {
		cfg.MaxSize = defaultMaxSize
	}

	// 부모 디렉터리 생성
	dir := filepath.Dir(cfg.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("rollingwriter: 디렉터리 생성 실패: %w", err)
	}

	// 파일 열기 (append 모드)
	f, err := os.OpenFile(cfg.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("rollingwriter: 파일 열기 실패: %w", err)
	}

	// 현재 파일 크기 확인
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("rollingwriter: 파일 상태 확인 실패: %w", err)
	}

	return &RollingWriter{
		cfg:  cfg,
		file: f,
		size: info.Size(),
	}, nil
}

// Write 는 데이터를 현재 파일에 기록한다.
// 기록 후 파일 크기가 MaxSize 를 초과하면 먼저 회전(rotate)한 뒤 기록한다.
func (rw *RollingWriter) Write(p []byte) (int, error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	if rw.file == nil {
		return 0, fmt.Errorf("rollingwriter: 파일이 닫혀 있습니다")
	}

	// 현재 크기 + 새 데이터가 MaxSize 를 초과하면 회전
	if rw.size+int64(len(p)) > rw.cfg.MaxSize {
		if err := rw.rotate(); err != nil {
			return 0, fmt.Errorf("rollingwriter: 회전 실패: %w", err)
		}
	}

	n, err := rw.file.Write(p)
	rw.size += int64(n)
	return n, err
}

// Close 는 현재 파일을 닫는다.
func (rw *RollingWriter) Close() error {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	if rw.file == nil {
		return nil
	}

	err := rw.file.Close()
	rw.file = nil
	return err
}

// rotate 는 현재 파일을 백업으로 이동하고 새 파일을 연다.
// mu 가 이미 잠긴 상태에서 호출된다.
func (rw *RollingWriter) rotate() error {
	// 현재 파일 닫기
	if err := rw.file.Close(); err != nil {
		return fmt.Errorf("파일 닫기 실패: %w", err)
	}

	// 백업 파일명 생성: {basename}-{timestamp}{ext}
	ext := filepath.Ext(rw.cfg.FilePath)
	basename := strings.TrimSuffix(filepath.Base(rw.cfg.FilePath), ext)
	dir := filepath.Dir(rw.cfg.FilePath)
	timestamp := time.Now().Format("20060102T150405")
	backupName := filepath.Join(dir, fmt.Sprintf("%s-%s%s", basename, timestamp, ext))

	// 현재 파일을 백업으로 이름 변경
	if err := os.Rename(rw.cfg.FilePath, backupName); err != nil {
		return fmt.Errorf("파일 이름 변경 실패: %w", err)
	}

	// 새 파일 열기
	f, err := os.OpenFile(rw.cfg.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("새 파일 열기 실패: %w", err)
	}

	rw.file = f
	rw.size = 0

	// 정리 작업 수행
	rw.cleanup()

	return nil
}

// cleanup 은 오래된/초과된 백업 파일을 삭제하고 압축을 수행한다.
// mu 가 이미 잠긴 상태에서 호출된다.
func (rw *RollingWriter) cleanup() {
	ext := filepath.Ext(rw.cfg.FilePath)
	basename := strings.TrimSuffix(filepath.Base(rw.cfg.FilePath), ext)
	dir := filepath.Dir(rw.cfg.FilePath)

	// 백업 파일 목록 조회
	pattern := filepath.Join(dir, fmt.Sprintf("%s-*%s", basename, ext))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return
	}

	// .gz 파일도 포함하여 조회
	gzPattern := filepath.Join(dir, fmt.Sprintf("%s-*%s.gz", basename, ext))
	gzMatches, err := filepath.Glob(gzPattern)
	if err == nil {
		matches = append(matches, gzMatches...)
	}

	// 이름순 정렬 (타임스탬프 기반이므로 시간순 정렬과 동일)
	sort.Strings(matches)

	// MaxBackups 초과 시 오래된 백업 삭제
	if rw.cfg.MaxBackups > 0 && len(matches) > rw.cfg.MaxBackups {
		excess := matches[:len(matches)-rw.cfg.MaxBackups]
		for _, f := range excess {
			os.Remove(f)
		}
		matches = matches[len(matches)-rw.cfg.MaxBackups:]
	}

	// MaxAge 초과 시 오래된 백업 삭제
	if rw.cfg.MaxAge > 0 {
		cutoff := time.Now().Add(-time.Duration(rw.cfg.MaxAge) * 24 * time.Hour)
		remaining := make([]string, 0, len(matches))
		for _, f := range matches {
			info, err := os.Stat(f)
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				os.Remove(f)
			} else {
				remaining = append(remaining, f)
			}
		}
		matches = remaining
	}

	// 압축 활성화 시 비압축 백업 파일을 gzip 으로 압축
	if rw.cfg.Compress {
		for _, f := range matches {
			if strings.HasSuffix(f, ".gz") {
				continue
			}
			rw.compressFile(f)
		}
	}
}

// compressFile 은 파일을 gzip 으로 압축하고 원본을 삭제한다.
func (rw *RollingWriter) compressFile(path string) {
	src, err := os.Open(path)
	if err != nil {
		return
	}
	defer src.Close()

	dst, err := os.Create(path + ".gz")
	if err != nil {
		return
	}
	defer dst.Close()

	gw := gzip.NewWriter(dst)
	defer gw.Close()

	if _, err := io.Copy(gw, src); err != nil {
		return
	}

	// gzip writer 를 먼저 닫아야 flush 가 완료된다
	if err := gw.Close(); err != nil {
		return
	}

	// 원본 파일 닫기 후 삭제
	src.Close()
	os.Remove(path)
}
