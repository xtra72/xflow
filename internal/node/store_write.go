package node

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent/system"
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

// StoreMetaWriter 는 data_type/tags 메타데이터를 함께 지정하여 기록할 수 있는
// 선택적(optional) 인터페이스이다. system.NodeStoreAdapter 가 이를 구현한다.
//
// 중요: 옵션 타입은 system.StoreWriteMeta 를 그대로 사용한다. 과거에는 node 패키지에
// 동일 형태의 StoreWriteMeta 를 중복 정의했는데, Go 인터페이스 만족은 메서드 시그니처가
// 정확히 일치해야 하므로(파라미터 타입 포함) 어댑터의 SetWithMeta(system.StoreWriteMeta)
// 가 이 인터페이스를 만족하지 못했다. 그 결과 store-write 노드의 data_type/metric_type
// 메타가 한 번도 적용되지 못하고(키가 항상 동적 string/unknown), 일반 Set 로 폴백했다.
// node 패키지는 이미 system 을 import 하므로(store_read.go 등) system 타입을 직접 사용한다.
//
// store 가 이 인터페이스를 만족하고 노드에 data_type/metric_type/tags 가 설정된 경우에만
// SetWithMeta 가 사용되며, 그 외에는 기존 StoreWriter.Set/SetWithTTL 로 폴백한다 (하위 호환).
type StoreMetaWriter interface {
	SetWithMeta(ctx context.Context, key string, value any, opts system.StoreWriteMeta) error
}

// 컴파일 타임 보장: 실제 store 어댑터(*system.NodeStoreAdapter)가 StoreMetaWriter 를
// 만족해야 한다. 과거 opts 타입(node.StoreWriteMeta vs system.StoreWriteMeta) 불일치로
// 만족하지 못해 메타 경로가 죽어있던 회귀를 영구 차단한다.
var _ StoreMetaWriter = (*system.NodeStoreAdapter)(nil)

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
	keyMappings map[string]string // 다중 (키,값) 매핑: 키 템플릿 → 값 경로. 한 메시지에서 여러 키를 기록 (key_template 과 병행 가능)
	namespace   string            // Store 네임스페이스
	ttl         time.Duration     // TTL (0이면 만료 없음)
	dataType    string            // 기록되는 키에 부여할 data_type (빈 문자열이면 미지정)
	metricType  string            // 기록되는 키에 부여할 metric_type. 리터럴 또는 `$.` 경로 (빈 문자열이면 미지정)
	tags        map[string]string // 기록되는 키에 부여할 태그. 값은 리터럴 또는 `$.` 경로 (nil/빈 맵이면 미지정)

	// metrics 는 다중 메트릭 설정이다. 비어 있지 않으면 key_template 의 키에 대해
	// 각 metric 별로 별도 시리즈(metric_type 로 구분)를 기록한다. 각 metric 은 자체
	// value_key/data_type/dead-band 를 갖는다. 비어 있으면 legacy 단일 필드
	// (value_key/metric_type/data_type/min_*)로 동작한다(하위호환).
	metrics []metricSpec

	// dead-band(미세변화 억제) 설정. 미세 흔들림에 따른 과도한 데이터 생성을 막는다.
	// minInterval 이 0이면 기능 전체가 비활성화된다(완전 하위호환).
	//
	// 저장 규칙(시리즈별 = key+metric_type+sorted(tags)):
	//   - 경과시간 >= minInterval                 → 저장 (heartbeat: 변화가 없어도 강제 기록)
	//   - 경과시간 <  minInterval AND 미세변화      → 생략
	//   - 경과시간 <  minInterval AND 유의미한 변화  → 저장
	//
	// "유의미한 변화" 판정(직전 *저장값* 기준):
	//   - 숫자: |Δ| > minChange (절대) 또는 |Δ|/|prev|*100 > minChangePercent (상대) 중 하나라도 참
	//   - 비숫자: 직전 저장값과 다르면 유의미, 같으면 미세변화로 간주
	minInterval         time.Duration // 최대 억제 구간. 0이면 dead-band 비활성.
	minChange           float64       // 절대 변화 임계값 (hasMinChange 일 때만 적용)
	hasMinChange        bool          // min_change 설정 여부
	minChangePercent    float64       // 상대(%) 변화 임계값 (hasMinChangePercent 일 때만 적용)
	hasMinChangePercent bool          // min_change_percent 설정 여부

	// 스냅샷 dead-band 우회: device_state.report 처럼 "전체 상태 스냅샷" 메시지는
	// 모든 메트릭을 매번 함께 기록해야 한다(메트릭별 카운트 일치). 이런 메시지는
	// dead-band(min_interval/min_change)를 완전히 우회하여 무조건 저장한다.
	//   - snapshotPath: 스냅샷 식별값의 `$.` 경로 (예: "$.payload.message_type"). "" 면 비활성.
	//   - snapshotValues: 스냅샷으로 간주할 값 집합 (예: {"device_state.report"}).
	snapshotPath   string          // 스냅샷 식별 경로 ("" = 비활성)
	snapshotValues map[string]bool // 스냅샷으로 간주할 값들

	dedupMu    sync.Mutex            // lastStored 보호
	lastStored map[string]dedupState // 시리즈 시그니처 → 마지막 저장 상태
}

// dedupState 는 한 시리즈(key+metric+tags)의 마지막으로 *저장된* 값과 시각이다.
// dead-band 판정의 기준점이 되며, 억제된(미저장) 메시지로는 갱신되지 않는다.
type dedupState struct {
	value any
	at    time.Time
}

// deadbandConf 는 단일 기록(메트릭 또는 legacy 단일 키)의 미세변화 억제 설정이다.
// minInterval 이 0이면 비활성(항상 저장).
type deadbandConf struct {
	minInterval         time.Duration
	minChange           float64
	hasMinChange        bool
	minChangePercent    float64
	hasMinChangePercent bool
}

// metricSpec 은 다중 메트릭의 한 항목이다. 각 메트릭은 key_template 의 키에
// 자신의 metric_type 으로 구분되는 시리즈로 기록되며, 고유한 value_key/data_type/
// dead-band 를 갖는다.
type metricSpec struct {
	metricType string // 리터럴 또는 `$.` 경로
	valueKey   string // `$.` 경로 (빈 값은 Configure 에서 "$.payload.value" 로 기본 설정)
	dataType   string // 6종 enum 또는 ""
	deadband   deadbandConf
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

// metricTypePattern 은 metric_type 으로 허용되는 문자 패턴이다 (system 계층의 metricTypePattern 과 동일).
// metric_type 이 리터럴일 때 Configure 시점에, `$.` 경로일 때 해석 결과를 Process 시점에 검증한다.
var metricTypePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// NewStoreWriteNode 는 새로운 StoreWriteNode를 생성하는 팩토리 함수이다.
func NewStoreWriteNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &StoreWriteNode{
		BaseNode:   base,
		namespace:  "default",
		agentRef:   def.AgentRef,
		lastStored: make(map[string]dedupState),
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

	// key_mappings (선택): 다중 (키,값) 매핑.
	//   - 맵의 key  = 키 템플릿 (key_template 과 동일한 {...} 보간 지원, 리터럴도 가능)
	//   - 맵의 value = 값 경로 (value_key 와 동일한 규칙: 빈 문자열이면 전체 payload, 그 외는 `$.` 경로)
	// 각 엔트리가 하나의 store 쓰기가 되며, key_template/value_key 와 병행 지정 가능하다.
	if v, ok := config["key_mappings"]; ok {
		if raw, ok := v.(map[string]any); ok && len(raw) > 0 {
			parsed := make(map[string]string, len(raw))
			for kt, vp := range raw {
				if kt == "" {
					return fmt.Errorf("store-write: key_mappings 키 템플릿은 비어 있을 수 없습니다")
				}
				sv, ok := vp.(string)
				if !ok {
					return fmt.Errorf("store-write: key_mappings[%q] value must be a string", kt)
				}
				// value 는 value_key 와 동일 규칙: 빈 문자열(전체 payload) 또는 `$.` 경로.
				if sv != "" && !strings.HasPrefix(sv, "$.") {
					return fmt.Errorf(
						"store-write: key_mappings[%q] value %q must be a $.-path (e.g. $.payload.state.mode) or empty for whole payload", kt, sv)
				}
				parsed[kt] = sv
			}
			n.keyMappings = parsed
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
	// 6종 enum(int/float/string/boolean/bytes/json) 또는 "auto"(값 타입 추론)만 허용한다.
	// "auto" 는 쓰기 값의 Go 타입에서 구체 타입을 추론해 키별로 고정한다(가변 타입 단일 노드 지원).
	if v, ok := config["data_type"]; ok {
		if s, ok := v.(string); ok && s != "" {
			if s != system.DataTypeAuto && !validDataTypes[s] {
				return fmt.Errorf(
					"store-write: invalid data_type %q (must be one of: int, float, string, boolean, bytes, json, auto)", s)
			}
			n.dataType = s
		}
	}

	// metric_type (선택): 기록되는 키에 부여할 metric_type.
	// 값은 리터럴 또는 `$.` 경로(메시지 필드 참조)이다. `$.` 로 시작하면 Process 시점에
	// resolveTemplateExpr 로 메시지에서 해석하고, 아니면 리터럴 그대로 사용한다.
	// 리터럴인 경우에만 Configure 시점에 정규식(^[a-zA-Z0-9_-]+$)을 검증한다.
	// (`$.` 경로의 해석 결과 검증은 Process 시점의 책임이다.)
	if v, ok := config["metric_type"]; ok {
		if s, ok := v.(string); ok && s != "" {
			if !strings.HasPrefix(s, "$.") && !metricTypePattern.MatchString(s) {
				return fmt.Errorf(
					"store-write: invalid metric_type %q (must match ^[a-zA-Z0-9_-]+$ or be a $.-path)", s)
			}
			n.metricType = s
		}
	}

	// tags (선택): 기록되는 키에 부여할 key-value 태그.
	// map[string]any 로 들어온 값을 map[string]string 으로 정규화하며,
	// key(태그 이름)는 ^[a-zA-Z0-9_-]+$ 를 만족해야 하고 value 는 문자열이어야 한다.
	// value 는 리터럴 또는 `$.` 경로이며, 해석은 Process 시점에 이뤄진다 (여기서는 raw 문자열로 보관).
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

	// node-level dead-band (legacy 단일 키 / key_mappings 에 적용).
	// min_interval/min_change/min_change_percent 를 공유 파서로 해석한다.
	nodeDB, err := parseDeadband(config, "")
	if err != nil {
		return err
	}
	n.minInterval = nodeDB.minInterval
	n.minChange = nodeDB.minChange
	n.hasMinChange = nodeDB.hasMinChange
	n.minChangePercent = nodeDB.minChangePercent
	n.hasMinChangePercent = nodeDB.hasMinChangePercent

	// 스냅샷 dead-band 우회(선택): snapshot_value_path + snapshot_values.
	// 전체 상태 스냅샷(예: device_state.report)을 모든 메트릭 동일 카운트로 기록하기 위해
	// 해당 메시지는 dead-band 를 우회해 무조건 저장한다. 둘은 함께 지정해야 한다.
	if err := n.configureSnapshotBypass(config); err != nil {
		return err
	}

	// metrics (선택): 다중 메트릭. 각 항목은 metric_type/value_key/data_type 와
	// 자체 dead-band(min_interval/min_change/min_change_percent)를 갖는다.
	// 비어 있지 않으면 key_template 의 키에 대해 metric 별 시리즈를 기록한다.
	if v, ok := config["metrics"]; ok {
		raw, ok := v.([]any)
		if !ok {
			return fmt.Errorf("store-write: metrics must be a list")
		}
		specs := make([]metricSpec, 0, len(raw))
		for i, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("store-write: metrics[%d] must be an object", i)
			}
			spec, err := parseMetricSpec(m, i)
			if err != nil {
				return err
			}
			specs = append(specs, spec)
		}
		n.metrics = specs
	}

	// 검증: 기록할 키가 없으면 거부한다 (fail-fast).
	// 최소 하나의 키 소스(단일 key_template 또는 key_mappings 엔트리)는 반드시 제공되어야 한다.
	if n.keyTemplate == "" && len(n.keyMappings) == 0 {
		return fmt.Errorf("store-write: key_template 또는 key_mappings 중 최소 하나는 지정해야 합니다")
	}
	// metrics 는 key_template 의 키에 기록되므로 key_template 이 필요하다.
	if len(n.metrics) > 0 && n.keyTemplate == "" {
		return fmt.Errorf("store-write: metrics 를 사용하려면 key_template 이 필요합니다")
	}

	return nil
}

// configureSnapshotBypass 는 snapshot_value_path / snapshot_values 설정을 해석한다.
//
// 정책: 둘 다 지정되거나 둘 다 없어야 한다(한쪽만 지정 시 설정 오류).
//   - snapshot_value_path: `$.` 경로 문자열 (예: "$.payload.message_type").
//   - snapshot_values: 비어 있지 않은 문자열 리스트 (예: ["device_state.report"]).
func (n *StoreWriteNode) configureSnapshotBypass(config map[string]any) error {
	pathRaw, hasPath := config["snapshot_value_path"]
	valsRaw, hasVals := config["snapshot_values"]
	if !hasPath && !hasVals {
		return nil
	}
	if hasPath != hasVals {
		return fmt.Errorf(
			"store-write: snapshot_value_path 와 snapshot_values 는 함께 지정해야 합니다")
	}
	path, ok := pathRaw.(string)
	if !ok || !strings.HasPrefix(path, "$.") {
		return fmt.Errorf(
			"store-write: snapshot_value_path 는 `$.` 로 시작하는 문자열이어야 합니다")
	}
	list, ok := valsRaw.([]any)
	if !ok || len(list) == 0 {
		return fmt.Errorf(
			"store-write: snapshot_values 는 비어 있지 않은 리스트여야 합니다")
	}
	values := make(map[string]bool, len(list))
	for i, item := range list {
		s, ok := item.(string)
		if !ok || s == "" {
			return fmt.Errorf(
				"store-write: snapshot_values[%d] 는 비어 있지 않은 문자열이어야 합니다", i)
		}
		values[s] = true
	}
	n.snapshotPath = path
	n.snapshotValues = values
	return nil
}

// isSnapshotMessage 는 이 메시지가 "전체 상태 스냅샷"인지(= dead-band 우회 대상) 판정한다.
// snapshotPath 가 비활성이거나 경로 해석 실패/값 불일치면 false.
func (n *StoreWriteNode) isSnapshotMessage(msg message.Message) bool {
	if n.snapshotPath == "" {
		return false
	}
	v, err := resolveTemplateExpr(n.snapshotPath, msg)
	if err != nil {
		return false
	}
	return n.snapshotValues[metaToString(v)]
}

// parseDeadband 는 config(노드 레벨 또는 metric 항목)에서 dead-band 설정을 해석한다.
// ctx 는 에러 메시지 접두사이다(노드 레벨은 "", metric 은 "metrics[i]").
func parseDeadband(cfg map[string]any, ctx string) (deadbandConf, error) {
	var db deadbandConf
	where := ""
	if ctx != "" {
		where = ctx + " "
	}
	if v, ok := cfg["min_interval"]; ok {
		if s, ok := v.(string); ok && s != "" {
			d, err := time.ParseDuration(s)
			if err != nil {
				return db, fmt.Errorf("store-write: %sinvalid min_interval %q: %w", where, s, err)
			}
			if d < 0 {
				return db, fmt.Errorf("store-write: %smin_interval must be >= 0, got %q", where, s)
			}
			db.minInterval = d
		}
	}
	if v, ok := cfg["min_change"]; ok {
		f, ok := toFloat64(v)
		if !ok {
			return db, fmt.Errorf("store-write: %smin_change must be a number", where)
		}
		if f < 0 {
			return db, fmt.Errorf("store-write: %smin_change must be >= 0", where)
		}
		db.minChange = f
		db.hasMinChange = true
	}
	if v, ok := cfg["min_change_percent"]; ok {
		f, ok := toFloat64(v)
		if !ok {
			return db, fmt.Errorf("store-write: %smin_change_percent must be a number", where)
		}
		if f < 0 {
			return db, fmt.Errorf("store-write: %smin_change_percent must be >= 0", where)
		}
		db.minChangePercent = f
		db.hasMinChangePercent = true
	}
	return db, nil
}

// parseMetricSpec 은 metrics 배열의 한 항목(map)을 metricSpec 으로 해석·검증한다.
func parseMetricSpec(m map[string]any, idx int) (metricSpec, error) {
	spec := metricSpec{valueKey: "$.payload.value"}

	if v, ok := m["metric_type"]; ok {
		s, ok := v.(string)
		if !ok {
			return spec, fmt.Errorf("store-write: metrics[%d].metric_type must be a string", idx)
		}
		if s != "" && !strings.HasPrefix(s, "$.") && !metricTypePattern.MatchString(s) {
			return spec, fmt.Errorf(
				"store-write: metrics[%d] invalid metric_type %q (must match ^[a-zA-Z0-9_-]+$ or be a $.-path)", idx, s)
		}
		spec.metricType = s
	}

	if v, ok := m["value_key"]; ok {
		s, ok := v.(string)
		if !ok {
			return spec, fmt.Errorf("store-write: metrics[%d].value_key must be a string", idx)
		}
		if s != "" {
			if !strings.HasPrefix(s, "$.") {
				return spec, fmt.Errorf(
					"store-write: metrics[%d].value_key %q must be a $.-path (e.g. $.payload.temperature)", idx, s)
			}
			spec.valueKey = s
		}
		// 빈 문자열이면 기본값 "$.payload.value" 유지.
	}

	if v, ok := m["data_type"]; ok {
		s, ok := v.(string)
		if !ok {
			return spec, fmt.Errorf("store-write: metrics[%d].data_type must be a string", idx)
		}
		if s != "" {
			if s != system.DataTypeAuto && !validDataTypes[s] {
				return spec, fmt.Errorf(
					"store-write: metrics[%d] invalid data_type %q (must be one of: int, float, string, boolean, bytes, json, auto)", idx, s)
			}
			spec.dataType = s
		}
	}

	db, err := parseDeadband(m, fmt.Sprintf("metrics[%d]", idx))
	if err != nil {
		return spec, err
	}
	spec.deadband = db

	return spec, nil
}

// legacyDeadband 는 노드 레벨 dead-band 설정을 deadbandConf 로 반환한다(legacy 경로용).
func (n *StoreWriteNode) legacyDeadband() deadbandConf {
	return deadbandConf{
		minInterval:         n.minInterval,
		minChange:           n.minChange,
		hasMinChange:        n.hasMinChange,
		minChangePercent:    n.minChangePercent,
		hasMinChangePercent: n.hasMinChangePercent,
	}
}

// storeTarget 은 한 메시지에서 기록할 단일 기록 단위이다.
// 각 타깃은 자신의 metric_type/data_type/dead-band 를 가진다(다중 메트릭 지원).
type storeTarget struct {
	key        string
	value      any
	metricType string       // 이 기록의 해석된 metric_type ("" = 미지정/unknown)
	dataType   string       // 이 기록의 data_type ("" = 미지정)
	deadband   deadbandConf // 이 기록의 미세변화 억제 설정
}

// Process 는 메시지 데이터를 Store에 기록하고, 원본 메시지를 그대로 반환한다.
//
// 다중 메트릭/다중 키 지원:
//   - metrics 가 설정되어 있으면 key_template 의 키에 각 metric 을 metric_type 별
//     시리즈로 기록한다(각 metric 의 value_key/data_type/dead-band 적용).
//   - metrics 가 비어 있으면 legacy 단일 key_template/value_key 1건을 기록한다.
//   - key_mappings 의 각 엔트리는 자체 키에 추가로 기록된다(노드 레벨 공유 메타 적용).
//
// 태그(tags)는 모든 기록에 동일하게 적용된다.
//
// 에러 정책:
//   - 키/값 해석 실패는 즉시 에러를 반환한다(키는 필수).
//   - 개별 키 쓰기 실패 시 fail-fast 로 첫 실패에서 에러를 반환한다.
func (n *StoreWriteNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	if n.store == nil {
		return nil, ErrStoreNotConfigured
	}

	// 기록할 타깃 목록을 구성한다(타깃별 metric_type/data_type/dead-band 포함).
	targets, err := n.buildTargets(msg)
	if err != nil {
		return nil, err
	}

	// tags 는 메시지 단위로 한 번만 해석한다(모든 기록에 공유 적용).
	resolvedTags := n.resolveTags(msg)

	// 각 타깃을 기록한다. 첫 쓰기 실패에서 즉시 에러를 반환한다(fail-fast).
	//
	// dead-band: 시리즈(key+metric+tags)별로 미세변화면 저장을 생략한다(메시지는 그대로 pass-through).
	// 판정은 직전 *저장값* 기준이며, 실제 저장 성공 후에만 상태를 갱신한다.
	// 전체 상태 스냅샷(예: device_state.report)은 dead-band 를 우회하여 모든 메트릭을
	// 매번 함께 저장한다(메트릭별 카운트 일치). change 스트림 등 그 외 메시지는 기존 dead-band 적용.
	isSnapshot := n.isSnapshotMessage(msg)
	ts := msg.Timestamp()
	for i := range targets {
		t := targets[i]
		sig := seriesSignature(t.key, t.metricType, resolvedTags)
		if !isSnapshot && !n.shouldStore(&t.deadband, sig, t.value, ts) {
			continue
		}
		if err := n.writeOne(ctx, t.key, t.value, t.metricType, t.dataType, resolvedTags); err != nil {
			return nil, err
		}
		// 스냅샷은 무조건 저장하므로 dead-band 상태를 갱신하지 않는다(판정에 사용되지 않음).
		if !isSnapshot {
			n.commitDedup(&t.deadband, sig, t.value, ts)
		}
	}

	// pass-through: 원본 메시지를 그대로 반환
	return []message.Message{msg}, nil
}

// buildTargets 는 한 메시지에서 기록할 모든 타깃을 구성한다.
//
//  1. key_template:
//     - metrics 가 있으면 metric 별로 1건씩(같은 키, 서로 다른 metric_type).
//     - 없으면 legacy 단일 (key_template, value_key) 1건.
//  2. key_mappings 의 각 엔트리: 자체 키에 1건씩(노드 레벨 공유 메타).
//
// 키 또는 값 해석에 실패하면 즉시 에러를 반환한다(키는 필수).
func (n *StoreWriteNode) buildTargets(msg message.Message) ([]storeTarget, error) {
	targets := make([]storeTarget, 0, len(n.metrics)+1+len(n.keyMappings))

	// 1. key_template 기반 기록.
	if n.keyTemplate != "" {
		key, err := resolveKeyTemplate(n.keyTemplate, msg)
		if err != nil {
			return nil, fmt.Errorf("store-write: %w", err)
		}

		if len(n.metrics) > 0 {
			// 다중 메트릭: 각 metric 을 같은 키에 metric_type 별 시리즈로 기록.
			for i := range n.metrics {
				m := n.metrics[i]
				value, err := n.resolveValue(m.valueKey, msg)
				if err != nil {
					return nil, fmt.Errorf("store-write: metrics[%d] value_key %q: %w", i, m.valueKey, err)
				}
				targets = append(targets, storeTarget{
					key:        key,
					value:      value,
					metricType: n.resolveMetricExpr(m.metricType, msg),
					dataType:   m.dataType,
					deadband:   m.deadband,
				})
			}
		} else {
			// legacy 단일 key_template/value_key.
			value, err := n.resolveValue(n.valueKey, msg)
			if err != nil {
				return nil, fmt.Errorf("store-write: value_key %q: %w", n.valueKey, err)
			}
			targets = append(targets, storeTarget{
				key:        key,
				value:      value,
				metricType: n.resolveMetricExpr(n.metricType, msg),
				dataType:   n.dataType,
				deadband:   n.legacyDeadband(),
			})
		}
	}

	// 2. key_mappings 각 엔트리 (노드 레벨 공유 메타).
	for keyTemplate, valuePath := range n.keyMappings {
		key, err := resolveKeyTemplate(keyTemplate, msg)
		if err != nil {
			return nil, fmt.Errorf("store-write: key_mappings key %q: %w", keyTemplate, err)
		}
		value, err := n.resolveValue(valuePath, msg)
		if err != nil {
			return nil, fmt.Errorf("store-write: key_mappings[%q] value %q: %w", keyTemplate, valuePath, err)
		}
		targets = append(targets, storeTarget{
			key:        key,
			value:      value,
			metricType: n.resolveMetricExpr(n.metricType, msg),
			dataType:   n.dataType,
			deadband:   n.legacyDeadband(),
		})
	}

	return targets, nil
}

// resolveValue 는 값 경로를 메시지에서 해석한다 (value_key 와 key_mappings 값에 공통 사용).
//
//	path == ""          → 전체 payload (map) 저장.
//	path == "$.payload.X" → 해당 `$.` 경로 값.
func (n *StoreWriteNode) resolveValue(path string, msg message.Message) (any, error) {
	if path == "" {
		// 빈 경로이면 전체 payload 를 저장.
		return msg.Payload().ToMap(), nil
	}
	return resolveTemplateExpr(path, msg)
}

// writeOne 은 단일 (키,값)을 공유 메타와 함께 Store 에 기록한다.
//
// data_type / metric_type / tags 중 하나라도 설정됐고 store 가 StoreMetaWriter 를 만족하면
// SetWithMeta 를 사용하여 메타데이터를 함께 적용한다. 그 외에는 기존 Set/SetWithTTL 경로로 폴백한다 (하위 호환).
func (n *StoreWriteNode) writeOne(ctx context.Context, key string, value any, metric, dataType string, tags map[string]string) error {
	// useMeta 판정은 **기본값 적용 전**의 설정값으로 한다 (아래 기본값 주석의 (2) 참조).
	useMeta := dataType != "" || metric != "" || len(tags) > 0
	if mw, ok := n.store.(StoreMetaWriter); ok && useMeta {
		// [선행 결정의 역전] data_type 미지정의 기본값을 "auto"(DataTypeAuto) 로 승격한다.
		//
		// WHY: 49884cb8 이 DataTypeAuto sentinel 을 도입할 때 "기존 빈 data_type 의 string
		//   동작은 보존(비회귀)" 을 의도적으로 선택했다. 그러나 그 결과 data_type 을 설정하지
		//   않은 기존 플로우는 store 의 "동적 = string" 정책(agent/system/store.go
		//   checkKeyAllowed)으로 키가 data_type=string 으로 자동 등록되고, stringifyValue 가
		//   쓰기 시점에 값을 문자열로 바꾼다 — float64(29.8) → "29.8". 그러면 store 기반
		//   차트가 전부 빈 화면이 된다(프론트 storeChartValue 와 백엔드 집계 toFloat64 가
		//   문자열 값을 정상적으로 거부하기 때문). 사용자가 모든 노드를 일일이 편집하지 않고도
		//   이 플로우들이 복구되도록 기본값을 auto 로 올린다.
		//
		// IMPACT (회귀가 아니라 기본값 승격이며, 사라진 동작은 아래가 전부이다):
		//   - 숫자/불리언 값은 이제 숫자/불리언 시리즈로 등록·저장된다(이전: 문자열).
		//   - 문자열 값은 auto 추론 결과도 string 이므로 동작이 동일하다(비숫자 시리즈 안전).
		//   - 명시 data_type(6종 enum 및 명시 "auto")의 의미는 전혀 바뀌지 않는다.
		//   - 이미 "동적 string" 으로 등록된 기존 시리즈는 다음 쓰기에서 추론 타입으로
		//     덮어써진다(SetKeyDataType 의 동적 string 갱신 정책). 단 과거 히스토리에 남은
		//     문자열 값은 소급 변환되지 않는다.
		//   - 위험: auto 는 **첫 쓰기 값**으로 타입을 고정하므로, 첫 값이 Go int 인 시리즈는
		//     int 로 고정되고 이후 비정수 float 쓰기가 ErrTypeMismatch 로 거부된다.
		//     (JSON 을 거친 값은 모두 float64 이므로 이 경로에서는 발생하지 않는다.)
		//     store_write_default_datatype_test.go 가 이 동작을 명시적으로 고정한다.
		//
		// WHERE: 파싱 시점(Configure/parseMetricSpec)이 아니라 **쓰기 호출 지점**에 둔다.
		//   (1) Configure 는 "명시적 빈 문자열" 과 "키 자체가 없음" 을 구분하지 않으므로
		//       (둘 다 s != "" 검사에서 걸러진다) 파싱 시점에는 구분할 정보 자체가 없다.
		//   (2) 더 중요하게, 파싱 시점에 n.dataType 을 채우면 위 useMeta 판정이 항상 참이 되어,
		//       메타가 전혀 없던 노드까지 bare key 경로에서 시리즈 인코딩 키
		//       ("unknown||key", EncodeSeriesKey) 경로로 옮겨간다. 이는 저장 키가 바뀌는
		//       변경이라 같은 키를 읽는 store-read 노드와 기존 저장 데이터가 끊긴다.
		//       따라서 기본값은 이미 메타 경로에 들어온 쓰기에만 적용한다.
		//   결과적으로 legacy 단일 키와 metrics[] 양쪽 타깃이 모두 이 한 곳을 지나므로,
		//   실효 data_type 은 여전히 한 지점에서만 결정된다.
		if dataType == "" {
			dataType = system.DataTypeAuto
		}
		opts := system.StoreWriteMeta{
			DataType:   dataType,
			MetricType: metric,
			Tags:       tags,
			TTL:        n.ttl,
		}
		if err := mw.SetWithMeta(ctx, key, value, opts); err != nil {
			return fmt.Errorf("store-write: %w", err)
		}
		return nil
	}
	if n.ttl > 0 {
		if err := n.store.SetWithTTL(ctx, key, value, n.ttl); err != nil {
			return fmt.Errorf("store-write: %w", err)
		}
		return nil
	}
	if err := n.store.Set(ctx, key, value); err != nil {
		return fmt.Errorf("store-write: %w", err)
	}
	return nil
}

// resolveMetricExpr 은 metric_type 표현식(expr)을 메시지 단위로 해석한다.
// legacy 노드 레벨 metric_type 과 metrics[].metric_type 양쪽에서 공통 사용한다.
//
// 동작:
//   - expr 가 비어있으면 "" 반환 (metric_type 미설정 — 어댑터가 기존값/unknown 보존).
//   - `$.` prefix 면 resolveTemplateExpr 로 메시지에서 값을 해석하여 문자열화한다.
//     해석 실패(경로 없음 등)나 해석 결과가 빈 문자열이면 "" 반환(생략).
//   - 그 외는 리터럴 그대로.
//
// 정책: 해석된 최종 값이 metric_type 정규식(^[a-zA-Z0-9_-]+$)을 위반하면 "" 반환(해당 metric 생략).
// 리터럴은 Configure 에서 이미 검증되었으나, `$.` 경로 해석 결과는 여기서 다시 검증한다.
func (n *StoreWriteNode) resolveMetricExpr(expr string, msg message.Message) string {
	if expr == "" {
		return ""
	}

	var resolved string
	if strings.HasPrefix(expr, "$.") {
		v, err := resolveTemplateExpr(expr, msg)
		if err != nil {
			// 해석 실패 → metric_type 생략 (해당 항목만 건너뛰고 계속).
			return ""
		}
		resolved = metaToString(v)
	} else {
		resolved = expr
	}

	if resolved == "" || !metricTypePattern.MatchString(resolved) {
		// 빈 값 또는 정규식 위반 → 생략.
		return ""
	}
	return resolved
}

// resolveTags 는 config 의 tags(값이 리터럴 또는 `$.` 경로)를 메시지 단위로 해석한다.
//
// 각 값에 대해:
//   - `$.` prefix 면 resolveTemplateExpr 로 메시지에서 해석하여 문자열화한다.
//     해석 실패(경로 없음 등)면 해당 태그만 생략하고 계속한다.
//   - 그 외는 리터럴 그대로.
//
// 키(태그 이름)는 항상 리터럴이며 Configure 에서 이미 검증되었다.
// 해석 결과가 빈 맵이면 nil 을 반환하여 어댑터가 tags 미설정으로 처리하게 한다.
func (n *StoreWriteNode) resolveTags(msg message.Message) map[string]string {
	if len(n.tags) == 0 {
		return nil
	}
	out := make(map[string]string, len(n.tags))
	for tk, tv := range n.tags {
		if strings.HasPrefix(tv, "$.") {
			v, err := resolveTemplateExpr(tv, msg)
			if err != nil {
				// 해석 실패 → 해당 태그만 생략하고 계속.
				continue
			}
			out[tk] = metaToString(v)
			continue
		}
		// 리터럴 값은 그대로.
		out[tk] = tv
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// metaToString 은 `$.` 경로 해석 결과를 metric_type / tag 값으로 쓰기 위해 문자열화한다.
// influxdb-write 가 tag 값에 쓰는 influxToString 과 동일한 규약(오브젝트는 JSON, 스칼라는 fmt,
// nil 은 빈 문자열)을 사용하여 일관성을 유지한다. 숫자/불리언도 문자열로 변환된다.
func metaToString(v any) string {
	return influxToString(v)
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

// shouldStore 는 dead-band 규칙에 따라 이 (시리즈, 값)을 지금 저장해야 하는지 판정한다.
// 상태를 변경하지 않는다(읽기 전용). 저장이 확정되면 호출 측이 commitDedup 으로 갱신한다.
//
//	minInterval == 0           → 기능 OFF, 항상 저장.
//	직전 저장 기록 없음          → 최초 저장.
//	경과시간 >= minInterval     → heartbeat 저장(변화 없어도).
//	경과시간 <  minInterval     → 유의미한 변화일 때만 저장(아니면 생략).
func (n *StoreWriteNode) shouldStore(db *deadbandConf, sig string, value any, ts time.Time) bool {
	if db.minInterval <= 0 {
		return true
	}
	n.dedupMu.Lock()
	prev, ok := n.lastStored[sig]
	n.dedupMu.Unlock()
	if !ok {
		return true
	}
	if ts.Sub(prev.at) >= db.minInterval {
		return true
	}
	return db.isSignificantChange(prev.value, value)
}

// commitDedup 은 저장이 성공한 뒤 시리즈의 마지막 저장 상태를 갱신한다.
// dead-band 가 비활성(minInterval<=0)이면 추적할 필요가 없으므로 아무것도 하지 않는다.
func (n *StoreWriteNode) commitDedup(db *deadbandConf, sig string, value any, ts time.Time) {
	if db.minInterval <= 0 {
		return
	}
	n.dedupMu.Lock()
	n.lastStored[sig] = dedupState{value: value, at: ts}
	n.dedupMu.Unlock()
}

// isSignificantChange 는 직전 저장값(prev) 대비 현재값(cur)이 유의미한 변화인지 판정한다.
//
// 숫자 ↔ 숫자:
//   - min_change(절대) 또는 min_change_percent(상대) 중 하나라도 임계 초과 → 유의미.
//   - 둘 다 설정됐는데 모두 임계 이하 → 미세변화(false).
//   - 둘 다 미설정 → 값이 다르면 유의미(순수 시간 기반 중복 제거).
//
// 그 외(비숫자 또는 한쪽만 숫자):
//   - 직전 저장값과 다르면 유의미, 같으면 미세변화로 간주.
func (db *deadbandConf) isSignificantChange(prev, cur any) bool {
	pf, pok := toFloat64(prev)
	cf, cok := toFloat64(cur)
	if pok && cok {
		delta := cf - pf
		if delta < 0 {
			delta = -delta
		}
		if db.hasMinChange && delta > db.minChange {
			return true
		}
		if db.hasMinChangePercent {
			if pf == 0 {
				if cf != 0 {
					return true
				}
			} else {
				denom := pf
				if denom < 0 {
					denom = -denom
				}
				if delta/denom*100 > db.minChangePercent {
					return true
				}
			}
		}
		if db.hasMinChange || db.hasMinChangePercent {
			// 설정된 임계값을 모두 통과(이하)했으므로 미세변화.
			return false
		}
		// 임계값 미설정: 값이 다르면 유의미.
		return delta != 0
	}
	// 비숫자: 직전 저장값과 다르면 저장, 같으면 억제.
	return !reflect.DeepEqual(prev, cur)
}

// seriesSignature 는 dead-band 상태를 분리하기 위한 시리즈 식별자를 만든다.
// 저장소의 분류 기준(SeriesID = key + metric_type + sorted(tags))과 동일한 조합을 사용하여,
// 같은 키라도 metric_type/tags 가 다르면 독립적으로 추적된다.
// 태그는 키 기준 정렬하여 맵 순서와 무관하게 동일 시그니처를 보장한다.
func seriesSignature(key, metric string, tags map[string]string) string {
	var b strings.Builder
	b.WriteString(key)
	b.WriteByte(0)
	b.WriteString(metric)
	if len(tags) > 0 {
		keys := make([]string, 0, len(tags))
		for k := range tags {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteByte(0)
			b.WriteString(k)
			b.WriteByte('=')
			b.WriteString(tags[k])
		}
	}
	return b.String()
}
