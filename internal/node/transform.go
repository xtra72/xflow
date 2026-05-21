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
	// v0.15.0: metadata_expression / metadata_mode 제거 — 항상 payload 로 결과 쓰도록 통일.
	var vars map[string]any
	reservedKeys := map[string]bool{
		"expression": true, "mode": true, "transform": true, "strip_nulls": true,
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
			// v0.14.0: Type 과 Timestamp 보존 (이전엔 message.New 가 새로 생성).
			opts := []message.Option{
				message.WithPayload(message.NewPayload(cleaned)),
				message.WithType(result.Type()),
				message.WithTimestamp(result.Timestamp()),
			}
			for mk, mv := range result.Metadata().All() {
				opts = append(opts, message.WithMetadata(mk, mv))
			}
			return message.New(opts...), nil
		}
	}

	// v0.15.0: metadata_expression / metadata_mode 기능 제거. transform 결과는 항상 payload 로.
	// 기존 metadata 는 변환 함수가 보존 (Clone) — metadata 가공이 필요하면 별도 노드 사용 권장.

	n.mu.Lock()
	n.transformFn = fn
	n.mu.Unlock()
	return nil
}
