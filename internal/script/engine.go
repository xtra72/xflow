package script

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

// SourceType 은 스크립트 소스의 유형을 나타낸다.
type SourceType int

const (
	// SourceInline 은 인라인 스크립트 소스이다.
	SourceInline SourceType = iota
	// SourceFile 은 파일 기반 스크립트 소스이다.
	SourceFile
	// SourceStore 는 Store 기반 스크립트 소스이다.
	SourceStore
)

// ScriptSource 는 스크립트 소스 정보를 담는 구조체이다.
type ScriptSource struct {
	// Type 은 소스 유형이다.
	Type SourceType
	// Content 는 스크립트 본문이다 (SourceInline 시 사용).
	Content string
	// Path 는 스크립트 파일 경로이다 (SourceFile 시 사용).
	Path string
	// StoreKey 는 Store에서 스크립트를 조회할 키이다 (SourceStore 시 사용).
	StoreKey string
	// Name 은 스크립트 이름이다 (scriptID 생성에 사용).
	Name string
}

// ScriptEngine 은 스크립트 엔진의 공개 인터페이스이다.
type ScriptEngine interface {
	// Execute 는 컴파일된 스크립트를 실행한다.
	Execute(ctx context.Context, scriptID string, input any) (any, error)
	// Compile 은 스크립트 소스를 컴파일하고 캐시에 저장한다.
	Compile(ctx context.Context, source ScriptSource) (string, error)
	// Remove 는 캐시에서 스크립트를 제거한다.
	Remove(scriptID string) error
	// Stats 는 엔진 통계를 반환한다.
	Stats() *ScriptEngineStats
	// Info 는 엔진 정보를 반환한다.
	Info() ScriptEngineInfo
}

// ScriptEngineInfo 는 엔진의 현재 상태 정보이다.
type ScriptEngineInfo struct {
	PoolSize         int
	ActiveVMs        int
	IdleVMs          int
	CachedScripts    int
	SandboxEnabled   bool
	MaxExecutionTime time.Duration
}

// ScriptEngineStats 는 엔진의 실행 통계이다.
type ScriptEngineStats struct {
	TotalExecutions atomic.Int64
	TotalErrors     atomic.Int64
	TotalTimeouts   atomic.Int64
	VMPoolExhausted atomic.Int64
	CacheHits       atomic.Int64
	CacheMisses     atomic.Int64
}

// engineConfig 는 엔진 내부 설정이다.
type engineConfig struct {
	poolSize         int
	maxExecutionTime time.Duration
	sandboxConfig    SandboxConfig
	maxVMUses        int
}

// EngineOption 은 엔진 생성 옵션 함수 타입이다.
type EngineOption func(*engineConfig)

// WithPoolSize 는 VM 풀 크기를 설정한다.
func WithPoolSize(size int) EngineOption {
	return func(c *engineConfig) {
		if size > 0 {
			c.poolSize = size
		}
	}
}

// WithMaxExecutionTime 는 스크립트 최대 실행 시간을 설정한다.
func WithMaxExecutionTime(d time.Duration) EngineOption {
	return func(c *engineConfig) {
		c.maxExecutionTime = d
	}
}

// WithSandboxConfig 는 샌드박스 설정을 지정한다.
func WithSandboxConfig(cfg SandboxConfig) EngineOption {
	return func(c *engineConfig) {
		c.sandboxConfig = cfg
	}
}

// WithMaxVMUses 는 VM 재사용 횟수 제한을 설정한다.
// 제한 초과 시 VM을 폐기하고 새로 생성한다.
func WithMaxVMUses(n int) EngineOption {
	return func(c *engineConfig) {
		if n > 0 {
			c.maxVMUses = n
		}
	}
}

// DefaultScriptEngine 은 ScriptEngine 인터페이스의 기본 구현체이다.
type DefaultScriptEngine struct {
	config engineConfig
	pool   *vmPool
	cache  sync.Map // map[string]*lua.FunctionProto
	stats  ScriptEngineStats
	inited bool
	mu     sync.Mutex
}

// NewScriptEngine 은 새 DefaultScriptEngine을 생성한다.
func NewScriptEngine(opts ...EngineOption) *DefaultScriptEngine {
	cfg := engineConfig{
		poolSize:         8,
		maxExecutionTime: 5 * time.Second,
		sandboxConfig:    DefaultSandboxConfig(),
		maxVMUses:        0, // 0은 제한 없음
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &DefaultScriptEngine{
		config: cfg,
	}
}

// Init 은 VM 풀과 캐시를 초기화한다.
func (e *DefaultScriptEngine) Init(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.inited {
		return nil
	}

	e.pool = newVMPool(e.config.poolSize, e.config.maxVMUses, e.config.sandboxConfig)
	e.inited = true
	return nil
}

// Shutdown 은 모든 VM을 닫고 캐시를 비운다.
func (e *DefaultScriptEngine) Shutdown(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.inited {
		return nil
	}

	if e.pool != nil {
		e.pool.closeAll()
	}

	// 캐시 비우기
	e.cache.Range(func(key, value any) bool {
		e.cache.Delete(key)
		return true
	})

	e.inited = false
	return nil
}

// Compile 은 스크립트 소스를 컴파일하여 캐시에 저장한다.
// 동일한 이름의 스크립트가 이미 있으면 덮어쓴다.
func (e *DefaultScriptEngine) Compile(ctx context.Context, source ScriptSource) (string, error) {
	// 소스 유효성 검사
	if source.Content == "" {
		return "", &ScriptError{
			Err:      ErrInvalidScriptSource,
			ScriptID: source.Name,
			Detail:   "empty script content",
		}
	}

	// 스크립트 ID 생성
	scriptID := generateScriptID(source)

	// 파싱 및 컴파일
	chunk, err := parse.Parse(strings.NewReader(source.Content), source.Name)
	if err != nil {
		return "", &ScriptError{
			Err:      ErrScriptCompileFailed,
			ScriptID: scriptID,
			Detail:   err.Error(),
		}
	}

	proto, err := lua.Compile(chunk, source.Name)
	if err != nil {
		return "", &ScriptError{
			Err:      ErrScriptCompileFailed,
			ScriptID: scriptID,
			Detail:   err.Error(),
		}
	}

	// 캐시에 저장
	e.cache.Store(scriptID, proto)

	return scriptID, nil
}

// Execute 는 컴파일된 스크립트를 실행한다.
func (e *DefaultScriptEngine) Execute(ctx context.Context, scriptID string, input any) (any, error) {
	e.stats.TotalExecutions.Add(1)

	// 캐시에서 FunctionProto 조회
	protoVal, ok := e.cache.Load(scriptID)
	if !ok {
		e.stats.TotalErrors.Add(1)
		e.stats.CacheMisses.Add(1)
		return nil, &ScriptError{
			Err:      ErrScriptNotFound,
			ScriptID: scriptID,
			Detail:   "script not in cache",
		}
	}
	e.stats.CacheHits.Add(1)

	proto := protoVal.(*lua.FunctionProto)

	// VM 풀에서 획득
	L, err := e.pool.acquire()
	if err != nil {
		e.stats.TotalErrors.Add(1)
		e.stats.VMPoolExhausted.Add(1)
		return nil, &ScriptError{
			Err:      ErrVMPoolExhausted,
			ScriptID: scriptID,
			Detail:   "no available VMs in pool",
		}
	}

	// 실행 완료 후 VM 반환
	defer e.pool.release(L)

	// input을 전역 "msg"로 설정
	if input != nil {
		L.SetGlobal("msg", ToLuaValue(L, input))
	} else {
		L.SetGlobal("msg", lua.LNil)
	}

	// 실행 시간 제한 설정
	execTimeout := e.config.maxExecutionTime
	execCtx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	L.SetContext(execCtx)
	defer L.RemoveContext()

	// FunctionProto를 스택에 push하고 실행
	lfunc := L.NewFunctionFromProto(proto)
	L.Push(lfunc)

	top := L.GetTop()
	err = L.PCall(0, lua.MultRet, nil)
	if err != nil {
		e.stats.TotalErrors.Add(1)
		// context timeout/cancel 확인
		if execCtx.Err() != nil {
			e.stats.TotalTimeouts.Add(1)
			return nil, &ScriptError{
				Err:      ErrScriptTimeout,
				ScriptID: scriptID,
				Detail:   fmt.Sprintf("execution exceeded %v", execTimeout),
			}
		}
		return nil, &ScriptError{
			Err:      ErrScriptExecutionFailed,
			ScriptID: scriptID,
			Detail:   err.Error(),
		}
	}

	// 반환값 처리: PCall 전 스택 크기와 비교하여 반환값 유무를 확인한다.
	// PCall은 함수 자체를 스택에서 제거하므로, 새 top이 이전 top - 1보다
	// 크면 반환값이 있는 것이다.
	var result any
	nResults := L.GetTop() - (top - 1) // top에는 function이 포함되어 있었으므로 -1
	if nResults > 0 {
		ret := L.Get(-1)
		L.Pop(nResults)
		result = FromLuaValue(ret)
	}

	// 전역 변수 정리
	L.SetGlobal("msg", lua.LNil)

	return result, nil
}

// Remove 는 캐시에서 스크립트를 제거한다.
func (e *DefaultScriptEngine) Remove(scriptID string) error {
	if _, ok := e.cache.Load(scriptID); !ok {
		return &ScriptError{
			Err:      ErrScriptNotFound,
			ScriptID: scriptID,
			Detail:   "script not in cache",
		}
	}
	e.cache.Delete(scriptID)
	return nil
}

// Stats 는 엔진 실행 통계를 반환한다.
func (e *DefaultScriptEngine) Stats() *ScriptEngineStats {
	return &e.stats
}

// Info 는 엔진 현재 상태 정보를 반환한다.
func (e *DefaultScriptEngine) Info() ScriptEngineInfo {
	info := ScriptEngineInfo{
		PoolSize:         e.config.poolSize,
		SandboxEnabled:   e.config.sandboxConfig.Enabled,
		MaxExecutionTime: e.config.maxExecutionTime,
	}

	if e.pool != nil {
		info.IdleVMs = len(e.pool.pool)
		info.ActiveVMs = e.pool.size - info.IdleVMs
	}

	// 캐시된 스크립트 수
	count := 0
	e.cache.Range(func(_, _ any) bool {
		count++
		return true
	})
	info.CachedScripts = count

	return info
}

// generateScriptID 는 ScriptSource에서 scriptID를 생성한다.
// Name이 있으면 Name을 사용하고, 없으면 Content의 해시를 사용한다.
func generateScriptID(source ScriptSource) string {
	if source.Name != "" {
		return source.Name
	}
	hash := sha256.Sum256([]byte(source.Content))
	return fmt.Sprintf("script-%x", hash[:8])
}

// ============================================================
// vmPool - 내부 VM 풀 구현
// ============================================================

type vmPool struct {
	pool    chan *lua.LState
	size    int
	maxUses int
	sandbox SandboxConfig
	uses    sync.Map // map[*lua.LState]int
}

func newVMPool(size, maxUses int, sandbox SandboxConfig) *vmPool {
	p := &vmPool{
		pool:    make(chan *lua.LState, size),
		size:    size,
		maxUses: maxUses,
		sandbox: sandbox,
	}
	// 풀 초기화
	for i := 0; i < size; i++ {
		p.pool <- p.createVM()
	}
	return p
}

// acquire 는 풀에서 VM을 가져온다. 비차단 방식이다.
func (p *vmPool) acquire() (*lua.LState, error) {
	select {
	case L := <-p.pool:
		return L, nil
	default:
		return nil, ErrVMPoolExhausted
	}
}

// release 는 VM을 풀에 반환한다. maxUses 초과 시 재생성한다.
func (p *vmPool) release(L *lua.LState) {
	if p.maxUses > 0 {
		countVal, _ := p.uses.LoadOrStore(L, 0)
		count := countVal.(int) + 1
		if count >= p.maxUses {
			// 사용 횟수 초과: 기존 VM 닫고 새 VM 생성
			p.uses.Delete(L)
			L.Close()
			L = p.createVM()
		} else {
			p.uses.Store(L, count)
		}
	}

	// 풀 용량 초과 시 그냥 닫기 (풀이 이미 가득 찬 경우)
	select {
	case p.pool <- L:
	default:
		L.Close()
	}
}

// createVM 은 새 LState를 생성하고 샌드박스를 적용한다.
func (p *vmPool) createVM() *lua.LState {
	L := lua.NewState()
	ApplySandbox(L, p.sandbox) //nolint:errcheck
	return L
}

// closeAll 은 풀의 모든 VM을 닫는다.
func (p *vmPool) closeAll() {
	close(p.pool)
	for L := range p.pool {
		p.uses.Delete(L)
		L.Close()
	}
}
