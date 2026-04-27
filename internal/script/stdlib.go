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
	}
}

// StdlibDeps 는 표준 라이브러리 모듈의 외부 의존성이다.
type StdlibDeps struct {
	Store  StoreAccessor
	Logger LogFunc
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
		registerStore(L, xflowTbl, deps.Store)
	}

	return nil
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

func registerStore(L *lua.LState, xflow *lua.LTable, store StoreAccessor) {
	mod := L.NewTable()

	// xflow.store.get(key) -> 값 또는 nil
	L.SetField(mod, "get", L.NewFunction(func(L *lua.LState) int {
		if store == nil {
			L.RaiseError("store accessor not configured")
			return 0
		}
		key := L.CheckString(1)

		val, err := store.Get(context.Background(), key)
		if err != nil {
			L.Push(lua.LNil)
			return 1
		}

		L.Push(ToLuaValue(L, val))
		return 1
	}))

	// xflow.store.set(key, value)
	L.SetField(mod, "set", L.NewFunction(func(L *lua.LState) int {
		if store == nil {
			L.RaiseError("store accessor not configured")
			return 0
		}
		key := L.CheckString(1)
		val := L.CheckAny(2)

		goVal := FromLuaValue(val)
		if err := store.Set(context.Background(), key, goVal); err != nil {
			L.RaiseError("store set failed: %s", err.Error())
			return 0
		}

		return 0
	}))

	// xflow.store.delete(key)
	L.SetField(mod, "delete", L.NewFunction(func(L *lua.LState) int {
		if store == nil {
			L.RaiseError("store accessor not configured")
			return 0
		}
		key := L.CheckString(1)

		if err := store.Delete(context.Background(), key); err != nil {
			L.RaiseError("store delete failed: %s", err.Error())
			return 0
		}

		return 0
	}))

	// xflow.store.has(key) -> bool
	L.SetField(mod, "has", L.NewFunction(func(L *lua.LState) int {
		if store == nil {
			L.RaiseError("store accessor not configured")
			return 0
		}
		key := L.CheckString(1)

		exists, err := store.Has(context.Background(), key)
		if err != nil {
			L.Push(lua.LFalse)
			return 1
		}

		if exists {
			L.Push(lua.LTrue)
		} else {
			L.Push(lua.LFalse)
		}
		return 1
	}))

	L.SetField(xflow, "store", mod)
}
