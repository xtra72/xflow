package lg

import (
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// LGCNPConfig 는 LGCNP-01 프로토콜 패킷 캡처 에이전트의 설정이다.
type LGCNPConfig struct {
	SerialPort          string        // 시리얼 포트 경로 (serial 모드에서 필수)
	BaudRate            int           // 보레이트 (기본: 1200)
	DataBits            int           // 데이터 비트 (기본: 8)
	StopBits            int           // 스톱 비트 (기본: 1)
	Parity              string        // 패리티 (기본: "none")
	ReadTimeout         time.Duration // 읽기 타임아웃 (기본: 500ms)
	MsgChannelSize      int           // 메시지 채널 버퍼 크기 (기본: 256)
	ReconnectInterval   time.Duration // 재연결 시도 간격 (기본: 5s)
	MaxReconnectBackoff time.Duration // 최대 재연결 백오프 (기본: 5m)
	VerifyRedundancy    bool          // TYPE-B 이중 기록 검증 (기본: true)
	VerifyODUChecksum   bool          // v0.18.1: TYPE-A ODU 체크섬 검증 (기본: true). 일부 디바이스 변형은 SEQ=04 b[19] 가 fixed 0x55 marker — 이 경우 false 로 설정해 체크섬 검증 우회.

	// 디바이스 관리
	AutoDiscovery  bool                // 자동 디바이스 발견 (기본: true)
	NotifyInterval time.Duration       // v0.6.0: report_interval 의 backing field
	ReportMode     string              // v0.6.0: "relative" (default) 또는 "absolute" (wall-clock 정렬)
	OfflineTimeout time.Duration       // 통신 없음 → 오프라인 판정 (기본: 30s)
	Devices        []agent.DeviceEntry // 설정 기반 디바이스 목록

	// 제어 기능 (미지원 — 플레이스홀더)
	ControlEnabled bool // 항상 false

	// 출력 옵션
	IncludeRawHex bool // raw_hex 필드 포함 여부 (기본: false, 디버깅/RE 시 opt-in)
	DedupeFrames  bool // 동일 state 의 중복 frame emit 차단 (기본: true)

	// 트랜스포트 타입 선택
	TransportType     string        // "serial", "tcp-client", "tcp-server" (기본: "serial")
	TCPHost           string        // TCP 호스트 주소 (기본: "0.0.0.0")
	TCPPort           int           // TCP 포트 번호
	TCPReadTimeout    time.Duration // TCP 읽기 타임아웃 (기본: 500ms)
	TCPWriteTimeout   time.Duration // TCP 쓰기 타임아웃 (기본: 1s)
	TCPConnectTimeout time.Duration // TCP 연결 타임아웃 (기본: 5s)

	// EventTempThreshold 는 change 트리거 event 보고의 실내온도 변화 임계값이다 (단위: ℃, v0.6.6).
	// IDU frame 의 CurrentTemp 만 변경되고 |Δ| < EventTempThreshold 면 emit suppress.
	// 기본 1.0℃. 0 이하면 게이트 비활성 (DedupeFrames 만 적용).
	EventTempThreshold float64
}

// parseLGCNPConfig 는 Transport.Options 맵에서 LGCNPConfig 를 파싱한다.
func parseLGCNPConfig(opts map[string]any) (LGCNPConfig, error) {
	cfg := LGCNPConfig{
		BaudRate:            1200,
		DataBits:            8,
		StopBits:            1,
		Parity:              "none",
		ReadTimeout:         500 * time.Millisecond,
		MsgChannelSize:      256,
		ReconnectInterval:   5 * time.Second,
		MaxReconnectBackoff: 5 * time.Minute,
		VerifyRedundancy:    true,
		VerifyODUChecksum:   true,
		AutoDiscovery:       true,
		OfflineTimeout:      30 * time.Second,
		ControlEnabled:      false,
		IncludeRawHex:       false, // 운영 기본 false (페이로드 크기 절감), 디버깅 시 opt-in
		DedupeFrames:        true,  // 동일 state 반복 emit 차단
		TransportType:       "serial",
		TCPHost:             "0.0.0.0",
		TCPReadTimeout:      500 * time.Millisecond,
		TCPWriteTimeout:     1 * time.Second,
		TCPConnectTimeout:   5 * time.Second,
		EventTempThreshold:  1.0,
	}

	// transport_type (기본: "serial")
	if v, ok := opts["transport_type"]; ok {
		cfg.TransportType = v.(string)
	}
	switch cfg.TransportType {
	case "serial", "tcp-client", "tcp-server":
		// 유효한 트랜스포트 타입
	default:
		return LGCNPConfig{}, ErrLGCNPUnknownTransportType
	}

	// serial_port (serial 모드에서만 필수)
	if v, ok := opts["serial_port"]; ok {
		cfg.SerialPort = v.(string)
	}
	if cfg.TransportType == "serial" && cfg.SerialPort == "" {
		return LGCNPConfig{}, ErrLGCNPSerialPortRequired
	}

	// tcp_host
	if v, ok := opts["tcp_host"]; ok {
		cfg.TCPHost = v.(string)
	}
	if cfg.TransportType == "tcp-client" && cfg.TCPHost == "" {
		return LGCNPConfig{}, ErrLGCNPTCPHostRequired
	}

	// tcp_port
	if v, ok := opts["tcp_port"]; ok {
		cfg.TCPPort = toInt(v)
	}
	if (cfg.TransportType == "tcp-client" || cfg.TransportType == "tcp-server") && cfg.TCPPort <= 0 {
		return LGCNPConfig{}, ErrLGCNPTCPPortRequired
	}

	// tcp_read_timeout
	if v, ok := opts["tcp_read_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGCNPConfig{}, fmt.Errorf("lgcnp: invalid tcp_read_timeout: %w", err)
		}
		cfg.TCPReadTimeout = d
	}

	// tcp_write_timeout
	if v, ok := opts["tcp_write_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGCNPConfig{}, fmt.Errorf("lgcnp: invalid tcp_write_timeout: %w", err)
		}
		cfg.TCPWriteTimeout = d
	}

	// tcp_connect_timeout
	if v, ok := opts["tcp_connect_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGCNPConfig{}, fmt.Errorf("lgcnp: invalid tcp_connect_timeout: %w", err)
		}
		cfg.TCPConnectTimeout = d
	}

	// baud_rate
	if v, ok := opts["baud_rate"]; ok {
		br := toInt(v)
		if br <= 0 {
			return LGCNPConfig{}, ErrLGCNPInvalidBaudRate
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
			return LGCNPConfig{}, fmt.Errorf("lgcnp: invalid read_timeout: %w", err)
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
			return LGCNPConfig{}, fmt.Errorf("lgcnp: invalid reconnect_interval: %w", err)
		}
		cfg.ReconnectInterval = d
	}

	// max_reconnect_backoff
	if v, ok := opts["max_reconnect_backoff"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGCNPConfig{}, fmt.Errorf("lgcnp: invalid max_reconnect_backoff: %w", err)
		}
		cfg.MaxReconnectBackoff = d
	}

	// verify_redundancy
	if v, ok := opts["verify_redundancy"]; ok {
		if b, isBool := v.(bool); isBool {
			cfg.VerifyRedundancy = b
		}
	}

	// v0.18.1: verify_odu_checksum — TYPE-A ODU 체크섬 검증 토글.
	// 일부 디바이스 변형은 SEQ=04 b[19] 가 fixed 0x55 marker — false 로 설정하면
	// 체크섬 mismatch 에도 frame 을 폐기하지 않음.
	if v, ok := opts["verify_odu_checksum"]; ok {
		if b, isBool := v.(bool); isBool {
			cfg.VerifyODUChecksum = b
		}
	}

	// auto_discovery
	if v, ok := opts["auto_discovery"]; ok {
		if b, isBool := v.(bool); isBool {
			cfg.AutoDiscovery = b
		}
	}

	// report_interval (이전: notify_interval) — 주기적 상태보고 간격. v0.6.0 통합 명칭.
	// notify_interval 은 deprecation alias.
	for _, key := range []string{"report_interval", "notify_interval"} {
		v, ok := opts[key]
		if !ok {
			continue
		}
		s, sok := v.(string)
		if !sok {
			continue
		}
		d, err := time.ParseDuration(s)
		if err != nil {
			return LGCNPConfig{}, fmt.Errorf("lgcnp: invalid %s: %w", key, err)
		}
		cfg.NotifyInterval = d
	}

	// report_mode — 상태보고 시점 정책 (v0.6.0). "relative" (기본) 또는 "absolute".
	if v, ok := opts["report_mode"]; ok {
		if s, sok := v.(string); sok {
			switch s {
			case "relative", "absolute", "":
				cfg.ReportMode = s
			default:
				return LGCNPConfig{}, fmt.Errorf("lgcnp: invalid report_mode %q (must be 'relative' or 'absolute')", s)
			}
		}
	}
	if cfg.ReportMode == "" {
		cfg.ReportMode = "relative"
	}

	// offline_timeout
	if v, ok := opts["offline_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGCNPConfig{}, fmt.Errorf("lgcnp: invalid offline_timeout: %w", err)
		}
		cfg.OfflineTimeout = d
	}

	// include_raw_hex — raw_hex 필드 포함 여부 (기본 false, opt-in)
	if v, ok := opts["include_raw_hex"]; ok {
		if b, isBool := v.(bool); isBool {
			cfg.IncludeRawHex = b
		}
	}

	// dedupe_frames — 동일 state 반복 emit 차단 (기본 true)
	if v, ok := opts["dedupe_frames"]; ok {
		if b, isBool := v.(bool); isBool {
			cfg.DedupeFrames = b
		}
	}

	// devices (선택)
	cfg.Devices = agent.ParseDevices(opts)

	// event_temp_threshold (v0.6.6) — 실내온도 변화 임계값 (단위 ℃, 기본 1.0).
	if v, ok := opts["event_temp_threshold"]; ok {
		f, err := toFloat64(v)
		if err != nil {
			return LGCNPConfig{}, fmt.Errorf("lgcnp: invalid event_temp_threshold: %w", err)
		}
		cfg.EventTempThreshold = f
	}

	return cfg, nil
}
