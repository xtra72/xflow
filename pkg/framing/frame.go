package framing

import (
	"encoding/binary"
	"fmt"
	"io"
)

// --- frameFramer: 프로토콜 수준 프레임 감지 (STX/길이/ETX/체크섬) ---

type frameFramer struct {
	stx                  []byte
	etx                  []byte
	lengthOffset         int
	lengthSize           int    // 1 또는 2
	lengthEndian         string // "big" 또는 "little"
	lengthIncludesHeader bool
	lengthAdjustment     int
	checksum             string // "none", "sum8", "xor"
	maxMessageSize       int
}

func (f *frameFramer) Read(r io.Reader) ([]byte, error) {
	// 1. STX 탐색 — 바이트 단위로 읽으며 STX 패턴 매칭
	stxBuf := make([]byte, len(f.stx))
	if err := f.findSTX(r, stxBuf); err != nil {
		return nil, err
	}

	// 2. 헤더 읽기 — STX 이후 length_offset 까지 추가 바이트 읽기
	headerExtra := f.lengthOffset - len(f.stx)
	var header []byte
	header = append(header, stxBuf...)
	if headerExtra > 0 {
		extra := make([]byte, headerExtra)
		if _, err := io.ReadFull(r, extra); err != nil {
			return nil, err
		}
		header = append(header, extra...)
	}

	// 3. 길이 필드 읽기
	lenBuf := make([]byte, f.lengthSize)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, err
	}
	header = append(header, lenBuf...)

	payloadLen := f.decodeLength(lenBuf)

	// length_includes_header 처리
	if f.lengthIncludesHeader {
		// 길이에 지금까지 읽은 바이트(header)가 포함되어 있으므로 빼준다
		payloadLen -= len(header)
		if payloadLen < 0 {
			payloadLen = 0
		}
	}

	// length_adjustment 보정
	payloadLen += f.lengthAdjustment
	if payloadLen < 0 {
		payloadLen = 0
	}

	// 최대 메시지 크기 검증
	totalSize := len(header) + payloadLen
	if f.maxMessageSize > 0 && totalSize > f.maxMessageSize {
		return nil, fmt.Errorf("%w: %d bytes (max %d)", ErrFrameTooLarge, totalSize, f.maxMessageSize)
	}

	// 4. 나머지 페이로드 읽기
	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}
	}

	frame := append(header, payload...)

	// 5. ETX 검증
	if len(f.etx) > 0 {
		etxStart := len(frame) - len(f.etx)
		if f.checksum != "none" {
			etxStart -= 1 // 체크섬 바이트 앞에 ETX
		}
		if etxStart < 0 || etxStart+len(f.etx) > len(frame) {
			return nil, fmt.Errorf("%w", ErrETXMismatch)
		}
		for i, b := range f.etx {
			if frame[etxStart+i] != b {
				return nil, fmt.Errorf("%w", ErrETXMismatch)
			}
		}
	}

	// 6. 체크섬 검증
	if f.checksum != "none" {
		csIdx := len(frame) - 1
		if csIdx < 0 {
			return nil, fmt.Errorf("%w", ErrChecksumMismatch)
		}
		expected := frame[csIdx]
		data := frame[:csIdx]
		var calculated byte
		switch f.checksum {
		case "sum8":
			calculated = checksumSum8(data)
		case "xor":
			calculated = checksumXOR(data)
		}
		if calculated != expected {
			return nil, fmt.Errorf("%w: expected 0x%02X, got 0x%02X", ErrChecksumMismatch, expected, calculated)
		}
	}

	return frame, nil
}

func (f *frameFramer) Write(w io.Writer, data []byte) error {
	_, err := w.Write(data)
	return err
}

// Drain 은 버퍼에서 STX/길이/ETX/체크섬 기반 프레임을 모두 추출한다.
// Read 와 동일한 파싱 알고리즘을 사용하며, LGCP 샘플 등 결정적 입력에
// 대해 Read 와 동일한 프레임 시퀀스를 생성한다.
//
// 복구 정책:
//
//   - STX 를 찾지 못하면 noise 는 모두 소비되고 remainder 가 비어진다.
//   - STX 이후 헤더/길이/페이로드 바이트가 부족하면 STX 부터 remainder 로
//     유지되어 다음 호출에서 더 많은 바이트가 올 때 이어서 파싱한다.
//   - 파싱 에러 (ETX 불일치, 체크섬 불일치, 최대 크기 초과) 가 발생하면
//     지금까지 조립된 프레임, 해당 STX 바로 다음부터 시작하는 remainder,
//     에러를 반환한다. remainder 는 다음 STX 를 탐색하는 데 사용된다.
func (f *frameFramer) Drain(buf []byte) ([][]byte, []byte, error) {
	var frames [][]byte
	pos := 0
	for pos < len(buf) {
		// 1. STX 탐색
		stxIdx := f.findSTXInBuf(buf[pos:])
		if stxIdx < 0 {
			// STX 없음 → noise 모두 버리고 종료
			return frames, nil, nil
		}
		framePos := pos + stxIdx

		// 2. 헤더/길이/페이로드 파싱 시도. 바이트 부족이면 framePos 부터
		//    remainder 로 유지한다. 파싱 에러이면 framePos+1 이후부터
		//    재탐색하도록 remainder 를 구성하여 반환.
		frame, consumed, partial, err := f.parseFrameAt(buf[framePos:])
		if partial {
			// 더 많은 바이트가 필요함
			rem := make([]byte, len(buf)-framePos)
			copy(rem, buf[framePos:])
			return frames, rem, nil
		}
		if err != nil {
			// 파싱 에러 → 현재 STX 를 버리고 다음 바이트부터 재탐색
			// remainder 는 framePos+1 부터 끝까지
			rem := make([]byte, len(buf)-framePos-1)
			copy(rem, buf[framePos+1:])
			return frames, rem, err
		}
		frames = append(frames, frame)
		pos = framePos + consumed
	}
	return frames, nil, nil
}

// findSTXInBuf 는 버퍼에서 STX 패턴의 시작 인덱스를 찾는다.
// 찾지 못하면 -1 을 반환한다.
func (f *frameFramer) findSTXInBuf(buf []byte) int {
	if len(f.stx) == 0 || len(buf) == 0 {
		return -1
	}
	for i := 0; i+len(f.stx) <= len(buf); i++ {
		match := true
		for j, b := range f.stx {
			if buf[i+j] != b {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// parseFrameAt 는 buf[0] 이 STX 의 첫 바이트인 상태로 호출되어 하나의
// 프레임을 파싱한다. 반환:
//
//	frame:    파싱 성공 시 프레임 바이트
//	consumed: 프레임에 소비된 바이트 수 (frame 길이와 동일)
//	partial:  true 이면 바이트 부족 (호출자가 더 많은 바이트를 기다려야 함)
//	err:      ETX/체크섬/크기 등 파싱 에러
func (f *frameFramer) parseFrameAt(buf []byte) (frame []byte, consumed int, partial bool, err error) {
	// STX 전체 확인
	if len(buf) < len(f.stx) {
		return nil, 0, true, nil
	}
	// header = STX + (lengthOffset - len(stx)) extra bytes + length field
	headerExtra := f.lengthOffset - len(f.stx)
	if headerExtra < 0 {
		headerExtra = 0
	}
	headerSize := len(f.stx) + headerExtra + f.lengthSize
	if len(buf) < headerSize {
		return nil, 0, true, nil
	}

	// 길이 필드 디코딩
	lenBufStart := len(f.stx) + headerExtra
	lenBuf := buf[lenBufStart : lenBufStart+f.lengthSize]
	payloadLen := f.decodeLength(lenBuf)

	if f.lengthIncludesHeader {
		payloadLen -= headerSize
		if payloadLen < 0 {
			payloadLen = 0
		}
	}
	payloadLen += f.lengthAdjustment
	if payloadLen < 0 {
		payloadLen = 0
	}

	totalSize := headerSize + payloadLen
	if f.maxMessageSize > 0 && totalSize > f.maxMessageSize {
		return nil, 0, false, fmt.Errorf("%w: %d bytes (max %d)", ErrFrameTooLarge, totalSize, f.maxMessageSize)
	}
	if len(buf) < totalSize {
		return nil, 0, true, nil
	}

	frame = make([]byte, totalSize)
	copy(frame, buf[:totalSize])

	// ETX 검증 (Read 와 동일 로직)
	if len(f.etx) > 0 {
		etxStart := len(frame) - len(f.etx)
		if f.checksum != "none" {
			etxStart -= 1
		}
		if etxStart < 0 || etxStart+len(f.etx) > len(frame) {
			return nil, 0, false, fmt.Errorf("%w", ErrETXMismatch)
		}
		for i, b := range f.etx {
			if frame[etxStart+i] != b {
				return nil, 0, false, fmt.Errorf("%w", ErrETXMismatch)
			}
		}
	}

	// 체크섬 검증
	if f.checksum != "none" {
		csIdx := len(frame) - 1
		if csIdx < 0 {
			return nil, 0, false, fmt.Errorf("%w", ErrChecksumMismatch)
		}
		expected := frame[csIdx]
		data := frame[:csIdx]
		var calculated byte
		switch f.checksum {
		case "sum8":
			calculated = checksumSum8(data)
		case "xor":
			calculated = checksumXOR(data)
		}
		if calculated != expected {
			return nil, 0, false, fmt.Errorf("%w: expected 0x%02X, got 0x%02X", ErrChecksumMismatch, expected, calculated)
		}
	}

	return frame, totalSize, false, nil
}

// findSTX 는 리더에서 바이트 단위로 읽으며 STX 패턴을 찾는다.
func (f *frameFramer) findSTX(r io.Reader, buf []byte) error {
	single := make([]byte, 1)
	matchIdx := 0

	for {
		if _, err := io.ReadFull(r, single); err != nil {
			return err
		}
		if single[0] == f.stx[matchIdx] {
			buf[matchIdx] = single[0]
			matchIdx++
			if matchIdx == len(f.stx) {
				return nil
			}
		} else {
			// 매칭 실패 — 처음부터 다시
			matchIdx = 0
			if single[0] == f.stx[0] {
				buf[0] = single[0]
				matchIdx = 1
			}
		}
	}
}

// decodeLength 는 길이 필드 바이트를 정수로 디코딩한다.
func (f *frameFramer) decodeLength(buf []byte) int {
	if f.lengthSize == 1 {
		return int(buf[0])
	}
	// 2 바이트
	if f.lengthEndian == "little" {
		return int(binary.LittleEndian.Uint16(buf))
	}
	return int(binary.BigEndian.Uint16(buf))
}

// checksumSum8 는 데이터의 바이트 합을 계산한다.
func checksumSum8(data []byte) byte {
	var sum byte
	for _, b := range data {
		sum += b
	}
	return sum
}

// checksumXOR 는 데이터의 XOR 체크섬을 계산한다.
func checksumXOR(data []byte) byte {
	var result byte
	for _, b := range data {
		result ^= b
	}
	return result
}
