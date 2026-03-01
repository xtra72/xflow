package modbusserver

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	modbus "github.com/xtra/xflow/internal/modbus"
)

// ---------------------------------------------------------------------------
// NewRegisterMap 초기화 테스트
// ---------------------------------------------------------------------------

func TestNewRegisterMap_Initialization(t *testing.T) {
	cfg := RegisterMapConfig{
		Coils: []*RegisterAreaConfig{{
			StartAddress:  0,
			Count:         10,
			InitialValues: []any{true, false, true},
		}},
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress:  100,
			Count:         5,
			InitialValues: []any{float64(1000), float64(2000)},
		}},
		InputRegisters: []*RegisterAreaConfig{{
			StartAddress:  200,
			Count:         3,
			InitialValues: []any{float64(500)},
		}},
	}

	rm := NewRegisterMap(cfg)
	require.NotNil(t, rm)

	// 코일 초기값 확인
	coils, err := rm.ReadCoils(0, 3)
	require.NoError(t, err)
	assert.Equal(t, []bool{true, false, true}, coils)

	// 초기값이 없는 코일은 false 여야 한다
	coils, err = rm.ReadCoils(3, 2)
	require.NoError(t, err)
	assert.Equal(t, []bool{false, false}, coils)

	// 보유 레지스터 초기값 확인
	regs, err := rm.ReadHoldingRegisters(100, 2)
	require.NoError(t, err)
	assert.Equal(t, []uint16{1000, 2000}, regs)

	// 초기값이 없는 레지스터는 0 이어야 한다
	regs, err = rm.ReadHoldingRegisters(102, 3)
	require.NoError(t, err)
	assert.Equal(t, []uint16{0, 0, 0}, regs)

	// 입력 레지스터 초기값 확인
	regs, err = rm.ReadInputRegisters(200, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{500}, regs)
}

// ---------------------------------------------------------------------------
// ReadHoldingRegisters 테스트
// ---------------------------------------------------------------------------

func TestRegisterMap_ReadHoldingRegisters(t *testing.T) {
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress:  0,
			Count:         10,
			InitialValues: []any{float64(100), float64(200), float64(300), float64(400), float64(500)},
		}},
	}
	rm := NewRegisterMap(cfg)

	tests := []struct {
		name     string
		start    uint16
		quantity uint16
		want     []uint16
	}{
		{"전체 초기값 읽기", 0, 5, []uint16{100, 200, 300, 400, 500}},
		{"부분 읽기", 1, 3, []uint16{200, 300, 400}},
		{"단일 읽기", 4, 1, []uint16{500}},
		{"초기값 이후 영역 읽기", 5, 5, []uint16{0, 0, 0, 0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rm.ReadHoldingRegisters(tt.start, tt.quantity)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRegisterMap_ReadHoldingRegisters_OutOfRange(t *testing.T) {
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 100,
			Count:        10,
		}},
	}
	rm := NewRegisterMap(cfg)

	tests := []struct {
		name     string
		start    uint16
		quantity uint16
	}{
		{"시작 주소가 범위 아래", 99, 1},
		{"시작+수량이 범위 초과", 105, 10},
		{"완전히 범위 밖", 200, 1},
		{"수량 0", 100, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := rm.ReadHoldingRegisters(tt.start, tt.quantity)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrAddressNotMapped)
		})
	}
}

// ---------------------------------------------------------------------------
// WriteHoldingRegisters 테스트
// ---------------------------------------------------------------------------

func TestRegisterMap_WriteHoldingRegisters(t *testing.T) {
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress:  0,
			Count:         10,
			InitialValues: []any{float64(100), float64(200), float64(300)},
		}},
	}
	rm := NewRegisterMap(cfg)

	// 값 변경 쓰기
	cs, err := rm.WriteHoldingRegisters(0, []uint16{999, 888})
	require.NoError(t, err)
	require.NotNil(t, cs)

	assert.Equal(t, "holding_registers", cs.Area)
	assert.Equal(t, uint16(0), cs.Address)
	assert.Equal(t, uint16(2), cs.Quantity)
	assert.Equal(t, []uint16{100, 200}, cs.OldValues)
	assert.Equal(t, []uint16{999, 888}, cs.NewValues)

	// 쓰기 결과 확인
	got, err := rm.ReadHoldingRegisters(0, 3)
	require.NoError(t, err)
	assert.Equal(t, []uint16{999, 888, 300}, got)
}

func TestRegisterMap_WriteHoldingRegisters_NoChange(t *testing.T) {
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress:  0,
			Count:         5,
			InitialValues: []any{float64(100), float64(200)},
		}},
	}
	rm := NewRegisterMap(cfg)

	// 동일한 값으로 쓰기 — ChangeSet 은 nil 이어야 한다
	cs, err := rm.WriteHoldingRegisters(0, []uint16{100, 200})
	require.NoError(t, err)
	assert.Nil(t, cs)
}

// ---------------------------------------------------------------------------
// ReadCoils / WriteCoils 테스트
// ---------------------------------------------------------------------------

func TestRegisterMap_ReadCoils(t *testing.T) {
	cfg := RegisterMapConfig{
		Coils: []*RegisterAreaConfig{{
			StartAddress:  0,
			Count:         8,
			InitialValues: []any{true, true, false, true},
		}},
	}
	rm := NewRegisterMap(cfg)

	got, err := rm.ReadCoils(0, 4)
	require.NoError(t, err)
	assert.Equal(t, []bool{true, true, false, true}, got)

	// 초기값이 없는 코일은 false
	got, err = rm.ReadCoils(4, 4)
	require.NoError(t, err)
	assert.Equal(t, []bool{false, false, false, false}, got)
}

func TestRegisterMap_WriteCoils(t *testing.T) {
	cfg := RegisterMapConfig{
		Coils: []*RegisterAreaConfig{{
			StartAddress:  0,
			Count:         8,
			InitialValues: []any{false, false, false, false},
		}},
	}
	rm := NewRegisterMap(cfg)

	cs, err := rm.WriteCoils(1, []bool{true, true})
	require.NoError(t, err)
	require.NotNil(t, cs)

	assert.Equal(t, "coils", cs.Area)
	assert.Equal(t, uint16(1), cs.Address)
	assert.Equal(t, uint16(2), cs.Quantity)
	assert.Equal(t, []bool{false, false}, cs.OldValues)
	assert.Equal(t, []bool{true, true}, cs.NewValues)

	// 쓰기 결과 확인
	got, err := rm.ReadCoils(0, 4)
	require.NoError(t, err)
	assert.Equal(t, []bool{false, true, true, false}, got)
}

func TestRegisterMap_WriteCoils_NoChange(t *testing.T) {
	cfg := RegisterMapConfig{
		Coils: []*RegisterAreaConfig{{
			StartAddress:  0,
			Count:         4,
			InitialValues: []any{true, false},
		}},
	}
	rm := NewRegisterMap(cfg)

	cs, err := rm.WriteCoils(0, []bool{true, false})
	require.NoError(t, err)
	assert.Nil(t, cs)
}

// ---------------------------------------------------------------------------
// DiscreteInputs / InputRegisters 쓰기 테스트 (내부 전용)
// ---------------------------------------------------------------------------

func TestRegisterMap_WriteDiscreteInputs(t *testing.T) {
	cfg := RegisterMapConfig{
		DiscreteInputs: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        4,
		}},
	}
	rm := NewRegisterMap(cfg)

	cs, err := rm.WriteDiscreteInputs(0, []bool{true, false, true})
	require.NoError(t, err)
	require.NotNil(t, cs)
	assert.Equal(t, "discrete_inputs", cs.Area)

	got, err := rm.ReadDiscreteInputs(0, 4)
	require.NoError(t, err)
	assert.Equal(t, []bool{true, false, true, false}, got)
}

func TestRegisterMap_WriteInputRegisters(t *testing.T) {
	cfg := RegisterMapConfig{
		InputRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        3,
		}},
	}
	rm := NewRegisterMap(cfg)

	cs, err := rm.WriteInputRegisters(0, []uint16{111, 222, 333})
	require.NoError(t, err)
	require.NotNil(t, cs)
	assert.Equal(t, "input_registers", cs.Area)

	got, err := rm.ReadInputRegisters(0, 3)
	require.NoError(t, err)
	assert.Equal(t, []uint16{111, 222, 333}, got)
}

// ---------------------------------------------------------------------------
// ValidateAddress 테스트
// ---------------------------------------------------------------------------

func TestRegisterMap_ValidateAddress(t *testing.T) {
	cfg := RegisterMapConfig{
		Coils: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        100,
		}},
		DiscreteInputs: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        50,
		}},
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 100,
			Count:        50,
		}},
		InputRegisters: []*RegisterAreaConfig{{
			StartAddress: 200,
			Count:        30,
		}},
	}
	rm := NewRegisterMap(cfg)

	tests := []struct {
		name      string
		fc        byte
		start     uint16
		quantity  uint16
		wantError bool
	}{
		// FC01 코일 읽기 — 유효
		{"FC01 코일 유효", 0x01, 0, 10, false},
		{"FC01 코일 경계", 0x01, 90, 10, false},
		{"FC01 코일 초과", 0x01, 91, 10, true},

		// FC02 이산 입력 읽기
		{"FC02 이산입력 유효", 0x02, 0, 50, false},
		{"FC02 이산입력 초과", 0x02, 0, 51, true},

		// FC03 보유 레지스터 읽기
		{"FC03 보유레지스터 유효", 0x03, 100, 50, false},
		{"FC03 보유레지스터 초과", 0x03, 100, 51, true},
		{"FC03 보유레지스터 시작 주소 오류", 0x03, 99, 1, true},

		// FC04 입력 레지스터 읽기
		{"FC04 입력레지스터 유효", 0x04, 200, 30, false},
		{"FC04 입력레지스터 초과", 0x04, 225, 10, true},

		// FC05 단일 코일 쓰기
		{"FC05 단일 코일 유효", 0x05, 50, 1, false},
		{"FC05 단일 코일 초과", 0x05, 100, 1, true},

		// FC06 단일 레지스터 쓰기
		{"FC06 단일 레지스터 유효", 0x06, 149, 1, false},
		{"FC06 단일 레지스터 초과", 0x06, 150, 1, true},

		// FC15 다중 코일 쓰기
		{"FC15 다중 코일 유효", 0x0F, 0, 100, false},
		{"FC15 다중 코일 초과", 0x0F, 50, 51, true},

		// FC16 다중 레지스터 쓰기
		{"FC16 다중 레지스터 유효", 0x10, 100, 50, false},
		{"FC16 다중 레지스터 초과", 0x10, 140, 11, true},

		// 매핑되지 않은 FC
		{"유효하지 않은 FC", 0x07, 0, 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rm.ValidateAddress(tt.fc, tt.start, tt.quantity)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetSnapshot 테스트
// ---------------------------------------------------------------------------

func TestRegisterMap_GetSnapshot(t *testing.T) {
	cfg := RegisterMapConfig{
		Coils: []*RegisterAreaConfig{{
			StartAddress:  0,
			Count:         4,
			InitialValues: []any{true, false},
		}},
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress:  0,
			Count:         3,
			InitialValues: []any{float64(100)},
		}},
	}
	rm := NewRegisterMap(cfg)

	snap := rm.GetSnapshot()
	require.NotNil(t, snap)

	// 스냅샷에 코일과 보유 레지스터가 포함되어야 한다
	coilSnap, ok := snap["coils"].(map[uint16]bool)
	require.True(t, ok)
	assert.Equal(t, true, coilSnap[0])
	assert.Equal(t, false, coilSnap[1])

	hrSnap, ok := snap["holding_registers"].(map[uint16]uint16)
	require.True(t, ok)
	assert.Equal(t, uint16(100), hrSnap[0])

	// 스냅샷 수정이 원본에 영향을 주지 않는지 확인 (deep copy)
	coilSnap[0] = false
	coils, err := rm.ReadCoils(0, 1)
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, coils, "스냅샷 수정이 원본에 영향을 주면 안 된다")
}

// ---------------------------------------------------------------------------
// 동시성 테스트
// ---------------------------------------------------------------------------

func TestRegisterMap_Concurrency(t *testing.T) {
	t.Parallel()

	cfg := RegisterMapConfig{
		Coils: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        100,
		}},
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        100,
		}},
	}
	rm := NewRegisterMap(cfg)

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines * 4) // 읽기 2종 + 쓰기 2종

	// 코일 읽기 고루틴
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = rm.ReadCoils(0, 10)
			}
		}()
	}

	// 코일 쓰기 고루틴
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				vals := make([]bool, 10)
				vals[id%10] = true
				_, _ = rm.WriteCoils(0, vals)
			}
		}(i)
	}

	// 보유 레지스터 읽기 고루틴
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = rm.ReadHoldingRegisters(0, 10)
			}
		}()
	}

	// 보유 레지스터 쓰기 고루틴
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				vals := make([]uint16, 10)
				vals[id%10] = uint16(j)
				_, _ = rm.WriteHoldingRegisters(0, vals)
			}
		}(i)
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// 영역이 설정되지 않은 경우 읽기/쓰기 에러 테스트
// ---------------------------------------------------------------------------

func TestRegisterMap_ReadFromUnconfiguredArea(t *testing.T) {
	// 보유 레지스터만 설정된 경우 코일 읽기는 에러여야 한다
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        10,
		}},
	}
	rm := NewRegisterMap(cfg)

	_, err := rm.ReadCoils(0, 1)
	assert.ErrorIs(t, err, ErrAddressNotMapped)

	_, err = rm.ReadDiscreteInputs(0, 1)
	assert.ErrorIs(t, err, ErrAddressNotMapped)

	_, err = rm.ReadInputRegisters(0, 1)
	assert.ErrorIs(t, err, ErrAddressNotMapped)
}

func TestRegisterMap_WriteToUnconfiguredArea(t *testing.T) {
	// 설정되지 않은 영역에 쓰기를 시도하면 에러여야 한다
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        10,
		}},
	}
	rm := NewRegisterMap(cfg)

	_, err := rm.WriteCoils(0, []bool{true})
	assert.ErrorIs(t, err, ErrAddressNotMapped)

	_, err = rm.WriteDiscreteInputs(0, []bool{true})
	assert.ErrorIs(t, err, ErrAddressNotMapped)

	_, err = rm.WriteInputRegisters(0, []uint16{100})
	assert.ErrorIs(t, err, ErrAddressNotMapped)

	_, err = rm.WriteHoldingRegisters(100, []uint16{100})
	assert.ErrorIs(t, err, ErrAddressNotMapped)
}

// ---------------------------------------------------------------------------
// GetSnapshot 전체 영역 테스트
// ---------------------------------------------------------------------------

func TestRegisterMap_GetSnapshot_AllAreas(t *testing.T) {
	// 4개 영역 모두 포함된 스냅샷을 확인한다
	cfg := RegisterMapConfig{
		Coils: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        2,
		}},
		DiscreteInputs: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        2,
		}},
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        2,
		}},
		InputRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        2,
		}},
	}
	rm := NewRegisterMap(cfg)

	snap := rm.GetSnapshot()
	require.NotNil(t, snap)

	_, ok := snap["coils"]
	assert.True(t, ok, "스냅샷에 coils 포함 필요")

	_, ok = snap["discrete_inputs"]
	assert.True(t, ok, "스냅샷에 discrete_inputs 포함 필요")

	_, ok = snap["holding_registers"]
	assert.True(t, ok, "스냅샷에 holding_registers 포함 필요")

	_, ok = snap["input_registers"]
	assert.True(t, ok, "스냅샷에 input_registers 포함 필요")
}

// ---------------------------------------------------------------------------
// anyToUint16 헬퍼 테스트
// ---------------------------------------------------------------------------

func TestAnyToUint16(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want uint16
	}{
		{"int", 1000, uint16(1000)},
		{"float64", float64(2000), uint16(2000)},
		{"uint16", uint16(3000), uint16(3000)},
		{"string (미지원)", "4000", uint16(0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, anyToUint16(tt.val))
		})
	}
}

// ---------------------------------------------------------------------------
// NewRegisterMap 초기값 int 타입 테스트
// ---------------------------------------------------------------------------

func TestNewRegisterMap_IntInitialValues(t *testing.T) {
	// int 타입의 초기값도 올바르게 변환되는지 확인한다
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress:  0,
			Count:         3,
			InitialValues: []any{int(100), uint16(200), float64(300)},
		}},
		InputRegisters: []*RegisterAreaConfig{{
			StartAddress:  0,
			Count:         2,
			InitialValues: []any{int(500), float64(600)},
		}},
	}
	rm := NewRegisterMap(cfg)

	regs, err := rm.ReadHoldingRegisters(0, 3)
	require.NoError(t, err)
	assert.Equal(t, []uint16{100, 200, 300}, regs)

	regs, err = rm.ReadInputRegisters(0, 2)
	require.NoError(t, err)
	assert.Equal(t, []uint16{500, 600}, regs)
}

// ---------------------------------------------------------------------------
// 타입 오버레이 테스트 (Milestone 3)
// ---------------------------------------------------------------------------

func TestRegisterMap_TypeOverlayInitialization(t *testing.T) {
	// type_map 설정으로 RegisterMap 을 생성하고 typeOverlay 가 올바르게 구축되는지 확인한다.
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        10,
			TypeMap: []modbus.TypeMapEntry{
				{Address: 0, DataType: modbus.DataTypeFloat32, ByteOrder: modbus.ByteOrderBigEndian},
				{Address: 4, DataType: modbus.DataTypeInt32, ByteOrder: modbus.ByteOrderLittleEndian},
			},
		}},
	}
	rm := NewRegisterMap(cfg)

	overlay := rm.GetTypeOverlay()
	require.NotNil(t, overlay)
	require.Len(t, overlay, 2)

	// holding_registers:0 → float32
	entry, ok := overlay["holding_registers:0"]
	require.True(t, ok, "holding_registers:0 오버레이가 있어야 한다")
	assert.Equal(t, modbus.DataTypeFloat32, entry.DataType)
	assert.Equal(t, uint16(2), entry.RegisterCount)
	assert.Equal(t, modbus.ByteOrderBigEndian, entry.ByteOrder)

	// holding_registers:4 → int32, little_endian
	entry, ok = overlay["holding_registers:4"]
	require.True(t, ok, "holding_registers:4 오버레이가 있어야 한다")
	assert.Equal(t, modbus.DataTypeInt32, entry.DataType)
	assert.Equal(t, uint16(2), entry.RegisterCount)
	assert.Equal(t, modbus.ByteOrderLittleEndian, entry.ByteOrder)
}

func TestRegisterMap_ReadTyped_Float32(t *testing.T) {
	// float32(3.14) 에 해당하는 IEEE 754 값을 레지스터에 직접 쓰고
	// ReadTyped 로 읽어서 float32(3.14) 을 반환하는지 확인한다.
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        10,
		}},
	}
	rm := NewRegisterMap(cfg)

	// float32(3.14)를 IEEE 754 레지스터로 변환하여 raw 쓰기
	regs := modbus.Float32ToRegisters(3.14, modbus.ByteOrderBigEndian)
	_, err := rm.WriteHoldingRegisters(0, regs[:])
	require.NoError(t, err)

	// ReadTyped 로 읽기
	val, err := rm.ReadTyped("holding_registers", 0, modbus.DataTypeFloat32, modbus.ByteOrderBigEndian)
	require.NoError(t, err)

	f, ok := val.(float32)
	require.True(t, ok)
	assert.InDelta(t, float32(3.14), f, 0.001)
}

func TestRegisterMap_WriteTyped_Float32(t *testing.T) {
	// WriteTyped 로 float32(3.14) 를 쓰고 raw 레지스터가 올바른 IEEE 754 값을 갖는지 확인한다.
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        10,
		}},
	}
	rm := NewRegisterMap(cfg)

	_, err := rm.WriteTyped("holding_registers", 0, float64(3.14), modbus.DataTypeFloat32, modbus.ByteOrderBigEndian)
	require.NoError(t, err)

	// raw 레지스터 읽기
	regs, err := rm.ReadHoldingRegisters(0, 2)
	require.NoError(t, err)

	// 기대값: float32(3.14) 의 IEEE 754 빅엔디안 레지스터
	expected := modbus.Float32ToRegisters(float32(3.14), modbus.ByteOrderBigEndian)
	assert.Equal(t, expected[0], regs[0])
	assert.Equal(t, expected[1], regs[1])
}

func TestRegisterMap_ReadTyped_Int32(t *testing.T) {
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        10,
		}},
	}
	rm := NewRegisterMap(cfg)

	// int32(-100000) 을 레지스터에 직접 쓰기
	regs := modbus.Int32ToRegisters(-100000, modbus.ByteOrderBigEndian)
	_, err := rm.WriteHoldingRegisters(0, regs[:])
	require.NoError(t, err)

	// ReadTyped 로 읽기
	val, err := rm.ReadTyped("holding_registers", 0, modbus.DataTypeInt32, modbus.ByteOrderBigEndian)
	require.NoError(t, err)

	i, ok := val.(int32)
	require.True(t, ok)
	assert.Equal(t, int32(-100000), i)
}

func TestRegisterMap_ReadTyped_Uint32(t *testing.T) {
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        10,
		}},
	}
	rm := NewRegisterMap(cfg)

	// uint32(100000) 을 레지스터에 직접 쓰기
	regs := modbus.Uint32ToRegisters(100000, modbus.ByteOrderBigEndian)
	_, err := rm.WriteHoldingRegisters(0, regs[:])
	require.NoError(t, err)

	// ReadTyped 로 읽기
	val, err := rm.ReadTyped("holding_registers", 0, modbus.DataTypeUint32, modbus.ByteOrderBigEndian)
	require.NoError(t, err)

	u, ok := val.(uint32)
	require.True(t, ok)
	assert.Equal(t, uint32(100000), u)
}

func TestRegisterMap_ReadTyped_Int16(t *testing.T) {
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        10,
		}},
	}
	rm := NewRegisterMap(cfg)

	// int16(-1) → uint16(0xFFFF) 을 레지스터에 직접 쓰기
	_, err := rm.WriteHoldingRegisters(0, []uint16{modbus.Int16ToRegister(-1)})
	require.NoError(t, err)

	// ReadTyped 로 읽기
	val, err := rm.ReadTyped("holding_registers", 0, modbus.DataTypeInt16, modbus.ByteOrderBigEndian)
	require.NoError(t, err)

	i, ok := val.(int16)
	require.True(t, ok)
	assert.Equal(t, int16(-1), i)
}

func TestRegisterMap_WriteTyped_RoundTrip(t *testing.T) {
	// WriteTyped 후 ReadTyped 로 값이 보존되는지 확인한다.
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        10,
		}},
	}
	rm := NewRegisterMap(cfg)

	tests := []struct {
		name     string
		value    any
		dataType string
	}{
		{"float32", float64(2.718), modbus.DataTypeFloat32},
		{"int32", int(-50000), modbus.DataTypeInt32},
		{"uint32", float64(70000), modbus.DataTypeUint32},
		{"int16", int(-100), modbus.DataTypeInt16},
		{"uint16", float64(12345), modbus.DataTypeUint16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := rm.WriteTyped("holding_registers", 0, tt.value, tt.dataType, modbus.ByteOrderBigEndian)
			require.NoError(t, err)

			val, err := rm.ReadTyped("holding_registers", 0, tt.dataType, modbus.ByteOrderBigEndian)
			require.NoError(t, err)

			switch tt.dataType {
			case modbus.DataTypeFloat32:
				f, ok := val.(float32)
				require.True(t, ok)
				assert.InDelta(t, float32(tt.value.(float64)), f, 0.001)
			case modbus.DataTypeInt32:
				i, ok := val.(int32)
				require.True(t, ok)
				assert.Equal(t, int32(tt.value.(int)), i)
			case modbus.DataTypeUint32:
				u, ok := val.(uint32)
				require.True(t, ok)
				assert.Equal(t, uint32(tt.value.(float64)), u)
			case modbus.DataTypeInt16:
				i, ok := val.(int16)
				require.True(t, ok)
				assert.Equal(t, int16(tt.value.(int)), i)
			case modbus.DataTypeUint16:
				u, ok := val.(uint16)
				require.True(t, ok)
				assert.Equal(t, uint16(tt.value.(float64)), u)
			}
		})
	}
}

func TestRegisterMap_RawReadUnaffected(t *testing.T) {
	// WriteTyped 후에도 raw ReadHoldingRegisters 가 정상 동작하고 uint16 값을 반환하는지 확인한다.
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        10,
		}},
	}
	rm := NewRegisterMap(cfg)

	// WriteTyped 로 float32(1.5) 쓰기
	_, err := rm.WriteTyped("holding_registers", 0, float64(1.5), modbus.DataTypeFloat32, modbus.ByteOrderBigEndian)
	require.NoError(t, err)

	// raw 읽기는 여전히 uint16 슬라이스를 반환해야 한다
	regs, err := rm.ReadHoldingRegisters(0, 2)
	require.NoError(t, err)
	require.Len(t, regs, 2)

	// IEEE 754: float32(1.5) = 0x3FC00000 → high=0x3FC0, low=0x0000
	expected := modbus.Float32ToRegisters(1.5, modbus.ByteOrderBigEndian)
	assert.Equal(t, expected[0], regs[0])
	assert.Equal(t, expected[1], regs[1])
}

func TestRegisterMap_ReadTyped_DefaultUint16(t *testing.T) {
	// ReadTyped 에 DataTypeUint16 을 사용하면 어떤 주소에서든 동작해야 한다.
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress:  0,
			Count:         5,
			InitialValues: []any{float64(42), float64(100)},
		}},
	}
	rm := NewRegisterMap(cfg)

	val, err := rm.ReadTyped("holding_registers", 0, modbus.DataTypeUint16, modbus.ByteOrderBigEndian)
	require.NoError(t, err)
	u, ok := val.(uint16)
	require.True(t, ok)
	assert.Equal(t, uint16(42), u)

	val, err = rm.ReadTyped("holding_registers", 1, modbus.DataTypeUint16, modbus.ByteOrderBigEndian)
	require.NoError(t, err)
	u, ok = val.(uint16)
	require.True(t, ok)
	assert.Equal(t, uint16(100), u)
}

func TestRegisterMap_InitialValues_WithDataType(t *testing.T) {
	// data_type="float32" 와 initial_values=[3.14] 로 설정하면
	// 레지스터에 IEEE 754 인코딩된 값이 저장되어야 한다.
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress:  0,
			Count:         4,
			DataType:      modbus.DataTypeFloat32,
			InitialValues: []any{float64(3.14)},
		}},
	}
	rm := NewRegisterMap(cfg)

	// raw 레지스터 읽기: float32(3.14) 의 IEEE 754 빅엔디안 레지스터
	regs, err := rm.ReadHoldingRegisters(0, 2)
	require.NoError(t, err)

	expected := modbus.Float32ToRegisters(float32(3.14), modbus.ByteOrderBigEndian)
	assert.Equal(t, expected[0], regs[0])
	assert.Equal(t, expected[1], regs[1])

	// ReadTyped 로 확인
	val, err := rm.ReadTyped("holding_registers", 0, modbus.DataTypeFloat32, modbus.ByteOrderBigEndian)
	require.NoError(t, err)
	f, ok := val.(float32)
	require.True(t, ok)
	assert.InDelta(t, float32(3.14), f, 0.001)
}

func TestRegisterMap_TypeOverlay_DefaultDataType(t *testing.T) {
	// data_type="float32" 와 count=4 로 설정하면
	// stride 2 주소(0, 2) 에 오버레이 엔트리가 생성되어야 한다.
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        4,
			DataType:     modbus.DataTypeFloat32,
		}},
	}
	rm := NewRegisterMap(cfg)

	overlay := rm.GetTypeOverlay()
	require.NotNil(t, overlay)

	// float32 는 2 레지스터이므로 count=4 일 때 addr 0, 2 에 엔트리 생성
	entry0, ok := overlay["holding_registers:0"]
	require.True(t, ok, "addr 0 에 오버레이가 있어야 한다")
	assert.Equal(t, modbus.DataTypeFloat32, entry0.DataType)
	assert.Equal(t, uint16(2), entry0.RegisterCount)

	entry2, ok := overlay["holding_registers:2"]
	require.True(t, ok, "addr 2 에 오버레이가 있어야 한다")
	assert.Equal(t, modbus.DataTypeFloat32, entry2.DataType)
	assert.Equal(t, uint16(2), entry2.RegisterCount)

	// addr 1, 3 에는 오버레이가 없어야 한다
	_, ok = overlay["holding_registers:1"]
	assert.False(t, ok, "addr 1 에 오버레이가 없어야 한다")

	_, ok = overlay["holding_registers:3"]
	assert.False(t, ok, "addr 3 에 오버레이가 없어야 한다")
}

func TestRegisterMap_ReadTyped_InputRegisters(t *testing.T) {
	// input_registers 에서도 ReadTyped/WriteTyped 가 동작하는지 확인한다.
	cfg := RegisterMapConfig{
		InputRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        10,
		}},
	}
	rm := NewRegisterMap(cfg)

	// WriteTyped 로 float32(42.5) 쓰기
	_, err := rm.WriteTyped("input_registers", 0, float64(42.5), modbus.DataTypeFloat32, modbus.ByteOrderBigEndian)
	require.NoError(t, err)

	// ReadTyped 로 읽기
	val, err := rm.ReadTyped("input_registers", 0, modbus.DataTypeFloat32, modbus.ByteOrderBigEndian)
	require.NoError(t, err)

	f, ok := val.(float32)
	require.True(t, ok)
	assert.InDelta(t, float32(42.5), f, 0.001)
}

func TestRegisterMap_ReadTyped_UnsupportedArea(t *testing.T) {
	// 지원하지 않는 영역에 ReadTyped/WriteTyped 호출 시 에러 반환 확인
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        10,
		}},
	}
	rm := NewRegisterMap(cfg)

	_, err := rm.ReadTyped("coils", 0, modbus.DataTypeUint16, modbus.ByteOrderBigEndian)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")

	_, err = rm.WriteTyped("coils", 0, uint16(1), modbus.DataTypeUint16, modbus.ByteOrderBigEndian)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestRegisterMap_GetTypeOverlay_EmptyOverlay(t *testing.T) {
	// 타입 오버레이가 없으면 nil 을 반환해야 한다.
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        10,
		}},
	}
	rm := NewRegisterMap(cfg)

	overlay := rm.GetTypeOverlay()
	assert.Nil(t, overlay)
}

func TestRegisterMap_InitialValues_MultipleFloat32(t *testing.T) {
	// 여러 float32 초기값이 올바르게 저장되는지 확인한다.
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress:  0,
			Count:         6,
			DataType:      modbus.DataTypeFloat32,
			InitialValues: []any{float64(1.5), float64(2.5), float64(3.5)},
		}},
	}
	rm := NewRegisterMap(cfg)

	// 각 float32 초기값 확인 (stride 2)
	for i, expected := range []float32{1.5, 2.5, 3.5} {
		val, err := rm.ReadTyped("holding_registers", uint16(i*2), modbus.DataTypeFloat32, modbus.ByteOrderBigEndian)
		require.NoError(t, err)
		f, ok := val.(float32)
		require.True(t, ok)
		assert.InDelta(t, expected, f, 0.001, "초기값 인덱스 %d", i)
	}
}

// ---------------------------------------------------------------------------
// 다중 세그먼트 테스트
// ---------------------------------------------------------------------------

func TestRegisterMap_MultiSegment_ReadWrite(t *testing.T) {
	// 두 개의 분리된 세그먼트에서 읽기/쓰기가 각각 동작하는지 확인한다.
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{
			{
				StartAddress:  0,
				Count:         10,
				InitialValues: []any{float64(100), float64(200)},
			},
			{
				StartAddress:  200,
				Count:         10,
				InitialValues: []any{float64(500), float64(600)},
			},
		},
	}
	rm := NewRegisterMap(cfg)

	// 첫 번째 세그먼트 읽기
	regs, err := rm.ReadHoldingRegisters(0, 2)
	require.NoError(t, err)
	assert.Equal(t, []uint16{100, 200}, regs)

	// 두 번째 세그먼트 읽기
	regs, err = rm.ReadHoldingRegisters(200, 2)
	require.NoError(t, err)
	assert.Equal(t, []uint16{500, 600}, regs)

	// 첫 번째 세그먼트 쓰기
	_, err = rm.WriteHoldingRegisters(5, []uint16{999})
	require.NoError(t, err)
	regs, err = rm.ReadHoldingRegisters(5, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{999}, regs)

	// 두 번째 세그먼트 쓰기
	_, err = rm.WriteHoldingRegisters(205, []uint16{888})
	require.NoError(t, err)
	regs, err = rm.ReadHoldingRegisters(205, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{888}, regs)
}

func TestRegisterMap_MultiSegment_GapRejection(t *testing.T) {
	// 세그먼트 사이의 빈 영역에 대한 접근은 거부되어야 한다.
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{
			{StartAddress: 0, Count: 10},
			{StartAddress: 200, Count: 10},
		},
	}
	rm := NewRegisterMap(cfg)

	// 빈 영역 읽기 시도
	_, err := rm.ReadHoldingRegisters(10, 1)
	assert.ErrorIs(t, err, ErrAddressNotMapped)

	_, err = rm.ReadHoldingRegisters(100, 1)
	assert.ErrorIs(t, err, ErrAddressNotMapped)

	_, err = rm.ReadHoldingRegisters(199, 1)
	assert.ErrorIs(t, err, ErrAddressNotMapped)

	// 빈 영역 쓰기 시도
	_, err = rm.WriteHoldingRegisters(50, []uint16{1})
	assert.ErrorIs(t, err, ErrAddressNotMapped)
}

func TestRegisterMap_MultiSegment_CrossBoundary(t *testing.T) {
	// 세그먼트 경계를 넘는 읽기/쓰기는 거부되어야 한다.
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{
			{StartAddress: 0, Count: 10},
			{StartAddress: 200, Count: 10},
		},
	}
	rm := NewRegisterMap(cfg)

	// 첫 번째 세그먼트 경계를 넘는 읽기
	_, err := rm.ReadHoldingRegisters(5, 10)
	assert.ErrorIs(t, err, ErrAddressNotMapped)

	// 두 번째 세그먼트 경계를 넘는 쓰기
	_, err = rm.WriteHoldingRegisters(205, []uint16{1, 2, 3, 4, 5, 6})
	assert.ErrorIs(t, err, ErrAddressNotMapped)
}

func TestRegisterMap_MultiSegment_Coils(t *testing.T) {
	// 코일 영역의 다중 세그먼트 동작을 확인한다.
	cfg := RegisterMapConfig{
		Coils: []*RegisterAreaConfig{
			{
				StartAddress:  0,
				Count:         8,
				InitialValues: []any{true, true, false},
			},
			{
				StartAddress:  100,
				Count:         8,
				InitialValues: []any{false, true},
			},
		},
	}
	rm := NewRegisterMap(cfg)

	// 첫 번째 세그먼트
	coils, err := rm.ReadCoils(0, 3)
	require.NoError(t, err)
	assert.Equal(t, []bool{true, true, false}, coils)

	// 두 번째 세그먼트
	coils, err = rm.ReadCoils(100, 2)
	require.NoError(t, err)
	assert.Equal(t, []bool{false, true}, coils)

	// 빈 영역
	_, err = rm.ReadCoils(50, 1)
	assert.ErrorIs(t, err, ErrAddressNotMapped)
}

func TestRegisterMap_MultiSegment_ValidateAddress(t *testing.T) {
	// ValidateAddress 가 다중 세그먼트에서 올바르게 동작하는지 확인한다.
	cfg := RegisterMapConfig{
		InputRegisters: []*RegisterAreaConfig{
			{StartAddress: 0, Count: 50},
			{StartAddress: 300, Count: 50},
		},
	}
	rm := NewRegisterMap(cfg)

	tests := []struct {
		name      string
		fc        byte
		start     uint16
		quantity  uint16
		wantError bool
	}{
		{"첫 번째 세그먼트 유효", 0x04, 0, 10, false},
		{"첫 번째 세그먼트 경계", 0x04, 40, 10, false},
		{"두 번째 세그먼트 유효", 0x04, 300, 10, false},
		{"두 번째 세그먼트 경계", 0x04, 340, 10, false},
		{"빈 영역", 0x04, 100, 1, true},
		{"세그먼트 초과", 0x04, 45, 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rm.ValidateAddress(tt.fc, tt.start, tt.quantity)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRegisterMap_MultiSegment_Snapshot(t *testing.T) {
	// 다중 세그먼트의 스냅샷이 모든 세그먼트 데이터를 포함하는지 확인한다.
	cfg := RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{
			{
				StartAddress:  0,
				Count:         5,
				InitialValues: []any{float64(10)},
			},
			{
				StartAddress:  100,
				Count:         5,
				InitialValues: []any{float64(20)},
			},
		},
	}
	rm := NewRegisterMap(cfg)

	snap := rm.GetSnapshot()
	require.NotNil(t, snap)

	hrSnap, ok := snap["holding_registers"].(map[uint16]uint16)
	require.True(t, ok)

	// 두 세그먼트의 초기값이 모두 포함되어야 한다
	assert.Equal(t, uint16(10), hrSnap[0])
	assert.Equal(t, uint16(20), hrSnap[100])

	// 초기값 없는 레지스터는 0
	assert.Equal(t, uint16(0), hrSnap[1])
	assert.Equal(t, uint16(0), hrSnap[101])
}
