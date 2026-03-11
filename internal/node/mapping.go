package node

import (
	"context"
	"fmt"
	"sync"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// MappingNode 는 메시지 필드 값을 키로 사용하여 매핑 테이블에서 대응하는 값을 조회하는 처리 노드이다.
// 센서 코드를 이름으로 변환하거나, 상태 코드를 메시지로 변환하는 등의 룩업 패턴을 지원한다.
type MappingNode struct {
	*BaseNode
	field        string         // 소스 필드 JSONPath (예: $.payload.status)
	mappings     map[string]any // 키-값 매핑 테이블
	defaultValue any            // 키 미발견 시 기본값
	hasDefault   bool           // 기본값 설정 여부
	target       string         // 출력 필드명 (빈 문자열이면 소스 필드 덮어쓰기)
	mu           sync.RWMutex   // 설정 보호 뮤텍스
}

// 인터페이스 컴파일 체크
var _ Node = (*MappingNode)(nil)

// NewMappingNode 는 새로운 MappingNode를 생성하는 팩토리 함수이다.
func NewMappingNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &MappingNode{
		BaseNode: base,
	}
	return n, nil
}

// Init 은 MappingNode를 초기화한다.
func (n *MappingNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Configure 는 MappingNode의 설정을 적용한다.
// field(필수), mappings(필수), default(선택), target(선택)을 설정한다.
func (n *MappingNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	// field (필수)
	field, ok := config["field"]
	if !ok {
		return fmt.Errorf("mapping configure: %w: field is required", ErrInvalidConfig)
	}
	fieldStr, ok := field.(string)
	if !ok || fieldStr == "" {
		return fmt.Errorf("mapping configure: %w: field must be a non-empty string", ErrInvalidConfig)
	}

	// mappings (필수)
	mappingsRaw, ok := config["mappings"]
	if !ok {
		return fmt.Errorf("mapping configure: %w: mappings is required", ErrInvalidConfig)
	}
	mappings, ok := mappingsRaw.(map[string]any)
	if !ok || len(mappings) == 0 {
		return fmt.Errorf("mapping configure: %w: mappings must be a non-empty map", ErrInvalidConfig)
	}

	// default (선택)
	var defaultValue any
	hasDefault := false
	if def, exists := config["default"]; exists {
		defaultValue = def
		hasDefault = true
	}

	// target (선택)
	var target string
	if t, exists := config["target"]; exists {
		if ts, ok := t.(string); ok {
			target = ts
		}
	}

	// 원자적 설정 적용
	n.mu.Lock()
	n.field = fieldStr
	n.mappings = mappings
	n.defaultValue = defaultValue
	n.hasDefault = hasDefault
	n.target = target
	n.mu.Unlock()

	return nil
}

// Process 는 메시지의 소스 필드 값을 매핑 테이블에서 조회하여 결과를 기록한다.
func (n *MappingNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	field := n.field
	mappings := n.mappings
	defaultValue := n.defaultValue
	hasDefault := n.hasDefault
	target := n.target
	n.mu.RUnlock()

	// 메시지를 맵으로 변환하고 소스 필드 값 추출
	msgMap := messageToMap(msg)
	p := message.NewPayload(msgMap)
	raw, err := p.GetPath(field)
	if err != nil || raw == nil {
		return nil, ErrMappingFieldNotFound
	}

	// 소스 값을 문자열 키로 변환
	key := fmt.Sprintf("%v", raw)

	// 매핑 테이블에서 조회
	result, found := mappings[key]
	if !found {
		if hasDefault {
			result = defaultValue
		} else {
			return nil, ErrMappingKeyNotFound
		}
	}

	// 결과 기록
	if target != "" {
		msg.Payload().Set(target, result)
	} else {
		// 소스 필드의 최종 키에 기록
		lastKey := lastPathKey(field)
		msg.Payload().Set(lastKey, result)
	}

	return []message.Message{msg}, nil
}

// Shutdown 은 MappingNode를 종료한다.
func (n *MappingNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// lastPathKey 는 JSONPath에서 마지막 키를 추출한다.
// 예: "$.payload.status_code" -> "status_code"
func lastPathKey(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[i+1:]
		}
	}
	return path
}
