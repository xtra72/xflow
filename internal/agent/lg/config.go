package lg

import (
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// LGAPConfig 는 LG LGAP HVAC 에이전트의 설정을 나타낸다.
type LGAPConfig struct {
	SerialPort          string
	BaudRate            int
	DataBits            int
	StopBits            int
	Parity              string
	ReadTimeout         time.Duration
	PollInterval        time.Duration
	InterCommandDelay   time.Duration
	Devices             []agent.DeviceEntry
	MsgChannelSize      int
	OfflineThreshold    int
	ReconnectInterval   time.Duration
	MaxReconnectBackoff time.Duration
}

// parseLGAPConfig 는 Transport.Options 맵에서 LGAPConfig 를 파싱한다.
func parseLGAPConfig(opts map[string]any) (LGAPConfig, error) {
	cfg := LGAPConfig{
		BaudRate:            4800,
		DataBits:            8,
		StopBits:            1,
		Parity:              "none",
		ReadTimeout:         500 * time.Millisecond,
		PollInterval:        30 * time.Second,
		InterCommandDelay:   50 * time.Millisecond,
		MsgChannelSize:      256,
		OfflineThreshold:    3,
		ReconnectInterval:   5 * time.Second,
		MaxReconnectBackoff: 5 * time.Minute,
	}

	// serial_port (필수)
	if v, ok := opts["serial_port"]; ok {
		cfg.SerialPort = v.(string)
	}
	if cfg.SerialPort == "" {
		return LGAPConfig{}, fmt.Errorf("lgap: serial_port is required")
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

	// read_timeout
	if v, ok := opts["read_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGAPConfig{}, fmt.Errorf("lgap: invalid read_timeout: %w", err)
		}
		cfg.ReadTimeout = d
	}

	// poll_interval
	if v, ok := opts["poll_interval"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGAPConfig{}, fmt.Errorf("lgap: invalid poll_interval: %w", err)
		}
		cfg.PollInterval = d
	}

	// inter_command_delay
	if v, ok := opts["inter_command_delay"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGAPConfig{}, fmt.Errorf("lgap: invalid inter_command_delay: %w", err)
		}
		cfg.InterCommandDelay = d
	}

	// devices (선택)
	cfg.Devices = agent.ParseDevices(opts)

	// msg_channel_size
	if v, ok := opts["msg_channel_size"]; ok {
		cfg.MsgChannelSize = toInt(v)
	}

	// offline_threshold
	if v, ok := opts["offline_threshold"]; ok {
		cfg.OfflineThreshold = toInt(v)
	}

	// reconnect_interval
	if v, ok := opts["reconnect_interval"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGAPConfig{}, fmt.Errorf("lgap: invalid reconnect_interval: %w", err)
		}
		cfg.ReconnectInterval = d
	}

	// max_reconnect_backoff
	if v, ok := opts["max_reconnect_backoff"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGAPConfig{}, fmt.Errorf("lgap: invalid max_reconnect_backoff: %w", err)
		}
		cfg.MaxReconnectBackoff = d
	}

	return cfg, nil
}

// parseZoneKey 는 존 키 문자열을 정수값으로 변환한다.
// "0x10" (16진수) 또는 "16" (10진수) 형식을 모두 지원한다.
func parseZoneKey(key string) any {
	// 16진수 접두사 처리
	if len(key) > 2 && (key[:2] == "0x" || key[:2] == "0X") {
		var val int
		_, err := fmt.Sscanf(key, "0x%x", &val)
		if err != nil {
			_, _ = fmt.Sscanf(key, "0X%x", &val)
		}
		return val
	}
	// 10진수 처리
	var val int
	_, _ = fmt.Sscanf(key, "%d", &val)
	return val
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
