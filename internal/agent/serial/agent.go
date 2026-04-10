package serial

import (
	"context"
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
	mu          sync.Mutex // Write 직렬화 및 port 접근 보호
	connected   atomic.Bool
	paused      atomic.Bool
	stopCh      chan struct{}
	stopOnce    sync.Once
	wg          sync.WaitGroup
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
	if a.CurrentState() != lifecycle.StateRunning {
		return fmt.Errorf("serial agent: not in running state (current: %s)", a.CurrentState())
	}

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
	// gap_timeout이 설정되면 프레임 간격 감지에 사용한다 (모든 프레이밍 모드).
	// 스트림 모드에서는 idle_timeout을 사용하여 프레임 경계를 감지한다.
	readTimeout := a.config.ReadTimeout
	if a.config.GapTimeout > 0 {
		readTimeout = a.config.GapTimeout
	} else if a.config.Framing == FramingStream {
		readTimeout = a.config.IdleTimeout
	}
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

		// 일시 정지 상태이면 데이터를 버린다
		if a.paused.Load() {
			continue
		}

		// msgCh 에 비차단 전송 (가득 차면 드롭)
		select {
		case a.msgCh <- data:
		default:
			a.logger.Warn("시리얼 에이전트 메시지 버퍼 가득 참, 드롭")
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

	// 고루틴 종료 대기
	a.wg.Wait()

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

	err := a.framer.Write(port, data)
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		return nil, fmt.Errorf("serial agent: write failed: %w", err)
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
