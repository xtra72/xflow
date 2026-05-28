package century

// CRC16ARC 는 SPEC-CENTURY-HVACR-001 REQ-CENTURY-004 의 CRC-16/ARC 알고리즘을 구현한다.
//
// 매개변수 (CRC-16/ARC == CRC-16/IBM):
//   - Polynomial: 0x8005 (reflected 0xA001)
//   - Init:       0x0000   ← Modbus RTU 의 0xFFFF 가 아님에 유의
//   - RefIn / RefOut: true / true
//   - XorOut:     0x0000
//
// 적용 범위는 호출자가 결정한다 (보통 헤더 + payload, 트레일 CRC 2바이트 제외).
// 저장 순서는 little-endian: frame[8+N] = crc & 0xFF, frame[8+N+1] = (crc >> 8) & 0xFF.
//
// 본 구현은 프로토콜 문서 §9 의 Python 참조 구현과 비트 단위 동일하다.
func CRC16ARC(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}
