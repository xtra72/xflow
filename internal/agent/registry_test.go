package agent

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistry(t *testing.T) {
	reg := NewRegistry()
	require.NotNil(t, reg)
	assert.Equal(t, 0, reg.Count())
	assert.Empty(t, reg.List())
}

func TestRegistry_Register(t *testing.T) {
	reg := NewRegistry()
	a := createTestAgent(t, "a1", "Agent 1", "custom")

	err := reg.Register(a)
	require.NoError(t, err)
	assert.Equal(t, 1, reg.Count())
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	reg := NewRegistry()
	a1 := createTestAgent(t, "a1", "Agent 1", "custom")
	a2 := createTestAgent(t, "a1", "Agent 1 Copy", "custom")

	require.NoError(t, reg.Register(a1))

	err := reg.Register(a2)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentAlreadyExists)
	assert.Equal(t, 1, reg.Count())
}

func TestRegistry_Unregister(t *testing.T) {
	reg := NewRegistry()
	a := createTestAgent(t, "a1", "Agent 1", "custom")

	require.NoError(t, reg.Register(a))
	assert.Equal(t, 1, reg.Count())

	err := reg.Unregister("a1")
	require.NoError(t, err)
	assert.Equal(t, 0, reg.Count())
}

func TestRegistry_Unregister_NotFound(t *testing.T) {
	reg := NewRegistry()

	err := reg.Unregister("nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestRegistry_Get(t *testing.T) {
	reg := NewRegistry()
	a := createTestAgent(t, "a1", "Agent 1", "custom")

	require.NoError(t, reg.Register(a))

	found, ok := reg.Get("a1")
	assert.True(t, ok)
	assert.Equal(t, "a1", found.ID())
}

func TestRegistry_Get_NotFound(t *testing.T) {
	reg := NewRegistry()

	found, ok := reg.Get("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, found)
}

func TestRegistry_GetByName(t *testing.T) {
	reg := NewRegistry()
	a := createTestAgent(t, "a1", "Agent 1", "custom")

	require.NoError(t, reg.Register(a))

	found, ok := reg.GetByName("Agent 1")
	assert.True(t, ok)
	assert.Equal(t, "a1", found.ID())
}

func TestRegistry_GetByName_NotFound(t *testing.T) {
	reg := NewRegistry()

	found, ok := reg.GetByName("Nonexistent")
	assert.False(t, ok)
	assert.Nil(t, found)
}

func TestRegistry_GetByType(t *testing.T) {
	reg := NewRegistry()
	a1 := createTestAgent(t, "a1", "Agent 1", "custom")
	a2 := createTestAgent(t, "a2", "Agent 2", "custom")
	a3 := createTestAgent(t, "a3", "Agent 3", "mqtt")

	require.NoError(t, reg.Register(a1))
	require.NoError(t, reg.Register(a2))
	require.NoError(t, reg.Register(a3))

	customs := reg.GetByType("custom")
	assert.Len(t, customs, 2)

	mqtts := reg.GetByType("mqtt")
	assert.Len(t, mqtts, 1)

	unknowns := reg.GetByType("unknown")
	assert.Empty(t, unknowns)
}

func TestRegistry_List(t *testing.T) {
	reg := NewRegistry()
	a1 := createTestAgent(t, "a1", "Agent 1", "custom")
	a2 := createTestAgent(t, "a2", "Agent 2", "mqtt")

	require.NoError(t, reg.Register(a1))
	require.NoError(t, reg.Register(a2))

	list := reg.List()
	assert.Len(t, list, 2)
}

func TestRegistry_Count(t *testing.T) {
	reg := NewRegistry()
	assert.Equal(t, 0, reg.Count())

	a1 := createTestAgent(t, "a1", "Agent 1", "custom")
	require.NoError(t, reg.Register(a1))
	assert.Equal(t, 1, reg.Count())

	a2 := createTestAgent(t, "a2", "Agent 2", "custom")
	require.NoError(t, reg.Register(a2))
	assert.Equal(t, 2, reg.Count())

	require.NoError(t, reg.Unregister("a1"))
	assert.Equal(t, 1, reg.Count())
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	reg := NewRegistry()
	var wg sync.WaitGroup
	iterations := 100

	// 동시에 에이전트를 등록한다.
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			a := createTestAgent(t, "concurrent-agent", "Concurrent", "custom")
			_ = reg.Register(a) // 중복 에러 무시
		}(i)
	}

	// 동시에 조회한다.
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = reg.Count()
			_ = reg.List()
			reg.Get("concurrent-agent")
		}()
	}

	wg.Wait()

	// 하나만 등록되었는지 확인한다.
	assert.Equal(t, 1, reg.Count())
}

// --- Helper ---

func createTestAgent(t *testing.T, id, name, agentType string) Agent {
	t.Helper()
	ba := NewBaseAgent()
	cfg := AgentConfig{ID: id, Name: name, Type: agentType}
	require.NoError(t, ba.Init(cfg))
	return ba
}
