package serial

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goserial "go.bug.st/serial"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// --- 테스트용 목 시리얼 포트 ---

// mockSerialPort 는 테스트를 위한 시리얼 포트 모의 구현이다.
type mockSerialPort struct {
	readBuf  *bytes.Buffer // Read 가 반환할 데이터
	writeBuf *bytes.Buffer // Write 에 기록된 데이터
	closed   atomic.Bool
	readErr  error // Read 가 반환할 에러
	writeErr error // Write 가 반환할 에러
	mu       sync.Mutex
}

func newMockSerialPort() *mockSerialPort {
	return &mockSerialPort{
		readBuf:  bytes.NewBuffer(nil),
		writeBuf: bytes.NewBuffer(nil),
	}
}

func (m *mockSerialPort) Read(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed.Load() {
		return 0, io.EOF
	}
	if m.readErr != nil {
		return 0, m.readErr
	}
	if m.readBuf.Len() == 0 {
		// 데이터가 없으면 타임아웃처럼 빈 응답 반환
		return 0, io.EOF
	}
	return m.readBuf.Read(p)
}

func (m *mockSerialPort) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	return m.writeBuf.Write(p)
}

func (m *mockSerialPort) Close() error {
	m.closed.Store(true)
	return nil
}

func (m *mockSerialPort) SetReadTimeout(_ time.Duration) error {
	return nil
}

// feedData 는 목 포트에 읽기용 데이터를 추가한다.
func (m *mockSerialPort) feedData(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readBuf.Write(data)
}

// getWrittenData 는 목 포트에 기록된 데이터를 반환한다.
func (m *mockSerialPort) getWrittenData() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writeBuf.Bytes()
}

// setReadError 는 다음 Read 호출이 반환할 에러를 설정한다.
func (m *mockSerialPort) setReadError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readErr = err
}

// --- blockingMockSerialPort: readLoop 테스트를 위한 블로킹 목 ---

// blockingMockSerialPort 는 채널 기반으로 읽기를 제어하는 목 포트이다.
//
// 실제 go.bug.st/serial 포트와 마찬가지로 SetReadTimeout 계약을 준수한다:
// dataCh/errCh 에 아무것도 오지 않으면 readTimeout 경과 후 (0, nil) 을
// 반환한다 (타임아웃 시 빈 응답). 이 동작은 RS-485 half-duplex I/O 직렬화
// (portIOMu) 도입 이후 필수이다 — readLoop 는 reader.Read 동안 portIOMu 를
// 점유하므로, Read 가 무한 블로킹되면 Process(Write)가 영구 starve 된다.
// 실제 포트는 SetReadTimeout 으로 bounded 되므로 이 목도 동일하게 동작해야
// 충실한 (faithful) 테스트 더블이 된다.
type blockingMockSerialPort struct {
	dataCh      chan []byte
	errCh       chan error
	writeBuf    *bytes.Buffer
	closed      atomic.Bool
	writeErr    error        // Write 호출 시 반환할 에러 (nil 이면 정상). readLoop 에 영향 없음.
	readTimeout atomic.Int64 // SetReadTimeout 으로 설정되는 읽기 타임아웃 (ns)
	mu          sync.Mutex
}

// defaultMockReadTimeout 은 SetReadTimeout 이 호출되지 않은 경우
// (예: createAgentWithPortOverride 처럼 Start 를 우회하는 경로) 사용되는
// 기본 읽기 타임아웃이다.
const defaultMockReadTimeout = 50 * time.Millisecond

func newBlockingMockSerialPort() *blockingMockSerialPort {
	m := &blockingMockSerialPort{
		dataCh:   make(chan []byte, 10),
		errCh:    make(chan error, 1),
		writeBuf: bytes.NewBuffer(nil),
	}
	m.readTimeout.Store(int64(defaultMockReadTimeout))
	return m
}

func (m *blockingMockSerialPort) Read(p []byte) (int, error) {
	if m.closed.Load() {
		return 0, io.EOF
	}

	to := time.Duration(m.readTimeout.Load())
	select {
	case data := <-m.dataCh:
		n := copy(p, data)
		return n, nil
	case err := <-m.errCh:
		return 0, err
	case <-time.After(to):
		// 실제 포트의 읽기 타임아웃 동작: 데이터 없음 → (0, nil).
		return 0, nil
	}
}

func (m *blockingMockSerialPort) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	if m.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	return m.writeBuf.Write(p)
}

// failWrites 는 이후 Write 호출이 항상 주어진 에러를 반환하도록 구성한다.
// readLoop 에는 영향을 주지 않으므로, 에이전트 상태 (Running) 가 변경되지 않는다.
// "Write 만 실패하는" 시나리오 (예: 포트는 살아있지만 일시적 IO 실패) 재현용.
func (m *blockingMockSerialPort) failWrites(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeErr = err
}

func (m *blockingMockSerialPort) Close() error {
	m.closed.Store(true)
	// 블로킹된 Read 를 풀어주기 위해 에러 전송
	select {
	case m.errCh <- io.EOF:
	default:
	}
	return nil
}

func (m *blockingMockSerialPort) SetReadTimeout(t time.Duration) error {
	// 실제 포트와 동일하게 읽기 타임아웃을 반영한다.
	// 비양수 값은 무한 블로킹을 의미하지만, 프로덕션 코드의 effectiveReadTimeout
	// 이 항상 양수를 보장하므로 여기서는 받은 값을 그대로 저장한다.
	if t > 0 {
		m.readTimeout.Store(int64(t))
	}
	return nil
}

// --- 테스트 헬퍼 ---

// makeSerialConfig 는 테스트용 AgentConfig 를 생성한다.
func makeSerialConfig() agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "test-serial",
		Name: "Test Serial",
		Type: "serial",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"port":         "/dev/ttyUSB0",
				"baud_rate":    9600,
				"data_bits":    8,
				"parity":       "none",
				"stop_bits":    float64(1),
				"read_timeout": "100ms",
			},
		},
	}
}

// createAgentWithMock 는 목 포트를 사용하는 SerialAgent 를 생성하고 시작한다.
func createAgentWithMock(t *testing.T, mock serialPort) *SerialAgent {
	t.Helper()

	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)
	// 목 오프너로 교체
	sa.opener = func(_ string, _ *goserial.Mode) (goserial.Port, error) {
		return mock.(goserial.Port), nil
	}

	return sa
}

// createAgentWithPortOverride 는 목 포트를 직접 주입하는 SerialAgent 를 생성한다.
// goserial.Port 인터페이스를 만족하지 않는 목을 위한 헬퍼.
func createAgentWithPortOverride(t *testing.T) (*SerialAgent, *blockingMockSerialPort) {
	t.Helper()

	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)
	mock := newBlockingMockSerialPort()

	// Start 대신 직접 포트를 설정
	sa.mu.Lock()
	sa.port = mock
	sa.mu.Unlock()
	sa.connected.Store(true)
	sa.reader = NewSerialConnReader(sa.framer, mock)

	sa.wg.Add(1)
	go sa.readLoop()

	return sa, mock
}

// --- 테스트 케이스: NewSerialAgent ---

func TestNewSerialAgent_ValidConfig(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)
	require.NotNil(t, a)

	assert.Equal(t, "test-serial", a.ID())
	assert.Equal(t, "Test Serial", a.Name())
	assert.Equal(t, "serial", a.Type())
}

func TestNewSerialAgent_InvalidConfig_MissingPort(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "test-serial",
		Name: "Test Serial",
		Type: "serial",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				// port 누락
				"baud_rate": 9600,
			},
		},
	}

	a, err := NewSerialAgent(cfg)
	assert.Error(t, err)
	assert.Nil(t, a)
	assert.Contains(t, err.Error(), "port")
}

func TestNewSerialAgent_InvalidConfig_InvalidFraming(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "test-serial",
		Name: "Test Serial",
		Type: "serial",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"port":    "/dev/ttyUSB0",
				"framing": "invalid_framing",
			},
		},
	}

	a, err := NewSerialAgent(cfg)
	assert.Error(t, err)
	assert.Nil(t, a)
}

// --- 테스트 케이스: 라이프사이클 ---

func TestSerialAgent_StartAndStop(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)
	mock := newBlockingMockSerialPort()

	// 목 오프너 설정
	sa.opener = func(_ string, _ *goserial.Mode) (goserial.Port, error) {
		// blockingMockSerialPort 는 goserial.Port 를 만족하지 않으므로
		// 직접 port 필드를 설정
		return nil, fmt.Errorf("mock: use direct injection")
	}

	// 직접 포트 주입으로 Start 시뮬레이션
	sa.mu.Lock()
	sa.port = mock
	sa.mu.Unlock()
	sa.connected.Store(true)
	sa.reader = NewSerialConnReader(sa.framer, mock)

	sa.wg.Add(1)
	go sa.readLoop()

	assert.True(t, sa.TransportConnected())

	ctx := context.Background()
	err = sa.Stop(ctx)
	require.NoError(t, err)

	assert.False(t, sa.TransportConnected())
	assert.Equal(t, lifecycle.StateStopped, sa.CurrentState())
}

func TestSerialAgent_PauseAndResume(t *testing.T) {
	sa, mock := createAgentWithPortOverride(t)
	defer func() {
		_ = sa.Stop(context.Background())
	}()
	_ = mock // 포트 참조 유지

	ctx := context.Background()

	// 일시 정지
	err := sa.Pause(ctx)
	require.NoError(t, err)
	assert.True(t, sa.paused.Load())
	assert.Equal(t, lifecycle.StatePaused, sa.CurrentState())

	// 재개
	err = sa.Resume(ctx)
	require.NoError(t, err)
	assert.False(t, sa.paused.Load())
	assert.Equal(t, lifecycle.StateRunning, sa.CurrentState())
}

// --- 테스트 케이스: Process ---

func TestSerialAgent_Process_SendsData(t *testing.T) {
	sa, mock := createAgentWithPortOverride(t)
	defer func() {
		_ = sa.Stop(context.Background())
	}()

	sendData := []byte("hello serial")
	_, err := sa.Process(sendData)
	require.NoError(t, err)

	mock.mu.Lock()
	written := make([]byte, mock.writeBuf.Len())
	copy(written, mock.writeBuf.Bytes())
	mock.mu.Unlock()

	assert.Equal(t, sendData, written)
}

func TestSerialAgent_Process_NotRunning(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)

	// 정지 상태로 전이
	_ = sa.TransitionTo(lifecycle.StateStopping)
	_ = sa.TransitionTo(lifecycle.StateStopped)

	_, err = sa.Process([]byte("data"))
	assert.ErrorIs(t, err, ErrNotRunning)
}

func TestSerialAgent_Process_TracksStats(t *testing.T) {
	sa, _ := createAgentWithPortOverride(t)
	defer func() {
		_ = sa.Stop(context.Background())
	}()

	data := []byte("test data")
	_, err := sa.Process(data)
	require.NoError(t, err)

	stats := sa.Stats()
	assert.Equal(t, int64(1), stats.MessagesSent)
	assert.Equal(t, int64(len(data)), stats.BytesWritten)
}

// --- 테스트 케이스: readLoop ---

func TestSerialAgent_ReadLoop_DeliversToMsgCh(t *testing.T) {
	sa, mock := createAgentWithPortOverride(t)
	defer func() {
		_ = sa.Stop(context.Background())
	}()

	testData := []byte("received data")
	mock.dataCh <- testData

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	data, err := sa.ReceiveMessage(ctx)
	require.NoError(t, err)
	assert.Equal(t, testData, data)

	// 통계 확인
	stats := sa.Stats()
	assert.Equal(t, int64(1), stats.MessagesReceived)
	assert.Equal(t, int64(len(testData)), stats.BytesRead)
}

func TestSerialAgent_ReadLoop_PausedDiscardsData(t *testing.T) {
	sa, mock := createAgentWithPortOverride(t)
	defer func() {
		_ = sa.Stop(context.Background())
	}()

	// 일시 정지
	sa.paused.Store(true)

	// 데이터 전송
	mock.dataCh <- []byte("discarded data")

	// 메시지 채널은 비어 있어야 한다
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 0, len(sa.msgCh))

	// 통계는 기록된다 (읽기는 수행됨)
	stats := sa.Stats()
	assert.Equal(t, int64(1), stats.MessagesReceived)
}

// --- 테스트 케이스: USB 분리 감지 ---

func TestIsDisconnectError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		expect bool
	}{
		{
			name:   "ENXIO (장치 없음)",
			err:    syscall.ENXIO,
			expect: true,
		},
		{
			name:   "EIO (I/O 에러)",
			err:    syscall.EIO,
			expect: true,
		},
		{
			name:   "ENOENT (파일 없음)",
			err:    syscall.ENOENT,
			expect: false,
		},
		{
			name:   "일반 에러",
			err:    fmt.Errorf("some error"),
			expect: false,
		},
		{
			name:   "래핑된 ENXIO",
			err:    fmt.Errorf("wrapped: %w", syscall.ENXIO),
			expect: true,
		},
		{
			name:   "래핑된 EIO",
			err:    fmt.Errorf("wrapped: %w", syscall.EIO),
			expect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDisconnectError(tt.err)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestSerialAgent_ReadLoop_DisconnectTransitionsToError(t *testing.T) {
	sa, mock := createAgentWithPortOverride(t)
	defer func() {
		// 이미 Error 상태이므로 Stop 으로 정리
		_ = sa.Stop(context.Background())
	}()

	// ENXIO 에러 전송으로 USB 분리 시뮬레이션
	mock.errCh <- syscall.ENXIO

	// Error 상태로 전이될 때까지 대기
	assert.Eventually(t, func() bool {
		return sa.CurrentState() == lifecycle.StateError
	}, 2*time.Second, 10*time.Millisecond, "USB 분리 시 Error 상태로 전이되어야 한다")

	assert.False(t, sa.TransportConnected())
}

// --- 테스트 케이스: Health ---

func TestSerialAgent_Health(t *testing.T) {
	tests := []struct {
		name      string
		state     lifecycle.State
		connected bool
		paused    bool
		expected  agent.HealthState
	}{
		{
			name:      "Running 이고 연결됨",
			state:     lifecycle.StateRunning,
			connected: true,
			expected:  agent.HealthHealthy,
		},
		{
			name:      "Running 이지만 연결 안됨",
			state:     lifecycle.StateRunning,
			connected: false,
			expected:  agent.HealthDegraded,
		},
		{
			name:     "Paused",
			state:    lifecycle.StatePaused,
			expected: agent.HealthDegraded,
		},
		{
			name:     "Stopped",
			state:    lifecycle.StateStopped,
			expected: agent.HealthUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := makeSerialConfig()
			a, err := NewSerialAgent(cfg)
			require.NoError(t, err)

			sa := a.(*SerialAgent)
			sa.connected.Store(tt.connected)

			// 상태 전이 (테스트를 위해 직접 상태 설정이 필요한 경우 우회)
			switch tt.state {
			case lifecycle.StatePaused:
				_ = sa.TransitionTo(lifecycle.StatePaused)
			case lifecycle.StateStopped:
				_ = sa.TransitionTo(lifecycle.StateStopping)
				_ = sa.TransitionTo(lifecycle.StateStopped)
			}
			// Running 은 이미 기본 상태

			health := sa.Health()
			assert.Equal(t, tt.expected, health.Status)
		})
	}
}

// --- 테스트 케이스: Info ---

func TestSerialAgent_Info(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	info := a.Info()
	assert.Equal(t, "test-serial", info.ID)
	assert.Equal(t, "Test Serial", info.Name)
	assert.Equal(t, "serial", info.Type)
	assert.Equal(t, lifecycle.StateRunning, info.State)
	assert.False(t, info.CreatedAt.IsZero())
}

// --- 테스트 케이스: TransportConnected ---

func TestSerialAgent_TransportConnected(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)

	// 초기 상태: 연결 안됨
	assert.False(t, sa.TransportConnected())

	// 연결 상태 변경
	sa.connected.Store(true)
	assert.True(t, sa.TransportConnected())

	sa.connected.Store(false)
	assert.False(t, sa.TransportConnected())
}

// --- 테스트 케이스: BufferInfo ---

func TestSerialAgent_BufferInfo(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)

	pending, capacity := sa.BufferInfo()
	assert.Equal(t, 0, pending)
	assert.Equal(t, 256, capacity)

	// 메시지 추가
	sa.msgCh <- []byte("test")
	pending, capacity = sa.BufferInfo()
	assert.Equal(t, 1, pending)
	assert.Equal(t, 256, capacity)
}

// --- 테스트 케이스: ReceiveMessage ---

func TestSerialAgent_ReceiveMessage_BlocksAndReturns(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)

	expected := []byte("hello")

	go func() {
		time.Sleep(50 * time.Millisecond)
		sa.msgCh <- expected
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	data, err := sa.ReceiveMessage(ctx)
	require.NoError(t, err)
	assert.Equal(t, expected, data)
}

func TestSerialAgent_ReceiveMessage_ContextCanceled(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 즉시 취소

	_, err = sa.ReceiveMessage(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

// --- 테스트 케이스: State ---

func TestSerialAgent_State(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)
	sa.connected.Store(true)
	sa.paused.Store(true)

	state := sa.State()
	assert.Equal(t, true, state["connected"])
	assert.Equal(t, "/dev/ttyUSB0", state["port"])
	assert.Equal(t, 9600, state["baud_rate"])
	assert.Equal(t, true, state["paused"])
}

// --- 테스트 케이스: Configure ---

func TestSerialAgent_Configure_ValidConfig(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	newCfg := makeSerialConfig()
	newCfg.Name = "Updated Serial"
	err = a.Configure(newCfg)
	require.NoError(t, err)

	assert.Equal(t, "Updated Serial", a.Name())
}

func TestSerialAgent_Configure_InvalidConfig(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	invalidCfg := agent.AgentConfig{} // 유효하지 않은 설정
	err = a.Configure(invalidCfg)
	assert.Error(t, err)
}

// --- 테스트 케이스: Stats ---

func TestSerialAgent_Stats_Initial(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	stats := a.Stats()
	assert.Equal(t, int64(0), stats.MessagesReceived)
	assert.Equal(t, int64(0), stats.MessagesSent)
	assert.Equal(t, int64(0), stats.MessagesErrored)
	assert.Equal(t, int64(0), stats.BytesRead)
	assert.Equal(t, int64(0), stats.BytesWritten)
}

// --- 테스트 케이스: 동시성 안전 ---

func TestSerialAgent_ConcurrentProcess(t *testing.T) {
	sa, _ := createAgentWithPortOverride(t)
	defer func() {
		_ = sa.Stop(context.Background())
	}()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			data := []byte(fmt.Sprintf("msg-%d", n))
			_, _ = sa.Process(data)
		}(i)
	}

	wg.Wait()

	stats := sa.Stats()
	assert.Equal(t, int64(10), stats.MessagesSent)
}

// --- 테스트 케이스: parityFromString ---

func TestParityFromString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected goserial.Parity
	}{
		{name: "even", input: "even", expected: goserial.EvenParity},
		{name: "odd", input: "odd", expected: goserial.OddParity},
		{name: "mark", input: "mark", expected: goserial.MarkParity},
		{name: "space", input: "space", expected: goserial.SpaceParity},
		{name: "none", input: "none", expected: goserial.NoParity},
		{name: "empty string defaults to NoParity", input: "", expected: goserial.NoParity},
		{name: "unknown defaults to NoParity", input: "invalid", expected: goserial.NoParity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parityFromString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- 테스트 케이스: stopBitsFromInt ---

func TestStopBitsFromInt(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected goserial.StopBits
	}{
		{name: "1 stop bit", input: 1, expected: goserial.OneStopBit},
		{name: "2 stop bits", input: 2, expected: goserial.TwoStopBits},
		{name: "0 defaults to OneStopBit", input: 0, expected: goserial.OneStopBit},
		{name: "negative defaults to OneStopBit", input: -1, expected: goserial.OneStopBit},
		{name: "3 defaults to OneStopBit", input: 3, expected: goserial.OneStopBit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stopBitsFromInt(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- 테스트 케이스: isTimeoutError ---

// timeoutError 는 Timeout() bool 인터페이스를 구현하는 에러이다.
type timeoutError struct {
	timeout bool
}

func (e *timeoutError) Error() string {
	return "timeout error"
}

func (e *timeoutError) Timeout() bool {
	return e.timeout
}

func TestIsTimeoutError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "context.DeadlineExceeded is timeout",
			err:      context.DeadlineExceeded,
			expected: true,
		},
		{
			name:     "wrapped DeadlineExceeded is timeout",
			err:      fmt.Errorf("wrapped: %w", context.DeadlineExceeded),
			expected: true,
		},
		{
			name:     "regular error is not timeout",
			err:      fmt.Errorf("some error"),
			expected: false,
		},
		{
			name:     "io.EOF is not timeout",
			err:      io.EOF,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTimeoutError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- 테스트 케이스: Init (public method) ---

func TestSerialAgent_Init(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)

	// Init on already-running agent will fail (invalid state transition)
	err = sa.Init(cfg)
	assert.Error(t, err, "Init on already-running agent should fail due to invalid transition")
}

func TestSerialAgent_Init_InvalidConfig(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)

	// Init with empty config should fail validation
	err = sa.Init(agent.AgentConfig{})
	assert.Error(t, err)
}

// --- 테스트 케이스: Start ---

func TestSerialAgent_Start_Success(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)
	mock := newMockSerialPort()

	// Set up a mock opener that returns the mock port (as goserial.Port)
	// Since mockSerialPort doesn't implement goserial.Port fully,
	// we use a custom opener that directly sets the port field.
	mockPort := newBlockingMockSerialPort()
	sa.opener = func(_ string, _ *goserial.Mode) (goserial.Port, error) {
		// We can't return blockingMockSerialPort as goserial.Port,
		// so we simulate Start manually
		return nil, fmt.Errorf("mock opener")
	}
	_ = mock
	_ = mockPort

	// 2026-05-14 hotfix: Stopped/Error 상태에서도 Start 가 lifecycle 우회 경로를 거쳐
	// Running 으로 전이한 뒤 opener 를 호출한다 (사용자 보고: 시리얼 분리 후 Error
	// 상태에서 Start API 호출이 영구 거부되던 회귀 해소).
	_ = sa.TransitionTo(lifecycle.StateStopping)
	_ = sa.TransitionTo(lifecycle.StateStopped)

	err = sa.Start(context.Background())
	assert.Error(t, err)
	// Stopped 에서 lifecycle 통과 후 opener 단계에서 실패하는지 확인
	assert.Contains(t, err.Error(), "open port")
}

func TestSerialAgent_Start_OpenerError(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)

	// Mock opener that returns an error
	sa.opener = func(_ string, _ *goserial.Mode) (goserial.Port, error) {
		return nil, fmt.Errorf("port open failed")
	}

	err = sa.Start(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "open port")
}

// --- 테스트 케이스: Process edge cases ---

func TestSerialAgent_Process_NotConnected(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)
	// Running state but not connected
	sa.connected.Store(false)

	_, err = sa.Process([]byte("data"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestSerialAgent_Process_NilPort(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)
	sa.connected.Store(true)
	// port is nil

	_, err = sa.Process([]byte("data"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "port is nil")
}

func TestSerialAgent_Process_WriteError(t *testing.T) {
	// 포트를 닫는 방식으로 write 실패를 유도하면 readLoop 가 먼저 EOF 를 감지해
	// state 를 StateError 로 전이시키고, 이어진 Process 호출은 ErrNotRunning
	// (race-condition: Linux CI 에서 재현됨) 을 반환하게 된다.
	// 대신 mock 의 `failWrites` 로 Write 만 실패시켜 readLoop / state 에 영향이
	// 없도록 한다.
	sa, mock := createAgentWithPortOverride(t)
	defer func() {
		_ = sa.Stop(context.Background())
	}()

	mock.failWrites(io.ErrClosedPipe)

	_, err := sa.Process([]byte("data"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "write failed")

	stats := sa.Stats()
	assert.Equal(t, int64(1), stats.MessagesErrored)
}

// --- 테스트 케이스: readLoop edge cases ---

func TestSerialAgent_ReadLoop_TimeoutContinues(t *testing.T) {
	sa, mock := createAgentWithPortOverride(t)
	defer func() {
		_ = sa.Stop(context.Background())
	}()

	// Send a timeout error, then real data
	mock.errCh <- context.DeadlineExceeded

	// After timeout, readLoop should continue and process next data
	testData := []byte("after timeout")
	mock.dataCh <- testData

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	data, err := sa.ReceiveMessage(ctx)
	require.NoError(t, err)
	assert.Equal(t, testData, data)
}

func TestSerialAgent_ReadLoop_EmptyDataIgnored(t *testing.T) {
	sa, mock := createAgentWithPortOverride(t)
	defer func() {
		_ = sa.Stop(context.Background())
	}()

	// Send empty data followed by real data
	mock.dataCh <- []byte{}
	mock.dataCh <- []byte("real data")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	data, err := sa.ReceiveMessage(ctx)
	require.NoError(t, err)
	assert.Equal(t, []byte("real data"), data)
}

func TestSerialAgent_ReadLoop_BufferFullDropsMessage(t *testing.T) {
	sa, mock := createAgentWithPortOverride(t)
	defer func() {
		_ = sa.Stop(context.Background())
	}()

	// Fill the message buffer to capacity
	for i := 0; i < cap(sa.msgCh); i++ {
		sa.msgCh <- []byte("fill")
	}

	// Send one more message - should be dropped (buffer full)
	mock.dataCh <- []byte("overflow")

	// Give readLoop time to process the overflow message
	time.Sleep(100 * time.Millisecond)

	// Stats should record the received message even if dropped
	stats := sa.Stats()
	assert.True(t, stats.MessagesReceived >= 1)
}

func TestSerialAgent_ReadLoop_NonTimeoutNonDisconnectError(t *testing.T) {
	sa, mock := createAgentWithPortOverride(t)
	defer func() {
		_ = sa.Stop(context.Background())
	}()

	// Send a non-timeout, non-disconnect error (e.g., io.EOF)
	mock.errCh <- io.ErrUnexpectedEOF

	// Should transition to error state
	assert.Eventually(t, func() bool {
		return sa.CurrentState() == lifecycle.StateError
	}, 2*time.Second, 10*time.Millisecond)

	assert.False(t, sa.TransportConnected())
}

// --- 테스트 케이스: ReceiveMessage stopCh ---

func TestSerialAgent_ReceiveMessage_Stopped(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)

	// Close stopCh to simulate stop
	close(sa.stopCh)

	_, err = sa.ReceiveMessage(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stopped")
}
