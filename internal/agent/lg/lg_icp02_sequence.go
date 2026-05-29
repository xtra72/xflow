package lg

import "sync"

// Icp02SequenceManager 는 LG ICP-02 제어 프레임의 시퀀스 번호를 관리한다.
//
// 버스에서 캡처된 컨트롤러 프레임의 SEQ0/SEQ1 을 추적하고,
// 제어 프레임 전송 시 컨트롤러의 다음 시퀀스 값을 사용한다.
// 이렇게 해야 실내기가 유효한 시퀀스로 인식한다.
//
// SEQ0: CMD 타입별 독립 카운터 (컨트롤러가 관찰된 마지막 값 + 1)
// SEQ1: 전역 프레임 카운터 (컨트롤러가 관찰된 마지막 값 + 1)
//
// 모든 메서드는 동시성 안전하다.
type Icp02SequenceManager struct {
	mu        sync.Mutex
	seq0Map   map[[2]byte]byte // CMD별 관찰된 마지막 SEQ0
	alloc0Map map[[2]byte]byte // CMD별 할당 high-water mark
	seq1      byte             // 관찰된 마지막 SEQ1
	alloc1    byte             // SEQ1 할당 high-water mark
	synced    bool             // 버스에서 최소 1개 이상 관찰했는지
}

// NewIcp02SequenceManager 는 시퀀스 관리자를 생성한다.
func NewIcp02SequenceManager() *Icp02SequenceManager {
	return &Icp02SequenceManager{
		seq0Map:   make(map[[2]byte]byte),
		alloc0Map: make(map[[2]byte]byte),
	}
}

// ObserveFrame 은 캡처된 컨트롤러 프레임의 시퀀스 값을 기록한다.
// captureLoop 에서 컨트롤러(SA=controllerAddr) 발신 프레임을 관찰할 때 호출한다.
// 관찰값이 할당 high-water mark 보다 앞서면 갱신한다.
func (s *Icp02SequenceManager) ObserveFrame(cmd [2]byte, seq0, seq1 byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq0Map[cmd] = seq0
	// alloc 맵 갱신: 관찰값이 현재 할당값보다 앞설 때만 (또는 첫 관찰)
	if _, exists := s.alloc0Map[cmd]; !exists || int8(seq0-s.alloc0Map[cmd]) > 0 {
		s.alloc0Map[cmd] = seq0
	}

	s.seq1 = seq1
	if !s.synced || int8(seq1-s.alloc1) > 0 {
		s.alloc1 = seq1
	}
	s.synced = true
}

// Synced 는 버스에서 최소 1개 이상의 컨트롤러 프레임을 관찰했는지 반환한다.
func (s *Icp02SequenceManager) Synced() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.synced
}

// NextSEQ0 는 지정된 CMD 에 대한 다음 SEQ0 값을 반환한다.
// 관찰된 마지막 값 + 1 을 사용한다. 0xFF 이후 0x00 으로 순환한다.
// 읽기 전용: 내부 카운터를 변경하지 않는다.
func (s *Icp02SequenceManager) NextSEQ0(cmd [2]byte) byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.seq0Map[cmd] + 1
}

// NextSEQ1 는 다음 전역 SEQ1 값을 반환한다.
// 관찰된 마지막 값 + 1 을 사용한다. 0xFF 이후 0x00 으로 순환한다.
// 읽기 전용: 내부 카운터를 변경하지 않는다.
func (s *Icp02SequenceManager) NextSEQ1() byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.seq1 + 1
}

// AllocSEQ0 는 다음 SEQ0 값을 반환하고 할당 카운터를 전진시킨다.
// 프레임 전송 시 사용: 매 호출마다 고유한 SEQ0 를 보장한다.
// ObserveFrame 이 중간에 호출되어도 할당값이 뒤로 가지 않는다.
func (s *Icp02SequenceManager) AllocSEQ0(cmd [2]byte) byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := s.alloc0Map[cmd] + 1
	s.alloc0Map[cmd] = next
	return next
}

// AllocSEQ1 는 다음 SEQ1 값을 반환하고 할당 카운터를 전진시킨다.
// 프레임 전송 시 사용: 매 호출마다 고유한 SEQ1 를 보장한다.
func (s *Icp02SequenceManager) AllocSEQ1() byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := s.alloc1 + 1
	s.alloc1 = next
	return next
}

// Reset 은 모든 시퀀스 카운터를 0x00 으로 초기화한다.
func (s *Icp02SequenceManager) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq0Map = make(map[[2]byte]byte)
	s.alloc0Map = make(map[[2]byte]byte)
	s.seq1 = 0
	s.alloc1 = 0
	s.synced = false
}
