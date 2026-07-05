package node

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// =============================================================================
// 내장 함수 레지스트리
// =============================================================================

// defaultBuiltinFuncs 는 기본 내장 함수 레지스트리를 반환한다.
// 시간, 수학, 문자열, 타입 변환 함수를 포함한다.
func defaultBuiltinFuncs() map[string]BuiltinFunc {
	funcs := map[string]BuiltinFunc{
		// 시간 함수
		"now": builtinNow,
		// 수학 함수
		"round": builtinRound,
		"floor": builtinFloor,
		"ceil":  builtinCeil,
		"abs":   builtinAbs,
		"min":   builtinMin,
		"max":   builtinMax,
		// 문자열 함수
		"upper": builtinUpper,
		"lower": builtinLower,
		"trim":  builtinTrim,
		"len":   builtinLen,
		// 타입 변환 함수
		"int":    builtinInt,
		"float":  builtinFloat,
		"string": builtinString,
		"bool":   builtinBool,
	}
	// message-slim-metadata / enrich: 룩업이 설정되어 있으면 agentInfo/deviceInfo
	// 빌트인을 추가한다(미설정 시 등록되지 않아 기존 동작 불변).
	registerLookupBuiltins(funcs)
	return funcs
}

// =============================================================================
// 시간 함수
// =============================================================================

// builtinNow 는 현재 시간을 RFC3339Nano 형식 문자열로 반환한다.
func builtinNow(args []any) (any, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("%w: now expects 0 args, got %d", ErrArgumentCount, len(args))
	}
	return time.Now().Format(time.RFC3339Nano), nil
}

// =============================================================================
// 수학 함수
// =============================================================================

// builtinRound 는 가장 가까운 정수로 반올림한다.
func builtinRound(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: round expects 1 arg, got %d", ErrArgumentCount, len(args))
	}
	val, ok := toFloat64(args[0])
	if !ok {
		return nil, fmt.Errorf("%w: round expects numeric arg, got %T", ErrTypeMismatch, args[0])
	}
	return math.Round(val), nil
}

// builtinFloor 는 내림한 값을 반환한다.
func builtinFloor(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: floor expects 1 arg, got %d", ErrArgumentCount, len(args))
	}
	val, ok := toFloat64(args[0])
	if !ok {
		return nil, fmt.Errorf("%w: floor expects numeric arg, got %T", ErrTypeMismatch, args[0])
	}
	return math.Floor(val), nil
}

// builtinCeil 은 올림한 값을 반환한다.
func builtinCeil(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: ceil expects 1 arg, got %d", ErrArgumentCount, len(args))
	}
	val, ok := toFloat64(args[0])
	if !ok {
		return nil, fmt.Errorf("%w: ceil expects numeric arg, got %T", ErrTypeMismatch, args[0])
	}
	return math.Ceil(val), nil
}

// builtinAbs 는 절대값을 반환한다.
func builtinAbs(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: abs expects 1 arg, got %d", ErrArgumentCount, len(args))
	}
	val, ok := toFloat64(args[0])
	if !ok {
		return nil, fmt.Errorf("%w: abs expects numeric arg, got %T", ErrTypeMismatch, args[0])
	}
	return math.Abs(val), nil
}

// builtinMin 은 두 숫자 중 작은 값을 반환한다.
func builtinMin(args []any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("%w: min expects 2 args, got %d", ErrArgumentCount, len(args))
	}
	a, ok := toFloat64(args[0])
	if !ok {
		return nil, fmt.Errorf("%w: min expects numeric first arg, got %T", ErrTypeMismatch, args[0])
	}
	b, ok := toFloat64(args[1])
	if !ok {
		return nil, fmt.Errorf("%w: min expects numeric second arg, got %T", ErrTypeMismatch, args[1])
	}
	return math.Min(a, b), nil
}

// builtinMax 는 두 숫자 중 큰 값을 반환한다.
func builtinMax(args []any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("%w: max expects 2 args, got %d", ErrArgumentCount, len(args))
	}
	a, ok := toFloat64(args[0])
	if !ok {
		return nil, fmt.Errorf("%w: max expects numeric first arg, got %T", ErrTypeMismatch, args[0])
	}
	b, ok := toFloat64(args[1])
	if !ok {
		return nil, fmt.Errorf("%w: max expects numeric second arg, got %T", ErrTypeMismatch, args[1])
	}
	return math.Max(a, b), nil
}

// =============================================================================
// 문자열 함수
// =============================================================================

// builtinUpper 는 문자열을 대문자로 변환한다.
func builtinUpper(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: upper expects 1 arg, got %d", ErrArgumentCount, len(args))
	}
	s, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: upper expects string arg, got %T", ErrTypeMismatch, args[0])
	}
	return strings.ToUpper(s), nil
}

// builtinLower 는 문자열을 소문자로 변환한다.
func builtinLower(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: lower expects 1 arg, got %d", ErrArgumentCount, len(args))
	}
	s, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: lower expects string arg, got %T", ErrTypeMismatch, args[0])
	}
	return strings.ToLower(s), nil
}

// builtinTrim 은 문자열 양쪽 공백을 제거한다.
func builtinTrim(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: trim expects 1 arg, got %d", ErrArgumentCount, len(args))
	}
	s, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: trim expects string arg, got %T", ErrTypeMismatch, args[0])
	}
	return strings.TrimSpace(s), nil
}

// builtinLen 은 문자열, 슬라이스, 맵의 길이를 반환한다.
func builtinLen(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: len expects 1 arg, got %d", ErrArgumentCount, len(args))
	}
	switch v := args[0].(type) {
	case string:
		return float64(len(v)), nil
	case []any:
		return float64(len(v)), nil
	case map[string]any:
		return float64(len(v)), nil
	default:
		return nil, fmt.Errorf("%w: len expects string, slice, or map, got %T", ErrTypeMismatch, args[0])
	}
}

// =============================================================================
// 타입 변환 함수
// =============================================================================

// builtinInt 는 값을 정수(truncated float64)로 변환한다.
func builtinInt(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: int expects 1 arg, got %d", ErrArgumentCount, len(args))
	}
	switch v := args[0].(type) {
	case float64:
		return math.Trunc(v), nil
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: int cannot parse string %q", ErrTypeMismatch, v)
		}
		return math.Trunc(f), nil
	case bool:
		if v {
			return float64(1), nil
		}
		return float64(0), nil
	default:
		// toFloat64 로 다른 숫자 타입 시도
		if f, ok := toFloat64(args[0]); ok {
			return math.Trunc(f), nil
		}
		return nil, fmt.Errorf("%w: int expects numeric, string, or bool, got %T", ErrTypeMismatch, args[0])
	}
}

// builtinFloat 는 값을 float64로 변환한다.
func builtinFloat(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: float expects 1 arg, got %d", ErrArgumentCount, len(args))
	}
	switch v := args[0].(type) {
	case float64:
		return v, nil
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: float cannot parse string %q", ErrTypeMismatch, v)
		}
		return f, nil
	case bool:
		if v {
			return float64(1), nil
		}
		return float64(0), nil
	default:
		// toFloat64 로 다른 숫자 타입 시도 (int, int64 등)
		if f, ok := toFloat64(args[0]); ok {
			return f, nil
		}
		return nil, fmt.Errorf("%w: float expects numeric, string, or bool, got %T", ErrTypeMismatch, args[0])
	}
}

// builtinString 은 값을 문자열로 변환한다.
func builtinString(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: string expects 1 arg, got %d", ErrArgumentCount, len(args))
	}
	if args[0] == nil {
		return "", nil
	}
	return fmt.Sprintf("%v", args[0]), nil
}

// builtinBool 은 값을 boolean으로 변환한다.
func builtinBool(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: bool expects 1 arg, got %d", ErrArgumentCount, len(args))
	}
	switch v := args[0].(type) {
	case bool:
		return v, nil
	case float64:
		return v != 0, nil
	case string:
		switch v {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return nil, fmt.Errorf("%w: bool cannot convert string %q", ErrTypeMismatch, v)
		}
	case nil:
		return false, nil
	default:
		return nil, fmt.Errorf("%w: bool expects bool, number, string, or nil, got %T", ErrTypeMismatch, args[0])
	}
}
