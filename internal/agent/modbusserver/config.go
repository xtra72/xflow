package modbusserver

import (
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// 설정 구조체
// ---------------------------------------------------------------------------

// ModbusServerConfig 는 MODBUS/TCP 서버 에이전트의 설정을 나타낸다.
type ModbusServerConfig struct {
	ListenAddress  string        // 리슨 주소 (기본값 "0.0.0.0")
	ListenPort     int           // 리슨 포트 (기본값 502, 범위 1-65535)
	UnitID         byte          // 유닛 ID (기본값 1, 범위 0-247)
	MaxConnections int           // 최대 연결 수 (기본값 10, > 0)
	IdleTimeout    time.Duration // 유휴 타임아웃 (기본값 60s)
	MsgChannelSize int           // 메시지 채널 버퍼 크기 (기본값 256)
	RegisterMap    RegisterMapConfig
}

// RegisterMapConfig 는 레지스터 맵의 설정을 나타낸다.
type RegisterMapConfig struct {
	Coils            *RegisterAreaConfig // 코일 영역 (FC01/FC05/FC15)
	DiscreteInputs   *RegisterAreaConfig // 이산 입력 영역 (FC02)
	HoldingRegisters *RegisterAreaConfig // 보유 레지스터 영역 (FC03/FC06/FC16)
	InputRegisters   *RegisterAreaConfig // 입력 레지스터 영역 (FC04)
}

// RegisterAreaConfig 는 단일 레지스터 영역의 설정을 나타낸다.
type RegisterAreaConfig struct {
	StartAddress  uint16 // 시작 주소
	Count         uint16 // 레지스터 수 (필수, > 0)
	InitialValues []any  // 초기값 (선택); 코일/DI 는 bool, 레지스터는 숫자
}

// ---------------------------------------------------------------------------
// 설정 파싱
// ---------------------------------------------------------------------------

// parseModbusServerConfig 는 Transport.Options 맵에서 ModbusServerConfig 를 파싱한다.
func parseModbusServerConfig(opts map[string]any) (ModbusServerConfig, error) {
	cfg := ModbusServerConfig{
		ListenAddress:  "0.0.0.0",
		ListenPort:     502,
		UnitID:         1,
		MaxConnections: 10,
		IdleTimeout:    60 * time.Second,
		MsgChannelSize: 256,
	}

	// listen_address
	if v, ok := opts["listen_address"]; ok {
		if s, ok := v.(string); ok {
			cfg.ListenAddress = s
		}
	}

	// listen_port
	if v, ok := opts["listen_port"]; ok {
		cfg.ListenPort = toInt(v)
	}
	if cfg.ListenPort < 0 || cfg.ListenPort > 65535 {
		return ModbusServerConfig{}, fmt.Errorf(
			"modbus-server: listen_port must be 0-65535 (got %d)", cfg.ListenPort)
	}

	// unit_id
	if v, ok := opts["unit_id"]; ok {
		id := toInt(v)
		if id < 0 || id > 247 {
			return ModbusServerConfig{}, fmt.Errorf(
				"modbus-server: unit_id must be 0-247 (got %d)", id)
		}
		cfg.UnitID = byte(id)
	}

	// max_connections
	if v, ok := opts["max_connections"]; ok {
		cfg.MaxConnections = toInt(v)
	}
	if cfg.MaxConnections < 1 {
		return ModbusServerConfig{}, fmt.Errorf(
			"modbus-server: max_connections must be > 0 (got %d)", cfg.MaxConnections)
	}

	// idle_timeout
	if v, ok := opts["idle_timeout"]; ok {
		if s, ok := v.(string); ok {
			d, err := time.ParseDuration(s)
			if err != nil {
				return ModbusServerConfig{}, fmt.Errorf("modbus-server: invalid idle_timeout: %w", err)
			}
			cfg.IdleTimeout = d
		}
	}

	// msg_channel_size
	if v, ok := opts["msg_channel_size"]; ok {
		cfg.MsgChannelSize = toInt(v)
	}

	// register_map (필수)
	rmRaw, ok := opts["register_map"]
	if !ok {
		return ModbusServerConfig{}, fmt.Errorf(
			"modbus-server: register_map is required: %w", ErrInvalidRegisterMap)
	}
	rmMap, ok := rmRaw.(map[string]any)
	if !ok {
		return ModbusServerConfig{}, fmt.Errorf(
			"modbus-server: register_map must be a map: %w", ErrInvalidRegisterMap)
	}

	rmCfg, err := parseRegisterMapConfig(rmMap)
	if err != nil {
		return ModbusServerConfig{}, err
	}
	cfg.RegisterMap = rmCfg

	return cfg, nil
}

// parseRegisterMapConfig 는 레지스터 맵 설정 맵을 RegisterMapConfig 로 파싱한다.
func parseRegisterMapConfig(m map[string]any) (RegisterMapConfig, error) {
	var cfg RegisterMapConfig
	hasArea := false

	// coils
	if v, ok := m["coils"]; ok {
		areaMap, ok := v.(map[string]any)
		if !ok {
			return RegisterMapConfig{}, fmt.Errorf("modbus-server: register_map.coils must be a map")
		}
		area, err := parseRegisterAreaConfig(areaMap, "coils")
		if err != nil {
			return RegisterMapConfig{}, err
		}
		cfg.Coils = &area
		hasArea = true
	}

	// discrete_inputs
	if v, ok := m["discrete_inputs"]; ok {
		areaMap, ok := v.(map[string]any)
		if !ok {
			return RegisterMapConfig{}, fmt.Errorf("modbus-server: register_map.discrete_inputs must be a map")
		}
		area, err := parseRegisterAreaConfig(areaMap, "discrete_inputs")
		if err != nil {
			return RegisterMapConfig{}, err
		}
		cfg.DiscreteInputs = &area
		hasArea = true
	}

	// holding_registers
	if v, ok := m["holding_registers"]; ok {
		areaMap, ok := v.(map[string]any)
		if !ok {
			return RegisterMapConfig{}, fmt.Errorf("modbus-server: register_map.holding_registers must be a map")
		}
		area, err := parseRegisterAreaConfig(areaMap, "holding_registers")
		if err != nil {
			return RegisterMapConfig{}, err
		}
		cfg.HoldingRegisters = &area
		hasArea = true
	}

	// input_registers
	if v, ok := m["input_registers"]; ok {
		areaMap, ok := v.(map[string]any)
		if !ok {
			return RegisterMapConfig{}, fmt.Errorf("modbus-server: register_map.input_registers must be a map")
		}
		area, err := parseRegisterAreaConfig(areaMap, "input_registers")
		if err != nil {
			return RegisterMapConfig{}, err
		}
		cfg.InputRegisters = &area
		hasArea = true
	}

	// 최소 하나의 영역이 있어야 한다
	if !hasArea {
		return RegisterMapConfig{}, fmt.Errorf(
			"modbus-server: register_map must have at least one area: %w", ErrInvalidRegisterMap)
	}

	return cfg, nil
}

// parseRegisterAreaConfig 는 레지스터 영역 설정 맵을 RegisterAreaConfig 로 파싱한다.
func parseRegisterAreaConfig(m map[string]any, areaName string) (RegisterAreaConfig, error) {
	var area RegisterAreaConfig

	// start_address
	if v, ok := m["start_address"]; ok {
		area.StartAddress = toUint16(v)
	}

	// count (필수, > 0)
	if v, ok := m["count"]; ok {
		area.Count = toUint16(v)
	}
	if area.Count == 0 {
		return RegisterAreaConfig{}, fmt.Errorf(
			"modbus-server: register_map.%s.count must be > 0", areaName)
	}

	// initial_values (선택)
	if v, ok := m["initial_values"]; ok {
		if vals, ok := v.([]any); ok {
			if len(vals) > int(area.Count) {
				return RegisterAreaConfig{}, fmt.Errorf(
					"modbus-server: register_map.%s.initial_values length (%d) exceeds count (%d)",
					areaName, len(vals), area.Count)
			}
			area.InitialValues = vals
		}
	}

	return area, nil
}

// ---------------------------------------------------------------------------
// 타입 변환 헬퍼
// ---------------------------------------------------------------------------

// toInt 는 int 또는 float64 값을 int 로 변환한다.
// YAML/JSON 파싱에서 숫자가 float64 로 전달될 수 있으므로 두 타입 모두 처리한다.
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}

// toByte 는 int 또는 float64 값을 byte 로 변환한다.
func toByte(v any) byte {
	switch n := v.(type) {
	case int:
		return byte(n)
	case float64:
		return byte(n)
	default:
		return 0
	}
}

// toUint16 는 int 또는 float64 값을 uint16 으로 변환한다.
func toUint16(v any) uint16 {
	switch n := v.(type) {
	case int:
		return uint16(n)
	case float64:
		return uint16(n)
	default:
		return 0
	}
}
