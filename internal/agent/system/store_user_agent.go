package system

import (
	"context"
	"encoding/json"
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
// @spec SPEC-STORE-003: 반환 옵션에는 allow_dynamic_keys 와 keys (정적 키+태그) 가 포함된다.
// 해당 필드가 없으면 기본값(allowDynamicKeys=true, staticKeys=nil)이 유지되어 하위호환된다.
// 파싱 실패(중복 키, 태그 key 형식 위반, 타입 오류)시 에러를 반환한다.
func parseStoreConfig(cfg agent.AgentConfig) ([]StoreOption, error) {
	var opts []StoreOption

	options := cfg.Transport.Options
	if options == nil {
		return opts, nil
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

	// @spec SPEC-STORE-003: allow_dynamic_keys (bool, 기본 true)
	if raw, ok := options["allow_dynamic_keys"]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("store config: allow_dynamic_keys must be bool, got %T", raw)
		}
		opts = append(opts, WithAllowDynamicKeys(b))
	}

	// @spec SPEC-STORE-003: keys ([]map) 정적 키 목록 + 태그 메타데이터
	if raw, ok := options["keys"]; ok {
		staticKeys, err := parseStaticKeysRaw(raw)
		if err != nil {
			return nil, err
		}
		opts = append(opts, WithStaticKeys(staticKeys))
	}

	return opts, nil
}

// @spec SPEC-STORE-003
// parseStaticKeysRaw 는 options["keys"] 값을 정적 키 → 태그 맵으로 변환한다.
// 입력 형식: []any 에 담긴 map[string]any 각 엔트리는 {"key": string, "tags": map[string]any}.
//
// 검증 규칙:
//   - 중복 key → ErrDuplicateStaticKey
//   - 태그 key 가 tagKeyPattern 위반 → ErrInvalidTagKey
//   - 태그 value 가 string 이 아니면 → 명시적 에러 (panic 대신)
//   - 타입 오류 시 설명 포함 에러 반환
//
// 빈 배열([]) 이면 빈 맵을 반환한다. nil 이면 nil 을 반환한다.
func parseStaticKeysRaw(raw any) (map[string]map[string]string, error) {
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

	result := make(map[string]map[string]string, len(list))
	for i, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("store config: keys[%d] must be a map, got %T", i, item)
		}

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

		tags, err := parseTagsRaw(entry["tags"], key)
		if err != nil {
			return nil, err
		}
		result[key] = tags
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
func (a *UserStoreAgent) NodeStoreForNamespace(namespace string) any {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return NewNodeStoreAdapter(a.inner.ForNamespace(namespace))
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

	namespace, _ := params["namespace"].(string)
	if namespace == "" {
		namespace = "default"
	}

	a.mu.RLock()
	inner := a.inner
	a.mu.RUnlock()

	if inner == nil {
		return nil, fmt.Errorf("store not initialized")
	}

	store := inner.ForNamespace(namespace)
	entries, err := store.GetHistory(context.Background(), key)
	if err != nil {
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
func (a *UserStoreAgent) Configure(config agent.AgentConfig) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.agentConfig = config
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
		rawKey, _ := key.(string)
		displayKey := rawKey
		ns := item.namespace
		if ns != "" {
			displayKey = strings.TrimPrefix(rawKey, ns+":")
		}

		entry := map[string]any{
			"key":           displayKey,
			"value":         item.value,
			"namespace":     ns,
			"created_at":    item.createdAt.Format(time.RFC3339),
			"updated_at":    item.updatedAt.Format(time.RFC3339),
			"history_count": len(item.history),
		}

		// @spec SPEC-STORE-003: 정적 키로 선언된 키에 대해서는 태그 맵을 첨부한다.
		// 동적으로 쓰여진 키(정적 목록에 없음)는 tags 필드를 생략한다.
		if tags, ok := inner.config.staticKeys[displayKey]; ok && len(tags) > 0 {
			copied := make(map[string]string, len(tags))
			for tk, tv := range tags {
				copied[tk] = tv
			}
			entry["tags"] = copied
		}

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
