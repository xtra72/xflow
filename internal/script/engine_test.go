package script

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// ScriptSource 테스트
// ============================================================

func TestSourceType_Constants(t *testing.T) {
	assert.Equal(t, SourceType(0), SourceInline)
}

func TestScriptSource_Fields(t *testing.T) {
	src := ScriptSource{
		Type:    SourceInline,
		Content: "return 42",
		Name:    "test-script",
	}
	assert.Equal(t, SourceInline, src.Type)
	assert.Equal(t, "return 42", src.Content)
	assert.Equal(t, "test-script", src.Name)
}

// ============================================================
// EngineOption 테스트
// ============================================================

func TestWithPoolSize(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(5))
	require.NotNil(t, engine)
	info := engine.Info()
	assert.Equal(t, 5, info.PoolSize)
}

func TestWithMaxExecutionTime(t *testing.T) {
	engine := NewScriptEngine(WithMaxExecutionTime(10 * time.Second))
	require.NotNil(t, engine)
	info := engine.Info()
	assert.Equal(t, 10*time.Second, info.MaxExecutionTime)
}

func TestWithSandboxConfig(t *testing.T) {
	cfg := SandboxConfig{
		Enabled:         true,
		DisabledModules: []string{"os"},
	}
	engine := NewScriptEngine(WithSandboxConfig(cfg))
	require.NotNil(t, engine)
	info := engine.Info()
	assert.True(t, info.SandboxEnabled)
}

func TestWithMaxVMUses(t *testing.T) {
	engine := NewScriptEngine(WithMaxVMUses(100))
	require.NotNil(t, engine)
	// maxVMUses는 내부 설정이므로 Info에서 직접 확인 불가
	// 동작은 VM 풀 재사용 테스트에서 확인
}

// ============================================================
// Engine Init/Shutdown 테스트
// ============================================================

func TestEngine_InitAndShutdown(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()

	err := engine.Init(ctx)
	require.NoError(t, err)

	info := engine.Info()
	assert.Equal(t, 2, info.PoolSize)
	assert.Equal(t, 2, info.IdleVMs)
	assert.Equal(t, 0, info.ActiveVMs)

	err = engine.Shutdown(ctx)
	require.NoError(t, err)
}

func TestEngine_DoubleInit(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()

	err := engine.Init(ctx)
	require.NoError(t, err)
	defer engine.Shutdown(ctx)

	// 두 번째 Init은 에러 없이 처리되어야 한다
	err = engine.Init(ctx)
	assert.NoError(t, err)
}

func TestEngine_ShutdownWithoutInit(t *testing.T) {
	engine := NewScriptEngine()
	ctx := context.Background()

	// Init 없이 Shutdown은 에러 없이 처리되어야 한다
	err := engine.Shutdown(ctx)
	assert.NoError(t, err)
}

// ============================================================
// Compile 테스트
// ============================================================

func TestEngine_Compile_InlineScript(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: "return 42",
		Name:    "test",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)
	assert.NotEmpty(t, scriptID)
}

func TestEngine_Compile_InvalidScript(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: "this is not valid lua {{{}}}",
		Name:    "invalid",
	}
	_, err := engine.Compile(ctx, src)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrScriptCompileFailed)
}

func TestEngine_Compile_EmptyContent(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: "",
		Name:    "empty",
	}
	_, err := engine.Compile(ctx, src)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidScriptSource)
}

func TestEngine_Compile_ReturnsConsistentID(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: "return 1",
		Name:    "my-script",
	}

	// Name이 같으면 동일한 ID 반환
	id1, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	id2, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	assert.Equal(t, id1, id2)
}

func TestEngine_Compile_CacheStats(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: "return 1",
		Name:    "cached-script",
	}

	_, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	info := engine.Info()
	assert.Equal(t, 1, info.CachedScripts)
}

// ============================================================
// Execute 테스트
// ============================================================

func TestEngine_Execute_SimpleReturn(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: "return 42",
		Name:    "simple-return",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	result, err := engine.Execute(ctx, scriptID, nil)
	require.NoError(t, err)
	assert.Equal(t, float64(42), result)
}

func TestEngine_Execute_StringReturn(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: `return "hello"`,
		Name:    "string-return",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	result, err := engine.Execute(ctx, scriptID, nil)
	require.NoError(t, err)
	assert.Equal(t, "hello", result)
}

func TestEngine_Execute_WithInput(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: `return msg.name`,
		Name:    "with-input",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	input := map[string]any{"name": "world"}
	result, err := engine.Execute(ctx, scriptID, input)
	require.NoError(t, err)
	assert.Equal(t, "world", result)
}

func TestEngine_Execute_NotFound(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	_, err := engine.Execute(ctx, "nonexistent", nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrScriptNotFound)
}

func TestEngine_Execute_ScriptError(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: `error("something went wrong")`,
		Name:    "error-script",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	_, err = engine.Execute(ctx, scriptID, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrScriptExecutionFailed)
}

func TestEngine_Execute_NilReturn(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: `-- 반환값 없음`,
		Name:    "nil-return",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	result, err := engine.Execute(ctx, scriptID, nil)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestEngine_Execute_TableReturn(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: `return {name="test", count=3}`,
		Name:    "table-return",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	result, err := engine.Execute(ctx, scriptID, input(nil))
	require.NoError(t, err)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "test", m["name"])
	assert.Equal(t, float64(3), m["count"])
}

func TestEngine_Execute_WithTimeout(t *testing.T) {
	engine := NewScriptEngine(
		WithPoolSize(2),
		WithMaxExecutionTime(100*time.Millisecond),
	)
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: `while true do end`,
		Name:    "infinite-loop",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	_, err = engine.Execute(ctx, scriptID, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrScriptTimeout)
}

func TestEngine_Execute_WithContextCancel(t *testing.T) {
	engine := NewScriptEngine(
		WithPoolSize(2),
		WithMaxExecutionTime(5*time.Second),
	)
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: `while true do end`,
		Name:    "cancel-script",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	execCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	_, err = engine.Execute(execCtx, scriptID, nil)
	assert.Error(t, err)
}

func TestEngine_Execute_Stats(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: "return 1",
		Name:    "stats-script",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	// 성공적인 실행
	_, err = engine.Execute(ctx, scriptID, nil)
	require.NoError(t, err)

	stats := engine.Stats()
	assert.Equal(t, int64(1), stats.TotalExecutions.Load())
	assert.Equal(t, int64(0), stats.TotalErrors.Load())

	// 실패하는 실행
	_, _ = engine.Execute(ctx, "nonexistent", nil)
	stats = engine.Stats()
	assert.Equal(t, int64(2), stats.TotalExecutions.Load())
	assert.Equal(t, int64(1), stats.TotalErrors.Load())
}

// ============================================================
// Remove 테스트
// ============================================================

func TestEngine_Remove(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: "return 1",
		Name:    "removable",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	// 제거 전 실행 가능
	_, err = engine.Execute(ctx, scriptID, nil)
	require.NoError(t, err)

	// 제거
	err = engine.Remove(scriptID)
	require.NoError(t, err)

	// 제거 후 실행 불가
	_, err = engine.Execute(ctx, scriptID, nil)
	assert.ErrorIs(t, err, ErrScriptNotFound)
}

func TestEngine_Remove_NotFound(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	err := engine.Remove("nonexistent")
	assert.ErrorIs(t, err, ErrScriptNotFound)
}

// ============================================================
// VMPool 테스트
// ============================================================

func TestEngine_VMPool_Exhaustion(t *testing.T) {
	engine := NewScriptEngine(
		WithPoolSize(1),
		WithMaxExecutionTime(2*time.Second),
	)
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: `while true do end`,
		Name:    "blocking",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	// 첫 번째 실행 - VM을 점유하게 됨 (별도 goroutine에서)
	started := make(chan struct{})
	go func() {
		close(started)
		engine.Execute(ctx, scriptID, nil) //nolint:errcheck
	}()
	<-started
	// 잠시 대기하여 VM이 점유되도록 함
	time.Sleep(50 * time.Millisecond)

	// 두 번째 실행 - VM 풀이 고갈되어야 함
	_, err = engine.Execute(ctx, scriptID, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrVMPoolExhausted)
}

func TestEngine_VMPool_Recycling(t *testing.T) {
	engine := NewScriptEngine(
		WithPoolSize(2),
		WithMaxVMUses(2),
	)
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: "return 1",
		Name:    "recycle",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	// maxVMUses가 2이므로 3번째 사용 시 VM이 재생성되어야 한다
	for i := 0; i < 5; i++ {
		result, err := engine.Execute(ctx, scriptID, nil)
		require.NoError(t, err, "execution %d should succeed", i)
		assert.Equal(t, float64(1), result)
	}
}

// ============================================================
// Info / Stats 테스트
// ============================================================

func TestEngine_Info(t *testing.T) {
	engine := NewScriptEngine(
		WithPoolSize(4),
		WithMaxExecutionTime(10*time.Second),
		WithSandboxConfig(DefaultSandboxConfig()),
	)
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	info := engine.Info()
	assert.Equal(t, 4, info.PoolSize)
	assert.Equal(t, 4, info.IdleVMs)
	assert.Equal(t, 0, info.ActiveVMs)
	assert.Equal(t, 0, info.CachedScripts)
	assert.True(t, info.SandboxEnabled)
	assert.Equal(t, 10*time.Second, info.MaxExecutionTime)
}

func TestEngine_Stats_Initial(t *testing.T) {
	engine := NewScriptEngine()
	stats := engine.Stats()
	assert.Equal(t, int64(0), stats.TotalExecutions.Load())
	assert.Equal(t, int64(0), stats.TotalErrors.Load())
	assert.Equal(t, int64(0), stats.TotalTimeouts.Load())
	assert.Equal(t, int64(0), stats.VMPoolExhausted.Load())
	assert.Equal(t, int64(0), stats.CacheHits.Load())
	assert.Equal(t, int64(0), stats.CacheMisses.Load())
}

// ============================================================
// Sandbox 통합 테스트
// ============================================================

func TestEngine_Execute_SandboxBlocksOs(t *testing.T) {
	engine := NewScriptEngine(
		WithPoolSize(2),
		WithSandboxConfig(DefaultSandboxConfig()),
	)
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: `if os == nil then return "blocked" else return "allowed" end`,
		Name:    "sandbox-test",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	result, err := engine.Execute(ctx, scriptID, nil)
	require.NoError(t, err)
	assert.Equal(t, "blocked", result)
}

// ============================================================
// 동시성 테스트
// ============================================================

func TestEngine_ConcurrentExecution(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(4))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	src := ScriptSource{
		Type:    SourceInline,
		Content: `return msg.idx * 2`,
		Name:    "concurrent",
	}
	scriptID, err := engine.Compile(ctx, src)
	require.NoError(t, err)

	const goroutines = 20
	var wg sync.WaitGroup
	var successCount atomic.Int64

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			input := map[string]any{"idx": idx}
			result, err := engine.Execute(ctx, scriptID, input)
			if err == nil {
				expected := float64(idx * 2)
				if result == expected {
					successCount.Add(1)
				}
			}
		}(i)
	}

	wg.Wait()
	// 풀 사이즈가 4이므로 동시에 최대 4개 실행 가능
	// 나머지는 풀 고갈로 실패할 수 있음
	assert.GreaterOrEqual(t, successCount.Load(), int64(4), "at least pool size number of executions should succeed")
}

func TestEngine_ConcurrentCompile(t *testing.T) {
	engine := NewScriptEngine(WithPoolSize(2))
	ctx := context.Background()
	require.NoError(t, engine.Init(ctx))
	defer engine.Shutdown(ctx)

	const goroutines = 10
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			src := ScriptSource{
				Type:    SourceInline,
				Content: "return 1",
				Name:    "concurrent-compile",
			}
			_, err := engine.Compile(ctx, src)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()
}

// ============================================================
// ScriptEngine 인터페이스 컴파일 타임 확인
// ============================================================

func TestScriptEngineInterface(t *testing.T) {
	// DefaultScriptEngine이 ScriptEngine 인터페이스를 구현하는지 확인
	var _ ScriptEngine = (*DefaultScriptEngine)(nil)
}

// input 은 테스트 헬퍼로, nil을 any 타입으로 반환한다
func input(v any) any {
	return v
}
