package lg

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
// LGCNP-01 패시브 패킷 캡처 에이전트
// ---------------------------------------------------------------------------

// LGCNPAgent 는 LG CN-485(LGCNP-01) 프로토콜 패킷을 패시브하게 캡처하는 에이전트이다.
// 시리얼 버스를 리스닝만 하며, 절대로 데이터를 전송하지 않는다.
type LGCNPAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig agent.AgentConfig
	lgcnpConfig LGCNPConfig
	transport   LGAPTransport // 기존 시리얼 트랜스포트 재사용
	stopCh      chan struct{}
	msgCh       chan []byte
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
	oduFramesCaptured atomic.Int64
	iduFramesCaptured atomic.Int64
	framesInvalid     atomic.Int64
	framesDropped     atomic.Int64
	bytesReceived     atomic.Int64
	bytesSkipped      atomic.Int64 // v0.18.14: STX 동기화 복구로 폐기한 byte 누적
	parseErrors       atomic.Int64 // v0.18.14: parser 에러 누적 (EOF / connection / timeout 제외)
	idleTimeouts      atomic.Int64 // v0.18.16: read deadline 만료 누적 (정상 idle 상태)

	// 전체 시퀀스 카운터
	captureSeq atomic.Int64

	// 최근 프레임 링 버퍼 (get_recent 명령용)
	recentMu     sync.RWMutex
	recentFrames []lgcnpFrameRecord
	recentIdx    int
	recentFull   bool
	recentNotify chan struct{} // 새 프레임 도착 알림 (폴링 노드용)

	// Bridge 소비자 활성 여부
	bridgeActive atomic.Bool

	// 드롭 로그 rate-limit
	lastDropLog atomic.Int64

	// 디바이스 관리
	iduDevices  map[string]*LGCNPDevice     // 주소(hex) → IDU 디바이스
	oduState    *LGCNPODUState              // ODU 상태 (단일)
	oduLastSeen time.Time                   // ODU 마지막 수신 시각
	lastStates  map[string]LGCNPDeviceState // 주소(hex) → 이전 상태 (변경 감지용)

	// frame-level dedup — 이전 emit 한 frame event JSON 의 state 영역만 추출/보관해
	// 동일 state 반복 emit 을 차단한다. timestamp_ms / seq / raw_hex 같이 매 frame
	// 마다 바뀌는 메타는 비교에서 제외한다 (extractFrameState helper).
	dedupMu     sync.Mutex
	lastODUEmit []byte         // 최근 emit 한 ODU state JSON (SEQ=02)
	lastIDUEmit map[int][]byte // IDUNum → 최근 emit 한 IDU state JSON
	// v0.7.0: event_temp_threshold gate 의 비교 baseline (마지막 emit 시점 parsed state).
	// dedupMu 로 보호됨. slot 등 메타는 iduDevices 에서 lookup.
	lastIDUParsed map[int]LGCNPIDUParsed
	lastODUParsed *LGCNPODUParsed
	// V2 콜백 (Phase D 1급 — UUID + composite). SPEC-DEVICE-IDENTITY-001 § M3.
	// Phase D (xflowd v1.0) 부터 V1 시그니처는 완전 제거됨.
	onDeviceStateChangeV2 agent.DeviceStateChangeCallbackV2
}

// lgcnpFrameRecord 는 링 버퍼에 저장되는 프레임 레코드이다.
type lgcnpFrameRecord struct {
	Event     json.RawMessage `json:"event"`
	Timestamp time.Time       `json:"timestamp"`
	Seq       int64           `json:"seq"`
}

// lgcnpRecentBufferSize 는 최근 프레임 링 버퍼의 크기이다.
const lgcnpRecentBufferSize = 64

// lgcnpODUUnitID 는 ODU 디바이스의 unit_id 표준 값이다 (v0.18.12).
//
// 이전 (v0.18.6~v0.18.11): "odu"
// 신규 (v0.18.12+): "0" — IDU 와 함께 정수 ID 체계로 통일.
const lgcnpODUUnitID = "0"

// lgcnpIDUUnitID 는 IDU 디바이스의 unit_id 를 IDU 번호 (1~5) 로부터 생성한다 (v0.18.12).
//
// 이전 (v0.18.6~v0.18.11): "idu-N"
// 신규 (v0.18.12+): "N" — ODU 와 함께 정수 ID 체계로 통일.
func lgcnpIDUUnitID(iduNum int) string {
	return fmt.Sprintf("%d", iduNum)
}

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*LGCNPAgent)(nil)
var _ agent.MessageReceiver = (*LGCNPAgent)(nil)
var _ agent.StatefulAgent = (*LGCNPAgent)(nil)
var _ agent.BufferInfoProvider = (*LGCNPAgent)(nil)
var _ agent.TransportChecker = (*LGCNPAgent)(nil)

// ---------------------------------------------------------------------------
// LGCNPFrameEvent: JSON 이벤트 구조체
// ---------------------------------------------------------------------------

// LGCNPFrameMetadata 는 LGCNP frame event 의 metadata 그룹이다 (v0.5.0 통합 schema).
//
// 사용자 요구 "metadata => slot_num, label". IDU 의 slot_num 은 state 에서 분리해 본
// 그룹으로 이동. label 은 device-level 식별자 (idu/odu 명칭).
//
// v0.6.8: device_type 추가. v0.18.3: 값 체계 변경 "indoor"→"HVACR.IDU",
// "outdoor"→"HVACR.ODU" — 카테고리 prefix 도입 (HVACR = HVAC+Refrigerant).
// type 필드가 "device_state" 로 통일됨에 따라 운영자가 IDU/ODU 를 구별할 수 있도록 한다.
type LGCNPFrameMetadata struct {
	Label      string `json:"label,omitempty"`
	SlotNum    int    `json:"slot_num,omitempty"`
	DeviceType string `json:"device_type,omitempty"` // "HVACR.IDU" / "HVACR.ODU"
}

// LGCNPODUFrameEvent 는 캡처된 TYPE-A ODU 프레임의 JSON 이벤트이다.
//
// v0.5.0 Breaking 통합 schema (사용자 요구) —
//   - top-level: type, dev_id, trigger, last_seen_ms, raw_hex(옵션)
//   - state: 5 cycle temp (omitempty 로 미수신 시 자동 제외)
//   - metadata: label
//   - 제거: timestamp_ms, seq, odu_seq, checksum_valid (운영 불필요, RE 시 별도 노드)
type LGCNPODUFrameEvent struct {
	// v0.9.0: Type 필드 제거. metadata.message_type ("device_state.<trigger>") 가
	// 노드 단에서 schema 식별 역할 담당.
	// v0.18.6: 기기별 프로토콜 식별자는 unit_id, 글로벌 고유 UUID 는 device_id.
	DevID      string             `json:"unit_id"`             // 프로토콜 식별자 (예: "odu")
	DeviceID   string             `json:"device_id,omitempty"` // v0.18.6: 글로벌 UUID
	Trigger    string             `json:"trigger"`
	LastSeenMs int64              `json:"last_seen_ms"`
	RawHex     string             `json:"raw_hex,omitempty"` // include_raw_hex=true 시에만 노출
	State      *LGCNPODUParsed    `json:"state,omitempty"`
	Metadata   LGCNPFrameMetadata `json:"metadata,omitempty"`
}

// LGCNPODUParsed 는 TYPE-A 프레임에서 파싱된 데이터이다.
type LGCNPODUParsed struct {
	OutdoorTemp       *float64 `json:"outdoor_temperature,omitempty"`
	CompSuctionTemp   *float64 `json:"compressor_suction_temperature,omitempty"`
	CompDischargeTemp *float64 `json:"compressor_discharge_temperature,omitempty"`
	CondenserTempA    *float64 `json:"condenser_temperature_a,omitempty"`
	CondenserTempB    *float64 `json:"condenser_temperature_b,omitempty"`
}

// LGCNPIDUFrameEvent 는 캡처된 TYPE-B IDU 프레임의 JSON 이벤트이다.
//
// v0.5.0 Breaking 통합 schema (사용자 요구) —
//   - top-level: type, dev_id, trigger, last_seen_ms, raw_hex(옵션)
//   - state: power/mode/fan_speed/target_temp/current_temp/inlet_temp/outlet_temp
//   - metadata: label, slot_num
//   - 제거: timestamp_ms, seq, idu_addr, idu_num, cmd_raw, cmd_cycle, active_state,
//     set_temp_reliable, redundancy_valid (운영 불필요, RE 시 별도 노드)
type LGCNPIDUFrameEvent struct {
	// v0.9.0: Type 필드 제거. metadata.message_type 가 schema 식별 역할 담당.
	// v0.18.6: unit_id (프로토콜) + device_id (UUID) 분리.
	DevID      string             `json:"unit_id"`             // 프로토콜 식별자 (예: "idu-1")
	DeviceID   string             `json:"device_id,omitempty"` // v0.18.6: 글로벌 UUID
	Trigger    string             `json:"trigger"`
	LastSeenMs int64              `json:"last_seen_ms"`
	RawHex     string             `json:"raw_hex,omitempty"` // include_raw_hex=true 시에만 노출
	State      *LGCNPIDUParsed    `json:"state,omitempty"`
	Metadata   LGCNPFrameMetadata `json:"metadata,omitempty"`
}

// LGCNPIDUParsed 는 TYPE-B 프레임에서 파싱된 데이터이다.
//
// v0.5.0: slot_num 을 metadata 로 이동 (state 가 아닌 device 식별자 성격).
// v0.7.5: Mode/FanSpeed 를 hvac 통일 ID (int) 로 변경.
//
//	Mode: 0=off/auto, 1=cool, 2=heat, 3=dry, 4=fan
//	FanSpeed: 0=off, 1=auto, 2=quiet, 3=low, 4=medium, 5=high, 6=turbo
type LGCNPIDUParsed struct {
	Power       bool    `json:"power"`
	TargetTemp  float64 `json:"target_temperature"`  // 이전: set_temp
	CurrentTemp float64 `json:"current_temperature"` // 이전: room_temp
	InletTemp   float64 `json:"inlet_temperature"`
	OutletTemp  float64 `json:"outlet_temperature"`
	FanSpeed    int     `json:"fan_speed"` // v0.7.5: hvac 통일 ID
	Mode        int     `json:"mode"`      // v0.7.5: hvac 통일 ID
}

// ---------------------------------------------------------------------------
// 팩토리 함수
// ---------------------------------------------------------------------------

// NewLGCNPAgent 는 LGCNP-01 패시브 캡처 에이전트를 생성한다.
func NewLGCNPAgent(config agent.AgentConfig) (agent.Agent, error) {
	lgcnpConfig, err := parseLGCNPConfig(config.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("lgcnp agent: %w", err)
	}

	transport, err := newLGCNPTransport(lgcnpConfig)
	if err != nil {
		return nil, fmt.Errorf("lgcnp agent: %w", err)
	}

	a := &LGCNPAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("lgcnp")),
		lgcnpConfig:   lgcnpConfig,
		transport:     transport,
		stopCh:        make(chan struct{}),
		msgCh:         make(chan []byte, lgcnpConfig.MsgChannelSize),
		stats:         agent.NewAgentStats(),
		logger:        agent.ResolveLogger(config),
		createdAt:     time.Now(),
		recentFrames:  make([]lgcnpFrameRecord, lgcnpRecentBufferSize),
		recentNotify:  make(chan struct{}, 1),
		iduDevices:    make(map[string]*LGCNPDevice),
		oduState:      &LGCNPODUState{},
		lastStates:    make(map[string]LGCNPDeviceState),
		lastIDUEmit:   make(map[int][]byte),
		lastIDUParsed: make(map[int]LGCNPIDUParsed),
	}

	if err := a.Init(config); err != nil {
		return nil, err
	}

	return a, nil
}

// newLGCNPTransport 는 LGCNPConfig 에 따라 적절한 트랜스포트를 생성한다.
func newLGCNPTransport(cfg LGCNPConfig) (LGAPTransport, error) {
	// LGCP 트랜스포트 생성 로직 재사용
	lgcpCfg := lgcnpSerialConfigFromLGCNP(cfg)
	switch cfg.TransportType {
	case "serial":
		return newLGAPSerialTransport(lgcpCfg), nil
	case "tcp-client":
		return newLGAPTCPClientTransport(lgcnpToLGCPConfig(cfg)), nil
	case "tcp-server":
		return newLGAPTCPServerTransport(lgcnpToLGCPConfig(cfg)), nil
	default:
		return nil, ErrLGCNPUnknownTransportType
	}
}

// lgcnpSerialConfigFromLGCNP 는 LGCNPConfig 에서 LGAPConfig 호환 값을 생성한다.
func lgcnpSerialConfigFromLGCNP(cfg LGCNPConfig) LGAPConfig {
	return LGAPConfig{
		SerialPort:  cfg.SerialPort,
		BaudRate:    cfg.BaudRate,
		DataBits:    cfg.DataBits,
		StopBits:    cfg.StopBits,
		Parity:      cfg.Parity,
		ReadTimeout: cfg.ReadTimeout,
	}
}

// lgcnpToLGCPConfig 는 LGCNPConfig 를 LGCPConfig 로 변환한다 (TCP 트랜스포트용).
func lgcnpToLGCPConfig(cfg LGCNPConfig) LGCPConfig {
	return LGCPConfig{
		TransportType:     cfg.TransportType,
		TCPHost:           cfg.TCPHost,
		TCPPort:           cfg.TCPPort,
		TCPReadTimeout:    cfg.TCPReadTimeout,
		TCPWriteTimeout:   cfg.TCPWriteTimeout,
		TCPConnectTimeout: cfg.TCPConnectTimeout,
	}
}

// ---------------------------------------------------------------------------
// agent.Agent 인터페이스 구현
// ---------------------------------------------------------------------------

// Init 은 에이전트를 초기화한다.
func (a *LGCNPAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("lgcnp init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("lgcnp init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("lgcnp init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	a.logger.Info("lgcnp: 에이전트 초기화 완료",
		"serial_port", a.lgcnpConfig.SerialPort,
		"baud_rate", a.lgcnpConfig.BaudRate,
	)

	return nil
}

// Start 는 트랜스포트를 열고 캡처 루프를 시작한다.
func (a *LGCNPAgent) Start(_ context.Context) error {
	if a.CurrentState() == lifecycle.StateRunning && a.transport.Available() {
		return nil
	}

	a.stopCh = make(chan struct{})

	// 설정에 정의된 디바이스 주소 등록
	a.registerConfigDevices()

	if err := a.transport.Open(); err != nil {
		a.logger.Warn("lgcnp: 트랜스포트 연결 실패, 재연결 대기", "error", err)
		go a.reconnectLoop()
	} else {
		go a.captureLoop()
	}

	// 주기적 상태 보고 타이머
	if a.lgcnpConfig.NotifyInterval > 0 {
		go a.notifyLoop()
	}

	// 통신 없음 오프라인 감시
	go a.offlineWatchLoop()

	a.stats.SetStartedAt(time.Now())
	a.logger.Info("lgcnp: 에이전트 시작 완료")
	return nil
}

// Stop 은 에이전트를 정지한다.
func (a *LGCNPAgent) Stop(_ context.Context) error {
	if a.CurrentState() == lifecycle.StateStopped {
		return nil
	}
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("lgcnp stop: %w", err)
	}

	close(a.stopCh)

	if err := a.transport.Close(); err != nil {
		a.logger.Warn("lgcnp: transport close error", "error", err)
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
		return fmt.Errorf("lgcnp stop: %w", err)
	}

	return nil
}

// Pause 는 Running -> Paused 로 전환한다.
func (a *LGCNPAgent) Pause(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("lgcnp pause: %w", err)
	}
	a.mu.Lock()
	a.paused = true
	a.mu.Unlock()
	return nil
}

// Resume 은 Paused -> Running 으로 전환한다.
func (a *LGCNPAgent) Resume(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("lgcnp resume: %w", err)
	}
	a.mu.Lock()
	a.paused = false
	a.mu.Unlock()
	return nil
}

// Health 는 에이전트의 건강 상태를 반환한다.
func (a *LGCNPAgent) Health() agent.HealthStatus {
	now := time.Now()
	state := a.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		return agent.HealthStatus{
			Status:    agent.HealthHealthy,
			LastCheck: now,
			Message:   "lgcnp agent is running",
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "lgcnp agent is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("lgcnp agent is in %s state", state),
		}
	}
}

// lgcnpProcessRequest 는 Process 메서드의 JSON 요청 구조체이다.
type lgcnpProcessRequest struct {
	Command string `json:"command"`
	Count   int    `json:"count,omitempty"`
	NodeID  string `json:"node_id,omitempty"`
	FlowID  string `json:"flow_id,omitempty"`
	LastSeq int64  `json:"last_seq,omitempty"`
	DevID   string `json:"dev_id,omitempty"` // v0.7.3: get_state — "odu" or "idu-N"
}

// Process 는 JSON 명령을 처리한다.
// 지원 명령: get_stats, get_recent, drain
func (a *LGCNPAgent) Process(data []byte) ([]byte, error) {
	var req lgcnpProcessRequest
	if err := json.Unmarshal(data, &req); err != nil {
		a.stats.IncrInternalMessagesReceived()
		a.stats.IncrInternalMessagesErrored()
		return nil, fmt.Errorf("lgcnp process: invalid request: %w", err)
	}
	// v0.6.4: 1 per Process call (Century / NASA 와 통일). 이전 patterns 는
	// processGetRecent/processDrain 안에서 AddInternalMessagesSent(N) 으로 frame
	// 단위 카운팅 → 송수신 100x 왜곡. 수정 후 1 Process = 1 received + 1 sent.
	a.stats.IncrInternalMessagesReceived()
	a.stats.IncrInternalMessagesSent()
	if req.NodeID != "" {
		a.stats.IncrNodeRefReceived(req.NodeID, req.FlowID)
		a.stats.IncrNodeRefSent(req.NodeID, req.FlowID)
	}

	var result []byte
	var err error

	switch req.Command {
	case "get_stats":
		result, err = a.processGetStats()
	case "get_recent":
		// v0.7.1: count 의미 통일 (5개 HVAC 노드 공통)
		//   count > 0: 최근 count 개 frame (lastSeq 이후, 비파괴)
		//   count == 0: drain — 전체 frame 반환 후 버퍼 비움 (destructive)
		count := req.Count
		if count == 0 {
			result, err = a.processDrain(lgcnpRecentBufferSize, req.NodeID, req.FlowID)
		} else {
			if count < 0 {
				count = 10
			}
			result, err = a.processGetRecent(count, req.LastSeq, req.NodeID, req.FlowID)
		}
	case "drain":
		// v0.7.1 deprecated: use "get_recent" with count=0.
		count := req.Count
		if count <= 0 {
			count = lgcnpRecentBufferSize
		}
		result, err = a.processDrain(count, req.NodeID, req.FlowID)
	case "get_all":
		// v0.7.2: 5개 HVAC 노드 통일 명령. 모든 IDU + ODU 의 즉시 snapshot 반환.
		result, err = a.processGetAll()
	case "get_state":
		// v0.7.3: 단일 device 조회 (dev_id = "odu" 또는 "idu-N").
		result, err = a.processGetState(&req)
	default:
		a.stats.IncrInternalMessagesErrored()
		return nil, fmt.Errorf("lgcnp: unsupported command %q", req.Command)
	}

	if err != nil {
		a.stats.IncrInternalMessagesErrored()
		return nil, err
	}

	return result, nil
}

// processGetState 는 단일 device 의 즉시 snapshot 을 반환한다 (v0.7.3, v0.18.12).
//
// dev_id 입력 (v0.18.12 부터 정수 형식 + legacy 형식 모두 수용):
//   - "0": ODU (v0.18.12 권장) — legacy "odu" 도 호환
//   - "1" .. "5": IDU (v0.18.12 권장) — legacy "idu-N" 도 호환
//
// 출력 unit_id 는 새 형식 ("0" / "1"~"5") 로 통일.
func (a *LGCNPAgent) processGetState(req *lgcnpProcessRequest) ([]byte, error) {
	if req.DevID == "" {
		return json.Marshal(map[string]any{
			"status": "error",
			"error":  "missing dev_id",
		})
	}
	a.mu.RLock()
	defer a.mu.RUnlock()

	// v0.18.6: unit_id (프로토콜) + device_id (UUID) 분리.
	// v0.18.12: ODU = "0", legacy "odu" 호환.
	if req.DevID == "0" || req.DevID == "odu" {
		if a.oduFramesCaptured.Load() == 0 {
			return json.Marshal(map[string]any{
				"status":  "not_found",
				"unit_id": lgcnpODUUnitID,
			})
		}
		oduSnap := a.oduState.snapshot()
		d := map[string]any{
			"unit_id":     lgcnpODUUnitID,
			"device_id":   agent.ResolveDeviceID(context.Background(), a.ID(), lgcnpODUUnitID),
			"label":       "outdoor",
			"device_type": "HVACR.ODU",
			"online":      true,
			"state":       oduSnap.toProperties(),
		}
		if !a.oduLastSeen.IsZero() {
			d["last_seen_ms"] = a.oduLastSeen.UnixMilli()
		}
		return json.Marshal(map[string]any{
			"status": "ok",
			"device": d,
		})
	}

	// IDU 검색: req.DevID == "N" (v0.18.12) 또는 legacy "idu-N".
	var iduNum int
	if n, err := fmt.Sscanf(req.DevID, "idu-%d", &iduNum); err != nil || n != 1 {
		if _, err := fmt.Sscanf(req.DevID, "%d", &iduNum); err != nil {
			return json.Marshal(map[string]any{
				"status": "error",
				"error":  fmt.Sprintf("invalid dev_id %q (expected '0' for ODU or '1'~'5' for IDU)", req.DevID),
			})
		}
	}
	for _, dev := range a.iduDevices {
		if dev.IDUNum != iduNum {
			continue
		}
		unitID := lgcnpIDUUnitID(dev.IDUNum)
		d := map[string]any{
			"unit_id":     unitID,
			"device_id":   agent.ResolveDeviceID(context.Background(), a.ID(), unitID),
			"label":       dev.Label,
			"device_type": "HVACR.IDU",
			"online":      dev.Online,
		}
		if dev.State != nil {
			d["state"] = dev.State.toProperties()
		}
		if !dev.LastSeen.IsZero() {
			d["last_seen_ms"] = dev.LastSeen.UnixMilli()
		}
		return json.Marshal(map[string]any{
			"status": "ok",
			"device": d,
		})
	}
	return json.Marshal(map[string]any{
		"status":  "not_found",
		"unit_id": req.DevID,
	})
}

// processGetAll 은 모든 IDU + ODU 디바이스의 즉시 snapshot 을 반환한다 (v0.7.2).
// 5개 HVAC 노드 통일 명령 — NASA 의 processGetAllStates 패턴 차용.
func (a *LGCNPAgent) processGetAll() ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	devices := make([]map[string]any, 0, len(a.iduDevices)+1)

	// IDU 디바이스들 (v0.18.6: unit_id + device_id 분리, v0.18.12: unit_id = "1"~"5")
	for _, dev := range a.iduDevices {
		unitID := lgcnpIDUUnitID(dev.IDUNum)
		d := map[string]any{
			"unit_id":     unitID,
			"device_id":   agent.ResolveDeviceID(context.Background(), a.ID(), unitID),
			"label":       dev.Label,
			"device_type": "HVACR.IDU",
			"online":      dev.Online,
		}
		if dev.State != nil {
			d["state"] = dev.State.toProperties()
		}
		if !dev.LastSeen.IsZero() {
			d["last_seen_ms"] = dev.LastSeen.UnixMilli()
		}
		devices = append(devices, d)
	}

	// ODU 디바이스 (1개) — v0.18.12: unit_id = "0"
	if a.oduFramesCaptured.Load() > 0 {
		oduSnap := a.oduState.snapshot()
		d := map[string]any{
			"unit_id":     lgcnpODUUnitID,
			"device_id":   agent.ResolveDeviceID(context.Background(), a.ID(), lgcnpODUUnitID),
			"label":       "outdoor",
			"device_type": "HVACR.ODU",
			"online":      true,
			"state":       oduSnap.toProperties(),
		}
		if !a.oduLastSeen.IsZero() {
			d["last_seen_ms"] = a.oduLastSeen.UnixMilli()
		}
		devices = append(devices, d)
	}

	return json.Marshal(map[string]any{
		"status":  "ok",
		"devices": devices,
	})
}

// processGetStats 는 캡처 통계를 반환한다.
func (a *LGCNPAgent) processGetStats() ([]byte, error) {
	stats := map[string]any{
		"odu_frames_captured": a.oduFramesCaptured.Load(),
		"idu_frames_captured": a.iduFramesCaptured.Load(),
		"frames_invalid":      a.framesInvalid.Load(),
		"frames_dropped":      a.framesDropped.Load(),
		"bytes_skipped":       a.bytesSkipped.Load(), // v0.18.14
		"parse_errors":        a.parseErrors.Load(),  // v0.18.14
		"idle_timeouts":       a.idleTimeouts.Load(), // v0.18.16
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
func (a *LGCNPAgent) processGetRecent(count int, lastSeq int64, nodeID, flowID string) ([]byte, error) {
	a.recentMu.RLock()
	defer a.recentMu.RUnlock()

	total := lgcnpRecentBufferSize
	if !a.recentFull {
		total = a.recentIdx
	}
	if count > total {
		count = total
	}

	result := make([]json.RawMessage, 0, count)
	var maxSeq int64
	for i := 0; i < count; i++ {
		idx := (a.recentIdx - 1 - i + lgcnpRecentBufferSize) % lgcnpRecentBufferSize
		rec := a.recentFrames[idx]
		if lastSeq > 0 && rec.Seq <= lastSeq {
			break
		}
		result = append(result, rec.Event)
		if rec.Seq > maxSeq {
			maxSeq = rec.Seq
		}
	}

	// v0.6.4: 통계 카운팅은 Process() top-level 에서 1회만 수행 (중복 방지).
	// nodeID / flowID 도 Process() 가 사용. 본 함수는 순수 응답 빌딩만 담당.
	_ = nodeID
	_ = flowID

	// v0.6.5: last_seq 를 응답에 포함. 노드가 다음 폴링에 lastSeq 로 전달하여
	// 중복 frame emit 방지. v0.5.0 의 event JSON 슬림화 (seq 필드 제거) 로
	// 노드 측 per-frame seq filter 가 무력화된 버그를 fix — 사용자 보고
	// "lgcnp-status 에서 메시지 수신 안됨" root cause.
	return json.Marshal(map[string]any{
		"count":    len(result),
		"frames":   result,
		"last_seq": maxSeq,
	})
}

// processDrain 은 링 버퍼에서 최대 count 개 프레임을 반환하고 버퍼를 리셋한다.
func (a *LGCNPAgent) processDrain(count int, nodeID, flowID string) ([]byte, error) {
	a.recentMu.Lock()
	defer a.recentMu.Unlock()

	total := lgcnpRecentBufferSize
	if !a.recentFull {
		total = a.recentIdx
	}
	if count > total {
		count = total
	}

	result := make([]json.RawMessage, 0, count)
	var maxSeq int64
	for i := 0; i < count; i++ {
		idx := (a.recentIdx - 1 - i + lgcnpRecentBufferSize) % lgcnpRecentBufferSize
		rec := a.recentFrames[idx]
		result = append(result, rec.Event)
		if rec.Seq > maxSeq {
			maxSeq = rec.Seq
		}
	}

	// 버퍼 리셋
	a.recentIdx = 0
	a.recentFull = false

	// v0.6.4: 통계 카운팅은 Process() top-level 에서 1회만 수행.
	_ = nodeID
	_ = flowID

	// v0.6.5: last_seq 응답 포함. drain 은 destructive 라 lastSeq 추적 불필요하지만
	// 노드 코드가 get_recent/drain 공통 핸들러 사용하므로 형식 통일.
	return json.Marshal(map[string]any{
		"count":    len(result),
		"frames":   result,
		"last_seq": maxSeq,
	})
}

// Configure 는 에이전트 설정을 업데이트한다.
func (a *LGCNPAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("lgcnp configure: %w", err)
	}

	if len(config.Transport.Options) > 0 {
		lgcnpCfg, err := parseLGCNPConfig(config.Transport.Options)
		if err != nil {
			return fmt.Errorf("lgcnp configure: re-parse config: %w", err)
		}
		a.mu.Lock()
		a.lgcnpConfig = lgcnpCfg
		a.agentConfig = config
		a.mu.Unlock()
		// v0.18.20: 설정 변경 즉시 노출 — 사용자가 Web UI 에서 변경 시 적용 여부 확인 용도.
		a.logger.Info("lgcnp: 설정 업데이트됨",
			"dedupe_frames", lgcnpCfg.DedupeFrames,
			"event_temp_threshold", lgcnpCfg.EventTempThreshold,
			"notify_interval", lgcnpCfg.NotifyInterval,
			"verify_redundancy", lgcnpCfg.VerifyRedundancy,
		)
	} else {
		a.mu.Lock()
		a.agentConfig = config
		a.mu.Unlock()
	}

	return nil
}

// ID 는 에이전트 ID 를 반환한다.
func (a *LGCNPAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *LGCNPAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *LGCNPAgent) Type() string {
	return "lgcnp"
}

// Info 는 에이전트 정보의 스냅샷을 반환한다.
func (a *LGCNPAgent) Info() agent.AgentInfo {
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
		Type:      "lgcnp",
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
func (a *LGCNPAgent) Stats() agent.StatsSnapshot {
	s := a.stats.Snapshot()
	s.MsgBufferPending, s.MsgBufferCapacity = a.BufferInfo()
	s.Extra = map[string]any{
		"odu_frames_captured": a.oduFramesCaptured.Load(),
		"idu_frames_captured": a.iduFramesCaptured.Load(),
		"frames_dropped":      a.framesDropped.Load(),
		"bytes_skipped":       a.bytesSkipped.Load(), // v0.18.14
		"parse_errors":        a.parseErrors.Load(),  // v0.18.14
		"idle_timeouts":       a.idleTimeouts.Load(), // v0.18.16
		"bytes_received":      a.bytesReceived.Load(),
		"transport_connected": a.transport.Available(),
	}
	return s
}

// ---------------------------------------------------------------------------
// agent.MessageReceiver 인터페이스 구현
// ---------------------------------------------------------------------------

// ReceiveMessage 는 msgCh 에서 메시지를 수신한다.
func (a *LGCNPAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	a.bridgeActive.Store(true)
	select {
	case data := <-a.msgCh:
		a.stats.IncrInternalMessagesSent()
		return data, nil
	case <-a.stopCh:
		return nil, fmt.Errorf("lgcnp: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ---------------------------------------------------------------------------
// agent.StatefulAgent 인터페이스 구현
// ---------------------------------------------------------------------------

// State 는 캡처 상태를 반환한다.
func (a *LGCNPAgent) State() map[string]any {
	result := map[string]any{
		"odu_frames_captured": a.oduFramesCaptured.Load(),
		"idu_frames_captured": a.iduFramesCaptured.Load(),
		"frames_dropped":      a.framesDropped.Load(),
		"bytes_skipped":       a.bytesSkipped.Load(), // v0.18.14
		"parse_errors":        a.parseErrors.Load(),  // v0.18.14
		"idle_timeouts":       a.idleTimeouts.Load(), // v0.18.16
		"bytes_received":      a.bytesReceived.Load(),
		"transport_connected": a.transport.Available(),
	}

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
func (a *LGCNPAgent) BufferInfo() (int, int) {
	return len(a.msgCh), cap(a.msgCh)
}

// FrameNotifyCh 는 새 프레임 도착 시 신호를 보내는 채널을 반환한다.
func (a *LGCNPAgent) FrameNotifyCh() <-chan struct{} {
	return a.recentNotify
}

// ---------------------------------------------------------------------------
// device.DeviceProvider 인터페이스 구현
// ---------------------------------------------------------------------------

// DeviceProvider 는 이 에이전트의 디바이스를 device.DeviceProvider 로 노출한다.
func (a *LGCNPAgent) DeviceProvider() device.DeviceProvider {
	return NewLGCNPDeviceProvider(a)
}

// ---------------------------------------------------------------------------
// agent.TransportChecker 인터페이스 구현
// ---------------------------------------------------------------------------

// TransportConnected 는 시리얼 트랜스포트의 실제 연결 상태를 반환한다.
func (a *LGCNPAgent) TransportConnected() bool {
	return a.transport.Available()
}

// ---------------------------------------------------------------------------
// 캡처 루프 (패시브 리스닝)
// ---------------------------------------------------------------------------

// captureLoop 는 시리얼 버스에서 LGCNP-01 프레임을 패시브하게 캡처한다.
func (a *LGCNPAgent) captureLoop() {
	reader := &transportReader{transport: a.transport}
	parser := NewLGCNPFrameParser(reader)

	a.logger.Info("lgcnp: 캡처 루프 시작",
		"verify_redundancy", a.lgcnpConfig.VerifyRedundancy,
		"dedupe_frames", a.lgcnpConfig.DedupeFrames,
		"event_temp_threshold", a.lgcnpConfig.EventTempThreshold,
		"notify_interval", a.lgcnpConfig.NotifyInterval,
	)

	for {
		select {
		case <-a.stopCh:
			a.logger.Info("lgcnp: 캡처 루프 종료 (stopCh)")
			return
		default:
		}

		a.mu.RLock()
		paused := a.paused
		a.mu.RUnlock()
		if paused {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		frameType, oduFrame, iduFrame, err := parser.ReadFrame()
		if err != nil {
			if err == io.EOF {
				a.logger.Info("lgcnp: 트랜스포트 EOF, 재연결 시도")
				go a.reconnectLoop()
				return
			}
			if isLGAPConnectionError(err) {
				a.logger.Warn("lgcnp: 트랜스포트 연결 에러, 재연결 시도", "error", err)
				go a.reconnectLoop()
				return
			}
			// v0.18.16: read deadline 만료 (i/o timeout) 은 idle bus 의 정상 상태.
			// 별도 카운터만 증가, 로그는 emit 하지 않음 (spam 방지). 누적 통계는
			// get_stats 의 idle_timeouts 로 노출.
			if isLGAPTimeoutError(err) {
				a.idleTimeouts.Add(1)
				continue
			}
			// v0.18.14: 파서 에러도 DEBUG 로그로 노출 (이전엔 silent continue).
			a.parseErrors.Add(1)
			a.logger.Debug("lgcnp: 프레임 파싱 에러 — skip",
				"error", err,
				"total_errors", a.parseErrors.Load(),
			)
			continue
		}

		// v0.18.14: STX 동기화 복구로 폐기한 byte 가 있으면 DEBUG 로그.
		if skipped := parser.LastSkippedCount(); skipped > 0 {
			a.bytesSkipped.Add(int64(skipped))
			a.logger.Debug("lgcnp: STX 동기화 — 알 수 없는 byte 폐기",
				"skipped", skipped,
				"sample", hex.EncodeToString(parser.LastSkippedSample()),
				"total_skipped", a.bytesSkipped.Load(),
			)
		}

		// 통계 업데이트
		a.stats.IncrExternalMessagesReceived()
		a.stats.UpdateLastActivity()
		a.stats.RecordFirstMessage()

		switch frameType {
		case 'A':
			if a.lgcnpConfig.LogIO {
				a.logger.Info("lgcnp[io]: ODU frame parsed",
					"seq", oduFrame.SEQ,
					"checksum_valid", oduFrame.ChecksumValid,
					"raw", hex.EncodeToString(oduFrame.Raw[:]),
				)
			}
			a.handleODUFrame(oduFrame)
		case 'B':
			if a.lgcnpConfig.LogIO {
				a.logger.Info("lgcnp[io]: IDU frame parsed",
					"idu_num", iduFrame.IDUNum,
					"idu_addr", fmt.Sprintf("%02x", iduFrame.IDUAddr),
					"redundancy_valid", iduFrame.RedundancyValid,
					"structure_valid", iduFrame.StructureValid,
					"range_ok", iduFrame.RangeOk,
					"raw", hex.EncodeToString(iduFrame.Raw[:]),
				)
			}
			a.handleIDUFrame(iduFrame)
		}
	}
}

// handleODUFrame 은 TYPE-A ODU 프레임을 처리한다.
func (a *LGCNPAgent) handleODUFrame(f *LGCNPODUFrame) {
	a.oduFramesCaptured.Add(1)
	a.bytesReceived.Add(int64(lgcnpODUFrameLen))
	a.stats.AddBytesRead(int64(lgcnpODUFrameLen))
	seq := a.captureSeq.Add(1) // pushRecentFrame 내부 추적용 (event JSON 에는 노출 안 함)

	// v0.5.0 통합 schema — top-level: type/dev_id/trigger/last_seen_ms/raw_hex(옵션).
	// v0.6.8: type 을 "device_state" 로 통일 (Century/NASA 와 일치). IDU/ODU 구별은
	// dev_id ("odu" / "idu-N") + metadata.device_type 으로.
	evt := LGCNPODUFrameEvent{
		DevID:      lgcnpODUUnitID,                                                      // v0.18.12: "0"
		DeviceID:   agent.ResolveDeviceID(context.Background(), a.ID(), lgcnpODUUnitID), // v0.18.6, v0.18.12
		Trigger:    "change",
		LastSeenMs: f.Timestamp.UnixMilli(),
		Metadata: LGCNPFrameMetadata{
			Label:      "outdoor",
			DeviceType: "HVACR.ODU",
		},
	}
	// raw_hex 는 include_raw_hex=true 일 때만 노출 (운영 페이로드 절감).
	if a.lgcnpConfig.IncludeRawHex {
		evt.RawHex = hex.EncodeToString(f.Raw[:])
	}

	// 체크섬 검증 실패 시 통계만 기록.
	// v0.18.1: VerifyODUChecksum=false 면 폐기하지 않고 진행 (일부 디바이스 변형의
	// SEQ=04 가 fixed 0x55 marker 사용 — 표준 SUM checksum 과 무관).
	if !f.ChecksumValid {
		a.framesInvalid.Add(1)
		if a.lgcnpConfig.VerifyODUChecksum {
			a.logger.Debug("lgcnp: ODU 프레임 체크섬 실패 — 폐기",
				"seq", f.SEQ,
				"raw", hex.EncodeToString(f.Raw[:]),
			)
			return
		}
		a.logger.Debug("lgcnp: ODU 프레임 체크섬 mismatch (verify_odu_checksum=false 로 계속 진행)",
			"seq", f.SEQ,
			"raw", hex.EncodeToString(f.Raw[:]),
		)
	}

	// SEQ=02: 실시간 냉동 사이클 데이터
	if f.SEQ == 0x02 {
		outdoorTemp := lgcnpDecodeSensorTemp(f.Raw[6])
		suctionTemp := lgcnpDecodeSensorTemp(f.Raw[8])
		dischargeTemp := lgcnpDecodeSensorTemp(f.Raw[11])
		condenserA := lgcnpDecodeSensorTemp(f.Raw[14])
		condenserB := lgcnpDecodeSensorTemp(f.Raw[15])

		evt.State = &LGCNPODUParsed{
			OutdoorTemp:       &outdoorTemp,
			CompSuctionTemp:   &suctionTemp,
			CompDischargeTemp: &dischargeTemp,
			CondenserTempA:    &condenserA,
			CondenserTempB:    &condenserB,
		}

		a.mu.Lock()
		a.oduState.OutdoorTemp = &outdoorTemp
		a.oduState.CompSuctionTemp = &suctionTemp
		a.oduState.CompDischargeTemp = &dischargeTemp
		a.oduState.CondenserTempA = &condenserA
		a.oduState.CondenserTempB = &condenserB
		a.oduLastSeen = f.Timestamp
		a.mu.Unlock()
	}

	// SEQ=04: 운전 평균 온도
	if f.SEQ == 0x04 {
		avgTemp := lgcnpDecodeSensorTemp(f.Raw[10])

		a.mu.Lock()
		a.oduState.AvgTemp = &avgTemp
		a.oduLastSeen = f.Timestamp
		a.mu.Unlock()
	}

	// 의미 없는 메시지 차단 — state 가 없는 ODU frame (SEQ=0x01/0x03/0x05 등 미파싱
	// 또는 SEQ=0x04 처럼 evt.State 에 데이터를 싣지 않는 경우) 은 metadata 만 가진
	// 빈 메시지이므로 emit/push 모두 skip 한다. 사용자 보고 "의미 없는 메시지 제거"
	// 직접 fix. 통계 (oduFramesCaptured / bytesReceived) 는 이미 위에서 누적됨.
	if evt.State == nil {
		return
	}

	b, err := json.Marshal(evt)
	if err != nil {
		a.logger.Warn("lgcnp: ODU event marshal failed", "error", err)
		return
	}

	// v0.18.23 (2026-05-27): 정기 보고 캐시 (lastODUParsed) 를 dedup 게이트와
	// 무관하게 항상 갱신. IDU 동일 패턴.
	a.dedupMu.Lock()
	parsedCopy := *evt.State
	a.lastODUParsed = &parsedCopy
	a.dedupMu.Unlock()

	// frame dedup — state 가 직전 emit 과 동일하면 push/emit 모두 skip.
	if a.lgcnpConfig.DedupeFrames {
		if emit, reason := a.shouldEmitODU(evt.State); !emit {
			// v0.18.17: dedup drop 도 DEBUG 로그로 노출.
			a.logger.Debug("lgcnp: ODU 프레임 dedup — skip",
				"unit_id", lgcnpODUUnitID,
				"seq", f.SEQ,
				"reason", reason,
			)
			return
		}
	}

	a.pushRecentFrame(b, f.Timestamp, seq)
	if a.bridgeActive.Load() {
		a.sendFrameEvent(b)
	}
}

// shouldEmitODU 는 ODU SEQ=02 frame 의 state 가 직전 emit 한 state 와 다른지 검사한다.
// 동일하면 false (emit skip), 다르거나 첫 emit 이면 true 로 반환하고 cache 갱신.
//
// 비교 대상: outdoor_temp / comp_suction_temp / comp_discharge_temp / condenser_temp_a/b.
// timestamp_ms / seq / raw_hex 같은 매 frame 마다 바뀌는 메타는 자동 제외 (state 만 비교).
//
// v0.6.7: event_temp_threshold gate 추가. ODU 의 모든 필드가 온도이므로
// max|Δ| < threshold 면 emit suppress.
// v0.18.17: dedup 사유 반환 (DEBUG 로그 가시성).
// v0.18.21: keepalive 는 기존 notifyLoop (NotifyInterval, report_interval 옵션)
// 으로 처리. v0.18.18 의 중복 StateReportInterval 로직 제거.
func (a *LGCNPAgent) shouldEmitODU(state *LGCNPODUParsed) (bool, string) {
	if state == nil {
		return false, "nil_state"
	}
	cur, err := json.Marshal(state)
	if err != nil {
		return true, "" // marshal 실패 시 보수적으로 emit
	}
	a.dedupMu.Lock()
	defer a.dedupMu.Unlock()
	if a.lastODUEmit != nil && bytes.Equal(a.lastODUEmit, cur) {
		return false, "identical"
	}
	// v0.6.7: 온도 임계값 게이트 — ODU 의 모든 필드가 온도 (비온도 없음).
	if a.lgcnpConfig.EventTempThreshold > 0 && a.lastODUParsed != nil {
		if maxTempDeltaLGCNPODU(*a.lastODUParsed, *state) < a.lgcnpConfig.EventTempThreshold {
			return false, "temp_threshold"
		}
	}
	a.lastODUEmit = cur
	parsedCopy := *state
	a.lastODUParsed = &parsedCopy
	return true, ""
}

// maxTempDeltaLGCNPODU 는 ODU 의 온도 센서값들 중 최대 |Δ| 를 반환한다 (v0.6.7).
// pointer 가 한쪽만 nil 인 경우는 변화로 간주 (큰 값 반환).
// 둘 다 nil 이면 0 (차이 없음).
func maxTempDeltaLGCNPODU(prev, curr LGCNPODUParsed) float64 {
	return maxFloat64(
		ptrFloat64AbsDelta(prev.OutdoorTemp, curr.OutdoorTemp),
		ptrFloat64AbsDelta(prev.CompSuctionTemp, curr.CompSuctionTemp),
		ptrFloat64AbsDelta(prev.CompDischargeTemp, curr.CompDischargeTemp),
		ptrFloat64AbsDelta(prev.CondenserTempA, curr.CondenserTempA),
		ptrFloat64AbsDelta(prev.CondenserTempB, curr.CondenserTempB),
	)
}

// ptrFloat64AbsDelta 는 두 *float64 의 절대차를 반환한다 (v0.6.7).
// 한쪽만 nil 이면 매우 큰 값을 반환하여 게이트를 통과시킨다.
// 둘 다 nil 이면 0.
func ptrFloat64AbsDelta(a, b *float64) float64 {
	if a == nil && b == nil {
		return 0
	}
	if a == nil || b == nil {
		// 첫 관측 또는 nil 전이는 의미있는 변화 — 게이트 우회.
		return 1e9
	}
	return absDeltaFloat64(*a, *b)
}

// handleIDUFrame 은 TYPE-B IDU 프레임을 처리한다.
func (a *LGCNPAgent) handleIDUFrame(f *LGCNPIDUFrame) {
	a.iduFramesCaptured.Add(1)
	a.bytesReceived.Add(int64(lgcnpIDUFrameLen))
	a.stats.AddBytesRead(int64(lgcnpIDUFrameLen))
	seq := a.captureSeq.Add(1)

	// 6계층 신뢰성 검증: 이중 기록(계층2) + 구조(계층3) 필수 통과
	frameValid := f.RedundancyValid && f.StructureValid
	if !frameValid {
		a.framesInvalid.Add(1)
		if a.lgcnpConfig.VerifyRedundancy {
			a.logger.Debug("lgcnp: IDU 프레임 검증 실패 — 폐기",
				"unit_id", lgcnpIDUUnitID(f.IDUNum),
				"redundancy", f.RedundancyValid,
				"structure", f.StructureValid,
				"raw", hex.EncodeToString(f.Raw[:]),
			)
			return
		}
	}

	// CMD 주기 판별 (bit6 기반)
	cmdCycle := "A"
	if f.CycleBit {
		cmdCycle = "B"
	}

	// v0.5.0 통합 schema —
	//   top-level: type / unit_id (프로토콜 식별자) / device_id (UUID) / trigger / last_seen_ms / raw_hex (옵션)
	//   state: 5 핵심 + inlet_temp/outlet_temp
	//   metadata: slot_num (state 에서 이동)
	// v0.6.8: type 을 "device_state" 로 통일. metadata.device_type="indoor" 추가.
	// v0.18.6: unit_id (프로토콜) + device_id (UUID) 분리.
	unitID := lgcnpIDUUnitID(f.IDUNum) // v0.18.12: "1"~"5"
	evt := LGCNPIDUFrameEvent{
		DevID:      unitID,
		DeviceID:   agent.ResolveDeviceID(context.Background(), a.ID(), unitID),
		Trigger:    "change",
		LastSeenMs: f.Timestamp.UnixMilli(),
		State: &LGCNPIDUParsed{
			Power:       f.OpMode&0x20 == 0,
			TargetTemp:  f.SetTemp,
			CurrentTemp: f.RoomTemp,
			InletTemp:   f.InletTemp,
			OutletTemp:  f.OutletTemp,
			FanSpeed:    lgcnpFanSpeedToHVACID(lgcnpFanByteToID(f.FanByte, f.DevType)),
			Mode:        lgcnpOpModeToHVACID(lgcnpOpModeToID(f.OpMode)),
		},
		Metadata: LGCNPFrameMetadata{
			Label:      fmt.Sprintf("indoor-%d", f.IDUNum),
			SlotNum:    int(f.SlotNum),
			DeviceType: "HVACR.IDU",
		},
	}
	// raw_hex 는 include_raw_hex=true 일 때만 노출.
	if a.lgcnpConfig.IncludeRawHex {
		evt.RawHex = hex.EncodeToString(f.Raw[:])
	}

	b, err := json.Marshal(evt)
	if err != nil {
		a.logger.Warn("lgcnp: IDU event marshal failed", "error", err)
		return
	}

	// 디바이스 상태 갱신: RangeOk 실패해도 디바이스 등록/갱신은 수행
	// (전원 OFF 시 온도값이 정상 범위를 벗어날 수 있음).
	// v0.18.22 (2026-05-27): AutoDiscovery 게이트를 updateIDUDeviceState 내부로
	// 이동. config 등록 디바이스의 IDUNum/State 갱신이 항상 동작하도록 함.
	a.updateIDUDeviceState(f, cmdCycle)
	if !f.RangeOk {
		a.logger.Debug("lgcnp: IDU 온도 범위 초과",
			"unit_id", lgcnpIDUUnitID(f.IDUNum),
			"current_temperature", f.RoomTemp,
			"inlet_temperature", f.InletTemp,
			"outlet_temperature", f.OutletTemp,
		)
	}
	// 미인식 b[30] 풍속 바이트를 디버그 로그로 남긴다 (DEV_TYPE별 인코딩 학습용)
	// v0.18.10: DEV_TYPE 별 매핑까지 고려해 알려진 조합은 suppress.
	if !lgcnpIsKnownFanByte(f.DevType, f.FanByte) {
		a.logger.Debug("lgcnp: 미인식 풍속 바이트",
			"unit_id", lgcnpIDUUnitID(f.IDUNum),
			"fan_byte", fmt.Sprintf("0x%02X", f.FanByte),
			"device_type", fmt.Sprintf("0x%02X", f.DevType),
		)
	}

	// v0.18.23 (2026-05-27): 정기 보고 캐시 (lastIDUParsed) 를 dedup 게이트와
	// 무관하게 항상 갱신. 이전엔 shouldEmitIDU 내부에서만 갱신되어
	// DedupeFrames=false 시 lastIDUParsed 가 영원히 비어 있어 정기 보고 skip.
	if evt.State != nil {
		a.dedupMu.Lock()
		if a.lastIDUParsed == nil {
			a.lastIDUParsed = make(map[int]LGCNPIDUParsed)
		}
		a.lastIDUParsed[f.IDUNum] = *evt.State
		a.dedupMu.Unlock()
	}

	// frame dedup — 동일 IDU 의 state 가 직전 emit 과 동일하면 skip.
	if a.lgcnpConfig.DedupeFrames {
		if emit, reason := a.shouldEmitIDU(f.IDUNum, evt.State); !emit {
			// v0.18.17: dedup drop 도 DEBUG 로그로 노출 + 사유.
			a.logger.Debug("lgcnp: IDU 프레임 dedup — skip",
				"unit_id", lgcnpIDUUnitID(f.IDUNum),
				"reason", reason,
			)
			return
		}
	}

	a.pushRecentFrame(b, f.Timestamp, seq)
	if a.bridgeActive.Load() {
		a.sendFrameEvent(b)
	}
}

// shouldEmitIDU 는 IDU frame 의 state 가 직전 emit 과 다른지 검사한다.
// IDUNum 별로 캐시를 관리하여 다중 IDU 환경에서 독립 dedup.
//
// 반환값:
//   - emit=true: 새 state 로 emit 해야 함 (reason="")
//   - emit=false: dedup skip. reason 은 "identical" (전체 동일) 또는
//     "temp_threshold" (비온도 동일 + 온도 |Δ| < threshold) 또는 "nil_state".
//
// v0.6.6: event_temp_threshold gate 추가.
// v0.18.17: dedup 사유 반환 (DEBUG 로그 가시성).
// v0.18.21: keepalive 는 기존 notifyLoop (NotifyInterval, report_interval 옵션)
// 으로 처리. v0.18.18 의 중복 StateReportInterval 로직 제거.
func (a *LGCNPAgent) shouldEmitIDU(iduNum int, state *LGCNPIDUParsed) (bool, string) {
	if state == nil {
		return false, "nil_state"
	}
	cur, err := json.Marshal(state)
	if err != nil {
		return true, ""
	}
	a.dedupMu.Lock()
	defer a.dedupMu.Unlock()
	if prev, ok := a.lastIDUEmit[iduNum]; ok && bytes.Equal(prev, cur) {
		return false, "identical"
	}
	// v0.6.7: 온도 임계값 게이트 — 비온도 필드 변경 없이 온도 센서값(current/inlet/outlet)
	// 만 변경된 경우 max|Δtemp| < threshold 면 emit suppress.
	if a.lgcnpConfig.EventTempThreshold > 0 {
		if prev, ok := a.lastIDUParsed[iduNum]; ok &&
			!nonTempFieldsChangedLGCNPIDU(prev, *state) {
			if maxTempDeltaLGCNPIDU(prev, *state) < a.lgcnpConfig.EventTempThreshold {
				return false, "temp_threshold"
			}
		}
	}
	a.lastIDUEmit[iduNum] = cur
	if a.lastIDUParsed == nil {
		a.lastIDUParsed = make(map[int]LGCNPIDUParsed)
	}
	a.lastIDUParsed[iduNum] = *state
	return true, ""
}

// nonTempFieldsChangedLGCNPIDU 는 비온도 필드 (Power/TargetTemp/FanSpeed/Mode) 중
// 하나라도 변경되었는지 검사한다 (v0.6.7).
// 참고: TargetTemp 는 사용자 설정 값이라 비온도(제어) 카테고리로 분류한다.
func nonTempFieldsChangedLGCNPIDU(prev, curr LGCNPIDUParsed) bool {
	return prev.Power != curr.Power ||
		prev.TargetTemp != curr.TargetTemp ||
		prev.FanSpeed != curr.FanSpeed ||
		prev.Mode != curr.Mode
}

// maxTempDeltaLGCNPIDU 는 IDU 의 온도 센서값들 (CurrentTemp/InletTemp/OutletTemp)
// 중 최대 |Δ| 를 반환한다 (v0.6.7).
func maxTempDeltaLGCNPIDU(prev, curr LGCNPIDUParsed) float64 {
	return maxFloat64(
		absDeltaFloat64(prev.CurrentTemp, curr.CurrentTemp),
		absDeltaFloat64(prev.InletTemp, curr.InletTemp),
		absDeltaFloat64(prev.OutletTemp, curr.OutletTemp),
	)
}

// absDeltaFloat64 는 |a - b| 를 반환한다 (v0.6.7 lg 패키지 공통 헬퍼).
func absDeltaFloat64(a, b float64) float64 {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d
}

// maxFloat64 는 가변 인자 중 최대값을 반환한다 (v0.6.7).
func maxFloat64(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, v := range values[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

// pushRecentFrame 은 프레임 이벤트를 링 버퍼에 추가한다.
func (a *LGCNPAgent) pushRecentFrame(eventJSON []byte, ts time.Time, seq int64) {
	a.recentMu.Lock()
	defer a.recentMu.Unlock()

	a.recentFrames[a.recentIdx] = lgcnpFrameRecord{
		Event:     json.RawMessage(eventJSON),
		Timestamp: ts,
		Seq:       seq,
	}
	a.recentIdx = (a.recentIdx + 1) % lgcnpRecentBufferSize
	if a.recentIdx == 0 {
		a.recentFull = true
	}

	notified := false
	select {
	case a.recentNotify <- struct{}{}:
		notified = true
	default:
	}

	if a.lgcnpConfig.LogIO {
		a.logger.Info("lgcnp[io]: ring push",
			"seq", seq,
			"event_size", len(eventJSON),
			"ring_idx", a.recentIdx,
			"notified", notified,
			"bridge_active", a.bridgeActive.Load(),
		)
	}
}

// sendFrameEvent 는 프레임 이벤트를 msgCh 로 전송한다.
func (a *LGCNPAgent) sendFrameEvent(data []byte) {
	select {
	case a.msgCh <- data:
		return
	default:
	}

	// 버퍼 풀 — 가장 오래된 메시지를 드레인
	select {
	case <-a.msgCh:
	default:
	}

	dropped := a.framesDropped.Add(1)
	a.stats.IncrDroppedMessages()
	// v0.18.17: 매 회 DEBUG 로그 — rate-limited WARN 과 별도로 모든 drop 을 가시화.
	a.logger.Debug("lgcnp: msgCh full — oldest frame dropped",
		"total_dropped", dropped,
		"ch_cap", cap(a.msgCh),
	)
	now := time.Now().UnixNano()
	last := a.lastDropLog.Load()
	if now-last > 10_000_000_000 && a.lastDropLog.CompareAndSwap(last, now) {
		a.logger.Warn("lgcnp: msgCh full, dropping oldest frame",
			"total_dropped", dropped,
			"ch_cap", cap(a.msgCh),
		)
	}

	select {
	case a.msgCh <- data:
	default:
	}
}

// ---------------------------------------------------------------------------
// 재연결 루프
// ---------------------------------------------------------------------------

func (a *LGCNPAgent) reconnectLoop() {
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

	a.setAllDevicesOffline()

	baseInterval := a.lgcnpConfig.ReconnectInterval
	maxBackoff := a.lgcnpConfig.MaxReconnectBackoff
	attempt := 0

	for {
		select {
		case <-a.stopCh:
			return
		default:
		}

		_ = a.transport.Close()
		err := a.transport.Open()

		if err == nil {
			a.logger.Info("lgcnp: 트랜스포트 재연결 성공", "attempts", attempt+1)
			go a.captureLoop()
			return
		}

		a.reconnectMu.Lock()
		a.reconnectAttempts = attempt + 1
		a.reconnectMu.Unlock()

		if attempt == 0 {
			a.logger.Warn("lgcnp: 트랜스포트 재연결 시도 중", "error", err)
		}

		backoff := baseInterval
		for i := 0; i < attempt; i++ {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
				break
			}
		}
		attempt++

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
// 디바이스 관리
// ---------------------------------------------------------------------------

func (a *LGCNPAgent) registerConfigDevices() {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	for _, entry := range a.lgcnpConfig.Devices {
		if _, exists := a.iduDevices[entry.Address]; exists {
			continue
		}
		label := entry.Name
		devType := "HVACR.IDU"
		if entry.Address == "odu" {
			devType = "HVACR.ODU"
			if label == "" {
				label = "outdoor"
			}
		} else {
			if label == "" {
				label = "indoor-" + entry.Address
			}
		}
		a.iduDevices[entry.Address] = &LGCNPDevice{
			Address:  entry.Address,
			Label:    label,
			Type:     devType,
			Online:   false,
			LastSeen: now,
			Source:   "config",
			State:    &LGCNPDeviceState{},
		}
	}
}

// updateIDUDeviceState 는 IDU 프레임에서 디바이스 상태를 갱신한다.
//
// SPEC-LGCNP-001 v0.18.22 (2026-05-27): AutoDiscovery 게이트를 새 디바이스 생성에만
// 적용하도록 변경. 기존 (config 등록 / auto-발견된) 디바이스의 state / IDUNum /
// LastSeen 갱신은 AutoDiscovery 와 무관하게 항상 수행. 이전엔 handleIDUFrame
// 이 AutoDiscovery==false 일 때 본 함수를 호출 자체 안 했으므로 config 디바이스의
// 동적 상태가 절대 갱신되지 않아 정기 보고가 emit 되지 않던 결함.
func (a *LGCNPAgent) updateIDUDeviceState(f *LGCNPIDUFrame, cmdCycle string) {
	addrHex := fmt.Sprintf("%02x", f.IDUAddr)

	a.mu.Lock()
	defer a.mu.Unlock()

	dev, ok := a.iduDevices[addrHex]
	if !ok {
		// 새 디바이스 자동 등록은 AutoDiscovery 가 활성일 때만.
		// 비활성 시 미등록 주소의 frame 은 state 갱신 없이 무시.
		if !a.lgcnpConfig.AutoDiscovery {
			return
		}
		dev = &LGCNPDevice{
			Address:  addrHex,
			Label:    fmt.Sprintf("indoor-%d", f.IDUNum),
			Type:     "HVACR.IDU",
			Online:   true,
			LastSeen: f.Timestamp,
			Source:   "auto",
			State:    &LGCNPDeviceState{},
			IDUNum:   f.IDUNum,
			SlotNum:  f.SlotNum,
		}
		a.iduDevices[addrHex] = dev
		a.logger.Info("lgcnp: IDU 디바이스 발견",
			"address", addrHex, "unit_id", lgcnpIDUUnitID(f.IDUNum))
	}

	dev.Online = true
	dev.LastSeen = f.Timestamp
	// v0.7.0: slot_num 갱신 (정기 보고 metadata 재현용).
	dev.SlotNum = f.SlotNum
	// v0.18.22 (2026-05-27): IDUNum 갱신. config 로 등록된 디바이스는 생성 시점에
	// IDUNum=0 (기본값) 이므로, frame 수신 시 실제 IDUNum 으로 갱신해야 정기 보고
	// (emitPeriodicReport) 의 lastIDUParsed[d.IDUNum] 매칭이 동작한다.
	// 누락 시 config IDU 만 상태 보고가 emit 되지 않는 버그 (사용자 보고 2026-05-27).
	dev.IDUNum = f.IDUNum

	prev := dev.State.snapshot()

	// 상태 병합
	powerOn := f.OpMode&0x20 == 0 // 원시 OP_MODE bit5: 0=ON, 1=OFF
	dev.State.Power = &powerOn
	dev.State.RoomTemp = &f.RoomTemp
	dev.State.InletTemp = &f.InletTemp
	dev.State.OutletTemp = &f.OutletTemp
	fanSpeedID := lgcnpFanByteToID(f.FanByte, f.DevType)
	dev.State.FanSpeed = &fanSpeedID
	opMode := lgcnpOpModeToID(f.OpMode)
	dev.State.OpMode = &opMode
	if f.SetTempReliable {
		dev.State.SetTemp = &f.SetTemp
	}
	dev.State.CMDCycle = &cmdCycle
	devType := int(f.DevType)
	dev.State.DevType = &devType
	deviceID := int(f.DeviceID)
	dev.State.DeviceID = &deviceID

	curr := dev.State.snapshot()
	if lgcnpDeviceStateChanged(prev, curr) {
		a.lastStates[addrHex] = curr

		// SPEC-DEVICE-IDENTITY-001 Phase D § M3 — V2 단일 호출.
		if v2 := a.onDeviceStateChangeV2; v2 != nil {
			agentName := a.agentConfig.Name
			globalID := fmt.Sprintf("%s:%s", agentName, addrHex)
			deviceUID := agent.ResolveDeviceID(context.Background(), agentName, addrHex)
			go v2(agentName, deviceUID, globalID)
		}
	}
}

func (a *LGCNPAgent) setAllDevicesOffline() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, dev := range a.iduDevices {
		dev.Online = false
	}
}

func (a *LGCNPAgent) offlineWatchLoop() {
	interval := a.lgcnpConfig.OfflineTimeout / 2
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

func (a *LGCNPAgent) checkDeviceTimeouts() {
	now := time.Now()
	timeout := a.lgcnpConfig.OfflineTimeout

	a.mu.Lock()
	for _, dev := range a.iduDevices {
		if dev.Online && now.Sub(dev.LastSeen) > timeout {
			dev.Online = false
		}
	}
	a.mu.Unlock()
}

// notifyLoop 은 NotifyInterval 마다 마지막으로 수신된 상태를 trigger="report" 로
// emit 한다 (v0.6.8). 변화 없이도 운영자에게 alive 신호를 제공.
//
// 출력 형식은 change emit 과 동일 (type="device_state"). 마지막 lastIDUParsed /
// lastODUParsed 캐시 (dedupMu 보호) 를 기반으로 frame event 를 재생성한다.
func (a *LGCNPAgent) notifyLoop() {
	ticker := time.NewTicker(a.lgcnpConfig.NotifyInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.emitPeriodicReport()
		}
	}
}

// emitPeriodicReport 는 모든 등록된 IDU + ODU 의 마지막 캐시된 state 를
// trigger="report" 로 emit 한다 (v0.7.0).
//
// 단순화 모델:
//   - IDU: iduDevices 의 각 dev 에서 IDUNum/SlotNum 메타 + lastIDUParsed 의 state
//   - ODU: oduFramesCaptured>0 일 때 lastODUParsed 의 state
//
// 한 번도 frame 이 관측되지 않은 디바이스 (lastIDUParsed/lastODUParsed 비어 있음) 는 skip.
func (a *LGCNPAgent) emitPeriodicReport() {
	now := time.Now()

	// IDU: device + state snapshot 수집.
	type iduItem struct {
		iduNum  int
		slotNum byte
		state   LGCNPIDUParsed
	}
	a.mu.RLock()
	devs := make([]*LGCNPDevice, 0, len(a.iduDevices))
	for _, d := range a.iduDevices {
		devs = append(devs, d)
	}
	a.mu.RUnlock()

	a.dedupMu.Lock()
	items := make([]iduItem, 0, len(devs))
	skippedIDUs := make([]int, 0)
	parsedKeys := make([]int, 0, len(a.lastIDUParsed))
	for k := range a.lastIDUParsed {
		parsedKeys = append(parsedKeys, k)
	}
	for _, d := range devs {
		if st, ok := a.lastIDUParsed[d.IDUNum]; ok {
			items = append(items, iduItem{iduNum: d.IDUNum, slotNum: d.SlotNum, state: st})
		} else {
			skippedIDUs = append(skippedIDUs, d.IDUNum)
		}
	}
	var odu *LGCNPODUParsed
	if a.lastODUParsed != nil {
		cp := *a.lastODUParsed
		odu = &cp
	}
	a.dedupMu.Unlock()

	// v0.18.23: 진단 로그 (LogIO 활성 시 INFO, 평시 DEBUG). 상태 보고 누락
	// 원인 추적: idu_devices_total / lastIDUParsed_keys / emitted_items / skipped_idu_nums.
	if a.lgcnpConfig.LogIO {
		a.logger.Info("lgcnp[io]: periodic report",
			"idu_devices_total", len(devs),
			"lastIDUParsed_keys", parsedKeys,
			"emitted_idu_count", len(items),
			"skipped_idu_nums", skippedIDUs,
			"odu_observed", odu != nil,
			"bridge_active", a.bridgeActive.Load(),
		)
	} else {
		a.logger.Debug("lgcnp: emitPeriodicReport",
			"idu_devices_total", len(devs),
			"lastIDUParsed_size", len(parsedKeys),
			"emitted_idu_count", len(items),
			"skipped_idu_count", len(skippedIDUs),
			"odu_observed", odu != nil,
		)
	}

	for _, it := range items {
		state := it.state
		a.emitIDUDeviceState(it.iduNum, it.slotNum, &state, "report", now)
	}
	if odu != nil {
		a.emitODUDeviceState(odu, "report", now)
	}
}

// emitIDUDeviceState 는 IDU 디바이스 상태를 통합 schema (type="device_state") 로
// emit 한다 (v0.7.0). recentFrames + msgCh (bridge 활성 시) 양쪽에 push.
//
// v0.18.21: unit_id 정수 형식 ("1"~"5") + device_id UUID 통일 (v0.18.12 표준).
func (a *LGCNPAgent) emitIDUDeviceState(iduNum int, slot byte, state *LGCNPIDUParsed, trigger string, now time.Time) {
	unitID := lgcnpIDUUnitID(iduNum)
	evt := LGCNPIDUFrameEvent{
		DevID:      unitID,
		DeviceID:   agent.ResolveDeviceID(context.Background(), a.ID(), unitID),
		Trigger:    trigger,
		LastSeenMs: now.UnixMilli(),
		State:      state,
		Metadata: LGCNPFrameMetadata{
			Label:      fmt.Sprintf("indoor-%d", iduNum),
			SlotNum:    int(slot),
			DeviceType: "HVACR.IDU",
		},
	}
	b, err := json.Marshal(evt)
	if err != nil {
		return
	}
	seq := a.captureSeq.Add(1)
	a.pushRecentFrame(b, now, seq)
	if a.bridgeActive.Load() {
		a.sendFrameEvent(b)
	}
}

// emitODUDeviceState 는 ODU 디바이스 상태를 통합 schema 로 emit 한다 (v0.7.0).
//
// v0.18.21: unit_id="0" + device_id UUID 통일 (v0.18.12 표준).
func (a *LGCNPAgent) emitODUDeviceState(state *LGCNPODUParsed, trigger string, now time.Time) {
	evt := LGCNPODUFrameEvent{
		DevID:      lgcnpODUUnitID,
		DeviceID:   agent.ResolveDeviceID(context.Background(), a.ID(), lgcnpODUUnitID),
		Trigger:    trigger,
		LastSeenMs: now.UnixMilli(),
		State:      state,
		Metadata: LGCNPFrameMetadata{
			Label:      "outdoor",
			DeviceType: "HVACR.ODU",
		},
	}
	b, err := json.Marshal(evt)
	if err != nil {
		return
	}
	seq := a.captureSeq.Add(1)
	a.pushRecentFrame(b, now, seq)
	if a.bridgeActive.Load() {
		a.sendFrameEvent(b)
	}
}

// SetDeviceStateChangeCallbackV2 는 1급 V2 콜백을 등록한다.
// (agentName, deviceUID, deviceCompositeID) 인자. UUID 가 1급.
//
// SPEC-DEVICE-IDENTITY-001 § M3 (Phase D — V1 setter 제거).
func (a *LGCNPAgent) SetDeviceStateChangeCallbackV2(fn agent.DeviceStateChangeCallbackV2) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onDeviceStateChangeV2 = fn
}

// ListDevices 는 현재 관리 중인 모든 디바이스의 스냅샷을 반환한다.
func (a *LGCNPAgent) ListDevices() []LGCNPDevice {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]LGCNPDevice, 0, len(a.iduDevices)+1)

	// ODU 디바이스
	oduSnap := a.oduState.snapshot()
	result = append(result, LGCNPDevice{
		Address:  "odu",
		Label:    "outdoor",
		Type:     "HVACR.ODU",
		Online:   a.oduFramesCaptured.Load() > 0,
		LastSeen: a.oduLastSeen,
		Source:   "auto",
		ODUState: &oduSnap,
	})

	// IDU 디바이스
	for _, dev := range a.iduDevices {
		cp := *dev
		if dev.State != nil {
			s := dev.State.snapshot()
			cp.State = &s
		}
		result = append(result, cp)
	}
	return result
}
