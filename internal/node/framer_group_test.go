package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFramerNode_FrameMessage_PreservesGroup 는 framer 가 입력 메시지의 nested group 을
// 출력 프레임 메시지로 전파하는지 검증한다 (P2 Class B: framer.go makeFrameMessageLocked).
func TestFramerNode_FrameMessage_PreservesGroup(t *testing.T) {
	fn := newFramerNodeForTest(t, "framer-grp", "newline")
	require.NoError(t, fn.Init(context.Background()))

	in := inputMsgWithRaw([]byte("hello\n"), "flatKey", "flatVal")
	in.Metadata().SetGroup("agent", map[string]string{"type": "serial", "id": "node-1"})

	results, err := fn.Process(context.Background(), in)
	require.NoError(t, err)
	outMsgs, _ := splitResults(results)
	require.Len(t, outMsgs, 1)

	// flat 키 보존
	v, ok := outMsgs[0].Metadata().Get("flatKey")
	assert.True(t, ok)
	assert.Equal(t, "flatVal", v)

	// group 보존
	agent, ok := outMsgs[0].Metadata().GetGroup("agent")
	require.True(t, ok, "framer 프레임 메시지에서 agent group 이 손실되었다")
	assert.Equal(t, "serial", agent["type"])
	assert.Equal(t, "node-1", agent["id"])
}
