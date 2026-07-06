package system

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===== 컴파일타임 인터페이스 준수 =====

func TestInfluxDBAgent_컴파일타임_InfluxManager인터페이스(t *testing.T) {
	var _ InfluxManager = (*InfluxDBAgent)(nil)
}

// ===== 에이전트 위임 - 성공 경로 =====

func TestInfluxDBAgent_ListBuckets_위임(t *testing.T) {
	want := []BucketInfo{{ID: "b1", Name: "metrics", OrgID: "o1", RetentionSeconds: 3600}}
	mock := &mockInfluxClient{
		listBucketsFunc: func(_ context.Context) ([]BucketInfo, error) {
			return want, nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	got, err := a.ListBuckets(context.Background())
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestInfluxDBAgent_CreateBucket_위임(t *testing.T) {
	var gotName string
	var gotRetention int64
	mock := &mockInfluxClient{
		createBucketFunc: func(_ context.Context, name string, retentionSeconds int64) (BucketInfo, error) {
			gotName = name
			gotRetention = retentionSeconds
			return BucketInfo{ID: "b2", Name: name, RetentionSeconds: retentionSeconds}, nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	info, err := a.CreateBucket(context.Background(), "newbucket", 7200)
	require.NoError(t, err)
	assert.Equal(t, "newbucket", gotName)
	assert.Equal(t, int64(7200), gotRetention)
	assert.Equal(t, "b2", info.ID)
}

func TestInfluxDBAgent_DeleteBucket_위임(t *testing.T) {
	var gotRef string
	mock := &mockInfluxClient{
		deleteBucketFunc: func(_ context.Context, bucketRef string) error {
			gotRef = bucketRef
			return nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	require.NoError(t, a.DeleteBucket(context.Background(), "metrics"))
	assert.Equal(t, "metrics", gotRef)
}

func TestInfluxDBAgent_TruncateBucket_위임(t *testing.T) {
	var gotRef string
	mock := &mockInfluxClient{
		truncateBucketFunc: func(_ context.Context, bucketRef string) error {
			gotRef = bucketRef
			return nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	require.NoError(t, a.TruncateBucket(context.Background(), "metrics"))
	assert.Equal(t, "metrics", gotRef)
}

func TestInfluxDBAgent_ListMeasurements_위임(t *testing.T) {
	var gotBucket string
	mock := &mockInfluxClient{
		listMeasurementsFunc: func(_ context.Context, bucket string) ([]string, error) {
			gotBucket = bucket
			return []string{"cpu", "mem"}, nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	got, err := a.ListMeasurements(context.Background(), "explicit")
	require.NoError(t, err)
	assert.Equal(t, []string{"cpu", "mem"}, got)
	assert.Equal(t, "explicit", gotBucket)
}

func TestInfluxDBAgent_ListMeasurements_기본버킷_fallback(t *testing.T) {
	// bucket 을 빈 문자열로 전달하면 설정된 기본 버킷(테스트 config 의 "my-bucket") 을 사용한다.
	var gotBucket string
	mock := &mockInfluxClient{
		listMeasurementsFunc: func(_ context.Context, bucket string) ([]string, error) {
			gotBucket = bucket
			return nil, nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	_, err := a.ListMeasurements(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, a.defaultBucket(), gotBucket)
	assert.NotEmpty(t, gotBucket)
}

func TestInfluxDBAgent_DeleteMeasurement_위임(t *testing.T) {
	var gotBucket, gotMeasurement string
	mock := &mockInfluxClient{
		deleteMeasurementFunc: func(_ context.Context, bucket, measurement string) error {
			gotBucket = bucket
			gotMeasurement = measurement
			return nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	require.NoError(t, a.DeleteMeasurement(context.Background(), "b", "cpu"))
	assert.Equal(t, "b", gotBucket)
	assert.Equal(t, "cpu", gotMeasurement)
}

func TestInfluxDBAgent_DeleteMeasurement_기본버킷_fallback(t *testing.T) {
	var gotBucket string
	mock := &mockInfluxClient{
		deleteMeasurementFunc: func(_ context.Context, bucket, _ string) error {
			gotBucket = bucket
			return nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	require.NoError(t, a.DeleteMeasurement(context.Background(), "", "cpu"))
	assert.Equal(t, a.defaultBucket(), gotBucket)
}

// ===== 에러 전파 =====

func TestInfluxDBAgent_관리메서드_에러전파(t *testing.T) {
	sentinel := errors.New("boom")
	mock := &mockInfluxClient{
		listBucketsFunc:       func(_ context.Context) ([]BucketInfo, error) { return nil, sentinel },
		createBucketFunc:      func(_ context.Context, _ string, _ int64) (BucketInfo, error) { return BucketInfo{}, sentinel },
		deleteBucketFunc:      func(_ context.Context, _ string) error { return sentinel },
		truncateBucketFunc:    func(_ context.Context, _ string) error { return sentinel },
		listMeasurementsFunc:  func(_ context.Context, _ string) ([]string, error) { return nil, sentinel },
		deleteMeasurementFunc: func(_ context.Context, _, _ string) error { return sentinel },
	}
	a := newTestInfluxDBAgent(mock)
	ctx := context.Background()

	_, err := a.ListBuckets(ctx)
	assert.ErrorIs(t, err, sentinel)
	_, err = a.CreateBucket(ctx, "x", 0)
	assert.ErrorIs(t, err, sentinel)
	assert.ErrorIs(t, a.DeleteBucket(ctx, "x"), sentinel)
	assert.ErrorIs(t, a.TruncateBucket(ctx, "x"), sentinel)
	_, err = a.ListMeasurements(ctx, "x")
	assert.ErrorIs(t, err, sentinel)
	assert.ErrorIs(t, a.DeleteMeasurement(ctx, "x", "cpu"), sentinel)
}

// ===== nil client 가드 =====

func TestInfluxDBAgent_관리메서드_nil클라이언트_에러(t *testing.T) {
	a := newTestInfluxDBAgent(mockInfluxClientNil())
	// client 를 nil 로 강제하여 미연결 상태를 재현한다.
	a.mu.Lock()
	a.client = nil
	a.mu.Unlock()
	ctx := context.Background()

	_, err := a.ListBuckets(ctx)
	assert.ErrorIs(t, err, errClientNotInitialized)
	_, err = a.CreateBucket(ctx, "x", 0)
	assert.ErrorIs(t, err, errClientNotInitialized)
	assert.ErrorIs(t, a.DeleteBucket(ctx, "x"), errClientNotInitialized)
	assert.ErrorIs(t, a.TruncateBucket(ctx, "x"), errClientNotInitialized)
	_, err = a.ListMeasurements(ctx, "x")
	assert.ErrorIs(t, err, errClientNotInitialized)
	assert.ErrorIs(t, a.DeleteMeasurement(ctx, "x", "cpu"), errClientNotInitialized)
}

// mockInfluxClientNil 은 nil-client 테스트 진입점을 만들기 위한 헬퍼이다.
func mockInfluxClientNil() *mockInfluxClient { return &mockInfluxClient{} }

// ===== v3 어댑터 스텁 =====

func TestInfluxV3Client_관리메서드_미지원(t *testing.T) {
	c := &influxV3Client{}
	ctx := context.Background()

	_, err := c.ListBuckets(ctx)
	assert.ErrorIs(t, err, ErrManagementNotSupported)

	_, err = c.CreateBucket(ctx, "x", 0)
	assert.ErrorIs(t, err, ErrManagementNotSupported)

	assert.ErrorIs(t, c.DeleteBucket(ctx, "x"), ErrManagementNotSupported)
	assert.ErrorIs(t, c.TruncateBucket(ctx, "x"), ErrManagementNotSupported)

	_, err = c.ListMeasurements(ctx, "x")
	assert.ErrorIs(t, err, ErrManagementNotSupported)

	assert.ErrorIs(t, c.DeleteMeasurement(ctx, "x", "cpu"), ErrManagementNotSupported)
}
