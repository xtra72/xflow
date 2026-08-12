package chirpstack

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// newDisabledAgent 는 브로커 연결 없이(비활성화) 생성한 에이전트를 만든다.
// 비활성화 상태에서는 Init 이 MQTT 연결/watchdog 기동을 건너뛰므로 lifecycle 만
// 독립적으로 검증할 수 있다.
func newDisabledAgent(t *testing.T, name string, opts map[string]any) (*ChirpStackAgent, agent.AgentConfig) {
	t.Helper()
	disabled := false
	cfg := agent.AgentConfig{
		ID:        "id-" + name,
		Name:      name,
		Type:      "chirpstack",
		Enabled:   &disabled,
		Transport: agent.TransportConfig{Options: opts},
	}
	raw, err := NewChirpStackAgent(cfg)
	if err != nil {
		t.Fatalf("NewChirpStackAgent: %v", err)
	}
	a, ok := raw.(*ChirpStackAgent)
	if !ok {
		t.Fatalf("unexpected agent type %T", raw)
	}
	return a, cfg
}

// withOptions 는 기존 설정의 Transport.Options 만 교체한 사본을 만든다.
func withOptions(cfg agent.AgentConfig, opts map[string]any) agent.AgentConfig {
	next := cfg
	next.Transport = agent.TransportConfig{Options: opts}
	return next
}

// ---------------------------------------------------------------------------
// FIX-2: Configure 가 csConfig 를 재파싱한다
// ---------------------------------------------------------------------------

// TestConfigure_ReparsesChirpStackConfig 는 Configure 가 raw config 맵뿐 아니라
// 파싱된 csConfig 까지 갱신하는지 검증한다.
//
// 이전에는 csConfig 가 NewChirpStackAgent 에서 1회만 파싱되어, UI 에서 저장한
// emit_comm_state 토글이 조용히 무시되었다(에이전트 재시작 없이는 영원히 반영 안 됨).
func TestConfigure_ReparsesChirpStackConfig(t *testing.T) {
	a, cfg := newDisabledAgent(t, "cs-cfg-reparse", map[string]any{
		"emit_comm_state": false,
		"broker":          "tcp://localhost:1883",
	})

	if a.cs().EmitCommState {
		t.Fatal("초기 emit_comm_state 는 false 여야 한다 (테스트 전제)")
	}

	next := withOptions(cfg, map[string]any{
		"emit_comm_state": true,
		"broker":          "tcp://localhost:1883",
	})
	if err := a.Configure(next); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	if !a.cs().EmitCommState {
		t.Error("Configure 후 파싱된 csConfig.EmitCommState 가 true 여야 한다 " +
			"(raw config 만 갱신하고 csConfig 를 방치하면 변경이 조용히 무시된다)")
	}
	// 공개 조회 경로(status 노드가 사용)에서도 동일하게 보여야 한다.
	if !a.CommStateEnabled() {
		t.Error("CommStateEnabled() 가 갱신된 설정을 반영하지 않는다")
	}
}

// TestConfigure_ReparsesNonBooleanKnobs 는 duration / 숫자 노브도 함께 재파싱되는지
// 검증한다.
func TestConfigure_ReparsesNonBooleanKnobs(t *testing.T) {
	a, cfg := newDisabledAgent(t, "cs-cfg-knobs", map[string]any{
		"offline_threshold": "300s",
	})
	if got := a.cs().OfflineThreshold; got != 300*time.Second {
		t.Fatalf("초기 offline_threshold = %v, want 300s", got)
	}

	next := withOptions(cfg, map[string]any{
		"offline_threshold":    "45s",
		"comm_report_interval": "10s",
	})
	if err := a.Configure(next); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	if got := a.cs().OfflineThreshold; got != 45*time.Second {
		t.Errorf("offline_threshold = %v, want 45s", got)
	}
	if got := a.cs().CommReportInterval; got != 10*time.Second {
		t.Errorf("comm_report_interval = %v, want 10s", got)
	}
}

// TestConfigure_MalformedConfigKeepsPreviousCsConfig 는 파싱 실패 시 에러를 반환하고
// 이전 csConfig 를 그대로 보존하는지 검증한다 (절반만 갱신된 상태 금지).
func TestConfigure_MalformedConfigKeepsPreviousCsConfig(t *testing.T) {
	a, cfg := newDisabledAgent(t, "cs-cfg-malformed", map[string]any{
		"emit_comm_state":   true,
		"offline_threshold": "45s",
	})
	before := *a.cs()

	tests := []struct {
		name string
		opts map[string]any
	}{
		{"emit_comm_state 가 불리언이 아님", map[string]any{"emit_comm_state": "yes"}},
		{"offline_threshold duration 파싱 불가", map[string]any{"offline_threshold": "45절"}},
		{"qos 범위 초과", map[string]any{"qos": 7}},
		{"broker 가 문자열이 아님", map[string]any{"broker": 1883}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := a.Configure(withOptions(cfg, tt.opts)); err == nil {
				t.Fatal("잘못된 설정은 에러로 거부되어야 한다")
			}
			after := *a.cs()
			if after.EmitCommState != before.EmitCommState ||
				after.OfflineThreshold != before.OfflineThreshold ||
				after.Broker != before.Broker ||
				after.QoS != before.QoS {
				t.Errorf("파싱 실패 후 csConfig 가 변경되었다: before=%+v after=%+v", before, after)
			}
		})
	}
}

// TestConfigure_LenientParseStillUsedAtConstruction 는 생성 경로의 관용(lenient)
// 파싱 동작이 보존되는지 검증한다 — strict 검증은 런타임 재설정 경로에만 적용된다.
func TestConfigure_LenientParseStillUsedAtConstruction(t *testing.T) {
	a, _ := newDisabledAgent(t, "cs-cfg-lenient", map[string]any{
		"emit_comm_state": "yes", // 타입 불일치 — 생성 시에는 조용히 무시된다(기존 동작).
	})
	if a.cs().EmitCommState {
		t.Error("생성 경로에서 타입 불일치 옵션은 무시되고 기본값(false)이어야 한다")
	}
}

// ---------------------------------------------------------------------------
// FIX-3: Stop → Start 후 done 채널 재생성 (hot-spin 방지)
// ---------------------------------------------------------------------------

// TestStopStartRecreatesDoneChannel 는 Stop→Start 이후 ReceiveMessage 가 빈 recvCh
// 에서 정상적으로 블로킹하는지 검증한다.
//
// done 을 재생성하지 않으면 `case <-a.done` 이 영구히 ready 상태가 되어
// ReceiveMessage 가 즉시 에러를 반환하고, 노드 수신 루프가 블로킹 없이 재시도하며
// CPU 코어 하나를 태운다(hot-spin).
func TestStopStartRecreatesDoneChannel(t *testing.T) {
	a, _ := newDisabledAgent(t, "cs-restart-done", nil)
	ctx := context.Background()

	if err := a.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Stop 직후에는 즉시 에러 반환이 정상 동작이다 (기준선).
	stopCtx, cancelStop := context.WithTimeout(ctx, time.Second)
	defer cancelStop()
	if _, err := a.ReceiveMessage(stopCtx); err == nil {
		t.Fatal("Stop 이후 ReceiveMessage 는 에러여야 한다 (테스트 전제)")
	}

	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	const wait = 150 * time.Millisecond
	recvCtx, cancelRecv := context.WithTimeout(ctx, wait)
	defer cancelRecv()

	start := time.Now()
	_, err := a.ReceiveMessage(recvCtx)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("재시작 후 ReceiveMessage = %v, want context.DeadlineExceeded "+
			"(done 채널이 재생성되지 않아 즉시 반환한다)", err)
	}
	// 즉시 반환 여부가 hot-spin 의 직접 신호이다.
	if elapsed < wait/2 {
		t.Errorf("재시작 후 ReceiveMessage 가 %v 만에 반환했다 — 블로킹하지 않으면 "+
			"노드 수신 루프가 hot-spin 한다", elapsed)
	}
}

// TestStopStartDeliversAfterRestart 는 재시작 후에도 수신 경로가 실제로 동작하는지
// 검증한다 (done 재생성이 수신을 막지 않음).
func TestStopStartDeliversAfterRestart(t *testing.T) {
	a, _ := newDisabledAgent(t, "cs-restart-deliver", nil)
	ctx := context.Background()

	if err := a.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	a.enqueue([]byte(`{"measurement":"temperature"}`), "test")

	recvCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	data, err := a.ReceiveMessage(recvCtx)
	if err != nil {
		t.Fatalf("재시작 후 ReceiveMessage: %v", err)
	}
	if string(data) != `{"measurement":"temperature"}` {
		t.Errorf("수신 데이터 = %q", string(data))
	}
}

// TestStopStartStopIsIdempotent 는 done/doneOnce 재생성 후 두 번째 Stop 이
// 패닉(닫힌 채널 재close) 없이 동작하는지 검증한다.
func TestStopStartStopIsIdempotent(t *testing.T) {
	a, _ := newDisabledAgent(t, "cs-restart-idem", nil)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := a.Stop(ctx); err != nil {
			t.Fatalf("Stop(%d): %v", i, err)
		}
		if err := a.Stop(ctx); err != nil {
			t.Fatalf("Stop 재호출(%d): %v", i, err)
		}
		if err := a.Start(ctx); err != nil {
			t.Fatalf("Start(%d): %v", i, err)
		}
	}

	// 마지막 재시작 후에도 done 은 열려 있어야 한다.
	recvCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if _, err := a.ReceiveMessage(recvCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("반복 Stop/Start 후 ReceiveMessage = %v, want context.DeadlineExceeded", err)
	}
}

// TestStopWithoutStartKeepsDoneClosed 는 Stop 이후(재시작 없이)에는 done 이 닫힌
// 상태로 유지되어 정지된 에이전트가 되살아나지 않는지 검증한다.
func TestStopWithoutStartKeepsDoneClosed(t *testing.T) {
	a, _ := newDisabledAgent(t, "cs-stop-stays-stopped", nil)
	ctx := context.Background()

	if err := a.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	for i := 0; i < 3; i++ {
		recvCtx, cancel := context.WithTimeout(ctx, time.Second)
		_, err := a.ReceiveMessage(recvCtx)
		cancel()
		if err == nil || errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Stop 이후 %d 회차 ReceiveMessage = %v, want stopped 에러", i, err)
		}
	}
	if !a.stopped.Load() {
		t.Error("Stop 이후 stopped 가드가 해제되었다 — 세션 부활 방지가 무력화된다")
	}
}
