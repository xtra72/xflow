package adapter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// TestSocketAdapter_BridgeAdapterInterface 는 SocketAdapter가 BridgeAdapter 인터페이스를 구현하는지 확인한다.
func TestSocketAdapter_BridgeAdapterInterface(t *testing.T) {
	var _ node.BridgeAdapter = (*SocketAdapter)(nil)
}

// TestSocketAdapter_Validate 는 소켓 어댑터의 방향 검증 로직을 테스트한다.
func TestSocketAdapter_Validate(t *testing.T) {
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
			adapter := NewSocketAdapter()
			err := adapter.Validate(node.BridgeConfig{Direction: tt.direction})
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "socket adapter")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestSocketAdapter_DefaultConfig 는 기본 설정이 올바른 값을 반환하는지 확인한다.
func TestSocketAdapter_DefaultConfig(t *testing.T) {
	adapter := NewSocketAdapter()
	config := adapter.DefaultConfig()

	assert.Equal(t, flow.BridgeIn, config.Direction)
	assert.Equal(t, 1024, config.BufferSize)
}

// TestSocketAdapter_TransformToFlow 는 바이트 데이터가 메시지로 변환되는지 확인한다.
func TestSocketAdapter_TransformToFlow(t *testing.T) {
	adapter := NewSocketAdapter()
	data := []byte("hello socket")
	meta := node.AgentMeta{AgentType: "tcp-server"}

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
	assert.Equal(t, "hello socket", strData)

	// 에이전트 타입이 메타데이터에 설정되어야 한다.
	agentType, ok := msg.Metadata().Get("agent.type")
	assert.True(t, ok)
	assert.Equal(t, "tcp-server", agentType)
}

// TestSocketAdapter_TransformToFlow_NilData 는 nil 데이터가 패닉 없이 처리되는지 확인한다.
func TestSocketAdapter_TransformToFlow_NilData(t *testing.T) {
	adapter := NewSocketAdapter()

	msg, err := adapter.TransformToFlow(nil, node.AgentMeta{})
	require.NoError(t, err)
	assert.NotNil(t, msg)

	// nil 데이터는 빈 바이트 슬라이스와 빈 문자열로 저장되어야 한다.
	raw, ok := msg.Payload().Get("raw")
	assert.True(t, ok)
	assert.Equal(t, []byte(nil), raw)
}

// TestSocketAdapter_TransformToFlow_EmptyData 는 빈 데이터가 올바르게 처리되는지 확인한다.
func TestSocketAdapter_TransformToFlow_EmptyData(t *testing.T) {
	adapter := NewSocketAdapter()

	msg, err := adapter.TransformToFlow([]byte{}, node.AgentMeta{AgentType: "udp-client"})
	require.NoError(t, err)
	assert.NotNil(t, msg)

	raw, ok := msg.Payload().Get("raw")
	assert.True(t, ok)
	assert.Equal(t, []byte{}, raw)

	strData, ok := msg.Payload().Get("data")
	assert.True(t, ok)
	assert.Equal(t, "", strData)
}

// TestSocketAdapter_TransformToFlow_BinaryData 는 바이너리 데이터가 올바르게 처리되는지 확인한다.
func TestSocketAdapter_TransformToFlow_BinaryData(t *testing.T) {
	adapter := NewSocketAdapter()
	data := []byte{0x00, 0x01, 0xFF, 0xFE}
	meta := node.AgentMeta{AgentType: "tcp-client"}

	msg, err := adapter.TransformToFlow(data, meta)
	require.NoError(t, err)

	raw, ok := msg.Payload().Get("raw")
	assert.True(t, ok)
	assert.Equal(t, data, raw)
}

// TestSocketAdapter_TransformToAgent_WithRawPayload 는 raw 페이로드가 바이트로 변환되는지 확인한다.
func TestSocketAdapter_TransformToAgent_WithRawPayload(t *testing.T) {
	adapter := NewSocketAdapter()
	msg := message.New()
	msg.Payload().Set("raw", []byte("hello"))

	data, meta, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), data)
	assert.Equal(t, node.AgentMeta{}, meta)
}

// TestSocketAdapter_TransformToAgent_FallbackJSON 은 raw가 없을 때 JSON 직렬화되는지 확인한다.
func TestSocketAdapter_TransformToAgent_FallbackJSON(t *testing.T) {
	adapter := NewSocketAdapter()
	msg := message.New()
	msg.Payload().Set("key", "value")

	data, _, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)
	assert.NotNil(t, data)
	assert.Contains(t, string(data), "key")
	assert.Contains(t, string(data), "value")
}

// TestSocketAdapter_TransformToAgent_RawNotBytes 는 raw가 []byte가 아닐 때 JSON 폴백되는지 확인한다.
func TestSocketAdapter_TransformToAgent_RawNotBytes(t *testing.T) {
	adapter := NewSocketAdapter()
	msg := message.New()
	msg.Payload().Set("raw", "not-bytes")

	data, _, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)
	assert.NotNil(t, data)
	// JSON 폴백으로 직렬화되어야 한다.
	assert.Contains(t, string(data), "raw")
}

// TestSocketAdapter_HandleControl 은 제어 메시지 처리가 nil을 반환하는지 확인한다.
func TestSocketAdapter_HandleControl(t *testing.T) {
	adapter := NewSocketAdapter()
	msg := message.New()

	err := adapter.HandleControl(msg)
	assert.NoError(t, err)
}
