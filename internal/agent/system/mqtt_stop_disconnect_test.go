package system

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// fakeStopMQTTClient 는 mqtt.Client 인터페이스를 만족하는 테스트용 fake이다.
// mqtt.Client 를 임베딩하여 미구현 메서드는 자동으로 인터페이스를 만족시키고
// (호출 시 nil panic — Stop() 은 IsConnected/Unsubscribe/Disconnect 만 호출한다),
// Stop() 이 사용하는 메서드만 오버라이드하여 호출 여부를 기록한다.
//
// 이 fake 로 "연결 끊김(IsConnected==false) 상태에서도 Stop() 이 Disconnect() 를
// 호출하는가" 를 검증한다. Paho 의 auto-reconnect 를 취소하려면 Disconnect() 호출이
// IsConnected() 여부와 무관하게 항상 일어나야 하기 때문이다.
type fakeStopMQTTClient struct {
	mqtt.Client // 임베딩: 미사용 메서드 자동 만족

	mu                sync.Mutex
	connected         bool
	disconnectCalls   int
	disconnectQuiesce uint
	unsubscribed      [][]string
	subscribed        []string // Subscribe 로 요청된 토픽 기록 (stopped-guard 검증용)
}

func (f *fakeStopMQTTClient) IsConnected() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connected
}

func (f *fakeStopMQTTClient) Disconnect(quiesce uint) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.disconnectCalls++
	f.disconnectQuiesce = quiesce
}

func (f *fakeStopMQTTClient) Unsubscribe(topics ...string) mqtt.Token {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unsubscribed = append(f.unsubscribed, append([]string(nil), topics...))
	return &fakeToken{}
}

// Subscribe 는 OnConnect 재구독 경로(mqtt subscribe / thingplus subscribeDownlink)가
// 호출하는 메서드이다. stopped-guard 가 동작하면 이 메서드가 호출되지 않아야 한다.
func (f *fakeStopMQTTClient) Subscribe(topic string, _ byte, _ mqtt.MessageHandler) mqtt.Token {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subscribed = append(f.subscribed, topic)
	return &fakeToken{}
}

func (f *fakeStopMQTTClient) disconnectCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.disconnectCalls
}

func (f *fakeStopMQTTClient) subscribeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subscribed)
}

// newRunningMQTTAgentForStop 는 Stop() 테스트를 위해 Running 상태이고 주입된
// fake 클라이언트를 가진 MQTTAgent 를 구성한다. 실제 브로커에 연결하지 않는다.
func newRunningMQTTAgentForStop(t *testing.T, client mqtt.Client) *MQTTAgent {
	t.Helper()
	a := &MQTTAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("mqtt-client")),
		agentConfig: agent.AgentConfig{
			ID:   "agent-mqtt-stop",
			Name: "stop-mqtt",
			Type: "mqtt-client",
		},
		mqttConfig: MQTTConfig{
			Topics: []string{"sensor/#"},
			QoS:    1,
		},
		client: client,
		recvCh: make(chan []byte, 4),
		done:   make(chan struct{}),
		stats:  agent.NewAgentStats(),
		logger: slog.Default(),
	}
	// Created → Initializing → Running 으로 전이하여 Stop() 의 Running→Stopping 을 허용한다.
	require.NoError(t, a.TransitionTo(lifecycle.StateInitializing))
	require.NoError(t, a.TransitionTo(lifecycle.StateRunning))
	return a
}

// TestMQTTAgent_Stop_DisconnectsWhenDisconnected 는 재현 테스트(RED)이다.
//
// 증상: 브로커 연결이 flapping 중(IsConnected==false)일 때 Stop() 을 호출해도
// Disconnect() 가 호출되지 않아 Paho 의 auto-reconnect 가 살아남아 계속 재연결한다.
//
// 수정 전: Stop() 은 IsConnected() 가 true 일 때만 Disconnect() 를 호출하므로
// 이 테스트는 실패한다(Disconnect 미호출).
// 수정 후: Stop() 은 client != nil 이면 항상 Disconnect() 를 호출하므로 통과한다.
func TestMQTTAgent_Stop_DisconnectsWhenDisconnected(t *testing.T) {
	fake := &fakeStopMQTTClient{connected: false} // flapping: 현재 연결 끊김
	a := newRunningMQTTAgentForStop(t, fake)

	err := a.Stop(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, fake.disconnectCount(),
		"연결 끊김 상태에서도 Stop() 은 Disconnect() 를 호출하여 Paho auto-reconnect 를 취소해야 한다")
	assert.Equal(t, lifecycle.StateStopped, a.CurrentState())
}

// TestMQTTAgent_Stop_ConnectedUnsubscribesAndDisconnects 는 연결된 정상 경로를 검증한다.
// 연결 상태에서는 구독 해제 후 Disconnect 를 호출해야 한다.
func TestMQTTAgent_Stop_ConnectedUnsubscribesAndDisconnects(t *testing.T) {
	fake := &fakeStopMQTTClient{connected: true}
	a := newRunningMQTTAgentForStop(t, fake)

	err := a.Stop(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, fake.disconnectCount(),
		"연결 상태에서 Stop() 은 Disconnect() 를 호출해야 한다")

	fake.mu.Lock()
	unsubCount := len(fake.unsubscribed)
	fake.mu.Unlock()
	assert.Equal(t, 1, unsubCount,
		"연결 상태에서 Stop() 은 설정된 토픽을 Unsubscribe 해야 한다")
	assert.Equal(t, lifecycle.StateStopped, a.CurrentState())
}

// TestMQTTAgent_TransportConnected 는 TransportChecker 구현이 fake 클라이언트의
// IsConnected() 를 그대로 반영하는지 검증한다. 이를 통해 UI 연결 상태가 라이프사이클
// 상태가 아닌 실제 Paho 연결을 반영하게 된다.
func TestMQTTAgent_TransportConnected(t *testing.T) {
	// 컴파일 타임: MQTTAgent 는 TransportChecker 를 구현해야 한다.
	var _ agent.TransportChecker = (*MQTTAgent)(nil)

	connectedFake := &fakeStopMQTTClient{connected: true}
	a := newRunningMQTTAgentForStop(t, connectedFake)
	assert.True(t, a.TransportConnected(),
		"연결된 클라이언트에서는 TransportConnected() 가 true 여야 한다")

	disconnectedFake := &fakeStopMQTTClient{connected: false}
	b := newRunningMQTTAgentForStop(t, disconnectedFake)
	assert.False(t, b.TransportConnected(),
		"연결 끊긴 클라이언트에서는 TransportConnected() 가 false 여야 한다")

	// client 가 nil 이면 false 여야 한다 (스레드 세이프 nil 처리).
	c := newRunningMQTTAgentForStop(t, nil)
	c.mu.Lock()
	c.client = nil
	c.mu.Unlock()
	assert.False(t, c.TransportConnected(),
		"client 가 nil 이면 TransportConnected() 는 false 여야 한다")
}

// TestMQTTAgent_Stop_DefeatsReconnectResurrection 는 재현 테스트(RED)이다.
//
// 증상(버그): Stop() 후에도 Paho 의 connect-retry/auto-reconnect goroutine 이 살아남아
// 재연결하면 OnConnectHandler(=subscribe)가 다시 토픽을 구독하여 세션이 부활한다.
//
// 검증: Stop() 이 stopped 플래그를 설정하고, 이후 OnConnect 경로(subscribe)가 호출되면
//   - 즉시 Disconnect 하여 재연결을 무력화하고,
//   - 토픽을 재구독하지 않아야 한다(Subscribe 미호출).
//
// 수정 전: subscribe() 에 stopped-guard 가 없으므로 Subscribe 가 호출되어 실패한다.
// 수정 후: stopped-guard 가 Disconnect 후 즉시 반환하여 통과한다.
func TestMQTTAgent_Stop_DefeatsReconnectResurrection(t *testing.T) {
	// Paho 가 Stop 이후 재연결에 성공한 상황을 모사: 연결됨 상태.
	fake := &fakeStopMQTTClient{connected: true}
	a := newRunningMQTTAgentForStop(t, fake)

	require.NoError(t, a.Stop(context.Background()))
	disconnectsAfterStop := fake.disconnectCount()
	require.GreaterOrEqual(t, disconnectsAfterStop, 1, "Stop() 은 Disconnect 를 호출해야 한다")

	// Paho 가 Stop 이후 재연결에 성공하여 OnConnectHandler 가 호출되는 상황을 재현.
	a.subscribe(fake)

	assert.Equal(t, 0, fake.subscribeCount(),
		"Stop 이후 재연결 시 토픽을 재구독하면 안 된다 (세션 부활 방지)")
	assert.Greater(t, fake.disconnectCount(), disconnectsAfterStop,
		"Stop 이후 재연결 시 stopped-guard 가 즉시 Disconnect 하여 재연결을 무력화해야 한다")
}

// TestMQTTAgent_Stop_Idempotent 는 Stop() 이 두 번 호출되어도 에러 없이 동작하고
// 매번 Disconnect 하여 연결 끊김 상태를 유지하는지 검증한다.
func TestMQTTAgent_Stop_Idempotent(t *testing.T) {
	fake := &fakeStopMQTTClient{connected: true}
	a := newRunningMQTTAgentForStop(t, fake)

	require.NoError(t, a.Stop(context.Background()), "첫 번째 Stop 은 성공해야 한다")
	assert.Equal(t, lifecycle.StateStopped, a.CurrentState())
	first := fake.disconnectCount()
	assert.Equal(t, 1, first)

	// 두 번째 Stop: StateStopped 에서 호출되어도 에러가 없어야 하고 다시 Disconnect 해야 한다.
	require.NoError(t, a.Stop(context.Background()), "두 번째 Stop(idempotent) 도 성공해야 한다")
	assert.Equal(t, lifecycle.StateStopped, a.CurrentState())
	assert.Equal(t, 2, fake.disconnectCount(),
		"idempotent Stop 도 Disconnect 를 호출하여 연결 끊김을 보장해야 한다")
}
