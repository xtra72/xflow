package script

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// engine_stdlib_integration_test.go (message-slim-metadata / Follow-up 1) 는
// 프로덕션 스크립트 Engine 경로(createVM → RegisterStdlib → Execute)를 통해
// xflow.agent.get / xflow.device.get 이 실제로 동작함을 검증한다.
//
// 손수 만든 LState 가 아니라 DefaultScriptEngine 을 WithStdlib 로 구성하여
// VM 풀 생성 시 stdlib 가 등록되는 전체 배선을 증명한다.

// newStdlibEngine 은 agent/device 룩업이 주입된 stdlib 활성 엔진을 생성/초기화한다.
func newStdlibEngine(t *testing.T) *DefaultScriptEngine {
	t.Helper()
	deps := StdlibDeps{
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
	opts := StdlibOptions{EnableAgent: true, EnableDevice: true}

	eng := NewScriptEngine(WithStdlib(opts, deps))
	require.NoError(t, eng.Init(context.Background()))
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })
	return eng
}

// runScript 는 스크립트를 컴파일/실행하고 결과를 반환한다.
func runScript(t *testing.T, eng *DefaultScriptEngine, name, src string) any {
	t.Helper()
	id, err := eng.Compile(context.Background(), ScriptSource{Type: SourceInline, Content: src, Name: name})
	require.NoError(t, err)
	res, err := eng.Execute(context.Background(), id, nil)
	require.NoError(t, err)
	return res
}

// TestEngineStdlib_AgentGet_EndToEnd 는 실 엔진을 통해 xflow.agent.get 이
// 주입된 룩업 결과를 반환함을 검증한다(배선 증명).
func TestEngineStdlib_AgentGet_EndToEnd(t *testing.T) {
	eng := newStdlibEngine(t)

	res := runScript(t, eng, "agent_get", `
		local a = xflow.agent.get("a-1")
		return a.type .. "|" .. a.id .. "|" .. a.name
	`)
	assert.Equal(t, "serial|a-1|reader", res)
}

// TestEngineStdlib_DeviceGet_EndToEnd 는 xflow.device.get 배선을 검증한다.
func TestEngineStdlib_DeviceGet_EndToEnd(t *testing.T) {
	eng := newStdlibEngine(t)

	res := runScript(t, eng, "device_get", `
		local d = xflow.device.get("d-1")
		return d.type .. "|" .. d.id .. "|" .. d.name
	`)
	assert.Equal(t, "HVACR.IDU|d-1|room1", res)
}

// TestEngineStdlib_NotFound_EndToEnd 는 미존재 id 가 nil 로 반환됨을 검증한다.
func TestEngineStdlib_NotFound_EndToEnd(t *testing.T) {
	eng := newStdlibEngine(t)

	res := runScript(t, eng, "agent_missing", `
		local a = xflow.agent.get("nope")
		if a == nil then return "nil" end
		return "not-nil"
	`)
	assert.Equal(t, "nil", res)
}

// TestEngineStdlib_SurvivesVMRecycle 는 maxUses 재활용으로 VM 이 새로 생성되어도
// 새 VM 에 stdlib 가 재등록됨을 검증한다(createVM 이 초기 생성/재활용 모두 커버).
func TestEngineStdlib_SurvivesVMRecycle(t *testing.T) {
	deps := StdlibDeps{
		Agent: func(id string) (AgentInfo, bool) {
			return AgentInfo{Type: "serial", ID: id, Name: "reader"}, true
		},
	}
	// poolSize=1, maxVMUses=1 → 매 실행마다 VM 재활용(재생성) 발생.
	eng := NewScriptEngine(
		WithStdlib(StdlibOptions{EnableAgent: true}, deps),
		WithPoolSize(1),
		WithMaxVMUses(1),
	)
	require.NoError(t, eng.Init(context.Background()))
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })

	id, err := eng.Compile(context.Background(), ScriptSource{
		Type: SourceInline, Name: "recycle", Content: `return xflow.agent.get("x").type`,
	})
	require.NoError(t, err)

	// 여러 번 실행 → VM 재활용 후에도 xflow.agent 가 계속 동작해야 한다.
	for i := 0; i < 3; i++ {
		res, execErr := eng.Execute(context.Background(), id, nil)
		require.NoError(t, execErr, "iteration %d", i)
		assert.Equal(t, "serial", res, "iteration %d: 재활용된 VM 에도 stdlib 가 등록되어야 한다", i)
	}
}

// TestEngineStdlib_DisabledByDefault 는 WithStdlib 미사용 시 xflow 글로벌이
// 노출되지 않아 기존 동작이 보존됨을 검증한다.
func TestEngineStdlib_DisabledByDefault(t *testing.T) {
	eng := NewScriptEngine() // stdlib 옵션 없음
	require.NoError(t, eng.Init(context.Background()))
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })

	res := runScript(t, eng, "no_stdlib", `
		if xflow == nil then return "nil" end
		return "present"
	`)
	assert.Equal(t, "nil", res, "WithStdlib 미사용 시 xflow 글로벌은 없어야 한다")
}
