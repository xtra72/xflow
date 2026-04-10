package framing

import "io"

// --- fixedSizeFramer: 고정 크기 프레이밍 ---

type fixedSizeFramer struct {
	fixedSize int
}

func (f *fixedSizeFramer) Read(r io.Reader) ([]byte, error) {
	buf := make([]byte, f.fixedSize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (f *fixedSizeFramer) Write(w io.Writer, data []byte) error {
	buf := make([]byte, f.fixedSize)
	copy(buf, data) // 짧으면 제로 패딩, 길면 잘림
	_, err := w.Write(buf)
	return err
}

// Drain 은 버퍼를 고정 크기 청크로 분할한다. fixedSize 배수에 미치지 못하는
// 잔여 바이트는 remainder 로 유지되어 다음 호출에서 이어서 처리된다.
func (f *fixedSizeFramer) Drain(buf []byte) ([][]byte, []byte, error) {
	if f.fixedSize <= 0 || len(buf) < f.fixedSize {
		if len(buf) == 0 {
			return nil, nil, nil
		}
		return nil, buf, nil
	}
	var frames [][]byte
	pos := 0
	for pos+f.fixedSize <= len(buf) {
		frame := make([]byte, f.fixedSize)
		copy(frame, buf[pos:pos+f.fixedSize])
		frames = append(frames, frame)
		pos += f.fixedSize
	}
	if pos == len(buf) {
		return frames, nil, nil
	}
	rem := make([]byte, len(buf)-pos)
	copy(rem, buf[pos:])
	return frames, rem, nil
}
