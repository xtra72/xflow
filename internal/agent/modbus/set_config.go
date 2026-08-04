package modbus

import (
	"encoding/json"
	"fmt"
	"time"
)

// initOnlySerialKeys 는 런타임 set_config 로 변경할 수 없는 RTU 시리얼 하드웨어 파라미터 키이다(M9).
// 트랜스포트 오픈에 귀속되므로 init 전용으로 유지한다(spec §5.5.1, AC-08 (b)).
var initOnlySerialKeys = []string{"serial_port", "port", "baud", "baud_rate", "data_bits", "stop_bits", "parity"}

// processSetConfig 는 실행 중인 클라이언트 에이전트를 재시작 없이 재구성한다(M9, REQ-MODBUS-006-02/05, AC-08).
//
// 플로우 노드가 agent_ref → Process([]byte) 로 발행하는
// {"command":"set_config","device_id":...,"params":{...}} JSON 을 처리한다. 이는 신규 병렬
// 메커니즘이 아니라 기존 Process 명령 디스패치 + Configure/SetPollInterval 런타임 변경 선례의 확장이다.
//
// 동작 규약:
//   - init 전용 필드(transport tcp↔rtu 전환, RTU 시리얼 하드웨어 파라미터)는 오류로 거부하고
//     에이전트는 직전 설정으로 계속 동작한다(부분 적용 없음, AC-08 (b)).
//   - 런타임 가변 필드는 parseModbusConfig/parseRegisterGroupConfig 와 동일한 파싱·검증 규칙을
//     재사용하여 적용 전 전량 검증한다(검증 실패 시 아무 것도 적용하지 않음).
//   - 적용은 기존 관례(Configure/SetPollInterval)와 동일하게 a.mu.Lock()(RWMutex) 하에서
//     수행하여 폴링 goroutine 과의 경합을 방지한다.
//
// 런타임 가변 필드:
//   - register_groups (디바이스 스코프): 추가/수정/삭제/enable-disable(존재/부재로 표현),
//     그룹별 poll_interval, data_type/byte_order(TypeOverlay 재구축)
//   - poll_interval / request_timeout / reconnect_interval (에이전트 기본 케이던스·타깃 파라미터)
func (a *ModbusAgent) processSetConfig(req *processRequest) ([]byte, error) {
	if req.Params == nil {
		return nil, fmt.Errorf("modbus set_config: params required")
	}
	params := req.Params

	// (1) init 전용 필드 거부 — 어떤 변경보다 먼저 검사하여 부분 적용을 원천 차단한다(AC-08 (b)).
	if err := rejectInitOnlyFields(params); err != nil {
		return nil, err
	}

	// (2) 변경 필드 파싱·검증 — update_device 와 공유하는 단일 소스(parseDeviceReconfig, 중복 회피).
	// parseModbusConfig 와 동일한 규칙을 재사용하여 전량 검증한다(부분 적용 금지).
	rc, err := parseDeviceReconfig(params)
	if err != nil {
		return nil, err
	}

	// register_groups / unit_id 변경은 대상 디바이스를 필요로 한다(디바이스 스코프). findDevice 는
	// 불변 필드(dev.config.ID)만 읽으므로 락 없이 안전하다.
	var dev *ModbusDevice
	if rc.hasDeviceScoped() {
		if req.DeviceID == "" {
			return nil, fmt.Errorf("modbus set_config: device_id required when changing register_groups or unit_id")
		}
		d, err := a.findDevice(req.DeviceID)
		if err != nil {
			return nil, err
		}
		dev = d
	}

	if !rc.hasAny() {
		return nil, fmt.Errorf("modbus set_config: no runtime-mutable fields provided")
	}

	// (3) 전량 검증 통과 → a.mu.Lock() 하에 원자적 적용(부분 적용 없음).
	// 적용 로직은 update_device 와 공유하는 단일 경로(applyDeviceReconfigLocked)를 사용한다.
	a.mu.Lock()
	defer a.mu.Unlock()
	a.applyDeviceReconfigLocked(dev, rc)

	resp := map[string]any{
		"status":    "reconfigured",
		"device_id": req.DeviceID,
		"applied":   appliedSetConfigKeys(rc.hasGroups, rc.hasUnitID, rc.hasPoll, rc.hasReqTO, rc.hasReconn),
	}
	return json.Marshal(resp)
}

// rejectInitOnlyFields 는 런타임 변경이 금지된 init 전용 필드가 params 에 포함되어 있으면
// ErrInitOnlyField 를 반환한다(M9, AC-08 (b)). transport 전환과 RTU 시리얼 하드웨어 파라미터가 대상이다.
func rejectInitOnlyFields(params map[string]any) error {
	if _, ok := params["transport"]; ok {
		return fmt.Errorf(
			"modbus set_config: transport (tcp<->rtu switch) cannot change at runtime: %w",
			ErrInitOnlyField)
	}
	for _, k := range initOnlySerialKeys {
		if _, ok := params[k]; ok {
			return fmt.Errorf(
				"modbus set_config: serial hardware parameter %q cannot change at runtime: %w",
				k, ErrInitOnlyField)
		}
	}
	return nil
}

// parseSetConfigRegisterGroups 는 set_config params 의 register_groups 값을 파싱·검증한다(M9).
// parseRegisterGroupConfig(init 경로와 동일)를 재사용하여 fc(1-4)·quantity(>0)·poll_interval(>0)·
// data_type/byte_order 유효성 등을 init 과 동일하게 검증한다.
func parseSetConfigRegisterGroups(v any) ([]RegisterGroupConfig, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("modbus set_config: register_groups must be an array")
	}
	groups := make([]RegisterGroupConfig, 0, len(list))
	for j, raw := range list {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("modbus set_config: register_groups[%d] is not a map", j)
		}
		rg, err := parseRegisterGroupConfig(m, 0, j)
		if err != nil {
			return nil, err
		}
		groups = append(groups, rg)
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("modbus set_config: register_groups must not be empty")
	}
	return groups, nil
}

// parseUnitIDParam 은 set_config 의 unit_id 파라미터를 파싱·검증한다(SPEC-MODBUS-006 unit_id 런타임 가변화).
// JSON 숫자는 float64 로 전달되므로 int/float64 를 모두 허용하되, MODBUS unit_id 는 1바이트이므로
// [0,255] 범위를 벗어나거나 정수가 아니면 설정 오류로 거부한다(init 경로 toByte 의 무언의 절삭을
// 런타임 경로에서는 명시적 오류로 승격 — 부분 적용 금지 원칙과 정합).
func parseUnitIDParam(v any) (byte, error) {
	var n int
	switch x := v.(type) {
	case int:
		n = x
	case float64:
		if x != float64(int(x)) {
			return 0, fmt.Errorf("modbus set_config: unit_id must be an integer, got %v", x)
		}
		n = int(x)
	default:
		return 0, fmt.Errorf("modbus set_config: unit_id must be a number")
	}
	if n < 0 || n > 255 {
		return 0, fmt.Errorf("modbus set_config: unit_id must be in [0,255], got %d", n)
	}
	return byte(n), nil
}

// parseDurationParam 은 set_config 의 duration 문자열 파라미터를 파싱한다(M9).
// parseModbusConfig 와 동일하게 time.ParseDuration 을 사용한다.
func parseDurationParam(name string, v any) (time.Duration, error) {
	s, ok := v.(string)
	if !ok {
		return 0, fmt.Errorf("modbus set_config: %s must be a duration string", name)
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("modbus set_config: invalid %s: %w", name, err)
	}
	return d, nil
}

// rebuildDeviceTypeOverlay 는 대상 디바이스의 캐시 TypeOverlay 를 현재 레지스터 그룹으로 재구축한다(M9).
// 호출자는 a.mu.Lock() 을 보유해야 한다. 그룹에서 타입 정보가 사라지면 오버레이를 nil 로 해제한다.
func (a *ModbusAgent) rebuildDeviceTypeOverlay(dev *ModbusDevice) {
	cache, ok := a.caches[dev.config.ID]
	if !ok {
		return
	}
	// buildCacheTypeOverlay 는 타입 정보가 없으면 nil 을 반환한다. SetTypeOverlay(nil) 은 오버레이를 해제한다.
	cache.SetTypeOverlay(buildCacheTypeOverlay(dev.config.RegisterGroups))
}

// appliedSetConfigKeys 는 실제로 적용된 필드 키 목록을 응답용으로 만든다(M9, unit_id 추가).
func appliedSetConfigKeys(hasGroups, hasUnitID, hasPoll, hasReqTO, hasReconn bool) []string {
	applied := make([]string, 0, 5)
	if hasGroups {
		applied = append(applied, "register_groups")
	}
	if hasUnitID {
		applied = append(applied, "unit_id")
	}
	if hasPoll {
		applied = append(applied, "poll_interval")
	}
	if hasReqTO {
		applied = append(applied, "request_timeout")
	}
	if hasReconn {
		applied = append(applied, "reconnect_interval")
	}
	return applied
}
