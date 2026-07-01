package ws

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/message"
)

// countingBroadcaster 는 BroadcastMessage 호출 횟수와 마지막 페이로드를 기록하는
// 테스트용 messageBroadcaster 구현이다.
type countingBroadcaster struct {
	mu          sync.Mutex
	calls       atomic.Int64
	lastType    string
	lastPayload any
}

func (c *countingBroadcaster) BroadcastMessage(msgType string, payload any) error {
	c.calls.Add(1)
	c.mu.Lock()
	c.lastType = msgType
	c.lastPayload = payload
	c.mu.Unlock()
	return nil
}

func (c *countingBroadcaster) snapshot() (string, any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastType, c.lastPayload
}

// ---------------------------------------------------------------------------
// TapRegistry 테스트
// ---------------------------------------------------------------------------

func TestTapRegistry_DefaultNotTapped(t *testing.T) {
	r := NewTapRegistry()
	assert.False(t, r.IsTapped("flow-1", "node-1"), "노드는 기본적으로 tap 되지 않아야 한다")
}

func TestTapRegistry_SetTapEnableDisable(t *testing.T) {
	r := NewTapRegistry()

	r.SetTap("flow-1", "node-1", true)
	assert.True(t, r.IsTapped("flow-1", "node-1"))
	// 다른 노드/플로우는 영향받지 않는다.
	assert.False(t, r.IsTapped("flow-1", "node-2"))
	assert.False(t, r.IsTapped("flow-2", "node-1"))

	r.SetTap("flow-1", "node-1", false)
	assert.False(t, r.IsTapped("flow-1", "node-1"))
}

func TestTapRegistry_TappedNodesForFlow(t *testing.T) {
	r := NewTapRegistry()
	r.SetTap("flow-1", "node-1", true)
	r.SetTap("flow-1", "node-2", true)
	r.SetTap("flow-2", "node-9", true)

	got := r.TappedNodes("flow-1")
	assert.ElementsMatch(t, []string{"node-1", "node-2"}, got)

	got2 := r.TappedNodes("flow-2")
	assert.ElementsMatch(t, []string{"node-9"}, got2)

	assert.Empty(t, r.TappedNodes("flow-unknown"))
}

func TestTapRegistry_ConcurrentAccess(t *testing.T) {
	r := NewTapRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.SetTap("flow-1", "node-1", true)
			_ = r.IsTapped("flow-1", "node-1")
			_ = r.TappedNodes("flow-1")
			r.SetTap("flow-1", "node-1", false)
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// TapObserver 테스트
// ---------------------------------------------------------------------------

func TestTapObserver_TappedNodeBroadcasts(t *testing.T) {
	reg := NewTapRegistry()
	bc := &countingBroadcaster{}
	obs := NewTapObserver(reg, bc)

	reg.SetTap("flow-1", "node-1", true)

	pl := message.NewPayload(map[string]any{"temp": 21.5})
	msg := message.New(
		message.WithID("msg-123"),
		message.WithType("device.state"),
		message.WithPayload(pl),
	)

	obs.OnNodeOutput("flow-1", "node-1", "out", msg)

	require.Equal(t, int64(1), bc.calls.Load(), "tap된 노드는 정확히 한 번 broadcast 되어야 한다")

	msgType, payload := bc.snapshot()
	assert.Equal(t, TypeNodeOutput, msgType)

	np, ok := payload.(NodeOutputPayload)
	require.True(t, ok, "payload는 NodeOutputPayload 타입이어야 한다")
	assert.Equal(t, "flow-1", np.FlowID)
	assert.Equal(t, "node-1", np.NodeID)
	assert.Equal(t, "out", np.Port)

	// Message 는 debug 노드(buildMessageMap)와 동일한 whole-message 맵 형태이다:
	// id / type / time(RFC3339) / timestamp(epoch ms) / name / payload / metadata.
	mm := np.Message
	assert.Equal(t, "msg-123", mm["id"])
	assert.Equal(t, "device.state", mm["type"])
	assert.Contains(t, mm, "time")
	assert.Contains(t, mm, "timestamp")
	assert.Contains(t, mm, "payload")
	assert.Contains(t, mm, "metadata")
	// timestamp 는 epoch ms (int64) — debug 노드와 동일.
	_, isInt64 := mm["timestamp"].(int64)
	assert.True(t, isInt64, "timestamp는 epoch ms(int64)여야 한다")
	// payload 내용 보존
	plMap, ok := mm["payload"].(map[string]any)
	require.True(t, ok)
	assert.InDelta(t, 21.5, plMap["temp"], 0.001)

	// 전체 페이로드가 JSON 직렬화 가능해야 한다 (WS 브로드캐스트 호환).
	_, err := json.Marshal(np)
	require.NoError(t, err)
}

// TestTapObserver_EgressSlimsGroupsByDefault 는 expander 미설정(기본) 시 node.output
// egress 메타데이터의 agent / device 그룹이 id-only 로 슬림화됨을 검증한다
// (message-slim-metadata). group 형태는 유지되고 type/name 만 wire 에서 제거된다.
func TestTapObserver_EgressSlimsGroupsByDefault(t *testing.T) {
	reg := NewTapRegistry()
	bc := &countingBroadcaster{}
	obs := NewTapObserver(reg, bc)
	reg.SetTap("flow-1", "node-1", true)

	msg := message.New(message.WithID("m1"), message.WithType("event"))
	msg.Metadata().SetGroup("agent", map[string]string{"type": "serial", "id": "a-1", "name": "reader"})
	msg.Metadata().SetGroup("device", map[string]string{"type": "HVACR.IDU", "id": "d-1", "name": "room1"})

	obs.OnNodeOutput("flow-1", "node-1", "out", msg)

	_, payload := bc.snapshot()
	np := payload.(NodeOutputPayload)
	md := np.Message["metadata"].(map[string]any)

	agent := md["agent"].(map[string]string)
	assert.Equal(t, map[string]string{"id": "a-1"}, agent, "agent 그룹은 id-only 슬림")
	device := md["device"].(map[string]string)
	assert.Equal(t, map[string]string{"id": "d-1"}, device, "device 그룹은 id-only 슬림")

	// 원본 메시지는 변형되지 않아야 한다(내부 흐름 무영향).
	origAgent, _ := msg.Metadata().GetGroup("agent")
	assert.Equal(t, "serial", origAgent["type"], "원본 agent 그룹은 full 유지")
}

// TestTapObserver_EgressExpandsWhenExpanderSet 는 expander 설정(expand opt-in) 시
// node.output egress 메타데이터의 그룹이 레지스트리 조회로 type/name 까지 역-수화됨을
// 검증한다 (message-slim-metadata).
func TestTapObserver_EgressExpandsWhenExpanderSet(t *testing.T) {
	reg := NewTapRegistry()
	bc := &countingBroadcaster{}
	obs := NewTapObserver(reg, bc).WithExpander(&message.GroupExpander{
		Agent: func(id string) (map[string]string, bool) {
			if id == "a-1" {
				return map[string]string{"type": "serial", "name": "reader"}, true
			}
			return nil, false
		},
	})
	reg.SetTap("flow-1", "node-1", true)

	msg := message.New(message.WithID("m1"), message.WithType("event"))
	msg.Metadata().SetGroup("agent", map[string]string{"id": "a-1"})

	obs.OnNodeOutput("flow-1", "node-1", "out", msg)

	_, payload := bc.snapshot()
	np := payload.(NodeOutputPayload)
	md := np.Message["metadata"].(map[string]any)
	agent := md["agent"].(map[string]string)
	assert.Equal(t, "a-1", agent["id"])
	assert.Equal(t, "serial", agent["type"], "expand=true 시 type 역-수화")
	assert.Equal(t, "reader", agent["name"], "expand=true 시 name 역-수화")
}

func TestTapObserver_UntappedNodeNoBroadcast(t *testing.T) {
	reg := NewTapRegistry()
	bc := &countingBroadcaster{}
	obs := NewTapObserver(reg, bc)

	// node-1 만 tap, node-2 는 tap 하지 않음.
	reg.SetTap("flow-1", "node-1", true)

	msg := message.New(message.WithID("msg-x"))
	obs.OnNodeOutput("flow-1", "node-2", "out", msg)

	assert.Equal(t, int64(0), bc.calls.Load(),
		"tap 되지 않은 노드는 broadcast(직렬화)되지 않아야 한다 (fast-path)")
}

func TestTapObserver_NilMessageSafe(t *testing.T) {
	reg := NewTapRegistry()
	bc := &countingBroadcaster{}
	obs := NewTapObserver(reg, bc)
	reg.SetTap("flow-1", "node-1", true)

	require.NotPanics(t, func() {
		obs.OnNodeOutput("flow-1", "node-1", "out", nil)
	})
	// nil 메시지는 broadcast 하지 않는다.
	assert.Equal(t, int64(0), bc.calls.Load())
}
