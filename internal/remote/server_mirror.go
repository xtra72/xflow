// server_mirror.go 는 M4 서버 측 인벤토리 미러 수신/캐시/목록을 구현한다
// (@SPEC:SPEC-REMOTE-001 M4, spec §5.5 server, REQ-E03/E04/E05/E06/E08, A4).
//
// 캐시 권위(A4/REQ-E08): 미러 캐시는 오직 노드가 push 한 snapshot/delta 로만
// 변경된다. 서버 admin 편집은 본 경로를 호출하지 않고 명령(Dispatch, 그룹 D)으로
// 전파되며, 노드가 적용 후 delta 를 push 해야 캐시가 갱신된다. 따라서 본 파일의
// 변경 메서드는 read-loop(노드 push) 에서만 호출된다.
//
// 보안: 미러 적용 시 출처는 연결의 인증된 instanceID 를 사용한다(페이로드가 주장하는
// instance_id 를 신뢰하지 않음 — 노드 스푸핑 방지). 페이로드 instance_id 가 연결
// instanceID 와 다르면 경고 후 연결 instanceID 로 강제한다.
package remote

import (
	"context"
	"encoding/json"

	"github.com/xtra/xflow/internal/storage"
)

// MirroredResourceView 는 목록 API 응답을 위한 미러 행 + 노드 라이브 상태 결합이다
// (REQ-E04/E05/E06). Online 은 출처 노드의 현재 연결 상태이며, Online=false 는
// last-known(오프라인) 표식이다(REQ-E06).
type MirroredResourceView struct {
	storage.MirroredResource
	Online bool
}

// handleInventorySnapshot 은 inventory_snapshot 을 수신해 노드별 미러를 종류별로
// 교체한다(REQ-E01/E03). mirror 미구성 시 무시한다.
func (s *Server) handleInventorySnapshot(ctx context.Context, connInstanceID string, payload []byte) {
	if s.mirror == nil {
		return
	}
	var p InventorySnapshotPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		s.logger.Debug("inventory_snapshot 디코드 실패", "error", err, "instance_id", connInstanceID)
		return
	}
	id := s.resolveSourceID(connInstanceID, p.InstanceID)

	if err := s.mirror.ReplaceFlows(ctx, id, toMirrorRows(id, p.Flows)); err != nil {
		s.logger.Error("flow 미러 교체 실패", "instance_id", id, "error", err)
	}
	if err := s.mirror.ReplaceAgents(ctx, id, toMirrorRows(id, p.Agents)); err != nil {
		s.logger.Error("agent 미러 교체 실패", "instance_id", id, "error", err)
	}
	if err := s.mirror.ReplaceDevices(ctx, id, toMirrorRows(id, p.Devices)); err != nil {
		s.logger.Error("device 미러 교체 실패", "instance_id", id, "error", err)
	}
	s.logger.Info("인벤토리 스냅샷 수신",
		"instance_id", id, "flows", len(p.Flows), "agents", len(p.Agents), "devices", len(p.Devices))
}

// handleInventoryDelta 는 inventory_delta 를 수신해 미러에 op 를 적용한다(REQ-E02/E03).
// mirror 미구성 시 무시한다. 중복 remove/미존재 행은 멱등 처리된다(저장소 보장).
func (s *Server) handleInventoryDelta(ctx context.Context, connInstanceID string, payload []byte) {
	if s.mirror == nil {
		return
	}
	var p InventoryDeltaPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		s.logger.Debug("inventory_delta 디코드 실패", "error", err, "instance_id", connInstanceID)
		return
	}
	id := s.resolveSourceID(connInstanceID, p.InstanceID)

	switch p.Op {
	case OpAdd, OpUpdate:
		row := toMirrorRow(id, p.Item)
		if err := s.mirror.UpsertResource(ctx, p.Kind, row); err != nil {
			s.logger.Error("미러 델타 upsert 실패",
				"instance_id", id, "kind", p.Kind, "op", p.Op, "error", err)
			return
		}
	case OpRemove:
		if err := s.mirror.DeleteResource(ctx, id, p.Kind, p.Item.ID); err != nil {
			s.logger.Error("미러 델타 remove 실패",
				"instance_id", id, "kind", p.Kind, "error", err)
			return
		}
	default:
		s.logger.Warn("알 수 없는 inventory_delta op", "op", p.Op, "instance_id", id)
		return
	}
	s.logger.Debug("인벤토리 델타 적용",
		"instance_id", id, "op", p.Op, "kind", p.Kind, "item_id", p.Item.ID)
}

// resolveSourceID 는 미러 출처 노드 ID 를 결정한다. 연결의 인증 instanceID 를 권위로
// 삼아 노드 스푸핑을 방지한다(페이로드 instance_id 가 다르면 경고).
func (s *Server) resolveSourceID(connInstanceID, payloadInstanceID string) string {
	if payloadInstanceID != "" && payloadInstanceID != connInstanceID {
		s.logger.Warn("인벤토리 페이로드 instance_id 불일치 — 연결 instance_id 로 강제",
			"conn", connInstanceID, "payload", payloadInstanceID)
	}
	return connInstanceID
}

// toMirrorRows 는 InventoryItem 슬라이스를 storage.MirroredResource 로 변환한다.
func toMirrorRows(instanceID string, items []InventoryItem) []storage.MirroredResource {
	out := make([]storage.MirroredResource, 0, len(items))
	for _, it := range items {
		out = append(out, toMirrorRow(instanceID, it))
	}
	return out
}

// toMirrorRow 는 단일 InventoryItem 을 storage.MirroredResource 로 변환한다.
// Definition(이미 redacted — F06)은 문자열로 저장한다.
func toMirrorRow(instanceID string, it InventoryItem) storage.MirroredResource {
	return storage.MirroredResource{
		ID:               it.ID,
		SourceInstanceID: instanceID,
		Name:             it.Name,
		Kind:             it.Kind,
		Status:           it.Status,
		Definition:       string(it.Definition),
		UpdatedAt:        it.UpdatedAt,
	}
}

// nodeOnline 은 instance_id 의 현재 라이브 online 여부를 반환한다(목록 태깅용).
func (s *Server) nodeOnline(instanceID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.nodes[instanceID]
	return ok && st.Online
}

// withOnline 은 미러 행에 출처 노드의 라이브 online 상태를 결합한다(REQ-E06).
func (s *Server) withOnline(rows []storage.MirroredResource) []MirroredResourceView {
	out := make([]MirroredResourceView, 0, len(rows))
	for _, r := range rows {
		out = append(out, MirroredResourceView{MirroredResource: r, Online: s.nodeOnline(r.SourceInstanceID)})
	}
	return out
}

// ListMirroredFlows 는 한 노드의 flow 미러를 online 태그와 함께 반환한다(REQ-E05/E06).
func (s *Server) ListMirroredFlows(ctx context.Context, instanceID string) ([]MirroredResourceView, error) {
	return s.listMirroredByNode(ctx, instanceID, s.mirrorListFlows)
}

// ListMirroredAgents 는 한 노드의 agent 미러를 반환한다(REQ-E05/E06).
func (s *Server) ListMirroredAgents(ctx context.Context, instanceID string) ([]MirroredResourceView, error) {
	return s.listMirroredByNode(ctx, instanceID, s.mirrorListAgents)
}

// ListMirroredDevices 는 한 노드의 device 미러를 반환한다(REQ-E05/E06).
func (s *Server) ListMirroredDevices(ctx context.Context, instanceID string) ([]MirroredResourceView, error) {
	return s.listMirroredByNode(ctx, instanceID, s.mirrorListDevices)
}

// ListAllMirroredFlows 는 전 노드의 flow 미러를 출처/online 태그와 함께 반환한다
// (통합 목록 — REQ-E05/E06).
func (s *Server) ListAllMirroredFlows(ctx context.Context) ([]MirroredResourceView, error) {
	if s.mirror == nil {
		return []MirroredResourceView{}, nil
	}
	rows, err := s.mirror.ListAllFlows(ctx)
	if err != nil {
		return nil, err
	}
	return s.withOnline(rows), nil
}

// ListAllMirroredAgents 는 전 노드의 agent 미러를 반환한다(REQ-E05/E06).
func (s *Server) ListAllMirroredAgents(ctx context.Context) ([]MirroredResourceView, error) {
	if s.mirror == nil {
		return []MirroredResourceView{}, nil
	}
	rows, err := s.mirror.ListAllAgents(ctx)
	if err != nil {
		return nil, err
	}
	return s.withOnline(rows), nil
}

// ListAllMirroredDevices 는 전 노드의 device 미러를 반환한다(REQ-E05/E06).
func (s *Server) ListAllMirroredDevices(ctx context.Context) ([]MirroredResourceView, error) {
	if s.mirror == nil {
		return []MirroredResourceView{}, nil
	}
	rows, err := s.mirror.ListAllDevices(ctx)
	if err != nil {
		return nil, err
	}
	return s.withOnline(rows), nil
}

// mirrorListFn 은 노드별 미러 조회 함수 타입이다.
type mirrorListFn func(ctx context.Context, instanceID string) ([]storage.MirroredResource, error)

func (s *Server) mirrorListFlows(ctx context.Context, id string) ([]storage.MirroredResource, error) {
	return s.mirror.ListFlows(ctx, id)
}
func (s *Server) mirrorListAgents(ctx context.Context, id string) ([]storage.MirroredResource, error) {
	return s.mirror.ListAgents(ctx, id)
}
func (s *Server) mirrorListDevices(ctx context.Context, id string) ([]storage.MirroredResource, error) {
	return s.mirror.ListDevices(ctx, id)
}

// listMirroredByNode 는 노드 존재 검증 후 kind 미러를 online 태그와 함께 반환한다.
// 노드가 repo 에 없으면 storage.ErrManagedNodeNotFound 를 반환한다(핸들러 404 매핑).
func (s *Server) listMirroredByNode(ctx context.Context, instanceID string, list mirrorListFn) ([]MirroredResourceView, error) {
	if s.mirror == nil {
		return []MirroredResourceView{}, nil
	}
	if err := s.ensureNodeExists(ctx, instanceID); err != nil {
		return nil, err
	}
	rows, err := list(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	return s.withOnline(rows), nil
}

// ensureNodeExists 는 instance_id 가 알려진 노드인지 확인한다(repo 우선, 없으면
// in-memory). 미존재 시 storage.ErrManagedNodeNotFound 를 반환한다(REQ-E05 — 알 수
// 없는 노드는 404).
func (s *Server) ensureNodeExists(ctx context.Context, instanceID string) error {
	if s.repo != nil {
		if _, err := s.repo.Get(ctx, instanceID); err != nil {
			return err // ErrManagedNodeNotFound 포함.
		}
		return nil
	}
	if _, ok := s.NodeState(instanceID); !ok {
		return storage.ErrManagedNodeNotFound
	}
	return nil
}

// DeleteNode 는 관리 노드를 삭제하고 그 노드의 미러 캐시를 함께 정리한다(orphan
// 방지). repo 와 mirror 가 함께 정리되도록 서버를 단일 진입점으로 둔다.
func (s *Server) DeleteNode(ctx context.Context, instanceID string) error {
	if s.repo == nil {
		return ErrNoRepo
	}
	if err := s.repo.Delete(ctx, instanceID); err != nil {
		return err
	}
	if s.mirror != nil {
		if err := s.mirror.DeleteByNode(ctx, instanceID); err != nil {
			s.logger.Error("노드 삭제 시 미러 정리 실패", "instance_id", instanceID, "error", err)
		}
	}
	s.mu.Lock()
	delete(s.nodes, instanceID)
	s.mu.Unlock()
	s.logger.Info("관리 노드 삭제 + 미러 정리", "instance_id", instanceID)
	return nil
}

var _ mirrorListFn = (*Server)(nil).mirrorListFlows
