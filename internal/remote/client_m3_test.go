// client_m3_test.go 는 M3 클라이언트 측 명령 수신→적용→결과 반환을 검증한다
// (@SPEC:SPEC-REMOTE-001 M3, REQ-D02/D03/D04/D05/D08/D09).
//
// clientFakeConn 은 client_test.go 에 정의된 것을 재사용한다.
package remote

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingApplier 는 CommandApplier 의 테스트 구현이다.
type recordingApplier struct {
	lastDomain string
	lastAction string
	result     json.RawMessage
	err        error
}

func (r *recordingApplier) Apply(_ context.Context, domain, action string, _ json.RawMessage) (json.RawMessage, error) {
	r.lastDomain = domain
	r.lastAction = action
	if r.err != nil {
		return nil, r.err
	}
	return r.result, nil
}

// startApprovedClient 는 영속 토큰을 가진(승인 상태) 클라이언트를 dial 시키고, 서버
// 측 clientFakeConn 을 반환한다. applier 가 주입된다.
func startApprovedClient(t *testing.T, applier CommandApplier) (*Client, *clientFakeConn, context.CancelFunc) {
	t.Helper()
	conn := newClientFakeConn()
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		return conn, nil
	})

	dir := t.TempDir()
	// 승인 상태를 모사하기 위해 노드 토큰을 사전 영속한다(hasToken()=true).
	require.NoError(t, SaveNodeToken(dir, "node-token-xyz"))

	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example/api/remote/ws",
		InstanceID:        "node-m3",
		HeartbeatInterval: time.Hour, // heartbeat 간섭 방지.
		DataDir:           dir,
		Applier:           applier,
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	cli.Start(ctx)

	// 토큰 보유 → 첫 메시지는 hello(세션 복원 경로). 소비하여 채널을 비운다.
	select {
	case <-conn.fromClient:
	case <-time.After(time.Second):
		t.Fatal("핸드셰이크(hello) 송신 타임아웃")
	}
	return cli, conn, cancel
}

// readResult 는 클라이언트가 서버로 보낸 다음 command_result 를 읽는다.
func readResult(t *testing.T, conn *clientFakeConn) CommandResultPayload {
	t.Helper()
	select {
	case data := <-conn.fromClient:
		msg, err := DecodeMessage(data)
		require.NoError(t, err)
		require.Equal(t, TypeCommandResult, msg.Type)
		var res CommandResultPayload
		require.NoError(t, json.Unmarshal(msg.Payload, &res))
		return res
	case <-time.After(2 * time.Second):
		t.Fatal("command_result 수신 타임아웃")
		return CommandResultPayload{}
	}
}

// injectCommand 는 서버가 클라이언트로 보내는 command 를 주입한다.
func injectCommand(t *testing.T, conn *clientFakeConn, cmd CommandPayload) {
	t.Helper()
	msg, err := NewCommandMessage(cmd)
	require.NoError(t, err)
	data, err := msg.Encode()
	require.NoError(t, err)
	conn.toClient <- data
}

// TestClient_AppliesCommandAndReturnsResult 는 승인 노드가 command 를 적용하고 결과를
// 같은 연결로 반환하는지 검증한다(REQ-D02/D05).
func TestClient_AppliesCommandAndReturnsResult(t *testing.T) {
	applier := &recordingApplier{result: json.RawMessage(`{"deployed":true}`)}
	cli, conn, cancel := startApprovedClient(t, applier)
	defer cancel()
	defer cli.Stop()

	injectCommand(t, conn, CommandPayload{
		CommandID: "cmd-1",
		Domain:    DomainFlow,
		Action:    "deploy",
		Args:      json.RawMessage(`{"id":"f1"}`),
	})

	res := readResult(t, conn)
	assert.Equal(t, "cmd-1", res.CommandID)
	assert.True(t, res.OK)
	assert.JSONEq(t, `{"deployed":true}`, string(res.Result))
	assert.Equal(t, DomainFlow, applier.lastDomain)
	assert.Equal(t, "deploy", applier.lastAction)
}

// TestClient_ApplyFailureReturnsError 는 적용 실패가 command_result 오류로 반환되는지
// 검증한다(REQ-D09 — 부분 적용 없음).
func TestClient_ApplyFailureReturnsError(t *testing.T) {
	applier := &recordingApplier{err: errors.New("flow not found")}
	cli, conn, cancel := startApprovedClient(t, applier)
	defer cancel()
	defer cli.Stop()

	injectCommand(t, conn, CommandPayload{
		CommandID: "cmd-2",
		Domain:    DomainAgent,
		Action:    "start",
	})

	res := readResult(t, conn)
	assert.Equal(t, "cmd-2", res.CommandID)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "flow not found")
}

// TestClient_UnapprovedRejectsCommand 는 미승인(토큰 미보유) 노드가 command 를
// 거부하는지 검증한다(REQ-D08).
func TestClient_UnapprovedRejectsCommand(t *testing.T) {
	conn := newClientFakeConn()
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		return conn, nil
	})
	applier := &recordingApplier{result: json.RawMessage(`"should-not-apply"`)}
	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example/api/remote/ws",
		InstanceID:        "node-unapproved",
		HeartbeatInterval: time.Hour,
		Applier:           applier,
		// DataDir 없음 → 토큰 미보유 → 미승인.
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cli.Start(ctx)
	defer cli.Stop()

	// 토큰 미보유 → 첫 메시지는 register. 소비한다.
	select {
	case <-conn.fromClient:
	case <-time.After(time.Second):
		t.Fatal("register 송신 타임아웃")
	}

	injectCommand(t, conn, CommandPayload{
		CommandID: "cmd-3",
		Domain:    DomainFlow,
		Action:    "delete",
	})

	res := readResult(t, conn)
	assert.False(t, res.OK, "미승인 노드는 명령을 거부해야 함")
	assert.Contains(t, res.Error, "not approved")
	assert.Empty(t, applier.lastAction, "거부된 명령은 어댑터를 호출하지 않아야 함")
}

// TestClient_NilApplierRejectsCommand 는 applier 미구성 시 명령이 거부되는지
// 검증한다.
func TestClient_NilApplierRejectsCommand(t *testing.T) {
	cli, conn, cancel := startApprovedClient(t, nil) // applier=nil
	defer cancel()
	defer cli.Stop()

	injectCommand(t, conn, CommandPayload{
		CommandID: "cmd-4",
		Domain:    DomainFlow,
		Action:    "start",
	})

	res := readResult(t, conn)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "applier")
}

// panicApplier 는 Apply 에서 panic 을 발생시키는 CommandApplier 테스트 구현이다.
// 신뢰 경계(서버 명령이 프로세스 경계를 넘어 적용됨)에서 panic 복구 가드를 검증한다.
type panicApplier struct {
	panicValue any
}

func (p *panicApplier) Apply(_ context.Context, _, _ string, _ json.RawMessage) (json.RawMessage, error) {
	panic(p.panicValue)
}

// TestClient_ApplierPanicRecovered 는 Applier.Apply panic 시 (1) 고루틴/데몬이 죽지
// 않고 (2) command_result{ok:false} 가 같은 연결로 반환되며 (3) panic 값이 verbatim
// 으로 외부에 노출되지 않는지 검증한다(신뢰 경계 로버스트니스, REQ-D09 정신).
func TestClient_ApplierPanicRecovered(t *testing.T) {
	applier := &panicApplier{panicValue: "secret-token=abcdef nil deref"}
	cli, conn, cancel := startApprovedClient(t, applier)
	defer cancel()
	defer cli.Stop()

	injectCommand(t, conn, CommandPayload{
		CommandID: "cmd-panic",
		Domain:    DomainFlow,
		Action:    "deploy",
		Args:      json.RawMessage(`{"id":"f1"}`),
	})

	// (1)(2) panic 이 복구되어 ok=false 결과가 반환되어야 한다(고루틴 생존 → 결과 수신).
	res := readResult(t, conn)
	assert.Equal(t, "cmd-panic", res.CommandID)
	assert.False(t, res.OK, "panic 은 ok=false 로 변환되어야 함")
	assert.NotEmpty(t, res.Error, "오류 메시지가 있어야 함")

	// (3) panic 값(시크릿 가능성)이 외부 응답에 verbatim 노출되지 않아야 한다(REQ-F06).
	assert.NotContains(t, res.Error, "secret-token=abcdef",
		"panic 값/시크릿은 외부에 노출되지 않아야 함")

	// 세션이 살아 있어 후속 명령이 정상 처리되는지 확인(고루틴/세션 미손상).
	okApplier := &panicApplier{} // 사용 안 함; 후속은 동일 client 의 nil-패닉 아님 경로.
	_ = okApplier
	injectCommand(t, conn, CommandPayload{
		CommandID: "cmd-after-panic",
		Domain:    DomainFlow,
		Action:    "deploy",
		Args:      json.RawMessage(`{"id":"f2"}`),
	})
	res2 := readResult(t, conn)
	assert.Equal(t, "cmd-after-panic", res2.CommandID,
		"panic 이후에도 세션이 계속 명령을 처리해야 함")
	assert.False(t, res2.OK, "동일 panicApplier 이므로 다시 ok=false")
}

// TestClient_ApplierPanicWithErrorValue 는 panic 값이 error 타입일 때도 복구되어
// 일반화된 오류로 변환되는지 검증한다.
func TestClient_ApplierPanicWithErrorValue(t *testing.T) {
	applier := &panicApplier{panicValue: errors.New("runtime failure")}
	cli, conn, cancel := startApprovedClient(t, applier)
	defer cancel()
	defer cli.Stop()

	injectCommand(t, conn, CommandPayload{
		CommandID: "cmd-panic-err",
		Domain:    DomainAgent,
		Action:    "start",
	})

	res := readResult(t, conn)
	assert.Equal(t, "cmd-panic-err", res.CommandID)
	assert.False(t, res.OK)
	assert.NotEmpty(t, res.Error)
}

// TestClient_MalformedCommandIgnored 는 command_id 누락/디코드 실패 명령이 세션을
// 중단시키지 않는지 검증한다.
func TestClient_MalformedCommandIgnored(t *testing.T) {
	applier := &recordingApplier{result: json.RawMessage(`"ok"`)}
	cli, conn, cancel := startApprovedClient(t, applier)
	defer cancel()
	defer cli.Stop()

	// command_id 누락 → 무시(결과 없음).
	injectCommand(t, conn, CommandPayload{Domain: DomainFlow, Action: "start"})

	// 이어서 정상 명령 → 정상 결과. malformed 가 루프를 중단시키지 않았음을 확인.
	injectCommand(t, conn, CommandPayload{
		CommandID: "cmd-5", Domain: DomainFlow, Action: "start",
	})
	res := readResult(t, conn)
	assert.Equal(t, "cmd-5", res.CommandID)
	assert.True(t, res.OK)
}
