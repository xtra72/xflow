package chirpstack

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/xtra/xflow/internal/agent"
)

// defaultChirpStackTopic 는 ChirpStack application 이벤트 기본 구독 토픽이다.
// ChirpStack MQTT integration 은 application/<id>/device/<devEui>/event/<type>
// 형태로 발행하므로 "application/#" 로 전 애플리케이션 이벤트를 포괄한다.
const defaultChirpStackTopic = "application/#"

// measurement 방출 모드 (measurement_emit_mode).
//
//   - per_measurement(기본): 업링크 1건 → measurement 당 1개 메시지 fan-out.
//     REQ-FROZEN-01/02 의 동결된 기본 경로이며 절대 변경되지 않는다.
//   - combined: 업링크 1건 → 모든 measurement 를 payload 최상위 flat 키로 담은
//     메시지 1개. opt-in 이며 기본 경로에 영향을 주지 않는다.
const (
	measurementEmitModePerMeasurement = "per_measurement"
	measurementEmitModeCombined       = "combined"
)

// ErrInvalidMeasurementEmitMode 는 measurement_emit_mode 가 per_measurement/combined
// 이외일 때 반환된다.
//
// xsfm 의 ErrInvalidStateEmitMode 검증 규율과 동형이다 — 조용한 폴백 대신 설정 오타를
// 조기에 드러낸다. 사용자가 combined 를 의도하고 오타를 냈는데 조용히 per_measurement
// 로 돌아가면 "왜 여전히 2개가 오지" 를 디버깅할 단서가 전혀 남지 않는다.
var ErrInvalidMeasurementEmitMode = errors.New("chirpstack: invalid measurement_emit_mode (must be 'per_measurement' or 'combined')")

// 타임스탬프 소스 (timestamp_source).
//
//   - uplink(기본): 업링크 payload 의 time 필드(RFC3339)를 메시지 타임스탬프로 쓴다.
//     REQ-FROZEN-02 / REQ-FROZEN-A 의 동결된 기본 경로이며 절대 변경되지 않는다.
//   - server: 업링크를 수신한 서버 시각을 메시지 타임스탬프로 쓴다. 게이트웨이/디바이스
//     시계가 틀어져 있어도 서버 기준의 일관된 순서를 얻는 opt-in 경로이다.
const (
	timestampSourceUplink = "uplink"
	timestampSourceServer = "server"
)

// ErrInvalidTimestampSource 는 timestamp_source 가 uplink/server 이외일 때 반환된다.
//
// ErrInvalidMeasurementEmitMode 와 동일한 검증 규율이다 — 조용한 폴백 대신 설정 오타를
// 조기에 드러낸다. server 를 의도한 오타가 조용히 uplink 로 돌아가면 "왜 여전히 장비
// 시계 시각이 찍히지" 를 진단할 단서가 전혀 남지 않는다.
var ErrInvalidTimestampSource = errors.New("chirpstack: invalid timestamp_source (must be 'uplink' or 'server')")

// ChirpStackConfig 는 ChirpStack 에이전트의 트랜스포트 설정이다.
//
// system/mqtt_agent.go 의 MQTTConfig 트랜스포트 서브셋을 미러링한다(발행 노브 제외).
// comm-state 노브(emit_comm_state / comm_report_interval / offline_threshold)는
// M5(comm-state) 범위에서 추가된다.
type ChirpStackConfig struct {
	Broker            string   // MQTT 브로커 주소
	ClientID          string   // MQTT 클라이언트 식별자
	Username          string   // 인증 사용자명
	Password          string   // 인증 비밀번호
	Topics            []string // 구독 토픽 (기본 "application/#")
	QoS               byte     // QoS 레벨 (0/1/2)
	KeepAliveSec      int      // 연결 유지 간격(초)
	AutoReconnect     bool     // 자동 재연결 여부
	CleanSession      bool     // 클린 세션 여부
	BufferSize        int      // 수신 버퍼 크기
	ConnectTimeoutSec int      // 연결 타임아웃(초)

	// M5 comm-state 노브 (REQ-FROZEN-03, REQ-M5-01/03/04).
	EmitCommState      bool          // device_state emit 게이트 (기본 false)
	CommReportInterval time.Duration // 주기 report 간격 (0=off, change 는 유지)
	OfflineThreshold   time.Duration // staleness→offline 임계 (기본 300s)

	// MeasurementEmitMode 는 업링크 1건을 몇 개의 메시지로 방출할지 결정한다.
	// "per_measurement"(기본, 동결 경로) | "combined"(opt-in).
	MeasurementEmitMode string

	// TimestampSource 는 메시지 타임스탬프를 어디에서 가져올지 결정한다.
	// "uplink"(기본, 동결 경로: payload 의 time 필드) | "server"(opt-in: 수신 시각).
	TimestampSource string
}

// defaultOfflineThreshold 는 업링크 staleness→offline 판정의 보수적 기본 임계이다.
// LoRaWAN 클래스 A 디바이스 업링크 주기가 디바이스마다 상이하므로 넉넉히 잡는다.
const defaultOfflineThreshold = 300 * time.Second

// parseChirpStackConfig 는 AgentConfig 에서 ChirpStackConfig 를 파싱한다.
// parseMQTTConfig 관용구(기본값 세팅 + Transport.Options 타입 어서션 오버라이드)를
// 재사용한다.
//
// ⚠ 기존 배포에 영향을 주는 동작 변경 (문자열 강제변환 도입)
//
// web UI 는 qos 를 select 위젯의 문자열('0'/'1'/'2')로, topics 를 "쉼표로 구분한"
// 한 줄 문자열로 보낸다. 이전 파서에는 두 헬퍼 모두 문자열 분기가 없어서 사용자가
// 저장한 값이 조용히 버려졌다. 이제 정상 반영되므로, "저장한 적 없는 설정" 이 아니라
// "저장했지만 지금까지 무시되던 설정" 이 재시작 시점에 처음으로 실제 적용된다:
//
//  1. QoS: qos:"1"/"2" 를 고른 에이전트는 지금까지 실제로는 QoS 0 으로 구독해 왔다.
//     재시작 후 비로소 사용자가 고른 QoS 로 구독한다. QoS 1/2 는 브로커 재전송을
//     동반하므로 중복 수신 처리(멱등성)와 브로커 부하가 달라질 수 있다.
//  2. Topics: 쉼표 목록을 입력한 에이전트는 지금까지 기본값 "application/#" 으로
//     구독해 왔다. 재시작 후 입력한 토픽만 구독한다 — 즉 구독 범위가 좁아지며,
//     목록에 빠진 application 의 업링크는 더 이상 수신되지 않는다.
//
// 둘 다 "사용자 의도대로 고쳐진 것" 이지만 관측 가능한 런타임 동작이 바뀌는 변경이다.
// 트랜스포트 노브는 이미 열린 MQTT 연결에 반영되지 않으므로(Configure 주석 참조)
// 변화 시점은 저장 시점이 아니라 다음 에이전트 재시작 시점이다.
func parseChirpStackConfig(cfg agent.AgentConfig) ChirpStackConfig {
	cc := ChirpStackConfig{
		Broker:              "tcp://localhost:1883",
		ClientID:            "xflow-chirpstack-" + uuid.New().String(),
		Topics:              []string{defaultChirpStackTopic},
		QoS:                 1,
		KeepAliveSec:        60,
		AutoReconnect:       true,
		CleanSession:        true,
		BufferSize:          1024,
		ConnectTimeoutSec:   10,
		OfflineThreshold:    defaultOfflineThreshold,
		MeasurementEmitMode: measurementEmitModePerMeasurement,
		TimestampSource:     timestampSourceUplink,
	}

	opts := cfg.Transport.Options
	if opts == nil {
		return cc
	}

	if v, ok := opts["broker"].(string); ok && v != "" {
		cc.Broker = v
	}
	if v, ok := opts["client_id"].(string); ok && v != "" {
		cc.ClientID = v
	}
	if v, ok := opts["username"].(string); ok {
		cc.Username = v
	}
	if v, ok := opts["password"].(string); ok {
		cc.Password = v
	}
	// 아래 숫자/목록 노브는 모두 "강제변환에 성공한 값만 반영" 규칙을 따른다.
	// 해석할 수 없는 값은 위에서 세팅한 기본값을 그대로 남긴다 — 이전에는 toInt 가
	// 실패를 0 으로 뭉개서, qos:"1" 을 고른 사용자가 조용히 QoS 0 으로 동작했다.
	if v, ok := opts["topics"]; ok {
		if topics, ok := coerceStringSlice(v); ok && len(topics) > 0 {
			cc.Topics = topics
		}
	}
	if v, ok := opts["qos"]; ok {
		if n, ok := coerceInt(v); ok {
			cc.QoS = byte(n)
		}
	}
	if v, ok := opts["keep_alive_sec"]; ok {
		if n, ok := coerceInt(v); ok {
			cc.KeepAliveSec = n
		}
	}
	if v, ok := opts["auto_reconnect"].(bool); ok {
		cc.AutoReconnect = v
	}
	if v, ok := opts["clean_session"].(bool); ok {
		cc.CleanSession = v
	}
	if v, ok := opts["buffer_size"]; ok {
		if n, ok := coerceInt(v); ok && n > 0 {
			cc.BufferSize = n
		}
	}
	if v, ok := opts["connect_timeout_sec"]; ok {
		if n, ok := coerceInt(v); ok {
			cc.ConnectTimeoutSec = n
		}
	}

	// M5 comm-state 노브.
	if v, ok := opts["emit_comm_state"].(bool); ok {
		cc.EmitCommState = v
	}
	if v, ok := opts["comm_report_interval"]; ok {
		if d, ok := coerceDuration(v); ok {
			cc.CommReportInterval = d
		}
	}
	if v, ok := opts["offline_threshold"]; ok {
		if d, ok := coerceDuration(v); ok && d > 0 {
			cc.OfflineThreshold = d
		}
	}

	// measurement_emit_mode: 유효값만 반영한다. 무효값은 호출자가
	// validateMeasurementEmitMode 로 이미 거부했거나(생성/재설정 경로) 거부할 것이므로
	// 여기서는 기본값(per_measurement)을 유지한다.
	if v, ok := opts["measurement_emit_mode"].(string); ok {
		switch v {
		case measurementEmitModePerMeasurement, measurementEmitModeCombined:
			cc.MeasurementEmitMode = v
		}
	}

	// timestamp_source: measurement_emit_mode 와 동일한 규약 — 유효값만 반영하고
	// 무효값은 호출자(validateTimestampSource)가 이미 거부했거나 거부할 것이므로
	// 여기서는 기본값(uplink)을 유지한다.
	if v, ok := opts["timestamp_source"].(string); ok {
		switch v {
		case timestampSourceUplink, timestampSourceServer:
			cc.TimestampSource = v
		}
	}

	return cc
}

// validateMeasurementEmitMode 는 measurement_emit_mode 옵션의 타입/enum 을 검증한다.
//
// 키가 없거나 빈 문자열이면 "미지정"으로 보아 기본값(per_measurement)을 허용한다.
// 그 외 무효값은 ErrInvalidMeasurementEmitMode 로 거부한다(조용한 폴백 금지).
func validateMeasurementEmitMode(opts map[string]any) error {
	v, ok := opts["measurement_emit_mode"]
	if !ok {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("%w: 문자열이어야 합니다 (got %T)", ErrInvalidMeasurementEmitMode, v)
	}
	switch s {
	case "", measurementEmitModePerMeasurement, measurementEmitModeCombined:
		return nil
	default:
		return fmt.Errorf("%w: got %q", ErrInvalidMeasurementEmitMode, s)
	}
}

// validateTimestampSource 는 timestamp_source 옵션의 타입/enum 을 검증한다.
//
// validateMeasurementEmitMode 와 동형이다: 키가 없거나 빈 문자열이면 "미지정"으로 보아
// 기본값(uplink)을 허용하고, 그 외 무효값은 ErrInvalidTimestampSource 로 거부한다
// (조용한 폴백 금지).
func validateTimestampSource(opts map[string]any) error {
	v, ok := opts["timestamp_source"]
	if !ok {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("%w: 문자열이어야 합니다 (got %T)", ErrInvalidTimestampSource, v)
	}
	switch s {
	case "", timestampSourceUplink, timestampSourceServer:
		return nil
	default:
		return fmt.Errorf("%w: got %q", ErrInvalidTimestampSource, s)
	}
}

// parseChirpStackConfigStrict 는 런타임 재설정(Configure) 경로용 파서이다.
//
// parseChirpStackConfig 는 강제변환에 실패한 옵션을 조용히 무시하고 기본값을 남긴다.
// 생성 경로에서는 그 관용(lenient) 동작을 그대로 보존하지만, 런타임 재설정에서는
// 사용자가 방금 저장한 값이 조용히 사라지는 것이 곧 결함이므로 에러로 거부한다.
// 호출자는 에러 시 이전 설정을 유지해야 한다.
//
// 두 경로의 허용 집합은 이제 동일하다 — 검증기와 파서가 같은 coerceXxx 를 쓰므로
// "생성은 되는데 저장은 안 되는" 비대칭(qos 문자열 결함)이 구조적으로 재발할 수 없다.
// 갈라지는 것은 실패 처리 방식뿐이다:
//
//   - strict(사용자가 방금 저장한 값): 에러. 값이 조용히 사라지면 안 된다.
//   - lenient(생성/기동, 이미 저장된 값의 재생): 해당 노브만 기본값 유지, 기동은 계속.
//     기동에서 하드 실패시키면 과거에 잘못 저장된 값 하나로 에이전트를 만들 수조차
//     없게 되어(부팅 불가), 사용자가 UI 로 고칠 기회마저 사라진다. 실패한 노브가
//     "조용한 0"(QoS 0 / keep-alive 0) 이 아니라 "선언된 기본값" 이 된다는 점이
//     이번 수정의 핵심이며, 사용자 발 쓰기(Configure)는 전부 strict 가 막는다.
//     대가: 기동 시점에는 잘못된 값이 여전히 조용히 무시된다(파서에 logger 가 없다).
func parseChirpStackConfigStrict(cfg agent.AgentConfig) (ChirpStackConfig, error) {
	if err := validateChirpStackOptions(cfg.Transport.Options); err != nil {
		return ChirpStackConfig{}, err
	}
	return parseChirpStackConfig(cfg), nil
}

// validateChirpStackOptions 는 Transport.Options 의 ChirpStack 노브 타입/범위를 검증한다.
// 키가 없으면 통과한다(기본값 사용).
func validateChirpStackOptions(opts map[string]any) error {
	if opts == nil {
		return nil
	}

	for _, key := range []string{"broker", "client_id", "username", "password"} {
		if v, ok := opts[key]; ok {
			if _, ok := v.(string); !ok {
				return fmt.Errorf("%s 는 문자열이어야 합니다 (got %T)", key, v)
			}
		}
	}
	for _, key := range []string{"auto_reconnect", "clean_session", "emit_comm_state"} {
		if v, ok := opts[key]; ok {
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("%s 는 불리언이어야 합니다 (got %T)", key, v)
			}
		}
	}
	// 아래 검증은 전부 파서와 동일한 coerceXxx 로 판정한다 — "검증기가 통과시킨 값은
	// 파서가 반드시 같은 값으로 읽는다" 를 타입 판별 중복 없이 보장하기 위해서다.
	// 빈 문자열(폼에서 지운 필드)은 미지정으로 보아 건너뛰고 기본값을 쓴다.
	for _, key := range []string{"keep_alive_sec", "buffer_size", "connect_timeout_sec"} {
		v, ok := opts[key]
		if !ok || isUnspecified(v) {
			continue
		}
		if _, ok := coerceInt(v); !ok {
			return fmt.Errorf("%s 는 숫자여야 합니다 (got %T: %v)", key, v, v)
		}
	}
	if v, ok := opts["qos"]; ok && !isUnspecified(v) {
		n, ok := coerceInt(v)
		if !ok {
			return fmt.Errorf("qos 는 숫자여야 합니다 (got %T: %v)", v, v)
		}
		if n < 0 || n > 2 {
			return fmt.Errorf("qos 는 0/1/2 여야 합니다 (got %d)", n)
		}
	}
	if v, ok := opts["topics"]; ok && !isUnspecified(v) {
		list, ok := coerceStringSlice(v)
		if !ok || len(list) == 0 {
			return fmt.Errorf("topics 는 비어 있지 않은 문자열 목록 또는 쉼표로 구분한 문자열이어야 합니다 (got %T: %v)", v, v)
		}
	}
	for _, key := range []string{"comm_report_interval", "offline_threshold"} {
		v, ok := opts[key]
		if !ok || isUnspecified(v) {
			continue
		}
		if _, ok := coerceDuration(v); !ok {
			return fmt.Errorf("%s 는 숫자(초) 또는 duration 문자열이어야 합니다 (got %T: %v)", key, v, v)
		}
	}
	if err := validateMeasurementEmitMode(opts); err != nil {
		return err
	}
	return validateTimestampSource(opts)
}

// ── 공유 강제변환(coercion) 계층 ──────────────────────────────────────────────
//
// 파서와 검증기가 "무엇을 받아들일 수 있는가" 의 정의를 반드시 공유하도록 만드는 층이다.
//
// 이전에는 검증기가 isNumeric() 으로 Go 타입만 확인하고 파서는 toInt() 로 따로 변환해서,
// 두 경로의 허용 집합이 소리 없이 어긋났다: 생성(관용 파서)은 qos:"1" 을 통과시키고
// Configure(검증기)는 같은 값을 거부해 "만들 수는 있는데 저장은 안 되는" 상태가 되었다.
// 이제 두 경로 모두 아래 coerceXxx 만 사용하므로, 새 필드가 추가되어도 그 비대칭이
// 구조적으로 재발할 수 없다 — 검증기는 "강제변환이 되는가" 를 묻고, 파서는 그 결과를 쓴다.
//
// 공통 규약:
//   - 반환값 ok=false 는 "이 값은 쓸 수 없다" 는 뜻이며, 절대 조용한 0 이 아니다.
//     strict 경로(Configure)는 ok=false 를 에러로 올리고, 관용 경로(생성)는 기본값을
//     유지한다(둘 다 조용한 0 을 만들지 않는다).
//   - 부분 성공은 금지한다. 목록 일부 원소만 살려 두는 식의 조용한 누락이 이번 결함의
//     근원 형태이므로, 하나라도 해석 불가면 전체를 거부한다.

// coerceInt 는 옵션 값을 int 로 강제 변환한다.
//
// 허용:
//   - int / int64 / float64 / byte — JSON 숫자는 언제나 float64 로 들어온다.
//   - 10진 정수 문자열("2", " -3 " 처럼 앞뒤 공백 허용). web UI 의 qos 는 select
//     위젯이라 '0'/'1'/'2' 를 문자열로 보낸다.
//
// 거부(ok=false):
//   - 빈 문자열, 소수점 문자열("1.0"), 그 밖의 문자열/타입.
//
// "1.0" 을 거부하는 이유: JSON 에는 정수 타입이 없어 숫자 1 도 float64 로 도착하므로
// 숫자 분기의 절삭은 불가피하다. 반면 문자열은 사용자가 직접 타이핑한 텍스트이므로
// 없어도 되는 절삭 규칙을 새로 발명할 이유가 없다 — 정수 노브에 소수를 적은 것은
// 오타로 보고 알려 주는 편이 낫다.
func coerceInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case byte:
		return int(n), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

// coerceDuration 은 옵션 값을 time.Duration 으로 강제 변환한다.
//
// 허용:
//   - 숫자(int/int64/float64/byte) — "초" 로 해석한다.
//   - 정수 문자열("60") — 숫자 분기와 동일하게 "초" 로 해석한다. 숫자 60 과
//     문자열 "60" 이 다르게 동작하면 coerceInt 를 고친 의미가 없다.
//   - duration 문자열("60s", "1m30s") — time.ParseDuration 규칙.
//
// 거부(ok=false): 빈 문자열, 해석 불가 문자열, 그 밖의 타입.
func coerceDuration(v any) (time.Duration, bool) {
	if n, ok := coerceInt(v); ok {
		return time.Duration(n) * time.Second, true
	}
	s, ok := v.(string)
	if !ok {
		return 0, false
	}
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	return d, true
}

// coerceStringSlice 는 옵션 값을 []string 으로 강제 변환한다.
//
// 허용:
//   - []string / []any(모든 원소가 문자열)
//   - 쉼표로 구분한 문자열("a, b") — web UI 의 '구독 토픽' 필드가 "쉼표로 구분" 이라
//     안내하며 한 줄 문자열로 보낸다.
//
// 어느 형태든 각 원소의 앞뒤 공백을 제거하고 빈 원소는 버려, 세 입력 형태가 같은
// 결과를 내도록 정규화한다.
//
// 거부(ok=false): 문자열이 아닌 원소가 섞인 목록, 목록이 될 수 없는 타입.
// 문자열이 아닌 원소를 조용히 건너뛰던 이전 동작은 부분 성공(조용한 누락)이므로 없앴다.
//
// 결과가 빈 목록인 것 자체는 ok=true 이다 — "비어 있어도 되는가" 는 호출자가 판단한다.
func coerceStringSlice(v any) ([]string, bool) {
	switch s := v.(type) {
	case []string:
		return normalizeStringList(s), true
	case []any:
		items := make([]string, 0, len(s))
		for _, item := range s {
			str, ok := item.(string)
			if !ok {
				return nil, false
			}
			items = append(items, str)
		}
		return normalizeStringList(items), true
	case string:
		return normalizeStringList(strings.Split(s, ",")), true
	default:
		return nil, false
	}
}

// normalizeStringList 는 각 원소의 앞뒤 공백을 제거하고 빈 원소를 버린다.
func normalizeStringList(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// isUnspecified 는 "키는 있지만 값이 비어 있는" 경우를 미지정으로 볼지 판별한다.
//
// HTML 폼은 "비움" 과 "미전송" 을 구분하지 못해, 사용자가 지운 필드가 빈 문자열로
// 도착한다. 이 파일은 이미 같은 관례를 쓰고 있다 — broker/client_id 는 `v != ""`
// 로 빈 값을 무시하고, validateMeasurementEmitMode 는 "" 를 허용값에 포함한다.
// 그 관례를 숫자/duration/목록 노브까지 일관되게 넓힌다: 빈 값은 에러가 아니라
// "기본값 사용" 이다(조용한 0 이 아니라 기본값이라는 점이 핵심이다).
func isUnspecified(v any) bool {
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) == ""
}
