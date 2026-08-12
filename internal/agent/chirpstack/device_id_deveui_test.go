// device_id_deveui_test.go 는 "ChirpStack 디바이스의 device_id 는 랜덤 UUID 가 아니라
// LoRaWAN devEui 이다" 계약을 검증한다.
//
// 변경 전(BEFORE) 동작: upsertDevice 가 ResolveDeviceID 만 호출했으므로 저장소의
// GetOrCreate 가 랜덤 UUID v4 를 발급했고, metadata.device.id 에는 그 UUID 가 실렸다.
// 변경 후(AFTER): upsertDevice 가 먼저 SetDeviceID(agent, devEui, devEui) 로 저장소를
// 시딩하므로 동일한 ResolveDeviceID 호출이 devEui 를 반환한다. 하류(promoteDevIDWithUUID
// 등 31개 호출부)는 한 줄도 바뀌지 않는다.
//
// BEFORE/AFTER 대비는 TestDeviceID_PreExistingUuidIsConverted 가 저장소에 실제
// UUID 를 먼저 심어 두고(=변경 전 상태) 업링크 1건으로 전환되는지 관측한다.
package chirpstack

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/storage"
)

// withRealDeviceIDRepo 는 프로덕션과 동일한 in-memory 저장소 구현을 설치한다.
//
// 테스트 전용 스텁이 아니라 실제 구현(storage.DeviceIDMemoryRepository)을 쓰는 이유:
// 이 파일의 핵심 주장 두 가지 — (1) 기존 매핑을 덮어쓴다, (2) 다른 키에 이미 배정된
// device_id 는 ErrDeviceIDConflict 로 거부된다 — 는 모두 저장소의 실제 Set 의미론에
// 달려 있다. 스텁으로 대체하면 검증 대상이 사라진다.
func withRealDeviceIDRepo(t *testing.T) *storage.DeviceIDMemoryRepository {
	t.Helper()
	repo := storage.NewDeviceIDMemoryRepository()
	agent.SetDeviceIDRepository(repo)
	t.Cleanup(func() { agent.SetDeviceIDRepository(nil) })
	return repo
}

// countingDeviceIDRepo 는 Set 호출 횟수를 세고 임의 에러를 주입할 수 있는 데코레이터이다.
type countingDeviceIDRepo struct {
	inner agent.DeviceIDRepository

	mu       sync.Mutex
	setCalls int
	// setErr 가 non-nil 이면 inner 로 위임하지 않고 그 에러를 반환한다.
	setErr error
}

func newCountingRepo(inner agent.DeviceIDRepository) *countingDeviceIDRepo {
	return &countingDeviceIDRepo{inner: inner}
}

func (r *countingDeviceIDRepo) GetOrCreate(ctx context.Context, a, u string) (string, error) {
	return r.inner.GetOrCreate(ctx, a, u)
}

func (r *countingDeviceIDRepo) Get(ctx context.Context, a, u string) (string, error) {
	return r.inner.Get(ctx, a, u)
}

func (r *countingDeviceIDRepo) Set(ctx context.Context, a, u, id string) error {
	r.mu.Lock()
	r.setCalls++
	err := r.setErr
	r.mu.Unlock()
	if err != nil {
		return err
	}
	return r.inner.Set(ctx, a, u, id)
}

func (r *countingDeviceIDRepo) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.setCalls
}

func (r *countingDeviceIDRepo) failWith(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setErr = err
}

// newLoggingTestAgent 는 로그를 buf 로 캡처하는 비활성화 에이전트를 만든다.
// 경고 로그가 실제로 방출되는지 관측하기 위한 것이며, 그 외에는
// newRunningTestAgent 와 동일하다.
func newLoggingTestAgent(t *testing.T, name string, buf *strings.Builder) *ChirpStackAgent {
	t.Helper()
	cfg := newTestConfig("id-"+name, name)
	cfg.Logger = slog.New(slog.NewTextHandler(&syncWriter{b: buf}, &slog.HandlerOptions{Level: slog.LevelWarn}))
	raw, err := NewChirpStackAgent(cfg)
	if err != nil {
		t.Fatalf("NewChirpStackAgent: %v", err)
	}
	a, ok := raw.(*ChirpStackAgent)
	if !ok {
		t.Fatalf("unexpected agent type %T", raw)
	}
	return a
}

// syncWriter 는 strings.Builder 를 동시 쓰기로부터 보호한다 (-race 대비).
type syncWriter struct {
	mu sync.Mutex
	b  *strings.Builder
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
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

// TestDeviceID_IsDevEuiNotUuid 는 업링크 1건 후 디바이스의 ID()/UID() 가 devEui
// 자신이며 UUID 가 아님을 검증한다 (본 변경의 핵심 계약).
func TestDeviceID_IsDevEuiNotUuid(t *testing.T) {
	repo := withRealDeviceIDRepo(t)
	a := newRunningTestAgent(t, "devid-basic")

	a.handleUplink(loadRawUplink(t), "application/x")

	devs := a.DeviceProvider().Devices()
	if len(devs) != 1 {
		t.Fatalf("Devices() len = %d, want 1", len(devs))
	}
	d := devs[0]

	if d.UID() != fixtureDevEui {
		t.Errorf("UID() = %q, want devEui %q", d.UID(), fixtureDevEui)
	}
	if d.ID() != fixtureDevEui {
		t.Errorf("ID() = %q, want devEui %q", d.ID(), fixtureDevEui)
	}
	if _, err := uuid.Parse(d.UID()); err == nil {
		t.Errorf("UID() = %q 가 UUID 로 파싱된다 — 랜덤 UUID 가 아니라 devEui 여야 한다", d.UID())
	}

	// 저장소에도 devEui 가 값으로 들어가 있어야 한다 (재시작 후에도 동일하게 해석되도록).
	stored, err := repo.Get(context.Background(), a.Name(), fixtureDevEui)
	if err != nil {
		t.Fatalf("repo.Get: %v", err)
	}
	if stored != fixtureDevEui {
		t.Errorf("저장소 값 = %q, want %q", stored, fixtureDevEui)
	}

	// Provider.Device(devEui) 조회도 성립해야 한다 (조회 키가 UID 이므로).
	got, err := a.DeviceProvider().Device(fixtureDevEui)
	if err != nil {
		t.Fatalf("Device(devEui) 조회 실패: %v", err)
	}
	if got.UID() != fixtureDevEui {
		t.Errorf("Device(devEui).UID() = %q", got.UID())
	}
}

// TestDeviceID_PreExistingUuidIsConverted 는 이미 UUID 를 보유한(=변경 전) 디바이스가
// 다음 업링크에서 devEui 로 전환되는지 검증한다 (FULL 전환 결정).
//
// 이 테스트가 곧 BEFORE/AFTER 대비이다: 사전 시딩된 UUID 가 BEFORE 상태이고, 업링크
// 1건 이후의 값이 AFTER 상태이다. 또한 저장소 Set 이 기존 매핑을 실제로 덮어쓰는지를
// 실제 구현으로 확인한다(스텁 아님).
func TestDeviceID_PreExistingUuidIsConverted(t *testing.T) {
	repo := withRealDeviceIDRepo(t)
	a := newRunningTestAgent(t, "devid-convert")
	ctx := context.Background()

	// BEFORE: 변경 전 에이전트가 남겨 놓았을 랜덤 UUID 를 그대로 심는다.
	legacy := uuid.New().String()
	if err := agent.SetDeviceID(ctx, a.Name(), fixtureDevEui, legacy); err != nil {
		t.Fatalf("legacy UUID 시딩 실패: %v", err)
	}
	if got := agent.ResolveDeviceID(ctx, a.Name(), fixtureDevEui); got != legacy {
		t.Fatalf("BEFORE 상태 준비 실패: ResolveDeviceID = %q, want %q", got, legacy)
	}

	// AFTER: 업링크 1건.
	a.handleUplink(loadRawUplink(t), "application/x")

	if got := agent.ResolveDeviceID(ctx, a.Name(), fixtureDevEui); got != fixtureDevEui {
		t.Errorf("전환 실패: ResolveDeviceID = %q, want devEui %q", got, fixtureDevEui)
	}
	stored, _ := repo.Get(ctx, a.Name(), fixtureDevEui)
	if stored == legacy {
		t.Errorf("저장소가 기존 UUID(%q)를 그대로 유지했다 — 덮어쓰기가 일어나지 않았다", legacy)
	}
	if stored != fixtureDevEui {
		t.Errorf("저장소 값 = %q, want %q", stored, fixtureDevEui)
	}
	if a.DeviceProvider().Devices()[0].UID() != fixtureDevEui {
		t.Errorf("UID() = %q, want %q", a.DeviceProvider().Devices()[0].UID(), fixtureDevEui)
	}
}

// TestDeviceID_RegisteredOnceAcrossManyUplinks 는 등록이 디바이스당 1회만 일어나고
// 업링크마다 반복되지 않음을 검증한다 (hot path 저장소 I/O 방지).
func TestDeviceID_RegisteredOnceAcrossManyUplinks(t *testing.T) {
	counting := newCountingRepo(storage.NewDeviceIDMemoryRepository())
	agent.SetDeviceIDRepository(counting)
	t.Cleanup(func() { agent.SetDeviceIDRepository(nil) })

	a := newRunningTestAgent(t, "devid-once")

	const n = 5
	raw := loadRawUplink(t)
	for i := 0; i < n; i++ {
		a.handleUplink(raw, "application/x")
	}

	if got := counting.calls(); got != 1 {
		t.Errorf("Set 호출 = %d회 (업링크 %d건), want 1회", got, n)
	}
	if a.DeviceProvider().Devices()[0].UID() != fixtureDevEui {
		t.Errorf("UID() = %q, want %q", a.DeviceProvider().Devices()[0].UID(), fixtureDevEui)
	}
}

// TestDeviceID_TransientFailureIsRetried 는 충돌이 아닌 일시적 등록 실패가 다음
// 업링크에서 재시도되는지 검증한다 (영구 종결과 구분).
func TestDeviceID_TransientFailureIsRetried(t *testing.T) {
	counting := newCountingRepo(storage.NewDeviceIDMemoryRepository())
	counting.failWith(fmt.Errorf("persist device id (Set): 디스크 오류"))
	agent.SetDeviceIDRepository(counting)
	t.Cleanup(func() { agent.SetDeviceIDRepository(nil) })

	var logBuf strings.Builder
	a := newLoggingTestAgent(t, "devid-transient", &logBuf)
	raw := loadRawUplink(t)

	a.handleUplink(raw, "application/x")
	a.handleUplink(raw, "application/x")
	if got := counting.calls(); got != 2 {
		t.Errorf("Set 호출 = %d회, want 2회 (일시 실패는 재시도되어야 한다)", got)
	}

	// 복구되면 다음 업링크에서 성공하고, 이후로는 더 시도하지 않는다.
	counting.failWith(nil)
	a.handleUplink(raw, "application/x")
	a.handleUplink(raw, "application/x")
	if got := counting.calls(); got != 3 {
		t.Errorf("Set 호출 = %d회, want 3회 (복구 후 1회 성공하면 종결)", got)
	}
	if a.DeviceProvider().Devices()[0].UID() != fixtureDevEui {
		t.Errorf("UID() = %q, want %q", a.DeviceProvider().Devices()[0].UID(), fixtureDevEui)
	}
	if !strings.Contains(logBuf.String(), "재시도") {
		t.Errorf("일시 실패 경고 로그가 없다:\n%s", logBuf.String())
	}
}

// TestDeviceID_ConflictDoesNotBreakUplink 는 devEui 가 다른 (agent, unit) 에 이미
// 배정돼 충돌할 때 (a) 경고를 남기고 (b) 업링크를 실패시키지 않으며 (c) 측정 스트림이
// 계속 흐르는지 검증한다.
//
// 재현 방법: 동일 devEui 를 두 에이전트가 각자 등록한다. 저장소 키는
// (agentName, unitID) 이므로 두 번째 등록은 "이 device_id 는 이미 다른 키에 배정됨"
// 으로 거부된다 — LoRaWAN devEui 는 전역 고유하지만 ChirpStack 서버 2대를 각각
// 다른 에이전트로 붙이면 실제로 발생할 수 있는 상황이다.
func TestDeviceID_ConflictDoesNotBreakUplink(t *testing.T) {
	withRealDeviceIDRepo(t)
	ctx := context.Background()

	first := newRunningTestAgent(t, "devid-conflict-a")
	first.handleUplink(loadRawUplink(t), "application/x")
	if got := agent.ResolveDeviceID(ctx, first.Name(), fixtureDevEui); got != fixtureDevEui {
		t.Fatalf("선행 에이전트 등록 실패: %q", got)
	}

	var logBuf strings.Builder
	second := newLoggingTestAgent(t, "devid-conflict-b", &logBuf)

	// 두 번째 에이전트의 기존 device_id(자동 발급 UUID)를 미리 만들어 둔다 —
	// 충돌 시 "기존 값을 그대로 유지"하는지 관측하기 위함이다.
	existing := agent.ResolveDeviceID(ctx, second.Name(), fixtureDevEui)
	if existing == "" || existing == fixtureDevEui {
		t.Fatalf("사전 조건 실패: 두 번째 에이전트의 기존 device_id = %q", existing)
	}

	second.handleUplink(loadRawUplink(t), "application/x")

	// (a) 경고 로그: devEui 와 양측 당사자가 드러나야 한다.
	logs := logBuf.String()
	if !strings.Contains(logs, fixtureDevEui) {
		t.Errorf("경고 로그에 devEui 가 없다:\n%s", logs)
	}
	if !strings.Contains(logs, "devid-conflict-b") {
		t.Errorf("경고 로그에 등록 시도 에이전트가 없다:\n%s", logs)
	}
	if !strings.Contains(logs, "devid-conflict-a") {
		t.Errorf("경고 로그에 충돌 상대(기존 보유 키)가 없다:\n%s", logs)
	}

	// (b) 업링크는 실패하지 않는다 — 로스터에 디바이스가 생성되어 있다.
	devs := second.DeviceProvider().Devices()
	if len(devs) != 1 {
		t.Fatalf("충돌 후 Devices() len = %d, want 1", len(devs))
	}
	// ResolveDeviceID 는 기존에 갖고 있던 값을 그대로 반환한다.
	if got := devs[0].UID(); got != existing {
		t.Errorf("충돌 후 UID() = %q, want 기존 값 %q", got, existing)
	}

	// (c) 측정 스트림은 계속 흐른다.
	events, _ := drainRecords(t, second)
	if len(events) != 1 {
		t.Fatalf("충돌 후 방출 레코드 = %d건, want 1건", len(events))
	}
	if events[0].UnitID != fixtureDevEui {
		t.Errorf("레코드 unit_id = %q, want %q", events[0].UnitID, fixtureDevEui)
	}
	if events[0].Measurement != "magnet_status" {
		t.Errorf("레코드 measurement = %q", events[0].Measurement)
	}

	// 선행 에이전트의 매핑은 훼손되지 않는다.
	if got := agent.ResolveDeviceID(ctx, first.Name(), fixtureDevEui); got != fixtureDevEui {
		t.Errorf("선행 에이전트 매핑 훼손: %q", got)
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

// TestDeviceID_DevEuiCaseNormalized 는 대문자 devEui 가 소문자로 정규화되어
// (1) 로스터 맵 키, (2) 등록 device_id, (3) 방출 레코드 unit_id 세 곳에 일관되게
// 적용되는지 검증한다.
//
// 세 곳 중 하나라도 어긋나면 동일 물리 디바이스가 두 개의 device_id 로 쪼개진다.
func TestDeviceID_DevEuiCaseNormalized(t *testing.T) {
	withRealDeviceIDRepo(t)
	a := newRunningTestAgent(t, "devid-case")

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

	// (2) 등록 device_id.
	if got := a.DeviceProvider().Devices()[0].UID(); got != fixtureDevEui {
		t.Errorf("UID() = %q, want 소문자 devEui %q", got, fixtureDevEui)
	}

	// (3) 방출 레코드 unit_id — 노드는 이 값으로 ResolveDeviceID 를 재조회하므로
	// 여기서 대문자가 새면 하류에서 별도 device_id 가 생긴다.
	events, _ := drainRecords(t, a)
	if len(events) != 1 {
		t.Fatalf("방출 레코드 = %d건, want 1건", len(events))
	}
	if events[0].UnitID != fixtureDevEui {
		t.Errorf("레코드 unit_id = %q, want 소문자 %q", events[0].UnitID, fixtureDevEui)
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

// TestDeviceID_ConcurrentUplinksAreRaceFree 는 동시 업링크에서 등록/로스터 갱신이
// 데이터 레이스 없이 동작하고 최종 device_id 가 devEui 로 수렴하는지 검증한다.
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
		t.Fatalf("Devices() len = %d, want %d", len(devs), len(euis))
	}
	seen := make(map[string]bool, len(devs))
	for _, d := range devs {
		seen[d.UID()] = true
	}
	for _, eui := range euis {
		want := strings.ToLower(eui)
		if !seen[want] {
			t.Errorf("device_id %q 가 없다 (관측: %v)", want, seen)
		}
	}
}
