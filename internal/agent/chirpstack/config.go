package chirpstack

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/xtra/xflow/internal/agent"
)

// defaultChirpStackTopic 는 ChirpStack application 이벤트 기본 구독 토픽이다.
// ChirpStack MQTT integration 은 application/<id>/device/<devEui>/event/<type>
// 형태로 발행하므로 "application/#" 로 전 애플리케이션 이벤트를 포괄한다.
const defaultChirpStackTopic = "application/#"

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
}

// defaultOfflineThreshold 는 업링크 staleness→offline 판정의 보수적 기본 임계이다.
// LoRaWAN 클래스 A 디바이스 업링크 주기가 디바이스마다 상이하므로 넉넉히 잡는다.
const defaultOfflineThreshold = 300 * time.Second

// parseChirpStackConfig 는 AgentConfig 에서 ChirpStackConfig 를 파싱한다.
// parseMQTTConfig 관용구(기본값 세팅 + Transport.Options 타입 어서션 오버라이드)를
// 재사용한다.
func parseChirpStackConfig(cfg agent.AgentConfig) ChirpStackConfig {
	cc := ChirpStackConfig{
		Broker:            "tcp://localhost:1883",
		ClientID:          "xflow-chirpstack-" + uuid.New().String(),
		Topics:            []string{defaultChirpStackTopic},
		QoS:               1,
		KeepAliveSec:      60,
		AutoReconnect:     true,
		CleanSession:      true,
		BufferSize:        1024,
		ConnectTimeoutSec: 10,
		OfflineThreshold:  defaultOfflineThreshold,
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
	if v, ok := opts["topics"]; ok {
		if topics := toStringSlice(v); len(topics) > 0 {
			cc.Topics = topics
		}
	}
	if v, ok := opts["qos"]; ok {
		cc.QoS = byte(toInt(v))
	}
	if v, ok := opts["keep_alive_sec"]; ok {
		cc.KeepAliveSec = toInt(v)
	}
	if v, ok := opts["auto_reconnect"].(bool); ok {
		cc.AutoReconnect = v
	}
	if v, ok := opts["clean_session"].(bool); ok {
		cc.CleanSession = v
	}
	if v, ok := opts["buffer_size"]; ok {
		if n := toInt(v); n > 0 {
			cc.BufferSize = n
		}
	}
	if v, ok := opts["connect_timeout_sec"]; ok {
		cc.ConnectTimeoutSec = toInt(v)
	}

	// M5 comm-state 노브.
	if v, ok := opts["emit_comm_state"].(bool); ok {
		cc.EmitCommState = v
	}
	if v, ok := opts["comm_report_interval"]; ok {
		cc.CommReportInterval = toDuration(v)
	}
	if v, ok := opts["offline_threshold"]; ok {
		if d := toDuration(v); d > 0 {
			cc.OfflineThreshold = d
		}
	}

	return cc
}

// parseChirpStackConfigStrict 는 런타임 재설정(Configure) 경로용 파서이다.
//
// parseChirpStackConfig 는 타입이 어긋난 옵션을 조용히 무시하고 기본값을 남긴다.
// 생성 경로에서는 그 관용(lenient) 동작을 그대로 보존하지만, 런타임 재설정에서는
// 사용자가 방금 저장한 값이 조용히 사라지는 것이 곧 결함이므로 에러로 거부한다.
// 호출자는 에러 시 이전 설정을 유지해야 한다.
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
	for _, key := range []string{"keep_alive_sec", "buffer_size", "connect_timeout_sec"} {
		if v, ok := opts[key]; ok && !isNumeric(v) {
			return fmt.Errorf("%s 는 숫자여야 합니다 (got %T)", key, v)
		}
	}
	if v, ok := opts["qos"]; ok {
		if !isNumeric(v) {
			return fmt.Errorf("qos 는 숫자여야 합니다 (got %T)", v)
		}
		if n := toInt(v); n < 0 || n > 2 {
			return fmt.Errorf("qos 는 0/1/2 여야 합니다 (got %d)", n)
		}
	}
	if v, ok := opts["topics"]; ok {
		if len(toStringSlice(v)) == 0 {
			return fmt.Errorf("topics 는 비어 있지 않은 문자열 목록이어야 합니다 (got %T)", v)
		}
	}
	for _, key := range []string{"comm_report_interval", "offline_threshold"} {
		v, ok := opts[key]
		if !ok {
			continue
		}
		if isNumeric(v) {
			continue
		}
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("%s 는 숫자(초) 또는 duration 문자열이어야 합니다 (got %T)", key, v)
		}
		if _, err := time.ParseDuration(s); err != nil {
			return fmt.Errorf("%s duration 파싱 실패 (%q): %w", key, s, err)
		}
	}
	return nil
}

// isNumeric 은 toInt / toDuration 이 숫자로 해석할 수 있는 타입인지 판별한다.
func isNumeric(v any) bool {
	switch v.(type) {
	case int, int64, float64, byte:
		return true
	default:
		return false
	}
}

// toDuration 은 인터페이스 값을 time.Duration 으로 변환한다.
//
//   - 숫자(int/int64/float64/byte)는 "초" 단위로 해석한다 (JSON 숫자는 float64).
//   - 문자열은 time.ParseDuration 으로 해석하고, 실패 시 0 을 반환한다.
//   - 그 외 타입은 0.
func toDuration(v any) time.Duration {
	switch n := v.(type) {
	case int:
		return time.Duration(n) * time.Second
	case int64:
		return time.Duration(n) * time.Second
	case float64:
		return time.Duration(n) * time.Second
	case byte:
		return time.Duration(n) * time.Second
	case string:
		if d, err := time.ParseDuration(n); err == nil {
			return d
		}
		return 0
	default:
		return 0
	}
}

// toInt 는 인터페이스 값을 int 로 변환한다 (JSON unmarshal 시 숫자는 float64).
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case byte:
		return int(n)
	default:
		return 0
	}
}

// toStringSlice 는 인터페이스 값을 []string 으로 변환한다.
func toStringSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		result := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	default:
		return nil
	}
}
