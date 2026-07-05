package device

import (
	"context"
	"sync"
	"time"
)

// HistorySnapshot 은 특정 시점에 기록된 디바이스 상태 스냅샷이다.
//
// 디바이스 ID(UUID)별 링버퍼에 누적되며, 주기 스냅샷 방식으로 수집된다(수신마다의
// 중앙 이벤트가 없으므로 일정 주기로 현재 State 를 포착한다). Properties 는 수집
// 시점의 깊은 복사본(얕은 map 복사)이며, 원본 디바이스 상태 변경에 영향받지 않는다.
type HistorySnapshot struct {
	// Timestamp 는 스냅샷 수집 시각(epoch milliseconds, 프로젝트 규약)이다.
	Timestamp int64 `json:"timestamp"`

	// Online 은 수집 시점의 온라인 여부이다.
	Online bool `json:"online"`

	// LastSeen 은 수집 시점의 디바이스 LastSeen(epoch milliseconds)이다.
	LastSeen int64 `json:"last_seen"`

	// Properties 는 수집 시점의 프로토콜별 속성 맵의 복사본이다.
	Properties map[string]any `json:"properties"`
}

// historyRegistry 는 DeviceHistoryRecorder 가 의존하는 최소 레지스트리 인터페이스이다.
//
// 레코더는 도메인 순수성을 위해 전체 DeviceRegistry 가 아닌 List 만 의존한다(의존성
// 방향: device → device, 외부 계층 의존 없음). inMemoryRegistry 가 이를 만족한다.
type historyRegistry interface {
	List(filter DeviceFilter) []Device
}

// deviceRing 은 단일 디바이스의 고정 용량 링버퍼이다.
//
// 최신 maxEntries 개의 스냅샷만 보관하며, 가득 차면 가장 오래된 항목을 덮어쓴다.
// lastSeenGC 는 레지스트리에서 사라진 디바이스의 만료 판정(GC)을 위해 마지막으로
// 레지스트리에 존재했던 시각을 기록한다.
type deviceRing struct {
	entries    []HistorySnapshot // 원형 저장소(cap == maxEntries)
	start      int               // 가장 오래된 항목의 인덱스
	size       int               // 현재 저장된 항목 수
	lastSeenGC int64             // 마지막으로 레지스트리에 존재한 시각(epoch ms, GC용)
}

// append 는 링버퍼에 스냅샷을 추가한다. 가득 차면 가장 오래된 항목을 덮어쓴다.
func (r *deviceRing) append(snap HistorySnapshot, maxEntries int) {
	if cap(r.entries) == 0 {
		r.entries = make([]HistorySnapshot, maxEntries)
	}
	idx := (r.start + r.size) % maxEntries
	r.entries[idx] = snap
	if r.size < maxEntries {
		r.size++
	} else {
		// 가득 참 — start 를 한 칸 전진(가장 오래된 항목 폐기).
		r.start = (r.start + 1) % maxEntries
	}
}

// last 는 가장 최근 스냅샷을 반환한다(중복 억제 비교용). 비어 있으면 (zero, false).
func (r *deviceRing) last(maxEntries int) (HistorySnapshot, bool) {
	if r.size == 0 {
		return HistorySnapshot{}, false
	}
	idx := (r.start + r.size - 1) % maxEntries
	return r.entries[idx], true
}

// snapshots 는 최신순(newest-first)으로 최대 limit 개의 스냅샷을 반환한다.
func (r *deviceRing) snapshots(maxEntries, limit int) []HistorySnapshot {
	if r.size == 0 || limit <= 0 {
		return []HistorySnapshot{}
	}
	n := r.size
	if limit < n {
		n = limit
	}
	out := make([]HistorySnapshot, 0, n)
	for i := 0; i < n; i++ {
		// 최신 → 과거 순으로 역방향 순회.
		idx := (r.start + r.size - 1 - i + maxEntries*2) % maxEntries
		out = append(out, r.entries[idx])
	}
	return out
}

// DeviceHistoryConfig 는 DeviceHistoryRecorder 의 동작 설정이다.
type DeviceHistoryConfig struct {
	// Interval 은 스냅샷 수집 주기이다.
	Interval time.Duration

	// MaxEntries 는 디바이스별 링버퍼 최대 보관 개수이다.
	MaxEntries int

	// GCAfter 는 레지스트리에서 사라진 디바이스의 버퍼를 만료(삭제)하기까지의
	// 유예 시간이다. 0 이면 기본값(10 * Interval)을 사용한다.
	GCAfter time.Duration
}

// 기본값.
const (
	defaultHistoryInterval   = 10 * time.Second
	defaultHistoryMaxEntries = 100
)

// DeviceHistoryRecorder 는 주기적으로 전체 디바이스의 현재 상태를 스냅샷하여
// 디바이스별 링버퍼에 보관한다.
//
// 동작:
//   - Start 가 호출되면 ticker 기반 고루틴이 Interval 마다 registry.List(filter{})
//     로 전체 디바이스를 조회하고 각 디바이스의 State()를 스냅샷한다.
//   - 중복 억제: 직전 스냅샷과 Properties + Online 이 동일하면 새 엔트리를 추가하지
//     않는다(불필요한 중복 방지). LastSeen 만 갱신된 경우는 기록하지 않는다(기본은
//     Properties/Online 변화 기준).
//   - GC: 레지스트리에 더 이상 존재하지 않는 디바이스는 GCAfter 경과 후 버퍼를 삭제한다.
//
// 동시성: 모든 상태 접근은 RWMutex 로 보호된다. History 조회는 수집 고루틴과 안전하게
// 병행된다.
type DeviceHistoryRecorder struct {
	registry historyRegistry
	cfg      DeviceHistoryConfig

	mu    sync.RWMutex
	rings map[string]*deviceRing // device ID(UUID) -> 링버퍼

	// now 는 시각 주입 지점(테스트 결정성). 기본 time.Now.
	now func() time.Time

	stopOnce sync.Once
	stopped  chan struct{}
}

// NewDeviceHistoryRecorder 는 새 레코더를 생성한다. cfg 의 누락/무효 값은 기본값으로
// 보정된다(Interval<=0 → 10s, MaxEntries<=0 → 100, GCAfter<=0 → 10*Interval).
func NewDeviceHistoryRecorder(registry historyRegistry, cfg DeviceHistoryConfig) *DeviceHistoryRecorder {
	if cfg.Interval <= 0 {
		cfg.Interval = defaultHistoryInterval
	}
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = defaultHistoryMaxEntries
	}
	if cfg.GCAfter <= 0 {
		cfg.GCAfter = 10 * cfg.Interval
	}
	return &DeviceHistoryRecorder{
		registry: registry,
		cfg:      cfg,
		rings:    make(map[string]*deviceRing),
		now:      time.Now,
		stopped:  make(chan struct{}),
	}
}

// Start 는 수집 고루틴을 시작한다. ctx 취소 시 고루틴이 정리되고 stopped 가 닫힌다.
//
// registry 가 nil 이면 즉시 반환한다(no-op — 비활성 구성 안전).
func (r *DeviceHistoryRecorder) Start(ctx context.Context) {
	if r == nil || r.registry == nil {
		return
	}
	go r.run(ctx)
}

// run 은 ticker 루프이다. 매 주기 snapshot 을 수행하고, ctx 취소 시 종료한다.
func (r *DeviceHistoryRecorder) run(ctx context.Context) {
	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()
	defer close(r.stopped)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.snapshotOnce()
		}
	}
}

// Wait 는 수집 고루틴이 종료될 때까지 대기한다(graceful shutdown 보조 — 테스트/배선용).
func (r *DeviceHistoryRecorder) Wait() {
	<-r.stopped
}

// snapshotOnce 는 1회 스냅샷 사이클을 수행한다. 외부에서도 호출 가능하여 테스트가
// ticker 없이 결정적으로 수집을 트리거할 수 있다.
func (r *DeviceHistoryRecorder) snapshotOnce() {
	devices := r.registry.List(DeviceFilter{})
	nowMs := r.now().UnixMilli()

	r.mu.Lock()
	defer r.mu.Unlock()

	// 이번 사이클에 관측된 디바이스 ID 집합(GC 판정용).
	seen := make(map[string]struct{}, len(devices))

	for _, d := range devices {
		id := d.ID()
		if id == "" {
			// UUID 미해석 디바이스는 키가 불안정하므로 건너뛴다(graceful degradation).
			continue
		}
		seen[id] = struct{}{}

		state := d.State()
		snap := HistorySnapshot{
			Timestamp:  nowMs,
			Online:     d.Online(),
			LastSeen:   d.LastSeen().UnixMilli(),
			Properties: copyProperties(state.Properties),
		}

		ring := r.rings[id]
		if ring == nil {
			ring = &deviceRing{}
			r.rings[id] = ring
		}
		ring.lastSeenGC = nowMs

		// 중복 억제: 직전 스냅샷과 Properties + Online 이 동일하면 추가하지 않는다.
		if prev, ok := ring.last(r.cfg.MaxEntries); ok {
			if prev.Online == snap.Online && propertiesEqual(prev.Properties, snap.Properties) {
				continue
			}
		}
		ring.append(snap, r.cfg.MaxEntries)
	}

	// GC: 이번 사이클에 미관측 + GCAfter 경과한 디바이스 버퍼 삭제.
	for id, ring := range r.rings {
		if _, ok := seen[id]; ok {
			continue
		}
		if nowMs-ring.lastSeenGC >= r.cfg.GCAfter.Milliseconds() {
			delete(r.rings, id)
		}
	}
}

// History 는 device ID(UUID)의 스냅샷을 최신순으로 최대 limit 개 반환한다.
//
// 디바이스가 없거나 이력이 없으면 빈 슬라이스를 반환한다(에러 아님). limit <= 0 또는
// limit > MaxEntries 이면 MaxEntries 로 clamp 한다.
func (r *DeviceHistoryRecorder) History(deviceID string, limit int) []HistorySnapshot {
	if r == nil {
		return []HistorySnapshot{}
	}
	if limit <= 0 || limit > r.cfg.MaxEntries {
		limit = r.cfg.MaxEntries
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	ring := r.rings[deviceID]
	if ring == nil {
		return []HistorySnapshot{}
	}
	return ring.snapshots(r.cfg.MaxEntries, limit)
}

// MaxEntries 는 디바이스별 링버퍼 상한을 반환한다(핸들러의 limit clamp 기본값 산출용).
func (r *DeviceHistoryRecorder) MaxEntries() int {
	if r == nil {
		return defaultHistoryMaxEntries
	}
	return r.cfg.MaxEntries
}

// TrackedCount 는 현재 버퍼를 보유한 디바이스 수를 반환한다(테스트/관측용).
func (r *DeviceHistoryRecorder) TrackedCount() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.rings)
}

// copyProperties 는 properties 맵의 얕은 복사본을 반환한다.
//
// 값은 프로토콜별 스칼라(숫자/문자열/bool)가 대부분이므로 얕은 복사로 충분하다.
// nil 입력은 비어 있지 않은 빈 맵으로 정규화하여 이후 비교/직렬화를 단순화한다.
func copyProperties(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// propertiesEqual 은 두 properties 맵이 동등한지(키 집합 + 값) 비교한다.
//
// 값 비교는 == 으로 수행한다. 스칼라(숫자/문자열/bool)에 대해 안전하며, 비교 불가능한
// 타입(슬라이스/맵)이 값에 포함되면 패닉을 피하기 위해 동등하지 않은 것으로 간주한다
// (보수적 — 변화로 판단하여 기록).
func propertiesEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		if !valueComparable(av) || !valueComparable(bv) {
			// 비교 불가 타입 — 보수적으로 다르다고 판단(기록 누락 방지).
			return false
		}
		if av != bv {
			return false
		}
	}
	return true
}

// valueComparable 은 값이 == 비교에 안전한지(comparable kind) 검사한다.
func valueComparable(v any) bool {
	switch v.(type) {
	case nil, bool, string,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, uintptr,
		float32, float64,
		complex64, complex128:
		return true
	default:
		return false
	}
}
