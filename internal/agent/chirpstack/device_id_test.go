// device_id_test.go 는 ChirpStack 디바이스의 device_id 계약을 검증한다.
//
// 계약: device_id 는 저장소가 발급한 UUID v4 이며, 에이전트는 이를 프로토콜 식별자
// (devEui)로 덮어쓰지 않는다. UUID 형식은 관례가 아니라 API 경계에서 강제되는
// 플랫폼 불변식이다 — device.ClassifyDeviceRef 가 UUID v4 또는 "agent/name" 만
// 인식하므로, device_id 가 devEui 이면 GET /devices/{ref} 가 404 로 떨어진다
// (SPEC-DEVICE-IDENTITY-001 Phase D).
//
// devEui 는 id 자리를 뺏는 대신 디바이스 정보(dev_eui)로 노출한다 — 그 계약은
// TestProperties_DevEui* 가 검증한다.
//
// 정규화/동시성 테스트가 여기 남아 있는 이유: devEui 는 로스터 맵 키 · comm 맵 키 ·
// 방출 레코드 unit_id · device_id 조회 키 네 곳으로 갈라지므로, 대소문자 변형이
// 한 곳만 새어도 동일 물리 디바이스가 두 개의 device_id 를 갖게 된다. 이는
// device_id 를 UUID 로 되돌린 뒤에도 그대로 성립하는 실제 동작이다.
package chirpstack

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/storage"
)

// uuidV4Re 는 device_id 가 UUID v4 인지 형식으로 검증한다.
//
// "devEui 가 아니다" 라는 부정 단언 대신 형식 정규식을 쓰는 이유: 부정 단언은 devEui
// 도 UUID 도 아닌 제3의 값(빈 문자열, 레거시 composite 등)을 통과시켜 회귀를 놓친다.
var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// withRealDeviceIDRepo 는 프로덕션과 동일한 in-memory 저장소 구현을 설치한다.
//
// 테스트 전용 스텁이 아니라 실제 구현(storage.DeviceIDMemoryRepository)을 쓰는 이유:
// "UUID 를 발급하는 주체" 가 곧 검증 대상이므로, 스텁이 UUID 를 돌려주면 아무것도
// 증명하지 못한다.
func withRealDeviceIDRepo(t *testing.T) *storage.DeviceIDMemoryRepository {
	t.Helper()
	repo := storage.NewDeviceIDMemoryRepository()
	agent.SetDeviceIDRepository(repo)
	t.Cleanup(func() { agent.SetDeviceIDRepository(nil) })
	return repo
}

// rawUplinkWithDevEui 는 지정 devEui 를 갖는 최소 업링크 JSON 을 만든다.
// 픽스처(loadRawUplink)는 devEui 가 고정이므로 대소문자 검증에는 쓸 수 없다.
func rawUplinkWithDevEui(t *testing.T, devEui string, obj map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"time": "2026-08-11T23:32:01.129+00:00",
		"deviceInfo": map[string]any{
			"devEui":            devEui,
			"deviceName":        "WS301-180806",
			"deviceProfileName": "WS301",
			"applicationId":     "96b4d719-f23f-40aa-9f94-a0f2d0354342",
		},
		"object": obj,
	})
	if err != nil {
		t.Fatalf("marshal uplink: %v", err)
	}
	return b
}

// TestDeviceID_IsUuidNotDevEui 는 업링크 1건 후 device_id 가 UUID v4 이며 devEui 가
// 아님을 검증한다. 저장소에 실제로 영속된 값까지 확인해, 재시작 후에도 동일한 UUID 로
// 해석되는지(=devEui 가 새어 들어가지 않았는지) 관측한다.
func TestDeviceID_IsUuidNotDevEui(t *testing.T) {
	repo := withRealDeviceIDRepo(t)
	a := newRunningTestAgent(t, "devid-uuid")

	a.handleUplink(loadRawUplink(t), "application/x")

	devs := a.DeviceProvider().Devices()
	if len(devs) != 1 {
		t.Fatalf("Devices() len = %d, want 1", len(devs))
	}
	d := devs[0]

	if !uuidV4Re.MatchString(d.UID()) {
		t.Errorf("UID() = %q, want UUID v4 (정규식 %s)", d.UID(), uuidV4Re)
	}
	if d.ID() != d.UID() {
		t.Errorf("ID()=%q != UID()=%q", d.ID(), d.UID())
	}
	if d.UID() == fixtureDevEui {
		t.Errorf("UID() 가 devEui(%q) 다 — device_id 는 UUID 여야 한다", fixtureDevEui)
	}

	// 저장소에 영속된 값도 UUID 여야 한다 (에이전트가 devEui 로 덮어쓰지 않았음).
	stored, err := repo.Get(context.Background(), a.Name(), fixtureDevEui)
	if err != nil {
		t.Fatalf("repo.Get: %v", err)
	}
	if stored != d.UID() {
		t.Errorf("저장소 값 = %q, want %q", stored, d.UID())
	}
	if !uuidV4Re.MatchString(stored) {
		t.Errorf("저장소에 비-UUID 매핑(%q)이 남아 있다", stored)
	}
}

// TestDeviceID_ClassifiesAsUuidRef 는 device_id 가 device.ClassifyDeviceRef 에서
// DeviceRefUUID 로 분류되는지 검증한다 — 회귀를 직접 봉인하는 단언이다.
//
// 이 분류가 DeviceRefUnknown 이면 GET /devices/{ref} 가 404 를 반환하고 UI 는
// "디바이스 정보를 불러올 수 없습니다" 를 띄운다. device 패키지는 읽기 전용으로만
// 참조한다(수정하지 않는다).
func TestDeviceID_ClassifiesAsUuidRef(t *testing.T) {
	withRealDeviceIDRepo(t)
	a := newRunningTestAgent(t, "devid-classify")

	a.handleUplink(loadRawUplink(t), "application/x")

	deviceID := a.DeviceProvider().Devices()[0].ID()
	if got := device.ClassifyDeviceRef(deviceID); got != device.DeviceRefUUID {
		t.Errorf("ClassifyDeviceRef(%q) = %v, want DeviceRefUUID — API 경계에서 404 가 된다",
			deviceID, got)
	}
	// 대조군: devEui 를 ref 로 넣으면 Unknown 이다. 이것이 회귀의 실패 기전이었다.
	if got := device.ClassifyDeviceRef(fixtureDevEui); got != device.DeviceRefUnknown {
		t.Errorf("ClassifyDeviceRef(devEui=%q) = %v, want DeviceRefUnknown", fixtureDevEui, got)
	}
}

// TestDeviceID_NilRepositoryIsGraceful 는 저장소 미설정(nil) 상태에서 panic 없이
// 업링크가 처리되고 측정 스트림이 흐르는지 검증한다 (graceful degradation 보존).
func TestDeviceID_NilRepositoryIsGraceful(t *testing.T) {
	agent.SetDeviceIDRepository(nil)

	a := newRunningTestAgent(t, "devid-nil-repo")
	a.handleUplink(loadRawUplink(t), "application/x")

	devs := a.DeviceProvider().Devices()
	if len(devs) != 1 {
		t.Fatalf("Devices() len = %d, want 1", len(devs))
	}
	// 저장소가 없으면 ResolveDeviceID 는 "" 를 반환한다(기존 계약 그대로).
	if got := devs[0].UID(); got != "" {
		t.Errorf("저장소 미설정 시 UID() = %q, want \"\"", got)
	}
	if devs[0].Name() != "WS301-180806" {
		t.Errorf("Name() = %q — 로스터는 저장소와 무관하게 채워져야 한다", devs[0].Name())
	}

	events, _ := drainRecords(t, a)
	if len(events) != 1 {
		t.Fatalf("방출 레코드 = %d건, want 1건", len(events))
	}
	if events[0].UnitID != fixtureDevEui {
		t.Errorf("레코드 unit_id = %q, want %q", events[0].UnitID, fixtureDevEui)
	}
}

// TestDevEui_CaseNormalized 는 대문자 devEui 가 소문자로 정규화되어 (1) 로스터 맵 키,
// (2) 방출 레코드 unit_id, (3) dev_eui 속성 세 곳에 일관되게 적용되는지 검증한다.
//
// 세 곳 중 하나라도 어긋나면 동일 물리 디바이스가 두 개의 device_id 로 쪼개진다.
func TestDevEui_CaseNormalized(t *testing.T) {
	withRealDeviceIDRepo(t)
	a := newRunningTestAgent(t, "deveui-case")

	upper := strings.ToUpper(fixtureDevEui)
	if upper == fixtureDevEui {
		t.Fatal("픽스처 devEui 에 hex 문자가 없어 대소문자 검증이 불가능하다")
	}
	a.handleUplink(rawUplinkWithDevEui(t, upper, map[string]any{"magnet_status": "close"}), "application/x")

	// (1) 로스터 맵 키.
	a.devicesMu.RLock()
	_, lowerKeyed := a.devices[fixtureDevEui]
	_, upperKeyed := a.devices[upper]
	rosterLen := len(a.devices)
	a.devicesMu.RUnlock()
	if !lowerKeyed {
		t.Errorf("로스터가 소문자 키(%q)로 저장되지 않았다", fixtureDevEui)
	}
	if upperKeyed {
		t.Errorf("로스터에 대문자 키(%q)가 존재한다", upper)
	}

	// (2) 방출 레코드 unit_id — 노드는 이 값으로 ResolveDeviceID 를 재조회하므로
	// 여기서 대문자가 새면 하류에서 별도 device_id 가 생긴다.
	events, _ := drainRecords(t, a)
	if len(events) != 1 {
		t.Fatalf("방출 레코드 = %d건, want 1건", len(events))
	}
	if events[0].UnitID != fixtureDevEui {
		t.Errorf("레코드 unit_id = %q, want 소문자 %q", events[0].UnitID, fixtureDevEui)
	}

	// (3) dev_eui 라벨 (metadata.labels — 런타임 상태가 아니라 디바이스 식별 정보).
	if got := a.DeviceProvider().Devices()[0].Metadata().Labels[labelKeyDevEui]; got != fixtureDevEui {
		t.Errorf("Metadata().Labels[dev_eui] = %v, want 소문자 %q", got, fixtureDevEui)
	}

	// 대소문자가 섞여 들어와도 디바이스는 하나로 접힌다.
	a.handleUplink(rawUplinkWithDevEui(t, fixtureDevEui, map[string]any{"magnet_status": "open"}), "application/x")
	a.devicesMu.RLock()
	after := len(a.devices)
	a.devicesMu.RUnlock()
	if rosterLen != 1 || after != 1 {
		t.Errorf("로스터 크기 = %d → %d, want 1 → 1 (대소문자 변형이 한 디바이스로 접혀야 한다)", rosterLen, after)
	}
}

// TestDeviceID_ConcurrentUplinksAreRaceFree 는 동시 업링크에서 UID 발급/로스터 갱신이
// 데이터 레이스 없이 동작하고, 디바이스마다 서로 다른 UUID 로 수렴하는지 검증한다.
// (-race 로 실행할 때 의미가 있다.)
func TestDeviceID_ConcurrentUplinksAreRaceFree(t *testing.T) {
	withRealDeviceIDRepo(t)
	a := newRunningTestAgent(t, "devid-race")

	// 서로 다른 devEui 여러 개 + 동일 devEui 중복 — 두 경로를 함께 밟는다.
	euis := []string{fixtureDevEui, "aabbccddeeff0011", "00112233445566AA"}

	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			eui := euis[i%len(euis)]
			a.handleUplink(rawUplinkWithDevEui(t, eui, map[string]any{"magnet_status": "close"}), "application/x")
		}(i)
	}
	wg.Wait()

	devs := a.DeviceProvider().Devices()
	if len(devs) != len(euis) {
		t.Fatalf("Devices() len = %d, want %d (대소문자 변형이 접혀야 한다)", len(devs), len(euis))
	}

	// 각 디바이스는 고유한 UUID 를 갖는다 (동시 발급이 값을 섞지 않는다).
	seenUUID := make(map[string]bool, len(devs))
	seenEui := make(map[string]bool, len(devs))
	for _, d := range devs {
		if !uuidV4Re.MatchString(d.UID()) {
			t.Errorf("UID() = %q, want UUID v4", d.UID())
		}
		if seenUUID[d.UID()] {
			t.Errorf("UUID %q 가 두 디바이스에 중복 배정됐다", d.UID())
		}
		seenUUID[d.UID()] = true
		seenEui[d.Metadata().Labels[labelKeyDevEui]] = true
	}
	for _, eui := range euis {
		if want := strings.ToLower(eui); !seenEui[want] {
			t.Errorf("dev_eui %q 가 없다 (관측: %v)", want, seenEui)
		}
	}
}
