package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/lifecycle"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	require.NotNil(t, m)
	assert.Empty(t, m.List())
}

func TestManager_Create(t *testing.T) {
	m := NewManager()

	cfg := AgentConfig{ID: "a1", Name: "Agent 1", Type: "custom"}
	agent, err := m.Create(cfg)
	require.NoError(t, err)
	require.NotNil(t, agent)
	assert.Equal(t, "a1", agent.ID())
	assert.Equal(t, lifecycle.StateRunning, agent.(*BaseAgent).CurrentState())
}

func TestManager_Create_DuplicateID(t *testing.T) {
	m := NewManager()

	cfg := AgentConfig{ID: "a1", Name: "Agent 1"}
	_, err := m.Create(cfg)
	require.NoError(t, err)

	_, err = m.Create(cfg)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentAlreadyExists)
}

func TestManager_Create_InvalidConfig(t *testing.T) {
	m := NewManager()

	cfg := AgentConfig{ID: "", Name: ""}
	_, err := m.Create(cfg)
	assert.Error(t, err)
}

func TestManager_Start(t *testing.T) {
	m := NewManager()

	cfg := AgentConfig{ID: "a1", Name: "Agent 1"}
	_, err := m.Create(cfg)
	require.NoError(t, err)

	// Create already leaves it Running, so Start should be no-op.
	err = m.Start(context.Background(), "a1")
	assert.NoError(t, err)
}

func TestManager_Start_NotFound(t *testing.T) {
	m := NewManager()

	err := m.Start(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestManager_Stop(t *testing.T) {
	m := NewManager()

	cfg := AgentConfig{ID: "a1", Name: "Agent 1"}
	_, err := m.Create(cfg)
	require.NoError(t, err)

	err = m.Stop(context.Background(), "a1")
	require.NoError(t, err)
}

func TestManager_Stop_NotFound(t *testing.T) {
	m := NewManager()

	err := m.Stop(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestManager_Restart(t *testing.T) {
	m := NewManager()

	cfg := AgentConfig{ID: "a1", Name: "Agent 1"}
	_, err := m.Create(cfg)
	require.NoError(t, err)

	err = m.Restart(context.Background(), "a1")
	require.NoError(t, err)

	// Agent should be running after restart.
	agent, err := m.Get("a1")
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, agent.(*BaseAgent).CurrentState())
}

func TestManager_Restart_NotFound(t *testing.T) {
	m := NewManager()

	err := m.Restart(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestManager_Delete(t *testing.T) {
	m := NewManager()

	cfg := AgentConfig{ID: "a1", Name: "Agent 1"}
	_, err := m.Create(cfg)
	require.NoError(t, err)

	err = m.Delete("a1")
	require.NoError(t, err)

	// Agent should no longer exist.
	_, err = m.Get("a1")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestManager_Delete_NotFound(t *testing.T) {
	m := NewManager()

	err := m.Delete("nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestManager_Delete_StopsRunningAgent(t *testing.T) {
	m := NewManager()

	cfg := AgentConfig{ID: "a1", Name: "Agent 1"}
	_, err := m.Create(cfg)
	require.NoError(t, err)

	// Delete should stop the running agent first.
	err = m.Delete("a1")
	require.NoError(t, err)
	assert.Empty(t, m.List())
}

func TestManager_Get(t *testing.T) {
	m := NewManager()

	cfg := AgentConfig{ID: "a1", Name: "Agent 1"}
	_, err := m.Create(cfg)
	require.NoError(t, err)

	agent, err := m.Get("a1")
	require.NoError(t, err)
	assert.Equal(t, "a1", agent.ID())
}

func TestManager_Get_NotFound(t *testing.T) {
	m := NewManager()

	_, err := m.Get("nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestManager_List(t *testing.T) {
	m := NewManager()

	cfg1 := AgentConfig{ID: "a1", Name: "Agent 1"}
	cfg2 := AgentConfig{ID: "a2", Name: "Agent 2"}

	_, err := m.Create(cfg1)
	require.NoError(t, err)
	_, err = m.Create(cfg2)
	require.NoError(t, err)

	list := m.List()
	assert.Len(t, list, 2)
}

func TestManager_Shutdown(t *testing.T) {
	m := NewManager()

	cfg1 := AgentConfig{ID: "a1", Name: "Agent 1"}
	cfg2 := AgentConfig{ID: "a2", Name: "Agent 2"}

	_, err := m.Create(cfg1)
	require.NoError(t, err)
	_, err = m.Create(cfg2)
	require.NoError(t, err)

	err = m.Shutdown(context.Background())
	require.NoError(t, err)

	// 모든 Agent가 Stopped 상태여야 한다.
	for _, a := range m.List() {
		ba := a.(*BaseAgent)
		assert.Equal(t, lifecycle.StateStopped, ba.CurrentState())
	}
}

func TestManager_Shutdown_Empty(t *testing.T) {
	m := NewManager()

	err := m.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestManager_Summary(t *testing.T) {
	m := NewManager()

	cfg1 := AgentConfig{ID: "a1", Name: "Agent 1"}
	cfg2 := AgentConfig{ID: "a2", Name: "Agent 2"}

	_, err := m.Create(cfg1)
	require.NoError(t, err)
	_, err = m.Create(cfg2)
	require.NoError(t, err)

	// Stop one agent
	require.NoError(t, m.Stop(context.Background(), "a2"))

	summary := m.Summary()

	assert.Equal(t, 2, summary.TotalAgents)
	assert.Equal(t, 1, summary.RunningAgents)
	assert.Equal(t, 1, summary.StoppedAgents)
	assert.Len(t, summary.Agents, 2)
}

func TestManager_Summary_Empty(t *testing.T) {
	m := NewManager()

	summary := m.Summary()

	assert.Equal(t, 0, summary.TotalAgents)
	assert.Empty(t, summary.Agents)
}

func TestManager_ConcurrentAccess(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup

	// 동시에 여러 Agent를 생성한다.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cfg := AgentConfig{
				ID:   "concurrent-agent",
				Name: "Concurrent Agent",
			}
			_, _ = m.Create(cfg) // 중복 에러 무시
		}(i)
	}

	// 동시에 조회한다.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.List()
			_ = m.Summary()
			_, _ = m.Get("concurrent-agent")
		}()
	}

	wg.Wait()

	// 하나의 Agent만 생성되었는지 확인한다.
	list := m.List()
	assert.Equal(t, 1, len(list))
}

func TestManager_Create_WithRegisteredType(t *testing.T) {
	m := NewManager()

	// TypeRegistry에 팩토리 등록
	m.typeReg.RegisterType("test-type", func(config AgentConfig) (Agent, error) {
		ba := NewBaseAgent()
		if err := ba.Init(config); err != nil {
			return nil, err
		}
		return ba, nil
	})

	cfg := AgentConfig{ID: "a1", Name: "Agent 1", Type: "test-type"}
	agent, err := m.Create(cfg)
	require.NoError(t, err)
	assert.Equal(t, "a1", agent.ID())
}
