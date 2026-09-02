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
// SPEC-MODBUS-013 REQ-01 — 레지스터 그룹 사용 여부(enabled)
// AC-01 기본값 사용 / AC-02 미사용 그룹 무읽기 / AC-03 전체 미사용 디바이스 무동작
// ---------------------------------------------------------------------------

// TestParseRegisterGroupConfig_Enabled 는 그룹 사용 여부 파싱을 검증한다(AC-01).
func TestParseRegisterGroupConfig_Enabled(t *testing.T) {
	tests := []struct {
		name        string
		value       any
		hasValue    bool
		wantErr     bool
		wantPtrNil  bool
		wantEnabled bool
	}{
		{name: "미지정 → nil, IsEnabled()=true(하위 호환)", hasValue: false, wantPtrNil: true, wantEnabled: true},
		{name: "true → 사용", value: true, hasValue: true, wantEnabled: true},
		{name: "false → 미사용", value: false, hasValue: true, wantEnabled: false},
		{name: "문자열 → 오류", value: "true", hasValue: true, wantErr: true},
		{name: "숫자 → 오류", value: 1, hasValue: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := map[string]any{
				"name":          "g",
				"function_code": 3,
				"start_address": 0,
				"quantity":      10,
			}
			if tt.hasValue {
				m["enabled"] = tt.value
			}
			rg, err := parseRegisterGroupConfig(m, 0, 0)
			if tt.wantErr {
				require.Error(t, err, "오류를 기대했다")
				return
			}
			require.NoError(t, err)
			if tt.wantPtrNil {
				assert.Nil(t, rg.Enabled, "미지정이면 포인터는 nil 이어야 한다")
			}
			assert.Equal(t, tt.wantEnabled, rg.IsEnabled())
		})
	}
}

// TestRegisterGroupConfig_IsEnabled_ZeroValue 는 구조체 zero value 가 "사용"으로
// 해석되는지 확인한다. 값 타입 bool 이었다면 여기서 false 가 되어 기존 그룹이
// 무음 정지하는 회귀가 발생한다(포인터 타입 선택의 직접 근거).
func TestRegisterGroupConfig_IsEnabled_ZeroValue(t *testing.T) {
	var rg RegisterGroupConfig
	assert.True(t, rg.IsEnabled(), "zero value 는 사용(true)이어야 한다")
}

// enabledAgentConfig 는 fc3 그룹 3개(가운데만 enabled=false)를 가진 단일 디바이스 설정을 만든다.
func enabledAgentConfig(enabledFlags [3]any) agent.AgentConfig {
	groups := make([]any, 0, 3)
	specs := [3]struct {
		name string
		addr int
	}{{"g0", 0}, {"g1", 10}, {"g2", 20}}
	for i, sp := range specs {
		g := map[string]any{
			"name":          sp.name,
			"function_code": 3,
			"start_address": sp.addr,
			"quantity":      2,
		}
		if enabledFlags[i] != nil {
			g["enabled"] = enabledFlags[i]
		}
		groups = append(groups, g)
	}
	return agent.AgentConfig{
		ID:   "modbus-enabled",
		Name: "Enabled Test Agent",
		Type: "modbus-client",
		Transport: agent.TransportConfig{
			Type: "modbus-tcp",
			Options: map[string]any{
				"read_mode":        "cached",
				"mode":             "interval",
				"poll_interval":    "10s",
				"request_timeout":  "1s",
				"msg_channel_size": 64,
				"devices": []any{
					map[string]any{
						"id": "d1", "host": "10.0.0.1", "port": 502, "unit_id": 1,
						"register_groups": groups,
					},
				},
			},
		},
	}
}

// readPDUAddrs 는 mock 이 기록한 PDU 들에서 (fc, 시작주소, 수량) 삼중항을 뽑는다.
// 읽기 PDU 형식: [fc, addrHi, addrLo, qtyHi, qtyLo].
func readPDUAddrs(t *testing.T, mt *mockModbusTransport) [][3]int {
	t.Helper()
	mt.mu.Lock()
	defer mt.mu.Unlock()
	out := make([][3]int, 0, len(mt.sentFrames))
	for _, f := range mt.sentFrames {
		require.GreaterOrEqual(t, len(f), 5, "읽기 PDU 는 최소 5바이트")
		out = append(out, [3]int{
			int(f[0]),
			int(f[1])<<8 | int(f[2]),
			int(f[3])<<8 | int(f[4]),
		})
	}
	return out
}

// TestPoll_DisabledGroupNotRead 는 미사용 그룹이 읽히지 않고 나머지는 정상 동작함을 검증한다(AC-02).
func TestPoll_DisabledGroupNotRead(t *testing.T) {
	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 2)}
	a, _ := newTestModbusAgent(t, enabledAgentConfig([3]any{nil, false, true}), mt)
	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	a.pollDevices(false)

	got := readPDUAddrs(t, mt)
	// g0(미지정→사용, addr 0)·g2(명시 true, addr 20)만 읽히고 g1(addr 10)은 읽히지 않는다.
	assert.Equal(t, [][3]int{{3, 0, 2}, {3, 20, 2}}, got,
		"미사용 그룹(addr 10)에 대한 읽기가 발생하면 안 된다")
}

// TestPoll_AllDisabledDevice_NoRead 는 모든 그룹이 미사용인 디바이스가 아무 읽기도
// 수행하지 않고 오류로도 처리되지 않음을 검증한다(AC-03).
func TestPoll_AllDisabledDevice_NoRead(t *testing.T) {
	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 2)}
	a, _ := newTestModbusAgent(t, enabledAgentConfig([3]any{false, false, false}), mt)
	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	errBefore := a.stats.Snapshot().ExternalMessagesErrored

	a.pollDevices(false)

	assert.Empty(t, readPDUAddrs(t, mt), "물리 읽기가 0회여야 한다")
	assert.Equal(t, errBefore, a.stats.Snapshot().ExternalMessagesErrored,
		"오류 통계가 증가하면 안 된다")
}

// TestStartDeviceBlockLoops_SkipsDisabled 는 전용 케이던스 그룹이라도 미사용이면
// 폴링 goroutine 이 뜨지 않음을 검증한다(AC-02, 세 호출부 공통 단일 지점).
func TestStartDeviceBlockLoops_SkipsDisabled(t *testing.T) {
	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 2)}
	a, _ := newTestModbusAgent(t, enabledAgentConfig([3]any{nil, nil, nil}), mt)

	disabled := false
	enabled := true
	dev := a.devices[0]

	a.mu.Lock()
	dev.config.RegisterGroups = []RegisterGroupConfig{
		{Name: "off", FunctionCode: 3, StartAddress: 0, Quantity: 2,
			PollInterval: 200 * time.Millisecond, Enabled: &disabled},
		{Name: "on", FunctionCode: 3, StartAddress: 100, Quantity: 2,
			PollInterval: 200 * time.Millisecond, Enabled: &enabled},
	}
	a.startDeviceBlockLoops(dev)
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.stopDeviceBlockLoops("d1")
		a.mu.Unlock()
	}()

	a.mu.RLock()
	_, offExists := a.groupStops[groupStatKey("d1", "off")]
	_, onExists := a.groupStops[groupStatKey("d1", "on")]
	a.mu.RUnlock()
	assert.False(t, offExists, "미사용 그룹의 폴링 루프가 등록되면 안 된다")
	assert.True(t, onExists, "사용 그룹의 폴링 루프는 등록되어야 한다")
}

// TestProcessListDevices_EnabledAlwaysPresent 는 list_devices 응답이 항상 enabled 키를
// 유효값으로 포함하는지 검증한다(AC-01, 프론트 체크박스 계약).
func TestProcessListDevices_EnabledAlwaysPresent(t *testing.T) {
	mt := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, enabledAgentConfig([3]any{nil, false, true}), mt)
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	raw, err := a.Process(listDevicesJSON(t))
	require.NoError(t, err)

	var out struct {
		Data []struct {
			RegisterGroups []struct {
				Name    string `json:"name"`
				Enabled *bool  `json:"enabled"`
			} `json:"register_groups"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	require.Len(t, out.Data, 1)
	require.Len(t, out.Data[0].RegisterGroups, 3)

	for _, g := range out.Data[0].RegisterGroups {
		require.NotNil(t, g.Enabled, "그룹 %q 에 enabled 키가 있어야 한다", g.Name)
	}
	assert.True(t, *out.Data[0].RegisterGroups[0].Enabled, "미지정 → true")
	assert.False(t, *out.Data[0].RegisterGroups[1].Enabled, "명시 false → false")
	assert.True(t, *out.Data[0].RegisterGroups[2].Enabled, "명시 true → true")
}
