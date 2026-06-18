// grouping.go 는 v1.4(M9, 그룹 K)의 노드 그룹핑 + BASIC 시스템 정보 수신/uptime 파생
// + 노드별 운영 요약(미러 파생)을 구현한다(@SPEC:SPEC-REMOTE-001 M9, REQ-K02~K05/
// K07/K08/K10, A13/A15).
//
// 그룹은 서버 운영 메타데이터이다(A13): 그룹 배정/해제는 managed_nodes.group_name 갱신
// 만 수행하고 노드로 명령을 전파하지 않는다(그룹 D 비경유 — 미러/노드 정의 무관).
//
// 시스템 정보는 register/heartbeat 페이로드로 보고되며(REQ-K07), 서버는 제공된 필드만
// 저장하고 미제공은 보존한다(REQ-K08/K09 하위 호환). uptime 은 started_at 과 서버 현재
// 시각의 차로 파생한다(저장하지 않음 — REQ-K08, OQ-K4 서버 파생).
//
// 운영 요약은 기존 미러(그룹 E) 집계에서 파생한다(REQ-K10/A15 — 신규 노드 왕복 없음,
// 오프라인 last-known 제공).
package remote

import (
	"context"
	"encoding/json"
	"time"

	"github.com/xtra/xflow/internal/storage"
)

// handleHeartbeat 는 heartbeat 페이로드의 BASIC 시스템 정보를 저장한다(v1.4 M9, REQ-K08).
//
// 생존성(touch)은 호출 측(server.go)이 이미 갱신했으므로, 본 함수는 시스템 정보 갱신만
// 담당한다. 제공된 필드만 갱신하고 미제공(빈값/0)은 기존값을 보존한다(REQ-K09 하위 호환).
// 모든 필드가 비어 있으면(구버전 노드) no-op 으로 회귀를 피한다.
func (s *Server) handleHeartbeat(ctx context.Context, connInstanceID string, payload []byte) {
	if s.repo == nil {
		return
	}
	var p HeartbeatPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		s.logger.Debug("heartbeat 페이로드 디코드 실패", "error", err, "instance_id", connInstanceID)
		return
	}
	// 출처는 연결의 인증된 instanceID 를 권위로 삼는다(노드 스푸핑 방지 — server_mirror 일관).
	// 노드 해상도(display_*)도 함께 갱신하며, 미제공(0)은 기존값을 보존한다(v1.6 M11, REQ-M01/M03).
	s.storeSystemInfo(ctx, connInstanceID, p.OS, p.Arch, p.StartedAt, p.DisplayWidth, p.DisplayHeight)
}

// SetNodeGroup 은 노드의 단일 그룹 라벨을 배정/변경한다(REQ-K02). 그룹은 서버 운영
// 메타데이터이므로 노드로 명령을 전파하지 않는다(A13). repo 미구성 시 ErrNoRepo.
func (s *Server) SetNodeGroup(ctx context.Context, instanceID, groupName string) error {
	if s.repo == nil {
		return ErrNoRepo
	}
	if err := s.repo.SetNodeGroup(ctx, instanceID, groupName); err != nil {
		return err // ErrManagedNodeNotFound 포함.
	}
	s.logger.Info("관리 노드 그룹 배정", "instance_id", instanceID, "group", groupName)
	return nil
}

// ClearNodeGroup 은 노드의 그룹을 해제하여 "전체" 버킷으로 환원한다(REQ-K02/K05).
// SetNodeGroup(빈 문자열)과 동일하다. repo 미구성 시 ErrNoRepo.
func (s *Server) ClearNodeGroup(ctx context.Context, instanceID string) error {
	return s.SetNodeGroup(ctx, instanceID, "")
}

// ListGroups 는 distinct 그룹 라벨 + 노드 수를 반환한다(REQ-K03). 응답은 항상 "전체"
// 버킷(빈 라벨)을 포함한다. repo 미구성 시 빈 목록을 반환한다(모드 안전).
func (s *Server) ListGroups(ctx context.Context) ([]storage.NodeGroupCount, error) {
	if s.repo == nil {
		return []storage.NodeGroupCount{}, nil
	}
	return s.repo.ListGroups(ctx)
}

// GroupDispatchResult 는 그룹 일괄 명령/업데이트의 노드별 결과이다(그룹 관리).
// 부분 성공을 허용하므로 노드마다 OK/Error 를 개별 보고한다.
type GroupDispatchResult struct {
	InstanceID string          `json:"instance_id"`
	OK         bool            `json:"ok"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
}

// RenameGroup 은 oldName 그룹의 모든 노드 group_name 을 newName 으로 일괄 변경한다(그룹 관리).
// 영향받은 노드 수를 반환한다(0 이면 해당 그룹 없음). repo 미구성 시 ErrNoRepo.
func (s *Server) RenameGroup(ctx context.Context, oldName, newName string) (int, error) {
	if s.repo == nil {
		return 0, ErrNoRepo
	}
	n, err := s.repo.RenameGroup(ctx, oldName, newName)
	if err != nil {
		return 0, err
	}
	s.logger.Info("관리 노드 그룹 이름변경", "from", oldName, "to", newName, "moved", n)
	return n, nil
}

// DeleteGroup 은 groupName 그룹을 삭제하여 멤버를 "전체" 버킷으로 이동한다(그룹 관리).
// 영향받은 노드 수를 반환한다(0 이면 해당 그룹 없음). repo 미구성 시 ErrNoRepo.
func (s *Server) DeleteGroup(ctx context.Context, groupName string) (int, error) {
	if s.repo == nil {
		return 0, ErrNoRepo
	}
	n, err := s.repo.DeleteGroup(ctx, groupName)
	if err != nil {
		return 0, err
	}
	s.logger.Info("관리 노드 그룹 삭제", "group", groupName, "moved", n)
	return n, nil
}

// DispatchGroup 은 groupName 그룹 내 승인 노드 전체에 명령을 디스패치하고 노드별 결과를
// 모은다(그룹 일괄 명령/업데이트). 오프라인/적용 실패 노드는 해당 결과의 Error 로 보고되어
// 부분 성공을 허용한다(미승인 노드는 그룹 대상이 아니므로 건너뛴다). repo 미구성 시 ErrNoRepo.
func (s *Server) DispatchGroup(
	ctx context.Context,
	groupName, domain, action string,
	args json.RawMessage,
) ([]GroupDispatchResult, error) {
	if s.repo == nil {
		return nil, ErrNoRepo
	}
	nodes, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]GroupDispatchResult, 0)
	for _, n := range nodes {
		if n.GroupName != groupName || n.Status != RegStatusApproved {
			continue
		}
		r := GroupDispatchResult{InstanceID: n.InstanceID}
		res, derr := s.Dispatch(ctx, n.InstanceID, domain, action, args)
		if derr != nil {
			r.Error = derr.Error()
		} else {
			r.OK = true
			r.Result = res
		}
		results = append(results, r)
	}
	return results, nil
}

// NodeSummary 는 노드별 운영 요약을 미러에서 파생해 반환한다(REQ-K10/A15). 노드 존재를
// 먼저 검증하고(미존재 → ErrManagedNodeNotFound), 미러 집계로 요약을 산출한다(오프라인
// 시에도 last-known 미러 제공 — REQ-E06). mirror 미구성 시 0 요약을 반환한다.
func (s *Server) NodeSummary(ctx context.Context, instanceID string) (storage.NodeOperationalSummary, error) {
	if err := s.ensureNodeExists(ctx, instanceID); err != nil {
		return storage.NodeOperationalSummary{}, err
	}
	if s.mirror == nil {
		return storage.NodeOperationalSummary{}, nil
	}
	return s.mirror.NodeSummary(ctx, instanceID)
}

// NodeDetail 은 노드 메타 + BASIC 시스템 정보 + uptime + 운영 요약을 결합해 반환한다
// (REQ-K08/K10). 프론트엔드 노드 대시보드(9.4)가 소비하는 단일 응답이다.
//
// uptime 은 started_at>0 일 때만 (now - started_at)으로 파생하고, started_at==0(미보고)
// 이면 -1(미표시)을 반환한다(REQ-K08 — 서버 파생, 저장 안 함). online 은 라이브 추적값을
// 우선 적용한다(REQ-B05). 미존재 노드는 ErrManagedNodeNotFound.
type NodeDetail struct {
	Node      storage.ManagedNode
	Online    bool  // 라이브 연결 상태(REQ-B05, repo 의 online 보다 우선)
	UptimeMs  int64 // now - started_at (started_at>0 일 때만, 아니면 -1 — REQ-K08)
	HasUptime bool  // started_at>0 여부(uptime 표시 가능)
	// EffectiveWidth/EffectiveHeight 는 프론트엔드(고정 캔버스 스케일러)가 사용할
	// 최종 노드 해상도이다(v1.6 M11 확장). 우선순위: 오버라이드(둘 다 양수) > 노드 보고값.
	// 둘 다 없으면 0,0 이며 프론트엔드가 기본값(1920×1080)/컨테이너 크기로 폴백한다(REQ-M03).
	// 오버라이드/보고값은 Node 필드(DisplayOverride*/Display*)로 별도 노출되어 UI 가 소스를
	// 표시할 수 있다.
	EffectiveWidth  int // effective 가로 px(오버라이드 우선, 아니면 노드 보고값, 둘 다 없으면 0)
	EffectiveHeight int // effective 세로 px(오버라이드 우선, 아니면 노드 보고값, 둘 다 없으면 0)
	Summary         storage.NodeOperationalSummary
}

// NodeDetail 은 instance_id 의 종합 상세를 반환한다(REQ-K08/K10). repo 미구성 시 ErrNoRepo.
func (s *Server) NodeDetail(ctx context.Context, instanceID string) (NodeDetail, error) {
	if s.repo == nil {
		return NodeDetail{}, ErrNoRepo
	}
	node, err := s.repo.Get(ctx, instanceID)
	if err != nil {
		return NodeDetail{}, err // ErrManagedNodeNotFound 포함.
	}

	// 라이브 online 추적값을 우선 적용한다(repo online 은 영속 시점 — 라이브가 권위).
	online := node.Online
	if st, ok := s.NodeState(instanceID); ok {
		online = st.Online
	}
	node.Online = online

	detail := NodeDetail{Node: node, Online: online}
	detail.UptimeMs, detail.HasUptime = deriveUptime(node.StartedAt, time.Now().UnixMilli())
	// effective 해상도 파생(v1.6 M11 확장): 오버라이드(둘 다 양수) 우선, 아니면 노드 보고값.
	detail.EffectiveWidth, detail.EffectiveHeight = effectiveDisplay(node)

	// 운영 요약(미러 파생 — REQ-K10). mirror 미구성 시 0 요약.
	if s.mirror != nil {
		sum, sumErr := s.mirror.NodeSummary(ctx, instanceID)
		if sumErr != nil {
			s.logger.Warn("노드 운영 요약 파생 실패", "instance_id", instanceID, "error", sumErr)
		} else {
			detail.Summary = sum
		}
	}
	return detail, nil
}

// deriveUptime 은 started_at(epoch ms)과 현재 시각(epoch ms)으로 uptime 을 파생한다
// (REQ-K08 — 서버 파생, OQ-K4). started_at<=0(미보고)이면 (-1, false)를 반환해 uptime
// 미표시를 신호한다. nowMs<startedAt(시계 역전)이면 0 으로 클램프한다.
func deriveUptime(startedAtMs, nowMs int64) (int64, bool) {
	if startedAtMs <= 0 {
		return -1, false
	}
	up := nowMs - startedAtMs
	if up < 0 {
		up = 0
	}
	return up, true
}

// effectiveDisplay 는 노드의 effective 해상도를 파생한다(v1.6 M11 확장). 관리자 오버라이드
// (DisplayOverrideWidth/Height 가 둘 다 양수)가 있으면 그 값을, 아니면 노드 보고값
// (DisplayWidth/Height)을 사용한다. 둘 다 없으면 0,0 을 반환하며 프론트엔드가 폴백한다
// (REQ-M03 — 기본값/컨테이너 크기). 부분 오버라이드(한쪽만 양수)는 무효로 간주해 보고값으로
// 폴백한다(저장소가 부분값을 0,0 으로 정규화하므로 정상 경로에서는 발생하지 않음).
func effectiveDisplay(node storage.ManagedNode) (int, int) {
	if node.DisplayOverrideWidth > 0 && node.DisplayOverrideHeight > 0 {
		return node.DisplayOverrideWidth, node.DisplayOverrideHeight
	}
	return node.DisplayWidth, node.DisplayHeight
}

// SetNodeDisplayOverride 는 관리자가 노드 해상도 오버라이드를 설정한다(v1.6 M11 확장).
// 그룹 배정과 동일하게 서버 운영 메타데이터이므로 노드로 명령을 전파하지 않는다(A13).
// width<=0 또는 height<=0 은 저장소에서 0,0 으로 해제된다. repo 미구성 시 ErrNoRepo.
func (s *Server) SetNodeDisplayOverride(ctx context.Context, instanceID string, width, height int) error {
	if s.repo == nil {
		return ErrNoRepo
	}
	if err := s.repo.SetNodeDisplayOverride(ctx, instanceID, width, height); err != nil {
		return err // ErrManagedNodeNotFound 포함.
	}
	s.logger.Info("관리 노드 해상도 오버라이드 설정", "instance_id", instanceID, "width", width, "height", height)
	return nil
}

// ClearNodeDisplayOverride 는 노드 해상도 오버라이드를 해제한다(effective=노드 보고값으로
// 폴백). SetNodeDisplayOverride(0,0)과 동일하다. repo 미구성 시 ErrNoRepo.
func (s *Server) ClearNodeDisplayOverride(ctx context.Context, instanceID string) error {
	return s.SetNodeDisplayOverride(ctx, instanceID, 0, 0)
}
