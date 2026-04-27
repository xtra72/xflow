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

// UDPPeerInfo 는 UDP 피어 정보를 나타낸다.
type UDPPeerInfo struct {
	RemoteAddr    string    // 원격 주소 (host:port)
	LastSeenAt    time.Time // 마지막 수신 시각
	BytesReceived int64     // 수신 바이트 수
	DatagramCount int64     // 수신 데이터그램 수
}

// UDPServerAgent 는 UDP 서버 에이전트이다.
// UDP 데이터그램을 수신하고 피어를 추적한다.
type UDPServerAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig agent.AgentConfig
	config      UDPServerConfig
	conn        *net.UDPConn
	blocker     ConnectionManager // IP 차단 용도로만 사용
	peers       map[string]*UDPPeerInfo
	msgCh       chan []byte
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
var _ agent.Agent = (*UDPServerAgent)(nil)
var _ agent.MessageReceiver = (*UDPServerAgent)(nil)
var _ agent.StatefulAgent = (*UDPServerAgent)(nil)
var _ agent.BufferInfoProvider = (*UDPServerAgent)(nil)

// udpProcessCommand 는 UDP 서버 Process 메서드의 JSON 요청 구조체이다.
type udpProcessCommand struct {
	Command string `json:"command"`
	Target  string `json:"target,omitempty"`  // send 대상 주소
	Data    string `json:"data,omitempty"`    // base64 인코딩된 데이터
	Address string `json:"address,omitempty"` // block/unblock 대상 IP
}

// NewUDPServerAgent 는 새 UDPServerAgent 를 생성하고 초기화한다.
func NewUDPServerAgent(agentConfig agent.AgentConfig) (agent.Agent, error) {
	cfg, err := ParseUDPServerConfig(agentConfig.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("udp-server agent: %w", err)
	}

	a := &UDPServerAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("udp-server")),
		config:        cfg,
		blocker:       NewConnectionManager(0),
		peers:         make(map[string]*UDPPeerInfo),
		msgCh:         make(chan []byte, 256),
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
func (a *UDPServerAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("udp-server init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("udp-server init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("udp-server init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	return nil
}

// Start 는 UDP 리스너를 시작하고 recvLoop 고루틴을 실행한다.
func (a *UDPServerAgent) Start(_ context.Context) error {
	if a.CurrentState() != lifecycle.StateRunning {
		return fmt.Errorf("udp-server start: agent is not in running state (current: %s)", a.CurrentState())
	}

	udpAddr := &net.UDPAddr{
		IP:   net.ParseIP(a.config.Host),
		Port: a.config.Port,
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("udp-server start: %w", err)
	}

	a.mu.Lock()
	a.conn = conn
	a.mu.Unlock()

	a.wg.Add(1)
	go a.recvLoop()

	a.logger.Info("udp-server: 리스너 시작", "addr", conn.LocalAddr().String())
	return nil
}

// Stop 은 UDP 연결을 닫고 고루틴이 완료되기를 기다린다.
func (a *UDPServerAgent) Stop(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("udp-server stop: %w", err)
	}

	close(a.stopCh)

	a.mu.Lock()
	conn := a.conn
	a.conn = nil
	a.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}

	a.wg.Wait()

	if err := a.TransitionTo(lifecycle.StateStopped); err != nil {
		return fmt.Errorf("udp-server stop: %w", err)
	}

	return nil
}

// Pause 는 Running -> Paused 로 전환한다.
func (a *UDPServerAgent) Pause(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("udp-server pause: %w", err)
	}
	a.mu.Lock()
	a.paused = true
	a.mu.Unlock()
	return nil
}

// Resume 은 Paused -> Running 으로 전환한다.
func (a *UDPServerAgent) Resume(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("udp-server resume: %w", err)
	}
	a.mu.Lock()
	a.paused = false
	a.mu.Unlock()
	return nil
}

// Health 는 에이전트의 건강 상태를 반환한다.
func (a *UDPServerAgent) Health() agent.HealthStatus {
	now := time.Now()
	state := a.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		return agent.HealthStatus{
			Status:    agent.HealthHealthy,
			LastCheck: now,
			Message:   "udp-server agent is running",
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "udp-server agent is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("udp-server agent is in %s state", state),
		}
	}
}

// Process 는 JSON 명령을 처리한다.
// 지원 명령: send, list_peers, block, unblock.
func (a *UDPServerAgent) Process(data []byte) ([]byte, error) {
	var cmd udpProcessCommand
	if err := json.Unmarshal(data, &cmd); err != nil {
		return nil, fmt.Errorf("udp-server process: invalid JSON: %w", err)
	}

	switch cmd.Command {
	case "send":
		return a.processSend(cmd)
	case "list_peers":
		return a.processListPeers()
	case "block":
		return nil, a.blocker.Block(cmd.Address)
	case "unblock":
		return nil, a.blocker.Unblock(cmd.Address)
	default:
		return nil, fmt.Errorf("udp-server process: unknown command %q", cmd.Command)
	}
}

// processSend 는 특정 피어에 데이터를 전송한다.
func (a *UDPServerAgent) processSend(cmd udpProcessCommand) ([]byte, error) {
	payload, err := base64.StdEncoding.DecodeString(cmd.Data)
	if err != nil {
		return nil, fmt.Errorf("udp-server send: invalid base64 data: %w", err)
	}

	targetAddr, err := net.ResolveUDPAddr("udp", cmd.Target)
	if err != nil {
		return nil, fmt.Errorf("udp-server send: invalid target address %q: %w", cmd.Target, err)
	}

	a.mu.RLock()
	conn := a.conn
	a.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("udp-server send: not listening")
	}

	n, err := conn.WriteToUDP(payload, targetAddr)
	if err != nil {
		return nil, fmt.Errorf("udp-server send: write failed: %w", err)
	}

	a.stats.IncrExternalMessagesSent()
	a.stats.AddBytesWritten(int64(n))

	return json.Marshal(map[string]any{"status": "sent"})
}

// processListPeers 는 알려진 피어 목록을 반환한다.
func (a *UDPServerAgent) processListPeers() ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	peers := make([]map[string]any, 0, len(a.peers))
	for _, p := range a.peers {
		peers = append(peers, map[string]any{
			"remote_addr":    p.RemoteAddr,
			"last_seen_at":   p.LastSeenAt.Format(time.RFC3339),
			"bytes_received": p.BytesReceived,
			"datagram_count": p.DatagramCount,
		})
	}
	return json.Marshal(map[string]any{"peers": peers})
}

// Configure 는 에이전트 설정을 업데이트한다.
func (a *UDPServerAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("udp-server configure: %w", err)
	}
	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()
	return nil
}

// ID 는 에이전트 ID 를 반환한다.
func (a *UDPServerAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *UDPServerAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *UDPServerAgent) Type() string {
	return "udp-server"
}

// Info 는 에이전트 정보 스냅샷을 반환한다.
func (a *UDPServerAgent) Info() agent.AgentInfo {
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
		Type:      "udp-server",
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
func (a *UDPServerAgent) Stats() agent.StatsSnapshot {
	s := a.stats.Snapshot()
	s.MsgBufferPending, s.MsgBufferCapacity = a.BufferInfo()
	return s
}

// ReceiveMessage 는 msgCh 에서 메시지를 수신한다.
func (a *UDPServerAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case data := <-a.msgCh:
		return data, nil
	case <-a.stopCh:
		return nil, fmt.Errorf("udp-server: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// State 는 에이전트의 런타임 상태를 반환한다.
func (a *UDPServerAgent) State() map[string]any {
	a.mu.RLock()
	conn := a.conn
	peerList := make([]map[string]any, 0, len(a.peers))
	for _, p := range a.peers {
		peerList = append(peerList, map[string]any{
			"remote_addr":    p.RemoteAddr,
			"last_seen_at":   p.LastSeenAt.Format(time.RFC3339),
			"bytes_received": p.BytesReceived,
			"datagram_count": p.DatagramCount,
		})
	}
	a.mu.RUnlock()

	listenAddr := ""
	if conn != nil {
		listenAddr = conn.LocalAddr().String()
	}

	return map[string]any{
		"peers":       peerList,
		"blocked":     a.blocker.BlockedList(),
		"listen_addr": listenAddr,
	}
}

// BufferInfo 는 메시지 버퍼의 현재 상태를 반환한다.
func (a *UDPServerAgent) BufferInfo() (int, int) {
	return len(a.msgCh), cap(a.msgCh)
}

// recvLoop 는 UDP 데이터그램을 수신하는 루프이다.
func (a *UDPServerAgent) recvLoop() {
	defer a.wg.Done()

	buf := make([]byte, a.config.BufferSize)

	for {
		a.mu.RLock()
		conn := a.conn
		a.mu.RUnlock()

		if conn == nil {
			return
		}

		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			// stopCh 가 닫혔는지 확인.
			select {
			case <-a.stopCh:
				return
			default:
			}
			a.logger.Warn("udp-server: read error", "error", err)
			continue
		}

		// 차단된 IP 인지 확인.
		if a.blocker.IsBlocked(remoteAddr.String()) {
			a.logger.Debug("udp-server: blocked datagram dropped", "addr", remoteAddr.String())
			continue
		}

		// 피어 추적 업데이트.
		addrStr := remoteAddr.String()
		a.mu.Lock()
		peer, exists := a.peers[addrStr]
		if !exists {
			peer = &UDPPeerInfo{
				RemoteAddr: addrStr,
			}
			a.peers[addrStr] = peer
		}
		peer.LastSeenAt = time.Now()
		peer.BytesReceived += int64(n)
		peer.DatagramCount++
		a.mu.Unlock()

		// 통계 업데이트.
		a.stats.IncrExternalMessagesReceived()
		a.stats.AddBytesRead(int64(n))
		a.stats.UpdateLastActivity()

		// 데이터 복사 후 msgCh 로 전송.
		data := make([]byte, n)
		copy(data, buf[:n])

		a.mu.RLock()
		paused := a.paused
		a.mu.RUnlock()

		if paused {
			// 일시정지 중이면 드롭 (UDP 는 손실 허용).
			continue
		}

		select {
		case a.msgCh <- data:
		default:
			a.logger.Warn("udp-server: msgCh full, dropping datagram",
				"addr", addrStr, "bytes", n)
		}
	}
}
