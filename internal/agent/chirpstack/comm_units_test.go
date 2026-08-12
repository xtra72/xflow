package chirpstack

import (
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// TestParseChirpStackConfig_CommKnobs 는 comm-state 노브 파싱을 검증한다
// (REQ-M5-01/03/04): emit_comm_state / comm_report_interval / offline_threshold.
func TestParseChirpStackConfig_CommKnobs(t *testing.T) {
	tests := []struct {
		name        string
		opts        map[string]any
		wantEmit    bool
		wantReport  time.Duration
		wantOffline time.Duration
	}{
		{
			name:        "defaults",
			opts:        nil,
			wantEmit:    false,
			wantReport:  0,
			wantOffline: defaultOfflineThreshold, // 300s.
		},
		{
			name:        "int seconds",
			opts:        map[string]any{"emit_comm_state": true, "comm_report_interval": 60, "offline_threshold": 300},
			wantEmit:    true,
			wantReport:  60 * time.Second,
			wantOffline: 300 * time.Second,
		},
		{
			name:        "float seconds (JSON number)",
			opts:        map[string]any{"emit_comm_state": true, "comm_report_interval": float64(90), "offline_threshold": float64(120)},
			wantEmit:    true,
			wantReport:  90 * time.Second,
			wantOffline: 120 * time.Second,
		},
		{
			name:        "duration string",
			opts:        map[string]any{"comm_report_interval": "45s", "offline_threshold": "2m"},
			wantEmit:    false,
			wantReport:  45 * time.Second,
			wantOffline: 2 * time.Minute,
		},
		{
			name:        "zero offline keeps default",
			opts:        map[string]any{"offline_threshold": 0},
			wantEmit:    false,
			wantReport:  0,
			wantOffline: defaultOfflineThreshold,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cc := parseChirpStackConfig(agent.AgentConfig{
				Transport: agent.TransportConfig{Options: tc.opts},
			})
			if cc.EmitCommState != tc.wantEmit {
				t.Errorf("EmitCommState = %v, want %v", cc.EmitCommState, tc.wantEmit)
			}
			if cc.CommReportInterval != tc.wantReport {
				t.Errorf("CommReportInterval = %v, want %v", cc.CommReportInterval, tc.wantReport)
			}
			if cc.OfflineThreshold != tc.wantOffline {
				t.Errorf("OfflineThreshold = %v, want %v", cc.OfflineThreshold, tc.wantOffline)
			}
		})
	}
}

// TestToDuration 은 toDuration 타입 분기를 커버한다.
func TestToDuration(t *testing.T) {
	cases := []struct {
		in   any
		want time.Duration
	}{
		{5, 5 * time.Second},
		{int64(7), 7 * time.Second},
		{float64(9), 9 * time.Second},
		{byte(3), 3 * time.Second},
		{"1500ms", 1500 * time.Millisecond},
		{"not-a-duration", 0},
		{[]int{1}, 0}, // 미지원 타입.
	}
	for _, c := range cases {
		if got := toDuration(c.in); got != c.want {
			t.Errorf("toDuration(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestBestGateway 는 rxInfo 중 최대 rssi 대표 게이트웨이 선택을 검증한다 (REQ-M5-05).
func TestBestGateway(t *testing.T) {
	// 실측 픽스처: rssi -113 / -57 → 최대 -57 게이트웨이 선택.
	up, err := decodeUplink(loadRawUplink(t))
	if err != nil {
		t.Fatalf("decodeUplink: %v", err)
	}
	if len(up.RxInfo) != 2 {
		t.Fatalf("rxInfo len = %d, want 2", len(up.RxInfo))
	}
	rssi, snr, gw, ok := bestGateway(up.RxInfo)
	if !ok {
		t.Fatal("bestGateway ok=false, want true")
	}
	if rssi != -57 || snr != 13.5 || gw != "24e124fffef79304" {
		t.Errorf("bestGateway = (%d, %v, %q), want (-57, 13.5, 24e124fffef79304)", rssi, snr, gw)
	}

	// 빈 rxInfo → ok=false.
	if _, _, _, ok := bestGateway(nil); ok {
		t.Error("bestGateway(nil) ok=true, want false")
	}

	// 단일 게이트웨이.
	r, s, g, ok := bestGateway([]uplinkRxInfo{{GatewayID: "g1", RSSI: -80, SNR: 5.0}})
	if !ok || r != -80 || s != 5.0 || g != "g1" {
		t.Errorf("single gateway = (%d,%v,%q,%v)", r, s, g, ok)
	}

	// 동률 rssi → 먼저 나온 항목 유지(결정적).
	r2, _, g2, _ := bestGateway([]uplinkRxInfo{
		{GatewayID: "first", RSSI: -60},
		{GatewayID: "second", RSSI: -60},
	})
	if r2 != -60 || g2 != "first" {
		t.Errorf("tie-break = (%d,%q), want (-60,first)", r2, g2)
	}
}

// TestBuildDeviceStateRecord 는 commEntry → deviceStateRecord 매핑을 검증한다.
func TestBuildDeviceStateRecord(t *testing.T) {
	e := commEntry{
		lastSeenMs: 1786490870585,
		online:     true,
		rssi:       -57,
		snr:        13.5,
		gatewayID:  "gw-1",
	}
	rec := buildDeviceStateRecord("devEui-1", commTriggerChange, e)

	if rec.Record != recordKindDeviceState {
		t.Errorf("record = %q, want device_state", rec.Record)
	}
	if rec.Trigger != commTriggerChange {
		t.Errorf("trigger = %q", rec.Trigger)
	}
	if rec.UnitID != "devEui-1" {
		t.Errorf("unit_id = %q", rec.UnitID)
	}
	if rec.TimeMs != 1786490870585 || rec.LastSeenMs != 1786490870585 {
		t.Errorf("timeMs/lastSeenMs = %d/%d, want 1786490870585", rec.TimeMs, rec.LastSeenMs)
	}
	if !rec.State.Online || rec.State.RSSI != -57 || rec.State.SNR != 13.5 ||
		rec.State.GatewayID != "gw-1" || rec.State.LastSeenMs != 1786490870585 {
		t.Errorf("state group mismatch: %+v", rec.State)
	}
}
