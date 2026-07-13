package samsung

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// SPEC-HVACR-STALE-OFFLINE: Samsung LastSeen 기반 stale offline 감지 테스트
//
// 버그: Samsung 폴링은 비동기(요청/응답 미매칭)이므로, 트랜스포트가 정상인 한
// Send 가 성공하여 ErrorCount 가 0으로 유지되고 dev.Online 이 영원히 true 로 남는다.
// 실내기 정전 등으로 응답이 끊겨도 offline 전이가 발생하지 않는다.
//
// device_connection 별도 스트림 제거 후: stale 전이는 device_state change(state.online=false)
// 로 방출되며, 복구는 device_state change(state.online=true) 로 방출된다. 아래 단언은
// drainStateMsgs/findStateMsg (probe_test.go) 로 device_state 를 소비한다.
// ---------------------------------------------------------------------------

// TestStale_ReproBug_NoOfflineChangeWhenSilent 는 버그 재현 테스트이다.
// online 디바이스가 staleness 임계값보다 오래 프레임을 받지 못하면 offline change 가
// 방출되어야 하지만, 버그 코드에서는 방출되지 않는다.
//
// 실제 pollLoop 를 구동한다. transport 는 정상(available)이고 status query Send 도
// 성공하므로, 버그 코드 경로에서는 ErrorCount 가 증가하지 않아 offline 전이가 없다.
func TestStale_ReproBug_NoOfflineChangeWhenSilent(t *testing.T) {
	a, _ := newConnAgent(t, "200001")
	addr, _ := ParseNasaAddress("200001")

	// 1) online baseline 확정 (device_state change online=true 방출 후 소비)
	a.handleMessage(onlineFrame(addr))
	_ = drainStateMsgs(t, a)

	// 2) 짧은 PollInterval + LastSeen 백데이팅으로 즉시 stale 상태를 만든다.
	//    StatusQueryEnabled=true (능동 폴링) + transport available → Send 성공 →
	//    버그 코드에서는 ErrorCount 가 0으로 유지되어 offline 이 되지 않는다.
	a.mu.Lock()
	a.hvacr01Config.PollInterval = 10 * time.Millisecond
	a.hvacr01Config.StatusQueryEnabled = true
	a.devices[addr].LastSeen = time.Now().Add(-1 * time.Hour)
	a.mu.Unlock()

	// 3) 실제 pollLoop 를 구동하여 여러 tick 을 경과시킨다.
	a.wg.Add(1)
	go func() { defer a.wg.Done(); a.pollLoop() }()

	time.Sleep(150 * time.Millisecond)
	close(a.stopCh)
	a.wg.Wait()

	// 4) stale 디바이스는 offline change(state.online=false) 를 방출해야 한다.
	msgs := drainStateMsgs(t, a)
	offlineChange := findStateMsg(msgs, "change", false)
	require.NotNil(t, offlineChange,
		"stale 디바이스는 offline device_state change 를 방출해야 한다 (버그: 미방출)")
}

// TestStale_DirectCheck_OfflineChangeEmitted 는 checkStaleDevices 를 직접 호출하여
// (tick 타이밍 비의존) stale 전이 시 offline change 가 방출되는지 확정 검증한다.
func TestStale_DirectCheck_OfflineChangeEmitted(t *testing.T) {
	a, _ := newConnAgent(t, "200001")
	addr, _ := ParseNasaAddress("200001")

	a.handleMessage(onlineFrame(addr))
	_ = drainStateMsgs(t, a)

	// LastSeen 을 임계값(3 × 30s = 90s) 초과로 백데이팅.
	a.mu.Lock()
	a.devices[addr].LastSeen = time.Now().Add(-10 * time.Minute)
	a.mu.Unlock()

	a.checkStaleDevices()

	msgs := drainStateMsgs(t, a)
	require.Len(t, msgs, 1)
	require.Equal(t, "change", msgs[0]["trigger"])
	online, ok := stateOnline(msgs[0])
	require.True(t, ok)
	require.Equal(t, false, online)

	// 이미 offline 이므로 재호출 시 중복 방출 없어야 한다.
	a.checkStaleDevices()
	require.Empty(t, drainStateMsgs(t, a), "이미 offline 인 device 는 재방출하지 않는다")
}

// TestStale_PassiveMode_DetectionWorks 는 status_query_enabled=false(passive sniff)
// 모드에서도 pollLoop ticker 기반 stale 검사가 동작하는지 검증한다.
func TestStale_PassiveMode_DetectionWorks(t *testing.T) {
	a, _ := newConnAgent(t, "200001")
	addr, _ := ParseNasaAddress("200001")

	a.handleMessage(onlineFrame(addr))
	_ = drainStateMsgs(t, a)

	a.mu.Lock()
	a.hvacr01Config.PollInterval = 10 * time.Millisecond
	a.hvacr01Config.StatusQueryEnabled = false // passive: 능동 쿼리 송신 안 함
	a.devices[addr].LastSeen = time.Now().Add(-1 * time.Hour)
	a.mu.Unlock()

	a.wg.Add(1)
	go func() { defer a.wg.Done(); a.pollLoop() }()

	time.Sleep(150 * time.Millisecond)
	close(a.stopCh)
	a.wg.Wait()

	msgs := drainStateMsgs(t, a)
	require.NotNil(t, findStateMsg(msgs, "change", false),
		"passive 모드에서도 stale offline 이 감지되어야 한다")
}

// TestStale_RecoveryAfterStaleOffline_OnlineChange 는 stale-offline 이후 첫 프레임이
// online change(state.online=true) 를 방출하는지 검증한다.
func TestStale_RecoveryAfterStaleOffline_OnlineChange(t *testing.T) {
	a, _ := newConnAgent(t, "200001")
	addr, _ := ParseNasaAddress("200001")

	// online baseline → stale offline
	a.handleMessage(onlineFrame(addr))
	_ = drainStateMsgs(t, a)
	a.mu.Lock()
	a.devices[addr].LastSeen = time.Now().Add(-10 * time.Minute)
	a.mu.Unlock()
	a.checkStaleDevices()
	off := drainStateMsgs(t, a)
	require.Len(t, off, 1)
	offOnline, ok := stateOnline(off[0])
	require.True(t, ok)
	require.Equal(t, false, offOnline)

	// 복구: 첫 프레임 → online change
	a.handleMessage(onlineFrame(addr))
	rec := drainStateMsgs(t, a)
	require.NotEmpty(t, rec)
	recMsg := findStateMsg(rec, "change", true)
	require.NotNil(t, recMsg, "복구는 device_state change(online=true) 를 방출해야 한다")

	// ErrorCount 는 handleMessage 가 0 으로 리셋
	a.mu.RLock()
	ec := a.devices[addr].ErrorCount
	online := a.devices[addr].Online
	a.mu.RUnlock()
	require.Equal(t, 0, ec)
	require.True(t, online)
}

// TestStale_NeverSeen_NoTransition 는 한 번도 수신되지 않은(LastSeen zero) online device
// 는 stale 판정하지 않는지 검증한다 (never-seen 규칙).
func TestStale_NeverSeen_NoTransition(t *testing.T) {
	a, _ := newConnAgent(t, "200001")
	addr, _ := ParseNasaAddress("200001")

	// LastSeen zero + Online=true (방어적 조합).
	a.mu.Lock()
	a.devices[addr].Online = true
	a.devices[addr].LastSeen = time.Time{} // zero
	a.mu.Unlock()

	a.checkStaleDevices()

	require.Empty(t, drainStateMsgs(t, a), "LastSeen zero device 는 stale 판정 대상이 아니다")
	a.mu.RLock()
	online := a.devices[addr].Online
	a.mu.RUnlock()
	require.True(t, online, "never-seen device 는 stale 로 offline 전환되지 않는다")
}

// TestStale_SetAllDevicesOffline_IndividualChanges 는 트랜스포트 끊김 시 모든 online device 가
// 개별 offline device_state change 를 방출하는지 (배칭 금지) 검증한다.
func TestStale_SetAllDevicesOffline_IndividualChanges(t *testing.T) {
	a, _ := newConnAgent(t, "200001", "200002", "200003")
	addr1, _ := ParseNasaAddress("200001")
	addr2, _ := ParseNasaAddress("200002")
	addr3, _ := ParseNasaAddress("200003")

	// 세 device 모두 online 확정.
	a.handleMessage(onlineFrame(addr1))
	a.handleMessage(onlineFrame(addr2))
	a.handleMessage(onlineFrame(addr3))
	_ = drainStateMsgs(t, a)

	a.setAllDevicesOffline()

	msgs := drainStateMsgs(t, a)
	require.Len(t, msgs, 3, "device 당 개별 change (배칭 금지)")
	for _, m := range msgs {
		require.Equal(t, "change", m["trigger"])
		online, ok := stateOnline(m)
		require.True(t, ok)
		require.Equal(t, false, online)
		require.NotContains(t, m, "devices", "배열 배칭 금지")
		// SPEC-DEVICE-IDENTITY-001: unit_id 는 dotted address format
		require.Contains(t, m, "unit_id")
	}
}

// ---------------------------------------------------------------------------
// offline_timeout 3-way 의미 테스트 (unset → 파생 / 양수 → verbatim / 0 → 비활성)
// ---------------------------------------------------------------------------

// TestStale_Threshold_Unset_DerivesFromPollInterval 는 offline_timeout 미설정(-1) 시
// 임계값이 OfflineThreshold × PollInterval 임을 검증한다.
func TestStale_Threshold_Unset_DerivesFromPollInterval(t *testing.T) {
	a, _ := newConnAgent(t, "200001")
	a.mu.Lock()
	a.hvacr01Config.OfflineTimeout = -1 // 미설정
	a.hvacr01Config.OfflineThreshold = 3
	a.hvacr01Config.PollInterval = 30 * time.Second
	got := a.staleOfflineThreshold()
	a.mu.Unlock()
	require.Equal(t, 90*time.Second, got, "미설정 → 3 × 30s 파생")
}

// TestStale_Threshold_ExplicitPositive_WinsOverDerived 는 offline_timeout="5s" 가
// 파생값(90s)을 제치고 verbatim 5s 로 사용되며, 5s<t<90s 동안 침묵한 device 가
// offline 으로 전환되는지 검증한다.
func TestStale_Threshold_ExplicitPositive_WinsOverDerived(t *testing.T) {
	a, _ := newConnAgent(t, "200001")
	addr, _ := ParseNasaAddress("200001")

	a.handleMessage(onlineFrame(addr))
	_ = drainStateMsgs(t, a)

	a.mu.Lock()
	a.hvacr01Config.OfflineTimeout = 5 * time.Second // 명시적 양수
	a.hvacr01Config.PollInterval = 30 * time.Second  // 파생값은 90s 이지만 무시되어야 함
	require.Equal(t, 5*time.Second, a.staleOfflineThreshold(), "명시적 양수가 파생값을 이긴다")
	// 5s 초과 90s 미만으로 침묵 → 파생(90s)이었다면 online 유지, verbatim(5s)이면 offline.
	a.devices[addr].LastSeen = time.Now().Add(-10 * time.Second)
	a.mu.Unlock()

	a.checkStaleDevices()

	msgs := drainStateMsgs(t, a)
	require.NotNil(t, findStateMsg(msgs, "change", false),
		"명시 5s 임계값 초과(10s 침묵) → offline 전환 (파생 90s 였다면 미전환)")
}

// TestStale_Threshold_Zero_Disabled_NoTransition 는 offline_timeout="0s" 시 staleness
// 감지가 완전히 비활성화되어 오래 침묵한 device 도 전환/방출되지 않음을 검증한다.
// 단, transport-disconnect bulk offline(setAllDevicesOffline)은 여전히 동작해야 한다.
func TestStale_Threshold_Zero_Disabled_NoTransition(t *testing.T) {
	a, _ := newConnAgent(t, "200001")
	addr, _ := ParseNasaAddress("200001")

	a.handleMessage(onlineFrame(addr))
	_ = drainStateMsgs(t, a)

	a.mu.Lock()
	a.hvacr01Config.OfflineTimeout = 0                         // 명시적 0 → 비활성
	a.devices[addr].LastSeen = time.Now().Add(-24 * time.Hour) // 아주 오래 침묵
	a.mu.Unlock()

	a.checkStaleDevices()

	require.Empty(t, drainStateMsgs(t, a), "offline_timeout=0 이면 stale 전환/방출 없음")
	a.mu.RLock()
	online := a.devices[addr].Online
	a.mu.RUnlock()
	require.True(t, online, "비활성 상태에서는 오래 침묵해도 online 유지")

	// 비활성 범위는 staleness 로 한정 — transport-disconnect bulk offline 은 여전히 동작.
	a.setAllDevicesOffline()
	msgs := drainStateMsgs(t, a)
	require.NotNil(t, findStateMsg(msgs, "change", false),
		"offline_timeout=0 이어도 transport-disconnect bulk offline 은 방출되어야 한다")
}
