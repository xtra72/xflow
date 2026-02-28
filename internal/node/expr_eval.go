package node

import (
	"errors"
	"fmt"
	"math"

	"github.com/xtra/xflow/pkg/message"
)

// =============================================================================
// 내장 함수 타입 및 평가 컨텍스트
// =============================================================================

// BuiltinFunc 는 내장 함수의 시그니처이다.
type BuiltinFunc func(args []any) (any, error)

// EvalContext 는 표현식 평가 시 필요한 컨텍스트이다.
type EvalContext struct {
	data      map[string]any         // messageToMap(msg) 결과
	variables map[string]any         // config에서 수집한 변수 바인딩
	functions map[string]BuiltinFunc // 내장 함수 레지스트리
}

// NewEvalContext 는 새 평가 컨텍스트를 생성한다.
func NewEvalContext(data map[string]any, variables map[string]any, functions map[string]BuiltinFunc) *EvalContext {
	if data == nil {
		data = map[string]any{}
	}
	if variables == nil {
		variables = map[string]any{}
	}
	if functions == nil {
		functions = map[string]BuiltinFunc{}
	}
	return &EvalContext{
		data:      data,
		variables: variables,
		functions: functions,
	}
}

// =============================================================================
// AST 평가기
// =============================================================================

// exprEval 은 AST 노드를 평가하여 결과를 반환한다.
func exprEval(node ExprNode, ctx *EvalContext) (any, error) {
	switch n := node.(type) {
	case *LiteralNode:
		return evalLiteral(n)
	case *PathNode:
		return evalPath(n, ctx)
	case *VarNode:
		return evalVar(n, ctx)
	case *BinaryNode:
		return evalBinary(n, ctx)
	case *ConcatNode:
		return evalConcat(n, ctx)
	case *IndexNode:
		return evalIndex(n, ctx)
	case *CallNode:
		return evalCall(n, ctx)
	case *ObjectNode:
		return evalObject(n, ctx)
	default:
		return nil, fmt.Errorf("%w: unknown node type %T", ErrInvalidExpression, node)
	}
}

// evalLiteral 은 리터럴 노드를 평가한다.
func evalLiteral(n *LiteralNode) (any, error) {
	return n.Value, nil
}

// evalPath 는 JSONPath 경로 노드를 평가한다.
// 경로를 찾을 수 없으면 nil을 반환한다 (에러가 아님).
func evalPath(n *PathNode, ctx *EvalContext) (any, error) {
	payload := message.NewPayload(ctx.data)
	val, err := payload.GetPath(n.Path)
	if err != nil {
		if errors.Is(err, message.ErrPathNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return val, nil
}

// evalVar 는 변수 참조 노드를 평가한다.
func evalVar(n *VarNode, ctx *EvalContext) (any, error) {
	val, ok := ctx.variables[n.Name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUndefinedVariable, n.Name)
	}
	return val, nil
}

// evalBinary 는 산술 이항 연산 노드를 평가한다.
func evalBinary(n *BinaryNode, ctx *EvalContext) (any, error) {
	leftVal, err := exprEval(n.Left, ctx)
	if err != nil {
		return nil, err
	}
	rightVal, err := exprEval(n.Right, ctx)
	if err != nil {
		return nil, err
	}

	leftF, leftOk := toFloat64(leftVal)
	rightF, rightOk := toFloat64(rightVal)

	if !leftOk || !rightOk {
		return nil, fmt.Errorf("%w: cannot perform arithmetic on %T and %T",
			ErrTypeMismatch, leftVal, rightVal)
	}

	switch n.Op {
	case "+":
		return leftF + rightF, nil
	case "-":
		return leftF - rightF, nil
	case "*":
		return leftF * rightF, nil
	case "/":
		if rightF == 0 {
			return nil, fmt.Errorf("%w: %v / 0", ErrDivisionByZero, leftF)
		}
		return leftF / rightF, nil
	case "%":
		if rightF == 0 {
			return nil, fmt.Errorf("%w: %v %% 0", ErrDivisionByZero, leftF)
		}
		return math.Mod(leftF, rightF), nil
	default:
		return nil, fmt.Errorf("%w: unknown operator %q", ErrInvalidExpression, n.Op)
	}
}

// evalConcat 는 문자열 연결 노드를 평가한다.
func evalConcat(n *ConcatNode, ctx *EvalContext) (any, error) {
	leftVal, err := exprEval(n.Left, ctx)
	if err != nil {
		return nil, err
	}
	rightVal, err := exprEval(n.Right, ctx)
	if err != nil {
		return nil, err
	}

	return toString(leftVal) + toString(rightVal), nil
}

// toString 은 값을 문자열로 변환한다.
// nil → "", string → 그대로, 기타 → fmt.Sprintf("%v", val)
func toString(val any) string {
	if val == nil {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", val)
}

// evalIndex 는 인덱스/룩업 노드를 평가한다.
func evalIndex(n *IndexNode, ctx *EvalContext) (any, error) {
	targetVal, err := exprEval(n.Target, ctx)
	if err != nil {
		return nil, err
	}
	keyVal, err := exprEval(n.Key, ctx)
	if err != nil {
		return nil, err
	}

	keyStr := fmt.Sprintf("%v", keyVal)

	m, ok := targetVal.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: index target must be a map, got %T", ErrTypeMismatch, targetVal)
	}

	val, exists := m[keyStr]
	if !exists {
		return nil, nil
	}
	return val, nil
}

// evalCall 은 함수 호출 노드를 평가한다.
func evalCall(n *CallNode, ctx *EvalContext) (any, error) {
	fn, ok := ctx.functions[n.Name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUndefinedFunction, n.Name)
	}

	args := make([]any, len(n.Args))
	for i, argNode := range n.Args {
		val, err := exprEval(argNode, ctx)
		if err != nil {
			return nil, err
		}
		args[i] = val
	}

	return fn(args)
}

// evalObject 는 오브젝트 노드를 평가한다.
func evalObject(n *ObjectNode, ctx *EvalContext) (any, error) {
	result := make(map[string]any, len(n.Fields))
	for _, field := range n.Fields {
		val, err := exprEval(field.Value, ctx)
		if err != nil {
			return nil, err
		}
		result[field.Key] = val
	}
	return result, nil
}

// =============================================================================
// 표현식 컴파일 (v2)
// =============================================================================

// compileExpressionV2 는 expression 문자열을 새로운 파서로 파싱하고 TransformFunc로 컴파일한다.
func compileExpressionV2(expr string, mode TransformMode, vars map[string]any) (TransformFunc, error) {
	tokens, err := exprTokenize(expr)
	if err != nil {
		return nil, err
	}

	ast, err := exprParse(tokens)
	if err != nil {
		return nil, err
	}

	functions := defaultBuiltinFuncs()

	return func(msg message.Message) (message.Message, error) {
		data := messageToMap(msg)
		ctx := NewEvalContext(data, vars, functions)

		result, evalErr := exprEval(ast, ctx)
		if evalErr != nil {
			return nil, evalErr
		}

		// 결과를 map[string]any로 변환
		var resultMap map[string]any
		switch v := result.(type) {
		case map[string]any:
			resultMap = v
		default:
			// ObjectNode가 아닌 단일 값의 경우 "_result" 키로 래핑
			resultMap = map[string]any{"_result": result}
		}

		// 모드에 따른 페이로드 구성
		var payload map[string]any
		switch mode {
		case TransformModeMerge:
			// 원본 페이로드 복사 후 결과로 덮어쓰기
			payload = msg.Payload().ToMap()
			for k, v := range resultMap {
				payload[k] = v
			}
		default:
			// select: 결과만 사용
			payload = resultMap
		}

		// 새 메시지 생성 (메타데이터 보존)
		opts := []message.Option{
			message.WithPayload(message.NewPayload(payload)),
		}
		for k, v := range msg.Metadata().All() {
			opts = append(opts, message.WithMetadata(k, v))
		}
		return message.New(opts...), nil
	}, nil
}
