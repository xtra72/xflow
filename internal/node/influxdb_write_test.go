package node

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// mockInfluxDBAgent 는 테스트용 InfluxDB 에이전트 모의 객체이다.
type mockInfluxDBAgent struct {
	processFunc func(data []byte) ([]byte, error)
}

func (m *mockInfluxDBAgent) Process(data []byte) ([]byte, error) {
	return m.processFunc(data)
}

// mockInfluxDBReceiver 는 테스트용 InfluxDB 수신 에이전트 모의 객체이다.
type mockInfluxDBReceiver struct {
	mockInfluxDBAgent
	receiveFunc func(ctx context.Context) ([]byte, error)
}

func (m *mockInfluxDBReceiver) ReceiveMessage(ctx context.Context) ([]byte, error) {
	return m.receiveFunc(ctx)
}

func TestInfluxDBWriteNode_Configure(t *testing.T) {
	def := flow.NodeDef{ID: "iw1", Type: "influxdb-write"}
	n, err := NewInfluxDBWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"measurement": "temperature",
		"tag_mappings": map[string]any{
			"host":   "hostname",
			"region": "location",
		},
		"field_mappings": map[string]any{
			"value": "temp_value",
		},
		"timestamp_key": "ts",
	})
	require.NoError(t, err)

	iw := n.(*InfluxDBWriteNode)
	assert.Equal(t, "temperature", iw.measurement)
	assert.Equal(t, map[string]string{"host": "hostname", "region": "location"}, iw.tagMappings)
	assert.Equal(t, map[string]string{"value": "temp_value"}, iw.fieldMappings)
	assert.Equal(t, "ts", iw.timestampKey)
}

func TestInfluxDBWriteNode_Process_FixedMeasurement(t *testing.T) {
	var captured influxdbWriteData

	mock := &mockInfluxDBAgent{
		processFunc: func(data []byte) ([]byte, error) {
			err := json.Unmarshal(data, &captured)
			return nil, err
		},
	}

	def := flow.NodeDef{ID: "iw2", Type: "influxdb-write"}
	n, err := NewInfluxDBWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"_influxdb_agent": mock,
		"measurement":     "cpu",
		"tag_mappings": map[string]any{
			"host": "hostname",
		},
		"field_mappings": map[string]any{
			"usage": "cpu_usage",
		},
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"hostname":  "server-01",
		"cpu_usage": 75.5,
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// pass-through 확인
	assert.Equal(t, msg.ID(), results[0].ID())

	// 캡처된 데이터 확인
	assert.Equal(t, "cpu", captured.Measurement)
	assert.Equal(t, "server-01", captured.Tags["host"])
	assert.Equal(t, 75.5, captured.Fields["usage"])
}

func TestInfluxDBWriteNode_Process_DynamicMeasurement(t *testing.T) {
	var captured influxdbWriteData

	mock := &mockInfluxDBAgent{
		processFunc: func(data []byte) ([]byte, error) {
			err := json.Unmarshal(data, &captured)
			return nil, err
		},
	}

	def := flow.NodeDef{ID: "iw3", Type: "influxdb-write"}
	n, err := NewInfluxDBWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"_influxdb_agent": mock,
		"measurement_key": "metric_name",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"metric_name": "memory",
		"usage":       82.3,
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "memory", captured.Measurement)
}

func TestInfluxDBWriteNode_Process_AllPayload(t *testing.T) {
	var captured influxdbWriteData

	mock := &mockInfluxDBAgent{
		processFunc: func(data []byte) ([]byte, error) {
			err := json.Unmarshal(data, &captured)
			return nil, err
		},
	}

	def := flow.NodeDef{ID: "iw4", Type: "influxdb-write"}
	n, err := NewInfluxDBWriteNode(def)
	require.NoError(t, err)

	// field_mappings를 설정하지 않으면 전체 payload가 fields로 사용
	err = n.Configure(map[string]any{
		"_influxdb_agent": mock,
		"measurement":     "raw_data",
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

	assert.Equal(t, "raw_data", captured.Measurement)
	assert.Equal(t, float64(100), captured.Fields["value1"])
	assert.Equal(t, float64(200), captured.Fields["value2"])
}

func TestInfluxDBWriteNode_Process_WithTimestamp(t *testing.T) {
	var captured influxdbWriteData

	mock := &mockInfluxDBAgent{
		processFunc: func(data []byte) ([]byte, error) {
			err := json.Unmarshal(data, &captured)
			return nil, err
		},
	}

	def := flow.NodeDef{ID: "iw5", Type: "influxdb-write"}
	n, err := NewInfluxDBWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"_influxdb_agent": mock,
		"measurement":     "sensor",
		"timestamp_key":   "ts",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	payload := message.NewPayload(map[string]any{
		"value": float64(42),
		"ts":    int64(1700000000000),
	})
	msg := message.New(message.WithPayload(payload))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "sensor", captured.Measurement)
	require.NotNil(t, captured.Timestamp)
	assert.Equal(t, int64(1700000000000), *captured.Timestamp)
}

func TestInfluxDBWriteNode_Process_EmptyMeasurement(t *testing.T) {
	mock := &mockInfluxDBAgent{
		processFunc: func(data []byte) ([]byte, error) {
			return nil, nil
		},
	}

	def := flow.NodeDef{ID: "iw6", Type: "influxdb-write"}
	n, err := NewInfluxDBWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"_influxdb_agent": mock,
		// measurement도 measurement_key도 없음
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"v": 1})))
	_, err = n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "measurement is empty")
}

// TestInfluxDBWriteNode_Process_DefaultMetadataToTags 는 v0.14.0 의 기본 동작
// (tag_mappings 미지정 시 모든 metadata 를 tags 로 사용) 을 검증한다.
func TestInfluxDBWriteNode_Process_DefaultMetadataToTags(t *testing.T) {
	var captured influxdbWriteData
	mock := &mockInfluxDBAgent{
		processFunc: func(data []byte) ([]byte, error) {
			return nil, json.Unmarshal(data, &captured)
		},
	}

	def := flow.NodeDef{ID: "iw-default-tags", Type: "influxdb-write"}
	n, err := NewInfluxDBWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"_influxdb_agent": mock,
		"measurement":     "device_state",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": float64(1)})))
	msg.Metadata().Set("dev_id", "0x3B")
	msg.Metadata().Set("device_type", "indoor")

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	assert.Equal(t, "0x3B", captured.Tags["dev_id"], "metadata.dev_id 가 tag 로 매핑되어야 함")
	assert.Equal(t, "indoor", captured.Tags["device_type"])
}

// TestInfluxDBWriteNode_Process_JSONPathTagMapping 은 tag_mappings 가 JSONPath
// 문법 ($.metadata.X / $.payload.X / $.type) 을 지원하는지 검증한다 (v0.14.0).
func TestInfluxDBWriteNode_Process_JSONPathTagMapping(t *testing.T) {
	var captured influxdbWriteData
	mock := &mockInfluxDBAgent{
		processFunc: func(data []byte) ([]byte, error) {
			return nil, json.Unmarshal(data, &captured)
		},
	}

	def := flow.NodeDef{ID: "iw-jsonpath", Type: "influxdb-write"}
	n, err := NewInfluxDBWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"_influxdb_agent": mock,
		"measurement":     "hvac",
		"tag_mappings": map[string]any{
			"device":     "$.metadata.dev_id",
			"event_type": "$.type",
			"location":   "$.payload.room",
		},
		"field_mappings": map[string]any{
			"temperature": "$.payload.current_temp",
		},
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"room":         "living",
		"current_temp": float64(23.5),
	})))
	msg.SetType("device_state.report")
	msg.Metadata().Set("dev_id", "0x3B")

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	assert.Equal(t, "0x3B", captured.Tags["device"], "$.metadata.dev_id → tag")
	assert.Equal(t, "device_state.report", captured.Tags["event_type"], "$.type → tag")
	assert.Equal(t, "living", captured.Tags["location"], "$.payload.room → tag")
	assert.Equal(t, float64(23.5), captured.Fields["temperature"], "$.payload.current_temp → field")
}

// TestInfluxDBWriteNode_Process_DefaultTimestamp 는 timestamp_key 미지정 시
// msg.Timestamp() 가 기본 사용되는지 검증한다 (v0.14.0).
func TestInfluxDBWriteNode_Process_DefaultTimestamp(t *testing.T) {
	var captured influxdbWriteData
	mock := &mockInfluxDBAgent{
		processFunc: func(data []byte) ([]byte, error) {
			return nil, json.Unmarshal(data, &captured)
		},
	}

	def := flow.NodeDef{ID: "iw-default-ts", Type: "influxdb-write"}
	n, err := NewInfluxDBWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"_influxdb_agent": mock,
		"measurement":     "sensor",
	})
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": float64(1)})))
	msg.SetTimestamp(time.UnixMilli(1779350220888))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	require.NotNil(t, captured.Timestamp, "v0.14.0: 기본 timestamp 는 msg.Timestamp() 사용")
	assert.Equal(t, int64(1779350220888), *captured.Timestamp)
}

func TestInfluxDBWriteNode_NoAgent(t *testing.T) {
	def := flow.NodeDef{ID: "iw7", Type: "influxdb-write"}
	n, err := NewInfluxDBWriteNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"measurement": "test",
	})
	require.NoError(t, err)

	// Init에서 _influxdb_agent도 없고 agent_ref도 없으므로 에러
	err = n.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent_ref is required")
}
