package system

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/tsdb"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// TSDBConfig 는 TSDB 에이전트의 설정이다.
type TSDBConfig struct {
	// MaxSeries 는 최대 시리즈 수이다.
	MaxSeries int `json:"max_series"`

	// MaxPointsPerSeries 는 시리즈당 최대 포인트 수이다.
	MaxPointsPerSeries int `json:"max_points_per_series"`

	// MaxMemoryBytes 는 최대 메모리 사용량(바이트)이다.
	MaxMemoryBytes int64 `json:"max_memory_bytes"`

	// MaxAge 는 데이터 보존 기간이다.
	MaxAge time.Duration `json:"max_age"`

	// EvictionInterval 은 퇴거 실행 간격이다.
	EvictionInterval time.Duration `json:"eviction_interval"`

	// MaxQueryPoints 는 쿼리당 최대 반환 포인트 수이다.
	MaxQueryPoints int `json:"max_query_points"`

	// QueryTimeout 은 쿼리 타임아웃이다.
	QueryTimeout time.Duration `json:"query_timeout"`
}

// parseTSDBConfig 는 AgentConfig에서 TSDBConfig를 파싱한다.
func parseTSDBConfig(cfg agent.AgentConfig) TSDBConfig {
	tc := TSDBConfig{
		MaxSeries:          10000,
		MaxPointsPerSeries: 100000,
		MaxMemoryBytes:     256 * 1024 * 1024, // 256MB
		MaxAge:             24 * time.Hour,
		EvictionInterval:   30 * time.Second,
		MaxQueryPoints:     10000,
		QueryTimeout:       10 * time.Second,
	}

	opts := cfg.Transport.Options
	if opts == nil {
		return tc
	}

	if v, ok := opts["max_series"]; ok {
		tc.MaxSeries = toInt(v)
	}
	if v, ok := opts["max_points_per_series"]; ok {
		tc.MaxPointsPerSeries = toInt(v)
	}
	if v, ok := opts["max_memory_bytes"]; ok {
		switch n := v.(type) {
		case int:
			tc.MaxMemoryBytes = int64(n)
		case int64:
			tc.MaxMemoryBytes = n
		case float64:
			tc.MaxMemoryBytes = int64(n)
		}
	}
	if v, ok := opts["max_memory_mb"]; ok {
		switch n := v.(type) {
		case int:
			tc.MaxMemoryBytes = int64(n) * 1024 * 1024
		case float64:
			tc.MaxMemoryBytes = int64(n) * 1024 * 1024
		}
	}
	if v, ok := opts["max_age"].(string); ok {
		if d, err := time.ParseDuration(v); err == nil {
			tc.MaxAge = d
		}
	}
	if v, ok := opts["eviction_interval"].(string); ok {
		if d, err := time.ParseDuration(v); err == nil {
			tc.EvictionInterval = d
		}
	}
	if v, ok := opts["max_query_points"]; ok {
		tc.MaxQueryPoints = toInt(v)
	}
	if v, ok := opts["query_timeout"].(string); ok {
		if d, err := time.ParseDuration(v); err == nil {
			tc.QueryTimeout = d
		}
	}

	return tc
}

// toTSDBConfig 는 TSDBConfig를 tsdb.Config로 변환한다.
func toTSDBConfig(tc TSDBConfig) tsdb.Config {
	cfg := tsdb.DefaultConfig()
	if tc.MaxSeries > 0 {
		cfg.MaxSeries = tc.MaxSeries
	}
	if tc.MaxPointsPerSeries > 0 {
		cfg.MaxPointsPerSeries = tc.MaxPointsPerSeries
	}
	if tc.MaxMemoryBytes > 0 {
		cfg.MaxMemoryBytes = tc.MaxMemoryBytes
	}
	if tc.MaxAge > 0 {
		cfg.MaxAge = tc.MaxAge
	}
	if tc.EvictionInterval > 0 {
		cfg.EvictionInterval = tc.EvictionInterval
	}
	if tc.MaxQueryPoints > 0 {
		cfg.MaxQueryPoints = tc.MaxQueryPoints
	}
	if tc.QueryTimeout > 0 {
		cfg.QueryTimeout = tc.QueryTimeout
	}
	return cfg
}

// TSDBAgent 는 인메모리 시계열 데이터베이스를 관리하는 시스템 에이전트이다.
// tsdb-write, tsdb-query 노드가 AgentResolver를 통해 이 에이전트를 찾아
// TSDB() 메서드로 TSDB 인스턴스에 접근한다.
type TSDBAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig agent.AgentConfig
	tsdbConfig  TSDBConfig
	db          tsdb.TSDB
	logger      *slog.Logger
	stats       *agent.AgentStats
	mu          sync.RWMutex
	startedAt   time.Time
	createdAt   time.Time
}

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*TSDBAgent)(nil)
var _ agent.StatefulAgent = (*TSDBAgent)(nil)

// NewTSDBAgent 는 TSDBAgent 팩토리 함수이다.
func NewTSDBAgent(config agent.AgentConfig) (agent.Agent, error) {
	tc := parseTSDBConfig(config)

	a := &TSDBAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("tsdb")),
		agentConfig:   config,
		tsdbConfig:    tc,
		logger:        agent.ResolveLogger(config),
		stats:         agent.NewAgentStats(),
		createdAt:     time.Now(),
	}

	if err := a.Init(config); err != nil {
		return nil, err
	}

	return a, nil
}

// TSDB 는 내부 TSDB 인스턴스를 반환한다.
// tsdb-write, tsdb-query 노드가 이 메서드를 통해 TSDB에 접근한다.
func (a *TSDBAgent) TSDB() tsdb.TSDB {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.db
}

// Init 은 에이전트를 초기화하고 TSDB 인스턴스를 생성한다.
func (a *TSDBAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("tsdb init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("tsdb init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.db = tsdb.New(toTSDBConfig(a.tsdbConfig))
	a.mu.Unlock()

	a.logger.Info("tsdb 에이전트 초기화 완료",
		"max_series", a.tsdbConfig.MaxSeries,
		"max_memory_mb", a.tsdbConfig.MaxMemoryBytes/(1024*1024),
		"max_age", a.tsdbConfig.MaxAge,
	)

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("tsdb init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	return nil
}

// Start 는 에이전트를 시작한다. 이미 Running이면 no-op.
func (a *TSDBAgent) Start(_ context.Context) error {
	switch a.CurrentState() {
	case lifecycle.StateRunning:
		return nil
	case lifecycle.StateStopped:
		if err := a.TransitionTo(lifecycle.StateCreated); err != nil {
			return fmt.Errorf("tsdb start: reset to created: %w", err)
		}
		a.mu.RLock()
		cfg := a.agentConfig
		a.mu.RUnlock()
		return a.Init(cfg)
	default:
		return fmt.Errorf("tsdb: not in running state (current: %s)", a.CurrentState())
	}
}

// Stop 은 에이전트를 정지하고 TSDB를 닫는다.
func (a *TSDBAgent) Stop(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("tsdb stop: %w", err)
	}

	a.mu.Lock()
	if a.db != nil {
		a.db.Close()
		a.db = nil
	}
	a.mu.Unlock()

	a.logger.Info("tsdb 에이전트 정지 완료")

	return a.TransitionTo(lifecycle.StateStopped)
}

// Pause 는 에이전트를 일시정지한다.
func (a *TSDBAgent) Pause(_ context.Context) error {
	return a.TransitionTo(lifecycle.StatePaused)
}

// Resume 은 에이전트를 재개한다.
func (a *TSDBAgent) Resume(_ context.Context) error {
	return a.TransitionTo(lifecycle.StateRunning)
}

// Health 는 에이전트의 헬스 상태를 반환한다.
func (a *TSDBAgent) Health() agent.HealthStatus {
	state := a.CurrentState()
	if state == lifecycle.StateRunning || state == lifecycle.StatePaused {
		return agent.HealthStatus{Status: agent.HealthHealthy}
	}
	return agent.HealthStatus{Status: agent.HealthUnhealthy}
}

// Process 는 no-op이다. TSDB는 노드가 직접 TSDB() 인스턴스를 사용한다.
func (a *TSDBAgent) Process(_ []byte) ([]byte, error) {
	return nil, nil
}

// Configure 는 에이전트 설정을 변경한다.
func (a *TSDBAgent) Configure(config agent.AgentConfig) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.agentConfig = config
	a.tsdbConfig = parseTSDBConfig(config)
	return nil
}

// ID 는 에이전트의 고유 식별자를 반환한다.
func (a *TSDBAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트의 이름을 반환한다.
func (a *TSDBAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트의 타입을 반환한다.
func (a *TSDBAgent) Type() string {
	return "tsdb"
}

// Info 는 에이전트의 상세 정보를 반환한다.
func (a *TSDBAgent) Info() agent.AgentInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var uptime time.Duration
	if !a.startedAt.IsZero() {
		uptime = time.Since(a.startedAt)
	}

	return agent.AgentInfo{
		ID:     a.agentConfig.ID,
		Name:   a.agentConfig.Name,
		Type:   "tsdb",
		State:  a.CurrentState(),
		Config: a.agentConfig,
		Stats:  a.stats.Snapshot(),
		Uptime: uptime,
	}
}

// Stats 는 에이전트의 처리 통계를 반환한다.
func (a *TSDBAgent) Stats() agent.StatsSnapshot {
	return a.stats.Snapshot()
}

// State 는 TSDB의 런타임 상태를 반환한다 (StatefulAgent 구현).
func (a *TSDBAgent) State() map[string]any {
	a.mu.RLock()
	db := a.db
	a.mu.RUnlock()

	if db == nil {
		return map[string]any{"status": "closed"}
	}

	s := db.Stats()
	return map[string]any{
		"status":        "running",
		"series_count":  s.SeriesCount,
		"total_points":  s.TotalPoints,
		"memory_bytes":  s.MemoryBytes,
		"max_series":    a.tsdbConfig.MaxSeries,
		"max_memory_mb": a.tsdbConfig.MaxMemoryBytes / (1024 * 1024),
	}
}
