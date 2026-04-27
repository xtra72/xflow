package lifecycle

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// callbackEntry 는 콜백 함수와 고유 ID를 묶은 내부 구조체이다.
type callbackEntry struct {
	id      int
	cb      StateChangeCallback
	removed bool
}

// BaseLifecycle 은 Lifecycle 인터페이스의 공통 상태 관리를 제공하는 기본 구현체이다.
type BaseLifecycle struct {
	name      string
	state     State
	mu        sync.Mutex
	callbacks []callbackEntry
	nextID    int
}

// NewBaseLifecycle 은 주어진 옵션으로 BaseLifecycle을 생성한다.
// 초기 상태는 StateCreated이다.
func NewBaseLifecycle(opts ...BaseOption) *BaseLifecycle {
	b := &BaseLifecycle{
		state: StateCreated,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// TransitionTo 는 현재 상태에서 newState로 전이를 시도한다.
// 유효하지 않은 전이이면 에러를 반환한다.
// 콜백은 락 해제 후 호출되어 데드락을 방지한다.
func (b *BaseLifecycle) TransitionTo(newState State) error {
	b.mu.Lock()

	if !IsValidTransition(b.state, newState) {
		from := b.state
		b.mu.Unlock()
		return fmt.Errorf("lifecycle: %q에서 %q로의 전이는 허용되지 않는다: %w",
			from, newState, ErrInvalidStateTransition)
	}

	from := b.state
	b.state = newState

	// 콜백 스냅샷을 복사한다 (락 안에서)
	snapshot := make([]callbackEntry, len(b.callbacks))
	copy(snapshot, b.callbacks)

	b.mu.Unlock()

	// 락 해제 후 콜백을 호출한다 (데드락 방지)
	event := StateChangeEvent{
		Component: b.name,
		From:      from,
		To:        newState,
		Timestamp: time.Now(),
	}

	for _, entry := range snapshot {
		if entry.removed {
			continue
		}
		b.safeCallCallback(entry.cb, event)
	}

	return nil
}

// safeCallCallback 은 콜백을 안전하게 호출한다.
// 콜백에서 패닉이 발생해도 복구하여 다른 콜백에 영향을 주지 않는다 (AC-LIFE-001-25).
func (b *BaseLifecycle) safeCallCallback(cb StateChangeCallback, event StateChangeEvent) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "lifecycle: 콜백 패닉 복구: %v\n", r)
		}
	}()
	cb(event)
}

// CurrentState 는 컴포넌트의 현재 상태를 반환한다 (스레드 안전).
func (b *BaseLifecycle) CurrentState() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// OnStateChange 는 상태 변경 콜백을 등록하고 구독 해제 함수를 반환한다.
func (b *BaseLifecycle) OnStateChange(cb StateChangeCallback) UnsubscribeFunc {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	b.callbacks = append(b.callbacks, callbackEntry{
		id: id,
		cb: cb,
	})
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		for i := range b.callbacks {
			if b.callbacks[i].id == id {
				b.callbacks[i].removed = true
				break
			}
		}
	}
}

// ComponentName 은 컴포넌트의 이름을 반환한다.
func (b *BaseLifecycle) ComponentName() string {
	return b.name
}
