package node

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/xtra/xflow/internal/agent"
)

// memDeviceIDRepo 는 테스트용 인메모리 DeviceIDRepository 이다.
type memDeviceIDRepo struct {
	mu sync.Mutex
	m  map[string]string
}

func (r *memDeviceIDRepo) key(a, u string) string { return a + "|" + u }
func (r *memDeviceIDRepo) GetOrCreate(_ context.Context, a, u string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.key(a, u)
	if v, ok := r.m[k]; ok {
		return v, nil
	}
	id := uuid.New().String()
	r.m[k] = id
	return id, nil
}
func (r *memDeviceIDRepo) Get(_ context.Context, a, u string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.m[r.key(a, u)], nil
}
func (r *memDeviceIDRepo) Set(_ context.Context, a, u, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[r.key(a, u)] = id
	return nil
}

// TestBuildChirpStackMessage_DeviceGroupGolden 은 노드가 unit_id 를 device 그룹으로
// 승격하여 다운스트림 계약(device.id UUID + device.name)을 완성하는지 검증한다
// (AC-1a device 그룹, AC-2, REQ-M3-04, REQ-FROZEN-02).
//
// 에이전트 M4 등록(ResolveDeviceID + SetDeviceInfo)을 시뮬레이션한 뒤 실측 픽스처
// 값으로 레코드를 빌드한다.
func TestBuildChirpStackMessage_DeviceGroupGolden(t *testing.T) {
	agent.SetDeviceIDRepository(&memDeviceIDRepo{m: make(map[string]string)})
	t.Cleanup(func() { agent.SetDeviceIDRepository(nil) })

	const agentName = "cs-golden"
	const devEui = "24e124141d180806"

	// 에이전트 디바이스 라이프사이클 시뮬레이션.
	wantUID := agent.ResolveDeviceID(context.Background(), agentName, devEui)
	if _, err := uuid.Parse(wantUID); err != nil {
		t.Fatalf("ResolveDeviceID did not yield UUID: %q", wantUID)
	}
	agent.SetDeviceInfo(agentName, devEui, agent.DeviceInfo{DeviceType: "WS301", Label: "WS301-180806"})

	wantTS := time.Date(2026, 8, 11, 23, 32, 1, 129_000_000, time.UTC)
	rec := map[string]any{
		"measurement": "magnet_status",
		"value":       "close",
		"unit_id":     devEui,
		"time_ms":     wantTS.UnixMilli(),
		"tags":        map[string]string{"location": "실습실", "point": "앞문", "spot": "앞문"},
	}
	data, _ := json.Marshal(rec)

	msg, ok := buildChirpStackMessage(data, "node-1", nil, agentName, DefaultEmitOptions())
	if !ok {
		t.Fatal("buildChirpStackMessage ok=false")
	}

	// 다운스트림 계약 전체 (REQ-FROZEN-02).
	if msg.Type() != "event" {
		t.Errorf("type = %q", msg.Type())
	}
	if !msg.Timestamp().Equal(wantTS) {
		t.Errorf("timestamp = %v, want %v", msg.Timestamp().UTC(), wantTS)
	}
	if v, _ := msg.Payload().Get("value"); v != "close" {
		t.Errorf("payload.value = %v", v)
	}
	if m, _ := msg.Metadata().Get("measurement"); m != "magnet_status" {
		t.Errorf("metadata.measurement = %q", m)
	}
	tags, _ := msg.Metadata().GetGroup("tags")
	if tags["location"] != "실습실" || tags["point"] != "앞문" || tags["spot"] != "앞문" {
		t.Errorf("metadata.tags = %v", tags)
	}

	// device 그룹 승격 (REQ-M3-04): id(UUID)/name/type.
	dg, ok := msg.Metadata().GetGroup("device")
	if !ok {
		t.Fatal("metadata.device group missing")
	}
	if dg["id"] != wantUID {
		t.Errorf("device.id = %q, want %q", dg["id"], wantUID)
	}
	if dg["name"] != "WS301-180806" {
		t.Errorf("device.name = %q, want WS301-180806", dg["name"])
	}
	if dg["type"] != "WS301" {
		t.Errorf("device.type = %q, want WS301", dg["type"])
	}
}
