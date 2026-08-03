package modbusserver

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	modbus "github.com/xtra/xflow/internal/modbus"
)

// ---------------------------------------------------------------------------
// parseModbusServerConfig 테스트
// ---------------------------------------------------------------------------

func TestParseModbusServerConfig_ValidConfig(t *testing.T) {
	// 모든 필드를 명시적으로 설정한 완전한 설정을 파싱한다.
	opts := map[string]any{
		"listen_address":   "192.168.1.100",
		"listen_port":      float64(5020),
		"unit_id":          float64(10),
		"max_connections":  float64(20),
		"idle_timeout":     "30s",
		"msg_channel_size": float64(512),
		"register_map": map[string]any{
			"coils": map[string]any{
				"start_address":  float64(0),
				"count":          float64(100),
				"initial_values": []any{true, false, true},
			},
			"discrete_inputs": map[string]any{
				"start_address": float64(0),
				"count":         float64(50),
			},
			"holding_registers": map[string]any{
				"start_address":  float64(100),
				"count":          float64(10),
				"initial_values": []any{float64(100), float64(200), float64(300)},
			},
			"input_registers": map[string]any{
				"start_address": float64(200),
				"count":         float64(20),
			},
		},
	}

	cfg, err := parseModbusServerConfig(opts)
	require.NoError(t, err)

	assert.Equal(t, "192.168.1.100", cfg.ListenAddress)
	assert.Equal(t, 5020, cfg.ListenPort)
	assert.Equal(t, byte(10), cfg.UnitID)
	assert.Equal(t, 20, cfg.MaxConnections)
	assert.Equal(t, 30*time.Second, cfg.IdleTimeout)
	assert.Equal(t, 512, cfg.MsgChannelSize)

	// 코일 영역 확인
	require.Len(t, cfg.RegisterMap.Coils, 1)
	assert.Equal(t, uint16(0), cfg.RegisterMap.Coils[0].StartAddress)
	assert.Equal(t, uint16(100), cfg.RegisterMap.Coils[0].Count)
	assert.Equal(t, []any{true, false, true}, cfg.RegisterMap.Coils[0].InitialValues)

	// 이산 입력 영역 확인
	require.Len(t, cfg.RegisterMap.DiscreteInputs, 1)
	assert.Equal(t, uint16(0), cfg.RegisterMap.DiscreteInputs[0].StartAddress)
	assert.Equal(t, uint16(50), cfg.RegisterMap.DiscreteInputs[0].Count)

	// 보유 레지스터 영역 확인
	require.Len(t, cfg.RegisterMap.HoldingRegisters, 1)
	assert.Equal(t, uint16(100), cfg.RegisterMap.HoldingRegisters[0].StartAddress)
	assert.Equal(t, uint16(10), cfg.RegisterMap.HoldingRegisters[0].Count)
	assert.Equal(t, []any{float64(100), float64(200), float64(300)}, cfg.RegisterMap.HoldingRegisters[0].InitialValues)

	// 입력 레지스터 영역 확인
	require.Len(t, cfg.RegisterMap.InputRegisters, 1)
	assert.Equal(t, uint16(200), cfg.RegisterMap.InputRegisters[0].StartAddress)
	assert.Equal(t, uint16(20), cfg.RegisterMap.InputRegisters[0].Count)
}

func TestParseModbusServerConfig_DefaultValues(t *testing.T) {
	// register_map 만 제공하고 나머지는 기본값을 사용한다.
	opts := map[string]any{
		"register_map": map[string]any{
			"holding_registers": map[string]any{
				"start_address": float64(0),
				"count":         float64(10),
			},
		},
	}

	cfg, err := parseModbusServerConfig(opts)
	require.NoError(t, err)

	assert.Equal(t, "0.0.0.0", cfg.ListenAddress)
	assert.Equal(t, 502, cfg.ListenPort)
	assert.Equal(t, byte(1), cfg.UnitID)
	assert.Equal(t, 10, cfg.MaxConnections)
	assert.Equal(t, 60*time.Second, cfg.IdleTimeout)
	assert.Equal(t, 256, cfg.MsgChannelSize)
}

func TestParseModbusServerConfig_MissingRegisterMap(t *testing.T) {
	// register_map 이 없어도 role=main 은 zero-device 서버로 구성된다(오류 아님).
	// (디바이스는 생성 후 config 업데이트로 추가된다.)
	opts := map[string]any{
		"listen_port": float64(5020),
	}

	cfg, err := parseModbusServerConfig(opts)
	require.NoError(t, err)
	assert.Empty(t, cfg.Devices, "register_map 없는 main 은 zero-device")
}

func TestParseModbusServerConfig_InvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port float64
	}{
		{"포트 70000", 70000},
		{"음수 포트", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := map[string]any{
				"listen_port": tt.port,
				"register_map": map[string]any{
					"holding_registers": map[string]any{
						"start_address": float64(0),
						"count":         float64(10),
					},
				},
			}

			_, err := parseModbusServerConfig(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "listen_port")
		})
	}
}

func TestParseModbusServerConfig_InvalidUnitID(t *testing.T) {
	// unit_id 248 은 유효 범위(0-247)를 벗어난다.
	opts := map[string]any{
		"unit_id": float64(248),
		"register_map": map[string]any{
			"holding_registers": map[string]any{
				"start_address": float64(0),
				"count":         float64(10),
			},
		},
	}

	_, err := parseModbusServerConfig(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unit_id")
}

func TestParseModbusServerConfig_InvalidMaxConnections(t *testing.T) {
	tests := []struct {
		name string
		val  float64
	}{
		{"0", 0},
		{"음수", -5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := map[string]any{
				"max_connections": tt.val,
				"register_map": map[string]any{
					"holding_registers": map[string]any{
						"start_address": float64(0),
						"count":         float64(10),
					},
				},
			}

			_, err := parseModbusServerConfig(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "max_connections")
		})
	}
}

func TestParseModbusServerConfig_InitialValues(t *testing.T) {
	// 초기값이 올바르게 파싱되는지 확인한다.
	opts := map[string]any{
		"register_map": map[string]any{
			"coils": map[string]any{
				"start_address":  float64(0),
				"count":          float64(5),
				"initial_values": []any{true, false, true, false, true},
			},
			"holding_registers": map[string]any{
				"start_address":  float64(0),
				"count":          float64(3),
				"initial_values": []any{float64(1000), float64(2000), float64(3000)},
			},
		},
	}

	cfg, err := parseModbusServerConfig(opts)
	require.NoError(t, err)

	require.Len(t, cfg.RegisterMap.Coils, 1)
	assert.Len(t, cfg.RegisterMap.Coils[0].InitialValues, 5)

	require.Len(t, cfg.RegisterMap.HoldingRegisters, 1)
	assert.Len(t, cfg.RegisterMap.HoldingRegisters[0].InitialValues, 3)
}

func TestParseModbusServerConfig_InitialValuesExceedCount(t *testing.T) {
	// initial_values 길이가 count 를 초과하면 에러를 반환해야 한다.
	tests := []struct {
		name string
		area string
		opts map[string]any
	}{
		{
			"코일 초기값 초과",
			"coils",
			map[string]any{
				"register_map": map[string]any{
					"coils": map[string]any{
						"start_address":  float64(0),
						"count":          float64(2),
						"initial_values": []any{true, false, true}, // count=2 인데 3개
					},
				},
			},
		},
		{
			"보유 레지스터 초기값 초과",
			"holding_registers",
			map[string]any{
				"register_map": map[string]any{
					"holding_registers": map[string]any{
						"start_address":  float64(0),
						"count":          float64(1),
						"initial_values": []any{float64(100), float64(200)}, // count=1 인데 2개
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseModbusServerConfig(tt.opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "initial_values")
		})
	}
}

func TestParseModbusServerConfig_EmptyRegisterMap(t *testing.T) {
	// register_map 에 영역이 하나도 없으면 에러를 반환해야 한다.
	opts := map[string]any{
		"register_map": map[string]any{},
	}

	_, err := parseModbusServerConfig(opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRegisterMap)
}

func TestParseModbusServerConfig_ZeroCount(t *testing.T) {
	// count 가 0 인 영역이 있으면 에러를 반환해야 한다.
	opts := map[string]any{
		"register_map": map[string]any{
			"holding_registers": map[string]any{
				"start_address": float64(0),
				"count":         float64(0),
			},
		},
	}

	_, err := parseModbusServerConfig(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "count")
}

// ---------------------------------------------------------------------------
// 타입 변환 헬퍼 테스트
// ---------------------------------------------------------------------------

func TestToInt(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want int
	}{
		{"int", 42, 42},
		{"float64", float64(42), 42},
		{"string (미지원)", "42", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, toInt(tt.val))
		})
	}
}

func TestToByte(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want byte
	}{
		{"int", 10, byte(10)},
		{"float64", float64(10), byte(10)},
		{"string (미지원)", "10", byte(0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, toByte(tt.val))
		})
	}
}

func TestToUint16(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want uint16
	}{
		{"int", 1000, uint16(1000)},
		{"float64", float64(1000), uint16(1000)},
		{"string (미지원)", "1000", uint16(0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, toUint16(tt.val))
		})
	}
}

// ---------------------------------------------------------------------------
// register_map 파싱 에러 경로 테스트
// ---------------------------------------------------------------------------

func TestParseModbusServerConfig_RegisterMapNotMap(t *testing.T) {
	// register_map 이 맵이 아닌 경우 에러를 반환해야 한다.
	opts := map[string]any{
		"register_map": "invalid",
	}

	_, err := parseModbusServerConfig(opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRegisterMap)
}

func TestParseModbusServerConfig_AreaNotMap(t *testing.T) {
	// 영역이 맵이 아닌 경우 에러를 반환해야 한다.
	areas := []string{"coils", "discrete_inputs", "holding_registers", "input_registers"}
	for _, area := range areas {
		t.Run(area, func(t *testing.T) {
			opts := map[string]any{
				"register_map": map[string]any{
					area: "invalid",
				},
			}
			_, err := parseModbusServerConfig(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), area)
		})
	}
}

func TestParseModbusServerConfig_InvalidIdleTimeout(t *testing.T) {
	opts := map[string]any{
		"idle_timeout": "not-a-duration",
		"register_map": map[string]any{
			"holding_registers": map[string]any{
				"start_address": float64(0),
				"count":         float64(10),
			},
		},
	}

	_, err := parseModbusServerConfig(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "idle_timeout")
}

func TestParseModbusServerConfig_IntTypePorts(t *testing.T) {
	// int 타입으로 전달되는 경우도 처리해야 한다
	opts := map[string]any{
		"listen_port":      502,
		"unit_id":          1,
		"max_connections":  5,
		"msg_channel_size": 128,
		"register_map": map[string]any{
			"holding_registers": map[string]any{
				"start_address": 0,
				"count":         10,
			},
		},
	}

	cfg, err := parseModbusServerConfig(opts)
	require.NoError(t, err)
	assert.Equal(t, 502, cfg.ListenPort)
	assert.Equal(t, byte(1), cfg.UnitID)
	assert.Equal(t, 5, cfg.MaxConnections)
	assert.Equal(t, 128, cfg.MsgChannelSize)
}

// ---------------------------------------------------------------------------
// data_type / type_map 파싱 테스트
// ---------------------------------------------------------------------------

func TestParseModbusServerConfig_DataType(t *testing.T) {
	// data_type 필드가 올바르게 파싱되는지 확인한다.
	opts := map[string]any{
		"register_map": map[string]any{
			"holding_registers": map[string]any{
				"start_address": float64(0),
				"count":         float64(10),
				"data_type":     "float32",
			},
			"input_registers": map[string]any{
				"start_address": float64(0),
				"count":         float64(10),
				"data_type":     "int32",
			},
		},
	}

	cfg, err := parseModbusServerConfig(opts)
	require.NoError(t, err)

	require.Len(t, cfg.RegisterMap.HoldingRegisters, 1)
	assert.Equal(t, "float32", cfg.RegisterMap.HoldingRegisters[0].DataType)

	require.Len(t, cfg.RegisterMap.InputRegisters, 1)
	assert.Equal(t, "int32", cfg.RegisterMap.InputRegisters[0].DataType)
}

func TestParseModbusServerConfig_TypeMap(t *testing.T) {
	// type_map 배열이 올바르게 파싱되는지 확인한다.
	opts := map[string]any{
		"register_map": map[string]any{
			"holding_registers": map[string]any{
				"start_address": float64(0),
				"count":         float64(10),
				"type_map": []any{
					map[string]any{
						"address":   float64(0),
						"data_type": "float32",
					},
					map[string]any{
						"address":    float64(4),
						"data_type":  "int32",
						"byte_order": "little_endian",
					},
				},
			},
		},
	}

	cfg, err := parseModbusServerConfig(opts)
	require.NoError(t, err)

	require.Len(t, cfg.RegisterMap.HoldingRegisters, 1)
	require.Len(t, cfg.RegisterMap.HoldingRegisters[0].TypeMap, 2)

	// 첫 번째 엔트리 확인
	assert.Equal(t, uint16(0), cfg.RegisterMap.HoldingRegisters[0].TypeMap[0].Address)
	assert.Equal(t, "float32", cfg.RegisterMap.HoldingRegisters[0].TypeMap[0].DataType)
	assert.Equal(t, modbus.ByteOrderBigEndian, cfg.RegisterMap.HoldingRegisters[0].TypeMap[0].ByteOrder)

	// 두 번째 엔트리 확인
	assert.Equal(t, uint16(4), cfg.RegisterMap.HoldingRegisters[0].TypeMap[1].Address)
	assert.Equal(t, "int32", cfg.RegisterMap.HoldingRegisters[0].TypeMap[1].DataType)
	assert.Equal(t, "little_endian", cfg.RegisterMap.HoldingRegisters[0].TypeMap[1].ByteOrder)
}

func TestParseModbusServerConfig_TypeMapOverlap(t *testing.T) {
	// float32 at addr 0 은 레지스터 0,1 을 사용한다.
	// int32 at addr 1 은 레지스터 1,2 를 사용한다.
	// → 레지스터 1 에서 겹침 → ErrTypeMapOverlap
	opts := map[string]any{
		"register_map": map[string]any{
			"holding_registers": map[string]any{
				"start_address": float64(0),
				"count":         float64(10),
				"type_map": []any{
					map[string]any{
						"address":   float64(0),
						"data_type": "float32",
					},
					map[string]any{
						"address":   float64(1),
						"data_type": "int32",
					},
				},
			},
		},
	}

	_, err := parseModbusServerConfig(opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTypeMapOverlap)
}

func TestParseModbusServerConfig_TypeMapOutOfRange(t *testing.T) {
	// float32 at addr 9 는 레지스터 9,10 을 사용한다.
	// count=10 이면 범위는 [0, 10) → 레지스터 10 은 범위 초과
	opts := map[string]any{
		"register_map": map[string]any{
			"holding_registers": map[string]any{
				"start_address": float64(0),
				"count":         float64(10),
				"type_map": []any{
					map[string]any{
						"address":   float64(9),
						"data_type": "float32",
					},
				},
			},
		},
	}

	_, err := parseModbusServerConfig(opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTypeMapOutOfRange)
}

func TestParseModbusServerConfig_InvalidDataType(t *testing.T) {
	// data_type "float64" 는 지원하지 않는 타입이다.
	opts := map[string]any{
		"register_map": map[string]any{
			"holding_registers": map[string]any{
				"start_address": float64(0),
				"count":         float64(10),
				"data_type":     "float64",
			},
		},
	}

	_, err := parseModbusServerConfig(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data_type")
	assert.Contains(t, err.Error(), "float64")
}

func TestParseModbusServerConfig_BackwardCompatibility(t *testing.T) {
	// data_type, type_map 없이 기존 설정이 정상 동작하는지 확인한다.
	opts := map[string]any{
		"register_map": map[string]any{
			"holding_registers": map[string]any{
				"start_address":  float64(0),
				"count":          float64(10),
				"initial_values": []any{float64(100), float64(200)},
			},
		},
	}

	cfg, err := parseModbusServerConfig(opts)
	require.NoError(t, err)

	require.Len(t, cfg.RegisterMap.HoldingRegisters, 1)
	assert.Equal(t, uint16(0), cfg.RegisterMap.HoldingRegisters[0].StartAddress)
	assert.Equal(t, uint16(10), cfg.RegisterMap.HoldingRegisters[0].Count)
	assert.Equal(t, "", cfg.RegisterMap.HoldingRegisters[0].DataType)
	assert.Nil(t, cfg.RegisterMap.HoldingRegisters[0].TypeMap)
	assert.Equal(t, []any{float64(100), float64(200)}, cfg.RegisterMap.HoldingRegisters[0].InitialValues)
}

func TestParseModbusServerConfig_TypeMapInvalidByteOrder(t *testing.T) {
	// type_map에 잘못된 byte_order가 주어지면 에러를 반환해야 한다.
	opts := map[string]any{
		"register_map": map[string]any{
			"holding_registers": map[string]any{
				"start_address": float64(0),
				"count":         float64(10),
				"type_map": []any{
					map[string]any{
						"address":    float64(0),
						"data_type":  "float32",
						"byte_order": "invalid_order",
					},
				},
			},
		},
	}

	_, err := parseModbusServerConfig(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "byte_order")
	assert.Contains(t, err.Error(), "invalid_order")
}

func TestParseModbusServerConfig_TypeMapValidByteOrders(t *testing.T) {
	// big_endian과 little_endian 모두 정상 동작해야 한다.
	for _, bo := range []string{"big_endian", "little_endian"} {
		t.Run(bo, func(t *testing.T) {
			opts := map[string]any{
				"register_map": map[string]any{
					"holding_registers": map[string]any{
						"start_address": float64(0),
						"count":         float64(10),
						"type_map": []any{
							map[string]any{
								"address":    float64(0),
								"data_type":  "float32",
								"byte_order": bo,
							},
						},
					},
				},
			}

			cfg, err := parseModbusServerConfig(opts)
			require.NoError(t, err)
			require.Len(t, cfg.RegisterMap.HoldingRegisters, 1)
			require.Len(t, cfg.RegisterMap.HoldingRegisters[0].TypeMap, 1)
			assert.Equal(t, bo, cfg.RegisterMap.HoldingRegisters[0].TypeMap[0].ByteOrder)
		})
	}
}

// ---------------------------------------------------------------------------
// 다중 세그먼트 파싱 테스트
// ---------------------------------------------------------------------------

func TestParseModbusServerConfig_MultiSegment(t *testing.T) {
	// 배열 형식으로 다중 세그먼트를 파싱한다.
	opts := map[string]any{
		"register_map": map[string]any{
			"input_registers": []any{
				map[string]any{
					"start_address": float64(0),
					"count":         float64(100),
				},
				map[string]any{
					"start_address": float64(200),
					"count":         float64(50),
				},
			},
		},
	}

	cfg, err := parseModbusServerConfig(opts)
	require.NoError(t, err)
	require.Len(t, cfg.RegisterMap.InputRegisters, 2)
	assert.Equal(t, uint16(0), cfg.RegisterMap.InputRegisters[0].StartAddress)
	assert.Equal(t, uint16(100), cfg.RegisterMap.InputRegisters[0].Count)
	assert.Equal(t, uint16(200), cfg.RegisterMap.InputRegisters[1].StartAddress)
	assert.Equal(t, uint16(50), cfg.RegisterMap.InputRegisters[1].Count)
}

func TestParseModbusServerConfig_MultiSegment_BackwardCompat(t *testing.T) {
	// 기존 단일 맵 형식이 여전히 동작하는지 확인한다 (하위 호환).
	opts := map[string]any{
		"register_map": map[string]any{
			"holding_registers": map[string]any{
				"start_address": float64(0),
				"count":         float64(10),
			},
		},
	}

	cfg, err := parseModbusServerConfig(opts)
	require.NoError(t, err)
	require.Len(t, cfg.RegisterMap.HoldingRegisters, 1)
	assert.Equal(t, uint16(0), cfg.RegisterMap.HoldingRegisters[0].StartAddress)
	assert.Equal(t, uint16(10), cfg.RegisterMap.HoldingRegisters[0].Count)
}

func TestParseModbusServerConfig_MultiSegment_Overlap(t *testing.T) {
	// 세그먼트 간 겹침이 있으면 에러를 반환한다.
	opts := map[string]any{
		"register_map": map[string]any{
			"holding_registers": []any{
				map[string]any{
					"start_address": float64(0),
					"count":         float64(100),
				},
				map[string]any{
					"start_address": float64(50),
					"count":         float64(100),
				},
			},
		},
	}

	_, err := parseModbusServerConfig(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overlap")
}

func TestParseModbusServerConfig_MultiSegment_EmptyArray(t *testing.T) {
	// 빈 배열이면 에러를 반환한다.
	opts := map[string]any{
		"register_map": map[string]any{
			"coils": []any{},
		},
	}

	_, err := parseModbusServerConfig(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one segment")
}

func TestParseModbusServerConfig_MultiSegment_AllAreas(t *testing.T) {
	// 4개 영역 모두 다중 세그먼트를 사용할 수 있다.
	opts := map[string]any{
		"register_map": map[string]any{
			"coils": []any{
				map[string]any{"start_address": float64(0), "count": float64(10)},
				map[string]any{"start_address": float64(100), "count": float64(10)},
			},
			"discrete_inputs": []any{
				map[string]any{"start_address": float64(0), "count": float64(20)},
			},
			"holding_registers": []any{
				map[string]any{"start_address": float64(0), "count": float64(50)},
				map[string]any{"start_address": float64(200), "count": float64(50)},
				map[string]any{"start_address": float64(400), "count": float64(50)},
			},
			"input_registers": map[string]any{
				"start_address": float64(0),
				"count":         float64(100),
			},
		},
	}

	cfg, err := parseModbusServerConfig(opts)
	require.NoError(t, err)
	assert.Len(t, cfg.RegisterMap.Coils, 2)
	assert.Len(t, cfg.RegisterMap.DiscreteInputs, 1)
	assert.Len(t, cfg.RegisterMap.HoldingRegisters, 3)
	assert.Len(t, cfg.RegisterMap.InputRegisters, 1)
}

func TestParseModbusServerConfig_MultiSegment_NoOverlap(t *testing.T) {
	// 인접한 세그먼트는 겹침이 아니다 (경계 테스트).
	opts := map[string]any{
		"register_map": map[string]any{
			"holding_registers": []any{
				map[string]any{
					"start_address": float64(0),
					"count":         float64(100),
				},
				map[string]any{
					"start_address": float64(100),
					"count":         float64(100),
				},
			},
		},
	}

	cfg, err := parseModbusServerConfig(opts)
	require.NoError(t, err)
	require.Len(t, cfg.RegisterMap.HoldingRegisters, 2)
}

// ---------------------------------------------------------------------------
// 멀티-디바이스 설정 파싱 테스트
// ---------------------------------------------------------------------------

func TestParseModbusServerConfig_MultiDevice(t *testing.T) {
	// devices 배열로 다중 디바이스를 설정한다.
	opts := map[string]any{
		"listen_port": float64(5020),
		"devices": []any{
			map[string]any{
				"unit_id": float64(1),
				"name":    "device-1",
				"register_map": map[string]any{
					"holding_registers": map[string]any{
						"start_address": float64(0),
						"count":         float64(10),
					},
				},
			},
			map[string]any{
				"unit_id": float64(2),
				"name":    "device-2",
				"register_map": map[string]any{
					"coils": map[string]any{
						"start_address": float64(0),
						"count":         float64(100),
					},
				},
			},
		},
	}

	cfg, err := parseModbusServerConfig(opts)
	require.NoError(t, err)

	require.Len(t, cfg.Devices, 2)

	// 첫 번째 디바이스 확인
	assert.Equal(t, byte(1), cfg.Devices[0].UnitID)
	assert.Equal(t, "device-1", cfg.Devices[0].Name)
	require.Len(t, cfg.Devices[0].RegisterMap.HoldingRegisters, 1)
	assert.Equal(t, uint16(10), cfg.Devices[0].RegisterMap.HoldingRegisters[0].Count)

	// 두 번째 디바이스 확인
	assert.Equal(t, byte(2), cfg.Devices[1].UnitID)
	assert.Equal(t, "device-2", cfg.Devices[1].Name)
	require.Len(t, cfg.Devices[1].RegisterMap.Coils, 1)
	assert.Equal(t, uint16(100), cfg.Devices[1].RegisterMap.Coils[0].Count)
}

func TestParseModbusServerConfig_MultiDevice_BackwardCompat(t *testing.T) {
	// 기존 단일 unit_id + register_map 설정이 Devices 에 자동 변환되는지 확인한다.
	opts := map[string]any{
		"unit_id": float64(5),
		"register_map": map[string]any{
			"holding_registers": map[string]any{
				"start_address": float64(0),
				"count":         float64(10),
			},
		},
	}

	cfg, err := parseModbusServerConfig(opts)
	require.NoError(t, err)

	// Devices 에 자동 변환됨
	require.Len(t, cfg.Devices, 1)
	assert.Equal(t, byte(5), cfg.Devices[0].UnitID)
	assert.Equal(t, "", cfg.Devices[0].Name)
	require.Len(t, cfg.Devices[0].RegisterMap.HoldingRegisters, 1)
	assert.Equal(t, uint16(10), cfg.Devices[0].RegisterMap.HoldingRegisters[0].Count)

	// 하위 호환 필드도 유지
	assert.Equal(t, byte(5), cfg.UnitID)
	require.Len(t, cfg.RegisterMap.HoldingRegisters, 1)
}

func TestParseModbusServerConfig_MultiDevice_DuplicateUnitID(t *testing.T) {
	// 동일한 unit_id 를 가진 디바이스가 있으면 에러를 반환한다.
	opts := map[string]any{
		"devices": []any{
			map[string]any{
				"unit_id": float64(1),
				"register_map": map[string]any{
					"holding_registers": map[string]any{
						"start_address": float64(0),
						"count":         float64(10),
					},
				},
			},
			map[string]any{
				"unit_id": float64(1),
				"register_map": map[string]any{
					"coils": map[string]any{
						"start_address": float64(0),
						"count":         float64(10),
					},
				},
			},
		},
	}

	_, err := parseModbusServerConfig(opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDuplicateUnitID)
}

func TestParseModbusServerConfig_MultiDevice_UnitIDRange(t *testing.T) {
	// devices 의 unit_id 는 1-247 범위여야 한다.
	tests := []struct {
		name   string
		unitID float64
	}{
		{"unit_id 0", 0},
		{"unit_id 248", 248},
		{"unit_id 255", 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := map[string]any{
				"devices": []any{
					map[string]any{
						"unit_id": tt.unitID,
						"register_map": map[string]any{
							"holding_registers": map[string]any{
								"start_address": float64(0),
								"count":         float64(10),
							},
						},
					},
				},
			}

			_, err := parseModbusServerConfig(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unit_id")
		})
	}
}

func TestParseModbusServerConfig_MultiDevice_MissingUnitID(t *testing.T) {
	// devices 항목에 unit_id 가 없으면 에러를 반환한다.
	opts := map[string]any{
		"devices": []any{
			map[string]any{
				"name": "no-unit-id",
				"register_map": map[string]any{
					"holding_registers": map[string]any{
						"start_address": float64(0),
						"count":         float64(10),
					},
				},
			},
		},
	}

	_, err := parseModbusServerConfig(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unit_id")
}

func TestParseModbusServerConfig_MultiDevice_MissingRegisterMap(t *testing.T) {
	// devices 항목에 register_map 이 없으면 에러를 반환한다.
	opts := map[string]any{
		"devices": []any{
			map[string]any{
				"unit_id": float64(1),
			},
		},
	}

	_, err := parseModbusServerConfig(opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRegisterMap)
}

func TestParseModbusServerConfig_MultiDevice_EmptyArray(t *testing.T) {
	// devices 가 빈 배열이면 에러를 반환한다.
	opts := map[string]any{
		"devices": []any{},
	}

	_, err := parseModbusServerConfig(opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidDeviceConfig)
}

func TestParseModbusServerConfig_MultiDevice_NotArray(t *testing.T) {
	// devices 가 배열이 아니면 에러를 반환한다.
	opts := map[string]any{
		"devices": "invalid",
	}

	_, err := parseModbusServerConfig(opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidDeviceConfig)
}

func TestParseModbusServerConfig_MultiDevice_NoNameOptional(t *testing.T) {
	// name 은 선택 필드이며, 없어도 정상 동작한다.
	opts := map[string]any{
		"devices": []any{
			map[string]any{
				"unit_id": float64(1),
				"register_map": map[string]any{
					"holding_registers": map[string]any{
						"start_address": float64(0),
						"count":         float64(10),
					},
				},
			},
		},
	}

	cfg, err := parseModbusServerConfig(opts)
	require.NoError(t, err)
	require.Len(t, cfg.Devices, 1)
	assert.Equal(t, byte(1), cfg.Devices[0].UnitID)
	assert.Equal(t, "", cfg.Devices[0].Name)
}

func TestParseModbusServerConfig_NoRegisterMapNoDevices(t *testing.T) {
	// register_map 도 devices 도 없으면 zero-device main 으로 구성된다(오류 아님).
	opts := map[string]any{
		"listen_port": float64(5020),
	}

	cfg, err := parseModbusServerConfig(opts)
	require.NoError(t, err)
	assert.Equal(t, RoleMain, cfg.Role)
	assert.Empty(t, cfg.Devices)
}
