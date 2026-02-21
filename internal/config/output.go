package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ParseLogOutput 은 observe.output 문자열을 파싱하여 io.Writer와 io.Closer를 반환한다.
// output 파라미터는 다음 형식을 지원한다:
//   - "stdout" → os.Stdout, nil, nil
//   - "/path/to/file" → *os.File, *os.File, nil
//   - "stdout+/path/to/file" → io.MultiWriter(os.Stdout, *os.File), *os.File, nil
//
// 파일 경로가 지정된 경우:
//   - 디렉토리가 없으면 os.MkdirAll로 자동 생성 (0755 퍼미션)
//   - 파일은 append 모드로 열림 (os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644 퍼미션)
//   - 반환된 io.Closer를 호출자가 닫아야 함
func ParseLogOutput(output string) (io.Writer, io.Closer, error) {
	if output == "" {
		return nil, nil, fmt.Errorf("%w: 빈 문자열", ErrInvalidLogOutput)
	}

	if output == "stdout" {
		return os.Stdout, nil, nil
	}

	// "stdout+파일경로" 형식
	if strings.HasPrefix(output, "stdout+") {
		filePath := strings.TrimPrefix(output, "stdout+")
		if filePath == "" {
			return nil, nil, fmt.Errorf("%w: stdout+ 뒤에 파일 경로가 필요합니다", ErrInvalidLogOutput)
		}
		file, err := openLogFile(filePath)
		if err != nil {
			return nil, nil, err
		}
		return io.MultiWriter(os.Stdout, file), file, nil
	}

	// 단일 파일 경로
	file, err := openLogFile(output)
	if err != nil {
		return nil, nil, err
	}
	return file, file, nil
}

// openLogFile 은 로그 파일을 append 모드로 연다.
// 디렉토리가 없으면 자동 생성한다.
func openLogFile(path string) (*os.File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("%w: 디렉토리 생성 실패: %s", ErrInvalidLogOutput, err)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("%w: 파일 열기 실패: %s", ErrInvalidLogOutput, err)
	}

	return file, nil
}
