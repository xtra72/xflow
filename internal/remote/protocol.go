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
	"encoding/json"
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

	// --- M8 그룹 J: READ/QUERY 프록시 + 스트리밍 프록시 (READ-ONLY) ---

	// TypeQuery 는 per-domain query-action READ 질의이다(server→client, REQ-J01/J02).
	TypeQuery = "query"
	// TypeQueryResult 는 질의 응답/ack 이다(client→server, REQ-J01/J06/J07).
	TypeQueryResult = "query_result"
	// TypeSubscribe 는 실시간 스트림 구독 시작이다(server→client, REQ-J08).
	TypeSubscribe = "subscribe"
	// TypeStreamData 는 구독 소스 갱신 push 이다(client→server, REQ-J08).
	TypeStreamData = "stream_data"
	// TypeUnsubscribe 는 스트림 구독 해제·teardown 이다(both, REQ-J08b).
	TypeUnsubscribe = "unsubscribe"
)

// M8(그룹 J) per-domain query-action 상수 (spec §5.10.1 매핑 표, REQ-J04 — FULL
// 커버리지). 동일 문자열이 여러 도메인에 걸쳐 재사용된다(예: list/get 은 모든 도메인).
// allowlist 는 IsAllowedQueryAction 이 도메인별로 강제한다(미열거·변경 의미 action 거부).
//
// 변경 의미 action(create/update/delete/execute/start/stop 등)은 본 집합에 포함되지
// 않으며(READ-ONLY — REQ-J03), 변경은 그룹 D 명령 / 그룹 I CRUD 경로로만 수행된다.
const (
	// QueryActionList 는 목록 질의이다(flow/agent/device). 그룹 E 미러 우선, 라이브 보강.
	QueryActionList = "list"
	// QueryActionGet 은 상세 정의 질의이다(flow/agent/device).
	QueryActionGet = "get"
	// QueryActionStatus 는 플로우 상태 질의이다(flow).
	QueryActionStatus = "status"
	// QueryActionNodes 는 플로우 노드 목록 질의이다(flow).
	QueryActionNodes = "nodes"
	// QueryActionNode 는 단일 플로우 노드 런타임 질의이다(flow).
	QueryActionNode = "node"
	// QueryActionLogs 는 플로우/노드 로그 질의이다(flow).
	QueryActionLogs = "logs"
	// QueryActionStats 는 에이전트 통계 질의이다(agent, 스트림 가능 — 라이브).
	QueryActionStats = "stats"
	// QueryActionConfig 는 에이전트 설정 질의이다(agent).
	QueryActionConfig = "config"
	// QueryActionDevices 는 에이전트 연결 디바이스 질의이다(agent).
	QueryActionDevices = "devices"
	// QueryActionTopics 는 에이전트 토픽 질의이다(agent).
	QueryActionTopics = "topics"
	// QueryActionStore 는 에이전트 store 질의이다(agent).
	QueryActionStore = "store"
	// QueryActionSessions 는 에이전트 세션 질의이다(agent).
	QueryActionSessions = "sessions"
	// QueryActionSeries 는 에이전트 시리즈/TSDB 질의이다(agent, 스트림 가능 — 라이브).
	QueryActionSeries = "series"
	// QueryActionState 는 디바이스 실시간 상태 질의이다(device, 스트림 가능 — 라이브).
	QueryActionState = "state"
	// QueryActionCommands 는 디바이스 명령 스펙 질의이다(device).
	QueryActionCommands = "commands"
	// QueryActionMetadata 는 디바이스 메타데이터(읽기) 질의이다(device).
	QueryActionMetadata = "metadata"

	// --- M10 그룹 L: 대시보드 config + 메트릭 read query-action (READ-ONLY) ---

	// QueryActionGetShared 는 노드의 공유(global) 대시보드 config 질의이다
	// (dashboard, REQ-L01 — GET /dashboards/shared 매핑).
	QueryActionGetShared = "get_shared"
	// QueryActionGetMine 은 노드의 개인(user) 대시보드 config 질의이다
	// (dashboard, REQ-L01 — GET /dashboards/mine 매핑). args.owner 로 노드-로컬
	// 사용자를 지정한다(노드 권위 — A17).
	QueryActionGetMine = "get_mine"
	// QueryActionMetrics 는 노드의 시스템 메트릭 스냅샷 질의이다(monitor, REQ-L05 —
	// GET /monitor/metrics 매핑). 완만 변동이므로 단기 TTL 캐시 대상이다(REQ-J16).
	QueryActionMetrics = "metrics"
)

// M8 스트림 action 상수 (spec §5.10.2, REQ-J08). 스트림 가능한 라이브 action 만
// 열거한다. query-action 상수와 동일 문자열을 재사용한다(state/stats/series).
const (
	// StreamActionState 는 디바이스 실시간 상태 스트림이다(device.state).
	StreamActionState = QueryActionState
	// StreamActionStats 는 에이전트 라이브 통계 스트림이다(agent.stats).
	StreamActionStats = QueryActionStats
	// StreamActionSeries 는 에이전트 라이브 시리즈 스트림이다(agent.series).
	StreamActionSeries = QueryActionSeries

	// --- M10 그룹 L: 차트 + 로그 라이브 스트림 action (READ-ONLY, 캐시 우회) ---

	// StreamActionChart 는 노드의 차트 채널 라이브 스트림이다(chart.chart, REQ-L07).
	// args 는 {"channelName": "..."} 이며, 노드는 자신의 in-process 차트 채널 hub
	// (/ws/chart/{channel} 직결 대신)를 구독해 backfill/append 프레임을 중계한다.
	StreamActionChart = "chart"
	// StreamActionLogs 는 노드의 로그 라이브 스트림이다(monitor.logs, REQ-L06).
	// 노드는 자신의 로그 스트림 소스를 구독해 stream_data 로 push 한다(캐시 우회).
	StreamActionLogs = "logs"
)

// 원격 query/stream 도메인 상수 (M10 그룹 L, spec §5.1/§5.10 — dashboard/monitor/chart).
//
// 기존 command 도메인(DomainFlow/DomainAgent/DomainDevice)과 달리, 이들은 read 프록시
// 전용 도메인이다(READ-ONLY — REQ-J03). 변경 경로(그룹 D 명령)는 본 도메인을 사용하지
// 않는다(대시보드 config 편집·메트릭/로그/차트 mutation 은 v1.5 비목표 — REQ-L12).
const (
	// DomainDashboard 는 노드의 대시보드 config read 도메인이다(REQ-L01, READ-ONLY).
	DomainDashboard = "dashboard"
	// DomainMonitor 는 노드의 시스템 메트릭(query)·로그(stream) read 도메인이다(REQ-L05/L06).
	DomainMonitor = "monitor"
	// DomainChart 는 노드의 차트 채널 라이브 스트림 도메인이다(REQ-L07, stream 전용).
	DomainChart = "chart"
)

// allowedQueryActions 는 도메인별 허용 read query-action 집합이다(REQ-J04 — FULL
// 커버리지). 본 맵에 없는 (domain, action) 조합은 거부된다(미열거·변경 의미 차단 —
// REQ-J03).
var allowedQueryActions = map[string]map[string]struct{}{
	DomainFlow: {
		QueryActionList:   {},
		QueryActionGet:    {},
		QueryActionStatus: {},
		QueryActionNodes:  {},
		QueryActionNode:   {},
		QueryActionLogs:   {},
	},
	DomainAgent: {
		QueryActionList:     {},
		QueryActionGet:      {},
		QueryActionStats:    {},
		QueryActionConfig:   {},
		QueryActionDevices:  {},
		QueryActionTopics:   {},
		QueryActionStore:    {},
		QueryActionSessions: {},
		QueryActionSeries:   {},
	},
	DomainDevice: {
		QueryActionList:     {},
		QueryActionGet:      {},
		QueryActionState:    {},
		QueryActionCommands: {},
		QueryActionMetadata: {},
	},
	// M10 그룹 L: 대시보드 config read(READ-ONLY — REQ-L01). put/delete 등 변경 의미
	// action 은 본 집합에 없으므로 거부된다(원격 config 편집 비목표 — REQ-J03/L12).
	DomainDashboard: {
		QueryActionGetShared: {},
		QueryActionGetMine:   {},
	},
	// M10 그룹 L: 시스템 메트릭 스냅샷 read(REQ-L05, 단기 TTL 캐시 대상). logs 는
	// 스트림 action 이므로 query allowlist 에 포함하지 않는다(REQ-L06).
	DomainMonitor: {
		QueryActionMetrics: {},
	},
}

// streamableActions 는 도메인별 스트림 가능한 라이브 action 집합이다(REQ-J08).
// 캐시 우회·서버 경유 중계 대상이다(REQ-J16). 그 외 action 은 스트림 불가(폴링 폴백).
var streamableActions = map[string]map[string]struct{}{
	DomainDevice: {
		StreamActionState: {},
	},
	DomainAgent: {
		StreamActionStats:  {},
		StreamActionSeries: {},
	},
	// M10 그룹 L: 차트 채널 라이브 스트림(REQ-L07). args=channelName. 노드 in-process
	// 차트 hub 구독 → backfill/append 중계(별도 WS 경로 미신설). 캐시 우회(라이브).
	DomainChart: {
		StreamActionChart: {},
	},
	// M10 그룹 L: 로그 라이브 스트림(REQ-L06). 노드 로그 스트림 소스 구독 → tail push.
	// 캐시 우회(라이브). monitor.metrics 는 query-action 이므로 스트림 집합에 없다.
	DomainMonitor: {
		StreamActionLogs: {},
	},
}

// IsAllowedQueryAction 은 (domain, queryAction)이 허용된 read query-action 인지
// 반환한다(REQ-J04 allowlist). 미열거 도메인·미열거/변경 의미 action 은 false 이다
// (READ-ONLY 강제 — REQ-J03).
func IsAllowedQueryAction(domain, queryAction string) bool {
	actions, ok := allowedQueryActions[domain]
	if !ok {
		return false
	}
	_, ok = actions[queryAction]
	return ok
}

// IsStreamableAction 은 (domain, streamAction)이 스트림 가능한 라이브 action 인지
// 반환한다(REQ-J08). 비스트림 action 은 폴링 폴백 대상이다.
func IsStreamableAction(domain, streamAction string) bool {
	actions, ok := streamableActions[domain]
	if !ok {
		return false
	}
	_, ok = actions[streamAction]
	return ok
}

// 원격 명령 도메인 상수 (spec §5.1 command.domain, REQ-D02/D03/D04).
const (
	// DomainFlow 는 플로우 명령 도메인이다(FlowServiceAdapter 적용, REQ-D02).
	DomainFlow = "flow"
	// DomainAgent 는 에이전트 명령 도메인이다(AgentServiceAdapter 적용, REQ-D03).
	DomainAgent = "agent"
	// DomainDevice 는 IoT 디바이스 메타데이터 명령 도메인이다(device 서비스 적용, REQ-D04).
	DomainDevice = "device"
	// DomainSystem 은 노드 자체 운영 명령 도메인이다(버전 관리 Phase 2 — 자가 업데이트).
	// 자원(flow/agent/device)이 아닌 노드 프로세스 수준 동작을 다룬다.
	DomainSystem = "system"
)

// 원격 system 도메인 action 상수 (버전 관리 Phase 2).
const (
	// ActionSystemUpdate 는 노드 자가 업데이트 action 이다. args 는 SystemUpdateArgs.
	ActionSystemUpdate = "update"
)

// SystemUpdateArgs 는 system/update 명령의 args 스키마이다(버전 관리 Phase 2).
type SystemUpdateArgs struct {
	// TargetVersion 은 적용할 목표 버전(vMAJOR.MINOR.PATCH)이다. 비면 채널 최신.
	TargetVersion string `json:"target_version,omitempty"`
	// Channel 은 릴리스 채널(stable/beta/nightly)이다. 비면 노드 기본 채널.
	Channel string `json:"channel,omitempty"`
	// UpdateURL 은 다운로드 소스(릴리스 API 베이스 URL) 오버라이드다. 관리 서버가
	// 저장한 소스를 주입한다. 비면 노드 로컬 설정(update.update_url)을 사용한다. 공개키는
	// 절대 전달하지 않는다 — 노드가 자기 로컬 공개키로 서명을 검증한다(무결성 보장).
	UpdateURL string `json:"update_url,omitempty"`
	// Restart 가 true 면 바이너리 교체 성공 후 결과 전송 후 graceful 재시작을 수행한다.
	// 기본 false: 교체만 하고 restart_required=true 를 반환한다(운영 측 재시작 위임).
	Restart bool `json:"restart,omitempty"`
}

// SystemUpdateResult 는 system/update 명령의 결과 스키마이다.
type SystemUpdateResult struct {
	NewVersion      string `json:"new_version"`
	BackupPath      string `json:"backup_path,omitempty"`
	AppliedAtMs     int64  `json:"applied_at_ms"`
	RestartRequired bool   `json:"restart_required"`
	Restarting      bool   `json:"restarting"`
}

// 원격 명령 action 상수 (spec §5.1 command.action, v1.2 그룹 I — REQ-I01~I06).
//
// flow/agent 도메인의 FULL CRUD 편집을 위한 action 의미를 고정한다. 기존 command/
// command_result 메시지를 그대로 재사용하며(신규 메시지 타입 없음), 클라이언트는 각
// action 을 해당 어댑터 메서드(FlowServiceAdapter/AgentServiceAdapter 의 Create/Update/
// Delete)로 라우팅한다(REQ-I06, A5 — 로컬 API 와 동일 검증).
//
// 시크릿 생략 라운드트립 규약(REQ-I07, spec §5.9 OPEN QUESTION 6 RESOLVED —
// "필드 부재 + 노드 backfill"):
//
//   - ActionCreate: args 는 신규 자원 정의 JSON 이다. 노드 어댑터 Create 가 ID 를
//     부여·반환하고(node-assigned ID — OPEN QUESTION 8), 서버는 반환된 ID 를 응답에
//     사용한다. 신규 자원은 자동 노출되지 않는다(opt-in 보존 — OPEN QUESTION 9).
//   - ActionUpdate: args 는 {"id": "...", ...갱신 정의 JSON} 이다. 갱신 정의에서
//     마스킹/미변경 시크릿 필드는 와이어에서 완전히 생략된다(sentinel/자리표시자 금지).
//     노드는 어댑터 호출 전 기존 자원 정의를 로드하여 부재한 시크릿 필드를 기존값으로
//     backfill 한 뒤 적용한다(병합은 client/apply 측 책임 — secret_fields SoT 재사용).
//   - ActionDelete: args 는 {"id": "..."} 이다. 노드 어댑터 Delete 를 호출한다.
//
// 본 상수 값은 기존 cmd/xflowd 어댑터 라우팅의 문자열 리터럴("create"/"update"/
// "delete")과 동일하므로 와이어 호환을 유지한다.
const (
	// ActionCreate 는 flow/agent 신규 생성 action 이다(REQ-I01/I04, node-assigned ID).
	ActionCreate = "create"
	// ActionUpdate 는 flow/agent 기존 자원 수정 action 이다(REQ-I02/I04, 시크릿 backfill).
	ActionUpdate = "update"
	// ActionDelete 는 flow/agent 자원 삭제 action 이다(REQ-I03/I04).
	ActionDelete = "delete"
)

// 인벤토리 델타 연산 상수 (spec §5.1 inventory_delta.op, REQ-E02).
const (
	// OpAdd 는 노출 자원 추가이다(REQ-E02).
	OpAdd = "add"
	// OpUpdate 는 노출 자원 변경이다(REQ-E02).
	OpUpdate = "update"
	// OpRemove 는 노출 자원 제거(또는 노출 해제)이다(REQ-E02/A07).
	OpRemove = "remove"
)

// 인벤토리 자원 종류 상수. command 도메인 상수와 동일 문자열을 재사용한다
// (DomainFlow/DomainAgent/DomainDevice == "flow"/"agent"/"device").
const (
	// KindFlow 는 플로우 자원 종류이다.
	KindFlow = DomainFlow
	// KindAgent 는 에이전트 자원 종류이다.
	KindAgent = DomainAgent
	// KindDevice 는 IoT 디바이스 자원 종류이다.
	KindDevice = DomainDevice
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
//
// v1.4(M9, 그룹 K): OS/Arch/StartedAt 는 선택적 BASIC 시스템 정보이다(REQ-K07). 토큰
// 보유 재접속(hello) 경로에서도 시스템 정보를 갱신할 수 있도록 운반한다. 미존재(구버전
// 노드)는 빈값으로 처리된다(하위 호환 — REQ-K09). 자원 메트릭(CPU/메모리/디스크)은 제외.
type HelloPayload struct {
	InstanceID string `json:"instance_id"`
	Hostname   string `json:"hostname"`
	Version    string `json:"version"`
	OS         string `json:"os,omitempty"`         // runtime.GOOS (REQ-K07)
	Arch       string `json:"arch,omitempty"`       // runtime.GOARCH (REQ-K07)
	StartedAt  int64  `json:"started_at,omitempty"` // 프로세스 시작 시각(epoch ms, REQ-K07)
	// DisplayWidth/DisplayHeight 는 노드 장비 모니터 해상도이다(px, v1.6 M11, REQ-M01).
	// 노드 config(display.resolution/width+height)에서 파생되며, 미설정(구버전/헤드리스)
	// 노드는 0 으로 생략된다(하위 호환 — REQ-M03).
	DisplayWidth  int `json:"display_width,omitempty"`  // 장비 화면 가로 px (REQ-M01)
	DisplayHeight int `json:"display_height,omitempty"` // 장비 화면 세로 px (REQ-M01)
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
//
// EnrollmentToken 은 선택적 가입 토큰이다(v1.1 그룹 H, REQ-REMOTE-H05). 설정 시
// 서버는 토큰을 검증하여 관리자 수동 승인 없이 노드를 자동 승인한다(유효한 경우).
// 시크릿이므로 로깅 대상이 아니다(REQ-F06/H06).
// v1.4(M9, 그룹 K): OS/Arch/StartedAt 는 선택적 BASIC 시스템 정보이다(REQ-K07). 노드는
// 최초 register 에 이를 채워 보고하고, 서버는 managed_nodes 에 저장한다(REQ-K08). 미존재
// (구버전 노드)는 빈값으로 처리되어 등록/관리가 정상 동작한다(하위 호환 — REQ-K09).
// 자원 메트릭(CPU/메모리/디스크)은 보고하지 않는다(본 마일스톤 제외).
type RegisterPayload struct {
	InstanceID      string          `json:"instance_id"`
	Hostname        string          `json:"hostname,omitempty"`
	Version         string          `json:"version,omitempty"`
	Exposure        ExposureSummary `json:"exposure,omitempty"`
	BootstrapSecret string          `json:"bootstrap_secret,omitempty"`
	EnrollmentToken string          `json:"enrollment_token,omitempty"`
	OS              string          `json:"os,omitempty"`         // runtime.GOOS (REQ-K07)
	Arch            string          `json:"arch,omitempty"`       // runtime.GOARCH (REQ-K07)
	StartedAt       int64           `json:"started_at,omitempty"` // 프로세스 시작 시각(epoch ms, REQ-K07)
	// DisplayWidth/DisplayHeight 는 노드 장비 모니터 해상도이다(px, v1.6 M11, REQ-M01).
	// 노드는 최초 register 에 config 파생 해상도를 운반하고 서버는 managed_nodes 에
	// 저장한다(REQ-M02). 미보고(구버전/미설정)는 0 으로 생략된다(하위 호환 — REQ-M03).
	DisplayWidth  int `json:"display_width,omitempty"`  // 장비 화면 가로 px (REQ-M01)
	DisplayHeight int `json:"display_height,omitempty"` // 장비 화면 세로 px (REQ-M01)
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

// CommandPayload 는 원격 명령 페이로드이다(server→client, REQ-D01/D07, spec §5.1).
//
// CommandID 로 요청-결과를 1:1 상관(correlation)한다(REQ-D07). TargetInstanceID 는
// 명령 대상 노드이며, 서버는 대상이 승인+온라인일 때만 디스패치한다(REQ-D01/D08).
// Domain(flow|agent|device) + Action 으로 클라이언트의 로컬 어댑터 메서드에
// 라우팅하며, Args 는 도메인/액션별 인코딩된 인자이다(REQ-D02/D03/D04).
//
// Args 는 시크릿(에이전트/디바이스 자격증명 등)을 포함할 수 있으므로 verbatim
// 로깅 금지이다(REQ-F06). 감사 로그는 domain/action/command_id 만 남긴다.
type CommandPayload struct {
	CommandID        string          `json:"command_id"`
	TargetInstanceID string          `json:"target_instance_id"`
	Domain           string          `json:"domain"`
	Action           string          `json:"action"`
	Args             json.RawMessage `json:"args,omitempty"`
}

// CommandResultPayload 는 명령 결과/ack 페이로드이다(client→server, REQ-D05/D09,
// spec §5.1).
//
// CommandID 로 원본 명령과 상관된다(REQ-D07). OK 가 true 이면 Result 에 적용 결과가
// 담기고, false 이면 Error 에 실패 사유가 담긴다. 적용 실패는 부분 적용 없이 보고
// 된다(REQ-D09).
type CommandResultPayload struct {
	CommandID string          `json:"command_id"`
	OK        bool            `json:"ok"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// InventoryItem 은 미러링되는 단일 자원(플로우/에이전트/디바이스)의 중립 표현이다
// (REQ-E01/E02/E04, spec §5.1/§5.4).
//
// Definition 은 자원 정의/설정의 JSON 이며, 노드를 떠나기 전 반드시 redaction
// 정책(F06)으로 시크릿이 제거된 상태여야 한다. remote 패키지는 redaction 을 직접
// 수행하지 않고(handler 패키지 import cycle 회피), cmd/xflowd 의 InventorySource
// 어댑터가 redaction 을 적용한 Definition 을 채운다(spec §5.5 어댑터 브리지).
//
// UpdatedAt 은 epoch milliseconds(int64) 이다(프로젝트 규약).
type InventoryItem struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Kind       string          `json:"kind"` // flow | agent | device
	Status     string          `json:"status,omitempty"`
	Definition json.RawMessage `json:"definition,omitempty"` // redacted (F06)
	UpdatedAt  int64           `json:"updated_at,omitempty"` // epoch ms
}

// InventorySnapshotPayload 는 접속 시 전체 인벤토리 스냅샷이다(client→server,
// REQ-E01, spec §5.1).
//
// 노출 범위(REQ-E07/A04)로 필터되고 redaction(F06)된 자원만 포함된다. 서버는
// 수신 시 해당 노드의 미러 행을 종류별로 교체한다(REQ-E03).
type InventorySnapshotPayload struct {
	InstanceID string          `json:"instance_id"`
	Flows      []InventoryItem `json:"flows,omitempty"`
	Agents     []InventoryItem `json:"agents,omitempty"`
	Devices    []InventoryItem `json:"devices,omitempty"`
}

// InventoryDeltaPayload 는 노출 자원 변경 델타이다(client→server, REQ-E02, spec §5.1).
//
// Op 는 add|update|remove 중 하나이며, Kind 는 flow|agent|device 이다. remove 의
// 경우 Item 은 ID 만 채워질 수 있다(정의 불필요). 노출 해제(A07)도 remove 로 신호한다.
type InventoryDeltaPayload struct {
	InstanceID string        `json:"instance_id"`
	Op         string        `json:"op"`   // add | update | remove
	Kind       string        `json:"kind"` // flow | agent | device
	Item       InventoryItem `json:"item"`
}

// HeartbeatPayload 는 heartbeat 페이로드이다(REQ-B03).
// TS 는 epoch milliseconds(int64) 이다.
//
// v1.4(M9, 그룹 K): OS/Arch/Version/StartedAt 는 선택적 BASIC 시스템 정보 갱신이다
// (REQ-K07). 주기 heartbeat 로 변경 가능 필드(version/started_at 재기동 반영)를 갱신
// 한다. 미존재(구버전 노드)는 빈값으로 처리되며 서버는 기존값을 보존한다(REQ-K08/K09).
type HeartbeatPayload struct {
	InstanceID string `json:"instance_id"`
	TS         int64  `json:"ts"`
	OS         string `json:"os,omitempty"`         // runtime.GOOS (REQ-K07)
	Arch       string `json:"arch,omitempty"`       // runtime.GOARCH (REQ-K07)
	Version    string `json:"version,omitempty"`    // 노드 버전(REQ-K07)
	StartedAt  int64  `json:"started_at,omitempty"` // 프로세스 시작 시각(epoch ms, REQ-K07)
	// DisplayWidth/DisplayHeight 는 노드 장비 모니터 해상도 갱신이다(px, v1.6 M11,
	// REQ-M01). 운영자가 config 해상도를 변경·재기동하면 heartbeat 로 갱신된다. 미보고
	// (구버전 노드/생략)는 0 이며 서버는 기존값을 보존한다(REQ-M03 preserve-on-omit).
	DisplayWidth  int `json:"display_width,omitempty"`  // 장비 화면 가로 px (REQ-M01)
	DisplayHeight int `json:"display_height,omitempty"` // 장비 화면 세로 px (REQ-M01)
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

// NewHeartbeatMessageWithInfo 는 BASIC 시스템 정보 + 노드 해상도를 실은 heartbeat
// 메시지를 생성한다(v1.4 M9 / v1.6 M11, REQ-K07/M01). ts 는 현재 시각의 epoch ms 이고,
// os/arch/version/startedAt 은 노드가 런타임에서 수집한 값, displayWidth/displayHeight 는
// 노드 config 에서 파생한 장비 모니터 해상도이다(빈값/0 은 omitempty 로 와이어에서 생략 —
// 하위 호환, 서버는 미보고 필드를 보존한다 — REQ-K09/M03).
func NewHeartbeatMessageWithInfo(instanceID, osName, arch, version string, startedAtMs int64, displayWidth, displayHeight int) (*ws.Message, error) {
	return ws.NewMessage(TypeHeartbeat, HeartbeatPayload{
		InstanceID:    instanceID,
		TS:            time.Now().UnixMilli(),
		OS:            osName,
		Arch:          arch,
		Version:       version,
		StartedAt:     startedAtMs,
		DisplayWidth:  displayWidth,
		DisplayHeight: displayHeight,
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

// NewCommandMessage 는 CommandPayload 를 ws.Message 봉투로 인코딩한다(REQ-D01).
func NewCommandMessage(p CommandPayload) (*ws.Message, error) {
	return ws.NewMessage(TypeCommand, p)
}

// NewCommandResultMessage 는 CommandResultPayload 를 ws.Message 봉투로 인코딩한다
// (REQ-D05).
func NewCommandResultMessage(p CommandResultPayload) (*ws.Message, error) {
	return ws.NewMessage(TypeCommandResult, p)
}

// NewInventorySnapshotMessage 는 InventorySnapshotPayload 를 ws.Message 봉투로
// 인코딩한다(REQ-E01).
func NewInventorySnapshotMessage(p InventorySnapshotPayload) (*ws.Message, error) {
	return ws.NewMessage(TypeInventorySnapshot, p)
}

// NewInventoryDeltaMessage 는 InventoryDeltaPayload 를 ws.Message 봉투로
// 인코딩한다(REQ-E02).
func NewInventoryDeltaMessage(p InventoryDeltaPayload) (*ws.Message, error) {
	return ws.NewMessage(TypeInventoryDelta, p)
}

// --- M8 그룹 J: query/stream 페이로드 구조 + 생성자 (READ-ONLY 프록시) ---

// QueryPayload 는 per-domain query-action READ 질의 페이로드이다(server→client,
// REQ-J01/J02, spec §5.1/§5.10).
//
// QueryID 로 요청-응답을 1:1 상관(correlation)한다(REQ-J02 — D07 패턴 준용).
// TargetInstanceID 는 질의 대상 노드이며, 서버는 대상이 승인+온라인+노출 범위일 때만
// 전달한다(REQ-J05). Domain(flow|agent|device) + QueryAction(열거 allowlist —
// REQ-J04)으로 노드의 로컬 read 핸들러를 지정하고, Args 는 도메인/action 별 인자
// (예: {"id": "..."})이다.
//
// READ-ONLY(REQ-J03): QueryAction 은 read 의미만 허용된다(IsAllowedQueryAction 강제).
// 변경 의미 action 은 노드 측에서 거부된다.
type QueryPayload struct {
	QueryID          string          `json:"query_id"`
	TargetInstanceID string          `json:"target_instance_id,omitempty"`
	Domain           string          `json:"domain"`
	QueryAction      string          `json:"query_action"`
	Args             json.RawMessage `json:"args,omitempty"`
}

// QueryResultPayload 는 질의 응답/ack 페이로드이다(client→server, REQ-J02/J06/J07,
// spec §5.1).
//
// QueryID 로 원본 질의와 상관된다. OK 가 true 이면 Data 에 redacted JSON 본문이
// 담기고(REQ-J06 — 노드가 전송 전 마스킹), false 이면 Error 에 사유가 담긴다. 서버는
// 본 ok/error 를 502(node-error) 매핑에 사용한다(REQ-J07).
type QueryResultPayload struct {
	QueryID string          `json:"query_id"`
	OK      bool            `json:"ok"`
	Data    json.RawMessage `json:"data,omitempty"` // redacted (REQ-J06)
	Error   string          `json:"error,omitempty"`
}

// SubscribePayload 는 실시간 스트림 구독 시작 페이로드이다(server→client, REQ-J08,
// spec §5.10.2).
//
// SubscriptionID 로 다중 구독을 구분한다(REQ-J08b). Domain + StreamAction(스트림
// 가능 action — IsStreamableAction)으로 노드의 실시간 소스를 지정한다. Args 는 대상
// 식별 인자(예: {"id": "..."})이다. READ-ONLY(REQ-J03).
type SubscribePayload struct {
	SubscriptionID   string          `json:"subscription_id"`
	TargetInstanceID string          `json:"target_instance_id,omitempty"`
	Domain           string          `json:"domain"`
	StreamAction     string          `json:"stream_action"`
	Args             json.RawMessage `json:"args,omitempty"`
}

// StreamDataPayload 는 구독 소스 갱신 push 페이로드이다(client→server, REQ-J08).
//
// SubscriptionID 로 구독과 상관된다. Payload 는 redacted JSON 갱신 본문이다(REQ-J06).
// Error 가 비어 있지 않으면 구독 실패/종료를 의미하며(게이팅 위반·비스트림 action·
// 소스 오류), 서버는 해당 구독을 종료해야 한다(터미널 프레임).
type StreamDataPayload struct {
	SubscriptionID string          `json:"subscription_id"`
	Payload        json.RawMessage `json:"payload,omitempty"` // redacted (REQ-J06)
	Error          string          `json:"error,omitempty"`
}

// UnsubscribePayload 는 스트림 구독 해제·teardown 페이로드이다(both, REQ-J08b).
type UnsubscribePayload struct {
	SubscriptionID string `json:"subscription_id"`
}

// NewQueryMessage 는 QueryPayload 를 ws.Message 봉투로 인코딩한다(REQ-J01).
func NewQueryMessage(p QueryPayload) (*ws.Message, error) {
	return ws.NewMessage(TypeQuery, p)
}

// NewQueryResultMessage 는 QueryResultPayload 를 ws.Message 봉투로 인코딩한다(REQ-J02).
func NewQueryResultMessage(p QueryResultPayload) (*ws.Message, error) {
	return ws.NewMessage(TypeQueryResult, p)
}

// NewSubscribeMessage 는 SubscribePayload 를 ws.Message 봉투로 인코딩한다(REQ-J08).
func NewSubscribeMessage(p SubscribePayload) (*ws.Message, error) {
	return ws.NewMessage(TypeSubscribe, p)
}

// NewStreamDataMessage 는 StreamDataPayload 를 ws.Message 봉투로 인코딩한다(REQ-J08).
func NewStreamDataMessage(p StreamDataPayload) (*ws.Message, error) {
	return ws.NewMessage(TypeStreamData, p)
}

// NewUnsubscribeMessage 는 UnsubscribePayload 를 ws.Message 봉투로 인코딩한다(REQ-J08b).
func NewUnsubscribeMessage(p UnsubscribePayload) (*ws.Message, error) {
	return ws.NewMessage(TypeUnsubscribe, p)
}

// DecodeMessage 는 ws.DecodeMessage 의 패키지-로컬 별칭이다(테스트/호출 편의).
func DecodeMessage(data []byte) (*ws.Message, error) {
	return ws.DecodeMessage(data)
}
