package agent

import (
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTypeRegistry(t *testing.T) {
	tr := NewTypeRegistry()
	require.NotNil(t, tr)
	assert.Empty(t, tr.ListTypes())
}

func TestTypeRegistry_RegisterType(t *testing.T) {
	tr := NewTypeRegistry()

	factory := func(config AgentConfig) (Agent, error) {
		ba := NewBaseAgent()
		if err := ba.Init(config); err != nil {
			return nil, err
		}
		return ba, nil
	}

	err := tr.RegisterType("custom", factory)
	require.NoError(t, err)
	assert.True(t, tr.HasType("custom"))
}

func TestTypeRegistry_RegisterType_Duplicate(t *testing.T) {
	tr := NewTypeRegistry()

	factory := func(config AgentConfig) (Agent, error) {
		return NewBaseAgent(), nil
	}

	err := tr.RegisterType("custom", factory)
	require.NoError(t, err)

	// 중복 등록은 에러를 반환해야 한다.
	err = tr.RegisterType("custom", factory)
	assert.Error(t, err)
}

func TestTypeRegistry_CreateAgent(t *testing.T) {
	tr := NewTypeRegistry()

	factory := func(config AgentConfig) (Agent, error) {
		ba := NewBaseAgent()
		if err := ba.Init(config); err != nil {
			return nil, err
		}
		return ba, nil
	}

	require.NoError(t, tr.RegisterType("custom", factory))

	cfg := AgentConfig{ID: "a1", Name: "Agent 1", Type: "custom"}
	agent, err := tr.CreateAgent("custom", cfg)
	require.NoError(t, err)
	require.NotNil(t, agent)
	assert.Equal(t, "a1", agent.ID())
}

func TestTypeRegistry_CreateAgent_UnregisteredType(t *testing.T) {
	tr := NewTypeRegistry()

	cfg := AgentConfig{ID: "a1", Name: "Agent 1", Type: "unknown"}
	agent, err := tr.CreateAgent("unknown", cfg)
	assert.Error(t, err)
	assert.Nil(t, agent)
}

func TestTypeRegistry_ListTypes(t *testing.T) {
	tr := NewTypeRegistry()

	factory := func(config AgentConfig) (Agent, error) {
		return NewBaseAgent(), nil
	}

	require.NoError(t, tr.RegisterType("custom", factory))
	require.NoError(t, tr.RegisterType("mqtt-client", factory))
	require.NoError(t, tr.RegisterType("http", factory))

	types := tr.ListTypes()
	sort.Strings(types)
	assert.Equal(t, []string{"custom", "http", "mqtt-client"}, types)
}

func TestTypeRegistry_HasType(t *testing.T) {
	tr := NewTypeRegistry()

	assert.False(t, tr.HasType("custom"))

	factory := func(config AgentConfig) (Agent, error) {
		return NewBaseAgent(), nil
	}
	require.NoError(t, tr.RegisterType("custom", factory))

	assert.True(t, tr.HasType("custom"))
	assert.False(t, tr.HasType("unknown"))
}

func TestTypeRegistry_ConcurrentAccess(t *testing.T) {
	tr := NewTypeRegistry()
	var wg sync.WaitGroup

	// 동시에 여러 타입을 등록한다.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			factory := func(config AgentConfig) (Agent, error) {
				return NewBaseAgent(), nil
			}
			// 에러는 중복이 발생할 수 있으므로 무시한다.
			_ = tr.RegisterType("type-concurrent", factory)
		}(i)
	}

	wg.Wait()

	// 적어도 하나의 등록은 성공해야 한다.
	assert.True(t, tr.HasType("type-concurrent"))
}

func TestDefaultRegistry_PackageLevel(t *testing.T) {
	// 패키지 수준의 DefaultTypeReg가 존재하는지 확인한다.
	require.NotNil(t, DefaultTypeReg)
}
