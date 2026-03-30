package system

import (
	"sync"
	"sync/atomic"
	"time"
)

// ttlManager 는 백그라운드 TTL 만료 스캔을 관리하는 구조체이다.
type ttlManager struct {
	store        *VolatileStore
	scanInterval time.Duration
	stopCh       chan struct{}
	paused       atomic.Bool
	running      bool
	mu           sync.Mutex
}

// newTTLManager 는 VolatileStore와 스캔 주기로 새 ttlManager를 생성한다.
func newTTLManager(store *VolatileStore, scanInterval time.Duration) *ttlManager {
	return &ttlManager{
		store:        store,
		scanInterval: scanInterval,
		stopCh:       make(chan struct{}),
	}
}

// Start 는 백그라운드 TTL 스캔 고루틴을 시작한다.
func (t *ttlManager) Start() {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return
	}
	t.running = true
	t.stopCh = make(chan struct{})
	t.mu.Unlock()

	go t.loop()
}

// Stop 은 백그라운드 스캔 고루틴을 종료한다.
func (t *ttlManager) Stop() {
	t.mu.Lock()
	if !t.running {
		t.mu.Unlock()
		return
	}
	t.running = false
	close(t.stopCh)
	t.mu.Unlock()
}

// Pause 는 스캔을 일시정지한다.
func (t *ttlManager) Pause() {
	t.paused.Store(true)
}

// Resume 은 일시정지된 스캔을 재개한다.
func (t *ttlManager) Resume() {
	t.paused.Store(false)
}

// SetInterval 은 스캔 주기를 변경한다.
// 다음 스캔 주기부터 적용된다.
func (t *ttlManager) SetInterval(interval time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.scanInterval = interval
}

// isRunning 은 고루틴이 실행 중인지 반환한다.
func (t *ttlManager) isRunning() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

// getInterval 은 현재 스캔 주기를 반환한다 (스레드 안전).
func (t *ttlManager) getInterval() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.scanInterval
}

// loop 은 백그라운드 스캔 루프이다.
func (t *ttlManager) loop() {
	ticker := time.NewTicker(t.getInterval())
	defer ticker.Stop()

	for {
		select {
		case <-t.stopCh:
			return

		case <-ticker.C:
			if !t.paused.Load() {
				t.scan()
			}
			// 매 틱마다 주기 변경을 반영한다
			ticker.Reset(t.getInterval())
		}
	}
}

// scan 은 만료된 키를 스캔하여 삭제하고, 삭제된 키 수를 반환한다.
// historyTTL이 설정된 경우, 만료되지 않은 키의 오래된 히스토리 항목도 정리한다.
func (t *ttlManager) scan() int {
	now := time.Now()
	deleted := 0

	t.store.data.Range(func(k, v any) bool {
		item := v.(*storeItem)
		if !item.expiresAt.IsZero() && now.After(item.expiresAt) {
			t.store.data.Delete(k)
			deleted++
		}
		return true
	})

	// historyTTL이 설정된 경우 만료되지 않은 키의 오래된 히스토리 항목을 정리한다
	if t.store.historyTTL > 0 {
		cutoff := now.Add(-t.store.historyTTL)
		t.store.data.Range(func(k, v any) bool {
			item := v.(*storeItem)
			if len(item.history) == 0 {
				return true
			}
			// 히스토리는 최신순이므로, cutoff 이전 항목을 뒤에서부터 찾아 잘라낸다
			trimIdx := len(item.history)
			for i, h := range item.history {
				if h.timestamp.Before(cutoff) {
					trimIdx = i
					break
				}
			}
			if trimIdx < len(item.history) {
				item.history = item.history[:trimIdx]
			}
			return true
		})
	}

	return deleted
}
