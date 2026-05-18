package century

import (
	"crypto/sha256"
	"sync"
)

// writeDedupRetention 은 deduplicator 가 보관하는 최근 cycle 수의 상한이다 (REQ-CENTURY-027).
//
// Century master 는 한 cycle 마다 두 번의 WRITE 를 보내고 cycle 길이는 ~512ms 이므로
// 4 cycle (= 약 2초) 분량의 기억만으로 cycle 경계를 안전히 넘긴 dedup 결정을 내릴 수 있다.
// 그보다 오래된 cycle 의 payload 흔적은 새 cycle 의 신규 명령으로 해석되어야 하므로 폐기한다.
const writeDedupRetention = 4

// WriteDeduplicator 는 동일 polling cycle 내 중복 WRITE 프레임을 추적한다 (REQ-CENTURY-027).
//
// 사용 패턴:
//
//	if !d.ShouldEmit(cycleID, rawPayload) {
//	    // 같은 cycle 내 중복 — decoded event 를 emit 하지 않는다.
//	    a.stats.writesDeduped.Add(1)
//	    return
//	}
//
// dedup 키는 (cycleID, sha256(rawPayload)) 이다. payload 의 fingerprint 만 보관하므로
// 메모리 사용량은 O(N cycles × M unique payloads) 로 제한된다. 4 cycle 만 보관하여
// 무한정 증가하지 않는다.
//
// concurrent-safe.
type WriteDeduplicator struct {
	mu sync.Mutex
	// cycles 는 retention 정책에 따라 최근 N 개의 cycle 만 보관한다.
	// FIFO 순서로 evict 한다 — 가장 오래된 cycle 의 엔트리를 제거.
	cycles []cycleEntry
}

type cycleEntry struct {
	id   uint64
	seen map[[32]byte]struct{} // sha256(payload) → 관측 여부
}

// NewWriteDeduplicator 는 빈 deduplicator 를 생성한다.
func NewWriteDeduplicator() *WriteDeduplicator {
	return &WriteDeduplicator{}
}

// ShouldEmit 는 주어진 cycleID 와 payload 에 대해 decoded event 를 emit 할지 여부를 반환한다.
//
// 처음 보는 (cycle, payload) 조합이면 true 를 반환하고 내부 상태에 기록한다.
// 이미 같은 cycle 에서 같은 payload 가 관측되었으면 false 를 반환한다 (dedup).
//
// dedup 의 cycle 격리: 같은 payload 라도 cycle 이 다르면 emit 한다 (AC-F3).
func (d *WriteDeduplicator) ShouldEmit(cycleID uint64, payload []byte) bool {
	fp := sha256.Sum256(payload)

	d.mu.Lock()
	defer d.mu.Unlock()

	// cycleID 에 해당하는 엔트리 검색 또는 생성.
	idx := d.findCycleIndex(cycleID)
	if idx == -1 {
		// 새 cycle 엔트리 추가 (retention 정책 적용).
		if len(d.cycles) >= writeDedupRetention {
			d.cycles = d.cycles[1:] // 가장 오래된 cycle 제거.
		}
		d.cycles = append(d.cycles, cycleEntry{
			id:   cycleID,
			seen: map[[32]byte]struct{}{fp: {}},
		})
		return true
	}

	// 기존 cycle 엔트리에서 fingerprint 검색.
	entry := &d.cycles[idx]
	if _, dup := entry.seen[fp]; dup {
		return false
	}
	entry.seen[fp] = struct{}{}
	return true
}

// findCycleIndex 는 cycles 슬라이스에서 주어진 cycleID 의 인덱스를 반환한다.
// 없으면 -1.
func (d *WriteDeduplicator) findCycleIndex(cycleID uint64) int {
	for i := range d.cycles {
		if d.cycles[i].id == cycleID {
			return i
		}
	}
	return -1
}
