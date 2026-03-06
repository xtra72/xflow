package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/storage"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// AgentServiceAdapter 는 handler.AgentManager 인터페이스를 구현하여
// agent.Manager 와 연결하는 서비스 어댑터이다.
type AgentServiceAdapter struct {
	manager agent.Manager
	repo    storage.AgentRepository // 영속 저장소 (nil 허용)
	logger  *slog.Logger
}

// NewAgentServiceAdapter 는 새 AgentServiceAdapter 를 생성한다.
func NewAgentServiceAdapter(mgr agent.Manager, repo storage.AgentRepository, logger *slog.Logger) *AgentServiceAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &AgentServiceAdapter{
		manager: mgr,
		repo:    repo,
		logger:  logger,
	}
}

// ListAgents 는 페이지네이션을 적용하여 에이전트 목록을 반환한다.
func (a *AgentServiceAdapter) ListAgents(ctx context.Context, opts dto.ListOptions) ([]handler.AgentInfo, int64, error) {
	agents := a.manager.List()
	var result []handler.AgentInfo

	for _, ag := range agents {
		info := agentToHandlerInfo(ag, "")
		if opts.Status == "" || info.Status == opts.Status {
			result = append(result, *info)
		}
	}

	total := int64(len(result))
	start := opts.Offset()
	if start >= len(result) {
		return []handler.AgentInfo{}, total, nil
	}
	end := start + opts.Size
	if end > len(result) {
		end = len(result)
	}

	return result[start:end], total, nil
}

// GetAgent 는 ID 로 에이전트를 조회한다.
func (a *AgentServiceAdapter) GetAgent(ctx context.Context, id string, detail string) (*handler.AgentInfo, error) {
	ag, err := a.manager.Get(id)
	if err != nil {
		return nil, err
	}
	return agentToHandlerInfo(ag, detail), nil
}

// CreateAgent 는 새 에이전트를 생성한다.
func (a *AgentServiceAdapter) CreateAgent(ctx context.Context, req *dto.AgentCreateRequest) (*handler.AgentInfo, error) {
	cfg := agent.AgentConfig{
		ID:   uuid.New().String(),
		Name: req.Name,
		Type: req.Type,
	}

	if req.Config != nil {
		// Transport.Options에 설정 전달 (typed agent 팩토리에서 사용)
		cfg.Transport = agent.TransportConfig{
			Type:    req.Type,
			Options: req.Config,
		}

		// Metadata에도 문자열로 저장 (API 응답용)
		cfg.Metadata = make(map[string]string, len(req.Config))
		for k, v := range req.Config {
			cfg.Metadata[k] = fmt.Sprint(v)
		}
	}

	ag, err := a.manager.Create(cfg)
	if err != nil {
		return nil, err
	}

	// 영속 저장소에 저장
	if a.repo != nil {
		if err := a.repo.Save(ctx, cfg); err != nil {
			// 롤백: Manager 에서도 삭제
			_ = a.manager.Delete(cfg.ID)
			a.logger.Error("agent 저장소 저장 실패, 롤백 수행", "agentID", cfg.ID, "error", err)
			return nil, fmt.Errorf("persist agent: %w", err)
		}
	}

	a.logger.Info("agent created", "agentID", ag.ID(), "agentName", ag.Name())
	return agentToHandlerInfo(ag, ""), nil
}

// UpdateAgent 는 에이전트 설정을 업데이트한다.
func (a *AgentServiceAdapter) UpdateAgent(ctx context.Context, id string, req *dto.AgentUpdateRequest) (*handler.AgentInfo, error) {
	ag, err := a.manager.Get(id)
	if err != nil {
		return nil, err
	}

	// 현재 설정을 기반으로 업데이트
	info := ag.Info()
	cfg := info.Config

	if req.Name != nil {
		cfg.Name = *req.Name
	}
	if req.Config != nil {
		// Transport.Options에 설정 전달 (typed agent 팩토리에서 파싱하는 중첩 구조 보존)
		cfg.Transport.Options = req.Config

		// Metadata에 평탄한 문자열 값만 저장 (API 응답용)
		cfg.Metadata = make(map[string]string, len(req.Config))
		for k, v := range req.Config {
			switch v.(type) {
			case map[string]any, []any:
				// 중첩 객체/배열은 Metadata에 저장하지 않음 (Transport.Options에서 관리)
			default:
				cfg.Metadata[k] = fmt.Sprint(v)
			}
		}
	}

	if err := ag.Configure(cfg); err != nil {
		return nil, err
	}

	// 영속 저장소 갱신
	if a.repo != nil {
		if err := a.repo.Save(ctx, cfg); err != nil {
			a.logger.Warn("agent 저장소 갱신 실패", "agentID", id, "error", err)
		}
	}

	return agentToHandlerInfo(ag, ""), nil
}

// DeleteAgent 는 에이전트를 삭제한다.
func (a *AgentServiceAdapter) DeleteAgent(ctx context.Context, id string) error {
	// 저장소에서 먼저 삭제 (실패 시 manager 삭제 안 함)
	if a.repo != nil {
		if err := a.repo.Delete(ctx, id); err != nil && !errors.Is(err, storage.ErrAgentNotFound) {
			return fmt.Errorf("delete agent from storage: %w", err)
		}
	}
	return a.manager.Delete(id)
}

// StartAgent 는 에이전트를 시작한다.
func (a *AgentServiceAdapter) StartAgent(ctx context.Context, id string) error {
	return a.manager.Start(ctx, id)
}

// StopAgent 는 에이전트를 정지한다.
func (a *AgentServiceAdapter) StopAgent(ctx context.Context, id string) error {
	return a.manager.Stop(ctx, id)
}

// RestartAgent 는 에이전트를 재시작한다.
func (a *AgentServiceAdapter) RestartAgent(ctx context.Context, id string) error {
	return a.manager.Restart(ctx, id)
}

// ConfigureAgent 는 에이전트 설정을 변경한다.
func (a *AgentServiceAdapter) ConfigureAgent(ctx context.Context, id string, cfg map[string]any) error {
	ag, err := a.manager.Get(id)
	if err != nil {
		return err
	}

	info := ag.Info()
	agentCfg := info.Config

	// Transport.Options 업데이트 (팩토리에서 파싱하는 설정)
	agentCfg.Transport.Options = cfg

	// Metadata 에 평탄한 문자열 값만 저장 (API 응답용)
	agentCfg.Metadata = make(map[string]string, len(cfg))
	for k, v := range cfg {
		switch v.(type) {
		case map[string]any, []any:
			// 중첩 객체/배열은 Metadata에 저장하지 않음 (Transport.Options에서 관리)
		default:
			agentCfg.Metadata[k] = fmt.Sprint(v)
		}
	}

	if err := ag.Configure(agentCfg); err != nil {
		return err
	}

	// 영속 저장소 갱신
	if a.repo != nil {
		if err := a.repo.Save(ctx, agentCfg); err != nil {
			a.logger.Warn("agent 저장소 갱신 실패 (configure)", "agentID", id, "error", err)
		}
	}

	return nil
}

// AgentStats 는 에이전트의 상세 통계를 반환한다.
func (a *AgentServiceAdapter) AgentStats(ctx context.Context, id string) (*handler.AgentStatsInfo, error) {
	ag, err := a.manager.Get(id)
	if err != nil {
		return nil, err
	}

	info := ag.Info()
	stats := ag.Stats()

	result := &handler.AgentStatsInfo{
		ID:          info.ID,
		Status:      string(info.State),
		MessagesIn:  stats.MessagesReceived,
		MessagesOut: stats.MessagesSent,
		ErrorCount:  stats.MessagesErrored,
		Connected:   info.State == lifecycle.StateRunning,
	}

	if info.Uptime > 0 {
		result.Uptime = info.Uptime.Truncate(time.Second).String()
	}

	return result, nil
}

// agentToHandlerInfo 는 agent.Agent 를 handler.AgentInfo 로 변환한다.
// detail 이 "summary" 이면 health, stats, uptime 등을 포함하고,
// "full" 이면 shared_info 와 StatefulAgent.State() 도 포함한다.
func agentToHandlerInfo(ag agent.Agent, detail string) *handler.AgentInfo {
	info := ag.Info()

	// Transport.Options 우선 사용 (중첩 구조 보존)
	// Metadata 는 flat map[string]string 이므로 중첩 맵이 fmt.Sprint() 로 평탄화됨
	var cfg map[string]any
	if len(info.Config.Transport.Options) > 0 {
		cfg = make(map[string]any, len(info.Config.Transport.Options))
		for k, v := range info.Config.Transport.Options {
			cfg[k] = v
		}
	} else {
		cfg = make(map[string]any, len(info.Config.Metadata))
		for k, v := range info.Config.Metadata {
			cfg[k] = v
		}
	}

	connected := info.State == lifecycle.StateRunning
	result := &handler.AgentInfo{
		ID:        info.ID,
		Name:      info.Name,
		Type:      info.Type,
		Status:    string(info.State),
		Config:    cfg,
		Connected: &connected,
	}

	// summary 또는 full 이면 상세 정보 추가
	if detail == "summary" || detail == "full" {
		result.Health = &handler.AgentHealthInfo{
			Status:    string(info.Health.Status),
			LastCheck: info.Health.LastCheck,
		}
		result.Stats = &handler.AgentStatsResponse{
			MessagesIn:  info.Stats.MessagesReceived,
			MessagesOut: info.Stats.MessagesSent,
			Errors:      info.Stats.MessagesErrored,
		}
		if info.Uptime > 0 {
			result.Uptime = info.Uptime.Truncate(time.Second).String()
		}
		if !info.StartedAt.IsZero() {
			t := info.StartedAt
			result.StartedAt = &t
		}
		if !info.CreatedAt.IsZero() {
			t := info.CreatedAt
			result.CreatedAt = &t
		}
	}

	// full 이면 shared_info 와 state 추가
	if detail == "full" {
		if info.SharedInfo != nil {
			result.SharedInfo = &handler.AgentSharedInfo{
				RefCount: info.SharedInfo.RefCount,
				Flows:    info.SharedInfo.Flows,
			}
		}
		// StatefulAgent 인터페이스 구현 확인
		if sa, ok := ag.(agent.StatefulAgent); ok {
			result.State = sa.State()
		}
	}

	return result
}

// ExecAgent 는 에이전트에 Process 커맨드를 전송하고 결과를 반환한다.
func (a *AgentServiceAdapter) ExecAgent(ctx context.Context, id string, data []byte) (json.RawMessage, error) {
	ag, err := a.manager.Get(id)
	if err != nil {
		return nil, err
	}

	result, err := ag.Process(data)
	if err != nil {
		return nil, fmt.Errorf("agent exec: %w", err)
	}

	return json.RawMessage(result), nil
}

// 컴파일 타임 인터페이스 검증
var _ handler.AgentManager = (*AgentServiceAdapter)(nil)
