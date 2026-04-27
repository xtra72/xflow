package framing

import "io"

// --- streamFramer: 유휴 타임아웃 기반 스트림 프레이밍 ---
// 수신 데이터를 누적하다가 idle timeout(기본 1ms) 동안 추가 데이터가 없으면
// 누적된 데이터를 하나의 프레임으로 반환한다.
// 실제 유휴 감지는 시리얼 포트의 ReadTimeout 설정에 의존한다.

type streamFramer struct {
	bufferSize int
}

func (f *streamFramer) Read(r io.Reader) ([]byte, error) {
	buf := make([]byte, f.bufferSize)
	var acc []byte

	for {
		n, err := r.Read(buf)
		if n > 0 {
			acc = append(acc, buf[:n]...)
			continue
		}
		// 추가 데이터 없음 (타임아웃) — 누적 데이터가 있으면 플러시
		if len(acc) > 0 {
			return acc, nil
		}
		// 누적 데이터 없이 실제 오류 발생
		if err != nil {
			return nil, err
		}
		// 순수 타임아웃, 아직 데이터 없음 — 계속 대기
	}
}

func (f *streamFramer) Write(w io.Writer, data []byte) error {
	_, err := w.Write(data)
	return err
}

// Drain 은 스트림 모드의 passthrough 시맨틱을 구현한다. Read 의 유휴 타임아웃
// 기반 누적 동작은 바이트 슬라이스 맥락에서 의미가 없으므로, Drain 은 호출
// 시점에 버퍼에 있는 모든 바이트를 하나의 프레임으로 반환하고 remainder 를
// 비운다.
func (f *streamFramer) Drain(buf []byte) ([][]byte, []byte, error) {
	if len(buf) == 0 {
		return nil, nil, nil
	}
	frame := make([]byte, len(buf))
	copy(frame, buf)
	return [][]byte{frame}, nil, nil
}
