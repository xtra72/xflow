package samsung

import "encoding/binary"

// CalcCRC16 calculates CRC16-CCITT (polynomial 0x1021, init 0x0000) over the given data.
// This is the CRC-16/XMODEM variant used by the Samsung NASA HVAC protocol.
func CalcCRC16(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// VerifyCRC16 checks if the CRC16 at the end of data matches the calculated CRC.
// data includes the 2-byte CRC in Big-Endian at the end.
// Returns false if data is shorter than 2 bytes.
func VerifyCRC16(data []byte) bool {
	if len(data) < 2 {
		return false
	}
	payload := data[:len(data)-2]
	expected := binary.BigEndian.Uint16(data[len(data)-2:])
	return CalcCRC16(payload) == expected
}
