package system

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

// SetInterval 은 주기적으로 반복 실행되는 인터벌 타이머를 등록한다.
// 인터벌이 최소값 미만이면 ErrIntervalTooShort 를 반환한다.
func (t *TimerAgent) SetInterval(id string, interval time.Duration, handler TimerHandler) (TimerID, error) {
	// 공통 유효성 검사 (closed, paused, empty ID, nil handler, duplicate, max)
	if err := t.checkRegistration(id, handler); err != nil {
		return "", err
	}

	// 최소 인터벌 검증
	if interval < t.config.minInterval {
		return "", fmt.Errorf("%w", ErrIntervalTooShort)
	}

	timerID := TimerID(id)

	// 고루틴 취소를 위한 컨텍스트
	ctx, cancel := context.WithCancel(context.Background())

	// 타이머 엔트리 생성
	entry := &timerEntry{
		info: TimerInfo{
			ID:         timerID,
			Type:       TimerTypeInterval,
			Expression: interval.String(),
			Active:     true,
		},
		handler: handler,
		ticker:  time.NewTicker(interval),
		cancel:  cancel,
	}

	// 맵에 등록 (race 방지를 위해 Lock 후 재검증)
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		cancel()
		entry.ticker.Stop()
		return "", ErrTimerClosed
	}
	if t.paused {
		t.mu.Unlock()
		cancel()
		entry.ticker.Stop()
		return "", ErrTimerPaused
	}
	if _, exists := t.timers[timerID]; exists {
		t.mu.Unlock()
		cancel()
		entry.ticker.Stop()
		return "", ErrDuplicateTimerID
	}
	if len(t.timers) >= t.config.maxTimers {
		t.mu.Unlock()
		cancel()
		entry.ticker.Stop()
		return "", ErrMaxTimersReached
	}
	t.timers[timerID] = entry
	t.mu.Unlock()

	// 인터벌 고루틴 시작 (wg로 고루틴 추적)
	t.wg.Add(1)
	go t.runInterval(ctx, entry)

	return timerID, nil
}

// runInterval 은 인터벌 타이머의 반복 실행 고루틴이다.
func (t *TimerAgent) runInterval(ctx context.Context, entry *timerEntry) {
	defer t.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-entry.ticker.C:
			// paused 상태이면 핸들러 호출을 스킵한다
			t.mu.RLock()
			isPaused := t.paused
			t.mu.RUnlock()

			if isPaused {
				continue
			}

			count := atomic.AddInt64(&entry.tickCount, 1)
			atomic.AddInt64(&t.stats.totalTriggers, 1)

			// LastFired 업데이트
			now := time.Now()
			t.mu.Lock()
			entry.info.LastFired = now
			entry.info.FireCount = count
			t.mu.Unlock()

			trigger := TimerTrigger{
				TimerID:   entry.info.ID,
				TriggerAt: now,
				TickCount: count,
			}

			t.safeCall(entry.handler, trigger)
		}
	}
}
