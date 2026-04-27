package modbusserver

import (
	"testing"

	modbus "github.com/xtra/xflow/internal/agent/modbus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 인코딩/디코딩 헬퍼 테스트
// ---------------------------------------------------------------------------

func TestEncodeCoils(t *testing.T) {
	tests := []struct {
		name     string
		values   []bool
		expected []byte
	}{
		{
			name:     "빈 슬라이스",
			values:   []bool{},
			expected: []byte{},
		},
		{
			name:     "단일 코일 ON",
			values:   []bool{true},
			expected: []byte{0x01},
		},
		{
			name:     "단일 코일 OFF",
			values:   []bool{false},
			expected: []byte{0x00},
		},
		{
			name:     "8개 코일 (1바이트 가득)",
			values:   []bool{true, false, true, false, true, false, true, false},
			expected: []byte{0x55}, // 01010101 -> LSB first: bit0=1,bit1=0,bit2=1,...
		},
		{
			name:     "8개 코일 모두 ON",
			values:   []bool{true, true, true, true, true, true, true, true},
			expected: []byte{0xFF},
		},
		{
			name:     "10개 코일 (2바이트, 패딩 포함)",
			values:   []bool{true, false, true, true, false, false, true, true, true, false},
			expected: []byte{0xCD, 0x01}, // 바이트0: 11001101=0xCD, 바이트1: 00000001=0x01
		},
		{
			name:     "3개 코일 (패딩 비트 0)",
			values:   []bool{true, true, false},
			expected: []byte{0x03}, // 00000011
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodeCoils(tt.values)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDecodeCoilBits(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		quantity int
		expected []bool
	}{
		{
			name:     "빈 데이터",
			data:     []byte{},
			quantity: 0,
			expected: []bool{},
		},
		{
			name:     "단일 코일 ON",
			data:     []byte{0x01},
			quantity: 1,
			expected: []bool{true},
		},
		{
			name:     "8개 코일",
			data:     []byte{0x55},
			quantity: 8,
			expected: []bool{true, false, true, false, true, false, true, false},
		},
		{
			name:     "10개 코일 (패딩 비트 무시)",
			data:     []byte{0xCD, 0x01},
			quantity: 10,
			expected: []bool{true, false, true, true, false, false, true, true, true, false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decodeCoilBits(tt.data, tt.quantity)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEncodeDecodeCoils_RoundTrip(t *testing.T) {
	// 다양한 패턴으로 라운드 트립 검증
	patterns := [][]bool{
		{true},
		{false, true, true, false},
		{true, true, true, true, true, true, true, true, true}, // 9개
		{false, false, false, false, false, false, false, false, true}, // 9번째만 ON
	}

	for _, original := range patterns {
		encoded := encodeCoils(original)
		decoded := decodeCoilBits(encoded, len(original))
		assert.Equal(t, original, decoded, "라운드 트립 실패: %v", original)
	}
}

func TestEncodeRegisters(t *testing.T) {
	tests := []struct {
		name     string
		values   []uint16
		expected []byte
	}{
		{
			name:     "빈 슬라이스",
			values:   []uint16{},
			expected: []byte{},
		},
		{
			name:     "단일 레지스터",
			values:   []uint16{0x0064}, // 100
			expected: []byte{0x00, 0x64},
		},
		{
			name:     "여러 레지스터 Big-Endian",
			values:   []uint16{0x0001, 0xFF00, 0x1234},
			expected: []byte{0x00, 0x01, 0xFF, 0x00, 0x12, 0x34},
		},
		{
			name:     "최대값",
			values:   []uint16{0xFFFF},
			expected: []byte{0xFF, 0xFF},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodeRegisters(tt.values)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDecodeRegisterBytes(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected []uint16
	}{
		{
			name:     "빈 데이터",
			data:     []byte{},
			expected: []uint16{},
		},
		{
			name:     "단일 레지스터",
			data:     []byte{0x00, 0x64},
			expected: []uint16{0x0064},
		},
		{
			name:     "여러 레지스터",
			data:     []byte{0x00, 0x01, 0xFF, 0x00, 0x12, 0x34},
			expected: []uint16{0x0001, 0xFF00, 0x1234},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decodeRegisterBytes(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEncodeDecodeRegisters_RoundTrip(t *testing.T) {
	original := []uint16{100, 200, 300, 0xFFFF, 0}
	encoded := encodeRegisters(original)
	decoded := decodeRegisterBytes(encoded)
	assert.Equal(t, original, decoded)
}

// ---------------------------------------------------------------------------
// RequestHandler 테스트 헬퍼
// ---------------------------------------------------------------------------

// newTestRegisterMap 는 테스트용 RegisterMap 을 생성한다.
func newTestRegisterMap() *RegisterMap {
	cfg := RegisterMapConfig{
		Coils: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        100,
		}},
		DiscreteInputs: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        100,
		}},
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        100,
		}},
		InputRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        100,
		}},
	}
	return NewRegisterMap(cfg)
}

// buildReadPDU 는 읽기 요청 PDU 를 생성한다 (FC + StartAddr + Quantity).
func buildReadPDU(fc byte, startAddr, quantity uint16) []byte {
	pdu := make([]byte, 5)
	pdu[0] = fc
	pdu[1] = byte(startAddr >> 8)
	pdu[2] = byte(startAddr)
	pdu[3] = byte(quantity >> 8)
	pdu[4] = byte(quantity)
	return pdu
}

// ---------------------------------------------------------------------------
// FC01 ReadCoils 테스트
// ---------------------------------------------------------------------------

func TestRequestHandler_ReadCoils(t *testing.T) {
	rm := newTestRegisterMap()
	// 주소 0~2 에 코일 값을 설정
	rm.WriteCoils(0, []bool{true, false, true})

	handler := NewRequestHandler(rm, nil)

	tests := []struct {
		name           string
		pdu            []byte
		expectFC       byte
		expectDataLen  int
		expectFirstBit bool // 응답 데이터의 첫 번째 코일 값
	}{
		{
			name:           "3개 코일 읽기",
			pdu:            buildReadPDU(modbus.FC01ReadCoils, 0, 3),
			expectFC:       modbus.FC01ReadCoils,
			expectDataLen:  3, // FC(1) + ByteCount(1) + Data(1) = 3
			expectFirstBit: true,
		},
		{
			name:          "10개 코일 읽기 (2바이트 응답)",
			pdu:           buildReadPDU(modbus.FC01ReadCoils, 0, 10),
			expectFC:      modbus.FC01ReadCoils,
			expectDataLen: 4, // FC(1) + ByteCount(1) + Data(2) = 4
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := handler.HandleRequest(tt.pdu)
			require.NotNil(t, resp)
			assert.Equal(t, tt.expectFC, resp[0])
			assert.Len(t, resp, tt.expectDataLen)
		})
	}
}

// ---------------------------------------------------------------------------
// FC02 ReadDiscreteInputs 테스트
// ---------------------------------------------------------------------------

func TestRequestHandler_ReadDiscreteInputs(t *testing.T) {
	rm := newTestRegisterMap()
	rm.WriteDiscreteInputs(0, []bool{true, true, false})

	handler := NewRequestHandler(rm, nil)
	pdu := buildReadPDU(modbus.FC02ReadDiscreteInputs, 0, 3)
	resp := handler.HandleRequest(pdu)

	require.NotNil(t, resp)
	assert.Equal(t, modbus.FC02ReadDiscreteInputs, resp[0])
	assert.Equal(t, byte(1), resp[1]) // byteCount = 1
	// 비트: bit0=1, bit1=1, bit2=0 -> 0x03
	assert.Equal(t, byte(0x03), resp[2])
}

// ---------------------------------------------------------------------------
// FC03 ReadHoldingRegisters 테스트
// ---------------------------------------------------------------------------

func TestRequestHandler_ReadHoldingRegisters(t *testing.T) {
	rm := newTestRegisterMap()
	rm.WriteHoldingRegisters(0, []uint16{100, 200, 300})

	handler := NewRequestHandler(rm, nil)

	tests := []struct {
		name       string
		start      uint16
		quantity   uint16
		expectLen  int
		expectVals []uint16
	}{
		{
			name:       "3개 레지스터 읽기",
			start:      0,
			quantity:   3,
			expectLen:  8, // FC(1) + ByteCount(1) + Data(6)
			expectVals: []uint16{100, 200, 300},
		},
		{
			name:       "1개 레지스터 읽기",
			start:      1,
			quantity:   1,
			expectLen:  4, // FC(1) + ByteCount(1) + Data(2)
			expectVals: []uint16{200},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdu := buildReadPDU(modbus.FC03ReadHoldingRegisters, tt.start, tt.quantity)
			resp := handler.HandleRequest(pdu)
			require.NotNil(t, resp)
			assert.Equal(t, modbus.FC03ReadHoldingRegisters, resp[0])
			assert.Len(t, resp, tt.expectLen)

			// 레지스터 데이터 디코딩 검증
			byteCount := int(resp[1])
			decoded := decodeRegisterBytes(resp[2 : 2+byteCount])
			assert.Equal(t, tt.expectVals, decoded)
		})
	}
}

// ---------------------------------------------------------------------------
// FC04 ReadInputRegisters 테스트
// ---------------------------------------------------------------------------

func TestRequestHandler_ReadInputRegisters(t *testing.T) {
	rm := newTestRegisterMap()
	rm.WriteInputRegisters(0, []uint16{500, 600})

	handler := NewRequestHandler(rm, nil)
	pdu := buildReadPDU(modbus.FC04ReadInputRegisters, 0, 2)
	resp := handler.HandleRequest(pdu)

	require.NotNil(t, resp)
	assert.Equal(t, modbus.FC04ReadInputRegisters, resp[0])
	byteCount := int(resp[1])
	assert.Equal(t, 4, byteCount)
	decoded := decodeRegisterBytes(resp[2 : 2+byteCount])
	assert.Equal(t, []uint16{500, 600}, decoded)
}

// ---------------------------------------------------------------------------
// FC05 WriteSingleCoil 테스트
// ---------------------------------------------------------------------------

func TestRequestHandler_WriteSingleCoil(t *testing.T) {
	rm := newTestRegisterMap()
	handler := NewRequestHandler(rm, nil)

	tests := []struct {
		name        string
		pdu         []byte
		expectCoil  bool
		expectError bool
	}{
		{
			name:       "코일 ON (0xFF00)",
			pdu:        []byte{modbus.FC05WriteSingleCoil, 0x00, 0x00, 0xFF, 0x00},
			expectCoil: true,
		},
		{
			name:       "코일 OFF (0x0000)",
			pdu:        []byte{modbus.FC05WriteSingleCoil, 0x00, 0x00, 0x00, 0x00},
			expectCoil: false,
		},
		{
			name:        "잘못된 값 (0x1234) → 예외 응답",
			pdu:         []byte{modbus.FC05WriteSingleCoil, 0x00, 0x00, 0x12, 0x34},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, _ := handler.HandleWriteRequest(tt.pdu, "127.0.0.1:9999")
			require.NotNil(t, resp)

			if tt.expectError {
				// 예외 응답: FC|0x80 + ExceptionCode
				assert.Equal(t, modbus.FC05WriteSingleCoil|0x80, resp[0])
				assert.Equal(t, modbus.ExceptionIllegalDataValue, resp[1])
			} else {
				// 에코백 응답 검증
				assert.Equal(t, tt.pdu, resp)
				// 레지스터 맵에 반영 확인
				vals, err := rm.ReadCoils(0, 1)
				require.NoError(t, err)
				assert.Equal(t, tt.expectCoil, vals[0])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FC06 WriteSingleRegister 테스트
// ---------------------------------------------------------------------------

func TestRequestHandler_WriteSingleRegister(t *testing.T) {
	rm := newTestRegisterMap()
	handler := NewRequestHandler(rm, nil)

	// FC06: addr=0x0005, value=0x1234
	pdu := []byte{modbus.FC06WriteSingleRegister, 0x00, 0x05, 0x12, 0x34}
	resp, cs := handler.HandleWriteRequest(pdu, "127.0.0.1:9999")

	require.NotNil(t, resp)
	// 에코백
	assert.Equal(t, pdu, resp)
	// 레지스터 값 확인
	vals, err := rm.ReadHoldingRegisters(5, 1)
	require.NoError(t, err)
	assert.Equal(t, uint16(0x1234), vals[0])
	// ChangeSet 확인
	require.NotNil(t, cs)
	assert.Equal(t, "holding_registers", cs.Area)
}

// ---------------------------------------------------------------------------
// FC15 WriteMultipleCoils 테스트
// ---------------------------------------------------------------------------

func TestRequestHandler_WriteMultipleCoils(t *testing.T) {
	rm := newTestRegisterMap()
	handler := NewRequestHandler(rm, nil)

	// FC15: addr=0, quantity=10, byteCount=2, data=[0xCD, 0x01]
	// 코일: 1,0,1,1,0,0,1,1, 1,0
	pdu := []byte{
		modbus.FC15WriteMultipleCoils,
		0x00, 0x00, // start address
		0x00, 0x0A, // quantity = 10
		0x02,       // byte count
		0xCD, 0x01, // coil data
	}

	resp, cs := handler.HandleWriteRequest(pdu, "127.0.0.1:9999")
	require.NotNil(t, resp)

	// 응답: FC(1) + StartAddr(2) + Quantity(2) = 5 바이트
	assert.Len(t, resp, 5)
	assert.Equal(t, modbus.FC15WriteMultipleCoils, resp[0])
	assert.Equal(t, byte(0x00), resp[1]) // start addr high
	assert.Equal(t, byte(0x00), resp[2]) // start addr low
	assert.Equal(t, byte(0x00), resp[3]) // quantity high
	assert.Equal(t, byte(0x0A), resp[4]) // quantity low

	// 레지스터 맵 확인
	vals, err := rm.ReadCoils(0, 10)
	require.NoError(t, err)
	expected := []bool{true, false, true, true, false, false, true, true, true, false}
	assert.Equal(t, expected, vals)

	require.NotNil(t, cs)
	assert.Equal(t, "coils", cs.Area)
}

// ---------------------------------------------------------------------------
// FC16 WriteMultipleRegisters 테스트
// ---------------------------------------------------------------------------

func TestRequestHandler_WriteMultipleRegisters(t *testing.T) {
	rm := newTestRegisterMap()
	handler := NewRequestHandler(rm, nil)

	// FC16: addr=0, quantity=3, byteCount=6, values=[100, 200, 300]
	pdu := []byte{
		modbus.FC16WriteMultipleRegisters,
		0x00, 0x00, // start address
		0x00, 0x03, // quantity = 3
		0x06,                   // byte count
		0x00, 0x64, // 100
		0x00, 0xC8, // 200
		0x01, 0x2C, // 300
	}

	resp, cs := handler.HandleWriteRequest(pdu, "127.0.0.1:9999")
	require.NotNil(t, resp)

	// 응답: FC(1) + StartAddr(2) + Quantity(2) = 5 바이트
	assert.Len(t, resp, 5)
	assert.Equal(t, modbus.FC16WriteMultipleRegisters, resp[0])

	// 레지스터 맵 확인
	vals, err := rm.ReadHoldingRegisters(0, 3)
	require.NoError(t, err)
	assert.Equal(t, []uint16{100, 200, 300}, vals)

	require.NotNil(t, cs)
	assert.Equal(t, "holding_registers", cs.Area)
}

// ---------------------------------------------------------------------------
// 예외 응답 테스트
// ---------------------------------------------------------------------------

func TestRequestHandler_IllegalFunction(t *testing.T) {
	rm := newTestRegisterMap()
	handler := NewRequestHandler(rm, nil)

	// 지원하지 않는 FC (0x07)
	pdu := buildReadPDU(0x07, 0, 1)
	resp := handler.HandleRequest(pdu)

	require.NotNil(t, resp)
	assert.Equal(t, byte(0x07|0x80), resp[0])
	assert.Equal(t, modbus.ExceptionIllegalFunction, resp[1])
}

func TestRequestHandler_IllegalDataAddress(t *testing.T) {
	rm := newTestRegisterMap()
	handler := NewRequestHandler(rm, nil)

	// 매핑 범위(0~99) 밖의 주소 읽기 시도
	pdu := buildReadPDU(modbus.FC03ReadHoldingRegisters, 200, 1)
	resp := handler.HandleRequest(pdu)

	require.NotNil(t, resp)
	assert.Equal(t, byte(modbus.FC03ReadHoldingRegisters|0x80), resp[0])
	assert.Equal(t, modbus.ExceptionIllegalDataAddress, resp[1])
}

func TestRequestHandler_QuantityZero(t *testing.T) {
	rm := newTestRegisterMap()
	handler := NewRequestHandler(rm, nil)

	// quantity = 0
	pdu := buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 0)
	resp := handler.HandleRequest(pdu)

	require.NotNil(t, resp)
	assert.Equal(t, byte(modbus.FC03ReadHoldingRegisters|0x80), resp[0])
	assert.Equal(t, modbus.ExceptionIllegalDataValue, resp[1])
}

func TestRequestHandler_QuantityExceeded(t *testing.T) {
	tests := []struct {
		name     string
		fc       byte
		quantity uint16
	}{
		{
			name:     "코일 읽기 최대 초과 (>2000)",
			fc:       modbus.FC01ReadCoils,
			quantity: 2001,
		},
		{
			name:     "레지스터 읽기 최대 초과 (>125)",
			fc:       modbus.FC03ReadHoldingRegisters,
			quantity: 126,
		},
		{
			name:     "이산 입력 읽기 최대 초과 (>2000)",
			fc:       modbus.FC02ReadDiscreteInputs,
			quantity: 2001,
		},
		{
			name:     "입력 레지스터 읽기 최대 초과 (>125)",
			fc:       modbus.FC04ReadInputRegisters,
			quantity: 126,
		},
	}

	rm := newTestRegisterMap()
	handler := NewRequestHandler(rm, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdu := buildReadPDU(tt.fc, 0, tt.quantity)
			resp := handler.HandleRequest(pdu)

			require.NotNil(t, resp)
			assert.Equal(t, tt.fc|0x80, resp[0])
			assert.Equal(t, modbus.ExceptionIllegalDataValue, resp[1])
		})
	}
}

func TestRequestHandler_ShortPDU(t *testing.T) {
	rm := newTestRegisterMap()
	handler := NewRequestHandler(rm, nil)

	// PDU 가 너무 짧음 (최소 5바이트: FC + StartAddr(2) + Quantity(2))
	resp := handler.HandleRequest([]byte{modbus.FC03ReadHoldingRegisters, 0x00})

	require.NotNil(t, resp)
	assert.Equal(t, byte(modbus.FC03ReadHoldingRegisters|0x80), resp[0])
}

func TestRequestHandler_EmptyPDU(t *testing.T) {
	rm := newTestRegisterMap()
	handler := NewRequestHandler(rm, nil)

	// 빈 PDU
	resp := handler.HandleRequest([]byte{})
	require.Nil(t, resp)
}

// ---------------------------------------------------------------------------
// makeExceptionPDU 테스트
// ---------------------------------------------------------------------------

func TestMakeExceptionPDU(t *testing.T) {
	result := makeExceptionPDU(0x03, modbus.ExceptionIllegalDataAddress)
	assert.Equal(t, []byte{0x83, 0x02}, result)
}
