// protocol.go 는 관리 WS 채널의 메시지 Type 상수와 페이로드 구조를 정의한다
// (REQ-REMOTE-N04, spec §5.1). 전송 봉투는 기존 internal/api/ws.Message{Type,
// Payload,Timestamp} + ws.NewMessage 를 재사용하며, 신규 메시지 버스를 만들지
// 않는다.
//
// 봉투의 Timestamp 는 RFC3339(ws.NewMessage 규약)이고, 페이로드 내부의 시간
// 필드(ts)는 프로젝트 규약인 epoch milliseconds(int64, UnixMilli)를 사용한다
// (spec §5.1 각주).
//
// M1 범위는 연결 라이프사이클이므로 hello/heartbeat/status 페이로드만 구체화하고,
// register/command/inventory 등의 Type 상수는 후속 마일스톤(M2~M4) 연결을 위해
// 미리 선언만 해 둔다(payload 구조는 해당 마일스톤에서 추가).
package remote

import (
	"time"

	"github.com/xtra/xflow/internal/api/ws"
)

// 관리 메시지 Type 상수 (spec §5.1). 전체 집합을 선언한다.
const (
	// TypeHello 는 클라이언트가 연결 직후 보내는 최소 connect/hello 메시지이다
	// (instance_id + hostname + version 운반). M1 핸드셰이크 식별용.
	//
	// NOTE: spec §5.1 표에는 명시되지 않은 M1 보조 메시지이다. M2 에서 정식
	// register 흐름이 도입되면 hello 는 미등록 노드의 1차 식별 신호로 사용되며,
	// 등록 자체는 TypeRegister 가 담당한다(현재는 seam).
	TypeHello = "hello"

	// TypeRegister 는 등록 요청이다(client→server, REQ-C01, M2).
	TypeRegister = "register"
	// TypeRegisterAck 는 등록 응답·토큰 발급이다(server→client, REQ-C03/C04, M2).
	TypeRegisterAck = "register_ack"
	// TypeCommand 는 원격 명령이다(server→client, REQ-D01, M3).
	TypeCommand = "command"
	// TypeCommandResult 는 명령 결과/ack 이다(client→server, REQ-D05, M3).
	TypeCommandResult = "command_result"
	// TypeInventorySnapshot 는 접속 시 전체 인벤토리이다(client→server, REQ-E01, M4).
	TypeInventorySnapshot = "inventory_snapshot"
	// TypeInventoryDelta 는 변경 델타이다(client→server, REQ-E02, M4).
	TypeInventoryDelta = "inventory_delta"
	// TypeHeartbeat 는 생존성 신호이다(both, REQ-B03).
	TypeHeartbeat = "heartbeat"
	// TypeStatus 는 상태 텔레메트리이다(client→server, REQ-B05 보조).
	TypeStatus = "status"
)

// 등록 상태 문자열 상수 (spec §5.6 상태 머신, managed_nodes.status).
const (
	// RegStatusPending 은 등록 요청이 접수되어 관리자 결정을 대기 중인 상태이다(REQ-C02).
	RegStatusPending = "pending"
	// RegStatusApproved 는 승인되어 노드 토큰을 받은 상태이다(REQ-C03/C04).
	RegStatusApproved = "approved"
	// RegStatusRejected 는 관리자가 거부한 상태이다(REQ-C03).
	RegStatusRejected = "rejected"
	// RegStatusRevoked 는 승인 후 폐기된 상태이다. 토큰은 blacklist 되고 재인증이
	// 거부된다(REQ-C07/F07).
	RegStatusRevoked = "revoked"
)

// HelloPayload 는 최소 connect/hello 페이로드이다(M1 식별용).
// instance_id + hostname + version 으로 노드를 식별한다.
type HelloPayload struct {
	InstanceID string `json:"instance_id"`
	Hostname   string `json:"hostname"`
	Version    string `json:"version"`
}

// ExposureSummary 는 register 요청에 실리는 노출 범위 요약이다(REQ-C01, REQ-A04).
// 각 필드는 노출 정책 문자열("all" | "none" | 목록)이며, 실제 미러링 평가는 M4 에서
// 수행한다. M2 는 등록 요청에 요약을 운반하는 용도로만 사용한다.
type ExposureSummary struct {
	Flows   string `json:"flows,omitempty"`
	Agents  string `json:"agents,omitempty"`
	Devices string `json:"devices,omitempty"`
}

// RegisterPayload 는 등록 요청 페이로드이다(client→server, REQ-C01, spec §5.1).
//
// 미등록(또는 pending) 노드가 자신을 등록하기 위해 보낸다. instance_id 로 노드를
// 식별하고, hostname/version/exposure 요약을 운반한다. BootstrapSecret 은 선택적
// 사전 공유 시크릿으로, 서버에 bootstrap_secret 이 구성된 경우 1차 신뢰 검증에
// 사용된다(REQ-C08). 시크릿이므로 로깅/커밋 대상이 아니다(REQ-F06).
type RegisterPayload struct {
	InstanceID      string          `json:"instance_id"`
	Hostname        string          `json:"hostname,omitempty"`
	Version         string          `json:"version,omitempty"`
	Exposure        ExposureSummary `json:"exposure,omitempty"`
	BootstrapSecret string          `json:"bootstrap_secret,omitempty"`
}

// RegisterAckPayload 는 등록 응답 페이로드이다(server→client, REQ-C03/C04, spec §5.1).
//
// Status 는 pending|approved|rejected 중 하나이다. 승인 시 NodeToken(JWT)이
// 포함되며, 클라이언트는 이를 영속하여 이후 재접속 인증에 사용한다(REQ-C04/C05).
// 거부 시 Reason 으로 사유를 전달할 수 있다(REQ-C03). NodeToken 은 시크릿이므로
// 로깅 대상이 아니다(REQ-F06).
type RegisterAckPayload struct {
	Status    string `json:"status"`
	NodeToken string `json:"node_token,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// HeartbeatPayload 는 heartbeat 페이로드이다(REQ-B03).
// TS 는 epoch milliseconds(int64) 이다.
type HeartbeatPayload struct {
	InstanceID string `json:"instance_id"`
	TS         int64  `json:"ts"`
}

// StatusPayload 는 status 텔레메트리 페이로드이다(REQ-B05 보조).
// TS 는 epoch milliseconds(int64) 이다.
type StatusPayload struct {
	InstanceID string `json:"instance_id"`
	Online     bool   `json:"online"`
	Health     string `json:"health"`
	TS         int64  `json:"ts"`
}

// NewHelloMessage 는 HelloPayload 를 ws.Message 봉투로 인코딩한다.
func NewHelloMessage(p HelloPayload) (*ws.Message, error) {
	return ws.NewMessage(TypeHello, p)
}

// NewHeartbeatMessage 는 instance_id 에 대한 heartbeat 메시지를 생성한다.
// 페이로드의 ts 는 현재 시각의 epoch milliseconds 이다.
func NewHeartbeatMessage(instanceID string) (*ws.Message, error) {
	return ws.NewMessage(TypeHeartbeat, HeartbeatPayload{
		InstanceID: instanceID,
		TS:         time.Now().UnixMilli(),
	})
}

// NewStatusMessage 는 StatusPayload 를 ws.Message 봉투로 인코딩한다.
func NewStatusMessage(p StatusPayload) (*ws.Message, error) {
	return ws.NewMessage(TypeStatus, p)
}

// NewRegisterMessage 는 RegisterPayload 를 ws.Message 봉투로 인코딩한다(REQ-C01).
func NewRegisterMessage(p RegisterPayload) (*ws.Message, error) {
	return ws.NewMessage(TypeRegister, p)
}

// NewRegisterAckMessage 는 RegisterAckPayload 를 ws.Message 봉투로 인코딩한다
// (REQ-C03/C04).
func NewRegisterAckMessage(p RegisterAckPayload) (*ws.Message, error) {
	return ws.NewMessage(TypeRegisterAck, p)
}

// DecodeMessage 는 ws.DecodeMessage 의 패키지-로컬 별칭이다(테스트/호출 편의).
func DecodeMessage(data []byte) (*ws.Message, error) {
	return ws.DecodeMessage(data)
}
