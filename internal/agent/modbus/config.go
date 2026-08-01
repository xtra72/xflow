package modbus

import (
	"fmt"
	"time"

	modbus "github.com/xtra/xflow/internal/modbus"
)

// 트랜스포트 디스크리미네이터 상수.
const (
	// TransportTCP 는 MODBUS/TCP(MBAP) 트랜스포트이다(기본값).
	TransportTCP = "tcp"
	// TransportRTU 는 MODBUS RTU(시리얼, CRC 프레이밍) 트랜스포트이다.
	TransportRTU = "rtu"
)

// ModbusConfig 는 MODBUS 클라이언트 에이전트의 설정을 나타낸다.
type ModbusConfig struct {
	Transport         string        // "tcp" | "rtu" (기본값 "tcp", 생략 시 하위 호환)
	Serial            SerialConfig  // Transport == "rtu" 일 때만 유효한 시리얼 파라미터
	Mode              string        // "interval" | "event"
	ReadMode          string        // "direct" | "cached"
	PollInterval      time.Duration // 기본값 5s
	HeartbeatInterval time.Duration // 기본값 60s (event 모드)
	StaleThreshold    time.Duration // 기본값 PollInterval*3
	WriteTimeout      time.Duration // 기본값 5s
	EnableWriteEvents bool          // 기본값 true
	ReconnectInterval time.Duration // 기본값 10s
	MaxRetries        int           // 기본값 3
	RequestTimeout    time.Duration // 기본값 3s
	MsgChannelSize    int           // 기본값 256
	Devices           []DeviceConfig
}

// SerialConfig 는 RTU 트랜스포트의 시리얼 포트 파라미터이다(A-10).
// transport == "rtu" 일 때 Transport.Options 에서 파싱·검증된다.
type SerialConfig struct {
	Port     string // 시리얼 포트 경로 (필수, 예: /dev/ttyUSB0)
	BaudRate int    // 기본값 9600
	DataBits int    // 기본값 8
	StopBits int    // 기본값 1 (1 또는 2)
	Parity   string // "none" | "even" | "odd" (기본값 "none")
}

// DeviceConfig 는 단일 MODBUS 디바이스의 설정을 나타낸다.
type DeviceConfig struct {
	ID             string
	Host           string
	Port           int // 기본값 502
	UnitID         byte
	RegisterGroups []RegisterGroupConfig
}

// RegisterGroupConfig 는 레지스터 그룹의 설정을 나타낸다.
type RegisterGroupConfig struct {
	Name         string
	FunctionCode byte // 1, 2, 3, 4
	StartAddress uint16
	Quantity     uint16
	DataType     string                // 그룹 기본 데이터 타입 (기본: "uint16")
	TypeMap      []modbus.TypeMapEntry // 주소별 타입 오버라이드 (선택)
}

// parseModbusConfig 는 Transport.Options 맵에서 ModbusConfig 를 파싱한다.
func parseModbusConfig(opts map[string]any) (ModbusConfig, error) {
	cfg := ModbusConfig{
		Transport:         TransportTCP,
		Mode:              "interval",
		ReadMode:          "cached",
		PollInterval:      5 * time.Second,
		HeartbeatInterval: 60 * time.Second,
		WriteTimeout:      5 * time.Second,
		EnableWriteEvents: true,
		ReconnectInterval: 10 * time.Second,
		MaxRetries:        3,
		RequestTimeout:    3 * time.Second,
		MsgChannelSize:    256,
	}

	// transport (선택, 기본 "tcp" — 생략 시 기존 TCP 동작 보존, AC-03)
	if v, ok := opts["transport"]; ok {
		s, _ := v.(string)
		switch s {
		case TransportTCP, "":
			cfg.Transport = TransportTCP
		case TransportRTU:
			cfg.Transport = TransportRTU
		default:
			return ModbusConfig{}, fmt.Errorf("modbus: transport %q: %w", s, ErrInvalidTransport)
		}
	}

	// RTU 시리얼 파라미터 (transport == "rtu" 일 때 파싱·검증, A-10)
	if cfg.Transport == TransportRTU {
		sc, err := parseSerialConfig(opts)
		if err != nil {
			return ModbusConfig{}, err
		}
		cfg.Serial = sc
	}

	// mode
	if v, ok := opts["mode"]; ok {
		s, _ := v.(string)
		if s != "interval" && s != "event" {
			return ModbusConfig{}, fmt.Errorf("modbus: invalid mode %q (must be \"interval\" or \"event\")", s)
		}
		cfg.Mode = s
	}

	// read_mode
	if v, ok := opts["read_mode"]; ok {
		s, _ := v.(string)
		if s != "direct" && s != "cached" {
			return ModbusConfig{}, fmt.Errorf("modbus: invalid read_mode %q (must be \"direct\" or \"cached\")", s)
		}
		cfg.ReadMode = s
	}

	// poll_interval
	if v, ok := opts["poll_interval"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return ModbusConfig{}, fmt.Errorf("modbus: invalid poll_interval: %w", err)
		}
		cfg.PollInterval = d
	}

	// heartbeat_interval
	if v, ok := opts["heartbeat_interval"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return ModbusConfig{}, fmt.Errorf("modbus: invalid heartbeat_interval: %w", err)
		}
		cfg.HeartbeatInterval = d
	}

	// stale_threshold
	staleSet := false
	if v, ok := opts["stale_threshold"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return ModbusConfig{}, fmt.Errorf("modbus: invalid stale_threshold: %w", err)
		}
		cfg.StaleThreshold = d
		staleSet = true
	}

	// write_timeout
	if v, ok := opts["write_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return ModbusConfig{}, fmt.Errorf("modbus: invalid write_timeout: %w", err)
		}
		cfg.WriteTimeout = d
	}

	// enable_write_events
	if v, ok := opts["enable_write_events"]; ok {
		cfg.EnableWriteEvents = v.(bool)
	}

	// reconnect_interval
	if v, ok := opts["reconnect_interval"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return ModbusConfig{}, fmt.Errorf("modbus: invalid reconnect_interval: %w", err)
		}
		cfg.ReconnectInterval = d
	}

	// max_retries
	if v, ok := opts["max_retries"]; ok {
		cfg.MaxRetries = toInt(v)
	}

	// request_timeout
	if v, ok := opts["request_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return ModbusConfig{}, fmt.Errorf("modbus: invalid request_timeout: %w", err)
		}
		cfg.RequestTimeout = d
	}

	// msg_channel_size
	if v, ok := opts["msg_channel_size"]; ok {
		cfg.MsgChannelSize = toInt(v)
	}

	// devices (필수)
	if v, ok := opts["devices"]; ok {
		switch devList := v.(type) {
		case []any:
			for i, raw := range devList {
				devMap, ok := raw.(map[string]any)
				if !ok {
					return ModbusConfig{}, fmt.Errorf("modbus: devices[%d] is not a map", i)
				}
				dc, err := parseDeviceConfig(devMap, i)
				if err != nil {
					return ModbusConfig{}, err
				}
				cfg.Devices = append(cfg.Devices, dc)
			}
		}
	}

	if len(cfg.Devices) == 0 {
		return ModbusConfig{}, fmt.Errorf("modbus: devices is required and must not be empty")
	}

	// stale_threshold 기본값: PollInterval * 3
	if !staleSet {
		cfg.StaleThreshold = cfg.PollInterval * 3
	}

	return cfg, nil
}

// parseSerialConfig 는 Transport.Options 에서 RTU 시리얼 파라미터를 파싱·검증한다(A-10).
// serial_port(또는 port)는 필수이며, 나머지는 관례적 기본값을 가진다.
// 검증은 파싱 단계에서 수행되며, 무효 값은 설정 오류로 거부한다.
func parseSerialConfig(opts map[string]any) (SerialConfig, error) {
	sc := SerialConfig{
		BaudRate: 9600,
		DataBits: 8,
		StopBits: 1,
		Parity:   "none",
	}

	// serial_port / port (필수)
	if v, ok := opts["serial_port"]; ok {
		sc.Port, _ = v.(string)
	} else if v, ok := opts["port"]; ok {
		sc.Port, _ = v.(string)
	}
	if sc.Port == "" {
		return SerialConfig{}, ErrMissingSerialPort
	}

	// baud_rate (기본 9600, > 0)
	if v, ok := opts["baud_rate"]; ok {
		sc.BaudRate = toInt(v)
	}
	if sc.BaudRate <= 0 {
		return SerialConfig{}, fmt.Errorf("modbus: baud_rate must be > 0 (got %d): %w", sc.BaudRate, ErrInvalidSerialParam)
	}

	// data_bits (기본 8, 5-8)
	if v, ok := opts["data_bits"]; ok {
		sc.DataBits = toInt(v)
	}
	if sc.DataBits < 5 || sc.DataBits > 8 {
		return SerialConfig{}, fmt.Errorf("modbus: data_bits must be 5-8 (got %d): %w", sc.DataBits, ErrInvalidSerialParam)
	}

	// stop_bits (기본 1, 1 또는 2)
	if v, ok := opts["stop_bits"]; ok {
		sc.StopBits = toInt(v)
	}
	if sc.StopBits != 1 && sc.StopBits != 2 {
		return SerialConfig{}, fmt.Errorf("modbus: stop_bits must be 1 or 2 (got %d): %w", sc.StopBits, ErrInvalidSerialParam)
	}

	// parity (기본 "none", none|even|odd)
	if v, ok := opts["parity"]; ok {
		if s, ok := v.(string); ok && s != "" {
			sc.Parity = s
		}
	}
	switch sc.Parity {
	case "none", "even", "odd":
	default:
		return SerialConfig{}, fmt.Errorf("modbus: parity %q must be none|even|odd: %w", sc.Parity, ErrInvalidSerialParam)
	}

	return sc, nil
}

// parseDeviceConfig 는 디바이스 설정 맵을 DeviceConfig 로 파싱한다.
func parseDeviceConfig(m map[string]any, idx int) (DeviceConfig, error) {
	dc := DeviceConfig{
		Port:   502,
		UnitID: 1,
	}

	// id
	if v, ok := m["id"]; ok {
		dc.ID, _ = v.(string)
	}

	// host (필수)
	if v, ok := m["host"]; ok {
		dc.Host, _ = v.(string)
	}
	if dc.Host == "" {
		return DeviceConfig{}, fmt.Errorf("modbus: devices[%d].host is required", idx)
	}

	// port
	if v, ok := m["port"]; ok {
		dc.Port = toInt(v)
	}

	// unit_id
	if v, ok := m["unit_id"]; ok {
		dc.UnitID = toByte(v)
	}

	// register_groups
	if v, ok := m["register_groups"]; ok {
		switch rgList := v.(type) {
		case []any:
			for j, raw := range rgList {
				rgMap, ok := raw.(map[string]any)
				if !ok {
					return DeviceConfig{}, fmt.Errorf("modbus: devices[%d].register_groups[%d] is not a map", idx, j)
				}
				rg, err := parseRegisterGroupConfig(rgMap, idx, j)
				if err != nil {
					return DeviceConfig{}, err
				}
				dc.RegisterGroups = append(dc.RegisterGroups, rg)
			}
		}
	}

	return dc, nil
}

// parseRegisterGroupConfig 는 레지스터 그룹 설정 맵을 RegisterGroupConfig 로 파싱한다.
func parseRegisterGroupConfig(m map[string]any, devIdx, rgIdx int) (RegisterGroupConfig, error) {
	rg := RegisterGroupConfig{}

	// name
	if v, ok := m["name"]; ok {
		rg.Name, _ = v.(string)
	}

	// function_code (필수, 1-4 만 유효)
	if v, ok := m["function_code"]; ok {
		rg.FunctionCode = toByte(v)
	}
	if rg.FunctionCode < 1 || rg.FunctionCode > 4 {
		return RegisterGroupConfig{}, fmt.Errorf(
			"modbus: devices[%d].register_groups[%d].function_code must be 1, 2, 3, or 4 (got %d)",
			devIdx, rgIdx, rg.FunctionCode,
		)
	}

	// start_address
	if v, ok := m["start_address"]; ok {
		rg.StartAddress = toUint16(v)
	}

	// quantity (필수, > 0)
	if v, ok := m["quantity"]; ok {
		rg.Quantity = toUint16(v)
	}
	if rg.Quantity == 0 {
		return RegisterGroupConfig{}, fmt.Errorf(
			"modbus: devices[%d].register_groups[%d].quantity must be > 0",
			devIdx, rgIdx,
		)
	}

	// data_type (선택, 기본값 "uint16")
	if v, ok := m["data_type"]; ok {
		if s, ok := v.(string); ok {
			if !modbus.IsValidDataType(s) {
				return RegisterGroupConfig{}, fmt.Errorf(
					"modbus: devices[%d].register_groups[%d].data_type is not supported: %q: %w",
					devIdx, rgIdx, s, ErrUnsupportedDataType)
			}
			rg.DataType = s
		}
	}

	// type_map (선택)
	if v, ok := m["type_map"]; ok {
		if entries, ok := v.([]any); ok {
			prefix := fmt.Sprintf("devices[%d].register_groups[%d]", devIdx, rgIdx)
			typeMap, err := parseClientTypeMap(entries, prefix)
			if err != nil {
				return RegisterGroupConfig{}, err
			}
			rg.TypeMap = typeMap
		}
	}

	// type_map 검증
	if len(rg.TypeMap) > 0 {
		prefix := fmt.Sprintf("devices[%d].register_groups[%d]", devIdx, rgIdx)
		if err := validateClientTypeMap(rg.TypeMap, rg.StartAddress, rg.Quantity, prefix); err != nil {
			return RegisterGroupConfig{}, err
		}
	}

	return rg, nil
}

// parseClientTypeMap 은 []any 로부터 TypeMapEntry 슬라이스를 파싱한다.
func parseClientTypeMap(entries []any, prefix string) ([]modbus.TypeMapEntry, error) {
	result := make([]modbus.TypeMapEntry, 0, len(entries))
	for i, entry := range entries {
		m, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(
				"modbus: %s.type_map[%d] must be a map", prefix, i)
		}

		var tme modbus.TypeMapEntry

		// address (필수)
		if v, ok := m["address"]; ok {
			tme.Address = toUint16(v)
		} else {
			return nil, fmt.Errorf(
				"modbus: %s.type_map[%d].address is required", prefix, i)
		}

		// data_type (필수)
		if v, ok := m["data_type"]; ok {
			if s, ok := v.(string); ok {
				if !modbus.IsValidDataType(s) {
					return nil, fmt.Errorf(
						"modbus: %s.type_map[%d].data_type is not supported: %q: %w",
						prefix, i, s, ErrUnsupportedDataType)
				}
				tme.DataType = s
			}
		} else {
			return nil, fmt.Errorf(
				"modbus: %s.type_map[%d].data_type is required", prefix, i)
		}

		// byte_order (선택, 기본값 "big_endian")
		tme.ByteOrder = modbus.ByteOrderBigEndian
		if v, ok := m["byte_order"]; ok {
			if s, ok := v.(string); ok {
				tme.ByteOrder = s
			}
		}

		result = append(result, tme)
	}
	return result, nil
}

// validateClientTypeMap 은 type_map 의 겹침 검사 및 범위 검사를 수행한다.
func validateClientTypeMap(typeMap []modbus.TypeMapEntry, startAddr, count uint16, prefix string) error {
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
			return fmt.Errorf("modbus: %s.type_map[%d]: %w", prefix, i, err)
		}

		entryEnd := entry.Address + regCount

		// 범위 검사: [startAddr, startAddr+count) 안에 있어야 한다
		if entry.Address < startAddr || entryEnd > endAddr {
			return fmt.Errorf(
				"modbus: %s.type_map address %d (type %s, %d regs) exceeds range [%d, %d): %w",
				prefix, entry.Address, entry.DataType, regCount, startAddr, endAddr, ErrTypeMapOutOfRange)
		}

		// 이전 엔트리와의 겹침 검사
		for j, prev := range occupied {
			if entry.Address < prev.end && entryEnd > prev.start {
				return fmt.Errorf(
					"modbus: %s.type_map[%d] (addr %d) overlaps with type_map[%d] (addr %d-%d): %w",
					prefix, i, entry.Address, j, prev.start, prev.end-1, ErrTypeMapOverlap)
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
