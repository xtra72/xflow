package modbus

import (
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// M1 — F4 프레임 로그 / Raw 프레임 (REQ-MODBUS-008-01, AC-01/AC-02)
// ---------------------------------------------------------------------------
//
// 게이트웨이 observability_test.go 의 캡처 핸들러 패턴을 클라이언트로 포팅한다.
// mock 이 아닌 실제 트랜스포트(ModbusTCPTransport/ModbusRTUTransport)의 SendAndReceive
// 프레임 hook 을 net.Pipe/mock 시리얼 포트로 구동하여 검증한다(하드웨어 불필요).

// ---------------------------------------------------------------------------
// 캡처 slog 핸들러 (프레임 로그 검증용)
// ---------------------------------------------------------------------------

type frameRecord struct {
	msg   string
	attrs map[string]any
}

type frameCaptureHandler struct {
	mu    sync.Mutex
	recs  []frameRecord
	level slog.Level
}

func newFrameCapture(level slog.Level) *frameCaptureHandler {
	return &frameCaptureHandler{level: level}
}

func (h *frameCaptureHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }

func (h *frameCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	rec := frameRecord{msg: r.Message, attrs: map[string]any{}}
	r.Attrs(func(a slog.Attr) bool {
		rec.attrs[a.Key] = a.Value.Any()
		return true
	})
	h.mu.Lock()
	h.recs = append(h.recs, rec)
	h.mu.Unlock()
	return nil
}

func (h *frameCaptureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *frameCaptureHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *frameCaptureHandler) snapshot() []frameRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]frameRecord, len(h.recs))
	copy(out, h.recs)
	return out
}

// clientFramesByDir 는 "modbus-client: frame" 레코드 중 dir 이 일치하는 것을 반환한다.
func clientFramesByDir(recs []frameRecord, dir string) []frameRecord {
	var out []frameRecord
	for _, r := range recs {
		if r.msg == "modbus-client: frame" && r.attrs["dir"] == dir {
			out = append(out, r)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// TCP 프레임 로그 (net.Pipe 로 실제 ModbusTCPTransport.SendAndReceive 구동)
// ---------------------------------------------------------------------------

// runTCPFrameLog 는 net.Pipe 로 실제 TCP 트랜스포트를 구동하여 FC03 읽기 1회를 수행하고
// 캡처된 로그 레코드를 반환한다. frames/raw 토글을 파라미터로 받는다.
func runTCPFrameLog(t *testing.T, frames, raw bool) []frameRecord {
	t.Helper()
	cap := newFrameCapture(slog.LevelDebug)
	logger := slog.New(cap)

	tr := NewModbusTCPTransport("10.0.0.1", 502, time.Second, logger)
	tr.setObs(newClientObs(frames, raw))

	server, client := net.Pipe()
	tr.conn = client
	tr.connected = true
	defer client.Close()
	defer server.Close()

	// 서버 goroutine: 요청 MBAP 프레임을 읽고 FC03 응답 프레임을 회신한다.
	done := make(chan struct{})
	go func() {
		defer close(done)
		hdr := make([]byte, MBAPHeaderSize)
		if _, err := io.ReadFull(server, hdr); err != nil {
			return
		}
		length := binary.BigEndian.Uint16(hdr[4:6])
		pduBuf := make([]byte, int(length)-1) // UnitID 는 헤더에 포함
		if _, err := io.ReadFull(server, pduBuf); err != nil {
			return
		}
		txID := binary.BigEndian.Uint16(hdr[0:2])
		resp := buildFC03Response(txID, hdr[6], 1) // 1 레지스터 응답
		_, _ = server.Write(resp)
	}()

	reqPDU := buildReadPDU(FC03ReadHoldingRegisters, 0, 1)
	_, err := tr.SendAndReceive(context.Background(), 1, reqPDU)
	require.NoError(t, err)
	<-done
	return cap.snapshot()
}

// TestClientLogFrames_TCP_EmitsRXandTX 는 AC-01(TCP)을 검증한다:
// log_frames:true 에서 TCP TX/RX 프레임 요약(dir/unit_id/fc/len)이 "modbus-client: frame" 으로 로그된다.
func TestClientLogFrames_TCP_EmitsRXandTX(t *testing.T) {
	recs := runTCPFrameLog(t, true, false)

	tx := clientFramesByDir(recs, "TX")
	rx := clientFramesByDir(recs, "RX")
	require.Len(t, tx, 1, "TX 프레임 요약 1건이 로그되어야 한다")
	require.Len(t, rx, 1, "RX 프레임 요약 1건이 로그되어야 한다")

	// 요약 필드: unit_id, fc, len. raw off → hex 없음.
	assert.Equal(t, uint64(1), tx[0].attrs["unit_id"]) // slog 는 byte 를 uint64 로 저장
	assert.Equal(t, "0x03", tx[0].attrs["fc"])
	assert.Contains(t, tx[0].attrs, "len")
	assert.NotContains(t, tx[0].attrs, "hex")
	// RX 응답 FC 도 0x03(정상 FC03 응답).
	assert.Equal(t, "0x03", rx[0].attrs["fc"])
	assert.NotContains(t, rx[0].attrs, "hex")
}

// TestClientLogFrames_TCP_RawIncludesHex 는 AC-02(c)(TCP)을 검증한다:
// log_frames:true + log_raw_frames:true 에서 전체 ADU hex 가 함께 로그된다.
func TestClientLogFrames_TCP_RawIncludesHex(t *testing.T) {
	recs := runTCPFrameLog(t, true, true)

	tx := clientFramesByDir(recs, "TX")
	rx := clientFramesByDir(recs, "RX")
	require.Len(t, tx, 1)
	require.Len(t, rx, 1)
	assert.Contains(t, tx[0].attrs, "hex")
	assert.NotEmpty(t, tx[0].attrs["hex"])
	assert.Contains(t, rx[0].attrs, "hex")
	assert.NotEmpty(t, rx[0].attrs["hex"])
}

// TestClientLogFrames_TCP_DisabledEmitsNothing 는 AC-02(a)(TCP)을 검증한다:
// 플래그 없음(frames off) → 프레임 로그가 전혀 방출되지 않는다(no-op).
func TestClientLogFrames_TCP_DisabledEmitsNothing(t *testing.T) {
	recs := runTCPFrameLog(t, false, false)
	assert.Empty(t, clientFramesByDir(recs, "TX"))
	assert.Empty(t, clientFramesByDir(recs, "RX"))
}

// TestClientLogFrames_TCP_RawWithoutFramesIsNoOp 는 AC-02 게이팅을 검증한다:
// log_frames:false 이면 log_raw_frames:true 여도 프레임 로그는 no-op 이다.
func TestClientLogFrames_TCP_RawWithoutFramesIsNoOp(t *testing.T) {
	recs := runTCPFrameLog(t, false, true)
	assert.Empty(t, clientFramesByDir(recs, "TX"))
	assert.Empty(t, clientFramesByDir(recs, "RX"))
}

// ---------------------------------------------------------------------------
// RTU 프레임 로그 (mock 시리얼 포트로 실제 ModbusRTUTransport.SendAndReceive 구동)
// ---------------------------------------------------------------------------

// runRTUFrameLog 는 mock 시리얼 포트로 실제 RTU 트랜스포트를 구동하여 FC03 읽기 1회를
// 수행하고 캡처된 로그 레코드를 반환한다.
func runRTUFrameLog(t *testing.T, frames, raw bool) []frameRecord {
	t.Helper()
	cap := newFrameCapture(slog.LevelDebug)

	respPDU := []byte{0x03, 0x04, 0x12, 0x34, 0x56, 0x78}
	respADU := buildRTUADU(0x01, respPDU) // 유효 CRC 포함
	port := newMockSerialPort(respADU)

	tr := newTestRTUTransport(port)
	tr.logger = slog.New(cap)
	tr.setObs(newClientObs(frames, raw))
	require.NoError(t, tr.Connect(context.Background()))

	reqPDU := buildReadPDU(FC03ReadHoldingRegisters, 0, 2)
	_, err := tr.SendAndReceive(context.Background(), 0x01, reqPDU)
	require.NoError(t, err)
	return cap.snapshot()
}

// TestClientLogFrames_RTU_EmitsRXandTX 는 AC-01(RTU)을 검증한다:
// log_frames:true 에서 RTU TX/RX 프레임 요약이 "modbus-client: frame" 으로 로그된다(CRC 프레이밍).
func TestClientLogFrames_RTU_EmitsRXandTX(t *testing.T) {
	recs := runRTUFrameLog(t, true, false)

	tx := clientFramesByDir(recs, "TX")
	rx := clientFramesByDir(recs, "RX")
	require.Len(t, tx, 1, "RTU TX 프레임 요약 1건이 로그되어야 한다")
	require.Len(t, rx, 1, "RTU RX 프레임 요약 1건이 로그되어야 한다")

	assert.Equal(t, uint64(1), tx[0].attrs["unit_id"])
	assert.Equal(t, "0x03", tx[0].attrs["fc"])
	assert.Contains(t, tx[0].attrs, "len")
	assert.NotContains(t, tx[0].attrs, "hex")
	assert.Equal(t, "0x03", rx[0].attrs["fc"])
}

// TestClientLogFrames_RTU_RawIncludesHex 는 AC-02(c)(RTU)을 검증한다.
func TestClientLogFrames_RTU_RawIncludesHex(t *testing.T) {
	recs := runRTUFrameLog(t, true, true)

	tx := clientFramesByDir(recs, "TX")
	rx := clientFramesByDir(recs, "RX")
	require.Len(t, tx, 1)
	require.Len(t, rx, 1)
	assert.Contains(t, tx[0].attrs, "hex")
	assert.NotEmpty(t, tx[0].attrs["hex"])
	assert.Contains(t, rx[0].attrs, "hex")
	assert.NotEmpty(t, rx[0].attrs["hex"])
}

// TestClientLogFrames_RTU_DisabledEmitsNothing 는 AC-02(a)(RTU)을 검증한다(no-op).
func TestClientLogFrames_RTU_DisabledEmitsNothing(t *testing.T) {
	recs := runRTUFrameLog(t, false, false)
	assert.Empty(t, clientFramesByDir(recs, "TX"))
	assert.Empty(t, clientFramesByDir(recs, "RX"))
}

// ---------------------------------------------------------------------------
// 설정 파싱 + Configure 라이브 토글
// ---------------------------------------------------------------------------

// TestParseModbusConfig_FrameLogFlags 는 log_frames/log_raw_frames 파싱을 검증한다(AC-02).
func TestParseModbusConfig_FrameLogFlags(t *testing.T) {
	base := minimalAgentConfig().Transport.Options

	// (a) 플래그 없음 → 둘 다 false.
	cfg, err := parseModbusConfig(base)
	require.NoError(t, err)
	assert.False(t, cfg.LogFrames)
	assert.False(t, cfg.LogRawFrames)

	// (b) log_frames:true + log_raw_frames:false.
	opts := minimalAgentConfig().Transport.Options
	opts["log_frames"] = true
	cfg, err = parseModbusConfig(opts)
	require.NoError(t, err)
	assert.True(t, cfg.LogFrames)
	assert.False(t, cfg.LogRawFrames)

	// (c) 둘 다 true.
	opts = minimalAgentConfig().Transport.Options
	opts["log_frames"] = true
	opts["log_raw_frames"] = true
	cfg, err = parseModbusConfig(opts)
	require.NoError(t, err)
	assert.True(t, cfg.LogFrames)
	assert.True(t, cfg.LogRawFrames)
}

// TestConfigure_LiveTogglesFrameLog 는 Configure 로 프레임 로그 토글을 재시작 없이 갱신함을 검증한다
// (게이트웨이 atomic 라이브 갱신 패턴 준거).
func TestConfigure_LiveTogglesFrameLog(t *testing.T) {
	a, _ := newTestModbusAgent(t, minimalAgentConfig())

	// 기본값: 두 토글 모두 off.
	assert.False(t, a.obs.framesOn())
	assert.False(t, a.obs.rawOn())

	// Configure 로 두 토글을 켠다(재시작 없이 즉시 반영).
	newCfg := minimalAgentConfig()
	newCfg.Transport.Options["log_frames"] = true
	newCfg.Transport.Options["log_raw_frames"] = true
	require.NoError(t, a.Configure(newCfg))

	assert.True(t, a.obs.framesOn())
	assert.True(t, a.obs.rawOn())

	// 다시 끈다.
	off := minimalAgentConfig()
	require.NoError(t, a.Configure(off))
	assert.False(t, a.obs.framesOn())
	assert.False(t, a.obs.rawOn())
}
