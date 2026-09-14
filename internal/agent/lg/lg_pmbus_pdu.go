package lg

import (
	"fmt"
)

// ---------------------------------------------------------------------------
// PMBUSB00A Modbus PDU 빌더 / 응답 파서
//
// SPEC-LG-HVACR-003 § M2. 순수 PDU 만 다룬다 — ADU 프레이밍(슬레이브 주소 + CRC)은
// internal/modbus 의 BuildRTUADU / ParseRTUADU 가, MBAP 은 TCP 트랜스포트가 소유한다.
//
// CRC 격리: 본 파일은 LGCP 계열 체크섬(CRC-16/XMODEM)을 참조하지 않는다. Modbus 구간의
// CRC-16/MODBUS 는 internal/modbus 가 단독으로 소유한다.
// ---------------------------------------------------------------------------

// Modbus 프로토콜 한계값.
const (
	pmbusMaxReadBits  = 2000 // FC01/02 최대 읽기 비트 수
	pmbusMaxReadRegs  = 125  // FC03/04 최대 읽기 워드 수
	pmbusMaxWriteRegs = 123  // FC16 최대 쓰기 워드 수
)

// Coil 쓰기 값 (FC05). Modbus 는 ON 을 0xFF00, OFF 를 0x0000 으로 표현한다.
const (
	pmbusCoilOn  uint16 = 0xFF00
	pmbusCoilOff uint16 = 0x0000
)

// buildReadPDU 는 읽기 요청 PDU 를 생성한다 (FC01/02/03/04).
//
//	[FC][addr hi][addr lo][qty hi][qty lo]
func buildReadPDU(fc byte, addr, quantity uint16) ([]byte, error) {
	switch fc {
	case pmbusFCReadCoils, pmbusFCReadDiscreteInputs:
		if quantity == 0 || quantity > pmbusMaxReadBits {
			return nil, fmt.Errorf("%w: bits %d", ErrHvacr03InvalidQuantity, quantity)
		}
	case pmbusFCReadHolding, pmbusFCReadInput:
		if quantity == 0 || quantity > pmbusMaxReadRegs {
			return nil, fmt.Errorf("%w: registers %d", ErrHvacr03InvalidQuantity, quantity)
		}
	default:
		return nil, fmt.Errorf("%w: read fc 0x%02X", ErrHvacr03UnexpectedFunctionCode, fc)
	}
	return []byte{
		fc,
		byte(addr >> 8), byte(addr),
		byte(quantity >> 8), byte(quantity),
	}, nil
}

// buildWriteCoilPDU 는 단일 코일 쓰기 요청 PDU 를 생성한다 (FC05).
//
//	[05][addr hi][addr lo][val hi][val lo]
func buildWriteCoilPDU(addr uint16, on bool) []byte {
	v := pmbusCoilOff
	if on {
		v = pmbusCoilOn
	}
	return []byte{
		pmbusFCWriteSingleCoil,
		byte(addr >> 8), byte(addr),
		byte(v >> 8), byte(v),
	}
}

// buildWriteRegisterPDU 는 단일 레지스터 쓰기 요청 PDU 를 생성한다 (FC06).
//
//	[06][addr hi][addr lo][val hi][val lo]
func buildWriteRegisterPDU(addr, value uint16) []byte {
	return []byte{
		pmbusFCWriteSingleReg,
		byte(addr >> 8), byte(addr),
		byte(value >> 8), byte(value),
	}
}

// buildWriteMultiplePDU 는 연속 레지스터 쓰기 요청 PDU 를 생성한다 (FC16).
//
//	[10][addr hi][addr lo][qty hi][qty lo][byte count][val...]
//
// 모드·풍량·온도를 한 트랜잭션으로 쓰기 위해 필요하다. 에어컨은 모드별로 마지막
// 온도·풍량을 기억하므로, 개별 FC06 으로 분할하면 모드 쓰기 직후 실내기가 기억된
// 값으로 온도를 되돌리는 중간 상태가 생긴다.
func buildWriteMultiplePDU(addr uint16, values []uint16) ([]byte, error) {
	n := len(values)
	if n == 0 || n > pmbusMaxWriteRegs {
		return nil, fmt.Errorf("%w: write registers %d", ErrHvacr03InvalidQuantity, n)
	}
	pdu := make([]byte, 0, 6+n*2)
	pdu = append(pdu,
		pmbusFCWriteMultipleRegs,
		byte(addr>>8), byte(addr),
		byte(n>>8), byte(n),
		byte(n*2),
	)
	for _, v := range values {
		pdu = append(pdu, byte(v>>8), byte(v))
	}
	return pdu, nil
}

// checkResponseFC 는 응답 PDU 의 function code 를 검사한다.
// 예외 응답(최상위 비트 설정)이면 *ModbusException 을 반환한다.
func checkResponseFC(pdu []byte, expectFC byte) error {
	if len(pdu) < 2 {
		return fmt.Errorf("%w: got %d bytes", ErrHvacr03ShortResponse, len(pdu))
	}
	fc := pdu[0]
	if fc == expectFC {
		return nil
	}
	if fc == expectFC|0x80 {
		return &ModbusException{FunctionCode: expectFC, Code: pdu[1]}
	}
	return fmt.Errorf("%w: expected 0x%02X, got 0x%02X", ErrHvacr03UnexpectedFunctionCode, expectFC, fc)
}

// parseBitResponse 는 FC01/02 응답 PDU 에서 비트 배열을 추출한다.
//
//	[FC][byte count][data...]
//
// 비트는 LSB 우선으로 채워진다 — data[0] 의 bit0 이 첫 번째 코일이다.
func parseBitResponse(pdu []byte, expectFC byte, quantity uint16) ([]bool, error) {
	if err := checkResponseFC(pdu, expectFC); err != nil {
		return nil, err
	}
	byteCount := int(pdu[1])
	if len(pdu) < 2+byteCount {
		return nil, fmt.Errorf("%w: declared %d bytes, got %d", ErrHvacr03ShortResponse, byteCount, len(pdu)-2)
	}
	need := (int(quantity) + 7) / 8
	if byteCount < need {
		return nil, fmt.Errorf("%w: need %d bytes for %d bits, got %d", ErrHvacr03ShortResponse, need, quantity, byteCount)
	}
	data := pdu[2 : 2+byteCount]
	bits := make([]bool, quantity)
	for i := 0; i < int(quantity); i++ {
		bits[i] = data[i/8]&(1<<(uint(i)%8)) != 0
	}
	return bits, nil
}

// parseRegisterResponse 는 FC03/04 응답 PDU 에서 워드 배열을 추출한다.
//
//	[FC][byte count][hi][lo]...
func parseRegisterResponse(pdu []byte, expectFC byte, quantity uint16) ([]uint16, error) {
	if err := checkResponseFC(pdu, expectFC); err != nil {
		return nil, err
	}
	byteCount := int(pdu[1])
	if len(pdu) < 2+byteCount {
		return nil, fmt.Errorf("%w: declared %d bytes, got %d", ErrHvacr03ShortResponse, byteCount, len(pdu)-2)
	}
	need := int(quantity) * 2
	if byteCount < need {
		return nil, fmt.Errorf("%w: need %d bytes for %d registers, got %d", ErrHvacr03ShortResponse, need, quantity, byteCount)
	}
	data := pdu[2 : 2+byteCount]
	regs := make([]uint16, quantity)
	for i := 0; i < int(quantity); i++ {
		regs[i] = uint16(data[i*2])<<8 | uint16(data[i*2+1])
	}
	return regs, nil
}

// parseWriteEchoResponse 는 FC05/06 응답의 에코를 검증한다.
// 정상 응답은 요청과 동일한 [FC][addr][value] 를 돌려준다.
func parseWriteEchoResponse(pdu []byte, expectFC byte, addr, value uint16) error {
	if err := checkResponseFC(pdu, expectFC); err != nil {
		return err
	}
	if len(pdu) < 5 {
		return fmt.Errorf("%w: write echo needs 5 bytes, got %d", ErrHvacr03ShortResponse, len(pdu))
	}
	gotAddr := uint16(pdu[1])<<8 | uint16(pdu[2])
	gotVal := uint16(pdu[3])<<8 | uint16(pdu[4])
	if gotAddr != addr || gotVal != value {
		return fmt.Errorf("lg_hvacr03: write echo mismatch (sent addr=%d val=0x%04X, got addr=%d val=0x%04X)",
			addr, value, gotAddr, gotVal)
	}
	return nil
}

// parseWriteMultipleResponse 는 FC16 응답을 검증한다.
// 정상 응답은 [10][addr hi][addr lo][qty hi][qty lo] 이다.
func parseWriteMultipleResponse(pdu []byte, addr uint16, quantity int) error {
	if err := checkResponseFC(pdu, pmbusFCWriteMultipleRegs); err != nil {
		return err
	}
	if len(pdu) < 5 {
		return fmt.Errorf("%w: write-multiple response needs 5 bytes, got %d", ErrHvacr03ShortResponse, len(pdu))
	}
	gotAddr := uint16(pdu[1])<<8 | uint16(pdu[2])
	gotQty := int(uint16(pdu[3])<<8 | uint16(pdu[4]))
	if gotAddr != addr || gotQty != quantity {
		return fmt.Errorf("lg_hvacr03: write-multiple echo mismatch (sent addr=%d qty=%d, got addr=%d qty=%d)",
			addr, quantity, gotAddr, gotQty)
	}
	return nil
}
