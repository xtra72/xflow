package modbus

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// 파라미터 추출 헬퍼
// ---------------------------------------------------------------------------

// paramUint16 은 params 맵에서 key 에 해당하는 uint16 값을 추출한다.
// JSON/YAML 디코딩 시 숫자가 float64 로 전달될 수 있으므로 두 타입을 모두 처리한다.
func paramUint16(params map[string]any, key string) (uint16, error) {
	v, ok := params[key]
	if !ok {
		return 0, fmt.Errorf("modbus: missing param %q", key)
	}
	switch n := v.(type) {
	case float64:
		return uint16(n), nil
	case int:
		return uint16(n), nil
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, fmt.Errorf("modbus: param %q is not a valid number: %w", key, err)
		}
		return uint16(i), nil
	default:
		return 0, fmt.Errorf("modbus: param %q must be a number (got %T)", key, v)
	}
}

// paramBool 은 params 맵에서 key 에 해당하는 bool 값을 추출한다.
func paramBool(params map[string]any, key string) (bool, error) {
	v, ok := params[key]
	if !ok {
		return false, fmt.Errorf("modbus: missing param %q", key)
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("modbus: param %q must be a boolean (got %T)", key, v)
	}
	return b, nil
}

// paramBoolSlice 는 params 맵에서 key 에 해당하는 []bool 값을 추출한다.
// JSON 디코딩 시 []any 로 전달되므로 각 요소를 bool 로 변환한다.
func paramBoolSlice(params map[string]any, key string) ([]bool, error) {
	v, ok := params[key]
	if !ok {
		return nil, fmt.Errorf("modbus: missing param %q", key)
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("modbus: param %q must be an array (got %T)", key, v)
	}
	result := make([]bool, len(arr))
	for i, elem := range arr {
		b, ok := elem.(bool)
		if !ok {
			return nil, fmt.Errorf("modbus: param %q[%d] must be a boolean (got %T)", key, i, elem)
		}
		result[i] = b
	}
	return result, nil
}

// paramUint16Slice 는 params 맵에서 key 에 해당하는 []uint16 값을 추출한다.
// JSON 디코딩 시 숫자가 float64 로 전달될 수 있으므로 두 타입을 모두 처리한다.
func paramUint16Slice(params map[string]any, key string) ([]uint16, error) {
	v, ok := params[key]
	if !ok {
		return nil, fmt.Errorf("modbus: missing param %q", key)
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("modbus: param %q must be an array (got %T)", key, v)
	}
	result := make([]uint16, len(arr))
	for i, elem := range arr {
		switch n := elem.(type) {
		case float64:
			result[i] = uint16(n)
		case int:
			result[i] = uint16(n)
		case json.Number:
			iv, err := n.Int64()
			if err != nil {
				return nil, fmt.Errorf("modbus: param %q[%d] is not a valid number: %w", key, i, err)
			}
			result[i] = uint16(iv)
		default:
			return nil, fmt.Errorf("modbus: param %q[%d] must be a number (got %T)", key, i, elem)
		}
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// 공통 쓰기 응답 빌더
// ---------------------------------------------------------------------------

// writeSuccessResponse 는 쓰기 성공 시 JSON 응답을 생성한다.
func writeSuccessResponse(deviceID string, unitID byte, command string, addr uint16, quantity uint16) ([]byte, error) {
	resp := map[string]any{
		"status":    "ok",
		"device_id": deviceID,
		"unit_id":   unitID,
		"command":   command,
		"address":   addr,
		"quantity":  quantity,
		"timestamp": time.Now().Format(time.RFC3339),
	}
	return json.Marshal(resp)
}

// writeExceptionResponse 는 MODBUS 예외 발생 시 JSON 응답을 생성한다.
func writeExceptionResponse(deviceID string, unitID byte, command string, exc *ModbusException) ([]byte, error) {
	resp := map[string]any{
		"status":         "error",
		"device_id":      deviceID,
		"unit_id":        unitID,
		"command":        command,
		"error":          exc.Error(),
		"exception_code": exc.Code,
		"timestamp":      time.Now().Format(time.RFC3339),
	}
	return json.Marshal(resp)
}

// ---------------------------------------------------------------------------
// FC05: 단일 코일 쓰기
// ---------------------------------------------------------------------------

// processWriteCoil 은 write_coil 명령을 처리한다 (FC05).
// params: address (uint16), value (bool)
func (a *ModbusAgent) processWriteCoil(req *processRequest) ([]byte, error) {
	// 파라미터 추출
	addr, err := paramUint16(req.Params, "address")
	if err != nil {
		return nil, err
	}
	value, err := paramBool(req.Params, "value")
	if err != nil {
		return nil, err
	}

	// 디바이스 조회
	dev, err := a.findDevice(req.DeviceID)
	if err != nil {
		return nil, err
	}

	// 온라인 확인
	if !dev.IsOnline() {
		return nil, ErrDeviceOffline
	}

	// 프레임 빌드 및 전송
	txID := dev.nextTransactionID()
	frame := buildWriteSingleCoilRequest(txID, dev.config.UnitID, addr, value)

	ctx, cancel := context.WithTimeout(context.Background(), a.config.WriteTimeout)
	defer cancel()

	resp, err := dev.SendFrame(ctx, frame)
	if err != nil {
		a.stats.IncrMessagesErrored()
		return nil, err
	}

	// 응답 파싱
	unitID, _, respAddr, quantity, parseErr := parseWriteResponse(resp)
	if parseErr != nil {
		// MODBUS 예외 확인
		if exc, ok := parseErr.(*ModbusException); ok {
			a.stats.IncrMessagesErrored()
			if a.config.EnableWriteEvents {
				a.sendEvent("write_error", map[string]any{
					"device_id":      dev.config.ID,
					"unit_id":        dev.config.UnitID,
					"command":        "write_coil",
					"address":        addr,
					"error":          exc.Error(),
					"exception_code": exc.Code,
					"timestamp":      time.Now().Format(time.RFC3339),
				})
			}
			return writeExceptionResponse(dev.config.ID, unitID, "write_coil", exc)
		}
		a.stats.IncrMessagesErrored()
		return nil, parseErr
	}

	// Write-Through 캐시 갱신 (성공 시에만)
	if cache, ok := a.caches[dev.config.ID]; ok {
		cache.UpdateCoils(addr, []bool{value})
	}

	a.stats.IncrMessagesSent()
	a.stats.AddBytesWritten(int64(len(frame)))
	a.stats.UpdateLastActivity()

	if a.config.EnableWriteEvents {
		a.sendEvent("write_success", map[string]any{
			"device_id": dev.config.ID,
			"unit_id":   dev.config.UnitID,
			"command":   "write_coil",
			"address":   addr,
			"timestamp": time.Now().Format(time.RFC3339),
		})
	}

	return writeSuccessResponse(dev.config.ID, unitID, "write_coil", respAddr, quantity)
}

// ---------------------------------------------------------------------------
// FC06: 단일 레지스터 쓰기
// ---------------------------------------------------------------------------

// processWriteRegister 는 write_register 명령을 처리한다 (FC06).
// params: address (uint16), value (uint16)
func (a *ModbusAgent) processWriteRegister(req *processRequest) ([]byte, error) {
	// 파라미터 추출
	addr, err := paramUint16(req.Params, "address")
	if err != nil {
		return nil, err
	}
	value, err := paramUint16(req.Params, "value")
	if err != nil {
		return nil, err
	}

	// 디바이스 조회
	dev, err := a.findDevice(req.DeviceID)
	if err != nil {
		return nil, err
	}

	// 온라인 확인
	if !dev.IsOnline() {
		return nil, ErrDeviceOffline
	}

	// 프레임 빌드 및 전송
	txID := dev.nextTransactionID()
	frame := buildWriteSingleRegisterRequest(txID, dev.config.UnitID, addr, value)

	ctx, cancel := context.WithTimeout(context.Background(), a.config.WriteTimeout)
	defer cancel()

	resp, err := dev.SendFrame(ctx, frame)
	if err != nil {
		a.stats.IncrMessagesErrored()
		return nil, err
	}

	// 응답 파싱
	unitID, _, respAddr, quantity, parseErr := parseWriteResponse(resp)
	if parseErr != nil {
		if exc, ok := parseErr.(*ModbusException); ok {
			a.stats.IncrMessagesErrored()
			if a.config.EnableWriteEvents {
				a.sendEvent("write_error", map[string]any{
					"device_id":      dev.config.ID,
					"unit_id":        dev.config.UnitID,
					"command":        "write_register",
					"address":        addr,
					"error":          exc.Error(),
					"exception_code": exc.Code,
					"timestamp":      time.Now().Format(time.RFC3339),
				})
			}
			return writeExceptionResponse(dev.config.ID, unitID, "write_register", exc)
		}
		a.stats.IncrMessagesErrored()
		return nil, parseErr
	}

	// Write-Through 캐시 갱신
	if cache, ok := a.caches[dev.config.ID]; ok {
		cache.UpdateHoldingRegisters(addr, []uint16{value})
	}

	a.stats.IncrMessagesSent()
	a.stats.AddBytesWritten(int64(len(frame)))
	a.stats.UpdateLastActivity()

	if a.config.EnableWriteEvents {
		a.sendEvent("write_success", map[string]any{
			"device_id": dev.config.ID,
			"unit_id":   dev.config.UnitID,
			"command":   "write_register",
			"address":   addr,
			"timestamp": time.Now().Format(time.RFC3339),
		})
	}

	return writeSuccessResponse(dev.config.ID, unitID, "write_register", respAddr, quantity)
}

// ---------------------------------------------------------------------------
// FC15: 다중 코일 쓰기
// ---------------------------------------------------------------------------

// processWriteCoils 는 write_coils 명령을 처리한다 (FC15).
// params: address (uint16), values ([]bool)
func (a *ModbusAgent) processWriteCoils(req *processRequest) ([]byte, error) {
	// 파라미터 추출
	addr, err := paramUint16(req.Params, "address")
	if err != nil {
		return nil, err
	}
	values, err := paramBoolSlice(req.Params, "values")
	if err != nil {
		return nil, err
	}

	// 유효성 검증
	if len(values) == 0 {
		return nil, ErrQuantityExceeded
	}
	if len(values) > MaxCoilsWrite {
		return nil, ErrQuantityExceeded
	}

	// 디바이스 조회
	dev, err := a.findDevice(req.DeviceID)
	if err != nil {
		return nil, err
	}

	// 온라인 확인
	if !dev.IsOnline() {
		return nil, ErrDeviceOffline
	}

	// 프레임 빌드 및 전송
	txID := dev.nextTransactionID()
	frame := buildWriteMultipleCoilsRequest(txID, dev.config.UnitID, addr, values)

	ctx, cancel := context.WithTimeout(context.Background(), a.config.WriteTimeout)
	defer cancel()

	resp, err := dev.SendFrame(ctx, frame)
	if err != nil {
		a.stats.IncrMessagesErrored()
		return nil, err
	}

	// 응답 파싱
	unitID, _, respAddr, quantity, parseErr := parseWriteResponse(resp)
	if parseErr != nil {
		if exc, ok := parseErr.(*ModbusException); ok {
			a.stats.IncrMessagesErrored()
			if a.config.EnableWriteEvents {
				a.sendEvent("write_error", map[string]any{
					"device_id":      dev.config.ID,
					"unit_id":        dev.config.UnitID,
					"command":        "write_coils",
					"address":        addr,
					"error":          exc.Error(),
					"exception_code": exc.Code,
					"timestamp":      time.Now().Format(time.RFC3339),
				})
			}
			return writeExceptionResponse(dev.config.ID, unitID, "write_coils", exc)
		}
		a.stats.IncrMessagesErrored()
		return nil, parseErr
	}

	// Write-Through 캐시 갱신
	if cache, ok := a.caches[dev.config.ID]; ok {
		cache.UpdateCoils(addr, values)
	}

	a.stats.IncrMessagesSent()
	a.stats.AddBytesWritten(int64(len(frame)))
	a.stats.UpdateLastActivity()

	if a.config.EnableWriteEvents {
		a.sendEvent("write_success", map[string]any{
			"device_id": dev.config.ID,
			"unit_id":   dev.config.UnitID,
			"command":   "write_coils",
			"address":   addr,
			"timestamp": time.Now().Format(time.RFC3339),
		})
	}

	return writeSuccessResponse(dev.config.ID, unitID, "write_coils", respAddr, quantity)
}

// ---------------------------------------------------------------------------
// FC16: 다중 레지스터 쓰기
// ---------------------------------------------------------------------------

// processWriteRegisters 는 write_registers 명령을 처리한다 (FC16).
// params: address (uint16), values ([]uint16)
func (a *ModbusAgent) processWriteRegisters(req *processRequest) ([]byte, error) {
	// 파라미터 추출
	addr, err := paramUint16(req.Params, "address")
	if err != nil {
		return nil, err
	}
	values, err := paramUint16Slice(req.Params, "values")
	if err != nil {
		return nil, err
	}

	// 유효성 검증
	if len(values) == 0 {
		return nil, ErrQuantityExceeded
	}
	if len(values) > MaxRegistersWrite {
		return nil, ErrQuantityExceeded
	}

	// 디바이스 조회
	dev, err := a.findDevice(req.DeviceID)
	if err != nil {
		return nil, err
	}

	// 온라인 확인
	if !dev.IsOnline() {
		return nil, ErrDeviceOffline
	}

	// 프레임 빌드 및 전송
	txID := dev.nextTransactionID()
	frame := buildWriteMultipleRegistersRequest(txID, dev.config.UnitID, addr, values)

	ctx, cancel := context.WithTimeout(context.Background(), a.config.WriteTimeout)
	defer cancel()

	resp, err := dev.SendFrame(ctx, frame)
	if err != nil {
		a.stats.IncrMessagesErrored()
		return nil, err
	}

	// 응답 파싱
	unitID, _, respAddr, quantity, parseErr := parseWriteResponse(resp)
	if parseErr != nil {
		if exc, ok := parseErr.(*ModbusException); ok {
			a.stats.IncrMessagesErrored()
			if a.config.EnableWriteEvents {
				a.sendEvent("write_error", map[string]any{
					"device_id":      dev.config.ID,
					"unit_id":        dev.config.UnitID,
					"command":        "write_registers",
					"address":        addr,
					"error":          exc.Error(),
					"exception_code": exc.Code,
					"timestamp":      time.Now().Format(time.RFC3339),
				})
			}
			return writeExceptionResponse(dev.config.ID, unitID, "write_registers", exc)
		}
		a.stats.IncrMessagesErrored()
		return nil, parseErr
	}

	// Write-Through 캐시 갱신
	if cache, ok := a.caches[dev.config.ID]; ok {
		cache.UpdateHoldingRegisters(addr, values)
	}

	a.stats.IncrMessagesSent()
	a.stats.AddBytesWritten(int64(len(frame)))
	a.stats.UpdateLastActivity()

	if a.config.EnableWriteEvents {
		a.sendEvent("write_success", map[string]any{
			"device_id": dev.config.ID,
			"unit_id":   dev.config.UnitID,
			"command":   "write_registers",
			"address":   addr,
			"timestamp": time.Now().Format(time.RFC3339),
		})
	}

	return writeSuccessResponse(dev.config.ID, unitID, "write_registers", respAddr, quantity)
}
