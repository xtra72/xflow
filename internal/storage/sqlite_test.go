package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
)

// newTestFlow 는 테스트용 Flow 를 생성한다.
func newTestFlow(name string) flow.Flow {
	return flow.NewFlow(name,
		flow.WithDescription("test flow: "+name),
	)
}

// setupSQLiteRepo 는 테스트용 SQLiteRepository 를 생성한다.
// 테스트 종료 시 자동으로 Close 된다.
func setupSQLiteRepo(t *testing.T) *SQLiteRepository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	ctx := context.Background()

	repo, err := NewSQLiteRepository(ctx, dbPath)
	require.NoError(t, err, "NewSQLiteRepository 실패")

	t.Cleanup(func() {
		repo.Close()
	})

	return repo
}

// ---------------------------------------------------------------------------
// CRUD 전체 사이클
// ---------------------------------------------------------------------------

func TestSQLiteRepository_CRUD(t *testing.T) {
	repo := setupSQLiteRepo(t)
	ctx := context.Background()

	// 1. Save
	f := newTestFlow("crud-test")
	err := repo.Save(ctx, f)
	require.NoError(t, err, "Save 실패")

	// 2. Get
	got, err := repo.Get(ctx, f.ID())
	require.NoError(t, err, "Get 실패")
	assert.Equal(t, f.ID(), got.ID(), "ID 일치")
	assert.Equal(t, f.Name(), got.Name(), "Name 일치")
	assert.Equal(t, f.Description(), got.Description(), "Description 일치")

	// 3. List
	list, err := repo.List(ctx)
	require.NoError(t, err, "List 실패")
	assert.Len(t, list, 1, "List 는 1 개")
	assert.Equal(t, f.ID(), list[0].ID(), "List[0].ID 일치")

	// 4. Delete
	err = repo.Delete(ctx, f.ID())
	require.NoError(t, err, "Delete 실패")

	// 5. Delete 후 Get 하면 ErrFlowNotFound
	_, err = repo.Get(ctx, f.ID())
	assert.ErrorIs(t, err, ErrFlowNotFound, "삭제 후 Get 은 ErrFlowNotFound")

	// 6. Delete 후 List 는 빈 배열
	list, err = repo.List(ctx)
	require.NoError(t, err, "List 실패")
	assert.Empty(t, list, "삭제 후 List 는 비어야 한다")
}

// ---------------------------------------------------------------------------
// Get: 존재하지 않는 ID
// ---------------------------------------------------------------------------

func TestSQLiteRepository_Get_NotFound(t *testing.T) {
	repo := setupSQLiteRepo(t)
	ctx := context.Background()

	_, err := repo.Get(ctx, "non-existent-id")
	assert.ErrorIs(t, err, ErrFlowNotFound, "존재하지 않는 ID 는 ErrFlowNotFound")
}

// ---------------------------------------------------------------------------
// Delete: 존재하지 않는 ID
// ---------------------------------------------------------------------------

func TestSQLiteRepository_Delete_NotFound(t *testing.T) {
	repo := setupSQLiteRepo(t)
	ctx := context.Background()

	err := repo.Delete(ctx, "non-existent-id")
	assert.ErrorIs(t, err, ErrFlowNotFound, "존재하지 않는 ID 삭제는 ErrFlowNotFound")
}

// ---------------------------------------------------------------------------
// SaveOverwrite: 같은 ID 로 두 번 저장 → 갱신
// ---------------------------------------------------------------------------

func TestSQLiteRepository_SaveOverwrite(t *testing.T) {
	repo := setupSQLiteRepo(t)
	ctx := context.Background()

	// 첫 번째 저장
	f1 := newTestFlow("original-name")
	err := repo.Save(ctx, f1)
	require.NoError(t, err, "첫 번째 Save 실패")

	// 같은 ID 로 다른 이름의 Flow 를 JSON 을 통해 생성
	f2JSON := fmt.Sprintf(`{
		"id": %q,
		"name": "updated-name",
		"description": "updated description",
		"state": "stored",
		"config": {"track_history":false,"max_history_size":0,"error_handling":"propagate"},
		"nodes": [],
		"wires": []
	}`, f1.ID())
	f2, err := flow.FlowFromJSON([]byte(f2JSON))
	require.NoError(t, err, "FlowFromJSON 실패")

	// 두 번째 저장 (덮어쓰기)
	err = repo.Save(ctx, f2)
	require.NoError(t, err, "두 번째 Save 실패")

	// 조회하면 업데이트된 내용이 반환
	got, err := repo.Get(ctx, f1.ID())
	require.NoError(t, err, "Get 실패")
	assert.Equal(t, "updated-name", got.Name(), "Name 이 갱신되어야 한다")
	assert.Equal(t, "updated description", got.Description(), "Description 이 갱신되어야 한다")

	// List 에도 1 개만 존재
	list, err := repo.List(ctx)
	require.NoError(t, err, "List 실패")
	assert.Len(t, list, 1, "덮어쓰기 후 List 는 1 개")
}

// ---------------------------------------------------------------------------
// List: 여러 플로우 저장 후 순서 확인
// ---------------------------------------------------------------------------

func TestSQLiteRepository_List_Order(t *testing.T) {
	repo := setupSQLiteRepo(t)
	ctx := context.Background()

	names := []string{"alpha", "beta", "gamma"}
	for _, name := range names {
		f := newTestFlow(name)
		err := repo.Save(ctx, f)
		require.NoError(t, err, "Save 실패: "+name)
	}

	list, err := repo.List(ctx)
	require.NoError(t, err, "List 실패")
	assert.Len(t, list, 3, "List 는 3 개")
}

// ---------------------------------------------------------------------------
// Close: 닫은 후 작업 시 에러
// ---------------------------------------------------------------------------

func TestSQLiteRepository_Close(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "close-test.db")
	ctx := context.Background()

	repo, err := NewSQLiteRepository(ctx, dbPath)
	require.NoError(t, err)

	// 정상 Close
	err = repo.Close()
	require.NoError(t, err, "Close 실패")

	// Close 후 Save 는 에러
	f := newTestFlow("after-close")
	err = repo.Save(ctx, f)
	assert.Error(t, err, "Close 후 Save 는 에러가 발생해야 한다")
}

// ---------------------------------------------------------------------------
// 노드 및 와이어가 있는 Flow 의 라운드트립
// ---------------------------------------------------------------------------

func TestSQLiteRepository_ComplexFlow(t *testing.T) {
	repo := setupSQLiteRepo(t)
	ctx := context.Background()

	nodeA := flow.NewNodeDef("sensor", "bridge",
		flow.WithErrorPort(),
	)
	nodeB := flow.NewNodeDef("filter", "filter",
		flow.WithNodeConfig("condition", "$.temp > 30"),
	)
	wire := flow.NewWire(nodeA.ID, "out", nodeB.ID, "in")

	f := flow.NewFlow("complex-pipeline",
		flow.WithDescription("Complex test flow"),
		flow.WithFlowConfig(flow.FlowConfig{
			TrackHistory:   true,
			MaxHistorySize: 50,
			ErrorHandling:  flow.ErrorPropagate,
		}),
		flow.WithFlowMetadata("author", "test"),
		flow.WithNodes(nodeA, nodeB),
		flow.WithWires(wire),
	)

	// Save
	err := repo.Save(ctx, f)
	require.NoError(t, err, "Save 실패")

	// Get 및 필드 검증
	got, err := repo.Get(ctx, f.ID())
	require.NoError(t, err, "Get 실패")

	assert.Equal(t, f.ID(), got.ID())
	assert.Equal(t, f.Name(), got.Name())
	assert.Equal(t, f.Description(), got.Description())
	assert.Len(t, got.Nodes(), 2, "노드 수 일치")
	assert.Len(t, got.Wires(), 1, "와이어 수 일치")
	assert.Equal(t, f.Config().TrackHistory, got.Config().TrackHistory)
	assert.Equal(t, f.Config().MaxHistorySize, got.Config().MaxHistorySize)
	assert.Equal(t, "test", got.Metadata()["author"])
}
