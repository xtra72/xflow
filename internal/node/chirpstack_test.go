package node

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/xtra/xflow/pkg/flow"
)

// TestBuildChirpStackMessage_DownstreamContract 는 에이전트 레코드로부터 빌드된
// 메시지가 다운스트림 계약을 보존하는지 검증한다 (REQ-FROZEN-01/02, AC-1a/2).
//
// device 그룹(id UUID/name) 승격은 DeviceIDRepository + SetDeviceInfo 가 필요하므로
// M4 통합 테스트에서 검증한다. 본 테스트는 device 를 제외한 계약 경로를 검증한다.
func TestBuildChirpStackMessage_DownstreamContract(t *testing.T) {
	// 2026-08-11T23:32:01.129Z 에 해당하는 UnixMilli.
	wantTS := time.Date(2026, 8, 11, 23, 32, 1, 129_000_000, time.UTC)
	rec := map[string]any{
		"measurement": "magnet_status",
		"value":       "close",
		"unit_id":     "24e124141d180806",
		"time_ms":     wantTS.UnixMilli(),
		"tags":        map[string]string{"location": "실습실", "point": "앞문", "spot": "앞문"},
	}
	data, _ := json.Marshal(rec)

	// agentName="" + repo 미설정 → device 그룹은 비어있고 unit_id 는 payload 에서 제거됨.
	msg, ok := buildChirpStackMessage(data, "node-1", nil, "", DefaultEmitOptions())
	if !ok {
		t.Fatal("buildChirpStackMessage returned ok=false")
	}

	if msg.Type() != "event" {
		t.Errorf("type = %q, want event", msg.Type())
	}
	if !msg.Timestamp().Equal(wantTS) {
		t.Errorf("timestamp = %v, want %v", msg.Timestamp().UTC(), wantTS)
	}
	if v, _ := msg.Payload().Get("value"); v != "close" {
		t.Errorf("payload.value = %v, want close", v)
	}
	if _, ok := msg.Payload().Get("unit_id"); ok {
		t.Error("payload.unit_id should be removed after device promotion")
	}
	if m, _ := msg.Metadata().Get("measurement"); m != "magnet_status" {
		t.Errorf("metadata.measurement = %q, want magnet_status", m)
	}
	tags, ok := msg.Metadata().GetGroup("tags")
	if !ok {
		t.Fatal("metadata.tags group missing")
	}
	if tags["location"] != "실습실" || tags["point"] != "앞문" || tags["spot"] != "앞문" {
		t.Errorf("tags verbatim mismatch: %v", tags)
	}
}

// TestBuildChirpStackMessage_NonScalarSafe 는 잘못된 JSON 이 ok=false 로 안전하게
// 처리되는지 검증한다.
func TestBuildChirpStackMessage_BadJSON(t *testing.T) {
	if _, ok := buildChirpStackMessage([]byte("not-json"), "n", nil, "", DefaultEmitOptions()); ok {
		t.Error("bad JSON should return ok=false")
	}
}

// TestChirpStackInNode_RegisteredAndFactory 는 chirpstack-in 이 노드 레지스트리에
// 등록되고 팩토리가 SourceNode 를 생성하는지 검증한다 (REQ-M3-06, AC-8).
func TestChirpStackInNode_RegisteredAndFactory(t *testing.T) {
	r := NewRegistry()
	if !r.Has("chirpstack-in") {
		t.Fatal("chirpstack-in not registered in node registry")
	}

	n, err := r.Create(flow.NodeDef{ID: "n1", Name: "cs", Type: "chirpstack-in"})
	if err != nil {
		t.Fatalf("Create(chirpstack-in): %v", err)
	}
	if _, ok := n.(SourceNode); !ok {
		t.Error("chirpstack-in node should implement SourceNode")
	}
}

// TestChirpStackInNode_ConfigureRequiresAgentRef 는 agent_ref 누락 시 Configure 가
// 에러를 반환하는지 검증한다.
func TestChirpStackInNode_ConfigureRequiresAgentRef(t *testing.T) {
	n, err := NewChirpStackInNode(flow.NodeDef{ID: "n1", Name: "cs", Type: "chirpstack-in"})
	if err != nil {
		t.Fatalf("NewChirpStackInNode: %v", err)
	}
	if err := n.Configure(map[string]any{}); err == nil {
		t.Error("Configure without agent_ref should error")
	}
	if err := n.Configure(map[string]any{"agent_ref": "cs-agent"}); err != nil {
		t.Errorf("Configure with agent_ref: %v", err)
	}
}
