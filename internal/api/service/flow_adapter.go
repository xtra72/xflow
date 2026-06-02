package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/internal/storage"
	"github.com/xtra/xflow/pkg/flow"
)

// flowStateToAPIStatus 는 엔진의 FlowState를 API 상태 문자열로 변환한다.
// 엔진 상태를 그대로 전달하되, 중간 상태는 가장 가까운 안정 상태로 매핑한다.
// stored, loaded, running, stopped, error
func flowStateToAPIStatus(state flow.FlowState) string {
	switch state {
	case flow.FlowStored:
		return "stored"
	case flow.FlowLoaded, flow.FlowInitializing:
		return "loaded"
	case flow.FlowRunning, flow.FlowPaused:
		return "running"
	case flow.FlowStopping, flow.FlowStopped:
		return "stopped"
	case flow.FlowError:
		return "error"
	default:
		return "stored"
	}
}

// FlowServiceAdapter 는 handler.FlowManager 인터페이스를 구현하여
// engine.Engine 과 연결하는 서비스 어댑터이다.
// 미배포 플로우를 자체 저장소에 보관하고, 배포 시 엔진에 위임한다.
type FlowServiceAdapter struct {
	engine *engine.Engine
	repo   storage.FlowRepository
	logger *slog.Logger
}

// NewFlowServiceAdapter 는 새 FlowServiceAdapter 를 생성한다.
func NewFlowServiceAdapter(eng *engine.Engine, repo storage.FlowRepository, logger *slog.Logger) *FlowServiceAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &FlowServiceAdapter{
		engine: eng,
		repo:   repo,
		logger: logger,
	}
}

// CreateFlow 는 정의(Definition)를 파싱하여 Flow 를 생성하고 저장소에 보관한다.
// 동일 이름의 플로우가 이미 존재하면 기존 플로우를 삭제하고 새로 저장한다 (upsert by name).
func (a *FlowServiceAdapter) CreateFlow(ctx context.Context, req *dto.FlowCreateRequest) (*handler.FlowInfo, error) {
	// import 모드: 노드/와이어 ID 를 모두 재생성하여 동일 플로우 다중 import 시
	// ID 충돌을 방지한다. 일반 생성/저장 경로에는 영향이 없다(플래그 미설정 시).
	// (SPEC: flow-management requirement 2)
	definition := req.Definition
	if req.RegenerateIDs {
		definition = RegenerateDefinitionIDs(req.Definition)
		a.logger.Info("flow create: import 모드 ID 재생성", "flowName", req.Name)
	}

	// definition 을 JSON 으로 변환하여 Flow 객체 생성
	f, err := a.flowFromDefinition(req.Name, req.Description, definition)
	if err != nil {
		return nil, fmt.Errorf("flow create: %w", err)
	}

	// 동일 이름의 기존 플로우가 있으면 삭제한다 (중복 방지).
	if existing, existingID := a.findFlowByName(ctx, f.Name()); existing {
		if delErr := a.repo.Delete(ctx, existingID); delErr != nil {
			a.logger.Warn("flow create: 기존 플로우 삭제 실패", "name", f.Name(), "existingID", existingID, "error", delErr)
		} else {
			a.logger.Info("flow create: 동일 이름 기존 플로우 교체", "name", f.Name(), "oldID", existingID, "newID", f.ID())
		}
	}

	if err := a.repo.Save(ctx, f); err != nil {
		return nil, fmt.Errorf("flow create: save: %w", err)
	}
	a.logger.Info("flow created", "flowID", f.ID(), "flowName", f.Name())

	return a.flowToInfo(f), nil
}

// findFlowByName 은 저장소에서 지정된 이름의 플로우를 검색한다.
// 존재하면 (true, id) 를 반환하고, 없으면 (false, "") 를 반환한다.
func (a *FlowServiceAdapter) findFlowByName(ctx context.Context, name string) (bool, string) {
	flows, err := a.repo.List(ctx)
	if err != nil {
		return false, ""
	}
	for _, f := range flows {
		if f.Name() == name {
			return true, f.ID()
		}
	}
	return false, ""
}

// GetFlow 는 엔진 또는 저장소에서 플로우를 조회한다.
func (a *FlowServiceAdapter) GetFlow(ctx context.Context, id string) (*handler.FlowInfo, error) {
	// 1. 엔진에서 배포된 플로우 확인
	status, err := a.engine.GetFlowStatus(id)
	if err == nil {
		info := flowStatusToInfo(status)
		// 저장소에서 플로우 정의를 가져와 React Flow config 와 auto_start 메타데이터를 채운다
		if f, repoErr := a.repo.Get(ctx, id); repoErr == nil {
			info.Config = a.flowToReactFlowConfig(f)
			info.AutoStart = f.Metadata()["auto_start"] == "true"
		}
		return info, nil
	}

	// 2. 저장소에서 미배포 플로우 확인
	f, err := a.repo.Get(ctx, id)
	if err != nil {
		return nil, engine.ErrFlowNotFound
	}

	return a.flowToInfo(f), nil
}

// ListFlows 는 엔진의 배포된 플로우와 저장소의 미배포 플로우를 병합하여 반환한다.
func (a *FlowServiceAdapter) ListFlows(ctx context.Context, opts dto.ListOptions) ([]handler.FlowInfo, int64, error) {
	var result []handler.FlowInfo

	// 1. 저장소에서 플로우 목록 (메타데이터 조회를 위해 먼저 로드)
	stored, err := a.repo.List(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list stored flows: %w", err)
	}
	storedMap := make(map[string]flow.Flow, len(stored))
	for _, f := range stored {
		storedMap[f.ID()] = f
	}

	// 2. 엔진에서 배포된 플로우 목록
	deployedIDs := make(map[string]bool)
	for _, s := range a.engine.ListFlows() {
		info := flowStatusToInfo(s)
		// 저장소에서 auto_start 메타데이터 확인
		if sf, ok := storedMap[s.FlowID]; ok {
			info.AutoStart = sf.Metadata()["auto_start"] == "true"
		}
		if opts.Status == "" || info.Status == opts.Status {
			result = append(result, *info)
		}
		deployedIDs[s.FlowID] = true
	}

	// 3. 저장소에서 미배포 플로우 추가
	for _, f := range stored {
		if deployedIDs[f.ID()] {
			continue
		}
		info := a.flowToInfo(f)
		if opts.Status == "" || info.Status == opts.Status {
			result = append(result, *info)
		}
	}

	// 정렬 적용
	sortField, ascending := parseSortParam(opts.Sort)
	sort.Slice(result, func(i, j int) bool {
		var vi, vj string
		switch sortField {
		case "status":
			vi, vj = result[i].Status, result[j].Status
		case "created_at":
			vi, vj = result[i].CreatedAt, result[j].CreatedAt
		case "updated_at":
			vi, vj = result[i].UpdatedAt, result[j].UpdatedAt
		default: // "name" 및 알 수 없는 필드
			vi, vj = result[i].Name, result[j].Name
		}
		if ascending {
			return strings.ToLower(vi) < strings.ToLower(vj)
		}
		return strings.ToLower(vi) > strings.ToLower(vj)
	})

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
	f, err := a.repo.Get(ctx, id)
	if err != nil {
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
		// 기존 ID 를 definition 에 주입하여 보존한다
		req.Definition["id"] = id
		newF, err := a.flowFromDefinition(name, desc, req.Definition)
		if err != nil {
			return nil, fmt.Errorf("flow update: %w", err)
		}
		// auto_start 메타데이터 이전
		if req.AutoStart != nil {
			if *req.AutoStart {
				newF.SetMetadata("auto_start", "true")
			} else {
				newF.RemoveMetadata("auto_start")
			}
		} else if f.Metadata()["auto_start"] == "true" {
			// 기존 auto_start 유지
			newF.SetMetadata("auto_start", "true")
		}

		// 기존 플로우를 삭제 후 동일 ID 로 새 플로우 저장
		if err := a.repo.Delete(ctx, id); err != nil && !errors.Is(err, storage.ErrFlowNotFound) {
			return nil, fmt.Errorf("flow update: delete old: %w", err)
		}
		if err := a.repo.Save(ctx, newF); err != nil {
			return nil, fmt.Errorf("flow update: save: %w", err)
		}
		return a.flowToInfo(newF), nil
	}

	// auto_start 메타데이터 업데이트
	if req.AutoStart != nil {
		if *req.AutoStart {
			f.SetMetadata("auto_start", "true")
		} else {
			f.RemoveMetadata("auto_start")
		}
	}

	// description 또는 auto_start 변경 시 저장
	if req.Description != nil || req.AutoStart != nil {
		if err := a.repo.Save(ctx, f); err != nil {
			return nil, fmt.Errorf("flow update: save: %w", err)
		}
	}

	return a.flowToInfo(f), nil
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

	// 2. 저장소에서 삭제 (엔진에만 있었을 수 있으므로 ErrFlowNotFound 무시)
	_ = a.repo.Delete(ctx, id)

	return nil
}

// DeployFlow 는 저장소의 플로우를 엔진에 배포한다.
// 저장소에 플로우 정의를 유지하여 서버 재시작 시 복구할 수 있도록 한다.
// 저장소에 플로우가 없지만 엔진에 배포된 경우, 엔진의 정의를 사용하여 재배포한다.
func (a *FlowServiceAdapter) DeployFlow(ctx context.Context, id string) error {
	a.logger.Debug("deploy flow: start", "flowID", id)

	f, repoErr := a.repo.Get(ctx, id)
	fromEngine := false

	if repoErr != nil {
		a.logger.Warn("deploy flow: repo.Get failed", "flowID", id, "error", repoErr)
		// 저장소에 없으면 엔진 런타임에서 플로우 정의를 가져온다.
		// (예: 정지된 플로우를 재시작할 때, 저장소가 유실된 경우)
		engineFlow, engineErr := a.engine.GetFlow(id)
		if engineErr != nil {
			a.logger.Error("deploy flow: flow not found in repo or engine", "flowID", id)
			return engine.ErrFlowNotFound
		}
		f = engineFlow
		fromEngine = true
	}

	// 이미 배포된 플로우인 경우 재배포한다 (undeploy → deploy).
	if status, sErr := a.engine.GetFlowStatus(id); sErr == nil {
		// 실행 중이면 먼저 정지한다.
		if status.State == flow.FlowRunning || status.State == flow.FlowPaused {
			if stopErr := a.engine.StopFlow(ctx, id); stopErr != nil {
				return fmt.Errorf("flow redeploy: stop failed: %w", stopErr)
			}
		}
		if unErr := a.engine.UndeployFlow(ctx, id); unErr != nil {
			return fmt.Errorf("flow redeploy: undeploy failed: %w", unErr)
		}
	}

	// 엔진에서 가져온 플로우의 상태를 FlowStored 로 리셋하여
	// engine.DeployFlow 에서 FlowLoaded 전이가 가능하도록 한다.
	// UndeployFlow 이후에 수행해야 상태 충돌이 발생하지 않는다.
	if fromEngine {
		_ = f.SetState(flow.FlowStored)
		// 저장소에도 동기화하여 이후 재시작 시 사용할 수 있도록 한다.
		_ = a.repo.Save(ctx, f)
	}

	return a.engine.DeployFlow(ctx, f)
}

// StartFlow 는 엔진의 배포된 플로우를 시작한다.
// 항상 최신 저장소 정의를 반영하기 위해, 이미 배포된 플로우도 재배포한다.
func (a *FlowServiceAdapter) StartFlow(ctx context.Context, id string) error {
	a.logger.Debug("start flow: begin", "flowID", id)

	// 항상 (재)배포하여 최신 설정을 반영한다.
	a.logger.Debug("start flow: deploying latest definition", "flowID", id)
	if deployErr := a.DeployFlow(ctx, id); deployErr != nil {
		return fmt.Errorf("flow start: deploy failed: %w", deployErr)
	}

	startErr := a.engine.StartFlow(ctx, id)
	if startErr != nil {
		a.logger.Error("start flow: engine.StartFlow failed", "flowID", id, "error", startErr)
	}
	return startErr
}

// StopFlow 는 엔진의 실행 중인 플로우를 정지한다.
// 배포되지 않았거나 이미 정지된 상태인 경우 무시한다.
func (a *FlowServiceAdapter) StopFlow(ctx context.Context, id string) error {
	status, err := a.engine.GetFlowStatus(id)
	if errors.Is(err, engine.ErrFlowNotFound) {
		return nil // 배포되지 않은 플로우
	}
	if err != nil {
		return err
	}
	if status.State != flow.FlowRunning && status.State != flow.FlowPaused {
		return nil // 이미 정지된 상태
	}
	return a.engine.StopFlow(ctx, id)
}

// RestartFlow 는 플로우를 정지한 후 다시 시작한다.
// 정지 후에는 노드/와이어가 해제되므로, 재배포(undeploy → deploy)를 거쳐 시작한다.
// Stop 실패 시에도 undeploy/재deploy를 시도하여 설정 변경을 반영한다.
func (a *FlowServiceAdapter) RestartFlow(ctx context.Context, id string) error {
	// 실행 중이면 정지 시도 — 타임아웃 10초 (블로킹 방지)
	stopCtx, stopCancel := context.WithTimeout(ctx, 10*time.Second)
	stopDone := make(chan error, 1)
	go func() { stopDone <- a.StopFlow(stopCtx, id) }()
	select {
	case stopErr := <-stopDone:
		if stopErr != nil {
			a.logger.Warn("flow restart: stop failed, continuing", "flowID", id, "error", stopErr)
		}
	case <-stopCtx.Done():
		a.logger.Warn("flow restart: stop timed out, forcing undeploy", "flowID", id)
	}
	stopCancel()

	// 배포 해제 (엔진에서 제거) — Stop 실패/타임아웃 시에도 시도
	if _, sErr := a.engine.GetFlowStatus(id); sErr == nil {
		if unErr := a.engine.UndeployFlow(ctx, id); unErr != nil {
			a.logger.Warn("flow restart: undeploy failed", "flowID", id, "error", unErr)
		}
	}
	// 재배포 + 시작 (저장소에서 최신 정의를 다시 읽음)
	if err := a.DeployFlow(ctx, id); err != nil {
		return fmt.Errorf("flow restart: deploy failed: %w", err)
	}
	return a.engine.StartFlow(ctx, id)
}

// UndeployFlow 는 정지되거나 로드된 플로우를 배포 해제한다.
// 엔진 메모리에서 제거되어 stored 상태로 되돌아간다.
func (a *FlowServiceAdapter) UndeployFlow(ctx context.Context, id string) error {
	status, err := a.engine.GetFlowStatus(id)
	if errors.Is(err, engine.ErrFlowNotFound) {
		return nil // 이미 배포 해제된 상태
	}
	if err != nil {
		return err
	}
	if status.State == flow.FlowRunning || status.State == flow.FlowPaused {
		return fmt.Errorf("flow undeploy: flow is %s, stop it first", status.State)
	}
	return a.engine.UndeployFlow(ctx, id)
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
// 엔진에 배포되지 않은 플로우는 저장소에서 조회하여 Draft 상태로 반환한다.
func (a *FlowServiceAdapter) FlowStatus(ctx context.Context, id string) (*handler.FlowStatusInfo, error) {
	status, err := a.engine.GetFlowStatus(id)
	if errors.Is(err, engine.ErrFlowNotFound) {
		// 엔진에 배포되지 않은 플로우: 저장소에서 존재 확인 후 stored 상태 반환
		if _, repoErr := a.repo.Get(ctx, id); repoErr != nil {
			return nil, engine.ErrFlowNotFound
		}
		return &handler.FlowStatusInfo{
			ID:     id,
			Status: "stored",
		}, nil
	}
	if err != nil {
		return nil, err
	}

	info := &handler.FlowStatusInfo{
		ID:           status.FlowID,
		Status:       flowStateToAPIStatus(status.State),
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
				NodeID:    n.NodeID,
				NodeName:  n.Name,
				NodeType:  n.Type,
				Processed: n.Processed,
				Errors:    n.Errors,
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

// ReconfigureFlowNode 는 실행 중인 플로우 내 특정 노드에 부분 설정을 즉시 적용한다.
// 엔진의 ReconfigureNode 에 위임하며, 엔진 에러(ErrFlowNotFound/ErrNodeNotFound)는
// 그대로 전파되어 핸들러에서 MapDomainError 를 통해 404 로 변환된다.
func (a *FlowServiceAdapter) ReconfigureFlowNode(_ context.Context, flowID, nodeID string, config map[string]any) error {
	return a.engine.ReconfigureNode(flowID, nodeID, config)
}

// RenameAgentInFlows 는 저장소의 모든 플로우에서 oldName 에이전트 참조를 newName 으로 변경한다.
// NodeDef.AgentRef.AgentName (bridge 노드)과 Config["agent_ref"] (비-bridge 노드) 모두 업데이트한다.
func (a *FlowServiceAdapter) RenameAgentInFlows(ctx context.Context, oldName, newName string) (int, error) {
	flows, err := a.repo.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("플로우 목록 조회 실패: %w", err)
	}

	updated := 0
	for _, f := range flows {
		changed := false
		nodes := f.Nodes()
		for i, n := range nodes {
			// bridge 노드: NodeDef.AgentRef.AgentName 업데이트
			if n.AgentRef != nil && n.AgentRef.AgentName == oldName {
				nodes[i].AgentRef.AgentName = newName
				changed = true
			}
			// 비-bridge 노드: Config["agent_ref"] 문자열 업데이트
			if ref, ok := n.Config["agent_ref"].(string); ok && ref == oldName {
				nodes[i].Config["agent_ref"] = newName
				changed = true
			}
		}
		if !changed {
			continue
		}

		// 변경된 노드를 플로우에 반영: 와이어 보존을 위해 전체 재구성
		wires := f.Wires()
		for _, orig := range f.Nodes() {
			_ = f.RemoveNode(orig.ID)
		}
		for _, n := range nodes {
			_ = f.AddNode(n)
		}
		for _, w := range wires {
			_ = f.AddWire(w)
		}

		if err := a.repo.Save(ctx, f); err != nil {
			a.logger.Warn("cascade rename: 플로우 저장 실패",
				"flowID", f.ID(), "error", err)
			continue
		}
		updated++
		a.logger.Info("cascade rename: 플로우 에이전트 참조 업데이트 완료",
			"flowID", f.ID(), "flowName", f.Name(),
			"oldName", oldName, "newName", newName)
	}
	return updated, nil
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
		pi := handler.PortInfo{
			ID:         p.ID,
			Name:       p.Name,
			Direction:  p.Direction,
			Connected:  p.Connected,
			Messages:   p.Messages,
			Throughput: fmt.Sprintf("%.3f", p.Throughput),
		}
		if p.ActiveFor > 0 {
			pi.ActiveFor = p.ActiveFor.Truncate(time.Second).String()
		}
		info.Ports = append(info.Ports, pi)
	}
	if len(n.Extra) > 0 {
		info.Extra = n.Extra
	}
	return info
}

// normalizeReactFlowDefinition 은 React Flow 형식의 정의를 XFlow 호환 형식으로 변환한다.
// React Flow 노드는 data 필드에 실제 정보를 담고 있으므로, 이를 XFlow 의
// type, name, inputs, outputs, metadata 로 변환한다.
// 이미 XFlow 형식인 경우에는 변환 없이 그대로 반환한다.
// normalizeExportedAgentRefs 는 XFlow 내보내기 포맷에서 agent + direction 필드를
// agent_ref 구조체로 역정규화한다.
// 내보내기 시 agent_ref → agent: {id, name} + direction 으로 분리되는데,
// 가져올 때 이를 다시 agent_ref 로 복원해야 FlowFromJSON 에서
// NodeDef.AgentRef 로 올바르게 역직렬화된다.
func normalizeExportedAgentRefs(nodeSlice []any) {
	for _, raw := range nodeSlice {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		// 이미 agent_ref 가 있으면 건너뛴다
		if _, has := node["agent_ref"]; has {
			continue
		}
		agentMap, ok := node["agent"].(map[string]any)
		if !ok {
			continue
		}
		// agent + direction → agent_ref 구조체 생성
		ref := make(map[string]any, 3)
		if id, ok := agentMap["id"].(string); ok && id != "" {
			ref["agent_id"] = id
		}
		if name, ok := agentMap["name"].(string); ok && name != "" {
			ref["agent_name"] = name
		}
		if dir, ok := node["direction"].(string); ok && dir != "" {
			ref["direction"] = dir
		}
		if len(ref) > 0 {
			node["agent_ref"] = ref
			delete(node, "agent")
			delete(node, "direction")
		}
	}
}

func normalizeReactFlowDefinition(def map[string]any) map[string]any {
	nodesRaw, ok := def["nodes"]
	if !ok {
		return def
	}
	nodeSlice, ok := nodesRaw.([]any)
	if !ok || len(nodeSlice) == 0 {
		return def
	}

	// React Flow 형식 감지: 첫 노드에 "data" 필드가 있는지 확인
	firstNode, ok := nodeSlice[0].(map[string]any)
	if !ok {
		return def
	}
	if _, hasData := firstNode["data"]; !hasData {
		// XFlow 내보내기 형식: agent + direction → agent_ref 역정규화
		normalizeExportedAgentRefs(nodeSlice)
		// flat 형식이라도 edges 키가 React Flow 스타일로 남아 있을 수 있으므로
		// 동일한 wires 변환을 적용하여 source/target → source_node_id/target_node_id,
		// sourceHandle/targetHandle → source_port/target_port 키 매핑을 보장한다.
		convertReactFlowEdgesToWires(def)
		return def
	}

	// --- 노드 변환: React Flow → XFlow ---
	convertedNodes := make([]any, 0, len(nodeSlice))
	for _, raw := range nodeSlice {
		node, ok := raw.(map[string]any)
		if !ok {
			convertedNodes = append(convertedNodes, raw)
			continue
		}

		converted := make(map[string]any)
		// ID 유지
		if id, ok := node["id"]; ok {
			converted["id"] = id
		}

		data, _ := node["data"].(map[string]any)
		if data == nil {
			convertedNodes = append(convertedNodes, node)
			continue
		}

		// data.nodeType → type (React Flow "custom" 대신 실제 타입 사용)
		if nodeType, ok := data["nodeType"]; ok {
			converted["type"] = nodeType
		}
		// data.label → name
		if label, ok := data["label"]; ok {
			converted["name"] = label
		}

		// data.ports → inputs / outputs 분리
		if portsRaw, ok := data["ports"]; ok {
			if ports, ok := portsRaw.([]any); ok {
				var inputs, outputs, errors []any
				for _, pRaw := range ports {
					p, ok := pRaw.(map[string]any)
					if !ok {
						continue
					}
					dir, _ := p["direction"].(string)
					portName, _ := p["name"].(string)
					portEntry := map[string]any{"name": portName}
					switch dir {
					case "input":
						inputs = append(inputs, portEntry)
					case "output":
						outputs = append(outputs, portEntry)
					case "error":
						errors = append(errors, portEntry)
					}
				}
				if len(inputs) > 0 {
					converted["inputs"] = inputs
				}
				if len(outputs) > 0 {
					converted["outputs"] = outputs
				}
				if len(errors) > 0 {
					converted["errors"] = errors
				}
			}
		}

		// position, category, status → metadata
		metadata := make(map[string]any)
		if pos, ok := node["position"].(map[string]any); ok {
			if x, ok := pos["x"]; ok {
				metadata["rf_position_x"] = fmt.Sprintf("%v", x)
			}
			if y, ok := pos["y"]; ok {
				metadata["rf_position_y"] = fmt.Sprintf("%v", y)
			}
		}
		if category, ok := data["category"]; ok {
			metadata["rf_category"] = fmt.Sprintf("%v", category)
		}
		if status, ok := data["status"]; ok {
			metadata["rf_status"] = fmt.Sprintf("%v", status)
		}
		if len(metadata) > 0 {
			converted["metadata"] = metadata
		}

		// data.enabled → enabled (노드 활성화 상태)
		if enabled, ok := data["enabled"]; ok {
			converted["enabled"] = enabled
		}

		// data 의 설정 필드를 config 맵으로 추출한다
		// 내부 속성(React Flow 메타데이터)은 제외한다
		internalKeys := map[string]bool{
			"label": true, "nodeType": true, "category": true,
			"icon": true, "status": true, "ports": true,
			"config_schema": true, "config": true, "type": true,
			"enabled": true,
		}
		// agent_ref 관련 필드는 별도 처리한다 (configMap에 중복 진입 방지)
		agentRefKeys := map[string]bool{
			"agent_ref": true, "agent_id": true, "agent_name": true, "agent_type": true, "direction": true,
		}
		configMap := make(map[string]any)
		// data.config 에 기존 설정이 있으면 먼저 병합
		if cfg, ok := data["config"]; ok {
			if cfgMap, ok := cfg.(map[string]any); ok {
				for k, v := range cfgMap {
					configMap[k] = v
				}
			}
		}
		// data 최상위의 설정 필드 추출 (condition, expression 등)
		for k, v := range data {
			if !internalKeys[k] && !agentRefKeys[k] {
				configMap[k] = v
			}
		}
		// agent_ref 구조 생성
		// agent_id 또는 agent_name 중 하나라도 있으면 agent_ref를 생성한다.
		// bridge, tsdb-write, tsdb-query, lgap-status, lgap-control, lgap 등
		// 에이전트 참조가 필요한 노드 타입에 적용된다.
		nodeType, _ := data["nodeType"].(string)
		agentID, _ := data["agent_id"].(string)
		agentName, _ := data["agent_name"].(string)
		if agentID != "" || agentName != "" {
			agentRef := map[string]any{
				"agent_id":   agentID,
				"agent_name": agentName,
			}
			if nodeType == "bridge" {
				direction, _ := data["direction"].(string)
				agentRef["direction"] = direction
			}
			converted["agent_ref"] = agentRef

			// bridge 외 에이전트 참조 노드(nasa, lgcp, lgap, mqtt, modbus 등)는
			// config["agent_ref"] 문자열로 에이전트를 resolve한다.
			// 노드 레벨 agent_ref 구조체와 별도로 config에도 agent_ref를 문자열로 넣어준다.
			if nodeType != "bridge" {
				ref := agentName
				if ref == "" {
					ref = agentID
				}
				configMap["agent_ref"] = ref
			}
		}
		if len(configMap) > 0 {
			converted["config"] = configMap
		}

		convertedNodes = append(convertedNodes, converted)
	}
	def["nodes"] = convertedNodes

	// --- 엣지 변환: React Flow → XFlow (wires) ---
	convertReactFlowEdgesToWires(def)

	return def
}

// convertReactFlowEdgesToWires 는 React Flow 형식의 edges 를 XFlow 표준 wires
// 형식으로 변환한다. source/target → source_node_id/target_node_id,
// sourceHandle/targetHandle → source_port/target_port 키 매핑을 수행한다.
// data 필드 존재 여부와 무관하게 항상 적용되어 flat 형식 import 도 정상 처리된다.
// 이미 표준 wires 가 존재하거나 edges 가 없으면 아무 동작도 하지 않는 idempotent
// 함수이며, edges 와 wires 가 모두 존재하면 기존 wires 를 보존한다.
// @SPEC: flow import data-loss hotfix (wires 보존, 2026-05-13)
func convertReactFlowEdgesToWires(def map[string]any) {
	edgesRaw, hasEdges := def["edges"]
	if !hasEdges {
		// edges 가 없으면 변환할 대상이 없다 (wires 가 있든 없든 그대로 둔다).
		return
	}
	// edges 와 wires 가 동시에 존재하는 비정상 입력에서는 기존 wires 를 우선한다.
	if _, hasWires := def["wires"]; hasWires {
		// 표준 wires 가 이미 있으므로 React Flow edges 는 무시하고 제거만 수행한다.
		delete(def, "edges")
		return
	}
	edgeSlice, ok := edgesRaw.([]any)
	if !ok {
		return
	}

	convertedWires := make([]any, 0, len(edgeSlice))
	for _, raw := range edgeSlice {
		edge, ok := raw.(map[string]any)
		if !ok {
			convertedWires = append(convertedWires, raw)
			continue
		}

		converted := make(map[string]any)
		if id, ok := edge["id"]; ok {
			converted["id"] = id
		}
		if source, ok := edge["source"]; ok {
			converted["source_node_id"] = source
		}
		if target, ok := edge["target"]; ok {
			converted["target_node_id"] = target
		}
		// sourceHandle → source_port (포트 이름 그대로)
		if sh, ok := edge["sourceHandle"].(string); ok {
			converted["source_port"] = sh
		}
		// targetHandle → target_port (포트 이름 그대로)
		if th, ok := edge["targetHandle"].(string); ok {
			converted["target_port"] = th
		}

		// wire name, type 추출
		if name, ok := edge["name"].(string); ok {
			converted["name"] = name
		}
		if wt, ok := edge["wire_type"].(string); ok {
			converted["type"] = wt
		}
		if modeStr, ok := edge["mode"].(string); ok && modeStr != "" {
			converted["mode"] = modeStr
		}
		if bufSize, ok := edge["buffer_size"].(float64); ok {
			converted["buffer_size"] = int(bufSize)
		}

		convertedWires = append(convertedWires, converted)
	}
	// React Flow 는 "edges" 키를 사용하지만 XFlow 는 "wires" 를 사용한다
	def["wires"] = convertedWires
	delete(def, "edges")
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

	// React Flow 형식인 경우 XFlow 호환 형식으로 정규화
	def = normalizeReactFlowDefinition(def)

	// map → JSON → Flow
	data, err := json.Marshal(def)
	if err != nil {
		return nil, fmt.Errorf("marshal definition: %w", err)
	}

	return flow.FlowFromJSON(data)
}

// flowToReactFlowConfig 는 flow.Flow 의 노드와 와이어를 React Flow 형식의
// config 맵으로 변환한다. 프론트엔드 에디터에서 사용하는 nodes, edges 구조를 생성한다.
// 위치 정보가 없는 노드는 Wire 연결을 기반으로 자동 배치한다.
func (a *FlowServiceAdapter) flowToReactFlowConfig(f flow.Flow) map[string]any {
	nodes := f.Nodes()
	wires := f.Wires()

	// 위치 정보가 있는 노드 수를 확인하여 자동 배치 필요 여부를 판단한다
	hasPositionCount := 0
	for _, n := range nodes {
		if n.Metadata != nil {
			if _, ok := n.Metadata["rf_position_x"]; ok {
				hasPositionCount++
			}
		}
	}
	needsAutoLayout := hasPositionCount == 0 && len(nodes) > 0

	// 자동 배치가 필요한 경우 Wire 그래프 기반으로 위치를 계산한다
	var autoPositions map[string][2]float64
	if needsAutoLayout {
		autoPositions = computeAutoLayout(nodes, wires)
	}

	// NodeDef → React Flow Node
	reactNodes := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		posX := 0.0
		posY := 0.0

		if needsAutoLayout {
			if pos, ok := autoPositions[n.ID]; ok {
				posX = pos[0]
				posY = pos[1]
			}
		} else if n.Metadata != nil {
			if xStr, ok := n.Metadata["rf_position_x"]; ok {
				if v, err := strconv.ParseFloat(xStr, 64); err == nil {
					posX = v
				}
			}
			if yStr, ok := n.Metadata["rf_position_y"]; ok {
				if v, err := strconv.ParseFloat(yStr, 64); err == nil {
					posY = v
				}
			}
		}

		// 카테고리 및 상태 추출
		category := ""
		status := "draft"
		if n.Metadata != nil {
			if c, ok := n.Metadata["rf_category"]; ok {
				category = c
			}
			if s, ok := n.Metadata["rf_status"]; ok {
				status = s
			}
		}
		// rf_category 메타데이터가 없으면 노드 레지스트리에서 카테고리를 조회한다
		if category == "" {
			if reg := a.engine.NodeRegistry(); reg != nil {
				if meta, ok := reg.TypeMeta(n.Type); ok {
					category = meta.Category
				}
			}
		}

		// 포트 목록 생성
		var ports []map[string]any
		for _, p := range n.Inputs {
			ports = append(ports, map[string]any{
				"name":      p.Name,
				"direction": "input",
			})
		}
		for _, p := range n.Outputs {
			ports = append(ports, map[string]any{
				"name":      p.Name,
				"direction": "output",
			})
		}
		for _, p := range n.Errors {
			ports = append(ports, map[string]any{
				"name":      p.Name,
				"direction": "error",
			})
		}

		nodeData := map[string]any{
			"label":    n.Name,
			"nodeType": n.Type,
			"category": category,
			"status":   status,
			"ports":    ports,
			"enabled":  n.IsEnabled(),
		}
		// 노드별 설정값(condition, expression 등)을 data에 병합한다
		for k, v := range n.Config {
			nodeData[k] = v
		}
		// AgentRef 가 있으면 프론트엔드가 기대하는 flat 구조로 병합한다.
		// UI 의 agent_select 필드 스키마 (DynamicForm.handleFieldChange) 는
		// fieldName=agent_ref 자체에 agent_id 문자열을 저장하므로, import 직후
		// PropertyPanel 의 필수 필드 검증을 통과시키려면 agent_ref 도 함께 채워야
		// 한다 (SPEC: flow import UI agent_ref 누락 hotfix, 2026-05-13).
		if n.AgentRef != nil {
			nodeData["agent_id"] = n.AgentRef.AgentID
			nodeData["agent_name"] = n.AgentRef.AgentName
			nodeData["direction"] = string(n.AgentRef.Direction)
			nodeData["agent_type"] = ""
			nodeData["agent_ref"] = n.AgentRef.AgentID
		}

		reactNode := map[string]any{
			"id":   n.ID,
			"type": "custom",
			"position": map[string]any{
				"x": posX,
				"y": posY,
			},
			"data": nodeData,
		}
		reactNodes = append(reactNodes, reactNode)
	}

	// Wire → React Flow Edge (핸들 ID = 포트 이름 그대로)
	reactEdges := make([]map[string]any, 0, len(wires))
	for _, w := range wires {
		reactEdge := map[string]any{
			"id":           w.ID,
			"type":         "custom",
			"source":       w.SourceNodeID,
			"target":       w.TargetNodeID,
			"sourceHandle": w.SourcePort,
			"targetHandle": w.TargetPort,
			"name":         w.Name,
			"wire_type":    string(w.Type),
			"mode":         string(w.Mode),
			"buffer_size":  w.BufferSize,
		}
		reactEdges = append(reactEdges, reactEdge)
	}

	return map[string]any{
		"nodes": reactNodes,
		"edges": reactEdges,
	}
}

// computeAutoLayout 은 Wire 연결 그래프를 기반으로 노드의 위치를 자동 계산한다.
// 위상 정렬을 사용하여 좌→우 방향으로 레이어를 배정하고,
// 각 레이어 내에서 수직으로 배치하여 노드가 겹치지 않도록 한다.
func computeAutoLayout(nodes []flow.NodeDef, wires []flow.Wire) map[string][2]float64 {
	const (
		horizontalGap = 280.0 // 레이어 간 수평 간격
		verticalGap   = 120.0 // 레이어 내 수직 간격
		marginX       = 80.0  // 좌측 여백
		marginY       = 60.0  // 상단 여백
	)

	positions := make(map[string][2]float64, len(nodes))

	if len(nodes) == 0 {
		return positions
	}

	// 노드 ID 집합 생성
	nodeSet := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		nodeSet[n.ID] = true
	}

	// 방향 그래프 구성: 인접 리스트 및 진입 차수
	outgoing := make(map[string][]string) // nodeID → 타겟 노드 목록
	inDegree := make(map[string]int)      // nodeID → 진입 와이어 수
	for _, n := range nodes {
		outgoing[n.ID] = nil
		inDegree[n.ID] = 0
	}
	for _, w := range wires {
		if !nodeSet[w.SourceNodeID] || !nodeSet[w.TargetNodeID] {
			continue
		}
		outgoing[w.SourceNodeID] = append(outgoing[w.SourceNodeID], w.TargetNodeID)
		inDegree[w.TargetNodeID]++
	}

	// 위상 정렬 + 최장 경로 기반 레이어 할당
	// 각 노드의 레이어는 소스로부터의 최장 경로 길이로 결정한다
	layer := make(map[string]int, len(nodes))
	queue := make([]string, 0)

	// 진입 차수 0인 노드(소스 노드)를 큐에 추가
	for _, n := range nodes {
		if inDegree[n.ID] == 0 {
			queue = append(queue, n.ID)
			layer[n.ID] = 0
		}
	}

	// BFS로 레이어 할당: 타겟 노드의 레이어 = max(현재, 소스+1)
	visited := make(map[string]bool, len(nodes))
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current] {
			continue
		}
		visited[current] = true

		for _, target := range outgoing[current] {
			newLayer := layer[current] + 1
			if newLayer > layer[target] {
				layer[target] = newLayer
			}
			inDegree[target]--
			if inDegree[target] <= 0 {
				queue = append(queue, target)
			}
		}
	}

	// 방문하지 못한 노드(순환 참조 등) 처리
	for _, n := range nodes {
		if !visited[n.ID] {
			layer[n.ID] = 0
		}
	}

	// 레이어별 노드 그룹화 (원본 순서 유지)
	maxLayer := 0
	for _, l := range layer {
		if l > maxLayer {
			maxLayer = l
		}
	}
	layerGroups := make([][]string, maxLayer+1)
	for i := range layerGroups {
		layerGroups[i] = make([]string, 0)
	}
	for _, n := range nodes {
		l := layer[n.ID]
		layerGroups[l] = append(layerGroups[l], n.ID)
	}

	// 위치 할당: 레이어 내 노드들을 수직 중앙 정렬
	for l, group := range layerGroups {
		for i, nodeID := range group {
			x := marginX + float64(l)*horizontalGap
			y := marginY + float64(i)*verticalGap
			positions[nodeID] = [2]float64{x, y}
		}
	}

	return positions
}

// flowToInfo 는 flow.Flow 를 handler.FlowInfo 로 변환한다.
func (a *FlowServiceAdapter) flowToInfo(f flow.Flow) *handler.FlowInfo {
	return &handler.FlowInfo{
		ID:          f.ID(),
		Name:        f.Name(),
		Description: f.Description(),
		Status:      flowStateToAPIStatus(f.State()),
		CreatedAt:   f.CreatedAt().Format(time.RFC3339),
		UpdatedAt:   f.UpdatedAt().Format(time.RFC3339),
		NodeCount:   len(f.Nodes()),
		Config:      a.flowToReactFlowConfig(f),
		AutoStart:   f.Metadata()["auto_start"] == "true",
	}
}

// flowStatusToInfo 는 engine.FlowStatus 를 handler.FlowInfo 로 변환한다.
// 배포된 플로우의 경우 엔진에서 노드/와이어 정의를 직접 제공하지 않으므로
// 빈 config 를 반환한다. 전체 config 가 필요한 경우 저장소에서 조회해야 한다.
func flowStatusToInfo(s engine.FlowStatus) *handler.FlowInfo {
	info := &handler.FlowInfo{
		ID:        s.FlowID,
		Name:      s.FlowName,
		Status:    flowStateToAPIStatus(s.State),
		NodeCount: s.NodeCount,
		Config: map[string]any{
			"nodes": []map[string]any{},
			"edges": []map[string]any{},
		},
	}
	if !s.StartedAt.IsZero() {
		info.CreatedAt = s.StartedAt.Format(time.RFC3339)
	}
	if s.Uptime > 0 {
		info.Uptime = s.Uptime.Truncate(time.Second).String()
	}
	return info
}

// 컴파일 타임 인터페이스 검증
var _ handler.FlowManager = (*FlowServiceAdapter)(nil)
