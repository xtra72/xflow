package chirpstack

import (
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
// M5(comm-state) 범위이므로 본 마일스톤에서는 포함하지 않는다.
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
}

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

	return cc
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
