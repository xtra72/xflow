package system

import (
	"context"
	"fmt"
	"strings"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/domain"
)

// truncateEpochStart 는 Truncate/DeleteMeasurement 시 삭제 시간범위의 시작점이다(epoch 0).
var truncateEpochStart = time.Unix(0, 0).UTC()

// truncateFutureStop 은 삭제 시간범위의 종료점이다.
// InfluxDB delete predicate 는 [start, stop) 반열림 구간이므로 충분히 먼 미래를
// 상한으로 사용한다. 3000-01-01 UTC 는 어떤 실제 데이터 타임스탬프보다도 크다.
var truncateFutureStop = time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC)

// influxV2Client 는 InfluxDB 2.x 클라이언트 어댑터이다.
type influxV2Client struct {
	client    influxdb2.Client
	org       string
	bucket    string
	precision string // v0.16.5: WriteData.Timestamp 의 단위 ("ns"/"us"/"ms"/"s").
}

// newInfluxV2Client 는 InfluxDB 2.x 클라이언트를 생성한다.
func newInfluxV2Client(cfg InfluxDBConfig) (*influxV2Client, error) {
	client := influxdb2.NewClient(cfg.URL, cfg.Token)

	return &influxV2Client{
		client:    client,
		org:       cfg.Org,
		bucket:    cfg.Bucket,
		precision: cfg.Precision,
	}, nil
}

// Write 는 InfluxDB 2.x 에 데이터를 쓴다.
func (c *influxV2Client) Write(ctx context.Context, data []WriteData) error {
	writeAPI := c.client.WriteAPIBlocking(c.org, c.bucket)

	for _, d := range data {
		p := influxdb2.NewPointWithMeasurement(d.Measurement)

		for k, v := range d.Tags {
			p.AddTag(k, v)
		}
		for k, v := range d.Fields {
			p.AddField(k, v)
		}
		if d.Timestamp != nil {
			// v0.16.5: Precision 설정에 따라 단위 변환 (이전: 항상 ns 로 해석되던 버그).
			p.SetTime(timestampToTime(*d.Timestamp, c.precision))
		} else {
			p.SetTime(time.Now())
		}

		if err := writeAPI.WritePoint(ctx, p); err != nil {
			return fmt.Errorf("influxdb v2 write: %w", err)
		}
	}

	return nil
}

// Query 는 InfluxDB 2.x 에 쿼리를 실행한다.
func (c *influxV2Client) Query(ctx context.Context, query string, lang string) ([]map[string]any, error) {
	switch lang {
	case "flux", "influxql":
		return c.queryFlux(ctx, query)
	case "sql":
		return nil, fmt.Errorf("influxdb v2: SQL 은 지원하지 않습니다. 'flux' 또는 'influxql' 을 사용하세요")
	default:
		return nil, fmt.Errorf("influxdb v2: 지원하지 않는 쿼리 언어 %q", lang)
	}
}

// queryFlux 는 Flux/InfluxQL 쿼리를 실행한다.
func (c *influxV2Client) queryFlux(ctx context.Context, query string) ([]map[string]any, error) {
	queryAPI := c.client.QueryAPI(c.org)
	result, err := queryAPI.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("influxdb v2 query: %w", err)
	}

	var rows []map[string]any
	for result.Next() {
		row := make(map[string]any)
		for k, v := range result.Record().Values() {
			row[k] = v
		}
		rows = append(rows, row)
	}
	if result.Err() != nil {
		return nil, fmt.Errorf("influxdb v2 query result: %w", result.Err())
	}

	return rows, nil
}

// Health 는 InfluxDB 2.x 의 헬스체크를 실행한다.
func (c *influxV2Client) Health(ctx context.Context) error {
	health, err := c.client.Health(ctx)
	if err != nil {
		return fmt.Errorf("influxdb v2 health: %w", err)
	}
	if health.Status != "pass" {
		msg := ""
		if health.Message != nil {
			msg = *health.Message
		}
		return fmt.Errorf("influxdb v2 health: status=%s, message=%s", health.Status, msg)
	}
	return nil
}

// Close 는 InfluxDB 2.x 클라이언트를 닫는다.
func (c *influxV2Client) Close() error {
	c.client.Close()
	return nil
}

// ListBuckets 는 org 에 속한 모든 버킷 목록을 반환한다.
//
// FindBucketsByOrgName 은 PagingOptions 없이 호출 시 기본 페이지(최대 20개) 만
// 반환하므로, Limit 을 최대값(100) 으로 지정해 offset 페이지네이션으로 전체를 수집한다.
func (c *influxV2Client) ListBuckets(ctx context.Context) ([]BucketInfo, error) {
	bucketsAPI := c.client.BucketsAPI()

	const pageLimit = 100
	var infos []BucketInfo
	for offset := 0; ; offset += pageLimit {
		page, err := bucketsAPI.FindBucketsByOrgName(ctx, c.org,
			api.PagingWithLimit(pageLimit),
			api.PagingWithOffset(offset),
		)
		if err != nil {
			return nil, fmt.Errorf("influxdb v2 list buckets: %w", err)
		}
		if page == nil || len(*page) == 0 {
			break
		}
		for i := range *page {
			infos = append(infos, bucketToInfo(&(*page)[i]))
		}
		if len(*page) < pageLimit {
			break
		}
	}
	return infos, nil
}

// CreateBucket 은 새 버킷을 생성한다.
// retentionSeconds > 0 이면 expire 보존 규칙을 적용하고, 그 외에는 무제한 보존이다.
func (c *influxV2Client) CreateBucket(ctx context.Context, name string, retentionSeconds int64) (BucketInfo, error) {
	if name == "" {
		return BucketInfo{}, fmt.Errorf("influxdb v2 create bucket: name 은 필수입니다")
	}

	org, err := c.client.OrganizationsAPI().FindOrganizationByName(ctx, c.org)
	if err != nil {
		return BucketInfo{}, fmt.Errorf("influxdb v2 create bucket: org 조회 실패 (%q): %w", c.org, err)
	}

	var rules []domain.RetentionRule
	if retentionSeconds > 0 {
		expire := domain.RetentionRuleTypeExpire
		rules = append(rules, domain.RetentionRule{
			EverySeconds: retentionSeconds,
			Type:         &expire,
		})
	}

	created, err := c.client.BucketsAPI().CreateBucketWithName(ctx, org, name, rules...)
	if err != nil {
		return BucketInfo{}, fmt.Errorf("influxdb v2 create bucket %q: %w", name, err)
	}
	return bucketToInfo(created), nil
}

// DeleteBucket 은 이름 또는 ID 로 지정된 버킷을 삭제한다.
// bucketRef 를 먼저 이름으로 조회하고, 실패하면 ID 로 간주해 삭제를 시도한다.
func (c *influxV2Client) DeleteBucket(ctx context.Context, bucketRef string) error {
	if bucketRef == "" {
		return fmt.Errorf("influxdb v2 delete bucket: bucket 은 필수입니다")
	}

	bucketsAPI := c.client.BucketsAPI()

	// 먼저 이름으로 조회 — 성공하면 해당 버킷을 삭제한다.
	if b, err := bucketsAPI.FindBucketByName(ctx, bucketRef); err == nil && b != nil {
		if err := bucketsAPI.DeleteBucket(ctx, b); err != nil {
			return fmt.Errorf("influxdb v2 delete bucket %q: %w", bucketRef, err)
		}
		return nil
	}

	// 이름 조회 실패 시 ID 로 간주해 삭제를 시도한다.
	if err := bucketsAPI.DeleteBucketWithID(ctx, bucketRef); err != nil {
		return fmt.Errorf("influxdb v2 delete bucket (id=%q): %w", bucketRef, err)
	}
	return nil
}

// TruncateBucket 은 버킷의 모든 데이터를 삭제하되 버킷 자체는 유지한다(초기화).
// predicate 를 빈 문자열로 두어 [epoch, 먼 미래) 구간의 전 데이터를 삭제한다.
func (c *influxV2Client) TruncateBucket(ctx context.Context, bucketRef string) error {
	if bucketRef == "" {
		return fmt.Errorf("influxdb v2 truncate bucket: bucket 은 필수입니다")
	}

	bucketName, err := c.resolveBucketName(ctx, bucketRef)
	if err != nil {
		return err
	}

	if err := c.client.DeleteAPI().DeleteWithName(ctx, c.org, bucketName,
		truncateEpochStart, truncateFutureStop, ""); err != nil {
		return fmt.Errorf("influxdb v2 truncate bucket %q: %w", bucketName, err)
	}
	return nil
}

// ListMeasurements 는 지정된 버킷의 measurement 이름 목록을 반환한다.
// Flux 의 schema.measurements() 를 실행해 _value 컬럼을 수집한다.
// bucket 이 빈 문자열이면 설정된 기본 버킷을 사용한다.
func (c *influxV2Client) ListMeasurements(ctx context.Context, bucket string) ([]string, error) {
	if bucket == "" {
		bucket = c.bucket
	}
	if bucket == "" {
		return nil, fmt.Errorf("influxdb v2 list measurements: bucket 은 필수입니다")
	}

	flux := fmt.Sprintf(
		"import \"influxdata/influxdb/schema\"\nschema.measurements(bucket: %q)",
		bucket,
	)

	rows, err := c.queryFlux(ctx, flux)
	if err != nil {
		return nil, fmt.Errorf("influxdb v2 list measurements (bucket=%q): %w", bucket, err)
	}

	measurements := make([]string, 0, len(rows))
	for _, row := range rows {
		if v, ok := row[fluxValueColumn]; ok {
			if s, ok := v.(string); ok && s != "" {
				measurements = append(measurements, s)
			}
		}
	}
	return measurements, nil
}

// DeleteMeasurement 는 버킷에서 지정된 measurement 의 모든 데이터를 삭제한다.
// predicate `_measurement="<name>"` 로 전체 시간범위의 데이터를 삭제한다.
// bucket 이 빈 문자열이면 설정된 기본 버킷을 사용한다.
func (c *influxV2Client) DeleteMeasurement(ctx context.Context, bucket, measurement string) error {
	if measurement == "" {
		return fmt.Errorf("influxdb v2 delete measurement: measurement 는 필수입니다")
	}
	if bucket == "" {
		bucket = c.bucket
	}
	if bucket == "" {
		return fmt.Errorf("influxdb v2 delete measurement: bucket 은 필수입니다")
	}

	// predicate 값의 큰따옴표를 이스케이프해 predicate 구문 손상을 방지한다.
	escaped := strings.ReplaceAll(measurement, `"`, `\"`)
	predicate := fmt.Sprintf(`_measurement="%s"`, escaped)

	if err := c.client.DeleteAPI().DeleteWithName(ctx, c.org, bucket,
		truncateEpochStart, truncateFutureStop, predicate); err != nil {
		return fmt.Errorf("influxdb v2 delete measurement %q (bucket=%q): %w", measurement, bucket, err)
	}
	return nil
}

// resolveBucketName 은 bucketRef(이름 또는 ID) 를 버킷 이름으로 해석한다.
// 이름 조회에 성공하면 그대로 반환하고, 실패하면 ID 조회를 시도한다.
// DeleteAPI.DeleteWithName 은 버킷 "이름" 을 요구하므로 ID 입력도 이름으로 정규화한다.
func (c *influxV2Client) resolveBucketName(ctx context.Context, bucketRef string) (string, error) {
	bucketsAPI := c.client.BucketsAPI()

	if b, err := bucketsAPI.FindBucketByName(ctx, bucketRef); err == nil && b != nil {
		return b.Name, nil
	}

	b, err := bucketsAPI.FindBucketByID(ctx, bucketRef)
	if err != nil || b == nil {
		return "", fmt.Errorf("influxdb v2: bucket %q 를 찾을 수 없습니다: %w", bucketRef, err)
	}
	return b.Name, nil
}

// bucketToInfo 는 domain.Bucket 을 BucketInfo 로 변환한다.
// 첫 번째 expire 보존 규칙의 EverySeconds 를 RetentionSeconds 로 사용하며,
// 규칙이 없으면 0(무제한) 이다.
func bucketToInfo(b *domain.Bucket) BucketInfo {
	if b == nil {
		return BucketInfo{}
	}
	info := BucketInfo{Name: b.Name}
	if b.Id != nil {
		info.ID = *b.Id
	}
	if b.OrgID != nil {
		info.OrgID = *b.OrgID
	}
	for _, r := range b.RetentionRules {
		if r.EverySeconds > 0 {
			info.RetentionSeconds = r.EverySeconds
			break
		}
	}
	return info
}
