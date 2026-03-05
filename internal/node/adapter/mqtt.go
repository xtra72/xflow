package adapter

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// MQTTAdapter 는 MQTT 프로토콜 전용 브릿지 어댑터이다.
// 토픽 라우팅, QoS 관리, Retained 플래그, 토픽 템플릿 보간을 지원한다.
type MQTTAdapter struct {
	defaultQoS      int
	defaultRetained bool
	publishTopic    string // 발행 토픽 템플릿 (선택)
}

// MQTTAdapterOption 은 MQTTAdapter 생성 옵션이다.
type MQTTAdapterOption func(*MQTTAdapter)

// WithDefaultQoS 는 기본 QoS 레벨을 설정한다 (0, 1, 2).
func WithDefaultQoS(qos int) MQTTAdapterOption {
	return func(a *MQTTAdapter) {
		a.defaultQoS = qos
	}
}

// WithDefaultRetained 는 기본 Retained 플래그를 설정한다.
func WithDefaultRetained(retained bool) MQTTAdapterOption {
	return func(a *MQTTAdapter) {
		a.defaultRetained = retained
	}
}

// WithPublishTopic 은 발행 시 사용할 토픽 템플릿을 설정한다.
// {field_name} 형식의 플레이스홀더를 메시지 페이로드 값으로 치환한다.
func WithPublishTopic(topic string) MQTTAdapterOption {
	return func(a *MQTTAdapter) {
		a.publishTopic = topic
	}
}

// NewMQTTAdapter 는 새로운 MQTTAdapter 인스턴스를 반환한다.
func NewMQTTAdapter(opts ...MQTTAdapterOption) *MQTTAdapter {
	a := &MQTTAdapter{}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Validate 는 주어진 BridgeConfig가 MQTT 어댑터에서 유효한지 검증한다.
// In/InOut 방향은 최소 1개의 토픽이 필수이며, QoS는 0~2 범위, 토픽 형식도 검증한다.
func (a *MQTTAdapter) Validate(config node.BridgeConfig) error {
	// In/InOut 방향은 토픽이 필수이다.
	direction := config.Direction
	if direction == flow.BridgeIn || direction == flow.BridgeInOut {
		if len(config.Topics) == 0 {
			return fmt.Errorf("mqtt adapter: topics required for direction %q", direction)
		}
	}

	// QoS 범위 검증 (0, 1, 2)
	if a.defaultQoS < 0 || a.defaultQoS > 2 {
		return fmt.Errorf("mqtt adapter: QoS must be 0, 1, or 2, got %d", a.defaultQoS)
	}

	// 토픽 형식 검증
	for _, topic := range config.Topics {
		if err := validateMQTTTopic(topic); err != nil {
			return fmt.Errorf("mqtt adapter: invalid topic %q: %w", topic, err)
		}
	}

	return nil
}

// DefaultConfig 는 MQTT 어댑터의 기본 BridgeConfig를 반환한다.
func (a *MQTTAdapter) DefaultConfig() node.BridgeConfig {
	return node.BridgeConfig{
		Direction: flow.BridgeIn,
		Transform: node.TransformConfig{Mode: "auto", PayloadFormat: node.PayloadFormatAuto},
		BufferSize: 256,
	}
}

// TransformToFlow 는 에이전트로부터 수신한 바이트 데이터를 플로우 Message로 변환한다.
// JSON 파싱을 시도하고, 실패 시 원본 바이트를 "_raw" 키에 저장한다.
// MQTT 메타데이터(topic, qos, retained)를 메시지 메타데이터에 설정한다.
func (a *MQTTAdapter) TransformToFlow(data []byte, meta node.AgentMeta) (message.Message, error) {
	msg := message.New()

	// JSON 파싱 시도, 실패 시 원본 저장
	if data != nil {
		if err := trySetJSONPayload(msg, data); err != nil {
			msg.Payload().Set("_raw", data)
		}
	}

	// MQTT 메타데이터 설정
	if meta.Topic != "" {
		msg.Metadata().Set("mqtt.topic", meta.Topic)
	}
	if meta.QoS != 0 {
		msg.Metadata().Set("mqtt.qos", strconv.Itoa(meta.QoS))
	} else if a.defaultQoS != 0 {
		msg.Metadata().Set("mqtt.qos", strconv.Itoa(a.defaultQoS))
	}
	if meta.Retained {
		msg.Metadata().Set("mqtt.retained", "true")
	}

	return msg, nil
}

// TransformToAgent 는 플로우 Message를 에이전트로 전송할 바이트 데이터로 변환한다.
// 페이로드를 JSON으로 직렬화하고, MQTT 메타데이터(topic, qos, retained)를 AgentMeta로 반환한다.
func (a *MQTTAdapter) TransformToAgent(msg message.Message) ([]byte, node.AgentMeta, error) {
	data, err := msg.Payload().ToJSON()
	if err != nil {
		return nil, node.AgentMeta{}, fmt.Errorf("mqtt adapter: payload to JSON: %w", err)
	}

	meta := node.AgentMeta{AgentType: "mqtt"}

	// 토픽: 메타데이터 우선, 없으면 publishTopic 템플릿 사용
	if topic, ok := msg.Metadata().Get("mqtt.topic"); ok {
		meta.Topic = topic
	} else if a.publishTopic != "" {
		meta.Topic = interpolateTemplate(a.publishTopic, msg.Payload())
	}

	// QoS: 메타데이터 우선, 없으면 기본값
	if qosStr, ok := msg.Metadata().Get("mqtt.qos"); ok {
		if qos, parseErr := strconv.Atoi(qosStr); parseErr == nil {
			meta.QoS = qos
		}
	} else {
		meta.QoS = a.defaultQoS
	}

	// Retained: 메타데이터 우선, 없으면 기본값
	if retStr, ok := msg.Metadata().Get("mqtt.retained"); ok {
		meta.Retained = (retStr == "true")
	} else {
		meta.Retained = a.defaultRetained
	}

	return data, meta, nil
}

// HandleControl 은 제어 메시지를 처리한다.
// 현재 MQTT 어댑터에서는 제어 메시지 처리가 없으므로 nil을 반환한다.
func (a *MQTTAdapter) HandleControl(_ message.Message) error {
	return nil
}

// templatePattern 은 {field_name} 형식의 플레이스홀더를 매칭하는 정규식이다.
var templatePattern = regexp.MustCompile(`\{(\w+)\}`)

// interpolateTemplate 은 템플릿 문자열의 {field} 플레이스홀더를 페이로드 값으로 치환한다.
// 페이로드에 해당 키가 없으면 원본 플레이스홀더를 유지한다.
func interpolateTemplate(template string, payload message.Payload) string {
	return templatePattern.ReplaceAllStringFunc(template, func(match string) string {
		key := match[1 : len(match)-1]
		if val, ok := payload.Get(key); ok {
			return fmt.Sprintf("%v", val)
		}
		return match
	})
}

// validateMQTTTopic 은 MQTT 토픽 문자열의 유효성을 검증한다.
// 빈 토픽, 최대 길이(65535바이트), 와일드카드(#, +) 규칙을 확인한다.
func validateMQTTTopic(topic string) error {
	if topic == "" {
		return fmt.Errorf("topic must not be empty")
	}
	if len(topic) > 65535 {
		return fmt.Errorf("topic too long (max 65535 bytes)")
	}

	// # 와일드카드는 마지막 레벨에만, + 와일드카드는 전체 레벨을 차지해야 한다.
	parts := strings.Split(topic, "/")
	for i, part := range parts {
		if part == "#" && i != len(parts)-1 {
			return fmt.Errorf("wildcard '#' must be at the end")
		}
		if strings.Contains(part, "#") && part != "#" {
			return fmt.Errorf("wildcard '#' must occupy entire level")
		}
		if strings.Contains(part, "+") && part != "+" {
			return fmt.Errorf("wildcard '+' must occupy entire level")
		}
	}

	return nil
}

// trySetJSONPayload 는 바이트 데이터를 JSON으로 파싱하여 메시지 페이로드에 설정한다.
// JSON 파싱에 실패하면 에러를 반환한다.
func trySetJSONPayload(msg message.Message, data []byte) error {
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	for k, v := range parsed {
		msg.Payload().Set(k, v)
	}
	return nil
}
