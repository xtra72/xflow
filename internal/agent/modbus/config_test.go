package modbus

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 헬퍼: 최소 유효 설정 맵
// ---------------------------------------------------------------------------

// minimalValidOpts 는 파싱에 성공하는 최소 설정 맵을 반환한다.
func minimalValidOpts() map[string]any {
	return map[string]any{
		"devices": []any{
			map[string]any{
				"host": "192.168.1.100",
				"register_groups": []any{
					map[string]any{
						"function_code": 3,
						"quantity":      10,
					},
				},
			},
		},
	}
}

// ===========================================================================
// 기본값 테스트
// ===========================================================================

// TestParseModbusConfig_Defaults 는 빈 옵션으로 기본값이 올바르게 적용되는지 검증한다.
func TestParseModbusConfig_Defaults(t *testing.T) {
	opts := minimalValidOpts()

	cfg, err := parseModbusConfig(opts)
	require.NoError(t, err)

	tests := []struct {
		name string
		got  any
		want any
	}{
		{name: "Mode", got: cfg.Mode, want: "interval"},
		{name: "ReadMode", got: cfg.ReadMode, want: "cached"},
		{name: "PollInterval", got: cfg.PollInterval, want: 5 * time.Second},
		{name: "HeartbeatInterval", got: cfg.HeartbeatInterval, want: 60 * time.Second},
		{name: "WriteTimeout", got: cfg.WriteTimeout, want: 5 * time.Second},
		{name: "EnableWriteEvents", got: cfg.EnableWriteEvents, want: true},
		{name: "ReconnectInterval", got: cfg.ReconnectInterval, want: 10 * time.Second},
		{name: "MaxRetries", got: cfg.MaxRetries, want: 3},
		{name: "RequestTimeout", got: cfg.RequestTimeout, want: 3 * time.Second},
		{name: "MsgChannelSize", got: cfg.MsgChannelSize, want: 256},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.got)
		})
	}
}

// ===========================================================================
// 전체 설정 테스트
// ===========================================================================

// TestParseModbusConfig_Full 는 모든 필드가 지정된 설정을 올바르게 파싱하는지 검증한다.
func TestParseModbusConfig_Full(t *testing.T) {
	opts := map[string]any{
		"mode":               "event",
		"read_mode":          "cached",
		"poll_interval":      "10s",
		"heartbeat_interval": "30s",
		"stale_threshold":    "45s",
		"write_timeout":      "3s",
		"enable_write_events": false,
		"reconnect_interval": "15s",
		"max_retries":        5,
		"request_timeout":    "2s",
		"msg_channel_size":   512,
		"devices": []any{
			map[string]any{
				"id":      "plc-1",
				"host":    "10.0.0.1",
				"port":    503,
				"unit_id": 2,
				"register_groups": []any{
					map[string]any{
						"name":          "holding_0-9",
						"function_code": 3,
						"start_address": 0,
						"quantity":      10,
					},
					map[string]any{
						"name":          "coils_0-15",
						"function_code": 1,
						"start_address": 0,
						"quantity":      16,
					},
				},
			},
			map[string]any{
				"id":   "plc-2",
				"host": "10.0.0.2",
				"register_groups": []any{
					map[string]any{
						"function_code": 4,
						"start_address": 100,
						"quantity":      5,
					},
				},
			},
		},
	}

	cfg, err := parseModbusConfig(opts)
	require.NoError(t, err)

	assert.Equal(t, "event", cfg.Mode)
	assert.Equal(t, "cached", cfg.ReadMode)
	assert.Equal(t, 10*time.Second, cfg.PollInterval)
	assert.Equal(t, 30*time.Second, cfg.HeartbeatInterval)
	assert.Equal(t, 45*time.Second, cfg.StaleThreshold)
	assert.Equal(t, 3*time.Second, cfg.WriteTimeout)
	assert.Equal(t, false, cfg.EnableWriteEvents)
	assert.Equal(t, 15*time.Second, cfg.ReconnectInterval)
	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 2*time.Second, cfg.RequestTimeout)
	assert.Equal(t, 512, cfg.MsgChannelSize)

	// 디바이스 검증
	require.Len(t, cfg.Devices, 2)

	// 디바이스 1
	d1 := cfg.Devices[0]
	assert.Equal(t, "plc-1", d1.ID)
	assert.Equal(t, "10.0.0.1", d1.Host)
	assert.Equal(t, 503, d1.Port)
	assert.Equal(t, byte(2), d1.UnitID)
	require.Len(t, d1.RegisterGroups, 2)

	assert.Equal(t, "holding_0-9", d1.RegisterGroups[0].Name)
	assert.Equal(t, FC03ReadHoldingRegisters, d1.RegisterGroups[0].FunctionCode)
	assert.Equal(t, uint16(0), d1.RegisterGroups[0].StartAddress)
	assert.Equal(t, uint16(10), d1.RegisterGroups[0].Quantity)

	assert.Equal(t, "coils_0-15", d1.RegisterGroups[1].Name)
	assert.Equal(t, FC01ReadCoils, d1.RegisterGroups[1].FunctionCode)

	// 디바이스 2: 기본 port, unit_id
	d2 := cfg.Devices[1]
	assert.Equal(t, "plc-2", d2.ID)
	assert.Equal(t, "10.0.0.2", d2.Host)
	assert.Equal(t, 502, d2.Port, "기본 포트")
	assert.Equal(t, byte(1), d2.UnitID, "기본 Unit ID")
}

// ===========================================================================
// 에러 케이스 테스트
// ===========================================================================

// TestParseModbusConfig_NoDevices 는 devices 누락 시 에러를 반환하는지 검증한다.
func TestParseModbusConfig_NoDevices(t *testing.T) {
	opts := map[string]any{}

	_, err := parseModbusConfig(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "devices is required")
}

// TestParseModbusConfig_EmptyDevices 는 빈 devices 배열에 대해 에러를 반환하는지 검증한다.
func TestParseModbusConfig_EmptyDevices(t *testing.T) {
	opts := map[string]any{
		"devices": []any{},
	}

	_, err := parseModbusConfig(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "devices is required")
}

// TestParseModbusConfig_InvalidMode 는 잘못된 mode 값에 대해 에러를 반환하는지 검증한다.
func TestParseModbusConfig_InvalidMode(t *testing.T) {
	opts := minimalValidOpts()
	opts["mode"] = "invalid"

	_, err := parseModbusConfig(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid mode")
}

// TestParseModbusConfig_InvalidReadMode 는 잘못된 read_mode 값에 대해 에러를 반환하는지 검증한다.
func TestParseModbusConfig_InvalidReadMode(t *testing.T) {
	opts := minimalValidOpts()
	opts["read_mode"] = "unknown"

	_, err := parseModbusConfig(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid read_mode")
}

// TestParseModbusConfig_InvalidFunctionCode 는 잘못된 function_code 에 대해 에러를 반환하는지 검증한다.
func TestParseModbusConfig_InvalidFunctionCode(t *testing.T) {
	opts := map[string]any{
		"devices": []any{
			map[string]any{
				"host": "192.168.1.100",
				"register_groups": []any{
					map[string]any{
						"function_code": 99,
						"quantity":      10,
					},
				},
			},
		},
	}

	_, err := parseModbusConfig(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "function_code must be 1, 2, 3, or 4")
}

// TestParseModbusConfig_ZeroQuantity 는 quantity=0 에 대해 에러를 반환하는지 검증한다.
func TestParseModbusConfig_ZeroQuantity(t *testing.T) {
	opts := map[string]any{
		"devices": []any{
			map[string]any{
				"host": "192.168.1.100",
				"register_groups": []any{
					map[string]any{
						"function_code": 3,
						"quantity":      0,
					},
				},
			},
		},
	}

	_, err := parseModbusConfig(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quantity must be > 0")
}

// TestParseModbusConfig_StaleThresholdDefault 는 stale_threshold 미설정 시
// PollInterval * 3 으로 자동 설정되는지 검증한다.
func TestParseModbusConfig_StaleThresholdDefault(t *testing.T) {
	opts := minimalValidOpts()
	opts["poll_interval"] = "10s"

	cfg, err := parseModbusConfig(opts)
	require.NoError(t, err)

	assert.Equal(t, 30*time.Second, cfg.StaleThreshold,
		"StaleThreshold 는 PollInterval(10s) * 3 = 30s 여야 한다")
}

// TestParseModbusConfig_StaleThresholdExplicit 는 명시적으로 설정된 stale_threshold 가
// 기본값을 덮어쓰는지 검증한다.
func TestParseModbusConfig_StaleThresholdExplicit(t *testing.T) {
	opts := minimalValidOpts()
	opts["poll_interval"] = "10s"
	opts["stale_threshold"] = "60s"

	cfg, err := parseModbusConfig(opts)
	require.NoError(t, err)

	assert.Equal(t, 60*time.Second, cfg.StaleThreshold,
		"명시적 StaleThreshold(60s)가 기본값(30s)을 덮어써야 한다")
}

// TestParseModbusConfig_DeviceNoHost 는 디바이스에 host 가 없을 때 에러를 반환하는지 검증한다.
func TestParseModbusConfig_DeviceNoHost(t *testing.T) {
	opts := map[string]any{
		"devices": []any{
			map[string]any{
				"register_groups": []any{
					map[string]any{
						"function_code": 3,
						"quantity":      10,
					},
				},
			},
		},
	}

	_, err := parseModbusConfig(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "host is required")
}

// ===========================================================================
// YAML float64 호환 테스트
// ===========================================================================

// TestParseModbusConfig_NumericAsFloat64 는 YAML 파싱 호환을 위해
// 숫자 필드가 float64 로 전달될 때 올바르게 처리되는지 검증한다.
func TestParseModbusConfig_NumericAsFloat64(t *testing.T) {
	opts := map[string]any{
		"max_retries":      float64(7),
		"msg_channel_size": float64(1024),
		"devices": []any{
			map[string]any{
				"host":    "10.0.0.1",
				"port":    float64(503),
				"unit_id": float64(5),
				"register_groups": []any{
					map[string]any{
						"function_code": float64(4),
						"start_address": float64(100),
						"quantity":      float64(25),
					},
				},
			},
		},
	}

	cfg, err := parseModbusConfig(opts)
	require.NoError(t, err)

	assert.Equal(t, 7, cfg.MaxRetries)
	assert.Equal(t, 1024, cfg.MsgChannelSize)
	assert.Equal(t, 503, cfg.Devices[0].Port)
	assert.Equal(t, byte(5), cfg.Devices[0].UnitID)
	assert.Equal(t, byte(4), cfg.Devices[0].RegisterGroups[0].FunctionCode)
	assert.Equal(t, uint16(100), cfg.Devices[0].RegisterGroups[0].StartAddress)
	assert.Equal(t, uint16(25), cfg.Devices[0].RegisterGroups[0].Quantity)
}

// ===========================================================================
// Duration 파싱 에러 테스트
// ===========================================================================

// TestParseModbusConfig_InvalidDuration 는 잘못된 duration 값에 대해 에러를 반환하는지 검증한다.
func TestParseModbusConfig_InvalidDuration(t *testing.T) {
	durationFields := []string{
		"poll_interval",
		"heartbeat_interval",
		"stale_threshold",
		"write_timeout",
		"reconnect_interval",
		"request_timeout",
	}

	for _, field := range durationFields {
		t.Run(field, func(t *testing.T) {
			opts := minimalValidOpts()
			opts[field] = "not-a-duration"

			_, err := parseModbusConfig(opts)
			require.Error(t, err, "field %s should reject invalid duration", field)
			assert.Contains(t, err.Error(), field)
		})
	}
}

// ===========================================================================
// DataType / TypeMap 테스트
// ===========================================================================

// TestParseRegisterGroupConfig_DataType 는 data_type 필드 파싱을 검증한다.
func TestParseRegisterGroupConfig_DataType(t *testing.T) {
	t.Run("기본값 (data_type 미지정)", func(t *testing.T) {
		opts := minimalValidOpts()
		cfg, err := parseModbusConfig(opts)
		require.NoError(t, err)

		rg := cfg.Devices[0].RegisterGroups[0]
		assert.Empty(t, rg.DataType, "data_type 미지정 시 빈 문자열이어야 한다")
	})

	t.Run("유효한 data_type", func(t *testing.T) {
		validTypes := []string{"uint16", "int16", "float32", "uint32", "int32"}
		for _, dt := range validTypes {
			t.Run(dt, func(t *testing.T) {
				opts := map[string]any{
					"devices": []any{
						map[string]any{
							"host": "192.168.1.100",
							"register_groups": []any{
								map[string]any{
									"function_code": 3,
									"quantity":      10,
									"data_type":     dt,
								},
							},
						},
					},
				}
				cfg, err := parseModbusConfig(opts)
				require.NoError(t, err)
				assert.Equal(t, dt, cfg.Devices[0].RegisterGroups[0].DataType)
			})
		}
	})

	t.Run("지원하지 않는 data_type", func(t *testing.T) {
		opts := map[string]any{
			"devices": []any{
				map[string]any{
					"host": "192.168.1.100",
					"register_groups": []any{
						map[string]any{
							"function_code": 3,
							"quantity":      10,
							"data_type":     "float64",
						},
					},
				},
			},
		}
		_, err := parseModbusConfig(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "data_type is not supported")
		assert.ErrorIs(t, err, ErrUnsupportedDataType)
	})
}

// TestParseRegisterGroupConfig_TypeMap 은 type_map 필드 파싱을 검증한다.
func TestParseRegisterGroupConfig_TypeMap(t *testing.T) {
	t.Run("유효한 type_map", func(t *testing.T) {
		opts := map[string]any{
			"devices": []any{
				map[string]any{
					"host": "192.168.1.100",
					"register_groups": []any{
						map[string]any{
							"function_code": 3,
							"start_address": float64(0),
							"quantity":      float64(20),
							"type_map": []any{
								map[string]any{
									"address":   float64(0),
									"data_type": "float32",
								},
								map[string]any{
									"address":    float64(2),
									"data_type":  "int32",
									"byte_order": "little_endian",
								},
								map[string]any{
									"address":   float64(10),
									"data_type": "int16",
								},
							},
						},
					},
				},
			},
		}
		cfg, err := parseModbusConfig(opts)
		require.NoError(t, err)

		rg := cfg.Devices[0].RegisterGroups[0]
		require.Len(t, rg.TypeMap, 3)

		// 첫 번째 엔트리: float32, 기본 byte_order
		assert.Equal(t, uint16(0), rg.TypeMap[0].Address)
		assert.Equal(t, "float32", rg.TypeMap[0].DataType)
		assert.Equal(t, "big_endian", rg.TypeMap[0].ByteOrder)

		// 두 번째 엔트리: int32, little_endian
		assert.Equal(t, uint16(2), rg.TypeMap[1].Address)
		assert.Equal(t, "int32", rg.TypeMap[1].DataType)
		assert.Equal(t, "little_endian", rg.TypeMap[1].ByteOrder)

		// 세 번째 엔트리: int16
		assert.Equal(t, uint16(10), rg.TypeMap[2].Address)
		assert.Equal(t, "int16", rg.TypeMap[2].DataType)
		assert.Equal(t, "big_endian", rg.TypeMap[2].ByteOrder)
	})

	t.Run("type_map address 누락", func(t *testing.T) {
		opts := map[string]any{
			"devices": []any{
				map[string]any{
					"host": "192.168.1.100",
					"register_groups": []any{
						map[string]any{
							"function_code": 3,
							"quantity":      float64(10),
							"type_map": []any{
								map[string]any{
									"data_type": "float32",
								},
							},
						},
					},
				},
			},
		}
		_, err := parseModbusConfig(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "address is required")
	})

	t.Run("type_map data_type 누락", func(t *testing.T) {
		opts := map[string]any{
			"devices": []any{
				map[string]any{
					"host": "192.168.1.100",
					"register_groups": []any{
						map[string]any{
							"function_code": 3,
							"quantity":      float64(10),
							"type_map": []any{
								map[string]any{
									"address": float64(0),
								},
							},
						},
					},
				},
			},
		}
		_, err := parseModbusConfig(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "data_type is required")
	})

	t.Run("type_map 지원하지 않는 data_type", func(t *testing.T) {
		opts := map[string]any{
			"devices": []any{
				map[string]any{
					"host": "192.168.1.100",
					"register_groups": []any{
						map[string]any{
							"function_code": 3,
							"quantity":      float64(10),
							"type_map": []any{
								map[string]any{
									"address":   float64(0),
									"data_type": "float64",
								},
							},
						},
					},
				},
			},
		}
		_, err := parseModbusConfig(opts)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUnsupportedDataType)
	})
}

// TestParseRegisterGroupConfig_TypeMapValidation 은 type_map 범위 및 겹침 검증을 테스트한다.
func TestParseRegisterGroupConfig_TypeMapValidation(t *testing.T) {
	t.Run("범위 초과 (ErrTypeMapOutOfRange)", func(t *testing.T) {
		opts := map[string]any{
			"devices": []any{
				map[string]any{
					"host": "192.168.1.100",
					"register_groups": []any{
						map[string]any{
							"function_code": 3,
							"start_address": float64(0),
							"quantity":      float64(5),
							"type_map": []any{
								map[string]any{
									"address":   float64(4),
									"data_type": "float32", // 주소 4-5, count=5 이므로 범위 초과
								},
							},
						},
					},
				},
			},
		}
		_, err := parseModbusConfig(opts)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTypeMapOutOfRange)
	})

	t.Run("주소 겹침 (ErrTypeMapOverlap)", func(t *testing.T) {
		opts := map[string]any{
			"devices": []any{
				map[string]any{
					"host": "192.168.1.100",
					"register_groups": []any{
						map[string]any{
							"function_code": 3,
							"start_address": float64(0),
							"quantity":      float64(10),
							"type_map": []any{
								map[string]any{
									"address":   float64(0),
									"data_type": "float32", // 주소 0-1
								},
								map[string]any{
									"address":   float64(1),
									"data_type": "uint16", // 주소 1: 겹침
								},
							},
						},
					},
				},
			},
		}
		_, err := parseModbusConfig(opts)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTypeMapOverlap)
	})

	t.Run("유효한 경계 (정확히 맞는 범위)", func(t *testing.T) {
		opts := map[string]any{
			"devices": []any{
				map[string]any{
					"host": "192.168.1.100",
					"register_groups": []any{
						map[string]any{
							"function_code": 3,
							"start_address": float64(0),
							"quantity":      float64(4),
							"type_map": []any{
								map[string]any{
									"address":   float64(0),
									"data_type": "float32", // 주소 0-1
								},
								map[string]any{
									"address":   float64(2),
									"data_type": "float32", // 주소 2-3
								},
							},
						},
					},
				},
			},
		}
		cfg, err := parseModbusConfig(opts)
		require.NoError(t, err)
		assert.Len(t, cfg.Devices[0].RegisterGroups[0].TypeMap, 2)
	})

	t.Run("비영점 시작 주소", func(t *testing.T) {
		opts := map[string]any{
			"devices": []any{
				map[string]any{
					"host": "192.168.1.100",
					"register_groups": []any{
						map[string]any{
							"function_code": 3,
							"start_address": float64(100),
							"quantity":      float64(10),
							"type_map": []any{
								map[string]any{
									"address":   float64(100),
									"data_type": "float32", // 주소 100-101
								},
								map[string]any{
									"address":   float64(108),
									"data_type": "int32", // 주소 108-109
								},
							},
						},
					},
				},
			},
		}
		cfg, err := parseModbusConfig(opts)
		require.NoError(t, err)
		assert.Len(t, cfg.Devices[0].RegisterGroups[0].TypeMap, 2)
	})

	t.Run("시작 주소 이전 (범위 초과)", func(t *testing.T) {
		opts := map[string]any{
			"devices": []any{
				map[string]any{
					"host": "192.168.1.100",
					"register_groups": []any{
						map[string]any{
							"function_code": 3,
							"start_address": float64(100),
							"quantity":      float64(10),
							"type_map": []any{
								map[string]any{
									"address":   float64(99), // 시작 주소 이전
									"data_type": "uint16",
								},
							},
						},
					},
				},
			},
		}
		_, err := parseModbusConfig(opts)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTypeMapOutOfRange)
	})
}

// TestParseRegisterGroupConfig_BackwardCompat 는 DataType/TypeMap 없이 기존 동작이 유지되는지 검증한다.
func TestParseRegisterGroupConfig_BackwardCompat(t *testing.T) {
	opts := minimalValidOpts()
	cfg, err := parseModbusConfig(opts)
	require.NoError(t, err)

	rg := cfg.Devices[0].RegisterGroups[0]
	assert.Empty(t, rg.DataType, "기본값: DataType 은 빈 문자열")
	assert.Nil(t, rg.TypeMap, "기본값: TypeMap 은 nil")
}
