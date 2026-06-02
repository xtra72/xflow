package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api/dto"
)

// reactFlowNodeWithSecrets 는 data 에 민감/비민감 필드를 모두 가진 노드를 만든다.
// data 의 비표준 키는 export 시 restoreConfigNesting 에 의해 node["config"] 로
// 재중첩되므로, 민감 키 리댁션 대상이 된다.
func reactFlowNodeWithSecrets(id, label string) map[string]any {
	return map[string]any{
		"id":   id,
		"type": "custom",
		"position": map[string]any{
			"x": 0.0,
			"y": 0.0,
		},
		"data": map[string]any{
			"label":    label,
			"nodeType": label,
			"status":   "draft",
			"enabled":  true,
			// 비민감 config
			"host": "broker.local",
			"port": 1883.0,
			// 민감 config (리댁션 대상)
			"password": "node-secret",
			"token":    "node-token",
			"username": "node-user",
		},
	}
}

// TestExport_RedactsNodeConfigSecrets 는 export 시 노드 config 의 민감 키가
// 제거되고 sensitive_fields 에 기록되는지 확인한다. (requirement 1)
func TestExport_RedactsNodeConfigSecrets(t *testing.T) {
	cfg := reactFlowConfig(
		reactFlowNodeWithSecrets("n1", "mqtt-pub"),
	)
	flowMock := &mockFlowManager{
		getFlowFn: func(_ context.Context, id string) (*FlowInfo, error) {
			return &FlowInfo{ID: id, Name: "flow-secret", Status: "draft", Config: cfg}, nil
		},
	}
	agentsMock := &mockAgentManager{
		listAgentsFn: func(_ context.Context, _ dto.ListOptions) ([]AgentInfo, int64, error) {
			return []AgentInfo{}, 0, nil
		},
	}

	router := setupFlowRouterWithAgents(flowMock, agentsMock)
	rec := doRequest(t, router, http.MethodGet, "/api/v1/flows/flow-secret/export", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// 1) 원시 JSON 문자열에 비밀 "값" 이 절대 등장하지 않아야 한다
	body := rec.Body.String()
	assert.NotContains(t, body, "node-secret", "password 값이 JSON 에 노출되면 안 된다")
	assert.NotContains(t, body, "node-token", "token 값이 JSON 에 노출되면 안 된다")
	assert.NotContains(t, body, "node-user", "username 값이 JSON 에 노출되면 안 된다")

	var resp dto.APIResponse[map[string]any]
	decodeJSON(t, rec, &resp)
	require.True(t, resp.Success)

	def := resp.Data["definition"].(map[string]any)
	nodes := def["nodes"].([]any)
	require.Len(t, nodes, 1)
	node := nodes[0].(map[string]any)

	// 2) 노드 config 에서 민감 키가 제거되고 비민감 키는 보존되어야 한다
	nodeCfg, ok := node["config"].(map[string]any)
	require.True(t, ok, "노드는 config 객체를 가져야 한다")
	assert.NotContains(t, nodeCfg, "password")
	assert.NotContains(t, nodeCfg, "token")
	assert.NotContains(t, nodeCfg, "username")
	assert.Equal(t, "broker.local", nodeCfg["host"], "비민감 키는 보존")

	// 3) sensitive_fields 가 제거된 키를 나열해야 한다
	rawFields, ok := node["sensitive_fields"].([]any)
	require.True(t, ok, "노드는 sensitive_fields 를 가져야 한다")
	fields := make([]string, 0, len(rawFields))
	for _, f := range rawFields {
		fields = append(fields, f.(string))
	}
	// 정렬된 결과: password, token, username
	assert.Equal(t, []string{"password", "token", "username"}, fields)
}

// TestExport_RedactsAgentConfigSecrets 는 export 시 required_agents 의 에이전트
// config 민감 키가 제거되고 sensitive_fields / required_secrets 가 채워지는지
// 확인한다. (requirement 1)
func TestExport_RedactsAgentConfigSecrets(t *testing.T) {
	cfg := reactFlowConfig(
		reactFlowNode("n1", "mqtt-pub", "broker01"),
	)
	flowMock := &mockFlowManager{
		getFlowFn: func(_ context.Context, id string) (*FlowInfo, error) {
			return &FlowInfo{ID: id, Name: "flow-A", Status: "draft", Config: cfg}, nil
		},
	}
	agentsMock := &mockAgentManager{
		listAgentsFn: func(_ context.Context, _ dto.ListOptions) ([]AgentInfo, int64, error) {
			return []AgentInfo{
				{
					ID:   "1",
					Name: "broker01",
					Type: "mqtt",
					Config: map[string]any{
						"host":     "broker.local",
						"port":     1883,
						"password": "agent-secret",
						"api_key":  "ak-123",
						"headers": map[string]any{
							"auth_token": "tok-xyz",
							"accept":     "json",
						},
					},
				},
			}, 1, nil
		},
	}

	router := setupFlowRouterWithAgents(flowMock, agentsMock)
	rec := doRequest(t, router, http.MethodGet, "/api/v1/flows/flow-A/export", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.NotContains(t, body, "agent-secret")
	assert.NotContains(t, body, "ak-123")
	assert.NotContains(t, body, "tok-xyz")

	var resp dto.APIResponse[map[string]any]
	decodeJSON(t, rec, &resp)
	require.True(t, resp.Success)

	agents := resp.Data["required_agents"].([]any)
	require.Len(t, agents, 1)
	entry := agents[0].(map[string]any)

	agentCfg := entry["config"].(map[string]any)
	assert.NotContains(t, agentCfg, "password")
	assert.NotContains(t, agentCfg, "api_key")
	assert.Equal(t, "broker.local", agentCfg["host"])

	// 중첩 headers 의 auth_token 도 제거되어야 한다
	headers := agentCfg["headers"].(map[string]any)
	assert.NotContains(t, headers, "auth_token")
	assert.Equal(t, "json", headers["accept"])

	// sensitive_fields: api_key, auth_token, password (정렬)
	rawFields := entry["sensitive_fields"].([]any)
	fields := make([]string, 0, len(rawFields))
	for _, f := range rawFields {
		fields = append(fields, f.(string))
	}
	assert.Equal(t, []string{"api_key", "auth_token", "password"}, fields)

	// 최상위 required_secrets 요약: {broker01: [api_key, auth_token, password]}
	summary, ok := resp.Data["required_secrets"].(map[string]any)
	require.True(t, ok, "required_secrets 요약이 존재해야 한다")
	brokerSecrets := summary["broker01"].([]any)
	assert.Len(t, brokerSecrets, 3)
}

// TestExport_DoesNotMutateLiveConfig 는 export 가 라이브 플로우 config 를
// 변경하지 않는지(리댁션이 복사본에서만 일어나는지) 확인한다. (CONSTRAINT)
func TestExport_DoesNotMutateLiveConfig(t *testing.T) {
	cfg := reactFlowConfig(
		reactFlowNodeWithSecrets("n1", "mqtt-pub"),
	)
	flowMock := &mockFlowManager{
		getFlowFn: func(_ context.Context, id string) (*FlowInfo, error) {
			return &FlowInfo{ID: id, Name: "flow-secret", Status: "draft", Config: cfg}, nil
		},
	}
	agentsMock := &mockAgentManager{
		listAgentsFn: func(_ context.Context, _ dto.ListOptions) ([]AgentInfo, int64, error) {
			return []AgentInfo{}, 0, nil
		},
	}

	router := setupFlowRouterWithAgents(flowMock, agentsMock)
	rec := doRequest(t, router, http.MethodGet, "/api/v1/flows/flow-secret/export", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// 원본 cfg 의 노드 data 에 password 가 그대로 남아 있어야 한다
	nodes := cfg["nodes"].([]map[string]any)
	data := nodes[0]["data"].(map[string]any)
	assert.Equal(t, "node-secret", data["password"], "라이브 config 의 비밀값은 변경되어서는 안 된다")
}

// TestExport_NoSensitiveFieldsWhenClean 는 민감 키가 없으면 sensitive_fields /
// required_secrets 키가 생성되지 않는지 확인한다.
func TestExport_NoSensitiveFieldsWhenClean(t *testing.T) {
	// 비민감 config 만 가진 노드
	node := map[string]any{
		"id":       "n1",
		"type":     "custom",
		"position": map[string]any{"x": 0.0, "y": 0.0},
		"data": map[string]any{
			"label": "filter",
			"host":  "x",
			"port":  1.0,
		},
	}
	cfg := reactFlowConfig(node)
	flowMock := &mockFlowManager{
		getFlowFn: func(_ context.Context, id string) (*FlowInfo, error) {
			return &FlowInfo{ID: id, Name: "clean", Status: "draft", Config: cfg}, nil
		},
	}
	agentsMock := &mockAgentManager{
		listAgentsFn: func(_ context.Context, _ dto.ListOptions) ([]AgentInfo, int64, error) {
			return []AgentInfo{}, 0, nil
		},
	}

	router := setupFlowRouterWithAgents(flowMock, agentsMock)
	rec := doRequest(t, router, http.MethodGet, "/api/v1/flows/clean/export", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.False(t, strings.Contains(body, "sensitive_fields"), "민감 키 없으면 sensitive_fields 키가 없어야 한다")
	assert.False(t, strings.Contains(body, "required_secrets"))

	var resp dto.APIResponse[map[string]any]
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)
}
