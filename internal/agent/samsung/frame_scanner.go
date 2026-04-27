package samsung

import (
	"bytes"
	"encoding/binary"
)

// maxFrameSize 는 허용하는 최대 프레임 크기이다.
// NASA 프로토콜에서 LEN 필드는 2바이트이므로 이론상 64KB까지 가능하지만,
// 실제 HVAC 프레임은 수백 바이트를 넘지 않는다.
const maxFrameSize = 4096

// frameScanner 는 시리얼 스트림에서 NASA 프로토콜 프레임을 버퍼링하여 추출한다.
//
// 동작 원리:
//  1. Write: STX가 없으면 전부 버린다. STX부터만 버퍼에 쌓는다.
//  2. Next: 버퍼 선두의 STX → LEN → 예상 크기 → ETX 검증.
//     실패하면 현재 STX를 버리고 다음 STX를 찾는다.
type frameScanner struct {
	buf []byte
}

// newFrameScanner 는 새로운 frameScanner를 생성한다.
func newFrameScanner() *frameScanner {
	return &frameScanner{}
}

// Write 는 수신된 바이트를 버퍼에 추가한다.
// 버퍼가 비어 있으면 STX 이전 바이트는 즉시 버린다.
func (s *frameScanner) Write(data []byte) {
	if len(s.buf) == 0 {
		// 버퍼 비어 있음 → STX를 찾아서 그 위치부터만 저장
		idx := bytes.IndexByte(data, FrameSTX)
		if idx == -1 {
			return // STX 없음, 전부 버림
		}
		data = data[idx:]
	}
	s.buf = append(s.buf, data...)
}

// Next 는 버퍼에서 완전한 프레임을 하나 추출한다.
// 완전한 프레임이 없으면 nil, false를 반환한다.
func (s *frameScanner) Next() ([]byte, bool) {
	for {
		// 버퍼 선두를 STX로 맞춘다
		s.alignToSTX()
		if len(s.buf) == 0 {
			return nil, false
		}

		// STX + LEN(2바이트) = 최소 3바이트 필요
		if len(s.buf) < 3 {
			return nil, false
		}

		// LEN → 총 프레임 크기
		frameLen := int(binary.BigEndian.Uint16(s.buf[1:3]))
		expectedTotal := 1 + frameLen + 1 // STX(1) + LEN값 + ETX(1)

		// LEN 유효성: 너무 작거나 크면 잘못된 STX
		if expectedTotal < MinFrameSize || expectedTotal > maxFrameSize {
			s.skipSTX()
			continue
		}

		// 프레임 전체가 아직 도착하지 않음
		if len(s.buf) < expectedTotal {
			return nil, false
		}

		// ETX 검증
		if s.buf[expectedTotal-1] != FrameETX {
			s.skipSTX()
			continue
		}

		// 프레임 추출
		frame := make([]byte, expectedTotal)
		copy(frame, s.buf[:expectedTotal])
		s.buf = s.buf[expectedTotal:]
		return frame, true
	}
}

// alignToSTX 는 버퍼 선두를 STX 위치로 맞춘다.
// STX가 없으면 버퍼를 비운다.
func (s *frameScanner) alignToSTX() {
	if len(s.buf) == 0 || s.buf[0] == FrameSTX {
		return
	}
	idx := bytes.IndexByte(s.buf, FrameSTX)
	if idx == -1 {
		s.buf = s.buf[:0]
		return
	}
	s.buf = s.buf[idx:]
}

// skipSTX 는 현재 STX를 건너뛰고 다음 STX를 찾는다.
func (s *frameScanner) skipSTX() {
	if len(s.buf) <= 1 {
		s.buf = s.buf[:0]
		return
	}
	s.buf = s.buf[1:]
	s.alignToSTX()
}

// Buffered 는 현재 버퍼에 남아 있는 바이트 수를 반환한다.
func (s *frameScanner) Buffered() int {
	return len(s.buf)
}

// Reset 은 내부 버퍼를 초기화한다.
func (s *frameScanner) Reset() {
	s.buf = s.buf[:0]
}
