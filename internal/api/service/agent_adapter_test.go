package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

// TestAgentServiceAdapter_ConfigureAgent_WithRepo 는 ConfigureAgent 가 중첩 구조를 포함한
// 설정을 영속 저장소에 올바르게 저장하는지 검증한다 (register_map 등).
func TestAgentServiceAdapter_ConfigureAgent_WithRepo(t *testing.T) {
	mgr := agent.NewManager()
	repo, err := storage.NewAgentFileRepository(t.TempDir())
	if err != nil {
		t.Fatalf("저장소 생성 실패: %v", err)
	}
	adapter := NewAgentServiceAdapter(mgr, repo, nil)

	info, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "modbus-test",
		Type: "",
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	// 중첩 구조 포함한 config 로 ConfigureAgent 호출
	newCfg := map[string]any{
		"listen_port": float64(502),
		"unit_id":     float64(1),
		"register_map": map[string]any{
			"input_registers": []any{
				map[string]any{"start_address": float64(0), "count": float64(10)},
			},
		},
	}
	err = adapter.ConfigureAgent(context.Background(), info.ID, newCfg)
	if err != nil {
		t.Fatalf("에이전트 설정 실패: %v", err)
	}

	// 저장소에서 다시 로드하여 Transport.Options 검증
	cfg, err := repo.Get(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("저장소에서 에이전트 조회 실패: %v", err)
	}

	// Transport.Options에 register_map이 보존되어야 한다
	rm, ok := cfg.Transport.Options["register_map"]
	if !ok {
		t.Fatal("Transport.Options에 register_map 이 없음")
	}
	rmMap, ok := rm.(map[string]any)
	if !ok {
		t.Fatalf("register_map 타입이 map[string]any 가 아님: %T", rm)
	}
	if _, ok := rmMap["input_registers"]; !ok {
		t.Error("register_map에 input_registers 가 없음")
	}

	// Metadata에는 중첩 객체가 포함되지 않아야 한다
	if _, ok := cfg.Metadata["register_map"]; ok {
		t.Error("Metadata에 register_map 이 포함되면 안 됨 (중첩 객체)")
	}
	// 스칼라 값은 Metadata에 있어야 한다
	if v, ok := cfg.Metadata["listen_port"]; !ok || v != "502" {
		t.Errorf("Metadata[listen_port] 불일치: got=%q", v)
	}
}

// TestNeedsRestart 는 transport 설정 변경 감지를 검증한다.
func TestNeedsRestart(t *testing.T) {
	tests := []struct {
		name    string
		oldOpts map[string]any
		newOpts map[string]any
		want    bool
	}{
		{
			name:    "port 변경 — 재시작 필요",
			oldOpts: map[string]any{"port": "/dev/ttyUSB0"},
			newOpts: map[string]any{"port": "/dev/ttyUSB1"},
			want:    true,
		},
		{
			name:    "serial_port 변경 — 재시작 필요",
			oldOpts: map[string]any{"serial_port": "/dev/ttyS0"},
			newOpts: map[string]any{"serial_port": "/dev/ttyS1"},
			want:    true,
		},
		{
			name:    "baud_rate 변경 — 재시작 필요",
			oldOpts: map[string]any{"port": "/dev/ttyUSB0", "baud_rate": 9600},
			newOpts: map[string]any{"port": "/dev/ttyUSB0", "baud_rate": 115200},
			want:    true,
		},
		{
			name:    "tcp_address 변경 — 재시작 필요",
			oldOpts: map[string]any{"tcp_address": "192.168.1.1:502"},
			newOpts: map[string]any{"tcp_address": "192.168.1.2:502"},
			want:    true,
		},
		{
			name:    "port 추가 — 재시작 필요",
			oldOpts: map[string]any{},
			newOpts: map[string]any{"port": "/dev/ttyUSB0"},
			want:    true,
		},
		{
			name:    "비 transport 키만 변경 — 재시작 불필요",
			oldOpts: map[string]any{"port": "/dev/ttyUSB0", "framing": "raw"},
			newOpts: map[string]any{"port": "/dev/ttyUSB0", "framing": "newline"},
			want:    false,
		},
		{
			name:    "동일 설정 — 재시작 불필요",
			oldOpts: map[string]any{"port": "/dev/ttyUSB0", "baud_rate": 9600},
			newOpts: map[string]any{"port": "/dev/ttyUSB0", "baud_rate": 9600},
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needsRestart(tt.oldOpts, tt.newOpts)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestAgentServiceAdapter_UpdateAgent_TransportOptions 는 UpdateAgent 가
// Transport.Options 에 중첩 구조를 올바르게 설정하는지 검증한다.
func TestAgentServiceAdapter_UpdateAgent_TransportOptions(t *testing.T) {
	mgr := agent.NewManager()
	repo, err := storage.NewAgentFileRepository(t.TempDir())
	if err != nil {
		t.Fatalf("저장소 생성 실패: %v", err)
	}
	adapter := NewAgentServiceAdapter(mgr, repo, nil)

	info, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name:   "update-test",
		Type:   "",
		Config: map[string]any{"key": "old"},
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	// UpdateAgent 로 중첩 config 업데이트
	_, err = adapter.UpdateAgent(context.Background(), info.ID, &dto.AgentUpdateRequest{
		Config: map[string]any{
			"listen_port": float64(503),
			"register_map": map[string]any{
				"holding_registers": []any{
					map[string]any{"start_address": float64(0), "count": float64(20)},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("에이전트 업데이트 실패: %v", err)
	}

	// 저장소에서 다시 로드하여 검증
	cfg, err := repo.Get(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("저장소에서 에이전트 조회 실패: %v", err)
	}

	// Transport.Options에 register_map이 보존되어야 한다
	rm, ok := cfg.Transport.Options["register_map"]
	if !ok {
		t.Fatal("Transport.Options에 register_map 이 없음")
	}
	if rmMap, ok := rm.(map[string]any); !ok {
		t.Fatalf("register_map 타입 불일치: %T", rm)
	} else if _, ok := rmMap["holding_registers"]; !ok {
		t.Error("register_map에 holding_registers 가 없음")
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

// TestAgentToHandlerInfo_ConfigSource 는 Config 필드가 Transport.Options 를
// 우선 사용하고, 없으면 Metadata 로 폴백하는지 검증한다.
func TestAgentToHandlerInfo_ConfigSource(t *testing.T) {
	tests := []struct {
		name       string
		config     agent.AgentConfig
		wantConfig map[string]any
	}{
		{
			name: "Transport.Options 가 있으면 구조 보존",
			config: agent.AgentConfig{
				ID:   "opts-id",
				Name: "opts-test",
				Transport: agent.TransportConfig{
					Type: "modbus-tcp-server",
					Options: map[string]any{
						"listen_port": 5020,
						"register_map": map[string]any{
							"coils": map[string]any{
								"count":         8,
								"start_address": 0,
							},
						},
					},
				},
				Metadata: map[string]string{
					"listen_port":  "5020",
					"register_map": "map[coils:map[count:8 start_address:0]]",
				},
			},
			wantConfig: map[string]any{
				"listen_port": 5020,
				"register_map": map[string]any{
					"coils": map[string]any{
						"count":         8,
						"start_address": 0,
					},
				},
			},
		},
		{
			name: "Transport.Options 가 없으면 Metadata 폴백",
			config: agent.AgentConfig{
				ID:   "meta-id",
				Name: "meta-test",
				Metadata: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			wantConfig: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStatefulAgent{
				info: agent.AgentInfo{
					ID:     tt.config.ID,
					Name:   tt.config.Name,
					Type:   "test",
					Config: tt.config,
				},
			}
			result := agentToHandlerInfo(mock, "")
			if len(result.Config) != len(tt.wantConfig) {
				t.Fatalf("Config 길이 불일치: got=%d, want=%d\ngot=%v", len(result.Config), len(tt.wantConfig), result.Config)
			}
			for k, want := range tt.wantConfig {
				got, ok := result.Config[k]
				if !ok {
					t.Errorf("Config[%q] 키가 없음", k)
					continue
				}
				// 중첩 맵은 포인터 비교 불가하므로 fmt.Sprint 로 비교
				if fmt.Sprint(got) != fmt.Sprint(want) {
					t.Errorf("Config[%q] 불일치:\n  got=%v (%T)\n  want=%v (%T)", k, got, got, want, want)
				}
			}
		})
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
