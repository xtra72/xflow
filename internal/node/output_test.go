package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

func TestNewOutputNode_Basic(t *testing.T) {
	def := flow.NodeDef{ID: "out1", Type: "output"}
	n, err := NewOutputNode(def)
	require.NoError(t, err)
	require.NotNil(t, n)

	on := n.(*OutputNode)
	assert.Equal(t, "[output]", on.prefix)
	assert.Nil(t, on.tmpl)
}

func TestOutputNode_Configure_Defaults(t *testing.T) {
	def := flow.NodeDef{ID: "out1", Type: "output"}
	n, err := NewOutputNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{})
	assert.NoError(t, err)

	on := n.(*OutputNode)
	assert.Equal(t, "[output]", on.prefix)
	assert.Nil(t, on.tmpl)
}

func TestOutputNode_Configure_WithTemplate(t *testing.T) {
	def := flow.NodeDef{ID: "out1", Type: "output"}
	n, err := NewOutputNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"prefix":   "[test]",
		"template": "temp={{.temperature}}",
	})
	assert.NoError(t, err)

	on := n.(*OutputNode)
	assert.Equal(t, "[test]", on.prefix)
	assert.NotNil(t, on.tmpl)
}

func TestOutputNode_Configure_InvalidTemplate(t *testing.T) {
	def := flow.NodeDef{ID: "out1", Type: "output"}
	n, err := NewOutputNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"template": "{{.invalid",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid template")
}

func TestOutputNode_Process_NoTemplate(t *testing.T) {
	def := flow.NodeDef{ID: "out1", Type: "output"}
	n, err := NewOutputNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 25.5,
	})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, msg.ID(), results[0].ID())
}

func TestOutputNode_Process_WithTemplate(t *testing.T) {
	def := flow.NodeDef{ID: "out1", Type: "output"}
	n, err := NewOutputNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"template": "temp={{.temperature}}",
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 25.5,
	})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	// Pass-through: 동일한 메시지가 반환된다
	assert.Equal(t, msg.ID(), results[0].ID())
}

func TestOutputNode_Process_PassThrough(t *testing.T) {
	def := flow.NodeDef{ID: "out1", Type: "output"}
	n, err := NewOutputNode(def)
	require.NoError(t, err)
	_ = n.Configure(map[string]any{})
	_ = n.Init(context.Background())

	payload := map[string]any{"key": "value", "num": 42.0}
	msg := message.New(message.WithPayload(message.NewPayload(payload)))
	originalID := msg.ID()

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 원본 메시지가 변경되지 않았는지 확인
	assert.Equal(t, originalID, results[0].ID())

	keyVal, keyOk := results[0].Payload().Get("key")
	assert.True(t, keyOk)
	assert.Equal(t, "value", keyVal)

	numVal, numOk := results[0].Payload().Get("num")
	assert.True(t, numOk)
	assert.Equal(t, 42.0, numVal)
}

func TestOutputNode_Shutdown(t *testing.T) {
	def := flow.NodeDef{ID: "out1", Type: "output"}
	n, err := NewOutputNode(def)
	require.NoError(t, err)
	_ = n.Init(context.Background())

	err = n.Shutdown(context.Background())
	assert.NoError(t, err)

	on := n.(*OutputNode)
	assert.Equal(t, lifecycle.StateStopping, on.BaseNode.CurrentState())
}
