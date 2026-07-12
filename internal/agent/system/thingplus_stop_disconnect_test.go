package system

import (
	"context"
	"log/slog"
	"testing"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// newRunningThingplusAgentForStop 는 Stop() 테스트를 위해 Running 상태이고 주입된
// fake 클라이언트를 가진 ThingplusGatewayAgent 를 구성한다. 실제 브로커에 연결하지 않는다.
func newRunningThingplusAgentForStop(t *testing.T, client mqtt.Client) *ThingplusGatewayAgent {
	t.Helper()
	a := &ThingplusGatewayAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("thingplus-gateway")),
		agentConfig: agent.AgentConfig{
			ID:   "agent-thingplus-stop",
			Name: "stop-thingplus",
			Type: "thingplus-gateway",
		},
		cfg:        ThingplusConfig{QoS: 1, BufferSize: 4},
		devices:    newDeviceStateMap(),
		mapping:    newNameIDMap(),
		client:     client,
		recvCh:     make(chan []byte, 4),
		done:       make(chan struct{}),
		stats:      agent.NewAgentStats(),
		logger:     slog.Default(),
		upBuf:      newBoundedUplinkBuffer(4),
		pendingRPC: newPendingRPCMap(4),
	}
	require.NoError(t, a.TransitionTo(lifecycle.StateInitializing))
	require.NoError(t, a.TransitionTo(lifecycle.StateRunning))
	return a
}

// TestThingplusAgent_Stop_DisconnectsWhenDisconnected 는 재현 테스트(RED)이다.
//
// 증상: 게이트웨이 브로커 연결이 flapping 중(IsConnected==false)일 때 Stop() 을
// 호출해도 Disconnect() 가 호출되지 않아 Paho 의 auto-reconnect 가 살아남아 계속
// 재연결하며, 중복 세션이 thingplus-gateway 를 EOF 로 kick 하는 상황을 지속시킨다.
//
// 수정 전: Stop() 은 IsConnected() 가 true 일 때만 Disconnect() 를 호출하므로
// 이 테스트는 실패한다(Disconnect 미호출).
// 수정 후: Stop() 은 client != nil 이면 항상 Disconnect() 를 호출하므로 통과한다.
func TestThingplusAgent_Stop_DisconnectsWhenDisconnected(t *testing.T) {
	fake := &fakeStopMQTTClient{connected: false} // flapping: 현재 연결 끊김
	a := newRunningThingplusAgentForStop(t, fake)

	err := a.Stop(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, fake.disconnectCount(),
		"연결 끊김 상태에서도 Stop() 은 Disconnect() 를 호출하여 Paho auto-reconnect 를 취소해야 한다")
	assert.Equal(t, lifecycle.StateStopped, a.CurrentState())
}

// TestThingplusAgent_Stop_ConnectedDisconnects 는 연결된 정상 경로를 검증한다.
func TestThingplusAgent_Stop_ConnectedDisconnects(t *testing.T) {
	fake := &fakeStopMQTTClient{connected: true}
	a := newRunningThingplusAgentForStop(t, fake)

	err := a.Stop(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, fake.disconnectCount(),
		"연결 상태에서 Stop() 은 Disconnect() 를 호출해야 한다")
	assert.Equal(t, lifecycle.StateStopped, a.CurrentState())
}

// TestThingplusAgent_TransportConnected 는 TransportChecker 구현이 fake 클라이언트의
// IsConnected() 를 그대로 반영하는지 검증한다.
func TestThingplusAgent_TransportConnected(t *testing.T) {
	// 컴파일 타임: ThingplusGatewayAgent 는 TransportChecker 를 구현해야 한다.
	var _ agent.TransportChecker = (*ThingplusGatewayAgent)(nil)

	connectedFake := &fakeStopMQTTClient{connected: true}
	a := newRunningThingplusAgentForStop(t, connectedFake)
	assert.True(t, a.TransportConnected(),
		"연결된 클라이언트에서는 TransportConnected() 가 true 여야 한다")

	disconnectedFake := &fakeStopMQTTClient{connected: false}
	b := newRunningThingplusAgentForStop(t, disconnectedFake)
	assert.False(t, b.TransportConnected(),
		"연결 끊긴 클라이언트에서는 TransportConnected() 가 false 여야 한다")

	c := newRunningThingplusAgentForStop(t, nil)
	c.mu.Lock()
	c.client = nil
	c.mu.Unlock()
	assert.False(t, c.TransportConnected(),
		"client 가 nil 이면 TransportConnected() 는 false 여야 한다")
}

// TestThingplusAgent_Stop_DefeatsReconnectResurrection 는 재현 테스트(RED)이다.
//
// 증상(버그): Stop() 후에도 Paho 의 connect-retry/auto-reconnect goroutine 이 살아남아
// 재연결하면 onConnect 가 다시 다운링크 토픽을 구독하고 디바이스를 재connect 하여
// 세션이 부활하며, 중복 세션이 게이트웨이를 EOF 로 kick 하는 상황을 지속시킨다.
//
// 검증: Stop() 이 stopped 플래그를 설정하고, 이후 onConnect 가 호출되면
//   - 즉시 Disconnect 하여 재연결을 무력화하고,
//   - 다운링크 토픽을 재구독하지 않아야 한다(Subscribe 미호출).
//
// 수정 전: onConnect 에 stopped-guard 가 없으므로 Subscribe 가 호출되어 실패한다.
// 수정 후: stopped-guard 가 Disconnect 후 즉시 반환하여 통과한다.
func TestThingplusAgent_Stop_DefeatsReconnectResurrection(t *testing.T) {
	fake := &fakeStopMQTTClient{connected: true}
	a := newRunningThingplusAgentForStop(t, fake)

	require.NoError(t, a.Stop(context.Background()))
	disconnectsAfterStop := fake.disconnectCount()
	require.GreaterOrEqual(t, disconnectsAfterStop, 1, "Stop() 은 Disconnect 를 호출해야 한다")

	// Paho 가 Stop 이후 재연결에 성공하여 onConnect 가 호출되는 상황을 재현.
	a.onConnect(fake)

	assert.Equal(t, 0, fake.subscribeCount(),
		"Stop 이후 재연결 시 다운링크 토픽을 재구독하면 안 된다 (세션 부활 방지)")
	assert.Greater(t, fake.disconnectCount(), disconnectsAfterStop,
		"Stop 이후 재연결 시 stopped-guard 가 즉시 Disconnect 하여 재연결을 무력화해야 한다")
}

// TestThingplusAgent_Stop_Idempotent 는 Stop() 이 두 번 호출되어도 에러 없이 동작하고
// 매번 Disconnect 하여 연결 끊김 상태를 유지하는지 검증한다.
func TestThingplusAgent_Stop_Idempotent(t *testing.T) {
	fake := &fakeStopMQTTClient{connected: true}
	a := newRunningThingplusAgentForStop(t, fake)

	require.NoError(t, a.Stop(context.Background()), "첫 번째 Stop 은 성공해야 한다")
	assert.Equal(t, lifecycle.StateStopped, a.CurrentState())
	assert.Equal(t, 1, fake.disconnectCount())

	require.NoError(t, a.Stop(context.Background()), "두 번째 Stop(idempotent) 도 성공해야 한다")
	assert.Equal(t, lifecycle.StateStopped, a.CurrentState())
	assert.Equal(t, 2, fake.disconnectCount(),
		"idempotent Stop 도 Disconnect 를 호출하여 연결 끊김을 보장해야 한다")
}
