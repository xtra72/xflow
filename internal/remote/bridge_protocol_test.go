// bridge_protocol_test.go 는 P1(그룹 RB) 라이브 플로우 브리지 프로토콜의 메시지 Type
// 상수·페이로드 구조·생성자·bridge_id 상관을 검증한다
// (@SPEC:SPEC-SUBFLOW-001 그룹 RB, REQ-SUBFLOW-RB01~RB04, spec §5.14).
//
// 브리지 메시지는 기존 ws.Message 봉투 위에서 command/query/stream RPC 와 대칭하되
// READ+WRITE 양방향이다(read-only query/stream 프록시와 구분 — REQ-SUBFLOW-RB04).
package remote

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBridgeMessageTypeConstants 는 flow-bridge 메시지 Type 상수가 spec §5.14 와
// 일치하고 서로 구분되는지 검증한다(REQ-SUBFLOW-RB03).
func TestBridgeMessageTypeConstants(t *testing.T) {
	assert.Equal(t, "bridge_open", TypeBridgeOpen)
	assert.Equal(t, "bridge_open_ack", TypeBridgeOpenAck)
	assert.Equal(t, "bridge_input", TypeBridgeInput)
	assert.Equal(t, "bridge_output", TypeBridgeOutput)
	assert.Equal(t, "bridge_status", TypeBridgeStatus)
	assert.Equal(t, "bridge_close", TypeBridgeClose)

	// 6개 타입이 모두 서로 다른 문자열이어야 한다(상관/라우팅 모호성 방지).
	all := []string{
		TypeBridgeOpen, TypeBridgeOpenAck, TypeBridgeInput,
		TypeBridgeOutput, TypeBridgeStatus, TypeBridgeClose,
	}
	seen := make(map[string]bool, len(all))
	for _, ty := range all {
		assert.Falsef(t, seen[ty], "브리지 메시지 타입 중복: %q", ty)
		seen[ty] = true
	}

	// 기존 query/stream(read-only) 타입과도 겹치지 않아야 한다(RB04 구분).
	for _, ro := range []string{TypeQuery, TypeQueryResult, TypeSubscribe, TypeStreamData, TypeUnsubscribe, TypeCommand} {
		assert.Falsef(t, seen[ro], "브리지 타입이 read-only/command 타입과 겹침: %q", ro)
	}
}

// TestBridgeStateConstants 는 bridge_status 의 상태 문자열 상수를 검증한다(REQ-SUBFLOW-RB09).
func TestBridgeStateConstants(t *testing.T) {
	assert.Equal(t, "running", BridgeStateRunning)
	assert.Equal(t, "stopped", BridgeStateStopped)
	assert.Equal(t, "offline", BridgeStateOffline)
	assert.Equal(t, "error", BridgeStateError)
}

// TestNewBridgeOpenMessage 는 bridge_open(server→node) 인코딩과 bridge_id 상관을
// 검증한다(REQ-SUBFLOW-RB03). 페이로드 = {bridge_id, instance_id, remote_flow_id}.
func TestNewBridgeOpenMessage(t *testing.T) {
	p := BridgeOpenPayload{
		BridgeID:     "br-1",
		InstanceID:   "node-7f3a",
		RemoteFlowID: "flow-abc123",
	}
	msg, err := NewBridgeOpenMessage(p)
	require.NoError(t, err)
	assert.Equal(t, TypeBridgeOpen, msg.Type)

	var decoded BridgeOpenPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &decoded))
	assert.Equal(t, "br-1", decoded.BridgeID)
	assert.Equal(t, "node-7f3a", decoded.InstanceID)
	assert.Equal(t, "flow-abc123", decoded.RemoteFlowID)
}

// TestNewBridgeOpenAckMessage 는 bridge_open_ack(node→server) 인코딩과 경계 포트
// 보고를 검증한다(REQ-SUBFLOW-RB03/RB06). 노드가 실제 입출력 경계 포트를 보고한다.
func TestNewBridgeOpenAckMessage(t *testing.T) {
	p := BridgeOpenAckPayload{
		BridgeID:    "br-1",
		OK:          true,
		InputPorts:  []string{"in1", "in2"},
		OutputPorts: []string{"out1"},
	}
	msg, err := NewBridgeOpenAckMessage(p)
	require.NoError(t, err)
	assert.Equal(t, TypeBridgeOpenAck, msg.Type)

	var decoded BridgeOpenAckPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &decoded))
	assert.Equal(t, "br-1", decoded.BridgeID)
	assert.True(t, decoded.OK)
	assert.Equal(t, []string{"in1", "in2"}, decoded.InputPorts)
	assert.Equal(t, []string{"out1"}, decoded.OutputPorts)

	// 실패 ack 는 error 를 운반한다.
	failMsg, err := NewBridgeOpenAckMessage(BridgeOpenAckPayload{
		BridgeID: "br-2", OK: false, Error: "remote flow not found",
	})
	require.NoError(t, err)
	var fail BridgeOpenAckPayload
	require.NoError(t, json.Unmarshal(failMsg.Payload, &fail))
	assert.False(t, fail.OK)
	assert.Equal(t, "remote flow not found", fail.Error)
}

// TestNewBridgeInputMessage 는 bridge_input(server→node, WRITE) 인코딩과 port/data
// 운반을 검증한다(REQ-SUBFLOW-RB03/RB04). data 는 json.RawMessage(불투명 페이로드).
func TestNewBridgeInputMessage(t *testing.T) {
	p := BridgeInputPayload{
		BridgeID: "br-1",
		Port:     "in1",
		Data:     json.RawMessage(`{"temp":22.5}`),
	}
	msg, err := NewBridgeInputMessage(p)
	require.NoError(t, err)
	assert.Equal(t, TypeBridgeInput, msg.Type)

	var decoded BridgeInputPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &decoded))
	assert.Equal(t, "br-1", decoded.BridgeID)
	assert.Equal(t, "in1", decoded.Port)
	assert.JSONEq(t, `{"temp":22.5}`, string(decoded.Data))
}

// TestNewBridgeOutputMessage 는 bridge_output(node→server, READ) 인코딩과 port/data
// 운반을 검증한다(REQ-SUBFLOW-RB03). data 는 노드 측 redaction 적용 후의 페이로드이다.
func TestNewBridgeOutputMessage(t *testing.T) {
	p := BridgeOutputPayload{
		BridgeID: "br-1",
		Port:     "out1",
		Data:     json.RawMessage(`{"state":"on"}`),
	}
	msg, err := NewBridgeOutputMessage(p)
	require.NoError(t, err)
	assert.Equal(t, TypeBridgeOutput, msg.Type)

	var decoded BridgeOutputPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &decoded))
	assert.Equal(t, "br-1", decoded.BridgeID)
	assert.Equal(t, "out1", decoded.Port)
	assert.JSONEq(t, `{"state":"on"}`, string(decoded.Data))
}

// TestNewBridgeStatusMessage 는 bridge_status(node→server) 인코딩과 라이프사이클/헬스
// 운반을 검증한다(REQ-SUBFLOW-RB03/RB09).
func TestNewBridgeStatusMessage(t *testing.T) {
	p := BridgeStatusPayload{
		BridgeID: "br-1",
		Status:   BridgeStateRunning,
	}
	msg, err := NewBridgeStatusMessage(p)
	require.NoError(t, err)
	assert.Equal(t, TypeBridgeStatus, msg.Type)

	var decoded BridgeStatusPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &decoded))
	assert.Equal(t, "br-1", decoded.BridgeID)
	assert.Equal(t, BridgeStateRunning, decoded.Status)

	// 오류 상태는 error 를 운반한다.
	errMsg, err := NewBridgeStatusMessage(BridgeStatusPayload{
		BridgeID: "br-1", Status: BridgeStateError, Error: "device read failed",
	})
	require.NoError(t, err)
	var es BridgeStatusPayload
	require.NoError(t, json.Unmarshal(errMsg.Payload, &es))
	assert.Equal(t, BridgeStateError, es.Status)
	assert.Equal(t, "device read failed", es.Error)
}

// TestNewBridgeCloseMessage 는 bridge_close(both) 인코딩과 teardown 상관을 검증한다
// (REQ-SUBFLOW-RB03). 양방향 발신 가능(매니저/노드 어느 쪽이든).
func TestNewBridgeCloseMessage(t *testing.T) {
	p := BridgeClosePayload{BridgeID: "br-1", Reason: "local flow undeployed"}
	msg, err := NewBridgeCloseMessage(p)
	require.NoError(t, err)
	assert.Equal(t, TypeBridgeClose, msg.Type)

	var decoded BridgeClosePayload
	require.NoError(t, json.Unmarshal(msg.Payload, &decoded))
	assert.Equal(t, "br-1", decoded.BridgeID)
	assert.Equal(t, "local flow undeployed", decoded.Reason)
}

// TestBridgeCorrelationByBridgeID 는 한 세션 위 다수 브리지가 bridge_id 로 구분되는지
// 검증한다(REQ-SUBFLOW-RB03/RB12 — 인스턴스 독립). 같은 타입 메시지라도 bridge_id 가
// 다르면 다른 브리지로 라우팅되어야 한다.
func TestBridgeCorrelationByBridgeID(t *testing.T) {
	a, err := NewBridgeOutputMessage(BridgeOutputPayload{BridgeID: "br-a", Port: "out1", Data: json.RawMessage(`1`)})
	require.NoError(t, err)
	b, err := NewBridgeOutputMessage(BridgeOutputPayload{BridgeID: "br-b", Port: "out1", Data: json.RawMessage(`2`)})
	require.NoError(t, err)

	var pa, pb BridgeOutputPayload
	require.NoError(t, json.Unmarshal(a.Payload, &pa))
	require.NoError(t, json.Unmarshal(b.Payload, &pb))
	assert.NotEqual(t, pa.BridgeID, pb.BridgeID, "서로 다른 브리지는 bridge_id 로 구분되어야 한다")
}

// TestBridgeMessageRoundTripThroughEnvelope 는 ws.Message 봉투를 통한 완전한 인코딩→
// 디코딩 라운드트립을 검증한다(REQ-SUBFLOW-RB02 — 기존 ws.Message 봉투 재사용).
func TestBridgeMessageRoundTripThroughEnvelope(t *testing.T) {
	msg, err := NewBridgeInputMessage(BridgeInputPayload{
		BridgeID: "br-1", Port: "in1", Data: json.RawMessage(`{"k":"v"}`),
	})
	require.NoError(t, err)

	// 봉투를 와이어 바이트로 직렬화한 뒤 DecodeMessage 로 복원한다.
	raw, err := json.Marshal(msg)
	require.NoError(t, err)
	decodedEnv, err := DecodeMessage(raw)
	require.NoError(t, err)
	assert.Equal(t, TypeBridgeInput, decodedEnv.Type)

	var p BridgeInputPayload
	require.NoError(t, json.Unmarshal(decodedEnv.Payload, &p))
	assert.Equal(t, "br-1", p.BridgeID)
	assert.Equal(t, "in1", p.Port)
}
