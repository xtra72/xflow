package framing

import "io"

// --- rawFramer: 원시 바이트 스트림 ---

type rawFramer struct {
	bufferSize int
}

func (f *rawFramer) Read(r io.Reader) ([]byte, error) {
	buf := make([]byte, f.bufferSize)
	n, err := r.Read(buf)
	if err != nil {
		return nil, err
	}
	// 2026-05-14 hotfix: three-index slice 로 cap 을 n 으로 제한한다.
	// cap == bufferSize 이면 downstream 의 append 가 공유 backing 배열에
	// 써넣어 다른 프레임을 변조할 수 있다.
	return buf[:n:n], nil
}

func (f *rawFramer) Write(w io.Writer, data []byte) error {
	_, err := w.Write(data)
	return err
}

// Drain 은 버퍼 전체를 하나의 프레임으로 반환한다. raw 모드는 구분자 없이
// 주어진 바이트 전체를 그대로 전달하는 passthrough 이므로 remainder 는 항상
// 비어있다. 빈 입력은 프레임 없음으로 처리된다.
func (f *rawFramer) Drain(buf []byte) ([][]byte, []byte, error) {
	if len(buf) == 0 {
		return nil, nil, nil
	}
	frame := make([]byte, len(buf))
	copy(frame, buf)
	return [][]byte{frame}, nil, nil
}
