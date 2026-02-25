package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// testAgentConfig 는 agent_file_test.go 에 정의된 공용 헬퍼를 사용한다.

// setupAgentSQLiteRepo 는 테스트용 AgentSQLiteRepository 를 생성한다.
// 테스트 종료 시 자동으로 Close 된다.
func setupAgentSQLiteRepo(t *testing.T) *AgentSQLiteRepository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	ctx := context.Background()

	repo, err := NewAgentSQLiteRepository(ctx, dbPath)
	require.NoError(t, err, "NewAgentSQLiteRepository 실패")

	t.Cleanup(func() {
		repo.Close()
	})

	return repo
}

// ---------------------------------------------------------------------------
// CRUD 전체 사이클
// ---------------------------------------------------------------------------

func TestAgentSQLiteRepository_CRUD(t *testing.T) {
	repo := setupAgentSQLiteRepo(t)
	ctx := context.Background()

	// 1. Save
	cfg := testAgentConfig("agent-1", "테스트 에이전트", "custom")
	err := repo.Save(ctx, cfg)
	require.NoError(t, err, "Save 실패")

	// 2. Get
	got, err := repo.Get(ctx, "agent-1")
	require.NoError(t, err, "Get 실패")
	assert.Equal(t, cfg.ID, got.ID, "ID 일치")
	assert.Equal(t, cfg.Name, got.Name, "Name 일치")
	assert.Equal(t, cfg.Type, got.Type, "Type 일치")
	assert.Equal(t, cfg.Transport.Type, got.Transport.Type, "Transport.Type 일치")
	assert.Equal(t, cfg.HealthCheckInterval, got.HealthCheckInterval, "HealthCheckInterval 일치")
	assert.Equal(t, cfg.MaxRestarts, got.MaxRestarts, "MaxRestarts 일치")
	assert.Equal(t, cfg.BufferSize, got.BufferSize, "BufferSize 일치")
	assert.Equal(t, cfg.Metadata["env"], got.Metadata["env"], "Metadata 일치")

	// 3. List
	list, err := repo.List(ctx)
	require.NoError(t, err, "List 실패")
	assert.Len(t, list, 1, "List 는 1 개")
	assert.Equal(t, cfg.ID, list[0].ID, "List[0].ID 일치")

	// 4. Delete
	err = repo.Delete(ctx, "agent-1")
	require.NoError(t, err, "Delete 실패")

	// 5. Delete 후 Get 하면 ErrAgentNotFound
	_, err = repo.Get(ctx, "agent-1")
	assert.ErrorIs(t, err, ErrAgentNotFound, "삭제 후 Get 은 ErrAgentNotFound")

	// 6. Delete 후 List 는 빈 슬라이스
	list, err = repo.List(ctx)
	require.NoError(t, err, "List 실패")
	assert.Empty(t, list, "삭제 후 List 는 비어야 한다")
}

// ---------------------------------------------------------------------------
// Get: 존재하지 않는 ID
// ---------------------------------------------------------------------------

func TestAgentSQLiteRepository_Get_NotFound(t *testing.T) {
	repo := setupAgentSQLiteRepo(t)
	ctx := context.Background()

	_, err := repo.Get(ctx, "non-existent-id")
	assert.ErrorIs(t, err, ErrAgentNotFound, "존재하지 않는 ID 는 ErrAgentNotFound")
}

// ---------------------------------------------------------------------------
// Delete: 존재하지 않는 ID
// ---------------------------------------------------------------------------

func TestAgentSQLiteRepository_Delete_NotFound(t *testing.T) {
	repo := setupAgentSQLiteRepo(t)
	ctx := context.Background()

	err := repo.Delete(ctx, "non-existent-id")
	assert.ErrorIs(t, err, ErrAgentNotFound, "존재하지 않는 ID 삭제는 ErrAgentNotFound")
}

// ---------------------------------------------------------------------------
// List: 빈 저장소
// ---------------------------------------------------------------------------

func TestAgentSQLiteRepository_List_Empty(t *testing.T) {
	repo := setupAgentSQLiteRepo(t)
	ctx := context.Background()

	list, err := repo.List(ctx)
	require.NoError(t, err, "List 실패")
	assert.Empty(t, list, "빈 저장소의 List 는 비어야 한다")
}

// ---------------------------------------------------------------------------
// Save: 동일 ID 로 두 번 저장 → 덮어쓰기 (Upsert)
// ---------------------------------------------------------------------------

func TestAgentSQLiteRepository_Save_Overwrite(t *testing.T) {
	repo := setupAgentSQLiteRepo(t)
	ctx := context.Background()

	// 첫 번째 저장
	cfg1 := testAgentConfig("agent-overwrite", "원본 이름", "custom")
	err := repo.Save(ctx, cfg1)
	require.NoError(t, err, "첫 번째 Save 실패")

	// 같은 ID 로 다른 내용 저장
	cfg2 := testAgentConfig("agent-overwrite", "갱신된 이름", "mqtt")
	cfg2.BufferSize = 4096
	cfg2.Metadata = map[string]string{"env": "prod"}
	err = repo.Save(ctx, cfg2)
	require.NoError(t, err, "두 번째 Save 실패")

	// 조회하면 갱신된 내용이 반환
	got, err := repo.Get(ctx, "agent-overwrite")
	require.NoError(t, err, "Get 실패")
	assert.Equal(t, "갱신된 이름", got.Name, "Name 이 갱신되어야 한다")
	assert.Equal(t, "mqtt", got.Type, "Type 이 갱신되어야 한다")
	assert.Equal(t, 4096, got.BufferSize, "BufferSize 가 갱신되어야 한다")
	assert.Equal(t, "prod", got.Metadata["env"], "Metadata 가 갱신되어야 한다")

	// List 에도 1 개만 존재
	list, err := repo.List(ctx)
	require.NoError(t, err, "List 실패")
	assert.Len(t, list, 1, "덮어쓰기 후 List 는 1 개")
}

// ---------------------------------------------------------------------------
// List: 여러 에이전트 저장 후 조회
// ---------------------------------------------------------------------------

func TestAgentSQLiteRepository_List_Multiple(t *testing.T) {
	repo := setupAgentSQLiteRepo(t)
	ctx := context.Background()

	configs := []agent.AgentConfig{
		testAgentConfig("agent-a", "에이전트 A", "custom"),
		testAgentConfig("agent-b", "에이전트 B", "mqtt"),
		testAgentConfig("agent-c", "에이전트 C", "system"),
	}

	for _, cfg := range configs {
		err := repo.Save(ctx, cfg)
		require.NoError(t, err, "Save 실패: "+cfg.ID)
	}

	list, err := repo.List(ctx)
	require.NoError(t, err, "List 실패")
	assert.Len(t, list, 3, "List 는 3 개")

	// ID 로 모든 에이전트가 존재하는지 확인
	ids := make(map[string]bool)
	for _, cfg := range list {
		ids[cfg.ID] = true
	}
	assert.True(t, ids["agent-a"], "agent-a 존재")
	assert.True(t, ids["agent-b"], "agent-b 존재")
	assert.True(t, ids["agent-c"], "agent-c 존재")
}
