package node

import (
	"context"
)

// mockInfluxDBAgent 는 테스트용 InfluxDB 에이전트 모의 객체이다.
// influxdb-read / influxdb-query / storage-write(influxdb 백엔드) 테스트가 공유한다.
type mockInfluxDBAgent struct {
	processFunc func(data []byte) ([]byte, error)
}

func (m *mockInfluxDBAgent) Process(data []byte) ([]byte, error) {
	return m.processFunc(data)
}

// Type 은 storage-write 의 백엔드 선택(agentTyper)을 만족시키기 위한 것이다.
func (m *mockInfluxDBAgent) Type() string { return "influxdb" }

// mockInfluxDBReceiver 는 테스트용 InfluxDB 수신 에이전트 모의 객체이다.
type mockInfluxDBReceiver struct {
	mockInfluxDBAgent
	receiveFunc func(ctx context.Context) ([]byte, error)
}

func (m *mockInfluxDBReceiver) ReceiveMessage(ctx context.Context) ([]byte, error) {
	return m.receiveFunc(ctx)
}
