package node

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 테스트 더블 (SPEC-CHIRPSTACK-002 M2)
// ---------------------------------------------------------------------------

// csPublished 는 fakeChirpStackAgent 가 기록한 발행 1건이다.
type csPublished struct {
	topic    string
	qos      byte
	retained bool
	payload  []byte
}

// fakeChirpStackAgent 는 control 노드가 요구하는 에이전트 능력(MessagePublisher +
// DownlinkTarget)을 흉내내며 발행을 기록한다.
type fakeChirpStackAgent struct {
	appID     string // "" 이면 applicationId 미캐시(AC-3b).
	profile   string
	published []csPublished
	pubErr    error
}

func (f *fakeChirpStackAgent) Init(_ agent.AgentConfig) error      { return nil }
func (f *fakeChirpStackAgent) Start(_ context.Context) error       { return nil }
func (f *fakeChirpStackAgent) Stop(_ context.Context) error        { return nil }
func (f *fakeChirpStackAgent) Pause(_ context.Context) error       { return nil }
func (f *fakeChirpStackAgent) Resume(_ context.Context) error      { return nil }
func (f *fakeChirpStackAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (f *fakeChirpStackAgent) Configure(_ agent.AgentConfig) error { return nil }
func (f *fakeChirpStackAgent) ID() string                          { return "fake-cs-id" }
func (f *fakeChirpStackAgent) Name() string                        { return "fake-cs" }
func (f *fakeChirpStackAgent) Type() string                        { return "chirpstack" }
func (f *fakeChirpStackAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (f *fakeChirpStackAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }
func (f *fakeChirpStackAgent) Process(_ []byte) ([]byte, error)    { return nil, nil }

func (f *fakeChirpStackAgent) PublishMessage(topic string, qos byte, retained bool, payload []byte) error {
	if f.pubErr != nil {
		return f.pubErr
	}
	f.published = append(f.published, csPublished{
		topic: topic, qos: qos, retained: retained,
		payload: append([]byte(nil), payload...),
	})
	return nil
}

func (f *fakeChirpStackAgent) DownlinkTarget(_ string) (string, string, bool) {
	if f.appID == "" {
		return "", "", false
	}
	return f.appID, f.profile, true
}

// newTestChirpStackControlNode 는 fake 에이전트에 연결된 control 노드를 만든다.
func newTestChirpStackControlNode(t *testing.T, fa *fakeChirpStackAgent) *ChirpStackControlNode {
	t.Helper()
	resolver := &mockMQTTResolver{transport: &mockMQTTTransport{agent: fa}}
	raw, err := NewChirpStackControlNode(
		flow.NodeDef{ID: "cs-ctl-1", Name: "cs-ctl", Type: "chirpstack-control"},
		WithAgentResolver(resolver),
	)
	if err != nil {
		t.Fatalf("NewChirpStackControlNode: %v", err)
	}
	n, ok := raw.(*ChirpStackControlNode)
	if !ok {
		t.Fatalf("unexpected node type %T", raw)
	}
	if err := n.Configure(map[string]any{"agent_ref": "cs-agent"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return n
}

// newControlMsg 는 control 노드 입력(payload typed command)을 만든다.
func newControlMsg(payload map[string]any) message.Message {
	msg := message.New()
	for k, v := range payload {
		msg.Payload().Set(k, v)
	}
	return msg
}

// ---------------------------------------------------------------------------
// AC-1 / AC-1b
// ---------------------------------------------------------------------------

// TestChirpStackControlNode_PublishesDownlink 는 WS301 typed command 가 정확히 1건의
// 다운링크로 발행되며 토픽/페이로드가 golden-vector 와 일치하는지 검증한다 (AC-1).
func TestChirpStackControlNode_PublishesDownlink(t *testing.T) {
	const (
		appID  = "96b4d719-f23f-40aa-9f94-a0f2d0354342"
		devEui = "24e124141d180806"
	)
	fa := &fakeChirpStackAgent{appID: appID, profile: "WS301"}
	n := newTestChirpStackControlNode(t, fa)

	out, err := n.Process(context.Background(), newControlMsg(map[string]any{
		"unit_id": devEui,
		"command": "reboot",
	}))
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("출력 메시지 = %d건, want 1", len(out))
	}
	if len(fa.published) != 1 {
		t.Fatalf("발행 = %d건, want 1", len(fa.published))
	}

	got := fa.published[0]
	wantTopic := "application/" + appID + "/device/" + devEui + "/command/down"
	if got.topic != wantTopic {
		t.Errorf("topic = %q, want %q", got.topic, wantTopic)
	}
	if got.retained {
		t.Error("다운링크 명령은 retained 여서는 안 된다")
	}

	var p struct {
		DevEui    string `json:"devEui"`
		Confirmed bool   `json:"confirmed"`
		FPort     uint8  `json:"fPort"`
		Data      string `json:"data"`
	}
	if err := json.Unmarshal(got.payload, &p); err != nil {
		t.Fatalf("페이로드 JSON 파싱 실패: %v (%s)", err, got.payload)
	}
	if p.DevEui != devEui {
		t.Errorf("devEui = %q, want %q", p.DevEui, devEui)
	}
	if p.Confirmed {
		t.Error("confirmed 기본값은 false 여야 한다")
	}
	if p.FPort != 85 {
		t.Errorf("fPort = %d, want 85", p.FPort)
	}
	if p.Data != "/xD/" { // base64(ff 10 ff) — WS301 reboot golden-vector.
		t.Errorf("data = %q, want %q", p.Data, "/xD/")
	}
}

// TestChirpStackControlNode_ConfirmedPassthrough 는 confirmed=true 가 페이로드로
// 그대로 통과되는지 검증한다 (AC-1b, A4 fire-and-publish).
func TestChirpStackControlNode_ConfirmedPassthrough(t *testing.T) {
	fa := &fakeChirpStackAgent{appID: "app-1", profile: "WS301"}
	n := newTestChirpStackControlNode(t, fa)

	if _, err := n.Process(context.Background(), newControlMsg(map[string]any{
		"device_id": "24e124141d180806", // unit_id 부재 시 device_id 폴백.
		"command":   "set_report_interval",
		"params":    map[string]any{"interval": float64(1200)},
		"confirmed": true,
	})); err != nil {
		t.Fatalf("Process: %v", err)
	}

	if len(fa.published) != 1 {
		t.Fatalf("발행 = %d건, want 1", len(fa.published))
	}
	var p struct {
		Confirmed bool   `json:"confirmed"`
		Data      string `json:"data"`
	}
	if err := json.Unmarshal(fa.published[0].payload, &p); err != nil {
		t.Fatalf("페이로드 JSON 파싱 실패: %v", err)
	}
	if !p.Confirmed {
		t.Error("confirmed=true 가 페이로드로 통과되어야 한다")
	}
	if p.Data != "/wOwBA==" { // base64(ff 03 b0 04) — 1200s 리틀엔디언.
		t.Errorf("data = %q, want %q", p.Data, "/wOwBA==")
	}
}

// ---------------------------------------------------------------------------
// AC-3 / AC-3b / AC-5
// ---------------------------------------------------------------------------

// TestChirpStackControlNode_Rejections 는 코덱 미등록/미지 command/applicationId
// 미캐시/필수 입력 누락이 에러 + 발행 0건으로 거부되는지 검증한다
// (AC-3, AC-3b, REQ-M2-04/05).
func TestChirpStackControlNode_Rejections(t *testing.T) {
	tests := []struct {
		name    string
		agent   *fakeChirpStackAgent
		payload map[string]any
	}{
		{
			name:    "미등록 deviceProfile 코덱 (AC-3)",
			agent:   &fakeChirpStackAgent{appID: "app-1", profile: "EM300-TH"},
			payload: map[string]any{"unit_id": "dev-1", "command": "reboot"},
		},
		{
			name:    "코덱이 모르는 command (AC-3)",
			agent:   &fakeChirpStackAgent{appID: "app-1", profile: "WS301"},
			payload: map[string]any{"unit_id": "dev-1", "command": "open_buzzer"},
		},
		{
			name:    "applicationId 미캐시 (AC-3b)",
			agent:   &fakeChirpStackAgent{appID: "", profile: "WS301"},
			payload: map[string]any{"unit_id": "dev-1", "command": "reboot"},
		},
		{
			name:    "devEui 누락",
			agent:   &fakeChirpStackAgent{appID: "app-1", profile: "WS301"},
			payload: map[string]any{"command": "reboot"},
		},
		{
			name:    "command 누락",
			agent:   &fakeChirpStackAgent{appID: "app-1", profile: "WS301"},
			payload: map[string]any{"unit_id": "dev-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := newTestChirpStackControlNode(t, tt.agent)

			out, err := n.Process(context.Background(), newControlMsg(tt.payload))
			if err == nil {
				t.Fatal("거부되어야 하는 입력이 성공했다")
			}
			if len(out) != 0 {
				t.Errorf("거부 시 출력 메시지는 0건이어야 한다, got %d", len(out))
			}
			if len(tt.agent.published) != 0 {
				t.Errorf("거부 시 발행 0건이어야 한다, got %d", len(tt.agent.published))
			}
		})
	}
}

// TestChirpStackControlNode_PublishGuardError 는 에이전트 발행 가드(미연결/stopped)
// 에러가 노드 에러로 전파되는지 검증한다 (AC-5, REQ-M1-04).
func TestChirpStackControlNode_PublishGuardError(t *testing.T) {
	fa := &fakeChirpStackAgent{
		appID:   "app-1",
		profile: "WS301",
		pubErr:  errChirpStackTestNotConnected,
	}
	n := newTestChirpStackControlNode(t, fa)

	out, err := n.Process(context.Background(), newControlMsg(map[string]any{
		"unit_id": "dev-1",
		"command": "reboot",
	}))
	if err == nil {
		t.Fatal("발행 가드 에러가 전파되어야 한다")
	}
	if len(out) != 0 {
		t.Errorf("에러 시 출력 메시지는 0건이어야 한다, got %d", len(out))
	}
	if len(fa.published) != 0 {
		t.Errorf("발행 0건이어야 한다, got %d", len(fa.published))
	}
}

// errChirpStackTestNotConnected 는 발행 가드 에러를 흉내내는 테스트 에러이다.
var errChirpStackTestNotConnected = errChirpStackTest("chirpstack: 브로커에 연결되어 있지 않음")

type errChirpStackTest string

func (e errChirpStackTest) Error() string { return string(e) }

// ---------------------------------------------------------------------------
// 설정 / 라이프사이클
// ---------------------------------------------------------------------------

// TestChirpStackControlNode_DevEuiFromMetadata 는 payload 에 devEui 셀렉터가 없을 때
// metadata 폴백으로 대상을 결정하는지 검증한다 (xsfmExtractDeviceID 관용구).
func TestChirpStackControlNode_DevEuiFromMetadata(t *testing.T) {
	fa := &fakeChirpStackAgent{appID: "app-1", profile: "WS301"}
	n := newTestChirpStackControlNode(t, fa)

	msg := newControlMsg(map[string]any{"command": "reboot"})
	msg.Metadata().Set("device_id", "24e124141d180806")

	if _, err := n.Process(context.Background(), msg); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(fa.published) != 1 {
		t.Fatalf("발행 = %d건, want 1", len(fa.published))
	}
}

// TestChirpStackControlNode_NonMapParamsRejected 는 params 가 맵이 아닌 경우 파라미터
// 부재로 취급되어 코덱이 거부하는지 검증한다 (발행 0건).
func TestChirpStackControlNode_NonMapParamsRejected(t *testing.T) {
	fa := &fakeChirpStackAgent{appID: "app-1", profile: "WS301"}
	n := newTestChirpStackControlNode(t, fa)

	_, err := n.Process(context.Background(), newControlMsg(map[string]any{
		"unit_id": "24e124141d180806",
		"command": "set_report_interval",
		"params":  "interval=1200", // 맵이 아님.
	}))
	if err == nil {
		t.Fatal("맵이 아닌 params 는 거부되어야 한다")
	}
	if len(fa.published) != 0 {
		t.Errorf("발행 0건이어야 한다, got %d", len(fa.published))
	}
}

// TestChirpStackControlNode_ConfigureRequiresAgentRef 는 agent_ref 필수 검증을
// 확인한다 (chirpstack-in 미러).
func TestChirpStackControlNode_ConfigureRequiresAgentRef(t *testing.T) {
	n, err := NewChirpStackControlNode(flow.NodeDef{ID: "n1", Name: "cs", Type: "chirpstack-control"})
	if err != nil {
		t.Fatalf("NewChirpStackControlNode: %v", err)
	}
	if err := n.Configure(map[string]any{}); err == nil {
		t.Error("agent_ref 없는 Configure 는 에러여야 한다")
	}
	if err := n.Configure(map[string]any{"agent_ref": "cs-agent"}); err != nil {
		t.Errorf("agent_ref 있는 Configure: %v", err)
	}
}

// TestChirpStackControlNode_InitRejectsNonDownlinker 는 다운링크 능력이 없는
// 에이전트를 Init 이 거부하는지 검증한다.
func TestChirpStackControlNode_InitRejectsNonDownlinker(t *testing.T) {
	resolver := &mockMQTTResolver{transport: &mockMQTTTransport{agent: &mockPlainAgent{}}}
	raw, err := NewChirpStackControlNode(
		flow.NodeDef{ID: "n2", Name: "cs", Type: "chirpstack-control"},
		WithAgentResolver(resolver),
	)
	if err != nil {
		t.Fatalf("NewChirpStackControlNode: %v", err)
	}
	n := raw.(*ChirpStackControlNode)
	if err := n.Configure(map[string]any{"agent_ref": "cs-agent"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := n.Init(context.Background()); err == nil {
		t.Error("다운링크 미지원 에이전트는 Init 에서 거부되어야 한다")
	}
}

// TestChirpStackControlNode_AgentRefAndShutdown 는 AgentRef 노출과 Shutdown/Reinit
// 경로를 검증한다.
func TestChirpStackControlNode_AgentRefAndShutdown(t *testing.T) {
	fa := &fakeChirpStackAgent{appID: "app-1", profile: "WS301"}
	n := newTestChirpStackControlNode(t, fa)

	if ref := n.AgentRef(); ref.AgentName != "cs-agent" || ref.AgentID != "cs-agent" {
		t.Errorf("AgentRef = %+v, want cs-agent", ref)
	}
	if err := n.Reinit(context.Background()); err != nil {
		t.Errorf("Reinit: %v", err)
	}
	if err := n.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}
