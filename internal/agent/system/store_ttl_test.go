package system

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTTLManager_BackgroundScanDeletesExpired 는 백그라운드 스캔이 만료된 키를 삭제하는지 검증한다.
func TestTTLManager_BackgroundScanDeletesExpired(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 0, 0)

	// 짧은 TTL로 키 저장
	err := store.SetWithTTL(ctx, "expire-1", "v1", 20*time.Millisecond)
	require.NoError(t, err)
	err = store.SetWithTTL(ctx, "expire-2", "v2", 20*time.Millisecond)
	require.NoError(t, err)
	err = store.Set(ctx, "alive", "v3")
	require.NoError(t, err)

	// 빠른 스캔 주기로 TTL 매니저 시작
	mgr := newTTLManager(store, 10*time.Millisecond)
	mgr.Start()
	defer mgr.Stop()

	// 만료 대기 + 스캔 주기 대기
	time.Sleep(80 * time.Millisecond)

	// 만료된 키가 삭제되었는지 확인
	has1, _ := store.Has(ctx, "expire-1")
	has2, _ := store.Has(ctx, "expire-2")
	hasAlive, _ := store.Has(ctx, "alive")

	assert.False(t, has1, "만료된 키 expire-1은 삭제되어야 한다")
	assert.False(t, has2, "만료된 키 expire-2는 삭제되어야 한다")
	assert.True(t, hasAlive, "만료되지 않은 키 alive는 유지되어야 한다")
}

// TestTTLManager_PauseAndResume 은 Pause가 스캔을 중지하고 Resume이 재개하는지 검증한다.
func TestTTLManager_PauseAndResume(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 0, 0)

	// Pause를 먼저 설정하여 시작 직후부터 스캔이 비활성화되도록 한다
	mgr := newTTLManager(store, 10*time.Millisecond)
	mgr.Pause() // Start 전에 Pause (atomic.Bool이므로 즉시 적용)
	mgr.Start()
	defer mgr.Stop()

	// Pause 상태에서 만료 키를 추가
	err := store.SetWithTTL(ctx, "paused-key", "val", 5*time.Millisecond)
	require.NoError(t, err)
	time.Sleep(60 * time.Millisecond) // 키가 만료될 시간 + 충분한 스캔 주기

	// Pause 상태이므로 백그라운드 스캔이 키를 삭제하지 않았어야 한다
	// (lazy expiration인 Get/Has는 삭제할 수 있으므로 직접 sync.Map을 확인)
	_, loaded := store.data.Load("paused-key")
	assert.True(t, loaded, "Pause 상태에서는 백그라운드 스캔이 키를 삭제하지 않아야 한다")

	// Resume 후 스캔이 재개되어야 한다
	mgr.Resume()
	time.Sleep(60 * time.Millisecond)

	_, loaded = store.data.Load("paused-key")
	assert.False(t, loaded, "Resume 후 만료된 키가 삭제되어야 한다")
}

// TestTTLManager_Stop 은 Stop이 고루틴을 종료하는지 검증한다.
func TestTTLManager_Stop(t *testing.T) {
	store := NewVolatileStore(MaxKeyLength, 0, 0)
	mgr := newTTLManager(store, 10*time.Millisecond)

	mgr.Start()
	assert.True(t, mgr.isRunning(), "Start 후 running 상태여야 한다")

	mgr.Stop()
	time.Sleep(30 * time.Millisecond) // 고루틴 종료 대기

	assert.False(t, mgr.isRunning(), "Stop 후 running 상태가 아니어야 한다")
}

// TestTTLManager_StopIdempotent 은 Stop을 여러 번 호출해도 안전한지 검증한다.
func TestTTLManager_StopIdempotent(t *testing.T) {
	store := NewVolatileStore(MaxKeyLength, 0, 0)
	mgr := newTTLManager(store, 10*time.Millisecond)

	mgr.Start()
	mgr.Stop()
	mgr.Stop() // 두 번째 Stop은 패닉 없이 동작해야 한다

	assert.False(t, mgr.isRunning())
}

// TestTTLManager_SetInterval 은 스캔 주기 변경을 검증한다.
func TestTTLManager_SetInterval(t *testing.T) {
	store := NewVolatileStore(MaxKeyLength, 0, 0)
	mgr := newTTLManager(store, 1*time.Second)

	mgr.Start()
	defer mgr.Stop()

	// 주기를 매우 짧게 변경
	mgr.SetInterval(5 * time.Millisecond)

	// 짧은 TTL 키 추가
	ctx := context.Background()
	_ = store.SetWithTTL(ctx, "fast-expire", "val", 10*time.Millisecond)

	// 빠른 주기로 스캔이 돌아 키가 삭제되어야 한다
	time.Sleep(80 * time.Millisecond)

	_, loaded := store.data.Load("fast-expire")
	assert.False(t, loaded, "SetInterval로 변경된 짧은 주기로 만료 키가 삭제되어야 한다")
}

// TestTTLManager_ScanReturnsCount 는 scan()이 삭제된 키 개수를 반환하는지 검증한다.
func TestTTLManager_ScanReturnsCount(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 0, 0)

	_ = store.SetWithTTL(ctx, "exp1", "v1", 1*time.Millisecond)
	_ = store.SetWithTTL(ctx, "exp2", "v2", 1*time.Millisecond)
	_ = store.Set(ctx, "alive", "v3")

	time.Sleep(10 * time.Millisecond)

	mgr := newTTLManager(store, time.Second)
	deleted := mgr.scan()

	assert.Equal(t, 2, deleted, "만료된 2개 키가 삭제되어야 한다")

	// alive 키는 남아있어야 한다
	has, _ := store.Has(ctx, "alive")
	assert.True(t, has)
}

// TestTTLManager_ScanNoExpired 는 만료된 키가 없을 때 scan()이 0을 반환하는지 검증한다.
func TestTTLManager_ScanNoExpired(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 0, 0)

	_ = store.Set(ctx, "key1", "v1")
	_ = store.Set(ctx, "key2", "v2")

	mgr := newTTLManager(store, time.Second)
	deleted := mgr.scan()

	assert.Equal(t, 0, deleted, "만료된 키가 없으면 0을 반환해야 한다")
}

// TestTTLManager_ConcurrentStartStop 은 동시 Start/Stop 호출이 안전한지 검증한다.
func TestTTLManager_ConcurrentStartStop(t *testing.T) {
	store := NewVolatileStore(MaxKeyLength, 0, 0)
	mgr := newTTLManager(store, 10*time.Millisecond)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		mgr.Start()
	}()

	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond)
		mgr.Stop()
	}()

	wg.Wait()
}
