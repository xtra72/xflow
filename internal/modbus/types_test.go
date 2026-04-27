package modbus

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// RegisterCountForType 테스트
// ---------------------------------------------------------------------------

func TestRegisterCountForType(t *testing.T) {
	tests := []struct {
		name     string
		dataType string
		want     uint16
		wantErr  bool
	}{
		// 1-레지스터 타입
		{name: "uint16은 1 레지스터", dataType: DataTypeUint16, want: 1},
		{name: "int16은 1 레지스터", dataType: DataTypeInt16, want: 1},
		// 2-레지스터 타입
		{name: "float32는 2 레지스터", dataType: DataTypeFloat32, want: 2},
		{name: "uint32는 2 레지스터", dataType: DataTypeUint32, want: 2},
		{name: "int32는 2 레지스터", dataType: DataTypeInt32, want: 2},
		// 잘못된 타입
		{name: "알 수 없는 타입은 에러", dataType: "float64", wantErr: true},
		{name: "빈 문자열은 에러", dataType: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RegisterCountForType(tt.dataType)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrUnsupportedDataType)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// IsValidDataType 테스트
// ---------------------------------------------------------------------------

func TestIsValidDataType(t *testing.T) {
	tests := []struct {
		name     string
		dataType string
		want     bool
	}{
		{name: "uint16 유효", dataType: DataTypeUint16, want: true},
		{name: "int16 유효", dataType: DataTypeInt16, want: true},
		{name: "float32 유효", dataType: DataTypeFloat32, want: true},
		{name: "uint32 유효", dataType: DataTypeUint32, want: true},
		{name: "int32 유효", dataType: DataTypeInt32, want: true},
		{name: "float64 무효", dataType: "float64", want: false},
		{name: "빈 문자열 무효", dataType: "", want: false},
		{name: "대문자 UINT16 무효", dataType: "UINT16", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsValidDataType(tt.dataType))
		})
	}
}

// ---------------------------------------------------------------------------
// Float32 변환 테스트
// ---------------------------------------------------------------------------

func TestFloat32ToRegisters(t *testing.T) {
	tests := []struct {
		name      string
		value     float32
		byteOrder string
		wantRegs  [2]uint16
	}{
		{
			name:      "3.14 빅엔디안",
			value:     3.14,
			byteOrder: ByteOrderBigEndian,
			wantRegs:  [2]uint16{0x4048, 0xF5C3},
		},
		{
			name:      "3.14 리틀엔디안",
			value:     3.14,
			byteOrder: ByteOrderLittleEndian,
			wantRegs:  [2]uint16{0xF5C3, 0x4048},
		},
		{
			name:      "0.0 빅엔디안",
			value:     0.0,
			byteOrder: ByteOrderBigEndian,
			wantRegs:  [2]uint16{0x0000, 0x0000},
		},
		{
			name:      "음수 -1.0 빅엔디안",
			value:     -1.0,
			byteOrder: ByteOrderBigEndian,
			wantRegs:  [2]uint16{0xBF80, 0x0000},
		},
		{
			name:      "MaxFloat32 빅엔디안",
			value:     math.MaxFloat32,
			byteOrder: ByteOrderBigEndian,
			wantRegs:  [2]uint16{0x7F7F, 0xFFFF},
		},
		{
			name:      "SmallestNonzeroFloat32 빅엔디안",
			value:     math.SmallestNonzeroFloat32,
			byteOrder: ByteOrderBigEndian,
			wantRegs:  [2]uint16{0x0000, 0x0001},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Float32ToRegisters(tt.value, tt.byteOrder)
			assert.Equal(t, tt.wantRegs, got)
		})
	}
}

func TestRegistersToFloat32(t *testing.T) {
	tests := []struct {
		name      string
		regs      [2]uint16
		byteOrder string
		want      float32
	}{
		{
			name:      "3.14 빅엔디안",
			regs:      [2]uint16{0x4048, 0xF5C3},
			byteOrder: ByteOrderBigEndian,
			want:      3.14,
		},
		{
			name:      "3.14 리틀엔디안",
			regs:      [2]uint16{0xF5C3, 0x4048},
			byteOrder: ByteOrderLittleEndian,
			want:      3.14,
		},
		{
			name:      "0.0 빅엔디안",
			regs:      [2]uint16{0x0000, 0x0000},
			byteOrder: ByteOrderBigEndian,
			want:      0.0,
		},
		{
			name:      "-1.0 빅엔디안",
			regs:      [2]uint16{0xBF80, 0x0000},
			byteOrder: ByteOrderBigEndian,
			want:      -1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RegistersToFloat32(tt.regs, tt.byteOrder)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// Float32 왕복(round-trip) 테스트
// ---------------------------------------------------------------------------

func TestFloat32RoundTrip(t *testing.T) {
	values := []float32{0.0, 1.0, -1.0, 3.14, -3.14, math.MaxFloat32, math.SmallestNonzeroFloat32, 100.5}
	orders := []string{ByteOrderBigEndian, ByteOrderLittleEndian}

	for _, v := range values {
		for _, order := range orders {
			t.Run(fmt.Sprintf("value=%v/order=%s", v, order), func(t *testing.T) {
				regs := Float32ToRegisters(v, order)
				got := RegistersToFloat32(regs, order)
				assert.Equal(t, v, got, "float32 왕복 실패: value=%v, order=%s", v, order)
			})
		}
	}
}

// ---------------------------------------------------------------------------
// Int32 변환 테스트
// ---------------------------------------------------------------------------

func TestInt32ToRegisters(t *testing.T) {
	tests := []struct {
		name      string
		value     int32
		byteOrder string
		wantRegs  [2]uint16
	}{
		{
			name:      "양수 빅엔디안",
			value:     100000,
			byteOrder: ByteOrderBigEndian,
			wantRegs:  [2]uint16{0x0001, 0x86A0},
		},
		{
			name:      "양수 리틀엔디안",
			value:     100000,
			byteOrder: ByteOrderLittleEndian,
			wantRegs:  [2]uint16{0x86A0, 0x0001},
		},
		{
			name:      "음수 -1 빅엔디안",
			value:     -1,
			byteOrder: ByteOrderBigEndian,
			wantRegs:  [2]uint16{0xFFFF, 0xFFFF},
		},
		{
			name:      "0 빅엔디안",
			value:     0,
			byteOrder: ByteOrderBigEndian,
			wantRegs:  [2]uint16{0x0000, 0x0000},
		},
		{
			name:      "MaxInt32 빅엔디안",
			value:     math.MaxInt32,
			byteOrder: ByteOrderBigEndian,
			wantRegs:  [2]uint16{0x7FFF, 0xFFFF},
		},
		{
			name:      "MinInt32 빅엔디안",
			value:     math.MinInt32,
			byteOrder: ByteOrderBigEndian,
			wantRegs:  [2]uint16{0x8000, 0x0000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Int32ToRegisters(tt.value, tt.byteOrder)
			assert.Equal(t, tt.wantRegs, got)
		})
	}
}

func TestRegistersToInt32(t *testing.T) {
	tests := []struct {
		name      string
		regs      [2]uint16
		byteOrder string
		want      int32
	}{
		{
			name:      "양수 빅엔디안",
			regs:      [2]uint16{0x0001, 0x86A0},
			byteOrder: ByteOrderBigEndian,
			want:      100000,
		},
		{
			name:      "양수 리틀엔디안",
			regs:      [2]uint16{0x86A0, 0x0001},
			byteOrder: ByteOrderLittleEndian,
			want:      100000,
		},
		{
			name:      "음수 -1 빅엔디안",
			regs:      [2]uint16{0xFFFF, 0xFFFF},
			byteOrder: ByteOrderBigEndian,
			want:      -1,
		},
		{
			name:      "MaxInt32 빅엔디안",
			regs:      [2]uint16{0x7FFF, 0xFFFF},
			byteOrder: ByteOrderBigEndian,
			want:      math.MaxInt32,
		},
		{
			name:      "MinInt32 빅엔디안",
			regs:      [2]uint16{0x8000, 0x0000},
			byteOrder: ByteOrderBigEndian,
			want:      math.MinInt32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RegistersToInt32(tt.regs, tt.byteOrder)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestInt32RoundTrip(t *testing.T) {
	values := []int32{0, 1, -1, 100000, -100000, math.MaxInt32, math.MinInt32}
	orders := []string{ByteOrderBigEndian, ByteOrderLittleEndian}

	for _, v := range values {
		for _, order := range orders {
			t.Run(fmt.Sprintf("value=%v/order=%s", v, order), func(t *testing.T) {
				regs := Int32ToRegisters(v, order)
				got := RegistersToInt32(regs, order)
				assert.Equal(t, v, got, "int32 왕복 실패: value=%v, order=%s", v, order)
			})
		}
	}
}

// ---------------------------------------------------------------------------
// Uint32 변환 테스트
// ---------------------------------------------------------------------------

func TestUint32ToRegisters(t *testing.T) {
	tests := []struct {
		name      string
		value     uint32
		byteOrder string
		wantRegs  [2]uint16
	}{
		{
			name:      "양수 빅엔디안",
			value:     100000,
			byteOrder: ByteOrderBigEndian,
			wantRegs:  [2]uint16{0x0001, 0x86A0},
		},
		{
			name:      "양수 리틀엔디안",
			value:     100000,
			byteOrder: ByteOrderLittleEndian,
			wantRegs:  [2]uint16{0x86A0, 0x0001},
		},
		{
			name:      "0 빅엔디안",
			value:     0,
			byteOrder: ByteOrderBigEndian,
			wantRegs:  [2]uint16{0x0000, 0x0000},
		},
		{
			name:      "MaxUint32 빅엔디안",
			value:     math.MaxUint32,
			byteOrder: ByteOrderBigEndian,
			wantRegs:  [2]uint16{0xFFFF, 0xFFFF},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Uint32ToRegisters(tt.value, tt.byteOrder)
			assert.Equal(t, tt.wantRegs, got)
		})
	}
}

func TestRegistersToUint32(t *testing.T) {
	tests := []struct {
		name      string
		regs      [2]uint16
		byteOrder string
		want      uint32
	}{
		{
			name:      "양수 빅엔디안",
			regs:      [2]uint16{0x0001, 0x86A0},
			byteOrder: ByteOrderBigEndian,
			want:      100000,
		},
		{
			name:      "양수 리틀엔디안",
			regs:      [2]uint16{0x86A0, 0x0001},
			byteOrder: ByteOrderLittleEndian,
			want:      100000,
		},
		{
			name:      "MaxUint32 빅엔디안",
			regs:      [2]uint16{0xFFFF, 0xFFFF},
			byteOrder: ByteOrderBigEndian,
			want:      math.MaxUint32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RegistersToUint32(tt.regs, tt.byteOrder)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUint32RoundTrip(t *testing.T) {
	values := []uint32{0, 1, 100000, math.MaxUint32}
	orders := []string{ByteOrderBigEndian, ByteOrderLittleEndian}

	for _, v := range values {
		for _, order := range orders {
			t.Run(fmt.Sprintf("value=%v/order=%s", v, order), func(t *testing.T) {
				regs := Uint32ToRegisters(v, order)
				got := RegistersToUint32(regs, order)
				assert.Equal(t, v, got, "uint32 왕복 실패: value=%v, order=%s", v, order)
			})
		}
	}
}

// ---------------------------------------------------------------------------
// Int16 <-> Uint16 변환 테스트
// ---------------------------------------------------------------------------

func TestInt16ToRegister(t *testing.T) {
	tests := []struct {
		name  string
		value int16
		want  uint16
	}{
		{name: "양수", value: 100, want: 100},
		{name: "0", value: 0, want: 0},
		{name: "-1 → 0xFFFF", value: -1, want: 0xFFFF},
		{name: "MinInt16 → 0x8000", value: math.MinInt16, want: 0x8000},
		{name: "MaxInt16 → 0x7FFF", value: math.MaxInt16, want: 0x7FFF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Int16ToRegister(tt.value))
		})
	}
}

func TestRegisterToInt16(t *testing.T) {
	tests := []struct {
		name string
		reg  uint16
		want int16
	}{
		{name: "양수", reg: 100, want: 100},
		{name: "0", reg: 0, want: 0},
		{name: "0xFFFF → -1", reg: 0xFFFF, want: -1},
		{name: "0x8000 → MinInt16", reg: 0x8000, want: math.MinInt16},
		{name: "0x7FFF → MaxInt16", reg: 0x7FFF, want: math.MaxInt16},
		{name: "MaxUint16 → -1", reg: math.MaxUint16, want: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RegisterToInt16(tt.reg))
		})
	}
}

func TestInt16RoundTrip(t *testing.T) {
	values := []int16{0, 1, -1, 100, -100, math.MaxInt16, math.MinInt16}

	for _, v := range values {
		t.Run(fmt.Sprintf("value=%v", v), func(t *testing.T) {
			reg := Int16ToRegister(v)
			got := RegisterToInt16(reg)
			assert.Equal(t, v, got, "int16 왕복 실패: value=%v", v)
		})
	}
}

// ---------------------------------------------------------------------------
// TypedValueToRegisters 테스트
// ---------------------------------------------------------------------------

func TestTypedValueToRegisters(t *testing.T) {
	t.Run("uint16 타입", func(t *testing.T) {
		tests := []struct {
			name    string
			value   any
			want    []uint16
			wantErr bool
		}{
			{name: "int 값", value: 42, want: []uint16{42}},
			{name: "int64 값", value: int64(42), want: []uint16{42}},
			{name: "float64 값", value: float64(42), want: []uint16{42}},
			{name: "uint16 값", value: uint16(42), want: []uint16{42}},
			{name: "int32 값", value: int32(42), want: []uint16{42}},
			{name: "MaxUint16", value: int(math.MaxUint16), want: []uint16{math.MaxUint16}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := TypedValueToRegisters(tt.value, DataTypeUint16, ByteOrderBigEndian)
				if tt.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					assert.Equal(t, tt.want, got)
				}
			})
		}
	})

	t.Run("int16 타입", func(t *testing.T) {
		tests := []struct {
			name  string
			value any
			want  []uint16
		}{
			{name: "양수 int", value: 100, want: []uint16{100}},
			{name: "음수 int", value: -1, want: []uint16{0xFFFF}},
			{name: "int16 값", value: int16(-100), want: []uint16{0xFF9C}}, // int16(-100) → uint16(0xFF9C)
			{name: "float64 값", value: float64(-1), want: []uint16{0xFFFF}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := TypedValueToRegisters(tt.value, DataTypeInt16, ByteOrderBigEndian)
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			})
		}
	})

	t.Run("float32 타입", func(t *testing.T) {
		tests := []struct {
			name  string
			value any
			want  []uint16
		}{
			{
				name:  "float64 값 3.14",
				value: float64(3.14),
				want:  []uint16{0x4048, 0xF5C3},
			},
			{
				name:  "float32 값",
				value: float32(3.14),
				want:  []uint16{0x4048, 0xF5C3},
			},
			{
				name:  "int 값 (자동 변환)",
				value: 42,
				want: func() []uint16 {
					regs := Float32ToRegisters(42.0, ByteOrderBigEndian)
					return regs[:]
				}(),
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := TypedValueToRegisters(tt.value, DataTypeFloat32, ByteOrderBigEndian)
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			})
		}
	})

	t.Run("int32 타입", func(t *testing.T) {
		tests := []struct {
			name  string
			value any
			want  []uint16
		}{
			{
				name:  "int 값",
				value: 100000,
				want:  []uint16{0x0001, 0x86A0},
			},
			{
				name:  "int32 값",
				value: int32(100000),
				want:  []uint16{0x0001, 0x86A0},
			},
			{
				name:  "float64 값",
				value: float64(100000),
				want:  []uint16{0x0001, 0x86A0},
			},
			{
				name:  "음수 int",
				value: -1,
				want:  []uint16{0xFFFF, 0xFFFF},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := TypedValueToRegisters(tt.value, DataTypeInt32, ByteOrderBigEndian)
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			})
		}
	})

	t.Run("uint32 타입", func(t *testing.T) {
		tests := []struct {
			name  string
			value any
			want  []uint16
		}{
			{
				name:  "int 값",
				value: 100000,
				want:  []uint16{0x0001, 0x86A0},
			},
			{
				name:  "float64 값",
				value: float64(100000),
				want:  []uint16{0x0001, 0x86A0},
			},
			{
				name:  "uint32 값",
				value: uint32(100000),
				want:  []uint16{0x0001, 0x86A0},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := TypedValueToRegisters(tt.value, DataTypeUint32, ByteOrderBigEndian)
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			})
		}
	})

	t.Run("리틀엔디안 변환", func(t *testing.T) {
		got, err := TypedValueToRegisters(float64(3.14), DataTypeFloat32, ByteOrderLittleEndian)
		require.NoError(t, err)
		assert.Equal(t, []uint16{0xF5C3, 0x4048}, got)
	})

	t.Run("에러 케이스", func(t *testing.T) {
		// 지원하지 않는 데이터 타입
		_, err := TypedValueToRegisters(42, "float64", ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUnsupportedDataType)

		// 잘못된 값 타입 (문자열)
		_, err = TypedValueToRegisters("hello", DataTypeUint16, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})
}

// ---------------------------------------------------------------------------
// RegistersToTypedValue 테스트
// ---------------------------------------------------------------------------

func TestRegistersToTypedValue(t *testing.T) {
	t.Run("uint16 타입", func(t *testing.T) {
		val, err := RegistersToTypedValue([]uint16{42}, DataTypeUint16, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Equal(t, uint16(42), val)
	})

	t.Run("int16 타입", func(t *testing.T) {
		val, err := RegistersToTypedValue([]uint16{0xFFFF}, DataTypeInt16, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Equal(t, int16(-1), val)
	})

	t.Run("float32 빅엔디안", func(t *testing.T) {
		val, err := RegistersToTypedValue([]uint16{0x4048, 0xF5C3}, DataTypeFloat32, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Equal(t, float32(3.14), val)
	})

	t.Run("float32 리틀엔디안", func(t *testing.T) {
		val, err := RegistersToTypedValue([]uint16{0xF5C3, 0x4048}, DataTypeFloat32, ByteOrderLittleEndian)
		require.NoError(t, err)
		assert.Equal(t, float32(3.14), val)
	})

	t.Run("int32 빅엔디안", func(t *testing.T) {
		val, err := RegistersToTypedValue([]uint16{0x0001, 0x86A0}, DataTypeInt32, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Equal(t, int32(100000), val)
	})

	t.Run("uint32 빅엔디안", func(t *testing.T) {
		val, err := RegistersToTypedValue([]uint16{0x0001, 0x86A0}, DataTypeUint32, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Equal(t, uint32(100000), val)
	})

	t.Run("레지스터 부족 에러 - float32", func(t *testing.T) {
		_, err := RegistersToTypedValue([]uint16{0x4048}, DataTypeFloat32, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInsufficientRegisters)
	})

	t.Run("레지스터 부족 에러 - int32", func(t *testing.T) {
		_, err := RegistersToTypedValue([]uint16{0x0001}, DataTypeInt32, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInsufficientRegisters)
	})

	t.Run("레지스터 부족 에러 - uint32", func(t *testing.T) {
		_, err := RegistersToTypedValue([]uint16{}, DataTypeUint32, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInsufficientRegisters)
	})

	t.Run("레지스터 부족 에러 - uint16 빈 슬라이스", func(t *testing.T) {
		_, err := RegistersToTypedValue([]uint16{}, DataTypeUint16, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInsufficientRegisters)
	})

	t.Run("레지스터 부족 에러 - int16 빈 슬라이스", func(t *testing.T) {
		_, err := RegistersToTypedValue([]uint16{}, DataTypeInt16, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInsufficientRegisters)
	})

	t.Run("지원하지 않는 데이터 타입", func(t *testing.T) {
		_, err := RegistersToTypedValue([]uint16{0x0000}, "float64", ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUnsupportedDataType)
	})

	t.Run("nil 슬라이스 패닉 방지", func(t *testing.T) {
		assert.NotPanics(t, func() {
			_, _ = RegistersToTypedValue(nil, DataTypeFloat32, ByteOrderBigEndian)
		})
	})
}

// ---------------------------------------------------------------------------
// Generic 왕복 테스트 (TypedValueToRegisters -> RegistersToTypedValue)
// ---------------------------------------------------------------------------

func TestGenericRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		value     any
		dataType  string
		byteOrder string
		expected  any
	}{
		{
			name: "uint16 왕복", value: uint16(1234),
			dataType: DataTypeUint16, byteOrder: ByteOrderBigEndian,
			expected: uint16(1234),
		},
		{
			name: "int16 왕복", value: int16(-100),
			dataType: DataTypeInt16, byteOrder: ByteOrderBigEndian,
			expected: int16(-100),
		},
		{
			name: "float32 빅엔디안 왕복", value: float32(3.14),
			dataType: DataTypeFloat32, byteOrder: ByteOrderBigEndian,
			expected: float32(3.14),
		},
		{
			name: "float32 리틀엔디안 왕복", value: float32(-99.99),
			dataType: DataTypeFloat32, byteOrder: ByteOrderLittleEndian,
			expected: float32(-99.99),
		},
		{
			name: "int32 빅엔디안 왕복", value: int32(-100000),
			dataType: DataTypeInt32, byteOrder: ByteOrderBigEndian,
			expected: int32(-100000),
		},
		{
			name: "uint32 리틀엔디안 왕복", value: uint32(4294967295),
			dataType: DataTypeUint32, byteOrder: ByteOrderLittleEndian,
			expected: uint32(4294967295),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regs, err := TypedValueToRegisters(tt.value, tt.dataType, tt.byteOrder)
			require.NoError(t, err)

			got, err := RegistersToTypedValue(regs, tt.dataType, tt.byteOrder)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ---------------------------------------------------------------------------
// 구조체 정의 확인 테스트
// ---------------------------------------------------------------------------

func TestTypeMapEntry(t *testing.T) {
	entry := TypeMapEntry{
		Address:   100,
		DataType:  DataTypeFloat32,
		ByteOrder: ByteOrderBigEndian,
	}
	assert.Equal(t, uint16(100), entry.Address)
	assert.Equal(t, DataTypeFloat32, entry.DataType)
	assert.Equal(t, ByteOrderBigEndian, entry.ByteOrder)
}

func TestTypeOverlayEntry(t *testing.T) {
	entry := TypeOverlayEntry{
		DataType:      DataTypeInt32,
		RegisterCount: 2,
		ByteOrder:     ByteOrderLittleEndian,
	}
	assert.Equal(t, DataTypeInt32, entry.DataType)
	assert.Equal(t, uint16(2), entry.RegisterCount)
	assert.Equal(t, ByteOrderLittleEndian, entry.ByteOrder)
}

// ---------------------------------------------------------------------------
// 경계값 종합 테스트
// ---------------------------------------------------------------------------

func TestBoundaryValues(t *testing.T) {
	t.Run("MaxFloat32 왕복", func(t *testing.T) {
		regs := Float32ToRegisters(math.MaxFloat32, ByteOrderBigEndian)
		got := RegistersToFloat32(regs, ByteOrderBigEndian)
		assert.Equal(t, float32(math.MaxFloat32), got)
	})

	t.Run("SmallestNonzeroFloat32 왕복", func(t *testing.T) {
		regs := Float32ToRegisters(math.SmallestNonzeroFloat32, ByteOrderBigEndian)
		got := RegistersToFloat32(regs, ByteOrderBigEndian)
		assert.Equal(t, float32(math.SmallestNonzeroFloat32), got)
	})

	t.Run("MaxInt32 왕복", func(t *testing.T) {
		regs := Int32ToRegisters(math.MaxInt32, ByteOrderBigEndian)
		got := RegistersToInt32(regs, ByteOrderBigEndian)
		assert.Equal(t, int32(math.MaxInt32), got)
	})

	t.Run("MinInt32 왕복", func(t *testing.T) {
		regs := Int32ToRegisters(math.MinInt32, ByteOrderBigEndian)
		got := RegistersToInt32(regs, ByteOrderBigEndian)
		assert.Equal(t, int32(math.MinInt32), got)
	})

	t.Run("MaxUint32 왕복", func(t *testing.T) {
		regs := Uint32ToRegisters(math.MaxUint32, ByteOrderBigEndian)
		got := RegistersToUint32(regs, ByteOrderBigEndian)
		assert.Equal(t, uint32(math.MaxUint32), got)
	})

	t.Run("MaxInt16 왕복", func(t *testing.T) {
		reg := Int16ToRegister(math.MaxInt16)
		got := RegisterToInt16(reg)
		assert.Equal(t, int16(math.MaxInt16), got)
	})

	t.Run("MinInt16 왕복", func(t *testing.T) {
		reg := Int16ToRegister(math.MinInt16)
		got := RegisterToInt16(reg)
		assert.Equal(t, int16(math.MinInt16), got)
	})

	t.Run("MaxUint16 → int16 변환", func(t *testing.T) {
		got := RegisterToInt16(math.MaxUint16)
		assert.Equal(t, int16(-1), got)
	})
}

// ---------------------------------------------------------------------------
// 내부 헬퍼 함수 커버리지 보강 테스트
// TypedValueToRegisters를 통해 다양한 입력 타입 분기를 테스트한다.
// ---------------------------------------------------------------------------

func TestTypedValueToRegisters_AllInputTypes(t *testing.T) {
	// --- uint16 타입: int16, float32 입력 ---
	t.Run("uint16/int16 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(int16(10), DataTypeUint16, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Equal(t, []uint16{10}, got)
	})
	t.Run("uint16/float32 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(float32(42), DataTypeUint16, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Equal(t, []uint16{42}, got)
	})

	// --- int16 타입: int64, int32, uint16, float32 입력 ---
	t.Run("int16/int64 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(int64(-5), DataTypeInt16, ByteOrderBigEndian)
		require.NoError(t, err)
		v := RegisterToInt16(got[0])
		assert.Equal(t, int16(-5), v)
	})
	t.Run("int16/int32 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(int32(7), DataTypeInt16, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Equal(t, []uint16{7}, got)
	})
	t.Run("int16/uint16 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(uint16(100), DataTypeInt16, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Equal(t, []uint16{100}, got)
	})
	t.Run("int16/float32 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(float32(-3), DataTypeInt16, ByteOrderBigEndian)
		require.NoError(t, err)
		v := RegisterToInt16(got[0])
		assert.Equal(t, int16(-3), v)
	})
	t.Run("int16/잘못된 타입", func(t *testing.T) {
		_, err := TypedValueToRegisters("bad", DataTypeInt16, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})

	// --- float32 타입: int64, int32, uint16 입력 ---
	t.Run("float32/int64 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(int64(10), DataTypeFloat32, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})
	t.Run("float32/int32 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(int32(10), DataTypeFloat32, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})
	t.Run("float32/uint16 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(uint16(10), DataTypeFloat32, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})
	t.Run("float32/잘못된 타입", func(t *testing.T) {
		_, err := TypedValueToRegisters("bad", DataTypeFloat32, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})

	// --- int32 타입: int64, uint16, float32 입력 ---
	t.Run("int32/int64 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(int64(-100), DataTypeInt32, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})
	t.Run("int32/uint16 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(uint16(50), DataTypeInt32, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})
	t.Run("int32/float32 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(float32(99), DataTypeInt32, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})
	t.Run("int32/잘못된 타입", func(t *testing.T) {
		_, err := TypedValueToRegisters("bad", DataTypeInt32, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})

	// --- uint32 타입: int64, uint16, float32 입력 ---
	t.Run("uint32/int64 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(int64(200), DataTypeUint32, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})
	t.Run("uint32/uint16 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(uint16(50), DataTypeUint32, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})
	t.Run("uint32/float32 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(float32(99), DataTypeUint32, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})
	t.Run("uint32/잘못된 타입", func(t *testing.T) {
		_, err := TypedValueToRegisters("bad", DataTypeUint32, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})

	// --- 새로 추가된 타입 분기 커버리지 보강 ---

	// uint16 타입: uint32 입력
	t.Run("uint16/uint32 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(uint32(100), DataTypeUint16, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Equal(t, []uint16{100}, got)
	})
	t.Run("uint16/uint32 오버플로우", func(t *testing.T) {
		_, err := TypedValueToRegisters(uint32(70000), DataTypeUint16, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})

	// int16 타입: uint32 입력
	t.Run("int16/uint32 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(uint32(100), DataTypeInt16, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Equal(t, []uint16{100}, got)
	})
	t.Run("int16/uint32 오버플로우", func(t *testing.T) {
		_, err := TypedValueToRegisters(uint32(40000), DataTypeInt16, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})

	// float32 타입: uint32, int16 입력
	t.Run("float32/uint32 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(uint32(100), DataTypeFloat32, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})
	t.Run("float32/int16 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(int16(-5), DataTypeFloat32, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})

	// int32 타입: uint32, int16 입력
	t.Run("int32/uint32 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(uint32(100), DataTypeInt32, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})
	t.Run("int32/uint32 오버플로우", func(t *testing.T) {
		_, err := TypedValueToRegisters(uint32(math.MaxUint32), DataTypeInt32, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})
	t.Run("int32/int16 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(int16(-10), DataTypeInt32, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})

	// uint32 타입: int32, int16 입력
	t.Run("uint32/int32 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(int32(100), DataTypeUint32, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})
	t.Run("uint32/int32 음수", func(t *testing.T) {
		_, err := TypedValueToRegisters(int32(-1), DataTypeUint32, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})
	t.Run("uint32/int16 입력", func(t *testing.T) {
		got, err := TypedValueToRegisters(int16(10), DataTypeUint32, ByteOrderBigEndian)
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})
	t.Run("uint32/int16 음수", func(t *testing.T) {
		_, err := TypedValueToRegisters(int16(-1), DataTypeUint32, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})

	// toUint16Value 추가 범위 검증
	t.Run("uint16/int64 음수", func(t *testing.T) {
		_, err := TypedValueToRegisters(int64(-1), DataTypeUint16, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})
	t.Run("uint16/int32 음수", func(t *testing.T) {
		_, err := TypedValueToRegisters(int32(-1), DataTypeUint16, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})
	t.Run("uint16/float64 음수", func(t *testing.T) {
		_, err := TypedValueToRegisters(float64(-1), DataTypeUint16, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})

	// toInt16Value 추가 범위 검증
	t.Run("int16/int64 오버플로우", func(t *testing.T) {
		_, err := TypedValueToRegisters(int64(40000), DataTypeInt16, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})
	t.Run("int16/int32 오버플로우", func(t *testing.T) {
		_, err := TypedValueToRegisters(int32(40000), DataTypeInt16, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})
	t.Run("int16/float64 오버플로우", func(t *testing.T) {
		_, err := TypedValueToRegisters(float64(40000), DataTypeInt16, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})

	// toInt32Value 추가 범위 검증: float64 음수 오버플로우
	t.Run("int32/float64 음수 오버플로우", func(t *testing.T) {
		_, err := TypedValueToRegisters(float64(-3e10), DataTypeInt32, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})

	// toUint32Value 추가 범위 검증
	t.Run("uint32/int64 음수", func(t *testing.T) {
		_, err := TypedValueToRegisters(int64(-1), DataTypeUint32, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})
	t.Run("uint32/float64 음수", func(t *testing.T) {
		_, err := TypedValueToRegisters(float64(-1), DataTypeUint32, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})
}

// ---------------------------------------------------------------------------
// Float32 특수 값 (NaN, Inf) 왕복 테스트
// ---------------------------------------------------------------------------

func TestFloat32SpecialValues(t *testing.T) {
	orders := []string{ByteOrderBigEndian, ByteOrderLittleEndian}

	for _, order := range orders {
		t.Run(fmt.Sprintf("NaN/order=%s", order), func(t *testing.T) {
			nan := float32(math.NaN())
			regs := Float32ToRegisters(nan, order)
			got := RegistersToFloat32(regs, order)
			assert.True(t, math.IsNaN(float64(got)), "NaN 왕복 실패: order=%s", order)
		})

		t.Run(fmt.Sprintf("+Inf/order=%s", order), func(t *testing.T) {
			posInf := float32(math.Inf(1))
			regs := Float32ToRegisters(posInf, order)
			got := RegistersToFloat32(regs, order)
			assert.Equal(t, posInf, got, "+Inf 왕복 실패: order=%s", order)
		})

		t.Run(fmt.Sprintf("-Inf/order=%s", order), func(t *testing.T) {
			negInf := float32(math.Inf(-1))
			regs := Float32ToRegisters(negInf, order)
			got := RegistersToFloat32(regs, order)
			assert.Equal(t, negInf, got, "-Inf 왕복 실패: order=%s", order)
		})
	}
}

// ---------------------------------------------------------------------------
// 범위 검증 테스트
// TypedValueToRegisters 공개 API를 통해 내부 범위 검사를 테스트한다.
// ---------------------------------------------------------------------------

func TestRangeValidation(t *testing.T) {
	t.Run("toUint16Value/int 오버플로우", func(t *testing.T) {
		_, err := TypedValueToRegisters(int(70000), DataTypeUint16, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})

	t.Run("toUint16Value/int 음수", func(t *testing.T) {
		_, err := TypedValueToRegisters(int(-1), DataTypeUint16, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})

	t.Run("toInt16Value/int 오버플로우", func(t *testing.T) {
		_, err := TypedValueToRegisters(int(40000), DataTypeInt16, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})

	t.Run("toInt32Value/float64 오버플로우", func(t *testing.T) {
		_, err := TypedValueToRegisters(float64(3e10), DataTypeInt32, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})

	t.Run("toUint32Value/int 음수", func(t *testing.T) {
		_, err := TypedValueToRegisters(int(-1), DataTypeUint32, ByteOrderBigEndian)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidValue)
	})
}
