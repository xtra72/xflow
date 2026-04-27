package modbusserver

import (
	"fmt"
	"time"

	modbus "github.com/xtra/xflow/internal/modbus"
)

// ---------------------------------------------------------------------------
// 설정 구조체
// ---------------------------------------------------------------------------

// ModbusServerConfig 는 MODBUS/TCP 서버 에이전트의 설정을 나타낸다.
type ModbusServerConfig struct {
	ListenAddress  string        // 리슨 주소 (기본값 "0.0.0.0")
	ListenPort     int           // 리슨 포트 (기본값 502, 범위 1-65535)
	UnitID         byte          // 유닛 ID (기본값 1, 범위 0-247) — 하위 호환용, Devices 가 없을 때 사용
	MaxConnections int           // 최대 연결 수 (기본값 10, > 0)
	IdleTimeout    time.Duration // 유휴 타임아웃 (기본값 60s)
	MsgChannelSize int           // 메시지 채널 버퍼 크기 (기본값 256)
	RegisterMap    RegisterMapConfig // 하위 호환용, Devices 가 없을 때 사용
	Devices        []DeviceConfig    // 다중 디바이스 설정 (M1: 멀티-디바이스 지원)
}

// DeviceConfig 는 단일 가상 디바이스의 설정을 나타낸다.
type DeviceConfig struct {
	UnitID       byte              // 유닛 ID (범위 1-247)
	Name         string            // 디바이스 이름 (선택, 로깅/식별용)
	RegisterMap  RegisterMapConfig // 디바이스별 레지스터 맵
	RegisterDefs []any             // 디바이스별 레지스터 정의 (Bridge Adapter 매핑용)
}

// RegisterMapConfig 는 레지스터 맵의 설정을 나타낸다.
// 각 영역은 하나 이상의 비연속 세그먼트를 가질 수 있다.
type RegisterMapConfig struct {
	Coils            []*RegisterAreaConfig // 코일 영역 (FC01/FC05/FC15)
	DiscreteInputs   []*RegisterAreaConfig // 이산 입력 영역 (FC02)
	HoldingRegisters []*RegisterAreaConfig // 보유 레지스터 영역 (FC03/FC06/FC16)
	InputRegisters   []*RegisterAreaConfig // 입력 레지스터 영역 (FC04)
}

// RegisterAreaConfig 는 단일 레지스터 영역의 설정을 나타낸다.
type RegisterAreaConfig struct {
	StartAddress  uint16              // 시작 주소
	Count         uint16              // 레지스터 수 (필수, > 0)
	InitialValues []any               // 초기값 (선택); 코일/DI 는 bool, 레지스터는 숫자
	DataType      string              // 영역 기본 데이터 타입 (기본: "uint16")
	TypeMap       []modbus.TypeMapEntry // 주소별 타입 오버라이드 (선택)
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

	// unit_id (하위 호환용)
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

	// ---------------------------------------------------------------
	// devices (멀티-디바이스) 또는 register_map (하위 호환)
	// ---------------------------------------------------------------
	if devicesRaw, ok := opts["devices"]; ok {
		// 멀티-디바이스 설정
		devices, err := parseDevicesConfig(devicesRaw)
		if err != nil {
			return ModbusServerConfig{}, err
		}
		cfg.Devices = devices
	} else if rmRaw, ok := opts["register_map"]; ok {
		// 하위 호환: unit_id + register_map → 단일 DeviceConfig 로 변환
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
		// 최상위 register_defs 를 단일 디바이스에 상속 (하위 호환)
		var topDefs []any
		if rawDefs, ok := opts["register_defs"]; ok {
			if defs, ok := rawDefs.([]any); ok {
				topDefs = defs
			}
		}
		cfg.Devices = []DeviceConfig{
			{
				UnitID:       cfg.UnitID,
				Name:         "",
				RegisterMap:  rmCfg,
				RegisterDefs: topDefs,
			},
		}
	} else {
		// register_map 도 devices 도 없는 경우
		return ModbusServerConfig{}, fmt.Errorf(
			"modbus-server: register_map or devices is required: %w", ErrInvalidRegisterMap)
	}

	// Devices 유효성 검증
	if err := validateDevices(cfg.Devices); err != nil {
		return ModbusServerConfig{}, err
	}

	return cfg, nil
}

// parseDevicesConfig 는 devices 배열을 파싱하여 []DeviceConfig 를 반환한다.
func parseDevicesConfig(raw any) ([]DeviceConfig, error) {
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("modbus-server: devices must be an array: %w", ErrInvalidDeviceConfig)
	}
	if len(arr) == 0 {
		return nil, fmt.Errorf("modbus-server: devices must have at least one device: %w", ErrInvalidDeviceConfig)
	}

	devices := make([]DeviceConfig, 0, len(arr))
	for i, item := range arr {
		devMap, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("modbus-server: devices[%d] must be a map: %w", i, ErrInvalidDeviceConfig)
		}

		var dev DeviceConfig

		// unit_id (필수, 1-247)
		if v, ok := devMap["unit_id"]; ok {
			id := toInt(v)
			if id < 1 || id > 247 {
				return nil, fmt.Errorf(
					"modbus-server: devices[%d].unit_id must be 1-247 (got %d)", i, id)
			}
			dev.UnitID = byte(id)
		} else {
			return nil, fmt.Errorf(
				"modbus-server: devices[%d].unit_id is required", i)
		}

		// name (선택)
		if v, ok := devMap["name"]; ok {
			if s, ok := v.(string); ok {
				dev.Name = s
			}
		}

		// register_map (필수)
		rmRaw, ok := devMap["register_map"]
		if !ok {
			return nil, fmt.Errorf(
				"modbus-server: devices[%d].register_map is required: %w", i, ErrInvalidRegisterMap)
		}
		rmMap, ok := rmRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(
				"modbus-server: devices[%d].register_map must be a map: %w", i, ErrInvalidRegisterMap)
		}
		rmCfg, err := parseRegisterMapConfig(rmMap)
		if err != nil {
			return nil, fmt.Errorf("modbus-server: devices[%d]: %w", i, err)
		}
		dev.RegisterMap = rmCfg

		// register_defs (선택: 디바이스별 레지스터 정의)
		if rawDefs, ok := devMap["register_defs"]; ok {
			if defs, ok := rawDefs.([]any); ok {
				dev.RegisterDefs = defs
			}
		}

		devices = append(devices, dev)
	}

	return devices, nil
}

// validateDevices 는 디바이스 목록의 유효성을 검증한다.
// - 최소 1개 디바이스 필요
// - Unit ID 범위: 1-247
// - Unit ID 중복 불가
func validateDevices(devices []DeviceConfig) error {
	if len(devices) == 0 {
		return fmt.Errorf("modbus-server: at least one device is required: %w", ErrInvalidDeviceConfig)
	}

	seen := make(map[byte]bool, len(devices))
	for i, dev := range devices {
		if dev.UnitID < 1 || dev.UnitID > 247 {
			return fmt.Errorf(
				"modbus-server: devices[%d].unit_id must be 1-247 (got %d)", i, dev.UnitID)
		}
		if seen[dev.UnitID] {
			return fmt.Errorf(
				"modbus-server: duplicate unit_id %d in devices: %w", dev.UnitID, ErrDuplicateUnitID)
		}
		seen[dev.UnitID] = true
	}

	return nil
}

// parseRegisterMapConfig 는 레지스터 맵 설정 맵을 RegisterMapConfig 로 파싱한다.
func parseRegisterMapConfig(m map[string]any) (RegisterMapConfig, error) {
	var cfg RegisterMapConfig
	hasArea := false

	// coils
	if v, ok := m["coils"]; ok {
		areas, err := parseAreaSegments(v, "coils")
		if err != nil {
			return RegisterMapConfig{}, err
		}
		cfg.Coils = areas
		hasArea = true
	}

	// discrete_inputs
	if v, ok := m["discrete_inputs"]; ok {
		areas, err := parseAreaSegments(v, "discrete_inputs")
		if err != nil {
			return RegisterMapConfig{}, err
		}
		cfg.DiscreteInputs = areas
		hasArea = true
	}

	// holding_registers
	if v, ok := m["holding_registers"]; ok {
		areas, err := parseAreaSegments(v, "holding_registers")
		if err != nil {
			return RegisterMapConfig{}, err
		}
		cfg.HoldingRegisters = areas
		hasArea = true
	}

	// input_registers
	if v, ok := m["input_registers"]; ok {
		areas, err := parseAreaSegments(v, "input_registers")
		if err != nil {
			return RegisterMapConfig{}, err
		}
		cfg.InputRegisters = areas
		hasArea = true
	}

	// 최소 하나의 영역이 있어야 한다
	if !hasArea {
		return RegisterMapConfig{}, fmt.Errorf(
			"modbus-server: register_map must have at least one area: %w", ErrInvalidRegisterMap)
	}

	return cfg, nil
}

// parseAreaSegments 는 단일 맵(하위 호환) 또는 배열(다중 세그먼트) 형식을 파싱한다.
// 단일 맵: {"start_address": 0, "count": 100}
// 다중 세그먼트: [{"start_address": 0, "count": 100}, {"start_address": 200, "count": 100}]
func parseAreaSegments(v any, areaName string) ([]*RegisterAreaConfig, error) {
	switch val := v.(type) {
	case map[string]any:
		// 단일 블록 (하위 호환)
		area, err := parseRegisterAreaConfig(val, areaName)
		if err != nil {
			return nil, err
		}
		return []*RegisterAreaConfig{&area}, nil

	case []any:
		// 다중 세그먼트
		if len(val) == 0 {
			return nil, fmt.Errorf("modbus-server: register_map.%s must have at least one segment", areaName)
		}
		areas := make([]*RegisterAreaConfig, 0, len(val))
		for i, item := range val {
			areaMap, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("modbus-server: register_map.%s[%d] must be a map", areaName, i)
			}
			segName := fmt.Sprintf("%s[%d]", areaName, i)
			area, err := parseRegisterAreaConfig(areaMap, segName)
			if err != nil {
				return nil, err
			}
			areas = append(areas, &area)
		}
		// 세그먼트 간 겹침 검증
		if err := validateSegmentOverlap(areas, areaName); err != nil {
			return nil, err
		}
		return areas, nil

	default:
		return nil, fmt.Errorf("modbus-server: register_map.%s must be a map or array", areaName)
	}
}

// validateSegmentOverlap 는 세그먼트 간 주소 범위 겹침을 검증한다.
func validateSegmentOverlap(segments []*RegisterAreaConfig, areaName string) error {
	for i := 0; i < len(segments); i++ {
		iStart := segments[i].StartAddress
		iEnd := iStart + segments[i].Count
		for j := i + 1; j < len(segments); j++ {
			jStart := segments[j].StartAddress
			jEnd := jStart + segments[j].Count
			if iStart < jEnd && jStart < iEnd {
				return fmt.Errorf(
					"modbus-server: register_map.%s segments overlap: [%d,%d) and [%d,%d)",
					areaName, iStart, iEnd, jStart, jEnd)
			}
		}
	}
	return nil
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

	// data_type (선택, 기본값 "uint16")
	if v, ok := m["data_type"]; ok {
		if s, ok := v.(string); ok {
			if !modbus.IsValidDataType(s) {
				return RegisterAreaConfig{}, fmt.Errorf(
					"modbus-server: register_map.%s.data_type is not supported: %q", areaName, s)
			}
			area.DataType = s
		}
	}

	// type_map (선택)
	if v, ok := m["type_map"]; ok {
		if entries, ok := v.([]any); ok {
			typeMap, err := parseTypeMap(entries, areaName)
			if err != nil {
				return RegisterAreaConfig{}, err
			}
			area.TypeMap = typeMap
		}
	}

	// type_map 검증
	if len(area.TypeMap) > 0 {
		if err := validateTypeMap(area.TypeMap, area.StartAddress, area.Count, areaName); err != nil {
			return RegisterAreaConfig{}, err
		}
	}

	return area, nil
}

// parseTypeMap은 []any 로부터 TypeMapEntry 슬라이스를 파싱한다.
func parseTypeMap(entries []any, areaName string) ([]modbus.TypeMapEntry, error) {
	result := make([]modbus.TypeMapEntry, 0, len(entries))
	for i, entry := range entries {
		m, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(
				"modbus-server: register_map.%s.type_map[%d] must be a map", areaName, i)
		}

		var tme modbus.TypeMapEntry

		// address (필수)
		if v, ok := m["address"]; ok {
			tme.Address = toUint16(v)
		} else {
			return nil, fmt.Errorf(
				"modbus-server: register_map.%s.type_map[%d].address is required", areaName, i)
		}

		// data_type (필수)
		if v, ok := m["data_type"]; ok {
			if s, ok := v.(string); ok {
				if !modbus.IsValidDataType(s) {
					return nil, fmt.Errorf(
						"modbus-server: register_map.%s.type_map[%d].data_type is not supported: %q", areaName, i, s)
				}
				tme.DataType = s
			}
		} else {
			return nil, fmt.Errorf(
				"modbus-server: register_map.%s.type_map[%d].data_type is required", areaName, i)
		}

		// byte_order (선택, 기본값 "big_endian")
		tme.ByteOrder = modbus.ByteOrderBigEndian
		if v, ok := m["byte_order"]; ok {
			if s, ok := v.(string); ok {
				tme.ByteOrder = s
			}
		}
		if tme.ByteOrder != modbus.ByteOrderBigEndian && tme.ByteOrder != modbus.ByteOrderLittleEndian {
			return nil, fmt.Errorf(
				"modbus-server: register_map.%s.type_map[%d].byte_order must be %q or %q (got %q)",
				areaName, i, modbus.ByteOrderBigEndian, modbus.ByteOrderLittleEndian, tme.ByteOrder)
		}

		result = append(result, tme)
	}
	return result, nil
}

// validateTypeMap은 type_map의 겹침 검사 및 범위 검사를 수행한다.
func validateTypeMap(typeMap []modbus.TypeMapEntry, startAddr, count uint16, areaName string) error {
	endAddr := startAddr + count

	// 겹침 감지를 위한 점유 범위 목록
	type addrRange struct {
		start uint16
		end   uint16 // exclusive
	}
	occupied := make([]addrRange, 0, len(typeMap))

	for i, entry := range typeMap {
		regCount, err := modbus.RegisterCountForType(entry.DataType)
		if err != nil {
			return fmt.Errorf("modbus-server: register_map.%s.type_map[%d]: %w", areaName, i, err)
		}

		entryEnd := entry.Address + regCount

		// 범위 검사: [startAddr, startAddr+count) 안에 있어야 한다
		if entry.Address < startAddr || entryEnd > endAddr {
			return fmt.Errorf(
				"modbus-server: register_map.%s.type_map address %d (type %s, %d regs) exceeds range [%d, %d): %w",
				areaName, entry.Address, entry.DataType, regCount, startAddr, endAddr, ErrTypeMapOutOfRange)
		}

		// 이전 엔트리와의 겹침 검사
		for j, prev := range occupied {
			if entry.Address < prev.end && entryEnd > prev.start {
				return fmt.Errorf(
					"modbus-server: register_map.%s.type_map[%d] (addr %d) overlaps with type_map[%d] (addr %d-%d): %w",
					areaName, i, entry.Address, j, prev.start, prev.end-1, ErrTypeMapOverlap)
			}
		}

		occupied = append(occupied, addrRange{start: entry.Address, end: entryEnd})
	}

	return nil
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
