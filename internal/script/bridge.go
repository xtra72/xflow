package script

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode"

	lua "github.com/yuin/gopher-lua"

	"github.com/xtra/xflow/pkg/message"
)

// ToLuaValue 는 Go 값을 Lua 값으로 변환한다.
// 지원 타입: nil, bool, int/int8~64, uint/uint8~64, float32/64,
// string, []byte, []any, map[string]any, struct, message.Message
// 미지원 타입(func, chan, complex)은 LNil을 반환한다.
func ToLuaValue(L *lua.LState, value any) lua.LValue {
	if value == nil {
		return lua.LNil
	}

	// message.Message 인터페이스 우선 확인
	if msg, ok := value.(message.Message); ok {
		return MessageToLuaTable(L, msg)
	}

	switch v := value.(type) {
	case bool:
		if v {
			return lua.LTrue
		}
		return lua.LFalse

	// 정수 타입
	case int:
		return lua.LNumber(float64(v))
	case int8:
		return lua.LNumber(float64(v))
	case int16:
		return lua.LNumber(float64(v))
	case int32:
		return lua.LNumber(float64(v))
	case int64:
		return lua.LNumber(float64(v))

	// 부호 없는 정수 타입
	case uint:
		return lua.LNumber(float64(v))
	case uint8:
		return lua.LNumber(float64(v))
	case uint16:
		return lua.LNumber(float64(v))
	case uint32:
		return lua.LNumber(float64(v))
	case uint64:
		return lua.LNumber(float64(v))

	// 부동소수점 타입
	case float32:
		return lua.LNumber(float64(v))
	case float64:
		return lua.LNumber(v)

	// 문자열 타입
	case string:
		return lua.LString(v)
	case []byte:
		return lua.LString(string(v))

	// 슬라이스 타입
	case []any:
		return sliceToLuaTable(L, v)

	// 맵 타입
	case map[string]any:
		return mapToLuaTable(L, v)
	}

	// reflect로 struct 처리
	rv := reflect.ValueOf(value)
	// 포인터인 경우 역참조
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return lua.LNil
		}
		rv = rv.Elem()
	}

	if rv.Kind() == reflect.Struct {
		return structToLuaTable(L, rv)
	}

	// 미지원 타입은 LNil 반환
	return lua.LNil
}

// FromLuaValue 는 Lua 값을 Go 값으로 변환한다.
// LNil→nil, LBool→bool, LNumber→float64, LString→string
// LTable(array)→[]any, LTable(hash)→map[string]any
func FromLuaValue(value lua.LValue) any {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case *lua.LNilType:
		return nil
	case lua.LBool:
		return bool(v)
	case lua.LNumber:
		return float64(v)
	case lua.LString:
		return string(v)
	case *lua.LTable:
		return luaTableToGo(v)
	default:
		return nil
	}
}

// MessageToLuaTable 는 message.Message를 Lua 테이블로 변환한다.
// 필드: id(string), type(string), timestamp(string), payload(table), metadata(table)
//
// SPEC-MESSAGE-TYPE-001 § T3 / M4 (v1.0): 1급 Message.Type() 값을 Lua 의
// top-level `msg.type` 키로 노출. 이전 metadata.message_type 컨벤션은 폐기.
// Lua 스크립트는 `msg.type` 으로 분류 식별에 접근한다.
func MessageToLuaTable(L *lua.LState, msg message.Message) *lua.LTable {
	tbl := L.NewTable()

	tbl.RawSetString("id", lua.LString(msg.ID()))
	tbl.RawSetString("type", lua.LString(msg.Type()))
	tbl.RawSetString("timestamp", lua.LString(msg.Timestamp().Format("2006-01-02T15:04:05.999999999Z07:00")))

	// payload 변환
	payloadData := msg.Payload().ToMap()
	tbl.RawSetString("payload", mapToLuaTable(L, payloadData))

	// metadata 변환
	metaData := msg.Metadata().All()
	metaTbl := L.NewTable()
	for k, v := range metaData {
		metaTbl.RawSetString(k, lua.LString(v))
	}
	tbl.RawSetString("metadata", metaTbl)

	return tbl
}

// LuaTableToMessage 는 Lua 테이블을 message.Message로 변환한다.
// 테이블에서 type, payload, metadata를 읽어 새 Message를 생성한다.
//
// SPEC-MESSAGE-TYPE-001 § T3 / M4 (v1.0): Lua 스크립트가 `msg.type = "..."` 로
// 분류를 설정하면 1급 Message.Type() 으로 변환된다. 이전 metadata.message_type
// 컨벤션은 폐기 (Lua → Go 양방향 1급 채널 일관성).
func LuaTableToMessage(L *lua.LState, tbl *lua.LTable) (message.Message, error) {
	if tbl == nil {
		return nil, errors.New("script: cannot convert nil table to message")
	}

	var opts []message.Option

	// type 추출 (SPEC-MESSAGE-TYPE-001 § T3): top-level type 키.
	typeVal := tbl.RawGetString("type")
	if ts, ok := typeVal.(lua.LString); ok && string(ts) != "" {
		opts = append(opts, message.WithType(string(ts)))
	}

	// payload 추출
	payloadVal := tbl.RawGetString("payload")
	if payloadTbl, ok := payloadVal.(*lua.LTable); ok {
		goMap := luaTableToGoMap(payloadTbl)
		opts = append(opts, message.WithPayload(message.NewPayload(goMap)))
	}

	// metadata 추출
	metaVal := tbl.RawGetString("metadata")
	if metaTbl, ok := metaVal.(*lua.LTable); ok {
		metaTbl.ForEach(func(key, val lua.LValue) {
			if ks, ok := key.(lua.LString); ok {
				if vs, ok := val.(lua.LString); ok {
					opts = append(opts, message.WithMetadata(string(ks), string(vs)))
				}
			}
		})
	}

	return message.New(opts...), nil
}

// LuaStringToBytes 는 LString을 []byte로 변환한다.
func LuaStringToBytes(value lua.LValue) ([]byte, error) {
	str, ok := value.(lua.LString)
	if !ok {
		return nil, fmt.Errorf("script: expected LString, got %s", value.Type())
	}
	return []byte(string(str)), nil
}

// sliceToLuaTable 는 Go 슬라이스를 Lua 배열 테이블로 변환한다.
func sliceToLuaTable(L *lua.LState, slice []any) *lua.LTable {
	tbl := L.NewTable()
	for _, item := range slice {
		tbl.Append(ToLuaValue(L, item))
	}
	return tbl
}

// mapToLuaTable 는 Go 맵을 Lua 해시 테이블로 변환한다.
func mapToLuaTable(L *lua.LState, m map[string]any) *lua.LTable {
	tbl := L.NewTable()
	for k, v := range m {
		tbl.RawSetString(k, ToLuaValue(L, v))
	}
	return tbl
}

// structToLuaTable 는 Go 구조체를 Lua 테이블로 변환한다.
// exported 필드만 snake_case 키로 변환된다.
func structToLuaTable(L *lua.LState, rv reflect.Value) *lua.LTable {
	tbl := L.NewTable()
	rt := rv.Type()

	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		// exported 필드만 처리
		if !field.IsExported() {
			continue
		}
		key := toSnakeCase(field.Name)
		val := rv.Field(i).Interface()
		tbl.RawSetString(key, ToLuaValue(L, val))
	}
	return tbl
}

// luaTableToGo 는 Lua 테이블을 Go 값으로 변환한다.
// 순수 배열 테이블은 []any로, 해시/혼합 테이블은 map[string]any로 변환된다.
func luaTableToGo(tbl *lua.LTable) any {
	maxN := tbl.MaxN()
	hasHash := false

	tbl.ForEach(func(key, _ lua.LValue) {
		if _, ok := key.(lua.LNumber); !ok {
			hasHash = true
		}
	})

	// 순수 배열 테이블 (해시 없고, 연속 정수 키만 있는 경우)
	if maxN > 0 && !hasHash {
		arr := make([]any, 0, maxN)
		for i := 1; i <= maxN; i++ {
			arr = append(arr, FromLuaValue(tbl.RawGetInt(i)))
		}
		return arr
	}

	// 해시 테이블 또는 혼합 테이블
	return luaTableToGoMap(tbl)
}

// luaTableToGoMap 는 Lua 테이블을 map[string]any로 변환한다.
func luaTableToGoMap(tbl *lua.LTable) map[string]any {
	result := make(map[string]any)
	tbl.ForEach(func(key, val lua.LValue) {
		var keyStr string
		switch k := key.(type) {
		case lua.LString:
			keyStr = string(k)
		case lua.LNumber:
			keyStr = fmt.Sprintf("%v", float64(k))
		default:
			keyStr = key.String()
		}
		result[keyStr] = FromLuaValue(val)
	})
	return result
}

// toSnakeCase 는 CamelCase 또는 PascalCase 문자열을 snake_case로 변환한다.
func toSnakeCase(s string) string {
	var result strings.Builder
	runes := []rune(s)

	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 {
				// 이전 문자가 소문자이거나 다음 문자가 소문자인 경우 언더스코어 추가
				prevIsLower := unicode.IsLower(runes[i-1])
				nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
				if prevIsLower || nextIsLower {
					result.WriteRune('_')
				}
			}
			result.WriteRune(unicode.ToLower(r))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}
