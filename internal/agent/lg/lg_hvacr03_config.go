package lg

import (
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// LG HVACR-03 (PMBUSB00A) 설정
//
// SPEC-LG-HVACR-003 § M3 / § 5 설정 스키마.
//
// 타입 안전 파싱: 본 파일의 헬퍼는 잘못된 타입의 YAML 값에 panic 하지 않고 설정
// 오류를 반환한다. 무검사 타입 단언(v.(string))은 쓰지 않는다.
// ---------------------------------------------------------------------------

// 폴링 주기 하한. 게이트웨이가 이보다 빠른 주기에서 응답을 거르기 시작한다.
// 9,600 bps 에서 실내기 16대를 4종 영역 모두 폴링하면 한 사이클에 2~4초가 걸린다.
const hvacr03MinPollInterval = 5 * time.Second

// Hvacr03Config 는 PMBUSB00A Modbus 게이트웨이 에이전트의 설정이다.
type Hvacr03Config struct {
	// --- 트랜스포트 ---
	TransportType string // "rtu" | "tcp-client" (기본: "rtu")
	SerialPort    string // rtu 필수 (예: /dev/ttyUSB0)
	BaudRate      int    // 기본 9600
	DataBits      int    // 기본 8
	StopBits      int    // 기본 1
	Parity        string // 기본 "none"
	TCPHost       string // tcp-client 필수
	TCPPort       int    // 기본 502
	SlaveID       byte   // 게이트웨이 슬레이브 주소 1~16 (DIP SW_02M), 기본 1

	// --- 폴링 ---
	PollInterval        time.Duration // 코일/Holding/Input 폴링 주기 (기본 10s, 하한 5s)
	ScanInterval        time.Duration // FC02 전체 스캔 주기 (기본 30s)
	RequestTimeout      time.Duration // 단일 트랜잭션 타임아웃 (기본 1s)
	OfflineTimeout      time.Duration // 무응답 → 오프라인 판정 (기본 30s)
	ReconnectInterval   time.Duration // 재연결 시도 간격 (기본 5s)
	MaxReconnectBackoff time.Duration // 최대 재연결 백오프 (기본 5m)

	// --- 미검증 가정 (프로토콜 문서 §8) ---
	// 문서값을 기본값으로 삼되 설정으로 열어 둔다. 현장 실측이 문서와 다르면
	// 재빌드 없이 YAML 한 줄로 교정한다.
	TempScale   int // 설정/센서 온도 스케일 (기본 10 — ×10)
	AddressBase int // 사용자 표기 주소의 기준 (기본 0 — N 은 0~15)
	FanAutoCode int // "자동"에 해당하는 풍량 코드 (기본 4; LGCP 는 5)

	// --- 디바이스 관리 ---
	AutoDiscovery      bool                // 스캔에서 발견된 실내기 자동 등록 (기본 true)
	ReportInterval     time.Duration       // 주기적 전체 상태 보고 (기본 60s)
	EventTempThreshold float64             // change 보고의 실내온도 변화 임계 (기본 1.0 ℃)
	MsgChannelSize     int                 // 메시지 채널 버퍼 크기 (기본 256)
	Devices            []agent.DeviceEntry // 설정 기반 디바이스 목록
	// DeviceTypes 는 주소별 기기 종류 지정이다 (사용자 표기 주소 → 종류).
	// ERV 는 프로토콜로 자동 판별할 신호가 없어 설정으로 지정해야 한다. 하이드로킷은
	// Discrete ④(목표 온도 기준 = 물)로 자동 승격되므로 지정이 선택적이다.
	DeviceTypes map[string]string

	// --- 제어 ---
	ControlEnabled     bool          // 제어 기능 토글 (기본 false)
	ControlVerifyDelay time.Duration // 쓰기 후 read-back 까지 대기 (기본 3s)

	// --- 로그 ---
	LogMessages     bool // TX/RX 프레임 hex 를 INFO 로그로 출력
	LogStateUpdates bool // 디바이스 state 갱신을 INFO 로그로 출력
	LogDecodeErrors bool // 응답 파싱 실패를 WARN 로그로 출력
	LogDrops        bool // msgCh full 로 인한 drop 을 per-drop WARN 로그로 출력
}

// defaultHvacr03Config 는 SPEC § 5 의 기본값을 반환한다.
func defaultHvacr03Config() Hvacr03Config {
	return Hvacr03Config{
		TransportType:       "rtu",
		BaudRate:            9600,
		DataBits:            8,
		StopBits:            1,
		Parity:              "none",
		TCPPort:             502,
		SlaveID:             1,
		PollInterval:        10 * time.Second,
		ScanInterval:        30 * time.Second,
		RequestTimeout:      1 * time.Second,
		OfflineTimeout:      30 * time.Second,
		ReconnectInterval:   5 * time.Second,
		MaxReconnectBackoff: 5 * time.Minute,
		TempScale:           10,
		AddressBase:         0,
		FanAutoCode:         4,
		AutoDiscovery:       true,
		ReportInterval:      60 * time.Second,
		EventTempThreshold:  1.0,
		MsgChannelSize:      256,
		ControlEnabled:      false,
		ControlVerifyDelay:  3 * time.Second,
	}
}

// parseHvacr03Config 는 Transport.Options 맵에서 Hvacr03Config 를 파싱한다.
func parseHvacr03Config(opts map[string]any) (Hvacr03Config, error) {
	cfg := defaultHvacr03Config()

	// --- 트랜스포트 ---
	if err := optString(opts, "transport_type", &cfg.TransportType); err != nil {
		return Hvacr03Config{}, err
	}
	switch cfg.TransportType {
	case "rtu", "tcp-client":
		// 유효. tcp-server 는 지원하지 않는다 — 본 에이전트는 능동 마스터이다.
	default:
		return Hvacr03Config{}, fmt.Errorf("%w: %q", ErrHvacr03UnknownTransportType, cfg.TransportType)
	}

	if err := optString(opts, "serial_port", &cfg.SerialPort); err != nil {
		return Hvacr03Config{}, err
	}
	if err := optString(opts, "tcp_host", &cfg.TCPHost); err != nil {
		return Hvacr03Config{}, err
	}
	if err := optInt(opts, "tcp_port", &cfg.TCPPort); err != nil {
		return Hvacr03Config{}, err
	}
	if err := optInt(opts, "baud_rate", &cfg.BaudRate); err != nil {
		return Hvacr03Config{}, err
	}
	if err := optInt(opts, "data_bits", &cfg.DataBits); err != nil {
		return Hvacr03Config{}, err
	}
	if err := optInt(opts, "stop_bits", &cfg.StopBits); err != nil {
		return Hvacr03Config{}, err
	}
	if err := optString(opts, "parity", &cfg.Parity); err != nil {
		return Hvacr03Config{}, err
	}

	switch cfg.TransportType {
	case "rtu":
		if cfg.SerialPort == "" {
			return Hvacr03Config{}, ErrHvacr03SerialPortRequired
		}
		if cfg.BaudRate <= 0 {
			return Hvacr03Config{}, fmt.Errorf("lg_hvacr03: baud_rate must be positive, got %d", cfg.BaudRate)
		}
	case "tcp-client":
		if cfg.TCPHost == "" {
			return Hvacr03Config{}, ErrHvacr03TCPHostRequired
		}
		if cfg.TCPPort <= 0 || cfg.TCPPort > 65535 {
			return Hvacr03Config{}, fmt.Errorf("lg_hvacr03: tcp_port out of range: %d", cfg.TCPPort)
		}
	}

	// slave_id — DIP 스위치가 표현할 수 있는 1~16.
	slaveID := int(cfg.SlaveID)
	if err := optInt(opts, "slave_id", &slaveID); err != nil {
		return Hvacr03Config{}, err
	}
	if slaveID < 1 || slaveID > 16 {
		return Hvacr03Config{}, fmt.Errorf("%w: got %d", ErrHvacr03InvalidSlaveID, slaveID)
	}
	cfg.SlaveID = byte(slaveID)

	// --- 폴링 ---
	if err := optDuration(opts, "poll_interval", &cfg.PollInterval); err != nil {
		return Hvacr03Config{}, err
	}
	if cfg.PollInterval < hvacr03MinPollInterval {
		return Hvacr03Config{}, fmt.Errorf("%w: got %s", ErrHvacr03PollIntervalTooShort, cfg.PollInterval)
	}
	if err := optDuration(opts, "scan_interval", &cfg.ScanInterval); err != nil {
		return Hvacr03Config{}, err
	}
	if err := optDuration(opts, "request_timeout", &cfg.RequestTimeout); err != nil {
		return Hvacr03Config{}, err
	}
	if err := optDuration(opts, "offline_timeout", &cfg.OfflineTimeout); err != nil {
		return Hvacr03Config{}, err
	}
	if err := optDuration(opts, "reconnect_interval", &cfg.ReconnectInterval); err != nil {
		return Hvacr03Config{}, err
	}
	if err := optDuration(opts, "max_reconnect_backoff", &cfg.MaxReconnectBackoff); err != nil {
		return Hvacr03Config{}, err
	}

	// --- 미검증 가정 ---
	if err := optInt(opts, "temp_scale", &cfg.TempScale); err != nil {
		return Hvacr03Config{}, err
	}
	if cfg.TempScale <= 0 {
		return Hvacr03Config{}, fmt.Errorf("lg_hvacr03: temp_scale must be positive, got %d", cfg.TempScale)
	}
	if err := optInt(opts, "address_base", &cfg.AddressBase); err != nil {
		return Hvacr03Config{}, err
	}
	if cfg.AddressBase < 0 || cfg.AddressBase > 1 {
		return Hvacr03Config{}, fmt.Errorf("lg_hvacr03: address_base must be 0 or 1, got %d", cfg.AddressBase)
	}
	if err := optInt(opts, "fan_auto_code", &cfg.FanAutoCode); err != nil {
		return Hvacr03Config{}, err
	}
	if cfg.FanAutoCode < 1 || cfg.FanAutoCode > 5 {
		return Hvacr03Config{}, fmt.Errorf("lg_hvacr03: fan_auto_code must be 1-5, got %d", cfg.FanAutoCode)
	}

	// --- 디바이스 관리 ---
	if err := optBool(opts, "auto_discovery", &cfg.AutoDiscovery); err != nil {
		return Hvacr03Config{}, err
	}
	if err := optDuration(opts, "report_interval", &cfg.ReportInterval); err != nil {
		return Hvacr03Config{}, err
	}
	if err := optFloat(opts, "event_temp_threshold", &cfg.EventTempThreshold); err != nil {
		return Hvacr03Config{}, err
	}
	if err := optInt(opts, "msg_channel_size", &cfg.MsgChannelSize); err != nil {
		return Hvacr03Config{}, err
	}
	if cfg.MsgChannelSize <= 0 {
		return Hvacr03Config{}, fmt.Errorf("lg_hvacr03: msg_channel_size must be positive, got %d", cfg.MsgChannelSize)
	}

	// devices — 주소는 0~15 정수 문자열이어야 한다. LGCP 의 8자리 hex 주소는 거부한다.
	devices := agent.ParseDevices(opts)
	for i := range devices {
		if _, err := pmbusParseUnitAddr(devices[i].Address, cfg.AddressBase); err != nil {
			return Hvacr03Config{}, fmt.Errorf("lg_hvacr03: device[%d]: %w", i, err)
		}
	}
	cfg.Devices = devices

	// device_type — agent.ParseDevices 가 인식하지 않는 확장 키이므로 직접 읽는다.
	deviceTypes, err := parseHvacr03DeviceTypes(opts, cfg.AddressBase)
	if err != nil {
		return Hvacr03Config{}, err
	}
	cfg.DeviceTypes = deviceTypes

	// --- 제어 ---
	if err := optBool(opts, "control_enabled", &cfg.ControlEnabled); err != nil {
		return Hvacr03Config{}, err
	}
	if err := optDuration(opts, "control_verify_delay", &cfg.ControlVerifyDelay); err != nil {
		return Hvacr03Config{}, err
	}

	// --- 로그 ---
	if err := optBool(opts, "log_messages", &cfg.LogMessages); err != nil {
		return Hvacr03Config{}, err
	}
	if err := optBool(opts, "log_state_updates", &cfg.LogStateUpdates); err != nil {
		return Hvacr03Config{}, err
	}
	if err := optBool(opts, "log_decode_errors", &cfg.LogDecodeErrors); err != nil {
		return Hvacr03Config{}, err
	}
	if err := optBool(opts, "log_drops", &cfg.LogDrops); err != nil {
		return Hvacr03Config{}, err
	}

	return cfg, nil
}

// parseHvacr03DeviceTypes 는 devices 항목의 device_type 키를 읽어 주소별 기기 종류
// 매핑을 만든다. 키가 없는 항목은 매핑에 넣지 않으며, 런타임에 기본 종류로 등록된다.
func parseHvacr03DeviceTypes(opts map[string]any, addressBase int) (map[string]string, error) {
	v, ok := opts["devices"]
	if !ok || v == nil {
		return nil, nil
	}
	items, ok := v.([]any)
	if !ok {
		return nil, nil // agent.ParseDevices 와 동일하게 관대하게 무시
	}

	var result map[string]string
	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		raw, ok := m["device_type"]
		if !ok || raw == nil {
			continue
		}
		dt, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("lg_hvacr03: device[%d].device_type must be a string, got %T", i, raw)
		}
		if !isValidPmbusDeviceType(dt) {
			return nil, fmt.Errorf("lg_hvacr03: device[%d].device_type %q is not one of %s/%s/%s",
				i, dt, pmbusDeviceTypeIDU, pmbusDeviceTypeERV, pmbusDeviceTypeAWHP)
		}
		addrRaw, ok := m["address"]
		if !ok {
			continue
		}
		addr := fmt.Sprintf("%v", addrRaw)
		if _, err := pmbusParseUnitAddr(addr, addressBase); err != nil {
			return nil, fmt.Errorf("lg_hvacr03: device[%d]: %w", i, err)
		}
		if result == nil {
			result = make(map[string]string)
		}
		result[addr] = dt
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// 타입 안전 옵션 파싱 헬퍼
//
// 키가 없으면 dst 를 건드리지 않는다(기본값 보존). 키가 있으나 타입이 어긋나면
// 설정 오류를 반환한다 — 무검사 단언으로 panic 하지 않는다.
// ---------------------------------------------------------------------------

// optString 은 문자열 옵션을 읽는다.
func optString(opts map[string]any, key string, dst *string) error {
	v, ok := opts[key]
	if !ok || v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("lg_hvacr03: %s must be a string, got %T", key, v)
	}
	*dst = s
	return nil
}

// optInt 는 정수 옵션을 읽는다. YAML 로더가 int / int64 / float64 중 무엇을
// 돌려주든 받아들이되, 소수부가 있는 실수는 거부한다.
func optInt(opts map[string]any, key string, dst *int) error {
	v, ok := opts[key]
	if !ok || v == nil {
		return nil
	}
	switch n := v.(type) {
	case int:
		*dst = n
	case int32:
		*dst = int(n)
	case int64:
		*dst = int(n)
	case float64:
		if n != float64(int(n)) {
			return fmt.Errorf("lg_hvacr03: %s must be an integer, got %v", key, n)
		}
		*dst = int(n)
	case float32:
		if n != float32(int(n)) {
			return fmt.Errorf("lg_hvacr03: %s must be an integer, got %v", key, n)
		}
		*dst = int(n)
	default:
		return fmt.Errorf("lg_hvacr03: %s must be an integer, got %T", key, v)
	}
	return nil
}

// optFloat 는 실수 옵션을 읽는다.
func optFloat(opts map[string]any, key string, dst *float64) error {
	v, ok := opts[key]
	if !ok || v == nil {
		return nil
	}
	switch n := v.(type) {
	case float64:
		*dst = n
	case float32:
		*dst = float64(n)
	case int:
		*dst = float64(n)
	case int64:
		*dst = float64(n)
	default:
		return fmt.Errorf("lg_hvacr03: %s must be a number, got %T", key, v)
	}
	return nil
}

// optBool 은 불리언 옵션을 읽는다.
func optBool(opts map[string]any, key string, dst *bool) error {
	v, ok := opts[key]
	if !ok || v == nil {
		return nil
	}
	b, ok := v.(bool)
	if !ok {
		return fmt.Errorf("lg_hvacr03: %s must be a boolean, got %T", key, v)
	}
	*dst = b
	return nil
}

// optDuration 은 duration 문자열 옵션을 읽는다 (예: "10s", "5m").
func optDuration(opts map[string]any, key string, dst *time.Duration) error {
	v, ok := opts[key]
	if !ok || v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("lg_hvacr03: %s must be a duration string, got %T", key, v)
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("lg_hvacr03: invalid %s: %w", key, err)
	}
	if d <= 0 {
		return fmt.Errorf("lg_hvacr03: %s must be positive, got %s", key, d)
	}
	*dst = d
	return nil
}
