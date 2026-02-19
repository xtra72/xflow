package node

import (
	"fmt"
	"strings"
	"time"

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
	// metadata를 map[string]any로 변환 (GetPath가 map[string]any를 기대함)
	metaAll := msg.Metadata().All()
	metaMap := make(map[string]any, len(metaAll))
	for k, v := range metaAll {
		metaMap[k] = v
	}

	return map[string]any{
		"id":        msg.ID(),
		"timestamp": msg.Timestamp().Format(time.RFC3339Nano),
		"payload":   msg.Payload().ToMap(),
		"metadata":  metaMap,
	}
}

// compileExpression 은 expression 문자열을 TransformFunc로 컴파일한다.
// $ 는 메시지 전체를 나타내며, 다음 경로를 지원한다:
//   - $.payload.field.subfield — 페이로드 데이터 접근
//   - $.metadata.key — 메타데이터 접근
//   - $.id — 메시지 ID 접근
//   - $.timestamp — 메시지 타임스탬프 접근
func compileExpression(expr string) (TransformFunc, error) {
	fields, err := parseExpression(expr)
	if err != nil {
		return nil, err
	}

	return func(msg message.Message) (message.Message, error) {
		msgMap := messageToMap(msg)
		srcPayload := message.NewPayload(msgMap)
		result := make(map[string]any, len(fields))

		for _, f := range fields {
			val, pathErr := srcPayload.GetPath(f.path)
			if pathErr != nil {
				// 경로를 찾을 수 없으면 nil로 설정
				result[f.key] = nil
				continue
			}
			result[f.key] = val
		}

		// 원본 메시지의 메타데이터를 유지하면서 새 페이로드를 설정한다
		opts := []message.Option{
			message.WithPayload(message.NewPayload(result)),
		}
		for k, v := range msg.Metadata().All() {
			opts = append(opts, message.WithMetadata(k, v))
		}
		return message.New(opts...), nil
	}, nil
}
