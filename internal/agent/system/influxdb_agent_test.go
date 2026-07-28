package system

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
