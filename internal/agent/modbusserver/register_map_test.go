package modbusserver

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// NewRegisterMap 초기화 테스트
// ---------------------------------------------------------------------------

func TestNewRegisterMap_Initialization(t *testing.T) {
	cfg := RegisterMapConfig{
		Coils: &RegisterAreaConfig{
			StartAddress:  0,
			Count:         10,
			InitialValues: []any{true, false, true},
		},
		HoldingRegisters: &RegisterAreaConfig{
			StartAddress:  100,
			Count:         5,
			InitialValues: []any{float64(1000), float64(2000)},
		},
		InputRegisters: &RegisterAreaConfig{
			StartAddress:  200,
			Count:         3,
			InitialValues: []any{float64(500)},
		},
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
		HoldingRegisters: &RegisterAreaConfig{
			StartAddress:  0,
			Count:         10,
			InitialValues: []any{float64(100), float64(200), float64(300), float64(400), float64(500)},
		},
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
		HoldingRegisters: &RegisterAreaConfig{
			StartAddress: 100,
			Count:        10,
		},
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
		HoldingRegisters: &RegisterAreaConfig{
			StartAddress:  0,
			Count:         10,
			InitialValues: []any{float64(100), float64(200), float64(300)},
		},
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
		HoldingRegisters: &RegisterAreaConfig{
			StartAddress:  0,
			Count:         5,
			InitialValues: []any{float64(100), float64(200)},
		},
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
		Coils: &RegisterAreaConfig{
			StartAddress:  0,
			Count:         8,
			InitialValues: []any{true, true, false, true},
		},
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
		Coils: &RegisterAreaConfig{
			StartAddress:  0,
			Count:         8,
			InitialValues: []any{false, false, false, false},
		},
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
		Coils: &RegisterAreaConfig{
			StartAddress:  0,
			Count:         4,
			InitialValues: []any{true, false},
		},
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
		DiscreteInputs: &RegisterAreaConfig{
			StartAddress: 0,
			Count:        4,
		},
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
		InputRegisters: &RegisterAreaConfig{
			StartAddress: 0,
			Count:        3,
		},
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
		Coils: &RegisterAreaConfig{
			StartAddress: 0,
			Count:        100,
		},
		DiscreteInputs: &RegisterAreaConfig{
			StartAddress: 0,
			Count:        50,
		},
		HoldingRegisters: &RegisterAreaConfig{
			StartAddress: 100,
			Count:        50,
		},
		InputRegisters: &RegisterAreaConfig{
			StartAddress: 200,
			Count:        30,
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
		Coils: &RegisterAreaConfig{
			StartAddress:  0,
			Count:         4,
			InitialValues: []any{true, false},
		},
		HoldingRegisters: &RegisterAreaConfig{
			StartAddress:  0,
			Count:         3,
			InitialValues: []any{float64(100)},
		},
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
		Coils: &RegisterAreaConfig{
			StartAddress: 0,
			Count:        100,
		},
		HoldingRegisters: &RegisterAreaConfig{
			StartAddress: 0,
			Count:        100,
		},
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
		HoldingRegisters: &RegisterAreaConfig{
			StartAddress: 0,
			Count:        10,
		},
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
		HoldingRegisters: &RegisterAreaConfig{
			StartAddress: 0,
			Count:        10,
		},
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
		Coils: &RegisterAreaConfig{
			StartAddress: 0,
			Count:        2,
		},
		DiscreteInputs: &RegisterAreaConfig{
			StartAddress: 0,
			Count:        2,
		},
		HoldingRegisters: &RegisterAreaConfig{
			StartAddress: 0,
			Count:        2,
		},
		InputRegisters: &RegisterAreaConfig{
			StartAddress: 0,
			Count:        2,
		},
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
		HoldingRegisters: &RegisterAreaConfig{
			StartAddress:  0,
			Count:         3,
			InitialValues: []any{int(100), uint16(200), float64(300)},
		},
		InputRegisters: &RegisterAreaConfig{
			StartAddress:  0,
			Count:         2,
			InitialValues: []any{int(500), float64(600)},
		},
	}
	rm := NewRegisterMap(cfg)

	regs, err := rm.ReadHoldingRegisters(0, 3)
	require.NoError(t, err)
	assert.Equal(t, []uint16{100, 200, 300}, regs)

	regs, err = rm.ReadInputRegisters(0, 2)
	require.NoError(t, err)
	assert.Equal(t, []uint16{500, 600}, regs)
}
