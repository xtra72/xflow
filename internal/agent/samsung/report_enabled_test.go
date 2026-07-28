package samsung

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// 디바이스별 상태 전송 on/off (report_enabled) 테스트
// ---------------------------------------------------------------------------

// drainDeviceEvents 는 msgCh 에서 디바이스 이벤트의 type 필드만 추출한다(non-blocking).
func drainDeviceEvents(a *Hvacr01Agent) []string {
	var out []string
	for {
		select {
		case b := <-a.msgCh:
			var m map[string]any
			if json.Unmarshal(b, &m) == nil {
				if t, ok := m["type"].(string); ok {
					out = append(out, t)
				}
			}
		default:
			return out
		}
	}
}

// TestReportEnabled_False_SuppressesAllEmissions 는 report_enabled=false 인 디바이스가
// device_state 스냅샷 / 디바이스 이벤트를 모두 억제하는지 검증한다.
func TestReportEnabled_False_SuppressesAllEmissions(t *testing.T) {
	a, _ := newConnAgent(t, "200001")
	addr, _ := ParseNasaAddress("200001")

	a.mu.Lock()
	a.devices[addr].ReportEnabled = false
	a.mu.Unlock()

	// 정상 online 전이를 유도하는 프레임. report_enabled=false 이므로 아무것도 방출 안 됨.
	a.handleMessage(onlineFrame(addr))
	// stale offline 경로도 억제되는지 확인차 markOfflineLocked 를 강제.
	a.mu.Lock()
	a.markOfflineLocked(addr, a.devices[addr], "test")
	a.mu.Unlock()

	// device_state 스냅샷 억제 (recentSnapshots 링버퍼 경유).
	assert.Empty(t, drainStateMsgs(t, a), "report_enabled=false 는 device_state 스냅샷 억제")

	// device_state 스냅샷 억제 (recentSnapshots 에 아무것도 없어야).
	a.recentMu.Lock()
	snapCount := len(a.recentSnapshots)
	a.recentMu.Unlock()
	assert.Equal(t, 0, snapCount, "report_enabled=false 는 device_state 스냅샷 억제")

	// 디바이스 이벤트(device_online/device_offline 등) 억제 (msgCh 경유).
	assert.Empty(t, drainDeviceEvents(a), "report_enabled=false 는 디바이스 이벤트 억제")
}

// TestReportEnabled_True_Emits 는 report_enabled=true(기본) 인 디바이스가 정상적으로
// device_state 스냅샷 / 디바이스 이벤트를 방출하는지 검증한다(억제 회귀 방지).
func TestReportEnabled_True_Emits(t *testing.T) {
	a, _ := newConnAgent(t, "200001")
	addr, _ := ParseNasaAddress("200001")
	// newConnAgent 는 ReportEnabled=true 로 device 를 생성한다.

	a.handleMessage(onlineFrame(addr))

	assert.NotEmpty(t, drainStateMsgs(t, a), "report_enabled=true 는 device_state 스냅샷 방출")
	assert.Contains(t, drainDeviceEvents(a), "device_online", "report_enabled=true 는 device_online 방출")

	a.recentMu.Lock()
	snapCount := len(a.recentSnapshots)
	a.recentMu.Unlock()
	assert.Greater(t, snapCount, 0, "report_enabled=true 는 상태 스냅샷 방출")
}

// TestProcessSetDevice_TogglesReportEnabled 는 set_device 명령이 dev.ReportEnabled 를
// 갱신하고 응답에 반영하는지 검증한다.
func TestProcessSetDevice_TogglesReportEnabled(t *testing.T) {
	a, _ := newConnAgent(t, "200001")
	addr, _ := ParseNasaAddress("200001")

	// off 로 토글.
	b, err := a.processSetDevice(&processRequest{
		Command: "set_device",
		Address: "200001",
		Params:  map[string]any{"report_enabled": false, "name": "거실"},
	})
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(b, &resp))
	assert.Equal(t, "ok", resp["status"])
	assert.Equal(t, false, resp["report_enabled"])
	assert.Equal(t, "거실", resp["name"])

	a.mu.RLock()
	re := a.devices[addr].ReportEnabled
	nm := a.devices[addr].Name
	a.mu.RUnlock()
	assert.False(t, re, "dev.ReportEnabled 가 false 로 갱신되어야 함")
	assert.Equal(t, "거실", nm)

	// 다시 on 으로 토글(idempotent 재갱신 확인).
	b2, err := a.processSetDevice(&processRequest{
		Command: "set_device",
		Address: "200001",
		Params:  map[string]any{"report_enabled": true},
	})
	require.NoError(t, err)
	var resp2 map[string]any
	require.NoError(t, json.Unmarshal(b2, &resp2))
	assert.Equal(t, true, resp2["report_enabled"])
}

// TestGetPersistableDevices_PreservesReportEnabled 는 report_enabled 가 영속화 대상
// DeviceEntry 로 왕복 보존되는지 검증한다(off 설정이 재시작 후에도 유지).
func TestGetPersistableDevices_PreservesReportEnabled(t *testing.T) {
	a, _ := newConnAgent(t, "200001", "200002")
	addr1, _ := ParseNasaAddress("200001")

	a.mu.Lock()
	a.devices[addr1].Source = "bridge"
	a.devices[addr1].ReportEnabled = false
	addr2, _ := ParseNasaAddress("200002")
	a.devices[addr2].Source = "bridge"
	a.mu.Unlock()

	entries := a.GetPersistableDevices()
	byAddr := map[string]agent.DeviceEntry{}
	for _, e := range entries {
		byAddr[e.Address] = e
	}

	e1 := byAddr[addr1.String()]
	require.NotNil(t, e1.ReportEnabled, "off 디바이스는 non-nil ReportEnabled")
	assert.False(t, *e1.ReportEnabled)

	e2 := byAddr[addr2.String()]
	require.NotNil(t, e2.ReportEnabled, "on 디바이스도 non-nil ReportEnabled")
	assert.True(t, *e2.ReportEnabled)
}
