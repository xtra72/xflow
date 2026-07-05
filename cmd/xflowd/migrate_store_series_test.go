// @spec SPEC-STORE-004 (O3, M6)
// migrate_store_series_test.go — store-series 서브명령 진입점 테스트.
//
// cmd 계층은 thin wrapper 이므로 (1) 명령이 migrate 그룹에 등록되는지,
// (2) noop 안내가 올바르게 출력되는지, (3) 향후 영속 백엔드 주입 경로
// (runStoreSeriesMigration) 가 fake repository 로 동작하는지를 검증한다.
// 실제 변환 정확성/멱등성/dry-run/검증은 internal/migrate/storeseries 단위 테스트가 담당한다.
package main

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

// TestMigrateStoreSeriesCmd_Registered 는 store-series 가 migrate 그룹에 등록되는지 확인한다.
func TestMigrateStoreSeriesCmd_Registered(t *testing.T) {
	t.Parallel()

	root := newMigrateCmd()
	var found bool
	for _, c := range root.Commands() {
		if c.Name() == "store-series" {
			found = true
			break
		}
	}
	assert.True(t, found, "migrate 그룹에 store-series 서브명령이 등록되어야 한다")
}

// TestMigrateStoreSeriesCmd_Noop 는 활성 영속 백엔드가 없을 때 안내 noop 을 출력하는지 확인한다.
func TestMigrateStoreSeriesCmd_Noop(t *testing.T) {
	t.Parallel()

	cmd := newMigrateStoreSeriesCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})

	require.NoError(t, cmd.Execute())

	output := out.String()
	assert.Contains(t, output, "noop")
	assert.Contains(t, output, "VolatileStore")
}

// cmdFakeRepo 는 runStoreSeriesMigration 의 향후 주입 경로 검증용 in-memory Repository 이다.
type cmdFakeRepo struct {
	mu      sync.Mutex
	entries map[string]*system.StoreEntry
}

func newCmdFakeRepo() *cmdFakeRepo {
	return &cmdFakeRepo{entries: make(map[string]*system.StoreEntry)}
}

func (r *cmdFakeRepo) ListKeys(_ context.Context, _ string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := make([]string, 0, len(r.entries))
	for k := range r.entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

func (r *cmdFakeRepo) GetEntry(_ context.Context, key string) (*system.StoreEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[key]
	if !ok {
		return nil, system.ErrKeyNotFound
	}
	cp := *e
	return &cp, nil
}

func (r *cmdFakeRepo) SetEntry(_ context.Context, key string, entry *system.StoreEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *entry
	r.entries[key] = &cp
	return nil
}

func (r *cmdFakeRepo) DeleteEntry(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, key)
	return nil
}

// TestRunStoreSeriesMigration_WithFakeRepo 는 향후 영속 백엔드 주입 경로가 fake repo 로
// 변환을 수행함을 확인한다 (핵심 로직 보존 검증).
func TestRunStoreSeriesMigration_WithFakeRepo(t *testing.T) {
	t.Parallel()

	repo := newCmdFakeRepo()
	repo.entries["room"] = &system.StoreEntry{Value: 22}

	var stdout, stderr bytes.Buffer
	err := runStoreSeriesMigration(context.Background(), repo, &migrateStoreSeriesFlags{}, &stdout, &stderr)
	require.NoError(t, err)

	repo.mu.Lock()
	_, hasOld := repo.entries["room"]
	_, hasNew := repo.entries["unknown||room"]
	repo.mu.Unlock()

	assert.False(t, hasOld, "변환 후 bare key 는 제거되어야 한다")
	assert.True(t, hasNew, "기본 시리즈 키로 변환되어야 한다")
	assert.Contains(t, stdout.String(), "converted=1")
}

// TestRunStoreSeriesMigration_DryRun 은 dry-run 이 리포지토리를 변경하지 않음을 확인한다.
func TestRunStoreSeriesMigration_DryRun(t *testing.T) {
	t.Parallel()

	repo := newCmdFakeRepo()
	repo.entries["room"] = &system.StoreEntry{Value: 22}

	var stdout, stderr bytes.Buffer
	err := runStoreSeriesMigration(context.Background(), repo, &migrateStoreSeriesFlags{dryRun: true}, &stdout, &stderr)
	require.NoError(t, err)

	repo.mu.Lock()
	_, hasOld := repo.entries["room"]
	repo.mu.Unlock()
	assert.True(t, hasOld, "dry-run 은 원본을 보존해야 한다")
}
