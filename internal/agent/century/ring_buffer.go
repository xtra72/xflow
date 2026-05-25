package century

import (
	"sync"
	"sync/atomic"
	"time"
)

// CapturedFrame 은 ring buffer 의 한 엔트리이다 (REQ-CENTURY-012).
//
// agent 의 captureLoop 이 *Frame 을 디코딩한 직후 ring buffer 에 push 하기 위해 사용된다.
// raw 바이트는 raw-frame node 의 송출 (REQ-CENTURY-019) 을 위해 보존된다.
// Decoded 는 Decode() 가 반환한 typed message (또는 디코드 실패 시 nil) 이다.
type CapturedFrame struct {
	Seq        uint64    // 캡처 시작 이후 단조 증가 시퀀스
	ReceivedAt time.Time // 프레임 수신 시각
	Raw        []byte    // 원시 바이트 (header + payload + CRC). 호출자가 보유 가능하도록 새로 할당된 슬라이스.
	Frame      *Frame    // ParseFrame 결과 (필수)
	Decoded    any       // Decode() 결과: *Reg02Decoded / *Reg03Decoded / *Reg04ReadDecoded / *Reg04WriteDecoded / *ACKDecoded, 디코드 실패 시 nil.
	DecodeErr  error     // Decode() 가 반환한 에러 (없으면 nil)
}

// FrameRingBuffer 는 고정 크기 FIFO 링 버퍼이다 (REQ-CENTURY-012).
//
// 가득 찬 상태에서 Push 시 가장 오래된 엔트리를 evict 하고 dropCount 를 증가시킨다.
// concurrent-safe (단일 mutex). drop 카운터는 sync/atomic.Uint64 로 lock 외부에서도 읽을 수 있다.
type FrameRingBuffer struct {
	capacity int
	mu       sync.Mutex
	buf      []CapturedFrame
	head     int // 다음 push 위치
	size     int // 현재 보유 개수 (≤ capacity)

	totalPushed atomic.Uint64
	dropCount   atomic.Uint64
}

// NewFrameRingBuffer 는 capacity 크기의 빈 ring buffer 를 생성한다.
//
// capacity 는 1 이상이어야 한다. 0 이하는 1 로 클램프한다 (방어).
func NewFrameRingBuffer(capacity int) *FrameRingBuffer {
	if capacity <= 0 {
		capacity = 1
	}
	return &FrameRingBuffer{
		capacity: capacity,
		buf:      make([]CapturedFrame, capacity),
	}
}

// Capacity 는 버퍼의 최대 크기를 반환한다.
func (r *FrameRingBuffer) Capacity() int { return r.capacity }

// Push 는 새 엔트리를 추가한다.
//
// 가득 찬 경우 가장 오래된 엔트리를 evict 하고 dropCount 를 1 증가시킨다.
// 반환값은 evict 로 인해 drop 이 발생했는지 여부 (호출자가 per-drop 로그 분기에 사용).
func (r *FrameRingBuffer) Push(rec CapturedFrame) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.totalPushed.Add(1)
	dropped := false
	if r.size == r.capacity {
		// 가장 오래된 엔트리를 덮어쓰고 size 는 유지.
		r.dropCount.Add(1)
		dropped = true
	} else {
		r.size++
	}
	r.buf[r.head] = rec
	r.head = (r.head + 1) % r.capacity
	return dropped
}

// Len 은 현재 보유 엔트리 개수를 반환한다.
func (r *FrameRingBuffer) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.size
}

// TotalPushed 는 누적 push 횟수를 반환한다 (drop 포함).
func (r *FrameRingBuffer) TotalPushed() uint64 {
	return r.totalPushed.Load()
}

// DropCount 는 누적 drop 횟수를 반환한다.
func (r *FrameRingBuffer) DropCount() uint64 {
	return r.dropCount.Load()
}

// GetRecent 는 가장 최근의 count 개 엔트리를 삽입 순서대로 반환한다 (비파괴).
//
// count 가 현재 size 보다 크면 size 개를 반환한다. 결과는 새 슬라이스이며 호출자가 안전하게 보유 가능.
// (AC-B7 의 비파괴 시맨틱)
func (r *FrameRingBuffer) GetRecent(count int) []CapturedFrame {
	r.mu.Lock()
	defer r.mu.Unlock()
	if count <= 0 || r.size == 0 {
		return nil
	}
	if count > r.size {
		count = r.size
	}
	out := make([]CapturedFrame, count)
	// 가장 오래된 인덱스부터 시작 (head 는 다음 push 위치 = 가장 오래된 + size).
	start := (r.head - r.size + r.capacity) % r.capacity
	// 우리는 count 개의 가장 최근 = 마지막 count 개를 원한다.
	// 가장 최근의 첫 번째는 (head - count + capacity) % capacity 에서 시작.
	begin := (r.head - count + r.capacity) % r.capacity
	for i := 0; i < count; i++ {
		out[i] = r.buf[(begin+i)%r.capacity]
	}
	_ = start
	return out
}

// Drain 은 모든 엔트리를 삽입 순서대로 반환하고 버퍼를 비운다.
//
// (AC-B8)
func (r *FrameRingBuffer) Drain() []CapturedFrame {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size == 0 {
		return nil
	}
	out := make([]CapturedFrame, r.size)
	start := (r.head - r.size + r.capacity) % r.capacity
	for i := 0; i < r.size; i++ {
		out[i] = r.buf[(start+i)%r.capacity]
	}
	// 버퍼 비우기.
	for i := range r.buf {
		r.buf[i] = CapturedFrame{}
	}
	r.head = 0
	r.size = 0
	return out
}
