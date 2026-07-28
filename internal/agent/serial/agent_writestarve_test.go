package serial

// 이 파일은 write starvation 결함에 대한 회귀 테스트를 담는다.
//
// 결함: readLoop 가 reader.Read() 동안 portIOMu 를 통째로 점유하면, length_prefix/frame
// 등의 프레이밍에서 완전한 프레임이 조립될 때까지 여러 하위 read 를 루프하는 동안
// 락이 계속 점유된다. 장비가 유휴/노이즈만 보내 프레임이 완성 안 되면, readLoop 는
// 락을 프레임 조립 내내 점유하고, 그 사이 Process(Write)는 락을 얻지 못해
// 상위 write_timeout(예: 5초)에 걸려 starve 된다.
//
// 수정: portIOReader 가 각 하위 read 마다 per-sub-read 로 락을 획득/해제하므로,
// 프레임 조립 루프 중간에 락이 자유로워져서 Process(Write)가 SetReadTimeout 으로
// bounded 된 타임아웃 범위 내에 진행된다.
//
// 모든 동시성 테스트는 `go test -race` 하에서 실행되어야 한다.

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- blockingFramePort: 완전한 프레임이 조립되지 않아 여러 하위 read 를 루프하는 포트 ---

// blockingFramePort 는 framer 가 여러 하위 read 를 루프하는 동안
// Process 의 Write 가 starve 되는지 측정한다. 각 Read 는 1바이트만 반환하고,
// 호출 횟수를 세어 framer 가 실제로 루프하는지 확인한다.
//
// 전체 프레임이 조립되지 않으면 framer.Read 는 EOF 를 반환해 readLoop 에서
// timeout 으로 처리되고 다시 시도하게 된다.
type blockingFramePort struct {
	// 매 Read 마다 1바이트씩 반환 (framer 가 여러 하위 read 를 루프하도록 유도)
	data       atomic.Int64 // 송신할 다음 바이트 (0, 1, 2, ...)
	readCount  atomic.Int32 // Read 호출 횟수 (동시성 측정용)
	writeCount atomic.Int32 // Write 호출 횟수 (동시성 측정용)
	closed     atomic.Bool
	mu         sync.Mutex
}

func newBlockingFramePort() *blockingFramePort {
	return &blockingFramePort{}
}

func (m *blockingFramePort) Read(p []byte) (int, error) {
	if m.closed.Load() {
		return 0, io.EOF
	}
	m.readCount.Add(1)
	// 1바이트씩만 반환하여 framer 가 여러 하위 read 를 루프하도록 유도한다.
	// 양수 반환이 프레임 완성을 의미하지 않으므로 framer 는 계속 Read 를 호출한다.
	if len(p) > 0 {
		p[0] = byte(m.data.Add(1))
		return 1, nil
	}
	return 0, nil
}

func (m *blockingFramePort) Write(p []byte) (int, error) {
	if m.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	m.writeCount.Add(1)
	return len(p), nil
}

func (m *blockingFramePort) Close() error {
	m.closed.Store(true)
	return nil
}

func (m *blockingFramePort) SetReadTimeout(_ time.Duration) error {
	return nil
}

// --- 테스트: Write starvation 없음 ---

// TestSerialAgent_WriteNotStarved 는 half_duplex=true 일 때
// readLoop 가 여러 하위 read 를 루프하는 동안에도 Process(Write)가
// 합리적인 시간(< 200ms) 내에 완료됨을 검증한다.
//
// 수정 전: readLoop 가 a.portIOLock 을 통째로 점유 → Process 는 락을 얻지 못함
//
//	→ write 가 SetReadTimeout 에 종속된 시간 이상 starvation.
//
// 수정 후: portIOReader 가 per-sub-read 로 락 획득/해제 → Process 는 각 sub-read
//
//	마다 락을 얻을 기회가 있음 → write 는 bounded time 내에 완료.
//
// `go test -race` 하에서 실행되어야 한다.
func TestSerialAgent_WriteNotStarved(t *testing.T) {
	cfg := makeSerialConfig()
	// half_duplex=true 를 명시 (기본값이지만 명확성 위해)
	cfg.Transport.Options["half_duplex"] = true
	a, err := NewSerialAgent(cfg)
	require.NoError(t, err)

	sa := a.(*SerialAgent)
	mock := newBlockingFramePort()

	// Start 를 우회하고 직접 포트를 주입한다 (blockingFramePort 는 goserial.Port 를
	// 만족하지 않으므로).
	sa.mu.Lock()
	sa.port = mock
	sa.mu.Unlock()
	sa.connected.Store(true)
	sa.reader = NewSerialConnReader(sa.framer, mock)

	sa.wg.Add(1)
	go sa.readLoop()

	defer func() {
		_ = sa.Stop(context.Background())
	}()

	// readLoop 가 여러 하위 read 를 루프하는 동안 Process(Write)를 호출한다.
	// 락을 per-sub-read 로 관리하면, Process 는 비교적 빠르게 완료된다.
	//
	// 수정 전 (락을 통째로 점유):
	//   - readLoop 가 SetReadTimeout(예: 200ms) 마다 한 번씩 framer.Read 를 전체
	//     호출하면서 framer 내부 여러 hunder sub-read 를 루프하는 동안
	//     portIOMu 를 점유
	//   - Process 의 Write 는 락을 기다리며 starvation
	//   - 결과: write_timeout(5s) 정도까지 대기 (FAIL)
	//
	// 수정 후 (per-sub-read 락):
	//   - readLoop 의 각 sub-read 는 락을 아주 짧은 시간 (sub-read syscall 시간)
	//     동안만 점유
	//   - Process 의 Write 는 SetReadTimeout(200ms) 내 충분히 락을 얻을 수 있음
	//   - 결과: write_timeout 과 무관하게 빠르게 완료 (PASS)
	writeCompletedWithin := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		_, err := sa.Process([]byte("test-write"))
		elapsed := time.Since(start)
		if err == nil || err.Error() == "serial agent: write failed: EOF" {
			// 정상 또는 EOF (Stop 호출로 인한 종료) — 어느 쪽이든 starvation 이 없다는
			// 증거. framer 의 프레임 조립이 실패하거나 포트 닫힘으로 빠르게 반환됨.
			writeCompletedWithin <- elapsed
		} else {
			// 예상 밖의 오류 → 실패
			t.Errorf("unexpected error: %v", err)
		}
	}()

	// write 가 200ms 이내에 완료되기를 기대한다.
	// (SetReadTimeout 의 기본값 200ms 이상 오래 대기하면 안 됨)
	select {
	case elapsed := <-writeCompletedWithin:
		assert.Less(t, elapsed, 1*time.Second,
			"Process(Write)가 1초 이상 starvation 되었다. "+
				"readLoop 가 portIOMu 를 프레임 조립 내내 점유했을 가능성. "+
				"per-sub-read 락을 적용했는지 확인하세요. "+
				"(실제 소요 시간=%v)", elapsed)
		t.Logf("Process(Write)가 정상 완료: %v", elapsed)
	case <-time.After(10 * time.Second):
		t.Fatal("Process(Write)가 10초 이상 반응 없음 — deadlock 발생 가능성")
	}

	// readLoop 의 Read 호출 횟수를 확인한다.
	// per-sub-read 로 락을 관리하려면 Read 가 여러 번 호출되어야 한다.
	// (정확한 횟수는 framer 와 SetReadTimeout 에 따라 다르지만,
	//  1회 미만은 framer 가 실제로 루프하지 않았다는 신호)
	reads := mock.readCount.Load()
	t.Logf("readLoop Read() 호출 횟수: %d", reads)
	assert.Greater(t, reads, int32(1),
		"framer 가 최소 2회 이상 Read 를 호출해야 함 (프레임 미완성 상황 모사)")
}
