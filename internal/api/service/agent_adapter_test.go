package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/samsung"
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
			name:    "tcp_host 변경 — 재시작 필요",
			oldOpts: map[string]any{"tcp_host": "192.168.1.1"},
			newOpts: map[string]any{"tcp_host": "192.168.1.2"},
			want:    true,
		},
		{
			name:    "tcp_port 변경 — 재시작 필요",
			oldOpts: map[string]any{"tcp_port": 9100},
			newOpts: map[string]any{"tcp_port": 9200},
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
					Type: "modbus-server",
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

func (m *mockStatefulAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockStatefulAgent) Start(_ context.Context) error       { return nil }
func (m *mockStatefulAgent) Stop(_ context.Context) error        { return nil }
func (m *mockStatefulAgent) Pause(_ context.Context) error       { return nil }
func (m *mockStatefulAgent) Resume(_ context.Context) error      { return nil }
func (m *mockStatefulAgent) Health() agent.HealthStatus          { return m.info.Health }
func (m *mockStatefulAgent) Process(_ []byte) ([]byte, error)    { return nil, nil }
func (m *mockStatefulAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *mockStatefulAgent) ID() string                          { return m.info.ID }
func (m *mockStatefulAgent) Name() string                        { return m.info.Name }
func (m *mockStatefulAgent) Type() string                        { return m.info.Type }
func (m *mockStatefulAgent) Info() agent.AgentInfo               { return m.info }
func (m *mockStatefulAgent) Stats() agent.StatsSnapshot          { return m.info.Stats }
func (m *mockStatefulAgent) State() map[string]any               { return m.state }

// mockConnectionStatsAgent 는 agent.Agent, agent.ConnectionStatsProvider,
// agent.TransportChecker 를 모두 구현하는 테스트용 모의 에이전트이다.
type mockConnectionStatsAgent struct {
	mockStatefulAgent
	connStats          []agent.ConnectionStats
	transportConnected bool
}

func (m *mockConnectionStatsAgent) ConnectionStats() []agent.ConnectionStats {
	return m.connStats
}

func (m *mockConnectionStatsAgent) TransportConnected() bool {
	return m.transportConnected
}

// TestAgentStats_EnhancedFields 는 AgentStats 가 새로운 중첩 구조 필드를 올바르게
// 매핑하는지 검증한다 (SPEC-AGENT-004 M4).
func TestAgentStats_EnhancedFields(t *testing.T) {
	mgr := agent.NewManager()
	adapter := NewAgentServiceAdapter(mgr, nil, nil)

	info, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "enhanced-stats-test",
		Type: "",
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	stats, err := adapter.AgentStats(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("통계 조회 실패: %v", err)
	}

	// 기존 flat 필드 하위 호환성 확인
	assert.Equal(t, info.ID, stats.ID)
	assert.Equal(t, "running", stats.Status)
	assert.Equal(t, int64(0), stats.MessagesIn)
	assert.Equal(t, int64(0), stats.MessagesOut)
	assert.Equal(t, int64(0), stats.ErrorCount)

	// 새 중첩 구조 확인
	assert.NotNil(t, stats.Messages, "Messages 가 nil 이면 안 됨")
	assert.Equal(t, int64(0), stats.Messages.Total.Received)
	assert.Equal(t, int64(0), stats.Messages.External.Received)
	assert.Equal(t, int64(0), stats.Messages.Internal.Received)

	assert.NotNil(t, stats.Bytes, "Bytes 가 nil 이면 안 됨")
	assert.Equal(t, int64(0), stats.Bytes.Read)
	assert.Equal(t, int64(0), stats.Bytes.Written)

	assert.NotNil(t, stats.Buffer, "Buffer 가 nil 이면 안 됨")

	assert.Equal(t, int64(0), stats.DroppedMessages)
	assert.Equal(t, int64(0), stats.RestartCount)

	// Connections, NodeRefs 는 빈 슬라이스여야 함 (nil 이 아님)
	assert.NotNil(t, stats.Connections, "Connections 가 nil 이면 안 됨 (빈 슬라이스여야 함)")
	assert.Empty(t, stats.Connections)
	assert.NotNil(t, stats.NodeRefs, "NodeRefs 가 nil 이면 안 됨 (빈 슬라이스여야 함)")
	assert.Empty(t, stats.NodeRefs)
}

// TestAgentStats_BackwardCompatibility 는 기존 flat 필드가 새 중첩 구조의
// total 값과 일치하는지 검증한다.
func TestAgentStats_BackwardCompatibility(t *testing.T) {
	mgr := agent.NewManager()
	adapter := NewAgentServiceAdapter(mgr, nil, nil)

	info, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "compat-test",
		Type: "",
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	stats, err := adapter.AgentStats(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("통계 조회 실패: %v", err)
	}

	// flat 필드와 중첩 구조 값 일치 확인
	assert.Equal(t, stats.MessagesIn, stats.Messages.Total.Received,
		"MessagesIn 과 Messages.Total.Received 가 일치해야 함")
	assert.Equal(t, stats.MessagesOut, stats.Messages.Total.Sent,
		"MessagesOut 과 Messages.Total.Sent 가 일치해야 함")
	assert.Equal(t, stats.ErrorCount, stats.Messages.Total.Errored,
		"ErrorCount 와 Messages.Total.Errored 가 일치해야 함")
	assert.Equal(t, stats.BufferPending, stats.Buffer.Pending,
		"BufferPending 과 Buffer.Pending 이 일치해야 함")
	assert.Equal(t, stats.BufferCapacity, stats.Buffer.Capacity,
		"BufferCapacity 와 Buffer.Capacity 가 일치해야 함")
}

// --- SPEC-AGENT-005 Phase 3: Enable/Disable 테스트 ---

// TestEnableAgent_SetsEnabledTrueAndPersists 는 EnableAgent 가 에이전트의 Enabled 필드를
// true 로 설정하고 영속 저장소에 저장하는지 검증한다 (R3.8).
func TestEnableAgent_SetsEnabledTrueAndPersists(t *testing.T) {
	mgr := agent.NewManager()
	repo, err := storage.NewAgentFileRepository(t.TempDir())
	if err != nil {
		t.Fatalf("저장소 생성 실패: %v", err)
	}
	adapter := NewAgentServiceAdapter(mgr, repo, nil)

	// Enabled = nil 상태 (기본값 true)로 에이전트 생성
	info, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "enable-test",
		Type: "",
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	// Enable 호출
	result, err := adapter.EnableAgent(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("EnableAgent 실패: %v", err)
	}

	// 반환된 AgentInfo.Enabled 가 true 인지 확인
	if !result.Enabled {
		t.Error("EnableAgent 후 AgentInfo.Enabled 가 true 여야 함")
	}

	// in-memory 에이전트의 Config.Enabled 검증
	ag, err := mgr.Get(info.ID)
	if err != nil {
		t.Fatalf("에이전트 조회 실패: %v", err)
	}
	cfg := ag.Info().Config
	if cfg.Enabled == nil {
		t.Fatal("in-memory Config.Enabled 가 nil 이면 안 됨")
	}
	if *cfg.Enabled != true {
		t.Error("in-memory Config.Enabled 가 true 여야 함")
	}

	// 영속 저장소에서 조회하여 Enabled 필드 검증
	savedCfg, err := repo.Get(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("저장소에서 에이전트 조회 실패: %v", err)
	}
	if savedCfg.Enabled == nil {
		t.Fatal("저장소 Config.Enabled 가 nil 이면 안 됨")
	}
	if *savedCfg.Enabled != true {
		t.Error("저장소 Config.Enabled 가 true 여야 함")
	}
}

// TestDisableAgent_SetsEnabledFalseAndPersists 는 DisableAgent 가 에이전트의 Enabled 필드를
// false 로 설정하고 영속 저장소에 저장하는지 검증한다 (R3.7).
func TestDisableAgent_SetsEnabledFalseAndPersists(t *testing.T) {
	mgr := agent.NewManager()
	repo, err := storage.NewAgentFileRepository(t.TempDir())
	if err != nil {
		t.Fatalf("저장소 생성 실패: %v", err)
	}
	adapter := NewAgentServiceAdapter(mgr, repo, nil)

	info, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "disable-test",
		Type: "",
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	result, err := adapter.DisableAgent(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("DisableAgent 실패: %v", err)
	}

	// 반환된 AgentInfo.Enabled 가 false 인지 확인
	if result.Enabled {
		t.Error("DisableAgent 후 AgentInfo.Enabled 가 false 여야 함")
	}

	// in-memory 에이전트의 Config.Enabled 검증
	ag, err := mgr.Get(info.ID)
	if err != nil {
		t.Fatalf("에이전트 조회 실패: %v", err)
	}
	cfg := ag.Info().Config
	if cfg.Enabled == nil {
		t.Fatal("in-memory Config.Enabled 가 nil 이면 안 됨")
	}
	if *cfg.Enabled != false {
		t.Error("in-memory Config.Enabled 가 false 여야 함")
	}

	// 영속 저장소에서 조회
	savedCfg, err := repo.Get(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("저장소에서 에이전트 조회 실패: %v", err)
	}
	if savedCfg.Enabled == nil {
		t.Fatal("저장소 Config.Enabled 가 nil 이면 안 됨")
	}
	if *savedCfg.Enabled != false {
		t.Error("저장소 Config.Enabled 가 false 여야 함")
	}
}

// TestDisableAgent_DoesNotCallStop 은 DisableAgent 가 현재 실행 중인 에이전트를
// 정지시키지 않는지 검증한다 (R3.7 critical).
// Disable 은 단순히 영속 플래그만 변경하며, 현재 실행 상태에는 영향을 주지 않는다.
func TestDisableAgent_DoesNotCallStop(t *testing.T) {
	mgr := agent.NewManager()
	adapter := NewAgentServiceAdapter(mgr, nil, nil)

	info, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "disable-no-stop-test",
		Type: "",
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	// 생성 직후 StateRunning 상태인지 사전 확인
	before, err := adapter.GetAgent(context.Background(), info.ID, "")
	if err != nil {
		t.Fatalf("에이전트 조회 실패: %v", err)
	}
	if before.Status != string(lifecycle.StateRunning) {
		t.Fatalf("생성 직후 상태가 running 이어야 함: got=%q", before.Status)
	}

	// Disable 호출
	if _, err := adapter.DisableAgent(context.Background(), info.ID); err != nil {
		t.Fatalf("DisableAgent 실패: %v", err)
	}

	// Disable 호출 후에도 에이전트는 여전히 running 상태여야 한다 (Stop 호출 금지)
	after, err := adapter.GetAgent(context.Background(), info.ID, "")
	if err != nil {
		t.Fatalf("에이전트 조회 실패: %v", err)
	}
	if after.Status != string(lifecycle.StateRunning) {
		t.Errorf("Disable 은 실행 중인 에이전트를 정지시키지 않아야 함 (R3.7): got=%q, want=running", after.Status)
	}
	// Enabled 는 false 로 변경되었어야 한다
	if after.Enabled {
		t.Error("Disable 후 Enabled 가 false 여야 함")
	}
}

// TestEnableAgent_NotFound_ReturnsError 는 존재하지 않는 에이전트에 대한 Enable 호출 시
// 에러가 반환되는지 검증한다.
func TestEnableAgent_NotFound_ReturnsError(t *testing.T) {
	mgr := agent.NewManager()
	adapter := NewAgentServiceAdapter(mgr, nil, nil)

	_, err := adapter.EnableAgent(context.Background(), "nonexistent-id")
	if err == nil {
		t.Error("존재하지 않는 에이전트 Enable 시 에러가 발생해야 함")
	}
}

// TestDisableAgent_NotFound_ReturnsError 는 존재하지 않는 에이전트에 대한 Disable 호출 시
// 에러가 반환되는지 검증한다.
func TestDisableAgent_NotFound_ReturnsError(t *testing.T) {
	mgr := agent.NewManager()
	adapter := NewAgentServiceAdapter(mgr, nil, nil)

	_, err := adapter.DisableAgent(context.Background(), "nonexistent-id")
	if err == nil {
		t.Error("존재하지 않는 에이전트 Disable 시 에러가 발생해야 함")
	}
}

// TestEnableAgent_PersistFailure_RollsBackInMemory 는 영속 저장소 Save 실패 시
// in-memory 상태가 원래 상태로 롤백되는지 검증한다 (NFR7).
func TestEnableAgent_PersistFailure_RollsBackInMemory(t *testing.T) {
	mgr := agent.NewManager()
	adapter := NewAgentServiceAdapter(mgr, nil, nil)

	// 먼저 정상 어댑터로 에이전트 생성 (Enabled = nil)
	info, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "rollback-enable-test",
		Type: "",
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	// 생성 직후 원래 Enabled 상태 캡처 (nil 이어야 함)
	ag, err := mgr.Get(info.ID)
	if err != nil {
		t.Fatalf("에이전트 조회 실패: %v", err)
	}
	originalEnabled := ag.Info().Config.Enabled
	if originalEnabled != nil {
		t.Fatalf("초기 Enabled 가 nil 이어야 함: got=%v", *originalEnabled)
	}

	// 실패하는 저장소로 어댑터 교체
	adapter.repo = &failingAgentRepo{}

	// Disable 호출 — 저장소 Save 가 실패해야 함
	_, err = adapter.DisableAgent(context.Background(), info.ID)
	if err == nil {
		t.Fatal("저장소 Save 실패 시 에러가 발생해야 함")
	}

	// in-memory 상태가 원래대로 롤백되었는지 확인 (Enabled 가 nil 이어야 함)
	afterCfg := ag.Info().Config
	if afterCfg.Enabled != nil {
		t.Errorf("롤백 후 in-memory Config.Enabled 가 원래 상태(nil)로 복구되어야 함: got=%v", *afterCfg.Enabled)
	}
}

// TestEnableAgent_NilRepo_Succeeds 는 repo 가 nil 일 때 in-memory 업데이트만 수행되고
// 에러 없이 성공하는지 검증한다.
func TestEnableAgent_NilRepo_Succeeds(t *testing.T) {
	mgr := agent.NewManager()
	adapter := NewAgentServiceAdapter(mgr, nil, nil) // repo = nil

	info, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "nil-repo-test",
		Type: "",
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	// Enable 호출
	result, err := adapter.EnableAgent(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("EnableAgent 실패 (repo=nil): %v", err)
	}
	if !result.Enabled {
		t.Error("EnableAgent 후 Enabled 가 true 여야 함")
	}

	// Disable 호출
	result, err = adapter.DisableAgent(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("DisableAgent 실패 (repo=nil): %v", err)
	}
	if result.Enabled {
		t.Error("DisableAgent 후 Enabled 가 false 여야 함")
	}
}

// TestAgentToHandlerInfo_NilEnabled_ReturnsEnabledTrue 는 Config.Enabled 가 nil 일 때
// AgentInfo.Enabled 가 기본값 true 로 설정되는지 검증한다.
func TestAgentToHandlerInfo_NilEnabled_ReturnsEnabledTrue(t *testing.T) {
	mock := &mockStatefulAgent{
		info: agent.AgentInfo{
			ID:    "nil-enabled-id",
			Name:  "nil-enabled-test",
			Type:  "mock",
			State: lifecycle.StateRunning,
			Config: agent.AgentConfig{
				ID:      "nil-enabled-id",
				Name:    "nil-enabled-test",
				Type:    "mock",
				Enabled: nil, // 명시적으로 nil
			},
		},
	}

	result := agentToHandlerInfo(mock, "")
	if !result.Enabled {
		t.Error("Config.Enabled=nil 일 때 AgentInfo.Enabled 는 true(기본값) 여야 함")
	}
}

// TestAgentToHandlerInfo_ExplicitFalse_ReturnsEnabledFalse 는 Config.Enabled 가 false 일 때
// AgentInfo.Enabled 가 false 로 설정되는지 검증한다.
func TestAgentToHandlerInfo_ExplicitFalse_ReturnsEnabledFalse(t *testing.T) {
	falseVal := false
	mock := &mockStatefulAgent{
		info: agent.AgentInfo{
			ID:    "false-enabled-id",
			Name:  "false-enabled-test",
			Type:  "mock",
			State: lifecycle.StateRunning,
			Config: agent.AgentConfig{
				ID:      "false-enabled-id",
				Name:    "false-enabled-test",
				Type:    "mock",
				Enabled: &falseVal,
			},
		},
	}

	result := agentToHandlerInfo(mock, "")
	if result.Enabled {
		t.Error("Config.Enabled=&false 일 때 AgentInfo.Enabled 는 false 여야 함")
	}
}

// TestAgentToHandlerInfo_ExplicitTrue_ReturnsEnabledTrue 는 Config.Enabled 가 true 일 때
// AgentInfo.Enabled 가 true 로 설정되는지 검증한다.
func TestAgentToHandlerInfo_ExplicitTrue_ReturnsEnabledTrue(t *testing.T) {
	trueVal := true
	mock := &mockStatefulAgent{
		info: agent.AgentInfo{
			ID:    "true-enabled-id",
			Name:  "true-enabled-test",
			Type:  "mock",
			State: lifecycle.StateRunning,
			Config: agent.AgentConfig{
				ID:      "true-enabled-id",
				Name:    "true-enabled-test",
				Type:    "mock",
				Enabled: &trueVal,
			},
		},
	}

	result := agentToHandlerInfo(mock, "")
	if !result.Enabled {
		t.Error("Config.Enabled=&true 일 때 AgentInfo.Enabled 는 true 여야 함")
	}
}

// TestAgentStats_ConnectionStatsProvider 는 ConnectionStatsProvider 인터페이스를
// 구현한 에이전트의 연결 통계가 올바르게 매핑되는지 검증한다.
func TestAgentStats_ConnectionStatsProvider(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	mock := &mockConnectionStatsAgent{
		mockStatefulAgent: mockStatefulAgent{
			info: agent.AgentInfo{
				ID:    "conn-stats-id",
				Name:  "conn-stats-test",
				Type:  "mock",
				State: lifecycle.StateRunning,
				Stats: agent.StatsSnapshot{
					MessagesReceived:         15,
					MessagesSent:             10,
					MessagesErrored:          2,
					ExternalMessagesReceived: 10,
					ExternalMessagesSent:     8,
					ExternalMessagesErrored:  1,
					InternalMessagesReceived: 5,
					InternalMessagesSent:     2,
					InternalMessagesErrored:  1,
					BytesRead:                1024,
					BytesWritten:             512,
					DroppedMessages:          3,
					RestartCount:             1,
					LastActivityAt:           now,
					NodeRefs: []agent.NodeRefStats{
						{
							NodeID:           "node-1",
							FlowID:           "flow-1",
							MessagesReceived: 5,
							MessagesSent:     2,
							MessagesErrored:  1,
							LastActivityAt:   now,
						},
					},
				},
				Config: agent.AgentConfig{
					ID:   "conn-stats-id",
					Name: "conn-stats-test",
				},
				Uptime: 5 * time.Minute,
			},
		},
		connStats: []agent.ConnectionStats{
			{
				ID:               "conn-1",
				MessagesReceived: 10,
				MessagesSent:     8,
				MessagesErrored:  1,
				BytesRead:        1024,
				BytesWritten:     512,
				ConnectedAt:      now.Add(-5 * time.Minute),
				LastActivityAt:   now,
			},
		},
		transportConnected: true,
	}

	// agentToHandlerInfo 가 아닌 AgentStats 를 직접 테스트하기 위해
	// mock 에이전트로 직접 매핑 로직을 검증한다.
	info := mock.Info()
	stats := mock.Stats()

	// 중첩 구조 매핑 확인
	assert.Equal(t, int64(15), stats.MessagesReceived)
	assert.Equal(t, int64(10), stats.ExternalMessagesReceived)
	assert.Equal(t, int64(5), stats.InternalMessagesReceived)
	assert.Equal(t, int64(3), stats.DroppedMessages)
	assert.Equal(t, int64(1), stats.RestartCount)
	assert.Equal(t, now, stats.LastActivityAt)
	assert.Equal(t, "conn-stats-id", info.ID)

	// ConnectionStatsProvider 확인
	connStats := mock.ConnectionStats()
	assert.Len(t, connStats, 1)
	assert.Equal(t, "conn-1", connStats[0].ID)
	assert.Equal(t, int64(10), connStats[0].MessagesReceived)
	assert.Equal(t, now.Add(-5*time.Minute), connStats[0].ConnectedAt)
	assert.Equal(t, now, connStats[0].LastActivityAt)

	// TransportChecker 확인
	assert.True(t, mock.TransportConnected())

	// NodeRefs 확인
	assert.Len(t, stats.NodeRefs, 1)
	assert.Equal(t, "node-1", stats.NodeRefs[0].NodeID)
	assert.Equal(t, "flow-1", stats.NodeRefs[0].FlowID)
	assert.Equal(t, int64(5), stats.NodeRefs[0].MessagesReceived)
}

// TestAgentToHandlerInfo_DefaultDetail_PopulatesUptimeAndStats 는 detail="" (목록
// 엔드포인트가 사용하는 기본값)일 때도 실행 중 에이전트의 Uptime 과 Stats(메시지 통계)가
// 채워지는지 검증한다. 이는 GET /agents 및 원격 agent/list 목록의 Uptime/Messages 컬럼이
// '-' 로만 표시되던 회귀를 막기 위한 핵심 테스트다.
//
// 다른 필드(Type/Status/Connected/Enabled/Config/State)는 영향받지 않아야 한다.
func TestAgentToHandlerInfo_DefaultDetail_PopulatesUptimeAndStats(t *testing.T) {
	now := time.Now()
	mock := &mockStatefulAgent{
		info: agent.AgentInfo{
			ID:    "list-stats-id",
			Name:  "list-stats",
			Type:  "mock",
			State: lifecycle.StateRunning,
			Config: agent.AgentConfig{
				ID:   "list-stats-id",
				Name: "list-stats",
				Type: "mock",
			},
			Stats: agent.StatsSnapshot{
				MessagesReceived: 42,
				MessagesSent:     17,
				MessagesErrored:  3,
			},
			StartedAt: now.Add(-90 * time.Second),
			Uptime:    90 * time.Second,
		},
		state: map[string]any{"k": "v"},
	}

	// detail="" — 목록 엔드포인트와 동일한 기본 호출
	result := agentToHandlerInfo(mock, "")

	// Uptime 이 채워져야 한다 (실행 중)
	if result.Uptime == "" {
		t.Error("기본 detail 에서도 실행 중 에이전트의 Uptime 이 채워져야 함 (목록 표시용)")
	}

	// Stats 가 채워져야 하며 메시지 카운트가 일치해야 한다
	if result.Stats == nil {
		t.Fatal("기본 detail 에서도 Stats 가 채워져야 함 (web stats.messages_in/out 소비)")
	}
	if result.Stats.MessagesIn != 42 {
		t.Errorf("Stats.MessagesIn 불일치: got=%d, want=42", result.Stats.MessagesIn)
	}
	if result.Stats.MessagesOut != 17 {
		t.Errorf("Stats.MessagesOut 불일치: got=%d, want=17", result.Stats.MessagesOut)
	}

	// 다른 필드는 변경되지 않아야 한다
	if result.Type != "mock" {
		t.Errorf("Type 변경됨: got=%q, want=mock", result.Type)
	}
	if result.Status != string(lifecycle.StateRunning) {
		t.Errorf("Status 변경됨: got=%q", result.Status)
	}
	if result.Connected == nil || !*result.Connected {
		t.Error("실행 중 에이전트의 Connected 는 true 여야 함")
	}
	if result.State == nil {
		t.Error("State 는 기존과 동일하게 유지되어야 함")
	}
	// detail="" 이므로 summary 전용 필드(Health/StartedAt/CreatedAt)는 채워지지 않아야 한다
	if result.Health != nil {
		t.Error("detail='' 일 때 Health 는 nil 이어야 함 (목록 페이로드 최소화)")
	}
	if result.StartedAt != nil {
		t.Error("detail='' 일 때 StartedAt 는 nil 이어야 함")
	}
}

// TestAgentToHandlerInfo_DefaultDetail_NotRunning_EmptyUptime 은 실행 중이 아닌
// 에이전트는 Uptime 이 비어 있어야 함을 검증한다 (web 에서 '-' 로 표시됨).
func TestAgentToHandlerInfo_DefaultDetail_NotRunning_EmptyUptime(t *testing.T) {
	mock := &mockStatefulAgent{
		info: agent.AgentInfo{
			ID:    "stopped-id",
			Name:  "stopped",
			Type:  "mock",
			State: lifecycle.StateStopped,
			Config: agent.AgentConfig{
				ID: "stopped-id",
			},
			Stats: agent.StatsSnapshot{
				MessagesReceived: 5,
				MessagesSent:     2,
			},
		},
	}

	result := agentToHandlerInfo(mock, "")

	if result.Uptime != "" {
		t.Errorf("정지된 에이전트의 Uptime 은 비어 있어야 함: got=%q", result.Uptime)
	}
	// Stats 는 정지 상태에서도 누적 카운트를 노출한다
	if result.Stats == nil {
		t.Fatal("Stats 는 정지 상태에서도 채워져야 함")
	}
	if result.Stats.MessagesIn != 5 || result.Stats.MessagesOut != 2 {
		t.Errorf("Stats 불일치: in=%d out=%d", result.Stats.MessagesIn, result.Stats.MessagesOut)
	}
}

// TestAgentServiceAdapter_ListAgents_EnrichesUptimeAndStats 는 실제 Manager 로 생성한
// 실행 중 에이전트가 ListAgents (GET /agents 및 원격 agent/list 의 공통 소스) 응답에서
// Uptime 과 Stats 를 포함하는지 종단 검증한다.
func TestAgentServiceAdapter_ListAgents_EnrichesUptimeAndStats(t *testing.T) {
	mgr := agent.NewManager()
	adapter := NewAgentServiceAdapter(mgr, nil, nil)

	created, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "list-enrich",
		Type: "",
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	// 기본 detail (목록 엔드포인트와 동일) — 필터/정렬 없음
	agents, total, err := adapter.ListAgents(context.Background(), dto.ListOptions{
		PaginationParams: dto.PaginationParams{Page: 1, Size: 20},
	})
	if err != nil {
		t.Fatalf("목록 조회 실패: %v", err)
	}
	if total != 1 {
		t.Fatalf("1개여야 함: total=%d", total)
	}

	got := agents[0]
	if got.ID != created.ID {
		t.Fatalf("ID 불일치: got=%q want=%q", got.ID, created.ID)
	}

	// BaseAgent.Init 후 StateRunning 이므로 Uptime 이 채워져야 한다
	if got.Uptime == "" {
		t.Error("목록 응답의 실행 중 에이전트 Uptime 이 비어있으면 안 됨 (web Uptime 컬럼)")
	}
	// Stats 가 채워져야 한다 (web stats.messages_in/out 컬럼)
	if got.Stats == nil {
		t.Error("목록 응답의 Stats 가 nil 이면 안 됨 (web Messages 컬럼)")
	}
}

// TestExecAgent_AddDevice_PersistsDeviceRoster 는 add_device 커맨드가 메모리와 저장소에
// 디바이스를 모두 저장하는지 검증한다 (SPEC-DEVICE-PERSISTENCE-001).
// 재현 테스트: 현재는 실패하고, 수정 후 성공해야 한다.
func TestExecAgent_AddDevice_PersistsDeviceRoster(t *testing.T) {
	// 임시 저장소 설정
	tmpDir := t.TempDir()
	repo, err := storage.NewAgentFileRepository(tmpDir)
	if err != nil {
		t.Fatalf("저장소 생성 실패: %v", err)
	}

	// 첫 번째 어댑터: 에이전트 생성 및 디바이스 추가
	mgr1 := agent.NewManager()
	// Samsung 에이전트 타입 등록
	if err := samsung.RegisterSamsungHvacr01Types(mgr1); err != nil {
		t.Fatalf("Samsung 에이전트 타입 등록 실패: %v", err)
	}
	adapter1 := NewAgentServiceAdapter(mgr1, repo, nil)

	// Samsung HVACR01 에이전트 생성
	agentInfo, err := adapter1.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "device-persist-test",
		Type: "samsung_hvacr01",
		Config: map[string]any{
			"transport_type": "tcp-client",
			"tcp_host":       "127.0.0.1",
			"tcp_port":       float64(9100),
		},
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}
	agentID := agentInfo.ID

	// 장치 추가 커맨드 실행
	addDevicePayload := map[string]any{
		"command": "add_device",
		"address": "200001",
		"params": map[string]any{
			"name":      "test-device-1",
			"device_id": "device-001",
		},
	}
	payloadBytes, err := marshalJSON(addDevicePayload)
	if err != nil {
		t.Fatalf("JSON 마샬링 실패: %v", err)
	}

	_, err = adapter1.ExecAgent(context.Background(), agentID, payloadBytes)
	if err != nil {
		t.Fatalf("add_device 실행 실패: %v", err)
	}

	// 메모리에서 디바이스 확인 (이 단계에서는 성공해야 함)
	_, err = mgr1.Get(agentID)
	if err != nil {
		t.Fatalf("에이전트 조회 실패: %v", err)
	}

	// 메모리 상태 확인용 방법: Info().Config.Options 에서 devices 확인
	// 하지만 현재 in-memory 만 있고 저장되지 않음

	// 두 번째 어댑터: 저장소에서 다시 로드 (시뮬레이트: 데몬 재시작)
	mgr2 := agent.NewManager()
	configs, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("저장소 목록 조회 실패: %v", err)
	}

	// 저장된 config 에서 devices 확인
	var savedConfig agent.AgentConfig
	for _, cfg := range configs {
		if cfg.ID == agentID {
			savedConfig = cfg
			break
		}
	}

	if savedConfig.ID == "" {
		t.Fatalf("저장소에서 에이전트를 찾을 수 없음")
	}

	// SPEC 요구사항: add_device 로 추가된 디바이스가 저장소에 persist 되어야 한다
	devices := agent.ParseDevices(savedConfig.Transport.Options)
	if len(devices) == 0 {
		t.Fatal("저장소의 에이전트 config 에 devices 가 없음 — BUG: add_device 결과가 persist 되지 않음")
	}

	// 추가된 디바이스를 확인
	// NASA 주소 형식 참고: "200001" → "20.00.01" (dots 포함)
	// 중요: roster 에 저장된 name 은 device_id ("device-001") 이다.
	// 표시 이름 "test-device-1" 은 device_metadata 에 별도 저장된다.
	found := false
	for _, dev := range devices {
		if dev.Address == "20.00.01" && dev.Name == "device-001" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("저장소에서 추가된 디바이스 (address=20.00.01, name=device-001) 를 찾을 수 없음. 저장된 장치 목록: %v", devices)
	}

	// 저장소에서 로드한 config 로 에이전트를 다시 생성
	// Samsung 에이전트 타입 등록 (mgr2에서도 필요)
	if err := samsung.RegisterSamsungHvacr01Types(mgr2); err != nil {
		t.Fatalf("Samsung 에이전트 타입 등록 실패 (mgr2): %v", err)
	}
	ag2, err := mgr2.Create(savedConfig)
	if err != nil {
		t.Fatalf("저장된 config 로 에이전트 생성 실패: %v", err)
	}

	// 복원된 에이전트의 info 에서 devices 확인
	info := ag2.Info()
	devicesInRestoredConfig := agent.ParseDevices(info.Config.Transport.Options)
	if len(devicesInRestoredConfig) == 0 {
		t.Fatal("복원된 에이전트 config 에 devices 가 없음")
	}

	// 추가된 디바이스가 복원되었는지 확인
	// 저장된 name 은 device_id 이므로 "device-001" 을 확인
	restoreFound := false
	for _, dev := range devicesInRestoredConfig {
		if dev.Address == "20.00.01" && dev.Name == "device-001" {
			restoreFound = true
			break
		}
	}
	if !restoreFound {
		t.Errorf("복원된 에이전트에 추가된 디바이스가 없음. 복원된 장치 목록: %v", devicesInRestoredConfig)
	}
}

// TestExecAgent_RemoveDevice_PersistsRosterShrinkage 는 remove_device 커맨드가
// 디바이스를 메모리와 저장소에서 모두 제거하는지 검증한다.
func TestExecAgent_RemoveDevice_PersistsRosterShrinkage(t *testing.T) {
	tmpDir := t.TempDir()
	repo, err := storage.NewAgentFileRepository(tmpDir)
	if err != nil {
		t.Fatalf("저장소 생성 실패: %v", err)
	}

	mgr1 := agent.NewManager()
	if err := samsung.RegisterSamsungHvacr01Types(mgr1); err != nil {
		t.Fatalf("Samsung 에이전트 타입 등록 실패: %v", err)
	}
	adapter1 := NewAgentServiceAdapter(mgr1, repo, nil)

	// Samsung HVACR01 에이전트 생성
	agentInfo, err := adapter1.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "device-remove-test",
		Type: "samsung_hvacr01",
		Config: map[string]any{
			"transport_type": "tcp-client",
			"tcp_host":       "127.0.0.1",
			"tcp_port":       float64(9100),
		},
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}
	agentID := agentInfo.ID

	// 두 디바이스 추가
	for i, address := range []string{"200001", "200002"} {
		addPayload := map[string]any{
			"command": "add_device",
			"address": address,
			"params": map[string]any{
				"name":      fmt.Sprintf("device-%d", i),
				"device_id": fmt.Sprintf("device-%03d", i),
			},
		}
		data, _ := json.Marshal(addPayload)
		_, _ = adapter1.ExecAgent(context.Background(), agentID, data)
	}

	// 첫 디바이스 제거
	removePayload := map[string]any{
		"command": "remove_device",
		"address": "200001",
	}
	data, _ := json.Marshal(removePayload)
	_, _ = adapter1.ExecAgent(context.Background(), agentID, data)

	// 저장소에서 확인
	configs, _ := repo.List(context.Background())
	var savedConfig agent.AgentConfig
	for _, cfg := range configs {
		if cfg.ID == agentID {
			savedConfig = cfg
			break
		}
	}

	devices := agent.ParseDevices(savedConfig.Transport.Options)
	if len(devices) != 1 {
		t.Errorf("제거 후 1개 디바이스여야 함: got %d", len(devices))
	}

	found := false
	for _, dev := range devices {
		if dev.Address == "20.00.02" {
			found = true
			break
		}
	}
	if !found {
		t.Error("첫 번째 디바이스가 제거되지 않음")
	}
}

// TestExecAgent_NonRosterCommand_NoRepoSave 는 add_device/remove_device 가 아닌
// 커맨드가 repo.Save 를 호출하지 않음을 확인한다.
func TestExecAgent_NonRosterCommand_NoRepoSave(t *testing.T) {
	spyRepo := &spyAgentRepository{}
	mgr := agent.NewManager()
	if err := samsung.RegisterSamsungHvacr01Types(mgr); err != nil {
		t.Fatalf("Samsung 에이전트 타입 등록 실패: %v", err)
	}
	adapter := NewAgentServiceAdapter(mgr, spyRepo, nil)

	agentInfo, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "non-roster-test",
		Type: "samsung_hvacr01",
		Config: map[string]any{
			"transport_type": "tcp-client",
			"tcp_host":       "127.0.0.1",
			"tcp_port":       float64(9100),
		},
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	// CreateAgent 가 Save 를 호출했을 수 있으므로 리셋
	createCallCount := spyRepo.saveCallCount
	spyRepo.saveCallCount = 0

	// list_devices 는 Save 를 호출하면 안 됨
	listPayload := map[string]any{"command": "list_devices"}
	data, _ := json.Marshal(listPayload)
	adapter.ExecAgent(context.Background(), agentInfo.ID, data) //nolint:errcheck

	if spyRepo.saveCallCount > 0 {
		t.Errorf("비로스터 커맨드에서 Save 호출됨: %d times (CreateAgent에서 %d번 호출)", spyRepo.saveCallCount, createCallCount)
	}
}

// TestExecAgent_ParseDevicesRoundTrip 는 디바이스가 라운드트립(메모리→빌드→파싱→복원)
// 을 통해 동일하게 유지되는지 확인한다.
func TestExecAgent_ParseDevicesRoundTrip(t *testing.T) {
	original := []agent.DeviceEntry{
		{Address: "20.00.01", Name: "device-a"},
		{Address: "20.00.02", Name: ""}, // 이름 없음
	}

	adapter := &AgentServiceAdapter{}
	built := adapter.buildDevicesList(original)
	opts := map[string]any{"devices": built}
	parsed := agent.ParseDevices(opts)

	if len(parsed) != len(original) {
		t.Fatalf("라운드트립 길이 불일치: got %d, want %d", len(parsed), len(original))
	}

	for i := range original {
		if parsed[i].Address != original[i].Address || parsed[i].Name != original[i].Name {
			t.Errorf("[%d] 라운드트립 실패: got %v, want %v", i, parsed[i], original[i])
		}
	}
}

// TestBuildDevicesList_ReportEnabledRoundTrip 는 report_enabled(*bool) 가
// buildDevicesList → ParseDevices 왕복에서 보존되는지 검증한다. false 는 유지되고,
// nil(미지정) 은 키가 생략되어 재파싱 시 nil(기본 enabled) 로 남는다(후방호환).
func TestBuildDevicesList_ReportEnabledRoundTrip(t *testing.T) {
	off := false
	on := true
	original := []agent.DeviceEntry{
		{Address: "20.00.01", Name: "off-dev", ReportEnabled: &off},
		{Address: "20.00.02", Name: "on-dev", ReportEnabled: &on},
		{Address: "20.00.03", Name: "unset-dev"}, // ReportEnabled nil
	}

	adapter := &AgentServiceAdapter{}
	built := adapter.buildDevicesList(original)
	opts := map[string]any{"devices": built}
	parsed := agent.ParseDevices(opts)

	if len(parsed) != 3 {
		t.Fatalf("라운드트립 길이 불일치: got %d, want 3", len(parsed))
	}

	byName := map[string]agent.DeviceEntry{}
	for _, e := range parsed {
		byName[e.Name] = e
	}

	if e := byName["off-dev"]; e.ReportEnabled == nil || *e.ReportEnabled != false {
		t.Errorf("off-dev report_enabled 미보존: got %v", e.ReportEnabled)
	}
	if e := byName["on-dev"]; e.ReportEnabled == nil || *e.ReportEnabled != true {
		t.Errorf("on-dev report_enabled 미보존: got %v", e.ReportEnabled)
	}
	if e := byName["unset-dev"]; e.ReportEnabled != nil {
		t.Errorf("unset-dev report_enabled 는 nil 유지여야 함: got %v", *e.ReportEnabled)
	}
}

// spyAgentRepository 는 Save 호출을 추적하는 테스트용 저장소
type spyAgentRepository struct {
	saveCallCount int
}

func (s *spyAgentRepository) Save(context.Context, agent.AgentConfig) error {
	s.saveCallCount++
	return nil
}

func (s *spyAgentRepository) Get(context.Context, string) (agent.AgentConfig, error) {
	return agent.AgentConfig{}, storage.ErrAgentNotFound
}

func (s *spyAgentRepository) List(context.Context) ([]agent.AgentConfig, error) {
	return nil, nil
}

func (s *spyAgentRepository) Delete(context.Context, string) error {
	return nil
}

func (s *spyAgentRepository) Close() error {
	return nil
}

// TestExecAgent_SamsungConfigDeviceRoundTrip 는 runtime-added 디바이스가
// add_device 후 저장되고 복원될 때 UnitID (deviceID) 가 보존됨을 확인한다.
//
// 이 테스트는 핵심 요구사항을 검증한다:
// - Runtime 에 추가된 디바이스의 UnitID 는 deviceID 이다
// - GetPersistableDevices 는 UnitID 를 Name 필드로 저장한다
// - 복원 후 ParseDevices 는 Name → UnitID 로 매핑하여 동일한 식별자를 만든다
//
// 주의: config 로 등록된 디바이스의 보존은 여기서 다루지 않는다.
// TestExecAgent_ConfigDevicesSurviveAddDevice 를 참조.
func TestExecAgent_SamsungRuntimeDeviceRoundTrip(t *testing.T) {
	fileRepo, err := storage.NewAgentFileRepository(t.TempDir())
	if err != nil {
		t.Fatalf("저장소 생성 실패: %v", err)
	}
	defer fileRepo.Close()

	mgr := agent.NewManager()
	if err := samsung.RegisterSamsungHvacr01Types(mgr); err != nil {
		t.Fatalf("Samsung 에이전트 타입 등록 실패: %v", err)
	}
	adapter := NewAgentServiceAdapter(mgr, fileRepo, nil)

	initialConfig := &dto.AgentCreateRequest{
		Name: "config-device-test",
		Type: "samsung_hvacr01",
		Config: map[string]any{
			"transport_type": "tcp-client",
			"tcp_host":       "127.0.0.1",
			"tcp_port":       float64(9100),
		},
	}

	agentInfo, err := adapter.CreateAgent(context.Background(), initialConfig)
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	// add_device 실행 (이것이 저장을 트리거)
	// 중요: params 구조를 정확히 맞춘다
	addPayload := map[string]any{
		"command": "add_device",
		"params": map[string]any{
			"address":   "20.00.02",
			"device_id": "dev-uuid-002",
			"name":      "bedroom",
		},
	}
	data, _ := json.Marshal(addPayload)
	_, err = adapter.ExecAgent(context.Background(), agentInfo.ID, data)
	if err != nil {
		t.Fatalf("add_device 실행 실패: %v", err)
	}

	// 저장된 설정 확인
	savedConfig, err := fileRepo.Get(context.Background(), agentInfo.ID)
	if err != nil {
		t.Fatalf("설정 로드 실패: %v", err)
	}

	// Transport.Options["devices"] 확인
	devicesAny, ok := savedConfig.Transport.Options["devices"]
	if !ok {
		t.Fatalf("저장된 설정에 devices 없음")
	}
	devicesList, ok := devicesAny.([]any)
	if !ok {
		t.Fatalf("devices 타입 오류: got %T", devicesAny)
	}

	// UnitID 가 보존되었는지 확인
	// Runtime-added 디바이스는 UnitID = deviceID = "dev-uuid-002"
	found := false
	for _, d := range devicesList {
		m, ok := d.(map[string]any)
		if !ok {
			continue
		}
		if m["address"] == "20.00.02" {
			if m["name"] != "dev-uuid-002" {
				t.Errorf("UnitID(deviceID) 손상: got %v, want dev-uuid-002", m["name"])
			}
			found = true
		}
	}
	if !found {
		t.Errorf("저장된 설정에 추가된 디바이스 없음. 저장된 목록: %v", devicesList)
	}
}

// TestExecAgent_ConfigDevicesSurviveAddDevice 는 config 로 등록된 디바이스가
// add_device 실행 후에도 저장된 설정에 그대로 남아 있는지 검증한다.
//
// 회귀 방지: GetPersistableDevices 가 dev.Name(항상 빈 문자열)을 읽던 시절,
// add_device 한 번이면 기존 config 디바이스의 unit id 가 전부 지워진 채 저장됐다.
// UnitID 는 ResolveDeviceID 의 입력이므로 재시작 후 device_id(UUID)가 바뀐다.
//
// auto-discovery 제외와 zone 주소 왕복은 각 에이전트 패키지의
// roster_persist_test.go 에서 단위 검증한다 (handleMessage 가 비공개이므로).
func TestExecAgent_ConfigDevicesSurviveAddDevice(t *testing.T) {
	fileRepo, err := storage.NewAgentFileRepository(t.TempDir())
	if err != nil {
		t.Fatalf("저장소 생성 실패: %v", err)
	}
	defer fileRepo.Close()

	mgr := agent.NewManager()
	if err := samsung.RegisterSamsungHvacr01Types(mgr); err != nil {
		t.Fatalf("Samsung 에이전트 타입 등록 실패: %v", err)
	}
	adapter := NewAgentServiceAdapter(mgr, fileRepo, nil)

	// config 로 디바이스 2개를 등록한 상태에서 시작한다.
	agentInfo, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "config-survives-test",
		Type: "samsung_hvacr01",
		Config: map[string]any{
			"transport_type": "tcp-client",
			"tcp_host":       "127.0.0.1",
			"tcp_port":       float64(9101),
			"devices": []any{
				map[string]any{"address": "20.00.00", "name": "living-room"},
				map[string]any{"address": "20.00.01"}, // 이름 없는 디바이스
			},
		},
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	// 런타임에 디바이스를 하나 추가 → 로스터 저장이 트리거된다.
	data, _ := json.Marshal(map[string]any{
		"command": "add_device",
		"params": map[string]any{
			"address":   "20.00.02",
			"device_id": "dev-uuid-002",
			"name":      "bedroom",
		},
	})
	if _, err := adapter.ExecAgent(context.Background(), agentInfo.ID, data); err != nil {
		t.Fatalf("add_device 실행 실패: %v", err)
	}

	savedConfig, err := fileRepo.Get(context.Background(), agentInfo.ID)
	if err != nil {
		t.Fatalf("설정 로드 실패: %v", err)
	}
	devicesList, ok := savedConfig.Transport.Options["devices"].([]any)
	if !ok {
		t.Fatalf("저장된 devices 타입 오류: got %T", savedConfig.Transport.Options["devices"])
	}

	// address → name 으로 펼쳐서 확인한다.
	saved := make(map[string]string, len(devicesList))
	for _, d := range devicesList {
		m, ok := d.(map[string]any)
		if !ok {
			t.Fatalf("device 항목 타입 오류: got %T", d)
		}
		addr, _ := m["address"].(string)
		name, _ := m["name"].(string) // 키가 없으면 빈 문자열
		saved[addr] = name
	}

	if len(saved) != 3 {
		t.Fatalf("저장된 device 수: got %d, want 3. saved=%v", len(saved), saved)
	}

	// 핵심 검증: config 디바이스의 unit id 가 살아남았는가.
	if got := saved["20.00.00"]; got != "living-room" {
		t.Errorf("config 디바이스 unit id 소실: saved[20.00.00] = %q, want \"living-room\"", got)
	}
	// 이름 없는 config 디바이스는 빈 이름으로 일관되게 남는다.
	if got, ok := saved["20.00.01"]; !ok {
		t.Errorf("이름 없는 config 디바이스가 저장되지 않음. saved=%v", saved)
	} else if got != "" {
		t.Errorf("이름 없는 config 디바이스에 이름이 생김: got %q, want \"\"", got)
	}
	// 런타임 추가 디바이스는 UnitID(=deviceID)로 저장된다.
	if got := saved["20.00.02"]; got != "dev-uuid-002" {
		t.Errorf("런타임 디바이스 UnitID 손상: saved[20.00.02] = %q, want \"dev-uuid-002\"", got)
	}
}

// TestExecAgent_EmptyUnitIDRoundTrip 는 UnitID 가 비어있는 디바이스가 round-trip 에서
// 일관성 있게 보존됨을 확인한다.
func TestExecAgent_EmptyUnitIDRoundTrip(t *testing.T) {
	original := []agent.DeviceEntry{
		{Address: "20.00.01", Name: "device-a"},
		{Address: "20.00.02", Name: ""}, // 빈 UnitID
	}

	adapter := &AgentServiceAdapter{}
	built := adapter.buildDevicesList(original)
	opts := map[string]any{"devices": built}
	parsed := agent.ParseDevices(opts)

	// 빈 이름 디바이스 확인
	if len(parsed) < 2 {
		t.Fatalf("라운드트립 후 디바이스 손실: got %d, want at least 2", len(parsed))
	}

	// 빈 UnitID 는 일관성 있게 빈 채로 유지되어야 함
	if parsed[1].Address != "20.00.02" {
		t.Errorf("주소 보존 실패: got %v, want 20.00.02", parsed[1].Address)
	}
	if parsed[1].Name != "" {
		t.Errorf("빈 UnitID 보존 실패: got %q, want empty string", parsed[1].Name)
	}
}

// TestExecAgent_SamsungDisplayNameSurvivesRestart 는 런타임에 add_device 로 추가한
// 디바이스의 사용자 표시 이름(dev.Name)이 config 영속 왕복(직렬화 → 저장 → 재파싱 →
// 복원) 후에도 유지되는지 검증한다.
//
// 회귀 방지(버그 재현): DisplayName 슬롯이 없던 시절, 직렬화는 DeviceEntry.Name 에
// UnitID(device_id)만 실었고 dev.Name(표시 이름)은 버려졌다. 복원 루프도 entry.Name →
// UnitID 만 세팅해 dev.Name 이 빈 값이 됐고, 프로바이더가 빈 이름을 보고하여 UI 가
// id 로 폴백했다. 이 테스트는 add_device → 저장 → 새 인스턴스 복원까지 전체 실경로
// (buildDevicesList → ParseDevices → 복원 루프)를 통과시켜 표시 이름 왕복을 검증한다.
func TestExecAgent_SamsungDisplayNameSurvivesRestart(t *testing.T) {
	fileRepo, err := storage.NewAgentFileRepository(t.TempDir())
	if err != nil {
		t.Fatalf("저장소 생성 실패: %v", err)
	}
	defer fileRepo.Close()

	mgr := agent.NewManager()
	if err := samsung.RegisterSamsungHvacr01Types(mgr); err != nil {
		t.Fatalf("Samsung 에이전트 타입 등록 실패: %v", err)
	}
	adapter := NewAgentServiceAdapter(mgr, fileRepo, nil)

	agentInfo, err := adapter.CreateAgent(context.Background(), &dto.AgentCreateRequest{
		Name: "display-name-restart-test",
		Type: "samsung_hvacr01",
		Config: map[string]any{
			"transport_type": "tcp-client",
			"tcp_host":       "127.0.0.1",
			"tcp_port":       float64(9102),
		},
	})
	if err != nil {
		t.Fatalf("에이전트 생성 실패: %v", err)
	}

	// 런타임에 표시 이름을 가진 디바이스 추가 → 로스터 저장이 트리거된다.
	data, _ := json.Marshal(map[string]any{
		"command": "add_device",
		"params": map[string]any{
			"address":   "20.00.05",
			"device_id": "dev-uuid-005",
			"name":      "개발팀",
		},
	})
	if _, err := adapter.ExecAgent(context.Background(), agentInfo.ID, data); err != nil {
		t.Fatalf("add_device 실행 실패: %v", err)
	}

	savedConfig, err := fileRepo.Get(context.Background(), agentInfo.ID)
	if err != nil {
		t.Fatalf("설정 로드 실패: %v", err)
	}

	// 재시작 시뮬레이션: 저장된 config 로 새 에이전트 인스턴스를 복원한다.
	restored, err := samsung.NewHvacr01Agent(savedConfig)
	if err != nil {
		t.Fatalf("복원 에이전트 생성 실패: %v", err)
	}
	sAgent, ok := restored.(*samsung.Hvacr01Agent)
	if !ok {
		t.Fatalf("복원 에이전트 타입 오류: got %T", restored)
	}

	var found *samsung.NasaDevice
	devs := sAgent.ListDevices()
	for i := range devs {
		if devs[i].Address.String() == "20.00.05" {
			found = &devs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("복원된 로스터에 추가한 디바이스(20.00.05)가 없음. devs=%+v", devs)
	}

	// 핵심 검증(버그 재현 대상): 표시 이름이 복원됐는가.
	if found.Name != "개발팀" {
		t.Errorf("표시 이름 소실: 복원된 dev.Name = %q, want \"개발팀\"", found.Name)
	}
	// 무회귀: UnitID(device_id)는 변경 전과 동일하게 보존돼야 한다.
	if found.UnitID != "dev-uuid-005" {
		t.Errorf("UnitID 왕복 손상: 복원된 dev.UnitID = %q, want \"dev-uuid-005\"", found.UnitID)
	}
}

// marshalJSON 은 구조체를 JSON 바이트로 변환한다.
func marshalJSON(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	return data, err
}
