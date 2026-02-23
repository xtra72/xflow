package service

import (
	"context"
	"testing"

	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/internal/node"
)

func newTestEngine() *engine.Engine {
	return engine.NewEngine(
		engine.WithNodeRegistry(node.NewRegistry()),
	)
}

func TestFlowServiceAdapter_CreateFlow(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, nil)

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
	adapter := NewFlowServiceAdapter(eng, nil)

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
	adapter := NewFlowServiceAdapter(eng, nil)

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
	adapter := NewFlowServiceAdapter(eng, nil)

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
	adapter := NewFlowServiceAdapter(eng, nil)

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
	adapter := NewFlowServiceAdapter(eng, nil)

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
	adapter := NewFlowServiceAdapter(eng, nil)

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
	adapter := NewFlowServiceAdapter(eng, nil)

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
	adapter := NewFlowServiceAdapter(eng, nil)

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
	adapter := NewFlowServiceAdapter(eng, nil)

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
