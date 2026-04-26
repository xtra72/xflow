package system

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/lifecycle"
)

func TestStoreAgent_NewStoreAgent_StateCreated(t *testing.T) {
	// Arrange & Act: 기본 옵션으로 StoreAgent 생성
	agent := NewStoreAgent()

	// Assert: 초기 상태는 Created
	assert.Equal(t, lifecycle.StateCreated, agent.State())
}

func TestStoreAgent_NewStoreAgent_WithOptions(t *testing.T) {
	// Arrange & Act: 옵션을 지정하여 StoreAgent 생성
	agent := NewStoreAgent(
		WithBackend("volatile"),
		WithDefaultTTL(5*time.Minute),
		WithScanInterval(2*time.Second),
	)

	// Assert: 설정이 올바르게 적용됨
	cfg := agent.GetConfig()
	assert.Equal(t, "volatile", cfg["backend"])
	assert.Equal(t, 5*time.Minute, cfg["default_ttl"])
	assert.Equal(t, 2*time.Second, cfg["scan_interval"])
}

func TestStoreAgent_Init_TransitionsToRunning(t *testing.T) {
	// Arrange
	agent := NewStoreAgent()
	ctx := context.Background()

	// Act: Init 호출
	err := agent.Init(ctx)

	// Assert: Running 상태로 전이
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, agent.State())
}

func TestStoreAgent_Init_CreatesStoreAndTTLManager(t *testing.T) {
	// Arrange
	agent := NewStoreAgent()
	ctx := context.Background()

	// Act: Init 호출
	err := agent.Init(ctx)
	require.NoError(t, err)

	// Assert: 내부 스토어와 TTL 매니저가 생성됨
	assert.NotNil(t, agent.store)
	assert.NotNil(t, agent.ttlMgr)
	assert.True(t, agent.ttlMgr.isRunning())
}

func TestStoreAgent_Pause_TransitionsToPaused(t *testing.T) {
	// Arrange
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	// Act: Pause 호출
	err := agent.Pause(ctx)

	// Assert: Paused 상태로 전이
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StatePaused, agent.State())
}

func TestStoreAgent_Pause_SetReturnsErrStorePaused(t *testing.T) {
	// Arrange
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	// ForNamespace를 통해 Store 획득
	store := agent.ForNamespace("test")

	// Pause
	require.NoError(t, agent.Pause(ctx))

	// Act: 쓰기 연산 시도
	err := store.Set(ctx, "key", "value")

	// Assert: ErrStorePaused 반환
	assert.ErrorIs(t, err, ErrStorePaused)
}

func TestStoreAgent_Pause_GetStillWorks(t *testing.T) {
	// Arrange: 데이터를 미리 저장
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	store := agent.ForNamespace("test")
	require.NoError(t, store.Set(ctx, "key", "value"))

	// Pause
	require.NoError(t, agent.Pause(ctx))

	// Act: 읽기 연산 시도
	entry, err := store.Get(ctx, "key")

	// Assert: 읽기는 Pause 상태에서도 가능
	require.NoError(t, err)
	assert.Equal(t, "value", entry.Value)
}

func TestStoreAgent_Resume_TransitionsToRunning(t *testing.T) {
	// Arrange
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))
	require.NoError(t, agent.Pause(ctx))

	// Act: Resume 호출
	err := agent.Resume(ctx)

	// Assert: Running 상태로 전이
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, agent.State())
}

func TestStoreAgent_Resume_SetWorksAgain(t *testing.T) {
	// Arrange
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	store := agent.ForNamespace("test")

	require.NoError(t, agent.Pause(ctx))
	require.NoError(t, agent.Resume(ctx))

	// Act: 다시 쓰기 가능
	err := store.Set(ctx, "key", "value")

	// Assert
	require.NoError(t, err)
}

func TestStoreAgent_Stop_TransitionsToStopped(t *testing.T) {
	// Arrange
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	// Act: Stop 호출
	err := agent.Stop(ctx)

	// Assert: Stopped 상태로 전이
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopped, agent.State())
}

func TestStoreAgent_Stop_GetReturnsErrStoreClosed(t *testing.T) {
	// Arrange
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	store := agent.ForNamespace("test")
	require.NoError(t, store.Set(ctx, "key", "value"))

	// Stop
	require.NoError(t, agent.Stop(ctx))

	// Act: 읽기 연산 시도
	_, err := store.Get(ctx, "key")

	// Assert: ErrStoreClosed 반환
	assert.ErrorIs(t, err, ErrStoreClosed)
}

func TestStoreAgent_Stop_TTLManagerStopped(t *testing.T) {
	// Arrange
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	// Act: Stop 호출
	require.NoError(t, agent.Stop(ctx))

	// Assert: TTL 매니저가 정지됨
	assert.False(t, agent.ttlMgr.isRunning())
}

func TestStoreAgent_Configure_Running_ChangeScanInterval(t *testing.T) {
	// Arrange
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	// Act: Running 상태에서 설정 변경
	err := agent.Configure(ctx, map[string]any{
		"ttl_scan_interval": "5s",
	})

	// Assert
	require.NoError(t, err)
	cfg := agent.GetConfig()
	assert.Equal(t, 5*time.Second, cfg["scan_interval"])

	// 정리
	require.NoError(t, agent.Stop(ctx))
}

func TestStoreAgent_Configure_Created_ReturnsError(t *testing.T) {
	// Arrange: Init 호출 전 상태
	agent := NewStoreAgent()
	ctx := context.Background()

	// Act: Created 상태에서 Configure 호출
	err := agent.Configure(ctx, map[string]any{
		"ttl_scan_interval": "5s",
	})

	// Assert: 에러 반환 (Running 또는 Paused가 아님)
	assert.Error(t, err)
}

func TestStoreAgent_GetConfig(t *testing.T) {
	// Arrange
	agent := NewStoreAgent(
		WithBackend("volatile"),
		WithDefaultTTL(10*time.Minute),
		WithScanInterval(3*time.Second),
		WithMaxKeyLength(256),
	)

	// Act
	cfg := agent.GetConfig()

	// Assert
	assert.Equal(t, "volatile", cfg["backend"])
	assert.Equal(t, 10*time.Minute, cfg["default_ttl"])
	assert.Equal(t, 3*time.Second, cfg["scan_interval"])
	assert.Equal(t, 256, cfg["max_key_length"])
}

func TestStoreAgent_HealthCheck_Running_Healthy(t *testing.T) {
	// Arrange
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	// Act
	status := agent.HealthCheck(ctx)

	// Assert
	assert.True(t, status.Healthy)
	assert.Equal(t, "volatile", status.Details["backend"])
	assert.Equal(t, string(lifecycle.StateRunning), status.Details["state"])

	// 정리
	require.NoError(t, agent.Stop(ctx))
}

func TestStoreAgent_HealthCheck_Stopped_NotHealthy(t *testing.T) {
	// Arrange
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))
	require.NoError(t, agent.Stop(ctx))

	// Act
	status := agent.HealthCheck(ctx)

	// Assert
	assert.False(t, status.Healthy)
}

func TestStoreAgent_ForNamespace_ReturnsNamespacedStore(t *testing.T) {
	// Arrange
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	// Act: 네임스페이스 스토어 생성
	store := agent.ForNamespace("my-flow")

	// Assert: 정상 동작
	require.NoError(t, store.Set(ctx, "key", "val"))
	entry, err := store.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, "val", entry.Value)
	assert.Equal(t, "my-flow", entry.Namespace)

	// 정리
	require.NoError(t, agent.Stop(ctx))
}

func TestStoreAgent_ForNamespace_Paused_WritesFail_ReadsWork(t *testing.T) {
	// Arrange
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	store := agent.ForNamespace("test")
	require.NoError(t, store.Set(ctx, "pre-pause", "data"))

	// Pause
	require.NoError(t, agent.Pause(ctx))

	// Assert: 쓰기 실패
	assert.ErrorIs(t, store.Set(ctx, "new-key", "val"), ErrStorePaused)
	assert.ErrorIs(t, store.SetWithTTL(ctx, "new-key", "val", time.Minute), ErrStorePaused)
	assert.ErrorIs(t, store.Delete(ctx, "pre-pause"), ErrStorePaused)
	assert.ErrorIs(t, store.Clear(ctx), ErrStorePaused)

	// Assert: 읽기 성공
	entry, err := store.Get(ctx, "pre-pause")
	require.NoError(t, err)
	assert.Equal(t, "data", entry.Value)

	exists, err := store.Has(ctx, "pre-pause")
	require.NoError(t, err)
	assert.True(t, exists)

	keys, err := store.Keys(ctx, "*")
	require.NoError(t, err)
	assert.Contains(t, keys, "pre-pause")

	// 정리
	require.NoError(t, agent.Resume(ctx))
	require.NoError(t, agent.Stop(ctx))
}

func TestStoreAgent_ForNamespace_Stopped_AllOpsFail(t *testing.T) {
	// Arrange
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	store := agent.ForNamespace("test")
	require.NoError(t, store.Set(ctx, "key", "val"))

	// Stop
	require.NoError(t, agent.Stop(ctx))

	// Assert: 모든 연산 실패
	_, err := store.Get(ctx, "key")
	assert.ErrorIs(t, err, ErrStoreClosed)

	assert.ErrorIs(t, store.Set(ctx, "key", "val"), ErrStoreClosed)
	assert.ErrorIs(t, store.SetWithTTL(ctx, "key", "val", time.Minute), ErrStoreClosed)
	assert.ErrorIs(t, store.Delete(ctx, "key"), ErrStoreClosed)

	_, err = store.Has(ctx, "key")
	assert.ErrorIs(t, err, ErrStoreClosed)

	_, err = store.Keys(ctx, "*")
	assert.ErrorIs(t, err, ErrStoreClosed)

	assert.ErrorIs(t, store.Clear(ctx), ErrStoreClosed)
}

func TestStoreAgent_DoubleInit_ReturnsError(t *testing.T) {
	// Arrange
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	// Act: 두 번째 Init 호출
	err := agent.Init(ctx)

	// Assert: 에러 반환 (Running에서 Initializing으로의 전이는 허용되지 않음)
	assert.Error(t, err)

	// 정리
	require.NoError(t, agent.Stop(ctx))
}

func TestStoreAgent_FullLifecycle(t *testing.T) {
	// Arrange
	agent := NewStoreAgent()
	ctx := context.Background()

	// Created -> Init(Running)
	assert.Equal(t, lifecycle.StateCreated, agent.State())
	require.NoError(t, agent.Init(ctx))
	assert.Equal(t, lifecycle.StateRunning, agent.State())

	// Running -> Pause
	require.NoError(t, agent.Pause(ctx))
	assert.Equal(t, lifecycle.StatePaused, agent.State())

	// Pause -> Resume(Running)
	require.NoError(t, agent.Resume(ctx))
	assert.Equal(t, lifecycle.StateRunning, agent.State())

	// Running -> Stop(Stopped)
	require.NoError(t, agent.Stop(ctx))
	assert.Equal(t, lifecycle.StateStopped, agent.State())
}

func TestStoreAgent_Configure_DefaultTTL(t *testing.T) {
	// Arrange
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	// Act: default_ttl 변경
	err := agent.Configure(ctx, map[string]any{
		"default_ttl": "10m",
	})

	// Assert
	require.NoError(t, err)
	cfg := agent.GetConfig()
	assert.Equal(t, 10*time.Minute, cfg["default_ttl"])

	// 정리
	require.NoError(t, agent.Stop(ctx))
}

func TestStoreAgent_Start_AlreadyRunning_NoError(t *testing.T) {
	// Arrange: Init으로 이미 Running 상태
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	// Act: Start 호출 (이미 Running)
	err := agent.Start(ctx)

	// Assert: 에러 없음 (no-op)
	assert.NoError(t, err)

	// 정리
	require.NoError(t, agent.Stop(ctx))
}

// @spec SPEC-STORE-003
// TestAgentStore_ClearHistory_PausedRejected 는 ClearHistory 가 쓰기 연산으로 분류되어
// Paused 상태에서 ErrStorePaused 를 반환하는지 검증한다 (Delete 와 일관된 정책).
func TestAgentStore_ClearHistory_PausedRejected(t *testing.T) {
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))
	defer func() { _ = agent.Stop(ctx) }()

	store := agent.ForNamespace("default")
	require.NoError(t, store.Set(ctx, "k", 1))
	require.NoError(t, store.Set(ctx, "k", 2)) // 히스토리 1개 생성

	require.NoError(t, agent.Pause(ctx))

	err := store.ClearHistory(ctx, "k")
	assert.ErrorIs(t, err, ErrStorePaused, "Paused 상태에서 ClearHistory 는 거부되어야 한다")
}

// @spec SPEC-STORE-003
// TestAgentStore_ClearHistory_ClosedRejected 는 ClearHistory 가 Stopped 상태에서
// ErrStoreClosed 를 반환하는지 검증한다.
func TestAgentStore_ClearHistory_ClosedRejected(t *testing.T) {
	agent := NewStoreAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	store := agent.ForNamespace("default")
	require.NoError(t, store.Set(ctx, "k", 1))

	require.NoError(t, agent.Stop(ctx))

	err := store.ClearHistory(ctx, "k")
	assert.ErrorIs(t, err, ErrStoreClosed, "Stopped 상태에서 ClearHistory 는 거부되어야 한다")
}
