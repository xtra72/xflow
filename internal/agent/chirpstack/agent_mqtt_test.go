package chirpstack

import (
	"context"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// newRunningTestAgent 는 브로커에 연결하지 않는(비활성화) 러닝 에이전트를 만든다.
// 트랜스포트 경로(enqueue/ReceiveMessage/State)를 실제 브로커 없이 단위 검증한다.
func newRunningTestAgent(t *testing.T, name string) *ChirpStackAgent {
	t.Helper()
	resetNameRegistryForTest()
	raw, err := NewChirpStackAgent(newTestConfig("id-"+name, name))
	if err != nil {
		t.Fatalf("NewChirpStackAgent: %v", err)
	}
	a, ok := raw.(*ChirpStackAgent)
	if !ok {
		t.Fatalf("unexpected agent type %T", raw)
	}
	return a
}

// TestChirpStackAgent_DisabledStaysCreated 는 비활성화 에이전트가 브로커에 연결하지
// 않고 미연결 상태로 남는지 검증한다 (REQ-M2-04).
func TestChirpStackAgent_DisabledStaysCreated(t *testing.T) {
	a := newRunningTestAgent(t, "disabled-cs")
	if a.TransportConnected() {
		t.Error("disabled agent should not be connected")
	}
	if a.CurrentState() == lifecycle.StateRunning {
		t.Error("disabled agent should not be Running")
	}
}

// TestChirpStackAgent_ReceiveMessage 는 enqueue 한 바이트를 ReceiveMessage 가
// 반환하는지 검증한다 (REQ-M2-02: 수신 채널 노출).
func TestChirpStackAgent_ReceiveMessage(t *testing.T) {
	a := newRunningTestAgent(t, "recv-cs")

	want := []byte(`{"hello":"world"}`)
	a.enqueue(want, "application/1/device/x/event/up")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, err := a.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessage: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("ReceiveMessage = %q, want %q", got, want)
	}
}

// TestChirpStackAgent_StopStopsReceive 는 Stop 후 ReceiveMessage 가 에러로
// 반환되어 소비자 루프가 종료되는지 검증한다 (stopped 가드 / done 채널).
func TestChirpStackAgent_StopStopsReceive(t *testing.T) {
	a := newRunningTestAgent(t, "stop-cs")

	if err := a.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !a.stopped.Load() {
		t.Error("stopped guard should be set after Stop")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := a.ReceiveMessage(ctx); err == nil {
		t.Error("ReceiveMessage after Stop should return error")
	}
}

// TestChirpStackAgent_StateSnapshot 는 State() 가 기본 구독 토픽과 미연결 상태를
// 노출하는지 검증한다 (StatefulAgent).
func TestChirpStackAgent_StateSnapshot(t *testing.T) {
	a := newRunningTestAgent(t, "state-cs")
	st := a.State()

	if st["connected"] != false {
		t.Errorf("connected = %v, want false", st["connected"])
	}
	topics, ok := st["topics"].([]string)
	if !ok || len(topics) == 0 || topics[0] != "application/#" {
		t.Errorf("topics = %v, want [application/#]", st["topics"])
	}
}

// TestChirpStackAgent_EnabledInterfaceAssertions 는 선택 인터페이스 구현을 보증한다.
func TestChirpStackAgent_EnabledInterfaceAssertions(t *testing.T) {
	a := newRunningTestAgent(t, "iface-cs")
	var _ agent.MessageReceiver = a
	var _ agent.TransportChecker = a
	var _ agent.StatefulAgent = a
}
