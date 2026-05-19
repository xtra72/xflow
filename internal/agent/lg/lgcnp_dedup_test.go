package lg

import (
	"sync"
	"testing"
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
	if !a.shouldEmitODU(state1) {
		t.Fatalf("첫 호출: shouldEmitODU=false, want true")
	}

	// 동일 state 재호출 → false.
	if a.shouldEmitODU(state1) {
		t.Errorf("동일 state 반복: shouldEmitODU=true, want false (dedup)")
	}

	// 값이 다른 state → true.
	outdoor2 := 26.0
	state2 := &LGCNPODUParsed{
		OutdoorTemp:     &outdoor2,
		CompSuctionTemp: &suction,
	}
	if !a.shouldEmitODU(state2) {
		t.Errorf("변경된 state: shouldEmitODU=false, want true")
	}

	// 변경 후 다시 동일 → false.
	if a.shouldEmitODU(state2) {
		t.Errorf("재변경 후 동일: shouldEmitODU=true, want false")
	}

	// nil state → false (skip).
	if a.shouldEmitODU(nil) {
		t.Errorf("nil state: shouldEmitODU=true, want false")
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
		Mode:        1,
		FanSpeed:    2,
	}
	state2 := &LGCNPIDUParsed{
		Power:       false,
		TargetTemp:  20.0,
		CurrentTemp: 21.0,
		Mode:        0,
		FanSpeed:    1,
	}

	// IDU#1 첫 emit → true
	if !a.shouldEmitIDU(1, state1) {
		t.Fatalf("IDU#1 first: want true")
	}
	// IDU#1 동일 state 반복 → false
	if a.shouldEmitIDU(1, state1) {
		t.Errorf("IDU#1 duplicate: want false")
	}
	// IDU#2 (다른 IDU) 첫 emit → true (IDU#1 의 cache 와 독립)
	if !a.shouldEmitIDU(2, state2) {
		t.Errorf("IDU#2 first: want true (independent cache)")
	}
	// IDU#2 동일 state 반복 → false
	if a.shouldEmitIDU(2, state2) {
		t.Errorf("IDU#2 duplicate: want false")
	}
	// IDU#1 변경 후 → true
	state1Changed := &LGCNPIDUParsed{
		Power:       true,
		TargetTemp:  26.0, // 변경
		CurrentTemp: 24.0,
		Mode:        1,
		FanSpeed:    2,
	}
	if !a.shouldEmitIDU(1, state1Changed) {
		t.Errorf("IDU#1 changed: want true")
	}
	// IDU#2 는 영향 없음 — 동일 state 면 여전히 false
	if a.shouldEmitIDU(2, state2) {
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
