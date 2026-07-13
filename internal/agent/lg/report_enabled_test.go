package lg

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// 디바이스별 상태 전송 on/off (report_enabled) 테스트
// ---------------------------------------------------------------------------

// drainMsgMapsLG 는 msgCh 를 한 번에 비우고 파싱된 맵 목록을 반환한다.
func drainMsgMapsLG(a *LGAPAgent) []map[string]any {
	var out []map[string]any
	for {
		select {
		case b := <-a.msgCh:
			var m map[string]any
			if json.Unmarshal(b, &m) == nil {
				out = append(out, m)
			}
		default:
			return out
		}
	}
}

// isDeviceStateMsg 는 device_state 메시지인지 판별한다.
// device_state 는 최상위 type 필드가 없고(sendEventLocked eventType="") state 그룹을
// 갖는다. device_online/offline 등 이벤트는 최상위 type 필드를 갖는다.
func isDeviceStateMsg(m map[string]any) bool {
	if _, hasType := m["type"]; hasType {
		return false
	}
	_, hasState := m["state"].(map[string]any)
	return hasState
}

// stateGroup 은 device_state 메시지의 state 그룹을 반환한다.
func stateGroup(m map[string]any) map[string]any {
	s, _ := m["state"].(map[string]any)
	return s
}

// TestReportEnabledLG_False_SuppressesAllEmissions 는 report_enabled=false 인 LGAP
// 디바이스가 device_state / 디바이스 이벤트를 모두 억제하는지 검증한다.
func TestReportEnabledLG_False_SuppressesAllEmissions(t *testing.T) {
	a, _ := newConnLGAPAgent(t, 0, time.Second, 0x10)

	a.mu.Lock()
	a.devices[0x10].ReportEnabled = false
	a.mu.Unlock()

	// online 전이 + 상태 변경을 유도. report_enabled=false 이므로 아무것도 방출 안 됨.
	a.handleResponse(0x10, onlineResponse(0x10))
	// offline 전이 경로도 억제되는지 확인차 incrementErrorCount 로 threshold 도달 유도.
	for i := 0; i < a.lgapConfig.OfflineThreshold; i++ {
		a.incrementErrorCount(0x10)
	}

	assert.Empty(t, drainMsgMapsLG(a), "report_enabled=false 는 모든 방출 억제(state/event)")

	a.recentMu.Lock()
	snapCount := len(a.recentSnapshots)
	a.recentMu.Unlock()
	assert.Equal(t, 0, snapCount, "report_enabled=false 는 device_state 스냅샷 억제")
}

// TestReportEnabledLG_True_Emits 는 report_enabled=true(기본) 인 디바이스가 정상적으로
// device_state(online=true) / device_online 을 방출하는지 검증한다(억제 회귀 방지).
// device_connection 스트림 제거(연결 정보 device_state 일원화) 이후 online 은 device_state
// 의 state 그룹으로 전달된다.
func TestReportEnabledLG_True_Emits(t *testing.T) {
	a, _ := newConnLGAPAgent(t, 0, time.Second, 0x10)
	// newConnLGAPAgent 는 ReportEnabled=true 로 device 를 생성한다.

	a.handleResponse(0x10, onlineResponse(0x10))

	msgs := drainMsgMapsLG(a)
	var sawState, sawOnline bool
	for _, m := range msgs {
		if isDeviceStateMsg(m) {
			sawState = true
			s := stateGroup(m)
			assert.Equal(t, true, s["online"], "device_state.state.online=true (online 전이)")
			assert.Contains(t, s, "error_count", "state 그룹에 error_count 포함")
			assert.Contains(t, s, "offline_threshold", "state 그룹에 offline_threshold 포함")
			// transport_connected 는 online=true 이면 정보량이 없어 생략한다(offline 일 때만 emit).
			assert.NotContains(t, s, "transport_connected", "online=true 이면 transport_connected 생략")
		}
		if t, _ := m["type"].(string); t == "device_online" {
			sawOnline = true
		}
	}
	assert.True(t, sawState, "report_enabled=true 는 device_state 방출")
	assert.True(t, sawOnline, "report_enabled=true 는 device_online 방출")
}

// TestProcessSetDeviceLG_TogglesReportEnabled 는 set_device 명령이 dev.ReportEnabled 를
// 갱신하고 응답에 반영하는지 검증한다.
func TestProcessSetDeviceLG_TogglesReportEnabled(t *testing.T) {
	a, _ := newConnLGAPAgent(t, 0, time.Second, 0x10)
	zone := 0x10

	b, err := a.processSetDevice(&processRequest{
		Command: "set_device",
		Zone:    &zone,
		Params:  map[string]any{"report_enabled": false, "name": "거실"},
	})
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(b, &resp))
	assert.Equal(t, "ok", resp["status"])
	assert.Equal(t, false, resp["report_enabled"])
	assert.Equal(t, "거실", resp["name"])

	a.mu.RLock()
	re := a.devices[0x10].ReportEnabled
	a.mu.RUnlock()
	assert.False(t, re, "dev.ReportEnabled 가 false 로 갱신되어야 함")
}

// TestGetPersistableDevicesLG_PreservesReportEnabled 는 report_enabled 가 영속화 대상
// DeviceEntry 로 왕복 보존되는지 검증한다.
func TestGetPersistableDevicesLG_PreservesReportEnabled(t *testing.T) {
	a, _ := newConnLGAPAgent(t, 0, time.Second, 0x10, 0x11)

	a.mu.Lock()
	a.devices[0x10].ReportEnabled = false
	a.mu.Unlock()

	entries := a.GetPersistableDevices()
	byAddr := map[string]agent.DeviceEntry{}
	for _, e := range entries {
		byAddr[e.Address] = e
	}

	e1 := byAddr[fmt.Sprintf("0x%x", 0x10)]
	require.NotNil(t, e1.ReportEnabled, "off 디바이스는 non-nil ReportEnabled")
	assert.False(t, *e1.ReportEnabled)

	e2 := byAddr[fmt.Sprintf("0x%x", 0x11)]
	require.NotNil(t, e2.ReportEnabled, "on 디바이스도 non-nil ReportEnabled")
	assert.True(t, *e2.ReportEnabled)
}
