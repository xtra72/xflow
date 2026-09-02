package modbus

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// M0 — 하위 호환 특성화(characterization) 베이스라인 (DDD PRESERVE)
// ---------------------------------------------------------------------------
//
// 이 파일은 기존 modbus-client 의 하위 호환 동작을 스냅샷으로 고정하여 M1~M5 편집의
// 회귀 게이트(AC-09)로 삼는다. 캡처 대상:
//   - per-device 필드 없음 + share_session 없음 + 프레임 로그 없음의 기존 설정이
//     오늘과 동일한 연결 토폴로지·파싱 기본값으로 동작함.
//   - TCP: 디바이스별 독립 트랜스포트(연결 공유 없음).
//   - RTU: 전 디바이스가 단일 버스(단일 트랜스포트)를 공유.
//   - 프레임 로그 토글은 기본 비활성(no-op).
//
// 하드웨어 없이 검증한다(트랜스포트 인스턴스 아이덴티티/설정 파싱만 관찰).

// discardLogger 는 특성화 테스트용 조용한 로거를 반환한다.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// twoDeviceRTUAgentConfig 는 RTU 단일 버스 토폴로지 검증용 2-디바이스 설정을 반환한다.
func twoDeviceRTUAgentConfig() agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "modbus-rtu-2",
		Name: "Test Modbus RTU Agent",
		Type: "modbus-client",
		Transport: agent.TransportConfig{
			Type: "modbus-rtu",
			Options: map[string]any{
				"transport":   "rtu",
				"serial_port": "/dev/ttyUSB0",
				"baud_rate":   9600,
				"devices": []any{
					map[string]any{
						"id":      "slave-1",
						"unit_id": 1,
						"register_groups": []any{
							map[string]any{"name": "hold", "function_code": 3, "start_address": 0, "quantity": 4},
						},
					},
					map[string]any{
						"id":      "slave-2",
						"unit_id": 2,
						"register_groups": []any{
							map[string]any{"name": "inp", "function_code": 4, "start_address": 0, "quantity": 4},
						},
					},
				},
			},
		},
	}
}

// TestCharacterize_TCPTopology_IndependentTransports 는 기존 TCP 토폴로지를 고정한다:
// per-device 필드 없이 다중 TCP 디바이스는 각자 독립된 ModbusTCPTransport 를 가진다
// (연결 공유 없음 — F3 share_session 도입 후에도 기본값에서 이 토폴로지가 유지되어야 함).
func TestCharacterize_TCPTopology_IndependentTransports(t *testing.T) {
	cfg := twoDeviceAgentConfig()
	cfg.Logger = discardLogger()

	raw, err := NewModbusAgent(cfg)
	require.NoError(t, err)
	a := raw.(*ModbusAgent)

	require.Len(t, a.devices, 2)

	t0, ok0 := a.devices[0].transport.(*ModbusTCPTransport)
	t1, ok1 := a.devices[1].transport.(*ModbusTCPTransport)
	require.True(t, ok0, "device 0 은 ModbusTCPTransport 여야 한다")
	require.True(t, ok1, "device 1 은 ModbusTCPTransport 여야 한다")

	// 독립 연결: 두 트랜스포트는 서로 다른 인스턴스여야 한다(연결 공유 없음).
	assert.NotSame(t, t0, t1, "TCP 디바이스는 독립된 트랜스포트를 가져야 한다(공유 없음)")
	// 각 트랜스포트는 자신의 엔드포인트로 구성되어야 한다.
	assert.Equal(t, "10.0.0.1:502", t0.endpointAddr())
	assert.Equal(t, "10.0.0.2:502", t1.endpointAddr())
}

// TestCharacterize_RTUTopology_SharedBus 는 기존 RTU 토폴로지를 고정한다:
// 전 디바이스가 단일 ModbusRTUTransport(단일 버스)를 공유한다(반이중 멀티드롭).
func TestCharacterize_RTUTopology_SharedBus(t *testing.T) {
	cfg := twoDeviceRTUAgentConfig()
	cfg.Logger = discardLogger()

	raw, err := NewModbusAgent(cfg)
	require.NoError(t, err)
	a := raw.(*ModbusAgent)

	require.Len(t, a.devices, 2)

	rtu0, ok0 := a.devices[0].transport.(*ModbusRTUTransport)
	rtu1, ok1 := a.devices[1].transport.(*ModbusRTUTransport)
	require.True(t, ok0, "device 0 은 ModbusRTUTransport 여야 한다")
	require.True(t, ok1, "device 1 은 ModbusRTUTransport 여야 한다")

	// 단일 버스 공유: 두 디바이스는 동일한 트랜스포트 인스턴스를 공유해야 한다.
	assert.Same(t, rtu0, rtu1, "RTU 디바이스는 단일 버스(단일 트랜스포트)를 공유해야 한다")
}

// TestCharacterize_DefaultConfig_FrameLogDisabled 는 프레임 로그 키가 없는 기존 설정에서
// 프레임 로그가 기본 비활성(no-op)임을 고정한다(AC-02 (a), AC-09).
func TestCharacterize_DefaultConfig_FrameLogDisabled(t *testing.T) {
	cfg := minimalAgentConfig()
	cfg.Logger = discardLogger()

	raw, err := NewModbusAgent(cfg)
	require.NoError(t, err)
	a := raw.(*ModbusAgent)

	// obs 는 항상 존재하되, 두 토글 모두 꺼져 있어야 한다(완전 no-op).
	require.NotNil(t, a.obs)
	assert.False(t, a.obs.framesOn(), "log_frames 키 부재 시 프레임 요약 로그는 꺼져 있어야 한다")
	assert.False(t, a.obs.rawOn(), "log_raw_frames 키 부재 시 raw 로그는 꺼져 있어야 한다")
}

// TestCharacterize_ParseDefaults_UnchangedWithoutNewKeys 는 신규 키(log_frames/
// log_raw_frames)가 없는 설정의 파싱 결과가 기존 기본값과 동일함을 고정한다(하위 호환).
func TestCharacterize_ParseDefaults_UnchangedWithoutNewKeys(t *testing.T) {
	opts := minimalAgentConfig().Transport.Options

	cfg, err := parseModbusConfig(opts)
	require.NoError(t, err)

	// 기존 기본값 유지 확인 (대표 필드).
	assert.Equal(t, TransportTCP, cfg.Transport)
	assert.Equal(t, "cached", cfg.ReadMode)
	assert.Equal(t, "interval", cfg.Mode)
	// 신규 프레임 로그 필드는 기본 false 여야 한다(하위 호환 — 바이트 동일 동작).
	assert.False(t, cfg.LogFrames)
	assert.False(t, cfg.LogRawFrames)
}

// TestCharacterize_NoOpRoundTripStable 는 프레임 로그 비활성 상태에서 읽기 트랜잭션이
// 기존과 동일하게 성공하고, 프레임 로그가 전혀 방출되지 않음을 고정한다(mock 트랜스포트,
// no-op 경로 — AC-02 (a)).
func TestCharacterize_NoOpRoundTripStable(t *testing.T) {
	cfg := directModeAgentConfig()
	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
	a, _ := newTestModbusAgent(t, cfg, mt)
	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	// obs 는 기본 비활성.
	require.False(t, a.obs.framesOn())

	// 직접 읽기 트랜잭션 수행 (기존 동작).
	data, err := json.Marshal(map[string]any{
		"command": "read_registers", "device_id": "plc-1", "force": true,
	})
	require.NoError(t, err)
	_, err = a.Process(data)
	require.NoError(t, err)

	// mock 트랜스포트는 프레임 로그 hook 을 갖지 않으므로 방출이 없어야 한다(경로 안정성 확인).
	require.Len(t, mt.sentFrames, 1, "1회 읽기 트랜잭션이 수행되어야 한다")
}
