package system

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/tsdb"
	"github.com/xtra/xflow/pkg/lifecycle"
)

func TestTSDBAgent_NewAndInit(t *testing.T) {
	tsdb.ResetMemoryUsage()
	config := agent.AgentConfig{
		ID:   "tsdb-001",
		Name: "test-tsdb",
		Type: "tsdb",
	}

	a, err := NewTSDBAgent(config)
	require.NoError(t, err)
	require.NotNil(t, a)

	// NewTSDBAgent 팩토리에서 Init()이 호출되므로 바로 Running 상태
	assert.Equal(t, "tsdb-001", a.ID())
	assert.Equal(t, "test-tsdb", a.Name())
	assert.Equal(t, "tsdb", a.Type())
	assert.Equal(t, lifecycle.StateRunning, a.(*TSDBAgent).CurrentState())

	// TSDB 인스턴스가 생성되어야 한다
	tsdbAgent := a.(*TSDBAgent)
	require.NotNil(t, tsdbAgent.TSDB())
}

func TestTSDBAgent_TSDBProvider(t *testing.T) {
	tsdb.ResetMemoryUsage()
	config := agent.AgentConfig{
		ID:   "tsdb-002",
		Name: "provider-test",
		Type: "tsdb",
	}

	a, err := NewTSDBAgent(config)
	require.NoError(t, err)

	tsdbAgent := a.(*TSDBAgent)
	db := tsdbAgent.TSDB()
	require.NotNil(t, db)

	// TSDB에 데이터 기록
	err = db.Write("test_metric", map[string]string{"host": "server-01"},
		map[string]any{"value": float64(42.0)})
	require.NoError(t, err)

	// 데이터 조회 확인
	keys := db.SeriesKeys()
	assert.Len(t, keys, 1)
}

func TestTSDBAgent_CustomConfig(t *testing.T) {
	tsdb.ResetMemoryUsage()
	config := agent.AgentConfig{
		ID:   "tsdb-003",
		Name: "custom-config",
		Type: "tsdb",
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"max_series":            500,
				"max_points_per_series": 50000,
				"max_memory_mb":         128,
				"max_age":               "12h",
				"eviction_interval":     "1m",
				"max_query_points":      5000,
				"query_timeout":         "5s",
			},
		},
	}

	a, err := NewTSDBAgent(config)
	require.NoError(t, err)

	tsdbAgent := a.(*TSDBAgent)
	assert.Equal(t, 500, tsdbAgent.tsdbConfig.MaxSeries)
	assert.Equal(t, 50000, tsdbAgent.tsdbConfig.MaxPointsPerSeries)
	assert.Equal(t, int64(128*1024*1024), tsdbAgent.tsdbConfig.MaxMemoryBytes)
	assert.Equal(t, 12*time.Hour, tsdbAgent.tsdbConfig.MaxAge)
	assert.Equal(t, 1*time.Minute, tsdbAgent.tsdbConfig.EvictionInterval)
	assert.Equal(t, 5000, tsdbAgent.tsdbConfig.MaxQueryPoints)
	assert.Equal(t, 5*time.Second, tsdbAgent.tsdbConfig.QueryTimeout)
}

func TestTSDBAgent_Health(t *testing.T) {
	tsdb.ResetMemoryUsage()
	config := agent.AgentConfig{
		ID:   "tsdb-004",
		Name: "health-test",
		Type: "tsdb",
	}

	a, err := NewTSDBAgent(config)
	require.NoError(t, err)

	// 팩토리에서 Init()이 호출되므로 바로 Healthy
	health := a.Health()
	assert.Equal(t, agent.HealthHealthy, health.Status)
}

func TestTSDBAgent_StopClosesTSDB(t *testing.T) {
	tsdb.ResetMemoryUsage()
	config := agent.AgentConfig{
		ID:   "tsdb-005",
		Name: "stop-test",
		Type: "tsdb",
	}

	a, err := NewTSDBAgent(config)
	require.NoError(t, err)

	tsdbAgent := a.(*TSDBAgent)
	require.NotNil(t, tsdbAgent.TSDB())

	// Stop 후 TSDB가 nil이어야 한다
	err = a.Stop(context.Background())
	require.NoError(t, err)
	assert.Nil(t, tsdbAgent.TSDB())
}

func TestTSDBAgent_State(t *testing.T) {
	tsdb.ResetMemoryUsage()
	config := agent.AgentConfig{
		ID:   "tsdb-006",
		Name: "state-test",
		Type: "tsdb",
	}

	a, err := NewTSDBAgent(config)
	require.NoError(t, err)

	tsdbAgent := a.(*TSDBAgent)
	state := tsdbAgent.State()
	assert.Equal(t, "running", state["status"])
	assert.Equal(t, 0, state["series_count"])
	assert.Equal(t, int64(0), state["total_points"])

	// 데이터 기록 후 상태 확인
	db := tsdbAgent.TSDB()
	err = db.Write("field", nil, map[string]any{"val": float64(1)})
	require.NoError(t, err)

	state = tsdbAgent.State()
	assert.Equal(t, 1, state["series_count"])
	assert.Equal(t, int64(1), state["total_points"])
}

func TestTSDBAgent_Info(t *testing.T) {
	tsdb.ResetMemoryUsage()
	config := agent.AgentConfig{
		ID:   "tsdb-007",
		Name: "info-test",
		Type: "tsdb",
	}

	a, err := NewTSDBAgent(config)
	require.NoError(t, err)

	info := a.Info()
	assert.Equal(t, "tsdb-007", info.ID)
	assert.Equal(t, "info-test", info.Name)
	assert.Equal(t, "tsdb", info.Type)
	assert.Equal(t, lifecycle.StateRunning, info.State)
	assert.True(t, info.Uptime > 0)
}

func TestRegisterTSDBTypes(t *testing.T) {
	mgr := agent.NewManager()
	err := RegisterTSDBTypes(mgr)
	require.NoError(t, err)

	// tsdb 타입으로 에이전트 생성 가능해야 한다
	a, err := mgr.Create(agent.AgentConfig{
		ID:   "tsdb-reg-001",
		Name: "registered-tsdb",
		Type: "tsdb",
	})
	require.NoError(t, err)
	assert.Equal(t, "tsdb", a.Type())
}
