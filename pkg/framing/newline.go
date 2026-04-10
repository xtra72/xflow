package framing

import (
	"bufio"
	"io"
)

// --- newlineFramer: 구분자 기반 프레이밍 ---

type newlineFramer struct {
	delimiter  byte
	bufferSize int
}

func (f *newlineFramer) Read(r io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, f.bufferSize), f.bufferSize)
	scanner.Split(f.splitFunc())

	if scanner.Scan() {
		return scanner.Bytes(), nil
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

func (f *newlineFramer) Write(w io.Writer, data []byte) error {
	buf := make([]byte, len(data)+1)
	copy(buf, data)
	buf[len(data)] = f.delimiter
	_, err := w.Write(buf)
	return err
}

// splitFunc 는 커스텀 구분자를 위한 bufio.SplitFunc 을 반환한다.
func (f *newlineFramer) splitFunc() bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		for i := 0; i < len(data); i++ {
			if data[i] == f.delimiter {
				return i + 1, data[:i], nil
			}
		}
		if atEOF && len(data) > 0 {
			return len(data), data, nil
		}
		return 0, nil, nil
	}
}

// ConfigureScanner 는 주어진 리더에 대해 newlineFramer 의 설정으로 구성된
// bufio.Scanner 를 생성하여 반환한다. ScannerConfigurer 인터페이스를 구현한다.
//
// 이 메서드는 호출자 (예: SerialConnReader) 가 리더 수명 동안 하나의 scanner 를
// 재사용하여 버퍼 데이터 손실을 방지하도록 하기 위해 존재한다. Read() 메서드는
// 매 호출마다 새 scanner 를 생성하므로 이 경로와는 독립적이다.
func (f *newlineFramer) ConfigureScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, f.bufferSize), f.bufferSize)
	scanner.Split(f.splitFunc())
	return scanner
}

// Drain 은 버퍼에서 delimiter 로 구분된 완성된 프레임을 모두 추출한다.
// splitFunc 의 동작과 일관되게, delimiter 는 소비되지만 반환된 프레임에는
// 포함되지 않는다. 마지막 delimiter 이후의 잔여 바이트는 remainder 로
// 돌아가 다음 Drain 호출에서 이어서 처리된다.
func (f *newlineFramer) Drain(buf []byte) ([][]byte, []byte, error) {
	if len(buf) == 0 {
		return nil, nil, nil
	}

	var frames [][]byte
	start := 0
	for i := 0; i < len(buf); i++ {
		if buf[i] == f.delimiter {
			frame := make([]byte, i-start)
			copy(frame, buf[start:i])
			frames = append(frames, frame)
			start = i + 1
		}
	}

	if start == len(buf) {
		return frames, nil, nil
	}
	return frames, buf[start:], nil
}
