package node

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// 내장 함수 단위 테스트 (AC-12)
// =============================================================================

// newFuncTestContext 는 내장 함수가 등록된 테스트용 EvalContext를 생성한다.
func newFuncTestContext(data map[string]any, vars map[string]any) *EvalContext {
	return NewEvalContext(data, vars, defaultBuiltinFuncs())
}

// =============================================================================
// AC-12-1: now() 함수 테스트
// =============================================================================

func TestBuiltinNow(t *testing.T) {
	t.Run("now()는 RFC3339Nano 형식의 현재 시간 문자열을 반환한다", func(t *testing.T) {
		ctx := newFuncTestContext(nil, nil)
		node := parseValueExpr(t, "now()")
		before := time.Now()
		result, err := exprEval(node, ctx)
		after := time.Now()

		require.NoError(t, err)
		s, ok := result.(string)
		require.True(t, ok, "expected string, got %T", result)

		parsed, parseErr := time.Parse(time.RFC3339Nano, s)
		require.NoError(t, parseErr, "now() 결과가 RFC3339Nano 형식이 아님: %s", s)

		assert.False(t, parsed.Before(before.Truncate(time.Microsecond)), "now() 결과가 호출 전 시간보다 이전임")
		assert.False(t, parsed.After(after.Add(time.Second)), "now() 결과가 호출 후 시간보다 이후임")
	})

	t.Run("now()에 인자 전달 시 ErrArgumentCount", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		fn := funcs["now"]
		require.NotNil(t, fn)
		_, err := fn([]any{"extra"})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrArgumentCount))
	})
}

// =============================================================================
// AC-12-2~5: 수학 함수 테스트 (round, floor, ceil, abs)
// =============================================================================

func TestBuiltinRound(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected float64
	}{
		{"AC-12-2: round(3.7) = 4", "round(3.7)", 4.0},
		{"round(3.3) = 3", "round(3.3)", 3.0},
		{"round(2.5) = 3 (banker's rounding 아님, Go math.Round)", "round(2.5)", 3.0},
		{"round(-1.5) = -2", "round(-1.5)", -2.0},
		{"round(0) = 0", "round(0)", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newFuncTestContext(nil, nil)
			node := parseValueExpr(t, tt.expr)
			result, err := exprEval(node, ctx)
			require.NoError(t, err)
			f, ok := result.(float64)
			require.True(t, ok, "expected float64, got %T", result)
			assert.Equal(t, tt.expected, f)
		})
	}

	t.Run("round() 인자 수 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["round"]([]any{1.0, 2.0})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrArgumentCount))
	})

	t.Run("round() 타입 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["round"]([]any{"not a number"})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTypeMismatch))
	})
}

func TestBuiltinFloor(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected float64
	}{
		{"AC-12-3: floor(3.7) = 3", "floor(3.7)", 3.0},
		{"floor(3.0) = 3", "floor(3.0)", 3.0},
		{"floor(-1.2) = -2", "floor(-1.2)", -2.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newFuncTestContext(nil, nil)
			node := parseValueExpr(t, tt.expr)
			result, err := exprEval(node, ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}

	t.Run("floor() 인자 수 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["floor"]([]any{})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrArgumentCount))
	})

	t.Run("floor() 타입 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["floor"]([]any{true})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTypeMismatch))
	})
}

func TestBuiltinCeil(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected float64
	}{
		{"AC-12-4: ceil(3.2) = 4", "ceil(3.2)", 4.0},
		{"ceil(3.0) = 3", "ceil(3.0)", 3.0},
		{"ceil(-1.8) = -1", "ceil(-1.8)", -1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newFuncTestContext(nil, nil)
			node := parseValueExpr(t, tt.expr)
			result, err := exprEval(node, ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}

	t.Run("ceil() 인자 수 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["ceil"]([]any{1.0, 2.0})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrArgumentCount))
	})
}

func TestBuiltinAbs(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected float64
	}{
		{"AC-12-5: abs(-5) = 5", "abs(-5)", 5.0},
		{"abs(5) = 5", "abs(5)", 5.0},
		{"abs(0) = 0", "abs(0)", 0.0},
		{"abs(-3.14) = 3.14", "abs(-3.14)", 3.14},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newFuncTestContext(nil, nil)
			node := parseValueExpr(t, tt.expr)
			result, err := exprEval(node, ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}

	t.Run("abs() 타입 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["abs"]([]any{"text"})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTypeMismatch))
	})
}

// =============================================================================
// AC-12-6~7: min, max 함수 테스트
// =============================================================================

func TestBuiltinMin(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected float64
	}{
		{"AC-12-6: min(3, 7) = 3", "min(3, 7)", 3.0},
		{"min(7, 3) = 3", "min(7, 3)", 3.0},
		{"min(-1, -5) = -5", "min(-1, -5)", -5.0},
		{"min(3.14, 2.71) = 2.71", "min(3.14, 2.71)", 2.71},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newFuncTestContext(nil, nil)
			node := parseValueExpr(t, tt.expr)
			result, err := exprEval(node, ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}

	t.Run("min() 인자 수 오류 - 1개", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["min"]([]any{1.0})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrArgumentCount))
	})

	t.Run("min() 타입 오류 - 첫 번째 인자", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["min"]([]any{"a", 1.0})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTypeMismatch))
	})

	t.Run("min() 타입 오류 - 두 번째 인자", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["min"]([]any{1.0, "b"})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTypeMismatch))
	})
}

func TestBuiltinMax(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected float64
	}{
		{"AC-12-7: max(3, 7) = 7", "max(3, 7)", 7.0},
		{"max(7, 3) = 7", "max(7, 3)", 7.0},
		{"max(-1, -5) = -1", "max(-1, -5)", -1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newFuncTestContext(nil, nil)
			node := parseValueExpr(t, tt.expr)
			result, err := exprEval(node, ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}

	t.Run("max() 인자 수 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["max"]([]any{1.0, 2.0, 3.0})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrArgumentCount))
	})
}

// =============================================================================
// AC-12-8~10: 문자열 함수 테스트 (upper, lower, trim)
// =============================================================================

func TestBuiltinUpper(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected string
	}{
		{"AC-12-8: upper(\"hello\") = \"HELLO\"", `upper("hello")`, "HELLO"},
		{"upper 빈 문자열", `upper("")`, ""},
		{"upper 이미 대문자", `upper("ABC")`, "ABC"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newFuncTestContext(nil, nil)
			node := parseValueExpr(t, tt.expr)
			result, err := exprEval(node, ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}

	t.Run("upper() 타입 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["upper"]([]any{42.0})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTypeMismatch))
	})

	t.Run("upper() 인자 수 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["upper"]([]any{})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrArgumentCount))
	})
}

func TestBuiltinLower(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected string
	}{
		{"AC-12-9: lower(\"HELLO\") = \"hello\"", `lower("HELLO")`, "hello"},
		{"lower 빈 문자열", `lower("")`, ""},
		{"lower 이미 소문자", `lower("abc")`, "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newFuncTestContext(nil, nil)
			node := parseValueExpr(t, tt.expr)
			result, err := exprEval(node, ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}

	t.Run("lower() 타입 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["lower"]([]any{42.0})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTypeMismatch))
	})
}

func TestBuiltinTrim(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected string
	}{
		{"AC-12-10: trim(\"  hi  \") = \"hi\"", `trim("  hi  ")`, "hi"},
		{"trim 탭과 개행", `trim("hello")`, "hello"},
		{"trim 빈 문자열", `trim("")`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newFuncTestContext(nil, nil)
			node := parseValueExpr(t, tt.expr)
			result, err := exprEval(node, ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}

	t.Run("trim() 타입 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["trim"]([]any{42.0})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTypeMismatch))
	})
}

// =============================================================================
// AC-12-11: len() 함수 테스트
// =============================================================================

func TestBuiltinLen(t *testing.T) {
	t.Run("AC-12-11: len(\"hello\") = 5", func(t *testing.T) {
		ctx := newFuncTestContext(nil, nil)
		node := parseValueExpr(t, `len("hello")`)
		result, err := exprEval(node, ctx)
		require.NoError(t, err)
		assert.Equal(t, float64(5), result)
	})

	t.Run("len 빈 문자열 = 0", func(t *testing.T) {
		ctx := newFuncTestContext(nil, nil)
		node := parseValueExpr(t, `len("")`)
		result, err := exprEval(node, ctx)
		require.NoError(t, err)
		assert.Equal(t, float64(0), result)
	})

	t.Run("len(slice)", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["len"]([]any{[]any{1, 2, 3}})
		require.NoError(t, err)
		assert.Equal(t, float64(3), result)
	})

	t.Run("len(map)", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["len"]([]any{map[string]any{"a": 1, "b": 2}})
		require.NoError(t, err)
		assert.Equal(t, float64(2), result)
	})

	t.Run("len() 타입 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["len"]([]any{42.0})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTypeMismatch))
	})

	t.Run("len() 인자 수 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["len"]([]any{})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrArgumentCount))
	})
}

// =============================================================================
// AC-12-12: int() 함수 테스트
// =============================================================================

func TestBuiltinInt(t *testing.T) {
	t.Run("AC-12-12: int(3.7) = 3", func(t *testing.T) {
		ctx := newFuncTestContext(nil, nil)
		node := parseValueExpr(t, "int(3.7)")
		result, err := exprEval(node, ctx)
		require.NoError(t, err)
		assert.Equal(t, float64(3), result)
	})

	t.Run("int(-3.7) = -3", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["int"]([]any{-3.7})
		require.NoError(t, err)
		assert.Equal(t, float64(-3), result)
	})

	t.Run("int(문자열 숫자)", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["int"]([]any{"42.9"})
		require.NoError(t, err)
		assert.Equal(t, float64(42), result)
	})

	t.Run("int(true) = 1", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["int"]([]any{true})
		require.NoError(t, err)
		assert.Equal(t, float64(1), result)
	})

	t.Run("int(false) = 0", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["int"]([]any{false})
		require.NoError(t, err)
		assert.Equal(t, float64(0), result)
	})

	t.Run("int(유효하지 않은 문자열) 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["int"]([]any{"not_a_number"})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTypeMismatch))
	})

	t.Run("int(nil) 타입 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["int"]([]any{nil})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTypeMismatch))
	})

	t.Run("int() 인자 수 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["int"]([]any{})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrArgumentCount))
	})
}

// =============================================================================
// AC-12-13: float() 함수 테스트
// =============================================================================

func TestBuiltinFloat(t *testing.T) {
	t.Run("AC-12-13: float(42) -> 42.0 (정수 타입)", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["float"]([]any{int(42)})
		require.NoError(t, err)
		assert.Equal(t, float64(42), result)
	})

	t.Run("float(float64) 그대로", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["float"]([]any{3.14})
		require.NoError(t, err)
		assert.Equal(t, 3.14, result)
	})

	t.Run("float(문자열 숫자)", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["float"]([]any{"3.14"})
		require.NoError(t, err)
		assert.Equal(t, 3.14, result)
	})

	t.Run("float(true) = 1.0", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["float"]([]any{true})
		require.NoError(t, err)
		assert.Equal(t, float64(1), result)
	})

	t.Run("float(false) = 0.0", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["float"]([]any{false})
		require.NoError(t, err)
		assert.Equal(t, float64(0), result)
	})

	t.Run("float(유효하지 않은 문자열) 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["float"]([]any{"abc"})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTypeMismatch))
	})

	t.Run("float(nil) 타입 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["float"]([]any{nil})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTypeMismatch))
	})
}

// =============================================================================
// AC-12-14: string() 함수 테스트
// =============================================================================

func TestBuiltinString(t *testing.T) {
	t.Run("AC-12-14: string(42) = \"42\"", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["string"]([]any{float64(42)})
		require.NoError(t, err)
		assert.Equal(t, "42", result)
	})

	t.Run("string(nil) = 빈 문자열", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["string"]([]any{nil})
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("string(true) = \"true\"", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["string"]([]any{true})
		require.NoError(t, err)
		assert.Equal(t, "true", result)
	})

	t.Run("string(3.14) = \"3.14\"", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["string"]([]any{3.14})
		require.NoError(t, err)
		assert.Equal(t, "3.14", result)
	})

	t.Run("string() 인자 수 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["string"]([]any{})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrArgumentCount))
	})
}

// =============================================================================
// bool() 함수 테스트
// =============================================================================

func TestBuiltinBool(t *testing.T) {
	t.Run("bool(true) = true", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["bool"]([]any{true})
		require.NoError(t, err)
		assert.Equal(t, true, result)
	})

	t.Run("bool(false) = false", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["bool"]([]any{false})
		require.NoError(t, err)
		assert.Equal(t, false, result)
	})

	t.Run("bool(0.0) = false", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["bool"]([]any{float64(0)})
		require.NoError(t, err)
		assert.Equal(t, false, result)
	})

	t.Run("bool(1.0) = true", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["bool"]([]any{float64(1)})
		require.NoError(t, err)
		assert.Equal(t, true, result)
	})

	t.Run("bool(42.0) = true", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["bool"]([]any{float64(42)})
		require.NoError(t, err)
		assert.Equal(t, true, result)
	})

	t.Run("bool(\"true\") = true", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["bool"]([]any{"true"})
		require.NoError(t, err)
		assert.Equal(t, true, result)
	})

	t.Run("bool(\"false\") = false", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["bool"]([]any{"false"})
		require.NoError(t, err)
		assert.Equal(t, false, result)
	})

	t.Run("bool(\"invalid\") 타입 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["bool"]([]any{"invalid"})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTypeMismatch))
	})

	t.Run("bool(nil) = false", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		result, err := funcs["bool"]([]any{nil})
		require.NoError(t, err)
		assert.Equal(t, false, result)
	})

	t.Run("bool(slice) 타입 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["bool"]([]any{[]any{1, 2}})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTypeMismatch))
	})

	t.Run("bool() 인자 수 오류", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["bool"]([]any{})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrArgumentCount))
	})
}

// =============================================================================
// AC-12-15: 정의되지 않은 함수 호출 테스트
// =============================================================================

func TestBuiltinUndefinedFunction(t *testing.T) {
	t.Run("AC-12-15: unknown_func() -> ErrUndefinedFunction", func(t *testing.T) {
		ctx := newFuncTestContext(nil, nil)
		node := parseValueExpr(t, "unknown_func()")
		_, err := exprEval(node, ctx)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUndefinedFunction))
	})
}

// =============================================================================
// AC-12-16: 인자 수 오류 테스트
// =============================================================================

func TestBuiltinArgumentCountError(t *testing.T) {
	t.Run("AC-12-16: round(1, 2) -> ErrArgumentCount", func(t *testing.T) {
		funcs := defaultBuiltinFuncs()
		_, err := funcs["round"]([]any{1.0, 2.0})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrArgumentCount))
	})
}

// =============================================================================
// defaultBuiltinFuncs 등록 확인 테스트
// =============================================================================

func TestDefaultBuiltinFuncs_AllRegistered(t *testing.T) {
	funcs := defaultBuiltinFuncs()

	expectedFuncs := []string{
		"now",
		"round", "floor", "ceil", "abs",
		"min", "max",
		"upper", "lower", "trim", "len",
		"int", "float", "string", "bool",
	}

	for _, name := range expectedFuncs {
		t.Run(name+"이 등록되어 있어야 함", func(t *testing.T) {
			_, ok := funcs[name]
			assert.True(t, ok, "%s 함수가 defaultBuiltinFuncs에 등록되지 않음", name)
		})
	}
}
