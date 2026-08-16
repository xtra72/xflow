package device

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- 테스트용 디바이스/레지스트리 ---

// histTestDevice 는 history 테스트용 최소 Device 구현이다.
type histTestDevice struct {
	id         string
	online     bool
	lastSeen   time.Time
	properties map[string]any
}

func (d *histTestDevice) ID() string               { return d.id }
func (d *histTestDevice) UID() string              { return d.id }
func (d *histTestDevice) Name() string             { return d.id }
func (d *histTestDevice) Type() DeviceType         { return DeviceTypeSensor }
func (d *histTestDevice) Protocol() string         { return "test" }
func (d *histTestDevice) AgentName() string        { return "agent" }
func (d *histTestDevice) Online() bool             { return d.online }
func (d *histTestDevice) LastSeen() time.Time      { return d.lastSeen }
func (d *histTestDevice) Metadata() DeviceMetadata { return DeviceMetadata{} }
func (d *histTestDevice) Source() string           { return "auto" }
func (d *histTestDevice) Capabilities() []string   { return nil }
func (d *histTestDevice) State() DeviceState {
	return DeviceState{
		Online:     d.online,
		LastSeen:   d.lastSeen,
		Properties: d.properties,
	}
}

// histStubRegistry 는 List 결과를 제어 가능한 stub 레지스트리이다.
type histStubRegistry struct {
	mu      sync.Mutex
	devices []Device
}

func (s *histStubRegistry) set(devs ...Device) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices = devs
}

func (s *histStubRegistry) List(_ DeviceFilter) []Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Device, len(s.devices))
	copy(out, s.devices)
	return out
}

// --- 기본값 보정 ---

func TestNewDeviceHistoryRecorder_Defaults(t *testing.T) {
	tests := []struct {
		name         string
		in           DeviceHistoryConfig
		wantMax      int
		wantGCAfter  time.Duration
		wantInterval time.Duration
	}{
		{
			name:         "전부 무효 → 기본값",
			in:           DeviceHistoryConfig{},
			wantMax:      defaultHistoryMaxEntries,
			wantInterval: defaultHistoryInterval,
			wantGCAfter:  10 * defaultHistoryInterval,
		},
		{
			name:         "명시 값 보존 + GCAfter 파생",
			in:           DeviceHistoryConfig{Interval: 2 * time.Second, MaxEntries: 5},
			wantMax:      5,
			wantInterval: 2 * time.Second,
			wantGCAfter:  20 * time.Second,
		},
		{
			name:         "GCAfter 명시 보존",
			in:           DeviceHistoryConfig{Interval: time.Second, MaxEntries: 3, GCAfter: 99 * time.Second},
			wantMax:      3,
			wantInterval: time.Second,
			wantGCAfter:  99 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewDeviceHistoryRecorder(&histStubRegistry{}, tt.in)
			assert.Equal(t, tt.wantMax, r.cfg.MaxEntries)
			assert.Equal(t, tt.wantInterval, r.cfg.Interval)
			assert.Equal(t, tt.wantGCAfter, r.cfg.GCAfter)
			assert.Equal(t, tt.wantMax, r.MaxEntries())
		})
	}
}

// --- 스냅샷 수집 ---

func TestDeviceHistoryRecorder_SnapshotOnce(t *testing.T) {
	reg := &histStubRegistry{}
	reg.set(&histTestDevice{
		id:         "dev-1",
		online:     true,
		lastSeen:   time.UnixMilli(1000),
		properties: map[string]any{"temp": 22},
	})

	r := NewDeviceHistoryRecorder(reg, DeviceHistoryConfig{Interval: time.Second, MaxEntries: 10})
	r.snapshotOnce()

	hist := r.History("dev-1", 0)
	require.Len(t, hist, 1)
	assert.True(t, hist[0].Online)
	assert.Equal(t, int64(1000), hist[0].LastSeen)
	assert.Equal(t, 22, hist[0].Properties["temp"])
	assert.Equal(t, 1, r.TrackedCount())
}

// TestDeviceHistoryRecorder_PropertiesCopied 는 스냅샷이 원본 맵 변경에 영향받지
// 않는 복사본인지 검증한다.
func TestDeviceHistoryRecorder_PropertiesCopied(t *testing.T) {
	props := map[string]any{"k": 1}
	dev := &histTestDevice{id: "d", online: true, properties: props}
	reg := &histStubRegistry{}
	reg.set(dev)

	r := NewDeviceHistoryRecorder(reg, DeviceHistoryConfig{MaxEntries: 10})
	r.snapshotOnce()

	// 원본 맵 변형 후에도 기록된 스냅샷은 보존되어야 한다.
	props["k"] = 999

	hist := r.History("d", 0)
	require.Len(t, hist, 1)
	assert.Equal(t, 1, hist[0].Properties["k"])
}

// --- 링버퍼 상한 ---

func TestDeviceHistoryRecorder_RingBufferCap(t *testing.T) {
	dev := &histTestDevice{id: "d", online: true, properties: map[string]any{"n": 0}}
	reg := &histStubRegistry{}
	reg.set(dev)

	max := 3
	r := NewDeviceHistoryRecorder(reg, DeviceHistoryConfig{MaxEntries: max})

	// 5번 변화 → 최신 3개만 유지.
	for i := 1; i <= 5; i++ {
		dev.properties = map[string]any{"n": i}
		r.snapshotOnce()
	}

	hist := r.History("d", 0)
	require.Len(t, hist, max, "링버퍼는 MaxEntries 를 초과 보관하지 않아야 한다")
	// 최신순: n=5,4,3
	assert.Equal(t, 5, hist[0].Properties["n"])
	assert.Equal(t, 4, hist[1].Properties["n"])
	assert.Equal(t, 3, hist[2].Properties["n"])
}

// --- 중복 억제 ---

func TestDeviceHistoryRecorder_Dedup(t *testing.T) {
	dev := &histTestDevice{id: "d", online: true, properties: map[string]any{"v": "a"}}
	reg := &histStubRegistry{}
	reg.set(dev)

	r := NewDeviceHistoryRecorder(reg, DeviceHistoryConfig{MaxEntries: 10})

	r.snapshotOnce() // 기록 (첫 항목)
	r.snapshotOnce() // 동일 → skip
	r.snapshotOnce() // 동일 → skip

	require.Len(t, r.History("d", 0), 1, "Properties/Online 무변화 시 중복 억제")

	// Properties 변화 → 기록.
	dev.properties = map[string]any{"v": "b"}
	r.snapshotOnce()
	require.Len(t, r.History("d", 0), 2)

	// Online 변화만으로도 기록.
	dev.online = false
	r.snapshotOnce()
	require.Len(t, r.History("d", 0), 3)
}

// --- 중첩 컨테이너 properties (결함 재현 방지) ---

// chirpShapedProps 는 ChirpStack 프로바이더가 실제로 내는 properties 모양을 만든다:
// gateways(구조체 슬라이스) + measurements(중첩 맵) 두 키뿐이며 스칼라가 하나도 없다.
//
// 게이트웨이 항목을 map 이 아니라 **구조체**로 만드는 것이 핵심이다 — 실제 프로바이더가
// []deviceGatewayView 를 싣기 때문에, map/slice 만 재귀하는 비교로는 잡히지 않는
// 경로를 그대로 재현한다.
type histGatewayView struct {
	GatewayID  string
	RSSI       int
	LastSeenMs int64
	Stale      bool
}

func chirpShapedProps(temp float64, timeMs int64, stale bool) map[string]any {
	return map[string]any{
		"gateways": []histGatewayView{
			{GatewayID: "gw-a", RSSI: -97, LastSeenMs: timeMs, Stale: stale},
			{GatewayID: "gw-b", RSSI: -105, LastSeenMs: timeMs, Stale: stale},
		},
		"measurements": map[string]any{
			"temperature": map[string]any{"value": temp, "time_ms": timeMs},
			"humidity":    map[string]any{"value": 41.0, "time_ms": timeMs},
		},
	}
}

// TestDeviceHistoryRecorder_DedupNestedProperties 는 보고된 결함의 회귀 가드이다.
//
// 결함: 중첩 컨테이너를 값으로 갖는 properties 는 비교가 항상 "다름"으로 떨어져,
// 값이 한 글자도 바뀌지 않아도 스냅샷 주기마다(기본 10초) 엔트리가 영원히 쌓였다.
// 사용자에게는 같은 값이 반복되는 합성 행으로 보인다.
//
// 이 테스트는 수정 전 코드에서 반드시 실패한다(1개 기대 → 실제 3개).
func TestDeviceHistoryRecorder_DedupNestedProperties(t *testing.T) {
	dev := &histTestDevice{
		id:         "chirp-1",
		online:     true,
		properties: chirpShapedProps(21.5, 1_000, false),
	}
	reg := &histStubRegistry{}
	reg.set(dev)

	r := NewDeviceHistoryRecorder(reg, DeviceHistoryConfig{MaxEntries: 10})

	r.snapshotOnce() // 최초 기록
	r.snapshotOnce() // 무변화 → skip 되어야 한다
	r.snapshotOnce() // 무변화 → skip 되어야 한다

	require.Len(t, r.History("chirp-1", 0), 1,
		"중첩 properties 가 동일하면 엔트리를 추가하지 않아야 한다(주기 스팸 방지)")

	// 실제 측정값 변화 → 정확히 1개 추가.
	dev.properties = chirpShapedProps(22.0, 2_000, false)
	r.snapshotOnce()
	r.snapshotOnce() // 다시 무변화 → skip

	hist := r.History("chirp-1", 0)
	require.Len(t, hist, 2, "실측값이 바뀌면 정확히 1개만 추가되어야 한다")

	measurements, ok := hist[0].Properties["measurements"].(map[string]any)
	require.True(t, ok)
	temperature, ok := measurements["temperature"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 22.0, temperature["value"], "최신 엔트리는 변경된 값을 담아야 한다")
}

// histVolatileDevice 는 HistoryComparable 을 구현해 조회 시각 파생 필드(stale)를
// 비교 표면에서 중립화하는 디바이스이다(chirpDeviceAdapter 와 동일한 계약).
type histVolatileDevice struct {
	*histTestDevice
}

func (d *histVolatileDevice) HistoryComparisonProperties() (map[string]any, bool) {
	props := copyProperties(d.properties)
	if gws, ok := props["gateways"].([]histGatewayView); ok {
		// 원본을 건드리지 않도록 슬라이스를 새로 만든 뒤 중립화한다.
		neutral := make([]histGatewayView, len(gws))
		copy(neutral, gws)
		for i := range neutral {
			neutral[i].Stale = false
		}
		props["gateways"] = neutral
	}
	return props, true
}

// TestDeviceHistoryRecorder_VolatileFieldFlipIsNotAChange 는 조회 시각 파생 필드가
// 스스로 뒤집혀도 엔트리가 생기지 않음을 검증한다.
//
// gateways[].stale 은 time.Now() 와 offline 임계에서 파생되므로 업링크 없이도 임계
// 경과 순간 뒤집힌다. 그 뒤집힘은 "디바이스가 한 일"이 아니므로 이력에 남으면 안 된다.
func TestDeviceHistoryRecorder_VolatileFieldFlipIsNotAChange(t *testing.T) {
	base := &histTestDevice{
		id:         "chirp-2",
		online:     true,
		properties: chirpShapedProps(21.5, 1_000, false),
	}
	dev := &histVolatileDevice{histTestDevice: base}
	reg := &histStubRegistry{}
	reg.set(dev)

	r := NewDeviceHistoryRecorder(reg, DeviceHistoryConfig{MaxEntries: 10})
	r.snapshotOnce()
	require.Len(t, r.History("chirp-2", 0), 1)

	// stale 만 뒤집힘(업링크 없음) → 기록하지 않아야 한다.
	base.properties = chirpShapedProps(21.5, 1_000, true)
	r.snapshotOnce()
	r.snapshotOnce()

	require.Len(t, r.History("chirp-2", 0), 1,
		"조회 시각 파생 필드(stale) 단독 변화는 기록하지 않는다")

	// stale 은 비교에서만 제외될 뿐 payload 에는 남아 있어야 한다(UI 표시용).
	gws, ok := r.History("chirp-2", 0)[0].Properties["gateways"].([]histGatewayView)
	require.True(t, ok, "gateways 키는 payload 에 그대로 있어야 한다")
	require.Len(t, gws, 2)
	assert.False(t, gws[0].Stale, "최초 기록 시점의 stale 값이 payload 에 보존된다")

	// 실제 측정값 변화는 여전히 기록된다.
	base.properties = chirpShapedProps(23.0, 5_000, true)
	r.snapshotOnce()
	require.Len(t, r.History("chirp-2", 0), 2)
}

// histEventTimedDevice 는 HistoryEventTimed 를 구현해 실제 수신 시각을 제공한다.
type histEventTimedDevice struct {
	*histTestDevice
	eventMs int64
	hasTime bool
}

func (d *histEventTimedDevice) HistoryEventTimeMs() (int64, bool) {
	return d.eventMs, d.hasTime
}

// TestDeviceHistoryRecorder_EntryTimestamp 는 엔트리 시각 규칙을 검증한다:
// 프로바이더가 실제 수신 시각을 알면 그것을, 모르면 샘플 시각을 쓴다.
func TestDeviceHistoryRecorder_EntryTimestamp(t *testing.T) {
	t.Run("이벤트 시각 제공 → 그 값을 쓴다", func(t *testing.T) {
		base := &histTestDevice{id: "d", online: true, properties: map[string]any{"v": 1}}
		dev := &histEventTimedDevice{histTestDevice: base, eventMs: 1_700_000_000_123, hasTime: true}
		reg := &histStubRegistry{}
		reg.set(dev)

		r := NewDeviceHistoryRecorder(reg, DeviceHistoryConfig{MaxEntries: 10})
		r.now = func() time.Time { return time.UnixMilli(99_999) } // 샘플 시각(격자)
		r.snapshotOnce()

		hist := r.History("d", 0)
		require.Len(t, hist, 1)
		assert.Equal(t, int64(1_700_000_000_123), hist[0].Timestamp,
			"엔트리 시각은 샘플 시각이 아니라 실제 수신 시각이어야 한다")
	})

	t.Run("이벤트 시각 없음 → 샘플 시각 폴백", func(t *testing.T) {
		base := &histTestDevice{id: "d", online: true, properties: map[string]any{"v": 1}}
		dev := &histEventTimedDevice{histTestDevice: base, hasTime: false}
		reg := &histStubRegistry{}
		reg.set(dev)

		r := NewDeviceHistoryRecorder(reg, DeviceHistoryConfig{MaxEntries: 10})
		r.now = func() time.Time { return time.UnixMilli(99_999) }
		r.snapshotOnce()

		hist := r.History("d", 0)
		require.Len(t, hist, 1)
		assert.Equal(t, int64(99_999), hist[0].Timestamp, "미제공 시 샘플 시각으로 폴백")
	})

	t.Run("인터페이스 미구현 → 샘플 시각", func(t *testing.T) {
		reg := &histStubRegistry{}
		reg.set(&histTestDevice{id: "plain", online: true, properties: map[string]any{"v": 1}})

		r := NewDeviceHistoryRecorder(reg, DeviceHistoryConfig{MaxEntries: 10})
		r.now = func() time.Time { return time.UnixMilli(42) }
		r.snapshotOnce()

		hist := r.History("plain", 0)
		require.Len(t, hist, 1)
		assert.Equal(t, int64(42), hist[0].Timestamp)
	})
}

// TestOfflineDeviceWrapper_ForwardsHistoryHints 는 오프라인 래퍼가 선택적 이력
// 인터페이스를 위임하는지 검증한다.
//
// 래퍼는 인터페이스를 임베드하므로 승격 메서드가 Device 로 한정된다 — 위임이 없으면
// 에이전트가 오프라인이 되는 순간 힌트가 조용히 사라져 이력 동작이 달라진다.
func TestOfflineDeviceWrapper_ForwardsHistoryHints(t *testing.T) {
	base := &histTestDevice{id: "d", online: true, properties: chirpShapedProps(21.5, 1_000, false)}
	inner := &histVolatileDevice{histTestDevice: base}
	w := &offlineDeviceWrapper{Device: inner}

	props, ok := w.HistoryComparisonProperties()
	require.True(t, ok, "내부 Device 의 비교 표면이 위임되어야 한다")
	gws, isSlice := props["gateways"].([]histGatewayView)
	require.True(t, isSlice)
	assert.False(t, gws[0].Stale, "위임된 비교 표면에서 파생 필드가 중립화되어 있어야 한다")

	// 미구현 Device 는 "의견 없음"으로 폴백한다.
	plain := &offlineDeviceWrapper{Device: &histTestDevice{id: "p"}}
	_, hasCmp := plain.HistoryComparisonProperties()
	assert.False(t, hasCmp)
	_, hasTime := plain.HistoryEventTimeMs()
	assert.False(t, hasTime)

	// 이벤트 시각도 동일하게 위임된다.
	timed := &offlineDeviceWrapper{Device: &histEventTimedDevice{histTestDevice: base, eventMs: 777, hasTime: true}}
	ms, okTime := timed.HistoryEventTimeMs()
	require.True(t, okTime)
	assert.Equal(t, int64(777), ms)
}

// TestDeviceHistoryRecorder_DedupIgnoresLastSeenOnly 는 LastSeen 만 변경된 경우
// (Properties/Online 동일) 새 엔트리를 추가하지 않음을 검증한다.
func TestDeviceHistoryRecorder_DedupIgnoresLastSeenOnly(t *testing.T) {
	dev := &histTestDevice{id: "d", online: true, lastSeen: time.UnixMilli(1), properties: map[string]any{"v": 1}}
	reg := &histStubRegistry{}
	reg.set(dev)

	r := NewDeviceHistoryRecorder(reg, DeviceHistoryConfig{MaxEntries: 10})
	r.snapshotOnce()

	dev.lastSeen = time.UnixMilli(99999) // LastSeen 만 변경
	r.snapshotOnce()

	require.Len(t, r.History("d", 0), 1, "LastSeen 단독 변화는 기록하지 않는다(기본 정책)")
}

// --- GC ---

func TestDeviceHistoryRecorder_GC(t *testing.T) {
	dev := &histTestDevice{id: "gone", online: true, properties: map[string]any{"v": 1}}
	reg := &histStubRegistry{}
	reg.set(dev)

	// 결정적 시각 주입.
	current := time.UnixMilli(0)
	r := NewDeviceHistoryRecorder(reg, DeviceHistoryConfig{Interval: time.Second, MaxEntries: 10, GCAfter: 5 * time.Second})
	r.now = func() time.Time { return current }

	r.snapshotOnce()
	require.Equal(t, 1, r.TrackedCount())

	// 디바이스 제거.
	reg.set()

	// GCAfter 미경과 → 보존.
	current = time.UnixMilli(3000)
	r.snapshotOnce()
	assert.Equal(t, 1, r.TrackedCount(), "GCAfter 미경과 시 버퍼 보존")

	// GCAfter 경과 → 삭제.
	current = time.UnixMilli(6000)
	r.snapshotOnce()
	assert.Equal(t, 0, r.TrackedCount(), "GCAfter 경과 시 버퍼 만료")
	assert.Empty(t, r.History("gone", 0))
}

// --- History limit/clamp/빈결과 ---

func TestDeviceHistoryRecorder_HistoryLimitClamp(t *testing.T) {
	dev := &histTestDevice{id: "d", online: true, properties: map[string]any{"n": 0}}
	reg := &histStubRegistry{}
	reg.set(dev)

	r := NewDeviceHistoryRecorder(reg, DeviceHistoryConfig{MaxEntries: 10})
	for i := 1; i <= 8; i++ {
		dev.properties = map[string]any{"n": i}
		r.snapshotOnce()
	}

	tests := []struct {
		name    string
		limit   int
		wantLen int
	}{
		{"limit 0 → MaxEntries clamp(보유분 전체)", 0, 8},
		{"limit 음수 → MaxEntries clamp", -5, 8},
		{"limit 3 → 3개", 3, 3},
		{"limit > MaxEntries → MaxEntries clamp(보유분)", 1000, 8},
		{"limit == 보유분", 8, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.History("d", tt.limit)
			assert.Len(t, got, tt.wantLen)
			if len(got) > 1 {
				// 최신순 보장: 첫 항목이 가장 큰 n.
				assert.Equal(t, 8, got[0].Properties["n"])
			}
		})
	}
}

func TestDeviceHistoryRecorder_HistoryEmpty(t *testing.T) {
	r := NewDeviceHistoryRecorder(&histStubRegistry{}, DeviceHistoryConfig{MaxEntries: 10})

	// 알 수 없는 디바이스 → 빈 슬라이스(nil 아님).
	got := r.History("unknown", 0)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

// TestDeviceHistoryRecorder_SkipsEmptyID 는 ID 가 빈 디바이스를 건너뛰는지 검증한다.
func TestDeviceHistoryRecorder_SkipsEmptyID(t *testing.T) {
	reg := &histStubRegistry{}
	reg.set(&histTestDevice{id: "", online: true, properties: map[string]any{"v": 1}})

	r := NewDeviceHistoryRecorder(reg, DeviceHistoryConfig{MaxEntries: 10})
	r.snapshotOnce()

	assert.Equal(t, 0, r.TrackedCount())
}

// --- Start/Stop 라이프사이클 ---

func TestDeviceHistoryRecorder_StartStop(t *testing.T) {
	dev := &histTestDevice{id: "d", online: true, properties: map[string]any{"v": 1}}
	reg := &histStubRegistry{}
	reg.set(dev)

	r := NewDeviceHistoryRecorder(reg, DeviceHistoryConfig{Interval: 5 * time.Millisecond, MaxEntries: 10})

	ctx, cancel := context.WithCancel(context.Background())
	r.Start(ctx)

	// 최소 1회 수집될 때까지 대기.
	require.Eventually(t, func() bool {
		return len(r.History("d", 0)) >= 1
	}, time.Second, 2*time.Millisecond)

	cancel()
	r.Wait() // 고루틴 정리 확인(타임아웃 없이 반환되어야 함)
}

// TestDeviceHistoryRecorder_StartNilRegistry 는 nil 레지스트리에서 Start 가 no-op
// 임을 검증한다(비활성 구성 안전).
func TestDeviceHistoryRecorder_StartNilRegistry(t *testing.T) {
	r := NewDeviceHistoryRecorder(nil, DeviceHistoryConfig{})
	r.Start(context.Background()) // 패닉 없이 즉시 반환
	assert.Equal(t, 0, r.TrackedCount())
}

// --- 동시성 ---

func TestDeviceHistoryRecorder_ConcurrentAccess(t *testing.T) {
	reg := &histStubRegistry{}
	devs := make([]Device, 0, 20)
	for i := 0; i < 20; i++ {
		devs = append(devs, &histTestDevice{
			id:         fmt.Sprintf("dev-%d", i),
			online:     true,
			properties: map[string]any{"n": 0},
		})
	}
	reg.set(devs...)

	r := NewDeviceHistoryRecorder(reg, DeviceHistoryConfig{Interval: time.Millisecond, MaxEntries: 50})

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 수집 고루틴들.
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					r.snapshotOnce()
				}
			}
		}()
	}

	// 조회 고루틴들.
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = r.History("dev-1", 10)
					_ = r.TrackedCount()
				}
			}
		}()
	}

	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()

	// race detector 가 문제를 잡지 않으면 통과.
	assert.GreaterOrEqual(t, r.TrackedCount(), 1)
}

// TestPropertiesEqual 은 비교 헬퍼의 엣지 케이스를 검증한다.
func TestPropertiesEqual(t *testing.T) {
	tests := []struct {
		name string
		a    map[string]any
		b    map[string]any
		want bool
	}{
		{"동일 스칼라", map[string]any{"x": 1, "y": "a"}, map[string]any{"x": 1, "y": "a"}, true},
		{"값 다름", map[string]any{"x": 1}, map[string]any{"x": 2}, false},
		{"키 개수 다름", map[string]any{"x": 1}, map[string]any{"x": 1, "y": 2}, false},
		{"키 이름 다름", map[string]any{"x": 1}, map[string]any{"z": 1}, false},
		{"빈 맵 동일", map[string]any{}, map[string]any{}, true},

		// 아래 케이스는 결함 수정 전 `false`(비교불가 타입 → 다름 간주)를 기대했다.
		// 그 기대가 곧 결함이었다: 중첩 컨테이너를 값에 담는 프로바이더에서 중복 억제가
		// 통째로 무력화되어 주기마다 이력 엔트리가 쌓였다. 이제 깊은 동등성을 기대한다.
		{"중첩 슬라이스 동일 → 같음", map[string]any{"x": []int{1}}, map[string]any{"x": []int{1}}, true},

		// --- 깊은 동등성 ---
		{"중첩 맵 동일", map[string]any{"m": map[string]any{"a": 1}}, map[string]any{"m": map[string]any{"a": 1}}, true},
		{"중첩 맵 값 다름", map[string]any{"m": map[string]any{"a": 1}}, map[string]any{"m": map[string]any{"a": 2}}, false},
		{"중첩 맵 키 다름", map[string]any{"m": map[string]any{"a": 1}}, map[string]any{"m": map[string]any{"b": 1}}, false},
		{
			"맵의 슬라이스 동일",
			map[string]any{"s": []map[string]any{{"a": 1}, {"b": 2}}},
			map[string]any{"s": []map[string]any{{"a": 1}, {"b": 2}}},
			true,
		},
		{
			"맵의 슬라이스 값 다름",
			map[string]any{"s": []map[string]any{{"a": 1}}},
			map[string]any{"s": []map[string]any{{"a": 9}}},
			false,
		},
		{"슬라이스 순서 다름 → 다름", map[string]any{"x": []int{1, 2}}, map[string]any{"x": []int{2, 1}}, false},
		{"슬라이스 길이 다름", map[string]any{"x": []int{1}}, map[string]any{"x": []int{1, 2}}, false},
		{"3중 중첩 동일", map[string]any{"a": map[string]any{"b": []any{map[string]any{"c": "d"}}}}, map[string]any{"a": map[string]any{"b": []any{map[string]any{"c": "d"}}}}, true},

		// nil vs 빈 컨테이너 규약: **구분한다**(다르다고 판정).
		// "아직 모른다"(nil)와 "비어 있음"은 이 코드베이스에서 의미가 다르며, 그 전이는
		// UI 가 렌더하는 payload 의 실제 shape 변화이므로 이력에 남는 것이 옳다.
		{"nil 슬라이스 vs 빈 슬라이스 → 다름", map[string]any{"x": []int(nil)}, map[string]any{"x": []int{}}, false},
		{"nil 맵 vs 빈 맵 → 다름", map[string]any{"x": map[string]any(nil)}, map[string]any{"x": map[string]any{}}, false},
		{"nil 슬라이스끼리 → 같음", map[string]any{"x": []int(nil)}, map[string]any{"x": []int(nil)}, true},
		{"빈 슬라이스끼리 → 같음", map[string]any{"x": []int{}}, map[string]any{"x": []int{}}, true},
		{"nil any vs nil 슬라이스 → 다름", map[string]any{"x": nil}, map[string]any{"x": []int(nil)}, false},
		{"nil any 끼리 → 같음", map[string]any{"x": nil}, map[string]any{"x": nil}, true},

		// 타입이 다르면 다르다(스칼라 == 경로의 기존 동작과 동일).
		{"같은 값 다른 타입", map[string]any{"x": 1}, map[string]any{"x": int64(1)}, false},

		// 잔여 보수성: func 은 양쪽 nil 이 아닌 한 항상 다름(패닉 없이).
		{"func 값 → 다름", map[string]any{"f": func() {}}, map[string]any{"f": func() {}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, propertiesEqual(tt.a, tt.b))
		})
	}
}

// BenchmarkPropertiesEqual 는 스냅샷 주기마다 디바이스당 1회 수행되는 비교 비용을
// 측정한다. 상한 크기(measurements 64개, gateways 8개)에서의 값이 관심사이다.
func BenchmarkPropertiesEqual(b *testing.B) {
	// 상한 크기의 ChirpStack 모양 properties.
	build := func() map[string]any {
		gws := make([]histGatewayView, 8)
		for i := range gws {
			gws[i] = histGatewayView{GatewayID: fmt.Sprintf("gw-%02d", i), RSSI: -90 - i, LastSeenMs: 1_700_000_000_000}
		}
		ms := make(map[string]any, 64)
		for i := 0; i < 64; i++ {
			ms[fmt.Sprintf("m%02d", i)] = map[string]any{"value": float64(i), "time_ms": int64(1_700_000_000_000)}
		}
		return map[string]any{"gateways": gws, "measurements": ms}
	}
	a, c := build(), build()

	b.Run("chirpstack상한_동일", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !propertiesEqual(a, c) {
				b.Fatal("동일해야 한다")
			}
		}
	})

	b.Run("스칼라만_동일", func(b *testing.B) {
		x := map[string]any{"temp": 22.5, "hum": 41, "on": true, "name": "s"}
		y := map[string]any{"temp": 22.5, "hum": 41, "on": true, "name": "s"}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !propertiesEqual(x, y) {
				b.Fatal("동일해야 한다")
			}
		}
	})
}
