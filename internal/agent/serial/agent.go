package serial

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	goserial "go.bug.st/serial"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/framing"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// serialPort 는 테스트를 위한 시리얼 포트 추상화이다.
// go.bug.st/serial.Port 는 이 인터페이스를 이미 만족한다.
type serialPort interface {
	io.ReadWriteCloser
	// SetReadTimeout 은 읽기 타임아웃을 설정한다.
	SetReadTimeout(t time.Duration) error
}

// serialOpener 는 시리얼 포트 열기 함수 타입이다.
type serialOpener func(portName string, mode *goserial.Mode) (goserial.Port, error)

// defaultOpener 는 프로덕션용 기본 포트 오프너이다.
var defaultOpener serialOpener = goserial.Open

// SerialAgent 는 시리얼 포트를 통해 데이터를 송수신하는 에이전트이다.
// USB 장치 분리 감지 및 Pause/Resume 을 지원한다.
type SerialAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig agent.AgentConfig
	config      SerialConfig
	port        serialPort   // 테스트를 위해 인터페이스로 추상화
	opener      serialOpener // 포트 열기 함수 (테스트 시 교체 가능)
	framer      framing.Framer
	reader      *SerialConnReader
	msgCh       chan []byte
	rawCh       chan []byte // 원시 바이트 채널 (프레이밍 이전, raw_out 포트용)
	stats       *agent.AgentStats
	logger      *slog.Logger
	startedAt   time.Time
	createdAt   time.Time
	// mu 는 구조체 필드 (port 포인터, agentConfig, startedAt 등) 에 대한 접근을
	// 보호한다. 실제 물리 포트 I/O 는 보호하지 않는다 — 그것은 portIOMu 의 역할이다.
	mu sync.Mutex
	// portIOMu 는 half-duplex 모드에서 물리 포트 I/O 를 직렬화하는 실제 mutex 이다.
	// RS-485 half-duplex 버스는 송신과 수신을 물리적으로 동시에 수행할 수 없으므로,
	// Write 와 Read 가 같은 포트에서 절대 겹치지 않도록 이 mutex 로 직렬화한다.
	// (mu 와 분리한 이유: readLoop 가 per-sub-read 마다 portIOMu 를 획득/해제하므로,
	// mu 와 섞으면 lock-ordering cycle / deadlock 위험이 있다. 두 mutex 의 임계 구역은
	// 절대 중첩하지 않는다 — mu 로 port 포인터를 읽고 해제한 뒤 portIOReader 가
	// per-sub-read 마다 portIOMu 를 획득한다.)
	//
	// 고정(2025-01-28): 이전 설계는 readLoop 가 a.reader.Read() 동안 portIOMu 를
	// 통째로 쥐었으나, length_prefix/frame 등의 프레이밍은 io.ReadFull 로 완전한
	// 프레임이 조립될 때까지 여러 하위 read 를 루프한다. 장비가 유휴/노이즈만 보내
	// 프레임이 완성 안 되면 readLoop 가 포트 조립 내내 (다수의 read 타임아웃 사이클
	// 동안) 락을 점유했고, 그 사이 Process 의 Write 는 락을 얻지 못해 상위 write_timeout
	// (예: 5초) 에 걸려 starve 되었다. 수정: portIOReader 가 각 하위 read 마다
	// per-sub-read 로 락을 획득/해제하므로, 프레임 조립 루프 중간에 락이 자유로워져서
	// Process 의 Write 가 SetReadTimeout 으로 bounded 된 타임아웃 범위 내에 진행된다.
	// 이 굶주림은 full-duplex 포트에서는 불필요하므로 half_duplex=false 로 회피한다
	// (portIOLock 참고).
	portIOMu sync.Mutex
	// portIOLock 은 readLoop (portIOReader 를 거쳐 per-sub-read 마다 획득/해제) 와
	// Process (framer.Write 전후) 가 실제로 사용하는 포트 I/O 락이다.
	// half_duplex=true (기본) 이면 &portIOMu 를 가리켜 per-sub-read 마다 직렬화하고,
	// half_duplex=false (full-duplex) 이면 no-op locker 로 대체되어 read 와 write 가
	// 서로 배제하지 않는다 — go.bug.st/serial 의 Read/Write 는 같은 fd 에 대한 독립
	// syscall 이므로 full-duplex 포트에서 동시 호출이 안전하다. NewSerialAgent 에서
	// 한 번만 설정되며 이후 불변이므로 별도 동기화가 필요 없다.
	portIOLock sync.Locker
	connected  atomic.Bool
	paused     atomic.Bool
	stopCh     chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
}

// 컴파일 타임 인터페이스 구현 확인.
var _ agent.Agent = (*SerialAgent)(nil)
var _ agent.MessageReceiver = (*SerialAgent)(nil)
var _ agent.StatefulAgent = (*SerialAgent)(nil)
var _ agent.TransportChecker = (*SerialAgent)(nil)
var _ agent.BufferInfoProvider = (*SerialAgent)(nil)
var _ agent.RawMessageReceiver = (*SerialAgent)(nil)

// NewSerialAgent 는 새 SerialAgent 를 생성한다.
// 설정을 파싱하고 프레이머를 생성하지만, 포트는 Start 에서 연다.
func NewSerialAgent(agentConfig agent.AgentConfig) (agent.Agent, error) {
	cfg, err := ParseSerialConfig(agentConfig.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("serial agent: %w", err)
	}

	framer, err := framing.New(cfg.Framing, framing.Options{
		BufferSize:           cfg.BufferSize,
		Delimiter:            cfg.Delimiter,
		FixedSize:            cfg.FixedSize,
		MaxMessageSize:       cfg.MaxMessageSize,
		STX:                  cfg.STX,
		ETX:                  cfg.ETX,
		LengthOffset:         cfg.LengthOffset,
		LengthSize:           cfg.LengthSize,
		LengthEndian:         cfg.LengthEndian,
		LengthIncludesHeader: cfg.LengthIncludesHeader,
		LengthAdjustment:     cfg.LengthAdjustment,
		Checksum:             cfg.Checksum,
	})
	if err != nil {
		return nil, fmt.Errorf("serial agent: %w", err)
	}

	logger := agentConfig.Logger
	if logger == nil {
		logger = slog.Default()
	}

	a := &SerialAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("serial-" + agentConfig.ID)),
		agentConfig:   agentConfig,
		config:        cfg,
		opener:        defaultOpener,
		framer:        framer,
		msgCh:         make(chan []byte, 256),
		rawCh:         make(chan []byte, 256),
		stats:         agent.NewAgentStats(),
		logger:        logger,
		createdAt:     time.Now(),
		stopCh:        make(chan struct{}),
	}

	// 포트 I/O 락 선택: half_duplex 면 portIOMu 로 read/write 를 직렬화하고,
	// full-duplex 면 no-op locker 로 두어 read 가 write 를 굶기지 않게 한다.
	if cfg.HalfDuplex {
		a.portIOLock = &a.portIOMu
	} else {
		a.portIOLock = noopLocker{}
	}

	if err := a.init(agentConfig); err != nil {
		return nil, err
	}

	return a, nil
}

// init 은 에이전트를 초기화하고 Running 상태로 전이한다.
func (a *SerialAgent) init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("serial agent init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("serial agent init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("serial agent init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	return nil
}

// Init 은 에이전트를 초기화한다 (Agent 인터페이스).
func (a *SerialAgent) Init(config agent.AgentConfig) error {
	return a.init(config)
}

// Start 는 시리얼 포트를 열고 readLoop 를 시작한다.
func (a *SerialAgent) Start(_ context.Context) error {
	// Error / Stopped 상태에서 재시작을 지원하기 위해 유효한 전이 경로를 거쳐
	// Running 으로 이동한다 (2026-05-14 hotfix: 시리얼 포트 분리 감지로 StateError
	// 진입 후 사용자가 Start API 호출 시 영구 stuck 되는 회귀 해소).
	switch a.CurrentState() {
	case lifecycle.StateRunning:
		// 이미 Running — pass-through
	case lifecycle.StateError:
		_ = a.TransitionTo(lifecycle.StateStopping)
		_ = a.TransitionTo(lifecycle.StateStopped)
		fallthrough
	case lifecycle.StateStopped:
		if err := a.TransitionTo(lifecycle.StateCreated); err != nil {
			return fmt.Errorf("serial agent: transition Stopped -> Created: %w", err)
		}
		fallthrough
	case lifecycle.StateCreated:
		if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
			return fmt.Errorf("serial agent: transition Created -> Initializing: %w", err)
		}
		fallthrough
	case lifecycle.StateInitializing:
		if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
			return fmt.Errorf("serial agent: transition Initializing -> Running: %w", err)
		}
	default:
		return fmt.Errorf("serial agent: cannot start from state %s", a.CurrentState())
	}

	// Fix A: 재시작 (동일 인스턴스에 대한 Stop → Start) 안전성 확보.
	// 이전 Stop 에서 close(stopCh) + stopOnce 소진 상태가 남아있으면 새 readLoop 가
	// 닫힌 stopCh 를 만나 즉시 종료되어 데이터를 전혀 수신하지 못한다.
	// 이를 방지하기 위해 새 readLoop 를 spawn 하기 전에 라이프사이클 자원을 리셋한다.
	//
	// 주의: sync.WaitGroup 은 Wait 가 반환된 이후 재사용 가능하므로 별도 리셋이 필요 없다
	// (sync 패키지 문서: "Note that calls with a positive delta that occur when the counter
	// is zero must happen before a Wait"). Stop 의 wg.Wait 가 반환된 직후 호출되므로
	// 카운터는 0 인 상태가 보장된다.
	a.mu.Lock()
	a.stopCh = make(chan struct{})
	a.stopOnce = sync.Once{}
	a.mu.Unlock()

	mode := &goserial.Mode{
		BaudRate: a.config.BaudRate,
		DataBits: a.config.DataBits,
		Parity:   parityFromString(a.config.Parity),
		StopBits: stopBitsFromInt(a.config.StopBits),
	}

	port, err := a.opener(a.config.Port, mode)
	if err != nil {
		return fmt.Errorf("serial agent: open port %s: %w", a.config.Port, err)
	}

	// 읽기 타임아웃 설정 (readLoop 에서 stopCh 검사 주기)
	readTimeout := effectiveReadTimeout(a.config)
	if err := port.SetReadTimeout(readTimeout); err != nil {
		port.Close()
		return fmt.Errorf("serial agent: set read timeout: %w", err)
	}

	a.mu.Lock()
	a.port = port
	a.mu.Unlock()

	a.connected.Store(true)
	// raw_out 지원: 포트에서 읽은 원시 바이트를 rawCh 로 복사 전송
	var portReader io.Reader = port
	// portIOReader 는 각 하위 read 호출마다 portIOLock 을 획득/해제한다.
	// 이를 통해 framer 의 프레임 조립 루프 중간에 락이 자유로워져서 Process 의 Write 가
	// SetReadTimeout 으로 bounded 된 per-sub-read 타임아웃 범위 내에 진행된다.
	// half_duplex=true 이면 RS-485 half-duplex 안전성을 유지하고,
	// half_duplex=false 이면 no-op locker 로 read/write 직렬화가 사라져 write starvation 을 회피한다.
	portReader = &portIOReader{inner: portReader, lock: a.portIOLock}
	portReader = &rawTeeReader{reader: portReader, rawCh: a.rawCh}
	a.reader = NewSerialConnReader(a.framer, portReader)

	a.logger.Info("시리얼 포트 열림",
		"port", a.config.Port,
		"baud_rate", a.config.BaudRate)

	a.wg.Add(1)
	go a.readLoop()

	return nil
}

// readLoop 는 시리얼 포트에서 데이터를 읽어 msgCh 로 전달하는 고루틴이다.
// 일시 정지 상태에서는 읽은 데이터를 버린다.
// USB 장치 분리 시 Error 상태로 전이한다.
func (a *SerialAgent) readLoop() {
	defer a.wg.Done()

	for {
		select {
		case <-a.stopCh:
			return
		default:
		}

		// a.reader.Read() 는 내부적으로 portIOReader 를 거쳐 각 하위 read 호출마다
		// portIOLock 을 획득/해제한다. 이를 통해:
		// - half_duplex=true: 매 하위 read 타임아웃마다 락이 자유로워져서 Process 의
		//   Write 가 SetReadTimeout 으로 bounded 된 시간 내에 진행된다.
		// - half_duplex=false: no-op locker 이므로 read 와 write 가 직렬화되지 않아
		//   프레임 조립 중에 Process 의 Write 가 굶지 않는다.
		// Stop 은 portIOLock 을 획득하지 않고 port.Close() 로 mid-read 를 풀어준다.
		data, err := a.reader.Read()
		if err != nil {
			// stopCh 가 닫혔으면 정상 종료
			select {
			case <-a.stopCh:
				return
			default:
			}

			// USB 장치 분리 감지
			if isDisconnectError(err) {
				a.connected.Store(false)
				a.logger.Error("시리얼 포트 분리 감지", "port", a.config.Port, "error", err)
				_ = a.TransitionTo(lifecycle.StateError)
				return
			}

			// 읽기 타임아웃은 무시 (정상 동작)
			if isTimeoutError(err) {
				continue
			}

			// 프레이밍 에러 (ETX 불일치, 체크섬 불일치, 프레임 크기 초과)는
			// 해당 프레임만 버리고 다음 프레임을 시도한다.
			if isFramingError(err) {
				a.logger.Warn("시리얼 프레이밍 오류 (재시도)", "error", err)
				continue
			}

			// EOF 또는 기타 오류
			a.connected.Store(false)
			a.logger.Warn("시리얼 포트 읽기 오류", "error", err)
			_ = a.TransitionTo(lifecycle.StateError)
			return
		}

		// 빈 데이터 무시
		if len(data) == 0 {
			continue
		}

		a.stats.IncrExternalMessagesReceived()
		a.stats.AddBytesRead(int64(len(data)))
		a.stats.UpdateLastActivity()

		// log_messages: 수신(RX) 메시지를 hex 로 INFO 로그 (opt-in 진단용).
		if a.config.LogMessages {
			a.logger.Info("serial: RX", "port", a.config.Port, "len", len(data), "hex", hex.EncodeToString(data))
		}

		// 일시 정지 상태이면 데이터를 버린다
		if a.paused.Load() {
			continue
		}

		// msgCh 에 비차단 전송 (가득 차면 드롭)
		select {
		case a.msgCh <- data:
		default:
			a.stats.IncrDroppedMessages()
			if a.config.LogDrops {
				a.logger.Warn("시리얼 에이전트 메시지 버퍼 가득 참, 드롭")
			}
		}
	}
}

// Stop 는 에이전트를 정지하고 시리얼 포트를 닫는다.
func (a *SerialAgent) Stop(_ context.Context) error {
	a.stopOnce.Do(func() {
		close(a.stopCh)
	})

	// 포트 닫기
	a.mu.Lock()
	port := a.port
	a.port = nil
	a.mu.Unlock()

	if port != nil {
		port.Close()
	}

	a.connected.Store(false)

	// Fix B: 고루틴 종료를 유한 시간 동안 대기한다.
	// 일부 USB 시리얼 드라이버는 장치 물리적 분리 시 port.Close() 가 진행 중인
	// read syscall 을 즉시 풀어주지 못해 readLoop 가 OS 시스템 콜에서 무한
	// 블로킹된다. 이 경우 wg.Wait() 를 무제한 호출하면 Stop 자체가 영구히
	// 반환되지 않아 상위 계층 (Manager.Restart 등) 까지 hang 된다.
	//
	// 5초는 정상 driver 의 read timeout 사이클 + cleanup 여유를 모두 포괄하면서
	// 사용자가 체감하기에 과도하지 않은 상한값이다.
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 정상 종료.
	case <-time.After(5 * time.Second):
		a.logger.Warn("시리얼 에이전트: readLoop 종료 대기 시간 초과 — 강제 진행 (포트 분리 등의 OS 레벨 hang 가능성)",
			"port", a.config.Port)
	}

	// 상태 전이
	current := a.CurrentState()
	switch current {
	case lifecycle.StateError:
		_ = a.TransitionTo(lifecycle.StateStopped)
	case lifecycle.StateStopped:
		// 이미 정지됨
	default:
		_ = a.TransitionTo(lifecycle.StateStopping)
		_ = a.TransitionTo(lifecycle.StateStopped)
	}

	return nil
}

// Pause 는 에이전트를 일시 정지한다.
// 포트는 열린 채로 유지되며, 수신 데이터는 버려진다.
func (a *SerialAgent) Pause(_ context.Context) error {
	a.paused.Store(true)
	return a.TransitionTo(lifecycle.StatePaused)
}

// Resume 은 에이전트를 재개한다.
func (a *SerialAgent) Resume(_ context.Context) error {
	a.paused.Store(false)
	return a.TransitionTo(lifecycle.StateRunning)
}

// Health 는 에이전트의 헬스 상태를 반환한다.
func (a *SerialAgent) Health() agent.HealthStatus {
	now := time.Now()
	state := a.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		if a.connected.Load() {
			return agent.HealthStatus{
				Status:    agent.HealthHealthy,
				LastCheck: now,
				Message:   "serial port connected",
			}
		}
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "serial port not connected",
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "agent is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("agent is in %s state", state),
		}
	}
}

// Process 는 데이터를 시리얼 포트로 전송한다.
func (a *SerialAgent) Process(data []byte) ([]byte, error) {
	if a.CurrentState() != lifecycle.StateRunning {
		return nil, ErrNotRunning
	}

	if !a.connected.Load() {
		return nil, fmt.Errorf("serial agent: port not connected")
	}

	a.mu.Lock()
	port := a.port
	a.mu.Unlock()

	if port == nil {
		return nil, fmt.Errorf("serial agent: port is nil")
	}

	// portIOLock 으로 물리 포트 Write 를 감싼다. half_duplex=true 이면 portIOMu 로서
	// readLoop 의 reader.Read (portIOReader 를 거쳐 per-sub-read 로 락을 획득/해제)
	// 와 직렬화되어 RS-485 half-duplex 에서 송수신이 겹치지 않도록 보장한다.
	// half_duplex=false 이면 no-op locker 이므로 블로킹 read 와 무관하게 즉시 Write
	// 를 수행한다.
	// 주의: mu 는 위에서 이미 해제했으므로 두 lock 의 임계 구역은 중첩하지 않는다.
	a.portIOLock.Lock()
	err := a.framer.Write(port, data)
	a.portIOLock.Unlock()
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		return nil, fmt.Errorf("serial agent: write failed: %w", err)
	}

	// log_messages: 송신(TX) 메시지를 hex 로 INFO 로그 (opt-in 진단용). 비활성 시 기존 Debug 유지.
	if a.config.LogMessages {
		a.logger.Info("serial: TX", "port", a.config.Port, "len", len(data), "hex", hex.EncodeToString(data))
	} else {
		a.logger.Debug("시리얼 포트 출력",
			"port", a.config.Port,
			"len", len(data),
			"hex", hex.EncodeToString(data))
	}

	a.stats.IncrExternalMessagesSent()
	a.stats.AddBytesWritten(int64(len(data)))
	a.stats.UpdateLastActivity()

	return nil, nil
}

// Configure 는 에이전트 설정을 업데이트한다.
func (a *SerialAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("serial agent configure: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	return nil
}

// ReceiveMessage 는 수신된 메시지를 반환한다.
func (a *SerialAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case data := <-a.msgCh:
		return data, nil
	case <-a.stopCh:
		return nil, fmt.Errorf("serial agent: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ReceiveRawMessage 는 프레이밍 이전의 원시 바이트 채널을 반환한다.
func (a *SerialAgent) ReceiveRawMessage() <-chan []byte {
	return a.rawCh
}

// State 는 에이전트의 런타임 상태를 반환한다.
func (a *SerialAgent) State() map[string]any {
	return map[string]any{
		"connected": a.connected.Load(),
		"port":      a.config.Port,
		"baud_rate": a.config.BaudRate,
		"paused":    a.paused.Load(),
	}
}

// TransportConnected 는 시리얼 포트 연결 여부를 반환한다.
func (a *SerialAgent) TransportConnected() bool {
	return a.connected.Load()
}

// BufferInfo 는 메시지 버퍼 사용 현황을 반환한다.
func (a *SerialAgent) BufferInfo() (pending int, capacity int) {
	return len(a.msgCh), cap(a.msgCh)
}

// ID 는 에이전트 ID 를 반환한다.
func (a *SerialAgent) ID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *SerialAgent) Name() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *SerialAgent) Type() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.agentConfig.Type
}

// Info 는 에이전트 정보 스냅샷을 반환한다.
func (a *SerialAgent) Info() agent.AgentInfo {
	a.mu.Lock()
	cfg := a.agentConfig
	startedAt := a.startedAt
	createdAt := a.createdAt
	a.mu.Unlock()

	state := a.CurrentState()
	var uptime time.Duration
	if state == lifecycle.StateRunning && !startedAt.IsZero() {
		uptime = time.Since(startedAt)
	}

	return agent.AgentInfo{
		ID:        cfg.ID,
		Name:      cfg.Name,
		Type:      cfg.Type,
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
func (a *SerialAgent) Stats() agent.StatsSnapshot {
	return a.stats.Snapshot()
}

// minReadTimeout 은 SetReadTimeout 에 전달할 수 있는 최소 양수 타임아웃이다.
// 사용자가 read_timeout / idle_timeout 을 0 이나 음수로 설정한 경우 (또는
// time.ParseDuration 이 그런 값을 허용한 경우) 이 값으로 보정한다.
//
// 비양수 타임아웃은 일부 시리얼 드라이버 (go.bug.st/serial 포함) 에서
// 무한 블로킹 (NoTimeout) 을 의미한다. readLoop 는 reader.Read 동안 portIOMu 를
// 점유하므로, 무한 블로킹은 곧 Process(Write)의 영구 starvation 으로 이어진다.
// 따라서 effective timeout 은 반드시 유한 양수여야 한다.
const minReadTimeout = 200 * time.Millisecond

// effectiveReadTimeout 은 SetReadTimeout 에 전달할 실제 읽기 타임아웃을 계산한다.
//
// gap_timeout 이 설정되면 프레임 간격 감지에 사용한다 (모든 프레이밍 모드).
// 스트림 모드에서는 idle_timeout 을 사용하여 프레임 경계를 감지한다.
// 그 외에는 read_timeout 을 사용한다.
//
// 계산된 값이 비양수이면 minReadTimeout 으로 보정한다 — 이는 readLoop 가
// portIOMu 를 쥔 채 무한 블로킹되어 Write 를 starve 시키는 것을 방지한다.
// 양수 값은 (1ms 같은 작은 값 포함) 그대로 보존되므로 기존 동작에 영향이 없다.
func effectiveReadTimeout(cfg SerialConfig) time.Duration {
	readTimeout := cfg.ReadTimeout
	if cfg.GapTimeout > 0 {
		readTimeout = cfg.GapTimeout
	} else if cfg.Framing == FramingStream {
		readTimeout = cfg.IdleTimeout
	}
	if readTimeout <= 0 {
		return minReadTimeout
	}
	return readTimeout
}

// isDisconnectError 는 USB 장치 분리 등으로 인한 연결 오류인지 판별한다.
// ENXIO (No such device or address), EIO (I/O error) 를 감지한다.
func isDisconnectError(err error) bool {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.ENXIO || errno == syscall.EIO
	}
	return false
}

// isTimeoutError 는 읽기 타임아웃 오류인지 판별한다.
func isTimeoutError(err error) bool {
	// go.bug.st/serial 은 타임아웃 시 빈 데이터(n=0)를 반환하거나
	// os.ErrDeadlineExceeded 를 반환할 수 있다.
	// 하지만 실제로는 n=0, err=nil 을 반환하는 경우가 대부분이므로
	// 여기서는 명시적 타임아웃 에러만 처리한다.
	return errors.Is(err, context.DeadlineExceeded)
}

// isFramingError 는 프레이밍 수준 에러인지 판별한다.
// ETX 불일치, 체크섬 불일치, 프레임 크기 초과 등은 해당 프레임만 무효이며
// 다음 프레임부터 정상 수신이 가능하다.
func isFramingError(err error) bool {
	return errors.Is(err, ErrETXMismatch) ||
		errors.Is(err, ErrChecksumMismatch) ||
		errors.Is(err, ErrFrameTooLarge)
}

// parityFromString 은 문자열 패리티 값을 goserial.Parity 로 변환한다.
func parityFromString(s string) goserial.Parity {
	switch s {
	case "even":
		return goserial.EvenParity
	case "odd":
		return goserial.OddParity
	case "mark":
		return goserial.MarkParity
	case "space":
		return goserial.SpaceParity
	default:
		return goserial.NoParity
	}
}

// stopBitsFromInt 는 정수 스톱 비트 값을 goserial.StopBits 로 변환한다.
func stopBitsFromInt(n int) goserial.StopBits {
	switch n {
	case 2:
		return goserial.TwoStopBits
	default:
		return goserial.OneStopBit
	}
}

// noopLocker 는 아무 동작도 하지 않는 sync.Locker 이다.
// full-duplex 포트 (half_duplex=false) 에서 portIOLock 으로 사용되어 물리 포트
// read/write 를 직렬화하지 않는다 — TX/RX 가 물리적으로 분리되어 동시 접근이
// 안전하므로, 블로킹 read 가 write 를 굶기는 half-duplex 결함을 회피한다.
type noopLocker struct{}

func (noopLocker) Lock()   {}
func (noopLocker) Unlock() {}

// rawTeeReader 는 io.Reader 를 감싸서 읽은 원시 바이트를 rawCh 로 비차단 전송한다.
type rawTeeReader struct {
	reader io.Reader
	rawCh  chan []byte
}

func (r *rawTeeReader) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	if n > 0 {
		cp := make([]byte, n)
		copy(cp, p[:n])
		// 비차단 전송 — rawCh 가 가득 차면 드롭
		select {
		case r.rawCh <- cp:
		default:
		}
	}
	return n, err
}

// portIOReader 는 underlying 포트 리더를 감싸서 각 단일 Read 호출마다
// portIOLock 을 획득/해제한다. 이를 통해 프레이머의 프레임 조립 루프가
// 여러 하위 read 를 반복 호출하는 동안에도 각 하위 read 마다 락이 자유로워져서
// Process 의 framer.Write 가 SetReadTimeout 으로 bounded 된 per-sub-read 타임아웃
// 범위 내에 진행될 수 있다. half_duplex=true 이면 RS-485 half-duplex 안전성을
// 유지하고 (per-sub-read 마다 락으로 직렬화), half_duplex=false 이면 no-op locker
// 로 read/write 직렬화가 사라져 write starvation 을 회피한다.
type portIOReader struct {
	inner io.Reader
	lock  sync.Locker
}

func (r *portIOReader) Read(p []byte) (int, error) {
	r.lock.Lock()
	n, err := r.inner.Read(p)
	r.lock.Unlock()
	return n, err
}
