package node

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withExprLookups 는 테스트 동안 expression 룩업을 설정하고 종료 시 원복한다.
func withExprLookups(t *testing.T, agent AgentInfoLookup, device DeviceInfoLookup) {
	t.Helper()
	prevA, prevD := currentExprLookups()
	SetExprLookups(agent, device)
	t.Cleanup(func() { SetExprLookups(prevA, prevD) })
}

// TestExprBuiltin_AgentInfo_Found 는 agentInfo(id) 가 {type,id,name} 맵을 반환함을 검증한다.
func TestExprBuiltin_AgentInfo_Found(t *testing.T) {
	withExprLookups(t,
		AgentLookupFunc(func(id string) (RegistryMeta, bool) {
			if id == "a-1" {
				return RegistryMeta{Type: "serial", ID: "a-1", Name: "reader"}, true
			}
			return RegistryMeta{}, false
		}), nil)

	funcs := defaultBuiltinFuncs()
	fn, ok := funcs["agentInfo"]
	require.True(t, ok, "agentInfo 빌트인이 등록되어야 한다")

	res, err := fn([]any{"a-1"})
	require.NoError(t, err)
	m, ok := res.(map[string]any)
	require.True(t, ok, "결과는 맵이어야 한다: %T", res)
	assert.Equal(t, "serial", m["type"])
	assert.Equal(t, "a-1", m["id"])
	assert.Equal(t, "reader", m["name"])
}

// TestExprBuiltin_AgentInfo_NotFound 는 미존재 id 에 대해 nil 을 반환함을 검증한다.
func TestExprBuiltin_AgentInfo_NotFound(t *testing.T) {
	withExprLookups(t,
		AgentLookupFunc(func(id string) (RegistryMeta, bool) { return RegistryMeta{}, false }),
		nil)

	funcs := defaultBuiltinFuncs()
	fn := funcs["agentInfo"]
	require.NotNil(t, fn)

	res, err := fn([]any{"missing"})
	require.NoError(t, err)
	assert.Nil(t, res, "not-found 는 nil 을 반환해야 한다")
}

// TestExprBuiltin_DeviceInfo_Found 는 deviceInfo(id) 동작을 검증한다.
func TestExprBuiltin_DeviceInfo_Found(t *testing.T) {
	withExprLookups(t, nil,
		DeviceLookupFunc(func(id string) (RegistryMeta, bool) {
			if id == "d-1" {
				return RegistryMeta{Type: "HVACR.IDU", ID: "d-1", Name: "room1"}, true
			}
			return RegistryMeta{}, false
		}))

	funcs := defaultBuiltinFuncs()
	fn, ok := funcs["deviceInfo"]
	require.True(t, ok)

	res, err := fn([]any{"d-1"})
	require.NoError(t, err)
	m := res.(map[string]any)
	assert.Equal(t, "HVACR.IDU", m["type"])
	assert.Equal(t, "room1", m["name"])
}

// TestExprBuiltin_NotRegisteredWhenNoLookup 는 룩업 미설정 시 빌트인이 등록되지
// 않아 기존 동작이 유지됨을 검증한다.
func TestExprBuiltin_NotRegisteredWhenNoLookup(t *testing.T) {
	withExprLookups(t, nil, nil)

	funcs := defaultBuiltinFuncs()
	if _, ok := funcs["agentInfo"]; ok {
		t.Error("룩업 미설정 시 agentInfo 는 등록되지 않아야 한다")
	}
	if _, ok := funcs["deviceInfo"]; ok {
		t.Error("룩업 미설정 시 deviceInfo 는 등록되지 않아야 한다")
	}
}

// TestExprBuiltin_AgentInfo_InExpression 은 표현식 평가 경로에서 agentInfo 가
// 실제로 동작함을 검증한다(파서→평가 통합).
func TestExprBuiltin_AgentInfo_InExpression(t *testing.T) {
	withExprLookups(t,
		AgentLookupFunc(func(id string) (RegistryMeta, bool) {
			return RegistryMeta{Type: "serial", ID: id, Name: "reader"}, true
		}), nil)

	ctx := NewEvalContext(nil, nil, defaultBuiltinFuncs())
	node := parseValueExpr(t, `agentInfo("a-1")`)
	res, err := exprEval(node, ctx)
	require.NoError(t, err)
	m, ok := res.(map[string]any)
	require.True(t, ok, "표현식 결과는 맵이어야 한다: %T", res)
	assert.Equal(t, "serial", m["type"])
	assert.Equal(t, "a-1", m["id"])
}

// TestExprBuiltin_ArgValidation 은 인자 개수/타입 검증을 확인한다.
func TestExprBuiltin_ArgValidation(t *testing.T) {
	withExprLookups(t,
		AgentLookupFunc(func(id string) (RegistryMeta, bool) { return RegistryMeta{}, false }),
		nil)
	fn := defaultBuiltinFuncs()["agentInfo"]
	require.NotNil(t, fn)

	if _, err := fn([]any{}); err == nil {
		t.Error("인자 0개는 에러여야 한다")
	}
	if _, err := fn([]any{123}); err == nil {
		t.Error("비문자열 인자는 에러여야 한다")
	}
	if res, err := fn([]any{""}); err != nil || res != nil {
		t.Errorf("빈 id 는 (nil, nil) 이어야 한다: res=%v err=%v", res, err)
	}
}
