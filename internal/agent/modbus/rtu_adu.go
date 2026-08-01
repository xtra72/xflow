package modbus

// ---------------------------------------------------------------------------
// Modbus RTU ADU 프레이밍
// ---------------------------------------------------------------------------
//
// RTU ADU 형식: [unitID][PDU...][CRC-lo][CRC-hi]
//
// TCP(MBAP)와 RTU 는 동일한 PDU 빌더(protocol.go)를 공유하며, ADU 계층만
// 다르다. 이 파일은 순수 CRC/프레이밍 로직만 담으며 시리얼 I/O 는 포함하지
// 않는다(시리얼 마스터는 후속 마일스톤 M3 에서 이 로직을 배선한다).

// rtuADUMinLen 은 유효한 RTU ADU 의 최소 길이이다: unitID(1) + FC(1) + CRC(2).
const rtuADUMinLen = 4

// buildRTUADU 는 unitID 와 순수 PDU 로부터 RTU ADU 프레임을 생성한다.
// 결과: [unitID][PDU...][CRC-lo][CRC-hi] (CRC 는 리틀엔디언).
func buildRTUADU(unitID byte, pdu []byte) []byte {
	adu := make([]byte, 0, 1+len(pdu)+2)
	adu = append(adu, unitID)
	adu = append(adu, pdu...)

	crc := modbusCRC16(adu)
	adu = append(adu, byte(crc&0xFF), byte(crc>>8)) // 리틀엔디언: lo 먼저

	return adu
}

// parseRTUADU 는 수신한 RTU 응답 ADU 를 검증하고 순수 응답 PDU([FC][payload...])를
// 반환한다. CRC 재계산·검증, unitID 일치, function code 일관성을 확인한다.
// CRC 불일치·손상 프레임은 오류로 반환하며 유효한 결과로 상위에 넘기지 않는다(AC-07).
//
// expectUnitID 는 요청에 사용한 unitID, requestFC 는 요청 function code 이다.
// 예외 응답(requestFC|0x80)은 유효한 프레임으로 통과시키고, 상위 파서
// (parseReadResponse/parseWriteResponse)가 ModbusException 을 감지하도록 한다.
func parseRTUADU(adu []byte, expectUnitID byte, requestFC byte) ([]byte, error) {
	if len(adu) < rtuADUMinLen {
		return nil, ErrRTUFrameTooShort
	}

	// CRC 검증: 마지막 2바이트가 리틀엔디언 CRC, 나머지에 대해 재계산.
	dataLen := len(adu) - 2
	wantCRC := uint16(adu[dataLen]) | uint16(adu[dataLen+1])<<8
	if gotCRC := modbusCRC16(adu[:dataLen]); wantCRC != gotCRC {
		return nil, ErrCRCMismatch
	}

	// unitID 일치 확인.
	if adu[0] != expectUnitID {
		return nil, ErrUnitIDMismatch
	}

	// function code 일관성: 정상(requestFC) 또는 예외(requestFC|0x80)만 허용.
	respFC := adu[1]
	if respFC != requestFC && respFC != (requestFC|0x80) {
		return nil, ErrFunctionCodeMismatch
	}

	// 순수 응답 PDU = unitID 와 CRC 를 제외한 [FC][payload...].
	pdu := make([]byte, dataLen-1)
	copy(pdu, adu[1:dataLen])
	return pdu, nil
}
