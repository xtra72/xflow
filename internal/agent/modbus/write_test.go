package modbus

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 쓰기 응답 프레임 빌더 (테스트 헬퍼)
// ---------------------------------------------------------------------------

// buildFC05Response 는 FC05 단일 코일 쓰기 에코백 응답 프레임을 생성한다.
// MODBUS 명세에 따라 요청과 동일한 주소 + 값을 에코백한다.
func buildFC05Response(txID uint16, unitID byte, addr uint16, value bool) []byte {
	frame := make([]byte, 12)
	binary.BigEndian.PutUint16(frame[0:2], txID)
	binary.BigEndian.PutUint16(frame[2:4], MBAPProtocolID)
	binary.BigEndian.PutUint16(frame[4:6], 6) // Length: UnitID(1) + FC(1) + Addr(2) + Value(2)
	frame[6] = unitID
	frame[7] = FC05WriteSingleCoil
	binary.BigEndian.PutUint16(frame[8:10], addr)
	if value {
		frame[10] = 0xFF
		frame[11] = 0x00
	}
	return frame
}

// buildFC06Response 는 FC06 단일 레지스터 쓰기 에코백 응답 프레임을 생성한다.
func buildFC06Response(txID uint16, unitID byte, addr uint16, value uint16) []byte {
	frame := make([]byte, 12)
	binary.BigEndian.PutUint16(frame[0:2], txID)
	binary.BigEndian.PutUint16(frame[2:4], MBAPProtocolID)
	binary.BigEndian.PutUint16(frame[4:6], 6)
	frame[6] = unitID
	frame[7] = FC06WriteSingleRegister
	binary.BigEndian.PutUint16(frame[8:10], addr)
	binary.BigEndian.PutUint16(frame[10:12], value)
	return frame
}

// buildFC15Response 는 FC15 다중 코일 쓰기 확인 응답 프레임을 생성한다.
// 응답에는 시작 주소와 수량이 포함된다.
func buildFC15Response(txID uint16, unitID byte, addr uint16, quantity uint16) []byte {
	frame := make([]byte, 12)
	binary.BigEndian.PutUint16(frame[0:2], txID)
	binary.BigEndian.PutUint16(frame[2:4], MBAPProtocolID)
	binary.BigEndian.PutUint16(frame[4:6], 6)
	frame[6] = unitID
	frame[7] = FC15WriteMultipleCoils
	binary.BigEndian.PutUint16(frame[8:10], addr)
	binary.BigEndian.PutUint16(frame[10:12], quantity)
	return frame
}

// buildFC16Response 는 FC16 다중 레지스터 쓰기 확인 응답 프레임을 생성한다.
func buildFC16Response(txID uint16, unitID byte, addr uint16, quantity uint16) []byte {
	frame := make([]byte, 12)
	binary.BigEndian.PutUint16(frame[0:2], txID)
	binary.BigEndian.PutUint16(frame[2:4], MBAPProtocolID)
	binary.BigEndian.PutUint16(frame[4:6], 6)
	frame[6] = unitID
	frame[7] = FC16WriteMultipleRegisters
	binary.BigEndian.PutUint16(frame[8:10], addr)
	binary.BigEndian.PutUint16(frame[10:12], quantity)
	return frame
}

// buildExceptionResponse 는 MODBUS 예외 응답 프레임을 생성한다.
// fc 는 원래 기능 코드, exceptionCode 는 예외 코드이다.
func buildExceptionResponse(txID uint16, unitID byte, fc byte, exceptionCode byte) []byte {
	frame := make([]byte, 9)
	binary.BigEndian.PutUint16(frame[0:2], txID)
	binary.BigEndian.PutUint16(frame[2:4], MBAPProtocolID)
	binary.BigEndian.PutUint16(frame[4:6], 3) // Length: UnitID(1) + FC(1) + ExCode(1)
	frame[6] = unitID
	frame[7] = fc | 0x80 // 예외 응답: 기능 코드에 0x80 비트 설정
	frame[8] = exceptionCode
	return frame
}

// ---------------------------------------------------------------------------
// FC05: 단일 코일 쓰기 테스트
// ---------------------------------------------------------------------------

func TestProcessWriteCoil_Success(t *testing.T) {
	response := buildFC05Response(0, 1, 100, true)
	mt := &mockModbusTransport{connected: true, response: response}
	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	// write_coil 명령
	data, _ := json.Marshal(map[string]any{
		"command":   "write_coil",
		"device_id": "plc-1",
		"params": map[string]any{
			"address": 100,
			"value":   true,
		},
	})
	result, err := a.Process(data)
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(result, &resp))

	assert.Equal(t, "ok", resp["status"])
	assert.Equal(t, "plc-1", resp["device_id"])
	assert.Equal(t, "write_coil", resp["command"])
	assert.Equal(t, float64(100), resp["address"])
	assert.Equal(t, float64(1), resp["quantity"])
	assert.NotEmpty(t, resp["timestamp"])

	// 캐시가 갱신되었는지 확인
	cache := a.caches["plc-1"]
	cache.mu.RLock()
	coilVal, ok := cache.Coils[100]
	cache.mu.RUnlock()
	assert.True(t, ok, "코일 100 이 캐시에 존재해야 한다")
	assert.True(t, coilVal, "코일 100 이 true 여야 한다")

	// 통계 확인
	stats := a.Stats()
	assert.Equal(t, int64(1), stats.MessagesSent, "MessagesSent 가 1 이어야 한다")
}

// ---------------------------------------------------------------------------
// FC06: 단일 레지스터 쓰기 테스트
// ---------------------------------------------------------------------------

func TestProcessWriteRegister_Success(t *testing.T) {
	response := buildFC06Response(0, 1, 200, 12345)
	mt := &mockModbusTransport{connected: true, response: response}
	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	data, _ := json.Marshal(map[string]any{
		"command":   "write_register",
		"device_id": "plc-1",
		"params": map[string]any{
			"address": 200,
			"value":   12345,
		},
	})
	result, err := a.Process(data)
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(result, &resp))

	assert.Equal(t, "ok", resp["status"])
	assert.Equal(t, "plc-1", resp["device_id"])
	assert.Equal(t, "write_register", resp["command"])
	assert.Equal(t, float64(200), resp["address"])
	assert.Equal(t, float64(1), resp["quantity"])

	// 캐시 확인
	cache := a.caches["plc-1"]
	cache.mu.RLock()
	regVal, ok := cache.HoldingRegisters[200]
	cache.mu.RUnlock()
	assert.True(t, ok, "레지스터 200 이 캐시에 존재해야 한다")
	assert.Equal(t, uint16(12345), regVal)

	stats := a.Stats()
	assert.Equal(t, int64(1), stats.MessagesSent)
}

// ---------------------------------------------------------------------------
// FC15: 다중 코일 쓰기 테스트
// ---------------------------------------------------------------------------

func TestProcessWriteCoils_Success(t *testing.T) {
	values := []bool{true, false, true, true, false}
	response := buildFC15Response(0, 1, 50, uint16(len(values)))
	mt := &mockModbusTransport{connected: true, response: response}
	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	data, _ := json.Marshal(map[string]any{
		"command":   "write_coils",
		"device_id": "plc-1",
		"params": map[string]any{
			"address": 50,
			"values":  values,
		},
	})
	result, err := a.Process(data)
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(result, &resp))

	assert.Equal(t, "ok", resp["status"])
	assert.Equal(t, "write_coils", resp["command"])
	assert.Equal(t, float64(50), resp["address"])
	assert.Equal(t, float64(5), resp["quantity"])

	// 캐시 확인
	cache := a.caches["plc-1"]
	cache.mu.RLock()
	assert.Equal(t, true, cache.Coils[50])
	assert.Equal(t, false, cache.Coils[51])
	assert.Equal(t, true, cache.Coils[52])
	assert.Equal(t, true, cache.Coils[53])
	assert.Equal(t, false, cache.Coils[54])
	cache.mu.RUnlock()

	stats := a.Stats()
	assert.Equal(t, int64(1), stats.MessagesSent)
}

// ---------------------------------------------------------------------------
// FC16: 다중 레지스터 쓰기 테스트
// ---------------------------------------------------------------------------

func TestProcessWriteRegisters_Success(t *testing.T) {
	values := []any{float64(1000), float64(2000), float64(3000)}
	response := buildFC16Response(0, 1, 300, 3)
	mt := &mockModbusTransport{connected: true, response: response}
	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	data, _ := json.Marshal(map[string]any{
		"command":   "write_registers",
		"device_id": "plc-1",
		"params": map[string]any{
			"address": 300,
			"values":  values,
		},
	})
	result, err := a.Process(data)
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(result, &resp))

	assert.Equal(t, "ok", resp["status"])
	assert.Equal(t, "write_registers", resp["command"])
	assert.Equal(t, float64(300), resp["address"])
	assert.Equal(t, float64(3), resp["quantity"])

	// 캐시 확인
	cache := a.caches["plc-1"]
	cache.mu.RLock()
	assert.Equal(t, uint16(1000), cache.HoldingRegisters[300])
	assert.Equal(t, uint16(2000), cache.HoldingRegisters[301])
	assert.Equal(t, uint16(3000), cache.HoldingRegisters[302])
	cache.mu.RUnlock()

	stats := a.Stats()
	assert.Equal(t, int64(1), stats.MessagesSent)
}

// ---------------------------------------------------------------------------
// 디바이스 오프라인 테스트
// ---------------------------------------------------------------------------

func TestProcessWrite_DeviceOffline(t *testing.T) {
	mt := &mockModbusTransport{connected: false}
	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

	// 디바이스를 명시적으로 오프라인 설정
	a.devices[0].mu.Lock()
	a.devices[0].online = false
	a.devices[0].mu.Unlock()

	tests := []struct {
		name    string
		command string
		params  map[string]any
	}{
		{
			name:    "write_coil",
			command: "write_coil",
			params:  map[string]any{"address": 100, "value": true},
		},
		{
			name:    "write_register",
			command: "write_register",
			params:  map[string]any{"address": 200, "value": 1234},
		},
		{
			name:    "write_coils",
			command: "write_coils",
			params:  map[string]any{"address": 50, "values": []bool{true, false}},
		},
		{
			name:    "write_registers",
			command: "write_registers",
			params:  map[string]any{"address": 300, "values": []any{float64(1000)}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := json.Marshal(map[string]any{
				"command":   tc.command,
				"device_id": "plc-1",
				"params":    tc.params,
			})
			_, err := a.Process(data)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrDeviceOffline),
				"오프라인 디바이스에서 ErrDeviceOffline 이 반환되어야 한다")
		})
	}
}

// ---------------------------------------------------------------------------
// 디바이스 미존재 테스트
// ---------------------------------------------------------------------------

func TestProcessWrite_DeviceNotFound(t *testing.T) {
	a, _ := newTestModbusAgent(t, minimalAgentConfig())

	data, _ := json.Marshal(map[string]any{
		"command":   "write_coil",
		"device_id": "non-existent",
		"params": map[string]any{
			"address": 100,
			"value":   true,
		},
	})
	_, err := a.Process(data)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDeviceNotFound),
		"존재하지 않는 디바이스에서 ErrDeviceNotFound 가 반환되어야 한다")
}

// ---------------------------------------------------------------------------
// 수량 초과 테스트
// ---------------------------------------------------------------------------

func TestProcessWrite_QuantityExceeded(t *testing.T) {
	mt := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	t.Run("FC15 수량 초과", func(t *testing.T) {
		// MaxCoilsWrite = 1968 초과
		bigValues := make([]bool, MaxCoilsWrite+1)
		data, _ := json.Marshal(map[string]any{
			"command":   "write_coils",
			"device_id": "plc-1",
			"params": map[string]any{
				"address": 0,
				"values":  bigValues,
			},
		})
		_, err := a.Process(data)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrQuantityExceeded),
			"FC15 최대 수량 초과 시 ErrQuantityExceeded 가 반환되어야 한다")
	})

	t.Run("FC16 수량 초과", func(t *testing.T) {
		// MaxRegistersWrite = 123 초과
		bigValues := make([]any, MaxRegistersWrite+1)
		for i := range bigValues {
			bigValues[i] = float64(i)
		}
		data, _ := json.Marshal(map[string]any{
			"command":   "write_registers",
			"device_id": "plc-1",
			"params": map[string]any{
				"address": 0,
				"values":  bigValues,
			},
		})
		_, err := a.Process(data)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrQuantityExceeded),
			"FC16 최대 수량 초과 시 ErrQuantityExceeded 가 반환되어야 한다")
	})

	t.Run("FC15 빈 배열", func(t *testing.T) {
		data, _ := json.Marshal(map[string]any{
			"command":   "write_coils",
			"device_id": "plc-1",
			"params": map[string]any{
				"address": 0,
				"values":  []bool{},
			},
		})
		_, err := a.Process(data)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrQuantityExceeded),
			"FC15 빈 배열 시 ErrQuantityExceeded 가 반환되어야 한다")
	})

	t.Run("FC16 빈 배열", func(t *testing.T) {
		data, _ := json.Marshal(map[string]any{
			"command":   "write_registers",
			"device_id": "plc-1",
			"params": map[string]any{
				"address": 0,
				"values":  []any{},
			},
		})
		_, err := a.Process(data)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrQuantityExceeded),
			"FC16 빈 배열 시 ErrQuantityExceeded 가 반환되어야 한다")
	})
}

// ---------------------------------------------------------------------------
// MODBUS 예외 응답 테스트
// ---------------------------------------------------------------------------

func TestProcessWrite_ModbusException(t *testing.T) {
	tests := []struct {
		name          string
		command       string
		params        map[string]any
		fc            byte
		exceptionCode byte
	}{
		{
			name:          "FC05 예외",
			command:       "write_coil",
			params:        map[string]any{"address": 100, "value": true},
			fc:            FC05WriteSingleCoil,
			exceptionCode: ExceptionIllegalDataAddress,
		},
		{
			name:          "FC06 예외",
			command:       "write_register",
			params:        map[string]any{"address": 200, "value": 5000},
			fc:            FC06WriteSingleRegister,
			exceptionCode: ExceptionIllegalDataValue,
		},
		{
			name:          "FC15 예외",
			command:       "write_coils",
			params:        map[string]any{"address": 50, "values": []bool{true}},
			fc:            FC15WriteMultipleCoils,
			exceptionCode: ExceptionSlaveDeviceFailure,
		},
		{
			name:          "FC16 예외",
			command:       "write_registers",
			params:        map[string]any{"address": 300, "values": []any{float64(1000)}},
			fc:            FC16WriteMultipleRegisters,
			exceptionCode: ExceptionIllegalFunction,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// 예외 응답 프레임 생성
			excResp := buildExceptionResponse(0, 1, tc.fc, tc.exceptionCode)
			mt := &mockModbusTransport{connected: true, response: excResp}
			a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

			a.devices[0].mu.Lock()
			a.devices[0].online = true
			a.devices[0].mu.Unlock()

			data, _ := json.Marshal(map[string]any{
				"command":   tc.command,
				"device_id": "plc-1",
				"params":    tc.params,
			})

			// MODBUS 예외는 JSON 에러 응답을 반환해야 한다 (에러가 아닌 데이터)
			result, err := a.Process(data)
			require.NoError(t, err, "MODBUS 예외 시 Process 는 에러를 반환하지 않아야 한다")

			var resp map[string]any
			require.NoError(t, json.Unmarshal(result, &resp))

			assert.Equal(t, "error", resp["status"])
			assert.Equal(t, "plc-1", resp["device_id"])
			assert.Equal(t, tc.command, resp["command"])
			assert.Equal(t, float64(tc.exceptionCode), resp["exception_code"])
			assert.NotEmpty(t, resp["error"])
			assert.NotEmpty(t, resp["timestamp"])

			// 예외 시 MessagesErrored 증가
			stats := a.Stats()
			assert.Equal(t, int64(1), stats.MessagesErrored)
		})
	}
}

// ---------------------------------------------------------------------------
// Write-Through 캐시: 실패 시 캐시 미갱신 테스트
// ---------------------------------------------------------------------------

func TestProcessWrite_CacheNotUpdatedOnFailure(t *testing.T) {
	t.Run("네트워크 에러 시 코일 캐시 미갱신", func(t *testing.T) {
		mt := &mockModbusTransport{
			connected:   true,
			sendRecvErr: errors.New("network error"),
		}
		a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

		a.devices[0].mu.Lock()
		a.devices[0].online = true
		a.devices[0].mu.Unlock()

		// 캐시에 기존 값 설정
		cache := a.caches["plc-1"]
		cache.UpdateCoils(100, []bool{false})

		data, _ := json.Marshal(map[string]any{
			"command":   "write_coil",
			"device_id": "plc-1",
			"params": map[string]any{
				"address": 100,
				"value":   true,
			},
		})

		_, err := a.Process(data)
		require.Error(t, err, "네트워크 에러 시 에러가 반환되어야 한다")

		// 캐시가 변경되지 않았는지 확인
		cache.mu.RLock()
		coilVal := cache.Coils[100]
		cache.mu.RUnlock()
		assert.False(t, coilVal, "쓰기 실패 시 캐시가 갱신되지 않아야 한다 (여전히 false)")
	})

	t.Run("네트워크 에러 시 레지스터 캐시 미갱신", func(t *testing.T) {
		mt := &mockModbusTransport{
			connected:   true,
			sendRecvErr: errors.New("network error"),
		}
		a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

		a.devices[0].mu.Lock()
		a.devices[0].online = true
		a.devices[0].mu.Unlock()

		// 캐시에 기존 값 설정
		cache := a.caches["plc-1"]
		cache.UpdateHoldingRegisters(200, []uint16{9999})

		data, _ := json.Marshal(map[string]any{
			"command":   "write_register",
			"device_id": "plc-1",
			"params": map[string]any{
				"address": 200,
				"value":   12345,
			},
		})

		_, err := a.Process(data)
		require.Error(t, err)

		cache.mu.RLock()
		regVal := cache.HoldingRegisters[200]
		cache.mu.RUnlock()
		assert.Equal(t, uint16(9999), regVal,
			"쓰기 실패 시 캐시가 갱신되지 않아야 한다 (여전히 9999)")
	})

	t.Run("MODBUS 예외 시 캐시 미갱신", func(t *testing.T) {
		excResp := buildExceptionResponse(0, 1, FC06WriteSingleRegister, ExceptionIllegalDataAddress)
		mt := &mockModbusTransport{connected: true, response: excResp}
		a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

		a.devices[0].mu.Lock()
		a.devices[0].online = true
		a.devices[0].mu.Unlock()

		// 캐시에 기존 값 설정
		cache := a.caches["plc-1"]
		cache.UpdateHoldingRegisters(200, []uint16{7777})

		data, _ := json.Marshal(map[string]any{
			"command":   "write_register",
			"device_id": "plc-1",
			"params": map[string]any{
				"address": 200,
				"value":   12345,
			},
		})

		// MODBUS 예외는 JSON 에러 응답을 반환
		result, err := a.Process(data)
		require.NoError(t, err)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(result, &resp))
		assert.Equal(t, "error", resp["status"])

		// 캐시 확인: 기존 값이 유지되어야 한다
		cache.mu.RLock()
		regVal := cache.HoldingRegisters[200]
		cache.mu.RUnlock()
		assert.Equal(t, uint16(7777), regVal,
			"MODBUS 예외 시 캐시가 갱신되지 않아야 한다 (여전히 7777)")
	})
}

// ---------------------------------------------------------------------------
// 파라미터 타입 변환 테스트
// ---------------------------------------------------------------------------

func TestParamUint16_TypeConversions(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]any
		key     string
		want    uint16
		wantErr bool
	}{
		{
			name:   "float64",
			params: map[string]any{"addr": float64(100)},
			key:    "addr",
			want:   100,
		},
		{
			name:   "int",
			params: map[string]any{"addr": 200},
			key:    "addr",
			want:   200,
		},
		{
			name:    "missing key",
			params:  map[string]any{},
			key:     "addr",
			wantErr: true,
		},
		{
			name:    "wrong type",
			params:  map[string]any{"addr": "not-a-number"},
			key:     "addr",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := paramUint16(tc.params, tc.key)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestParamBool_TypeConversions(t *testing.T) {
	t.Run("valid bool", func(t *testing.T) {
		v, err := paramBool(map[string]any{"val": true}, "val")
		require.NoError(t, err)
		assert.True(t, v)
	})

	t.Run("missing key", func(t *testing.T) {
		_, err := paramBool(map[string]any{}, "val")
		require.Error(t, err)
	})

	t.Run("wrong type", func(t *testing.T) {
		_, err := paramBool(map[string]any{"val": "true"}, "val")
		require.Error(t, err)
	})
}

func TestParamBoolSlice_TypeConversions(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		params := map[string]any{"vals": []any{true, false, true}}
		got, err := paramBoolSlice(params, "vals")
		require.NoError(t, err)
		assert.Equal(t, []bool{true, false, true}, got)
	})

	t.Run("missing key", func(t *testing.T) {
		_, err := paramBoolSlice(map[string]any{}, "vals")
		require.Error(t, err)
	})

	t.Run("wrong element type", func(t *testing.T) {
		params := map[string]any{"vals": []any{true, "not-bool"}}
		_, err := paramBoolSlice(params, "vals")
		require.Error(t, err)
	})
}

func TestParamUint16Slice_TypeConversions(t *testing.T) {
	t.Run("valid float64", func(t *testing.T) {
		params := map[string]any{"vals": []any{float64(100), float64(200)}}
		got, err := paramUint16Slice(params, "vals")
		require.NoError(t, err)
		assert.Equal(t, []uint16{100, 200}, got)
	})

	t.Run("missing key", func(t *testing.T) {
		_, err := paramUint16Slice(map[string]any{}, "vals")
		require.Error(t, err)
	})

	t.Run("wrong element type", func(t *testing.T) {
		params := map[string]any{"vals": []any{float64(100), "not-number"}}
		_, err := paramUint16Slice(params, "vals")
		require.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// 필수 파라미터 누락 테스트
// ---------------------------------------------------------------------------

func TestProcessWrite_MissingParams(t *testing.T) {
	mt := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	t.Run("write_coil params 없음", func(t *testing.T) {
		data, _ := json.Marshal(map[string]any{
			"command":   "write_coil",
			"device_id": "plc-1",
		})
		_, err := a.Process(data)
		require.Error(t, err, "params 가 없으면 에러가 발생해야 한다")
	})

	t.Run("write_coil address 누락", func(t *testing.T) {
		data, _ := json.Marshal(map[string]any{
			"command":   "write_coil",
			"device_id": "plc-1",
			"params": map[string]any{
				"value": true,
			},
		})
		_, err := a.Process(data)
		require.Error(t, err, "address 누락 시 에러가 발생해야 한다")
	})

	t.Run("write_register value 누락", func(t *testing.T) {
		data, _ := json.Marshal(map[string]any{
			"command":   "write_register",
			"device_id": "plc-1",
			"params": map[string]any{
				"address": 100,
			},
		})
		_, err := a.Process(data)
		require.Error(t, err, "value 누락 시 에러가 발생해야 한다")
	})
}

// ---------------------------------------------------------------------------
// SendFrame 단위 테스트
// ---------------------------------------------------------------------------

func TestModbusDevice_SendFrame(t *testing.T) {
	t.Run("정상 전송", func(t *testing.T) {
		response := buildFC05Response(0, 1, 100, true)
		mt := &mockModbusTransport{connected: true, response: response}
		dev := newModbusDeviceWithTransport(DeviceConfig{ID: "test-dev", UnitID: 1}, mt, nil)
		dev.online = true

		frame := buildWriteSingleCoilRequest(0, 1, 100, true)
		resp, err := dev.SendFrame(nil, frame)
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("디바이스 오프라인", func(t *testing.T) {
		mt := &mockModbusTransport{connected: false}
		dev := newModbusDeviceWithTransport(DeviceConfig{ID: "test-dev", UnitID: 1}, mt, nil)
		dev.online = true

		frame := buildWriteSingleCoilRequest(0, 1, 100, true)
		_, err := dev.SendFrame(nil, frame)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrDeviceOffline))
	})

	t.Run("전송 에러", func(t *testing.T) {
		mt := &mockModbusTransport{
			connected:   true,
			sendRecvErr: errors.New("send failed"),
		}
		dev := newModbusDeviceWithTransport(DeviceConfig{ID: "test-dev", UnitID: 1}, mt, nil)
		dev.online = true

		frame := buildWriteSingleCoilRequest(0, 1, 100, true)
		_, err := dev.SendFrame(nil, frame)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "send failed")
	})
}
