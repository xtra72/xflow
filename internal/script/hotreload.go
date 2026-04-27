package script

import (
	"context"
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

// scriptVersion 은 스크립트의 버전 정보를 추적한다.
type scriptVersion struct {
	version       int
	currentProto  *lua.FunctionProto
	previousProto *lua.FunctionProto
}

// HotReloadManager 는 스크립트 핫 리로드를 관리한다.
type HotReloadManager struct {
	engine   *DefaultScriptEngine
	versions sync.Map // map[string]*scriptVersion
	mu       sync.Mutex
	callback func(scriptID string, version int, err error)
}

// NewHotReloadManager 는 새 HotReloadManager를 생성한다.
func NewHotReloadManager(engine *DefaultScriptEngine) *HotReloadManager {
	return &HotReloadManager{
		engine: engine,
	}
}

// OnReload 는 리로드 시 호출될 콜백을 등록한다.
func (h *HotReloadManager) OnReload(callback func(scriptID string, version int, err error)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.callback = callback
}

// Reload 는 스크립트를 새 소스로 핫 리로드한다.
// 1. 새 소스를 컴파일한다.
// 2. 컴파일 실패 시 기존 스크립트를 유지하고 에러를 반환한다.
// 3. 현재 proto를 previous로 저장하고, 새 proto를 current로 설정한다.
// 4. 엔진 캐시를 원자적으로 업데이트한다.
// 5. 버전을 증가시킨다.
// 6. 콜백이 등록되어 있으면 호출한다.
func (h *HotReloadManager) Reload(ctx context.Context, scriptID string, newSource ScriptSource) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 새 소스 컴파일
	if newSource.Content == "" {
		err := &ScriptError{
			Err:      ErrInvalidScriptSource,
			ScriptID: scriptID,
			Detail:   "empty script content",
		}
		h.invokeCallback(scriptID, 0, err)
		return err
	}

	chunk, parseErr := parse.Parse(strings.NewReader(newSource.Content), newSource.Name)
	if parseErr != nil {
		err := &ScriptError{
			Err:      ErrScriptCompileFailed,
			ScriptID: scriptID,
			Detail:   parseErr.Error(),
		}
		h.invokeCallback(scriptID, 0, err)
		return err
	}

	newProto, compileErr := lua.Compile(chunk, newSource.Name)
	if compileErr != nil {
		err := &ScriptError{
			Err:      ErrScriptCompileFailed,
			ScriptID: scriptID,
			Detail:   compileErr.Error(),
		}
		h.invokeCallback(scriptID, 0, err)
		return err
	}

	// 현재 proto를 캐시에서 로드
	var currentProto *lua.FunctionProto
	if protoVal, ok := h.engine.cache.Load(scriptID); ok {
		currentProto = protoVal.(*lua.FunctionProto)
	}

	// 버전 정보 업데이트
	var ver *scriptVersion
	if v, ok := h.versions.Load(scriptID); ok {
		ver = v.(*scriptVersion)
	} else {
		ver = &scriptVersion{}
	}

	ver.previousProto = currentProto
	ver.currentProto = newProto
	ver.version++
	h.versions.Store(scriptID, ver)

	// 엔진 캐시 원자적 업데이트
	h.engine.cache.Store(scriptID, newProto)

	// 콜백 호출
	h.invokeCallback(scriptID, ver.version, nil)

	return nil
}

// Rollback 은 스크립트를 이전 버전으로 롤백한다.
func (h *HotReloadManager) Rollback(ctx context.Context, scriptID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	v, ok := h.versions.Load(scriptID)
	if !ok {
		return &ScriptError{
			Err:      ErrHotReloadFailed,
			ScriptID: scriptID,
			Detail:   "no version history for script",
		}
	}

	ver := v.(*scriptVersion)
	if ver.previousProto == nil {
		return &ScriptError{
			Err:      ErrHotReloadFailed,
			ScriptID: scriptID,
			Detail:   "no previous version to rollback to",
		}
	}

	// 이전 버전으로 복원
	h.engine.cache.Store(scriptID, ver.previousProto)

	// 버전 정보 업데이트
	ver.currentProto = ver.previousProto
	ver.previousProto = nil
	ver.version--
	h.versions.Store(scriptID, ver)

	// 콜백 호출
	h.invokeCallback(scriptID, ver.version, nil)

	return nil
}

// Version 은 스크립트의 현재 버전을 반환한다.
func (h *HotReloadManager) Version(scriptID string) (int, error) {
	v, ok := h.versions.Load(scriptID)
	if !ok {
		return 0, &ScriptError{
			Err:      ErrScriptNotFound,
			ScriptID: scriptID,
			Detail:   "no version info for script",
		}
	}
	return v.(*scriptVersion).version, nil
}

// invokeCallback 은 등록된 콜백을 호출한다.
func (h *HotReloadManager) invokeCallback(scriptID string, version int, err error) {
	if h.callback != nil {
		h.callback(scriptID, version, err)
	}
}
