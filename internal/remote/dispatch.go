// dispatch.go 는 M3 서버 측 원격 명령 디스패처를 구현한다
// (@SPEC:SPEC-REMOTE-001 M3, spec §5.5 server / §5.7, REQ-D01/D05/D06/D07/D08, F04/F05).
//
// 디스패치 경로(spec §5.7 서버 측):
//
//  1. 대상 노드가 승인+온라인(managed)인지 검증한다(REQ-D01/D08 — 미승인/오프라인
//     노드는 거절).
//  2. 고유 CommandID 를 부여하고 pending 맵(CommandID → 결과 채널)에 등록한다(REQ-D07).
//  3. 노드의 라이브 연결로 command 를 전송한다(REQ-D01).
//  4. 제한 시간 내 매칭되는 command_result 를 대기한다(REQ-D06). 타임아웃 시 명령을
//     미적용으로 간주하고 pending 을 정리한 뒤 명확한 오류를 반환한다.
//
// 동시 다수 명령은 CommandID 로 구분되어 각자의 결과 채널로 라우팅된다(REQ-D07).
//
// 감사(F05): 디스패치 시작 시 구조화 로그를, 결과(성공/실패/타임아웃) 시 영속 감사
// 레코드를 남긴다(누가[ctx actor]/언제/대상/도메인·액션/ok·err — M6 recordCommandAudit).
// Args 와 클라이언트 오류 사유는 시크릿을 포함할 수 있으므로 verbatim 로깅·기록하지
// 않는다(REQ-F06 — 감사에는 분류만).
package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/xtra/xflow/internal/storage"
)

// ErrNodeNotManaged 는 대상 노드가 승인+온라인이 아닐 때(미승인/오프라인) 반환된다
// (REQ-D08, B07 — 오프라인 명령 거절).
var ErrNodeNotManaged = errors.New("remote: target node is not managed (not approved or offline)")

// ErrNoConn 은 대상 노드의 라이브 연결을 찾을 수 없을 때 반환된다.
var ErrNoConn = errors.New("remote: no live connection for target node")

// ErrCommandTimeout 은 제한 시간 내 command_result 가 도착하지 않을 때 반환된다
// (REQ-D06 — 미적용 처리).
var ErrCommandTimeout = errors.New("remote: command timed out (treated as not applied)")

// ErrCommandFailed 은 클라이언트가 적용 실패(command_result.ok=false)를 보고했을 때
// 반환된다(REQ-D09). 메시지에 클라이언트 오류 사유가 포함된다.
var ErrCommandFailed = errors.New("remote: command failed on node")

// Dispatch 는 승인+온라인 노드에 원격 명령을 디스패치하고 결과를 기다린다
// (REQ-D01/D05/D06/D07/D08).
//
// 반환: 성공 시 클라이언트의 result 페이로드(json.RawMessage), 실패 시 오류.
// 오류 종류:
//   - ErrNodeNotManaged: 미승인/오프라인 노드(REQ-D08/B07).
//   - ErrNoConn: 라이브 연결 부재(경합 상황).
//   - ErrCommandTimeout: 제한 시간 초과(REQ-D06).
//   - ErrCommandFailed: 클라이언트 적용 실패(REQ-D09).
//   - ctx.Err(): 호출자 컨텍스트 취소.
func (s *Server) Dispatch(ctx context.Context, instanceID, domain, action string, args json.RawMessage) (json.RawMessage, error) {
	// 1) 승인+온라인 게이트(M2 IsManaged 재사용 — REQ-D01/D08).
	if !s.IsManaged(instanceID) {
		s.logger.Warn("원격 명령 거절 — 대상 노드 미관리",
			"instance_id", instanceID, "domain", domain, "action", action)
		return nil, ErrNodeNotManaged
	}

	// 2) 라이브 연결 확보.
	nc, ok := s.connFor(instanceID)
	if !ok {
		return nil, ErrNoConn
	}

	// 3) 고유 CommandID + 결과 채널 등록(REQ-D07).
	commandID := uuid.NewString()
	resultCh := s.registerPending(commandID)
	// 어떤 종료 경로(성공/타임아웃/취소)에서도 pending 을 정리한다(goroutine/메모리 누수 방지).
	defer s.clearPending(commandID)

	cmd := CommandPayload{
		CommandID:        commandID,
		TargetInstanceID: instanceID,
		Domain:           domain,
		Action:           action,
		Args:             args,
	}
	msg, err := NewCommandMessage(cmd)
	if err != nil {
		return nil, fmt.Errorf("remote: encode command: %w", err)
	}

	// 감사(F05): 디스패치 시작 구조화 로그. Args 는 시크릿 가능성으로 로깅 제외(REQ-F06).
	// 영속 감사 레코드는 터미널 결과(성공/실패/타임아웃) 시 recordCommandAudit 로 1행
	// 기록한다(아래 select). actor 는 ctx(ContextWithActor)에서 읽는다.
	s.logger.Info("원격 명령 디스패치",
		"command_id", commandID, "instance_id", instanceID,
		"domain", domain, "action", action)

	if err := writeEnvelope(nc.conn, msg); err != nil {
		return nil, fmt.Errorf("remote: send command: %w", err)
	}

	// 4) 결과 대기(타임아웃/취소 처리 — REQ-D06).
	select {
	case res := <-resultCh:
		if !res.OK {
			// 감사(F05): 적용 실패를 영속화한다(result=error). 클라이언트 오류 사유는
			// 시크릿일 수 있으므로 감사 reason 에는 분류만 남기고 verbatim 은 제외한다
			// (REQ-F06 — 시크릿 echo 금지). 구조화 로그도 사유를 남기지 않는다.
			s.logger.Warn("원격 명령 실패",
				"command_id", commandID, "instance_id", instanceID,
				"domain", domain, "action", action)
			s.recordCommandAudit(ctx, instanceID, domain, action,
				storage.AuditResultError, "node reported apply failure")
			return nil, fmt.Errorf("%w: %s", ErrCommandFailed, res.Error)
		}
		// 감사(F05): 적용 성공을 영속화한다(result=ok).
		s.logger.Info("원격 명령 성공",
			"command_id", commandID, "instance_id", instanceID,
			"domain", domain, "action", action)
		s.recordCommandAudit(ctx, instanceID, domain, action, storage.AuditResultOK, "")
		// M8(REQ-J16): 변경 성공 → 해당 노드의 READ 캐시를 무효화한다(stale 방지).
		// 노드 단위 무효화는 보수적이지만 안전하다(변경 args 에서 자원 단위 키를 항상
		// 신뢰 추출할 수 없으므로 — node-scoped 가 최소 안전 보장).
		s.invalidateQueryCacheNode(instanceID)
		return res.Result, nil

	case <-time.After(s.commandTimeout()):
		s.logger.Warn("원격 명령 타임아웃 — 미적용 처리",
			"command_id", commandID, "instance_id", instanceID,
			"domain", domain, "action", action)
		s.recordCommandAudit(ctx, instanceID, domain, action,
			storage.AuditResultError, "command timeout (not applied)")
		return nil, ErrCommandTimeout

	case <-ctx.Done():
		s.recordCommandAudit(ctx, instanceID, domain, action,
			storage.AuditResultError, "context canceled")
		return nil, ctx.Err()
	}
}

// commandTimeout 은 설정된 명령 제한 시간(없으면 기본값)을 반환한다.
func (s *Server) commandTimeout() time.Duration {
	if s.cmdTimeout > 0 {
		return s.cmdTimeout
	}
	return DefaultCommandTimeout
}

// registerPending 은 CommandID 에 대한 결과 채널을 등록하고 반환한다.
// 버퍼 크기 1 로 하여, 결과 라우팅이 Dispatch 의 타임아웃과 경합해도 송신이 절대
// 블로킹되지 않도록 한다(goroutine 누수 방지).
func (s *Server) registerPending(commandID string) chan CommandResultPayload {
	ch := make(chan CommandResultPayload, 1)
	s.pendingMu.Lock()
	s.pending[commandID] = ch
	s.pendingMu.Unlock()
	return ch
}

// clearPending 은 CommandID 의 pending 항목을 제거한다(멱등).
func (s *Server) clearPending(commandID string) {
	s.pendingMu.Lock()
	delete(s.pending, commandID)
	s.pendingMu.Unlock()
}

// routeCommandResult 는 수신한 command_result 를 대기 중인 결과 채널로 라우팅한다
// (REQ-D05/D07). 매칭되는 pending 항목이 없으면(타임아웃 후 늦게 도착 등) 조용히
// 폐기한다.
func (s *Server) routeCommandResult(payload []byte) {
	var res CommandResultPayload
	if err := json.Unmarshal(payload, &res); err != nil {
		s.logger.Debug("command_result 디코드 실패", "error", err)
		return
	}
	if res.CommandID == "" {
		s.logger.Debug("command_result 에 command_id 누락 — 폐기")
		return
	}

	// pending 항목을 찾아 제거한다(1:1 상관 — 동일 결과가 두 번 라우팅되지 않도록).
	s.pendingMu.Lock()
	ch, ok := s.pending[res.CommandID]
	if ok {
		delete(s.pending, res.CommandID)
	}
	s.pendingMu.Unlock()

	if !ok {
		// 늦게 도착한 결과(타임아웃 후) 또는 중복 command_id — 안전하게 폐기한다.
		s.logger.Debug("매칭되는 pending 명령 없음 — command_result 폐기",
			"command_id", res.CommandID)
		return
	}

	// 버퍼 1 채널이므로 송신은 블로킹되지 않는다.
	ch <- res
}
