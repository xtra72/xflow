package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// mockInfluxClient는 테스트용 InfluxClient 모의 구현체이다.
type mockInfluxClient struct {
	writeFunc  func(ctx context.Context, data []WriteData) error
	queryFunc  func(ctx context.Context, query string, lang string) ([]map[string]any, error)
	healthFunc func(ctx context.Context) error
	closeFunc  func() error

	listBucketsFunc       func(ctx context.Context) ([]BucketInfo, error)
	createBucketFunc      func(ctx context.Context, name string, retentionSeconds int64) (BucketInfo, error)
	deleteBucketFunc      func(ctx context.Context, bucketRef string) error
	truncateBucketFunc    func(ctx context.Context, bucketRef string) error
	listMeasurementsFunc  func(ctx context.Context, bucket string) ([]string, error)
	deleteMeasurementFunc func(ctx context.Context, bucket, measurement string) error

	// @spec SPEC-TSDB-002 §2.10 (U10) — 스키마 디스커버리 D2~D4.
	listTagKeysFunc   func(ctx context.Context, bucket, measurement string) ([]string, error)
	listTagValuesFunc func(ctx context.Context, bucket, measurement, tagKey string) ([]string, error)
	listFieldKeysFunc func(ctx context.Context, bucket, measurement string) ([]string, error)
}

func (m *mockInfluxClient) Write(ctx context.Context, data []WriteData) error {
	if m.writeFunc != nil {
		return m.writeFunc(ctx, data)
	}
	return nil
}

func (m *mockInfluxClient) Query(ctx context.Context, query string, lang string) ([]map[string]any, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, query, lang)
	}
	return nil, nil
}

func (m *mockInfluxClient) Health(ctx context.Context) error {
	if m.healthFunc != nil {
		return m.healthFunc(ctx)
	}
	return nil
}

func (m *mockInfluxClient) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func (m *mockInfluxClient) ListBuckets(ctx context.Context) ([]BucketInfo, error) {
	if m.listBucketsFunc != nil {
		return m.listBucketsFunc(ctx)
	}
	return nil, nil
}

func (m *mockInfluxClient) CreateBucket(ctx context.Context, name string, retentionSeconds int64) (BucketInfo, error) {
	if m.createBucketFunc != nil {
		return m.createBucketFunc(ctx, name, retentionSeconds)
	}
	return BucketInfo{}, nil
}

func (m *mockInfluxClient) DeleteBucket(ctx context.Context, bucketRef string) error {
	if m.deleteBucketFunc != nil {
		return m.deleteBucketFunc(ctx, bucketRef)
	}
	return nil
}

func (m *mockInfluxClient) TruncateBucket(ctx context.Context, bucketRef string) error {
	if m.truncateBucketFunc != nil {
		return m.truncateBucketFunc(ctx, bucketRef)
	}
	return nil
}

func (m *mockInfluxClient) ListMeasurements(ctx context.Context, bucket string) ([]string, error) {
	if m.listMeasurementsFunc != nil {
		return m.listMeasurementsFunc(ctx, bucket)
	}
	return nil, nil
}

func (m *mockInfluxClient) DeleteMeasurement(ctx context.Context, bucket, measurement string) error {
	if m.deleteMeasurementFunc != nil {
		return m.deleteMeasurementFunc(ctx, bucket, measurement)
	}
	return nil
}

func (m *mockInfluxClient) ListTagKeys(ctx context.Context, bucket, measurement string) ([]string, error) {
	if m.listTagKeysFunc != nil {
		return m.listTagKeysFunc(ctx, bucket, measurement)
	}
	return nil, nil
}

func (m *mockInfluxClient) ListTagValues(ctx context.Context, bucket, measurement, tagKey string, _ map[string]string) ([]string, error) {
	if m.listTagValuesFunc != nil {
		return m.listTagValuesFunc(ctx, bucket, measurement, tagKey)
	}
	return nil, nil
}

func (m *mockInfluxClient) ListFieldKeys(ctx context.Context, bucket, measurement string) ([]string, error) {
	if m.listFieldKeysFunc != nil {
		return m.listFieldKeysFunc(ctx, bucket, measurement)
	}
	return nil, nil
}

// newTestInfluxDBAgent는 테스트용 InfluxDBAgent를 mock 클라이언트와 함께 생성한다.
// Running 상태로 전환된 에이전트를 반환한다.
func newTestInfluxDBAgent(mock InfluxClient) *InfluxDBAgent {
	cfg := newInfluxDBTestConfig("3")
	ic, _ := parseInfluxDBConfig(cfg)
	a := &InfluxDBAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("influxdb")),
		agentConfig:   cfg,
		influxConfig:  ic,
		client:        mock,
		recvCh:        make(chan []byte, ic.BufferSize),
		done:          make(chan struct{}),
		stats:         agent.NewAgentStats(),
		logger:        slog.Default(),
		createdAt:     time.Now(),
		startedAt:     time.Now(),
	}
	// Running 상태로 전환
	_ = a.TransitionTo(lifecycle.StateInitializing)
	_ = a.TransitionTo(lifecycle.StateRunning)
	return a
}

// ===== 컴파일타임 인터페이스 준수 테스트 =====

func TestInfluxDBAgent_컴파일타임_Agent인터페이스(t *testing.T) {
	// InfluxDBAgent가 agent.Agent 인터페이스를 구현하는지 컴파일타임에 확인한다.
	var _ agent.Agent = (*InfluxDBAgent)(nil)
}

func TestInfluxDBAgent_컴파일타임_MessageReceiver인터페이스(t *testing.T) {
	// InfluxDBAgent가 agent.MessageReceiver 인터페이스를 구현하는지 컴파일타임에 확인한다.
	var _ agent.MessageReceiver = (*InfluxDBAgent)(nil)
}

// ===== Process - 쓰기 테스트 =====

func TestInfluxDBAgent_Process_단일쓰기(t *testing.T) {
	// 단일 WriteData를 전송하면 client.Write가 올바른 데이터로 호출되어야 한다.
	var capturedData []WriteData
	mock := &mockInfluxClient{
		writeFunc: func(_ context.Context, data []WriteData) error {
			capturedData = data
			return nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	input := map[string]any{
		"measurement": "cpu",
		"tags":        map[string]any{"host": "server01"},
		"fields":      map[string]any{"usage": 85.5},
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.NoError(t, err)
	require.Len(t, capturedData, 1)
	assert.Equal(t, "cpu", capturedData[0].Measurement)
	assert.Equal(t, map[string]string{"host": "server01"}, capturedData[0].Tags)
}

func TestInfluxDBAgent_Process_배치쓰기(t *testing.T) {
	// JSON 배열로 여러 WriteData를 전송하면 배치 쓰기가 실행되어야 한다.
	var capturedData []WriteData
	mock := &mockInfluxClient{
		writeFunc: func(_ context.Context, data []WriteData) error {
			capturedData = data
			return nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	input := []map[string]any{
		{"measurement": "cpu", "fields": map[string]any{"usage": 85.5}},
		{"measurement": "mem", "fields": map[string]any{"free": 1024}},
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.NoError(t, err)
	require.Len(t, capturedData, 2)
	assert.Equal(t, "cpu", capturedData[0].Measurement)
	assert.Equal(t, "mem", capturedData[1].Measurement)
}

func TestInfluxDBAgent_Process_쓰기실패(t *testing.T) {
	// client.Write가 에러를 반환하면 Process도 에러를 반환해야 한다.
	mock := &mockInfluxClient{
		writeFunc: func(_ context.Context, _ []WriteData) error {
			return fmt.Errorf("connection refused")
		},
	}
	a := newTestInfluxDBAgent(mock)

	input := map[string]any{
		"measurement": "cpu",
		"fields":      map[string]any{"usage": 85.5},
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

// ===== Process - 쿼리 테스트 =====

func TestInfluxDBAgent_Process_쿼리(t *testing.T) {
	// "query" 키가 있는 JSON은 쿼리 리크에스트로 처리되어야 한다.
	expectedRows := []map[string]any{
		{"time": "2024-01-01T00:00:00Z", "value": 42.0},
	}
	var capturedQuery string
	mock := &mockInfluxClient{
		queryFunc: func(_ context.Context, query string, _ string) ([]map[string]any, error) {
			capturedQuery = query
			return expectedRows, nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	input := map[string]any{
		"query": "SELECT * FROM cpu",
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.NoError(t, err)
	assert.Equal(t, "SELECT * FROM cpu", capturedQuery)

	// recvCh에 결과가 전달되었는지 확인
	select {
	case result := <-a.recvCh:
		var rows []map[string]any
		require.NoError(t, json.Unmarshal(result, &rows))
		assert.Len(t, rows, 1)
	default:
		t.Fatal("recvCh에 결과가 전달되지 않았다")
	}
}

func TestInfluxDBAgent_Process_쿼리_언어지정(t *testing.T) {
	// 쿼리 요청에 language 필드가 있으면 해당 언어가 사용되어야 한다.
	var capturedLang string
	mock := &mockInfluxClient{
		queryFunc: func(_ context.Context, _ string, lang string) ([]map[string]any, error) {
			capturedLang = lang
			return nil, nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	input := map[string]any{
		"query":    "SELECT * FROM cpu",
		"language": "influxql",
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.NoError(t, err)
	assert.Equal(t, "influxql", capturedLang)
}

func TestInfluxDBAgent_Process_쿼리_기본언어(t *testing.T) {
	// language 필드가 없으면 설정의 기본 쿼리 언어가 사용되어야 한다.
	var capturedLang string
	mock := &mockInfluxClient{
		queryFunc: func(_ context.Context, _ string, lang string) ([]map[string]any, error) {
			capturedLang = lang
			return nil, nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	input := map[string]any{
		"query": "SELECT * FROM cpu",
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.NoError(t, err)
	// v3 기본 언어는 "sql"
	assert.Equal(t, "sql", capturedLang)
}

// ===== Process - 리크에스트 타입 감지 테스트 =====

func TestInfluxDBAgent_Process_빈데이터(t *testing.T) {
	// 빈 데이터를 전송하면 에러를 반환해야 한다.
	a := newTestInfluxDBAgent(&mockInfluxClient{})

	_, err := a.Process([]byte{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestInfluxDBAgent_Process_잘못된JSON(t *testing.T) {
	// 유효하지 않은 JSON을 전송하면 에러를 반환해야 한다.
	a := newTestInfluxDBAgent(&mockInfluxClient{})

	_, err := a.Process([]byte(`{invalid json`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JSON")
}

func TestInfluxDBAgent_Process_미지원형식(t *testing.T) {
	// "query"도 "measurement"도 없는 JSON 오브젝트는 에러를 반환해야 한다.
	a := newTestInfluxDBAgent(&mockInfluxClient{})

	input := map[string]any{
		"unknown_key": "value",
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "query")
	assert.Contains(t, err.Error(), "measurement")
}

// ===== Process - 유효성 검증 테스트 =====

func TestInfluxDBAgent_Process_measurement누락(t *testing.T) {
	// measurement 필드가 없는 쓰기 데이터는 유효성 검증 에러를 반환해야 한다.
	a := newTestInfluxDBAgent(&mockInfluxClient{})

	input := map[string]any{
		"measurement": "",
		"fields":      map[string]any{"usage": 85.5},
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "measurement")
}

func TestInfluxDBAgent_Process_fields누락(t *testing.T) {
	// fields 필드가 없는 쓰기 데이터는 유효성 검증 에러를 반환해야 한다.
	a := newTestInfluxDBAgent(&mockInfluxClient{})

	input := map[string]any{
		"measurement": "cpu",
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "필드")
}

// ===== ReceiveMessage 테스트 =====

func TestInfluxDBAgent_ReceiveMessage_정상(t *testing.T) {
	// recvCh에 데이터가 있으면 ReceiveMessage가 이를 반환해야 한다.
	a := newTestInfluxDBAgent(&mockInfluxClient{})
	expected := []byte(`{"result":"ok"}`)
	a.recvCh <- expected

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	result, err := a.ReceiveMessage(ctx)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestInfluxDBAgent_ReceiveMessage_Done(t *testing.T) {
	// done 채널이 닫히면 ReceiveMessage가 에러를 반환해야 한다.
	a := newTestInfluxDBAgent(&mockInfluxClient{})
	close(a.done)

	ctx := context.Background()
	_, err := a.ReceiveMessage(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stopped")
}

func TestInfluxDBAgent_ReceiveMessage_ContextCancel(t *testing.T) {
	// 컨텍스트가 취소되면 ReceiveMessage가 에러를 반환해야 한다.
	a := newTestInfluxDBAgent(&mockInfluxClient{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 즉시 취소

	_, err := a.ReceiveMessage(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// ===== 라이프사이클 테스트 =====

func TestInfluxDBAgent_Pause_Resume(t *testing.T) {
	// Pause 후 Process는 에러를 반환하고, Resume 후 다시 정상 동작해야 한다.
	mock := &mockInfluxClient{
		writeFunc: func(_ context.Context, _ []WriteData) error {
			return nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	input := map[string]any{
		"measurement": "cpu",
		"fields":      map[string]any{"usage": 50.0},
	}
	data, _ := json.Marshal(input)

	// Pause
	err := a.Pause(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StatePaused, a.CurrentState())

	// Pause 상태에서 Process 호출 시 에러
	_, err = a.Process(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "paused")

	// Resume
	err = a.Resume(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, a.CurrentState())

	// Resume 후 정상 동작
	_, err = a.Process(data)
	require.NoError(t, err)
}

func TestInfluxDBAgent_Stop(t *testing.T) {
	// Stop은 클라이언트를 닫고 done 채널을 닫아야 한다.
	closeCalled := false
	mock := &mockInfluxClient{
		closeFunc: func() error {
			closeCalled = true
			return nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	err := a.Stop(context.Background())
	require.NoError(t, err)
	assert.True(t, closeCalled, "client.Close()가 호출되어야 한다")
	assert.Equal(t, lifecycle.StateStopped, a.CurrentState())

	// done 채널이 닫혔는지 확인
	select {
	case <-a.done:
		// 채널이 닫힌 것을 확인
	default:
		t.Fatal("done 채널이 닫히지 않았다")
	}
}

func TestInfluxDBAgent_Health(t *testing.T) {
	// Health는 상태에 따라 올바른 HealthStatus를 반환해야 한다.
	tests := []struct {
		name           string
		state          lifecycle.State
		expectedStatus agent.HealthState
	}{
		{
			name:           "Running 상태",
			state:          lifecycle.StateRunning,
			expectedStatus: agent.HealthHealthy,
		},
		{
			name:           "Paused 상태",
			state:          lifecycle.StatePaused,
			expectedStatus: agent.HealthDegraded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestInfluxDBAgent(&mockInfluxClient{})
			if tt.state == lifecycle.StatePaused {
				_ = a.Pause(context.Background())
			}
			health := a.Health()
			assert.Equal(t, tt.expectedStatus, health.Status)
		})
	}
}

// ===== 에이전트 식별자 테스트 =====

func TestInfluxDBAgent_ID_Name_Type(t *testing.T) {
	// ID, Name, Type이 설정된 값을 올바르게 반환해야 한다.
	a := newTestInfluxDBAgent(&mockInfluxClient{})

	assert.Equal(t, "agent-influxdb-test", a.ID())
	assert.Equal(t, "test-influxdb", a.Name())
	assert.Equal(t, "influxdb", a.Type())
}

// ===== 타입 등록 테스트 =====

func TestRegisterInfluxDBTypes(t *testing.T) {
	// 최초 등록은 성공하고 중복 등록은 에러를 반환해야 한다.
	mgr := agent.NewManager()
	err := RegisterInfluxDBTypes(mgr)
	require.NoError(t, err)

	// 중복 등록 시 에러
	err = RegisterInfluxDBTypes(mgr)
	assert.Error(t, err)
}

// ===== 통계 테스트 =====

func TestInfluxDBAgent_Stats_쓰기후(t *testing.T) {
	// 성공적인 쓰기 후 통계가 업데이트되어야 한다.
	mock := &mockInfluxClient{
		writeFunc: func(_ context.Context, _ []WriteData) error {
			return nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	// 쓰기 전 통계 확인
	statsBefore := a.Stats()
	assert.Equal(t, int64(0), statsBefore.MessagesSent)
	assert.Equal(t, int64(0), statsBefore.BytesWritten)

	input := map[string]any{
		"measurement": "cpu",
		"fields":      map[string]any{"usage": 85.5},
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.NoError(t, err)

	// 쓰기 후 통계 확인
	statsAfter := a.Stats()
	assert.Equal(t, int64(1), statsAfter.MessagesSent)
	assert.Greater(t, statsAfter.BytesWritten, int64(0))
}

// ===== validateWriteData 테스트 =====

func TestValidateWriteData(t *testing.T) {
	// 다양한 WriteData에 대한 유효성 검증을 테이블 기반으로 테스트한다.
	tests := []struct {
		name    string
		data    WriteData
		wantErr bool
		errMsg  string
	}{
		{
			name: "유효한 데이터",
			data: WriteData{
				Measurement: "cpu",
				Fields:      map[string]any{"usage": 85.5},
			},
			wantErr: false,
		},
		{
			name: "태그 포함 유효한 데이터",
			data: WriteData{
				Measurement: "cpu",
				Tags:        map[string]string{"host": "server01"},
				Fields:      map[string]any{"usage": 85.5},
			},
			wantErr: false,
		},
		{
			name: "measurement 누락",
			data: WriteData{
				Measurement: "",
				Fields:      map[string]any{"usage": 85.5},
			},
			wantErr: true,
			errMsg:  "measurement",
		},
		{
			name: "fields 누락 (nil)",
			data: WriteData{
				Measurement: "cpu",
				Fields:      nil,
			},
			wantErr: true,
			errMsg:  "필드",
		},
		{
			name: "fields 빈 맵",
			data: WriteData{
				Measurement: "cpu",
				Fields:      map[string]any{},
			},
			wantErr: true,
			errMsg:  "필드",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWriteData(&tt.data)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ===== 구조화 시리즈 질의 (@spec SPEC-TSDB-002 §2.6 · §2.8) =====

// newTestInfluxDBAgentWithVersion 는 지정 버전의 테스트 에이전트를 만든다.
func newTestInfluxDBAgentWithVersion(version string, mock InfluxClient) *InfluxDBAgent {
	cfg := newInfluxDBTestConfig(version)
	ic, _ := parseInfluxDBConfig(cfg)
	a := &InfluxDBAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("influxdb")),
		agentConfig:   cfg,
		influxConfig:  ic,
		client:        mock,
		recvCh:        make(chan []byte, ic.BufferSize),
		done:          make(chan struct{}),
		stats:         agent.NewAgentStats(),
		logger:        slog.Default(),
		createdAt:     time.Now(),
		startedAt:     time.Now(),
	}
	_ = a.TransitionTo(lifecycle.StateInitializing)
	_ = a.TransitionTo(lifecycle.StateRunning)
	return a
}

func newTestSeriesSpec() SeriesQuerySpec {
	return SeriesQuerySpec{
		Measurement: "cpu",
		Field:       "usage",
		Tags:        map[string]string{"host": "a"},
		StartMs:     1_700_000_000_000,
		EndMs:       1_700_003_600_000,
		IntervalMs:  60_000,
		Aggregation: SeriesAggAverage,
	}
}

func TestInfluxDBAgent_QuerySeriesBuckets_버전분기(t *testing.T) {
	// v2 는 Flux, v3 는 InfluxQL 로 질의해야 한다.
	cases := []struct {
		version   string
		wantLang  string
		wantInQry string
	}{
		{version: "2", wantLang: "flux", wantInQry: "aggregateWindow"},
		// 여는 괄호를 리터럴에 넣지 않는다 — AC-22 의 offset 인자 스캔
		// (GROUP BY time\([^)]*,) 이 이 테스트 표의 쉼표에 걸린다.
		{version: "3", wantLang: "influxql", wantInQry: "GROUP BY time"},
	}
	for _, tc := range cases {
		t.Run("v"+tc.version, func(t *testing.T) {
			var gotQuery, gotLang string
			mock := &mockInfluxClient{
				queryFunc: func(_ context.Context, q, lang string) ([]map[string]any, error) {
					gotQuery, gotLang = q, lang
					return nil, nil
				},
			}
			a := newTestInfluxDBAgentWithVersion(tc.version, mock)

			_, err := a.QuerySeriesBuckets(context.Background(), newTestSeriesSpec())
			require.NoError(t, err)
			assert.Equal(t, tc.wantLang, gotLang)
			assert.Contains(t, gotQuery, tc.wantInQry)
		})
	}
}

func TestInfluxDBAgent_QuerySeriesBuckets_빈bucket은에이전트기본값을쓴다(t *testing.T) {
	var gotQuery string
	mock := &mockInfluxClient{
		queryFunc: func(_ context.Context, q, _ string) ([]map[string]any, error) {
			gotQuery = q
			return nil, nil
		},
	}
	a := newTestInfluxDBAgentWithVersion("2", mock)

	spec := newTestSeriesSpec()
	spec.Bucket = "" // 미지정
	_, err := a.QuerySeriesBuckets(context.Background(), spec)
	require.NoError(t, err)
	assert.Contains(t, gotQuery, `from(bucket: "my-bucket")`)
}

func TestInfluxDBAgent_QuerySeriesBuckets_Flux결과정규화(t *testing.T) {
	base := time.UnixMilli(1_700_000_040_000).UTC()
	mock := &mockInfluxClient{
		queryFunc: func(_ context.Context, _, _ string) ([]map[string]any, error) {
			return []map[string]any{
				// 일부러 시간 역순으로 준다 — 결과는 오름차순이어야 한다.
				{"_time": base.Add(2 * time.Minute), "_value": 3.0},
				{"_time": base, "_value": 1.0},
				{"_time": base.Add(time.Minute), "_value": nil}, // fill 로 만들어진 빈 버킷
				{"_value": 9.0}, // 시간 없음 → 버려진다
			}, nil
		},
	}
	a := newTestInfluxDBAgentWithVersion("2", mock)

	got, err := a.QuerySeriesBuckets(context.Background(), newTestSeriesSpec())
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, base.UnixMilli(), got[0].StartMs)
	assert.Equal(t, 1.0, got[0].Value)
	assert.Equal(t, base.Add(time.Minute).UnixMilli(), got[1].StartMs)
	assert.Nil(t, got[1].Value, "빈 버킷의 null 은 그대로 통과해야 한다")
	assert.Equal(t, base.Add(2*time.Minute).UnixMilli(), got[2].StartMs)
}

func TestInfluxDBAgent_QuerySeriesBuckets_InfluxQL결과정규화(t *testing.T) {
	// InfluxQL 결과의 시간 컬럼은 time, 값 컬럼은 집계 함수 이름(mean)이다.
	base := time.UnixMilli(1_700_000_040_000).UTC()
	mock := &mockInfluxClient{
		queryFunc: func(_ context.Context, _, _ string) ([]map[string]any, error) {
			return []map[string]any{
				{"time": base, "mean": 1.5},
				{"time": base.Add(time.Minute).UnixNano(), "mean": 2.5},
				{"time": base.Add(2 * time.Minute).Format(time.RFC3339Nano), "mean": 3.5},
			}, nil
		},
	}
	a := newTestInfluxDBAgentWithVersion("3", mock)

	got, err := a.QuerySeriesBuckets(context.Background(), newTestSeriesSpec())
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, base.UnixMilli(), got[0].StartMs)
	assert.Equal(t, 1.5, got[0].Value)
	assert.Equal(t, base.Add(time.Minute).UnixMilli(), got[1].StartMs)
	assert.Equal(t, 2.5, got[1].Value)
	assert.Equal(t, base.Add(2*time.Minute).UnixMilli(), got[2].StartMs)
	assert.Equal(t, 3.5, got[2].Value)
}

func TestInfluxDBAgent_QuerySeriesBuckets_버킷시작으로정렬한다(t *testing.T) {
	// 경계와 어긋난 타임스탬프가 와도 버킷 시작으로 내림 정렬된다(§2.8).
	mock := &mockInfluxClient{
		queryFunc: func(_ context.Context, _, _ string) ([]map[string]any, error) {
			return []map[string]any{
				{"_time": time.UnixMilli(1_700_000_073_456).UTC(), "_value": 1.0},
			}, nil
		},
	}
	a := newTestInfluxDBAgentWithVersion("2", mock)

	got, err := a.QuerySeriesBuckets(context.Background(), newTestSeriesSpec())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, SeriesBucketStartMs(1_700_000_073_456, 60_000), got[0].StartMs)
	assert.Zero(t, got[0].StartMs%60_000)
}

func TestInfluxDBAgent_QuerySeriesBuckets_오류전파(t *testing.T) {
	t.Run("클라이언트 미초기화", func(t *testing.T) {
		a := newTestInfluxDBAgentWithVersion("2", nil)
		a.client = nil
		_, err := a.QuerySeriesBuckets(context.Background(), newTestSeriesSpec())
		require.Error(t, err)
	})

	t.Run("지원하지 않는 버전", func(t *testing.T) {
		a := newTestInfluxDBAgentWithVersion("2", &mockInfluxClient{})
		a.influxConfig.Version = "9"
		_, err := a.QuerySeriesBuckets(context.Background(), newTestSeriesSpec())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "지원하지 않는 버전")
	})

	t.Run("쿼리 생성 실패는 실행 전에 드러난다", func(t *testing.T) {
		called := false
		mock := &mockInfluxClient{
			queryFunc: func(_ context.Context, _, _ string) ([]map[string]any, error) {
				called = true
				return nil, nil
			},
		}
		a := newTestInfluxDBAgentWithVersion("2", mock)
		spec := newTestSeriesSpec()
		spec.Measurement = "cpu\nDROP"
		_, err := a.QuerySeriesBuckets(context.Background(), spec)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUnescapableIdentifier))
		assert.False(t, called, "생성 실패한 쿼리를 실행해서는 안 된다")
	})

	t.Run("업스트림 오류 래핑", func(t *testing.T) {
		mock := &mockInfluxClient{
			queryFunc: func(_ context.Context, _, _ string) ([]map[string]any, error) {
				return nil, context.DeadlineExceeded
			},
		}
		a := newTestInfluxDBAgentWithVersion("2", mock)
		_, err := a.QuerySeriesBuckets(context.Background(), newTestSeriesSpec())
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.DeadlineExceeded), "408 매핑을 위해 sentinel 이 보존되어야 한다")
	})
}

func TestSeriesRowTimeMs_컬럼이름과값타입(t *testing.T) {
	base := time.UnixMilli(1_700_000_040_000).UTC()
	cases := []struct {
		name   string
		row    map[string]any
		wantMs int64
		wantOK bool
	}{
		{"flux _time (time.Time)", map[string]any{"_time": base}, base.UnixMilli(), true},
		{"influxql time (time.Time)", map[string]any{"time": base}, base.UnixMilli(), true},
		{"int64 나노초", map[string]any{"time": base.UnixNano()}, base.UnixMilli(), true},
		{"int 나노초", map[string]any{"time": int(base.UnixNano())}, base.UnixMilli(), true},
		{"uint64 나노초", map[string]any{"time": uint64(base.UnixNano())}, base.UnixMilli(), true},
		{"float64 나노초", map[string]any{"time": float64(base.UnixNano())}, base.UnixMilli(), true},
		{"RFC3339Nano 문자열", map[string]any{"time": base.Format(time.RFC3339Nano)}, base.UnixMilli(), true},
		{"파싱 불가 문자열", map[string]any{"time": "not-a-time"}, 0, false},
		{"zero time", map[string]any{"_time": time.Time{}}, 0, false},
		{"알 수 없는 타입", map[string]any{"time": []int{1}}, 0, false},
		{"시간 컬럼 부재", map[string]any{"_value": 1.0}, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := seriesRowTimeMs(tc.row)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantMs, got)
			}
		})
	}
}

func TestSeriesRowValue_컬럼우선순위(t *testing.T) {
	// _value 가 있으면 그것을 쓰고, 없으면 집계 함수 이름 컬럼으로 폴백한다.
	assert.Equal(t, 1.0, seriesRowValue(map[string]any{"_value": 1.0, "mean": 2.0}, "mean"))
	assert.Equal(t, 2.0, seriesRowValue(map[string]any{"mean": 2.0}, "mean"))
	// 키가 있으나 값이 nil 인 빈 버킷은 nil 그대로 통과한다.
	assert.Nil(t, seriesRowValue(map[string]any{"_value": nil}, "mean"))
	// 둘 다 없으면 nil.
	assert.Nil(t, seriesRowValue(map[string]any{"other": 3.0}, "mean"))
}

func TestNormalizeSeriesBuckets_미지의집계는거부된다(t *testing.T) {
	_, err := normalizeSeriesBuckets(nil, SeriesQuerySpec{Aggregation: SeriesAggregation(99)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported series aggregation")
}

// ===== group by 결과 운반 (SPEC-TSDB-004 §2.5 U5) =====

// TestInfluxDBAgent_QuerySeriesBuckets_GroupBy_태그운반 은 그룹 키 컬럼이
// SeriesBucket.Tags 로 옮겨지는지 고정한다(AC-07).
//
// v2 · v3 를 같은 표로 도는 이유는 normalizeSeriesBuckets 가 방언 중립이기
// 때문이다 — 그룹 키는 어느 방언에서나 결과 **행의 컬럼**으로 온다.
func TestInfluxDBAgent_QuerySeriesBuckets_GroupBy_태그운반(t *testing.T) {
	base := time.UnixMilli(1_700_000_040_000).UTC()

	cases := map[string]struct {
		version  string
		timeCol  string
		valueCol string
	}{
		"v2 flux":     {"2", "_time", "_value"},
		"v3 influxql": {"3", "time", "mean"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			mock := &mockInfluxClient{
				queryFunc: func(_ context.Context, _, _ string) ([]map[string]any, error) {
					return []map[string]any{
						// 일부러 그룹이 섞이고 시간도 역순인 순서로 준다.
						{tc.timeCol: base.Add(time.Minute), tc.valueCol: 2.0, "host": "b", "rack": "r1"},
						{tc.timeCol: base, tc.valueCol: 1.0, "host": "b", "rack": "r1"},
						{tc.timeCol: base, tc.valueCol: 9.0, "host": "a", "rack": "r1"},
						{tc.timeCol: base.Add(time.Minute), tc.valueCol: 8.0, "host": "a", "rack": "r1"},
					}, nil
				},
			}
			a := newTestInfluxDBAgentWithVersion(tc.version, mock)

			spec := newTestSeriesSpec()
			spec.Tags = nil
			spec.GroupBy = []string{"rack", "host"} // 정렬 전 순서

			got, err := a.QuerySeriesBuckets(context.Background(), spec)
			require.NoError(t, err)
			require.Len(t, got, 4)

			// 그룹 태그 사전순(host a → b) 우선, 그 안에서 시각 오름차순(§2.8 UB1-8).
			assert.Equal(t, map[string]string{"host": "a", "rack": "r1"}, got[0].Tags)
			assert.Equal(t, 9.0, got[0].Value)
			assert.Equal(t, map[string]string{"host": "a", "rack": "r1"}, got[1].Tags)
			assert.Equal(t, 8.0, got[1].Value)
			assert.Less(t, got[0].StartMs, got[1].StartMs)

			assert.Equal(t, map[string]string{"host": "b", "rack": "r1"}, got[2].Tags)
			assert.Equal(t, 1.0, got[2].Value)
			assert.Equal(t, map[string]string{"host": "b", "rack": "r1"}, got[3].Tags)
			assert.Less(t, got[2].StartMs, got[3].StartMs)
		})
	}
}

// TestInfluxDBAgent_QuerySeriesBuckets_GroupBy_없으면Tags는nil 은 §2.5 의
// 정확 일치 모드 계약을 고정한다. nil 이어야 라벨 구성이 본 축 도입 이전과
// 같아진다(§2.9 U9).
func TestInfluxDBAgent_QuerySeriesBuckets_GroupBy_없으면Tags는nil(t *testing.T) {
	base := time.UnixMilli(1_700_000_040_000).UTC()
	mock := &mockInfluxClient{
		queryFunc: func(_ context.Context, _, _ string) ([]map[string]any, error) {
			// 행에 태그 컬럼이 있어도 GroupBy 가 비면 싣지 않는다.
			return []map[string]any{
				{"_time": base, "_value": 1.0, "host": "a"},
			}, nil
		},
	}
	a := newTestInfluxDBAgentWithVersion("2", mock)

	got, err := a.QuerySeriesBuckets(context.Background(), newTestSeriesSpec())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Nil(t, got[0].Tags)
}

// TestInfluxDBAgent_QuerySeriesBuckets_GroupBy_태그결손 은 엣지 케이스를
// 고정한다 — 일부 행에만 그룹 태그가 있으면 결손 행은 **빈 값인 별도 그룹**이
// 되어야 한다. 조용히 버리면 데이터가 사라지고 사용자는 결손 사실을 모른다.
func TestInfluxDBAgent_QuerySeriesBuckets_GroupBy_태그결손(t *testing.T) {
	base := time.UnixMilli(1_700_000_040_000).UTC()
	mock := &mockInfluxClient{
		queryFunc: func(_ context.Context, _, _ string) ([]map[string]any, error) {
			return []map[string]any{
				{"_time": base, "_value": 1.0, "host": "a"},
				{"_time": base, "_value": 2.0}, // host 결손
			}, nil
		},
	}
	a := newTestInfluxDBAgentWithVersion("2", mock)

	spec := newTestSeriesSpec()
	spec.Tags = nil
	spec.GroupBy = []string{"host"}

	got, err := a.QuerySeriesBuckets(context.Background(), spec)
	require.NoError(t, err)
	require.Len(t, got, 2, "결손 행을 버리지 않는다")
	// 빈 문자열이 사전순으로 앞선다.
	assert.Equal(t, map[string]string{"host": ""}, got[0].Tags)
	assert.Equal(t, 2.0, got[0].Value)
	assert.Equal(t, map[string]string{"host": "a"}, got[1].Tags)
}

// TestInfluxDBAgent_QuerySeriesBuckets_GroupBy_비문자열태그값 은 클라이언트가
// 태그를 문자열이 아닌 타입으로 넘겼을 때의 처분을 고정한다.
//
// 태그는 규약상 문자열이지만 방언·클라이언트에 따라 숫자나 불리언이 올 수 있다.
// 그 경우 그룹이 통째로 빈 값으로 접히면 서로 다른 시리즈가 한 줄로 합쳐지므로,
// 표기로 대체해 그룹을 보존한다.
func TestInfluxDBAgent_QuerySeriesBuckets_GroupBy_비문자열태그값(t *testing.T) {
	base := time.UnixMilli(1_700_000_040_000).UTC()
	mock := &mockInfluxClient{
		queryFunc: func(_ context.Context, _, _ string) ([]map[string]any, error) {
			return []map[string]any{
				{"_time": base, "_value": 1.0, "port": 8080},
				{"_time": base, "_value": 2.0, "port": 9090},
			}, nil
		},
	}
	a := newTestInfluxDBAgentWithVersion("2", mock)

	spec := newTestSeriesSpec()
	spec.Tags = nil
	spec.GroupBy = []string{"port"}

	got, err := a.QuerySeriesBuckets(context.Background(), spec)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, map[string]string{"port": "8080"}, got[0].Tags)
	assert.Equal(t, map[string]string{"port": "9090"}, got[1].Tags)
}

// TestInfluxDBAgent_QuerySeriesBuckets_GroupBy_순서안정성 은 AC-17 을 고정한다.
// 백엔드가 행 순서를 바꿔 돌려줘도 그룹 순서는 태그 값 사전순으로 같아야 한다.
func TestInfluxDBAgent_QuerySeriesBuckets_GroupBy_순서안정성(t *testing.T) {
	base := time.UnixMilli(1_700_000_040_000).UTC()
	rows := [][]map[string]any{
		{
			{"_time": base, "_value": 1.0, "host": "c"},
			{"_time": base, "_value": 2.0, "host": "a"},
			{"_time": base, "_value": 3.0, "host": "b"},
		},
		{
			{"_time": base, "_value": 3.0, "host": "b"},
			{"_time": base, "_value": 1.0, "host": "c"},
			{"_time": base, "_value": 2.0, "host": "a"},
		},
	}

	var signatures []string
	for _, r := range rows {
		rowsCopy := r
		mock := &mockInfluxClient{
			queryFunc: func(_ context.Context, _, _ string) ([]map[string]any, error) {
				return rowsCopy, nil
			},
		}
		a := newTestInfluxDBAgentWithVersion("2", mock)
		spec := newTestSeriesSpec()
		spec.Tags = nil
		spec.GroupBy = []string{"host"}

		got, err := a.QuerySeriesBuckets(context.Background(), spec)
		require.NoError(t, err)

		var sb strings.Builder
		for _, b := range got {
			sb.WriteString(b.Tags["host"])
		}
		signatures = append(signatures, sb.String())
	}
	assert.Equal(t, "abc", signatures[0])
	assert.Equal(t, signatures[0], signatures[1], "백엔드 행 순서와 무관하게 그룹 순서가 같아야 한다")
}

// TestInfluxDBAgent_QuerySeriesBuckets_GroupBy_V2왕복 은 **실제 v2 클라이언트**로
// 끝에서 끝까지 태워 spec.md §6 가정 1 을 검증한다 — queryFlux 가 결과 행의
// 모든 컬럼을 통과시키므로 keep 이 남긴 태그 컬럼이 살아온다.
//
// 모의 클라이언트 테스트는 "행에 태그 컬럼이 있다면" 을 가정하지만, 이 테스트는
// 그 가정 자체를 CSV 왕복으로 확인한다. 가정이 깨지면 그룹 태그가 통째로 사라져
// 모든 그룹이 빈 태그로 접힌다.
func TestInfluxDBAgent_QuerySeriesBuckets_GroupBy_V2왕복(t *testing.T) {
	// keep(columns: ["_time","_value","host"]) 이 남긴 형상의 주석 CSV.
	const csv = "#datatype,string,long,dateTime:RFC3339,double,string\r\n" +
		"#group,false,false,false,false,true\r\n" +
		"#default,_result,,,,\r\n" +
		",result,table,_time,_value,host\r\n" +
		",,0,2023-11-14T22:14:00Z,1.5,a\r\n" +
		",,1,2023-11-14T22:14:00Z,2.5,b\r\n"

	var seenQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(body, &req)
		seenQuery = req.Query
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = w.Write([]byte(csv))
	}))
	defer ts.Close()

	c, err := newInfluxV2Client(InfluxDBConfig{URL: ts.URL, Token: "tok", Org: "org", Bucket: "metrics"})
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	a := newTestInfluxDBAgentWithVersion("2", c)

	spec := newTestSeriesSpec()
	spec.Bucket = "metrics"
	spec.Tags = nil
	spec.GroupBy = []string{"host"}

	got, err := a.QuerySeriesBuckets(context.Background(), spec)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, map[string]string{"host": "a"}, got[0].Tags)
	assert.Equal(t, map[string]string{"host": "b"}, got[1].Tags)

	// 생성된 쿼리가 실제로 그룹 축을 실었는지도 함께 고정한다.
	assert.Contains(t, seenQuery, `|> group(columns: ["host"])`)
	assert.Contains(t, seenQuery, `|> keep(columns: ["_time", "_value", "host"])`)
}
