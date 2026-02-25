package engine

import (
	"testing"
	"time"
)

// TestPortCounterRecord 는 portCounter.Record 가 메시지 수를 증가시키고
// firstSeen/lastSeen 을 올바르게 기록하는지 검증한다.
func TestPortCounterRecord(t *testing.T) {
	pc := &portCounter{}

	// 초기 상태: 모든 값이 0
	if got := pc.messages.Load(); got != 0 {
		t.Errorf("초기 messages = %d, want 0", got)
	}
	if got := pc.firstSeen.Load(); got != 0 {
		t.Errorf("초기 firstSeen = %d, want 0", got)
	}
	if got := pc.lastSeen.Load(); got != 0 {
		t.Errorf("초기 lastSeen = %d, want 0", got)
	}

	// 첫 번째 Record 호출
	before := time.Now().UnixNano()
	pc.Record()
	after := time.Now().UnixNano()

	if got := pc.messages.Load(); got != 1 {
		t.Errorf("Record 1회 후 messages = %d, want 1", got)
	}
	first := pc.firstSeen.Load()
	if first < before || first > after {
		t.Errorf("firstSeen = %d, want [%d, %d] 범위", first, before, after)
	}
	last := pc.lastSeen.Load()
	if last < before || last > after {
		t.Errorf("lastSeen = %d, want [%d, %d] 범위", last, before, after)
	}

	// 두 번째 Record 호출 — firstSeen 은 변하지 않아야 한다
	time.Sleep(1 * time.Millisecond)
	pc.Record()

	if got := pc.messages.Load(); got != 2 {
		t.Errorf("Record 2회 후 messages = %d, want 2", got)
	}
	if got := pc.firstSeen.Load(); got != first {
		t.Errorf("두 번째 Record 후 firstSeen이 변경됨: %d -> %d", first, got)
	}
	if got := pc.lastSeen.Load(); got <= last {
		t.Errorf("두 번째 Record 후 lastSeen이 증가하지 않음: %d <= %d", got, last)
	}
}

// TestPortCounterSnapshot 은 Snapshot 이 올바른 통계를 반환하는지 검증한다.
func TestPortCounterSnapshot(t *testing.T) {
	pc := &portCounter{}

	// 메시지 없을 때 스냅샷
	snap := pc.Snapshot()
	if snap.Messages != 0 {
		t.Errorf("빈 상태 Messages = %d, want 0", snap.Messages)
	}
	if snap.Throughput != 0 {
		t.Errorf("빈 상태 Throughput = %f, want 0", snap.Throughput)
	}
	if snap.ActiveFor != 0 {
		t.Errorf("빈 상태 ActiveFor = %v, want 0", snap.ActiveFor)
	}

	// 여러 메시지 기록 후 스냅샷
	pc.Record()
	time.Sleep(10 * time.Millisecond) // 경과 시간을 만들기 위해
	pc.Record()
	pc.Record()

	snap = pc.Snapshot()
	if snap.Messages != 3 {
		t.Errorf("3회 Record 후 Messages = %d, want 3", snap.Messages)
	}
	if snap.ActiveFor <= 0 {
		t.Errorf("ActiveFor = %v, want > 0", snap.ActiveFor)
	}
	// Throughput: firstSeen~lastSeen 사이 경과 > 0 이므로 양수여야 한다
	if snap.Throughput <= 0 {
		t.Errorf("Throughput = %f, want > 0", snap.Throughput)
	}
}

// TestPortCounterConcurrency 는 동시 접근 안전성을 검증한다.
func TestPortCounterConcurrency(t *testing.T) {
	pc := &portCounter{}
	const goroutines = 10
	const recordsPerGoroutine = 100

	done := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < recordsPerGoroutine; j++ {
				pc.Record()
			}
			done <- struct{}{}
		}()
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}

	want := int64(goroutines * recordsPerGoroutine)
	if got := pc.messages.Load(); got != want {
		t.Errorf("동시 Record 후 messages = %d, want %d", got, want)
	}

	snap := pc.Snapshot()
	if snap.Messages != want {
		t.Errorf("동시 Record 후 Snapshot.Messages = %d, want %d", snap.Messages, want)
	}
}

// TestNodeCounterWithPortCounters 는 nodeCounter 에 portCounters 맵이
// 올바르게 초기화되고 사용되는지 검증한다.
func TestNodeCounterWithPortCounters(t *testing.T) {
	nc := &nodeCounter{
		portCounters: map[string]*portCounter{
			"in":    {},
			"out":   {},
			"error": {},
		},
	}

	// "in" 포트에 기록
	nc.portCounters["in"].Record()
	nc.portCounters["in"].Record()

	// "out" 포트에 기록
	nc.portCounters["out"].Record()

	// 각 포트 카운터 검증
	if got := nc.portCounters["in"].messages.Load(); got != 2 {
		t.Errorf("in 포트 messages = %d, want 2", got)
	}
	if got := nc.portCounters["out"].messages.Load(); got != 1 {
		t.Errorf("out 포트 messages = %d, want 1", got)
	}
	if got := nc.portCounters["error"].messages.Load(); got != 0 {
		t.Errorf("error 포트 messages = %d, want 0", got)
	}
}

// TestPortStatsSnapshotFields 는 PortStatsSnapshot 의 필드가 올바르게 설정되는지 검증한다.
func TestPortStatsSnapshotFields(t *testing.T) {
	pc := &portCounter{}

	pc.Record()
	// firstSeen 과 lastSeen 이 같은 경우 elapsed == 0 이므로 throughput 은 0
	snap := pc.Snapshot()
	if snap.Messages != 1 {
		t.Errorf("Messages = %d, want 1", snap.Messages)
	}
	// elapsed == 0 (firstSeen == lastSeen) 이면 Throughput == 0
	if snap.Throughput != 0 {
		t.Errorf("단일 Record 시 Throughput = %f, want 0 (elapsed == 0)", snap.Throughput)
	}
	if snap.ActiveFor <= 0 {
		t.Errorf("ActiveFor = %v, want > 0", snap.ActiveFor)
	}
}
