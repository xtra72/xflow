package node

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// influxdbAgent 는 InfluxDB 에이전트의 최소 인터페이스이다.
type influxdbAgent interface {
	Process(data []byte) ([]byte, error)
}

// influxdbMessageReceiver 는 쿼리 결과 비동기 수신 인터페이스이다.
type influxdbMessageReceiver interface {
	influxdbAgent
	ReceiveMessage(ctx context.Context) ([]byte, error)
}

// influxdbWriteData 는 InfluxDB 에 쓰기 위한 데이터 구조체이다.
type influxdbWriteData struct {
	Measurement string            `json:"measurement"`
	Tags        map[string]string `json:"tags,omitempty"`
	Fields      map[string]any    `json:"fields"`
	Timestamp   *int64            `json:"timestamp,omitempty"`
}

// InfluxDBWriteNode 는 메시지 데이터를 InfluxDB에 기록하는 노드이다.
// 메시지의 payload에서 measurement, tags, fields를 추출하여 InfluxDB 에이전트로 전송하고,
// 원본 메시지를 그대로 다음 노드로 전달한다 (pass-through).
type InfluxDBWriteNode struct {
	*BaseNode
	agent          influxdbAgent
	resolver       AgentResolver
	agentRef       *flow.AgentRef
	measurement    string            // 고정 measurement 이름 (빈 문자열이면 payload에서 추출)
	measurementKey string            // payload에서 measurement를 추출할 키
	tagMappings    map[string]string // 태그 매핑: tag_name -> payload_key
	fieldMappings  map[string]string // 필드 매핑: field_name -> payload_key (비어있으면 전체 payload)
	timestampKey   string            // payload에서 타임스탬프를 추출할 키
}

// NewInfluxDBWriteNode 는 새로운 InfluxDBWriteNode를 생성하는 팩토리 함수이다.
func NewInfluxDBWriteNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &InfluxDBWriteNode{
		BaseNode: base,
		agentRef: def.AgentRef,
	}
	// WithAgentResolver 옵션으로 주입된 resolver를 필드에 저장
	if r, ok := base.config["_agent_resolver"]; ok {
		if resolver, ok := r.(AgentResolver); ok {
			n.resolver = resolver
		}
	}
	return n, nil
}

// Init 은 InfluxDBWriteNode를 초기화하고 AgentResolver로 InfluxDB 에이전트를 해석한다.
func (n *InfluxDBWriteNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.resolveInfluxDB(ctx); err != nil {
		return fmt.Errorf("influxdb-write init: %w", err)
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// resolveInfluxDB 는 AgentResolver를 사용하여 InfluxDB 에이전트를 찾는다.
func (n *InfluxDBWriteNode) resolveInfluxDB(ctx context.Context) error {
	// config["_influxdb_agent"]로 직접 주입된 경우 (테스트용)
	if a, ok := n.config["_influxdb_agent"]; ok {
		if agent, ok := a.(influxdbAgent); ok {
			n.agent = agent
			return nil
		}
	}

	// AgentRef가 없으면 에러
	if n.agentRef == nil {
		return fmt.Errorf("agent_ref is required for influxdb-write node")
	}

	// resolver 확인
	if n.resolver == nil {
		return fmt.Errorf("agent resolver not configured")
	}

	// 에이전트 해석
	transport, err := n.resolver.ResolveAgent(ctx, *n.agentRef)
	if err != nil {
		return fmt.Errorf("failed to resolve influxdb agent %q: %w", n.agentRef.AgentName, err)
	}

	// AgentAccessor로 원본 에이전트 추출
	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return fmt.Errorf("influxdb agent transport does not support AgentAccessor")
	}

	// influxdbAgent 인터페이스로 타입 단언
	agent, ok := accessor.UnderlyingAgent().(influxdbAgent)
	if !ok {
		return fmt.Errorf("agent %q does not implement influxdbAgent", n.agentRef.AgentName)
	}

	n.agent = agent
	return nil
}

// Shutdown 은 InfluxDBWriteNode를 종료한다.
func (n *InfluxDBWriteNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 InfluxDBWriteNode의 설정을 적용한다.
//
// 지원하는 설정 키:
//   - "measurement": string - 고정 measurement 이름
//   - "measurement_key": string - payload에서 measurement를 추출할 키
//   - "tag_mappings": map[string]any - 태그 매핑 (tag_name -> payload_key)
//   - "field_mappings": map[string]any - 필드 매핑 (field_name -> payload_key)
//   - "timestamp_key": string - payload에서 타임스탬프를 추출할 키
func (n *InfluxDBWriteNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	if v, ok := config["measurement"]; ok {
		if s, ok := v.(string); ok {
			n.measurement = s
		}
	}

	if v, ok := config["measurement_key"]; ok {
		if s, ok := v.(string); ok {
			n.measurementKey = s
		}
	}

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

	if v, ok := config["timestamp_key"]; ok {
		if s, ok := v.(string); ok {
			n.timestampKey = s
		}
	}

	return nil
}

// Process 는 메시지 데이터를 InfluxDB에 기록하고, 원본 메시지를 그대로 반환한다.
func (n *InfluxDBWriteNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	if n.agent == nil {
		return nil, fmt.Errorf("influxdb-write: agent not configured")
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
		return nil, fmt.Errorf("influxdb-write: measurement is empty")
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

	// WriteData 구성
	wd := influxdbWriteData{
		Measurement: measurement,
		Tags:        tags,
		Fields:      fields,
	}

	// 타임스탬프 추출
	if n.timestampKey != "" {
		if v, ok := msg.Payload().Get(n.timestampKey); ok {
			switch ts := v.(type) {
			case int64:
				wd.Timestamp = &ts
			case float64:
				tsInt := int64(ts)
				wd.Timestamp = &tsInt
			}
		}
	}

	// JSON 직렬화 후 에이전트로 전송
	data, err := json.Marshal(wd)
	if err != nil {
		return nil, fmt.Errorf("influxdb-write: marshal error: %w", err)
	}

	if _, err := n.agent.Process(data); err != nil {
		return nil, fmt.Errorf("influxdb-write: %w", err)
	}

	// pass-through: 원본 메시지를 그대로 반환
	return []message.Message{msg}, nil
}
