// client_mock.go — fixture-driven SchemaClient mock for tests.
//
// MockClient 는 실제 Influx 서버 없이 schema 응답을 반환하는 테스트 전용
// 구현이다. 본 마이그레이션 도구는 절대 실제 Influx 인스턴스에 접근해서는
// 안 되므로, 모든 통합 테스트는 본 mock 을 사용한다 (안전 가드).
package tsdbtags

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
)

// MockSchema 는 fixture JSON 의 직렬화 표현이다.
//
// JSON 구조 예시:
//
//	{
//	  "version": "v2",
//	  "measurements": ["indoor_temp", "outdoor_temp"],
//	  "tags_by_measurement": {
//	    "indoor_temp": {
//	      "device_id": ["lgcnp:81", "lgcnp:82", "a58ba668-...-..."],
//	      "agent":     ["lgcnp"]
//	    }
//	  }
//	}
type MockSchema struct {
	Version           Target                         `json:"version"`
	Measurements      []string                       `json:"measurements"`
	TagsByMeasurement map[string]map[string][]string `json:"tags_by_measurement"`
	// PingErr / FailingMeasurement 는 에러 시나리오 테스트용 (옵션).
	PingErr            string `json:"ping_err,omitempty"`
	FailingMeasurement string `json:"failing_measurement,omitempty"`
}

// MockClient 는 SchemaClient 의 fixture 기반 구현이다.
type MockClient struct {
	schema MockSchema
	closed bool
}

// NewMockClient 는 schema fixture 로부터 MockClient 를 생성한다.
func NewMockClient(schema MockSchema) *MockClient {
	return &MockClient{schema: schema}
}

// NewMockClientFromFile 은 testdata JSON 파일에서 MockClient 를 로드한다.
func NewMockClientFromFile(path string) (*MockClient, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("mock schema 파일 읽기 실패 (%s): %w", path, err)
	}
	var s MockSchema
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("mock schema JSON 파싱 실패 (%s): %w", path, err)
	}
	if s.Version == "" {
		return nil, fmt.Errorf("mock schema (%s) 에 version 필드가 비어 있습니다", path)
	}
	return NewMockClient(s), nil
}

// Version 은 fixture 에서 선언된 target 을 반환한다.
func (m *MockClient) Version() Target { return m.schema.Version }

// Ping 은 fixture 의 PingErr 가 비어 있으면 nil 을 반환한다.
func (m *MockClient) Ping(_ context.Context) error {
	if m.closed {
		return errors.New("mock client 가 이미 닫혔습니다")
	}
	if m.schema.PingErr != "" {
		return errors.New(m.schema.PingErr)
	}
	return nil
}

// ListMeasurements 는 fixture 의 measurement 리스트를 정렬해 반환한다.
func (m *MockClient) ListMeasurements(_ context.Context) ([]string, error) {
	if m.closed {
		return nil, errors.New("mock client 가 이미 닫혔습니다")
	}
	out := make([]string, len(m.schema.Measurements))
	copy(out, m.schema.Measurements)
	sort.Strings(out)
	return out, nil
}

// ListTagKeys 는 특정 measurement 의 tag key 들을 정렬해 반환한다.
func (m *MockClient) ListTagKeys(_ context.Context, measurement string) ([]string, error) {
	if m.closed {
		return nil, errors.New("mock client 가 이미 닫혔습니다")
	}
	if measurement == m.schema.FailingMeasurement {
		return nil, fmt.Errorf("mock: measurement %q 조회 실패 시뮬레이션", measurement)
	}
	tags, ok := m.schema.TagsByMeasurement[measurement]
	if !ok {
		return []string{}, nil
	}
	out := make([]string, 0, len(tags))
	for k := range tags {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

// ListTagValues 는 (measurement, tagKey) 의 tag value 들을 정렬해 반환한다.
func (m *MockClient) ListTagValues(_ context.Context, measurement, tagKey string) ([]string, error) {
	if m.closed {
		return nil, errors.New("mock client 가 이미 닫혔습니다")
	}
	tags, ok := m.schema.TagsByMeasurement[measurement]
	if !ok {
		return []string{}, nil
	}
	values, ok := tags[tagKey]
	if !ok {
		return []string{}, nil
	}
	out := make([]string, len(values))
	copy(out, values)
	sort.Strings(out)
	return out, nil
}

// Close 는 mock client 를 닫음 상태로 표시한다.
func (m *MockClient) Close() error {
	m.closed = true
	return nil
}
