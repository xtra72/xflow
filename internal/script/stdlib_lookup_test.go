package script

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lookupDeps 는 agent/device 룩업이 설정된 StdlibDeps 를 반환한다.
func lookupDeps() StdlibDeps {
	return StdlibDeps{
		Agent: func(id string) (AgentInfo, bool) {
			if id == "a-1" {
				return AgentInfo{Type: "serial", ID: "a-1", Name: "reader"}, true
			}
			return AgentInfo{}, false
		},
		Device: func(id string) (AgentInfo, bool) {
			if id == "d-1" {
				return AgentInfo{Type: "HVACR.IDU", ID: "d-1", Name: "room1"}, true
			}
			return AgentInfo{}, false
		},
	}
}

// TestLua_AgentGet_Found 는 xflow.agent.get 이 {type,id,name} 테이블을 반환함을 검증한다.
func TestLua_AgentGet_Found(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), lookupDeps())

	res := runLua(t, L, `
		local a = xflow.agent.get("a-1")
		return a.type .. "|" .. a.id .. "|" .. a.name
	`)
	assert.Equal(t, "serial|a-1|reader", res)
}

// TestLua_AgentGet_NotFound 는 미존재 id 에 대해 nil 을 반환함을 검증한다.
func TestLua_AgentGet_NotFound(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), lookupDeps())

	res := runLua(t, L, `
		local a = xflow.agent.get("missing")
		if a == nil then return "nil" end
		return "not-nil"
	`)
	assert.Equal(t, "nil", res)
}

// TestLua_DeviceGet_Found 는 xflow.device.get 동작을 검증한다.
func TestLua_DeviceGet_Found(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), lookupDeps())

	res := runLua(t, L, `
		local d = xflow.device.get("d-1")
		return d.type .. "|" .. d.id .. "|" .. d.name
	`)
	assert.Equal(t, "HVACR.IDU|d-1|room1", res)
}

// TestLua_DeviceGet_NotFound 는 미존재 device id 에 nil 반환을 검증한다.
func TestLua_DeviceGet_NotFound(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), lookupDeps())

	res := runLua(t, L, `
		local d = xflow.device.get("nope")
		if d == nil then return "nil" end
		return "not-nil"
	`)
	assert.Equal(t, "nil", res)
}

// TestLua_AgentGet_NoLookupReturnsNil 은 룩업 미주입 시 nil 반환(에러 아님)을 검증한다.
func TestLua_AgentGet_NoLookupReturnsNil(t *testing.T) {
	// deps 없이(Agent=nil) 등록하되 EnableAgent 는 true.
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	res := runLua(t, L, `
		local a = xflow.agent.get("a-1")
		if a == nil then return "nil" end
		return "not-nil"
	`)
	assert.Equal(t, "nil", res)
}

// TestLua_AgentModuleDisabled 는 EnableAgent=false 시 xflow.agent 가 없음을 검증한다.
func TestLua_AgentModuleDisabled(t *testing.T) {
	opts := DefaultStdlibOptions()
	opts.EnableAgent = false
	L := newTestState(t, opts, lookupDeps())

	res := runLua(t, L, `
		if xflow.agent == nil then return "nil" end
		return "present"
	`)
	assert.Equal(t, "nil", res)
}

// TestDefaultStdlibOptions_LookupModules 는 기본 옵션에서 agent/device 모듈이
// 활성화됨을 검증한다.
func TestDefaultStdlibOptions_LookupModules(t *testing.T) {
	opts := DefaultStdlibOptions()
	require.True(t, opts.EnableAgent, "EnableAgent 기본 true")
	require.True(t, opts.EnableDevice, "EnableDevice 기본 true")
}
