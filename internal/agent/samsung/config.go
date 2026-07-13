package samsung

import (
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// Hvacr01Config 는 Samsung NASA HVAC 에이전트의 설정을 나타낸다.
type Hvacr01Config struct {
	TransportType      string
	SerialPort         string
	BaudRate           int
	DataBits           int
	StopBits           int
	Parity             string
	TCPHost            string
	TCPPort            int
	ConnectTimeout     time.Duration
	ReadTimeout        time.Duration
	PollInterval       time.Duration
	StatusQueryEnabled bool          // v0.6.1: 주기적 상태 확인 요청 (BuildStatusQuery) 송신 여부. false 면 passive sniff only (기본 true)
	NotifyInterval     time.Duration // v0.6.0: report_interval 의 backing field. 옵션 명칭은 report_interval. 2026-05-29: 기본 60s (LG 통일)
	ReportMode         string        // v0.6.0: "relative" (default) 또는 "absolute" (wall-clock 정렬). Century 와 통일.
	Devices            []agent.DeviceEntry
	ProtocolFile       string
	AutoDiscovery      bool // 2026-05-29: 기본 true (LG / Century 통일)
	RegistryPath       string
	OfflineThreshold   int
	// OfflineTimeout 은 LastSeen 기반 stale offline 판정 임계값이다 (offline_timeout 키).
	// 3-way 의미(사용자 승인):
	//   - 양수: 그 값을 임계값으로 verbatim 사용.
	//   - 0(명시): staleness 감지 완전 비활성 (transport-disconnect bulk offline 은 무관하게 동작).
	//   - 음수 sentinel(-1, 미설정): OfflineThreshold × PollInterval 파생 (기본 3×30s=90s).
	// 파서는 opts 에 키가 있을 때만 값을 대입하므로 "미설정(-1)"과 "명시적 0"을 구분한다.
	// 검증 규칙상 사용자는 음수를 넣을 수 없어 -1 은 오직 "미설정"만을 뜻한다.
	OfflineTimeout        time.Duration
	MsgChannelSize        int
	UnsupportedMsgSets    map[uint16]bool // 필터링할 메시지 셋 인덱스
	LogUnsupportedMsgSets bool            // 필터링 시 로그 출력 여부
	LogDecodeErrors       bool            // 일반 decode error 로그 출력 여부 (기본값 false — 운영 환경 noise 억제)
	LogDrops              bool            // 2026-05-29: msgCh full 로 인한 event drop 을 WARN 로그로 출력 (기본 false, Century / LG 통일).
	LogStateUpdates       bool            // 2026-05-29: 디바이스 state 갱신마다 핵심 필드 + raw payload INFO 로그 (Century logDecodedState 패턴).
	LogMessages           bool            // 디바이스와의 송/수신(TX/RX) 프레임을 hex 로 INFO 로그 (기본 false, opt-in 진단용).
	IncludeRawHex         bool            // 2026-05-29: 이전 IncludeRawMessageSets — RawMessageSets (원본 NASA 메시지 전체) 포함 여부 (기본 false, opt-in). LG IncludeRawHex 와 명칭 통일.
	ReconnectInterval     time.Duration   // 재연결 기본 간격 (기본값 5s)
	MaxReconnectBackoff   time.Duration   // 재연결 최대 백오프 (기본값 5m)
	StatusQueryDelay      time.Duration   // 제어 후 상태 조회 간격 (기본값 3s)
	StatusQueryRetries    int             // 제어 후 상태 조회 횟수 (기본값 3)
	BuzzerOnControl       bool            // 제어 명령 시 실내기 부저 울림 (기본값 false)
	ControlEnabled        bool            // 능동 제어 (set_multiple) 활성 여부 (기본값 true). false 면 제어 명령 거부.

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

// parseHvacr01Config 는 Transport.Options 맵에서 Hvacr01Config 를 파싱한다.
//
// 2026-05-29 breaking changes (3종 HVACR 에이전트 통일):
//   - notify_interval / include_raw_message_sets alias 제거 → report_interval / include_raw_hex 만 허용.
//   - report_interval 기본값 0 → 60s (LG 통일).
//   - auto_discovery 기본값 false → true (LG / Century 통일).
func parseHvacr01Config(opts map[string]any) (Hvacr01Config, error) {
	// Reject deprecated alias keys with clear errors (2026-05-29 breaking).
	if _, ok := opts["notify_interval"]; ok {
		return Hvacr01Config{}, fmt.Errorf("samsung_hvacr01: deprecated option 'notify_interval' is removed; use 'report_interval' instead")
	}
	if _, ok := opts["include_raw_message_sets"]; ok {
		return Hvacr01Config{}, fmt.Errorf("samsung_hvacr01: deprecated option 'include_raw_message_sets' is removed; use 'include_raw_hex' instead")
	}

	cfg := Hvacr01Config{
		BaudRate:            9600,
		DataBits:            8,
		StopBits:            1,
		Parity:              "even",
		ConnectTimeout:      5 * time.Second,
		ReadTimeout:         3 * time.Second,
		PollInterval:        30 * time.Second,
		StatusQueryEnabled:  true,             // v0.6.1: 주기적 상태 확인 요청 기본 활성 (기존 동작 보존)
		NotifyInterval:      60 * time.Second, // 2026-05-29: 기본 60s (LG 통일).
		ReportMode:          "relative",
		AutoDiscovery:       true, // 2026-05-29: 기본 true (LG / Century 통일).
		OfflineTimeout:      -1,   // sentinel: 미설정 → staleOfflineThreshold 에서 OfflineThreshold×PollInterval 파생
		OfflineThreshold:    3,
		MsgChannelSize:      256,
		ReconnectInterval:   5 * time.Second,
		MaxReconnectBackoff: 5 * time.Minute,
		StatusQueryDelay:    3 * time.Second,
		StatusQueryRetries:  3,
		ControlEnabled:      true, // 2026-05-29: 능동 제어 (set_multiple) 기본 활성 (기존 동작 보존)
		EventTempThreshold:  1.0,
	}

	// transport_type (필수)
	//
	// 2026-05-29 breaking changes (LG/Century 통일):
	//   - 유효 값: "serial", "tcp-client", "tcp-server".
	//   - 이전의 단일 "tcp" 값은 거부되며, 사용자는 "tcp-client" 로 명시적으로 마이그레이션해야 한다.
	if v, ok := opts["transport_type"]; ok {
		s, sok := v.(string)
		if !sok {
			return Hvacr01Config{}, fmt.Errorf("samsung_hvacr01: transport_type must be a string")
		}
		cfg.TransportType = s
	}
	if cfg.TransportType == "" {
		return Hvacr01Config{}, fmt.Errorf("samsung_hvacr01: transport_type is required")
	}
	switch cfg.TransportType {
	case "serial", "tcp-client", "tcp-server":
		// valid transport types
	case "tcp":
		// 2026-05-29 breaking: explicit migration error pointing user to new value.
		return Hvacr01Config{}, ErrDeprecatedTCPTransport
	default:
		return Hvacr01Config{}, fmt.Errorf("%w: got %q", ErrInvalidTransportType, cfg.TransportType)
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
	//
	// 2026-05-29 (tcp-server 추가):
	//   - tcp-client: 원격 서버 IP — 필수.
	//   - tcp-server: 바인드 주소 — 미지정 시 "0.0.0.0" (모든 인터페이스).
	if v, ok := opts["tcp_host"]; ok {
		if s, ok := v.(string); ok {
			cfg.TCPHost = s
		}
	}

	// tcp_port (int 또는 float64; YAML/JSON 모두 호환)
	if v, ok := opts["tcp_port"]; ok {
		cfg.TCPPort = toInt(v)
	}

	// TCP 모드별 필드 검증 (2026-05-29 추가):
	//   - tcp-server: tcp_host 기본값 "0.0.0.0" 적용.
	//   - tcp-client: tcp_host 필수 (서버 IP 지정 필수).
	//   - 양쪽 모두: tcp_port 필수 (1-65535).
	// 실제 transport 생성 시점에도 검증되지만, parse 단계에서 명시적 에러를 반환하여
	// agent 생성 실패 메시지를 더 명확하게 한다.
	if cfg.TransportType == "tcp-server" && cfg.TCPHost == "" {
		cfg.TCPHost = "0.0.0.0"
	}
	if cfg.TransportType == "tcp-client" && cfg.TCPHost == "" {
		return Hvacr01Config{}, ErrTCPHostRequired
	}
	if cfg.TransportType == "tcp-client" || cfg.TransportType == "tcp-server" {
		if cfg.TCPPort < 1 || cfg.TCPPort > 65535 {
			return Hvacr01Config{}, fmt.Errorf("%w: got %d", ErrTCPPortRequired, cfg.TCPPort)
		}
	}

	// connect_timeout
	if v, ok := opts["connect_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return Hvacr01Config{}, fmt.Errorf("samsung_hvacr01: invalid connect_timeout: %w", err)
		}
		cfg.ConnectTimeout = d
	}

	// read_timeout
	if v, ok := opts["read_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return Hvacr01Config{}, fmt.Errorf("samsung_hvacr01: invalid read_timeout: %w", err)
		}
		cfg.ReadTimeout = d
	}

	// poll_interval
	if v, ok := opts["poll_interval"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return Hvacr01Config{}, fmt.Errorf("samsung_hvacr01: invalid poll_interval: %w", err)
		}
		cfg.PollInterval = d
	}

	// offline_timeout (v0.6.2) — LastSeen 기반 stale offline 판정 임계값 (3-way, OfflineTimeout 필드 주석 참조).
	// 키가 opts 에 존재할 때만 대입하여 "미설정(-1 sentinel)"과 "명시적 0(비활성)"을 구분한다.
	// 명시적 0 은 허용값이며(>= 0 검증 통과) staleness 감지를 비활성화한다.
	if v, ok := opts["offline_timeout"]; ok {
		s, sok := v.(string)
		if sok {
			d, err := time.ParseDuration(s)
			if err != nil {
				return Hvacr01Config{}, fmt.Errorf("samsung_hvacr01: invalid offline_timeout: %w", err)
			}
			if d < 0 {
				return Hvacr01Config{}, fmt.Errorf("samsung_hvacr01: offline_timeout must be >= 0, got %s", d)
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

	// report_interval — 주기적 상태보고 간격. v0.6.0 통합 명칭, 2026-05-29 단일화.
	if v, ok := opts["report_interval"]; ok {
		s, sok := v.(string)
		if sok {
			d, err := time.ParseDuration(s)
			if err != nil {
				return Hvacr01Config{}, fmt.Errorf("samsung_hvacr01: invalid report_interval: %w", err)
			}
			cfg.NotifyInterval = d
		}
	}

	// report_mode — 상태보고 시점 정책. "relative" (기본) 또는 "absolute" (wall-clock 정렬).
	// v0.6.0 통합 옵션 — Century 와 동일 의미.
	if v, ok := opts["report_mode"]; ok {
		if s, sok := v.(string); sok {
			switch s {
			case "relative", "absolute", "":
				cfg.ReportMode = s
			default:
				return Hvacr01Config{}, fmt.Errorf("samsung_hvacr01: invalid report_mode %q (must be 'relative' or 'absolute')", s)
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

	// include_raw_hex (이전: include_raw_message_sets, 2026-05-29 rename — LG 통일).
	// 기본값 false. RawMessageSets (원본 NASA 메시지 전체) 는 페이로드 크기를 크게
	// 늘리므로 기본적으로 출력에서 제외하고, 디버깅 시에만 opt-in 으로 활성화한다.
	if v, ok := opts["include_raw_hex"]; ok {
		if b, ok := v.(bool); ok {
			cfg.IncludeRawHex = b
		}
	}

	// log_drops (기본값: false) — msgCh full 로 인한 event drop 을 WARN 로그로 출력.
	// 2026-05-29: Century / LG 와 통일.
	if v, ok := opts["log_drops"]; ok {
		if b, ok := v.(bool); ok {
			cfg.LogDrops = b
		}
	}

	// log_state_updates (기본값: false) — 디바이스 state 갱신마다 핵심 필드 +
	// raw payload hex 를 INFO 로그로 출력 (Century logDecodedState 패턴).
	// 진단용 — 비정상 값 추적 시 opt-in.
	if v, ok := opts["log_state_updates"]; ok {
		if b, ok := v.(bool); ok {
			cfg.LogStateUpdates = b
		}
	}

	// log_messages (기본값: false) — 디바이스와의 송/수신(TX/RX) 프레임을 hex 로
	// INFO 로그로 출력. 진단용 opt-in (버스 트래픽 확인, 통신 문제 추적).
	if v, ok := opts["log_messages"]; ok {
		if b, ok := v.(bool); ok {
			cfg.LogMessages = b
		}
	}

	// reconnect_interval
	if v, ok := opts["reconnect_interval"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return Hvacr01Config{}, fmt.Errorf("samsung_hvacr01: invalid reconnect_interval: %w", err)
		}
		cfg.ReconnectInterval = d
	}

	// max_reconnect_backoff
	if v, ok := opts["max_reconnect_backoff"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return Hvacr01Config{}, fmt.Errorf("samsung_hvacr01: invalid max_reconnect_backoff: %w", err)
		}
		cfg.MaxReconnectBackoff = d
	}

	// status_query_delay
	if v, ok := opts["status_query_delay"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return Hvacr01Config{}, fmt.Errorf("samsung_hvacr01: invalid status_query_delay: %w", err)
		}
		cfg.StatusQueryDelay = d
	}

	// status_query_retries
	if v, ok := opts["status_query_retries"]; ok {
		cfg.StatusQueryRetries = toInt(v)
	}

	// control_enabled (선택, 기본값 true)
	// 2026-05-29: 능동 제어 (set_multiple) 활성/비활성 토글.
	// false 면 set_multiple 명령이 거부된다. UI 일관성 위해 LG/Century 와 동일하게 노출.
	if v, ok := opts["control_enabled"]; ok {
		cfg.ControlEnabled = toBool(v)
	}

	// buzzer_on_control (선택, 기본값 false)
	if v, ok := opts["buzzer_on_control"]; ok {
		cfg.BuzzerOnControl = toBool(v)
	}

	// event_temp_threshold (v0.6.6) — 실내온도 변화 임계값 (단위 ℃, 기본 1.0).
	if v, ok := opts["event_temp_threshold"]; ok {
		f, err := toFloat64(v)
		if err != nil {
			return Hvacr01Config{}, fmt.Errorf("samsung_hvacr01: invalid event_temp_threshold: %w", err)
		}
		cfg.EventTempThreshold = f
	}

	// 연결 정보는 device_state 단일 스트림으로 일원화되었으므로 별도 connection 옵션
	// (connection_report_interval / startup_probe_timeout / deprecated connection_notify_interval)
	// 은 더 이상 파싱하지 않는다. 기존 config 에 이 키들이 남아 있어도 hard-error 없이 조용히
	// 무시된다(알 수 없는 키 무시 정책). 주기 보고는 report_interval(device_state) 로 수렴한다.

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
