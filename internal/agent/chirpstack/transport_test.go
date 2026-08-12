package chirpstack

import (
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// fakeToken 은 즉시 완료되는 mqtt.Token 이다.
type fakeToken struct{}

func (fakeToken) Wait() bool                     { return true }
func (fakeToken) WaitTimeout(time.Duration) bool { return true }
func (fakeToken) Done() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
func (fakeToken) Error() error { return nil }

// fakeErrToken 은 즉시 완료되지만 에러를 반환하는 mqtt.Token 이다 (발행 실패 경로 검증용).
type fakeErrToken struct{ err error }

func (t fakeErrToken) Wait() bool                     { return true }
func (t fakeErrToken) WaitTimeout(time.Duration) bool { return true }
func (t fakeErrToken) Done() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
func (t fakeErrToken) Error() error { return t.err }

// publishedMessage 는 fakeClient 가 기록한 발행 1건이다 (SPEC-CHIRPSTACK-002 M1/M2:
// 발행 0건/1건 및 정확한 토픽·QoS·retained·페이로드 검증에 사용된다).
type publishedMessage struct {
	topic    string
	qos      byte
	retained bool
	payload  []byte
}

// fakeClient 는 subscribe/publish 로직 검증용 최소 mqtt.Client 구현이다.
type fakeClient struct {
	subscribed   []string
	disconnected bool
	connected    bool

	// published 는 Publish 호출 기록이다. publishErr 이 설정되면 실패 토큰을 반환한다.
	published  []publishedMessage
	publishErr error
}

func (c *fakeClient) IsConnected() bool      { return c.connected }
func (c *fakeClient) IsConnectionOpen() bool { return c.connected }
func (c *fakeClient) Connect() mqtt.Token    { return fakeToken{} }
func (c *fakeClient) Disconnect(uint)        { c.disconnected = true }
func (c *fakeClient) Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
	var b []byte
	switch v := payload.(type) {
	case []byte:
		b = append([]byte(nil), v...)
	case string:
		b = []byte(v)
	}
	c.published = append(c.published, publishedMessage{
		topic: topic, qos: qos, retained: retained, payload: b,
	})
	if c.publishErr != nil {
		return fakeErrToken{err: c.publishErr}
	}
	return fakeToken{}
}
func (c *fakeClient) Subscribe(topic string, _ byte, _ mqtt.MessageHandler) mqtt.Token {
	c.subscribed = append(c.subscribed, topic)
	return fakeToken{}
}
func (c *fakeClient) SubscribeMultiple(map[string]byte, mqtt.MessageHandler) mqtt.Token {
	return fakeToken{}
}
func (c *fakeClient) Unsubscribe(...string) mqtt.Token        { return fakeToken{} }
func (c *fakeClient) AddRoute(string, mqtt.MessageHandler)    {}
func (c *fakeClient) OptionsReader() mqtt.ClientOptionsReader { return mqtt.ClientOptionsReader{} }

// fakeMessage 는 messageHandler 검증용 최소 mqtt.Message 구현이다.
type fakeMessage struct {
	topic   string
	payload []byte
}

func (m fakeMessage) Duplicate() bool   { return false }
func (m fakeMessage) Qos() byte         { return 0 }
func (m fakeMessage) Retained() bool    { return false }
func (m fakeMessage) Topic() string     { return m.topic }
func (m fakeMessage) MessageID() uint16 { return 0 }
func (m fakeMessage) Payload() []byte   { return m.payload }
func (m fakeMessage) Ack()              {}

// TestSubscribe_NotStopped 는 정상 상태에서 설정 토픽을 구독하는지 검증한다.
func TestSubscribe_NotStopped(t *testing.T) {
	a := newRunningTestAgent(t, "sub-cs") // 기본 토픽 application/#.
	fc := &fakeClient{connected: true}

	a.subscribe(fc)

	if len(fc.subscribed) != 1 || fc.subscribed[0] != "application/#" {
		t.Errorf("subscribed = %v, want [application/#]", fc.subscribed)
	}
	if fc.disconnected {
		t.Error("should not disconnect when not stopped")
	}
}

// TestSubscribe_StoppedGuard 는 Stop 이후 재연결 시 재구독 없이 즉시 Disconnect
// 하는지 검증한다 (세션 부활 방지).
func TestSubscribe_StoppedGuard(t *testing.T) {
	a := newRunningTestAgent(t, "subg-cs")
	a.stopped.Store(true)
	fc := &fakeClient{connected: true}

	a.subscribe(fc)

	if !fc.disconnected {
		t.Error("stopped subscribe should Disconnect")
	}
	if len(fc.subscribed) != 0 {
		t.Errorf("stopped subscribe should not subscribe, got %v", fc.subscribed)
	}
}

// TestMessageHandler_RoutesToHandleUplink 는 수신 콜백이 업링크를 fan-out 하여
// 레코드를 수신 채널에 넣는지 검증한다.
func TestMessageHandler_RoutesToHandleUplink(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "mh-cs")

	a.messageHandler(nil, fakeMessage{topic: "application/x", payload: loadRawUplink(t)})

	if len(a.recvCh) == 0 {
		t.Error("messageHandler should enqueue at least one measurement record")
	}
}
