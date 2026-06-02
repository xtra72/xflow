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

// connMessage 는 연결 정보를 포함하는 내부 메시지 구조체이다.
type connMessage struct {
	Data       []byte
	RemoteAddr string
}

// TCPServerAgent 는 TCP 서버 에이전트이다.
// 클라이언트 연결을 수락하고 프레이밍에 따라 메시지를 수신한다.
type TCPServerAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig agent.AgentConfig
	config      TCPServerConfig
	listener    net.Listener
	connections ConnectionManager
	framer      Framer
	msgCh       chan connMessage
	pauseBuf    []connMessage // 일시정지 중 버퍼링된 메시지
	stats       *agent.AgentStats
	logger      *slog.Logger
	startedAt   time.Time
	createdAt   time.Time
	mu          sync.RWMutex
	paused      bool
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*TCPServerAgent)(nil)
var _ agent.MessageReceiver = (*TCPServerAgent)(nil)
var _ agent.StatefulAgent = (*TCPServerAgent)(nil)
var _ agent.TransportChecker = (*TCPServerAgent)(nil)
var _ agent.BufferInfoProvider = (*TCPServerAgent)(nil)
var _ agent.ConnAwareReceiver = (*TCPServerAgent)(nil)

// processCommand 는 Process 메서드의 JSON 요청 구조체이다.
type processCommand struct {
	Command string `json:"command"`
	Target  string `json:"target,omitempty"`
	Data    string `json:"data,omitempty"`    // base64 인코딩된 데이터
	Address string `json:"address,omitempty"` // block/unblock 대상 주소
}

// NewTCPServerAgent 는 새 TCPServerAgent 를 생성하고 초기화한다.
func NewTCPServerAgent(agentConfig agent.AgentConfig) (agent.Agent, error) {
	cfg, err := ParseTCPServerConfig(agentConfig.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("tcp-server agent: %w", err)
	}

	framer, err := NewFramer(cfg.Framing, FramerOptions{
		BufferSize:     cfg.BufferSize,
		Delimiter:      cfg.Delimiter,
		FixedSize:      cfg.FixedSize,
		MaxMessageSize: cfg.MaxMessageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("tcp-server agent: %w", err)
	}

	a := &TCPServerAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("tcp-server")),
		config:        cfg,
		connections:   NewConnectionManager(cfg.MaxConnections),
		framer:        framer,
		msgCh:         make(chan connMessage, 256),
		stats:         agent.NewAgentStats(),
		logger:        agent.ResolveLogger(agentConfig),
		createdAt:     time.Now(),
		stopCh:        make(chan struct{}),
	}

	if err := a.Init(agentConfig); err != nil {
		return nil, err
	}

	return a, nil
}

// Init 은 에이전트를 초기화한다.
func (a *TCPServerAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("tcp-server init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("tcp-server init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("tcp-server init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	return nil
}

// Start 는 TCP 리스너를 시작하고 acceptLoop 고루틴을 실행한다.
func (a *TCPServerAgent) Start(_ context.Context) error {
	if a.CurrentState() != lifecycle.StateRunning {
		return fmt.Errorf("tcp-server start: agent is not in running state (current: %s)", a.CurrentState())
	}

	addr := fmt.Sprintf("%s:%d", a.config.Host, a.config.Port)
	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		return fmt.Errorf("tcp-server start: %w", err)
	}

	a.mu.Lock()
	a.listener = ln
	a.mu.Unlock()

	a.wg.Add(1)
	go a.acceptLoop()

	a.logger.Info("tcp-server: 리스너 시작", "addr", ln.Addr().String())
	return nil
}

// Stop 은 리스너를 닫고 모든 연결을 종료한 후 고루틴들이 완료되기를 기다린다.
func (a *TCPServerAgent) Stop(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("tcp-server stop: %w", err)
	}

	close(a.stopCh)

	a.mu.Lock()
	ln := a.listener
	a.listener = nil
	a.mu.Unlock()

	if ln != nil {
		_ = ln.Close()
	}

	_ = a.connections.CloseAll()

	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 고루틴이 정상 종료됨
	case <-time.After(10 * time.Second):
		a.logger.Warn("tcp-server: stop timeout, forcing shutdown")
	}

	if err := a.TransitionTo(lifecycle.StateStopped); err != nil {
		return fmt.Errorf("tcp-server stop: %w", err)
	}

	return nil
}

// Pause 는 Running -> Paused 로 전환한다.
// 일시정지 중 수신된 데이터는 pauseBuf 에 버퍼링된다.
func (a *TCPServerAgent) Pause(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("tcp-server pause: %w", err)
	}
	a.mu.Lock()
	a.paused = true
	a.mu.Unlock()
	return nil
}

// Resume 은 Paused -> Running 으로 전환하고 버퍼링된 메시지를 flush 한다.
func (a *TCPServerAgent) Resume(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("tcp-server resume: %w", err)
	}

	a.mu.Lock()
	a.paused = false
	buf := a.pauseBuf
	a.pauseBuf = nil
	a.mu.Unlock()

	// 버퍼링된 메시지를 msgCh 로 flush.
	for _, cm := range buf {
		select {
		case a.msgCh <- cm:
		default:
			a.logger.Warn("tcp-server: msgCh full during flush, dropping message")
		}
	}

	return nil
}

// Health 는 에이전트의 건강 상태를 반환한다.
func (a *TCPServerAgent) Health() agent.HealthStatus {
	now := time.Now()
	state := a.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		return agent.HealthStatus{
			Status:    agent.HealthHealthy,
			LastCheck: now,
			Message:   "tcp-server agent is running",
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "tcp-server agent is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("tcp-server agent is in %s state", state),
		}
	}
}

// Process 는 JSON 명령을 처리한다.
// 지원 명령: send, list_connections, block, unblock.
func (a *TCPServerAgent) Process(data []byte) ([]byte, error) {
	var cmd processCommand
	if err := json.Unmarshal(data, &cmd); err != nil {
		return nil, fmt.Errorf("tcp-server process: invalid JSON: %w", err)
	}

	switch cmd.Command {
	case "send":
		return a.processSend(cmd)
	case "list_connections":
		return a.processListConnections()
	case "block":
		return nil, a.connections.Block(cmd.Address)
	case "unblock":
		return nil, a.connections.Unblock(cmd.Address)
	default:
		return nil, fmt.Errorf("tcp-server process: unknown command %q", cmd.Command)
	}
}

// processSend 는 특정 연결 또는 모든 연결에 데이터를 전송한다.
func (a *TCPServerAgent) processSend(cmd processCommand) ([]byte, error) {
	payload, err := base64.StdEncoding.DecodeString(cmd.Data)
	if err != nil {
		return nil, fmt.Errorf("tcp-server send: invalid base64 data: %w", err)
	}

	// Broadcast 옵션이 켜져 있으면 Target 지정과 무관하게 모든 연결로 전송한다.
	// Target 이 비어 있을 때도 기존과 동일하게 모든 연결로 전송한다.
	if a.config.Broadcast || cmd.Target == "" {
		// Broadcast to all connections.
		list := a.connections.List()
		for i := range list {
			addr := list[i].RemoteAddr
			info, ok := a.connections.Get(addr)
			if !ok {
				continue
			}
			if writeErr := a.framer.Write(info.conn, payload); writeErr != nil {
				a.logger.Warn("tcp-server: broadcast write failed",
					"addr", addr, "error", writeErr)
			} else {
				info.BytesSent.Add(int64(len(payload)))
				info.PacketsSent.Add(1)
				a.stats.IncrExternalMessagesSent()
				a.stats.AddBytesWritten(int64(len(payload)))
			}
		}
		return json.Marshal(map[string]any{"status": "broadcast_sent"})
	}

	// Send to specific connection.
	info, ok := a.connections.Get(cmd.Target)
	if !ok {
		return nil, fmt.Errorf("tcp-server send: connection %q not found", cmd.Target)
	}
	if err := a.framer.Write(info.conn, payload); err != nil {
		return nil, fmt.Errorf("tcp-server send: write failed: %w", err)
	}
	info.BytesSent.Add(int64(len(payload)))
	info.PacketsSent.Add(1)
	a.stats.IncrExternalMessagesSent()
	a.stats.AddBytesWritten(int64(len(payload)))

	return json.Marshal(map[string]any{"status": "sent"})
}

// processListConnections 는 활성 연결 목록을 반환한다.
func (a *TCPServerAgent) processListConnections() ([]byte, error) {
	list := a.connections.List()
	conns := make([]map[string]any, 0, len(list))
	for i := range list {
		conns = append(conns, map[string]any{
			"remote_addr":      list[i].RemoteAddr,
			"connected_at":     list[i].ConnectedAt.Format(time.RFC3339),
			"bytes_sent":       list[i].BytesSent.Load(),
			"bytes_received":   list[i].BytesReceived.Load(),
			"packets_sent":     list[i].PacketsSent.Load(),
			"packets_received": list[i].PacketsReceived.Load(),
		})
	}
	return json.Marshal(map[string]any{"connections": conns})
}

// Configure 는 에이전트 설정을 업데이트한다.
func (a *TCPServerAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("tcp-server configure: %w", err)
	}
	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()
	return nil
}

// ID 는 에이전트 ID 를 반환한다.
func (a *TCPServerAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *TCPServerAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *TCPServerAgent) Type() string {
	return "tcp-server"
}

// Info 는 에이전트 정보 스냅샷을 반환한다.
func (a *TCPServerAgent) Info() agent.AgentInfo {
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
		Type:      "tcp-server",
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
func (a *TCPServerAgent) Stats() agent.StatsSnapshot {
	s := a.stats.Snapshot()
	s.MsgBufferPending, s.MsgBufferCapacity = a.BufferInfo()
	return s
}

// ReceiveMessage 는 msgCh 에서 메시지를 수신한다.
// agent.MessageReceiver 인터페이스 구현.
func (a *TCPServerAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case cm := <-a.msgCh:
		return cm.Data, nil
	case <-a.stopCh:
		return nil, fmt.Errorf("tcp-server: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ReceiveMessageFrom 은 메시지와 함께 원격 주소를 반환한다.
// agent.ConnAwareReceiver 인터페이스 구현.
func (a *TCPServerAgent) ReceiveMessageFrom(ctx context.Context) ([]byte, string, error) {
	select {
	case cm := <-a.msgCh:
		return cm.Data, cm.RemoteAddr, nil
	case <-a.stopCh:
		return nil, "", fmt.Errorf("tcp-server: stopped")
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
}

// State 는 에이전트의 런타임 상태를 반환한다.
// agent.StatefulAgent 인터페이스 구현.
func (a *TCPServerAgent) State() map[string]any {
	list := a.connections.List()
	conns := make([]map[string]any, 0, len(list))
	for i := range list {
		conns = append(conns, map[string]any{
			"remote_addr":      list[i].RemoteAddr,
			"connected_at":     list[i].ConnectedAt.Format(time.RFC3339),
			"bytes_sent":       list[i].BytesSent.Load(),
			"bytes_received":   list[i].BytesReceived.Load(),
			"packets_sent":     list[i].PacketsSent.Load(),
			"packets_received": list[i].PacketsReceived.Load(),
		})
	}

	a.mu.RLock()
	ln := a.listener
	a.mu.RUnlock()

	listenerAddr := ""
	if ln != nil {
		listenerAddr = ln.Addr().String()
	}

	return map[string]any{
		"connections":   conns,
		"blocked":       a.connections.BlockedList(),
		"listener_addr": listenerAddr,
	}
}

// TransportConnected 는 리스너가 활성 상태인지 반환한다.
// agent.TransportChecker 인터페이스 구현.
func (a *TCPServerAgent) TransportConnected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.listener != nil
}

// BufferInfo 는 메시지 버퍼의 현재 상태를 반환한다.
// agent.BufferInfoProvider 인터페이스 구현.
func (a *TCPServerAgent) BufferInfo() (int, int) {
	return len(a.msgCh), cap(a.msgCh)
}

// acceptLoop 는 새 연결을 수락하는 루프이다.
func (a *TCPServerAgent) acceptLoop() {
	defer a.wg.Done()

	for {
		a.mu.RLock()
		ln := a.listener
		a.mu.RUnlock()

		if ln == nil {
			return
		}

		conn, err := ln.Accept()
		if err != nil {
			// stopCh 가 닫혔는지 확인.
			select {
			case <-a.stopCh:
				return
			default:
			}
			a.logger.Warn("tcp-server: accept error", "error", err)
			continue
		}

		remoteAddr := conn.RemoteAddr().String()

		// 일시정지 상태면 새 연결을 거부.
		a.mu.RLock()
		paused := a.paused
		a.mu.RUnlock()
		if paused {
			_ = conn.Close()
			continue
		}

		// 차단된 주소인지 확인.
		if a.connections.IsBlocked(remoteAddr) {
			_ = conn.Close()
			a.logger.Debug("tcp-server: blocked connection rejected", "addr", remoteAddr)
			continue
		}

		// 연결 추가 시도.
		if err := a.connections.Add(conn); err != nil {
			_ = conn.Close()
			a.logger.Debug("tcp-server: connection rejected", "addr", remoteAddr, "error", err)
			continue
		}

		a.wg.Add(1)
		go a.handleConn(conn)
	}
}

// handleConn 는 단일 연결에서 데이터를 읽는 루프이다.
func (a *TCPServerAgent) handleConn(conn net.Conn) {
	defer a.wg.Done()
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	defer func() { _ = a.connections.Remove(remoteAddr) }()

	reader := NewConnReader(a.framer, conn)

	for {
		// stopCh 확인.
		select {
		case <-a.stopCh:
			return
		default:
		}

		data, err := reader.Read()
		if err != nil {
			// stopCh 가 닫힌 후 에러는 정상 종료.
			select {
			case <-a.stopCh:
			default:
				a.logger.Debug("tcp-server: read error", "addr", remoteAddr, "error", err)
			}
			return
		}

		a.stats.IncrExternalMessagesReceived()
		a.stats.AddBytesRead(int64(len(data)))
		a.stats.UpdateLastActivity()

		// 수신 통계 업데이트.
		if info, ok := a.connections.Get(remoteAddr); ok {
			info.BytesReceived.Add(int64(len(data)))
			info.PacketsReceived.Add(1)
		}

		a.mu.RLock()
		paused := a.paused
		a.mu.RUnlock()

		if paused {
			// 일시정지 중이면 pauseBuf 에 버퍼링.
			a.mu.Lock()
			a.pauseBuf = append(a.pauseBuf, connMessage{Data: data, RemoteAddr: remoteAddr})
			a.mu.Unlock()
		} else {
			// msgCh 에 논블로킹 전송.
			select {
			case a.msgCh <- connMessage{Data: data, RemoteAddr: remoteAddr}:
			default:
				a.logger.Warn("tcp-server: msgCh full, dropping message",
					"addr", remoteAddr, "bytes", len(data))
			}
		}
	}
}
