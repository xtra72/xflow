package node

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	measurement    string            // 고정 measurement 이름 (빈 문자열이면 measurement_key 사용)
	measurementKey string            // measurement 추출 키 (JSONPath: $.payload.X / $.metadata.X / $.type)
	tagMappings    map[string]string // 태그 매핑: InfluxDB 태그 이름 → metadata 키 (v0.16.3). 비어있으면 모든 metadata 를 tags 로
	fieldMappings  map[string]string // 필드 매핑 (field_name -> JSONPath). 비어있으면 전체 payload 를 fields 로
	timestampKey   string            // 타임스탬프 추출 키 (JSONPath). 비어있으면 msg.Timestamp() 사용 (v0.14.0)
	boolToInt      bool              // true이면 boolean 값을 0/1 정수로 변환
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

	// v0.16.3: tag_mappings 는 map[InfluxDB tag name → metadata key]. 비어있으면 모든
	// metadata 를 tags 로. 호환: list 형식 (key/value 동일) + map 형식 모두 허용.
	for _, key := range []string{"tag_mappings", "tag_keys", "tags"} {
		v, ok := config[key]
		if !ok {
			continue
		}
		switch val := v.(type) {
		case map[string]any:
			if n.tagMappings == nil {
				n.tagMappings = make(map[string]string, len(val))
			}
			for tagName, mk := range val {
				if s, ok := mk.(string); ok {
					n.tagMappings[tagName] = s
				}
			}
		case map[string]string:
			if n.tagMappings == nil {
				n.tagMappings = make(map[string]string, len(val))
			}
			for tagName, mk := range val {
				n.tagMappings[tagName] = mk
			}
		case []string:
			// list 형식: tag name 과 metadata key 가 동일 (v0.14.2 호환).
			if n.tagMappings == nil {
				n.tagMappings = make(map[string]string, len(val))
			}
			for _, k := range val {
				n.tagMappings[k] = k
			}
		case []any:
			if n.tagMappings == nil {
				n.tagMappings = make(map[string]string, len(val))
			}
			for _, item := range val {
				if s, ok := item.(string); ok {
					n.tagMappings[s] = s
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

	if v, ok := config["bool_to_int"]; ok {
		if b, ok := v.(bool); ok {
			n.boolToInt = b
		}
	}

	return nil
}

// Process 는 메시지 데이터를 InfluxDB에 기록하고, 원본 메시지를 그대로 반환한다.
//
// v0.14.0 매핑 규칙:
//   - 기본 tags: msg.Metadata().All() (모든 metadata)
//   - 기본 fields: msg.Payload().ToMap() (전체 payload)
//   - 기본 timestamp: msg.Timestamp() (메시지 timestamp)
//   - tag_mappings / field_mappings 지정 시 명시적 매핑 사용 — 값은 JSONPath
//     ($.payload.X, $.metadata.X, $.type, $.timestamp) 또는 legacy payload key
//   - measurement_key / timestamp_key 도 동일한 JSONPath 문법 지원
func (n *InfluxDBWriteNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	if n.agent == nil {
		return nil, fmt.Errorf("influxdb-write: agent not configured")
	}

	// measurement 결정 — 명시적 measurement > measurement_key > 에러.
	// v0.14.0: measurement_key 가 $. prefix 면 JSONPath 로 해석.
	measurement := n.measurement
	if measurement == "" && n.measurementKey != "" {
		if v, err := resolveTemplateExpr(n.measurementKey, msg); err == nil {
			// 추출값이 오브젝트면 JSON 문자열로 변환한다.
			measurement = influxToString(v)
		}
	}
	if measurement == "" {
		return nil, fmt.Errorf("influxdb-write: measurement is empty")
	}

	// tags 추출 — v0.16.3: tag_mappings 는 map[InfluxDB tag name → metadata key].
	// 비어있으면 모든 metadata 를 tags 로 (이름 동일).
	tags := make(map[string]string)
	if len(n.tagMappings) > 0 {
		for tagName, mapping := range n.tagMappings {
			// 값이 $. prefix 면 JSONPath 로 메시지 내 임의 키를 참조한다
			// ($.payload.X, $.metadata.X, $.type, $.timestamp). 오브젝트는 문자열로
			// 변환한다. 그 외(legacy)는 metadata 키 직접 참조로 하위 호환을 유지한다.
			if strings.HasPrefix(mapping, "$.") {
				if v, err := resolveTemplateExpr(mapping, msg); err == nil {
					tags[tagName] = influxToString(v)
				}
				continue
			}
			if v, ok := msg.Metadata().Get(mapping); ok {
				tags[tagName] = v
			}
		}
	} else {
		// 기본: 모든 metadata 를 tags 로.
		// P2: Flux 태그는 평면 string key=value 이므로 nested group 은 단일 태그가 될 수
		// 없다. group 값은 "{group}.{field}" 평면 태그로 펼친다(예: agent.id, device.id).
		// flat string 키는 그대로 사용한다.
		//
		// message-slim-metadata: 스토리지 write 는 외부 경계이므로 agent / device 그룹을
		// id-only 로 슬림화한다(type/name 은 레지스트리의 정규 데이터로 중복 저장하지
		// 않음). 내부 메시지 흐름은 영향받지 않는다 — 여기서는 Raw() 복사본을 슬림화한
		// DTO 위에서만 동작한다. 결과적으로 agent.id / device.id 태그만 기록된다.
		for k, v := range message.SlimGroupsToID(msg.Metadata().Raw()) {
			switch val := v.(type) {
			case string:
				tags[k] = val
			case map[string]string:
				for fk, fv := range val {
					tags[k+"."+fk] = fv
				}
			}
		}
	}

	// fields 추출 — v0.14.0: 기본 = 전체 payload, 매핑 지정 시 JSONPath 해석.
	var fields map[string]any
	if len(n.fieldMappings) > 0 {
		fields = make(map[string]any, len(n.fieldMappings))
		for fieldName, expr := range n.fieldMappings {
			if v, err := resolveTemplateExpr(expr, msg); err == nil {
				// 오브젝트 값은 InfluxDB 필드로 쓸 수 없으므로 JSON 문자열로 변환한다.
				fields[fieldName] = influxFieldValue(v)
			}
		}
	} else {
		// field_mappings가 없으면 전체 payload를 fields로 사용.
		// 중첩 오브젝트 값은 문자열로 변환한다.
		fields = msg.Payload().ToMap()
		for k, v := range fields {
			fields[k] = influxFieldValue(v)
		}
	}

	// bool → int 변환
	if n.boolToInt {
		for k, v := range fields {
			if b, ok := v.(bool); ok {
				if b {
					fields[k] = 1
				} else {
					fields[k] = 0
				}
			}
		}
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

	// 타임스탬프 — v0.14.0: 기본 = msg.Timestamp, timestamp_key 지정 시 JSONPath 해석.
	if n.timestampKey != "" {
		if v, err := resolveTemplateExpr(n.timestampKey, msg); err == nil {
			switch ts := v.(type) {
			case int64:
				wd.Timestamp = &ts
			case float64:
				tsInt := int64(ts)
				wd.Timestamp = &tsInt
			case int:
				tsInt := int64(ts)
				wd.Timestamp = &tsInt
			}
		}
	} else {
		// 기본: 메시지 timestamp (epoch ms).
		ts := msg.Timestamp().UnixMilli()
		wd.Timestamp = &ts
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

// influxIsObject 는 값이 InfluxDB 스칼라 필드/태그로 쓸 수 없는 오브젝트
// (map / slice 등 구조형) 인지 보고한다. 스칼라(string/number/bool/nil)는 false.
func influxIsObject(v any) bool {
	switch v.(type) {
	case nil, bool, string,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return false
	default:
		// map[string]any, []any, 구조체 등 → 오브젝트.
		return true
	}
}

// influxToString 은 값을 문자열로 변환한다(태그/ measurement 용 — 항상 문자열).
// 오브젝트는 JSON 으로, 스칼라는 fmt 로 변환한다. nil 은 빈 문자열.
func influxToString(v any) string {
	if v == nil {
		return ""
	}
	if influxIsObject(v) {
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
	}
	return fmt.Sprintf("%v", v)
}

// influxFieldValue 는 필드 값으로 사용할 값을 반환한다. 스칼라는 타입을 보존하고
// (InfluxDB 가 int/float/bool/string 필드 타입을 구분하므로), 오브젝트는 InfluxDB
// 필드로 쓸 수 없으므로 JSON 문자열로 변환한다.
func influxFieldValue(v any) any {
	if influxIsObject(v) {
		return influxToString(v)
	}
	return v
}
