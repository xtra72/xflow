package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// testAgentConfig 는 테스트용 AgentConfig 를 생성하는 헬퍼이다.
func testAgentConfig(id, name, agentType string) agent.AgentConfig {
	return agent.AgentConfig{
		ID:   id,
		Name: name,
		Type: agentType,
		Transport: agent.TransportConfig{
			Type:    "tcp",
			Options: map[string]any{"host": "localhost", "port": 8080},
		},
		HealthCheckInterval: 30 * time.Second,
		MaxRestarts:         5,
		BufferSize:          2048,
		Metadata:            map[string]string{"env": "test"},
	}
}

func TestAgentFileRepository_CRUD(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewAgentFileRepository(dir)
	require.NoError(t, err)
	defer repo.Close()

	ctx := context.Background()
	cfg := testAgentConfig("agent-1", "Test Agent", "custom")

	// Create (Save)
	err = repo.Save(ctx, cfg)
	require.NoError(t, err)

	// Read (Get)
	got, err := repo.Get(ctx, "agent-1")
	require.NoError(t, err)
	assert.Equal(t, cfg.ID, got.ID)
	assert.Equal(t, cfg.Name, got.Name)
	assert.Equal(t, cfg.Type, got.Type)
	assert.Equal(t, cfg.Transport.Type, got.Transport.Type)
	assert.Equal(t, cfg.HealthCheckInterval, got.HealthCheckInterval)
	assert.Equal(t, cfg.MaxRestarts, got.MaxRestarts)
	assert.Equal(t, cfg.BufferSize, got.BufferSize)
	assert.Equal(t, cfg.Metadata, got.Metadata)

	// Update (Save 로 덮어쓰기)
	cfg.Name = "Updated Agent"
	cfg.MaxRestarts = 10
	err = repo.Save(ctx, cfg)
	require.NoError(t, err)

	got, err = repo.Get(ctx, "agent-1")
	require.NoError(t, err)
	assert.Equal(t, "Updated Agent", got.Name)
	assert.Equal(t, 10, got.MaxRestarts)

	// Delete
	err = repo.Delete(ctx, "agent-1")
	require.NoError(t, err)

	// 삭제 후 Get 은 ErrAgentNotFound 반환
	_, err = repo.Get(ctx, "agent-1")
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestAgentFileRepository_Get_NotFound(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewAgentFileRepository(dir)
	require.NoError(t, err)
	defer repo.Close()

	ctx := context.Background()

	_, err = repo.Get(ctx, "non-existent-id")
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestAgentFileRepository_Delete_NotFound(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewAgentFileRepository(dir)
	require.NoError(t, err)
	defer repo.Close()

	ctx := context.Background()

	err = repo.Delete(ctx, "non-existent-id")
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestAgentFileRepository_List_Empty(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewAgentFileRepository(dir)
	require.NoError(t, err)
	defer repo.Close()

	ctx := context.Background()

	configs, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, configs)
}

func TestAgentFileRepository_Save_Overwrite(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewAgentFileRepository(dir)
	require.NoError(t, err)
	defer repo.Close()

	ctx := context.Background()

	// 첫 번째 저장
	cfg := testAgentConfig("overwrite-agent", "Original", "custom")
	err = repo.Save(ctx, cfg)
	require.NoError(t, err)

	// 동일 ID 로 두 번째 저장
	cfg.Name = "Overwritten"
	cfg.BufferSize = 4096
	err = repo.Save(ctx, cfg)
	require.NoError(t, err)

	// 마지막으로 저장된 값 확인
	got, err := repo.Get(ctx, "overwrite-agent")
	require.NoError(t, err)
	assert.Equal(t, "Overwritten", got.Name)
	assert.Equal(t, 4096, got.BufferSize)
}

func TestAgentFileRepository_List_Multiple(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewAgentFileRepository(dir)
	require.NoError(t, err)
	defer repo.Close()

	ctx := context.Background()

	// 3개 에이전트 저장
	agents := []agent.AgentConfig{
		testAgentConfig("agent-a", "Agent A", "tcp"),
		testAgentConfig("agent-b", "Agent B", "serial"),
		testAgentConfig("agent-c", "Agent C", "mqtt-client"),
	}
	for _, cfg := range agents {
		err := repo.Save(ctx, cfg)
		require.NoError(t, err)
	}

	// List 호출
	configs, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, configs, 3)

	// 모든 에이전트의 ID 가 포함되어 있는지 확인
	ids := make(map[string]bool)
	for _, cfg := range configs {
		ids[cfg.ID] = true
	}
	for _, expected := range []string{"agent-a", "agent-b", "agent-c"} {
		assert.True(t, ids[expected], "List 결과에 %q 가 포함되지 않음", expected)
	}
}
