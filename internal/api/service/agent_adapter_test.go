package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/storage"
	"github.com/xtra/xflow/pkg/lifecycle"
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
	_, err := adapter.GetAgent(context.Background(), "nonexistent", "")
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

	got, err := adapter.GetAgent(context.Background(), info.ID, "")
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
	_, err = adapter.GetAgent(context.Background(), info.ID, "")
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
	got, err := adapter.GetAgent(context.Background(), info.ID, "")
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

// TestAgentServiceAdapter_GetAgent_DetailSummary 는 detail="summary" 로
// GetAgent 를 호출했을 때 health, stats, uptime, connected, started_at,
// created_at 필드가 채워지는지 검증한다.
func TestAgentServiceAdapter_GetAgent_DetailSummary(t *testing.T) {
	mgr := agent.NewManager()
	adapter := NewAgentServiceAdapter(mgr, nil, nil)

	// 에이전트 생성 (BaseAgent.Init 후 StateRunning, startedAt/createdAt 설정됨)
	info, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "detail-summary-test",
		Type: "",
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	got, err := adapter.GetAgent(context.Background(), info.ID, "summary")
	if err != nil {
		t.Fatalf("에이전트 조회 실패: %v", err)
	}

	// 기본 필드 확인
	if got.ID != info.ID {
		t.Errorf("ID 불일치: got=%q, want=%q", got.ID, info.ID)
	}

	// summary 전용 필드 확인
	if got.Health == nil {
		t.Error("Health 가 nil 이면 안 됨 (summary)")
	}
	if got.Stats == nil {
		t.Error("Stats 가 nil 이면 안 됨 (summary)")
	}
	if got.Connected == nil {
		t.Error("Connected 가 nil 이면 안 됨 (summary)")
	} else if !*got.Connected {
		t.Error("Running 상태 에이전트의 Connected 는 true 여야 함")
	}
	if got.Uptime == "" {
		t.Error("Running 상태 에이전트의 Uptime 이 비어있으면 안 됨")
	}
	if got.StartedAt == nil {
		t.Error("StartedAt 이 nil 이면 안 됨 (summary)")
	}
	if got.CreatedAt == nil {
		t.Error("CreatedAt 이 nil 이면 안 됨 (summary)")
	}

	// full 전용 필드는 비어있어야 함
	if got.State != nil {
		t.Error("detail=summary 일 때 State 는 nil 이어야 함")
	}
}

// TestAgentServiceAdapter_GetAgent_DetailFull 은 detail="full" 로
// agentToHandlerInfo 를 호출했을 때 summary 필드 + shared_info + state 가
// 모두 채워지는지 검증한다.
func TestAgentServiceAdapter_GetAgent_DetailFull(t *testing.T) {
	now := time.Now()
	mock := &mockStatefulAgent{
		info: agent.AgentInfo{
			ID:    "full-test-id",
			Name:  "full-test",
			Type:  "mock",
			State: lifecycle.StateRunning,
			Health: agent.HealthStatus{
				Status:    agent.HealthHealthy,
				LastCheck: now,
			},
			Config: agent.AgentConfig{
				ID:   "full-test-id",
				Name: "full-test",
				Type: "mock",
			},
			Stats: agent.StatsSnapshot{
				MessagesReceived: 10,
				MessagesSent:     5,
				MessagesErrored:  1,
			},
			SharedInfo: &agent.SharedInfo{
				RefCount: 2,
				Flows:    []string{"flow-a", "flow-b"},
			},
			StartedAt: now.Add(-1 * time.Minute),
			Uptime:    1 * time.Minute,
			CreatedAt: now.Add(-10 * time.Minute),
		},
		state: map[string]any{"test_key": "test_value"},
	}

	result := agentToHandlerInfo(mock, "full")

	// 기본 필드
	if result.ID != "full-test-id" {
		t.Errorf("ID 불일치: got=%q", result.ID)
	}
	if result.Name != "full-test" {
		t.Errorf("Name 불일치: got=%q", result.Name)
	}
	if result.Status != string(lifecycle.StateRunning) {
		t.Errorf("Status 불일치: got=%q", result.Status)
	}

	// summary 필드
	if result.Health == nil {
		t.Error("Health 가 nil 이면 안 됨 (full)")
	}
	if result.Stats == nil {
		t.Error("Stats 가 nil 이면 안 됨 (full)")
	} else {
		if result.Stats.MessagesIn != 10 {
			t.Errorf("MessagesIn 불일치: got=%d, want=10", result.Stats.MessagesIn)
		}
		if result.Stats.MessagesOut != 5 {
			t.Errorf("MessagesOut 불일치: got=%d, want=5", result.Stats.MessagesOut)
		}
	}
	if result.Connected == nil {
		t.Error("Connected 가 nil 이면 안 됨 (full)")
	} else if !*result.Connected {
		t.Error("Running 상태 에이전트의 Connected 는 true 여야 함")
	}
	if result.Uptime == "" {
		t.Error("Uptime 이 비어있으면 안 됨 (full)")
	}
	if result.StartedAt == nil {
		t.Error("StartedAt 이 nil 이면 안 됨 (full)")
	}
	if result.CreatedAt == nil {
		t.Error("CreatedAt 이 nil 이면 안 됨 (full)")
	}

	// full 전용 필드: SharedInfo
	if result.SharedInfo == nil {
		t.Fatal("SharedInfo 가 nil 이면 안 됨 (full)")
	}
	if result.SharedInfo.RefCount != 2 {
		t.Errorf("SharedInfo.RefCount 불일치: got=%d, want=2", result.SharedInfo.RefCount)
	}
	if len(result.SharedInfo.Flows) != 2 {
		t.Errorf("SharedInfo.Flows 길이 불일치: got=%d, want=2", len(result.SharedInfo.Flows))
	}

	// full 전용 필드: State (StatefulAgent)
	if result.State == nil {
		t.Fatal("State 가 nil 이면 안 됨 (full, StatefulAgent)")
	}
	if v, ok := result.State["test_key"]; !ok || v != "test_value" {
		t.Errorf("State[test_key] 불일치: got=%v", result.State)
	}
}

// mockStatefulAgent 는 agent.Agent 와 agent.StatefulAgent 를 모두 구현하는
// 테스트용 모의 에이전트이다.
type mockStatefulAgent struct {
	info  agent.AgentInfo
	state map[string]any
}

func (m *mockStatefulAgent) Init(_ agent.AgentConfig) error            { return nil }
func (m *mockStatefulAgent) Start(_ context.Context) error             { return nil }
func (m *mockStatefulAgent) Stop(_ context.Context) error              { return nil }
func (m *mockStatefulAgent) Pause(_ context.Context) error             { return nil }
func (m *mockStatefulAgent) Resume(_ context.Context) error            { return nil }
func (m *mockStatefulAgent) Health() agent.HealthStatus                { return m.info.Health }
func (m *mockStatefulAgent) Process(_ []byte) ([]byte, error)          { return nil, nil }
func (m *mockStatefulAgent) Configure(_ agent.AgentConfig) error       { return nil }
func (m *mockStatefulAgent) ID() string                                { return m.info.ID }
func (m *mockStatefulAgent) Name() string                              { return m.info.Name }
func (m *mockStatefulAgent) Type() string                              { return m.info.Type }
func (m *mockStatefulAgent) Info() agent.AgentInfo                     { return m.info }
func (m *mockStatefulAgent) Stats() agent.StatsSnapshot                { return m.info.Stats }
func (m *mockStatefulAgent) State() map[string]any                     { return m.state }
