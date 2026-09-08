package chirpstack

import (
	"testing"

	"github.com/xtra/xflow/internal/agent"
)

// 고정 설치 디바이스 복원 검증.
//
// 이 에이전트의 디바이스는 업링크가 도착해야 발견된다. LoRaWAN 센서는 보고 주기가
// 길어, 재시작 후 첫 업링크까지 로스터에서 사라진 것처럼 보였다 — "재시작 시에도
// 디바이스를 유지합니다"라는 고정 설치의 약속이 지켜지지 않던 지점이다.

func newPinnedTestAgent(t *testing.T, name string) *ChirpStackAgent {
	t.Helper()
	resetNameRegistryForTest()

	a, err := NewChirpStackAgent(newTestConfig("id-"+name, name))
	if err != nil {
		t.Fatalf("NewChirpStackAgent: %v", err)
	}
	cs, ok := a.(*ChirpStackAgent)
	if !ok {
		t.Fatalf("타입 단언 실패: %T", a)
	}
	return cs
}

func TestRegisterPinnedDevices_CreatesOfflinePlaceholder(t *testing.T) {
	a := newPinnedTestAgent(t, "cs-pin-1")

	a.RegisterPinnedDevices([]agent.DeviceEntry{{Address: "24e124725d081175"}})

	devices := a.listDevices()
	if len(devices) != 1 {
		t.Fatalf("디바이스 1개를 기대했으나 %d개", len(devices))
	}
	d := devices[0]
	if d.devEui != "24e124725d081175" {
		t.Errorf("devEui = %q, 기대 %q", d.devEui, "24e124725d081175")
	}
	// 업링크가 오기 전이므로 값을 지어내지 않는다.
	if !d.lastSeen.IsZero() {
		t.Errorf("lastSeen 은 제로여야 한다(오프라인): %v", d.lastSeen)
	}
	if len(d.measurements) != 0 {
		t.Errorf("측정치는 비어 있어야 한다: %v", d.measurements)
	}
}

func TestRegisterPinnedDevices_PlaceholderIsOffline(t *testing.T) {
	a := newPinnedTestAgent(t, "cs-pin-2")
	a.RegisterPinnedDevices([]agent.DeviceEntry{{Address: "aabbccddeeff0011"}})

	provider := a.DeviceProvider()
	devs := provider.Devices()
	if len(devs) != 1 {
		t.Fatalf("디바이스 1개를 기대했으나 %d개", len(devs))
	}
	if devs[0].Online() {
		t.Error("자리표시자는 오프라인이어야 한다 — 업링크가 아직 없다")
	}
}

// LocalID 가 devEui 여야 복원이 성립한다. Name() 은 ChirpStack 라벨이라 로스터 키와
// 다르므로, 라벨로 저장하면 다음 부팅에서 엉뚱한 키의 디바이스가 생긴다.
func TestDeviceAdapter_LocalIDIsDevEui(t *testing.T) {
	a := newPinnedTestAgent(t, "cs-pin-3")
	a.RegisterPinnedDevices([]agent.DeviceEntry{{Address: "1122334455667788"}})

	devs := a.DeviceProvider().Devices()
	if len(devs) != 1 {
		t.Fatalf("디바이스 1개를 기대했으나 %d개", len(devs))
	}
	lp, ok := devs[0].(interface{ LocalID() string })
	if !ok {
		t.Fatal("어댑터가 LocalID() 를 구현해야 한다 — 없으면 Name() 으로 폴백해 라벨이 저장된다")
	}
	if lp.LocalID() != "1122334455667788" {
		t.Errorf("LocalID = %q, 기대 devEui", lp.LocalID())
	}
}

func TestRegisterPinnedDevices_DoesNotOverwriteDiscovered(t *testing.T) {
	a := newPinnedTestAgent(t, "cs-pin-4")

	// 이미 발견되어 값이 채워진 디바이스.
	a.devicesMu.Lock()
	a.devices["deadbeef00000001"] = &deviceState{
		devEui:     "deadbeef00000001",
		deviceName: "실제 라벨",
	}
	a.devicesMu.Unlock()

	a.RegisterPinnedDevices([]agent.DeviceEntry{{Address: "deadbeef00000001"}})

	devices := a.listDevices()
	if len(devices) != 1 {
		t.Fatalf("디바이스 1개를 기대했으나 %d개", len(devices))
	}
	if devices[0].deviceName != "실제 라벨" {
		t.Errorf("발견된 값을 덮어써서는 안 된다: deviceName = %q", devices[0].deviceName)
	}
}

func TestRegisterPinnedDevices_IgnoresEmptyAddress(t *testing.T) {
	a := newPinnedTestAgent(t, "cs-pin-5")

	a.RegisterPinnedDevices([]agent.DeviceEntry{{Address: ""}, {Address: "  "}})

	// 빈 주소는 로스터 키가 될 수 없다. 공백 문자열은 키로는 유효하므로 만들어지되,
	// 빈 문자열만은 건너뛴다.
	for _, d := range a.listDevices() {
		if d.devEui == "" {
			t.Error("빈 devEui 디바이스가 만들어졌다")
		}
	}
}
