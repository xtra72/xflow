package node

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ErrStoreNotConfigured 는 Store 인스턴스가 주입되지 않았을 때 반환된다.
var ErrStoreNotConfigured = errors.New("node: store instance not configured")

// StoreWriter 는 노드에서 Store에 기록하기 위한 인터페이스이다.
// 순환 의존을 방지하기 위해 node 패키지 내에 최소 인터페이스로 정의한다.
type StoreWriter interface {
	Set(ctx context.Context, key string, value any) error
	SetWithTTL(ctx context.Context, key string, value any, ttl time.Duration) error
}

// StoreReader 는 노드에서 Store를 읽기 위한 인터페이스이다.
type StoreReader interface {
	Get(ctx context.Context, key string) (any, bool, error)
	Has(ctx context.Context, key string) (bool, error)
	GetHistory(ctx context.Context, key string) ([]any, error)
}

// StoreWriteMeta 는 SetWithMeta 호출 시 부여할 메타데이터 옵션이다.
// system.StoreWriteMeta 와 동일 형태이며, 어댑터가 이 노드 인터페이스를 구현한다.
type StoreWriteMeta struct {
	// DataType 은 빈 문자열이 아니면 기록되는 키를 그 data_type 으로 등록/고정한다.
	DataType string
	// Tags 는 비어있지 않으면 기록되는 키에 태그를 부여한다.
	Tags map[string]string
	// TTL 은 0 보다 크면 값 쓰기에 TTL 을 적용한다.
	TTL time.Duration
}

// StoreMetaWriter 는 data_type/tags 메타데이터를 함께 지정하여 기록할 수 있는
// 선택적(optional) 인터페이스이다. NodeStoreAdapter 가 이를 구현한다.
//
// store 가 이 인터페이스를 만족하고 노드에 data_type 또는 tags 가 설정된 경우에만
// SetWithMeta 가 사용되며, 그 외에는 기존 StoreWriter.Set/SetWithTTL 로 폴백한다 (하위 호환).
type StoreMetaWriter interface {
	SetWithMeta(ctx context.Context, key string, value any, opts StoreWriteMeta) error
}

// storeProvider 는 네임스페이스별 Store 어댑터를 제공하는 에이전트의 인터페이스이다.
// UserStoreAgent 가 이 인터페이스를 구현하며, AgentResolver로 해석된 에이전트에서
// 타입 단언을 통해 StoreWriter/StoreReader 에 접근한다.
//
// 반환값은 StoreWriter + StoreReader를 모두 만족하는 NodeStoreAdapter이다.
// 순환 의존을 방지하기 위해 any를 반환하고, 호출 측에서 타입 단언한다.
type storeProvider interface {
	NodeStoreForNamespace(namespace string) any
}

// StoreWriteNode 는 메시지 데이터를 키-값 저장소에 기록하는 노드이다.
// 메시지의 payload에서 key_template을 해석하여 키를 생성하고,
// value_key로 지정된 값 또는 전체 payload를 저장한 뒤
// 원본 메시지를 그대로 다음 노드로 전달한다 (pass-through).
//
// Store 인스턴스는 agent_ref로 지정된 Store 에이전트에서 가져온다.
// Init 시 AgentResolver를 통해 에이전트를 찾고, storeProvider 인터페이스로
// Store 인스턴스에 접근한다.
type StoreWriteNode struct {
	*BaseNode
	store       StoreWriter
	resolver    AgentResolver     // AgentResolver (생성 시 옵션에서 추출)
	agentRef    *flow.AgentRef    // Store 에이전트 참조
	keyTemplate string            // 키 템플릿 (예: "{location}:{sensor}")
	valueKey    string            // payload에서 저장할 값의 키 (빈 문자열이면 전체 payload)
	namespace   string            // Store 네임스페이스
	ttl         time.Duration     // TTL (0이면 만료 없음)
	dataType    string            // 기록되는 키에 부여할 data_type (빈 문자열이면 미지정)
	tags        map[string]string // 기록되는 키에 부여할 태그 (nil/빈 맵이면 미지정)
}

// validDataTypes 는 store-write 노드 config 의 data_type 으로 허용되는 6종 enum 이다.
// system 계층의 DataType enum 과 동일하며, 노드 패키지의 순환 의존을 피하기 위해 여기서도 정의한다.
var validDataTypes = map[string]bool{
	"int":     true,
	"float":   true,
	"string":  true,
	"boolean": true,
	"bytes":   true,
	"json":    true,
}

// tagKeyPattern 은 태그 key 로 허용되는 문자 패턴이다 (system 계층과 동일).
var tagKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// NewStoreWriteNode 는 새로운 StoreWriteNode를 생성하는 팩토리 함수이다.
func NewStoreWriteNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &StoreWriteNode{
		BaseNode:  base,
		namespace: "default",
		agentRef:  def.AgentRef,
	}
	// WithAgentResolver 옵션으로 주입된 resolver를 필드에 저장
	if r, ok := base.config["_agent_resolver"]; ok {
		if resolver, ok := r.(AgentResolver); ok {
			n.resolver = resolver
		}
	}
	return n, nil
}

// Init 은 StoreWriteNode를 초기화하고 AgentResolver로 Store 에이전트를 해석한다.
func (n *StoreWriteNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	// AgentResolver를 통해 Store 에이전트 해석
	if err := n.resolveStore(ctx); err != nil {
		return fmt.Errorf("store-write init: %w", err)
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// resolveStore 는 AgentResolver를 사용하여 Store 에이전트를 찾고 StoreWriter를 추출한다.
func (n *StoreWriteNode) resolveStore(ctx context.Context) error {
	// config["_store"]로 직접 주입된 경우 (테스트용 하위 호환성)
	if s, ok := n.config["_store"]; ok {
		if writer, ok := s.(StoreWriter); ok {
			n.store = writer
			return nil
		}
	}

	// AgentRef가 없으면 즉시 실패 (fail-fast).
	// 이전에는 lazy-fail (Process 시점 ErrStoreNotConfigured)이었으나
	// 디버깅을 어렵게 만들고 잘못 설정된 플로우가 Running 상태로 진입하는 문제가 있어
	// 다른 스토리지 노드(influxdb/tsdb)와 동일하게 Init 단계에서 거부한다.
	if n.agentRef == nil {
		return fmt.Errorf("agent_ref is required for store-write node")
	}

	// resolver 확인
	if n.resolver == nil {
		return fmt.Errorf("agent resolver not configured")
	}

	// 에이전트 해석
	transport, err := n.resolver.ResolveAgent(ctx, *n.agentRef)
	if err != nil {
		return fmt.Errorf("failed to resolve store agent %q: %w", n.agentRef.AgentName, err)
	}

	// AgentAccessor로 원본 에이전트 추출
	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return fmt.Errorf("store agent transport does not support AgentAccessor")
	}

	// storeProvider 인터페이스로 네임스페이스별 Store 인스턴스 추출
	provider, ok := accessor.UnderlyingAgent().(storeProvider)
	if !ok {
		return fmt.Errorf("agent %q does not implement storeProvider", n.agentRef.AgentName)
	}

	instance := provider.NodeStoreForNamespace(n.namespace)
	if instance == nil {
		return fmt.Errorf("store agent %q returned nil store for namespace %q", n.agentRef.AgentName, n.namespace)
	}

	// NodeStoreAdapter → StoreWriter 타입 단언
	writer, ok := instance.(StoreWriter)
	if !ok {
		return fmt.Errorf("store agent %q returned incompatible type for StoreWriter", n.agentRef.AgentName)
	}
	n.store = writer

	return nil
}

// Shutdown 은 StoreWriteNode를 종료한다.
func (n *StoreWriteNode) Shutdown(ctx context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 StoreWriteNode의 설정을 적용한다.
//
// 지원하는 설정 키:
//   - "key_template": string - 키 템플릿 ({field} 형식 플레이스홀더)
//   - "value_key": string - payload에서 저장할 값의 키 (빈 문자열이면 전체 payload)
//   - "namespace": string - Store 네임스페이스 (기본값: "default")
//   - "ttl": string - TTL 기간 문자열 (예: "5m", "1h")
func (n *StoreWriteNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	if v, ok := config["key_template"]; ok {
		if s, ok := v.(string); ok {
			n.keyTemplate = s
		}
	}

	if v, ok := config["value_key"]; ok {
		if s, ok := v.(string); ok {
			// value_key 는 단일 message-field-path 선택자이므로 `$.` prefix 를 강제한다.
			// 빈 문자열은 전체 payload 저장으로 허용한다.
			// (key_template 의 {field} 보간은 별도 구문이므로 영향받지 않는다.)
			if s != "" && !strings.HasPrefix(s, "$.") {
				return fmt.Errorf(
					"store-write: value_key %q must be a $.-path (e.g. $.payload.state.mode, $.metadata.device.id)", s)
			}
			n.valueKey = s
		}
	}

	if v, ok := config["namespace"]; ok {
		if s, ok := v.(string); ok {
			n.namespace = s
		}
	}

	if v, ok := config["ttl"]; ok {
		if s, ok := v.(string); ok && s != "" {
			d, err := time.ParseDuration(s)
			if err != nil {
				return fmt.Errorf("store-write: invalid ttl %q: %w", s, err)
			}
			n.ttl = d
		}
	}

	// data_type (선택): 지정 시 기록되는 키를 그 타입으로 등록한다.
	// 6종 enum(int/float/string/boolean/bytes/json) 외 값은 거부한다.
	if v, ok := config["data_type"]; ok {
		if s, ok := v.(string); ok && s != "" {
			if !validDataTypes[s] {
				return fmt.Errorf(
					"store-write: invalid data_type %q (must be one of: int, float, string, boolean, bytes, json)", s)
			}
			n.dataType = s
		}
	}

	// tags (선택): 기록되는 키에 부여할 key-value 태그.
	// map[string]any 로 들어온 값을 map[string]string 으로 정규화하며,
	// key 는 ^[a-zA-Z0-9_-]+$ 를 만족해야 하고 value 는 문자열이어야 한다.
	if v, ok := config["tags"]; ok {
		if raw, ok := v.(map[string]any); ok && len(raw) > 0 {
			parsed := make(map[string]string, len(raw))
			for tk, tv := range raw {
				if !tagKeyPattern.MatchString(tk) {
					return fmt.Errorf(
						"store-write: invalid tag key %q (must match ^[a-zA-Z0-9_-]+$)", tk)
				}
				sv, ok := tv.(string)
				if !ok {
					return fmt.Errorf("store-write: tag %q value must be a string", tk)
				}
				parsed[tk] = sv
			}
			n.tags = parsed
		}
	}

	return nil
}

// Process 는 메시지 데이터를 Store에 기록하고, 원본 메시지를 그대로 반환한다.
func (n *StoreWriteNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	if n.store == nil {
		return nil, ErrStoreNotConfigured
	}

	// 키 해석
	key, err := resolveKeyTemplate(n.keyTemplate, msg)
	if err != nil {
		return nil, fmt.Errorf("store-write: %w", err)
	}

	// 값 추출 — value_key 는 Configure 에서 `$.` prefix 가 강제된 단일 경로 선택자이다.
	//   value_key="$.payload.state.current_temp" → payload 의 중첩 경로
	//   value_key="$.metadata.dev_id"            → metadata 값
	var value any
	if n.valueKey != "" {
		v, err := resolveTemplateExpr(n.valueKey, msg)
		if err != nil {
			return nil, fmt.Errorf("store-write: value_key %q: %w", n.valueKey, err)
		}
		value = v
	} else {
		// value_key가 없으면 전체 payload를 저장
		value = msg.Payload().ToMap()
	}

	// Store에 기록.
	// data_type 또는 tags 가 설정됐고 store 가 StoreMetaWriter 를 만족하면 SetWithMeta 를 사용하여
	// 메타데이터를 함께 적용한다. 그 외에는 기존 Set/SetWithTTL 경로로 폴백한다 (하위 호환).
	useMeta := n.dataType != "" || len(n.tags) > 0
	if mw, ok := n.store.(StoreMetaWriter); ok && useMeta {
		opts := StoreWriteMeta{
			DataType: n.dataType,
			Tags:     n.tags,
			TTL:      n.ttl,
		}
		if err := mw.SetWithMeta(ctx, key, value, opts); err != nil {
			return nil, fmt.Errorf("store-write: %w", err)
		}
	} else if n.ttl > 0 {
		if err := n.store.SetWithTTL(ctx, key, value, n.ttl); err != nil {
			return nil, fmt.Errorf("store-write: %w", err)
		}
	} else {
		if err := n.store.Set(ctx, key, value); err != nil {
			return nil, fmt.Errorf("store-write: %w", err)
		}
	}

	// pass-through: 원본 메시지를 그대로 반환
	return []message.Message{msg}, nil
}

// resolveKeyTemplate 는 {expr} 플레이스홀더를 메시지 값으로 치환한다.
//
// 지원 문법:
//   - {field}                — payload 의 field (legacy, backward compatible)
//   - {$.payload.field}      — payload 의 field (명시적)
//   - {$.payload.a.b.c}      — payload 의 중첩 경로 (map[string]any traversal)
//   - {$.metadata.field}     — metadata 의 field
//
// 예: "{$.metadata.dev_id}:{$.payload.state.mode}" +
//
//	metadata{dev_id:"idu-1"} + payload{state:{mode:1}}
//	→ "idu-1:1"
func resolveKeyTemplate(template string, msg message.Message) (string, error) {
	result := template
	for {
		start := strings.Index(result, "{")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}")
		if end == -1 {
			break
		}
		end += start

		expr := result[start+1 : end]
		v, err := resolveTemplateExpr(expr, msg)
		if err != nil {
			return "", err
		}
		result = result[:start] + fmt.Sprintf("%v", v) + result[end+1:]
	}
	return result, nil
}

// resolveTemplateExpr 는 단일 {expr} 식을 해석한다 (v0.7.9).
// expr 가 "$." prefix 면 JSONPath-like 경로, 그 외는 payload 직접 필드.
//
// v0.13.0 확장: 메시지 top-level 필드 ($.id, $.type, $.timestamp) 지원.
//   - $.id        → msg.ID() (string)
//   - $.type      → msg.Type() (string)
//   - $.timestamp → msg.Timestamp().UnixMilli() (int64 epoch ms)
//   - $.payload.X / $.payload.x.y → payload JSONPath
//   - $.metadata.X → metadata 단일 키
func resolveTemplateExpr(expr string, msg message.Message) (any, error) {
	if !strings.HasPrefix(expr, "$.") {
		// Legacy: payload 직접 필드.
		// SPEC-NODE-001 v1.5.0: dot notation 지원 — {item.id} 또는 {item.nested.field}
		// 형태로 nested 객체 traverse. 단일 segment 는 기존 flat lookup 동작 유지.
		parts := strings.Split(expr, ".")
		if len(parts) == 1 {
			v, ok := msg.Payload().Get(expr)
			if !ok {
				return nil, fmt.Errorf("key template field %q not found in payload", expr)
			}
			return v, nil
		}
		return lookupPayloadPath(msg.Payload(), parts)
	}

	parts := strings.Split(expr[2:], ".")
	// Top-level 단일 segment 처리 ($.id, $.type, $.timestamp) (v0.13.0)
	if len(parts) == 1 {
		switch parts[0] {
		case "id":
			return msg.ID(), nil
		case "type":
			return msg.Type(), nil
		case "timestamp":
			return msg.Timestamp().UnixMilli(), nil
		case "payload", "metadata":
			// payload/metadata 는 sub-path 가 필수.
			return nil, fmt.Errorf("invalid key template path %q (expected $.payload.field or $.metadata.field)", expr)
		default:
			return nil, fmt.Errorf("unknown key template root %q (expected $.payload, $.metadata, $.id, $.type, $.timestamp)", parts[0])
		}
	}
	switch parts[0] {
	case "payload":
		return lookupPayloadPath(msg.Payload(), parts[1:])
	case "metadata":
		switch len(parts) {
		case 2:
			// $.metadata.{key} — flat 메타데이터 단일 키.
			v, ok := msg.Metadata().Get(parts[1])
			if !ok {
				return nil, fmt.Errorf("metadata key %q not found", parts[1])
			}
			return v, nil
		case 3:
			// $.metadata.{group}.{field} — 그룹(device/agent 등) 의 필드.
			// 예: $.metadata.device.id / $.metadata.device.type / $.metadata.agent.type.
			group, ok := msg.Metadata().GetGroup(parts[1])
			if !ok {
				return nil, fmt.Errorf("metadata group %q not found", parts[1])
			}
			v, ok := group[parts[2]]
			if !ok {
				return nil, fmt.Errorf("metadata group field %q.%q not found", parts[1], parts[2])
			}
			return v, nil
		default:
			// 그룹은 한 단계 깊이만 지원한다.
			return nil, fmt.Errorf("metadata path %q: too deep (groups are one level)", expr)
		}
	default:
		return nil, fmt.Errorf("unknown key template root %q (expected $.payload, $.metadata, $.id, $.type, $.timestamp)", parts[0])
	}
}

// lookupPayloadPath 는 payload 의 점-구분 경로를 따라간다 (v0.7.9).
// 첫 segment 는 payload.Get 으로 가져오고, 이후 segment 는 map[string]any 로 traverse.
func lookupPayloadPath(payload message.Payload, path []string) (any, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("empty payload path")
	}
	v, ok := payload.Get(path[0])
	if !ok {
		return nil, fmt.Errorf("payload key %q not found", path[0])
	}
	for i := 1; i < len(path); i++ {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("payload path %q: %q is not a nested object", strings.Join(path, "."), path[i-1])
		}
		v, ok = m[path[i]]
		if !ok {
			return nil, fmt.Errorf("payload path key %q not found", path[i])
		}
	}
	return v, nil
}
