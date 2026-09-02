package node

import (
	"fmt"
	"strings"

	"github.com/xtra/xflow/pkg/message"
)

// expressionField 는 expression에서 파싱된 단일 필드 매핑이다.
// 출력 키(key)를 입력 JSONPath(path)에 대응시킨다.
type expressionField struct {
	key  string // 출력 필드 이름
	path string // 입력 JSONPath (예: "$.object.temperature")
}

// parseExpression 은 "{ key1: $.path1, key2: $.path2 }" 형식의 expression 문자열을
// expressionField 슬라이스로 파싱한다.
//
// 지원 형식:
//
//	{ device_id: $.deviceInfo.devEui, temperature: $.object.temperature }
//
// 규칙:
//   - 외부 중괄호 { } 필수
//   - 필드는 쉼표(,)로 구분
//   - 각 필드는 "key: $.path" 형식
//   - key는 공백 없는 단순 식별자
//   - path는 $.으로 시작하는 JSONPath 경로
func parseExpression(expr string) ([]expressionField, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("%w: empty expression", ErrInvalidExpression)
	}

	// 외부 중괄호 제거
	if !strings.HasPrefix(expr, "{") || !strings.HasSuffix(expr, "}") {
		return nil, fmt.Errorf("%w: expression must be enclosed in { }", ErrInvalidExpression)
	}
	inner := strings.TrimSpace(expr[1 : len(expr)-1])
	if inner == "" {
		return nil, fmt.Errorf("%w: empty expression body", ErrInvalidExpression)
	}

	// 쉼표로 필드 분할
	parts := strings.Split(inner, ",")
	fields := make([]expressionField, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// 첫 번째 콜론으로 key와 path 분리
		colonIdx := strings.Index(part, ":")
		if colonIdx < 0 {
			return nil, fmt.Errorf("%w: field %q missing colon separator", ErrInvalidExpression, part)
		}

		key := strings.TrimSpace(part[:colonIdx])
		path := strings.TrimSpace(part[colonIdx+1:])

		if key == "" {
			return nil, fmt.Errorf("%w: empty field key", ErrInvalidExpression)
		}
		if !strings.HasPrefix(path, "$.") {
			return nil, fmt.Errorf("%w: path %q must start with $.", ErrInvalidExpression, path)
		}

		fields = append(fields, expressionField{key: key, path: path})
	}

	if len(fields) == 0 {
		return nil, fmt.Errorf("%w: no fields defined", ErrInvalidExpression)
	}

	return fields, nil
}

// messageToMap 은 Message를 map[string]any로 변환한다.
// $ 경로 해석 시 메시지 전체 구조를 탐색할 수 있도록 한다.
//
// 반환 맵 구조:
//
//	{
//	  "id":        "uuid-string",
//	  "timestamp": "2006-01-02T15:04:05.999999999Z07:00",
//	  "payload":   { ... payload data ... },
//	  "metadata":  { "key": "value", ... }
//	}
func messageToMap(msg message.Message) map[string]any {
	// metadata를 map[string]any로 변환 (GetPath가 map[string]any를 기대함).
	// P2: Raw() 로 flat 키(string) + nested group(map[string]string)을 모두 노출한다.
	// group 은 map[string]any 로 변환하여 expression 이 metadata.agent.type 처럼
	// 중첩 경로로 접근할 수 있게 한다(GetPath 는 map[string]any 만 순회).
	raw := msg.Metadata().Raw()
	metaMap := make(map[string]any, len(raw))
	for k, v := range raw {
		switch val := v.(type) {
		case map[string]string:
			g := make(map[string]any, len(val))
			for gk, gv := range val {
				g[gk] = gv
			}
			metaMap[k] = g
		default:
			metaMap[k] = v
		}
	}

	// v0.16.2: timestamp 는 epoch ms (int64) — storage-write/storage-write 의
	// resolveTemplateExpr ($.timestamp) 과 일관성 유지.
	return map[string]any{
		"id":        msg.ID(),
		"type":      msg.Type(),
		"timestamp": msg.Timestamp().UnixMilli(),
		"payload":   msg.Payload().ToMap(),
		"metadata":  metaMap,
	}
}

// TransformMode 는 expression 변환 모드를 나타낸다.
type TransformMode string

const (
	// TransformModeSelect 는 지정된 필드만 추출하여 새 페이로드를 만든다 (기본값).
	TransformModeSelect TransformMode = "select"
	// TransformModeMerge 는 원본 페이로드를 유지하면서 지정 필드만 덮어쓴다.
	TransformModeMerge TransformMode = "merge"
	// TransformModeExclude 는 지정된 필드를 페이로드에서 제거한다.
	TransformModeExclude TransformMode = "exclude"
)

// expressionStep 은 파이프라인의 단일 변환 단계이다.
type expressionStep struct {
	mode  TransformMode
	value string
}

// parseExpressionSteps 는 YAML 배열 형식의 expression 설정을 파싱한다.
// 각 요소는 단일 키를 가진 맵이다: { "select": "{ ... }" } 또는 { "exclude": "field1, field2" }
func parseExpressionSteps(raw []any) ([]expressionStep, error) {
	steps := make([]expressionStep, 0, len(raw))
	for i, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: step %d must be a map", ErrInvalidExpression, i)
		}
		if len(m) != 1 {
			return nil, fmt.Errorf("%w: step %d must have exactly one mode key", ErrInvalidExpression, i)
		}
		for mode, val := range m {
			valStr, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("%w: step %d value must be a string", ErrInvalidExpression, i)
			}
			steps = append(steps, expressionStep{
				mode:  TransformMode(mode),
				value: valStr,
			})
		}
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("%w: no expression steps defined", ErrInvalidExpression)
	}
	return steps, nil
}

// parseFieldNames 는 쉼표로 구분된 필드명 문자열을 파싱한다.
func parseFieldNames(s string) []string {
	parts := strings.Split(s, ",")
	fields := make([]string, 0, len(parts))
	for _, p := range parts {
		f := strings.TrimSpace(p)
		if f != "" {
			fields = append(fields, f)
		}
	}
	return fields
}

// compileExclude 는 쉼표로 구분된 필드명 목록을 파싱하여
// 해당 필드를 페이로드에서 제거하는 TransformFunc를 반환한다.
func compileExclude(fieldNames string) (TransformFunc, error) {
	fields := parseFieldNames(fieldNames)
	if len(fields) == 0 {
		return nil, fmt.Errorf("%w: empty exclude fields", ErrInvalidExpression)
	}

	return func(msg message.Message) (message.Message, error) {
		result := msg.Payload().ToMap()
		for _, f := range fields {
			delete(result, f)
		}
		// v0.14.0: 원본 msg 의 Type / Timestamp 보존.
		opts := []message.Option{
			message.WithPayload(message.NewPayload(result)),
			message.WithType(msg.Type()),
			message.WithTimestamp(msg.Timestamp()),
		}
		for k, v := range msg.Metadata().All() {
			opts = append(opts, message.WithMetadata(k, v))
		}
		out := message.New(opts...)
		// P2: nested group 보존(All() 은 string 만 반환).
		message.CopyMetadataGroups(out.Metadata(), msg.Metadata())
		return out, nil
	}, nil
}

// compileExpressionPipeline 는 여러 변환 단계를 체이닝하는 TransformFunc를 생성한다.
// 각 단계의 출력이 다음 단계의 입력이 된다.
func compileExpressionPipeline(steps []expressionStep, vars map[string]any) (TransformFunc, error) {
	fns := make([]TransformFunc, 0, len(steps))
	for _, step := range steps {
		var fn TransformFunc
		var err error
		switch step.mode {
		case TransformModeExclude:
			fn, err = compileExclude(step.value)
		default:
			fn, err = compileExpressionV2(step.value, step.mode, vars)
		}
		if err != nil {
			return nil, err
		}
		fns = append(fns, fn)
	}

	if len(fns) == 1 {
		return fns[0], nil
	}

	return func(msg message.Message) (message.Message, error) {
		current := msg
		for _, fn := range fns {
			var err error
			current, err = fn(current)
			if err != nil {
				return nil, err
			}
		}
		return current, nil
	}, nil
}

// compileExpression 은 expression 문자열을 TransformFunc로 컴파일한다.
// $ 는 메시지 전체를 나타내며, 다음 경로를 지원한다:
//   - $.payload.field.subfield — 페이로드 데이터 접근
//   - $.metadata.key — 메타데이터 접근
//   - $.id — 메시지 ID 접근
//   - $.timestamp — 메시지 타임스탬프 접근
//
// mode:
//   - "select" (기본값) — expression에 정의된 필드만 추출하여 새 페이로드 생성
//   - "merge" — 원본 페이로드를 유지하면서 expression 결과를 덮어쓰기
func compileExpression(expr string, mode TransformMode) (TransformFunc, error) {
	fields, err := parseExpression(expr)
	if err != nil {
		return nil, err
	}

	if mode == "" {
		mode = TransformModeSelect
	}

	return func(msg message.Message) (message.Message, error) {
		msgMap := messageToMap(msg)
		srcPayload := message.NewPayload(msgMap)

		// expression 필드 해석
		extracted := make(map[string]any, len(fields))
		for _, f := range fields {
			val, pathErr := srcPayload.GetPath(f.path)
			if pathErr != nil {
				extracted[f.key] = nil
				continue
			}
			extracted[f.key] = val
		}

		// 결과 페이로드 구성
		var result map[string]any
		switch mode {
		case TransformModeMerge:
			// 원본 페이로드 복사 후 expression 결과로 덮어쓰기
			result = msg.Payload().ToMap()
			for k, v := range extracted {
				result[k] = v
			}
		default:
			// select: expression 필드만 추출
			result = extracted
		}

		// v0.14.0: 원본 msg 의 Type / Timestamp 보존.
		opts := []message.Option{
			message.WithPayload(message.NewPayload(result)),
			message.WithType(msg.Type()),
			message.WithTimestamp(msg.Timestamp()),
		}
		for k, v := range msg.Metadata().All() {
			opts = append(opts, message.WithMetadata(k, v))
		}
		out := message.New(opts...)
		// P2: nested group 보존(All() 은 string 만 반환).
		message.CopyMetadataGroups(out.Metadata(), msg.Metadata())
		return out, nil
	}, nil
}
