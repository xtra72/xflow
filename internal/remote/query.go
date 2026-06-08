// query.go 는 M8(그룹 J) 노드 측 READ/QUERY 프록시와 스트리밍 프록시의 소스
// 인터페이스를 정의한다(@SPEC:SPEC-REMOTE-001 M8, spec §5.5/§5.10, REQ-J01/J03/J06/J08).
//
// 디커플링(import cycle 회피, M3 apply.go / M4 inventory.go 와 동일 패턴):
//
//	internal/remote 는 internal/api/service / internal/api/handler 를 import 하지
//	않는다(handler 가 remote 를 import 하므로 역방향은 cycle). 본 파일은 중립
//	QuerySource / StreamSource 인터페이스만 정의하고, 구체 어댑터(FlowServiceAdapter
//	의 FlowStatus/ListFlowNodes, AgentServiceAdapter 의 AgentStats/GetAgent, device
//	레지스트리의 State/Commands 등) 바인딩은 cmd/xflowd 가 담당한다(spec §5.5 어댑터
//	브리지, A10 — 노드의 기존 로컬 read 핸들러 재실행).
//
// READ-ONLY(REQ-J03):
//
//	QuerySource/StreamSource 는 읽기 전용이다. 변경 의미 action 은 client 가 allowlist
//	(IsAllowedQueryAction/IsStreamableAction)로 거부하며 소스에 도달하지 않는다.
//
// redaction(REQ-J06):
//
//	노드는 query/stream 응답을 전송 전 마스킹한다. remote 는 handler.RedactSensitiveConfig
//	에 비의존하므로(cycle), client 가 ClientConfig.QueryRedactor(cmd/xflowd 가 secret_fields
//	SoT 로 구성)를 적용한다. 미구성이면 pass-through(테스트/미시크릿 데이터).
package remote

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrQueryActionUnsupported 는 노드가 해당 query-action 을 로컬 read 핸들러로 매핑할
// 수 없을 때(데이터 미가용) 반환된다. 패닉 대신 명확한 not-supported 오류를 돌려
// 서버가 502(node-error)로 매핑하게 한다(REQ-J07).
var ErrQueryActionUnsupported = errors.New("remote: query action not supported by node")

// QuerySource 는 노드의 로컬 read 핸들러를 per-domain query-action 으로 노출하는
// 추상화이다(REQ-J01/J04, A10). client 는 이 인터페이스만 의존하며, cmd/xflowd 가
// 구체 어댑터를 바인딩한다.
//
// 반환 JSON 은 redaction 전의 원본일 수 있다. client 가 전송 전 QueryRedactor 로
// 마스킹한다(REQ-J06). 데이터를 매핑할 수 없으면 ErrQueryActionUnsupported 를
// 반환한다(패닉 금지).
type QuerySource interface {
	// Query 는 domain/queryAction 에 따라 노드의 로컬 read 핸들러를 호출하고 결과
	// JSON 을 반환한다. 미지원 action 은 ErrQueryActionUnsupported 를 반환한다.
	Query(ctx context.Context, domain, queryAction string, args json.RawMessage) (json.RawMessage, error)
}

// QueryRedactor 는 query/stream 응답 본문을 전송 전 마스킹하는 추상화이다(REQ-J06).
// cmd/xflowd 가 secret_fields SoT(handler.RedactSensitiveConfig/IsSensitiveConfigKey)
// 로 구성한다. nil 이면 client 는 pass-through 한다.
type QueryRedactor interface {
	// Redact 는 JSON 본문의 시크릿 필드를 마스킹/제외하여 반환한다. 비-객체/디코드
	// 불가 본문은 그대로 반환할 수 있다(graceful — 비시크릿 데이터).
	Redact(data json.RawMessage) json.RawMessage
}

// QueryRedactorFunc 는 함수를 QueryRedactor 로 어댑트한다.
type QueryRedactorFunc func(data json.RawMessage) json.RawMessage

// Redact 는 QueryRedactor 를 구현한다.
func (f QueryRedactorFunc) Redact(data json.RawMessage) json.RawMessage { return f(data) }

// StreamSubscription 은 단일 실시간 스트림 구독을 나타낸다(REQ-J08/J08b).
//
// Updates 채널로 소스 갱신을 수신하고, Close 로 소스 구독을 해제한다(teardown).
// 구현(cmd/xflowd)은 디바이스 실시간 상태/에이전트 라이브 통계 등을 폴링/구독하여
// Updates 로 흘린다. 백프레셔를 위해 Updates 는 bounded buffer 여야 하며, full 시
// 소스는 최신값 우선으로 coalesce/drop 한다(메모리 폭증 방지 — REQ-J08b).
type StreamSubscription interface {
	// Updates 는 redaction 전 갱신 JSON 을 흘리는 채널이다. 소스 종료 시 close 될 수
	// 있다(client 펌프는 close 를 정상 종료로 처리).
	Updates() <-chan json.RawMessage
	// Close 는 소스 구독을 해제한다(멱등). unsubscribe / 세션 종료 시 호출된다.
	Close() error
}

// StreamSource 는 노드의 실시간 데이터 소스를 stream-action 으로 노출하는 추상화이다
// (REQ-J08, A12). client 는 이 인터페이스만 의존하며, cmd/xflowd 가 구체 소스를
// 바인딩한다. 미지원 stream-action 은 ErrQueryActionUnsupported 를 반환한다.
type StreamSource interface {
	// Subscribe 는 domain/streamAction 의 실시간 소스를 구독하고 StreamSubscription 을
	// 반환한다. 미지원이면 ErrQueryActionUnsupported 를 반환한다(패닉 금지).
	Subscribe(ctx context.Context, domain, streamAction string, args json.RawMessage) (StreamSubscription, error)
}
