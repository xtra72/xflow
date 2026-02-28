package node

import (
	"fmt"
	"strconv"
)

// =============================================================================
// AST 노드 정의
// =============================================================================

// ExprNode 는 표현식 AST의 노드 인터페이스이다.
type ExprNode interface {
	exprNode() // 마커 메서드
}

// ObjectField 는 오브젝트 리터럴의 단일 필드를 나타낸다.
type ObjectField struct {
	Key   string
	Value ExprNode
}

// ObjectNode 는 오브젝트 리터럴 { key: value, ... } 를 나타낸다.
type ObjectNode struct {
	Fields []ObjectField
}

// BinaryNode 는 산술 연산 (left op right) 을 나타낸다.
type BinaryNode struct {
	Left  ExprNode
	Op    string // "+", "-", "*", "/", "%"
	Right ExprNode
}

// ConcatNode 는 문자열 연결 (left & right) 을 나타낸다.
type ConcatNode struct {
	Left  ExprNode
	Right ExprNode
}

// PathNode 는 JSONPath 경로 ($.payload.field) 를 나타낸다.
type PathNode struct {
	Path string
}

// VarNode 는 변수 참조 ($var_name) 를 나타낸다.
type VarNode struct {
	Name string
}

// IndexNode 는 인덱스/룩업 ($var[key_expr]) 을 나타낸다.
type IndexNode struct {
	Target ExprNode // 보통 VarNode
	Key    ExprNode
}

// CallNode 는 함수 호출 (func_name(args...)) 을 나타낸다.
type CallNode struct {
	Name string
	Args []ExprNode
}

// LiteralNode 는 리터럴 값 (42, "hello", true, null) 을 나타낸다.
type LiteralNode struct {
	Value any // float64, string, bool, 또는 nil
}

// ExprNode 인터페이스 구현 (마커 메서드)
func (*ObjectNode)  exprNode() {}
func (*BinaryNode)  exprNode() {}
func (*ConcatNode)  exprNode() {}
func (*PathNode)    exprNode() {}
func (*VarNode)     exprNode() {}
func (*IndexNode)   exprNode() {}
func (*CallNode)    exprNode() {}
func (*LiteralNode) exprNode() {}

// =============================================================================
// 파서 구현
// =============================================================================

// exprParser 는 토큰 슬라이스를 AST로 변환하는 재귀 하강 파서이다.
type exprParser struct {
	tokens []exprToken
	pos    int
}

// exprParse 는 토큰 슬라이스를 AST로 파싱한다.
// 최상위는 반드시 ObjectNode여야 한다.
func exprParse(tokens []exprToken) (ExprNode, error) {
	if len(tokens) == 0 || (len(tokens) == 1 && tokens[0].typ == exprTokenEOF) {
		return nil, fmt.Errorf("%w: empty expression", ErrInvalidExpression)
	}

	p := &exprParser{tokens: tokens, pos: 0}

	// 최상위는 반드시 ObjectNode여야 한다
	if p.current().typ != exprTokenLBrace {
		return nil, fmt.Errorf("%w: expression must start with '{', got %q at position %d",
			ErrInvalidExpression, p.current().value, p.current().pos)
	}

	node, err := p.parseObjectLiteral()
	if err != nil {
		return nil, err
	}

	// 모든 토큰이 소비되었는지 확인 (EOF만 남아야 함)
	if p.current().typ != exprTokenEOF {
		return nil, fmt.Errorf("%w: unexpected token %q at position %d",
			ErrInvalidExpression, p.current().value, p.current().pos)
	}

	return node, nil
}

// current 는 현재 토큰을 반환한다.
func (p *exprParser) current() exprToken {
	if p.pos >= len(p.tokens) {
		return exprToken{typ: exprTokenEOF}
	}
	return p.tokens[p.pos]
}

// advance 는 다음 토큰으로 이동하고 이전 토큰을 반환한다.
func (p *exprParser) advance() exprToken {
	tok := p.current()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}

// expect 는 현재 토큰이 기대 타입인지 확인하고 소비한다.
func (p *exprParser) expect(typ exprTokenType) (exprToken, error) {
	tok := p.current()
	if tok.typ != typ {
		return tok, fmt.Errorf("%w: expected token type %d, got %q at position %d",
			ErrInvalidExpression, typ, tok.value, tok.pos)
	}
	return p.advance(), nil
}

// =============================================================================
// 문법 규칙 메서드 (우선순위 낮은 것부터)
// =============================================================================

// parseObjectLiteral 은 오브젝트 리터럴 { key: value, ... } 을 파싱한다.
func (p *exprParser) parseObjectLiteral() (*ObjectNode, error) {
	_, err := p.expect(exprTokenLBrace)
	if err != nil {
		return nil, fmt.Errorf("%w: expected '{'", ErrInvalidExpression)
	}

	var fields []ObjectField

	// 빈 오브젝트가 아닌 경우 필드 파싱
	for p.current().typ != exprTokenRBrace {
		if len(fields) > 0 {
			_, err := p.expect(exprTokenComma)
			if err != nil {
				return nil, fmt.Errorf("%w: expected ',' between fields at position %d",
					ErrInvalidExpression, p.current().pos)
			}
		}

		// 키는 식별자(exprTokenIdent)여야 한다
		keyTok, err := p.expect(exprTokenIdent)
		if err != nil {
			return nil, fmt.Errorf("%w: expected identifier as field key, got %q at position %d",
				ErrInvalidExpression, p.current().value, p.current().pos)
		}

		_, err = p.expect(exprTokenColon)
		if err != nil {
			return nil, fmt.Errorf("%w: expected ':' after key %q at position %d",
				ErrInvalidExpression, keyTok.value, p.current().pos)
		}

		value, err := p.parseValueExpr()
		if err != nil {
			return nil, err
		}

		fields = append(fields, ObjectField{Key: keyTok.value, Value: value})
	}

	_, err = p.expect(exprTokenRBrace)
	if err != nil {
		return nil, fmt.Errorf("%w: expected '}'", ErrInvalidExpression)
	}

	return &ObjectNode{Fields: fields}, nil
}

// parseValueExpr 는 값 표현식을 파싱한다 (= concat_expr).
func (p *exprParser) parseValueExpr() (ExprNode, error) {
	return p.parseConcatExpr()
}

// parseConcatExpr 는 문자열 연결 표현식을 파싱한다.
// concat_expr = additive_expr { "&" additive_expr }
func (p *exprParser) parseConcatExpr() (ExprNode, error) {
	left, err := p.parseAdditiveExpr()
	if err != nil {
		return nil, err
	}

	for p.current().typ == exprTokenAmpersand {
		p.advance() // & 소비
		right, err := p.parseAdditiveExpr()
		if err != nil {
			return nil, err
		}
		left = &ConcatNode{Left: left, Right: right}
	}

	return left, nil
}

// parseAdditiveExpr 는 덧셈/뺄셈 표현식을 파싱한다.
// additive_expr = multiplicative_expr { ("+" | "-") multiplicative_expr }
func (p *exprParser) parseAdditiveExpr() (ExprNode, error) {
	left, err := p.parseMultiplicativeExpr()
	if err != nil {
		return nil, err
	}

	for p.current().typ == exprTokenPlus || p.current().typ == exprTokenMinus {
		op := p.advance() // 연산자 소비
		right, err := p.parseMultiplicativeExpr()
		if err != nil {
			return nil, err
		}
		left = &BinaryNode{Left: left, Op: op.value, Right: right}
	}

	return left, nil
}

// parseMultiplicativeExpr 는 곱셈/나눗셈/모듈로 표현식을 파싱한다.
// multiplicative_expr = unary_expr { ("*" | "/" | "%") unary_expr }
func (p *exprParser) parseMultiplicativeExpr() (ExprNode, error) {
	left, err := p.parseUnaryExpr()
	if err != nil {
		return nil, err
	}

	for p.current().typ == exprTokenStar || p.current().typ == exprTokenSlash || p.current().typ == exprTokenPercent {
		op := p.advance() // 연산자 소비
		right, err := p.parseUnaryExpr()
		if err != nil {
			return nil, err
		}
		left = &BinaryNode{Left: left, Op: op.value, Right: right}
	}

	return left, nil
}

// parseUnaryExpr 는 단항 표현식을 파싱한다.
// unary_expr = ["-"] primary_expr
func (p *exprParser) parseUnaryExpr() (ExprNode, error) {
	if p.current().typ == exprTokenMinus {
		p.advance() // - 소비

		// 최적화: - 뒤에 숫자가 오면 음수 리터럴로 처리
		if p.current().typ == exprTokenNumber {
			numTok := p.advance()
			val, err := strconv.ParseFloat(numTok.value, 64)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid number %q", ErrInvalidExpression, numTok.value)
			}
			return &LiteralNode{Value: -val}, nil
		}

		// 일반 경우: BinaryNode{LiteralNode(0), "-", expr}
		expr, err := p.parsePrimaryExpr()
		if err != nil {
			return nil, err
		}
		return &BinaryNode{Left: &LiteralNode{Value: float64(0)}, Op: "-", Right: expr}, nil
	}

	return p.parsePrimaryExpr()
}

// parsePrimaryExpr 는 기본 표현식을 파싱한다.
// primary_expr = path | var_expr | func_call | literal | object | "(" value_expr ")"
func (p *exprParser) parsePrimaryExpr() (ExprNode, error) {
	tok := p.current()

	switch tok.typ {
	// JSONPath 경로: $.payload.field
	case exprTokenPath:
		p.advance()
		return &PathNode{Path: tok.value}, nil

	// 변수 참조: $var 또는 $var[key]
	case exprTokenVar:
		p.advance()
		var node ExprNode = &VarNode{Name: tok.value}

		// [ 가 오면 IndexNode로 변환
		if p.current().typ == exprTokenLBracket {
			p.advance() // [ 소비
			key, err := p.parseValueExpr()
			if err != nil {
				return nil, err
			}
			_, err = p.expect(exprTokenRBracket)
			if err != nil {
				return nil, fmt.Errorf("%w: expected ']' after index expression at position %d",
					ErrInvalidExpression, p.current().pos)
			}
			node = &IndexNode{Target: node, Key: key}
		}

		return node, nil

	// 식별자: 함수 호출 (ident 뒤에 '(' 가 오는 경우)
	case exprTokenIdent:
		// Ident 뒤에 ( 가 오면 함수 호출
		if p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].typ == exprTokenLParen {
			return p.parseFuncCall()
		}
		// Ident 단독은 값 표현식으로 유효하지 않다 (객체 키에서만 사용)
		return nil, fmt.Errorf("%w: unexpected identifier %q at position %d (identifiers are only valid as object keys or function names)",
			ErrInvalidExpression, tok.value, tok.pos)

	// 숫자 리터럴
	case exprTokenNumber:
		p.advance()
		val, err := strconv.ParseFloat(tok.value, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid number %q", ErrInvalidExpression, tok.value)
		}
		return &LiteralNode{Value: val}, nil

	// 문자열 리터럴
	case exprTokenString:
		p.advance()
		return &LiteralNode{Value: tok.value}, nil

	// bool 리터럴
	case exprTokenBool:
		p.advance()
		return &LiteralNode{Value: tok.value == "true"}, nil

	// null 리터럴
	case exprTokenNull:
		p.advance()
		return &LiteralNode{Value: nil}, nil

	// 중첩 오브젝트 리터럴
	case exprTokenLBrace:
		return p.parseObjectLiteral()

	// 괄호 그룹: ( value_expr )
	case exprTokenLParen:
		p.advance() // ( 소비
		expr, err := p.parseValueExpr()
		if err != nil {
			return nil, err
		}
		_, err = p.expect(exprTokenRParen)
		if err != nil {
			return nil, fmt.Errorf("%w: expected ')' at position %d",
				ErrInvalidExpression, p.current().pos)
		}
		return expr, nil

	default:
		return nil, fmt.Errorf("%w: unexpected token %q at position %d",
			ErrInvalidExpression, tok.value, tok.pos)
	}
}

// parseFuncCall 은 함수 호출 name(args...) 을 파싱한다.
func (p *exprParser) parseFuncCall() (*CallNode, error) {
	nameTok := p.advance() // 함수 이름 소비
	p.advance()            // ( 소비

	var args []ExprNode

	// 인자 파싱
	if p.current().typ != exprTokenRParen {
		arg, err := p.parseValueExpr()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)

		for p.current().typ == exprTokenComma {
			p.advance() // , 소비
			arg, err := p.parseValueExpr()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
		}
	}

	_, err := p.expect(exprTokenRParen)
	if err != nil {
		return nil, fmt.Errorf("%w: expected ')' after function arguments at position %d",
			ErrInvalidExpression, p.current().pos)
	}

	return &CallNode{Name: nameTok.value, Args: args}, nil
}
