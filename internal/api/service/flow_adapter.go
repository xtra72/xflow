package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/pkg/flow"
)

// FlowServiceAdapter 는 handler.FlowManager 인터페이스를 구현하여
// engine.Engine 과 연결하는 서비스 어댑터이다.
// 미배포 플로우를 자체 저장소에 보관하고, 배포 시 엔진에 위임한다.
type FlowServiceAdapter struct {
	engine *engine.Engine
	mu     sync.RWMutex
	// flowStore 는 생성되었지만 아직 엔진에 배포되지 않은 플로우를 보관한다.
	flowStore map[string]flow.Flow
	logger    *slog.Logger
}

// NewFlowServiceAdapter 는 새 FlowServiceAdapter 를 생성한다.
func NewFlowServiceAdapter(eng *engine.Engine, logger *slog.Logger) *FlowServiceAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &FlowServiceAdapter{
		engine:    eng,
		flowStore: make(map[string]flow.Flow),
		logger:    logger,
	}
}

// CreateFlow 는 정의(Definition)를 파싱하여 Flow 를 생성하고 저장소에 보관한다.
func (a *FlowServiceAdapter) CreateFlow(ctx context.Context, req *dto.FlowCreateRequest) (*handler.FlowInfo, error) {
	// definition 을 JSON 으로 변환하여 Flow 객체 생성
	f, err := a.flowFromDefinition(req.Name, req.Description, req.Definition)
	if err != nil {
		return nil, fmt.Errorf("flow create: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.flowStore[f.ID()] = f
	a.logger.Info("flow created", "flowID", f.ID(), "flowName", f.Name())

	return flowToInfo(f), nil
}

// GetFlow 는 엔진 또는 저장소에서 플로우를 조회한다.
func (a *FlowServiceAdapter) GetFlow(ctx context.Context, id string) (*handler.FlowInfo, error) {
	// 1. 엔진에서 배포된 플로우 확인
	status, err := a.engine.GetFlowStatus(id)
	if err == nil {
		return flowStatusToInfo(status), nil
	}

	// 2. 로컬 저장소에서 미배포 플로우 확인
	a.mu.RLock()
	f, ok := a.flowStore[id]
	a.mu.RUnlock()

	if !ok {
		return nil, engine.ErrFlowNotFound
	}

	return flowToInfo(f), nil
}

// ListFlows 는 엔진의 배포된 플로우와 저장소의 미배포 플로우를 병합하여 반환한다.
func (a *FlowServiceAdapter) ListFlows(ctx context.Context, opts dto.ListOptions) ([]handler.FlowInfo, int64, error) {
	var result []handler.FlowInfo

	// 1. 엔진에서 배포된 플로우 목록
	deployedIDs := make(map[string]bool)
	for _, s := range a.engine.ListFlows() {
		info := flowStatusToInfo(s)
		if opts.Status == "" || info.Status == opts.Status {
			result = append(result, *info)
		}
		deployedIDs[s.FlowID] = true
	}

	// 2. 저장소에서 미배포 플로우 추가
	a.mu.RLock()
	for id, f := range a.flowStore {
		if deployedIDs[id] {
			continue
		}
		info := flowToInfo(f)
		if opts.Status == "" || info.Status == opts.Status {
			result = append(result, *info)
		}
	}
	a.mu.RUnlock()

	// 페이지네이션 적용
	total := int64(len(result))
	start := opts.Offset()
	if start >= len(result) {
		return []handler.FlowInfo{}, total, nil
	}
	end := start + opts.Size
	if end > len(result) {
		end = len(result)
	}

	return result[start:end], total, nil
}

// UpdateFlow 는 저장소의 미배포 플로우를 업데이트한다.
func (a *FlowServiceAdapter) UpdateFlow(ctx context.Context, id string, req *dto.FlowUpdateRequest) (*handler.FlowInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	f, ok := a.flowStore[id]
	if !ok {
		return nil, engine.ErrFlowNotFound
	}

	if req.Name != nil {
		// Flow 인터페이스에는 SetName 이 없으므로 재생성이 필요하다.
		// 여기서는 description 변경만 지원한다.
		_ = req.Name // 향후 확장 시 사용
	}
	if req.Description != nil {
		f.SetDescription(*req.Description)
	}
	if req.Definition != nil {
		// 새 정의로 Flow 재생성
		name := f.Name()
		desc := f.Description()
		if req.Name != nil {
			name = *req.Name
		}
		if req.Description != nil {
			desc = *req.Description
		}
		newF, err := a.flowFromDefinition(name, desc, req.Definition)
		if err != nil {
			return nil, fmt.Errorf("flow update: %w", err)
		}
		// 기존 ID 를 유지하기 위해 새 플로우로 교체하지 않고, 교체 시 새 ID 사용
		delete(a.flowStore, id)
		a.flowStore[newF.ID()] = newF
		return flowToInfo(newF), nil
	}

	return flowToInfo(f), nil
}

// DeleteFlow 는 엔진에서 배포 해제하거나 저장소에서 삭제한다.
func (a *FlowServiceAdapter) DeleteFlow(ctx context.Context, id string) error {
	// 1. 엔진에서 배포된 플로우인 경우 정지 후 배포 해제
	status, err := a.engine.GetFlowStatus(id)
	if err == nil {
		// 실행 중이면 정지
		if status.State == flow.FlowRunning || status.State == flow.FlowPaused {
			if stopErr := a.engine.StopFlow(ctx, id); stopErr != nil {
				return fmt.Errorf("flow delete: stop failed: %w", stopErr)
			}
		}
		// 배포 해제
		if undeployErr := a.engine.UndeployFlow(ctx, id); undeployErr != nil {
			return fmt.Errorf("flow delete: undeploy failed: %w", undeployErr)
		}
	}

	// 2. 저장소에서 삭제
	a.mu.Lock()
	delete(a.flowStore, id)
	a.mu.Unlock()

	return nil
}

// DeployFlow 는 저장소의 플로우를 엔진에 배포한다.
func (a *FlowServiceAdapter) DeployFlow(ctx context.Context, id string) error {
	a.mu.Lock()
	f, ok := a.flowStore[id]
	if !ok {
		a.mu.Unlock()
		return engine.ErrFlowNotFound
	}
	// 저장소에서 제거 (엔진이 소유권을 가짐)
	delete(a.flowStore, id)
	a.mu.Unlock()

	return a.engine.DeployFlow(ctx, f)
}

// StartFlow 는 엔진의 배포된 플로우를 시작한다.
func (a *FlowServiceAdapter) StartFlow(ctx context.Context, id string) error {
	return a.engine.StartFlow(ctx, id)
}

// StopFlow 는 엔진의 실행 중인 플로우를 정지한다.
func (a *FlowServiceAdapter) StopFlow(ctx context.Context, id string) error {
	return a.engine.StopFlow(ctx, id)
}

// RestartFlow 는 플로우를 정지한 후 다시 시작한다.
func (a *FlowServiceAdapter) RestartFlow(ctx context.Context, id string) error {
	if err := a.engine.StopFlow(ctx, id); err != nil {
		return fmt.Errorf("flow restart: stop failed: %w", err)
	}
	return a.engine.StartFlow(ctx, id)
}

// ConfigureFlow 는 엔진의 설정을 변경한다.
func (a *FlowServiceAdapter) ConfigureFlow(ctx context.Context, id string, cfg map[string]any) error {
	// 엔진에 배포된 플로우인지 확인
	if _, err := a.engine.GetFlowStatus(id); err != nil {
		return err
	}
	return a.engine.Configure(ctx, cfg)
}

// FlowStatus 는 플로우의 상세 상태를 반환한다.
func (a *FlowServiceAdapter) FlowStatus(ctx context.Context, id string) (*handler.FlowStatusInfo, error) {
	status, err := a.engine.GetFlowStatus(id)
	if err != nil {
		return nil, err
	}

	info := &handler.FlowStatusInfo{
		ID:           status.FlowID,
		Status:       string(status.State),
		MessageCount: status.MessageCount,
		ErrorCount:   status.ErrorCount,
	}

	if status.Uptime > 0 {
		info.Uptime = status.Uptime.Truncate(time.Second).String()
	}

	// NodeStats 채우기
	nodes, err := a.engine.GetFlowNodes(id)
	if err == nil {
		stats := make([]handler.NodeStatInfo, len(nodes))
		for i, n := range nodes {
			stats[i] = handler.NodeStatInfo{
				NodeID:   n.NodeID,
				NodeType: n.Type,
			}
		}
		info.NodeStats = stats
	}

	return info, nil
}

// ListFlowNodes 는 배포된 플로우의 모든 노드 인스턴스 정보를 반환한다.
func (a *FlowServiceAdapter) ListFlowNodes(_ context.Context, flowID string) ([]handler.FlowNodeInfo, error) {
	nodes, err := a.engine.GetFlowNodes(flowID)
	if err != nil {
		return nil, err
	}

	result := make([]handler.FlowNodeInfo, len(nodes))
	for i, n := range nodes {
		result[i] = engineNodeToFlowNodeInfo(n)
	}
	return result, nil
}

// GetFlowNode 는 배포된 플로우 내 특정 노드 인스턴스 정보를 반환한다.
func (a *FlowServiceAdapter) GetFlowNode(_ context.Context, flowID, nodeID string) (*handler.FlowNodeInfo, error) {
	n, err := a.engine.GetFlowNode(flowID, nodeID)
	if err != nil {
		return nil, err
	}

	info := engineNodeToFlowNodeInfo(*n)
	return &info, nil
}

// engineNodeToFlowNodeInfo 는 engine.NodeInstanceInfo를 handler.FlowNodeInfo로 변환한다.
func engineNodeToFlowNodeInfo(n engine.NodeInstanceInfo) handler.FlowNodeInfo {
	info := handler.FlowNodeInfo{
		NodeID: n.NodeID,
		Name:   n.Name,
		Type:   n.Type,
		State:  n.State,
		Config: n.Config,
	}
	for _, p := range n.Ports {
		info.Ports = append(info.Ports, handler.PortInfo{
			ID:        p.ID,
			Name:      p.Name,
			Direction: p.Direction,
			Connected: p.Connected,
		})
	}
	return info
}

// flowFromDefinition 은 정의 맵에서 Flow 를 생성한다.
func (a *FlowServiceAdapter) flowFromDefinition(name, description string, definition map[string]any) (flow.Flow, error) {
	// definition 에 name, description 을 병합
	def := make(map[string]any, len(definition))
	for k, v := range definition {
		def[k] = v
	}
	if name != "" {
		def["name"] = name
	}
	if description != "" {
		def["description"] = description
	}

	// map → JSON → Flow
	data, err := json.Marshal(def)
	if err != nil {
		return nil, fmt.Errorf("marshal definition: %w", err)
	}

	return flow.FlowFromJSON(data)
}

// flowToInfo 는 flow.Flow 를 handler.FlowInfo 로 변환한다.
func flowToInfo(f flow.Flow) *handler.FlowInfo {
	return &handler.FlowInfo{
		ID:          f.ID(),
		Name:        f.Name(),
		Description: f.Description(),
		Status:      string(f.State()),
		CreatedAt:   f.CreatedAt().Format(time.RFC3339),
		UpdatedAt:   f.UpdatedAt().Format(time.RFC3339),
		NodeCount:   len(f.Nodes()),
	}
}

// flowStatusToInfo 는 engine.FlowStatus 를 handler.FlowInfo 로 변환한다.
func flowStatusToInfo(s engine.FlowStatus) *handler.FlowInfo {
	info := &handler.FlowInfo{
		ID:        s.FlowID,
		Name:      s.FlowName,
		Status:    string(s.State),
		NodeCount: s.NodeCount,
	}
	if !s.StartedAt.IsZero() {
		info.CreatedAt = s.StartedAt.Format(time.RFC3339)
	}
	return info
}

// 컴파일 타임 인터페이스 검증
var _ handler.FlowManager = (*FlowServiceAdapter)(nil)
