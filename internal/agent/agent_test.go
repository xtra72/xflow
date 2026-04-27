package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// --- Transport interface tests ---

func TestTransport_InterfaceCompliance(t *testing.T) {
	// Transport 인터페이스가 올바르게 정의되어 있는지 확인한다.
	var _ Transport = (*mockTransport)(nil)
}

// --- BaseAgent tests ---

func TestNewBaseAgent(t *testing.T) {
	ba := NewBaseAgent()

	require.NotNil(t, ba)
	assert.Equal(t, lifecycle.StateCreated, ba.CurrentState())
}

func TestBaseAgent_Init(t *testing.T) {
	ba := NewBaseAgent()

	cfg := AgentConfig{
		ID:   "agent-1",
		Name: "Test Agent",
		Type: "custom",
	}

	err := ba.Init(cfg)
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, ba.CurrentState())
	assert.Equal(t, "agent-1", ba.ID())
	assert.Equal(t, "Test Agent", ba.Name())
	assert.Equal(t, "custom", ba.Type())
}

func TestBaseAgent_Init_InvalidConfig(t *testing.T) {
	ba := NewBaseAgent()

	cfg := AgentConfig{
		ID:   "",
		Name: "",
	}

	err := ba.Init(cfg)
	require.Error(t, err)
	// 상태는 Created에 머물러야 한다.
	assert.Equal(t, lifecycle.StateCreated, ba.CurrentState())
}

func TestBaseAgent_Init_AlreadyInitialized(t *testing.T) {
	ba := NewBaseAgent()

	cfg := AgentConfig{ID: "a1", Name: "Agent 1"}
	err := ba.Init(cfg)
	require.NoError(t, err)

	// 두 번째 Init 호출은 실패해야 한다 (이미 Running 상태).
	err = ba.Init(cfg)
	assert.Error(t, err)
}

func TestBaseAgent_Start_AlreadyRunning(t *testing.T) {
	ba := initBaseAgent(t, "a1", "Agent 1")

	// 이미 Running이므로 Start는 no-op이어야 한다.
	err := ba.Start(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, ba.CurrentState())
}

func TestBaseAgent_Stop(t *testing.T) {
	ba := initBaseAgent(t, "a1", "Agent 1")

	err := ba.Stop(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopped, ba.CurrentState())
}

func TestBaseAgent_Stop_AlreadyStopped(t *testing.T) {
	ba := initBaseAgent(t, "a1", "Agent 1")

	err := ba.Stop(context.Background())
	require.NoError(t, err)

	// 두 번째 Stop은 에러를 반환해야 한다.
	err = ba.Stop(context.Background())
	assert.Error(t, err)
}

func TestBaseAgent_Pause(t *testing.T) {
	ba := initBaseAgent(t, "a1", "Agent 1")

	err := ba.Pause(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StatePaused, ba.CurrentState())
}

func TestBaseAgent_Resume(t *testing.T) {
	ba := initBaseAgent(t, "a1", "Agent 1")

	err := ba.Pause(context.Background())
	require.NoError(t, err)

	err = ba.Resume(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, ba.CurrentState())
}

func TestBaseAgent_Resume_NotPaused(t *testing.T) {
	ba := initBaseAgent(t, "a1", "Agent 1")

	// Running 상태에서 Resume은 실패해야 한다.
	err := ba.Resume(context.Background())
	assert.Error(t, err)
}

func TestBaseAgent_Pause_NotRunning(t *testing.T) {
	ba := initBaseAgent(t, "a1", "Agent 1")

	err := ba.Stop(context.Background())
	require.NoError(t, err)

	// Stopped 상태에서 Pause는 실패해야 한다.
	err = ba.Pause(context.Background())
	assert.Error(t, err)
}

func TestBaseAgent_FullLifecycle(t *testing.T) {
	// Init -> Start(no-op) -> Pause -> Resume -> Stop 전체 생명주기
	ba := NewBaseAgent()

	cfg := AgentConfig{ID: "a1", Name: "Agent 1"}
	require.NoError(t, ba.Init(cfg))
	assert.Equal(t, lifecycle.StateRunning, ba.CurrentState())

	require.NoError(t, ba.Start(context.Background()))
	assert.Equal(t, lifecycle.StateRunning, ba.CurrentState())

	require.NoError(t, ba.Pause(context.Background()))
	assert.Equal(t, lifecycle.StatePaused, ba.CurrentState())

	require.NoError(t, ba.Resume(context.Background()))
	assert.Equal(t, lifecycle.StateRunning, ba.CurrentState())

	require.NoError(t, ba.Stop(context.Background()))
	assert.Equal(t, lifecycle.StateStopped, ba.CurrentState())
}

func TestBaseAgent_Health(t *testing.T) {
	ba := initBaseAgent(t, "a1", "Agent 1")

	hs := ba.Health()
	assert.Equal(t, HealthHealthy, hs.Status)
}

func TestBaseAgent_Health_Stopped(t *testing.T) {
	ba := initBaseAgent(t, "a1", "Agent 1")

	require.NoError(t, ba.Stop(context.Background()))

	hs := ba.Health()
	assert.Equal(t, HealthUnhealthy, hs.Status)
}

func TestBaseAgent_Process_NoTransport(t *testing.T) {
	ba := initBaseAgent(t, "a1", "Agent 1")

	_, err := ba.Process([]byte("test"))
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrTransportNotAvailable)
}

func TestBaseAgent_Process_WithTransport(t *testing.T) {
	ba := NewBaseAgent()
	mt := &mockTransport{available: true, readData: []byte("response")}
	ba.transport = mt

	cfg := AgentConfig{ID: "a1", Name: "Agent 1"}
	require.NoError(t, ba.Init(cfg))

	result, err := ba.Process([]byte("input"))
	require.NoError(t, err)
	assert.Equal(t, []byte("input"), mt.lastWritten)
	assert.Equal(t, []byte("response"), result)
}

func TestBaseAgent_Configure(t *testing.T) {
	ba := initBaseAgent(t, "a1", "Agent 1")

	newCfg := AgentConfig{
		ID:          "a1",
		Name:        "Updated Agent",
		MaxRestarts: 5,
	}

	err := ba.Configure(newCfg)
	require.NoError(t, err)
	assert.Equal(t, "Updated Agent", ba.Name())
}

func TestBaseAgent_Configure_TransportTypeChange_WhenRunning(t *testing.T) {
	ba := NewBaseAgent()
	ba.transport = &mockTransport{available: true}

	cfg := AgentConfig{
		ID:   "a1",
		Name: "Agent 1",
		Transport: TransportConfig{
			Type: "serial",
		},
	}
	require.NoError(t, ba.Init(cfg))

	newCfg := AgentConfig{
		ID:   "a1",
		Name: "Agent 1",
		Transport: TransportConfig{
			Type: "tcp",
		},
	}

	err := ba.Configure(newCfg)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrConfigImmutable)
}

func TestBaseAgent_Info(t *testing.T) {
	ba := initBaseAgent(t, "a1", "Agent 1")

	info := ba.Info()

	assert.Equal(t, "a1", info.ID)
	assert.Equal(t, "Agent 1", info.Name)
	assert.Equal(t, lifecycle.StateRunning, info.State)
	assert.False(t, info.CreatedAt.IsZero())
}

func TestBaseAgent_Stats(t *testing.T) {
	ba := initBaseAgent(t, "a1", "Agent 1")

	snap := ba.Stats()

	assert.Equal(t, int64(0), snap.MessagesReceived)
	assert.Equal(t, int64(0), snap.MessagesSent)
}

func TestBaseAgent_IDNameType(t *testing.T) {
	ba := initBaseAgent(t, "test-id", "Test Name")
	ba.config.Type = "custom"

	assert.Equal(t, "test-id", ba.ID())
	assert.Equal(t, "Test Name", ba.Name())
	assert.Equal(t, "custom", ba.Type())
}

func TestBaseAgent_CreatedAt(t *testing.T) {
	before := time.Now()
	ba := NewBaseAgent()
	after := time.Now()

	assert.False(t, ba.createdAt.IsZero())
	assert.True(t, !ba.createdAt.Before(before))
	assert.True(t, !ba.createdAt.After(after))
}

// --- Helpers ---

// initBaseAgent creates and initializes a BaseAgent for testing.
func initBaseAgent(t *testing.T, id, name string) *BaseAgent {
	t.Helper()
	ba := NewBaseAgent()
	cfg := AgentConfig{ID: id, Name: name}
	require.NoError(t, ba.Init(cfg))
	return ba
}

// --- Mock Transport ---

type mockTransport struct {
	available   bool
	opened      bool
	closed      bool
	readData    []byte
	lastWritten []byte
	openErr     error
	closeErr    error
	readErr     error
	writeErr    error
}

func (m *mockTransport) Open(_ TransportConfig) error {
	if m.openErr != nil {
		return m.openErr
	}
	m.opened = true
	return nil
}

func (m *mockTransport) Close() error {
	if m.closeErr != nil {
		return m.closeErr
	}
	m.closed = true
	return nil
}

func (m *mockTransport) Read(buf []byte) (int, error) {
	if m.readErr != nil {
		return 0, m.readErr
	}
	n := copy(buf, m.readData)
	return n, nil
}

func (m *mockTransport) Write(data []byte) (int, error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	m.lastWritten = make([]byte, len(data))
	copy(m.lastWritten, data)
	return len(data), nil
}

func (m *mockTransport) Available() bool {
	return m.available
}
