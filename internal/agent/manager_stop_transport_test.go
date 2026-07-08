package agent

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// fakeTransportAgent 는 TransportChecker 를 구현하는 테스트용 에이전트이다.
// 라이프사이클 상태(State)와 실제 트랜스포트 연결(TransportConnected)을 독립적으로
// 제어할 수 있어, "State==Stopped 이지만 트랜스포트는 여전히 연결됨(Paho 재연결 후)"
// 이라는 desync 상황을 재현한다.
type fakeTransportAgent struct {
	*lifecycle.BaseLifecycle
	cfg           AgentConfig
	transportUp   atomic.Bool // 실제 Paho 연결 여부 (State 와 독립)
	stopCallCount atomic.Int64
}

func newFakeTransportAgent(cfg AgentConfig) *fakeTransportAgent {
	return &fakeTransportAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("fake-transport")),
		cfg:           cfg,
	}
}

func (f *fakeTransportAgent) Init(config AgentConfig) error {
	f.cfg = config
	if err := f.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return f.TransitionTo(lifecycle.StateRunning)
}

func (f *fakeTransportAgent) Start(context.Context) error { return nil }

// Stop 은 호출 횟수를 기록하고, 트랜스포트를 강제로 내려 desync 를 해소한다.
// StateStopped 에서 재호출되어도 에러 없이 동작한다(idempotent).
func (f *fakeTransportAgent) Stop(context.Context) error {
	f.stopCallCount.Add(1)
	f.transportUp.Store(false) // 강제 disconnect
	if f.CurrentState() != lifecycle.StateStopped {
		if err := f.TransitionTo(lifecycle.StateStopping); err != nil {
			return err
		}
		return f.TransitionTo(lifecycle.StateStopped)
	}
	return nil
}

func (f *fakeTransportAgent) Pause(context.Context) error  { return nil }
func (f *fakeTransportAgent) Resume(context.Context) error { return nil }
func (f *fakeTransportAgent) Health() HealthStatus         { return HealthStatus{} }
func (f *fakeTransportAgent) Process([]byte) ([]byte, error) {
	return nil, nil
}
func (f *fakeTransportAgent) Configure(config AgentConfig) error { f.cfg = config; return nil }
func (f *fakeTransportAgent) ID() string                         { return f.cfg.ID }
func (f *fakeTransportAgent) Name() string                       { return f.cfg.Name }
func (f *fakeTransportAgent) Type() string                       { return f.cfg.Type }
func (f *fakeTransportAgent) Info() AgentInfo {
	return AgentInfo{ID: f.cfg.ID, Name: f.cfg.Name, Type: f.cfg.Type, State: f.CurrentState(), Config: f.cfg}
}
func (f *fakeTransportAgent) Stats() StatsSnapshot { return StatsSnapshot{} }

// TransportConnected 는 실제 트랜스포트 연결 여부를 반환한다 (agent.TransportChecker).
func (f *fakeTransportAgent) TransportConnected() bool { return f.transportUp.Load() }

var _ Agent = (*fakeTransportAgent)(nil)
var _ TransportChecker = (*fakeTransportAgent)(nil)

// TestManager_Stop_ForcesDisconnectWhenStoppedButTransportConnected 는 재현 테스트(RED)이다.
//
// 증상: manager.Stop 은 State==StateStopped 이면 early-return 하여 agent.Stop() 을
// 호출하지 않는다. 그런데 Paho 가 이전 Stop 이후 재연결에 성공하면 트랜스포트는 살아있는데
// 라이프사이클은 Stopped 로 남아, UI 에서 Stop 을 눌러도 no-op 이 되어 끊을 수 없게 된다.
//
// 수정 전: manager.Stop 이 State==Stopped 만 보고 early-return → agent.Stop 미호출 → 실패.
// 수정 후: State==Stopped 이면서 TransportConnected()==true 이면 여전히 연결된 것으로 보고
// agent.Stop() 을 호출하여 강제 disconnect 한다.
func TestManager_Stop_ForcesDisconnectWhenStoppedButTransportConnected(t *testing.T) {
	m := NewManager()
	require.NoError(t, m.RegisterType("fake-transport", func(cfg AgentConfig) (Agent, error) {
		a := newFakeTransportAgent(cfg)
		if err := a.Init(cfg); err != nil {
			return nil, err
		}
		return a, nil
	}))

	created, err := m.Create(AgentConfig{ID: "ft1", Name: "ft", Type: "fake-transport"})
	require.NoError(t, err)
	fa := created.(*fakeTransportAgent)

	// desync 재현: 라이프사이클은 Stopped 이지만 Paho 트랜스포트는 재연결되어 살아있음.
	require.NoError(t, fa.TransitionTo(lifecycle.StateStopping))
	require.NoError(t, fa.TransitionTo(lifecycle.StateStopped))
	fa.transportUp.Store(true) // Paho 가 Stop 이후 재연결에 성공한 상황

	require.Equal(t, lifecycle.StateStopped, fa.CurrentState())
	require.True(t, fa.TransportConnected())

	// manager.Stop 은 트랜스포트가 여전히 연결되어 있으므로 agent.Stop 을 호출해야 한다.
	require.NoError(t, m.Stop(context.Background(), "ft1"))

	assert.Equal(t, int64(1), fa.stopCallCount.Load(),
		"State==Stopped 이어도 트랜스포트가 연결되어 있으면 manager 는 agent.Stop 을 호출해야 한다")
	assert.False(t, fa.TransportConnected(),
		"manager.Stop 후 트랜스포트는 강제로 끊겨야 한다")
}

// TestManager_Stop_GenuinelyStoppedIsNoOp 는 트랜스포트도 끊긴 진짜 Stopped 상태에서는
// manager.Stop 이 early-return(no-op) 하여 agent.Stop 을 다시 호출하지 않음을 검증한다.
func TestManager_Stop_GenuinelyStoppedIsNoOp(t *testing.T) {
	m := NewManager()
	require.NoError(t, m.RegisterType("fake-transport", func(cfg AgentConfig) (Agent, error) {
		a := newFakeTransportAgent(cfg)
		if err := a.Init(cfg); err != nil {
			return nil, err
		}
		return a, nil
	}))

	created, err := m.Create(AgentConfig{ID: "ft2", Name: "ft", Type: "fake-transport"})
	require.NoError(t, err)
	fa := created.(*fakeTransportAgent)

	// 진짜 정지: 라이프사이클 Stopped + 트랜스포트도 끊김.
	require.NoError(t, fa.TransitionTo(lifecycle.StateStopping))
	require.NoError(t, fa.TransitionTo(lifecycle.StateStopped))
	fa.transportUp.Store(false)

	require.NoError(t, m.Stop(context.Background(), "ft2"))

	assert.Equal(t, int64(0), fa.stopCallCount.Load(),
		"진짜 정지(트랜스포트도 끊김) 상태에서 manager.Stop 은 no-op 이어야 한다")
}
