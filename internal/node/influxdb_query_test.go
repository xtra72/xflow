package node

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

func TestInfluxDBQueryNode_Configure(t *testing.T) {
	def := flow.NodeDef{ID: "iq1", Type: "influxdb-query"}
	n, err := NewInfluxDBQueryNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"query":      "from(bucket:\"test\")",
		"language":   "influxql",
		"timeout":    "5s",
		"result_key": "data",
	})
	require.NoError(t, err)

	iq := n.(*InfluxDBQueryNode)
	assert.Equal(t, "from(bucket:\"test\")", iq.query)
	assert.Equal(t, "influxql", iq.language)
	assert.Equal(t, "data", iq.resultKey)
}

func TestInfluxDBQueryNode_Configure_Defaults(t *testing.T) {
	def := flow.NodeDef{ID: "iq2", Type: "influxdb-query"}
	n, err := NewInfluxDBQueryNode(def)
	require.NoError(t, err)

	iq := n.(*InfluxDBQueryNode)
	assert.Equal(t, "flux", iq.language)
	assert.Equal(t, "results", iq.resultKey)
}

func TestInfluxDBQueryNode_Process_ConfigQuery(t *testing.T) {
	rows := []map[string]any{
		{"time": "2024-01-01T00:00:00Z", "value": 25.0},
	}
	rowsJSON, _ := json.Marshal(rows)

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

	def := flow.NodeDef{ID: "iq3", Type: "influxdb-query"}
	n, err := NewInfluxDBQueryNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"_influxdb_agent": mock,
		"query":           "from(bucket:\"test\")",
		"timeout":         "5s",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	msg := message.New()
	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 결과 확인
	v, ok := results[0].Payload().Get("results")
	require.True(t, ok)
	// JSON unmarshal 결과는 []map[string]any 타입이다
	resultList, ok := v.([]map[string]any)
	require.True(t, ok)
	assert.Len(t, resultList, 1)
}

func TestInfluxDBQueryNode_Process_PayloadQuery(t *testing.T) {
	rows := []map[string]any{
		{"host": "server-01", "cpu": 75.5},
	}
	rowsJSON, _ := json.Marshal(rows)

	var capturedQuery string
	mock := &mockInfluxDBReceiver{
		mockInfluxDBAgent: mockInfluxDBAgent{
			processFunc: func(data []byte) ([]byte, error) {
				var qr struct {
					Query string `json:"query"`
				}
				_ = json.Unmarshal(data, &qr)
				capturedQuery = qr.Query
				return nil, nil
			},
		},
		receiveFunc: func(ctx context.Context) ([]byte, error) {
			return rowsJSON, nil
		},
	}

	def := flow.NodeDef{ID: "iq4", Type: "influxdb-query"}
	n, err := NewInfluxDBQueryNode(def)
	require.NoError(t, err)

	// query를 설정하지 않음 → payload에서 추출
	err = n.Configure(map[string]any{
		"_influxdb_agent": mock,
		"timeout":         "5s",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"query": "SELECT * FROM cpu WHERE host = 'server-01'",
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "SELECT * FROM cpu WHERE host = 'server-01'", capturedQuery)
}

func TestInfluxDBQueryNode_Process_VariableSubstitution(t *testing.T) {
	var capturedQuery string
	rows := []map[string]any{{"v": 1}}
	rowsJSON, _ := json.Marshal(rows)

	mock := &mockInfluxDBReceiver{
		mockInfluxDBAgent: mockInfluxDBAgent{
			processFunc: func(data []byte) ([]byte, error) {
				var qr struct {
					Query string `json:"query"`
				}
				_ = json.Unmarshal(data, &qr)
				capturedQuery = qr.Query
				return nil, nil
			},
		},
		receiveFunc: func(ctx context.Context) ([]byte, error) {
			return rowsJSON, nil
		},
	}

	def := flow.NodeDef{ID: "iq5", Type: "influxdb-query"}
	n, err := NewInfluxDBQueryNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"_influxdb_agent": mock,
		"query":           "SELECT * FROM cpu WHERE host = $host AND usage > $threshold",
		"timeout":         "5s",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"host":      "server-01",
		"threshold": float64(50),
	})
	msg := message.New(message.WithPayload(payload))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	// 문자열은 작은따옴표로 감싸고, 숫자는 그대로
	assert.Equal(t, "SELECT * FROM cpu WHERE host = 'server-01' AND usage > 50", capturedQuery)
}

func TestInfluxDBQueryNode_Process_NoQuery(t *testing.T) {
	mock := &mockInfluxDBReceiver{
		mockInfluxDBAgent: mockInfluxDBAgent{
			processFunc: func(data []byte) ([]byte, error) {
				return nil, nil
			},
		},
		receiveFunc: func(ctx context.Context) ([]byte, error) {
			return nil, nil
		},
	}

	def := flow.NodeDef{ID: "iq6", Type: "influxdb-query"}
	n, err := NewInfluxDBQueryNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"_influxdb_agent": mock,
		// query 없음
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	// payload에도 query 없음
	msg := message.New()
	_, err = n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no query found")
}

func TestInfluxDBQueryNode_NoAgent(t *testing.T) {
	def := flow.NodeDef{ID: "iq7", Type: "influxdb-query"}
	n, err := NewInfluxDBQueryNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"query": "from(bucket:\"test\")",
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent_ref is required")
}

func TestInfluxDBQueryNode_Process_CustomResultKey(t *testing.T) {
	rows := []map[string]any{{"k": "v"}}
	rowsJSON, _ := json.Marshal(rows)

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

	def := flow.NodeDef{ID: "iq8", Type: "influxdb-query"}
	n, err := NewInfluxDBQueryNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"_influxdb_agent": mock,
		"query":           "from(bucket:\"test\")",
		"result_key":      "data",
		"timeout":         "5s",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	msg := message.New()
	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// "data" 키로 결과가 저장되어야 한다
	_, ok := results[0].Payload().Get("data")
	assert.True(t, ok)

	// "results" 키에는 없어야 한다
	_, ok = results[0].Payload().Get("results")
	assert.False(t, ok)
}

func TestSubstituteInfluxDBVariables(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		payload  map[string]any
		expected string
	}{
		{
			name:     "문자열 변수",
			query:    "SELECT * FROM cpu WHERE host = $host",
			payload:  map[string]any{"host": "server-01"},
			expected: "SELECT * FROM cpu WHERE host = 'server-01'",
		},
		{
			name:     "숫자 변수 (float64)",
			query:    "SELECT * FROM cpu WHERE usage > $threshold",
			payload:  map[string]any{"threshold": float64(50)},
			expected: "SELECT * FROM cpu WHERE usage > 50",
		},
		{
			name:     "불리언 변수",
			query:    "SELECT * FROM cpu WHERE active = $active",
			payload:  map[string]any{"active": true},
			expected: "SELECT * FROM cpu WHERE active = true",
		},
		{
			name:     "누락된 변수는 원본 유지",
			query:    "SELECT * FROM cpu WHERE host = $host",
			payload:  map[string]any{},
			expected: "SELECT * FROM cpu WHERE host = $host",
		},
		{
			name:     "복합 치환",
			query:    "SELECT $field FROM $measurement WHERE host = $host",
			payload:  map[string]any{"field": "cpu", "measurement": "metrics", "host": "srv"},
			expected: "SELECT 'cpu' FROM 'metrics' WHERE host = 'srv'",
		},
		{
			name:     "정수 변수",
			query:    "LIMIT $n",
			payload:  map[string]any{"n": int64(100)},
			expected: "LIMIT 100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := message.NewPayload(tt.payload)
			result := substituteInfluxDBVariables(tt.query, payload)
			assert.Equal(t, tt.expected, result)
		})
	}
}
