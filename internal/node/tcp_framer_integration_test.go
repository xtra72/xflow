package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/message"
)

// ===========================================================================
// SPEC-NODE-003 통합 테스트: TCPInNode → FramerNode 파이프라인
// ===========================================================================

// TestIntegration_TCPServer_Framer_MultiClientFraming 은 TCP 서버 모드에서
// 두 클라이언트가 교대로 부분 데이터를 전송할 때 framer 노드가 connection_id
// 기반으로 독립 스트림 버퍼를 유지하여 각 클라이언트별 프레임을 올바르게
// 조립하는지 검증한다 (AC4.1, AC4.3).
//
// 시뮬레이션:
//   - 클라이언트 A (remoteAddr="A:1"): msg1="hel", msg3="lo\n"
//   - 클라이언트 B (remoteAddr="B:2"): msg2="wor", msg4="ld\n"
//   - 기대 결과: 프레임 "hello" (stream A), 프레임 "world" (stream B)
func TestIntegration_TCPServer_Framer_MultiClientFraming(t *testing.T) {
	t.Parallel()

	// 1. TCPInNode 를 다중 클라이언트 mock 으로 생성
	multiAgent := &mockTCPMultiClientAgent{
		messages: []connMsg{
			{data: []byte("hel"), addr: "A:1"},
			{data: []byte("wor"), addr: "B:2"},
			{data: []byte("lo\n"), addr: "A:1"},
			{data: []byte("ld\n"), addr: "B:2"},
		},
	}
	tcpIn := newTestTCPInNode(multiAgent)
	go tcpIn.receiveLoop()
	defer close(tcpIn.stopCh)

	// 2. FramerNode 생성 (framing=newline, stream_key_metadata=connection_id 기본값)
	fn := newFramerNodeForTest(t, "framer-tcp-integration", "newline")
	require.NoError(t, fn.Init(context.Background()))

	// 3. TCPInNode → FramerNode 파이프라인 실행
	var allFrames []message.Message

	for i := 0; i < 4; i++ {
		select {
		case msg := <-tcpIn.sourceCh:
			out, errs := processExpectOK(t, fn, msg)
			assert.Len(t, errs, 0, "에러 메시지가 없어야 한다")
			allFrames = append(allFrames, out...)
		case <-time.After(3 * time.Second):
			t.Fatalf("TCPInNode 메시지 %d 수신 타임아웃", i)
		}
	}

	// 4. 결과 검증: 정확히 2개 프레임이 생성되어야 한다
	require.Len(t, allFrames, 2, "두 개의 완성 프레임이 있어야 한다")

	// 프레임 내용 확인
	raw0, _ := allFrames[0].Payload().Get("raw")
	raw1, _ := allFrames[1].Payload().Get("raw")
	assert.Equal(t, []byte("hello"), raw0.([]byte))
	assert.Equal(t, []byte("world"), raw1.([]byte))

	// frame.stream_key 확인
	sk0, _ := allFrames[0].Metadata().Get("frame.stream_key")
	sk1, _ := allFrames[1].Metadata().Get("frame.stream_key")
	assert.Equal(t, "A:1", sk0)
	assert.Equal(t, "B:2", sk1)

	// frame.index 확인 (각 스트림 독립적으로 0부터 시작)
	idx0, _ := allFrames[0].Metadata().Get("frame.index")
	idx1, _ := allFrames[1].Metadata().Get("frame.index")
	assert.Equal(t, "0", idx0)
	assert.Equal(t, "0", idx1)
}

// TestIntegration_TCPServer_Framer_IndependentFrameIndex 는 각 클라이언트의
// frame.index 가 독립적으로 증가하는지 검증한다 (AC4.2).
func TestIntegration_TCPServer_Framer_IndependentFrameIndex(t *testing.T) {
	t.Parallel()

	multiAgent := &mockTCPMultiClientAgent{
		messages: []connMsg{
			{data: []byte("a1\na2\n"), addr: "A:1"},
			{data: []byte("b1\n"), addr: "B:2"},
		},
	}
	tcpIn := newTestTCPInNode(multiAgent)
	go tcpIn.receiveLoop()
	defer close(tcpIn.stopCh)

	fn := newFramerNodeForTest(t, "framer-tcp-idx", "newline")
	require.NoError(t, fn.Init(context.Background()))

	var allFrames []message.Message
	for i := 0; i < 2; i++ {
		select {
		case msg := <-tcpIn.sourceCh:
			out, errs := processExpectOK(t, fn, msg)
			assert.Len(t, errs, 0)
			allFrames = append(allFrames, out...)
		case <-time.After(3 * time.Second):
			t.Fatalf("메시지 %d 수신 타임아웃", i)
		}
	}

	// 클라이언트 A: 2 프레임, 클라이언트 B: 1 프레임
	require.Len(t, allFrames, 3)

	// 클라이언트 A 의 프레임들
	idx0, _ := allFrames[0].Metadata().Get("frame.index")
	idx1, _ := allFrames[1].Metadata().Get("frame.index")
	assert.Equal(t, "0", idx0, "stream A frame.index[0]")
	assert.Equal(t, "1", idx1, "stream A frame.index[1]")

	// 클라이언트 B 의 프레임 (독립적으로 0부터)
	idx2, _ := allFrames[2].Metadata().Get("frame.index")
	assert.Equal(t, "0", idx2, "stream B frame.index[0]")
}

// TestIntegration_TCPClient_Framer_SingleStream 은 TCP 클라이언트 모드에서
// 단일 스트림으로 프레이밍되는지 검증한다 (AC4.3).
func TestIntegration_TCPClient_Framer_SingleStream(t *testing.T) {
	t.Parallel()

	clientAgent := &mockTCPClientAgent{
		receiveData: []byte("hello\nworld\n"),
	}
	tcpIn := newTestTCPInNode(clientAgent)
	go tcpIn.receiveLoop()
	defer close(tcpIn.stopCh)

	fn := newFramerNodeForTest(t, "framer-tcp-client", "newline")
	require.NoError(t, fn.Init(context.Background()))

	// 1개 메시지 수신
	select {
	case msg := <-tcpIn.sourceCh:
		out, errs := processExpectOK(t, fn, msg)
		assert.Len(t, errs, 0)
		require.Len(t, out, 2, "2개 프레임이 생성되어야 한다")

		raw0, _ := out[0].Payload().Get("raw")
		raw1, _ := out[1].Payload().Get("raw")
		assert.Equal(t, []byte("hello"), raw0.([]byte))
		assert.Equal(t, []byte("world"), raw1.([]byte))

		// frame.stream_key 는 노드 ID 와 동일해야 한다
		sk0, _ := out[0].Metadata().Get("frame.stream_key")
		sk1, _ := out[1].Metadata().Get("frame.stream_key")
		assert.Equal(t, tcpIn.ID(), sk0)
		assert.Equal(t, tcpIn.ID(), sk1)
	case <-time.After(3 * time.Second):
		t.Fatal("메시지 수신 타임아웃")
	}
}

// TestIntegration_TCPServer_Framer_NoExtraConfig 은 framer 노드에 별도
// stream_key_metadata 설정 없이 (기본값 "connection_id") TCPInNode 의
// connection_id 와 자동 연동되는지 검증한다 (AC4.4).
func TestIntegration_TCPServer_Framer_NoExtraConfig(t *testing.T) {
	t.Parallel()

	multiAgent := &mockTCPMultiClientAgent{
		messages: []connMsg{
			{data: []byte("test\n"), addr: "192.168.1.1:1234"},
		},
	}
	tcpIn := newTestTCPInNode(multiAgent)
	go tcpIn.receiveLoop()
	defer close(tcpIn.stopCh)

	// stream_key_metadata 를 명시하지 않은 기본 framer 노드
	fn := newFramerNodeForTest(t, "framer-tcp-noconfig", "newline")
	require.NoError(t, fn.Init(context.Background()))

	// 기본값이 "connection_id" 인지 확인
	assert.Equal(t, "connection_id", fn.nodeOptions.StreamKeyMetadata)

	select {
	case msg := <-tcpIn.sourceCh:
		out, errs := processExpectOK(t, fn, msg)
		assert.Len(t, errs, 0)
		require.Len(t, out, 1)

		sk, _ := out[0].Metadata().Get("frame.stream_key")
		assert.Equal(t, "192.168.1.1:1234", sk, "추가 설정 없이 connection_id 기반 스트림 키가 사용되어야 한다")
	case <-time.After(3 * time.Second):
		t.Fatal("메시지 수신 타임아웃")
	}
}
