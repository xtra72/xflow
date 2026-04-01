package adapter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// TestSerialAdapter_BridgeAdapterInterface 는 SerialAdapter가 BridgeAdapter 인터페이스를 구현하는지 확인한다.
func TestSerialAdapter_BridgeAdapterInterface(t *testing.T) {
	var _ node.BridgeAdapter = (*SerialAdapter)(nil)
}

// TestSerialAdapter_Validate 는 시리얼 어댑터의 방향 검증 로직을 테스트한다.
func TestSerialAdapter_Validate(t *testing.T) {
	tests := []struct {
		name      string
		direction flow.BridgeDirection
		wantErr   bool
	}{
		{
			name:      "BridgeIn 유효",
			direction: flow.BridgeIn,
			wantErr:   false,
		},
		{
			name:      "BridgeOut 유효",
			direction: flow.BridgeOut,
			wantErr:   false,
		},
		{
			name:      "BridgeInOut 유효",
			direction: flow.BridgeInOut,
			wantErr:   false,
		},
		{
			name:      "BridgeRequestReply 유효",
			direction: flow.BridgeRequestReply,
			wantErr:   false,
		},
		{
			name:      "잘못된 방향",
			direction: flow.BridgeDirection("invalid"),
			wantErr:   true,
		},
		{
			name:      "빈 방향",
			direction: flow.BridgeDirection(""),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewSerialAdapter()
			err := adapter.Validate(node.BridgeConfig{Direction: tt.direction})
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "serial adapter")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestSerialAdapter_DefaultConfig 는 기본 설정이 올바른 값을 반환하는지 확인한다.
// 시리얼 어댑터는 기본 방향이 BridgeInOut (양방향)이다.
func TestSerialAdapter_DefaultConfig(t *testing.T) {
	adapter := NewSerialAdapter()
	config := adapter.DefaultConfig()

	assert.Equal(t, flow.BridgeInOut, config.Direction)
	assert.Equal(t, 1024, config.BufferSize)
}

// TestSerialAdapter_TransformToFlow 는 바이트 데이터가 메시지로 변환되는지 확인한다.
func TestSerialAdapter_TransformToFlow(t *testing.T) {
	adapter := NewSerialAdapter()
	data := []byte("hello serial")
	meta := node.AgentMeta{AgentType: "serial"}

	msg, err := adapter.TransformToFlow(data, meta)
	require.NoError(t, err)
	assert.NotNil(t, msg)

	// raw 바이트가 페이로드에 저장되어야 한다.
	raw, ok := msg.Payload().Get("raw")
	assert.True(t, ok)
	assert.Equal(t, data, raw)

	// 문자열 데이터도 저장되어야 한다.
	strData, ok := msg.Payload().Get("data")
	assert.True(t, ok)
	assert.Equal(t, "hello serial", strData)

	// 에이전트 타입이 메타데이터에 설정되어야 한다.
	agentType, ok := msg.Metadata().Get("agent.type")
	assert.True(t, ok)
	assert.Equal(t, "serial", agentType)
}

// TestSerialAdapter_TransformToFlow_EmptyMeta 는 빈 메타데이터 시 agent.type이 설정되지 않는지 확인한다.
func TestSerialAdapter_TransformToFlow_EmptyMeta(t *testing.T) {
	adapter := NewSerialAdapter()
	data := []byte("data")

	msg, err := adapter.TransformToFlow(data, node.AgentMeta{})
	require.NoError(t, err)

	_, ok := msg.Metadata().Get("agent.type")
	assert.False(t, ok)
}

// TestSerialAdapter_TransformToAgent_WithRawPayload 는 raw 페이로드가 바이트로 변환되는지 확인한다.
func TestSerialAdapter_TransformToAgent_WithRawPayload(t *testing.T) {
	adapter := NewSerialAdapter()
	msg := message.New()
	msg.Payload().Set("raw", []byte{0x01, 0x02, 0x03})

	data, meta, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x01, 0x02, 0x03}, data)
	assert.Equal(t, node.AgentMeta{}, meta)
}

// TestSerialAdapter_TransformToAgent_FallbackJSON 은 raw가 없을 때 JSON 직렬화되는지 확인한다.
func TestSerialAdapter_TransformToAgent_FallbackJSON(t *testing.T) {
	adapter := NewSerialAdapter()
	msg := message.New()
	msg.Payload().Set("command", "reset")

	data, _, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)
	assert.NotNil(t, data)
	assert.Contains(t, string(data), "command")
	assert.Contains(t, string(data), "reset")
}

// TestSerialAdapter_HandleControl 은 제어 메시지 처리가 nil을 반환하는지 확인한다.
func TestSerialAdapter_HandleControl(t *testing.T) {
	adapter := NewSerialAdapter()
	msg := message.New()

	err := adapter.HandleControl(msg)
	assert.NoError(t, err)
}
