// client_v3.go — InfluxDB v3 read-only schema 어댑터 (InfluxQL 기반).
//
// InfluxCommunity/influxdb3-go 의 Query API 를 사용한다. v3 는 SQL 이 기본이지만
// schema 메타데이터 조회는 InfluxQL 의 SHOW MEASUREMENTS / SHOW TAG KEYS /
// SHOW TAG VALUES 가 더 직관적이다 (SQL 의 information_schema 도 가능하나
// 환경에 따라 미지원).
//
// 본 어댑터는 write API (WritePoints / Write) 를 절대 호출하지 않는다.
package tsdbtags

import (
	"context"
	"fmt"

	"github.com/InfluxCommunity/influxdb3-go/v2/influxdb3"
)

// influxV3SchemaClient 는 SchemaClient 의 v3 구현이다.
type influxV3SchemaClient struct {
	client   *influxdb3.Client
	database string
}

// NewV3SchemaClient 는 v3 Influx 서버에 연결되는 read-only SchemaClient 를 생성한다.
//
// caller 는 사용 후 반드시 Close() 를 호출해야 한다.
func NewV3SchemaClient(url, token, database, org string) (SchemaClient, error) {
	c, err := influxdb3.New(influxdb3.ClientConfig{
		Host:         url,
		Token:        token,
		Database:     database,
		Organization: org,
	})
	if err != nil {
		return nil, fmt.Errorf("influx v3 client init: %w", err)
	}
	return &influxV3SchemaClient{client: c, database: database}, nil
}

// Version 은 항상 TargetV3 를 반환한다.
func (c *influxV3SchemaClient) Version() Target { return TargetV3 }

// Ping 은 v3 의 경우 client 의 활성 상태만 확인한다 (전용 health endpoint 부재).
func (c *influxV3SchemaClient) Ping(_ context.Context) error {
	if c.client == nil {
		return fmt.Errorf("influx v3: client is nil")
	}
	return nil
}

// ListMeasurements 는 SHOW MEASUREMENTS 로 measurement 목록을 조회한다.
func (c *influxV3SchemaClient) ListMeasurements(ctx context.Context) ([]string, error) {
	return c.queryStringColumn(ctx, "SHOW MEASUREMENTS", "name")
}

// ListTagKeys 는 SHOW TAG KEYS FROM <measurement> 로 조회한다.
//
// measurement 명은 InfluxQL identifier 로 quoted (보수적).
func (c *influxV3SchemaClient) ListTagKeys(ctx context.Context, measurement string) ([]string, error) {
	q := fmt.Sprintf(`SHOW TAG KEYS FROM "%s"`, escapeInfluxQLIdent(measurement))
	return c.queryStringColumn(ctx, q, "tagKey")
}

// ListTagValues 는 SHOW TAG VALUES FROM <measurement> WITH KEY = "<tagKey>" 로 조회한다.
func (c *influxV3SchemaClient) ListTagValues(ctx context.Context, measurement, tagKey string) ([]string, error) {
	q := fmt.Sprintf(`SHOW TAG VALUES FROM "%s" WITH KEY = "%s"`,
		escapeInfluxQLIdent(measurement),
		escapeInfluxQLIdent(tagKey),
	)
	return c.queryStringColumn(ctx, q, "value")
}

// Close 는 v3 client 의 연결을 해제한다.
func (c *influxV3SchemaClient) Close() error {
	if c.client == nil {
		return nil
	}
	return c.client.Close()
}

// queryStringColumn 은 InfluxQL 쿼리 결과의 특정 column 을 string slice 로 반환한다.
func (c *influxV3SchemaClient) queryStringColumn(ctx context.Context, query, column string) ([]string, error) {
	iter, err := c.client.Query(ctx, query, influxdb3.WithQueryType(influxdb3.InfluxQL))
	if err != nil {
		return nil, fmt.Errorf("influx v3 schema query: %w", err)
	}

	var out []string
	for iter.Next() {
		row := iter.Value()
		if v, ok := row[column]; ok {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out, nil
}

// escapeInfluxQLIdent 는 InfluxQL identifier 의 큰따옴표를 escape 한다.
//
// 보수적 escape — 일반적으로 measurement / tag key 명은 안전한 문자만 포함하나
// 방어적으로 처리한다.
func escapeInfluxQLIdent(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			out = append(out, '\\', '"')
		} else {
			out = append(out, s[i])
		}
	}
	return string(out)
}
