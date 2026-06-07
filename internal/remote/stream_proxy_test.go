// stream_proxy_test.go 는 M8(그룹 J) 서버 측 스트리밍 프록시(팬아웃/teardown/백프레셔)를
// 검증한다(@SPEC:SPEC-REMOTE-001 M8, REQ-J08/J08b).
//
// 검증:
//   - subscribe → 노드로 subscribe 1회 전송 + 브라우저가 stream_data 프레임 수신(REQ-J08).
//   - 다중 소비자 팬아웃: 동일 키 구독은 노드 구독 1개를 공유한다(REQ-J08).
//   - 브라우저 연결 종료(Unsubscribe) → 마지막 소비자면 노드로 unsubscribe 전송(REQ-J08b).
//   - 노드 오프라인/세션 종료 → 그 노드의 모든 스트림 teardown(Done 신호, REQ-J08b).
//   - 터미널 stream_data{error} → 해당 스트림 모든 소비자 종료(Done 신호, REQ-J08).
//   - 백프레셔: 느린 소비자가 다른 소비자/읽기 루프를 막지 않는다(REQ-J08b).
//   - 누수 없음(-race + teardown 확인).
//
// 종료 판정은 StreamHandle.Done 으로 한다(Frames 는 레이스 회피를 위해 close 하지 않음).
// fakeConn / approvedNodeConn 는 server_test.go / dispatch_test.go 의 것을 재사용한다.
package remote

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readSubscribe 는 서버가 conn 으로 보낸 다음 subscribe 를 읽어 디코드한다.
func readSubscribe(t *testing.T, conn *fakeConn) SubscribePayload {
	t.Helper()
	select {
	case data := <-conn.outgoing:
		msg, err := DecodeMessage(data)
		require.NoError(t, err)
		require.Equal(t, TypeSubscribe, msg.Type)
		var sub SubscribePayload
		require.NoError(t, json.Unmarshal(msg.Payload, &sub))
		return sub
	case <-time.After(time.Second):
		t.Fatal("subscribe 수신 타임아웃")
		return SubscribePayload{}
	}
}

// injectStreamData 는 subscription_id 에 대한 stream_data 프레임을 노드처럼 주입한다.
func injectStreamData(t *testing.T, conn *fakeConn, subID string, payload json.RawMessage, errMsg string) {
	t.Helper()
	msg, err := NewStreamDataMessage(StreamDataPayload{
		SubscriptionID: subID, Payload: payload, Error: errMsg,
	})
	require.NoError(t, err)
	conn.inject(t, msg)
}

// TestSubscribeStream_FanOut 은 subscribe → 프레임 팬아웃 → 브라우저 수신을 검증한다
// (REQ-J08).
func TestSubscribeStream_FanOut(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-s1")
	defer cancel()

	h, err := srv.SubscribeStream("node-s1", DomainDevice, StreamActionState,
		json.RawMessage(`{"id":"d1"}`))
	require.NoError(t, err)
	defer h.Unsubscribe()

	sub := readSubscribe(t, conn)
	assert.Equal(t, "node-s1", sub.TargetInstanceID)
	assert.Equal(t, DomainDevice, sub.Domain)
	assert.Equal(t, StreamActionState, sub.StreamAction)
	require.NotEmpty(t, sub.SubscriptionID)

	injectStreamData(t, conn, sub.SubscriptionID, json.RawMessage(`{"on":true}`), "")

	select {
	case frame := <-h.Frames:
		assert.JSONEq(t, `{"on":true}`, string(frame))
	case <-time.After(time.Second):
		t.Fatal("브라우저가 stream_data 프레임을 수신하지 못함")
	}
}

// TestSubscribeStream_MultiConsumerSharesNodeSubscription 은 동일 키의 다중 소비자가
// 노드 구독 1개를 공유하고 둘 다 프레임을 받는지 검증한다(REQ-J08 팬아웃).
func TestSubscribeStream_MultiConsumerSharesNodeSubscription(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-s2")
	defer cancel()

	args := json.RawMessage(`{"id":"a1"}`)
	h1, err := srv.SubscribeStream("node-s2", DomainAgent, StreamActionStats, args)
	require.NoError(t, err)
	defer h1.Unsubscribe()
	sub := readSubscribe(t, conn) // 첫 소비자만 노드 subscribe 유발.

	h2, err := srv.SubscribeStream("node-s2", DomainAgent, StreamActionStats, args)
	require.NoError(t, err)
	defer h2.Unsubscribe()

	// 두 번째 소비자는 노드 재구독을 유발하지 않아야 한다(outgoing 에 새 subscribe 없음).
	select {
	case <-conn.outgoing:
		t.Fatal("동일 키 재구독에 중복 노드 subscribe 발생")
	case <-time.After(100 * time.Millisecond):
	}

	injectStreamData(t, conn, sub.SubscriptionID, json.RawMessage(`{"rx":7}`), "")

	for _, frames := range []<-chan json.RawMessage{h1.Frames, h2.Frames} {
		select {
		case frame := <-frames:
			assert.JSONEq(t, `{"rx":7}`, string(frame))
		case <-time.After(time.Second):
			t.Fatal("소비자가 팬아웃 프레임을 수신하지 못함")
		}
	}
}

// TestSubscribeStream_LastConsumerUnsubscribes 는 마지막 소비자 해제 시 노드로
// unsubscribe 가 전송되는지 검증한다(REQ-J08b).
func TestSubscribeStream_LastConsumerUnsubscribes(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-s3")
	defer cancel()

	h, err := srv.SubscribeStream("node-s3", DomainDevice, StreamActionState,
		json.RawMessage(`{"id":"d1"}`))
	require.NoError(t, err)
	sub := readSubscribe(t, conn)

	h.Unsubscribe() // 마지막 소비자 해제 → 노드로 unsubscribe.

	select {
	case data := <-conn.outgoing:
		msg, err := DecodeMessage(data)
		require.NoError(t, err)
		require.Equal(t, TypeUnsubscribe, msg.Type)
		var u UnsubscribePayload
		require.NoError(t, json.Unmarshal(msg.Payload, &u))
		assert.Equal(t, sub.SubscriptionID, u.SubscriptionID)
	case <-time.After(time.Second):
		t.Fatal("마지막 소비자 해제 시 unsubscribe 가 전송되지 않음")
	}
}

// TestSubscribeStream_TerminalErrorEndsStream 은 터미널 error 프레임이 소비자를
// 종료(Done)하는지 검증한다(REQ-J08).
func TestSubscribeStream_TerminalErrorEndsStream(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-s4")
	defer cancel()

	h, err := srv.SubscribeStream("node-s4", DomainAgent, StreamActionSeries,
		json.RawMessage(`{"id":"a1"}`))
	require.NoError(t, err)
	defer h.Unsubscribe()
	sub := readSubscribe(t, conn)

	injectStreamData(t, conn, sub.SubscriptionID, nil, "source error")

	select {
	case <-h.Done:
		// 터미널 오류 → Done 신호.
	case <-time.After(time.Second):
		t.Fatal("터미널 오류 후 Done 신호가 오지 않음")
	}
}

// TestSubscribeStream_NodeOfflineTeardown 은 노드 세션 종료 시 그 노드의 모든
// 브라우저 스트림이 teardown(Done)되는지 검증한다(REQ-J08b).
func TestSubscribeStream_NodeOfflineTeardown(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-s5")

	h, err := srv.SubscribeStream("node-s5", DomainDevice, StreamActionState,
		json.RawMessage(`{"id":"d1"}`))
	require.NoError(t, err)
	defer h.Unsubscribe()
	_ = readSubscribe(t, conn)

	// 노드 세션 종료(연결 닫힘) → handleConnection defer 가 teardownNodeStreams 호출.
	conn.Close()
	cancel()

	select {
	case <-h.Done:
		// 노드 오프라인 → Done 신호.
	case <-time.After(2 * time.Second):
		t.Fatal("노드 오프라인 후 스트림이 teardown 되지 않음")
	}
}

// TestSubscribeStream_Backpressure 는 느린 소비자가 다른 소비자/읽기 루프를 막지
// 않는지 검증한다(REQ-J08b). 느린 소비자는 채널을 읽지 않고, 빠른 소비자는 프레임을
// 계속 수신해야 한다.
func TestSubscribeStream_Backpressure(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-s6")
	defer cancel()

	args := json.RawMessage(`{"id":"d1"}`)
	// 느린 소비자: 채널을 읽지 않는다(버퍼가 곧 가득 참).
	hSlow, err := srv.SubscribeStream("node-s6", DomainDevice, StreamActionState, args)
	require.NoError(t, err)
	defer hSlow.Unsubscribe()
	sub := readSubscribe(t, conn)

	// 빠른 소비자: 동일 키 공유.
	hFast, err := srv.SubscribeStream("node-s6", DomainDevice, StreamActionState, args)
	require.NoError(t, err)
	defer hFast.Unsubscribe()

	// 많은 프레임을 주입한다(느린 소비자 버퍼 초과). 읽기 루프가 막히면 안 된다.
	for i := 0; i < 200; i++ {
		injectStreamData(t, conn, sub.SubscriptionID,
			json.RawMessage(`{"seq":`+itoa(i)+`}`), "")
	}

	// 빠른 소비자는 프레임을 계속 수신해야 한다(블로킹 없음 — 백프레셔가 격리).
	select {
	case <-hFast.Frames:
		// 수신 성공 — 읽기 루프가 느린 소비자에 의해 막히지 않음.
	case <-time.After(2 * time.Second):
		t.Fatal("빠른 소비자가 프레임을 수신하지 못함(읽기 루프 블로킹 의심)")
	}
}

// itoa 는 작은 정수를 문자열로 변환한다(테스트 헬퍼 — strconv import 회피).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
