package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/tsdb"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// newTestTSDBInstance 는 테스트용 TSDB 인스턴스를 생성한다.
func newTestTSDBInstance(t *testing.T) tsdb.TSDB {
	t.Helper()
	tsdb.ResetMemoryUsage()
	cfg := tsdb.DefaultConfig()
	cfg.EvictionInterval = 1 * time.Hour // 테스트 중 자동 퇴거 방지
	db := tsdb.New(cfg)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestTSDBWriteNode_Configure(t *testing.T) {
	def := flow.NodeDef{ID: "tw1", Type: "tsdb-write"}
	n, err := NewTSDBWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"measurement": "temperature",
		"tag_mappings": map[string]any{
			"host":   "hostname",
			"region": "location",
		},
		"field_mappings": map[string]any{
			"value": "temp_value",
			"unit":  "temp_unit",
		},
	})
	require.NoError(t, err)

	tw := n.(*TSDBWriteNode)
	assert.Equal(t, "temperature", tw.measurement)
	assert.Equal(t, map[string]string{"host": "hostname", "region": "location"}, tw.tagMappings)
	assert.Equal(t, map[string]string{"value": "temp_value", "unit": "temp_unit"}, tw.fieldMappings)
}

func TestTSDBWriteNode_Process_FixedMeasurement(t *testing.T) {
	def := flow.NodeDef{ID: "tw2", Type: "tsdb-write"}
	n, err := NewTSDBWriteNode(def)
	require.NoError(t, err)

	db := newTestTSDBInstance(t)
	err = n.Configure(map[string]any{
		"_tsdb":       db,
		"measurement": "cpu",
		"tag_mappings": map[string]any{
			"host": "hostname",
		},
		"field_mappings": map[string]any{
			"usage": "cpu_usage",
		},
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	// 입력 메시지 생성
	payload := message.NewPayload(map[string]any{
		"hostname":  "server-01",
		"cpu_usage": 75.5,
	})
	msg := message.New(message.WithPayload(payload))

	// Process 실행
	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// TSDB에 데이터가 기록되었는지 확인
	keys := db.FilterSeries("cpu", map[string]string{"host": "server-01"})
	assert.Len(t, keys, 1)
}

func TestTSDBWriteNode_Process_DynamicMeasurement(t *testing.T) {
	def := flow.NodeDef{ID: "tw3", Type: "tsdb-write"}
	n, err := NewTSDBWriteNode(def)
	require.NoError(t, err)

	db := newTestTSDBInstance(t)
	err = n.Configure(map[string]any{
		"_tsdb":           db,
		"measurement_key": "metric_name",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	// payload에 measurement 이름 포함
	payload := message.NewPayload(map[string]any{
		"metric_name": "memory",
		"usage":       float64(82.3),
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// TSDB에 "memory" measurement로 기록되었는지 확인
	keys := db.FilterSeries("memory", nil)
	assert.Len(t, keys, 1)
}

func TestTSDBWriteNode_Process_FieldMappings(t *testing.T) {
	def := flow.NodeDef{ID: "tw4", Type: "tsdb-write"}
	n, err := NewTSDBWriteNode(def)
	require.NoError(t, err)

	db := newTestTSDBInstance(t)
	err = n.Configure(map[string]any{
		"_tsdb":       db,
		"measurement": "sensor",
		"field_mappings": map[string]any{
			"temp": "temperature",
			"hum":  "humidity",
		},
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"temperature": float64(25.0),
		"humidity":    float64(60.0),
		"extra_field": "이 필드는 무시됨",
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 기록된 데이터 확인 - sensor measurement로 조회
	keys := db.FilterSeries("sensor", nil)
	require.Len(t, keys, 1)

	// latest 포인트 조회
	points, err := db.Latest(keys[0], 1)
	require.NoError(t, err)
	require.Len(t, points, 1)
	assert.Equal(t, float64(25.0), points[0].Fields["temp"])
	assert.Equal(t, float64(60.0), points[0].Fields["hum"])
	// extra_field는 기록되지 않아야 함
	_, hasExtra := points[0].Fields["extra_field"]
	assert.False(t, hasExtra)
}

func TestTSDBWriteNode_Process_AllPayload(t *testing.T) {
	def := flow.NodeDef{ID: "tw5", Type: "tsdb-write"}
	n, err := NewTSDBWriteNode(def)
	require.NoError(t, err)

	db := newTestTSDBInstance(t)
	// field_mappings를 설정하지 않으면 전체 payload가 fields로 사용됨
	err = n.Configure(map[string]any{
		"_tsdb":       db,
		"measurement": "raw_data",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"value1": float64(100),
		"value2": float64(200),
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 기록 확인
	keys := db.FilterSeries("raw_data", nil)
	require.Len(t, keys, 1)

	points, err := db.Latest(keys[0], 1)
	require.NoError(t, err)
	require.Len(t, points, 1)
	assert.Equal(t, float64(100), points[0].Fields["value1"])
	assert.Equal(t, float64(200), points[0].Fields["value2"])
}

func TestTSDBWriteNode_Process_PassThrough(t *testing.T) {
	def := flow.NodeDef{ID: "tw6", Type: "tsdb-write"}
	n, err := NewTSDBWriteNode(def)
	require.NoError(t, err)

	db := newTestTSDBInstance(t)
	err = n.Configure(map[string]any{
		"_tsdb":       db,
		"measurement": "test",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"data": float64(42),
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 반환된 메시지가 원본 메시지와 동일한지 확인 (pass-through)
	assert.Equal(t, msg.ID(), results[0].ID())
	v, ok := results[0].Payload().Get("data")
	assert.True(t, ok)
	assert.Equal(t, float64(42), v)
}

func TestTSDBWriteNode_Process_NoTSDB(t *testing.T) {
	def := flow.NodeDef{ID: "tw7", Type: "tsdb-write"}
	n, err := NewTSDBWriteNode(def)
	require.NoError(t, err)

	// TSDB를 주입하지 않고 Configure
	err = n.Configure(map[string]any{
		"measurement": "test",
	})
	require.NoError(t, err)
	// Init에서 _tsdb도 없고 agent_ref도 없으므로 에러
	err = n.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent_ref is required")
}
