package samsung

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/hvac"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// Hvacr01Agent 는 Samsung NASA HVAC 에이전트이다.
// agent.Agent, agent.MessageReceiver 인터페이스를 구현한다.
type Hvacr01Agent struct {
	*lifecycle.BaseLifecycle
	agentConfig   agent.AgentConfig
	hvacr01Config Hvacr01Config
	devices       map[NasaAddress]*NasaDevice
	deviceIDs     map[string]NasaAddress // device_id -> address 역참조
	transport     NasaTransport
	protocol      NasaProtocol
	mu            sync.RWMutex
	seqNum        byte
	pollTicker    *time.Ticker
	notifyTicker  *time.Ticker
	lastStates    map[NasaAddress]NasaDeviceState
	wg            sync.WaitGroup
	stopCh        chan struct{}
	msgCh         chan []byte // Bridge 메시지 (ReceiveMessage)
	stats         *agent.AgentStats
	logger        *slog.Logger
	startedAt     time.Time
	createdAt     time.Time
	paused        bool
	warnedUnknown map[NasaAddress]bool // 미등록 주소 최초 경고 여부

	disconnectCh      chan struct{} // 연결 끊김 시그널 (receiveLoop → pollLoop)
	reconnectMu       sync.Mutex    // reconnecting 상태 보호
	isReconnecting    bool          // 재연결 진행 중 여부
	reconnectAttempts int           // 현재 재연결 시도 횟수

	statusQueryCancel context.CancelFunc // 진행 중인 상태 조회 goroutine 취소

	// 디바이스 상태 변경 알림 (폴링 노드용)
	stateNotify chan struct{}

	// cachedAllStates 는 processGetAllStates 응답의 atomic 캐시이다.
	// handleMessage 종료 시 write lock 내에서 갱신되므로, 읽기 측은 락 없이 접근 가능하다.
	cachedAllStates atomic.Value // []byte

	// recentSnapshots 는 상태 변경 스냅샷의 링버퍼이다 (LGCP get_recent 패턴).
	// handleMessage 종료 시 write lock 내에서 push되며, get_recent_states 에서 drain한다.
	recentMu        sync.Mutex
	recentSnapshots []recentStateEntry
	recentSeq       int64

	// onDeviceStateChangeV2 는 Phase D 의 1급 콜백 (UUID + composite).
	// Phase D (xflowd v1.0) 부터 V1 시그니처는 완전 제거됨.
	// SPEC-DEVICE-IDENTITY-001 § M3.
	onDeviceStateChangeV2 agent.DeviceStateChangeCallbackV2
}

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*Hvacr01Agent)(nil)
var _ agent.MessageReceiver = (*Hvacr01Agent)(nil)
var _ agent.StatefulAgent = (*Hvacr01Agent)(nil)
var _ agent.BufferInfoProvider = (*Hvacr01Agent)(nil)
var _ agent.TransportChecker = (*Hvacr01Agent)(nil)

// TransportConnected 는 시리얼 트랜스포트의 실제 연결 상태를 반환한다.
func (a *Hvacr01Agent) TransportConnected() bool {
	return a.transport.Available()
}

// DeviceProvider 는 이 에이전트의 디바이스를 device.DeviceProvider 로 노출한다.
func (a *Hvacr01Agent) DeviceProvider() device.DeviceProvider {
	return NewHvacr01DeviceProvider(a)
}

// SetDeviceStateChangeCallbackV2 는 1급 V2 콜백을 등록한다.
// (agentName, deviceUID, deviceCompositeID) 인자. UUID 가 1급.
//
// SPEC-DEVICE-IDENTITY-001 § M3 (Phase D — V1 setter 제거).
func (a *Hvacr01Agent) SetDeviceStateChangeCallbackV2(fn agent.DeviceStateChangeCallbackV2) {
	a.onDeviceStateChangeV2 = fn
}

// processRequest 는 Process 메서드의 JSON 요청 구조체이다.
// recentStateEntry 는 상태 스냅샷 링버퍼의 항목이다.
type recentStateEntry struct {
	Seq  int64  `json:"seq"`
	Data []byte `json:"data"` // 개별 디바이스 상태 JSON
}

const recentSnapshotsCapacity = 128

type processRequest struct {
	Command    string         `json:"command"`
	Address    string         `json:"address,omitempty"`
	DeviceID   string         `json:"device_id,omitempty"`
	Params     map[string]any `json:"params,omitempty"`
	DeviceType string         `json:"device_type,omitempty"`
	NodeID     string         `json:"node_id,omitempty"` // 호출 노드 식별자 (노드별 통계용)
	FlowID     string         `json:"flow_id,omitempty"` // 호출 플로우 식별자 (노드별 통계용)
}

// NewHvacr01Agent 는 Hvacr01Agent 팩토리 함수이다.
func NewHvacr01Agent(config agent.AgentConfig) (agent.Agent, error) {
	hvacr01Config, err := parseHvacr01Config(config.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("samsung_hvacr01 agent: %w", err)
	}

	transport, err := NewNasaTransport(hvacr01Config.TransportType, config.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("samsung_hvacr01 agent: %w", err)
	}

	protocol := NewNasaProtocol()

	a := &Hvacr01Agent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("samsung_hvacr01")),
		hvacr01Config: hvacr01Config,
		devices:       make(map[NasaAddress]*NasaDevice),
		deviceIDs:     make(map[string]NasaAddress),
		transport:     transport,
		protocol:      protocol,
		lastStates:    make(map[NasaAddress]NasaDeviceState),
		warnedUnknown: make(map[NasaAddress]bool),
		disconnectCh:  make(chan struct{}),
		stopCh:        make(chan struct{}),
		msgCh:         make(chan []byte, hvacr01Config.MsgChannelSize),
		stateNotify:   make(chan struct{}, 1),
		stats:         agent.NewAgentStats(),
		logger:        agent.ResolveLogger(config),
		createdAt:     time.Now(),
	}

	// 설정에 정의된 디바이스 등록
	for _, entry := range hvacr01Config.Devices {
		addr, parseErr := ParseNasaAddress(entry.Address)
		if parseErr != nil {
			return nil, fmt.Errorf("samsung_hvacr01 agent: invalid device address %q: %w", entry.Address, parseErr)
		}
		devType := DetectDeviceType(addr)
		// source 보존: 영속화 왕복에서 런타임("bridge") 디바이스가 출처를 유지하도록
		// entry.Source 를 우선한다. 비어 있으면(yaml 선언 또는 구 포맷) "config".
		source := entry.Source
		if source == "" {
			source = "config"
		}
		dev := &NasaDevice{
			Address: addr,
			Type:    devType,
			UnitID:  entry.Name,
			Online:  false,
			Source:  source,
		}
		if devType == "HVACR.IDU" {
			dev.State = &NasaDeviceState{RawMessageSets: make(map[uint16][]byte)}
		}
		a.devices[addr] = dev
		if entry.Name != "" {
			a.deviceIDs[entry.Name] = addr
		}
	}

	if err := a.Init(config); err != nil {
		return nil, err
	}

	return a, nil
}

// Init 은 에이전트를 초기화한다.
func (a *Hvacr01Agent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("samsung_hvacr01 init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("samsung_hvacr01 init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("samsung_hvacr01 init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	a.logger.Info("samsung_hvacr01: 에이전트 초기화 완료",
		"transport", a.hvacr01Config.TransportType,
		"devices", len(a.devices),
	)

	return nil
}

// Start 는 트랜스포트를 열고 폴링/수신 루프를 시작한다.
func (a *Hvacr01Agent) Start(ctx context.Context) error {
	if a.CurrentState() == lifecycle.StateRunning && a.transport.Available() {
		return nil // 이미 실행 중이면 no-op
	}

	// 동일 인스턴스 재기동 (Stop → Start) 시 stopCh / disconnectCh 가 이전 Stop 에서
	// close 된 채로 남아있으면 새 receiveLoop / pollLoop 가 닫힌 채널을 만나
	// 즉시 종료된다. 새 채널로 교체하여 회귀를 방지한다.
	// (Manager.Restart 는 새 인스턴스를 생성하므로 영향받지 않지만, 직접 Stop/Start
	// 호출 경로를 방어한다.)
	a.mu.Lock()
	select {
	case <-a.stopCh:
		a.stopCh = make(chan struct{})
	default:
	}
	select {
	case <-a.disconnectCh:
		a.disconnectCh = make(chan struct{})
	default:
	}
	a.mu.Unlock()

	if err := a.transport.Open(); err != nil {
		// 연결 실패 시 에러 반환 대신 재연결 루프 시작
		a.logger.Warn("samsung_hvacr01: 트랜스포트 연결 실패, 재연결 대기", "error", err)
		a.wg.Add(1)
		go func() { defer a.wg.Done(); a.reconnectLoop() }()
	} else {
		// 연결 성공 시 정상 루프 시작
		a.wg.Add(2)
		go func() { defer a.wg.Done(); a.pollLoop() }()
		go func() { defer a.wg.Done(); a.receiveLoop() }()
	}

	a.mu.Lock()
	if a.hvacr01Config.NotifyInterval > 0 {
		a.notifyTicker = time.NewTicker(a.hvacr01Config.NotifyInterval)
		// v0.6.8: notifyLoop goroutine 시작 (이전: ticker 만 생성되고 소비 안 됨).
		a.wg.Add(1)
		go func() { defer a.wg.Done(); a.notifyLoop() }()
	}
	a.mu.Unlock()

	// SPEC-HVACR-CONNSTATE-001: 비동기 startup probe + 주기 connection-report 루프.
	// 기존 a.wg 패턴을 그대로 사용하여 Stop 시 join 된다(§7.2). Start 는 probe 완료를
	// 기다리지 않고 즉시 반환한다(E6/AC-8).
	a.wg.Add(1)
	go a.startupProbeLoop()
	if a.hvacr01Config.ConnectionReportInterval > 0 {
		a.wg.Add(1)
		go a.connectionReportLoop()
	}

	a.logger.Info("samsung_hvacr01: 에이전트 시작 완료")
	return nil
}

// notifyLoop 은 NotifyInterval 마다 모든 온라인 디바이스의 마지막 캐시된 상태를
// trigger="report" 로 emit 한다 (v0.6.8).
// pushRecentSnapshot 패턴을 재사용하되 trigger 만 "report" 로 차별화.
func (a *Hvacr01Agent) notifyLoop() {
	for {
		a.mu.RLock()
		t := a.notifyTicker
		a.mu.RUnlock()
		if t == nil {
			return
		}
		select {
		case <-a.stopCh:
			return
		case <-t.C:
			a.emitPeriodicReport()
		}
	}
}

// emitPeriodicReport 는 모든 등록된 디바이스를 순회하며 trigger="report" 스냅샷을
// 송신한다 (v0.6.8).
//
// 실외기(ODU)·offline 디바이스 포함: 이전에는 dev.State==nil(실외기 등 운전상태가
// 없는 디바이스)을 skip 해 실외기 상태 전송이 누락됐다. State 유무·online 여부와
// 무관하게 등록된 모든 디바이스를 보고한다 — 실외기는 online/ready, offline
// 디바이스는 online=false 로 전송된다(연결 상태 가시성).
func (a *Hvacr01Agent) emitPeriodicReport() {
	a.mu.Lock()
	addrs := make([]NasaAddress, 0, len(a.devices))
	for addr := range a.devices {
		addrs = append(addrs, addr)
	}
	a.mu.Unlock()

	for _, addr := range addrs {
		a.mu.Lock()
		dev := a.devices[addr]
		if dev == nil {
			a.mu.Unlock()
			continue
		}
		a.pushRecentSnapshotWithTrigger(addr, "report")
		a.mu.Unlock()
	}
}

// Stop 은 에이전트를 정지한다.
func (a *Hvacr01Agent) Stop(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("samsung_hvacr01 stop: %w", err)
	}

	// goroutine 들에게 종료 시그널
	close(a.stopCh)

	a.mu.Lock()
	if a.statusQueryCancel != nil {
		a.statusQueryCancel()
	}
	if a.pollTicker != nil {
		a.pollTicker.Stop()
		a.pollTicker = nil
	}
	if a.notifyTicker != nil {
		a.notifyTicker.Stop()
		a.notifyTicker = nil
	}
	a.mu.Unlock()

	// goroutine 종료 대기
	a.wg.Wait()

	// 트랜스포트 닫기
	if err := a.transport.Close(); err != nil {
		a.logger.Warn("samsung_hvacr01: transport close error", "error", err)
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
		return fmt.Errorf("samsung_hvacr01 stop: %w", err)
	}

	return nil
}

// Pause 는 Running -> Paused 로 전환한다.
func (a *Hvacr01Agent) Pause(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("samsung_hvacr01 pause: %w", err)
	}
	a.mu.Lock()
	a.paused = true
	a.mu.Unlock()
	return nil
}

// Resume 은 Paused -> Running 으로 전환한다.
func (a *Hvacr01Agent) Resume(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("samsung_hvacr01 resume: %w", err)
	}
	a.mu.Lock()
	a.paused = false
	a.mu.Unlock()
	return nil
}

// Health 는 에이전트의 건강 상태를 반환한다.
func (a *Hvacr01Agent) Health() agent.HealthStatus {
	now := time.Now()
	state := a.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		return agent.HealthStatus{
			Status:    agent.HealthHealthy,
			LastCheck: now,
			Message:   "samsung_hvacr01 agent is running",
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "samsung_hvacr01 agent is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("samsung_hvacr01 agent is in %s state", state),
		}
	}
}

// Process 는 JSON 명령을 디스패치하여 처리한다.
// 통계는 개별 커맨드 핸들러에서 의미 있는 데이터 전송 시에만 기록한다.
// 폴링 쿼리(get_recent_states 등)는 실제 데이터가 있을 때만 카운트한다.
func (a *Hvacr01Agent) Process(data []byte) ([]byte, error) {
	var req processRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("samsung_hvacr01 process: invalid JSON: %w", err)
	}

	// 2026-05-29: control_enabled 게이트. false 면 능동 제어 명령 거부 (read-only 는 허용).
	switch req.Command {
	case "set_power", "set_mode", "target_temperature", "set_fan_speed", "set_multiple":
		if !a.hvacr01Config.ControlEnabled {
			return nil, fmt.Errorf("samsung_hvacr01: control disabled (control_enabled=false)")
		}
	}

	switch req.Command {
	case "set_power":
		return a.processSetPower(&req)
	case "set_mode":
		return a.processSetMode(&req)
	case "target_temperature":
		return a.processSetTemperature(&req)
	case "set_fan_speed":
		return a.processSetFanSpeed(&req)
	case "set_multiple":
		return a.processSetMultiple(&req)
	case "get_state":
		return a.processGetState(&req)
	case "get_stats":
		// v0.7.3: 5개 HVAC 노드 통일 명령. 에이전트 캡처/송수신 통계 반환.
		return a.processGetStats()
	case "get_all", "get_all_states":
		// v0.7.1: get_all_states → get_all (5개 HVAC 노드 명령 통일).
		// get_all_states 는 deprecation alias 로 silent accept.
		return a.processGetAllStates()
	case "get_recent", "get_recent_states":
		// v0.7.1: get_recent_states → get_recent (5개 HVAC 노드 명령 통일).
		// count > 0: 최근 count 개 snapshot. count == 0: drain (NASA 는 cumulative
		// buffer 라 drain 자체는 의미 없지만, 호환성 위해 count<=0 시 default 10 사용).
		return a.processGetRecentStates(&req)
	case "request_state":
		// v0.18.26 (2026-05-28): 노드의 inactivity-fallback 요청.
		// 등록된 모든 디바이스의 마지막 캐시된 상태를 trigger="response" 로
		// emit 한다. 노드는 FrameNotifyCh 신호를 받아 drain 으로 메시지 수신.
		// node 가 보낸 group_id / unit_id 어드레싱은 현재 인스턴스에서는
		// broadcast 로 동작 (forward-compat). 노드 측 필터가 적용된다.
		return a.processRequestState(&req)
	case "add_device":
		return a.processAddDevice(&req)
	case "remove_device":
		return a.processRemoveDevice(&req)
	case "list_devices":
		return a.processListDevices()
	default:
		return nil, ErrInvalidCommand
	}
}

// processRequestState 는 모든 등록된 디바이스의 마지막 상태를 trigger="response"
// 로 push 경로에 emit 한다 (v0.18.26).
//
// 노드의 inactivity-fallback 모델 지원:
//   - 노드가 inactivity_timeout 동안 frame 신호를 받지 못하면 request_state 발송
//   - 에이전트가 캐시된 상태를 push 경로 (msgCh + ring buffer + FrameNotifyCh)
//     로 재emit → 노드가 drain 으로 흡수
//
// 현재 구현은 broadcast (모든 디바이스). req.Params 의 unit_id / group_id 는
// 후속 버전에서 정밀 타깃팅을 위해 사용 (현재는 무시).
func (a *Hvacr01Agent) processRequestState(req *processRequest) ([]byte, error) {
	_ = req // 어드레싱 파라미터는 forward-compat 차원에서 수용만, 동작은 broadcast.

	a.mu.Lock()
	addrs := make([]NasaAddress, 0, len(a.devices))
	for addr := range a.devices {
		addrs = append(addrs, addr)
	}
	a.mu.Unlock()

	emitted := 0
	for _, addr := range addrs {
		a.mu.Lock()
		dev := a.devices[addr]
		if dev == nil || dev.State == nil {
			a.mu.Unlock()
			continue
		}
		a.pushRecentSnapshotWithTrigger(addr, "response")
		a.mu.Unlock()
		emitted++
	}

	return json.Marshal(map[string]any{
		"status":  "ok",
		"emitted": emitted,
	})
}

// ---------------------------------------------------------------------------
// 제어 명령 처리
// ---------------------------------------------------------------------------

// processSetPower 는 전원 켜기/끄기 명령을 처리한다.
func (a *Hvacr01Agent) processSetPower(req *processRequest) ([]byte, error) {
	addr, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}
	if !dev.Online {
		return nil, ErrDeviceOffline
	}

	power, ok := req.Params["power"].(bool)
	if !ok {
		return nil, fmt.Errorf("samsung_hvacr01: power parameter must be boolean")
	}

	a.logger.Debug("samsung_hvacr01: set_power 요청", "device", dev.UnitID, "addr", addr.String(), "power", power)

	var val byte
	if power {
		val = 0x01
	}

	sets := []NasaMessageSet{{Index: MsgPower, Value: []byte{val}}}
	if err := a.sendControlCommand(addr, sets); err != nil {
		return nil, err
	}

	// 제어 명령 후 즉시 상태 조회를 전송하여 실제 하드웨어 상태를 빠르게 반영한다.
	a.sendImmediateStatusQuery(addr)

	return a.buildSuccessResponse(addr, dev.UnitID, map[string]any{"power": power})
}

// processSetMode 는 운전 모드 변경 명령을 처리한다.
func (a *Hvacr01Agent) processSetMode(req *processRequest) ([]byte, error) {
	addr, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}
	if !dev.Online {
		return nil, ErrDeviceOffline
	}

	modeStr, ok := req.Params["mode"].(string)
	if !ok {
		return nil, fmt.Errorf("samsung_hvacr01: mode parameter must be string")
	}

	modeVal, exists := StringToMode[modeStr]
	if !exists {
		return nil, ErrInvalidMode
	}

	a.logger.Debug("samsung_hvacr01: set_mode 요청", "device", dev.UnitID, "addr", addr.String(), "mode", modeStr)

	sets := []NasaMessageSet{{Index: MsgMode, Value: []byte{modeVal}}}
	if err := a.sendControlCommand(addr, sets); err != nil {
		return nil, err
	}

	a.sendImmediateStatusQuery(addr)

	return a.buildSuccessResponse(addr, dev.UnitID, map[string]any{"mode": modeStr})
}

// processSetTemperature 는 목표 온도 설정 명령을 처리한다.
func (a *Hvacr01Agent) processSetTemperature(req *processRequest) ([]byte, error) {
	addr, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}
	if !dev.Online {
		return nil, ErrDeviceOffline
	}

	tempVal, ok := req.Params["target_temperature"].(float64)
	if !ok {
		return nil, fmt.Errorf("samsung_hvacr01: target_temp parameter must be number")
	}

	if tempVal < 16.0 || tempVal > 30.0 {
		return nil, ErrTemperatureOutOfRange
	}

	a.logger.Debug("samsung_hvacr01: target_temperature 요청", "device", dev.UnitID, "addr", addr.String(), "target_temperature", tempVal)

	encoded := EncodeTemperature(float32(tempVal))
	sets := []NasaMessageSet{{Index: MsgTargetTemp, Value: []byte{byte(encoded >> 8), byte(encoded & 0xFF)}}}
	if err := a.sendControlCommand(addr, sets); err != nil {
		return nil, err
	}

	a.sendImmediateStatusQuery(addr)

	return a.buildSuccessResponse(addr, dev.UnitID, map[string]any{"target_temperature": tempVal})
}

// processSetFanSpeed 는 팬 속도 변경 명령을 처리한다.
func (a *Hvacr01Agent) processSetFanSpeed(req *processRequest) ([]byte, error) {
	addr, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}
	if !dev.Online {
		return nil, ErrDeviceOffline
	}

	speedStr, ok := req.Params["fan_speed"].(string)
	if !ok {
		return nil, fmt.Errorf("samsung_hvacr01: fan_speed parameter must be string")
	}

	speedVal, exists := StringToFanSpeed[speedStr]
	if !exists {
		return nil, ErrInvalidFanSpeed
	}

	a.logger.Debug("samsung_hvacr01: set_fan_speed 요청", "device", dev.UnitID, "addr", addr.String(), "fan_speed", speedStr)

	sets := []NasaMessageSet{{Index: MsgFanSpeed, Value: []byte{speedVal}}}
	if err := a.sendControlCommand(addr, sets); err != nil {
		return nil, err
	}

	a.sendImmediateStatusQuery(addr)

	return a.buildSuccessResponse(addr, dev.UnitID, map[string]any{"fan_speed": speedStr})
}

// processSetMultiple 는 복수 설정 변경 명령을 처리한다.
func (a *Hvacr01Agent) processSetMultiple(req *processRequest) ([]byte, error) {
	addr, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}
	if !dev.Online {
		return nil, ErrDeviceOffline
	}

	var sets []NasaMessageSet
	result := make(map[string]any)

	// power (nil이면 건너뜀)
	if powerVal, ok := req.Params["power"]; ok && powerVal != nil {
		power, _ := powerVal.(bool)
		var val byte
		if power {
			val = 0x01
		}
		sets = append(sets, NasaMessageSet{Index: MsgPower, Value: []byte{val}})
		result["power"] = power
	}

	// mode (nil이면 건너뜀)
	if modeVal, ok := req.Params["mode"]; ok && modeVal != nil {
		modeStr, _ := modeVal.(string)
		modeByte, exists := StringToMode[modeStr]
		if !exists {
			return nil, ErrInvalidMode
		}
		sets = append(sets, NasaMessageSet{Index: MsgMode, Value: []byte{modeByte}})
		result["mode"] = modeStr
	}

	// target_temp (nil이면 건너뜀)
	if tempVal, ok := req.Params["target_temperature"]; ok && tempVal != nil {
		temp, _ := tempVal.(float64)
		if temp < 16.0 || temp > 30.0 {
			return nil, ErrTemperatureOutOfRange
		}
		encoded := EncodeTemperature(float32(temp))
		sets = append(sets, NasaMessageSet{Index: MsgTargetTemp, Value: []byte{byte(encoded >> 8), byte(encoded & 0xFF)}})
		result["target_temperature"] = temp
	}

	// fan_speed (nil이면 건너뜀)
	if speedVal, ok := req.Params["fan_speed"]; ok && speedVal != nil {
		speedStr, _ := speedVal.(string)
		speedByte, exists := StringToFanSpeed[speedStr]
		if !exists {
			return nil, ErrInvalidFanSpeed
		}
		sets = append(sets, NasaMessageSet{Index: MsgFanSpeed, Value: []byte{speedByte}})
		result["fan_speed"] = speedStr
	}

	if len(sets) == 0 {
		return nil, fmt.Errorf("samsung_hvacr01: set_multiple requires at least one setting")
	}

	if err := a.sendControlCommand(addr, sets); err != nil {
		return nil, err
	}

	return a.buildSuccessResponse(addr, dev.UnitID, result)
}

// ---------------------------------------------------------------------------
// 상태 조회 명령 처리
// ---------------------------------------------------------------------------

// processGetStats 는 에이전트의 캡처/송수신 통계를 반환한다 (v0.7.3).
// 5개 HVAC 노드 통일 명령 — Century/LG ICP-01/LGCP 의 get_stats 패턴 차용.
func (a *Hvacr01Agent) processGetStats() ([]byte, error) {
	snap := a.stats.Snapshot()

	a.mu.RLock()
	devicesCount := len(a.devices)
	a.mu.RUnlock()

	a.reconnectMu.Lock()
	reconnecting := a.isReconnecting
	reconnectAttempts := a.reconnectAttempts
	a.reconnectMu.Unlock()

	stats := map[string]any{
		"external_messages_received": snap.ExternalMessagesReceived,
		"external_messages_sent":     snap.ExternalMessagesSent,
		"internal_messages_received": snap.InternalMessagesReceived,
		"internal_messages_sent":     snap.InternalMessagesSent,
		"messages_errored":           snap.MessagesErrored,
		"bytes_read":                 snap.BytesRead,
		"bytes_written":              snap.BytesWritten,
		"devices_count":              devicesCount,
		"transport_connected":        a.transport.Available(),
		"reconnecting":               reconnecting,
		"reconnect_attempts":         reconnectAttempts,
	}
	return json.Marshal(stats)
}

// processGetState 는 단일 디바이스 상태 조회 명령을 처리한다.
func (a *Hvacr01Agent) processGetState(req *processRequest) ([]byte, error) {
	addr, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}

	resp := map[string]any{
		"status":      "ok",
		"address":     addr.String(),
		"unit_id":     effectiveDeviceID(addr, dev.UnitID),
		"device_id":   agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, effectiveDeviceID(addr, dev.UnitID)),
		"device_type": dev.Type,
		"online":      dev.Online,
	}

	if dev.State != nil {
		resp["state"] = dev.State.StateForJSON(a.hvacr01Config.IncludeRawHex)
	}
	if !dev.LastSeen.IsZero() {
		resp["last_seen_ms"] = dev.LastSeen.UnixMilli()
	}

	return json.Marshal(resp)
}

// processGetAllStates 는 전체 디바이스 상태 조회 명령을 처리한다.
// atomic 캐시에서 즉시 반환하여 write lock 경합을 회피한다.
func (a *Hvacr01Agent) processGetAllStates() ([]byte, error) {
	if v := a.cachedAllStates.Load(); v != nil {
		return v.([]byte), nil
	}
	// 캐시가 없는 경우 (최초 호출) — 직접 빌드
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.buildAllStatesJSON()
}

// buildAllStatesJSON 는 전체 디바이스 상태를 JSON으로 직렬화한다.
// 호출 시 a.mu 락(읽기 또는 쓰기)이 잡혀 있어야 한다.
func (a *Hvacr01Agent) buildAllStatesJSON() ([]byte, error) {
	var devices []map[string]any
	for addr, dev := range a.devices {
		d := map[string]any{
			"address":     addr.String(),
			"unit_id":     effectiveDeviceID(addr, dev.UnitID),
			"device_id":   agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, effectiveDeviceID(addr, dev.UnitID)),
			"device_type": dev.Type,
			"online":      dev.Online,
		}
		if dev.State != nil {
			d["state"] = dev.State.StateForJSON(a.hvacr01Config.IncludeRawHex)
		}
		if !dev.LastSeen.IsZero() {
			d["last_seen_ms"] = dev.LastSeen.UnixMilli()
		}
		devices = append(devices, d)
	}

	return json.Marshal(map[string]any{
		"status":  "ok",
		"devices": devices,
	})
}

// pushRecentSnapshot 는 변경된 단일 디바이스의 상태를 링버퍼에 push한다.
// 호출 시 a.mu 쓰기 락이 잡혀 있어야 한다.
// addr 이 지정되면 해당 디바이스만 저장하여 메시지 증폭을 방지한다.
//
// v0.5.0 통합 schema (사용자 요구 — 3 에이전트 공통 출력 형식):
//   - top-level: type, dev_id, trigger, last_seen_ms
//   - state: online + 5 핵심 + swing_vertical/filter_alarm/error_code (NASA only)
//   - metadata: label, device_type
//   - 제거: top-level address (필요 시 metadata 확장), timestamp_ms
func (a *Hvacr01Agent) pushRecentSnapshot(addr NasaAddress) {
	a.pushRecentSnapshotWithTrigger(addr, "change")
}

// pushRecentSnapshotWithTrigger 는 trigger 를 명시적으로 지정해 스냅샷을 push 한다 (v0.6.8).
// 정기 보고 (notifyLoop) 에서는 "report", 변경 감지 시는 "change" 로 호출된다.
func (a *Hvacr01Agent) pushRecentSnapshotWithTrigger(addr NasaAddress, trigger string) {
	dev, ok := a.devices[addr]
	if !ok {
		return
	}

	// state 그룹 빌드: online 을 시작으로 NasaDeviceState 의 필드 흡수.
	state := map[string]any{"online": dev.Online}
	if dev.State != nil {
		// StateForJSON 결과를 unmarshal 해 state 맵에 평탄화 — online 과 함께 단일 그룹.
		if raw, err := json.Marshal(dev.State.StateForJSON(a.hvacr01Config.IncludeRawHex)); err == nil {
			var inner map[string]any
			if json.Unmarshal(raw, &inner) == nil {
				for k, v := range inner {
					state[k] = v
				}
			}
		}
	} else {
		// 운전상태가 없는 디바이스(실외기 ODU 등)는 통신 준비(ready) 를 상태로 노출한다.
		// 실외기는 online + ready 가 유일한 의미있는 상태이다.
		state["ready"] = dev.Ready
	}

	// metadata 그룹 빌드 (v0.6.4: label fallback chain — Name → DeviceID → address).
	// 사용자 보고 "NASA metadata 에 이름 누락" 의 fix: auto-discovered 디바이스는
	// Name 이 비어있어도 DeviceID / address 로 항상 라벨이 채워진다.
	metadata := map[string]any{}
	label := dev.Name
	if label == "" {
		label = dev.UnitID
	}
	if label == "" {
		label = addr.String()
	}
	if label != "" {
		metadata["name"] = label
	}
	if dev.Type != "" {
		metadata["device_type"] = dev.Type
	}

	// v0.9.0: payload.type 제거. metadata.message_type ("device_state.<trigger>") 가
	// 노드 단에서 schema 식별 역할을 한다.
	d := map[string]any{
		"unit_id":   effectiveDeviceID(addr, dev.UnitID),
		"device_id": agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, effectiveDeviceID(addr, dev.UnitID)),
		"trigger":   trigger,
		"state":     state,
	}
	if !dev.LastSeen.IsZero() {
		d["last_seen_ms"] = dev.LastSeen.UnixMilli()
	}
	if len(metadata) > 0 {
		d["metadata"] = metadata
	}

	b, err := json.Marshal(d)
	if err != nil {
		return
	}

	a.recentMu.Lock()
	a.recentSeq++
	entry := recentStateEntry{Seq: a.recentSeq, Data: b}
	if len(a.recentSnapshots) >= recentSnapshotsCapacity {
		a.recentSnapshots = a.recentSnapshots[1:]
	}
	a.recentSnapshots = append(a.recentSnapshots, entry)
	a.recentMu.Unlock()
}

// processGetRecentStates 는 last_seq 이후의 스냅샷을 반환한다 (LGCP get_recent 패턴).
// 요청: {"command":"get_recent_states","params":{"last_seq":N,"count":M}}
// 응답: {"count":N,"snapshots":[{"seq":1,"devices":[...]},...]}
func (a *Hvacr01Agent) processGetRecentStates(req *processRequest) ([]byte, error) {
	var lastSeq int64
	var count int
	if v, ok := req.Params["last_seq"]; ok {
		switch n := v.(type) {
		case float64:
			lastSeq = int64(n)
		case int64:
			lastSeq = n
		}
	}
	if v, ok := req.Params["count"]; ok {
		switch n := v.(type) {
		case float64:
			count = int(n)
		case int:
			count = n
		}
	}
	if count <= 0 {
		count = 32
	}

	a.recentMu.Lock()
	// lastSeq 이후의 엔트리만 필터링
	var entries []recentStateEntry
	for _, e := range a.recentSnapshots {
		if e.Seq > lastSeq {
			entries = append(entries, e)
			if len(entries) >= count {
				break
			}
		}
	}
	a.recentMu.Unlock()

	// JSON 응답 구성 (각 스냅샷은 단일 디바이스)
	snapshots := make([]json.RawMessage, len(entries))
	for i, e := range entries {
		snap, _ := json.Marshal(map[string]any{
			"seq":    e.Seq,
			"device": json.RawMessage(e.Data),
		})
		snapshots[i] = snap
	}

	// 실제 스냅샷이 있을 때만 내부 통계 기록 (빈 폴링은 카운트하지 않음)
	// 1회 요청 → 1회 응답이므로 수신/송신 각 1건 카운트 (스냅샷 수와 무관)
	if len(snapshots) > 0 {
		a.stats.IncrInternalMessagesReceived()
		a.stats.IncrInternalMessagesSent()
		if req.NodeID != "" {
			a.stats.IncrNodeRefReceived(req.NodeID, req.FlowID)
			a.stats.IncrNodeRefSent(req.NodeID, req.FlowID)
		}
	}

	return json.Marshal(map[string]any{
		"count":     len(snapshots),
		"snapshots": snapshots,
	})
}

// ---------------------------------------------------------------------------
// 디바이스 관리 명령 처리
// ---------------------------------------------------------------------------

// processAddDevice 는 디바이스 추가 명령을 처리한다.
func (a *Hvacr01Agent) processAddDevice(req *processRequest) ([]byte, error) {
	// API exec DTO에서는 params 내에 전달 — 폴백 처리
	address := req.Address
	if address == "" {
		if v, ok := req.Params["address"].(string); ok {
			address = v
		}
	}
	deviceID := req.DeviceID
	if deviceID == "" {
		if v, ok := req.Params["device_id"].(string); ok {
			deviceID = v
		}
	}
	devType := req.DeviceType
	if devType == "" {
		if v, ok := req.Params["device_type"].(string); ok {
			devType = v
		}
	}

	// name 파라미터 추출 (사용자 정의 디바이스 이름)
	name := ""
	if v, ok := req.Params["name"].(string); ok {
		name = v
	}

	if address == "" {
		return nil, fmt.Errorf("samsung_hvacr01: address is required for add_device")
	}
	addr, err := ParseNasaAddress(address)
	if err != nil {
		return nil, fmt.Errorf("samsung_hvacr01: invalid address: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.devices[addr]; exists {
		return nil, ErrDeviceAlreadyRegistered
	}

	// device_id 중복 체크
	if deviceID != "" {
		if _, exists := a.deviceIDs[deviceID]; exists {
			return nil, ErrDuplicateDeviceID
		}
	}

	if devType == "" {
		devType = DetectDeviceType(addr)
	}

	dev := &NasaDevice{
		Address: addr,
		UnitID:  deviceID,
		Name:    name,
		Type:    devType,
		Online:  false,
		Source:  "bridge",
	}
	if devType == "HVACR.IDU" {
		dev.State = &NasaDeviceState{RawMessageSets: make(map[uint16][]byte)}
	}

	a.devices[addr] = dev
	if deviceID != "" {
		a.deviceIDs[deviceID] = addr
	}

	// 이벤트 전송 (락 밖에서 하면 좋지만 non-blocking 이므로 무방)
	regData := map[string]any{
		"address":     addr.String(),
		"unit_id":     addr.String(),
		"device_id":   agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, addr.String()),
		"name":        name,
		"device_type": devType,
	}
	if dev.State != nil {
		regData["state"] = dev.State.StateForJSON(false)
	}
	a.sendEventLocked("device_registered", regData)

	// 디바이스 추가 후 캐시 갱신
	if b, err := a.buildAllStatesJSON(); err == nil {
		a.cachedAllStates.Store(b)
	}

	resp := map[string]any{
		"status":      "ok",
		"address":     addr.String(),
		"unit_id":     deviceID,
		"device_id":   agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, deviceID),
		"name":        name,
		"device_type": devType,
	}
	return json.Marshal(resp)
}

// processRemoveDevice 는 디바이스 제거 명령을 처리한다.
func (a *Hvacr01Agent) processRemoveDevice(req *processRequest) ([]byte, error) {
	// API exec DTO에서는 params 내에 전달 — 폴백 처리
	if req.Address == "" {
		if v, ok := req.Params["address"].(string); ok {
			req.Address = v
		}
	}
	if req.DeviceID == "" {
		if v, ok := req.Params["device_id"].(string); ok {
			req.DeviceID = v
		}
	}

	addr, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}

	// config 소스 디바이스도 UI 에서 삭제 가능하게 한다(보호 제거). 수동 추가 후
	// 재시작으로 "config" 로 굳은 디바이스를 사용자가 직접 삭제할 수 있어야 하기 때문이다.
	// yaml 파일에 선언된 디바이스는 삭제해도 재시작 시 yaml 에서 다시 로드된다.

	a.mu.Lock()
	defer a.mu.Unlock()

	// deviceIDs 맵에서도 제거
	if dev.UnitID != "" {
		delete(a.deviceIDs, dev.UnitID)
	}
	delete(a.devices, addr)

	unregData := map[string]any{
		"address":   addr.String(),
		"unit_id":   addr.String(),
		"device_id": agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, addr.String()),
	}
	if dev.State != nil {
		unregData["state"] = dev.State.StateForJSON(false)
	}
	a.sendEventLocked("device_unregistered", unregData)

	// 디바이스 제거 후 캐시 갱신
	if b, err := a.buildAllStatesJSON(); err == nil {
		a.cachedAllStates.Store(b)
	}

	resp := map[string]any{
		"status":    "ok",
		"address":   addr.String(),
		"unit_id":   dev.UnitID,
		"device_id": agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, dev.UnitID),
	}
	return json.Marshal(resp)
}

// processListDevices 는 디바이스 목록 조회 명령을 처리한다.
func (a *Hvacr01Agent) processListDevices() ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var devices []map[string]any
	for addr, dev := range a.devices {
		devices = append(devices, map[string]any{
			"address":     addr.String(),
			"unit_id":     addr.String(),
			"device_id":   agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, addr.String()),
			"device_type": dev.Type,
			"online":      dev.Online,
			"source":      dev.Source,
		})
	}

	return json.Marshal(map[string]any{
		"status":  "ok",
		"devices": devices,
	})
}

// ---------------------------------------------------------------------------
// 헬퍼 메서드
// ---------------------------------------------------------------------------

// resolveDevice 는 요청에서 디바이스 주소와 포인터를 해석한다.
// device_id 가 우선이며, 없으면 address 를 사용한다.
func (a *Hvacr01Agent) resolveDevice(req *processRequest) (NasaAddress, *NasaDevice, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var addr NasaAddress

	if req.DeviceID != "" {
		resolved, ok := a.deviceIDs[req.DeviceID]
		if !ok {
			// UUID 폴백: 전역 device.List / REST 응답은 UUID(1급 식별자) 만 노출하고
			// bus address 를 노출하지 않으므로, 삭제/실행 경로가 device_id 로 UUID 를
			// 전달한다. deviceIDs 는 UnitID 로만 키잉되므로, UUID 는 각 디바이스의
			// emit-경로 UUID (ResolveDeviceID(name, addr.String())) 와 대조해 역매칭한다.
			if byUUID, ok2 := a.addrByDeviceUUID(req.DeviceID); ok2 {
				resolved, ok = byUUID, true
			}
		}
		if !ok {
			return NasaAddress{}, nil, ErrDeviceIDNotFound
		}
		addr = resolved
	} else if req.Address != "" {
		var err error
		addr, err = ParseNasaAddress(req.Address)
		if err != nil {
			return NasaAddress{}, nil, fmt.Errorf("samsung_hvacr01: invalid address: %w", err)
		}
	} else {
		return NasaAddress{}, nil, fmt.Errorf("samsung_hvacr01: address or device_id is required")
	}

	dev, ok := a.devices[addr]
	if !ok {
		return NasaAddress{}, nil, ErrDeviceNotFound
	}

	return addr, dev, nil
}

// addrByDeviceUUID 는 글로벌 UUID(device_id) 를 각 디바이스의 emit-경로 UUID
// (ResolveDeviceID(name, addr.String())) 와 대조해 해당 주소를 역매칭한다.
// 매칭 실패 시 (zero, false).
//
// 주의(RWMutex 비재진입): 호출자(resolveDevice) 가 a.mu 를 보유한 상태에서 호출하므로
// 여기서 a.mu 를 재-lock 하지 않으며 a.Name() 도 호출하지 않는다(재진입 deadlock 회피).
// ResolveDeviceID 는 a.mu 와 무관한 별도 저장소를 사용하므로 재진입 위험이 없다.
func (a *Hvacr01Agent) addrByDeviceUUID(uuid string) (NasaAddress, bool) {
	for addr := range a.devices {
		if agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, addr.String()) == uuid {
			return addr, true
		}
	}
	return NasaAddress{}, false
}

// GetPersistableDevices 는 현재 메모리의 디바이스 중 영속 저장할 대상 디바이스만 반환한다.
// 자동 발견("auto") 디바이스는 제외한다 — 재시작 시 다시 발견되므로 설정에 쌓을 필요가 없다.
// 반환 형식은 ParseDevices() 와 round-trip 되도록 DeviceEntry 로 맞춘다.
// 중요: Name 필드는 항상 dev.UnitID 를 사용한다. ParseDevices() 시 entry.Name → UnitID 로
// 매핑되므로, 역으로 저장할 때는 UnitID → Name 으로 써야 device_id(UUID) 안정성이 보존된다.
func (a *Hvacr01Agent) GetPersistableDevices() []agent.DeviceEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var result []agent.DeviceEntry
	for addr, dev := range a.devices {
		if dev.Source == "auto" {
			continue
		}
		result = append(result, agent.DeviceEntry{
			Address: addr.String(),
			Name:    dev.UnitID,
			Source:  dev.Source, // source 보존: 재시작 후에도 "bridge" 유지 → 삭제 가능
		})
	}
	return result
}

// sendControlCommand 는 제어 프레임을 빌드하고 트랜스포트로 전송한다.
// 부저 메시지셋(0x4050)을 자동 추가한다 (On=0x00, Off=0x01).
func (a *Hvacr01Agent) sendControlCommand(addr NasaAddress, sets []NasaMessageSet) error {
	// 부저: 에어컨 기본 동작이 부저 울림이므로 항상 명시적으로 설정한다.
	// BuzzerOnControl=true → 0x00(On), false → 0x01(Off, 억제)
	hasBuzzer := false
	for _, s := range sets {
		if s.Index == MsgBuzzer {
			hasBuzzer = true
			break
		}
	}
	if !hasBuzzer {
		buzzerVal := byte(0x01) // Off (억제)
		if a.hvacr01Config.BuzzerOnControl {
			buzzerVal = 0x00 // On
		}
		sets = append(sets, NasaMessageSet{Index: MsgBuzzer, Value: []byte{buzzerVal}})
	}

	// 제어 명령 전송 로그
	setNames := make([]string, 0, len(sets))
	for _, s := range sets {
		setNames = append(setNames, fmt.Sprintf("0x%04X(%d bytes)", s.Index, len(s.Value)))
	}
	a.logger.Debug("samsung_hvacr01: 제어 명령 전송", "addr", addr.String(), "sets", setNames)

	seq := a.nextSeqNum()
	frame, err := a.protocol.BuildControlCommand(addr, seq, sets)
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		a.logger.Error("samsung_hvacr01: 제어 프레임 빌드 실패", "addr", addr.String(), "error", err)
		return fmt.Errorf("samsung_hvacr01: build control command failed: %w", err)
	}

	if err := a.sendFrame(addr.String(), frame); err != nil {
		a.stats.IncrExternalMessagesErrored()
		a.logger.Error("samsung_hvacr01: 제어 명령 전송 실패", "addr", addr.String(), "error", err)
		return fmt.Errorf("samsung_hvacr01: send failed: %w", err)
	}

	a.stats.IncrExternalMessagesSent()
	a.stats.AddBytesWritten(int64(len(frame)))
	a.stats.UpdateLastActivity()
	a.logger.Debug("samsung_hvacr01: 제어 명령 전송 완료", "addr", addr.String(), "seq", seq, "frame_size", len(frame))
	return nil
}

// sendImmediateStatusQuery 는 제어 명령 직후 해당 디바이스의 상태를 반복 조회한다.
// 설정된 간격(StatusQueryDelay)과 횟수(StatusQueryRetries)에 따라 상태를 조회한다.
// 새 제어 명령이 발행되면 이전 goroutine 을 취소하여 시리얼 포트 경합을 방지한다.
func (a *Hvacr01Agent) sendImmediateStatusQuery(addr NasaAddress) {
	// 이전 상태 조회 goroutine 취소
	a.mu.Lock()
	if a.statusQueryCancel != nil {
		a.statusQueryCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.statusQueryCancel = cancel
	a.mu.Unlock()

	delay := a.hvacr01Config.StatusQueryDelay
	retries := a.hvacr01Config.StatusQueryRetries

	go func() {
		defer cancel()
		for i := 0; i < retries; i++ {
			select {
			case <-ctx.Done():
				a.logger.Debug("samsung_hvacr01: 상태 조회 취소 (새 제어 명령)", "addr", addr.String(), "attempt", i+1)
				return
			case <-time.After(delay):
			}
			seq := a.nextSeqNum()
			frame, err := a.protocol.BuildStatusQuery(addr, seq)
			if err != nil {
				a.logger.Debug("samsung_hvacr01: 즉시 상태 조회 빌드 실패", "addr", addr.String(), "error", err)
				return
			}
			if err := a.sendFrame(addr.String(), frame); err != nil {
				a.logger.Debug("samsung_hvacr01: 즉시 상태 조회 전송 실패", "addr", addr.String(), "error", err)
				return
			}
			a.stats.IncrExternalMessagesSent()
			a.logger.Debug("samsung_hvacr01: 제어 후 상태 조회 전송", "addr", addr.String(), "attempt", i+1, "seq", seq, "delay", delay)
		}
	}()
}

// effectiveDeviceID 는 emit/state/telemetry 메시지의 unit_id 로 사용할 localID 를 결정한다.
// SPEC-DEVICE-IDENTITY-001 M1/M2: adapter (및 V2 callback) 의 localID 와 일관되도록
// 항상 주소의 점으로 구분된 16진수 표현(NasaAddress.String())을 반환한다.
// 이는 device registry/list 와 emit side 의 UUID 일관성을 보장한다.
// deviceID 파라미터는 사용되지 않으며(legacy), 항상 addr.String() 을 반환한다.
func effectiveDeviceID(addr NasaAddress, deviceID string) string {
	return addr.String()
}

// buildSuccessResponse 는 제어 명령 성공 응답 JSON 을 생성한다.
func (a *Hvacr01Agent) buildSuccessResponse(addr NasaAddress, deviceID string, result map[string]any) ([]byte, error) {
	// v0.18.6: unit_id (프로토콜 식별자) + device_id (UUID).
	unitID := effectiveDeviceID(addr, deviceID)
	resp := map[string]any{
		"status":    "ok",
		"address":   addr.String(),
		"unit_id":   unitID,
		"device_id": agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, unitID),
		"result":    result,
	}
	// 제어 명령 응답: 내부 수신 1 + 내부 송신 1
	a.stats.IncrInternalMessagesReceived()
	a.stats.IncrInternalMessagesSent()
	return json.Marshal(resp)
}

// nextSeqNum 은 시퀀스 번호를 증가시키고 반환한다.
func (a *Hvacr01Agent) nextSeqNum() byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	current := a.seqNum
	if a.seqNum == 0xFF {
		a.seqNum = 0x00
	} else {
		a.seqNum++
	}
	return current
}

// sendEvent 는 이벤트를 msgCh 로 비동기 전송한다 (락 없이 호출).
func (a *Hvacr01Agent) sendEvent(eventType string, data map[string]any) {
	evt := map[string]any{"type": eventType}
	for k, v := range data {
		evt[k] = v
	}
	b, err := json.Marshal(evt)
	if err != nil {
		a.logger.Warn("samsung_hvacr01: event marshal failed", "error", err)
		return
	}
	select {
	case a.msgCh <- b:
		a.logger.Debug("samsung_hvacr01: 이벤트 msgCh 전송 성공",
			"type", eventType,
			"chLen", len(a.msgCh),
			"chCap", cap(a.msgCh),
		)
	default:
		// 2026-05-29: log_drops 옵션으로 통일 (Century / LG 통일). 기본 false 면 DEBUG.
		if a.hvacr01Config.LogDrops {
			a.logger.Warn("samsung_hvacr01: msgCh full, dropping event", "type", eventType)
		} else {
			a.logger.Debug("samsung_hvacr01: msgCh full, dropping event", "type", eventType)
		}
	}
}

// markOfflineLocked 는 online 디바이스를 오프라인으로 전환하고 관련 이벤트를 방출한다.
// 호출 전제: a.mu 보유. incrementErrorCount(ErrorCount threshold 도달)와 checkStaleDevices
// (LastSeen 경과), setAllDevicesOffline(트랜스포트 끊김)이 이 단일 helper 를 공유하여
// online→offline 전이 동작이 절대 어긋나지 않도록 한다.
//
// reason 은 진단용 사유 문자열이다("error_threshold" / "stale" / "transport_disconnected").
// 기존 device_offline 이벤트 페이로드는 그대로 보존하고 reason 은 로그에만 남긴다.
//
// 주의(N1/§7.1): RWMutex 비재진입 트랩을 피하기 위해 a.Name()/a.ID() 를 절대 호출하지
// 않고 a.agentConfig.Name 을 직접 읽는다.
func (a *Hvacr01Agent) markOfflineLocked(addr NasaAddress, dev *NasaDevice, reason string) {
	if !dev.Online {
		return
	}
	dev.Online = false
	a.sendEventLocked("device_offline", map[string]any{
		"address":   addr.String(),
		"unit_id":   addr.String(),
		"device_id": agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, addr.String()),
	})
	// SPEC-HVACR-CONNSTATE-001 E2: online→offline 전이 즉시 change 방출(tick 독립).
	// initial 미방출 device 는 순서 보장(E9)을 위해 skip.
	if dev.connInitialEmitted {
		a.emitConnectionLocked(addr, dev, connTriggerChange, time.Now().UnixMilli())
	}

	// 오프라인 전환도 device_state 스냅샷으로 전송한다(online→offline 대칭). online 전환은
	// pushRecentSnapshot 하지만 offline 은 connection change 만 방출해, report_interval 이
	// 0(주기 report 비활성)이면 device_state 스트림에 오프라인(online=false)이 영영 실리지
	// 않던 문제 수정. 호출측이 a.mu 를 보유하므로 여기서 push + notify 한다.
	a.pushRecentSnapshotWithTrigger(addr, "change")
	select {
	case a.stateNotify <- struct{}{}:
	default:
	}

	a.logger.Warn("samsung_hvacr01: 디바이스 오프라인",
		"address", addr.String(),
		"device_id", dev.UnitID,
		"error_count", dev.ErrorCount,
		"reason", reason,
	)
}

// incrementErrorCount 는 디바이스의 에러 카운트를 증가시키고,
// OfflineThreshold 에 도달하면 디바이스를 오프라인으로 전환한다.
func (a *Hvacr01Agent) incrementErrorCount(addr NasaAddress) {
	a.mu.Lock()
	defer a.mu.Unlock()

	dev, ok := a.devices[addr]
	if !ok {
		return
	}

	dev.ErrorCount++
	if dev.ErrorCount >= a.hvacr01Config.OfflineThreshold && dev.Online {
		a.markOfflineLocked(addr, dev, "error_threshold")
	}
}

// staleOfflineThreshold 은 LastSeen 기반 offline 판정 임계값을 계산한다 (offline_timeout 3-way).
//   - OfflineTimeout > 0: 그 값을 verbatim 사용 (명시적 임계값이 파생값을 이긴다).
//   - OfflineTimeout == 0: 비활성. 방어적으로 0 을 반환하나, 호출측(checkStaleDevices)이
//     disabled 를 먼저 early-return 으로 처리하므로 이 경로는 실사용되지 않는다.
//   - OfflineTimeout < 0 (sentinel: 미설정): OfflineThreshold × PollInterval 파생.
//     PollInterval<=0 이면 30s, OfflineThreshold<=0 이면 3 으로 방어 대체 (기본 3×30s=90s).
func (a *Hvacr01Agent) staleOfflineThreshold() time.Duration {
	ot := a.hvacr01Config.OfflineTimeout
	switch {
	case ot > 0:
		return ot
	case ot == 0:
		return 0
	default:
		poll := a.hvacr01Config.PollInterval
		if poll <= 0 {
			poll = 30 * time.Second
		}
		n := a.hvacr01Config.OfflineThreshold
		if n <= 0 {
			n = 3
		}
		return time.Duration(n) * poll
	}
}

// checkStaleDevices 는 pollLoop ticker 마다 호출되어, LastSeen 이 staleOfflineThreshold
// 를 초과해 갱신되지 않은 online 디바이스를 오프라인으로 전환한다.
//
// Samsung 폴링은 비동기(요청/응답 미매칭)이므로, 이 LastSeen 경과 검사가 실내기 정전/
// 응답없음을 감지하는 유일한 경로이다. status_query_enabled=false(passive sniff) 모드에서도
// 동작한다 — pollLoop 가 이 함수를 능동 쿼리 송신 skip(continue) 이전에 호출하기 때문이다.
//
// never-seen 규칙: LastSeen 이 zero 인 디바이스는 stale 판정하지 않는다. online 디바이스는
// handleMessage 가 LastSeen 을 반드시 갱신했으므로 online && zero 조합은 발생하지 않지만
// 방어적으로 skip 한다. 최초 통신 전 디바이스는 이미 Online=false(startup probe 또는 초기
// 상태)이므로 아래 online 게이트에서 걸러진다.
//
// ErrorCount 는 건드리지 않는다: ErrorCount 는 send/build 연속 실패용 threshold 카운터로,
// LastSeen 경과와는 독립된 감지 축이다. 두 메커니즘을 뒤섞지 않도록 stale 경로에서 리셋하지
// 않으며, 복구는 handleMessage 가 ErrorCount=0 으로 초기화하므로 영향받지 않는다.
func (a *Hvacr01Agent) checkStaleDevices() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// offline_timeout == 0 (명시적) → staleness 감지 비활성 (3-way 규칙).
	// transport-disconnect bulk offline(setAllDevicesOffline)은 이와 무관하게 동작한다.
	if a.hvacr01Config.OfflineTimeout == 0 {
		return
	}

	// Stop 이후에는 방출 금지 (emitConnectionReports 선례, N10).
	select {
	case <-a.stopCh:
		return
	default:
	}

	threshold := a.staleOfflineThreshold()
	now := time.Now()
	for addr, dev := range a.devices {
		if !dev.Online {
			continue
		}
		if dev.LastSeen.IsZero() {
			continue
		}
		if now.Sub(dev.LastSeen) > threshold {
			a.markOfflineLocked(addr, dev, "stale")
		}
	}
}

// probeSilentDevices 는 마지막 수신 후 PollInterval 이상 조용한 online 디바이스에
// 상태 쿼리(확인 probe)를 보내 응답을 유도한다.
//
// 설계(사용자): "상태 확인"과 "offline 판정"은 별개 축이다. passive 모드
// (status_query_enabled=false)는 블랭킷 폴을 하지 않으므로, 디바이스가 자체
// 브로드캐스트 주기(> offline 임계값)로만 통신하면 정상 online 인데도 주기적으로
// offline 오탐되어 online/offline 플래핑이 발생한다. 이를 막기 위해 침묵한 online
// 디바이스에 targeted 쿼리를 보내 응답을 유도한다:
//   - online 이면 응답 → handleMessage 가 LastSeen 갱신 → online 유지
//   - 실제 offline 이면 무응답 → checkStaleDevices 가 90초(임계값)에 offline
//
// PollInterval 간격(pollLoop tick)마다 재-probe 되어 "설정 간격으로 재요청"을 구현한다.
// offline 감지가 비활성(offline_timeout==0)이면 probe 도 불필요하므로 skip 한다.
// 주의(nextSeqNum 은 a.mu.Lock): 락을 잡지 않은 상태에서 호출한다.
func (a *Hvacr01Agent) probeSilentDevices() {
	if a.hvacr01Config.OfflineTimeout == 0 || !a.transport.Available() {
		return
	}

	a.mu.RLock()
	probeAfter := a.hvacr01Config.PollInterval
	if probeAfter <= 0 {
		probeAfter = 30 * time.Second
	}
	now := time.Now()
	var toProbe []NasaAddress
	for addr, dev := range a.devices {
		if !dev.Online || dev.LastSeen.IsZero() {
			continue
		}
		if now.Sub(dev.LastSeen) >= probeAfter {
			toProbe = append(toProbe, addr)
		}
	}
	a.mu.RUnlock()

	for _, addr := range toProbe {
		seq := a.nextSeqNum()
		frame, err := a.protocol.BuildStatusQuery(addr, seq)
		if err != nil {
			continue
		}
		if err := a.sendFrame(addr.String(), frame); err != nil {
			continue
		}
		a.stats.IncrExternalMessagesSent()
		a.logger.Debug("samsung_hvacr01: 침묵 디바이스 확인 probe 전송", "addr", addr.String(), "seq", seq)
	}
}

// setAllDevicesOffline 은 모든 online 디바이스를 즉시 오프라인으로 전환한다.
// 트랜스포트 연결이 끊어졌을 때(reconnectLoop 진입 시) 호출된다 — LGAP setAllDevicesOffline
// 대칭. device 당 개별 change 를 방출하며(배칭 금지), 두 번째 initial 을 방출하지 않고(N9),
// initial 미방출 device 는 순서 보장(E9)을 위해 skip 한다(markOfflineLocked 게이트).
//
// 의도된 LGAP 비대칭(사용자 승인): Samsung 은 markOfflineLocked 를 재사용하므로 device 당
// legacy device_offline 이벤트도 함께 방출한다. LGAP 의 inline setAllDevicesOffline 은
// device_offline 을 방출하지 않는다. 이 차이는 additive/더 정확한 것으로 수용되었으며,
// 단일 offline 전이 helper 공유(drift 방지)를 위한 의도적 선택이다. LGAP 은 수정하지 않는다.
func (a *Hvacr01Agent) setAllDevicesOffline() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Stop 이후에는 방출 금지 (N10).
	select {
	case <-a.stopCh:
		return
	default:
	}

	var offlined int
	for addr, dev := range a.devices {
		if dev.Online {
			offlined++
			a.markOfflineLocked(addr, dev, "transport_disconnected")
		}
	}
	if offlined > 0 {
		a.logger.Info("samsung_hvacr01: 통신 끊김, 디바이스 오프라인 전환", "count", offlined)
	}
}

// sendEventLocked 는 sendEvent 와 동일하지만 이미 락이 잡혀 있을 때 사용한다.
func (a *Hvacr01Agent) sendEventLocked(eventType string, data map[string]any) {
	evt := map[string]any{"type": eventType}
	for k, v := range data {
		evt[k] = v
	}
	b, err := json.Marshal(evt)
	if err != nil {
		return
	}
	select {
	case a.msgCh <- b:
	default:
		// 드롭
	}
}

// ListDevices 는 등록된 디바이스 목록을 반환한다.
func (a *Hvacr01Agent) ListDevices() []NasaDevice {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]NasaDevice, 0, len(a.devices))
	for _, dev := range a.devices {
		result = append(result, *dev)
	}
	return result
}

// GetDeviceState 는 지정된 주소의 디바이스 상태를 반환한다.
func (a *Hvacr01Agent) GetDeviceState(addr NasaAddress) (*NasaDeviceState, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	dev, ok := a.devices[addr]
	if !ok {
		return nil, ErrDeviceNotFound
	}
	return dev.State, nil
}

// GetDeviceByID 는 device_id 로 디바이스를 조회한다.
func (a *Hvacr01Agent) GetDeviceByID(deviceID string) (*NasaDevice, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	addr, ok := a.deviceIDs[deviceID]
	if !ok {
		return nil, ErrDeviceIDNotFound
	}
	dev, ok := a.devices[addr]
	if !ok {
		return nil, ErrDeviceNotFound
	}
	return dev, nil
}

// ---------------------------------------------------------------------------
// 재연결 루프
// ---------------------------------------------------------------------------

// reconnectLoop 는 트랜스포트 재연결을 시도하는 고루틴이다.
// 지수 백오프를 적용하며, 첫 시도만 Warn, 이후는 Debug 로그.
func (a *Hvacr01Agent) reconnectLoop() {
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

	// 통신 끊김 → 모든 디바이스 즉시 오프라인 전환 (LGAP reconnectLoop 대칭).
	// receiveLoop 가 Available()==false 를 감지해 reconnectLoop 를 기동하는 유일한
	// disconnect 경로이므로, isReconnecting 게이트 직후 이 지점이 정확한 hook 이다.
	a.setAllDevicesOffline()

	// 재연결 시작 이벤트
	a.sendEvent("transport_reconnecting", map[string]any{
		"timestamp_ms": time.Now().UnixMilli(),
	})

	baseInterval := a.hvacr01Config.ReconnectInterval
	maxBackoff := a.hvacr01Config.MaxReconnectBackoff
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
			a.logger.Info("samsung_hvacr01: 트랜스포트 재연결 성공",
				"attempts", attempt+1,
				"downtime", time.Since(disconnectedAt).Round(time.Second).String(),
			)
			a.sendEvent("transport_reconnected", map[string]any{
				"attempt_count":    attempt + 1,
				"downtime_seconds": int(time.Since(disconnectedAt).Seconds()),
				"timestamp_ms":     time.Now().UnixMilli(),
			})

			// 새 disconnectCh 생성 후 수신/폴링 루프 재시작
			a.mu.Lock()
			a.disconnectCh = make(chan struct{})
			a.mu.Unlock()

			a.wg.Add(2)
			go func() { defer a.wg.Done(); a.pollLoop() }()
			go func() { defer a.wg.Done(); a.receiveLoop() }()
			return
		}

		// 재연결 실패
		a.reconnectMu.Lock()
		a.reconnectAttempts = attempt + 1
		a.reconnectMu.Unlock()

		if attempt == 0 {
			a.logger.Warn("samsung_hvacr01: 트랜스포트 재연결 시도 중", "error", err)
		} else {
			a.logger.Debug("samsung_hvacr01: 트랜스포트 재연결 시도", "attempt", attempt+1, "error", err)
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
// 폴링 / 수신 루프
// ---------------------------------------------------------------------------

// pollLoop 는 주기적으로 디바이스 상태를 쿼리한다.
func (a *Hvacr01Agent) pollLoop() {
	ticker := time.NewTicker(a.hvacr01Config.PollInterval)
	a.mu.Lock()
	a.pollTicker = ticker
	a.mu.Unlock()

	defer ticker.Stop()

	a.mu.RLock()
	disconnectCh := a.disconnectCh
	a.mu.RUnlock()

	for {
		select {
		case <-a.stopCh:
			return
		case <-disconnectCh:
			a.logger.Debug("samsung_hvacr01: pollLoop 연결 끊김으로 종료")
			return
		case <-ticker.C:
			a.mu.RLock()
			paused := a.paused
			// 디바이스 주소 스냅샷 복사
			addrs := make([]NasaAddress, 0, len(a.devices))
			if !paused {
				for addr := range a.devices {
					addrs = append(addrs, addr)
				}
			}
			a.mu.RUnlock()
			if paused {
				continue
			}

			// LastSeen 경과 기반 stale offline 검사. StatusQueryEnabled 여부와 무관하게
			// 매 tick 실행되어야 하므로 아래 passive-mode continue 이전에 호출한다.
			// Samsung 비동기 폴링에서 실내기 정전/응답없음을 감지하는 유일한 경로.
			a.checkStaleDevices()

			// v0.6.1: status_query_enabled=false 면 능동적 상태 쿼리 송신 skip
			// (passive sniff only). ticker 는 계속 동작하나 query 만 안 보냄 —
			// 다른 ticker 기반 housekeeping 작업이 향후 추가될 여지를 남긴다.
			if !a.hvacr01Config.StatusQueryEnabled {
				// passive: 능동적 블랭킷 폴은 skip 하되, 침묵한 online 디바이스에 한해
				// 연결 확인 probe 를 전송한다(설계: "상태 확인"은 offline 판정과 별개 축).
				// 자체 브로드캐스트 주기가 offline 임계값보다 긴 디바이스의 플래핑 방지.
				a.probeSilentDevices()
				a.logger.Debug("samsung_hvacr01: 블랭킷 폴 skip (passive), 침묵 디바이스 probe 수행", "devices", len(addrs))
				continue
			}

			a.logger.Debug("samsung_hvacr01: 폴링 시작", "devices", len(addrs))
			for _, addr := range addrs {
				seq := a.nextSeqNum()
				frame, err := a.protocol.BuildStatusQuery(addr, seq)
				if err != nil {
					a.logger.Warn("samsung_hvacr01: build status query failed", "addr", addr.String(), "error", err)
					a.incrementErrorCount(addr)
					continue
				}
				if err := a.sendFrame(addr.String(), frame); err != nil {
					a.logger.Warn("samsung_hvacr01: send status query failed", "addr", addr.String(), "error", err)
					a.incrementErrorCount(addr)
					continue
				}
				a.logger.Debug("samsung_hvacr01: 상태 쿼리 전송", "addr", addr.String(), "seq", seq)
				a.stats.IncrExternalMessagesSent()
			}
		}
	}
}

// sendFrame 은 프레임을 트랜스포트로 전송한다. log_messages 옵션이 켜져 있으면
// 송신(TX) 프레임을 hex 로 INFO 로그한다(디바이스 송/수신 진단용). 반환 error 는
// 호출측이 기존과 동일하게 처리한다.
func (a *Hvacr01Agent) sendFrame(addr string, frame []byte) error {
	if a.hvacr01Config.LogMessages {
		a.logger.Info("samsung_hvacr01: TX", "addr", addr, "len", len(frame), "hex", hex.EncodeToString(frame))
	}
	return a.transport.Send(frame)
}

// isReadTimeout 은 err 가 읽기 데드라인 초과(i/o timeout, 즉 "수신 메시지 없음")
// 인지 판별한다. TCP read deadline 만료는 net.Error.Timeout()==true 이거나
// os.ErrDeadlineExceeded 로 나타난다. 정상 상황이므로 로그에서 제외하는 데 쓴다.
func isReadTimeout(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	return (errors.As(err, &netErr) && netErr.Timeout()) || errors.Is(err, os.ErrDeadlineExceeded)
}

// receiveLoop 는 트랜스포트에서 데이터를 수신하고 디바이스 상태를 업데이트한다.
func (a *Hvacr01Agent) receiveLoop() {
	buf := make([]byte, 1024)
	scanner := newFrameScanner()

	for {
		select {
		case <-a.stopCh:
			return
		default:
		}

		n, err := a.transport.Receive(buf)
		if err != nil {
			// stopCh 가 닫혀 있으면 정상 종료
			select {
			case <-a.stopCh:
				return
			default:
			}

			// 연결 끊김 판별: Available() == false 이면 재연결 루프 시작
			if !a.transport.Available() {
				a.logger.Warn("samsung_hvacr01: 트랜스포트 연결 끊김 감지", "error", err)
				a.sendEvent("transport_disconnected", map[string]any{
					"reason":       err.Error(),
					"timestamp_ms": time.Now().UnixMilli(),
				})
				// pollLoop 에 연결 끊김 시그널
				a.mu.RLock()
				ch := a.disconnectCh
				a.mu.RUnlock()
				select {
				case <-ch:
				default:
					close(ch)
				}
				// 재연결 루프 시작
				a.wg.Add(1)
				go func() { defer a.wg.Done(); a.reconnectLoop() }()
				return
			}

			// 읽기 데드라인 초과(i/o timeout)는 "수신 메시지 없음" 의 정상 상황이므로
			// 로그하지 않는다(주기적 timeout 이 로그를 도배하는 문제). 연결은 살아 있고
			// 다음 read 를 계속 시도한다. 그 외 일시적 에러만 Debug 로 남긴다.
			if isReadTimeout(err) {
				continue
			}
			a.logger.Debug("samsung_hvacr01: receive error (transient)", "error", err)
			continue
		}

		if n == 0 {
			continue
		}

		// 수신 바이트를 프레임 스캐너 버퍼에 축적
		scanner.Write(buf[:n])

		// 버퍼에서 완전한 프레임을 모두 추출하여 처리
		for {
			frame, ok := scanner.Next()
			if !ok {
				break
			}

			if a.hvacr01Config.LogMessages {
				a.logger.Info("samsung_hvacr01: RX", "len", len(frame), "hex", hex.EncodeToString(frame))
			}

			// a.logger.Debug("samsung_hvacr01: 프레임 추출 완료",
			// 	"frameBytes", len(frame),
			// )

			msg, err := a.protocol.Decode(frame)
			if err != nil {
				// unsupported 인덱스로 인한 디코드 에러는 설정에 따라 로그 억제
				if errors.Is(err, ErrInvalidMessageSetIndex) && len(a.hvacr01Config.UnsupportedMsgSets) > 0 {
					if !a.hvacr01Config.LogUnsupportedMsgSets {
						a.stats.IncrExternalMessagesErrored()
						continue
					}
				}
				// log_decode_errors 옵션이 false 면 일반 decode error 도 억제 (운영 환경 noise 방지).
				// 2026-05-14 hotfix: invalid message set index 등 빈번한 디코드 오류로 인한 로그 폭주 회피.
				if a.hvacr01Config.LogDecodeErrors {
					a.logger.Warn("samsung_hvacr01: decode error", "error", err)
				}
				a.stats.IncrExternalMessagesErrored()
				continue
			}

			// a.logger.Debug("samsung_hvacr01: 메시지 디코드 성공",
			// 	"source", msg.SourceAddr.String(),
			// 	"dest", msg.DestAddr.String(),
			// 	"sets", len(msg.MessageSets),
			// )

			a.stats.IncrExternalMessagesReceived()
			a.stats.AddBytesRead(int64(len(frame)))

			a.handleMessage(msg)
		}
	}
}

// handleMessage 는 수신된 메시지를 처리하여 디바이스 상태를 업데이트한다.
func (a *Hvacr01Agent) handleMessage(msg *NasaMessage) {
	srcAddr := msg.SourceAddr

	// a.logger.Debug("samsung_hvacr01: handleMessage 진입",
	// 	"source", srcAddr.String(),
	// )

	a.mu.Lock()
	defer a.mu.Unlock()
	// v0.18.6: handleMessage 내부에서 a.Name() 을 호출하면 RWMutex 재진입 deadlock 발생.
	// agentConfig.Name 을 lock 이 잡힌 상태에서 직접 읽어 agentName 으로 사용.
	agentName := a.agentConfig.Name

	dev, ok := a.devices[srcAddr]
	if !ok {
		if a.hvacr01Config.AutoDiscovery {
			devType := DetectDeviceType(srcAddr)
			dev = &NasaDevice{
				Address:  srcAddr,
				Type:     devType,
				Online:   true,
				LastSeen: time.Now(),
				Source:   "auto",
			}
			if devType == "HVACR.IDU" {
				dev.State = &NasaDeviceState{RawMessageSets: make(map[uint16][]byte)}
			}
			a.devices[srcAddr] = dev
			evtData := map[string]any{
				"address":     srcAddr.String(),
				"unit_id":     srcAddr.String(),
				"device_id":   agent.ResolveDeviceID(context.Background(), agentName, srcAddr.String()),
				"device_type": devType,
			}
			if dev.State != nil {
				evtData["state"] = dev.State.StateForJSON(false)
			}
			a.sendEventLocked("device_discovered", evtData)
		} else {
			if !a.warnedUnknown[srcAddr] {
				a.warnedUnknown[srcAddr] = true
				a.logger.Warn("samsung_hvacr01: unknown device (이후 debug로 전환)", "address", srcAddr.String())
			} else {
				a.logger.Debug("samsung_hvacr01: unknown device", "address", srcAddr.String())
			}
			return
		}
	}

	wasOffline := !dev.Online
	dev.Online = true
	dev.LastSeen = time.Now()
	dev.ErrorCount = 0

	// snapshotShouldPush 는 recentSnapshots 에 push 할지 여부를 추적한다.
	// 불필요한 중복 emit 방지 — LastSeen 만 갱신되는 heartbeat frame 은 push 하지 않는다.
	// 사용자 보고: outdoor 디바이스 (state nil) 가 매 frame 마다 동일한 snapshot 을
	// emit 하던 결함 fix.
	snapshotShouldPush := false

	if wasOffline {
		onlineData := map[string]any{
			"address":   srcAddr.String(),
			"unit_id":   srcAddr.String(),
			"device_id": agent.ResolveDeviceID(context.Background(), agentName, srcAddr.String()),
		}
		if dev.State != nil {
			onlineData["state"] = dev.State.StateForJSON(false)
		}
		a.sendEventLocked("device_online", onlineData)
		snapshotShouldPush = true
	}

	// SPEC-HVACR-CONNSTATE-001 E3: 첫 통신 도착 시 device당 1회 initial(online)을,
	// 이후 offline→online 복구 시 change 를 방출한다. connInitialEmitted 게이트가
	// per-device 순서 보장(E9)과 프로세스당 1회 initial(N9)을 동시에 만족시킨다.
	// (RWMutex 비재진입: 이 경로는 a.mu 보유 중이므로 a.Name() 대신 agentName 사용.)
	if !dev.connInitialEmitted {
		dev.connInitialEmitted = true
		a.emitConnectionLocked(srcAddr, dev, connTriggerInitial, time.Now().UnixMilli())
	} else if wasOffline {
		a.emitConnectionLocked(srcAddr, dev, connTriggerChange, time.Now().UnixMilli())
	}

	// 실내기 상태 업데이트
	if dev.State != nil && len(msg.MessageSets) > 0 {
		sets := msg.MessageSets
		if len(a.hvacr01Config.UnsupportedMsgSets) > 0 {
			sets = a.filterMessageSets(sets, srcAddr)
		}
		if len(sets) == 0 {
			return
		}

		// 이전 상태 저장
		prevState := a.lastStates[srcAddr]

		dev.State.UpdateFromMessageSets(sets)

		// 2026-05-29: log_state_updates 진단용 — 디코드된 핵심 필드 + raw payload hex
		// INFO 로그 (Century logDecodedState 패턴). AllCoreObserved 게이트 이전에 출력해
		// 초기 관측 누락도 진단 가능.
		if a.hvacr01Config.LogStateUpdates {
			payloadHex := ""
			if len(msg.Raw) > 0 {
				payloadHex = hex.EncodeToString(msg.Raw)
			}
			a.logger.Info("samsung_hvacr01: state update",
				"address", srcAddr.String(),
				"unit_id", dev.UnitID,
				"power", dev.State.Power,
				"mode", dev.State.Mode,
				"target_temp", dev.State.TargetTemp,
				"current_temp", dev.State.CurrentTemp,
				"fan_speed", dev.State.FanSpeed,
				"sets_count", len(sets),
				"payload_hex", payloadHex,
			)
		}

		// 상태 변화 발생 시 현재 상태를 그대로 노드로 전송한다.
		// (이전에는 5 핵심 필드가 모두 관측될 때까지 emit 을 보류했으나, 핵심 필드를
		// 보내지 않는 디바이스는 영영 전송되지 않는 문제가 있어 게이트를 제거함.
		// 관측되지 않은 필드는 zero-value 로 노출된다.)

		// 변경 감지
		currentState := *dev.State
		if stateChanged(prevState, currentState) {
			// v0.6.7: event_temp_threshold gate — 비온도 필드 변경 없이 실내온도만
			// 변경된 경우 |Δ| < threshold 면 emit suppress.
			// lastStates 도 갱신하지 않아 다음 frame 에서 prev 와 다시 비교 (누적 감지).
			if a.hvacr01Config.EventTempThreshold > 0 &&
				!nonTempFieldsChangedHvacr01(prevState, currentState) {
				if maxTempDeltaHvacr01(prevState, currentState) < a.hvacr01Config.EventTempThreshold {
					return
				}
			}
			a.lastStates[srcAddr] = currentState
			a.logger.Debug("samsung_hvacr01: 상태 변경 감지",
				"device", dev.UnitID, "addr", srcAddr.String(),
				"power", currentState.Power, "mode", currentState.Mode,
				"target_temperature", currentState.TargetTemp, "fan_speed", currentState.FanSpeed)
			a.sendEventLocked("device_state_changed", map[string]any{
				"address":   srcAddr.String(),
				"unit_id":   srcAddr.String(),
				"device_id": agent.ResolveDeviceID(context.Background(), agentName, srcAddr.String()),
				"state":     (&currentState).StateForJSON(false),
			})
			snapshotShouldPush = true
			// WebSocket 브로드캐스트 콜백 (SPEC-DEVICE-IDENTITY-001 Phase D § M3 — V2 단일).
			// 주의: a.Name()은 a.mu.RLock()을 호출하므로 write lock 보유 중
			// 재진입 데드락을 피하려면 a.agentConfig.Name을 직접 참조해야 한다.
			if v2 := a.onDeviceStateChangeV2; v2 != nil {
				globalID := fmt.Sprintf("%s:%s", agentName, srcAddr.String())
				deviceUID := agent.ResolveDeviceID(context.Background(), agentName, srcAddr.String())
				a.logger.Debug("samsung_hvacr01: WebSocket 상태 변경 브로드캐스트", "agent", agentName, "globalID", globalID, "device_uid", deviceUID)
				go v2(agentName, deviceUID, globalID)
			}
		}
	}

	a.stats.UpdateLastActivity()

	// processGetAllStates 용 atomic 캐시 갱신 (write lock 내에서 실행)
	if b, err := a.buildAllStatesJSON(); err == nil {
		a.cachedAllStates.Store(b)
	}

	// get_recent_states 용 스냅샷 링버퍼에 push (변경된 디바이스만).
	// snapshotShouldPush 가 true 일 때만 push — heartbeat 만 갱신되는 frame 은 skip.
	// 사용자 보고 "outdoor 매 frame 마다 중복 emit" root cause fix.
	if snapshotShouldPush {
		a.pushRecentSnapshot(srcAddr)
	}

	// 폴링 노드에 새 데이터 도착 알림 (non-blocking)
	select {
	case a.stateNotify <- struct{}{}:
	default:
	}
}

// filterMessageSets 는 unsupported 목록에 포함된 메시지 셋을 필터링한다.
func (a *Hvacr01Agent) filterMessageSets(sets []NasaMessageSet, addr NasaAddress) []NasaMessageSet {
	filtered := make([]NasaMessageSet, 0, len(sets))
	for _, ms := range sets {
		if a.hvacr01Config.UnsupportedMsgSets[ms.Index] {
			if a.hvacr01Config.LogUnsupportedMsgSets {
				a.logger.Debug("samsung_hvacr01: unsupported message set filtered",
					slog.String("address", addr.String()),
					slog.String("msg_index", fmt.Sprintf("0x%04X", ms.Index)),
				)
			}
			continue
		}
		filtered = append(filtered, ms)
	}
	return filtered
}

// nonTempFieldsChangedHvacr01 는 비온도 필드 (Power/Mode/TargetTemp/FanSpeed) 중
// 하나라도 변경되었는지 검사한다 (v0.6.7).
// TargetTemp 는 사용자 설정 값이라 비온도(제어) 카테고리. 호출 전제: stateChanged=true.
func nonTempFieldsChangedHvacr01(prev, current NasaDeviceState) bool {
	return prev.Power != current.Power ||
		prev.Mode != current.Mode ||
		prev.TargetTemp != current.TargetTemp ||
		prev.FanSpeed != current.FanSpeed
}

// maxTempDeltaHvacr01 는 온도 센서값 (CurrentTemp) 의 |Δ| 를 반환한다 (v0.6.7).
// NASA 는 NasaDeviceState 에 단일 실내온도만 보유 (outdoor 는 별도 device).
func maxTempDeltaHvacr01(prev, current NasaDeviceState) float64 {
	d := float64(current.CurrentTemp - prev.CurrentTemp)
	if d < 0 {
		d = -d
	}
	return d
}

// stateChanged 는 두 상태가 다른지 비교한다.
func stateChanged(prev, current NasaDeviceState) bool {
	if prev.Power != current.Power {
		return true
	}
	if prev.Mode != current.Mode {
		return true
	}
	if prev.TargetTemp != current.TargetTemp {
		return true
	}
	if prev.CurrentTemp != current.CurrentTemp {
		return true
	}
	if prev.FanSpeed != current.FanSpeed {
		return true
	}
	return false
}

// FrameNotifyCh 는 디바이스 상태 변경 시 알림 채널을 반환한다 (agent.FrameNotifier 구현).
func (a *Hvacr01Agent) FrameNotifyCh() <-chan struct{} {
	return a.stateNotify
}

// ---------------------------------------------------------------------------
// agent.Agent 인터페이스 메서드 (정보 조회)
// ---------------------------------------------------------------------------

// Configure 는 에이전트 설정을 업데이트한다.
func (a *Hvacr01Agent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("samsung_hvacr01 configure: %w", err)
	}

	// Transport.Options에서 hvacr01Config 재파싱
	if len(config.Transport.Options) > 0 {
		hvacr01Cfg, err := parseHvacr01Config(config.Transport.Options)
		if err != nil {
			return fmt.Errorf("samsung_hvacr01 configure: re-parse config: %w", err)
		}

		a.mu.Lock()
		oldPoll := a.hvacr01Config.PollInterval
		oldNotify := a.hvacr01Config.NotifyInterval
		a.hvacr01Config = hvacr01Cfg
		a.agentConfig = config

		// 실행 중인 ticker 재설정 (간격이 변경된 경우)
		if hvacr01Cfg.PollInterval != oldPoll && a.pollTicker != nil {
			a.pollTicker.Reset(hvacr01Cfg.PollInterval)
			a.logger.Info("poll interval 변경 적용", "old", oldPoll, "new", hvacr01Cfg.PollInterval)
		}
		if hvacr01Cfg.NotifyInterval != oldNotify && a.notifyTicker != nil {
			if hvacr01Cfg.NotifyInterval > 0 {
				a.notifyTicker.Reset(hvacr01Cfg.NotifyInterval)
				a.logger.Info("notify interval 변경 적용", "old", oldNotify, "new", hvacr01Cfg.NotifyInterval)
			} else {
				a.notifyTicker.Stop()
				a.logger.Info("notify interval 비활성화")
			}
		}
		a.mu.Unlock()
	} else {
		a.mu.Lock()
		a.agentConfig = config
		a.mu.Unlock()
	}

	a.logger.Info("설정 업데이트 적용 완료", "agentID", config.ID)
	return nil
}

// ID 는 에이전트 ID 를 반환한다.
func (a *Hvacr01Agent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *Hvacr01Agent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *Hvacr01Agent) Type() string {
	return "samsung_hvacr01"
}

// Info 는 에이전트 정보의 스냅샷을 반환한다.
func (a *Hvacr01Agent) Info() agent.AgentInfo {
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
		Type:      "samsung_hvacr01",
		State:     state,
		Health:    a.Health(),
		Config:    cfg,
		Stats:     a.stats.Snapshot(),
		StartedAt: startedAt,
		Uptime:    uptime,
		CreatedAt: createdAt,
	}
}

// BufferInfo returns the pending and capacity of the message buffer.
func (a *Hvacr01Agent) BufferInfo() (int, int) {
	return len(a.msgCh), cap(a.msgCh)
}

// Stats 는 통계 스냅샷을 반환한다.
func (a *Hvacr01Agent) Stats() agent.StatsSnapshot {
	s := a.stats.Snapshot()
	s.MsgBufferPending, s.MsgBufferCapacity = a.BufferInfo()
	return s
}

// State 는 디바이스 요약 상태를 반환한다.
// agent.StatefulAgent 인터페이스 구현 — detail=full API 응답에 포함된다.
func (a *Hvacr01Agent) State() map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()

	onlineCount := 0
	devices := make([]map[string]any, 0, len(a.devices))
	for addr, dev := range a.devices {
		if dev.Online {
			onlineCount++
		}
		d := map[string]any{
			"address":     addr.String(),
			"unit_id":     addr.String(),
			"device_id":   agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, addr.String()),
			"device_type": dev.Type,
			"online":      dev.Online,
		}
		if dev.State != nil {
			// SPEC-DEVICE-IDENTITY-001 후속: mode/fan_speed 는 hvac 통일 ID (int) 로
			// emit. 어댑터 (samsung/device.go) 와 동일 컨벤션, 사람이 읽는 형태는
			// web UI 의 매핑 layer 가 담당.
			d["state"] = map[string]any{
				"power":               dev.State.Power,
				"mode":                hvac.ModeFromName(dev.State.Mode),
				"target_temperature":  dev.State.TargetTemp,
				"current_temperature": dev.State.CurrentTemp,
				"fan_speed":           hvac.FanSpeedFromName(dev.State.FanSpeed),
			}
		}
		if !dev.LastSeen.IsZero() {
			d["last_seen_ms"] = dev.LastSeen.UnixMilli()
		}
		devices = append(devices, d)
	}

	result := map[string]any{
		"device_count": len(a.devices),
		"online_count": onlineCount,
		"devices":      devices,
	}

	result["transport_connected"] = a.transport.Available()
	a.reconnectMu.Lock()
	result["reconnecting"] = a.isReconnecting
	result["reconnect_attempts"] = a.reconnectAttempts
	a.reconnectMu.Unlock()

	if len(a.hvacr01Config.UnsupportedMsgSets) > 0 {
		sets := make([]string, 0, len(a.hvacr01Config.UnsupportedMsgSets))
		for idx := range a.hvacr01Config.UnsupportedMsgSets {
			sets = append(sets, fmt.Sprintf("0x%04X", idx))
		}
		result["unsupported_msg_sets"] = sets
	}
	return result
}

// ReceiveMessage 는 msgCh 에서 메시지를 수신한다.
// agent.MessageReceiver 인터페이스 구현.
func (a *Hvacr01Agent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case data := <-a.msgCh:
		a.logger.Debug("samsung_hvacr01: ReceiveMessage 전달",
			"bytes", len(data),
			"preview", truncateForLog(data, 120),
		)
		return data, nil
	case <-a.stopCh:
		return nil, fmt.Errorf("samsung_hvacr01: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// truncateForLog 는 바이트 데이터를 로깅용으로 잘라서 문자열로 반환한다.
func truncateForLog(data []byte, maxLen int) string {
	if len(data) <= maxLen {
		return string(data)
	}
	return string(data[:maxLen]) + "..."
}
