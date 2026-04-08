package system

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// HTTPSenderConfig 는 HTTP Sender 에이전트의 설정이다.
type HTTPSenderConfig struct {
	// URL 는 전송 대상 URL이다.
	URL string `json:"url"`

	// Method 는 HTTP 메서드이다 (기본: POST).
	Method string `json:"method"`

	// ContentType 는 요청의 Content-Type이다 (기본: application/json).
	ContentType string `json:"content_type"`

	// TimeoutSec 는 요청 타임아웃(초)이다.
	TimeoutSec int `json:"timeout_sec"`

	// Headers 는 추가 HTTP 헤더이다.
	Headers map[string]string `json:"headers"`
}

// parseHTTPSenderConfig 는 AgentConfig에서 HTTPSenderConfig를 파싱한다.
func parseHTTPSenderConfig(cfg agent.AgentConfig) HTTPSenderConfig {
	sc := HTTPSenderConfig{
		Method:      "POST",
		ContentType: "application/json",
		TimeoutSec:  30,
	}

	opts := cfg.Transport.Options
	if opts == nil {
		return sc
	}

	if v, ok := opts["url"].(string); ok && v != "" {
		sc.URL = v
	}
	if v, ok := opts["method"].(string); ok && v != "" {
		sc.Method = v
	}
	if v, ok := opts["content_type"].(string); ok && v != "" {
		sc.ContentType = v
	}
	if v, ok := opts["timeout_sec"]; ok {
		sc.TimeoutSec = toInt(v)
	}
	if v, ok := opts["headers"].(map[string]any); ok {
		sc.Headers = make(map[string]string)
		for k, val := range v {
			if s, ok := val.(string); ok {
				sc.Headers[k] = s
			}
		}
	}

	return sc
}

// HTTPSenderAgent 는 HTTP 요청으로 데이터를 전송하는 에이전트이다.
type HTTPSenderAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig agent.AgentConfig
	httpConfig  HTTPSenderConfig
	client      *http.Client
	stats       *agent.AgentStats
	mu          sync.RWMutex
	startedAt   time.Time
	createdAt   time.Time
}

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*HTTPSenderAgent)(nil)

// NewHTTPSenderAgent 는 HTTPSenderAgent 팩토리 함수이다.
func NewHTTPSenderAgent(config agent.AgentConfig) (agent.Agent, error) {
	sc := parseHTTPSenderConfig(config)

	a := &HTTPSenderAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("http-sender")),
		httpConfig:    sc,
		stats:         agent.NewAgentStats(),
		createdAt:     time.Now(),
	}

	if err := a.Init(config); err != nil {
		return nil, err
	}

	return a, nil
}

// Init 은 에이전트를 초기화하고 HTTP 클라이언트를 설정한다.
func (a *HTTPSenderAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("http-sender init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("http-sender init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	// HTTP 클라이언트 설정
	a.client = &http.Client{
		Timeout: time.Duration(a.httpConfig.TimeoutSec) * time.Second,
	}

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("http-sender init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	return nil
}

// Start 는 이미 Running 상태이면 no-op이다.
func (a *HTTPSenderAgent) Start(_ context.Context) error {
	switch a.CurrentState() {
	case lifecycle.StateRunning:
		return nil
	case lifecycle.StateStopped:
		if err := a.TransitionTo(lifecycle.StateCreated); err != nil {
			return fmt.Errorf("http-sender start: reset to created: %w", err)
		}
		a.mu.RLock()
		cfg := a.agentConfig
		a.mu.RUnlock()
		return a.Init(cfg)
	default:
		return fmt.Errorf("http-sender start: not in running state (current: %s)", a.CurrentState())
	}
}

// Stop 은 HTTP 클라이언트를 정리하고 종료한다.
func (a *HTTPSenderAgent) Stop(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("http-sender stop: %w", err)
	}

	// 진행 중인 요청을 대기하기 위해 CloseIdleConnections 호출
	if a.client != nil {
		a.client.CloseIdleConnections()
	}

	if err := a.TransitionTo(lifecycle.StateStopped); err != nil {
		return fmt.Errorf("http-sender stop: %w", err)
	}

	return nil
}

// Pause 는 Running -> Paused 전이한다.
func (a *HTTPSenderAgent) Pause(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("http-sender pause: %w", err)
	}
	return nil
}

// Resume 은 Paused -> Running 전이한다.
func (a *HTTPSenderAgent) Resume(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("http-sender resume: %w", err)
	}
	return nil
}

// Health 는 에이전트의 건강 상태를 반환한다.
func (a *HTTPSenderAgent) Health() agent.HealthStatus {
	now := time.Now()
	state := a.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		return agent.HealthStatus{
			Status:    agent.HealthHealthy,
			LastCheck: now,
			Message:   "http-sender is running",
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "http-sender is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("http-sender is in %s state", state),
		}
	}
}

// Process 는 데이터를 HTTP 요청으로 전송한다.
func (a *HTTPSenderAgent) Process(data []byte) ([]byte, error) {
	if a.httpConfig.URL == "" {
		a.stats.IncrExternalMessagesErrored()
		return nil, fmt.Errorf("http-sender process: URL is not configured")
	}

	req, err := http.NewRequest(a.httpConfig.Method, a.httpConfig.URL, bytes.NewReader(data))
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		return nil, fmt.Errorf("http-sender process: create request failed: %w", err)
	}

	req.Header.Set("Content-Type", a.httpConfig.ContentType)
	for k, v := range a.httpConfig.Headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		return nil, fmt.Errorf("http-sender process: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		return nil, fmt.Errorf("http-sender process: read response failed: %w", err)
	}

	a.stats.IncrExternalMessagesSent()
	a.stats.AddBytesWritten(int64(len(data)))
	a.stats.UpdateLastActivity()

	if resp.StatusCode >= 400 {
		a.stats.IncrExternalMessagesErrored()
		return respBody, fmt.Errorf("http-sender process: server returned %d", resp.StatusCode)
	}

	return respBody, nil
}

// Configure 는 에이전트 설정을 업데이트한다.
func (a *HTTPSenderAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("http-sender configure: %w", err)
	}
	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()
	return nil
}

// ID 는 에이전트 ID를 반환한다.
func (a *HTTPSenderAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *HTTPSenderAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *HTTPSenderAgent) Type() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Type
}

// Info 는 에이전트 정보 스냅샷을 반환한다.
func (a *HTTPSenderAgent) Info() agent.AgentInfo {
	a.mu.RLock()
	cfg := a.agentConfig
	startedAt := a.startedAt
	createdAt := a.createdAt
	a.mu.RUnlock()

	state := a.CurrentState()
	var uptime time.Duration
	if state == lifecycle.StateRunning && !startedAt.IsZero() {
		uptime = time.Since(startedAt)
	}

	return agent.AgentInfo{
		ID:        cfg.ID,
		Name:      cfg.Name,
		Type:      cfg.Type,
		State:     state,
		Health:    a.Health(),
		Config:    cfg,
		Stats:     a.stats.Snapshot(),
		StartedAt: startedAt,
		Uptime:    uptime,
		CreatedAt: createdAt,
	}
}

// Stats 는 통계 스냅샷을 반환한다.
func (a *HTTPSenderAgent) Stats() agent.StatsSnapshot {
	return a.stats.Snapshot()
}
