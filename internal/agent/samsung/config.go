package samsung

import (
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// NASAConfig 는 Samsung NASA HVAC 에이전트의 설정을 나타낸다.
type NASAConfig struct {
	TransportType         string
	SerialPort            string
	BaudRate              int
	DataBits              int
	StopBits              int
	Parity                string
	TCPHost               string
	TCPPort               int
	ConnectTimeout        time.Duration
	ReadTimeout           time.Duration
	PollInterval          time.Duration
	StatusQueryEnabled    bool          // v0.6.1: 주기적 상태 확인 요청 (BuildStatusQuery) 송신 여부. false 면 passive sniff only (기본 true)
	NotifyInterval        time.Duration // v0.6.0: report_interval 의 backing field. 옵션 명칭은 report_interval 권장.
	ReportMode            string        // v0.6.0: "relative" (default) 또는 "absolute" (wall-clock 정렬). Century 와 통일.
	Devices               []agent.DeviceEntry
	ProtocolFile          string
	AutoDiscovery         bool
	RegistryPath          string
	OfflineThreshold      int
	OfflineTimeout        time.Duration // v0.6.2: 디바이스 통신 없음 → 오프라인 판정 시간 (기본 30s). 0 = 비활성
	MsgChannelSize        int
	UnsupportedMsgSets    map[uint16]bool // 필터링할 메시지 셋 인덱스
	LogUnsupportedMsgSets bool            // 필터링 시 로그 출력 여부
	LogDecodeErrors       bool            // 일반 decode error 로그 출력 여부 (기본값 false — 운영 환경 noise 억제)
	IncludeRawMessageSets bool            // 상태 조회 시 RawMessageSets 포함 여부
	ReconnectInterval     time.Duration   // 재연결 기본 간격 (기본값 5s)
	MaxReconnectBackoff   time.Duration   // 재연결 최대 백오프 (기본값 5m)
	StatusQueryDelay      time.Duration   // 제어 후 상태 조회 간격 (기본값 3s)
	StatusQueryRetries    int             // 제어 후 상태 조회 횟수 (기본값 3)
	BuzzerOnControl       bool            // 제어 명령 시 실내기 부저 울림 (기본값 false)

	// EventTempThreshold 는 change 트리거 event 보고의 실내온도 변화 임계값이다 (단위: ℃, v0.6.6).
	//
	// change 감지 시 변경된 필드가 실내온도(current_temp)뿐이면
	// |curr_temp − lastReportTemp| >= EventTempThreshold 일 때만 emit 한다.
	// 온도 외 필드(power/mode/target_temp/fan_speed 등)가 함께 변경되면
	// 임계값과 무관하게 즉시 emit (기존 동작 유지).
	//
	// 기본 1.0℃. 0 이하면 게이트 비활성. report 시점에도 lastReportTemp 갱신.
	EventTempThreshold float64
}

// parseNASAConfig 는 Transport.Options 맵에서 NASAConfig 를 파싱한다.
func parseNASAConfig(opts map[string]any) (NASAConfig, error) {
	cfg := NASAConfig{
		BaudRate:            9600,
		DataBits:            8,
		StopBits:            1,
		Parity:              "even",
		ConnectTimeout:      5 * time.Second,
		ReadTimeout:         3 * time.Second,
		PollInterval:        30 * time.Second,
		StatusQueryEnabled:  true, // v0.6.1: 주기적 상태 확인 요청 기본 활성 (기존 동작 보존)
		NotifyInterval:      0,
		OfflineTimeout:      30 * time.Second, // v0.6.2: 디바이스 오프라인 판정 시간 default
		OfflineThreshold:    3,
		MsgChannelSize:      256,
		ReconnectInterval:   5 * time.Second,
		MaxReconnectBackoff: 5 * time.Minute,
		StatusQueryDelay:    3 * time.Second,
		StatusQueryRetries:  3,
		EventTempThreshold:  1.0,
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

	// tcp_host (LG ICP-01/LGCP 패턴과 통일)
	if v, ok := opts["tcp_host"]; ok {
		if s, ok := v.(string); ok {
			cfg.TCPHost = s
		}
	}

	// tcp_port (int 또는 float64; YAML/JSON 모두 호환)
	if v, ok := opts["tcp_port"]; ok {
		cfg.TCPPort = toInt(v)
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

	// offline_timeout (v0.6.2) — 디바이스 통신 없음 → 오프라인 판정 시간.
	if v, ok := opts["offline_timeout"]; ok {
		s, sok := v.(string)
		if sok {
			d, err := time.ParseDuration(s)
			if err != nil {
				return NASAConfig{}, fmt.Errorf("samsung-nasa: invalid offline_timeout: %w", err)
			}
			if d < 0 {
				return NASAConfig{}, fmt.Errorf("samsung-nasa: offline_timeout must be >= 0, got %s", d)
			}
			cfg.OfflineTimeout = d
		}
	}

	// status_query_enabled (v0.6.1) — 주기적 상태 확인 요청 송신 여부.
	// false 면 pollLoop 가 BuildStatusQuery 송신을 skip → passive sniff only 모드.
	// 기본 true (v0.6.1 이전과 동일 동작).
	if v, ok := opts["status_query_enabled"]; ok {
		if b, bok := v.(bool); bok {
			cfg.StatusQueryEnabled = b
		}
	}

	// report_interval (이전: notify_interval) — 주기적 상태보고 간격. v0.6.0 통합 명칭.
	// notify_interval 은 deprecation alias 로 silent accept.
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
			return NASAConfig{}, fmt.Errorf("samsung-nasa: invalid %s: %w", key, err)
		}
		cfg.NotifyInterval = d
	}

	// report_mode — 상태보고 시점 정책. "relative" (기본) 또는 "absolute" (wall-clock 정렬).
	// v0.6.0 통합 옵션 — Century 와 동일 의미.
	if v, ok := opts["report_mode"]; ok {
		if s, sok := v.(string); sok {
			switch s {
			case "relative", "absolute", "":
				cfg.ReportMode = s
			default:
				return NASAConfig{}, fmt.Errorf("samsung-nasa: invalid report_mode %q (must be 'relative' or 'absolute')", s)
			}
		}
	}
	if cfg.ReportMode == "" {
		cfg.ReportMode = "relative"
	}

	// devices (선택)
	cfg.Devices = agent.ParseDevices(opts)

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

	// log_decode_errors (기본값: false — 운영 환경 noise 억제, 디버깅 시 true)
	// 2026-05-14 hotfix: invalid message set index 등 디코드 오류가 운영 중 빈번하게
	// 발생하면 로그 폭주로 이어지므로 옵션으로 끌 수 있게 한다.
	if v, ok := opts["log_decode_errors"]; ok {
		if b, ok := v.(bool); ok {
			cfg.LogDecodeErrors = b
		}
	}

	// include_raw_message_sets (기본값: false)
	// RawMessageSets(원본 NASA 메시지 전체)는 페이로드 크기를 크게 늘리므로
	// 기본적으로 출력에서 제외하고, 디버깅 시에만 opt-in 으로 활성화한다.
	cfg.IncludeRawMessageSets = false
	if v, ok := opts["include_raw_message_sets"]; ok {
		if b, ok := v.(bool); ok {
			cfg.IncludeRawMessageSets = b
		}
	}

	// reconnect_interval
	if v, ok := opts["reconnect_interval"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return NASAConfig{}, fmt.Errorf("samsung-nasa: invalid reconnect_interval: %w", err)
		}
		cfg.ReconnectInterval = d
	}

	// max_reconnect_backoff
	if v, ok := opts["max_reconnect_backoff"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return NASAConfig{}, fmt.Errorf("samsung-nasa: invalid max_reconnect_backoff: %w", err)
		}
		cfg.MaxReconnectBackoff = d
	}

	// status_query_delay
	if v, ok := opts["status_query_delay"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return NASAConfig{}, fmt.Errorf("samsung-nasa: invalid status_query_delay: %w", err)
		}
		cfg.StatusQueryDelay = d
	}

	// status_query_retries
	if v, ok := opts["status_query_retries"]; ok {
		cfg.StatusQueryRetries = toInt(v)
	}

	// buzzer_on_control (선택, 기본값 false)
	if v, ok := opts["buzzer_on_control"]; ok {
		cfg.BuzzerOnControl = toBool(v)
	}

	// event_temp_threshold (v0.6.6) — 실내온도 변화 임계값 (단위 ℃, 기본 1.0).
	if v, ok := opts["event_temp_threshold"]; ok {
		f, err := toFloat64(v)
		if err != nil {
			return NASAConfig{}, fmt.Errorf("samsung-nasa: invalid event_temp_threshold: %w", err)
		}
		cfg.EventTempThreshold = f
	}

	return cfg, nil
}

// toFloat64 는 수치 후보를 float64 로 변환한다 (v0.6.6).
func toFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	default:
		return 0, fmt.Errorf("expected number, got %T", v)
	}
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

func toBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	default:
		return false
	}
}
