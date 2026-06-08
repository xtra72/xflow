// client_m8_query_test.go 는 M8(그룹 J) 노드 측 query 수신→로컬 read 매핑→redaction→
// query_result 반환을 검증한다(@SPEC:SPEC-REMOTE-001 M8, REQ-J01/J03/J05/J06).
//
// clientFakeConn / startApprovedClient 헬퍼는 client_test.go / client_m3_test.go 에
// 정의된 것을 재사용한다.
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

// recordingQuerySource 는 QuerySource 의 테스트 구현이다.
type recordingQuerySource struct {
	lastDomain string
	lastAction string
	data       json.RawMessage
	err        error
}

func (r *recordingQuerySource) Query(_ context.Context, domain, action string, _ json.RawMessage) (json.RawMessage, error) {
	r.lastDomain = domain
	r.lastAction = action
	if r.err != nil {
		return nil, r.err
	}
	return r.data, nil
}

// startApprovedQueryClient 는 승인 상태 클라이언트에 QuerySource(+선택 redactor)를
// 주입하여 dial 시키고 서버 측 fake conn 을 반환한다.
func startApprovedQueryClient(t *testing.T, src QuerySource, redactor QueryRedactor) (*Client, *clientFakeConn, context.CancelFunc) {
	t.Helper()
	conn := newClientFakeConn()
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		return conn, nil
	})
	dir := t.TempDir()
	require.NoError(t, SaveNodeToken(dir, "node-token-q"))

	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example/api/remote/ws",
		InstanceID:        "node-q",
		HeartbeatInterval: time.Hour,
		DataDir:           dir,
		QuerySource:       src,
		QueryRedactor:     redactor,
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	cli.Start(ctx)

	// 토큰 보유 → 첫 메시지는 hello. 소비한다.
	select {
	case <-conn.fromClient:
	case <-time.After(time.Second):
		t.Fatal("핸드셰이크(hello) 송신 타임아웃")
	}
	return cli, conn, cancel
}

// injectQuery 는 서버가 클라이언트로 보내는 query 를 주입한다.
func injectQuery(t *testing.T, conn *clientFakeConn, q QueryPayload) {
	t.Helper()
	msg, err := NewQueryMessage(q)
	require.NoError(t, err)
	data, err := msg.Encode()
	require.NoError(t, err)
	conn.toClient <- data
}

// readQueryResult 는 클라이언트가 보낸 다음 query_result 를 읽는다.
func readQueryResult(t *testing.T, conn *clientFakeConn) QueryResultPayload {
	t.Helper()
	select {
	case data := <-conn.fromClient:
		msg, err := DecodeMessage(data)
		require.NoError(t, err)
		require.Equal(t, TypeQueryResult, msg.Type)
		var res QueryResultPayload
		require.NoError(t, json.Unmarshal(msg.Payload, &res))
		return res
	case <-time.After(2 * time.Second):
		t.Fatal("query_result 수신 타임아웃")
		return QueryResultPayload{}
	}
}

// TestClient_QueryDispatchReturnsData 는 승인 노드가 query 를 로컬 read 소스로
// 디스패치하고 결과를 같은 연결로 반환하는지 검증한다(REQ-J01/J02).
func TestClient_QueryDispatchReturnsData(t *testing.T) {
	src := &recordingQuerySource{data: json.RawMessage(`{"status":"running"}`)}
	cli, conn, cancel := startApprovedQueryClient(t, src, nil)
	defer cancel()
	defer cli.Stop()

	injectQuery(t, conn, QueryPayload{
		QueryID:     "q-1",
		Domain:      DomainFlow,
		QueryAction: QueryActionStatus,
		Args:        json.RawMessage(`{"id":"f1"}`),
	})

	res := readQueryResult(t, conn)
	assert.Equal(t, "q-1", res.QueryID)
	assert.True(t, res.OK)
	assert.JSONEq(t, `{"status":"running"}`, string(res.Data))
	assert.Equal(t, DomainFlow, src.lastDomain)
	assert.Equal(t, QueryActionStatus, src.lastAction)
}

// TestClient_QueryRedactsBeforeSend 는 query_result 본문이 전송 전 redaction 되는지
// 검증한다(REQ-J06 — 노드가 전송 전 마스킹).
func TestClient_QueryRedactsBeforeSend(t *testing.T) {
	src := &recordingQuerySource{data: json.RawMessage(`{"password":"hunter2"}`)}
	// redactor 는 본문을 마스킹된 값으로 치환한다(실제 정책 모사).
	redactor := QueryRedactorFunc(func(_ json.RawMessage) json.RawMessage {
		return json.RawMessage(`{"password":"***"}`)
	})
	cli, conn, cancel := startApprovedQueryClient(t, src, redactor)
	defer cancel()
	defer cli.Stop()

	injectQuery(t, conn, QueryPayload{
		QueryID:     "q-redact",
		Domain:      DomainAgent,
		QueryAction: QueryActionGet,
		Args:        json.RawMessage(`{"id":"a1"}`),
	})

	res := readQueryResult(t, conn)
	assert.True(t, res.OK)
	assert.JSONEq(t, `{"password":"***"}`, string(res.Data))
	assert.NotContains(t, string(res.Data), "hunter2", "시크릿은 와이어로 전송되면 안 됨")
}

// TestClient_QueryUnknownActionRejected 는 미열거(또는 변경 의미) query-action 이
// 패닉 없이 오류로 거부되는지 검증한다(REQ-J03/J04).
func TestClient_QueryUnknownActionRejected(t *testing.T) {
	src := &recordingQuerySource{data: json.RawMessage(`"should-not-run"`)}
	cli, conn, cancel := startApprovedQueryClient(t, src, nil)
	defer cancel()
	defer cli.Stop()

	// 변경 의미 action(delete) → READ-ONLY 위반으로 거부, 소스 미호출.
	injectQuery(t, conn, QueryPayload{
		QueryID:     "q-mut",
		Domain:      DomainFlow,
		QueryAction: ActionDelete,
	})

	res := readQueryResult(t, conn)
	assert.False(t, res.OK, "변경 의미 query-action 은 거부되어야 함")
	assert.NotEmpty(t, res.Error)
	assert.Empty(t, src.lastAction, "거부된 query 는 소스를 호출하지 않아야 함(READ-ONLY)")
}

// TestClient_QueryUnapprovedRejected 는 미승인 노드의 query 가 거부되는지 검증한다
// (REQ-J05 — 승인 게이팅).
func TestClient_QueryUnapprovedRejected(t *testing.T) {
	conn := newClientFakeConn()
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		return conn, nil
	})
	src := &recordingQuerySource{data: json.RawMessage(`"x"`)}
	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example/api/remote/ws",
		InstanceID:        "node-q-unapproved",
		HeartbeatInterval: time.Hour,
		QuerySource:       src,
		// DataDir 없음 → 토큰 미보유 → 미승인.
	}, dialer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cli.Start(ctx)
	defer cli.Stop()

	// register 소비.
	select {
	case <-conn.fromClient:
	case <-time.After(time.Second):
		t.Fatal("register 송신 타임아웃")
	}

	injectQuery(t, conn, QueryPayload{
		QueryID:     "q-2",
		Domain:      DomainFlow,
		QueryAction: QueryActionGet,
	})

	res := readQueryResult(t, conn)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "not approved")
	assert.Empty(t, src.lastAction)
}

// TestClient_QueryNilSourceRejected 는 QuerySource 미구성 시 query 가 거부되는지
// 검증한다.
func TestClient_QueryNilSourceRejected(t *testing.T) {
	cli, conn, cancel := startApprovedQueryClient(t, nil, nil)
	defer cancel()
	defer cli.Stop()

	injectQuery(t, conn, QueryPayload{
		QueryID:     "q-3",
		Domain:      DomainAgent,
		QueryAction: QueryActionStats,
	})

	res := readQueryResult(t, conn)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "source")
}

// TestClient_QuerySourceErrorReturned 는 소스 오류가 query_result 오류로 반환되는지
// 검증한다(REQ-J07 — node-error).
func TestClient_QuerySourceErrorReturned(t *testing.T) {
	src := &recordingQuerySource{err: errors.New("flow not found")}
	cli, conn, cancel := startApprovedQueryClient(t, src, nil)
	defer cancel()
	defer cli.Stop()

	injectQuery(t, conn, QueryPayload{
		QueryID:     "q-err",
		Domain:      DomainFlow,
		QueryAction: QueryActionStatus,
	})

	res := readQueryResult(t, conn)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "flow not found")
}

// panicQuerySource 는 Query 에서 panic 을 발생시키는 QuerySource 테스트 구현이다.
type panicQuerySource struct{ v any }

func (p *panicQuerySource) Query(_ context.Context, _, _ string, _ json.RawMessage) (json.RawMessage, error) {
	panic(p.v)
}

// TestClient_QueryPanicRecovered 는 QuerySource.Query panic 시 (1) 데몬이 죽지 않고
// (2) query_result{ok:false} 가 반환되며 (3) panic 값(시크릿)이 노출되지 않는지
// 검증한다(신뢰 경계 로버스트니스, REQ-J06).
func TestClient_QueryPanicRecovered(t *testing.T) {
	src := &panicQuerySource{v: "secret-token=abcdef boom"}
	cli, conn, cancel := startApprovedQueryClient(t, src, nil)
	defer cancel()
	defer cli.Stop()

	injectQuery(t, conn, QueryPayload{
		QueryID:     "q-panic",
		Domain:      DomainAgent,
		QueryAction: QueryActionStats,
	})

	res := readQueryResult(t, conn)
	assert.Equal(t, "q-panic", res.QueryID)
	assert.False(t, res.OK)
	assert.NotContains(t, res.Error, "secret-token=abcdef", "panic 값/시크릿은 노출 금지")

	// 세션 생존 확인: 후속 query 처리.
	injectQuery(t, conn, QueryPayload{
		QueryID:     "q-after",
		Domain:      DomainAgent,
		QueryAction: QueryActionStats,
	})
	res2 := readQueryResult(t, conn)
	assert.Equal(t, "q-after", res2.QueryID)
}

// TestClient_QueryMalformedIgnored 는 query_id 누락 query 가 세션을 중단시키지 않는지
// 검증한다.
func TestClient_QueryMalformedIgnored(t *testing.T) {
	src := &recordingQuerySource{data: json.RawMessage(`"ok"`)}
	cli, conn, cancel := startApprovedQueryClient(t, src, nil)
	defer cancel()
	defer cli.Stop()

	// query_id 누락 → 무시(결과 없음).
	injectQuery(t, conn, QueryPayload{Domain: DomainFlow, QueryAction: QueryActionGet})

	// 이어서 정상 query → 정상 결과.
	injectQuery(t, conn, QueryPayload{
		QueryID: "q-ok", Domain: DomainFlow, QueryAction: QueryActionGet,
	})
	res := readQueryResult(t, conn)
	assert.Equal(t, "q-ok", res.QueryID)
	assert.True(t, res.OK)
}
