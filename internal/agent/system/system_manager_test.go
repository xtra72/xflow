package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Construction tests
// ---------------------------------------------------------------------------

func TestSystemAgentManager_New(t *testing.T) {
	mgr := NewSystemAgentManager()
	assert.NotNil(t, mgr)
	assert.False(t, mgr.IsInitialized())
	assert.False(t, mgr.IsStarted())
}

func TestSystemAgentManager_Accessors_BeforeInit(t *testing.T) {
	mgr := NewSystemAgentManager()
	assert.Nil(t, mgr.Event())
	assert.Nil(t, mgr.File())
}

// ---------------------------------------------------------------------------
// Initialize tests
// ---------------------------------------------------------------------------

func TestSystemAgentManager_Initialize(t *testing.T) {
	mgr := NewSystemAgentManager()
	sandbox := t.TempDir()
	cfg := SystemConfig{
		EventBufferSize: 128,
		FileSandboxRoot: sandbox,
	}

	err := mgr.Initialize(cfg)
	require.NoError(t, err)
	assert.True(t, mgr.IsInitialized())
	assert.NotNil(t, mgr.Event())
	assert.NotNil(t, mgr.File())

	// Cleanup
	_ = mgr.Stop(context.Background())
}

func TestSystemAgentManager_Initialize_AlreadyInitialized(t *testing.T) {
	mgr := NewSystemAgentManager()
	sandbox := t.TempDir()
	cfg := SystemConfig{
		EventBufferSize: 128,
		FileSandboxRoot: sandbox,
	}

	require.NoError(t, mgr.Initialize(cfg))
	defer func() { _ = mgr.Stop(context.Background()) }()

	err := mgr.Initialize(cfg)
	assert.ErrorIs(t, err, ErrAlreadyInitialized)
}

func TestSystemAgentManager_Initialize_DefaultBufferSize(t *testing.T) {
	mgr := NewSystemAgentManager()
	sandbox := t.TempDir()
	cfg := SystemConfig{
		FileSandboxRoot: sandbox,
	}

	err := mgr.Initialize(cfg)
	require.NoError(t, err)
	assert.NotNil(t, mgr.Event())

	// Cleanup
	_ = mgr.Stop(context.Background())
}

// ---------------------------------------------------------------------------
// Start tests
// ---------------------------------------------------------------------------

func TestSystemAgentManager_Start(t *testing.T) {
	mgr := NewSystemAgentManager()
	sandbox := t.TempDir()
	cfg := SystemConfig{
		EventBufferSize: 128,
		FileSandboxRoot: sandbox,
	}
	require.NoError(t, mgr.Initialize(cfg))

	err := mgr.Start(context.Background())
	require.NoError(t, err)
	assert.True(t, mgr.IsStarted())

	// Cleanup
	require.NoError(t, mgr.Stop(context.Background()))
}

func TestSystemAgentManager_Start_NotInitialized(t *testing.T) {
	mgr := NewSystemAgentManager()
	err := mgr.Start(context.Background())
	assert.ErrorIs(t, err, ErrNotInitialized)
}

// ---------------------------------------------------------------------------
// Stop tests
// ---------------------------------------------------------------------------

func TestSystemAgentManager_Stop(t *testing.T) {
	mgr := NewSystemAgentManager()
	sandbox := t.TempDir()
	cfg := SystemConfig{
		EventBufferSize: 128,
		FileSandboxRoot: sandbox,
	}
	require.NoError(t, mgr.Initialize(cfg))
	require.NoError(t, mgr.Start(context.Background()))

	err := mgr.Stop(context.Background())
	require.NoError(t, err)
	assert.False(t, mgr.IsStarted())
}

func TestSystemAgentManager_Stop_NotInitialized(t *testing.T) {
	mgr := NewSystemAgentManager()
	err := mgr.Stop(context.Background())
	assert.ErrorIs(t, err, ErrNotInitialized)
}

func TestSystemAgentManager_Stop_NotStarted(t *testing.T) {
	mgr := NewSystemAgentManager()
	sandbox := t.TempDir()
	cfg := SystemConfig{
		EventBufferSize: 128,
		FileSandboxRoot: sandbox,
	}
	require.NoError(t, mgr.Initialize(cfg))

	// Stop without Start should still work (stop agents that Init put in Running)
	err := mgr.Stop(context.Background())
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Integration: full lifecycle
// ---------------------------------------------------------------------------

func TestSystemAgentManager_FullLifecycle(t *testing.T) {
	mgr := NewSystemAgentManager()
	sandbox := t.TempDir()
	cfg := SystemConfig{
		EventBufferSize: 64,
		FileSandboxRoot: sandbox,
	}

	// Initialize
	require.NoError(t, mgr.Initialize(cfg))
	assert.True(t, mgr.IsInitialized())

	// Verify agents are accessible
	eventAgent := mgr.Event()
	fileAgent := mgr.File()
	require.NotNil(t, eventAgent)
	require.NotNil(t, fileAgent)

	// Start
	require.NoError(t, mgr.Start(context.Background()))
	assert.True(t, mgr.IsStarted())

	// Use event agent
	done := make(chan struct{})
	_, err := eventAgent.Subscribe("test.lifecycle", func(_ Event) {
		close(done)
	})
	require.NoError(t, err)

	require.NoError(t, eventAgent.Emit("test.lifecycle", "data"))
	<-done

	// Use file agent
	require.NoError(t, fileAgent.WriteFile(sandbox+"/lifecycle.txt", []byte("hello"), 0644))
	data, err := fileAgent.ReadFile(sandbox + "/lifecycle.txt")
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), data)

	// Stop
	require.NoError(t, mgr.Stop(context.Background()))
	assert.False(t, mgr.IsStarted())
}

// ---------------------------------------------------------------------------
// Concurrency test
// ---------------------------------------------------------------------------

func TestSystemAgentManager_Concurrent_Access(t *testing.T) {
	mgr := NewSystemAgentManager()
	sandbox := t.TempDir()
	cfg := SystemConfig{
		EventBufferSize: 128,
		FileSandboxRoot: sandbox,
	}
	require.NoError(t, mgr.Initialize(cfg))
	require.NoError(t, mgr.Start(context.Background()))
	defer func() { _ = mgr.Stop(context.Background()) }()

	done := make(chan struct{})
	const goroutines = 10

	for i := 0; i < goroutines; i++ {
		go func() {
			_ = mgr.IsInitialized()
			_ = mgr.IsStarted()
			_ = mgr.Event()
			_ = mgr.File()
		}()
	}

	go func() {
		for i := 0; i < goroutines; i++ {
			// concurrent done waiter
		}
		close(done)
	}()

	<-done
}
