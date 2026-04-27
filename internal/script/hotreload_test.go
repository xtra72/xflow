package script

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// NewHotReloadManager 테스트
// ============================================================

func TestNewHotReloadManager(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	hrm := NewHotReloadManager(engine)
	require.NotNil(t, hrm)
}

// ============================================================
// Reload 테스트
// ============================================================

func TestHotReload_Reload_Success(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	// 원본 스크립트 컴파일
	src := ScriptSource{
		Type:    SourceInline,
		Content: "return 1",
		Name:    "reload-test",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	// 실행 확인
	result, err := engine.Execute(ctx, scriptID, nil)
	require.NoError(t, err)
	assert.Equal(t, float64(1), result)

	// 핫 리로드
	hrm := NewHotReloadManager(engine)
	newSrc := ScriptSource{
		Type:    SourceInline,
		Content: "return 2",
		Name:    "reload-test",
	}

	err = hrm.Reload(ctx, scriptID, newSrc)
	require.NoError(t, err)

	// 리로드 후 실행 확인
	result, err = engine.Execute(ctx, scriptID, nil)
	require.NoError(t, err)
	assert.Equal(t, float64(2), result)
}

func TestHotReload_Reload_CompileFail_KeepsExisting(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	// 원본 스크립트 컴파일
	src := ScriptSource{
		Type:    SourceInline,
		Content: "return 1",
		Name:    "compile-fail-test",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	// 잘못된 소스로 리로드 시도
	hrm := NewHotReloadManager(engine)
	badSrc := ScriptSource{
		Type:    SourceInline,
		Content: "this is not valid lua {{{}}}",
		Name:    "compile-fail-test",
	}

	err = hrm.Reload(ctx, scriptID, badSrc)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrScriptCompileFailed)

	// 원본 스크립트가 여전히 작동해야 한다
	result, err := engine.Execute(ctx, scriptID, nil)
	require.NoError(t, err)
	assert.Equal(t, float64(1), result)
}

func TestHotReload_Reload_VersionIncrement(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: "return 1",
		Name:    "version-test",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	hrm := NewHotReloadManager(engine)

	// 첫 번째 리로드
	newSrc := ScriptSource{
		Type:    SourceInline,
		Content: "return 2",
		Name:    "version-test",
	}
	err = hrm.Reload(ctx, scriptID, newSrc)
	require.NoError(t, err)

	ver, err := hrm.Version(scriptID)
	require.NoError(t, err)
	assert.Equal(t, 1, ver)

	// 두 번째 리로드
	newSrc2 := ScriptSource{
		Type:    SourceInline,
		Content: "return 3",
		Name:    "version-test",
	}
	err = hrm.Reload(ctx, scriptID, newSrc2)
	require.NoError(t, err)

	ver, err = hrm.Version(scriptID)
	require.NoError(t, err)
	assert.Equal(t, 2, ver)
}

func TestHotReload_Reload_Callback(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: "return 1",
		Name:    "callback-test",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	hrm := NewHotReloadManager(engine)

	var callbackID string
	var callbackVer int
	var callbackErr error
	hrm.OnReload(func(id string, version int, reloadErr error) {
		callbackID = id
		callbackVer = version
		callbackErr = reloadErr
	})

	// 성공적인 리로드
	newSrc := ScriptSource{
		Type:    SourceInline,
		Content: "return 2",
		Name:    "callback-test",
	}
	err = hrm.Reload(ctx, scriptID, newSrc)
	require.NoError(t, err)

	assert.Equal(t, scriptID, callbackID)
	assert.Equal(t, 1, callbackVer)
	assert.NoError(t, callbackErr)
}

func TestHotReload_Reload_Callback_OnFailure(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: "return 1",
		Name:    "callback-fail-test",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	hrm := NewHotReloadManager(engine)

	var callbackErr error
	hrm.OnReload(func(id string, version int, reloadErr error) {
		callbackErr = reloadErr
	})

	// 실패하는 리로드
	badSrc := ScriptSource{
		Type:    SourceInline,
		Content: "invalid lua {{{",
		Name:    "callback-fail-test",
	}
	err = hrm.Reload(ctx, scriptID, badSrc)
	assert.Error(t, err)
	assert.Error(t, callbackErr)
}

// ============================================================
// Rollback 테스트
// ============================================================

func TestHotReload_Rollback_Success(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	// 원본 스크립트
	src := ScriptSource{
		Type:    SourceInline,
		Content: "return 1",
		Name:    "rollback-test",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	hrm := NewHotReloadManager(engine)

	// 리로드
	newSrc := ScriptSource{
		Type:    SourceInline,
		Content: "return 2",
		Name:    "rollback-test",
	}
	err = hrm.Reload(ctx, scriptID, newSrc)
	require.NoError(t, err)

	// 리로드 후 결과 확인
	result, err := engine.Execute(ctx, scriptID, nil)
	require.NoError(t, err)
	assert.Equal(t, float64(2), result)

	// 롤백
	err = hrm.Rollback(ctx, scriptID)
	require.NoError(t, err)

	// 롤백 후 원본으로 돌아와야 한다
	result, err = engine.Execute(ctx, scriptID, nil)
	require.NoError(t, err)
	assert.Equal(t, float64(1), result)
}

func TestHotReload_Rollback_NoPreviousVersion(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	hrm := NewHotReloadManager(engine)

	// 버전 정보가 없는 스크립트에 대한 롤백
	err := hrm.Rollback(ctx, "nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrHotReloadFailed)
}

func TestHotReload_Rollback_NoInitialVersion(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	// 스크립트 컴파일 (리로드 없이)
	src := ScriptSource{
		Type:    SourceInline,
		Content: "return 1",
		Name:    "no-reload-rollback",
	}
	_, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	hrm := NewHotReloadManager(engine)

	// 한 번도 리로드하지 않은 스크립트에 대한 롤백
	err = hrm.Rollback(ctx, "no-reload-rollback")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrHotReloadFailed)
}

func TestHotReload_Rollback_VersionDecrement(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: "return 1",
		Name:    "ver-dec-test",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	hrm := NewHotReloadManager(engine)

	// 두 번 리로드
	for i := 2; i <= 3; i++ {
		newSrc := ScriptSource{
			Type:    SourceInline,
			Content: "return " + string(rune('0'+i)),
			Name:    "ver-dec-test",
		}
		err = hrm.Reload(ctx, scriptID, newSrc)
		require.NoError(t, err)
	}

	ver, err := hrm.Version(scriptID)
	require.NoError(t, err)
	assert.Equal(t, 2, ver)

	// 롤백
	err = hrm.Rollback(ctx, scriptID)
	require.NoError(t, err)

	ver, err = hrm.Version(scriptID)
	require.NoError(t, err)
	assert.Equal(t, 1, ver)
}

func TestHotReload_Rollback_Callback(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: "return 1",
		Name:    "rollback-cb-test",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	hrm := NewHotReloadManager(engine)

	var callbackCount int
	hrm.OnReload(func(id string, version int, reloadErr error) {
		callbackCount++
	})

	// 리로드
	newSrc := ScriptSource{
		Type:    SourceInline,
		Content: "return 2",
		Name:    "rollback-cb-test",
	}
	err = hrm.Reload(ctx, scriptID, newSrc)
	require.NoError(t, err)
	assert.Equal(t, 1, callbackCount)

	// 롤백 시에도 콜백 호출
	err = hrm.Rollback(ctx, scriptID)
	require.NoError(t, err)
	assert.Equal(t, 2, callbackCount)
}

// ============================================================
// Version 테스트
// ============================================================

func TestHotReload_Version_NotFound(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	hrm := NewHotReloadManager(engine)

	_, err := hrm.Version("nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrScriptNotFound)
}

// ============================================================
// 동시성 테스트
// ============================================================

func TestHotReload_ConcurrentReload(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(4))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: "return 0",
		Name:    "concurrent-reload",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	hrm := NewHotReloadManager(engine)

	var callbackCount atomic.Int64
	hrm.OnReload(func(id string, version int, reloadErr error) {
		callbackCount.Add(1)
	})

	const goroutines = 10
	var wg sync.WaitGroup

	for i := 1; i <= goroutines; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			newSrc := ScriptSource{
				Type:    SourceInline,
				Content: "return " + string(rune('0'+val%10)),
				Name:    "concurrent-reload",
			}
			hrm.Reload(ctx, scriptID, newSrc) //nolint:errcheck
		}(i)
	}

	wg.Wait()

	// 모든 리로드에 대해 콜백이 호출되어야 한다
	assert.Equal(t, int64(goroutines), callbackCount.Load())

	// 스크립트는 여전히 실행 가능해야 한다
	_, err = engine.Execute(ctx, scriptID, nil)
	assert.NoError(t, err)
}
