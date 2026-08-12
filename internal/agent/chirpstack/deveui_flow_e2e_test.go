// deveui_flow_e2e_test.go 는 "업링크 → 에이전트 → chirpstack-in 노드 → flow message"
// 전 구간에서 metadata.device.id 가 devEui 인지를 관측한다.
//
// 본 파일은 외부 테스트 패키지(chirpstack_test)이므로 internal/node 를 import 할 수
// 있다(node → chirpstack 단방향 의존은 유지된다 — inventory_e2e_test.go 와 동일한
// 배치). 에이전트 단위 테스트만으로는 "노드가 승격한 device 그룹"을 관측할 수 없어
// 계약이 실제로 하류까지 도달하는지 확인되지 않는다.
package chirpstack_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/chirpstack"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/internal/storage"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// e2eTransport 는 chirpstack-in 노드가 요구하는 AgentTransport + AgentAccessor 를
// 최소 구현으로 만족시킨다. 노드는 UnderlyingAgent() 로 얻은 에이전트의
// MessageReceiver 만 사용하므로 Send/Receive 는 호출되지 않는다.
type e2eTransport struct{ a agent.Agent }

func (t *e2eTransport) Send(_ context.Context, _ message.Message) error { return nil }
func (t *e2eTransport) Receive(_ context.Context) (message.Message, error) {
	return nil, context.Canceled
}
func (t *e2eTransport) UnderlyingAgent() agent.Agent { return t.a }

// e2eResolver 는 항상 같은 에이전트를 반환하는 AgentResolver 이다.
type e2eResolver struct{ a agent.Agent }

func (r *e2eResolver) ResolveAgent(_ context.Context, _ flow.AgentRef) (node.AgentTransport, error) {
	return &e2eTransport{a: r.a}, nil
}

// startChirpStackInNode 는 chirpstack-in 노드를 실제 팩토리로 만들어 기동한다.
func startChirpStackInNode(t *testing.T, a agent.Agent, agentRef string) node.SourceNode {
	t.Helper()
	def := flow.NodeDef{
		ID:     "cs-in-" + agentRef,
		Type:   "chirpstack-in",
		Config: map[string]any{"agent_ref": agentRef},
	}
	n, err := node.NewChirpStackInNode(def, node.WithAgentResolver(&e2eResolver{a: a}))
	if err != nil {
		t.Fatalf("NewChirpStackInNode: %v", err)
	}
	if err := n.Configure(map[string]any{"agent_ref": agentRef}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = n.Shutdown(context.Background()) })

	src, ok := n.(node.SourceNode)
	if !ok {
		t.Fatalf("chirpstack-in 노드가 SourceNode 가 아니다: %T", n)
	}
	return src
}

// readMessage 는 소스 채널에서 메시지 1건을 기다린다.
func readMessage(t *testing.T, src node.SourceNode) message.Message {
	t.Helper()
	select {
	case msg := <-src.SourceCh():
		return msg
	case <-time.After(3 * time.Second):
		t.Fatal("flow message 를 3초 내에 수신하지 못했다")
		return nil
	}
}

// newDevEuiE2EAgent 는 실제 저장소 구현을 설치한 비활성화 에이전트를 만든다.
func newDevEuiE2EAgent(t *testing.T, name string) *chirpstack.ChirpStackAgent {
	t.Helper()
	agent.SetDeviceIDRepository(storage.NewDeviceIDMemoryRepository())
	t.Cleanup(func() { agent.SetDeviceIDRepository(nil) })

	disabled := false
	raw, err := chirpstack.NewChirpStackAgent(agent.AgentConfig{
		ID:      "id-" + name,
		Name:    name,
		Type:    "chirpstack",
		Enabled: &disabled,
	})
	if err != nil {
		t.Fatalf("NewChirpStackAgent: %v", err)
	}
	a, ok := raw.(*chirpstack.ChirpStackAgent)
	if !ok {
		t.Fatalf("unexpected agent type %T", raw)
	}
	return a
}

// TestFlowMessage_DeviceIDIsDevEui 는 flow message 의 metadata.device.id 가
// devEui 인지를 노드 빌더까지 구동해 관측한다 (본 변경의 하류 도달 확인).
//
// 관측된 metadata.device 객체 전체를 로그로 남긴다 — 보고서의 "실제 방출 값" 근거이며,
// 주장이 아니라 실행 결과이다.
func TestFlowMessage_DeviceIDIsDevEui(t *testing.T) {
	const agentName = "cs-e2e-devid"
	a := newDevEuiE2EAgent(t, agentName)
	src := startChirpStackInNode(t, a, agentName)

	a.HandleUplinkForTest(csRawUplink(t, map[string]any{"temperature": 21.5}), "application/x")

	msg := readMessage(t, src)

	deviceGroup, ok := msg.Metadata().GetGroup("device")
	if !ok {
		t.Fatal("metadata.device 그룹이 없다")
	}
	pretty, err := json.MarshalIndent(deviceGroup, "", "  ")
	if err != nil {
		t.Fatalf("marshal device group: %v", err)
	}
	t.Logf("flow message 가 싣는 metadata.device 객체:\n%s", pretty)

	if deviceGroup["id"] != csDevEui {
		t.Errorf("metadata.device.id = %q, want devEui %q", deviceGroup["id"], csDevEui)
	}
	if deviceGroup["name"] != "WS301-180806" {
		t.Errorf("metadata.device.name = %q — 기존 계약이 보존되어야 한다", deviceGroup["name"])
	}
	if deviceGroup["type"] != "WS301" {
		t.Errorf("metadata.device.type = %q — 기존 계약이 보존되어야 한다", deviceGroup["type"])
	}

	// 기존 다운스트림 계약(REQ-FROZEN-01/02)이 함께 보존되는지도 확인한다.
	if msg.Type() != "event" {
		t.Errorf("type = %q, want event", msg.Type())
	}
	if v, _ := msg.Payload().Get("value"); v != 21.5 {
		t.Errorf("payload.value = %v, want 21.5", v)
	}
	if _, ok := msg.Payload().Get("unit_id"); ok {
		t.Error("payload.unit_id 는 device 승격 후 제거되어야 한다")
	}
	if m, _ := msg.Metadata().Get("measurement"); m != "temperature" {
		t.Errorf("metadata.measurement = %q, want temperature", m)
	}
}

// TestFlowMessage_PreExistingUuidConvertedEndToEnd 는 이미 UUID 를 보유한 디바이스가
// 다음 업링크에서 flow message 상에서도 devEui 로 전환되는지 관측한다 (FULL 전환).
func TestFlowMessage_PreExistingUuidConvertedEndToEnd(t *testing.T) {
	const agentName = "cs-e2e-convert"
	a := newDevEuiE2EAgent(t, agentName)
	ctx := context.Background()

	// BEFORE: 변경 전 자동 발급 UUID 를 그대로 심는다.
	legacy := agent.ResolveDeviceID(ctx, agentName, csDevEui)
	if legacy == "" || legacy == csDevEui {
		t.Fatalf("BEFORE 상태 준비 실패: 기존 device_id = %q", legacy)
	}

	src := startChirpStackInNode(t, a, agentName)
	a.HandleUplinkForTest(csRawUplink(t, map[string]any{"temperature": 1.0}), "application/x")

	msg := readMessage(t, src)
	deviceGroup, ok := msg.Metadata().GetGroup("device")
	if !ok {
		t.Fatal("metadata.device 그룹이 없다")
	}
	t.Logf("BEFORE device_id=%q → AFTER metadata.device.id=%q", legacy, deviceGroup["id"])

	if deviceGroup["id"] == legacy {
		t.Errorf("metadata.device.id 가 기존 UUID(%q)로 남아 있다 — 전환되지 않았다", legacy)
	}
	if deviceGroup["id"] != csDevEui {
		t.Errorf("metadata.device.id = %q, want devEui %q", deviceGroup["id"], csDevEui)
	}
}
