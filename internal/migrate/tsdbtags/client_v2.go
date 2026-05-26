// client_v2.go — InfluxDB v2 read-only schema 어댑터 (Flux 기반).
//
// influxdata/influxdb-client-go/v2 의 QueryAPI 를 사용하여 schema 메타데이터를
// Flux 쿼리로 조회한다. 본 어댑터는 절대 write API (WriteAPI / WriteAPIBlocking)
// 를 호출하지 않는다.
//
// 사용 Flux 함수 (Influx Cloud / OSS v2 표준):
//   - import "influxdata/influxdb/schema"
//   - schema.measurements(bucket: "<bucket>")
//   - schema.measurementTagKeys(bucket: "<bucket>", measurement: "<m>")
//   - schema.measurementTagValues(bucket: "<bucket>", measurement: "<m>", tag: "<k>")
package tsdbtags

import (
	"context"
	"fmt"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
)

// influxV2SchemaClient 는 SchemaClient 의 v2 구현이다.
type influxV2SchemaClient struct {
	client influxdb2.Client
	org    string
	bucket string
}

// NewV2SchemaClient 는 v2 Influx 서버에 연결되는 read-only SchemaClient 를 생성한다.
//
// caller 는 사용 후 반드시 Close() 를 호출해야 한다 (HTTP 연결 해제).
func NewV2SchemaClient(url, token, org, bucket string) SchemaClient {
	c := influxdb2.NewClient(url, token)
	return &influxV2SchemaClient{client: c, org: org, bucket: bucket}
}

// Version 은 항상 TargetV2 를 반환한다.
func (c *influxV2SchemaClient) Version() Target { return TargetV2 }

// Ping 은 Influx 서버의 health endpoint 를 호출한다.
func (c *influxV2SchemaClient) Ping(ctx context.Context) error {
	health, err := c.client.Health(ctx)
	if err != nil {
		return fmt.Errorf("influx v2 health: %w", err)
	}
	if health == nil || health.Status != "pass" {
		return fmt.Errorf("influx v2 health: not pass")
	}
	return nil
}

// ListMeasurements 는 schema.measurements() 로 measurement 목록을 조회한다.
func (c *influxV2SchemaClient) ListMeasurements(ctx context.Context) ([]string, error) {
	q := fmt.Sprintf(`import "influxdata/influxdb/schema"
schema.measurements(bucket: %q)`, c.bucket)
	return c.queryStringColumn(ctx, q, "_value")
}

// ListTagKeys 는 schema.measurementTagKeys() 로 tag key 목록을 조회한다.
func (c *influxV2SchemaClient) ListTagKeys(ctx context.Context, measurement string) ([]string, error) {
	q := fmt.Sprintf(`import "influxdata/influxdb/schema"
schema.measurementTagKeys(bucket: %q, measurement: %q)`, c.bucket, measurement)
	keys, err := c.queryStringColumn(ctx, q, "_value")
	if err != nil {
		return nil, err
	}
	// _start / _stop / _measurement / _field 등의 내부 column 은 제외.
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if len(k) > 0 && k[0] == '_' {
			continue
		}
		out = append(out, k)
	}
	return out, nil
}

// ListTagValues 는 schema.measurementTagValues() 로 tag value 목록을 조회한다.
func (c *influxV2SchemaClient) ListTagValues(ctx context.Context, measurement, tagKey string) ([]string, error) {
	q := fmt.Sprintf(`import "influxdata/influxdb/schema"
schema.measurementTagValues(bucket: %q, measurement: %q, tag: %q)`,
		c.bucket, measurement, tagKey)
	return c.queryStringColumn(ctx, q, "_value")
}

// Close 는 v2 client 의 연결을 해제한다.
func (c *influxV2SchemaClient) Close() error {
	c.client.Close()
	return nil
}

// queryStringColumn 은 Flux 쿼리 결과의 특정 column 을 string slice 로 반환한다.
//
// schema.* 함수들은 모두 _value column 에 결과를 담아 반환한다 (string).
func (c *influxV2SchemaClient) queryStringColumn(ctx context.Context, query, column string) ([]string, error) {
	queryAPI := c.client.QueryAPI(c.org)
	result, err := queryAPI.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("influx v2 schema query: %w", err)
	}
	defer closeQueryResult(result)

	var out []string
	for result.Next() {
		rec := result.Record()
		if rec == nil {
			continue
		}
		v := rec.ValueByKey(column)
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	if result.Err() != nil {
		return nil, fmt.Errorf("influx v2 schema query result: %w", result.Err())
	}
	return out, nil
}

// closeQueryResult 는 QueryTableResult 의 Close 가 없을 때 (특정 버전) graceful no-op.
func closeQueryResult(_ *api.QueryTableResult) {
	// influxdb-client-go v2 의 QueryTableResult 는 Close 를 노출하지 않는다.
	// 명시적 close 가 필요 없는 패턴 — 자동 lifecycle.
}
