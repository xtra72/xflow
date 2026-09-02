package device

import (
	"context"
	"reflect"
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

// HistoryComparable 은 **변화 감지 전용 비교 표면**을 제공하는 선택적 인터페이스이다.
//
// 문제: properties 중에는 저장값이 아니라 **조회 시각에 파생되는** 필드가 있다.
// ChirpStack 의 gateways[].stale 이 그 예로, time.Now() 와 offline 임계에서 파생되므로
// 디바이스가 아무것도 하지 않아도 임계 경과 순간 스스로 뒤집힌다. 그 뒤집힘을 변화로
// 세면 "디바이스가 한 일"을 전혀 나타내지 않는 이력 엔트리가 생긴다.
//
// 해법의 핵심은 **payload 와 비교 표면의 분리**이다. 반환 맵은 중복 억제 비교에만
// 쓰이고, 저장/직렬화되는 payload 는 여전히 State().Properties 전체이다 — 즉 stale 은
// "비교에서 제외"될 뿐 "payload 에서 제거"되지 않으므로 UI 의 staleness 표시는 그대로
// 동작한다.
//
// 이 방식을 고른 이유(대안 대비): 레코더 측 제외 목록은 범용 device 패키지가 특정
// 프로바이더의 필드 이름("stale")과 중첩 위치를 알아야 해서 문자열 결합이 생기고,
// 프로바이더가 파생 필드를 아예 내지 않게 하면 UI 가 필요한 정보를 잃는다. 어떤 필드가
// 조회 시각 파생인지는 **오직 프로바이더만 아는 사실**이므로 프로바이더가 답하게 한다.
//
// 계약: 반환 맵은 호출자 소유의 새 값이어야 한다(호출 간 재사용/변조 금지 — 레코더가
// 다음 틱까지 보관한다). 두 번째 반환값이 false 이면 레코더는 payload 를 그대로
// 비교에 쓴다(미구현 프로바이더와 동일 경로).
type HistoryComparable interface {
	HistoryComparisonProperties() (map[string]any, bool)
}

// HistoryEventTimed 는 스냅샷 수집 시각 대신 쓸 **실제 이벤트(수신) 시각**을 제공하는
// 선택적 인터페이스이다.
//
// 레코더는 주기 샘플러이므로 기본 Timestamp 는 샘플 시각이며, 그 값은 10초 격자에
// 정렬된다 — 실제 수신 시각이 아니다. 프로바이더가 properties 안에 업링크 파생 시각을
// 이미 싣고 있다면(ChirpStack 의 measurements[k].time_ms 등) 그쪽이 진실에 가깝다.
//
// 범용 device 패키지가 중첩 구조를 뒤져 "time_ms 같은 키"를 찾는 방식은 프로바이더별
// 규약을 범용 코드에 하드코딩하는 것이므로 택하지 않았다. 어느 값이 이벤트 시각인지는
// 프로바이더가 답한다.
//
// 계약: epoch milliseconds(프로젝트 규약). 시각을 알 수 없으면 (0, false) 를 반환하며
// 이때 레코더는 샘플 시각으로 폴백한다. 반환 시각은 **단조 비감소**여야 한다 — 이력은
// 삽입 순서로 최신순 정렬되므로, 뒤로 가는 시각은 표시 순서와 어긋난다.
type HistoryEventTimed interface {
	HistoryEventTimeMs() (int64, bool)
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

	// lastCompare 는 **마지막으로 append 된** 엔트리의 비교 표면이다(payload 아님).
	// HistoryComparable 프로바이더에서는 payload 와 다를 수 있으므로 따로 보관한다.
	// 중복으로 skip 된 틱은 이 값을 갱신하지 않는다 — 기준선은 기록된 엔트리이다.
	lastCompare map[string]any
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
//     Properties/Online 변화 기준). 비교는 중첩 구조까지 구조적으로 수행되며
//     (propertiesEqual), 조회 시각 파생 필드는 프로바이더가 비교 표면에서 제외할 수
//     있다(HistoryComparable) — 그렇지 않으면 디바이스가 아무 일도 하지 않았는데
//     파생값이 스스로 뒤집히면서 주기마다 무의미한 엔트리가 쌓인다.
//   - 엔트리 시각: 기본은 샘플 시각이나, 프로바이더가 실제 수신 시각을 알면 그 값을
//     쓴다(HistoryEventTimed).
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
		props := copyProperties(state.Properties)

		// 비교 표면은 payload 와 분리된다: 프로바이더가 조회 시각 파생 필드를 중립화한
		// 표면을 제공하면 그것을 쓰고(HistoryComparable), 아니면 payload 를 그대로 쓴다.
		compare := props
		if hc, ok := d.(HistoryComparable); ok {
			if alt, has := hc.HistoryComparisonProperties(); has {
				compare = alt
			}
		}

		// 엔트리 시각: 프로바이더가 실제 이벤트(수신) 시각을 알면 그것을 쓰고, 아니면
		// 샘플 시각으로 폴백한다(HistoryEventTimed).
		ts := nowMs
		if he, ok := d.(HistoryEventTimed); ok {
			if ev, has := he.HistoryEventTimeMs(); has && ev > 0 {
				ts = ev
			}
		}

		snap := HistorySnapshot{
			Timestamp:  ts,
			Online:     d.Online(),
			LastSeen:   d.LastSeen().UnixMilli(),
			Properties: props,
		}

		ring := r.rings[id]
		if ring == nil {
			ring = &deviceRing{}
			r.rings[id] = ring
		}
		ring.lastSeenGC = nowMs

		// 중복 억제: 직전 기록 엔트리와 비교 표면 + Online 이 동일하면 추가하지 않는다.
		if prev, ok := ring.last(r.cfg.MaxEntries); ok {
			if prev.Online == snap.Online && propertiesEqual(ring.lastCompare, compare) {
				continue
			}
		}
		ring.append(snap, r.cfg.MaxEntries)
		ring.lastCompare = compare
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
// 값 비교는 valuesEqual 에 위임하며 **중첩 구조까지 구조적으로** 비교한다. 이전 구현은
// 슬라이스/맵 값을 만나면 무조건 "다르다"로 판정했는데, 이는 중첩 properties 를 내는
// 프로바이더에서 중복 억제를 통째로 무력화했다(ChirpStack 은 gateways 슬라이스 +
// measurements 맵만 내므로 매 틱 무조건 다르다고 판정되어, 값이 한 글자도 변하지
// 않아도 주기마다 이력 엔트리가 쌓였다).
func propertiesEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		if !valuesEqual(av, bv) {
			return false
		}
	}
	return true
}

// valuesEqual 은 properties 값 1쌍의 동등성을 판정한다.
//
// 스칼라는 == 로 직접 비교하고(대다수 프로바이더의 경로 — reflect 를 아예 타지 않는다),
// 그 밖의 값은 reflect.DeepEqual 로 구조 비교한다.
//
// reflect.DeepEqual 선택 근거: 비교 대상이 map[string]any / []any 로 한정되지 않는다.
// ChirpStack 의 gateways 는 []deviceGatewayView — 프로바이더 고유 **구조체 슬라이스**
// 이므로, map/slice 만 손으로 재귀하는 구현으로는 정작 문제의 값을 비교하지 못한다.
// DeepEqual 은 임의 타입에 대해 동작하며 패닉하지 않는다(비교 불가 값도 안전).
//
// 여전히 "다름"으로 떨어지는 경우(의도된 잔여 보수성):
//   - func 값: 양쪽 모두 nil 일 때만 같다고 본다 — 그 외에는 항상 다름.
//   - NaN: NaN != NaN 이라 항상 다름(스칼라 == 경로의 기존 동작과 동일).
//   - nil 과 빈 슬라이스/맵: 서로 다르다고 본다(아래 규약 참조).
//
// nil vs 빈 컨테이너 규약: **구분한다**(다르다고 판정). 이 코드베이스는 "아직 모른다"
// (키 부재/nil)와 "비어 있음"을 적극적으로 구분하며(chirpstack provider 의 부재 표현
// 규약), nil → 빈 슬라이스 전이는 UI 가 렌더하는 payload 의 실제 shape 변화이므로
// 이력에 남는 것이 옳다. DeepEqual 의 기본 의미론이 그대로 이 규약이다.
func valuesEqual(a, b any) bool {
	if valueComparable(a) && valueComparable(b) {
		return a == b
	}
	return reflect.DeepEqual(a, b)
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
