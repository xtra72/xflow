package lg

import (
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// TestLGCNPAgent_ShouldEmitODU_Deduplicates 는 ODU frame dedup helper 의
// 동작을 검증한다. 동일한 LGCNPODUParsed state 가 들어오면 false (skip).
func TestLGCNPAgent_ShouldEmitODU_Deduplicates(t *testing.T) {
	a := &LGCNPAgent{
		dedupMu: sync.Mutex{},
	}

	outdoor := 25.5
	suction := 10.0
	state1 := &LGCNPODUParsed{
		OutdoorTemp:     &outdoor,
		CompSuctionTemp: &suction,
	}

	// 첫 호출 → 새 state 이므로 true.
	if emit, _ := a.shouldEmitODU(state1); !emit {
		t.Fatalf("첫 호출: shouldEmitODU=false, want true")
	}

	// 동일 state 재호출 → false.
	if emit, reason := a.shouldEmitODU(state1); emit {
		t.Errorf("동일 state 반복: shouldEmitODU=true, want false (dedup)")
	} else if reason != "identical" {
		t.Errorf("reason=%q, want \"identical\"", reason)
	}

	// 값이 다른 state → true.
	outdoor2 := 26.0
	state2 := &LGCNPODUParsed{
		OutdoorTemp:     &outdoor2,
		CompSuctionTemp: &suction,
	}
	if emit, _ := a.shouldEmitODU(state2); !emit {
		t.Errorf("변경된 state: shouldEmitODU=false, want true")
	}

	// 변경 후 다시 동일 → false.
	if emit, _ := a.shouldEmitODU(state2); emit {
		t.Errorf("재변경 후 동일: shouldEmitODU=true, want false")
	}

	// nil state → false (skip).
	if emit, reason := a.shouldEmitODU(nil); emit {
		t.Errorf("nil state: shouldEmitODU=true, want false")
	} else if reason != "nil_state" {
		t.Errorf("reason=%q, want \"nil_state\"", reason)
	}
}

// TestLGCNPAgent_ShouldEmitIDU_IndependentPerIDU 는 다중 IDU 환경에서 각
// IDU 의 dedup 캐시가 독립적으로 관리되는지 검증한다.
func TestLGCNPAgent_ShouldEmitIDU_IndependentPerIDU(t *testing.T) {
	a := &LGCNPAgent{
		dedupMu:     sync.Mutex{},
		lastIDUEmit: make(map[int][]byte),
	}

	state1 := &LGCNPIDUParsed{
		Power:       true,
		TargetTemp:  25.0,
		CurrentTemp: 24.0,
		Mode:        3, // hvac.ModeDry
		FanSpeed:    3, // hvac.FanLow
	}
	state2 := &LGCNPIDUParsed{
		Power:       false,
		TargetTemp:  20.0,
		CurrentTemp: 21.0,
		Mode:        1, // hvac.ModeCool
		FanSpeed:    2, // hvac.FanQuiet
	}

	// IDU#1 첫 emit → true
	if emit, _ := a.shouldEmitIDU(1, state1); !emit {
		t.Fatalf("IDU#1 first: want true")
	}
	// IDU#1 동일 state 반복 → false
	if emit, reason := a.shouldEmitIDU(1, state1); emit {
		t.Errorf("IDU#1 duplicate: want false")
	} else if reason != "identical" {
		t.Errorf("reason=%q, want \"identical\"", reason)
	}
	// IDU#2 (다른 IDU) 첫 emit → true (IDU#1 의 cache 와 독립)
	if emit, _ := a.shouldEmitIDU(2, state2); !emit {
		t.Errorf("IDU#2 first: want true (independent cache)")
	}
	// IDU#2 동일 state 반복 → false
	if emit, _ := a.shouldEmitIDU(2, state2); emit {
		t.Errorf("IDU#2 duplicate: want false")
	}
	// IDU#1 변경 후 → true
	state1Changed := &LGCNPIDUParsed{
		Power:       true,
		TargetTemp:  26.0, // 변경
		CurrentTemp: 24.0,
		Mode:        3, // hvac.ModeDry
		FanSpeed:    3, // hvac.FanLow
	}
	if emit, _ := a.shouldEmitIDU(1, state1Changed); !emit {
		t.Errorf("IDU#1 changed: want true")
	}
	// IDU#2 는 영향 없음 — 동일 state 면 여전히 false
	if emit, _ := a.shouldEmitIDU(2, state2); emit {
		t.Errorf("IDU#2 unchanged after IDU#1 change: want false")
	}
}

// TestParseLGCNPConfig_IncludeRawHexAndDedupe 는 v0.x 신규 옵션 파싱을
// 검증한다.
func TestParseLGCNPConfig_IncludeRawHexAndDedupe(t *testing.T) {
	t.Run("defaults: include_raw_hex=false, dedupe_frames=true", func(t *testing.T) {
		cfg, err := parseLGCNPConfig(map[string]any{
			"serial_port": "/dev/ttyTEST",
		})
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if cfg.IncludeRawHex {
			t.Errorf("default IncludeRawHex = true, want false")
		}
		if !cfg.DedupeFrames {
			t.Errorf("default DedupeFrames = false, want true")
		}
	})
	t.Run("include_raw_hex=true opts in", func(t *testing.T) {
		cfg, err := parseLGCNPConfig(map[string]any{
			"serial_port":     "/dev/ttyTEST",
			"include_raw_hex": true,
		})
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if !cfg.IncludeRawHex {
			t.Errorf("IncludeRawHex = false, want true")
		}
	})
	t.Run("dedupe_frames=false disables dedup", func(t *testing.T) {
		cfg, err := parseLGCNPConfig(map[string]any{
			"serial_port":   "/dev/ttyTEST",
			"dedupe_frames": false,
		})
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if cfg.DedupeFrames {
			t.Errorf("DedupeFrames = true, want false (explicit opt-out)")
		}
	})
}

// newMinimalLGCNPAgentForTest 는 handleODUFrame 의 emit gate 회귀 검증용으로
// 트랜스포트 / lifecycle 없이 메시지 채널만 갖춘 최소 agent 를 만든다.
func newMinimalLGCNPAgentForTest() *LGCNPAgent {
	a := &LGCNPAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("lgcnp-test")),
		lgcnpConfig:   LGCNPConfig{DedupeFrames: true, IncludeRawHex: false},
		msgCh:         make(chan []byte, 16),
		stats:         agent.NewAgentStats(),
		logger:        slog.Default(),
		recentFrames:  make([]lgcnpFrameRecord, lgcnpRecentBufferSize),
		recentNotify:  make(chan struct{}, 1),
		iduDevices:    make(map[string]*LGCNPDevice),
		oduState:      &LGCNPODUState{},
		lastStates:    make(map[string]LGCNPDeviceState),
		lastIDUEmit:   make(map[int][]byte),
	}
	a.bridgeActive.Store(true)
	return a
}

// TestLGCNPAgent_HandleODUFrame_SkipsEmptyState 는 사용자 보고
// "{"checksum_valid":true,"odu_seq":3,"seq":53,"timestamp_ms":...,
//
//	"type":"lgcnp_odu_frame"} 같은 의미 없는 메시지" 의 회귀 테스트이다.
//
// SEQ=0x01/0x03/0x05 등 미파싱 ODU frame 은 evt.State 가 nil 이므로 emit/push
// 모두 skip 되어야 한다. SEQ=0x02 (실시간 cycle) 만 의미 있는 state 를 갖는다.
func TestLGCNPAgent_HandleODUFrame_SkipsEmptyState(t *testing.T) {
	a := newMinimalLGCNPAgentForTest()

	// SEQ=0x01 raw 로부터 frame 생성 (state 가 채워지지 않는 SEQ).
	var raw [20]byte
	copy(raw[:], lgcnpTestODU_SEQ01)
	f := &LGCNPODUFrame{
		Raw:           raw,
		SEQ:           0x01,
		Timestamp:     time.Unix(1779180894, 0),
		ChecksumValid: true,
	}
	a.handleODUFrame(f)

	// msgCh 는 비어있어야 한다 (의미 없는 메시지 emit 차단).
	select {
	case data := <-a.msgCh:
		t.Errorf("SEQ=0x01 frame leaked to msgCh: %s", data)
	default:
		// expected — skip empty state.
	}

	// recentFrames 도 채워지지 않아야 한다.
	a.recentMu.RLock()
	hasRecent := a.recentIdx != 0 || a.recentFull
	a.recentMu.RUnlock()
	if hasRecent {
		t.Errorf("SEQ=0x01 frame pushed to recentFrames (want skipped)")
	}

	// 통계 카운터는 정상 증가 — frame 자체는 수신되었으므로.
	if got := a.oduFramesCaptured.Load(); got != 1 {
		t.Errorf("oduFramesCaptured = %d, want 1 (frame counted before skip)", got)
	}

	// SEQ=0x02 frame 은 emit 되어야 한다 (state 가 채워짐).
	var raw2 [20]byte
	copy(raw2[:], lgcnpTestODU_SEQ02)
	f2 := &LGCNPODUFrame{
		Raw:           raw2,
		SEQ:           0x02,
		Timestamp:     time.Unix(1779180894, 0),
		ChecksumValid: true,
	}
	a.handleODUFrame(f2)

	select {
	case data := <-a.msgCh:
		// state 필드가 포함된 의미 있는 메시지여야 한다.
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if _, ok := m["state"]; !ok {
			t.Errorf("SEQ=0x02 emit missing 'state' field: %s", data)
		}
		// raw_hex 는 IncludeRawHex=false 이므로 없어야 한다.
		if _, ok := m["raw_hex"]; ok {
			t.Errorf("raw_hex leaked despite IncludeRawHex=false: %s", data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("SEQ=0x02 frame did NOT emit (gate too aggressive)")
	}

	// SEQ=0x04 frame 도 evt.State 가 nil 이므로 skip 되어야 한다
	// (handleODUFrame 이 SEQ=0x04 에서는 oduState 만 갱신, evt.State 미설정).
	if len(lgcnpTestODU_SEQ04) >= 20 {
		var raw4 [20]byte
		copy(raw4[:], lgcnpTestODU_SEQ04)
		f4 := &LGCNPODUFrame{
			Raw:           raw4,
			SEQ:           0x04,
			Timestamp:     time.Unix(1779180894, 0),
			ChecksumValid: true,
		}
		a.handleODUFrame(f4)
		select {
		case data := <-a.msgCh:
			t.Errorf("SEQ=0x04 frame leaked to msgCh (evt.State nil): %s", data)
		case <-time.After(50 * time.Millisecond):
			// expected — skip empty state.
		}
	}
}
