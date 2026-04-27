package lg

// ---------------------------------------------------------------------------
// LGCP CRC-16 계산
// ---------------------------------------------------------------------------

// CalcLGCPCRC16 은 CRC-16/XMODEM 알고리즘으로 CRC 를 계산하여 반환한다.
// 입력은 프레임의 frame[0:len-2] (STX, LEN 포함, CRC 제외) 영역이어야 한다.
//
// CRC-16/XMODEM 파라미터:
//   - 다항식: 0x1021
//   - 초깃값: 0x0000
//   - RefIn:  false
//   - RefOut: false
//   - XorOut: 0x0000
func CalcLGCPCRC16(data []byte) uint16 {
	reg := uint16(0x0000)
	for _, b := range data {
		reg ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if reg&0x8000 != 0 {
				reg = (reg << 1) ^ 0x1021
			} else {
				reg <<= 1
			}
		}
	}
	return reg
}

// VerifyLGCPCRC 는 완전한 LGCP 프레임의 CRC 가 유효한지 검증한다.
// frame 은 STX 부터 CRC 까지 포함한 전체 바이트여야 한다.
// CRC 계산 범위: frame[0:len-2] (STX, LEN 포함, CRC 제외).
// CRC 저장 형식: 빅엔디안 2바이트.
func VerifyLGCPCRC(frame []byte) bool {
	if len(frame) < 4 { // 최소: STX + LEN + CRC(2)
		return false
	}
	data := frame[0 : len(frame)-2]
	calc := CalcLGCPCRC16(data)
	stored := uint16(frame[len(frame)-2])<<8 | uint16(frame[len(frame)-1])
	return calc == stored
}
