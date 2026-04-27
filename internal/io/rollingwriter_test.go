package xflowio

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRollingWriter_Basic 는 기본 쓰기 동작을 검증한다.
func TestRollingWriter_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	rw, err := NewRollingWriter(RollingWriterConfig{
		FilePath: path,
	})
	require.NoError(t, err)
	defer rw.Close()

	data := []byte("hello world\n")
	n, err := rw.Write(data)
	require.NoError(t, err)
	assert.Equal(t, len(data), n)

	// 파일 존재 및 내용 확인
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello world\n", string(content))
}

// TestRollingWriter_Rotation 은 MaxSize 초과 시 파일 회전을 검증한다.
func TestRollingWriter_Rotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	rw, err := NewRollingWriter(RollingWriterConfig{
		FilePath: path,
		MaxSize:  100, // 100 바이트로 설정
	})
	require.NoError(t, err)
	defer rw.Close()

	// 100 바이트 초과 데이터 기록
	data := strings.Repeat("x", 60) + "\n"
	_, err = rw.Write([]byte(data))
	require.NoError(t, err)

	// 두 번째 쓰기 시 회전 발생
	_, err = rw.Write([]byte(data))
	require.NoError(t, err)

	// 백업 파일 존재 확인
	matches, err := filepath.Glob(filepath.Join(dir, "app-*.log"))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(matches), 1, "백업 파일이 존재해야 합니다")
}

// TestRollingWriter_MaxBackups 는 MaxBackups 초과 시 오래된 백업 삭제를 검증한다.
func TestRollingWriter_MaxBackups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	// 미리 백업 파일 4개를 수동으로 생성
	for i := 0; i < 4; i++ {
		ts := time.Date(2024, 1, 1+i, 0, 0, 0, 0, time.UTC).Format("20060102T150405")
		bp := filepath.Join(dir, "app-"+ts+".log")
		err := os.WriteFile(bp, []byte("backup data"), 0644)
		require.NoError(t, err)
	}

	rw, err := NewRollingWriter(RollingWriterConfig{
		FilePath:   path,
		MaxSize:    50, // 50 바이트
		MaxBackups: 2,
	})
	require.NoError(t, err)
	defer rw.Close()

	// 회전을 유발하여 cleanup 실행
	data := strings.Repeat("y", 51) + "\n"
	_, err = rw.Write([]byte(data))
	require.NoError(t, err)
	_, err = rw.Write([]byte(data))
	require.NoError(t, err)

	// 백업 파일 개수 확인: 최대 2개만 남아야 함
	matches, err := filepath.Glob(filepath.Join(dir, "app-*.log"))
	require.NoError(t, err)
	assert.LessOrEqual(t, len(matches), 2, "백업 파일은 최대 %d개여야 합니다", 2)
}

// TestRollingWriter_MaxAge 는 MaxAge 초과 시 오래된 백업 삭제를 검증한다.
func TestRollingWriter_MaxAge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	// 오래된 백업 파일을 수동으로 생성
	oldBackup := filepath.Join(dir, "app-20200101T000000.log")
	err := os.WriteFile(oldBackup, []byte("old data"), 0644)
	require.NoError(t, err)

	// 수정 시간을 2일 전으로 설정
	oldTime := time.Now().Add(-48 * time.Hour)
	err = os.Chtimes(oldBackup, oldTime, oldTime)
	require.NoError(t, err)

	rw, err := NewRollingWriter(RollingWriterConfig{
		FilePath: path,
		MaxSize:  50,
		MaxAge:   1, // 1일
	})
	require.NoError(t, err)
	defer rw.Close()

	// 회전을 유발
	data := strings.Repeat("z", 51) + "\n"
	_, err = rw.Write([]byte(data))
	require.NoError(t, err)
	_, err = rw.Write([]byte(data))
	require.NoError(t, err)

	// 오래된 백업 파일이 삭제되었는지 확인
	_, err = os.Stat(oldBackup)
	assert.True(t, os.IsNotExist(err), "MaxAge 를 초과한 백업 파일은 삭제되어야 합니다")
}

// TestRollingWriter_Compress 는 압축 기능을 검증한다.
func TestRollingWriter_Compress(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	rw, err := NewRollingWriter(RollingWriterConfig{
		FilePath: path,
		MaxSize:  50,
		Compress: true,
	})
	require.NoError(t, err)
	defer rw.Close()

	// 회전을 유발
	data := strings.Repeat("a", 51) + "\n"
	_, err = rw.Write([]byte(data))
	require.NoError(t, err)
	_, err = rw.Write([]byte(data))
	require.NoError(t, err)

	// .gz 파일이 존재하는지 확인
	matches, err := filepath.Glob(filepath.Join(dir, "app-*.log.gz"))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(matches), 1, ".gz 백업 파일이 존재해야 합니다")

	// .gz 파일이 유효한 gzip 인지 확인
	f, err := os.Open(matches[0])
	require.NoError(t, err)
	defer f.Close()

	gr, err := gzip.NewReader(f)
	require.NoError(t, err)
	defer gr.Close()

	content, err := io.ReadAll(gr)
	require.NoError(t, err)
	assert.Contains(t, string(content), strings.Repeat("a", 51))
}

// TestRollingWriter_Close 는 닫힌 후 쓰기가 실패하는지 검증한다.
func TestRollingWriter_Close(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	rw, err := NewRollingWriter(RollingWriterConfig{
		FilePath: path,
	})
	require.NoError(t, err)

	// 정상 쓰기 확인
	_, err = rw.Write([]byte("hello"))
	require.NoError(t, err)

	// 닫기
	err = rw.Close()
	require.NoError(t, err)

	// 닫힌 후 쓰기 시도
	_, err = rw.Write([]byte("world"))
	assert.Error(t, err, "닫힌 writer 에 쓰기하면 에러가 발생해야 합니다")
}
