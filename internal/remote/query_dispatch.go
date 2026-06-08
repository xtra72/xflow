// query_dispatch.go 는 M8(그룹 J) 서버 측 READ/QUERY 프록시 디스패처를 구현한다
// (@SPEC:SPEC-REMOTE-001 M8, spec §5.5 server / §5.10, REQ-J01/J02/J05/J07/J16).
//
// 디스패치 경로(dispatch.go 의 명령 패턴을 READ-ONLY 로 미러링):
//
//  1. 호출자(핸들러)가 게이팅을 선평가한다(승인+온라인 IsManaged → 503, 노출 범위
//     IsResourceExposed → 404 — REQ-J05). DispatchQuery 자신도 IsManaged 를 최종
//     방어선으로 재확인한다(미관리 노드로의 누출 방지).
//  2. 라이브 action(IsStreamableAction)이 아니면 TTL 캐시를 조회한다(REQ-J16). 적중
//     시 노드 왕복 없이 캐시된 redacted 본문을 반환한다(게이팅은 이미 호출자가 평가).
//  3. 캐시 미스/라이브면 고유 QueryID 를 부여하고 pending 맵에 등록한 뒤 노드 라이브
//     세션으로 query 를 전송한다(REQ-J01).
//  4. 제한 시간 내 매칭 query_result 를 대기한다(REQ-J07). 타임아웃 → ErrQueryTimeout
//     (504), 노드 ok:false → ErrQueryFailed(502).
//  5. 성공 시(비라이브) redacted 본문을 캐시에 저장한다(REQ-J16).
//
// 동시 다수 질의는 QueryID 로 구분되어 각자의 결과 채널로 라우팅된다(routeQueryResult).
//
// READ-ONLY: 본 경로는 변경을 수행하지 않는다. 일반 READ 는 감사하지 않으며(REQ-J05),
// 노출 범위 위반/오류 접근만 핸들러가 감사한다(remote_query.go).
package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// DefaultQueryTimeout 은 디스패치된 READ 질의의 결과 대기 기본 제한 시간이다(REQ-J07).
// 명령보다 짧게 잡아 디테일 패널 응답성을 확보한다.
const DefaultQueryTimeout = 15 * time.Second

// ErrQueryTimeout 은 제한 시간 내 query_result 가 도착하지 않을 때 반환된다(REQ-J07 → 504).
var ErrQueryTimeout = errors.New("remote: query timed out")

// ErrQueryFailed 은 노드가 질의 실패(query_result.ok=false)를 보고했을 때 반환된다
// (REQ-J07 → 502). 메시지에 노드 오류 사유가 포함된다.
var ErrQueryFailed = errors.New("remote: query failed on node")

// DispatchQuery 는 승인+온라인 노드에 READ 질의를 디스패치하고 결과를 기다린다
// (REQ-J01/J02/J07). 라이브 action 이 아니면 TTL 캐시를 우선 사용한다(REQ-J16).
//
// 반환: 성공 시 노드가 redaction 한 데이터(json.RawMessage), 실패 시 오류.
// 오류 종류:
//   - ErrNodeNotManaged: 미승인/오프라인 노드(REQ-J05 → 503).
//   - ErrNoConn: 라이브 연결 부재(경합 상황 → 503).
//   - ErrQueryTimeout: 제한 시간 초과(REQ-J07 → 504).
//   - ErrQueryFailed: 노드 질의 실패(REQ-J07 → 502).
//   - ctx.Err(): 호출자 컨텍스트 취소.
func (s *Server) DispatchQuery(ctx context.Context, instanceID, domain, queryAction string, args json.RawMessage) (json.RawMessage, error) {
	// 1) 최종 게이트(REQ-J05 — 호출자 선평가 + 최종 방어선). 미관리 노드 누출 방지.
	if !s.IsManaged(instanceID) {
		return nil, ErrNodeNotManaged
	}

	// 2) 캐시 조회(라이브 action 우회 — REQ-J16). 라이브: device.state/agent.stats/agent.series.
	live := IsStreamableAction(domain, queryAction)
	cacheKey := ""
	if !live && s.queryCache != nil {
		cacheKey = queryCacheKey(instanceID, domain, queryAction, args)
		if data, ok := s.queryCache.get(cacheKey); ok {
			return data, nil
		}
	}

	// 3) 라이브 연결 확보.
	nc, ok := s.connFor(instanceID)
	if !ok {
		return nil, ErrNoConn
	}

	// 4) 고유 QueryID + 결과 채널 등록.
	queryID := uuid.NewString()
	resultCh := s.registerPendingQuery(queryID)
	defer s.clearPendingQuery(queryID)

	q := QueryPayload{
		QueryID:          queryID,
		TargetInstanceID: instanceID,
		Domain:           domain,
		QueryAction:      queryAction,
		Args:             args,
	}
	msg, err := NewQueryMessage(q)
	if err != nil {
		return nil, fmt.Errorf("remote: encode query: %w", err)
	}
	if err := writeEnvelope(nc.conn, msg); err != nil {
		return nil, fmt.Errorf("remote: send query: %w", err)
	}

	// 5) 결과 대기(타임아웃/취소 — REQ-J07).
	select {
	case res := <-resultCh:
		if !res.OK {
			return nil, fmt.Errorf("%w: %s", ErrQueryFailed, res.Error)
		}
		// 성공(비라이브) — redacted 본문을 캐시에 저장한다(REQ-J16).
		if !live && s.queryCache != nil && cacheKey != "" {
			s.queryCache.putForNode(instanceID, cacheKey, res.Data)
		}
		return res.Data, nil

	case <-time.After(s.queryResultTimeout()):
		return nil, ErrQueryTimeout

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// queryResultTimeout 은 설정된 질의 제한 시간(없으면 기본값)을 반환한다.
func (s *Server) queryResultTimeout() time.Duration {
	if s.queryTimeout > 0 {
		return s.queryTimeout
	}
	return DefaultQueryTimeout
}

// invalidateQueryCacheNode 는 한 노드의 READ 캐시를 무효화한다(변경 성공 후 — REQ-J16).
// 그룹 D/I 명령(Dispatch) 성공 시 호출되어 stale 응답을 방지한다.
func (s *Server) invalidateQueryCacheNode(instanceID string) {
	if s.queryCache != nil {
		s.queryCache.invalidateNode(instanceID)
	}
}

// registerPendingQuery 는 QueryID 에 대한 결과 채널을 등록하고 반환한다(버퍼 1 —
// 라우팅이 타임아웃과 경합해도 송신이 블로킹되지 않게 한다).
func (s *Server) registerPendingQuery(queryID string) chan QueryResultPayload {
	ch := make(chan QueryResultPayload, 1)
	s.pendingQMu.Lock()
	s.pendingQuery[queryID] = ch
	s.pendingQMu.Unlock()
	return ch
}

// clearPendingQuery 는 QueryID 의 pending 항목을 제거한다(멱등).
func (s *Server) clearPendingQuery(queryID string) {
	s.pendingQMu.Lock()
	delete(s.pendingQuery, queryID)
	s.pendingQMu.Unlock()
}

// routeQueryResult 는 수신한 query_result 를 대기 중인 결과 채널로 라우팅한다(REQ-J02).
// 매칭되는 pending 항목이 없으면(타임아웃 후 늦게 도착 등) 조용히 폐기한다.
func (s *Server) routeQueryResult(payload []byte) {
	var res QueryResultPayload
	if err := json.Unmarshal(payload, &res); err != nil {
		s.logger.Debug("query_result 디코드 실패", "error", err)
		return
	}
	if res.QueryID == "" {
		s.logger.Debug("query_result 에 query_id 누락 — 폐기")
		return
	}
	s.pendingQMu.Lock()
	ch, ok := s.pendingQuery[res.QueryID]
	if ok {
		delete(s.pendingQuery, res.QueryID)
	}
	s.pendingQMu.Unlock()
	if !ok {
		s.logger.Debug("매칭되는 pending 질의 없음 — query_result 폐기", "query_id", res.QueryID)
		return
	}
	ch <- res // 버퍼 1 — 비블로킹.
}
