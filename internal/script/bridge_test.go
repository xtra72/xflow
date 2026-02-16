package script

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	lua "github.com/yuin/gopher-lua"

	"github.com/xtra/xflow/pkg/message"
)

// ============================================================
// ToLuaValue 테스트
// ============================================================

func TestToLuaValue_Nil(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	result := ToLuaValue(L, nil)
	assert.Equal(t, lua.LNil, result)
}

func TestToLuaValue_Bool(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tests := []struct {
		name   string
		input  bool
		expect lua.LBool
	}{
		{"true", true, lua.LTrue},
		{"false", false, lua.LFalse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToLuaValue(L, tt.input)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestToLuaValue_IntTypes(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tests := []struct {
		name   string
		input  any
		expect float64
	}{
		{"int", int(42), 42},
		{"int8", int8(8), 8},
		{"int16", int16(16), 16},
		{"int32", int32(32), 32},
		{"int64", int64(64), 64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToLuaValue(L, tt.input)
			num, ok := result.(lua.LNumber)
			require.True(t, ok, "expected LNumber")
			assert.Equal(t, lua.LNumber(tt.expect), num)
		})
	}
}

func TestToLuaValue_UintTypes(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tests := []struct {
		name   string
		input  any
		expect float64
	}{
		{"uint", uint(10), 10},
		{"uint8", uint8(8), 8},
		{"uint16", uint16(16), 16},
		{"uint32", uint32(32), 32},
		{"uint64", uint64(64), 64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToLuaValue(L, tt.input)
			num, ok := result.(lua.LNumber)
			require.True(t, ok, "expected LNumber")
			assert.Equal(t, lua.LNumber(tt.expect), num)
		})
	}
}

func TestToLuaValue_FloatTypes(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tests := []struct {
		name   string
		input  any
		expect float64
	}{
		{"float32", float32(3.14), float64(float32(3.14))},
		{"float64", float64(2.718), 2.718},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToLuaValue(L, tt.input)
			num, ok := result.(lua.LNumber)
			require.True(t, ok, "expected LNumber")
			assert.InDelta(t, tt.expect, float64(num), 0.001)
		})
	}
}

func TestToLuaValue_String(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	result := ToLuaValue(L, "hello world")
	str, ok := result.(lua.LString)
	require.True(t, ok)
	assert.Equal(t, lua.LString("hello world"), str)
}

func TestToLuaValue_ByteSlice(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	result := ToLuaValue(L, []byte("binary data"))
	str, ok := result.(lua.LString)
	require.True(t, ok)
	assert.Equal(t, lua.LString("binary data"), str)
}

func TestToLuaValue_Slice(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	input := []any{"a", 42, true}
	result := ToLuaValue(L, input)
	tbl, ok := result.(*lua.LTable)
	require.True(t, ok, "expected LTable")

	// Lua 배열은 1-indexed
	assert.Equal(t, lua.LString("a"), tbl.RawGetInt(1))
	assert.Equal(t, lua.LNumber(42), tbl.RawGetInt(2))
	assert.Equal(t, lua.LTrue, tbl.RawGetInt(3))
}

func TestToLuaValue_Map(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	input := map[string]any{
		"name": "test",
		"age":  30,
	}
	result := ToLuaValue(L, input)
	tbl, ok := result.(*lua.LTable)
	require.True(t, ok, "expected LTable")

	assert.Equal(t, lua.LString("test"), tbl.RawGetString("name"))
	assert.Equal(t, lua.LNumber(30), tbl.RawGetString("age"))
}

func TestToLuaValue_NestedMapAndSlice(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	input := map[string]any{
		"items": []any{"a", "b"},
		"nested": map[string]any{
			"key": "value",
		},
	}
	result := ToLuaValue(L, input)
	tbl, ok := result.(*lua.LTable)
	require.True(t, ok)

	items := tbl.RawGetString("items")
	itemsTbl, ok := items.(*lua.LTable)
	require.True(t, ok)
	assert.Equal(t, lua.LString("a"), itemsTbl.RawGetInt(1))
	assert.Equal(t, lua.LString("b"), itemsTbl.RawGetInt(2))

	nested := tbl.RawGetString("nested")
	nestedTbl, ok := nested.(*lua.LTable)
	require.True(t, ok)
	assert.Equal(t, lua.LString("value"), nestedTbl.RawGetString("key"))
}

func TestToLuaValue_Struct(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	type TestStruct struct {
		FieldName   string
		AnotherOne  int
		privateData string //nolint:unused
	}

	input := TestStruct{
		FieldName:   "hello",
		AnotherOne:  42,
		privateData: "secret",
	}
	result := ToLuaValue(L, input)
	tbl, ok := result.(*lua.LTable)
	require.True(t, ok)

	// snake_case 변환 확인
	assert.Equal(t, lua.LString("hello"), tbl.RawGetString("field_name"))
	assert.Equal(t, lua.LNumber(42), tbl.RawGetString("another_one"))
	// private 필드는 포함되지 않아야 한다
	assert.Equal(t, lua.LNil, tbl.RawGetString("private_data"))
}

func TestToLuaValue_StructPointer(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	type Ptr struct {
		Value string
	}
	input := &Ptr{Value: "ptr"}
	result := ToLuaValue(L, input)
	tbl, ok := result.(*lua.LTable)
	require.True(t, ok)
	assert.Equal(t, lua.LString("ptr"), tbl.RawGetString("value"))
}

func TestToLuaValue_UnsupportedType(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// func 타입은 LNil로 변환되어야 한다
	result := ToLuaValue(L, func() {})
	assert.Equal(t, lua.LNil, result)

	// chan 타입
	ch := make(chan int)
	result = ToLuaValue(L, ch)
	assert.Equal(t, lua.LNil, result)

	// complex 타입
	result = ToLuaValue(L, complex(1, 2))
	assert.Equal(t, lua.LNil, result)
}

func TestToLuaValue_Message(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	msg := message.New(
		message.WithPayload(message.NewPayload(map[string]any{
			"key": "value",
			"num": 42,
		})),
		message.WithMetadata("source", "test"),
	)

	result := ToLuaValue(L, msg)
	tbl, ok := result.(*lua.LTable)
	require.True(t, ok)

	// id 필드 확인
	idVal := tbl.RawGetString("id")
	assert.NotEqual(t, lua.LNil, idVal)

	// timestamp 필드 확인
	tsVal := tbl.RawGetString("timestamp")
	assert.NotEqual(t, lua.LNil, tsVal)

	// payload 테이블 확인
	payloadVal := tbl.RawGetString("payload")
	payloadTbl, ok := payloadVal.(*lua.LTable)
	require.True(t, ok)
	assert.Equal(t, lua.LString("value"), payloadTbl.RawGetString("key"))

	// metadata 테이블 확인
	metaVal := tbl.RawGetString("metadata")
	metaTbl, ok := metaVal.(*lua.LTable)
	require.True(t, ok)
	assert.Equal(t, lua.LString("test"), metaTbl.RawGetString("source"))
}

// ============================================================
// FromLuaValue 테스트
// ============================================================

func TestFromLuaValue_Nil(t *testing.T) {
	result := FromLuaValue(lua.LNil)
	assert.Nil(t, result)
}

func TestFromLuaValue_Bool(t *testing.T) {
	assert.Equal(t, true, FromLuaValue(lua.LTrue))
	assert.Equal(t, false, FromLuaValue(lua.LFalse))
}

func TestFromLuaValue_Number(t *testing.T) {
	result := FromLuaValue(lua.LNumber(3.14))
	assert.Equal(t, float64(3.14), result)
}

func TestFromLuaValue_String(t *testing.T) {
	result := FromLuaValue(lua.LString("hello"))
	assert.Equal(t, "hello", result)
}

func TestFromLuaValue_Table_Array(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tbl := L.NewTable()
	tbl.RawSetInt(1, lua.LString("a"))
	tbl.RawSetInt(2, lua.LNumber(42))
	tbl.RawSetInt(3, lua.LTrue)

	result := FromLuaValue(tbl)
	arr, ok := result.([]any)
	require.True(t, ok, "expected []any for array table")
	require.Len(t, arr, 3)
	assert.Equal(t, "a", arr[0])
	assert.Equal(t, float64(42), arr[1])
	assert.Equal(t, true, arr[2])
}

func TestFromLuaValue_Table_Hash(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tbl := L.NewTable()
	tbl.RawSetString("name", lua.LString("test"))
	tbl.RawSetString("active", lua.LTrue)

	result := FromLuaValue(tbl)
	m, ok := result.(map[string]any)
	require.True(t, ok, "expected map[string]any for hash table")
	assert.Equal(t, "test", m["name"])
	assert.Equal(t, true, m["active"])
}

func TestFromLuaValue_Table_Mixed(t *testing.T) {
	// 배열과 해시가 혼합된 경우 map[string]any로 반환
	L := lua.NewState()
	defer L.Close()

	tbl := L.NewTable()
	tbl.RawSetInt(1, lua.LString("first"))
	tbl.RawSetString("key", lua.LString("value"))

	result := FromLuaValue(tbl)
	// 혼합 테이블은 map[string]any로 반환된다
	m, ok := result.(map[string]any)
	require.True(t, ok, "expected map[string]any for mixed table")
	assert.Equal(t, "value", m["key"])
}

func TestFromLuaValue_Table_Empty(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tbl := L.NewTable()
	result := FromLuaValue(tbl)
	// 빈 테이블은 빈 map으로 반환된다
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Empty(t, m)
}

// ============================================================
// MessageToLuaTable 테스트
// ============================================================

func TestMessageToLuaTable(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	msg := message.New(
		message.WithPayload(message.NewPayload(map[string]any{
			"data": "hello",
		})),
		message.WithMetadata("env", "prod"),
	)

	tbl := MessageToLuaTable(L, msg)
	require.NotNil(t, tbl)

	// id
	idStr := tbl.RawGetString("id")
	assert.IsType(t, lua.LString(""), idStr)
	assert.NotEmpty(t, string(idStr.(lua.LString)))

	// timestamp
	tsStr := tbl.RawGetString("timestamp")
	assert.IsType(t, lua.LString(""), tsStr)

	// payload
	payload := tbl.RawGetString("payload")
	payloadTbl, ok := payload.(*lua.LTable)
	require.True(t, ok)
	assert.Equal(t, lua.LString("hello"), payloadTbl.RawGetString("data"))

	// metadata
	meta := tbl.RawGetString("metadata")
	metaTbl, ok := meta.(*lua.LTable)
	require.True(t, ok)
	assert.Equal(t, lua.LString("prod"), metaTbl.RawGetString("env"))
}

// ============================================================
// LuaTableToMessage 테스트
// ============================================================

func TestLuaTableToMessage(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tbl := L.NewTable()

	payloadTbl := L.NewTable()
	payloadTbl.RawSetString("result", lua.LString("ok"))
	payloadTbl.RawSetString("count", lua.LNumber(5))
	tbl.RawSetString("payload", payloadTbl)

	metaTbl := L.NewTable()
	metaTbl.RawSetString("source", lua.LString("lua"))
	tbl.RawSetString("metadata", metaTbl)

	msg, err := LuaTableToMessage(L, tbl)
	require.NoError(t, err)
	require.NotNil(t, msg)

	// payload 확인
	val, ok := msg.Payload().Get("result")
	require.True(t, ok)
	assert.Equal(t, "ok", val)

	val, ok = msg.Payload().Get("count")
	require.True(t, ok)
	assert.Equal(t, float64(5), val)

	// metadata 확인
	src, ok := msg.Metadata().Get("source")
	require.True(t, ok)
	assert.Equal(t, "lua", src)
}

func TestLuaTableToMessage_EmptyTable(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tbl := L.NewTable()
	msg, err := LuaTableToMessage(L, tbl)
	require.NoError(t, err)
	require.NotNil(t, msg)
	assert.NotEmpty(t, msg.ID())
}

func TestLuaTableToMessage_NilTable(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	_, err := LuaTableToMessage(L, nil)
	assert.Error(t, err)
}

// ============================================================
// LuaStringToBytes 테스트
// ============================================================

func TestLuaStringToBytes_ValidString(t *testing.T) {
	val := lua.LString("hello bytes")
	b, err := LuaStringToBytes(val)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello bytes"), b)
}

func TestLuaStringToBytes_EmptyString(t *testing.T) {
	val := lua.LString("")
	b, err := LuaStringToBytes(val)
	require.NoError(t, err)
	assert.Equal(t, []byte(""), b)
}

func TestLuaStringToBytes_NonString(t *testing.T) {
	_, err := LuaStringToBytes(lua.LNumber(42))
	assert.Error(t, err)
}

func TestLuaStringToBytes_Nil(t *testing.T) {
	_, err := LuaStringToBytes(lua.LNil)
	assert.Error(t, err)
}

// ============================================================
// snake_case 변환 테스트
// ============================================================

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"FieldName", "field_name"},
		{"AnotherOne", "another_one"},
		{"ID", "id"},
		{"HTTPServer", "http_server"},
		{"SimpleField", "simple_field"},
		{"A", "a"},
		{"Ab", "ab"},
		{"ABc", "a_bc"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, toSnakeCase(tt.input))
		})
	}
}

// ============================================================
// 동시성 안전성 테스트
// ============================================================

func TestBridge_ConcurrentAccess(t *testing.T) {
	const goroutines = 10

	done := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer func() { done <- struct{}{} }()

			L := lua.NewState()
			defer L.Close()

			// ToLuaValue 동시 호출
			input := map[string]any{
				"index": idx,
				"data":  "test",
			}
			lv := ToLuaValue(L, input)
			assert.NotEqual(t, lua.LNil, lv)

			// FromLuaValue 동시 호출
			result := FromLuaValue(lv)
			assert.NotNil(t, result)

			// MessageToLuaTable 동시 호출
			msg := message.New(
				message.WithPayload(message.NewPayload(map[string]any{"idx": idx})),
			)
			tbl := MessageToLuaTable(L, msg)
			assert.NotNil(t, tbl)

			// LuaTableToMessage 동시 호출
			_, err := LuaTableToMessage(L, tbl)
			assert.NoError(t, err)
		}(i)
	}

	// 모든 goroutine 완료 대기
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for i := 0; i < goroutines; i++ {
		select {
		case <-done:
		case <-timer.C:
			t.Fatal("timeout waiting for goroutines")
		}
	}
}
