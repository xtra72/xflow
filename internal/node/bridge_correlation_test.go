package node

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/message"
)

// --- CorrelationTracker 생성 테스트 ---

// TestNewCorrelationTracker_생성 은 CorrelationTracker가 올바르게 생성되는지 확인한다.
func TestNewCorrelationTracker_생성(t *testing.T) {
	tracker := NewCorrelationTracker(30 * time.Second)
	require.NotNil(t, tracker)
	assert.Equal(t, 0, tracker.PendingCount())
	assert.Equal(t, int64(0), tracker.TimeoutCount())
}

// --- Track / Resolve 테스트 ---

// TestCorrelationTracker_Track_Resolve_정상 은 Track 후 Resolve가 정상 동작하는지 확인한다.
func TestCorrelationTracker_Track_Resolve_정상(t *testing.T) {
	tracker := NewCorrelationTracker(30 * time.Second)
	defer tracker.Close()

	ch := make(chan message.Message, 1)
	tracker.Track("corr-1", ch)

	assert.Equal(t, 1, tracker.PendingCount())

	response := message.New()
	resolved := tracker.Resolve("corr-1", response)
	assert.True(t, resolved)

	// 채널에서 응답을 수신할 수 있어야 한다
	select {
	case msg := <-ch:
		assert.Equal(t, response.ID(), msg.ID())
	case <-time.After(1 * time.Second):
		t.Fatal("응답 수신 타임아웃")
	}
}

// TestCorrelationTracker_Resolve_미등록ID 는 등록되지 않은 ID로 Resolve하면 false를 반환하는지 확인한다.
func TestCorrelationTracker_Resolve_미등록ID(t *testing.T) {
	tracker := NewCorrelationTracker(30 * time.Second)
	defer tracker.Close()

	response := message.New()
	resolved := tracker.Resolve("not-exists", response)
	assert.False(t, resolved)
}

// TestCorrelationTracker_Resolve_중복호출 은 같은 ID로 두 번 Resolve하면 두 번째는 false를 반환하는지 확인한다.
func TestCorrelationTracker_Resolve_중복호출(t *testing.T) {
	tracker := NewCorrelationTracker(30 * time.Second)
	defer tracker.Close()

	ch := make(chan message.Message, 1)
	tracker.Track("corr-1", ch)

	response := message.New()
	assert.True(t, tracker.Resolve("corr-1", response))
	assert.False(t, tracker.Resolve("corr-1", response))
}

// --- SetTimeout 테스트 ---

// TestCorrelationTracker_SetTimeout 은 타임아웃 변경이 동작하는지 확인한다.
func TestCorrelationTracker_SetTimeout(t *testing.T) {
	tracker := NewCorrelationTracker(30 * time.Second)
	defer tracker.Close()

	tracker.SetTimeout(5 * time.Second)
	// 에러 없이 완료되면 성공
}

// --- Cleanup 테스트 ---

// TestCorrelationTracker_Cleanup_만료항목제거 는 만료된 항목을 제거하고 채널을 닫는지 확인한다.
func TestCorrelationTracker_Cleanup_만료항목제거(t *testing.T) {
	tracker := NewCorrelationTracker(50 * time.Millisecond)
	defer tracker.Close()

	ch := make(chan message.Message, 1)
	tracker.Track("expired-1", ch)

	// 만료 대기
	time.Sleep(100 * time.Millisecond)

	tracker.Cleanup()

	assert.Equal(t, 0, tracker.PendingCount())
	assert.Equal(t, int64(1), tracker.TimeoutCount())

	// 채널이 닫혀야 한다
	_, ok := <-ch
	assert.False(t, ok, "만료된 항목의 채널이 닫혀야 한다")
}

// TestCorrelationTracker_Cleanup_유효항목보존 은 아직 만료되지 않은 항목을 보존하는지 확인한다.
func TestCorrelationTracker_Cleanup_유효항목보존(t *testing.T) {
	tracker := NewCorrelationTracker(10 * time.Second)
	defer tracker.Close()

	ch := make(chan message.Message, 1)
	tracker.Track("valid-1", ch)

	tracker.Cleanup()

	assert.Equal(t, 1, tracker.PendingCount())
}

// --- PendingCount 테스트 ---

// TestCorrelationTracker_PendingCount 은 추적 중인 항목 수를 정확히 반환하는지 확인한다.
func TestCorrelationTracker_PendingCount(t *testing.T) {
	tracker := NewCorrelationTracker(30 * time.Second)
	defer tracker.Close()

	assert.Equal(t, 0, tracker.PendingCount())

	ch1 := make(chan message.Message, 1)
	ch2 := make(chan message.Message, 1)
	ch3 := make(chan message.Message, 1)

	tracker.Track("a", ch1)
	assert.Equal(t, 1, tracker.PendingCount())

	tracker.Track("b", ch2)
	assert.Equal(t, 2, tracker.PendingCount())

	tracker.Track("c", ch3)
	assert.Equal(t, 3, tracker.PendingCount())

	tracker.Resolve("b", message.New())
	assert.Equal(t, 2, tracker.PendingCount())
}

// --- Close 테스트 ---

// TestCorrelationTracker_Close_모든채널닫기 는 Close가 모든 대기 채널을 닫는지 확인한다.
func TestCorrelationTracker_Close_모든채널닫기(t *testing.T) {
	tracker := NewCorrelationTracker(30 * time.Second)

	channels := make([]chan message.Message, 5)
	for i := range channels {
		channels[i] = make(chan message.Message, 1)
		tracker.Track("close-"+string(rune('A'+i)), channels[i])
	}

	assert.Equal(t, 5, tracker.PendingCount())

	tracker.Close()

	assert.Equal(t, 0, tracker.PendingCount())

	// 모든 채널이 닫혀야 한다
	for i, ch := range channels {
		_, ok := <-ch
		assert.False(t, ok, "채널 %d가 닫혀야 한다", i)
	}
}

// TestCorrelationTracker_Close_중복호출 은 Close를 여러 번 호출해도 패닉이 발생하지 않는지 확인한다.
func TestCorrelationTracker_Close_중복호출(t *testing.T) {
	tracker := NewCorrelationTracker(30 * time.Second)

	ch := make(chan message.Message, 1)
	tracker.Track("dup-close", ch)

	// 패닉 없이 여러 번 호출 가능해야 한다
	assert.NotPanics(t, func() {
		tracker.Close()
		tracker.Close()
	})
}

// --- StartCleanupLoop 테스트 ---

// TestCorrelationTracker_StartCleanupLoop_자동정리 는 클린업 루프가 만료된 항목을 자동으로 제거하는지 확인한다.
func TestCorrelationTracker_StartCleanupLoop_자동정리(t *testing.T) {
	timeout := 50 * time.Millisecond
	tracker := NewCorrelationTracker(timeout)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tracker.StartCleanupLoop(ctx)

	ch := make(chan message.Message, 1)
	tracker.Track("auto-cleanup", ch)

	// 클린업 루프가 실행될 시간 대기 (timeout/2 간격 + 만료 시간)
	time.Sleep(timeout * 3)

	assert.Equal(t, 0, tracker.PendingCount())
	assert.Equal(t, int64(1), tracker.TimeoutCount())

	tracker.Close()
}

// TestCorrelationTracker_StartCleanupLoop_컨텍스트취소 는 컨텍스트 취소 시 루프가 종료되는지 확인한다.
func TestCorrelationTracker_StartCleanupLoop_컨텍스트취소(t *testing.T) {
	tracker := NewCorrelationTracker(1 * time.Hour) // 긴 타임아웃
	defer tracker.Close()

	ctx, cancel := context.WithCancel(context.Background())
	tracker.StartCleanupLoop(ctx)

	// 즉시 취소
	cancel()

	// 패닉 없이 종료되면 성공
	time.Sleep(50 * time.Millisecond)
}

// --- 동시성 테스트 ---

// TestCorrelationTracker_동시Track_Resolve 는 여러 고루틴에서 동시에 Track/Resolve를 호출해도 안전한지 확인한다.
func TestCorrelationTracker_동시Track_Resolve(t *testing.T) {
	tracker := NewCorrelationTracker(30 * time.Second)
	defer tracker.Close()

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines * 2) // Track + Resolve

	// 동시 Track
	channels := make([]chan message.Message, goroutines)
	for i := 0; i < goroutines; i++ {
		channels[i] = make(chan message.Message, 1)
	}

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			id := "concurrent-" + string(rune('A'+idx%26)) + "-" + time.Now().String()
			tracker.Track(id, channels[idx])
		}(i)
	}

	// 동시 Resolve (일부는 실패할 수 있음 - ID가 매칭되지 않을 수 있으므로)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			tracker.Resolve("nonexistent", message.New())
		}()
	}

	wg.Wait()
	// 패닉 없이 완료되면 성공
}

// TestCorrelationTracker_동시Cleanup 은 Cleanup이 동시에 호출되어도 안전한지 확인한다.
func TestCorrelationTracker_동시Cleanup(t *testing.T) {
	tracker := NewCorrelationTracker(1 * time.Millisecond)

	const goroutines = 10
	for i := 0; i < goroutines; i++ {
		ch := make(chan message.Message, 1)
		tracker.Track("cleanup-"+string(rune('A'+i)), ch)
	}

	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			tracker.Cleanup()
		}()
	}
	wg.Wait()

	tracker.Close()
	// 패닉 없이 완료되면 성공
}

// TestCorrelationTracker_Track후_즉시Resolve 는 Track 직후 Resolve가 정상 동작하는지 확인한다.
func TestCorrelationTracker_Track후_즉시Resolve(t *testing.T) {
	tracker := NewCorrelationTracker(30 * time.Second)
	defer tracker.Close()

	const count = 50
	var wg sync.WaitGroup
	wg.Add(count)

	for i := 0; i < count; i++ {
		go func(idx int) {
			defer wg.Done()
			id := "quick-" + string(rune('A'+(idx%26)))
			ch := make(chan message.Message, 1)
			tracker.Track(id, ch)
			response := message.New()
			tracker.Resolve(id, response)
		}(i)
	}

	wg.Wait()
}
