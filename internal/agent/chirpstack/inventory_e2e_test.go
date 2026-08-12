// Package chirpstack_test - inventory_e2e_test.go: inventory(source=devices) ↔
// ChirpStack 프로바이더 경계 통합 테스트.
//
// 본 파일은 외부 테스트 패키지(chirpstack_test)이므로 internal/node 를 import 할 수
// 있다(node → chirpstack 단방향 의존은 유지된다). 개별 패키지 단위 테스트는 경계
// 결함(필드 이름/모양 불일치)을 잡지 못하므로, 양쪽을 함께 구동해 "inventory 노드가
// ChirpStack 디바이스에 대해 실제로 무엇을 방출하는가"를 관측한다.
package chirpstack_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/chirpstack"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// csDevEui 는 통합 테스트에서 사용하는 devEui 이다.
const csDevEui = "24e124141d180806"

// csMemDeviceIDRepo 는 테스트용 인메모리 DeviceIDRepository 이다 (UID 발급).
// 결정적 UUID 를 발급해 관측 JSON 을 안정적으로 만든다.
type csMemDeviceIDRepo struct{ m map[string]string }

func (r *csMemDeviceIDRepo) key(a, u string) string { return a + "|" + u }

func (r *csMemDeviceIDRepo) GetOrCreate(_ context.Context, a, u string) (string, error) {
	k := r.key(a, u)
	if v, ok := r.m[k]; ok {
		return v, nil
	}
	r.m[k] = "11111111-2222-4333-8444-555555555555"
	return r.m[k], nil
}

func (r *csMemDeviceIDRepo) Get(_ context.Context, a, u string) (string, error) {
	return r.m[r.key(a, u)], nil
}

func (r *csMemDeviceIDRepo) Set(_ context.Context, a, u, id string) error {
	r.m[r.key(a, u)] = id
	return nil
}

// newE2EAgent 는 비활성화(브로커 미연결) ChirpStack 에이전트를 만든다.
func newE2EAgent(t *testing.T, name string, opts map[string]any) *chirpstack.ChirpStackAgent {
	t.Helper()
	agent.SetDeviceIDRepository(&csMemDeviceIDRepo{m: make(map[string]string)})
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

// csRawUplink 는 원시 ChirpStack 업링크 JSON 을 만든다 (게이트웨이 2개).
func csRawUplink(t *testing.T, obj map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"time": "2026-08-11T23:32:01.129+00:00",
		"deviceInfo": map[string]any{
			"devEui":            csDevEui,
			"deviceName":        "WS301-180806",
			"deviceProfileName": "WS301",
			"applicationId":     "96b4d719-f23f-40aa-9f94-a0f2d0354342",
			"tags":              map[string]any{"location": "실습실", "spot": "앞문"},
		},
		"object": obj,
		"rxInfo": []any{
			map[string]any{"gatewayId": "24e124fffef5dccc", "rssi": -113, "snr": -9.5},
			map[string]any{"gatewayId": "24e124fffef79304", "rssi": -57, "snr": 13.5},
		},
	})
	if err != nil {
		t.Fatalf("marshal uplink: %v", err)
	}
	return b
}

// inventoryItems 는 레지스트리에 프로바이더를 등록하고 inventory(source=devices)
// 노드를 1회 구동해 항목 목록을 반환한다.
func inventoryItems(t *testing.T, agentName string, p device.DeviceProvider) []map[string]any {
	t.Helper()
	reg := device.NewRegistry()
	reg.RegisterProvider(agentName, p) // cmd/xflowd/main.go 와 동일한 배선.

	def := flow.NodeDef{ID: "inv-" + agentName, Type: "inventory", Config: map[string]any{"source": "devices"}}
	n, err := node.NewInventoryNode(def, node.WithDeviceRegistryFunc(func() device.DeviceRegistry { return reg }))
	if err != nil {
		t.Fatalf("NewInventoryNode: %v", err)
	}
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = n.Shutdown(context.Background()) })

	out, err := n.Process(context.Background(), message.New())
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("Process 메시지 수 = %d, want 1", len(out))
	}
	items, ok := out[0].Payload().Get("items")
	if !ok {
		t.Fatal("payload.items 없음")
	}
	list, ok := items.([]map[string]any)
	if !ok {
		t.Fatalf("items = %#v, want []map[string]any", items)
	}
	return list
}

// TestInventoryNode_ChirpStackDeviceEmission 은 inventory(source=devices) 노드가
// ChirpStack 디바이스에 대해 실제로 방출하는 JSON 을 관측하고 검증한다.
//
// 경계 검증 포인트:
//   - online 은 lastSeen 파생값이다 (FIX-A).
//   - state.properties 에 rssi/snr/gateway_id 가 실린다 (FEAT-B).
//   - state.properties.measurements 에 최신 측정값이 실린다 (FEAT-C).
func TestInventoryNode_ChirpStackDeviceEmission(t *testing.T) {
	a := newE2EAgent(t, "cs-inv", map[string]any{"emit_comm_state": true})
	a.HandleUplinkForTest(csRawUplink(t, map[string]any{
		"temperature": 29.8,
		"humidity":    55.2,
	}), "application/x")

	// (1) 함수 경계: DeviceProvider().Devices() 가 확장된 데이터를 반환한다.
	devs := a.DeviceProvider().Devices()
	if len(devs) != 1 {
		t.Fatalf("Devices() len = %d, want 1", len(devs))
	}
	st := devs[0].State()
	if !st.Online {
		t.Error("State().Online = false 직후 업링크, want true")
	}
	if st.Properties["rssi"] != -57 {
		t.Errorf("properties[rssi] = %v, want -57", st.Properties["rssi"])
	}
	if m, ok := st.Properties["measurements"].(map[string]any); !ok || m["temperature"] != 29.8 {
		t.Errorf("properties[measurements] = %v, want temperature 29.8", st.Properties["measurements"])
	}

	// (2) inventory 노드 경계: 실제 방출 메시지의 항목을 관측한다.
	list := inventoryItems(t, "cs-inv", a.DeviceProvider())
	if len(list) != 1 {
		t.Fatalf("items len = %d, want 1", len(list))
	}
	item := list[0]

	// last_seen 은 실행 시각이라 비결정적이므로 출력용으로만 자리표시자로 바꾼다.
	item["last_seen"] = "RFC3339-RUNTIME"
	if stateObj, ok := item["state"].(map[string]any); ok {
		stateObj["last_seen"] = "RFC3339-RUNTIME"
	}

	pretty, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		t.Fatalf("marshal item: %v", err)
	}
	// 관측 결과(주장 아님)를 그대로 출력한다 — 보고서의 "실제 방출 JSON" 근거.
	t.Logf("inventory(source=devices) 가 방출하는 ChirpStack 디바이스 항목:\n%s", pretty)

	if item["online"] != true {
		t.Errorf("item[online] = %v, want true", item["online"])
	}
	if item["protocol"] != "chirpstack" {
		t.Errorf("item[protocol] = %v", item["protocol"])
	}
	itemState, ok := item["state"].(map[string]any)
	if !ok {
		t.Fatalf("item[state] = %#v, want map", item["state"])
	}
	props, ok := itemState["properties"].(map[string]any)
	if !ok {
		t.Fatalf("item.state.properties = %#v, want map", itemState["properties"])
	}
	if props["gateway_id"] != "24e124fffef79304" || props["snr"] != 13.5 {
		t.Errorf("item.state.properties 링크 품질 불일치: %v", props)
	}
	if m, ok := props["measurements"].(map[string]any); !ok || m["humidity"] != 55.2 {
		t.Errorf("item.state.properties.measurements 불일치: %v", props["measurements"])
	}
}

// TestInventoryNode_ChirpStackStaleDeviceIsOffline 은 오래 침묵한 디바이스가
// inventory 노드에서 online:false 로 보고되는지 검증한다 (FIX-A 의 경계 확인).
//
// 로스터 내부를 조작하지 않고, offline_threshold 를 짧게 설정한 뒤 실제로 그만큼
// 기다린다 — 공개 경로만으로 staleness 전이를 재현한다.
func TestInventoryNode_ChirpStackStaleDeviceIsOffline(t *testing.T) {
	a := newE2EAgent(t, "cs-stale", map[string]any{"offline_threshold": "20ms"})
	a.HandleUplinkForTest(csRawUplink(t, map[string]any{"temperature": 1.0}), "application/x")

	if !a.DeviceProvider().Devices()[0].Online() {
		t.Fatal("업링크 직후 Online() = false, want true")
	}

	time.Sleep(60 * time.Millisecond) // 임계(20ms) 초과까지 대기.

	list := inventoryItems(t, "cs-stale", a.DeviceProvider())
	if len(list) != 1 {
		t.Fatalf("items len = %d, want 1", len(list))
	}
	if list[0]["online"] != false {
		t.Errorf("item[online] = %v, want false (임계 초과 침묵)", list[0]["online"])
	}
	state, ok := list[0]["state"].(map[string]any)
	if !ok || state["online"] != false {
		t.Errorf("item.state.online = %v, want false", state["online"])
	}
	// FEAT-C: offline 이어도 마지막으로 알려진 측정값은 유지된다.
	props, ok := state["properties"].(map[string]any)
	if !ok {
		t.Fatalf("item.state.properties = %#v, want map", state["properties"])
	}
	if m, ok := props["measurements"].(map[string]any); !ok || m["temperature"] != 1.0 {
		t.Errorf("offline 디바이스의 measurements 캐시가 유실되었다: %v", props["measurements"])
	}
	// emit_comm_state=false 이므로 링크 품질 키는 부재여야 한다 (FEAT-B).
	for _, k := range []string{"rssi", "snr", "gateway_id"} {
		if v, ok := props[k]; ok {
			t.Errorf("properties[%s] 존재(=%v) — emit_comm_state=false 이면 부재여야 한다", k, v)
		}
	}
}
