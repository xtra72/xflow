package serial

// 이 파일은 RS-485 half-duplex 시리얼 포트의 동시 I/O 결함에 대한
// 회귀 테스트를 담는다.
//
// 결함: SerialAgent.Process(Write)와 readLoop(Read)가 동일 물리 포트에
// 동시 접근하면, half-duplex 버스에서 송수신이 물리적으로 겹쳐 프레임이
// 깨진다 (0x55 preamble → 0x00 등). mu 는 a.port 포인터 읽기만 보호하고
// 실제 I/O 는 보호하지 않았다.
//
// 수정: portIOMu 로 모든 물리 포트 I/O (Write/Read) 를 직렬화한다.
//
// 모든 동시성 테스트는 `go test -race` 하에서 실행되어야 한다.

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goserial "go.bug.st/serial"

	"github.com/xtra/xflow/internal/agent"
)

// --- overlapDetectingPort: Read/Write 겹침을 감지하는 목 포트 ---

// overlapDetectingPort 는 Read 와 Write 가 물리적으로 겹쳐 호출되는지
// 감지하는 목 시리얼 포트이다.
//
// inIO 카운터는 I/O 진입 시 증가, 종료 시 감소한다. half-duplex 포트에서는
// 어느 순간에도 동시에 진행 중인 I/O 가 1 개를 초과할 수 없다. 1 을 초과하면
// overlap 플래그를 set 한다.
type overlapDetectingPort struct {
	inIO        atomic.Int32 // 현재 진행 중인 I/O 작업 수 (Read + Write)
	overlap     atomic.Bool  // Read/Write 겹침이 한 번이라도 감지되면 true
	maxObserved atomic.Int32 // 관측된 최대 동시 I/O 수 (디버깅용)

	readDelay  time.Duration // Read 임계 구역 내부에서 머무는 시간
	writeDelay time.Duration // Write 임계 구역 내부에서 머무는 시간

	closed atomic.Bool
}

func newOverlapDetectingPort(readDelay, writeDelay time.Duration) *overlapDetectingPort {
	return &overlapDetectingPort{
		readDelay:  readDelay,
		writeDelay: writeDelay,
	}
}

// enterIO 는 I/O 임계 구역 진입을 기록하고 겹침을 검사한다.
func (m *overlapDetectingPort) enterIO() {
	n := m.inIO.Add(1)
	if n > 1 {
		m.overlap.Store(true)
	}
	// maxObserved 갱신 (관측용, 정확성에 민감하지 않음)
	for {
		cur := m.maxObserved.Load()
		if n <= cur || m.maxObserved.CompareAndSwap(cur, n) {
			break
		}
	}
}

// exitIO 는 I/O 임계 구역 종료를 기록한다.
func (m *overlapDetectingPort) exitIO() {
	m.inIO.Add(-1)
}

func (m *overlapDetectingPort) Read(p []byte) (int, error) {
	if m.closed.Load() {
		return 0, io.EOF
	}
	m.enterIO()
	defer m.exitIO()
	if m.readDelay > 0 {
		time.Sleep(m.readDelay)
	}
	if m.closed.Load() {
		return 0, io.EOF
	}
	// 데이터가 도착하지 않은 것처럼 0 바이트 반환 (raw framer 는 그대로 반환).
	// SetReadTimeout 으로 bounded 된 실제 포트의 타임아웃 동작을 모사한다.
	return 0, nil
}

func (m *overlapDetectingPort) Write(p []byte) (int, error) {
	if m.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	m.enterIO()
	defer m.exitIO()
	if m.writeDelay > 0 {
		time.Sleep(m.writeDelay)
	}
	return len(p), nil
}

func (m *overlapDetectingPort) Close() error {
	m.closed.Store(true)
	return nil
}

func (m *overlapDetectingPort) SetReadTimeout(_ time.Duration) error {
	return nil
}

// --- closeUnblockPort: Close 호출 시까지 Read 가 블로킹되는 목 포트 ---

// closeUnblockPort 는 Read 가 Close 호출 시까지 블로킹되는 목 포트이다.
// Stop 이 portIOMu 가 아니라 port.Close() 로 mid-read 를 풀어주는지 검증한다.
type closeUnblockPort struct {
	unblock  chan struct{} // Close 시 닫힌다
	closed   atomic.Bool
	readsIn  atomic.Int32 // Read 에 진입한 횟수
	writeBuf []byte
	mu       sync.Mutex
}

func newCloseUnblockPort() *closeUnblockPort {
	return &closeUnblockPort{
		unblock: make(chan struct{}),
	}
}

func (m *closeUnblockPort) Read(p []byte) (int, error) {
	m.readsIn.Add(1)
	<-m.unblock // Close 가 호출될 때까지 블로킹
	return 0, io.EOF
}

func (m *closeUnblockPort) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	m.writeBuf = append(m.writeBuf, p...)
	return len(p), nil
}

func (m *closeUnblockPort) Close() error {
	if m.closed.CompareAndSwap(false, true) {
		close(m.unblock) // 블로킹된 Read 를 풀어준다
	}
	return nil
}

func (m *closeUnblockPort) SetReadTimeout(_ time.Duration) error {
	return nil
}

// --- timeoutCapturePort: SetReadTimeout 인자를 캡처하는 목 포트 ---
//
// goserial.Port 인터페이스 전체를 만족하므로 sa.opener 로 주입되어
// 실제 Start() 경로 (SetReadTimeout 호출 포함) 를 통과한다.
type timeoutCapturePort struct {
	capturedTimeout atomic.Int64 // SetReadTimeout 에 전달된 마지막 값 (ns)
	timeoutSet      atomic.Bool
	closed          atomic.Bool
}

func newTimeoutCapturePort() *timeoutCapturePort {
	return &timeoutCapturePort{}
}

func (m *timeoutCapturePort) Read(p []byte) (int, error) {
	if m.closed.Load() {
		return 0, io.EOF
	}
	// 데이터 없음 → 타임아웃처럼 즉시 반환 (bounded read 모사).
	return 0, nil
}

func (m *timeoutCapturePort) Write(p []byte) (int, error) {
	if m.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	return len(p), nil
}

func (m *timeoutCapturePort) Close() error {
	m.closed.Store(true)
	return nil
}

func (m *timeoutCapturePort) SetReadTimeout(t time.Duration) error {
	m.capturedTimeout.Store(int64(t))
	m.timeoutSet.Store(true)
	return nil
}

// goserial.Port 의 나머지 메서드 — 테스트에서는 사용되지 않는다.
func (m *timeoutCapturePort) SetMode(_ *goserial.Mode) error { return nil }
func (m *timeoutCapturePort) ResetInputBuffer() error        { return nil }
func (m *timeoutCapturePort) ResetOutputBuffer() error       { return nil }
func (m *timeoutCapturePort) SetDTR(_ bool) error            { return nil }
func (m *timeoutCapturePort) SetRTS(_ bool) error            { return nil }
func (m *timeoutCapturePort) GetModemStatusBits() (*goserial.ModemStatusBits, error) {
	return &goserial.ModemStatusBits{}, nil
}
func (m *timeoutCapturePort) Drain() error                { return nil }
func (m *timeoutCapturePort) Break(_ time.Duration) error { return nil }

// makeSerialConfigWithReadTimeout 는 read_timeout 을 지정한 테스트 설정을 만든다.
func makeSerialConfigWithReadTimeout(readTimeout string) agent.AgentConfig {
	cfg := makeSerialConfig()
	cfg.Transport.Options["read_timeout"] = readTimeout
	return cfg
}

// --- 테스트 1: 동시 Write/Read 시 물리 포트 I/O 가 겹치지 않아야 한다 ---

// TestSerialAgent_ConcurrentWriteRead_NoPortOverlap 는 Process(Write)와
// readLoop(Read)가 동시에 실행될 때 물리 포트 I/O 가 절대 겹치지 않음을 검증한다.
//
// 수정 전: framer.Write 와 reader.Read 가 mutex 없이 동시 실행 → overlap 감지 → FAIL.
// 수정 후: portIOMu 가 모든 포트 I/O 를 직렬화 → overlap 없음 → PASS.
//
// `go test -race` 하에서 실행되어야 한다.
func TestSerialAgent_ConcurrentWriteRead_NoPortOverlap(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)

	// 임계 구역 내부에 약간의 지연을 두어 겹침 발생 확률을 높인다.
	mock := newOverlapDetectingPort(200*time.Microsecond, 200*time.Microsecond)

	// Start 를 우회하고 직접 포트를 주입한다 (overlapDetectingPort 는
	// goserial.Port 를 만족하지 않으므로).
	// portIOReader 로 감싼다: 각 sub-read 마다 락을 획득/해제하여
	// 물리 포트 I/O 를 per-sub-read 로 직렬화한다.
	sa.mu.Lock()
	sa.port = mock
	sa.mu.Unlock()
	sa.connected.Store(true)
	var portReader io.Reader = mock
	portReader = &portIOReader{inner: portReader, lock: sa.portIOLock}
	sa.reader = NewSerialConnReader(sa.framer, portReader)

	sa.wg.Add(1)
	go sa.readLoop()

	defer func() {
		_ = sa.Stop(context.Background())
	}()

	// Process(Write)를 다수의 고루틴에서 동시에, 여러 번 호출한다.
	const writers = 4
	const iterations = 200
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_, _ = sa.Process([]byte(fmt.Sprintf("w%d-%d", id, i)))
			}
		}(w)
	}
	wg.Wait()

	// readLoop 가 계속 Read 를 수행하도록 잠시 더 대기한다.
	time.Sleep(20 * time.Millisecond)

	assert.False(t, mock.overlap.Load(),
		"물리 포트 I/O 겹침이 감지되었다 (관측된 최대 동시 I/O=%d). "+
			"half-duplex RS-485 에서 Write 와 Read 는 절대 겹치면 안 된다.",
		mock.maxObserved.Load())
	assert.LessOrEqual(t, mock.maxObserved.Load(), int32(1),
		"동시 진행 I/O 수는 1 을 초과할 수 없다")
}

// --- 테스트 2: Stop 은 portIOMu 가 아니라 port.Close() 로 mid-read 를 풀어야 한다 ---

// TestSerialAgent_Stop_UnblocksReadHoldingLock 는 readLoop 가 portIOMu 를
// 쥔 채 Read 에서 블로킹되어 있을 때 Stop 이 deadlock 없이 유한 시간 안에
// 반환됨을 검증한다.
//
// Stop 은 portIOMu 를 획득하려 해서는 안 된다 (mid-read 뒤에서 블로킹된다).
// port.Close() 가 Read 를 풀어주는 메커니즘이어야 한다.
//
// `go test -race` 하에서 실행되어야 한다.
func TestSerialAgent_Stop_UnblocksReadHoldingLock(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)
	mock := newCloseUnblockPort()

	sa.mu.Lock()
	sa.port = mock
	sa.mu.Unlock()
	sa.connected.Store(true)
	sa.reader = NewSerialConnReader(sa.framer, mock)

	sa.wg.Add(1)
	go sa.readLoop()

	// readLoop 가 Read 에 진입 (그리고 portIOMu 를 쥠) 할 때까지 대기.
	require.Eventually(t, func() bool {
		return mock.readsIn.Load() >= 1
	}, 2*time.Second, 5*time.Millisecond, "readLoop 가 Read 에 진입해야 한다")

	// Stop 은 5초 bounded wait 안에 반환되어야 한다 (deadlock 금지).
	done := make(chan error, 1)
	go func() {
		done <- sa.Stop(context.Background())
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(7 * time.Second):
		t.Fatal("Stop 이 반환되지 않았다 — portIOMu 에서 deadlock 발생 가능성")
	}
}

// --- 테스트 3: read_timeout 이 비양수여도 유효한 양수 타임아웃으로 보정되어야 한다 ---

// TestSerialAgent_ReadTimeout_BoundsLockHold 는 read_timeout 설정이
// 0 이거나 음수일 때 Start 가 SetReadTimeout 에 전달하는 값이 항상 유한
// 양수임을 검증한다.
//
// 비양수 타임아웃은 일부 드라이버에서 무한 블로킹을 의미하며, readLoop 가
// portIOMu 를 쥔 채 영원히 블로킹되어 Process(Write)가 starve 된다.
//
// `go test -race` 하에서 실행되어야 한다.
func TestSerialAgent_ReadTimeout_BoundsLockHold(t *testing.T) {
	tests := []struct {
		name        string
		readTimeout string
	}{
		{name: "read_timeout 0s", readTimeout: "0s"},
		{name: "read_timeout 음수", readTimeout: "-5ms"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := makeSerialConfigWithReadTimeout(tt.readTimeout)
			a, err := NewSerialAgent(cfg)
			require.NoError(t, err)

			sa := a.(*SerialAgent)
			mock := newTimeoutCapturePort()

			// timeoutCapturePort 는 goserial.Port 전체를 만족하므로
			// 실제 Start() 경로 (SetReadTimeout 호출 포함) 를 통과한다.
			sa.opener = func(_ string, _ *goserial.Mode) (goserial.Port, error) {
				return mock, nil
			}

			require.NoError(t, sa.Start(context.Background()))
			defer func() {
				_ = sa.Stop(context.Background())
			}()

			// SetReadTimeout 에 전달된 값은 항상 유한 양수여야 한다.
			require.True(t, mock.timeoutSet.Load(), "SetReadTimeout 이 호출되어야 한다")
			captured := time.Duration(mock.capturedTimeout.Load())
			assert.Greater(t, captured, time.Duration(0),
				"비양수 read_timeout (%s) 은 유한 양수 effective timeout 으로 "+
					"보정되어 SetReadTimeout 에 전달되어야 한다 (실제 전달값=%s). "+
					"비양수 값은 일부 드라이버에서 무한 블로킹 → readLoop 가 "+
					"portIOMu 를 쥔 채 영원히 멈춰 Process(Write)가 starve 된다.",
				tt.readTimeout, captured)
		})
	}
}

// --- 테스트: effectiveReadTimeout 분기 검증 ---

// TestEffectiveReadTimeout 는 effectiveReadTimeout 의 모든 분기를 검증한다.
// 이 함수는 Start() 의 인라인 로직을 순수 함수로 추출한 것이며 (behavior-preserving),
// 추가로 비양수 값을 minReadTimeout 으로 보정하는 로직이 더해졌다.
//
// 핵심 보존 속성: 양수 값은 (1ms 같은 작은 값 포함) 절대 변경되지 않는다.
func TestEffectiveReadTimeout(t *testing.T) {
	tests := []struct {
		name     string
		cfg      SerialConfig
		expected time.Duration
	}{
		{
			name:     "양수 read_timeout 은 그대로 보존",
			cfg:      SerialConfig{ReadTimeout: 100 * time.Millisecond},
			expected: 100 * time.Millisecond,
		},
		{
			name:     "작은 양수 read_timeout (1ms) 도 그대로 보존 — 기존 동작 보존",
			cfg:      SerialConfig{ReadTimeout: 1 * time.Millisecond},
			expected: 1 * time.Millisecond,
		},
		{
			name: "gap_timeout 이 양수이면 우선 사용",
			cfg: SerialConfig{
				ReadTimeout: 100 * time.Millisecond,
				GapTimeout:  20 * time.Millisecond,
			},
			expected: 20 * time.Millisecond,
		},
		{
			name: "stream 프레이밍은 idle_timeout 사용",
			cfg: SerialConfig{
				ReadTimeout: 100 * time.Millisecond,
				IdleTimeout: 5 * time.Millisecond,
				Framing:     FramingStream,
			},
			expected: 5 * time.Millisecond,
		},
		{
			name: "stream 프레이밍이라도 gap_timeout 이 우선",
			cfg: SerialConfig{
				IdleTimeout: 5 * time.Millisecond,
				GapTimeout:  30 * time.Millisecond,
				Framing:     FramingStream,
			},
			expected: 30 * time.Millisecond,
		},
		{
			name:     "read_timeout 0 은 minReadTimeout 으로 보정",
			cfg:      SerialConfig{ReadTimeout: 0},
			expected: minReadTimeout,
		},
		{
			name:     "read_timeout 음수는 minReadTimeout 으로 보정",
			cfg:      SerialConfig{ReadTimeout: -5 * time.Millisecond},
			expected: minReadTimeout,
		},
		{
			name: "stream 프레이밍 + idle_timeout 0 은 minReadTimeout 으로 보정",
			cfg: SerialConfig{
				IdleTimeout: 0,
				Framing:     FramingStream,
			},
			expected: minReadTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectiveReadTimeout(tt.cfg)
			assert.Equal(t, tt.expected, got)
			assert.Greater(t, got, time.Duration(0),
				"effective read timeout 은 항상 유한 양수여야 한다")
		})
	}
}
