package node

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/pkg/flow"
)

func TestInfluxDBReadNode_Configure(t *testing.T) {
	def := flow.NodeDef{ID: "ir1", Type: "influxdb-read"}
	n, err := NewInfluxDBReadNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"query":         "from(bucket:\"test\")",
		"language":      "influxql",
		"poll_interval": "1m",
		"timeout":       "5s",
	})
	require.NoError(t, err)

	ir := n.(*InfluxDBReadNode)
	assert.Equal(t, "from(bucket:\"test\")", ir.query)
	assert.Equal(t, "influxql", ir.language)
	assert.Equal(t, 1*time.Minute, ir.pollInterval)
	assert.Equal(t, 5*time.Second, ir.timeout)
}

func TestInfluxDBReadNode_Configure_Defaults(t *testing.T) {
	def := flow.NodeDef{ID: "ir2", Type: "influxdb-read"}
	n, err := NewInfluxDBReadNode(def)
	require.NoError(t, err)

	ir := n.(*InfluxDBReadNode)
	assert.Equal(t, "flux", ir.language)
	assert.Equal(t, 30*time.Second, ir.pollInterval)
	assert.Equal(t, 10*time.Second, ir.timeout)
}

func TestInfluxDBReadNode_Configure_InvalidDuration(t *testing.T) {
	def := flow.NodeDef{ID: "ir3", Type: "influxdb-read"}
	n, err := NewInfluxDBReadNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"poll_interval": "invalid",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid poll_interval")
}

func TestInfluxDBReadNode_SourceCh(t *testing.T) {
	// 쿼리 결과를 JSON 배열로 반환하는 모의 에이전트
	rows := []map[string]any{
		{"time": "2024-01-01T00:00:00Z", "value": 25.0},
		{"time": "2024-01-01T00:01:00Z", "value": 26.0},
	}
	rowsJSON, err := json.Marshal(rows)
	require.NoError(t, err)

	mock := &mockInfluxDBReceiver{
		mockInfluxDBAgent: mockInfluxDBAgent{
			processFunc: func(data []byte) ([]byte, error) {
				return nil, nil
			},
		},
		receiveFunc: func(ctx context.Context) ([]byte, error) {
			return rowsJSON, nil
		},
	}

	def := flow.NodeDef{ID: "ir4", Type: "influxdb-read"}
	n, err := NewInfluxDBReadNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"_influxdb_agent": mock,
		"query":           "from(bucket:\"test\")",
		"poll_interval":   "100ms",
		"timeout":         "1s",
	})
	require.NoError(t, err)

	require.NoError(t, n.Init(context.Background()))

	ir := n.(*InfluxDBReadNode)
	ch := ir.SourceCh()

	// 첫 번째 폴링에서 2개의 메시지가 도착해야 한다
	var received int
	timeout := time.After(2 * time.Second)
	for received < 2 {
		select {
		case msg := <-ch:
			v, ok := msg.Payload().Get("value")
			assert.True(t, ok)
			assert.NotNil(t, v)
			received++
		case <-timeout:
			t.Fatal("타임아웃: SourceCh에서 메시지를 받지 못함")
		}
	}

	assert.Equal(t, 2, received)

	// 종료
	require.NoError(t, n.Shutdown(context.Background()))
}

func TestInfluxDBReadNode_EmptyResults(t *testing.T) {
	// 빈 배열을 반환하는 모의 에이전트
	emptyJSON, _ := json.Marshal([]map[string]any{})

	mock := &mockInfluxDBReceiver{
		mockInfluxDBAgent: mockInfluxDBAgent{
			processFunc: func(data []byte) ([]byte, error) {
				return nil, nil
			},
		},
		receiveFunc: func(ctx context.Context) ([]byte, error) {
			return emptyJSON, nil
		},
	}

	def := flow.NodeDef{ID: "ir5", Type: "influxdb-read"}
	n, err := NewInfluxDBReadNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"_influxdb_agent": mock,
		"query":           "from(bucket:\"empty\")",
		"poll_interval":   "100ms",
		"timeout":         "1s",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	ir := n.(*InfluxDBReadNode)
	ch := ir.SourceCh()

	// 빈 결과이므로 메시지가 도착하지 않아야 한다
	select {
	case <-ch:
		t.Fatal("빈 결과에서 메시지가 도착함")
	case <-time.After(300 * time.Millisecond):
		// 예상대로 메시지 없음
	}

	require.NoError(t, n.Shutdown(context.Background()))
}

func TestInfluxDBReadNode_NoAgent(t *testing.T) {
	def := flow.NodeDef{ID: "ir6", Type: "influxdb-read"}
	n, err := NewInfluxDBReadNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"query": "from(bucket:\"test\")",
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent_ref is required")
}
