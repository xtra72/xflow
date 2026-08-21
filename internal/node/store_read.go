package node

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// toAnySlice 는 임의 슬라이스 타입 ([]string, []int, []any 등)을 []any 로 변환한다.
// 슬라이스가 아니면 nil 반환.
func toAnySlice(v any) []any {
	if v == nil {
		return nil
	}
	if arr, ok := v.([]any); ok {
		return arr
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil
	}
	result := make([]any, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		result[i] = rv.Index(i).Interface()
	}
	return result
}

// StoreReadNode 는 키-값 저장소에서 데이터를 조회하는 노드이다.
//
// 메시지의 payload에서 key_template을 해석하여 키를 생성하고,
// read_mode 에 따라 현재값 또는 시계열 엔트리 배열을 메시지 payload의
// output_key 에 기록한 뒤 다음 노드로 전달한다.
//
// read_mode 에 상관없이 output_key 에 들어가는 값은 항상 []map[string]any
// 형태로 통일되어 있으며, 각 항목은 {"value": <any>, "timestamp": <int64>} 를
// 포함한다. timestamp 는 프로젝트 전체 컨벤션에 따라 epoch 밀리초(UnixMilli)
// 이다. 결과는 최신순(내림차순)이며 현재값을 포함한다.
//
// Store 인스턴스는 agent_ref로 지정된 Store 에이전트에서 가져온다.
// Init 시 AgentResolver를 통해 에이전트를 찾고, storeProvider 인터페이스로
// Store 인스턴스에 접근한다.

// HistoryQueryReader 는 Store 계층의 QueryHistory 기능을 노드 계층에 노출하는 인터페이스이다.
// system.NodeStoreAdapter 가 이 인터페이스를 구현한다.
type HistoryQueryReader interface {
	QueryHistory(ctx context.Context, key string, query system.HistoryQuery) ([]map[string]any, error)
}

// MetadataReader 는 Store 엔트리의 메타데이터를 조회하는 인터페이스이다.
// GetMetadata 는 count, created_at, updated_at, oldest_at 등의 메타데이터를
// map[string]any 형태로 반환한다.
type MetadataReader interface {
	GetMetadata(ctx context.Context, key string) (map[string]any, error)
}

// ReadMode 는 store-read 노드의 조회 모드이다.
type ReadMode string

const (
	ReadModeLatest    ReadMode = "latest"
	ReadModeLastN     ReadMode = "last_n"
	ReadModeDuration  ReadMode = "duration"
	ReadModeTimeRange ReadMode = "time_range"
	ReadModeSinceN    ReadMode = "since_n"
)

// StoreReadNode 는 Store 에서 값을 조회하여 메시지 payload 에 추가하는 노드이다.
type StoreReadNode struct {
	*BaseNode
	store           StoreReader
	resolver        AgentResolver  // AgentResolver (생성 시 옵션에서 추출)
	agentRef        *flow.AgentRef // Store 에이전트 참조
	keyTemplate     string         // 키 템플릿 (예: "{device}:{field}")
	namespace       string         // Store 네임스페이스
	outputKey       string         // 조회 결과를 저장할 payload 키 (기본값: "store_value")
	includeMetadata bool           // 메타데이터를 함께 조회할지 여부 (기본값: false)
	entriesField    string         // 배치 읽기: payload 에서 배열을 추출할 필드 (예: "rooms")
	entriesVar      string         // 배치 읽기: 배열 각 요소를 매핑할 변수명 (예: "item" → {item})

	// 조회 모드 파라미터
	readMode      ReadMode      // 조회 모드 (기본값: latest)
	count         int           // last_n, since_n 에서 사용
	duration      time.Duration // duration 에서 사용
	fromTemplate  string        // time_range 의 from 파라미터 (리터럴 또는 {field})
	toTemplate    string        // time_range 의 to 파라미터 (리터럴 또는 {field})
	sinceTemplate string        // since_n 의 since 파라미터 (리터럴 또는 {field})
}

// NewStoreReadNode 는 새로운 StoreReadNode를 생성하는 팩토리 함수이다.
func NewStoreReadNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &StoreReadNode{
		BaseNode:  base,
		namespace: "default",
		outputKey: "store_value",
		readMode:  ReadModeLatest,
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

// Init 은 StoreReadNode를 초기화하고 AgentResolver로 Store 에이전트를 해석한다.
func (n *StoreReadNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	// AgentResolver를 통해 Store 에이전트 해석
	if err := n.resolveStore(ctx); err != nil {
		return fmt.Errorf("store-read init: %w", err)
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// resolveStore 는 AgentResolver를 사용하여 Store 에이전트를 찾고 StoreReader를 추출한다.
func (n *StoreReadNode) resolveStore(ctx context.Context) error {
	// config["_store"]로 직접 주입된 경우 (테스트용 하위 호환성)
	if s, ok := n.config["_store"]; ok {
		if reader, ok := s.(StoreReader); ok {
			n.store = reader
			return nil
		}
	}

	// AgentRef가 없으면 즉시 실패 (fail-fast).
	// 이전에는 lazy-fail (Process 시점 ErrStoreNotConfigured)이었으나
	// 디버깅을 어렵게 만들고 잘못 설정된 플로우가 Running 상태로 진입하는 문제가 있어
	// 다른 스토리지 노드(influxdb/tsdb)와 동일하게 Init 단계에서 거부한다.
	if n.agentRef == nil {
		return fmt.Errorf("agent_ref is required for store-read node")
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

	// NodeStoreAdapter → StoreReader 타입 단언
	reader, ok := instance.(StoreReader)
	if !ok {
		return fmt.Errorf("store agent %q returned incompatible type for StoreReader", n.agentRef.AgentName)
	}
	n.store = reader

	return nil
}

// Shutdown 은 StoreReadNode를 종료한다.
func (n *StoreReadNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 StoreReadNode의 설정을 적용한다.
//
// 지원하는 설정 키:
//   - "key_template"     : string - 키 템플릿 ({field} 형식 플레이스홀더)
//   - "namespace"        : string - Store 네임스페이스 (기본값: "default")
//   - "output_key"       : string - 조회 결과를 저장할 payload 키 (기본값: "store_value")
//   - "include_metadata" : bool   - 메타데이터를 함께 조회할지 여부 (기본값: false)
//   - "read_mode"        : string - 조회 모드 (기본값: "latest")
//   - "count"            : int    - last_n, since_n 에서 사용 (> 0)
//   - "duration"         : string - duration 에서 사용 (예: "5m", "1h")
//   - "from"             : string - time_range 의 시작 (RFC3339 또는 "{field}")
//   - "to"               : string - time_range 의 끝 (RFC3339 또는 "{field}")
//   - "since"            : string - since_n 의 기준 시각 (RFC3339 또는 "{field}")
func (n *StoreReadNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	if v, ok := config["key_template"]; ok {
		if s, ok := v.(string); ok {
			n.keyTemplate = s
		}
	}

	if v, ok := config["namespace"]; ok {
		if s, ok := v.(string); ok {
			n.namespace = s
		}
	}

	if v, ok := config["output_key"]; ok {
		if s, ok := v.(string); ok {
			n.outputKey = s
		}
	}

	if v, ok := config["include_metadata"]; ok {
		if b, ok := v.(bool); ok {
			n.includeMetadata = b
		}
	}

	if v, ok := config["read_mode"]; ok {
		if s, ok := v.(string); ok && s != "" {
			n.readMode = ReadMode(s)
		}
	}

	if v, ok := config["count"]; ok {
		if i, err := coerceInt(v); err == nil {
			n.count = i
		}
	}

	if v, ok := config["duration"]; ok {
		if s, ok := v.(string); ok && s != "" {
			d, err := time.ParseDuration(s)
			if err != nil {
				return fmt.Errorf("store-read: invalid duration %q: %w", s, err)
			}
			n.duration = d
		}
	}

	if v, ok := config["from"]; ok {
		if s, ok := v.(string); ok {
			n.fromTemplate = s
		}
	}

	if v, ok := config["to"]; ok {
		if s, ok := v.(string); ok {
			n.toTemplate = s
		}
	}

	if v, ok := config["since"]; ok {
		if s, ok := v.(string); ok {
			n.sinceTemplate = s
		}
	}

	if v, ok := config["entries_field"]; ok {
		if s, ok := v.(string); ok && s != "" {
			n.entriesField = s
		}
	}
	if v, ok := config["entries_var"]; ok {
		if s, ok := v.(string); ok && s != "" {
			n.entriesVar = s
		}
	}

	return n.validateModeParams()
}

// validateModeParams 는 read_mode 에 필요한 파라미터가 모두 채워졌는지 검증한다.
// 동적 참조({field})는 런타임 해석 대상이므로 비어 있지만 않으면 통과시킨다.
func (n *StoreReadNode) validateModeParams() error {
	switch n.readMode {
	case ReadModeLatest:
		return nil
	case ReadModeLastN:
		if n.count <= 0 {
			return fmt.Errorf("store-read: read_mode %q requires count > 0", n.readMode)
		}
		return nil
	case ReadModeDuration:
		if n.duration <= 0 {
			return fmt.Errorf("store-read: read_mode %q requires duration > 0", n.readMode)
		}
		return nil
	case ReadModeTimeRange:
		if n.fromTemplate == "" || n.toTemplate == "" {
			return fmt.Errorf("store-read: read_mode %q requires from and to", n.readMode)
		}
		return nil
	case ReadModeSinceN:
		if n.sinceTemplate == "" {
			return fmt.Errorf("store-read: read_mode %q requires since", n.readMode)
		}
		if n.count <= 0 {
			return fmt.Errorf("store-read: read_mode %q requires count > 0", n.readMode)
		}
		return nil
	default:
		return fmt.Errorf("store-read: unknown read_mode %q", n.readMode)
	}
}

// Process 는 Store에서 값을 조회하여 메시지 payload에 추가하고 반환한다.
// 결과는 항상 output_key 아래 []map[string]any 형태로 기록되며,
// 키가 존재하지 않으면 빈 배열을 기록한다.
func (n *StoreReadNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	if n.store == nil {
		return nil, ErrStoreNotConfigured
	}

	// entries_field 배치 모드
	if n.entriesField != "" {
		return n.processBatch(ctx, msg)
	}

	return n.processSingle(ctx, msg)
}

// processSingle 은 단일 키 조회 (기존 동작).
func (n *StoreReadNode) processSingle(ctx context.Context, msg message.Message) ([]message.Message, error) {
	key, err := resolveKeyTemplate(n.keyTemplate, msg)
	if err != nil {
		return nil, fmt.Errorf("store-read: %w", err)
	}

	query, err := n.buildQuery(msg.Payload())
	if err != nil {
		return nil, fmt.Errorf("store-read: %w", err)
	}

	reader, ok := n.store.(HistoryQueryReader)
	if !ok {
		return nil, fmt.Errorf("store-read: store does not implement HistoryQueryReader")
	}

	entries, err := reader.QueryHistory(ctx, key, query)
	if err != nil {
		return nil, fmt.Errorf("store-read: %w", err)
	}
	if entries == nil {
		entries = []map[string]any{}
	}

	msg.Payload().Set(n.outputKey, entries)

	if n.includeMetadata {
		if mr, ok := n.store.(MetadataReader); ok {
			meta, metaErr := mr.GetMetadata(ctx, key)
			if metaErr == nil {
				for k, v := range meta {
					msg.Payload().Set(k, v)
				}
			}
		}
	}

	return []message.Message{msg}, nil
}

// processBatch 는 entries_field 의 배열 각 요소를 entries_var 로 매핑하여 다중 키를 조회한다.
// 결과: output_key 에 map[string]any (요소값 → []map[string]any) 형태로 기록.
func (n *StoreReadNode) processBatch(ctx context.Context, msg message.Message) ([]message.Message, error) {
	raw, ok := msg.Payload().Get(n.entriesField)
	if !ok {
		return nil, fmt.Errorf("store-read: entries_field %q not found in payload", n.entriesField)
	}
	arr := toAnySlice(raw)
	if arr == nil {
		return nil, fmt.Errorf("store-read: entries_field %q is not an array (got %T: %v)", n.entriesField, raw, raw)
	}

	reader, ok := n.store.(HistoryQueryReader)
	if !ok {
		return nil, fmt.Errorf("store-read: store does not implement HistoryQueryReader")
	}

	query, err := n.buildQuery(msg.Payload())
	if err != nil {
		return nil, fmt.Errorf("store-read: %w", err)
	}

	varName := n.entriesVar
	if varName == "" {
		varName = "item"
	}

	// SPEC-NODE-001 v1.5.0: 객체 배열 지원 — 요소 원본을 그대로 payload 에 set,
	// key_template 의 {entries_var.field} dot notation 으로 nested 접근.
	// 결과 map 키는 resolved store 키 사용 (primitive·객체 일관). Breaking: 이전엔
	// elemStr 가 결과 키였으나 이제 resolved key 로 변경.
	result := make(map[string]any, len(arr))
	for i, elem := range arr {
		// 임시로 변수를 payload 에 설정하여 resolveKeyTemplate 이 참조하도록 함.
		// primitive 든 객체든 원본 그대로 주입한다.
		msg.Payload().Set(varName, elem)
		key, err := resolveKeyTemplate(n.keyTemplate, msg)
		if err != nil {
			return nil, fmt.Errorf("store-read: batch key resolve at index %d: %w", i, err)
		}

		entries, err := reader.QueryHistory(ctx, key, query)
		if err != nil {
			return nil, fmt.Errorf("store-read: batch read %q: %w", key, err)
		}
		if entries == nil {
			entries = []map[string]any{}
		}
		result[key] = entries
	}

	// 임시 변수 정리 (varName 은 n.entriesVar 또는 기본값 "item")
	msg.Payload().Delete(varName)
	msg.Payload().Set(n.outputKey, result)

	return []message.Message{msg}, nil
}

// buildQuery 는 메시지 payload 로부터 동적 참조를 해석하여 system.HistoryQuery 를 구성한다.
func (n *StoreReadNode) buildQuery(payload message.Payload) (system.HistoryQuery, error) {
	q := system.HistoryQuery{
		Mode:     system.QueryMode(n.readMode),
		Count:    n.count,
		Duration: n.duration,
	}

	if n.fromTemplate != "" {
		t, err := resolveTimeTemplate(n.fromTemplate, payload)
		if err != nil {
			return q, fmt.Errorf("from: %w", err)
		}
		q.From = t
	}
	if n.toTemplate != "" {
		t, err := resolveTimeTemplate(n.toTemplate, payload)
		if err != nil {
			return q, fmt.Errorf("to: %w", err)
		}
		q.To = t
	}
	if n.sinceTemplate != "" {
		t, err := resolveTimeTemplate(n.sinceTemplate, payload)
		if err != nil {
			return q, fmt.Errorf("since: %w", err)
		}
		q.Since = t
	}

	if err := q.Validate(); err != nil {
		return q, err
	}
	return q, nil
}

// resolveTimeTemplate 은 템플릿 문자열 또는 {field} 참조를 time.Time 으로 해석한다.
// 다음 입력을 지원한다:
//   - epoch ms 리터럴: "1713268800000" (int64)
//   - RFC3339 리터럴: "2026-04-16T12:00:00Z" (사람이 입력하는 설정 값용 호환)
//   - 단일 플레이스홀더: "{field}" → payload[field] 값 (epoch ms int64/float64 우선, string/time.Time 호환)
func resolveTimeTemplate(template string, payload message.Payload) (time.Time, error) {
	trimmed := strings.TrimSpace(template)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("empty time template")
	}

	// 단일 {field} 형태인지 판별
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") &&
		strings.Count(trimmed, "{") == 1 && strings.Count(trimmed, "}") == 1 {
		field := trimmed[1 : len(trimmed)-1]
		raw, ok := payload.Get(field)
		if !ok {
			return time.Time{}, fmt.Errorf("time template field %q not found in payload", field)
		}
		return coerceTime(raw)
	}

	// 리터럴: 먼저 epoch ms 로 파싱 시도, 실패 시 RFC3339 로 폴백
	if ms, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return time.UnixMilli(ms), nil
	}
	t, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time %q (expected epoch ms or RFC3339): %w", trimmed, err)
	}
	return t, nil
}

// coerceTime 은 payload 에서 꺼낸 임의 값을 time.Time 으로 변환한다.
// 프로젝트 컨벤션에 따라 숫자는 epoch 밀리초(UnixMilli)로 해석한다.
// 지원 타입: int64/int/float64(UnixMilli), string(epoch ms 또는 RFC3339), time.Time.
func coerceTime(v any) (time.Time, error) {
	switch t := v.(type) {
	case int64:
		return time.UnixMilli(t), nil
	case int:
		return time.UnixMilli(int64(t)), nil
	case float64:
		// JSON 디코딩된 숫자는 float64 로 오는 경우가 많다
		return time.UnixMilli(int64(t)), nil
	case time.Time:
		return t, nil
	case string:
		// 숫자 문자열이면 epoch ms 로 해석, 아니면 RFC3339 로 폴백
		if ms, err := strconv.ParseInt(t, 10, 64); err == nil {
			return time.UnixMilli(ms), nil
		}
		parsed, err := time.Parse(time.RFC3339, t)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid time string %q (expected epoch ms or RFC3339): %w", t, err)
		}
		return parsed, nil
	default:
		return time.Time{}, fmt.Errorf("unsupported time type %T", v)
	}
}

// coerceInt 은 설정 값에서 int 를 추출한다. JSON 디코딩으로 float64 가 올 수도 있으므로 함께 처리한다.
func coerceInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("unsupported int type %T", v)
	}
}
