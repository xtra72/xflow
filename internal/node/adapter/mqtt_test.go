package adapter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// --- Validate 테스트 ---

// TestMQTTAdapter_Validate 는 다양한 BridgeConfig에 대한 유효성 검증을 확인한다.
func TestMQTTAdapter_Validate(t *testing.T) {
	tests := []struct {
		name    string
		adapter *MQTTAdapter
		config  node.BridgeConfig
		wantErr string
	}{
		{
			name:    "유효한 In 방향 설정",
			adapter: NewMQTTAdapter(),
			config: node.BridgeConfig{
				Direction: flow.BridgeIn,
				Topics:    []string{"sensor/temperature"},
			},
			wantErr: "",
		},
		{
			name:    "유효한 Out 방향 (토픽 없어도 허용)",
			adapter: NewMQTTAdapter(),
			config: node.BridgeConfig{
				Direction: flow.BridgeOut,
			},
			wantErr: "",
		},
		{
			name:    "유효한 InOut 방향 설정",
			adapter: NewMQTTAdapter(),
			config: node.BridgeConfig{
				Direction: flow.BridgeInOut,
				Topics:    []string{"device/#"},
			},
			wantErr: "",
		},
		{
			name:    "유효한 RequestReply 방향 (토픽 없어도 허용)",
			adapter: NewMQTTAdapter(),
			config: node.BridgeConfig{
				Direction: flow.BridgeRequestReply,
			},
			wantErr: "",
		},
		{
			name:    "In 방향에 토픽 없으면 에러",
			adapter: NewMQTTAdapter(),
			config: node.BridgeConfig{
				Direction: flow.BridgeIn,
				Topics:    []string{},
			},
			wantErr: "topics required for direction",
		},
		{
			name:    "InOut 방향에 토픽 없으면 에러",
			adapter: NewMQTTAdapter(),
			config: node.BridgeConfig{
				Direction: flow.BridgeInOut,
			},
			wantErr: "topics required for direction",
		},
		{
			name:    "잘못된 QoS (음수)",
			adapter: NewMQTTAdapter(WithDefaultQoS(-1)),
			config: node.BridgeConfig{
				Direction: flow.BridgeOut,
			},
			wantErr: "QoS must be 0, 1, or 2",
		},
		{
			name:    "잘못된 QoS (3)",
			adapter: NewMQTTAdapter(WithDefaultQoS(3)),
			config: node.BridgeConfig{
				Direction: flow.BridgeOut,
			},
			wantErr: "QoS must be 0, 1, or 2",
		},
		{
			name:    "유효한 QoS 0",
			adapter: NewMQTTAdapter(WithDefaultQoS(0)),
			config: node.BridgeConfig{
				Direction: flow.BridgeOut,
			},
			wantErr: "",
		},
		{
			name:    "유효한 QoS 1",
			adapter: NewMQTTAdapter(WithDefaultQoS(1)),
			config: node.BridgeConfig{
				Direction: flow.BridgeOut,
			},
			wantErr: "",
		},
		{
			name:    "유효한 QoS 2",
			adapter: NewMQTTAdapter(WithDefaultQoS(2)),
			config: node.BridgeConfig{
				Direction: flow.BridgeOut,
			},
			wantErr: "",
		},
		{
			name:    "잘못된 토픽 형식: # 가 중간에 위치",
			adapter: NewMQTTAdapter(),
			config: node.BridgeConfig{
				Direction: flow.BridgeIn,
				Topics:    []string{"sensor/#/data"},
			},
			wantErr: "wildcard '#' must be at the end",
		},
		{
			name:    "잘못된 토픽 형식: # 가 전체 레벨이 아닌 경우",
			adapter: NewMQTTAdapter(),
			config: node.BridgeConfig{
				Direction: flow.BridgeIn,
				Topics:    []string{"sensor/temp#"},
			},
			wantErr: "wildcard '#' must occupy entire level",
		},
		{
			name:    "잘못된 토픽 형식: + 가 전체 레벨이 아닌 경우",
			adapter: NewMQTTAdapter(),
			config: node.BridgeConfig{
				Direction: flow.BridgeIn,
				Topics:    []string{"sensor/temp+ature"},
			},
			wantErr: "wildcard '+' must occupy entire level",
		},
		{
			name:    "유효한 와일드카드 토픽: # 끝",
			adapter: NewMQTTAdapter(),
			config: node.BridgeConfig{
				Direction: flow.BridgeIn,
				Topics:    []string{"sensor/#"},
			},
			wantErr: "",
		},
		{
			name:    "유효한 와일드카드 토픽: + 중간",
			adapter: NewMQTTAdapter(),
			config: node.BridgeConfig{
				Direction: flow.BridgeIn,
				Topics:    []string{"sensor/+/data"},
			},
			wantErr: "",
		},
		{
			name:    "빈 토픽 문자열",
			adapter: NewMQTTAdapter(),
			config: node.BridgeConfig{
				Direction: flow.BridgeIn,
				Topics:    []string{""},
			},
			wantErr: "topic must not be empty",
		},
		{
			name:    "여러 토픽 중 하나가 잘못된 경우",
			adapter: NewMQTTAdapter(),
			config: node.BridgeConfig{
				Direction: flow.BridgeIn,
				Topics:    []string{"sensor/ok", "bad/#/topic"},
			},
			wantErr: "wildcard '#' must be at the end",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.adapter.Validate(tt.config)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

// --- TransformToFlow 테스트 ---

// TestMQTTAdapter_TransformToFlow 는 JSON 데이터를 플로우 메시지로 변환하는 것을 확인한다.
func TestMQTTAdapter_TransformToFlow(t *testing.T) {
	adapter := NewMQTTAdapter()

	data := []byte(`{"temperature":25.5,"unit":"celsius"}`)
	meta := node.AgentMeta{
		Topic:    "sensor/temperature",
		QoS:      1,
		Retained: true,
	}

	msg, err := adapter.TransformToFlow(data, meta)
	require.NoError(t, err)
	require.NotNil(t, msg)

	// JSON 필드가 페이로드에 설정되어야 한다.
	temp, ok := msg.Payload().Get("temperature")
	assert.True(t, ok)
	assert.Equal(t, 25.5, temp)

	unit, ok := msg.Payload().Get("unit")
	assert.True(t, ok)
	assert.Equal(t, "celsius", unit)

	// MQTT 메타데이터 확인
	topic, ok := msg.Metadata().Get("mqtt.topic")
	assert.True(t, ok)
	assert.Equal(t, "sensor/temperature", topic)

	qos, ok := msg.Metadata().Get("mqtt.qos")
	assert.True(t, ok)
	assert.Equal(t, "1", qos)

	retained, ok := msg.Metadata().Get("mqtt.retained")
	assert.True(t, ok)
	assert.Equal(t, "true", retained)
}

// TestMQTTAdapter_TransformToFlow_DefaultQoS 는 메타 QoS가 0일 때 기본 QoS가 적용되는지 확인한다.
func TestMQTTAdapter_TransformToFlow_DefaultQoS(t *testing.T) {
	adapter := NewMQTTAdapter(WithDefaultQoS(2))

	data := []byte(`{"key":"value"}`)
	meta := node.AgentMeta{
		Topic: "test/topic",
		QoS:   0, // 제로값
	}

	msg, err := adapter.TransformToFlow(data, meta)
	require.NoError(t, err)

	// 기본 QoS 2가 적용되어야 한다.
	qos, ok := msg.Metadata().Get("mqtt.qos")
	assert.True(t, ok)
	assert.Equal(t, "2", qos)
}

// TestMQTTAdapter_TransformToFlow_MetaQoSOverridesDefault 는 메타 QoS가 기본 QoS보다 우선하는지 확인한다.
func TestMQTTAdapter_TransformToFlow_MetaQoSOverridesDefault(t *testing.T) {
	adapter := NewMQTTAdapter(WithDefaultQoS(2))

	data := []byte(`{"key":"value"}`)
	meta := node.AgentMeta{
		Topic: "test/topic",
		QoS:   1, // 메타 QoS가 기본값보다 우선
	}

	msg, err := adapter.TransformToFlow(data, meta)
	require.NoError(t, err)

	qos, ok := msg.Metadata().Get("mqtt.qos")
	assert.True(t, ok)
	assert.Equal(t, "1", qos)
}

// TestMQTTAdapter_TransformToFlow_RawData 는 JSON이 아닌 데이터가 _raw로 저장되는지 확인한다.
func TestMQTTAdapter_TransformToFlow_RawData(t *testing.T) {
	adapter := NewMQTTAdapter()

	data := []byte("not-json-data")
	meta := node.AgentMeta{
		Topic: "sensor/raw",
	}

	msg, err := adapter.TransformToFlow(data, meta)
	require.NoError(t, err)
	require.NotNil(t, msg)

	// _raw 키에 원본 바이트가 저장되어야 한다.
	raw, ok := msg.Payload().Get("_raw")
	assert.True(t, ok)
	assert.Equal(t, data, raw)

	// JSON 키는 없어야 한다.
	_, ok = msg.Payload().Get("temperature")
	assert.False(t, ok)
}

// TestMQTTAdapter_TransformToFlow_NilData 는 nil 데이터 처리를 확인한다.
func TestMQTTAdapter_TransformToFlow_NilData(t *testing.T) {
	adapter := NewMQTTAdapter()

	meta := node.AgentMeta{
		Topic: "sensor/null",
	}

	msg, err := adapter.TransformToFlow(nil, meta)
	require.NoError(t, err)
	require.NotNil(t, msg)

	// 페이로드에 _raw 키가 없어야 한다.
	_, ok := msg.Payload().Get("_raw")
	assert.False(t, ok)

	// 토픽 메타데이터는 설정되어야 한다.
	topic, ok := msg.Metadata().Get("mqtt.topic")
	assert.True(t, ok)
	assert.Equal(t, "sensor/null", topic)
}

// TestMQTTAdapter_TransformToFlow_EmptyTopic 은 빈 토픽일 때 메타데이터가 설정되지 않는지 확인한다.
func TestMQTTAdapter_TransformToFlow_EmptyTopic(t *testing.T) {
	adapter := NewMQTTAdapter()

	data := []byte(`{"key":"value"}`)
	meta := node.AgentMeta{}

	msg, err := adapter.TransformToFlow(data, meta)
	require.NoError(t, err)

	_, ok := msg.Metadata().Get("mqtt.topic")
	assert.False(t, ok)
}

// TestMQTTAdapter_TransformToFlow_RetainedFalse 는 Retained=false일 때 메타데이터가 없는지 확인한다.
func TestMQTTAdapter_TransformToFlow_RetainedFalse(t *testing.T) {
	adapter := NewMQTTAdapter()

	data := []byte(`{"key":"value"}`)
	meta := node.AgentMeta{
		Topic:    "test",
		Retained: false,
	}

	msg, err := adapter.TransformToFlow(data, meta)
	require.NoError(t, err)

	_, ok := msg.Metadata().Get("mqtt.retained")
	assert.False(t, ok)
}

// --- TransformToAgent 테스트 ---

// TestMQTTAdapter_TransformToAgent 는 플로우 메시지를 에이전트 데이터로 변환하는 것을 확인한다.
func TestMQTTAdapter_TransformToAgent(t *testing.T) {
	adapter := NewMQTTAdapter()

	msg := message.New()
	msg.Payload().Set("temperature", 25.5)
	msg.Metadata().Set("mqtt.topic", "sensor/temperature")
	msg.Metadata().Set("mqtt.qos", "1")
	msg.Metadata().Set("mqtt.retained", "true")

	data, meta, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)
	require.NotNil(t, data)

	// 토픽, QoS, Retained 확인
	assert.Equal(t, "mqtt", meta.AgentType)
	assert.Equal(t, "sensor/temperature", meta.Topic)
	assert.Equal(t, 1, meta.QoS)
	assert.True(t, meta.Retained)

	// JSON 데이터 확인
	assert.Contains(t, string(data), "temperature")
}

// TestMQTTAdapter_TransformToAgent_TopicTemplate 은 토픽 템플릿 보간을 확인한다.
func TestMQTTAdapter_TransformToAgent_TopicTemplate(t *testing.T) {
	adapter := NewMQTTAdapter(WithPublishTopic("devices/{device_id}/status"))

	msg := message.New()
	msg.Payload().Set("device_id", "sensor-001")
	msg.Payload().Set("status", "online")

	data, meta, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)
	require.NotNil(t, data)

	// 템플릿이 페이로드 값으로 치환되어야 한다.
	assert.Equal(t, "devices/sensor-001/status", meta.Topic)
}

// TestMQTTAdapter_TransformToAgent_TopicTemplate_MissingField 는 누락된 필드 처리를 확인한다.
func TestMQTTAdapter_TransformToAgent_TopicTemplate_MissingField(t *testing.T) {
	adapter := NewMQTTAdapter(WithPublishTopic("devices/{device_id}/status"))

	msg := message.New()
	msg.Payload().Set("status", "online")
	// device_id가 페이로드에 없음

	_, meta, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)

	// 누락된 필드는 원본 플레이스홀더를 유지한다.
	assert.Equal(t, "devices/{device_id}/status", meta.Topic)
}

// TestMQTTAdapter_TransformToAgent_MetadataTopicOverridesTemplate 은 메타데이터 토픽이 템플릿보다 우선하는지 확인한다.
func TestMQTTAdapter_TransformToAgent_MetadataTopicOverridesTemplate(t *testing.T) {
	adapter := NewMQTTAdapter(WithPublishTopic("devices/{device_id}/status"))

	msg := message.New()
	msg.Payload().Set("device_id", "sensor-001")
	msg.Metadata().Set("mqtt.topic", "override/topic")

	_, meta, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)

	// 메타데이터 토픽이 우선해야 한다.
	assert.Equal(t, "override/topic", meta.Topic)
}

// TestMQTTAdapter_TransformToAgent_DefaultValues 는 기본값 폴백을 확인한다.
func TestMQTTAdapter_TransformToAgent_DefaultValues(t *testing.T) {
	adapter := NewMQTTAdapter(
		WithDefaultQoS(2),
		WithDefaultRetained(true),
	)

	msg := message.New()
	msg.Payload().Set("key", "value")
	// 메타데이터에 mqtt.qos, mqtt.retained 없음

	_, meta, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)

	// 기본값이 적용되어야 한다.
	assert.Equal(t, 2, meta.QoS)
	assert.True(t, meta.Retained)
}

// TestMQTTAdapter_TransformToAgent_MetadataOverridesDefaults 는 메타데이터가 기본값보다 우선하는지 확인한다.
func TestMQTTAdapter_TransformToAgent_MetadataOverridesDefaults(t *testing.T) {
	adapter := NewMQTTAdapter(
		WithDefaultQoS(2),
		WithDefaultRetained(true),
	)

	msg := message.New()
	msg.Payload().Set("key", "value")
	msg.Metadata().Set("mqtt.qos", "0")
	msg.Metadata().Set("mqtt.retained", "false")

	_, meta, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)

	// 메타데이터 값이 우선해야 한다.
	assert.Equal(t, 0, meta.QoS)
	assert.False(t, meta.Retained)
}

// TestMQTTAdapter_TransformToAgent_InvalidQoSIgnored 는 잘못된 QoS 문자열이 무시되는지 확인한다.
func TestMQTTAdapter_TransformToAgent_InvalidQoSIgnored(t *testing.T) {
	adapter := NewMQTTAdapter(WithDefaultQoS(1))

	msg := message.New()
	msg.Payload().Set("key", "value")
	msg.Metadata().Set("mqtt.qos", "invalid")

	_, meta, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)

	// 파싱 실패 시 제로값이 된다.
	assert.Equal(t, 0, meta.QoS)
}

// TestMQTTAdapter_TransformToAgent_NoTopic 은 토픽 없이 변환할 때 빈 토픽을 확인한다.
func TestMQTTAdapter_TransformToAgent_NoTopic(t *testing.T) {
	adapter := NewMQTTAdapter()

	msg := message.New()
	msg.Payload().Set("key", "value")

	_, meta, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)

	assert.Equal(t, "", meta.Topic)
}

// --- HandleControl 테스트 ---

// TestMQTTAdapter_HandleControl 은 제어 메시지 처리가 nil을 반환하는지 확인한다.
func TestMQTTAdapter_HandleControl(t *testing.T) {
	adapter := NewMQTTAdapter()

	msg := message.New()
	msg.Payload().Set("action", "subscribe")

	err := adapter.HandleControl(msg)
	assert.NoError(t, err)
}

// --- DefaultConfig 테스트 ---

// TestMQTTAdapter_DefaultConfig 는 기본 설정이 올바른지 확인한다.
func TestMQTTAdapter_DefaultConfig(t *testing.T) {
	adapter := NewMQTTAdapter()
	cfg := adapter.DefaultConfig()

	assert.Equal(t, flow.BridgeIn, cfg.Direction)
	assert.Equal(t, "auto", cfg.Transform.Mode)
	assert.Equal(t, node.PayloadFormatAuto, cfg.Transform.PayloadFormat)
	assert.Equal(t, 256, cfg.BufferSize)
}

// --- BridgeAdapter 인터페이스 구현 확인 ---

// TestMQTTAdapter_ImplementsBridgeAdapter 는 MQTTAdapter가 BridgeAdapter 인터페이스를 구현하는지 확인한다.
func TestMQTTAdapter_ImplementsBridgeAdapter(t *testing.T) {
	var _ node.BridgeAdapter = NewMQTTAdapter()
}

// --- validateMQTTTopic 테스트 ---

// TestValidateMQTTTopic 은 MQTT 토픽 유효성 검증을 확인한다.
func TestValidateMQTTTopic(t *testing.T) {
	tests := []struct {
		name    string
		topic   string
		wantErr string
	}{
		{name: "유효한 일반 토픽", topic: "sensor/temperature", wantErr: ""},
		{name: "유효한 루트 토픽", topic: "data", wantErr: ""},
		{name: "유효한 다중 레벨 토픽", topic: "a/b/c/d/e", wantErr: ""},
		{name: "유효한 # 와일드카드 (끝)", topic: "sensor/#", wantErr: ""},
		{name: "유효한 # 단독", topic: "#", wantErr: ""},
		{name: "유효한 + 와일드카드 (중간)", topic: "sensor/+/data", wantErr: ""},
		{name: "유효한 + 와일드카드 (시작)", topic: "+/temperature", wantErr: ""},
		{name: "유효한 + 단독", topic: "+", wantErr: ""},
		{name: "유효한 + 와 # 조합", topic: "+/sensor/#", wantErr: ""},
		{name: "빈 토픽", topic: "", wantErr: "topic must not be empty"},
		{name: "# 가 중간에 위치", topic: "sensor/#/data", wantErr: "wildcard '#' must be at the end"},
		{name: "# 가 전체 레벨이 아닌 경우", topic: "sensor/temp#", wantErr: "wildcard '#' must occupy entire level"},
		{name: "+ 가 전체 레벨이 아닌 경우", topic: "sensor/temp+ature", wantErr: "wildcard '+' must occupy entire level"},
		{name: "# 가 문자와 섞인 경우", topic: "a#", wantErr: "wildcard '#' must occupy entire level"},
		{name: "+ 가 앞에 문자가 있는 경우", topic: "a+", wantErr: "wildcard '+' must occupy entire level"},
		{
			name:    "최대 길이 초과",
			topic:   strings.Repeat("a", 65536),
			wantErr: "topic too long",
		},
		{
			name:    "최대 길이 경계값 (65535)",
			topic:   strings.Repeat("a", 65535),
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMQTTTopic(tt.topic)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

// --- interpolateTemplate 테스트 ---

// TestInterpolateTemplate 은 토픽 템플릿 보간 동작을 확인한다.
func TestInterpolateTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		payload  map[string]any
		expected string
	}{
		{
			name:     "단일 필드 치환",
			template: "devices/{device_id}/data",
			payload:  map[string]any{"device_id": "sensor-001"},
			expected: "devices/sensor-001/data",
		},
		{
			name:     "복수 필드 치환",
			template: "{region}/{device_id}/status",
			payload:  map[string]any{"region": "us-east", "device_id": "dev-42"},
			expected: "us-east/dev-42/status",
		},
		{
			name:     "누락된 필드는 원본 유지",
			template: "devices/{device_id}/status",
			payload:  map[string]any{"other": "value"},
			expected: "devices/{device_id}/status",
		},
		{
			name:     "플레이스홀더 없는 템플릿",
			template: "static/topic/path",
			payload:  map[string]any{"device_id": "sensor-001"},
			expected: "static/topic/path",
		},
		{
			name:     "빈 템플릿",
			template: "",
			payload:  map[string]any{"device_id": "sensor-001"},
			expected: "",
		},
		{
			name:     "숫자 값 치환",
			template: "sensor/{id}/temp",
			payload:  map[string]any{"id": float64(42)},
			expected: "sensor/42/temp",
		},
		{
			name:     "boolean 값 치환",
			template: "device/{active}/data",
			payload:  map[string]any{"active": true},
			expected: "device/true/data",
		},
		{
			name:     "빈 페이로드",
			template: "devices/{device_id}/data",
			payload:  map[string]any{},
			expected: "devices/{device_id}/data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := message.NewPayload(tt.payload)
			result := interpolateTemplate(tt.template, payload)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- trySetJSONPayload 테스트 ---

// TestTrySetJSONPayload_ValidJSON 은 유효한 JSON 파싱을 확인한다.
func TestTrySetJSONPayload_ValidJSON(t *testing.T) {
	msg := message.New()
	data := []byte(`{"name":"test","value":123}`)

	err := trySetJSONPayload(msg, data)
	assert.NoError(t, err)

	name, ok := msg.Payload().Get("name")
	assert.True(t, ok)
	assert.Equal(t, "test", name)

	value, ok := msg.Payload().Get("value")
	assert.True(t, ok)
	assert.Equal(t, float64(123), value)
}

// TestTrySetJSONPayload_InvalidJSON 은 잘못된 JSON에서 에러를 반환하는지 확인한다.
func TestTrySetJSONPayload_InvalidJSON(t *testing.T) {
	msg := message.New()
	data := []byte("not-json")

	err := trySetJSONPayload(msg, data)
	assert.Error(t, err)
}

// TestTrySetJSONPayload_JSONArray 는 JSON 배열이 에러를 반환하는지 확인한다.
func TestTrySetJSONPayload_JSONArray(t *testing.T) {
	msg := message.New()
	data := []byte(`[1, 2, 3]`)

	err := trySetJSONPayload(msg, data)
	// JSON 배열은 map[string]any로 파싱할 수 없으므로 에러
	assert.Error(t, err)
}

// --- 옵션 패턴 테스트 ---

// TestMQTTAdapterOptions 는 어댑터 옵션이 올바르게 적용되는지 확인한다.
func TestMQTTAdapterOptions(t *testing.T) {
	adapter := NewMQTTAdapter(
		WithDefaultQoS(2),
		WithDefaultRetained(true),
		WithPublishTopic("test/{id}"),
	)

	assert.Equal(t, 2, adapter.defaultQoS)
	assert.True(t, adapter.defaultRetained)
	assert.Equal(t, "test/{id}", adapter.publishTopic)
}

// TestNewMQTTAdapter_NoOptions 는 옵션 없이 생성 시 제로값을 확인한다.
func TestNewMQTTAdapter_NoOptions(t *testing.T) {
	adapter := NewMQTTAdapter()

	assert.Equal(t, 0, adapter.defaultQoS)
	assert.False(t, adapter.defaultRetained)
	assert.Equal(t, "", adapter.publishTopic)
}
