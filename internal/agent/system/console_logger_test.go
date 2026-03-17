package system

import (
	"context"
	"os"
	"path/filepath"
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

// === Milestone 2: 신규 테스트 ===

// TestConsoleLoggerAgent_OutputStderr 는 stderr 출력 설정을 검증한다.
func TestConsoleLoggerAgent_OutputStderr(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "test",
		Name: "test",
		Type: "console-logger",
		Transport: agent.TransportConfig{
			Options: map[string]any{"output": "stderr"},
		},
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)
	defer ag.Stop(context.Background())

	cla := ag.(*ConsoleLoggerAgent)
	assert.Equal(t, "stderr", cla.logConfig.Output)
}

// TestConsoleLoggerAgent_OutputFile 은 파일 출력을 검증한다.
func TestConsoleLoggerAgent_OutputFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	cfg := agent.AgentConfig{
		ID:   "test",
		Name: "test",
		Type: "console-logger",
		Transport: agent.TransportConfig{
			Options: map[string]any{"output": path},
		},
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	_, err = ag.Process([]byte(`{"msg":"hello"}`))
	require.NoError(t, err)

	_ = ag.Stop(context.Background())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "hello")
}

// TestConsoleLoggerAgent_FormatJSON 은 JSON 형식 출력을 검증한다.
func TestConsoleLoggerAgent_FormatJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	cfg := agent.AgentConfig{
		ID:   "test",
		Name: "test",
		Type: "console-logger",
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"output": path,
				"format": "json",
			},
		},
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	_, err = ag.Process([]byte(`{"msg":"hello"}`))
	require.NoError(t, err)

	_ = ag.Stop(context.Background())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"msg":"message received"`)
}

// TestConsoleLoggerAgent_RollingFile 은 롤링 파일 설정을 검증한다.
func TestConsoleLoggerAgent_RollingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	cfg := agent.AgentConfig{
		ID:   "test",
		Name: "test",
		Type: "console-logger",
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"output":   path,
				"max_size": 1, // 1MB
			},
		},
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	cla := ag.(*ConsoleLoggerAgent)
	assert.Equal(t, int64(1*1024*1024), cla.logConfig.MaxSize)

	_ = ag.Stop(context.Background())
}

// TestConsoleLoggerAgent_StopClosesFile 은 Stop 시 파일이 닫히는지 검증한다.
func TestConsoleLoggerAgent_StopClosesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	cfg := agent.AgentConfig{
		ID:   "test",
		Name: "test",
		Type: "console-logger",
		Transport: agent.TransportConfig{
			Options: map[string]any{"output": path},
		},
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	err = ag.Stop(context.Background())
	assert.NoError(t, err)
}

// TestConsoleLoggerAgent_ConfigureNewOutput 은 Configure 로 출력 대상 변경을 검증한다.
func TestConsoleLoggerAgent_ConfigureNewOutput(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "test1.log")
	path2 := filepath.Join(dir, "test2.log")

	cfg := agent.AgentConfig{
		ID:   "test",
		Name: "test",
		Type: "console-logger",
		Transport: agent.TransportConfig{
			Options: map[string]any{"output": path1},
		},
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	_, _ = ag.Process([]byte(`{"msg":"first"}`))

	// 새 파일로 재설정
	newCfg := cfg
	newCfg.Transport.Options = map[string]any{"output": path2}
	err = ag.Configure(newCfg)
	require.NoError(t, err)

	_, _ = ag.Process([]byte(`{"msg":"second"}`))

	_ = ag.Stop(context.Background())

	data1, _ := os.ReadFile(path1)
	data2, _ := os.ReadFile(path2)
	assert.Contains(t, string(data1), "first")
	assert.Contains(t, string(data2), "second")
}
