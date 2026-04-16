package node

import (
	"fmt"
	"unicode"
)

// =============================================================================
// 토큰 정의 (Expression 렉서)
// =============================================================================

// exprTokenType 은 변환 표현식의 토큰 타입을 나타낸다.
type exprTokenType int

const (
	exprTokenPath      exprTokenType = iota // $.payload.field ($ + . 로 시작하는 JSONPath)
	exprTokenVar                            // $variable_name ($ + 문자/밑줄로 시작하는 변수)
	exprTokenNumber                         // 42, 3.14
	exprTokenString                         // "hello" 또는 'hello'
	exprTokenBool                           // true, false
	exprTokenNull                           // null
	exprTokenIdent                          // 식별자 (함수 이름, 객체 키 등)
	exprTokenPlus                           // +
	exprTokenMinus                          // -
	exprTokenStar                           // *
	exprTokenSlash                          // /
	exprTokenPercent                        // %
	exprTokenAmpersand                      // & (문자열 연결)
	exprTokenLBrace                         // {
	exprTokenRBrace                         // }
	exprTokenLBracket                       // [
	exprTokenRBracket                       // ]
	exprTokenLParen                         // (
	exprTokenRParen                         // )
	exprTokenColon                          // :
	exprTokenComma                          // ,
	exprTokenEOF                            // 입력 종료
)

// exprToken 은 렉서가 생성하는 단일 토큰이다.
type exprToken struct {
	typ   exprTokenType
	value string
	pos   int
}

// =============================================================================
// 렉서 구현
// =============================================================================

// exprSingleCharTokens 는 단일 문자를 토큰 타입으로 매핑하는 조회 테이블이다.
var exprSingleCharTokens = map[rune]exprTokenType{
	'+': exprTokenPlus,
	'-': exprTokenMinus,
	'*': exprTokenStar,
	'/': exprTokenSlash,
	'%': exprTokenPercent,
	'&': exprTokenAmpersand,
	'{': exprTokenLBrace,
	'}': exprTokenRBrace,
	'[': exprTokenLBracket,
	']': exprTokenRBracket,
	'(': exprTokenLParen,
	')': exprTokenRParen,
	':': exprTokenColon,
	',': exprTokenComma,
}

// isExprPathStop 은 경로 토큰을 종료시키는 문자인지 판단한다.
// 공백, 연산자, 괄호, 구분자에서 경로가 끝난다.
func isExprPathStop(ch rune) bool {
	if _, ok := exprSingleCharTokens[ch]; ok {
		return true
	}
	return unicode.IsSpace(ch)
}

// isExprPathChar 은 경로 세그먼트 내에서 허용되는 문자인지 판단한다.
func isExprPathChar(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == '.'
}

// isIndexOrWildcard 은 대괄호 내부 rune 슬라이스가 정수 인덱스 또는 와일드카드인지 판단한다.
// JSONPath 의 $.a[0], $.items[*] 등에서 경로 일부로 수용할지 결정하는 게이트로 사용된다.
func isIndexOrWildcard(inner []rune) bool {
	if len(inner) == 0 {
		return false
	}
	if len(inner) == 1 && inner[0] == '*' {
		return true
	}
	for _, r := range inner {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// exprTokenize 는 변환 표현식 문자열을 토큰 슬라이스로 변환한다.
// 문자별 스캔으로 JSONPath, 변수, 연산자, 리터럴, 식별자를 인식한다.
func exprTokenize(expr string) ([]exprToken, error) {
	var tokens []exprToken
	runes := []rune(expr)
	i := 0

	for i < len(runes) {
		ch := runes[i]

		// 공백 건너뛰기
		if unicode.IsSpace(ch) {
			i++
			continue
		}

		// $ 로 시작: Path 또는 Var
		if ch == '$' {
			start := i
			i++ // $ 소비

			// 입력 끝이면 에러
			if i >= len(runes) || (!unicode.IsLetter(runes[i]) && runes[i] != '_' && runes[i] != '.') {
				return nil, fmt.Errorf("%w: unexpected '$' at position %d", ErrInvalidExpression, start)
			}

			if runes[i] == '.' {
				// Path 토큰: $. 으로 시작
				i++ // . 소비
				for i < len(runes) {
					if isExprPathChar(runes[i]) {
						i++
						continue
					}
					// 배열 인덱스 구문 [N] 또는 [*] 를 경로의 일부로 소비한다.
					// 대괄호 내용이 정수 리터럴이거나 '*' 일 때만 경로에 포함하고,
					// 그 외(식별자/표현식 등)는 경로 종료로 해석해 뒤이은 토큰으로 남긴다.
					if runes[i] == '[' {
						// 닫는 ']' 검색
						j := i + 1
						for j < len(runes) && runes[j] != ']' {
							j++
						}
						if j >= len(runes) {
							// 경로 내부의 미종료 '[' — 경로 여기서 종료, 이후 '[' 는 별도 토큰으로 처리
							break
						}
						inner := runes[i+1 : j]
						if isIndexOrWildcard(inner) {
							i = j + 1 // '[...]' 전체 소비
							continue
						}
					}
					break
				}
				tokens = append(tokens, exprToken{typ: exprTokenPath, value: string(runes[start:i]), pos: start})
			} else {
				// Var 토큰: $ + 문자/밑줄로 시작
				for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_') {
					i++
				}
				// value 는 $ 를 제외한 변수명
				tokens = append(tokens, exprToken{typ: exprTokenVar, value: string(runes[start+1 : i]), pos: start})
			}
			continue
		}

		// 단일 문자 토큰: 연산자 및 구분자
		if typ, ok := exprSingleCharTokens[ch]; ok {
			tokens = append(tokens, exprToken{typ: typ, value: string(ch), pos: i})
			i++
			continue
		}

		// 문자열 리터럴: '...' 또는 "..."
		if ch == '\'' || ch == '"' {
			quote := ch
			start := i
			i++ // 여는 따옴표 소비
			for i < len(runes) && runes[i] != quote {
				i++
			}
			if i >= len(runes) {
				return nil, fmt.Errorf("%w: unterminated string at position %d", ErrInvalidExpression, start)
			}
			value := string(runes[start+1 : i])
			i++ // 닫는 따옴표 소비
			tokens = append(tokens, exprToken{typ: exprTokenString, value: value, pos: start})
			continue
		}

		// 숫자 리터럴: 양수만 (음수는 Minus + Number)
		if unicode.IsDigit(ch) {
			start := i
			for i < len(runes) && unicode.IsDigit(runes[i]) {
				i++
			}
			if i < len(runes) && runes[i] == '.' {
				i++ // . 소비
				for i < len(runes) && unicode.IsDigit(runes[i]) {
					i++
				}
			}
			tokens = append(tokens, exprToken{typ: exprTokenNumber, value: string(runes[start:i]), pos: start})
			continue
		}

		// 키워드 및 식별자: true, false, null, 또는 일반 식별자
		if unicode.IsLetter(ch) || ch == '_' {
			start := i
			for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_') {
				i++
			}
			word := string(runes[start:i])
			switch word {
			case "true", "false":
				tokens = append(tokens, exprToken{typ: exprTokenBool, value: word, pos: start})
			case "null":
				tokens = append(tokens, exprToken{typ: exprTokenNull, value: word, pos: start})
			default:
				tokens = append(tokens, exprToken{typ: exprTokenIdent, value: word, pos: start})
			}
			continue
		}

		// 인식할 수 없는 문자
		return nil, fmt.Errorf("%w: unexpected character %q at position %d", ErrInvalidExpression, string(ch), i)
	}

	tokens = append(tokens, exprToken{typ: exprTokenEOF, value: "", pos: i})
	return tokens, nil
}
