// access.go 는 "관리자가 이 노드를 들여다봤다" 를 기록한다
// (@SPEC:SPEC-REMOTE-LOG-001).
//
// # 노드가 보낸 신호와 사람이 한 행위는 다른 칸이다
//
// `last_seen` 은 노드가 살아 있다는 신호(keep-alive)이고, 여기서 다루는
// `last_access_at` 은 사람이 그 노드를 관리하려고 들어온 시각이다. 둘을 한 칸에
// 담으면 "노드가 죽었나" 와 "아무도 안 본 지 오래됐나" 중 어느 질문에도 답하지
// 못한다.
//
// # 왜 두 개의 창(window)인가
//
// 노드 화면은 5초마다 상세를 다시 조회한다. 그 요청 하나하나를 그대로 받아 적으면
// 창을 열어 둔 30분이 감사 로그 360줄이 되고, DB 쓰기도 그만큼 일어난다. 그래서
// 갱신은 두 갈래로 나뉜다.
//
//   - 시각 갱신(accessTouchInterval): 자주, 그러나 매 요청은 아니다. 화면이 읽는
//     "마지막 접속" 은 분 단위로 충분하다.
//   - 로그 1줄(accessSessionWindow): 뜸하게. 한 번 들어와 머무는 동안은 한 줄이다.
//     자리를 뜨고 한참 뒤 다시 들어오면 그때 새 줄이 생긴다 — 그것이 사람이 읽고
//     싶어 하는 "접속 이력" 의 낱알이다.
package remote

import (
	"context"
	"time"

	"github.com/xtra/xflow/internal/storage"
)

const (
	// accessTouchInterval 은 last_access_at 을 DB 에 쓰는 최소 간격이다.
	accessTouchInterval = 30 * time.Second
	// accessSessionWindow 는 같은 접속으로 묶는 시간이다. 이보다 오래 비면 새 접속이다.
	accessSessionWindow = 10 * time.Minute
)

// TouchAccess 는 관리자의 원격 관리 접근을 기록한다.
//
// actor 가 빈 문자열이면(시스템 내부 호출 등) 아무것도 하지 않는다 — 사람이 한 일이
// 아닌 것을 "마지막 접속" 으로 적으면 그 칸의 뜻이 무너진다.
func (s *Server) TouchAccess(ctx context.Context, instanceID, actor string) {
	if s.repo == nil || instanceID == "" || actor == "" {
		return
	}
	now := time.Now()

	s.mu.Lock()
	lastTouch := s.lastAccessTouch[instanceID]
	lastAudit := s.lastAccessAudit[instanceID]
	needTouch := lastTouch.IsZero() || now.Sub(lastTouch) >= accessTouchInterval
	needAudit := lastAudit.IsZero() || now.Sub(lastAudit) >= accessSessionWindow
	if needTouch {
		s.lastAccessTouch[instanceID] = now
	}
	if needAudit {
		s.lastAccessAudit[instanceID] = now
	}
	s.mu.Unlock()

	// 락 밖에서 DB 를 지난다(persistOnline 규약 일관 — 데드락 방지).
	if needTouch {
		if err := s.repo.SetLastAccess(ctx, instanceID, actor, now.UnixMilli()); err != nil {
			// 없는 노드(삭제 직후 등)는 흔한 경우이므로 Debug 로 둔다.
			s.logger.Debug("마지막 접속 기록 실패", "instance_id", instanceID, "error", err)
		}
	}
	if needAudit {
		// 주체는 호출자가 넘긴 값이다 — ctx 에는 실려 오지 않는 경로가 있다.
		s.recordAuditAs(ctx, instanceID, storage.AuditActionAccess, actor, "")
	}
}
