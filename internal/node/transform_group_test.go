package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// msgWithGroup 은 flat 키와 nested group 을 가진 입력 메시지를 만든다.
func msgWithGroup() message.Message {
	msg := message.New(
		message.WithType("event"),
		message.WithMetadata("flatKey", "flatVal"),
		message.WithPayload(message.NewPayload(map[string]any{"temp": 21, "humidity": 55})),
	)
	msg.Metadata().SetGroup("agent", map[string]string{"type": "serial", "id": "node-1"})
	return msg
}

// TestTransformNode_ExpressionSelect_PreservesGroup 는 expression(select) 변환이
// 출력 메시지에서 nested group 을 보존하는지 검증한다 (P2 Class B: transform.go).
func TestTransformNode_ExpressionSelect_PreservesGroup(t *testing.T) {
	def := flow.NewNodeDef("transform-grp", "transform")
	n, _ := NewTransformNode(def)

	// select 모드: payload 에서 temp 만 추출 → 메시지 재구성 경로.
	require.NoError(t, n.Configure(map[string]any{
		"expression": "{ temp: $.payload.temp }",
		"mode":       "select",
	}))

	out, err := n.Process(context.Background(), msgWithGroup())
	require.NoError(t, err)
	require.Len(t, out, 1)

	// flat 키는 보존
	v, ok := out[0].Metadata().Get("flatKey")
	assert.True(t, ok)
	assert.Equal(t, "flatVal", v)

	// group 도 보존되어야 한다
	agent, ok := out[0].Metadata().GetGroup("agent")
	require.True(t, ok, "transform select 후 agent group 이 손실되었다")
	assert.Equal(t, "serial", agent["type"])
	assert.Equal(t, "node-1", agent["id"])
}

// TestTransformNode_MetadataExpression_PreservesGroup 는 metadata_expression 경로가
// 출력 메시지에서 원본 group 을 보존하는지 검증한다 (P2 Class B: transform.go ~229).
func TestTransformNode_MetadataExpression_PreservesGroup(t *testing.T) {
	def := flow.NewNodeDef("transform-meta-grp", "transform")
	n, _ := NewTransformNode(def)

	require.NoError(t, n.Configure(map[string]any{
		"metadata_expression": "{ added: \"yes\" }",
		"metadata_mode":       "merge",
	}))

	out, err := n.Process(context.Background(), msgWithGroup())
	require.NoError(t, err)
	require.Len(t, out, 1)

	agent, ok := out[0].Metadata().GetGroup("agent")
	require.True(t, ok, "metadata_expression 후 agent group 이 손실되었다")
	assert.Equal(t, "serial", agent["type"])
}
