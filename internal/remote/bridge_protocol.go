// bridge_protocol.go 는 라이브 플로우 브리지(live flow bridge)의 관리 WS 메시지 Type
// 상수와 페이로드 구조를 정의한다(@SPEC:SPEC-SUBFLOW-001 그룹 RB, REQ-SUBFLOW-RB01~RB04,
// spec §5.14).
//
// 배경 — v1.2 임베딩 → v1.3 라이브 브리지(SUPERSEDE):
//
//	v1.2 는 원격 서브플로우 정의를 매니저로 fetch 하여 인라인 확장·실행했다. 그러나
//	매니저에는 원격 노드의 디바이스가 없어 device-bound 플로우가 무출력이었다. v1.3 은
//	원격 참조 flow-node 를 "라이브 브리지"로 전환한다: 원격 플로우는 원격 노드에서
//	그 노드의 디바이스/에이전트/시크릿으로 실행되고, 로컬 flow-node 는 기존 remote WS
//	세션 위의 브리지 엔드포인트가 되어 메시지를 양방향으로 패싱한다.
//
// 전송 — 기존 remote WS 세션 재사용(REQ-SUBFLOW-RB02):
//
//	신규 서버/포트/인증을 만들지 않는다. SPEC-REMOTE-001 의 영속 WS 세션(command 그룹
//	D·query/stream 그룹 J 운반) 위에 flow-bridge 메시지 타입만 추가한다. 봉투는 기존
//	ws.Message{Type, Payload, Timestamp} + ws.NewMessage 를 그대로 사용한다.
//
// READ+WRITE 양방향(REQ-SUBFLOW-RB04):
//
//	command(그룹 D)·query/stream(그룹 J) RPC 와 대칭하되, 브리지는 입력 주입(WRITE)+
//	출력 수신(READ)을 모두 포함한다. 따라서 READ-ONLY 인 query/stream 프록시(REQ-J03)와
//	구분되며, 입력 방향은 별도 쓰기 authz(REQ-SUBFLOW-RB11)를 가진다(P2/P3 강제).
//
// 상관(correlation) — bridge_id:
//
//	매니저가 각 원격 참조 flow-node 인스턴스에 고유 bridge_id 를 부여한다(REQ-SUBFLOW-
//	RB03/RB12). command_id(REQ-D07)·query_id(REQ-J02)·subscription_id(REQ-J08b)와 같은
//	패턴으로, 한 세션 위 다수 브리지를 구분한다. command/query 와 달리 브리지는 1:1
//	요청-응답이 아니라 open~close 사이 다수 input/output 프레임이 흐르는 지속 채널이다.
//
// 메시지 흐름(intended message flow — P2/P3 구현 계약):
//
//	manager(server)                              node(client)
//	  │  bridge_open{bridge_id, instance_id,        │
//	  │             remote_flow_id}                 │
//	  │ ──────────────────────────────────────────▶│  참조 플로우를 running 으로 만들고
//	  │                                             │  입출력 경계 포트를 tap.
//	  │  bridge_open_ack{bridge_id, ok,             │
//	  │      input_ports[], output_ports[], error?} │
//	  │ ◀──────────────────────────────────────────│  실제 경계 포트 보고(authoritative).
//	  │                                             │
//	  │  bridge_input{bridge_id, port, data}        │
//	  │ ──────────────────────────────────────────▶│  원격 입력 경계 포트로 주입(WRITE).
//	  │                                             │
//	  │  bridge_output{bridge_id, port, data}       │
//	  │ ◀──────────────────────────────────────────│  원격 출력 경계 포트 메시지 중계(READ).
//	  │                                             │
//	  │  bridge_status{bridge_id, status, error?}   │
//	  │ ◀──────────────────────────────────────────│  running/stopped/error(라이프사이클).
//	  │                                             │
//	  │  bridge_close{bridge_id, reason}            │  (양방향 — 매니저 undeploy 또는
//	  │ ◀─────────────────────────────────────────▶│   노드 측 오류/정지 시 발신)
//
// 본 파일은 P1 범위로 메시지 타입·페이로드·생성자만 정의한다. 서버/노드 측 라우팅·
// 디스패치·라이프사이클(open ack 처리·경계 포트 tap·teardown)은 P2(노드 측)·P3(매니저
// 측 엔진 통합)에서 구현한다.
package remote

import (
	"encoding/json"

	"github.com/xtra/xflow/internal/api/ws"
)

// flow-bridge 메시지 Type 상수 (spec §5.14, REQ-SUBFLOW-RB03). command/query/stream 과
// 대칭하되 READ+WRITE 양방향이다(REQ-SUBFLOW-RB04 — read-only 프록시와 구분).
const (
	// TypeBridgeOpen 은 브리지 개설 요청이다(server→node, REQ-SUBFLOW-RB05). 노드는
	// 참조 플로우를 running 으로 만들고 입출력 경계 포트를 tap 한다. stream subscribe 와
	// 유사하나 양방향 채널을 연다.
	TypeBridgeOpen = "bridge_open"
	// TypeBridgeOpenAck 은 브리지 개설 결과이다(node→server, REQ-SUBFLOW-RB05/RB06).
	// 실행 시작 성공/실패와 노드의 실제 입출력 경계 포트를 보고한다(authoritative).
	TypeBridgeOpenAck = "bridge_open_ack"
	// TypeBridgeInput 은 로컬 입력 핸들 메시지를 원격 입력 경계 포트로 전달한다
	// (server→node, WRITE — REQ-SUBFLOW-RB04/RB07).
	TypeBridgeInput = "bridge_input"
	// TypeBridgeOutput 은 원격 출력 경계 포트 메시지를 로컬 출력 핸들로 전달한다
	// (node→server, READ — REQ-SUBFLOW-RB07). 노드 측 redaction 적용 후 전송한다.
	TypeBridgeOutput = "bridge_output"
	// TypeBridgeStatus 는 브리지 라이프사이클/헬스 신호이다(node→server, REQ-SUBFLOW-
	// RB09). running/stopped/offline/error.
	TypeBridgeStatus = "bridge_status"
	// TypeBridgeClose 는 브리지 teardown 이다(both — REQ-SUBFLOW-RB09). 매니저 undeploy
	// 또는 노드 측 오류/정지 시 어느 쪽이든 발신할 수 있다.
	TypeBridgeClose = "bridge_close"
)

// 브리지 상태 문자열 상수 (spec §5.14/§5.15, REQ-SUBFLOW-RB09). bridge_status.status 와
// FlowBridge 상태 표시기(REQ-SUBFLOW-RU06)에서 공유한다.
const (
	// BridgeStateRunning 은 원격 플로우가 실행 중이고 브리지가 활성인 상태이다.
	BridgeStateRunning = "running"
	// BridgeStateStopped 은 원격 플로우가 정지된(정상 teardown) 상태이다.
	BridgeStateStopped = "stopped"
	// BridgeStateOffline 은 노드 오프라인/WS 끊김으로 브리지 불가 상태이다(무출력).
	BridgeStateOffline = "offline"
	// BridgeStateError 는 노드 측 open/실행 오류 상태이다(무출력, detail 에 사유).
	BridgeStateError = "error"
)

// BridgeOpenPayload 는 브리지 개설 요청 페이로드이다(server→node, REQ-SUBFLOW-RB05,
// spec §5.14).
//
// BridgeID 로 브리지 인스턴스를 상관한다(REQ-SUBFLOW-RB03/RB12 — 한 세션 위 다수 브리지
// 구분). InstanceID 는 대상 원격 노드이며, RemoteFlowID 는 그 노드에서 실행할 참조
// 플로우 id 이다(remote://{instance_id}/{remote_flow_id} 정규형에서 분해 — REQ-SUBFLOW-R01).
//
// 노드는 RemoteFlowID 플로우를 deploy+start(소유 모델 = 권고 자동 배포, OQ-RB1/RB5)하고
// 입출력 경계 포트를 tap 한 뒤 bridge_open_ack 로 실제 경계 포트를 보고한다. 경계 포트는
// 노드가 권위(authoritative)이므로 open 요청에는 싣지 않는다(ack 에서 보고).
type BridgeOpenPayload struct {
	BridgeID     string `json:"bridge_id"`
	InstanceID   string `json:"instance_id"`
	RemoteFlowID string `json:"remote_flow_id"`
}

// BridgeOpenAckPayload 는 브리지 개설 결과 페이로드이다(node→server, REQ-SUBFLOW-RB05/
// RB06, spec §5.14).
//
// BridgeID 로 원본 open 과 상관된다. OK 가 true 이면 참조 플로우 실행 시작 성공이며,
// InputPorts/OutputPorts 에 노드가 tap 한 실제 입출력 경계 포트 이름을 보고한다(이름 기반
// 매핑 — REQ-SUBFLOW-RB06). OK 가 false 이면 Error 에 실패 사유가 담긴다(502 계열 매핑).
type BridgeOpenAckPayload struct {
	BridgeID    string   `json:"bridge_id"`
	OK          bool     `json:"ok"`
	InputPorts  []string `json:"input_ports,omitempty"`
	OutputPorts []string `json:"output_ports,omitempty"`
	Error       string   `json:"error,omitempty"`
}

// BridgeInputPayload 는 로컬 입력 핸들 메시지를 원격 입력 경계 포트로 주입하는
// 페이로드이다(server→node, WRITE — REQ-SUBFLOW-RB04/RB07, spec §5.14).
//
// BridgeID 로 브리지와 상관된다. Port 는 원격 플로우의 입력 경계 포트 이름이며(이름 기반
// 매핑 — REQ-SUBFLOW-RB06), Data 는 불투명한 메시지 페이로드이다(json.RawMessage). 입력
// 방향은 쓰기 authz(REQ-SUBFLOW-RB11)에 종속된다 — 미승인/범위 밖 주입은 노드가 거부한다.
type BridgeInputPayload struct {
	BridgeID string          `json:"bridge_id"`
	Port     string          `json:"port"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// BridgeOutputPayload 는 원격 출력 경계 포트 메시지를 로컬 출력 핸들로 전달하는
// 페이로드이다(node→server, READ — REQ-SUBFLOW-RB07, spec §5.14).
//
// BridgeID 로 브리지와 상관된다. Port 는 원격 플로우의 출력 경계 포트 이름이며, Data 는
// 노드 측 redaction(SPEC-REMOTE-001 REQ-F06/J06 준용 — §5.14)을 적용한 뒤의 페이로드이다.
type BridgeOutputPayload struct {
	BridgeID string          `json:"bridge_id"`
	Port     string          `json:"port"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// BridgeStatusPayload 는 브리지 라이프사이클/헬스 페이로드이다(node→server,
// REQ-SUBFLOW-RB09, spec §5.14/§5.15).
//
// BridgeID 로 브리지와 상관된다. Status 는 running/stopped/offline/error 중 하나이며,
// Error 가 비어 있지 않으면 노드 측 오류 사유이다(flow-node 상태 표시기 REQ-SUBFLOW-RU06).
type BridgeStatusPayload struct {
	BridgeID string `json:"bridge_id"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

// BridgeClosePayload 는 브리지 teardown 페이로드이다(both, REQ-SUBFLOW-RB09, spec §5.14).
//
// BridgeID 로 브리지와 상관된다. Reason 은 teardown 사유(로컬 undeploy/노드 정지/오류
// 등)이다. 매니저(로컬 undeploy)·노드(정지/오류) 어느 쪽이든 발신할 수 있다.
type BridgeClosePayload struct {
	BridgeID string `json:"bridge_id"`
	Reason   string `json:"reason,omitempty"`
}

// NewBridgeOpenMessage 는 BridgeOpenPayload 를 ws.Message 봉투로 인코딩한다(REQ-SUBFLOW-RB05).
func NewBridgeOpenMessage(p BridgeOpenPayload) (*ws.Message, error) {
	return ws.NewMessage(TypeBridgeOpen, p)
}

// NewBridgeOpenAckMessage 는 BridgeOpenAckPayload 를 ws.Message 봉투로 인코딩한다
// (REQ-SUBFLOW-RB05/RB06).
func NewBridgeOpenAckMessage(p BridgeOpenAckPayload) (*ws.Message, error) {
	return ws.NewMessage(TypeBridgeOpenAck, p)
}

// NewBridgeInputMessage 는 BridgeInputPayload 를 ws.Message 봉투로 인코딩한다
// (REQ-SUBFLOW-RB04/RB07 — WRITE).
func NewBridgeInputMessage(p BridgeInputPayload) (*ws.Message, error) {
	return ws.NewMessage(TypeBridgeInput, p)
}

// NewBridgeOutputMessage 는 BridgeOutputPayload 를 ws.Message 봉투로 인코딩한다
// (REQ-SUBFLOW-RB07 — READ).
func NewBridgeOutputMessage(p BridgeOutputPayload) (*ws.Message, error) {
	return ws.NewMessage(TypeBridgeOutput, p)
}

// NewBridgeStatusMessage 는 BridgeStatusPayload 를 ws.Message 봉투로 인코딩한다
// (REQ-SUBFLOW-RB09).
func NewBridgeStatusMessage(p BridgeStatusPayload) (*ws.Message, error) {
	return ws.NewMessage(TypeBridgeStatus, p)
}

// NewBridgeCloseMessage 는 BridgeClosePayload 를 ws.Message 봉투로 인코딩한다
// (REQ-SUBFLOW-RB09).
func NewBridgeCloseMessage(p BridgeClosePayload) (*ws.Message, error) {
	return ws.NewMessage(TypeBridgeClose, p)
}
