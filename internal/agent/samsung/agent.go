package samsung

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// NASAAgent 는 Samsung NASA HVAC 에이전트이다.
// agent.Agent, agent.MessageReceiver 인터페이스를 구현한다.
type NASAAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig  agent.AgentConfig
	nasaConfig   NASAConfig
	devices      map[NASAAddress]*NASADevice
	deviceIDs    map[string]NASAAddress // device_id -> address 역참조
	transport    NASATransport
	protocol     NASAProtocol
	mu           sync.RWMutex
	seqNum       byte
	pollTicker   *time.Ticker
	notifyTicker *time.Ticker
	lastStates   map[NASAAddress]NASADeviceState
	stopCh       chan struct{}
	msgCh        chan []byte // Bridge 메시지 (ReceiveMessage)
	stats         *agent.AgentStats
	logger        *slog.Logger
	startedAt     time.Time
	createdAt     time.Time
	paused        bool
	warnedUnknown map[NASAAddress]bool // 미등록 주소 최초 경고 여부

	disconnectCh      chan struct{} // 연결 끊김 시그널 (receiveLoop → pollLoop)
	reconnectMu       sync.Mutex   // reconnecting 상태 보호
	isReconnecting    bool         // 재연결 진행 중 여부
	reconnectAttempts int          // 현재 재연결 시도 횟수

	statusQueryCancel context.CancelFunc // 진행 중인 상태 조회 goroutine 취소

	// onDeviceStateChange 는 디바이스 상태 변경 시 호출되는 콜백이다.
	// agentName 과 deviceID (global ID) 를 인자로 받는다.
	onDeviceStateChange func(agentName, deviceID string)
}

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*NASAAgent)(nil)
var _ agent.MessageReceiver = (*NASAAgent)(nil)
var _ agent.StatefulAgent = (*NASAAgent)(nil)
var _ agent.BufferInfoProvider = (*NASAAgent)(nil)

// DeviceProvider 는 이 에이전트의 디바이스를 device.DeviceProvider 로 노출한다.
func (a *NASAAgent) DeviceProvider() device.DeviceProvider {
	return NewNASADeviceProvider(a)
}

// SetDeviceStateChangeCallback 은 디바이스 상태 변경 시 호출되는 콜백을 등록한다.
func (a *NASAAgent) SetDeviceStateChangeCallback(fn func(agentName, deviceID string)) {
	a.onDeviceStateChange = fn
}

// processRequest 는 Process 메서드의 JSON 요청 구조체이다.
type processRequest struct {
	Command    string         `json:"command"`
	Address    string         `json:"address,omitempty"`
	DeviceID   string         `json:"device_id,omitempty"`
	Params     map[string]any `json:"params,omitempty"`
	DeviceType string         `json:"device_type,omitempty"`
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
		stats:         agent.NewAgentStats(),
		logger:        agent.ResolveLogger(config),
		createdAt:     time.Now(),
	}

	// 설정에 정의된 디바이스 주소 등록
	for _, addrStr := range nasaConfig.DeviceAddresses {
		addr, parseErr := ParseNASAAddress(addrStr)
		if parseErr != nil {
			return nil, fmt.Errorf("samsung-nasa agent: invalid device address %q: %w", addrStr, parseErr)
		}
		devType := DetectDeviceType(addr)
		dev := &NASADevice{
			Address: addr,
			Type:    devType,
			Online:  false,
			Source:  "config",
		}
		if devType == "indoor" {
			dev.State = &NASADeviceState{RawMessageSets: make(map[uint16][]byte)}
		}
		a.devices[addr] = dev
	}

	// device_ids 매핑 등록
	for deviceID, addrStr := range nasaConfig.DeviceIDs {
		addr, parseErr := ParseNASAAddress(addrStr)
		if parseErr != nil {
			return nil, fmt.Errorf("samsung-nasa agent: invalid device_id address %q for %q: %w", addrStr, deviceID, parseErr)
		}
		a.deviceIDs[deviceID] = addr
		// 디바이스가 이미 등록되어 있으면 DeviceID 설정
		if dev, ok := a.devices[addr]; ok {
			dev.DeviceID = deviceID
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

	if err := a.transport.Open(); err != nil {
		// 연결 실패 시 에러 반환 대신 재연결 루프 시작
		a.logger.Warn("samsung-nasa: 트랜스포트 연결 실패, 재연결 대기", "error", err)
		go a.reconnectLoop()
	} else {
		// 연결 성공 시 정상 루프 시작
		go a.pollLoop()
		go a.receiveLoop()
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
	if a.pollTicker != nil {
		a.pollTicker.Stop()
		a.pollTicker = nil
	}
	if a.notifyTicker != nil {
		a.notifyTicker.Stop()
		a.notifyTicker = nil
	}
	a.mu.Unlock()

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
func (a *NASAAgent) processGetAllStates() ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

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
	a.sendEventLocked("device_registered", map[string]any{
		"address":     addr.String(),
		"device_id":   deviceID,
		"device_type": devType,
	})

	resp := map[string]any{
		"status":      "ok",
		"address":     addr.String(),
		"device_id":   deviceID,
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

	a.sendEventLocked("device_unregistered", map[string]any{
		"address":   addr.String(),
		"device_id": dev.DeviceID,
	})

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
func (a *NASAAgent) sendControlCommand(addr NASAAddress, sets []NASAMessageSet) error {
	// 제어 명령 전송 로그
	setNames := make([]string, 0, len(sets))
	for _, s := range sets {
		setNames = append(setNames, fmt.Sprintf("0x%04X(%d bytes)", s.Index, len(s.Value)))
	}
	a.logger.Debug("samsung-nasa: 제어 명령 전송", "addr", addr.String(), "sets", setNames)

	seq := a.nextSeqNum()
	frame, err := a.protocol.BuildControlCommand(addr, seq, sets)
	if err != nil {
		a.stats.IncrMessagesErrored()
		a.logger.Error("samsung-nasa: 제어 프레임 빌드 실패", "addr", addr.String(), "error", err)
		return fmt.Errorf("samsung-nasa: build control command failed: %w", err)
	}

	if err := a.transport.Send(frame); err != nil {
		a.stats.IncrMessagesErrored()
		a.logger.Error("samsung-nasa: 제어 명령 전송 실패", "addr", addr.String(), "error", err)
		return fmt.Errorf("samsung-nasa: send failed: %w", err)
	}

	a.stats.IncrMessagesSent()
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
			a.stats.IncrMessagesSent()
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

			go a.pollLoop()
			go a.receiveLoop()
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
					continue
				}
				if err := a.transport.Send(frame); err != nil {
					a.logger.Warn("samsung-nasa: send status query failed", "addr", addr.String(), "error", err)
					continue
				}
				a.logger.Debug("samsung-nasa: 상태 쿼리 전송", "addr", addr.String(), "seq", seq)
				a.stats.IncrMessagesSent()
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
				go a.reconnectLoop()
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
						a.stats.IncrMessagesErrored()
						continue
					}
				}
				a.logger.Warn("samsung-nasa: decode error", "error", err)
				a.stats.IncrMessagesErrored()
				continue
			}

			// a.logger.Debug("samsung-nasa: 메시지 디코드 성공",
			// 	"source", msg.SourceAddr.String(),
			// 	"dest", msg.DestAddr.String(),
			// 	"sets", len(msg.MessageSets),
			// )

			a.stats.IncrMessagesReceived()
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
			a.sendEventLocked("device_discovered", map[string]any{
				"address":     srcAddr.String(),
				"device_type": devType,
			})
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
		a.sendEventLocked("device_online", map[string]any{
			"address":   srcAddr.String(),
			"device_id": dev.DeviceID,
		})
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
			})
			// WebSocket 브로드캐스트 콜백
			// 주의: a.Name()은 a.mu.RLock()을 호출하므로 write lock 보유 중
			// 재진입 데드락을 피하려면 a.agentConfig.Name을 직접 참조해야 한다.
			if fn := a.onDeviceStateChange; fn != nil {
				agentName := a.agentConfig.Name
				globalID := fmt.Sprintf("%s:%s", agentName, formatNASAAddress(srcAddr))
				a.logger.Debug("samsung-nasa: WebSocket 상태 변경 브로드캐스트", "agent", agentName, "globalID", globalID)
				go fn(agentName, globalID)
			}
		}
	}

	a.stats.UpdateLastActivity()
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
		a.nasaConfig = nasaCfg
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
