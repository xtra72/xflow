package node

import (
	"context"
	"encoding/json"
	"fmt"
)

// influxdbAgent 는 InfluxDB 에이전트의 최소 인터페이스이다.
type influxdbAgent interface {
	Process(data []byte) ([]byte, error)
}

// influxdbMessageReceiver 는 쿼리 결과 비동기 수신 인터페이스이다.
// influxdb-read / influxdb-query 노드가 사용한다.
type influxdbMessageReceiver interface {
	influxdbAgent
	ReceiveMessage(ctx context.Context) ([]byte, error)
}

// influxdbWriteData 는 InfluxDB 에 쓰기 위한 데이터 구조체이다.
type influxdbWriteData struct {
	Measurement string            `json:"measurement"`
	Tags        map[string]string `json:"tags,omitempty"`
	Fields      map[string]any    `json:"fields"`
	Timestamp   *int64            `json:"timestamp,omitempty"`
}

// influxdbBackend 는 storage-write 노드의 InfluxDB 백엔드이다.
//
// 매핑 규약 (중립 어휘 → InfluxDB 개념):
//   - series_key    → measurement
//   - values[].name → field 이름
//   - tags          → InfluxDB 태그
//   - timestamp     → point 타임스탬프 (epoch ms)
//
// data_type 은 InfluxDB 가 값의 Go 타입으로 필드 타입을 결정하므로 사용하지 않는다.
type influxdbBackend struct {
	agent     influxdbAgent
	boolToInt bool
}

// backendName 은 에러 메시지에 쓰일 백엔드 이름이다.
func (b *influxdbBackend) backendName() string { return "influxdb" }

// resolve 는 InfluxDB 에이전트를 쓰기 핸들로 확보한다.
func (b *influxdbBackend) resolve(underlying any, cfg *storageWriteConfig) error {
	agent, ok := underlying.(influxdbAgent)
	if !ok {
		return fmt.Errorf("agent does not implement influxdbAgent")
	}
	b.agent = agent
	b.boolToInt = cfg.boolToInt
	return nil
}

// write 는 배치를 하나의 InfluxDB point 로 만들어 에이전트에 전달한다.
// 같은 series_key + timestamp 의 값들이 한 point 의 여러 field 로 묶인다.
func (b *influxdbBackend) write(_ context.Context, batch storageBatch) error {
	if b.agent == nil {
		return fmt.Errorf("agent not configured")
	}

	fields := make(map[string]any, len(batch.values))
	for _, v := range batch.values {
		// 복합 값은 InfluxDB 필드로 쓸 수 없으므로 JSON 문자열로 변환한다.
		fv := scalarFieldValue(v.value)
		if b.boolToInt {
			if bv, ok := fv.(bool); ok {
				if bv {
					fv = 1
				} else {
					fv = 0
				}
			}
		}
		fields[v.name] = fv
	}
	if len(fields) == 0 {
		return nil
	}

	ts := batch.timestamp
	wd := influxdbWriteData{
		Measurement: batch.seriesKey,
		Tags:        batch.tags,
		Fields:      fields,
		Timestamp:   &ts,
	}

	data, err := json.Marshal(wd)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}
	if _, err := b.agent.Process(data); err != nil {
		return err
	}
	return nil
}
