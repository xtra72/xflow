package modbus

// ---------------------------------------------------------------------------
// Modbus RTU CRC-16
// ---------------------------------------------------------------------------
//
// 규격: 반사형(reflected) 다항식 0xA001, 초기값 0xFFFF.
// 결과 CRC 는 RTU ADU 에 리틀엔디언(CRC-lo 먼저, CRC-hi 나중)으로 부착한다.
//
// 주의: 이 알고리즘은 century 패키지의 CRC-16/ARC(init 0x0000)나
// samsung 패키지의 CCITT 와 다르다. 혼동을 피하기 위해 in-house 로 구현한다.

// rtuCRCPoly 는 Modbus RTU CRC-16 의 반사형 다항식이다.
const rtuCRCPoly uint16 = 0xA001

// rtuCRCInit 는 Modbus RTU CRC-16 의 초기값이다.
const rtuCRCInit uint16 = 0xFFFF

// modbusCRC16 은 data 에 대한 Modbus RTU CRC-16 값을 계산한다.
// 반환값은 uint16 이며, 와이어에는 리틀엔디언(byte(crc), byte(crc>>8))으로 부착한다.
func modbusCRC16(data []byte) uint16 {
	crc := rtuCRCInit
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&0x0001 != 0 {
				crc >>= 1
				crc ^= rtuCRCPoly
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}
