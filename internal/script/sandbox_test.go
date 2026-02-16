package script

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	lua "github.com/yuin/gopher-lua"
)

// ============================================================
// SandboxConfig 테스트
// ============================================================

func TestDefaultSandboxConfig(t *testing.T) {
	cfg := DefaultSandboxConfig()

	assert.True(t, cfg.Enabled)
	assert.Equal(t, []string{"os", "io", "debug"}, cfg.DisabledModules)
	assert.Equal(t, []string{"loadfile", "dofile"}, cfg.DisabledFunctions)
	assert.Equal(t, 5*time.Second, cfg.MaxExecutionTime)
	assert.Equal(t, 128, cfg.MaxMemoryMB)
	assert.False(t, cfg.AllowDynamicLoad)
}

func TestSandboxConfig_CustomValues(t *testing.T) {
	cfg := SandboxConfig{
		Enabled:           true,
		DisabledModules:   []string{"os"},
		DisabledFunctions: []string{"loadfile"},
		MaxExecutionTime:  10 * time.Second,
		MaxMemoryMB:       256,
		AllowDynamicLoad:  true,
	}

	assert.True(t, cfg.Enabled)
	assert.Equal(t, []string{"os"}, cfg.DisabledModules)
	assert.Equal(t, 10*time.Second, cfg.MaxExecutionTime)
	assert.True(t, cfg.AllowDynamicLoad)
}

// ============================================================
// ApplySandbox 테스트
// ============================================================

func TestApplySandbox_DisablesModules(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	cfg := DefaultSandboxConfig()
	err := ApplySandbox(L, cfg)
	require.NoError(t, err)

	// os 모듈이 nil이어야 한다
	osVal := L.GetGlobal("os")
	assert.Equal(t, lua.LNil, osVal, "os module should be nil")

	// io 모듈이 nil이어야 한다
	ioVal := L.GetGlobal("io")
	assert.Equal(t, lua.LNil, ioVal, "io module should be nil")

	// debug 모듈이 nil이어야 한다
	debugVal := L.GetGlobal("debug")
	assert.Equal(t, lua.LNil, debugVal, "debug module should be nil")
}

func TestApplySandbox_DisablesFunctions(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	cfg := DefaultSandboxConfig()
	err := ApplySandbox(L, cfg)
	require.NoError(t, err)

	// loadfile 함수가 nil이어야 한다
	loadfileVal := L.GetGlobal("loadfile")
	assert.Equal(t, lua.LNil, loadfileVal, "loadfile should be nil")

	// dofile 함수가 nil이어야 한다
	dofileVal := L.GetGlobal("dofile")
	assert.Equal(t, lua.LNil, dofileVal, "dofile should be nil")
}

func TestApplySandbox_DisablesLoadWhenDynamicLoadFalse(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	cfg := DefaultSandboxConfig()
	cfg.AllowDynamicLoad = false
	err := ApplySandbox(L, cfg)
	require.NoError(t, err)

	// load 함수가 nil이어야 한다
	loadVal := L.GetGlobal("load")
	assert.Equal(t, lua.LNil, loadVal, "load should be nil when AllowDynamicLoad is false")
}

func TestApplySandbox_KeepsLoadWhenDynamicLoadTrue(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	cfg := DefaultSandboxConfig()
	cfg.AllowDynamicLoad = true
	err := ApplySandbox(L, cfg)
	require.NoError(t, err)

	// load 함수가 여전히 존재해야 한다
	loadVal := L.GetGlobal("load")
	assert.NotEqual(t, lua.LNil, loadVal, "load should remain when AllowDynamicLoad is true")
}

func TestApplySandbox_AllowsSafeModules(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	cfg := DefaultSandboxConfig()
	err := ApplySandbox(L, cfg)
	require.NoError(t, err)

	// string, table, math 등 안전한 모듈은 유지되어야 한다
	stringVal := L.GetGlobal("string")
	assert.NotEqual(t, lua.LNil, stringVal, "string module should remain")

	tableVal := L.GetGlobal("table")
	assert.NotEqual(t, lua.LNil, tableVal, "table module should remain")

	mathVal := L.GetGlobal("math")
	assert.NotEqual(t, lua.LNil, mathVal, "math module should remain")
}

func TestApplySandbox_AllowsSafeFunctions(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	cfg := DefaultSandboxConfig()
	err := ApplySandbox(L, cfg)
	require.NoError(t, err)

	// print, type, tostring 등 안전한 함수는 유지되어야 한다
	printVal := L.GetGlobal("print")
	assert.NotEqual(t, lua.LNil, printVal, "print should remain")

	typeVal := L.GetGlobal("type")
	assert.NotEqual(t, lua.LNil, typeVal, "type should remain")

	tostringVal := L.GetGlobal("tostring")
	assert.NotEqual(t, lua.LNil, tostringVal, "tostring should remain")
}

func TestApplySandbox_DisabledFalse(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	cfg := SandboxConfig{
		Enabled: false,
	}
	err := ApplySandbox(L, cfg)
	require.NoError(t, err)

	// 샌드박스 비활성화 시 모든 모듈이 유지되어야 한다
	osVal := L.GetGlobal("os")
	assert.NotEqual(t, lua.LNil, osVal, "os module should remain when sandbox disabled")
}

func TestApplySandbox_CustomModules(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	cfg := SandboxConfig{
		Enabled:         true,
		DisabledModules: []string{"math"},
	}
	err := ApplySandbox(L, cfg)
	require.NoError(t, err)

	// math가 비활성화되어야 한다
	mathVal := L.GetGlobal("math")
	assert.Equal(t, lua.LNil, mathVal, "math module should be nil")

	// os는 유지되어야 한다 (비활성화 목록에 없음)
	osVal := L.GetGlobal("os")
	assert.NotEqual(t, lua.LNil, osVal, "os module should remain")
}

func TestApplySandbox_EmptyConfig(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	cfg := SandboxConfig{
		Enabled:           true,
		DisabledModules:   []string{},
		DisabledFunctions: []string{},
		AllowDynamicLoad:  true,
	}
	err := ApplySandbox(L, cfg)
	require.NoError(t, err)

	// 비활성화 목록이 비어있으면 모두 유지
	osVal := L.GetGlobal("os")
	assert.NotEqual(t, lua.LNil, osVal)
}

func TestApplySandbox_VerifyScriptCantAccessOs(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	cfg := DefaultSandboxConfig()
	err := ApplySandbox(L, cfg)
	require.NoError(t, err)

	// os.execute 시도 시 에러 발생 확인
	luaErr := L.DoString(`
		local result = os
		if result == nil then
			error("os is nil - sandbox working")
		end
	`)
	assert.Error(t, luaErr, "should error when accessing disabled module in script")
	assert.Contains(t, luaErr.Error(), "os is nil")
}
