// 시리얼 에이전트의 재시작 (Start-after-Stop) 및 강제 종료 (USB 분리 등으로 인한
// 블로킹 Read 발생 시) 동작을 검증하는 테스트.
//
// 회귀 방지 시나리오:
//   1. RestartCycle_ReaderActive — 동일 인스턴스에서 Start → Stop → Start 사이클
//      후 readLoop 가 새 stopCh 를 사용하여 정상적으로 데이터를 처리하는가
//   2. RestartCycle_StopIdempotent — 동일 인스턴스에서 두 번째 Stop 호출이
//      두 번째 readLoop 의 종료를 정상적으로 처리하는가 (stopOnce 소진 회귀 방지)
//   3. Stop_BoundedWhenReadBlocks — Read 가 영구히 블로킹되는 경우에도 Stop 이
//      유한 시간 내에 반환되는가 (USB 분리 시 hang 회귀 방지)

package serial

import (
	"bytes"
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goserial "go.bug.st/serial"

	"github.com/xtra/xflow/pkg/lifecycle"
)

// --- restartableMockSerialPort: Start/Stop 재시작 시나리오용 목 포트 ---
//
// goserial.Port 인터페이스를 만족하므로 sa.opener 로 주입 가능하다.
type restartableMockSerialPort struct {
	dataCh   chan []byte
	errCh    chan error
	writeBuf *bytes.Buffer
	closed   atomic.Bool
	mu       sync.Mutex
}

func newRestartableMockSerialPort() *restartableMockSerialPort {
	return &restartableMockSerialPort{
		dataCh:   make(chan []byte, 10),
		errCh:    make(chan error, 1),
		writeBuf: bytes.NewBuffer(nil),
	}
}

func (m *restartableMockSerialPort) Read(p []byte) (int, error) {
	if m.closed.Load() {
		return 0, io.EOF
	}
	select {
	case data := <-m.dataCh:
		n := copy(p, data)
		return n, nil
	case err := <-m.errCh:
		return 0, err
	}
}

func (m *restartableMockSerialPort) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	return m.writeBuf.Write(p)
}

func (m *restartableMockSerialPort) Close() error {
	if m.closed.Swap(true) {
		return nil
	}
	// 블로킹된 Read 를 풀어주기 위해 에러 전송 (정상 driver 동작 시뮬레이션).
	select {
	case m.errCh <- io.EOF:
	default:
	}
	return nil
}

func (m *restartableMockSerialPort) SetReadTimeout(_ time.Duration) error { return nil }

// goserial.Port 의 나머지 메서드 — 테스트에서는 사용되지 않는다.
func (m *restartableMockSerialPort) SetMode(_ *goserial.Mode) error { return nil }
func (m *restartableMockSerialPort) ResetInputBuffer() error        { return nil }
func (m *restartableMockSerialPort) ResetOutputBuffer() error       { return nil }
func (m *restartableMockSerialPort) SetDTR(_ bool) error            { return nil }
func (m *restartableMockSerialPort) SetRTS(_ bool) error            { return nil }
func (m *restartableMockSerialPort) GetModemStatusBits() (*goserial.ModemStatusBits, error) {
	return &goserial.ModemStatusBits{}, nil
}
func (m *restartableMockSerialPort) Drain() error                { return nil }
func (m *restartableMockSerialPort) Break(_ time.Duration) error { return nil }

// --- hangingMockSerialPort: USB 분리 hang 시뮬레이션 ---
//
// Close() 가 호출되어도 진행 중인 Read() 가 풀려나지 않는 상황을 재현한다.
// 일부 USB 시리얼 드라이버에서 장치를 물리적으로 제거하면 read syscall 이
// 즉시 EIO 를 반환하지 못하고 hang 되는 현상을 흉내낸다.
type hangingMockSerialPort struct {
	closed      atomic.Bool
	readEntered chan struct{} // 첫 Read 진입 시그널 — 테스트 동기화용
	readOnce    sync.Once
}

func newHangingMockSerialPort() *hangingMockSerialPort {
	return &hangingMockSerialPort{
		readEntered: make(chan struct{}),
	}
}

func (m *hangingMockSerialPort) Read(_ []byte) (int, error) {
	m.readOnce.Do(func() { close(m.readEntered) })
	// Close 가 호출되어도 풀려나지 않고 영구히 블로킹.
	select {} // 영원히 블로킹
}

func (m *hangingMockSerialPort) Write(p []byte) (int, error) {
	if m.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	return len(p), nil
}

func (m *hangingMockSerialPort) Close() error {
	m.closed.Store(true)
	// 의도적으로 Read 를 풀어주지 않는다.
	return nil
}

func (m *hangingMockSerialPort) SetReadTimeout(_ time.Duration) error { return nil }
func (m *hangingMockSerialPort) SetMode(_ *goserial.Mode) error       { return nil }
func (m *hangingMockSerialPort) ResetInputBuffer() error              { return nil }
func (m *hangingMockSerialPort) ResetOutputBuffer() error             { return nil }
func (m *hangingMockSerialPort) SetDTR(_ bool) error                  { return nil }
func (m *hangingMockSerialPort) SetRTS(_ bool) error                  { return nil }
func (m *hangingMockSerialPort) GetModemStatusBits() (*goserial.ModemStatusBits, error) {
	return &goserial.ModemStatusBits{}, nil
}
func (m *hangingMockSerialPort) Drain() error                { return nil }
func (m *hangingMockSerialPort) Break(_ time.Duration) error { return nil }

// --- 회귀 테스트: Start → Stop → Start 사이클에서 readLoop 가 살아있는가 ---
func TestSerialAgent_RestartCycle_ReaderActive(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)

	// 매 Start 마다 새 목을 발급한다 (이전 목은 Close 됨).
	var current atomic.Pointer[restartableMockSerialPort]
	current.Store(newRestartableMockSerialPort())
	sa.opener = func(_ string, _ *goserial.Mode) (goserial.Port, error) {
		m := current.Load()
		if m.closed.Load() {
			m = newRestartableMockSerialPort()
			current.Store(m)
		}
		return m, nil
	}

	ctx := context.Background()

	// 1차 Start
	require.NoError(t, sa.Start(ctx))
	assert.True(t, sa.TransportConnected())

	// 1차 라운드: 데이터 송수신 확인
	current.Load().dataCh <- []byte("first round")
	rctx, rcancel := context.WithTimeout(ctx, 2*time.Second)
	data, rerr := sa.ReceiveMessage(rctx)
	rcancel()
	require.NoError(t, rerr)
	assert.Equal(t, []byte("first round"), data)

	// Stop → 동일 인스턴스 재기동 준비
	require.NoError(t, sa.Stop(ctx))
	require.NoError(t, sa.TransitionTo(lifecycle.StateCreated))
	require.NoError(t, sa.Init(cfg))

	// 2차 Start — 핵심 회귀 검증 지점.
	// 결함이 남아있다면 새 readLoop 가 이미 닫힌 stopCh 를 만나 즉시 종료하여
	// 두 번째 라운드 데이터가 수신되지 않는다.
	require.NoError(t, sa.Start(ctx))
	assert.True(t, sa.TransportConnected())

	// 2차 라운드: 새 readLoop 가 활성 상태임을 확인 (회귀 검증 핵심)
	current.Load().dataCh <- []byte("second round")
	rctx2, rcancel2 := context.WithTimeout(ctx, 2*time.Second)
	defer rcancel2()
	data2, rerr2 := sa.ReceiveMessage(rctx2)
	require.NoError(t, rerr2,
		"두 번째 Start 후 readLoop 가 활성 상태여야 한다 — 회귀: stopCh 미리셋으로 즉시 종료")
	assert.Equal(t, []byte("second round"), data2)

	require.NoError(t, sa.Stop(ctx))
}

// --- 회귀 테스트: 두 번째 Stop 이 두 번째 readLoop 의 종료를 신호하는가 ---
//
// stopOnce 가 첫 Stop 시 소진된 채로 재시작하면, 두 번째 Stop 의
// close(stopCh) 가 no-op 이 되어 readLoop 가 종료 시그널을 받지 못한다.
// 결과적으로 wg.Wait 가 블로킹되어 Stop 이 무한 대기한다.
func TestSerialAgent_RestartCycle_StopIdempotent(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)

	var current atomic.Pointer[restartableMockSerialPort]
	current.Store(newRestartableMockSerialPort())
	sa.opener = func(_ string, _ *goserial.Mode) (goserial.Port, error) {
		m := current.Load()
		if m.closed.Load() {
			m = newRestartableMockSerialPort()
			current.Store(m)
		}
		return m, nil
	}

	ctx := context.Background()

	// 1차 Start → Stop
	require.NoError(t, sa.Start(ctx))
	require.NoError(t, sa.Stop(ctx))

	// 동일 인스턴스 재기동
	require.NoError(t, sa.TransitionTo(lifecycle.StateCreated))
	require.NoError(t, sa.Init(cfg))
	require.NoError(t, sa.Start(ctx))

	// 2차 Stop — 유한 시간 내 반환되어야 한다.
	stopDone := make(chan error, 1)
	go func() {
		stopDone <- sa.Stop(ctx)
	}()

	select {
	case serr := <-stopDone:
		require.NoError(t, serr)
	case <-time.After(8 * time.Second):
		t.Fatal("두 번째 Stop 이 8초 내에 반환되지 못함 — 회귀: stopOnce 미리셋으로 close 미발동")
	}
}

// --- 회귀 테스트: Read 가 영구 블로킹되어도 Stop 이 유한 시간 내 반환되는가 ---
//
// 기존 결함:
//   - port.Close() 가 진행 중인 read syscall 을 풀어주지 못하는 경우 (USB hang)
//   - wg.Wait() 가 무한 대기
//
// 수정:
//   - wg.Wait 를 5초 타임아웃으로 감싼다 (Fix B).
func TestSerialAgent_Stop_BoundedWhenReadBlocks(t *testing.T) {
	cfg := makeSerialConfig()
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)
	hangMock := newHangingMockSerialPort()

	sa.opener = func(_ string, _ *goserial.Mode) (goserial.Port, error) {
		return hangMock, nil
	}

	ctx := context.Background()
	require.NoError(t, sa.Start(ctx))

	// readLoop 가 실제로 Read 안에서 블로킹될 때까지 대기 — Stop 시점 동기화.
	select {
	case <-hangMock.readEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("readLoop 가 Read 에 진입하지 못함")
	}

	// Stop — Read 가 영원히 블로킹되더라도 유한 시간 내에 반환되어야 한다.
	stopDone := make(chan error, 1)
	start := time.Now()
	go func() {
		stopDone <- sa.Stop(ctx)
	}()

	select {
	case serr := <-stopDone:
		elapsed := time.Since(start)
		require.NoError(t, serr)
		// Fix B 의 5초 타임아웃 + 약간의 여유.
		assert.Less(t, elapsed, 7*time.Second,
			"Stop 이 hang 상황에서 유한 시간 (~5초) 내에 반환되어야 한다 — 회귀: 무한 wg.Wait")
		t.Logf("Stop 반환 시간: %s (5초 bounded wait + cleanup)", elapsed)
	case <-time.After(10 * time.Second):
		t.Fatal("Stop 이 10초 내에 반환되지 못함 — 회귀: USB hang 시 무한 블로킹")
	}
}
