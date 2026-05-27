package century

import (
	"testing"
	"time"
)

// TestAgent_StatsCounters_UpdateOnFrameCapture 는 사용자 보고
// "Century-HVAC 통계정보 업데이트 안됨" 의 회귀 테스트이다.
//
// 원인: captureLoop 에서 cStats (granular counters) 만 증가시키고 a.stats
// (Web UI 가 읽는 표준 agent.AgentStats) 는 갱신하지 않아 messages_in /
// bytes_read / last_activity 가 영구 0 으로 표시됨.
//
// 수정: captureLoop 의 frame 수신 직후 a.stats.IncrExternalMessagesReceived,
// AddBytesRead, UpdateLastActivity 호출 — NASA / LG ICP-01 과 동일 패턴.
//
// 본 테스트는 Reg02 + Reg04 frame 주입 후 표준 stats 가 0 이 아님을 검증한다.
func TestAgent_StatsCounters_UpdateOnFrameCapture(t *testing.T) {
	t.Parallel()
	batch := append([]byte{}, mustBuildReg02ResponseFrame(t, 0x3B)...)
	batch = append(batch, mustBuildReg04ResponseFrame(t, 0x3B)...)
	a, _, cleanup := makeTestAgent(t, nil, batch)
	defer cleanup()

	// 2 frame 모두 captureLoop 가 처리할 시간 확보.
	waitUntil(t, 1*time.Second, func() bool {
		return a.cStats.framesCaptured.Load() >= 2
	}, "2 frames not captured")

	// granular cStats 는 이미 증가 — sanity 확인.
	if got := a.cStats.framesCaptured.Load(); got < 2 {
		t.Errorf("cStats.framesCaptured = %d, want >=2", got)
	}

	// 표준 agent.AgentStats 도 증가해야 한다 (사용자 보고 fix 의 핵심).
	snap := a.stats.Snapshot()
	if snap.MessagesReceived < 2 {
		t.Errorf("stats.MessagesReceived = %d, want >=2 (Web UI 통계 카운터)", snap.MessagesReceived)
	}
	if snap.BytesRead == 0 {
		t.Errorf("stats.BytesRead = 0, want >0 (Web UI 바이트 카운터)")
	}
	if snap.LastActivityAt.IsZero() {
		t.Errorf("stats.LastActivityAt zero, want recent timestamp")
	}
}
