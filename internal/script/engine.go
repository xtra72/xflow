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

	// stdlibEnabled 는 VM 생성 시 xflow 표준 라이브러리를 등록할지 여부이다.
	// WithStdlib 옵션으로만 true 가 된다. 미설정(false) 시 createVM 은
	// RegisterStdlib 를 호출하지 않아 기존 동작(xflow 글로벌 미노출)을 그대로 유지한다.
	stdlibEnabled bool
	// stdlibOptions 는 활성화할 xflow 모듈 집합이다 (stdlibEnabled 일 때만 사용).
	stdlibOptions StdlibOptions
	// stdlibDeps 는 xflow 모듈의 외부 의존성이다 (agent/device 룩업 등).
	stdlibDeps StdlibDeps
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

// WithStdlib 는 프로덕션 VM 에 xflow 표준 라이브러리를 등록하도록 설정한다.
//
// 이 옵션이 주어지면 풀의 모든 VM(초기 생성 및 maxUses 재활용 포함)이 생성 직후
// (샌드박스 적용 이후) RegisterStdlib(L, opts, deps) 를 1회 실행하여 xflow.* 모듈을
// 노출한다. 옵션 미사용 시 xflow 글로벌이 노출되지 않아 기존 동작이 그대로 유지된다.
//
// 성능: 등록은 VM 생성 시점에 1회만 수행되며 실행(Execute)마다 반복되지 않는다.
func WithStdlib(opts StdlibOptions, deps StdlibDeps) EngineOption {
	return func(c *engineConfig) {
		c.stdlibEnabled = true
		c.stdlibOptions = opts
		c.stdlibDeps = deps
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

	// storeBindings 는 실행별(per-Execute) 스토어 바인딩이다.
	// 키는 *lua.LState, 값은 StoreAccessor. 실행 직전 바인딩하고 실행 후 해제한다.
	//
	// 동시성: VM 은 pool.acquire ~ release 구간에서 한 고루틴이 배타적으로 소유하므로
	// (acquire 는 채널에서 VM 을 꺼내고 release 가 되돌린다), 특정 *lua.LState 에 대한
	// 바인딩 set/clear/read 는 그 구간 안에서만 발생하여 경쟁이 없다. sync.Map 은
	// 서로 다른 LState 항목에 대한 동시 접근(다른 VM 을 쓰는 병렬 Execute)만 처리한다.
	storeBindings sync.Map // map[*lua.LState]StoreAccessor
}

// resolveBoundStore 는 주어진 LState 에 현재 바인딩된 StoreAccessor 를 반환한다.
// stdlib 의 xflow.store 모듈이 StoreProvider 로 사용한다. 바인딩이 없으면 nil.
func (e *DefaultScriptEngine) resolveBoundStore(L *lua.LState) StoreAccessor {
	if v, ok := e.storeBindings.Load(L); ok {
		if s, ok := v.(StoreAccessor); ok {
			return s
		}
	}
	return nil
}

// bindStore 는 실행 직전 LState 에 스토어를 바인딩한다(store 가 nil 이면 no-op).
func (e *DefaultScriptEngine) bindStore(L *lua.LState, store StoreAccessor) {
	if store == nil {
		return
	}
	e.storeBindings.Store(L, store)
}

// unbindStore 는 실행 후 LState 의 스토어 바인딩을 해제한다.
// error/panic 여부와 무관하게 반드시 호출되어야 한다(VM 이 풀로 반환되기 때문).
func (e *DefaultScriptEngine) unbindStore(L *lua.LState) {
	e.storeBindings.Delete(L)
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

	// 실행별 스토어 바인딩(Follow-up A): stdlib 이 활성화되어 있으면 엔진 자신의
	// per-L 바인딩 해석자를 StoreProvider 로 주입한다. 이렇게 하면 풀링된 VM 의
	// xflow.store 가 고정 스토어가 아니라 "이번 실행에 바인딩된" 네임스페이스 스토어를
	// 매 호출 조회한다. 호출 측(cmd)이 별도 StoreProvider 를 주지 않아도 되도록
	// 여기서 엔진이 자동으로 연결한다(기존 고정 Store 는 폴백으로 보존).
	deps := e.config.stdlibDeps
	if e.config.stdlibEnabled && deps.StoreProvider == nil {
		deps.StoreProvider = e.resolveBoundStore
	}

	e.pool = newVMPool(e.config.poolSize, e.config.maxVMUses, e.config.sandboxConfig, stdlibConfig{
		enabled: e.config.stdlibEnabled,
		options: e.config.stdlibOptions,
		deps:    deps,
	})
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

// Execute 는 컴파일된 스크립트를 실행한다(스토어 바인딩 없음).
// 하위 호환을 위해 유지하며, ExecuteWithStore(store=nil) 로 위임한다.
func (e *DefaultScriptEngine) Execute(ctx context.Context, scriptID string, input any) (any, error) {
	return e.ExecuteWithStore(ctx, scriptID, input, nil)
}

// ExecuteWithStore 는 이번 실행에 한정된 StoreAccessor 를 바인딩하여 스크립트를
// 실행한다(Follow-up A). store 가 nil 이면 바인딩 없이 실행하며(=Execute 와 동일),
// xflow.store 는 nil-safe 로 동작한다.
//
// 바인딩 수명: VM 획득(acquire) 직후 bindStore, 반환(release) 전 defer unbindStore
// 로 반드시 해제한다. error/timeout/panic 어느 경로에서도 defer 가 실행되어 VM 이
// 다음 실행으로 스토어를 누출하지 않는다. VM 은 acquire~release 구간에서 배타적이므로
// per-L 바인딩은 경쟁이 없다.
func (e *DefaultScriptEngine) ExecuteWithStore(ctx context.Context, scriptID string, input any, store StoreAccessor) (any, error) {
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

	// 이번 실행의 스토어를 이 VM 에 바인딩하고, 실행 후 반드시 해제한다.
	e.bindStore(L, store)
	defer e.unbindStore(L)

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

// stdlibConfig 는 VM 생성 시 xflow 표준 라이브러리 등록 설정을 묶는다.
type stdlibConfig struct {
	enabled bool
	options StdlibOptions
	deps    StdlibDeps
}

type vmPool struct {
	pool    chan *lua.LState
	size    int
	maxUses int
	sandbox SandboxConfig
	stdlib  stdlibConfig
	uses    sync.Map // map[*lua.LState]int
}

func newVMPool(size, maxUses int, sandbox SandboxConfig, stdlib stdlibConfig) *vmPool {
	p := &vmPool{
		pool:    make(chan *lua.LState, size),
		size:    size,
		maxUses: maxUses,
		sandbox: sandbox,
		stdlib:  stdlib,
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

// createVM 은 새 LState를 생성하고 샌드박스를 적용한 뒤, 활성화된 경우 xflow
// 표준 라이브러리를 등록한다.
//
// 순서 주의: RegisterStdlib 는 반드시 ApplySandbox 이후에 호출한다. 샌드박스가
// 위험 전역(os/io/load 등)을 제거한 다음 xflow 모듈을 얹어야, 샌드박스 제약이
// xflow 등록으로 약화되지 않는다. 등록은 VM 생성 시 1회만 수행되며 실행마다
// 반복되지 않는다(성능 보존).
func (p *vmPool) createVM() *lua.LState {
	L := lua.NewState()
	ApplySandbox(L, p.sandbox) //nolint:errcheck
	if p.stdlib.enabled {
		// 등록 실패는 치명적이지 않다(모듈 부재와 동일 — 스크립트가 xflow.* 를
		// 쓰지 않으면 무해). 방어적으로 무시하되, VM 자체는 정상 사용 가능하다.
		_ = RegisterStdlib(L, p.stdlib.options, p.stdlib.deps)
	}
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
