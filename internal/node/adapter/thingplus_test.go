package adapter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/message"
)

func TestThingplusAdapter_TransformToFlow_PreservesTypeFromMessageJSON(t *testing.T) {
	// 에이전트가 방출한 message.Message JSON (Type 포함)을 복원하여 Type 을 보존한다.
	orig := message.New(
		message.WithType("thingplus.rpc.request"),
		message.WithPayload(message.NewPayload(map[string]any{
			"device": "Device A", "id": 1, "method": "setValue",
		})),
	)
	data, err := orig.MarshalJSON()
	require.NoError(t, err)

	a := NewThingplusAdapter()
	msg, err := a.TransformToFlow(data, node.AgentMeta{})
	require.NoError(t, err)
	assert.Equal(t, "thingplus.rpc.request", msg.Type(), "message JSON 의 Type 이 보존되어야 한다")

	pm := msg.Payload().ToMap()
	assert.Equal(t, "Device A", pm["device"])
	assert.EqualValues(t, 1, pm["id"])
}

func TestThingplusAdapter_TransformToFlow_RawJSONTypePromotion(t *testing.T) {
	// message.Message JSON 이 아닌 원시 JSON 은 payload 의 "type" 필드를 Type 으로 승격한다.
	raw := []byte(`{"type":"thingplus.attr.update","device":"Dev","data":{"fw":"1.0"}}`)

	a := NewThingplusAdapter()
	msg, err := a.TransformToFlow(raw, node.AgentMeta{})
	require.NoError(t, err)
	assert.Equal(t, "thingplus.attr.update", msg.Type())
	assert.Equal(t, "Dev", msg.Payload().ToMap()["device"])
}

func TestThingplusAdapter_TransformToFlow_NonJSONFallback(t *testing.T) {
	a := NewThingplusAdapter()
	msg, err := a.TransformToFlow([]byte("not-json"), node.AgentMeta{})
	require.NoError(t, err)
	raw, ok := msg.Payload().Get("_raw")
	assert.True(t, ok, "비-JSON 은 _raw 로 보관되어야 한다")
	assert.Equal(t, []byte("not-json"), raw)
}

func TestThingplusAdapter_TransformToAgent_SerializesMessage(t *testing.T) {
	// RPC 응답 message 를 직렬화하여 에이전트가 Type/payload 로 분기할 수 있게 한다.
	msg := message.New(
		message.WithType("thingplus.rpc.response"),
		message.WithPayload(message.NewPayload(map[string]any{
			"device": "Device A", "id": 1, "data": map[string]any{"ok": true},
		})),
	)

	a := NewThingplusAdapter()
	data, meta, err := a.TransformToAgent(msg)
	require.NoError(t, err)
	assert.Equal(t, "thingplus-gateway", meta.AgentType)

	// 직렬화 결과는 FromJSON 으로 복원 가능해야 하며 Type 이 보존되어야 한다.
	restored, err := message.FromJSON(data)
	require.NoError(t, err)
	assert.Equal(t, "thingplus.rpc.response", restored.Type())
	assert.Equal(t, "Device A", restored.Payload().ToMap()["device"])
}

func TestThingplusAdapter_ValidateAndDefaults(t *testing.T) {
	a := NewThingplusAdapter()
	assert.NoError(t, a.Validate(node.BridgeConfig{}))
	assert.NoError(t, a.HandleControl(message.New()))

	cfg := a.DefaultConfig()
	assert.Equal(t, 256, cfg.BufferSize)
}

func TestThingplusAdapter_Registered(t *testing.T) {
	// register.go init() 로 전역 레지스트리에 등록되어 있어야 한다.
	adapter, ok := node.GetAdapter("thingplus-gateway")
	require.True(t, ok, "thingplus-gateway 어댑터가 등록되어 있어야 한다")
	_, isThingplus := adapter.(*ThingplusAdapter)
	assert.True(t, isThingplus)
}
