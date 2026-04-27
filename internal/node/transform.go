package node

import (
	"context"
	"fmt"
	"sync"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// TransformFunc 는 메시지를 변환하는 함수 타입이다.
// 변환 실패 시 에러를 반환한다.
type TransformFunc func(msg message.Message) (message.Message, error)

// TransformNode 는 메시지를 변환하는 노드이다.
// 변환 함수가 nil이면 모든 메시지를 그대로 통과시킨다 (pass-through).
type TransformNode struct {
	*BaseNode
	transformFn TransformFunc
	mu          sync.RWMutex
}

// NewTransformNode 는 새로운 TransformNode를 생성하는 팩토리 함수이다.
func NewTransformNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &TransformNode{
		BaseNode: base,
	}
	return n, nil
}

// Init 은 TransformNode를 초기화한다.
func (n *TransformNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 변환 함수를 적용하여 메시지를 변환한다.
// 변환 함수가 nil이면 메시지를 그대로 통과시킨다.
// 변환 함수가 에러를 반환하면 nil과 에러를 반환한다 (호출자가 에러 포트 처리).
func (n *TransformNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	fn := n.transformFn
	n.mu.RUnlock()

	if fn == nil {
		return []message.Message{msg}, nil
	}

	result, err := fn(msg)
	if err != nil {
		return nil, err
	}
	return []message.Message{result}, nil
}

// Shutdown 은 TransformNode를 종료한다.
func (n *TransformNode) Shutdown(ctx context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 TransformNode의 설정을 적용한다.
// 우선순위: config["transform"](TransformFunc) > config["expression"]
//
// expression 형식:
//   - string: 단일 변환 (mode 키로 select/merge/exclude 지정, 기본값 select)
//   - []any:  파이프라인 (각 단계가 순서대로 실행)
//
// 파이프라인 예시:
//
//	expression:
//	  - select: "{ device_id: $.payload.device_id, temp: $.payload.object.temperature }"
//	  - exclude: "firmware, raw_adc"
//	  - merge: "{ source: $.metadata._source }"
func (n *TransformNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	// 1순위: TransformFunc 직접 설정 (프로그래밍 방식)
	if tf, ok := config["transform"]; ok {
		if fn, ok := tf.(TransformFunc); ok {
			n.mu.Lock()
			n.transformFn = fn
			n.mu.Unlock()
			return nil
		}
	}

	// 2순위: expression 설정 (YAML 설정 방식)
	expr, ok := config["expression"]
	if !ok {
		return nil
	}

	// strip_nulls 옵션: 결과 페이로드에서 nil 값 제거
	stripNulls, _ := config["strip_nulls"].(bool)

	// 변수 바인딩 수집: 예약 키를 제외한 모든 config 키
	var vars map[string]any
	reservedKeys := map[string]bool{
		"expression": true, "mode": true, "transform": true, "strip_nulls": true,
		"metadata_expression": true, "metadata_mode": true,
	}
	for k, v := range config {
		if !reservedKeys[k] {
			if vars == nil {
				vars = make(map[string]any)
			}
			vars[k] = v
		}
	}

	var fn TransformFunc
	var err error

	switch v := expr.(type) {
	case string:
		if v == "" {
			return nil
		}
		// 단일 expression
		mode := TransformModeSelect
		if m, ok := config["mode"]; ok {
			if mStr, ok := m.(string); ok {
				mode = TransformMode(mStr)
			}
		}
		if mode == TransformModeExclude {
			fn, err = compileExclude(v)
		} else {
			fn, err = compileExpressionV2(v, mode, vars)
		}
	case []any:
		// 파이프라인 (배열 형식)
		steps, parseErr := parseExpressionSteps(v)
		if parseErr != nil {
			return fmt.Errorf("transform configure: %w", parseErr)
		}
		fn, err = compileExpressionPipeline(steps, vars)
	default:
		return fmt.Errorf("transform configure: %w: expression must be string or array", ErrInvalidExpression)
	}

	if err != nil {
		return fmt.Errorf("transform configure: %w", err)
	}

	// strip_nulls: 결과에서 nil 값을 가진 키를 제거
	if stripNulls {
		orig := fn
		fn = func(msg message.Message) (message.Message, error) {
			result, err := orig(msg)
			if err != nil {
				return nil, err
			}
			cleaned := make(map[string]any)
			for k, v := range result.Payload().ToMap() {
				if v != nil {
					cleaned[k] = v
				}
			}
			opts := []message.Option{
				message.WithPayload(message.NewPayload(cleaned)),
			}
			for mk, mv := range result.Metadata().All() {
				opts = append(opts, message.WithMetadata(mk, mv))
			}
			return message.New(opts...), nil
		}
	}

	// metadata_expression: 페이로드와 동일한 구문으로 메타데이터 구성
	// 결과 맵을 dot notation으로 flatten하여 메타데이터에 설정한다.
	if metaExpr, ok := config["metadata_expression"]; ok {
		metaFn, metaExcludeFields, metaErr := compileMetadataTransform(metaExpr, config, vars)
		if metaErr != nil {
			return fmt.Errorf("transform configure: metadata: %w", metaErr)
		}

		if metaFn != nil || len(metaExcludeFields) > 0 {
			origFn := fn
			fn = func(msg message.Message) (message.Message, error) {
				// 1. payload 변환 실행
				var result message.Message
				if origFn != nil {
					r, innerErr := origFn(msg)
					if innerErr != nil {
						return nil, innerErr
					}
					result = r
				} else {
					result = msg
				}

				// 2. 메타데이터 구성
				opts := []message.Option{
					message.WithPayload(result.Payload()),
				}

				if len(metaExcludeFields) > 0 {
					// exclude: 지정된 키를 메타데이터에서 제거
					exclude := make(map[string]bool, len(metaExcludeFields))
					for _, k := range metaExcludeFields {
						exclude[k] = true
					}
					for k, v := range result.Metadata().All() {
						if !exclude[k] {
							opts = append(opts, message.WithMetadata(k, v))
						}
					}
				} else {
					// select/merge: 기존 메타데이터 보존 후 expression 결과 추가
					for k, v := range result.Metadata().All() {
						opts = append(opts, message.WithMetadata(k, v))
					}
					// metadata expression 평가 (원본 메시지 기반)
					metaResult, evalErr := metaFn(msg)
					if evalErr != nil {
						return nil, evalErr
					}
					flat := flattenToStringMap(metaResult.Payload().ToMap(), "")
					for k, v := range flat {
						opts = append(opts, message.WithMetadata(k, v))
					}
				}

				return message.New(opts...), nil
			}
		}
	}

	n.mu.Lock()
	n.transformFn = fn
	n.mu.Unlock()
	return nil
}

// compileMetadataTransform 은 metadata_expression 설정을 TransformFunc 또는 제외 필드 목록으로 컴파일한다.
// expression과 동일한 구문을 지원하며, 결과는 메타데이터에 설정된다.
func compileMetadataTransform(expr any, config map[string]any, vars map[string]any) (TransformFunc, []string, error) {
	switch v := expr.(type) {
	case string:
		if v == "" {
			return nil, nil, nil
		}
		mode := TransformModeMerge
		if m, ok := config["metadata_mode"]; ok {
			if s, ok := m.(string); ok {
				mode = TransformMode(s)
			}
		}
		if mode == TransformModeExclude {
			fields := parseFieldNames(v)
			if len(fields) == 0 {
				return nil, nil, fmt.Errorf("%w: empty metadata exclude fields", ErrInvalidExpression)
			}
			return nil, fields, nil
		}
		fn, err := compileExpressionV2(v, TransformModeSelect, vars)
		return fn, nil, err
	case []any:
		steps, err := parseExpressionSteps(v)
		if err != nil {
			return nil, nil, err
		}
		// 단일 exclude 단계면 필드명만 추출
		if len(steps) == 1 && steps[0].mode == TransformModeExclude {
			fields := parseFieldNames(steps[0].value)
			if len(fields) == 0 {
				return nil, nil, fmt.Errorf("%w: empty metadata exclude fields", ErrInvalidExpression)
			}
			return nil, fields, nil
		}
		fn, compileErr := compileExpressionPipeline(steps, vars)
		return fn, nil, compileErr
	default:
		return nil, nil, fmt.Errorf("%w: metadata_expression must be string or array", ErrInvalidExpression)
	}
}

// flattenToStringMap 은 중첩 맵을 dot notation 기반 플랫 문자열 맵으로 변환한다.
// { mqtt: { topic: "hvac/status/room1", qos: 1 } } → { "mqtt.topic": "hvac/status/room1", "mqtt.qos": "1" }
// nil 값은 건너뛴다.
func flattenToStringMap(m map[string]any, prefix string) map[string]string {
	result := make(map[string]string)
	for k, v := range m {
		fullKey := k
		if prefix != "" {
			fullKey = prefix + "." + k
		}
		if v == nil {
			continue
		}
		switch val := v.(type) {
		case map[string]any:
			for fk, fv := range flattenToStringMap(val, fullKey) {
				result[fk] = fv
			}
		case string:
			result[fullKey] = val
		case bool:
			if val {
				result[fullKey] = "true"
			} else {
				result[fullKey] = "false"
			}
		default:
			result[fullKey] = fmt.Sprintf("%v", v)
		}
	}
	return result
}
