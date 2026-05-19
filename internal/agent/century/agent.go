package century

import (
	"bytes"
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
	"github.com/xtra/xflow/internal/device"
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

// agentStats 는 Century 에이전트의 누적 통계이다 (REQ-CENTURY-025, REQ-CENTURY-027, REQ-CENTURY-035).
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

	// v0.3.0 device-centric emit (REQ-CENTURY-033/034/035).
	deviceStateEmits   atomic.Uint64 // total device_state emit (change + keepalive)
	changeEmits        atomic.Uint64 // trigger="change" count
	reportEmits        atomic.Uint64 // trigger="keepalive" count
	deviceStateDropped atomic.Uint64 // msgCh full drops for device_state messages
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

	// v0.3.0 device-centric emit state (REQ-CENTURY-033/034/035).
	//
	// emitMu 는 lastEmitState / lastEmitTime / lastEmitOnline / lastEmitSeen /
	// lastReportTime 의 동시 접근을 직렬화한다
	// (captureLoop 의 change-detect path 와 keepalive ticker 가 동시 접근).
	emitMu         sync.Mutex
	lastEmitState  map[byte]CenturyDeviceStateSnapshot
	lastEmitTime   map[byte]time.Time
	lastEmitOnline map[byte]bool
	lastEmitSeen   map[byte]bool // tracks whether a first emit has happened for this device
	// lastReportTime 은 keepalive emit 의 독립 타이머이다 (v0.3.10 — keepalive 정상 동작 fix).
	// change 트리거 emit 은 lastEmitTime 만 갱신하고 본 필드는 건드리지 않는다.
	// 첫 emit 시 (change 또는 keepalive 무관) 본 필드도 초기화되어 첫 interval 의
	// 기준점이 된다. 이후 keepalive 가 fire 할 때만 본 필드를 now 로 갱신한다.
	// 결과: change 가 자주 일어나도 keepalive 는 interval 마다 독립적으로 fire 한다.
	lastReportTime map[byte]time.Time
	reportStopCh   chan struct{} // closed in Stop to terminate reportLoop early

	// v0.6.6: event_temp_threshold gate 용 마지막 보고 시점 실내온도 (per device).
	// change/report 무관하게 emit 이 실제로 발생한 모든 시점에 갱신된다.
	// EqualsExceptCurrentTemp(prev, snap) && |snap.CurrentTemp − lastReportTemp| < threshold
	// 인 경우 change emit 을 suppress 한다.
	lastReportTemp    map[byte]float32
	lastReportTempSet map[byte]bool

	// v0.3.6: register-decoded change detection.
	// (dev_id, register) 별 최근 emit 한 transformed JSON 을 보관하여 동일 state 반복
	// emit 을 방지한다. captureLoop 단일 goroutine 에서 접근하므로 별도 mutex 불필요.
	lastRegisterEmit map[registerEmitKey][]byte

	// v0.3.11: polling-friendly device_state buffer (NASA recentSnapshots 패턴).
	//
	// 사용자 보고: century-status 노드는 ringBuffer 를 polling 하는데, device_state
	// emit (change/keepalive) 은 msgCh 로만 보내져 polling 경로에서 보이지 않았다.
	// keepalive 가 "전송 안됨" 으로 관측된 root cause.
	//
	// 본 버퍼는 maybeEmitDeviceState 에서 msgCh push 와 별도로 추가 push 되며,
	// processDrainDeviceState 가 polling node 의 요청에 따라 drain 한다.
	// msgCh (Bridge 컨슈머용) 와 독립적이라 두 경로가 경쟁하지 않는다.
	deviceStateBufMu  sync.Mutex
	deviceStateBuf    []json.RawMessage
	deviceStateBufMax int
}

// registerEmitKey 는 register-decoded change detection 의 캐시 키이다.
type registerEmitKey struct {
	DevID    byte
	Register byte
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
	// Lifecycle 을 StateUnknown → StateInitializing → StateRunning 으로 전이시킨다.
	// agent.DefaultManager 는 등록된 factory 의 경우 Init() 을 명시적으로 호출하지 않으므로
	// (manager.go: else 폴백 분기에서만 Init 호출) factory 가 책임진다.
	// samsung-nasa, lgcnp 와 동일한 패턴 — 이를 누락하면 Health/Info/State 가 stopped 로 보고된다.
	if err := a.Init(config); err != nil {
		return nil, fmt.Errorf("century agent: %w", err)
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
		BaseLifecycle:     lifecycle.NewBaseLifecycle(lifecycle.WithName("century")),
		agentConfig:       config,
		centuryConfig:     centuryCfg,
		devices:           make(map[byte]*CenturyDevice),
		ringBuffer:        NewFrameRingBuffer(centuryCfg.RingBufferSize),
		cycleTracker:      NewCycleTracker(centuryCfg.CycleIdleTimeout),
		writeDeduper:      NewWriteDeduplicator(),
		stats:             agent.NewAgentStats(),
		logger:            agent.ResolveLogger(config),
		msgCh:             make(chan []byte, 256),
		frameNotify:       make(chan struct{}, 1),
		stopCh:            make(chan struct{}),
		doneCh:            make(chan struct{}),
		createdAt:         time.Now(),
		lastEmitState:     make(map[byte]CenturyDeviceStateSnapshot),
		lastEmitTime:      make(map[byte]time.Time),
		lastEmitOnline:    make(map[byte]bool),
		lastEmitSeen:      make(map[byte]bool),
		lastReportTime:    make(map[byte]time.Time),
		reportStopCh:      make(chan struct{}),
		lastRegisterEmit:  make(map[registerEmitKey][]byte),
		lastReportTemp:    make(map[byte]float32),
		lastReportTempSet: make(map[byte]bool),
		// v0.3.11: device_state polling buffer. 기본 capacity = ringBuffer 의 절반
		// (예: 128 → 64). 너무 작으면 polling 간격 사이에 drop, 너무 크면 메모리 낭비.
		deviceStateBufMax: centuryCfg.RingBufferSize / 2,
	}
}

// registerCodeFromDecoded 는 디코딩된 메시지에서 register 코드 (0x02/0x03/0x04) 를 추출한다.
// Reg04WriteDecoded 는 read 와 같은 register=0x04 이지만 의미 다르므로 high-bit 으로 구분 (0x84).
// ACK 는 register 가 없으므로 caller 가 ACK 분기 후 호출해야 한다.
func registerCodeFromDecoded(decoded any) byte {
	switch m := decoded.(type) {
	case *Reg02Decoded:
		return m.Register
	case *Reg03Decoded:
		return m.Register
	case *Reg04ReadDecoded:
		return m.Register
	case *Reg04WriteDecoded:
		return m.Register | 0x80 // 0x04 → 0x84 (write 구분)
	default:
		return 0
	}
}

// nonComparableEmitKeys 는 register-decoded change detection 비교에서 제외할 메타
// 필드들이다 — 매 frame 마다 변동하지만 의미 변화가 아닌 메타 정보.
//   - timestamp_ms / seq: 매 frame 변동
//   - dev_id: cache key 의 일부 (동일 dev_id 끼리만 비교하므로 중복)
//
// 이것들이 제거된 후 남은 payload (state / inferred / unknown 그룹 + register/direction
// 등) 가 의미 변화 비교 대상이다.
var nonComparableEmitKeys = map[string]struct{}{
	"timestamp_ms": {},
	"seq":          {},
	"dev_id":       {},
}

// extractComparablePayload 는 transformed JSON 에서 비교 대상 메타 필드 (timestamp_ms /
// seq / dev_id) 를 제거한 결과 bytes 를 반환한다 (v0.3.6 change detection).
//
// 제거 후 남은 key 가 0 개면 nil 반환 — 의미 데이터가 없는 메시지로 분류 (emit 안 함).
// 일반적으로 Reg04Read 처럼 모든 필드가 inferred 인데 include_inferred_fields=false 인
// 경우 state 그룹조차 없는 빈 메시지가 됨. 이런 메시지는 사용자 trace 노이즈이므로 차단.
func extractComparablePayload(transformed []byte) []byte {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(transformed, &m); err != nil {
		return nil
	}
	for k := range nonComparableEmitKeys {
		delete(m, k)
	}
	if len(m) == 0 {
		return nil
	}
	out, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return out
}

// shouldEmitRegisterChange 는 (dev_id, register) 별 마지막 emit 된 비교 payload 와 비교하여
// 의미 있는 변화가 있는 경우에만 true 를 반환한다 (v0.3.6 register-decoded change detection).
//
// timestamp_ms / seq / dev_id 는 비교 대상 외 (extractComparablePayload 가 제거).
// 비교 후 남은 의미 payload 가 없으면 (모든 옵션 그룹 비활성 + register-info=false 인
// Reg04Read 처럼 빈 메시지) emit 안 함. captureLoop 단일 goroutine 에서 호출하므로
// mutex 불필요.
func (a *CenturyAgent) shouldEmitRegisterChange(devID, register byte, transformed []byte) bool {
	payload := extractComparablePayload(transformed)
	if payload == nil {
		// 의미 데이터가 없는 메시지 (state / inferred / unknown 그룹 모두 비어있고 메타도
		// 옵션에 따라 제거됨). 사용자 trace 노이즈이므로 emit 안 함.
		return false
	}
	key := registerEmitKey{DevID: devID, Register: register}
	prev, seen := a.lastRegisterEmit[key]
	if seen && bytes.Equal(prev, payload) {
		return false // 동일 payload — emit skip
	}
	dup := make([]byte, len(payload))
	copy(dup, payload)
	a.lastRegisterEmit[key] = dup
	return true
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

	// Reset reportStopCh on (re)start so that consecutive Start/Stop cycles work.
	a.mu.Lock()
	select {
	case <-a.reportStopCh:
		a.reportStopCh = make(chan struct{})
	default:
	}
	a.mu.Unlock()

	go a.captureLoop()
	go a.offlineWatchLoop()
	// v0.3.0 (REQ-CENTURY-035): keepalive ticker is conditional on EmitDeviceState
	// and ReportInterval > 0. The loop self-checks and returns early if disabled,
	// so it's safe to spawn unconditionally.
	go a.reportLoop()
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
	// Signal keepalive loop to exit too (it also selects on stopCh, but a dedicated
	// channel makes it explicit and survives future refactors that may decouple
	// keepalive from capture lifecycle).
	a.mu.Lock()
	select {
	case <-a.reportStopCh:
	default:
		close(a.reportStopCh)
	}
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
//
// v0.6.3: NodeID / FlowID 추가 — 노드별 통계 (IncrNodeRef*) 추적에 사용.
type centuryProcessRequest struct {
	Command string `json:"command"`
	Count   int    `json:"count,omitempty"`
	LastSeq uint64 `json:"last_seq,omitempty"`
	NodeID  string `json:"node_id,omitempty"`
	FlowID  string `json:"flow_id,omitempty"`
}

// Process 는 JSON 명령을 처리한다. 본 에이전트는 패시브 캡처 전용이므로 어떠한 경우에도
// transport.Write() 를 호출하지 않는다 (AC-B9, REQ-CENTURY-017).
//
// 지원 명령: get_stats / get_recent / drain / drain_device_state (v0.3.11).
// 그 외 모든 명령은 not_supported 응답.
//
// v0.6.3: 내부간 송수신 통계 (Internal Messages Received/Sent + NodeRef) 갱신.
// NASA 와 동일 패턴 — 노드별 호출 횟수가 Web UI 의 노드 통계에 표시되도록.
func (a *CenturyAgent) Process(data []byte) ([]byte, error) {
	var req centuryProcessRequest
	if err := json.Unmarshal(data, &req); err != nil {
		// Invalid JSON 도 통계상 received 1건 + errored 1건으로 카운트.
		a.stats.IncrInternalMessagesReceived()
		a.stats.IncrInternalMessagesErrored()
		return nil, fmt.Errorf("century process: invalid request: %w", err)
	}
	// 노드→에이전트 호출 카운트 (송수신 모두).
	a.stats.IncrInternalMessagesReceived()
	a.stats.IncrInternalMessagesSent() // 응답을 보낼 것이므로 동시에 sent 카운트.
	if req.NodeID != "" {
		a.stats.IncrNodeRefReceived(req.NodeID, req.FlowID)
		a.stats.IncrNodeRefSent(req.NodeID, req.FlowID)
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
	case "drain_device_state":
		// v0.3.11: polling 노드가 device_state (change/keepalive) 이벤트를
		// 가져오기 위한 비파괴 drain. msgCh 와 독립적인 buffer 를 비운다.
		return a.processDrainDeviceState()
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
		// v0.3.0 device-centric emit counters (REQ-CENTURY-033/034/035).
		"device_state_emits":   a.cStats.deviceStateEmits.Load(),
		"change_emits":         a.cStats.changeEmits.Load(),
		"keepalive_emits":      a.cStats.reportEmits.Load(),
		"device_state_dropped": a.cStats.deviceStateDropped.Load(),
	}
	return json.Marshal(stats)
}

// capturedFrameEvent 는 get_recent / drain 응답의 한 entry 이다.
//
// v0.3.5: RawHex / FunctionCode / Register 는 omitempty 적용 — IncludeRawHex /
// IncludeRegisterInfo 옵션 비활성 시 frameToEvent 가 빈 값으로 설정하여 JSON 에서 제외.
type capturedFrameEvent struct {
	Seq          uint64          `json:"seq"`
	TimestampMs  int64           `json:"timestamp_ms"`
	RawHex       string          `json:"raw_hex,omitempty"`
	FunctionCode byte            `json:"function_code,omitempty"`
	Register     *byte           `json:"register,omitempty"`
	Decoded      json.RawMessage `json:"decoded,omitempty"`
	DecodeError  string          `json:"decode_error,omitempty"`
}

// frameToEvent 는 CapturedFrame 을 직렬화 가능한 event 로 변환한다.
//
// v0.3.5: decoded payload 를 state-grouped 구조로 재구성 + register/raw_hex 옵션 적용.
//   - confirmed 필드: state 그룹에 value 만 평탄화 (alias 포함)
//   - inferred 필드: includeInferred=true 시 inferred 그룹
//   - unknown 필드: includeUnknown=true 시 unknown 그룹
//   - register/direction: includeRegisterInfo=true 시에만 노출
//   - raw_hex: includeRawHex=true 시에만 노출 (capturedFrameEvent.RawHex omitempty)
//   - function_code: includeRegisterInfo 와 묶어서 제어 (register-level 메타)
func frameToEvent(c CapturedFrame, includeInferredFields, includeUnknownFields, includeRegisterInfo, includeRawHex bool) capturedFrameEvent {
	ev := capturedFrameEvent{
		Seq:         c.Seq,
		TimestampMs: c.ReceivedAt.UnixMilli(),
	}
	if includeRawHex {
		ev.RawHex = hex.EncodeToString(c.Raw)
	}
	if c.Frame != nil && includeRegisterInfo {
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
			if transformed, terr := transformDecodedPayload(b, includeInferredFields, includeUnknownFields, includeRegisterInfo); terr == nil {
				b = transformed
			}
			ev.Decoded = b
		}
	}
	return ev
}

// processGetRecent 는 ring buffer 의 최근 count 프레임을 lastSeq 이후만 필터링하여 반환한다 (AC-B7, 비파괴).
func (a *CenturyAgent) processGetRecent(count int, lastSeq uint64) ([]byte, error) {
	cfg := a.snapshotConfig()
	recs := a.ringBuffer.GetRecent(count)
	out := make([]capturedFrameEvent, 0, len(recs))
	for _, c := range recs {
		if lastSeq > 0 && c.Seq <= lastSeq {
			continue
		}
		if ev, ok := a.frameToEventIfChanged(c, cfg); ok {
			out = append(out, ev)
		}
	}
	return json.Marshal(map[string]any{"count": len(out), "frames": out})
}

// processDrain 은 ring buffer 의 모든 프레임을 반환하고 버퍼를 비운다 (AC-B8).
func (a *CenturyAgent) processDrain() ([]byte, error) {
	cfg := a.snapshotConfig()
	recs := a.ringBuffer.Drain()
	out := make([]capturedFrameEvent, 0, len(recs))
	for _, c := range recs {
		if ev, ok := a.frameToEventIfChanged(c, cfg); ok {
			out = append(out, ev)
		}
	}
	return json.Marshal(map[string]any{"count": len(out), "frames": out})
}

// frameToEventIfChanged 는 frameToEvent + change detection 을 한 번에 처리한다.
//
// v0.3.8: century-status 노드의 processGetRecent / processDrain 출력에도 dedup 적용.
// captureLoop msgCh emit 과 같은 lastRegisterEmit 캐시를 공유한다.
//
//   - ACKDecoded 는 의미 없는 응답이므로 항상 skip (사용자 trace 노이즈 제거).
//   - state / inferred / unknown / register-meta 가 이전과 동일한 frame 은 skip.
//   - 빈 의미 메시지 (extractComparablePayload 가 nil 반환) 도 skip.
//   - decode 실패 frame 은 변화 비교 불가하므로 그대로 emit (trace 가치 있음).
func (a *CenturyAgent) frameToEventIfChanged(c CapturedFrame, cfg CenturyConfig) (capturedFrameEvent, bool) {
	if c.Decoded != nil {
		if _, isACK := c.Decoded.(*ACKDecoded); isACK {
			return capturedFrameEvent{}, false
		}
	}
	ev := frameToEvent(c, cfg.IncludeInferredFields, cfg.IncludeUnknownFields, cfg.IncludeRegisterInfo, cfg.IncludeRawHex)
	if c.Decoded != nil {
		if subDevID, ok := subDevIDFromDecoded(c.Decoded); ok {
			register := registerCodeFromDecoded(c.Decoded)
			a.emitMu.Lock()
			emit := a.shouldEmitRegisterChange(subDevID, register, ev.Decoded)
			a.emitMu.Unlock()
			if !emit {
				return capturedFrameEvent{}, false
			}
		}
	}
	return ev, true
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
//
// v0.6.3: msgCh 에서 한 건을 꺼내 bridge 컨슈머에게 전달한 것은 "에이전트 →
// 노드 internal sent" 1 건으로 카운트된다 (NASA / LGCNP 패턴).
func (a *CenturyAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	a.bridgeActive.Store(true)
	select {
	case data, ok := <-a.msgCh:
		if !ok {
			return nil, ErrAgentStopped
		}
		a.stats.IncrInternalMessagesSent()
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

// DeviceProvider 는 이 에이전트의 디바이스를 unified device.DeviceProvider 로
// 노출한다. cmd/xflowd/main.go 의 deviceRegistry 가 이 메서드를 type assertion
// 으로 감지하여 시스템-wide device list 에 century 디바이스를 등록한다.
// NASA / LGCNP 와 동일한 패턴이며, 이 메서드가 누락되면 web UI 의 device list
// 에서 century 디바이스가 표시되지 않는다.
func (a *CenturyAgent) DeviceProvider() device.DeviceProvider {
	return NewCenturyDeviceProvider(a)
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
		// Standard agent.AgentStats — UI 통계 카운터 (messages_in / bytes_read / last_activity).
		// NASA/LGCNP 와 동일 패턴. 본 호출이 없으면 Web UI 의 메시지 수신 / 바이트 / 최근 활동
		// 시각이 영구 0 으로 표시된다 (사용자 보고).
		a.stats.IncrExternalMessagesReceived()
		a.stats.AddBytesRead(int64(len(rawCopy)))
		a.stats.UpdateLastActivity()

		// Snapshot config under lock to avoid races with Configure().
		cfg := a.snapshotConfig()

		// Decode.
		decoded, decodeErr := Decode(f, now.UnixMilli())
		if decodeErr != nil {
			a.cStats.decodeErrors.Add(1)
			// v0.6.3: 표준 AgentStats 의 ExternalMessagesErrored 도 함께 증가 — Web UI 의
			// 에러 카운터 갱신.
			a.stats.IncrExternalMessagesErrored()
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
		//
		// 1차: decoded message 에서 sub_dev_id 추출하여 device state 까지 갱신.
		// 2차 (fallback): decoded == nil 이지만 frame payload prefix 에 sub_dev_id 가
		// 있으면 device 자동 발견만이라도 처리한다. 이는 다음 케이스를 커버한다:
		//  - 마스터의 READ request (FCRead=0x0B) — decoder 가 ErrUnsupportedDirection 반환
		//  - 미지원 register 응답 — decoder 가 ErrUnknownRegister 반환
		//  - decode 가 실패한 frame (CRC 는 통과했으나 payload 형식 미준수)
		// 사용자가 보고한 "자동 디바이스 등록 안됨" 증상 — 실제 환경에서 Slave 응답이
		// sniff 라인에서 누락되거나 형식이 약간 다른 경우, READ request 만으로도
		// device map 에 sub_dev_id 가 등록되어야 한다.
		if decoded != nil {
			a.touchDeviceFromDecoded(decoded, f, now, cfg.AutoDiscovery)
		} else if subDevID, ok := subDevIDFromFrame(f); ok {
			a.touchDeviceFromSubDevID(subDevID, now, cfg.AutoDiscovery)
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
			// v0.6.3: 표준 AgentStats 의 DroppedMessages 도 증가 — Web UI 운영 통계 갱신.
			a.stats.IncrDroppedMessages()
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

		// v0.5.1 Breaking: register-decoded msgCh emit 경로 제거.
		// 모든 register state 는 device_state event 의 state 그룹으로 통합되었다
		// (TempEvapAC / TempEvapBC 포함). 운영자는 device_state 단일 stream 만 소비.
		// raw frame 이 필요한 RE/디버깅은 century-raw-frame 노드를 사용한다.
		_ = emitDecoded // dedupe 통계는 유지하나 emit 결정에는 더 이상 영향 없음.

		// v0.3.0 (REQ-CENTURY-033/034/035): device-centric DeviceStateEvent emit.
		// Triggered by frame decode when a sub_dev_id is available, regardless of
		// the message type. The change detector compares against lastEmitState and
		// emits with trigger="change" only when one of the 5 core fields differs
		// (or it's the first emit, or online transition occurred).
		if cfg.EmitDeviceState && decoded != nil {
			if subDevID, ok := subDevIDFromDecoded(decoded); ok {
				a.maybeEmitDeviceState(subDevID, now, "")
			}
		}
	}
}

// centuryFieldAliases 는 register-decoded 필드명을 NASA/Century device_state 의 통일된
// 5 핵심 필드명으로 매핑한다 (v0.3.4 — 5종 에이전트 schema 통일 작업의 일환).
//
// 적용 위치: transformDecodedPayload 가 confirmed/inferred/unknown 그룹을 빌드할 때.
// register-level raw 필드명 (SPEC §6 의 setpoint_c 등) → device-level 통일 명 (target_temp 등).
var centuryFieldAliases = map[string]string{
	"setpoint_c": "target_temp",  // Reg02 설정온도 (NASA TargetTemp 와 통일)
	"temp_A_c":   "current_temp", // Reg04 실내온도 (NASA CurrentTemp 와 통일)
	"fan":        "fan_speed",    // Reg02 풍량 (NASA FanSpeed 와 통일)
	// mode 는 이미 통일됨
	// temp_evap_a_c / temp_evap_b_c 는 device-level state 가 아니므로 alias 없음 (그대로 노출)
}

func applyCenturyAlias(k string) string {
	if alias, ok := centuryFieldAliases[k]; ok {
		return alias
	}
	return k
}

// registerMetaTopLevelKeys 는 register-level 메타데이터로 분류되어 IncludeRegisterInfo=false
// 일 때 출력에서 제거되는 top-level 키 집합이다 (v0.3.5). dev_id, timestamp_ms, state 그룹은
// 운영 trace 의 핵심이므로 옵션과 무관하게 항상 출력된다.
var registerMetaTopLevelKeys = map[string]struct{}{
	"register":  {},
	"direction": {},
}

// transformDecodedPayload 는 register-decoded JSON 페이로드를 v0.3.4 의 state-grouped
// 구조로 재구성한다 (5종 에이전트 schema 통일).
//
// 입력 (raw register-decoded JSON):
//
//	{"mode":{"raw":0,"status":"confirmed","value":"off"}, "fan":{"status":"confirmed","value":0},
//	 "setpoint_c":{"raw":270,"status":"confirmed","value":27}, "op_val_1":{"status":"inferred","value":0},
//	 "reg02_byte_0":{"status":"unknown","value":0}, "register":2, "sub_dev_id":59, ...}
//
// 출력 (default = include_inferred=false, include_unknown=false):
//
//	{"state":{"mode":"off","fan_speed":0,"target_temp":27}, "register":2, "sub_dev_id":59, ...}
//
// 출력 (include_inferred=true):
//
//	{"state":{...}, "inferred":{"op_val_1":0, ...}, "register":2, ...}
//
// 출력 (include_unknown=true):
//
//	{"state":{...}, "unknown":{"reg02_byte_0":0, ...}, "register":2, ...}
//
// 규칙:
//   - confirmed 필드는 value 만 추출하여 state 그룹으로 평탄화 (raw/status 메타데이터 제거)
//   - centuryFieldAliases 로 register-level 필드명을 device-level 통일명으로 매핑
//     (setpoint_c→target_temp, temp_A_c→current_temp, fan→fan_speed)
//   - inferred 필드는 includeInferred=true 일 때만 별도 inferred 그룹으로 (alias 미적용)
//   - unknown 필드는 includeUnknown=true 일 때만 별도 unknown 그룹으로 (alias 미적용)
//   - 비-nested 필드 (register, sub_dev_id, raw_hex, seq, timestamp_ms, direction) 는 top-level 유지
//
// 입력이 valid JSON object 가 아니거나 파싱 실패 시 원본을 그대로 반환한다 (best-effort).
//
// v0.3.5: includeRegisterInfo=false 일 때 register/direction 같은 register-level 메타데이터를
// 제거한다. dev_id/timestamp_ms/state 그룹은 옵션과 무관하게 항상 출력.
func transformDecodedPayload(payload []byte, includeInferred, includeUnknown, includeRegisterInfo bool) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(payload, &m); err != nil {
		return payload, err
	}
	state := make(map[string]any)
	inferred := make(map[string]any)
	unknown := make(map[string]any)
	rest := make(map[string]json.RawMessage)

	for k, raw := range m {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			// Not a nested object — keep at top-level (register, sub_dev_id, raw_hex, seq, ...).
			// v0.3.5: register/direction 은 includeRegisterInfo=false 시 제거.
			if !includeRegisterInfo {
				if _, isRegisterMeta := registerMetaTopLevelKeys[k]; isRegisterMeta {
					continue
				}
			}
			rest[k] = raw
			continue
		}
		statusRaw, hasStatus := obj["status"]
		if !hasStatus {
			rest[k] = raw
			continue
		}
		var statusStr string
		if err := json.Unmarshal(statusRaw, &statusStr); err != nil {
			rest[k] = raw
			continue
		}
		// value 추출 (있으면). 없으면 nested 그대로.
		var val any
		if valRaw, ok := obj["value"]; ok {
			_ = json.Unmarshal(valRaw, &val)
		} else {
			// value 가 없는 케이스 — 전체 객체를 그대로 (드뭄).
			_ = json.Unmarshal(raw, &val)
		}
		switch statusStr {
		case "confirmed":
			state[applyCenturyAlias(k)] = val
		case "inferred":
			if includeInferred {
				inferred[k] = val
			}
		case "unknown":
			if includeUnknown {
				unknown[k] = val
			}
		default:
			// 알 수 없는 status 값 — top-level 보존.
			rest[k] = raw
		}
	}

	out := make(map[string]any, len(rest)+3)
	for k, raw := range rest {
		out[k] = raw
	}
	if len(state) > 0 {
		out["state"] = state
	}
	if includeInferred && len(inferred) > 0 {
		out["inferred"] = inferred
	}
	if includeUnknown && len(unknown) > 0 {
		out["unknown"] = unknown
	}
	return json.Marshal(out)
}

// emitToMsgCh sends payload bytes to msgCh with drop-oldest semantics on full.
// dropCounter (if non-nil) is incremented on drop. Used by both register-decoded
// and device_state emit paths to share the same channel discipline.
func (a *CenturyAgent) emitToMsgCh(b []byte, dropCounter *atomic.Uint64) {
	select {
	case a.msgCh <- b:
		return
	default:
	}
	// Channel full: drop oldest, try once more.
	select {
	case <-a.msgCh:
	default:
	}
	select {
	case a.msgCh <- b:
	default:
		if dropCounter != nil {
			dropCounter.Add(1)
		}
	}
}

// maybeEmitDeviceState builds a fresh device_state snapshot and emits to msgCh
// when one of the 5 core fields differs from lastEmitState[subDevID] or when
// the online state changed (REQ-CENTURY-033/035). Always emits on first observation.
//
// triggerOverride: pass "keepalive" from the keepalive ticker to force-emit
// regardless of equality. Empty string lets the function pick "change" or no-op.
//
// Thread-safe via emitMu. Caller must hold no locks on the agent or devices.
//
// v0.4.1: Reg02 (power/mode/fan/target_temp 의 원천) 미수신 상태에서는 device_state
// emit 을 건너뛴다.
//
// v0.4.2: Reg04Read (current_temp 의 원천) 도 함께 gate. 사용자 보고: 첫 emit 이
// current_temp=0 으로 나온 후 직후 emit 에서 25.2 로 정정되는 결함. 사용자 요구
// "초기값이 없으며, 값이 설정되지 않으면 반환하지 않음" 에 부합하도록 5 핵심 필드
// 의 모든 원천 register (Reg02 + Reg04Read) 가 적어도 한 번 관측된 후에만 emit.
//
// 정상 시나리오: master 의 cycle (~512ms) 안에 Reg02/Reg03/Reg04 가 모두 polling
// 되므로 첫 emit 까지 최대 ~512ms 대기. 매우 드문 케이스 (예: master 가 Reg02 만
// polling) 에서는 emit 이 영구 지연될 수 있으나, 그 경우 5 핵심 중 current_temp
// 가 미정의이므로 emit 보류가 의미 보존에 더 부합한다.
func (a *CenturyAgent) maybeEmitDeviceState(subDevID byte, now time.Time, triggerOverride string) {
	a.devicesMu.RLock()
	dev, ok := a.devices[subDevID]
	a.devicesMu.RUnlock()
	if !ok || dev == nil {
		return
	}
	devSnap := dev.Snapshot()
	// v0.4.1/v0.4.2: Reg02 + Reg04Read 모두 수신된 후에만 emit.
	// 그 전에는 5 핵심 중 일부가 0/fallback 으로 노출되어 운영자가 오해할 위험이 있음.
	// keepalive emit 도 lastEmitSeen 가드로 함께 차단됨.
	if devSnap.State == nil || devSnap.State.Reg02 == nil || devSnap.State.Reg04Read == nil {
		return
	}
	snap := BuildDeviceStateSnapshot(devSnap.State, devSnap.Online)

	a.emitMu.Lock()
	prev, hasPrev := a.lastEmitState[subDevID]
	prevOnline, hasPrevOnline := a.lastEmitOnline[subDevID]
	seen := a.lastEmitSeen[subDevID]

	trigger := triggerOverride
	if trigger == "" {
		// Decide: change or no-op.
		if !seen {
			trigger = TriggerChange
		} else if hasPrev && !prev.Equals(snap) {
			trigger = TriggerChange
		} else if hasPrevOnline && prevOnline != snap.Online {
			trigger = TriggerChange
		} else {
			a.emitMu.Unlock()
			return
		}
	}

	// v0.6.6: event_temp_threshold gate — change 트리거이면서 비온도 필드는
	// 변화 없고 online 도 동일한 경우, |Δcurrent_temp| < threshold 면 emit suppress.
	// 정기 보고(TriggerReport) 와 첫 emit (!seen) 은 게이트 적용 대상이 아니다.
	if trigger == TriggerChange && seen && hasPrev {
		cfg := a.snapshotConfig()
		if cfg.EventTempThreshold > 0 && prev.EqualsExceptCurrentTemp(snap) {
			onlineUnchanged := !hasPrevOnline || prevOnline == snap.Online
			if onlineUnchanged && a.lastReportTempSet[subDevID] {
				delta := snap.CurrentTemp - a.lastReportTemp[subDevID]
				if delta < 0 {
					delta = -delta
				}
				if float64(delta) < cfg.EventTempThreshold {
					a.emitMu.Unlock()
					return
				}
			}
		}
	}

	// Update bookkeeping under lock so concurrent keepalive ticker sees fresh state.
	a.lastEmitState[subDevID] = snap
	a.lastEmitTime[subDevID] = now
	a.lastEmitOnline[subDevID] = snap.Online
	a.lastEmitSeen[subDevID] = true
	// v0.6.6: lastReportTemp 는 emit 이 실제로 발생한 모든 시점에 갱신
	// (change/report 무관). 다음 임계값 비교의 기준점이 된다.
	a.lastReportTemp[subDevID] = snap.CurrentTemp
	a.lastReportTempSet[subDevID] = true
	// v0.3.10: lastReportTime 갱신 규칙
	//   - change: 첫 emit (anchor) 일 때만 초기화. 이후 change 는 갱신하지 않음.
	//   - keepalive: 항상 now 로 갱신 → 다음 interval 의 기준점.
	// 결과: change 가 자주 일어나도 keepalive 는 interval 마다 fire.
	if trigger == TriggerReport {
		a.lastReportTime[subDevID] = now
	} else if _, hasAnchor := a.lastReportTime[subDevID]; !hasAnchor {
		a.lastReportTime[subDevID] = now
	}
	a.emitMu.Unlock()

	// v0.5.0: device_state 는 여러 register frame 의 종합이므로 단일 raw_hex 가 없다.
	// include_raw_hex 옵션은 register-decoded 메시지에만 적용되며, device_state 의
	// raw_hex 는 빈 string (omitempty 로 자동 제외).
	ev := NewDeviceStateEvent(snap, subDevID, devSnap.Label, devSnap.LastSeen.UnixMilli(), trigger, "")
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	a.cStats.deviceStateEmits.Add(1)
	switch trigger {
	case TriggerReport:
		a.cStats.reportEmits.Add(1)
	default:
		a.cStats.changeEmits.Add(1)
	}
	a.emitToMsgCh(b, &a.cStats.deviceStateDropped)
	// v0.3.11: polling-friendly buffer 에도 push (msgCh 와 독립).
	// century-status 노드 등 polling 경로의 사용자가 keepalive/change device_state 를
	// 볼 수 있도록 한다. msgCh 의 Bridge 컨슈머와 경쟁하지 않음.
	a.pushDeviceStateBuf(b)
}

// pushDeviceStateBuf 는 polling-friendly buffer 에 device_state JSON 을 추가한다 (v0.3.11).
// 버퍼가 가득 차면 가장 오래된 항목부터 drop 한다 (drop-oldest semantics).
// deviceStateBufMax <= 0 이면 push 자체를 무시한다 (no-op).
func (a *CenturyAgent) pushDeviceStateBuf(b []byte) {
	if a.deviceStateBufMax <= 0 {
		return
	}
	// caller 가 b 를 재사용하지 않도록 copy 보관.
	cp := make([]byte, len(b))
	copy(cp, b)
	a.deviceStateBufMu.Lock()
	if len(a.deviceStateBuf) >= a.deviceStateBufMax {
		// drop oldest.
		drop := len(a.deviceStateBuf) - a.deviceStateBufMax + 1
		a.deviceStateBuf = a.deviceStateBuf[drop:]
	}
	a.deviceStateBuf = append(a.deviceStateBuf, cp)
	a.deviceStateBufMu.Unlock()
}

// processDrainDeviceState 는 device_state polling buffer 를 비파괴적으로 drain 한다 (v0.3.11).
// polling node (century-status / century / century-raw-frame) 의 pollLoop 에서 호출된다.
//
// 응답 형식:
//
//	{"count": N, "events": [event1_json, event2_json, ...]}
//
// 각 event 는 device_state JSON 원본 (NewDeviceStateEvent marshal 결과).
func (a *CenturyAgent) processDrainDeviceState() ([]byte, error) {
	a.deviceStateBufMu.Lock()
	events := a.deviceStateBuf
	a.deviceStateBuf = nil
	a.deviceStateBufMu.Unlock()
	out := make([]json.RawMessage, len(events))
	for i, b := range events {
		out[i] = json.RawMessage(b)
	}
	return json.Marshal(map[string]any{
		"count":  len(out),
		"events": out,
	})
}

// reportLoop fires device_state keepalive emits when a device has not been
// touched for the configured keepalive interval (REQ-CENTURY-035).
//
// Granularity: 1s ticker for ReportInterval >= 1s, otherwise ReportInterval/4
// (min 25ms) to keep tests with sub-second intervals responsive without spinning.
//
// Disabled when EmitDeviceState=false or ReportInterval<=0.
func (a *CenturyAgent) reportLoop() {
	cfg := a.snapshotConfig()
	if !cfg.EmitDeviceState || cfg.ReportInterval <= 0 {
		return
	}
	interval := 1 * time.Second
	if cfg.ReportInterval < interval {
		interval = cfg.ReportInterval / 4
		if interval < 25*time.Millisecond {
			interval = 25 * time.Millisecond
		}
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-a.stopCh:
			return
		case <-a.reportStopCh:
			return
		case <-ticker.C:
			a.checkReportEmits()
		}
	}
}

// checkReportEmits iterates devices and emits trigger="keepalive" for any
// device whose keepalive due-time has passed.
//
// v0.3.9: ReportMode 에 따라 due-time 계산이 달라진다.
//   - "relative" (기본): now - lastEmitTime >= ReportInterval
//   - "absolute": 직전 wall-clock 정렬 시점 (floor(now/interval)*interval) 가
//     마지막 emit 시점 이후이면 emit. 즉 매 interval 의 배수 시점에 한 번씩 emit.
//
// Devices that have never had a first emit (lastEmitSeen=false) are skipped —
// they will receive a change emit on the first frame.
func (a *CenturyAgent) checkReportEmits() {
	cfg := a.snapshotConfig()
	if !cfg.EmitDeviceState || cfg.ReportInterval <= 0 {
		return
	}
	now := a.now()

	a.devicesMu.RLock()
	ids := make([]byte, 0, len(a.devices))
	for id := range a.devices {
		ids = append(ids, id)
	}
	a.devicesMu.RUnlock()

	for _, id := range ids {
		a.emitMu.Lock()
		// v0.3.10: keepalive 는 lastReportTime 을 기준으로 fire 한다 (change 와 독립).
		// 첫 change emit 이 lastReportTime 을 anchor 로 설정한 뒤부터 fire 가능.
		lastKA, hasKA := a.lastReportTime[id]
		seen := a.lastEmitSeen[id]
		a.emitMu.Unlock()
		if !seen || !hasKA {
			continue
		}
		if !shouldReportFire(now, lastKA, cfg.ReportInterval, cfg.ReportMode) {
			continue
		}
		a.maybeEmitDeviceState(id, now, TriggerReport)
	}
}

// shouldReportFire 는 현재 시각, 마지막 emit 시각, interval, mode 를 받아
// keepalive emit 이 필요한지 여부를 반환한다 (v0.3.9).
//
//   - "absolute": wall-clock 정렬 — now.Truncate(interval) 가 lastTime 보다 이후이면 fire.
//     예: interval=60s, last=12:34:50 (간격 50), now=12:35:05 → truncate=12:35:00 > last → fire.
//     디바이스가 여러 대일 때 같은 정렬 시점에 동기 emit 된다.
//   - "relative" (default): now - lastTime >= interval.
//
// interval <= 0 이면 absolute 도 의미 없으므로 false 반환 (caller 가 사전 가드).
func shouldReportFire(now, lastTime time.Time, interval time.Duration, mode string) bool {
	if interval <= 0 {
		return false
	}
	if mode == "absolute" {
		boundary := now.Truncate(interval)
		return boundary.After(lastTime)
	}
	// "relative" 또는 기타 (default).
	return now.Sub(lastTime) >= interval
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

// subDevIDFromFrame 은 decode 실패한 frame 으로부터 sub_dev_id 를 추출한다.
// payload prefix 가 존재하는 모든 non-ACK frame (READ request 포함) 에 적용된다.
// ACK (payload length 1) 는 prefix 가 없으므로 (0, false) 반환.
func subDevIDFromFrame(f *Frame) (byte, bool) {
	if f == nil || f.IsACK() || len(f.Payload) < 1 {
		return 0, false
	}
	return f.Payload[0], true
}

// touchDeviceFromSubDevID 는 sub_dev_id 만으로 device 자동 발견을 수행한다 (state update 없음).
// decode 실패 frame 의 fallback 경로에서 사용된다 — Reg*Decoded 가 없으므로 Update 는 호출하지 않고
// LastSeen 만 갱신한다.
func (a *CenturyAgent) touchDeviceFromSubDevID(subDevID byte, now time.Time, autoDiscovery bool) {
	a.devicesMu.Lock()
	dev, exists := a.devices[subDevID]
	if !exists {
		if !autoDiscovery {
			a.devicesMu.Unlock()
			return
		}
		dev = NewCenturyDevice(subDevID, "auto", now)
		a.devices[subDevID] = dev
		a.cStats.devicesDiscovered.Add(1)
		a.logger.Info("century: 디바이스 자동 발견 (frame prefix)", "sub_dev_id", fmt.Sprintf("0x%02X", subDevID))
	}
	a.devicesMu.Unlock()
	dev.Touch(now)
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
//
// v0.3.0 (REQ-CENTURY-035): online → false 전이 시 emit_device_state=true 이면 즉시
// `trigger="change"` 로 device_state 를 emit 한다 (downstream 에 offline 알림).
func (a *CenturyAgent) checkDeviceTimeouts() {
	cfg := a.snapshotConfig()
	now := a.now()
	timeout := cfg.OfflineTimeout
	a.devicesMu.RLock()
	devicesSnapshot := make([]*CenturyDevice, 0, len(a.devices))
	for _, d := range a.devices {
		devicesSnapshot = append(devicesSnapshot, d)
	}
	a.devicesMu.RUnlock()
	for _, d := range devicesSnapshot {
		if d.IsStale(now, timeout) {
			d.mu.Lock()
			transitionedToOffline := false
			if d.Online {
				d.Online = false
				transitionedToOffline = true
			}
			subDevID := d.SubDevID
			d.mu.Unlock()
			if transitionedToOffline {
				a.logger.Info("century: 디바이스 오프라인",
					"sub_dev_id", fmt.Sprintf("0x%02X", subDevID),
				)
				if fn := a.onDeviceStateChange; fn != nil {
					go fn(a.Name(), fmt.Sprintf("%s:%02x", a.Name(), subDevID))
				}
				// v0.3.0: emit immediate device_state with trigger="change"
				// reflecting online=false (REQ-CENTURY-035, AC-H7).
				if cfg.EmitDeviceState {
					a.maybeEmitDeviceState(subDevID, now, "")
				}
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
