package samsung

import (
	"testing"
)

// TestNASAAgent_PushRecentSnapshot_Deduplicates 는 사용자 보고
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
func TestNASAAgent_PushRecentSnapshot_Deduplicates(t *testing.T) {
	a, _, _ := newTestAgent(t)
	addr, _ := ParseNASAAddress("200001")

	// 초기 frame 으로 device 가 online 으로 전이 (snapshotShouldPush=true 경로).
	msg := &NASAMessage{
		SourceAddr:  addr,
		DestAddr:    AddrController,
		CommandCode: CmdNormalRequest,
		MessageSets: []NASAMessageSet{
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
	msgChanged := &NASAMessage{
		SourceAddr:  addr,
		DestAddr:    AddrController,
		CommandCode: CmdNormalRequest,
		MessageSets: []NASAMessageSet{
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

// TestNASAAgent_PushRecentSnapshot_OutdoorOnlyOnTransitions 는 outdoor 디바이스
// 처럼 dev.State 가 nil 인 경우의 dedup 동작을 검증한다.
//
// 사용자 보고 실제 시나리오: outdoor (address 10.00.00) 가 매 frame 마다 동일
// snapshot 출력. dev.State == nil 이므로 state-change 검사는 항상 skip 되고,
// online 전이 후에는 더 이상 push 되어서는 안 된다.
func TestNASAAgent_PushRecentSnapshot_OutdoorOnlyOnTransitions(t *testing.T) {
	a, _, _ := newTestAgent(t)

	// auto-discovery 로 outdoor 가 처음 등록되는 시나리오.
	a.mu.Lock()
	a.nasaConfig.AutoDiscovery = true
	a.mu.Unlock()

	// outdoor address: 10.xx.xx (DetectDeviceType 기준)
	outdoorAddr, _ := ParseNASAAddress("100000")

	msg := &NASAMessage{
		SourceAddr:  outdoorAddr,
		DestAddr:    AddrController,
		CommandCode: CmdNormalRequest,
		MessageSets: []NASAMessageSet{},
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
