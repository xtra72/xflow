package system

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/xtra/xflow/pkg/lifecycle"
)

// MaxKeyLength 는 키 문자열의 최대 허용 길이(바이트)이다.
const MaxKeyLength = 512

// Store 는 키-값 저장소 인터페이스이다.
type Store interface {
	// Get 은 주어진 키에 해당하는 엔트리를 반환한다.
	// 키가 존재하지 않거나 만료된 경우 ErrKeyNotFound 를 반환한다.
	Get(ctx context.Context, key string) (StoreEntry, error)

	// Set 은 주어진 키에 값을 저장한다.
	// 기존 키가 존재하면 값과 UpdatedAt만 갱신하고 CreatedAt과 TTL을 보존한다.
	Set(ctx context.Context, key string, value any) error

	// SetWithTTL 은 주어진 키에 TTL과 함께 값을 저장한다.
	// 기존 키가 존재하면 CreatedAt을 보존하되 ExpiresAt을 갱신한다.
	SetWithTTL(ctx context.Context, key string, value any, ttl time.Duration) error

	// Delete 는 주어진 키를 삭제한다.
	// 키가 존재하지 않아도 에러를 반환하지 않는다.
	Delete(ctx context.Context, key string) error

	// Has 는 주어진 키가 존재하고 만료되지 않았는지 확인한다.
	Has(ctx context.Context, key string) (bool, error)

	// Keys 는 패턴에 일치하는 키 목록을 반환한다.
	// 빈 문자열이나 "*"은 모든 키를 반환한다.
	// 패턴은 path.Match 형식을 따른다.
	Keys(ctx context.Context, pattern string) ([]string, error)

	// Clear 는 모든 키를 삭제한다.
	Clear(ctx context.Context) error

	// GetHistory 는 주어진 키의 값 변경 히스토리를 최신순으로 반환한다.
	// 키가 존재하지 않으면 ErrKeyNotFound를 반환한다.
	// 키가 존재하지만 히스토리가 없으면 빈 슬라이스를 반환한다.
	//
	// 주의: 반환 결과는 이전 값(Set 이전의 값)들만 포함하며, 현재값은 포함하지 않는다.
	// 현재값까지 포함한 시계열 질의가 필요하면 QueryHistory 를 사용한다.
	GetHistory(ctx context.Context, key string) ([]HistoryEntry, error)

	// QueryHistory 는 HistoryQuery 조건에 부합하는 시계열 엔트리를 최신순으로 반환한다.
	// GetHistory 와 달리 결과에 현재값(StoreEntry.Value)을 포함한다.
	// 키가 존재하지 않으면 ErrKeyNotFound를 반환한다.
	// 결과가 없으면 빈 슬라이스와 nil 에러를 반환한다.
	QueryHistory(ctx context.Context, key string, query HistoryQuery) ([]HistoryEntry, error)

	// @spec SPEC-STORE-003
	// ClearHistory 는 주어진 키의 히스토리만 비우고 엔트리(value/ttl/createdAt 등)는 보존한다.
	// 키가 존재하지 않거나 만료된 경우 ErrKeyNotFound 를 반환한다.
	// 키가 존재하지만 히스토리가 비어있으면 no-op 으로 nil 을 반환한다.
	//
	// 정책 분기(정적 키는 ClearHistory, 동적 키는 Delete) 는 핸들러 계층의 책임이며,
	// 이 메서드 자체는 정적/동적을 구분하지 않는다.
	ClearHistory(ctx context.Context, key string) error
}

// QueryMode 는 HistoryQuery 의 조회 모드를 나타낸다.
type QueryMode string

const (
	// QueryModeLatest 는 현재값 1개만 반환한다. 다른 파라미터는 무시된다.
	QueryModeLatest QueryMode = "latest"
	// QueryModeLastN 는 최신순으로 Count 개를 반환한다.
	QueryModeLastN QueryMode = "last_n"
	// QueryModeDuration 는 now-Duration 이후 엔트리를 반환한다.
	QueryModeDuration QueryMode = "duration"
	// QueryModeTimeRange 는 [From, To] 구간의 엔트리를 반환한다 (양끝 포함).
	QueryModeTimeRange QueryMode = "time_range"
	// QueryModeSinceN 는 Since 시각 이후 최신순으로 Count 개를 반환한다.
	QueryModeSinceN QueryMode = "since_n"
)

// HistoryQuery 는 QueryHistory 호출 시 사용하는 조회 파라미터이다.
// Mode 에 따라 필요한 필드만 채우면 된다.
type HistoryQuery struct {
	Mode     QueryMode     // 조회 모드
	Count    int           // last_n, since_n 에서 사용 (> 0)
	Duration time.Duration // duration 에서 사용 (> 0)
	From     time.Time     // time_range 에서 사용 (inclusive)
	To       time.Time     // time_range 에서 사용 (inclusive)
	Since    time.Time     // since_n 에서 사용
}

// Validate 는 HistoryQuery 가 Mode 에 필요한 필드를 갖추었는지 검증한다.
func (q HistoryQuery) Validate() error {
	switch q.Mode {
	case QueryModeLatest:
		return nil
	case QueryModeLastN:
		if q.Count <= 0 {
			return fmt.Errorf("query mode %q requires count > 0", q.Mode)
		}
		return nil
	case QueryModeDuration:
		if q.Duration <= 0 {
			return fmt.Errorf("query mode %q requires duration > 0", q.Mode)
		}
		return nil
	case QueryModeTimeRange:
		if q.From.IsZero() || q.To.IsZero() {
			return fmt.Errorf("query mode %q requires from and to", q.Mode)
		}
		if q.To.Before(q.From) {
			return fmt.Errorf("query mode %q requires to >= from", q.Mode)
		}
		return nil
	case QueryModeSinceN:
		if q.Since.IsZero() {
			return fmt.Errorf("query mode %q requires since", q.Mode)
		}
		if q.Count <= 0 {
			return fmt.Errorf("query mode %q requires count > 0", q.Mode)
		}
		return nil
	default:
		return fmt.Errorf("unknown query mode: %q", q.Mode)
	}
}

// HistoryEntry 는 값 변경 히스토리의 개별 항목이다.
type HistoryEntry struct {
	Value     any       // 이전 값
	Timestamp time.Time // 해당 값이 기록된 시각
}

// StoreEntry 는 저장 엔트리를 나타내는 구조체이다.
type StoreEntry struct {
	Value          any           // 저장된 값
	TTL            time.Duration // 남은 유효 시간 (0이면 만료 없음)
	CreatedAt      time.Time     // 최초 생성 시각
	UpdatedAt      time.Time     // 마지막 갱신 시각
	Namespace      string        // 소속 네임스페이스
	ExpiresAt      time.Time     // 만료 예정 시각 (zero value면 만료 없음)
	HistoryCount   int           // 현재 히스토리 항목 수
	MaxHistorySize int           // 최대 히스토리 보관 수 (0이면 비활성)
}

// StoreRepository 는 영속 저장소 백엔드 인터페이스이다.
type StoreRepository interface {
	// GetEntry 는 키에 해당하는 엔트리를 조회한다.
	GetEntry(ctx context.Context, key string) (*StoreEntry, error)

	// SetEntry 는 키에 엔트리를 저장한다.
	SetEntry(ctx context.Context, key string, entry *StoreEntry) error

	// DeleteEntry 는 키를 삭제한다.
	DeleteEntry(ctx context.Context, key string) error

	// ListKeys 는 패턴에 일치하는 키 목록을 조회한다.
	ListKeys(ctx context.Context, pattern string) ([]string, error)

	// DeleteExpired 는 before 시각 이전에 만료된 엔트리를 삭제하고 삭제 건수를 반환한다.
	DeleteExpired(ctx context.Context, before time.Time) (int, error)

	// ClearNamespace 는 주어진 네임스페이스의 모든 엔트리를 삭제한다.
	ClearNamespace(ctx context.Context, namespace string) error
}

// ---------------------------------------------------------------------------
// StoreAgent - Store System Agent (Lifecycle + Configurable + HealthChecker)
// ---------------------------------------------------------------------------

// StoreAgent 는 Store System Agent이다.
// Lifecycle, Configurable, HealthChecker 인터페이스를 구현한다.
type StoreAgent struct {
	*lifecycle.BaseLifecycle          // 임베딩
	config                  storeConfig    // 설정
	store                   *VolatileStore // 내부 저장소 (현재는 volatile만)
	ttlMgr                  *ttlManager    // TTL 매니저
	mu                      sync.RWMutex   // 상태 보호
	paused                  bool           // Pause 상태 플래그
	closed                  bool           // Stop 상태 플래그
}

// NewStoreAgent 는 주어진 옵션으로 StoreAgent를 생성한다.
// 초기 상태는 StateCreated이며, 스토어와 TTL 매니저는 Init에서 생성된다.
func NewStoreAgent(opts ...StoreOption) *StoreAgent {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	return &StoreAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("store-agent")),
		config:        cfg,
	}
}

// Init 은 StoreAgent를 초기화한다.
// Created 상태에서만 호출 가능하며, VolatileStore와 ttlManager를 생성하고 Running 상태로 전이한다.
func (s *StoreAgent) Init(_ context.Context) error {
	// Created → Initializing 전이
	if err := s.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("store-agent: init 전이 실패: %w", err)
	}

	// VolatileStore 생성
	s.store = NewVolatileStore(s.config.maxKeyLength, s.config.maxHistorySize, s.config.historyTTL)

	// TTL 매니저 생성 및 시작
	s.ttlMgr = newTTLManager(s.store, s.config.scanInterval)
	s.ttlMgr.Start()

	// Initializing → Running 전이
	if err := s.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("store-agent: running 전이 실패: %w", err)
	}

	return nil
}

// Start 는 컴포넌트를 시작한다.
// Init이 이미 Running으로 전이하므로, 이미 Running 상태이면 no-op이다.
func (s *StoreAgent) Start(_ context.Context) error {
	if s.CurrentState() == lifecycle.StateRunning {
		return nil
	}
	return fmt.Errorf("store-agent: start는 Running 상태에서만 no-op (현재: %s)", s.CurrentState())
}

// Pause 는 StoreAgent를 일시정지한다.
// Running → Paused 전이. 쓰기 연산은 비활성화되고 읽기는 허용된다.
func (s *StoreAgent) Pause(_ context.Context) error {
	if err := s.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("store-agent: pause 전이 실패: %w", err)
	}

	s.mu.Lock()
	s.paused = true
	s.mu.Unlock()

	s.ttlMgr.Pause()

	return nil
}

// Resume 은 일시정지된 StoreAgent를 재개한다.
// Paused → Running 전이.
func (s *StoreAgent) Resume(_ context.Context) error {
	if err := s.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("store-agent: resume 전이 실패: %w", err)
	}

	s.mu.Lock()
	s.paused = false
	s.mu.Unlock()

	s.ttlMgr.Resume()

	return nil
}

// Stop 은 StoreAgent를 정지한다.
// Running 또는 Paused → Stopping → Stopped 전이.
func (s *StoreAgent) Stop(_ context.Context) error {
	if err := s.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("store-agent: stopping 전이 실패: %w", err)
	}

	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()

	s.ttlMgr.Stop()

	if err := s.TransitionTo(lifecycle.StateStopped); err != nil {
		return fmt.Errorf("store-agent: stopped 전이 실패: %w", err)
	}

	return nil
}

// State 는 컴포넌트의 현재 상태를 반환한다.
func (s *StoreAgent) State() lifecycle.State {
	return s.CurrentState()
}

// Configure 는 런타임에 설정을 변경한다.
// Running 또는 Paused 상태에서만 호출 가능하다.
// 지원 키: "ttl_scan_interval" (string duration), "default_ttl" (string duration)
func (s *StoreAgent) Configure(_ context.Context, cfg map[string]any) error {
	state := s.CurrentState()
	if state != lifecycle.StateRunning && state != lifecycle.StatePaused {
		return fmt.Errorf("store-agent: configure는 Running/Paused에서만 가능 (현재: %s): %w",
			state, lifecycle.ErrInvalidStateForConfigure)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if v, ok := cfg["ttl_scan_interval"]; ok {
		str, ok := v.(string)
		if !ok {
			return fmt.Errorf("store-agent: ttl_scan_interval은 string이어야 한다")
		}
		d, err := time.ParseDuration(str)
		if err != nil {
			return fmt.Errorf("store-agent: ttl_scan_interval 파싱 실패: %w", err)
		}
		s.config.scanInterval = d
		if s.ttlMgr != nil {
			s.ttlMgr.SetInterval(d)
		}
	}

	if v, ok := cfg["default_ttl"]; ok {
		str, ok := v.(string)
		if !ok {
			return fmt.Errorf("store-agent: default_ttl은 string이어야 한다")
		}
		d, err := time.ParseDuration(str)
		if err != nil {
			return fmt.Errorf("store-agent: default_ttl 파싱 실패: %w", err)
		}
		s.config.defaultTTL = d
	}

	if v, ok := cfg["max_history_size"]; ok {
		n, ok := v.(int)
		if !ok {
			return fmt.Errorf("store-agent: max_history_size는 int이어야 한다")
		}
		s.config.maxHistorySize = n
		if s.store != nil {
			s.store.maxHistorySize = n
		}
	}

	if v, ok := cfg["history_ttl"]; ok {
		str, ok := v.(string)
		if !ok {
			return fmt.Errorf("store-agent: history_ttl은 string이어야 한다")
		}
		d, err := time.ParseDuration(str)
		if err != nil {
			return fmt.Errorf("store-agent: history_ttl 파싱 실패: %w", err)
		}
		s.config.historyTTL = d
		if s.store != nil {
			s.store.historyTTL = d
		}
	}

	return nil
}

// GetConfig 는 현재 설정을 map으로 반환한다.
func (s *StoreAgent) GetConfig() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]any{
		"backend":          s.config.backend,
		"default_ttl":      s.config.defaultTTL,
		"scan_interval":    s.config.scanInterval,
		"max_key_length":   s.config.maxKeyLength,
		"max_history_size": s.config.maxHistorySize,
		"history_ttl":      s.config.historyTTL,
	}
}

// HealthCheck 는 StoreAgent의 건강 상태를 반환한다.
func (s *StoreAgent) HealthCheck(_ context.Context) lifecycle.HealthStatus {
	state := s.CurrentState()
	healthy := state == lifecycle.StateRunning || state == lifecycle.StatePaused

	details := map[string]any{
		"backend": s.config.backend,
		"state":   string(state),
	}

	// Running/Paused 상태에서만 키 수를 집계한다
	if healthy && s.store != nil {
		count := 0
		s.store.data.Range(func(_, _ any) bool {
			count++
			return true
		})
		details["key_count"] = count
	}

	message := "store-agent is healthy"
	if !healthy {
		message = fmt.Sprintf("store-agent is not healthy (state: %s)", state)
	}

	return lifecycle.HealthStatus{
		Healthy:     healthy,
		Message:     message,
		LastChecked: time.Now(),
		Details:     details,
	}
}

// ForNamespace 는 주어진 네임스페이스에 대한 Store를 반환한다.
// agentStore를 통해 StoreAgent의 상태(paused/closed)를 확인하고,
// NamespacedStore를 통해 네임스페이스 접두사를 적용한다.
func (s *StoreAgent) ForNamespace(namespace string) Store {
	return NewNamespacedStore(&agentStore{agent: s}, namespace)
}

// @spec SPEC-STORE-003
// StaticTagsFor 는 사용자 관점 key 의 정적 태그 맵 복사본을 반환한다.
// key 가 정적 키 목록에 없으면 빈 맵을 반환한다.
func (s *StoreAgent) StaticTagsFor(key string) map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.config.staticKeys[key]
	if !ok {
		return map[string]string{}
	}
	out := make(map[string]string, len(t))
	for k, v := range t {
		out[k] = v
	}
	return out
}

// @spec SPEC-STORE-003
// SetAllowDynamicKeys 는 allowDynamicKeys 정책 플래그를 런타임에 갱신한다.
// 동시 호출에 안전하며, 내부 VolatileStore 에 저장된 기존 값에는 영향을 주지 않는다.
// 재시작 없이 Web UI 등에서 toggle 된 값이 즉시 반영되도록 한다.
func (s *StoreAgent) SetAllowDynamicKeys(allow bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.allowDynamicKeys = allow
}

// @spec SPEC-STORE-003
// SetStaticKeys 는 정적 키 → 태그 맵을 런타임에 교체한다.
// 동시 호출에 안전하며, 내부 VolatileStore 에 저장된 기존 값에는 영향을 주지 않는다
// (정책 변경이 저장된 데이터를 삭제하지 않는다).
//
// nil 또는 빈 맵을 전달하면 정적 키 정의가 제거된다.
// 전달된 맵은 깊은 복사되어 내부에 저장되므로, 호출자가 이후 수정해도 안전하다.
func (s *StoreAgent) SetStaticKeys(keys map[string]map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(keys) == 0 {
		s.config.staticKeys = nil
		return
	}
	cloned := make(map[string]map[string]string, len(keys))
	for k, tags := range keys {
		tagCopy := make(map[string]string, len(tags))
		for tk, tv := range tags {
			tagCopy[tk] = tv
		}
		cloned[k] = tagCopy
	}
	s.config.staticKeys = cloned
}

// @spec SPEC-STORE-003
// StaticKeyTags 는 (사용자 키 → 태그 맵) 전체 복사본을 반환한다.
// 정적 키가 하나도 없으면 빈 맵을 반환한다.
// 반환 맵은 호출자 전용 복사본으로, 내부 상태와 분리되어 있다.
func (s *StoreAgent) StaticKeyTags() map[string]map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.config.staticKeys) == 0 {
		return map[string]map[string]string{}
	}
	out := make(map[string]map[string]string, len(s.config.staticKeys))
	for k, tags := range s.config.staticKeys {
		copied := make(map[string]string, len(tags))
		for tk, tv := range tags {
			copied[tk] = tv
		}
		out[k] = copied
	}
	return out
}

// ---------------------------------------------------------------------------
// agentStore - StoreAgent의 상태를 확인하고 내부 VolatileStore에 위임하는 래퍼
// ---------------------------------------------------------------------------

// agentStore 는 StoreAgent의 상태를 확인하고 내부 VolatileStore에 위임하는 래퍼이다.
type agentStore struct {
	agent *StoreAgent
}

// 컴파일 타임 인터페이스 체크
var _ Store = (*agentStore)(nil)

// checkClosed 는 스토어가 닫혔는지 확인한다.
func (as *agentStore) checkClosed() error {
	as.agent.mu.RLock()
	defer as.agent.mu.RUnlock()
	if as.agent.closed {
		return ErrStoreClosed
	}
	return nil
}

// checkWrite 는 쓰기 연산이 가능한지 확인한다 (closed + paused 체크).
func (as *agentStore) checkWrite() error {
	as.agent.mu.RLock()
	defer as.agent.mu.RUnlock()
	if as.agent.closed {
		return ErrStoreClosed
	}
	if as.agent.paused {
		return ErrStorePaused
	}
	return nil
}

// Get 은 읽기 연산이므로 closed만 확인한다 (paused에서도 읽기 허용).
func (as *agentStore) Get(ctx context.Context, key string) (StoreEntry, error) {
	if err := as.checkClosed(); err != nil {
		return StoreEntry{}, err
	}
	return as.agent.store.Get(ctx, key)
}

// Set 은 쓰기 연산이므로 closed와 paused를 모두 확인한다.
func (as *agentStore) Set(ctx context.Context, key string, value any) error {
	if err := as.checkWrite(); err != nil {
		return err
	}
	return as.agent.store.Set(ctx, key, value)
}

// SetWithTTL 은 쓰기 연산이므로 closed와 paused를 모두 확인한다.
func (as *agentStore) SetWithTTL(ctx context.Context, key string, value any, ttl time.Duration) error {
	if err := as.checkWrite(); err != nil {
		return err
	}
	return as.agent.store.SetWithTTL(ctx, key, value, ttl)
}

// @spec SPEC-STORE-003
// checkKeyAllowed 는 사용자 관점 key (네임스페이스 접두사 제외) 가 쓰기 허용 대상인지 검사한다.
// allowDynamicKeys=true 이면 항상 허용이다.
// allowDynamicKeys=false 이면 정적 키 목록에 등록된 키만 허용되고,
// 미등록 키는 ErrKeyNotAllowed 를 반환한다.
//
// NamespacedStore 가 쓰기 전에 keyGatekeeper 인터페이스 단언으로 호출한다.
func (as *agentStore) checkKeyAllowed(key string) error {
	as.agent.mu.RLock()
	defer as.agent.mu.RUnlock()
	if as.agent.config.allowDynamicKeys {
		return nil
	}
	if _, ok := as.agent.config.staticKeys[key]; ok {
		return nil
	}
	return ErrKeyNotAllowed
}

// @spec SPEC-STORE-003
// TagsFor 는 사용자 관점 key 의 정적 태그 맵을 복사하여 반환한다.
// key 가 정적 키 목록에 없으면 빈 맵을 반환한다.
// 반환된 맵은 내부 저장소와 분리된 복사본이므로 호출자가 자유롭게 수정할 수 있다.
func (as *agentStore) TagsFor(key string) map[string]string {
	as.agent.mu.RLock()
	defer as.agent.mu.RUnlock()
	t, ok := as.agent.config.staticKeys[key]
	if !ok {
		return map[string]string{}
	}
	out := make(map[string]string, len(t))
	for k, v := range t {
		out[k] = v
	}
	return out
}

// Delete 는 쓰기 연산이므로 closed와 paused를 모두 확인한다.
func (as *agentStore) Delete(ctx context.Context, key string) error {
	if err := as.checkWrite(); err != nil {
		return err
	}
	return as.agent.store.Delete(ctx, key)
}

// Has 는 읽기 연산이므로 closed만 확인한다 (paused에서도 읽기 허용).
func (as *agentStore) Has(ctx context.Context, key string) (bool, error) {
	if err := as.checkClosed(); err != nil {
		return false, err
	}
	return as.agent.store.Has(ctx, key)
}

// Keys 는 읽기 연산이므로 closed만 확인한다 (paused에서도 읽기 허용).
func (as *agentStore) Keys(ctx context.Context, pattern string) ([]string, error) {
	if err := as.checkClosed(); err != nil {
		return nil, err
	}
	return as.agent.store.Keys(ctx, pattern)
}

// Clear 는 쓰기 연산이므로 closed와 paused를 모두 확인한다.
func (as *agentStore) Clear(ctx context.Context) error {
	if err := as.checkWrite(); err != nil {
		return err
	}
	return as.agent.store.Clear(ctx)
}

// GetHistory 는 읽기 연산이므로 closed만 확인한다 (paused에서도 읽기 허용).
func (as *agentStore) GetHistory(ctx context.Context, key string) ([]HistoryEntry, error) {
	if err := as.checkClosed(); err != nil {
		return nil, err
	}
	return as.agent.store.GetHistory(ctx, key)
}

// QueryHistory 는 읽기 연산이므로 closed만 확인한다 (paused에서도 읽기 허용).
func (as *agentStore) QueryHistory(ctx context.Context, key string, q HistoryQuery) ([]HistoryEntry, error) {
	if err := as.checkClosed(); err != nil {
		return nil, err
	}
	return as.agent.store.QueryHistory(ctx, key, q)
}

// @spec SPEC-STORE-003
// ClearHistory 는 쓰기 연산으로 분류한다 (storeItem 의 history 필드를 변경하므로).
// closed/paused 검사를 모두 거친 뒤 내부 VolatileStore 에 위임한다.
func (as *agentStore) ClearHistory(ctx context.Context, key string) error {
	if err := as.checkWrite(); err != nil {
		return err
	}
	return as.agent.store.ClearHistory(ctx, key)
}

// setItemNamespace 는 내부 VolatileStore에 위임한다 (namespaceWriter 구현).
func (as *agentStore) setItemNamespace(key string, namespace string) {
	as.agent.store.setItemNamespace(key, namespace)
}
