package modbus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// M2/M3 — buildDevices 토폴로지 (REQ-MODBUS-008-02/03, AC-03~AC-06)
// ---------------------------------------------------------------------------

// newAgentFromOpts 는 Transport.Options 로 modbus-client 를 생성해 반환한다(조용한 로거).
func newAgentFromOpts(t *testing.T, opts map[string]any) *ModbusAgent {
	t.Helper()
	cfg := agent.AgentConfig{
		ID:        "modbus-bd",
		Name:      "buildDevices test",
		Type:      "modbus-client",
		Transport: agent.TransportConfig{Type: "modbus-tcp", Options: opts},
		Logger:    discardLogger(),
	}
	raw, err := NewModbusAgent(cfg)
	require.NoError(t, err)
	return raw.(*ModbusAgent)
}

// TestBuildDevices_MixedPerDeviceTransport 는 에이전트 기본 tcp 하에서 D1 은 상속(tcp),
// D2 는 per-device rtu override 로 구성되어 각각 올바른 트랜스포트를 가짐을 검증한다(AC-04).
func TestBuildDevices_MixedPerDeviceTransport(t *testing.T) {
	d2 := map[string]any{
		"id":          "d2",
		"unit_id":     2,
		"transport":   "rtu",
		"serial_port": "/dev/ttyUSB0",
		"register_groups": []any{
			map[string]any{"name": "g", "function_code": 4, "start_address": 0, "quantity": 2},
		},
	}
	a := newAgentFromOpts(t, map[string]any{
		"transport": "tcp",
		"devices":   []any{makeTCPDevice("d1", "10.0.0.1"), d2},
	})
	require.Len(t, a.devices, 2)

	tcp, okTCP := a.devices[0].transport.(*ModbusTCPTransport)
	require.True(t, okTCP, "D1(상속)은 ModbusTCPTransport 여야 한다")
	assert.Equal(t, "10.0.0.1:502", tcp.endpointAddr())

	rtu, okRTU := a.devices[1].transport.(*ModbusRTUTransport)
	require.True(t, okRTU, "D2(per-device override)는 ModbusRTUTransport 여야 한다")
	assert.Equal(t, "/dev/ttyUSB0", rtu.serial.Port)
}

// TestBuildDevices_ShareSession_TCPSameEndpointShared 는 share_session:true 에서 동일
// (host,port)를 대상으로 하는 두 디바이스가 하나의 공유 트랜스포트를 사용하고, 엔드포인트가
// 다른 디바이스는 공유하지 않음을 검증한다(AC-05).
func TestBuildDevices_ShareSession_TCPSameEndpointShared(t *testing.T) {
	// D1/D2: 동일 (10.0.0.1:502), 서로 다른 unit_id → 공유.
	d1 := makeTCPDevice("d1", "10.0.0.1")
	d1["unit_id"] = 1
	d2 := makeTCPDevice("d2", "10.0.0.1")
	d2["unit_id"] = 2
	// D3: 다른 엔드포인트 → 미공유.
	d3 := makeTCPDevice("d3", "10.0.0.2")

	a := newAgentFromOpts(t, map[string]any{
		"transport":     "tcp",
		"share_session": true,
		"devices":       []any{d1, d2, d3},
	})
	require.Len(t, a.devices, 3)

	st0, ok0 := a.devices[0].transport.(*sharedTransport)
	st1, ok1 := a.devices[1].transport.(*sharedTransport)
	require.True(t, ok0, "D1 은 공유 트랜스포트여야 한다")
	require.True(t, ok1, "D2 는 공유 트랜스포트여야 한다")
	assert.Same(t, st0, st1, "동일 (host,port) 디바이스는 하나의 공유 트랜스포트를 사용해야 한다(AC-05)")
	assert.Equal(t, 2, st0.refCount(), "공유 그룹 참조 수는 디바이스 수(2)와 같아야 한다")

	// 공유 하부는 ModbusTCPTransport 여야 한다.
	inner, okInner := st0.inner.(*ModbusTCPTransport)
	require.True(t, okInner)
	assert.Equal(t, "10.0.0.1:502", inner.endpointAddr())

	// D3(다른 엔드포인트)는 별개 인스턴스여야 한다.
	st2, ok2 := a.devices[2].transport.(*sharedTransport)
	require.True(t, ok2)
	assert.NotSame(t, st0, st2, "엔드포인트가 다른 디바이스는 공유하지 않아야 한다")
	assert.Equal(t, 1, st2.refCount())
}

// TestBuildDevices_ShareSessionDefaultFalse_KeepsCurrentTopology 는 share_session 부재 시
// 기본 토폴로지(TCP 독립 / RTU 단일 버스)가 그대로 유지됨을 검증한다(AC-06(a)).
// (raw 트랜스포트 타입이 유지되어 공유 래퍼가 개입하지 않음도 함께 확인.)
func TestBuildDevices_ShareSessionDefaultFalse_KeepsCurrentTopology(t *testing.T) {
	// share_session 키 없음 + 동일 엔드포인트 두 TCP 디바이스 → 여전히 독립.
	d1 := makeTCPDevice("d1", "10.0.0.1")
	d2 := makeTCPDevice("d2", "10.0.0.1")
	a := newAgentFromOpts(t, map[string]any{
		"transport": "tcp",
		"devices":   []any{d1, d2},
	})
	require.Len(t, a.devices, 2)

	t0, ok0 := a.devices[0].transport.(*ModbusTCPTransport)
	t1, ok1 := a.devices[1].transport.(*ModbusTCPTransport)
	require.True(t, ok0)
	require.True(t, ok1)
	assert.NotSame(t, t0, t1, "share_session 부재 시 동일 엔드포인트라도 TCP 는 독립이어야 한다(현 토폴로지)")
}

// TestBuildDevices_PerDeviceShareOverride 는 에이전트 기본 share_session:false 에서 두
// 디바이스만 per-device share_session:true 로 동일 엔드포인트를 공유하고, 오버라이드 없는
// 디바이스는 독립임을 검증한다(F3 per-device override).
func TestBuildDevices_PerDeviceShareOverride(t *testing.T) {
	d1 := makeTCPDevice("d1", "10.0.0.1")
	d1["unit_id"] = 1
	d1["share_session"] = true
	d2 := makeTCPDevice("d2", "10.0.0.1")
	d2["unit_id"] = 2
	d2["share_session"] = true
	// D3: 동일 엔드포인트지만 오버라이드 없음(에이전트 기본 false) → 미공유.
	d3 := makeTCPDevice("d3", "10.0.0.1")
	d3["unit_id"] = 3

	a := newAgentFromOpts(t, map[string]any{
		"transport":     "tcp",
		"share_session": false,
		"devices":       []any{d1, d2, d3},
	})
	require.Len(t, a.devices, 3)

	st0, ok0 := a.devices[0].transport.(*sharedTransport)
	st1, ok1 := a.devices[1].transport.(*sharedTransport)
	require.True(t, ok0)
	require.True(t, ok1)
	assert.Same(t, st0, st1, "per-device share=true 동일 엔드포인트 디바이스는 공유해야 한다")
	assert.Equal(t, 2, st0.refCount())

	// D3 는 공유하지 않으므로 독립 raw TCP 트랜스포트여야 한다.
	_, isShared := a.devices[2].transport.(*sharedTransport)
	assert.False(t, isShared, "오버라이드 없는(에이전트 false 상속) 디바이스는 공유하지 않아야 한다")
	_, isRawTCP := a.devices[2].transport.(*ModbusTCPTransport)
	assert.True(t, isRawTCP, "비공유 디바이스는 독립 ModbusTCPTransport 여야 한다")
}

// TestBuildDevices_ShareSession_RTUOverrideSeparateFromInherited 는 세션 공유 경로에서 RTU
// per-device override(다른 시리얼 포트)가 에이전트-기본 rtu 상속 버스와 분리됨을 검증한다(A-5).
func TestBuildDevices_ShareSession_RTUOverrideSeparateFromInherited(t *testing.T) {
	// 에이전트 기본 rtu(/dev/ttyUSB0), share_session:true.
	// D1/D2: 상속 → 단일 버스(/dev/ttyUSB0) 공유.
	// D3: per-device rtu override(/dev/ttyUSB1) → 별개 버스.
	inh1 := map[string]any{"id": "d1", "unit_id": 1, "register_groups": []any{
		map[string]any{"name": "g", "function_code": 3, "start_address": 0, "quantity": 2}}}
	inh2 := map[string]any{"id": "d2", "unit_id": 2, "register_groups": []any{
		map[string]any{"name": "g", "function_code": 3, "start_address": 0, "quantity": 2}}}
	ovr3 := map[string]any{"id": "d3", "unit_id": 3, "transport": "rtu", "serial_port": "/dev/ttyUSB1",
		"register_groups": []any{
			map[string]any{"name": "g", "function_code": 3, "start_address": 0, "quantity": 2}}}

	a := newAgentFromOpts(t, map[string]any{
		"transport":     "rtu",
		"serial_port":   "/dev/ttyUSB0",
		"share_session": true,
		"devices":       []any{inh1, inh2, ovr3},
	})
	require.Len(t, a.devices, 3)

	st0 := a.devices[0].transport.(*sharedTransport)
	st1 := a.devices[1].transport.(*sharedTransport)
	st2 := a.devices[2].transport.(*sharedTransport)
	assert.Same(t, st0, st1, "동일 시리얼 포트 상속 디바이스는 단일 버스를 공유해야 한다")
	assert.NotSame(t, st0, st2, "다른 시리얼 포트 override 디바이스는 별개 버스여야 한다(A-5)")

	inner0 := st0.inner.(*ModbusRTUTransport)
	inner2 := st2.inner.(*ModbusRTUTransport)
	assert.Equal(t, "/dev/ttyUSB0", inner0.serial.Port)
	assert.Equal(t, "/dev/ttyUSB1", inner2.serial.Port)
}
