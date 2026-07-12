package lg

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/hvac"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// LGAPAgent 는 LG LGAP HVAC 에이전트이다.
// agent.Agent, agent.MessageReceiver, agent.StatefulAgent, agent.BufferInfoProvider 인터페이스를 구현한다.
type LGAPAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig agent.AgentConfig
	lgapConfig  LGAPConfig
	devices     map[byte]*LGAPDevice // zone -> device
	deviceIDs   map[string]byte      // device_id -> zone 역참조
	transport   LGAPTransport
	protocol    LGAPProtocol
	mu          sync.RWMutex
	pollMu      sync.Mutex // 시리얼 포트 동시 접근 방지
	pollTicker  *time.Ticker
	lastStates  map[byte]LGAPDeviceState // zone -> 마지막 상태
	stopCh      chan struct{}
	msgCh       chan []byte // Bridge 메시지 (ReceiveMessage)
	stats       *agent.AgentStats
	logger      *slog.Logger
	startedAt   time.Time
	createdAt   time.Time
	paused      bool

	reconnectMu       sync.Mutex // reconnecting 상태 보호
	isReconnecting    bool       // 재연결 진행 중 여부
	reconnectAttempts int        // 현재 재연결 시도 횟수

	// connWg 는 SPEC-HVACR-CONNSTATE-001 이 도입한 신규 connection-state goroutine
	// (비동기 startup probe + 주기 connection-report 루프) 전용 WaitGroup 이다.
	// Stop 에서 join 되어 leak 을 방지한다(§7.2/N10).
	//
	// [수용된 비대칭] LGAP 의 기존 bare goroutine(pollLoop/notifyLoop/reconnectLoop)은
	// 본 SPEC 범위 밖이라 connWg 에 편입하지 않는다. 이 수명주기 비대칭의 전면 해소는
	// 후속 리팩터링 SPEC 의 몫이다(§7.2 참조).
	connWg sync.WaitGroup

	// v0.7.2: get_recent 용 cumulative snapshot buffer (NASA recentSnapshots 패턴).
	// emit (change/report) 시마다 push 되며 lastSeq 이후 entry 만 반환.
	recentMu        sync.Mutex
	recentSeq       int64
	recentSnapshots []lgapRecentEntry

	// onDeviceStateChangeV2 는 SPEC-DEVICE-IDENTITY-001 Phase D 의 1급 콜백이다.
	// (agentName, deviceUID, deviceCompositeID) 를 인자로 받는다.
	// deviceUID 는 UUID v4 (저장소 미설정 시 빈 문자열).
	// Phase D (xflowd v1.0) 부터 V1 시그니처는 완전히 제거되었다.
	onDeviceStateChangeV2 agent.DeviceStateChangeCallbackV2
}

// lgapRecentEntry 는 LGAP recent snapshot 링버퍼 항목이다 (v0.7.2).
type lgapRecentEntry struct {
	Seq  int64
	Data []byte
}

const lgapRecentSnapshotsCapacity = 128

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*LGAPAgent)(nil)
var _ agent.MessageReceiver = (*LGAPAgent)(nil)
var _ agent.StatefulAgent = (*LGAPAgent)(nil)
var _ agent.BufferInfoProvider = (*LGAPAgent)(nil)
var _ agent.TransportChecker = (*LGAPAgent)(nil)

// TransportConnected 는 시리얼 트랜스포트의 실제 연결 상태를 반환한다.
func (a *LGAPAgent) TransportConnected() bool {
	return a.transport.Available()
}

// DeviceProvider 는 이 에이전트의 디바이스를 device.DeviceProvider 로 노출한다.
func (a *LGAPAgent) DeviceProvider() device.DeviceProvider {
	return NewLGAPDeviceProvider(a)
}

// SetDeviceStateChangeCallbackV2 는 1급 V2 콜백을 등록한다.
// (agentName, deviceUID, deviceCompositeID) 를 인자로 받는다.
//
// SPEC-DEVICE-IDENTITY-001 § M3 (Phase D — V1 setter 제거).
func (a *LGAPAgent) SetDeviceStateChangeCallbackV2(fn agent.DeviceStateChangeCallbackV2) {
	a.onDeviceStateChangeV2 = fn
}

// processRequest 는 Process 메서드의 JSON 요청 구조체이다.
type processRequest struct {
	Command  string         `json:"command"`
	Zone     *int           `json:"zone,omitempty"`
	DeviceID string         `json:"device_id,omitempty"`
	Params   map[string]any `json:"params,omitempty"`
	// v0.7.2: get_recent 용.
	LastSeq int64 `json:"last_seq,omitempty"`
	Count   int   `json:"count,omitempty"`
}

// NewLGAPAgent 는 LGAPAgent 팩토리 함수이다.
func NewLGAPAgent(config agent.AgentConfig) (agent.Agent, error) {
	lgapConfig, err := parseLGAPConfig(config.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("lgap agent: %w", err)
	}

	transport := newLGAPSerialTransport(lgapConfig)
	protocol := NewLGAPProtocol()

	a := &LGAPAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("lgap")),
		lgapConfig:    lgapConfig,
		devices:       make(map[byte]*LGAPDevice),
		deviceIDs:     make(map[string]byte),
		transport:     transport,
		protocol:      protocol,
		lastStates:    make(map[byte]LGAPDeviceState),
		stopCh:        make(chan struct{}),
		msgCh:         make(chan []byte, lgapConfig.MsgChannelSize),
		stats:         agent.NewAgentStats(),
		logger:        agent.ResolveLogger(config),
		createdAt:     time.Now(),
	}

	// 설정에 정의된 디바이스 등록
	for _, entry := range lgapConfig.Devices {
		zone := toInt(parseZoneKey(entry.Address))
		zoneByte := byte(zone)
		// source 보존: 런타임("bridge") 디바이스가 영속화 왕복 후에도 출처를 유지해
		// 삭제 가능성이 보존되도록 entry.Source 를 우선한다. 비어 있으면 "config".
		source := entry.Source
		if source == "" {
			source = "config"
		}
		dev := &LGAPDevice{
			Zone:   zoneByte,
			UnitID: entry.Name,
			Online: false,
			State:  &LGAPDeviceState{},
			Source: source,
		}
		a.devices[zoneByte] = dev
		if entry.Name != "" {
			a.deviceIDs[entry.Name] = zoneByte
		}
	}

	if err := a.Init(config); err != nil {
		return nil, err
	}

	return a, nil
}

// Init 은 에이전트를 초기화한다.
func (a *LGAPAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("lgap init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("lgap init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("lgap init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	a.logger.Info("lgap: 에이전트 초기화 완료",
		"serial_port", a.lgapConfig.SerialPort,
		"devices", len(a.devices),
	)

	return nil
}

// Start 는 트랜스포트를 열고 폴링 루프를 시작한다.
func (a *LGAPAgent) Start(_ context.Context) error {
	if a.CurrentState() == lifecycle.StateRunning && a.transport.Available() {
		return nil // 이미 실행 중이면 no-op
	}

	// 동일 인스턴스 재기동 시 이전 Stop 에서 close 된 stopCh 를 새 채널로 교체한다.
	// (Manager.Restart 는 새 인스턴스를 사용하지만, 직접 Stop/Start 경로를 방어.)
	a.mu.Lock()
	select {
	case <-a.stopCh:
		a.stopCh = make(chan struct{})
	default:
	}
	a.mu.Unlock()

	if err := a.transport.Open(); err != nil {
		// 연결 실패 시 에러 반환 대신 재연결 루프 시작
		a.logger.Warn("lgap: 트랜스포트 연결 실패, 재연결 대기", "error", err)
		go a.reconnectLoop()
	} else {
		// 연결 성공 시 폴링 루프 시작
		go a.pollLoop()
	}

	// v0.6.8: 정기 상태 보고 루프 (NotifyInterval > 0 일 때만 시작).
	if a.lgapConfig.NotifyInterval > 0 {
		go a.notifyLoop()
	}

	// SPEC-HVACR-CONNSTATE-001: 비동기 startup probe + 주기 connection-report 루프.
	// 기존 bare goroutine 과 달리 connWg 로 join 하여 Stop 시 leak 을 방지한다(§7.2).
	// Start 는 probe 완료를 기다리지 않고 즉시 반환한다(E6/AC-8).
	a.connWg.Add(1)
	go a.startupProbeLoop()
	if a.lgapConfig.ConnectionReportInterval > 0 {
		a.connWg.Add(1)
		go a.connectionReportLoop()
	}

	a.logger.Info("lgap: 에이전트 시작 완료")
	return nil
}

// notifyLoop 은 NotifyInterval 마다 모든 디바이스의 마지막 상태를 trigger="report"
// 로 emit 한다 (v0.6.8). LGCP 의 sendDeviceNotifications 패턴 차용.
func (a *LGAPAgent) notifyLoop() {
	ticker := time.NewTicker(a.lgapConfig.NotifyInterval)
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

// emitPeriodicReport 는 모든 등록된 디바이스의 상태를 trigger="report" 로
// emit 한다 (v0.6.8).
func (a *LGAPAgent) emitPeriodicReport() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for zone, dev := range a.devices {
		a.emitDeviceStateLocked(zone, dev, "report")
	}
}

// emitDeviceStateLocked 는 디바이스 상태를 5개 HVAC 에이전트 통합 schema 로
// emit 한다 (v0.7.0). 호출 전제: a.mu 락 보유.
//
// 출력 schema: {type:"device_state", dev_id, trigger, last_seen_ms, state, metadata}
//   - trigger: "change" | "report"
//   - state: LGAPDeviceState.StateForJSON()
//   - metadata: label / zone / device_type
//
// v0.7.2: get_recent 용 recentSnapshots 버퍼에도 push.
func (a *LGAPAgent) emitDeviceStateLocked(zone byte, dev *LGAPDevice, trigger string) {
	if dev == nil || dev.State == nil {
		return
	}
	label := dev.Name
	if label == "" {
		label = dev.UnitID
	}
	if label == "" {
		label = fmt.Sprintf("zone-%02X", zone)
	}
	metadata := map[string]any{
		"name":        label,
		"zone":        fmt.Sprintf("0x%02X", zone),
		"device_type": "HVACR.IDU",
	}
	// v0.9.0: payload.type 제거. eventType="" 로 sendEventLocked 호출 시 type 필드 주입 skip.
	// v0.18.6: unit_id (프로토콜) + device_id (UUID) 분리.
	// FIX: a.Name() 호출 금지 — caller 가 a.mu 쓰기 락 보유 중. a.Name() 은 같은
	// mutex 의 RLock 을 시도하여 자기 deadlock 을 일으킨다. agentConfig.Name 직접 사용.
	payload := map[string]any{
		"unit_id":   dev.UnitID,
		"device_id": agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, dev.UnitID),
		"trigger":   trigger,
		"state":     dev.State.StateForJSON(),
		"metadata":  metadata,
	}
	if !dev.LastSeen.IsZero() {
		payload["last_seen_ms"] = dev.LastSeen.UnixMilli()
	}
	a.sendEventLocked("", payload)

	// v0.7.2: recentSnapshots 에도 push (get_recent 노드 요청에 응답).
	// payload 는 sendEventLocked 가 type 필드를 덮어쓰므로 이미 type=device_state.
	b, err := json.Marshal(payload)
	if err == nil {
		a.recentMu.Lock()
		a.recentSeq++
		entry := lgapRecentEntry{Seq: a.recentSeq, Data: b}
		if len(a.recentSnapshots) >= lgapRecentSnapshotsCapacity {
			a.recentSnapshots = a.recentSnapshots[1:]
		}
		a.recentSnapshots = append(a.recentSnapshots, entry)
		a.recentMu.Unlock()
	}
}

// processGetStats 는 에이전트의 캡처/송수신 통계를 반환한다 (v0.7.3).
// 5개 HVAC 노드 통일 명령 — Century/LG ICP-01/LGCP 의 get_stats 패턴 차용.
func (a *LGAPAgent) processGetStats() ([]byte, error) {
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

// processGetRecent 는 last_seq 이후의 device_state 스냅샷을 반환한다 (v0.7.2).
//
//	count > 0: 최근 count 개
//	count == 0 또는 미지정: 전체 누적 (NASA 의 default 32 와 다름 — v0.7.1 통일 의미)
func (a *LGAPAgent) processGetRecent(req *processRequest) ([]byte, error) {
	count := req.Count
	a.recentMu.Lock()
	entries := make([]lgapRecentEntry, 0, len(a.recentSnapshots))
	for _, e := range a.recentSnapshots {
		if e.Seq > req.LastSeq {
			entries = append(entries, e)
			if count > 0 && len(entries) >= count {
				break
			}
		}
	}
	a.recentMu.Unlock()

	snapshots := make([]json.RawMessage, len(entries))
	for i, e := range entries {
		snap, _ := json.Marshal(map[string]any{
			"seq":    e.Seq,
			"device": json.RawMessage(e.Data),
		})
		snapshots[i] = snap
	}

	return json.Marshal(map[string]any{
		"count":     len(snapshots),
		"snapshots": snapshots,
	})
}

// Stop 은 에이전트를 정지한다.
func (a *LGAPAgent) Stop(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("lgap stop: %w", err)
	}

	// goroutine 들에게 종료 시그널
	close(a.stopCh)

	a.mu.Lock()
	if a.pollTicker != nil {
		a.pollTicker.Stop()
		a.pollTicker = nil
	}
	a.mu.Unlock()

	// SPEC-HVACR-CONNSTATE-001 §7.2/N10: 신규 connection-state goroutine(startup probe +
	// 주기 리포트)을 msgCh 드레인 이전에 join 한다. 이로써 (a) goroutine leak 이 없고,
	// (b) Stop 시작 이후 방출된 메시지가 남지 않는다(join 이후 드레인이 모두 청소).
	// 기존 bare goroutine(pollLoop 등)은 본 SPEC 범위 밖이라 join 하지 않는다(수용된 비대칭).
	a.connWg.Wait()

	// 트랜스포트 닫기
	if err := a.transport.Close(); err != nil {
		a.logger.Warn("lgap: transport close error", "error", err)
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
		return fmt.Errorf("lgap stop: %w", err)
	}

	return nil
}

// Pause 는 Running -> Paused 로 전환한다.
func (a *LGAPAgent) Pause(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("lgap pause: %w", err)
	}
	a.mu.Lock()
	a.paused = true
	a.mu.Unlock()
	return nil
}

// Resume 은 Paused -> Running 으로 전환한다.
func (a *LGAPAgent) Resume(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("lgap resume: %w", err)
	}
	a.mu.Lock()
	a.paused = false
	a.mu.Unlock()
	return nil
}

// Health 는 에이전트의 건강 상태를 반환한다.
func (a *LGAPAgent) Health() agent.HealthStatus {
	now := time.Now()
	state := a.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		return agent.HealthStatus{
			Status:    agent.HealthHealthy,
			LastCheck: now,
			Message:   "lgap agent is running",
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "lgap agent is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("lgap agent is in %s state", state),
		}
	}
}

// Process 는 JSON 명령을 디스패치하여 처리한다.
func (a *LGAPAgent) Process(data []byte) ([]byte, error) {
	var req processRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("lgap process: invalid JSON: %w", err)
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
	case "get_all", "get_all_states":
		// v0.7.1: get_all_states → get_all (5개 HVAC 노드 명령 통일).
		// get_all_states 는 deprecation alias 로 silent accept.
		return a.processGetAllStates()
	case "get_recent":
		// v0.7.2: 5개 HVAC 노드 통일 명령. recentSnapshots 에서 lastSeq 이후 반환.
		return a.processGetRecent(&req)
	case "get_stats":
		// v0.7.3: 5개 HVAC 노드 통일 명령. 에이전트 캡처/송수신 통계 반환.
		return a.processGetStats()
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

// ---------------------------------------------------------------------------
// 제어 명령 처리
// ---------------------------------------------------------------------------

// processSetPower 는 전원 켜기/끄기 명령을 처리한다.
func (a *LGAPAgent) processSetPower(req *processRequest) ([]byte, error) {
	zone, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}
	if !dev.Online {
		return nil, ErrDeviceOffline
	}

	power, ok := req.Params["power"].(bool)
	if !ok {
		return nil, fmt.Errorf("lgap: power parameter must be boolean")
	}

	a.logger.Debug("lgap: set_power 요청", "device", dev.UnitID, "zone", fmt.Sprintf("0x%02X", zone), "power", power)

	var flags byte
	if power {
		flags |= FlagPower
	}
	// 현재 상태를 기반으로 모드/팬/온도 유지
	modeCombo := EncodeModeCombo(StringToMode[dev.State.Mode], StringToFanSpeed[dev.State.FanSpeed], dev.State.SwingAuto)
	temp := EncodeTargetTemp(dev.State.TargetTemp)

	if err := a.sendControlCommand(zone, flags, modeCombo, temp); err != nil {
		return nil, err
	}

	return a.buildSuccessResponse(zone, dev.UnitID, map[string]any{"power": power})
}

// processSetMode 는 운전 모드 변경 명령을 처리한다.
func (a *LGAPAgent) processSetMode(req *processRequest) ([]byte, error) {
	zone, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}
	if !dev.Online {
		return nil, ErrDeviceOffline
	}

	modeStr, ok := req.Params["mode"].(string)
	if !ok {
		return nil, fmt.Errorf("lgap: mode parameter must be string")
	}

	modeVal, exists := StringToMode[modeStr]
	if !exists {
		return nil, ErrInvalidMode
	}

	a.logger.Debug("lgap: set_mode 요청", "device", dev.UnitID, "zone", fmt.Sprintf("0x%02X", zone), "mode", modeStr)

	var flags byte
	if dev.State.Power {
		flags |= FlagPower
	}
	modeCombo := EncodeModeCombo(modeVal, StringToFanSpeed[dev.State.FanSpeed], dev.State.SwingAuto)
	temp := EncodeTargetTemp(dev.State.TargetTemp)

	if err := a.sendControlCommand(zone, flags, modeCombo, temp); err != nil {
		return nil, err
	}

	return a.buildSuccessResponse(zone, dev.UnitID, map[string]any{"mode": modeStr})
}

// processSetTemperature 는 목표 온도 설정 명령을 처리한다.
func (a *LGAPAgent) processSetTemperature(req *processRequest) ([]byte, error) {
	zone, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}
	if !dev.Online {
		return nil, ErrDeviceOffline
	}

	tempVal, ok := req.Params["target_temperature"].(float64)
	if !ok {
		return nil, fmt.Errorf("lgap: target_temp parameter must be number")
	}

	if tempVal < 16.0 || tempVal > 30.0 {
		return nil, ErrTemperatureOutOfRange
	}

	a.logger.Debug("lgap: target_temperature 요청", "device", dev.UnitID, "zone", fmt.Sprintf("0x%02X", zone), "target_temperature", tempVal)

	var flags byte
	if dev.State.Power {
		flags |= FlagPower
	}
	modeCombo := EncodeModeCombo(StringToMode[dev.State.Mode], StringToFanSpeed[dev.State.FanSpeed], dev.State.SwingAuto)
	temp := EncodeTargetTemp(int(tempVal))

	if err := a.sendControlCommand(zone, flags, modeCombo, temp); err != nil {
		return nil, err
	}

	return a.buildSuccessResponse(zone, dev.UnitID, map[string]any{"target_temperature": tempVal})
}

// processSetFanSpeed 는 팬 속도 변경 명령을 처리한다.
func (a *LGAPAgent) processSetFanSpeed(req *processRequest) ([]byte, error) {
	zone, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}
	if !dev.Online {
		return nil, ErrDeviceOffline
	}

	speedStr, ok := req.Params["fan_speed"].(string)
	if !ok {
		return nil, fmt.Errorf("lgap: fan_speed parameter must be string")
	}

	speedVal, exists := StringToFanSpeed[speedStr]
	if !exists {
		return nil, ErrInvalidFanSpeed
	}

	a.logger.Debug("lgap: set_fan_speed 요청", "device", dev.UnitID, "zone", fmt.Sprintf("0x%02X", zone), "fan_speed", speedStr)

	var flags byte
	if dev.State.Power {
		flags |= FlagPower
	}
	modeCombo := EncodeModeCombo(StringToMode[dev.State.Mode], speedVal, dev.State.SwingAuto)
	temp := EncodeTargetTemp(dev.State.TargetTemp)

	if err := a.sendControlCommand(zone, flags, modeCombo, temp); err != nil {
		return nil, err
	}

	return a.buildSuccessResponse(zone, dev.UnitID, map[string]any{"fan_speed": speedStr})
}

// processSetMultiple 는 복수 설정 변경 명령을 처리한다.
func (a *LGAPAgent) processSetMultiple(req *processRequest) ([]byte, error) {
	zone, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}
	if !dev.Online {
		return nil, ErrDeviceOffline
	}

	result := make(map[string]any)

	// 기존 상태 기반으로 시작
	power := dev.State.Power
	mode := StringToMode[dev.State.Mode]
	fan := StringToFanSpeed[dev.State.FanSpeed]
	swingAuto := dev.State.SwingAuto
	targetTemp := dev.State.TargetTemp

	// power
	if powerVal, ok := req.Params["power"]; ok && powerVal != nil {
		p, _ := powerVal.(bool)
		power = p
		result["power"] = power
	}

	// mode
	if modeVal, ok := req.Params["mode"]; ok && modeVal != nil {
		modeStr, _ := modeVal.(string)
		modeByte, exists := StringToMode[modeStr]
		if !exists {
			return nil, ErrInvalidMode
		}
		mode = modeByte
		result["mode"] = modeStr
	}

	// target_temp
	if tempVal, ok := req.Params["target_temperature"]; ok && tempVal != nil {
		temp, _ := tempVal.(float64)
		if temp < 16.0 || temp > 30.0 {
			return nil, ErrTemperatureOutOfRange
		}
		targetTemp = int(temp)
		result["target_temperature"] = temp
	}

	// fan_speed
	if speedVal, ok := req.Params["fan_speed"]; ok && speedVal != nil {
		speedStr, _ := speedVal.(string)
		speedByte, exists := StringToFanSpeed[speedStr]
		if !exists {
			return nil, ErrInvalidFanSpeed
		}
		fan = speedByte
		result["fan_speed"] = speedStr
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("lgap: set_multiple requires at least one setting")
	}

	var flags byte
	if power {
		flags |= FlagPower
	}
	modeCombo := EncodeModeCombo(mode, fan, swingAuto)
	temp := EncodeTargetTemp(targetTemp)

	if err := a.sendControlCommand(zone, flags, modeCombo, temp); err != nil {
		return nil, err
	}

	return a.buildSuccessResponse(zone, dev.UnitID, result)
}

// ---------------------------------------------------------------------------
// 상태 조회 명령 처리
// ---------------------------------------------------------------------------

// processGetState 는 단일 디바이스 상태 조회 명령을 처리한다.
func (a *LGAPAgent) processGetState(req *processRequest) ([]byte, error) {
	zone, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}

	resp := map[string]any{
		"status":    "ok",
		"zone":      fmt.Sprintf("0x%02X", zone),
		"unit_id":   dev.UnitID,
		"device_id": agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, dev.UnitID),
		"online":    dev.Online,
	}

	if dev.State != nil {
		resp["state"] = dev.State.StateForJSON()
	}
	if !dev.LastSeen.IsZero() {
		resp["last_seen_ms"] = dev.LastSeen.UnixMilli()
	}

	return json.Marshal(resp)
}

// processGetAllStates 는 전체 디바이스 상태 조회 명령을 처리한다.
func (a *LGAPAgent) processGetAllStates() ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var devices []map[string]any
	for zone, dev := range a.devices {
		d := map[string]any{
			"zone":      fmt.Sprintf("0x%02X", zone),
			"unit_id":   dev.UnitID,
			"device_id": agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, dev.UnitID),
			"online":    dev.Online,
		}
		if dev.State != nil {
			d["state"] = dev.State.StateForJSON()
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

// ---------------------------------------------------------------------------
// 디바이스 관리 명령 처리
// ---------------------------------------------------------------------------

// processAddDevice 는 디바이스 추가 명령을 처리한다.
func (a *LGAPAgent) processAddDevice(req *processRequest) ([]byte, error) {
	// zone 파라미터 추출
	var zoneInt int
	if req.Zone != nil {
		zoneInt = *req.Zone
	} else if v, ok := req.Params["zone"]; ok {
		zoneInt = toInt(v)
	} else {
		return nil, fmt.Errorf("lgap: zone is required for add_device")
	}

	deviceID := req.DeviceID
	if deviceID == "" {
		if v, ok := req.Params["device_id"].(string); ok {
			deviceID = v
		}
	}

	// name 파라미터 추출 (사용자 정의 디바이스 이름)
	name := ""
	if v, ok := req.Params["name"].(string); ok {
		name = v
	}

	zoneByte := byte(zoneInt)

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.devices[zoneByte]; exists {
		return nil, ErrDeviceAlreadyRegistered
	}

	// device_id 중복 체크
	if deviceID != "" {
		if _, exists := a.deviceIDs[deviceID]; exists {
			return nil, ErrDuplicateDeviceID
		}
	}

	dev := &LGAPDevice{
		Zone:   zoneByte,
		UnitID: deviceID,
		Name:   name,
		Online: false,
		State:  &LGAPDeviceState{},
		Source: "bridge",
	}

	a.devices[zoneByte] = dev
	if deviceID != "" {
		a.deviceIDs[deviceID] = zoneByte
	}

	// 이벤트 전송
	a.sendEventLocked("device_registered", map[string]any{
		"zone":      fmt.Sprintf("0x%02X", zoneByte),
		"unit_id":   deviceID,
		"device_id": agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, deviceID),
		"name":      name,
	})

	resp := map[string]any{
		"status":    "ok",
		"zone":      fmt.Sprintf("0x%02X", zoneByte),
		"unit_id":   deviceID,
		"device_id": agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, deviceID),
		"name":      name,
	}
	return json.Marshal(resp)
}

// processRemoveDevice 는 디바이스 제거 명령을 처리한다.
func (a *LGAPAgent) processRemoveDevice(req *processRequest) ([]byte, error) {
	// params 폴백 처리
	if req.Zone == nil {
		if v, ok := req.Params["zone"]; ok {
			z := toInt(v)
			req.Zone = &z
		}
	}
	if req.DeviceID == "" {
		if v, ok := req.Params["device_id"].(string); ok {
			req.DeviceID = v
		}
	}

	zone, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}

	// config 소스 디바이스도 UI 에서 삭제 가능하게 한다(보호 제거). 수동 추가 후
	// 재시작으로 "config" 로 굳은 디바이스를 사용자가 직접 삭제할 수 있어야 하기 때문이다.

	a.mu.Lock()
	defer a.mu.Unlock()

	// deviceIDs 맵에서도 제거
	if dev.UnitID != "" {
		delete(a.deviceIDs, dev.UnitID)
	}
	delete(a.devices, zone)

	a.sendEventLocked("device_unregistered", map[string]any{
		"zone":      fmt.Sprintf("0x%02X", zone),
		"unit_id":   dev.UnitID,
		"device_id": agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, dev.UnitID),
	})

	resp := map[string]any{
		"status":    "ok",
		"zone":      fmt.Sprintf("0x%02X", zone),
		"unit_id":   dev.UnitID,
		"device_id": agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, dev.UnitID),
	}
	return json.Marshal(resp)
}

// processListDevices 는 디바이스 목록 조회 명령을 처리한다.
func (a *LGAPAgent) processListDevices() ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var devices []map[string]any
	for zone, dev := range a.devices {
		devices = append(devices, map[string]any{
			"zone":      fmt.Sprintf("0x%02X", zone),
			"unit_id":   dev.UnitID,
			"device_id": agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, dev.UnitID),
			"online":    dev.Online,
			"source":    dev.Source,
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

// resolveDevice 는 요청에서 디바이스 존과 포인터를 해석한다.
// device_id 가 우선이며, 없으면 zone 을 사용한다.
func (a *LGAPAgent) resolveDevice(req *processRequest) (byte, *LGAPDevice, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var zone byte

	if req.DeviceID != "" {
		resolved, ok := a.deviceIDs[req.DeviceID]
		if !ok {
			// UUID 폴백: 전역 device.List / REST 응답은 UUID(1급 식별자) 만 노출하므로
			// (zone/UnitID 미노출), 삭제/실행 경로가 device_id 로 UUID 를 전달한다.
			// deviceIDs 는 UnitID 로만 키잉되므로, UUID 는 각 디바이스의 emit-경로 UUID
			// (ResolveDeviceID(name, dev.UnitID)) 와 대조해 역매칭한다.
			if byUUID, ok2 := a.zoneByDeviceUUID(req.DeviceID); ok2 {
				resolved, ok = byUUID, true
			}
		}
		if !ok {
			return 0, nil, ErrDeviceIDNotFound
		}
		zone = resolved
	} else if req.Zone != nil {
		zone = byte(*req.Zone)
	} else {
		return 0, nil, fmt.Errorf("lgap: zone or device_id is required")
	}

	dev, ok := a.devices[zone]
	if !ok {
		return 0, nil, ErrDeviceNotFound
	}

	return zone, dev, nil
}

// zoneByDeviceUUID 는 글로벌 UUID(device_id) 를 각 디바이스의 레지스트리 UUID 와
// 대조해 해당 zone 을 역매칭한다. 매칭 실패 시 (0, false).
//
// localID 는 반드시 어댑터(SamsungNasaDeviceInfo.Address = formatZone(zone)) 와 동일한
// formatZone(zone) 을 사용한다 — 프론트엔드/REST 가 받는 device.uid 는 레지스트리
// 어댑터의 UID() (ResolveDeviceID(name, formatZone(zone))) 이기 때문이다. dev.UnitID 를
// 쓰면 emit 경로 UUID 와는 맞아도 레지스트리 UUID 와 어긋날 수 있다.
//
// 주의(RWMutex 비재진입): 호출자(resolveDevice) 가 a.mu 를 보유한 상태에서 호출하므로
// 여기서 a.mu 를 재-lock 하지 않으며 a.Name() 도 호출하지 않는다(재진입 deadlock 회피).
func (a *LGAPAgent) zoneByDeviceUUID(uuid string) (byte, bool) {
	for zone := range a.devices {
		if agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, formatZone(zone)) == uuid {
			return zone, true
		}
	}
	return 0, false
}

// sendControlCommand 는 제어 패킷을 빌드하고 트랜스포트로 전송한다.
// pollMu 를 사용하여 시리얼 포트 동시 접근을 방지한다.
func (a *LGAPAgent) sendControlCommand(zone byte, flags byte, modeCombo byte, temp byte) error {
	a.pollMu.Lock()
	defer a.pollMu.Unlock()

	pkt := a.protocol.BuildControlCommand(zone, flags, modeCombo, temp)

	a.logger.Debug("lgap: 제어 명령 전송",
		"zone", fmt.Sprintf("0x%02X", zone),
		"flags", fmt.Sprintf("0x%02X", flags),
		"tx", hex.EncodeToString(pkt),
	)

	if err := a.transport.Send(pkt); err != nil {
		a.stats.IncrExternalMessagesErrored()
		a.logger.Error("lgap: 제어 명령 전송 실패", "zone", fmt.Sprintf("0x%02X", zone), "error", err)
		return fmt.Errorf("lgap: send failed: %w", err)
	}

	a.stats.IncrExternalMessagesSent()
	a.stats.AddBytesWritten(int64(len(pkt)))

	// 동기 응답 수신 (16바이트)
	buf := make([]byte, ResponseSize)
	n, err := a.transport.Receive(buf)
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		a.logger.Error("lgap: 응답 수신 실패", "zone", fmt.Sprintf("0x%02X", zone), "error", err)
		return fmt.Errorf("lgap: receive failed: %w", err)
	}

	a.stats.IncrExternalMessagesReceived()
	a.stats.AddBytesRead(int64(n))

	a.logger.Debug("lgap: 제어 응답 수신",
		"zone", fmt.Sprintf("0x%02X", zone),
		"rx", hex.EncodeToString(buf[:n]),
		"bytes", n,
	)

	if n < ResponseSize {
		a.logger.Warn("lgap: 응답 크기 부족", "zone", fmt.Sprintf("0x%02X", zone), "received", n)
		return ErrNoResponse
	}

	resp, err := a.protocol.DecodeResponse(buf[:ResponseSize])
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		return fmt.Errorf("lgap: decode response failed: %w", err)
	}

	// 에러 코드 확인
	if resp.Error != 0 {
		return ErrResponseError
	}

	// 응답으로 상태 업데이트
	a.handleResponse(zone, resp)

	a.stats.UpdateLastActivity()
	a.logger.Debug("lgap: 제어 명령 전송 완료", "zone", fmt.Sprintf("0x%02X", zone))
	return nil
}

// buildSuccessResponse 는 제어 명령 성공 응답 JSON 을 생성한다.
func (a *LGAPAgent) buildSuccessResponse(zone byte, deviceID string, result map[string]any) ([]byte, error) {
	resp := map[string]any{
		"status":    "ok",
		"zone":      fmt.Sprintf("0x%02X", zone),
		"unit_id":   deviceID,
		"device_id": agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, deviceID),
		"result":    result,
	}
	return json.Marshal(resp)
}

// sendEvent 는 이벤트를 msgCh 로 비동기 전송한다 (락 없이 호출).
func (a *LGAPAgent) sendEvent(eventType string, data map[string]any) {
	evt := map[string]any{"type": eventType}
	for k, v := range data {
		evt[k] = v
	}
	b, err := json.Marshal(evt)
	if err != nil {
		a.logger.Warn("lgap: event marshal failed", "error", err)
		return
	}
	a.sendToMsgCh(b, eventType)
}

// sendEventLocked 는 sendEvent 와 동일하지만 이미 락이 잡혀 있을 때 사용한다.
func (a *LGAPAgent) sendEventLocked(eventType string, data map[string]any) {
	// v0.9.0: eventType == "" 면 type 필드 주입 skip (device_state 의 경우
	// 노드의 message_type="device_state.<trigger>" 가 schema 식별 역할 담당).
	var b []byte
	var err error
	if eventType == "" {
		b, err = json.Marshal(data)
	} else {
		evt := map[string]any{"type": eventType}
		for k, v := range data {
			evt[k] = v
		}
		b, err = json.Marshal(evt)
	}
	if err != nil {
		return
	}
	logType := eventType
	if logType == "" {
		logType = "device_state"
	}
	a.sendToMsgCh(b, logType)
}

// sendToMsgCh 는 데이터를 msgCh 로 전송한다.
// 버퍼가 가득 차면 가장 오래된 메시지를 드롭하고 최신 메시지를 삽입한다 (ring buffer 전략).
func (a *LGAPAgent) sendToMsgCh(data []byte, eventType string) {
	select {
	case a.msgCh <- data:
		a.logger.Debug("lgap: 이벤트 msgCh 전송 성공",
			"type", eventType,
			"chLen", len(a.msgCh),
			"chCap", cap(a.msgCh),
		)
		return
	default:
	}

	// 버퍼 풀 — 가장 오래된 메시지를 드레인하여 공간 확보
	select {
	case <-a.msgCh:
	default:
	}
	a.logger.Warn("lgap: msgCh full, dropping oldest event", "type", eventType)

	select {
	case a.msgCh <- data:
	default:
	}
}

// handleResponse 는 응답을 처리하여 디바이스 상태를 업데이트한다.
func (a *LGAPAgent) handleResponse(zone byte, resp *LGAPResponse) {
	a.mu.Lock()
	defer a.mu.Unlock()

	dev, ok := a.devices[zone]
	if !ok {
		return
	}

	wasOffline := !dev.Online
	dev.Online = true
	dev.LastSeen = time.Now()
	dev.ErrorCount = 0

	// SPEC-HVACR-CONNSTATE-001: 첫 성공 통신을 initial=online baseline 으로 확정한다.
	// 아직 initial 이 방출되지 않았다면 여기서 initial=online 을 먼저 방출해 per-device
	// 순서(E9)를 보장한다. 이미 initial 이 방출된 뒤의 offline→online 복구는 change(E3).
	connNow := time.Now().UnixMilli()
	if !dev.connInitialEmitted {
		dev.connInitialEmitted = true
		a.emitConnectionLocked(dev, connTriggerInitial, connNow)
	} else if wasOffline {
		a.emitConnectionLocked(dev, connTriggerChange, connNow)
	}

	if wasOffline {
		a.sendEventLocked("device_online", map[string]any{
			"zone":      fmt.Sprintf("0x%02X", zone),
			"unit_id":   dev.UnitID,
			"device_id": agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, dev.UnitID),
		})
	}

	// 상태 업데이트
	if dev.State != nil {
		// 이전 상태 저장
		prevState := a.lastStates[zone]

		dev.State.UpdateFromResponse(resp)

		// 변경 감지
		currentState := *dev.State
		if lgapStateChanged(prevState, currentState) {
			// v0.6.7: event_temp_threshold gate — 비온도 필드 변경 없이 온도 센서값
			// (RoomTemp + PipeInTemp + PipeOutTemp) 만 변경된 경우 max|Δ| < threshold
			// 면 emit suppress. (v0.6.6: RoomTemp 만 검사 → Pipe 온도 변경 시
			// 새어나가는 결함 fix)
			if a.lgapConfig.EventTempThreshold > 0 &&
				!nonTempFieldsChangedLGAP(prevState, currentState) {
				if maxTempDeltaLGAP(prevState, currentState) < a.lgapConfig.EventTempThreshold {
					return
				}
			}
			a.lastStates[zone] = currentState
			a.logger.Debug("lgap: 상태 변경 감지",
				"device", dev.UnitID, "zone", fmt.Sprintf("0x%02X", zone),
				"power", currentState.Power, "mode", currentState.Mode,
				"target_temperature", currentState.TargetTemp, "fan_speed", currentState.FanSpeed)
			// v0.7.0: 통합 schema (type="device_state") 로 emit. 이전 별도 event
			// type ("device_state_changed") 폐기.
			a.emitDeviceStateLocked(zone, dev, "change")
			// WebSocket 브로드캐스트 콜백 (SPEC-DEVICE-IDENTITY-001 Phase D § M3 — V2 단일).
			if v2 := a.onDeviceStateChangeV2; v2 != nil {
				agentName := a.agentConfig.Name
				globalID := fmt.Sprintf("%s:%02X", agentName, zone)
				deviceUID := agent.ResolveDeviceID(context.Background(), agentName, dev.UnitID)
				a.logger.Debug("lgap: WebSocket 상태 변경 브로드캐스트", "agent", agentName, "globalID", globalID, "device_uid", deviceUID)
				go v2(agentName, deviceUID, globalID)
			}
		}
	}

	a.stats.UpdateLastActivity()
}

// ListDevices 는 등록된 디바이스 목록을 반환한다.
func (a *LGAPAgent) ListDevices() []LGAPDevice {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]LGAPDevice, 0, len(a.devices))
	for _, dev := range a.devices {
		result = append(result, *dev)
	}
	return result
}

// GetDeviceState 는 지정된 존의 디바이스 상태를 반환한다.
func (a *LGAPAgent) GetDeviceState(zone byte) (*LGAPDeviceState, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	dev, ok := a.devices[zone]
	if !ok {
		return nil, ErrDeviceNotFound
	}
	return dev.State, nil
}

// ---------------------------------------------------------------------------
// 재연결 루프
// ---------------------------------------------------------------------------

// reconnectLoop 는 트랜스포트 재연결을 시도하는 고루틴이다.
// 지수 백오프를 적용하며, 첫 시도만 Warn, 이후는 Debug 로그.
func (a *LGAPAgent) reconnectLoop() {
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
	a.sendEvent("transport_reconnecting", map[string]any{
		"timestamp_ms": time.Now().UnixMilli(),
	})

	baseInterval := a.lgapConfig.ReconnectInterval
	maxBackoff := a.lgapConfig.MaxReconnectBackoff
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
			a.logger.Info("lgap: 트랜스포트 재연결 성공",
				"attempts", attempt+1,
				"downtime", time.Since(disconnectedAt).Round(time.Second).String(),
			)
			a.sendEvent("transport_reconnected", map[string]any{
				"attempt_count":    attempt + 1,
				"downtime_seconds": int(time.Since(disconnectedAt).Seconds()),
				"timestamp_ms":     time.Now().UnixMilli(),
			})

			// 폴링 루프 재시작
			go a.pollLoop()
			return
		}

		// 재연결 실패
		a.reconnectMu.Lock()
		a.reconnectAttempts = attempt + 1
		a.reconnectMu.Unlock()

		if attempt == 0 {
			a.logger.Warn("lgap: 트랜스포트 재연결 시도 중", "error", err)
		} else {
			a.logger.Debug("lgap: 트랜스포트 재연결 시도", "attempt", attempt+1, "error", err)
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
// 폴링 루프 (동기식: 존별로 send + receive)
// ---------------------------------------------------------------------------

// pollLoop 는 주기적으로 모든 디바이스의 상태를 쿼리한다.
// LGAP 는 동기식 master/slave 이므로 각 존에 대해 순차적으로 send + receive 를 수행한다.
func (a *LGAPAgent) pollLoop() {
	ticker := time.NewTicker(a.lgapConfig.PollInterval)
	a.mu.Lock()
	a.pollTicker = ticker
	a.mu.Unlock()

	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.mu.RLock()
			paused := a.paused
			// 디바이스 존 스냅샷 복사
			zones := make([]byte, 0, len(a.devices))
			if !paused {
				for zone := range a.devices {
					zones = append(zones, zone)
				}
			}
			a.mu.RUnlock()
			if paused {
				continue
			}

			a.logger.Debug("lgap: 폴링 시작", "devices", len(zones))
			for _, zone := range zones {
				a.pollZone(zone)

				// 존 간 딜레이
				if a.lgapConfig.InterCommandDelay > 0 {
					time.Sleep(a.lgapConfig.InterCommandDelay)
				}
			}
		}
	}
}

// pollZone 은 단일 존에 대해 상태 쿼리를 수행한다.
// pollMu 를 사용하여 시리얼 포트 동시 접근을 방지한다.
func (a *LGAPAgent) pollZone(zone byte) {
	a.pollMu.Lock()
	defer a.pollMu.Unlock()

	pkt := a.protocol.BuildStatusQuery(zone)

	a.logger.Debug("lgap: 상태 쿼리 전송",
		"zone", fmt.Sprintf("0x%02X", zone),
		"tx", hex.EncodeToString(pkt),
	)

	if err := a.transport.Send(pkt); err != nil {
		// 연결 끊김 감지
		if !a.transport.Available() {
			a.logger.Warn("lgap: 트랜스포트 연결 끊김 감지", "error", err)
			a.sendEvent("transport_disconnected", map[string]any{
				"reason":       err.Error(),
				"timestamp_ms": time.Now().UnixMilli(),
			})
			go a.reconnectLoop()
			return
		}
		a.logger.Warn("lgap: 상태 쿼리 전송 실패", "zone", fmt.Sprintf("0x%02X", zone), "error", err)
		a.incrementErrorCount(zone)
		return
	}

	a.stats.IncrExternalMessagesSent()
	a.stats.AddBytesWritten(int64(len(pkt)))

	// 동기 응답 수신 (16바이트)
	buf := make([]byte, ResponseSize)
	n, err := a.transport.Receive(buf)
	if err != nil {
		// 연결 끊김 감지
		if !a.transport.Available() {
			a.logger.Warn("lgap: 트랜스포트 연결 끊김 감지", "error", err)
			a.sendEvent("transport_disconnected", map[string]any{
				"reason":       err.Error(),
				"timestamp_ms": time.Now().UnixMilli(),
			})
			go a.reconnectLoop()
			return
		}
		a.logger.Debug("lgap: 응답 수신 실패 (일시적)", "zone", fmt.Sprintf("0x%02X", zone), "error", err)
		a.incrementErrorCount(zone)
		return
	}

	if n < ResponseSize {
		a.logger.Debug("lgap: 응답 크기 부족", "zone", fmt.Sprintf("0x%02X", zone), "received", n)
		a.incrementErrorCount(zone)
		return
	}

	a.stats.IncrExternalMessagesReceived()
	a.stats.AddBytesRead(int64(n))

	a.logger.Debug("lgap: 상태 응답 수신",
		"zone", fmt.Sprintf("0x%02X", zone),
		"rx", hex.EncodeToString(buf[:n]),
		"bytes", n,
	)

	resp, err := a.protocol.DecodeResponse(buf[:ResponseSize])
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		a.logger.Debug("lgap: 응답 디코드 실패", "zone", fmt.Sprintf("0x%02X", zone), "error", err)
		a.incrementErrorCount(zone)
		return
	}

	// 응답으로 상태 업데이트
	a.handleResponse(zone, resp)
}

// incrementErrorCount 는 디바이스의 에러 카운트를 증가시키고 오프라인 판별한다.
func (a *LGAPAgent) incrementErrorCount(zone byte) {
	a.mu.Lock()
	defer a.mu.Unlock()

	dev, ok := a.devices[zone]
	if !ok {
		return
	}

	dev.ErrorCount++
	if dev.ErrorCount >= a.lgapConfig.OfflineThreshold && dev.Online {
		dev.Online = false
		a.sendEventLocked("device_offline", map[string]any{
			"zone":      fmt.Sprintf("0x%02X", zone),
			"unit_id":   dev.UnitID,
			"device_id": agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, dev.UnitID),
		})
		// SPEC-HVACR-CONNSTATE-001 E2: online→offline 전이 즉시 change 방출(tick 독립).
		// initial 미방출 device 는 순서 보장(E9)을 위해 skip — offline 은 online 상태에서만
		// 발생하므로(=initial online 방출됨) 이 게이트는 방어적이다.
		if dev.connInitialEmitted {
			a.emitConnectionLocked(dev, connTriggerChange, time.Now().UnixMilli())
		}
		a.logger.Warn("lgap: 디바이스 오프라인",
			"zone", fmt.Sprintf("0x%02X", zone),
			"device_id", dev.UnitID,
			"error_count", dev.ErrorCount,
		)
	}
}

// RegisterPinnedDevices 는 메타데이터에서 고정 설치로 표시된 디바이스를 등록한다.
// agent.Start() 이후에 호출된다.
func (a *LGAPAgent) RegisterPinnedDevices(entries []agent.DeviceEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, entry := range entries {
		zone := toInt(parseZoneKey(entry.Address))
		zoneByte := byte(zone)
		if _, exists := a.devices[zoneByte]; exists {
			continue
		}
		dev := &LGAPDevice{
			Zone:   zoneByte,
			UnitID: entry.Name,
			Online: false,
			State:  &LGAPDeviceState{},
			Source: "pinned",
		}
		a.devices[zoneByte] = dev
		if entry.Name != "" {
			a.deviceIDs[entry.Name] = zoneByte
		}
		a.logger.Info("lgap: 고정 설치 디바이스 등록",
			"zone", fmt.Sprintf("0x%02X", zoneByte), "name", entry.Name)
	}
}

// setAllDevicesOffline 은 모든 디바이스를 오프라인으로 전환한다.
// 트랜스포트 연결이 끊어졌을 때 호출된다.
func (a *LGAPAgent) setAllDevicesOffline() {
	a.mu.Lock()
	var offlined int
	now := time.Now().UnixMilli()
	for _, dev := range a.devices {
		if dev.Online {
			dev.Online = false
			offlined++
			// SPEC-HVACR-CONNSTATE-001 E4/AC-15: bulk offline 은 device 당 개별 change
			// (offline) 메시지를 방출한다(배칭 금지, 두 번째 initial 아님, N9).
			// initial 미방출 device 는 순서 보장(E9)을 위해 skip.
			if dev.connInitialEmitted {
				a.emitConnectionLocked(dev, connTriggerChange, now)
			}
		}
	}
	a.mu.Unlock()

	if offlined > 0 {
		a.logger.Info("lgap: 통신 끊김, 디바이스 오프라인 전환", "count", offlined)
	}
}

// nonTempFieldsChangedLGAP 는 비온도 필드 (Power/Mode/TargetTemp/FanSpeed/ErrorCode)
// 중 하나라도 변경되었는지 검사한다 (v0.6.7).
func nonTempFieldsChangedLGAP(prev, current LGAPDeviceState) bool {
	return prev.Power != current.Power ||
		prev.Mode != current.Mode ||
		prev.TargetTemp != current.TargetTemp ||
		prev.FanSpeed != current.FanSpeed ||
		prev.ErrorCode != current.ErrorCode
}

// maxTempDeltaLGAP 는 온도 센서값 (RoomTemp + PipeInTemp + PipeOutTemp) 의
// 최대 |Δ| 를 반환한다 (v0.6.7).
func maxTempDeltaLGAP(prev, current LGAPDeviceState) float64 {
	d := absDeltaFloat32(prev.RoomTemp, current.RoomTemp)
	if x := absDeltaFloat32(prev.PipeInTemp, current.PipeInTemp); x > d {
		d = x
	}
	if x := absDeltaFloat32(prev.PipeOutTemp, current.PipeOutTemp); x > d {
		d = x
	}
	return d
}

// absDeltaFloat32 는 |a - b| 를 float64 로 반환한다.
func absDeltaFloat32(a, b float32) float64 {
	d := float64(a - b)
	if d < 0 {
		d = -d
	}
	return d
}

// lgapStateChanged 는 두 상태가 다른지 비교한다.
func lgapStateChanged(prev, current LGAPDeviceState) bool {
	if prev.Power != current.Power {
		return true
	}
	if prev.Mode != current.Mode {
		return true
	}
	if prev.TargetTemp != current.TargetTemp {
		return true
	}
	if prev.RoomTemp != current.RoomTemp {
		return true
	}
	if prev.FanSpeed != current.FanSpeed {
		return true
	}
	if prev.ErrorCode != current.ErrorCode {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// agent.Agent 인터페이스 메서드 (정보 조회)
// ---------------------------------------------------------------------------

// Configure 는 에이전트 설정을 업데이트한다.
func (a *LGAPAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("lgap configure: %w", err)
	}

	// Transport.Options에서 lgapConfig 재파싱
	if len(config.Transport.Options) > 0 {
		lgapCfg, err := parseLGAPConfig(config.Transport.Options)
		if err != nil {
			return fmt.Errorf("lgap configure: re-parse config: %w", err)
		}
		a.mu.Lock()
		a.lgapConfig = lgapCfg
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
func (a *LGAPAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *LGAPAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *LGAPAgent) Type() string {
	return "lgap"
}

// Info 는 에이전트 정보의 스냅샷을 반환한다.
func (a *LGAPAgent) Info() agent.AgentInfo {
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
		Type:      "lgap",
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
func (a *LGAPAgent) BufferInfo() (int, int) {
	return len(a.msgCh), cap(a.msgCh)
}

// Stats 는 통계 스냅샷을 반환한다.
func (a *LGAPAgent) Stats() agent.StatsSnapshot {
	s := a.stats.Snapshot()
	s.MsgBufferPending, s.MsgBufferCapacity = a.BufferInfo()
	return s
}

// State 는 디바이스 요약 상태를 반환한다.
// agent.StatefulAgent 인터페이스 구현 — detail=full API 응답에 포함된다.
func (a *LGAPAgent) State() map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()

	onlineCount := 0
	devices := make([]map[string]any, 0, len(a.devices))
	for zone, dev := range a.devices {
		if dev.Online {
			onlineCount++
		}
		d := map[string]any{
			"zone":      fmt.Sprintf("0x%02X", zone),
			"unit_id":   dev.UnitID,
			"device_id": agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, dev.UnitID),
			"online":    dev.Online,
		}
		if dev.State != nil {
			// SPEC-DEVICE-IDENTITY-001 후속: mode/fan_speed 는 hvac 통일 ID (int).
			// 어댑터 (lg/device.go) 와 동일 컨벤션.
			d["state"] = map[string]any{
				"power":               dev.State.Power,
				"mode":                hvac.ModeFromName(dev.State.Mode),
				"target_temperature":  dev.State.TargetTemp,
				"current_temperature": dev.State.RoomTemp,
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

	return result
}

// GetPersistableDevices 는 현재 메모리의 디바이스 중 영속 저장할 대상 디바이스만 반환한다.
// LGAP 는 auto-discovery 개념이 없으므로 "config" 또는 "bridge"(runtime 추가) Source 인 모든 디바이스를 반환한다.
// 반환된 DeviceEntry 배열은 agent.ParseDevices() 를 통해 다시 파싱할 수 있는 형식이다.
//
// 이 메서드는 add_device/remove_device 명령 후 저장소에 디바이스 목록을 persist 하기 위해
// internal/api/service/agent_adapter.go 의 ExecAgent 에서 호출된다.
func (a *LGAPAgent) GetPersistableDevices() []agent.DeviceEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var result []agent.DeviceEntry
	for zone, dev := range a.devices {
		// LGAP 는 auto 개념이 없으므로 모든 non-config 디바이스를 포함
		// (config 와 bridge/runtime-added 모두 영속화)
		// 중요: Name 필드는 항상 dev.UnitID 를 사용한다.
		// ParseDevices() 시 entry.Name → UnitID 로 매핑되므로,
		// 역으로 저장할 때는 UnitID → Name 으로 써야 round-trip 이 보존된다.
		result = append(result, agent.DeviceEntry{
			Address: fmt.Sprintf("0x%x", zone),
			Name:    dev.UnitID,
			Source:  dev.Source, // source 보존: 재시작 후에도 "bridge" 유지 → 삭제 가능
		})
	}
	return result
}

// ReceiveMessage 는 msgCh 에서 메시지를 수신한다.
// agent.MessageReceiver 인터페이스 구현.
func (a *LGAPAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case data := <-a.msgCh:
		a.logger.Debug("lgap: ReceiveMessage 전달",
			"bytes", len(data),
		)
		return data, nil
	case <-a.stopCh:
		return nil, fmt.Errorf("lgap: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
