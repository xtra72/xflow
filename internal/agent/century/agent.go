package century

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// Century HVAC 패시브 캡처 에이전트 (REQ-CENTURY-001 ~ REQ-CENTURY-027)
//
// 본 에이전트는 RS-485 회선에 RX-only 로 부착되어 마스터와 슬레이브 간 통신을
// 패시브하게 캡처한다. 디코딩한 reg 0x02/0x03/0x04 응답 + reg 0x04 WRITE +
// ACK 이벤트는 ring buffer 에 저장되고, downstream node 가 polling 또는 frame
// notify 채널을 통해 소비한다.
//
// CRITICAL INVARIANT (AC-B9):
//
//	captureLoop / Process / ReceiveMessage / Stop 의 어떠한 경로도 transport.Write()
//	를 호출하지 않는다. recordingTransport 의 WriteCount 가 0 임을 단위 테스트로
//	회귀 방지한다.
// ---------------------------------------------------------------------------

// agentStats 는 Century 에이전트의 누적 통계이다 (REQ-CENTURY-025, REQ-CENTURY-027).
type agentStats struct {
	framesCaptured atomic.Uint64
	framesValid    atomic.Uint64
	framesInvalid  atomic.Uint64
	framesDropped  atomic.Uint64
	bytesReceived  atomic.Uint64

	// 단계별 invalid 세분화 (REQ-CENTURY-011)
	invalidLengthMismatch atomic.Uint64
	invalidCRCMismatch    atomic.Uint64
	invalidHeaderInvalid  atomic.Uint64
	invalidPayloadPrefix  atomic.Uint64
	invalidRegisterLength atomic.Uint64

	// 디코딩별 카운터
	reg02ResponseCount           atomic.Uint64
	reg03ResponseCount           atomic.Uint64
	reg04ResponseCount           atomic.Uint64
	reg04WriteCount              atomic.Uint64
	ackCount                     atomic.Uint64
	unconfirmedFieldObservations atomic.Uint64
	decodeErrors                 atomic.Uint64

	// 다중 IDU
	devicesDiscovered atomic.Uint64

	// WRITE 중복 제거 (REQ-CENTURY-027)
	writesDeduped atomic.Uint64
}

// CenturyAgent 는 Century HVAC 패시브 캡처 에이전트이다 (REQ-CENTURY-001).
type CenturyAgent struct {
	*lifecycle.BaseLifecycle

	agentConfig   agent.AgentConfig
	centuryConfig CenturyConfig

	transport io.ReadWriteCloser // RX-only 사용; Write 는 절대 호출하지 않음
	// transportProvider 는 (re-)Open 가능한 transport 를 생성한다.
	// production 에서는 serial dial, test 에서는 recordingTransport 를 반환.
	transportProvider func() (io.ReadWriteCloser, error)

	scanner *FrameScanner

	// 디바이스 관리 (다중 IDU 지원 — REQ-CENTURY-013)
	devicesMu sync.RWMutex
	devices   map[byte]*CenturyDevice

	// ring buffer (REQ-CENTURY-012)
	ringBuffer *FrameRingBuffer

	// cycle tracker + WRITE dedup (REQ-CENTURY-027)
	cycleTracker *CycleTracker
	writeDeduper *WriteDeduplicator

	// 카운터
	stats      *agent.AgentStats
	cStats     agentStats
	captureSeq atomic.Uint64

	// 라이프사이클
	stopCh       chan struct{}
	doneCh       chan struct{}
	frameNotify  chan struct{}
	msgCh        chan []byte
	bridgeActive atomic.Bool

	mu        sync.RWMutex
	paused    bool
	startedAt time.Time
	createdAt time.Time

	logger *slog.Logger

	// nowFunc 는 테스트 가능한 clock. nil 이면 time.Now.
	nowFunc func() time.Time

	// onDeviceStateChange 콜백 (선택)
	onDeviceStateChange func(agentName, deviceID string)
}

// Compile-time interface checks.
var _ agent.Agent = (*CenturyAgent)(nil)
var _ agent.MessageReceiver = (*CenturyAgent)(nil)
var _ agent.StatefulAgent = (*CenturyAgent)(nil)
var _ agent.TransportChecker = (*CenturyAgent)(nil)
var _ agent.FrameNotifier = (*CenturyAgent)(nil)
var _ agent.BufferInfoProvider = (*CenturyAgent)(nil)

// NewCenturyAgent 는 Century HVAC 에이전트를 생성한다 (REQ-CENTURY-001, REQ-CENTURY-029, REQ-CENTURY-030).
//
// production 경로에서 transportProvider 는 cfg.TransportType 에 따라 serial / tcp-client /
// tcp-server 트랜스포트를 dial 또는 listen 한다. 모든 트랜스포트는 RX-only 로 사용되며
// transport.Write() 는 어떠한 경로로도 호출되지 않는다 (AC-B9 / AC-G8 invariant).
//
// v0.2.0 (M6): tcp-client 와 tcp-server 모드 신규. transport-aware cycle_idle_timeout
// default 가 cfg 단계에서 이미 적용되어 있다 (REQ-CENTURY-032).
func NewCenturyAgent(config agent.AgentConfig) (agent.Agent, error) {
	centuryCfg, err := parseCenturyConfig(config.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("century agent: %w", err)
	}
	a := newCenturyAgentWithConfig(config, centuryCfg)
	a.transportProvider = func() (io.ReadWriteCloser, error) {
		// snapshotConfig 를 사용하여 Configure() 와의 race 를 피한다.
		cfg := a.snapshotConfig()
		// Start() 가 자체적으로 stopCh 로 cancel 처리를 하므로 ctx 는 Background 로 충분하다.
		// tcp-* 의 dial/listen 중 cancel 이 필요하면 reconnect loop 가 context.WithCancel 을 사용한다.
		return openTransport(context.Background(), cfg)
	}
	return a, nil
}

// newCenturyAgentForTest 는 테스트 전용 생성자이다. transport 는 이미 "열린" 상태로 주입된다.
func newCenturyAgentForTest(config agent.AgentConfig, centuryCfg CenturyConfig, transport io.ReadWriteCloser) *CenturyAgent {
	a := newCenturyAgentWithConfig(config, centuryCfg)
	a.transport = transport
	a.transportProvider = func() (io.ReadWriteCloser, error) {
		return transport, nil
	}
	return a
}

func newCenturyAgentWithConfig(config agent.AgentConfig, centuryCfg CenturyConfig) *CenturyAgent {
	return &CenturyAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("century")),
		agentConfig:   config,
		centuryConfig: centuryCfg,
		devices:       make(map[byte]*CenturyDevice),
		ringBuffer:    NewFrameRingBuffer(centuryCfg.RingBufferSize),
		cycleTracker:  NewCycleTracker(centuryCfg.CycleIdleTimeout),
		writeDeduper:  NewWriteDeduplicator(),
		stats:         agent.NewAgentStats(),
		logger:        agent.ResolveLogger(config),
		msgCh:         make(chan []byte, 256),
		frameNotify:   make(chan struct{}, 1),
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
		createdAt:     time.Now(),
	}
}

// now returns the agent's clock (test-injectable via nowFunc).
func (a *CenturyAgent) now() time.Time {
	if a.nowFunc != nil {
		return a.nowFunc()
	}
	return time.Now()
}

// snapshotConfig returns a read-only snapshot of the current CenturyConfig.
// Always use this from goroutines (captureLoop / offlineWatchLoop) to avoid
// races with Configure() which mutates centuryConfig under a.mu.
func (a *CenturyAgent) snapshotConfig() CenturyConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.centuryConfig
}

// ---------------------------------------------------------------------------
// agent.Agent 인터페이스 구현
// ---------------------------------------------------------------------------

// Init 은 에이전트를 초기화한다 (AC-D2: serial_port 누락 시 ErrSerialPortRequired).
func (a *CenturyAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("century init: %w", err)
	}
	// Init 단계는 lifecycle StateInitializing → StateRunning 으로 전이한다 (BaseAgent 패턴).
	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("century init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	// 사전 등록 devices.
	a.registerConfigDevices()

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("century init: %w", err)
	}
	a.mu.Lock()
	a.startedAt = a.now()
	a.mu.Unlock()
	a.logger.Info("century: 에이전트 초기화 완료",
		"serial_port", a.centuryConfig.SerialPort,
		"baud_rate", a.centuryConfig.BaudRate,
		"sub_dev_id", fmt.Sprintf("0x%02X", a.centuryConfig.SubDevID),
		"auto_discovery", a.centuryConfig.AutoDiscovery,
		"dedupe_writes", a.centuryConfig.DedupeWrites,
	)
	return nil
}

// Start 는 transport 를 열고 captureLoop 를 시작한다.
//
// (REQ-CENTURY-013, REQ-CENTURY-014 — capture loop + offline watch loop 동시 시작)
func (a *CenturyAgent) Start(_ context.Context) error {
	if a.transport == nil {
		if a.transportProvider == nil {
			return fmt.Errorf("century start: %w", ErrTransportNotOpen)
		}
		t, err := a.transportProvider()
		if err != nil {
			return fmt.Errorf("century start: %w", err)
		}
		a.transport = t
		// Wire agent slog into tcp-server wrapper so secondary-rejection INFO
		// events use the same structured logger as the rest of the agent.
		if srv, ok := t.(*tcpServerTransport); ok && a.logger != nil {
			srv.SetLogger(a.logger)
		}
	}

	// 동일 인스턴스 재기동을 위한 stopCh / doneCh 재설정 (samsung 패턴 참조).
	a.mu.Lock()
	select {
	case <-a.stopCh:
		a.stopCh = make(chan struct{})
	default:
	}
	select {
	case <-a.doneCh:
		a.doneCh = make(chan struct{})
	default:
	}
	a.scanner = NewFrameScanner(a.transport)
	a.mu.Unlock()

	go a.captureLoop()
	go a.offlineWatchLoop()
	a.stats.SetStartedAt(a.now())
	a.logger.Info("century: 에이전트 시작 완료")
	return nil
}

// Stop 은 capture loop 와 transport 를 종료한다.
func (a *CenturyAgent) Stop(_ context.Context) error {
	if a.CurrentState() == lifecycle.StateStopped {
		return nil
	}
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("century stop: %w", err)
	}

	close(a.stopCh)
	a.mu.Lock()
	t := a.transport
	a.mu.Unlock()
	if t != nil {
		_ = t.Close()
	}
	<-a.doneCh

	if err := a.TransitionTo(lifecycle.StateStopped); err != nil {
		return fmt.Errorf("century stop: %w", err)
	}
	a.logger.Info("century: 에이전트 정지")
	return nil
}

// Pause 는 capture loop 를 일시 정지시킨다.
func (a *CenturyAgent) Pause(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("century pause: %w", err)
	}
	a.mu.Lock()
	a.paused = true
	a.mu.Unlock()
	return nil
}

// Resume 는 일시정지된 capture loop 를 재개한다.
func (a *CenturyAgent) Resume(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("century resume: %w", err)
	}
	a.mu.Lock()
	a.paused = false
	a.mu.Unlock()
	return nil
}

// Health 는 에이전트 건강 상태를 반환한다.
func (a *CenturyAgent) Health() agent.HealthStatus {
	now := a.now()
	state := a.CurrentState()
	switch state {
	case lifecycle.StateRunning:
		return agent.HealthStatus{Status: agent.HealthHealthy, LastCheck: now, Message: "century agent is running"}
	case lifecycle.StatePaused:
		return agent.HealthStatus{Status: agent.HealthDegraded, LastCheck: now, Message: "century agent is paused"}
	default:
		return agent.HealthStatus{Status: agent.HealthUnhealthy, LastCheck: now, Message: fmt.Sprintf("century agent is in %s state", state)}
	}
}

// centuryProcessRequest 는 Process 의 JSON 명령 구조체이다.
type centuryProcessRequest struct {
	Command string `json:"command"`
	Count   int    `json:"count,omitempty"`
	LastSeq uint64 `json:"last_seq,omitempty"`
}

// Process 는 JSON 명령을 처리한다. 본 에이전트는 패시브 캡처 전용이므로 어떠한 경우에도
// transport.Write() 를 호출하지 않는다 (AC-B9, REQ-CENTURY-017).
//
// 지원 명령: get_stats / get_recent / drain. 그 외 모든 명령은 not_supported 응답.
func (a *CenturyAgent) Process(data []byte) ([]byte, error) {
	var req centuryProcessRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("century process: invalid request: %w", err)
	}
	switch req.Command {
	case "get_stats":
		return a.processGetStats()
	case "get_recent":
		count := req.Count
		if count <= 0 {
			count = 10
		}
		return a.processGetRecent(count, req.LastSeq)
	case "drain":
		return a.processDrain()
	default:
		// not_supported 응답 (REQ-CENTURY-017). Process 에서 ErrControlNotSupported 를 반환하여
		// node 가 이를 status response 로 wrap 할 수 있게 한다.
		body := map[string]any{
			"status":  "not_supported",
			"reason":  "century_passive_only",
			"message": "Century HVAC agent operates in passive sniff mode; control commands are never transmitted",
			"command": req.Command,
		}
		return json.Marshal(body)
	}
}

// processGetStats 는 통계 JSON 을 반환한다 (AC-B6).
func (a *CenturyAgent) processGetStats() ([]byte, error) {
	stats := map[string]any{
		"frames_captured":                a.cStats.framesCaptured.Load(),
		"frames_valid":                   a.cStats.framesValid.Load(),
		"frames_invalid":                 a.cStats.framesInvalid.Load(),
		"frames_dropped":                 a.cStats.framesDropped.Load(),
		"bytes_received":                 a.cStats.bytesReceived.Load(),
		"invalid_length_mismatch":        a.cStats.invalidLengthMismatch.Load(),
		"invalid_crc_mismatch":           a.cStats.invalidCRCMismatch.Load(),
		"invalid_header_invalid":         a.cStats.invalidHeaderInvalid.Load(),
		"invalid_payload_prefix":         a.cStats.invalidPayloadPrefix.Load(),
		"invalid_register_length":        a.cStats.invalidRegisterLength.Load(),
		"reg02_response_count":           a.cStats.reg02ResponseCount.Load(),
		"reg03_response_count":           a.cStats.reg03ResponseCount.Load(),
		"reg04_response_count":           a.cStats.reg04ResponseCount.Load(),
		"reg04_write_count":              a.cStats.reg04WriteCount.Load(),
		"ack_count":                      a.cStats.ackCount.Load(),
		"unconfirmed_field_observations": a.cStats.unconfirmedFieldObservations.Load(),
		"writes_deduped":                 a.cStats.writesDeduped.Load(),
		"devices_discovered":             a.cStats.devicesDiscovered.Load(),
		"transport_connected":            a.TransportConnected(),
	}
	return json.Marshal(stats)
}

// capturedFrameEvent 는 get_recent / drain 응답의 한 entry 이다.
type capturedFrameEvent struct {
	Seq          uint64          `json:"seq"`
	TimestampMs  int64           `json:"timestamp_ms"`
	RawHex       string          `json:"raw_hex"`
	FunctionCode byte            `json:"function_code"`
	Register     *byte           `json:"register,omitempty"`
	Decoded      json.RawMessage `json:"decoded,omitempty"`
	DecodeError  string          `json:"decode_error,omitempty"`
}

// frameToEvent 는 CapturedFrame 을 직렬화 가능한 event 로 변환한다.
func frameToEvent(c CapturedFrame) capturedFrameEvent {
	ev := capturedFrameEvent{
		Seq:         c.Seq,
		TimestampMs: c.ReceivedAt.UnixMilli(),
		RawHex:      hex.EncodeToString(c.Raw),
	}
	if c.Frame != nil {
		ev.FunctionCode = c.Frame.FunctionCode
		if reg, ok := c.Frame.Register(); ok {
			r := reg
			ev.Register = &r
		}
	}
	if c.DecodeErr != nil {
		ev.DecodeError = c.DecodeErr.Error()
	}
	if c.Decoded != nil {
		if b, err := json.Marshal(c.Decoded); err == nil {
			ev.Decoded = b
		}
	}
	return ev
}

// processGetRecent 는 ring buffer 의 최근 count 프레임을 lastSeq 이후만 필터링하여 반환한다 (AC-B7, 비파괴).
func (a *CenturyAgent) processGetRecent(count int, lastSeq uint64) ([]byte, error) {
	recs := a.ringBuffer.GetRecent(count)
	out := make([]capturedFrameEvent, 0, len(recs))
	for _, c := range recs {
		if lastSeq > 0 && c.Seq <= lastSeq {
			continue
		}
		out = append(out, frameToEvent(c))
	}
	return json.Marshal(map[string]any{"count": len(out), "frames": out})
}

// processDrain 은 ring buffer 의 모든 프레임을 반환하고 버퍼를 비운다 (AC-B8).
func (a *CenturyAgent) processDrain() ([]byte, error) {
	recs := a.ringBuffer.Drain()
	out := make([]capturedFrameEvent, 0, len(recs))
	for _, c := range recs {
		out = append(out, frameToEvent(c))
	}
	return json.Marshal(map[string]any{"count": len(out), "frames": out})
}

// Configure 는 에이전트 설정을 동적으로 업데이트한다.
func (a *CenturyAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("century configure: %w", err)
	}
	if len(config.Transport.Options) > 0 {
		newCfg, err := parseCenturyConfig(config.Transport.Options)
		if err != nil {
			return fmt.Errorf("century configure: %w", err)
		}
		a.mu.Lock()
		a.centuryConfig = newCfg
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
func (a *CenturyAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *CenturyAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다 (REQ-CENTURY-001).
func (a *CenturyAgent) Type() string { return "century-hvac" }

// Info 는 에이전트 정보 스냅샷을 반환한다.
func (a *CenturyAgent) Info() agent.AgentInfo {
	a.mu.RLock()
	cfg := a.agentConfig
	startedAt := a.startedAt
	createdAt := a.createdAt
	a.mu.RUnlock()
	state := a.CurrentState()
	var uptime time.Duration
	if state == lifecycle.StateRunning && !startedAt.IsZero() {
		uptime = a.now().Sub(startedAt)
	}
	return agent.AgentInfo{
		ID:        cfg.ID,
		Name:      cfg.Name,
		Type:      a.Type(),
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
func (a *CenturyAgent) Stats() agent.StatsSnapshot {
	s := a.stats.Snapshot()
	pending, capV := a.BufferInfo()
	s.MsgBufferPending = pending
	s.MsgBufferCapacity = capV
	s.Extra = map[string]any{
		"frames_captured":    a.cStats.framesCaptured.Load(),
		"frames_valid":       a.cStats.framesValid.Load(),
		"frames_dropped":     a.cStats.framesDropped.Load(),
		"writes_deduped":     a.cStats.writesDeduped.Load(),
		"devices_discovered": a.cStats.devicesDiscovered.Load(),
	}
	return s
}

// ---------------------------------------------------------------------------
// MessageReceiver / StatefulAgent / TransportChecker / FrameNotifier / BufferInfoProvider
// ---------------------------------------------------------------------------

// ReceiveMessage 는 msgCh 에서 다음 디코딩된 이벤트를 읽어 반환한다.
func (a *CenturyAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	a.bridgeActive.Store(true)
	select {
	case data, ok := <-a.msgCh:
		if !ok {
			return nil, ErrAgentStopped
		}
		return data, nil
	case <-a.stopCh:
		return nil, ErrAgentStopped
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// State 는 에이전트 상태 스냅샷을 반환한다 (StatefulAgent).
func (a *CenturyAgent) State() map[string]any {
	out := map[string]any{
		"frames_captured":    a.cStats.framesCaptured.Load(),
		"frames_valid":       a.cStats.framesValid.Load(),
		"frames_invalid":     a.cStats.framesInvalid.Load(),
		"frames_dropped":     a.cStats.framesDropped.Load(),
		"writes_deduped":     a.cStats.writesDeduped.Load(),
		"devices_discovered": a.cStats.devicesDiscovered.Load(),
		"devices":            a.listDevicesForState(),
	}
	return out
}

// listDevicesForState 는 State() 에 임베드되는 디바이스 요약 리스트를 반환한다.
func (a *CenturyAgent) listDevicesForState() []map[string]any {
	a.devicesMu.RLock()
	defer a.devicesMu.RUnlock()
	out := make([]map[string]any, 0, len(a.devices))
	for _, d := range a.devices {
		snap := d.Snapshot()
		out = append(out, map[string]any{
			"sub_dev_id": fmt.Sprintf("0x%02X", snap.SubDevID),
			"label":      snap.Label,
			"source":     snap.Source,
			"online":     snap.Online,
			"last_seen":  snap.LastSeen.UnixMilli(),
		})
	}
	return out
}

// TransportConnected 는 트랜스포트가 살아있는지 여부를 반환한다.
//
// reconnectWithBackoff 가 a.transport 를 nil 로 잠시 비웠다가 새 객체로 교체할 수 있으므로
// a.mu 로 동기화한다.
func (a *CenturyAgent) TransportConnected() bool {
	a.mu.RLock()
	hasT := a.transport != nil
	a.mu.RUnlock()
	return hasT && a.CurrentState() == lifecycle.StateRunning
}

// FrameNotifyCh 는 새 프레임 도착 알림 채널을 반환한다 (AC-C8 의 즉시 반응 용).
func (a *CenturyAgent) FrameNotifyCh() <-chan struct{} {
	return a.frameNotify
}

// BufferInfo 는 msgCh 의 사용량을 반환한다.
func (a *CenturyAgent) BufferInfo() (int, int) {
	return len(a.msgCh), cap(a.msgCh)
}

// ListDevices 는 등록된 모든 CenturyDevice 의 스냅샷을 반환한다.
//
// (REQ-CENTURY-015 의 DeviceProvider 어댑터를 위한 helper. provider.go 에서 사용 예정 — M4)
func (a *CenturyAgent) ListDevices() []CenturyDeviceSnapshot {
	a.devicesMu.RLock()
	defer a.devicesMu.RUnlock()
	out := make([]CenturyDeviceSnapshot, 0, len(a.devices))
	for _, d := range a.devices {
		out = append(out, d.Snapshot())
	}
	return out
}

// SetDeviceStateChangeCallback 는 디바이스 상태 변경 콜백을 등록한다.
func (a *CenturyAgent) SetDeviceStateChangeCallback(fn func(agentName, deviceID string)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onDeviceStateChange = fn
}

// ---------------------------------------------------------------------------
// Capture Loop & Helpers
// ---------------------------------------------------------------------------

// captureLoop 는 FrameScanner 로부터 프레임을 받아 디코딩하고 ring buffer 에 저장한다.
//
// 본 함수는 transport.Write() 를 직접도 간접도 호출하지 않는다 (AC-B9).
//
// v0.2.0 (M6): tcp-client 모드에서 transport read 실패 시 exponential backoff 로
// 재연결한다 (REQ-CENTURY-031). serial 과 tcp-server 모드는 기존처럼 한 번에 종료한다.
func (a *CenturyAgent) captureLoop() {
	defer close(a.doneCh)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// stopCh 가 close 되면 ctx 도 cancel.
	go func() {
		select {
		case <-a.stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	for {
		select {
		case <-a.stopCh:
			return
		default:
		}

		a.mu.RLock()
		paused := a.paused
		a.mu.RUnlock()
		if paused {
			select {
			case <-a.stopCh:
				return
			case <-time.After(20 * time.Millisecond):
			}
			continue
		}

		a.mu.RLock()
		scanner := a.scanner
		a.mu.RUnlock()
		f, err := scanner.Next(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			a.cStats.invalidCRCMismatch.Add(scanner.Stats().CRCErrors - a.cStats.invalidCRCMismatch.Load())

			// tcp-client only: attempt exponential-backoff reconnect (REQ-CENTURY-031).
			cfg := a.snapshotConfig()
			if cfg.TransportType == "tcp-client" {
				if reconnected := a.reconnectWithBackoff(ctx, cfg); reconnected {
					continue
				}
				return
			}
			// serial / tcp-server: surface the error and stop the loop.
			return
		}

		now := a.now()
		a.cStats.framesCaptured.Add(1)
		rawCopy := make([]byte, HeaderLength+int(f.PayloadLength)+CRCLength)
		// reconstruct the raw bytes from the parsed frame: header + payload + CRC LE.
		// (FrameScanner consumed and discarded raw; reconstructing here is the cheapest
		// way to expose them to downstream nodes without modifying the scanner.)
		rawCopy = reconstructRawFrame(f)
		a.cStats.bytesReceived.Add(uint64(len(rawCopy)))

		// Snapshot config under lock to avoid races with Configure().
		cfg := a.snapshotConfig()

		// Decode.
		decoded, decodeErr := Decode(f, now.UnixMilli())
		if decodeErr != nil {
			a.cStats.decodeErrors.Add(1)
			if cfg.LogDecodeErrors {
				a.logger.Warn("century: 디코드 실패", "error", decodeErr)
			}
		} else {
			a.cStats.framesValid.Add(1)
		}

		// WRITE dedup (REQ-CENTURY-027). Must run BEFORE bumpDecodeCounter so that
		// deduped WRITEs do not contribute to reg04_write_count (AC-F1).
		cycleID := a.cycleTracker.OnFrame(f, now)
		emitDecoded := true
		if _, isWrite := decoded.(*Reg04WriteDecoded); isWrite && cfg.DedupeWrites {
			// dedup 키는 raw payload (sub_dev_id + reserved2 + register + data).
			if !a.writeDeduper.ShouldEmit(cycleID, f.Payload) {
				emitDecoded = false
				a.cStats.writesDeduped.Add(1)
				if cfg.LogDrops {
					a.logger.Debug("century: WRITE dedup", "cycle", cycleID, "sub_dev_id", fmt.Sprintf("0x%02X", f.Payload[0]))
				}
			}
		}

		// Bump per-message-type counters only for messages that will actually emit.
		// Deduped WRITEs are NOT counted in reg04_write_count (AC-F1).
		if decodeErr == nil && emitDecoded {
			a.bumpDecodeCounter(decoded)
		}

		// Device discovery + state update (REQ-CENTURY-013).
		if decoded != nil {
			a.touchDeviceFromDecoded(decoded, f, now, cfg.AutoDiscovery)
		}

		// Ring buffer push.
		seq := a.captureSeq.Add(1)
		entry := CapturedFrame{
			Seq:        seq,
			ReceivedAt: now,
			Raw:        rawCopy,
			Frame:      f,
			Decoded:    decoded,
			DecodeErr:  decodeErr,
		}
		if dropped := a.ringBuffer.Push(entry); dropped {
			a.cStats.framesDropped.Add(1)
			if cfg.LogDrops {
				a.logger.Warn("century: 에이전트 메시지 버퍼 가득 참, 드롭",
					"total_dropped", a.cStats.framesDropped.Load(),
					"ring_capacity", a.ringBuffer.Capacity(),
				)
			}
		}

		// FrameNotify (non-blocking).
		select {
		case a.frameNotify <- struct{}{}:
		default:
		}

		// Emit decoded event to msgCh (best-effort, non-blocking on full).
		if emitDecoded && decoded != nil && a.bridgeActive.Load() {
			if b, err := json.Marshal(decoded); err == nil {
				select {
				case a.msgCh <- b:
				default:
					// msgCh full — drop oldest.
					select {
					case <-a.msgCh:
					default:
					}
					select {
					case a.msgCh <- b:
					default:
					}
				}
			}
		}
	}
}

// reconstructRawFrame 은 *Frame 으로부터 원시 wire bytes 를 재구성한다.
//
// FrameScanner 가 raw 를 보관하지 않으므로, 에이전트가 ring buffer / raw-frame node 송출용으로
// 직접 재구성한다. 헤더 + payload + CRC(LE) 형식.
func reconstructRawFrame(f *Frame) []byte {
	pl := int(f.PayloadLength)
	raw := make([]byte, HeaderLength+pl+CRCLength)
	raw[0] = byte(f.Src)
	raw[1] = byte(f.Src >> 8)
	raw[2] = byte(f.Dst)
	raw[3] = byte(f.Dst >> 8)
	raw[4] = byte(f.PayloadLength)
	raw[5] = byte(f.PayloadLength >> 8)
	raw[6] = f.Reserved
	raw[7] = f.FunctionCode
	copy(raw[8:8+pl], f.Payload)
	raw[8+pl] = byte(f.CRC)
	raw[8+pl+1] = byte(f.CRC >> 8)
	return raw
}

// bumpDecodeCounter 는 디코딩된 메시지 타입별 카운터를 증가시킨다.
func (a *CenturyAgent) bumpDecodeCounter(decoded any) {
	switch decoded.(type) {
	case *Reg02Decoded:
		a.cStats.reg02ResponseCount.Add(1)
	case *Reg03Decoded:
		a.cStats.reg03ResponseCount.Add(1)
	case *Reg04ReadDecoded:
		a.cStats.reg04ResponseCount.Add(1)
	case *Reg04WriteDecoded:
		a.cStats.reg04WriteCount.Add(1)
	case *ACKDecoded:
		a.cStats.ackCount.Add(1)
	}
}

// touchDeviceFromDecoded 는 decoded message 의 sub_dev_id 를 키로 device 를 찾거나 등록하고,
// 해당 device 의 state 와 LastSeen 을 갱신한다 (REQ-CENTURY-013, REQ-CENTURY-014).
//
// autoDiscovery 는 호출자가 snapshotConfig() 로 미리 안전하게 추출해 전달한다.
func (a *CenturyAgent) touchDeviceFromDecoded(decoded any, f *Frame, now time.Time, autoDiscovery bool) {
	subDevID, ok := subDevIDFromDecoded(decoded)
	if !ok {
		// ACK 등 sub_dev_id 가 없는 메시지는 device touch 대상 외.
		return
	}
	a.devicesMu.Lock()
	dev, exists := a.devices[subDevID]
	if !exists {
		if !autoDiscovery {
			// auto_discovery=false 이면 새 device 자동 등록 안 함 — 설정 device 만 사용.
			a.devicesMu.Unlock()
			return
		}
		dev = NewCenturyDevice(subDevID, "auto", now)
		a.devices[subDevID] = dev
		a.cStats.devicesDiscovered.Add(1)
		a.logger.Info("century: 디바이스 자동 발견", "sub_dev_id", fmt.Sprintf("0x%02X", subDevID))
	}
	a.devicesMu.Unlock()

	dev.Touch(now)
	dev.Update(decoded, now)
	_ = f
}

// subDevIDFromDecoded 는 디코딩된 메시지에서 sub_dev_id 를 추출한다. ACK 는 (0, false) 반환.
func subDevIDFromDecoded(decoded any) (byte, bool) {
	switch m := decoded.(type) {
	case *Reg02Decoded:
		return m.SubDevID, true
	case *Reg03Decoded:
		return m.SubDevID, true
	case *Reg04ReadDecoded:
		return m.SubDevID, true
	case *Reg04WriteDecoded:
		return m.SubDevID, true
	default:
		return 0, false
	}
}

// registerConfigDevices 는 설정에 사전 등록된 device 들을 추가한다 (REQ-CENTURY-013, Source="config").
func (a *CenturyAgent) registerConfigDevices() {
	now := a.now()
	cfg := a.snapshotConfig()
	a.devicesMu.Lock()
	defer a.devicesMu.Unlock()
	for _, entry := range cfg.Devices {
		if entry.Address == "" {
			continue
		}
		// address can be "0x3B" or "59" — parse with parseHexOrInt.
		n, err := parseHexOrInt(entry.Address)
		if err != nil {
			a.logger.Warn("century: invalid device address", "address", entry.Address, "error", err)
			continue
		}
		sub := byte(n)
		if _, exists := a.devices[sub]; exists {
			continue
		}
		dev := NewCenturyDevice(sub, "config", now)
		dev.Online = false // not yet seen on the wire
		if entry.Name != "" {
			dev.Label = entry.Name
		}
		a.devices[sub] = dev
	}
}

// offlineWatchLoop 는 1초 ticker 로 stale device 를 offline 으로 전이시킨다 (REQ-CENTURY-014).
func (a *CenturyAgent) offlineWatchLoop() {
	cfg := a.snapshotConfig()
	interval := 1 * time.Second
	if cfg.OfflineTimeout < interval {
		// 짧은 timeout 의 경우 더 자주 검사 (테스트 친화적).
		interval = cfg.OfflineTimeout / 4
		if interval < 10*time.Millisecond {
			interval = 10 * time.Millisecond
		}
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.checkDeviceTimeouts()
		}
	}
}

// checkDeviceTimeouts 는 모든 device 의 LastSeen 을 검사하여 offline 으로 전이한다.
func (a *CenturyAgent) checkDeviceTimeouts() {
	now := a.now()
	timeout := a.snapshotConfig().OfflineTimeout
	a.devicesMu.RLock()
	devicesSnapshot := make([]*CenturyDevice, 0, len(a.devices))
	for _, d := range a.devices {
		devicesSnapshot = append(devicesSnapshot, d)
	}
	a.devicesMu.RUnlock()
	for _, d := range devicesSnapshot {
		if d.IsStale(now, timeout) {
			d.mu.Lock()
			if d.Online {
				d.Online = false
				d.mu.Unlock()
				a.logger.Info("century: 디바이스 오프라인",
					"sub_dev_id", fmt.Sprintf("0x%02X", d.SubDevID),
				)
				if fn := a.onDeviceStateChange; fn != nil {
					go fn(a.Name(), fmt.Sprintf("%s:%02x", a.Name(), d.SubDevID))
				}
			} else {
				d.mu.Unlock()
			}
		}
	}
}

// reconnectWithBackoff 는 tcp-client 모드에서 transport 가 끊겼을 때 exponential backoff 로
// 재연결을 시도한다 (REQ-CENTURY-031).
//
// 동작:
//   - 기존 transport 를 close.
//   - cfg.ReconnectInitial 부터 시작하여 매 실패마다 2배씩 증가, cfg.MaxReconnectBackoff 가 상한.
//   - backoff 동안 ctx cancel 이 발생하면 즉시 false 반환.
//   - 재연결 성공 시 a.transport 와 a.scanner 가 새 객체로 교체되고 true 반환 (backoff 리셋).
//
// AC-B9 invariant: 재연결로 새로 열린 transport 도 RX-only — wrapper 의 Write 가 차단된다.
func (a *CenturyAgent) reconnectWithBackoff(ctx context.Context, cfg CenturyConfig) bool {
	if a.transportProvider == nil {
		return false
	}
	// Close old transport so any lingering FD is released.
	a.mu.Lock()
	oldTransport := a.transport
	a.transport = nil
	a.mu.Unlock()
	if oldTransport != nil {
		_ = oldTransport.Close()
	}

	backoff := cfg.ReconnectInitial
	if backoff <= 0 {
		backoff = DefaultReconnectInitial
	}
	maxBackoff := cfg.MaxReconnectBackoff
	if maxBackoff <= 0 {
		maxBackoff = DefaultMaxReconnectBackoff
	}

	for {
		// Sleep first (current backoff). On the very first iteration this
		// matches the SPEC: "fail → wait reconnect_initial → try" (AC-G2).
		select {
		case <-ctx.Done():
			return false
		case <-a.stopCh:
			return false
		case <-time.After(backoff):
		}

		t, err := a.transportProvider()
		if err != nil {
			a.logger.Warn("century: TCP-client reconnect failed", "error", err, "next_backoff", backoff)
			// Double the backoff, capped at max.
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		// Success.
		a.mu.Lock()
		a.transport = t
		a.scanner = NewFrameScanner(t)
		a.mu.Unlock()
		a.logger.Info("century: TCP-client reconnected")
		return true
	}
}

// errIsClosedOrCanceled 는 io.EOF / context.Canceled / 빈 read 등 fatal 종료 신호인지 판정한다.
func errIsClosedOrCanceled(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF || err == io.ErrUnexpectedEOF || err == context.Canceled || err == context.DeadlineExceeded {
		return true
	}
	return false
}
