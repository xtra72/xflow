package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/storage"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// nameResolver 는 노드 ID/플로우 ID를 이름으로 해석하는 선택적 인터페이스이다.
// engine.Engine 이 이 인터페이스를 만족한다.
type nameResolver interface {
	ResolveNodeName(flowID, nodeID string) (string, bool)
	ResolveFlowName(flowID string) (string, bool)
	ResolveFlowNameByNodeID(nodeID string) (flowID, flowName string, ok bool)
}

// AgentServiceAdapter 는 handler.AgentManager 인터페이스를 구현하여
// agent.Manager 와 연결하는 서비스 어댑터이다.
type AgentServiceAdapter struct {
	manager  agent.Manager
	repo     storage.AgentRepository // 영속 저장소 (nil 허용)
	logger   *slog.Logger
	resolver nameResolver // 노드/플로우 이름 해석 (nil 허용)
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

// SetNameResolver 는 노드/플로우 이름 해석기를 설정한다.
// Engine 을 래핑한 engineNameResolver 를 전달하면 NodeRef 통계에 이름이 포함된다.
func (a *AgentServiceAdapter) SetNameResolver(r nameResolver) {
	a.resolver = r
}

// ListAgents 는 페이지네이션을 적용하여 에이전트 목록을 반환한다.
func (a *AgentServiceAdapter) ListAgents(ctx context.Context, opts dto.ListOptions) ([]handler.AgentInfo, int64, error) {
	agents := a.manager.List()
	var result []handler.AgentInfo

	for _, ag := range agents {
		info := agentToHandlerInfo(ag, opts.Detail)
		if opts.Status == "" || info.Status == opts.Status {
			result = append(result, *info)
		}
	}

	// 정렬 적용
	sortField, ascending := parseSortParam(opts.Sort)
	sort.Slice(result, func(i, j int) bool {
		var vi, vj string
		switch sortField {
		case "type":
			vi, vj = result[i].Type, result[j].Type
		case "status":
			vi, vj = result[i].Status, result[j].Status
		default: // "name" 및 알 수 없는 필드
			vi, vj = result[i].Name, result[j].Name
		}
		if ascending {
			return strings.ToLower(vi) < strings.ToLower(vj)
		}
		return strings.ToLower(vi) > strings.ToLower(vj)
	})

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
	if req.LogLevel != nil {
		cfg.LogLevel = *req.LogLevel
		// 런타임에도 즉시 적용
		if err := a.manager.SetAgentLogLevel(id, *req.LogLevel); err != nil {
			a.logger.Warn("agent 로그 레벨 런타임 변경 실패", "agentID", id, "level", *req.LogLevel, "error", err)
		}
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

// transportKeys 는 변경 시 에이전트 재시작이 필요한 transport 설정 키 목록이다.
var transportKeys = []string{
	"transport_type", "port", "serial_port", "baud_rate", "data_bits", "stop_bits", "parity",
	"tcp_host", "tcp_port",
}

// needsRestart 는 이전 설정과 새 설정을 비교하여 transport 재시작이 필요한지 판단한다.
func needsRestart(oldOpts, newOpts map[string]any) bool {
	for _, key := range transportKeys {
		oldVal, oldOK := oldOpts[key]
		newVal, newOK := newOpts[key]
		if oldOK != newOK || fmt.Sprint(oldVal) != fmt.Sprint(newVal) {
			return true
		}
	}
	return false
}

// ConfigureAgent 는 에이전트 설정을 변경한다.
// transport 관련 설정(시리얼 포트, TCP 주소 등)이 변경되면 자동으로 에이전트를 재시작한다.
func (a *AgentServiceAdapter) ConfigureAgent(ctx context.Context, id string, cfg map[string]any) error {
	ag, err := a.manager.Get(id)
	if err != nil {
		return err
	}

	info := ag.Info()
	agentCfg := info.Config
	oldOpts := agentCfg.Transport.Options

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

	// transport 설정이 변경되면 자동 재시작 (새 transport로 재생성)
	if needsRestart(oldOpts, cfg) {
		a.logger.Info("transport 설정 변경 감지, 에이전트 재시작", "agentID", id)
		if err := a.manager.Restart(ctx, id); err != nil {
			return fmt.Errorf("configure: auto-restart failed: %w", err)
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

	connected := info.State == lifecycle.StateRunning
	if connected {
		if tc, ok := ag.(agent.TransportChecker); ok {
			connected = tc.TransportConnected()
		}
	}

	result := &handler.AgentStatsInfo{
		// 기존 flat 필드 (하위 호환성)
		ID:             info.ID,
		Status:         string(info.State),
		MessagesIn:     stats.MessagesReceived,
		MessagesOut:    stats.MessagesSent,
		ErrorCount:     stats.MessagesErrored,
		Connected:      connected,
		BufferPending:  stats.MsgBufferPending,
		BufferCapacity: stats.MsgBufferCapacity,

		// 새 중첩 구조
		Messages: &handler.EnhancedMessagesStats{
			Total: handler.MessageCounters{
				Received: stats.MessagesReceived,
				Sent:     stats.MessagesSent,
				Errored:  stats.MessagesErrored,
			},
			External: handler.MessageCounters{
				Received: stats.ExternalMessagesReceived,
				Sent:     stats.ExternalMessagesSent,
				Errored:  stats.ExternalMessagesErrored,
			},
			Internal: handler.MessageCounters{
				Received: stats.InternalMessagesReceived,
				Sent:     stats.InternalMessagesSent,
				Errored:  stats.InternalMessagesErrored,
			},
		},
		Bytes: &handler.BytesStats{
			Read:    stats.BytesRead,
			Written: stats.BytesWritten,
		},
		Buffer: &handler.BufferStatsInfo{
			Pending:  stats.MsgBufferPending,
			Capacity: stats.MsgBufferCapacity,
		},
		DroppedMessages: stats.DroppedMessages,
		RestartCount:    stats.RestartCount,
		Connections:     []handler.ConnectionStatsResponse{},
		NodeRefs:        []handler.NodeRefStatsResponse{},
	}

	if info.Uptime > 0 {
		result.Uptime = info.Uptime.Truncate(time.Second).String()
	}

	if stats.LoadTime > 0 {
		result.LoadTime = stats.LoadTime.String()
	}

	if stats.AvgProcessingLatency > 0 {
		result.AvgProcessLatency = stats.AvgProcessingLatency.String()
	}

	if !stats.LastActivityAt.IsZero() {
		result.LastActivityAt = stats.LastActivityAt.Format(time.RFC3339)
	}

	// ConnectionStatsProvider 인터페이스 확인
	if csp, ok := ag.(agent.ConnectionStatsProvider); ok {
		connStats := csp.ConnectionStats()
		result.Connections = make([]handler.ConnectionStatsResponse, len(connStats))
		for i, cs := range connStats {
			result.Connections[i] = handler.ConnectionStatsResponse{
				ID:               cs.ID,
				MessagesReceived: cs.MessagesReceived,
				MessagesSent:     cs.MessagesSent,
				MessagesErrored:  cs.MessagesErrored,
				BytesRead:        cs.BytesRead,
				BytesWritten:     cs.BytesWritten,
			}
			if !cs.ConnectedAt.IsZero() {
				result.Connections[i].ConnectedAt = cs.ConnectedAt.Format(time.RFC3339)
			}
			if !cs.LastActivityAt.IsZero() {
				result.Connections[i].LastActivityAt = cs.LastActivityAt.Format(time.RFC3339)
			}
		}
	}

	// NodeRefStats 매핑 (resolver가 있으면 이름 해석 + 미해석 노드 필터링)
	if len(stats.NodeRefs) > 0 {
		refs := make([]handler.NodeRefStatsResponse, 0, len(stats.NodeRefs))
		for _, nr := range stats.NodeRefs {
			ref := handler.NodeRefStatsResponse{
				NodeID:           nr.NodeID,
				FlowID:           nr.FlowID,
				MessagesReceived: nr.MessagesReceived,
				MessagesSent:     nr.MessagesSent,
				MessagesErrored:  nr.MessagesErrored,
			}
			if !nr.LastActivityAt.IsZero() {
				ref.LastActivityAt = nr.LastActivityAt.Format(time.RFC3339)
			}
			if a.resolver != nil {
				// 노드 이름 해석 (flowID가 빈 경우 전체 플로우 검색)
				nodeName, nodeFound := a.resolver.ResolveNodeName(nr.FlowID, nr.NodeID)
				if !nodeFound {
					// 현재 배포된 플로우에 없는 노드 → 제외
					continue
				}
				ref.NodeName = nodeName

				// 플로우 이름 해석 (flowID가 빈 경우 nodeID로 역검색)
				if nr.FlowID != "" {
					if name, ok := a.resolver.ResolveFlowName(nr.FlowID); ok {
						ref.FlowName = name
					}
				} else {
					if fid, fname, ok := a.resolver.ResolveFlowNameByNodeID(nr.NodeID); ok {
						ref.FlowID = fid
						ref.FlowName = fname
					}
				}
			}
			refs = append(refs, ref)
		}
		result.NodeRefs = refs
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
	// TransportChecker 구현 에이전트는 실제 트랜스포트 연결 상태를 반영한다.
	if connected {
		if tc, ok := ag.(agent.TransportChecker); ok {
			connected = tc.TransportConnected()
		}
	}
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
			MessagesIn:     info.Stats.MessagesReceived,
			MessagesOut:    info.Stats.MessagesSent,
			Errors:         info.Stats.MessagesErrored,
			BufferPending:  info.Stats.MsgBufferPending,
			BufferCapacity: info.Stats.MsgBufferCapacity,
		}
		if connected && info.Uptime > 0 {
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

	// StatefulAgent 인터페이스 구현 시 state 추가 (summary/full 모두)
	if sa, ok := ag.(agent.StatefulAgent); ok {
		state := sa.State()
		if detail == "summary" {
			// summary 에서는 entries 목록을 제거하여 응답 크기를 줄인다.
			delete(state, "entries")
		}
		result.State = state
	}

	// full 이면 shared_info 추가
	if detail == "full" {
		if info.SharedInfo != nil {
			result.SharedInfo = &handler.AgentSharedInfo{
				RefCount: info.SharedInfo.RefCount,
				Flows:    info.SharedInfo.Flows,
			}
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
