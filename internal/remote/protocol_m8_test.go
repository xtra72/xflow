// protocol_m8_test.go 는 M8(그룹 J) READ/QUERY 프록시 + 스트리밍 프록시 프로토콜의
// 메시지 Type 상수·페이로드 구조·생성자·query-action allowlist 를 검증한다
// (@SPEC:SPEC-REMOTE-001 M8, REQ-J01/J02/J04/J08, spec §5.1/§5.10).
package remote

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQueryMessageTypeConstants 는 M8 메시지 Type 상수가 spec §5.1 과 일치하는지
// 검증한다(REQ-J01/J08).
func TestQueryMessageTypeConstants(t *testing.T) {
	assert.Equal(t, "query", TypeQuery)
	assert.Equal(t, "query_result", TypeQueryResult)
	assert.Equal(t, "subscribe", TypeSubscribe)
	assert.Equal(t, "stream_data", TypeStreamData)
	assert.Equal(t, "unsubscribe", TypeUnsubscribe)
}

// TestNewQueryMessage 는 query 페이로드 인코딩/상관 id 운반을 검증한다(REQ-J02).
func TestNewQueryMessage(t *testing.T) {
	p := QueryPayload{
		QueryID:          "q-1",
		TargetInstanceID: "node-1",
		Domain:           DomainAgent,
		QueryAction:      QueryActionStats,
		Args:             json.RawMessage(`{"id":"a1"}`),
	}
	msg, err := NewQueryMessage(p)
	require.NoError(t, err)
	assert.Equal(t, TypeQuery, msg.Type)

	var decoded QueryPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &decoded))
	assert.Equal(t, "q-1", decoded.QueryID)
	assert.Equal(t, DomainAgent, decoded.Domain)
	assert.Equal(t, QueryActionStats, decoded.QueryAction)
}

// TestNewQueryResultMessage 는 query_result 의 ok/data/error 운반을 검증한다(REQ-J02).
func TestNewQueryResultMessage(t *testing.T) {
	msg, err := NewQueryResultMessage(QueryResultPayload{
		QueryID: "q-2",
		OK:      true,
		Data:    json.RawMessage(`{"online":true}`),
	})
	require.NoError(t, err)
	assert.Equal(t, TypeQueryResult, msg.Type)

	var res QueryResultPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &res))
	assert.Equal(t, "q-2", res.QueryID)
	assert.True(t, res.OK)
	assert.JSONEq(t, `{"online":true}`, string(res.Data))
}

// TestNewSubscribeStreamMessages 는 스트림 메시지(subscribe/stream_data/unsubscribe)
// 인코딩과 subscription_id 운반을 검증한다(REQ-J08/J08b).
func TestNewSubscribeStreamMessages(t *testing.T) {
	sub, err := NewSubscribeMessage(SubscribePayload{
		SubscriptionID:   "s-1",
		TargetInstanceID: "node-1",
		Domain:           DomainDevice,
		StreamAction:     StreamActionState,
		Args:             json.RawMessage(`{"id":"d1"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, TypeSubscribe, sub.Type)

	data, err := NewStreamDataMessage(StreamDataPayload{
		SubscriptionID: "s-1",
		Payload:        json.RawMessage(`{"v":1}`),
	})
	require.NoError(t, err)
	assert.Equal(t, TypeStreamData, data.Type)

	unsub, err := NewUnsubscribeMessage(UnsubscribePayload{SubscriptionID: "s-1"})
	require.NoError(t, err)
	assert.Equal(t, TypeUnsubscribe, unsub.Type)

	var sp SubscribePayload
	require.NoError(t, json.Unmarshal(sub.Payload, &sp))
	assert.Equal(t, "s-1", sp.SubscriptionID)
	assert.Equal(t, StreamActionState, sp.StreamAction)
}

// TestQueryActionAllowlist 는 도메인별 read query-action allowlist 가 FULL 커버리지를
// 포함하고, 변경 의미 action(create/update/delete/execute/start/stop)을 배제하는지
// 검증한다(REQ-J03/J04).
func TestQueryActionAllowlist(t *testing.T) {
	// flow 도메인 — FULL 커버리지.
	for _, qa := range []string{
		QueryActionList, QueryActionGet, QueryActionStatus,
		QueryActionNodes, QueryActionNode, QueryActionLogs,
	} {
		assert.Truef(t, IsAllowedQueryAction(DomainFlow, qa),
			"flow.%s 는 허용 query-action 이어야 함", qa)
	}
	// agent 도메인 — FULL 커버리지.
	for _, qa := range []string{
		QueryActionList, QueryActionGet, QueryActionStats, QueryActionConfig,
		QueryActionDevices, QueryActionTopics, QueryActionStore,
		QueryActionSessions, QueryActionSeries,
	} {
		assert.Truef(t, IsAllowedQueryAction(DomainAgent, qa),
			"agent.%s 는 허용 query-action 이어야 함", qa)
	}
	// device 도메인 — FULL 커버리지.
	for _, qa := range []string{
		QueryActionList, QueryActionGet, QueryActionState,
		QueryActionCommands, QueryActionMetadata,
	} {
		assert.Truef(t, IsAllowedQueryAction(DomainDevice, qa),
			"device.%s 는 허용 query-action 이어야 함", qa)
	}

	// 변경 의미 action 은 어느 도메인에서도 거부(READ-ONLY, REQ-J03).
	for _, dom := range []string{DomainFlow, DomainAgent, DomainDevice} {
		for _, mut := range []string{
			ActionCreate, ActionUpdate, ActionDelete,
			"execute", "start", "stop", "deploy", "restart", "metadata-write",
		} {
			assert.Falsef(t, IsAllowedQueryAction(dom, mut),
				"%s.%s 변경 action 은 query allowlist 에서 배제되어야 함", dom, mut)
		}
	}

	// 미열거 도메인 거부.
	assert.False(t, IsAllowedQueryAction("system", QueryActionGet))
	// 미열거 action 거부.
	assert.False(t, IsAllowedQueryAction(DomainFlow, "stats"),
		"flow 는 stats query-action 을 지원하지 않음(미열거)")
}

// TestStreamActionAllowlist 는 스트림 가능한 action 집합을 검증한다(REQ-J08).
// 스트리밍 대상: device.state, agent.stats, agent.series.
func TestStreamActionAllowlist(t *testing.T) {
	assert.True(t, IsStreamableAction(DomainDevice, StreamActionState))
	assert.True(t, IsStreamableAction(DomainAgent, StreamActionStats))
	assert.True(t, IsStreamableAction(DomainAgent, StreamActionSeries))

	// 비스트림 action 거부.
	assert.False(t, IsStreamableAction(DomainFlow, QueryActionGet))
	assert.False(t, IsStreamableAction(DomainDevice, QueryActionMetadata))
	assert.False(t, IsStreamableAction(DomainAgent, ActionUpdate))
}
