package samsung

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// NASAAgent 는 Samsung NASA HVAC 에이전트이다.
// agent.Agent, agent.MessageReceiver 인터페이스를 구현한다.
type NASAAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig   agent.AgentConfig
	nasaConfig    NASAConfig
	devices       map[NASAAddress]*NASADevice
	deviceIDs     map[string]NASAAddress // device_id -> address 역참조
	transport     NASATransport
	protocol      NASAProtocol
	mu            sync.RWMutex
	seqNum        byte
	pollTicker    *time.Ticker
	notifyTicker  *time.Ticker
	lastStates    map[NASAAddress]NASADeviceState
	wg            sync.WaitGroup
	stopCh        chan struct{}
	msgCh         chan []byte // Bridge 메시지 (ReceiveMessage)
	stats         *agent.AgentStats
	logger        *slog.Logger
	startedAt     time.Time
	createdAt     time.Time
	paused        bool
	warnedUnknown map[NASAAddress]bool // 미등록 주소 최초 경고 여부

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

	// onDeviceStateChange 는 디바이스 상태 변경 시 호출되는 콜백이다.
	// agentName 과 deviceID (global ID) 를 인자로 받는다.
	onDeviceStateChange func(agentName, deviceID string)
}

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*NASAAgent)(nil)
var _ agent.MessageReceiver = (*NASAAgent)(nil)
var _ agent.StatefulAgent = (*NASAAgent)(nil)
var _ agent.BufferInfoProvider = (*NASAAgent)(nil)
var _ agent.TransportChecker = (*NASAAgent)(nil)

// TransportConnected 는 시리얼 트랜스포트의 실제 연결 상태를 반환한다.
func (a *NASAAgent) TransportConnected() bool {
	return a.transport.Available()
}

// DeviceProvider 는 이 에이전트의 디바이스를 device.DeviceProvider 로 노출한다.
func (a *NASAAgent) DeviceProvider() device.DeviceProvider {
	return NewNASADeviceProvider(a)
}

// SetDeviceStateChangeCallback 은 디바이스 상태 변경 시 호출되는 콜백을 등록한다.
func (a *NASAAgent) SetDeviceStateChangeCallback(fn func(agentName, deviceID string)) {
	a.onDeviceStateChange = fn
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

// NewNASAAgent 는 NASAAgent 팩토리 함수이다.
func NewNASAAgent(config agent.AgentConfig) (agent.Agent, error) {
	nasaConfig, err := parseNASAConfig(config.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("samsung-nasa agent: %w", err)
	}

	transport, err := NewNASATransport(nasaConfig.TransportType, config.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("samsung-nasa agent: %w", err)
	}

	protocol := NewNASAProtocol()

	a := &NASAAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("samsung-nasa")),
		nasaConfig:    nasaConfig,
		devices:       make(map[NASAAddress]*NASADevice),
		deviceIDs:     make(map[string]NASAAddress),
		transport:     transport,
		protocol:      protocol,
		lastStates:    make(map[NASAAddress]NASADeviceState),
		warnedUnknown: make(map[NASAAddress]bool),
		disconnectCh:  make(chan struct{}),
		stopCh:        make(chan struct{}),
		msgCh:         make(chan []byte, nasaConfig.MsgChannelSize),
		stateNotify:   make(chan struct{}, 1),
		stats:         agent.NewAgentStats(),
		logger:        agent.ResolveLogger(config),
		createdAt:     time.Now(),
	}

	// 설정에 정의된 디바이스 등록
	for _, entry := range nasaConfig.Devices {
		addr, parseErr := ParseNASAAddress(entry.Address)
		if parseErr != nil {
			return nil, fmt.Errorf("samsung-nasa agent: invalid device address %q: %w", entry.Address, parseErr)
		}
		devType := DetectDeviceType(addr)
		dev := &NASADevice{
			Address:  addr,
			Type:     devType,
			DeviceID: entry.Name,
			Online:   false,
			Source:   "config",
		}
		if devType == "indoor" {
			dev.State = &NASADeviceState{RawMessageSets: make(map[uint16][]byte)}
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
func (a *NASAAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("samsung-nasa init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("samsung-nasa init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("samsung-nasa init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	a.logger.Info("samsung-nasa: 에이전트 초기화 완료",
		"transport", a.nasaConfig.TransportType,
		"devices", len(a.devices),
	)

	return nil
}

// Start 는 트랜스포트를 열고 폴링/수신 루프를 시작한다.
func (a *NASAAgent) Start(ctx context.Context) error {
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
		a.logger.Warn("samsung-nasa: 트랜스포트 연결 실패, 재연결 대기", "error", err)
		a.wg.Add(1)
		go func() { defer a.wg.Done(); a.reconnectLoop() }()
	} else {
		// 연결 성공 시 정상 루프 시작
		a.wg.Add(2)
		go func() { defer a.wg.Done(); a.pollLoop() }()
		go func() { defer a.wg.Done(); a.receiveLoop() }()
	}

	a.mu.Lock()
	if a.nasaConfig.NotifyInterval > 0 {
		a.notifyTicker = time.NewTicker(a.nasaConfig.NotifyInterval)
	}
	a.mu.Unlock()

	a.logger.Info("samsung-nasa: 에이전트 시작 완료")
	return nil
}

// Stop 은 에이전트를 정지한다.
func (a *NASAAgent) Stop(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("samsung-nasa stop: %w", err)
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
		a.logger.Warn("samsung-nasa: transport close error", "error", err)
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
		return fmt.Errorf("samsung-nasa stop: %w", err)
	}

	return nil
}

// Pause 는 Running -> Paused 로 전환한다.
func (a *NASAAgent) Pause(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("samsung-nasa pause: %w", err)
	}
	a.mu.Lock()
	a.paused = true
	a.mu.Unlock()
	return nil
}

// Resume 은 Paused -> Running 으로 전환한다.
func (a *NASAAgent) Resume(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("samsung-nasa resume: %w", err)
	}
	a.mu.Lock()
	a.paused = false
	a.mu.Unlock()
	return nil
}

// Health 는 에이전트의 건강 상태를 반환한다.
func (a *NASAAgent) Health() agent.HealthStatus {
	now := time.Now()
	state := a.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		return agent.HealthStatus{
			Status:    agent.HealthHealthy,
			LastCheck: now,
			Message:   "samsung-nasa agent is running",
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "samsung-nasa agent is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("samsung-nasa agent is in %s state", state),
		}
	}
}

// Process 는 JSON 명령을 디스패치하여 처리한다.
// 통계는 개별 커맨드 핸들러에서 의미 있는 데이터 전송 시에만 기록한다.
// 폴링 쿼리(get_recent_states 등)는 실제 데이터가 있을 때만 카운트한다.
func (a *NASAAgent) Process(data []byte) ([]byte, error) {
	var req processRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("samsung-nasa process: invalid JSON: %w", err)
	}

	switch req.Command {
	case "set_power":
		return a.processSetPower(&req)
	case "set_mode":
		return a.processSetMode(&req)
	case "set_temperature":
		return a.processSetTemperature(&req)
	case "set_fan_speed":
		return a.processSetFanSpeed(&req)
	case "set_multiple":
		return a.processSetMultiple(&req)
	case "get_state":
		return a.processGetState(&req)
	case "get_all_states":
		return a.processGetAllStates()
	case "get_recent_states":
		return a.processGetRecentStates(&req)
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
func (a *NASAAgent) processSetPower(req *processRequest) ([]byte, error) {
	addr, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}
	if !dev.Online {
		return nil, ErrDeviceOffline
	}

	power, ok := req.Params["power"].(bool)
	if !ok {
		return nil, fmt.Errorf("samsung-nasa: power parameter must be boolean")
	}

	a.logger.Debug("samsung-nasa: set_power 요청", "device", dev.DeviceID, "addr", addr.String(), "power", power)

	var val byte
	if power {
		val = 0x01
	}

	sets := []NASAMessageSet{{Index: MsgPower, Value: []byte{val}}}
	if err := a.sendControlCommand(addr, sets); err != nil {
		return nil, err
	}

	// 제어 명령 후 즉시 상태 조회를 전송하여 실제 하드웨어 상태를 빠르게 반영한다.
	a.sendImmediateStatusQuery(addr)

	return a.buildSuccessResponse(addr, dev.DeviceID, map[string]any{"power": power})
}

// processSetMode 는 운전 모드 변경 명령을 처리한다.
func (a *NASAAgent) processSetMode(req *processRequest) ([]byte, error) {
	addr, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}
	if !dev.Online {
		return nil, ErrDeviceOffline
	}

	modeStr, ok := req.Params["mode"].(string)
	if !ok {
		return nil, fmt.Errorf("samsung-nasa: mode parameter must be string")
	}

	modeVal, exists := StringToMode[modeStr]
	if !exists {
		return nil, ErrInvalidMode
	}

	a.logger.Debug("samsung-nasa: set_mode 요청", "device", dev.DeviceID, "addr", addr.String(), "mode", modeStr)

	sets := []NASAMessageSet{{Index: MsgMode, Value: []byte{modeVal}}}
	if err := a.sendControlCommand(addr, sets); err != nil {
		return nil, err
	}

	a.sendImmediateStatusQuery(addr)

	return a.buildSuccessResponse(addr, dev.DeviceID, map[string]any{"mode": modeStr})
}

// processSetTemperature 는 목표 온도 설정 명령을 처리한다.
func (a *NASAAgent) processSetTemperature(req *processRequest) ([]byte, error) {
	addr, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}
	if !dev.Online {
		return nil, ErrDeviceOffline
	}

	tempVal, ok := req.Params["target_temp"].(float64)
	if !ok {
		return nil, fmt.Errorf("samsung-nasa: target_temp parameter must be number")
	}

	if tempVal < 16.0 || tempVal > 30.0 {
		return nil, ErrTemperatureOutOfRange
	}

	a.logger.Debug("samsung-nasa: set_temperature 요청", "device", dev.DeviceID, "addr", addr.String(), "target_temp", tempVal)

	encoded := EncodeTemperature(float32(tempVal))
	sets := []NASAMessageSet{{Index: MsgTargetTemp, Value: []byte{byte(encoded >> 8), byte(encoded & 0xFF)}}}
	if err := a.sendControlCommand(addr, sets); err != nil {
		return nil, err
	}

	a.sendImmediateStatusQuery(addr)

	return a.buildSuccessResponse(addr, dev.DeviceID, map[string]any{"target_temp": tempVal})
}

// processSetFanSpeed 는 팬 속도 변경 명령을 처리한다.
func (a *NASAAgent) processSetFanSpeed(req *processRequest) ([]byte, error) {
	addr, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}
	if !dev.Online {
		return nil, ErrDeviceOffline
	}

	speedStr, ok := req.Params["fan_speed"].(string)
	if !ok {
		return nil, fmt.Errorf("samsung-nasa: fan_speed parameter must be string")
	}

	speedVal, exists := StringToFanSpeed[speedStr]
	if !exists {
		return nil, ErrInvalidFanSpeed
	}

	a.logger.Debug("samsung-nasa: set_fan_speed 요청", "device", dev.DeviceID, "addr", addr.String(), "fan_speed", speedStr)

	sets := []NASAMessageSet{{Index: MsgFanSpeed, Value: []byte{speedVal}}}
	if err := a.sendControlCommand(addr, sets); err != nil {
		return nil, err
	}

	a.sendImmediateStatusQuery(addr)

	return a.buildSuccessResponse(addr, dev.DeviceID, map[string]any{"fan_speed": speedStr})
}

// processSetMultiple 는 복수 설정 변경 명령을 처리한다.
func (a *NASAAgent) processSetMultiple(req *processRequest) ([]byte, error) {
	addr, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}
	if !dev.Online {
		return nil, ErrDeviceOffline
	}

	var sets []NASAMessageSet
	result := make(map[string]any)

	// power (nil이면 건너뜀)
	if powerVal, ok := req.Params["power"]; ok && powerVal != nil {
		power, _ := powerVal.(bool)
		var val byte
		if power {
			val = 0x01
		}
		sets = append(sets, NASAMessageSet{Index: MsgPower, Value: []byte{val}})
		result["power"] = power
	}

	// mode (nil이면 건너뜀)
	if modeVal, ok := req.Params["mode"]; ok && modeVal != nil {
		modeStr, _ := modeVal.(string)
		modeByte, exists := StringToMode[modeStr]
		if !exists {
			return nil, ErrInvalidMode
		}
		sets = append(sets, NASAMessageSet{Index: MsgMode, Value: []byte{modeByte}})
		result["mode"] = modeStr
	}

	// target_temp (nil이면 건너뜀)
	if tempVal, ok := req.Params["target_temp"]; ok && tempVal != nil {
		temp, _ := tempVal.(float64)
		if temp < 16.0 || temp > 30.0 {
			return nil, ErrTemperatureOutOfRange
		}
		encoded := EncodeTemperature(float32(temp))
		sets = append(sets, NASAMessageSet{Index: MsgTargetTemp, Value: []byte{byte(encoded >> 8), byte(encoded & 0xFF)}})
		result["target_temp"] = temp
	}

	// fan_speed (nil이면 건너뜀)
	if speedVal, ok := req.Params["fan_speed"]; ok && speedVal != nil {
		speedStr, _ := speedVal.(string)
		speedByte, exists := StringToFanSpeed[speedStr]
		if !exists {
			return nil, ErrInvalidFanSpeed
		}
		sets = append(sets, NASAMessageSet{Index: MsgFanSpeed, Value: []byte{speedByte}})
		result["fan_speed"] = speedStr
	}

	if len(sets) == 0 {
		return nil, fmt.Errorf("samsung-nasa: set_multiple requires at least one setting")
	}

	if err := a.sendControlCommand(addr, sets); err != nil {
		return nil, err
	}

	return a.buildSuccessResponse(addr, dev.DeviceID, result)
}

// ---------------------------------------------------------------------------
// 상태 조회 명령 처리
// ---------------------------------------------------------------------------

// processGetState 는 단일 디바이스 상태 조회 명령을 처리한다.
func (a *NASAAgent) processGetState(req *processRequest) ([]byte, error) {
	addr, dev, err := a.resolveDevice(req)
	if err != nil {
		return nil, err
	}

	resp := map[string]any{
		"status":      "ok",
		"address":     addr.String(),
		"device_id":   dev.DeviceID,
		"device_type": dev.Type,
		"online":      dev.Online,
	}

	if dev.State != nil {
		resp["state"] = dev.State.StateForJSON(a.nasaConfig.IncludeRawMessageSets)
	}
	if !dev.LastSeen.IsZero() {
		resp["last_seen"] = dev.LastSeen.Format(time.RFC3339)
	}

	return json.Marshal(resp)
}

// processGetAllStates 는 전체 디바이스 상태 조회 명령을 처리한다.
// atomic 캐시에서 즉시 반환하여 write lock 경합을 회피한다.
func (a *NASAAgent) processGetAllStates() ([]byte, error) {
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
func (a *NASAAgent) buildAllStatesJSON() ([]byte, error) {
	var devices []map[string]any
	for addr, dev := range a.devices {
		d := map[string]any{
			"address":     addr.String(),
			"device_id":   dev.DeviceID,
			"device_type": dev.Type,
			"online":      dev.Online,
		}
		if dev.State != nil {
			d["state"] = dev.State.StateForJSON(a.nasaConfig.IncludeRawMessageSets)
		}
		if !dev.LastSeen.IsZero() {
			d["last_seen"] = dev.LastSeen.Format(time.RFC3339)
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
func (a *NASAAgent) pushRecentSnapshot(addr NASAAddress) {
	dev, ok := a.devices[addr]
	if !ok {
		return
	}

	d := map[string]any{
		"address":     addr.String(),
		"device_id":   dev.DeviceID,
		"device_type": dev.Type,
		"online":      dev.Online,
	}
	if dev.State != nil {
		d["state"] = dev.State.StateForJSON(a.nasaConfig.IncludeRawMessageSets)
	}
	if !dev.LastSeen.IsZero() {
		d["last_seen"] = dev.LastSeen.Format(time.RFC3339)
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
func (a *NASAAgent) processGetRecentStates(req *processRequest) ([]byte, error) {
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
func (a *NASAAgent) processAddDevice(req *processRequest) ([]byte, error) {
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
		return nil, fmt.Errorf("samsung-nasa: address is required for add_device")
	}
	addr, err := ParseNASAAddress(address)
	if err != nil {
		return nil, fmt.Errorf("samsung-nasa: invalid address: %w", err)
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

	dev := &NASADevice{
		Address:  addr,
		DeviceID: deviceID,
		Name:     name,
		Type:     devType,
		Online:   false,
		Source:   "bridge",
	}
	if devType == "indoor" {
		dev.State = &NASADeviceState{RawMessageSets: make(map[uint16][]byte)}
	}

	a.devices[addr] = dev
	if deviceID != "" {
		a.deviceIDs[deviceID] = addr
	}

	// 이벤트 전송 (락 밖에서 하면 좋지만 non-blocking 이므로 무방)
	regData := map[string]any{
		"address":     addr.String(),
		"device_id":   deviceID,
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
		"device_id":   deviceID,
		"name":        name,
		"device_type": devType,
	}
	return json.Marshal(resp)
}

// processRemoveDevice 는 디바이스 제거 명령을 처리한다.
func (a *NASAAgent) processRemoveDevice(req *processRequest) ([]byte, error) {
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

	if dev.Source == "config" {
		return nil, ErrConfigDeviceProtected
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// deviceIDs 맵에서도 제거
	if dev.DeviceID != "" {
		delete(a.deviceIDs, dev.DeviceID)
	}
	delete(a.devices, addr)

	unregData := map[string]any{
		"address":   addr.String(),
		"device_id": dev.DeviceID,
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
		"device_id": dev.DeviceID,
	}
	return json.Marshal(resp)
}

// processListDevices 는 디바이스 목록 조회 명령을 처리한다.
func (a *NASAAgent) processListDevices() ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var devices []map[string]any
	for addr, dev := range a.devices {
		devices = append(devices, map[string]any{
			"address":     addr.String(),
			"device_id":   dev.DeviceID,
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
func (a *NASAAgent) resolveDevice(req *processRequest) (NASAAddress, *NASADevice, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var addr NASAAddress

	if req.DeviceID != "" {
		resolved, ok := a.deviceIDs[req.DeviceID]
		if !ok {
			return NASAAddress{}, nil, ErrDeviceIDNotFound
		}
		addr = resolved
	} else if req.Address != "" {
		var err error
		addr, err = ParseNASAAddress(req.Address)
		if err != nil {
			return NASAAddress{}, nil, fmt.Errorf("samsung-nasa: invalid address: %w", err)
		}
	} else {
		return NASAAddress{}, nil, fmt.Errorf("samsung-nasa: address or device_id is required")
	}

	dev, ok := a.devices[addr]
	if !ok {
		return NASAAddress{}, nil, ErrDeviceNotFound
	}

	return addr, dev, nil
}

// sendControlCommand 는 제어 프레임을 빌드하고 트랜스포트로 전송한다.
// 부저 메시지셋(0x4050)을 자동 추가한다 (On=0x00, Off=0x01).
func (a *NASAAgent) sendControlCommand(addr NASAAddress, sets []NASAMessageSet) error {
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
		if a.nasaConfig.BuzzerOnControl {
			buzzerVal = 0x00 // On
		}
		sets = append(sets, NASAMessageSet{Index: MsgBuzzer, Value: []byte{buzzerVal}})
	}

	// 제어 명령 전송 로그
	setNames := make([]string, 0, len(sets))
	for _, s := range sets {
		setNames = append(setNames, fmt.Sprintf("0x%04X(%d bytes)", s.Index, len(s.Value)))
	}
	a.logger.Debug("samsung-nasa: 제어 명령 전송", "addr", addr.String(), "sets", setNames)

	seq := a.nextSeqNum()
	frame, err := a.protocol.BuildControlCommand(addr, seq, sets)
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		a.logger.Error("samsung-nasa: 제어 프레임 빌드 실패", "addr", addr.String(), "error", err)
		return fmt.Errorf("samsung-nasa: build control command failed: %w", err)
	}

	if err := a.transport.Send(frame); err != nil {
		a.stats.IncrExternalMessagesErrored()
		a.logger.Error("samsung-nasa: 제어 명령 전송 실패", "addr", addr.String(), "error", err)
		return fmt.Errorf("samsung-nasa: send failed: %w", err)
	}

	a.stats.IncrExternalMessagesSent()
	a.stats.AddBytesWritten(int64(len(frame)))
	a.stats.UpdateLastActivity()
	a.logger.Debug("samsung-nasa: 제어 명령 전송 완료", "addr", addr.String(), "seq", seq, "frame_size", len(frame))
	return nil
}

// sendImmediateStatusQuery 는 제어 명령 직후 해당 디바이스의 상태를 반복 조회한다.
// 설정된 간격(StatusQueryDelay)과 횟수(StatusQueryRetries)에 따라 상태를 조회한다.
// 새 제어 명령이 발행되면 이전 goroutine 을 취소하여 시리얼 포트 경합을 방지한다.
func (a *NASAAgent) sendImmediateStatusQuery(addr NASAAddress) {
	// 이전 상태 조회 goroutine 취소
	a.mu.Lock()
	if a.statusQueryCancel != nil {
		a.statusQueryCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.statusQueryCancel = cancel
	a.mu.Unlock()

	delay := a.nasaConfig.StatusQueryDelay
	retries := a.nasaConfig.StatusQueryRetries

	go func() {
		defer cancel()
		for i := 0; i < retries; i++ {
			select {
			case <-ctx.Done():
				a.logger.Debug("samsung-nasa: 상태 조회 취소 (새 제어 명령)", "addr", addr.String(), "attempt", i+1)
				return
			case <-time.After(delay):
			}
			seq := a.nextSeqNum()
			frame, err := a.protocol.BuildStatusQuery(addr, seq)
			if err != nil {
				a.logger.Debug("samsung-nasa: 즉시 상태 조회 빌드 실패", "addr", addr.String(), "error", err)
				return
			}
			if err := a.transport.Send(frame); err != nil {
				a.logger.Debug("samsung-nasa: 즉시 상태 조회 전송 실패", "addr", addr.String(), "error", err)
				return
			}
			a.stats.IncrExternalMessagesSent()
			a.logger.Debug("samsung-nasa: 제어 후 상태 조회 전송", "addr", addr.String(), "attempt", i+1, "seq", seq, "delay", delay)
		}
	}()
}

// buildSuccessResponse 는 제어 명령 성공 응답 JSON 을 생성한다.
func (a *NASAAgent) buildSuccessResponse(addr NASAAddress, deviceID string, result map[string]any) ([]byte, error) {
	resp := map[string]any{
		"status":    "ok",
		"address":   addr.String(),
		"device_id": deviceID,
		"result":    result,
	}
	// 제어 명령 응답: 내부 수신 1 + 내부 송신 1
	a.stats.IncrInternalMessagesReceived()
	a.stats.IncrInternalMessagesSent()
	return json.Marshal(resp)
}

// nextSeqNum 은 시퀀스 번호를 증가시키고 반환한다.
func (a *NASAAgent) nextSeqNum() byte {
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
func (a *NASAAgent) sendEvent(eventType string, data map[string]any) {
	evt := map[string]any{"type": eventType}
	for k, v := range data {
		evt[k] = v
	}
	b, err := json.Marshal(evt)
	if err != nil {
		a.logger.Warn("samsung-nasa: event marshal failed", "error", err)
		return
	}
	select {
	case a.msgCh <- b:
		a.logger.Debug("samsung-nasa: 이벤트 msgCh 전송 성공",
			"type", eventType,
			"chLen", len(a.msgCh),
			"chCap", cap(a.msgCh),
		)
	default:
		a.logger.Warn("samsung-nasa: msgCh full, dropping event", "type", eventType)
	}
}

// incrementErrorCount 는 디바이스의 에러 카운트를 증가시키고,
// OfflineThreshold 에 도달하면 디바이스를 오프라인으로 전환한다.
func (a *NASAAgent) incrementErrorCount(addr NASAAddress) {
	a.mu.Lock()
	defer a.mu.Unlock()

	dev, ok := a.devices[addr]
	if !ok {
		return
	}

	dev.ErrorCount++
	if dev.ErrorCount >= a.nasaConfig.OfflineThreshold && dev.Online {
		dev.Online = false
		a.sendEventLocked("device_offline", map[string]any{
			"address":   addr.String(),
			"device_id": dev.DeviceID,
		})
		a.logger.Warn("samsung-nasa: 디바이스 오프라인",
			"address", addr.String(),
			"device_id", dev.DeviceID,
			"error_count", dev.ErrorCount,
		)
	}
}

// sendEventLocked 는 sendEvent 와 동일하지만 이미 락이 잡혀 있을 때 사용한다.
func (a *NASAAgent) sendEventLocked(eventType string, data map[string]any) {
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
func (a *NASAAgent) ListDevices() []NASADevice {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]NASADevice, 0, len(a.devices))
	for _, dev := range a.devices {
		result = append(result, *dev)
	}
	return result
}

// GetDeviceState 는 지정된 주소의 디바이스 상태를 반환한다.
func (a *NASAAgent) GetDeviceState(addr NASAAddress) (*NASADeviceState, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	dev, ok := a.devices[addr]
	if !ok {
		return nil, ErrDeviceNotFound
	}
	return dev.State, nil
}

// GetDeviceByID 는 device_id 로 디바이스를 조회한다.
func (a *NASAAgent) GetDeviceByID(deviceID string) (*NASADevice, error) {
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
func (a *NASAAgent) reconnectLoop() {
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

	// 재연결 시작 이벤트
	a.sendEvent("transport_reconnecting", map[string]any{
		"timestamp": time.Now().Format(time.RFC3339),
	})

	baseInterval := a.nasaConfig.ReconnectInterval
	maxBackoff := a.nasaConfig.MaxReconnectBackoff
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
			a.logger.Info("samsung-nasa: 트랜스포트 재연결 성공",
				"attempts", attempt+1,
				"downtime", time.Since(disconnectedAt).Round(time.Second).String(),
			)
			a.sendEvent("transport_reconnected", map[string]any{
				"attempt_count":    attempt + 1,
				"downtime_seconds": int(time.Since(disconnectedAt).Seconds()),
				"timestamp":        time.Now().Format(time.RFC3339),
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
			a.logger.Warn("samsung-nasa: 트랜스포트 재연결 시도 중", "error", err)
		} else {
			a.logger.Debug("samsung-nasa: 트랜스포트 재연결 시도", "attempt", attempt+1, "error", err)
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
func (a *NASAAgent) pollLoop() {
	ticker := time.NewTicker(a.nasaConfig.PollInterval)
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
			a.logger.Debug("samsung-nasa: pollLoop 연결 끊김으로 종료")
			return
		case <-ticker.C:
			a.mu.RLock()
			paused := a.paused
			// 디바이스 주소 스냅샷 복사
			addrs := make([]NASAAddress, 0, len(a.devices))
			if !paused {
				for addr := range a.devices {
					addrs = append(addrs, addr)
				}
			}
			a.mu.RUnlock()
			if paused {
				continue
			}

			a.logger.Debug("samsung-nasa: 폴링 시작", "devices", len(addrs))
			for _, addr := range addrs {
				seq := a.nextSeqNum()
				frame, err := a.protocol.BuildStatusQuery(addr, seq)
				if err != nil {
					a.logger.Warn("samsung-nasa: build status query failed", "addr", addr.String(), "error", err)
					a.incrementErrorCount(addr)
					continue
				}
				if err := a.transport.Send(frame); err != nil {
					a.logger.Warn("samsung-nasa: send status query failed", "addr", addr.String(), "error", err)
					a.incrementErrorCount(addr)
					continue
				}
				a.logger.Debug("samsung-nasa: 상태 쿼리 전송", "addr", addr.String(), "seq", seq)
				a.stats.IncrExternalMessagesSent()
			}
		}
	}
}

// receiveLoop 는 트랜스포트에서 데이터를 수신하고 디바이스 상태를 업데이트한다.
func (a *NASAAgent) receiveLoop() {
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
				a.logger.Warn("samsung-nasa: 트랜스포트 연결 끊김 감지", "error", err)
				a.sendEvent("transport_disconnected", map[string]any{
					"reason":    err.Error(),
					"timestamp": time.Now().Format(time.RFC3339),
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

			// 타임아웃 등 일시적 에러 — 계속 수신
			a.logger.Debug("samsung-nasa: receive error (transient)", "error", err)
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

			// a.logger.Debug("samsung-nasa: 프레임 추출 완료",
			// 	"frameBytes", len(frame),
			// )

			msg, err := a.protocol.Decode(frame)
			if err != nil {
				// unsupported 인덱스로 인한 디코드 에러는 설정에 따라 로그 억제
				if errors.Is(err, ErrInvalidMessageSetIndex) && len(a.nasaConfig.UnsupportedMsgSets) > 0 {
					if !a.nasaConfig.LogUnsupportedMsgSets {
						a.stats.IncrExternalMessagesErrored()
						continue
					}
				}
				// log_decode_errors 옵션이 false 면 일반 decode error 도 억제 (운영 환경 noise 방지).
				// 2026-05-14 hotfix: invalid message set index 등 빈번한 디코드 오류로 인한 로그 폭주 회피.
				if a.nasaConfig.LogDecodeErrors {
					a.logger.Warn("samsung-nasa: decode error", "error", err)
				}
				a.stats.IncrExternalMessagesErrored()
				continue
			}

			// a.logger.Debug("samsung-nasa: 메시지 디코드 성공",
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
func (a *NASAAgent) handleMessage(msg *NASAMessage) {
	srcAddr := msg.SourceAddr

	// a.logger.Debug("samsung-nasa: handleMessage 진입",
	// 	"source", srcAddr.String(),
	// )

	a.mu.Lock()
	defer a.mu.Unlock()

	dev, ok := a.devices[srcAddr]
	if !ok {
		if a.nasaConfig.AutoDiscovery {
			devType := DetectDeviceType(srcAddr)
			dev = &NASADevice{
				Address:  srcAddr,
				Type:     devType,
				Online:   true,
				LastSeen: time.Now(),
				Source:   "auto",
			}
			if devType == "indoor" {
				dev.State = &NASADeviceState{RawMessageSets: make(map[uint16][]byte)}
			}
			a.devices[srcAddr] = dev
			evtData := map[string]any{
				"address":     srcAddr.String(),
				"device_id":   dev.DeviceID,
				"device_type": devType,
			}
			if dev.State != nil {
				evtData["state"] = dev.State.StateForJSON(false)
			}
			a.sendEventLocked("device_discovered", evtData)
		} else {
			if !a.warnedUnknown[srcAddr] {
				a.warnedUnknown[srcAddr] = true
				a.logger.Warn("samsung-nasa: unknown device (이후 debug로 전환)", "address", srcAddr.String())
			} else {
				a.logger.Debug("samsung-nasa: unknown device", "address", srcAddr.String())
			}
			return
		}
	}

	wasOffline := !dev.Online
	dev.Online = true
	dev.LastSeen = time.Now()
	dev.ErrorCount = 0

	if wasOffline {
		onlineData := map[string]any{
			"address":   srcAddr.String(),
			"device_id": dev.DeviceID,
		}
		if dev.State != nil {
			onlineData["state"] = dev.State.StateForJSON(false)
		}
		a.sendEventLocked("device_online", onlineData)
	}

	// 실내기 상태 업데이트
	if dev.State != nil && len(msg.MessageSets) > 0 {
		sets := msg.MessageSets
		if len(a.nasaConfig.UnsupportedMsgSets) > 0 {
			sets = a.filterMessageSets(sets, srcAddr)
		}
		if len(sets) == 0 {
			return
		}

		// 이전 상태 저장
		prevState := a.lastStates[srcAddr]

		dev.State.UpdateFromMessageSets(sets)

		// 변경 감지
		currentState := *dev.State
		if stateChanged(prevState, currentState) {
			a.lastStates[srcAddr] = currentState
			a.logger.Debug("samsung-nasa: 상태 변경 감지",
				"device", dev.DeviceID, "addr", srcAddr.String(),
				"power", currentState.Power, "mode", currentState.Mode,
				"target_temp", currentState.TargetTemp, "fan_speed", currentState.FanSpeed)
			a.sendEventLocked("device_state_changed", map[string]any{
				"address":   srcAddr.String(),
				"device_id": dev.DeviceID,
				"state":     (&currentState).StateForJSON(false),
			})
			// WebSocket 브로드캐스트 콜백
			// 주의: a.Name()은 a.mu.RLock()을 호출하므로 write lock 보유 중
			// 재진입 데드락을 피하려면 a.agentConfig.Name을 직접 참조해야 한다.
			if fn := a.onDeviceStateChange; fn != nil {
				agentName := a.agentConfig.Name
				globalID := fmt.Sprintf("%s:%s", agentName, srcAddr.String())
				a.logger.Debug("samsung-nasa: WebSocket 상태 변경 브로드캐스트", "agent", agentName, "globalID", globalID)
				go fn(agentName, globalID)
			}
		}
	}

	a.stats.UpdateLastActivity()

	// processGetAllStates 용 atomic 캐시 갱신 (write lock 내에서 실행)
	if b, err := a.buildAllStatesJSON(); err == nil {
		a.cachedAllStates.Store(b)
	}

	// get_recent_states 용 스냅샷 링버퍼에 push (변경된 디바이스만)
	a.pushRecentSnapshot(srcAddr)

	// 폴링 노드에 새 데이터 도착 알림 (non-blocking)
	select {
	case a.stateNotify <- struct{}{}:
	default:
	}
}

// filterMessageSets 는 unsupported 목록에 포함된 메시지 셋을 필터링한다.
func (a *NASAAgent) filterMessageSets(sets []NASAMessageSet, addr NASAAddress) []NASAMessageSet {
	filtered := make([]NASAMessageSet, 0, len(sets))
	for _, ms := range sets {
		if a.nasaConfig.UnsupportedMsgSets[ms.Index] {
			if a.nasaConfig.LogUnsupportedMsgSets {
				a.logger.Debug("samsung-nasa: unsupported message set filtered",
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

// stateChanged 는 두 상태가 다른지 비교한다.
func stateChanged(prev, current NASADeviceState) bool {
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
func (a *NASAAgent) FrameNotifyCh() <-chan struct{} {
	return a.stateNotify
}

// ---------------------------------------------------------------------------
// agent.Agent 인터페이스 메서드 (정보 조회)
// ---------------------------------------------------------------------------

// Configure 는 에이전트 설정을 업데이트한다.
func (a *NASAAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("samsung-nasa configure: %w", err)
	}

	// Transport.Options에서 nasaConfig 재파싱
	if len(config.Transport.Options) > 0 {
		nasaCfg, err := parseNASAConfig(config.Transport.Options)
		if err != nil {
			return fmt.Errorf("samsung-nasa configure: re-parse config: %w", err)
		}

		a.mu.Lock()
		oldPoll := a.nasaConfig.PollInterval
		oldNotify := a.nasaConfig.NotifyInterval
		a.nasaConfig = nasaCfg
		a.agentConfig = config

		// 실행 중인 ticker 재설정 (간격이 변경된 경우)
		if nasaCfg.PollInterval != oldPoll && a.pollTicker != nil {
			a.pollTicker.Reset(nasaCfg.PollInterval)
			a.logger.Info("poll interval 변경 적용", "old", oldPoll, "new", nasaCfg.PollInterval)
		}
		if nasaCfg.NotifyInterval != oldNotify && a.notifyTicker != nil {
			if nasaCfg.NotifyInterval > 0 {
				a.notifyTicker.Reset(nasaCfg.NotifyInterval)
				a.logger.Info("notify interval 변경 적용", "old", oldNotify, "new", nasaCfg.NotifyInterval)
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
func (a *NASAAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *NASAAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *NASAAgent) Type() string {
	return "samsung-nasa"
}

// Info 는 에이전트 정보의 스냅샷을 반환한다.
func (a *NASAAgent) Info() agent.AgentInfo {
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
		Type:      "samsung-nasa",
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
func (a *NASAAgent) BufferInfo() (int, int) {
	return len(a.msgCh), cap(a.msgCh)
}

// Stats 는 통계 스냅샷을 반환한다.
func (a *NASAAgent) Stats() agent.StatsSnapshot {
	s := a.stats.Snapshot()
	s.MsgBufferPending, s.MsgBufferCapacity = a.BufferInfo()
	return s
}

// State 는 디바이스 요약 상태를 반환한다.
// agent.StatefulAgent 인터페이스 구현 — detail=full API 응답에 포함된다.
func (a *NASAAgent) State() map[string]any {
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
			"device_id":   dev.DeviceID,
			"device_type": dev.Type,
			"online":      dev.Online,
		}
		if dev.State != nil {
			d["state"] = map[string]any{
				"power":        dev.State.Power,
				"mode":         dev.State.Mode,
				"target_temp":  dev.State.TargetTemp,
				"current_temp": dev.State.CurrentTemp,
				"fan_speed":    dev.State.FanSpeed,
			}
		}
		if !dev.LastSeen.IsZero() {
			d["last_seen"] = dev.LastSeen.Format(time.RFC3339)
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

	if len(a.nasaConfig.UnsupportedMsgSets) > 0 {
		sets := make([]string, 0, len(a.nasaConfig.UnsupportedMsgSets))
		for idx := range a.nasaConfig.UnsupportedMsgSets {
			sets = append(sets, fmt.Sprintf("0x%04X", idx))
		}
		result["unsupported_msg_sets"] = sets
	}
	return result
}

// ReceiveMessage 는 msgCh 에서 메시지를 수신한다.
// agent.MessageReceiver 인터페이스 구현.
func (a *NASAAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case data := <-a.msgCh:
		a.logger.Debug("samsung-nasa: ReceiveMessage 전달",
			"bytes", len(data),
			"preview", truncateForLog(data, 120),
		)
		return data, nil
	case <-a.stopCh:
		return nil, fmt.Errorf("samsung-nasa: stopped")
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
