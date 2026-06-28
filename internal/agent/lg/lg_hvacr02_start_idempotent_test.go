// lg_hvacr02_start_idempotent_test.go — Start 중복 호출 시 백그라운드 루프
// (notifyLoop/captureLoop/offlineWatchLoop) 가 누적되지 않음을 검증.
//
// 증상: 한 디바이스의 report 가 한 틱에 N건 중복 저장됨. 원인: Start 의 fast-path
// 가드가 transport.Available() 에 의존하여, 재연결 윈도우(transport unavailable)에
// Start 가 다시 호출되면 가드를 빠져나가 notifyLoop 등 고루틴이 누적되었다.
// fix: bgStarted CAS 로 루프 기동을 정확히 1회로 제한.
package lg

import (
	"context"
	"io"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/pkg/lifecycle"
)

// blockingMockTransport 는 Receive 가 Close 까지 블록되어 captureLoop 가 EOF→reconnect
// 로 빠지지 않게 한다(openCount 오염 방지). Available 은 항상 false 를 반환하여 Start 의
// fast-path 가드(Running && Available)를 우회시켜 bgStarted 가드 경로를 시험한다.
type blockingMockTransport struct {
	openCount atomic.Int32
	release   chan struct{}
	closed    atomic.Bool
}

func newBlockingMockTransport() *blockingMockTransport {
	return &blockingMockTransport{release: make(chan struct{})}
}

func (m *blockingMockTransport) Open() error {
	m.openCount.Add(1)
	return nil
}

func (m *blockingMockTransport) Close() error {
	if m.closed.CompareAndSwap(false, true) {
		close(m.release)
	}
	return nil
}

func (m *blockingMockTransport) Send(_ []byte) error { return nil }

func (m *blockingMockTransport) Receive(_ []byte) (int, error) {
	<-m.release
	return 0, io.EOF
}

func (m *blockingMockTransport) Available() bool { return false }

func (m *blockingMockTransport) Write(d []byte) (int, error) { return len(d), nil }

func TestStart_Idempotent_NoDuplicateBackgroundLoops(t *testing.T) {
	mock := newBlockingMockTransport()
	a := newTestHvacr02Agent(t, mock)
	a.BaseLifecycle = lifecycle.NewBaseLifecycle(lifecycle.WithName("test-lg_hvacr02"))
	require.NoError(t, a.TransitionTo(lifecycle.StateInitializing))
	require.NoError(t, a.TransitionTo(lifecycle.StateRunning))
	// notifyLoop 은 openCount 로 검증하지 않으므로 스킵(틱 노이즈 제거). spawn 블록 진입은
	// transport.Open() 호출(openCount)로 판정한다 — Open 은 모든 루프 spawn 직전에 실행된다.
	a.hvacr02Config.NotifyInterval = 0

	// transport.Available()==false 라 매 호출 fast-path 를 우회한다. 가드가 없으면
	// 매번 spawn 블록이 실행되어 Open 이 누적된다.
	require.NoError(t, a.Start(context.Background()))
	require.NoError(t, a.Start(context.Background()))
	require.NoError(t, a.Start(context.Background()))

	assert.Equal(t, int32(1), mock.openCount.Load(),
		"중복 Start 가 백그라운드 루프를 중복 기동하면 안 된다 (Open 1회)")

	// Stop 은 bgStarted 를 리셋하여 다음 Start 에서 다시 기동할 수 있게 해야 한다.
	require.NoError(t, a.Stop(context.Background()))
	assert.False(t, a.bgStarted.Load(), "Stop 후 bgStarted 는 리셋되어야 한다")
}
