package samsung

import (
	"fmt"
	"time"
)

// NASAConfig 는 Samsung NASA HVAC 에이전트의 설정을 나타낸다.
type NASAConfig struct {
	TransportType    string
	SerialPort       string
	BaudRate         int
	DataBits         int
	StopBits         int
	Parity           string
	TCPAddr          string
	ConnectTimeout   time.Duration
	ReadTimeout      time.Duration
	PollInterval     time.Duration
	NotifyInterval   time.Duration
	DeviceAddresses  []string
	DeviceIDs        map[string]string
	ProtocolFile     string
	AutoDiscovery    bool
	RegistryPath     string
	OfflineThreshold   int
	MsgChannelSize     int
	UnsupportedMsgSets    map[uint16]bool // 필터링할 메시지 셋 인덱스
	LogUnsupportedMsgSets bool             // 필터링 시 로그 출력 여부
	IncludeRawMessageSets bool             // 상태 조회 시 RawMessageSets 포함 여부
}

// parseNASAConfig 는 Transport.Options 맵에서 NASAConfig 를 파싱한다.
func parseNASAConfig(opts map[string]any) (NASAConfig, error) {
	cfg := NASAConfig{
		BaudRate:         9600,
		DataBits:         8,
		StopBits:         1,
		Parity:           "even",
		ConnectTimeout:   5 * time.Second,
		ReadTimeout:      3 * time.Second,
		PollInterval:     30 * time.Second,
		NotifyInterval:   0,
		OfflineThreshold: 3,
		MsgChannelSize:   256,
	}

	// transport_type (필수)
	if v, ok := opts["transport_type"]; ok {
		cfg.TransportType = v.(string)
	}
	if cfg.TransportType == "" {
		return NASAConfig{}, fmt.Errorf("samsung-nasa: transport_type is required")
	}

	// serial_port
	if v, ok := opts["serial_port"]; ok {
		cfg.SerialPort = v.(string)
	}

	// baud_rate
	if v, ok := opts["baud_rate"]; ok {
		cfg.BaudRate = toInt(v)
	}

	// data_bits
	if v, ok := opts["data_bits"]; ok {
		cfg.DataBits = toInt(v)
	}

	// stop_bits
	if v, ok := opts["stop_bits"]; ok {
		cfg.StopBits = toInt(v)
	}

	// parity
	if v, ok := opts["parity"]; ok {
		cfg.Parity = v.(string)
	}

	// tcp_address
	if v, ok := opts["tcp_address"]; ok {
		cfg.TCPAddr = v.(string)
	}

	// connect_timeout
	if v, ok := opts["connect_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return NASAConfig{}, fmt.Errorf("samsung-nasa: invalid connect_timeout: %w", err)
		}
		cfg.ConnectTimeout = d
	}

	// read_timeout
	if v, ok := opts["read_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return NASAConfig{}, fmt.Errorf("samsung-nasa: invalid read_timeout: %w", err)
		}
		cfg.ReadTimeout = d
	}

	// poll_interval
	if v, ok := opts["poll_interval"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return NASAConfig{}, fmt.Errorf("samsung-nasa: invalid poll_interval: %w", err)
		}
		cfg.PollInterval = d
	}

	// notify_interval
	if v, ok := opts["notify_interval"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return NASAConfig{}, fmt.Errorf("samsung-nasa: invalid notify_interval: %w", err)
		}
		cfg.NotifyInterval = d
	}

	// device_addresses (필수)
	if v, ok := opts["device_addresses"]; ok {
		switch addrs := v.(type) {
		case []any:
			for _, a := range addrs {
				cfg.DeviceAddresses = append(cfg.DeviceAddresses, a.(string))
			}
		case []string:
			cfg.DeviceAddresses = addrs
		}
	}
	if len(cfg.DeviceAddresses) == 0 {
		return NASAConfig{}, fmt.Errorf("samsung-nasa: device_addresses is required")
	}

	// device_ids
	if v, ok := opts["device_ids"]; ok {
		switch ids := v.(type) {
		case map[string]any:
			cfg.DeviceIDs = make(map[string]string, len(ids))
			for k, val := range ids {
				cfg.DeviceIDs[k] = val.(string)
			}
		case map[string]string:
			cfg.DeviceIDs = ids
		}
	}

	// protocol_file
	if v, ok := opts["protocol_file"]; ok {
		cfg.ProtocolFile = v.(string)
	}

	// auto_discovery
	if v, ok := opts["auto_discovery"]; ok {
		cfg.AutoDiscovery = v.(bool)
	}

	// registry_path
	if v, ok := opts["registry_path"]; ok {
		cfg.RegistryPath = v.(string)
	}

	// offline_threshold
	if v, ok := opts["offline_threshold"]; ok {
		cfg.OfflineThreshold = toInt(v)
	}

	// msg_channel_size
	if v, ok := opts["msg_channel_size"]; ok {
		cfg.MsgChannelSize = toInt(v)
	}

	// unsupported_msg_sets
	if v, ok := opts["unsupported_msg_sets"]; ok {
		if sets, ok := v.([]any); ok && len(sets) > 0 {
			cfg.UnsupportedMsgSets = make(map[uint16]bool, len(sets))
			for _, s := range sets {
				if idx := toInt(s); idx > 0 {
					cfg.UnsupportedMsgSets[uint16(idx)] = true
				}
			}
		}
	}

	// log_unsupported_msg_sets (기본값: false)
	if v, ok := opts["log_unsupported_msg_sets"]; ok {
		if b, ok := v.(bool); ok {
			cfg.LogUnsupportedMsgSets = b
		}
	}

	// include_raw_message_sets (기본값: true)
	cfg.IncludeRawMessageSets = true
	if v, ok := opts["include_raw_message_sets"]; ok {
		if b, ok := v.(bool); ok {
			cfg.IncludeRawMessageSets = b
		}
	}

	return cfg, nil
}

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
