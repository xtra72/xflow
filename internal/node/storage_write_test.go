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

// --- 테스트 헬퍼 ---

// newStorageWriteNode 는 설정을 적용하고 Init 까지 마친 storage-write 노드를 만든다.
func newStorageWriteNode(t *testing.T, agent any, cfg map[string]any) *StorageWriteNode {
	t.Helper()
	def := flow.NodeDef{ID: "sw", Type: "storage-write"}
	n, err := NewStorageWriteNode(def)
	require.NoError(t, err)

	full := make(map[string]any, len(cfg)+1)
	for k, v := range cfg {
		full[k] = v
	}
	full["_storage_agent"] = agent

	require.NoError(t, n.Configure(full))
	require.NoError(t, n.Init(context.Background()))
	return n.(*StorageWriteNode)
}

// captureInflux 는 InfluxDB 에이전트로 전달된 write 페이로드를 순서대로 모은다.
func captureInflux(captured *[]influxdbWriteData) *mockInfluxDBAgent {
	return &mockInfluxDBAgent{
		processFunc: func(data []byte) ([]byte, error) {
			var wd influxdbWriteData
			if err := json.Unmarshal(data, &wd); err != nil {
				return nil, err
			}
			*captured = append(*captured, wd)
			return nil, nil
		},
	}
}

// sensorMsg 는 payload + device 그룹 metadata 를 가진 테스트 메시지를 만든다.
func sensorMsg(payload map[string]any, ts time.Time) message.Message {
	msg := message.New(
		message.WithPayload(message.NewPayload(payload)),
		message.WithTimestamp(ts),
		message.WithMetadata("region", "kr"),
	)
	msg.Metadata().SetGroup("device", map[string]string{"id": "dev-1", "type": "sensor"})
	return msg
}

// --- Configure 검증 ---

func TestStorageWrite_Configure_MeasurementRequiredInFieldsMode(t *testing.T) {
	n, err := NewStorageWriteNode(flow.NodeDef{ID: "sw", Type: "storage-write"})
	require.NoError(t, err)

	err = n.Configure(map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "measurement")
}

func TestStorageWrite_Configure_SplitModeNeedsNoMeasurement(t *testing.T) {
	n, err := NewStorageWriteNode(flow.NodeDef{ID: "sw", Type: "storage-write"})
	require.NoError(t, err)

	require.NoError(t, n.Configure(map[string]any{"payload_mode": "split"}))
}

func TestStorageWrite_Configure_InvalidFields(t *testing.T) {
	cases := []struct {
		name    string
		config  map[string]any
		wantErr string
	}{
		{
			name:    "payload_mode enum 위반",
			config:  map[string]any{"payload_mode": "wide", "measurement": "m"},
			wantErr: "payload_mode",
		},
		{
			name:    "ttl 파싱 실패",
			config:  map[string]any{"measurement": "m", "ttl": "5분"},
			wantErr: "ttl",
		},
		{
			name:    "exclude_keys 가 리스트가 아님",
			config:  map[string]any{"measurement": "m", "exclude_keys": "device"},
			wantErr: "exclude_keys",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, err := NewStorageWriteNode(flow.NodeDef{ID: "sw", Type: "storage-write"})
			require.NoError(t, err)
			err = n.Configure(tc.config)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// --- 백엔드 선택 ---

func TestStorageWrite_BackendSelection(t *testing.T) {
	cfg := map[string]any{"measurement": "m"}

	t.Run("store 에이전트", func(t *testing.T) {
		n := newStorageWriteNode(t, &mockStoreAgent{store: newMockStore()}, cfg)
		assert.Equal(t, "store", n.backend.backendName())
	})

	t.Run("influxdb 에이전트", func(t *testing.T) {
		var captured []influxdbWriteData
		n := newStorageWriteNode(t, captureInflux(&captured), cfg)
		assert.Equal(t, "influxdb", n.backend.backendName())
	})

	t.Run("지원하지 않는 타입", func(t *testing.T) {
		n, err := NewStorageWriteNode(flow.NodeDef{ID: "sw", Type: "storage-write"})
		require.NoError(t, err)
		require.NoError(t, n.Configure(map[string]any{
			"measurement":    "m",
			"_storage_agent": &mockUnsupportedAgent{},
		}))

		err = n.Init(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported agent type")
	})
}

// mockUnsupportedAgent 는 storage-write 가 지원하지 않는 타입의 에이전트이다.
type mockUnsupportedAgent struct{}

func (a *mockUnsupportedAgent) Type() string { return "mqtt" }

// --- 메시지 규약: payload=측정값 / metadata=태그 / timestamp=시각 ---

// TestStorageWrite_FieldsMode 는 payload 의 모든 키/값이 하나의 measurement 아래
// 여러 field 로 기록되고, 태그와 시각이 메시지에서 자동으로 채워지는지 확인한다.
func TestStorageWrite_FieldsMode(t *testing.T) {
	ts := time.UnixMilli(1_700_000_000_000)
	var captured []influxdbWriteData
	n := newStorageWriteNode(t, captureInflux(&captured), map[string]any{
		"measurement": "sensor",
	})

	out, err := n.Process(context.Background(),
		sensorMsg(map[string]any{"temperature": 23.5, "humidity": float64(60)}, ts))
	require.NoError(t, err)
	require.Len(t, out, 1, "pass-through 여야 한다")

	require.Len(t, captured, 1, "fields 모드는 배치 1개")
	wd := captured[0]
	assert.Equal(t, "sensor", wd.Measurement)
	assert.Equal(t, map[string]any{"temperature": 23.5, "humidity": float64(60)}, wd.Fields)
	// metadata → 태그 (그룹은 "{group}.{field}" 평면 태그로 펼쳐진다).
	assert.Equal(t, "kr", wd.Tags["region"])
	assert.Equal(t, "dev-1", wd.Tags["device.id"])
	assert.Equal(t, "sensor", wd.Tags["device.type"])
	// msg.timestamp → 기록 시각.
	require.NotNil(t, wd.Timestamp)
	assert.Equal(t, ts.UnixMilli(), *wd.Timestamp)
}

// TestStorageWrite_SplitMode 는 payload 키마다 별도 시리즈로 분리되고,
// 값 이름이 관례상 "value" 로 고정되는지 확인한다.
func TestStorageWrite_SplitMode(t *testing.T) {
	ts := time.UnixMilli(1_700_000_000_000)
	var captured []influxdbWriteData
	n := newStorageWriteNode(t, captureInflux(&captured), map[string]any{
		"payload_mode": "split",
	})

	_, err := n.Process(context.Background(),
		sensorMsg(map[string]any{"temperature": 23.5, "humidity": float64(60)}, ts))
	require.NoError(t, err)

	require.Len(t, captured, 2, "키마다 별도 시리즈")
	// 결과는 키 정렬 순서로 결정적이다.
	assert.Equal(t, "humidity", captured[0].Measurement)
	assert.Equal(t, map[string]any{"value": float64(60)}, captured[0].Fields)
	assert.Equal(t, "temperature", captured[1].Measurement)
	assert.Equal(t, map[string]any{"value": 23.5}, captured[1].Fields)
	// 태그는 두 시리즈에 동일하게 적용된다 (디바이스 구분은 태그가 담당).
	assert.Equal(t, "dev-1", captured[0].Tags["device.id"])
	assert.Equal(t, "dev-1", captured[1].Tags["device.id"])
}

// TestStorageWrite_SplitMode_ObjectExpandsToFields 는 split 모드에서 오브젝트 값이
// 그 오브젝트의 키들을 필드로 펼쳐 기록되는지 확인한다(JSON 문자열로 뭉개지지 않는다).
func TestStorageWrite_SplitMode_ObjectExpandsToFields(t *testing.T) {
	var captured []influxdbWriteData
	n := newStorageWriteNode(t, captureInflux(&captured), map[string]any{
		"payload_mode": "split",
	})

	_, err := n.Process(context.Background(), sensorMsg(map[string]any{
		"radio": map[string]any{"count": float64(2), "rssi": float64(-109)},
	}, time.Now()))
	require.NoError(t, err)

	require.Len(t, captured, 1)
	assert.Equal(t, "radio", captured[0].Measurement)
	assert.Equal(t, map[string]any{"count": float64(2), "rssi": float64(-109)}, captured[0].Fields)
}

// TestStorageWrite_SplitMode_ScalarUsesValueField 는 스칼라 값이 "value" 필드로
// 기록되는지 확인한다.
func TestStorageWrite_SplitMode_ScalarUsesValueField(t *testing.T) {
	var captured []influxdbWriteData
	n := newStorageWriteNode(t, captureInflux(&captured), map[string]any{
		"payload_mode": "split",
	})

	_, err := n.Process(context.Background(),
		sensorMsg(map[string]any{"temperature": 23.5}, time.Now()))
	require.NoError(t, err)

	require.Len(t, captured, 1)
	assert.Equal(t, "temperature", captured[0].Measurement)
	assert.Equal(t, map[string]any{"value": 23.5}, captured[0].Fields)
}

// TestStorageWrite_SplitMode_ObjectWithNoUsableKeys 는 필드 이름으로 쓸 수 없는 키만
// 가진 오브젝트가 통째로 생략되는지 확인한다.
func TestStorageWrite_SplitMode_ObjectWithNoUsableKeys(t *testing.T) {
	var captured []influxdbWriteData
	n := newStorageWriteNode(t, captureInflux(&captured), map[string]any{
		"payload_mode": "split",
	})

	_, err := n.Process(context.Background(), sensorMsg(map[string]any{
		"bad": map[string]any{"has space": 1.0},
	}, time.Now()))
	require.NoError(t, err)
	assert.Empty(t, captured)
}

// TestStorageWrite_MeasurementTemplate 는 measurement 가 {…} 보간을 지원하는지 확인한다.
func TestStorageWrite_MeasurementTemplate(t *testing.T) {
	var captured []influxdbWriteData
	n := newStorageWriteNode(t, captureInflux(&captured), map[string]any{
		"measurement": "{$.metadata.device.id}:sensor",
	})

	_, err := n.Process(context.Background(),
		sensorMsg(map[string]any{"temperature": 1.0}, time.Now()))
	require.NoError(t, err)

	require.Len(t, captured, 1)
	assert.Equal(t, "dev-1:sensor", captured[0].Measurement)
}

// TestStorageWrite_MeasurementResolutionFails 는 measurement 템플릿을 해석할 수
// 없으면 에러를 반환하는지 확인한다 (시리즈 이름은 필수).
func TestStorageWrite_MeasurementResolutionFails(t *testing.T) {
	var captured []influxdbWriteData
	n := newStorageWriteNode(t, captureInflux(&captured), map[string]any{
		"measurement": "{$.payload.missing}",
	})

	_, err := n.Process(context.Background(),
		sensorMsg(map[string]any{"temperature": 1.0}, time.Now()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "measurement")
}

// TestStorageWrite_ExcludeKeys 는 제외 키가 측정값에서 빠지는지 확인한다.
func TestStorageWrite_ExcludeKeys(t *testing.T) {
	var captured []influxdbWriteData
	n := newStorageWriteNode(t, captureInflux(&captured), map[string]any{
		"measurement":  "sensor",
		"exclude_keys": []any{"room", "device"},
	})

	_, err := n.Process(context.Background(), sensorMsg(map[string]any{
		"temperature": 23.5,
		"room":        "lab",
		"device":      "dev-1",
	}, time.Now()))
	require.NoError(t, err)

	require.Len(t, captured, 1)
	assert.Equal(t, map[string]any{"temperature": 23.5}, captured[0].Fields)
}

// TestStorageWrite_SkipsUnusableKeys 는 측정 종류 이름으로 쓸 수 없는 키
// (공백/기호 포함)를 건너뛰는지 확인한다.
func TestStorageWrite_SkipsUnusableKeys(t *testing.T) {
	var captured []influxdbWriteData
	n := newStorageWriteNode(t, captureInflux(&captured), map[string]any{
		"measurement": "sensor",
	})

	_, err := n.Process(context.Background(), sensorMsg(map[string]any{
		"temperature": 23.5,
		"bad key!":    1.0,
	}, time.Now()))
	require.NoError(t, err)

	require.Len(t, captured, 1)
	assert.Equal(t, map[string]any{"temperature": 23.5}, captured[0].Fields)
}

// TestStorageWrite_EmptyPayload_NoWrite 는 기록할 측정값이 없으면 백엔드를
// 호출하지 않고 pass-through 하는지 확인한다.
func TestStorageWrite_EmptyPayload_NoWrite(t *testing.T) {
	var captured []influxdbWriteData
	n := newStorageWriteNode(t, captureInflux(&captured), map[string]any{
		"measurement":  "sensor",
		"exclude_keys": []any{"temperature"},
	})

	out, err := n.Process(context.Background(),
		sensorMsg(map[string]any{"temperature": 23.5}, time.Now()))
	require.NoError(t, err)
	assert.Len(t, out, 1)
	assert.Empty(t, captured)
}

// TestStorageWrite_CompositeValue 는 중첩 오브젝트 값이 JSON 문자열로
// 변환되어 기록되는지 확인한다 (스칼라 필드에 오브젝트를 쓸 수 없으므로).
func TestStorageWrite_CompositeValue(t *testing.T) {
	var captured []influxdbWriteData
	n := newStorageWriteNode(t, captureInflux(&captured), map[string]any{
		"measurement": "sensor",
	})

	_, err := n.Process(context.Background(), sensorMsg(map[string]any{
		"state": map[string]any{"mode": "cool"},
	}, time.Now()))
	require.NoError(t, err)

	require.Len(t, captured, 1)
	assert.Equal(t, `{"mode":"cool"}`, captured[0].Fields["state"])
}

// --- 핵심 약속: 같은 설정 + 에이전트만 교체 ---

// TestStorageWrite_SameConfig_BothBackends 는 동일한 노드 설정이 store / influxdb
// 양쪽에서 모두 동작하며 같은 시리즈·측정값·태그로 기록되는지 확인한다.
func TestStorageWrite_SameConfig_BothBackends(t *testing.T) {
	ts := time.UnixMilli(1_700_000_000_000)
	cfg := map[string]any{"measurement": "{$.metadata.device.id}"}
	payload := map[string]any{"temperature": 23.5, "humidity": float64(60)}

	// --- store 백엔드 ---
	store := newMockStore()
	storeNode := newStorageWriteNode(t, &mockStoreAgent{store: store}, cfg)
	_, err := storeNode.Process(context.Background(), sensorMsg(payload, ts))
	require.NoError(t, err)

	writes := store.metaWrites("dev-1")
	require.Len(t, writes, 2)
	byMetric := make(map[string]mockStoreMetaWrite, len(writes))
	for _, w := range writes {
		byMetric[w.opts.Field] = w
	}
	assert.Equal(t, 23.5, byMetric["temperature"].value)
	assert.Equal(t, float64(60), byMetric["humidity"].value)
	assert.Equal(t, "dev-1", byMetric["temperature"].opts.Tags["device.id"])
	// 타입은 항상 auto 추론 — 설정에서 받지 않는다.
	assert.Equal(t, storageDataTypeAuto, byMetric["temperature"].opts.DataType)

	// --- influxdb 백엔드 (설정 동일) ---
	var captured []influxdbWriteData
	influxNode := newStorageWriteNode(t, captureInflux(&captured), cfg)
	_, err = influxNode.Process(context.Background(), sensorMsg(payload, ts))
	require.NoError(t, err)

	require.Len(t, captured, 1)
	assert.Equal(t, "dev-1", captured[0].Measurement)
	assert.Equal(t, map[string]any{"temperature": 23.5, "humidity": float64(60)}, captured[0].Fields)
	assert.Equal(t, "dev-1", captured[0].Tags["device.id"])
}

// --- 백엔드 전용 옵션 ---

// TestStorageWrite_BoolToInt 는 influxdb 전용 bool_to_int 가 적용되는지 확인한다.
func TestStorageWrite_BoolToInt(t *testing.T) {
	var captured []influxdbWriteData
	n := newStorageWriteNode(t, captureInflux(&captured), map[string]any{
		"measurement": "m",
		"bool_to_int": true,
	})

	_, err := n.Process(context.Background(), sensorMsg(map[string]any{"on": true}, time.Now()))
	require.NoError(t, err)

	require.Len(t, captured, 1)
	assert.Equal(t, float64(1), captured[0].Fields["on"], "JSON 왕복으로 1 은 float64 가 된다")
}

// TestStorageWrite_StoreTTL 는 store 전용 ttl 이 쓰기 옵션으로 전달되는지 확인한다.
func TestStorageWrite_StoreTTL(t *testing.T) {
	store := newMockStore()
	n := newStorageWriteNode(t, &mockStoreAgent{store: store}, map[string]any{
		"measurement": "m",
		"ttl":         "10m",
	})

	_, err := n.Process(context.Background(), sensorMsg(map[string]any{"v": 1.5}, time.Now()))
	require.NoError(t, err)

	writes := store.metaWrites("m")
	require.Len(t, writes, 1)
	assert.Equal(t, 10*time.Minute, writes[0].opts.TTL)
}

// TestStorageWrite_BackendOnlyOptionsAreInert 는 상대 백엔드 전용 옵션이
// 섞여 있어도 오류 없이 무시되는지 확인한다 (에이전트 교체 호환성의 전제).
func TestStorageWrite_BackendOnlyOptionsAreInert(t *testing.T) {
	cfg := map[string]any{
		"measurement": "m",
		"ttl":         "10m",     // store 전용
		"namespace":   "sensors", // store 전용
		"bool_to_int": true,      // influxdb 전용
	}
	payload := map[string]any{"v": 1.0}

	var captured []influxdbWriteData
	influxNode := newStorageWriteNode(t, captureInflux(&captured), cfg)
	_, err := influxNode.Process(context.Background(), sensorMsg(payload, time.Now()))
	require.NoError(t, err)
	assert.Len(t, captured, 1)

	store := newMockStore()
	storeNode := newStorageWriteNode(t, &mockStoreAgent{store: store}, cfg)
	_, err = storeNode.Process(context.Background(), sensorMsg(payload, time.Now()))
	require.NoError(t, err)
	assert.Len(t, store.metaWrites("m"), 1)
}

// --- 초기화 실패 ---

func TestStorageWrite_Init_RequiresAgentRef(t *testing.T) {
	n, err := NewStorageWriteNode(flow.NodeDef{ID: "sw", Type: "storage-write"})
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{"measurement": "m"}))

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent_ref")
}
