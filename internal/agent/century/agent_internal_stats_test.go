package century

import (
	"context"
	"testing"
	"time"
)

// TestAgent_InternalStats_OnProcess 는 v0.6.3 회귀 테스트이다.
//
// 사용자 보고: "내부간 송수신(에이전트와 노드간) 통계, 운영 통계 누락".
// Process() 호출 시 표준 agent.AgentStats 의 InternalMessagesReceived /
// InternalMessagesSent 가 증가해야 한다 (NASA / LGCNP 패턴).
func TestAgent_InternalStats_OnProcess(t *testing.T) {
	t.Parallel()
	a, _, cleanup := makeTestAgent(t, nil, nil)
	defer cleanup()

	before := a.stats.Snapshot()

	// get_stats 호출 (가장 가벼운 Process 명령).
	_, err := a.Process([]byte(`{"command":"get_stats","node_id":"n1","flow_id":"f1"}`))
	if err != nil {
		t.Fatalf("Process(get_stats) error: %v", err)
	}

	after := a.stats.Snapshot()

	if got := after.InternalMessagesReceived - before.InternalMessagesReceived; got != 1 {
		t.Errorf("InternalMessagesReceived delta = %d, want 1", got)
	}
	if got := after.InternalMessagesSent - before.InternalMessagesSent; got != 1 {
		t.Errorf("InternalMessagesSent delta = %d, want 1", got)
	}
}

// TestAgent_InternalStats_InvalidJSON 는 Process 가 invalid JSON 을 받았을
// 때 received + errored 가 증가하는지 검증한다.
func TestAgent_InternalStats_InvalidJSON(t *testing.T) {
	t.Parallel()
	a, _, cleanup := makeTestAgent(t, nil, nil)
	defer cleanup()

	before := a.stats.Snapshot()

	_, err := a.Process([]byte(`not valid json`))
	if err == nil {
		t.Fatal("Process: want error for invalid JSON")
	}

	after := a.stats.Snapshot()

	if got := after.InternalMessagesReceived - before.InternalMessagesReceived; got != 1 {
		t.Errorf("InternalMessagesReceived delta = %d, want 1 (invalid request still counted)", got)
	}
	// MessagesErrored 는 total + internal 모두 증가.
	if got := after.MessagesErrored - before.MessagesErrored; got != 1 {
		t.Errorf("MessagesErrored delta = %d, want 1 (invalid JSON)", got)
	}
}

// TestAgent_InternalStats_OnReceiveMessage 는 ReceiveMessage 호출이 msgCh
// 의 한 이벤트를 노드에게 전달할 때 InternalMessagesSent 가 증가하는지
// 검증한다.
func TestAgent_InternalStats_OnReceiveMessage(t *testing.T) {
	t.Parallel()
	batch := append([]byte{}, mustBuildReg02ResponseFrame(t, 0x3B)...)
	batch = append(batch, mustBuildReg04ResponseFrame(t, 0x3B)...)
	a, _, cleanup := makeTestAgent(t, nil, batch)
	defer cleanup()

	// device_state event 가 msgCh 로 흘러나올 때까지 대기.
	a.bridgeActive.Store(true)
	waitUntil(t, 2*time.Second, func() bool {
		return a.cStats.framesCaptured.Load() >= 2
	}, "frames not captured")

	before := a.stats.Snapshot()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	data, err := a.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessage: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("ReceiveMessage: empty payload")
	}

	after := a.stats.Snapshot()

	if got := after.InternalMessagesSent - before.InternalMessagesSent; got != 1 {
		t.Errorf("InternalMessagesSent delta = %d, want 1 (ReceiveMessage push)", got)
	}
}
