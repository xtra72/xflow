package chirpstack

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/xtra/xflow/internal/agent"
)

// memDeviceIDRepo 는 테스트용 인메모리 DeviceIDRepository 이다.
type memDeviceIDRepo struct {
	mu sync.Mutex
	m  map[string]string
}

func newMemDeviceIDRepo() *memDeviceIDRepo {
	return &memDeviceIDRepo{m: make(map[string]string)}
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

// withMemDeviceIDRepo 는 인메모리 device-id 저장소를 설치하고 정리 훅을 건다.
func withMemDeviceIDRepo(t *testing.T) {
	t.Helper()
	agent.SetDeviceIDRepository(newMemDeviceIDRepo())
	t.Cleanup(func() { agent.SetDeviceIDRepository(nil) })
}

// TestChirpStackAgent_DeviceAutoCreateAndUID 는 첫 업링크 시 디바이스 자동 생성 +
// UID 발급 + Provider 노출을 검증한다 (AC-3a, REQ-M4-01/03).
func TestChirpStackAgent_DeviceAutoCreateAndUID(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "prov-a")

	a.handleUplink(loadRawUplink(t), "application/x")

	devs := a.DeviceProvider().Devices()
	if len(devs) != 1 {
		t.Fatalf("Devices() len = %d, want 1", len(devs))
	}
	d := devs[0]
	if d.Name() != "WS301-180806" {
		t.Errorf("Name() = %q, want WS301-180806", d.Name())
	}
	if _, err := uuid.Parse(d.UID()); err != nil {
		t.Errorf("UID() = %q, not a UUID: %v", d.UID(), err)
	}
	if d.ID() != d.UID() {
		t.Errorf("ID()=%q != UID()=%q", d.ID(), d.UID())
	}
	// Provider.Device(UID) 조회.
	got, err := a.DeviceProvider().Device(d.UID())
	if err != nil || got.UID() != d.UID() {
		t.Errorf("Device(UID) lookup failed: %v", err)
	}
}

// TestChirpStackAgent_ReUplinkSameUID 는 동일 devEui 재업링크가 동일 UID 를
// 재사용하는지 검증한다 (AC-3b, REQ-M4-01).
func TestChirpStackAgent_ReUplinkSameUID(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "prov-b")

	raw := loadRawUplink(t)
	a.handleUplink(raw, "application/x")
	uid1 := a.DeviceProvider().Devices()[0].UID()

	a.handleUplink(raw, "application/x")
	devs := a.DeviceProvider().Devices()
	if len(devs) != 1 {
		t.Fatalf("Devices() len = %d, want 1 (no duplicate)", len(devs))
	}
	if devs[0].UID() != uid1 {
		t.Errorf("UID changed on re-uplink: %q → %q", uid1, devs[0].UID())
	}
}

// TestChirpStackAgent_MetadataPersistence 는 SetDeviceInfo 등록과 메타데이터
// (Name/tags→Labels) 지속화를 검증한다 (AC-3c, REQ-M4-02/04).
func TestChirpStackAgent_MetadataPersistence(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "prov-c")

	a.handleUplink(loadRawUplink(t), "application/x")

	// SetDeviceInfo 등록 확인 (DeviceType=deviceProfileName, Label=deviceName).
	info, ok := agent.GetDeviceInfo(a.Name(), "24e124141d180806")
	if !ok {
		t.Fatal("GetDeviceInfo returned ok=false")
	}
	if info.DeviceType != "WS301" || info.Label != "WS301-180806" {
		t.Errorf("DeviceInfo = %+v, want {WS301, WS301-180806}", info)
	}

	// 메타데이터: Name + tags→Labels(verbatim).
	meta := a.DeviceProvider().Devices()[0].Metadata()
	if meta.Name != "WS301-180806" {
		t.Errorf("Metadata.Name = %q", meta.Name)
	}
	if meta.Labels["location"] != "실습실" || meta.Labels["point"] != "앞문" || meta.Labels["spot"] != "앞문" {
		t.Errorf("Metadata.Labels tags mismatch: %v", meta.Labels)
	}
}

// TestChirpStackAgent_DeviceProviderAutoWireable 는 main.go 의 인터페이스 어서션
// (DeviceProvider() device.DeviceProvider)을 만족하는지 컴파일 타임에 보증한다.
func TestChirpStackAgent_DeviceProviderAutoWireable(t *testing.T) {
	a := newRunningTestAgent(t, "prov-wire")
	if a.DeviceProvider() == nil {
		t.Fatal("DeviceProvider() returned nil")
	}
}
