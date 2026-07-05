package script

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	lua "github.com/yuin/gopher-lua"
)

// ============================================================
// 테스트 헬퍼
// ============================================================

// newTestState 는 테스트용 LState를 생성하고 stdlib를 등록한다.
func newTestState(t *testing.T, opts StdlibOptions, deps StdlibDeps) *lua.LState {
	t.Helper()
	L := lua.NewState()
	t.Cleanup(func() { L.Close() })
	err := RegisterStdlib(L, opts, deps)
	require.NoError(t, err)
	return L
}

// runLua 는 Lua 코드를 실행하고 최상위 반환값을 Go 값으로 반환한다.
func runLua(t *testing.T, L *lua.LState, code string) any {
	t.Helper()
	err := L.DoString(code)
	require.NoError(t, err, "Lua execution failed: %s", code)

	ret := L.Get(-1)
	L.Pop(1)
	return FromLuaValue(ret)
}

// ============================================================
// DefaultStdlibOptions 테스트
// ============================================================

func TestDefaultStdlibOptions(t *testing.T) {
	opts := DefaultStdlibOptions()
	assert.True(t, opts.EnableJSON)
	assert.True(t, opts.EnableLog)
	assert.True(t, opts.EnableTime)
	assert.True(t, opts.EnableString)
	assert.True(t, opts.EnableMath)
	assert.True(t, opts.EnableStore)
	assert.False(t, opts.EnableEvent)
}

// ============================================================
// RegisterStdlib 테스트
// ============================================================

func TestRegisterStdlib_CreatesXflowGlobal(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	xflow := L.GetGlobal("xflow")
	assert.NotEqual(t, lua.LNil, xflow)
	_, ok := xflow.(*lua.LTable)
	assert.True(t, ok, "xflow should be a table")
}

func TestRegisterStdlib_AllModulesDisabled(t *testing.T) {
	opts := StdlibOptions{} // 모두 false
	L := newTestState(t, opts, StdlibDeps{})

	// xflow 테이블은 생성되지만 하위 모듈은 없어야 한다
	xflow := L.GetGlobal("xflow")
	assert.NotEqual(t, lua.LNil, xflow)
}

// ============================================================
// xflow.json 테스트
// ============================================================

func TestStdlib_JSON_Encode(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	result := runLua(t, L, `return xflow.json.encode({name="test", count=3})`)
	str, ok := result.(string)
	require.True(t, ok)
	// JSON 출력의 키 순서는 보장되지 않으므로 부분 문자열 확인
	assert.Contains(t, str, `"name":"test"`)
	assert.Contains(t, str, `"count":3`)
}

func TestStdlib_JSON_Encode_String(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	result := runLua(t, L, `return xflow.json.encode("hello")`)
	assert.Equal(t, `"hello"`, result)
}

func TestStdlib_JSON_Encode_Number(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	result := runLua(t, L, `return xflow.json.encode(42)`)
	assert.Equal(t, "42", result)
}

func TestStdlib_JSON_Decode(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	result := runLua(t, L, `
		local t = xflow.json.decode('{"name":"test","count":3}')
		return t.name
	`)
	assert.Equal(t, "test", result)
}

func TestStdlib_JSON_Decode_Array(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	result := runLua(t, L, `
		local t = xflow.json.decode('[1,2,3]')
		return t[2]
	`)
	assert.Equal(t, float64(2), result)
}

func TestStdlib_JSON_Decode_InvalidJSON(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	err := L.DoString(`xflow.json.decode("not json{{{")`)
	assert.Error(t, err)
}

func TestStdlib_JSON_RoundTrip(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	result := runLua(t, L, `
		local original = {name="test", value=42}
		local json_str = xflow.json.encode(original)
		local decoded = xflow.json.decode(json_str)
		return decoded.name
	`)
	assert.Equal(t, "test", result)
}

// ============================================================
// xflow.log 테스트
// ============================================================

func TestStdlib_Log_Debug(t *testing.T) {
	var mu sync.Mutex
	var logs []string
	logFn := func(level, msg string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		logs = append(logs, level+":"+msg)
	}

	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{Logger: logFn})

	err := L.DoString(`xflow.log.debug("test debug message")`)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, logs, 1)
	assert.Equal(t, "debug:test debug message", logs[0])
}

func TestStdlib_Log_AllLevels(t *testing.T) {
	var mu sync.Mutex
	var logs []string
	logFn := func(level, msg string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		logs = append(logs, level+":"+msg)
	}

	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{Logger: logFn})

	err := L.DoString(`
		xflow.log.debug("d")
		xflow.log.info("i")
		xflow.log.warn("w")
		xflow.log.error("e")
	`)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, logs, 4)
	assert.Equal(t, "debug:d", logs[0])
	assert.Equal(t, "info:i", logs[1])
	assert.Equal(t, "warn:w", logs[2])
	assert.Equal(t, "error:e", logs[3])
}

func TestStdlib_Log_NilLogger(t *testing.T) {
	// Logger가 nil이어도 패닉 없이 동작해야 한다
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	err := L.DoString(`xflow.log.info("should not panic")`)
	assert.NoError(t, err)
}

// ============================================================
// xflow.time 테스트
// ============================================================

func TestStdlib_Time_Now(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	result := runLua(t, L, `return xflow.time.now()`)
	num, ok := result.(float64)
	require.True(t, ok)
	// 유닉스 타임스탬프는 양수여야 한다
	assert.Greater(t, num, float64(0))
}

func TestStdlib_Time_NowMs(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	result := runLua(t, L, `return xflow.time.now_ms()`)
	num, ok := result.(float64)
	require.True(t, ok)
	// 밀리초 타임스탬프는 초 타임스탬프보다 훨씬 커야 한다
	assert.Greater(t, num, float64(1000000000000))
}

func TestStdlib_Time_Format(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	result := runLua(t, L, `return xflow.time.format(0, "2006-01-02")`)
	str, ok := result.(string)
	require.True(t, ok)
	assert.Equal(t, "1970-01-01", str)
}

func TestStdlib_Time_Parse(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	result := runLua(t, L, `return xflow.time.parse("2026-01-15", "2006-01-02")`)
	num, ok := result.(float64)
	require.True(t, ok)
	assert.Greater(t, num, float64(0))
}

func TestStdlib_Time_Parse_Invalid(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	err := L.DoString(`xflow.time.parse("not-a-date", "2006-01-02")`)
	assert.Error(t, err)
}

// ============================================================
// xflow.string 테스트
// ============================================================

func TestStdlib_String_Trim(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	result := runLua(t, L, `return xflow.string.trim("  hello  ")`)
	assert.Equal(t, "hello", result)
}

func TestStdlib_String_Split(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	result := runLua(t, L, `
		local parts = xflow.string.split("a,b,c", ",")
		return parts[2]
	`)
	assert.Equal(t, "b", result)
}

func TestStdlib_String_Join(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	result := runLua(t, L, `return xflow.string.join({"a","b","c"}, "-")`)
	assert.Equal(t, "a-b-c", result)
}

func TestStdlib_String_StartsWith(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	result := runLua(t, L, `return xflow.string.starts_with("hello world", "hello")`)
	assert.Equal(t, true, result)

	result = runLua(t, L, `return xflow.string.starts_with("hello world", "world")`)
	assert.Equal(t, false, result)
}

func TestStdlib_String_EndsWith(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	result := runLua(t, L, `return xflow.string.ends_with("hello world", "world")`)
	assert.Equal(t, true, result)

	result = runLua(t, L, `return xflow.string.ends_with("hello world", "hello")`)
	assert.Equal(t, false, result)
}

// ============================================================
// xflow.math 테스트
// ============================================================

func TestStdlib_Math_Round(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	tests := []struct {
		code     string
		expected float64
	}{
		{`return xflow.math.round(3.14159, 2)`, 3.14},
		{`return xflow.math.round(3.145, 2)`, 3.15},
		{`return xflow.math.round(3.5, 0)`, 4},
		{`return xflow.math.round(-2.5, 0)`, -3},
	}

	for _, tt := range tests {
		result := runLua(t, L, tt.code)
		assert.InDelta(t, tt.expected, result, 0.001, "code: %s", tt.code)
	}
}

func TestStdlib_Math_Clamp(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	tests := []struct {
		code     string
		expected float64
	}{
		{`return xflow.math.clamp(5, 0, 10)`, 5},
		{`return xflow.math.clamp(-5, 0, 10)`, 0},
		{`return xflow.math.clamp(15, 0, 10)`, 10},
	}

	for _, tt := range tests {
		result := runLua(t, L, tt.code)
		assert.Equal(t, tt.expected, result, "code: %s", tt.code)
	}
}

func TestStdlib_Math_Lerp(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	tests := []struct {
		code     string
		expected float64
	}{
		{`return xflow.math.lerp(0, 10, 0)`, 0},
		{`return xflow.math.lerp(0, 10, 1)`, 10},
		{`return xflow.math.lerp(0, 10, 0.5)`, 5},
		{`return xflow.math.lerp(10, 20, 0.25)`, 12.5},
	}

	for _, tt := range tests {
		result := runLua(t, L, tt.code)
		assert.InDelta(t, tt.expected, result, 0.001, "code: %s", tt.code)
	}
}

// ============================================================
// xflow.store 테스트
// ============================================================

func TestStdlib_Store_SetAndGet(t *testing.T) {
	store := newMockStore()
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{Store: store})

	err := L.DoString(`xflow.store.set("key1", "value1")`)
	require.NoError(t, err)

	result := runLua(t, L, `return xflow.store.get("key1")`)
	assert.Equal(t, "value1", result)
}

func TestStdlib_Store_Delete(t *testing.T) {
	store := newMockStore()
	store.data["key1"] = "value1"
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{Store: store})

	err := L.DoString(`xflow.store.delete("key1")`)
	require.NoError(t, err)

	_, exists := store.data["key1"]
	assert.False(t, exists)
}

func TestStdlib_Store_Has(t *testing.T) {
	store := newMockStore()
	store.data["key1"] = "value1"
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{Store: store})

	result := runLua(t, L, `return xflow.store.has("key1")`)
	assert.Equal(t, true, result)

	result = runLua(t, L, `return xflow.store.has("missing")`)
	assert.Equal(t, false, result)
}

func TestStdlib_Store_GetNonexistent(t *testing.T) {
	store := newMockStore()
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{Store: store})

	// 존재하지 않는 키는 nil 반환
	result := runLua(t, L, `return xflow.store.get("missing")`)
	assert.Nil(t, result)
}

func TestStdlib_Store_NilStore(t *testing.T) {
	// Follow-up A: per-execution 바인딩 모델에서 스토어 미구성은 정상 시나리오이므로
	// xflow.store 는 에러를 raise 하지 않고 nil-safe 로 동작한다:
	//   get → nil, has → false, set/delete → no-op(에러 없음).
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	// get → nil (에러 아님)
	got := runLua(t, L, `local v = xflow.store.get("key1"); if v == nil then return "nil" end; return "not-nil"`)
	assert.Equal(t, "nil", got)

	// has → false
	has := runLua(t, L, `return xflow.store.has("key1")`)
	assert.Equal(t, false, has)

	// set/delete → no-op, 에러 없이 통과
	assert.NoError(t, L.DoString(`xflow.store.set("key1", "v"); xflow.store.delete("key1")`))
}

// ============================================================
// 모듈 선택적 활성화 테스트
// ============================================================

func TestStdlib_SelectiveModules(t *testing.T) {
	opts := StdlibOptions{
		EnableJSON:   true,
		EnableLog:    false,
		EnableTime:   false,
		EnableString: false,
		EnableMath:   false,
		EnableStore:  false,
		EnableEvent:  false,
	}
	L := newTestState(t, opts, StdlibDeps{})

	// JSON은 활성화되어야 한다
	result := runLua(t, L, `return xflow.json.encode(42)`)
	assert.Equal(t, "42", result)

	// log는 비활성화되어야 한다
	err := L.DoString(`return xflow.log.info("test")`)
	assert.Error(t, err) // xflow.log가 nil이므로 에러
}

// ============================================================
// JSON encode nil 처리 테스트
// ============================================================

func TestStdlib_JSON_Encode_Nil(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	result := runLua(t, L, `return xflow.json.encode(nil)`)
	assert.Equal(t, "null", result)
}

// ============================================================
// JSON encode boolean 처리 테스트
// ============================================================

func TestStdlib_JSON_Encode_Boolean(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	result := runLua(t, L, `return xflow.json.encode(true)`)
	assert.Equal(t, "true", result)

	result = runLua(t, L, `return xflow.json.encode(false)`)
	assert.Equal(t, "false", result)
}

// ============================================================
// String split 빈 문자열 테스트
// ============================================================

func TestStdlib_String_Split_EmptyString(t *testing.T) {
	L := newTestState(t, DefaultStdlibOptions(), StdlibDeps{})

	// 빈 문자열 split 결과
	_ = strings.Split("", ",") // 참고: Go에서 빈 문자열 split은 [""]를 반환
	result := runLua(t, L, `
		local parts = xflow.string.split("", ",")
		return #parts
	`)
	// 빈 문자열 split 결과는 하나의 빈 문자열을 포함
	assert.Equal(t, float64(1), result)
}
