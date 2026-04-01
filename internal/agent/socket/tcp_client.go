package socket

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// TCPClientAgent 는 TCP 클라이언트 에이전트이다.
// 서버에 연결하여 데이터를 송수신하며, 자동 재연결을 지원한다.
type TCPClientAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig     agent.AgentConfig
	config          TCPClientConfig
	conn            net.Conn
	framer          Framer
	msgCh           chan []byte
	stats           *agent.AgentStats
	logger          *slog.Logger
	startedAt       time.Time
	createdAt       time.Time
	mu              sync.RWMutex
	connected       atomic.Bool
	stopCh          chan struct{}
	stopOnce        sync.Once
	wg              sync.WaitGroup
	reconnectCancel context.CancelFunc
}

// 컴파일 타임 인터페이스 구현 확인.
var _ agent.Agent = (*TCPClientAgent)(nil)
var _ agent.MessageReceiver = (*TCPClientAgent)(nil)
var _ agent.StatefulAgent = (*TCPClientAgent)(nil)
var _ agent.TransportChecker = (*TCPClientAgent)(nil)
var _ agent.BufferInfoProvider = (*TCPClientAgent)(nil)

// NewTCPClientAgent 는 새 TCPClientAgent 를 생성하고 초기화한다.
func NewTCPClientAgent(agentConfig agent.AgentConfig) (agent.Agent, error) {
	cfg, err := ParseTCPClientConfig(agentConfig.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("tcp client agent: %w", err)
	}

	framer, err := NewFramer(cfg.Framing, FramerOptions{
		BufferSize:     cfg.BufferSize,
		Delimiter:      cfg.Delimiter,
		FixedSize:      cfg.FixedSize,
		MaxMessageSize: cfg.MaxMessageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("tcp client agent: %w", err)
	}

	logger := agentConfig.Logger
	if logger == nil {
		logger = slog.Default()
	}

	a := &TCPClientAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("tcp-client-" + agentConfig.ID)),
		agentConfig:   agentConfig,
		config:        cfg,
		framer:        framer,
		msgCh:         make(chan []byte, 256),
		stats:         agent.NewAgentStats(),
		logger:        logger,
		createdAt:     time.Now(),
		stopCh:        make(chan struct{}),
	}

	if err := a.init(agentConfig); err != nil {
		return nil, err
	}

	return a, nil
}

// init 은 에이전트를 초기화하고 Running 상태로 전이한다.
func (a *TCPClientAgent) init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("tcp client agent init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("tcp client agent init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("tcp client agent init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	return nil
}

// Init 은 에이전트를 초기화한다 (Agent 인터페이스).
func (a *TCPClientAgent) Init(config agent.AgentConfig) error {
	return a.init(config)
}

// Start 는 서버 연결을 시작한다.
// 연결 성공 시 readLoop 를 시작하고, 실패 시 reconnectLoop 를 시작한다.
func (a *TCPClientAgent) Start(_ context.Context) error {
	if a.CurrentState() != lifecycle.StateRunning {
		return fmt.Errorf("tcp client agent: not in running state (current: %s)", a.CurrentState())
	}

	err := a.connect()
	if err != nil {
		a.logger.Warn("TCP 클라이언트 초기 연결 실패, 재연결 시작",
			"addr", a.serverAddr(),
			"error", err)
		a.startReconnectLoop()
	}

	return nil
}

// serverAddr 는 서버 주소 문자열을 반환한다.
func (a *TCPClientAgent) serverAddr() string {
	return fmt.Sprintf("%s:%d", a.config.Host, a.config.Port)
}

// connect 는 TCP 서버에 연결을 시도한다.
func (a *TCPClientAgent) connect() error {
	addr := a.serverAddr()
	conn, err := net.DialTimeout("tcp", addr, a.config.ConnectTimeout)
	if err != nil {
		return err
	}

	a.mu.Lock()
	a.conn = conn
	a.mu.Unlock()

	a.connected.Store(true)

	a.logger.Info("TCP 클라이언트 연결됨", "addr", addr)

	a.wg.Add(1)
	go a.readLoop()

	return nil
}

// readLoop 는 연결에서 데이터를 읽어 msgCh 로 전달하는 고루틴이다.
func (a *TCPClientAgent) readLoop() {
	defer a.wg.Done()

	a.mu.RLock()
	conn := a.conn
	a.mu.RUnlock()

	reader := NewConnReader(a.framer, conn)

	for {
		select {
		case <-a.stopCh:
			return
		default:
		}

		data, err := reader.Read()
		if err != nil {
			// stopCh 가 닫혔으면 정상 종료
			select {
			case <-a.stopCh:
				return
			default:
			}

			a.connected.Store(false)
			a.logger.Warn("TCP 클라이언트 읽기 오류, 재연결 시작", "error", err)
			a.startReconnectLoop()
			return
		}

		a.stats.IncrMessagesReceived()
		a.stats.AddBytesRead(int64(len(data)))
		a.stats.UpdateLastActivity()

		// msgCh 에 비차단 전송 (가득 차면 드롭)
		select {
		case a.msgCh <- data:
		default:
			a.logger.Warn("TCP 클라이언트 메시지 버퍼 가득 참, 드롭")
		}
	}
}

// startReconnectLoop 는 재연결 루프 고루틴을 시작한다.
func (a *TCPClientAgent) startReconnectLoop() {
	a.wg.Add(1)
	go a.reconnectLoop()
}

// reconnectLoop 는 백그라운드에서 재연결을 시도하는 고루틴이다.
// 지수 백오프 + 지터(+-20%)를 사용한다.
func (a *TCPClientAgent) reconnectLoop() {
	defer a.wg.Done()

	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.reconnectCancel = cancel
	a.mu.Unlock()
	defer cancel()

	attempt := 0

	for {
		// stopCh 확인
		select {
		case <-a.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		if attempt > 0 {
			backoff := a.calculateBackoff(attempt)

			select {
			case <-a.stopCh:
				return
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}

		// 연결 시도
		err := a.connect()
		if err == nil {
			a.logger.Info("TCP 클라이언트 재연결 성공", "attempts", attempt+1)
			return
		}

		attempt++
		a.logger.Warn("TCP 클라이언트 재연결 실패",
			"attempt", attempt,
			"error", err)

		// 최대 재시도 확인
		if a.config.MaxRetries > 0 && attempt >= a.config.MaxRetries {
			a.logger.Error("TCP 클라이언트 최대 재시도 도달",
				"max_retries", a.config.MaxRetries)
			_ = a.TransitionTo(lifecycle.StateError)
			return
		}
	}
}

// calculateBackoff 는 지수 백오프 + 지터를 계산한다.
// base * 2^(attempt-1), 최대 60초, +-20% 지터.
func (a *TCPClientAgent) calculateBackoff(attempt int) time.Duration {
	base := a.config.ReconnectInterval
	multiplier := math.Pow(2, float64(attempt-1))
	backoff := time.Duration(float64(base) * multiplier)

	maxBackoff := 60 * time.Second
	if backoff > maxBackoff {
		backoff = maxBackoff
	}

	// +-20% 지터
	jitter := float64(backoff) * 0.2 * (2*rand.Float64() - 1)
	backoff = time.Duration(float64(backoff) + jitter)

	if backoff < 0 {
		backoff = time.Millisecond
	}

	return backoff
}

// Stop 는 에이전트를 정지한다.
func (a *TCPClientAgent) Stop(_ context.Context) error {
	a.stopOnce.Do(func() {
		close(a.stopCh)
	})

	// 재연결 루프 취소
	a.mu.RLock()
	cancelFn := a.reconnectCancel
	a.mu.RUnlock()
	if cancelFn != nil {
		cancelFn()
	}

	// 연결 닫기
	a.mu.Lock()
	conn := a.conn
	a.conn = nil
	a.mu.Unlock()

	if conn != nil {
		conn.Close()
	}

	a.connected.Store(false)

	// 고루틴 대기
	a.wg.Wait()

	// 상태 전이: Error -> Stopped 가능, Running -> Stopping -> Stopped
	current := a.CurrentState()
	switch current {
	case lifecycle.StateError:
		_ = a.TransitionTo(lifecycle.StateStopped)
	case lifecycle.StateStopped:
		// 이미 정지됨
	default:
		_ = a.TransitionTo(lifecycle.StateStopping)
		_ = a.TransitionTo(lifecycle.StateStopped)
	}

	return nil
}

// Pause 는 에이전트를 일시 정지한다.
func (a *TCPClientAgent) Pause(_ context.Context) error {
	return a.TransitionTo(lifecycle.StatePaused)
}

// Resume 은 에이전트를 재개한다.
func (a *TCPClientAgent) Resume(_ context.Context) error {
	return a.TransitionTo(lifecycle.StateRunning)
}

// Health 는 에이전트의 헬스 상태를 반환한다.
func (a *TCPClientAgent) Health() agent.HealthStatus {
	now := time.Now()
	state := a.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		if a.connected.Load() {
			return agent.HealthStatus{
				Status:    agent.HealthHealthy,
				LastCheck: now,
				Message:   "connected to server",
			}
		}
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "reconnecting to server",
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "agent is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("agent is in %s state", state),
		}
	}
}

// Process 는 데이터를 서버로 전송한다.
func (a *TCPClientAgent) Process(data []byte) ([]byte, error) {
	if !a.connected.Load() {
		return nil, fmt.Errorf("tcp client agent: not connected")
	}

	a.mu.RLock()
	conn := a.conn
	a.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("tcp client agent: connection is nil")
	}

	err := a.framer.Write(conn, data)
	if err != nil {
		a.stats.IncrMessagesErrored()
		return nil, fmt.Errorf("tcp client agent: write failed: %w", err)
	}

	a.stats.IncrMessagesSent()
	a.stats.AddBytesWritten(int64(len(data)))
	a.stats.UpdateLastActivity()

	return nil, nil
}

// Configure 는 에이전트 설정을 업데이트한다.
func (a *TCPClientAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("tcp client agent configure: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	return nil
}

// ReceiveMessage 는 수신된 메시지를 반환한다.
func (a *TCPClientAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case data := <-a.msgCh:
		return data, nil
	case <-a.stopCh:
		return nil, fmt.Errorf("tcp client agent: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// State 는 에이전트의 런타임 상태를 반환한다.
func (a *TCPClientAgent) State() map[string]any {
	state := map[string]any{
		"connected": a.connected.Load(),
	}

	a.mu.RLock()
	if a.conn != nil {
		state["server_addr"] = a.conn.RemoteAddr().String()
	}
	a.mu.RUnlock()

	return state
}

// TransportConnected 는 연결 여부를 반환한다.
func (a *TCPClientAgent) TransportConnected() bool {
	return a.connected.Load()
}

// BufferInfo 는 메시지 버퍼 사용 현황을 반환한다.
func (a *TCPClientAgent) BufferInfo() (pending int, capacity int) {
	return len(a.msgCh), cap(a.msgCh)
}

// ID 는 에이전트 ID 를 반환한다.
func (a *TCPClientAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *TCPClientAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *TCPClientAgent) Type() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Type
}

// Info 는 에이전트 정보 스냅샷을 반환한다.
func (a *TCPClientAgent) Info() agent.AgentInfo {
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
func (a *TCPClientAgent) Stats() agent.StatsSnapshot {
	return a.stats.Snapshot()
}
