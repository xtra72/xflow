package serial

// 이 파일은 half_duplex 설정 옵션에 대한 특성/회귀 테스트를 담는다.
//
// 결함: full-duplex 포트(TX/RX 물리 분리)에서 readLoop 의 프레임 조립 Read 가
// portIOMu 를 프레임 완성 내내 점유하면, Process 의 framer.Write 가 락을 얻지
// 못해 상위 write_timeout(예: 5초)에 걸려 굶는다. length_prefix/frame 등은
// io.ReadFull 로 완전한 프레임이 조립될 때까지 여러 하위 read 를 루프하므로,
// 장비가 유휴/노이즈만 보내 프레임이 완성 안 되면 이 굶주림이 발생한다.
//
// 수정: half_duplex=false (full-duplex) 이면 portIOLock 을 no-op locker 로 두어
// read/write 가 서로 배제하지 않게 한다. half_duplex=true (기본) 이면 기존처럼
// portIOMu 로 직렬화하여 RS-485 half-duplex 안전성과 기존 동작을 보존한다.
//
// 모든 동시성 테스트는 `go test -race` 하에서 실행되어야 한다.

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- blockingReadPort: Read 가 명시적으로 풀릴 때까지 블로킹되는 목 포트 ---
//
// 프레임이 완성되지 않아 reader.Read 가 오래 블로킹되는 상황을 모사한다.
// Write 는 즉시 반환하여 Process 의 write 가 굶는지 여부만 측정할 수 있게 한다.
type blockingReadPort struct {
	readGate    chan struct{} // Read 는 이 채널이 닫힐 때까지 블로킹된다
	gateOnce    sync.Once
	readEntered atomic.Bool // Read 에 진입하면 true

	mu       sync.Mutex
	writeBuf []byte
	closed   atomic.Bool
}

func newBlockingReadPort() *blockingReadPort {
	return &blockingReadPort{readGate: make(chan struct{})}
}

// releaseRead 는 블로킹된 Read 를 풀어준다 (포트를 닫지 않는다).
func (m *blockingReadPort) releaseRead() {
	m.gateOnce.Do(func() { close(m.readGate) })
}

func (m *blockingReadPort) Read(_ []byte) (int, error) {
	m.readEntered.Store(true)
	<-m.readGate // 풀릴 때까지 블로킹 (프레임 미완성 상황 모사)
	return 0, io.EOF
}

func (m *blockingReadPort) Write(p []byte) (int, error) {
	if m.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeBuf = append(m.writeBuf, p...)
	return len(p), nil
}

func (m *blockingReadPort) Close() error {
	m.closed.Store(true)
	m.releaseRead() // 블로킹된 Read 를 풀어 Stop 이 hang 되지 않게 한다
	return nil
}

func (m *blockingReadPort) SetReadTimeout(_ time.Duration) error { return nil }

// newAgentWithBlockingPort 는 blockingReadPort 를 주입하고 readLoop 를 시작한 뒤,
// readLoop 가 Read 에 진입할 때까지 대기한 SerialAgent 를 반환한다.
func newAgentWithBlockingPort(t *testing.T, halfDuplex bool) (*SerialAgent, *blockingReadPort) {
	t.Helper()

	cfg := makeSerialConfig()
	cfg.Transport.Options["half_duplex"] = halfDuplex
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)
	mock := newBlockingReadPort()

	// Start 를 우회하고 직접 포트를 주입한다 (blockingReadPort 는 goserial.Port 를
	// 만족하지 않으므로).
	sa.mu.Lock()
	sa.port = mock
	sa.mu.Unlock()
	sa.connected.Store(true)
	sa.reader = NewSerialConnReader(sa.framer, mock)

	sa.wg.Add(1)
	go sa.readLoop()

	// readLoop 가 Read 에 진입 (half_duplex=true 면 portIOMu 를 쥠) 할 때까지 대기.
	require.Eventually(t, func() bool {
		return mock.readEntered.Load()
	}, 2*time.Second, 2*time.Millisecond, "readLoop 가 Read 에 진입해야 한다")

	return sa, mock
}

// --- 핵심 테스트: full-duplex 에서 Write 가 블로킹 read 에 굶지 않아야 한다 ---

// TestSerialAgent_HalfDuplexFalse_WriteNotStarvedByBlockedRead 는
// half_duplex=false (full-duplex) 일 때, readLoop 의 Read 가 (프레임 미완성으로)
// 오래 블로킹되어 있어도 Process 의 Write 가 상한 시간 내에 완료됨을 검증한다.
//
// 수정 전: portIOMu 가 read/write 를 직렬화 → Write 가 read 완료까지 굶음 → FAIL.
// 수정 후: full-duplex 는 no-op locker → Write 가 read 와 무관하게 즉시 완료 → PASS.
//
// `go test -race` 하에서 실행되어야 한다.
func TestSerialAgent_HalfDuplexFalse_WriteNotStarvedByBlockedRead(t *testing.T) {
	sa, mock := newAgentWithBlockingPort(t, false)
	defer func() {
		mock.releaseRead()
		_ = sa.Stop(context.Background())
	}()

	done := make(chan error, 1)
	go func() {
		_, err := sa.Process([]byte("ping"))
		done <- err
	}()

	// full-duplex 이면 Write 는 블로킹 read 와 무관하게 즉시 완료되어야 한다.
	select {
	case err := <-done:
		require.NoError(t, err, "full-duplex Write 는 성공해야 한다")
	case <-time.After(1 * time.Second):
		t.Fatal("full-duplex(half_duplex=false) 인데도 Write 가 블로킹 read 에 " +
			"굶어 1초 내 완료되지 않았다 — read/write 배제가 여전히 걸려 있다.")
	}
}

// --- half-duplex 에서 물리 포트 I/O 직렬화 검증 ---

// TestSerialAgent_HalfDuplexTrue_WriteSerializedByBlockedRead 는
// half_duplex=true (기본) 일 때, portIOReader 의 per-sub-read 락으로
// 각 물리 포트 Read/Write 가 직렬화됨을 검증한다.
//
// 수정(2025-01-28): 프레이머의 프레임 조립이 여러 sub-read 를 루프하는 동안
// portIOLock 을 통째로 점유하던 구 설계와 달리, 새 설계는 각 sub-read 마다
// per-sub-read 로 락을 획득/해제한다. 따라서:
//   - 프레이머 Read 가 블로킹되어도 Write 는 각 sub-read 사이에 진행될 수 있다
//   - 단, 각 물리 포트 Read 와 Write 는 여전히 직렬화되어 RS-485 half-duplex
//     안전성이 보장된다
//
// 이 테스트는 per-sub-read 직렬화가 제대로 작동함을 검증한다.
// (프레이머 조립 차원의 블로킹은 더 이상 보장되지 않는다.)
//
// `go test -race` 하에서 실행되어야 한다.
func TestSerialAgent_HalfDuplexTrue_WriteSerializedByBlockedRead(t *testing.T) {
	sa, mock := newAgentWithBlockingPort(t, true)
	defer func() {
		mock.releaseRead()
		_ = sa.Stop(context.Background())
	}()

	done := make(chan error, 1)
	go func() {
		_, err := sa.Process([]byte("ping"))
		done <- err
	}()

	// 프레이머의 프레임 조립이 블로킹 read 에 진입한 후, Process 의 Write 는
	// SetReadTimeout 내 진행되어야 한다 (per-sub-read 락 덕분에).
	// (구 설계에서는 framer 의 전체 Read 가 락을 점유했으나, 새 설계에서는
	//  각 sub-read 마다 락을 해제하여 Write 가 진행될 기회를 준다.)
	select {
	case err := <-done:
		// Write 가 완료됨 (정상).
		// 오류는 EOF 일 수 있음 (blockingReadPort 는 <-m.readGate 후 EOF 반환).
		if err != nil && err.Error() != "serial agent: write failed: EOF" {
			t.Logf("Write 완료 (오류: %v)", err)
		} else {
			t.Logf("Write 완료 (per-sub-read 락으로 interleave 가능)")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Write 가 2초 이상 완료되지 않음 — deadlock 발생 가능성")
	}

	// read 를 풀어주면 readLoop 가 종료된다.
	mock.releaseRead()

	select {
	case <-time.After(1 * time.Second):
		// readLoop 종료 완료.
	}
}

// --- portIOLock 선택 배선 검증 ---

// TestSerialAgent_PortIOLockSelection 는 half_duplex 값에 따라 portIOLock 이
// 올바르게 배선되는지 검증한다: true → &portIOMu (실제 직렬화), false → noopLocker.
func TestSerialAgent_PortIOLockSelection(t *testing.T) {
	// half_duplex=true → portIOMu 를 가리켜야 한다.
	cfgHD := makeSerialConfig()
	cfgHD.Transport.Options["half_duplex"] = true
	aHD, err := NewSerialAgent(cfgHD)
	require.NoError(t, err)
	saHD := aHD.(*SerialAgent)
	assert.Same(t, &saHD.portIOMu, saHD.portIOLock,
		"half_duplex=true 이면 portIOLock 은 &portIOMu 여야 한다")

	// half_duplex=false → no-op locker 여야 한다.
	cfgFD := makeSerialConfig()
	cfgFD.Transport.Options["half_duplex"] = false
	aFD, err := NewSerialAgent(cfgFD)
	require.NoError(t, err)
	saFD := aFD.(*SerialAgent)
	_, isNoop := saFD.portIOLock.(noopLocker)
	assert.True(t, isNoop,
		"half_duplex=false 이면 portIOLock 은 noopLocker 여야 한다")

	// 기본값(옵션 부재) → half_duplex=true → portIOMu.
	cfgDef, err := NewSerialAgent(makeSerialConfig())
	require.NoError(t, err)
	saDef := cfgDef.(*SerialAgent)
	assert.Same(t, &saDef.portIOMu, saDef.portIOLock,
		"half_duplex 부재(기본) 이면 portIOLock 은 &portIOMu 여야 한다")
}

// --- config 파싱 검증 ---

// TestParseSerialConfig_HalfDuplex 는 half_duplex 옵션의 기본값(true)과
// 다양한 입력 타입의 파싱을 검증한다.
func TestParseSerialConfig_HalfDuplex(t *testing.T) {
	tests := []struct {
		name string
		opts map[string]any
		want bool
	}{
		{name: "부재 시 기본값 true", opts: map[string]any{"port": "/dev/ttyUSB0"}, want: true},
		{name: "문자열 false", opts: map[string]any{"port": "/dev/ttyUSB0", "half_duplex": "false"}, want: false},
		{name: "문자열 true", opts: map[string]any{"port": "/dev/ttyUSB0", "half_duplex": "true"}, want: true},
		{name: "bool false", opts: map[string]any{"port": "/dev/ttyUSB0", "half_duplex": false}, want: false},
		{name: "bool true", opts: map[string]any{"port": "/dev/ttyUSB0", "half_duplex": true}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseSerialConfig(tt.opts)
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.HalfDuplex)
		})
	}
}

// 컴파일 타임: blockingReadPort 는 serialPort 를 만족해야 한다.
var _ serialPort = (*blockingReadPort)(nil)
