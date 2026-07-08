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
