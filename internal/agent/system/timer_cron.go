package system

import (
	"fmt"
	"sync/atomic"
	"time"
)

// SetCron 은 cron 표현식 기반 타이머를 등록한다.
// 유효하지 않은 cron 표현식이면 ErrInvalidCronExpression 을 반환한다.
func (t *TimerAgent) SetCron(id string, cronExpr string, handler TimerHandler) (TimerID, error) {
	// 공통 유효성 검사 (closed, paused, empty ID, nil handler, duplicate, max)
	if err := t.checkRegistration(id, handler); err != nil {
		return "", err
	}

	timerID := TimerID(id)

	// cron 표현식 유효성 검증
	_, err := t.config.cronParser.Parse(cronExpr)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrInvalidCronExpression, err.Error())
	}

	// 타이머 엔트리 생성
	entry := &timerEntry{
		info: TimerInfo{
			ID:         timerID,
			Type:       TimerTypeCron,
			Expression: cronExpr,
			Active:     true,
		},
		handler: handler,
	}

	// cron 스케줄러에 등록 (래핑된 핸들러)
	cronEntryID, err := t.cronSched.AddFunc(cronExpr, func() {
		// paused 상태이면 핸들러 호출을 스킵한다
		t.mu.RLock()
		isPaused := t.paused
		t.mu.RUnlock()

		if isPaused {
			return
		}

		count := atomic.AddInt64(&entry.tickCount, 1)
		atomic.AddInt64(&t.stats.totalTriggers, 1)

		now := time.Now()
		t.mu.Lock()
		entry.info.LastFired = now
		entry.info.FireCount = count
		t.mu.Unlock()

		trigger := TimerTrigger{
			TimerID:    entry.info.ID,
			TriggerAt:  now,
			TickCount:  count,
			ScheduleID: fmt.Sprintf("%d", entry.cronID),
		}

		t.safeCall(entry.handler, trigger)
	})
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrInvalidCronExpression, err.Error())
	}

	entry.cronID = cronEntryID

	// 맵에 등록 (race 방지를 위해 Lock 후 재검증)
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		t.cronSched.Remove(cronEntryID)
		return "", ErrTimerClosed
	}
	if t.paused {
		t.mu.Unlock()
		t.cronSched.Remove(cronEntryID)
		return "", ErrTimerPaused
	}
	if _, exists := t.timers[timerID]; exists {
		t.mu.Unlock()
		t.cronSched.Remove(cronEntryID)
		return "", ErrDuplicateTimerID
	}
	if len(t.timers) >= t.config.maxTimers {
		t.mu.Unlock()
		t.cronSched.Remove(cronEntryID)
		return "", ErrMaxTimersReached
	}
	t.timers[timerID] = entry
	t.mu.Unlock()

	return timerID, nil
}
