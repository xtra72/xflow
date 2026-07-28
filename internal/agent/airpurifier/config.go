package airpurifier

import (
	"fmt"
	"strings"
	"time"
)

// TransportMode 값 상수.
const (
	transportModeDirect = "direct"
	transportModePort   = "port"
)

// fan_speed_power_off_policy 값 상수 (전원 OFF 상태에서 풍량 명령 처리 정책, REQ-AIRPUR-001-03-05).
const (
	// fanSpeedPolicyReject 은 전원 OFF 디바이스의 풍량 명령을 ErrPowerOff 로 거부한다 (기본).
	fanSpeedPolicyReject = "reject"
	// fanSpeedPolicyPowerOnFirst 은 전원 OFF 디바이스에 전원 ON 을 먼저 방출한 뒤 풍량을 방출한다.
	fanSpeedPolicyPowerOnFirst = "power_on_first"
)

// ConfigDevice 는 설정에서 선언된 디바이스 시드 항목이다 (REQ-AIRPUR-001-02-03).
// device_id 필수, 나머지는 선택. 로스터 Device 로 확장되어 Source="config" 로 등록된다.
type ConfigDevice struct {
	DeviceID string
	Name     string
	GroupID  string
	Station  string
	Place    string
	Index    int
}

// StationSeed 는 역사 레지스트리 시드 항목이다 (REQ-AIRPUR-001-02-10).
// B1 에서는 파싱·저장만 하고 실제 lookup/CRUD 배선은 후속 배치에서 이루어진다.
type StationSeed struct {
	Station     string
	Line        string
	DisplayName string
	Order       int
}

// AirPurifierConfig 는 공기청정기 에이전트 설정이다 (REQ-AIRPUR-001-01-04).
// AgentConfig.Transport.Options 맵에서 파싱된다 (thingplus_agent.go 패턴 준수).
type AirPurifierConfig struct {
	// TransportMode 는 I/O 경계 선택이다: "direct"(에이전트가 브로커 소유) | "port"(외부 노드 I/O).
	TransportMode string

	// 브로커 설정 (direct 모드).
	Broker   string
	TLS      bool
	CACert   string
	ClientID string
	Username string
	Password string
	QoS      byte

	// 브로커 연결 파라미터 (direct 모드, 내부 기본값 보유).
	KeepAlive      time.Duration
	ConnectTimeout time.Duration
	AutoReconnect  bool

	// 토픽 템플릿 (direct 모드 필수, {device_id} placeholder 포함).
	StateTopicTemplate   string
	CommandTopicTemplate string

	// PayloadMapping 은 설정 주도 페이로드 시임이다 (양 모드 공통 필수).
	PayloadMapping PayloadMapping

	// 모니터링/제어 타임아웃.
	OfflineTimeout         time.Duration
	ControlResponseTimeout time.Duration
	LWTEnabled             bool

	// FanSpeedPowerOffPolicy 는 전원 OFF 디바이스의 풍량 명령 처리 정책이다 (REQ-AIRPUR-001-03-05).
	// "reject"(기본, ErrPowerOff 거부) | "power_on_first"(전원 ON 선방출 후 풍량).
	FanSpeedPowerOffPolicy string

	// 영속화/레지스트리 경로 (후속 배치에서 배선; B1 은 파싱·저장만).
	RegistryPath        string
	StationRegistryPath string
	StationRegistry     []StationSeed

	// Devices 는 설정 기반 디바이스 시드이다.
	Devices []ConfigDevice
}

// parseAirPurifierConfig 는 Transport.Options 맵에서 AirPurifierConfig 를 파싱·검증한다
// (REQ-AIRPUR-001-01-04/05/06/10/13).
func parseAirPurifierConfig(opts map[string]any) (AirPurifierConfig, error) {
	cfg := AirPurifierConfig{
		TransportMode:          transportModeDirect,
		QoS:                    1,
		KeepAlive:              30 * time.Second,
		ConnectTimeout:         5 * time.Second,
		AutoReconnect:          true,
		OfflineTimeout:         60 * time.Second,
		ControlResponseTimeout: 5 * time.Second,
		LWTEnabled:             true,
		FanSpeedPowerOffPolicy: fanSpeedPolicyReject,
	}

	// transport_mode (기본 "direct", enum {direct, port}).
	if v, ok := opts["transport_mode"]; ok {
		s, sok := v.(string)
		if !sok {
			return AirPurifierConfig{}, fmt.Errorf("%w: transport_mode must be a string", ErrInvalidTransportMode)
		}
		if s != "" {
			cfg.TransportMode = s
		}
	}
	switch cfg.TransportMode {
	case transportModeDirect, transportModePort:
		// valid
	default:
		return AirPurifierConfig{}, fmt.Errorf("%w: got %q", ErrInvalidTransportMode, cfg.TransportMode)
	}

	// 브로커 설정 (모드 무관하게 파싱; direct 모드에서만 검증).
	if v, ok := opts["broker"]; ok {
		if s, ok := v.(string); ok {
			cfg.Broker = s
		}
	}
	if v, ok := opts["tls"]; ok {
		cfg.TLS = toBool(v)
	}
	if v, ok := opts["ca_cert"]; ok {
		if s, ok := v.(string); ok {
			cfg.CACert = s
		}
	}
	if v, ok := opts["client_id"]; ok {
		if s, ok := v.(string); ok {
			cfg.ClientID = s
		}
	}
	if v, ok := opts["username"]; ok {
		if s, ok := v.(string); ok {
			cfg.Username = s
		}
	}
	if v, ok := opts["password"]; ok {
		if s, ok := v.(string); ok {
			cfg.Password = s
		}
	}
	if v, ok := opts["qos"]; ok {
		cfg.QoS = byte(toInt(v))
	}
	if v, ok := opts["keep_alive_sec"]; ok {
		cfg.KeepAlive = time.Duration(toInt(v)) * time.Second
	}
	if v, ok := opts["auto_reconnect"]; ok {
		cfg.AutoReconnect = toBool(v)
	}

	// 토픽 템플릿.
	if v, ok := opts["state_topic_template"]; ok {
		if s, ok := v.(string); ok {
			cfg.StateTopicTemplate = s
		}
	}
	if v, ok := opts["command_topic_template"]; ok {
		if s, ok := v.(string); ok {
			cfg.CommandTopicTemplate = s
		}
	}

	// payload_mapping (양 모드 공통 필수).
	mapping, err := parsePayloadMapping(opts)
	if err != nil {
		return AirPurifierConfig{}, err
	}
	cfg.PayloadMapping = mapping

	// 타임아웃 (time.Duration 문자열, 0 허용).
	if d, ok, err := parseDurationOpt(opts, "offline_timeout"); err != nil {
		return AirPurifierConfig{}, err
	} else if ok {
		cfg.OfflineTimeout = d
	}
	if d, ok, err := parseDurationOpt(opts, "control_response_timeout"); err != nil {
		return AirPurifierConfig{}, err
	} else if ok {
		cfg.ControlResponseTimeout = d
	}
	if v, ok := opts["connect_timeout"]; ok {
		if s, ok := v.(string); ok {
			d, err := time.ParseDuration(s)
			if err != nil {
				return AirPurifierConfig{}, fmt.Errorf("airpurifier: invalid connect_timeout: %w", err)
			}
			cfg.ConnectTimeout = d
		}
	}
	if v, ok := opts["lwt_enabled"]; ok {
		cfg.LWTEnabled = toBool(v)
	}
	if v, ok := opts["fan_speed_power_off_policy"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.FanSpeedPowerOffPolicy = s
		}
	}

	// 영속화/레지스트리 경로.
	if v, ok := opts["registry_path"]; ok {
		if s, ok := v.(string); ok {
			cfg.RegistryPath = s
		}
	}
	if v, ok := opts["station_registry_path"]; ok {
		if s, ok := v.(string); ok {
			cfg.StationRegistryPath = s
		}
	}
	cfg.StationRegistry = parseStationRegistry(opts)

	// 설정 기반 디바이스 시드.
	cfg.Devices = parseConfigDevices(opts)

	// 모드별 검증 (REQ-AIRPUR-001-01-13).
	if cfg.TransportMode == transportModeDirect {
		if cfg.Broker == "" {
			return AirPurifierConfig{}, ErrBrokerRequired
		}
		if err := validateTopicTemplate(cfg.StateTopicTemplate); err != nil {
			return AirPurifierConfig{}, err
		}
		if err := validateTopicTemplate(cfg.CommandTopicTemplate); err != nil {
			return AirPurifierConfig{}, err
		}
	}

	return cfg, nil
}

// validateTopicTemplate 은 토픽 템플릿에 {device_id} placeholder 가 있는지 검증한다.
func validateTopicTemplate(tmpl string) error {
	if !strings.Contains(tmpl, deviceIDPlaceholder) {
		return fmt.Errorf("%w: got %q", ErrInvalidTopicTemplate, tmpl)
	}
	return nil
}

// parsePayloadMapping 은 opts["payload_mapping"] 을 PayloadMapping 으로 파싱한다.
//
// power_field / fan_speed_field 는 문자열(필드명, native 표현) 또는 객체(name + 값 매핑)
// 두 형태를 모두 수용한다. 누락 시 ErrInvalidPayloadMapping.
func parsePayloadMapping(opts map[string]any) (PayloadMapping, error) {
	v, ok := opts["payload_mapping"]
	if !ok {
		return PayloadMapping{}, ErrInvalidPayloadMapping
	}
	m, ok := v.(map[string]any)
	if !ok {
		return PayloadMapping{}, ErrInvalidPayloadMapping
	}

	power, ok := parseBoolField(m["power_field"])
	if !ok || power.Name == "" {
		return PayloadMapping{}, ErrInvalidPayloadMapping
	}
	fan, ok := parseIntField(m["fan_speed_field"])
	if !ok || fan.Name == "" {
		return PayloadMapping{}, ErrInvalidPayloadMapping
	}

	mapping := PayloadMapping{Power: power, FanSpeed: fan}
	if online, ok := parseBoolField(m["online_field"]); ok && online.Name != "" {
		mapping.Online = &online
	}
	return mapping, nil
}

// parseBoolField 는 boolean 축 필드 정의를 파싱한다.
// 문자열: 필드명(native bool). 객체: {name, on/on_value, off/off_value}.
func parseBoolField(v any) (BoolField, bool) {
	switch t := v.(type) {
	case nil:
		return BoolField{}, false
	case string:
		return BoolField{Name: t}, true
	case map[string]any:
		f := BoolField{Name: stringField(t, "name", "field")}
		f.OnValue = firstPresent(t, "on_value", "on")
		f.OffValue = firstPresent(t, "off_value", "off")
		return f, true
	default:
		return BoolField{}, false
	}
}

// parseIntField 는 정수 축 필드 정의를 파싱한다.
// 문자열: 필드명(native int). 객체: {name, values:{"1":x,"2":y,"3":z}}.
func parseIntField(v any) (IntField, bool) {
	switch t := v.(type) {
	case nil:
		return IntField{}, false
	case string:
		return IntField{Name: t}, true
	case map[string]any:
		f := IntField{Name: stringField(t, "name", "field")}
		if vals, ok := t["values"].(map[string]any); ok && len(vals) > 0 {
			f.Values = make(map[int]any, len(vals))
			for k, wire := range vals {
				f.Values[toInt(k)] = wire
			}
		}
		return f, true
	default:
		return IntField{}, false
	}
}

// parseConfigDevices 는 opts["devices"] 배열을 ConfigDevice 슬라이스로 파싱한다.
func parseConfigDevices(opts map[string]any) []ConfigDevice {
	v, ok := opts["devices"]
	if !ok {
		return nil
	}
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	var devices []ConfigDevice
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		d := ConfigDevice{
			DeviceID: stringField(m, "device_id", "id"),
			Name:     stringField(m, "name"),
			GroupID:  stringField(m, "group_id"),
			Station:  stringField(m, "station"),
			Place:    stringField(m, "place"),
		}
		if idx, ok := m["index"]; ok {
			d.Index = toInt(idx)
		}
		if d.DeviceID != "" {
			devices = append(devices, d)
		}
	}
	return devices
}

// parseStationRegistry 는 opts["station_registry"] 시드를 파싱한다 (파싱·저장만; B1).
// 형태: {"<station>": {line, display_name, order}} 또는 [{station, line, ...}].
func parseStationRegistry(opts map[string]any) []StationSeed {
	v, ok := opts["station_registry"]
	if !ok {
		return nil
	}
	var seeds []StationSeed
	switch t := v.(type) {
	case map[string]any:
		for station, raw := range t {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			seeds = append(seeds, StationSeed{
				Station:     station,
				Line:        stringField(m, "line"),
				DisplayName: stringField(m, "display_name"),
				Order:       toInt(m["order"]),
			})
		}
	case []any:
		for _, raw := range t {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			station := stringField(m, "station")
			if station == "" {
				continue
			}
			seeds = append(seeds, StationSeed{
				Station:     station,
				Line:        stringField(m, "line"),
				DisplayName: stringField(m, "display_name"),
				Order:       toInt(m["order"]),
			})
		}
	}
	return seeds
}

// parseDurationOpt 는 opts[key] 를 time.Duration 으로 파싱한다.
// 키가 없으면 (0, false, nil). 음수는 에러. 0 은 허용.
func parseDurationOpt(opts map[string]any, key string) (time.Duration, bool, error) {
	v, ok := opts[key]
	if !ok {
		return 0, false, nil
	}
	s, ok := v.(string)
	if !ok {
		return 0, false, fmt.Errorf("airpurifier: %s must be a duration string", key)
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, false, fmt.Errorf("airpurifier: invalid %s: %w", key, err)
	}
	if d < 0 {
		return 0, false, fmt.Errorf("airpurifier: %s must be >= 0, got %s", key, d)
	}
	return d, true, nil
}

// ---------------------------------------------------------------------------
// 값 변환 헬퍼 (YAML/JSON 파싱에서 숫자가 float64 로 전달될 수 있으므로 통일 처리).
// ---------------------------------------------------------------------------

// stringField 는 map 에서 후보 키들을 순서대로 조회해 첫 문자열 값을 반환한다.
func stringField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
			if v != nil {
				return fmt.Sprintf("%v", v)
			}
		}
	}
	return ""
}

// firstPresent 는 map 에서 후보 키들을 순서대로 조회해 첫 존재 값을 반환한다 (any).
func firstPresent(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

// toInt 는 int/float64/string 값을 int 로 변환한다.
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	case string:
		var i int
		_, _ = fmt.Sscanf(n, "%d", &i)
		return i
	default:
		return 0
	}
}

// toBool 는 bool 값을 안전하게 추출한다 (그 외 타입은 false).
func toBool(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}
