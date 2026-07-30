package xsfm

import (
	"fmt"
	"time"
)

// TransportMode 값 상수.
const (
	transportModeDirect = "direct"
	transportModePort   = "port"
)

// fan_speed_power_off_policy 값 상수 (전원 OFF 상태에서 풍량 명령 처리 정책, REQ-XSFM-001-03-05).
const (
	// fanSpeedPolicyReject 은 전원 OFF 디바이스의 풍량 명령을 ErrPowerOff 로 거부한다 (기본).
	fanSpeedPolicyReject = "reject"
	// fanSpeedPolicyPowerOnFirst 은 전원 OFF 디바이스에 전원 ON 을 먼저 방출한 뒤 풍량을 방출한다.
	fanSpeedPolicyPowerOnFirst = "power_on_first"
)

// state_emit_mode 값 상수 (상태 방출 모드, SPEC-XSFM-AGENT-IO-001 RD-2).
const (
	// stateEmitModeEvent 은 현행 on-change device_state_changed 방출이다 (기본, 무회귀).
	// 주기 방출기를 기동하지 않는다.
	stateEmitModeEvent = "event"
	// stateEmitModeInterval 은 state_emit_interval 주기로 전체 디바이스 풀 스냅샷
	// (device_state_snapshot)을 방출하고 on-change device_state_changed 는 억제한다.
	stateEmitModeInterval = "interval"
	// stateEmitModeBoth 은 on-change device_state_changed 와 주기 풀 스냅샷 heartbeat 를 병행 방출한다.
	stateEmitModeBoth = "both"
)

// liveness_source 값 상수 (오프라인 감지의 생존 시각 소스).
const (
	// livenessSourceReceive 는 LastSeen(생존 판정)을 에이전트 수신 시각으로 설정한다 (기본, 하위호환).
	// 디바이스 시계 오차에 영향받지 않아 offline 판정이 안정적이다.
	livenessSourceReceive = "receive"
	// livenessSourcePayload 는 디바이스 보고 시각(time_field 엔벨로프)이 있으면 그 값을 LastSeen 으로
	// 설정한다. offline 판정이 디바이스 보고 시각 기준이 되어 네트워크 지연이 아닌 실제 디바이스
	// 관측 시각을 반영하지만, 디바이스 시계 스큐가 판정에 직접 영향을 준다(아래 주의 참조).
	livenessSourcePayload = "payload"
)

// ConfigDevice 는 설정에서 선언된 디바이스 시드 항목이다 (REQ-XSFM-001-02-03).
// device_id 필수, 나머지는 선택. 로스터 Device 로 확장되어 Source="config" 로 등록된다.
type ConfigDevice struct {
	DeviceID string
	Name     string
	GroupID  string
	Station  string
	Place    string
	Index    int
}

// StationSeed 는 역사 레지스트리 시드 항목이다 (REQ-XSFM-001-02-10).
// B1 에서는 파싱·저장만 하고 실제 lookup/CRUD 배선은 후속 배치에서 이루어진다.
type StationSeed struct {
	Station     string
	Line        string
	DisplayName string
	Order       int
}

// XSFMConfig 는 설비 에이전트 설정이다 (REQ-XSFM-001-01-04).
// AgentConfig.Transport.Options 맵에서 파싱된다 (thingplus_agent.go 패턴 준수).
type XSFMConfig struct {
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

	// 토픽 템플릿 (direct 모드 필수, 최소 1개 {placeholder} 포함).
	StateTopicTemplate   string
	CommandTopicTemplate string

	// stateHasAttribute / commandHasAttribute 는 각 템플릿에 {attribute} placeholder 가
	// 있는지의 파생 플래그이다 (attribute-per-topic 디코드/인코드 모드 디스패치, M14).
	// stateHasAttribute 는 상태 유입 디코드·구독을, commandHasAttribute 는 제어 방출을 결정한다.
	stateHasAttribute   bool
	commandHasAttribute bool

	// commandHasLineCode 는 명령 템플릿에 {line_code} placeholder 가 있는지의 파생 플래그이다
	// (SPEC-XSFM-LINE-001 RD-8, REQ-08-01/02). true 이면 outbound 렌더 시 디바이스의 파생 라인
	// (ResolveLine(station))을 roster 락 밖에서 해석해 {line_code} 로 주입한다(controlDevice). false
	// 이면(대부분의 기존 템플릿·port 모드) 라인 해석 자체를 건너뛰어 무회귀이다.
	commandHasLineCode bool

	// stateIsComposite 는 상태 템플릿이 합성 주소 모델(station/place/index 다중 필드)인지의
	// 파생 플래그이다. true 이면 유입 상태를 보조 인덱스(compositeKey → device_id/UUID)로 조회하고
	// 미등록 시 UUID 를 생성해 auto 등록한다. false({device_id} 단일 필드 = blob 모델)이면 topic 의
	// device_id 를 로스터 키로 직접 조회한다(하위호환).
	stateIsComposite bool

	// PayloadMapping 은 설정 주도 페이로드 시임이다 (양 모드 공통 필수).
	PayloadMapping PayloadMapping

	// 모니터링/제어 타임아웃.
	OfflineTimeout         time.Duration
	ControlResponseTimeout time.Duration
	LWTEnabled             bool

	// FanSpeedPowerOffPolicy 는 전원 OFF 디바이스의 풍량 명령 처리 정책이다 (REQ-XSFM-001-03-05).
	// "reject"(기본, ErrPowerOff 거부) | "power_on_first"(전원 ON 선방출 후 풍량).
	FanSpeedPowerOffPolicy string

	// LivenessSource 는 오프라인 감지(LastSeen/staleness)가 사용할 생존 시각 소스이다 (json "liveness_source").
	// "receive"(기본, 하위호환): LastSeen = 에이전트 수신 시각. "payload": 디바이스 보고 시각(time_field
	// 엔벨로프)이 있으면 그 값을 LastSeen 으로 사용해 offline 판정이 디바이스 시각 기준이 된다.
	//
	// 주의(스큐 경고): "payload" 모드에서는 디바이스 시계 오차(clock skew)가 offline 판정에 직접 영향을
	// 준다 — 디바이스 시계가 실제보다 뒤처지면 살아있어도 stale 로 오판할 수 있고, 앞서면 오프라인
	// 전환이 지연될 수 있다. 방출 메시지 timestamp 는 이 옵션과 무관하게 time_field 설정 시 항상
	// 디바이스 시각을 쓴다(불변). 기본 "receive" 는 스큐에 영향받지 않는다.
	LivenessSource string

	// LogMessages 는 송/수신(RX/TX) 프레임 로그 여부이다 (기본 false, opt-in 진단용).
	// true 이면 각 상태 유입(RX: 토픽+페이로드+디코드 축)과 명령 방출(TX: 토픽+페이로드+축)을
	// INFO 로 로그한다. thingplus/samsung 의 log_messages 옵션과 동형이다.
	// 주의: 진단용이며 로그 볼륨이 크므로 운영에서는 꺼둘 것.
	LogMessages bool

	// LogMQTT 는 MQTT 클라이언트 생명주기 로그 여부이다 (기본 false, opt-in 진단용).
	// true 이면 연결 성공/해제·구독·발행 등 브로커 생명주기를 INFO 로 로그한다. direct 모드에서만
	// 의미가 있으며(port 모드는 브로커가 없어 no-op), 진단용이므로 운영에서는 꺼둘 것.
	LogMQTT bool

	// 영속화/레지스트리 경로.
	//
	// 의미(자동 기본값): 빈 값이면 서버 기본 디렉터리(<dataDir>/xsfm/<agentID>/)를
	// 자동으로 사용해 영속화가 기본 ON 이다. 명시적으로 경로를 지정하면 그 경로가 우선한다
	// (설정 경로 > 기본 경로 > 인메모리). 서버 기본 디렉터리 자체가 미설정인 경우(단위 테스트/
	// 임베딩 등 SetDefaultRegistryDir 미호출)에만 빈 값 = 영속화 비활성(인메모리)로 남는다.
	//   - RegistryPath        : 디바이스 로스터(device_registry.json) 저장 디렉터리.
	//   - StationRegistryPath : 역사/위치 레지스트리(station_registry.json) 저장 디렉터리.
	RegistryPath        string
	StationRegistryPath string
	StationRegistry     []StationSeed

	// Devices 는 설정 기반 디바이스 시드이다.
	Devices []ConfigDevice

	// ForwardReceivedToNode 는 수신된 모든 파싱 상태를 노드로 전달(device_state_received)할지의
	// 패스스루 탭 옵션이다 (기본 false, SPEC-XSFM-AGENT-IO-001 RD-1). state_emit_mode 와 독립적으로
	// 매 수신마다 방출되며, direct·port 양 모드에 동일 적용된다(mode-agnostic, RD-4).
	ForwardReceivedToNode bool

	// StateEmitMode 는 상태 방출 모드이다 (RD-2): "event"(기본, on-change) | "interval"(주기 풀
	// 스냅샷, on-change 억제) | "both"(on-change + 주기 스냅샷 heartbeat).
	StateEmitMode string

	// StateEmitInterval 은 interval/both 모드의 주기 스냅샷 방출 주기이다 (기본 60s, RD-3).
	// event 모드에서는 무시된다. <=0 이면 주기 방출기 미기동(3-way 비활성).
	StateEmitInterval time.Duration
}

// @MX:ANCHOR: XSFMConfig 파싱·검증의 단일 진입점 — 기본값·enum 검증 계약을 보존한다.
// @MX:REASON: NewXSFMAgent/Init + 다수 테스트가 호출한다(fan_in>=3). 기본 설정(forward off,
// state_emit_mode=event, interval=60s)은 현행 방출 동작과 바이트 동일해야 하며(무회귀), enum 무효값은
// 조용한 폴백 없이 센티널 에러로 거부한다(transport_mode/liveness_source/state_emit_mode 동형).
//
// parseXSFMConfig 는 Transport.Options 맵에서 XSFMConfig 를 파싱·검증한다
// (REQ-XSFM-001-01-04/05/06/10/13, SPEC-XSFM-AGENT-IO-001 RD-1/RD-2/RD-3).
func parseXSFMConfig(opts map[string]any) (XSFMConfig, error) {
	cfg := XSFMConfig{
		TransportMode:          transportModeDirect,
		QoS:                    1,
		KeepAlive:              30 * time.Second,
		ConnectTimeout:         5 * time.Second,
		AutoReconnect:          true,
		OfflineTimeout:         90 * time.Second,
		ControlResponseTimeout: 5 * time.Second,
		LWTEnabled:             true,
		FanSpeedPowerOffPolicy: fanSpeedPolicyReject,
		LivenessSource:         livenessSourceReceive,
		StateEmitMode:          stateEmitModeEvent,
		StateEmitInterval:      60 * time.Second,
	}

	// transport_mode (기본 "direct", enum {direct, port}).
	if v, ok := opts["transport_mode"]; ok {
		s, sok := v.(string)
		if !sok {
			return XSFMConfig{}, fmt.Errorf("%w: transport_mode must be a string", ErrInvalidTransportMode)
		}
		if s != "" {
			cfg.TransportMode = s
		}
	}
	switch cfg.TransportMode {
	case transportModeDirect, transportModePort:
		// valid
	default:
		return XSFMConfig{}, fmt.Errorf("%w: got %q", ErrInvalidTransportMode, cfg.TransportMode)
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
	// attribute-per-topic 모드 파생 플래그 (M14): 템플릿에 {attribute} 가 있으면 축별
	// 스칼라 디코드/인코드, 없으면 기존 JSON-blob 경로(하위호환).
	cfg.stateHasAttribute = templateHasAttribute(cfg.StateTopicTemplate)
	cfg.commandHasAttribute = templateHasAttribute(cfg.CommandTopicTemplate)
	// {line_code} 파생 플래그(SPEC-XSFM-LINE-001 RD-8, REQ-08-01): 명령 템플릿에 {line_code} 가
	// 있으면 outbound 렌더가 파생 라인을 주입한다(controlDevice). 없으면 라인 해석을 건너뛴다(무회귀).
	cfg.commandHasLineCode = templateHasLineCode(cfg.CommandTopicTemplate)
	// 합성 주소 모델 판별(M14): 상태 템플릿의 비-attribute placeholder 가 {device_id} 단독(또는
	// placeholder 없음)이면 blob 모델, 그 외(station/place/index 등)면 합성 주소 모델이다.
	cfg.stateIsComposite = templateIsComposite(cfg.StateTopicTemplate)

	// payload_mapping (양 모드 공통 필수).
	mapping, err := parsePayloadMapping(opts)
	if err != nil {
		return XSFMConfig{}, err
	}
	cfg.PayloadMapping = mapping

	// 타임아웃 (time.Duration 문자열, 0 허용).
	if d, ok, err := parseDurationOpt(opts, "offline_timeout"); err != nil {
		return XSFMConfig{}, err
	} else if ok {
		cfg.OfflineTimeout = d
	}
	if d, ok, err := parseDurationOpt(opts, "control_response_timeout"); err != nil {
		return XSFMConfig{}, err
	} else if ok {
		cfg.ControlResponseTimeout = d
	}
	if v, ok := opts["connect_timeout"]; ok {
		if s, ok := v.(string); ok {
			d, err := time.ParseDuration(s)
			if err != nil {
				return XSFMConfig{}, fmt.Errorf("xsfm: invalid connect_timeout: %w", err)
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

	// liveness_source (기본 "receive", enum {receive, payload}). 명시적 무효값은 에러로 거부한다
	// (transport_mode 검증 패턴과 동형 — 조용한 폴백보다 설정 오타를 조기에 드러낸다). 빈 문자열은
	// 미지정으로 보아 기본값을 유지한다.
	if v, ok := opts["liveness_source"]; ok {
		s, sok := v.(string)
		if !sok {
			return XSFMConfig{}, fmt.Errorf("%w: liveness_source must be a string", ErrInvalidLivenessSource)
		}
		if s != "" {
			switch s {
			case livenessSourceReceive, livenessSourcePayload:
				cfg.LivenessSource = s
			default:
				return XSFMConfig{}, fmt.Errorf("%w: got %q", ErrInvalidLivenessSource, s)
			}
		}
	}

	// 진단 로그 토글 (기본 false, opt-in). log_messages: 송/수신 프레임 로그,
	// log_mqtt: MQTT 생명주기 로그. 둘 다 독립적이며 운영에서는 꺼두는 것을 권장한다.
	if v, ok := opts["log_messages"]; ok {
		cfg.LogMessages = toBool(v)
	}
	if v, ok := opts["log_mqtt"]; ok {
		cfg.LogMQTT = toBool(v)
	}

	// 수신-전달 옵션 + 상태 방출 모드 (SPEC-XSFM-AGENT-IO-001 RD-1/RD-2/RD-3).
	// forward_received_to_node (bool, 기본 false): ON 시 수신된 모든 파싱 상태를 device_state_received
	// 로 노드에 전달한다(패스스루 탭, state_emit_mode 와 독립).
	if v, ok := opts["forward_received_to_node"]; ok {
		cfg.ForwardReceivedToNode = toBool(v)
	}
	// state_emit_mode (기본 "event", enum {event, interval, both}). 명시적 무효값은 에러로 거부한다
	// (transport_mode/liveness_source 검증 패턴과 동형 — 조용한 폴백보다 설정 오타를 조기에 드러낸다).
	// 빈 문자열은 미지정으로 보아 기본값을 유지한다.
	if v, ok := opts["state_emit_mode"]; ok {
		s, sok := v.(string)
		if !sok {
			return XSFMConfig{}, fmt.Errorf("%w: state_emit_mode must be a string", ErrInvalidStateEmitMode)
		}
		if s != "" {
			cfg.StateEmitMode = s
		}
	}
	switch cfg.StateEmitMode {
	case stateEmitModeEvent, stateEmitModeInterval, stateEmitModeBoth:
		// valid
	default:
		return XSFMConfig{}, fmt.Errorf("%w: got %q", ErrInvalidStateEmitMode, cfg.StateEmitMode)
	}
	// state_emit_interval (기본 60s, RD-3; parseDurationOpt 규약 계승 — 음수 거부, 0 허용).
	if d, ok, err := parseDurationOpt(opts, "state_emit_interval"); err != nil {
		return XSFMConfig{}, err
	} else if ok {
		cfg.StateEmitInterval = d
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

	// 모드별 검증 (REQ-XSFM-001-01-13).
	if cfg.TransportMode == transportModeDirect {
		if cfg.Broker == "" {
			return XSFMConfig{}, ErrBrokerRequired
		}
		if err := validateTopicTemplate(cfg.StateTopicTemplate); err != nil {
			return XSFMConfig{}, err
		}
		if err := validateTopicTemplate(cfg.CommandTopicTemplate); err != nil {
			return XSFMConfig{}, err
		}
	}

	return cfg, nil
}

// validateTopicTemplate 은 토픽 템플릿에 최소 1개의 {placeholder} 세그먼트가 있는지 검증한다
// (M14: {device_id} 를 강제하지 않는다 — 다중 필드 템플릿 허용). placeholder 가 하나도 없으면
// 와일드카드 구독/합성 키를 만들 수 없으므로 거부한다.
func validateTopicTemplate(tmpl string) error {
	if len(placeholderNames(tmpl)) == 0 {
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
	// value_field / time_field (선택, attribute-per-topic 엔벨로프 추출). 둘 다 빈 값이면 비활성 —
	// 페이로드 전체를 원시 스칼라로 보는 기존 동작을 유지한다(하위호환). 예: {"time":<ts>,"value":1}
	// 페이로드에 value_field="value", time_field="time".
	mapping.ValueField = stringField(m, "value_field")
	mapping.TimeField = stringField(m, "time_field")
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
		// device_id 또는 위치 계층(station/place/index) 중 하나라도 있으면 시드로 수용한다.
		// 다중 필드 모델(M14)에서는 device_id 없이 station/place/index 만으로 키가 합성된다.
		if d.DeviceID != "" || d.Station != "" || d.Place != "" || d.Index != 0 {
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
		return 0, false, fmt.Errorf("xsfm: %s must be a duration string", key)
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, false, fmt.Errorf("xsfm: invalid %s: %w", key, err)
	}
	if d < 0 {
		return 0, false, fmt.Errorf("xsfm: %s must be >= 0, got %s", key, d)
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
