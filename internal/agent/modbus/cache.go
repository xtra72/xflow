package modbus

import (
	"fmt"
	"sync"
	"time"
)

// RegisterCache 는 디바이스별 레지스터 값의 인메모리 캐시이다.
// 동시성 보호를 위해 sync.RWMutex 를 사용한다.
type RegisterCache struct {
	Coils            map[uint16]bool      // FC01 Coil 값 캐시
	DiscreteInputs   map[uint16]bool      // FC02 Discrete Input 값 캐시
	HoldingRegisters map[uint16]uint16    // FC03 Holding Register 값 캐시
	InputRegisters   map[uint16]uint16    // FC04 Input Register 값 캐시
	LastUpdateTime   map[string]time.Time // 레지스터 그룹별 마지막 갱신 시각 (키: "FC{code}_{startAddr}")
	mu               sync.RWMutex
}

// NewRegisterCache 는 모든 맵이 초기화된 RegisterCache 를 생성한다.
func NewRegisterCache() *RegisterCache {
	return &RegisterCache{
		Coils:            make(map[uint16]bool),
		DiscreteInputs:   make(map[uint16]bool),
		HoldingRegisters: make(map[uint16]uint16),
		InputRegisters:   make(map[uint16]uint16),
		LastUpdateTime:   make(map[string]time.Time),
	}
}

// UpdateCoils 는 FC01 읽기 결과로 코일 캐시를 갱신한다.
// startAddr 부터 values 슬라이스 길이만큼의 코일을 업데이트한다.
func (c *RegisterCache) UpdateCoils(startAddr uint16, values []bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, v := range values {
		c.Coils[startAddr+uint16(i)] = v
	}
}

// UpdateDiscreteInputs 는 FC02 읽기 결과로 이산 입력 캐시를 갱신한다.
func (c *RegisterCache) UpdateDiscreteInputs(startAddr uint16, values []bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, v := range values {
		c.DiscreteInputs[startAddr+uint16(i)] = v
	}
}

// UpdateHoldingRegisters 는 FC03 읽기 결과로 보유 레지스터 캐시를 갱신한다.
func (c *RegisterCache) UpdateHoldingRegisters(startAddr uint16, values []uint16) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, v := range values {
		c.HoldingRegisters[startAddr+uint16(i)] = v
	}
}

// UpdateInputRegisters 는 FC04 읽기 결과로 입력 레지스터 캐시를 갱신한다.
func (c *RegisterCache) UpdateInputRegisters(startAddr uint16, values []uint16) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, v := range values {
		c.InputRegisters[startAddr+uint16(i)] = v
	}
}

// UpdateFromRead 는 원시 응답 데이터를 디코딩한 뒤 적절한 Update* 메서드를 호출하는 디스패처이다.
// protocol.go 의 decodeCoils/decodeRegisters 를 사용하여 rawData 를 디코딩한다.
// LastUpdateTime 도 갱신한다. 그룹 키 형식: "FC{fc}_{startAddr}" (예: "FC03_100").
func (c *RegisterCache) UpdateFromRead(fc byte, startAddr uint16, rawData []byte, quantity uint16) {
	groupKey := fmt.Sprintf("FC%d_%d", fc, startAddr)

	switch fc {
	case FC01ReadCoils:
		coils := decodeCoils(rawData, int(quantity))
		c.UpdateCoils(startAddr, coils)
	case FC02ReadDiscreteInputs:
		dis := decodeCoils(rawData, int(quantity))
		c.UpdateDiscreteInputs(startAddr, dis)
	case FC03ReadHoldingRegisters:
		regs := decodeRegisters(rawData)
		c.UpdateHoldingRegisters(startAddr, regs)
	case FC04ReadInputRegisters:
		regs := decodeRegisters(rawData)
		c.UpdateInputRegisters(startAddr, regs)
	}

	c.mu.Lock()
	c.LastUpdateTime[groupKey] = time.Now()
	c.mu.Unlock()
}

// IsStale 은 레지스터 그룹이 stale 한지 확인한다.
// 마지막 갱신 시각이 threshold 보다 오래되었거나 키가 존재하지 않으면 true 를 반환한다.
func (c *RegisterCache) IsStale(groupKey string, threshold time.Duration) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	t, ok := c.LastUpdateTime[groupKey]
	if !ok {
		return true
	}
	return time.Since(t) > threshold
}

// CompareAndUpdate 는 이벤트 모드에서 변경 감지에 사용된다.
// 새 데이터와 현재 캐시 값을 비교하여, 변경이 있으면 캐시를 갱신하고
// changed=true 와 변경된 데이터를 반환한다.
// 첫 번째 업데이트(해당 주소 범위에 캐시가 없는 경우)는 항상 changed=true 를 반환한다.
func (c *RegisterCache) CompareAndUpdate(fc byte, startAddr uint16, rawData []byte, quantity uint16) (changed bool, changedData map[string]any) {
	changedData = make(map[string]any)

	switch fc {
	case FC01ReadCoils, FC02ReadDiscreteInputs:
		newCoils := decodeCoils(rawData, int(quantity))
		changed = c.compareBoolValues(fc, startAddr, newCoils, changedData)
	case FC03ReadHoldingRegisters, FC04ReadInputRegisters:
		newRegs := decodeRegisters(rawData)
		changed = c.compareRegisterValues(fc, startAddr, newRegs, changedData)
	}

	if changed {
		c.UpdateFromRead(fc, startAddr, rawData, quantity)
	}

	return changed, changedData
}

// compareBoolValues 는 코일/이산입력 값을 비교하여 변경 여부를 판별한다.
func (c *RegisterCache) compareBoolValues(fc byte, startAddr uint16, newValues []bool, changedData map[string]any) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var cacheMap map[uint16]bool
	if fc == FC01ReadCoils {
		cacheMap = c.Coils
	} else {
		cacheMap = c.DiscreteInputs
	}

	anyChanged := false
	changedCoils := make(map[uint16]bool)

	for i, newVal := range newValues {
		addr := startAddr + uint16(i)
		oldVal, exists := cacheMap[addr]
		if !exists || oldVal != newVal {
			anyChanged = true
			changedCoils[addr] = newVal
		}
	}

	if anyChanged {
		changedData["changed_values"] = changedCoils
	}

	return anyChanged
}

// compareRegisterValues 는 레지스터 값을 비교하여 변경 여부를 판별한다.
func (c *RegisterCache) compareRegisterValues(fc byte, startAddr uint16, newValues []uint16, changedData map[string]any) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var cacheMap map[uint16]uint16
	if fc == FC03ReadHoldingRegisters {
		cacheMap = c.HoldingRegisters
	} else {
		cacheMap = c.InputRegisters
	}

	anyChanged := false
	changedRegs := make(map[uint16]uint16)

	for i, newVal := range newValues {
		addr := startAddr + uint16(i)
		oldVal, exists := cacheMap[addr]
		if !exists || oldVal != newVal {
			anyChanged = true
			changedRegs[addr] = newVal
		}
	}

	if anyChanged {
		changedData["changed_values"] = changedRegs
	}

	return anyChanged
}

// GetSnapshot 은 캐시 전체의 읽기 잠금 스냅샷을 반환한다.
// State() 및 get_cache 명령에서 사용된다.
func (c *RegisterCache) GetSnapshot() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 코일 복제
	coils := make(map[uint16]bool, len(c.Coils))
	for k, v := range c.Coils {
		coils[k] = v
	}

	// 이산 입력 복제
	discreteInputs := make(map[uint16]bool, len(c.DiscreteInputs))
	for k, v := range c.DiscreteInputs {
		discreteInputs[k] = v
	}

	// 보유 레지스터 복제
	holdingRegisters := make(map[uint16]uint16, len(c.HoldingRegisters))
	for k, v := range c.HoldingRegisters {
		holdingRegisters[k] = v
	}

	// 입력 레지스터 복제
	inputRegisters := make(map[uint16]uint16, len(c.InputRegisters))
	for k, v := range c.InputRegisters {
		inputRegisters[k] = v
	}

	// 마지막 갱신 시각 복제
	lastUpdateTimes := make(map[string]string, len(c.LastUpdateTime))
	for k, v := range c.LastUpdateTime {
		lastUpdateTimes[k] = v.Format(time.RFC3339)
	}

	return map[string]any{
		"coils":              coils,
		"discrete_inputs":    discreteInputs,
		"holding_registers":  holdingRegisters,
		"input_registers":    inputRegisters,
		"last_update_times":  lastUpdateTimes,
	}
}

// StaleGroups 는 threshold 보다 오래된 그룹 키 목록을 반환한다.
func (c *RegisterCache) StaleGroups(threshold time.Duration) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var stale []string
	now := time.Now()
	for key, t := range c.LastUpdateTime {
		if now.Sub(t) > threshold {
			stale = append(stale, key)
		}
	}
	return stale
}
