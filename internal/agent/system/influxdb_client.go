package system

import (
	"context"
	"fmt"
)

// WriteData 는 InfluxDB 에 쓰기 위한 데이터 구조체이다.
type WriteData struct {
	Measurement string            `json:"measurement"`
	Tags        map[string]string `json:"tags,omitempty"`
	Fields      map[string]any    `json:"fields"`
	Timestamp   *int64            `json:"timestamp,omitempty"`
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
