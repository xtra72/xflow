package modbus

import (
	"fmt"
	"time"
)

// ModbusConfig 는 MODBUS/TCP 에이전트의 설정을 나타낸다.
type ModbusConfig struct {
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
	FunctionCode byte   // 1, 2, 3, 4
	StartAddress uint16
	Quantity     uint16
}

// parseModbusConfig 는 Transport.Options 맵에서 ModbusConfig 를 파싱한다.
func parseModbusConfig(opts map[string]any) (ModbusConfig, error) {
	cfg := ModbusConfig{
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

	return rg, nil
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
