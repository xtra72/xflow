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

// TestHvacr01Agent_PartialStateEmitsOnChange 는 5 핵심 필드가 모두 관측되지
// 않았더라도 상태 변화가 발생하면 현재 상태를 그대로 emit 함을 검증한다.
//
// 이전에는 AllCoreObserved() 게이트로 5 핵심 필드(power/mode/target_temp/
// current_temp/fan_speed)가 모두 관측될 때까지 emit 을 보류했으나, 핵심 필드를
// 보내지 않는 디바이스는 상태가 영영 노드로 전송되지 않는 문제가 있어 게이트를
// 제거했다. 이제 변화 시 현재 상태(미관측 필드는 zero value)를 즉시 전송한다.
func TestHvacr01Agent_PartialStateEmitsOnChange(t *testing.T) {
	a, _, _ := newTestAgent(t)
	addr, _ := ParseNasaAddress("200001")

	// Frame 1: power + mode 만 (5 핵심 중 2개) — 부분 상태.
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
	if afterMsg1 < 1 {
		t.Fatalf("Frame 1 (power+mode, 부분 상태): recentSnapshots count=%d, want >=1 (게이트 제거 후 즉시 emit)", afterMsg1)
	}
}
