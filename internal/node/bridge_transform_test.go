package node

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/message"
)

// --- BridgeTransformer 인터페이스 준수 ---

var _ BridgeTransformer = (*DefaultTransformer)(nil)

// --- DefaultTransformer 생성 테스트 ---

// TestNewDefaultTransformer_생성 은 DefaultTransformer가 올바르게 생성되는지 확인한다.
func TestNewDefaultTransformer_생성(t *testing.T) {
	tr := NewDefaultTransformer()
	assert.NotNil(t, tr)
}

// --- AgentToFlow 테스트 ---

// TestDefaultTransformer_AgentToFlow_바이트데이터 는 바이트 데이터를 Message로 변환하는지 확인한다.
func TestDefaultTransformer_AgentToFlow_바이트데이터(t *testing.T) {
	tr := NewDefaultTransformer()
	data := []byte(`{"hello": "world"}`)

	msg, err := tr.AgentToFlow(data)
	require.NoError(t, err)
	require.NotNil(t, msg)

	// _raw 키에 원본 바이트가 저장되어야 한다
	raw, ok := msg.Payload().Get("_raw")
	require.True(t, ok)
	assert.Equal(t, data, raw)
}

// TestDefaultTransformer_AgentToFlow_빈데이터 는 빈 바이트도 처리할 수 있는지 확인한다.
func TestDefaultTransformer_AgentToFlow_빈데이터(t *testing.T) {
	tr := NewDefaultTransformer()
	data := []byte{}

	msg, err := tr.AgentToFlow(data)
	require.NoError(t, err)
	require.NotNil(t, msg)

	raw, ok := msg.Payload().Get("_raw")
	require.True(t, ok)
	assert.Equal(t, data, raw)
}

// TestDefaultTransformer_AgentToFlow_nil데이터 는 nil 바이트도 처리할 수 있는지 확인한다.
func TestDefaultTransformer_AgentToFlow_nil데이터(t *testing.T) {
	tr := NewDefaultTransformer()

	msg, err := tr.AgentToFlow(nil)
	require.NoError(t, err)
	require.NotNil(t, msg)

	raw, ok := msg.Payload().Get("_raw")
	require.True(t, ok)
	assert.Nil(t, raw)
}

// TestDefaultTransformer_AgentToFlow_고유ID 는 생성된 메시지마다 고유 ID를 가지는지 확인한다.
func TestDefaultTransformer_AgentToFlow_고유ID(t *testing.T) {
	tr := NewDefaultTransformer()
	data := []byte("test")

	msg1, _ := tr.AgentToFlow(data)
	msg2, _ := tr.AgentToFlow(data)

	assert.NotEqual(t, msg1.ID(), msg2.ID())
}

// --- FlowToAgent 테스트 ---

// TestDefaultTransformer_FlowToAgent_바이트페이로드 는 _raw가 []byte일 때 직접 반환하는지 확인한다.
func TestDefaultTransformer_FlowToAgent_바이트페이로드(t *testing.T) {
	tr := NewDefaultTransformer()
	expected := []byte(`{"key": "value"}`)

	msg := message.New()
	msg.Payload().Set("_raw", expected)

	result, err := tr.FlowToAgent(msg)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

// TestDefaultTransformer_FlowToAgent_문자열페이로드 는 _raw가 string일 때 []byte로 변환하는지 확인한다.
func TestDefaultTransformer_FlowToAgent_문자열페이로드(t *testing.T) {
	tr := NewDefaultTransformer()
	msg := message.New()
	msg.Payload().Set("_raw", "hello world")

	result, err := tr.FlowToAgent(msg)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello world"), result)
}

// TestDefaultTransformer_FlowToAgent_raw없음_JSON폴백 은 _raw가 없으면 Payload를 JSON으로 직렬화하는지 확인한다.
func TestDefaultTransformer_FlowToAgent_raw없음_JSON폴백(t *testing.T) {
	tr := NewDefaultTransformer()
	msg := message.New()
	msg.Payload().Set("name", "test")
	msg.Payload().Set("value", float64(42))

	result, err := tr.FlowToAgent(msg)
	require.NoError(t, err)

	// JSON으로 파싱 가능해야 한다
	var parsed map[string]any
	err = json.Unmarshal(result, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "test", parsed["name"])
	assert.Equal(t, float64(42), parsed["value"])
}

// TestDefaultTransformer_FlowToAgent_빈페이로드_JSON폴백 은 빈 페이로드도 JSON 폴백이 동작하는지 확인한다.
func TestDefaultTransformer_FlowToAgent_빈페이로드_JSON폴백(t *testing.T) {
	tr := NewDefaultTransformer()
	msg := message.New()

	result, err := tr.FlowToAgent(msg)
	require.NoError(t, err)

	// 빈 JSON 객체여야 한다
	var parsed map[string]any
	err = json.Unmarshal(result, &parsed)
	require.NoError(t, err)
	assert.Empty(t, parsed)
}

// TestDefaultTransformer_왕복변환 은 AgentToFlow -> FlowToAgent 왕복이 데이터를 보존하는지 확인한다.
func TestDefaultTransformer_왕복변환(t *testing.T) {
	tr := NewDefaultTransformer()
	original := []byte(`{"data": "round-trip"}`)

	// 에이전트 -> 플로우
	msg, err := tr.AgentToFlow(original)
	require.NoError(t, err)

	// 플로우 -> 에이전트
	result, err := tr.FlowToAgent(msg)
	require.NoError(t, err)

	assert.Equal(t, original, result)
}
