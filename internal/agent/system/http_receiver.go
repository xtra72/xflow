package system

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// HTTPReceiverConfig 는 HTTP Receiver 에이전트의 설정이다.
type HTTPReceiverConfig struct {
	// ListenAddr 는 HTTP 서버의 수신 주소이다 (예: ":8081").
	ListenAddr string `json:"listen_addr"`

	// Path 는 수신 경로이다 (예: "/data").
	Path string `json:"path"`

	// Method 는 허용할 HTTP 메서드이다 (예: "POST").
	Method string `json:"method"`

	// TimeoutSec 는 요청 타임아웃(초)이다.
	TimeoutSec int `json:"timeout_sec"`

	// ContentType 는 허용할 Content-Type이다 (비어있으면 모두 허용).
	ContentType string `json:"content_type"`

	// BufferSize 는 수신 버퍼 크기이다.
	BufferSize int `json:"buffer_size"`

	// MaxBodyBytes 는 요청 본문의 최대 크기이다.
	MaxBodyBytes int64 `json:"max_body_bytes"`
}

// parseHTTPReceiverConfig 는 AgentConfig에서 HTTPReceiverConfig를 파싱한다.
func parseHTTPReceiverConfig(cfg agent.AgentConfig) HTTPReceiverConfig {
	hc := HTTPReceiverConfig{
		ListenAddr:   ":8080",
		Path:         "/",
		Method:       "POST",
		TimeoutSec:   30,
		BufferSize:   256,
		MaxBodyBytes: 1 << 20, // 1MB
	}

	opts := cfg.Transport.Options
	if opts == nil {
		return hc
	}

	if v, ok := opts["listen_addr"].(string); ok && v != "" {
		hc.ListenAddr = v
	}
	if v, ok := opts["path"].(string); ok && v != "" {
		hc.Path = v
	}
	if v, ok := opts["method"].(string); ok && v != "" {
		hc.Method = v
	}
	if v, ok := opts["timeout_sec"]; ok {
		hc.TimeoutSec = toInt(v)
	}
	if v, ok := opts["content_type"].(string); ok {
		hc.ContentType = v
	}
	if v, ok := opts["buffer_size"]; ok {
		hc.BufferSize = toInt(v)
	}
	if v, ok := opts["max_body_bytes"]; ok {
		hc.MaxBodyBytes = toInt64(v)
	}

	return hc
}

// HTTPReceiverAgent 는 HTTP 엔드포인트에서 데이터를 수신하는 에이전트이다.
// agent.Agent 와 agent.MessageReceiver 인터페이스를 구현한다.
type HTTPReceiverAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig   agent.AgentConfig
	httpConfig    HTTPReceiverConfig
	server        *http.Server
	listener      net.Listener
	recvCh        chan []byte
	done          chan struct{} // Stop 시그널
	stats         *agent.AgentStats
	droppedOnStop int64 // Stop시 드레인된 메시지 수
	mu            sync.RWMutex
	startedAt     time.Time
	createdAt     time.Time
}

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*HTTPReceiverAgent)(nil)
var _ agent.MessageReceiver = (*HTTPReceiverAgent)(nil)

// NewHTTPReceiverAgent 는 HTTPReceiverAgent 팩토리 함수이다.
func NewHTTPReceiverAgent(config agent.AgentConfig) (agent.Agent, error) {
	hc := parseHTTPReceiverConfig(config)

	a := &HTTPReceiverAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("http-receiver")),
		httpConfig:    hc,
		recvCh:        make(chan []byte, hc.BufferSize),
		done:          make(chan struct{}),
		stats:         agent.NewAgentStats(),
		createdAt:     time.Now(),
	}

	if err := a.Init(config); err != nil {
		return nil, err
	}

	return a, nil
}

// Init 은 에이전트를 초기화하고 HTTP 서버를 시작한다.
func (a *HTTPReceiverAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("http-receiver init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("http-receiver init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	// HTTP 서버 설정
	mux := http.NewServeMux()
	mux.HandleFunc(a.httpConfig.Path, a.handleRequest)

	a.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  time.Duration(a.httpConfig.TimeoutSec) * time.Second,
		WriteTimeout: time.Duration(a.httpConfig.TimeoutSec) * time.Second,
	}

	// 리스너를 미리 열어서 포트 바인딩 실패를 즉시 감지한다.
	ln, err := net.Listen("tcp", a.httpConfig.ListenAddr)
	if err != nil {
		_ = a.TransitionTo(lifecycle.StateError)
		return fmt.Errorf("http-receiver init: listen failed: %w", err)
	}
	a.listener = ln

	// HTTP 서버를 백그라운드에서 시작한다.
	go func() {
		if serveErr := a.server.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			_ = a.TransitionTo(lifecycle.StateError)
		}
	}()

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("http-receiver init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	return nil
}

// handleRequest 는 HTTP 요청을 처리하여 수신 채널에 전달한다.
func (a *HTTPReceiverAgent) handleRequest(w http.ResponseWriter, r *http.Request) {
	// 메서드 확인
	if a.httpConfig.Method != "" && r.Method != a.httpConfig.Method {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Content-Type 확인
	if a.httpConfig.ContentType != "" && r.Header.Get("Content-Type") != a.httpConfig.ContentType {
		http.Error(w, "unsupported content type", http.StatusUnsupportedMediaType)
		return
	}

	// 본문 읽기
	body := http.MaxBytesReader(w, r.Body, a.httpConfig.MaxBodyBytes)
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		a.stats.IncrMessagesErrored()
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// 채널에 전달 (비차단, 버퍼가 가득 차면 503 반환)
	select {
	case a.recvCh <- data:
		a.stats.IncrMessagesReceived()
		a.stats.AddBytesRead(int64(len(data)))
		a.stats.UpdateLastActivity()
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	default:
		a.stats.IncrMessagesErrored()
		http.Error(w, "buffer full", http.StatusServiceUnavailable)
	}
}

// ReceiveMessage 는 수신 채널에서 메시지를 가져온다.
// agent.MessageReceiver 인터페이스 구현.
// Stop()이 호출되면 즉시 에러를 반환한다.
func (a *HTTPReceiverAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case data := <-a.recvCh:
		return data, nil
	case <-a.done:
		return nil, fmt.Errorf("http-receiver: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Start 는 이미 Running 상태이면 no-op이다.
func (a *HTTPReceiverAgent) Start(_ context.Context) error {
	if a.CurrentState() == lifecycle.StateRunning {
		return nil
	}
	return fmt.Errorf("http-receiver start: not in running state (current: %s)", a.CurrentState())
}

// Stop 은 HTTP 서버를 종료하고, 남은 버퍼를 드레인하고, ReceiveMessage 대기자를 깨운다.
func (a *HTTPReceiverAgent) Stop(ctx context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("http-receiver stop: %w", err)
	}

	// 1. HTTP 서버 graceful shutdown (새 요청 거부)
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if a.server != nil {
		_ = a.server.Shutdown(shutdownCtx)
	}

	// 2. ReceiveMessage 대기자에게 종료 시그널
	close(a.done)

	// 3. 버퍼에 남은 메시지 드레인 및 카운트
	var dropped int64
	for {
		select {
		case <-a.recvCh:
			dropped++
		default:
			// 버퍼 비어있음
			a.mu.Lock()
			a.droppedOnStop = dropped
			a.mu.Unlock()

			if err := a.TransitionTo(lifecycle.StateStopped); err != nil {
				return fmt.Errorf("http-receiver stop: %w", err)
			}
			return nil
		}
	}
}

// Pause 는 Running -> Paused 전이한다.
func (a *HTTPReceiverAgent) Pause(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("http-receiver pause: %w", err)
	}
	return nil
}

// Resume 은 Paused -> Running 전이한다.
func (a *HTTPReceiverAgent) Resume(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("http-receiver resume: %w", err)
	}
	return nil
}

// Health 는 에이전트의 건강 상태를 반환한다.
func (a *HTTPReceiverAgent) Health() agent.HealthStatus {
	now := time.Now()
	state := a.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		return agent.HealthStatus{
			Status:    agent.HealthHealthy,
			LastCheck: now,
			Message:   "http-receiver is running",
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "http-receiver is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("http-receiver is in %s state", state),
		}
	}
}

// Process 는 HTTP Receiver에서는 사용하지 않는다 (수신 전용).
// BridgeOut 모드에서 Send를 호출하면 이 메서드가 호출되지만, receiver는 수신 전용이다.
func (a *HTTPReceiverAgent) Process(_ []byte) ([]byte, error) {
	return nil, nil
}

// Configure 는 에이전트 설정을 업데이트한다.
func (a *HTTPReceiverAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("http-receiver configure: %w", err)
	}
	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()
	return nil
}

// ID 는 에이전트 ID를 반환한다.
func (a *HTTPReceiverAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *HTTPReceiverAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *HTTPReceiverAgent) Type() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Type
}

// Info 는 에이전트 정보 스냅샷을 반환한다.
func (a *HTTPReceiverAgent) Info() agent.AgentInfo {
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
func (a *HTTPReceiverAgent) Stats() agent.StatsSnapshot {
	return a.stats.Snapshot()
}

// ListenAddr 는 실제 바인딩된 주소를 반환한다 (테스트용).
func (a *HTTPReceiverAgent) ListenAddr() string {
	if a.listener != nil {
		return a.listener.Addr().String()
	}
	return ""
}

// DroppedOnStop 은 Stop시 드레인된 메시지 수를 반환한다 (관측성).
func (a *HTTPReceiverAgent) DroppedOnStop() int64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.droppedOnStop
}

// 숫자 변환 헬퍼
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	default:
		return 0
	}
}
