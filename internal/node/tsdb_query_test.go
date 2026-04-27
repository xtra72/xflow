package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

func TestTSDBQueryNode_Configure(t *testing.T) {
	def := flow.NodeDef{ID: "tq1", Type: "tsdb-query"}
	n, err := NewTSDBQueryNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"measurement": "cpu",
		"tags": map[string]any{
			"host": "server-01",
		},
		"time_range":  "1h",
		"aggregation": "avg",
		"field":       "usage",
		"bucket":      "5m",
		"limit":       100,
	})
	require.NoError(t, err)

	tq := n.(*TSDBQueryNode)
	assert.Equal(t, "cpu", tq.measurement)
	assert.Equal(t, map[string]string{"host": "server-01"}, tq.tagFilters)
	assert.Equal(t, time.Hour, tq.timeRange)
	assert.Equal(t, "avg", tq.aggregation)
	assert.Equal(t, "usage", tq.field)
	assert.Equal(t, 5*time.Minute, tq.bucketInterval)
	assert.Equal(t, 100, tq.limit)
}

func TestTSDBQueryNode_Configure_InvalidTimeRange(t *testing.T) {
	def := flow.NodeDef{ID: "tq2", Type: "tsdb-query"}
	n, err := NewTSDBQueryNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"time_range": "invalid",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid time_range")
}

func TestTSDBQueryNode_Configure_InvalidBucket(t *testing.T) {
	def := flow.NodeDef{ID: "tq3", Type: "tsdb-query"}
	n, err := NewTSDBQueryNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"bucket": "bad",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid bucket")
}

func TestTSDBQueryNode_Process_RawQuery(t *testing.T) {
	def := flow.NodeDef{ID: "tq4", Type: "tsdb-query"}
	n, err := NewTSDBQueryNode(def)
	require.NoError(t, err)

	db := newTestTSDBInstance(t)

	// 테스트 데이터 기록
	for i := range 5 {
		err := db.Write("temperature", map[string]string{"room": "office"},
			map[string]any{"value": float64(20 + i)})
		require.NoError(t, err)
		time.Sleep(1 * time.Millisecond) // 타임스탬프 분리를 위한 대기
	}

	err = n.Configure(map[string]any{
		"_tsdb":       db,
		"measurement": "temperature",
		"tags": map[string]any{
			"room": "office",
		},
		"time_range": "1h",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	// 트리거 메시지 (쿼리 노드는 메시지 내용을 무시하고 설정 기반으로 조회)
	msg := message.New()

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 결과 payload 확인
	resultPayload := results[0].Payload()

	queryInfo, ok := resultPayload.Get("query")
	assert.True(t, ok)
	queryMap, ok := queryInfo.(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "temperature", queryMap["measurement"])

	queryResults, ok := resultPayload.Get("results")
	assert.True(t, ok)
	resultsList, ok := queryResults.([]map[string]any)
	assert.True(t, ok)
	assert.Greater(t, len(resultsList), 0)

	// 첫 번째 결과의 포인트 확인
	firstResult := resultsList[0]
	points, ok := firstResult["points"].([]map[string]any)
	assert.True(t, ok)
	assert.Equal(t, 5, len(points))
}

func TestTSDBQueryNode_Process_Aggregation(t *testing.T) {
	def := flow.NodeDef{ID: "tq5", Type: "tsdb-query"}
	n, err := NewTSDBQueryNode(def)
	require.NoError(t, err)

	db := newTestTSDBInstance(t)

	// 테스트 데이터 기록: 10, 20, 30 -> avg = 20
	values := []float64{10.0, 20.0, 30.0}
	for _, v := range values {
		err := db.Write("metric", nil, map[string]any{"value": v})
		require.NoError(t, err)
		time.Sleep(1 * time.Millisecond)
	}

	err = n.Configure(map[string]any{
		"_tsdb":       db,
		"measurement": "metric",
		"time_range":  "1h",
		"aggregation": "avg",
		"field":       "value",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	msg := message.New()
	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 결과에 쿼리 정보가 포함되어 있는지 확인
	resultPayload := results[0].Payload()
	queryInfo, ok := resultPayload.Get("query")
	assert.True(t, ok)
	queryMap, ok := queryInfo.(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "avg", queryMap["aggregation"])
	assert.Equal(t, "value", queryMap["field"])

	// 결과가 존재하는지 확인
	queryResults, ok := resultPayload.Get("results")
	assert.True(t, ok)
	resultsList, ok := queryResults.([]map[string]any)
	assert.True(t, ok)
	assert.Greater(t, len(resultsList), 0)
}

func TestTSDBQueryNode_Process_NoTSDB(t *testing.T) {
	def := flow.NodeDef{ID: "tq6", Type: "tsdb-query"}
	n, err := NewTSDBQueryNode(def)
	require.NoError(t, err)

	// TSDB 미주입
	err = n.Configure(map[string]any{
		"measurement": "test",
	})
	require.NoError(t, err)
	// Init에서 _tsdb도 없고 agent_ref도 없으므로 에러
	err = n.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent_ref is required")
}
