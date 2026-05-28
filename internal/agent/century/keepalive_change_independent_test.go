package century

import (
	"testing"
	"time"
)

// TestAgent_KeepaliveFiresDespiteFrequentChanges 는 v0.3.10 의 regression 테스트이다.
//
// 사용자 보고: "change 메시지는 도착하고 keepalive 만 안옴".
//
// 원인: v0.3.0~v0.3.9 의 checkKeepaliveEmits 는 lastEmitTime 을 기준으로 했는데,
// change emit 마다 lastEmitTime 이 갱신되므로 change 가 자주 일어나면 keepalive
// 임계값에 절대 도달하지 못함 (예: 현장에서 current_temp 가 매 cycle 0.1°C 진동).
//
// 수정 (v0.3.10): lastKeepaliveTime 을 lastEmitTime 과 분리. change 는 anchor 가
// 비어있을 때만 초기화하고, 이후 change 는 lastKeepaliveTime 에 영향 없음.
// 결과: change 빈도와 무관하게 keepalive 가 interval 마다 fire.
//
// 본 테스트는 짧은 keepalive_interval (300ms) + mode 가 같지만 다른 temp 값으로
// 매 100ms 마다 frame 을 주입해 change emit 을 강제로 일으킨다. 그래도 keepalive
// 가 한 번 이상 fire 해야 한다.
func TestAgent_KeepaliveFiresDespiteFrequentChanges(t *testing.T) {
	t.Parallel()

	// 최초 Reg02 + Reg04 (v0.4.2 gate) + keepalive_interval=300ms.
	initial := append([]byte{}, mustBuildReg02ResponseFrame(t, 0x3B)...)
	initial = append(initial, mustBuildReg04ResponseFrame(t, 0x3B)...)
	a, rt, cleanup := makeTestAgent(t, map[string]any{
		"keepalive_interval": "300ms",
	}, initial)
	defer cleanup()

	// 매 80ms 마다 추가 frame 을 주입한다 (change 를 매번 트리거할 목적).
	// 300ms 안에 ~3~4번 의 change 가 발생하지만, keepalive 가 적어도 한 번
	// 발생해야 한다.
	stopFeed := make(chan struct{})
	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		modeAlt := byte(0x00) // alternate mode to force state changes
		for {
			select {
			case <-stopFeed:
				return
			case <-ticker.C:
				// mode 를 0x00 / 0x01 alternating 으로 보내 매 frame 마다
				// Icp01DeviceStateSnapshot.Equals 가 false 가 되도록 유도.
				rt.deliver(mustBuildReg02ResponseFrameMode(t, 0x3B, modeAlt))
				if modeAlt == 0x00 {
					modeAlt = 0x01
				} else {
					modeAlt = 0x00
				}
			}
		}
	}()
	defer close(stopFeed)

	// 최소 5 emit 까지 기다린다 (초기 change + ~3~4 change + 최소 1 keepalive).
	msgs := waitForMsgCount(t, a, 5, 1500*time.Millisecond)
	if len(msgs) < 2 {
		t.Fatalf("expected multiple emits, got %d: %v", len(msgs), msgs)
	}

	sawChange := 0
	sawKeepalive := 0
	for _, m := range msgs {
		switch m["trigger"] {
		case TriggerChange:
			sawChange++
		case TriggerReport:
			sawKeepalive++
		}
	}

	// change 는 여러 번 발생해야 한다 (alternating mode 가 작동했다는 증거).
	if sawChange < 2 {
		t.Errorf("expected multiple change emits, got %d (msgs=%d)", sawChange, len(msgs))
	}
	// 핵심 invariant: change 가 자주 일어나도 keepalive 가 최소 한 번은 fire.
	if sawKeepalive < 1 {
		t.Errorf("v0.3.10 regression: keepalive did not fire despite frequent changes — got %d change, %d keepalive (total=%d)",
			sawChange, sawKeepalive, len(msgs))
	}

	// 트랜스포트 write 불변식 (AC-B9).
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0", rt.WriteCount())
	}
}
