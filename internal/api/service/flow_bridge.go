package service

import (
	"context"
	"encoding/json"
)

// ---------------------------------------------------------------------------
// 매니저 측 라이브 플로우 브리지 계약 — SPEC-SUBFLOW-001 v1.3 그룹 RB (P3 소비)
// ---------------------------------------------------------------------------
//
// 원격 참조 flow-node(remote://...)는 v1.3 에서 라이브 브리지로 동작한다. ExpandSubflows
// 는 원격 참조를 확장하지 않고 flow-node 를 라이브 노드로 남긴다(REQ-SUBFLOW-RB01/RB07).
// P3 의 매니저 측 엔진 통합은 그 flow-node 를 브리지 엔드포인트로 실행하며, 본 파일이
// 정의하는 FlowBridgeOpener/FlowBridge 인터페이스를 통해 원격 노드와 통신한다.
//
// 구현 위치 / import-cycle 회피:
//
//	본 인터페이스는 service 패키지에 둔다(엔진/서비스 소비처). 구체 구현은 remote.Server
//	의 flow-bridge 메시지(internal/remote/bridge_protocol.go: bridge_open/bridge_open_ack/
//	bridge_input/bridge_output/bridge_status/bridge_close)를 운반하며, remote.Server 와
//	service 를 모두 아는 와이어링 계층(cmd/xflowd)에서 P3 에 주입한다. 따라서 service
//	패키지는 internal/remote 를 import 하지 않는다(과거 RemoteFlowFetcher 와 동일한
//	import-cycle 회피 전략).
//
// 의도된 메시지 흐름(intended message flow — bridge_protocol.go 와 대응):
//
//	1. OpenBridge(ctx, instanceID, remoteFlowID, inputPorts, outputPorts)
//	     → 매니저가 bridge_open 을 노드에 전송. 노드가 참조 플로우를 running 으로 만들고
//	       경계 포트를 tap 한 뒤 bridge_open_ack 를 반환하면 FlowBridge 핸들이 활성화된다.
//	       inputPorts/outputPorts 는 flow-node 가 보유한 핸들(원격 플로우 inputs/outputs
//	       에서 산출 — REQ-SUBFLOW-RU05)이며, 이름 기반 매핑(RB06)에 사용된다.
//	2. FlowBridge.SendInput(port, data)
//	     → 로컬 입력 핸들 수신 메시지를 bridge_input 으로 노드의 원격 입력 경계 포트에
//	       주입한다(WRITE — REQ-SUBFLOW-RB07, 쓰기 authz RB11 종속).
//	3. FlowBridge.Outputs()
//	     → 노드의 bridge_output(원격 출력 경계 포트 메시지)을 채널로 수신한다(READ).
//	       P3 엔진이 이를 flow-node 출력 핸들의 하류 와이어로 emit 한다.
//	4. FlowBridge.Status()
//	     → bridge_status(running/stopped/offline/error)를 채널로 수신한다. flow-node
//	       상태 표시기(REQ-SUBFLOW-RU06)와 오프라인 무출력 처리(RB09)에 사용된다.
//	5. FlowBridge.Close()
//	     → bridge_close 를 전송하고 채널을 정리한다(teardown — 로컬 undeploy/노드 오프라인).
//
// 본 파일은 P1 범위로 인터페이스와 값 타입만 정의한다. 구현은 없다(P3).

// BridgeOutput 은 원격 출력 경계 포트에서 로컬 출력 핸들로 전달되는 단일 메시지이다
// (bridge_output 페이로드의 service 측 표현 — REQ-SUBFLOW-RB07).
//
//   - Port: 원격 플로우의 출력 경계 포트 이름(이름 기반 매핑 — REQ-SUBFLOW-RB06).
//   - Data: 노드 측 redaction 적용 후의 불투명 페이로드(§5.14).
type BridgeOutput struct {
	Port string
	Data json.RawMessage
}

// BridgeInputMsg 는 로컬 입력 핸들에서 원격 입력 경계 포트로 주입되는 단일 메시지이다
// (bridge_input 페이로드의 service 측 표현 — REQ-SUBFLOW-RB07, WRITE).
//
// SendInput 의 인자를 구조화한 형태로, 구현/테스트가 전송 이력을 표현하는 데 쓸 수 있다.
type BridgeInputMsg struct {
	Port string
	Data json.RawMessage
}

// BridgeStatus 는 브리지 라이프사이클/헬스 상태이다(bridge_status 페이로드의 service 측
// 표현 — REQ-SUBFLOW-RB09).
//
//   - State: running/stopped/offline/error(remote.BridgeState* 와 동일 문자열 규약).
//   - Detail: 오류/사유 상세(선택).
type BridgeStatus struct {
	State  string
	Detail string
}

// FlowBridge 는 한 원격 참조 flow-node 인스턴스의 라이브 브리지 핸들이다
// (REQ-SUBFLOW-RB07/RB12 — flow-node 별 독립 브리지). 매니저 측 엔진(P3)이 소비한다.
//
// 구현은 remote.Server 의 WS 세션 위 bridge_* 메시지로 동작하며, 한 핸들은 하나의
// bridge_id 에 대응한다(인스턴스 독립 — REQ-SUBFLOW-RB12).
type FlowBridge interface {
	// SendInput 은 로컬 입력 핸들 메시지를 원격 입력 경계 포트(port)로 주입한다(WRITE,
	// bridge_input — REQ-SUBFLOW-RB07). port 는 원격 플로우 입력 경계 포트 이름이다(RB06).
	SendInput(port string, data json.RawMessage) error

	// Outputs 는 원격 출력 경계 포트 메시지(bridge_output)를 수신하는 채널을 반환한다
	// (READ — REQ-SUBFLOW-RB07). Close 시 닫힌다. P3 엔진이 출력 핸들 하류로 emit 한다.
	Outputs() <-chan BridgeOutput

	// Status 는 브리지 라이프사이클/헬스(bridge_status)를 수신하는 채널을 반환한다
	// (REQ-SUBFLOW-RB09). Close 시 닫힌다. flow-node 상태 표시기(RU06)에 사용된다.
	Status() <-chan BridgeStatus

	// Close 는 브리지를 teardown 한다(bridge_close — REQ-SUBFLOW-RB09). 멱등이어야 하며,
	// Outputs/Status 채널을 닫는다. 로컬 undeploy/노드 오프라인 시 호출된다.
	Close() error
}

// FlowBridgeOpener 는 원격 참조 flow-node 를 위한 라이브 브리지를 개설하는 팩토리이다
// (REQ-SUBFLOW-RB05). 매니저 측 엔진(P3)이 배포 시 원격 참조 flow-node 마다 호출한다.
//
// 구체 구현은 remote.Server 위에서 bridge_open 을 노드에 전송하고 bridge_open_ack 를
// 기다린 뒤 FlowBridge 핸들을 반환한다(cmd/xflowd 와이어링에서 주입 — import-cycle 회피).
// 게이팅(승인∧온라인∧노출 — REQ-SUBFLOW-RB08)/쓰기 authz(RB11)는 구현이 강제한다.
type FlowBridgeOpener interface {
	// OpenBridge 는 instanceID 노드에서 remoteFlowID 플로우를 running 으로 만들고
	// 브리지를 연다(REQ-SUBFLOW-RB05). inputPorts/outputPorts 는 flow-node 가 보유한
	// 입출력 핸들 이름(원격 플로우 inputs/outputs 산출 — RU05)이며, 이름 기반 경계 포트
	// 매핑(RB06)에 사용된다. 게이팅 실패/오프라인/노드 오류 시 에러를 반환한다(RB08/RB09).
	OpenBridge(ctx context.Context, instanceID, remoteFlowID string, inputPorts, outputPorts []string) (FlowBridge, error)
}
