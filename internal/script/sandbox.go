package script

import (
	"time"

	lua "github.com/yuin/gopher-lua"
)

// SandboxConfig 는 Lua 실행 샌드박스 설정을 정의한다.
type SandboxConfig struct {
	// Enabled 는 샌드박스 활성화 여부이다.
	Enabled bool
	// DisabledModules 는 비활성화할 Lua 모듈 목록이다 (예: "os", "io", "debug").
	DisabledModules []string
	// DisabledFunctions 는 비활성화할 전역 함수 목록이다 (예: "loadfile", "dofile").
	DisabledFunctions []string
	// MaxExecutionTime 는 스크립트 최대 실행 시간이다.
	MaxExecutionTime time.Duration
	// MaxMemoryMB 는 최대 메모리 사용량(MB)이다.
	MaxMemoryMB int
	// AllowDynamicLoad 는 load 함수 허용 여부이다. false이면 load 함수도 제거된다.
	AllowDynamicLoad bool
}

// DefaultSandboxConfig 는 기본 샌드박스 설정을 반환한다.
// os, io, debug 모듈과 loadfile, dofile 함수가 비활성화되며,
// 최대 실행 시간은 5초, 최대 메모리는 128MB이다.
func DefaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		Enabled:           true,
		DisabledModules:   []string{"os", "io", "debug"},
		DisabledFunctions: []string{"loadfile", "dofile"},
		MaxExecutionTime:  5 * time.Second,
		MaxMemoryMB:       128,
		AllowDynamicLoad:  false,
	}
}

// ApplySandbox 는 Lua 상태에 샌드박스 설정을 적용한다.
// 지정된 모듈과 함수를 전역 스코프에서 nil로 설정하여 제거한다.
// AllowDynamicLoad가 false이면 "load" 함수도 제거한다.
func ApplySandbox(L *lua.LState, config SandboxConfig) error {
	if !config.Enabled {
		return nil
	}

	// 비활성화할 모듈을 nil로 설정
	for _, mod := range config.DisabledModules {
		L.SetGlobal(mod, lua.LNil)
	}

	// 비활성화할 함수를 nil로 설정
	for _, fn := range config.DisabledFunctions {
		L.SetGlobal(fn, lua.LNil)
	}

	// AllowDynamicLoad가 false이면 load 함수도 제거
	if !config.AllowDynamicLoad {
		L.SetGlobal("load", lua.LNil)
	}

	return nil
}
