package framing

import (
	"encoding/binary"
	"fmt"
	"io"
)

// --- lengthPrefixFramer: 4바이트 빅엔디안 길이 접두사 ---

type lengthPrefixFramer struct {
	maxMessageSize int
}

func (f *lengthPrefixFramer) Read(r io.Reader) ([]byte, error) {
	// 4바이트 길이 헤더 읽기
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(header)

	// 최대 메시지 크기 검증
	if f.maxMessageSize > 0 && int(length) > f.maxMessageSize {
		return nil, fmt.Errorf("%w: %d bytes (max %d)", ErrMaxMessageSize, length, f.maxMessageSize)
	}

	// 페이로드 읽기
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (f *lengthPrefixFramer) Write(w io.Writer, data []byte) error {
	// 최대 메시지 크기 검증
	if f.maxMessageSize > 0 && len(data) > f.maxMessageSize {
		return fmt.Errorf("%w: %d bytes (max %d)", ErrMaxMessageSize, len(data), f.maxMessageSize)
	}

	// 4바이트 길이 헤더 + 페이로드 전송
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

// maxSaneLengthPrefix 는 maxMessageSize 가 미설정 (0) 일 때 길이 필드의
// 상한 안전 검사에 사용된다. 256 MiB 를 초과하는 length 는 손상된 헤더로
// 판단하여 ErrLengthInvalid 를 반환한다.
const maxSaneLengthPrefix = 256 * 1024 * 1024

// Drain 은 버퍼에서 4 바이트 빅엔디안 길이 접두사 프레임을 모두 추출한다.
// Read 와 동일한 알고리즘 (4 바이트 헤더 + payloadLen 바이트 페이로드) 를
// 사용하며, 부분 헤더나 부분 페이로드는 remainder 로 유지된다.
func (f *lengthPrefixFramer) Drain(buf []byte) ([][]byte, []byte, error) {
	var frames [][]byte
	pos := 0
	for {
		// 헤더 (4 바이트) 확보 가능한지 확인
		if len(buf)-pos < 4 {
			if pos == 0 {
				return frames, buf, nil
			}
			rem := make([]byte, len(buf)-pos)
			copy(rem, buf[pos:])
			return frames, rem, nil
		}
		length := binary.BigEndian.Uint32(buf[pos : pos+4])
		if f.maxMessageSize > 0 && int(length) > f.maxMessageSize {
			return frames, buf[pos:], fmt.Errorf("%w: %d bytes (max %d)", ErrMaxMessageSize, length, f.maxMessageSize)
		}
		// maxMessageSize 미설정 시 안전 상한 검사: 손상된 헤더 감지
		if f.maxMessageSize == 0 && length > maxSaneLengthPrefix {
			return frames, buf[pos:], fmt.Errorf("%w: decoded length %d exceeds sane limit", ErrLengthInvalid, length)
		}
		frameEnd := pos + 4 + int(length)
		if frameEnd > len(buf) {
			// 페이로드 불완전 → 헤더부터 remainder 로
			if pos == 0 {
				return frames, buf, nil
			}
			rem := make([]byte, len(buf)-pos)
			copy(rem, buf[pos:])
			return frames, rem, nil
		}
		payload := make([]byte, length)
		copy(payload, buf[pos+4:frameEnd])
		frames = append(frames, payload)
		pos = frameEnd
		if pos == len(buf) {
			return frames, nil, nil
		}
	}
}
