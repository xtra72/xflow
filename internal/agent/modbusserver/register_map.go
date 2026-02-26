package modbusserver

import (
	"sync"
)

// ---------------------------------------------------------------------------
// 기능 코드 상수 (modbus 패키지의 unexported 상수를 로컬로 재정의)
// ---------------------------------------------------------------------------

const (
	fcReadCoils              byte = 0x01
	fcReadDiscreteInputs     byte = 0x02
	fcReadHoldingRegisters   byte = 0x03
	fcReadInputRegisters     byte = 0x04
	fcWriteSingleCoil        byte = 0x05
	fcWriteSingleRegister    byte = 0x06
	fcWriteMultipleCoils     byte = 0x0F
	fcWriteMultipleRegisters byte = 0x10
)

// ---------------------------------------------------------------------------
// 타입 정의
// ---------------------------------------------------------------------------

// AddressRange 는 레지스터 주소 범위를 나타낸다.
type AddressRange struct {
	Start uint16
	Count uint16
}

// ChangeSet 는 레지스터 값 변경 내역을 나타낸다.
type ChangeSet struct {
	Area      string // "coils", "discrete_inputs", "holding_registers", "input_registers"
	Address   uint16
	Quantity  uint16
	OldValues any // []bool 또는 []uint16
	NewValues any // []bool 또는 []uint16
}

// RegisterMap 는 MODBUS 서버의 공유 레지스터 맵이다.
// 모든 읽기/쓰기 작업은 동시성 안전하게 처리된다.
type RegisterMap struct {
	coils            map[uint16]bool
	discreteInputs   map[uint16]bool
	holdingRegisters map[uint16]uint16
	inputRegisters   map[uint16]uint16

	coilRange            AddressRange
	discreteInputRange   AddressRange
	holdingRegisterRange AddressRange
	inputRegisterRange   AddressRange

	mu sync.RWMutex
}

// ---------------------------------------------------------------------------
// 생성자
// ---------------------------------------------------------------------------

// NewRegisterMap 는 RegisterMapConfig 로부터 RegisterMap 을 생성하고 초기화한다.
func NewRegisterMap(cfg RegisterMapConfig) *RegisterMap {
	rm := &RegisterMap{
		coils:            make(map[uint16]bool),
		discreteInputs:   make(map[uint16]bool),
		holdingRegisters: make(map[uint16]uint16),
		inputRegisters:   make(map[uint16]uint16),
	}

	// 코일 영역 초기화
	if cfg.Coils != nil {
		rm.coilRange = AddressRange{Start: cfg.Coils.StartAddress, Count: cfg.Coils.Count}
		for i := uint16(0); i < cfg.Coils.Count; i++ {
			rm.coils[cfg.Coils.StartAddress+i] = false
		}
		for i, v := range cfg.Coils.InitialValues {
			if b, ok := v.(bool); ok {
				rm.coils[cfg.Coils.StartAddress+uint16(i)] = b
			}
		}
	}

	// 이산 입력 영역 초기화
	if cfg.DiscreteInputs != nil {
		rm.discreteInputRange = AddressRange{Start: cfg.DiscreteInputs.StartAddress, Count: cfg.DiscreteInputs.Count}
		for i := uint16(0); i < cfg.DiscreteInputs.Count; i++ {
			rm.discreteInputs[cfg.DiscreteInputs.StartAddress+i] = false
		}
		for i, v := range cfg.DiscreteInputs.InitialValues {
			if b, ok := v.(bool); ok {
				rm.discreteInputs[cfg.DiscreteInputs.StartAddress+uint16(i)] = b
			}
		}
	}

	// 보유 레지스터 영역 초기화
	if cfg.HoldingRegisters != nil {
		rm.holdingRegisterRange = AddressRange{Start: cfg.HoldingRegisters.StartAddress, Count: cfg.HoldingRegisters.Count}
		for i := uint16(0); i < cfg.HoldingRegisters.Count; i++ {
			rm.holdingRegisters[cfg.HoldingRegisters.StartAddress+i] = 0
		}
		for i, v := range cfg.HoldingRegisters.InitialValues {
			rm.holdingRegisters[cfg.HoldingRegisters.StartAddress+uint16(i)] = anyToUint16(v)
		}
	}

	// 입력 레지스터 영역 초기화
	if cfg.InputRegisters != nil {
		rm.inputRegisterRange = AddressRange{Start: cfg.InputRegisters.StartAddress, Count: cfg.InputRegisters.Count}
		for i := uint16(0); i < cfg.InputRegisters.Count; i++ {
			rm.inputRegisters[cfg.InputRegisters.StartAddress+i] = 0
		}
		for i, v := range cfg.InputRegisters.InitialValues {
			rm.inputRegisters[cfg.InputRegisters.StartAddress+uint16(i)] = anyToUint16(v)
		}
	}

	return rm
}

// ---------------------------------------------------------------------------
// 읽기 메서드
// ---------------------------------------------------------------------------

// ReadCoils 는 지정된 범위의 코일 값을 읽는다.
func (rm *RegisterMap) ReadCoils(start, quantity uint16) ([]bool, error) {
	if err := rm.validateBoolRange(rm.coilRange, start, quantity); err != nil {
		return nil, err
	}

	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]bool, quantity)
	for i := uint16(0); i < quantity; i++ {
		result[i] = rm.coils[start+i]
	}
	return result, nil
}

// ReadDiscreteInputs 는 지정된 범위의 이산 입력 값을 읽는다.
func (rm *RegisterMap) ReadDiscreteInputs(start, quantity uint16) ([]bool, error) {
	if err := rm.validateBoolRange(rm.discreteInputRange, start, quantity); err != nil {
		return nil, err
	}

	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]bool, quantity)
	for i := uint16(0); i < quantity; i++ {
		result[i] = rm.discreteInputs[start+i]
	}
	return result, nil
}

// ReadHoldingRegisters 는 지정된 범위의 보유 레지스터 값을 읽는다.
func (rm *RegisterMap) ReadHoldingRegisters(start, quantity uint16) ([]uint16, error) {
	if err := rm.validateRegRange(rm.holdingRegisterRange, start, quantity); err != nil {
		return nil, err
	}

	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]uint16, quantity)
	for i := uint16(0); i < quantity; i++ {
		result[i] = rm.holdingRegisters[start+i]
	}
	return result, nil
}

// ReadInputRegisters 는 지정된 범위의 입력 레지스터 값을 읽는다.
func (rm *RegisterMap) ReadInputRegisters(start, quantity uint16) ([]uint16, error) {
	if err := rm.validateRegRange(rm.inputRegisterRange, start, quantity); err != nil {
		return nil, err
	}

	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]uint16, quantity)
	for i := uint16(0); i < quantity; i++ {
		result[i] = rm.inputRegisters[start+i]
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// 쓰기 메서드
// ---------------------------------------------------------------------------

// WriteCoils 는 지정된 범위의 코일 값을 쓴다.
// 실제 변경이 있을 때만 ChangeSet 을 반환한다.
func (rm *RegisterMap) WriteCoils(start uint16, values []bool) (*ChangeSet, error) {
	quantity := uint16(len(values))
	if err := rm.validateBoolRange(rm.coilRange, start, quantity); err != nil {
		return nil, err
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	return rm.writeBoolArea(rm.coils, "coils", start, values), nil
}

// WriteHoldingRegisters 는 지정된 범위의 보유 레지스터 값을 쓴다.
// 실제 변경이 있을 때만 ChangeSet 을 반환한다.
func (rm *RegisterMap) WriteHoldingRegisters(start uint16, values []uint16) (*ChangeSet, error) {
	quantity := uint16(len(values))
	if err := rm.validateRegRange(rm.holdingRegisterRange, start, quantity); err != nil {
		return nil, err
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	return rm.writeRegArea(rm.holdingRegisters, "holding_registers", start, values), nil
}

// WriteDiscreteInputs 는 이산 입력 값을 쓴다 (Bridge Process 내부 전용).
func (rm *RegisterMap) WriteDiscreteInputs(start uint16, values []bool) (*ChangeSet, error) {
	quantity := uint16(len(values))
	if err := rm.validateBoolRange(rm.discreteInputRange, start, quantity); err != nil {
		return nil, err
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	return rm.writeBoolArea(rm.discreteInputs, "discrete_inputs", start, values), nil
}

// WriteInputRegisters 는 입력 레지스터 값을 쓴다 (Bridge Process 내부 전용).
func (rm *RegisterMap) WriteInputRegisters(start uint16, values []uint16) (*ChangeSet, error) {
	quantity := uint16(len(values))
	if err := rm.validateRegRange(rm.inputRegisterRange, start, quantity); err != nil {
		return nil, err
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	return rm.writeRegArea(rm.inputRegisters, "input_registers", start, values), nil
}

// ---------------------------------------------------------------------------
// 주소 검증
// ---------------------------------------------------------------------------

// ValidateAddress 는 주어진 기능 코드와 주소 범위가 유효한지 검증한다.
func (rm *RegisterMap) ValidateAddress(fc byte, start, quantity uint16) error {
	switch fc {
	case fcReadCoils, fcWriteSingleCoil, fcWriteMultipleCoils:
		return rm.validateBoolRange(rm.coilRange, start, quantity)
	case fcReadDiscreteInputs:
		return rm.validateBoolRange(rm.discreteInputRange, start, quantity)
	case fcReadHoldingRegisters, fcWriteSingleRegister, fcWriteMultipleRegisters:
		return rm.validateRegRange(rm.holdingRegisterRange, start, quantity)
	case fcReadInputRegisters:
		return rm.validateRegRange(rm.inputRegisterRange, start, quantity)
	default:
		return ErrAddressNotMapped
	}
}

// ---------------------------------------------------------------------------
// 스냅샷
// ---------------------------------------------------------------------------

// GetSnapshot 는 모든 레지스터의 깊은 복사 스냅샷을 반환한다.
func (rm *RegisterMap) GetSnapshot() map[string]any {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	snap := make(map[string]any)

	// 코일 스냅샷
	if rm.coilRange.Count > 0 {
		coilCopy := make(map[uint16]bool, len(rm.coils))
		for k, v := range rm.coils {
			coilCopy[k] = v
		}
		snap["coils"] = coilCopy
	}

	// 이산 입력 스냅샷
	if rm.discreteInputRange.Count > 0 {
		diCopy := make(map[uint16]bool, len(rm.discreteInputs))
		for k, v := range rm.discreteInputs {
			diCopy[k] = v
		}
		snap["discrete_inputs"] = diCopy
	}

	// 보유 레지스터 스냅샷
	if rm.holdingRegisterRange.Count > 0 {
		hrCopy := make(map[uint16]uint16, len(rm.holdingRegisters))
		for k, v := range rm.holdingRegisters {
			hrCopy[k] = v
		}
		snap["holding_registers"] = hrCopy
	}

	// 입력 레지스터 스냅샷
	if rm.inputRegisterRange.Count > 0 {
		irCopy := make(map[uint16]uint16, len(rm.inputRegisters))
		for k, v := range rm.inputRegisters {
			irCopy[k] = v
		}
		snap["input_registers"] = irCopy
	}

	return snap
}

// ---------------------------------------------------------------------------
// 내부 헬퍼
// ---------------------------------------------------------------------------

// validateBoolRange 는 bool 영역의 주소 범위를 검증한다.
func (rm *RegisterMap) validateBoolRange(ar AddressRange, start, quantity uint16) error {
	if ar.Count == 0 {
		return ErrAddressNotMapped
	}
	if quantity == 0 {
		return ErrAddressNotMapped
	}
	if start < ar.Start || start+quantity > ar.Start+ar.Count {
		return ErrAddressNotMapped
	}
	return nil
}

// validateRegRange 는 레지스터 영역의 주소 범위를 검증한다.
func (rm *RegisterMap) validateRegRange(ar AddressRange, start, quantity uint16) error {
	if ar.Count == 0 {
		return ErrAddressNotMapped
	}
	if quantity == 0 {
		return ErrAddressNotMapped
	}
	if start < ar.Start || start+quantity > ar.Start+ar.Count {
		return ErrAddressNotMapped
	}
	return nil
}

// writeBoolArea 는 bool 맵에 값을 쓰고 변경 내역을 추적한다.
// 호출자가 Lock 을 보유해야 한다.
func (rm *RegisterMap) writeBoolArea(area map[uint16]bool, areaName string, start uint16, values []bool) *ChangeSet {
	quantity := uint16(len(values))
	oldVals := make([]bool, quantity)
	newVals := make([]bool, quantity)
	changed := false

	for i := uint16(0); i < quantity; i++ {
		addr := start + i
		old := area[addr]
		oldVals[i] = old
		newVals[i] = values[i]
		if old != values[i] {
			changed = true
		}
		area[addr] = values[i]
	}

	if !changed {
		return nil
	}

	return &ChangeSet{
		Area:      areaName,
		Address:   start,
		Quantity:  quantity,
		OldValues: oldVals,
		NewValues: newVals,
	}
}

// writeRegArea 는 uint16 맵에 값을 쓰고 변경 내역을 추적한다.
// 호출자가 Lock 을 보유해야 한다.
func (rm *RegisterMap) writeRegArea(area map[uint16]uint16, areaName string, start uint16, values []uint16) *ChangeSet {
	quantity := uint16(len(values))
	oldVals := make([]uint16, quantity)
	newVals := make([]uint16, quantity)
	changed := false

	for i := uint16(0); i < quantity; i++ {
		addr := start + i
		old := area[addr]
		oldVals[i] = old
		newVals[i] = values[i]
		if old != values[i] {
			changed = true
		}
		area[addr] = values[i]
	}

	if !changed {
		return nil
	}

	return &ChangeSet{
		Area:      areaName,
		Address:   start,
		Quantity:  quantity,
		OldValues: oldVals,
		NewValues: newVals,
	}
}

// anyToUint16 는 any 값을 uint16 으로 변환한다.
func anyToUint16(v any) uint16 {
	switch n := v.(type) {
	case int:
		return uint16(n)
	case float64:
		return uint16(n)
	case uint16:
		return n
	default:
		return 0
	}
}
