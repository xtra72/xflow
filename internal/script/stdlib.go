package script

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// LogFunc 는 로그 콜백 함수 타입이다.
type LogFunc func(level, msg string, args ...any)

// StdlibOptions 는 xflow 표준 라이브러리 모듈 활성화 옵션이다.
type StdlibOptions struct {
	EnableJSON   bool
	EnableLog    bool
	EnableTime   bool
	EnableString bool
	EnableMath   bool
	EnableStore  bool
	EnableEvent  bool
	// EnableAgent 는 xflow.agent 모듈(agent.get) 활성화 여부이다
	// (message-slim-metadata / enrich).
	EnableAgent bool
	// EnableDevice 는 xflow.device 모듈(device.get) 활성화 여부이다.
	EnableDevice bool
}

// DefaultStdlibOptions 는 기본 표준 라이브러리 옵션을 반환한다.
// EnableEvent를 제외한 모든 모듈이 활성화된다.
func DefaultStdlibOptions() StdlibOptions {
	return StdlibOptions{
		EnableJSON:   true,
		EnableLog:    true,
		EnableTime:   true,
		EnableString: true,
		EnableMath:   true,
		EnableStore:  true,
		EnableEvent:  false,
		EnableAgent:  true,
		EnableDevice: true,
	}
}

// AgentInfo 는 xflow.agent.get / xflow.device.get 이 반환하는 정규 식별 정보이다.
// script 패키지는 internal/node 를 import 하지 않으므로(레이어링) 자체 값 타입을 둔다.
type AgentInfo struct {
	Type string
	ID   string
	Name string
}

// AgentInfoLookup 은 id 로 agent 정보를 조회하는 함수 타입이다. 매칭 없으면 ok=false.
type AgentInfoLookup func(id string) (AgentInfo, bool)

// DeviceInfoLookup 은 id 로 device 정보를 조회하는 함수 타입이다. 매칭 없으면 ok=false.
type DeviceInfoLookup func(id string) (AgentInfo, bool)

// StoreProvider 는 현재 실행(Execute)에 바인딩된 StoreAccessor 를 LState 기준으로
// 동적으로 해석하는 함수이다 (message-slim-metadata / Follow-up A).
//
// 풀링된 VM 은 여러 스크립트 노드/플로우가 공유하므로, 올바른 네임스페이스 스토어는
// 실행 단위로 달라진다. 따라서 xflow.store 모듈은 고정 Store 대신 이 provider 로
// "이번 실행의 스토어"를 매 호출 해석한다. 엔진이 실행 직전 LState 에 스토어를
// 바인딩하고 실행 후 해제하며, provider 는 그 바인딩을 조회한다. 바인딩이 없으면
// nil 을 반환하여 xflow.store 가 기존처럼 nil/false 로 graceful 하게 동작한다.
type StoreProvider func(L *lua.LState) StoreAccessor

// StdlibDeps 는 표준 라이브러리 모듈의 외부 의존성이다.
type StdlibDeps struct {
	// Store 는 고정 StoreAccessor 이다(하위 호환 — 손수 만든 LState 테스트 등).
	// StoreProvider 가 설정되면 그쪽이 우선한다.
	Store StoreAccessor
	// StoreProvider 는 실행별 동적 스토어 해석자이다(설정 시 Store 보다 우선).
	StoreProvider StoreProvider
	Logger        LogFunc
	// Agent 는 xflow.agent.get 이 사용하는 룩업이다(nil 이면 get 은 nil 반환).
	Agent AgentInfoLookup
	// Device 는 xflow.device.get 이 사용하는 룩업이다(nil 이면 get 은 nil 반환).
	Device DeviceInfoLookup
}

// RegisterStdlib 는 xflow 표준 라이브러리를 Lua 상태에 등록한다.
// xflow 글로벌 테이블 아래에 각 모듈을 서브 테이블로 등록한다.
func RegisterStdlib(L *lua.LState, opts StdlibOptions, deps StdlibDeps) error {
	// xflow 글로벌 테이블 생성
	xflow := L.GetGlobal("xflow")
	if xflow == lua.LNil {
		xflow = L.NewTable()
		L.SetGlobal("xflow", xflow)
	}
	xflowTbl := xflow.(*lua.LTable)

	if opts.EnableJSON {
		registerJSON(L, xflowTbl)
	}
	if opts.EnableLog {
		registerLog(L, xflowTbl, deps.Logger)
	}
	if opts.EnableTime {
		registerTime(L, xflowTbl)
	}
	if opts.EnableString {
		registerString(L, xflowTbl)
	}
	if opts.EnableMath {
		registerMath(L, xflowTbl)
	}
	if opts.EnableStore {
		registerStore(L, xflowTbl, deps.Store, deps.StoreProvider)
	}
	if opts.EnableAgent {
		registerAgentInfo(L, xflowTbl, deps.Agent)
	}
	if opts.EnableDevice {
		registerDeviceInfo(L, xflowTbl, deps.Device)
	}

	return nil
}

// ============================================================
// xflow.agent / xflow.device 모듈 (message-slim-metadata / enrich)
// ============================================================

// registerAgentInfo 는 xflow.agent 모듈을 등록한다.
//
// xflow.agent.get(id) -> {type=..., id=..., name=...} 테이블 또는 nil.
//
// 룩업 미주입(nil) 또는 not-found 시 nil 을 반환한다(에러 아님) — store.get 과
// 동일한 방어적 정책. id 는 항상 소스 인자 우선.
func registerAgentInfo(L *lua.LState, xflow *lua.LTable, lookup AgentInfoLookup) {
	mod := L.NewTable()
	L.SetField(mod, "get", L.NewFunction(func(L *lua.LState) int {
		L.Push(lookupInfoToLua(L, lookup, L.CheckString(1)))
		return 1
	}))
	L.SetField(xflow, "agent", mod)
}

// registerDeviceInfo 는 xflow.device 모듈을 등록한다.
//
// xflow.device.get(id) -> {type=..., id=..., name=...} 테이블 또는 nil.
func registerDeviceInfo(L *lua.LState, xflow *lua.LTable, lookup DeviceInfoLookup) {
	mod := L.NewTable()
	L.SetField(mod, "get", L.NewFunction(func(L *lua.LState) int {
		L.Push(lookupInfoToLua(L, lookup, L.CheckString(1)))
		return 1
	}))
	L.SetField(xflow, "device", mod)
}

// lookupInfoToLua 는 룩업을 수행하여 Lua 테이블 또는 nil 을 반환한다.
// lookup 이 nil 이거나 not-found 또는 빈 id 이면 lua.LNil 을 반환한다.
// 비어있지 않은 필드만 테이블에 포함하며, id 는 소스 인자를 우선 사용한다.
func lookupInfoToLua(L *lua.LState, lookup func(string) (AgentInfo, bool), id string) lua.LValue {
	if lookup == nil || id == "" {
		return lua.LNil
	}
	info, ok := lookup(id)
	if !ok {
		return lua.LNil
	}
	tbl := L.NewTable()
	if info.Type != "" {
		L.SetField(tbl, "type", lua.LString(info.Type))
	}
	L.SetField(tbl, "id", lua.LString(id))
	if info.Name != "" {
		L.SetField(tbl, "name", lua.LString(info.Name))
	}
	return tbl
}

// ============================================================
// xflow.json 모듈
// ============================================================

func registerJSON(L *lua.LState, xflow *lua.LTable) {
	mod := L.NewTable()

	// xflow.json.encode(value) -> JSON 문자열
	L.SetField(mod, "encode", L.NewFunction(func(L *lua.LState) int {
		val := L.CheckAny(1)
		goVal := FromLuaValue(val)

		data, err := json.Marshal(goVal)
		if err != nil {
			L.ArgError(1, fmt.Sprintf("json encode failed: %s", err.Error()))
			return 0
		}

		L.Push(lua.LString(string(data)))
		return 1
	}))

	// xflow.json.decode(jsonStr) -> Lua 테이블
	L.SetField(mod, "decode", L.NewFunction(func(L *lua.LState) int {
		str := L.CheckString(1)

		var goVal any
		if err := json.Unmarshal([]byte(str), &goVal); err != nil {
			L.ArgError(1, fmt.Sprintf("json decode failed: %s", err.Error()))
			return 0
		}

		L.Push(ToLuaValue(L, goVal))
		return 1
	}))

	L.SetField(xflow, "json", mod)
}

// ============================================================
// xflow.log 모듈
// ============================================================

func registerLog(L *lua.LState, xflow *lua.LTable, logger LogFunc) {
	mod := L.NewTable()

	makeLogFn := func(level string) *lua.LFunction {
		return L.NewFunction(func(L *lua.LState) int {
			msg := L.CheckString(1)
			if logger != nil {
				logger(level, msg)
			}
			return 0
		})
	}

	L.SetField(mod, "debug", makeLogFn("debug"))
	L.SetField(mod, "info", makeLogFn("info"))
	L.SetField(mod, "warn", makeLogFn("warn"))
	L.SetField(mod, "error", makeLogFn("error"))

	L.SetField(xflow, "log", mod)
}

// ============================================================
// xflow.time 모듈
// ============================================================

func registerTime(L *lua.LState, xflow *lua.LTable) {
	mod := L.NewTable()

	// xflow.time.now() -> 유닉스 타임스탬프 (float64, 초 단위)
	L.SetField(mod, "now", L.NewFunction(func(L *lua.LState) int {
		now := time.Now()
		L.Push(lua.LNumber(float64(now.Unix()) + float64(now.Nanosecond())/1e9))
		return 1
	}))

	// xflow.time.now_ms() -> 유닉스 밀리초 타임스탬프 (정수)
	L.SetField(mod, "now_ms", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(time.Now().UnixMilli()))
		return 1
	}))

	// xflow.time.format(timestamp, layout) -> 포맷된 문자열
	L.SetField(mod, "format", L.NewFunction(func(L *lua.LState) int {
		timestamp := L.CheckNumber(1)
		layout := L.CheckString(2)

		t := time.Unix(int64(timestamp), 0).UTC()
		L.Push(lua.LString(t.Format(layout)))
		return 1
	}))

	// xflow.time.parse(str, layout) -> 유닉스 타임스탬프
	L.SetField(mod, "parse", L.NewFunction(func(L *lua.LState) int {
		str := L.CheckString(1)
		layout := L.CheckString(2)

		t, err := time.Parse(layout, str)
		if err != nil {
			L.ArgError(1, fmt.Sprintf("time parse failed: %s", err.Error()))
			return 0
		}

		L.Push(lua.LNumber(t.Unix()))
		return 1
	}))

	L.SetField(xflow, "time", mod)
}

// ============================================================
// xflow.string 모듈
// ============================================================

func registerString(L *lua.LState, xflow *lua.LTable) {
	mod := L.NewTable()

	// xflow.string.trim(str) -> 양쪽 공백 제거
	L.SetField(mod, "trim", L.NewFunction(func(L *lua.LState) int {
		str := L.CheckString(1)
		L.Push(lua.LString(strings.TrimSpace(str)))
		return 1
	}))

	// xflow.string.split(str, sep) -> 분할된 문자열 테이블
	L.SetField(mod, "split", L.NewFunction(func(L *lua.LState) int {
		str := L.CheckString(1)
		sep := L.CheckString(2)

		parts := strings.Split(str, sep)
		tbl := L.NewTable()
		for _, p := range parts {
			tbl.Append(lua.LString(p))
		}
		L.Push(tbl)
		return 1
	}))

	// xflow.string.join(tbl, sep) -> 합쳐진 문자열
	L.SetField(mod, "join", L.NewFunction(func(L *lua.LState) int {
		tbl := L.CheckTable(1)
		sep := L.CheckString(2)

		var parts []string
		tbl.ForEach(func(_, val lua.LValue) {
			parts = append(parts, val.String())
		})
		L.Push(lua.LString(strings.Join(parts, sep)))
		return 1
	}))

	// xflow.string.starts_with(str, prefix) -> bool
	L.SetField(mod, "starts_with", L.NewFunction(func(L *lua.LState) int {
		str := L.CheckString(1)
		prefix := L.CheckString(2)
		if strings.HasPrefix(str, prefix) {
			L.Push(lua.LTrue)
		} else {
			L.Push(lua.LFalse)
		}
		return 1
	}))

	// xflow.string.ends_with(str, suffix) -> bool
	L.SetField(mod, "ends_with", L.NewFunction(func(L *lua.LState) int {
		str := L.CheckString(1)
		suffix := L.CheckString(2)
		if strings.HasSuffix(str, suffix) {
			L.Push(lua.LTrue)
		} else {
			L.Push(lua.LFalse)
		}
		return 1
	}))

	L.SetField(xflow, "string", mod)
}

// ============================================================
// xflow.math 모듈
// ============================================================

func registerMath(L *lua.LState, xflow *lua.LTable) {
	mod := L.NewTable()

	// xflow.math.round(num, decimals) -> 반올림된 수
	L.SetField(mod, "round", L.NewFunction(func(L *lua.LState) int {
		num := float64(L.CheckNumber(1))
		decimals := float64(L.CheckNumber(2))

		pow := math.Pow(10, decimals)
		rounded := math.Round(num*pow) / pow
		L.Push(lua.LNumber(rounded))
		return 1
	}))

	// xflow.math.clamp(num, min, max) -> 범위 내 제한된 수
	L.SetField(mod, "clamp", L.NewFunction(func(L *lua.LState) int {
		num := float64(L.CheckNumber(1))
		minVal := float64(L.CheckNumber(2))
		maxVal := float64(L.CheckNumber(3))

		result := math.Max(minVal, math.Min(maxVal, num))
		L.Push(lua.LNumber(result))
		return 1
	}))

	// xflow.math.lerp(a, b, t) -> 선형 보간 값
	L.SetField(mod, "lerp", L.NewFunction(func(L *lua.LState) int {
		a := float64(L.CheckNumber(1))
		b := float64(L.CheckNumber(2))
		t := float64(L.CheckNumber(3))

		result := a + (b-a)*t
		L.Push(lua.LNumber(result))
		return 1
	}))

	L.SetField(xflow, "math", mod)
}

// ============================================================
// xflow.store 모듈
// ============================================================

// registerStore 는 xflow.store 모듈을 등록한다.
//
// 스토어 해석(message-slim-metadata / Follow-up A): provider 가 있으면 매 호출마다
// provider(L) 로 "이번 실행에 바인딩된" 스토어를 얻고(실행별 네임스페이스 스코프),
// 없으면 고정 store 로 폴백한다. 둘 다 없거나 바인딩 미설정이면 graceful:
//   - get → nil, has → false, set/delete → no-op (에러를 raise 하지 않는다).
//
// 과거에는 store 가 nil 일 때 RaiseError 했으나, per-execution 바인딩 모델에서는
// "스토어 미구성 노드"가 정상 시나리오이므로 nil-safe 로 전환한다(스크립트 중단 방지).
func registerStore(L *lua.LState, xflow *lua.LTable, store StoreAccessor, provider StoreProvider) {
	mod := L.NewTable()

	// resolve 는 현재 호출 컨텍스트(L)의 유효한 스토어를 반환한다(없으면 nil).
	resolve := func(L *lua.LState) StoreAccessor {
		if provider != nil {
			if s := provider(L); s != nil {
				return s
			}
		}
		return store
	}

	// xflow.store.get(key) -> 값 또는 nil
	L.SetField(mod, "get", L.NewFunction(func(L *lua.LState) int {
		s := resolve(L)
		key := L.CheckString(1)
		if s == nil {
			L.Push(lua.LNil)
			return 1
		}
		val, err := s.Get(context.Background(), key)
		if err != nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(ToLuaValue(L, val))
		return 1
	}))

	// xflow.store.set(key, value)
	L.SetField(mod, "set", L.NewFunction(func(L *lua.LState) int {
		s := resolve(L)
		key := L.CheckString(1)
		val := L.CheckAny(2)
		if s == nil {
			// 바인딩 미설정 → no-op (graceful).
			return 0
		}
		goVal := FromLuaValue(val)
		if err := s.Set(context.Background(), key, goVal); err != nil {
			L.RaiseError("store set failed: %s", err.Error())
			return 0
		}
		return 0
	}))

	// xflow.store.delete(key)
	L.SetField(mod, "delete", L.NewFunction(func(L *lua.LState) int {
		s := resolve(L)
		key := L.CheckString(1)
		if s == nil {
			return 0
		}
		if err := s.Delete(context.Background(), key); err != nil {
			L.RaiseError("store delete failed: %s", err.Error())
			return 0
		}
		return 0
	}))

	// xflow.store.has(key) -> bool
	L.SetField(mod, "has", L.NewFunction(func(L *lua.LState) int {
		s := resolve(L)
		key := L.CheckString(1)
		if s == nil {
			L.Push(lua.LFalse)
			return 1
		}
		exists, err := s.Has(context.Background(), key)
		if err != nil || !exists {
			L.Push(lua.LFalse)
			return 1
		}
		L.Push(lua.LTrue)
		return 1
	}))

	L.SetField(xflow, "store", mod)
}
