package lg

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
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// LGCP 패시브 패킷 캡처 에이전트
// ---------------------------------------------------------------------------

// LGCPAgent 는 LG 내부 제어 프로토콜(LGCP) 패킷을 패시브하게 캡처하는 에이전트이다.
// 시리얼 버스를 리스닝만 하며, 절대로 데이터를 전송하지 않는다.
// agent.Agent, agent.MessageReceiver, agent.StatefulAgent,
// agent.BufferInfoProvider, agent.TransportChecker 인터페이스를 구현한다.
type LGCPAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig agent.AgentConfig
	lgcpConfig  LGCPConfig
	transport   LGAPTransport // 기존 시리얼 트랜스포트 재사용
	stopCh      chan struct{}
	msgCh       chan []byte // Bridge 메시지 (ReceiveMessage)
	stats       *agent.AgentStats
	logger      *slog.Logger
	mu          sync.RWMutex
	startedAt   time.Time
	createdAt   time.Time
	paused      bool

	// 재연결 상태
	reconnectMu       sync.Mutex
	isReconnecting    bool
	reconnectAttempts int

	// 캡처 통계 (atomic)
	framesCaptured atomic.Int64
	framesValid    atomic.Int64
	framesInvalid  atomic.Int64
	framesDropped  atomic.Int64
	bytesReceived  atomic.Int64

	// 최근 프레임 링 버퍼 (get_recent 명령용)
	recentMu     sync.RWMutex
	recentFrames []lgcpFrameRecord
	recentIdx    int
	recentFull   bool
	recentNotify chan struct{} // 새 프레임 도착 알림 (폴링 노드용)

	// Bridge 소비자 활성 여부 (ReceiveMessage 호출 시 true)
	bridgeActive atomic.Bool

	// 드롭 로그 rate-limit
	lastDropLog atomic.Int64 // UnixNano

	// 디바이스 관리
	devices      map[string]*LGCPDevice     // 주소(hex) → 디바이스
	lastStates   map[string]LGCPDeviceState // 주소(hex) → 이전 상태 (변경 감지용)
	notifyTicker *time.Ticker               // 주기적 상태 보고 타이머

	// 콜백
	onDeviceStateChange func(agentName, deviceID string)

	// 제어 기능 (SPEC-LGCP-002)
	writeMu        sync.Mutex
	frameBuilder   *LGCPFrameBuilder
	seqManager     *LGCPSequenceManager // Controller→Unit 방향 SEQ 추적
	unitSeqManager *LGCPSequenceManager // Unit→Controller 방향 SEQ 추적 (서모스탯 사칭용)
	lastSentFrame  []byte
	lastSentTime   time.Time
	lastRecvTime   atomic.Int64 // UnixNano — 마지막 프레임 수신 시각 (버스 충돌 방지)
}

// lgcpFrameRecord 는 링 버퍼에 저장되는 프레임 레코드이다.
type lgcpFrameRecord struct {
	Event     json.RawMessage `json:"event"`
	Timestamp time.Time       `json:"timestamp"`
	Seq       int64           `json:"seq"` // 캡처 시퀀스 (last_seq 필터링용)
}

// lgcpRecentBufferSize 는 최근 프레임 링 버퍼의 크기이다.
const lgcpRecentBufferSize = 64

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*LGCPAgent)(nil)
var _ agent.MessageReceiver = (*LGCPAgent)(nil)
var _ agent.StatefulAgent = (*LGCPAgent)(nil)
var _ agent.BufferInfoProvider = (*LGCPAgent)(nil)
var _ agent.TransportChecker = (*LGCPAgent)(nil)

// ---------------------------------------------------------------------------
// LGCPFrameEvent / ParsedHeader: JSON 이벤트 구조체
// ---------------------------------------------------------------------------

// LGCPFrameEvent 는 캡처된 프레임의 JSON 이벤트 구조체이다.
//
// v0.x: 5종 에이전트 schema 통일 — Timestamp 가 RFC3339 문자열에서 epoch
// milliseconds (int64) 로 변경.
type LGCPFrameEvent struct {
	Type        string        `json:"type"`             // "lgcp_frame" 또는 "lgcp_partial_frame"
	TimestampMs int64         `json:"timestamp_ms"`     // epoch ms (이전: timestamp 문자열)
	Seq         int64         `json:"seq"`              // 캡처 시퀀스 번호
	RawHex      string        `json:"raw_hex"`          // 원시 바이트 (hex)
	Length      int           `json:"length"`           // 프레임 길이
	CRCValid    bool          `json:"crc_valid"`        // CRC 검증 결과
	Parsed      *ParsedHeader `json:"parsed,omitempty"` // 파싱된 헤더 (정상 프레임만)
	Error       *string       `json:"error,omitempty"`  // 파싱 에러 메시지
}

// ParsedHeader 는 파싱된 LGCP 프레임 헤더이다.
type ParsedHeader struct {
	DA         string              `json:"da"`                 // 목적지 주소 (hex)
	DALabel    string              `json:"da_label,omitempty"` // 목적지 주소 라벨
	SA         string              `json:"sa"`                 // 소스 주소 (hex)
	SALabel    string              `json:"sa_label,omitempty"` // 소스 주소 라벨
	CMD        string              `json:"cmd"`                // 명령 코드 (hex)
	CMDName    string              `json:"cmd_name,omitempty"` // 명령 타입 이름
	SEQ0       int                 `json:"seq0"`               // 명령 시퀀스 번호
	PLEN       int                 `json:"plen"`               // 페이로드 길이
	PayloadHex string              `json:"payload_hex"`        // 페이로드 (hex)
	SEQ1       int                 `json:"seq1"`               // 프레임 시퀀스 번호
	Decoded    *LGCPDecodedPayload `json:"state,omitempty"`    // 해석된 필드 (v0.x: NASA/Century 와 통일하여 "state" 키)
	Pairs      []LGCPRegPairJSON   `json:"pairs,omitempty"`    // 레지스터-속성 쌍 원본 (프로토콜 분석용)
}

// lgcpCMDNames 는 알려진 LGCP 명령 코드와 이름의 매핑이다.
var lgcpCMDNames = map[string]string{
	"0204": "status",    // 상태 조회/보고
	"0201": "control",   // 제어 명령
	"0604": "keepalive", // Keep-alive
	"021D": "special",   // 특수 명령
}

// defaultLGCPLabel 은 주소에 대한 기본 라벨을 반환한다.
// 설정에서 지정되지 않은 디바이스에 자동 부여된다.
func defaultLGCPLabel(addrHex string) string {
	switch addrHex {
	case "ffffffff":
		return "broadcast"
	case "44550000":
		return "controller"
	default:
		return "indoor-" + addrHex
	}
}

// deviceLabel 은 주소에 대한 라벨을 반환한다.
// 등록된 디바이스의 라벨을 우선 사용하고, 없으면 defaultLGCPLabel 로 폴백한다.
// mu.RLock() 을 잡은 상태에서 호출해야 한다.
func (a *LGCPAgent) deviceLabel(addrHex string) string {
	if dev, ok := a.devices[addrHex]; ok && dev.Label != "" {
		return dev.Label
	}
	return defaultLGCPLabel(addrHex)
}

// ---------------------------------------------------------------------------
// transportReader: LGAPTransport.Receive() → io.Reader 어댑터
// ---------------------------------------------------------------------------

// transportReader 는 LGAPTransport.Receive() 를 io.Reader 로 래핑하는 어댑터이다.
// LGCPFrameParser 는 io.Reader 를 요구하지만, LGAPTransport.Receive() 는
// 별도 시그니처를 가지므로 이 어댑터가 필요하다.
type transportReader struct {
	transport LGAPTransport
}

// Read 는 io.Reader 를 구현한다.
func (r *transportReader) Read(p []byte) (int, error) {
	return r.transport.Receive(p)
}

// ---------------------------------------------------------------------------
// 팩토리 함수
// ---------------------------------------------------------------------------

// NewLGCPAgent 는 LGCP 패시브 캡처 에이전트를 생성한다.
func NewLGCPAgent(config agent.AgentConfig) (agent.Agent, error) {
	lgcpConfig, err := parseLGCPConfig(config.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("lgcp agent: %w", err)
	}

	transport, err := newLGCPTransport(lgcpConfig)
	if err != nil {
		return nil, fmt.Errorf("lgcp agent: %w", err)
	}

	a := &LGCPAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("lgcp")),
		lgcpConfig:    lgcpConfig,
		transport:     transport,
		stopCh:        make(chan struct{}),
		msgCh:         make(chan []byte, lgcpConfig.MsgChannelSize),
		stats:         agent.NewAgentStats(),
		logger:        agent.ResolveLogger(config),
		createdAt:     time.Now(),
		recentFrames:  make([]lgcpFrameRecord, lgcpRecentBufferSize),
		recentNotify:  make(chan struct{}, 1),
		devices:       make(map[string]*LGCPDevice),
		lastStates:    make(map[string]LGCPDeviceState),
	}

	if err := a.Init(config); err != nil {
		return nil, err
	}

	return a, nil
}

// newLGCPTransport 는 LGCPConfig.TransportType 에 따라 적절한 트랜스포트를 생성한다.
func newLGCPTransport(cfg LGCPConfig) (LGAPTransport, error) {
	switch cfg.TransportType {
	case "serial":
		return newLGAPSerialTransport(lgcpSerialConfigFromLGCP(cfg)), nil
	case "tcp-client":
		return newLGAPTCPClientTransport(cfg), nil
	case "tcp-server":
		return newLGAPTCPServerTransport(cfg), nil
	default:
		return nil, ErrLGCPUnknownTransportType
	}
}

// lgcpSerialConfigFromLGCP 는 LGCPConfig 에서 LGAPConfig 호환 값을 생성한다.
// newLGAPSerialTransport 에 전달하기 위한 최소 설정만 포함한다.
func lgcpSerialConfigFromLGCP(cfg LGCPConfig) LGAPConfig {
	return LGAPConfig{
		SerialPort:  cfg.SerialPort,
		BaudRate:    cfg.BaudRate,
		DataBits:    cfg.DataBits,
		StopBits:    cfg.StopBits,
		Parity:      cfg.Parity,
		ReadTimeout: cfg.ReadTimeout,
	}
}

// ---------------------------------------------------------------------------
// agent.Agent 인터페이스 구현
// ---------------------------------------------------------------------------

// Init 은 에이전트를 초기화한다.
func (a *LGCPAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("lgcp init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("lgcp init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("lgcp init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	a.logger.Info("lgcp: 에이전트 초기화 완료",
		"serial_port", a.lgcpConfig.SerialPort,
	)

	return nil
}

// Start 는 트랜스포트를 열고 캡처 루프를 시작한다.
func (a *LGCPAgent) Start(_ context.Context) error {
	if a.CurrentState() == lifecycle.StateRunning && a.transport.Available() {
		return nil // 이미 실행 중이면 no-op
	}

	// 재시작 시 stopCh 재생성 (이전 Stop 에서 close 됨)
	a.stopCh = make(chan struct{})

	// 설정에 정의된 디바이스 주소 등록
	a.registerConfigDevices()

	if err := a.transport.Open(); err != nil {
		// 연결 실패 시 에러 반환 대신 재연결 루프 시작
		a.logger.Warn("lgcp: 트랜스포트 연결 실패, 재연결 대기", "error", err)
		go a.reconnectLoop()
	} else {
		// 연결 성공 시 캡처 루프 시작
		go a.captureLoop()
	}

	// 주기적 상태 보고 타이머
	if a.lgcpConfig.NotifyInterval > 0 {
		go a.notifyLoop()
	}

	// 통신 없음 오프라인 감시
	go a.offlineWatchLoop()

	a.stats.SetStartedAt(time.Now())
	a.logger.Info("lgcp: 에이전트 시작 완료")
	return nil
}

// Stop 은 에이전트를 정지한다.
func (a *LGCPAgent) Stop(_ context.Context) error {
	if a.CurrentState() == lifecycle.StateStopped {
		return nil
	}
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("lgcp stop: %w", err)
	}

	// notifyTicker 정지
	a.mu.Lock()
	if a.notifyTicker != nil {
		a.notifyTicker.Stop()
	}
	a.mu.Unlock()

	// goroutine 들에게 종료 시그널
	close(a.stopCh)

	// 트랜스포트 닫기
	if err := a.transport.Close(); err != nil {
		a.logger.Warn("lgcp: transport close error", "error", err)
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
		return fmt.Errorf("lgcp stop: %w", err)
	}

	return nil
}

// Pause 는 Running -> Paused 로 전환한다.
func (a *LGCPAgent) Pause(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("lgcp pause: %w", err)
	}
	a.mu.Lock()
	a.paused = true
	a.mu.Unlock()
	return nil
}

// Resume 은 Paused -> Running 으로 전환한다.
func (a *LGCPAgent) Resume(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("lgcp resume: %w", err)
	}
	a.mu.Lock()
	a.paused = false
	a.mu.Unlock()
	return nil
}

// Health 는 에이전트의 건강 상태를 반환한다.
func (a *LGCPAgent) Health() agent.HealthStatus {
	now := time.Now()
	state := a.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		return agent.HealthStatus{
			Status:    agent.HealthHealthy,
			LastCheck: now,
			Message:   "lgcp agent is running",
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "lgcp agent is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("lgcp agent is in %s state", state),
		}
	}
}

// lgcpProcessRequest 는 Process 메서드의 JSON 요청 구조체이다.
type lgcpProcessRequest struct {
	Command string         `json:"command"`
	Count   int            `json:"count,omitempty"`    // get_recent 에서 사용
	Address string         `json:"address,omitempty"`  // 제어 대상 실내기 주소 (hex)
	Params  map[string]any `json:"params,omitempty"`   // 제어 파라미터
	NodeID  string         `json:"node_id,omitempty"`  // 호출 노드 식별자 (노드별 통계용)
	FlowID  string         `json:"flow_id,omitempty"`  // 호출 플로우 식별자 (노드별 통계용)
	LastSeq int64          `json:"last_seq,omitempty"` // 노드가 마지막으로 수신한 seq (중복 필터링)
}

// Process 는 JSON 명령을 처리한다.
// 지원 명령: get_stats, get_recent, drain, set_power, set_temperature, set_fan_speed, set_mode, set_multiple
func (a *LGCPAgent) Process(data []byte) ([]byte, error) {
	var req lgcpProcessRequest
	if err := json.Unmarshal(data, &req); err != nil {
		a.stats.IncrMessagesErrored()
		return nil, fmt.Errorf("lgcp process: invalid request: %w", err)
	}

	var result []byte
	var err error

	switch req.Command {
	case "get_stats":
		result, err = a.processGetStats()
	case "get_recent":
		count := req.Count
		if count <= 0 {
			count = 10
		}
		result, err = a.processGetRecent(count, req.LastSeq, req.NodeID, req.FlowID)
	case "drain":
		count := req.Count
		if count <= 0 {
			count = lgcpRecentBufferSize
		}
		result, err = a.processDrain(count, req.NodeID, req.FlowID)
	case "set_power":
		result, err = a.processControlCommand(req)
	case "set_temperature":
		result, err = a.processControlCommand(req)
	case "set_fan_speed":
		result, err = a.processControlCommand(req)
	case "set_mode":
		result, err = a.processControlCommand(req)
	case "set_multiple":
		result, err = a.processControlCommand(req)
	default:
		a.stats.IncrMessagesErrored()
		return nil, fmt.Errorf("lgcp: unsupported command %q", req.Command)
	}

	if err != nil {
		a.stats.IncrMessagesErrored()
		return nil, err
	}

	return result, nil
}

// processControlCommand 는 제어 명령을 서모스탯 사칭 모드로 처리한다.
//
// 서모스탯 사칭 모드: 컨트롤러(44550000)가 실내기 상태를 주기적으로 덮어쓰므로,
// 직접 실내기에 명령을 보내도 무효화된다. 대신 서모스탯(실내기)을 사칭하여
// 컨트롤러에 "설정 변경 보고"를 보내면 컨트롤러가 내부 상태를 갱신한다.
//
// 프레임 방향: SA=실내기(사칭), DA=컨트롤러
// 레지스터: Unit→Controller 형식 (0x60+ 레지스터)
// SEQ0: Unit→Controller 방향의 관찰된 시퀀스 사용
func (a *LGCPAgent) processControlCommand(req lgcpProcessRequest) ([]byte, error) {
	a.logger.Info("lgcp: 제어 명령 수신 (서모스탯 사칭 모드)",
		"command", req.Command, "address", req.Address, "params", req.Params)

	if !a.lgcpConfig.ControlEnabled {
		a.logger.Warn("lgcp: 제어 비활성화 상태")
		return nil, ErrLGCPControlNotEnabled
	}

	if req.Address == "" {
		return nil, fmt.Errorf("%w: address", ErrLGCPMissingParam)
	}

	// 주소 파싱
	unitAddr, err := ParseHexAddress(req.Address)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrLGCPInvalidAddress, req.Address)
	}
	ctrlAddr, err := ParseHexAddress(a.lgcpConfig.ControllerAddress)
	if err != nil {
		return nil, fmt.Errorf("lgcp: invalid controller address: %w", err)
	}

	// 서모스탯 레지스터 형식으로 페이로드 생성 (Unit→Controller 방향)
	thermoPayload, err := a.buildThermostatPayloadForCommand(req)
	if err != nil {
		a.logger.Error("lgcp: 서모스탯 페이로드 생성 실패", "error", err)
		return nil, err
	}

	// 프레임 빌드 준비
	if a.frameBuilder == nil {
		a.frameBuilder = NewLGCPFrameBuilder()
	}
	if a.unitSeqManager == nil {
		a.unitSeqManager = NewLGCPSequenceManager()
	}
	// seqManager: SEQ1 할당에 사용 (전역 프레임 카운터)
	if a.seqManager == nil {
		a.seqManager = NewLGCPSequenceManager()
	}

	cmd := [2]byte{0x02, 0x01}

	// 컨트롤러 사칭용 페이로드 생성 (Controller→Unit 방향)
	ctrlPayload := a.buildControllerPayloadForCommand(req)

	currentFanCode, currentModeCode := a.getCurrentFanModeCode(req.Address)
	currentTempC := a.getCurrentTempC(req.Address)
	a.logger.Info("lgcp: 양방향 제어 시작 (타이밍 분리)",
		"unit_seq_synced", a.unitSeqManager.Synced(),
		"ctrl_seq_synced", a.seqManager.Synced(),
		"current_fan_code", currentFanCode,
		"current_mode_code", currentModeCode,
		"current_temp_c", currentTempC,
		"thermo_payload", hex.EncodeToString(thermoPayload),
		"ctrl_payload", hex.EncodeToString(ctrlPayload))

	// 양방향 전송 (타이밍 분리):
	// 전략: 양방향 연속 전송으로 유닛의 안전 인터록 해제 시도.
	// 유닛 팬이 90Hz로 반응 → 우리 ctrl→unit 프레임을 실제 수신 중.
	// 컨트롤러는 power:ON과 outdoor_active:true를 분리 전송하므로,
	// 우리 ctrl→unit(둘 다 포함)이 유닛에 직접 전달되어야 함.
	// 유닛은 안전 인터록으로 수 사이클 유지 후 전환할 수 있으므로 길게 유지.
	const rounds = 30
	const roundInterval = 2 * time.Second    // 빠른 갱신 (폴링 사이클당 ~5회)
	const ctrlDelay = 500 * time.Millisecond // 서모스탯 직후 빠르게 전송

	for i := 0; i < rounds; i++ {
		if i > 0 {
			time.Sleep(roundInterval)
		}

		// (A) 서모스탯 사칭: Unit→Controller (outdoor_active:true 트리거)
		seq0 := a.unitSeqManager.AllocSEQ0(cmd)
		seq1 := a.seqManager.AllocSEQ1()
		tPayload := appendPayloadCRC(cmd, seq0, thermoPayload)
		tFrame := a.frameBuilder.Build(ctrlAddr, unitAddr, cmd, seq0, tPayload, seq1)

		if err := a.sendFrame(tFrame); err != nil {
			a.logger.Error("lgcp: 서모스탯 프레임 전송 실패", "round", i+1, "error", err)
			if i == 0 {
				return nil, fmt.Errorf("%w: %v", ErrLGCPSerialWriteFailed, err)
			}
			break
		}
		a.logger.Info("lgcp: 서모스탯 사칭 전송",
			"round", fmt.Sprintf("%d/%d", i+1, rounds),
			"direction", "unit→ctrl",
			"seq0", fmt.Sprintf("0x%02X", seq0))

		// (B) 컨트롤러 사칭: Controller→Unit (전 라운드)
		// ctrl→unit에 outdoor_active:true + power:ON 동시 포함.
		// 컨트롤러가 분리 전송하는 문제를 우리가 직접 보완.
		if ctrlPayload != nil {
			time.Sleep(ctrlDelay)

			cSeq0 := a.seqManager.AllocSEQ0(cmd)
			cSeq1 := a.seqManager.AllocSEQ1()
			cPayload := appendPayloadCRC(cmd, cSeq0, ctrlPayload)
			cFrame := a.frameBuilder.Build(unitAddr, ctrlAddr, cmd, cSeq0, cPayload, cSeq1)

			if err := a.sendFrame(cFrame); err != nil {
				a.logger.Warn("lgcp: 컨트롤러 사칭 프레임 전송 실패", "round", i+1, "error", err)
			} else {
				a.logger.Info("lgcp: 컨트롤러 사칭 전송",
					"round", fmt.Sprintf("%d/%d", i+1, rounds),
					"direction", "ctrl→unit",
					"seq0", fmt.Sprintf("0x%02X", cSeq0))
			}
		}
	}

	a.logger.Info("lgcp: 양방향 제어 완료", "total_rounds", rounds)

	// 비동기 상태 확인
	verified := a.waitForStateChange(req.Address, a.lgcpConfig.ControlVerifyTimeout)

	resp := map[string]any{
		"status":   "ok",
		"command":  req.Command,
		"address":  req.Address,
		"mode":     "dual_timed",
		"verified": verified,
	}
	if !verified {
		resp["message"] = "thermostat command sent, verification timeout"
	}

	a.stats.IncrInternalMessagesSent()
	return json.Marshal(resp)
}

// buildControllerPayloadForCommand 는 컨트롤러 사칭용 페이로드를 생성한다.
// Controller→Unit 방향의 레지스터 형식 (0x10-0x29 네임스페이스)을 사용한다.
// 에러 시 nil 을 반환하며 (서모스탯 페이로드가 메인이므로 실패해도 계속).
func (a *LGCPAgent) buildControllerPayloadForCommand(req lgcpProcessRequest) []byte {
	params := req.Params
	if params == nil {
		return nil
	}

	switch req.Command {
	case "set_power":
		power, ok := params["power"]
		if !ok {
			return nil
		}
		on, _ := power.(bool)
		// ctrl→unit 페이로드: 0x13 레지스터 제외 (관찰 전용 레지스터, 포함 시 유닛 거부)
		// compCap=9: 실제 컨트롤러는 4를 사용하나, 9가 더 강한 팬 반응을 유발
		if on {
			return []byte{0x10, 0xC1, 0x18, 0x41, 0x18, 0x89, 0x29, 0xC0}
		}
		return []byte{0x10, 0xC0, 0x18, 0x40, 0x18, 0x80, 0x29, 0xC0}

	case "set_temperature":
		temp, ok := params["target_temp"]
		if !ok {
			return nil
		}
		var tempC float64
		switch t := temp.(type) {
		case float64:
			tempC = t
		case int:
			tempC = float64(t)
		}
		p, err := encodeTemperaturePayload(tempC)
		if err != nil {
			return nil
		}
		return p

	case "set_fan_speed":
		fs, ok := params["fan_speed"]
		if !ok {
			return nil
		}
		fanStr, _ := fs.(string)
		fanCode, err := lookupFanSpeedCode(fanStr)
		if err != nil {
			return nil
		}
		_, currentModeCode := a.getCurrentFanModeCode(req.Address)
		return encodeFanModePayload(fanCode, currentModeCode)

	case "set_mode":
		m, ok := params["mode"]
		if !ok {
			return nil
		}
		modeStr, _ := m.(string)
		modeCode, err := lookupModeCode(modeStr)
		if err != nil {
			return nil
		}
		currentFanCode, _ := a.getCurrentFanModeCode(req.Address)
		return encodeFanModePayload(currentFanCode, modeCode)

	case "set_multiple":
		currentFanCode, currentModeCode := a.getCurrentFanModeCode(req.Address)
		p, _ := buildControlPayload(params, currentFanCode, currentModeCode)
		return p
	}
	return nil
}

// buildThermostatPayloadForCommand 는 서모스탯 사칭용 페이로드를 생성한다.
// Unit→Controller 방향의 레지스터 형식 (0x60+ 네임스페이스)을 사용한다.
func (a *LGCPAgent) buildThermostatPayloadForCommand(req lgcpProcessRequest) ([]byte, error) {
	params := req.Params
	if params == nil {
		params = make(map[string]any)
	}

	// 현재 디바이스 상태에서 기본값 조회
	currentFanCode, currentModeCode := a.getCurrentFanModeCode(req.Address)
	currentTempC := a.getCurrentTempC(req.Address)

	switch req.Command {
	case "set_power":
		power, ok := params["power"]
		if !ok {
			return nil, fmt.Errorf("%w: power", ErrLGCPMissingParam)
		}
		on, _ := power.(bool)
		// 서모스탯 형식: 62,41(ON)/62,40(OFF) + 64,50,XY + 64,8V
		return encodeThermostatPowerPayload(on, currentFanCode, currentModeCode, currentTempC), nil

	case "set_temperature":
		temp, ok := params["target_temp"]
		if !ok {
			return nil, fmt.Errorf("%w: target_temp", ErrLGCPMissingParam)
		}
		var tempC float64
		switch t := temp.(type) {
		case float64:
			tempC = t
		case int:
			tempC = float64(t)
		}
		// 서모스탯 형식: 64,8V (온도만)
		return encodeThermostatTempPayload(tempC)

	case "set_fan_speed":
		fs, ok := params["fan_speed"]
		if !ok {
			return nil, fmt.Errorf("%w: fan_speed", ErrLGCPMissingParam)
		}
		fanStr, _ := fs.(string)
		fanCode, err := lookupFanSpeedCode(fanStr)
		if err != nil {
			return nil, err
		}
		// 서모스탯 형식: 64,50,XY
		return encodeThermostatFanModePayload(fanCode, currentModeCode), nil

	case "set_mode":
		m, ok := params["mode"]
		if !ok {
			return nil, fmt.Errorf("%w: mode", ErrLGCPMissingParam)
		}
		modeStr, _ := m.(string)
		modeCode, err := lookupModeCode(modeStr)
		if err != nil {
			return nil, err
		}
		// 서모스탯 형식: 64,50,XY
		return encodeThermostatFanModePayload(currentFanCode, modeCode), nil

	case "set_multiple":
		return buildThermostatPayload(params, currentFanCode, currentModeCode, currentTempC)

	default:
		return nil, fmt.Errorf("lgcp: unknown control command %q", req.Command)
	}
}

// getCurrentFanModeCode 는 디바이스의 현재 풍량/모드 코드를 반환한다.
// 대상 디바이스에 mode/fan 정보가 없으면 동일 컨트롤러의 다른 디바이스에서 조회한다.
// 모든 디바이스에 정보가 없으면 기본값을 반환한다.
func (a *LGCPAgent) getCurrentFanModeCode(address string) (fanCode, modeCode int) {
	fanCode = lgcpDefaultFanCode
	modeCode = lgcpDefaultModeCode

	a.mu.RLock()
	defer a.mu.RUnlock()

	fanFound, modeFound := false, false

	// 1차: 대상 디바이스에서 조회
	if dev, ok := a.devices[address]; ok && dev.State != nil {
		if dev.State.FanSpeed != nil {
			if code, ok := lgcpFanSpeedCodes[*dev.State.FanSpeed]; ok {
				fanCode = code
				fanFound = true
			}
		}
		if dev.State.Mode != nil {
			if code, ok := lgcpModeCodes[*dev.State.Mode]; ok {
				modeCode = code
				modeFound = true
			}
		}
	}

	// 2차: 미확인 항목이 있으면 동일 컨트롤러의 다른 디바이스에서 폴백
	if !fanFound || !modeFound {
		for addr, dev := range a.devices {
			if addr == address || dev.State == nil {
				continue
			}
			if !fanFound && dev.State.FanSpeed != nil {
				if code, ok := lgcpFanSpeedCodes[*dev.State.FanSpeed]; ok {
					fanCode = code
					fanFound = true
				}
			}
			if !modeFound && dev.State.Mode != nil {
				if code, ok := lgcpModeCodes[*dev.State.Mode]; ok {
					modeCode = code
					modeFound = true
				}
			}
			if fanFound && modeFound {
				break
			}
		}
	}

	return
}

// getCurrentTempC 는 디바이스의 현재 설정 온도를 반환한다.
// 상태 미확인 시 기본값 24.0 을 반환한다.
func (a *LGCPAgent) getCurrentTempC(address string) float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	dev, ok := a.devices[address]
	if !ok || dev.State == nil || dev.State.SetTempC == nil {
		return 24.0
	}
	return *dev.State.SetTempC
}

// sendFrame 은 제어 프레임을 시리얼 포트로 전송하고 에코 필터용 정보를 기록한다.
// RS-485 버스 충돌을 방지하기 위해 전송 전에 버스가 조용해질 때까지 대기한다.
func (a *LGCPAgent) sendFrame(frame []byte) error {
	// 버스 quiet 대기: 마지막 수신 후 최소 50ms 경과 대기
	a.waitForBusQuiet(50 * time.Millisecond)

	a.writeMu.Lock()
	defer a.writeMu.Unlock()

	a.lastSentFrame = make([]byte, len(frame))
	copy(a.lastSentFrame, frame)
	a.lastSentTime = time.Now()

	_, err := a.transport.Write(frame)
	if err == nil {
		a.stats.IncrExternalMessagesSent()
		a.stats.AddBytesWritten(int64(len(frame)))
		a.stats.UpdateLastActivity()
	}
	return err
}

// waitForBusQuiet 는 RS-485 버스에서 마지막 프레임 수신 후 minQuiet 이상
// 경과할 때까지 대기한다. 최대 2초 대기 후 타임아웃한다.
func (a *LGCPAgent) waitForBusQuiet(minQuiet time.Duration) {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		lastNano := a.lastRecvTime.Load()
		if lastNano == 0 {
			return // 아직 수신 없음 — 바로 전송
		}
		elapsed := time.Since(time.Unix(0, lastNano))
		if elapsed >= minQuiet {
			return
		}
		time.Sleep(minQuiet - elapsed)
	}
	a.logger.Warn("lgcp: 버스 quiet 대기 타임아웃 (2초), 강제 전송")
}

// isEcho 는 수신된 프레임이 자신이 전송한 에코인지 판별한다.
// 에코 윈도우 내에 전송 프레임과 동일한 바이트인 경우 에코로 판정한다.
func (a *LGCPAgent) isEcho(frameRaw []byte) bool {
	a.writeMu.Lock()
	sent := a.lastSentFrame
	sentTime := a.lastSentTime
	a.writeMu.Unlock()

	if sent == nil {
		return false
	}

	// 에코 윈도우: 프레임 크기 기반 (보레이트 9600bps 기준, 여유 계수 3x)
	// 바이트당 ~1ms @9600bps, 프레임 크기 * 3ms
	echoWindow := time.Duration(len(sent)*3) * time.Millisecond
	if echoWindow < 50*time.Millisecond {
		echoWindow = 50 * time.Millisecond // 최소 50ms
	}

	if time.Since(sentTime) > echoWindow {
		return false
	}

	if len(frameRaw) != len(sent) {
		return false
	}

	for i := range frameRaw {
		if frameRaw[i] != sent[i] {
			return false
		}
	}

	return true
}

// waitForStateChange 는 제어 전송 후 상태 변경을 대기한다.
// 캡처 루프가 업데이트하는 디바이스 상태를 polling 으로 확인한다.
func (a *LGCPAgent) waitForStateChange(address string, timeout time.Duration) bool {
	if timeout <= 0 {
		return false
	}

	// 전송 직전 상태 스냅샷
	a.mu.RLock()
	prevState, hasPrev := a.lastStates[address]
	a.mu.RUnlock()

	deadline := time.After(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return false
		case <-ticker.C:
			a.mu.RLock()
			currentState, hasCurrent := a.lastStates[address]
			a.mu.RUnlock()

			if !hasPrev && hasCurrent {
				return true // 새 상태 등장
			}
			if hasPrev && hasCurrent && currentState != prevState {
				return true // 상태 변경 감지
			}
		}
	}
}

// processGetStats 는 캡처 통계를 반환한다.
func (a *LGCPAgent) processGetStats() ([]byte, error) {
	stats := map[string]any{
		"frames_captured":     a.framesCaptured.Load(),
		"frames_valid":        a.framesValid.Load(),
		"frames_invalid":      a.framesInvalid.Load(),
		"frames_dropped":      a.framesDropped.Load(),
		"bytes_received":      a.bytesReceived.Load(),
		"transport_connected": a.transport.Available(),
	}

	a.reconnectMu.Lock()
	stats["reconnecting"] = a.isReconnecting
	stats["reconnect_attempts"] = a.reconnectAttempts
	a.reconnectMu.Unlock()

	return json.Marshal(stats)
}

// processGetRecent 는 최근 프레임 중 lastSeq 이후의 새 프레임만 반환한다.
// lastSeq가 0이면 최근 count개를 반환한다 (하위 호환).
func (a *LGCPAgent) processGetRecent(count int, lastSeq int64, nodeID, flowID string) ([]byte, error) {
	a.recentMu.RLock()
	defer a.recentMu.RUnlock()

	// 링 버퍼에서 유효한 항목 수 계산
	total := lgcpRecentBufferSize
	if !a.recentFull {
		total = a.recentIdx
	}

	if count > total {
		count = total
	}

	// 최신 항목부터 역순으로 추출, lastSeq 필터링
	result := make([]json.RawMessage, 0, count)
	for i := 0; i < count; i++ {
		idx := (a.recentIdx - 1 - i + lgcpRecentBufferSize) % lgcpRecentBufferSize
		rec := a.recentFrames[idx]
		if lastSeq > 0 && rec.Seq <= lastSeq {
			break // seq는 단조 증가하므로 이 이후는 전부 이전 프레임
		}
		result = append(result, rec.Event)
	}

	// 실제 신규 프레임 수만 카운트 (lastSeq 필터 통과분)
	if n := int64(len(result)); n > 0 {
		a.stats.AddInternalMessagesSent(n)
		if nodeID != "" {
			a.stats.IncrNodeRefSent(nodeID, flowID)
		}
	}

	return json.Marshal(map[string]any{
		"count":  len(result),
		"frames": result,
	})
}

// processDrain 은 링 버퍼에서 최대 count 개 프레임을 반환하고 버퍼를 리셋한다.
// get_recent와 달리 읽은 프레임을 소비(consume)하여 에이전트에 남지 않는다.
// 반환 순서는 최신→오래된 순서이다 (get_recent와 동일).
func (a *LGCPAgent) processDrain(count int, nodeID, flowID string) ([]byte, error) {
	a.recentMu.Lock()
	defer a.recentMu.Unlock()

	// 링 버퍼에서 유효한 항목 수 계산
	total := lgcpRecentBufferSize
	if !a.recentFull {
		total = a.recentIdx
	}

	if count > total {
		count = total
	}

	// 최신 항목부터 역순으로 추출
	result := make([]json.RawMessage, 0, count)
	for i := 0; i < count; i++ {
		idx := (a.recentIdx - 1 - i + lgcpRecentBufferSize) % lgcpRecentBufferSize
		result = append(result, a.recentFrames[idx].Event)
	}

	// 버퍼 리셋
	a.recentIdx = 0
	a.recentFull = false

	// 프레임이 있을 때만 내부 송신 카운트 + 노드별 통계
	if n := int64(len(result)); n > 0 {
		a.stats.AddInternalMessagesSent(n)
		if nodeID != "" {
			for range result {
				a.stats.IncrNodeRefSent(nodeID, flowID)
			}
		}
	}

	return json.Marshal(map[string]any{
		"count":  len(result),
		"frames": result,
	})
}

// Configure 는 에이전트 설정을 업데이트한다.
func (a *LGCPAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("lgcp configure: %w", err)
	}

	if len(config.Transport.Options) > 0 {
		lgcpCfg, err := parseLGCPConfig(config.Transport.Options)
		if err != nil {
			return fmt.Errorf("lgcp configure: re-parse config: %w", err)
		}
		a.mu.Lock()
		a.lgcpConfig = lgcpCfg
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
func (a *LGCPAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *LGCPAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *LGCPAgent) Type() string {
	return "lgcp"
}

// Info 는 에이전트 정보의 스냅샷을 반환한다.
func (a *LGCPAgent) Info() agent.AgentInfo {
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
		Type:      "lgcp",
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
func (a *LGCPAgent) Stats() agent.StatsSnapshot {
	s := a.stats.Snapshot()
	s.MsgBufferPending, s.MsgBufferCapacity = a.BufferInfo()
	s.Extra = map[string]any{
		"frames_captured":     a.framesCaptured.Load(),
		"frames_valid":        a.framesValid.Load(),
		"frames_invalid":      a.framesInvalid.Load(),
		"frames_dropped":      a.framesDropped.Load(),
		"bytes_received":      a.bytesReceived.Load(),
		"transport_connected": a.transport.Available(),
	}
	return s
}

// ---------------------------------------------------------------------------
// agent.MessageReceiver 인터페이스 구현
// ---------------------------------------------------------------------------

// ReceiveMessage 는 msgCh 에서 메시지를 수신한다.
func (a *LGCPAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	a.bridgeActive.Store(true)
	select {
	case data := <-a.msgCh:
		a.stats.IncrInternalMessagesSent()
		a.logger.Debug("lgcp: ReceiveMessage 전달",
			"bytes", len(data),
		)
		return data, nil
	case <-a.stopCh:
		return nil, fmt.Errorf("lgcp: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ---------------------------------------------------------------------------
// agent.StatefulAgent 인터페이스 구현
// ---------------------------------------------------------------------------

// State 는 캡처 상태를 반환한다.
func (a *LGCPAgent) State() map[string]any {
	result := map[string]any{
		"frames_captured": a.framesCaptured.Load(),
		"frames_valid":    a.framesValid.Load(),
		"frames_invalid":  a.framesInvalid.Load(),
		"frames_dropped":  a.framesDropped.Load(),
		"bytes_received":  a.bytesReceived.Load(),
	}

	result["transport_connected"] = a.transport.Available()
	a.reconnectMu.Lock()
	result["reconnecting"] = a.isReconnecting
	result["reconnect_attempts"] = a.reconnectAttempts
	a.reconnectMu.Unlock()

	return result
}

// ---------------------------------------------------------------------------
// agent.BufferInfoProvider 인터페이스 구현
// ---------------------------------------------------------------------------

// BufferInfo 는 메시지 버퍼의 현재 사용량과 용량을 반환한다.
func (a *LGCPAgent) BufferInfo() (int, int) {
	return len(a.msgCh), cap(a.msgCh)
}

// FrameNotifyCh 는 새 프레임 도착 시 신호를 보내는 채널을 반환한다.
// 폴링 노드가 타이머 대기 없이 즉시 새 프레임을 수신할 수 있도록 한다.
func (a *LGCPAgent) FrameNotifyCh() <-chan struct{} {
	return a.recentNotify
}

// ---------------------------------------------------------------------------
// device.DeviceProvider 인터페이스 구현
// ---------------------------------------------------------------------------

// DeviceProvider 는 이 에이전트의 디바이스를 device.DeviceProvider 로 노출한다.
func (a *LGCPAgent) DeviceProvider() device.DeviceProvider {
	return NewLGCPDeviceProvider(a)
}

// ---------------------------------------------------------------------------
// agent.TransportChecker 인터페이스 구현
// ---------------------------------------------------------------------------

// TransportConnected 는 시리얼 트랜스포트의 실제 연결 상태를 반환한다.
func (a *LGCPAgent) TransportConnected() bool {
	return a.transport.Available()
}

// ---------------------------------------------------------------------------
// 캡처 루프 (패시브 리스닝)
// ---------------------------------------------------------------------------

// captureLoop 는 시리얼 버스에서 LGCP 프레임을 패시브하게 캡처한다.
// io.Reader 어댑터를 통해 LGCPFrameParser 로 프레임을 읽고,
// JSON 이벤트로 변환하여 msgCh 로 전송한다.
func (a *LGCPAgent) captureLoop() {
	reader := &transportReader{transport: a.transport}
	parser := NewLGCPFrameParser(reader, a.lgcpConfig.VerifyCRC)

	a.logger.Info("lgcp: 캡처 루프 시작",
		"verify_crc", a.lgcpConfig.VerifyCRC,
	)

	for {
		select {
		case <-a.stopCh:
			a.logger.Info("lgcp: 캡처 루프 종료 (stopCh)")
			return
		default:
		}

		// Paused 상태이면 잠시 대기
		a.mu.RLock()
		paused := a.paused
		a.mu.RUnlock()
		if paused {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// 프레임 읽기
		frame, err := parser.ReadFrame()
		if err != nil {
			if err == io.EOF {
				a.logger.Info("lgcp: 트랜스포트 EOF, 재연결 시도")
				go a.reconnectLoop()
				return
			}
			// 연결 에러 감지
			if isLGAPConnectionError(err) {
				a.logger.Warn("lgcp: 트랜스포트 연결 에러, 재연결 시도", "error", err)
				go a.reconnectLoop()
				return
			}
			// 읽기 타임아웃 등은 무시하고 계속 리스닝
			continue
		}

		if frame == nil {
			continue
		}

		// RS-485 버스 활동 추적 (제어 프레임 전송 타이밍용)
		a.lastRecvTime.Store(time.Now().UnixNano())

		// RS-485 에코 필터링: 자신이 전송한 프레임이면 건너뜀
		if a.isEcho(frame.Raw) {
			a.logger.Debug("lgcp: 에코 프레임 무시", "len", len(frame.Raw))
			continue
		}

		// 통계 업데이트
		a.framesCaptured.Add(1)
		a.bytesReceived.Add(int64(len(frame.Raw)))
		a.stats.IncrExternalMessagesReceived()
		a.stats.AddBytesRead(int64(len(frame.Raw)))
		a.stats.UpdateLastActivity()
		a.stats.RecordFirstMessage()

		// 프레임 이벤트 생성 및 전송
		a.handleCapturedFrame(frame)
	}
}

// handleCapturedFrame 은 캡처된 프레임을 JSON 이벤트로 변환하여 msgCh 로 전송한다.
func (a *LGCPAgent) handleCapturedFrame(frame *LGCPFrame) {
	seq := a.framesCaptured.Load()

	evt := LGCPFrameEvent{
		TimestampMs: frame.Timestamp.UnixMilli(),
		Seq:         seq,
		RawHex:      hex.EncodeToString(frame.Raw),
		Length:      frame.Length,
		CRCValid:    frame.CRCValid,
	}

	if frame.ParseErr != nil {
		// 파싱 에러가 있는 프레임 (부분 프레임)
		evt.Type = "lgcp_partial_frame"
		errMsg := frame.ParseErr.Error()
		evt.Error = &errMsg
		a.framesInvalid.Add(1)
	} else {
		// 정상 프레임
		evt.Type = "lgcp_frame"
		daHex := hex.EncodeToString(frame.DA)
		saHex := hex.EncodeToString(frame.SA)
		cmdHex := fmt.Sprintf("%02X%02X", frame.CMD[0], frame.CMD[1])
		decoded := DecodePayload(frame.Payload)
		var pairs []LGCPRegPairJSON
		if decoded != nil {
			pairs = decoded.Pairs
			decoded.Pairs = nil // decoded에서 pairs 제거 (parsed.pairs로 이동)
		}
		a.mu.RLock()
		daLabel := a.deviceLabel(daHex)
		saLabel := a.deviceLabel(saHex)
		a.mu.RUnlock()
		evt.Parsed = &ParsedHeader{
			DA:         daHex,
			DALabel:    daLabel,
			SA:         saHex,
			SALabel:    saLabel,
			CMD:        cmdHex,
			CMDName:    lgcpCMDNames[cmdHex],
			SEQ0:       int(frame.SEQ0),
			PLEN:       len(frame.Payload),
			PayloadHex: hex.EncodeToString(frame.Payload),
			SEQ1:       int(frame.SEQ1),
			Decoded:    decoded,
			Pairs:      pairs,
		}

		if frame.CRCValid {
			a.framesValid.Add(1)

			// 컨트롤러 발신 프레임의 SEQ0/SEQ1 추적 (제어 시퀀스 동기화)
			if a.lgcpConfig.ControlEnabled && saHex == a.lgcpConfig.ControllerAddress {
				if a.seqManager == nil {
					a.seqManager = NewLGCPSequenceManager()
				}
				a.seqManager.ObserveFrame(frame.CMD, frame.SEQ0, frame.SEQ1)
			}

			// Unit→Controller 방향 프레임의 SEQ0/SEQ1 추적 (서모스탯 사칭용)
			if a.lgcpConfig.ControlEnabled && daHex == a.lgcpConfig.ControllerAddress && saHex != a.lgcpConfig.ControllerAddress {
				if a.unitSeqManager == nil {
					a.unitSeqManager = NewLGCPSequenceManager()
				}
				a.unitSeqManager.ObserveFrame(frame.CMD, frame.SEQ0, frame.SEQ1)
			}

			if a.lgcpConfig.AutoDiscovery {
				// CRC 유효 프레임의 SA/DA 디바이스를 발견/등록
				a.ensureDevices(saHex, daHex, frame.Timestamp)

				// 디코딩 결과가 있으면 상태도 갱신
				if decoded != nil {
					a.updateDeviceState(saHex, daHex, cmdHex, decoded, frame.Timestamp)
				}
			}
		} else {
			a.framesInvalid.Add(1)
		}
	}

	// JSON 직렬화
	b, err := json.Marshal(evt)
	if err != nil {
		a.logger.Warn("lgcp: event marshal failed", "error", err)
		return
	}

	// 최근 프레임 링 버퍼에 저장
	a.pushRecentFrame(b, frame.Timestamp, seq)

	// msgCh 로 전송 (bridge 소비자가 있을 때만)
	if a.bridgeActive.Load() {
		a.sendFrameEvent(b)
	}
}

// pushRecentFrame 은 프레임 이벤트를 링 버퍼에 추가한다.
func (a *LGCPAgent) pushRecentFrame(eventJSON []byte, ts time.Time, seq int64) {
	a.recentMu.Lock()
	defer a.recentMu.Unlock()

	a.recentFrames[a.recentIdx] = lgcpFrameRecord{
		Event:     json.RawMessage(eventJSON),
		Timestamp: ts,
		Seq:       seq,
	}
	a.recentIdx = (a.recentIdx + 1) % lgcpRecentBufferSize
	if a.recentIdx == 0 {
		a.recentFull = true
	}

	// 폴링 노드에 새 프레임 도착 알림 (non-blocking)
	select {
	case a.recentNotify <- struct{}{}:
	default:
	}
}

// sendFrameEvent 는 프레임 이벤트를 msgCh 로 전송한다.
// 버퍼가 가득 차면 가장 오래된 메시지를 드롭하고 최신 메시지를 삽입한다 (ring buffer 전략).
// 이를 통해 항상 최신 데이터가 보존된다.
func (a *LGCPAgent) sendFrameEvent(data []byte) {
	select {
	case a.msgCh <- data:
		return
	default:
	}

	// 버퍼 풀 — 가장 오래된 메시지를 드레인하여 공간 확보
	select {
	case <-a.msgCh:
	default:
	}

	dropped := a.framesDropped.Add(1)
	a.stats.IncrDroppedMessages()
	now := time.Now().UnixNano()
	last := a.lastDropLog.Load()
	if now-last > 10_000_000_000 && a.lastDropLog.CompareAndSwap(last, now) {
		a.logger.Warn("lgcp: msgCh full, dropping oldest frame",
			"total_dropped", dropped,
			"ch_cap", cap(a.msgCh),
		)
	}

	// 새 메시지 삽입 시도 (드레인 후에도 경합으로 실패할 수 있으므로 non-blocking)
	select {
	case a.msgCh <- data:
	default:
	}
}

// ---------------------------------------------------------------------------
// 재연결 루프 (지수 백오프)
// ---------------------------------------------------------------------------

// reconnectLoop 는 트랜스포트 연결이 끊어졌을 때 지수 백오프로 재연결을 시도한다.
func (a *LGCPAgent) reconnectLoop() {
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
		a.reconnectAttempts = 0
		a.reconnectMu.Unlock()
	}()

	// 통신 끊김 → 모든 디바이스 오프라인 전환
	a.setAllDevicesOffline()

	// 재연결 시작 이벤트
	a.sendStatusEvent("transport_reconnecting", map[string]any{
		"timestamp_ms": time.Now().UnixMilli(),
	})

	baseInterval := a.lgcpConfig.ReconnectInterval
	maxBackoff := a.lgcpConfig.MaxReconnectBackoff
	attempt := 0
	disconnectedAt := time.Now()

	for {
		select {
		case <-a.stopCh:
			return
		default:
		}

		// 이전 연결 정리 후 재연결 시도
		_ = a.transport.Close()
		err := a.transport.Open()

		if err == nil {
			// 재연결 성공
			a.logger.Info("lgcp: 트랜스포트 재연결 성공",
				"attempts", attempt+1,
				"downtime", time.Since(disconnectedAt).Round(time.Second).String(),
			)
			a.sendStatusEvent("transport_reconnected", map[string]any{
				"attempt_count":    attempt + 1,
				"downtime_seconds": int(time.Since(disconnectedAt).Seconds()),
				"timestamp_ms":     time.Now().UnixMilli(),
			})

			// 캡처 루프 재시작
			go a.captureLoop()
			return
		}

		// 재연결 실패
		a.reconnectMu.Lock()
		a.reconnectAttempts = attempt + 1
		a.reconnectMu.Unlock()

		if attempt == 0 {
			a.logger.Warn("lgcp: 트랜스포트 재연결 시도 중", "error", err)
		} else {
			a.logger.Debug("lgcp: 트랜스포트 재연결 시도", "attempt", attempt+1, "error", err)
		}

		// 지수 백오프 계산
		backoff := baseInterval
		for i := 0; i < attempt; i++ {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
				break
			}
		}

		attempt++

		// 백오프 대기 (stopCh 로 취소 가능)
		timer := time.NewTimer(backoff)
		select {
		case <-a.stopCh:
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// ---------------------------------------------------------------------------
// 상태 이벤트 전송 헬퍼
// ---------------------------------------------------------------------------

// sendStatusEvent 는 상태 이벤트를 msgCh 로 전송한다.
// bridge 소비자가 없으면 전송을 건너뛴다.
func (a *LGCPAgent) sendStatusEvent(eventType string, data map[string]any) {
	if !a.bridgeActive.Load() {
		return
	}
	evt := map[string]any{"type": eventType}
	for k, v := range data {
		evt[k] = v
	}
	b, err := json.Marshal(evt)
	if err != nil {
		a.logger.Warn("lgcp: status event marshal failed", "error", err)
		return
	}
	// ring buffer 전략: 버퍼 풀이면 가장 오래된 메시지를 드롭
	select {
	case a.msgCh <- b:
		a.logger.Debug("lgcp: 상태 이벤트 전송 성공", "type", eventType)
		return
	default:
	}

	select {
	case <-a.msgCh:
	default:
	}
	a.logger.Warn("lgcp: msgCh full, dropping oldest for status event", "type", eventType)

	select {
	case a.msgCh <- b:
	default:
	}
}

// ---------------------------------------------------------------------------
// 디바이스 관리
// ---------------------------------------------------------------------------

// registerConfigDevices 는 설정에 정의된 디바이스를 사전 등록한다.
func (a *LGCPAgent) registerConfigDevices() {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	for _, entry := range a.lgcpConfig.Devices {
		a.registerOneConfigDevice(entry.Address, entry.Name, now)
	}
}

// RegisterPinnedDevices 는 메타데이터에서 고정 설치로 표시된 디바이스를 등록한다.
// agent.Start() 이후에 호출된다.
func (a *LGCPAgent) RegisterPinnedDevices(entries []agent.DeviceEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	for _, entry := range entries {
		if _, exists := a.devices[entry.Address]; exists {
			continue // 이미 등록됨 (config 디바이스 등)
		}
		label := entry.Name
		if label == "" {
			label = defaultLGCPLabel(entry.Address)
		}
		dev := &LGCPDevice{
			Address:  entry.Address,
			Label:    label,
			Type:     detectLGCPDeviceType(entry.Address),
			Online:   false,
			LastSeen: now,
			Source:   "pinned",
			State:    &LGCPDeviceState{},
		}
		a.devices[entry.Address] = dev
		a.logger.Info("lgcp: 고정 설치 디바이스 등록",
			"address", entry.Address, "label", dev.Label, "type", dev.Type)
	}
}

// registerOneConfigDevice 는 하나의 설정 디바이스를 등록한다.
// mu.Lock() 을 잡은 상태에서 호출해야 한다.
func (a *LGCPAgent) registerOneConfigDevice(addrHex, label string, ts time.Time) {
	if _, exists := a.devices[addrHex]; exists {
		return // 이미 등록됨
	}
	if label == "" {
		label = defaultLGCPLabel(addrHex)
	}
	dev := &LGCPDevice{
		Address:  addrHex,
		Label:    label,
		Type:     detectLGCPDeviceType(addrHex),
		Online:   false, // 아직 프레임을 수신하지 않았으므로 오프라인
		LastSeen: ts,
		Source:   "config",
		State:    &LGCPDeviceState{},
	}
	a.devices[addrHex] = dev
	a.logger.Info("lgcp: 설정 디바이스 등록",
		"address", addrHex, "label", dev.Label, "type", dev.Type)
}

// ensureDevices 는 SA/DA 주소의 디바이스를 발견/등록한다 (lock 포함).
// CRC 유효한 프레임에서 호출되어 디코딩 여부와 무관하게 디바이스를 등록한다.
// 새 디바이스가 발견되면 WebSocket 이벤트를 발행하여 UI 에 알린다.
func (a *LGCPAgent) ensureDevices(saHex, daHex string, ts time.Time) {
	a.mu.Lock()
	prevLen := len(a.devices)
	a.ensureDevice(saHex, ts)
	a.ensureDevice(daHex, ts)
	added := len(a.devices) > prevLen
	fn := a.onDeviceStateChange
	agentName := a.agentConfig.Name
	a.mu.Unlock()

	// 새 디바이스가 추가되었으면 UI 에 알림 (lock 밖에서)
	if added && fn != nil {
		for _, addr := range []string{saHex, daHex} {
			if addr != "ffffffff" {
				globalID := fmt.Sprintf("%s:%s", agentName, addr)
				go fn(agentName, globalID)
			}
		}
	}
}

// ensureDevice 는 주소에 대한 디바이스가 없으면 생성한다.
// mu.Lock() 을 잡은 상태에서 호출해야 한다.
func (a *LGCPAgent) ensureDevice(addrHex string, ts time.Time) *LGCPDevice {
	if addrHex == "ffffffff" {
		return nil // 브로드캐스트 주소는 디바이스로 등록하지 않음
	}
	dev, ok := a.devices[addrHex]
	if !ok {
		dev = &LGCPDevice{
			Address:  addrHex,
			Label:    defaultLGCPLabel(addrHex),
			Type:     detectLGCPDeviceType(addrHex),
			Online:   true,
			LastSeen: ts,
			Source:   "auto",
			State:    &LGCPDeviceState{},
		}
		a.devices[addrHex] = dev
		a.logger.Info("lgcp: 디바이스 발견",
			"address", addrHex, "label", dev.Label, "type", dev.Type)
	}
	return dev
}

// updateDeviceState 는 캡처된 프레임에서 디바이스 상태를 갱신하고 변경을 감지한다.
//
// 제어 명령 (cmd=0201, 실외기→실내기): DA 디바이스에 제어 필드 병합
// 상태 응답 (cmd=0204 등, 실내기→실외기): SA 디바이스에 응답 필드 병합
func (a *LGCPAgent) updateDeviceState(saHex, daHex, cmdHex string, decoded *LGCPDecodedPayload, ts time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// SA 디바이스 갱신 (프레임을 보낸 디바이스)
	if saDev := a.ensureDevice(saHex, ts); saDev != nil {
		saDev.Online = true
		saDev.LastSeen = ts
	}

	// DA 디바이스 갱신 (프레임을 받는 디바이스)
	if daHex != "ffffffff" {
		if daDev := a.ensureDevice(daHex, ts); daDev != nil {
			daDev.LastSeen = ts
		}
	}

	// 상태 병합 대상 결정:
	// - 제어 명령 (0201): DA가 제어 대상 → DA 디바이스에 제어 필드만 병합
	// - 상태 응답 (0204 등): SA가 보고 → SA 디바이스에 전체 병합
	var targetAddr string
	controlOnly := false
	switch cmdHex {
	case "0201":
		targetAddr = daHex
		controlOnly = true
	default:
		targetAddr = saHex
	}

	if targetAddr == "ffffffff" {
		return
	}

	dev := a.devices[targetAddr]
	if dev == nil || dev.State == nil {
		return
	}

	prev := dev.State.snapshot()
	if controlOnly {
		dev.State.mergeControlFields(decoded)
	} else {
		dev.State.mergeDecoded(decoded)
	}
	curr := dev.State.snapshot()

	if stateChanged(prev, curr) {
		// v0.6.7: event_temp_threshold gate — 비온도 필드 변경 없이 온도 센서값
		// (IndoorTempC + PipeTemp1C + PipeTemp2C) 만 변경된 경우 max|Δ| < threshold
		// 면 emit suppress. (v0.6.6: IndoorTempC 만 검사 → Pipe 온도 변경 시
		// 새어나가는 결함 fix). lastStates 갱신 안 함 → 다음 frame 에서 누적 감지.
		if a.lgcpConfig.EventTempThreshold > 0 && !nonTempFieldsChangedLGCP(prev, curr) {
			if maxTempDeltaLGCP(prev, curr) < a.lgcpConfig.EventTempThreshold {
				return
			}
		}

		a.lastStates[targetAddr] = curr

		a.logger.Debug("lgcp: 디바이스 상태 변경",
			"address", targetAddr, "label", dev.Label)

		// v0.7.0: 통합 schema (type="device_state") 로 change emit. 이전엔 emit
		// 없이 콜백만 호출했으나, 다른 4개 HVAC 에이전트와 동일 패턴으로 통일.
		a.emitDeviceStateLocked(dev, "change")

		// 변경 이벤트 발행 (별도 goroutine, lock 밖에서 할 수 없으므로 채널 사용)
		if fn := a.onDeviceStateChange; fn != nil {
			agentName := a.agentConfig.Name
			globalID := fmt.Sprintf("%s:%s", agentName, targetAddr)
			go fn(agentName, globalID)
		}
	}
}

// emitDeviceStateLocked 는 디바이스 상태를 5개 HVAC 에이전트 통합 schema 로
// emit 한다 (v0.7.0). 호출 전제: a.mu 락 보유.
//
// 출력 schema: {type:"device_state", dev_id, trigger, last_seen_ms, state, metadata}
//   - trigger: "change" | "report"
//   - state: LGCPDeviceState.toProperties(dev.Type)
//   - metadata: label / address / device_type
func (a *LGCPAgent) emitDeviceStateLocked(dev *LGCPDevice, trigger string) {
	if dev == nil || dev.State == nil {
		return
	}
	metadata := map[string]any{
		"label":       dev.Label,
		"address":     dev.Address,
		"device_type": dev.Type,
	}
	payload := map[string]any{
		"dev_id":   dev.Address,
		"trigger":  trigger,
		"state":    dev.State.toProperties(dev.Type),
		"metadata": metadata,
	}
	if !dev.LastSeen.IsZero() {
		payload["last_seen_ms"] = dev.LastSeen.UnixMilli()
	}
	a.sendStatusEvent("device_state", payload)
}

// notifyLoop 은 주기적으로 모든 디바이스 상태를 이벤트로 보고한다.
func (a *LGCPAgent) notifyLoop() {
	ticker := time.NewTicker(a.lgcpConfig.NotifyInterval)
	a.mu.Lock()
	a.notifyTicker = ticker
	a.mu.Unlock()

	for {
		select {
		case <-a.stopCh:
			ticker.Stop()
			return
		case <-ticker.C:
			a.sendDeviceNotifications()
		}
	}
}

// sendDeviceNotifications 은 모든 온라인 디바이스의 현재 상태를 trigger="report"
// 로 emit 한다 (v0.7.0: 통합 schema device_state 로 통일).
func (a *LGCPAgent) sendDeviceNotifications() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, dev := range a.devices {
		if dev.Online && dev.State != nil {
			a.emitDeviceStateLocked(dev, "report")
		}
	}
}

// setAllDevicesOffline 은 모든 디바이스를 오프라인으로 전환한다.
// 트랜스포트 연결이 끊어졌을 때 호출된다.
func (a *LGCPAgent) setAllDevicesOffline() {
	a.mu.Lock()
	var offlined []string
	for addr, dev := range a.devices {
		if dev.Online {
			dev.Online = false
			offlined = append(offlined, addr)
		}
	}
	fn := a.onDeviceStateChange
	agentName := a.agentConfig.Name
	a.mu.Unlock()

	if len(offlined) > 0 {
		a.logger.Info("lgcp: 통신 끊김, 디바이스 오프라인 전환", "count", len(offlined))
		// UI 에 오프라인 상태 알림
		if fn != nil {
			for _, addr := range offlined {
				globalID := fmt.Sprintf("%s:%s", agentName, addr)
				go fn(agentName, globalID)
			}
		}
	}
}

// offlineWatchLoop 은 주기적으로 디바이스의 LastSeen 을 확인하여
// OfflineTimeout 이상 통신이 없으면 오프라인으로 전환한다.
func (a *LGCPAgent) offlineWatchLoop() {
	interval := a.lgcpConfig.OfflineTimeout / 2
	if interval < 5*time.Second {
		interval = 5 * time.Second
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

// checkDeviceTimeouts 은 LastSeen 기반으로 개별 디바이스 오프라인 전환을 수행한다.
func (a *LGCPAgent) checkDeviceTimeouts() {
	now := time.Now()
	timeout := a.lgcpConfig.OfflineTimeout

	a.mu.Lock()
	var offlined []string
	for addr, dev := range a.devices {
		if dev.Online && now.Sub(dev.LastSeen) > timeout {
			dev.Online = false
			offlined = append(offlined, addr)
		}
	}
	fn := a.onDeviceStateChange
	agentName := a.agentConfig.Name
	a.mu.Unlock()

	if fn != nil {
		for _, addr := range offlined {
			globalID := fmt.Sprintf("%s:%s", agentName, addr)
			go fn(agentName, globalID)
		}
	}
	for _, addr := range offlined {
		a.logger.Info("lgcp: 통신 타임아웃, 디바이스 오프라인", "address", addr, "timeout", timeout)
	}
}

// SetDeviceStateChangeCallback 은 디바이스 상태 변경 콜백을 등록한다.
func (a *LGCPAgent) SetDeviceStateChangeCallback(fn func(agentName, deviceID string)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onDeviceStateChange = fn
}

// ListDevices 는 현재 관리 중인 모든 디바이스의 스냅샷을 반환한다.
func (a *LGCPAgent) ListDevices() []LGCPDevice {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]LGCPDevice, 0, len(a.devices))
	for _, dev := range a.devices {
		cp := *dev
		if dev.State != nil {
			s := dev.State.snapshot()
			cp.State = &s
		}
		result = append(result, cp)
	}
	return result
}
