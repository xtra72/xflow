package lg

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/internal/agent"
	amodbus "github.com/xtra/xflow/internal/agent/modbus"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// LG HVACR-03 — PMBUSB00A Modbus 게이트웨이 제어 에이전트
//
// SPEC-LG-HVACR-003. lg_hvacr02 와 디바이스 관리·메시지 출력·명령 인터페이스를
// 동일하게 맞추되, 수집은 패시브 캡처가 아니라 능동 폴링이고 제어는 서모스탯 사칭이
// 아니라 정식 레지스터 쓰기이다.
//
// 락 규약: a.mu 를 보유한 상태에서 a.Name() / a.ID() / a.Type() 을 호출하지 않는다.
// Go RWMutex 는 재귀 락을 금지하므로 자기 deadlock 이 된다 (lg_hvacr02 v0.18.6 트랩).
// 락 하에서는 a.agentConfig.Name 을 직접 읽는다.
// ---------------------------------------------------------------------------

// hvacr03RecentBufferSize 는 최근 이벤트 링 버퍼의 크기이다 (lg_hvacr02 와 동일).
const hvacr03RecentBufferSize = 64

// Hvacr03Agent 는 PMBUSB00A Modbus 게이트웨이를 폴링·제어하는 에이전트이다.
type Hvacr03Agent struct {
	*lifecycle.BaseLifecycle
	agentConfig   agent.AgentConfig
	hvacr03Config Hvacr03Config
	transport     amodbus.ModbusTransport
	stopCh        chan struct{}
	msgCh         chan []byte
	stats         *agent.AgentStats
	logger        *slog.Logger
	mu            sync.RWMutex
	startedAt     time.Time
	createdAt     time.Time
	paused        bool

	// txMu 는 모든 Modbus 트랜잭션을 직렬화한다. RTU 트랜스포트가 자체 turnaround
	// mutex 를 갖지만, "쓰기 → 대기 → read-back" 같은 복합 트랜잭션을 하나의 단위로
	// 보호하려면 상위 락이 따로 필요하다.
	txMu sync.Mutex

	// 재연결 상태
	reconnectMu       sync.Mutex
	isReconnecting    bool
	reconnectAttempts int

	// 통계 (atomic)
	eventsEmitted atomic.Int64
	pollsTotal    atomic.Int64
	pollsFailed   atomic.Int64
	scansTotal    atomic.Int64
	writesTotal   atomic.Int64
	writesFailed  atomic.Int64
	framesDropped atomic.Int64
	decodeErrors  atomic.Int64

	// 최근 이벤트 링 버퍼 (get_recent / drain 용)
	recentMu     sync.RWMutex
	recentEvents []pmbusEventRecord
	recentIdx    int
	recentFull   bool
	recentNotify chan struct{}

	// Bridge 소비자 활성 여부
	bridgeActive atomic.Bool

	// 백그라운드 루프 기동 여부. 중복 기동 시 같은 디바이스 report 가 한 틱에
	// 여러 건 중복 발행되므로 CAS 로 정확히 1회로 제한한다.
	bgStarted atomic.Bool

	// 드롭 로그 rate-limit
	lastDropLog atomic.Int64 // UnixNano

	// 트랜스포트 연결 상태. ModbusTransport.IsConnected 가 있지만, 재연결 루프와의
	// 경합을 피하기 위해 에이전트 측에서도 별도로 추적한다.
	connected atomic.Bool

	// 디바이스 관리
	devices     map[string]*PmbusDevice   // 사용자 표기 주소 → 디바이스
	lastEmitted map[string]map[string]any // 주소 → 직전 발행 투영 (dedup)

	// V2 콜백 (UUID + composite)
	onDeviceStateChangeV2 agent.DeviceStateChangeCallbackV2
}

// pmbusEventRecord 는 링 버퍼에 저장되는 이벤트 레코드이다.
type pmbusEventRecord struct {
	Event     json.RawMessage `json:"event"`
	Timestamp time.Time       `json:"timestamp"`
	Seq       int64           `json:"seq"`
}

// 컴파일 타임 인터페이스 체크
var (
	_ agent.Agent              = (*Hvacr03Agent)(nil)
	_ agent.MessageReceiver    = (*Hvacr03Agent)(nil)
	_ agent.StatefulAgent      = (*Hvacr03Agent)(nil)
	_ agent.BufferInfoProvider = (*Hvacr03Agent)(nil)
	_ agent.TransportChecker   = (*Hvacr03Agent)(nil)
	_ agent.FrameNotifier      = (*Hvacr03Agent)(nil)
)

// ---------------------------------------------------------------------------
// 팩토리
// ---------------------------------------------------------------------------

// NewHvacr03Agent 는 PMBUSB00A 게이트웨이 에이전트를 생성한다.
func NewHvacr03Agent(config agent.AgentConfig) (agent.Agent, error) {
	cfg, err := parseHvacr03Config(config.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("lg_hvacr03 agent: %w", err)
	}

	logger := agent.ResolveLogger(config)
	transport, err := newHvacr03Transport(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("lg_hvacr03 agent: %w", err)
	}

	a := &Hvacr03Agent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("lg_hvacr03")),
		hvacr03Config: cfg,
		transport:     transport,
		stopCh:        make(chan struct{}),
		msgCh:         make(chan []byte, cfg.MsgChannelSize),
		stats:         agent.NewAgentStats(),
		logger:        logger,
		createdAt:     time.Now(),
		recentEvents:  make([]pmbusEventRecord, hvacr03RecentBufferSize),
		recentNotify:  make(chan struct{}, 1),
		devices:       make(map[string]*PmbusDevice),
		lastEmitted:   make(map[string]map[string]any),
	}

	if err := a.Init(config); err != nil {
		return nil, err
	}
	return a, nil
}

// newHvacr03Transport 는 설정에 따라 RTU 또는 TCP 트랜스포트를 생성한다.
// 두 구현 모두 internal/agent/modbus 의 검증된 트랜스포트를 재사용한다.
func newHvacr03Transport(cfg Hvacr03Config, logger *slog.Logger) (amodbus.ModbusTransport, error) {
	switch cfg.TransportType {
	case "rtu":
		serial := amodbus.SerialConfig{
			Port:     cfg.SerialPort,
			BaudRate: cfg.BaudRate,
			DataBits: cfg.DataBits,
			StopBits: cfg.StopBits,
			Parity:   cfg.Parity,
		}
		return amodbus.NewModbusRTUTransport(serial, cfg.RequestTimeout, logger), nil
	case "tcp-client":
		return amodbus.NewModbusTCPTransport(cfg.TCPHost, cfg.TCPPort, cfg.RequestTimeout, logger), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrHvacr03UnknownTransportType, cfg.TransportType)
	}
}

// ---------------------------------------------------------------------------
// agent.Agent 생명주기
// ---------------------------------------------------------------------------

// Init 은 에이전트를 초기화한다.
func (a *Hvacr03Agent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("lg_hvacr03 init: %w", err)
	}
	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("lg_hvacr03 init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("lg_hvacr03 init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	a.logger.Info("lg_hvacr03: 에이전트 초기화 완료",
		"transport", a.hvacr03Config.TransportType,
		"slave_id", a.hvacr03Config.SlaveID,
	)
	return nil
}

// Start 는 트랜스포트를 열고 스캔·폴링 루프를 시작한다.
func (a *Hvacr03Agent) Start(ctx context.Context) error {
	if a.CurrentState() == lifecycle.StateRunning && a.connected.Load() {
		return nil
	}

	// 백그라운드 루프 중복 기동 방지. 위 fast-path 가드는 연결 상태에 의존하므로,
	// 재연결 윈도우처럼 잠시 미연결인 동안 Start 가 다시 호출되면 가드를 빠져나가
	// 루프 고루틴이 누적된다. CAS 로 정확히 1회로 제한한다.
	if !a.bgStarted.CompareAndSwap(false, true) {
		return nil
	}

	a.stopCh = make(chan struct{})
	a.registerConfigDevices()

	if err := a.transport.Connect(ctx); err != nil {
		a.logger.Warn("lg_hvacr03: 트랜스포트 연결 실패, 재연결 대기", "error", err)
		go a.reconnectLoop()
	} else {
		a.connected.Store(true)
		go a.scanLoop()
		go a.pollLoop()
	}

	if a.hvacr03Config.ReportInterval > 0 {
		go a.notifyLoop()
	}
	go a.offlineWatchLoop()

	a.stats.SetStartedAt(time.Now())
	a.logger.Info("lg_hvacr03: 에이전트 시작 완료")
	return nil
}

// Stop 은 에이전트를 정지한다.
func (a *Hvacr03Agent) Stop(_ context.Context) error {
	if a.CurrentState() == lifecycle.StateStopped {
		return nil
	}
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("lg_hvacr03 stop: %w", err)
	}

	close(a.stopCh)
	a.bgStarted.Store(false)
	a.connected.Store(false)

	if err := a.transport.Close(); err != nil {
		a.logger.Warn("lg_hvacr03: transport close error", "error", err)
	}

	// msgCh 드레인
	for {
		select {
		case <-a.msgCh:
		default:
			goto drained
		}
	}
drained:

	if err := a.TransitionTo(lifecycle.StateStopped); err != nil {
		return fmt.Errorf("lg_hvacr03 stop: %w", err)
	}
	return nil
}

// Pause 는 Running -> Paused 로 전환한다.
func (a *Hvacr03Agent) Pause(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("lg_hvacr03 pause: %w", err)
	}
	a.mu.Lock()
	a.paused = true
	a.mu.Unlock()
	return nil
}

// Resume 은 Paused -> Running 으로 전환한다.
func (a *Hvacr03Agent) Resume(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("lg_hvacr03 resume: %w", err)
	}
	a.mu.Lock()
	a.paused = false
	a.mu.Unlock()
	return nil
}

// isPaused 는 일시정지 여부를 반환한다.
func (a *Hvacr03Agent) isPaused() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.paused
}

// Health 는 에이전트의 건강 상태를 반환한다.
func (a *Hvacr03Agent) Health() agent.HealthStatus {
	now := time.Now()
	switch state := a.CurrentState(); state {
	case lifecycle.StateRunning:
		if !a.connected.Load() {
			return agent.HealthStatus{
				Status:    agent.HealthDegraded,
				LastCheck: now,
				Message:   "lg_hvacr03 agent is running but the gateway is not connected",
			}
		}
		return agent.HealthStatus{
			Status:    agent.HealthHealthy,
			LastCheck: now,
			Message:   "lg_hvacr03 agent is running",
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "lg_hvacr03 agent is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("lg_hvacr03 agent is in %s state", state),
		}
	}
}

// Configure 는 에이전트 설정을 업데이트한다.
//
// 트랜스포트 종류·시리얼 파라미터는 트랜스포트 오픈에 귀속되므로 런타임 변경 대상이
// 아니다. 변경이 필요하면 에이전트를 재시작해야 한다.
func (a *Hvacr03Agent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("lg_hvacr03 configure: %w", err)
	}
	if len(config.Transport.Options) > 0 {
		cfg, err := parseHvacr03Config(config.Transport.Options)
		if err != nil {
			return fmt.Errorf("lg_hvacr03 configure: re-parse config: %w", err)
		}
		a.mu.Lock()
		a.hvacr03Config = cfg
		a.agentConfig = config
		a.mu.Unlock()
	} else {
		a.mu.Lock()
		a.agentConfig = config
		a.mu.Unlock()
	}
	return nil
}

// ID 는 에이전트 ID 를 반환한다.
func (a *Hvacr03Agent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *Hvacr03Agent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *Hvacr03Agent) Type() string {
	return "lg_hvacr03"
}

// Info 는 에이전트 정보의 스냅샷을 반환한다.
func (a *Hvacr03Agent) Info() agent.AgentInfo {
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
		Type:      "lg_hvacr03",
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
func (a *Hvacr03Agent) Stats() agent.StatsSnapshot {
	s := a.stats.Snapshot()
	s.MsgBufferPending, s.MsgBufferCapacity = a.BufferInfo()
	s.Extra = a.statsExtra()
	return s
}

// statsExtra 는 타입별 통계 항목을 반환한다.
func (a *Hvacr03Agent) statsExtra() map[string]any {
	a.mu.RLock()
	devicesTotal := len(a.devices)
	devicesOnline := 0
	for _, dev := range a.devices {
		if dev.Online {
			devicesOnline++
		}
	}
	a.mu.RUnlock()

	return map[string]any{
		"events_emitted":      a.eventsEmitted.Load(),
		"polls_total":         a.pollsTotal.Load(),
		"polls_failed":        a.pollsFailed.Load(),
		"scans_total":         a.scansTotal.Load(),
		"writes_total":        a.writesTotal.Load(),
		"writes_failed":       a.writesFailed.Load(),
		"frames_dropped":      a.framesDropped.Load(),
		"decode_errors":       a.decodeErrors.Load(),
		"devices_total":       devicesTotal,
		"devices_online":      devicesOnline,
		"transport_connected": a.connected.Load(),
	}
}

// ---------------------------------------------------------------------------
// 선택적 인터페이스
// ---------------------------------------------------------------------------

// ReceiveMessage 는 msgCh 에서 메시지를 수신한다.
func (a *Hvacr03Agent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	a.bridgeActive.Store(true)
	select {
	case data := <-a.msgCh:
		a.stats.IncrInternalMessagesSent()
		return data, nil
	case <-a.stopCh:
		return nil, fmt.Errorf("lg_hvacr03: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// State 는 런타임 상태를 반환한다.
func (a *Hvacr03Agent) State() map[string]any {
	result := a.statsExtra()
	a.reconnectMu.Lock()
	result["reconnecting"] = a.isReconnecting
	result["reconnect_attempts"] = a.reconnectAttempts
	a.reconnectMu.Unlock()
	return result
}

// BufferInfo 는 메시지 버퍼의 사용량과 용량을 반환한다.
func (a *Hvacr03Agent) BufferInfo() (int, int) {
	return len(a.msgCh), cap(a.msgCh)
}

// FrameNotifyCh 는 새 이벤트 도착 시 신호를 보내는 채널을 반환한다.
func (a *Hvacr03Agent) FrameNotifyCh() <-chan struct{} {
	return a.recentNotify
}

// TransportConnected 는 게이트웨이 연결 상태를 반환한다.
func (a *Hvacr03Agent) TransportConnected() bool {
	return a.connected.Load()
}

// DeviceProvider 는 이 에이전트의 디바이스를 device.DeviceProvider 로 노출한다.
func (a *Hvacr03Agent) DeviceProvider() device.DeviceProvider {
	return NewHvacr03DeviceProvider(a)
}

// SetDeviceStateChangeCallbackV2 는 V2 콜백을 등록한다.
func (a *Hvacr03Agent) SetDeviceStateChangeCallbackV2(fn agent.DeviceStateChangeCallbackV2) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onDeviceStateChangeV2 = fn
}

// ---------------------------------------------------------------------------
// Modbus 트랜잭션
// ---------------------------------------------------------------------------

// sendAndReceive 는 단일 Modbus 트랜잭션을 수행한다.
// 모든 트랜잭션이 이 길목을 지나므로, txMu 하나로 버스 접근이 직렬화된다.
func (a *Hvacr03Agent) sendAndReceive(ctx context.Context, pdu []byte) ([]byte, error) {
	if !a.connected.Load() {
		return nil, ErrHvacr03NotConnected
	}

	a.mu.RLock()
	slaveID := a.hvacr03Config.SlaveID
	logMessages := a.hvacr03Config.LogMessages
	a.mu.RUnlock()

	a.txMu.Lock()
	defer a.txMu.Unlock()

	if logMessages {
		a.logger.Info("lg_hvacr03: TX", "slave", slaveID, "pdu", fmt.Sprintf("% X", pdu))
	}

	resp, err := a.transport.SendAndReceive(ctx, slaveID, pdu)
	if err != nil {
		return nil, err
	}

	if logMessages {
		a.logger.Info("lg_hvacr03: RX", "slave", slaveID, "pdu", fmt.Sprintf("% X", resp))
	}
	return resp, nil
}

// readBits 는 FC01/FC02 읽기를 수행한다.
func (a *Hvacr03Agent) readBits(ctx context.Context, fc byte, addr, quantity uint16) ([]bool, error) {
	pdu, err := buildReadPDU(fc, addr, quantity)
	if err != nil {
		return nil, err
	}
	resp, err := a.sendAndReceive(ctx, pdu)
	if err != nil {
		return nil, err
	}
	return parseBitResponse(resp, fc, quantity)
}

// readRegisters 는 FC03/FC04 읽기를 수행한다.
func (a *Hvacr03Agent) readRegisters(ctx context.Context, fc byte, addr, quantity uint16) ([]uint16, error) {
	pdu, err := buildReadPDU(fc, addr, quantity)
	if err != nil {
		return nil, err
	}
	resp, err := a.sendAndReceive(ctx, pdu)
	if err != nil {
		return nil, err
	}
	return parseRegisterResponse(resp, fc, quantity)
}

// writeCoil 은 FC05 단일 코일 쓰기를 수행한다.
func (a *Hvacr03Agent) writeCoil(ctx context.Context, addr uint16, on bool) error {
	pdu := buildWriteCoilPDU(addr, on)
	resp, err := a.sendAndReceive(ctx, pdu)
	a.writesTotal.Add(1)
	if err != nil {
		a.writesFailed.Add(1)
		return err
	}
	want := pmbusCoilOff
	if on {
		want = pmbusCoilOn
	}
	if err := parseWriteEchoResponse(resp, pmbusFCWriteSingleCoil, addr, want); err != nil {
		a.writesFailed.Add(1)
		return err
	}
	return nil
}

// writeRegister 는 FC06 단일 레지스터 쓰기를 수행한다.
func (a *Hvacr03Agent) writeRegister(ctx context.Context, addr, value uint16) error {
	pdu := buildWriteRegisterPDU(addr, value)
	resp, err := a.sendAndReceive(ctx, pdu)
	a.writesTotal.Add(1)
	if err != nil {
		a.writesFailed.Add(1)
		return err
	}
	if err := parseWriteEchoResponse(resp, pmbusFCWriteSingleReg, addr, value); err != nil {
		a.writesFailed.Add(1)
		return err
	}
	return nil
}

// writeRegisters 는 FC16 연속 레지스터 쓰기를 수행한다.
func (a *Hvacr03Agent) writeRegisters(ctx context.Context, addr uint16, values []uint16) error {
	pdu, err := buildWriteMultiplePDU(addr, values)
	if err != nil {
		return err
	}
	resp, err := a.sendAndReceive(ctx, pdu)
	a.writesTotal.Add(1)
	if err != nil {
		a.writesFailed.Add(1)
		return err
	}
	if err := parseWriteMultipleResponse(resp, addr, len(values)); err != nil {
		a.writesFailed.Add(1)
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// 재연결
// ---------------------------------------------------------------------------

// reconnectLoop 는 연결이 끊어졌을 때 지수 백오프로 재연결을 시도한다.
func (a *Hvacr03Agent) reconnectLoop() {
	a.reconnectMu.Lock()
	if a.isReconnecting {
		a.reconnectMu.Unlock()
		return
	}
	a.isReconnecting = true
	a.reconnectAttempts = 0
	a.reconnectMu.Unlock()

	defer func() {
		a.reconnectMu.Lock()
		a.isReconnecting = false
		a.reconnectMu.Unlock()
	}()

	a.mu.RLock()
	interval := a.hvacr03Config.ReconnectInterval
	maxBackoff := a.hvacr03Config.MaxReconnectBackoff
	a.mu.RUnlock()

	backoff := interval
	for {
		select {
		case <-a.stopCh:
			return
		case <-time.After(backoff):
		}

		a.reconnectMu.Lock()
		a.reconnectAttempts++
		attempt := a.reconnectAttempts
		a.reconnectMu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := a.transport.Connect(ctx)
		cancel()

		if err == nil {
			a.connected.Store(true)
			a.logger.Info("lg_hvacr03: 재연결 성공", "attempts", attempt)
			go a.scanLoop()
			go a.pollLoop()
			return
		}

		a.logger.Warn("lg_hvacr03: 재연결 실패", "attempt", attempt, "backoff", backoff, "error", err)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// handleTransportFailure 는 트랜잭션 실패가 연결 단절로 판단될 때 재연결을 개시한다.
// 모든 디바이스를 오프라인으로 표시하여 상태가 과거 값에 멈춰 있지 않게 한다.
func (a *Hvacr03Agent) handleTransportFailure(err error) {
	if !a.connected.CompareAndSwap(true, false) {
		return // 이미 다른 경로에서 처리 중
	}
	a.logger.Warn("lg_hvacr03: 트랜스포트 연결 끊김", "error", err)
	_ = a.transport.Close()
	a.setAllDevicesOffline()
	go a.reconnectLoop()
}

// ---------------------------------------------------------------------------
// 이벤트 버퍼
// ---------------------------------------------------------------------------

// pushRecentEvent 는 링 버퍼에 이벤트를 넣고 폴링 노드에 알린다.
func (a *Hvacr03Agent) pushRecentEvent(eventJSON []byte, ts time.Time, seq int64) {
	a.recentMu.Lock()
	a.recentEvents[a.recentIdx] = pmbusEventRecord{
		Event:     json.RawMessage(eventJSON),
		Timestamp: ts,
		Seq:       seq,
	}
	a.recentIdx = (a.recentIdx + 1) % hvacr03RecentBufferSize
	if a.recentIdx == 0 {
		a.recentFull = true
	}
	a.recentMu.Unlock()

	select {
	case a.recentNotify <- struct{}{}:
	default:
	}
}

// sendEvent 는 이벤트를 msgCh 로 전송한다.
// 버퍼가 가득 차면 가장 오래된 메시지를 드롭하여 최신 데이터를 보존한다.
func (a *Hvacr03Agent) sendEvent(data []byte) {
	select {
	case a.msgCh <- data:
		return
	default:
	}

	select {
	case <-a.msgCh:
	default:
	}

	dropped := a.framesDropped.Add(1)
	a.stats.IncrDroppedMessages()

	now := time.Now().UnixNano()
	last := a.lastDropLog.Load()
	if now-last > 10_000_000_000 && a.lastDropLog.CompareAndSwap(last, now) {
		a.logger.Warn("lg_hvacr03: msgCh full, dropping oldest event (rate-limited)",
			"total_dropped", dropped, "ch_cap", cap(a.msgCh))
	}
	a.mu.RLock()
	logDrops := a.hvacr03Config.LogDrops
	a.mu.RUnlock()
	if logDrops {
		a.logger.Warn("lg_hvacr03: msgCh full, dropping oldest event (per-drop)",
			"total_dropped", dropped, "ch_cap", cap(a.msgCh))
	}

	select {
	case a.msgCh <- data:
	default:
	}
}
