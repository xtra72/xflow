package system

import (
	"context"
	"fmt"
	"time"
)

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
