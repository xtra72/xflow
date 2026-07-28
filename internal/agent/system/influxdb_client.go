package system

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrManagementNotSupported 는 클라이언트가 bucket/measurement 관리 조작을
// 지원하지 않을 때 반환하는 센티넬 에러이다.
//
// InfluxDB 3.x 어댑터는 관리 API 를 제공하지 않으므로 모든 관리 메서드가
// 이 에러를 반환한다. HTTP 핸들러는 errors.Is 로 이 에러를 감지하여
// 501 Not Implemented 로 매핑한다.
var ErrManagementNotSupported = errors.New("influxdb: management operations are not supported by this client")

// BucketInfo 는 InfluxDB 버킷(v2) 의 메타데이터를 나타낸다.
//
// RetentionSeconds 는 데이터 보존 기간(초) 이며, 0 은 무제한 보존을 의미한다.
type BucketInfo struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	OrgID            string `json:"org_id"`
	RetentionSeconds int64  `json:"retention_seconds"`
}

// WriteData 는 InfluxDB 에 쓰기 위한 데이터 구조체이다.
//
// Timestamp 의 단위는 에이전트의 `Precision` 설정 ("ns" / "us" / "ms" / "s") 을 따른다.
// v0.16.5 부터 기본 Precision 은 "ms" (이전엔 "ns") — node 가 msg.Timestamp().UnixMilli()
// 를 전달하기 때문.
type WriteData struct {
	Measurement string            `json:"measurement"`
	Tags        map[string]string `json:"tags,omitempty"`
	Fields      map[string]any    `json:"fields"`
	Timestamp   *int64            `json:"timestamp,omitempty"`
}

// timestampToTime 은 epoch 정수 (precision 단위) 를 time.Time 으로 변환한다 (v0.16.5).
//
// 이전 버그: time.Unix(0, ts) 가 ts 를 ns 로 해석 → ms 값이 1970-01-30 으로 변환됨.
func timestampToTime(ts int64, precision string) time.Time {
	switch precision {
	case "s":
		return time.Unix(ts, 0)
	case "us":
		return time.UnixMicro(ts)
	case "ns":
		return time.Unix(0, ts)
	case "ms":
		fallthrough
	default:
		// 기본: epoch ms (node 가 msg.Timestamp().UnixMilli() 를 보냄).
		return time.UnixMilli(ts)
	}
}

// QueryRequest 는 쿼리 요청을 나타낸다.
type QueryRequest struct {
	Query    string `json:"query"`
	Language string `json:"language,omitempty"`
}

// InfluxClient 는 InfluxDB 2.x/3.x 의 조작을 추상화하는 인터페이스이다.
type InfluxClient interface {
	Write(ctx context.Context, data []WriteData) error
	Query(ctx context.Context, query string, lang string) ([]map[string]any, error)
	Health(ctx context.Context) error
	Close() error

	// --- 관리 조작 (v2 지원, v3 는 ErrManagementNotSupported 반환) ---

	// ListBuckets 는 조직 내 모든 버킷 목록을 반환한다.
	ListBuckets(ctx context.Context) ([]BucketInfo, error)

	// CreateBucket 은 새 버킷을 생성한다.
	// retentionSeconds 가 0 이하이면 무제한 보존으로 생성한다.
	CreateBucket(ctx context.Context, name string, retentionSeconds int64) (BucketInfo, error)

	// DeleteBucket 은 이름 또는 ID 로 지정된 버킷을 삭제한다.
	DeleteBucket(ctx context.Context, bucketRef string) error

	// TruncateBucket 은 버킷의 모든 데이터를 삭제하되 버킷 자체는 유지한다(초기화).
	TruncateBucket(ctx context.Context, bucketRef string) error

	// ListMeasurements 는 지정된 버킷의 measurement 이름 목록을 반환한다.
	ListMeasurements(ctx context.Context, bucket string) ([]string, error)

	// DeleteMeasurement 는 버킷에서 지정된 measurement 의 모든 데이터를 삭제한다.
	DeleteMeasurement(ctx context.Context, bucket, measurement string) error
}

// NewInfluxClient 는 설정에 따라 적절한 InfluxDB 클라이언트를 생성한다.
func NewInfluxClient(cfg InfluxDBConfig) (InfluxClient, error) {
	switch cfg.Version {
	case "2":
		return newInfluxV2Client(cfg)
	case "3":
		return newInfluxV3Client(cfg)
	default:
		return nil, fmt.Errorf("influxdb: 지원하지 않는 버전 %q ('2' 또는 '3'을 지정하세요)", cfg.Version)
	}
}
