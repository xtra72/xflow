package node

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/message"
)

// 본 파일은 "상세 정보(detail)" 토글(emit_detail / emit_metadata.detail)을 검증한다.
//
// 계약:
//   - detail ON (기본, 키 부재)  → agent{id,name,type} / device{id,name,type,dev_eui} 전체.
//   - detail OFF               → agent{id} / device{id} 만. 그 외 그룹 필드 전부 제거.
//   - 축소 대상은 agent / device 그룹뿐이다. measurement / tags / timestamp / payload 는
//     frozen 계약(REQ-FROZEN-A, REQ-FROZEN-02)이므로 절대 건드리지 않는다.
//   - 축소는 chirpstack 로컬이다. 공유 헬퍼(emitAgentGroup / mergeDeviceGroup /
//     promoteDevIDWithUUID)의 동작은 바뀌지 않으므로 다른 노드 타입의 출력은 불변이다.

// csDetailFixture 는 detail 테스트가 공유하는 실측 픽스처이다.
const (
	csDetailAgentName = "cs-detail"
	csDetailDevEui    = "24e124141d180806"
	csDetailDevType   = "WS301"
	csDetailDevLabel  = "WS301-180806"
)

// setupCSDetailDevice 는 에이전트 디바이스 라이프사이클(UUID 발급 + DeviceInfo 등록)을
// 시뮬레이션하고 기대 UUID 를 반환한다.
func setupCSDetailDevice(t *testing.T) string {
	t.Helper()
	agent.SetDeviceIDRepository(&memDeviceIDRepo{m: make(map[string]string)})
	t.Cleanup(func() { agent.SetDeviceIDRepository(nil) })

	uid := agent.ResolveDeviceID(context.Background(), csDetailAgentName, csDetailDevEui)
	if _, err := uuid.Parse(uid); err != nil {
		t.Fatalf("ResolveDeviceID did not yield UUID: %q", uid)
	}
	agent.SetDeviceInfo(csDetailAgentName, csDetailDevEui, agent.DeviceInfo{
		DeviceType: csDetailDevType,
		Label:      csDetailDevLabel,
	})
	return uid
}

// csDetailAgent 는 agent 그룹을 채우기 위한 최소 에이전트 더블이다.
// 기존 fakeChirpStackAgent 를 재사용하되 Type/ID/Name 만 쓴다.
func csDetailAgent() agent.Agent { return &fakeChirpStackAgent{} }

// wantGroup 은 그룹이 정확히 기대한 키-값 집합인지 검증한다(초과 키도 실패).
func wantGroup(t *testing.T, msg message.Message, key string, want map[string]string) {
	t.Helper()
	got, ok := msg.Metadata().GetGroup(key)
	if !ok {
		t.Fatalf("metadata.%s 그룹이 없습니다 (want %v)", key, want)
	}
	if len(got) != len(want) {
		t.Errorf("metadata.%s = %v, want %v (키 개수 %d != %d)", key, got, want, len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("metadata.%s.%s = %q, want %q (전체 %v)", key, k, got[k], v, got)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("metadata.%s 에 예상치 못한 키 %q=%q 존재 (전체 %v)", key, k, got[k], got)
		}
	}
}

// wantNoGroup 은 그룹이 아예 존재하지 않음을 검증한다(빈 그룹 {} 도 실패).
func wantNoGroup(t *testing.T, msg message.Message, key string) {
	t.Helper()
	if g, ok := msg.Metadata().GetGroup(key); ok {
		t.Errorf("metadata.%s 그룹이 없어야 하는데 존재합니다: %v", key, g)
	}
}

// csDetailMeasurementRecord 는 per-measurement 레코드 픽스처를 만든다.
func csDetailMeasurementRecord(tsMs int64) []byte {
	data, _ := json.Marshal(map[string]any{
		"measurement": "magnet_status",
		"value":       "close",
		"unit_id":     csDetailDevEui,
		"time_ms":     tsMs,
		"tags":        map[string]string{"location": "실습실", "point": "앞문", "spot": "앞문"},
	})
	return data
}

// ---------------------------------------------------------------------------
// AC-D1: 기본(키 부재) 은 오늘과 동일 — 전체 그룹
// ---------------------------------------------------------------------------

// TestChirpStackDetail_DefaultKeepsFullGroups 는 detail 키가 없을 때 metadata 가
// 오늘의 동작(agent{id,name,type} + device{id,name,type,dev_eui})과 동일한지 검증한다.
func TestChirpStackDetail_DefaultKeepsFullGroups(t *testing.T) {
	wantUID := setupCSDetailDevice(t)
	ts := time.Date(2026, 8, 11, 23, 32, 1, 129_000_000, time.UTC)

	var opts MetadataEmitOptions
	parseEmitMetadata(map[string]any{}, &opts) // detail 키 부재 → 기본 ON.
	if !opts.Detail {
		t.Fatal("detail 키 부재 시 Detail 은 기본 true 여야 합니다")
	}

	msg, ok := buildChirpStackMessage(csDetailMeasurementRecord(ts.UnixMilli()), "node-1", csDetailAgent(), csDetailAgentName, opts)
	if !ok {
		t.Fatal("buildChirpStackMessage ok=false")
	}

	wantGroup(t, msg, "agent", map[string]string{
		"type": "chirpstack-client", "id": "fake-cs-id", "name": "fake-cs",
	})
	wantGroup(t, msg, "device", map[string]string{
		"id": wantUID, "name": csDetailDevLabel, "type": csDetailDevType, "dev_eui": csDetailDevEui,
	})
}

// ---------------------------------------------------------------------------
// AC-D2: detail OFF → id 만. frozen 계약은 그대로.
// ---------------------------------------------------------------------------

// TestChirpStackDetail_OffReducesToIDOnly 는 emit_detail:false 일 때 agent/device 가
// id 하나로 축소되고, measurement / tags / timestamp / payload 는 전혀 변하지 않는지
// 검증한다 (REQ-FROZEN-A 과대축소 금지).
func TestChirpStackDetail_OffReducesToIDOnly(t *testing.T) {
	wantUID := setupCSDetailDevice(t)
	ts := time.Date(2026, 8, 11, 23, 32, 1, 129_000_000, time.UTC)

	var opts MetadataEmitOptions
	parseEmitMetadata(map[string]any{"emit_detail": false}, &opts)
	if opts.Detail {
		t.Fatal("emit_detail:false 가 Detail 을 끄지 못했습니다")
	}

	msg, ok := buildChirpStackMessage(csDetailMeasurementRecord(ts.UnixMilli()), "node-1", csDetailAgent(), csDetailAgentName, opts)
	if !ok {
		t.Fatal("buildChirpStackMessage ok=false")
	}

	// 축소 대상: agent / device 는 id 만.
	wantGroup(t, msg, "agent", map[string]string{"id": "fake-cs-id"})
	wantGroup(t, msg, "device", map[string]string{"id": wantUID})

	// frozen 계약: 아래는 detail 과 무관하게 보존되어야 한다.
	if m, _ := msg.Metadata().Get("measurement"); m != "magnet_status" {
		t.Errorf("metadata.measurement = %q, want magnet_status (frozen)", m)
	}
	wantGroup(t, msg, "tags", map[string]string{
		"location": "실습실", "point": "앞문", "spot": "앞문",
	})
	if !msg.Timestamp().Equal(ts) {
		t.Errorf("timestamp = %v, want %v (frozen)", msg.Timestamp().UTC(), ts)
	}
	if v, _ := msg.Payload().Get("value"); v != "close" {
		t.Errorf("payload.value = %v, want close (frozen)", v)
	}
	if msg.Type() != "event" {
		t.Errorf("type = %q, want event (frozen)", msg.Type())
	}
}

// ---------------------------------------------------------------------------
// AC-D2b: metadata 전체 golden — ON / OFF 의 정확한 객체를 고정한다
// ---------------------------------------------------------------------------

// TestChirpStackDetail_MetadataGolden 은 detail ON / OFF 각각의 metadata 객체
// 전체를 JSON 으로 고정한다. 그룹 단위 검증(wantGroup)이 놓치는 "그룹 외 키의
// 추가/삭제"까지 잡는 것이 목적이다.
//
// UUID 는 repo 를 미리 seed 해 결정적으로 만든다.
func TestChirpStackDetail_MetadataGolden(t *testing.T) {
	const fixedUID = "11111111-2222-3333-4444-555555555555"

	repo := &memDeviceIDRepo{m: make(map[string]string)}
	if err := repo.Set(context.Background(), csDetailAgentName, csDetailDevEui, fixedUID); err != nil {
		t.Fatalf("repo.Set: %v", err)
	}
	agent.SetDeviceIDRepository(repo)
	t.Cleanup(func() { agent.SetDeviceIDRepository(nil) })
	agent.SetDeviceInfo(csDetailAgentName, csDetailDevEui, agent.DeviceInfo{
		DeviceType: csDetailDevType,
		Label:      csDetailDevLabel,
	})

	ts := time.Date(2026, 8, 11, 23, 32, 1, 129_000_000, time.UTC)

	tests := []struct {
		name   string
		config map[string]any
		want   string
	}{
		{
			name:   "detail ON (기본, 키 부재)",
			config: map[string]any{},
			want: `{
  "agent": {
    "id": "fake-cs-id",
    "name": "fake-cs",
    "type": "chirpstack-client"
  },
  "device": {
    "dev_eui": "24e124141d180806",
    "id": "11111111-2222-3333-4444-555555555555",
    "name": "WS301-180806",
    "type": "WS301"
  },
  "measurement": "magnet_status",
  "tags": {
    "location": "실습실",
    "point": "앞문",
    "spot": "앞문"
  }
}`,
		},
		{
			name:   "detail OFF (emit_detail:false)",
			config: map[string]any{"emit_detail": false},
			want: `{
  "agent": {
    "id": "fake-cs-id"
  },
  "device": {
    "id": "11111111-2222-3333-4444-555555555555"
  },
  "measurement": "magnet_status",
  "tags": {
    "location": "실습실",
    "point": "앞문",
    "spot": "앞문"
  }
}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var opts MetadataEmitOptions
			parseEmitMetadata(tc.config, &opts)

			msg, ok := buildChirpStackMessage(
				csDetailMeasurementRecord(ts.UnixMilli()), "node-1",
				csDetailAgent(), csDetailAgentName, opts)
			if !ok {
				t.Fatal("buildChirpStackMessage ok=false")
			}

			got, err := json.MarshalIndent(msg.Metadata().Raw(), "", "  ")
			if err != nil {
				t.Fatalf("metadata 직렬화 실패: %v", err)
			}
			t.Logf("관측된 metadata (%s):\n%s", tc.name, got)
			if string(got) != tc.want {
				t.Errorf("metadata mismatch\n--- got ---\n%s\n--- want ---\n%s", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AC-D3: 중첩 형식 + 평탄 우선순위
// ---------------------------------------------------------------------------

// TestChirpStackDetail_ConfigParsing 은 중첩(emit_metadata.detail) / 평탄(emit_detail)
// 두 형식과 그 우선순위(평탄이 중첩을 덮어씀)를 검증한다 — 기존 emit_agent/emit_device
// 관용구와 동일하다.
func TestChirpStackDetail_ConfigParsing(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
		want   bool
	}{
		{"키 부재 → 기본 ON", map[string]any{}, true},
		{"중첩 false", map[string]any{"emit_metadata": map[string]any{"detail": false}}, false},
		{"중첩 true", map[string]any{"emit_metadata": map[string]any{"detail": true}}, true},
		{"평탄 false", map[string]any{"emit_detail": false}, false},
		{"평탄 true", map[string]any{"emit_detail": true}, true},
		{
			"평탄(false) 이 중첩(true) 을 덮어씀",
			map[string]any{"emit_metadata": map[string]any{"detail": true}, "emit_detail": false},
			false,
		},
		{
			"평탄(true) 이 중첩(false) 을 덮어씀",
			map[string]any{"emit_metadata": map[string]any{"detail": false}, "emit_detail": true},
			true,
		},
		{"bool 이 아닌 값은 무시 → 기본 ON", map[string]any{"emit_detail": "false"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var opts MetadataEmitOptions
			parseEmitMetadata(tc.config, &opts)
			if opts.Detail != tc.want {
				t.Errorf("Detail = %v, want %v", opts.Detail, tc.want)
			}
		})
	}
}

// TestChirpStackDetail_NestedFormEndToEnd 는 중첩 형식이 실제 메시지 축소까지
// 이어지는지 검증한다(파싱 단위 테스트만으로는 배선 누락을 잡지 못한다).
func TestChirpStackDetail_NestedFormEndToEnd(t *testing.T) {
	wantUID := setupCSDetailDevice(t)

	var opts MetadataEmitOptions
	parseEmitMetadata(map[string]any{"emit_metadata": map[string]any{"detail": false}}, &opts)

	msg, ok := buildChirpStackMessage(csDetailMeasurementRecord(1), "node-1", csDetailAgent(), csDetailAgentName, opts)
	if !ok {
		t.Fatal("buildChirpStackMessage ok=false")
	}
	wantGroup(t, msg, "agent", map[string]string{"id": "fake-cs-id"})
	wantGroup(t, msg, "device", map[string]string{"id": wantUID})
}

// ---------------------------------------------------------------------------
// AC-D4: 그룹 토글과의 상호작용 — 빈 그룹을 만들지 않는다
// ---------------------------------------------------------------------------

// TestChirpStackDetail_InteractionWithGroupToggles 는 detail OFF 와 emit_agent /
// emit_device OFF 가 겹칠 때 "빈 그룹({})" 이 아니라 "그룹 없음" 이 되는지 검증한다.
func TestChirpStackDetail_InteractionWithGroupToggles(t *testing.T) {
	wantUID := setupCSDetailDevice(t)

	t.Run("detail OFF + agent OFF → agent 그룹 자체가 없음", func(t *testing.T) {
		var opts MetadataEmitOptions
		parseEmitMetadata(map[string]any{"emit_detail": false, "emit_agent": false}, &opts)

		msg, ok := buildChirpStackMessage(csDetailMeasurementRecord(1), "n", csDetailAgent(), csDetailAgentName, opts)
		if !ok {
			t.Fatal("ok=false")
		}
		wantNoGroup(t, msg, "agent")
		// device 는 여전히 축소된 형태로 남는다.
		wantGroup(t, msg, "device", map[string]string{"id": wantUID})
	})

	t.Run("detail OFF + device OFF → device 그룹 자체가 없음", func(t *testing.T) {
		var opts MetadataEmitOptions
		parseEmitMetadata(map[string]any{"emit_detail": false, "emit_device": false}, &opts)

		msg, ok := buildChirpStackMessage(csDetailMeasurementRecord(1), "n", csDetailAgent(), csDetailAgentName, opts)
		if !ok {
			t.Fatal("ok=false")
		}
		wantNoGroup(t, msg, "device")
		wantGroup(t, msg, "agent", map[string]string{"id": "fake-cs-id"})
	})

	t.Run("detail OFF + 양쪽 OFF → 두 그룹 모두 없음", func(t *testing.T) {
		var opts MetadataEmitOptions
		parseEmitMetadata(map[string]any{"emit_detail": false, "emit_agent": false, "emit_device": false}, &opts)

		msg, ok := buildChirpStackMessage(csDetailMeasurementRecord(1), "n", csDetailAgent(), csDetailAgentName, opts)
		if !ok {
			t.Fatal("ok=false")
		}
		wantNoGroup(t, msg, "agent")
		wantNoGroup(t, msg, "device")
	})
}

// TestChirpStackDetail_MissingIDDropsGroup 은 id 를 확정할 수 없는 디바이스(UUID
// 미해석)에서 detail OFF 가 "dev_eui 만 남은 그룹" 을 남기지 않고 그룹 전체를
// 제거하는지 검증한다.
//
// 결정: id 가 없으면 그룹을 제거한다. detail OFF 의 계약은 "id 만 emit" 인데 id 가
// 없으면 emit 할 것이 없고, {} 나 {dev_eui} 를 남기면 계약이 깨진다.
func TestChirpStackDetail_MissingIDDropsGroup(t *testing.T) {
	// repo/DeviceInfo 미설정 + agentName="" → device.id 가 해석되지 않는다.
	// 그럼에도 setChirpStackDevEui 가 device{dev_eui} 그룹을 만든다.
	var onOpts MetadataEmitOptions
	parseEmitMetadata(map[string]any{}, &onOpts)
	onMsg, ok := buildChirpStackMessage(csDetailMeasurementRecord(1), "n", csDetailAgent(), "", onOpts)
	if !ok {
		t.Fatal("ok=false")
	}
	// 전제 확인: detail ON 에서는 id 없는 device{dev_eui} 그룹이 존재한다.
	wantGroup(t, onMsg, "device", map[string]string{"dev_eui": csDetailDevEui})

	var offOpts MetadataEmitOptions
	parseEmitMetadata(map[string]any{"emit_detail": false}, &offOpts)
	offMsg, ok := buildChirpStackMessage(csDetailMeasurementRecord(1), "n", csDetailAgent(), "", offOpts)
	if !ok {
		t.Fatal("ok=false")
	}
	// id 가 없으므로 그룹 전체 제거 — {} 도 {dev_eui} 도 아니다.
	wantNoGroup(t, offMsg, "device")
	// agent 는 id 가 있으므로 축소되어 남는다.
	wantGroup(t, offMsg, "agent", map[string]string{"id": "fake-cs-id"})
}

// ---------------------------------------------------------------------------
// AC-D5: 3종 메시지 kind 전부 적용 (per-measurement / combined / device_state)
// ---------------------------------------------------------------------------

// TestChirpStackDetail_CombinedMessage 는 combined 레코드에도 축소가 적용되며
// payload flat 키와 tags 는 보존되는지 검증한다.
func TestChirpStackDetail_CombinedMessage(t *testing.T) {
	wantUID := setupCSDetailDevice(t)
	ts := time.Date(2026, 8, 11, 23, 32, 1, 129_000_000, time.UTC)

	data, _ := json.Marshal(map[string]any{
		"record":  chirpStackRecordCombined,
		"values":  map[string]any{"magnet_status": "close", "battery": 98},
		"unit_id": csDetailDevEui,
		"time_ms": ts.UnixMilli(),
		"tags":    map[string]string{"location": "실습실"},
	})

	var opts MetadataEmitOptions
	parseEmitMetadata(map[string]any{"emit_detail": false}, &opts)

	msg, ok := buildChirpStackMessage(data, "n", csDetailAgent(), csDetailAgentName, opts)
	if !ok {
		t.Fatal("ok=false")
	}
	wantGroup(t, msg, "agent", map[string]string{"id": "fake-cs-id"})
	wantGroup(t, msg, "device", map[string]string{"id": wantUID})

	// frozen: tags / timestamp / payload flat 키 보존.
	wantGroup(t, msg, "tags", map[string]string{"location": "실습실"})
	if !msg.Timestamp().Equal(ts) {
		t.Errorf("timestamp = %v, want %v", msg.Timestamp().UTC(), ts)
	}
	if v, _ := msg.Payload().Get("magnet_status"); v != "close" {
		t.Errorf("payload.magnet_status = %v, want close", v)
	}
	if v, _ := msg.Payload().Get("battery"); v == nil {
		t.Error("payload.battery 누락")
	}
}

// TestChirpStackDetail_DeviceStateMessage 는 device_state 레코드에도 축소가 적용되며
// type / payload.state 는 보존되는지 검증한다.
func TestChirpStackDetail_DeviceStateMessage(t *testing.T) {
	wantUID := setupCSDetailDevice(t)
	ts := time.Date(2026, 8, 11, 23, 32, 1, 129_000_000, time.UTC)

	data, _ := json.Marshal(map[string]any{
		"record":       "device_state",
		"trigger":      "change",
		"unit_id":      csDetailDevEui,
		"time_ms":      ts.UnixMilli(),
		"last_seen_ms": ts.UnixMilli(),
		"state": map[string]any{
			"online": true, "rssi": -57, "snr": 13.5,
			"gateway_id": "24e124fffef79304", "last_seen_ms": ts.UnixMilli(),
		},
	})

	var opts MetadataEmitOptions
	parseEmitMetadata(map[string]any{"emit_detail": false}, &opts)

	msg, ok := buildChirpStackMessage(data, "n", csDetailAgent(), csDetailAgentName, opts)
	if !ok {
		t.Fatal("ok=false")
	}
	wantGroup(t, msg, "agent", map[string]string{"id": "fake-cs-id"})
	wantGroup(t, msg, "device", map[string]string{"id": wantUID})

	// frozen: type / state 보존.
	if msg.Type() != "device_state.change" {
		t.Errorf("type = %q, want device_state.change", msg.Type())
	}
	stateRaw, ok := msg.Payload().Get("state")
	if !ok {
		t.Fatal("payload.state 누락")
	}
	state, ok := stateRaw.(map[string]any)
	if !ok {
		t.Fatalf("payload.state 타입 = %T", stateRaw)
	}
	if state["online"] != true || state["gateway_id"] != "24e124fffef79304" {
		t.Errorf("payload.state 변형됨: %v", state)
	}
}

// ---------------------------------------------------------------------------
// AC-D6: control / status 노드에도 적용
// ---------------------------------------------------------------------------

// TestChirpStackDetail_ControlNode 는 control 노드의 응답 메시지에서 agent 그룹이
// 축소되고, chirpstack_command / chirpstack_downlink_topic flat metadata 는
// 보존되는지 검증한다.
func TestChirpStackDetail_ControlNode(t *testing.T) {
	fa := &fakeChirpStackAgent{appID: "app-1", profile: "WS301"}
	n := newTestChirpStackControlNode(t, fa)
	if err := n.Configure(map[string]any{"agent_ref": "cs-agent", "emit_detail": false}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	in := message.New()
	in.Payload().Set("unit_id", csDetailDevEui)
	in.Payload().Set("command", "reboot")

	out, err := n.Process(context.Background(), in)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("출력 %d건, want 1", len(out))
	}
	wantGroup(t, out[0], "agent", map[string]string{"id": "fake-cs-id"})

	// frozen: control 노드 고유 flat metadata 는 보존.
	if v, _ := out[0].Metadata().Get("chirpstack_command"); v != "reboot" {
		t.Errorf("metadata.chirpstack_command = %q, want reboot", v)
	}
	if v, _ := out[0].Metadata().Get("chirpstack_downlink_topic"); v == "" {
		t.Error("metadata.chirpstack_downlink_topic 누락")
	}
	if out[0].Type() != "response" {
		t.Errorf("type = %q, want response", out[0].Type())
	}
}

// TestChirpStackDetail_ControlNodeDefault 는 control 노드의 기본(키 부재) 동작이
// 오늘과 동일한 전체 agent 그룹인지 검증한다.
func TestChirpStackDetail_ControlNodeDefault(t *testing.T) {
	fa := &fakeChirpStackAgent{appID: "app-1", profile: "WS301"}
	n := newTestChirpStackControlNode(t, fa)

	in := message.New()
	in.Payload().Set("unit_id", csDetailDevEui)
	in.Payload().Set("command", "reboot")

	out, err := n.Process(context.Background(), in)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	wantGroup(t, out[0], "agent", map[string]string{
		"type": "chirpstack-client", "id": "fake-cs-id", "name": "fake-cs",
	})
}

// TestChirpStackDetail_StatusNode 는 status 노드의 device_state 응답에서 agent/device
// 가 축소되고 payload.state 는 보존되는지 검증한다.
func TestChirpStackDetail_StatusNode(t *testing.T) {
	wantUID := setupCSDetailDevice(t)
	lastSeen := time.Date(2026, 8, 11, 23, 32, 1, 129_000_000, time.UTC)

	fa := &fakeCommStateAgent{
		commOn: true,
		entry: &fakeCommEntry{
			online: true, rssi: -90, snr: 7.5,
			gatewayID: "gw1", lastSeenMs: lastSeen.UnixMilli(),
		},
	}
	n := newTestChirpStackStatusNode(t, fa)
	if err := n.Configure(map[string]any{"agent_ref": "cs-agent", "emit_detail": false}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	in := message.New()
	in.Payload().Set("unit_id", csDetailDevEui)

	out, err := n.Process(context.Background(), in)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("출력 %d건, want 1", len(out))
	}

	// status 노드의 에이전트 이름은 fakeCommStateAgent 기준이다.
	wantGroup(t, out[0], "agent", map[string]string{"id": "fake-cs-status-id"})
	// device.id 는 status 노드가 pre-capture 한 agentName("fake-cs-status") 기준으로
	// 해석되므로, csDetailAgentName 의 UUID 와는 다른 UUID 가 발급된다. 값 자체보다
	// "id 키 하나만 남는다" 가 계약이다.
	dg, ok := out[0].Metadata().GetGroup("device")
	if !ok {
		t.Fatal("metadata.device 그룹 누락")
	}
	if len(dg) != 1 || dg["id"] == "" {
		t.Errorf("metadata.device = %v, want id 키 하나만", dg)
	}
	if dg["id"] == wantUID {
		t.Logf("참고: status 노드 UUID 가 픽스처 UUID 와 동일 (%s)", dg["id"])
	}

	// frozen: state payload 보존.
	stateRaw, ok := out[0].Payload().Get("state")
	if !ok {
		t.Fatal("payload.state 누락")
	}
	state, _ := stateRaw.(map[string]any)
	if state["online"] != true || state["gateway_id"] != "gw1" {
		t.Errorf("payload.state 변형됨: %v", state)
	}
}

// ---------------------------------------------------------------------------
// AC-D7: 공유 헬퍼 불변 — 비 chirpstack 노드 영향 없음
// ---------------------------------------------------------------------------

// TestChirpStackDetail_SharedHelpersUnaffected 는 detail 축소가 chirpstack 로컬임을
// 검증한다. mqtt / serial / tcp / modbus 가 쓰는 공유 헬퍼(emitAgentGroup) 는
// Detail 값과 무관하게 전체 그룹을 만들어야 한다.
func TestChirpStackDetail_SharedHelpersUnaffected(t *testing.T) {
	// DefaultEmitOptions 를 쓰는 노드(serial_io / mqtt / tcp_io / modbus)의 기본값에
	// Detail 이 포함되어야 한다 — zero-value false 로 새면 그 노드들의 metadata 가
	// 조용히 축소된다.
	if !DefaultEmitOptions().Detail {
		t.Error("DefaultEmitOptions().Detail = false, want true")
	}

	for _, tc := range []struct {
		name string
		opts MetadataEmitOptions
	}{
		{"DefaultEmitOptions", DefaultEmitOptions()},
		{"Detail 명시 OFF", MetadataEmitOptions{Agent: true, Device: true, Detail: false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := message.New()
			emitAgentGroup(msg, csDetailAgent(), tc.opts)
			// 공유 헬퍼는 축소하지 않는다 — chirpstack 노드가 아니면 전체 그룹.
			wantGroup(t, msg, "agent", map[string]string{
				"type": "chirpstack-client", "id": "fake-cs-id", "name": "fake-cs",
			})

			mergeDeviceGroup(msg, tc.opts, "WS301", "dev-uuid", "라벨")
			wantGroup(t, msg, "device", map[string]string{
				"type": "WS301", "id": "dev-uuid", "name": "라벨",
			})
		})
	}
}

// TestChirpStackDetail_MQTTNodeMetadataUnchanged 는 실제 비 chirpstack 노드(mqtt)의
// config 파싱 경로에서 emit_detail 이 그룹 emit 정책을 바꾸지 않는지 검증한다.
//
// mqtt 노드는 축소 헬퍼를 호출하지 않으므로, emit_detail:false 를 넣어도 emit 되는
// agent 그룹은 전체 필드를 유지해야 한다.
func TestChirpStackDetail_MQTTNodeMetadataUnchanged(t *testing.T) {
	var cfg MQTTNodeConfig
	parseEmitMetadata(map[string]any{"emit_detail": false}, &cfg.EmitMetadata)

	msg := message.New()
	msg.Metadata().Set("node_id", "mqtt-1")
	emitAgentGroup(msg, csDetailAgent(), cfg.EmitMetadata)

	wantGroup(t, msg, "agent", map[string]string{
		"type": "chirpstack-client", "id": "fake-cs-id", "name": "fake-cs",
	})
	if v, _ := msg.Metadata().Get("node_id"); v != "mqtt-1" {
		t.Errorf("metadata.node_id = %q, want mqtt-1", v)
	}
}
