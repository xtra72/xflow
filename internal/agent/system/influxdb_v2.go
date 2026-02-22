package system

import (
	"context"
	"fmt"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

// influxV2Client 는 InfluxDB 2.x 클라이언트 어댑터이다.
type influxV2Client struct {
	client influxdb2.Client
	org    string
	bucket string
}

// newInfluxV2Client 는 InfluxDB 2.x 클라이언트를 생성한다.
func newInfluxV2Client(cfg InfluxDBConfig) (*influxV2Client, error) {
	client := influxdb2.NewClient(cfg.URL, cfg.Token)

	return &influxV2Client{
		client: client,
		org:    cfg.Org,
		bucket: cfg.Bucket,
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
			p.SetTime(time.Unix(0, *d.Timestamp))
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
