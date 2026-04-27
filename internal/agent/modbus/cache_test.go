package modbus

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	modbus "github.com/xtra/xflow/internal/modbus"
)

// ===========================================================================
// RegisterCache 단위 테스트
// ===========================================================================

// TestNewRegisterCache 는 초기화된 맵이 nil 이 아닌지 검증한다.
func TestNewRegisterCache(t *testing.T) {
	c := NewRegisterCache()

	assert.NotNil(t, c.Coils, "Coils 맵이 초기화되어야 한다")
	assert.NotNil(t, c.DiscreteInputs, "DiscreteInputs 맵이 초기화되어야 한다")
	assert.NotNil(t, c.HoldingRegisters, "HoldingRegisters 맵이 초기화되어야 한다")
	assert.NotNil(t, c.InputRegisters, "InputRegisters 맵이 초기화되어야 한다")
	assert.NotNil(t, c.LastUpdateTime, "LastUpdateTime 맵이 초기화되어야 한다")

	assert.Empty(t, c.Coils)
	assert.Empty(t, c.DiscreteInputs)
	assert.Empty(t, c.HoldingRegisters)
	assert.Empty(t, c.InputRegisters)
	assert.Empty(t, c.LastUpdateTime)
}

// TestRegisterCache_UpdateCoils 는 코일 캐시 갱신을 검증한다.
func TestRegisterCache_UpdateCoils(t *testing.T) {
	c := NewRegisterCache()
	values := []bool{true, false, true, true, false}

	c.UpdateCoils(100, values)

	assert.True(t, c.Coils[100])
	assert.False(t, c.Coils[101])
	assert.True(t, c.Coils[102])
	assert.True(t, c.Coils[103])
	assert.False(t, c.Coils[104])
	assert.Len(t, c.Coils, 5)
}

// TestRegisterCache_UpdateDiscreteInputs 는 이산 입력 캐시 갱신을 검증한다.
func TestRegisterCache_UpdateDiscreteInputs(t *testing.T) {
	c := NewRegisterCache()
	values := []bool{false, true, false}

	c.UpdateDiscreteInputs(200, values)

	assert.False(t, c.DiscreteInputs[200])
	assert.True(t, c.DiscreteInputs[201])
	assert.False(t, c.DiscreteInputs[202])
	assert.Len(t, c.DiscreteInputs, 3)
}

// TestRegisterCache_UpdateHoldingRegisters 는 보유 레지스터 캐시 갱신을 검증한다.
func TestRegisterCache_UpdateHoldingRegisters(t *testing.T) {
	c := NewRegisterCache()
	values := []uint16{1000, 2000, 3000}

	c.UpdateHoldingRegisters(0, values)

	assert.Equal(t, uint16(1000), c.HoldingRegisters[0])
	assert.Equal(t, uint16(2000), c.HoldingRegisters[1])
	assert.Equal(t, uint16(3000), c.HoldingRegisters[2])
	assert.Len(t, c.HoldingRegisters, 3)
}

// TestRegisterCache_UpdateInputRegisters 는 입력 레지스터 캐시 갱신을 검증한다.
func TestRegisterCache_UpdateInputRegisters(t *testing.T) {
	c := NewRegisterCache()
	values := []uint16{500, 600}

	c.UpdateInputRegisters(50, values)

	assert.Equal(t, uint16(500), c.InputRegisters[50])
	assert.Equal(t, uint16(600), c.InputRegisters[51])
	assert.Len(t, c.InputRegisters, 2)
}

// TestRegisterCache_UpdateFromRead_FC01 는 FC01 원시 데이터 디코딩 + 캐시 갱신을 검증한다.
func TestRegisterCache_UpdateFromRead_FC01(t *testing.T) {
	c := NewRegisterCache()
	// 코일 8개: 0b10101010 = 비트 1,3,5,7 true
	rawData := []byte{0xAA} // 10101010
	c.UpdateFromRead(FC01ReadCoils, 0, rawData, 8)

	assert.False(t, c.Coils[0]) // 비트 0
	assert.True(t, c.Coils[1])  // 비트 1
	assert.False(t, c.Coils[2]) // 비트 2
	assert.True(t, c.Coils[3])  // 비트 3
	assert.False(t, c.Coils[4]) // 비트 4
	assert.True(t, c.Coils[5])  // 비트 5
	assert.False(t, c.Coils[6]) // 비트 6
	assert.True(t, c.Coils[7])  // 비트 7

	// LastUpdateTime 갱신 확인
	c.mu.RLock()
	_, ok := c.LastUpdateTime["FC1_0"]
	c.mu.RUnlock()
	assert.True(t, ok, "FC1_0 LastUpdateTime 이 설정되어야 한다")
}

// TestRegisterCache_UpdateFromRead_FC03 는 FC03 원시 데이터 디코딩 + 캐시 갱신을 검증한다.
func TestRegisterCache_UpdateFromRead_FC03(t *testing.T) {
	c := NewRegisterCache()
	// 레지스터 3개: 0x0064 (100), 0x00C8 (200), 0x012C (300)
	rawData := make([]byte, 6)
	binary.BigEndian.PutUint16(rawData[0:2], 100)
	binary.BigEndian.PutUint16(rawData[2:4], 200)
	binary.BigEndian.PutUint16(rawData[4:6], 300)

	c.UpdateFromRead(FC03ReadHoldingRegisters, 10, rawData, 3)

	assert.Equal(t, uint16(100), c.HoldingRegisters[10])
	assert.Equal(t, uint16(200), c.HoldingRegisters[11])
	assert.Equal(t, uint16(300), c.HoldingRegisters[12])

	// LastUpdateTime 갱신 확인
	c.mu.RLock()
	_, ok := c.LastUpdateTime["FC3_10"]
	c.mu.RUnlock()
	assert.True(t, ok, "FC3_10 LastUpdateTime 이 설정되어야 한다")
}

// TestRegisterCache_UpdateFromRead_FC04 는 FC04 입력 레지스터 디코딩을 검증한다.
func TestRegisterCache_UpdateFromRead_FC04(t *testing.T) {
	c := NewRegisterCache()
	rawData := make([]byte, 4)
	binary.BigEndian.PutUint16(rawData[0:2], 9999)
	binary.BigEndian.PutUint16(rawData[2:4], 8888)

	c.UpdateFromRead(FC04ReadInputRegisters, 100, rawData, 2)

	assert.Equal(t, uint16(9999), c.InputRegisters[100])
	assert.Equal(t, uint16(8888), c.InputRegisters[101])

	c.mu.RLock()
	_, ok := c.LastUpdateTime["FC4_100"]
	c.mu.RUnlock()
	assert.True(t, ok)
}

// TestRegisterCache_UpdateFromRead_FC02 는 FC02 이산 입력 디코딩을 검증한다.
func TestRegisterCache_UpdateFromRead_FC02(t *testing.T) {
	c := NewRegisterCache()
	// 4개 이산 입력: 0b0101 -> 비트 0,2 true
	rawData := []byte{0x05}
	c.UpdateFromRead(FC02ReadDiscreteInputs, 50, rawData, 4)

	assert.True(t, c.DiscreteInputs[50])  // 비트 0
	assert.False(t, c.DiscreteInputs[51]) // 비트 1
	assert.True(t, c.DiscreteInputs[52])  // 비트 2
	assert.False(t, c.DiscreteInputs[53]) // 비트 3

	c.mu.RLock()
	_, ok := c.LastUpdateTime["FC2_50"]
	c.mu.RUnlock()
	assert.True(t, ok)
}

// TestRegisterCache_LastUpdateTime 는 그룹 키 형식과 시간 갱신을 검증한다.
func TestRegisterCache_LastUpdateTime(t *testing.T) {
	c := NewRegisterCache()

	before := time.Now()
	rawData := make([]byte, 4)
	binary.BigEndian.PutUint16(rawData[0:2], 100)
	binary.BigEndian.PutUint16(rawData[2:4], 200)
	c.UpdateFromRead(FC03ReadHoldingRegisters, 100, rawData, 2)
	after := time.Now()

	c.mu.RLock()
	ut, ok := c.LastUpdateTime["FC3_100"]
	c.mu.RUnlock()

	require.True(t, ok, "FC3_100 키가 존재해야 한다")
	assert.False(t, ut.Before(before), "갱신 시각이 호출 전 시각보다 이전이면 안 된다")
	assert.False(t, ut.After(after), "갱신 시각이 호출 후 시각보다 이후이면 안 된다")
}

// TestRegisterCache_IsStale 은 stale 판정 로직을 검증한다.
func TestRegisterCache_IsStale(t *testing.T) {
	c := NewRegisterCache()
	threshold := 100 * time.Millisecond

	// 존재하지 않는 키 -> stale
	assert.True(t, c.IsStale("FC3_0", threshold), "없는 키는 stale 이어야 한다")

	// 방금 갱신한 키 -> fresh
	rawData := make([]byte, 2)
	binary.BigEndian.PutUint16(rawData[0:2], 42)
	c.UpdateFromRead(FC03ReadHoldingRegisters, 0, rawData, 1)
	assert.False(t, c.IsStale("FC3_0", threshold), "방금 갱신한 키는 fresh 여야 한다")

	// threshold 만큼 대기 후 -> stale
	time.Sleep(threshold + 10*time.Millisecond)
	assert.True(t, c.IsStale("FC3_0", threshold), "threshold 경과 후 stale 이어야 한다")
}

// TestRegisterCache_StaleGroups 는 여러 그룹의 stale 판정을 검증한다.
func TestRegisterCache_StaleGroups(t *testing.T) {
	c := NewRegisterCache()
	threshold := 100 * time.Millisecond

	// 그룹 2개 갱신
	rawData := make([]byte, 2)
	binary.BigEndian.PutUint16(rawData[0:2], 1)
	c.UpdateFromRead(FC03ReadHoldingRegisters, 0, rawData, 1)
	c.UpdateFromRead(FC04ReadInputRegisters, 10, rawData, 1)

	// 아직 fresh -> stale 그룹 없음
	stale := c.StaleGroups(threshold)
	assert.Empty(t, stale, "방금 갱신하면 stale 그룹이 없어야 한다")

	// threshold 경과 후 모두 stale
	time.Sleep(threshold + 10*time.Millisecond)
	stale = c.StaleGroups(threshold)
	assert.Len(t, stale, 2, "threshold 경과 후 2개 그룹이 stale 이어야 한다")
	assert.Contains(t, stale, "FC3_0")
	assert.Contains(t, stale, "FC4_10")
}

// TestRegisterCache_CompareAndUpdate_Changed 는 값 변경 시 changed=true 를 검증한다.
func TestRegisterCache_CompareAndUpdate_Changed(t *testing.T) {
	c := NewRegisterCache()

	// 초기값 설정
	rawData := make([]byte, 4)
	binary.BigEndian.PutUint16(rawData[0:2], 100)
	binary.BigEndian.PutUint16(rawData[2:4], 200)
	c.UpdateFromRead(FC03ReadHoldingRegisters, 0, rawData, 2)

	// 다른 값으로 비교
	newData := make([]byte, 4)
	binary.BigEndian.PutUint16(newData[0:2], 100) // 동일
	binary.BigEndian.PutUint16(newData[2:4], 999) // 변경

	changed, changedData := c.CompareAndUpdate(FC03ReadHoldingRegisters, 0, newData, 2)
	assert.True(t, changed, "값이 변경되었으므로 changed=true 여야 한다")
	assert.NotEmpty(t, changedData, "changedData 가 비어있지 않아야 한다")

	// 변경된 값이 캐시에 반영되었는지 확인
	assert.Equal(t, uint16(100), c.HoldingRegisters[0])
	assert.Equal(t, uint16(999), c.HoldingRegisters[1])

	// changedData 에 변경된 레지스터가 포함되어야 한다
	cv, ok := changedData["changed_values"].(map[uint16]uint16)
	require.True(t, ok)
	assert.Equal(t, uint16(999), cv[1])
}

// TestRegisterCache_CompareAndUpdate_Unchanged 는 동일한 값에서 changed=false 를 검증한다.
func TestRegisterCache_CompareAndUpdate_Unchanged(t *testing.T) {
	c := NewRegisterCache()

	// 초기값 설정
	rawData := make([]byte, 4)
	binary.BigEndian.PutUint16(rawData[0:2], 100)
	binary.BigEndian.PutUint16(rawData[2:4], 200)
	c.UpdateFromRead(FC03ReadHoldingRegisters, 0, rawData, 2)

	// 동일한 값으로 비교
	changed, changedData := c.CompareAndUpdate(FC03ReadHoldingRegisters, 0, rawData, 2)
	assert.False(t, changed, "동일한 값이면 changed=false 여야 한다")
	assert.Empty(t, changedData["changed_values"], "변경된 값이 없어야 한다")
}

// TestRegisterCache_CompareAndUpdate_FirstUpdate 는 첫 업데이트 시 항상 changed=true 를 검증한다.
func TestRegisterCache_CompareAndUpdate_FirstUpdate(t *testing.T) {
	c := NewRegisterCache()

	// 빈 캐시에 처음 비교 -> 항상 changed
	rawData := make([]byte, 4)
	binary.BigEndian.PutUint16(rawData[0:2], 42)
	binary.BigEndian.PutUint16(rawData[2:4], 84)

	changed, changedData := c.CompareAndUpdate(FC03ReadHoldingRegisters, 0, rawData, 2)
	assert.True(t, changed, "첫 업데이트는 항상 changed=true 여야 한다")
	assert.NotEmpty(t, changedData)

	// 캐시에 반영 확인
	assert.Equal(t, uint16(42), c.HoldingRegisters[0])
	assert.Equal(t, uint16(84), c.HoldingRegisters[1])
}

// TestRegisterCache_CompareAndUpdate_Coils 는 코일 변경 감지를 검증한다.
func TestRegisterCache_CompareAndUpdate_Coils(t *testing.T) {
	c := NewRegisterCache()

	// 초기: 비트 0,1 true, 나머지 false
	rawData1 := []byte{0x03} // 00000011
	c.UpdateFromRead(FC01ReadCoils, 0, rawData1, 4)

	// 변경: 비트 0 true, 2 true -> 00000101
	rawData2 := []byte{0x05}
	changed, changedData := c.CompareAndUpdate(FC01ReadCoils, 0, rawData2, 4)
	assert.True(t, changed)

	cv, ok := changedData["changed_values"].(map[uint16]bool)
	require.True(t, ok)
	// 비트 1: true -> false (변경)
	assert.False(t, cv[1])
	// 비트 2: false -> true (변경)
	assert.True(t, cv[2])
}

// TestRegisterCache_GetSnapshot 은 스냅샷이 모든 맵을 포함하는지 검증한다.
func TestRegisterCache_GetSnapshot(t *testing.T) {
	c := NewRegisterCache()

	// 각 타입에 데이터 추가
	c.UpdateCoils(0, []bool{true, false})
	c.UpdateDiscreteInputs(10, []bool{false, true})
	c.UpdateHoldingRegisters(20, []uint16{1234})
	c.UpdateInputRegisters(30, []uint16{5678})

	// LastUpdateTime 설정
	rawData := make([]byte, 2)
	binary.BigEndian.PutUint16(rawData[0:2], 1234)
	c.UpdateFromRead(FC03ReadHoldingRegisters, 20, rawData, 1)

	snap := c.GetSnapshot()

	// 5개 키가 있어야 한다
	assert.Contains(t, snap, "coils")
	assert.Contains(t, snap, "discrete_inputs")
	assert.Contains(t, snap, "holding_registers")
	assert.Contains(t, snap, "input_registers")
	assert.Contains(t, snap, "last_update_times")

	// 코일 확인
	coils, ok := snap["coils"].(map[uint16]bool)
	require.True(t, ok)
	assert.True(t, coils[0])
	assert.False(t, coils[1])

	// 보유 레지스터 확인
	holdingRegs, ok := snap["holding_registers"].(map[uint16]uint16)
	require.True(t, ok)
	assert.Equal(t, uint16(1234), holdingRegs[20])

	// 스냅샷 수정이 원본에 영향을 주지 않는지 확인
	coils[999] = true
	assert.NotContains(t, c.Coils, uint16(999))
}

// TestRegisterCache_Concurrent 는 동시 읽기/쓰기에서 데이터 레이스가 없는지 검증한다.
func TestRegisterCache_Concurrent(t *testing.T) {
	c := NewRegisterCache()
	var wg sync.WaitGroup
	iterations := 100

	// 동시 쓰기 (코일)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			c.UpdateCoils(uint16(i), []bool{true, false})
		}
	}()

	// 동시 쓰기 (레지스터)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			c.UpdateHoldingRegisters(uint16(i), []uint16{uint16(i)})
		}
	}()

	// 동시 쓰기 (UpdateFromRead)
	wg.Add(1)
	go func() {
		defer wg.Done()
		rawData := make([]byte, 2)
		binary.BigEndian.PutUint16(rawData[0:2], 42)
		for i := 0; i < iterations; i++ {
			c.UpdateFromRead(FC03ReadHoldingRegisters, uint16(i*10), rawData, 1)
		}
	}()

	// 동시 읽기 (GetSnapshot)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = c.GetSnapshot()
		}
	}()

	// 동시 읽기 (IsStale)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = c.IsStale("FC3_0", time.Second)
		}
	}()

	// 동시 CompareAndUpdate
	wg.Add(1)
	go func() {
		defer wg.Done()
		rawData := make([]byte, 2)
		for i := 0; i < iterations; i++ {
			binary.BigEndian.PutUint16(rawData[0:2], uint16(i))
			_, _ = c.CompareAndUpdate(FC03ReadHoldingRegisters, 0, rawData, 1)
		}
	}()

	wg.Wait()
	// 데이터 레이스 없이 완료되면 성공 (go test -race 로 검증)
}

// ===========================================================================
// 에이전트 통합 테스트
// ===========================================================================

// TestModbusAgent_PollLoop_UpdatesCache 는 pollLoop 가 캐시를 갱신하는지 검증한다.
func TestModbusAgent_PollLoop_UpdatesCache(t *testing.T) {
	// FC03 정상 응답: 레지스터 10개, 값 0x0001 ~ 0x000A
	response := buildFC03ResponseWithValues(0, 1, []uint16{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	mt := &mockModbusTransport{
		connected: true,
		response:  response,
	}

	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

	// 디바이스 온라인 설정
	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	// Start (cached 모드 -> pollLoop 시작)
	err := a.Start(context.Background())
	require.NoError(t, err)

	// pollLoop 가 최소 1회 실행될 때까지 대기
	time.Sleep(300 * time.Millisecond)

	// 캐시가 갱신되었는지 확인
	cache, ok := a.caches["plc-1"]
	require.True(t, ok, "plc-1 캐시가 존재해야 한다")

	cache.mu.RLock()
	hasRegisters := len(cache.HoldingRegisters) > 0
	cache.mu.RUnlock()
	assert.True(t, hasRegisters, "pollLoop 후 HoldingRegisters 가 갱신되어야 한다")

	// 정리
	err = a.Stop(context.Background())
	require.NoError(t, err)
}

// TestModbusAgent_Process_GetCache 는 get_cache 명령을 검증한다.
func TestModbusAgent_Process_GetCache(t *testing.T) {
	a, _ := newTestModbusAgent(t, minimalAgentConfig())

	// 캐시에 직접 데이터 넣기
	if cache, ok := a.caches["plc-1"]; ok {
		cache.UpdateHoldingRegisters(0, []uint16{100, 200, 300})
	}

	data, _ := json.Marshal(map[string]any{
		"command":   "get_cache",
		"device_id": "plc-1",
	})
	result, err := a.Process(data)
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(result, &resp))

	assert.Equal(t, "plc-1", resp["device_id"])
	assert.NotEmpty(t, resp["timestamp"])

	cacheData, ok := resp["cache"].(map[string]any)
	require.True(t, ok, "cache 필드가 있어야 한다")
	assert.Contains(t, cacheData, "holding_registers")
	assert.Contains(t, cacheData, "coils")
}

// TestModbusAgent_Process_GetCache_DeviceNotFound 는 존재하지 않는 디바이스에 대한 에러를 검증한다.
func TestModbusAgent_Process_GetCache_DeviceNotFound(t *testing.T) {
	a, _ := newTestModbusAgent(t, minimalAgentConfig())

	data, _ := json.Marshal(map[string]any{
		"command":   "get_cache",
		"device_id": "nonexistent",
	})
	_, err := a.Process(data)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeviceNotFound)
}

// TestModbusAgent_Process_GetAllCaches 는 get_all_caches 명령을 검증한다.
func TestModbusAgent_Process_GetAllCaches(t *testing.T) {
	mt1 := &mockModbusTransport{connected: true}
	mt2 := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, twoDeviceAgentConfig(), mt1, mt2)

	// 각 디바이스 캐시에 데이터 넣기
	if cache, ok := a.caches["plc-1"]; ok {
		cache.UpdateHoldingRegisters(0, []uint16{111})
	}
	if cache, ok := a.caches["plc-2"]; ok {
		cache.UpdateInputRegisters(100, []uint16{222})
	}

	data, _ := json.Marshal(map[string]any{
		"command": "get_all_caches",
	})
	result, err := a.Process(data)
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(result, &resp))

	assert.NotEmpty(t, resp["timestamp"])
	caches, ok := resp["caches"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, caches, "plc-1")
	assert.Contains(t, caches, "plc-2")
}

// TestModbusAgent_State_WithCache 는 State() 에 캐시 정보가 포함되는지 검증한다.
func TestModbusAgent_State_WithCache(t *testing.T) {
	a, _ := newTestModbusAgent(t, minimalAgentConfig())

	// 캐시에 데이터 넣기
	if cache, ok := a.caches["plc-1"]; ok {
		rawData := make([]byte, 4)
		binary.BigEndian.PutUint16(rawData[0:2], 100)
		binary.BigEndian.PutUint16(rawData[2:4], 200)
		cache.UpdateFromRead(FC03ReadHoldingRegisters, 0, rawData, 2)
	}

	state := a.State()

	devices, ok := state["devices"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, devices, 1)

	dev := devices[0]

	// 캐시 필드 확인
	assert.Contains(t, dev, "cache", "State 에 cache 필드가 있어야 한다")
	assert.Contains(t, dev, "stale_groups", "State 에 stale_groups 필드가 있어야 한다")

	// 레지스터 그룹 정보 확인
	groups, ok := dev["register_groups"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, groups, 1)

	g := groups[0]
	assert.Equal(t, "holding_0-9", g["name"])
	assert.Equal(t, byte(3), g["function_code"])
	assert.Equal(t, uint16(0), g["start_address"])
	assert.Equal(t, uint16(10), g["quantity"])
	assert.Contains(t, g, "stale")
	assert.Contains(t, g, "last_update")
}

// TestModbusAgent_State_WithCache_EmptyCache 는 빈 캐시에서도 State 가 정상 동작하는지 검증한다.
func TestModbusAgent_State_WithCache_EmptyCache(t *testing.T) {
	a, _ := newTestModbusAgent(t, minimalAgentConfig())

	state := a.State()

	devices, ok := state["devices"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, devices, 1)

	dev := devices[0]
	assert.Contains(t, dev, "cache")
	assert.Contains(t, dev, "stale_groups")

	// stale_groups 는 빈 배열이어야 한다 (nil 이 아님)
	staleGroups, ok := dev["stale_groups"].([]string)
	require.True(t, ok)
	assert.Empty(t, staleGroups)

	// 레지스터 그룹은 stale=true (아직 갱신 안 됨)
	groups, ok := dev["register_groups"].([]map[string]any)
	require.True(t, ok)
	assert.True(t, groups[0]["stale"].(bool), "갱신되지 않은 그룹은 stale 이어야 한다")
}

// TestModbusAgent_CachesInitialized 는 에이전트 생성 시 캐시가 초기화되는지 검증한다.
func TestModbusAgent_CachesInitialized(t *testing.T) {
	t.Run("single device", func(t *testing.T) {
		a, _ := newTestModbusAgent(t, minimalAgentConfig())
		assert.Len(t, a.caches, 1)
		assert.Contains(t, a.caches, "plc-1")
		assert.NotNil(t, a.caches["plc-1"])
	})

	t.Run("two devices", func(t *testing.T) {
		mt1 := &mockModbusTransport{connected: true}
		mt2 := &mockModbusTransport{connected: true}
		a, _ := newTestModbusAgent(t, twoDeviceAgentConfig(), mt1, mt2)
		assert.Len(t, a.caches, 2)
		assert.Contains(t, a.caches, "plc-1")
		assert.Contains(t, a.caches, "plc-2")
	})
}

// ===========================================================================
// 테스트 헬퍼
// ===========================================================================

// ===========================================================================
// TypeOverlay 테스트
// ===========================================================================

// TestRegisterCache_SetTypeOverlay 는 TypeOverlay 설정 및 확인을 검증한다.
func TestRegisterCache_SetTypeOverlay(t *testing.T) {
	c := NewRegisterCache()

	assert.False(t, c.HasTypeOverlay(), "초기 상태에서는 TypeOverlay 가 없어야 한다")

	overlay := map[string]modbus.TypeOverlayEntry{
		"FC3:0": {DataType: "float32", RegisterCount: 2, ByteOrder: "big_endian"},
		"FC3:2": {DataType: "int32", RegisterCount: 2, ByteOrder: "big_endian"},
	}
	c.SetTypeOverlay(overlay)

	assert.True(t, c.HasTypeOverlay(), "설정 후 TypeOverlay 가 있어야 한다")
}

// TestRegisterCache_ReadTyped 는 타입 변환 읽기를 검증한다.
func TestRegisterCache_ReadTyped(t *testing.T) {
	c := NewRegisterCache()

	// float32 3.14 를 레지스터에 저장
	bits := math.Float32bits(3.14)
	high := uint16(bits >> 16)
	low := uint16(bits & 0xFFFF)
	c.UpdateHoldingRegisters(0, []uint16{high, low})

	// int16 -100 저장 (비트 재해석: int16(-100) -> uint16(0xFF9C))
	c.UpdateHoldingRegisters(2, []uint16{modbus.Int16ToRegister(-100)})

	// uint32 100000 저장
	u32 := uint32(100000)
	c.UpdateHoldingRegisters(3, []uint16{uint16(u32 >> 16), uint16(u32 & 0xFFFF)})

	t.Run("float32 읽기", func(t *testing.T) {
		val, err := c.ReadTyped(FC03ReadHoldingRegisters, 0, "float32", "big_endian")
		require.NoError(t, err)
		f, ok := val.(float32)
		require.True(t, ok, "float32 타입이어야 한다")
		assert.InDelta(t, 3.14, float64(f), 0.001)
	})

	t.Run("int16 읽기", func(t *testing.T) {
		val, err := c.ReadTyped(FC03ReadHoldingRegisters, 2, "int16", "big_endian")
		require.NoError(t, err)
		i, ok := val.(int16)
		require.True(t, ok, "int16 타입이어야 한다")
		assert.Equal(t, int16(-100), i)
	})

	t.Run("uint16 읽기 (기본 동작)", func(t *testing.T) {
		val, err := c.ReadTyped(FC03ReadHoldingRegisters, 2, "uint16", "big_endian")
		require.NoError(t, err)
		u, ok := val.(uint16)
		require.True(t, ok, "uint16 타입이어야 한다")
		assert.Equal(t, modbus.Int16ToRegister(-100), u) // 비트 재해석
	})

	t.Run("uint32 읽기", func(t *testing.T) {
		val, err := c.ReadTyped(FC03ReadHoldingRegisters, 3, "uint32", "big_endian")
		require.NoError(t, err)
		u, ok := val.(uint32)
		require.True(t, ok, "uint32 타입이어야 한다")
		assert.Equal(t, uint32(100000), u)
	})

	t.Run("FC04 InputRegisters 읽기", func(t *testing.T) {
		c.UpdateInputRegisters(100, []uint16{high, low})
		val, err := c.ReadTyped(FC04ReadInputRegisters, 100, "float32", "big_endian")
		require.NoError(t, err)
		f, ok := val.(float32)
		require.True(t, ok)
		assert.InDelta(t, 3.14, float64(f), 0.001)
	})

	t.Run("지원하지 않는 FC 코드", func(t *testing.T) {
		_, err := c.ReadTyped(FC01ReadCoils, 0, "uint16", "big_endian")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "FC03/FC04")
	})

	t.Run("캐시에 없는 주소", func(t *testing.T) {
		_, err := c.ReadTyped(FC03ReadHoldingRegisters, 9999, "uint16", "big_endian")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found in cache")
	})

	t.Run("Little-Endian float32", func(t *testing.T) {
		// Little-Endian: 하위 워드가 먼저
		c.UpdateHoldingRegisters(10, []uint16{low, high})
		val, err := c.ReadTyped(FC03ReadHoldingRegisters, 10, "float32", "little_endian")
		require.NoError(t, err)
		f, ok := val.(float32)
		require.True(t, ok)
		assert.InDelta(t, 3.14, float64(f), 0.001)
	})
}

// TestRegisterCache_GetSnapshot_WithTypeOverlay 는 TypeOverlay 설정 시 스냅샷에 typed 필드가 포함되는지 검증한다.
func TestRegisterCache_GetSnapshot_WithTypeOverlay(t *testing.T) {
	c := NewRegisterCache()

	// float32 3.14 저장
	bits := math.Float32bits(3.14)
	high := uint16(bits >> 16)
	low := uint16(bits & 0xFFFF)
	c.UpdateHoldingRegisters(0, []uint16{high, low})

	// uint16 값 저장
	c.UpdateHoldingRegisters(2, []uint16{42})

	// TypeOverlay 설정
	overlay := map[string]modbus.TypeOverlayEntry{
		"FC3:0": {DataType: "float32", RegisterCount: 2, ByteOrder: "big_endian"},
	}
	c.SetTypeOverlay(overlay)

	snap := c.GetSnapshot()

	// 기존 필드 확인
	assert.Contains(t, snap, "holding_registers")
	assert.Contains(t, snap, "coils")

	// typed_holding_registers 확인
	typedHolding, ok := snap["typed_holding_registers"].(map[uint16]any)
	require.True(t, ok, "typed_holding_registers 가 있어야 한다")
	assert.Contains(t, typedHolding, uint16(0))

	entry, ok := typedHolding[uint16(0)].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "float32", entry["data_type"])
	f, ok := entry["value"].(float32)
	require.True(t, ok)
	assert.InDelta(t, 3.14, float64(f), 0.001)
}

// TestRegisterCache_GetSnapshot_WithoutTypeOverlay 는 TypeOverlay 미설정 시 기존 동작과 동일한지 검증한다.
func TestRegisterCache_GetSnapshot_WithoutTypeOverlay(t *testing.T) {
	c := NewRegisterCache()
	c.UpdateHoldingRegisters(0, []uint16{100, 200})

	snap := c.GetSnapshot()

	assert.Contains(t, snap, "holding_registers")
	assert.NotContains(t, snap, "typed_holding_registers", "TypeOverlay 없으면 typed 필드가 없어야 한다")
	assert.NotContains(t, snap, "typed_input_registers")
}

// TestRegisterCache_CompareAndUpdate_WithTypeOverlay 는 TypeOverlay 설정 시 typed_changed_values 를 검증한다.
func TestRegisterCache_CompareAndUpdate_WithTypeOverlay(t *testing.T) {
	c := NewRegisterCache()

	// TypeOverlay 설정
	overlay := map[string]modbus.TypeOverlayEntry{
		"FC3:0": {DataType: "float32", RegisterCount: 2, ByteOrder: "big_endian"},
	}
	c.SetTypeOverlay(overlay)

	// 초기값 설정
	bits1 := math.Float32bits(1.0)
	rawData1 := make([]byte, 4)
	binary.BigEndian.PutUint16(rawData1[0:2], uint16(bits1>>16))
	binary.BigEndian.PutUint16(rawData1[2:4], uint16(bits1&0xFFFF))
	c.UpdateFromRead(FC03ReadHoldingRegisters, 0, rawData1, 2)

	// 새 값: float32 3.14
	bits2 := math.Float32bits(3.14)
	rawData2 := make([]byte, 4)
	binary.BigEndian.PutUint16(rawData2[0:2], uint16(bits2>>16))
	binary.BigEndian.PutUint16(rawData2[2:4], uint16(bits2&0xFFFF))

	changed, changedData := c.CompareAndUpdate(FC03ReadHoldingRegisters, 0, rawData2, 2)
	assert.True(t, changed, "값이 변경되었으므로 changed=true 여야 한다")

	// changed_values 확인 (raw uint16)
	cv, ok := changedData["changed_values"].(map[uint16]uint16)
	require.True(t, ok)
	assert.NotEmpty(t, cv)

	// typed_changed_values 확인
	tcv, ok := changedData["typed_changed_values"].(map[uint16]any)
	require.True(t, ok, "typed_changed_values 가 있어야 한다")
	assert.Contains(t, tcv, uint16(0))

	entry, ok := tcv[uint16(0)].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "float32", entry["data_type"])
	f, ok := entry["value"].(float32)
	require.True(t, ok)
	assert.InDelta(t, 3.14, float64(f), 0.001)
}

// TestRegisterCache_CompareAndUpdate_WithoutTypeOverlay 는 TypeOverlay 미설정 시 typed 필드가 없는지 검증한다.
func TestRegisterCache_CompareAndUpdate_WithoutTypeOverlay(t *testing.T) {
	c := NewRegisterCache()

	// 초기값 설정
	rawData1 := make([]byte, 4)
	binary.BigEndian.PutUint16(rawData1[0:2], 100)
	binary.BigEndian.PutUint16(rawData1[2:4], 200)
	c.UpdateFromRead(FC03ReadHoldingRegisters, 0, rawData1, 2)

	// 새 값
	rawData2 := make([]byte, 4)
	binary.BigEndian.PutUint16(rawData2[0:2], 100)
	binary.BigEndian.PutUint16(rawData2[2:4], 999)

	changed, changedData := c.CompareAndUpdate(FC03ReadHoldingRegisters, 0, rawData2, 2)
	assert.True(t, changed)
	assert.NotContains(t, changedData, "typed_changed_values", "TypeOverlay 없으면 typed 필드가 없어야 한다")
}

// TestRegisterCache_ReadTyped_Concurrent 는 ReadTyped 의 동시성 안전성을 검증한다.
func TestRegisterCache_ReadTyped_Concurrent(t *testing.T) {
	c := NewRegisterCache()

	bits := math.Float32bits(3.14)
	c.UpdateHoldingRegisters(0, []uint16{uint16(bits >> 16), uint16(bits & 0xFFFF)})

	overlay := map[string]modbus.TypeOverlayEntry{
		"FC3:0": {DataType: "float32", RegisterCount: 2, ByteOrder: "big_endian"},
	}
	c.SetTypeOverlay(overlay)

	var wg sync.WaitGroup
	iterations := 100

	// 동시 ReadTyped
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_, _ = c.ReadTyped(FC03ReadHoldingRegisters, 0, "float32", "big_endian")
		}
	}()

	// 동시 GetSnapshot
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = c.GetSnapshot()
		}
	}()

	// 동시 UpdateHoldingRegisters
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			newBits := math.Float32bits(float32(i))
			c.UpdateHoldingRegisters(0, []uint16{uint16(newBits >> 16), uint16(newBits & 0xFFFF)})
		}
	}()

	wg.Wait()
}

// ===========================================================================
// buildCacheTypeOverlay 테스트
// ===========================================================================

// TestBuildCacheTypeOverlay 는 디바이스 설정에서 TypeOverlay 맵을 올바르게 구축하는지 검증한다.
func TestBuildCacheTypeOverlay(t *testing.T) {
	tests := []struct {
		name     string
		groups   []RegisterGroupConfig
		wantNil  bool
		wantKeys []string // 기대하는 키 목록
	}{
		{
			name: "TypeMap 경로: 개별 주소 오버레이",
			groups: []RegisterGroupConfig{
				{
					Name:         "holding_typed",
					FunctionCode: FC03ReadHoldingRegisters,
					StartAddress: 0,
					Quantity:     10,
					TypeMap: []modbus.TypeMapEntry{
						{Address: 0, DataType: modbus.DataTypeFloat32, ByteOrder: modbus.ByteOrderBigEndian},
						{Address: 4, DataType: modbus.DataTypeInt32, ByteOrder: modbus.ByteOrderLittleEndian},
					},
				},
			},
			wantNil:  false,
			wantKeys: []string{"FC3:0", "FC3:4"},
		},
		{
			name: "TypeMap ByteOrder 기본값: big_endian",
			groups: []RegisterGroupConfig{
				{
					Name:         "holding_default_bo",
					FunctionCode: FC03ReadHoldingRegisters,
					StartAddress: 100,
					Quantity:     4,
					TypeMap: []modbus.TypeMapEntry{
						{Address: 100, DataType: modbus.DataTypeFloat32}, // ByteOrder 미지정
					},
				},
			},
			wantNil:  false,
			wantKeys: []string{"FC3:100"},
		},
		{
			name: "DataType stride 경로: float32 로 그룹 전체 분할",
			groups: []RegisterGroupConfig{
				{
					Name:         "holding_float32",
					FunctionCode: FC03ReadHoldingRegisters,
					StartAddress: 0,
					Quantity:     6,
					DataType:     modbus.DataTypeFloat32,
				},
			},
			wantNil:  false,
			wantKeys: []string{"FC3:0", "FC3:2", "FC3:4"},
		},
		{
			name: "DataType stride 경로: int32 로 그룹 분할 (나머지 레지스터 무시)",
			groups: []RegisterGroupConfig{
				{
					Name:         "holding_int32",
					FunctionCode: FC04ReadInputRegisters,
					StartAddress: 10,
					Quantity:     5, // 5 레지스터, int32 는 2개씩 → 10, 12 만 생성 (14 는 범위 초과)
					DataType:     modbus.DataTypeInt32,
				},
			},
			wantNil:  false,
			wantKeys: []string{"FC4:10", "FC4:12"},
		},
		{
			name: "FC01 코일 그룹은 스킵",
			groups: []RegisterGroupConfig{
				{
					Name:         "coils",
					FunctionCode: FC01ReadCoils,
					StartAddress: 0,
					Quantity:     10,
					DataType:     modbus.DataTypeUint16,
				},
			},
			wantNil: true,
		},
		{
			name: "FC02 이산입력 그룹은 스킵",
			groups: []RegisterGroupConfig{
				{
					Name:         "discrete_inputs",
					FunctionCode: FC02ReadDiscreteInputs,
					StartAddress: 0,
					Quantity:     10,
					DataType:     modbus.DataTypeFloat32,
				},
			},
			wantNil: true,
		},
		{
			name: "uint16 DataType 은 오버레이 생성 안 함",
			groups: []RegisterGroupConfig{
				{
					Name:         "holding_uint16",
					FunctionCode: FC03ReadHoldingRegisters,
					StartAddress: 0,
					Quantity:     10,
					DataType:     modbus.DataTypeUint16,
				},
			},
			wantNil: true,
		},
		{
			name: "DataType 빈 문자열이면 오버레이 생성 안 함",
			groups: []RegisterGroupConfig{
				{
					Name:         "holding_empty",
					FunctionCode: FC03ReadHoldingRegisters,
					StartAddress: 0,
					Quantity:     10,
					DataType:     "",
				},
			},
			wantNil: true,
		},
		{
			name: "빈 그룹 슬라이스",
			groups: []RegisterGroupConfig{},
			wantNil: true,
		},
		{
			name: "혼합: FC01 스킵 + FC03 TypeMap + FC04 DataType",
			groups: []RegisterGroupConfig{
				{
					Name:         "coils_skip",
					FunctionCode: FC01ReadCoils,
					StartAddress: 0,
					Quantity:     8,
					DataType:     modbus.DataTypeFloat32,
				},
				{
					Name:         "holding_typemap",
					FunctionCode: FC03ReadHoldingRegisters,
					StartAddress: 0,
					Quantity:     10,
					TypeMap: []modbus.TypeMapEntry{
						{Address: 0, DataType: modbus.DataTypeFloat32},
					},
				},
				{
					Name:         "input_stride",
					FunctionCode: FC04ReadInputRegisters,
					StartAddress: 100,
					Quantity:     4,
					DataType:     modbus.DataTypeUint32,
				},
			},
			wantNil:  false,
			wantKeys: []string{"FC3:0", "FC4:100", "FC4:102"},
		},
		{
			name: "int16 DataType 은 1 레지스터 stride",
			groups: []RegisterGroupConfig{
				{
					Name:         "holding_int16",
					FunctionCode: FC03ReadHoldingRegisters,
					StartAddress: 0,
					Quantity:     3,
					DataType:     modbus.DataTypeInt16,
				},
			},
			wantNil:  false,
			wantKeys: []string{"FC3:0", "FC3:1", "FC3:2"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			overlay := buildCacheTypeOverlay(tc.groups)
			if tc.wantNil {
				assert.Nil(t, overlay, "오버레이가 nil 이어야 한다")
				return
			}
			require.NotNil(t, overlay, "오버레이가 nil 이 아니어야 한다")
			assert.Len(t, overlay, len(tc.wantKeys), "오버레이 엔트리 수가 일치해야 한다")
			for _, key := range tc.wantKeys {
				_, ok := overlay[key]
				assert.True(t, ok, "키 %q 가 오버레이에 존재해야 한다", key)
			}
		})
	}
}

// TestBuildCacheTypeOverlay_EntryDetails 는 생성된 TypeOverlayEntry 의 세부 필드를 검증한다.
func TestBuildCacheTypeOverlay_EntryDetails(t *testing.T) {
	t.Run("TypeMap 엔트리 필드", func(t *testing.T) {
		groups := []RegisterGroupConfig{
			{
				FunctionCode: FC03ReadHoldingRegisters,
				StartAddress: 0,
				Quantity:     10,
				TypeMap: []modbus.TypeMapEntry{
					{Address: 0, DataType: modbus.DataTypeFloat32, ByteOrder: modbus.ByteOrderLittleEndian},
				},
			},
		}
		overlay := buildCacheTypeOverlay(groups)
		require.NotNil(t, overlay)

		entry, ok := overlay["FC3:0"]
		require.True(t, ok)
		assert.Equal(t, modbus.DataTypeFloat32, entry.DataType)
		assert.Equal(t, uint16(2), entry.RegisterCount)
		assert.Equal(t, modbus.ByteOrderLittleEndian, entry.ByteOrder)
	})

	t.Run("TypeMap ByteOrder 미지정 시 big_endian 기본값", func(t *testing.T) {
		groups := []RegisterGroupConfig{
			{
				FunctionCode: FC04ReadInputRegisters,
				StartAddress: 0,
				Quantity:     4,
				TypeMap: []modbus.TypeMapEntry{
					{Address: 0, DataType: modbus.DataTypeInt32}, // ByteOrder 미지정
				},
			},
		}
		overlay := buildCacheTypeOverlay(groups)
		require.NotNil(t, overlay)

		entry, ok := overlay["FC4:0"]
		require.True(t, ok)
		assert.Equal(t, modbus.ByteOrderBigEndian, entry.ByteOrder, "ByteOrder 기본값은 big_endian")
	})

	t.Run("DataType stride 엔트리는 항상 big_endian", func(t *testing.T) {
		groups := []RegisterGroupConfig{
			{
				FunctionCode: FC03ReadHoldingRegisters,
				StartAddress: 0,
				Quantity:     4,
				DataType:     modbus.DataTypeUint32,
			},
		}
		overlay := buildCacheTypeOverlay(groups)
		require.NotNil(t, overlay)

		for _, entry := range overlay {
			assert.Equal(t, modbus.ByteOrderBigEndian, entry.ByteOrder)
			assert.Equal(t, uint16(2), entry.RegisterCount)
			assert.Equal(t, modbus.DataTypeUint32, entry.DataType)
		}
	})
}

// ===========================================================================
// initCacheTypeOverlays 테스트
// ===========================================================================

// TestInitCacheTypeOverlays 는 모든 디바이스의 캐시에 TypeOverlay 가 올바르게 설정되는지 검증한다.
func TestInitCacheTypeOverlays(t *testing.T) {
	t.Run("TypeOverlay 가 있는 디바이스", func(t *testing.T) {
		cfg := minimalAgentConfig()
		// 디바이스에 DataType 추가
		devList := cfg.Transport.Options["devices"].([]any)
		devMap := devList[0].(map[string]any)
		rgList := devMap["register_groups"].([]any)
		rgMap := rgList[0].(map[string]any)
		rgMap["data_type"] = "float32"

		a, _ := newTestModbusAgent(t, cfg)

		// initCacheTypeOverlays 는 newModbusAgentWithTransport 에서 이미 호출됨
		cache, ok := a.caches["plc-1"]
		require.True(t, ok)
		assert.True(t, cache.HasTypeOverlay(), "TypeOverlay 가 설정되어야 한다")
	})

	t.Run("TypeOverlay 가 없는 디바이스 (기본 uint16)", func(t *testing.T) {
		cfg := minimalAgentConfig()
		// 기본 설정 (DataType 없음) → TypeOverlay 불필요
		a, _ := newTestModbusAgent(t, cfg)

		cache, ok := a.caches["plc-1"]
		require.True(t, ok)
		assert.False(t, cache.HasTypeOverlay(), "TypeOverlay 가 설정되지 않아야 한다")
	})

	t.Run("2개 디바이스: 하나만 TypeOverlay 있음", func(t *testing.T) {
		cfg := twoDeviceAgentConfig()
		// plc-1 에만 DataType 설정
		devList := cfg.Transport.Options["devices"].([]any)
		devMap1 := devList[0].(map[string]any)
		rgList1 := devMap1["register_groups"].([]any)
		rgMap1 := rgList1[0].(map[string]any)
		rgMap1["data_type"] = "int32"

		a, _ := newTestModbusAgent(t, cfg)

		cache1, ok := a.caches["plc-1"]
		require.True(t, ok)
		assert.True(t, cache1.HasTypeOverlay(), "plc-1 은 TypeOverlay 가 설정되어야 한다")

		cache2, ok := a.caches["plc-2"]
		require.True(t, ok)
		assert.False(t, cache2.HasTypeOverlay(), "plc-2 는 TypeOverlay 가 없어야 한다")
	})
}

// buildFC03ResponseWithValues 는 지정된 레지스터 값으로 FC03 응답을 생성한다.
func buildFC03ResponseWithValues(txID uint16, unitID byte, values []uint16) []byte {
	byteCount := byte(len(values) * 2)
	length := uint16(3 + byteCount)
	resp := make([]byte, 0, 7+2+int(byteCount))
	resp = append(resp,
		byte(txID>>8), byte(txID),
		0x00, 0x00,
		byte(length>>8), byte(length),
		unitID,
		FC03ReadHoldingRegisters,
		byteCount,
	)
	for _, v := range values {
		resp = append(resp, byte(v>>8), byte(v))
	}
	return resp
}
