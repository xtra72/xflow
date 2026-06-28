// bridge_envelope_test.go 는 라이브 브리지 직렬화(messageToJSON/messageFromJSON)가 메시지
// 봉투(id/type/timestamp/payload/metadata) 전체를 왕복 보존하는지 검증한다
// (@SPEC:SPEC-SUBFLOW-001 그룹 RB, REQ-SUBFLOW-RB06). 회귀 대상: flow-node 원격 브리지를
// 통과한 출력이 metadata/type/id 를 잃고 payload 만 살아남던 버그.
package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/message"
)

// TestBridgeEnvelope_RoundTripPreservesAllFields 는 보고된 예시 메시지(점 표기 메타데이터
// 포함)가 messageToJSON→messageFromJSON 왕복 후 id/type/timestamp/metadata/payload 를
// 모두 보존하는지 검증한다(버그 재현: type/metadata/id 손실).
func TestBridgeEnvelope_RoundTripPreservesAllFields(t *testing.T) {
	ts := time.UnixMilli(1780980196676)
	orig := message.New(
		message.WithID("f7b30000-0000-0000-0000-000000000001"),
		message.WithType("event"),
		message.WithTimestamp(ts),
		message.WithPayload(message.NewPayload(map[string]any{
			"data": "hello",
			"raw":  "726177",
		})),
		message.WithMetadata("serial.agent_type", "serial"),
		message.WithMetadata("serial.node_id", "serial-in-1"),
	)

	wire := messageToJSON(orig)
	got, err := messageFromJSON(wire)
	require.NoError(t, err)

	assert.Equal(t, orig.ID(), got.ID(), "id 보존")
	assert.Equal(t, "event", got.Type(), "type 보존")
	assert.Equal(t, ts.UnixMilli(), got.Timestamp().UnixMilli(), "timestamp(epoch ms) 보존")

	md := got.Metadata().All()
	assert.Equal(t, "serial", md["serial.agent_type"], "점 표기 메타데이터 보존")
	assert.Equal(t, "serial-in-1", md["serial.node_id"], "다중 메타데이터 키 보존")

	pm := got.Payload().ToMap()
	assert.Equal(t, "hello", pm["data"], "payload.data 보존")
	assert.Equal(t, "726177", pm["raw"], "payload.raw 보존")
}

// TestBridgeEnvelope_WireShapeMatchesOutputNode 는 messageToJSON 의 와이어 형태가 출력/디버그
// 노드(buildMessageMap)의 코어 필드 {id,type,timestamp,payload,metadata} 와 정확히 일치하며
// timestamp 가 epoch ms 숫자임을 검증한다.
func TestBridgeEnvelope_WireShapeMatchesOutputNode(t *testing.T) {
	ts := time.UnixMilli(1780980196676)
	orig := message.New(
		message.WithID("abc"),
		message.WithType("event"),
		message.WithTimestamp(ts),
		message.WithPayload(message.NewPayload(map[string]any{"data": "x"})),
		message.WithMetadata("k", "v"),
	)

	wire := messageToJSON(orig)

	var m map[string]any
	require.NoError(t, json.Unmarshal(wire, &m))

	// 정확히 코어 5개 키만(디버그 전용 time/level/name 제외).
	gotKeys := make([]string, 0, len(m))
	for k := range m {
		gotKeys = append(gotKeys, k)
	}
	assert.ElementsMatch(t, []string{"id", "type", "timestamp", "payload", "metadata"}, gotKeys,
		"와이어 형태는 출력 노드 코어 필드와 일치")

	assert.Equal(t, "abc", m["id"])
	assert.Equal(t, "event", m["type"])
	// timestamp 는 JSON 숫자(epoch ms) — json.Unmarshal 은 float64 로 디코드.
	tsNum, ok := m["timestamp"].(float64)
	require.True(t, ok, "timestamp 는 숫자(epoch ms)")
	assert.Equal(t, float64(1780980196676), tsNum)

	pm, ok := m["payload"].(map[string]any)
	require.True(t, ok, "payload 는 객체")
	assert.Equal(t, "x", pm["data"])

	md, ok := m["metadata"].(map[string]any)
	require.True(t, ok, "metadata 는 객체")
	assert.Equal(t, "v", md["k"])
}

// TestBridgeEnvelope_RoundTripPreservesGroups 는 nested group 메타데이터가
// messageToJSON→messageFromJSON 왕복 후 group 으로 보존되는지 검증한다 (P2 Class A:
// 라이브 브리지가 group 을 운반해야 한다). flat 키는 string 으로 유지된다.
func TestBridgeEnvelope_RoundTripPreservesGroups(t *testing.T) {
	ts := time.UnixMilli(1780980196676)
	orig := message.New(
		message.WithID("f7b30000-0000-0000-0000-000000000002"),
		message.WithType("event"),
		message.WithTimestamp(ts),
		message.WithPayload(message.NewPayload(map[string]any{"data": "hello"})),
		message.WithMetadata("flatKey", "flatVal"),
	)
	orig.Metadata().SetGroup("agent", map[string]string{"type": "serial", "id": "node-1"})

	wire := messageToJSON(orig)
	got, err := messageFromJSON(wire)
	require.NoError(t, err)

	// flat 키는 string 으로 보존
	if v, ok := got.Metadata().Get("flatKey"); !ok || v != "flatVal" {
		t.Errorf("flatKey = (%q, %v), 기대값 (\"flatVal\", true)", v, ok)
	}

	// group 은 group 으로 보존
	agent, ok := got.Metadata().GetGroup("agent")
	require.True(t, ok, "왕복 후 agent group 이 손실되었다")
	assert.Equal(t, "serial", agent["type"])
	assert.Equal(t, "node-1", agent["id"])
}

// TestBridgeEnvelope_GracefulFallbackNonEnvelope 는 봉투가 아닌 입력(스칼라/배열, payload 키
// 없는 객체)이 회귀 없이 사용 가능한 메시지를 만드는지 검증한다.
func TestBridgeEnvelope_GracefulFallbackNonEnvelope(t *testing.T) {
	t.Run("scalar wrapped in value", func(t *testing.T) {
		msg, err := messageFromJSON(json.RawMessage(`42`))
		require.NoError(t, err)
		v, ok := msg.Payload().Get("value")
		require.True(t, ok)
		assert.Equal(t, float64(42), v)
	})

	t.Run("non-envelope object passes through as payload", func(t *testing.T) {
		msg, err := messageFromJSON(json.RawMessage(`{"foo":"bar"}`))
		require.NoError(t, err)
		v, ok := msg.Payload().Get("foo")
		require.True(t, ok)
		assert.Equal(t, "bar", v)
	})

	t.Run("empty data yields empty-payload message", func(t *testing.T) {
		msg, err := messageFromJSON(nil)
		require.NoError(t, err)
		require.NotNil(t, msg)
		assert.Empty(t, msg.Payload().Keys())
	})

	t.Run("array passes through wrapped in value", func(t *testing.T) {
		msg, err := messageFromJSON(json.RawMessage(`[1,2,3]`))
		require.NoError(t, err)
		v, ok := msg.Payload().Get("value")
		require.True(t, ok)
		assert.Len(t, v, 3)
	})
}

// TestBridgeEnvelope_DetectionRequiresSiblingKey 는 봉투 판별식 강화를 검증한다:
//   - {"payload": {...}} (형제 봉투 키 없음) → 봉투 아님 → 객체 전체가 payload
//     (즉 payload 키가 그대로 payload 안에 보존됨).
//   - {"payload": {...}, "type": "event"} (형제 봉투 키 있음) → 봉투 → 복원.
func TestBridgeEnvelope_DetectionRequiresSiblingKey(t *testing.T) {
	t.Run("payload-only is treated as raw non-envelope payload", func(t *testing.T) {
		msg, err := messageFromJSON(json.RawMessage(`{"payload":{"x":1}}`))
		require.NoError(t, err)
		// 봉투가 아니므로 객체 전체가 payload — payload["payload"] == {"x":1}.
		pm := msg.Payload().ToMap()
		inner, ok := pm["payload"].(map[string]any)
		require.True(t, ok, "payload 키가 원시 페이로드 안에 보존되어야 함")
		assert.Equal(t, float64(1), inner["x"])
	})

	t.Run("payload with sibling type is an envelope", func(t *testing.T) {
		msg, err := messageFromJSON(json.RawMessage(`{"payload":{"x":1},"type":"event"}`))
		require.NoError(t, err)
		assert.Equal(t, "event", msg.Type(), "형제 type 키가 있으면 봉투로 복원")
		pm := msg.Payload().ToMap()
		assert.Equal(t, float64(1), pm["x"], "봉투 payload 가 메시지 payload 로 펼쳐짐")
		_, hasNested := pm["payload"]
		assert.False(t, hasNested, "봉투이면 payload 키가 중첩되지 않음")
	})

	t.Run("payload with sibling id is an envelope", func(t *testing.T) {
		msg, err := messageFromJSON(json.RawMessage(`{"payload":{"x":1},"id":"abc"}`))
		require.NoError(t, err)
		assert.Equal(t, "abc", msg.ID())
	})

	t.Run("payload with sibling metadata is an envelope", func(t *testing.T) {
		msg, err := messageFromJSON(json.RawMessage(`{"payload":{"x":1},"metadata":{"k":"v"}}`))
		require.NoError(t, err)
		assert.Equal(t, "v", msg.Metadata().All()["k"])
	})
}

// TestBridgeEnvelope_FloatTimestampTolerated 는 JSON 숫자가 float64 로 디코드되어도 timestamp
// 가 epoch ms 로 복원됨을 검증한다(직접 envelope JSON 주입 경로).
func TestBridgeEnvelope_FloatTimestampTolerated(t *testing.T) {
	env := json.RawMessage(`{"id":"x","type":"event","timestamp":1780980196676,"payload":{"data":"y"},"metadata":{"a":"b"}}`)
	got, err := messageFromJSON(env)
	require.NoError(t, err)
	assert.Equal(t, "x", got.ID())
	assert.Equal(t, "event", got.Type())
	assert.Equal(t, int64(1780980196676), got.Timestamp().UnixMilli())
	assert.Equal(t, "b", got.Metadata().All()["a"])
	pm := got.Payload().ToMap()
	assert.Equal(t, "y", pm["data"])
}
