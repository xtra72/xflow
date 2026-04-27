package node

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustTokenize 는 테스트 헬퍼로, 토큰화에 실패하면 테스트를 중단한다.
func mustTokenize(t *testing.T, expr string) []exprToken {
	t.Helper()
	tokens, err := exprTokenize(expr)
	require.NoError(t, err, "토큰화 실패: %s", expr)
	return tokens
}

// =============================================================================
// AC-3: 기본 필드 매핑 (하위 호환성)
// =============================================================================

// TestExprParse_BasicFieldMapping 은 단순 필드 매핑을 테스트한다.
func TestExprParse_BasicFieldMapping(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantFields int
		checkAST   func(t *testing.T, node ExprNode)
	}{
		{
			name:       "단일 필드 매핑",
			input:      "{ temp: $.payload.temperature }",
			wantFields: 1,
			checkAST: func(t *testing.T, node ExprNode) {
				obj, ok := node.(*ObjectNode)
				require.True(t, ok, "최상위는 ObjectNode여야 한다")
				require.Len(t, obj.Fields, 1)

				assert.Equal(t, "temp", obj.Fields[0].Key)
				path, ok := obj.Fields[0].Value.(*PathNode)
				require.True(t, ok, "값은 PathNode여야 한다")
				assert.Equal(t, "$.payload.temperature", path.Path)
			},
		},
		{
			name:       "다중 필드 매핑",
			input:      "{ a: $.x, b: $.y }",
			wantFields: 2,
			checkAST: func(t *testing.T, node ExprNode) {
				obj, ok := node.(*ObjectNode)
				require.True(t, ok)
				require.Len(t, obj.Fields, 2)

				assert.Equal(t, "a", obj.Fields[0].Key)
				pathA, ok := obj.Fields[0].Value.(*PathNode)
				require.True(t, ok)
				assert.Equal(t, "$.x", pathA.Path)

				assert.Equal(t, "b", obj.Fields[1].Key)
				pathB, ok := obj.Fields[1].Value.(*PathNode)
				require.True(t, ok)
				assert.Equal(t, "$.y", pathB.Path)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := mustTokenize(t, tt.input)
			node, err := exprParse(tokens)
			require.NoError(t, err)
			tt.checkAST(t, node)
		})
	}
}

// =============================================================================
// AC-4: 산술 연산
// =============================================================================

// TestExprParse_ArithmeticOperations 은 산술 연산 표현식을 테스트한다.
func TestExprParse_ArithmeticOperations(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		checkAST func(t *testing.T, node ExprNode)
	}{
		{
			name:  "경로 + 숫자",
			input: "{ addr: $._base + 2 }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				require.Len(t, obj.Fields, 1)
				assert.Equal(t, "addr", obj.Fields[0].Key)

				bin, ok := obj.Fields[0].Value.(*BinaryNode)
				require.True(t, ok, "값은 BinaryNode여야 한다")
				assert.Equal(t, "+", bin.Op)

				left, ok := bin.Left.(*PathNode)
				require.True(t, ok)
				assert.Equal(t, "$._base", left.Path)

				right, ok := bin.Right.(*LiteralNode)
				require.True(t, ok)
				assert.Equal(t, float64(2), right.Value)
			},
		},
		{
			name:  "곱셈 우선순위: temp * 1.8 + 32",
			input: "{ f: $.temp * 1.8 + 32 }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				require.Len(t, obj.Fields, 1)

				// + 가 최상위: ($.temp * 1.8) + 32
				add, ok := obj.Fields[0].Value.(*BinaryNode)
				require.True(t, ok, "최상위는 + BinaryNode여야 한다")
				assert.Equal(t, "+", add.Op)

				// 좌측: $.temp * 1.8
				mul, ok := add.Left.(*BinaryNode)
				require.True(t, ok, "좌측은 * BinaryNode여야 한다")
				assert.Equal(t, "*", mul.Op)

				mulLeft, ok := mul.Left.(*PathNode)
				require.True(t, ok)
				assert.Equal(t, "$.temp", mulLeft.Path)

				mulRight, ok := mul.Right.(*LiteralNode)
				require.True(t, ok)
				assert.Equal(t, 1.8, mulRight.Value)

				// 우측: 32
				addRight, ok := add.Right.(*LiteralNode)
				require.True(t, ok)
				assert.Equal(t, float64(32), addRight.Value)
			},
		},
		{
			name:  "괄호 우선순위 오버라이드",
			input: "{ x: ($.a + $.b) * 2 }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				require.Len(t, obj.Fields, 1)

				// * 가 최상위: ($.a + $.b) * 2
				mul, ok := obj.Fields[0].Value.(*BinaryNode)
				require.True(t, ok, "최상위는 * BinaryNode여야 한다")
				assert.Equal(t, "*", mul.Op)

				// 좌측: $.a + $.b
				add, ok := mul.Left.(*BinaryNode)
				require.True(t, ok, "좌측은 + BinaryNode여야 한다")
				assert.Equal(t, "+", add.Op)

				addLeft, ok := add.Left.(*PathNode)
				require.True(t, ok)
				assert.Equal(t, "$.a", addLeft.Path)

				addRight, ok := add.Right.(*PathNode)
				require.True(t, ok)
				assert.Equal(t, "$.b", addRight.Path)

				// 우측: 2
				mulRight, ok := mul.Right.(*LiteralNode)
				require.True(t, ok)
				assert.Equal(t, float64(2), mulRight.Value)
			},
		},
		{
			name:  "모듈로 연산",
			input: "{ r: $.a % 3 }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				require.Len(t, obj.Fields, 1)

				bin, ok := obj.Fields[0].Value.(*BinaryNode)
				require.True(t, ok)
				assert.Equal(t, "%", bin.Op)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := mustTokenize(t, tt.input)
			node, err := exprParse(tokens)
			require.NoError(t, err)
			tt.checkAST(t, node)
		})
	}
}

// =============================================================================
// AC-5: 문자열 연결
// =============================================================================

// TestExprParse_StringConcatenation 은 문자열 연결 표현식을 테스트한다.
func TestExprParse_StringConcatenation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		checkAST func(t *testing.T, node ExprNode)
	}{
		{
			name:  "3항 연결 체인",
			input: `{ tag: $.building & ":" & $.floor }`,
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				require.Len(t, obj.Fields, 1)
				assert.Equal(t, "tag", obj.Fields[0].Key)

				// 좌결합: ($.building & ":") & $.floor
				outer, ok := obj.Fields[0].Value.(*ConcatNode)
				require.True(t, ok, "최상위는 ConcatNode여야 한다")

				inner, ok := outer.Left.(*ConcatNode)
				require.True(t, ok, "좌측은 ConcatNode여야 한다")

				innerLeft, ok := inner.Left.(*PathNode)
				require.True(t, ok)
				assert.Equal(t, "$.building", innerLeft.Path)

				innerRight, ok := inner.Right.(*LiteralNode)
				require.True(t, ok)
				assert.Equal(t, ":", innerRight.Value)

				outerRight, ok := outer.Right.(*PathNode)
				require.True(t, ok)
				assert.Equal(t, "$.floor", outerRight.Path)
			},
		},
		{
			name:  "경로 + 문자열 혼합 연결",
			input: `{ mixed: $.name & " #" & $.id }`,
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				require.Len(t, obj.Fields, 1)

				outer, ok := obj.Fields[0].Value.(*ConcatNode)
				require.True(t, ok)

				inner, ok := outer.Left.(*ConcatNode)
				require.True(t, ok)

				_, ok = inner.Left.(*PathNode)
				require.True(t, ok)

				lit, ok := inner.Right.(*LiteralNode)
				require.True(t, ok)
				assert.Equal(t, " #", lit.Value)

				_, ok = outer.Right.(*PathNode)
				require.True(t, ok)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := mustTokenize(t, tt.input)
			node, err := exprParse(tokens)
			require.NoError(t, err)
			tt.checkAST(t, node)
		})
	}
}

// =============================================================================
// AC-6: 변수 참조 & 테이블 룩업
// =============================================================================

// TestExprParse_VariableAndTableLookup 은 변수 참조와 테이블 룩업을 테스트한다.
func TestExprParse_VariableAndTableLookup(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		checkAST func(t *testing.T, node ExprNode)
	}{
		{
			name:  "단순 변수 참조",
			input: "{ base: $address_table }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				require.Len(t, obj.Fields, 1)

				v, ok := obj.Fields[0].Value.(*VarNode)
				require.True(t, ok, "값은 VarNode여야 한다")
				assert.Equal(t, "address_table", v.Name)
			},
		},
		{
			name:  "문자열 키 인덱스",
			input: `{ base: $address_table["A:1F:L1"] }`,
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				require.Len(t, obj.Fields, 1)

				idx, ok := obj.Fields[0].Value.(*IndexNode)
				require.True(t, ok, "값은 IndexNode여야 한다")

				target, ok := idx.Target.(*VarNode)
				require.True(t, ok)
				assert.Equal(t, "address_table", target.Name)

				key, ok := idx.Key.(*LiteralNode)
				require.True(t, ok)
				assert.Equal(t, "A:1F:L1", key.Value)
			},
		},
		{
			name:  "동적 키 인덱스 (연결 표현식)",
			input: `{ base: $address_table[$.tags.building & ":" & $.tags.floor] }`,
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				require.Len(t, obj.Fields, 1)

				idx, ok := obj.Fields[0].Value.(*IndexNode)
				require.True(t, ok, "값은 IndexNode여야 한다")

				target, ok := idx.Target.(*VarNode)
				require.True(t, ok)
				assert.Equal(t, "address_table", target.Name)

				// key 는 ConcatNode 체인이어야 한다
				outerConcat, ok := idx.Key.(*ConcatNode)
				require.True(t, ok, "키는 ConcatNode여야 한다")

				innerConcat, ok := outerConcat.Left.(*ConcatNode)
				require.True(t, ok, "내부 키도 ConcatNode여야 한다")

				_, ok = innerConcat.Left.(*PathNode)
				require.True(t, ok)

				_, ok = innerConcat.Right.(*LiteralNode)
				require.True(t, ok)

				_, ok = outerConcat.Right.(*PathNode)
				require.True(t, ok)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := mustTokenize(t, tt.input)
			node, err := exprParse(tokens)
			require.NoError(t, err)
			tt.checkAST(t, node)
		})
	}
}

// =============================================================================
// AC-7: 함수 호출
// =============================================================================

// TestExprParse_FunctionCalls 은 함수 호출 표현식을 테스트한다.
func TestExprParse_FunctionCalls(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		checkAST func(t *testing.T, node ExprNode)
	}{
		{
			name:  "인자 없는 함수 호출",
			input: "{ ts: now() }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				require.Len(t, obj.Fields, 1)

				call, ok := obj.Fields[0].Value.(*CallNode)
				require.True(t, ok, "값은 CallNode여야 한다")
				assert.Equal(t, "now", call.Name)
				assert.Len(t, call.Args, 0)
			},
		},
		{
			name:  "단일 인자 함수 호출",
			input: "{ r: round($.payload.value) }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				require.Len(t, obj.Fields, 1)

				call, ok := obj.Fields[0].Value.(*CallNode)
				require.True(t, ok)
				assert.Equal(t, "round", call.Name)
				require.Len(t, call.Args, 1)

				arg, ok := call.Args[0].(*PathNode)
				require.True(t, ok)
				assert.Equal(t, "$.payload.value", arg.Path)
			},
		},
		{
			name:  "다중 인자 함수 호출",
			input: "{ m: max($.a, $.b) }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				require.Len(t, obj.Fields, 1)

				call, ok := obj.Fields[0].Value.(*CallNode)
				require.True(t, ok)
				assert.Equal(t, "max", call.Name)
				require.Len(t, call.Args, 2)

				arg0, ok := call.Args[0].(*PathNode)
				require.True(t, ok)
				assert.Equal(t, "$.a", arg0.Path)

				arg1, ok := call.Args[1].(*PathNode)
				require.True(t, ok)
				assert.Equal(t, "$.b", arg1.Path)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := mustTokenize(t, tt.input)
			node, err := exprParse(tokens)
			require.NoError(t, err)
			tt.checkAST(t, node)
		})
	}
}

// =============================================================================
// AC-8: 중첩 객체
// =============================================================================

// TestExprParse_NestedObjects 은 중첩 객체 표현식을 테스트한다.
func TestExprParse_NestedObjects(t *testing.T) {
	input := `{
		command: "set_input",
		params: {
			area: "input_registers",
			address: $._base + 0,
			value: $.payload.object.temperature,
			data_type: "float32"
		}
	}`

	tokens := mustTokenize(t, input)
	node, err := exprParse(tokens)
	require.NoError(t, err)

	obj, ok := node.(*ObjectNode)
	require.True(t, ok)
	require.Len(t, obj.Fields, 2)

	// 첫 번째 필드: command: "set_input"
	assert.Equal(t, "command", obj.Fields[0].Key)
	lit, ok := obj.Fields[0].Value.(*LiteralNode)
	require.True(t, ok)
	assert.Equal(t, "set_input", lit.Value)

	// 두 번째 필드: params: { ... }
	assert.Equal(t, "params", obj.Fields[1].Key)
	nested, ok := obj.Fields[1].Value.(*ObjectNode)
	require.True(t, ok, "params의 값은 ObjectNode여야 한다")
	require.Len(t, nested.Fields, 4)

	// nested 필드 검증
	assert.Equal(t, "area", nested.Fields[0].Key)
	areaLit, ok := nested.Fields[0].Value.(*LiteralNode)
	require.True(t, ok)
	assert.Equal(t, "input_registers", areaLit.Value)

	assert.Equal(t, "address", nested.Fields[1].Key)
	addrBin, ok := nested.Fields[1].Value.(*BinaryNode)
	require.True(t, ok)
	assert.Equal(t, "+", addrBin.Op)

	assert.Equal(t, "value", nested.Fields[2].Key)
	valuePath, ok := nested.Fields[2].Value.(*PathNode)
	require.True(t, ok)
	assert.Equal(t, "$.payload.object.temperature", valuePath.Path)

	assert.Equal(t, "data_type", nested.Fields[3].Key)
	dtLit, ok := nested.Fields[3].Value.(*LiteralNode)
	require.True(t, ok)
	assert.Equal(t, "float32", dtLit.Value)
}

// =============================================================================
// 리터럴 값 테스트
// =============================================================================

// TestExprParse_LiteralValues 는 다양한 리터럴 값을 테스트한다.
func TestExprParse_LiteralValues(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantValue any
	}{
		{"정수 리터럴", "{ x: 42 }", float64(42)},
		{"소수 리터럴", "{ x: 3.14 }", 3.14},
		{"문자열 리터럴", `{ x: "hello" }`, "hello"},
		{"true 리터럴", "{ x: true }", true},
		{"false 리터럴", "{ x: false }", false},
		{"null 리터럴", "{ x: null }", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := mustTokenize(t, tt.input)
			node, err := exprParse(tokens)
			require.NoError(t, err)

			obj, ok := node.(*ObjectNode)
			require.True(t, ok)
			require.Len(t, obj.Fields, 1)

			lit, ok := obj.Fields[0].Value.(*LiteralNode)
			require.True(t, ok, "값은 LiteralNode여야 한다")
			assert.Equal(t, tt.wantValue, lit.Value)
		})
	}
}

// =============================================================================
// 연산자 우선순위 테스트
// =============================================================================

// TestExprParse_OperatorPrecedence 는 연산자 우선순위를 테스트한다.
func TestExprParse_OperatorPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		checkAST func(t *testing.T, node ExprNode)
	}{
		{
			name:  "& 는 + 보다 낮은 우선순위",
			input: `{ x: $.a + 1 & $.b }`,
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				// & 가 최상위: ($.a + 1) & $.b
				concat, ok := obj.Fields[0].Value.(*ConcatNode)
				require.True(t, ok, "최상위는 ConcatNode여야 한다")

				add, ok := concat.Left.(*BinaryNode)
				require.True(t, ok, "좌측은 BinaryNode여야 한다")
				assert.Equal(t, "+", add.Op)

				_, ok = concat.Right.(*PathNode)
				require.True(t, ok)
			},
		},
		{
			name:  "* 는 + 보다 높은 우선순위",
			input: "{ x: 1 + 2 * 3 }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				// + 가 최상위: 1 + (2 * 3)
				add, ok := obj.Fields[0].Value.(*BinaryNode)
				require.True(t, ok)
				assert.Equal(t, "+", add.Op)

				left, ok := add.Left.(*LiteralNode)
				require.True(t, ok)
				assert.Equal(t, float64(1), left.Value)

				mul, ok := add.Right.(*BinaryNode)
				require.True(t, ok)
				assert.Equal(t, "*", mul.Op)
			},
		},
		{
			name:  "좌결합: 1 - 2 - 3 = (1 - 2) - 3",
			input: "{ x: 1 - 2 - 3 }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				outer, ok := obj.Fields[0].Value.(*BinaryNode)
				require.True(t, ok)
				assert.Equal(t, "-", outer.Op)

				inner, ok := outer.Left.(*BinaryNode)
				require.True(t, ok)
				assert.Equal(t, "-", inner.Op)

				innerLeft, ok := inner.Left.(*LiteralNode)
				require.True(t, ok)
				assert.Equal(t, float64(1), innerLeft.Value)

				innerRight, ok := inner.Right.(*LiteralNode)
				require.True(t, ok)
				assert.Equal(t, float64(2), innerRight.Value)

				outerRight, ok := outer.Right.(*LiteralNode)
				require.True(t, ok)
				assert.Equal(t, float64(3), outerRight.Value)
			},
		},
		{
			name:  "나눗셈 좌결합: 12 / 3 / 2 = (12 / 3) / 2",
			input: "{ x: 12 / 3 / 2 }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				outer, ok := obj.Fields[0].Value.(*BinaryNode)
				require.True(t, ok)
				assert.Equal(t, "/", outer.Op)

				inner, ok := outer.Left.(*BinaryNode)
				require.True(t, ok)
				assert.Equal(t, "/", inner.Op)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := mustTokenize(t, tt.input)
			node, err := exprParse(tokens)
			require.NoError(t, err)
			tt.checkAST(t, node)
		})
	}
}

// =============================================================================
// 단항 마이너스 테스트
// =============================================================================

// TestExprParse_UnaryMinus 는 단항 마이너스 연산을 테스트한다.
func TestExprParse_UnaryMinus(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		checkAST func(t *testing.T, node ExprNode)
	}{
		{
			name:  "음수 리터럴 최적화",
			input: "{ x: -42 }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				lit, ok := obj.Fields[0].Value.(*LiteralNode)
				require.True(t, ok, "음수 리터럴은 LiteralNode로 최적화되어야 한다")
				assert.Equal(t, float64(-42), lit.Value)
			},
		},
		{
			name:  "음수 소수",
			input: "{ x: -3.14 }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				lit, ok := obj.Fields[0].Value.(*LiteralNode)
				require.True(t, ok)
				assert.Equal(t, -3.14, lit.Value)
			},
		},
		{
			name:  "경로 부정: -$.a",
			input: "{ x: -$.a }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				bin, ok := obj.Fields[0].Value.(*BinaryNode)
				require.True(t, ok, "경로 부정은 BinaryNode(0 - path)여야 한다")
				assert.Equal(t, "-", bin.Op)

				left, ok := bin.Left.(*LiteralNode)
				require.True(t, ok)
				assert.Equal(t, float64(0), left.Value)

				_, ok = bin.Right.(*PathNode)
				require.True(t, ok)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := mustTokenize(t, tt.input)
			node, err := exprParse(tokens)
			require.NoError(t, err)
			tt.checkAST(t, node)
		})
	}
}

// =============================================================================
// 복합 표현식 테스트 (mqtt-to-modbus 시나리오)
// =============================================================================

// TestExprParse_ComplexExpressions 은 복합 표현식을 테스트한다.
func TestExprParse_ComplexExpressions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		checkAST func(t *testing.T, node ExprNode)
	}{
		{
			name: "변수 룩업 + 산술",
			input: `{ base: $address_table[$.tags.building & ":" & $.tags.floor] + 2 }`,
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				require.Len(t, obj.Fields, 1)

				// 최상위: IndexNode + 2
				add, ok := obj.Fields[0].Value.(*BinaryNode)
				require.True(t, ok, "최상위는 + BinaryNode여야 한다")
				assert.Equal(t, "+", add.Op)

				// 좌측: $address_table[concat_expr]
				idx, ok := add.Left.(*IndexNode)
				require.True(t, ok, "좌측은 IndexNode여야 한다")

				target, ok := idx.Target.(*VarNode)
				require.True(t, ok)
				assert.Equal(t, "address_table", target.Name)

				// 우측: 2
				right, ok := add.Right.(*LiteralNode)
				require.True(t, ok)
				assert.Equal(t, float64(2), right.Value)
			},
		},
		{
			name:  "함수 내 산술 인자",
			input: "{ r: round($.a * 1.8 + 32) }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				call, ok := obj.Fields[0].Value.(*CallNode)
				require.True(t, ok)
				assert.Equal(t, "round", call.Name)
				require.Len(t, call.Args, 1)

				// 인자: $.a * 1.8 + 32
				add, ok := call.Args[0].(*BinaryNode)
				require.True(t, ok)
				assert.Equal(t, "+", add.Op)

				mul, ok := add.Left.(*BinaryNode)
				require.True(t, ok)
				assert.Equal(t, "*", mul.Op)
			},
		},
		{
			name: "전체 mqtt-to-modbus 변환",
			input: `{
				command: "set_input",
				params: {
					area: "input_registers",
					address: $address_table[$.tags.building & ":" & $.tags.floor & ":" & $.tags.location] + 0,
					value: $.payload.object.temperature,
					data_type: "float32"
				}
			}`,
			checkAST: func(t *testing.T, node ExprNode) {
				obj, ok := node.(*ObjectNode)
				require.True(t, ok)
				require.Len(t, obj.Fields, 2)

				assert.Equal(t, "command", obj.Fields[0].Key)
				assert.Equal(t, "params", obj.Fields[1].Key)

				nested, ok := obj.Fields[1].Value.(*ObjectNode)
				require.True(t, ok)
				require.Len(t, nested.Fields, 4)

				// address 필드: $address_table[concat] + 0
				assert.Equal(t, "address", nested.Fields[1].Key)
				addrBin, ok := nested.Fields[1].Value.(*BinaryNode)
				require.True(t, ok)
				assert.Equal(t, "+", addrBin.Op)

				idx, ok := addrBin.Left.(*IndexNode)
				require.True(t, ok)
				_, ok = idx.Target.(*VarNode)
				require.True(t, ok)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := mustTokenize(t, tt.input)
			node, err := exprParse(tokens)
			require.NoError(t, err)
			tt.checkAST(t, node)
		})
	}
}

// =============================================================================
// 에러 케이스 테스트
// =============================================================================

// TestExprParse_Errors 는 파서 에러 케이스를 테스트한다.
func TestExprParse_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"빈 입력", ""},
		{"중괄호 없음", "$.payload.temp"},
		{"닫는 중괄호 누락", "{ x: 1"},
		{"콜론 누락", "{ x 1 }"},
		{"값 누락", "{ x: }"},
		{"여는 소괄호 누락 (함수)", "{ x: now) }"},
		{"닫는 소괄호 누락 (함수)", "{ x: now( }"},
		{"닫는 대괄호 누락 (인덱스)", `{ x: $v["k" }`},
		{"잘못된 필드 키 (숫자)", "{ 42: $.a }"},
		{"잘못된 필드 키 (문자열)", `{ "key": $.a }`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := exprTokenize(tt.input)
			if err != nil {
				// 토큰화 자체가 실패하면 파서도 실패한 것으로 간주
				return
			}
			_, err = exprParse(tokens)
			assert.Error(t, err, "파싱이 에러를 반환해야 한다: %s", tt.input)
		})
	}
}

// TestExprParse_ErrorTopLevelNotObject 은 최상위가 ObjectNode가 아닌 경우를 테스트한다.
func TestExprParse_ErrorTopLevelNotObject(t *testing.T) {
	// 최상위가 숫자이면 실패해야 한다
	tokens := mustTokenize(t, "42")
	_, err := exprParse(tokens)
	assert.Error(t, err)
}

// TestExprParse_ErrorExtraTokensAfterObject 은 객체 뒤에 추가 토큰이 있는 경우를 테스트한다.
func TestExprParse_ErrorExtraTokensAfterObject(t *testing.T) {
	tokens := mustTokenize(t, "{ x: 1 } extra")
	_, err := exprParse(tokens)
	assert.Error(t, err)
}

// =============================================================================
// ExprNode 인터페이스 구현 테스트
// =============================================================================

// TestExprNode_MarkerMethod 는 모든 노드 타입이 exprNode 마커를 구현하는지 테스트한다.
func TestExprNode_MarkerMethod(t *testing.T) {
	nodes := []ExprNode{
		&ObjectNode{},
		&BinaryNode{},
		&ConcatNode{},
		&PathNode{},
		&VarNode{},
		&IndexNode{},
		&CallNode{},
		&LiteralNode{},
	}

	for _, n := range nodes {
		// exprNode() 호출이 패닉 없이 성공하면 인터페이스를 구현한 것이다
		n.exprNode()
	}
}

// =============================================================================
// 엣지 케이스 테스트
// =============================================================================

// TestExprParse_EdgeCases 는 엣지 케이스를 테스트한다.
func TestExprParse_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		checkAST func(t *testing.T, node ExprNode)
	}{
		{
			name:  "단일 필드 bool 값",
			input: "{ active: true }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				lit, ok := obj.Fields[0].Value.(*LiteralNode)
				require.True(t, ok)
				assert.Equal(t, true, lit.Value)
			},
		},
		{
			name:  "단일 필드 null 값",
			input: "{ error: null }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				lit, ok := obj.Fields[0].Value.(*LiteralNode)
				require.True(t, ok)
				assert.Nil(t, lit.Value)
			},
		},
		{
			name:  "밑줄로 시작하는 키",
			input: "{ _base: $.a }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				assert.Equal(t, "_base", obj.Fields[0].Key)
			},
		},
		{
			name:  "빈 괄호 표현식",
			input: "{ x: ($.a) }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				path, ok := obj.Fields[0].Value.(*PathNode)
				require.True(t, ok)
				assert.Equal(t, "$.a", path.Path)
			},
		},
		{
			name:  "작은따옴표 문자열 값",
			input: "{ x: 'hello' }",
			checkAST: func(t *testing.T, node ExprNode) {
				obj := node.(*ObjectNode)
				lit, ok := obj.Fields[0].Value.(*LiteralNode)
				require.True(t, ok)
				assert.Equal(t, "hello", lit.Value)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := mustTokenize(t, tt.input)
			node, err := exprParse(tokens)
			require.NoError(t, err)
			tt.checkAST(t, node)
		})
	}
}

// TestExprParse_EmptyTokenSlice 는 빈 토큰 슬라이스를 테스트한다.
func TestExprParse_EmptyTokenSlice(t *testing.T) {
	_, err := exprParse(nil)
	assert.Error(t, err)

	_, err = exprParse([]exprToken{})
	assert.Error(t, err)
}

// TestExprParse_EOFOnly 는 EOF만 있는 토큰 슬라이스를 테스트한다.
func TestExprParse_EOFOnly(t *testing.T) {
	tokens := []exprToken{{typ: exprTokenEOF, value: "", pos: 0}}
	_, err := exprParse(tokens)
	assert.Error(t, err)
}
