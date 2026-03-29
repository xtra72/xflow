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
		Type: "logger",
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)

	assert.Equal(t, "test-console-logger", ag.ID())
	assert.Equal(t, "test-logger", ag.Name())
	assert.Equal(t, "logger", ag.Type())
}

func TestConsoleLoggerAgent_Process(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "test-console-logger",
		Name: "test-logger",
		Type: "logger",
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
		Type: "logger",
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
		Type: "logger",
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
		Type: "logger",
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
		Type: "logger",
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	info := ag.Info()
	assert.Equal(t, "test-console-logger", info.ID)
	assert.Equal(t, "test-logger", info.Name)
	assert.Equal(t, "logger", info.Type)
	assert.Equal(t, lifecycle.StateRunning, info.State)
}

func TestConsoleLoggerAgent_Configure(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "test-console-logger",
		Name: "test-logger",
		Type: "logger",
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
		Type: "logger",
	})
	require.NoError(t, err)
	assert.Equal(t, "logger", ag.Type())
}

// === Milestone 2: 신규 테스트 ===

// TestConsoleLoggerAgent_OutputStderr 는 stderr 출력 설정을 검증한다.
func TestConsoleLoggerAgent_OutputStderr(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "test",
		Name: "test",
		Type: "logger",
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
		Type: "logger",
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
		Type: "logger",
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
		Type: "logger",
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
		Type: "logger",
		Transport: agent.TransportConfig{
			Options: map[string]any{"output": path},
		},
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	err = ag.Stop(context.Background())
	assert.NoError(t, err)
}

// === Milestone 3: PublishMessage (MessagePublisher) 테스트 ===

// TestConsoleLoggerAgent_PublishMessage_FileWrite 는 topic 을 파일 경로로 사용하여 데이터를 기록하는지 검증한다.
func TestConsoleLoggerAgent_PublishMessage_FileWrite(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "capture.jsonl")

	cfg := agent.AgentConfig{
		ID:   "test",
		Name: "test",
		Type: "logger",
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	pub := ag.(*ConsoleLoggerAgent)

	// PublishMessage 로 파일에 기록
	err = pub.PublishMessage(filePath, 0, false, []byte(`{"type":"frame","seq":1}`))
	require.NoError(t, err)

	err = pub.PublishMessage(filePath, 0, false, []byte(`{"type":"frame","seq":2}`))
	require.NoError(t, err)

	_ = ag.Stop(context.Background())

	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Contains(t, string(data), `{"type":"frame","seq":1}`)
	assert.Contains(t, string(data), `{"type":"frame","seq":2}`)

	// 통계 확인
	stats := ag.Stats()
	assert.Equal(t, int64(2), stats.MessagesReceived)
	assert.Equal(t, int64(2), stats.MessagesSent)
}

// TestConsoleLoggerAgent_PublishMessage_MultiFile 은 여러 topic(파일)에 동시에 기록하는지 검증한다.
func TestConsoleLoggerAgent_PublishMessage_MultiFile(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "file1.jsonl")
	file2 := filepath.Join(dir, "file2.jsonl")

	cfg := agent.AgentConfig{
		ID:   "test",
		Name: "test",
		Type: "logger",
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	pub := ag.(*ConsoleLoggerAgent)

	err = pub.PublishMessage(file1, 0, false, []byte(`{"file":1}`))
	require.NoError(t, err)

	err = pub.PublishMessage(file2, 0, false, []byte(`{"file":2}`))
	require.NoError(t, err)

	err = pub.PublishMessage(file1, 0, false, []byte(`{"file":1,"seq":2}`))
	require.NoError(t, err)

	_ = ag.Stop(context.Background())

	data1, err := os.ReadFile(file1)
	require.NoError(t, err)
	assert.Contains(t, string(data1), `{"file":1}`)
	assert.Contains(t, string(data1), `{"file":1,"seq":2}`)

	data2, err := os.ReadFile(file2)
	require.NoError(t, err)
	assert.Contains(t, string(data2), `{"file":2}`)
}

// TestConsoleLoggerAgent_PublishMessage_EmptyTopic 은 빈 topic 시 기본 로거로 폴백하는지 검증한다.
func TestConsoleLoggerAgent_PublishMessage_EmptyTopic(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "test",
		Name: "test",
		Type: "logger",
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	pub := ag.(*ConsoleLoggerAgent)

	// 빈 topic → 기본 Process() 와 동일한 로깅 동작
	err = pub.PublishMessage("", 0, false, []byte(`{"msg":"fallback"}`))
	assert.NoError(t, err)

	stats := ag.Stats()
	assert.Equal(t, int64(1), stats.MessagesReceived)
	assert.Equal(t, int64(1), stats.MessagesSent)

	_ = ag.Stop(context.Background())
}

// TestConsoleLoggerAgent_PublishMessage_RollingConfig 은 에이전트의 롤링 설정이 PublishMessage 파일에도 적용되는지 검증한다.
func TestConsoleLoggerAgent_PublishMessage_RollingConfig(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "rolling.jsonl")

	cfg := agent.AgentConfig{
		ID:   "test",
		Name: "test",
		Type: "logger",
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"max_size":    float64(1), // 1MB (JSON round-trip 에서 float64)
				"max_backups": float64(3),
				"max_age":     float64(7),
				"compress":    true,
			},
		},
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	pub := ag.(*ConsoleLoggerAgent)

	// 롤링 writer 가 생성되는지 확인
	err = pub.PublishMessage(filePath, 0, false, []byte(`{"msg":"rolling"}`))
	require.NoError(t, err)

	// fileWriters 에 등록되었는지 확인
	pub.mu.RLock()
	fw, exists := pub.fileWriters[filePath]
	pub.mu.RUnlock()
	assert.True(t, exists)
	assert.NotNil(t, fw)

	_ = ag.Stop(context.Background())

	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Contains(t, string(data), `{"msg":"rolling"}`)
}

// TestConsoleLoggerAgent_StopClosesPublishFiles 은 Stop 시 PublishMessage 파일도 닫히는지 검증한다.
func TestConsoleLoggerAgent_StopClosesPublishFiles(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "f1.jsonl")
	file2 := filepath.Join(dir, "f2.jsonl")

	cfg := agent.AgentConfig{
		ID:   "test",
		Name: "test",
		Type: "logger",
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	pub := ag.(*ConsoleLoggerAgent)

	_ = pub.PublishMessage(file1, 0, false, []byte(`data1`))
	_ = pub.PublishMessage(file2, 0, false, []byte(`data2`))

	err = ag.Stop(context.Background())
	assert.NoError(t, err)

	// fileWriters 가 비어있는지 확인
	pub.mu.RLock()
	assert.Empty(t, pub.fileWriters)
	pub.mu.RUnlock()
}

// TestConsoleLoggerAgent_ConfigureClearsPublishFiles 은 Configure 시 기존 PublishMessage 파일이 정리되는지 검증한다.
func TestConsoleLoggerAgent_ConfigureClearsPublishFiles(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "old.jsonl")

	cfg := agent.AgentConfig{
		ID:   "test",
		Name: "test",
		Type: "logger",
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	pub := ag.(*ConsoleLoggerAgent)
	_ = pub.PublishMessage(filePath, 0, false, []byte(`old data`))

	// Configure 호출 → fileWriters 초기화됨
	err = ag.Configure(cfg)
	assert.NoError(t, err)

	pub.mu.RLock()
	assert.Empty(t, pub.fileWriters)
	pub.mu.RUnlock()

	_ = ag.Stop(context.Background())
}

// TestConsoleLoggerAgent_PublishMessage_SubdirCreation 은 서브디렉터리가 없을 때 자동 생성되는지 검증한다.
func TestConsoleLoggerAgent_PublishMessage_SubdirCreation(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "sub", "dir", "capture.jsonl")

	cfg := agent.AgentConfig{
		ID:   "test",
		Name: "test",
		Type: "logger",
	}

	ag, err := NewConsoleLoggerAgent(cfg)
	require.NoError(t, err)

	pub := ag.(*ConsoleLoggerAgent)

	err = pub.PublishMessage(filePath, 0, false, []byte(`{"msg":"subdir"}`))
	require.NoError(t, err)

	_ = ag.Stop(context.Background())

	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Contains(t, string(data), `{"msg":"subdir"}`)
}

// TestConsoleLoggerAgent_ConfigureNewOutput 은 Configure 로 출력 대상 변경을 검증한다.
func TestConsoleLoggerAgent_ConfigureNewOutput(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "test1.log")
	path2 := filepath.Join(dir, "test2.log")

	cfg := agent.AgentConfig{
		ID:   "test",
		Name: "test",
		Type: "logger",
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
