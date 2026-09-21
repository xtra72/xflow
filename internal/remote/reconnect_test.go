// 재연결 규칙의 계약 (@SPEC:SPEC-REMOTE-RECONNECT-001).
//
// 요구는 셋이다 — 끊어진 직후에는 짧게, 실패가 이어지면 간격을 늘려, 멈추지 않고
// 계속 시도한다. 그리고 하한/상한을 설정으로 정할 수 있어야 한다.
package remote

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReconnect_BackoffGrowsAndCaps(t *testing.T) {
	c := NewClient(ClientConfig{
		ServerURL:        "wss://example",
		InstanceID:       "n1",
		ReconnectInitial: 100 * time.Millisecond,
		ReconnectMax:     800 * time.Millisecond,
	}, DialerFunc(func(context.Context, string) (Conn, error) { return nil, context.Canceled }))

	// 지터가 ±20% 이므로 범위로 본다. 배수는 2^(attempt-1) 이다.
	for _, tc := range []struct {
		attempt int
		want    time.Duration
	}{
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 800 * time.Millisecond},
	} {
		got := c.calculateBackoff(tc.attempt)
		require.GreaterOrEqual(t, got, time.Duration(float64(tc.want)*0.8))
		require.LessOrEqual(t, got, time.Duration(float64(tc.want)*1.2))
	}

	// 상한을 넘기면 상한에서 멈춘다(지터 범위 안).
	for _, attempt := range []int{5, 10, 50} {
		got := c.calculateBackoff(attempt)
		require.LessOrEqual(t, got, time.Duration(float64(800*time.Millisecond)*1.2),
			"상한을 넘어서 자라면 안 된다")
	}
}

func TestReconnect_ConfiguredBoundsAreUsed(t *testing.T) {
	c := NewClient(ClientConfig{
		ServerURL:        "wss://example",
		InstanceID:       "n1",
		ReconnectInitial: 5 * time.Second,
		ReconnectMax:     20 * time.Second,
	}, DialerFunc(func(context.Context, string) (Conn, error) { return nil, context.Canceled }))

	require.Equal(t, 5*time.Second, c.cfg.ReconnectInitial)
	require.Equal(t, 20*time.Second, c.cfg.ReconnectMax)
	first := c.calculateBackoff(1)
	require.GreaterOrEqual(t, first, 4*time.Second, "첫 간격은 설정한 하한 근처여야 한다")
}

func TestReconnect_ZeroBoundsFallBackToDefaults(t *testing.T) {
	c := NewClient(ClientConfig{ServerURL: "wss://example", InstanceID: "n1"},
		DialerFunc(func(context.Context, string) (Conn, error) { return nil, context.Canceled }))

	require.Equal(t, DefaultReconnectInitial, c.cfg.ReconnectInitial)
	require.Equal(t, DefaultReconnectMax, c.cfg.ReconnectMax)
}

func TestReconnect_InvertedBoundsBecomeFixedInterval(t *testing.T) {
	// 하한 60s / 상한 1s 처럼 뒤집어 적으면 상한이 하한을 잘라 첫 시도부터 1s 가 된다.
	// 설정한 사람의 뜻과 반대이므로 상한을 하한까지 올려 "간격 고정" 으로 읽는다.
	c := NewClient(ClientConfig{
		ServerURL:        "wss://example",
		InstanceID:       "n1",
		ReconnectInitial: 60 * time.Second,
		ReconnectMax:     1 * time.Second,
	}, DialerFunc(func(context.Context, string) (Conn, error) { return nil, context.Canceled }))

	require.Equal(t, 60*time.Second, c.cfg.ReconnectMax)
}

func TestReconnect_KeepsTryingWhileDialFails(t *testing.T) {
	var mu sync.Mutex
	dials := 0
	dialer := DialerFunc(func(context.Context, string) (Conn, error) {
		mu.Lock()
		dials++
		mu.Unlock()
		return nil, context.DeadlineExceeded
	})

	c := NewClient(ClientConfig{
		ServerURL:        "wss://example",
		InstanceID:       "n1",
		ReconnectInitial: 5 * time.Millisecond,
		ReconnectMax:     10 * time.Millisecond,
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)
	defer func() {
		cancel()
		c.Stop()
	}()

	// 연결이 계속 실패해도 시도를 멈추지 않는다.
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return dials >= 3
	}, 2*time.Second, 5*time.Millisecond, "실패해도 재시도가 이어져야 한다")
}

func TestReconnect_ShortSessionDoesNotResetBackoff(t *testing.T) {
	// dial 은 성공하는데 서버가 곧장 닫는 상태. 매번 백오프를 되돌리면 최소 간격으로
	// 영원히 두드리게 되므로, 짧게 끝난 세션은 실패로 세어 간격을 늘려야 한다.
	var mu sync.Mutex
	var gaps []time.Duration
	last := time.Now()

	dialer := DialerFunc(func(context.Context, string) (Conn, error) {
		mu.Lock()
		now := time.Now()
		gaps = append(gaps, now.Sub(last))
		last = now
		mu.Unlock()

		conn := newClientFakeConn()
		_ = conn.Close() // 붙자마자 끊긴다.
		return conn, nil
	})

	c := NewClient(ClientConfig{
		ServerURL:         "wss://example",
		InstanceID:        "n1",
		HeartbeatInterval: time.Hour,
		ReconnectInitial:  20 * time.Millisecond,
		ReconnectMax:      2 * time.Second,
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)
	defer func() {
		cancel()
		c.Stop()
	}()

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(gaps) >= 4
	}, 3*time.Second, 10*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	// 간격이 **분명히** 자라야 한다. 이웃한 두 간격을 비교하면 지터(±20%)만으로도
	// 우연히 참이 되어, 백오프를 되돌리는 코드에서도 통과한다(실제로 그랬다).
	// 되돌리면 모든 간격이 하한 언저리(20ms±20%)에 머무르므로, 하한의 3배를
	// 넘겼는지로 가른다 — 자라는 경우 4번째는 160ms 언저리다.
	require.Greater(t, gaps[3], 60*time.Millisecond,
		"붙자마자 끊기는 세션은 백오프를 되돌리지 않아야 한다(간격이 하한에 머물렀다)")
}
