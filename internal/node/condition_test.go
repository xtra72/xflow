package node

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/message"
)

// =============================================================================
// 렉서(tokenize) 테스트
// =============================================================================

func TestTokenize_토큰타입별인식(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTyps []condTokenType
		wantVals []string
	}{
		{
			name:     "JSONPath 토큰",
			input:    "$.payload.temperature",
			wantTyps: []condTokenType{condTokenPath, condTokenEOF},
			wantVals: []string{"$.payload.temperature", ""},
		},
		{
			name:     "숫자 토큰 (정수)",
			input:    "42",
			wantTyps: []condTokenType{condTokenNumber, condTokenEOF},
			wantVals: []string{"42", ""},
		},
		{
			name:     "숫자 토큰 (소수)",
			input:    "3.14",
			wantTyps: []condTokenType{condTokenNumber, condTokenEOF},
			wantVals: []string{"3.14", ""},
		},
		{
			name:     "숫자 토큰 (음수)",
			input:    "-40.5",
			wantTyps: []condTokenType{condTokenNumber, condTokenEOF},
			wantVals: []string{"-40.5", ""},
		},
		{
			name:     "문자열 토큰 (작은따옴표)",
			input:    "'active'",
			wantTyps: []condTokenType{condTokenString, condTokenEOF},
			wantVals: []string{"active", ""},
		},
		{
			name:     "문자열 토큰 (큰따옴표)",
			input:    `"active"`,
			wantTyps: []condTokenType{condTokenString, condTokenEOF},
			wantVals: []string{"active", ""},
		},
		{
			name:     "bool 토큰 (true)",
			input:    "true",
			wantTyps: []condTokenType{condTokenBool, condTokenEOF},
			wantVals: []string{"true", ""},
		},
		{
			name:     "bool 토큰 (false)",
			input:    "false",
			wantTyps: []condTokenType{condTokenBool, condTokenEOF},
			wantVals: []string{"false", ""},
		},
		{
			name:     "null 토큰",
			input:    "null",
			wantTyps: []condTokenType{condTokenNull, condTokenEOF},
			wantVals: []string{"null", ""},
		},
		{
			name:     "비교 연산자 >=",
			input:    ">=",
			wantTyps: []condTokenType{condTokenCompareOp, condTokenEOF},
			wantVals: []string{">=", ""},
		},
		{
			name:     "비교 연산자 <=",
			input:    "<=",
			wantTyps: []condTokenType{condTokenCompareOp, condTokenEOF},
			wantVals: []string{"<=", ""},
		},
		{
			name:     "비교 연산자 ==",
			input:    "==",
			wantTyps: []condTokenType{condTokenCompareOp, condTokenEOF},
			wantVals: []string{"==", ""},
		},
		{
			name:     "비교 연산자 !=",
			input:    "!=",
			wantTyps: []condTokenType{condTokenCompareOp, condTokenEOF},
			wantVals: []string{"!=", ""},
		},
		{
			name:     "비교 연산자 >",
			input:    ">",
			wantTyps: []condTokenType{condTokenCompareOp, condTokenEOF},
			wantVals: []string{">", ""},
		},
		{
			name:     "비교 연산자 <",
			input:    "<",
			wantTyps: []condTokenType{condTokenCompareOp, condTokenEOF},
			wantVals: []string{"<", ""},
		},
		{
			name:     "논리 연산자 &&",
			input:    "&&",
			wantTyps: []condTokenType{condTokenLogicalOp, condTokenEOF},
			wantVals: []string{"&&", ""},
		},
		{
			name:     "논리 연산자 ||",
			input:    "||",
			wantTyps: []condTokenType{condTokenLogicalOp, condTokenEOF},
			wantVals: []string{"||", ""},
		},
		{
			name:     "NOT 연산자",
			input:    "!exists",
			wantTyps: []condTokenType{condTokenNot, condTokenFunc, condTokenEOF},
			wantVals: []string{"!", "exists", ""},
		},
		{
			name:     "괄호",
			input:    "()",
			wantTyps: []condTokenType{condTokenLParen, condTokenRParen, condTokenEOF},
			wantVals: []string{"(", ")", ""},
		},
		{
			name:     "exists 함수",
			input:    "exists",
			wantTyps: []condTokenType{condTokenFunc, condTokenEOF},
			wantVals: []string{"exists", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := tokenize(tt.input)
			require.NoError(t, err, "tokenize(%q)", tt.input)
			require.Len(t, tokens, len(tt.wantTyps), "토큰 개수 불일치")

			for i, tok := range tokens {
				assert.Equal(t, tt.wantTyps[i], tok.typ, "토큰[%d] 타입 불일치", i)
				assert.Equal(t, tt.wantVals[i], tok.value, "토큰[%d] 값 불일치", i)
			}
		})
	}
}

func TestTokenize_복합표현식(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTyps []condTokenType
	}{
		{
			name:  "단순 비교",
			input: "$.payload.temperature >= 30",
			wantTyps: []condTokenType{
				condTokenPath, condTokenCompareOp, condTokenNumber, condTokenEOF,
			},
		},
		{
			name:  "AND 표현식",
			input: "$.payload.temperature >= -40 && $.payload.temperature <= 150",
			wantTyps: []condTokenType{
				condTokenPath, condTokenCompareOp, condTokenNumber,
				condTokenLogicalOp,
				condTokenPath, condTokenCompareOp, condTokenNumber,
				condTokenEOF,
			},
		},
		{
			name:  "NOT + exists + AND",
			input: "!exists($.payload.error) && $.payload.value > 0",
			wantTyps: []condTokenType{
				condTokenNot, condTokenFunc, condTokenLParen, condTokenPath, condTokenRParen,
				condTokenLogicalOp,
				condTokenPath, condTokenCompareOp, condTokenNumber,
				condTokenEOF,
			},
		},
		{
			name:  "괄호 그룹 + 논리 연산",
			input: "($.a > 1 || $.b > 2) && $.c == true",
			wantTyps: []condTokenType{
				condTokenLParen,
				condTokenPath, condTokenCompareOp, condTokenNumber,
				condTokenLogicalOp,
				condTokenPath, condTokenCompareOp, condTokenNumber,
				condTokenRParen,
				condTokenLogicalOp,
				condTokenPath, condTokenCompareOp, condTokenBool,
				condTokenEOF,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := tokenize(tt.input)
			require.NoError(t, err)
			require.Len(t, tokens, len(tt.wantTyps))
			for i, tok := range tokens {
				assert.Equal(t, tt.wantTyps[i], tok.typ, "토큰[%d] 타입 불일치 (input=%q)", i, tt.input)
			}
		})
	}
}

func TestTokenize_공백처리(t *testing.T) {
	tokens, err := tokenize("  $.a  >=  30  ")
	require.NoError(t, err)
	require.Len(t, tokens, 4) // path, op, number, EOF
	assert.Equal(t, condTokenPath, tokens[0].typ)
	assert.Equal(t, condTokenCompareOp, tokens[1].typ)
	assert.Equal(t, condTokenNumber, tokens[2].typ)
	assert.Equal(t, condTokenEOF, tokens[3].typ)
}

func TestTokenize_에러케이스(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"인식불가 문자", "$.a @ 30"},
		{"종결되지 않은 작은따옴표 문자열", "'unterminated"},
		{"종결되지 않은 큰따옴표 문자열", `"unterminated`},
		{"단독 & 기호", "$.a & $.b"},
		{"단독 | 기호", "$.a | $.b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tokenize(tt.input)
			assert.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidExpression),
				"에러가 ErrInvalidExpression을 래핑해야 한다: %v", err)
		})
	}
}

func TestTokenize_JSONPath_배열인덱스(t *testing.T) {
	tokens, err := tokenize("$.items[0].name == 'test'")
	require.NoError(t, err)
	assert.Equal(t, "$.items[0].name", tokens[0].value)
	assert.Equal(t, condTokenPath, tokens[0].typ)
}

func TestTokenize_음수와빼기구분(t *testing.T) {
	// 음수: 비교 연산자 다음의 -는 음수
	tokens, err := tokenize("$.a >= -40.5")
	require.NoError(t, err)
	require.Len(t, tokens, 4) // path, op, number, EOF
	assert.Equal(t, "-40.5", tokens[2].value)
	assert.Equal(t, condTokenNumber, tokens[2].typ)
}

// =============================================================================
// 파서(parseCondition) 테스트
// =============================================================================

func TestParseCondition_단순비교(t *testing.T) {
	tokens, err := tokenize("$.payload.temp >= 30")
	require.NoError(t, err)

	node, err := parseCondition(tokens)
	require.NoError(t, err)
	require.NotNil(t, node)

	// comparisonNode 타입 확인
	cmp, ok := node.(*comparisonNode)
	require.True(t, ok, "comparisonNode 타입이어야 한다")
	assert.Equal(t, "$.payload.temp", cmp.path)
	assert.Equal(t, ">=", cmp.op)
	assert.Equal(t, float64(30), cmp.literal)
}

func TestParseCondition_AND표현식(t *testing.T) {
	tokens, err := tokenize("$.a > 1 && $.b < 10")
	require.NoError(t, err)

	node, err := parseCondition(tokens)
	require.NoError(t, err)

	logical, ok := node.(*logicalNode)
	require.True(t, ok, "logicalNode 타입이어야 한다")
	assert.Equal(t, "&&", logical.op)
}

func TestParseCondition_OR표현식(t *testing.T) {
	tokens, err := tokenize("$.a == 'x' || $.b == 'y'")
	require.NoError(t, err)

	node, err := parseCondition(tokens)
	require.NoError(t, err)

	logical, ok := node.(*logicalNode)
	require.True(t, ok, "logicalNode 타입이어야 한다")
	assert.Equal(t, "||", logical.op)
}

func TestParseCondition_NOT표현식(t *testing.T) {
	tokens, err := tokenize("!exists($.a)")
	require.NoError(t, err)

	node, err := parseCondition(tokens)
	require.NoError(t, err)

	notN, ok := node.(*notNode)
	require.True(t, ok, "notNode 타입이어야 한다")
	_, isExists := notN.expr.(*existsNode)
	assert.True(t, isExists, "내부 노드가 existsNode이어야 한다")
}

func TestParseCondition_괄호표현식(t *testing.T) {
	tokens, err := tokenize("($.a > 1 || $.b > 2) && $.c == true")
	require.NoError(t, err)

	node, err := parseCondition(tokens)
	require.NoError(t, err)

	// 최상위는 && (AND)
	logical, ok := node.(*logicalNode)
	require.True(t, ok, "최상위는 logicalNode && 이어야 한다")
	assert.Equal(t, "&&", logical.op)

	// 왼쪽은 || (OR) — 괄호로 묶인 부분
	leftLogical, ok := logical.left.(*logicalNode)
	require.True(t, ok, "왼쪽은 logicalNode || 이어야 한다")
	assert.Equal(t, "||", leftLogical.op)
}

func TestParseCondition_exists함수(t *testing.T) {
	tokens, err := tokenize("exists($.payload.field)")
	require.NoError(t, err)

	node, err := parseCondition(tokens)
	require.NoError(t, err)

	existsN, ok := node.(*existsNode)
	require.True(t, ok, "existsNode 타입이어야 한다")
	assert.Equal(t, "$.payload.field", existsN.path)
}

func TestParseCondition_연산자우선순위(t *testing.T) {
	// && 는 || 보다 우선순위가 높다
	// $.a > 1 || $.b > 2 && $.c > 3 은 $.a > 1 || ($.b > 2 && $.c > 3) 과 동일
	tokens, err := tokenize("$.a > 1 || $.b > 2 && $.c > 3")
	require.NoError(t, err)

	node, err := parseCondition(tokens)
	require.NoError(t, err)

	// 최상위는 || (OR)
	logical, ok := node.(*logicalNode)
	require.True(t, ok, "최상위는 logicalNode || 이어야 한다")
	assert.Equal(t, "||", logical.op)

	// 오른쪽은 && (AND)
	rightLogical, ok := logical.right.(*logicalNode)
	require.True(t, ok, "오른쪽은 logicalNode && 이어야 한다")
	assert.Equal(t, "&&", rightLogical.op)
}

func TestParseCondition_파싱에러(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"빈 표현식", ""},
		{"비교 연산자만", ">="},
		{"경로만 (비교값 없음)", "$.a >="},
		{"닫는 괄호 누락", "($.a > 1"},
		{"존재하지 않는 함수", "unknown($.a)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := tokenize(tt.input)
			if err != nil {
				// 렉서 에러도 유효한 결과
				return
			}
			_, err = parseCondition(tokens)
			assert.Error(t, err, "파싱 에러가 발생해야 한다: input=%q", tt.input)
		})
	}
}

// =============================================================================
// 평가기(evaluate) 테스트
// =============================================================================

func TestEvaluate_숫자비교(t *testing.T) {
	data := map[string]any{
		"payload": map[string]any{
			"temperature": float64(30),
		},
	}

	tests := []struct {
		name string
		op   string
		val  float64
		want bool
	}{
		{"30 > 25 = true", ">", 25, true},
		{"30 > 30 = false", ">", 30, false},
		{"30 > 35 = false", ">", 35, false},
		{"30 >= 30 = true", ">=", 30, true},
		{"30 >= 31 = false", ">=", 31, false},
		{"30 < 35 = true", "<", 35, true},
		{"30 < 30 = false", "<", 30, false},
		{"30 <= 30 = true", "<=", 30, true},
		{"30 <= 29 = false", "<=", 29, false},
		{"30 == 30 = true", "==", 30, true},
		{"30 == 31 = false", "==", 31, false},
		{"30 != 31 = true", "!=", 31, true},
		{"30 != 30 = false", "!=", 30, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &comparisonNode{
				path:    "$.payload.temperature",
				op:      tt.op,
				literal: tt.val,
			}
			got, err := n.evaluate(data)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEvaluate_문자열비교(t *testing.T) {
	data := map[string]any{
		"payload": map[string]any{
			"status": "active",
		},
	}

	tests := []struct {
		name string
		op   string
		val  string
		want bool
	}{
		{"active == active = true", "==", "active", true},
		{"active == inactive = false", "==", "inactive", false},
		{"active != inactive = true", "!=", "inactive", true},
		{"active != active = false", "!=", "active", false},
		// 문자열에 >, <, >=, <= 적용 시 false 반환
		{"문자열 > 비교 = false", ">", "a", false},
		{"문자열 < 비교 = false", "<", "z", false},
		{"문자열 >= 비교 = false", ">=", "active", false},
		{"문자열 <= 비교 = false", "<=", "active", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &comparisonNode{
				path:    "$.payload.status",
				op:      tt.op,
				literal: tt.val,
			}
			got, err := n.evaluate(data)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEvaluate_Bool비교(t *testing.T) {
	data := map[string]any{
		"payload": map[string]any{
			"enabled": true,
		},
	}

	tests := []struct {
		name string
		op   string
		val  bool
		want bool
	}{
		{"true == true = true", "==", true, true},
		{"true == false = false", "==", false, false},
		{"true != false = true", "!=", false, true},
		{"true != true = false", "!=", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &comparisonNode{
				path:    "$.payload.enabled",
				op:      tt.op,
				literal: tt.val,
			}
			got, err := n.evaluate(data)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEvaluate_Null비교(t *testing.T) {
	tests := []struct {
		name string
		data map[string]any
		path string
		op   string
		want bool
	}{
		{
			name: "nil 값 == null = true",
			data: map[string]any{"payload": map[string]any{"count": nil}},
			path: "$.payload.count",
			op:   "==",
			want: true,
		},
		{
			name: "존재하지 않는 경로 == null = true",
			data: map[string]any{"payload": map[string]any{}},
			path: "$.payload.missing",
			op:   "==",
			want: true,
		},
		{
			name: "값 있는 경로 == null = false",
			data: map[string]any{"payload": map[string]any{"count": float64(5)}},
			path: "$.payload.count",
			op:   "==",
			want: false,
		},
		{
			name: "nil 값 != null = false",
			data: map[string]any{"payload": map[string]any{"count": nil}},
			path: "$.payload.count",
			op:   "!=",
			want: false,
		},
		{
			name: "값 있는 경로 != null = true",
			data: map[string]any{"payload": map[string]any{"count": float64(5)}},
			path: "$.payload.count",
			op:   "!=",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &comparisonNode{
				path:    tt.path,
				op:      tt.op,
				literal: nil,
			}
			got, err := n.evaluate(tt.data)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEvaluate_경로없음_false반환(t *testing.T) {
	data := map[string]any{
		"payload": map[string]any{},
	}

	n := &comparisonNode{
		path:    "$.payload.missing",
		op:      ">=",
		literal: float64(30),
	}
	got, err := n.evaluate(data)
	require.NoError(t, err)
	assert.False(t, got, "경로가 존재하지 않으면 false를 반환해야 한다")
}

func TestEvaluate_exists_true(t *testing.T) {
	data := map[string]any{
		"payload": map[string]any{
			"field": "value",
		},
	}

	n := &existsNode{path: "$.payload.field"}
	got, err := n.evaluate(data)
	require.NoError(t, err)
	assert.True(t, got)
}

func TestEvaluate_exists_false(t *testing.T) {
	data := map[string]any{
		"payload": map[string]any{},
	}

	n := &existsNode{path: "$.payload.missing"}
	got, err := n.evaluate(data)
	require.NoError(t, err)
	assert.False(t, got)
}

func TestEvaluate_논리AND_단락평가(t *testing.T) {
	data := map[string]any{
		"payload": map[string]any{
			"a": float64(1),
		},
	}

	// false && <anything> = false (오른쪽 평가하지 않음)
	n := &logicalNode{
		left:  &comparisonNode{path: "$.payload.a", op: ">", literal: float64(10)},
		op:    "&&",
		right: &comparisonNode{path: "$.payload.a", op: "<", literal: float64(5)},
	}
	got, err := n.evaluate(data)
	require.NoError(t, err)
	assert.False(t, got)

	// true && true = true
	n2 := &logicalNode{
		left:  &comparisonNode{path: "$.payload.a", op: "==", literal: float64(1)},
		op:    "&&",
		right: &comparisonNode{path: "$.payload.a", op: "<", literal: float64(5)},
	}
	got2, err := n2.evaluate(data)
	require.NoError(t, err)
	assert.True(t, got2)
}

func TestEvaluate_논리OR_단락평가(t *testing.T) {
	data := map[string]any{
		"payload": map[string]any{
			"a": float64(1),
		},
	}

	// true || <anything> = true (오른쪽 평가하지 않음)
	n := &logicalNode{
		left:  &comparisonNode{path: "$.payload.a", op: "==", literal: float64(1)},
		op:    "||",
		right: &comparisonNode{path: "$.payload.a", op: ">", literal: float64(100)},
	}
	got, err := n.evaluate(data)
	require.NoError(t, err)
	assert.True(t, got)

	// false || false = false
	n2 := &logicalNode{
		left:  &comparisonNode{path: "$.payload.a", op: ">", literal: float64(10)},
		op:    "||",
		right: &comparisonNode{path: "$.payload.a", op: ">", literal: float64(100)},
	}
	got2, err := n2.evaluate(data)
	require.NoError(t, err)
	assert.False(t, got2)
}

func TestEvaluate_NOT부정(t *testing.T) {
	data := map[string]any{
		"payload": map[string]any{
			"field": "value",
		},
	}

	// !exists($.payload.field) = false (존재하므로)
	n := &notNode{
		expr: &existsNode{path: "$.payload.field"},
	}
	got, err := n.evaluate(data)
	require.NoError(t, err)
	assert.False(t, got)

	// !exists($.payload.missing) = true (존재하지 않으므로)
	n2 := &notNode{
		expr: &existsNode{path: "$.payload.missing"},
	}
	got2, err := n2.evaluate(data)
	require.NoError(t, err)
	assert.True(t, got2)
}

func TestEvaluate_복합중첩표현식(t *testing.T) {
	data := map[string]any{
		"payload": map[string]any{
			"temperature": float64(35),
			"humidity":    float64(60),
			"status":      "active",
		},
	}

	// ($.payload.temperature > 30 && $.payload.humidity > 50) || $.payload.status == 'error'
	n := &logicalNode{
		left: &logicalNode{
			left:  &comparisonNode{path: "$.payload.temperature", op: ">", literal: float64(30)},
			op:    "&&",
			right: &comparisonNode{path: "$.payload.humidity", op: ">", literal: float64(50)},
		},
		op:    "||",
		right: &comparisonNode{path: "$.payload.status", op: "==", literal: "error"},
	}

	got, err := n.evaluate(data)
	require.NoError(t, err)
	assert.True(t, got, "온도>30 AND 습도>50 이므로 true여야 한다")
}

// =============================================================================
// compileCondition 통합 테스트
// =============================================================================

func TestCompileCondition_통합_SPEC예시(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		payload map[string]any
		want    bool
	}{
		{
			name:    "온도 >= 30 (조건 충족)",
			expr:    "$.payload.temperature >= 30",
			payload: map[string]any{"temperature": float64(35)},
			want:    true,
		},
		{
			name:    "온도 >= 30 (조건 미충족)",
			expr:    "$.payload.temperature >= 30",
			payload: map[string]any{"temperature": float64(25)},
			want:    false,
		},
		{
			name:    "온도 범위 -40 ~ 150 (범위 내)",
			expr:    "$.payload.temperature >= -40 && $.payload.temperature <= 150",
			payload: map[string]any{"temperature": float64(25)},
			want:    true,
		},
		{
			name:    "온도 범위 -40 ~ 150 (범위 초과)",
			expr:    "$.payload.temperature >= -40 && $.payload.temperature <= 150",
			payload: map[string]any{"temperature": float64(200)},
			want:    false,
		},
		{
			name:    "상태 == active (일치)",
			expr:    "$.payload.status == 'active'",
			payload: map[string]any{"status": "active"},
			want:    true,
		},
		{
			name:    "상태 == active (불일치)",
			expr:    "$.payload.status == 'active'",
			payload: map[string]any{"status": "inactive"},
			want:    false,
		},
		{
			name:    "exists 필드 존재",
			expr:    "exists($.payload.error)",
			payload: map[string]any{"error": "some error"},
			want:    true,
		},
		{
			name:    "exists 필드 미존재",
			expr:    "exists($.payload.error)",
			payload: map[string]any{"status": "ok"},
			want:    false,
		},
		{
			name:    "!exists AND 양수 (조건 충족)",
			expr:    "!exists($.payload.error) && $.payload.value > 0",
			payload: map[string]any{"value": float64(10)},
			want:    true,
		},
		{
			name:    "!exists AND 양수 (에러 있음)",
			expr:    "!exists($.payload.error) && $.payload.value > 0",
			payload: map[string]any{"error": "err", "value": float64(10)},
			want:    false,
		},
		{
			name:    "불리언 true 확인",
			expr:    "$.payload.enabled == true",
			payload: map[string]any{"enabled": true},
			want:    true,
		},
		{
			name:    "불리언 false 확인",
			expr:    "$.payload.enabled == true",
			payload: map[string]any{"enabled": false},
			want:    false,
		},
		{
			name:    "null이 아닌 확인 (값 존재)",
			expr:    "$.payload.count != null",
			payload: map[string]any{"count": float64(5)},
			want:    true,
		},
		{
			name:    "null이 아닌 확인 (null 값)",
			expr:    "$.payload.count != null",
			payload: map[string]any{"count": nil},
			want:    false,
		},
		{
			name:    "null이 아닌 확인 (필드 미존재)",
			expr:    "$.payload.count != null",
			payload: map[string]any{},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := compileCondition(tt.expr)
			require.NoError(t, err, "compileCondition(%q)", tt.expr)

			msg := message.New(message.WithPayload(message.NewPayload(tt.payload)))
			got := cond(msg)
			assert.Equal(t, tt.want, got,
				"expr=%q payload=%v", tt.expr, tt.payload)
		})
	}
}

func TestCompileCondition_큰따옴표문자열(t *testing.T) {
	cond, err := compileCondition(`$.payload.status == "active"`)
	require.NoError(t, err)

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"status": "active",
	})))
	assert.True(t, cond(msg))
}

func TestCompileCondition_연산자우선순위_통합(t *testing.T) {
	// $.a == 1 || $.b == 2 && $.c == 3
	// && 가 || 보다 우선 → $.a == 1 || ($.b == 2 && $.c == 3)
	cond, err := compileCondition("$.payload.a == 1 || $.payload.b == 2 && $.payload.c == 3")
	require.NoError(t, err)

	// a==1이므로 || 왼쪽이 true → 전체 true
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"a": float64(1), "b": float64(0), "c": float64(0),
	})))
	assert.True(t, cond(msg1), "a==1이면 || 왼쪽이 true이므로 전체 true")

	// a!=1, b==2, c!=3 → || 왼쪽 false, && 오른쪽도 false → 전체 false
	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"a": float64(0), "b": float64(2), "c": float64(0),
	})))
	assert.False(t, cond(msg2), "a!=1 이고 b==2이나 c!=3이므로 전체 false")

	// a!=1, b==2, c==3 → || 왼쪽 false, && 오른쪽 true → 전체 true
	msg3 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"a": float64(0), "b": float64(2), "c": float64(3),
	})))
	assert.True(t, cond(msg3), "b==2 && c==3이므로 오른쪽 true → 전체 true")
}

func TestCompileCondition_에러케이스(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"빈 표현식", ""},
		{"잘못된 문법", "$.a >="},
		{"인식불가 문자", "$.a @ 30"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := compileCondition(tt.expr)
			assert.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidExpression),
				"에러가 ErrInvalidExpression을 래핑해야 한다: %v", err)
		})
	}
}

func TestCompileCondition_정수타입값처리(t *testing.T) {
	// 페이로드에 int 타입이 올 수도 있다 (JSON 디코딩 외)
	cond, err := compileCondition("$.payload.count > 5")
	require.NoError(t, err)

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"count": 10, // int (float64가 아님)
	})))
	assert.True(t, cond(msg), "int 타입도 float64로 변환되어 비교되어야 한다")
}

func TestCompileCondition_괄호그룹_통합(t *testing.T) {
	// ($.a > 1 || $.b > 2) && $.c == true
	cond, err := compileCondition("($.payload.a > 1 || $.payload.b > 2) && $.payload.c == true")
	require.NoError(t, err)

	// a=5 (>1), c=true → 왼쪽 true && 오른쪽 true → true
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"a": float64(5), "b": float64(0), "c": true,
	})))
	assert.True(t, cond(msg1))

	// a=0, b=0, c=true → 왼쪽 false && true → false
	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"a": float64(0), "b": float64(0), "c": true,
	})))
	assert.False(t, cond(msg2))

	// a=5, c=false → 왼쪽 true && false → false
	msg3 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"a": float64(5), "b": float64(0), "c": false,
	})))
	assert.False(t, cond(msg3))
}

func TestCompileCondition_메타데이터접근(t *testing.T) {
	cond, err := compileCondition("$.metadata.source == 'mqtt'")
	require.NoError(t, err)

	msg := message.New(
		message.WithPayload(message.NewPayload(map[string]any{})),
		message.WithMetadata("source", "mqtt"),
	)
	assert.True(t, cond(msg))
}
