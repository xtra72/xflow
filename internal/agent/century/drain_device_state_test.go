package century

import (
	"encoding/json"
	"testing"
	"time"
)

// TestProcessDrainDeviceState_BasicFlow 는 v0.3.11 의 신규 Process command
// "drain_device_state" 가 device_state 버퍼를 비파괴적으로 비우고 JSON 응답
// 형식이 올바른지 검증한다.
//
// 사용자 보고 root cause: century-status 노드의 polling 경로가 msgCh 를 보지
// 못해 keepalive 가 노출되지 않음. v0.3.11 은 별도 buffer 로 polling 지원.
func TestProcessDrainDeviceState_BasicFlow(t *testing.T) {
	t.Parallel()
	// v0.4.2 gate: Reg02 + Reg04 모두 주입 + 짧은 keepalive_interval.
	batch := append([]byte{}, mustBuildReg02ResponseFrame(t, 0x3B)...)
	batch = append(batch, mustBuildReg04ResponseFrame(t, 0x3B)...)
	a, rt, cleanup := makeTestAgent(t, map[string]any{
		"report_interval": "150ms",
	}, batch)
	defer cleanup()

	// 약간 기다려서 change emit 1번 + keepalive emit 1번 이상이 발생하도록 한다.
	// keepalive_interval=150ms, keepaliveLoop 의 ticker=interval/4=37ms (since <1s).
	time.Sleep(450 * time.Millisecond)

	// Drain 명령 실행.
	cmd := []byte(`{"command":"drain_device_state"}`)
	resp, err := a.Process(cmd)
	if err != nil {
		t.Fatalf("Process(drain_device_state) error: %v", err)
	}
	var result struct {
		Count  int               `json:"count"`
		Events []json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("unmarshal response: %v (raw=%s)", err, resp)
	}
	if result.Count == 0 {
		t.Fatalf("expected device_state events in buffer, got count=0 (raw=%s)", resp)
	}
	if len(result.Events) != result.Count {
		t.Errorf("count mismatch: count=%d events=%d", result.Count, len(result.Events))
	}

	// 적어도 1개의 keepalive trigger 가 포함되어야 한다 (v0.3.10 의 keepalive 분리 + v0.3.11 buffer 결합).
	sawChange := 0
	sawKeepalive := 0
	for _, evb := range result.Events {
		var ev map[string]any
		if err := json.Unmarshal(evb, &ev); err != nil {
			t.Errorf("unmarshal event: %v (raw=%s)", err, evb)
			continue
		}
		switch ev["trigger"] {
		case TriggerChange:
			sawChange++
		case TriggerReport:
			sawKeepalive++
		}
	}
	if sawChange < 1 {
		t.Errorf("expected >=1 change event in buffer, got %d", sawChange)
	}
	if sawKeepalive < 1 {
		t.Errorf("expected >=1 keepalive event in buffer, got %d (total=%d, count=%d)",
			sawKeepalive, result.Count, sawChange+sawKeepalive)
	}

	// 두 번째 drain 호출 시 buffer 가 거의 비어있어야 한다 (destructive drain).
	// keepalive_interval=150ms 라서 drain 사이에 keepalive 가 1개 추가될 수 있음 — 1 이하 허용.
	resp2, err := a.Process(cmd)
	if err != nil {
		t.Fatalf("Process(drain_device_state) 2nd call error: %v", err)
	}
	var result2 struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(resp2, &result2); err != nil {
		t.Fatalf("unmarshal 2nd response: %v", err)
	}
	if result2.Count > 1 {
		t.Errorf("expected count<=1 on 2nd drain (drained, only 0~1 keepalive race), got %d", result2.Count)
	}

	// AC-B9 트랜스포트 write 불변식.
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0", rt.WriteCount())
	}
}

// TestPushDeviceStateBuf_DropOldest 는 buffer 가 max 를 초과하면 가장 오래된
// 항목부터 drop 되는지 검증한다 (drop-oldest semantics).
func TestPushDeviceStateBuf_DropOldest(t *testing.T) {
	t.Parallel()
	// makeTestAgent 로 agent 만 빌드하고 deviceStateBuf 를 작은 max 로 강제 설정.
	a, _, cleanup := makeTestAgent(t, nil, nil)
	defer cleanup()

	a.deviceStateBufMu.Lock()
	a.deviceStateBufMax = 3
	a.deviceStateBuf = nil
	a.deviceStateBufMu.Unlock()

	// 5개 push.
	for i := 0; i < 5; i++ {
		a.pushDeviceStateBuf([]byte(`{"i":` + string(rune('0'+i)) + `}`))
	}

	a.deviceStateBufMu.Lock()
	buf := append([]json.RawMessage(nil), a.deviceStateBuf...)
	a.deviceStateBufMu.Unlock()

	if len(buf) != 3 {
		t.Fatalf("expected len(buf)=3 (max), got %d", len(buf))
	}
	// 가장 최근 3개 (i=2, 3, 4) 만 남아야 한다.
	wantSuffix := []string{`{"i":2}`, `{"i":3}`, `{"i":4}`}
	for i, want := range wantSuffix {
		if string(buf[i]) != want {
			t.Errorf("buf[%d]=%q, want %q", i, string(buf[i]), want)
		}
	}
}
