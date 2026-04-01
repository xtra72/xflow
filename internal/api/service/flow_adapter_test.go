package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/internal/storage"
	"github.com/xtra/xflow/pkg/flow"
)

func newTestEngine() *engine.Engine {
	return engine.NewEngine(
		engine.WithNodeRegistry(node.NewRegistry()),
	)
}

func newTestRepo(t *testing.T) storage.FlowRepository {
	t.Helper()
	repo, err := storage.NewFileRepository(t.TempDir())
	if err != nil {
		t.Fatalf("테스트 저장소 생성 실패: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func TestFlowServiceAdapter_CreateFlow(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	tests := []struct {
		name    string
		req     *dto.FlowCreateRequest
		wantErr bool
	}{
		{
			name: "유효한 플로우 생성",
			req: &dto.FlowCreateRequest{
				Name:        "test-flow",
				Description: "테스트 플로우",
				Definition: map[string]any{
					"name":  "test-flow",
					"nodes": []any{},
					"wires": []any{},
				},
			},
			wantErr: false,
		},
		{
			name: "이름 없는 정의도 req.Name 으로 보완",
			req: &dto.FlowCreateRequest{
				Name: "fallback-name",
				Definition: map[string]any{
					"nodes": []any{},
					"wires": []any{},
				},
			},
			wantErr: false,
		},
		{
			name: "이름이 없으면 에러",
			req: &dto.FlowCreateRequest{
				Definition: map[string]any{
					"nodes": []any{},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := adapter.CreateFlow(context.Background(), tt.req)
			if tt.wantErr {
				if err == nil {
					t.Error("에러가 발생해야 하지만 발생하지 않음")
				}
				return
			}
			if err != nil {
				t.Fatalf("에러가 발생하지 않아야 하지만 발생: %v", err)
			}
			if info.ID == "" {
				t.Error("플로우 ID 가 비어있음")
			}
			if info.Name != tt.req.Name {
				t.Errorf("이름 불일치: got=%q, want=%q", info.Name, tt.req.Name)
			}
			if info.Status != "stored" {
				t.Errorf("상태 불일치: got=%q, want=%q", info.Status, "stored")
			}
		})
	}
}

func TestFlowServiceAdapter_GetFlow(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	// 1. 존재하지 않는 플로우 조회
	_, err := adapter.GetFlow(context.Background(), "nonexistent")
	if err == nil {
		t.Error("존재하지 않는 플로우 조회 시 에러가 발생해야 함")
	}

	// 2. 생성 후 조회
	info, err := adapter.CreateFlow(context.Background(), &dto.FlowCreateRequest{
		Name: "get-test",
		Definition: map[string]any{
			"name":  "get-test",
			"nodes": []any{},
			"wires": []any{},
		},
	})
	if err != nil {
		t.Fatalf("플로우 생성 실패: %v", err)
	}

	got, err := adapter.GetFlow(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("플로우 조회 실패: %v", err)
	}
	if got.ID != info.ID {
		t.Errorf("ID 불일치: got=%q, want=%q", got.ID, info.ID)
	}
}

func TestFlowServiceAdapter_ListFlows(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	// 빈 목록
	flows, total, err := adapter.ListFlows(context.Background(), dto.ListOptions{
		PaginationParams: dto.PaginationParams{Page: 1, Size: 20},
	})
	if err != nil {
		t.Fatalf("목록 조회 실패: %v", err)
	}
	if total != 0 {
		t.Errorf("빈 목록이어야 함: total=%d", total)
	}
	if len(flows) != 0 {
		t.Errorf("빈 목록이어야 함: len=%d", len(flows))
	}

	// 2개 생성
	for _, name := range []string{"flow-a", "flow-b"} {
		_, err := adapter.CreateFlow(context.Background(), &dto.FlowCreateRequest{
			Name: name,
			Definition: map[string]any{
				"name":  name,
				"nodes": []any{},
				"wires": []any{},
			},
		})
		if err != nil {
			t.Fatalf("플로우 생성 실패: %v", err)
		}
	}

	flows, total, err = adapter.ListFlows(context.Background(), dto.ListOptions{
		PaginationParams: dto.PaginationParams{Page: 1, Size: 20},
	})
	if err != nil {
		t.Fatalf("목록 조회 실패: %v", err)
	}
	if total != 2 {
		t.Errorf("2개여야 함: total=%d", total)
	}
	if len(flows) != 2 {
		t.Errorf("2개여야 함: len=%d", len(flows))
	}
}

func TestFlowServiceAdapter_DeleteFlow(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	info, err := adapter.CreateFlow(context.Background(), &dto.FlowCreateRequest{
		Name: "delete-test",
		Definition: map[string]any{
			"name":  "delete-test",
			"nodes": []any{},
			"wires": []any{},
		},
	})
	if err != nil {
		t.Fatalf("플로우 생성 실패: %v", err)
	}

	// 삭제
	if err := adapter.DeleteFlow(context.Background(), info.ID); err != nil {
		t.Fatalf("플로우 삭제 실패: %v", err)
	}

	// 삭제 후 조회
	_, err = adapter.GetFlow(context.Background(), info.ID)
	if err == nil {
		t.Error("삭제된 플로우 조회 시 에러가 발생해야 함")
	}
}

func TestFlowServiceAdapter_DeployFlow(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	// 존재하지 않는 플로우 배포
	err := adapter.DeployFlow(context.Background(), "nonexistent")
	if err == nil {
		t.Error("존재하지 않는 플로우 배포 시 에러가 발생해야 함")
	}

	// 빈 노드/와이어 플로우 생성 및 배포
	info, err := adapter.CreateFlow(context.Background(), &dto.FlowCreateRequest{
		Name: "deploy-test",
		Definition: map[string]any{
			"name":  "deploy-test",
			"nodes": []any{},
			"wires": []any{},
		},
	})
	if err != nil {
		t.Fatalf("플로우 생성 실패: %v", err)
	}

	// 배포
	if err := adapter.DeployFlow(context.Background(), info.ID); err != nil {
		t.Fatalf("플로우 배포 실패: %v", err)
	}

	// 배포 후 엔진에서 조회 가능
	got, err := adapter.GetFlow(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("배포된 플로우 조회 실패: %v", err)
	}
	if got.Status != "loaded" {
		t.Errorf("배포 후 상태 불일치: got=%q, want=%q", got.Status, "loaded")
	}
}

func TestFlowServiceAdapter_FlowStatus(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	// 미배포 플로우 상태 조회 → 에러
	_, err := adapter.FlowStatus(context.Background(), "nonexistent")
	if err == nil {
		t.Error("미배포 플로우 상태 조회 시 에러가 발생해야 함")
	}

	// 배포 후 상태 조회
	info, _ := adapter.CreateFlow(context.Background(), &dto.FlowCreateRequest{
		Name: "status-test",
		Definition: map[string]any{
			"name":  "status-test",
			"nodes": []any{},
			"wires": []any{},
		},
	})
	_ = adapter.DeployFlow(context.Background(), info.ID)

	status, err := adapter.FlowStatus(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("상태 조회 실패: %v", err)
	}
	if status.ID != info.ID {
		t.Errorf("ID 불일치: got=%q, want=%q", status.ID, info.ID)
	}
}

func TestFlowServiceAdapter_ListFlows_Pagination(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	// 5개 생성
	for i := 0; i < 5; i++ {
		name := "page-test-" + string(rune('a'+i))
		_, _ = adapter.CreateFlow(context.Background(), &dto.FlowCreateRequest{
			Name: name,
			Definition: map[string]any{
				"name":  name,
				"nodes": []any{},
				"wires": []any{},
			},
		})
	}

	// 페이지 1, 크기 2
	flows, total, err := adapter.ListFlows(context.Background(), dto.ListOptions{
		PaginationParams: dto.PaginationParams{Page: 1, Size: 2},
	})
	if err != nil {
		t.Fatalf("목록 조회 실패: %v", err)
	}
	if total != 5 {
		t.Errorf("total 불일치: got=%d, want=5", total)
	}
	if len(flows) != 2 {
		t.Errorf("페이지 크기 불일치: got=%d, want=2", len(flows))
	}
}

func TestFlowServiceAdapter_ListFlowNodes(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	// 미배포 플로우 → 에러
	_, err := adapter.ListFlowNodes(context.Background(), "nonexistent")
	if err == nil {
		t.Error("미배포 플로우의 노드 목록 조회 시 에러가 발생해야 함")
	}

	// 빈 노드 플로우 배포 후 조회
	info, _ := adapter.CreateFlow(context.Background(), &dto.FlowCreateRequest{
		Name: "nodes-test",
		Definition: map[string]any{
			"name":  "nodes-test",
			"nodes": []any{},
			"wires": []any{},
		},
	})
	_ = adapter.DeployFlow(context.Background(), info.ID)

	nodes, err := adapter.ListFlowNodes(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("노드 목록 조회 실패: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("빈 노드 목록이어야 함: len=%d", len(nodes))
	}
}

func TestFlowServiceAdapter_GetFlowNode(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	// 미배포 플로우 → 에러
	_, err := adapter.GetFlowNode(context.Background(), "nonexistent", "any")
	if err == nil {
		t.Error("미배포 플로우의 노드 조회 시 에러가 발생해야 함")
	}

	// 배포 후 존재하지 않는 노드 → 에러
	info, _ := adapter.CreateFlow(context.Background(), &dto.FlowCreateRequest{
		Name: "node-test",
		Definition: map[string]any{
			"name":  "node-test",
			"nodes": []any{},
			"wires": []any{},
		},
	})
	_ = adapter.DeployFlow(context.Background(), info.ID)

	_, err = adapter.GetFlowNode(context.Background(), info.ID, "nonexistent")
	if err == nil {
		t.Error("존재하지 않는 노드 조회 시 에러가 발생해야 함")
	}
}

func TestFlowServiceAdapter_FlowStatus_NodeStats(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	// 빈 노드 플로우 배포 후 상태 조회 — NodeStats 가 빈 슬라이스이어야 함
	info, _ := adapter.CreateFlow(context.Background(), &dto.FlowCreateRequest{
		Name: "stats-test",
		Definition: map[string]any{
			"name":  "stats-test",
			"nodes": []any{},
			"wires": []any{},
		},
	})
	_ = adapter.DeployFlow(context.Background(), info.ID)

	status, err := adapter.FlowStatus(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("상태 조회 실패: %v", err)
	}
	if status.NodeStats == nil {
		// NodeStats 필드가 nil 이 아니고 빈 슬라이스일 수 있음 — 둘 다 허용
		// 노드가 0개이므로 nil 또는 빈 슬라이스
	}
	if len(status.NodeStats) != 0 {
		t.Errorf("빈 노드 플로우의 NodeStats 는 비어있어야 함: len=%d", len(status.NodeStats))
	}
}

// TestNormalizeReactFlowDefinition_BridgeAgentRef 는 bridge 노드의 agent_ref 생성을 검증한다.
// agent_id 가 비어있어도 agent_name 이 있으면 agent_ref 를 생성해야 한다.
func TestNormalizeReactFlowDefinition_BridgeAgentRef(t *testing.T) {
	tests := []struct {
		name          string
		nodeData      map[string]any
		wantAgentRef  bool
		wantAgentName string
		wantAgentID   string
		wantDirection string
	}{
		{
			name: "agent_id와 agent_name 모두 있으면 agent_ref 생성",
			nodeData: map[string]any{
				"nodeType":   "bridge",
				"agent_id":   "agent-001",
				"agent_name": "mqtt-broker",
				"direction":  "in",
			},
			wantAgentRef:  true,
			wantAgentID:   "agent-001",
			wantAgentName: "mqtt-broker",
			wantDirection: "in",
		},
		{
			name: "agent_id 비어있고 agent_name만 있으면 agent_ref 생성 (YAML 로드 케이스)",
			nodeData: map[string]any{
				"nodeType":   "bridge",
				"agent_id":   "",
				"agent_name": "modbus-gateway-server",
				"direction":  "out",
			},
			wantAgentRef:  true,
			wantAgentID:   "",
			wantAgentName: "modbus-gateway-server",
			wantDirection: "out",
		},
		{
			name: "agent_id 없고 agent_name만 있으면 agent_ref 생성",
			nodeData: map[string]any{
				"nodeType":   "bridge",
				"agent_name": "logger",
				"direction":  "out",
			},
			wantAgentRef:  true,
			wantAgentID:   "",
			wantAgentName: "logger",
			wantDirection: "out",
		},
		{
			name: "agent_id도 agent_name도 없으면 agent_ref 미생성",
			nodeData: map[string]any{
				"nodeType": "bridge",
			},
			wantAgentRef: false,
		},
		{
			name: "bridge가 아닌 노드도 agent_ref 생성 (direction 제외)",
			nodeData: map[string]any{
				"nodeType":   "tsdb-write",
				"agent_id":   "agent-001",
				"agent_name": "tsdb-engine",
			},
			wantAgentRef:  true,
			wantAgentID:   "agent-001",
			wantAgentName: "tsdb-engine",
			wantDirection: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// React Flow 형식의 definition 구성
			def := map[string]any{
				"nodes": []any{
					map[string]any{
						"id":       "node-1",
						"type":     "custom",
						"position": map[string]any{"x": 0.0, "y": 0.0},
						"data":     tt.nodeData,
					},
				},
				"edges": []any{},
			}

			result := normalizeReactFlowDefinition(def)

			nodesRaw, ok := result["nodes"].([]any)
			if !ok || len(nodesRaw) == 0 {
				t.Fatal("변환된 노드가 없음")
			}
			node, ok := nodesRaw[0].(map[string]any)
			if !ok {
				t.Fatal("변환된 노드 타입이 map[string]any 가 아님")
			}

			agentRef, hasRef := node["agent_ref"]
			if tt.wantAgentRef {
				if !hasRef {
					t.Fatal("agent_ref 가 있어야 하지만 없음")
				}
				ref, ok := agentRef.(map[string]any)
				if !ok {
					t.Fatal("agent_ref 타입이 map[string]any 가 아님")
				}
				if got := ref["agent_id"].(string); got != tt.wantAgentID {
					t.Errorf("agent_ref.agent_id 불일치: got=%q, want=%q", got, tt.wantAgentID)
				}
				if got := ref["agent_name"].(string); got != tt.wantAgentName {
					t.Errorf("agent_ref.agent_name 불일치: got=%q, want=%q", got, tt.wantAgentName)
				}
				gotDir, _ := ref["direction"].(string)
				if gotDir != tt.wantDirection {
					t.Errorf("agent_ref.direction 불일치: got=%q, want=%q", gotDir, tt.wantDirection)
				}
			} else {
				if hasRef {
					t.Errorf("agent_ref 가 없어야 하지만 있음: %v", agentRef)
				}
			}
		})
	}
}

// TestNormalizeReactFlowDefinition_ErrorPorts 는 error 포트가 normalizeReactFlowDefinition에서
// 올바르게 처리되는지 검증한다. direction "error" 포트는 errors 배열로 분리되어야 한다.
func TestNormalizeReactFlowDefinition_ErrorPorts(t *testing.T) {
	def := map[string]any{
		"nodes": []any{
			map[string]any{
				"id":       "node-1",
				"type":     "custom",
				"position": map[string]any{"x": 0.0, "y": 0.0},
				"data": map[string]any{
					"nodeType": "bridge",
					"label":    "mqtt-receiver",
					"ports": []any{
						map[string]any{"name": "in", "direction": "input"},
						map[string]any{"name": "out", "direction": "output"},
						map[string]any{"name": "error", "direction": "error"},
					},
					"agent_name": "mqtt-sensor-agent",
					"direction":  "in",
				},
			},
		},
		"edges": []any{},
	}

	result := normalizeReactFlowDefinition(def)

	nodesRaw, ok := result["nodes"].([]any)
	if !ok || len(nodesRaw) == 0 {
		t.Fatal("변환된 노드가 없음")
	}
	node, ok := nodesRaw[0].(map[string]any)
	if !ok {
		t.Fatal("변환된 노드 타입이 map[string]any 가 아님")
	}

	// inputs 검증
	inputs, ok := node["inputs"].([]any)
	if !ok || len(inputs) != 1 {
		t.Fatalf("inputs 가 1개여야 함: got %v", node["inputs"])
	}

	// outputs 검증
	outputs, ok := node["outputs"].([]any)
	if !ok || len(outputs) != 1 {
		t.Fatalf("outputs 가 1개여야 함: got %v", node["outputs"])
	}

	// errors 검증 — 핵심: direction "error" 포트가 errors 배열에 포함되어야 한다
	errors, ok := node["errors"].([]any)
	if !ok || len(errors) != 1 {
		t.Fatalf("errors 가 1개여야 함: got %v", node["errors"])
	}
	errPort, ok := errors[0].(map[string]any)
	if !ok {
		t.Fatal("errors[0] 타입이 map[string]any 가 아님")
	}
	if errPort["name"] != "error" {
		t.Errorf("errors[0].name 불일치: got=%q, want=%q", errPort["name"], "error")
	}
}

// TestNormalizeExportedAgentRefs 는 XFlow 내보내기 포맷의 agent + direction 을
// agent_ref 로 역정규화하는 로직을 검증한다.
func TestNormalizeExportedAgentRefs(t *testing.T) {
	tests := []struct {
		name      string
		node      map[string]any
		wantRef   map[string]any
		wantAgent bool // agent 키가 남아있어야 하는지
		wantDir   bool // direction 키가 남아있어야 하는지
	}{
		{
			name: "agent name + direction → agent_ref",
			node: map[string]any{
				"type":      "bridge",
				"name":      "lgcp-receiver",
				"agent":     map[string]any{"name": "lgcp-capture"},
				"direction": "in",
			},
			wantRef: map[string]any{
				"agent_name": "lgcp-capture",
				"direction":  "in",
			},
		},
		{
			name: "agent id + name → agent_ref",
			node: map[string]any{
				"type":      "bridge",
				"agent":     map[string]any{"id": "abc-123", "name": "my-agent"},
				"direction": "out",
			},
			wantRef: map[string]any{
				"agent_id":   "abc-123",
				"agent_name": "my-agent",
				"direction":  "out",
			},
		},
		{
			name: "agent without direction",
			node: map[string]any{
				"type":  "mqtt-publisher",
				"agent": map[string]any{"name": "mqtt-broker"},
			},
			wantRef: map[string]any{
				"agent_name": "mqtt-broker",
			},
		},
		{
			name: "already has agent_ref — skip",
			node: map[string]any{
				"type":      "bridge",
				"agent_ref": map[string]any{"agent_name": "existing"},
				"agent":     map[string]any{"name": "should-not-override"},
			},
			wantRef:   map[string]any{"agent_name": "existing"},
			wantAgent: true,
		},
		{
			name: "no agent field — skip",
			node: map[string]any{
				"type": "transform",
				"name": "converter",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeSlice := []any{tt.node}
			normalizeExportedAgentRefs(nodeSlice)

			if tt.wantRef == nil {
				_, hasRef := tt.node["agent_ref"]
				assert.False(t, hasRef, "agent_ref 가 생성되면 안 된다")
				return
			}

			ref, ok := tt.node["agent_ref"].(map[string]any)
			assert.True(t, ok, "agent_ref 가 map[string]any 이어야 한다")
			for k, want := range tt.wantRef {
				assert.Equal(t, want, ref[k], "agent_ref[%s] 불일치", k)
			}

			_, hasAgent := tt.node["agent"]
			assert.Equal(t, tt.wantAgent, hasAgent, "agent 키 존재 여부")

			_, hasDir := tt.node["direction"]
			assert.Equal(t, tt.wantDir, hasDir, "direction 키 존재 여부")
		})
	}
}

// TestNormalizeReactFlowDefinition_XFlowExportFormat 은 XFlow 내보내기 포맷
// (data 필드 없음) 이 normalizeReactFlowDefinition 을 통과할 때
// agent_ref 가 올바르게 생성되는지 검증한다.
func TestNormalizeReactFlowDefinition_XFlowExportFormat(t *testing.T) {
	def := map[string]any{
		"nodes": []any{
			map[string]any{
				"id":        "node-1",
				"type":      "bridge",
				"name":      "lgcp-receiver",
				"agent":     map[string]any{"name": "lgcp-capture"},
				"direction": "in",
			},
		},
		"edges": []any{},
	}

	result := normalizeReactFlowDefinition(def)

	nodes := result["nodes"].([]any)
	node := nodes[0].(map[string]any)
	ref, ok := node["agent_ref"].(map[string]any)
	assert.True(t, ok, "agent_ref 가 생성되어야 한다")
	assert.Equal(t, "lgcp-capture", ref["agent_name"])
	assert.Equal(t, "in", ref["direction"])
	_, hasAgent := node["agent"]
	assert.False(t, hasAgent, "agent 키가 제거되어야 한다")
}

// TestFlowToReactFlowConfig_ErrorPorts 는 flowToReactFlowConfig 에서 error 포트가
// React Flow 데이터에 direction "error" 로 포함되는지 검증한다.
func TestFlowToReactFlowConfig_ErrorPorts(t *testing.T) {
	adapter := NewFlowServiceAdapter(newTestEngine(), newTestRepo(t), nil)
	f := flow.NewFlow("test-flow",
		flow.WithNodes(
			flow.NewNodeDef("mqtt-receiver", "bridge", flow.WithErrorPort()),
		),
	)

	result := adapter.flowToReactFlowConfig(f)

	nodesRaw, ok := result["nodes"].([]map[string]any)
	if !ok || len(nodesRaw) == 0 {
		t.Fatal("React Flow 노드가 없음")
	}
	data, ok := nodesRaw[0]["data"].(map[string]any)
	if !ok {
		t.Fatal("data 필드가 없음")
	}
	ports, ok := data["ports"].([]map[string]any)
	if !ok {
		t.Fatalf("ports 가 없음: data=%v", data)
	}

	// error 포트 검색
	foundError := false
	for _, p := range ports {
		if p["direction"] == "error" && p["name"] == "error" {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Errorf("error 포트가 React Flow 데이터에 포함되어야 함: ports=%v", ports)
	}
}

// TestFlowServiceAdapter_CreateAndStart_EmptyDefinition 은 프론트엔드에서 빈 definition 으로
// 플로우를 생성한 후 바로 시작하는 시나리오를 재현한다.
// CreateFlowModal 이 definition: {} 를 전송하고, 에디터에서 Start 를 클릭하는 흐름.
func TestFlowServiceAdapter_CreateAndStart_EmptyDefinition(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	// 1. 프론트엔드 CreateFlowModal 과 동일: definition: {} (빈 맵)
	info, err := adapter.CreateFlow(context.Background(), &dto.FlowCreateRequest{
		Name:       "empty-def-flow",
		Definition: map[string]any{},
	})
	if err != nil {
		t.Fatalf("빈 definition 으로 플로우 생성 실패: %v", err)
	}
	if info.ID == "" {
		t.Fatal("생성된 플로우 ID 가 비어있음")
	}
	t.Logf("생성된 플로우 ID: %s", info.ID)

	// 2. GetFlow 로 조회 가능한지 확인 (에디터 페이지 로딩)
	got, err := adapter.GetFlow(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("생성된 플로우 조회 실패: %v", err)
	}
	if got.ID != info.ID {
		t.Errorf("ID 불일치: got=%q, want=%q", got.ID, info.ID)
	}

	// 3. StartFlow: 배포되지 않은 상태에서 바로 시작 시도
	err = adapter.StartFlow(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("플로우 시작 실패 (이것이 사용자의 '시작 실패: not found' 에러): %v", err)
	}

	// 4. 시작 후 상태 확인
	status, err := adapter.FlowStatus(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("시작 후 상태 조회 실패: %v", err)
	}
	if status.Status != "running" {
		t.Errorf("시작 후 상태가 running 이어야 함: got=%q", status.Status)
	}
}

// TestFlowServiceAdapter_CreateAndStart_WithSQLite 는 SQLite 저장소를 사용하여
// 동일한 시나리오를 테스트한다.
func TestFlowServiceAdapter_CreateAndStart_WithSQLite(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := storage.NewSQLiteRepository(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("SQLite 저장소 생성 실패: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, repo, nil)

	// 1. 빈 definition 으로 플로우 생성
	info, err := adapter.CreateFlow(context.Background(), &dto.FlowCreateRequest{
		Name:       "sqlite-test-flow",
		Definition: map[string]any{},
	})
	if err != nil {
		t.Fatalf("플로우 생성 실패: %v", err)
	}
	t.Logf("생성된 플로우 ID: %s", info.ID)

	// 2. repo.Get 으로 직접 조회 확인
	f, err := repo.Get(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("SQLite 에서 플로우 직접 조회 실패: %v", err)
	}
	if f.ID() != info.ID {
		t.Errorf("저장소 ID 불일치: got=%q, want=%q", f.ID(), info.ID)
	}

	// 3. StartFlow 시도
	err = adapter.StartFlow(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("SQLite 저장소로 플로우 시작 실패: %v", err)
	}

	// 4. 상태 확인
	status, err := adapter.FlowStatus(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("시작 후 상태 조회 실패: %v", err)
	}
	if status.Status != "running" {
		t.Errorf("시작 후 상태가 running 이어야 함: got=%q", status.Status)
	}
}

// TestFlowServiceAdapter_CreateFlow_DuplicateName 은 동일 이름의 플로우를 생성하면
// 기존 플로우가 교체되어 중복이 발생하지 않는지 검증한다.
func TestFlowServiceAdapter_CreateFlow_DuplicateName(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)
	ctx := context.Background()

	req := &dto.FlowCreateRequest{
		Name: "duplicate-test",
		Definition: map[string]any{
			"name":  "duplicate-test",
			"nodes": []any{},
			"wires": []any{},
		},
	}

	// 첫 번째 생성
	info1, err := adapter.CreateFlow(ctx, req)
	if err != nil {
		t.Fatalf("첫 번째 생성 실패: %v", err)
	}

	// 동일 이름으로 두 번째 생성
	info2, err := adapter.CreateFlow(ctx, req)
	if err != nil {
		t.Fatalf("두 번째 생성 실패: %v", err)
	}

	// ID 가 달라야 한다 (새로 생성됨)
	if info1.ID == info2.ID {
		t.Error("두 번째 생성의 ID 가 첫 번째와 같으면 안 됨")
	}

	// 목록에 동일 이름이 1개만 있어야 한다
	flows, total, err := adapter.ListFlows(ctx, dto.ListOptions{
		PaginationParams: dto.PaginationParams{Page: 1, Size: 20},
	})
	if err != nil {
		t.Fatalf("목록 조회 실패: %v", err)
	}
	if total != 1 {
		t.Errorf("플로우 개수: got %d, want 1 (중복 발생)", total)
	}
	if len(flows) != 1 {
		t.Errorf("플로우 슬라이스 길이: got %d, want 1", len(flows))
	}
	if len(flows) > 0 && flows[0].ID != info2.ID {
		t.Errorf("남은 플로우 ID: got %q, want %q (최신)", flows[0].ID, info2.ID)
	}
}
