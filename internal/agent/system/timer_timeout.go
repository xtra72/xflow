package system

import (
	"fmt"
	"sync/atomic"
	"time"
)

// SetTimeout 은 지정된 시간 후 1회 실행되는 타임아웃 타이머를 등록한다.
// 지연 시간이 0 이하이면 ErrInvalidDelay 를 반환한다.
// 실행 후 타이머 맵에서 자동으로 제거된다.
func (t *TimerAgent) SetTimeout(id string, delay time.Duration, handler TimerHandler) (TimerID, error) {
	// 공통 유효성 검사 (closed, paused, empty ID, nil handler, duplicate, max)
	if err := t.checkRegistration(id, handler); err != nil {
		return "", err
	}

	// 지연 시간 검증
	if delay <= 0 {
		return "", fmt.Errorf("%w", ErrInvalidDelay)
	}

	timerID := TimerID(id)

	// 타이머 엔트리 생성
	entry := &timerEntry{
		info: TimerInfo{
			ID:         timerID,
			Type:       TimerTypeTimeout,
			Expression: delay.String(),
			Active:     true,
		},
		handler: handler,
	}

	// time.AfterFunc 으로 1회 실행 타이머 등록
	entry.timer = time.AfterFunc(delay, func() {
		// paused 상태이면 핸들러 호출을 스킵한다
		t.mu.RLock()
		isPaused := t.paused
		isClosed := t.closed
		t.mu.RUnlock()

		if isClosed || isPaused {
			return
		}

		count := atomic.AddInt64(&entry.tickCount, 1)
		atomic.AddInt64(&t.stats.totalTriggers, 1)

		now := time.Now()

		trigger := TimerTrigger{
			TimerID:   entry.info.ID,
			TriggerAt: now,
			TickCount: count,
		}

		t.safeCall(entry.handler, trigger)

		// 실행 후 타이머 맵에서 자동 제거
		t.mu.Lock()
		entry.info.Active = false
		entry.info.LastFired = now
		entry.info.FireCount = count
		delete(t.timers, timerID)
		t.mu.Unlock()
	})

	// 맵에 등록 (race 방지를 위해 Lock 후 재검증)
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		entry.timer.Stop()
		return "", ErrTimerClosed
	}
	if t.paused {
		t.mu.Unlock()
		entry.timer.Stop()
		return "", ErrTimerPaused
	}
	if _, exists := t.timers[timerID]; exists {
		t.mu.Unlock()
		entry.timer.Stop()
		return "", ErrDuplicateTimerID
	}
	if len(t.timers) >= t.config.maxTimers {
		t.mu.Unlock()
		entry.timer.Stop()
		return "", ErrMaxTimersReached
	}
	t.timers[timerID] = entry
	t.mu.Unlock()

	return timerID, nil
}
