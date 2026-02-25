package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/storage"
)

func TestAgentServiceAdapter_CreateAgent(t *testing.T) {
	mgr := agent.NewManager()
	adapter := NewAgentServiceAdapter(mgr, nil, nil)

	tests := []struct {
		name    string
		req     *dto.AgentCreateRequest
		wantErr bool
	}{
		{
			name: "유효한 에이전트 생성",
			req: &dto.AgentCreateRequest{
				Name: "test-agent",
				Type: "",
			},
			wantErr: false,
		},
		{
			name: "설정 포함 에이전트 생성",
			req: &dto.AgentCreateRequest{
				Name: "config-agent",
				Type: "",
				Config: map[string]any{
					"broker": "localhost:1883",
				},
			},
			wantErr: false,
		},
		{
			name: "미등록 타입 에이전트 생성 실패",
			req: &dto.AgentCreateRequest{
				Name: "unknown-type",
				Type: "nonexistent-type",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := adapter.CreateAgent(context.Background(), tt.req)
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
				t.Error("에이전트 ID 가 비어있음")
			}
			if info.Name != tt.req.Name {
				t.Errorf("이름 불일치: got=%q, want=%q", info.Name, tt.req.Name)
			}
			if info.Type != tt.req.Type {
				t.Errorf("타입 불일치: got=%q, want=%q", info.Type, tt.req.Type)
			}
		})
	}
}

func TestAgentServiceAdapter_GetAgent(t *testing.T) {
	mgr := agent.NewManager()
	adapter := NewAgentServiceAdapter(mgr, nil, nil)

	// 존재하지 않는 에이전트 조회
	_, err := adapter.GetAgent(context.Background(), "nonexistent")
	if err == nil {
		t.Error("존재하지 않는 에이전트 조회 시 에러가 발생해야 함")
	}

	// 생성 후 조회
	info, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "get-test",
		Type: "",
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	got, err := adapter.GetAgent(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("에이전트 조회 실패: %v", err)
	}
	if got.ID != info.ID {
		t.Errorf("ID 불일치: got=%q, want=%q", got.ID, info.ID)
	}
	if got.Name != "get-test" {
		t.Errorf("이름 불일치: got=%q, want=%q", got.Name, "get-test")
	}
}

func TestAgentServiceAdapter_ListAgents(t *testing.T) {
	mgr := agent.NewManager()
	adapter := NewAgentServiceAdapter(mgr, nil, nil)

	// 빈 목록
	agents, total, err := adapter.ListAgents(context.Background(), dto.ListOptions{
		PaginationParams: dto.PaginationParams{Page: 1, Size: 20},
	})
	if err != nil {
		t.Fatalf("목록 조회 실패: %v", err)
	}
	if total != 0 {
		t.Errorf("빈 목록이어야 함: total=%d", total)
	}
	if len(agents) != 0 {
		t.Errorf("빈 목록이어야 함: len=%d", len(agents))
	}

	// 2개 생성
	for _, name := range []string{"agent-a", "agent-b"} {
		_, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
			Name: name,
			Type: "",
		})
		if err != nil {
			t.Fatalf("에이전트 생성 실패: %v", err)
		}
	}

	agents, total, err = adapter.ListAgents(context.Background(), dto.ListOptions{
		PaginationParams: dto.PaginationParams{Page: 1, Size: 20},
	})
	if err != nil {
		t.Fatalf("목록 조회 실패: %v", err)
	}
	if total != 2 {
		t.Errorf("2개여야 함: total=%d", total)
	}
}

func TestAgentServiceAdapter_DeleteAgent(t *testing.T) {
	mgr := agent.NewManager()
	adapter := NewAgentServiceAdapter(mgr, nil, nil)

	info, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "delete-test",
		Type: "",
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	// 삭제
	if err := adapter.DeleteAgent(context.Background(), info.ID); err != nil {
		t.Fatalf("에이전트 삭제 실패: %v", err)
	}

	// 삭제 후 조회
	_, err = adapter.GetAgent(context.Background(), info.ID)
	if err == nil {
		t.Error("삭제된 에이전트 조회 시 에러가 발생해야 함")
	}
}

func TestAgentServiceAdapter_AgentStats(t *testing.T) {
	mgr := agent.NewManager()
	adapter := NewAgentServiceAdapter(mgr, nil, nil)

	// 존재하지 않는 에이전트 통계 조회
	_, err := adapter.AgentStats(context.Background(), "nonexistent")
	if err == nil {
		t.Error("존재하지 않는 에이전트 통계 조회 시 에러가 발생해야 함")
	}

	// 생성 후 통계 조회
	info, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "stats-test",
		Type: "",
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	stats, err := adapter.AgentStats(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("통계 조회 실패: %v", err)
	}
	if stats.ID != info.ID {
		t.Errorf("ID 불일치: got=%q, want=%q", stats.ID, info.ID)
	}
}

func TestAgentServiceAdapter_StopAgent(t *testing.T) {
	mgr := agent.NewManager()
	adapter := NewAgentServiceAdapter(mgr, nil, nil)

	info, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "stop-test",
		Type: "",
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	// Running 상태에서 Stop
	if err := adapter.StopAgent(context.Background(), info.ID); err != nil {
		t.Fatalf("에이전트 정지 실패: %v", err)
	}

	// 정지 후 상태 확인
	got, err := adapter.GetAgent(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("에이전트 조회 실패: %v", err)
	}
	if got.Status != "stopped" {
		t.Errorf("상태 불일치: got=%q, want=%q", got.Status, "stopped")
	}
}

func TestAgentServiceAdapter_ConfigureAgent(t *testing.T) {
	mgr := agent.NewManager()
	adapter := NewAgentServiceAdapter(mgr, nil, nil)

	info, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "config-test",
		Type: "",
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	err = adapter.ConfigureAgent(context.Background(), info.ID, map[string]any{
		"key": "value",
	})
	if err != nil {
		t.Fatalf("에이전트 설정 실패: %v", err)
	}
}

func TestAgentServiceAdapter_CreateAgent_WithRepo(t *testing.T) {
	mgr := agent.NewManager()
	repo, err := storage.NewAgentFileRepository(t.TempDir())
	if err != nil {
		t.Fatalf("저장소 생성 실패: %v", err)
	}
	adapter := NewAgentServiceAdapter(mgr, repo, nil)

	info, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "repo-test",
		Type: "",
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	// 저장소에서 에이전트 조회
	cfg, err := repo.Get(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("저장소에서 에이전트 조회 실패: %v", err)
	}
	if cfg.Name != "repo-test" {
		t.Errorf("이름 불일치: got=%q, want=%q", cfg.Name, "repo-test")
	}
}

func TestAgentServiceAdapter_DeleteAgent_WithRepo(t *testing.T) {
	mgr := agent.NewManager()
	repo, err := storage.NewAgentFileRepository(t.TempDir())
	if err != nil {
		t.Fatalf("저장소 생성 실패: %v", err)
	}
	adapter := NewAgentServiceAdapter(mgr, repo, nil)

	info, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "delete-repo-test",
		Type: "",
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	// 삭제
	if err := adapter.DeleteAgent(context.Background(), info.ID); err != nil {
		t.Fatalf("에이전트 삭제 실패: %v", err)
	}

	// 저장소에서 삭제 확인
	_, err = repo.Get(context.Background(), info.ID)
	if err == nil {
		t.Error("저장소에서 삭제된 에이전트가 여전히 존재함")
	}
}

func TestAgentServiceAdapter_CreateAgent_RepoFailure_Rollback(t *testing.T) {
	mgr := agent.NewManager()
	repo := &failingAgentRepo{}
	adapter := NewAgentServiceAdapter(mgr, repo, nil)

	_, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "rollback-test",
		Type: "",
	})
	if err == nil {
		t.Fatal("저장소 실패 시 에러가 발생해야 함")
	}

	// manager 에서도 롤백되어 에이전트가 없어야 함
	agents := mgr.List()
	if len(agents) != 0 {
		t.Errorf("롤백 후 에이전트가 남아있음: count=%d", len(agents))
	}
}

// failingAgentRepo 는 항상 Save 에서 실패하는 테스트용 저장소이다.
type failingAgentRepo struct{}

func (f *failingAgentRepo) Save(_ context.Context, _ agent.AgentConfig) error {
	return fmt.Errorf("forced save failure")
}

func (f *failingAgentRepo) Get(_ context.Context, _ string) (agent.AgentConfig, error) {
	return agent.AgentConfig{}, storage.ErrAgentNotFound
}

func (f *failingAgentRepo) List(_ context.Context) ([]agent.AgentConfig, error) {
	return nil, nil
}

func (f *failingAgentRepo) Delete(_ context.Context, _ string) error {
	return nil
}

func (f *failingAgentRepo) Close() error {
	return nil
}
