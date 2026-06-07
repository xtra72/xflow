// client_m8_stream_test.go 는 M8(그룹 J) 노드 측 스트리밍 프록시(subscribe/stream_data/
// unsubscribe)의 push·teardown·백프레셔를 검증한다(@SPEC:SPEC-REMOTE-001 M8,
// REQ-J08/J08b/J06).
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

// fakeStreamSub 는 StreamSubscription 의 테스트 구현이다. updates 로 갱신을 주입하고
// closed 플래그로 teardown(Close) 호출 여부를 검증한다.
type fakeStreamSub struct {
	updates chan json.RawMessage
	closed  atomic.Bool
}

func newFakeStreamSub() *fakeStreamSub {
	return &fakeStreamSub{updates: make(chan json.RawMessage, 8)}
}

func (s *fakeStreamSub) Updates() <-chan json.RawMessage { return s.updates }

func (s *fakeStreamSub) Close() error {
	s.closed.Store(true)
	return nil
}

// recordingStreamSource 는 StreamSource 의 테스트 구현이다. 마지막 구독을 보관한다.
type recordingStreamSource struct {
	mu         sync.Mutex
	lastDomain string
	lastAction string
	sub        *fakeStreamSub
	err        error
}

func (r *recordingStreamSource) Subscribe(_ context.Context, domain, action string, _ json.RawMessage) (StreamSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastDomain = domain
	r.lastAction = action
	if r.err != nil {
		return nil, r.err
	}
	r.sub = newFakeStreamSub()
	return r.sub, nil
}

func (r *recordingStreamSource) subscription() *fakeStreamSub {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sub
}

// startApprovedStreamClient 는 승인 상태 클라이언트에 StreamSource(+redactor)를 주입한다.
func startApprovedStreamClient(t *testing.T, src StreamSource, redactor QueryRedactor) (*Client, *clientFakeConn, context.CancelFunc) {
	t.Helper()
	conn := newClientFakeConn()
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		return conn, nil
	})
	dir := t.TempDir()
	require.NoError(t, SaveNodeToken(dir, "node-token-s"))

	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example/api/remote/ws",
		InstanceID:        "node-s",
		HeartbeatInterval: time.Hour,
		DataDir:           dir,
		StreamSource:      src,
		QueryRedactor:     redactor,
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	cli.Start(ctx)

	select {
	case <-conn.fromClient:
	case <-time.After(time.Second):
		t.Fatal("핸드셰이크(hello) 송신 타임아웃")
	}
	return cli, conn, cancel
}

func injectSubscribe(t *testing.T, conn *clientFakeConn, p SubscribePayload) {
	t.Helper()
	msg, err := NewSubscribeMessage(p)
	require.NoError(t, err)
	data, err := msg.Encode()
	require.NoError(t, err)
	conn.toClient <- data
}

func injectUnsubscribe(t *testing.T, conn *clientFakeConn, id string) {
	t.Helper()
	msg, err := NewUnsubscribeMessage(UnsubscribePayload{SubscriptionID: id})
	require.NoError(t, err)
	data, err := msg.Encode()
	require.NoError(t, err)
	conn.toClient <- data
}

// readStreamData 는 클라이언트가 보낸 다음 stream_data 를 읽는다.
func readStreamData(t *testing.T, conn *clientFakeConn) StreamDataPayload {
	t.Helper()
	select {
	case data := <-conn.fromClient:
		msg, err := DecodeMessage(data)
		require.NoError(t, err)
		require.Equal(t, TypeStreamData, msg.Type)
		var sd StreamDataPayload
		require.NoError(t, json.Unmarshal(msg.Payload, &sd))
		return sd
	case <-time.After(2 * time.Second):
		t.Fatal("stream_data 수신 타임아웃")
		return StreamDataPayload{}
	}
}

// TestClient_SubscribePushesStreamData 는 subscribe 후 소스 갱신이 stream_data 로
// push 되는지 검증한다(REQ-J08).
func TestClient_SubscribePushesStreamData(t *testing.T) {
	src := &recordingStreamSource{}
	cli, conn, cancel := startApprovedStreamClient(t, src, nil)
	defer cancel()
	defer cli.Stop()

	injectSubscribe(t, conn, SubscribePayload{
		SubscriptionID: "s-1",
		Domain:         DomainDevice,
		StreamAction:   StreamActionState,
		Args:           json.RawMessage(`{"id":"d1"}`),
	})

	// 구독이 등록될 때까지 대기.
	require.Eventually(t, func() bool { return src.subscription() != nil }, time.Second, 5*time.Millisecond)
	src.subscription().updates <- json.RawMessage(`{"online":true}`)

	sd := readStreamData(t, conn)
	assert.Equal(t, "s-1", sd.SubscriptionID)
	assert.JSONEq(t, `{"online":true}`, string(sd.Payload))
	assert.Empty(t, sd.Error)
	assert.Equal(t, DomainDevice, src.lastDomain)
	assert.Equal(t, StreamActionState, src.lastAction)
}

// TestClient_StreamRedactsBeforeSend 는 stream_data 본문이 전송 전 redaction 되는지
// 검증한다(REQ-J06).
func TestClient_StreamRedactsBeforeSend(t *testing.T) {
	src := &recordingStreamSource{}
	redactor := QueryRedactorFunc(func(_ json.RawMessage) json.RawMessage {
		return json.RawMessage(`{"token":"***"}`)
	})
	cli, conn, cancel := startApprovedStreamClient(t, src, redactor)
	defer cancel()
	defer cli.Stop()

	injectSubscribe(t, conn, SubscribePayload{
		SubscriptionID: "s-r",
		Domain:         DomainAgent,
		StreamAction:   StreamActionStats,
	})
	require.Eventually(t, func() bool { return src.subscription() != nil }, time.Second, 5*time.Millisecond)
	src.subscription().updates <- json.RawMessage(`{"token":"secret"}`)

	sd := readStreamData(t, conn)
	assert.JSONEq(t, `{"token":"***"}`, string(sd.Payload))
	assert.NotContains(t, string(sd.Payload), "secret")
}

// TestClient_UnsubscribeTearsDown 는 unsubscribe 가 소스 구독을 Close 하고 이후
// stream_data 가 멈추는지 검증한다(REQ-J08b — teardown).
func TestClient_UnsubscribeTearsDown(t *testing.T) {
	src := &recordingStreamSource{}
	cli, conn, cancel := startApprovedStreamClient(t, src, nil)
	defer cancel()
	defer cli.Stop()

	injectSubscribe(t, conn, SubscribePayload{
		SubscriptionID: "s-2",
		Domain:         DomainDevice,
		StreamAction:   StreamActionState,
	})
	require.Eventually(t, func() bool { return src.subscription() != nil }, time.Second, 5*time.Millisecond)
	sub := src.subscription()

	injectUnsubscribe(t, conn, "s-2")

	// 구독이 Close 되어야 한다(소스 구독 해제 — 누수 없음).
	require.Eventually(t, func() bool { return sub.closed.Load() }, time.Second, 5*time.Millisecond,
		"unsubscribe 는 소스 구독을 Close 해야 함")
}

// TestClient_SessionEndTearsDownAllStreams 는 세션 종료 시 모든 구독이 teardown
// 되는지 검증한다(REQ-J08b — 노드 오프라인/연결 종료 시 전 구독 정리, 누수 없음).
func TestClient_SessionEndTearsDownAllStreams(t *testing.T) {
	src := &recordingStreamSource{}
	cli, conn, cancel := startApprovedStreamClient(t, src, nil)
	defer cancel()

	injectSubscribe(t, conn, SubscribePayload{
		SubscriptionID: "s-3",
		Domain:         DomainDevice,
		StreamAction:   StreamActionState,
	})
	require.Eventually(t, func() bool { return src.subscription() != nil }, time.Second, 5*time.Millisecond)
	sub := src.subscription()

	// 세션 종료(클라이언트 정지) → 모든 구독 teardown.
	cli.Stop()

	require.Eventually(t, func() bool { return sub.closed.Load() }, 2*time.Second, 5*time.Millisecond,
		"세션 종료 시 모든 구독이 Close 되어야 함(누수 없음)")
}

// TestClient_SubscribeNonStreamableRejected 는 비스트림 action 구독이 오류 프레임으로
// 거부되고 소스를 호출하지 않는지 검증한다(REQ-J04/J08).
func TestClient_SubscribeNonStreamableRejected(t *testing.T) {
	src := &recordingStreamSource{}
	cli, conn, cancel := startApprovedStreamClient(t, src, nil)
	defer cancel()
	defer cli.Stop()

	// flow.get 은 스트림 불가 → 거부.
	injectSubscribe(t, conn, SubscribePayload{
		SubscriptionID: "s-bad",
		Domain:         DomainFlow,
		StreamAction:   QueryActionGet,
	})

	sd := readStreamData(t, conn)
	assert.Equal(t, "s-bad", sd.SubscriptionID)
	assert.NotEmpty(t, sd.Error, "비스트림 action 은 오류 프레임으로 거부되어야 함")
	assert.Empty(t, src.lastAction, "거부된 구독은 소스를 호출하지 않아야 함")
}

// TestClient_SubscribeUnapprovedRejected 는 미승인 노드의 subscribe 가 거부되는지
// 검증한다(REQ-J05).
func TestClient_SubscribeUnapprovedRejected(t *testing.T) {
	conn := newClientFakeConn()
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		return conn, nil
	})
	src := &recordingStreamSource{}
	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example/api/remote/ws",
		InstanceID:        "node-s-unapproved",
		HeartbeatInterval: time.Hour,
		StreamSource:      src,
	}, dialer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cli.Start(ctx)
	defer cli.Stop()

	select {
	case <-conn.fromClient:
	case <-time.After(time.Second):
		t.Fatal("register 송신 타임아웃")
	}

	injectSubscribe(t, conn, SubscribePayload{
		SubscriptionID: "s-u",
		Domain:         DomainDevice,
		StreamAction:   StreamActionState,
	})

	sd := readStreamData(t, conn)
	assert.Contains(t, sd.Error, "not approved")
	assert.Empty(t, src.lastAction)
}

// blockingConn 은 WriteMessage 가 release 될 때까지 블로킹하는 Conn 래퍼이다.
// 백프레셔(느린 소비자) 검증에 사용한다: 스트림 쓰기가 막혀도 읽기 루프는 계속
// 다른 메시지를 처리해야 한다.
type blockingConn struct {
	*clientFakeConn
	releaseStreamWrite chan struct{}
	once               sync.Once
}

func (b *blockingConn) WriteMessage(data []byte) error {
	// stream_data 쓰기만 블로킹한다(릴리스 신호 전까지). 다른 메시지(query_result 등)는
	// 즉시 통과시켜, 막힌 스트림이 읽기 루프/다른 응답을 막지 않음을 검증한다.
	if msg, err := DecodeMessage(data); err == nil && msg.Type == TypeStreamData {
		select {
		case <-b.releaseStreamWrite:
		case <-b.closed:
			return nil
		}
	}
	return b.clientFakeConn.WriteMessage(data)
}

// TestClient_StreamBackpressureDoesNotBlockReadLoop 는 느린 스트림 소비자(쓰기 블로킹)가
// 읽기 루프/다른 query 응답을 막지 않는지 검증한다(REQ-J08b — 백프레셔, 비차단).
func TestClient_StreamBackpressureDoesNotBlockReadLoop(t *testing.T) {
	src := &recordingStreamSource{}
	qsrc := &recordingQuerySource{data: json.RawMessage(`{"ok":true}`)}

	base := newClientFakeConn()
	bconn := &blockingConn{
		clientFakeConn:     base,
		releaseStreamWrite: make(chan struct{}),
	}
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		return bconn, nil
	})
	dir := t.TempDir()
	require.NoError(t, SaveNodeToken(dir, "node-token-bp"))
	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example/api/remote/ws",
		InstanceID:        "node-bp",
		HeartbeatInterval: time.Hour,
		DataDir:           dir,
		StreamSource:      src,
		QuerySource:       qsrc,
	}, dialer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cli.Start(ctx)
	defer cli.Stop()

	// hello 소비.
	select {
	case <-base.fromClient:
	case <-time.After(time.Second):
		t.Fatal("hello 송신 타임아웃")
	}

	// 구독 시작 → 갱신 push → 스트림 쓰기가 블로킹된다.
	injectSubscribe(t, base, SubscribePayload{
		SubscriptionID: "s-bp",
		Domain:         DomainDevice,
		StreamAction:   StreamActionState,
	})
	require.Eventually(t, func() bool { return src.subscription() != nil }, time.Second, 5*time.Millisecond)
	src.subscription().updates <- json.RawMessage(`{"v":1}`)

	// 스트림 쓰기가 막힌 동안에도 query 는 정상 응답되어야 한다(읽기 루프 비차단).
	injectQuery(t, base, QueryPayload{
		QueryID: "q-during-bp", Domain: DomainFlow, QueryAction: QueryActionGet,
	})

	// query_result 가 stream_data 블로킹 와중에 도착해야 한다.
	deadline := time.After(2 * time.Second)
	gotQuery := false
	for !gotQuery {
		select {
		case data := <-base.fromClient:
			msg, derr := DecodeMessage(data)
			require.NoError(t, derr)
			if msg.Type == TypeQueryResult {
				var res QueryResultPayload
				require.NoError(t, json.Unmarshal(msg.Payload, &res))
				assert.Equal(t, "q-during-bp", res.QueryID)
				gotQuery = true
			}
			// stream_data 는 블로킹되어 여기 도달하지 않음(릴리스 전).
		case <-deadline:
			t.Fatal("스트림 쓰기 블로킹 중 query_result 가 도착하지 않음 — 읽기 루프가 막힘")
		}
	}

	// 스트림 쓰기 릴리스 → 세션 정상 종료(누수 없음).
	bconn.once.Do(func() { close(bconn.releaseStreamWrite) })
}
