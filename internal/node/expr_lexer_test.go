package node

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// exprTokenize 렉서 테스트
// =============================================================================

// TestExprTokenize_Path 는 JSONPath 토큰($. 접두사)을 테스트한다.
func TestExprTokenize_Path(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTyps []exprTokenType
		wantVals []string
	}{
		{
			name:     "단순 경로",
			input:    "$.payload.temp",
			wantTyps: []exprTokenType{exprTokenPath, exprTokenEOF},
			wantVals: []string{"$.payload.temp", ""},
		},
		{
			name:     "중첩 경로",
			input:    "$.payload.deviceInfo.tags.location",
			wantTyps: []exprTokenType{exprTokenPath, exprTokenEOF},
			wantVals: []string{"$.payload.deviceInfo.tags.location", ""},
		},
		{
			name:     "밑줄 포함 경로",
			input:    "$._base",
			wantTyps: []exprTokenType{exprTokenPath, exprTokenEOF},
			wantVals: []string{"$._base", ""},
		},
		{
			name:     "숫자 포함 경로 세그먼트",
			input:    "$.payload.sensor1.value",
			wantTyps: []exprTokenType{exprTokenPath, exprTokenEOF},
			wantVals: []string{"$.payload.sensor1.value", ""},
		},
		{
			name:     "최상위 필드",
			input:    "$.id",
			wantTyps: []exprTokenType{exprTokenPath, exprTokenEOF},
			wantVals: []string{"$.id", ""},
		},
		{
			name:     "배열 인덱스 포함 경로",
			input:    "$.payload.value[0].value",
			wantTyps: []exprTokenType{exprTokenPath, exprTokenEOF},
			wantVals: []string{"$.payload.value[0].value", ""},
		},
		{
			name:     "와일드카드 포함 경로",
			input:    "$.items[*].name",
			wantTyps: []exprTokenType{exprTokenPath, exprTokenEOF},
			wantVals: []string{"$.items[*].name", ""},
		},
		{
			name:     "연속 배열 인덱스",
			input:    "$.matrix[0][1]",
			wantTyps: []exprTokenType{exprTokenPath, exprTokenEOF},
			wantVals: []string{"$.matrix[0][1]", ""},
		},
		{
			name:     "배열 인덱스 후 필드 접근",
			input:    "$.store_value[2].timestamp",
			wantTyps: []exprTokenType{exprTokenPath, exprTokenEOF},
			wantVals: []string{"$.store_value[2].timestamp", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := exprTokenize(tt.input)
			require.NoError(t, err)
			require.Len(t, tokens, len(tt.wantTyps))
			for i, tok := range tokens {
				assert.Equal(t, tt.wantTyps[i], tok.typ, "토큰 %d 타입 불일치", i)
				assert.Equal(t, tt.wantVals[i], tok.value, "토큰 %d 값 불일치", i)
			}
		})
	}
}

// TestExprTokenize_Var 는 변수 토큰($ + 이름)을 테스트한다.
func TestExprTokenize_Var(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTyps []exprTokenType
		wantVals []string
	}{
		{
			name:     "변수",
			input:    "$address_table",
			wantTyps: []exprTokenType{exprTokenVar, exprTokenEOF},
			wantVals: []string{"address_table", ""},
		},
		{
			name:     "숫자 포함 변수",
			input:    "$var1",
			wantTyps: []exprTokenType{exprTokenVar, exprTokenEOF},
			wantVals: []string{"var1", ""},
		},
		{
			name:     "밑줄로 시작하는 변수",
			input:    "$_private",
			wantTyps: []exprTokenType{exprTokenVar, exprTokenEOF},
			wantVals: []string{"_private", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := exprTokenize(tt.input)
			require.NoError(t, err)
			require.Len(t, tokens, len(tt.wantTyps))
			for i, tok := range tokens {
				assert.Equal(t, tt.wantTyps[i], tok.typ, "토큰 %d 타입 불일치", i)
				assert.Equal(t, tt.wantVals[i], tok.value, "토큰 %d 값 불일치", i)
			}
		})
	}
}

// TestExprTokenize_Number 는 숫자 토큰을 테스트한다.
func TestExprTokenize_Number(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTyps []exprTokenType
		wantVals []string
	}{
		{
			name:     "정수",
			input:    "42",
			wantTyps: []exprTokenType{exprTokenNumber, exprTokenEOF},
			wantVals: []string{"42", ""},
		},
		{
			name:     "소수",
			input:    "3.14",
			wantTyps: []exprTokenType{exprTokenNumber, exprTokenEOF},
			wantVals: []string{"3.14", ""},
		},
		{
			name:     "0",
			input:    "0",
			wantTyps: []exprTokenType{exprTokenNumber, exprTokenEOF},
			wantVals: []string{"0", ""},
		},
		{
			name:     "0으로 시작하는 소수",
			input:    "0.5",
			wantTyps: []exprTokenType{exprTokenNumber, exprTokenEOF},
			wantVals: []string{"0.5", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := exprTokenize(tt.input)
			require.NoError(t, err)
			require.Len(t, tokens, len(tt.wantTyps))
			for i, tok := range tokens {
				assert.Equal(t, tt.wantTyps[i], tok.typ, "토큰 %d 타입 불일치", i)
				assert.Equal(t, tt.wantVals[i], tok.value, "토큰 %d 값 불일치", i)
			}
		})
	}
}

// TestExprTokenize_NegativeNumberIsSeparateTokens 는 음수가 Minus + Number 토큰으로 분리되는지 테스트한다.
func TestExprTokenize_NegativeNumberIsSeparateTokens(t *testing.T) {
	tokens, err := exprTokenize("-3.14")
	require.NoError(t, err)
	require.Len(t, tokens, 3)
	assert.Equal(t, exprTokenMinus, tokens[0].typ)
	assert.Equal(t, "-", tokens[0].value)
	assert.Equal(t, exprTokenNumber, tokens[1].typ)
	assert.Equal(t, "3.14", tokens[1].value)
	assert.Equal(t, exprTokenEOF, tokens[2].typ)
}

// TestExprTokenize_String 은 문자열 토큰을 테스트한다.
func TestExprTokenize_String(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTyps []exprTokenType
		wantVals []string
	}{
		{
			name:     "큰따옴표 문자열",
			input:    `"hello"`,
			wantTyps: []exprTokenType{exprTokenString, exprTokenEOF},
			wantVals: []string{"hello", ""},
		},
		{
			name:     "작은따옴표 문자열",
			input:    "'world'",
			wantTyps: []exprTokenType{exprTokenString, exprTokenEOF},
			wantVals: []string{"world", ""},
		},
		{
			name:     "빈 문자열 (큰따옴표)",
			input:    `""`,
			wantTyps: []exprTokenType{exprTokenString, exprTokenEOF},
			wantVals: []string{"", ""},
		},
		{
			name:     "빈 문자열 (작은따옴표)",
			input:    "''",
			wantTyps: []exprTokenType{exprTokenString, exprTokenEOF},
			wantVals: []string{"", ""},
		},
		{
			name:     "공백 포함 문자열",
			input:    `"hello world"`,
			wantTyps: []exprTokenType{exprTokenString, exprTokenEOF},
			wantVals: []string{"hello world", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := exprTokenize(tt.input)
			require.NoError(t, err)
			require.Len(t, tokens, len(tt.wantTyps))
			for i, tok := range tokens {
				assert.Equal(t, tt.wantTyps[i], tok.typ, "토큰 %d 타입 불일치", i)
				assert.Equal(t, tt.wantVals[i], tok.value, "토큰 %d 값 불일치", i)
			}
		})
	}
}

// TestExprTokenize_Bool 은 bool 토큰을 테스트한다.
func TestExprTokenize_Bool(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTyps []exprTokenType
		wantVals []string
	}{
		{
			name:     "true",
			input:    "true",
			wantTyps: []exprTokenType{exprTokenBool, exprTokenEOF},
			wantVals: []string{"true", ""},
		},
		{
			name:     "false",
			input:    "false",
			wantTyps: []exprTokenType{exprTokenBool, exprTokenEOF},
			wantVals: []string{"false", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := exprTokenize(tt.input)
			require.NoError(t, err)
			require.Len(t, tokens, len(tt.wantTyps))
			for i, tok := range tokens {
				assert.Equal(t, tt.wantTyps[i], tok.typ, "토큰 %d 타입 불일치", i)
				assert.Equal(t, tt.wantVals[i], tok.value, "토큰 %d 값 불일치", i)
			}
		})
	}
}

// TestExprTokenize_Null 은 null 토큰을 테스트한다.
func TestExprTokenize_Null(t *testing.T) {
	tokens, err := exprTokenize("null")
	require.NoError(t, err)
	require.Len(t, tokens, 2)
	assert.Equal(t, exprTokenNull, tokens[0].typ)
	assert.Equal(t, "null", tokens[0].value)
	assert.Equal(t, exprTokenEOF, tokens[1].typ)
}

// TestExprTokenize_Operators 는 산술/연결 연산자 토큰을 테스트한다.
func TestExprTokenize_Operators(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantTyp exprTokenType
		wantVal string
	}{
		{"플러스", "+", exprTokenPlus, "+"},
		{"마이너스", "-", exprTokenMinus, "-"},
		{"스타", "*", exprTokenStar, "*"},
		{"슬래시", "/", exprTokenSlash, "/"},
		{"퍼센트", "%", exprTokenPercent, "%"},
		{"앰퍼샌드", "&", exprTokenAmpersand, "&"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := exprTokenize(tt.input)
			require.NoError(t, err)
			require.Len(t, tokens, 2)
			assert.Equal(t, tt.wantTyp, tokens[0].typ)
			assert.Equal(t, tt.wantVal, tokens[0].value)
			assert.Equal(t, exprTokenEOF, tokens[1].typ)
		})
	}
}

// TestExprTokenize_AllOperatorsInSequence 는 모든 연산자가 연속으로 올 때를 테스트한다.
func TestExprTokenize_AllOperatorsInSequence(t *testing.T) {
	tokens, err := exprTokenize("+ - * / %")
	require.NoError(t, err)
	require.Len(t, tokens, 6) // 5 operators + EOF
	assert.Equal(t, exprTokenPlus, tokens[0].typ)
	assert.Equal(t, exprTokenMinus, tokens[1].typ)
	assert.Equal(t, exprTokenStar, tokens[2].typ)
	assert.Equal(t, exprTokenSlash, tokens[3].typ)
	assert.Equal(t, exprTokenPercent, tokens[4].typ)
	assert.Equal(t, exprTokenEOF, tokens[5].typ)
}

// TestExprTokenize_Delimiters 는 구분자 토큰을 테스트한다.
func TestExprTokenize_Delimiters(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantTyp exprTokenType
		wantVal string
	}{
		{"좌중괄호", "{", exprTokenLBrace, "{"},
		{"우중괄호", "}", exprTokenRBrace, "}"},
		{"좌대괄호", "[", exprTokenLBracket, "["},
		{"우대괄호", "]", exprTokenRBracket, "]"},
		{"좌소괄호", "(", exprTokenLParen, "("},
		{"우소괄호", ")", exprTokenRParen, ")"},
		{"콜론", ":", exprTokenColon, ":"},
		{"쉼표", ",", exprTokenComma, ","},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := exprTokenize(tt.input)
			require.NoError(t, err)
			require.Len(t, tokens, 2)
			assert.Equal(t, tt.wantTyp, tokens[0].typ)
			assert.Equal(t, tt.wantVal, tokens[0].value)
			assert.Equal(t, exprTokenEOF, tokens[1].typ)
		})
	}
}

// TestExprTokenize_AllDelimitersInSequence 는 모든 구분자가 연속으로 올 때를 테스트한다.
func TestExprTokenize_AllDelimitersInSequence(t *testing.T) {
	tokens, err := exprTokenize("{ } [ ] ( ) : ,")
	require.NoError(t, err)
	require.Len(t, tokens, 9) // 8 delimiters + EOF
	expected := []exprTokenType{
		exprTokenLBrace, exprTokenRBrace,
		exprTokenLBracket, exprTokenRBracket,
		exprTokenLParen, exprTokenRParen,
		exprTokenColon, exprTokenComma,
		exprTokenEOF,
	}
	for i, tok := range tokens {
		assert.Equal(t, expected[i], tok.typ, "토큰 %d 타입 불일치", i)
	}
}

// TestExprTokenize_Ident 는 식별자 토큰을 테스트한다.
func TestExprTokenize_Ident(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTyps []exprTokenType
		wantVals []string
	}{
		{
			name:     "함수 이름 now",
			input:    "now",
			wantTyps: []exprTokenType{exprTokenIdent, exprTokenEOF},
			wantVals: []string{"now", ""},
		},
		{
			name:     "함수 이름 round",
			input:    "round",
			wantTyps: []exprTokenType{exprTokenIdent, exprTokenEOF},
			wantVals: []string{"round", ""},
		},
		{
			name:     "밑줄 포함 식별자",
			input:    "my_func",
			wantTyps: []exprTokenType{exprTokenIdent, exprTokenEOF},
			wantVals: []string{"my_func", ""},
		},
		{
			name:     "숫자 포함 식별자",
			input:    "func1",
			wantTyps: []exprTokenType{exprTokenIdent, exprTokenEOF},
			wantVals: []string{"func1", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := exprTokenize(tt.input)
			require.NoError(t, err)
			require.Len(t, tokens, len(tt.wantTyps))
			for i, tok := range tokens {
				assert.Equal(t, tt.wantTyps[i], tok.typ, "토큰 %d 타입 불일치", i)
				assert.Equal(t, tt.wantVals[i], tok.value, "토큰 %d 값 불일치", i)
			}
		})
	}
}

// TestExprTokenize_Whitespace 는 공백 처리를 테스트한다.
func TestExprTokenize_Whitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"공백만", "   "},
		{"탭만", "\t\t"},
		{"개행만", "\n\n"},
		{"캐리지 리턴", "\r\n"},
		{"혼합 공백", " \t \n \r "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := exprTokenize(tt.input)
			require.NoError(t, err)
			require.Len(t, tokens, 1)
			assert.Equal(t, exprTokenEOF, tokens[0].typ)
		})
	}
}

// TestExprTokenize_EmptyInput 은 빈 입력을 테스트한다.
func TestExprTokenize_EmptyInput(t *testing.T) {
	tokens, err := exprTokenize("")
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	assert.Equal(t, exprTokenEOF, tokens[0].typ)
}

// TestExprTokenize_PositionTracking 은 토큰 위치 추적을 테스트한다.
func TestExprTokenize_PositionTracking(t *testing.T) {
	// "42 + 3" -> 위치: 42(0), +(3), 3(5)
	tokens, err := exprTokenize("42 + 3")
	require.NoError(t, err)
	require.Len(t, tokens, 4)
	assert.Equal(t, 0, tokens[0].pos) // 42
	assert.Equal(t, 3, tokens[1].pos) // +
	assert.Equal(t, 5, tokens[2].pos) // 3
}

// TestExprTokenize_ComplexExpression 은 복잡한 표현식의 토큰화를 테스트한다.
func TestExprTokenize_ComplexExpression(t *testing.T) {
	// { temp: $.payload.temperature, _base: $address_table[$.tag & ":" & $.floor] + 2 }
	input := `{ temp: $.payload.temperature, _base: $address_table[$.tag & ":" & $.floor] + 2 }`
	tokens, err := exprTokenize(input)
	require.NoError(t, err)

	// 기대되는 토큰 시퀀스 검증
	expectedTypes := []exprTokenType{
		exprTokenLBrace,   // {
		exprTokenIdent,    // temp
		exprTokenColon,    // :
		exprTokenPath,     // $.payload.temperature
		exprTokenComma,    // ,
		exprTokenIdent,    // _base
		exprTokenColon,    // :
		exprTokenVar,      // $address_table -> "address_table"
		exprTokenLBracket, // [
		exprTokenPath,     // $.tag
		exprTokenAmpersand, // &
		exprTokenString,   // ":"
		exprTokenAmpersand, // &
		exprTokenPath,     // $.floor
		exprTokenRBracket, // ]
		exprTokenPlus,     // +
		exprTokenNumber,   // 2
		exprTokenRBrace,   // }
		exprTokenEOF,
	}
	expectedValues := []string{
		"{", "temp", ":", "$.payload.temperature", ",",
		"_base", ":", "address_table", "[",
		"$.tag", "&", ":", "&", "$.floor", "]",
		"+", "2", "}", "",
	}

	require.Len(t, tokens, len(expectedTypes), "토큰 수 불일치")
	for i, tok := range tokens {
		assert.Equal(t, expectedTypes[i], tok.typ, "토큰 %d (%q) 타입 불일치", i, tok.value)
		assert.Equal(t, expectedValues[i], tok.value, "토큰 %d 값 불일치", i)
	}
}

// TestExprTokenize_FunctionCallExpression 은 함수 호출 표현식을 테스트한다.
func TestExprTokenize_FunctionCallExpression(t *testing.T) {
	// round($.payload.value, 2)
	input := "round($.payload.value, 2)"
	tokens, err := exprTokenize(input)
	require.NoError(t, err)

	expectedTypes := []exprTokenType{
		exprTokenIdent,    // round
		exprTokenLParen,   // (
		exprTokenPath,     // $.payload.value
		exprTokenComma,    // ,
		exprTokenNumber,   // 2
		exprTokenRParen,   // )
		exprTokenEOF,
	}
	expectedValues := []string{"round", "(", "$.payload.value", ",", "2", ")", ""}

	require.Len(t, tokens, len(expectedTypes))
	for i, tok := range tokens {
		assert.Equal(t, expectedTypes[i], tok.typ, "토큰 %d 타입 불일치", i)
		assert.Equal(t, expectedValues[i], tok.value, "토큰 %d 값 불일치", i)
	}
}

// TestExprTokenize_PathStopsAtOperator 는 경로가 연산자에서 끝나는지 테스트한다.
func TestExprTokenize_PathStopsAtOperator(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTyps []exprTokenType
		wantVals []string
	}{
		{
			name:     "경로 + 플러스",
			input:    "$.a+$.b",
			wantTyps: []exprTokenType{exprTokenPath, exprTokenPlus, exprTokenPath, exprTokenEOF},
			wantVals: []string{"$.a", "+", "$.b", ""},
		},
		{
			name:     "경로 + 앰퍼샌드",
			input:    "$.x&$.y",
			wantTyps: []exprTokenType{exprTokenPath, exprTokenAmpersand, exprTokenPath, exprTokenEOF},
			wantVals: []string{"$.x", "&", "$.y", ""},
		},
		{
			name:     "경로 + 중괄호",
			input:    "$.a}",
			wantTyps: []exprTokenType{exprTokenPath, exprTokenRBrace, exprTokenEOF},
			wantVals: []string{"$.a", "}", ""},
		},
		{
			// '[N]' / '[*]' 는 이제 경로 토큰의 일부로 흡수된다.
			// 정수/와일드카드가 아닌 대괄호 내용 (식별자, 표현식 등) 은 여전히 경로 종료로 해석된다.
			name:     "경로 + 비경로용 대괄호",
			input:    "$.a[key]",
			wantTyps: []exprTokenType{exprTokenPath, exprTokenLBracket, exprTokenIdent, exprTokenRBracket, exprTokenEOF},
			wantVals: []string{"$.a", "[", "key", "]", ""},
		},
		{
			name:     "경로 + 콜론",
			input:    "$.a:$.b",
			wantTyps: []exprTokenType{exprTokenPath, exprTokenColon, exprTokenPath, exprTokenEOF},
			wantVals: []string{"$.a", ":", "$.b", ""},
		},
		{
			name:     "경로 + 쉼표",
			input:    "$.a,$.b",
			wantTyps: []exprTokenType{exprTokenPath, exprTokenComma, exprTokenPath, exprTokenEOF},
			wantVals: []string{"$.a", ",", "$.b", ""},
		},
		{
			name:     "경로 + 소괄호",
			input:    "$.a)",
			wantTyps: []exprTokenType{exprTokenPath, exprTokenRParen, exprTokenEOF},
			wantVals: []string{"$.a", ")", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := exprTokenize(tt.input)
			require.NoError(t, err)
			require.Len(t, tokens, len(tt.wantTyps))
			for i, tok := range tokens {
				assert.Equal(t, tt.wantTyps[i], tok.typ, "토큰 %d 타입 불일치", i)
				assert.Equal(t, tt.wantVals[i], tok.value, "토큰 %d 값 불일치", i)
			}
		})
	}
}

// TestExprTokenize_PathVsVarDifferentiation 는 Path와 Var 토큰의 구분을 테스트한다.
func TestExprTokenize_PathVsVarDifferentiation(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantTyp exprTokenType
		wantVal string
	}{
		{
			name:    "$ + . = Path",
			input:   "$.field",
			wantTyp: exprTokenPath,
			wantVal: "$.field",
		},
		{
			name:    "$ + letter = Var",
			input:   "$myvar",
			wantTyp: exprTokenVar,
			wantVal: "myvar",
		},
		{
			name:    "$ + underscore = Var",
			input:   "$_private",
			wantTyp: exprTokenVar,
			wantVal: "_private",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := exprTokenize(tt.input)
			require.NoError(t, err)
			require.Len(t, tokens, 2) // token + EOF
			assert.Equal(t, tt.wantTyp, tokens[0].typ)
			assert.Equal(t, tt.wantVal, tokens[0].value)
		})
	}
}

// =============================================================================
// 에러 케이스 테스트
// =============================================================================

// TestExprTokenize_ErrorUnterminatedString 은 닫히지 않은 문자열 에러를 테스트한다.
func TestExprTokenize_ErrorUnterminatedString(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"큰따옴표 미종료", `"unterminated`},
		{"작은따옴표 미종료", "'unterminated"},
		{"빈 큰따옴표 미종료", `"`},
		{"빈 작은따옴표 미종료", "'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := exprTokenize(tt.input)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidExpression),
				"에러가 ErrInvalidExpression을 래핑해야 함: %v", err)
			assert.Contains(t, err.Error(), "unterminated string")
		})
	}
}

// TestExprTokenize_ErrorUnrecognizedCharacter 는 인식할 수 없는 문자 에러를 테스트한다.
func TestExprTokenize_ErrorUnrecognizedCharacter(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"@ 문자", "$.payload.temp @"},
		{"# 문자", "#"},
		{"~ 문자", "~"},
		{"^ 문자", "^"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := exprTokenize(tt.input)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidExpression),
				"에러가 ErrInvalidExpression을 래핑해야 함: %v", err)
		})
	}
}

// TestExprTokenize_ErrorDollarAlone 은 $ 단독 사용 에러를 테스트한다.
func TestExprTokenize_ErrorDollarAlone(t *testing.T) {
	_, err := exprTokenize("$ ")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidExpression))
}

// TestExprTokenize_WhitespaceAroundTokens 는 토큰 사이 다양한 공백 처리를 테스트한다.
func TestExprTokenize_WhitespaceAroundTokens(t *testing.T) {
	// 탭, 개행 혼합
	input := "{\n\ttemp:\t$.payload.temp\n}"
	tokens, err := exprTokenize(input)
	require.NoError(t, err)

	expectedTypes := []exprTokenType{
		exprTokenLBrace, exprTokenIdent, exprTokenColon,
		exprTokenPath, exprTokenRBrace, exprTokenEOF,
	}
	require.Len(t, tokens, len(expectedTypes))
	for i, tok := range tokens {
		assert.Equal(t, expectedTypes[i], tok.typ, "토큰 %d 타입 불일치", i)
	}
}

// TestExprTokenize_SingleCharTokens 는 단일 문자 토큰의 정확성을 테스트한다.
func TestExprTokenize_SingleCharTokens(t *testing.T) {
	singleChars := []struct {
		ch      string
		wantTyp exprTokenType
	}{
		{"+", exprTokenPlus},
		{"-", exprTokenMinus},
		{"*", exprTokenStar},
		{"/", exprTokenSlash},
		{"%", exprTokenPercent},
		{"&", exprTokenAmpersand},
		{"{", exprTokenLBrace},
		{"}", exprTokenRBrace},
		{"[", exprTokenLBracket},
		{"]", exprTokenRBracket},
		{"(", exprTokenLParen},
		{")", exprTokenRParen},
		{":", exprTokenColon},
		{",", exprTokenComma},
	}

	for _, sc := range singleChars {
		t.Run(sc.ch, func(t *testing.T) {
			tokens, err := exprTokenize(sc.ch)
			require.NoError(t, err)
			require.Len(t, tokens, 2)
			assert.Equal(t, sc.wantTyp, tokens[0].typ)
			assert.Equal(t, sc.ch, tokens[0].value)
			assert.Equal(t, 0, tokens[0].pos)
		})
	}
}

// TestExprTokenize_ObjectLiteralExpression 은 객체 리터럴 표현식을 테스트한다.
func TestExprTokenize_ObjectLiteralExpression(t *testing.T) {
	input := `{ name: "sensor", value: $.payload.temp * 1.8 + 32 }`
	tokens, err := exprTokenize(input)
	require.NoError(t, err)

	expectedTypes := []exprTokenType{
		exprTokenLBrace,  // {
		exprTokenIdent,   // name
		exprTokenColon,   // :
		exprTokenString,  // "sensor"
		exprTokenComma,   // ,
		exprTokenIdent,   // value
		exprTokenColon,   // :
		exprTokenPath,    // $.payload.temp
		exprTokenStar,    // *
		exprTokenNumber,  // 1.8
		exprTokenPlus,    // +
		exprTokenNumber,  // 32
		exprTokenRBrace,  // }
		exprTokenEOF,
	}

	require.Len(t, tokens, len(expectedTypes))
	for i, tok := range tokens {
		assert.Equal(t, expectedTypes[i], tok.typ, "토큰 %d (%q) 타입 불일치", i, tok.value)
	}
}

// TestExprTokenize_BoolAndNullInExpression 은 bool/null이 표현식 내에서 정확히 인식되는지 테스트한다.
func TestExprTokenize_BoolAndNullInExpression(t *testing.T) {
	input := "{ active: true, error: null, debug: false }"
	tokens, err := exprTokenize(input)
	require.NoError(t, err)

	expectedTypes := []exprTokenType{
		exprTokenLBrace, exprTokenIdent, exprTokenColon, exprTokenBool, exprTokenComma,
		exprTokenIdent, exprTokenColon, exprTokenNull, exprTokenComma,
		exprTokenIdent, exprTokenColon, exprTokenBool, exprTokenRBrace, exprTokenEOF,
	}

	require.Len(t, tokens, len(expectedTypes))
	for i, tok := range tokens {
		assert.Equal(t, expectedTypes[i], tok.typ, "토큰 %d (%q) 타입 불일치", i, tok.value)
	}
}
