package node

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/flow"
)

// ---------------------------------------------------------------------------
// 테스트 더블 (SPEC-CHIRPSTACK-002 M3)
// ---------------------------------------------------------------------------

// fakeCommStateAgent 는 status 노드가 요구하는 comm-state 조회 능력을 흉내낸다.
//
// MessagePublisher 도 함께 구현해 "status 노드가 발행 경로를 전혀 사용하지 않는다"
// (REQ-M3-02)를 기록 기반으로 검증할 수 있게 한다 — published 는 항상 0건이어야 한다.
type fakeCommStateAgent struct {
	entry      *fakeCommEntry // nil 이면 comm 엔트리 부재(AC-2b).
	commOn     bool
	recordErr  error
	published  []csPublished
	lastDevEui string
	lastTrig   string
	calls      int
}

// fakeCommEntry 는 에이전트 comm 맵에 캐시된 상태를 흉내낸다.
type fakeCommEntry struct {
	online     bool
	rssi       int
	snr        float64
	gatewayID  string
	lastSeenMs int64
}

func (f *fakeCommStateAgent) Init(_ agent.AgentConfig) error      { return nil }
func (f *fakeCommStateAgent) Start(_ context.Context) error       { return nil }
func (f *fakeCommStateAgent) Stop(_ context.Context) error        { return nil }
func (f *fakeCommStateAgent) Pause(_ context.Context) error       { return nil }
func (f *fakeCommStateAgent) Resume(_ context.Context) error      { return nil }
func (f *fakeCommStateAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (f *fakeCommStateAgent) Configure(_ agent.AgentConfig) error { return nil }
func (f *fakeCommStateAgent) ID() string                          { return "fake-cs-status-id" }
func (f *fakeCommStateAgent) Name() string                        { return "fake-cs-status" }
func (f *fakeCommStateAgent) Type() string                        { return "chirpstack" }
func (f *fakeCommStateAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (f *fakeCommStateAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }
func (f *fakeCommStateAgent) Process(_ []byte) ([]byte, error)    { return nil, nil }

// PublishMessage 는 호출되면 기록한다 — status 노드 경로에서는 호출되어서는 안 된다.
func (f *fakeCommStateAgent) PublishMessage(topic string, qos byte, retained bool, payload []byte) error {
	f.published = append(f.published, csPublished{
		topic: topic, qos: qos, retained: retained,
		payload: append([]byte(nil), payload...),
	})
	return nil
}

func (f *fakeCommStateAgent) CommStateEnabled() bool { return f.commOn }

// CommStateRecordJSON 은 실제 에이전트(buildDeviceStateRecord)와 동일한 shape 의
// device_state 레코드 JSON 을 만든다. 엔트리 부재 시 zero-value 로 조립한다.
func (f *fakeCommStateAgent) CommStateRecordJSON(devEui, trigger string) ([]byte, error) {
	f.calls++
	f.lastDevEui, f.lastTrig = devEui, trigger
	if f.recordErr != nil {
		return nil, f.recordErr
	}

	e := fakeCommEntry{}
	if f.entry != nil {
		e = *f.entry
	}
	return json.Marshal(map[string]any{
		"record":       "device_state",
		"trigger":      trigger,
		"unit_id":      devEui,
		"time_ms":      e.lastSeenMs,
		"last_seen_ms": e.lastSeenMs,
		"state": map[string]any{
			"online":       e.online,
			"rssi":         e.rssi,
			"snr":          e.snr,
			"gateway_id":   e.gatewayID,
			"last_seen_ms": e.lastSeenMs,
		},
	})
}

// newTestChirpStackStatusNode 는 fake 에이전트에 연결된 status 노드를 만든다.
func newTestChirpStackStatusNode(t *testing.T, fa *fakeCommStateAgent) *ChirpStackStatusNode {
	t.Helper()
	resolver := &mockMQTTResolver{transport: &mockMQTTTransport{agent: fa}}
	raw, err := NewChirpStackStatusNode(
		flow.NodeDef{ID: "cs-st-1", Name: "cs-st", Type: "chirpstack-status"},
		WithAgentResolver(resolver),
	)
	if err != nil {
		t.Fatalf("NewChirpStackStatusNode: %v", err)
	}
	n, ok := raw.(*ChirpStackStatusNode)
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

// ---------------------------------------------------------------------------
// AC-2: 캐시 상태 방출 — 0 publish / 0 poll
// ---------------------------------------------------------------------------

// TestChirpStackStatusNode_EmitsCachedCommState 는 캐시된 comm-state 가 1건의
// device_state 메시지로 방출되며 payload.state 가 캐시 값과 정확히 일치하고,
// MQTT 발행이 0건인지 검증한다 (AC-2, REQ-M3-01/02).
func TestChirpStackStatusNode_EmitsCachedCommState(t *testing.T) {
	const devEui = "24e124141d180806"
	lastSeen := time.Date(2026, 8, 11, 23, 32, 1, 129_000_000, time.UTC)

	fa := &fakeCommStateAgent{
		commOn: true,
		entry: &fakeCommEntry{
			online:     true,
			rssi:       -90,
			snr:        7.5,
			gatewayID:  "gw1",
			lastSeenMs: lastSeen.UnixMilli(),
		},
	}
	n := newTestChirpStackStatusNode(t, fa)

	out, err := n.Process(context.Background(), newControlMsg(map[string]any{
		"unit_id": devEui,
	}))
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("출력 메시지 = %d건, want 1 (트리거 1건당 1건)", len(out))
	}

	// REQ-M3-02: 발행 0건 / 폴 0건(에이전트 조회는 캐시 읽기 1회뿐).
	if len(fa.published) != 0 {
		t.Errorf("status 노드는 발행 0건이어야 한다, got %d", len(fa.published))
	}
	if fa.calls != 1 {
		t.Errorf("comm 캐시 조회 = %d회, want 1", fa.calls)
	}
	if fa.lastDevEui != devEui {
		t.Errorf("조회 devEui = %q, want %q", fa.lastDevEui, devEui)
	}

	msg := out[0]
	if msg.Type() != "device_state.report" {
		t.Errorf("type = %q, want device_state.report", msg.Type())
	}
	if !msg.Timestamp().Equal(lastSeen) {
		t.Errorf("timestamp = %v, want %v", msg.Timestamp().UTC(), lastSeen)
	}

	// top-level last_seen_ms 존재.
	lsRaw, ok := msg.Payload().Get("last_seen_ms")
	if !ok {
		t.Fatal("payload.last_seen_ms 누락")
	}
	if ls, _ := lsRaw.(int64); ls != lastSeen.UnixMilli() {
		t.Errorf("payload.last_seen_ms = %v, want %d", lsRaw, lastSeen.UnixMilli())
	}

	// unit_id 는 device 그룹으로 승격되며 payload 에서 제거된다.
	if _, ok := msg.Payload().Get("unit_id"); ok {
		t.Error("payload.unit_id 는 device 그룹 승격 후 제거되어야 한다")
	}

	stateRaw, ok := msg.Payload().Get("state")
	if !ok {
		t.Fatal("payload.state 누락")
	}
	state, ok := stateRaw.(map[string]any)
	if !ok {
		t.Fatalf("payload.state 타입 = %T, want map", stateRaw)
	}
	if state["online"] != true {
		t.Errorf("state.online = %v, want true", state["online"])
	}
	if state["rssi"] != -90 {
		t.Errorf("state.rssi = %v, want -90", state["rssi"])
	}
	if state["snr"] != 7.5 {
		t.Errorf("state.snr = %v, want 7.5", state["snr"])
	}
	if state["gateway_id"] != "gw1" {
		t.Errorf("state.gateway_id = %v, want gw1", state["gateway_id"])
	}
	if state["last_seen_ms"] != lastSeen.UnixMilli() {
		t.Errorf("state.last_seen_ms = %v, want %d", state["last_seen_ms"], lastSeen.UnixMilli())
	}
}

// ---------------------------------------------------------------------------
// AC-2b: comm 엔트리 부재 → offline/unknown
// ---------------------------------------------------------------------------

// TestChirpStackStatusNode_MissingEntryEmitsOffline 는 comm 엔트리 부재 시
// online=false 가 방출되고 online=true 가 조기 보고되지 않는지 검증한다
// (AC-2b, REQ-M3-04).
func TestChirpStackStatusNode_MissingEntryEmitsOffline(t *testing.T) {
	fa := &fakeCommStateAgent{commOn: true, entry: nil} // 엔트리 부재.
	n := newTestChirpStackStatusNode(t, fa)

	out, err := n.Process(context.Background(), newControlMsg(map[string]any{
		"unit_id": "24e124141d180806",
	}))
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("출력 메시지 = %d건, want 1", len(out))
	}
	if len(fa.published) != 0 {
		t.Errorf("status 노드는 발행 0건이어야 한다, got %d", len(fa.published))
	}

	stateRaw, ok := out[0].Payload().Get("state")
	if !ok {
		t.Fatal("payload.state 누락")
	}
	state, ok := stateRaw.(map[string]any)
	if !ok {
		t.Fatalf("payload.state 타입 = %T, want map", stateRaw)
	}
	if state["online"] == true {
		t.Fatal("엔트리 부재 시 online=true 를 조기 보고해서는 안 된다 (REQ-M3-04)")
	}
	if state["online"] != false {
		t.Errorf("state.online = %v, want false", state["online"])
	}
	if state["last_seen_ms"] != int64(0) {
		t.Errorf("state.last_seen_ms = %v, want 0 (unknown)", state["last_seen_ms"])
	}
}

// ---------------------------------------------------------------------------
// 거부 경로 / 설정 / 라이프사이클
// ---------------------------------------------------------------------------

// TestChirpStackStatusNode_Rejections 는 devEui 누락 및 조회 실패가 에러 + 발행 0건
// 으로 처리되는지 검증한다.
func TestChirpStackStatusNode_Rejections(t *testing.T) {
	tests := []struct {
		name    string
		agent   *fakeCommStateAgent
		payload map[string]any
	}{
		{
			name:    "devEui 누락",
			agent:   &fakeCommStateAgent{commOn: true},
			payload: map[string]any{},
		},
		{
			name:    "comm 조회 실패",
			agent:   &fakeCommStateAgent{commOn: true, recordErr: errChirpStackTest("직렬화 실패")},
			payload: map[string]any{"unit_id": "dev-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := newTestChirpStackStatusNode(t, tt.agent)

			out, err := n.Process(context.Background(), newControlMsg(tt.payload))
			if err == nil {
				t.Fatal("거부되어야 하는 입력이 성공했다")
			}
			if len(out) != 0 {
				t.Errorf("거부 시 출력 메시지는 0건이어야 한다, got %d", len(out))
			}
			if len(tt.agent.published) != 0 {
				t.Errorf("발행 0건이어야 한다, got %d", len(tt.agent.published))
			}
		})
	}
}

// TestChirpStackStatusNode_DevEuiFromMetadata 는 payload 에 셀렉터가 없을 때
// metadata 폴백으로 대상을 결정하는지 검증한다 (control 노드와 동일 관용구).
func TestChirpStackStatusNode_DevEuiFromMetadata(t *testing.T) {
	fa := &fakeCommStateAgent{commOn: true, entry: &fakeCommEntry{online: true}}
	n := newTestChirpStackStatusNode(t, fa)

	msg := newControlMsg(map[string]any{})
	msg.Metadata().Set("device_id", "24e124141d180806")

	if _, err := n.Process(context.Background(), msg); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if fa.lastDevEui != "24e124141d180806" {
		t.Errorf("조회 devEui = %q, want metadata 폴백 값", fa.lastDevEui)
	}
}

// TestChirpStackStatusNode_CommStateDisabledStillInits 는 emit_comm_state 가 꺼진
// 에이전트에서도 Init 이 하드 실패하지 않고(경고만) 방출 경로가 동작하는지 검증한다
// (R6 전략: strict 준수 + 공개).
func TestChirpStackStatusNode_CommStateDisabledStillInits(t *testing.T) {
	fa := &fakeCommStateAgent{commOn: false}
	n := newTestChirpStackStatusNode(t, fa) // Init 실패 시 t.Fatalf.

	out, err := n.Process(context.Background(), newControlMsg(map[string]any{"unit_id": "dev-1"}))
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("출력 메시지 = %d건, want 1", len(out))
	}
	stateRaw, _ := out[0].Payload().Get("state")
	state, ok := stateRaw.(map[string]any)
	if !ok {
		t.Fatalf("payload.state 타입 = %T", stateRaw)
	}
	if state["online"] != false {
		t.Errorf("emit_comm_state off 이면 항상 offline 이어야 한다, got %v", state["online"])
	}
}

// TestChirpStackStatusNode_RegisteredAndFactory 는 chirpstack-status 가 노드
// 레지스트리에 등록되고 팩토리가 노드를 생성하는지 검증한다 (REQ-M4-01).
func TestChirpStackStatusNode_RegisteredAndFactory(t *testing.T) {
	r := NewRegistry()
	for _, typeName := range []string{"chirpstack-status", "chirpstack-control"} {
		if !r.Has(typeName) {
			t.Fatalf("%s 가 노드 레지스트리에 등록되지 않았다", typeName)
		}
		if _, err := r.Create(flow.NodeDef{ID: "n1", Name: "cs", Type: typeName}); err != nil {
			t.Errorf("Create(%s): %v", typeName, err)
		}
	}
}

// TestChirpStackStatusNode_ConfigureRequiresAgentRef 는 agent_ref 필수 검증을
// 확인한다 (chirpstack-in 미러).
func TestChirpStackStatusNode_ConfigureRequiresAgentRef(t *testing.T) {
	n, err := NewChirpStackStatusNode(flow.NodeDef{ID: "n1", Name: "cs", Type: "chirpstack-status"})
	if err != nil {
		t.Fatalf("NewChirpStackStatusNode: %v", err)
	}
	if err := n.Configure(map[string]any{}); err == nil {
		t.Error("agent_ref 없는 Configure 는 에러여야 한다")
	}
	if err := n.Configure(map[string]any{"agent_ref": "cs-agent"}); err != nil {
		t.Errorf("agent_ref 있는 Configure: %v", err)
	}
}

// TestChirpStackStatusNode_InitRejectsNonCommReader 는 comm-state 조회 능력이 없는
// 에이전트를 Init 이 거부하는지 검증한다.
func TestChirpStackStatusNode_InitRejectsNonCommReader(t *testing.T) {
	resolver := &mockMQTTResolver{transport: &mockMQTTTransport{agent: &mockPlainAgent{}}}
	raw, err := NewChirpStackStatusNode(
		flow.NodeDef{ID: "n2", Name: "cs", Type: "chirpstack-status"},
		WithAgentResolver(resolver),
	)
	if err != nil {
		t.Fatalf("NewChirpStackStatusNode: %v", err)
	}
	n, ok := raw.(*ChirpStackStatusNode)
	if !ok {
		t.Fatalf("unexpected node type %T", raw)
	}
	if err := n.Configure(map[string]any{"agent_ref": "cs-agent"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := n.Init(context.Background()); err == nil {
		t.Error("comm-state 조회 미지원 에이전트는 Init 에서 거부되어야 한다")
	}
}

// TestChirpStackStatusNode_AgentRefAndShutdown 는 AgentRef 노출과 Shutdown/Reinit
// 경로를 검증한다.
func TestChirpStackStatusNode_AgentRefAndShutdown(t *testing.T) {
	fa := &fakeCommStateAgent{commOn: true}
	n := newTestChirpStackStatusNode(t, fa)

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
