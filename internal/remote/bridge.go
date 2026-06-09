// bridge.go 는 P2(노드 측 라이브 브리지)의 소스 인터페이스를 정의한다
// (@SPEC:SPEC-SUBFLOW-001 그룹 RB, REQ-SUBFLOW-RB05/RB06/RB07/RB10/RB11, plan §2-bis P2).
//
// 디커플링(import cycle 회피 — apply.go / query.go 와 동일 패턴):
//
//	internal/remote 는 internal/api/service / internal/engine / pkg/flow 를 import
//	하지 않는다(handler/service 가 remote 를 import 하므로 역방향은 cycle). 본 파일은
//	중립 BridgeFlowRunner / BridgeHandle 인터페이스만 정의하고, 구체 구현(참조 플로우의
//	배포·시작·정지 + 경계 포트 tap/inject)은 internal/api/service 의 어댑터가 담당하며
//	cmd/xflowd 가 client 에 주입한다(spec §5.15/§5.16 — 노드가 참조 플로우를 실행하고
//	입출력 경계 포트를 중계).
//
// 소유 모델(OQ-RB1/RB5 ✅ RESOLVED — 매니저 관리형 자동 배포):
//
//	client 는 bridge_open 수신 시 BridgeFlowRunner.OpenBridge 로 참조 플로우가 running
//	이 되도록 보장한다(미실행이면 deploy+start, 실행 중이면 재사용). "이 브리지가 시작
//	시켰는가"의 추적·정지 판단(다른 브리지가 아직 쓰는지 refcount)은 runner 가 소유하여
//	client(remote)를 엔진 라이프사이클 지식으로부터 격리한다. client 는 OpenBridge /
//	Inject / Close 와 onOutput 콜백만 사용한다.
//
// 백프레셔(RB10 — OQ-RB3): onOutput 콜백은 엔진 스레드에서 인라인 호출되므로 절대
//
//	블로킹하면 안 된다. client 의 onOutput 핸들러는 포트별 bounded buffer 에 enqueue
//	(+coalesce)만 하고 즉시 반환하며, 별도 펌프 고루틴이 WS 로 drain 한다(client_stream
//	의 drain-to-latest 패턴 재사용). 느린 WS write 가 엔진을 막지 않는다.
package remote

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrBridgeRunnerUnavailable 은 BridgeFlowRunner 가 구성되지 않은 노드에서 bridge_open
// 을 받았을 때 반환된다(미구성 노드 보호 — Applier/QuerySource 미구성과 동일 정신).
var ErrBridgeRunnerUnavailable = errors.New("remote: bridge flow runner not configured")

// BridgeOutputFunc 는 참조 플로우의 출력 경계 포트에서 나온 메시지 1건을 전달하는
// 콜백이다(REQ-SUBFLOW-RB07 — READ). port 는 출력 경계 포트 이름, data 는 메시지
// 페이로드 JSON(redaction 전)이다.
//
// 계약(RB10): 이 콜백은 엔진의 출력 경계 tap 에서 인라인 호출되므로 절대 블로킹하면
// 안 된다. 구현(client)은 enqueue+coalesce 후 즉시 반환한다.
type BridgeOutputFunc func(port string, data json.RawMessage)

// BridgeFlowRunner 는 노드가 참조 플로우를 라이브 실행하고 입출력 경계 포트를 tap 하는
// 추상화이다(REQ-SUBFLOW-RB05/RB07, spec §5.15/§5.16). client 는 이 인터페이스만
// 의존하며, cmd/xflowd 가 구체 어댑터(엔진/서비스 위에 구현)를 바인딩한다.
type BridgeFlowRunner interface {
	// OpenBridge 는 flowID 참조 플로우가 running 이 되도록 보장하고(미실행이면 deploy+
	// start, 실행 중이면 재사용 — OQ-RB1/RB5), 입출력 경계 포트를 tap 한다. onOutput 은
	// 출력 경계 포트 메시지마다 호출된다(블로킹 금지 — RB10). 성공 시 노드가 권위인
	// 실제 입출력 경계 포트 이름을 보고하는 BridgeHandle 을 반환한다(REQ-SUBFLOW-RB06).
	//
	// 참조 플로우가 없거나 실행 시작에 실패하면 오류를 반환한다(client 가 502 계열
	// bridge_open_ack{ok:false}로 매핑).
	OpenBridge(ctx context.Context, flowID string, onOutput BridgeOutputFunc) (BridgeHandle, error)
}

// BridgeHandle 은 단일 활성 브리지(참조 플로우 1 인스턴스 + 경계 tap)의 핸들이다.
// client 의 per-bridge 레지스트리가 보관하며, bridge_input 주입과 teardown 에 사용한다.
type BridgeHandle interface {
	// InputPorts 는 참조 플로우의 입력 경계 포트 이름 목록이다(authoritative — RB06).
	InputPorts() []string
	// OutputPorts 는 참조 플로우의 출력 경계 포트 이름 목록이다(authoritative — RB06).
	OutputPorts() []string
	// Inject 는 입력 경계 포트(port)로 메시지(data JSON)를 주입한다(REQ-SUBFLOW-RB07 —
	// WRITE). open 시점에 쓰기 authz 가 게이팅되었으므로(RB11) per-message 는 열린
	// 브리지 범위 내에서 수행된다. 미지의 포트/주입 실패는 오류를 반환한다.
	Inject(ctx context.Context, port string, data json.RawMessage) error
	// Close 는 브리지 tap 을 해제하고, 이 브리지가 auto-start 했고 다른 브리지가 더
	// 필요로 하지 않으면 참조 플로우를 정지한다(매니저 관리형 라이프사이클 — OQ-RB1/RB5).
	// 멱등하다(중복 호출 안전).
	Close(ctx context.Context) error
}

// BridgeAuditSink 는 노드 측 브리지 감사 기록 추상화이다(REQ-SUBFLOW-RB06/RB11).
// 브리지 open/close 와 입력 주입(who/when/node/flow)을 기록하되, 시크릿 페이로드 값은
// 절대 기록하지 않는다(REQ-SUBFLOW-RB06 — 시크릿 비노출, server audit.go 와 동일 정신).
//
// nil 이면 client 는 구조화 로그로만 감사한다(노드-로컬 audit 저장소가 없는 배포 허용).
type BridgeAuditSink interface {
	// RecordBridge 는 브리지 라이프사이클 이벤트를 기록한다. event 는
	// "open"/"open_rejected"/"close"/"input" 중 하나, flowID 는 원격 참조 플로우 id,
	// detail 은 사유/포트 등 비시크릿 메타이다(payload 값 제외).
	RecordBridge(ctx context.Context, event, bridgeID, flowID, detail string)
}
