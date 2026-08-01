package modbus

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/device"
	modbus "github.com/xtra/xflow/internal/modbus"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ModbusAgent 는 MODBUS/TCP 클라이언트 에이전트이다.
// agent.Agent, agent.MessageReceiver 인터페이스를 구현한다.
type ModbusAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig agent.AgentConfig
	config      ModbusConfig
	devices     []*ModbusDevice
	caches      map[string]*RegisterCache // 키: device config ID
	mu          sync.RWMutex
	pollTicker  *time.Ticker
	pollResetCh chan time.Duration // 폴링 간격 변경 시그널
	pollWg      sync.WaitGroup     // pollLoop + 그룹별 스케줄러 goroutine 수명 관리 (M5, 누수 방지)
	// groupStops 는 그룹별 폴링 스케줄러(groupPollLoop)의 개별 종료 채널이다(M9).
	// 키: groupStatKey(deviceID, groupName). set_config 런타임 재구성 시 개별 그룹 루프만
	// 정지·재시작하기 위해 사용한다. Start(단일 스레드) 또는 a.mu.Lock() 하에서만 접근한다.
	groupStops map[string]chan struct{}
	stopCh     chan struct{}
	msgCh      chan []byte // Bridge 메시지 (ReceiveMessage)
	stats      *agent.AgentStats
	// devStats/groupStats 는 디바이스별·그룹별 요청 통계이다(M7).
	// 생성 시 1회 채워지며 이후 맵 자체는 불변(엔트리는 atomic) — 폴링 goroutine 과의 경합 없음.
	devStats   map[string]*requestCounters // 키: deviceID
	groupStats map[string]*requestCounters // 키: groupStatKey(deviceID, groupName)
	logger     *slog.Logger
	startedAt  time.Time
	createdAt  time.Time
	paused     bool
	started    bool // Start() 호출 여부 (멱등성 보장)
}

// requestCounters 는 단일 스코프(디바이스 또는 그룹)의 요청 처리 통계를 원자적으로 추적한다(M7).
// 성공/오류 카운터와 지연(누적/최근)을 폴링 goroutine 간 경합 없이 갱신한다.
type requestCounters struct {
	success     atomic.Int64
	errors      atomic.Int64
	latencySum  atomic.Int64 // 누적 지연(나노초)
	latencyLast atomic.Int64 // 마지막 요청 지연(나노초)
}

// record 는 요청 완료 시 성공/오류와 지연을 반영한다(REQ-04).
func (c *requestCounters) record(ok bool, latency time.Duration) {
	if ok {
		c.success.Add(1)
	} else {
		c.errors.Add(1)
	}
	c.latencySum.Add(int64(latency))
	c.latencyLast.Store(int64(latency))
}

// avgLatency 는 누적 지연을 총 요청 수로 나눈 평균 지연을 반환한다.
func (c *requestCounters) avgLatency() time.Duration {
	total := c.success.Load() + c.errors.Load()
	if total == 0 {
		return 0
	}
	return time.Duration(c.latencySum.Load() / total)
}

// groupStatKey 는 그룹 통계 맵의 키를 생성한다(deviceID + 그룹명).
func groupStatKey(deviceID, groupName string) string {
	return deviceID + "\x1f" + groupName
}

// requestStatsMap 은 요청 카운터를 State() 응답용 맵으로 직렬화한다(M7).
func requestStatsMap(c *requestCounters) map[string]any {
	return map[string]any{
		"success":         c.success.Load(),
		"errors":          c.errors.Load(),
		"avg_latency_ms":  float64(c.avgLatency()) / float64(time.Millisecond),
		"last_latency_ms": float64(c.latencyLast.Load()) / float64(time.Millisecond),
	}
}

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*ModbusAgent)(nil)
var _ agent.MessageReceiver = (*ModbusAgent)(nil)
var _ agent.StatefulAgent = (*ModbusAgent)(nil)
var _ agent.PollingConfigurable = (*ModbusAgent)(nil)
var _ agent.BufferInfoProvider = (*ModbusAgent)(nil)
var _ agent.ConnectionStatsProvider = (*ModbusAgent)(nil)

// DeviceProvider 는 이 에이전트의 디바이스를 device.DeviceProvider 로 노출한다.
func (a *ModbusAgent) DeviceProvider() device.DeviceProvider {
	return NewModbusDeviceProvider(a)
}

// processRequest 는 Process 메서드의 JSON 요청 구조체이다.
type processRequest struct {
	Command  string         `json:"command"`
	DeviceID string         `json:"device_id,omitempty"`
	Force    bool           `json:"force,omitempty"`
	Params   map[string]any `json:"params,omitempty"`
}

// NewModbusAgent 는 새로운 MODBUS/TCP 에이전트를 생성한다.
func NewModbusAgent(agentConfig agent.AgentConfig) (agent.Agent, error) {
	cfg, err := parseModbusConfig(agentConfig.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("modbus agent: %w", err)
	}

	a := &ModbusAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("modbus-tcp")),
		config:        cfg,
		devices:       make([]*ModbusDevice, 0, len(cfg.Devices)),
		pollResetCh:   make(chan time.Duration, 1),
		groupStops:    make(map[string]chan struct{}),
		stopCh:        make(chan struct{}),
		msgCh:         make(chan []byte, cfg.MsgChannelSize),
		stats:         agent.NewAgentStats(),
		logger:        agent.ResolveLogger(agentConfig),
		createdAt:     time.Now(),
	}

	// 설정에 정의된 디바이스 생성 (transport 로 라우팅, M4)
	a.buildDevices(cfg)

	// 디바이스별 RegisterCache 생성
	a.caches = make(map[string]*RegisterCache, len(a.devices))
	for _, dev := range a.devices {
		a.caches[dev.config.ID] = NewRegisterCache()
	}

	// TypeOverlay 초기화
	a.initCacheTypeOverlays()

	// 디바이스별·그룹별 요청 통계 초기화 (M7)
	a.initRequestStats()

	if err := a.Init(agentConfig); err != nil {
		return nil, err
	}

	return a, nil
}

// buildDevices 는 파싱된 transport 에 따라 각 디바이스의 트랜스포트를 선택해 생성한다(M4).
//   - tcp(기본): 디바이스별 ModbusTCPTransport (기존 동작 그대로, AC-03)
//   - rtu: 단일 시리얼 버스를 공유하는 ModbusRTUTransport 를 모든 디바이스에 주입
//     (반이중 멀티드롭 — 하나의 포트/turnaround mutex 를 unitID 별로 직렬 공유, A-4)
func (a *ModbusAgent) buildDevices(cfg ModbusConfig) {
	if cfg.Transport == TransportRTU {
		rtu := NewModbusRTUTransport(cfg.Serial, cfg.RequestTimeout, a.logger)
		for i := range cfg.Devices {
			dev := newModbusDeviceWithTransport(cfg.Devices[i], rtu, a.logger)
			a.devices = append(a.devices, dev)
		}
		return
	}
	for i := range cfg.Devices {
		dev := NewModbusDevice(cfg.Devices[i], cfg.RequestTimeout, a.logger)
		a.devices = append(a.devices, dev)
	}
}

// initRequestStats 는 디바이스별·그룹별 요청 통계 카운터를 생성한다(M7).
// 생성 시 1회 호출되며, 맵은 이후 불변으로 유지되어 폴링 goroutine 이 락 없이 안전하게 읽는다.
func (a *ModbusAgent) initRequestStats() {
	a.devStats = make(map[string]*requestCounters, len(a.devices))
	a.groupStats = make(map[string]*requestCounters)
	for _, dev := range a.devices {
		a.devStats[dev.config.ID] = &requestCounters{}
		for _, rg := range dev.config.RegisterGroups {
			a.groupStats[groupStatKey(dev.config.ID, rg.Name)] = &requestCounters{}
		}
	}
}

// recordRequestStat 는 요청 완료 시 디바이스별·그룹별 통계를 갱신한다(M7, REQ-04).
// 트랜스포트 오류(ok=false)는 오류 카운터를, 성공은 성공 카운터를 증가시킨다(AC-06).
func (a *ModbusAgent) recordRequestStat(deviceID, groupName string, ok bool, latency time.Duration) {
	if c, found := a.devStats[deviceID]; found {
		c.record(ok, latency)
	}
	if groupName != "" {
		if c, found := a.groupStats[groupStatKey(deviceID, groupName)]; found {
			c.record(ok, latency)
		}
	}
}

// newModbusAgentWithTransport 는 테스트용 팩토리 함수이다.
// 각 디바이스에 주입된 트랜스포트를 사용한다.
func newModbusAgentWithTransport(agentConfig agent.AgentConfig, transports []ModbusTransport) (*ModbusAgent, error) {
	cfg, err := parseModbusConfig(agentConfig.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("modbus agent: %w", err)
	}

	a := &ModbusAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("modbus-tcp")),
		config:        cfg,
		devices:       make([]*ModbusDevice, 0, len(cfg.Devices)),
		pollResetCh:   make(chan time.Duration, 1),
		groupStops:    make(map[string]chan struct{}),
		stopCh:        make(chan struct{}),
		msgCh:         make(chan []byte, cfg.MsgChannelSize),
		stats:         agent.NewAgentStats(),
		logger:        agent.ResolveLogger(agentConfig),
		createdAt:     time.Now(),
	}

	// 테스트용: 각 디바이스에 주입된 트랜스포트 사용
	for i := range cfg.Devices {
		var transport ModbusTransport
		if i < len(transports) {
			transport = transports[i]
		} else {
			transport = NewModbusTCPTransport(cfg.Devices[i].Host, cfg.Devices[i].Port, cfg.RequestTimeout, a.logger)
		}
		dev := newModbusDeviceWithTransport(cfg.Devices[i], transport, a.logger)
		a.devices = append(a.devices, dev)
	}

	// 디바이스별 RegisterCache 생성
	a.caches = make(map[string]*RegisterCache, len(a.devices))
	for _, dev := range a.devices {
		a.caches[dev.config.ID] = NewRegisterCache()
	}

	// TypeOverlay 초기화
	a.initCacheTypeOverlays()

	// 디바이스별·그룹별 요청 통계 초기화 (M7)
	a.initRequestStats()

	if err := a.Init(agentConfig); err != nil {
		return nil, err
	}

	return a, nil
}

// Init 은 에이전트를 초기화한다.
func (a *ModbusAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("modbus init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("modbus init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("modbus init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	a.logger.Info("modbus: 에이전트 초기화 완료",
		"devices", len(a.devices),
		"readMode", a.config.ReadMode,
	)

	return nil
}

// Start 는 디바이스에 연결하고 pollLoop 를 시작한다.
// 멱등성: 이미 Start 가 호출된 경우 no-op 으로 반환한다.
func (a *ModbusAgent) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.started {
		a.mu.Unlock()
		return nil
	}
	a.mu.Unlock()

	if a.CurrentState() == lifecycle.StateRunning {
		a.logger.Debug("modbus: Start() 진입",
			"readMode", a.config.ReadMode,
			"pollInterval", a.config.PollInterval,
			"devices", len(a.devices),
		)

		// 디바이스 연결
		for _, dev := range a.devices {
			if err := dev.Connect(ctx); err != nil {
				a.logger.Warn("modbus: 디바이스 연결 실패",
					"device", dev.config.ID,
					"error", err,
				)
			}
		}

		a.mu.Lock()
		a.started = true
		a.mu.Unlock()

		// cached 모드일 때만 pollLoop 시작
		if a.config.ReadMode == "cached" {
			a.logger.Info("modbus: pollLoop 시작",
				"pollInterval", a.config.PollInterval,
				"mode", a.config.Mode,
			)
			a.pollWg.Add(1)
			go a.pollLoop()

			// 그룹별 독립 폴링 스케줄러 시작 (M5).
			// poll_interval 이 지정된 그룹마다 전용 goroutine 을 띄운다.
			// 하나도 없으면 루프는 만들어지지 않아 기존 단일-티커 동작과 동일하다(AC-03).
			// Start 는 단일 스레드 초기화이므로 groupStops 접근에 락이 불필요하다.
			for _, dev := range a.devices {
				for i := range dev.config.RegisterGroups {
					if dev.config.RegisterGroups[i].PollInterval > 0 {
						a.startGroupLoop(dev, dev.config.RegisterGroups[i])
					}
				}
			}
		}

		a.logger.Info("modbus: 에이전트 시작 완료")
		return nil
	}

	return fmt.Errorf("modbus start: agent is not in running state (current: %s)", a.CurrentState())
}

// Stop 은 에이전트를 정지한다.
func (a *ModbusAgent) Stop(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("modbus stop: %w", err)
	}

	// goroutine 들에게 종료 시그널
	close(a.stopCh)

	a.mu.Lock()
	a.started = false
	if a.pollTicker != nil {
		a.pollTicker.Stop()
		a.pollTicker = nil
	}
	a.mu.Unlock()

	// pollLoop + 그룹별 스케줄러 goroutine 이 모두 종료될 때까지 대기(M5, 누수·경합 방지).
	// stopCh 를 이미 닫았으므로 각 goroutine 은 진행 중인 폴을 마치고 반환한다.
	a.pollWg.Wait()

	// 디바이스 연결 종료
	for _, dev := range a.devices {
		if err := dev.Close(); err != nil {
			a.logger.Warn("modbus: 디바이스 연결 종료 실패",
				"device", dev.config.ID,
				"error", err,
			)
		}
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
		return fmt.Errorf("modbus stop: %w", err)
	}

	return nil
}

// Pause 는 Running -> Paused 로 전환한다.
func (a *ModbusAgent) Pause(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("modbus pause: %w", err)
	}
	a.mu.Lock()
	a.paused = true
	a.mu.Unlock()
	return nil
}

// Resume 은 Paused -> Running 으로 전환한다.
func (a *ModbusAgent) Resume(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("modbus resume: %w", err)
	}
	a.mu.Lock()
	a.paused = false
	a.mu.Unlock()
	return nil
}

// Health 는 에이전트의 건강 상태를 반환한다.
// 디바이스 온라인 비율에 따라 상태를 결정한다.
func (a *ModbusAgent) Health() agent.HealthStatus {
	now := time.Now()
	state := a.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		// 디바이스 온라인 비율 확인
		online := 0
		total := len(a.devices)
		for _, dev := range a.devices {
			if dev.IsOnline() {
				online++
			}
		}

		if total > 0 && online == 0 {
			return agent.HealthStatus{
				Status:    agent.HealthDegraded,
				LastCheck: now,
				Message:   fmt.Sprintf("modbus agent running but all %d devices offline", total),
			}
		}

		return agent.HealthStatus{
			Status:    agent.HealthHealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("modbus agent is running (%d/%d devices online)", online, total),
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "modbus agent is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("modbus agent is in %s state", state),
		}
	}
}

// pollLoop 는 주기적으로 디바이스 레지스터를 읽어 msgCh 로 전송한다.
// ReadMode 가 "cached" 일 때만 Start 에서 호출된다.
// Interval 모드: 매 폴 주기마다 전체 데이터 전송.
// Event 모드: 변경 감지 시에만 전송 + heartbeat 주기마다 전체 전송.
func (a *ModbusAgent) pollLoop() {
	defer a.pollWg.Done()
	pollTicker := time.NewTicker(a.config.PollInterval)
	a.mu.Lock()
	a.pollTicker = pollTicker
	a.mu.Unlock()
	defer pollTicker.Stop()

	// Event 모드: heartbeat 타이머 설정
	var heartbeatTicker *time.Ticker
	var heartbeatC <-chan time.Time
	if a.config.Mode == "event" {
		heartbeatTicker = time.NewTicker(a.config.HeartbeatInterval)
		heartbeatC = heartbeatTicker.C
		defer heartbeatTicker.Stop()
	}

	pollCount := 0
	for {
		select {
		case <-a.stopCh:
			a.logger.Debug("modbus: pollLoop 종료 (stopCh)", "totalPolls", pollCount)
			return
		case <-pollTicker.C:
			pollCount++
			a.logger.Debug("modbus: poll tick", "count", pollCount, "interval", a.config.PollInterval)
			a.pollDevices(false) // 일반 폴링
		case <-heartbeatC:
			a.pollDevices(true) // heartbeat: 전체 데이터 강제 전송
		case newInterval := <-a.pollResetCh:
			pollTicker.Reset(newInterval)
			a.logger.Info("modbus: 폴링 간격 변경 적용", "interval", newInterval)
		}
	}
}

// pollDevices 는 모든 온라인 디바이스의 레지스터를 읽는다.
// forceFullSend 가 true 이면 event 모드에서도 전체 데이터를 전송한다 (heartbeat).
func (a *ModbusAgent) pollDevices(forceFullSend bool) {
	// 런타임 가변 필드(요청 타임아웃·재연결 간격·stale 임계값)를 RLock 스냅샷으로 읽는다.
	// M9 set_config 가 a.mu.Lock() 하에 이 값들을 갱신하므로 경합을 피한다(-race).
	a.mu.RLock()
	paused := a.paused
	reqTimeout := a.config.RequestTimeout
	reconnectInterval := a.config.ReconnectInterval
	staleThreshold := a.config.StaleThreshold
	a.mu.RUnlock()
	if paused {
		return
	}

	a.logger.Debug("modbus: 폴링 시작",
		"devices", len(a.devices),
		"forceFullSend", forceFullSend,
	)

	ctx, cancel := context.WithTimeout(context.Background(), reqTimeout)
	defer cancel()

	for _, dev := range a.devices {
		if !dev.IsOnline() {
			// 오프라인 디바이스 재연결 시도
			if !dev.TryReconnect(ctx, reconnectInterval) {
				a.logger.Debug("modbus: 디바이스 재연결 실패 또는 대기 중",
					"device", dev.config.ID,
				)
				continue
			}
			a.logger.Info("modbus: 디바이스 재연결 성공", "device", dev.config.ID)
		}
		a.pollDevice(ctx, dev, forceFullSend)
	}

	// Stale 레지스터 그룹 경고 이벤트 전송
	for _, dev := range a.devices {
		if cache, ok := a.caches[dev.config.ID]; ok {
			staleGroups := cache.StaleGroups(staleThreshold)
			for _, groupKey := range staleGroups {
				a.sendEvent("register_group_stale", map[string]any{
					"device_id": dev.config.ID,
					"group_key": groupKey,
					"threshold": a.config.StaleThreshold.String(),
					"timestamp": time.Now().Format(time.RFC3339),
				})
			}
		}
	}
}

// pollDevice 는 단일 디바이스의 기본 케이던스 그룹(poll_interval 미지정)을 읽고
// 모드에 따라 이벤트를 전송한다. poll_interval 이 지정된 그룹은 groupPollLoop 가 담당한다(M5).
// poll_interval 을 가진 그룹이 하나도 없으면 모든 그룹이 여기서 폴링되어 기존 동작과 동일하다(AC-03).
func (a *ModbusAgent) pollDevice(ctx context.Context, dev *ModbusDevice, forceFullSend bool) {
	// 레지스터 그룹 슬라이스를 RLock 스냅샷으로 읽는다(M9). set_config 는 copy-on-write 로
	// dev.config.RegisterGroups 를 통째로 교체하므로, 스냅샷된 헤더는 안정적이다(-race).
	a.mu.RLock()
	groups := dev.config.RegisterGroups
	a.mu.RUnlock()
	for _, rg := range groups {
		if rg.PollInterval > 0 {
			// 그룹별 독립 케이던스 — 전용 스케줄러가 폴링하므로 기본 루프에서 제외(M5).
			continue
		}
		a.pollGroupRead(ctx, dev, rg, forceFullSend)
	}
}

// pollGroupRead 는 단일 (디바이스, 그룹)의 레지스터를 1회 읽어 캐시 갱신·이벤트 전송·통계 기록을 수행한다.
// 기본 케이던스 pollDevice 와 그룹별 스케줄러 groupPollLoop 양쪽에서 호출된다(M5).
// 트랜스포트(turnaround mutex)·캐시(mutex)·통계(atomic)가 모두 스레드 안전하므로
// 서로 다른 그룹의 동시 폴링에도 경합이 없다(-race).
func (a *ModbusAgent) pollGroupRead(ctx context.Context, dev *ModbusDevice, rg RegisterGroupConfig, forceFullSend bool) {
	a.mu.RLock()
	mode := a.config.Mode
	a.mu.RUnlock()

	start := time.Now()
	data, err := dev.ReadRegisters(ctx, rg)
	// 요청 완료 통계(디바이스별·그룹별) — 성공/오류 모두 기록(M7, AC-06).
	a.recordRequestStat(dev.config.ID, rg.Name, err == nil, time.Since(start))
	if err != nil {
		a.logger.Warn("modbus: 레지스터 읽기 실패",
			"device", dev.config.ID,
			"group", rg.Name,
			"error", err,
		)
		a.stats.IncrExternalMessagesErrored()
		return
	}

	a.stats.IncrExternalMessagesReceived()
	a.stats.AddBytesRead(int64(len(data)))
	a.stats.UpdateLastActivity()

	cache, ok := a.caches[dev.config.ID]
	if !ok {
		return
	}

	switch mode {
	case "interval":
		// Interval 모드: 항상 전체 데이터 전송
		cache.UpdateFromRead(rg.FunctionCode, rg.StartAddress, data, rg.Quantity)
		a.sendRegisterEvent(dev, rg, data, "interval")

	case "event":
		if forceFullSend {
			// Heartbeat: 변경 여부와 무관하게 전체 데이터 전송
			cache.UpdateFromRead(rg.FunctionCode, rg.StartAddress, data, rg.Quantity)
			a.sendRegisterEvent(dev, rg, data, "event_heartbeat")
		} else {
			// 변경 감지: 변경된 데이터만 전송
			changed, changedData := cache.CompareAndUpdate(rg.FunctionCode, rg.StartAddress, data, rg.Quantity)
			if changed {
				a.sendChangedEvent(dev, rg, changedData)
			}
		}
	}
}

// startGroupLoop 는 (디바이스, 그룹) 전용 폴링 goroutine 을 시작하고 개별 종료 채널을 등록한다(M5/M9).
// 호출자는 Start(단일 스레드) 이거나 a.mu.Lock() 을 보유해야 한다(groupStops 보호).
// M9 런타임 재구성(processSetConfig)이 개별 그룹 루프만 정지·재시작할 때 재사용한다.
func (a *ModbusAgent) startGroupLoop(dev *ModbusDevice, rg RegisterGroupConfig) {
	key := groupStatKey(dev.config.ID, rg.Name)
	stop := make(chan struct{})
	a.groupStops[key] = stop
	a.pollWg.Add(1)
	go a.groupPollLoop(dev, rg, stop)
}

// stopGroupLoop 는 (디바이스, 그룹) 전용 폴링 goroutine 에 개별 종료 시그널을 보내고 등록을 해제한다(M9).
// 호출자는 a.mu.Lock() 을 보유해야 한다. 대상 루프가 없으면 no-op.
func (a *ModbusAgent) stopGroupLoop(deviceID, groupName string) {
	key := groupStatKey(deviceID, groupName)
	if stop, ok := a.groupStops[key]; ok {
		close(stop)
		delete(a.groupStops, key)
	}
}

// groupPollLoop 는 poll_interval 이 지정된 단일 (디바이스, 그룹)을 자신의 케이던스로 독립 폴링한다(M5).
// 티커 수명은 이 goroutine 이 소유하며 전역 stopCh 종료, 개별 stop 종료, 또는 반환 시 반드시 정지된다
// (누수 방지, plan.md §5). 개별 stop 은 M9 런타임 재구성에서 이 그룹 루프만 재시작할 때 사용된다.
func (a *ModbusAgent) groupPollLoop(dev *ModbusDevice, rg RegisterGroupConfig, stop <-chan struct{}) {
	defer a.pollWg.Done()
	ticker := time.NewTicker(rg.PollInterval)
	defer ticker.Stop()

	a.logger.Info("modbus: 그룹별 폴링 스케줄러 시작",
		"device", dev.config.ID,
		"group", rg.Name,
		"interval", rg.PollInterval,
	)

	for {
		select {
		case <-a.stopCh:
			return
		case <-stop:
			// M9: 이 그룹 루프만 개별 정지(런타임 재구성으로 케이던스 변경/그룹 제거).
			return
		case <-ticker.C:
			a.pollGroupTick(dev, rg)
		}
	}
}

// pollGroupTick 는 그룹 스케줄러의 1회 폴을 수행한다: paused 확인, 온라인/재연결, 읽기(M5).
// 기본 pollDevices 의 디바이스 단위 온라인/재연결 로직과 동일한 규약을 그룹 단위로 적용한다.
func (a *ModbusAgent) pollGroupTick(dev *ModbusDevice, rg RegisterGroupConfig) {
	a.mu.RLock()
	paused := a.paused
	reqTimeout := a.config.RequestTimeout
	reconnectInterval := a.config.ReconnectInterval
	a.mu.RUnlock()
	if paused {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), reqTimeout)
	defer cancel()

	if !dev.IsOnline() {
		if !dev.TryReconnect(ctx, reconnectInterval) {
			return
		}
	}
	a.pollGroupRead(ctx, dev, rg, false)
}

// sendRegisterEvent 는 전체 레지스터 데이터 이벤트를 전송한다.
func (a *ModbusAgent) sendRegisterEvent(dev *ModbusDevice, rg RegisterGroupConfig, rawData []byte, mode string) {
	evtData := map[string]any{
		"device_id":     dev.config.ID,
		"unit_id":       dev.UnitID(),
		"timestamp":     time.Now().Format(time.RFC3339),
		"mode":          mode,
		"function_code": rg.FunctionCode,
		"start_address": rg.StartAddress,
		"quantity":      rg.Quantity,
		"group_name":    rg.Name,
		"data":          rawData,
	}

	// TypeOverlay 설정 시 해당 그룹 주소 범위에 속하는 typed_values 만 추가
	if cache, ok := a.caches[dev.config.ID]; ok && cache.HasTypeOverlay() {
		snapshot := cache.GetSnapshot()
		var srcKey string
		switch rg.FunctionCode {
		case FC03ReadHoldingRegisters:
			srcKey = "typed_holding_registers"
		case FC04ReadInputRegisters:
			srcKey = "typed_input_registers"
		}
		if srcKey != "" {
			if tv, ok := snapshot[srcKey]; ok {
				if typed, ok := tv.(map[uint16]any); ok {
					filtered := filterTypedValuesByRange(typed, rg.StartAddress, rg.Quantity)
					if len(filtered) > 0 {
						evtData["typed_values"] = filtered
					}
				}
			}
		}
	}

	a.sendEvent("register_data", evtData)
}

// sendChangedEvent 는 변경된 레지스터 데이터만 이벤트로 전송한다.
// changedData 에 typed_changed_values 가 포함되어 있으면 이벤트에도 전달한다.
func (a *ModbusAgent) sendChangedEvent(dev *ModbusDevice, rg RegisterGroupConfig, changedData map[string]any) {
	evtData := map[string]any{
		"device_id":     dev.config.ID,
		"unit_id":       dev.UnitID(),
		"timestamp":     time.Now().Format(time.RFC3339),
		"mode":          "event",
		"function_code": rg.FunctionCode,
		"start_address": rg.StartAddress,
		"group_name":    rg.Name,
		"changes":       changedData,
	}

	// changedData 에서 typed_changed_values 를 이벤트 최상위로 복사
	if tv, ok := changedData["typed_changed_values"]; ok {
		evtData["typed_changed_values"] = tv
	}

	a.sendEvent("register_changed", evtData)
}

// sendEvent 는 이벤트를 JSON 으로 마샬링하여 msgCh 에 논블로킹 전송한다.
func (a *ModbusAgent) sendEvent(eventType string, data map[string]any) {
	evt := map[string]any{"type": eventType}
	for k, v := range data {
		evt[k] = v
	}
	b, err := json.Marshal(evt)
	if err != nil {
		a.logger.Warn("modbus: event marshal failed", "error", err)
		return
	}
	select {
	case a.msgCh <- b:
		a.logger.Debug("modbus: 이벤트 msgCh 전송 성공",
			"type", eventType,
			"chLen", len(a.msgCh),
			"chCap", cap(a.msgCh),
		)
	default:
		a.logger.Warn("modbus: msgCh full, dropping event", "type", eventType)
	}
}

// ReceiveMessage 는 msgCh 에서 메시지를 수신한다.
// agent.MessageReceiver 인터페이스 구현.
func (a *ModbusAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case data := <-a.msgCh:
		a.logger.Debug("modbus: ReceiveMessage 전달",
			"bytes", len(data),
			"preview", truncateForLog(data, 120),
		)
		return data, nil
	case <-a.stopCh:
		return nil, fmt.Errorf("modbus: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Process 는 JSON 명령을 디스패치하여 처리한다.
func (a *ModbusAgent) Process(data []byte) ([]byte, error) {
	var req processRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("modbus process: invalid JSON: %w", err)
	}

	switch req.Command {
	case "read_registers":
		return a.processReadRegisters(&req)
	case "get_status":
		return a.processGetStatus()
	case "get_cache":
		return a.processGetCache(&req)
	case "get_all_caches":
		return a.processGetAllCaches()
	case "write_coil":
		return a.processWriteCoil(&req)
	case "write_register":
		return a.processWriteRegister(&req)
	case "write_coils":
		return a.processWriteCoils(&req)
	case "write_registers":
		return a.processWriteRegisters(&req)
	case "read_raw":
		return a.processReadRaw(&req)
	case "set_config":
		return a.processSetConfig(&req)
	default:
		return nil, ErrInvalidCommand
	}
}

// processReadRegisters 는 지정 디바이스의 레지스터를 읽어 반환한다.
// read_mode 와 force 파라미터에 따라 동작이 달라진다:
//   - cached 모드 + force=false: 캐시된 값을 반환 (디바이스 쿼리 없음)
//   - cached 모드 + force=true: 디바이스에서 직접 읽고 캐시 갱신 후 반환
//   - direct 모드: 항상 디바이스에서 직접 읽어 반환
func (a *ModbusAgent) processReadRegisters(req *processRequest) ([]byte, error) {
	dev, err := a.findDevice(req.DeviceID)
	if err != nil {
		return nil, err
	}

	// 런타임 가변 필드(read_mode·요청 타임아웃·레지스터 그룹)를 RLock 스냅샷으로 읽는다(M9).
	// set_config 가 register_groups/request_timeout 을 a.mu.Lock() 하에 갱신하므로 경합을 피한다.
	a.mu.RLock()
	readMode := a.config.ReadMode
	reqTimeout := a.config.RequestTimeout
	groups := dev.config.RegisterGroups
	a.mu.RUnlock()

	// cached 모드 + force=false: 캐시 스냅샷 반환
	if readMode == "cached" && !req.Force {
		cache, ok := a.caches[dev.config.ID]
		if !ok {
			return nil, ErrCacheNotFound
		}
		snapshot := cache.GetSnapshot()
		resp := map[string]any{
			"device_id": dev.config.ID,
			"timestamp": time.Now().Format(time.RFC3339),
			"mode":      "cached",
			"cache":     snapshot,
		}
		return json.Marshal(resp)
	}

	// direct 또는 force: 디바이스에서 직접 읽기
	if !dev.IsOnline() {
		return nil, ErrDeviceOffline
	}

	ctx, cancel := context.WithTimeout(context.Background(), reqTimeout)
	defer cancel()

	results := make([]map[string]any, 0, len(groups))
	for _, rg := range groups {
		start := time.Now()
		data, readErr := dev.ReadRegisters(ctx, rg)
		a.recordRequestStat(dev.config.ID, rg.Name, readErr == nil, time.Since(start)) // M7
		if readErr != nil {
			a.stats.IncrExternalMessagesErrored()
			results = append(results, map[string]any{
				"group_name": rg.Name,
				"error":      readErr.Error(),
			})
			continue
		}
		a.stats.IncrExternalMessagesReceived()
		a.stats.AddBytesRead(int64(len(data)))
		a.stats.UpdateLastActivity()

		// force 읽기: 캐시도 갱신
		if req.Force {
			if cache, ok := a.caches[dev.config.ID]; ok {
				cache.UpdateFromRead(rg.FunctionCode, rg.StartAddress, data, rg.Quantity)
			}
		}

		groupResult := map[string]any{
			"group_name":    rg.Name,
			"function_code": rg.FunctionCode,
			"start_address": rg.StartAddress,
			"quantity":      rg.Quantity,
			"data":          data,
		}

		// TypeOverlay 설정 시 해당 그룹 주소 범위에 속하는 typed_values 만 추가
		if cache, ok := a.caches[dev.config.ID]; ok && cache.HasTypeOverlay() {
			snapshot := cache.GetSnapshot()
			var srcKey string
			switch rg.FunctionCode {
			case FC03ReadHoldingRegisters:
				srcKey = "typed_holding_registers"
			case FC04ReadInputRegisters:
				srcKey = "typed_input_registers"
			}
			if srcKey != "" {
				if tv, ok := snapshot[srcKey]; ok {
					if typed, ok := tv.(map[uint16]any); ok {
						filtered := filterTypedValuesByRange(typed, rg.StartAddress, rg.Quantity)
						if len(filtered) > 0 {
							groupResult["typed_values"] = filtered
						}
					}
				}
			}
		}

		results = append(results, groupResult)
	}

	mode := "direct"
	if req.Force {
		mode = "force"
	}
	resp := map[string]any{
		"device_id": dev.config.ID,
		"timestamp": time.Now().Format(time.RFC3339),
		"mode":      mode,
		"registers": results,
	}

	return json.Marshal(resp)
}

// processReadRaw 는 지정된 Function Code, 주소, 수량으로 레지스터를 읽어 원시 바이트를 반환한다.
// 브릿지 주도 폴링에서 사용된다. params: function_code, address, quantity, unit_id (선택).
func (a *ModbusAgent) processReadRaw(req *processRequest) ([]byte, error) {
	if req.Params == nil {
		return nil, fmt.Errorf("modbus read_raw: params required")
	}

	fcRaw, ok := req.Params["function_code"]
	if !ok {
		return nil, fmt.Errorf("modbus read_raw: function_code required")
	}
	fc := toByte(fcRaw)

	addrRaw, ok := req.Params["address"]
	if !ok {
		return nil, fmt.Errorf("modbus read_raw: address required")
	}
	addr := toUint16(addrRaw)

	qtyRaw, ok := req.Params["quantity"]
	if !ok {
		return nil, fmt.Errorf("modbus read_raw: quantity required")
	}
	qty := toUint16(qtyRaw)

	// unit_id로 디바이스 매칭. 없으면 첫 번째 디바이스 사용.
	var dev *ModbusDevice
	if uidRaw, ok := req.Params["unit_id"]; ok {
		uid := toByte(uidRaw)
		for _, d := range a.devices {
			if d.UnitID() == uid {
				dev = d
				break
			}
		}
	}
	if dev == nil && len(a.devices) > 0 {
		dev = a.devices[0]
	}
	if dev == nil {
		return nil, ErrDeviceNotFound
	}

	if !dev.IsOnline() {
		return nil, ErrDeviceOffline
	}

	// 요청 타임아웃을 RLock 스냅샷으로 읽는다(M9, set_config 런타임 갱신 경합 방지).
	a.mu.RLock()
	reqTimeout := a.config.RequestTimeout
	a.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), reqTimeout)
	defer cancel()

	rg := RegisterGroupConfig{
		FunctionCode: fc,
		StartAddress: addr,
		Quantity:     qty,
	}
	data, err := dev.ReadRegisters(ctx, rg)
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		return nil, fmt.Errorf("modbus read_raw: %w", err)
	}

	a.stats.IncrExternalMessagesReceived()
	a.stats.AddBytesRead(int64(len(data)))
	a.stats.UpdateLastActivity()

	resp := map[string]any{
		"data": base64.StdEncoding.EncodeToString(data),
	}
	return json.Marshal(resp)
}

// processGetStatus 는 전체 디바이스 상태를 반환한다.
func (a *ModbusAgent) processGetStatus() ([]byte, error) {
	devices := make([]map[string]any, 0, len(a.devices))
	for _, dev := range a.devices {
		devices = append(devices, map[string]any{
			"device_id": dev.config.ID,
			"host":      dev.config.Host,
			"port":      dev.config.Port,
			"online":    dev.IsOnline(),
		})
	}

	resp := map[string]any{
		"device_count": len(a.devices),
		"devices":      devices,
	}

	return json.Marshal(resp)
}

// processGetCache 는 지정 디바이스의 캐시 스냅샷을 반환한다.
func (a *ModbusAgent) processGetCache(req *processRequest) ([]byte, error) {
	_, err := a.findDevice(req.DeviceID)
	if err != nil {
		return nil, err
	}

	cache, ok := a.caches[req.DeviceID]
	if !ok {
		return nil, ErrCacheNotFound
	}

	resp := map[string]any{
		"device_id": req.DeviceID,
		"timestamp": time.Now().Format(time.RFC3339),
		"cache":     cache.GetSnapshot(),
	}

	return json.Marshal(resp)
}

// processGetAllCaches 는 모든 디바이스의 캐시 스냅샷을 반환한다.
func (a *ModbusAgent) processGetAllCaches() ([]byte, error) {
	caches := make(map[string]any, len(a.devices))
	for _, dev := range a.devices {
		if cache, ok := a.caches[dev.config.ID]; ok {
			caches[dev.config.ID] = cache.GetSnapshot()
		}
	}

	resp := map[string]any{
		"timestamp": time.Now().Format(time.RFC3339),
		"caches":    caches,
	}

	return json.Marshal(resp)
}

// filterTypedValuesByRange 는 typed_values 맵에서 주소 범위 [startAddr, startAddr+quantity) 에
// 속하는 항목만 필터링하여 반환한다.
func filterTypedValuesByRange(typed map[uint16]any, startAddr, quantity uint16) map[uint16]any {
	filtered := make(map[uint16]any)
	endAddr := startAddr + quantity
	for addr, val := range typed {
		if addr >= startAddr && addr < endAddr {
			filtered[addr] = val
		}
	}
	return filtered
}

// findDevice 는 device ID 로 디바이스를 검색한다.
func (a *ModbusAgent) findDevice(deviceID string) (*ModbusDevice, error) {
	for _, dev := range a.devices {
		if dev.config.ID == deviceID {
			return dev, nil
		}
	}
	return nil, ErrDeviceNotFound
}

// buildCacheTypeOverlay 는 디바이스의 RegisterGroup 설정에서 TypeOverlay 맵을 구축한다.
// DataType 또는 TypeMap 이 설정된 경우에만 오버레이를 생성한다.
// 반환값이 nil 이면 해당 디바이스에 TypeOverlay 가 불필요하다.
func buildCacheTypeOverlay(groups []RegisterGroupConfig) map[string]modbus.TypeOverlayEntry {
	overlay := make(map[string]modbus.TypeOverlayEntry)

	for _, rg := range groups {
		// FC03/FC04 레지스터만 TypeOverlay 대상
		if rg.FunctionCode != FC03ReadHoldingRegisters && rg.FunctionCode != FC04ReadInputRegisters {
			continue
		}

		// TypeMap 이 있으면 각 엔트리를 개별 등록
		if len(rg.TypeMap) > 0 {
			for _, tm := range rg.TypeMap {
				key := fmt.Sprintf("FC%d:%d", rg.FunctionCode, tm.Address)
				regCount, err := modbus.RegisterCountForType(tm.DataType)
				if err != nil {
					// parseRegisterGroupConfig 에서 이미 DataType 유효성을 검증하므로
					// 여기서 에러가 발생할 가능성은 없다. 방어적 스킵.
					continue
				}
				byteOrder := tm.ByteOrder
				if byteOrder == "" {
					byteOrder = modbus.ByteOrderBigEndian
				}
				overlay[key] = modbus.TypeOverlayEntry{
					DataType:      tm.DataType,
					RegisterCount: regCount,
					ByteOrder:     byteOrder,
				}
			}
			continue
		}

		// DataType 이 설정된 경우 그룹 전체를 해당 타입으로 등록
		if rg.DataType != "" && rg.DataType != modbus.DataTypeUint16 {
			regCount, err := modbus.RegisterCountForType(rg.DataType)
			if err != nil {
				// parseRegisterGroupConfig 에서 이미 DataType 유효성을 검증하므로
				// 여기서 에러가 발생할 가능성은 없다. 방어적 스킵.
				continue
			}
			// 그룹 범위를 regCount 단위로 분할하여 오버레이 등록
			for addr := rg.StartAddress; addr+regCount <= rg.StartAddress+rg.Quantity; addr += regCount {
				key := fmt.Sprintf("FC%d:%d", rg.FunctionCode, addr)
				overlay[key] = modbus.TypeOverlayEntry{
					DataType:      rg.DataType,
					RegisterCount: regCount,
					ByteOrder:     modbus.ByteOrderBigEndian,
				}
			}
		}
	}

	if len(overlay) == 0 {
		return nil
	}
	return overlay
}

// initCacheTypeOverlays 는 모든 디바이스의 캐시에 TypeOverlay 를 설정한다.
func (a *ModbusAgent) initCacheTypeOverlays() {
	for _, dev := range a.devices {
		overlay := buildCacheTypeOverlay(dev.config.RegisterGroups)
		if overlay != nil {
			if cache, ok := a.caches[dev.config.ID]; ok {
				cache.SetTypeOverlay(overlay)
				a.logger.Info("modbus: TypeOverlay 설정 완료",
					"device", dev.config.ID,
					"entries", len(overlay),
				)
			}
		}
	}
}

// Configure 는 에이전트 설정을 업데이트한다.
func (a *ModbusAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("modbus configure: %w", err)
	}
	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()
	return nil
}

// ID 는 에이전트 ID 를 반환한다.
func (a *ModbusAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *ModbusAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *ModbusAgent) Type() string {
	return "modbus-tcp"
}

// Info 는 에이전트 정보의 스냅샷을 반환한다.
func (a *ModbusAgent) Info() agent.AgentInfo {
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
		Type:      "modbus-tcp",
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
func (a *ModbusAgent) BufferInfo() (int, int) {
	return len(a.msgCh), cap(a.msgCh)
}

// Stats 는 통계 스냅샷을 반환한다.
func (a *ModbusAgent) Stats() agent.StatsSnapshot {
	s := a.stats.Snapshot()
	s.MsgBufferPending, s.MsgBufferCapacity = a.BufferInfo()
	return s
}

// ConnectionStats 는 디바이스별 요청 처리 통계를 반환한다(M7, REQ-04).
// agent.ConnectionStatsProvider 인터페이스 구현. 각 디바이스를 하나의 연결 단위로 노출하며,
// 성공 요청 수는 MessagesReceived, 오류 요청 수는 MessagesErrored 로 매핑한다.
// 그룹별 통계와 지연(latency)은 State() 응답에서 추가로 표면화한다.
func (a *ModbusAgent) ConnectionStats() []agent.ConnectionStats {
	out := make([]agent.ConnectionStats, 0, len(a.devices))
	for _, dev := range a.devices {
		c, ok := a.devStats[dev.config.ID]
		if !ok {
			continue
		}
		out = append(out, agent.ConnectionStats{
			ID:               dev.config.ID,
			MessagesReceived: c.success.Load(),
			MessagesErrored:  c.errors.Load(),
		})
	}
	return out
}

// State 는 디바이스 요약 상태를 반환한다.
// agent.StatefulAgent 인터페이스 구현 — detail=full API 응답에 포함된다.
// 캐시 스냅샷과 레지스터 그룹 정보를 포함한다.
func (a *ModbusAgent) State() map[string]any {
	onlineCount := 0
	devices := make([]map[string]any, 0, len(a.devices))
	for _, dev := range a.devices {
		if dev.IsOnline() {
			onlineCount++
		}
		d := map[string]any{
			"device_id": dev.config.ID,
			"host":      dev.config.Host,
			"port":      dev.config.Port,
			"online":    dev.IsOnline(),
			"unit_id":   dev.UnitID(),
		}

		// 캐시 정보 추가
		if cache, ok := a.caches[dev.config.ID]; ok {
			d["cache"] = cache.GetSnapshot()
			staleGroups := cache.StaleGroups(a.config.StaleThreshold)
			if staleGroups == nil {
				staleGroups = []string{}
			}
			d["stale_groups"] = staleGroups
		}

		// 디바이스별 요청 통계 (M7)
		if c, ok := a.devStats[dev.config.ID]; ok {
			d["request_stats"] = requestStatsMap(c)
		}

		// 레지스터 그룹 정보 추가 — RLock 스냅샷으로 읽는다(M9, set_config copy-on-write 경합 방지).
		a.mu.RLock()
		rgSnapshot := dev.config.RegisterGroups
		a.mu.RUnlock()
		groups := make([]map[string]any, 0, len(rgSnapshot))
		for _, rg := range rgSnapshot {
			groupKey := fmt.Sprintf("FC%d_%d", rg.FunctionCode, rg.StartAddress)
			g := map[string]any{
				"name":          rg.Name,
				"function_code": rg.FunctionCode,
				"start_address": rg.StartAddress,
				"quantity":      rg.Quantity,
			}
			// 그룹별 폴링 주기 (M5): 지정 시 노출, 미지정이면 기본 주기 폴백
			if rg.PollInterval > 0 {
				g["poll_interval"] = rg.PollInterval.String()
			}
			if cache, ok := a.caches[dev.config.ID]; ok {
				g["stale"] = cache.IsStale(groupKey, a.config.StaleThreshold)
				cache.mu.RLock()
				if t, exists := cache.LastUpdateTime[groupKey]; exists {
					g["last_update"] = t.Format(time.RFC3339)
				}
				cache.mu.RUnlock()
			}
			// 그룹별 요청 통계 (M7)
			if c, ok := a.groupStats[groupStatKey(dev.config.ID, rg.Name)]; ok {
				g["request_stats"] = requestStatsMap(c)
			}
			groups = append(groups, g)
		}
		d["register_groups"] = groups

		devices = append(devices, d)
	}

	return map[string]any{
		"device_count": len(a.devices),
		"online_count": onlineCount,
		"read_mode":    a.config.ReadMode,
		"devices":      devices,
	}
}

// SetPollInterval 은 런타임에 폴링 간격을 변경한다.
// agent.PollingConfigurable 인터페이스 구현.
// 최소 100ms 이상이어야 하며, pollLoop 가 실행 중이면 즉시 반영된다.
func (a *ModbusAgent) SetPollInterval(d time.Duration) error {
	if d < 100*time.Millisecond {
		return fmt.Errorf("modbus: poll interval must be >= 100ms, got %v", d)
	}

	a.mu.Lock()
	a.config.PollInterval = d
	a.mu.Unlock()

	a.mu.RLock()
	wasStarted := a.started
	a.mu.RUnlock()

	// pollLoop 고루틴에 리셋 시그널 전송 (논블로킹)
	select {
	case a.pollResetCh <- d:
		a.logger.Debug("modbus: pollResetCh 전송 성공", "interval", d, "pollLoopRunning", wasStarted)
	default:
		a.logger.Debug("modbus: pollResetCh 전송 스킵 (채널 가득참)", "interval", d, "pollLoopRunning", wasStarted)
	}

	a.logger.Info("modbus: 폴링 간격 변경 요청", "interval", d)
	return nil
}

// truncateForLog 는 바이트 데이터를 로깅용으로 잘라서 문자열로 반환한다.
func truncateForLog(data []byte, maxLen int) string {
	if len(data) <= maxLen {
		return string(data)
	}
	return string(data[:maxLen]) + "..."
}
