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
	*lifecycle.BaseLifecycle                // 임베딩
	config                   storeConfig    // 설정
	store                    *VolatileStore // 내부 저장소 (현재는 volatile만)
	ttlMgr                   *ttlManager    // TTL 매니저
	mu                       sync.RWMutex   // 상태 보호
	paused                   bool           // Pause 상태 플래그
	closed                   bool           // Stop 상태 플래그
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

// @spec SPEC-STORE-003 v0.3.0
// StaticTagsFor 는 사용자 관점 key 의 정적 태그 맵 복사본을 반환한다.
// key 가 정적 키 목록에 없으면 빈 맵을 반환한다.
// v0.3.0 진화: 내부 staticKeys 가 StaticKeyMeta 로 변경되었으므로 .Tags 필드를 추출한다.
func (s *StoreAgent) StaticTagsFor(key string) map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	meta, ok := s.config.staticKeys[key]
	if !ok {
		return map[string]string{}
	}
	out := make(map[string]string, len(meta.Tags))
	for k, v := range meta.Tags {
		out[k] = v
	}
	return out
}

// @spec SPEC-STORE-003 v0.3.0
// StaticKeyMetaFor 는 사용자 관점 key 의 전체 메타데이터(StaticKeyMeta) 복사본을 반환한다.
// key 가 정적 키 목록에 없으면 (zero value, false) 를 반환한다.
// API 응답 구성(Phase D) 에서 data_type / metric_type / source 까지 노출할 때 사용된다.
//
// 반환된 StaticKeyMeta 의 Tags 는 깊은 복사본이며, 호출자가 수정해도 내부 상태에 영향이 없다.
func (s *StoreAgent) StaticKeyMetaFor(key string) (StaticKeyMeta, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	meta, ok := s.config.staticKeys[key]
	if !ok {
		return StaticKeyMeta{}, false
	}
	// Tags 깊은 복사로 호출자 수정으로부터 내부 상태를 보호.
	tagsCopy := make(map[string]string, len(meta.Tags))
	for tk, tv := range meta.Tags {
		tagsCopy[tk] = tv
	}
	return StaticKeyMeta{
		DataType:   meta.DataType,
		MetricType: meta.MetricType,
		Tags:       tagsCopy,
		Source:     meta.Source,
	}, true
}

// @spec SPEC-STORE-003 v0.3.0
// SetRegistrationType 은 키 등록 정책(RegistrationManual / RegistrationAuto) 을 런타임에 갱신한다.
// 동시 호출에 안전하며, 내부 VolatileStore 에 저장된 기존 값에는 영향을 주지 않는다.
// 재시작 없이 Web UI 등에서 toggle 된 값이 즉시 반영되도록 한다.
//
// v0.2.0 의 SetAllowDynamicKeys(bool) 를 clean rename 한 것이다 (no shim).
func (s *StoreAgent) SetRegistrationType(rt RegistrationType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.registrationType = rt
}

// @spec SPEC-STORE-003 v0.3.0
// SetStaticKeys 는 정적 키 → 메타데이터 맵을 런타임에 교체한다.
// 동시 호출에 안전하며, 내부 VolatileStore 에 저장된 기존 값에는 영향을 주지 않는다
// (정책 변경이 저장된 데이터를 삭제하지 않는다).
//
// nil 또는 빈 맵을 전달하면 정적 키 정의가 제거된다.
// 전달된 맵은 깊은 복사되어 내부에 저장되므로, 호출자가 이후 수정해도 안전하다.
//
// v0.3.0 진화: value 타입이 v0.2.0 의 map[string]string 에서 StaticKeyMeta 로 변경되었다.
func (s *StoreAgent) SetStaticKeys(keys map[string]StaticKeyMeta) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(keys) == 0 {
		s.config.staticKeys = nil
		return
	}
	cloned := make(map[string]StaticKeyMeta, len(keys))
	for k, meta := range keys {
		tagCopy := make(map[string]string, len(meta.Tags))
		for tk, tv := range meta.Tags {
			tagCopy[tk] = tv
		}
		cloned[k] = StaticKeyMeta{
			DataType:   meta.DataType,
			MetricType: meta.MetricType,
			Tags:       tagCopy,
			Source:     meta.Source,
		}
	}
	s.config.staticKeys = cloned
}

// @spec SPEC-STORE-003 v0.4.0
// SetKeyMeta 는 임의 엔트리(정적 또는 동적)의 metric_type 과 tags 를 설정한다.
// 사용자가 Web UI 등에서 동적으로 등록된 키에도 타입/태그를 나중에 부여할 수 있게 한다.
//
// 동작:
//   - key 가 이미 staticKeys 에 있으면 metric_type/tags 만 갱신하고 DataType/Source 는 보존한다
//     (정적 키의 명시 data_type, 동적 키의 string data_type 모두 보존 — PRESERVE).
//   - key 가 미등록이면 동적 string 키(DataType=string, Source=auto)로 신규 등록한 뒤 메타를 적용한다.
//     이로써 아직 값이 쓰여지지 않은 키에도 사전에 타입/태그를 지정할 수 있다.
//
// 입력 검증(metric_type 정규식, tags key/value)은 호출자(핸들러) 책임이며, 본 메서드는
// 전달된 값을 신뢰하고 깊은 복사하여 저장한다. metricType 이 빈 문자열이면 "unknown" 으로 normalize 한다.
//
// 동시 호출에 안전하며, 내부 VolatileStore 에 저장된 값(value/ttl/history)에는 영향을 주지 않는다.
func (s *StoreAgent) SetKeyMeta(key string, metricType string, tags map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if metricType == "" {
		metricType = MetricTypeUnknown
	}
	tagsCopy := make(map[string]string, len(tags))
	for tk, tv := range tags {
		tagsCopy[tk] = tv
	}

	if s.config.staticKeys == nil {
		s.config.staticKeys = make(map[string]StaticKeyMeta)
	}

	if existing, ok := s.config.staticKeys[key]; ok {
		// 기존 메타 보존(DataType/Source) + metric_type/tags 갱신.
		existing.MetricType = metricType
		existing.Tags = tagsCopy
		s.config.staticKeys[key] = existing
		return
	}

	// 미등록 키: 동적 string 키로 신규 등록.
	s.config.staticKeys[key] = StaticKeyMeta{
		DataType:   DataTypeString,
		MetricType: metricType,
		Tags:       tagsCopy,
		Source:     SourceAuto,
	}
}

// @spec SPEC-STORE-003 (store-write 노드 data_type/tags 지정)
// SetKeyDataType 은 지정된 key 를 명시 data_type 으로 등록/갱신한다.
// store-write 노드가 config 의 data_type 을 통해 동적 키를 특정 타입으로 고정(pin)할 때 사용한다.
//
// data_type 적용 정책 (PRESERVE 우선):
//   - 미등록 키: 지정 dataType + Source=auto 로 신규 등록한다. 이후 쓰기는 해당 타입으로 검증된다.
//   - 동적 string 키(Source=auto && data_type=string): 지정 dataType 으로 덮어쓴다.
//     (동적 키는 사용자가 노드 설정으로 타입을 명시한 것이므로 그 의도를 반영한다.)
//     단, 지정 dataType 도 string 이면 동적 string 그대로 유지된다.
//   - 정적/명시 data_type 키(Source=manual 또는 non-dynamic): DataType/Source 를 보존한다.
//     yaml 로 선언된 타입 계약을 노드 설정이 침범하지 못하도록 한다 (PRESERVE).
//
// metric_type 은 이 메서드의 범위가 아니다. 기존 키의 metric_type 은 보존되고,
// 신규 등록 키에는 "unknown" 이 부여된다.
//
// 동시 호출에 안전하며, 내부 VolatileStore 에 저장된 값(value/ttl/history)에는 영향을 주지 않는다.
func (s *StoreAgent) SetKeyDataType(key string, dataType DataType) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.config.staticKeys == nil {
		s.config.staticKeys = make(map[string]StaticKeyMeta)
	}

	if existing, ok := s.config.staticKeys[key]; ok {
		// 동적 string 키만 지정 타입으로 덮어쓴다. 그 외(정적/명시 타입)는 보존.
		if isDynamicStringMeta(existing) {
			existing.DataType = dataType
			// 동적 키에 명시 타입을 부여하면 더 이상 "동적 string" 이 아니므로
			// Source 를 manual 로 승격해 후속 쓰기에서 타입 검증(coercion 우회)이 적용되게 한다.
			// 단, 지정 타입이 string 이면 동적 string 정책을 그대로 유지한다.
			if dataType != DataTypeString {
				existing.Source = SourceManual
			}
			s.config.staticKeys[key] = existing
		}
		return
	}

	// 미등록 키: 지정 data_type 으로 신규 등록.
	// 지정 타입이 string 이면 동적 string 정책(Source=auto)을 따르고,
	// 그 외 타입이면 명시 등록(Source=manual)으로 타입 검증이 적용되게 한다.
	src := SourceManual
	if dataType == DataTypeString {
		src = SourceAuto
	}
	s.config.staticKeys[key] = StaticKeyMeta{
		DataType:   dataType,
		MetricType: MetricTypeUnknown,
		Tags:       map[string]string{},
		Source:     src,
	}
}

// @spec SPEC-STORE-003 v0.3.0
// StaticKeysSnapshot 은 (사용자 키 → StaticKeyMeta) 전체 깊은 복사본을 반환한다.
// 정적 키가 하나도 없으면 빈 맵을 반환한다.
// 반환 맵은 호출자 전용 복사본으로, 내부 상태와 분리되어 있다.
//
// API 핸들러(Phase D) 에서 필터/정렬/응답 빌드를 위한 일관된 스냅샷이 필요할 때 사용한다.
func (s *StoreAgent) StaticKeysSnapshot() map[string]StaticKeyMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.config.staticKeys) == 0 {
		return map[string]StaticKeyMeta{}
	}
	out := make(map[string]StaticKeyMeta, len(s.config.staticKeys))
	for k, meta := range s.config.staticKeys {
		tagsCopy := make(map[string]string, len(meta.Tags))
		for tk, tv := range meta.Tags {
			tagsCopy[tk] = tv
		}
		out[k] = StaticKeyMeta{
			DataType:   meta.DataType,
			MetricType: meta.MetricType,
			Tags:       tagsCopy,
			Source:     meta.Source,
		}
	}
	return out
}

// @spec SPEC-STORE-003 v0.3.0
// StaticKeyTags 는 (사용자 키 → 태그 맵) 전체 복사본을 반환한다.
// 정적 키가 하나도 없으면 빈 맵을 반환한다.
//
// v0.3.0 호환 shim: 내부 staticKeys 는 StaticKeyMeta 이지만, 기존 API 핸들러 인터페이스
// (KeyTags(ctx) → map[string]map[string]string) 와의 호환을 위해 tags-only view 를 빌드한다.
// Phase D 에서 핸들러가 StaticKeysSnapshot() 으로 마이그레이션되면 이 메서드는 제거 가능하다.
func (s *StoreAgent) StaticKeyTags() map[string]map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.config.staticKeys) == 0 {
		return map[string]map[string]string{}
	}
	out := make(map[string]map[string]string, len(s.config.staticKeys))
	for k, meta := range s.config.staticKeys {
		copied := make(map[string]string, len(meta.Tags))
		for tk, tv := range meta.Tags {
			copied[tk] = tv
		}
		out[k] = copied
	}
	return out
}

// @spec SPEC-STORE-003 v0.3.0 (Phase B 회귀 수정)
// SetMaxHistorySize 는 최대 히스토리 보관 수를 런타임에 변경한다.
// 동시 호출에 안전하며, 기존 저장된 엔트리에는 영향을 주지 않고
// 이후 Set() 호출부터 새 제한이 적용된다.
func (s *StoreAgent) SetMaxHistorySize(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.maxHistorySize = n
	if s.store != nil {
		s.store.maxHistorySize = n
	}
}

// @spec SPEC-STORE-003 v0.3.0 (Phase B 회귀 수정)
// SetHistoryTTL 은 히스토리 엔트리의 TTL을 런타임에 변경한다.
// 동시 호출에 안전하며, 기존 저장된 엔트리에는 영향을 주지 않고
// 이후 Set() 호출부터 새 TTL이 적용된다.
func (s *StoreAgent) SetHistoryTTL(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.historyTTL = d
	if s.store != nil {
		s.store.historyTTL = d
	}
}

// @spec SPEC-STORE-003 v0.3.0 (Phase B 회귀 수정)
// SetScanInterval 은 TTL 스캔 간격을 런타임에 변경한다.
// 동시 호출에 안전하며, TTL 매니저의 스캔 간격을 즉시 변경한다.
func (s *StoreAgent) SetScanInterval(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.scanInterval = d
	if s.ttlMgr != nil {
		s.ttlMgr.SetInterval(d)
	}
}

// @spec SPEC-STORE-003 v0.3.0 (Phase B 회귀 수정)
// SetDefaultTTL 은 기본 TTL을 런타임에 변경한다.
// 동시 호출에 안전하며, 이후 SetWithTTL() 호출의 기본값에만 영향을 준다.
func (s *StoreAgent) SetDefaultTTL(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.defaultTTL = d
}

// @spec SPEC-STORE-003 v0.3.0 (Phase B 회귀 수정)
// SetMaxKeyLength 는 최대 키 길이를 런타임에 변경한다.
// 동시 호출에 안전하며, 이후 Set() 호출에서 새 길이 제한을 검증한다.
func (s *StoreAgent) SetMaxKeyLength(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.maxKeyLength = n
	if s.store != nil {
		s.store.maxKeyLength = n
	}
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

// @spec SPEC-STORE-003 v0.3.0
// checkKeyAllowed 는 사용자 관점 key (네임스페이스 접두사 제외) 와 value 의 쌍이
// 쓰기 허용 대상인지 검사한다. NamespacedStore 가 쓰기 전에 keyGatekeeper 인터페이스
// 단언으로 호출하며, 거부 시 inner.Set 이 호출되지 않으므로 엔트리/히스토리에 흔적이
// 남지 않는다 (M2 unwanted 요구사항 보존).
//
// 결정 매트릭스:
//
//	registrationType  | key 등록상태 | value 검증            | 결과
//	------------------|-------------|-----------------------|---------------------------
//	manual            | 등록됨      | 타입 일치             | nil (허용)
//	manual            | 등록됨      | 타입 불일치           | ErrTypeMismatch
//	manual            | 미등록      | -                     | ErrKeyNotAllowed
//	auto              | 등록됨      | 타입 일치             | nil (허용)
//	auto              | 등록됨      | 타입 불일치           | ErrTypeMismatch
//	auto              | 미등록      | inferDataType 성공    | nil + 자동 등록 (SourceAuto)
//	auto              | 미등록      | inferDataType 실패    | ErrUnsupportedValueType
//
// 동시성 설계 (double-checked locking):
//  1. Fast path: RLock 으로 (regType, meta, exists) 를 한 번에 읽는다 — 등록된 키에 대한
//     일치 검증은 이 경로에서 종료되어 쓰기 락 없이 처리된다.
//  2. Slow path: auto 모드 + 미등록 키일 때만 inferDataType 후 Lock 을 취득하고
//     맵 재확인 (race winner 가 이미 등록했을 수 있음) → 등록된 경우 winner 의 DataType
//     기준으로 일치 검증, 미등록인 경우에만 실제 등록 수행.
//
// 락 정책: RLock → (release) → inferDataType → Lock — 락 다운그레이드 없음. RLock 보유 중
// inferDataType 호출은 의도적으로 회피했다 (encoding/json.Marshal 이 reflect 를 사용하므로
// RLock 보유 시간을 늘리지 않는 편이 안전).
func (as *agentStore) checkKeyAllowed(key string, value any) error {
	// Fast path: RLock 으로 등록 여부와 메타를 한 번에 본다.
	as.agent.mu.RLock()
	meta, exists := as.agent.config.staticKeys[key]
	regType := as.agent.config.registrationType
	as.agent.mu.RUnlock()

	if exists {
		// 등록된 키.
		// @spec SPEC-STORE-003 v0.4.0: 동적(SourceAuto) string 키는 어떤 값이든 허용한다.
		// 후속 쓰기는 coerceWriteValue 에서 string 으로 변환되므로 타입 불일치가 없다.
		// 이로써 동적 키는 사용자가 어떤 타입을 써도 항상 string 으로 보관된다.
		if isDynamicStringMeta(meta) {
			return nil
		}
		// manual 명시 또는 명시 data_type 으로 등록된 키: DataType 일치 검증 (type pinning).
		if !matchesDataType(value, meta.DataType) {
			return ErrTypeMismatch
		}
		return nil
	}

	// 미등록 키.
	if regType == RegistrationManual {
		// manual 모드는 미등록 키 쓰기를 거부한다 (M2 / Scenario 2).
		return ErrKeyNotAllowed
	}

	// @spec SPEC-STORE-003 v0.4.0
	// auto 모드 + 미등록: 값 타입과 무관하게 data_type=string 으로 자동 등록한다 (동적=string 정책).
	// 단, nil 및 의미 있는 문자열 표현이 없는 타입(channel, func)은 거부한다
	// (기존 ErrUnsupportedValueType 동작 보존). 그 외 모든 값은 stringifyValue 로 문자열화된다.
	if !isStringifiableValue(value) {
		return ErrUnsupportedValueType
	}

	// Slow path: 쓰기 락으로 자동 등록.
	as.agent.mu.Lock()
	defer as.agent.mu.Unlock()

	// Double-check: 다른 goroutine 이 RLock 해제 ~ Lock 취득 사이에 같은 키를 등록했을 수 있다.
	if existing, raced := as.agent.config.staticKeys[key]; raced {
		// race winner 가 동적 string 키로 등록했다면 그대로 허용 (값은 coerce 단계에서 변환).
		if isDynamicStringMeta(existing) {
			return nil
		}
		if !matchesDataType(value, existing.DataType) {
			return ErrTypeMismatch
		}
		return nil
	}

	// 자동 등록 수행: 항상 data_type=string, metric_type=unknown, 빈 태그, SourceAuto.
	if as.agent.config.staticKeys == nil {
		as.agent.config.staticKeys = make(map[string]StaticKeyMeta)
	}
	as.agent.config.staticKeys[key] = StaticKeyMeta{
		DataType:   DataTypeString,
		MetricType: MetricTypeUnknown,
		Tags:       map[string]string{},
		Source:     SourceAuto,
	}
	return nil
}

// @spec SPEC-STORE-003 v0.4.0
// isDynamicStringMeta 는 메타데이터가 "동적 string 키"인지 판별한다.
// 동적 string 키는 auto 모드에서 런타임 자동 등록된 키로, data_type=string + Source=auto 이다.
// 이런 키는 어떤 값이든 string 으로 변환되어 저장되므로 쓰기 시 타입 검증을 건너뛴다.
func isDynamicStringMeta(meta StaticKeyMeta) bool {
	return meta.Source == SourceAuto && meta.DataType == DataTypeString
}

// @spec SPEC-STORE-003 v0.4.0
// coerceWriteValue 는 쓰기 직전 값을 정책에 맞게 변환한다.
// 동적 string 키(isDynamicStringMeta)에 대해서는 값을 string 으로 변환하여 반환하고,
// 그 외(명시 data_type 키, 미등록 키)는 원본 값을 그대로 반환한다.
//
// NamespacedStore.Set/SetWithTTL 이 checkKeyAllowed 통과 후 이 메서드를 호출한다.
// checkKeyAllowed 가 미등록 키를 동적 string 으로 등록한 뒤이므로, 첫 쓰기 값도 여기서 string 화된다.
func (as *agentStore) coerceWriteValue(key string, value any) any {
	as.agent.mu.RLock()
	meta, ok := as.agent.config.staticKeys[key]
	as.agent.mu.RUnlock()
	if !ok {
		return value
	}
	if isDynamicStringMeta(meta) {
		return stringifyValue(value)
	}
	// 숫자 타입 정규화: 선언 data_type 에 맞춰 저장값을 변환한다(JSON float64 호환).
	//  - int  선언 + 정수값 float64(2.0) → int64(2)
	//  - float 선언 + 정수(int 계열)     → float64
	// 그 외에는 원본 값을 그대로 둔다.
	return coerceNumericToDeclared(value, meta.DataType)
}

// coerceNumericToDeclared 는 값을 선언된 숫자 data_type 에 맞춰 정규화한다.
// matchesDataType 의 JSON 숫자 호환(정수값 float ↔ int)과 짝을 이루어, 검증을 통과한
// 값이 선언 타입으로 저장되게 한다. 대상이 아니면 원본을 반환한다.
func coerceNumericToDeclared(value any, dt DataType) any {
	switch dt {
	case DataTypeInt:
		switch v := value.(type) {
		case float64:
			if isIntegralFloat(v) {
				return int64(v)
			}
		case float32:
			if isIntegralFloat(v) {
				return int64(v)
			}
		}
	case DataTypeFloat:
		switch v := value.(type) {
		case int:
			return float64(v)
		case int8:
			return float64(v)
		case int16:
			return float64(v)
		case int32:
			return float64(v)
		case int64:
			return float64(v)
		case uint:
			return float64(v)
		case uint8:
			return float64(v)
		case uint16:
			return float64(v)
		case uint32:
			return float64(v)
		case uint64:
			return float64(v)
		}
	}
	return value
}

// @spec SPEC-STORE-003 v0.3.0
// TagsFor 는 사용자 관점 key 의 정적 태그 맵을 복사하여 반환한다.
// key 가 정적 키 목록에 없으면 빈 맵을 반환한다.
// 반환된 맵은 내부 저장소와 분리된 복사본이므로 호출자가 자유롭게 수정할 수 있다.
//
// v0.3.0 진화: 내부 staticKeys 가 StaticKeyMeta 로 변경되었으므로 .Tags 필드를 추출한다.
func (as *agentStore) TagsFor(key string) map[string]string {
	as.agent.mu.RLock()
	defer as.agent.mu.RUnlock()
	meta, ok := as.agent.config.staticKeys[key]
	if !ok {
		return map[string]string{}
	}
	out := make(map[string]string, len(meta.Tags))
	for k, v := range meta.Tags {
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
