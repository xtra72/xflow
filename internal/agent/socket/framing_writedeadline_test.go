package socket

import (
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 회귀 테스트: framer conn.Write 쓰기 데드라인
//
// 근본 결함: framer 의 Write 가 conn.Write 에 데드라인을 걸지 않아, stale 클라이언트가
// 수신을 멈춰 커널 송신버퍼가 포화되면 conn.Write 가 무한 블록된다. 이로 인해 상위
// (processSend/Process/TCPOutNode)와 flow(runNode)가 정지하고, StopFlow 가 30초
// 상한을 소진한다("flow already deployed" 충돌).
//
// 수정: WriteTimeout(기본 5s) 을 각 framer 의 conn.Write 직전에 SetWriteDeadline 으로
// 적용한다. 데드라인 초과 시 timeout 에러가 반환되어 정상적으로 상위로 전파된다.
//
// net.Pipe 는 동기 in-memory conn 으로, 상대편이 Read 하지 않으면 Write 가 블록되므로
// "미수신 소비자" 를 정확히 재현한다. net.Pipe conn 은 SetWriteDeadline 을 지원한다.
// ---------------------------------------------------------------------------

// writeResult 는 goroutine 에서 실행된 framer.Write 의 결과를 전달한다.
type writeResult struct {
	err error
}

// runFramerWrite 는 framer.Write 를 goroutine 으로 실행하고 결과 채널을 반환한다.
func runFramerWrite(f Framer, conn net.Conn, data []byte) <-chan writeResult {
	ch := make(chan writeResult, 1)
	go func() {
		ch <- writeResult{err: f.Write(conn, data)}
	}()
	return ch
}

// assertTimeoutError 는 err 가 write 데드라인 초과(timeout) 에러인지 확인한다.
func assertTimeoutError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err, "미수신 소비자 상황에서 write 는 데드라인 초과 에러를 반환해야 한다")
	var netErr net.Error
	isTimeout := (errors.As(err, &netErr) && netErr.Timeout()) || errors.Is(err, os.ErrDeadlineExceeded)
	require.True(t, isTimeout, "에러는 write 데드라인 초과(timeout)여야 한다: %v", err)
}

// TestFramerWrite_DeadlineUnblocksStalledConsumer 는 미수신 소비자(net.Pipe 상대편
// 미Read) 상황에서 각 framer 의 Write 가 무한 블록하지 않고 데드라인 초과 에러를
// 반환/전파하는지 검증한다 (raw / newline / length_prefix / fixed_size 전부).
func TestFramerWrite_DeadlineUnblocksStalledConsumer(t *testing.T) {
	cases := []struct {
		name    string
		framing string
		opts    FramerOptions
	}{
		{"raw", FramingRaw, FramerOptions{BufferSize: 64, WriteTimeout: 50 * time.Millisecond}},
		{"newline", FramingNewline, FramerOptions{BufferSize: 64, WriteTimeout: 50 * time.Millisecond}},
		{"length_prefix", FramingLengthPrefix, FramerOptions{WriteTimeout: 50 * time.Millisecond}},
		{"fixed_size", FramingFixedSize, FramerOptions{FixedSize: 8, WriteTimeout: 50 * time.Millisecond}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := NewFramer(tc.framing, tc.opts)
			require.NoError(t, err)

			// net.Pipe: 상대편(c2)을 절대 Read 하지 않아 커널 송신버퍼 포화를 재현한다.
			c1, c2 := net.Pipe()
			defer c1.Close()
			defer c2.Close()

			ch := runFramerWrite(f, c1, []byte("payload-bytes"))

			select {
			case res := <-ch:
				assertTimeoutError(t, res.err)
			case <-time.After(2 * time.Second):
				t.Fatal("framer.Write 가 데드라인 내에 반환하지 않음 — 무한 블록 (버그 미수정)")
			}
		})
	}
}

// TestFramerWrite_NoDeadline_Blocks 는 WriteTimeout=0(데드라인 미설정, 기존 동작)일 때
// 미수신 소비자 상황에서 Write 가 블록됨을 특성화한다 (버그 재현 — 기존 동작 보존 확인).
// 테스트 종료 시 상대편을 Read 하여 goroutine 을 정리한다.
func TestFramerWrite_NoDeadline_Blocks(t *testing.T) {
	f, err := NewFramer(FramingRaw, FramerOptions{BufferSize: 64, WriteTimeout: 0})
	require.NoError(t, err)

	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	ch := runFramerWrite(f, c1, []byte("payload"))

	select {
	case <-ch:
		t.Fatal("WriteTimeout=0 인데 미수신 소비자 상황에서 Write 가 반환됨 (예상: 블록)")
	case <-time.After(150 * time.Millisecond):
		// 예상대로 블록 중. 상대편을 Read 하여 goroutine 을 정리(leak 방지).
	}

	buf := make([]byte, 64)
	_, _ = c2.Read(buf)
	select {
	case res := <-ch:
		require.NoError(t, res.err, "소비자가 Read 하면 Write 는 정상 완료되어야 한다")
	case <-time.After(2 * time.Second):
		t.Fatal("소비자 Read 후에도 Write 가 완료되지 않음")
	}
}

// TestFramerWrite_NormalPath_Succeeds 는 소비자가 정상 수신하면 데드라인 설정 여부와
// 무관하게 Write 가 성공하는지(정상 경로 회귀 없음) 검증한다.
func TestFramerWrite_NormalPath_Succeeds(t *testing.T) {
	f, err := NewFramer(FramingRaw, FramerOptions{BufferSize: 64, WriteTimeout: 500 * time.Millisecond})
	require.NoError(t, err)

	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	// 소비자: 백그라운드에서 지속적으로 Read 하여 Write 를 언블록한다.
	go func() {
		buf := make([]byte, 64)
		_, _ = c2.Read(buf)
	}()

	ch := runFramerWrite(f, c1, []byte("hello"))
	select {
	case res := <-ch:
		require.NoError(t, res.err, "정상 소비자 상황에서 Write 는 성공해야 한다")
	case <-time.After(2 * time.Second):
		t.Fatal("정상 경로에서 Write 가 완료되지 않음")
	}
}
