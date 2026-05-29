package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/framing"
	"github.com/xtra/xflow/pkg/message"
)

// --- FramerNode 인터페이스 준수 ---

var _ Node = (*FramerNode)(nil)

// framerTestDef 는 framer 노드 테스트용 기본 NodeDef 를 생성한다.
func framerTestDef(name string, framingMode string, extraCfg ...flow.NodeOption) flow.NodeDef {
	opts := []flow.NodeOption{
		flow.WithNodeConfig("framing", framingMode),
		flow.WithErrorPort(),
	}
	opts = append(opts, extraCfg...)
	return flow.NewNodeDef(name, "framer", opts...)
}

// newFramerNodeForTest 는 주어진 옵션으로 framer 노드를 생성하고 Configure 를
// 호출한 상태로 반환한다. 실패 시 테스트를 중단한다.
func newFramerNodeForTest(t *testing.T, name string, framingMode string, opts ...framerOption) *FramerNode {
	t.Helper()
	def := framerTestDef(name, framingMode)
	n, err := NewFramerNode(def)
	require.NoError(t, err)
	fn, ok := n.(*FramerNode)
	require.True(t, ok)
	for _, o := range opts {
		o(fn)
	}
	return fn
}

// framerOption 은 테스트용 framer 노드 조립 helper 이다.
type framerOption func(*FramerNode)

func withFramingOptions(mode string, opts framing.Options) framerOption {
	return func(n *FramerNode) {
		// Phase 2: per-stream framer 를 생성하기 위해 mode + options 만 저장한다.
		// 각 stream 은 Process 호출 시 framing.New 로 자신의 인스턴스를 만든다.
		if _, err := framing.New(mode, opts); err != nil {
			panic(err)
		}
		n.framingMode = mode
		n.options = opts
	}
}

// withNodeOptions 는 framerNodeOptions (stream_key_metadata, max_streams,
// stream_idle_timeout) 을 테스트에서 주입하기 위한 helper 이다.
func withNodeOptions(no framerNodeOptions) framerOption {
	return func(n *FramerNode) {
		n.nodeOptions = no
	}
}

// withIncompleteSink 는 Shutdown 시의 미완성 버퍼 보고를 테스트에서 수집하기
// 위한 sink 를 주입한다. 운영 환경에서는 sink 가 nil 이며 slog.Warn 만 기록된다.
func withIncompleteSink(sink func(report incompleteStreamReport)) framerOption {
	return func(n *FramerNode) {
		n.incompleteSink = sink
	}
}

// splitResults 는 Process 결과를 out 포트 / error 포트 메시지로 분리한다.
// error 포트 마커 키: "frame.port" = "error".
func splitResults(results []message.Message) (outMsgs, errMsgs []message.Message) {
	for _, m := range results {
		if v, ok := m.Metadata().Get("frame.port"); ok && v == "error" {
			errMsgs = append(errMsgs, m)
			continue
		}
		outMsgs = append(outMsgs, m)
	}
	return
}

// inputMsgWithRaw 는 raw 키에 주어진 바이트를 담은 Message 를 생성한다.
func inputMsgWithRaw(raw []byte, metaKV ...string) message.Message {
	var opts []message.Option
	for i := 0; i+1 < len(metaKV); i += 2 {
		opts = append(opts, message.WithMetadata(metaKV[i], metaKV[i+1]))
	}
	msg := message.New(opts...)
	msg.Payload().Set("raw", raw)
	return msg
}

// --- FramerNode 기본 생성 / 라이프사이클 ---

func TestFramerNode_NewFramerNode_ValidNewline(t *testing.T) {
	t.Parallel()
	def := framerTestDef("framer-1", "newline")
	n, err := NewFramerNode(def)
	require.NoError(t, err)
	require.NotNil(t, n)
	assert.Equal(t, "framer-1", n.Name())
	assert.Equal(t, "framer", n.Type())
}

func TestFramerNode_Lifecycle_StartStop(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-lc", "newline")
	ctx := context.Background()
	require.NoError(t, fn.Init(ctx))
	_, err := fn.Process(ctx, inputMsgWithRaw([]byte{}))
	require.NoError(t, err)
	require.NoError(t, fn.Shutdown(ctx))
	// Stop idempotent: 두 번째 호출은 에러를 내더라도 panic 이 아니어야 함
	_ = fn.Shutdown(ctx)
}

// --- Process: 정상 프레임 ---

func TestFramerNode_Process_SingleFrame_NewlineMode(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-newline-single", "newline")
	require.NoError(t, fn.Init(context.Background()))
	results, err := fn.Process(context.Background(), inputMsgWithRaw([]byte("hello\n")))
	require.NoError(t, err)
	outMsgs, errMsgs := splitResults(results)
	require.Len(t, outMsgs, 1)
	require.Len(t, errMsgs, 0)
	raw, ok := outMsgs[0].Payload().Get("raw")
	require.True(t, ok)
	assert.Equal(t, []byte("hello"), raw.([]byte))
	// hex 변환도 확인
	data, ok := outMsgs[0].Payload().Get("data")
	require.True(t, ok)
	assert.Equal(t, hex.EncodeToString([]byte("hello")), data.(string))
}

func TestFramerNode_Process_MultipleFrames_NewlineMode(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-newline-multi", "newline")
	require.NoError(t, fn.Init(context.Background()))
	results, err := fn.Process(context.Background(), inputMsgWithRaw([]byte("a\nbb\nccc\n")))
	require.NoError(t, err)
	outMsgs, _ := splitResults(results)
	require.Len(t, outMsgs, 3)
	want := []string{"a", "bb", "ccc"}
	for i, w := range want {
		raw, _ := outMsgs[i].Payload().Get("raw")
		assert.Equal(t, []byte(w), raw.([]byte))
	}
	// frame.index 메타가 0,1,2 순서여야 함
	for i := range outMsgs {
		idx, ok := outMsgs[i].Metadata().Get("frame.index")
		require.True(t, ok)
		assert.Equal(t, []string{"0", "1", "2"}[i], idx)
	}
}

func TestFramerNode_Process_PartialFrame_BuffersRemainder(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-partial", "newline")
	require.NoError(t, fn.Init(context.Background()))
	// 첫 호출: "a\nbb" → 1 프레임("a") + remainder "bb"
	results1, err := fn.Process(context.Background(), inputMsgWithRaw([]byte("a\nbb")))
	require.NoError(t, err)
	out1, _ := splitResults(results1)
	require.Len(t, out1, 1)
	// 두 번째 호출: "\nccc\n" → 2 프레임("bb", "ccc")
	results2, err := fn.Process(context.Background(), inputMsgWithRaw([]byte("\nccc\n")))
	require.NoError(t, err)
	out2, _ := splitResults(results2)
	require.Len(t, out2, 2)
	raw0, _ := out2[0].Payload().Get("raw")
	raw1, _ := out2[1].Payload().Get("raw")
	assert.Equal(t, []byte("bb"), raw0.([]byte))
	assert.Equal(t, []byte("ccc"), raw1.([]byte))
	// frame.index 는 누적: 두 번째 호출의 첫 프레임이 1 이어야 한다
	idx, _ := out2[0].Metadata().Get("frame.index")
	assert.Equal(t, "1", idx)
	idx2, _ := out2[1].Metadata().Get("frame.index")
	assert.Equal(t, "2", idx2)
}

func TestFramerNode_Process_FrameMode_Icp02Sample(t *testing.T) {
	t.Parallel()
	icp02Opts := framing.Options{
		STX:                  []byte{0x56},
		LengthOffset:         1,
		LengthSize:           1,
		LengthEndian:         "big",
		LengthIncludesHeader: true,
		LengthAdjustment:     0,
		Checksum:             "none",
		MaxMessageSize:       256,
	}
	raw, _ := hex.DecodeString("561404ffffffff04445500000604000102a1b7ed")

	fn := newFramerNodeForTest(t, "framer-lg_icp02", "frame", withFramingOptions("frame", icp02Opts))
	require.NoError(t, fn.Init(context.Background()))

	results, err := fn.Process(context.Background(), inputMsgWithRaw(raw))
	require.NoError(t, err)
	outMsgs, _ := splitResults(results)
	require.Len(t, outMsgs, 1)
	gotRaw, _ := outMsgs[0].Payload().Get("raw")
	assert.Equal(t, raw, gotRaw.([]byte))
	ft, _ := outMsgs[0].Metadata().Get("frame.framer_type")
	assert.Equal(t, "frame", ft)
}

func TestFramerNode_Process_LengthPrefix_BigEndian(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-lp", "length_prefix")
	require.NoError(t, fn.Init(context.Background()))

	var buf bytes.Buffer
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, 5)
	buf.Write(header)
	buf.Write([]byte("hello"))

	results, err := fn.Process(context.Background(), inputMsgWithRaw(buf.Bytes()))
	require.NoError(t, err)
	outMsgs, _ := splitResults(results)
	require.Len(t, outMsgs, 1)
	gotRaw, _ := outMsgs[0].Payload().Get("raw")
	assert.Equal(t, []byte("hello"), gotRaw.([]byte))
}

func TestFramerNode_Process_RawMode(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-raw", "raw")
	require.NoError(t, fn.Init(context.Background()))
	results, err := fn.Process(context.Background(), inputMsgWithRaw([]byte("raw bytes")))
	require.NoError(t, err)
	outMsgs, _ := splitResults(results)
	require.Len(t, outMsgs, 1)
	gotRaw, _ := outMsgs[0].Payload().Get("raw")
	assert.Equal(t, []byte("raw bytes"), gotRaw.([]byte))
}

func TestFramerNode_Process_StreamMode_Passthrough(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-stream", "stream")
	require.NoError(t, fn.Init(context.Background()))
	results, err := fn.Process(context.Background(), inputMsgWithRaw([]byte("chunk1")))
	require.NoError(t, err)
	outMsgs, _ := splitResults(results)
	require.Len(t, outMsgs, 1)
	gotRaw, _ := outMsgs[0].Payload().Get("raw")
	assert.Equal(t, []byte("chunk1"), gotRaw.([]byte))
}

func TestFramerNode_Process_FixedSizeMode(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-fix", "fixed_size",
		withFramingOptions("fixed_size", framing.Options{FixedSize: 3}))
	require.NoError(t, fn.Init(context.Background()))
	results, err := fn.Process(context.Background(), inputMsgWithRaw([]byte("abcdefghij")))
	require.NoError(t, err)
	outMsgs, _ := splitResults(results)
	require.Len(t, outMsgs, 3)
	want := []string{"abc", "def", "ghi"}
	for i, w := range want {
		raw, _ := outMsgs[i].Payload().Get("raw")
		assert.Equal(t, []byte(w), raw.([]byte))
	}
}

// --- Process: 입력 변종 ---

func TestFramerNode_Process_EmptyBytes_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-empty", "newline")
	require.NoError(t, fn.Init(context.Background()))
	results, err := fn.Process(context.Background(), inputMsgWithRaw([]byte{}))
	require.NoError(t, err)
	assert.Len(t, results, 0)
}

func TestFramerNode_Process_DataKeyHexFallback(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-hex", "newline")
	require.NoError(t, fn.Init(context.Background()))
	msg := message.New()
	// "hello\n" == 68 65 6c 6c 6f 0a
	msg.Payload().Set("data", "68656c6c6f0a")
	results, err := fn.Process(context.Background(), msg)
	require.NoError(t, err)
	outMsgs, _ := splitResults(results)
	require.Len(t, outMsgs, 1)
	gotRaw, _ := outMsgs[0].Payload().Get("raw")
	assert.Equal(t, []byte("hello"), gotRaw.([]byte))
}

func TestFramerNode_Process_InvalidPayload_RoutesToError(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-invalid", "newline")
	require.NoError(t, fn.Init(context.Background()))
	msg := message.New() // raw, data 모두 없음
	results, err := fn.Process(context.Background(), msg)
	// 에러는 error 포트로 전달되므로 Process 자체는 nil 에러를 반환
	require.NoError(t, err)
	outMsgs, errMsgs := splitResults(results)
	assert.Len(t, outMsgs, 0)
	require.Len(t, errMsgs, 1)
	code, ok := errMsgs[0].Metadata().Get("frame.error.code")
	require.True(t, ok)
	assert.Equal(t, "frame.input.invalid_payload", code)
}

// --- Process: 메타데이터 보존 및 설정 ---

func TestFramerNode_Process_PreservesMetadata(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-meta", "newline")
	require.NoError(t, fn.Init(context.Background()))
	msg := inputMsgWithRaw([]byte("hello\n"),
		"source_addr", "192.168.1.10",
		"timestamp", "2026-04-10T10:00:00Z",
		"connection_id", "conn-42",
	)
	results, err := fn.Process(context.Background(), msg)
	require.NoError(t, err)
	outMsgs, _ := splitResults(results)
	require.Len(t, outMsgs, 1)
	for _, key := range []string{"source_addr", "timestamp", "connection_id"} {
		v, ok := outMsgs[0].Metadata().Get(key)
		require.True(t, ok, "메타데이터 키 %s 가 보존되어야 함", key)
		assert.NotEmpty(t, v)
	}
}

// --- Process: 파싱 에러 ---

func TestFramerNode_Process_ETXMismatch_RoutesToError(t *testing.T) {
	t.Parallel()
	etxOpts := framing.Options{
		STX:          []byte{0x02},
		ETX:          []byte{0x03},
		LengthOffset: 1,
		LengthSize:   1,
		Checksum:     "none",
	}
	fn := newFramerNodeForTest(t, "framer-etx", "frame",
		withFramingOptions("frame", etxOpts))
	require.NoError(t, fn.Init(context.Background()))

	// ETX 위치에 0xFF (잘못된 ETX)
	input := []byte{0x02, 0x02, 'a', 0xFF}
	results, err := fn.Process(context.Background(), inputMsgWithRaw(input))
	require.NoError(t, err)
	_, errMsgs := splitResults(results)
	require.GreaterOrEqual(t, len(errMsgs), 1)
	code, ok := errMsgs[0].Metadata().Get("frame.error.code")
	require.True(t, ok)
	assert.Equal(t, "frame.parse.etx_mismatch", code)
}

func TestFramerNode_Process_MaxSizeExceeded_RoutesToError(t *testing.T) {
	t.Parallel()
	// length_prefix 에서 길이 필드가 max_message_size 를 초과
	lpOpts := framing.Options{MaxMessageSize: 10}
	fn := newFramerNodeForTest(t, "framer-lpmax", "length_prefix",
		withFramingOptions("length_prefix", lpOpts))
	require.NoError(t, fn.Init(context.Background()))

	var buf bytes.Buffer
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, 100) // max 10 초과
	buf.Write(header)

	results, err := fn.Process(context.Background(), inputMsgWithRaw(buf.Bytes()))
	require.NoError(t, err)
	_, errMsgs := splitResults(results)
	require.GreaterOrEqual(t, len(errMsgs), 1)
	code, _ := errMsgs[0].Metadata().Get("frame.error.code")
	assert.Equal(t, "frame.parse.max_size_exceeded", code)
}

func TestFramerNode_Configure_PassesThrough(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-cfg", "newline")
	err := fn.Configure(map[string]any{"arbitrary": "value"})
	require.NoError(t, err)
}

func TestFramerNode_Process_DataKeyInvalidHex_RoutesToError(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-badhex", "newline")
	require.NoError(t, fn.Init(context.Background()))
	msg := message.New()
	msg.Payload().Set("data", "not-hex-zzzz")
	results, err := fn.Process(context.Background(), msg)
	require.NoError(t, err)
	_, errMsgs := splitResults(results)
	require.Len(t, errMsgs, 1)
	code, _ := errMsgs[0].Metadata().Get("frame.error.code")
	assert.Equal(t, "frame.input.invalid_payload", code)
}

func TestFramerNode_Process_SetsFrameMetadata(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-framemeta", "newline")
	require.NoError(t, fn.Init(context.Background()))
	results, err := fn.Process(context.Background(), inputMsgWithRaw([]byte("a\nbb\n")))
	require.NoError(t, err)
	outMsgs, _ := splitResults(results)
	require.Len(t, outMsgs, 2)
	for i, msg := range outMsgs {
		idx, ok := msg.Metadata().Get("frame.index")
		require.True(t, ok)
		assert.Equal(t, []string{"0", "1"}[i], idx)

		ft, ok := msg.Metadata().Get("frame.framer_type")
		require.True(t, ok)
		assert.Equal(t, "newline", ft)

		sk, ok := msg.Metadata().Get("frame.stream_key")
		require.True(t, ok)
		assert.Equal(t, "", sk)
	}
}

// ---------------------------------------------------------------------------
// Phase 2: 다중 스트림 버퍼 (M4 R4.1 ~ R4.8)
// ---------------------------------------------------------------------------

// inputMsgForStream 은 stream key metadata 를 주입한 raw 메시지를 생성한다.
// keyName 이 빈 문자열이거나 value 가 빈 문자열이면 해당 메타데이터를 설정하지
// 않는다.
func inputMsgForStream(raw []byte, keyName, keyValue string) message.Message {
	if keyName == "" {
		return inputMsgWithRaw(raw)
	}
	return inputMsgWithRaw(raw, keyName, keyValue)
}

// processExpectOK 는 Process 호출 결과에서 에러가 없음을 assert 하고
// out/error 메시지를 분리해 반환한다.
func processExpectOK(t *testing.T, fn *FramerNode, msg message.Message) (outMsgs, errMsgs []message.Message) {
	t.Helper()
	results, err := fn.Process(context.Background(), msg)
	require.NoError(t, err)
	return splitResults(results)
}

// TestFramerNode_MultiStream_IndependentBuffers 는 서로 다른 connection_id 로
// 도착한 partial 프레임이 스트림별로 독립 조립됨을 검증한다 (R4.1, R4.8).
func TestFramerNode_MultiStream_IndependentBuffers(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-multi-indep", "newline")
	require.NoError(t, fn.Init(context.Background()))

	// Stream A: "foo\n" 를 두 조각으로 나눠 전송 → 1 프레임("foo")
	out, errs := processExpectOK(t, fn, inputMsgForStream([]byte("fo"), "connection_id", "A"))
	assert.Len(t, out, 0)
	assert.Len(t, errs, 0)

	// Stream B: "hi\nbye\n" 를 한 번에 전송 → 2 프레임("hi", "bye")
	out, errs = processExpectOK(t, fn, inputMsgForStream([]byte("hi\nbye\n"), "connection_id", "B"))
	require.Len(t, out, 2)
	assert.Len(t, errs, 0)
	raw0, _ := out[0].Payload().Get("raw")
	raw1, _ := out[1].Payload().Get("raw")
	assert.Equal(t, []byte("hi"), raw0.([]byte))
	assert.Equal(t, []byte("bye"), raw1.([]byte))

	// Stream A 완성
	out, errs = processExpectOK(t, fn, inputMsgForStream([]byte("o\n"), "connection_id", "A"))
	require.Len(t, out, 1)
	assert.Len(t, errs, 0)
	rawA, _ := out[0].Payload().Get("raw")
	assert.Equal(t, []byte("foo"), rawA.([]byte))
}

// TestFramerNode_MultiStream_FrameIndex_Independent 는 frame.index 가 스트림별로
// 독립적으로 증가함을 검증한다 (R4.7).
func TestFramerNode_MultiStream_FrameIndex_Independent(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-multi-idx", "newline")
	require.NoError(t, fn.Init(context.Background()))

	// Stream A: 3 프레임
	outA, _ := processExpectOK(t, fn, inputMsgForStream([]byte("a1\na2\na3\n"), "connection_id", "A"))
	require.Len(t, outA, 3)
	for i, want := range []string{"0", "1", "2"} {
		idx, _ := outA[i].Metadata().Get("frame.index")
		assert.Equal(t, want, idx, "stream A index[%d]", i)
	}

	// Stream B: 2 프레임
	outB, _ := processExpectOK(t, fn, inputMsgForStream([]byte("b1\nb2\n"), "connection_id", "B"))
	require.Len(t, outB, 2)
	for i, want := range []string{"0", "1"} {
		idx, _ := outB[i].Metadata().Get("frame.index")
		assert.Equal(t, want, idx, "stream B index[%d]", i)
	}

	// Stream A 에 다시 → 인덱스 3 부터 시작 (독립성)
	outA2, _ := processExpectOK(t, fn, inputMsgForStream([]byte("a4\n"), "connection_id", "A"))
	require.Len(t, outA2, 1)
	idx, _ := outA2[0].Metadata().Get("frame.index")
	assert.Equal(t, "3", idx)
}

// TestFramerNode_MultiStream_StreamKeyMetadata_Custom 은 stream_key_metadata
// 옵션이 "session_id" 인 경우를 검증한다. session_id 없이 connection_id 만
// 있는 메시지는 공용 버퍼 "" 로 흐른다.
func TestFramerNode_MultiStream_StreamKeyMetadata_Custom(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-multi-custom", "newline",
		withNodeOptions(framerNodeOptions{StreamKeyMetadata: "session_id"}))
	require.NoError(t, fn.Init(context.Background()))

	// connection_id 는 무시되어야 한다 → 공용 버퍼 ""
	outCommon, _ := processExpectOK(t, fn, inputMsgForStream([]byte("c1\n"), "connection_id", "X"))
	require.Len(t, outCommon, 1)
	sk, _ := outCommon[0].Metadata().Get("frame.stream_key")
	assert.Equal(t, "", sk)

	// session_id 는 분리 키로 사용
	outS1, _ := processExpectOK(t, fn, inputMsgForStream([]byte("s1\n"), "session_id", "sess-1"))
	require.Len(t, outS1, 1)
	sk1, _ := outS1[0].Metadata().Get("frame.stream_key")
	assert.Equal(t, "sess-1", sk1)

	outS2, _ := processExpectOK(t, fn, inputMsgForStream([]byte("s2\n"), "session_id", "sess-2"))
	require.Len(t, outS2, 1)
	sk2, _ := outS2[0].Metadata().Get("frame.stream_key")
	assert.Equal(t, "sess-2", sk2)

	// frame.index 검증: 공용(1 frame), sess-1(1 frame), sess-2(1 frame) 모두 0
	for _, m := range []message.Message{outCommon[0], outS1[0], outS2[0]} {
		idx, _ := m.Metadata().Get("frame.index")
		assert.Equal(t, "0", idx)
	}
}

// TestFramerNode_MultiStream_NoKeyMetadata_SingleBuffer 는 메타데이터에
// 스트림 키가 전혀 없을 때 모든 데이터가 공용 버퍼 "" 로 흐름을 검증한다.
func TestFramerNode_MultiStream_NoKeyMetadata_SingleBuffer(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-multi-none", "newline")
	require.NoError(t, fn.Init(context.Background()))

	out1, _ := processExpectOK(t, fn, inputMsgForStream([]byte("x1\n"), "", ""))
	out2, _ := processExpectOK(t, fn, inputMsgForStream([]byte("x2\n"), "", ""))
	require.Len(t, out1, 1)
	require.Len(t, out2, 1)

	// 동일 공용 버퍼에 누적 → 인덱스 0, 1
	idx1, _ := out1[0].Metadata().Get("frame.index")
	idx2, _ := out2[0].Metadata().Get("frame.index")
	assert.Equal(t, "0", idx1)
	assert.Equal(t, "1", idx2)

	// 둘 다 stream_key = ""
	sk1, _ := out1[0].Metadata().Get("frame.stream_key")
	sk2, _ := out2[0].Metadata().Get("frame.stream_key")
	assert.Equal(t, "", sk1)
	assert.Equal(t, "", sk2)
}

// TestFramerNode_MultiStream_EmptyKeyValue_SingleBuffer 는 키가 존재하지만 값이
// 빈 문자열일 때 공용 버퍼로 fall back 함을 검증한다 (R2.4 두 번째 절).
func TestFramerNode_MultiStream_EmptyKeyValue_SingleBuffer(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-multi-empty", "newline")
	require.NoError(t, fn.Init(context.Background()))

	out, _ := processExpectOK(t, fn, inputMsgForStream([]byte("e1\n"), "connection_id", ""))
	require.Len(t, out, 1)
	sk, _ := out[0].Metadata().Get("frame.stream_key")
	assert.Equal(t, "", sk)

	// 이후 빈 키 값으로 다시 → 같은 공용 버퍼에 누적 → 인덱스 1
	out2, _ := processExpectOK(t, fn, inputMsgForStream([]byte("e2\n"), "connection_id", ""))
	require.Len(t, out2, 1)
	idx, _ := out2[0].Metadata().Get("frame.index")
	assert.Equal(t, "1", idx)
}

// TestFramerNode_MultiStream_StreamKey_SetInOutputMetadata 는 frame.stream_key
// 메타데이터가 실제 사용된 키와 일치하는지 검증한다 (R2.10).
func TestFramerNode_MultiStream_StreamKey_SetInOutputMetadata(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-sk-meta", "newline")
	require.NoError(t, fn.Init(context.Background()))

	out, _ := processExpectOK(t, fn, inputMsgForStream([]byte("x\n"), "connection_id", "conn-42"))
	require.Len(t, out, 1)
	sk, _ := out[0].Metadata().Get("frame.stream_key")
	assert.Equal(t, "conn-42", sk)
}

// TestFramerNode_MaxStreams_RejectNewStream 는 max_streams 초과 시 새 스트림이
// 거부되고 기존 스트림은 영향받지 않음을 검증한다 (R4.3, R4.4, R4.8).
func TestFramerNode_MaxStreams_RejectNewStream(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-maxstreams", "newline",
		withNodeOptions(framerNodeOptions{MaxStreams: 2}))
	require.NoError(t, fn.Init(context.Background()))

	// Stream A, B 등록 → OK
	outA, errsA := processExpectOK(t, fn, inputMsgForStream([]byte("a\n"), "connection_id", "A"))
	require.Len(t, outA, 1)
	require.Len(t, errsA, 0)

	outB, errsB := processExpectOK(t, fn, inputMsgForStream([]byte("b\n"), "connection_id", "B"))
	require.Len(t, outB, 1)
	require.Len(t, errsB, 0)

	// Stream C 시도 → reject (error port)
	outC, errsC := processExpectOK(t, fn, inputMsgForStream([]byte("c\n"), "connection_id", "C"))
	assert.Len(t, outC, 0)
	require.Len(t, errsC, 1)
	code, _ := errsC[0].Metadata().Get("frame.error.code")
	assert.Equal(t, "frame.buffer.max_streams_exceeded", code)
}

// TestFramerNode_MaxStreams_ExistingStreamsContinueAfterRejection 는 거부 발생
// 이후에도 기존 스트림 A, B 가 정상 동작함을 검증한다.
func TestFramerNode_MaxStreams_ExistingStreamsContinueAfterRejection(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-maxstreams-cont", "newline",
		withNodeOptions(framerNodeOptions{MaxStreams: 2}))
	require.NoError(t, fn.Init(context.Background()))

	_, _ = processExpectOK(t, fn, inputMsgForStream([]byte("a1\n"), "connection_id", "A"))
	_, _ = processExpectOK(t, fn, inputMsgForStream([]byte("b1\n"), "connection_id", "B"))

	// 거부
	_, errsC := processExpectOK(t, fn, inputMsgForStream([]byte("c\n"), "connection_id", "C"))
	require.Len(t, errsC, 1)

	// A, B 계속 정상 처리
	outA, _ := processExpectOK(t, fn, inputMsgForStream([]byte("a2\n"), "connection_id", "A"))
	require.Len(t, outA, 1)
	rawA, _ := outA[0].Payload().Get("raw")
	assert.Equal(t, []byte("a2"), rawA.([]byte))
	idxA, _ := outA[0].Metadata().Get("frame.index")
	assert.Equal(t, "1", idxA) // A 의 두 번째 프레임

	outB, _ := processExpectOK(t, fn, inputMsgForStream([]byte("b2\n"), "connection_id", "B"))
	require.Len(t, outB, 1)
	idxB, _ := outB[0].Metadata().Get("frame.index")
	assert.Equal(t, "1", idxB)
}

// TestFramerNode_MaxStreams_Zero_Unlimited 는 max_streams=0 이 무제한을
// 의미함을 검증한다 (R1.6 기본값).
func TestFramerNode_MaxStreams_Zero_Unlimited(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-maxstreams-zero", "newline",
		withNodeOptions(framerNodeOptions{MaxStreams: 0}))
	require.NoError(t, fn.Init(context.Background()))

	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("conn-%d", i)
		out, errs := processExpectOK(t, fn,
			inputMsgForStream([]byte("x\n"), "connection_id", key))
		require.Len(t, out, 1, "stream %s", key)
		require.Len(t, errs, 0, "stream %s", key)
	}
}

// TestFramerNode_StreamIdleTimeout_LazyEviction 은 stream_idle_timeout 이
// 경과한 스트림이 다음 Process 호출에서 조용히 축출됨을 검증한다 (R4.5).
func TestFramerNode_StreamIdleTimeout_LazyEviction(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-idle", "newline",
		withNodeOptions(framerNodeOptions{StreamIdleTimeout: 80 * time.Millisecond}))
	require.NoError(t, fn.Init(context.Background()))

	// Stream A 에 부분 데이터 적재 (미완성)
	outA1, _ := processExpectOK(t, fn, inputMsgForStream([]byte("aa"), "connection_id", "A"))
	require.Len(t, outA1, 0)

	// 첫 프레임 완성 (frame.index = 0)
	outA2, _ := processExpectOK(t, fn, inputMsgForStream([]byte("\n"), "connection_id", "A"))
	require.Len(t, outA2, 1)
	idx, _ := outA2[0].Metadata().Get("frame.index")
	assert.Equal(t, "0", idx)

	// idle timeout 대기
	time.Sleep(150 * time.Millisecond)

	// Stream B 로 메시지 → 이 Process 호출에서 A 는 lazy 축출 (조용히)
	outB, errsB := processExpectOK(t, fn, inputMsgForStream([]byte("b\n"), "connection_id", "B"))
	require.Len(t, outB, 1)
	assert.Len(t, errsB, 0, "idle 축출은 error 를 발행하지 않는다")

	// Stream A 재등록 → index 가 0 부터 시작해야 함 (A 가 완전히 축출되었음을 의미)
	outA3, _ := processExpectOK(t, fn, inputMsgForStream([]byte("fresh\n"), "connection_id", "A"))
	require.Len(t, outA3, 1)
	idxA3, _ := outA3[0].Metadata().Get("frame.index")
	assert.Equal(t, "0", idxA3, "축출 이후 Stream A 의 인덱스가 리셋되어야 함")
}

// TestFramerNode_StreamIdleTimeout_Zero_Disabled 는 timeout=0 이 idle 축출을
// 비활성화함을 검증한다.
func TestFramerNode_StreamIdleTimeout_Zero_Disabled(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-idle-zero", "newline",
		withNodeOptions(framerNodeOptions{StreamIdleTimeout: 0}))
	require.NoError(t, fn.Init(context.Background()))

	outA1, _ := processExpectOK(t, fn, inputMsgForStream([]byte("a\n"), "connection_id", "A"))
	require.Len(t, outA1, 1)
	// 기간 경과 시뮬레이션 (timeout 이 없으므로 실제 sleep 대신 직접 lastActivity 조작도 가능하지만
	// 여기는 간단히 짧은 sleep 만 수행하고 그 이후에도 A 가 유지되어 인덱스가 이어짐을 확인)
	time.Sleep(30 * time.Millisecond)

	outA2, _ := processExpectOK(t, fn, inputMsgForStream([]byte("a2\n"), "connection_id", "A"))
	require.Len(t, outA2, 1)
	idx, _ := outA2[0].Metadata().Get("frame.index")
	assert.Equal(t, "1", idx, "timeout=0 이면 스트림이 유지되어 인덱스가 이어져야 한다")
}

// TestFramerNode_StreamIdleTimeout_ResetsOnActivity 는 새 activity 가 timeout
// 을 reset 함을 검증한다.
func TestFramerNode_StreamIdleTimeout_ResetsOnActivity(t *testing.T) {
	t.Parallel()
	fn := newFramerNodeForTest(t, "framer-idle-reset", "newline",
		withNodeOptions(framerNodeOptions{StreamIdleTimeout: 100 * time.Millisecond}))
	require.NoError(t, fn.Init(context.Background()))

	// t=0 : A 에 데이터
	_, _ = processExpectOK(t, fn, inputMsgForStream([]byte("a1\n"), "connection_id", "A"))
	// t=50ms : 다시 A 에 데이터 → lastActivity 갱신
	time.Sleep(50 * time.Millisecond)
	_, _ = processExpectOK(t, fn, inputMsgForStream([]byte("a2\n"), "connection_id", "A"))
	// t=130ms : (reset 이전 100ms 에서는 축출되었을 시점) 아직 유지됨
	time.Sleep(80 * time.Millisecond)
	outA, _ := processExpectOK(t, fn, inputMsgForStream([]byte("a3\n"), "connection_id", "A"))
	require.Len(t, outA, 1)
	idx, _ := outA[0].Metadata().Get("frame.index")
	assert.Equal(t, "2", idx, "reset 이후 A 의 인덱스가 유지되어야 한다")
}

// TestFramerNode_Shutdown_FlushesIncompleteBuffers_ErrorPort 는 Shutdown 시
// 각 미완성 스트림 버퍼에 대해 incomplete 리포트가 발행됨을 검증한다 (R4.6).
// 본 구현은 Option B (slog.Warn) + test sink 방식으로, 테스트는 sink 를
// 주입하여 리포트를 수집한다.
func TestFramerNode_Shutdown_FlushesIncompleteBuffers_ErrorPort(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var reports []incompleteStreamReport
	sink := func(r incompleteStreamReport) {
		mu.Lock()
		defer mu.Unlock()
		reports = append(reports, r)
	}
	fn := newFramerNodeForTest(t, "framer-shutdown-flush", "newline",
		withIncompleteSink(sink))
	require.NoError(t, fn.Init(context.Background()))

	// Stream A: 미완성 "foo"
	_, _ = processExpectOK(t, fn, inputMsgForStream([]byte("foo"), "connection_id", "A"))
	// Stream B: 미완성 "bar"
	_, _ = processExpectOK(t, fn, inputMsgForStream([]byte("bar"), "connection_id", "B"))

	// Shutdown
	require.NoError(t, fn.Shutdown(context.Background()))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, reports, 2, "두 미완성 스트림에 대해 각각 한 번씩 리포트되어야 함")

	// 리포트 내용 검증 (순서 독립적으로)
	seen := map[string]int{}
	for _, r := range reports {
		seen[r.StreamKey] = r.BytesRemaining
		assert.Equal(t, "frame.buffer.incomplete_on_stop", r.ErrorCode)
		assert.Equal(t, "newline", r.FramerType)
	}
	assert.Equal(t, 3, seen["A"], "Stream A 는 3 바이트 ('foo') 남아 있어야 함")
	assert.Equal(t, 3, seen["B"], "Stream B 는 3 바이트 ('bar') 남아 있어야 함")
}

// TestFramerNode_Shutdown_Idempotent 는 Shutdown 을 두 번 호출해도 리포트가
// 한 번만 발행되고 panic 이 없음을 검증한다 (R4.6, NFR17).
func TestFramerNode_Shutdown_Idempotent(t *testing.T) {
	t.Parallel()
	var count int
	var mu sync.Mutex
	sink := func(r incompleteStreamReport) {
		mu.Lock()
		defer mu.Unlock()
		count++
	}
	fn := newFramerNodeForTest(t, "framer-shutdown-idem", "newline",
		withIncompleteSink(sink))
	require.NoError(t, fn.Init(context.Background()))

	_, _ = processExpectOK(t, fn, inputMsgForStream([]byte("partial"), "connection_id", "A"))

	require.NoError(t, fn.Shutdown(context.Background()))
	require.NoError(t, fn.Shutdown(context.Background()))

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, count, "Shutdown 은 idempotent 하므로 리포트는 한 번만 발행되어야 함")
}

// TestFramerNode_Shutdown_NoIncompleteData_NoErrorEmitted 는 모든 스트림이
// 깨끗이 비었을 때 Shutdown 이 리포트를 생성하지 않음을 검증한다.
func TestFramerNode_Shutdown_NoIncompleteData_NoErrorEmitted(t *testing.T) {
	t.Parallel()
	var count int
	var mu sync.Mutex
	sink := func(r incompleteStreamReport) {
		mu.Lock()
		defer mu.Unlock()
		count++
	}
	fn := newFramerNodeForTest(t, "framer-shutdown-clean", "newline",
		withIncompleteSink(sink))
	require.NoError(t, fn.Init(context.Background()))

	// 완전한 프레임 "x\n" → 버퍼 비어 있음
	_, _ = processExpectOK(t, fn, inputMsgForStream([]byte("x\n"), "connection_id", "A"))

	require.NoError(t, fn.Shutdown(context.Background()))

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 0, count, "깨끗한 Shutdown 은 리포트를 발행하지 않아야 함")
}

// TestFramerNode_MultiStream_ErrorInOneStream_DoesNotAffectOthers 는 한 스트림
// 에서 파싱 에러가 발생해도 다른 스트림의 조립이 정상 진행됨을 검증한다 (R4.8).
func TestFramerNode_MultiStream_ErrorInOneStream_DoesNotAffectOthers(t *testing.T) {
	t.Parallel()
	// frame 모드 + ETX 검증
	etxOpts := framing.Options{
		STX:          []byte{0x02},
		ETX:          []byte{0x03},
		LengthOffset: 1,
		LengthSize:   1,
		Checksum:     "none",
	}
	fn := newFramerNodeForTest(t, "framer-multi-err", "frame",
		withFramingOptions("frame", etxOpts))
	require.NoError(t, fn.Init(context.Background()))

	// Stream A: 잘못된 ETX → error
	badInput := []byte{0x02, 0x02, 'a', 0xFF}
	_, errsA := processExpectOK(t, fn, inputMsgForStream(badInput, "connection_id", "A"))
	require.GreaterOrEqual(t, len(errsA), 1)
	code, _ := errsA[0].Metadata().Get("frame.error.code")
	assert.Equal(t, "frame.parse.etx_mismatch", code)
	// 에러 메시지의 stream_key 도 A 여야 함
	skA, _ := errsA[0].Metadata().Get("frame.stream_key")
	assert.Equal(t, "A", skA)

	// Stream B: 올바른 프레임 → 성공
	goodInput := []byte{0x02, 0x02, 'b', 0x03}
	outB, errsB := processExpectOK(t, fn, inputMsgForStream(goodInput, "connection_id", "B"))
	assert.Len(t, errsB, 0)
	require.Len(t, outB, 1)
	gotB, _ := outB[0].Payload().Get("raw")
	assert.Equal(t, goodInput, gotB.([]byte))
}

// TestFramerNode_MultiStream_ManyStreams_Stress 는 다수의 스트림이 동시에
// 존재할 때의 기본 정확성을 검증한다.
func TestFramerNode_MultiStream_ManyStreams_Stress(t *testing.T) {
	t.Parallel()
	const numStreams = 50
	fn := newFramerNodeForTest(t, "framer-multi-stress", "newline",
		withNodeOptions(framerNodeOptions{MaxStreams: 0}))
	require.NoError(t, fn.Init(context.Background()))

	for i := 0; i < numStreams; i++ {
		key := fmt.Sprintf("s-%d", i)
		payload := []byte(fmt.Sprintf("hello-%d\n", i))
		out, errs := processExpectOK(t, fn, inputMsgForStream(payload, "connection_id", key))
		require.Len(t, out, 1, "stream %s", key)
		require.Len(t, errs, 0, "stream %s", key)
		sk, _ := out[0].Metadata().Get("frame.stream_key")
		assert.Equal(t, key, sk)
		idx, _ := out[0].Metadata().Get("frame.index")
		assert.Equal(t, "0", idx)
	}
}

// ---------------------------------------------------------------------------
// Phase 5: 에러 포트 통합 및 관측성 (R7.1 ~ R7.6)
// ---------------------------------------------------------------------------

// TestFramerNode_Error_ChecksumMismatch_FrameMode 는 frame 모드에서 체크섬
// 불일치 시 error 포트로 frame.parse.checksum_mismatch 코드가 전달됨을 검증한다.
func TestFramerNode_Error_ChecksumMismatch_FrameMode(t *testing.T) {
	t.Parallel()
	// STX=0x02, ETX=0x03, length_size=1, checksum=sum8
	csOpts := framing.Options{
		STX:          []byte{0x02},
		ETX:          []byte{0x03},
		LengthOffset: 1,
		LengthSize:   1,
		Checksum:     "sum8",
	}
	fn := newFramerNodeForTest(t, "framer-cs", "frame",
		withFramingOptions("frame", csOpts))
	require.NoError(t, fn.Init(context.Background()))

	// 프레임 구성: STX(0x02) + length(0x04) + payload('AB') + ETX(0x03) + checksum
	// length = 4: length 필드(1) + payload(2) + ETX(1) = 4 (lengthIncludesHeader=false 이므로 길이 필드 이후의 바이트 수)
	// sum8 체크섬: STX 이후 ~ checksum 직전까지의 합
	// 올바른 체크섬 대신 잘못된 값 (0xFF) 을 삽입
	input := []byte{0x02, 0x04, 'A', 'B', 0x03, 0xFF}
	results, err := fn.Process(context.Background(), inputMsgWithRaw(input))
	require.NoError(t, err)
	_, errMsgs := splitResults(results)
	require.GreaterOrEqual(t, len(errMsgs), 1, "expected at least one error message")
	code, ok := errMsgs[0].Metadata().Get("frame.error.code")
	require.True(t, ok)
	assert.Equal(t, "frame.parse.checksum_mismatch", code)
}

// TestFramerNode_Error_AllCodes_Metadata 는 각 에러 코드에 대해 에러 메시지에
// 필수 메타데이터 (frame.error.code, frame.framer_type, frame.stream_key,
// frame.buffer.bytes_at_error) 가 모두 존재함을 검증한다.
func TestFramerNode_Error_AllCodes_Metadata(t *testing.T) {
	t.Parallel()

	// 필수 메타데이터 키
	requiredKeys := []string{
		"frame.error.code",
		"frame.framer_type",
		"frame.stream_key",
		"frame.buffer.bytes_at_error",
	}

	// 각 에러 코드별 테스트 케이스를 테이블로 정의
	cases := []struct {
		name         string
		framingMode  string
		framingOpts  *framing.Options
		nodeOpts     *framerNodeOptions
		input        func() message.Message
		expectedCode string
	}{
		{
			name:        "invalid_payload",
			framingMode: "newline",
			input: func() message.Message {
				// raw/data 키 없음
				return message.New()
			},
			expectedCode: "frame.input.invalid_payload",
		},
		{
			name:        "etx_mismatch",
			framingMode: "frame",
			framingOpts: &framing.Options{
				STX:          []byte{0x02},
				ETX:          []byte{0x03},
				LengthOffset: 1,
				LengthSize:   1,
				Checksum:     "none",
			},
			input: func() message.Message {
				// ETX 위치에 잘못된 바이트
				return inputMsgWithRaw([]byte{0x02, 0x02, 'a', 0xFF})
			},
			expectedCode: "frame.parse.etx_mismatch",
		},
		{
			name:        "checksum_mismatch",
			framingMode: "frame",
			framingOpts: &framing.Options{
				STX:          []byte{0x02},
				ETX:          []byte{0x03},
				LengthOffset: 1,
				LengthSize:   1,
				Checksum:     "sum8",
			},
			input: func() message.Message {
				return inputMsgWithRaw([]byte{0x02, 0x04, 'A', 'B', 0x03, 0xFF})
			},
			expectedCode: "frame.parse.checksum_mismatch",
		},
		{
			name:        "max_size_exceeded",
			framingMode: "length_prefix",
			framingOpts: &framing.Options{MaxMessageSize: 10},
			input: func() message.Message {
				header := make([]byte, 4)
				binary.BigEndian.PutUint32(header, 100) // max 10 초과
				return inputMsgWithRaw(header)
			},
			expectedCode: "frame.parse.max_size_exceeded",
		},
		{
			name:        "max_streams_exceeded",
			framingMode: "newline",
			nodeOpts:    &framerNodeOptions{MaxStreams: 1, StreamKeyMetadata: "connection_id"},
			input: func() message.Message {
				// 두 번째 스트림 → max_streams 초과 (첫 번째는 setup 에서 주입)
				return inputMsgWithRaw([]byte("data\n"), "connection_id", "stream-2")
			},
			expectedCode: "frame.buffer.max_streams_exceeded",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := []framerOption{}
			if tc.framingOpts != nil {
				opts = append(opts, withFramingOptions(tc.framingMode, *tc.framingOpts))
			}
			if tc.nodeOpts != nil {
				opts = append(opts, withNodeOptions(*tc.nodeOpts))
			}
			fn := newFramerNodeForTest(t, "framer-meta-"+tc.name, tc.framingMode, opts...)
			require.NoError(t, fn.Init(context.Background()))

			// max_streams 테스트: 먼저 하나의 스트림을 생성해야 함
			if tc.expectedCode == "frame.buffer.max_streams_exceeded" {
				_, _ = fn.Process(context.Background(),
					inputMsgWithRaw([]byte("first\n"), "connection_id", "stream-1"))
			}

			results, err := fn.Process(context.Background(), tc.input())
			require.NoError(t, err)
			_, errMsgs := splitResults(results)
			require.GreaterOrEqual(t, len(errMsgs), 1,
				"expected at least one error message for code %s", tc.expectedCode)

			errMsg := errMsgs[0]
			// 에러 코드 확인
			code, ok := errMsg.Metadata().Get("frame.error.code")
			require.True(t, ok, "frame.error.code missing")
			assert.Equal(t, tc.expectedCode, code)

			// 필수 메타데이터 키 존재 확인
			for _, key := range requiredKeys {
				_, exists := errMsg.Metadata().Get(key)
				assert.True(t, exists, "required metadata key %q missing for error code %s", key, tc.expectedCode)
			}
		})
	}
}

// TestFramerNode_Error_LengthInvalid_LengthPrefix 는 length_prefix 모드에서
// 디코딩된 길이가 안전 상한을 초과할 때 frame.parse.length_invalid 코드가
// 반환됨을 검증한다.
func TestFramerNode_Error_LengthInvalid_LengthPrefix(t *testing.T) {
	t.Parallel()
	// maxMessageSize=0 (미설정) 이면 안전 상한 (256MiB) 을 초과하는 길이 검사
	lpOpts := framing.Options{MaxMessageSize: 0}
	fn := newFramerNodeForTest(t, "framer-leninv", "length_prefix",
		withFramingOptions("length_prefix", lpOpts))
	require.NoError(t, fn.Init(context.Background()))

	header := make([]byte, 4)
	// 0xFFFFFFFF = 4294967295, 안전 상한 (256MiB = 268435456) 초과
	binary.BigEndian.PutUint32(header, 0xFFFFFFFF)
	results, err := fn.Process(context.Background(), inputMsgWithRaw(header))
	require.NoError(t, err)
	_, errMsgs := splitResults(results)
	require.GreaterOrEqual(t, len(errMsgs), 1)
	code, _ := errMsgs[0].Metadata().Get("frame.error.code")
	assert.Equal(t, "frame.parse.length_invalid", code)
}
