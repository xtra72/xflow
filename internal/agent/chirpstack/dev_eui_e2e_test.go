// dev_eui_e2e_test.go 는 "devEui 는 device_id 가 아니라 디바이스 정보다" 계약을
// 두 경계에서 관측한다.
//
//	(1) flow message 경계: 업링크 → 에이전트 → chirpstack-in 노드 → message.
//	    metadata.device.id 는 UUID v4 이고, devEui 는 metadata.device.dev_eui 로 실린다.
//	(2) inventory 경계: DeviceState.Properties.dev_eui → inventory(source=devices) 항목.
//
// 본 파일은 외부 테스트 패키지(chirpstack_test)이므로 internal/node · internal/device 를
// import 할 수 있다(node → chirpstack 단방향 의존은 유지된다 — inventory_e2e_test.go 와
// 동일한 배치). 에이전트 단위 테스트만으로는 "노드가 승격한 device 그룹"을 관측할 수
// 없어 계약이 실제로 하류까지 도달하는지 확인되지 않는다.
package chirpstack_test

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/chirpstack"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/internal/storage"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// csUUIDv4Re 는 metadata.device.id 가 UUID v4 인지 형식으로 검증한다.
// "devEui 가 아니다" 라는 부정 단언은 제3의 값(빈 문자열 등)을 통과시키므로 쓰지 않는다.
var csUUIDv4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

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

// readMessageOfType 은 지정 prefix 로 시작하는 type 의 메시지가 나올 때까지 읽는다.
// 업링크 1건이 event 와 device_state 를 함께 방출할 수 있으므로 필요하다.
func readMessageOfType(t *testing.T, src node.SourceNode, prefix string) message.Message {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case msg := <-src.SourceCh():
			if strings.HasPrefix(msg.Type(), prefix) {
				return msg
			}
		case <-deadline:
			t.Fatalf("type=%s* 메시지를 3초 내에 수신하지 못했다", prefix)
			return nil
		}
	}
}

// newDevEuiE2EAgent 는 실제 저장소 구현(UUID 발급)을 설치한 비활성화 에이전트를 만든다.
//
// 스텁이 아니라 storage.DeviceIDMemoryRepository 를 쓰는 이유: 본 파일의 핵심 주장이
// "device_id 는 저장소가 발급한 UUID v4 다" 이므로, 발급 주체를 스텁으로 바꾸면
// 검증 대상이 사라진다.
func newDevEuiE2EAgent(t *testing.T, name string, opts map[string]any) *chirpstack.ChirpStackAgent {
	t.Helper()
	agent.SetDeviceIDRepository(storage.NewDeviceIDMemoryRepository())
	t.Cleanup(func() { agent.SetDeviceIDRepository(nil) })

	disabled := false
	raw, err := chirpstack.NewChirpStackAgent(agent.AgentConfig{
		ID:        "id-" + name,
		Name:      name,
		Type:      "chirpstack",
		Enabled:   &disabled,
		Transport: agent.TransportConfig{Options: opts},
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

// logMetadata 는 메시지의 metadata 객체 전체를 관측 로그로 남긴다.
// 보고서의 "실제 방출 metadata" 근거이며, 주장이 아니라 실행 결과이다.
func logMetadata(t *testing.T, label string, msg message.Message) {
	t.Helper()
	pretty, err := json.MarshalIndent(msg.Metadata().Raw(), "", "  ")
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	t.Logf("%s 가 싣는 metadata 객체 (type=%s):\n%s", label, msg.Type(), pretty)
}

// assertDeviceGroup 은 device 그룹의 공통 계약을 검증한다: id 는 UUID v4,
// dev_eui 는 소문자 devEui, name/type 은 기존 계약 보존.
func assertDeviceGroup(t *testing.T, msg message.Message) map[string]string {
	t.Helper()
	dg, ok := msg.Metadata().GetGroup("device")
	if !ok {
		t.Fatal("metadata.device 그룹이 없다")
	}
	if !csUUIDv4Re.MatchString(dg["id"]) {
		t.Errorf("metadata.device.id = %q, want UUID v4 (정규식 %s)", dg["id"], csUUIDv4Re)
	}
	if dg["id"] == csDevEui {
		t.Errorf("metadata.device.id 가 devEui 다 — UUID 여야 한다")
	}
	// 회귀를 직접 봉인하는 단언: 이 분류가 Unknown 이면 GET /devices/{ref} 가 404 다.
	if got := device.ClassifyDeviceRef(dg["id"]); got != device.DeviceRefUUID {
		t.Errorf("ClassifyDeviceRef(%q) = %v, want DeviceRefUUID", dg["id"], got)
	}
	if dg["dev_eui"] != csDevEui {
		t.Errorf("metadata.device.dev_eui = %q, want %q", dg["dev_eui"], csDevEui)
	}
	return dg
}

// TestFlowMessage_PerMeasurementCarriesUuidAndDevEui 는 per-measurement 메시지에서
// device.id 가 UUID 이고 dev_eui 가 도달하는지, 그리고 기존 동결 계약
// (REQ-FROZEN-A/02) 이 함께 보존되는지 관측한다.
func TestFlowMessage_PerMeasurementCarriesUuidAndDevEui(t *testing.T) {
	const agentName = "cs-e2e-perm"
	a := newDevEuiE2EAgent(t, agentName, nil)
	src := startChirpStackInNode(t, a, agentName)

	a.HandleUplinkForTest(csRawUplink(t, map[string]any{"temperature": 21.5}), "application/x")

	msg := readMessage(t, src)
	logMetadata(t, "per-measurement flow message", msg)
	dg := assertDeviceGroup(t, msg)

	if dg["name"] != "WS301-180806" {
		t.Errorf("metadata.device.name = %q — 기존 계약이 보존되어야 한다", dg["name"])
	}
	if dg["type"] != "WS301" {
		t.Errorf("metadata.device.type = %q — 기존 계약이 보존되어야 한다", dg["type"])
	}

	// 동결 계약(REQ-FROZEN-A / REQ-FROZEN-02): 새 필드 외에는 모양이 그대로여야 한다.
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
	tags, _ := msg.Metadata().GetGroup("tags")
	if tags["location"] != "실습실" || tags["spot"] != "앞문" {
		t.Errorf("metadata.tags = %v — verbatim 통과가 보존되어야 한다", tags)
	}
}

// TestFlowMessage_CombinedCarriesUuidAndDevEui 는 combined(측정치 통합) 경로에서도
// 동일한 device 그룹 계약이 성립하는지 관측한다.
func TestFlowMessage_CombinedCarriesUuidAndDevEui(t *testing.T) {
	const agentName = "cs-e2e-combined"
	a := newDevEuiE2EAgent(t, agentName, map[string]any{"measurement_emit_mode": "combined"})
	src := startChirpStackInNode(t, a, agentName)

	a.HandleUplinkForTest(csRawUplink(t, map[string]any{
		"temperature": 29.8,
		"humidity":    55.2,
	}), "application/x")

	msg := readMessage(t, src)
	logMetadata(t, "combined flow message", msg)
	assertDeviceGroup(t, msg)

	// combined 모양 보존: payload 는 flat 키, metadata.measurement 는 없다.
	if msg.Type() != "event" {
		t.Errorf("type = %q, want event", msg.Type())
	}
	if v, _ := msg.Payload().Get("temperature"); v != 29.8 {
		t.Errorf("payload.temperature = %v, want 29.8", v)
	}
	if v, _ := msg.Payload().Get("humidity"); v != 55.2 {
		t.Errorf("payload.humidity = %v, want 55.2", v)
	}
	if _, ok := msg.Metadata().Get("measurement"); ok {
		t.Error("combined 경로는 metadata.measurement 를 방출하지 않는다")
	}
}

// TestFlowMessage_DeviceStateCarriesUuidAndDevEui 는 device_state(comm-state fold)
// 경로에서도 동일한 device 그룹 계약이 성립하는지 관측한다.
func TestFlowMessage_DeviceStateCarriesUuidAndDevEui(t *testing.T) {
	const agentName = "cs-e2e-state"
	a := newDevEuiE2EAgent(t, agentName, map[string]any{"emit_comm_state": true})
	src := startChirpStackInNode(t, a, agentName)

	a.HandleUplinkForTest(csRawUplink(t, map[string]any{"temperature": 1.0}), "application/x")

	msg := readMessageOfType(t, src, "device_state")
	logMetadata(t, "device_state flow message", msg)
	assertDeviceGroup(t, msg)

	// device_state 모양 보존.
	if msg.Type() != "device_state.change" {
		t.Errorf("type = %q, want device_state.change", msg.Type())
	}
	// payload.state 는 중첩 그룹으로 유지된다 (REQ-FROZEN-03, 평탄화하지 않는다).
	stateRaw, ok := msg.Payload().Get("state")
	if !ok {
		t.Fatal("payload.state 가 없다")
	}
	state, ok := stateRaw.(map[string]any)
	if !ok {
		t.Fatalf("payload.state 타입 = %T, want map", stateRaw)
	}
	if state["online"] != true {
		t.Errorf("payload.state.online = %v, want true", state["online"])
	}
	if state["gateway_id"] != "24e124fffef79304" {
		t.Errorf("payload.state.gateway_id = %v", state["gateway_id"])
	}
}

// TestInventory_DevEuiAlwaysPresent 는 dev_eui 가 Metadata().Labels 에 항상 실리고
// DeviceState.Properties 에서는 사라졌는지를 emit_comm_state 양쪽 설정에서 검증하고,
// inventory 노드가 실제로 방출하는 디바이스 정보 JSON 을 관측한다.
//
// emit_comm_state=false 를 함께 도는 이유: rssi/snr/gateway_id 는 그 노브 뒤에
// 게이팅되지만 dev_eui 는 게이팅되지 않는다는 것이 본 변경의 계약이다.
func TestInventory_DevEuiAlwaysPresent(t *testing.T) {
	for _, tc := range []struct {
		name          string
		emitCommState bool
	}{
		{"emit_comm_state=true", true},
		{"emit_comm_state=false", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agentName := "cs-inv-eui"
			a := newDevEuiE2EAgent(t, agentName, map[string]any{"emit_comm_state": tc.emitCommState})
			a.HandleUplinkForTest(csRawUplink(t, map[string]any{"temperature": 29.8}), "application/x")

			// (1) 프로바이더 경계 — dev_eui 는 metadata.labels, properties 에는 부재.
			dev := a.DeviceProvider().Devices()[0]
			if got := dev.Metadata().Labels["dev_eui"]; got != csDevEui {
				t.Errorf("Metadata().Labels[dev_eui] = %v, want %q", got, csDevEui)
			}
			props := dev.State().Properties
			if v, ok := props["dev_eui"]; ok {
				t.Errorf("properties[dev_eui] 존재(=%v) — metadata.labels 로 이동했다", v)
			}
			if _, hasRSSI := props["rssi"]; hasRSSI != tc.emitCommState {
				t.Errorf("properties[rssi] 존재=%v, want %v (링크 품질만 노브에 게이팅된다)",
					hasRSSI, tc.emitCommState)
			}

			// (2) inventory 노드 경계 — 실제 방출 JSON 을 관측한다.
			item := inventoryItems(t, agentName, a.DeviceProvider())[0]
			item["last_seen"] = "RFC3339-RUNTIME" // 실행 시각은 비결정적이라 자리표시자.
			if st, ok := item["state"].(map[string]any); ok {
				st["last_seen"] = "RFC3339-RUNTIME"
			}
			pretty, err := json.MarshalIndent(item, "", "  ")
			if err != nil {
				t.Fatalf("marshal item: %v", err)
			}
			t.Logf("inventory(source=devices) 가 방출하는 디바이스 정보:\n%s", pretty)

			st, ok := item["state"].(map[string]any)
			if !ok {
				t.Fatalf("item[state] = %#v, want map", item["state"])
			}
			itemProps, ok := st["properties"].(map[string]any)
			if !ok {
				t.Fatalf("item.state.properties = %#v, want map", st["properties"])
			}
			if v, ok := itemProps["dev_eui"]; ok {
				t.Errorf("item.state.properties.dev_eui 존재(=%v) — metadata.labels 로 이동했다", v)
			}
			itemMeta, ok := item["metadata"].(map[string]any)
			if !ok {
				t.Fatalf("item[metadata] = %#v, want map", item["metadata"])
			}
			itemLabels, ok := itemMeta["labels"].(map[string]string)
			if !ok {
				t.Fatalf("item.metadata.labels = %#v, want map[string]string", itemMeta["labels"])
			}
			if itemLabels["dev_eui"] != csDevEui {
				t.Errorf("item.metadata.labels.dev_eui = %v, want %q", itemLabels["dev_eui"], csDevEui)
			}
			// id 는 UUID 계약을 유지한다 (inventory 항목 경계).
			if id, _ := item["id"].(string); !csUUIDv4Re.MatchString(id) {
				t.Errorf("item[id] = %v, want UUID v4", item["id"])
			}
		})
	}
}

// TestFrozenTagSurface_FlowMessageUnaffectedByRosterPromotion 은 로스터 쪽 태그 승격
// (group/location → 전용 필드)이 동결된 flow message 태그 표면을 건드리지 않는지 관측한다.
//
// 동결 계약(REQ-FROZEN-A / REQ-FROZEN-02): $.metadata.tags.* 는 ChirpStack 태그를
// 대소문자·키 이름 그대로 verbatim 통과시킨다. 본 변경은 device roster / DeviceProvider
// 출력에만 적용되므로, 같은 업링크가 만드는 두 표면이 의도대로 서로 다르게 보여야 한다:
//
//	flow message : tags.location / tags.group 이 그대로 있다 (승격 없음).
//	device roster: location/group 이 전용 필드로 빠지고 Labels 에는 없다.
func TestFrozenTagSurface_FlowMessageUnaffectedByRosterPromotion(t *testing.T) {
	const agentName = "cs-frozen-tags"
	a := newDevEuiE2EAgent(t, agentName, nil)
	src := startChirpStackInNode(t, a, agentName)

	raw, err := json.Marshal(map[string]any{
		"time": "2026-08-11T23:32:01.129+00:00",
		"deviceInfo": map[string]any{
			"devEui":            csDevEui,
			"deviceName":        "WS301-180806",
			"deviceProfileName": "WS301",
			"applicationId":     "96b4d719-f23f-40aa-9f94-a0f2d0354342",
			"tags":              map[string]any{"location": "실습실", "group": "3층", "spot": "앞문"},
		},
		"object": map[string]any{"temperature": 21.5},
	})
	if err != nil {
		t.Fatalf("marshal uplink: %v", err)
	}
	a.HandleUplinkForTest(raw, "application/x")

	// (1) flow message 표면 — verbatim 통과가 보존되어야 한다.
	msg := readMessage(t, src)
	logMetadata(t, "flow message (태그 동결 표면 회귀)", msg)
	assertDeviceGroup(t, msg) // metadata.device.dev_eui 는 그대로 남는다.

	tags, ok := msg.Metadata().GetGroup("tags")
	if !ok {
		t.Fatal("metadata.tags 그룹이 없다 — 동결 표면이 사라졌다")
	}
	for k, want := range map[string]string{"location": "실습실", "group": "3층", "spot": "앞문"} {
		if tags[k] != want {
			t.Errorf("metadata.tags[%s] = %q, want %q (verbatim 통과 동결)", k, tags[k], want)
		}
	}
	if len(tags) != 3 {
		t.Errorf("metadata.tags = %v, want 3개 태그 그대로 (승격/제거가 새면 안 된다)", tags)
	}

	// (2) roster 표면 — 승격이 여기에만 적용된다.
	md := a.DeviceProvider().Devices()[0].Metadata()
	if md.Location != "실습실" || md.Group != "3층" {
		t.Errorf("roster Metadata: Location=%q Group=%q, want 실습실/3층", md.Location, md.Group)
	}
	for _, k := range []string{"location", "group"} {
		if v, ok := md.Labels[k]; ok {
			t.Errorf("roster Labels[%s] 잔존(=%q) — 승격은 이동이다", k, v)
		}
	}
	if md.Labels["spot"] != "앞문" || md.Labels["dev_eui"] != csDevEui {
		t.Errorf("roster Labels = %v, want {spot, dev_eui}", md.Labels)
	}
}

// TestInventory_DevEuiIsLowercased 는 대문자 devEui 업링크가 dev_eui 속성에서도
// 소문자로 접히는지 검증한다 (정규화가 로스터 키에만 적용되고 노출 값에서 새는
// 실패 양상을 막는다).
func TestInventory_DevEuiIsLowercased(t *testing.T) {
	const agentName = "cs-inv-eui-case"
	a := newDevEuiE2EAgent(t, agentName, nil)

	upper := strings.ToUpper(csDevEui)
	raw, err := json.Marshal(map[string]any{
		"time": "2026-08-11T23:32:01.129+00:00",
		"deviceInfo": map[string]any{
			"devEui":            upper,
			"deviceName":        "WS301-180806",
			"deviceProfileName": "WS301",
			"applicationId":     "96b4d719-f23f-40aa-9f94-a0f2d0354342",
		},
		"object": map[string]any{"temperature": 1.0},
	})
	if err != nil {
		t.Fatalf("marshal uplink: %v", err)
	}
	a.HandleUplinkForTest(raw, "application/x")

	devs := a.DeviceProvider().Devices()
	if len(devs) != 1 {
		t.Fatalf("Devices() len = %d, want 1", len(devs))
	}
	if got := devs[0].Metadata().Labels["dev_eui"]; got != csDevEui {
		t.Errorf("Metadata().Labels[dev_eui] = %v, want 소문자 %q", got, csDevEui)
	}
}
