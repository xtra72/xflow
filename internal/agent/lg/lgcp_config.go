package lg

import (
	"fmt"
	"regexp"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// reHex8 는 8자리 16진수 문자열을 검증하는 정규식이다.
var reHex8 = regexp.MustCompile(`^[0-9a-fA-F]{8}$`)

// LGCPConfig 는 LGCP 패킷 캡처 에이전트의 설정이다.
type LGCPConfig struct {
	SerialPort          string        // 시리얼 포트 경로 (필수)
	BaudRate            int           // 보레이트 (기본: 9600)
	DataBits            int           // 데이터 비트 (기본: 8)
	StopBits            int           // 스톱 비트 (기본: 1)
	Parity              string        // 패리티 (기본: "none")
	ReadTimeout         time.Duration // 읽기 타임아웃 (기본: 500ms)
	MsgChannelSize      int           // 메시지 채널 버퍼 크기 (기본: 256)
	ReconnectInterval   time.Duration // 재연결 시도 간격 (기본: 5s)
	MaxReconnectBackoff time.Duration // 최대 재연결 백오프 (기본: 5m)
	VerifyCRC           bool          // CRC 검증 활성화 (기본: true)

	// 디바이스 관리
	AutoDiscovery   bool              // 자동 디바이스 발견 (기본: true)
	NotifyInterval  time.Duration     // 주기적 상태 보고 간격 (기본: 0 = 변경 시에만)
	OfflineTimeout  time.Duration     // 통신 없음 → 오프라인 판정 (기본: 30s)
	Devices         []agent.DeviceEntry // 설정 기반 디바이스 목록

	// 제어 기능
	ControllerAddress   string        // 컨트롤러 SA 주소, 8자리 HEX (기본: "44550000")
	ControlVerifyTimeout time.Duration // 제어 후 검증 타임아웃 (기본: 3s)
	ControlEnabled      bool          // 제어 기능 토글 (기본: false)

	// 트랜스포트 타입 선택
	TransportType      string        // "serial", "tcp-client", "tcp-server" (기본: "serial")

	// TCP 트랜스포트 설정
	TCPHost            string        // TCP 호스트 주소 (기본: "0.0.0.0")
	TCPPort            int           // TCP 포트 번호 (tcp-client, tcp-server 필수)
	TCPReadTimeout     time.Duration // TCP 읽기 타임아웃 (기본: 500ms)
	TCPWriteTimeout    time.Duration // TCP 쓰기 타임아웃 (기본: 1s)
	TCPConnectTimeout  time.Duration // TCP 연결 타임아웃 (기본: 5s)
}

// parseLGCPConfig 는 Transport.Options 맵에서 LGCPConfig 를 파싱한다.
func parseLGCPConfig(opts map[string]any) (LGCPConfig, error) {
	cfg := LGCPConfig{
		BaudRate:            9600,
		DataBits:            8,
		StopBits:            1,
		Parity:              "none",
		ReadTimeout:         500 * time.Millisecond,
		MsgChannelSize:      256,
		ReconnectInterval:   5 * time.Second,
		MaxReconnectBackoff: 5 * time.Minute,
		VerifyCRC:           true,
		AutoDiscovery:       true,
		OfflineTimeout:      30 * time.Second,
		ControllerAddress:   "44550000",
		ControlVerifyTimeout: 3 * time.Second,
		ControlEnabled:      false,
		TransportType:       "serial",
		TCPHost:             "0.0.0.0",
		TCPReadTimeout:      500 * time.Millisecond,
		TCPWriteTimeout:     1 * time.Second,
		TCPConnectTimeout:   5 * time.Second,
	}

	// transport_type (기본: "serial")
	if v, ok := opts["transport_type"]; ok {
		cfg.TransportType = v.(string)
	}
	switch cfg.TransportType {
	case "serial", "tcp-client", "tcp-server":
		// 유효한 트랜스포트 타입
	default:
		return LGCPConfig{}, ErrLGCPUnknownTransportType
	}

	// serial_port (serial 모드에서만 필수)
	if v, ok := opts["serial_port"]; ok {
		cfg.SerialPort = v.(string)
	}
	if cfg.TransportType == "serial" && cfg.SerialPort == "" {
		return LGCPConfig{}, ErrLGCPSerialPortRequired
	}

	// tcp_host
	if v, ok := opts["tcp_host"]; ok {
		cfg.TCPHost = v.(string)
	}
	if cfg.TransportType == "tcp-client" && cfg.TCPHost == "" {
		return LGCPConfig{}, ErrLGCPTCPHostRequired
	}

	// tcp_port (tcp-client, tcp-server 모드에서 필수)
	if v, ok := opts["tcp_port"]; ok {
		cfg.TCPPort = toInt(v)
	}
	if (cfg.TransportType == "tcp-client" || cfg.TransportType == "tcp-server") && cfg.TCPPort <= 0 {
		return LGCPConfig{}, ErrLGCPTCPPortRequired
	}

	// tcp_read_timeout
	if v, ok := opts["tcp_read_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGCPConfig{}, fmt.Errorf("lgcp: invalid tcp_read_timeout: %w", err)
		}
		cfg.TCPReadTimeout = d
	}

	// tcp_write_timeout
	if v, ok := opts["tcp_write_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGCPConfig{}, fmt.Errorf("lgcp: invalid tcp_write_timeout: %w", err)
		}
		cfg.TCPWriteTimeout = d
	}

	// tcp_connect_timeout
	if v, ok := opts["tcp_connect_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGCPConfig{}, fmt.Errorf("lgcp: invalid tcp_connect_timeout: %w", err)
		}
		cfg.TCPConnectTimeout = d
	}

	// baud_rate
	if v, ok := opts["baud_rate"]; ok {
		br := toInt(v)
		if br <= 0 {
			return LGCPConfig{}, ErrLGCPInvalidBaudRate
		}
		cfg.BaudRate = br
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
			return LGCPConfig{}, fmt.Errorf("lgcp: invalid read_timeout: %w", err)
		}
		cfg.ReadTimeout = d
	}

	// msg_channel_size
	if v, ok := opts["msg_channel_size"]; ok {
		cfg.MsgChannelSize = toInt(v)
	}

	// reconnect_interval
	if v, ok := opts["reconnect_interval"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGCPConfig{}, fmt.Errorf("lgcp: invalid reconnect_interval: %w", err)
		}
		cfg.ReconnectInterval = d
	}

	// max_reconnect_backoff
	if v, ok := opts["max_reconnect_backoff"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGCPConfig{}, fmt.Errorf("lgcp: invalid max_reconnect_backoff: %w", err)
		}
		cfg.MaxReconnectBackoff = d
	}

	// verify_crc
	if v, ok := opts["verify_crc"]; ok {
		if b, isBool := v.(bool); isBool {
			cfg.VerifyCRC = b
		}
	}

	// auto_discovery
	if v, ok := opts["auto_discovery"]; ok {
		if b, isBool := v.(bool); isBool {
			cfg.AutoDiscovery = b
		}
	}

	// notify_interval
	if v, ok := opts["notify_interval"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGCPConfig{}, fmt.Errorf("lgcp: invalid notify_interval: %w", err)
		}
		cfg.NotifyInterval = d
	}

	// offline_timeout
	if v, ok := opts["offline_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGCPConfig{}, fmt.Errorf("lgcp: invalid offline_timeout: %w", err)
		}
		cfg.OfflineTimeout = d
	}

	// controller_address
	if v, ok := opts["controller_address"]; ok {
		s, _ := v.(string)
		if !reHex8.MatchString(s) {
			return LGCPConfig{}, fmt.Errorf("lgcp: invalid controller_address: %q (must be 8 hex chars)", s)
		}
		cfg.ControllerAddress = s
	}

	// control_verify_timeout
	if v, ok := opts["control_verify_timeout"]; ok {
		switch tv := v.(type) {
		case string:
			d, err := time.ParseDuration(tv)
			if err != nil {
				return LGCPConfig{}, fmt.Errorf("lgcp: invalid control_verify_timeout: %w", err)
			}
			cfg.ControlVerifyTimeout = d
		case int:
			cfg.ControlVerifyTimeout = time.Duration(tv) * time.Millisecond
		case float64:
			cfg.ControlVerifyTimeout = time.Duration(tv) * time.Millisecond
		}
	}

	// control_enabled
	if v, ok := opts["control_enabled"]; ok {
		if b, isBool := v.(bool); isBool {
			cfg.ControlEnabled = b
		}
	}

	// devices (선택)
	cfg.Devices = agent.ParseDevices(opts)

	return cfg, nil
}
