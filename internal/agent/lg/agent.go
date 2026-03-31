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
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// LGAPAgent 는 LG LGAP HVAC 에이전트이다.
// agent.Agent, agent.MessageReceiver, agent.StatefulAgent, agent.BufferInfoProvider 인터페이스를 구현한다.
type LGAPAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig agent.AgentConfig
	lgapConfig  LGAPConfig
	devices     map[byte]*LGAPDevice       // zone -> device
	deviceIDs   map[string]byte            // device_id -> zone 역참조
	transport   LGAPTransport
	protocol    LGAPProtocol
	mu          sync.RWMutex
	pollMu      sync.Mutex                 // 시리얼 포트 동시 접근 방지
	pollTicker  *time.Ticker
	lastStates  map[byte]LGAPDeviceState   // zone -> 마지막 상태
	stopCh      chan struct{}
	msgCh       chan []byte                // Bridge 메시지 (ReceiveMessage)
	stats       *agent.AgentStats
	logger      *slog.Logger
	startedAt   time.Time
	createdAt   time.Time
	paused      bool

	reconnectMu       sync.Mutex // reconnecting 상태 보호
	isReconnecting    bool       // 재연결 진행 중 여부
	reconnectAttempts int        // 현재 재연결 시도 횟수

	// onDeviceStateChange 는 디바이스 상태 변경 시 호출되는 콜백이다.
	// agentName 과 deviceID (global ID) 를 인자로 받는다.
	onDeviceStateChange func(agentName, deviceID string)
}

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

// SetDeviceStateChangeCallback 은 디바이스 상태 변경 시 호출되는 콜백을 등록한다.
func (a *LGAPAgent) SetDeviceStateChangeCallback(fn func(agentName, deviceID string)) {
	a.onDeviceStateChange = fn
}

// processRequest 는 Process 메서드의 JSON 요청 구조체이다.
type processRequest struct {
	Command  string         `json:"command"`
	Zone     *int           `json:"zone,omitempty"`
	DeviceID string         `json:"device_id,omitempty"`
	Params   map[string]any `json:"params,omitempty"`
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
		dev := &LGAPDevice{
			Zone:     zoneByte,
			DeviceID: entry.Name,
			Online:   false,
			State:    &LGAPDeviceState{},
			Source:   "config",
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

	if err := a.transport.Open(); err != nil {
		// 연결 실패 시 에러 반환 대신 재연결 루프 시작
		a.logger.Warn("lgap: 트랜스포트 연결 실패, 재연결 대기", "error", err)
		go a.reconnectLoop()
	} else {
		// 연결 성공 시 폴링 루프 시작
		go a.pollLoop()
	}

	a.logger.Info("lgap: 에이전트 시작 완료")
	return nil
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

	a.logger.Debug("lgap: set_power 요청", "device", dev.DeviceID, "zone", fmt.Sprintf("0x%02X", zone), "power", power)

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

	return a.buildSuccessResponse(zone, dev.DeviceID, map[string]any{"power": power})
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

	a.logger.Debug("lgap: set_mode 요청", "device", dev.DeviceID, "zone", fmt.Sprintf("0x%02X", zone), "mode", modeStr)

	var flags byte
	if dev.State.Power {
		flags |= FlagPower
	}
	modeCombo := EncodeModeCombo(modeVal, StringToFanSpeed[dev.State.FanSpeed], dev.State.SwingAuto)
	temp := EncodeTargetTemp(dev.State.TargetTemp)

	if err := a.sendControlCommand(zone, flags, modeCombo, temp); err != nil {
		return nil, err
	}

	return a.buildSuccessResponse(zone, dev.DeviceID, map[string]any{"mode": modeStr})
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

	tempVal, ok := req.Params["target_temp"].(float64)
	if !ok {
		return nil, fmt.Errorf("lgap: target_temp parameter must be number")
	}

	if tempVal < 16.0 || tempVal > 30.0 {
		return nil, ErrTemperatureOutOfRange
	}

	a.logger.Debug("lgap: set_temperature 요청", "device", dev.DeviceID, "zone", fmt.Sprintf("0x%02X", zone), "target_temp", tempVal)

	var flags byte
	if dev.State.Power {
		flags |= FlagPower
	}
	modeCombo := EncodeModeCombo(StringToMode[dev.State.Mode], StringToFanSpeed[dev.State.FanSpeed], dev.State.SwingAuto)
	temp := EncodeTargetTemp(int(tempVal))

	if err := a.sendControlCommand(zone, flags, modeCombo, temp); err != nil {
		return nil, err
	}

	return a.buildSuccessResponse(zone, dev.DeviceID, map[string]any{"target_temp": tempVal})
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

	a.logger.Debug("lgap: set_fan_speed 요청", "device", dev.DeviceID, "zone", fmt.Sprintf("0x%02X", zone), "fan_speed", speedStr)

	var flags byte
	if dev.State.Power {
		flags |= FlagPower
	}
	modeCombo := EncodeModeCombo(StringToMode[dev.State.Mode], speedVal, dev.State.SwingAuto)
	temp := EncodeTargetTemp(dev.State.TargetTemp)

	if err := a.sendControlCommand(zone, flags, modeCombo, temp); err != nil {
		return nil, err
	}

	return a.buildSuccessResponse(zone, dev.DeviceID, map[string]any{"fan_speed": speedStr})
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
	if tempVal, ok := req.Params["target_temp"]; ok && tempVal != nil {
		temp, _ := tempVal.(float64)
		if temp < 16.0 || temp > 30.0 {
			return nil, ErrTemperatureOutOfRange
		}
		targetTemp = int(temp)
		result["target_temp"] = temp
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

	return a.buildSuccessResponse(zone, dev.DeviceID, result)
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
		"device_id": dev.DeviceID,
		"online":    dev.Online,
	}

	if dev.State != nil {
		resp["state"] = dev.State.StateForJSON()
	}
	if !dev.LastSeen.IsZero() {
		resp["last_seen"] = dev.LastSeen.Format(time.RFC3339)
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
			"device_id": dev.DeviceID,
			"online":    dev.Online,
		}
		if dev.State != nil {
			d["state"] = dev.State.StateForJSON()
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
		Zone:     zoneByte,
		DeviceID: deviceID,
		Name:     name,
		Online:   false,
		State:    &LGAPDeviceState{},
		Source:   "bridge",
	}

	a.devices[zoneByte] = dev
	if deviceID != "" {
		a.deviceIDs[deviceID] = zoneByte
	}

	// 이벤트 전송
	a.sendEventLocked("device_registered", map[string]any{
		"zone":      fmt.Sprintf("0x%02X", zoneByte),
		"device_id": deviceID,
		"name":      name,
	})

	resp := map[string]any{
		"status":    "ok",
		"zone":      fmt.Sprintf("0x%02X", zoneByte),
		"device_id": deviceID,
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

	if dev.Source == "config" {
		return nil, ErrConfigDeviceProtected
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// deviceIDs 맵에서도 제거
	if dev.DeviceID != "" {
		delete(a.deviceIDs, dev.DeviceID)
	}
	delete(a.devices, zone)

	a.sendEventLocked("device_unregistered", map[string]any{
		"zone":      fmt.Sprintf("0x%02X", zone),
		"device_id": dev.DeviceID,
	})

	resp := map[string]any{
		"status":    "ok",
		"zone":      fmt.Sprintf("0x%02X", zone),
		"device_id": dev.DeviceID,
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
			"device_id": dev.DeviceID,
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
		a.stats.IncrMessagesErrored()
		a.logger.Error("lgap: 제어 명령 전송 실패", "zone", fmt.Sprintf("0x%02X", zone), "error", err)
		return fmt.Errorf("lgap: send failed: %w", err)
	}

	a.stats.IncrMessagesSent()
	a.stats.AddBytesWritten(int64(len(pkt)))

	// 동기 응답 수신 (16바이트)
	buf := make([]byte, ResponseSize)
	n, err := a.transport.Receive(buf)
	if err != nil {
		a.stats.IncrMessagesErrored()
		a.logger.Error("lgap: 응답 수신 실패", "zone", fmt.Sprintf("0x%02X", zone), "error", err)
		return fmt.Errorf("lgap: receive failed: %w", err)
	}

	a.stats.IncrMessagesReceived()
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
		a.stats.IncrMessagesErrored()
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
		"device_id": deviceID,
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
	select {
	case a.msgCh <- b:
		a.logger.Debug("lgap: 이벤트 msgCh 전송 성공",
			"type", eventType,
			"chLen", len(a.msgCh),
			"chCap", cap(a.msgCh),
		)
	default:
		a.logger.Warn("lgap: msgCh full, dropping event", "type", eventType)
	}
}

// sendEventLocked 는 sendEvent 와 동일하지만 이미 락이 잡혀 있을 때 사용한다.
func (a *LGAPAgent) sendEventLocked(eventType string, data map[string]any) {
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

	if wasOffline {
		a.sendEventLocked("device_online", map[string]any{
			"zone":      fmt.Sprintf("0x%02X", zone),
			"device_id": dev.DeviceID,
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
			a.lastStates[zone] = currentState
			a.logger.Debug("lgap: 상태 변경 감지",
				"device", dev.DeviceID, "zone", fmt.Sprintf("0x%02X", zone),
				"power", currentState.Power, "mode", currentState.Mode,
				"target_temp", currentState.TargetTemp, "fan_speed", currentState.FanSpeed)
			a.sendEventLocked("device_state_changed", map[string]any{
				"zone":      fmt.Sprintf("0x%02X", zone),
				"device_id": dev.DeviceID,
			})
			// WebSocket 브로드캐스트 콜백
			if fn := a.onDeviceStateChange; fn != nil {
				agentName := a.agentConfig.Name
				globalID := fmt.Sprintf("%s:%02X", agentName, zone)
				a.logger.Debug("lgap: WebSocket 상태 변경 브로드캐스트", "agent", agentName, "globalID", globalID)
				go fn(agentName, globalID)
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
		"timestamp": time.Now().Format(time.RFC3339),
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
				"timestamp":        time.Now().Format(time.RFC3339),
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
				"reason":    err.Error(),
				"timestamp": time.Now().Format(time.RFC3339),
			})
			go a.reconnectLoop()
			return
		}
		a.logger.Warn("lgap: 상태 쿼리 전송 실패", "zone", fmt.Sprintf("0x%02X", zone), "error", err)
		a.incrementErrorCount(zone)
		return
	}

	a.stats.IncrMessagesSent()
	a.stats.AddBytesWritten(int64(len(pkt)))

	// 동기 응답 수신 (16바이트)
	buf := make([]byte, ResponseSize)
	n, err := a.transport.Receive(buf)
	if err != nil {
		// 연결 끊김 감지
		if !a.transport.Available() {
			a.logger.Warn("lgap: 트랜스포트 연결 끊김 감지", "error", err)
			a.sendEvent("transport_disconnected", map[string]any{
				"reason":    err.Error(),
				"timestamp": time.Now().Format(time.RFC3339),
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

	a.stats.IncrMessagesReceived()
	a.stats.AddBytesRead(int64(n))

	a.logger.Debug("lgap: 상태 응답 수신",
		"zone", fmt.Sprintf("0x%02X", zone),
		"rx", hex.EncodeToString(buf[:n]),
		"bytes", n,
	)

	resp, err := a.protocol.DecodeResponse(buf[:ResponseSize])
	if err != nil {
		a.stats.IncrMessagesErrored()
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
			"device_id": dev.DeviceID,
		})
		a.logger.Warn("lgap: 디바이스 오프라인",
			"zone", fmt.Sprintf("0x%02X", zone),
			"device_id", dev.DeviceID,
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
			Zone:     zoneByte,
			DeviceID: entry.Name,
			Online:   false,
			State:    &LGAPDeviceState{},
			Source:   "pinned",
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
	for _, dev := range a.devices {
		if dev.Online {
			dev.Online = false
			offlined++
		}
	}
	a.mu.Unlock()

	if offlined > 0 {
		a.logger.Info("lgap: 통신 끊김, 디바이스 오프라인 전환", "count", offlined)
	}
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
			"device_id": dev.DeviceID,
			"online":    dev.Online,
		}
		if dev.State != nil {
			d["state"] = map[string]any{
				"power":       dev.State.Power,
				"mode":        dev.State.Mode,
				"target_temp": dev.State.TargetTemp,
				"room_temp":   dev.State.RoomTemp,
				"fan_speed":   dev.State.FanSpeed,
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
