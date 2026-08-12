package chirpstack

import (
	"errors"
	"testing"
)

// setTestClient 는 테스트 에이전트에 fakeClient 를 주입한다 (a.mu 보호 하에 설정).
func setTestClient(a *ChirpStackAgent, c *fakeClient) {
	a.mu.Lock()
	a.client = c
	a.mu.Unlock()
}

// TestPublishMessage_Guards 는 발행 가드(nil/미연결 클라이언트, 빈 토픽, stopped)가
// 에러를 반환하고 발행을 0건으로 막는지 검증한다 (REQ-M1-01/04, AC-5).
//
// mqtt_agent.go:607-640 의 가드 패턴을 미러링한다.
func TestPublishMessage_Guards(t *testing.T) {
	tests := []struct {
		name    string
		client  *fakeClient // nil 이면 클라이언트 미주입(nil client 경로).
		stopped bool
		topic   string
	}{
		{
			name:   "nil 클라이언트",
			client: nil,
			topic:  "application/app-1/device/dev-1/command/down",
		},
		{
			name:   "미연결 클라이언트",
			client: &fakeClient{connected: false},
			topic:  "application/app-1/device/dev-1/command/down",
		},
		{
			name:   "빈 토픽",
			client: &fakeClient{connected: true},
			topic:  "",
		},
		{
			name:    "stopped 에이전트",
			client:  &fakeClient{connected: true},
			stopped: true,
			topic:   "application/app-1/device/dev-1/command/down",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newRunningTestAgent(t, "pubguard-cs")
			if tt.client != nil {
				setTestClient(a, tt.client)
			}
			a.stopped.Store(tt.stopped)

			err := a.PublishMessage(tt.topic, 0, false, []byte(`{"x":1}`))
			if err == nil {
				t.Fatal("PublishMessage 는 가드 위반 시 에러를 반환해야 한다")
			}
			if tt.client != nil && len(tt.client.published) != 0 {
				t.Errorf("발행 건수 = %d, want 0", len(tt.client.published))
			}
		})
	}
}

// TestPublishMessage_Success 는 연결 상태에서 정확히 1건이 지정한 토픽/QoS/retained/
// 페이로드로 발행되는지 검증한다 (REQ-M1-01).
func TestPublishMessage_Success(t *testing.T) {
	a := newRunningTestAgent(t, "pubok-cs")
	fc := &fakeClient{connected: true}
	setTestClient(a, fc)

	const topic = "application/96b4d719/device/24e124141d180806/command/down"
	payload := []byte(`{"devEui":"24e124141d180806","confirmed":false,"fPort":85,"data":"/xD/"}`)

	if err := a.PublishMessage(topic, 1, false, payload); err != nil {
		t.Fatalf("PublishMessage: %v", err)
	}

	if len(fc.published) != 1 {
		t.Fatalf("발행 건수 = %d, want 1", len(fc.published))
	}
	got := fc.published[0]
	if got.topic != topic {
		t.Errorf("topic = %q, want %q", got.topic, topic)
	}
	if got.qos != 1 {
		t.Errorf("qos = %d, want 1", got.qos)
	}
	if got.retained {
		t.Error("retained = true, want false")
	}
	if string(got.payload) != string(payload) {
		t.Errorf("payload = %q, want %q", got.payload, payload)
	}
	if a.stats.Snapshot().ExternalMessagesSent != 1 {
		t.Errorf("ExternalMessagesSent = %d, want 1", a.stats.Snapshot().ExternalMessagesSent)
	}
}

// TestPublishMessage_TokenError 는 브로커 발행 실패 시 에러를 감싸 반환하고 에러
// 통계를 증가시키는지 검증한다 (mqtt_agent.go 가드 미러).
func TestPublishMessage_TokenError(t *testing.T) {
	a := newRunningTestAgent(t, "puberr-cs")
	wantErr := errors.New("broker rejected")
	fc := &fakeClient{connected: true, publishErr: wantErr}
	setTestClient(a, fc)

	err := a.PublishMessage("application/a/device/d/command/down", 0, false, []byte(`{}`))
	if err == nil {
		t.Fatal("발행 실패 시 에러를 반환해야 한다")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want wrapped %v", err, wantErr)
	}
	if a.stats.Snapshot().ExternalMessagesErrored != 1 {
		t.Errorf("ExternalMessagesErrored = %d, want 1", a.stats.Snapshot().ExternalMessagesErrored)
	}
}
