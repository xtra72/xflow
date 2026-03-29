package tsdb

import "sync"

// Subscriber 는 시리즈별 실시간 구독을 관리한다.
type Subscriber struct {
	mu   sync.RWMutex
	subs map[string]map[chan DataPoint]struct{} // seriesKey -> set of channels
}

// NewSubscriber 는 새로운 Subscriber를 생성한다.
func NewSubscriber() *Subscriber {
	return &Subscriber{
		subs: make(map[string]map[chan DataPoint]struct{}),
	}
}

// Subscribe 는 지정된 시리즈 키에 대한 구독을 시작한다.
// 새 DataPoint가 기록될 때마다 채널로 전달된다.
// 반환된 함수를 호출하면 구독이 해제된다.
func (s *Subscriber) Subscribe(keys []string, ch chan DataPoint) (unsubscribe func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, key := range keys {
		if s.subs[key] == nil {
			s.subs[key] = make(map[chan DataPoint]struct{})
		}
		s.subs[key][ch] = struct{}{}
	}

	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		for _, key := range keys {
			if m, ok := s.subs[key]; ok {
				delete(m, ch)
				if len(m) == 0 {
					delete(s.subs, key)
				}
			}
		}
	}
}

// Notify 는 지정된 시리즈 키의 구독자들에게 새 DataPoint를 전달한다.
// 비차단(non-blocking) 전송으로, 채널이 가득 차면 해당 구독자를 건너뛴다.
func (s *Subscriber) Notify(key string, dp DataPoint) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	chs, ok := s.subs[key]
	if !ok {
		return
	}

	for ch := range chs {
		select {
		case ch <- dp:
		default:
			// 채널이 가득 차면 건너뛴다
		}
	}
}

// SubscriberCount 는 특정 시리즈의 구독자 수를 반환한다.
func (s *Subscriber) SubscriberCount(key string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.subs[key])
}

// Close 는 모든 구독을 해제하고 채널을 닫는다.
func (s *Subscriber) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 모든 채널을 닫는다
	closed := make(map[chan DataPoint]struct{})
	for _, chs := range s.subs {
		for ch := range chs {
			if _, ok := closed[ch]; !ok {
				close(ch)
				closed[ch] = struct{}{}
			}
		}
	}

	// 맵 초기화
	s.subs = make(map[string]map[chan DataPoint]struct{})
}
