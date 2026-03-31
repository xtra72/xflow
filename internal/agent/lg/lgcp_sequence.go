package lg

import "sync"

// LGCPSequenceManager 는 LGCP 제어 프레임의 시퀀스 번호를 관리한다.
// SEQ0: CMD 타입별 독립 카운터, SEQ1: 전역 프레임 카운터.
// 모든 메서드는 동시성 안전하다.
type LGCPSequenceManager struct {
	mu      sync.Mutex
	seq0Map map[[2]byte]byte
	seq1    byte
}

// NewLGCPSequenceManager 는 시퀀스 관리자를 생성한다.
func NewLGCPSequenceManager() *LGCPSequenceManager {
	return &LGCPSequenceManager{
		seq0Map: make(map[[2]byte]byte),
	}
}

// NextSEQ0 는 지정된 CMD 에 대한 SEQ0 값을 반환하고 카운터를 증가시킨다.
// 0xFF 이후 0x00 으로 순환한다.
func (s *LGCPSequenceManager) NextSEQ0(cmd [2]byte) byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	val := s.seq0Map[cmd]
	s.seq0Map[cmd] = val + 1 // 자동으로 0xFF -> 0x00 래핑
	return val
}

// NextSEQ1 는 전역 SEQ1 값을 반환하고 카운터를 증가시킨다.
// 0xFF 이후 0x00 으로 순환한다.
func (s *LGCPSequenceManager) NextSEQ1() byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	val := s.seq1
	s.seq1 = val + 1
	return val
}

// Reset 은 모든 시퀀스 카운터를 0x00 으로 초기화한다.
func (s *LGCPSequenceManager) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq0Map = make(map[[2]byte]byte)
	s.seq1 = 0
}
