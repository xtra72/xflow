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

// TestInfluxV3Client_관리메서드_미지원 은 v3 가 지원하지 않는 관리 조작 **5 종**을
// 잠근다.
//
// [SPEC-TSDB-002 §2.10 반전] 이전에는 6 종이었고 ListMeasurements 가 그중
// 하나였다. measurement 목록 조회는 관리 조작이 아니라 스키마 조회이며 v3 의
// SHOW MEASUREMENTS 로 실제 동작하므로, 그 단언만 제거하고 아래
// TestInfluxV3Client_ListMeasurements_501해제 로 반전했다. 나머지 5 종의 501 은
// 그대로 유지한다 — 본 SPEC 은 읽기 전용 디스커버리만 다룬다(spec.md §1.3).
func TestInfluxV3Client_관리메서드_미지원(t *testing.T) {
	c := &influxV3Client{}
	ctx := context.Background()

	_, err := c.ListBuckets(ctx)
	assert.ErrorIs(t, err, ErrManagementNotSupported)

	_, err = c.CreateBucket(ctx, "x", 0)
	assert.ErrorIs(t, err, ErrManagementNotSupported)

	assert.ErrorIs(t, c.DeleteBucket(ctx, "x"), ErrManagementNotSupported)
	assert.ErrorIs(t, c.TruncateBucket(ctx, "x"), ErrManagementNotSupported)

	assert.ErrorIs(t, c.DeleteMeasurement(ctx, "x", "cpu"), ErrManagementNotSupported)
}

// TestInfluxV3Client_ListMeasurements_501해제 는 위 테스트에서 빠진 한 종의
// 반전을 명시적으로 잠근다(spec.md §2.10 D1 · acceptance.md AC-32).
//
// 단언은 "ErrManagementNotSupported 가 아니다" 이다. 실제 조회는 네트워크를
// 타므로 여기서는 실패하지만, 그 실패가 미지원 센티넬이어서는 안 된다 — 그것이
// 501 을 만들던 원인이다. 쿼리 문자열과 컬럼 파싱은 influxdb_schema_test.go 가
// 네트워크 없이 검증한다.
func TestInfluxV3Client_ListMeasurements_501해제(t *testing.T) {
	c := &influxV3Client{}
	_, err := c.ListMeasurements(context.Background(), "x")
	assert.NotErrorIs(t, err, ErrManagementNotSupported,
		"v3 의 measurement 목록 조회는 SHOW MEASUREMENTS 로 동작해야 한다")
}
