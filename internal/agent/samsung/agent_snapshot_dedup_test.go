package samsung

import (
	"testing"
)

// TestHvacr01Agent_PushRecentSnapshot_Deduplicates 는 사용자 보고
// "outdoor 디바이스가 매 frame 마다 동일 snapshot 을 emit" 결함의 회귀 테스트이다.
//
// 원인 (수정 전): handleMessage 가 pushRecentSnapshot 을 unconditional 호출
// (line 1565). LastSeen 만 갱신되는 heartbeat frame 도 매번 push.
//
// 수정: snapshotShouldPush 플래그를 추적해 wasOffline 또는 stateChanged 일
// 때만 push. heartbeat 만 들어오는 frame 은 skip.
//
// 본 테스트는 known device 에 동일한 frame 을 3 번 보내 첫 1 회만 push 되고
// 이후 2 회는 skip 되는지 검증한다.
func TestHvacr01Agent_PushRecentSnapshot_Deduplicates(t *testing.T) {
	a, _, _ := newTestAgent(t)
	addr, _ := ParseNasaAddress("200001")

	// 초기 frame 으로 device 가 online 으로 전이 (snapshotShouldPush=true 경로).
	msg := &NasaMessage{
		SourceAddr:  addr,
		DestAddr:    AddrController,
		CommandCode: CmdNormalRequest,
		MessageSets: []NasaMessageSet{
			{Index: MsgPower, Value: []byte{0x01}},
			{Index: MsgMode, Value: []byte{0x01}},
			{Index: MsgFanSpeed, Value: []byte{0x02}},
			{Index: MsgTargetTemp, Value: []byte{0x00, 0xFA}},  // 25.0
			{Index: MsgCurrentTemp, Value: []byte{0x00, 0xF0}}, // 24.0
		},
	}

	// 디바이스를 offline 으로 미리 두면 첫 frame 이 online 전이를 트리거 → push 1회.
	a.mu.Lock()
	a.devices[addr].Online = false
	a.mu.Unlock()
	a.handleMessage(msg)

	a.recentMu.Lock()
	afterFirst := len(a.recentSnapshots)
	a.recentMu.Unlock()
	if afterFirst < 1 {
		t.Fatalf("first frame did not push to recentSnapshots: count=%d", afterFirst)
	}

	// 동일한 frame 2회 추가 — heartbeat 만 갱신되고 state/online 은 변화 없음.
	// snapshotShouldPush 가 false 이므로 push 되지 않아야 한다.
	a.handleMessage(msg)
	a.handleMessage(msg)

	a.recentMu.Lock()
	afterDuplicate := len(a.recentSnapshots)
	a.recentMu.Unlock()

	if afterDuplicate != afterFirst {
		t.Errorf("duplicate frames pushed extra snapshots: before=%d after=%d (want equal — heartbeat-only frames must NOT push)",
			afterFirst, afterDuplicate)
	}

	// state 변경 frame 은 다시 push 되어야 한다.
	msgChanged := &NasaMessage{
		SourceAddr:  addr,
		DestAddr:    AddrController,
		CommandCode: CmdNormalRequest,
		MessageSets: []NasaMessageSet{
			{Index: MsgTargetTemp, Value: []byte{0x01, 0x04}}, // 26.0 변경
		},
	}
	a.handleMessage(msgChanged)

	a.recentMu.Lock()
	afterChange := len(a.recentSnapshots)
	a.recentMu.Unlock()
	if afterChange <= afterDuplicate {
		t.Errorf("state-change frame did not push: before=%d after=%d (want >before)",
			afterDuplicate, afterChange)
	}
}

// TestHvacr01Agent_PushRecentSnapshot_OutdoorOnlyOnTransitions 는 outdoor 디바이스
// 처럼 dev.State 가 nil 인 경우의 dedup 동작을 검증한다.
//
// 사용자 보고 실제 시나리오: outdoor (address 10.00.00) 가 매 frame 마다 동일
// snapshot 출력. dev.State == nil 이므로 state-change 검사는 항상 skip 되고,
// online 전이 후에는 더 이상 push 되어서는 안 된다.
func TestHvacr01Agent_PushRecentSnapshot_OutdoorOnlyOnTransitions(t *testing.T) {
	a, _, _ := newTestAgent(t)

	// auto-discovery 로 outdoor 가 처음 등록되는 시나리오.
	a.mu.Lock()
	a.hvacr01Config.AutoDiscovery = true
	a.mu.Unlock()

	// outdoor address: 10.xx.xx (DetectDeviceType 기준)
	outdoorAddr, _ := ParseNasaAddress("100000")

	msg := &NasaMessage{
		SourceAddr:  outdoorAddr,
		DestAddr:    AddrController,
		CommandCode: CmdNormalRequest,
		MessageSets: []NasaMessageSet{},
	}

	// 첫 frame: device_discovered → snapshotShouldPush=true (wasOffline 가드 통해)
	a.handleMessage(msg)
	a.recentMu.Lock()
	firstCount := len(a.recentSnapshots)
	a.recentMu.Unlock()

	// 후속 frames 10 회 — state nil 이고 online 변화 없음 → push 되어서는 안 됨.
	for i := 0; i < 10; i++ {
		a.handleMessage(msg)
	}

	a.recentMu.Lock()
	finalCount := len(a.recentSnapshots)
	a.recentMu.Unlock()

	// outdoor 의 첫 frame 은 신규 device 자동 등록 시 wasOffline=true 로
	// push 1회. 이후 frames 는 모두 skip 되어야 한다.
	if finalCount > firstCount {
		t.Errorf("outdoor heartbeat frames leaked into recentSnapshots: first=%d final=%d (want equal — heartbeat-only must NOT push)",
			firstCount, finalCount)
	}
}

// TestHvacr01Agent_IndoorEmitGatedByAllCoreObserved 는 사용자 보고
// "초기값 0, fan_speed:\"\" 등 미수신 필드의 zero value 가 점진적으로 채워지면서
// 매 단계 emit 되는 결함" 의 회귀 테스트이다.
//
// 수정: NasaDeviceState.observedCore bitmask 가 5 핵심 필드 (power/mode/
// target_temp/current_temp/fan_speed) 의 관측 여부를 추적. handleMessage 가
// `!dev.State.AllCoreObserved()` 시 early return 으로 emit 보류.
//
// 시나리오 (사용자 실제 출력 재현):
//
//	Frame 1: power+mode → 부분 상태 (fan_speed:"", current_temp:0, target_temp:0)
//	Frame 2: fan_speed+filter_alarm 추가 → 여전히 current_temp/target_temp 미설정
//	Frame 3: current_temp+target_temp 추가 → 5 핵심 모두 관측됨
//
// 수정 전: 3 회 모두 emit (3 개의 부분 상태)
// 수정 후: Frame 3 에서만 첫 emit (완전한 상태)
func TestHvacr01Agent_IndoorEmitGatedByAllCoreObserved(t *testing.T) {
	a, _, _ := newTestAgent(t)
	addr, _ := ParseNasaAddress("200001")

	// 디바이스를 offline 상태로 두지 않아 device_online 이벤트와 분리한다.
	// (online transition 은 wasOffline 경로에서 별도 처리됨)

	// Frame 1: power + mode 만 (5 핵심 중 2개)
	msg1 := &NasaMessage{
		SourceAddr:  addr,
		DestAddr:    AddrController,
		CommandCode: CmdNormalRequest,
		MessageSets: []NasaMessageSet{
			{Index: MsgPower, Value: []byte{0x01}},
			{Index: MsgMode, Value: []byte{0x01}}, // cool
		},
	}
	a.handleMessage(msg1)

	a.recentMu.Lock()
	afterMsg1 := len(a.recentSnapshots)
	a.recentMu.Unlock()
	if afterMsg1 > 0 {
		t.Errorf("Frame 1 (power+mode only): recentSnapshots count=%d, want 0 (5 핵심 미완)", afterMsg1)
	}

	// Frame 2: fan_speed 추가 (3개)
	msg2 := &NasaMessage{
		SourceAddr:  addr,
		DestAddr:    AddrController,
		CommandCode: CmdNormalRequest,
		MessageSets: []NasaMessageSet{
			{Index: MsgFanSpeed, Value: []byte{0x02}}, // medium
		},
	}
	a.handleMessage(msg2)

	a.recentMu.Lock()
	afterMsg2 := len(a.recentSnapshots)
	a.recentMu.Unlock()
	if afterMsg2 > 0 {
		t.Errorf("Frame 2 (+ fan_speed): recentSnapshots count=%d, want 0 (current/target_temp 미수신)", afterMsg2)
	}

	// Frame 3: current_temp + target_temp 추가 → 5 핵심 모두 완료
	msg3 := &NasaMessage{
		SourceAddr:  addr,
		DestAddr:    AddrController,
		CommandCode: CmdNormalRequest,
		MessageSets: []NasaMessageSet{
			{Index: MsgTargetTemp, Value: []byte{0x00, 0xFA}},  // 25.0
			{Index: MsgCurrentTemp, Value: []byte{0x00, 0xF0}}, // 24.0
		},
	}
	a.handleMessage(msg3)

	a.recentMu.Lock()
	afterMsg3 := len(a.recentSnapshots)
	a.recentMu.Unlock()
	if afterMsg3 < 1 {
		t.Fatalf("Frame 3 (5 핵심 완료): recentSnapshots count=%d, want >=1", afterMsg3)
	}

	// 상태 검증: dev.State 의 observedCore 가 모두 set 되어 AllCoreObserved=true.
	a.mu.RLock()
	dev := a.devices[addr]
	a.mu.RUnlock()
	if !dev.State.AllCoreObserved() {
		t.Errorf("AllCoreObserved() = false, want true (5 핵심 모두 처리 후)")
	}
}
