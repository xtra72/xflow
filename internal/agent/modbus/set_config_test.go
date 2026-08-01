package modbus

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// M9 / AC-08: 노드 주도 런타임 설정 변경(set_config) — 재시작 없음 + init 전용 필드 거부.
// AC-08 은 agent.Process([]byte) 경계에서 검증 가능하므로(노드가 마샬링하는 것과 동일한
// {"command":"set_config",...} JSON), 여기서는 에이전트 핸들러를 직접 구동해 검증한다.
// ---------------------------------------------------------------------------

// oneGroupWithIntervalConfig 는 한 디바이스에 poll_interval 이 지정된 그룹 하나를 가지는 설정을 만든다.
func oneGroupWithIntervalConfig(groupInterval string) agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "modbus-ac08",
		Name: "AC08 Agent",
		Type: "modbus-tcp",
		Transport: agent.TransportConfig{
			Type: "modbus-tcp",
			Options: map[string]any{
				"read_mode":        "cached",
				"mode":             "interval",
				"poll_interval":    "5s", // 기본 케이던스는 느리게 — 그룹 케이던스가 관측 대상
				"request_timeout":  "1s",
				"msg_channel_size": 256,
				"devices": []any{
					map[string]any{
						"id":      "plc-1",
						"host":    "10.0.0.1",
						"port":    502,
						"unit_id": 1,
						"register_groups": []any{
							map[string]any{
								"name":          "group-a",
								"function_code": 3,
								"start_address": 0,
								"quantity":      10,
								"poll_interval": groupInterval,
							},
						},
					},
				},
			},
		},
	}
}

// testGroupInterval 은 현재 (디바이스, 그룹)의 poll_interval 을 RLock 하에 읽는 테스트 헬퍼이다.
func (a *ModbusAgent) testGroupInterval(deviceID, groupName string) time.Duration {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, dev := range a.devices {
		if dev.config.ID != deviceID {
			continue
		}
		for _, rg := range dev.config.RegisterGroups {
			if rg.Name == groupName {
				return rg.PollInterval
			}
		}
	}
	return 0
}

func setConfigJSON(t *testing.T, deviceID string, params map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"command":   "set_config",
		"device_id": deviceID,
		"params":    params,
	})
	require.NoError(t, err)
	return b
}

// TestAC08_NodeDrivenRuntimeReconfig_NoRestart 는 AC-08 의 주 시나리오를 검증한다:
//
//	(a) 노드 발행 set_config 로 그룹 poll_interval 을 재시작 없이 단축하면 더 짧은 케이던스로
//	    폴링이 관측되고, parseModbusConfig 와 동일한 검증을 통과한다.
//	(b) 같은 명령으로 transport tcp→rtu 전환을 시도하면 init 전용 필드로 거부되고,
//	    에이전트는 중단·재시작 없이 직전(단축된) 설정으로 계속 동작한다(부분 적용 없음).
func TestAC08_NodeDrivenRuntimeReconfig_NoRestart(t *testing.T) {
	mt := &mockModbusTransport{
		connected: true,
		response:  buildFC03Response(0, 1, 10),
	}
	// 초기: 그룹 A 를 느린 케이던스(200ms)로 폴링(전용 groupPollLoop).
	a, _ := newTestModbusAgent(t, oneGroupWithIntervalConfig("200ms"), mt)
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	require.Equal(t, 200*time.Millisecond, a.testGroupInterval("plc-1", "group-a"))

	// (a) 노드 발행 set_config: 그룹 poll_interval 을 20ms 로 단축(재시작 없음).
	cmd := setConfigJSON(t, "plc-1", map[string]any{
		"register_groups": []any{
			map[string]any{
				"name":          "group-a",
				"function_code": 3,
				"start_address": 0,
				"quantity":      10,
				"poll_interval": "20ms",
			},
		},
	})
	resp, err := a.Process(cmd)
	require.NoError(t, err, "런타임 set_config 는 성공해야 한다")
	require.NotNil(t, resp)

	// parseRegisterGroupConfig 검증을 재사용하여 반영됨(동일 규칙).
	require.Equal(t, 20*time.Millisecond, a.testGroupInterval("plc-1", "group-a"),
		"단축된 poll_interval 이 반영되어야 한다")
	assert.Equal(t, lifecycle.StateRunning, a.CurrentState(), "에이전트는 재시작 없이 계속 Running 이어야 한다")

	// 단축된 케이던스로 더 자주 폴링됨을 관측(재시작 없이 다음 폴 시점부터 반영).
	mt.mu.Lock()
	before := len(mt.sentFrames)
	mt.mu.Unlock()
	time.Sleep(220 * time.Millisecond) // 20ms 케이던스면 ~10회
	mt.mu.Lock()
	fastDelta := len(mt.sentFrames) - before
	mt.mu.Unlock()
	t.Logf("단축 후 220ms 동안 폴 횟수=%d", fastDelta)
	assert.GreaterOrEqual(t, fastDelta, 5, "단축된 케이던스로 더 자주 폴링되어야 한다(재시작 없이 반영)")

	// (b) transport tcp→rtu 전환 시도 → init 전용 필드로 거부.
	badCmd := setConfigJSON(t, "plc-1", map[string]any{"transport": "rtu"})
	_, err = a.Process(badCmd)
	require.Error(t, err, "transport 전환은 거부되어야 한다")
	assert.ErrorIs(t, err, ErrInitOnlyField, "init 전용 필드 오류여야 한다")

	// 거부 후에도 에이전트는 직전(단축된) 설정으로 계속 동작(부분 적용/재시작 없음).
	assert.Equal(t, lifecycle.StateRunning, a.CurrentState())
	assert.Equal(t, 20*time.Millisecond, a.testGroupInterval("plc-1", "group-a"),
		"거부 시 직전 설정(단축된 poll_interval)이 유지되어야 한다")

	mt.mu.Lock()
	afterReject := len(mt.sentFrames)
	mt.mu.Unlock()
	time.Sleep(150 * time.Millisecond)
	mt.mu.Lock()
	afterReject2 := len(mt.sentFrames)
	mt.mu.Unlock()
	assert.Greater(t, afterReject2, afterReject,
		"거부 후에도 직전 설정으로 계속 폴링해야 한다(중단 없음)")
}

// TestAC08_SetConfig_SerialHardwareRejected 는 RTU 시리얼 하드웨어 파라미터의 런타임 변경이
// init 전용으로 거부되는지 검증한다(AC-08 (b), REQ-05 unwanted).
func TestAC08_SetConfig_SerialHardwareRejected(t *testing.T) {
	for _, key := range []string{"serial_port", "baud_rate", "data_bits", "stop_bits", "parity", "port", "baud"} {
		t.Run(key, func(t *testing.T) {
			mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
			a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)
			require.NoError(t, a.Start(context.Background()))
			defer func() { _ = a.Stop(context.Background()) }()

			cmd := setConfigJSON(t, "plc-1", map[string]any{key: "some-value"})
			_, err := a.Process(cmd)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInitOnlyField, "%s 는 init 전용으로 거부되어야 한다", key)
			assert.Equal(t, lifecycle.StateRunning, a.CurrentState(), "거부 후에도 Running 유지")
		})
	}
}

// TestAC08_SetConfig_InvalidGroupRejected_NoPartialApply 는 유효하지 않은 register_groups
// (parseRegisterGroupConfig 검증 실패)가 거부되고, 직전 설정이 부분 적용 없이 유지되는지 검증한다(AC-08).
func TestAC08_SetConfig_InvalidGroupRejected_NoPartialApply(t *testing.T) {
	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
	a, _ := newTestModbusAgent(t, oneGroupWithIntervalConfig("100ms"), mt)
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	require.Equal(t, 100*time.Millisecond, a.testGroupInterval("plc-1", "group-a"))

	tests := []struct {
		name  string
		group map[string]any
	}{
		{
			name: "function_code 무효(9)",
			group: map[string]any{
				"name": "group-a", "function_code": 9,
				"start_address": 0, "quantity": 10, "poll_interval": "10ms",
			},
		},
		{
			name: "quantity 0",
			group: map[string]any{
				"name": "group-a", "function_code": 3,
				"start_address": 0, "quantity": 0, "poll_interval": "10ms",
			},
		},
		{
			name: "poll_interval 무효 문자열",
			group: map[string]any{
				"name": "group-a", "function_code": 3,
				"start_address": 0, "quantity": 10, "poll_interval": "not-a-duration",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := setConfigJSON(t, "plc-1", map[string]any{
				"register_groups": []any{tc.group},
			})
			_, err := a.Process(cmd)
			require.Error(t, err, "유효하지 않은 그룹은 거부되어야 한다")
			// 부분 적용 없음: 직전 설정 유지.
			assert.Equal(t, 100*time.Millisecond, a.testGroupInterval("plc-1", "group-a"),
				"검증 실패 시 직전 설정이 그대로 유지되어야 한다(부분 적용 금지)")
			assert.Equal(t, lifecycle.StateRunning, a.CurrentState())
		})
	}
}

// TestAC08_SetConfig_ParamValidationErrors 는 processSetConfig 의 파라미터 파싱·검증
// 오류 분기를 경계(a.Process)에서 검증한다: register_groups 형식 오류/빈 배열,
// duration 파라미터의 비문자열·무효 문자열, 런타임 가변 필드 부재. 어느 경우든
// 오류를 반환하고 에이전트는 재시작 없이 Running 을 유지한다(부분 적용 없음).
func TestAC08_SetConfig_ParamValidationErrors(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
	}{
		{"register_groups 배열 아님", map[string]any{"register_groups": "not-an-array"}},
		{"register_groups 빈 배열", map[string]any{"register_groups": []any{}}},
		{"register_groups 원소가 맵 아님", map[string]any{"register_groups": []any{"not-a-map"}}},
		{"request_timeout 문자열 아님", map[string]any{"request_timeout": 123}},
		{"request_timeout 무효 duration", map[string]any{"request_timeout": "not-a-duration"}},
		{"reconnect_interval 무효 duration", map[string]any{"reconnect_interval": "nope"}},
		{"런타임 가변 필드 없음", map[string]any{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
			a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)
			require.NoError(t, a.Start(context.Background()))
			defer func() { _ = a.Stop(context.Background()) }()

			cmd := setConfigJSON(t, "plc-1", tc.params)
			_, err := a.Process(cmd)
			require.Error(t, err, "유효하지 않은 파라미터는 거부되어야 한다")
			assert.Equal(t, lifecycle.StateRunning, a.CurrentState(), "거부 후에도 Running 유지")
		})
	}
}

// TestAC08_SetConfig_DefaultCadence_And_Timeouts 는 에이전트 기본 poll_interval 과
// request_timeout/reconnect_interval 을 런타임 변경할 수 있음을 검증한다(REQ-05 runtime-mutable).
func TestAC08_SetConfig_DefaultCadence_And_Timeouts(t *testing.T) {
	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	cmd := setConfigJSON(t, "plc-1", map[string]any{
		"poll_interval":      "250ms",
		"request_timeout":    "2s",
		"reconnect_interval": "5s",
	})
	resp, err := a.Process(cmd)
	require.NoError(t, err)
	require.NotNil(t, resp)

	a.mu.RLock()
	poll := a.config.PollInterval
	reqTO := a.config.RequestTimeout
	reconn := a.config.ReconnectInterval
	a.mu.RUnlock()
	assert.Equal(t, 250*time.Millisecond, poll)
	assert.Equal(t, 2*time.Second, reqTO)
	assert.Equal(t, 5*time.Second, reconn)
}

// TestAC08_SetConfig_AddGroup_RuntimeMutable 는 런타임에 그룹을 추가(디바이스 그룹 교체)하면
// 새 그룹이 폴링 대상이 되고 에이전트가 재시작되지 않음을 검증한다(REQ-02 add/modify).
func TestAC08_SetConfig_AddGroup_RuntimeMutable(t *testing.T) {
	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
	a, _ := newTestModbusAgent(t, oneGroupWithIntervalConfig("30ms"), mt)
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	// 그룹 A(빠름) + 신규 그룹 B(start 100, 빠름)로 교체.
	cmd := setConfigJSON(t, "plc-1", map[string]any{
		"register_groups": []any{
			map[string]any{
				"name": "group-a", "function_code": 3,
				"start_address": 0, "quantity": 10, "poll_interval": "30ms",
			},
			map[string]any{
				"name": "group-b", "function_code": 3,
				"start_address": 100, "quantity": 10, "poll_interval": "30ms",
			},
		},
	})
	_, err := a.Process(cmd)
	require.NoError(t, err)
	assert.Equal(t, 30*time.Millisecond, a.testGroupInterval("plc-1", "group-b"), "신규 그룹 B 가 반영되어야 한다")

	// 신규 그룹 B(start 100)가 실제로 폴링되는지 관측.
	require.Eventually(t, func() bool {
		mt.mu.Lock()
		defer mt.mu.Unlock()
		return countFramesByStart(mt.sentFrames, 0x64) >= 2 // start_address 100
	}, 2*time.Second, 20*time.Millisecond, "런타임 추가된 그룹 B 가 폴링되어야 한다")

	assert.Equal(t, lifecycle.StateRunning, a.CurrentState())
}

// ---------------------------------------------------------------------------
// AC-03 마감(행위 보존): transport 생략 = 기존 TCP 동작과 동일(에이전트 런타임 수준).
// config 파싱 수준 검증은 config_rtu_test.go 의 TestParseModbusConfig_TransportDefaultsTCP 가 담당하며,
// 여기서는 에이전트가 실제로 tcp 로 폴링·읽기하는 행위를 마감 검증한다.
// ---------------------------------------------------------------------------

// TestAC03_TransportAbsent_IdenticalTCPBehavior_Closing 는 transport 키가 전혀 없는 설정으로
// 생성된 에이전트가 tcp type id 를 보존하고, 폴링·직접 읽기가 기존 TCP 동작대로 수행됨을 마감 검증한다(AC-03).
func TestAC03_TransportAbsent_IdenticalTCPBehavior_Closing(t *testing.T) {
	cfg := minimalAgentConfig()
	// transport 키가 없음을 명시적으로 확인.
	_, hasTransport := cfg.Transport.Options["transport"]
	require.False(t, hasTransport, "이 테스트는 transport 키가 없는 설정을 전제로 한다")

	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
	a, _ := newTestModbusAgent(t, cfg, mt)

	// type id 보존.
	assert.Equal(t, "modbus-tcp", a.Type(), "transport 생략 시 type id 는 modbus-tcp 로 보존되어야 한다")

	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	// 폴링이 기존 TCP 동작대로 수행되어 프레임이 전송된다.
	require.Eventually(t, func() bool {
		mt.mu.Lock()
		defer mt.mu.Unlock()
		return len(mt.sentFrames) > 0
	}, 2*time.Second, 20*time.Millisecond, "transport 생략 시에도 기존 TCP 폴링이 수행되어야 한다")

	// 성공 통계가 증가하여 읽기 왕복이 정상 동작함을 확인.
	require.Eventually(t, func() bool {
		return a.devStats["plc-1"].success.Load() > 0
	}, 2*time.Second, 20*time.Millisecond, "TCP 읽기 왕복이 성공해야 한다")
}
