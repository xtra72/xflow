package node

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/xtra/xflow/pkg/message"
)

// =============================================================================
// 토큰 정의 (렉서)
// =============================================================================

// condTokenType 은 조건 표현식의 토큰 타입을 나타낸다.
type condTokenType int

const (
	condTokenPath      condTokenType = iota // $.payload.xxx
	condTokenNumber                         // 42, -40.5, 3.14
	condTokenString                         // 'active', "active"
	condTokenBool                           // true, false
	condTokenNull                           // null
	condTokenCompareOp                      // >=, <=, ==, !=, >, <
	condTokenLogicalOp                      // &&, ||
	condTokenNot                            // !
	condTokenLParen                         // (
	condTokenRParen                         // )
	condTokenFunc                           // exists
	condTokenEOF                            // 입력 종료
)

// condToken 은 렉서가 생성하는 단일 토큰이다.
type condToken struct {
	typ   condTokenType
	value string
	pos   int
}

// tokenize 는 조건 표현식 문자열을 토큰 슬라이스로 변환한다.
// 문자별 스캔으로 JSONPath, 비교/논리 연산자, 리터럴, 함수를 인식한다.
func tokenize(expr string) ([]condToken, error) {
	var tokens []condToken
	runes := []rune(expr)
	i := 0

	for i < len(runes) {
		ch := runes[i]

		// 공백 건너뛰기
		if unicode.IsSpace(ch) {
			i++
			continue
		}

		// JSONPath: $ 로 시작
		if ch == '$' {
			start := i
			i++ // $ 소비
			for i < len(runes) && (runes[i] == '.' || runes[i] == '[' || runes[i] == ']' ||
				unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_' || runes[i] == '*') {
				i++
			}
			tokens = append(tokens, condToken{typ: condTokenPath, value: string(runes[start:i]), pos: start})
			continue
		}

		// 비교 연산자: >=, <=, ==, !=, >, <
		if ch == '>' || ch == '<' || ch == '=' {
			start := i
			i++
			if i < len(runes) && runes[i] == '=' {
				i++
			}
			tokens = append(tokens, condToken{typ: condTokenCompareOp, value: string(runes[start:i]), pos: start})
			continue
		}

		// ! 연산자: != (비교) 또는 ! (NOT)
		if ch == '!' {
			if i+1 < len(runes) && runes[i+1] == '=' {
				tokens = append(tokens, condToken{typ: condTokenCompareOp, value: "!=", pos: i})
				i += 2
			} else {
				tokens = append(tokens, condToken{typ: condTokenNot, value: "!", pos: i})
				i++
			}
			continue
		}

		// 논리 연산자: &&, ||
		if ch == '&' {
			if i+1 < len(runes) && runes[i+1] == '&' {
				tokens = append(tokens, condToken{typ: condTokenLogicalOp, value: "&&", pos: i})
				i += 2
				continue
			}
			return nil, fmt.Errorf("%w: unexpected '&' at position %d, expected '&&'", ErrInvalidExpression, i)
		}
		if ch == '|' {
			if i+1 < len(runes) && runes[i+1] == '|' {
				tokens = append(tokens, condToken{typ: condTokenLogicalOp, value: "||", pos: i})
				i += 2
				continue
			}
			return nil, fmt.Errorf("%w: unexpected '|' at position %d, expected '||'", ErrInvalidExpression, i)
		}

		// 괄호
		if ch == '(' {
			tokens = append(tokens, condToken{typ: condTokenLParen, value: "(", pos: i})
			i++
			continue
		}
		if ch == ')' {
			tokens = append(tokens, condToken{typ: condTokenRParen, value: ")", pos: i})
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
			tokens = append(tokens, condToken{typ: condTokenString, value: value, pos: start})
			continue
		}

		// 숫자 리터럴: 양수, 음수, 소수
		// 음수는 이전 토큰이 비교 연산자이거나, 논리 연산자이거나, ( 이거나, 첫 번째 토큰일 때만
		if unicode.IsDigit(ch) || (ch == '-' && i+1 < len(runes) && unicode.IsDigit(runes[i+1]) && isNegativeNumberContext(tokens)) {
			start := i
			if ch == '-' {
				i++ // - 소비
			}
			for i < len(runes) && unicode.IsDigit(runes[i]) {
				i++
			}
			if i < len(runes) && runes[i] == '.' {
				i++ // . 소비
				for i < len(runes) && unicode.IsDigit(runes[i]) {
					i++
				}
			}
			tokens = append(tokens, condToken{typ: condTokenNumber, value: string(runes[start:i]), pos: start})
			continue
		}

		// 키워드: true, false, null, exists
		if unicode.IsLetter(ch) {
			start := i
			for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_') {
				i++
			}
			word := string(runes[start:i])
			switch word {
			case "true", "false":
				tokens = append(tokens, condToken{typ: condTokenBool, value: word, pos: start})
			case "null":
				tokens = append(tokens, condToken{typ: condTokenNull, value: word, pos: start})
			case "exists":
				tokens = append(tokens, condToken{typ: condTokenFunc, value: word, pos: start})
			default:
				return nil, fmt.Errorf("%w: unknown keyword %q at position %d", ErrInvalidExpression, word, start)
			}
			continue
		}

		return nil, fmt.Errorf("%w: unexpected character %q at position %d", ErrInvalidExpression, string(ch), i)
	}

	tokens = append(tokens, condToken{typ: condTokenEOF, value: "", pos: i})
	return tokens, nil
}

// isNegativeNumberContext 는 현재 위치에서 - 가 음수를 나타내는지 판단한다.
// 이전 토큰이 비교 연산자, 논리 연산자, (, ! 이거나 토큰이 없을 때 음수로 판단한다.
func isNegativeNumberContext(tokens []condToken) bool {
	if len(tokens) == 0 {
		return true
	}
	last := tokens[len(tokens)-1]
	return last.typ == condTokenCompareOp ||
		last.typ == condTokenLogicalOp ||
		last.typ == condTokenLParen ||
		last.typ == condTokenNot
}

// =============================================================================
// AST 노드 정의 (파서)
// =============================================================================

// condNode 는 조건 표현식 AST의 노드 인터페이스이다.
type condNode interface {
	evaluate(data map[string]any) (bool, error)
}

// comparisonNode 는 경로와 리터럴 값의 비교를 나타낸다.
type comparisonNode struct {
	path    string // JSONPath (예: "$.payload.temperature")
	op      string // 비교 연산자
	literal any    // 리터럴 값 (float64, string, bool, nil)
}

// logicalNode 는 두 조건의 논리 결합(&&, ||)을 나타낸다.
type logicalNode struct {
	left  condNode
	op    string // "&&" 또는 "||"
	right condNode
}

// notNode 는 조건의 부정(!)을 나타낸다.
type notNode struct {
	expr condNode
}

// existsNode 는 경로의 존재 여부 확인을 나타낸다.
type existsNode struct {
	path string
}

// =============================================================================
// 파서 (재귀 하강)
// =============================================================================

// condParser 는 토큰 슬라이스를 AST로 변환하는 파서이다.
type condParser struct {
	tokens []condToken
	pos    int
}

// parseCondition 은 토큰 슬라이스를 조건 AST로 파싱한다.
func parseCondition(tokens []condToken) (condNode, error) {
	if len(tokens) == 0 || (len(tokens) == 1 && tokens[0].typ == condTokenEOF) {
		return nil, fmt.Errorf("%w: empty condition expression", ErrInvalidExpression)
	}

	p := &condParser{tokens: tokens, pos: 0}
	node, err := p.parseOrExpr()
	if err != nil {
		return nil, err
	}

	// 모든 토큰이 소비되었는지 확인 (EOF만 남아야 함)
	if p.current().typ != condTokenEOF {
		return nil, fmt.Errorf("%w: unexpected token %q at position %d",
			ErrInvalidExpression, p.current().value, p.current().pos)
	}

	return node, nil
}

// current 는 현재 토큰을 반환한다.
func (p *condParser) current() condToken {
	if p.pos >= len(p.tokens) {
		return condToken{typ: condTokenEOF}
	}
	return p.tokens[p.pos]
}

// advance 는 다음 토큰으로 이동하고 이전 토큰을 반환한다.
func (p *condParser) advance() condToken {
	tok := p.current()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}

// expect 는 현재 토큰이 기대 타입인지 확인하고 소비한다.
func (p *condParser) expect(typ condTokenType) (condToken, error) {
	tok := p.current()
	if tok.typ != typ {
		return tok, fmt.Errorf("%w: expected token type %d, got %q at position %d",
			ErrInvalidExpression, typ, tok.value, tok.pos)
	}
	return p.advance(), nil
}

// parseOrExpr 는 || 연산을 파싱한다 (가장 낮은 우선순위).
func (p *condParser) parseOrExpr() (condNode, error) {
	left, err := p.parseAndExpr()
	if err != nil {
		return nil, err
	}

	for p.current().typ == condTokenLogicalOp && p.current().value == "||" {
		p.advance() // || 소비
		right, err := p.parseAndExpr()
		if err != nil {
			return nil, err
		}
		left = &logicalNode{left: left, op: "||", right: right}
	}

	return left, nil
}

// parseAndExpr 는 && 연산을 파싱한다.
func (p *condParser) parseAndExpr() (condNode, error) {
	left, err := p.parseUnaryExpr()
	if err != nil {
		return nil, err
	}

	for p.current().typ == condTokenLogicalOp && p.current().value == "&&" {
		p.advance() // && 소비
		right, err := p.parseUnaryExpr()
		if err != nil {
			return nil, err
		}
		left = &logicalNode{left: left, op: "&&", right: right}
	}

	return left, nil
}

// parseUnaryExpr 는 ! (NOT) 연산을 파싱한다.
func (p *condParser) parseUnaryExpr() (condNode, error) {
	if p.current().typ == condTokenNot {
		p.advance() // ! 소비
		expr, err := p.parseUnaryExpr()
		if err != nil {
			return nil, err
		}
		return &notNode{expr: expr}, nil
	}

	return p.parsePrimaryExpr()
}

// parsePrimaryExpr 는 기본 표현식 (비교, exists, 괄호)을 파싱한다.
func (p *condParser) parsePrimaryExpr() (condNode, error) {
	tok := p.current()

	// 괄호 그룹: ( expr )
	if tok.typ == condTokenLParen {
		p.advance() // ( 소비
		node, err := p.parseOrExpr()
		if err != nil {
			return nil, err
		}
		_, err = p.expect(condTokenRParen)
		if err != nil {
			return nil, fmt.Errorf("%w: missing closing parenthesis", ErrInvalidExpression)
		}
		return node, nil
	}

	// exists 함수: exists($.path)
	if tok.typ == condTokenFunc && tok.value == "exists" {
		p.advance() // exists 소비
		_, err := p.expect(condTokenLParen)
		if err != nil {
			return nil, fmt.Errorf("%w: expected '(' after exists", ErrInvalidExpression)
		}
		pathTok, err := p.expect(condTokenPath)
		if err != nil {
			return nil, fmt.Errorf("%w: expected path in exists()", ErrInvalidExpression)
		}
		_, err = p.expect(condTokenRParen)
		if err != nil {
			return nil, fmt.Errorf("%w: expected ')' after exists path", ErrInvalidExpression)
		}
		return &existsNode{path: pathTok.value}, nil
	}

	// 비교 표현식: $.path op literal
	if tok.typ == condTokenPath {
		pathTok := p.advance() // 경로 소비

		opTok, err := p.expect(condTokenCompareOp)
		if err != nil {
			return nil, fmt.Errorf("%w: expected comparison operator after path %q", ErrInvalidExpression, pathTok.value)
		}

		literal, err := p.parseLiteral()
		if err != nil {
			return nil, err
		}

		return &comparisonNode{path: pathTok.value, op: opTok.value, literal: literal}, nil
	}

	return nil, fmt.Errorf("%w: unexpected token %q at position %d",
		ErrInvalidExpression, tok.value, tok.pos)
}

// parseLiteral 은 리터럴 값(숫자, 문자열, bool, null)을 파싱한다.
func (p *condParser) parseLiteral() (any, error) {
	tok := p.current()

	switch tok.typ {
	case condTokenNumber:
		p.advance()
		val, err := strconv.ParseFloat(tok.value, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid number %q", ErrInvalidExpression, tok.value)
		}
		return val, nil

	case condTokenString:
		p.advance()
		return tok.value, nil

	case condTokenBool:
		p.advance()
		return tok.value == "true", nil

	case condTokenNull:
		p.advance()
		return nil, nil

	default:
		return nil, fmt.Errorf("%w: expected literal value, got %q at position %d",
			ErrInvalidExpression, tok.value, tok.pos)
	}
}

// =============================================================================
// 평가기 (각 노드의 evaluate 구현)
// =============================================================================

// evaluate 는 비교 노드를 평가한다.
// 경로에서 값을 조회하고 리터럴과 비교한다.
func (n *comparisonNode) evaluate(data map[string]any) (bool, error) {
	payload := message.NewPayload(data)
	val, err := payload.GetPath(n.path)

	// null 비교 특수 처리
	if n.literal == nil {
		if err != nil {
			// 경로 미존재: == null → true, != null → false
			if errors.Is(err, message.ErrPathNotFound) {
				return n.op == "==", nil
			}
			return false, err
		}
		// 경로 존재, 값이 nil 인지 확인
		if val == nil {
			return n.op == "==", nil
		}
		return n.op == "!=", nil
	}

	// 경로 미존재 시 false 반환 (null 비교 제외)
	if err != nil {
		if errors.Is(err, message.ErrPathNotFound) {
			return false, nil
		}
		return false, err
	}

	// 타입별 비교
	switch lit := n.literal.(type) {
	case float64:
		return compareNumbers(val, lit, n.op)
	case string:
		return compareStrings(val, lit, n.op)
	case bool:
		return compareBools(val, lit, n.op)
	default:
		return false, nil
	}
}

// evaluate 는 논리 노드를 평가한다.
// 단락 평가(short-circuit evaluation)를 적용한다.
func (n *logicalNode) evaluate(data map[string]any) (bool, error) {
	leftVal, err := n.left.evaluate(data)
	if err != nil {
		return false, err
	}

	switch n.op {
	case "&&":
		if !leftVal {
			return false, nil // 단락: 왼쪽이 false면 오른쪽 평가 안 함
		}
		return n.right.evaluate(data)
	case "||":
		if leftVal {
			return true, nil // 단락: 왼쪽이 true면 오른쪽 평가 안 함
		}
		return n.right.evaluate(data)
	default:
		return false, fmt.Errorf("%w: unknown logical operator %q", ErrInvalidExpression, n.op)
	}
}

// evaluate 는 NOT 노드를 평가한다.
func (n *notNode) evaluate(data map[string]any) (bool, error) {
	val, err := n.expr.evaluate(data)
	if err != nil {
		return false, err
	}
	return !val, nil
}

// evaluate 는 exists 노드를 평가한다.
// 경로가 존재하면 true, ErrPathNotFound면 false를 반환한다.
func (n *existsNode) evaluate(data map[string]any) (bool, error) {
	payload := message.NewPayload(data)
	_, err := payload.GetPath(n.path)
	if err != nil {
		if errors.Is(err, message.ErrPathNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// =============================================================================
// 비교 헬퍼 함수
// =============================================================================

// compareNumbers 는 숫자 비교를 수행한다.
// val이 숫자 타입(float64, int, int64 등)이면 float64로 변환하여 비교한다.
func compareNumbers(val any, lit float64, op string) (bool, error) {
	num, ok := toFloat64(val)
	if !ok {
		return false, nil
	}

	switch op {
	case ">":
		return num > lit, nil
	case ">=":
		return num >= lit, nil
	case "<":
		return num < lit, nil
	case "<=":
		return num <= lit, nil
	case "==":
		return num == lit, nil
	case "!=":
		return num != lit, nil
	default:
		return false, nil
	}
}

// compareStrings 는 문자열 비교를 수행한다.
// == 와 != 만 지원하며, 그 외 연산자는 false를 반환한다.
func compareStrings(val any, lit string, op string) (bool, error) {
	str, ok := val.(string)
	if !ok {
		return false, nil
	}

	switch op {
	case "==":
		return str == lit, nil
	case "!=":
		return str != lit, nil
	default:
		// 문자열에 >, <, >=, <= 적용 시 false 반환
		return false, nil
	}
}

// compareBools 는 불리언 비교를 수행한다.
// == 와 != 만 지원한다.
func compareBools(val any, lit bool, op string) (bool, error) {
	b, ok := val.(bool)
	if !ok {
		return false, nil
	}

	switch op {
	case "==":
		return b == lit, nil
	case "!=":
		return b != lit, nil
	default:
		return false, nil
	}
}

// toFloat64 는 aggregate.go에 정의되어 있다 (패키지 내 공유 함수).

// =============================================================================
// 진입점
// =============================================================================

// compileCondition 은 조건 표현식 문자열을 FilterCondition으로 컴파일한다.
// 렉서 → 파서 → AST → 클로저 형태의 FilterCondition을 반환한다.
//
// 지원 표현식 예시:
//   - $.payload.temperature >= 30
//   - $.payload.status == 'active'
//   - exists($.payload.error)
//   - !exists($.payload.error) && $.payload.value > 0
func compileCondition(expr string) (FilterCondition, error) {
	eval, err := compileConditionData(expr)
	if err != nil {
		return nil, err
	}
	return func(msg message.Message) bool {
		return eval(messageToMap(msg))
	}, nil
}

// compileConditionData 는 조건식을 data 맵에 직접 평가하는 함수로 컴파일한다.
// compileCondition 과 달리 message 래핑/messageToMap 깊은 복사 없이, 호출자가
// 구성한 data 맵({"payload": item} 등)을 그대로 AST 에 평가한다. 평가는 맵을
// 읽기만 하므로(GetPath) 복사가 불필요하다. inventory 처럼 대량 항목을 항목당
// 평가할 때 항목당 2회 깊은 복사를 제거하기 위해 사용한다.
//
// 호출자는 $. 루트에 맞춰 data 를 구성해야 한다(예: 항목 필드를 $.payload.X 로
// 참조하려면 data = {"payload": item}).
func compileConditionData(expr string) (func(map[string]any) bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("%w: empty condition expression", ErrInvalidExpression)
	}

	tokens, err := tokenize(expr)
	if err != nil {
		return nil, err
	}

	ast, err := parseCondition(tokens)
	if err != nil {
		return nil, err
	}

	return func(data map[string]any) bool {
		result, err := ast.evaluate(data)
		if err != nil {
			return false
		}
		return result
	}, nil
}
