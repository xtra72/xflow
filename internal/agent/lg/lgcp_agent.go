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
	recentMu     sync.Mutex
	recentFrames []lgcpFrameRecord
	recentIdx    int
	recentFull   bool

	// 드롭 로그 rate-limit
	lastDropLog atomic.Int64 // UnixNano

	// 디바이스 관리
	devices    map[string]*LGCPDevice      // 주소(hex) → 디바이스
	lastStates map[string]LGCPDeviceState  // 주소(hex) → 이전 상태 (변경 감지용)
	notifyTicker *time.Ticker              // 주기적 상태 보고 타이머

	// 콜백
	onDeviceStateChange func(agentName, deviceID string)
}

// lgcpFrameRecord 는 링 버퍼에 저장되는 프레임 레코드이다.
type lgcpFrameRecord struct {
	Event     json.RawMessage `json:"event"`
	Timestamp time.Time       `json:"timestamp"`
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
type LGCPFrameEvent struct {
	Type      string        `json:"type"`                // "lgcp_frame" 또는 "lgcp_partial_frame"
	Timestamp string        `json:"timestamp"`           // RFC3339
	Seq       int64         `json:"seq"`                 // 캡처 시퀀스 번호
	RawHex    string        `json:"raw_hex"`             // 원시 바이트 (hex)
	Length    int           `json:"length"`              // 프레임 길이
	CRCValid  bool          `json:"crc_valid"`           // CRC 검증 결과
	Parsed    *ParsedHeader `json:"parsed,omitempty"`    // 파싱된 헤더 (정상 프레임만)
	Error     *string       `json:"error,omitempty"`     // 파싱 에러 메시지
}

// ParsedHeader 는 파싱된 LGCP 프레임 헤더이다.
type ParsedHeader struct {
	DA         string              `json:"da"`                    // 목적지 주소 (hex)
	DALabel    string              `json:"da_label,omitempty"`    // 목적지 주소 라벨
	SA         string              `json:"sa"`                    // 소스 주소 (hex)
	SALabel    string              `json:"sa_label,omitempty"`    // 소스 주소 라벨
	CMD        string              `json:"cmd"`                   // 명령 코드 (hex)
	CMDName    string              `json:"cmd_name,omitempty"`    // 명령 타입 이름
	SEQ0       int                 `json:"seq0"`                  // 명령 시퀀스 번호
	PLEN       int                 `json:"plen"`                  // 페이로드 길이
	PayloadHex string              `json:"payload_hex"`           // 페이로드 (hex)
	SEQ1       int                 `json:"seq1"`                  // 프레임 시퀀스 번호
	Decoded    *LGCPDecodedPayload `json:"decoded,omitempty"`     // 해석된 필드 (알려진 레지스터)
	Pairs      []LGCPRegPairJSON   `json:"pairs,omitempty"`       // 레지스터-속성 쌍 원본 (프로토콜 분석용)
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

	transport := newLGAPSerialTransport(lgcpSerialConfigFromLGCP(lgcpConfig))

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
		devices:       make(map[string]*LGCPDevice),
		lastStates:    make(map[string]LGCPDeviceState),
	}

	if err := a.Init(config); err != nil {
		return nil, err
	}

	return a, nil
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
	Command string `json:"command"`
	Count   int    `json:"count,omitempty"` // get_recent 에서 사용
}

// Process 는 JSON 명령을 처리한다.
// LGCP 에이전트는 패시브이므로 캡처 통계 조회 명령만 지원한다.
// 지원 명령: get_stats, get_recent
func (a *LGCPAgent) Process(data []byte) ([]byte, error) {
	var req lgcpProcessRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("lgcp process: invalid request: %w", err)
	}

	switch req.Command {
	case "get_stats":
		return a.processGetStats()
	case "get_recent":
		count := req.Count
		if count <= 0 {
			count = 10
		}
		return a.processGetRecent(count)
	default:
		return nil, fmt.Errorf("lgcp: unsupported command %q (passive agent, read-only)", req.Command)
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

// processGetRecent 는 최근 N 개 프레임을 반환한다.
func (a *LGCPAgent) processGetRecent(count int) ([]byte, error) {
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
	return s
}

// ---------------------------------------------------------------------------
// agent.MessageReceiver 인터페이스 구현
// ---------------------------------------------------------------------------

// ReceiveMessage 는 msgCh 에서 메시지를 수신한다.
func (a *LGCPAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case data := <-a.msgCh:
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

		// 통계 업데이트
		a.framesCaptured.Add(1)
		a.bytesReceived.Add(int64(len(frame.Raw)))
		a.stats.IncrMessagesReceived()

		// 프레임 이벤트 생성 및 전송
		a.handleCapturedFrame(frame)
	}
}

// handleCapturedFrame 은 캡처된 프레임을 JSON 이벤트로 변환하여 msgCh 로 전송한다.
func (a *LGCPAgent) handleCapturedFrame(frame *LGCPFrame) {
	seq := a.framesCaptured.Load()

	evt := LGCPFrameEvent{
		Timestamp: frame.Timestamp.Format(time.RFC3339Nano),
		Seq:       seq,
		RawHex:    hex.EncodeToString(frame.Raw),
		Length:    frame.Length,
		CRCValid:  frame.CRCValid,
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
			// 실내 온도 디버그: raw ext 값과 계산 결과를 로그로 출력
			if decoded.IndoorTempC != nil {
				for _, p := range pairs {
					if p.Reg == "61" && len(p.Attr) >= 2 && p.Attr[0] == '9' {
						a.logger.Info("indoor_temp_debug",
							slog.String("sa", hex.EncodeToString(frame.SA)),
							slog.String("reg", p.Reg),
							slog.String("attr", p.Attr),
							slog.String("ext_hex", p.Ext),
							slog.Float64("temp_c", *decoded.IndoorTempC),
						)
					}
				}
			}
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
	a.pushRecentFrame(b, frame.Timestamp)

	// msgCh 로 전송 (non-blocking, 꽉 차면 드롭)
	a.sendFrameEvent(b)
}

// pushRecentFrame 은 프레임 이벤트를 링 버퍼에 추가한다.
func (a *LGCPAgent) pushRecentFrame(eventJSON []byte, ts time.Time) {
	a.recentMu.Lock()
	defer a.recentMu.Unlock()

	a.recentFrames[a.recentIdx] = lgcpFrameRecord{
		Event:     json.RawMessage(eventJSON),
		Timestamp: ts,
	}
	a.recentIdx = (a.recentIdx + 1) % lgcpRecentBufferSize
	if a.recentIdx == 0 {
		a.recentFull = true
	}
}

// sendFrameEvent 는 프레임 이벤트를 msgCh 로 non-blocking 전송한다.
func (a *LGCPAgent) sendFrameEvent(data []byte) {
	select {
	case a.msgCh <- data:
	default:
		dropped := a.framesDropped.Add(1)
		// 10초에 1번만 로그 출력
		now := time.Now().UnixNano()
		last := a.lastDropLog.Load()
		if now-last > 10_000_000_000 && a.lastDropLog.CompareAndSwap(last, now) {
			a.logger.Warn("lgcp: msgCh full, dropping frames",
				"total_dropped", dropped,
				"ch_cap", cap(a.msgCh),
			)
		}
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
		"timestamp": time.Now().Format(time.RFC3339),
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
				"timestamp":        time.Now().Format(time.RFC3339),
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
func (a *LGCPAgent) sendStatusEvent(eventType string, data map[string]any) {
	evt := map[string]any{"type": eventType}
	for k, v := range data {
		evt[k] = v
	}
	b, err := json.Marshal(evt)
	if err != nil {
		a.logger.Warn("lgcp: status event marshal failed", "error", err)
		return
	}
	select {
	case a.msgCh <- b:
		a.logger.Debug("lgcp: 상태 이벤트 전송 성공", "type", eventType)
	default:
		a.logger.Warn("lgcp: msgCh full, dropping status event", "type", eventType)
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
		a.lastStates[targetAddr] = curr

		a.logger.Debug("lgcp: 디바이스 상태 변경",
			"address", targetAddr, "label", dev.Label)

		// 변경 이벤트 발행 (별도 goroutine, lock 밖에서 할 수 없으므로 채널 사용)
		if fn := a.onDeviceStateChange; fn != nil {
			agentName := a.agentConfig.Name
			globalID := fmt.Sprintf("%s:%s", agentName, targetAddr)
			go fn(agentName, globalID)
		}
	}
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

// sendDeviceNotifications 은 모든 온라인 디바이스의 현재 상태를 이벤트로 전송한다.
func (a *LGCPAgent) sendDeviceNotifications() {
	a.mu.RLock()
	devsCopy := make([]*LGCPDevice, 0, len(a.devices))
	for _, dev := range a.devices {
		if dev.Online && dev.State != nil {
			devsCopy = append(devsCopy, dev)
		}
	}
	a.mu.RUnlock()

	for _, dev := range devsCopy {
		a.sendStatusEvent("device_state_report", map[string]any{
			"address":    dev.Address,
			"label":      dev.Label,
			"type":       dev.Type,
			"properties": dev.State.toProperties(dev.Type),
		})
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
