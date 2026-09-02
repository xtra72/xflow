package modbus

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// M1 — F1 백엔드 list_devices (REQ-MODBUS-009-01, AC-01/AC-02)
// list_devices 는 agent.Process([]byte) 경계에서 검증 가능하므로(노드가 마샬링하는 것과 동일한
// {"command":"list_devices"} JSON), 여기서는 에이전트 핸들러를 직접 구동해 검증한다.
// ---------------------------------------------------------------------------

// listDevicesJSON 은 list_devices 명령 JSON 을 만든다.
func listDevicesJSON(t *testing.T) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"command": "list_devices"})
	require.NoError(t, err)
	return b
}

// zeroDeviceAgentConfig 는 디바이스가 하나도 없는 테스트용 AgentConfig 를 반환한다(AC-02).
func zeroDeviceAgentConfig() agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "modbus-zero",
		Name: "Zero Device Agent",
		Type: "modbus-client",
		Transport: agent.TransportConfig{
			Type: "modbus-tcp",
			Options: map[string]any{
				"read_mode":        "cached",
				"poll_interval":    "100ms",
				"request_timeout":  "1s",
				"msg_channel_size": 64,
				// devices 없음 — 0-device 에이전트.
			},
		},
	}
}

// TestListDevices_MultiDevice 는 2개 이상 디바이스 구성에서 list_devices 가 각 디바이스의
// 식별·연결·폴링 메타데이터 배열을 반환함을 검증한다(AC-01).
func TestListDevices_MultiDevice(t *testing.T) {
	mt1 := &mockModbusTransport{connected: true}
	mt2 := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, twoDeviceAgentConfig(), mt1, mt2)

	resp, err := a.Process(listDevicesJSON(t))
	require.NoError(t, err)

	var out struct {
		Data        []map[string]any `json:"data"`
		DeviceCount int              `json:"device_count"`
	}
	require.NoError(t, json.Unmarshal(resp, &out))

	// device_count 는 modbus-gateway list_devices 표면과 정렬한다.
	assert.Equal(t, 2, out.DeviceCount)
	require.Len(t, out.Data, 2, "data 배열은 2개 디바이스를 담아야 한다")

	// device_id 로 인덱싱하여 순서 무관하게 검증한다.
	byID := make(map[string]map[string]any, len(out.Data))
	for _, d := range out.Data {
		id, _ := d["device_id"].(string)
		byID[id] = d
	}

	// plc-1: host 10.0.0.1, port 502, unit_id 1, transport tcp, fc3 group.
	d1, ok := byID["plc-1"]
	require.True(t, ok, "plc-1 이 목록에 포함되어야 한다")
	assert.Equal(t, "plc-1", d1["id"], "modbus-client 고유 필드 id 가 device_id 와 정합해야 한다")
	assert.Equal(t, "10.0.0.1", d1["host"])
	assert.Equal(t, float64(502), d1["port"], "JSON 숫자는 float64 로 디코드된다")
	assert.Equal(t, float64(1), d1["unit_id"])
	assert.Equal(t, "tcp", d1["transport"], "유효 트랜스포트는 tcp(에이전트 기본 상속)여야 한다")
	assert.Equal(t, false, d1["share_session"], "기본 share_session 은 false 여야 한다")
	_, hasOnline := d1["online"]
	assert.True(t, hasOnline, "online 필드가 존재해야 한다")

	groups1, ok := d1["register_groups"].([]any)
	require.True(t, ok, "register_groups 는 배열이어야 한다")
	require.Len(t, groups1, 1)
	g1 := groups1[0].(map[string]any)
	assert.Equal(t, "holding_0-9", g1["name"])
	assert.Equal(t, float64(3), g1["function_code"])
	assert.Equal(t, float64(0), g1["start_address"])
	assert.Equal(t, float64(10), g1["quantity"])

	// plc-2: unit_id 2, fc4 group.
	d2, ok := byID["plc-2"]
	require.True(t, ok, "plc-2 이 목록에 포함되어야 한다")
	assert.Equal(t, "10.0.0.2", d2["host"])
	assert.Equal(t, float64(2), d2["unit_id"])
	groups2 := d2["register_groups"].([]any)
	require.Len(t, groups2, 1)
	g2 := groups2[0].(map[string]any)
	assert.Equal(t, "input_100-104", g2["name"])
	assert.Equal(t, float64(4), g2["function_code"])
	assert.Equal(t, float64(100), g2["start_address"])
}

// TestListDevices_ZeroDevice_EmptyAndReadOnly 는 0-device 상태에서 list_devices 가 빈 목록을
// 오류 없이 반환하고(AC-02), 공유 상태(devices/캐시/통계 맵)를 변형하지 않음을 검증한다(read-only).
func TestListDevices_ZeroDevice_EmptyAndReadOnly(t *testing.T) {
	a, _ := newTestModbusAgent(t, zeroDeviceAgentConfig())
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	// read-only 검증을 위해 호출 전 공유 상태의 정체성/길이를 스냅샷한다.
	a.mu.RLock()
	devicesLenBefore := len(a.devices)
	cachesBefore := a.caches
	devStatsBefore := a.devStats
	groupStatsBefore := a.groupStats
	a.mu.RUnlock()

	resp, err := a.Process(listDevicesJSON(t))
	require.NoError(t, err, "0-device 에서도 오류 없이 반환해야 한다")

	var out struct {
		Data        []map[string]any `json:"data"`
		DeviceCount int              `json:"device_count"`
	}
	require.NoError(t, json.Unmarshal(resp, &out))
	assert.Equal(t, 0, out.DeviceCount)
	assert.Empty(t, out.Data, "0-device 는 빈 배열을 반환해야 한다")
	assert.NotNil(t, out.Data, "빈 배열은 null 이 아니라 [] 여야 한다(프론트 소비 안정성)")

	// read-only: devices/캐시/통계 맵이 변형되지 않아야 한다(동일 참조 + 동일 길이).
	a.mu.RLock()
	assert.Equal(t, devicesLenBefore, len(a.devices), "list_devices 는 devices 를 변형하지 않아야 한다")
	assert.Equal(t, len(cachesBefore), len(a.caches))
	assert.Equal(t, len(devStatsBefore), len(a.devStats))
	assert.Equal(t, len(groupStatsBefore), len(a.groupStats))
	a.mu.RUnlock()
}

// TestListDevices_ReadOnly_ConcurrentPolling 는 폴링 goroutine 이 동작 중인 상태에서 list_devices 를
// 반복 호출해도 공유 상태를 변형하지 않음을 검증한다(AC-02, `go test -race` 로 경합 부재 확인).
func TestListDevices_ReadOnly_ConcurrentPolling(t *testing.T) {
	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
	a, _ := newTestModbusAgent(t, oneGroupWithIntervalConfig("5ms"), mt)
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	// 폴링이 시작될 때까지 대기(핫 패스와 list_devices 읽기가 겹치게 한다).
	require.Eventually(t, func() bool {
		mt.mu.Lock()
		defer mt.mu.Unlock()
		return len(mt.sentFrames) > 0
	}, time.Second, 5*time.Millisecond, "폴링이 시작되어야 한다")

	// 폴링과 동시에 list_devices 를 반복 호출한다(-race 로 경합 부재를 확인).
	for i := 0; i < 20; i++ {
		resp, err := a.Process(listDevicesJSON(t))
		require.NoError(t, err)
		require.NotNil(t, resp)
		time.Sleep(2 * time.Millisecond)
	}
}
