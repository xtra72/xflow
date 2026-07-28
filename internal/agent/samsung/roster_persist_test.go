package samsung

import (
	"testing"
)

// TestGetPersistableDevices_ExcludesAutoDiscovered 는 auto-discovery 로 발견된
// device 가 영속 대상에서 제외되는지 검증한다.
//
// auto device 를 config 에 저장하면, 버스에서 사라진 유령 device 가 설정에
// 영구히 쌓인다. auto device 는 재시작 후 device 가 다시 송신하면 스스로 복구되므로
// 저장할 이유가 없다.
func TestGetPersistableDevices_ExcludesAutoDiscovered(t *testing.T) {
	a, _ := newConnAgent(t, "200001")
	a.hvacr01Config.AutoDiscovery = true

	cfgAddr, err := ParseNasaAddress("200001")
	if err != nil {
		t.Fatalf("config 주소 파싱 실패: %v", err)
	}

	// 미등록 주소에서 프레임 수신 → auto-discovery 로 device 등록
	autoAddr, err := ParseNasaAddress("200002")
	if err != nil {
		t.Fatalf("auto 주소 파싱 실패: %v", err)
	}
	a.handleMessage(&NasaMessage{
		SourceAddr:  autoAddr,
		DestAddr:    AddrController,
		CommandCode: CmdNormalRequest,
		MessageSets: []NasaMessageSet{{Index: MsgPower, Value: []byte{0x01}}},
	})

	// 테스트 전제 확인: auto-discovery 가 실제로 동작했는가
	a.mu.RLock()
	autoDev, autoRegistered := a.devices[autoAddr]
	a.mu.RUnlock()
	if !autoRegistered {
		t.Fatalf("auto-discovery 가 트리거되지 않음 — 테스트 전제 실패")
	}
	if autoDev.Source != "auto" {
		t.Fatalf("auto device 의 Source: got %q, want \"auto\"", autoDev.Source)
	}

	got := a.GetPersistableDevices()

	if len(got) != 1 {
		t.Fatalf("영속 대상 device 수: got %d, want 1 (auto 제외). got=%+v", len(got), got)
	}
	if got[0].Address != cfgAddr.String() {
		t.Errorf("영속 대상 주소: got %q, want %q", got[0].Address, cfgAddr.String())
	}
	for _, d := range got {
		if d.Address == autoAddr.String() {
			t.Errorf("auto-discovery device 가 영속 대상에 포함됨: %+v", d)
		}
	}
}

// TestGetPersistableDevices_PreservesConfigUnitID 는 config 로 등록된 device 의
// UnitID 가 DeviceEntry.Name 슬롯으로 보존되는지 검증한다.
//
// 회귀 방지: GetPersistableDevices 가 dev.Name 을 읽던 시절, config device 는
// Name 이 비어 있어(이름은 UnitID 에 저장됨) unit id 가 통째로 소실됐다.
// UnitID 는 ResolveDeviceID 의 입력이므로 소실되면 재시작 후 device_id(UUID)가 바뀐다.
func TestGetPersistableDevices_PreservesConfigUnitID(t *testing.T) {
	a, _ := newConnAgent(t, "200001")

	addr, err := ParseNasaAddress("200001")
	if err != nil {
		t.Fatalf("주소 파싱 실패: %v", err)
	}

	a.mu.Lock()
	a.devices[addr].UnitID = "living-room"
	a.devices[addr].Name = "" // config 경로는 Name 을 채우지 않는다
	a.mu.Unlock()

	got := a.GetPersistableDevices()
	if len(got) != 1 {
		t.Fatalf("영속 대상 device 수: got %d, want 1", len(got))
	}
	if got[0].Name != "living-room" {
		t.Errorf("UnitID 소실: DeviceEntry.Name = %q, want \"living-room\"", got[0].Name)
	}
}

// TestGetPersistableDevices_PreservesDisplayName 는 런타임 등록 device 의
// 사용자 표시 이름(dev.Name)이 DeviceEntry.DisplayName 슬롯으로 보존되고,
// Name 슬롯은 여전히 UnitID(device_id)를 실어 두 값이 독립적으로 왕복하는지 검증한다.
//
// 회귀 방지: DisplayName 슬롯이 없던 시절, 직렬화는 Name=UnitID 만 실었고 dev.Name
// (표시 이름)은 통째로 버려져 재시작 후 id 로 표시됐다.
func TestGetPersistableDevices_PreservesDisplayName(t *testing.T) {
	a, _ := newConnAgent(t, "200001")

	addr, err := ParseNasaAddress("200001")
	if err != nil {
		t.Fatalf("주소 파싱 실패: %v", err)
	}

	a.mu.Lock()
	a.devices[addr].Source = "bridge"
	a.devices[addr].UnitID = "dev-uuid-001" // device_id
	a.devices[addr].Name = "개발팀"            // 사용자 표시 이름
	a.mu.Unlock()

	got := a.GetPersistableDevices()
	if len(got) != 1 {
		t.Fatalf("영속 대상 device 수: got %d, want 1", len(got))
	}
	if got[0].Name != "dev-uuid-001" {
		t.Errorf("Name 슬롯(UnitID) 손상: got %q, want \"dev-uuid-001\"", got[0].Name)
	}
	if got[0].DisplayName != "개발팀" {
		t.Errorf("표시 이름 소실: DeviceEntry.DisplayName = %q, want \"개발팀\"", got[0].DisplayName)
	}
}

// TestGetPersistableDevices_EmptyDisplayName 는 표시 이름 없는 device 가
// 빈 DisplayName 으로 왕복하는지(후방호환) 검증한다.
func TestGetPersistableDevices_EmptyDisplayName(t *testing.T) {
	a, _ := newConnAgent(t, "200001")

	addr, err := ParseNasaAddress("200001")
	if err != nil {
		t.Fatalf("주소 파싱 실패: %v", err)
	}

	a.mu.Lock()
	a.devices[addr].UnitID = "living-room"
	a.devices[addr].Name = "" // 표시 이름 없음
	a.mu.Unlock()

	got := a.GetPersistableDevices()
	if len(got) != 1 {
		t.Fatalf("영속 대상 device 수: got %d, want 1", len(got))
	}
	if got[0].DisplayName != "" {
		t.Errorf("빈 표시 이름이 보존되지 않음: DisplayName = %q, want \"\"", got[0].DisplayName)
	}
	// 무회귀: 표시 이름과 무관하게 UnitID 는 Name 슬롯으로 보존.
	if got[0].Name != "living-room" {
		t.Errorf("UnitID 소실: Name = %q, want \"living-room\"", got[0].Name)
	}
}

// TestGetPersistableDevices_EmptyUnitID 는 이름 없는 config device 가
// 빈 Name 으로 일관되게 왕복하는지 검증한다.
func TestGetPersistableDevices_EmptyUnitID(t *testing.T) {
	a, _ := newConnAgent(t, "200001")

	addr, err := ParseNasaAddress("200001")
	if err != nil {
		t.Fatalf("주소 파싱 실패: %v", err)
	}

	a.mu.Lock()
	a.devices[addr].UnitID = ""
	a.mu.Unlock()

	got := a.GetPersistableDevices()
	if len(got) != 1 {
		t.Fatalf("영속 대상 device 수: got %d, want 1", len(got))
	}
	if got[0].Name != "" {
		t.Errorf("빈 UnitID 가 보존되지 않음: got %q, want \"\"", got[0].Name)
	}
}
