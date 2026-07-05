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
		{"비교불가 타입(슬라이스) → 다름 간주", map[string]any{"x": []int{1}}, map[string]any{"x": []int{1}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, propertiesEqual(tt.a, tt.b))
		})
	}
}
