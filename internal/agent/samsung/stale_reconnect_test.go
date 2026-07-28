package samsung

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 회귀 테스트: half-open 소켓 감지 → transport 강제 재연결 (tcp-client)
//
// 근본 결함: tcp-client 트랜스포트가 half-open(stale) 상태가 되면 Receive 가 매
// read_timeout 마다 timeout 에러를 반환하지만, isConnectionError(timeout)==false 라
// Available() 이 계속 true 로 남아 receiveLoop 가 재연결 경로(Available()==false)로
// 진입하지 못한다. checkStaleDevices 는 dev.Online=false 로만 뒤집고 transport 를
// 닫지 않아 수동 재연결이 필요했다.
//
// 수정: checkStaleDevices 가 "수신 이력이 있는 모든 디바이스" 가 idle-timeout(2 ×
// stale 임계값) 이상 전면 침묵하면 tcp-client 트랜스포트를 강제 Close() 하여 재연결
// 경로로 진입시킨다. 재연결 폭주 방지 가드(isReconnecting / lastStaleReconnect 쿨다운)
// 로 반복 close 를 막고, 정상 idle 과 구분한다.
// ---------------------------------------------------------------------------

// setupStaleReconnectAgent 는 tcp-client 트랜스포트 + 짧은 임계값의 테스트 에이전트를
// 만든다. OfflineTimeout=20ms → staleOfflineThreshold=20ms, idleTimeout=40ms.
func setupStaleReconnectAgent(t *testing.T) (*Hvacr01Agent, *mockTransport, NasaAddress) {
	t.Helper()
	a, mt := newConnAgent(t, "200001")
	addr, _ := ParseNasaAddress("200001")

	a.mu.Lock()
	a.hvacr01Config.TransportType = "tcp-client"
	a.hvacr01Config.OfflineTimeout = 20 * time.Millisecond
	a.mu.Unlock()

	return a, mt, addr
}

// TestStaleReconnect_HalfOpen_ForcesTransportClose 는 전면 침묵(half-open) 상황에서
// checkStaleDevices 가 tcp-client 트랜스포트를 강제 Close() 하는지 검증한다.
func TestStaleReconnect_HalfOpen_ForcesTransportClose(t *testing.T) {
	a, mt, addr := setupStaleReconnectAgent(t)

	// online + 마지막 수신이 idle-timeout(40ms) 을 훨씬 초과 → half-open 재현.
	a.mu.Lock()
	a.devices[addr].Online = true
	a.devices[addr].LastSeen = time.Now().Add(-500 * time.Millisecond)
	a.mu.Unlock()

	require.True(t, mt.Available(), "사전조건: transport available")

	a.checkStaleDevices()

	require.Equal(t, 1, mt.getCloseCallCount(),
		"half-open 감지 시 transport.Close() 가 1회 호출되어야 한다")
	require.False(t, mt.Available(),
		"강제 close 후 Available()==false → receiveLoop 재연결 경로 진입 가능")
}

// TestStaleReconnect_NormalIdle_NoForceClose 는 최근 수신(정상 idle) 상황에서는
// 강제 close 가 발생하지 않는지 검증한다 (half-open 과 구분).
func TestStaleReconnect_NormalIdle_NoForceClose(t *testing.T) {
	a, mt, addr := setupStaleReconnectAgent(t)

	// 최근 수신(idle-timeout 이내) → half-open 아님.
	a.mu.Lock()
	a.devices[addr].Online = true
	a.devices[addr].LastSeen = time.Now()
	a.mu.Unlock()

	a.checkStaleDevices()

	require.Equal(t, 0, mt.getCloseCallCount(),
		"정상 idle(최근 수신) 에서는 강제 close 하지 않아야 한다")
	require.True(t, mt.Available(), "정상 idle 에서 transport 는 available 유지")
}

// TestStaleReconnect_Cooldown_NoStorm 은 강제 close 직후 재연결(available 복구)이
// 이루어져도 쿨다운 가드가 idle-timeout 이내 재-close 를 막아 폭주하지 않는지 검증한다.
func TestStaleReconnect_Cooldown_NoStorm(t *testing.T) {
	a, mt, addr := setupStaleReconnectAgent(t)

	a.mu.Lock()
	a.devices[addr].Online = true
	a.devices[addr].LastSeen = time.Now().Add(-500 * time.Millisecond)
	a.mu.Unlock()

	// 1회차: half-open → 강제 close.
	a.checkStaleDevices()
	require.Equal(t, 1, mt.getCloseCallCount())

	// 재연결 성공을 시뮬레이션 (Available 복구). 디바이스는 여전히 침묵 상태.
	mt.setAvailable(true)

	// 2회차: 즉시 재호출 → 쿨다운(lastStaleReconnect < idleTimeout) 가 재-close 를 막아야 한다.
	a.checkStaleDevices()
	require.Equal(t, 1, mt.getCloseCallCount(),
		"쿨다운 이내 재호출 시 transport 를 다시 close 하면 안 된다 (재연결 폭주 방지)")
}

// TestStaleReconnect_NonTCPClient_NoForceClose 는 tcp-client 가 아닌(serial 등)
// 트랜스포트에서는 half-open 조건이어도 강제 close 하지 않는지 검증한다.
// (serial 은 opener-level read timeout 으로 이미 bounded; tcp-server 는 listener 붕괴 방지.)
func TestStaleReconnect_NonTCPClient_NoForceClose(t *testing.T) {
	a, mt, addr := setupStaleReconnectAgent(t)

	a.mu.Lock()
	a.hvacr01Config.TransportType = "serial" // tcp-client 아님
	a.devices[addr].Online = true
	a.devices[addr].LastSeen = time.Now().Add(-500 * time.Millisecond)
	a.mu.Unlock()

	a.checkStaleDevices()

	require.Equal(t, 0, mt.getCloseCallCount(),
		"serial 트랜스포트에서는 half-open 강제 close 를 하지 않아야 한다")
}

// TestStaleReconnect_AlreadyDisconnected_NoDoubleClose 는 이미 transport 가 끊긴
// (Available()==false) 상태에서는 중복 close 하지 않는지 검증한다 (reconnectLoop 가 처리 중).
func TestStaleReconnect_AlreadyDisconnected_NoDoubleClose(t *testing.T) {
	a, mt, addr := setupStaleReconnectAgent(t)

	a.mu.Lock()
	a.devices[addr].Online = true
	a.devices[addr].LastSeen = time.Now().Add(-500 * time.Millisecond)
	a.mu.Unlock()

	mt.setAvailable(false) // 이미 끊김

	a.checkStaleDevices()

	require.Equal(t, 0, mt.getCloseCallCount(),
		"이미 Available()==false 이면 중복 close 하지 않아야 한다")
}
