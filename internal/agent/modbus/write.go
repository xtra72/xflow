package modbus

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	modbus "github.com/xtra/xflow/internal/modbus"
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

// paramFloat64 는 params 맵에서 key 에 해당하는 float64 값을 추출한다.
// int, float64, json.Number 타입을 모두 처리한다.
func paramFloat64(params map[string]any, key string) (float64, error) {
	v, ok := params[key]
	if !ok {
		return 0, fmt.Errorf("modbus: missing param %q", key)
	}
	switch n := v.(type) {
	case float64:
		return n, nil
	case int:
		return float64(n), nil
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, fmt.Errorf("modbus: param %q is not a valid number: %w", key, err)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("modbus: param %q must be a number (got %T)", key, v)
	}
}

// paramString 은 params 맵에서 key 에 해당하는 string 값을 추출한다.
// 키가 없으면 빈 문자열과 false 를 반환한다 (선택적 파라미터용).
func paramString(params map[string]any, key string) (string, bool) {
	v, ok := params[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return s, true
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

	// PDU 빌드 및 전송 (unitID/ADU 프레이밍은 트랜스포트가 담당)
	pdu := buildWriteSingleCoilPDU(addr, value)

	ctx, cancel := context.WithTimeout(context.Background(), a.config.WriteTimeout)
	defer cancel()

	writeStart := time.Now()
	resp, err := dev.SendPDU(ctx, pdu)
	a.recordRequestStat(dev.config.ID, "", err == nil, time.Since(writeStart)) // M7: 쓰기 트랜스포트 통계
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		return nil, err
	}

	// 응답 파싱 (unitID 는 요청 unitID 를 그대로 사용 — 응답이 이를 에코함)
	_, respAddr, quantity, parseErr := parseWriteResponse(resp)
	if parseErr != nil {
		// MODBUS 예외 확인
		if exc, ok := parseErr.(*ModbusException); ok {
			a.stats.IncrExternalMessagesErrored()
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
			return writeExceptionResponse(dev.config.ID, dev.config.UnitID, "write_coil", exc)
		}
		a.stats.IncrExternalMessagesErrored()
		return nil, parseErr
	}

	// Write-Through 캐시 갱신 (성공 시에만)
	if cache, ok := a.caches[dev.config.ID]; ok {
		cache.UpdateCoils(addr, []bool{value})
	}

	a.stats.IncrExternalMessagesSent()
	a.stats.AddBytesWritten(int64(len(pdu)))
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

	return writeSuccessResponse(dev.config.ID, dev.config.UnitID, "write_coil", respAddr, quantity)
}

// ---------------------------------------------------------------------------
// FC06: 단일 레지스터 쓰기
// ---------------------------------------------------------------------------

// processWriteRegister 는 write_register 명령을 처리한다 (FC06).
// params: address (uint16), value (uint16 또는 float64), data_type (선택)
// data_type 이 2-레지스터 타입(float32, uint32, int32)이면 FC16 으로 자동 전환한다.
func (a *ModbusAgent) processWriteRegister(req *processRequest) ([]byte, error) {
	// 파라미터 추출
	addr, err := paramUint16(req.Params, "address")
	if err != nil {
		return nil, err
	}

	// data_type 파라미터 확인 (선택)
	dataType, hasDataType := paramString(req.Params, "data_type")
	if hasDataType {
		if !modbus.IsValidDataType(dataType) {
			return nil, fmt.Errorf("modbus: unsupported data_type %q: %w", dataType, ErrUnsupportedDataType)
		}
	}

	// byte_order 파라미터 확인 (선택, 기본값 big_endian)
	byteOrder := modbus.ByteOrderBigEndian
	if bo, ok := paramString(req.Params, "byte_order"); ok {
		byteOrder = bo
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

	// data_type 에 따라 분기
	if hasDataType {
		regCount, _ := modbus.RegisterCountForType(dataType)

		if regCount == 2 {
			// 2-레지스터 타입: FC16 사용
			rawValue, err := paramFloat64(req.Params, "value")
			if err != nil {
				return nil, err
			}

			regs, err := modbus.TypedValueToRegisters(rawValue, dataType, byteOrder)
			if err != nil {
				return nil, err
			}

			return a.sendWriteMultipleRegisters(dev, addr, regs, "write_register", dataType)
		}

		// 1-레지스터 타입 (uint16, int16): 타입 변환 후 FC06
		rawValue, err := paramFloat64(req.Params, "value")
		if err != nil {
			return nil, err
		}

		regs, err := modbus.TypedValueToRegisters(rawValue, dataType, byteOrder)
		if err != nil {
			return nil, err
		}

		return a.sendWriteSingleRegister(dev, addr, regs[0], "write_register", dataType)
	}

	// data_type 미지정: 기존 uint16 동작
	value, err := paramUint16(req.Params, "value")
	if err != nil {
		return nil, err
	}

	return a.sendWriteSingleRegister(dev, addr, value, "write_register", "")
}

// sendWriteSingleRegister 는 FC06 PDU 를 빌드하고 전송한다.
func (a *ModbusAgent) sendWriteSingleRegister(dev *ModbusDevice, addr uint16, value uint16, command string, dataType string) ([]byte, error) {
	pdu := buildWriteSingleRegisterPDU(addr, value)

	ctx, cancel := context.WithTimeout(context.Background(), a.config.WriteTimeout)
	defer cancel()

	writeStart := time.Now()
	resp, err := dev.SendPDU(ctx, pdu)
	a.recordRequestStat(dev.config.ID, "", err == nil, time.Since(writeStart)) // M7: 쓰기 트랜스포트 통계
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		return nil, err
	}

	_, respAddr, quantity, parseErr := parseWriteResponse(resp)
	if parseErr != nil {
		if exc, ok := parseErr.(*ModbusException); ok {
			a.stats.IncrExternalMessagesErrored()
			if a.config.EnableWriteEvents {
				evtData := map[string]any{
					"device_id":      dev.config.ID,
					"unit_id":        dev.config.UnitID,
					"command":        command,
					"address":        addr,
					"error":          exc.Error(),
					"exception_code": exc.Code,
					"timestamp":      time.Now().Format(time.RFC3339),
				}
				if dataType != "" {
					evtData["data_type"] = dataType
				}
				a.sendEvent("write_error", evtData)
			}
			return writeExceptionResponse(dev.config.ID, dev.config.UnitID, command, exc)
		}
		a.stats.IncrExternalMessagesErrored()
		return nil, parseErr
	}

	// Write-Through 캐시 갱신
	if cache, ok := a.caches[dev.config.ID]; ok {
		cache.UpdateHoldingRegisters(addr, []uint16{value})
	}

	a.stats.IncrExternalMessagesSent()
	a.stats.AddBytesWritten(int64(len(pdu)))
	a.stats.UpdateLastActivity()

	if a.config.EnableWriteEvents {
		evtData := map[string]any{
			"device_id": dev.config.ID,
			"unit_id":   dev.config.UnitID,
			"command":   command,
			"address":   addr,
			"timestamp": time.Now().Format(time.RFC3339),
		}
		if dataType != "" {
			evtData["data_type"] = dataType
		}
		a.sendEvent("write_success", evtData)
	}

	return writeSuccessResponse(dev.config.ID, dev.config.UnitID, command, respAddr, quantity)
}

// sendWriteMultipleRegisters 는 FC16 PDU 를 빌드하고 전송한다.
func (a *ModbusAgent) sendWriteMultipleRegisters(dev *ModbusDevice, addr uint16, values []uint16, command string, dataType string) ([]byte, error) {
	pdu := buildWriteMultipleRegistersPDU(addr, values)

	ctx, cancel := context.WithTimeout(context.Background(), a.config.WriteTimeout)
	defer cancel()

	writeStart := time.Now()
	resp, err := dev.SendPDU(ctx, pdu)
	a.recordRequestStat(dev.config.ID, "", err == nil, time.Since(writeStart)) // M7: 쓰기 트랜스포트 통계
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		return nil, err
	}

	_, respAddr, quantity, parseErr := parseWriteResponse(resp)
	if parseErr != nil {
		if exc, ok := parseErr.(*ModbusException); ok {
			a.stats.IncrExternalMessagesErrored()
			if a.config.EnableWriteEvents {
				evtData := map[string]any{
					"device_id":      dev.config.ID,
					"unit_id":        dev.config.UnitID,
					"command":        command,
					"address":        addr,
					"error":          exc.Error(),
					"exception_code": exc.Code,
					"timestamp":      time.Now().Format(time.RFC3339),
				}
				if dataType != "" {
					evtData["data_type"] = dataType
				}
				a.sendEvent("write_error", evtData)
			}
			return writeExceptionResponse(dev.config.ID, dev.config.UnitID, command, exc)
		}
		a.stats.IncrExternalMessagesErrored()
		return nil, parseErr
	}

	// Write-Through 캐시 갱신 (raw uint16 레지스터)
	if cache, ok := a.caches[dev.config.ID]; ok {
		cache.UpdateHoldingRegisters(addr, values)
	}

	a.stats.IncrExternalMessagesSent()
	a.stats.AddBytesWritten(int64(len(pdu)))
	a.stats.UpdateLastActivity()

	if a.config.EnableWriteEvents {
		evtData := map[string]any{
			"device_id": dev.config.ID,
			"unit_id":   dev.config.UnitID,
			"command":   command,
			"address":   addr,
			"timestamp": time.Now().Format(time.RFC3339),
		}
		if dataType != "" {
			evtData["data_type"] = dataType
		}
		a.sendEvent("write_success", evtData)
	}

	return writeSuccessResponse(dev.config.ID, dev.config.UnitID, command, respAddr, quantity)
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

	// PDU 빌드 및 전송 (unitID/ADU 프레이밍은 트랜스포트가 담당)
	pdu := buildWriteMultipleCoilsPDU(addr, values)

	ctx, cancel := context.WithTimeout(context.Background(), a.config.WriteTimeout)
	defer cancel()

	writeStart := time.Now()
	resp, err := dev.SendPDU(ctx, pdu)
	a.recordRequestStat(dev.config.ID, "", err == nil, time.Since(writeStart)) // M7: 쓰기 트랜스포트 통계
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		return nil, err
	}

	// 응답 파싱 (unitID 는 요청 unitID 를 그대로 사용 — 응답이 이를 에코함)
	_, respAddr, quantity, parseErr := parseWriteResponse(resp)
	if parseErr != nil {
		if exc, ok := parseErr.(*ModbusException); ok {
			a.stats.IncrExternalMessagesErrored()
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
			return writeExceptionResponse(dev.config.ID, dev.config.UnitID, "write_coils", exc)
		}
		a.stats.IncrExternalMessagesErrored()
		return nil, parseErr
	}

	// Write-Through 캐시 갱신
	if cache, ok := a.caches[dev.config.ID]; ok {
		cache.UpdateCoils(addr, values)
	}

	a.stats.IncrExternalMessagesSent()
	a.stats.AddBytesWritten(int64(len(pdu)))
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

	return writeSuccessResponse(dev.config.ID, dev.config.UnitID, "write_coils", respAddr, quantity)
}

// ---------------------------------------------------------------------------
// FC16: 다중 레지스터 쓰기
// ---------------------------------------------------------------------------

// processWriteRegisters 는 write_registers 명령을 처리한다 (FC16).
// params: address (uint16), values ([]uint16 또는 []float64), data_type (선택)
// data_type 지정 시 각 값을 해당 타입의 레지스터 표현으로 변환하여 FC16 전송한다.
func (a *ModbusAgent) processWriteRegisters(req *processRequest) ([]byte, error) {
	// 파라미터 추출
	addr, err := paramUint16(req.Params, "address")
	if err != nil {
		return nil, err
	}

	// data_type 파라미터 확인 (선택)
	dataType, hasDataType := paramString(req.Params, "data_type")
	if hasDataType {
		if !modbus.IsValidDataType(dataType) {
			return nil, fmt.Errorf("modbus: unsupported data_type %q: %w", dataType, ErrUnsupportedDataType)
		}
	}

	// byte_order 파라미터 확인 (선택, 기본값 big_endian)
	byteOrder := modbus.ByteOrderBigEndian
	if bo, ok := paramString(req.Params, "byte_order"); ok {
		byteOrder = bo
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

	var values []uint16

	if hasDataType {
		// data_type 지정: 각 값을 타입 변환하여 uint16 슬라이스로 결합
		values, err = extractTypedValues(req.Params, "values", dataType, byteOrder)
		if err != nil {
			return nil, err
		}
	} else {
		// data_type 미지정: 기존 uint16 배열 동작
		values, err = paramUint16Slice(req.Params, "values")
		if err != nil {
			return nil, err
		}
	}

	// 유효성 검증
	if len(values) == 0 {
		return nil, ErrQuantityExceeded
	}
	if len(values) > MaxRegistersWrite {
		return nil, ErrQuantityExceeded
	}

	return a.sendWriteMultipleRegisters(dev, addr, values, "write_registers", dataType)
}

// extractTypedValues 는 params["values"] 배열의 각 요소를 data_type 에 따라
// 레지스터 표현으로 변환하고, 모든 결과를 하나의 uint16 슬라이스로 결합한다.
func extractTypedValues(params map[string]any, key string, dataType string, byteOrder string) ([]uint16, error) {
	v, ok := params[key]
	if !ok {
		return nil, fmt.Errorf("modbus: missing param %q", key)
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("modbus: param %q must be an array (got %T)", key, v)
	}

	var allRegs []uint16
	for i, elem := range arr {
		var fval float64
		switch n := elem.(type) {
		case float64:
			fval = n
		case int:
			fval = float64(n)
		case json.Number:
			f, err := n.Float64()
			if err != nil {
				return nil, fmt.Errorf("modbus: param %q[%d] is not a valid number: %w", key, i, err)
			}
			fval = f
		default:
			return nil, fmt.Errorf("modbus: param %q[%d] must be a number (got %T)", key, i, elem)
		}

		regs, err := modbus.TypedValueToRegisters(fval, dataType, byteOrder)
		if err != nil {
			return nil, fmt.Errorf("modbus: param %q[%d] conversion error: %w", key, i, err)
		}
		allRegs = append(allRegs, regs...)
	}

	return allRegs, nil
}
