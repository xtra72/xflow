package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// @spec SPEC-STORE-003
// tagKeyPattern 은 태그 key 로 허용되는 문자 패턴이다.
// URL 쿼리 파싱 및 식별자 안전성을 위해 영문/숫자/밑줄/하이픈만 허용한다.
var tagKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// nodeStoreProvider 는 노드에서 Store 인스턴스에 접근하기 위한 인터페이스이다.
// UserStoreAgent 가 이 인터페이스를 구현하며,
// store-write, store-read 노드가 AgentResolver → AgentAccessor → nodeStoreProvider
// 경로로 Store에 접근한다.
// 순환 의존을 방지하기 위해 any를 반환하고, 노드 측에서 StoreWriter/StoreReader로 타입 단언한다.
type nodeStoreProvider interface {
	NodeStoreForNamespace(namespace string) any
}

// UserStoreAgent 는 사용자가 YAML로 등록하는 Store 시스템 에이전트이다.
// 내부적으로 StoreAgent를 감싸며, agent.Agent 인터페이스를 구현하여
// agent.Manager에 등록될 수 있다.
//
// store-write, store-read 노드가 AgentResolver를 통해 이 에이전트를 찾아
// StoreForNamespace() 메서드로 네임스페이스별 Store 인스턴스에 접근한다.
type UserStoreAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig agent.AgentConfig
	inner       *StoreAgent
	logger      *slog.Logger
	stats       *agent.AgentStats
	mu          sync.RWMutex
	startedAt   time.Time
	createdAt   time.Time
}

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*UserStoreAgent)(nil)
var _ agent.StatefulAgent = (*UserStoreAgent)(nil)
var _ nodeStoreProvider = (*UserStoreAgent)(nil)

// parseStoreConfig 는 AgentConfig.Transport.Options에서 StoreOption 목록을 파싱한다.
//
// @spec SPEC-STORE-003 v0.3.0: 반환 옵션에는 registration_type 과 keys (정적 키 + DataType +
// MetricType + Tags) 가 포함된다. 해당 필드가 없으면 기본값(RegistrationAuto, staticKeys=nil)이
// 유지된다. 파싱 실패(중복 키, 태그 key 형식 위반, data_type/metric_type/registration_type
// enum 위반, 타입 오류) 시 에러를 반환한다.
//
// v0.2.0 의 `allow_dynamic_keys` 필드는 제거되었으며 (clean rename, no shim),
// 잔존 시 명시적 마이그레이션 에러로 부팅이 거부된다.
func parseStoreConfig(cfg agent.AgentConfig) ([]StoreOption, error) {
	var opts []StoreOption

	options := cfg.Transport.Options
	if options == nil {
		return opts, nil
	}

	// @spec SPEC-STORE-003 v0.3.0
	// v0.2.0 의 'allow_dynamic_keys' 필드가 잔존하면 명시적 마이그레이션 에러로 부팅을 거부한다.
	// 조용한 호환 shim 대신 사용자가 yaml 을 명시적으로 갱신하도록 강제한다.
	if _, hasOld := options["allow_dynamic_keys"]; hasOld {
		return nil, fmt.Errorf(
			"store config: 'allow_dynamic_keys' is removed in v0.3.0; " +
				"use 'registration_type: manual|auto' instead",
		)
	}

	if v, ok := options["backend"].(string); ok && v != "" {
		opts = append(opts, WithBackend(v))
	}

	if v, ok := options["default_ttl"].(string); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			opts = append(opts, WithDefaultTTL(d))
		}
	}

	if v, ok := options["scan_interval"].(string); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			opts = append(opts, WithScanInterval(d))
		}
	}

	if v, ok := options["max_key_length"]; ok {
		opts = append(opts, WithMaxKeyLength(toInt(v)))
	}

	if v, ok := options["max_history_size"]; ok {
		opts = append(opts, WithMaxHistorySize(toInt(v)))
	}

	if v, ok := options["history_ttl"].(string); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			opts = append(opts, WithHistoryTTL(d))
		}
	}

	// key_tag (string, 선택) — 자동 요소 생성 시 키로 사용할 태그 이름.
	// 예: "name" → 쓰기 태그에 name 이 있으면 그 값을 키로, 없으면 기존 키(생성된 id).
	// 태그 key 형식(^[a-zA-Z0-9_-]+$)을 따른다.
	if raw, ok := options["key_tag"]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("store config: key_tag must be string, got %T", raw)
		}
		if s != "" && !tagKeyPattern.MatchString(s) {
			return nil, fmt.Errorf(
				"store config: key_tag %q must match pattern ^[a-zA-Z0-9_-]+$", s,
			)
		}
		opts = append(opts, WithKeyTag(s))
	}

	// @spec SPEC-STORE-003 v0.3.0
	// registration_type (string, default "auto") — "manual" 또는 "auto" enum.
	// manual 모드는 staticKeys 의 모든 엔트리에 data_type 명시를 요구한다 (parseStaticKeysRaw 에서 강제).
	registrationType := RegistrationAuto
	if raw, ok := options["registration_type"]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("store config: registration_type must be string, got %T", raw)
		}
		if err := validateRegistrationType(s); err != nil {
			return nil, fmt.Errorf(
				"store config: registration_type %q is invalid (must be 'manual' or 'auto')", s,
			)
		}
		registrationType = RegistrationType(s)
	}
	opts = append(opts, WithRegistrationType(registrationType))

	// @spec SPEC-STORE-003 v0.3.0
	// keys ([]map) 정적 키 목록 + 메타데이터 (data_type, metric_type, tags).
	// manual 모드에서는 각 엔트리의 data_type 명시가 필수이다.
	if raw, ok := options["keys"]; ok {
		staticKeys, err := parseStaticKeysRaw(raw, registrationType)
		if err != nil {
			return nil, err
		}
		opts = append(opts, WithStaticKeys(staticKeys))
	}

	return opts, nil
}

// @spec SPEC-STORE-003 v0.3.0
// parseStaticKeysRaw 는 options["keys"] 값을 정적 키 → StaticKeyMeta 맵으로 변환한다.
//
// 입력 형식 (v0.3.0): []any 에 담긴 map[string]any. 각 엔트리는
//
//	{
//	  "key":         string (required, non-empty, unique),
//	  "data_type":   string (manual 모드 필수, auto 모드 optional; 6종 enum),
//	  "metric_type": string (optional; ^[a-zA-Z0-9_-]+$, default "unknown"),
//	  "tags":        map[string]any (optional; tag key ^[a-zA-Z0-9_-]+$, value string),
//	}.
//
// registrationType 매개변수는 manual 모드에서 data_type 누락을 ErrInvalidDataType
// 으로 거부하기 위해 필요하다.
//
// 검증 규칙:
//   - 중복 key → ErrDuplicateStaticKey
//   - manual 모드에서 data_type 누락 → ErrInvalidDataType
//   - data_type enum 위반 → ErrInvalidDataType
//   - metric_type 정규식 위반 → ErrInvalidMetricType
//   - 태그 key 가 tagKeyPattern 위반 → ErrInvalidTagKey
//   - 태그 value 가 string 이 아니면 → 명시적 에러
//
// 모든 yaml 정의 키의 Source 는 SourceManual 로 설정된다 (auto 등록은 런타임에 추가).
//
// 빈 배열([]) 이면 빈 맵을 반환한다. nil 이면 nil 을 반환한다.
func parseStaticKeysRaw(raw any, registrationType RegistrationType) (map[string]StaticKeyMeta, error) {
	if raw == nil {
		return nil, nil
	}

	// YAML/JSON 디코딩 결과는 대개 []any 이지만, 이미 변환된 []map 도 받아들인다.
	var list []any
	switch v := raw.(type) {
	case []any:
		list = v
	case []map[string]any:
		list = make([]any, 0, len(v))
		for _, m := range v {
			list = append(list, m)
		}
	default:
		return nil, fmt.Errorf("store config: keys must be a list, got %T", raw)
	}

	result := make(map[string]StaticKeyMeta, len(list))
	for i, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("store config: keys[%d] must be a map, got %T", i, item)
		}

		// --- key (required, non-empty, unique) ---
		keyRaw, exists := entry["key"]
		if !exists {
			return nil, fmt.Errorf("store config: keys[%d] missing required field 'key'", i)
		}
		key, ok := keyRaw.(string)
		if !ok {
			return nil, fmt.Errorf("store config: keys[%d].key must be string, got %T", i, keyRaw)
		}
		if key == "" {
			return nil, fmt.Errorf("store config: keys[%d].key must be non-empty", i)
		}
		if _, dup := result[key]; dup {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateStaticKey, key)
		}

		// --- data_type (manual 모드 필수, auto 모드 optional) ---
		var dataType DataType
		dtRaw, hasDT := entry["data_type"]
		if hasDT {
			dtStr, ok := dtRaw.(string)
			if !ok {
				return nil, fmt.Errorf(
					"store config: keys[%q].data_type must be string, got %T", key, dtRaw,
				)
			}
			if err := validateDataTypeEnum(dtStr); err != nil {
				return nil, fmt.Errorf("%w: key=%q data_type=%q", ErrInvalidDataType, key, dtStr)
			}
			dataType = DataType(dtStr)
		} else if registrationType == RegistrationManual {
			// manual 모드에서는 data_type 명시 필수.
			return nil, fmt.Errorf(
				"%w: key=%q (data_type is required in manual registration mode)",
				ErrInvalidDataType, key,
			)
		}
		// auto 모드 + data_type 미명시: 빈 DataType 으로 두고 첫 쓰기 시 inferDataType 으로 결정 (Phase C).

		// --- metric_type (optional, default "unknown") ---
		var metricType string
		if mtRaw, hasMT := entry["metric_type"]; hasMT {
			mtStr, ok := mtRaw.(string)
			if !ok {
				return nil, fmt.Errorf(
					"store config: keys[%q].metric_type must be string, got %T", key, mtRaw,
				)
			}
			normalized, err := validateMetricType(mtStr)
			if err != nil {
				return nil, fmt.Errorf("%w: key=%q metric_type=%q", ErrInvalidMetricType, key, mtStr)
			}
			metricType = normalized
		} else {
			// 미지정 시 default "unknown" (validateMetricType("") 와 동일 결과).
			metricType = "unknown"
		}

		// --- tags (optional; key 정규식 + value string 검증) ---
		tags, err := parseTagsRaw(entry["tags"], key)
		if err != nil {
			return nil, err
		}

		result[key] = StaticKeyMeta{
			DataType:   dataType,
			MetricType: metricType,
			Tags:       tags,
			Source:     SourceManual, // yaml 정의 키는 모두 manual 등록.
		}
	}
	return result, nil
}

// @spec SPEC-STORE-003
// parseTagsRaw 는 단일 엔트리의 tags 필드를 map[string]string 으로 변환한다.
// tags 가 nil 이거나 생략되면 빈 맵을 반환한다.
// 태그 key 는 tagKeyPattern 을 만족해야 하며, value 는 반드시 string 이어야 한다.
func parseTagsRaw(raw any, ownerKey string) (map[string]string, error) {
	if raw == nil {
		return map[string]string{}, nil
	}

	var src map[string]any
	switch v := raw.(type) {
	case map[string]any:
		src = v
	case map[string]string:
		// 이미 강타입인 경우: 태그 key 만 검증한다.
		out := make(map[string]string, len(v))
		for tk, tv := range v {
			if !tagKeyPattern.MatchString(tk) {
				return nil, fmt.Errorf("%w: key=%q tag=%q", ErrInvalidTagKey, ownerKey, tk)
			}
			out[tk] = tv
		}
		return out, nil
	default:
		return nil, fmt.Errorf("store config: keys[%q].tags must be a map, got %T", ownerKey, raw)
	}

	out := make(map[string]string, len(src))
	for tk, tvRaw := range src {
		if !tagKeyPattern.MatchString(tk) {
			return nil, fmt.Errorf("%w: key=%q tag=%q", ErrInvalidTagKey, ownerKey, tk)
		}
		tv, ok := tvRaw.(string)
		if !ok {
			return nil, fmt.Errorf(
				"store config: keys[%q].tags[%q] must be string, got %T",
				ownerKey, tk, tvRaw,
			)
		}
		out[tk] = tv
	}
	return out, nil
}

// NewUserStoreAgent 는 UserStoreAgent 팩토리 함수이다.
func NewUserStoreAgent(config agent.AgentConfig) (agent.Agent, error) {
	storeOpts, err := parseStoreConfig(config)
	if err != nil {
		return nil, fmt.Errorf("store config: %w", err)
	}

	a := &UserStoreAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("store")),
		agentConfig:   config,
		inner:         NewStoreAgent(storeOpts...),
		logger:        agent.ResolveLogger(config),
		stats:         agent.NewAgentStats(),
		createdAt:     time.Now(),
	}

	if err := a.Init(config); err != nil {
		return nil, err
	}

	return a, nil
}

// NodeStoreForNamespace 는 지정된 네임스페이스의 NodeStoreAdapter를 반환한다.
// store-write, store-read 노드가 이 메서드를 통해 Store에 접근한다.
// 반환된 *NodeStoreAdapter는 node.StoreWriter + node.StoreReader를 모두 만족한다.
// 순환 의존을 방지하기 위해 any를 반환한다.
//
// 반환된 adapter 는 내부 resolver 를 통해 호출 시점의 a.inner 를 참조하므로,
// 에이전트 재시작(Stop → Start) 으로 inner 가 새 StoreAgent 로 교체되어도
// 플로우 노드가 기존에 들고 있는 adapter 가 자동으로 새 inner 의 네임스페이스
// 뷰로 라우팅된다. 생성 시점 스냅샷을 잡으면 재시작 후 옛 inner 로 향해
// 모든 쓰기/읽기가 실패하는 회귀가 발생한다.
func (a *UserStoreAgent) NodeStoreForNamespace(namespace string) any {
	// 빈 네임스페이스는 읽기 경로(QueryHistory/ListStoreKeys 등)와 동일하게
	// defaultStoreNamespace 로 정규화한다. 이렇게 하지 않으면 store-write 노드가
	// namespace="" 로 쓴 값(":key")을 API 쿼리(""→"default" 기본값, "default:key")가
	// 찾지 못해, 등록된 키인데도 "store: key not found" 가 발생한다.
	if namespace == "" {
		namespace = defaultStoreNamespace
	}
	// store resolver: 매 호출마다 현재 inner 의 네임스페이스 뷰를 반환.
	storeResolver := func() Store {
		a.mu.RLock()
		defer a.mu.RUnlock()
		return a.inner.ForNamespace(namespace)
	}
	// agent resolver: data_type/tags 메타 설정에 필요한 현재 *StoreAgent(inner) 를 반환.
	// store-write 노드의 SetWithMeta 경로에서 사용된다. 에이전트 재시작에 안전하도록
	// 호출 시점의 inner 를 lazy 하게 돌려준다.
	agentResolver := func() *StoreAgent {
		a.mu.RLock()
		defer a.mu.RUnlock()
		return a.inner
	}
	return NewLazyNodeStoreAdapterWithAgent(storeResolver, agentResolver, namespace)
}

// Init 은 에이전트를 초기화하고 내부 StoreAgent를 시작한다.
func (a *UserStoreAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("store init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("store init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	// 내부 StoreAgent 초기화
	ctx := context.Background()
	if err := a.inner.Init(ctx); err != nil {
		return fmt.Errorf("store init: inner store: %w", err)
	}

	a.logger.Info("store 에이전트 초기화 완료")

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("store init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	return nil
}

// Start 는 에이전트를 시작한다. 이미 Running이면 no-op.
func (a *UserStoreAgent) Start(_ context.Context) error {
	switch a.CurrentState() {
	case lifecycle.StateRunning:
		return nil
	case lifecycle.StateStopped:
		if err := a.TransitionTo(lifecycle.StateCreated); err != nil {
			return fmt.Errorf("store start: reset to created: %w", err)
		}
		a.mu.RLock()
		cfg := a.agentConfig
		a.mu.RUnlock()

		// 내부 StoreAgent 재생성
		opts, err := parseStoreConfig(cfg)
		if err != nil {
			return fmt.Errorf("store start: %w", err)
		}
		a.mu.Lock()
		a.inner = NewStoreAgent(opts...)
		a.mu.Unlock()

		return a.Init(cfg)
	default:
		return fmt.Errorf("store: not in running state (current: %s)", a.CurrentState())
	}
}

// Stop 은 에이전트를 정지하고 내부 StoreAgent를 닫는다.
func (a *UserStoreAgent) Stop(ctx context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("store stop: %w", err)
	}

	a.mu.Lock()
	if a.inner != nil {
		_ = a.inner.Stop(ctx)
	}
	a.mu.Unlock()

	a.logger.Info("store 에이전트 정지 완료")

	return a.TransitionTo(lifecycle.StateStopped)
}

// Pause 는 에이전트를 일시정지한다.
func (a *UserStoreAgent) Pause(ctx context.Context) error {
	if err := a.inner.Pause(ctx); err != nil {
		return fmt.Errorf("store pause: %w", err)
	}
	return a.TransitionTo(lifecycle.StatePaused)
}

// Resume 은 에이전트를 재개한다.
func (a *UserStoreAgent) Resume(ctx context.Context) error {
	if err := a.inner.Resume(ctx); err != nil {
		return fmt.Errorf("store resume: %w", err)
	}
	return a.TransitionTo(lifecycle.StateRunning)
}

// Health 는 에이전트의 헬스 상태를 반환한다.
func (a *UserStoreAgent) Health() agent.HealthStatus {
	state := a.CurrentState()
	if state == lifecycle.StateRunning || state == lifecycle.StatePaused {
		return agent.HealthStatus{Status: agent.HealthHealthy}
	}
	return agent.HealthStatus{Status: agent.HealthUnhealthy}
}

// Process 는 Store 에이전트의 명령을 처리한다.
// 웹 UI에서 exec API를 통해 get_history 등의 명령을 실행할 수 있다.
func (a *UserStoreAgent) Process(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var req struct {
		Command string         `json:"command"`
		Params  map[string]any `json:"params"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("store process: invalid request: %w", err)
	}

	switch req.Command {
	case "get_history":
		return a.processGetHistory(req.Params)
	default:
		return nil, fmt.Errorf("store process: unknown command: %s", req.Command)
	}
}

// processGetHistory 는 지정된 키의 히스토리를 조회하여 JSON으로 반환한다.
func (a *UserStoreAgent) processGetHistory(params map[string]any) ([]byte, error) {
	key, _ := params["key"].(string)
	if key == "" {
		return nil, fmt.Errorf("store get_history: key is required")
	}

	// 네임스페이스 라운드트립 버그 수정 (v0.7.0 M14):
	// 빈 네임스페이스("")로 저장된 키를 조회하려면 "default"로 강제하면 안 됨.
	// 웹 UI에서 전송한 namespace("")를 그대로 사용하여 저장된 실제 네임스페이스와 일치시킴.
	namespace, _ := params["namespace"].(string)
	// 빈 네임스페이스도 그대로 사용하고, 강제하지 않음
	// (이전: if namespace == "" { namespace = "default" })

	a.mu.RLock()
	inner := a.inner
	a.mu.RUnlock()

	if inner == nil {
		return nil, fmt.Errorf("store not initialized")
	}

	store := inner.ForNamespace(namespace)
	entries, err := store.GetHistory(context.Background(), key)
	if err != nil {
		// 키가 없거나 만료된 경우(ErrKeyNotFound)는 에러가 아니라 빈 이력으로
		// 응답한다. 목록 조회와 행 클릭 사이에 TTL 만료로 키가 사라질 수 있으며,
		// 이는 500 INTERNAL_ERROR 가 아니라 graceful 한 빈 결과여야 한다.
		if errors.Is(err, ErrKeyNotFound) {
			return json.Marshal(map[string]any{
				"key":       key,
				"namespace": namespace,
				"count":     0,
				"history":   []map[string]any{},
				"found":     false,
			})
		}
		return nil, fmt.Errorf("store get_history: %w", err)
	}

	history := make([]map[string]any, len(entries))
	for i, e := range entries {
		history[i] = map[string]any{
			"value":     e.Value,
			"timestamp": e.Timestamp.Format(time.RFC3339Nano),
		}
	}

	result := map[string]any{
		"key":       key,
		"namespace": namespace,
		"count":     len(history),
		"history":   history,
	}

	return json.Marshal(result)
}

// Configure 는 에이전트 설정을 변경한다.
//
// @spec SPEC-STORE-003 v0.3.0
// 정책 필드(registration_type, keys)는 런타임에 inner StoreAgent 로 즉시 전파되어
// 재시작 없이 반영된다. 운영 필드(scan_interval, default_ttl, max_history_size,
// max_key_length, history_ttl)는 백그라운드 goroutine 및 저장된 히스토리의 안전성을
// 보장하기 위해 재시작 시에만 반영된다(a.agentConfig 에 저장만 됨).
//
// 새 설정 파싱에 실패하면 inner 상태를 변경하지 않고 에러를 반환한다(검증-우선).
func (a *UserStoreAgent) Configure(config agent.AgentConfig) error {
	// 1) 새 설정을 먼저 파싱·검증한다. 실패 시 inner 변경 없이 에러 반환.
	newOpts, err := parseStoreConfig(config)
	if err != nil {
		return fmt.Errorf("store configure: parse: %w", err)
	}

	// 2) 모든 필드 추출: 옵션을 기본 storeConfig 에 적용하여 최종값을 계산한다.
	//    (parseStoreConfig 는 옵션 함수들을 반환하므로, 직접 storeConfig 에 적용해야
	//     실제 적용 결과를 얻을 수 있다.)
	allCfg := defaultConfig()
	for _, opt := range newOpts {
		opt(&allCfg)
	}
	newRegistrationType := allCfg.registrationType
	newStaticKeys := allCfg.staticKeys
	newMaxHistorySize := allCfg.maxHistorySize
	newHistoryTTL := allCfg.historyTTL
	newScanInterval := allCfg.scanInterval
	newDefaultTTL := allCfg.defaultTTL
	newMaxKeyLength := allCfg.maxKeyLength
	newKeyTag := allCfg.keyTag

	// 3) a.agentConfig 갱신 및 inner 스냅샷을 락 안에서, inner 에의 setter 호출은
	//    락 밖에서 수행한다(이중 락 교착 회피: a.mu 와 inner.mu 는 서로 독립적).
	a.mu.Lock()
	a.agentConfig = config
	inner := a.inner
	a.mu.Unlock()

	// 4) inner 가 아직 없으면(초기화 전) Init/Start 경로에서 반영되므로 skip.
	if inner != nil {
		// @spec SPEC-STORE-003 v0.3.0 (Phase B 회귀 수정)
		// v0.2.0 운영 필드 런타임 전파 (Phase B 에서 누락되었던 부분)
		inner.SetMaxHistorySize(newMaxHistorySize)
		inner.SetHistoryTTL(newHistoryTTL)
		inner.SetScanInterval(newScanInterval)
		inner.SetDefaultTTL(newDefaultTTL)
		inner.SetMaxKeyLength(newMaxKeyLength)

		// v0.3.0 신규 필드
		inner.SetRegistrationType(newRegistrationType)
		inner.SetStaticKeys(newStaticKeys)

		// key_tag 런타임 전파.
		inner.SetKeyTag(newKeyTag)
	}

	return nil
}

// ID 는 에이전트의 고유 식별자를 반환한다.
func (a *UserStoreAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트의 이름을 반환한다.
func (a *UserStoreAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트의 타입을 반환한다.
func (a *UserStoreAgent) Type() string {
	return "store"
}

// Info 는 에이전트의 상세 정보를 반환한다.
func (a *UserStoreAgent) Info() agent.AgentInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var uptime time.Duration
	if !a.startedAt.IsZero() {
		uptime = time.Since(a.startedAt)
	}

	return agent.AgentInfo{
		ID:     a.agentConfig.ID,
		Name:   a.agentConfig.Name,
		Type:   "store",
		State:  a.CurrentState(),
		Config: a.agentConfig,
		Stats:  a.stats.Snapshot(),
		Uptime: uptime,
	}
}

// Stats 는 에이전트의 처리 통계를 반환한다.
func (a *UserStoreAgent) Stats() agent.StatsSnapshot {
	return a.stats.Snapshot()
}

// State 는 Store의 런타임 상태를 반환한다 (StatefulAgent 구현).
// 모든 저장소 항목을 메타데이터와 함께 반환하여 웹 UI 키/값 테이블에 사용한다.
func (a *UserStoreAgent) State() map[string]any {
	a.mu.RLock()
	inner := a.inner
	a.mu.RUnlock()

	if inner == nil {
		return map[string]any{"status": "closed"}
	}

	const maxEntries = 1000
	now := time.Now()

	// inner.mu를 잡고 순회하여 스레드 안전성을 보장한다.
	inner.mu.RLock()
	var entries []map[string]any
	inner.store.data.Range(func(key, value any) bool {
		item, ok := value.(*storeItem)
		if !ok {
			return true
		}
		// 만료된 항목은 건너뛴다.
		if !item.expiresAt.IsZero() && item.expiresAt.Before(now) {
			return true
		}

		// 내부 키에서 네임스페이스 접두사를 분리한다.
		// VolatileStore에는 "sensors:factory-A/line-3:device_id" 형태로 저장되지만,
		// 사용자에게는 key_template 기준의 키("factory-A/line-3:device_id")만 표시한다.
		// prefixKey 는 ns="" 라도 ":"+key 로 저장하므로, ns+":" 접두사 제거는 ns=""
		// (= ":") 경우에도 항상 적용해야 displayKey 가 라운드트립된다.
		// (버그: 이전엔 ns!="" 일 때만 strip 해 ns="" 키가 ":09a..." 로 표시됐다.)
		rawKey, _ := key.(string)
		ns := item.namespace
		// 네임스페이스 접두사를 제거한 저장 키(= 시리즈 인코딩 키, 또는 레거시 bare 키).
		// @spec SPEC-STORE-004: 레지스트리/저장 키는 EncodeSeriesKey("metric|tags|key") 로
		// 키잉되므로, 메타 조회는 이 인코딩 키로 하고, 표시용 key 는 디코드된 사용자 key 로 한다.
		encodedKey := strings.TrimPrefix(rawKey, ns+":")
		userKey := decodeStorageKeyToSeries(encodedKey).Key

		entry := map[string]any{
			"key": userKey,
			// @spec SPEC-STORE-004: storage_key 는 인코딩 시리즈 키(저장/레지스트리 키)이다.
			// 표시·정렬·필터는 디코드된 key 를 쓰지만, 직접 저장 키로 동작하는 행 작업
			// (get_history, 메타 편집/승격 등)은 이 storage_key 를 사용해야 한다. 디코드된
			// key 로 조회하면 인코딩 키로 저장된 값을 못 찾는다(예: 히스토리 0).
			"storage_key":   encodedKey,
			"value":         item.value,
			"namespace":     ns,
			"created_at":    item.createdAt.Format(time.RFC3339),
			"updated_at":    item.updatedAt.Format(time.RFC3339),
			"history_count": len(item.history),
		}

		// @spec SPEC-STORE-003 v0.4.0: 모든 엔트리(정적 + 동적)에 metric_type 과 tags 를 노출한다.
		// @spec SPEC-STORE-004: 메타 조회는 인코딩 시리즈 키(encodedKey)로 한다(staticKeys 는
		// 인코딩 키로 키잉됨). 표시용 key 는 위에서 디코드한 사용자 key(userKey)이므로,
		// 같은 key 의 서로 다른 metric/tags 시리즈는 각각의 행으로 metric_type/tags 가 노출된다.
		metricType := MetricTypeUnknown
		tagsCopy := map[string]string{}
		if meta, ok := inner.config.staticKeys[encodedKey]; ok {
			if meta.MetricType != "" {
				metricType = meta.MetricType
			}
			for tk, tv := range meta.Tags {
				tagsCopy[tk] = tv
			}
		}
		entry["metric_type"] = metricType
		entry["tags"] = tagsCopy

		if item.expiresAt.IsZero() {
			entry["expires_at"] = ""
			entry["ttl"] = ""
		} else {
			entry["expires_at"] = item.expiresAt.Format(time.RFC3339)
			remaining := item.expiresAt.Sub(now).Truncate(time.Second)
			entry["ttl"] = remaining.String()
		}

		entries = append(entries, entry)
		return len(entries) < maxEntries
	})
	inner.mu.RUnlock()

	// 키 기준으로 정렬하여 일관된 표시를 보장한다.
	sort.Slice(entries, func(i, j int) bool {
		ki, _ := entries[i]["key"].(string)
		kj, _ := entries[j]["key"].(string)
		return ki < kj
	})

	// 전체 히스토리 항목 수 집계
	totalHistoryEntries := 0
	for _, e := range entries {
		if hc, ok := e["history_count"].(int); ok {
			totalHistoryEntries += hc
		}
	}

	return map[string]any{
		"status":                "running",
		"inner_state":           inner.CurrentState().String(),
		"total_keys":            len(entries),
		"total_history_entries": totalHistoryEntries,
		"max_history_size":      inner.store.maxHistorySize,
		"entries":               entries,
	}
}
