package system

import (
	"context"
	"fmt"
	"time"

	"github.com/InfluxCommunity/influxdb3-go/v2/influxdb3"
)

// influxV3Client 는 InfluxDB 3.x 클라이언트 어댑터이다.
type influxV3Client struct {
	client   *influxdb3.Client
	database string
}

// newInfluxV3Client 는 InfluxDB 3.x 클라이언트를 생성한다.
func newInfluxV3Client(cfg InfluxDBConfig) (*influxV3Client, error) {
	client, err := influxdb3.New(influxdb3.ClientConfig{
		Host:         cfg.URL,
		Token:        cfg.Token,
		Database:     cfg.Bucket,
		Organization: cfg.Org,
	})
	if err != nil {
		return nil, fmt.Errorf("influxdb v3 client: %w", err)
	}

	return &influxV3Client{
		client:   client,
		database: cfg.Bucket,
	}, nil
}

// Write 는 InfluxDB 3.x 에 데이터를 쓴다.
func (c *influxV3Client) Write(ctx context.Context, data []WriteData) error {
	points := make([]*influxdb3.Point, 0, len(data))
	for _, d := range data {
		p := influxdb3.NewPointWithMeasurement(d.Measurement)

		for k, v := range d.Tags {
			p.SetTag(k, v)
		}
		for k, v := range d.Fields {
			switch val := v.(type) {
			case float64:
				p.SetDoubleField(k, val)
			case int:
				p.SetIntegerField(k, int64(val))
			case int64:
				p.SetIntegerField(k, val)
			case string:
				p.SetStringField(k, val)
			case bool:
				p.SetBooleanField(k, val)
			default:
				p.SetField(k, val)
			}
		}
		if d.Timestamp != nil {
			p.SetTimestamp(time.Unix(0, *d.Timestamp))
		} else {
			p.SetTimestamp(time.Now())
		}

		points = append(points, p)
	}

	return c.client.WritePoints(ctx, points)
}

// Query 는 InfluxDB 3.x 에 쿼리를 실행한다.
func (c *influxV3Client) Query(ctx context.Context, query string, lang string) ([]map[string]any, error) {
	switch lang {
	case "sql":
		return c.querySQL(ctx, query)
	case "influxql":
		return c.queryInfluxQL(ctx, query)
	case "flux":
		return nil, fmt.Errorf("influxdb v3: Flux 는 지원하지 않습니다. 'sql' 또는 'influxql' 을 사용하세요")
	default:
		return nil, fmt.Errorf("influxdb v3: 지원하지 않는 쿼리 언어 %q", lang)
	}
}

// querySQL 는 SQL 쿼리를 실행한다.
func (c *influxV3Client) querySQL(ctx context.Context, query string) ([]map[string]any, error) {
	iterator, err := c.client.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("influxdb v3 sql query: %w", err)
	}

	return c.iteratorToMaps(iterator)
}

// queryInfluxQL 은 InfluxQL 쿼리를 실행한다.
func (c *influxV3Client) queryInfluxQL(ctx context.Context, query string) ([]map[string]any, error) {
	iterator, err := c.client.Query(ctx, query, influxdb3.WithQueryType(influxdb3.InfluxQL))
	if err != nil {
		return nil, fmt.Errorf("influxdb v3 influxql query: %w", err)
	}

	return c.iteratorToMaps(iterator)
}

// iteratorToMaps 는 QueryIterator 의 결과를 []map[string]any 로 변환한다.
func (c *influxV3Client) iteratorToMaps(iterator *influxdb3.QueryIterator) ([]map[string]any, error) {
	var rows []map[string]any
	for iterator.Next() {
		value := iterator.Value()
		row := make(map[string]any, len(value))
		for k, v := range value {
			row[k] = v
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// Health 는 InfluxDB 3.x 의 헬스체크를 실행한다.
// InfluxDB 3.x 에는 전용 헬스 엔드포인트가 없으므로 클라이언트 상태를 확인한다.
func (c *influxV3Client) Health(_ context.Context) error {
	if c.client == nil {
		return fmt.Errorf("influxdb v3: 클라이언트가 nil 입니다")
	}
	return nil
}

// Close 는 InfluxDB 3.x 클라이언트를 닫는다.
func (c *influxV3Client) Close() error {
	return c.client.Close()
}
