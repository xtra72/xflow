package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// AgentServiceAdapter 는 handler.AgentManager 인터페이스를 구현하여
// agent.Manager 와 연결하는 서비스 어댑터이다.
type AgentServiceAdapter struct {
	manager agent.Manager
	logger  *slog.Logger
}

// NewAgentServiceAdapter 는 새 AgentServiceAdapter 를 생성한다.
func NewAgentServiceAdapter(mgr agent.Manager, logger *slog.Logger) *AgentServiceAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &AgentServiceAdapter{
		manager: mgr,
		logger:  logger,
	}
}

// ListAgents 는 페이지네이션을 적용하여 에이전트 목록을 반환한다.
func (a *AgentServiceAdapter) ListAgents(ctx context.Context, opts dto.ListOptions) ([]handler.AgentInfo, int64, error) {
	agents := a.manager.List()
	var result []handler.AgentInfo

	for _, ag := range agents {
		info := agentToHandlerInfo(ag)
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
func (a *AgentServiceAdapter) GetAgent(ctx context.Context, id string) (*handler.AgentInfo, error) {
	ag, err := a.manager.Get(id)
	if err != nil {
		return nil, err
	}
	return agentToHandlerInfo(ag), nil
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

	a.logger.Info("agent created", "agentID", ag.ID(), "agentName", ag.Name())
	return agentToHandlerInfo(ag), nil
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
		cfg.Metadata = make(map[string]string, len(req.Config))
		for k, v := range req.Config {
			cfg.Metadata[k] = fmt.Sprint(v)
		}
	}

	if err := ag.Configure(cfg); err != nil {
		return nil, err
	}

	return agentToHandlerInfo(ag), nil
}

// DeleteAgent 는 에이전트를 삭제한다.
func (a *AgentServiceAdapter) DeleteAgent(ctx context.Context, id string) error {
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
	agentCfg.Metadata = make(map[string]string, len(cfg))
	for k, v := range cfg {
		agentCfg.Metadata[k] = fmt.Sprint(v)
	}

	return ag.Configure(agentCfg)
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
func agentToHandlerInfo(ag agent.Agent) *handler.AgentInfo {
	info := ag.Info()

	cfg := make(map[string]any, len(info.Config.Metadata))
	for k, v := range info.Config.Metadata {
		cfg[k] = v
	}

	return &handler.AgentInfo{
		ID:     info.ID,
		Name:   info.Name,
		Type:   info.Type,
		Status: string(info.State),
		Config: cfg,
	}
}

// 컴파일 타임 인터페이스 검증
var _ handler.AgentManager = (*AgentServiceAdapter)(nil)
