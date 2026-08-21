package node

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// storageBackend 는 storage-write 노드가 기록을 위임하는 백엔드 어댑터이다.
// 노드는 백엔드 중립 기록 단위(시리즈 + 측정값 + 태그 + 시각)를 만들고,
// 백엔드가 그것을 자신의 저장 모델로 번역한다.
//
// 구현: storeBackend (키-값 저장소), influxdbBackend (시계열 DB).
type storageBackend interface {
	// backendName 은 에러 메시지에 쓰일 백엔드 이름이다.
	backendName() string
	// resolve 는 해석된 에이전트에서 쓰기 핸들을 확보한다.
	resolve(underlying any, cfg *storageWriteConfig) error
	// write 는 한 배치(같은 시리즈로 묶인 측정값들)를 기록한다.
	write(ctx context.Context, batch storageBatch) error
}

// storageBatch 는 한 메시지에서 같은 시리즈로 묶인 기록 단위이다.
// 시계열 백엔드는 이를 하나의 point 로, 키-값 백엔드는 측정값별 개별 쓰기로 번역한다.
type storageBatch struct {
	seriesKey string
	tags      map[string]string
	values    []storageValue
	timestamp int64 // epoch ms
}

// storageValue 는 배치 안의 단일 측정값이다.
type storageValue struct {
	name  string // 시계열의 field 이름 / 저장소의 field
	value any
}

// payload 키 처리 모드.
const (
	// payloadModeFields 는 payload 의 모든 키/값을 하나의 시리즈 아래
	// 여러 측정값으로 기록한다 (시리즈 이름은 measurement 설정).
	payloadModeFields = "fields"
	// payloadModeSplit 는 payload 의 각 키를 개별 시리즈로 분리하고
	// 값을 그 시리즈의 단일 측정값으로 기록한다.
	payloadModeSplit = "split"
)

// splitValueName 은 split 모드에서 측정값에 부여하는 고정 이름이다.
// split 모드에서는 시리즈 이름이 이미 측정 종류를 나타내므로 값 이름은 관례상 "value" 다.
const splitValueName = "value"

// storageWriteConfig 는 storage-write 노드의 파싱된 설정이다.
// 백엔드 중립 필드와 백엔드 전용 필드가 함께 들어 있으며, 해당 없는 백엔드는
// 자신과 무관한 필드를 무시한다(에이전트만 바꿔도 같은 설정이 그대로 동작한다).
type storageWriteConfig struct {
	// --- 백엔드 중립 ---
	payloadMode string
	measurement string          // fields 모드의 시리즈 이름 ({…} 보간 지원)
	excludeKeys map[string]bool // 측정값으로 쓰지 않을 payload 키

	// --- 백엔드 전용 (해당 없는 백엔드는 무시) ---
	namespace string        // store
	ttl       time.Duration // store
	boolToInt bool          // influxdb
}

// agentTyper 는 에이전트의 타입 문자열을 노출하는 인터페이스이다.
// storage-write 는 이 값으로 백엔드를 선택한다 ("store" / "influxdb").
type agentTyper interface {
	Type() string
}

// StorageWriteNode 는 메시지를 스토리지 백엔드에 기록하는 통합 노드이다.
//
// 기록 규약은 메시지 구조를 그대로 따른다 — 설정으로 경로를 일일이 매핑하지 않는다:
//
//	msg.payload   → 측정값 (키 = 측정 종류, 값 = 측정값). 1개 이상.
//	msg.metadata  → 태그 (그룹은 "device.id" 형태의 평면 태그로 펼침)
//	msg.timestamp → 기록 시각 (epoch ms)
//
// payload 키를 어떻게 시리즈에 배치할지만 payload_mode 로 고른다:
//
//	fields 모드 — 모든 키/값을 measurement 하나 아래 여러 측정값으로.
//	split  모드 — 키마다 별도 시리즈로 분리하고 값을 그 시리즈의 값으로.
//
// 백엔드는 agent_ref 가 가리키는 에이전트의 타입으로 결정되므로, 설정을 그대로
// 둔 채 에이전트만 교체하면 저장 대상이 바뀐다.
//
// 기록 후 원본 메시지를 그대로 다음 노드로 전달한다 (pass-through).
type StorageWriteNode struct {
	*BaseNode
	backend  storageBackend
	resolver AgentResolver
	agentRef *flow.AgentRef
	cfg      storageWriteConfig
}

// NewStorageWriteNode 는 새로운 StorageWriteNode 를 생성하는 팩토리 함수이다.
func NewStorageWriteNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &StorageWriteNode{
		BaseNode: base,
		agentRef: def.AgentRef,
	}
	n.cfg.namespace = "default"
	n.cfg.payloadMode = payloadModeFields
	if r, ok := base.config["_agent_resolver"]; ok {
		if resolver, ok := r.(AgentResolver); ok {
			n.resolver = resolver
		}
	}
	return n, nil
}

// Init 은 노드를 초기화하고 agent_ref 가 가리키는 에이전트에서 백엔드를 결정한다.
func (n *StorageWriteNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if err := n.resolveBackend(ctx); err != nil {
		return fmt.Errorf("storage-write init: %w", err)
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Shutdown 은 노드를 종료한다.
func (n *StorageWriteNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// resolveBackend 는 에이전트를 해석하고 그 타입에 맞는 백엔드를 연결한다.
//
// 백엔드 선택은 에이전트의 Type() 값으로 한다. 능력(interface) 기반 판별은
// 쓸 수 없다 — Store 에이전트와 InfluxDB 에이전트가 둘 다 Process([]byte) 를
// 가지므로 influxdbAgent 단언이 양쪽 모두에 성립하기 때문이다.
func (n *StorageWriteNode) resolveBackend(ctx context.Context) error {
	// 테스트용 직접 주입 경로.
	//   _storage_backend: 백엔드를 통째로 주입한다(백엔드 구현 자체를 대체).
	//   _storage_agent:   에이전트만 주입하고 백엔드 선택/연결은 실제 경로를 탄다.
	if b, ok := n.config["_storage_backend"]; ok {
		if backend, ok := b.(storageBackend); ok {
			n.backend = backend
			return nil
		}
	}
	if a, ok := n.config["_storage_agent"]; ok {
		backend, err := newStorageBackend(a)
		if err != nil {
			return err
		}
		if err := backend.resolve(a, &n.cfg); err != nil {
			return fmt.Errorf("%s backend: %w", backend.backendName(), err)
		}
		n.backend = backend
		return nil
	}

	if n.agentRef == nil {
		return fmt.Errorf("agent_ref is required for storage-write node")
	}
	if n.resolver == nil {
		return fmt.Errorf("agent resolver not configured")
	}

	transport, err := n.resolver.ResolveAgent(ctx, *n.agentRef)
	if err != nil {
		return fmt.Errorf("failed to resolve storage agent %q: %w", n.agentRef.AgentName, err)
	}
	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return fmt.Errorf("storage agent transport does not support AgentAccessor")
	}
	underlying := accessor.UnderlyingAgent()

	backend, err := newStorageBackend(underlying)
	if err != nil {
		return fmt.Errorf("agent %q: %w", n.agentRef.AgentName, err)
	}
	if err := backend.resolve(underlying, &n.cfg); err != nil {
		return fmt.Errorf("agent %q (%s backend): %w", n.agentRef.AgentName, backend.backendName(), err)
	}
	n.backend = backend
	return nil
}

// newStorageBackend 는 에이전트 타입에 대응하는 백엔드를 만든다.
func newStorageBackend(underlying any) (storageBackend, error) {
	typer, ok := underlying.(agentTyper)
	if !ok {
		return nil, fmt.Errorf("agent does not expose Type()")
	}
	switch t := typer.Type(); t {
	case "store":
		return &storeBackend{}, nil
	case "influxdb":
		return &influxdbBackend{}, nil
	default:
		return nil, fmt.Errorf("unsupported agent type %q for storage-write (expected: store, influxdb)", t)
	}
}

// Configure 는 storage-write 의 설정을 파싱·검증한다.
//
// 백엔드 중립 키:
//   - "payload_mode": "fields" | "split" (기본 "fields")
//   - "measurement": string - fields 모드의 시리즈 이름. {…} 보간 지원. fields 모드에서 필수
//   - "exclude_keys": []string - 측정값으로 쓰지 않을 payload 키
//
// 백엔드 전용 키 (해당 없는 백엔드는 무시):
//   - "namespace", "ttl" (store) / "bool_to_int" (influxdb)
func (n *StorageWriteNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	if v, ok := config["payload_mode"]; ok {
		if s, ok := v.(string); ok && s != "" {
			if s != payloadModeFields && s != payloadModeSplit {
				return fmt.Errorf(
					"storage-write: invalid payload_mode %q (must be one of: fields, split)", s)
			}
			n.cfg.payloadMode = s
		}
	}

	if v, ok := config["measurement"]; ok {
		if s, ok := v.(string); ok {
			n.cfg.measurement = s
		}
	}
	// fields 모드는 시리즈 이름을 설정에서 받아야 한다.
	// split 모드는 payload 키가 시리즈 이름이므로 measurement 를 쓰지 않는다.
	if n.cfg.payloadMode == payloadModeFields && n.cfg.measurement == "" {
		return fmt.Errorf("storage-write: measurement 는 fields 모드에서 필수입니다")
	}

	if v, ok := config["exclude_keys"]; ok {
		keys, err := parseStringList(v, "storage-write: exclude_keys")
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			n.cfg.excludeKeys = make(map[string]bool, len(keys))
			for _, k := range keys {
				n.cfg.excludeKeys[k] = true
			}
		}
	}

	// --- 백엔드 전용 ---
	if v, ok := config["namespace"]; ok {
		if s, ok := v.(string); ok && s != "" {
			n.cfg.namespace = s
		}
	}
	if v, ok := config["ttl"]; ok {
		if s, ok := v.(string); ok && s != "" {
			d, err := time.ParseDuration(s)
			if err != nil {
				return fmt.Errorf("storage-write: invalid ttl %q: %w", s, err)
			}
			n.cfg.ttl = d
		}
	}
	if v, ok := config["bool_to_int"]; ok {
		if b, ok := v.(bool); ok {
			n.cfg.boolToInt = b
		}
	}

	return nil
}

// parseStringList 는 config 의 문자열 리스트를 파싱한다 ([]any / []string 모두 허용).
func parseStringList(v any, where string) ([]string, error) {
	switch list := v.(type) {
	case []string:
		return list, nil
	case []any:
		out := make([]string, 0, len(list))
		for i, item := range list {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s[%d] must be a string", where, i)
			}
			if s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("%s must be a list of strings", where)
	}
}

// Process 는 메시지를 기록 단위로 바꿔 백엔드에 기록하고 원본 메시지를 반환한다.
//
// 에러 정책:
//   - fields 모드에서 measurement 템플릿 해석 실패는 에러이다 (시리즈 이름은 필수).
//   - payload 에 기록할 측정값이 없으면 아무것도 기록하지 않고 pass-through 한다.
//   - 백엔드 쓰기 실패는 첫 실패에서 즉시 에러를 반환한다 (fail-fast).
func (n *StorageWriteNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	if n.backend == nil {
		return nil, fmt.Errorf("storage-write: backend not configured")
	}

	batches, err := n.buildBatches(msg)
	if err != nil {
		return nil, err
	}
	for _, batch := range batches {
		if err := n.backend.write(ctx, batch); err != nil {
			return nil, fmt.Errorf("storage-write (%s): %w", n.backend.backendName(), err)
		}
	}
	return []message.Message{msg}, nil
}

// buildBatches 는 메시지에서 기록 배치를 구성한다.
//
//	fields 모드 → 배치 1개 (measurement 시리즈에 payload 키들이 측정값으로)
//	split  모드 → payload 키마다 배치 1개 (키가 시리즈 이름, 값 이름은 "value")
//
// 두 모드 모두 태그는 metadata, 시각은 msg.timestamp 로 동일하게 채운다.
// 결과 순서는 payload 키 정렬 순서로 고정하여 결정적이다.
func (n *StorageWriteNode) buildBatches(msg message.Message) ([]storageBatch, error) {
	tags := metadataTags(msg)
	ts := msg.Timestamp().UnixMilli()

	// payload 에서 측정값 후보를 뽑는다 (제외 키 / 이름 규칙 위반 키는 건너뛴다).
	payload := msg.Payload().ToMap()
	names := make([]string, 0, len(payload))
	for k := range payload {
		if n.cfg.excludeKeys[k] {
			continue
		}
		// 측정 종류 이름으로 쓸 수 없는 키는 건너뛴다 (태그/시리즈 이름 규칙과 동일).
		if !fieldPattern.MatchString(k) {
			continue
		}
		names = append(names, k)
	}
	if len(names) == 0 {
		return nil, nil
	}
	sort.Strings(names)

	if n.cfg.payloadMode == payloadModeSplit {
		// 키마다 별도 measurement. 값 모양에 따라 필드를 정한다:
		//   스칼라  → 필드 하나, 이름은 관례상 "value" (measurement 가 이미 종류를 나타냄).
		//   오브젝트 → 오브젝트의 각 키가 필드가 된다 (예: radio{count, rssi}).
		// 오브젝트를 통째로 "value" 에 넣으면 JSON 문자열로 뭉개져 집계·차트가 쓸 수 없다.
		batches := make([]storageBatch, 0, len(names))
		for _, k := range names {
			values := splitValues(payload[k])
			if len(values) == 0 {
				continue
			}
			batches = append(batches, storageBatch{
				seriesKey: k,
				tags:      tags,
				values:    values,
				timestamp: ts,
			})
		}
		return batches, nil
	}

	// fields 모드: 하나의 시리즈에 모든 키/값을 측정값으로 담는다.
	seriesKey, err := resolveKeyTemplate(n.cfg.measurement, msg)
	if err != nil {
		return nil, fmt.Errorf("storage-write: measurement %q: %w", n.cfg.measurement, err)
	}
	values := make([]storageValue, 0, len(names))
	for _, k := range names {
		values = append(values, storageValue{name: k, value: payload[k]})
	}
	return []storageBatch{{
		seriesKey: seriesKey,
		tags:      tags,
		values:    values,
		timestamp: ts,
	}}, nil
}

// splitValues 는 split 모드에서 payload 값 하나를 그 measurement 의 필드 목록으로 편다.
//
//	스칼라(숫자/문자열/불리언/nil) → [{name: "value", value: v}]
//	오브젝트                      → 각 키가 필드. 필드 이름 규칙을 위반하는 키는 건너뛴다.
//	배열 등 그 외                  → [{name: "value", value: v}] (백엔드가 JSON 문자열로 기록)
//
// 오브젝트가 비어 있거나 쓸 수 있는 키가 하나도 없으면 빈 목록을 반환한다(그 measurement 생략).
func splitValues(v any) []storageValue {
	obj, ok := v.(map[string]any)
	if !ok {
		return []storageValue{{name: splitValueName, value: v}}
	}
	names := make([]string, 0, len(obj))
	for k := range obj {
		if !fieldPattern.MatchString(k) {
			continue
		}
		names = append(names, k)
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	out := make([]storageValue, 0, len(names))
	for _, k := range names {
		out = append(out, storageValue{name: k, value: obj[k]})
	}
	return out
}

// metadataTags 는 메시지 metadata 를 태그 맵으로 변환한다.
//
// 그룹(agent/device 등)은 "{group}.{field}" 평면 태그로 펼친다 — 스토리지 태그는
// 평면 문자열 key=value 이므로 중첩 그룹을 단일 태그로 표현할 수 없기 때문이다.
// (SlimGroupsToID 는 내부 제어 마커만 제거하고 그룹은 full 로 전달한다.)
func metadataTags(msg message.Message) map[string]string {
	raw := message.SlimGroupsToID(msg.Metadata().Raw())
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		switch val := v.(type) {
		case string:
			if tagKeyPattern.MatchString(k) {
				out[k] = val
			}
		case map[string]string:
			for fk, fv := range val {
				out[k+"."+fk] = fv
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
