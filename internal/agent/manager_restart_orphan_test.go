package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// registerFakeTransportType 는 재현 테스트에서 Restart 가 새 인스턴스를 생성할 수 있도록
// fake-transport 타입 팩토리를 매니저에 등록한다. fakeTransportAgent 는
// manager_stop_transport_test.go 에 정의되어 있으며 동일 패키지에서 재사용한다.
func registerFakeTransportType(t *testing.T, m *DefaultManager) {
	t.Helper()
	require.NoError(t, m.RegisterType("fake-transport", func(cfg AgentConfig) (Agent, error) {
		a := newFakeTransportAgent(cfg)
		if err := a.Init(cfg); err != nil {
			return nil, err
		}
		return a, nil
	}))
}

// TestManager_Restart_ForcesOldTeardownWhenStoppedButTransportConnected 는
// orphan(고아) 연결 재현 테스트(RED)이다.
//
// 증상: Restart 는 old.Info().State == StateStopped 이면 old.Stop() 을 건너뛰고
// 새 인스턴스만 생성/시작했다. 그러나 flapping 중 Paho 가 Stop 이후 재연결에 성공하면
// 라이프사이클은 Stopped 로 남았지만 트랜스포트는 살아있는 desync 가 발생한다. 이때
// old.Stop() 을 건너뛰면 old 인스턴스의 Paho 클라이언트가 레지스트리에서 사라진 채
// (registry.Unregister) 백그라운드 auto-reconnect 로 영원히 재연결하는 orphan 이 된다.
//
// 수정 전: State==Stopped 라서 old.Stop() 미호출 → old 트랜스포트 살아있음 → 실패(RED).
// 수정 후: Stop() 이 idempotent 하므로 old.Stop() 을 무조건 호출하여 old Paho 클라이언트를
// 강제 disconnect 한 뒤 새 인스턴스를 만든다.
func TestManager_Restart_ForcesOldTeardownWhenStoppedButTransportConnected(t *testing.T) {
	m := NewManager()
	registerFakeTransportType(t, m)

	created, err := m.Create(AgentConfig{ID: "rt1", Name: "rt", Type: "fake-transport"})
	require.NoError(t, err)
	oldAgent := created.(*fakeTransportAgent)

	// desync 재현: 라이프사이클은 Stopped 이지만 Paho 트랜스포트는 재연결되어 살아있음.
	require.NoError(t, oldAgent.TransitionTo(lifecycle.StateStopping))
	require.NoError(t, oldAgent.TransitionTo(lifecycle.StateStopped))
	oldAgent.transportUp.Store(true) // Paho 가 Stop 이후 재연결에 성공한 상황

	require.Equal(t, lifecycle.StateStopped, oldAgent.CurrentState())
	require.True(t, oldAgent.TransportConnected())
	require.Equal(t, int64(0), oldAgent.stopCallCount.Load())

	// Restart: old 인스턴스는 트랜스포트가 살아있으므로 반드시 teardown(old.Stop) 되어야 한다.
	require.NoError(t, m.Restart(context.Background(), "rt1"))

	// 핵심 단언: old 인스턴스의 Stop 이 호출되어 orphan Paho 연결이 정리되어야 한다.
	assert.GreaterOrEqual(t, oldAgent.stopCallCount.Load(), int64(1),
		"State==Stopped 이어도 트랜스포트가 연결되어 있으면 Restart 는 old.Stop 을 호출해 orphan 을 막아야 한다")
	assert.False(t, oldAgent.TransportConnected(),
		"Restart 후 old 인스턴스의 트랜스포트는 강제로 끊겨(orphan 제거) 있어야 한다")

	// 새 인스턴스가 레지스트리에 자리잡아야 한다 (old 와 다른 포인터).
	current, err := m.Get("rt1")
	require.NoError(t, err)
	assert.NotSame(t, oldAgent, current,
		"Restart 후 매니저는 새 인스턴스를 추적해야 한다")
}

// TestManager_Restart_GenuinelyStoppedStillRestarts 는 트랜스포트도 끊긴 진짜 Stopped
// 상태에서도 Restart 가 정상적으로 새 인스턴스를 생성/시작함을 검증한다.
// (Stop 호출 여부는 관여하지 않는다 — idempotent 이므로 호출되어도 무해하다.)
func TestManager_Restart_GenuinelyStoppedStillRestarts(t *testing.T) {
	m := NewManager()
	registerFakeTransportType(t, m)

	created, err := m.Create(AgentConfig{ID: "rt2", Name: "rt", Type: "fake-transport"})
	require.NoError(t, err)
	oldAgent := created.(*fakeTransportAgent)

	// 진짜 정지: 라이프사이클 Stopped + 트랜스포트도 끊김.
	require.NoError(t, oldAgent.TransitionTo(lifecycle.StateStopping))
	require.NoError(t, oldAgent.TransitionTo(lifecycle.StateStopped))
	oldAgent.transportUp.Store(false)

	require.NoError(t, m.Restart(context.Background(), "rt2"))

	current, err := m.Get("rt2")
	require.NoError(t, err)
	assert.NotSame(t, oldAgent, current,
		"진짜 정지 상태에서도 Restart 는 새 인스턴스를 생성/시작해야 한다")
	assert.Equal(t, lifecycle.StateRunning, current.Info().State,
		"새 인스턴스는 Start 후 Running 상태여야 한다")
}

// TestManager_Restart_RunningAgentStillTornDown 는 회귀 방지: Running 상태의 에이전트를
// Restart 할 때도 old.Stop 이 호출되어 기존 트랜스포트가 정리됨을 확인한다.
func TestManager_Restart_RunningAgentStillTornDown(t *testing.T) {
	m := NewManager()
	registerFakeTransportType(t, m)

	created, err := m.Create(AgentConfig{ID: "rt3", Name: "rt", Type: "fake-transport"})
	require.NoError(t, err)
	oldAgent := created.(*fakeTransportAgent)
	oldAgent.transportUp.Store(true) // Running + 연결됨

	require.Equal(t, lifecycle.StateRunning, oldAgent.CurrentState())

	require.NoError(t, m.Restart(context.Background(), "rt3"))

	assert.GreaterOrEqual(t, oldAgent.stopCallCount.Load(), int64(1),
		"Running 에이전트 Restart 시에도 old.Stop 으로 기존 트랜스포트를 정리해야 한다")
	assert.False(t, oldAgent.TransportConnected())
}
