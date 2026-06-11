package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GET /flows/{id}/subflow-stats 가 success 래퍼와 { flow_id, nodes[] } 형태를 반환하는지 검증한다.
func TestFlowHandler_SubflowStats_Shape(t *testing.T) {
	mock := &mockFlowManager{
		subflowStatsFn: func(_ context.Context, subflowID string) (*SubflowStatsInfo, error) {
			return &SubflowStatsInfo{
				FlowID: subflowID,
				Nodes: []SubflowNodeStat{
					{
						NodeID:    "A",
						Name:      "노드A",
						Type:      "transform",
						State:     "running",
						Processed: 15,
						Errors:    3,
						Ports: []PortInfo{
							{Name: "out", Direction: "output", Connected: true, Messages: 15, Delivered: 13, Throughput: "0.000"},
						},
					},
				},
			}, nil
		},
	}
	router := setupFlowRouter(mock)

	rec := doRequest(t, router, http.MethodGet, "/api/v1/flows/S/subflow-stats", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			FlowID string `json:"flow_id"`
			Nodes  []struct {
				NodeID    string `json:"node_id"`
				Processed int64  `json:"processed"`
				Errors    int64  `json:"errors"`
				Ports     []struct {
					Name      string `json:"name"`
					Messages  int64  `json:"messages"`
					Delivered int64  `json:"delivered"`
				} `json:"ports"`
			} `json:"nodes"`
		} `json:"data"`
	}
	decodeJSON(t, rec, &resp)

	assert.True(t, resp.Success)
	assert.Equal(t, "S", resp.Data.FlowID)
	require.Len(t, resp.Data.Nodes, 1)
	assert.Equal(t, "A", resp.Data.Nodes[0].NodeID)
	assert.Equal(t, int64(15), resp.Data.Nodes[0].Processed)
	assert.Equal(t, int64(3), resp.Data.Nodes[0].Errors)
	require.Len(t, resp.Data.Nodes[0].Ports, 1)
	assert.Equal(t, "out", resp.Data.Nodes[0].Ports[0].Name)
	assert.Equal(t, int64(13), resp.Data.Nodes[0].Ports[0].Delivered)
}

// 참조 부모가 없을 때 nodes 가 빈 배열([])로 직렬화되는지 검증한다.
func TestFlowHandler_SubflowStats_Empty(t *testing.T) {
	router := setupFlowRouter(&mockFlowManager{}) // 기본 mock: 빈 nodes 반환

	rec := doRequest(t, router, http.MethodGet, "/api/v1/flows/S/subflow-stats", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// nodes 키가 존재하고 빈 배열이어야 한다(null 아님).
	assert.Contains(t, rec.Body.String(), `"nodes":[]`)
}
