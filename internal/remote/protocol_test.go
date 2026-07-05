package remote

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMessageTypeConstants 는 관리 메시지 Type 상수의 전체 집합이 spec §5.1 과
// 일치하는지 검증한다(REQ-REMOTE-N04).
func TestMessageTypeConstants(t *testing.T) {
	assert.Equal(t, "register", TypeRegister)
	assert.Equal(t, "register_ack", TypeRegisterAck)
	assert.Equal(t, "command", TypeCommand)
	assert.Equal(t, "command_result", TypeCommandResult)
	assert.Equal(t, "inventory_snapshot", TypeInventorySnapshot)
	assert.Equal(t, "inventory_delta", TypeInventoryDelta)
	assert.Equal(t, "heartbeat", TypeHeartbeat)
	assert.Equal(t, "status", TypeStatus)
}

// TestNewHelloMessage 는 hello/heartbeat 가 ws.Message 봉투로 인코딩되고
// instance_id 를 담는지 검증한다(REQ-REMOTE-N04, B01).
func TestNewHelloMessage(t *testing.T) {
	hello := HelloPayload{
		InstanceID: "node-1",
		Hostname:   "host-a",
		Version:    "0.18.6",
	}

	msg, err := NewHelloMessage(hello)
	require.NoError(t, err)
	assert.Equal(t, TypeHello, msg.Type)
	// 봉투 Timestamp 는 RFC3339(기존 NewMessage 규약).
	_, parseErr := time.Parse(time.RFC3339, msg.Timestamp)
	assert.NoError(t, parseErr, "봉투 Timestamp 는 RFC3339 여야 함")

	var decoded HelloPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &decoded))
	assert.Equal(t, hello, decoded)
}

// TestNewHeartbeatMessage 는 heartbeat 페이로드의 ts 가 epoch milliseconds(int64)
// 인지 검증한다(프로젝트 규약, spec §5.1 각주).
func TestNewHeartbeatMessage(t *testing.T) {
	before := time.Now().UnixMilli()
	msg, err := NewHeartbeatMessage("node-1")
	require.NoError(t, err)
	after := time.Now().UnixMilli()

	assert.Equal(t, TypeHeartbeat, msg.Type)

	var hb HeartbeatPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &hb))
	assert.Equal(t, "node-1", hb.InstanceID)
	assert.GreaterOrEqual(t, hb.TS, before, "heartbeat ts 는 epoch ms 여야 함")
	assert.LessOrEqual(t, hb.TS, after)
}

// TestNewStatusMessage 는 status 텔레메트리 페이로드 인코딩을 검증한다.
func TestNewStatusMessage(t *testing.T) {
	msg, err := NewStatusMessage(StatusPayload{
		InstanceID: "node-1",
		Online:     true,
		Health:     "ok",
		TS:         time.Now().UnixMilli(),
	})
	require.NoError(t, err)
	assert.Equal(t, TypeStatus, msg.Type)

	var st StatusPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &st))
	assert.Equal(t, "node-1", st.InstanceID)
	assert.True(t, st.Online)
	assert.Equal(t, "ok", st.Health)
}

// TestHeartbeatRoundTripThroughEnvelope 는 ws.Message 봉투로 인코딩/디코딩한
// heartbeat 가 보존되는지(인프라 호환) 검증한다.
func TestHeartbeatRoundTripThroughEnvelope(t *testing.T) {
	msg, err := NewHeartbeatMessage("node-x")
	require.NoError(t, err)

	encoded, err := msg.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(encoded)
	require.NoError(t, err)
	assert.Equal(t, TypeHeartbeat, decoded.Type)

	var hb HeartbeatPayload
	require.NoError(t, json.Unmarshal(decoded.Payload, &hb))
	assert.Equal(t, "node-x", hb.InstanceID)
}
