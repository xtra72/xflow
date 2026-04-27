package node

// framer_parity_test.go verifies that the direct framing engine path
// (pkg/framing.Framer.Drain) and the FramerNode.Process path produce
// byte-identical frame sequences for the same input. This satisfies
// SPEC-NODE-002 M5 (R5.1~R5.5) — "serial agent vs framer node parity".

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/framing"
)

// --- helper: direct framer path (Path A) ---

// drainDirect creates a framer via framing.New and calls Drain on the
// concatenated input bytes. It returns the extracted frames.
func drainDirect(t *testing.T, mode string, opts framing.Options, data []byte) [][]byte {
	t.Helper()
	f, err := framing.New(mode, opts)
	require.NoError(t, err, "framing.New failed")
	frames, _, drainErr := f.Drain(data)
	// For parity tests we typically expect no error (caller checks separately
	// for error-path tests).
	if drainErr != nil {
		t.Logf("drainDirect: non-fatal drain error: %v", drainErr)
	}
	return frames
}

// --- helper: framer node path (Path B) ---

// processNode creates a FramerNode, initialises it, and feeds each chunk
// via Process, collecting out-port frames. Returns the concatenated frame
// sequence across all Process calls.
func processNode(t *testing.T, mode string, opts framing.Options, chunks [][]byte) [][]byte {
	t.Helper()
	fn := newFramerNodeForTest(t, "parity-node", mode, withFramingOptions(mode, opts))
	require.NoError(t, fn.Init(context.Background()))

	var allFrames [][]byte
	for _, chunk := range chunks {
		results, err := fn.Process(context.Background(), inputMsgWithRaw(chunk))
		require.NoError(t, err)
		outMsgs, _ := splitResults(results)
		for _, m := range outMsgs {
			raw, ok := m.Payload().Get("raw")
			require.True(t, ok, "output message missing raw key")
			allFrames = append(allFrames, raw.([]byte))
		}
	}
	return allFrames
}

// processNodeSingle is a convenience wrapper that feeds all data in one
// Process call.
func processNodeSingle(t *testing.T, mode string, opts framing.Options, data []byte) [][]byte {
	t.Helper()
	return processNode(t, mode, opts, [][]byte{data})
}

// assertFramesParity asserts that two frame sequences are identical in count,
// order, and byte content.
func assertFramesParity(t *testing.T, label string, a, b [][]byte) {
	t.Helper()
	require.Equal(t, len(a), len(b), "%s: frame count mismatch (direct=%d, node=%d)", label, len(a), len(b))
	for i := range a {
		assert.Truef(t, bytes.Equal(a[i], b[i]),
			"%s: frame %d differs\n  direct: %x\n    node: %x", label, i, a[i], b[i])
	}
}

// =============================================================================
// Test 1: LGCP samples — direct Drain vs FramerNode Process
// =============================================================================

func TestParity_LGCPSamples_DirectFramerVsFramerNode(t *testing.T) {
	t.Parallel()

	lgcpOpts := framing.Options{
		STX:                  []byte{0x56},
		LengthOffset:         1,
		LengthSize:           1,
		LengthEndian:         "big",
		LengthIncludesHeader: true,
		LengthAdjustment:     0,
		Checksum:             "none",
		MaxMessageSize:       256,
	}

	// Same LGCP samples used in TestFrameFramer_LGCPSamples.
	samples := []struct {
		name string
		hex  string
	}{
		{
			name: "plen=26 request (cmd=0204 status)",
			hex:  "562d044455006504445500000204b11a110010c018001ac01300134013c016001841188829c01dc03954a06492",
		},
		{
			name: "plen=41 response (cmd=0204 status)",
			hex:  "563c044455000004445500670204df2960c161d05e62104c624162d0c86f816fc07490198a8060816100b0406400645054648a64c065411fd857e37a",
		},
		{
			name: "plen=1 keep-alive (cmd=0604)",
			hex:  "561404ffffffff04445500000604000102a1b7ed",
		},
	}

	// Sub-test per individual sample.
	for _, s := range samples {
		s := s
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
			raw, err := hex.DecodeString(s.hex)
			require.NoError(t, err)

			directFrames := drainDirect(t, framing.ModeFrame, lgcpOpts, raw)
			nodeFrames := processNodeSingle(t, framing.ModeFrame, lgcpOpts, raw)
			assertFramesParity(t, s.name, directFrames, nodeFrames)
		})
	}

	// Concatenated stream — all three samples back-to-back.
	t.Run("concatenated stream", func(t *testing.T) {
		t.Parallel()
		var stream []byte
		for _, s := range samples {
			raw, _ := hex.DecodeString(s.hex)
			stream = append(stream, raw...)
		}
		directFrames := drainDirect(t, framing.ModeFrame, lgcpOpts, stream)
		nodeFrames := processNodeSingle(t, framing.ModeFrame, lgcpOpts, stream)
		assertFramesParity(t, "concatenated LGCP", directFrames, nodeFrames)
		require.Equal(t, len(samples), len(directFrames), "should extract exactly %d frames", len(samples))
	})
}

// =============================================================================
// Test 2: Newline mode — whole vs 1-byte-at-a-time chunked
// =============================================================================

func TestParity_NewlineMode_StressChunked(t *testing.T) {
	t.Parallel()

	nlOpts := framing.Options{BufferSize: 1024}
	input := []byte("line1\nline2\nline3\npartial")

	// Whole: feed all bytes at once.
	wholeFrames := processNodeSingle(t, framing.ModeNewline, nlOpts, input)

	// Chunked: feed 1 byte at a time.
	chunks := make([][]byte, len(input))
	for i := range input {
		chunks[i] = input[i : i+1]
	}
	chunkedFrames := processNode(t, framing.ModeNewline, nlOpts, chunks)

	// Both should produce the same complete frames. "partial" remains
	// buffered in both paths and should not appear as a frame.
	assertFramesParity(t, "newline whole vs chunked", wholeFrames, chunkedFrames)
	require.Equal(t, 3, len(wholeFrames), "should have 3 complete lines")

	// Verify frame contents.
	expected := []string{"line1", "line2", "line3"}
	for i, want := range expected {
		assert.Equal(t, want, string(wholeFrames[i]), "frame %d content", i)
	}
}

// =============================================================================
// Test 3: Length-prefix mode — whole vs chunked
// =============================================================================

func TestParity_LengthPrefix_Chunked(t *testing.T) {
	t.Parallel()

	lpOpts := framing.Options{}
	payloads := []string{"alpha", "beta", "gamma"}

	var stream bytes.Buffer
	for _, p := range payloads {
		header := make([]byte, 4)
		binary.BigEndian.PutUint32(header, uint32(len(p)))
		stream.Write(header)
		stream.Write([]byte(p))
	}
	data := stream.Bytes()

	// Direct path.
	directFrames := drainDirect(t, framing.ModeLengthPrefix, lpOpts, data)

	// Node whole path.
	nodeWhole := processNodeSingle(t, framing.ModeLengthPrefix, lpOpts, data)
	assertFramesParity(t, "lp direct vs node whole", directFrames, nodeWhole)

	// Node chunked: 1 byte at a time.
	chunks := make([][]byte, len(data))
	for i := range data {
		chunks[i] = data[i : i+1]
	}
	nodeChunked := processNode(t, framing.ModeLengthPrefix, lpOpts, chunks)
	assertFramesParity(t, "lp node whole vs chunked", nodeWhole, nodeChunked)

	// Verify payload contents (length_prefix Drain returns payload only).
	require.Equal(t, len(payloads), len(directFrames))
	for i, want := range payloads {
		assert.Equal(t, want, string(directFrames[i]), "payload %d", i)
	}
}

// =============================================================================
// Test 4: Frame mode — ETX mismatch recovery
// =============================================================================

func TestParity_FrameMode_ETXMismatch_Recovery(t *testing.T) {
	t.Parallel()

	// Use STX/ETX frame mode with ETX verification.
	opts := framing.Options{
		STX:                  []byte{0x02},
		ETX:                  []byte{0x03},
		LengthOffset:         1,
		LengthSize:           1,
		LengthEndian:         "big",
		LengthIncludesHeader: false,
		LengthAdjustment:     0,
		Checksum:             "none",
		MaxMessageSize:       256,
	}

	// Construct: corrupted frame (bad ETX) + valid frame.
	//
	// Corrupted: STX(02) LEN(03) P1 P2 P3 badETX(FF)  = 6 bytes total
	// Valid:     STX(02) LEN(03) P4 P5 P6 ETX(03)      = 6 bytes total
	//
	// Frame structure: STX + 1-byte length field = 2 byte header.
	// length_includes_header=false so LEN=03 means 3 payload bytes after header.
	// Total frame = 2 (header) + 3 (payload) + 1 (ETX) = 6 bytes? No —
	// Let's reason: the frameFramer treats the full frame as headerSize + payloadLen.
	// headerSize = len(STX) + headerExtra + lengthSize = 1 + 0 + 1 = 2.
	// payloadLen = LEN(03) + adjustment(0) = 3.
	// total = 2 + 3 = 5.
	// But ETX check: etxStart = len(frame) - len(etx) = 5 - 1 = 4.
	// So byte at index 4 must be 0x03.
	corrupted := []byte{0x02, 0x03, 0xAA, 0xBB, 0xFF} // ETX at idx 4 = 0xFF != 0x03
	valid := []byte{0x02, 0x03, 0xCC, 0xDD, 0x03}     // ETX at idx 4 = 0x03 OK

	combined := append(corrupted, valid...)

	// Direct path: Drain returns error on ETX mismatch but may have extracted
	// frames before the error, plus remainder for re-parsing.
	// The frame.go Drain on ETX mismatch: returns frames so far, remainder =
	// buf[framePos+1:], and the error. We need to continue draining the remainder.
	directFrames := drainWithRecovery(t, framing.ModeFrame, opts, combined)

	// Node path: FramerNode.Process calls Drain once; on error it emits the
	// error message and keeps the remainder in the buffer. A second Process
	// call with empty data could flush, but we just need to call Process with
	// the rest. Actually the node stores remainder in sb.buf, so a second
	// Process with empty or any data triggers re-drain. Let's send the full
	// combined in one Process, then send an empty byte to trigger re-drain.
	fn := newFramerNodeForTest(t, "parity-etx", framing.ModeFrame, withFramingOptions(framing.ModeFrame, opts))
	require.NoError(t, fn.Init(context.Background()))

	var nodeFrames [][]byte

	// First Process: combined bytes.
	results1, err := fn.Process(context.Background(), inputMsgWithRaw(combined))
	require.NoError(t, err)
	outMsgs1, errMsgs1 := splitResults(results1)
	for _, m := range outMsgs1 {
		raw, _ := m.Payload().Get("raw")
		nodeFrames = append(nodeFrames, raw.([]byte))
	}
	// There should be an error message for the ETX mismatch.
	require.NotEmpty(t, errMsgs1, "expected ETX mismatch error message")

	// The remainder after the error (buf[framePos+1:]) is stored in the node's
	// buffer. FramerNode.Process short-circuits on empty data, so we must send
	// at least one byte to trigger a re-drain. We send a single 0x00 byte
	// which won't form a valid STX and thus won't produce extra frames — it
	// just nudges the node to re-drain its internal buffer.
	results2, err := fn.Process(context.Background(), inputMsgWithRaw([]byte{0x00}))
	require.NoError(t, err)
	outMsgs2, _ := splitResults(results2)
	for _, m := range outMsgs2 {
		raw, _ := m.Payload().Get("raw")
		nodeFrames = append(nodeFrames, raw.([]byte))
	}

	// Both paths should have extracted the valid frame.
	assertFramesParity(t, "ETX recovery", directFrames, nodeFrames)
	require.GreaterOrEqual(t, len(directFrames), 1, "should recover at least the valid frame")
}

// drainWithRecovery repeatedly calls Drain on the remainder after errors
// until no more frames can be extracted, simulating full recovery.
func drainWithRecovery(t *testing.T, mode string, opts framing.Options, data []byte) [][]byte {
	t.Helper()
	var allFrames [][]byte
	buf := data
	for len(buf) > 0 {
		f, err := framing.New(mode, opts)
		require.NoError(t, err)
		frames, remainder, drainErr := f.Drain(buf)
		allFrames = append(allFrames, frames...)
		if drainErr != nil {
			// On error, continue with remainder (skip past the bad frame).
			if len(remainder) == 0 || bytes.Equal(remainder, buf) {
				break // no progress, avoid infinite loop
			}
			buf = remainder
			continue
		}
		break // no error, done
	}
	return allFrames
}

// =============================================================================
// Test 5: Fixed-size mode — exact + remainder parity
// =============================================================================

func TestParity_FixedSize_ExactAndRemainder(t *testing.T) {
	t.Parallel()

	fsOpts := framing.Options{FixedSize: 4}
	// 13 bytes = 3 full frames (12 bytes) + 1 byte remainder.
	input := []byte("abcdefghijklm")

	// Direct.
	directFrames := drainDirect(t, framing.ModeFixedSize, fsOpts, input)

	// Node whole.
	nodeWhole := processNodeSingle(t, framing.ModeFixedSize, fsOpts, input)
	assertFramesParity(t, "fixed_size direct vs node", directFrames, nodeWhole)

	// Node chunked: 2 bytes at a time.
	var chunks [][]byte
	for i := 0; i < len(input); i += 2 {
		end := i + 2
		if end > len(input) {
			end = len(input)
		}
		chunks = append(chunks, input[i:end])
	}
	nodeChunked := processNode(t, framing.ModeFixedSize, fsOpts, chunks)
	assertFramesParity(t, "fixed_size whole vs chunked", nodeWhole, nodeChunked)

	// Verify: 3 frames of 4 bytes each.
	require.Equal(t, 3, len(directFrames))
	assert.Equal(t, "abcd", string(directFrames[0]))
	assert.Equal(t, "efgh", string(directFrames[1]))
	assert.Equal(t, "ijkl", string(directFrames[2]))
}

// =============================================================================
// Test 6: Raw mode — trivial passthrough parity
// =============================================================================

func TestParity_RawMode(t *testing.T) {
	t.Parallel()

	rawOpts := framing.Options{BufferSize: 128}
	input := []byte("hello world raw passthrough")

	directFrames := drainDirect(t, framing.ModeRaw, rawOpts, input)
	nodeFrames := processNodeSingle(t, framing.ModeRaw, rawOpts, input)
	assertFramesParity(t, "raw mode", directFrames, nodeFrames)

	// Raw returns the entire input as a single frame.
	require.Equal(t, 1, len(directFrames))
	assert.Equal(t, input, directFrames[0])
}

// =============================================================================
// Bonus: LGCP concatenated — chunked at arbitrary boundaries
// =============================================================================

func TestParity_LGCPSamples_ChunkedAtArbitraryBoundaries(t *testing.T) {
	t.Parallel()

	lgcpOpts := framing.Options{
		STX:                  []byte{0x56},
		LengthOffset:         1,
		LengthSize:           1,
		LengthEndian:         "big",
		LengthIncludesHeader: true,
		LengthAdjustment:     0,
		Checksum:             "none",
		MaxMessageSize:       256,
	}

	hexSamples := []string{
		"562d044455006504445500000204b11a110010c018001ac01300134013c016001841188829c01dc03954a06492",
		"563c044455000004445500670204df2960c161d05e62104c624162d0c86f816fc07490198a8060816100b0406400645054648a64c065411fd857e37a",
		"561404ffffffff04445500000604000102a1b7ed",
	}
	var stream []byte
	for _, h := range hexSamples {
		raw, _ := hex.DecodeString(h)
		stream = append(stream, raw...)
	}

	// Direct: whole.
	directFrames := drainDirect(t, framing.ModeFrame, lgcpOpts, stream)
	require.Equal(t, 3, len(directFrames))

	// Node: chunked at various sizes (table-driven).
	chunkSizes := []int{1, 3, 7, 13, 20, 50}
	for _, cs := range chunkSizes {
		cs := cs
		t.Run(formatChunkLabel(cs), func(t *testing.T) {
			t.Parallel()
			var chunks [][]byte
			for i := 0; i < len(stream); i += cs {
				end := i + cs
				if end > len(stream) {
					end = len(stream)
				}
				chunks = append(chunks, stream[i:end])
			}
			nodeFrames := processNode(t, framing.ModeFrame, lgcpOpts, chunks)
			assertFramesParity(t, formatChunkLabel(cs), directFrames, nodeFrames)
		})
	}
}

func formatChunkLabel(size int) string {
	return "chunk_" + strconv.Itoa(size)
}
