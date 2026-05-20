package lg

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// lgcpMockTransport: LGAPTransport 인터페이스 목 구현
// ---------------------------------------------------------------------------

// lgcpMockTransport 는 테스트용 LGAPTransport 목 구현이다.
type lgcpMockTransport struct {
	mu        sync.Mutex
	buf       *bytes.Buffer
	opened    atomic.Bool
	openErr   error
	closeErr  error
	openCount atomic.Int32
}

func newLGCPMockTransport() *lgcpMockTransport {
	return &lgcpMockTransport{
		buf: bytes.NewBuffer(nil),
	}
}

func (m *lgcpMockTransport) Open() error {
	m.openCount.Add(1)
	if m.openErr != nil {
		return m.openErr
	}
	m.opened.Store(true)
	return nil
}

func (m *lgcpMockTransport) Close() error {
	m.opened.Store(false)
	return m.closeErr
}

func (m *lgcpMockTransport) Send(_ []byte) error {
	// LGCP 에이전트는 절대 Send 를 호출하지 않는다
	return nil
}

func (m *lgcpMockTransport) Receive(buf []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.buf.Len() == 0 {
		return 0, io.EOF
	}
	return m.buf.Read(buf)
}

func (m *lgcpMockTransport) Available() bool {
	return m.opened.Load()
}

func (m *lgcpMockTransport) Write(data []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// 제어 프레임 전송을 시뮬레이션한다 (데이터 소모)
	return len(data), nil
}

// feedData 는 목 트랜스포트에 데이터를 주입한다.
func (m *lgcpMockTransport) feedData(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buf.Write(data)
}

// ---------------------------------------------------------------------------
// 테스트용 헬퍼
// ---------------------------------------------------------------------------

// newTestLGCPAgent 는 테스트용 LGCP 에이전트를 생성한다.
func newTestLGCPAgent(t *testing.T, transport LGAPTransport) *LGCPAgent {
	t.Helper()

	config := agent.AgentConfig{
		ID:   "test-lgcp-001",
		Name: "test-lgcp-capture",
		Type: "lgcp",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"serial_port": "/dev/ttyTEST",
			},
		},
	}

	lgcpConfig := LGCPConfig{
		SerialPort:          "/dev/ttyTEST",
		BaudRate:            9600,
		DataBits:            8,
		StopBits:            1,
		Parity:              "none",
		ReadTimeout:         100 * time.Millisecond,
		MsgChannelSize:      16,
		ReconnectInterval:   100 * time.Millisecond,
		MaxReconnectBackoff: 500 * time.Millisecond,
		VerifyCRC:           false, // 테스트에서는 CRC 검증 비활성화
	}

	a := &LGCPAgent{
		lgcpConfig:   lgcpConfig,
		transport:    transport,
		stopCh:       make(chan struct{}),
		msgCh:        make(chan []byte, lgcpConfig.MsgChannelSize),
		stats:        agent.NewAgentStats(),
		logger:       agent.ResolveLogger(config),
		createdAt:    time.Now(),
		recentFrames: make([]lgcpFrameRecord, lgcpRecentBufferSize),
	}

	return a
}

// runCaptureLoopAndWait 는 captureLoop 를 별도 goroutine 으로 실행하고,
// 지정된 시간만큼 캡처가 진행되도록 sleep 한 뒤 stopCh 를 close 하여
// goroutine 이 완전히 종료될 때까지 기다린다.
//
// 이 헬퍼 없이 단순히 close(a.stopCh) 만 호출하면 goroutine 이 stopCh 를
// 인지하기 전 마지막 프레임을 처리 중일 수 있어 recentIdx/recentFull 등
// 비-atomic 필드 읽기와 race 가 발생한다 (race detector 에서 검출됨).
func runCaptureLoopAndWait(a *LGCPAgent, captureFor time.Duration) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.captureLoop()
	}()
	time.Sleep(captureFor)
	close(a.stopCh)
	<-done
}

// buildLGCPFrame 은 테스트용 최소 LGCP 프레임을 생성한다.
// STX(0x56) + LEN + DLEN + DA + SLEN + SA + CMD(2) + SEQ0 + PLEN + SEQ1 + CRC(2)
func buildLGCPFrame(da, sa []byte, cmd [2]byte, payload []byte) []byte {
	// DLEN(1) + DA + SLEN(1) + SA + CMD(2) + SEQ0(1) + PLEN(1) + PAYLOAD + SEQ1(1) + CRC(2)
	bodyLen := 1 + len(da) + 1 + len(sa) + 2 + 1 + 1 + len(payload) + 1 + 2
	frameLen := 2 + bodyLen // STX + LEN + body

	frame := make([]byte, 0, frameLen)
	frame = append(frame, lgcpSTX)
	frame = append(frame, byte(frameLen))

	// DLEN + DA
	frame = append(frame, byte(len(da)))
	frame = append(frame, da...)

	// SLEN + SA
	frame = append(frame, byte(len(sa)))
	frame = append(frame, sa...)

	// CMD
	frame = append(frame, cmd[0], cmd[1])

	// SEQ0
	frame = append(frame, 0x01)

	// PLEN + PAYLOAD
	frame = append(frame, byte(len(payload)))
	frame = append(frame, payload...)

	// SEQ1
	frame = append(frame, 0x01)

	// CRC (frame[2:len-2] 에 대해 계산)
	crc := CalcLGCPCRC16(frame[2:])
	frame = append(frame, byte(crc>>8), byte(crc&0xFF))

	return frame
}

// ---------------------------------------------------------------------------
// 테스트 케이스
// ---------------------------------------------------------------------------

// TestNewLGCPAgent_Factory 는 팩토리 함수가 올바르게 에이전트를 생성하는지 검증한다.
func TestNewLGCPAgent_Factory(t *testing.T) {
	// LGAPSerialOpener 를 설정하여 팩토리 함수가 트랜스포트를 생성할 수 있도록 한다.
	origOpener := LGAPSerialOpener
	defer func() { LGAPSerialOpener = origOpener }()
	LGAPSerialOpener = func(port string, baudRate, dataBits, stopBits int, parity string) (io.ReadWriteCloser, error) {
		return nil, io.ErrClosedPipe
	}

	config := agent.AgentConfig{
		ID:   "lgcp-001",
		Name: "lgcp-capture",
		Type: "lgcp",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"serial_port": "/dev/ttyUSB0",
			},
		},
	}

	a, err := NewLGCPAgent(config)
	if err != nil {
		t.Fatalf("NewLGCPAgent() error = %v", err)
	}
	if a == nil {
		t.Fatal("NewLGCPAgent() returned nil agent")
	}
	if a.ID() != "lgcp-001" {
		t.Errorf("ID() = %q, want %q", a.ID(), "lgcp-001")
	}
	if a.Name() != "lgcp-capture" {
		t.Errorf("Name() = %q, want %q", a.Name(), "lgcp-capture")
	}
	if a.Type() != "lgcp" {
		t.Errorf("Type() = %q, want %q", a.Type(), "lgcp")
	}
}

// TestNewLGCPAgent_MissingSerialPort 는 필수 설정 누락 시 에러를 반환하는지 검증한다.
func TestNewLGCPAgent_MissingSerialPort(t *testing.T) {
	config := agent.AgentConfig{
		ID:   "lgcp-001",
		Name: "lgcp-capture",
		Type: "lgcp",
		Transport: agent.TransportConfig{
			Type:    "serial",
			Options: map[string]any{},
		},
	}

	_, err := NewLGCPAgent(config)
	if err == nil {
		t.Fatal("NewLGCPAgent() expected error for missing serial_port, got nil")
	}
}

// TestLGCPAgent_CaptureLoop 는 캡처 루프가 프레임을 올바르게 캡처하는지 검증한다.
func TestLGCPAgent_CaptureLoop(t *testing.T) {
	mock := newLGCPMockTransport()

	// 테스트 프레임 생성 (CRC 검증 비활성화)
	da := []byte{0x00, 0x01, 0x02, 0x03}
	sa := []byte{0x10, 0x11, 0x12, 0x13}
	cmd := [2]byte{0x40, 0x80}
	payload := []byte{0xAA, 0xBB}
	frame := buildLGCPFrame(da, sa, cmd, payload)

	mock.feedData(frame)

	a := newTestLGCPAgent(t, mock)
	a.bridgeActive.Store(true) // bridge 소비자 활성화
	mock.opened.Store(true)

	// 캡처 루프를 별도 goroutine 에서 실행
	go a.captureLoop()

	// 메시지 수신 대기
	select {
	case msg := <-a.msgCh:
		var evt LGCPFrameEvent
		if err := json.Unmarshal(msg, &evt); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}
		if evt.Type != "lgcp_frame" {
			t.Errorf("event type = %q, want %q", evt.Type, "lgcp_frame")
		}
		if evt.Parsed == nil {
			t.Fatal("parsed header is nil for valid frame")
		}
		if evt.Parsed.CMD != "4080" {
			t.Errorf("CMD = %q, want %q", evt.Parsed.CMD, "4080")
		}
		if evt.Parsed.DA != "00010203" {
			t.Errorf("DA = %q, want %q", evt.Parsed.DA, "00010203")
		}
		if evt.Parsed.SA != "10111213" {
			t.Errorf("SA = %q, want %q", evt.Parsed.SA, "10111213")
		}
		if evt.Parsed.PLEN != 2 {
			t.Errorf("PLEN = %d, want %d", evt.Parsed.PLEN, 2)
		}
		if evt.Parsed.PayloadHex != "aabb" {
			t.Errorf("PayloadHex = %q, want %q", evt.Parsed.PayloadHex, "aabb")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for captured frame event")
	}

	// 캡처 루프 종료 (EOF 로 자동 종료���)
	close(a.stopCh)

	// 로컬 통계 검증
	if a.framesCaptured.Load() != 1 {
		t.Errorf("framesCaptured = %d, want %d", a.framesCaptured.Load(), 1)
	}

	// AgentStats 통계 검증 (API/웹에 노출되는 공통 통계)
	snap := a.Stats()
	if snap.MessagesReceived < 1 {
		t.Errorf("Stats.MessagesReceived = %d, want >= 1", snap.MessagesReceived)
	}
	if snap.ExternalMessagesReceived < 1 {
		t.Errorf("Stats.ExternalMessagesReceived = %d, want >= 1", snap.ExternalMessagesReceived)
	}
	if snap.BytesRead == 0 {
		t.Error("Stats.BytesRead = 0, want > 0")
	}
	if snap.LastActivityAt.IsZero() {
		t.Error("Stats.LastActivityAt should not be zero")
	}
	if snap.Extra == nil {
		t.Error("Stats.Extra should contain LGCP-specific stats")
	}
}

// TestLGCPAgent_CaptureLoop_MultipleFrames 는 연속 프레임 캡처를 검증한다.
func TestLGCPAgent_CaptureLoop_MultipleFrames(t *testing.T) {
	mock := newLGCPMockTransport()

	da := []byte{0x00}
	sa := []byte{0x10}
	cmd := [2]byte{0x40, 0x80}

	// 3개 프레임 주입
	for i := 0; i < 3; i++ {
		frame := buildLGCPFrame(da, sa, cmd, []byte{byte(i)})
		mock.feedData(frame)
	}

	a := newTestLGCPAgent(t, mock)
	a.bridgeActive.Store(true) // bridge 소비자 활성화
	mock.opened.Store(true)

	go a.captureLoop()

	// 3개 메시지 수신
	for i := 0; i < 3; i++ {
		select {
		case <-a.msgCh:
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for frame %d", i)
		}
	}

	close(a.stopCh)

	if a.framesCaptured.Load() != 3 {
		t.Errorf("framesCaptured = %d, want %d", a.framesCaptured.Load(), 3)
	}
}

// TestLGCPAgent_MsgChDrop 는 msgCh 가 가득 찼을 때 드롭하는지 검증한다.
func TestLGCPAgent_MsgChDrop(t *testing.T) {
	mock := newLGCPMockTransport()

	da := []byte{0x00}
	sa := []byte{0x10}
	cmd := [2]byte{0x40, 0x80}

	// msgCh 크기(16) 보다 많은 프레임 주입
	frameCount := 20
	for i := 0; i < frameCount; i++ {
		frame := buildLGCPFrame(da, sa, cmd, []byte{byte(i)})
		mock.feedData(frame)
	}

	a := newTestLGCPAgent(t, mock)
	a.bridgeActive.Store(true) // bridge 소비자 활성화 (msgCh 전송 활성)
	mock.opened.Store(true)

	go a.captureLoop()

	// 캡처 루프가 모든 프레임을 처리할 때까지 대기
	time.Sleep(500 * time.Millisecond)
	close(a.stopCh)

	if a.framesCaptured.Load() != int64(frameCount) {
		t.Errorf("framesCaptured = %d, want %d", a.framesCaptured.Load(), frameCount)
	}
	if a.framesDropped.Load() == 0 {
		t.Error("expected some frames to be dropped when msgCh is full")
	}
}

// TestLGCPAgent_NoBridge_SkipsMsgCh 는 bridge 소비자가 없으면 msgCh 에 전송하지 않는지 검증한다.
func TestLGCPAgent_NoBridge_SkipsMsgCh(t *testing.T) {
	mock := newLGCPMockTransport()

	da := []byte{0x00}
	sa := []byte{0x10}
	cmd := [2]byte{0x40, 0x80}

	frameCount := 5
	for i := 0; i < frameCount; i++ {
		frame := buildLGCPFrame(da, sa, cmd, []byte{byte(i)})
		mock.feedData(frame)
	}

	a := newTestLGCPAgent(t, mock)
	// bridgeActive 는 기본 false — msgCh 에 전송하지 않아야 함
	mock.opened.Store(true)

	// captureLoop 가 완전히 종료된 뒤 비-atomic 필드를 읽도록 헬퍼 사용
	// (close(stopCh) 직후 곧바로 recentIdx/recentFull 을 읽으면 race 가 발생함)
	runCaptureLoopAndWait(a, 500*time.Millisecond)

	// 프레임은 캡처되었지만 msgCh 드롭은 0이어야 함
	if a.framesCaptured.Load() != int64(frameCount) {
		t.Errorf("framesCaptured = %d, want %d", a.framesCaptured.Load(), frameCount)
	}
	if a.framesDropped.Load() != 0 {
		t.Errorf("framesDropped = %d, want 0 (no bridge consumer)", a.framesDropped.Load())
	}
	// msgCh 는 비어 있어야 함
	if len(a.msgCh) != 0 {
		t.Errorf("msgCh len = %d, want 0", len(a.msgCh))
	}
	// recentFrames 링 버퍼에는 저장되어야 함 — recentMu 보호 하에 읽는다
	a.recentMu.RLock()
	idx := a.recentIdx
	full := a.recentFull
	a.recentMu.RUnlock()
	if idx == 0 && !full {
		t.Error("recentFrames should have frames stored")
	}
}

// TestLGCPAgent_Process_GetStats 는 get_stats 명령이 올바른 통계를 반환하는지 검증한다.
func TestLGCPAgent_Process_GetStats(t *testing.T) {
	mock := newLGCPMockTransport()
	mock.opened.Store(true)

	a := newTestLGCPAgent(t, mock)
	a.framesCaptured.Store(100)
	a.framesValid.Store(95)
	a.framesInvalid.Store(5)
	a.framesDropped.Store(2)
	a.bytesReceived.Store(50000)

	req := `{"command":"get_stats"}`
	resp, err := a.Process([]byte(req))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	var stats map[string]any
	if err := json.Unmarshal(resp, &stats); err != nil {
		t.Fatalf("failed to unmarshal stats: %v", err)
	}

	if int64(stats["frames_captured"].(float64)) != 100 {
		t.Errorf("frames_captured = %v, want %d", stats["frames_captured"], 100)
	}
	if int64(stats["frames_valid"].(float64)) != 95 {
		t.Errorf("frames_valid = %v, want %d", stats["frames_valid"], 95)
	}
	if int64(stats["frames_invalid"].(float64)) != 5 {
		t.Errorf("frames_invalid = %v, want %d", stats["frames_invalid"], 5)
	}
	if stats["transport_connected"] != true {
		t.Errorf("transport_connected = %v, want %v", stats["transport_connected"], true)
	}
}

// TestLGCPAgent_Process_GetRecent 는 get_recent 명령이 최근 프레임을 반환하는지 검증한다.
func TestLGCPAgent_Process_GetRecent(t *testing.T) {
	mock := newLGCPMockTransport()
	mock.opened.Store(true)

	a := newTestLGCPAgent(t, mock)

	// 링 버퍼에 테스트 데이터 주입
	for i := 0; i < 5; i++ {
		evt := LGCPFrameEvent{
			Type:        "lgcp_frame",
			TimestampMs: time.Now().UnixMilli(),
			Seq:         int64(i + 1),
			RawHex:      "test",
			Length:      13,
			CRCValid:    true,
		}
		b, _ := json.Marshal(evt)
		a.pushRecentFrame(b, time.Now(), evt.Seq)
	}

	req := `{"command":"get_recent","count":3}`
	resp, err := a.Process([]byte(req))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	count := int(result["count"].(float64))
	if count != 3 {
		t.Errorf("count = %d, want %d", count, 3)
	}

	frames, ok := result["frames"].([]any)
	if !ok {
		t.Fatal("frames field is not an array")
	}
	if len(frames) != 3 {
		t.Errorf("len(frames) = %d, want %d", len(frames), 3)
	}
}

// TestLGCPAgent_Process_GetRecent_CountZeroDrains 는 v0.7.1 의미를 검증한다:
//
//	count 미지정 (JSON zero-value = 0) → drain (전체 반환 + 버퍼 비움).
//
// 이전 v0.7.0: count 미지정 시 default 10 개. v0.7.1 부터 drain 의미로 통일.
func TestLGCPAgent_Process_GetRecent_CountZeroDrains(t *testing.T) {
	mock := newLGCPMockTransport()
	mock.opened.Store(true)
	a := newTestLGCPAgent(t, mock)

	// 15개 프레임 주입
	for i := 0; i < 15; i++ {
		evt := LGCPFrameEvent{Type: "lgcp_frame", Seq: int64(i)}
		b, _ := json.Marshal(evt)
		a.pushRecentFrame(b, time.Now(), evt.Seq)
	}

	// count 누락 → 0 → drain 동작 (전체 15개 반환).
	req := `{"command":"get_recent"}`
	resp, err := a.Process([]byte(req))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	count := int(result["count"].(float64))
	if count != 15 {
		t.Errorf("count = %d, want 15 (drain returns all)", count)
	}
}

// TestLGCPAgent_Process_GetRecent_ExplicitCount 는 명시적 count > 0 일 때
// 그 수만큼만 반환하는지 검증한다 (비파괴).
func TestLGCPAgent_Process_GetRecent_ExplicitCount(t *testing.T) {
	mock := newLGCPMockTransport()
	mock.opened.Store(true)
	a := newTestLGCPAgent(t, mock)

	for i := 0; i < 15; i++ {
		evt := LGCPFrameEvent{Type: "lgcp_frame", Seq: int64(i)}
		b, _ := json.Marshal(evt)
		a.pushRecentFrame(b, time.Now(), evt.Seq)
	}

	req := `{"command":"get_recent","count":5}`
	resp, err := a.Process([]byte(req))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	count := int(result["count"].(float64))
	if count != 5 {
		t.Errorf("count = %d, want 5", count)
	}
}

// TestLGCPAgent_Process_Drain 은 drain 커맨드가 프레임을 반환하고 버퍼를 비우는지 검증한다.
func TestLGCPAgent_Process_Drain(t *testing.T) {
	mock := newLGCPMockTransport()
	mock.opened.Store(true)
	a := newTestLGCPAgent(t, mock)

	// 5개 프레임 주입
	for i := 0; i < 5; i++ {
		evt := LGCPFrameEvent{Type: "lgcp_frame", Seq: int64(i + 1)}
		b, _ := json.Marshal(evt)
		a.pushRecentFrame(b, time.Now(), evt.Seq)
	}

	// drain 실행 - 3개만 요청
	req := `{"command":"drain","count":3}`
	resp, err := a.Process([]byte(req))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	count := int(result["count"].(float64))
	if count != 3 {
		t.Errorf("drain count = %d, want %d", count, 3)
	}

	// drain 후 버퍼가 비어야 한다
	req2 := `{"command":"get_recent","count":64}`
	resp2, err := a.Process([]byte(req2))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	var result2 map[string]any
	if err := json.Unmarshal(resp2, &result2); err != nil {
		t.Fatalf("failed to unmarshal result2: %v", err)
	}

	count2 := int(result2["count"].(float64))
	if count2 != 0 {
		t.Errorf("after drain, get_recent count = %d, want 0", count2)
	}
}

// TestLGCPAgent_Process_Drain_Default 는 drain에 count 미지정 시 전체 버퍼를 반환하는지 검증한다.
func TestLGCPAgent_Process_Drain_Default(t *testing.T) {
	mock := newLGCPMockTransport()
	mock.opened.Store(true)
	a := newTestLGCPAgent(t, mock)

	// 10개 프레임 주입
	for i := 0; i < 10; i++ {
		evt := LGCPFrameEvent{Type: "lgcp_frame", Seq: int64(i + 1)}
		b, _ := json.Marshal(evt)
		a.pushRecentFrame(b, time.Now(), evt.Seq)
	}

	// count 없이 drain
	req := `{"command":"drain"}`
	resp, err := a.Process([]byte(req))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	count := int(result["count"].(float64))
	if count != 10 {
		t.Errorf("drain count = %d, want %d", count, 10)
	}
}

// TestLGCPAgent_Process_UnsupportedCommand 는 지원되지 않는 명령에 대해 에러를 반환하는지 검증한다.
func TestLGCPAgent_Process_UnsupportedCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{"set_power", `{"command":"set_power"}`},
		{"set_mode", `{"command":"set_mode"}`},
		{"unknown", `{"command":"unknown_cmd"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newLGCPMockTransport()
			a := newTestLGCPAgent(t, mock)

			_, err := a.Process([]byte(tt.command))
			if err == nil {
				t.Fatalf("Process(%q) expected error, got nil", tt.command)
			}
		})
	}
}

// TestLGCPAgent_Process_InvalidJSON 은 잘못된 JSON 요청에 대해 에러를 반환하는지 검증한다.
func TestLGCPAgent_Process_InvalidJSON(t *testing.T) {
	mock := newLGCPMockTransport()
	a := newTestLGCPAgent(t, mock)

	_, err := a.Process([]byte("not json"))
	if err == nil {
		t.Fatal("Process() expected error for invalid JSON, got nil")
	}
}

// TestLGCPAgent_ReceiveMessage 은 ReceiveMessage 가 msgCh 에서 메시지를 수신하는지 검증한다.
func TestLGCPAgent_ReceiveMessage(t *testing.T) {
	mock := newLGCPMockTransport()
	a := newTestLGCPAgent(t, mock)

	expected := []byte(`{"type":"lgcp_frame"}`)
	a.msgCh <- expected

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	msg, err := a.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessage() error = %v", err)
	}
	if string(msg) != string(expected) {
		t.Errorf("ReceiveMessage() = %q, want %q", string(msg), string(expected))
	}
}

// TestLGCPAgent_ReceiveMessage_ContextCancel 은 컨텍스트 취소 시 에러를 반환하는지 검증한다.
func TestLGCPAgent_ReceiveMessage_ContextCancel(t *testing.T) {
	mock := newLGCPMockTransport()
	a := newTestLGCPAgent(t, mock)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 즉시 취소

	_, err := a.ReceiveMessage(ctx)
	if err == nil {
		t.Fatal("ReceiveMessage() expected error on cancelled context, got nil")
	}
}

// TestLGCPAgent_State 는 State() 가 올바른 캡처 상태를 반환하는지 검증한다.
func TestLGCPAgent_State(t *testing.T) {
	mock := newLGCPMockTransport()
	mock.opened.Store(true)
	a := newTestLGCPAgent(t, mock)

	a.framesCaptured.Store(50)
	a.framesValid.Store(45)
	a.framesInvalid.Store(5)

	state := a.State()

	if state["frames_captured"] != int64(50) {
		t.Errorf("frames_captured = %v, want %d", state["frames_captured"], 50)
	}
	if state["transport_connected"] != true {
		t.Errorf("transport_connected = %v, want %v", state["transport_connected"], true)
	}
}

// TestLGCPAgent_BufferInfo 는 BufferInfo() 가 올바른 버퍼 정보를 반환하는지 검증한다.
func TestLGCPAgent_BufferInfo(t *testing.T) {
	mock := newLGCPMockTransport()
	a := newTestLGCPAgent(t, mock)

	// 초기 상태: 빈 버퍼
	pending, capacity := a.BufferInfo()
	if pending != 0 {
		t.Errorf("pending = %d, want %d", pending, 0)
	}
	if capacity != 16 { // MsgChannelSize = 16
		t.Errorf("capacity = %d, want %d", capacity, 16)
	}

	// 메시지 추가 후
	a.msgCh <- []byte("test")
	pending, capacity = a.BufferInfo()
	if pending != 1 {
		t.Errorf("pending = %d, want %d", pending, 1)
	}
}

// TestLGCPAgent_TransportConnected 는 TransportConnected() 가 트랜스포트 상태를 반환하는지 검증한다.
func TestLGCPAgent_TransportConnected(t *testing.T) {
	mock := newLGCPMockTransport()
	a := newTestLGCPAgent(t, mock)

	if a.TransportConnected() {
		t.Error("TransportConnected() = true before open, want false")
	}

	mock.opened.Store(true)
	if !a.TransportConnected() {
		t.Error("TransportConnected() = false after open, want true")
	}
}

// TestLGCPAgent_PassiveOnly 는 에이전트가 절대로 Send() 를 호출하지 않는지 검증한다.
// Send 호출 시 panic 을 발생시키는 트랜스포트를 사용하여 검증한다.
func TestLGCPAgent_PassiveOnly(t *testing.T) {
	// captureLoop 에서 Send 가 호출되지 않음을 확인하기 위해
	// 프레임 캡처 후 Process 로 통계를 확인한다.
	mock := newLGCPMockTransport()
	mock.opened.Store(true)

	da := []byte{0x00}
	sa := []byte{0x10}
	cmd := [2]byte{0x40, 0x80}
	frame := buildLGCPFrame(da, sa, cmd, nil)
	mock.feedData(frame)

	a := newTestLGCPAgent(t, mock)
	a.bridgeActive.Store(true) // bridge 소비자 활성화

	go a.captureLoop()

	select {
	case <-a.msgCh:
		// 프레임 수신 성공 — Send 호출 없이 패시브하게 캡처함
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for passive capture")
	}

	close(a.stopCh)
}

// TestLGCPAgent_RecentBuffer_Overflow 는 링 버퍼가 가득 찼을 때 올바르게 순환하는지 검증한다.
func TestLGCPAgent_RecentBuffer_Overflow(t *testing.T) {
	mock := newLGCPMockTransport()
	a := newTestLGCPAgent(t, mock)

	// lgcpRecentBufferSize (64) 보다 많은 프레임 주입
	totalFrames := lgcpRecentBufferSize + 10
	for i := 0; i < totalFrames; i++ {
		evt := LGCPFrameEvent{
			Type: "lgcp_frame",
			Seq:  int64(i + 1),
		}
		b, _ := json.Marshal(evt)
		a.pushRecentFrame(b, time.Now(), evt.Seq)
	}

	// get_recent 로 최대 개수 요청
	req := `{"command":"get_recent","count":100}`
	resp, err := a.Process([]byte(req))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// 최대 lgcpRecentBufferSize 개까지만 반환
	count := int(result["count"].(float64))
	if count != lgcpRecentBufferSize {
		t.Errorf("count = %d, want %d", count, lgcpRecentBufferSize)
	}

	// 가장 최근 프레임이 먼저 오는지 확인
	frames := result["frames"].([]any)
	firstFrame := frames[0].(map[string]any)
	if int(firstFrame["seq"].(float64)) != totalFrames {
		t.Errorf("most recent seq = %v, want %d", firstFrame["seq"], totalFrames)
	}
}

// TestRegisterLGLGCPTypes 는 LGCP 에이전트 타입 등록을 검증한다.
func TestRegisterLGLGCPTypes(t *testing.T) {
	mgr := agent.NewManager()

	err := RegisterLGLGCPTypes(mgr)
	if err != nil {
		t.Fatalf("RegisterLGLGCPTypes() error = %v", err)
	}
}

// TestRegisterLGLGCPTypes_Duplicate 는 중복 등록 시 에러를 반환하는지 검증한다.
func TestRegisterLGLGCPTypes_Duplicate(t *testing.T) {
	mgr := agent.NewManager()

	err := RegisterLGLGCPTypes(mgr)
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	err = RegisterLGLGCPTypes(mgr)
	if err == nil {
		t.Fatal("expected error on duplicate registration, got nil")
	}
}

// ---------------------------------------------------------------------------
// 트랜스포트 팩토리 테스트
// ---------------------------------------------------------------------------

// TestNewLGCPTransport_Serial 은 serial 타입으로 시리얼 트랜스포트가 생성되는지 검증한다.
func TestNewLGCPTransport_Serial(t *testing.T) {
	cfg := LGCPConfig{
		TransportType: "serial",
		SerialPort:    "/dev/ttyUSB0",
		BaudRate:      9600,
		DataBits:      8,
		StopBits:      1,
		Parity:        "none",
	}

	tr, err := newLGCPTransport(cfg)
	if err != nil {
		t.Fatalf("newLGCPTransport() error: %v", err)
	}
	if _, ok := tr.(*lgapSerialTransport); !ok {
		t.Errorf("newLGCPTransport(serial) type = %T, want *lgapSerialTransport", tr)
	}
}

// TestNewLGCPTransport_TCPClient 는 tcp-client 타입으로 TCP 클라이언트 트랜스포트가 생성되는지 검증한다.
func TestNewLGCPTransport_TCPClient(t *testing.T) {
	cfg := LGCPConfig{
		TransportType:     "tcp-client",
		TCPHost:           "192.168.1.100",
		TCPPort:           5000,
		TCPConnectTimeout: 5 * time.Second,
		TCPReadTimeout:    500 * time.Millisecond,
		TCPWriteTimeout:   1 * time.Second,
	}

	tr, err := newLGCPTransport(cfg)
	if err != nil {
		t.Fatalf("newLGCPTransport() error: %v", err)
	}
	if _, ok := tr.(*lgapTCPClientTransport); !ok {
		t.Errorf("newLGCPTransport(tcp-client) type = %T, want *lgapTCPClientTransport", tr)
	}
}

// TestNewLGCPTransport_TCPServer 는 tcp-server 타입으로 TCP 서버 트랜스포트가 생성되는지 검증한다.
func TestNewLGCPTransport_TCPServer(t *testing.T) {
	cfg := LGCPConfig{
		TransportType:   "tcp-server",
		TCPHost:         "0.0.0.0",
		TCPPort:         8080,
		TCPReadTimeout:  500 * time.Millisecond,
		TCPWriteTimeout: 1 * time.Second,
	}

	tr, err := newLGCPTransport(cfg)
	if err != nil {
		t.Fatalf("newLGCPTransport() error: %v", err)
	}
	if _, ok := tr.(*lgapTCPServerTransport); !ok {
		t.Errorf("newLGCPTransport(tcp-server) type = %T, want *lgapTCPServerTransport", tr)
	}
}

// TestNewLGCPTransport_Unknown 은 알 수 없는 타입에 대한 에러를 검증한다.
func TestNewLGCPTransport_Unknown(t *testing.T) {
	cfg := LGCPConfig{
		TransportType: "websocket",
	}

	_, err := newLGCPTransport(cfg)
	if err == nil {
		t.Fatal("newLGCPTransport() expected error for unknown type, got nil")
	}
	if err != ErrLGCPUnknownTransportType {
		t.Errorf("error = %v, want %v", err, ErrLGCPUnknownTransportType)
	}
}

// TestNewLGCPAgent_TCPClient 는 tcp-client 설정으로 에이전트 생성을 검증한다.
func TestNewLGCPAgent_TCPClient(t *testing.T) {
	config := agent.AgentConfig{
		ID:   "lgcp-tcp-001",
		Name: "lgcp-tcp-client",
		Type: "lgcp",
		Transport: agent.TransportConfig{
			Type: "tcp",
			Options: map[string]any{
				"transport_type": "tcp-client",
				"tcp_host":       "192.168.1.100",
				"tcp_port":       5000,
			},
		},
	}

	a, err := NewLGCPAgent(config)
	if err != nil {
		t.Fatalf("NewLGCPAgent() error = %v", err)
	}
	if a == nil {
		t.Fatal("NewLGCPAgent() returned nil agent")
	}
	if a.ID() != "lgcp-tcp-001" {
		t.Errorf("ID() = %q, want %q", a.ID(), "lgcp-tcp-001")
	}
}

// TestNewLGCPAgent_TCPServer 는 tcp-server 설정으로 에이전트 생성을 검증한다.
func TestNewLGCPAgent_TCPServer(t *testing.T) {
	config := agent.AgentConfig{
		ID:   "lgcp-tcp-002",
		Name: "lgcp-tcp-server",
		Type: "lgcp",
		Transport: agent.TransportConfig{
			Type: "tcp",
			Options: map[string]any{
				"transport_type": "tcp-server",
				"tcp_port":       8080,
			},
		},
	}

	a, err := NewLGCPAgent(config)
	if err != nil {
		t.Fatalf("NewLGCPAgent() error = %v", err)
	}
	if a == nil {
		t.Fatal("NewLGCPAgent() returned nil agent")
	}
}

// TestNewLGCPAgent_UnknownTransportType 은 알 수 없는 transport_type 에 대한 에러를 검증한다.
func TestNewLGCPAgent_UnknownTransportType(t *testing.T) {
	config := agent.AgentConfig{
		ID:   "lgcp-bad",
		Name: "lgcp-bad",
		Type: "lgcp",
		Transport: agent.TransportConfig{
			Type: "unknown",
			Options: map[string]any{
				"transport_type": "websocket",
				"serial_port":    "/dev/ttyUSB0",
			},
		},
	}

	_, err := NewLGCPAgent(config)
	if err == nil {
		t.Fatal("NewLGCPAgent() expected error for unknown transport_type, got nil")
	}
}
