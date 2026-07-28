package lg

import (
	"testing"
	"time"
)

// TestGetPersistableDevices_ZoneAddressRoundTrip 는 영속 저장된 zone 주소 문자열이
// parseZoneKey 로 원래 zone 바이트와 동일하게 복원되는지 검증한다.
//
// GetPersistableDevices 는 zone 을 "0x%x" 로 포맷한다. 이 문자열이 재시작 시
// ParseDevices → parseZoneKey 경로를 거쳐 동일한 zone 으로 돌아오지 않으면
// device 가 엉뚱한 zone 에 등록된다.
func TestGetPersistableDevices_ZoneAddressRoundTrip(t *testing.T) {
	// zone 0 은 "0x0" (길이 3) 으로, parseZoneKey 의 16진 접두사 분기 경계값이다.
	zones := []byte{0x00, 0x01, 0x10, 0x1f, 0xff}

	a, _ := newConnLGAPAgent(t, time.Hour, time.Hour, zones...)

	got := a.GetPersistableDevices()
	if len(got) != len(zones) {
		t.Fatalf("영속 대상 device 수: got %d, want %d", len(got), len(zones))
	}

	seen := make(map[byte]bool)
	for _, d := range got {
		zone := byte(toInt(parseZoneKey(d.Address)))
		seen[zone] = true
	}

	for _, want := range zones {
		if !seen[want] {
			t.Errorf("zone 0x%x 가 왕복하지 않음. 저장된 목록: %+v", want, got)
		}
	}
}

// TestGetPersistableDevices_PreservesConfigUnitID 는 config 로 등록된 LGAP device 의
// UnitID 가 DeviceEntry.Name 슬롯으로 보존되는지 검증한다.
//
// 회귀 방지: GetPersistableDevices 가 dev.Name 을 읽던 시절, config device 는
// Name 이 비어 있어(이름은 UnitID 에 저장됨) unit id 가 소실됐다.
// UnitID 는 ResolveDeviceID 의 입력이므로 소실되면 재시작 후 device_id(UUID)가 바뀐다.
func TestGetPersistableDevices_PreservesConfigUnitID(t *testing.T) {
	const zone byte = 0x10

	a, _ := newConnLGAPAgent(t, time.Hour, time.Hour, zone)

	a.mu.Lock()
	a.devices[zone].UnitID = "living-room"
	a.devices[zone].Name = "" // config 경로는 Name 을 채우지 않는다
	a.mu.Unlock()

	got := a.GetPersistableDevices()
	if len(got) != 1 {
		t.Fatalf("영속 대상 device 수: got %d, want 1", len(got))
	}
	if got[0].Name != "living-room" {
		t.Errorf("UnitID 소실: DeviceEntry.Name = %q, want \"living-room\"", got[0].Name)
	}
	if zoneBack := byte(toInt(parseZoneKey(got[0].Address))); zoneBack != zone {
		t.Errorf("zone 왕복 실패: got 0x%x, want 0x%x (address=%q)", zoneBack, zone, got[0].Address)
	}
}

// TestGetPersistableDevices_EmptyUnitID 는 이름 없는 config device 가
// 빈 Name 으로 일관되게 왕복하는지 검증한다.
func TestGetPersistableDevices_EmptyUnitID(t *testing.T) {
	const zone byte = 0x20

	a, _ := newConnLGAPAgent(t, time.Hour, time.Hour, zone)

	a.mu.Lock()
	a.devices[zone].UnitID = ""
	a.mu.Unlock()

	got := a.GetPersistableDevices()
	if len(got) != 1 {
		t.Fatalf("영속 대상 device 수: got %d, want 1", len(got))
	}
	if got[0].Name != "" {
		t.Errorf("빈 UnitID 가 보존되지 않음: got %q, want \"\"", got[0].Name)
	}
}
