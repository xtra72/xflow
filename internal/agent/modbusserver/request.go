package modbusserver

import (
	"encoding/binary"
	"errors"
	"log/slog"

	modbus "github.com/xtra/xflow/internal/agent/modbus"
)

// ExceptionGatewayTargetFailed 는 게이트웨이가 백킹 대상(upstream) 디바이스로부터
// 응답을 받지 못했음을 나타내는 MODBUS 예외 코드(0x0B)이다. internal/agent/modbus 를
// 변경하지 않기 위해 modbusserver 패키지에 로컬 상수로 정의한다(REQ-MODBUS-010-05).
const ExceptionGatewayTargetFailed byte = 0x0B

// mapStoreError 는 registerStore 가 반환한 오류를 MODBUS 예외 PDU 로 매핑한다.
// 백킹 실패(ErrGatewayTargetFailed)는 0x0B 로, 그 외(주소 미매핑 등)는 기존과 동일하게
// 0x02(Illegal Data Address)로 매핑한다. 순수 slave 경로는 ErrGatewayTargetFailed 를
// 반환하지 않으므로 기존 동작이 바이트 단위로 보존된다(하위 호환 HARD).
func mapStoreError(fc byte, err error) []byte {
	if errors.Is(err, ErrGatewayTargetFailed) {
		return makeExceptionPDU(fc, ExceptionGatewayTargetFailed)
	}
	return makeExceptionPDU(fc, modbus.ExceptionIllegalDataAddress)
}

// ---------------------------------------------------------------------------
// RequestHandler
// ---------------------------------------------------------------------------

// registerStore 는 RequestHandler 가 와이어 요청을 서빙하기 위해 사용하는 읽기/쓰기
// 인터페이스이다. *RegisterMap(로컬 전용 디바이스) 와 *deviceView(공유 세그먼트 주소
// 변환) 가 모두 이를 만족한다.
type registerStore interface {
	ReadCoils(start, quantity uint16) ([]bool, error)
	ReadDiscreteInputs(start, quantity uint16) ([]bool, error)
	ReadHoldingRegisters(start, quantity uint16) ([]uint16, error)
	ReadInputRegisters(start, quantity uint16) ([]uint16, error)
	WriteCoils(start uint16, values []bool) (*ChangeSet, error)
	WriteHoldingRegisters(start uint16, values []uint16) (*ChangeSet, error)
}

// 컴파일 타임 인터페이스 체크: *RegisterMap 은 registerStore 를 만족한다.
var _ registerStore = (*RegisterMap)(nil)

// RequestHandler processes MODBUS PDU requests against a registerStore.
type RequestHandler struct {
	store  registerStore
	logger *slog.Logger
}

// NewRequestHandler creates a new RequestHandler backed by a *RegisterMap.
func NewRequestHandler(rm *RegisterMap, logger *slog.Logger) *RequestHandler {
	return &RequestHandler{
		store:  rm,
		logger: logger,
	}
}

// newRequestHandlerWithStore 는 임의의 registerStore(예: *deviceView)로 RequestHandler 를 만든다.
func newRequestHandlerWithStore(store registerStore, logger *slog.Logger) *RequestHandler {
	return &RequestHandler{
		store:  store,
		logger: logger,
	}
}

// effectiveRegisterView 는 이 핸들러가 실제로 서빙하는 store 기준의 실효 레지스터 스냅샷과
// 영역별 카운트를 반환한다(get_device_status 노출용, SPEC-MODBUS-008). store 종류별로:
//   - *deviceView : 공유 세그먼트를 컨테이너 값으로 해석한 실효 맵(디바이스 자체 맵은
//     로컬 세그먼트만 담으므로 공유 값이 누락되는 버그를 교정).
//   - *backedStore: 폴/조회 값을 담는 inner 맵(마스터가 실제로 읽는 값).
//   - *RegisterMap: 로컬 맵(로컬 전용 디바이스 — 기존 동작과 바이트 동일).
//
// 스냅샷/카운트 형태는 RegisterMap.GetSnapshot/RegisterCounts 와 동일하여 프런트엔드가
// get_map 소비에 쓰는 형태를 그대로 유지한다(응답 스키마 불변, 내용만 실효화).
func (rh *RequestHandler) effectiveRegisterView() (snapshot map[string]any, counts map[string]int) {
	switch s := rh.store.(type) {
	case *deviceView:
		return s.EffectiveSnapshot(), s.EffectiveCounts()
	case *backedStore:
		return s.inner.GetSnapshot(), s.inner.RegisterCounts()
	case *RegisterMap:
		return s.GetSnapshot(), s.RegisterCounts()
	default:
		// 알 수 없는 store: 빈 스냅샷/카운트(방어적, 정상 경로에서는 도달하지 않음).
		return map[string]any{}, map[string]int{}
	}
}

// ---------------------------------------------------------------------------
// Read request handling
// ---------------------------------------------------------------------------

// HandleRequest processes a read-type PDU and returns the response PDU.
// Supported function codes: FC01, FC02, FC03, FC04.
// For unsupported or invalid requests, an exception PDU is returned.
// An empty PDU returns nil.
func (rh *RequestHandler) HandleRequest(pdu []byte) []byte {
	if len(pdu) == 0 {
		return nil
	}

	fc := pdu[0]

	// PDU must be at least 5 bytes: FC(1) + StartAddr(2) + Quantity(2)
	if len(pdu) < 5 {
		return makeExceptionPDU(fc, modbus.ExceptionIllegalDataValue)
	}

	startAddr := binary.BigEndian.Uint16(pdu[1:3])
	quantity := binary.BigEndian.Uint16(pdu[3:5])

	switch fc {
	case modbus.FC01ReadCoils:
		return rh.handleReadCoils(fc, startAddr, quantity)
	case modbus.FC02ReadDiscreteInputs:
		return rh.handleReadDiscreteInputs(fc, startAddr, quantity)
	case modbus.FC03ReadHoldingRegisters:
		return rh.handleReadHoldingRegisters(fc, startAddr, quantity)
	case modbus.FC04ReadInputRegisters:
		return rh.handleReadInputRegisters(fc, startAddr, quantity)
	default:
		return makeExceptionPDU(fc, modbus.ExceptionIllegalFunction)
	}
}

// handleReadCoils processes FC01 ReadCoils.
func (rh *RequestHandler) handleReadCoils(fc byte, startAddr, quantity uint16) []byte {
	if quantity == 0 || quantity > modbus.MaxCoilsRead {
		return makeExceptionPDU(fc, modbus.ExceptionIllegalDataValue)
	}

	vals, err := rh.store.ReadCoils(startAddr, quantity)
	if err != nil {
		return mapStoreError(fc, err)
	}

	data := encodeCoils(vals)
	resp := make([]byte, 2+len(data))
	resp[0] = fc
	resp[1] = byte(len(data))
	copy(resp[2:], data)
	return resp
}

// handleReadDiscreteInputs processes FC02 ReadDiscreteInputs.
func (rh *RequestHandler) handleReadDiscreteInputs(fc byte, startAddr, quantity uint16) []byte {
	if quantity == 0 || quantity > modbus.MaxCoilsRead {
		return makeExceptionPDU(fc, modbus.ExceptionIllegalDataValue)
	}

	vals, err := rh.store.ReadDiscreteInputs(startAddr, quantity)
	if err != nil {
		return mapStoreError(fc, err)
	}

	data := encodeCoils(vals)
	resp := make([]byte, 2+len(data))
	resp[0] = fc
	resp[1] = byte(len(data))
	copy(resp[2:], data)
	return resp
}

// handleReadHoldingRegisters processes FC03 ReadHoldingRegisters.
func (rh *RequestHandler) handleReadHoldingRegisters(fc byte, startAddr, quantity uint16) []byte {
	if quantity == 0 || quantity > modbus.MaxRegistersRead {
		return makeExceptionPDU(fc, modbus.ExceptionIllegalDataValue)
	}

	vals, err := rh.store.ReadHoldingRegisters(startAddr, quantity)
	if err != nil {
		return mapStoreError(fc, err)
	}

	data := encodeRegisters(vals)
	resp := make([]byte, 2+len(data))
	resp[0] = fc
	resp[1] = byte(len(data))
	copy(resp[2:], data)
	return resp
}

// handleReadInputRegisters processes FC04 ReadInputRegisters.
func (rh *RequestHandler) handleReadInputRegisters(fc byte, startAddr, quantity uint16) []byte {
	if quantity == 0 || quantity > modbus.MaxRegistersRead {
		return makeExceptionPDU(fc, modbus.ExceptionIllegalDataValue)
	}

	vals, err := rh.store.ReadInputRegisters(startAddr, quantity)
	if err != nil {
		return mapStoreError(fc, err)
	}

	data := encodeRegisters(vals)
	resp := make([]byte, 2+len(data))
	resp[0] = fc
	resp[1] = byte(len(data))
	copy(resp[2:], data)
	return resp
}

// ---------------------------------------------------------------------------
// Write request handling
// ---------------------------------------------------------------------------

// HandleWriteRequest processes a write-type PDU and returns the response PDU
// along with a ChangeSet if register values were modified.
// Supported function codes: FC05, FC06, FC15, FC16.
func (rh *RequestHandler) HandleWriteRequest(pdu []byte, remoteAddr string) ([]byte, *ChangeSet) {
	if len(pdu) == 0 {
		return nil, nil
	}

	fc := pdu[0]

	switch fc {
	case modbus.FC05WriteSingleCoil:
		return rh.handleWriteSingleCoil(pdu)
	case modbus.FC06WriteSingleRegister:
		return rh.handleWriteSingleRegister(pdu)
	case modbus.FC15WriteMultipleCoils:
		return rh.handleWriteMultipleCoils(pdu)
	case modbus.FC16WriteMultipleRegisters:
		return rh.handleWriteMultipleRegisters(pdu)
	default:
		return makeExceptionPDU(fc, modbus.ExceptionIllegalFunction), nil
	}
}

// handleWriteSingleCoil processes FC05 WriteSingleCoil.
func (rh *RequestHandler) handleWriteSingleCoil(pdu []byte) ([]byte, *ChangeSet) {
	fc := pdu[0]
	if len(pdu) < 5 {
		return makeExceptionPDU(fc, modbus.ExceptionIllegalDataValue), nil
	}

	addr := binary.BigEndian.Uint16(pdu[1:3])
	value := binary.BigEndian.Uint16(pdu[3:5])

	var coilVal bool
	switch value {
	case 0xFF00:
		coilVal = true
	case 0x0000:
		coilVal = false
	default:
		return makeExceptionPDU(fc, modbus.ExceptionIllegalDataValue), nil
	}

	cs, err := rh.store.WriteCoils(addr, []bool{coilVal})
	if err != nil {
		return mapStoreError(fc, err), nil
	}

	// Echo-back the request PDU as response
	resp := make([]byte, 5)
	copy(resp, pdu[:5])
	return resp, cs
}

// handleWriteSingleRegister processes FC06 WriteSingleRegister.
func (rh *RequestHandler) handleWriteSingleRegister(pdu []byte) ([]byte, *ChangeSet) {
	fc := pdu[0]
	if len(pdu) < 5 {
		return makeExceptionPDU(fc, modbus.ExceptionIllegalDataValue), nil
	}

	addr := binary.BigEndian.Uint16(pdu[1:3])
	value := binary.BigEndian.Uint16(pdu[3:5])

	cs, err := rh.store.WriteHoldingRegisters(addr, []uint16{value})
	if err != nil {
		return mapStoreError(fc, err), nil
	}

	// Echo-back the request PDU as response
	resp := make([]byte, 5)
	copy(resp, pdu[:5])
	return resp, cs
}

// handleWriteMultipleCoils processes FC15 WriteMultipleCoils.
func (rh *RequestHandler) handleWriteMultipleCoils(pdu []byte) ([]byte, *ChangeSet) {
	fc := pdu[0]
	if len(pdu) < 6 {
		return makeExceptionPDU(fc, modbus.ExceptionIllegalDataValue), nil
	}

	startAddr := binary.BigEndian.Uint16(pdu[1:3])
	quantity := binary.BigEndian.Uint16(pdu[3:5])
	byteCount := int(pdu[5])

	if quantity == 0 || quantity > modbus.MaxCoilsWrite {
		return makeExceptionPDU(fc, modbus.ExceptionIllegalDataValue), nil
	}

	expectedBytes := int((quantity + 7) / 8)
	if byteCount != expectedBytes || len(pdu) < 6+byteCount {
		return makeExceptionPDU(fc, modbus.ExceptionIllegalDataValue), nil
	}

	coilData := pdu[6 : 6+byteCount]
	values := decodeCoilBits(coilData, int(quantity))

	cs, err := rh.store.WriteCoils(startAddr, values)
	if err != nil {
		return mapStoreError(fc, err), nil
	}

	// Response: FC + StartAddr(2) + Quantity(2) = 5 bytes
	resp := make([]byte, 5)
	resp[0] = fc
	binary.BigEndian.PutUint16(resp[1:3], startAddr)
	binary.BigEndian.PutUint16(resp[3:5], quantity)
	return resp, cs
}

// handleWriteMultipleRegisters processes FC16 WriteMultipleRegisters.
func (rh *RequestHandler) handleWriteMultipleRegisters(pdu []byte) ([]byte, *ChangeSet) {
	fc := pdu[0]
	if len(pdu) < 6 {
		return makeExceptionPDU(fc, modbus.ExceptionIllegalDataValue), nil
	}

	startAddr := binary.BigEndian.Uint16(pdu[1:3])
	quantity := binary.BigEndian.Uint16(pdu[3:5])
	byteCount := int(pdu[5])

	if quantity == 0 || quantity > modbus.MaxRegistersWrite {
		return makeExceptionPDU(fc, modbus.ExceptionIllegalDataValue), nil
	}

	expectedBytes := int(quantity) * 2
	if byteCount != expectedBytes || len(pdu) < 6+byteCount {
		return makeExceptionPDU(fc, modbus.ExceptionIllegalDataValue), nil
	}

	regData := pdu[6 : 6+byteCount]
	values := decodeRegisterBytes(regData)

	cs, err := rh.store.WriteHoldingRegisters(startAddr, values)
	if err != nil {
		return mapStoreError(fc, err), nil
	}

	// Response: FC + StartAddr(2) + Quantity(2) = 5 bytes
	resp := make([]byte, 5)
	resp[0] = fc
	binary.BigEndian.PutUint16(resp[1:3], startAddr)
	binary.BigEndian.PutUint16(resp[3:5], quantity)
	return resp, cs
}

// ---------------------------------------------------------------------------
// Encoding / decoding helpers
// ---------------------------------------------------------------------------

// encodeCoils encodes a bool slice into packed bytes (LSB first).
// Each byte contains up to 8 coil values, starting from bit 0 (LSB).
func encodeCoils(values []bool) []byte {
	if len(values) == 0 {
		return []byte{}
	}

	byteCount := (len(values) + 7) / 8
	result := make([]byte, byteCount)

	for i, v := range values {
		if v {
			byteIdx := i / 8
			bitIdx := uint(i % 8)
			result[byteIdx] |= 1 << bitIdx
		}
	}

	return result
}

// decodeCoilBits decodes packed bytes into a bool slice, respecting quantity
// to trim padding bits.
func decodeCoilBits(data []byte, quantity int) []bool {
	if quantity == 0 {
		return []bool{}
	}

	result := make([]bool, quantity)
	for i := 0; i < quantity; i++ {
		byteIdx := i / 8
		bitIdx := uint(i % 8)
		if byteIdx < len(data) {
			result[i] = data[byteIdx]&(1<<bitIdx) != 0
		}
	}

	return result
}

// encodeRegisters encodes a uint16 slice into Big-Endian bytes.
func encodeRegisters(values []uint16) []byte {
	if len(values) == 0 {
		return []byte{}
	}

	result := make([]byte, len(values)*2)
	for i, v := range values {
		binary.BigEndian.PutUint16(result[i*2:i*2+2], v)
	}

	return result
}

// decodeRegisterBytes decodes Big-Endian bytes into a uint16 slice.
func decodeRegisterBytes(data []byte) []uint16 {
	if len(data) == 0 {
		return []uint16{}
	}

	count := len(data) / 2
	result := make([]uint16, count)
	for i := 0; i < count; i++ {
		result[i] = binary.BigEndian.Uint16(data[i*2 : i*2+2])
	}

	return result
}

// ---------------------------------------------------------------------------
// Exception helper
// ---------------------------------------------------------------------------

// makeExceptionPDU creates a MODBUS exception response PDU.
// The response is [fc|0x80, exCode].
func makeExceptionPDU(fc byte, exCode byte) []byte {
	return []byte{fc | 0x80, exCode}
}
