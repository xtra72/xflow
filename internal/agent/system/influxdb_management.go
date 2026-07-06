package system

import (
	"context"
	"fmt"
)

// InfluxManager 는 InfluxDB 에이전트가 제공하는 bucket/measurement 관리 계약이다.
// HTTP 핸들러가 타입 단언을 통해 이 인터페이스로 에이전트를 검증한다.
// *InfluxDBAgent 가 이 인터페이스를 만족한다.
type InfluxManager interface {
	ListBuckets(ctx context.Context) ([]BucketInfo, error)
	CreateBucket(ctx context.Context, name string, retentionSeconds int64) (BucketInfo, error)
	DeleteBucket(ctx context.Context, bucketRef string) error
	TruncateBucket(ctx context.Context, bucketRef string) error
	ListMeasurements(ctx context.Context, bucket string) ([]string, error)
	DeleteMeasurement(ctx context.Context, bucket, measurement string) error
}

// 컴파일 타임 인터페이스 준수 체크.
var _ InfluxManager = (*InfluxDBAgent)(nil)

// errClientNotInitialized 는 클라이언트 미초기화(미연결) 상태에서 관리 조작을
// 시도할 때 반환된다.
var errClientNotInitialized = fmt.Errorf("influxdb: client is not initialized")

// managementClient 는 client 가 초기화되어 있는지 확인하고 반환한다.
// nil 이면 errClientNotInitialized 를 반환하여 호출자가 명확한 에러를 전파하게 한다.
func (a *InfluxDBAgent) managementClient() (InfluxClient, error) {
	a.mu.RLock()
	c := a.client
	a.mu.RUnlock()
	if c == nil {
		return nil, errClientNotInitialized
	}
	return c, nil
}

// ListBuckets 는 client 에 위임하여 버킷 목록을 반환한다.
func (a *InfluxDBAgent) ListBuckets(ctx context.Context) ([]BucketInfo, error) {
	c, err := a.managementClient()
	if err != nil {
		return nil, err
	}
	return c.ListBuckets(ctx)
}

// CreateBucket 은 client 에 위임하여 버킷을 생성한다.
func (a *InfluxDBAgent) CreateBucket(ctx context.Context, name string, retentionSeconds int64) (BucketInfo, error) {
	c, err := a.managementClient()
	if err != nil {
		return BucketInfo{}, err
	}
	return c.CreateBucket(ctx, name, retentionSeconds)
}

// DeleteBucket 은 client 에 위임하여 버킷을 삭제한다.
func (a *InfluxDBAgent) DeleteBucket(ctx context.Context, bucketRef string) error {
	c, err := a.managementClient()
	if err != nil {
		return err
	}
	return c.DeleteBucket(ctx, bucketRef)
}

// TruncateBucket 은 client 에 위임하여 버킷 데이터를 초기화한다.
func (a *InfluxDBAgent) TruncateBucket(ctx context.Context, bucketRef string) error {
	c, err := a.managementClient()
	if err != nil {
		return err
	}
	return c.TruncateBucket(ctx, bucketRef)
}

// ListMeasurements 는 client 에 위임하여 measurement 목록을 반환한다.
// bucket 이 비어 있으면 에이전트 설정의 기본 버킷을 사용한다.
func (a *InfluxDBAgent) ListMeasurements(ctx context.Context, bucket string) ([]string, error) {
	c, err := a.managementClient()
	if err != nil {
		return nil, err
	}
	if bucket == "" {
		bucket = a.defaultBucket()
	}
	return c.ListMeasurements(ctx, bucket)
}

// DeleteMeasurement 는 client 에 위임하여 measurement 데이터를 삭제한다.
// bucket 이 비어 있으면 에이전트 설정의 기본 버킷을 사용한다.
func (a *InfluxDBAgent) DeleteMeasurement(ctx context.Context, bucket, measurement string) error {
	c, err := a.managementClient()
	if err != nil {
		return err
	}
	if bucket == "" {
		bucket = a.defaultBucket()
	}
	return c.DeleteMeasurement(ctx, bucket, measurement)
}

// defaultBucket 은 에이전트 설정의 기본 버킷명을 반환한다.
func (a *InfluxDBAgent) defaultBucket() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.influxConfig.Bucket
}
