package century

import (
	"context"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// TransportConnected() lock-free 회귀 테스트 (성능 버그 재현)
//
// 배경: `agent list` 는 server.ListAgents → agentToHandlerInfo →
// TransportConnected() 경로로 트랜스포트 liveness 를 조회한다.
// TransportConnected() 이 처리 루프/재연결 경로가 잡을 수 있는 a.mu 를 획득하면,
// 그 lock 이 풀릴 때까지 `agent list` 가 블록된다.
//
// 본 테스트는 다른 goroutine 이 a.mu 의 write lock 을 잡고 있는 동안에도
// TransportConnected() 이 지정 시간(50ms) 내에 반환되는지 검증한다.
// lock-free 수정 전(RLock 사용)에는 write lock 에 막혀 타임아웃으로 실패하고,
// 수정 후에는 즉시 통과한다.
// ---------------------------------------------------------------------------

// TestTransportConnected_LockFreeWhileMuHeld 는 a.mu.Lock() 이 외부 goroutine 에
// 의해 보유 중일 때도 TransportConnected() 이 즉시 반환되는지 검증한다.
func TestTransportConnected_LockFreeWhileMuHeld(t *testing.T) {
	rt := newRecordingTransport(nil)
	opts := map[string]any{"serial_port": "/dev/ttyTEST"}
	centuryCfg, err := parseHvacr01Config(opts)
	if err != nil {
		t.Fatalf("parseHvacr01Config: %v", err)
	}
	cfg := agent.AgentConfig{
		ID:        "century-lockfree-test",
		Name:      "century-lockfree-test",
		Type:      "century_hvacr01",
		Transport: agent.TransportConfig{Type: "serial", Options: opts},
	}
	a := newHvacr01AgentForTest(cfg, centuryCfg, rt)
	if err := a.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = a.Stop(context.Background()) }()

	// 외부 goroutine 이 a.mu 의 write lock 을 잡고 잠시 보유한다
	// (처리 루프/재연결이 a.transport 를 교체하는 상황 모사).
	muHeld := make(chan struct{})
	releaseMu := make(chan struct{})
	go func() {
		a.mu.Lock()
		close(muHeld)
		<-releaseMu
		a.mu.Unlock()
	}()

	<-muHeld // write lock 이 확실히 잡힌 상태.

	// 핵심 단언: write lock 보유 중에도 TransportConnected() 은 즉시 반환되어야 한다.
	done := make(chan bool, 1)
	go func() {
		done <- a.TransportConnected()
	}()

	select {
	case <-done:
		// 즉시 반환됨 — 정상.
	case <-time.After(50 * time.Millisecond):
		close(releaseMu)
		t.Fatal("TransportConnected() 이 50ms 내에 반환되지 않음 — a.mu write lock 에 막힘 (lock-free 위반)")
	}

	close(releaseMu)
}
