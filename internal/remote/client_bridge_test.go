// client_bridge_test.go 는 P2(노드 측 라이브 브리지)의 게이팅·자동배포·ack·출력 tap·
// 입력 주입·백프레셔·teardown 을 검증한다(@SPEC:SPEC-SUBFLOW-001 그룹 RB, P2,
// REQ-SUBFLOW-RB05/RB06/RB07/RB08/RB10/RB11).
package remote

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 테스트 더블: BridgeFlowRunner / BridgeHandle
// ---------------------------------------------------------------------------

// fakeBridgeHandle 는 BridgeHandle 의 테스트 구현이다. onOutput 콜백을 보관해
// 테스트가 출력 경계 메시지를 시뮬레이션하고, Inject/Close 호출을 기록한다.
type fakeBridgeHandle struct {
	inputs   []string
	outputs  []string
	onOutput BridgeOutputFunc

	mu        sync.Mutex
	injected  []injectedMsg // 주입된 (port,data) 기록
	closed    atomic.Bool
	injectErr error
}

type injectedMsg struct {
	port string
	data json.RawMessage
}

func (h *fakeBridgeHandle) InputPorts() []string  { return h.inputs }
func (h *fakeBridgeHandle) OutputPorts() []string { return h.outputs }

func (h *fakeBridgeHandle) Inject(_ context.Context, port string, data json.RawMessage) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.injectErr != nil {
		return h.injectErr
	}
	h.injected = append(h.injected, injectedMsg{port: port, data: data})
	return nil
}

func (h *fakeBridgeHandle) Close(_ context.Context) error {
	h.closed.Store(true)
	return nil
}

func (h *fakeBridgeHandle) injectedCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.injected)
}

func (h *fakeBridgeHandle) lastInjected() (injectedMsg, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.injected) == 0 {
		return injectedMsg{}, false
	}
	return h.injected[len(h.injected)-1], true
}

// emitOutput 은 출력 경계 포트 메시지를 시뮬레이션한다(엔진 tap 인라인 호출 모사).
func (h *fakeBridgeHandle) emitOutput(port string, data json.RawMessage) {
	if h.onOutput != nil {
		h.onOutput(port, data)
	}
}

// fakeBridgeRunner 는 BridgeFlowRunner 의 테스트 구현이다. OpenBridge 호출을 기록하고
// 미리 설정한 handle/err 을 반환한다.
type fakeBridgeRunner struct {
	mu          sync.Mutex
	openedFlows []string
	handle      *fakeBridgeHandle
	openErr     error
}

func (r *fakeBridgeRunner) OpenBridge(_ context.Context, flowID string, onOutput BridgeOutputFunc) (BridgeHandle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.openedFlows = append(r.openedFlows, flowID)
	if r.openErr != nil {
		return nil, r.openErr
	}
	if r.handle == nil {
		r.handle = &fakeBridgeHandle{inputs: []string{"in"}, outputs: []string{"out"}}
	}
	r.handle.onOutput = onOutput
	return r.handle, nil
}

func (r *fakeBridgeRunner) openCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.openedFlows)
}

// recordingBridgeAudit 는 BridgeAuditSink 의 테스트 구현이다.
type recordingBridgeAudit struct {
	mu      sync.Mutex
	records []bridgeAuditRec
}

type bridgeAuditRec struct {
	event, bridgeID, flowID, detail string
}

func (a *recordingBridgeAudit) RecordBridge(_ context.Context, event, bridgeID, flowID, detail string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.records = append(a.records, bridgeAuditRec{event, bridgeID, flowID, detail})
}

func (a *recordingBridgeAudit) events() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, len(a.records))
	for i, r := range a.records {
		out[i] = r.event
	}
	return out
}

// ---------------------------------------------------------------------------
// 테스트 하네스
// ---------------------------------------------------------------------------

// bridgeClientConfig 는 브리지 테스트용 ClientConfig 빌더이다.
type bridgeClientConfig struct {
	approved bool
	exposure ExposureSummary
	runner   BridgeFlowRunner
	redactor QueryRedactor
	audit    BridgeAuditSink
}

// startBridgeClient 는 구성된 ClientConfig 로 클라이언트를 시작하고 핸드셰이크를 소비한다.
func startBridgeClient(t *testing.T, c bridgeClientConfig) (*Client, *clientFakeConn, context.CancelFunc) {
	t.Helper()
	conn := newClientFakeConn()
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		return conn, nil
	})
	dir := t.TempDir()
	if c.approved {
		require.NoError(t, SaveNodeToken(dir, "node-token-b"))
	}

	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example/api/remote/ws",
		InstanceID:        "node-b",
		HeartbeatInterval: time.Hour,
		DataDir:           dir,
		Exposure:          c.exposure,
		BridgeRunner:      c.runner,
		BridgeAudit:       c.audit,
		QueryRedactor:     c.redactor,
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	cli.Start(ctx)

	select {
	case <-conn.fromClient: // 핸드셰이크(hello/register) 소비
	case <-time.After(time.Second):
		t.Fatal("핸드셰이크 송신 타임아웃")
	}
	return cli, conn, cancel
}

func injectBridgeOpen(t *testing.T, conn *clientFakeConn, p BridgeOpenPayload) {
	t.Helper()
	msg, err := NewBridgeOpenMessage(p)
	require.NoError(t, err)
	data, err := msg.Encode()
	require.NoError(t, err)
	conn.toClient <- data
}

func injectBridgeInput(t *testing.T, conn *clientFakeConn, p BridgeInputPayload) {
	t.Helper()
	msg, err := NewBridgeInputMessage(p)
	require.NoError(t, err)
	data, err := msg.Encode()
	require.NoError(t, err)
	conn.toClient <- data
}

func injectBridgeClose(t *testing.T, conn *clientFakeConn, p BridgeClosePayload) {
	t.Helper()
	msg, err := NewBridgeCloseMessage(p)
	require.NoError(t, err)
	data, err := msg.Encode()
	require.NoError(t, err)
	conn.toClient <- data
}

// readBridgeAck 는 클라이언트가 보낸 다음 bridge_open_ack 를 읽는다.
func readBridgeAck(t *testing.T, conn *clientFakeConn) BridgeOpenAckPayload {
	t.Helper()
	select {
	case data := <-conn.fromClient:
		msg, err := DecodeMessage(data)
		require.NoError(t, err)
		require.Equal(t, TypeBridgeOpenAck, msg.Type)
		var ack BridgeOpenAckPayload
		require.NoError(t, json.Unmarshal(msg.Payload, &ack))
		return ack
	case <-time.After(time.Second):
		t.Fatal("bridge_open_ack 수신 타임아웃")
		return BridgeOpenAckPayload{}
	}
}

// readBridgeOutput 는 클라이언트가 보낸 다음 bridge_output 를 읽는다.
func readBridgeOutput(t *testing.T, conn *clientFakeConn) BridgeOutputPayload {
	t.Helper()
	select {
	case data := <-conn.fromClient:
		msg, err := DecodeMessage(data)
		require.NoError(t, err)
		require.Equal(t, TypeBridgeOutput, msg.Type)
		var out BridgeOutputPayload
		require.NoError(t, json.Unmarshal(msg.Payload, &out))
		return out
	case <-time.After(time.Second):
		t.Fatal("bridge_output 수신 타임아웃")
		return BridgeOutputPayload{}
	}
}

// ---------------------------------------------------------------------------
// 게이팅 — 미승인/범위 밖 → ack ok:false (REQ-SUBFLOW-RB08/RB11)
// ---------------------------------------------------------------------------

// TestBridge_Unapproved_RejectsAck 는 미승인 노드의 bridge_open 을 거부함을 검증한다.
func TestBridge_Unapproved_RejectsAck(t *testing.T) {
	runner := &fakeBridgeRunner{}
	audit := &recordingBridgeAudit{}
	_, conn, cancel := startBridgeClient(t, bridgeClientConfig{
		approved: false,
		exposure: ExposureSummary{Flows: ExposeAll},
		runner:   runner,
		audit:    audit,
	})
	defer cancel()

	injectBridgeOpen(t, conn, BridgeOpenPayload{
		BridgeID: "b1", InstanceID: "node-b", RemoteFlowID: "flow-x",
	})

	ack := readBridgeAck(t, conn)
	assert.Equal(t, "b1", ack.BridgeID)
	assert.False(t, ack.OK)
	assert.NotEmpty(t, ack.Error)
	assert.Equal(t, 0, runner.openCount(), "미승인은 runner 를 호출하지 않아야 함")
	assert.Contains(t, audit.events(), "open_rejected")
}

// TestBridge_OutOfScope_RejectsAck 는 노출 범위 밖 참조 플로우를 거부함을 검증한다.
func TestBridge_OutOfScope_RejectsAck(t *testing.T) {
	runner := &fakeBridgeRunner{}
	_, conn, cancel := startBridgeClient(t, bridgeClientConfig{
		approved: true,
		exposure: ExposureSummary{Flows: "flow-allowed"}, // flow-x 비노출
		runner:   runner,
	})
	defer cancel()

	injectBridgeOpen(t, conn, BridgeOpenPayload{
		BridgeID: "b2", InstanceID: "node-b", RemoteFlowID: "flow-x",
	})

	ack := readBridgeAck(t, conn)
	assert.False(t, ack.OK)
	assert.Equal(t, 0, runner.openCount(), "범위 밖은 runner 를 호출하지 않아야 함")
}

// TestBridge_NoRunner_RejectsAck 는 runner 미구성 노드의 거부를 검증한다.
func TestBridge_NoRunner_RejectsAck(t *testing.T) {
	_, conn, cancel := startBridgeClient(t, bridgeClientConfig{
		approved: true,
		exposure: ExposureSummary{Flows: ExposeAll},
		runner:   nil,
	})
	defer cancel()

	injectBridgeOpen(t, conn, BridgeOpenPayload{
		BridgeID: "b3", InstanceID: "node-b", RemoteFlowID: "flow-x",
	})
	ack := readBridgeAck(t, conn)
	assert.False(t, ack.OK)
	assert.NotEmpty(t, ack.Error)
}

// ---------------------------------------------------------------------------
// 해피 패스 — 자동배포 + ack(ports) (REQ-SUBFLOW-RB05/RB06)
// ---------------------------------------------------------------------------

// TestBridge_Open_AutoDeploysAndAcksPorts 는 승인+범위 내 open 이 runner 로 자동배포하고
// 실제 경계 포트를 ack 로 보고함을 검증한다.
func TestBridge_Open_AutoDeploysAndAcksPorts(t *testing.T) {
	runner := &fakeBridgeRunner{
		handle: &fakeBridgeHandle{inputs: []string{"cmd"}, outputs: []string{"result", "log"}},
	}
	audit := &recordingBridgeAudit{}
	_, conn, cancel := startBridgeClient(t, bridgeClientConfig{
		approved: true,
		exposure: ExposureSummary{Flows: ExposeAll},
		runner:   runner,
		audit:    audit,
	})
	defer cancel()

	injectBridgeOpen(t, conn, BridgeOpenPayload{
		BridgeID: "b4", InstanceID: "node-b", RemoteFlowID: "flow-y",
	})

	ack := readBridgeAck(t, conn)
	assert.True(t, ack.OK)
	assert.Equal(t, "b4", ack.BridgeID)
	assert.Equal(t, []string{"cmd"}, ack.InputPorts)
	assert.Equal(t, []string{"result", "log"}, ack.OutputPorts)
	assert.Equal(t, 1, runner.openCount())
	assert.Equal(t, "flow-y", runner.openedFlows[0])
	assert.Contains(t, audit.events(), "open")
}

// TestBridge_Open_RunnerError_RejectsAck 는 runner 실행 실패가 ack ok:false 로 매핑됨을
// 검증한다(REQ-SUBFLOW-RB05 — 502 계열).
func TestBridge_Open_RunnerError_RejectsAck(t *testing.T) {
	runner := &fakeBridgeRunner{openErr: assertError("deploy failed")}
	_, conn, cancel := startBridgeClient(t, bridgeClientConfig{
		approved: true,
		exposure: ExposureSummary{Flows: ExposeAll},
		runner:   runner,
	})
	defer cancel()

	injectBridgeOpen(t, conn, BridgeOpenPayload{
		BridgeID: "b5", InstanceID: "node-b", RemoteFlowID: "flow-z",
	})
	ack := readBridgeAck(t, conn)
	assert.False(t, ack.OK)
	assert.Contains(t, ack.Error, "deploy failed")
}

// ---------------------------------------------------------------------------
// 출력 tap → bridge_output (+ redaction) (REQ-SUBFLOW-RB07)
// ---------------------------------------------------------------------------

// TestBridge_Output_ForwardsRedacted 는 출력 경계 메시지가 redaction 후 bridge_output
// 로 전송됨을 검증한다.
func TestBridge_Output_ForwardsRedacted(t *testing.T) {
	handle := &fakeBridgeHandle{inputs: []string{"in"}, outputs: []string{"out"}}
	runner := &fakeBridgeRunner{handle: handle}
	redactor := QueryRedactorFunc(func(_ json.RawMessage) json.RawMessage {
		return json.RawMessage(`{"secret":"***"}`)
	})
	_, conn, cancel := startBridgeClient(t, bridgeClientConfig{
		approved: true,
		exposure: ExposureSummary{Flows: ExposeAll},
		runner:   runner,
		redactor: redactor,
	})
	defer cancel()

	injectBridgeOpen(t, conn, BridgeOpenPayload{BridgeID: "b6", RemoteFlowID: "flow-y"})
	_ = readBridgeAck(t, conn)

	handle.emitOutput("out", json.RawMessage(`{"secret":"hunter2"}`))

	out := readBridgeOutput(t, conn)
	assert.Equal(t, "b6", out.BridgeID)
	assert.Equal(t, "out", out.Port)
	assert.JSONEq(t, `{"secret":"***"}`, string(out.Data))
}

// ---------------------------------------------------------------------------
// 입력 주입 (REQ-SUBFLOW-RB07 — WRITE)
// ---------------------------------------------------------------------------

// TestBridge_Input_InjectsIntoBoundary 는 bridge_input 이 입력 경계 포트로 주입됨을
// 검증하고, 입력 감사가 기록됨을 확인한다.
func TestBridge_Input_InjectsIntoBoundary(t *testing.T) {
	handle := &fakeBridgeHandle{inputs: []string{"cmd"}, outputs: []string{"out"}}
	runner := &fakeBridgeRunner{handle: handle}
	audit := &recordingBridgeAudit{}
	_, conn, cancel := startBridgeClient(t, bridgeClientConfig{
		approved: true,
		exposure: ExposureSummary{Flows: ExposeAll},
		runner:   runner,
		audit:    audit,
	})
	defer cancel()

	injectBridgeOpen(t, conn, BridgeOpenPayload{BridgeID: "b7", RemoteFlowID: "flow-y"})
	_ = readBridgeAck(t, conn)

	injectBridgeInput(t, conn, BridgeInputPayload{
		BridgeID: "b7", Port: "cmd", Data: json.RawMessage(`{"v":1}`),
	})

	require.Eventually(t, func() bool { return handle.injectedCount() == 1 },
		time.Second, 5*time.Millisecond)
	last, ok := handle.lastInjected()
	require.True(t, ok)
	assert.Equal(t, "cmd", last.port)
	assert.JSONEq(t, `{"v":1}`, string(last.data))
	assert.Contains(t, audit.events(), "input")
}

// TestBridge_Input_UnknownBridge_NoInject 는 미지의 bridge_id 입력이 무시됨을 검증한다.
func TestBridge_Input_UnknownBridge_NoInject(t *testing.T) {
	handle := &fakeBridgeHandle{inputs: []string{"cmd"}, outputs: []string{"out"}}
	runner := &fakeBridgeRunner{handle: handle}
	_, conn, cancel := startBridgeClient(t, bridgeClientConfig{
		approved: true,
		exposure: ExposureSummary{Flows: ExposeAll},
		runner:   runner,
	})
	defer cancel()

	injectBridgeOpen(t, conn, BridgeOpenPayload{BridgeID: "b8", RemoteFlowID: "flow-y"})
	_ = readBridgeAck(t, conn)

	// 다른 bridge_id 로 입력 — 무시되어야 함.
	injectBridgeInput(t, conn, BridgeInputPayload{
		BridgeID: "unknown", Port: "cmd", Data: json.RawMessage(`{"v":1}`),
	})
	// 짧게 대기 후 주입이 없었는지 확인.
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 0, handle.injectedCount())
}

// ---------------------------------------------------------------------------
// 백프레셔 — 느린 WS 가 엔진(onOutput)을 막지 않음 (REQ-SUBFLOW-RB10)
// ---------------------------------------------------------------------------

// TestBridge_Output_Backpressure_NonBlocking 는 WS 소비자가 멈춰도 onOutput 이
// 블로킹되지 않음을 검증한다(버퍼+oldest-drop). onOutput 다수 호출이 즉시 반환되어야 한다.
func TestBridge_Output_Backpressure_NonBlocking(t *testing.T) {
	handle := &fakeBridgeHandle{inputs: []string{"in"}, outputs: []string{"out"}}
	runner := &fakeBridgeRunner{handle: handle}
	_, conn, cancel := startBridgeClient(t, bridgeClientConfig{
		approved: true,
		exposure: ExposureSummary{Flows: ExposeAll},
		runner:   runner,
	})
	defer cancel()

	injectBridgeOpen(t, conn, BridgeOpenPayload{BridgeID: "b9", RemoteFlowID: "flow-y"})
	_ = readBridgeAck(t, conn)

	// WS 소비자를 멈춘 채(fromClient 를 읽지 않음) onOutput 을 많이 호출한다.
	// 버퍼 cap(32) + ack 1건으로 fromClient(cap 32)가 곧 가득 차지만 onOutput 은
	// 블로킹되지 않아야 한다(별도 펌프 + bounded buffer + oldest-drop).
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			handle.emitOutput("out", json.RawMessage(`{"i":1}`))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("onOutput 이 블로킹됨 — 백프레셔 실패(엔진을 막음)")
	}
}

// ---------------------------------------------------------------------------
// Teardown — bridge_close / 세션 종료 → 정지 + 누수 없음 (REQ-SUBFLOW-RB09)
// ---------------------------------------------------------------------------

// TestBridge_Close_TearsDownAndStops 는 bridge_close 가 handle.Close 를 호출(자동 정지
// 위임)하고 감사함을 검증한다.
func TestBridge_Close_TearsDownAndStops(t *testing.T) {
	handle := &fakeBridgeHandle{inputs: []string{"in"}, outputs: []string{"out"}}
	runner := &fakeBridgeRunner{handle: handle}
	audit := &recordingBridgeAudit{}
	_, conn, cancel := startBridgeClient(t, bridgeClientConfig{
		approved: true,
		exposure: ExposureSummary{Flows: ExposeAll},
		runner:   runner,
		audit:    audit,
	})
	defer cancel()

	injectBridgeOpen(t, conn, BridgeOpenPayload{BridgeID: "b10", RemoteFlowID: "flow-y"})
	_ = readBridgeAck(t, conn)

	injectBridgeClose(t, conn, BridgeClosePayload{BridgeID: "b10", Reason: "undeploy"})

	require.Eventually(t, func() bool { return handle.closed.Load() },
		time.Second, 5*time.Millisecond)
	assert.Contains(t, audit.events(), "close")
}

// TestBridge_SessionEnd_TearsDownAllBridges 는 세션 종료(연결 끊김)가 모든 활성 브리지를
// teardown 함을 검증한다(누수 없음 — REQ-SUBFLOW-RB09).
func TestBridge_SessionEnd_TearsDownAllBridges(t *testing.T) {
	handle := &fakeBridgeHandle{inputs: []string{"in"}, outputs: []string{"out"}}
	runner := &fakeBridgeRunner{handle: handle}
	cli, conn, cancel := startBridgeClient(t, bridgeClientConfig{
		approved: true,
		exposure: ExposureSummary{Flows: ExposeAll},
		runner:   runner,
	})
	defer cancel()

	injectBridgeOpen(t, conn, BridgeOpenPayload{BridgeID: "b11", RemoteFlowID: "flow-y"})
	_ = readBridgeAck(t, conn)

	// 세션 종료(클라이언트 정지) → 모든 브리지 teardown.
	cli.Stop()

	require.Eventually(t, func() bool { return handle.closed.Load() },
		2*time.Second, 5*time.Millisecond)
}

// TestBridge_MultipleBridges_Independent 는 서로 다른 bridge_id 의 다중 브리지가 독립
// 동작함을 검증한다(REQ-SUBFLOW-RB12 — flow-node 별 독립 bridge_id).
func TestBridge_MultipleBridges_Independent(t *testing.T) {
	// runner 가 매 open 마다 새 handle 을 주도록 별도 runner 를 구성한다.
	r := &multiBridgeRunner{}
	_, conn, cancel := startBridgeClient(t, bridgeClientConfig{
		approved: true,
		exposure: ExposureSummary{Flows: ExposeAll},
		runner:   r,
	})
	defer cancel()

	injectBridgeOpen(t, conn, BridgeOpenPayload{BridgeID: "bm1", RemoteFlowID: "flow-1"})
	ack1 := readBridgeAck(t, conn)
	injectBridgeOpen(t, conn, BridgeOpenPayload{BridgeID: "bm2", RemoteFlowID: "flow-2"})
	ack2 := readBridgeAck(t, conn)

	require.True(t, ack1.OK)
	require.True(t, ack2.OK)

	// bm1 만 닫는다 — bm2 의 handle 은 영향을 받지 않아야 한다.
	injectBridgeClose(t, conn, BridgeClosePayload{BridgeID: "bm1"})

	h1 := r.handleFor("flow-1")
	h2 := r.handleFor("flow-2")
	require.NotNil(t, h1)
	require.NotNil(t, h2)
	require.Eventually(t, func() bool { return h1.closed.Load() },
		time.Second, 5*time.Millisecond)
	assert.False(t, h2.closed.Load(), "bm2 는 영향받지 않아야 함")
}

// multiBridgeRunner 는 flowID 별로 별도 handle 을 발급하는 테스트 runner 이다.
type multiBridgeRunner struct {
	mu      sync.Mutex
	handles map[string]*fakeBridgeHandle
}

func (r *multiBridgeRunner) OpenBridge(_ context.Context, flowID string, onOutput BridgeOutputFunc) (BridgeHandle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.handles == nil {
		r.handles = make(map[string]*fakeBridgeHandle)
	}
	h := &fakeBridgeHandle{inputs: []string{"in"}, outputs: []string{"out"}, onOutput: onOutput}
	r.handles[flowID] = h
	return h, nil
}

func (r *multiBridgeRunner) handleFor(flowID string) *fakeBridgeHandle {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.handles[flowID]
}
