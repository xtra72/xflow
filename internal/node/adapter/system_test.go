package adapter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/message"
)

// TestSystemAdapter_Validate_ValidSubtypes 는 모든 유효한 서브타입이 검증을 통과하는지 확인한다.
func TestSystemAdapter_Validate_ValidSubtypes(t *testing.T) {
	tests := []struct {
		name    string
		subtype string
	}{
		{name: "store", subtype: "store"},
		{name: "timer", subtype: "timer"},
		{name: "logger", subtype: "logger"},
		{name: "event", subtype: "event"},
		{name: "file", subtype: "file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewSystemAdapter(tt.subtype)
			err := adapter.Validate(node.BridgeConfig{})
			assert.NoError(t, err)
		})
	}
}

// TestSystemAdapter_Validate_InvalidSubtype 는 유효하지 않은 서브타입이 에러를 반환하는지 확인한다.
func TestSystemAdapter_Validate_InvalidSubtype(t *testing.T) {
	tests := []struct {
		name    string
		subtype string
	}{
		{name: "invalid", subtype: "invalid"},
		{name: "empty", subtype: ""},
		{name: "unknown", subtype: "mqtt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewSystemAdapter(tt.subtype)
			err := adapter.Validate(node.BridgeConfig{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid system agent subtype")
			assert.Contains(t, err.Error(), tt.subtype)
		})
	}
}

// TestSystemAdapter_TransformToFlow_JSONData 는 JSON 데이터가 페이로드에 파싱되는지 확인한다.
func TestSystemAdapter_TransformToFlow_JSONData(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]any
		wantKeys []string
	}{
		{
			name:     "단일 키-값",
			data:     map[string]any{"key": "value"},
			wantKeys: []string{"key"},
		},
		{
			name:     "다중 키-값",
			data:     map[string]any{"name": "test", "count": float64(42)},
			wantKeys: []string{"count", "name"},
		},
		{
			name:     "중첩 객체",
			data:     map[string]any{"nested": map[string]any{"inner": "data"}},
			wantKeys: []string{"nested"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewSystemAdapter("store")
			jsonData, err := json.Marshal(tt.data)
			require.NoError(t, err)

			msg, err := adapter.TransformToFlow(jsonData, node.AgentMeta{})
			require.NoError(t, err)
			assert.NotNil(t, msg)

			// 페이로드에 파싱된 키가 존재하는지 확인
			keys := msg.Payload().Keys()
			assert.Equal(t, tt.wantKeys, keys)

			// 원본 데이터의 각 키-값이 올바르게 설정되었는지 확인
			for k, v := range tt.data {
				got, ok := msg.Payload().Get(k)
				assert.True(t, ok, "키 %q가 페이로드에 없음", k)
				assert.Equal(t, v, got)
			}
		})
	}
}

// TestSystemAdapter_TransformToFlow_RawData 는 JSON이 아닌 데이터가 _raw로 저장되는지 확인한다.
func TestSystemAdapter_TransformToFlow_RawData(t *testing.T) {
	adapter := NewSystemAdapter("logger")
	rawData := []byte("이것은 JSON이 아닌 원본 데이터입니다")

	msg, err := adapter.TransformToFlow(rawData, node.AgentMeta{})
	require.NoError(t, err)
	assert.NotNil(t, msg)

	// _raw 키에 원본 데이터가 저장되었는지 확인
	raw, ok := msg.Payload().Get("_raw")
	assert.True(t, ok, "_raw 키가 페이로드에 없음")
	assert.Equal(t, rawData, raw)
}

// TestSystemAdapter_TransformToFlow_NilData 는 nil 데이터가 패닉 없이 처리되는지 확인한다.
func TestSystemAdapter_TransformToFlow_NilData(t *testing.T) {
	adapter := NewSystemAdapter("timer")

	msg, err := adapter.TransformToFlow(nil, node.AgentMeta{})
	require.NoError(t, err)
	assert.NotNil(t, msg)

	// 빈 페이로드인지 확인
	keys := msg.Payload().Keys()
	assert.Empty(t, keys)
}

// TestSystemAdapter_TransformToFlow_WithMeta 는 AgentMeta가 메시지 메타데이터로 변환되는지 확인한다.
func TestSystemAdapter_TransformToFlow_WithMeta(t *testing.T) {
	adapter := NewSystemAdapter("store")
	meta := node.AgentMeta{
		Topic: "test/topic",
		QoS:   1,
	}

	msg, err := adapter.TransformToFlow(nil, meta)
	require.NoError(t, err)

	// MQTT 메타데이터가 설정되었는지 확인
	topic, ok := msg.Metadata().Get("mqtt.topic")
	assert.True(t, ok)
	assert.Equal(t, "test/topic", topic)

	qos, ok := msg.Metadata().Get("mqtt.qos")
	assert.True(t, ok)
	assert.Equal(t, "1", qos)
}

// TestSystemAdapter_TransformToAgent 는 메시지가 JSON 바이트와 AgentMeta로 변환되는지 확인한다.
func TestSystemAdapter_TransformToAgent(t *testing.T) {
	adapter := NewSystemAdapter("store")

	// 테스트 메시지 생성
	msg := message.New()
	msg.Payload().Set("operation", "get")
	msg.Payload().Set("key", "test-key")
	msg.Metadata().Set("store.operation", "get")

	data, meta, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)
	assert.NotNil(t, data)

	// JSON 데이터 검증
	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)
	assert.Equal(t, "get", result["operation"])
	assert.Equal(t, "test-key", result["key"])

	// AgentType이 "system"으로 설정되었는지 확인
	assert.Equal(t, "system", meta.AgentType)
}

// TestSystemAdapter_HandleControl 은 제어 메시지 처리가 nil을 반환하는지 확인한다.
func TestSystemAdapter_HandleControl(t *testing.T) {
	adapter := NewSystemAdapter("store")
	msg := message.New()

	err := adapter.HandleControl(msg)
	assert.NoError(t, err)
}

// TestSystemAdapter_DefaultConfig 는 기본 설정이 빈 BridgeConfig를 반환하는지 확인한다.
func TestSystemAdapter_DefaultConfig(t *testing.T) {
	adapter := NewSystemAdapter("timer")

	config := adapter.DefaultConfig()
	assert.Equal(t, node.BridgeConfig{}, config)
}
