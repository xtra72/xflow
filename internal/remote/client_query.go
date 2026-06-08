// client_query.go 는 M8(그룹 J) 노드 측 query 수신→로컬 read 매핑→redaction→
// query_result 반환을 구현한다(@SPEC:SPEC-REMOTE-001 M8, spec §5.7 준용/§5.10,
// REQ-J01/J02/J03/J05/J06).
//
// 처리 경로(handleCommand 와 대칭, READ-ONLY):
//
//  1. 승인 게이팅(REQ-J05): 미승인(토큰 미보유) 노드는 거부한다(handleCommand 와 동일
//     가드). 서버 측에서 노출 범위 게이팅을 1차 강제하나, 노드도 cheaply-checkable
//     한 allowlist 를 재확인한다.
//  2. READ-ONLY allowlist(REQ-J03/J04): IsAllowedQueryAction 으로 미열거·변경 의미
//     action 을 거부한다(소스 미호출).
//  3. QuerySource 디스패치(A10): domain/queryAction 을 노드의 로컬 read 핸들러로 매핑한다.
//  4. redaction(REQ-J06): 결과를 전송 전 QueryRedactor 로 마스킹한다.
//  5. query_result(ok+data | error) 반환(REQ-J02/J07).
//
// 적용은 별도 고루틴(handleServerMessage)에서 호출되어 읽기 루프를 막지 않으며, 신뢰
// 경계 panic 복구 가드로 데몬을 보호한다(REQ-J06 — 시크릿 비노출).
package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
)

// handleQuery 는 수신한 query 를 처리하고 query_result 를 같은 연결로 반환한다(REQ-J01).
func (c *Client) handleQuery(ctx context.Context, conn Conn, payload []byte) {
	// 신뢰 경계 panic 복구 가드: QuerySource.Query 또는 디코드/redaction 과정의 panic 이
	// 고루틴/데몬을 죽이지 않도록 복구하고, query_result{ok:false}로 변환한다. panic 값/
	// 스택은 시크릿을 담을 수 있으므로 외부에는 일반화 메시지만, 상세는 내부 로그만(REQ-J06).
	queryID := ""
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("원격 질의 처리 중 panic 복구",
				"query_id", queryID, "panic", r, "stack", string(debug.Stack()))
			c.sendQueryResult(conn, QueryResultPayload{
				QueryID: queryID,
				OK:      false,
				Error:   "internal error while handling query",
			})
		}
	}()

	var q QueryPayload
	if err := json.Unmarshal(payload, &q); err != nil {
		c.logger.Debug("query 디코드 실패", "error", err)
		return
	}
	if q.QueryID == "" {
		c.logger.Warn("query 에 query_id 누락 — 무시")
		return
	}
	queryID = q.QueryID

	// 1) 승인 게이팅(REQ-J05). 미승인 노드는 거부(handleCommand 와 동일 정신).
	if !c.hasToken() {
		c.logger.Warn("미승인 노드 — 원격 질의 거부",
			"query_id", q.QueryID, "domain", q.Domain, "query_action", q.QueryAction)
		c.sendQueryResult(conn, QueryResultPayload{
			QueryID: q.QueryID, OK: false, Error: "node not approved",
		})
		return
	}

	// 2) READ-ONLY allowlist(REQ-J03/J04). 미열거·변경 의미 action 거부.
	if !IsAllowedQueryAction(q.Domain, q.QueryAction) {
		c.logger.Warn("미허용 query-action — 거부(READ-ONLY)",
			"query_id", q.QueryID, "domain", q.Domain, "query_action", q.QueryAction)
		c.sendQueryResult(conn, QueryResultPayload{
			QueryID: q.QueryID, OK: false,
			Error: fmt.Sprintf("query action not allowed: %s/%s", q.Domain, q.QueryAction),
		})
		return
	}

	// 3) QuerySource 미구성이면 거부(미구성 노드 보호).
	if c.cfg.QuerySource == nil {
		c.logger.Warn("query source 미구성 — 원격 질의 거부", "query_id", q.QueryID)
		c.sendQueryResult(conn, QueryResultPayload{
			QueryID: q.QueryID, OK: false, Error: "query source not configured",
		})
		return
	}

	// 4) 디스패치(A10 — 노드의 로컬 read 핸들러 재실행). Args 는 시크릿 가능성으로 로깅 제외.
	data, err := c.cfg.QuerySource.Query(ctx, q.Domain, q.QueryAction, q.Args)
	if err != nil {
		c.logger.Warn("원격 질의 처리 실패",
			"query_id", q.QueryID, "domain", q.Domain,
			"query_action", q.QueryAction, "error", err)
		c.sendQueryResult(conn, QueryResultPayload{
			QueryID: q.QueryID, OK: false, Error: err.Error(),
		})
		return
	}

	// 5) redaction(REQ-J06 — 노드가 전송 전 마스킹). 미구성이면 pass-through.
	data = c.redact(data)

	c.sendQueryResult(conn, QueryResultPayload{
		QueryID: q.QueryID, OK: true, Data: data,
	})
}

// redact 는 QueryRedactor 가 구성된 경우 본문을 마스킹한다(REQ-J06). 미구성/빈 본문은
// 그대로 반환한다(graceful — 비시크릿 데이터·테스트).
func (c *Client) redact(data json.RawMessage) json.RawMessage {
	if c.cfg.QueryRedactor == nil || len(data) == 0 {
		return data
	}
	return c.cfg.QueryRedactor.Redact(data)
}

// sendQueryResult 는 query_result 를 연결로 전송한다(REQ-J02). 전송 실패는 로깅만 한다.
func (c *Client) sendQueryResult(conn Conn, res QueryResultPayload) {
	msg, err := NewQueryResultMessage(res)
	if err != nil {
		c.logger.Error("query_result 인코딩 실패", "error", err)
		return
	}
	if err := writeEnvelope(conn, msg); err != nil {
		c.logger.Debug("query_result 전송 실패", "query_id", res.QueryID, "error", err)
	}
}
