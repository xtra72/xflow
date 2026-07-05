// protocol_m2_test.go 는 M2(등록/승인) 프로토콜 페이로드의 인코딩/디코딩을
// 검증한다(REQ-REMOTE-C01/C03/C04, spec §5.1).
package remote

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewRegisterMessage 는 register 메시지가 ws.Message 봉투로 인코딩되고
// instance_id + 노드 메타 + 노출 요약을 담는지 검증한다(REQ-C01).
func TestNewRegisterMessage(t *testing.T) {
	p := RegisterPayload{
		InstanceID:      "node-r1",
		Hostname:        "host-r1",
		Version:         "1.2.3",
		Exposure:        ExposureSummary{Flows: "all", Agents: "none", Devices: "all"},
		BootstrapSecret: "s3cr3t",
	}

	msg, err := NewRegisterMessage(p)
	require.NoError(t, err)
	assert.Equal(t, TypeRegister, msg.Type)

	var decoded RegisterPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &decoded))
	assert.Equal(t, p, decoded)
}

// TestNewRegisterAckMessage_Pending 는 pending 응답에 토큰이 없는지 검증한다(REQ-C02).
func TestNewRegisterAckMessage_Pending(t *testing.T) {
	msg, err := NewRegisterAckMessage(RegisterAckPayload{Status: RegStatusPending})
	require.NoError(t, err)
	assert.Equal(t, TypeRegisterAck, msg.Type)

	var ack RegisterAckPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &ack))
	assert.Equal(t, RegStatusPending, ack.Status)
	assert.Empty(t, ack.NodeToken, "pending 응답은 토큰을 포함하지 않아야 함")
}

// TestNewRegisterAckMessage_Approved 는 approved 응답에 node_token 이 포함되는지
// 검증한다(REQ-C04).
func TestNewRegisterAckMessage_Approved(t *testing.T) {
	msg, err := NewRegisterAckMessage(RegisterAckPayload{
		Status:    RegStatusApproved,
		NodeToken: "jwt-token",
	})
	require.NoError(t, err)

	var ack RegisterAckPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &ack))
	assert.Equal(t, RegStatusApproved, ack.Status)
	assert.Equal(t, "jwt-token", ack.NodeToken)
}

// TestNewRegisterAckMessage_Rejected 는 rejected 응답에 사유가 포함될 수 있는지
// 검증한다(REQ-C03).
func TestNewRegisterAckMessage_Rejected(t *testing.T) {
	msg, err := NewRegisterAckMessage(RegisterAckPayload{
		Status: RegStatusRejected,
		Reason: "정책 위반",
	})
	require.NoError(t, err)

	var ack RegisterAckPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &ack))
	assert.Equal(t, RegStatusRejected, ack.Status)
	assert.Equal(t, "정책 위반", ack.Reason)
}

// TestRegistrationStatusConstants 는 등록 상태 문자열 상수를 검증한다(spec §5.6).
func TestRegistrationStatusConstants(t *testing.T) {
	assert.Equal(t, "pending", RegStatusPending)
	assert.Equal(t, "approved", RegStatusApproved)
	assert.Equal(t, "rejected", RegStatusRejected)
	assert.Equal(t, "revoked", RegStatusRevoked)
}
