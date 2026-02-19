package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

func TestNewConsoleLoggerAgent(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "test-console-logger",
		Name: "test-logger",
		Type: "console-logger",
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)

	assert.Equal(t, "test-console-logger", ag.ID())
	assert.Equal(t, "test-logger", ag.Name())
	assert.Equal(t, "console-logger", ag.Type())
}

func TestConsoleLoggerAgent_Process(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "test-console-logger",
		Name: "test-logger",
		Type: "console-logger",
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	// Process 는 데이터를 로깅하고 nil을 반환한다
	result, err := ag.Process([]byte(`{"key":"value"}`))
	assert.NoError(t, err)
	assert.Nil(t, result)

	// Stats 확인
	stats := ag.Stats()
	assert.Equal(t, int64(1), stats.MessagesReceived)
	assert.Equal(t, int64(1), stats.MessagesSent)
}

func TestConsoleLoggerAgent_ProcessMultiple(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "test-console-logger",
		Name: "test-logger",
		Type: "console-logger",
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		_, err := ag.Process([]byte(`{"msg":"hello"}`))
		assert.NoError(t, err)
	}

	stats := ag.Stats()
	assert.Equal(t, int64(5), stats.MessagesReceived)
	assert.Equal(t, int64(5), stats.MessagesSent)
}

func TestConsoleLoggerAgent_Lifecycle(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "test-console-logger",
		Name: "test-logger",
		Type: "console-logger",
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	cla := ag.(*ConsoleLoggerAgent)

	// Init 후 Running 상태
	assert.Equal(t, lifecycle.StateRunning, cla.CurrentState())

	// Start는 Running 상태에서 no-op
	err = ag.Start(context.Background())
	assert.NoError(t, err)

	// Pause
	err = ag.Pause(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, lifecycle.StatePaused, cla.CurrentState())

	// Resume
	err = ag.Resume(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, cla.CurrentState())

	// Stop
	err = ag.Stop(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopped, cla.CurrentState())
}

func TestConsoleLoggerAgent_Health(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "test-console-logger",
		Name: "test-logger",
		Type: "console-logger",
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	// Running 상태에서 Healthy
	health := ag.Health()
	assert.Equal(t, agent.HealthHealthy, health.Status)

	// Stop 후 Unhealthy
	_ = ag.Stop(context.Background())
	health = ag.Health()
	assert.Equal(t, agent.HealthUnhealthy, health.Status)
}

func TestConsoleLoggerAgent_Info(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "test-console-logger",
		Name: "test-logger",
		Type: "console-logger",
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	info := ag.Info()
	assert.Equal(t, "test-console-logger", info.ID)
	assert.Equal(t, "test-logger", info.Name)
	assert.Equal(t, "console-logger", info.Type)
	assert.Equal(t, lifecycle.StateRunning, info.State)
}

func TestConsoleLoggerAgent_Configure(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "test-console-logger",
		Name: "test-logger",
		Type: "console-logger",
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"prefix": "[custom]",
			},
		},
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	cla := ag.(*ConsoleLoggerAgent)
	assert.Equal(t, "[custom]", cla.logConfig.Prefix)

	// Configure로 설정 변경
	newCfg := cfg
	newCfg.Transport.Options = map[string]any{
		"prefix": "[updated]",
	}
	err = ag.Configure(newCfg)
	assert.NoError(t, err)
	assert.Equal(t, "[updated]", cla.logConfig.Prefix)
}

func TestRegisterConsoleLoggerType(t *testing.T) {
	mgr := agent.NewManager()

	err := RegisterConsoleLoggerType(mgr)
	require.NoError(t, err)

	// 등록된 타입으로 에이전트 생성
	ag, err := mgr.Create(agent.AgentConfig{
		ID:   "test-console-logger",
		Name: "test-logger",
		Type: "console-logger",
	})
	require.NoError(t, err)
	assert.Equal(t, "console-logger", ag.Type())
}
