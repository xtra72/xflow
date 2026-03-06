package adapter

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// --- Validate 테스트 ---

// TestModbusAdapter_Validate 는 다양한 설정 조합에서 Validate가 올바르게 동작하는지 확인한다.
func TestModbusAdapter_Validate(t *testing.T) {
	validRegs := []RegisterDef{
		{Name: "temp", Address: 0, Count: 1, DataType: "uint16"},
	}

	tests := []struct {
		name    string
		adapter *ModbusAdapter
		config  node.BridgeConfig
		wantErr string
	}{
		{
			name: "유효한 설정",
			adapter: NewModbusAdapter(
				WithUnitID(1),
				WithRegisters(validRegs),
			),
			config:  node.BridgeConfig{Direction: flow.BridgeIn},
			wantErr: "",
		},
		{
			name: "최대 UnitID 247",
			adapter: NewModbusAdapter(
				WithUnitID(247),
				WithRegisters(validRegs),
			),
			config:  node.BridgeConfig{Direction: flow.BridgeOut},
			wantErr: "",
		},
		{
			name: "UnitID가 0이면 에러",
			adapter: NewModbusAdapter(
				WithUnitID(0),
				WithRegisters(validRegs),
			),
			config:  node.BridgeConfig{Direction: flow.BridgeIn},
			wantErr: "unit ID must be 1-247, got 0",
		},
		{
			name: "UnitID가 248이면 에러",
			adapter: NewModbusAdapter(
				WithUnitID(248),
				WithRegisters(validRegs),
			),
			config:  node.BridgeConfig{Direction: flow.BridgeIn},
			wantErr: "unit ID must be 1-247, got 248",
		},
		{
			name: "빈 레지스터 목록이면 에러",
			adapter: NewModbusAdapter(
				WithUnitID(1),
				WithRegisters([]RegisterDef{}),
			),
			config:  node.BridgeConfig{Direction: flow.BridgeIn},
			wantErr: "at least one register definition required",
		},
		{
			name: "nil 레지스터 목록이면 에러",
			adapter: NewModbusAdapter(
				WithUnitID(1),
			),
			config:  node.BridgeConfig{Direction: flow.BridgeIn},
			wantErr: "at least one register definition required",
		},
		{
			name: "잘못된 데이터 타입이면 에러",
			adapter: NewModbusAdapter(
				WithUnitID(1),
				WithRegisters([]RegisterDef{
					{Name: "bad", Address: 0, Count: 1, DataType: "float64"},
				}),
			),
			config:  node.BridgeConfig{Direction: flow.BridgeIn},
			wantErr: "invalid data type \"float64\" for register \"bad\"",
		},
		{
			name: "In 방향에서 폴링 간격 < 100ms이면 에러",
			adapter: NewModbusAdapter(
				WithUnitID(1),
				WithRegisters(validRegs),
				WithPollingInterval(50*time.Millisecond),
			),
			config:  node.BridgeConfig{Direction: flow.BridgeIn},
			wantErr: "polling interval must be >= 100ms",
		},
		{
			name: "InOut 방향에서 폴링 간격 < 100ms이면 에러",
			adapter: NewModbusAdapter(
				WithUnitID(1),
				WithRegisters(validRegs),
				WithPollingInterval(99*time.Millisecond),
			),
			config:  node.BridgeConfig{Direction: flow.BridgeInOut},
			wantErr: "polling interval must be >= 100ms",
		},
		{
			name: "Out 방향에서 폴링 간격 < 100ms는 허용",
			adapter: NewModbusAdapter(
				WithUnitID(1),
				WithRegisters(validRegs),
				WithPollingInterval(10*time.Millisecond),
			),
			config:  node.BridgeConfig{Direction: flow.BridgeOut},
			wantErr: "",
		},
		{
			name: "In 방향에서 폴링 간격 100ms는 유효",
			adapter: NewModbusAdapter(
				WithUnitID(1),
				WithRegisters(validRegs),
				WithPollingInterval(100*time.Millisecond),
			),
			config:  node.BridgeConfig{Direction: flow.BridgeIn},
			wantErr: "",
		},
		{
			name: "폴링 간격 0은 허용 (미지정)",
			adapter: NewModbusAdapter(
				WithUnitID(1),
				WithRegisters(validRegs),
			),
			config:  node.BridgeConfig{Direction: flow.BridgeIn},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.adapter.Validate(tt.config)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

// --- registersToValue 테스트 ---

// TestRegistersToValue_uint16 은 uint16 레지스터 변환을 검증한다.
func TestRegistersToValue_uint16(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		byteOrder string
		want      float64
	}{
		{
			name:      "big endian 0x0100 = 256",
			data:      []byte{0x01, 0x00},
			byteOrder: "big",
			want:      256,
		},
		{
			name:      "little endian 0x0001 = 256",
			data:      []byte{0x00, 0x01},
			byteOrder: "little",
			want:      256,
		},
		{
			name:      "big endian 0xFFFF = 65535",
			data:      []byte{0xFF, 0xFF},
			byteOrder: "big",
			want:      65535,
		},
		{
			name:      "big endian 0x0000 = 0",
			data:      []byte{0x00, 0x00},
			byteOrder: "big",
			want:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := RegisterDef{DataType: "uint16", ByteOrder: tt.byteOrder, Count: 1}
			got, err := registersToValue(tt.data, reg)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRegistersToValue_int16 은 int16 레지스터 변환을 검증한다.
func TestRegistersToValue_int16(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		byteOrder string
		want      float64
	}{
		{
			name:      "양수 값 big endian 100",
			data:      []byte{0x00, 0x64},
			byteOrder: "big",
			want:      100,
		},
		{
			name:      "음수 값 big endian -1",
			data:      []byte{0xFF, 0xFF},
			byteOrder: "big",
			want:      -1,
		},
		{
			name: "음수 값 big endian -100",
			data: func() []byte {
				v := int16(-100)
				b := make([]byte, 2)
				binary.BigEndian.PutUint16(b, uint16(v))
				return b
			}(),
			byteOrder: "big",
			want:      -100,
		},
		{
			name:      "little endian 양수 32767",
			data:      []byte{0xFF, 0x7F},
			byteOrder: "little",
			want:      32767,
		},
		{
			name:      "little endian 음수 -32768",
			data:      []byte{0x00, 0x80},
			byteOrder: "little",
			want:      -32768,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := RegisterDef{DataType: "int16", ByteOrder: tt.byteOrder, Count: 1}
			got, err := registersToValue(tt.data, reg)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRegistersToValue_float32 은 float32 레지스터 변환을 검증한다.
func TestRegistersToValue_float32(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		byteOrder string
		want      float64
		checkNaN  bool
		checkInf  bool
	}{
		{
			name: "big endian 25.5",
			data: func() []byte {
				b := make([]byte, 4)
				binary.BigEndian.PutUint32(b, math.Float32bits(25.5))
				return b
			}(),
			byteOrder: "big",
			want:      float64(float32(25.5)),
		},
		{
			name: "little endian 25.5",
			data: func() []byte {
				b := make([]byte, 4)
				binary.LittleEndian.PutUint32(b, math.Float32bits(25.5))
				return b
			}(),
			byteOrder: "little",
			want:      float64(float32(25.5)),
		},
		{
			name: "big endian 0.0",
			data: func() []byte {
				b := make([]byte, 4)
				binary.BigEndian.PutUint32(b, math.Float32bits(0.0))
				return b
			}(),
			byteOrder: "big",
			want:      0.0,
		},
		{
			name: "big endian -1.5",
			data: func() []byte {
				b := make([]byte, 4)
				binary.BigEndian.PutUint32(b, math.Float32bits(-1.5))
				return b
			}(),
			byteOrder: "big",
			want:      float64(float32(-1.5)),
		},
		{
			name: "NaN 값",
			data: func() []byte {
				b := make([]byte, 4)
				binary.BigEndian.PutUint32(b, math.Float32bits(float32(math.NaN())))
				return b
			}(),
			byteOrder: "big",
			checkNaN:  true,
		},
		{
			name: "+Inf 값",
			data: func() []byte {
				b := make([]byte, 4)
				binary.BigEndian.PutUint32(b, math.Float32bits(float32(math.Inf(1))))
				return b
			}(),
			byteOrder: "big",
			checkInf:  true,
			want:      math.Inf(1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := RegisterDef{DataType: "float32", ByteOrder: tt.byteOrder, Count: 2}
			got, err := registersToValue(tt.data, reg)
			require.NoError(t, err)
			if tt.checkNaN {
				assert.True(t, math.IsNaN(got), "값이 NaN이어야 한다")
			} else if tt.checkInf {
				assert.True(t, math.IsInf(got, 1), "값이 +Inf이어야 한다")
			} else {
				assert.InDelta(t, tt.want, got, 1e-6)
			}
		})
	}
}

// TestRegistersToValue_uint32 은 uint32 레지스터 변환을 검증한다.
func TestRegistersToValue_uint32(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		byteOrder string
		want      float64
	}{
		{
			name: "big endian 100000",
			data: func() []byte {
				b := make([]byte, 4)
				binary.BigEndian.PutUint32(b, 100000)
				return b
			}(),
			byteOrder: "big",
			want:      100000,
		},
		{
			name: "little endian 100000",
			data: func() []byte {
				b := make([]byte, 4)
				binary.LittleEndian.PutUint32(b, 100000)
				return b
			}(),
			byteOrder: "little",
			want:      100000,
		},
		{
			name:      "big endian 0",
			data:      []byte{0x00, 0x00, 0x00, 0x00},
			byteOrder: "big",
			want:      0,
		},
		{
			name: "big endian max uint32",
			data: func() []byte {
				b := make([]byte, 4)
				binary.BigEndian.PutUint32(b, math.MaxUint32)
				return b
			}(),
			byteOrder: "big",
			want:      float64(math.MaxUint32),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := RegisterDef{DataType: "uint32", ByteOrder: tt.byteOrder, Count: 2}
			got, err := registersToValue(tt.data, reg)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRegistersToValue_int32 은 int32 레지스터 변환을 검증한다.
func TestRegistersToValue_int32(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		byteOrder string
		want      float64
	}{
		{
			name: "양수 big endian 50000",
			data: func() []byte {
				b := make([]byte, 4)
				binary.BigEndian.PutUint32(b, uint32(int32(50000)))
				return b
			}(),
			byteOrder: "big",
			want:      50000,
		},
		{
			name: "음수 big endian -50000",
			data: func() []byte {
				v := int32(-50000)
				b := make([]byte, 4)
				binary.BigEndian.PutUint32(b, uint32(v))
				return b
			}(),
			byteOrder: "big",
			want:      -50000,
		},
		{
			name: "little endian -1",
			data: func() []byte {
				v := int32(-1)
				b := make([]byte, 4)
				binary.LittleEndian.PutUint32(b, uint32(v))
				return b
			}(),
			byteOrder: "little",
			want:      -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := RegisterDef{DataType: "int32", ByteOrder: tt.byteOrder, Count: 2}
			got, err := registersToValue(tt.data, reg)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRegistersToValue_InsufficientData 는 데이터 부족 시 에러를 반환하는지 확인한다.
func TestRegistersToValue_InsufficientData(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		dataType string
	}{
		{name: "uint16 데이터 부족", data: []byte{0x01}, dataType: "uint16"},
		{name: "int16 데이터 부족", data: []byte{0x01}, dataType: "int16"},
		{name: "float32 데이터 부족", data: []byte{0x01, 0x02}, dataType: "float32"},
		{name: "uint32 데이터 부족", data: []byte{0x01, 0x02, 0x03}, dataType: "uint32"},
		{name: "int32 데이터 부족", data: []byte{0x01}, dataType: "int32"},
		{name: "uint16 빈 데이터", data: []byte{}, dataType: "uint16"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := RegisterDef{DataType: tt.dataType, ByteOrder: "big", Count: uint16(validDataTypes[tt.dataType])}
			_, err := registersToValue(tt.data, reg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "insufficient data")
		})
	}
}

// TestRegistersToValue_UnsupportedDataType 은 지원하지 않는 데이터 타입에서 에러를 반환하는지 확인한다.
func TestRegistersToValue_UnsupportedDataType(t *testing.T) {
	reg := RegisterDef{DataType: "float64", ByteOrder: "big", Count: 4}
	_, err := registersToValue([]byte{0, 0, 0, 0, 0, 0, 0, 0}, reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported data type")
}

// --- valueToRegisters 테스트 ---

// TestValueToRegisters 는 각 데이터 타입의 값-레지스터 변환을 검증한다.
func TestValueToRegisters(t *testing.T) {
	tests := []struct {
		name      string
		value     float64
		reg       RegisterDef
		wantBytes []byte
	}{
		{
			name:  "uint16 big endian 256",
			value: 256,
			reg:   RegisterDef{DataType: "uint16", ByteOrder: "big", Count: 1},
			wantBytes: func() []byte {
				b := make([]byte, 2)
				binary.BigEndian.PutUint16(b, 256)
				return b
			}(),
		},
		{
			name:  "int16 big endian -100",
			value: -100,
			reg:   RegisterDef{DataType: "int16", ByteOrder: "big", Count: 1},
			wantBytes: func() []byte {
				v := int16(-100)
				b := make([]byte, 2)
				binary.BigEndian.PutUint16(b, uint16(v))
				return b
			}(),
		},
		{
			name:  "uint32 big endian 100000",
			value: 100000,
			reg:   RegisterDef{DataType: "uint32", ByteOrder: "big", Count: 2},
			wantBytes: func() []byte {
				b := make([]byte, 4)
				binary.BigEndian.PutUint32(b, 100000)
				return b
			}(),
		},
		{
			name:  "int32 big endian -50000",
			value: -50000,
			reg:   RegisterDef{DataType: "int32", ByteOrder: "big", Count: 2},
			wantBytes: func() []byte {
				v := int32(-50000)
				b := make([]byte, 4)
				binary.BigEndian.PutUint32(b, uint32(v))
				return b
			}(),
		},
		{
			name:  "float32 big endian 25.5",
			value: float64(float32(25.5)),
			reg:   RegisterDef{DataType: "float32", ByteOrder: "big", Count: 2},
			wantBytes: func() []byte {
				b := make([]byte, 4)
				binary.BigEndian.PutUint32(b, math.Float32bits(25.5))
				return b
			}(),
		},
		{
			name:  "uint16 little endian 256",
			value: 256,
			reg:   RegisterDef{DataType: "uint16", ByteOrder: "little", Count: 1},
			wantBytes: func() []byte {
				b := make([]byte, 2)
				binary.LittleEndian.PutUint16(b, 256)
				return b
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := valueToRegisters(tt.value, tt.reg)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBytes, got)
		})
	}
}

// TestValueToRegisters_UnsupportedType 은 지원하지 않는 데이터 타입에서 에러를 반환하는지 확인한다.
func TestValueToRegisters_UnsupportedType(t *testing.T) {
	reg := RegisterDef{DataType: "string", ByteOrder: "big", Count: 1}
	_, err := valueToRegisters(42, reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported data type")
}

// TestValueToRegisters_Roundtrip 은 값을 레지스터로 변환 후 다시 값으로 역변환했을 때 동일한지 확인한다.
func TestValueToRegisters_Roundtrip(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		reg      RegisterDef
		expected float64
	}{
		{
			name:     "uint16 라운드트립",
			value:    12345,
			reg:      RegisterDef{DataType: "uint16", ByteOrder: "big", Count: 1},
			expected: 12345,
		},
		{
			name:     "int16 양수 라운드트립",
			value:    1000,
			reg:      RegisterDef{DataType: "int16", ByteOrder: "big", Count: 1},
			expected: 1000,
		},
		{
			name:     "int16 음수 라운드트립",
			value:    -1000,
			reg:      RegisterDef{DataType: "int16", ByteOrder: "big", Count: 1},
			expected: -1000,
		},
		{
			name:     "uint32 라운드트립",
			value:    1000000,
			reg:      RegisterDef{DataType: "uint32", ByteOrder: "big", Count: 2},
			expected: 1000000,
		},
		{
			name:     "int32 음수 라운드트립",
			value:    -1000000,
			reg:      RegisterDef{DataType: "int32", ByteOrder: "big", Count: 2},
			expected: -1000000,
		},
		{
			name:     "float32 라운드트립",
			value:    float64(float32(3.14)),
			reg:      RegisterDef{DataType: "float32", ByteOrder: "big", Count: 2},
			expected: float64(float32(3.14)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bytes, err := valueToRegisters(tt.value, tt.reg)
			require.NoError(t, err)

			got, err := registersToValue(bytes, tt.reg)
			require.NoError(t, err)
			assert.InDelta(t, tt.expected, got, 1e-6)
		})
	}
}

// --- TransformToFlow 테스트 ---

// TestModbusAdapter_TransformToFlow 는 레지스터 데이터가 Payload 값으로 올바르게 변환되는지 확인한다.
func TestModbusAdapter_TransformToFlow(t *testing.T) {
	regs := []RegisterDef{
		{Name: "temperature", Address: 0, Count: 1, DataType: "uint16", ByteOrder: "big", Scale: 0.1, Offset: 0},
		{Name: "humidity", Address: 1, Count: 1, DataType: "uint16", ByteOrder: "big", Scale: 0.1, Offset: 0},
	}
	adapter := NewModbusAdapter(WithUnitID(1), WithRegisters(regs))

	// temperature=250 (25.0 * 0.1 = 25.0), humidity=600 (60.0 * 0.1 = 60.0)
	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], 250)
	binary.BigEndian.PutUint16(data[2:4], 600)

	meta := node.AgentMeta{AgentType: "modbus", UnitID: 1, FunctionCode: 3}

	msg, err := adapter.TransformToFlow(data, meta)
	require.NoError(t, err)

	tempVal, ok := msg.Payload().Get("temperature")
	require.True(t, ok)
	assert.InDelta(t, 25.0, tempVal, 1e-6)

	humVal, ok := msg.Payload().Get("humidity")
	require.True(t, ok)
	assert.InDelta(t, 60.0, humVal, 1e-6)
}

// TestModbusAdapter_TransformToFlow_WithOffset 는 Scale과 Offset이 함께 적용되는지 확인한다.
func TestModbusAdapter_TransformToFlow_WithOffset(t *testing.T) {
	regs := []RegisterDef{
		{Name: "adjusted", Address: 0, Count: 1, DataType: "uint16", ByteOrder: "big", Scale: 2.0, Offset: 10.0},
	}
	adapter := NewModbusAdapter(WithUnitID(1), WithRegisters(regs))

	data := make([]byte, 2)
	binary.BigEndian.PutUint16(data, 5) // raw=5, result = 5*2.0 + 10.0 = 20.0

	msg, err := adapter.TransformToFlow(data, node.AgentMeta{})
	require.NoError(t, err)

	val, ok := msg.Payload().Get("adjusted")
	require.True(t, ok)
	assert.InDelta(t, 20.0, val, 1e-6)
}

// TestModbusAdapter_TransformToFlow_NilData 는 nil 데이터에 대해 빈 메시지를 반환하는지 확인한다.
func TestModbusAdapter_TransformToFlow_NilData(t *testing.T) {
	regs := []RegisterDef{
		{Name: "temp", Address: 0, Count: 1, DataType: "uint16"},
	}
	adapter := NewModbusAdapter(WithUnitID(1), WithRegisters(regs))

	msg, err := adapter.TransformToFlow(nil, node.AgentMeta{})
	require.NoError(t, err)
	assert.Empty(t, msg.Payload().Keys())
}

// TestModbusAdapter_TransformToFlow_PartialData 는 부분 데이터에서 가능한 레지스터만 변환하는지 확인한다.
func TestModbusAdapter_TransformToFlow_PartialData(t *testing.T) {
	regs := []RegisterDef{
		{Name: "first", Address: 0, Count: 1, DataType: "uint16", ByteOrder: "big"},
		{Name: "second", Address: 1, Count: 2, DataType: "float32", ByteOrder: "big"},
	}
	adapter := NewModbusAdapter(WithUnitID(1), WithRegisters(regs))

	// 첫 번째 레지스터만 충분한 데이터
	data := make([]byte, 2)
	binary.BigEndian.PutUint16(data, 42)

	msg, err := adapter.TransformToFlow(data, node.AgentMeta{})
	require.NoError(t, err)

	val, ok := msg.Payload().Get("first")
	require.True(t, ok)
	assert.InDelta(t, 42.0, val, 1e-6)

	_, ok = msg.Payload().Get("second")
	assert.False(t, ok, "데이터 부족한 두 번째 레지스터는 설정되지 않아야 한다")
}

// TestModbusAdapter_TransformToFlow_MetadataSet 는 AgentMeta가 메시지 메타데이터로 설정되는지 확인한다.
func TestModbusAdapter_TransformToFlow_MetadataSet(t *testing.T) {
	regs := []RegisterDef{
		{Name: "temp", Address: 0, Count: 1, DataType: "uint16", ByteOrder: "big"},
	}
	adapter := NewModbusAdapter(WithUnitID(1), WithRegisters(regs))

	data := make([]byte, 2)
	binary.BigEndian.PutUint16(data, 100)

	meta := node.AgentMeta{
		UnitID:        5,
		FunctionCode:  3,
		RegisterAddr:  100,
		RegisterCount: 10,
	}

	msg, err := adapter.TransformToFlow(data, meta)
	require.NoError(t, err)

	v, ok := msg.Metadata().Get("modbus.unit_id")
	assert.True(t, ok)
	assert.Equal(t, "5", v)

	v, ok = msg.Metadata().Get("modbus.function_code")
	assert.True(t, ok)
	assert.Equal(t, "3", v)
}

// --- TransformToAgent 테스트 ---

// TestModbusAdapter_TransformToAgent 는 Payload 값이 레지스터 바이트로 올바르게 변환되는지 확인한다.
func TestModbusAdapter_TransformToAgent(t *testing.T) {
	regs := []RegisterDef{
		{Name: "temperature", Address: 0, Count: 1, DataType: "uint16", ByteOrder: "big", Scale: 0.1},
		{Name: "setpoint", Address: 1, Count: 1, DataType: "uint16", ByteOrder: "big", Scale: 0.1},
	}
	adapter := NewModbusAdapter(WithUnitID(10), WithRegisters(regs))

	msg := message.New()
	msg.Payload().Set("temperature", 25.0) // raw = 25.0 / 0.1 = 250
	msg.Payload().Set("setpoint", 60.0)    // raw = 60.0 / 0.1 = 600

	data, meta, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)

	// bulk_write JSON 커맨드 검증
	var req bulkWriteRequest
	require.NoError(t, json.Unmarshal(data, &req))
	assert.Equal(t, "bulk_write", req.Command)

	writes, ok := req.Params["writes"]
	require.True(t, ok)
	writesSlice, ok := writes.([]any)
	require.True(t, ok)
	require.Len(t, writesSlice, 2)

	// temperature: raw = 25.0 / 0.1 = 250
	w0 := writesSlice[0].(map[string]any)
	assert.Equal(t, "holding_registers", w0["area"])
	assert.Equal(t, float64(0), w0["address"])
	assert.InDelta(t, 250.0, w0["value"].(float64), 1e-9)
	assert.Equal(t, "uint16", w0["data_type"])
	assert.Equal(t, "big_endian", w0["byte_order"])

	// setpoint: raw = 60.0 / 0.1 = 600
	w1 := writesSlice[1].(map[string]any)
	assert.Equal(t, "holding_registers", w1["area"])
	assert.Equal(t, float64(1), w1["address"])
	assert.InDelta(t, 600.0, w1["value"].(float64), 1e-9)

	// 메타데이터 검증
	assert.Equal(t, "modbus", meta.AgentType)
	assert.Equal(t, uint8(10), meta.UnitID)
}

// TestModbusAdapter_TransformToAgent_SingleRegister 는 단일 레지스터 쓰기 시 JSON 커맨드를 생성하는지 확인한다.
func TestModbusAdapter_TransformToAgent_SingleRegister(t *testing.T) {
	regs := []RegisterDef{
		{Name: "setpoint", Address: 0, Count: 1, DataType: "uint16", ByteOrder: "big"},
	}
	adapter := NewModbusAdapter(WithUnitID(1), WithRegisters(regs))

	msg := message.New()
	msg.Payload().Set("setpoint", float64(100))

	data, meta, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)

	// bulk_write JSON 커맨드 검증
	var req bulkWriteRequest
	require.NoError(t, json.Unmarshal(data, &req))
	assert.Equal(t, "bulk_write", req.Command)

	writes := req.Params["writes"].([]any)
	require.Len(t, writes, 1)

	w0 := writes[0].(map[string]any)
	assert.Equal(t, "holding_registers", w0["area"])
	assert.InDelta(t, 100.0, w0["value"].(float64), 1e-9)
	assert.Equal(t, "modbus", meta.AgentType)
}

// TestModbusAdapter_TransformToAgent_ReadOnly 는 ReadOnly 레지스터가 건너뛰어지는지 확인한다.
func TestModbusAdapter_TransformToAgent_ReadOnly(t *testing.T) {
	regs := []RegisterDef{
		{Name: "temperature", Address: 0, Count: 1, DataType: "uint16", ByteOrder: "big", ReadOnly: true},
		{Name: "setpoint", Address: 1, Count: 1, DataType: "uint16", ByteOrder: "big"},
	}
	adapter := NewModbusAdapter(WithUnitID(1), WithRegisters(regs))

	msg := message.New()
	msg.Payload().Set("temperature", float64(250)) // 읽기 전용 -> 무시됨
	msg.Payload().Set("setpoint", float64(600))

	data, _, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)

	// 읽기 전용 제외, setpoint만 포함된 bulk_write 커맨드
	var req bulkWriteRequest
	require.NoError(t, json.Unmarshal(data, &req))
	assert.Equal(t, "bulk_write", req.Command)

	writes := req.Params["writes"].([]any)
	require.Len(t, writes, 1)

	w0 := writes[0].(map[string]any)
	assert.Equal(t, float64(1), w0["address"]) // setpoint의 주소
	assert.InDelta(t, 600.0, w0["value"].(float64), 1e-9)
}

// TestModbusAdapter_TransformToAgent_MissingPayload 는 Payload에 없는 레지스터는 건너뛰는지 확인한다.
func TestModbusAdapter_TransformToAgent_MissingPayload(t *testing.T) {
	regs := []RegisterDef{
		{Name: "temp", Address: 0, Count: 1, DataType: "uint16", ByteOrder: "big"},
		{Name: "humidity", Address: 1, Count: 1, DataType: "uint16", ByteOrder: "big"},
	}
	adapter := NewModbusAdapter(WithUnitID(1), WithRegisters(regs))

	msg := message.New()
	msg.Payload().Set("temp", float64(100)) // humidity는 미설정

	data, _, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)

	// temp만 포함된 bulk_write 커맨드
	var req bulkWriteRequest
	require.NoError(t, json.Unmarshal(data, &req))
	assert.Equal(t, "bulk_write", req.Command)

	writes := req.Params["writes"].([]any)
	require.Len(t, writes, 1)

	w0 := writes[0].(map[string]any)
	assert.Equal(t, float64(0), w0["address"]) // temp의 주소
	assert.InDelta(t, 100.0, w0["value"].(float64), 1e-9)
}

// TestModbusAdapter_TransformToAgent_WithScaleOffset 는 역 Scale/Offset이 올바르게 적용되는지 확인한다.
func TestModbusAdapter_TransformToAgent_WithScaleOffset(t *testing.T) {
	regs := []RegisterDef{
		{Name: "value", Address: 0, Count: 1, DataType: "uint16", ByteOrder: "big", Scale: 2.0, Offset: 10.0},
	}
	adapter := NewModbusAdapter(WithUnitID(1), WithRegisters(regs))

	msg := message.New()
	msg.Payload().Set("value", 20.0) // raw = (20.0 - 10.0) / 2.0 = 5

	data, _, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)

	// bulk_write JSON 커맨드에서 역 Scale/Offset이 적용되었는지 검증
	var req bulkWriteRequest
	require.NoError(t, json.Unmarshal(data, &req))
	assert.Equal(t, "bulk_write", req.Command)

	writes := req.Params["writes"].([]any)
	require.Len(t, writes, 1)

	w0 := writes[0].(map[string]any)
	assert.InDelta(t, 5.0, w0["value"].(float64), 1e-9) // (20.0 - 10.0) / 2.0 = 5
}

// --- Scale/Offset 테스트 ---

// TestApplyScaleOffset 은 Scale과 Offset 적용이 올바른지 확인한다.
func TestApplyScaleOffset(t *testing.T) {
	tests := []struct {
		name   string
		raw    float64
		scale  float64
		offset float64
		want   float64
	}{
		{name: "기본 Scale=1, Offset=0", raw: 100, scale: 1.0, offset: 0, want: 100},
		{name: "Scale=0.1", raw: 250, scale: 0.1, offset: 0, want: 25.0},
		{name: "Scale=2, Offset=10", raw: 5, scale: 2.0, offset: 10.0, want: 20.0},
		{name: "Scale=0은 1로 취급", raw: 100, scale: 0, offset: 0, want: 100},
		{name: "음수 Offset", raw: 100, scale: 1.0, offset: -50, want: 50},
		{name: "음수 Scale", raw: 100, scale: -1.0, offset: 0, want: -100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyScaleOffset(tt.raw, tt.scale, tt.offset)
			assert.InDelta(t, tt.want, got, 1e-9)
		})
	}
}

// TestReverseScaleOffset 은 역 Scale/Offset 적용이 올바른지 확인한다.
func TestReverseScaleOffset(t *testing.T) {
	tests := []struct {
		name   string
		val    any
		scale  float64
		offset float64
		want   float64
	}{
		{name: "기본 Scale=1, Offset=0 float64", val: float64(100), scale: 1.0, offset: 0, want: 100},
		{name: "Scale=0.1 float64", val: float64(25.0), scale: 0.1, offset: 0, want: 250},
		{name: "Scale=2, Offset=10 float64", val: float64(20.0), scale: 2.0, offset: 10.0, want: 5.0},
		{name: "Scale=0은 1로 취급", val: float64(100), scale: 0, offset: 0, want: 100},
		{name: "int 타입", val: int(50), scale: 1.0, offset: 0, want: 50},
		{name: "int16 타입", val: int16(100), scale: 1.0, offset: 0, want: 100},
		{name: "uint16 타입", val: uint16(200), scale: 1.0, offset: 0, want: 200},
		{name: "float32 타입", val: float32(3.14), scale: 1.0, offset: 0, want: float64(float32(3.14))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reverseScaleOffset(tt.val, tt.scale, tt.offset)
			assert.InDelta(t, tt.want, got, 1e-4)
		})
	}
}

// TestReverseScaleOffset_Roundtrip 은 applyScaleOffset과 reverseScaleOffset이 역변환 관계인지 확인한다.
func TestReverseScaleOffset_Roundtrip(t *testing.T) {
	tests := []struct {
		name   string
		raw    float64
		scale  float64
		offset float64
	}{
		{name: "Scale=0.1", raw: 250, scale: 0.1, offset: 0},
		{name: "Scale=2, Offset=10", raw: 5, scale: 2.0, offset: 10.0},
		{name: "Scale=1, Offset=-50", raw: 100, scale: 1.0, offset: -50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scaled := applyScaleOffset(tt.raw, tt.scale, tt.offset)
			restored := reverseScaleOffset(scaled, tt.scale, tt.offset)
			assert.InDelta(t, tt.raw, restored, 1e-9)
		})
	}
}

// --- DefaultConfig / HandleControl 테스트 ---

// TestModbusAdapter_DefaultConfig 는 기본 설정이 올바른지 확인한다.
func TestModbusAdapter_DefaultConfig(t *testing.T) {
	adapter := NewModbusAdapter()
	cfg := adapter.DefaultConfig()
	assert.Equal(t, flow.BridgeInOut, cfg.Direction)
	assert.Equal(t, 64, cfg.BufferSize)
}

// TestModbusAdapter_HandleControl 은 HandleControl이 nil을 반환하는지 확인한다.
func TestModbusAdapter_HandleControl(t *testing.T) {
	adapter := NewModbusAdapter(WithUnitID(1))
	msg := message.New()
	err := adapter.HandleControl(msg)
	assert.NoError(t, err)
}

// --- BridgeAdapter 인터페이스 구현 확인 ---

// TestModbusAdapter_ImplementsBridgeAdapter 는 ModbusAdapter가 BridgeAdapter 인터페이스를 구현하는지 확인한다.
func TestModbusAdapter_ImplementsBridgeAdapter(t *testing.T) {
	var _ node.BridgeAdapter = &ModbusAdapter{}
}

// --- toFloat64 테스트 ---

// TestToFloat64 은 다양한 숫자 타입의 float64 변환을 검증한다.
func TestToFloat64(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want float64
	}{
		{name: "float64", val: float64(3.14), want: 3.14},
		{name: "float32", val: float32(2.5), want: float64(float32(2.5))},
		{name: "int", val: int(42), want: 42},
		{name: "int8", val: int8(-10), want: -10},
		{name: "int16", val: int16(-100), want: -100},
		{name: "int32", val: int32(100000), want: 100000},
		{name: "int64", val: int64(1000000), want: 1000000},
		{name: "uint", val: uint(42), want: 42},
		{name: "uint8", val: uint8(255), want: 255},
		{name: "uint16", val: uint16(65535), want: 65535},
		{name: "uint32", val: uint32(100000), want: 100000},
		{name: "uint64", val: uint64(1000000), want: 1000000},
		{name: "string은 0", val: "hello", want: 0},
		{name: "nil은 0", val: nil, want: 0},
		{name: "bool은 0", val: true, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toFloat64(tt.val)
			assert.InDelta(t, tt.want, got, 1e-6)
		})
	}
}

// --- getByteOrder 테스트 ---

// TestGetByteOrder 는 바이트 오더 선택이 올바른지 확인한다.
func TestGetByteOrder(t *testing.T) {
	assert.Equal(t, binary.BigEndian, getByteOrder("big"))
	assert.Equal(t, binary.LittleEndian, getByteOrder("little"))
	assert.Equal(t, binary.BigEndian, getByteOrder(""))      // 기본값
	assert.Equal(t, binary.BigEndian, getByteOrder("other"))  // 기본값
}

// --- AgentConfigurable 인터페이스 및 ConfigureFromAgent 테스트 ---

// TestModbusAdapter_ImplementsAgentConfigurable 은 *ModbusAdapter가 node.AgentConfigurable 인터페이스를 구현하는지 확인한다.
func TestModbusAdapter_ImplementsAgentConfigurable(t *testing.T) {
	var _ node.AgentConfigurable = &ModbusAdapter{}
}

// TestModbusAdapter_ConfigureFromAgent_정상 은 유효한 register_defs와 unit_id로 새 어댑터가 생성되는지 확인한다.
func TestModbusAdapter_ConfigureFromAgent_정상(t *testing.T) {
	original := NewModbusAdapter(
		WithUnitID(1),
		WithRegisters([]RegisterDef{
			{Name: "old", Address: 0, Count: 1, DataType: "uint16"},
		}),
		WithPollingInterval(500*time.Millisecond),
	)

	agentConfig := map[string]any{
		"unit_id": float64(10),
		"register_defs": []any{
			map[string]any{
				"name":      "temperature",
				"address":   float64(100),
				"count":     float64(1),
				"data_type": "uint16",
			},
			map[string]any{
				"name":       "humidity",
				"address":    float64(101),
				"count":      float64(2),
				"data_type":  "float32",
				"byte_order": "little",
				"scale":      float64(0.1),
				"offset":     float64(5.0),
				"read_only":  true,
			},
		},
	}

	result, err := original.ConfigureFromAgent(agentConfig)
	require.NoError(t, err)

	// 새 어댑터가 반환되어야 한다 (원본과 다른 인스턴스)
	assert.NotSame(t, original, result)

	newAdapter, ok := result.(*ModbusAdapter)
	require.True(t, ok, "반환된 어댑터가 *ModbusAdapter 타입이어야 한다")

	assert.Equal(t, uint8(10), newAdapter.unitID)
	require.Len(t, newAdapter.registers, 2)

	// 첫 번째 레지스터 검증
	assert.Equal(t, "temperature", newAdapter.registers[0].Name)
	assert.Equal(t, uint16(100), newAdapter.registers[0].Address)
	assert.Equal(t, uint16(1), newAdapter.registers[0].Count)
	assert.Equal(t, "uint16", newAdapter.registers[0].DataType)

	// 두 번째 레지스터 검증
	assert.Equal(t, "humidity", newAdapter.registers[1].Name)
	assert.Equal(t, uint16(101), newAdapter.registers[1].Address)
	assert.Equal(t, uint16(2), newAdapter.registers[1].Count)
	assert.Equal(t, "float32", newAdapter.registers[1].DataType)
	assert.Equal(t, "little", newAdapter.registers[1].ByteOrder)
	assert.InDelta(t, 0.1, newAdapter.registers[1].Scale, 1e-9)
	assert.InDelta(t, 5.0, newAdapter.registers[1].Offset, 1e-9)
	assert.True(t, newAdapter.registers[1].ReadOnly)

	// 폴링 간격이 원본에서 복사되었는지 확인
	assert.Equal(t, 500*time.Millisecond, newAdapter.pollingInterval)
}

// TestModbusAdapter_ConfigureFromAgent_RegisterDefs없으면_원본반환 은 register_defs가 없으면 원본 어댑터를 반환하는지 확인한다.
func TestModbusAdapter_ConfigureFromAgent_RegisterDefs없으면_원본반환(t *testing.T) {
	original := NewModbusAdapter(
		WithUnitID(5),
		WithRegisters([]RegisterDef{
			{Name: "temp", Address: 0, Count: 1, DataType: "uint16"},
		}),
	)

	// register_defs 키가 없는 설정
	agentConfig := map[string]any{
		"unit_id": float64(10),
	}

	result, err := original.ConfigureFromAgent(agentConfig)
	require.NoError(t, err)

	// 원본과 동일한 인스턴스가 반환되어야 한다
	assert.Same(t, original, result)
}

// TestModbusAdapter_ConfigureFromAgent_UnitID추출 은 다양한 숫자 타입의 unit_id가 올바르게 추출되는지 확인한다.
func TestModbusAdapter_ConfigureFromAgent_UnitID추출(t *testing.T) {
	tests := []struct {
		name       string
		unitIDVal  any
		wantUnitID uint8
	}{
		{
			name:       "float64 타입 unit_id",
			unitIDVal:  float64(10),
			wantUnitID: 10,
		},
		{
			name:       "int 타입 unit_id",
			unitIDVal:  int(20),
			wantUnitID: 20,
		},
		{
			name:       "int64 타입 unit_id",
			unitIDVal:  int64(30),
			wantUnitID: 30,
		},
		{
			name:       "uint8 타입 unit_id",
			unitIDVal:  uint8(40),
			wantUnitID: 40,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := NewModbusAdapter(WithUnitID(1))

			agentConfig := map[string]any{
				"unit_id": tt.unitIDVal,
				"register_defs": []any{
					map[string]any{
						"name":      "temp",
						"address":   float64(0),
						"count":     float64(1),
						"data_type": "uint16",
					},
				},
			}

			result, err := original.ConfigureFromAgent(agentConfig)
			require.NoError(t, err)

			newAdapter, ok := result.(*ModbusAdapter)
			require.True(t, ok)
			assert.Equal(t, tt.wantUnitID, newAdapter.unitID)
		})
	}

	// unit_id 미지정 시 원본의 unitID 사용
	t.Run("unit_id 미지정 시 원본 unitID 사용", func(t *testing.T) {
		original := NewModbusAdapter(WithUnitID(99))

		agentConfig := map[string]any{
			"register_defs": []any{
				map[string]any{
					"name":      "temp",
					"address":   float64(0),
					"count":     float64(1),
					"data_type": "uint16",
				},
			},
		}

		result, err := original.ConfigureFromAgent(agentConfig)
		require.NoError(t, err)

		newAdapter, ok := result.(*ModbusAdapter)
		require.True(t, ok)
		assert.Equal(t, uint8(99), newAdapter.unitID)
	})
}

// TestModbusAdapter_ConfigureFromAgent_CountAutoInfer 는 count 미지정 시 data_type으로부터 자동 추론되는지 확인한다.
func TestModbusAdapter_ConfigureFromAgent_CountAutoInfer(t *testing.T) {
	tests := []struct {
		name      string
		dataType  string
		wantCount uint16
	}{
		{name: "uint16은 count=1", dataType: "uint16", wantCount: 1},
		{name: "int16은 count=1", dataType: "int16", wantCount: 1},
		{name: "float32은 count=2", dataType: "float32", wantCount: 2},
		{name: "uint32은 count=2", dataType: "uint32", wantCount: 2},
		{name: "int32은 count=2", dataType: "int32", wantCount: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := NewModbusAdapter(WithUnitID(1))

			// count를 명시하지 않는 register_defs
			agentConfig := map[string]any{
				"register_defs": []any{
					map[string]any{
						"name":      "auto_count_reg",
						"address":   float64(0),
						"data_type": tt.dataType,
						// count 미지정
					},
				},
			}

			result, err := original.ConfigureFromAgent(agentConfig)
			require.NoError(t, err)

			newAdapter, ok := result.(*ModbusAdapter)
			require.True(t, ok)
			require.Len(t, newAdapter.registers, 1)
			assert.Equal(t, tt.wantCount, newAdapter.registers[0].Count,
				"data_type %q에 대한 자동 추론 count가 %d이어야 한다", tt.dataType, tt.wantCount)
		})
	}

	// count가 명시된 경우 명시된 값을 사용
	t.Run("count 명시 시 명시된 값 사용", func(t *testing.T) {
		original := NewModbusAdapter(WithUnitID(1))

		agentConfig := map[string]any{
			"register_defs": []any{
				map[string]any{
					"name":      "explicit_count",
					"address":   float64(0),
					"count":     float64(3),
					"data_type": "uint16",
				},
			},
		}

		result, err := original.ConfigureFromAgent(agentConfig)
		require.NoError(t, err)

		newAdapter, ok := result.(*ModbusAdapter)
		require.True(t, ok)
		require.Len(t, newAdapter.registers, 1)
		assert.Equal(t, uint16(3), newAdapter.registers[0].Count,
			"명시된 count가 우선되어야 한다")
	})
}

// TestModbusAdapter_ConfigureFromAgent_InvalidRegisterDefs 는 register_defs가 배열이 아닌 경우 에러를 반환하는지 확인한다.
func TestModbusAdapter_ConfigureFromAgent_InvalidRegisterDefs(t *testing.T) {
	tests := []struct {
		name        string
		registerDef any
		wantErr     string
	}{
		{
			name:        "문자열이면 에러",
			registerDef: "not-an-array",
			wantErr:     "register_defs must be an array",
		},
		{
			name:        "숫자이면 에러",
			registerDef: float64(42),
			wantErr:     "register_defs must be an array",
		},
		{
			name:        "맵이면 에러",
			registerDef: map[string]any{"name": "temp"},
			wantErr:     "register_defs must be an array",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := NewModbusAdapter(WithUnitID(1))

			agentConfig := map[string]any{
				"register_defs": tt.registerDef,
			}

			_, err := original.ConfigureFromAgent(agentConfig)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// --- parseRegisterDefs 테스트 ---

// TestParseRegisterDefs_정상 은 유효한 register_defs 배열이 올바르게 파싱되는지 확인한다.
func TestParseRegisterDefs_정상(t *testing.T) {
	raw := []any{
		map[string]any{
			"name":       "temperature",
			"address":    float64(100),
			"count":      float64(1),
			"data_type":  "uint16",
			"byte_order": "big",
			"scale":      float64(0.1),
			"offset":     float64(-40.0),
			"read_only":  true,
		},
		map[string]any{
			"name":      "setpoint",
			"address":   float64(200),
			"data_type": "float32",
			// count 미지정 -> data_type으로 추론 (float32 = 2)
		},
		map[string]any{
			"name":      "minimal",
			"data_type": "int16",
			// 최소한의 필드만 설정
		},
	}

	regs, err := parseRegisterDefs(raw)
	require.NoError(t, err)
	require.Len(t, regs, 3)

	// 첫 번째: 모든 필드 지정
	assert.Equal(t, "temperature", regs[0].Name)
	assert.Equal(t, uint16(100), regs[0].Address)
	assert.Equal(t, uint16(1), regs[0].Count)
	assert.Equal(t, "uint16", regs[0].DataType)
	assert.Equal(t, "big", regs[0].ByteOrder)
	assert.InDelta(t, 0.1, regs[0].Scale, 1e-9)
	assert.InDelta(t, -40.0, regs[0].Offset, 1e-9)
	assert.True(t, regs[0].ReadOnly)

	// 두 번째: count 자동 추론
	assert.Equal(t, "setpoint", regs[1].Name)
	assert.Equal(t, uint16(200), regs[1].Address)
	assert.Equal(t, uint16(2), regs[1].Count) // float32 -> count=2
	assert.Equal(t, "float32", regs[1].DataType)
	assert.InDelta(t, 1.0, regs[1].Scale, 1e-9) // 기본값

	// 세 번째: 최소 필드
	assert.Equal(t, "minimal", regs[2].Name)
	assert.Equal(t, uint16(0), regs[2].Address) // 미지정 -> 0
	assert.Equal(t, uint16(1), regs[2].Count)   // int16 -> count=1
	assert.Equal(t, "int16", regs[2].DataType)
}

// TestParseRegisterDefs_에러케이스 는 잘못된 register_defs에서 에러가 반환되는지 확인한다.
func TestParseRegisterDefs_에러케이스(t *testing.T) {
	tests := []struct {
		name    string
		raw     any
		wantErr string
	}{
		{
			name:    "배열이 아닌 문자열",
			raw:     "not-an-array",
			wantErr: "register_defs must be an array",
		},
		{
			name:    "배열이 아닌 숫자",
			raw:     42,
			wantErr: "register_defs must be an array",
		},
		{
			name: "배열 내 요소가 맵이 아닌 경우",
			raw: []any{
				"not-a-map",
			},
			wantErr: "register_defs[0] must be an object",
		},
		{
			name: "배열 내 두 번째 요소가 맵이 아닌 경우",
			raw: []any{
				map[string]any{"name": "ok", "data_type": "uint16"},
				42,
			},
			wantErr: "register_defs[1] must be an object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRegisterDefs(tt.raw)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
