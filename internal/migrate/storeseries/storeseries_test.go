// @spec SPEC-STORE-004 (O3, M6)
// storeseries_test.go — 시리즈 마이그레이션 유틸 단위 테스트.
//
// 검증 항목 (plan.md 리스크 "마이그레이션 데이터 손실" 대응):
//   - 변환 정확성: bare key → (key,"unknown",{}) 기본 시리즈 인코딩 키.
//   - 멱등성(idempotency): 이미 시리즈 인코딩된 키는 건너뛰고, 재실행 시 추가 변환 없음.
//   - dry-run 무변경: dry-run 모드는 리포지토리를 전혀 변경하지 않는다.
//   - 검증: 변환 후 value sha256 합산이 백업과 일치한다.
//   - 데이터 손실 방지: 변환 후 원본 키는 사라지고 새 키로 값이 보존된다.
package storeseries

import (
	"bytes"
	"context"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent/system"
)

// fakeRepo 는 테스트용 in-memory Repository 구현이다 (라이브 데이터 미사용).
type fakeRepo struct {
	mu      sync.Mutex
	entries map[string]*system.StoreEntry
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{entries: make(map[string]*system.StoreEntry)}
}

func (r *fakeRepo) put(key string, value any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[key] = &system.StoreEntry{Value: value}
}

func (r *fakeRepo) ListKeys(_ context.Context, _ string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := make([]string, 0, len(r.entries))
	for k := range r.entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

func (r *fakeRepo) GetEntry(_ context.Context, key string) (*system.StoreEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[key]
	if !ok {
		return nil, system.ErrKeyNotFound
	}
	cp := *e
	return &cp, nil
}

func (r *fakeRepo) SetEntry(_ context.Context, key string, entry *system.StoreEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *entry
	r.entries[key] = &cp
	return nil
}

func (r *fakeRepo) DeleteEntry(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, key)
	return nil
}

func (r *fakeRepo) snapshot() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]any, len(r.entries))
	for k, e := range r.entries {
		out[k] = e.Value
	}
	return out
}

// TestPlan_ClassifiesBareAndSeriesKeys 는 Plan 이 bare key 와 이미-시리즈 키를 올바르게
// 분류하는지 확인한다.
func TestPlan_ClassifiesBareAndSeriesKeys(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	repo.put("room", 22)                      // bare key → convert
	repo.put("device-uuid-1234", "online")    // bare key → convert
	repo.put("unknown||room", 99)             // 이미 시리즈(기본) → skip
	repo.put("temperature|area=a|sensor", 21) // 이미 시리즈 → skip

	p, err := NewPlanner(repo, Options{})
	require.NoError(t, err)

	plan, err := p.Plan(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 2, plan.ConvertCount(), "bare key 2개가 변환 대상이어야 한다")
	assert.Equal(t, 2, plan.AlreadySeriesCount(), "시리즈 키 2개는 건너뛰어야 한다")

	// 변환 키가 (key,"unknown",{}) 기본 시리즈로 매핑되는지 확인.
	conv := plan.Convert()
	got := map[string]string{}
	for _, e := range conv {
		got[e.OldKey] = e.NewKey
	}
	assert.Equal(t, "unknown||room", got["room"])
	assert.Equal(t, "unknown||device-uuid-1234", got["device-uuid-1234"])
}

// TestApply_DryRunMakesNoChanges 는 dry-run 모드가 리포지토리를 전혀 변경하지 않음을 확인한다.
func TestApply_DryRunMakesNoChanges(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	repo.put("room", 22)
	repo.put("humidity", 55)
	before := repo.snapshot()

	var out bytes.Buffer
	p, err := NewPlanner(repo, Options{DryRun: true, Stdout: &out})
	require.NoError(t, err)

	_, err = p.Plan(context.Background())
	require.NoError(t, err)

	res, err := p.Apply(context.Background())
	require.NoError(t, err)

	assert.True(t, res.DryRun)
	assert.Equal(t, before, repo.snapshot(), "dry-run 은 리포지토리를 변경하면 안 된다")
	assert.Contains(t, out.String(), "dry-run")
}

// TestApply_ConvertsBareKeysAndPreservesValues 는 실제 변환이 정확하고 값을 보존하며
// 검증을 통과하는지 확인한다 (데이터 손실 방지).
func TestApply_ConvertsBareKeysAndPreservesValues(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	repo.put("room", 22)
	repo.put("status", "online")

	var backup bytes.Buffer
	p, err := NewPlanner(repo, Options{Backup: &backup})
	require.NoError(t, err)

	_, err = p.Plan(context.Background())
	require.NoError(t, err)

	res, err := p.Apply(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 2, res.Converted)
	assert.True(t, res.Verified, "변환 후 value sha256 합산이 백업과 일치해야 한다")
	assert.NotEmpty(t, backup.String(), "백업 manifest 가 기록되어야 한다")

	// 원본 bare key 는 사라지고, 기본 시리즈 키로 값이 보존되어야 한다.
	snap := repo.snapshot()
	_, hasOldRoom := snap["room"]
	_, hasOldStatus := snap["status"]
	assert.False(t, hasOldRoom, "변환 후 원본 bare key 는 제거되어야 한다")
	assert.False(t, hasOldStatus)

	assert.EqualValues(t, 22, snap["unknown||room"])
	assert.Equal(t, "online", snap["unknown||status"])
}

// TestApply_Idempotent 는 두 번 실행해도 추가 변환이 없음을 확인한다 (멱등성).
func TestApply_Idempotent(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	repo.put("room", 22)

	ctx := context.Background()

	// 1차 실행.
	p1, err := NewPlanner(repo, Options{})
	require.NoError(t, err)
	_, err = p1.Plan(ctx)
	require.NoError(t, err)
	res1, err := p1.Apply(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, res1.Converted)

	afterFirst := repo.snapshot()

	// 2차 실행 — 이미 시리즈 키이므로 변환 대상 0.
	p2, err := NewPlanner(repo, Options{})
	require.NoError(t, err)
	plan2, err := p2.Plan(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, plan2.ConvertCount(), "재실행 시 변환 대상이 없어야 한다 (멱등)")
	assert.Equal(t, 1, plan2.AlreadySeriesCount())

	res2, err := p2.Apply(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, res2.Converted)
	assert.Equal(t, afterFirst, repo.snapshot(), "2차 실행은 리포지토리를 바꾸면 안 된다")
}

// TestApply_RequiresPlanFirst 는 Plan 없이 Apply 호출 시 에러를 반환하는지 확인한다.
func TestApply_RequiresPlanFirst(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	p, err := NewPlanner(repo, Options{})
	require.NoError(t, err)

	_, err = p.Apply(context.Background())
	assert.Error(t, err)
}

// TestNewPlanner_NilRepo 는 nil 리포지토리에 대해 에러를 반환하는지 확인한다.
func TestNewPlanner_NilRepo(t *testing.T) {
	t.Parallel()

	_, err := NewPlanner(nil, Options{})
	assert.Error(t, err)
}

// TestIsCanonicalSeriesKey 는 정규 시리즈 키 판정의 경계 케이스를 확인한다.
func TestIsCanonicalSeriesKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"bare key", "room", false},
		{"bare key with dot", "indoor.1.temp", false},
		{"default series", "unknown||room", true},
		{"series with tags", "temperature|area=a|room", true},
		{"series with multi tags sorted", "temp|floor=2,room=1|t", true},
		// `|` 를 포함하지만 정규 형식이 아닌 bare key (round-trip 불일치 → convert 대상).
		{"non-canonical pipe key", "a|b", false},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, isCanonicalSeriesKey(tc.key))
		})
	}
}

// TestPlan_Print 는 Plan.Print 가 요약 + convert/already-series 상세를 출력하는지 확인한다.
func TestPlan_Print(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	repo.put("room", 22)           // convert
	repo.put("unknown||sensor", 1) // already-series

	p, err := NewPlanner(repo, Options{})
	require.NoError(t, err)
	plan, err := p.Plan(context.Background())
	require.NoError(t, err)

	var out bytes.Buffer
	require.NoError(t, plan.Print(&out))

	s := out.String()
	assert.Contains(t, s, "Plan summary: convert=1 already-series=1")
	assert.Contains(t, s, "[convert]")
	assert.Contains(t, s, `"room" -> "unknown||room"`)
	assert.Contains(t, s, "[already-series]")
}

// TestResult_Print 는 Result.Print 출력 포맷을 확인한다.
func TestResult_Print(t *testing.T) {
	t.Parallel()

	r := &Result{Converted: 3, Skipped: 1, Verified: true, DryRun: false}
	var out bytes.Buffer
	require.NoError(t, r.Print(&out))
	assert.Contains(t, out.String(), "Result: converted=3 skipped=1 verified=true dry-run=false")
}

// errRepo 는 GetEntry 에서 에러를 반환하는 Repository 로, Apply 의 백업 조회 실패 경로를 검증한다.
type errRepo struct {
	*fakeRepo
}

func (e errRepo) GetEntry(_ context.Context, _ string) (*system.StoreEntry, error) {
	return nil, system.ErrKeyNotFound
}

// TestApply_BackupGetFails 는 변환 대상 엔트리 백업 조회 실패 시 에러를 반환하는지 확인한다.
func TestApply_BackupGetFails(t *testing.T) {
	t.Parallel()

	base := newFakeRepo()
	base.put("room", 22)
	repo := errRepo{fakeRepo: base}

	p, err := NewPlanner(repo, Options{})
	require.NoError(t, err)
	_, err = p.Plan(context.Background())
	require.NoError(t, err)

	_, err = p.Apply(context.Background())
	assert.Error(t, err, "백업용 GetEntry 실패 시 Apply 는 에러를 반환해야 한다")
}

// TestApply_BackupManifestWritten 은 Backup writer 에 manifest JSON 이 기록되는지 확인한다.
func TestApply_BackupManifestWritten(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	repo.put("room", 22)

	var backup bytes.Buffer
	p, err := NewPlanner(repo, Options{Backup: &backup, DryRun: true})
	require.NoError(t, err)
	_, err = p.Plan(context.Background())
	require.NoError(t, err)
	_, err = p.Apply(context.Background())
	require.NoError(t, err)

	assert.Contains(t, backup.String(), "value_sha256_sum")
	assert.Contains(t, backup.String(), "conversions")
}

// 컴파일 타임: 어떤 system.StoreRepository 든 본 Repository 인터페이스를 만족한다.
var _ Repository = (system.StoreRepository)(nil)
