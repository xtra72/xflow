package node

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/message"
)

// =============================================================================
// 테스트 헬퍼
// =============================================================================

// parseValueExpr 는 값 표현식 문자열을 파싱하여 ExprNode를 반환한다.
// 내부적으로 "{ _: <expr> }" 형태로 감싼 후 파서로 처리한다.
func parseValueExpr(t *testing.T, expr string) ExprNode {
	t.Helper()
	tokens, err := exprTokenize("{ _: " + expr + " }")
	require.NoError(t, err, "tokenize failed for: %s", expr)
	ast, err := exprParse(tokens)
	require.NoError(t, err, "parse failed for: %s", expr)
	obj, ok := ast.(*ObjectNode)
	require.True(t, ok, "expected ObjectNode, got %T", ast)
	require.Len(t, obj.Fields, 1, "expected 1 field in object")
	return obj.Fields[0].Value
}

// newTestContext 는 테스트용 EvalContext를 생성한다.
func newTestContext(data map[string]any, vars map[string]any) *EvalContext {
	return NewEvalContext(data, vars, nil)
}

// =============================================================================
// LiteralNode 평가 테스트
// =============================================================================

func TestExprEval_LiteralNode(t *testing.T) {
	ctx := newTestContext(nil, nil)

	tests := []struct {
		name     string
		expr     string
		expected any
	}{
		{"float64 정수", "42", float64(42)},
		{"float64 소수", "3.14", float64(3.14)},
		{"문자열", `"hello"`, "hello"},
		{"bool true", "true", true},
		{"bool false", "false", false},
		{"null", "null", nil},
		{"음수", "-5", float64(-5)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := parseValueExpr(t, tt.expr)
			result, err := exprEval(node, ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// PathNode 평가 테스트
// =============================================================================

func TestExprEval_PathNode(t *testing.T) {
	data := map[string]any{
		"payload": map[string]any{
			"temperature": 25.0,
			"name":        "sensor-1",
			"value": []any{
				map[string]any{"timestamp": 1000, "value": 21.0},
				map[string]any{"timestamp": 2000, "value": 22.5},
				map[string]any{"timestamp": 3000, "value": 24.0},
			},
		},
		"metadata": map[string]any{
			"source": "mqtt",
		},
	}
	ctx := newTestContext(data, nil)

	t.Run("유효한 경로에서 값 추출", func(t *testing.T) {
		node := parseValueExpr(t, "$.payload.temperature")
		result, err := exprEval(node, ctx)
		require.NoError(t, err)
		assert.Equal(t, 25.0, result)
	})

	t.Run("존재하지 않는 경로는 nil 반환", func(t *testing.T) {
		node := parseValueExpr(t, "$.payload.missing")
		result, err := exprEval(node, ctx)
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("메타데이터 경로 접근", func(t *testing.T) {
		node := parseValueExpr(t, "$.metadata.source")
		result, err := exprEval(node, ctx)
		require.NoError(t, err)
		assert.Equal(t, "mqtt", result)
	})

	// 배열 인덱싱 ($.a[N].b) — chart-emitter store_value 배열 접근 시나리오.
	// 재현 케이스: transform 표현식 $.payload.value[0].value
	t.Run("배열 인덱스 후 필드 접근", func(t *testing.T) {
		node := parseValueExpr(t, "$.payload.value[0].value")
		result, err := exprEval(node, ctx)
		require.NoError(t, err)
		assert.Equal(t, 21.0, result)
	})

	t.Run("배열 인덱스 중간값 접근", func(t *testing.T) {
		node := parseValueExpr(t, "$.payload.value[2].timestamp")
		result, err := exprEval(node, ctx)
		require.NoError(t, err)
		assert.Equal(t, 3000, result)
	})

	t.Run("배열 범위 초과 인덱스는 nil 반환", func(t *testing.T) {
		node := parseValueExpr(t, "$.payload.value[99].value")
		result, err := exprEval(node, ctx)
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("와일드카드 배열 접근", func(t *testing.T) {
		node := parseValueExpr(t, "$.payload.value[*].value")
		result, err := exprEval(node, ctx)
		require.NoError(t, err)
		assert.Equal(t, []any{21.0, 22.5, 24.0}, result)
	})
}

// =============================================================================
// VarNode 평가 테스트
// =============================================================================

func TestExprEval_VarNode(t *testing.T) {
	vars := map[string]any{
		"threshold": 100,
		"label":     "test",
	}
	ctx := newTestContext(nil, vars)

	t.Run("정의된 변수 참조", func(t *testing.T) {
		node := parseValueExpr(t, "$threshold")
		result, err := exprEval(node, ctx)
		require.NoError(t, err)
		assert.Equal(t, 100, result)
	})

	t.Run("정의되지 않은 변수 참조시 ErrUndefinedVariable", func(t *testing.T) {
		node := parseValueExpr(t, "$undefined_var")
		_, err := exprEval(node, ctx)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUndefinedVariable))
	})
}

// =============================================================================
// AC-9: 산술 연산 평가 테스트
// =============================================================================

func TestExprEval_Arithmetic(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		data     map[string]any
		expected float64
	}{
		{
			name:     "AC-9-1: $._base + 2",
			expr:     "$._base + 2",
			data:     map[string]any{"_base": 8},
			expected: 10,
		},
		{
			name:     "AC-9-2: $.temp * 1.8 + 32",
			expr:     "$.temp * 1.8 + 32",
			data:     map[string]any{"temp": 25.0},
			expected: 77.0,
		},
		{
			name:     "AC-9-3: $.a / $.b",
			expr:     "$.a / $.b",
			data:     map[string]any{"a": 10, "b": 3},
			expected: float64(10) / float64(3),
		},
		{
			name:     "AC-9-4: $.x % 3",
			expr:     "$.x % 3",
			data:     map[string]any{"x": 7},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := parseValueExpr(t, tt.expr)
			ctx := newTestContext(tt.data, nil)
			result, err := exprEval(node, ctx)
			require.NoError(t, err)
			f, ok := result.(float64)
			require.True(t, ok, "expected float64, got %T", result)
			assert.InDelta(t, tt.expected, f, 1e-9)
		})
	}
}

func TestExprEval_Arithmetic_DivisionByZero(t *testing.T) {
	// AC-9-5: $.a / 0 → ErrDivisionByZero
	data := map[string]any{"a": 10}
	node := parseValueExpr(t, "$.a / 0")
	ctx := newTestContext(data, nil)
	_, err := exprEval(node, ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDivisionByZero))
}

func TestExprEval_Arithmetic_ModuloByZero(t *testing.T) {
	data := map[string]any{"a": 10}
	node := parseValueExpr(t, "$.a % 0")
	ctx := newTestContext(data, nil)
	_, err := exprEval(node, ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDivisionByZero))
}

func TestExprEval_Arithmetic_TypeMismatch(t *testing.T) {
	// AC-9-6: string + number → 문자열 결합 ("abc1")
	// + 연산자는 한쪽이 문자열이면 문자열 결합으로 동작한다.
	ctx := newTestContext(map[string]any{"name": "abc"}, nil)
	node := parseValueExpr(t, "$.name + 1")
	result, err := exprEval(node, ctx)
	require.NoError(t, err)
	assert.Equal(t, "abc1", result)
}

func TestExprEval_Arithmetic_TypeMismatch_NonPlus(t *testing.T) {
	// string - number → ErrTypeMismatch (- 연산자는 문자열 결합 불가)
	ctx := newTestContext(map[string]any{"name": "abc"}, nil)
	node := parseValueExpr(t, "$.name - 1")
	_, err := exprEval(node, ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTypeMismatch))
}

// =============================================================================
// AC-10: 문자열 연결 평가 테스트
// =============================================================================

func TestExprEval_Concat(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		data     map[string]any
		expected string
	}{
		{
			// AC-10-1: $.building & ":" & $.floor
			name:     "AC-10-1: 문자열 연결",
			expr:     `$.building & ":" & $.floor`,
			data:     map[string]any{"building": "A", "floor": "1F"},
			expected: "A:1F",
		},
		{
			// AC-10-2: $.name & null
			name:     "AC-10-2: null과 연결",
			expr:     "$.name & null",
			data:     map[string]any{"name": "test"},
			expected: "test",
		},
		{
			// AC-10-3: $.count & " items"
			name:     "AC-10-3: 숫자와 문자열 연결",
			expr:     `$.count & " items"`,
			data:     map[string]any{"count": 42},
			expected: "42 items",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := parseValueExpr(t, tt.expr)
			ctx := newTestContext(tt.data, nil)
			result, err := exprEval(node, ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// AC-11: 변수 및 테이블 룩업 평가 테스트
// =============================================================================

func TestExprEval_IndexNode(t *testing.T) {
	t.Run("AC-11-1: 리터럴 키로 테이블 룩업", func(t *testing.T) {
		vars := map[string]any{
			"address_table": map[string]any{"A:1F:L1": 0},
		}
		node := parseValueExpr(t, `$address_table["A:1F:L1"]`)
		ctx := newTestContext(nil, vars)
		result, err := exprEval(node, ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, result)
	})

	t.Run("AC-11-2: 동적 키로 테이블 룩업", func(t *testing.T) {
		data := map[string]any{"tag": "A:1F:L1"}
		vars := map[string]any{
			"address_table": map[string]any{"A:1F:L1": 0},
		}
		node := parseValueExpr(t, "$address_table[$.tag]")
		ctx := newTestContext(data, vars)
		result, err := exprEval(node, ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, result)
	})

	t.Run("AC-11-3: 정의되지 않은 변수 참조", func(t *testing.T) {
		node := parseValueExpr(t, "$undefined_var")
		ctx := newTestContext(nil, map[string]any{})
		_, err := exprEval(node, ctx)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUndefinedVariable))
	})

	t.Run("AC-11-4: 존재하지 않는 키는 nil 반환", func(t *testing.T) {
		vars := map[string]any{
			"table": map[string]any{"a": 1},
		}
		node := parseValueExpr(t, `$table["missing_key"]`)
		ctx := newTestContext(nil, vars)
		result, err := exprEval(node, ctx)
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("맵이 아닌 대상에 인덱스 접근시 ErrTypeMismatch", func(t *testing.T) {
		vars := map[string]any{
			"not_a_map": "just a string",
		}
		node := parseValueExpr(t, `$not_a_map["key"]`)
		ctx := newTestContext(nil, vars)
		_, err := exprEval(node, ctx)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTypeMismatch))
	})
}

// =============================================================================
// CallNode 평가 테스트
// =============================================================================

func TestExprEval_CallNode(t *testing.T) {
	t.Run("정의된 함수 호출", func(t *testing.T) {
		funcs := map[string]BuiltinFunc{
			"double": func(args []any) (any, error) {
				f, ok := toFloat64(args[0])
				if !ok {
					return nil, ErrTypeMismatch
				}
				return f * 2, nil
			},
		}
		ctx := NewEvalContext(nil, nil, funcs)
		node := parseValueExpr(t, "double(21)")
		result, err := exprEval(node, ctx)
		require.NoError(t, err)
		assert.Equal(t, float64(42), result)
	})

	t.Run("정의되지 않은 함수 호출시 ErrUndefinedFunction", func(t *testing.T) {
		ctx := newTestContext(nil, nil)
		node := parseValueExpr(t, "unknown_fn(1)")
		_, err := exprEval(node, ctx)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUndefinedFunction))
	})
}

// =============================================================================
// ObjectNode 평가 테스트
// =============================================================================

func TestExprEval_ObjectNode(t *testing.T) {
	data := map[string]any{
		"temperature": 25.0,
		"humidity":    60,
	}
	ctx := newTestContext(data, nil)

	// 전체 오브젝트 표현식을 파싱
	tokens, err := exprTokenize(`{ temp: $.temperature, humid: $.humidity }`)
	require.NoError(t, err)
	ast, err := exprParse(tokens)
	require.NoError(t, err)

	result, err := exprEval(ast, ctx)
	require.NoError(t, err)

	m, ok := result.(map[string]any)
	require.True(t, ok, "expected map[string]any, got %T", result)
	assert.Equal(t, 25.0, m["temp"])
	assert.Equal(t, 60, m["humid"])
}

// =============================================================================
// PathNode 경로 해석 테스트 ($.field 는 $.payload.field 축약형)
// =============================================================================

func TestExprEval_PathNode_ShorthandPayload(t *testing.T) {
	// $.temp → messageToMap 결과에서 "payload" 아래 "temp" 검색
	// messageToMap은 {"payload": {...}, "metadata": {...}, "id": ..., "timestamp": ...}를 반환
	// 따라서 $.temp 는 최상위에서 "temp"를 찾고, 없으면 nil 반환

	// exprEval에서 PathNode 처리 시 data(= messageToMap 결과) 에서 GetPath를 사용
	data := map[string]any{
		"payload": map[string]any{"temp": 25.0},
	}
	ctx := newTestContext(data, nil)

	// $.payload.temp 는 정상 동작해야 함
	node := parseValueExpr(t, "$.payload.temp")
	result, err := exprEval(node, ctx)
	require.NoError(t, err)
	assert.Equal(t, 25.0, result)
}

// =============================================================================
// compileExpressionV2 통합 테스트
// =============================================================================

func TestCompileExpressionV2_Select(t *testing.T) {
	expr := `{ celsius: $.payload.temp, offset: $.payload.temp * 1.8 + 32 }`
	fn, err := compileExpressionV2(expr, TransformModeSelect, nil)
	require.NoError(t, err)

	msg := message.New(
		message.WithPayload(message.NewPayload(map[string]any{
			"temp": 25.0,
		})),
		message.WithMetadata("source", "test"),
	)

	result, err := fn(msg)
	require.NoError(t, err)

	payload := result.Payload().ToMap()
	assert.Equal(t, 25.0, payload["celsius"])
	assert.InDelta(t, 77.0, payload["offset"].(float64), 1e-9)
	assert.Equal(t, "test", result.Metadata().All()["source"])
}

func TestCompileExpressionV2_Merge(t *testing.T) {
	expr := `{ fahrenheit: $.payload.temp * 1.8 + 32 }`
	fn, err := compileExpressionV2(expr, TransformModeMerge, nil)
	require.NoError(t, err)

	msg := message.New(
		message.WithPayload(message.NewPayload(map[string]any{
			"temp":     25.0,
			"humidity": 60,
		})),
	)

	result, err := fn(msg)
	require.NoError(t, err)

	payload := result.Payload().ToMap()
	// 원본 필드 유지
	assert.Equal(t, 25.0, payload["temp"])
	assert.Equal(t, 60, payload["humidity"])
	// 새 필드 추가
	assert.InDelta(t, 77.0, payload["fahrenheit"].(float64), 1e-9)
}

func TestCompileExpressionV2_WithVariables(t *testing.T) {
	expr := `{ address: $address_table[$.payload.tag] }`
	vars := map[string]any{
		"address_table": map[string]any{
			"A:1F:L1": 40001,
			"B:2F:R3": 40002,
		},
	}
	fn, err := compileExpressionV2(expr, TransformModeSelect, vars)
	require.NoError(t, err)

	msg := message.New(
		message.WithPayload(message.NewPayload(map[string]any{
			"tag": "A:1F:L1",
		})),
	)

	result, err := fn(msg)
	require.NoError(t, err)

	payload := result.Payload().ToMap()
	assert.Equal(t, 40001, payload["address"])
}

func TestCompileExpressionV2_InvalidExpression(t *testing.T) {
	_, err := compileExpressionV2("not an expression", TransformModeSelect, nil)
	require.Error(t, err)
}

// =============================================================================
// 에지 케이스 테스트
// =============================================================================

func TestExprEval_NilPath_ReturnsNil(t *testing.T) {
	// data가 nil이면 모든 경로가 nil을 반환해야 한다
	ctx := newTestContext(nil, nil)
	node := parseValueExpr(t, "$.some.path")
	result, err := exprEval(node, ctx)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestExprEval_ArithmeticWithNilOperand(t *testing.T) {
	// nil 값에 대한 산술 연산은 ErrTypeMismatch
	data := map[string]any{}
	ctx := newTestContext(data, nil)
	node := parseValueExpr(t, "$.missing + 1")
	_, err := exprEval(node, ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTypeMismatch))
}

func TestExprEval_ConcatWithNil(t *testing.T) {
	// nil을 빈 문자열로 변환하여 연결
	ctx := newTestContext(nil, nil)
	node := parseValueExpr(t, `"prefix" & null & "suffix"`)
	result, err := exprEval(node, ctx)
	require.NoError(t, err)
	assert.Equal(t, "prefixsuffix", result)
}

func TestExprEval_NestedArithmetic(t *testing.T) {
	data := map[string]any{"a": 2, "b": 3, "c": 4}
	ctx := newTestContext(data, nil)
	// (a + b) * c = (2 + 3) * 4 = 20
	node := parseValueExpr(t, "($.a + $.b) * $.c")
	result, err := exprEval(node, ctx)
	require.NoError(t, err)
	assert.InDelta(t, 20.0, result.(float64), 1e-9)
}

func TestExprEval_ConcatFloatConversion(t *testing.T) {
	// float64 값은 문자열로 변환될 때 적절한 형태여야 한다
	data := map[string]any{"val": 3.14}
	ctx := newTestContext(data, nil)
	node := parseValueExpr(t, `$.val & " units"`)
	result, err := exprEval(node, ctx)
	require.NoError(t, err)
	assert.Equal(t, "3.14 units", result)
}

