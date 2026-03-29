package node

import (
	"context"
	"errors"
	"fmt"

	"github.com/xtra/xflow/internal/tsdb"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ErrTSDBNotConfigured 는 TSDB 인스턴스가 주입되지 않았을 때 반환된다.
var ErrTSDBNotConfigured = errors.New("node: tsdb instance not configured")

// tsdbProvider 는 TSDB 인스턴스를 제공하는 에이전트의 인터페이스이다.
// TSDBAgent가 이 인터페이스를 구현하며, AgentResolver로 해석된 에이전트에서
// 타입 단언을 통해 TSDB 인스턴스에 접근한다.
type tsdbProvider interface {
	TSDB() tsdb.TSDB
}

// TSDBWriteNode 는 메시지 데이터를 TSDB에 기록하는 노드이다.
// 메시지의 payload에서 measurement, tags, fields를 추출하여 TSDB에 기록하고,
// 원본 메시지를 그대로 다음 노드로 전달한다 (pass-through).
//
// TSDB 인스턴스는 agent_ref로 지정된 TSDB 에이전트에서 가져온다.
// Init 시 AgentResolver를 통해 에이전트를 찾고, tsdbProvider 인터페이스로
// TSDB 인스턴스에 접근한다.
type TSDBWriteNode struct {
	*BaseNode
	db             tsdb.TSDB
	resolver       AgentResolver         // AgentResolver (생성 시 옵션에서 추출)
	agentRef       *flow.AgentRef        // TSDB 에이전트 참조
	measurement    string                // 고정 measurement 이름 (빈 문자열이면 payload에서 추출)
	measurementKey string                // payload에서 measurement를 추출할 키
	tagMappings    map[string]string     // 태그 매핑: tag_name -> payload_key
	fieldMappings  map[string]string     // 필드 매핑: field_name -> payload_key (비어있으면 전체 payload)
}

// NewTSDBWriteNode 는 새로운 TSDBWriteNode를 생성하는 팩토리 함수이다.
func NewTSDBWriteNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &TSDBWriteNode{
		BaseNode: base,
		agentRef: def.AgentRef,
	}
	// WithAgentResolver 옵션으로 주입된 resolver를 필드에 저장
	// (Configure가 config를 덮어쓰기 전에 추출)
	if r, ok := base.config["_agent_resolver"]; ok {
		if resolver, ok := r.(AgentResolver); ok {
			n.resolver = resolver
		}
	}
	return n, nil
}

// Init 은 TSDBWriteNode를 초기화하고 AgentResolver로 TSDB 에이전트를 해석한다.
func (n *TSDBWriteNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	// AgentResolver를 통해 TSDB 에이전트 해석
	if err := n.resolveTSDB(ctx); err != nil {
		return fmt.Errorf("tsdb-write init: %w", err)
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// resolveTSDB 는 AgentResolver를 사용하여 TSDB 에이전트를 찾고 TSDB 인스턴스를 추출한다.
func (n *TSDBWriteNode) resolveTSDB(ctx context.Context) error {
	// config["_tsdb"]로 직접 주입된 경우 (테스트용)
	if db, ok := n.config["_tsdb"]; ok {
		if tsdbInst, ok := db.(tsdb.TSDB); ok {
			n.db = tsdbInst
			return nil
		}
	}

	// AgentRef가 없으면 에러
	if n.agentRef == nil {
		return fmt.Errorf("agent_ref is required for tsdb-write node")
	}

	// resolver 확인
	if n.resolver == nil {
		return fmt.Errorf("agent resolver not configured")
	}

	// 에이전트 해석
	transport, err := n.resolver.ResolveAgent(ctx, *n.agentRef)
	if err != nil {
		return fmt.Errorf("failed to resolve tsdb agent %q: %w", n.agentRef.AgentName, err)
	}

	// AgentAccessor로 원본 에이전트 추출
	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return fmt.Errorf("tsdb agent transport does not support AgentAccessor")
	}

	// tsdbProvider 인터페이스로 TSDB 인스턴스 추출
	provider, ok := accessor.UnderlyingAgent().(tsdbProvider)
	if !ok {
		return fmt.Errorf("agent %q does not implement tsdbProvider", n.agentRef.AgentName)
	}

	n.db = provider.TSDB()
	if n.db == nil {
		return fmt.Errorf("tsdb agent %q returned nil TSDB instance", n.agentRef.AgentName)
	}

	return nil
}

// Shutdown 은 TSDBWriteNode를 종료한다.
func (n *TSDBWriteNode) Shutdown(ctx context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 TSDBWriteNode의 설정을 적용한다.
//
// 지원하는 설정 키:
//   - "measurement": string - 고정 measurement 이름
//   - "measurement_key": string - payload에서 measurement를 추출할 키
//   - "tag_mappings": map[string]any - 태그 매핑 (tag_name -> payload_key)
//   - "field_mappings": map[string]any - 필드 매핑 (field_name -> payload_key)
func (n *TSDBWriteNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	// measurement 설정
	if v, ok := config["measurement"]; ok {
		if s, ok := v.(string); ok {
			n.measurement = s
		}
	}

	// measurement_key 설정
	if v, ok := config["measurement_key"]; ok {
		if s, ok := v.(string); ok {
			n.measurementKey = s
		}
	}

	// tag_mappings 설정
	if v, ok := config["tag_mappings"]; ok {
		if m, ok := v.(map[string]any); ok {
			n.tagMappings = make(map[string]string, len(m))
			for k, val := range m {
				if s, ok := val.(string); ok {
					n.tagMappings[k] = s
				}
			}
		}
	}

	// field_mappings 설정
	if v, ok := config["field_mappings"]; ok {
		if m, ok := v.(map[string]any); ok {
			n.fieldMappings = make(map[string]string, len(m))
			for k, val := range m {
				if s, ok := val.(string); ok {
					n.fieldMappings[k] = s
				}
			}
		}
	}

	return nil
}

// Process 는 메시지 데이터를 TSDB에 기록하고, 원본 메시지를 그대로 반환한다.
func (n *TSDBWriteNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	if n.db == nil {
		return nil, ErrTSDBNotConfigured
	}

	// measurement 결정
	measurement := n.measurement
	if measurement == "" && n.measurementKey != "" {
		if v, ok := msg.Payload().Get(n.measurementKey); ok {
			if s, ok := v.(string); ok {
				measurement = s
			}
		}
	}
	if measurement == "" {
		return nil, fmt.Errorf("tsdb-write: measurement is empty")
	}

	// tags 추출
	tags := make(map[string]string)
	if n.tagMappings != nil {
		for tagName, payloadKey := range n.tagMappings {
			if v, ok := msg.Payload().Get(payloadKey); ok {
				tags[tagName] = fmt.Sprintf("%v", v)
			}
		}
	}

	// fields 추출
	var fields map[string]any
	if len(n.fieldMappings) > 0 {
		fields = make(map[string]any, len(n.fieldMappings))
		for fieldName, payloadKey := range n.fieldMappings {
			if v, ok := msg.Payload().Get(payloadKey); ok {
				fields[fieldName] = v
			}
		}
	} else {
		// field_mappings가 없으면 전체 payload를 fields로 사용
		fields = msg.Payload().ToMap()
	}

	// 기록할 필드가 없으면 건너뛴다
	if len(fields) == 0 {
		return []message.Message{msg}, nil
	}

	// TSDB에 기록
	if err := n.db.Write(measurement, tags, fields); err != nil {
		return nil, fmt.Errorf("tsdb-write: %w", err)
	}

	// pass-through: 원본 메시지를 그대로 반환
	return []message.Message{msg}, nil
}
