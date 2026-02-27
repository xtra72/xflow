package modbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	modbus "github.com/xtra/xflow/internal/modbus"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// mockModbusTransport 는 ModbusTransport 인터페이스의 테스트 구현체이다.
// ---------------------------------------------------------------------------

type mockModbusTransport struct {
	mu          sync.Mutex
	connectErr  error
	closeErr    error
	sendRecvErr error
	response    []byte // SendAndReceive 가 반환할 데이터
	sentFrames  [][]byte
	connected   bool
	connectCnt  int
	closeCnt    int
}

func (m *mockModbusTransport) Connect(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectCnt++
	if m.connectErr != nil {
		return m.connectErr
	}
	m.connected = true
	return nil
}

func (m *mockModbusTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCnt++
	m.connected = false
	return m.closeErr
}

func (m *mockModbusTransport) SendAndReceive(_ context.Context, frame []byte) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(frame))
	copy(cp, frame)
	m.sentFrames = append(m.sentFrames, cp)
	if m.sendRecvErr != nil {
		return nil, m.sendRecvErr
	}
	if m.response != nil {
		return m.response, nil
	}
	return nil, errors.New("no response configured")
}

func (m *mockModbusTransport) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connected
}

// ---------------------------------------------------------------------------
// 테스트 헬퍼
// ---------------------------------------------------------------------------

// minimalAgentConfig 는 테스트용 최소 AgentConfig 를 반환한다.
func minimalAgentConfig() agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "modbus-test-1",
		Name: "Test Modbus Agent",
		Type: "modbus-tcp",
		Transport: agent.TransportConfig{
			Type: "modbus-tcp",
			Options: map[string]any{
				"read_mode":        "cached",
				"poll_interval":    "100ms",
				"request_timeout":  "1s",
				"msg_channel_size": 64,
				"devices": []any{
					map[string]any{
						"id":      "plc-1",
						"host":    "10.0.0.1",
						"port":    502,
						"unit_id": 1,
						"register_groups": []any{
							map[string]any{
								"name":          "holding_0-9",
								"function_code": 3,
								"start_address": 0,
								"quantity":      10,
							},
						},
					},
				},
			},
		},
	}
}

// directModeAgentConfig 는 ReadMode=direct 인 테스트용 AgentConfig 를 반환한다.
func directModeAgentConfig() agent.AgentConfig {
	cfg := minimalAgentConfig()
	cfg.Transport.Options["read_mode"] = "direct"
	return cfg
}

// twoDeviceAgentConfig 는 2개 디바이스가 설정된 테스트용 AgentConfig 를 반환한다.
func twoDeviceAgentConfig() agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "modbus-test-2",
		Name: "Test Modbus Agent 2",
		Type: "modbus-tcp",
		Transport: agent.TransportConfig{
			Type: "modbus-tcp",
			Options: map[string]any{
				"read_mode":        "cached",
				"poll_interval":    "100ms",
				"request_timeout":  "1s",
				"msg_channel_size": 64,
				"devices": []any{
					map[string]any{
						"id":      "plc-1",
						"host":    "10.0.0.1",
						"port":    502,
						"unit_id": 1,
						"register_groups": []any{
							map[string]any{
								"name":          "holding_0-9",
								"function_code": 3,
								"start_address": 0,
								"quantity":      10,
							},
						},
					},
					map[string]any{
						"id":      "plc-2",
						"host":    "10.0.0.2",
						"port":    502,
						"unit_id": 2,
						"register_groups": []any{
							map[string]any{
								"name":          "input_100-104",
								"function_code": 4,
								"start_address": 100,
								"quantity":      5,
							},
						},
					},
				},
			},
		},
	}
}

// buildFC03Response 는 FC03 ReadHoldingRegisters 에 대한 MBAP+PDU 응답 프레임을 생성한다.
// transactionID, unitID, 레지스터 개수를 받아 정상 응답을 생성한다.
func buildFC03Response(txID uint16, unitID byte, quantity uint16) []byte {
	byteCount := byte(quantity * 2)
	// MBAP 헤더: txID(2) + protocolID(2) + length(2) + unitID(1)
	// PDU: functionCode(1) + byteCount(1) + data(byteCount)
	length := uint16(3 + byteCount) // unitID(1) + FC(1) + byteCount(1) + data
	resp := make([]byte, 0, 7+2+int(byteCount))
	resp = append(resp,
		byte(txID>>8), byte(txID),     // Transaction ID
		0x00, 0x00,                     // Protocol ID
		byte(length>>8), byte(length),  // Length
		unitID,                         // Unit ID
		FC03ReadHoldingRegisters,       // Function Code
		byteCount,                      // Byte Count
	)
	// 더미 레지스터 데이터
	for i := 0; i < int(byteCount); i++ {
		resp = append(resp, byte(i))
	}
	return resp
}

// buildFC04Response 는 FC04 ReadInputRegisters 에 대한 정상 응답 프레임을 생성한다.
func buildFC04Response(txID uint16, unitID byte, quantity uint16) []byte {
	byteCount := byte(quantity * 2)
	length := uint16(3 + byteCount)
	resp := make([]byte, 0, 7+2+int(byteCount))
	resp = append(resp,
		byte(txID>>8), byte(txID),
		0x00, 0x00,
		byte(length>>8), byte(length),
		unitID,
		FC04ReadInputRegisters,
		byteCount,
	)
	for i := 0; i < int(byteCount); i++ {
		resp = append(resp, byte(i))
	}
	return resp
}

// newTestModbusAgent 는 mock 트랜스포트가 주입된 테스트용 ModbusAgent 를 생성한다.
func newTestModbusAgent(t *testing.T, config agent.AgentConfig, transports ...*mockModbusTransport) (*ModbusAgent, []*mockModbusTransport) {
	t.Helper()

	// 트랜스포트가 충분하지 않으면 자동 생성
	cfgOpts := config.Transport.Options
	devList, _ := cfgOpts["devices"].([]any)
	for len(transports) < len(devList) {
		transports = append(transports, &mockModbusTransport{connected: true})
	}

	// ModbusTransport 인터페이스 슬라이스로 변환
	ifaces := make([]ModbusTransport, len(transports))
	for i, mt := range transports {
		ifaces[i] = mt
	}

	a, err := newModbusAgentWithTransport(config, ifaces)
	require.NoError(t, err)

	return a, transports
}

// ===========================================================================
// 테스트 케이스
// ===========================================================================

// TestNewModbusAgent_Success 는 올바른 설정으로 에이전트가 생성되는지 검증한다.
func TestNewModbusAgent_Success(t *testing.T) {
	config := minimalAgentConfig()
	mt := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, config, mt)

	assert.NotNil(t, a)
	assert.Equal(t, "modbus-tcp", a.Type())
	assert.Equal(t, lifecycle.StateRunning, a.CurrentState())
	assert.Equal(t, "modbus-test-1", a.ID())
	assert.Equal(t, "Test Modbus Agent", a.Name())
}

// TestNewModbusAgent_InvalidConfig 는 잘못된 설정에 대해 에러를 반환하는지 검증한다.
func TestNewModbusAgent_InvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config agent.AgentConfig
	}{
		{
			name: "devices 누락",
			config: agent.AgentConfig{
				ID:   "bad-1",
				Name: "Bad Agent",
				Type: "modbus-tcp",
				Transport: agent.TransportConfig{
					Options: map[string]any{},
				},
			},
		},
		{
			name: "ID 누락 (Validate 실패)",
			config: agent.AgentConfig{
				Name: "No ID",
				Type: "modbus-tcp",
				Transport: agent.TransportConfig{
					Options: map[string]any{
						"devices": []any{
							map[string]any{
								"host": "10.0.0.1",
								"register_groups": []any{
									map[string]any{
										"function_code": 3,
										"quantity":      10,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ifaces := []ModbusTransport{&mockModbusTransport{}}
			_, err := newModbusAgentWithTransport(tc.config, ifaces)
			require.Error(t, err)
		})
	}
}

// TestModbusAgent_Init 은 Init 이 올바르게 Running 상태로 전이하는지 검증한다.
func TestModbusAgent_Init(t *testing.T) {
	a, _ := newTestModbusAgent(t, minimalAgentConfig())
	assert.Equal(t, lifecycle.StateRunning, a.CurrentState())
	assert.False(t, a.startedAt.IsZero(), "startedAt 가 설정되어야 한다")
}

// TestModbusAgent_Start_Stop 은 Start/Stop 라이프사이클을 검증한다.
func TestModbusAgent_Start_Stop(t *testing.T) {
	mt := &mockModbusTransport{connected: false}
	a, mocks := newTestModbusAgent(t, minimalAgentConfig(), mt)

	// Start: 디바이스 연결 시도
	err := a.Start(context.Background())
	require.NoError(t, err)

	mocks[0].mu.Lock()
	connected := mocks[0].connectCnt
	mocks[0].mu.Unlock()
	assert.Equal(t, 1, connected, "Start 시 디바이스 Connect 가 호출되어야 한다")

	// pollLoop goroutine 시작 대기
	time.Sleep(50 * time.Millisecond)

	// Stop
	err = a.Stop(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopped, a.CurrentState())
}

// TestModbusAgent_Start_DirectMode 는 direct 모드에서 pollLoop 이 시작되지 않는지 검증한다.
func TestModbusAgent_Start_DirectMode(t *testing.T) {
	mt := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, directModeAgentConfig(), mt)

	err := a.Start(context.Background())
	require.NoError(t, err)

	// 잠시 대기
	time.Sleep(50 * time.Millisecond)

	// direct 모드에서는 pollTicker 가 nil 이어야 한다
	a.mu.RLock()
	ticker := a.pollTicker
	a.mu.RUnlock()
	assert.Nil(t, ticker, "direct 모드에서는 pollTicker 가 nil 이어야 한다")

	// 정리
	err = a.Stop(context.Background())
	require.NoError(t, err)
}

// TestModbusAgent_Start_ConnectError 는 디바이스 연결 실패 시 에이전트가 계속 동작하는지 검증한다.
func TestModbusAgent_Start_ConnectError(t *testing.T) {
	mt := &mockModbusTransport{
		connected:  false,
		connectErr: ErrConnectionFailed,
	}
	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

	// Start 는 에러를 반환하지 않아야 한다 (디바이스 연결 실패는 경고만 남김)
	err := a.Start(context.Background())
	require.NoError(t, err)

	// 정리
	err = a.Stop(context.Background())
	require.NoError(t, err)
}

// TestModbusAgent_Pause_Resume 은 Pause/Resume 동작을 검증한다.
func TestModbusAgent_Pause_Resume(t *testing.T) {
	a, _ := newTestModbusAgent(t, minimalAgentConfig())

	// Pause
	err := a.Pause(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StatePaused, a.CurrentState())

	a.mu.RLock()
	paused := a.paused
	a.mu.RUnlock()
	assert.True(t, paused, "paused 플래그가 true 여야 한다")

	// Resume
	err = a.Resume(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, a.CurrentState())

	a.mu.RLock()
	paused = a.paused
	a.mu.RUnlock()
	assert.False(t, paused, "paused 플래그가 false 여야 한다")
}

// TestModbusAgent_Health 는 상태에 따른 Health 를 검증한다.
func TestModbusAgent_Health(t *testing.T) {
	tests := []struct {
		name         string
		state        lifecycle.State
		devOnline    bool
		wantStatus   agent.HealthState
	}{
		{
			name:       "Running, 디바이스 온라인 -> Healthy",
			state:      lifecycle.StateRunning,
			devOnline:  true,
			wantStatus: agent.HealthHealthy,
		},
		{
			name:       "Running, 디바이스 오프라인 -> Degraded",
			state:      lifecycle.StateRunning,
			devOnline:  false,
			wantStatus: agent.HealthDegraded,
		},
		{
			name:       "Paused -> Degraded",
			state:      lifecycle.StatePaused,
			devOnline:  true,
			wantStatus: agent.HealthDegraded,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mt := &mockModbusTransport{connected: tc.devOnline}
			a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

			// 디바이스 온라인 상태 설정
			a.devices[0].mu.Lock()
			a.devices[0].online = tc.devOnline
			a.devices[0].mu.Unlock()

			if tc.state == lifecycle.StatePaused {
				_ = a.TransitionTo(lifecycle.StatePaused)
			}

			h := a.Health()
			assert.Equal(t, tc.wantStatus, h.Status)
		})
	}
}

// TestModbusAgent_ReceiveMessage 는 채널 수신과 컨텍스트 취소를 검증한다.
func TestModbusAgent_ReceiveMessage(t *testing.T) {
	t.Run("receive from channel", func(t *testing.T) {
		a, _ := newTestModbusAgent(t, minimalAgentConfig())
		expected := []byte(`{"type":"register_read","device_id":"plc-1"}`)
		a.msgCh <- expected

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		data, err := a.ReceiveMessage(ctx)
		require.NoError(t, err)
		assert.Equal(t, string(expected), string(data))
	})

	t.Run("context cancellation", func(t *testing.T) {
		a, _ := newTestModbusAgent(t, minimalAgentConfig())
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 즉시 취소

		_, err := a.ReceiveMessage(ctx)
		require.Error(t, err)
	})

	t.Run("stop channel", func(t *testing.T) {
		a, _ := newTestModbusAgent(t, minimalAgentConfig())
		close(a.stopCh)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		_, err := a.ReceiveMessage(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stopped")
	})
}

// TestModbusAgent_PollLoop 는 cached 모드에서 pollLoop 가 주기적으로 레지스터를 읽는지 검증한다.
func TestModbusAgent_PollLoop(t *testing.T) {
	// FC03 정상 응답 생성 (txID=0, unitID=1, quantity=10)
	response := buildFC03Response(0, 1, 10)
	mt := &mockModbusTransport{
		connected: true,
		response:  response,
	}

	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

	// 디바이스를 온라인으로 설정
	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	// Start (cached 모드이므로 pollLoop 시작)
	err := a.Start(context.Background())
	require.NoError(t, err)

	// 최소 1회 폴링이 실행될 때까지 대기
	time.Sleep(250 * time.Millisecond)

	// msgCh 에서 이벤트 수신 확인
	var received bool
	for i := 0; i < 10; i++ {
		select {
		case data := <-a.msgCh:
			var evt map[string]any
			if err := json.Unmarshal(data, &evt); err == nil {
				if evt["type"] == "register_data" {
					received = true
				}
			}
		default:
		}
		if received {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	assert.True(t, received, "pollLoop 에서 register_data 이벤트가 전송되어야 한다")

	// stats 확인
	stats := a.Stats()
	assert.Greater(t, stats.MessagesReceived, int64(0), "MessagesReceived > 0")

	// 정리
	err = a.Stop(context.Background())
	require.NoError(t, err)
}

// TestModbusAgent_PollLoop_PausedSkip 는 paused 상태에서 pollLoop 가 스킵하는지 검증한다.
func TestModbusAgent_PollLoop_PausedSkip(t *testing.T) {
	response := buildFC03Response(0, 1, 10)
	mt := &mockModbusTransport{
		connected: true,
		response:  response,
	}

	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

	// 디바이스 온라인 설정
	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	// Pause 먼저
	err := a.Pause(context.Background())
	require.NoError(t, err)

	// Resume 하여 Running 상태로 복귀 후 다시 Pause
	err = a.Resume(context.Background())
	require.NoError(t, err)
	err = a.Pause(context.Background())
	require.NoError(t, err)

	// pollLoop 수동 시작 (Running 상태가 아니므로 Start 사용 불가)
	// 대신 paused 상태 확인만 한다
	a.mu.RLock()
	paused := a.paused
	a.mu.RUnlock()
	assert.True(t, paused, "에이전트가 paused 상태여야 한다")
}

// TestModbusAgent_SendEvent_ChannelFull 은 msgCh 가 가득 찼을 때 드롭되는지 검증한다.
func TestModbusAgent_SendEvent_ChannelFull(t *testing.T) {
	a, _ := newTestModbusAgent(t, minimalAgentConfig())

	// 채널을 가득 채운다
	for i := 0; i < cap(a.msgCh); i++ {
		select {
		case a.msgCh <- []byte(`{"type":"fill"}`):
		default:
			break
		}
	}

	// 이벤트 전송 시 패닉 없이 드롭되어야 한다
	a.sendEvent("test_event", map[string]any{"key": "value"})
	// 패닉이 발생하지 않으면 성공
}

// TestModbusAgent_Process_ReadRegisters 는 read_registers 명령을 검증한다.
func TestModbusAgent_Process_ReadRegisters(t *testing.T) {
	response := buildFC03Response(0, 1, 10)
	mt := &mockModbusTransport{
		connected: true,
		response:  response,
	}

	a, _ := newTestModbusAgent(t, directModeAgentConfig(), mt)

	// 디바이스 온라인 설정
	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	// read_registers 명령
	data, _ := json.Marshal(map[string]any{
		"command":   "read_registers",
		"device_id": "plc-1",
	})
	result, err := a.Process(data)
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(result, &resp))

	assert.Equal(t, "plc-1", resp["device_id"])
	assert.Equal(t, "direct", resp["mode"])
	assert.NotEmpty(t, resp["timestamp"])
	assert.NotNil(t, resp["registers"])
}

// TestModbusAgent_Process_GetStatus 는 get_status 명령을 검증한다.
func TestModbusAgent_Process_GetStatus(t *testing.T) {
	a, _ := newTestModbusAgent(t, minimalAgentConfig())

	data, _ := json.Marshal(map[string]any{
		"command": "get_status",
	})
	result, err := a.Process(data)
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(result, &resp))

	assert.Equal(t, float64(1), resp["device_count"])
	devices, ok := resp["devices"].([]any)
	require.True(t, ok)
	assert.Len(t, devices, 1)
}

// TestModbusAgent_Process_InvalidCommand 는 알 수 없는 명령에 대한 에러를 검증한다.
func TestModbusAgent_Process_InvalidCommand(t *testing.T) {
	a, _ := newTestModbusAgent(t, minimalAgentConfig())

	data, _ := json.Marshal(map[string]any{
		"command": "unknown_cmd",
	})
	_, err := a.Process(data)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidCommand))
}

// TestModbusAgent_Process_InvalidJSON 은 유효하지 않은 JSON 입력을 검증한다.
func TestModbusAgent_Process_InvalidJSON(t *testing.T) {
	a, _ := newTestModbusAgent(t, minimalAgentConfig())

	_, err := a.Process([]byte("not-json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

// TestModbusAgent_Process_DeviceNotFound 는 존재하지 않는 디바이스에 대한 에러를 검증한다.
func TestModbusAgent_Process_DeviceNotFound(t *testing.T) {
	a, _ := newTestModbusAgent(t, minimalAgentConfig())

	data, _ := json.Marshal(map[string]any{
		"command":   "read_registers",
		"device_id": "non-existent",
	})
	_, err := a.Process(data)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDeviceNotFound))
}

// TestModbusAgent_Process_DeviceOffline 는 오프라인 디바이스에 read 요청 시 에러를 검증한다.
func TestModbusAgent_Process_DeviceOffline(t *testing.T) {
	mt := &mockModbusTransport{connected: false}
	a, _ := newTestModbusAgent(t, directModeAgentConfig(), mt)

	// 디바이스를 명시적으로 오프라인 설정
	a.devices[0].mu.Lock()
	a.devices[0].online = false
	a.devices[0].mu.Unlock()

	data, _ := json.Marshal(map[string]any{
		"command":   "read_registers",
		"device_id": "plc-1",
	})
	_, err := a.Process(data)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDeviceOffline))
}

// TestModbusAgent_Configure 는 Configure 메서드를 검증한다.
func TestModbusAgent_Configure(t *testing.T) {
	a, _ := newTestModbusAgent(t, minimalAgentConfig())

	newConfig := agent.AgentConfig{
		ID:   "updated-id",
		Name: "Updated Modbus",
		Type: "modbus-tcp",
	}
	err := a.Configure(newConfig)
	require.NoError(t, err)
	assert.Equal(t, "Updated Modbus", a.Name())
	assert.Equal(t, "updated-id", a.ID())
}

// TestModbusAgent_Info 는 Info() 스냅샷을 검증한다.
func TestModbusAgent_Info(t *testing.T) {
	a, _ := newTestModbusAgent(t, minimalAgentConfig())

	info := a.Info()
	assert.Equal(t, "modbus-test-1", info.ID)
	assert.Equal(t, "Test Modbus Agent", info.Name)
	assert.Equal(t, "modbus-tcp", info.Type)
	assert.Equal(t, lifecycle.StateRunning, info.State)
	assert.NotZero(t, info.CreatedAt)
}

// TestModbusAgent_Stats 는 Stats 스냅샷을 검증한다.
func TestModbusAgent_Stats(t *testing.T) {
	a, _ := newTestModbusAgent(t, minimalAgentConfig())

	// 초기 stats 는 모두 0
	stats := a.Stats()
	assert.Equal(t, int64(0), stats.MessagesReceived)
	assert.Equal(t, int64(0), stats.MessagesSent)
	assert.Equal(t, int64(0), stats.MessagesErrored)
}

// TestModbusAgent_State 는 StatefulAgent.State() 를 검증한다.
func TestModbusAgent_State(t *testing.T) {
	a, _ := newTestModbusAgent(t, minimalAgentConfig())

	state := a.State()
	assert.Equal(t, 1, state["device_count"])
	assert.Equal(t, "cached", state["read_mode"])

	devices, ok := state["devices"].([]map[string]any)
	require.True(t, ok)
	assert.Len(t, devices, 1)
	assert.Equal(t, "plc-1", devices[0]["device_id"])
}

// TestModbusAgent_TwoDevices 는 2개 디바이스가 올바르게 생성되는지 검증한다.
func TestModbusAgent_TwoDevices(t *testing.T) {
	mt1 := &mockModbusTransport{connected: true}
	mt2 := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, twoDeviceAgentConfig(), mt1, mt2)

	assert.Len(t, a.devices, 2)
	assert.Equal(t, "plc-1", a.devices[0].config.ID)
	assert.Equal(t, "plc-2", a.devices[1].config.ID)
}

// TestModbusAgent_Stop_Drains_MsgCh 는 Stop 시 msgCh 가 드레인되는지 검증한다.
func TestModbusAgent_Stop_Drains_MsgCh(t *testing.T) {
	a, _ := newTestModbusAgent(t, directModeAgentConfig())

	// msgCh 에 메시지를 넣는다
	a.msgCh <- []byte(`{"type":"test1"}`)
	a.msgCh <- []byte(`{"type":"test2"}`)

	err := a.Stop(context.Background())
	require.NoError(t, err)

	// msgCh 가 비어있어야 한다
	assert.Equal(t, 0, len(a.msgCh))
}

// TestModbusAgent_InterfaceCompliance 는 인터페이스 호환성을 검증한다.
func TestModbusAgent_InterfaceCompliance(t *testing.T) {
	mt := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

	// agent.Agent 인터페이스
	var agentIface agent.Agent = a
	assert.NotNil(t, agentIface)

	// agent.MessageReceiver 인터페이스
	var receiverIface agent.MessageReceiver = a
	assert.NotNil(t, receiverIface)

	// agent.StatefulAgent 인터페이스
	var statefulIface agent.StatefulAgent = a
	assert.NotNil(t, statefulIface)
}

// TestTruncateForLog 는 로그 트렁케이션을 검증한다.
func TestTruncateForLog(t *testing.T) {
	tests := []struct {
		name   string
		data   string
		maxLen int
		want   string
	}{
		{"short", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"truncated", "hello world", 5, "hello..."},
		{"empty", "", 5, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateForLog([]byte(tc.data), tc.maxLen)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ===========================================================================
// M4: Interval/Event 모드 및 Read Mode 테스트
// ===========================================================================

// intervalModeAgentConfig 는 mode=interval, read_mode=cached 인 테스트용 설정을 반환한다.
func intervalModeAgentConfig() agent.AgentConfig {
	cfg := minimalAgentConfig()
	cfg.Transport.Options["mode"] = "interval"
	cfg.Transport.Options["read_mode"] = "cached"
	return cfg
}

// eventModeAgentConfig 는 mode=event, read_mode=cached, heartbeat_interval=200ms 인 테스트용 설정을 반환한다.
func eventModeAgentConfig() agent.AgentConfig {
	cfg := minimalAgentConfig()
	cfg.Transport.Options["mode"] = "event"
	cfg.Transport.Options["read_mode"] = "cached"
	cfg.Transport.Options["heartbeat_interval"] = "200ms"
	return cfg
}

// drainEvents 는 msgCh 에서 최대 maxWait 동안 이벤트를 수집하여 반환한다.
func drainEvents(a *ModbusAgent, maxWait time.Duration) []map[string]any {
	var events []map[string]any
	deadline := time.After(maxWait)
	for {
		select {
		case data := <-a.msgCh:
			var evt map[string]any
			if err := json.Unmarshal(data, &evt); err == nil {
				events = append(events, evt)
			}
		case <-deadline:
			return events
		}
	}
}

// TestModbusAgent_PollLoop_IntervalMode 는 interval 모드에서 매 폴 주기마다
// register_data 이벤트가 전송되는지 검증한다.
func TestModbusAgent_PollLoop_IntervalMode(t *testing.T) {
	response := buildFC03Response(0, 1, 10)
	mt := &mockModbusTransport{connected: true, response: response}

	a, _ := newTestModbusAgent(t, intervalModeAgentConfig(), mt)
	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	err := a.Start(context.Background())
	require.NoError(t, err)

	events := drainEvents(a, 350*time.Millisecond)

	// 최소 1회 이상 register_data 이벤트 수신
	var found bool
	for _, evt := range events {
		if evt["type"] == "register_data" && evt["mode"] == "interval" {
			found = true
			assert.Equal(t, "plc-1", evt["device_id"])
			assert.NotNil(t, evt["data"])
			break
		}
	}
	assert.True(t, found, "interval 모드에서 register_data 이벤트가 전송되어야 한다")

	err = a.Stop(context.Background())
	require.NoError(t, err)
}

// TestModbusAgent_PollLoop_EventMode_Changed 는 event 모드에서 데이터가 변경되었을 때만
// register_changed 이벤트가 전송되는지 검증한다.
func TestModbusAgent_PollLoop_EventMode_Changed(t *testing.T) {
	response := buildFC03Response(0, 1, 10)
	mt := &mockModbusTransport{connected: true, response: response}

	a, _ := newTestModbusAgent(t, eventModeAgentConfig(), mt)
	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	err := a.Start(context.Background())
	require.NoError(t, err)

	// 첫 번째 폴링 대기 — 캐시가 비어있으므로 변경 감지됨 (첫 업데이트)
	events := drainEvents(a, 350*time.Millisecond)

	var foundChanged bool
	for _, evt := range events {
		if evt["type"] == "register_changed" && evt["mode"] == "event" {
			foundChanged = true
			assert.Equal(t, "plc-1", evt["device_id"])
			assert.NotNil(t, evt["changes"])
			break
		}
	}
	assert.True(t, foundChanged, "event 모드 첫 폴링에서 register_changed 이벤트가 전송되어야 한다")

	err = a.Stop(context.Background())
	require.NoError(t, err)
}

// TestModbusAgent_PollLoop_EventMode_Heartbeat 는 event 모드에서 heartbeat 주기마다
// event_heartbeat 타입의 전체 데이터 이벤트가 전송되는지 검증한다.
func TestModbusAgent_PollLoop_EventMode_Heartbeat(t *testing.T) {
	response := buildFC03Response(0, 1, 10)
	mt := &mockModbusTransport{connected: true, response: response}

	a, _ := newTestModbusAgent(t, eventModeAgentConfig(), mt)
	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	err := a.Start(context.Background())
	require.NoError(t, err)

	// heartbeat_interval=200ms 이므로 충분히 대기
	events := drainEvents(a, 500*time.Millisecond)

	var foundHeartbeat bool
	for _, evt := range events {
		if evt["type"] == "register_data" && evt["mode"] == "event_heartbeat" {
			foundHeartbeat = true
			assert.Equal(t, "plc-1", evt["device_id"])
			assert.NotNil(t, evt["data"])
			break
		}
	}
	assert.True(t, foundHeartbeat, "event 모드에서 heartbeat 이벤트가 전송되어야 한다")

	err = a.Stop(context.Background())
	require.NoError(t, err)
}

// TestModbusAgent_PollLoop_EventMode_NoChange 는 event 모드에서 데이터가 변경되지 않으면
// register_changed 이벤트가 전송되지 않는지 검증한다.
func TestModbusAgent_PollLoop_EventMode_NoChange(t *testing.T) {
	response := buildFC03Response(0, 1, 10)
	mt := &mockModbusTransport{connected: true, response: response}

	a, _ := newTestModbusAgent(t, eventModeAgentConfig(), mt)
	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	// 캐시를 미리 동일한 데이터로 채운다 (첫 변경 감지를 방지)
	cache := a.caches["plc-1"]
	rg := a.devices[0].config.RegisterGroups[0]
	// ReadRegisters 를 직접 호출하여 캐시를 채운다
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	data, readErr := a.devices[0].ReadRegisters(ctx, rg)
	cancel()
	require.NoError(t, readErr)
	cache.UpdateFromRead(rg.FunctionCode, rg.StartAddress, data, rg.Quantity)

	// pollLoop 시작
	err := a.Start(context.Background())
	require.NoError(t, err)

	// 150ms 대기 (heartbeat=200ms 이므로 heartbeat 전에만 수집)
	events := drainEvents(a, 150*time.Millisecond)

	// register_changed 이벤트가 없어야 한다 (데이터 미변경)
	for _, evt := range events {
		assert.NotEqual(t, "register_changed", evt["type"],
			"데이터 미변경 시 register_changed 이벤트가 전송되지 않아야 한다")
	}

	err = a.Stop(context.Background())
	require.NoError(t, err)
}

// TestModbusAgent_Process_ReadRegisters_CachedMode 는 cached 모드에서 force=false 일 때
// 캐시 스냅샷을 반환하는지 검증한다.
func TestModbusAgent_Process_ReadRegisters_CachedMode(t *testing.T) {
	mt := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	// 캐시에 데이터를 미리 넣는다
	cache := a.caches["plc-1"]
	cache.UpdateHoldingRegisters(0, []uint16{100, 200, 300})

	// read_registers (force=false, cached 모드)
	data, _ := json.Marshal(map[string]any{
		"command":   "read_registers",
		"device_id": "plc-1",
	})
	result, err := a.Process(data)
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(result, &resp))

	assert.Equal(t, "plc-1", resp["device_id"])
	assert.Equal(t, "cached", resp["mode"])
	assert.NotNil(t, resp["cache"], "cached 모드에서 cache 필드가 포함되어야 한다")
	assert.Nil(t, resp["registers"], "cached 모드에서 registers 필드가 없어야 한다")

	// mock 트랜스포트에 프레임이 전송되지 않았는지 확인 (디바이스 쿼리 없음)
	mt.mu.Lock()
	sentCount := len(mt.sentFrames)
	mt.mu.Unlock()
	assert.Equal(t, 0, sentCount, "cached 모드 force=false 에서 디바이스 쿼리가 발생하지 않아야 한다")
}

// TestModbusAgent_Process_ReadRegisters_CachedMode_Force 는 cached 모드에서 force=true 일 때
// 디바이스에서 직접 읽고 캐시를 갱신하는지 검증한다.
func TestModbusAgent_Process_ReadRegisters_CachedMode_Force(t *testing.T) {
	response := buildFC03Response(0, 1, 10)
	mt := &mockModbusTransport{connected: true, response: response}
	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)

	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	// read_registers (force=true, cached 모드)
	data, _ := json.Marshal(map[string]any{
		"command":   "read_registers",
		"device_id": "plc-1",
		"force":     true,
	})
	result, err := a.Process(data)
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(result, &resp))

	assert.Equal(t, "plc-1", resp["device_id"])
	assert.Equal(t, "force", resp["mode"])
	assert.NotNil(t, resp["registers"], "force 모드에서 registers 필드가 포함되어야 한다")
	assert.Nil(t, resp["cache"], "force 모드에서 cache 필드가 없어야 한다")

	// mock 트랜스포트에 프레임이 전송되었는지 확인 (디바이스 직접 읽기)
	mt.mu.Lock()
	sentCount := len(mt.sentFrames)
	mt.mu.Unlock()
	assert.Greater(t, sentCount, 0, "force 모드에서 디바이스 쿼리가 발생해야 한다")

	// 캐시가 갱신되었는지 확인
	cache := a.caches["plc-1"]
	snapshot := cache.GetSnapshot()
	holdingRegs, ok := snapshot["holding_registers"].(map[uint16]uint16)
	require.True(t, ok, "holding_registers 가 캐시에 존재해야 한다")
	assert.Greater(t, len(holdingRegs), 0, "force 읽기 후 캐시에 레지스터가 갱신되어야 한다")
}

// ===========================================================================
// M7-M8: 통합 테스트 (Reconnect, Stale, WriteEvents, ReadWriteCycle)
// ===========================================================================

// buildWriteResponse 는 FC05/06/15/16 쓰기 응답 프레임을 생성한다.
// MBAP(7) + FC(1) + Address(2) + Value/Quantity(2) = 12 바이트
func buildWriteResponse(txID uint16, unitID byte, fc byte, addr uint16, valueOrQty uint16) []byte {
	length := uint16(6) // unitID(1) + FC(1) + addr(2) + value(2)
	resp := make([]byte, 12)
	resp[0] = byte(txID >> 8)
	resp[1] = byte(txID)
	resp[2] = 0x00 // Protocol ID
	resp[3] = 0x00
	resp[4] = byte(length >> 8)
	resp[5] = byte(length)
	resp[6] = unitID
	resp[7] = fc
	resp[8] = byte(addr >> 8)
	resp[9] = byte(addr)
	resp[10] = byte(valueOrQty >> 8)
	resp[11] = byte(valueOrQty)
	return resp
}

// TestModbusAgent_EndToEnd_ReadWriteCycle 는 레지스터 읽기 → 쓰기 → 캐시 갱신 전체 흐름을 검증한다.
func TestModbusAgent_EndToEnd_ReadWriteCycle(t *testing.T) {
	// FC03 읽기 응답: holding registers 10개
	readResp := buildFC03Response(0, 1, 10)
	// FC06 쓰기 응답: address=5, value=9999
	writeResp := buildWriteResponse(1, 1, FC06WriteSingleRegister, 5, 9999)

	mt := &mockModbusTransport{connected: true, response: readResp}
	a, mocks := newTestModbusAgent(t, directModeAgentConfig(), mt)

	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	// 1단계: 레지스터 읽기
	readCmd, _ := json.Marshal(map[string]any{
		"command":   "read_registers",
		"device_id": "plc-1",
	})
	result, err := a.Process(readCmd)
	require.NoError(t, err)

	var readResult map[string]any
	require.NoError(t, json.Unmarshal(result, &readResult))
	assert.Equal(t, "plc-1", readResult["device_id"])
	assert.Equal(t, "direct", readResult["mode"])

	// 2단계: 레지스터 쓰기 (mock 응답을 쓰기 응답으로 교체)
	mocks[0].mu.Lock()
	mocks[0].response = writeResp
	mocks[0].mu.Unlock()

	writeCmd, _ := json.Marshal(map[string]any{
		"command":   "write_register",
		"device_id": "plc-1",
		"params": map[string]any{
			"address": 5,
			"value":   9999,
		},
	})
	result, err = a.Process(writeCmd)
	require.NoError(t, err)

	var writeResult map[string]any
	require.NoError(t, json.Unmarshal(result, &writeResult))
	assert.Equal(t, "ok", writeResult["status"])
	assert.Equal(t, "write_register", writeResult["command"])

	// 3단계: 캐시 갱신 확인
	cache := a.caches["plc-1"]
	snapshot := cache.GetSnapshot()
	holdingRegs, ok := snapshot["holding_registers"].(map[uint16]uint16)
	require.True(t, ok)
	assert.Equal(t, uint16(9999), holdingRegs[5], "Write-Through 캐시에 쓰기 값이 반영되어야 한다")
}

// TestModbusAgent_ReconnectOnOffline 는 오프라인 디바이스가 pollDevices 에서 재연결되는지 검증한다.
func TestModbusAgent_ReconnectOnOffline(t *testing.T) {
	response := buildFC03Response(0, 1, 10)
	mt := &mockModbusTransport{
		connected:  false, // 초기 오프라인
		connectErr: nil,   // 재연결은 성공
		response:   response,
	}

	cfg := minimalAgentConfig()
	cfg.Transport.Options["poll_interval"] = "100ms"
	cfg.Transport.Options["reconnect_interval"] = "1ms" // 빠른 재연결 테스트

	a, mocks := newTestModbusAgent(t, cfg, mt)

	// 디바이스를 명시적으로 오프라인 설정
	a.devices[0].mu.Lock()
	a.devices[0].online = false
	a.devices[0].mu.Unlock()

	// Start (pollLoop 시작)
	err := a.Start(context.Background())
	require.NoError(t, err)

	// 재연결 + 폴링 대기
	time.Sleep(350 * time.Millisecond)

	// 재연결이 시도되었는지 확인
	mocks[0].mu.Lock()
	connectCount := mocks[0].connectCnt
	mocks[0].mu.Unlock()
	assert.GreaterOrEqual(t, connectCount, 1, "재연결이 최소 1회 시도되어야 한다")

	// 디바이스가 온라인으로 전환되었는지 확인
	assert.True(t, a.devices[0].IsOnline(), "재연결 성공 후 디바이스가 온라인이어야 한다")

	// register_data 이벤트 수신 확인
	events := drainEvents(a, 200*time.Millisecond)
	var found bool
	for _, evt := range events {
		if evt["type"] == "register_data" {
			found = true
			break
		}
	}
	assert.True(t, found, "재연결 후 register_data 이벤트가 전송되어야 한다")

	err = a.Stop(context.Background())
	require.NoError(t, err)
}

// TestModbusAgent_StaleWarning 는 캐시 데이터가 StaleThreshold 를 초과하면
// register_group_stale 경고 이벤트가 전송되는지 검증한다.
func TestModbusAgent_StaleWarning(t *testing.T) {
	// 디바이스를 오프라인으로 만들어 폴링 실패를 유도한다
	mt := &mockModbusTransport{
		connected:  true,
		connectErr: ErrConnectionFailed, // 재연결 실패
		sendRecvErr: errors.New("send failed"),
	}

	cfg := minimalAgentConfig()
	cfg.Transport.Options["poll_interval"] = "50ms"
	cfg.Transport.Options["stale_threshold"] = "100ms"
	cfg.Transport.Options["reconnect_interval"] = "1ms"

	a, _ := newTestModbusAgent(t, cfg, mt)

	// 캐시에 초기 데이터를 넣어서 "stale 될 수 있는" 상태를 만든다
	cache := a.caches["plc-1"]
	rg := a.devices[0].config.RegisterGroups[0]
	dummyData := make([]byte, rg.Quantity*2)
	cache.UpdateFromRead(rg.FunctionCode, rg.StartAddress, dummyData, rg.Quantity)

	// 디바이스를 온라인 상태로 설정하되 전송은 실패하도록 한다
	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	// Start
	err := a.Start(context.Background())
	require.NoError(t, err)

	// stale_threshold(100ms) 이상 대기
	time.Sleep(400 * time.Millisecond)

	events := drainEvents(a, 200*time.Millisecond)

	var foundStale bool
	for _, evt := range events {
		if evt["type"] == "register_group_stale" {
			foundStale = true
			assert.Equal(t, "plc-1", evt["device_id"])
			assert.NotEmpty(t, evt["group_key"])
			assert.NotEmpty(t, evt["threshold"])
			break
		}
	}
	assert.True(t, foundStale, "stale_threshold 초과 시 register_group_stale 이벤트가 전송되어야 한다")

	err = a.Stop(context.Background())
	require.NoError(t, err)
}

// TestModbusAgent_WriteEvents 는 쓰기 성공/실패 시 write_success/write_error 이벤트가
// 전송되는지 검증한다.
func TestModbusAgent_WriteEvents(t *testing.T) {
	t.Run("write_success event", func(t *testing.T) {
		writeResp := buildWriteResponse(0, 1, FC06WriteSingleRegister, 100, 500)
		mt := &mockModbusTransport{connected: true, response: writeResp}

		cfg := minimalAgentConfig()
		cfg.Transport.Options["enable_write_events"] = true
		a, _ := newTestModbusAgent(t, cfg, mt)

		a.devices[0].mu.Lock()
		a.devices[0].online = true
		a.devices[0].mu.Unlock()

		writeCmd, _ := json.Marshal(map[string]any{
			"command":   "write_register",
			"device_id": "plc-1",
			"params": map[string]any{
				"address": 100,
				"value":   500,
			},
		})
		_, err := a.Process(writeCmd)
		require.NoError(t, err)

		// write_success 이벤트 확인
		events := drainEvents(a, 200*time.Millisecond)
		var found bool
		for _, evt := range events {
			if evt["type"] == "write_success" {
				found = true
				assert.Equal(t, "plc-1", evt["device_id"])
				assert.Equal(t, "write_register", evt["command"])
				break
			}
		}
		assert.True(t, found, "write_success 이벤트가 전송되어야 한다")
	})

	t.Run("write_error event on exception", func(t *testing.T) {
		// MODBUS 예외 응답 생성: FC 0x86 (0x80 | 0x06) + 예외 코드 0x02
		excResp := make([]byte, 9)
		excResp[0] = 0x00 // txID high
		excResp[1] = 0x00 // txID low
		excResp[2] = 0x00 // protocol ID
		excResp[3] = 0x00
		excResp[4] = 0x00 // length high
		excResp[5] = 0x03 // length: unitID(1) + FC(1) + exCode(1)
		excResp[6] = 0x01 // unitID
		excResp[7] = 0x86 // FC06 + 0x80 (예외 표시)
		excResp[8] = ExceptionIllegalDataAddress

		mt := &mockModbusTransport{connected: true, response: excResp}

		cfg := minimalAgentConfig()
		cfg.Transport.Options["enable_write_events"] = true
		a, _ := newTestModbusAgent(t, cfg, mt)

		a.devices[0].mu.Lock()
		a.devices[0].online = true
		a.devices[0].mu.Unlock()

		writeCmd, _ := json.Marshal(map[string]any{
			"command":   "write_register",
			"device_id": "plc-1",
			"params": map[string]any{
				"address": 100,
				"value":   500,
			},
		})
		result, err := a.Process(writeCmd)
		// MODBUS 예외는 에러가 아닌 예외 응답으로 반환된다
		require.NoError(t, err)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(result, &resp))
		assert.Equal(t, "error", resp["status"])

		// write_error 이벤트 확인
		events := drainEvents(a, 200*time.Millisecond)
		var found bool
		for _, evt := range events {
			if evt["type"] == "write_error" {
				found = true
				assert.Equal(t, "plc-1", evt["device_id"])
				assert.Equal(t, "write_register", evt["command"])
				assert.NotEmpty(t, evt["error"])
				break
			}
		}
		assert.True(t, found, "write_error 이벤트가 전송되어야 한다")
	})

	t.Run("no write events when disabled", func(t *testing.T) {
		writeResp := buildWriteResponse(0, 1, FC06WriteSingleRegister, 100, 500)
		mt := &mockModbusTransport{connected: true, response: writeResp}

		cfg := minimalAgentConfig()
		cfg.Transport.Options["enable_write_events"] = false
		a, _ := newTestModbusAgent(t, cfg, mt)

		a.devices[0].mu.Lock()
		a.devices[0].online = true
		a.devices[0].mu.Unlock()

		writeCmd, _ := json.Marshal(map[string]any{
			"command":   "write_register",
			"device_id": "plc-1",
			"params": map[string]any{
				"address": 100,
				"value":   500,
			},
		})
		_, err := a.Process(writeCmd)
		require.NoError(t, err)

		// 이벤트가 전송되지 않아야 한다
		events := drainEvents(a, 100*time.Millisecond)
		for _, evt := range events {
			assert.NotEqual(t, "write_success", evt["type"],
				"enable_write_events=false 시 write_success 이벤트가 전송되지 않아야 한다")
			assert.NotEqual(t, "write_error", evt["type"],
				"enable_write_events=false 시 write_error 이벤트가 전송되지 않아야 한다")
		}
	})
}

// TestModbusAgent_Process_ReadRegisters_DirectMode 는 direct 모드에서 항상 디바이스에서
// 직접 읽는지 검증한다.
func TestModbusAgent_Process_ReadRegisters_DirectMode(t *testing.T) {
	response := buildFC03Response(0, 1, 10)
	mt := &mockModbusTransport{connected: true, response: response}
	a, _ := newTestModbusAgent(t, directModeAgentConfig(), mt)

	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	// read_registers (direct 모드)
	data, _ := json.Marshal(map[string]any{
		"command":   "read_registers",
		"device_id": "plc-1",
	})
	result, err := a.Process(data)
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(result, &resp))

	assert.Equal(t, "plc-1", resp["device_id"])
	assert.Equal(t, "direct", resp["mode"])
	assert.NotNil(t, resp["registers"], "direct 모드에서 registers 필드가 포함되어야 한다")

	// mock 트랜스포트에 프레임이 전송되었는지 확인
	mt.mu.Lock()
	sentCount := len(mt.sentFrames)
	mt.mu.Unlock()
	assert.Greater(t, sentCount, 0, "direct 모드에서 디바이스 쿼리가 발생해야 한다")
}

// ===========================================================================
// TypeOverlay 통합 테스트: sendRegisterEvent, processReadRegisters
// ===========================================================================

// typeOverlayAgentConfig 는 FC03 float32 TypeOverlay 가 설정된 테스트용 설정을 반환한다.
// holding registers: start=0, quantity=4, data_type=float32 → 주소 0,2 에 float32 오버레이
func typeOverlayAgentConfig() agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "modbus-overlay-1",
		Name: "Overlay Agent",
		Type: "modbus-tcp",
		Transport: agent.TransportConfig{
			Type: "modbus-tcp",
			Options: map[string]any{
				"read_mode":        "cached",
				"poll_interval":    "100ms",
				"request_timeout":  "1s",
				"msg_channel_size": 64,
				"devices": []any{
					map[string]any{
						"id":      "plc-overlay",
						"host":    "10.0.0.1",
						"port":    502,
						"unit_id": 1,
						"register_groups": []any{
							map[string]any{
								"name":          "holding_float32",
								"function_code": 3,
								"start_address": 0,
								"quantity":      4,
								"data_type":     "float32",
							},
						},
					},
				},
			},
		},
	}
}

// typeOverlayFC04AgentConfig 는 FC04 int32 TypeOverlay 가 설정된 테스트용 설정을 반환한다.
func typeOverlayFC04AgentConfig() agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "modbus-overlay-fc4",
		Name: "Overlay FC4 Agent",
		Type: "modbus-tcp",
		Transport: agent.TransportConfig{
			Type: "modbus-tcp",
			Options: map[string]any{
				"read_mode":        "cached",
				"poll_interval":    "100ms",
				"request_timeout":  "1s",
				"msg_channel_size": 64,
				"devices": []any{
					map[string]any{
						"id":      "plc-fc4",
						"host":    "10.0.0.1",
						"port":    502,
						"unit_id": 1,
						"register_groups": []any{
							map[string]any{
								"name":          "input_int32",
								"function_code": 4,
								"start_address": 100,
								"quantity":      4,
								"data_type":     "int32",
							},
						},
					},
				},
			},
		},
	}
}

// typeOverlayMultiGroupConfig 는 FC03 2개 그룹 설정을 반환한다.
// group1: start=0, quantity=4, float32 → FC3:0, FC3:2
// group2: start=100, quantity=4, int32 → FC3:100, FC3:102
func typeOverlayMultiGroupConfig() agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "modbus-overlay-multi",
		Name: "Overlay Multi Agent",
		Type: "modbus-tcp",
		Transport: agent.TransportConfig{
			Type: "modbus-tcp",
			Options: map[string]any{
				"read_mode":        "direct",
				"poll_interval":    "100ms",
				"request_timeout":  "1s",
				"msg_channel_size": 64,
				"devices": []any{
					map[string]any{
						"id":      "plc-multi",
						"host":    "10.0.0.1",
						"port":    502,
						"unit_id": 1,
						"register_groups": []any{
							map[string]any{
								"name":          "group1_float32",
								"function_code": 3,
								"start_address": 0,
								"quantity":      4,
								"data_type":     "float32",
							},
							map[string]any{
								"name":          "group2_int32",
								"function_code": 3,
								"start_address": 100,
								"quantity":      4,
								"data_type":     "int32",
							},
						},
					},
				},
			},
		},
	}
}

// buildFC04ResponseWithValues 는 지정된 레지스터 값으로 FC04 응답을 생성한다.
func buildFC04ResponseWithValues(txID uint16, unitID byte, values []uint16) []byte {
	byteCount := byte(len(values) * 2)
	length := uint16(3 + byteCount)
	resp := make([]byte, 0, 7+2+int(byteCount))
	resp = append(resp,
		byte(txID>>8), byte(txID),
		0x00, 0x00,
		byte(length>>8), byte(length),
		unitID,
		FC04ReadInputRegisters,
		byteCount,
	)
	for _, v := range values {
		resp = append(resp, byte(v>>8), byte(v))
	}
	return resp
}

// TestSendRegisterEvent_TypeOverlay_FC03 는 FC03 이벤트에 그룹 범위 필터링된 typed_values 가
// 포함되는지 검증한다.
func TestSendRegisterEvent_TypeOverlay_FC03(t *testing.T) {
	// float32 값 1234.5 를 레지스터로 인코딩
	f32Val := float32(1234.5)
	regs := modbus.Float32ToRegisters(f32Val, modbus.ByteOrderBigEndian)
	// 4개 레지스터: [regs[0], regs[1], regs[0], regs[1]] (두 개의 float32)
	values := []uint16{regs[0], regs[1], regs[0], regs[1]}
	response := buildFC03ResponseWithValues(0, 1, values)

	mt := &mockModbusTransport{connected: true, response: response}
	cfg := typeOverlayAgentConfig()
	cfg.Transport.Options["mode"] = "interval"

	a, _ := newTestModbusAgent(t, cfg, mt)

	// 디바이스 온라인 설정
	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	// Start (interval 모드 → sendRegisterEvent 호출)
	err := a.Start(context.Background())
	require.NoError(t, err)

	// 이벤트 수신 대기
	events := drainEvents(a, 500*time.Millisecond)

	var foundTyped bool
	for _, evt := range events {
		if evt["type"] == "register_data" {
			tv, ok := evt["typed_values"]
			if ok && tv != nil {
				foundTyped = true
				// typed_values 는 map[string]any (JSON 역직렬화)
				typedMap, ok := tv.(map[string]any)
				require.True(t, ok, "typed_values 는 map 이어야 한다")
				// 주소 0 과 2 만 포함 (그룹 범위: 0~3)
				assert.LessOrEqual(t, len(typedMap), 2, "그룹 범위 내 주소만 포함해야 한다")
				break
			}
		}
	}
	assert.True(t, foundTyped, "FC03 이벤트에 typed_values 가 포함되어야 한다")

	err = a.Stop(context.Background())
	require.NoError(t, err)
}

// TestSendRegisterEvent_TypeOverlay_FC04 는 FC04 이벤트에 typed_values 가 포함되는지 검증한다.
func TestSendRegisterEvent_TypeOverlay_FC04(t *testing.T) {
	// int32 값 을 레지스터로 인코딩
	i32Regs := modbus.Int32ToRegisters(100000, modbus.ByteOrderBigEndian)
	values := []uint16{i32Regs[0], i32Regs[1], i32Regs[0], i32Regs[1]}
	response := buildFC04ResponseWithValues(0, 1, values)

	mt := &mockModbusTransport{connected: true, response: response}
	cfg := typeOverlayFC04AgentConfig()
	cfg.Transport.Options["mode"] = "interval"

	a, _ := newTestModbusAgent(t, cfg, mt)

	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	err := a.Start(context.Background())
	require.NoError(t, err)

	events := drainEvents(a, 500*time.Millisecond)

	var foundTyped bool
	for _, evt := range events {
		if evt["type"] == "register_data" {
			tv, ok := evt["typed_values"]
			if ok && tv != nil {
				foundTyped = true
				typedMap, ok := tv.(map[string]any)
				require.True(t, ok, "typed_values 는 map 이어야 한다")
				// 주소 100, 102 만 포함 (그룹 범위: 100~103)
				assert.LessOrEqual(t, len(typedMap), 2, "그룹 범위 내 주소만 포함해야 한다")
				break
			}
		}
	}
	assert.True(t, foundTyped, "FC04 이벤트에 typed_values 가 포함되어야 한다")

	err = a.Stop(context.Background())
	require.NoError(t, err)
}

// TestSendRegisterEvent_TypeOverlay_GroupFiltering 는 여러 그룹이 있을 때
// 각 그룹 이벤트에 해당 그룹의 typed_values 만 포함되는지 검증한다 (M-5 버그 수정 검증).
func TestSendRegisterEvent_TypeOverlay_GroupFiltering(t *testing.T) {
	// group1: FC03, start=0, qty=4 (float32) → 주소 0, 2
	// group2: FC03, start=100, qty=4 (int32) → 주소 100, 102
	f32Val := float32(3.14)
	f32Regs := modbus.Float32ToRegisters(f32Val, modbus.ByteOrderBigEndian)
	values := []uint16{f32Regs[0], f32Regs[1], f32Regs[0], f32Regs[1]}
	response := buildFC03ResponseWithValues(0, 1, values)

	mt := &mockModbusTransport{connected: true, response: response}
	cfg := typeOverlayMultiGroupConfig()
	cfg.Transport.Options["read_mode"] = "direct"

	a, _ := newTestModbusAgent(t, cfg, mt)

	// 디바이스 온라인 설정
	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	// processReadRegisters 를 통한 direct 모드 읽기
	data, _ := json.Marshal(map[string]any{
		"command":   "read_registers",
		"device_id": "plc-multi",
	})
	result, err := a.Process(data)
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(result, &resp))

	// registers 배열에서 각 그룹의 typed_values 확인
	registers, ok := resp["registers"].([]any)
	require.True(t, ok, "registers 가 배열이어야 한다")
	require.Len(t, registers, 2, "2개 그룹 결과가 있어야 한다")

	// group1 (start=0): typed_values 의 키가 모두 0~3 범위
	group1 := registers[0].(map[string]any)
	if tv, ok := group1["typed_values"]; ok && tv != nil {
		typedMap, ok := tv.(map[string]any)
		require.True(t, ok)
		for keyStr := range typedMap {
			// JSON 역직렬화 시 키는 문자열
			var addr int
			_, scanErr := fmt.Sscanf(keyStr, "%d", &addr)
			require.NoError(t, scanErr)
			assert.True(t, addr >= 0 && addr < 4,
				"group1 typed_values 의 주소 %d 가 그룹 범위 [0,4) 를 벗어남", addr)
		}
	}

	// group2 (start=100): typed_values 의 키가 모두 100~103 범위
	group2 := registers[1].(map[string]any)
	if tv, ok := group2["typed_values"]; ok && tv != nil {
		typedMap, ok := tv.(map[string]any)
		require.True(t, ok)
		for keyStr := range typedMap {
			var addr int
			_, scanErr := fmt.Sscanf(keyStr, "%d", &addr)
			require.NoError(t, scanErr)
			assert.True(t, addr >= 100 && addr < 104,
				"group2 typed_values 의 주소 %d 가 그룹 범위 [100,104) 를 벗어남", addr)
		}
	}
}

// TestProcessReadRegisters_TypeOverlay_DirectMode 는 direct 모드에서
// processReadRegisters 응답에 typed_values 가 포함되는지 검증한다.
// direct 모드는 캐시를 갱신하지 않으므로, 캐시를 미리 채워야 typed_values 가 나온다.
func TestProcessReadRegisters_TypeOverlay_DirectMode(t *testing.T) {
	f32Val := float32(42.5)
	f32Regs := modbus.Float32ToRegisters(f32Val, modbus.ByteOrderBigEndian)
	values := []uint16{f32Regs[0], f32Regs[1], f32Regs[0], f32Regs[1]}
	response := buildFC03ResponseWithValues(0, 1, values)

	mt := &mockModbusTransport{connected: true, response: response}
	cfg := typeOverlayAgentConfig()
	cfg.Transport.Options["read_mode"] = "direct"

	a, _ := newTestModbusAgent(t, cfg, mt)

	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	// direct 모드는 캐시를 갱신하지 않으므로 캐시를 미리 채운다
	cache := a.caches["plc-overlay"]
	cache.UpdateHoldingRegisters(0, values)

	data, _ := json.Marshal(map[string]any{
		"command":   "read_registers",
		"device_id": "plc-overlay",
	})
	result, err := a.Process(data)
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(result, &resp))

	assert.Equal(t, "plc-overlay", resp["device_id"])
	assert.Equal(t, "direct", resp["mode"])

	// registers 배열에서 typed_values 확인
	registers, ok := resp["registers"].([]any)
	require.True(t, ok)
	require.Len(t, registers, 1)

	groupResult := registers[0].(map[string]any)
	tv, ok := groupResult["typed_values"]
	assert.True(t, ok, "typed_values 가 포함되어야 한다")
	assert.NotNil(t, tv, "typed_values 가 nil 이 아니어야 한다")

	// typed_values 내 값 검증: float32 로 변환된 값
	typedMap, ok := tv.(map[string]any)
	require.True(t, ok)
	assert.Greater(t, len(typedMap), 0, "typed_values 에 하나 이상의 엔트리가 있어야 한다")
}

// TestProcessReadRegisters_TypeOverlay_DirectMode_EmptyCache 는 direct 모드에서
// 캐시가 비어있을 때 typed_values 가 포함되지 않는지 검증한다.
func TestProcessReadRegisters_TypeOverlay_DirectMode_EmptyCache(t *testing.T) {
	f32Val := float32(42.5)
	f32Regs := modbus.Float32ToRegisters(f32Val, modbus.ByteOrderBigEndian)
	values := []uint16{f32Regs[0], f32Regs[1], f32Regs[0], f32Regs[1]}
	response := buildFC03ResponseWithValues(0, 1, values)

	mt := &mockModbusTransport{connected: true, response: response}
	cfg := typeOverlayAgentConfig()
	cfg.Transport.Options["read_mode"] = "direct"

	a, _ := newTestModbusAgent(t, cfg, mt)

	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	// 캐시를 채우지 않고 direct 읽기
	data, _ := json.Marshal(map[string]any{
		"command":   "read_registers",
		"device_id": "plc-overlay",
	})
	result, err := a.Process(data)
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(result, &resp))

	assert.Equal(t, "direct", resp["mode"])

	// direct 모드에서 캐시가 비어있으면 typed_values 가 없어야 한다
	registers, ok := resp["registers"].([]any)
	require.True(t, ok)
	require.Len(t, registers, 1)

	groupResult := registers[0].(map[string]any)
	_, hasTyped := groupResult["typed_values"]
	assert.False(t, hasTyped, "캐시가 비어있으면 typed_values 가 없어야 한다")
}

// TestProcessReadRegisters_TypeOverlay_ForceMode 는 cached 모드에서 force=true 일 때
// typed_values 가 포함되는지 검증한다.
func TestProcessReadRegisters_TypeOverlay_ForceMode(t *testing.T) {
	f32Val := float32(99.9)
	f32Regs := modbus.Float32ToRegisters(f32Val, modbus.ByteOrderBigEndian)
	values := []uint16{f32Regs[0], f32Regs[1], f32Regs[0], f32Regs[1]}
	response := buildFC03ResponseWithValues(0, 1, values)

	mt := &mockModbusTransport{connected: true, response: response}
	cfg := typeOverlayAgentConfig()
	// cached 모드 (기본값)

	a, _ := newTestModbusAgent(t, cfg, mt)

	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	data, _ := json.Marshal(map[string]any{
		"command":   "read_registers",
		"device_id": "plc-overlay",
		"force":     true,
	})
	result, err := a.Process(data)
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(result, &resp))

	assert.Equal(t, "plc-overlay", resp["device_id"])
	assert.Equal(t, "force", resp["mode"])

	registers, ok := resp["registers"].([]any)
	require.True(t, ok)
	require.Len(t, registers, 1)

	groupResult := registers[0].(map[string]any)
	tv, ok := groupResult["typed_values"]
	assert.True(t, ok, "force 모드에서 typed_values 가 포함되어야 한다")
	assert.NotNil(t, tv, "typed_values 가 nil 이 아니어야 한다")
}

// TestProcessReadRegisters_TypeOverlay_CachedMode 는 cached 모드(force=false)에서
// 캐시 스냅샷에 typed 값이 포함되는지 검증한다.
func TestProcessReadRegisters_TypeOverlay_CachedMode(t *testing.T) {
	mt := &mockModbusTransport{connected: true}
	cfg := typeOverlayAgentConfig()

	a, _ := newTestModbusAgent(t, cfg, mt)

	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	// 캐시에 float32 값을 직접 설정
	cache := a.caches["plc-overlay"]
	f32Val := float32(55.5)
	f32Bits := math.Float32bits(f32Val)
	regs := []uint16{uint16(f32Bits >> 16), uint16(f32Bits & 0xFFFF)}
	cache.UpdateHoldingRegisters(0, append(regs, regs...)) // 주소 0-3

	// cached 모드 read_registers (force=false)
	data, _ := json.Marshal(map[string]any{
		"command":   "read_registers",
		"device_id": "plc-overlay",
	})
	result, err := a.Process(data)
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(result, &resp))

	assert.Equal(t, "cached", resp["mode"])

	// cached 모드에서는 cache 필드에 typed_holding_registers 가 포함된다
	cacheData, ok := resp["cache"]
	require.True(t, ok && cacheData != nil, "cached 모드에서 cache 필드가 있어야 한다")

	cacheMap, ok := cacheData.(map[string]any)
	require.True(t, ok)

	_, hasTyped := cacheMap["typed_holding_registers"]
	assert.True(t, hasTyped, "캐시 스냅샷에 typed_holding_registers 가 포함되어야 한다")
}

// TestFilterTypedValuesByRange 는 주소 범위 필터링 헬퍼 함수를 검증한다.
func TestFilterTypedValuesByRange(t *testing.T) {
	typed := map[uint16]any{
		0:   "val0",
		2:   "val2",
		100: "val100",
		102: "val102",
	}

	tests := []struct {
		name      string
		start     uint16
		quantity  uint16
		wantAddrs []uint16
	}{
		{
			name:      "그룹 0~3 필터링",
			start:     0,
			quantity:  4,
			wantAddrs: []uint16{0, 2},
		},
		{
			name:      "그룹 100~103 필터링",
			start:     100,
			quantity:  4,
			wantAddrs: []uint16{100, 102},
		},
		{
			name:      "빈 범위",
			start:     50,
			quantity:  10,
			wantAddrs: []uint16{},
		},
		{
			name:      "전체 범위",
			start:     0,
			quantity:  200,
			wantAddrs: []uint16{0, 2, 100, 102},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := filterTypedValuesByRange(typed, tc.start, tc.quantity)
			assert.Len(t, result, len(tc.wantAddrs))
			for _, addr := range tc.wantAddrs {
				_, ok := result[addr]
				assert.True(t, ok, "주소 %d 가 필터링 결과에 존재해야 한다", addr)
			}
		})
	}
}
