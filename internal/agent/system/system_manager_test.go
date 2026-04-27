package system

import (
	"context"
	"log/slog"
	"testing"
	"time"

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
	assert.Nil(t, mgr.Logger())
	assert.Nil(t, mgr.Timer())
	assert.Nil(t, mgr.Store())
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
	assert.NotNil(t, mgr.Logger())
	assert.NotNil(t, mgr.Timer())
	assert.NotNil(t, mgr.Store())

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
// SystemConfig field tests
// ---------------------------------------------------------------------------

func TestSystemConfig_NewFields(t *testing.T) {
	cfg := SystemConfig{
		EventBufferSize:  256,
		FileSandboxRoot:  "/tmp/test",
		LogDefaultLevel:  slog.LevelWarn,
		StoreDefaultTTL:  5 * time.Minute,
		TimerMinInterval: 200 * time.Millisecond,
	}

	assert.Equal(t, slog.LevelWarn, cfg.LogDefaultLevel)
	assert.Equal(t, 5*time.Minute, cfg.StoreDefaultTTL)
	assert.Equal(t, 200*time.Millisecond, cfg.TimerMinInterval)
}

func TestSystemConfig_ZeroValues(t *testing.T) {
	cfg := SystemConfig{}

	// Zero value of slog.Level is LevelDebug (0), which is acceptable.
	assert.Equal(t, slog.Level(0), cfg.LogDefaultLevel)
	// Zero value of time.Duration is 0 (unlimited TTL).
	assert.Equal(t, time.Duration(0), cfg.StoreDefaultTTL)
	// Zero value of time.Duration is 0 (no minimum interval).
	assert.Equal(t, time.Duration(0), cfg.TimerMinInterval)
}

func TestSystemConfig_WithNewFields_Initialize(t *testing.T) {
	mgr := NewSystemAgentManager()
	sandbox := t.TempDir()
	cfg := SystemConfig{
		EventBufferSize:  128,
		FileSandboxRoot:  sandbox,
		LogDefaultLevel:  slog.LevelError,
		StoreDefaultTTL:  10 * time.Second,
		TimerMinInterval: 500 * time.Millisecond,
	}

	err := mgr.Initialize(cfg)
	require.NoError(t, err)
	assert.True(t, mgr.IsInitialized())

	// Cleanup
	_ = mgr.Stop(context.Background())
}

// ---------------------------------------------------------------------------
// Sentinel error tests
// ---------------------------------------------------------------------------

func TestSentinelErrors_NewErrors_Exist(t *testing.T) {
	assert.NotNil(t, ErrAgentInitFailed)
	assert.NotNil(t, ErrAgentStartFailed)
	assert.NotNil(t, ErrAgentStopFailed)
}

func TestSentinelErrors_NewErrors_Messages(t *testing.T) {
	assert.Equal(t, "system: agent initialization failed", ErrAgentInitFailed.Error())
	assert.Equal(t, "system: agent start failed", ErrAgentStartFailed.Error())
	assert.Equal(t, "system: agent stop failed", ErrAgentStopFailed.Error())
}

func TestSentinelErrors_AllDistinct(t *testing.T) {
	allErrors := []error{
		ErrAlreadyInitialized,
		ErrNotInitialized,
		ErrAgentTypeUnknown,
		ErrAgentInitFailed,
		ErrAgentStartFailed,
		ErrAgentStopFailed,
	}

	// Verify all errors are distinct from each other
	for i := 0; i < len(allErrors); i++ {
		for j := i + 1; j < len(allErrors); j++ {
			assert.NotEqual(t, allErrors[i], allErrors[j],
				"errors at index %d and %d should be distinct", i, j)
			assert.NotEqual(t, allErrors[i].Error(), allErrors[j].Error(),
				"error messages at index %d and %d should be distinct", i, j)
		}
	}
}

// ---------------------------------------------------------------------------
// Bridge handler access tests
// ---------------------------------------------------------------------------

func TestSystemAgentManager_Bridge_BeforeInit(t *testing.T) {
	mgr := NewSystemAgentManager()
	assert.Nil(t, mgr.LoggerBridge())
	assert.Nil(t, mgr.TimerBridge())
	assert.Nil(t, mgr.StoreBridge())
}

func TestSystemAgentManager_Bridge_AfterInit(t *testing.T) {
	mgr := NewSystemAgentManager()
	sandbox := t.TempDir()
	cfg := SystemConfig{
		EventBufferSize: 128,
		FileSandboxRoot: sandbox,
	}
	require.NoError(t, mgr.Initialize(cfg))
	defer func() { _ = mgr.Stop(context.Background()) }()

	assert.NotNil(t, mgr.LoggerBridge())
	assert.NotNil(t, mgr.TimerBridge())
	assert.NotNil(t, mgr.StoreBridge())
}

func TestSystemAgentManager_Bridge_ReturnsNewInstance(t *testing.T) {
	mgr := NewSystemAgentManager()
	sandbox := t.TempDir()
	cfg := SystemConfig{
		EventBufferSize: 128,
		FileSandboxRoot: sandbox,
	}
	require.NoError(t, mgr.Initialize(cfg))
	defer func() { _ = mgr.Stop(context.Background()) }()

	// Each call should return a new bridge instance (no caching)
	lb1 := mgr.LoggerBridge()
	lb2 := mgr.LoggerBridge()
	assert.NotSame(t, lb1, lb2)

	tb1 := mgr.TimerBridge()
	tb2 := mgr.TimerBridge()
	assert.NotSame(t, tb1, tb2)

	sb1 := mgr.StoreBridge()
	sb2 := mgr.StoreBridge()
	assert.NotSame(t, sb1, sb2)
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

	// Verify all agents are accessible
	loggerAgent := mgr.Logger()
	timerAgent := mgr.Timer()
	storeAgent := mgr.Store()
	require.NotNil(t, loggerAgent)
	require.NotNil(t, timerAgent)
	require.NotNil(t, storeAgent)

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

	// Use store agent via namespace
	ctx := context.Background()
	store := storeAgent.ForNamespace("lifecycle")
	require.NoError(t, store.Set(ctx, "test-key", "test-value"))
	entry, getErr := store.Get(ctx, "test-key")
	require.NoError(t, getErr)
	assert.Equal(t, "test-value", entry.Value)

	// Use logger agent
	require.NoError(t, loggerAgent.WriteLog(ctx, "test", slog.LevelInfo, "lifecycle test message"))

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
			_ = mgr.Logger()
			_ = mgr.Timer()
			_ = mgr.Store()
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
