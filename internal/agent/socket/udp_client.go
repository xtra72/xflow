package socket

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// UDPClientAgent 는 UDP 클라이언트 에이전트이다.
// 대상 서버에 데이터그램을 전송하고 응답을 수신한다.
type UDPClientAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig agent.AgentConfig
	config      UDPClientConfig
	conn        *net.UDPConn
	msgCh       chan []byte
	stats       *agent.AgentStats
	logger      *slog.Logger
	startedAt   time.Time
	createdAt   time.Time
	mu          sync.RWMutex
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

// 컴파일 타임 인터페이스 구현 확인.
var _ agent.Agent = (*UDPClientAgent)(nil)
var _ agent.MessageReceiver = (*UDPClientAgent)(nil)
var _ agent.StatefulAgent = (*UDPClientAgent)(nil)
var _ agent.BufferInfoProvider = (*UDPClientAgent)(nil)

// udpClientProcessCommand 는 UDP 클라이언트 Process 메서드의 JSON 요청 구조체이다.
type udpClientProcessCommand struct {
	Command string `json:"command"`
	Data    string `json:"data,omitempty"` // base64 인코딩된 데이터
}

// NewUDPClientAgent 는 새 UDPClientAgent 를 생성하고 초기화한다.
func NewUDPClientAgent(agentConfig agent.AgentConfig) (agent.Agent, error) {
	cfg, err := ParseUDPClientConfig(agentConfig.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("udp-client agent: %w", err)
	}

	logger := agentConfig.Logger
	if logger == nil {
		logger = slog.Default()
	}

	a := &UDPClientAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("udp-client-" + agentConfig.ID)),
		agentConfig:   agentConfig,
		config:        cfg,
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
func (a *UDPClientAgent) init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("udp-client agent init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("udp-client agent init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("udp-client agent init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	return nil
}

// Init 은 에이전트를 초기화한다 (Agent 인터페이스).
func (a *UDPClientAgent) Init(config agent.AgentConfig) error {
	return a.init(config)
}

// Start 는 대상 서버에 UDP 연결을 수립하고 recvLoop 를 시작한다.
func (a *UDPClientAgent) Start(_ context.Context) error {
	if a.CurrentState() != lifecycle.StateRunning {
		return fmt.Errorf("udp-client agent: not in running state (current: %s)", a.CurrentState())
	}

	serverAddr := &net.UDPAddr{
		IP:   net.ParseIP(a.config.Host),
		Port: a.config.Port,
	}
	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		return fmt.Errorf("udp-client agent: dial failed: %w", err)
	}

	a.mu.Lock()
	a.conn = conn
	a.mu.Unlock()

	a.wg.Add(1)
	go a.recvLoop()

	a.logger.Info("UDP 클라이언트 연결됨", "addr", serverAddr.String())
	return nil
}

// Stop 는 에이전트를 정지한다.
func (a *UDPClientAgent) Stop(_ context.Context) error {
	close(a.stopCh)

	a.mu.Lock()
	conn := a.conn
	a.conn = nil
	a.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}

	a.wg.Wait()

	current := a.CurrentState()
	switch current {
	case lifecycle.StateStopped:
		// 이미 정지됨
	default:
		_ = a.TransitionTo(lifecycle.StateStopping)
		_ = a.TransitionTo(lifecycle.StateStopped)
	}

	return nil
}

// Pause 는 에이전트를 일시 정지한다.
func (a *UDPClientAgent) Pause(_ context.Context) error {
	return a.TransitionTo(lifecycle.StatePaused)
}

// Resume 은 에이전트를 재개한다.
func (a *UDPClientAgent) Resume(_ context.Context) error {
	return a.TransitionTo(lifecycle.StateRunning)
}

// Health 는 에이전트의 헬스 상태를 반환한다.
func (a *UDPClientAgent) Health() agent.HealthStatus {
	now := time.Now()
	state := a.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		return agent.HealthStatus{
			Status:    agent.HealthHealthy,
			LastCheck: now,
			Message:   "udp-client agent is running",
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "udp-client agent is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("udp-client agent is in %s state", state),
		}
	}
}

// Process 는 JSON 명령을 처리한다.
// 지원 명령: send.
func (a *UDPClientAgent) Process(data []byte) ([]byte, error) {
	var cmd udpClientProcessCommand
	if err := json.Unmarshal(data, &cmd); err != nil {
		return nil, fmt.Errorf("udp-client process: invalid JSON: %w", err)
	}

	switch cmd.Command {
	case "send":
		return a.processSend(cmd)
	default:
		return nil, fmt.Errorf("udp-client process: unknown command %q", cmd.Command)
	}
}

// processSend 는 대상 서버에 데이터를 전송한다.
func (a *UDPClientAgent) processSend(cmd udpClientProcessCommand) ([]byte, error) {
	payload, err := base64.StdEncoding.DecodeString(cmd.Data)
	if err != nil {
		return nil, fmt.Errorf("udp-client send: invalid base64 data: %w", err)
	}

	a.mu.RLock()
	conn := a.conn
	a.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("udp-client send: not connected")
	}

	n, err := conn.Write(payload)
	if err != nil {
		a.stats.IncrMessagesErrored()
		return nil, fmt.Errorf("udp-client send: write failed: %w", err)
	}

	a.stats.IncrMessagesSent()
	a.stats.AddBytesWritten(int64(n))
	a.stats.UpdateLastActivity()

	return json.Marshal(map[string]any{"status": "sent"})
}

// Configure 는 에이전트 설정을 업데이트한다.
func (a *UDPClientAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("udp-client agent configure: %w", err)
	}
	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()
	return nil
}

// ReceiveMessage 는 수신된 메시지를 반환한다.
func (a *UDPClientAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case data := <-a.msgCh:
		return data, nil
	case <-a.stopCh:
		return nil, fmt.Errorf("udp-client agent: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// State 는 에이전트의 런타임 상태를 반환한다.
func (a *UDPClientAgent) State() map[string]any {
	state := map[string]any{
		"connected": false,
	}

	a.mu.RLock()
	if a.conn != nil {
		state["connected"] = true
		state["server_addr"] = a.conn.RemoteAddr().String()
	}
	a.mu.RUnlock()

	return state
}

// BufferInfo 는 메시지 버퍼 사용 현황을 반환한다.
func (a *UDPClientAgent) BufferInfo() (pending int, capacity int) {
	return len(a.msgCh), cap(a.msgCh)
}

// ID 는 에이전트 ID 를 반환한다.
func (a *UDPClientAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *UDPClientAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *UDPClientAgent) Type() string {
	return "udp-client"
}

// Info 는 에이전트 정보 스냅샷을 반환한다.
func (a *UDPClientAgent) Info() agent.AgentInfo {
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
		Type:      "udp-client",
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
func (a *UDPClientAgent) Stats() agent.StatsSnapshot {
	s := a.stats.Snapshot()
	s.MsgBufferPending, s.MsgBufferCapacity = a.BufferInfo()
	return s
}

// recvLoop 는 연결에서 응답 데이터를 읽어 msgCh 로 전달하는 고루틴이다.
func (a *UDPClientAgent) recvLoop() {
	defer a.wg.Done()

	buf := make([]byte, a.config.BufferSize)

	for {
		select {
		case <-a.stopCh:
			return
		default:
		}

		a.mu.RLock()
		conn := a.conn
		a.mu.RUnlock()

		if conn == nil {
			return
		}

		n, err := conn.Read(buf)
		if err != nil {
			// stopCh 가 닫혔으면 정상 종료.
			select {
			case <-a.stopCh:
				return
			default:
			}
			a.logger.Warn("UDP 클라이언트 읽기 오류", "error", err)
			return
		}

		data := make([]byte, n)
		copy(data, buf[:n])

		a.stats.IncrMessagesReceived()
		a.stats.AddBytesRead(int64(n))
		a.stats.UpdateLastActivity()

		select {
		case a.msgCh <- data:
		default:
			a.logger.Warn("UDP 클라이언트 메시지 버퍼 가득 참, 드롭")
		}
	}
}
