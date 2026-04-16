package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// fakeAgentLookup 은 AgentLookup 인터페이스를 구현하는 테스트용 페이크이다.
// List() 결과는 생성자에 주입된 슬라이스 그대로 반환한다.
type fakeAgentLookup struct {
	agents []agent.Agent
}

func (f *fakeAgentLookup) List() []agent.Agent { return f.agents }

// fakeAgentCommon 은 agent.Agent 인터페이스의 공통 no-op 메서드를 제공한다.
// 각 특화 페이크(fakeStoreAgent, fakeInfluxDBAgent)가 이를 임베딩하여 사용한다.
type fakeAgentCommon struct {
	id, name, typ string
}

func newFakeAgent(id, name, typ string) *fakeAgentCommon {
	return &fakeAgentCommon{id: id, name: name, typ: typ}
}

func (f *fakeAgentCommon) ID() string                          { return f.id }
func (f *fakeAgentCommon) Name() string                        { return f.name }
func (f *fakeAgentCommon) Type() string                        { return f.typ }
func (f *fakeAgentCommon) Init(_ agent.AgentConfig) error      { return nil }
func (f *fakeAgentCommon) Start(_ context.Context) error       { return nil }
func (f *fakeAgentCommon) Stop(_ context.Context) error        { return nil }
func (f *fakeAgentCommon) Pause(_ context.Context) error       { return nil }
func (f *fakeAgentCommon) Resume(_ context.Context) error      { return nil }
func (f *fakeAgentCommon) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (f *fakeAgentCommon) Process(_ []byte) ([]byte, error)    { return nil, nil }
func (f *fakeAgentCommon) Configure(_ agent.AgentConfig) error { return nil }
func (f *fakeAgentCommon) Info() agent.AgentInfo {
	return agent.AgentInfo{ID: f.id, Name: f.name, Type: f.typ}
}
func (f *fakeAgentCommon) Stats() agent.StatsSnapshot { return agent.StatsSnapshot{} }

// --- 응답 디코딩 헬퍼 ---

// queryEntry 는 표준 응답의 entries 항목 구조이다.
type queryEntry struct {
	Timestamp int64             `json:"timestamp"`
	Value     any               `json:"value"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// queryDataEnvelope 는 표준 응답의 data 부분 구조이다.
type queryDataEnvelope struct {
	Entries   []queryEntry `json:"entries"`
	Count     int          `json:"count"`
	Truncated bool         `json:"truncated"`
}

// queryResponse 는 표준 응답 전체 엔벨로프이다.
type queryResponse struct {
	Success bool              `json:"success"`
	Data    queryDataEnvelope `json:"data"`
}

// decodeQueryResponse 는 ResponseRecorder 에서 queryResponse 를 파싱한다.
func decodeQueryResponse(t *testing.T, rec *httptest.ResponseRecorder) queryResponse {
	t.Helper()
	var resp queryResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	return resp
}
