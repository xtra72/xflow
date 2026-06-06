// Remote management types matching Go DTOs in
// internal/api/handler/remote_admin.go (SPEC-REMOTE-001 M5).
//
// 모든 epoch 시각은 밀리초(int64 UnixMilli)이다 (프로젝트 timestamp 규약).

/**
 * 관리 노드 등록 상태 머신.
 * Go 핸들러는 문자열로 응답하며 (`status` 필드), 다음 4개 값을 가진다.
 *   - pending  : 등록 요청 후 승인 대기
 *   - approved : 승인되어 명령/미러 대상
 *   - rejected : 거부됨
 *   - revoked  : 승인 후 폐기됨
 */
export type RegistrationStatus = 'pending' | 'approved' | 'rejected' | 'revoked';

/**
 * 알려진 등록 상태 목록 (필터/검증용).
 */
export const REGISTRATION_STATUSES: readonly RegistrationStatus[] = [
  'pending',
  'approved',
  'rejected',
  'revoked',
] as const;

/**
 * 미러 자원 종류.
 * 노드별/통합 미러 행은 flow/agent/device 중 하나를 나타낸다.
 */
export type MirroredResourceKind = 'flow' | 'agent' | 'device';

/**
 * 인스턴스의 원격 관리 동작 모드.
 * Go `remote_management.mode` 설정과 1:1 매핑된다.
 *   - server   : 다른 노드를 관리하는 서버 (admin `/remote/*` 엔드포인트 활성)
 *   - client   : 서버에 등록되는 피관리 노드
 *   - disabled : 원격 관리 비활성
 *
 * server 모드가 아니면 `/remote/nodes` 등 admin 엔드포인트는 404 를 반환하므로,
 * UI 는 이 값을 보고 해당 쿼리 발행을 차단한다.
 */
export type RemoteMode = 'server' | 'client' | 'disabled';

/**
 * `GET /remote/mode` 응답 표현.
 * 표준 인증이 적용되며 모든 모드에서 사용 가능하다.
 */
export interface RemoteModeResponse {
  mode: RemoteMode;
}

/**
 * 관리 노드 응답 표현.
 * Go `ManagedNodeDTO` 와 1:1 매핑된다 (시크릿 토큰은 노출하지 않음 — REQ-F06).
 */
export interface ManagedNode {
  /** 노드 인스턴스 식별자 (글로벌 유일). */
  instance_id: string;
  /** 호스트명. */
  hostname: string;
  /** xflowd 버전 문자열. */
  version: string;
  /** 등록 상태 (문자열). 알 수 없는 값일 수 있으므로 string 으로 받는다. */
  status: string;
  /** 출처 노드의 현재 라이브 연결 상태. */
  online: boolean;
  /** 마지막 수신 시각 (epoch ms, 0 = 미수신). */
  last_seen: number;
}

/**
 * 미러 자원 목록 응답 표현 (M4, REQ-E04/E05/E06).
 * Go `MirroredResourceDTO` 와 1:1 매핑된다.
 *
 * `source_instance_id` 로 출처 노드를 태깅하고 (REQ-E04/E05),
 * `online` 으로 출처 노드의 라이브 상태를 표시한다
 * (online=false 는 last-known/offline 표식 — REQ-E06).
 * `definition` 은 노드가 redaction(F06)한 정의이므로 시크릿이 없다.
 */
export interface MirroredResource {
  /** 미러 자원 식별자 (출처 노드 스코프 내 유일). */
  id: string;
  /** 출처 노드 인스턴스 식별자 (자원이 속한 노드). */
  source_instance_id: string;
  /** 자원 이름. */
  name: string;
  /** 자원 종류 (flow/agent/device). 알 수 없는 값일 수 있으므로 string. */
  kind: string;
  /** 자원 상태 (running/stopped 등). 선택적. */
  status?: string;
  /** redaction 된 정의 (JSON 문자열). 선택적. */
  definition?: string;
  /** 마지막 갱신 시각 (epoch ms). */
  updated_at: number;
  /** 출처 노드 라이브 상태 (false = last-known/offline). */
  online: boolean;
}

/**
 * 노드 사전 등록(수동 등록) 요청 본문.
 * Go 핸들러 (POST /remote/nodes) 와 1:1 매핑된다.
 *
 * `instance_id` 는 필수이며 글로벌 유일해야 한다. `name` 은 선택적 표시명이다.
 * 성공 시 status="approved", online=false 인 신규 노드가 생성된다.
 */
export interface PreRegisterRequest {
  /** 사전 등록할 노드 인스턴스 식별자 (필수, 글로벌 유일). */
  instance_id: string;
  /** 노드 표시명 (선택적). */
  name?: string;
}

/**
 * Enrollment 토큰 메타데이터.
 * Go 핸들러 (GET /remote/enrollment-tokens) 응답과 1:1 매핑된다.
 *
 * 보안상 raw 토큰 값(`token`)은 절대 포함하지 않는다 — 발급(POST) 응답에서만
 * 1회 노출된다 (`EnrollmentTokenCreated` 참조).
 */
export interface EnrollmentToken {
  /** 토큰 식별자. */
  id: string;
  /** 토큰 라벨 (선택적). */
  label?: string;
  /** 생성 시각 (epoch ms). */
  created_at: number;
  /** 만료 시각 (epoch ms). 미지정 시 만료 없음. */
  expires_at?: number;
  /** 최대 사용 횟수. 미지정 시 무제한. */
  max_uses?: number;
  /** 현재까지 사용된 횟수. */
  uses: number;
  /** 폐기 여부. */
  revoked: boolean;
}

/**
 * Enrollment 토큰 발급 요청 본문.
 * Go 핸들러 (POST /remote/enrollment-tokens) 와 1:1 매핑된다.
 *
 * `expires_in` 은 Go duration 문자열이다 (예: "24h", "168h"). 모두 선택적이다.
 */
export interface EnrollmentTokenCreateRequest {
  /** 토큰 라벨 (선택적). */
  label?: string;
  /** 만료 기간 (Go duration 문자열, 예: "24h"). 빈 값/미지정 시 만료 없음. */
  expires_in?: string;
  /** 최대 사용 횟수 (선택적). 미지정 시 무제한. */
  max_uses?: number;
}

/**
 * Enrollment 토큰 발급 결과.
 * Go 핸들러 (POST /remote/enrollment-tokens) 의 201 응답과 1:1 매핑된다.
 *
 * `token` 은 raw 토큰으로 발급 시 1회만 노출된다 (목록 조회에는 포함되지 않음).
 * UI 는 이 값을 복사 가능한 필드로 1회 표시한 뒤 다시 보여주지 않는다.
 */
export interface EnrollmentTokenCreated {
  /** 토큰 식별자. */
  id: string;
  /** raw 토큰 값 (1회만 노출). */
  token: string;
  /** 토큰 라벨 (선택적). */
  label?: string;
  /** 만료 시각 (epoch ms, 선택적). */
  expires_at?: number;
  /** 최대 사용 횟수 (선택적). */
  max_uses?: number;
}

/**
 * 원격 명령 발행 요청 본문.
 * Go `commandRequest` 와 1:1 매핑된다 (POST /remote/nodes/{id}/command).
 */
export interface CommandRequest {
  /** 명령 도메인 (flow/agent/device 등). */
  domain: string;
  /** 명령 액션 (start/stop/deploy 등). */
  action: string;
  /** 명령 인자 (도메인/액션별 임의 페이로드). 선택적. */
  args?: Record<string, unknown>;
}

/**
 * 원격 명령 발행 결과.
 * Go Command 핸들러의 성공 응답 (`data`) 표현이다.
 */
export interface CommandResult {
  /** 명령이 적용된 대상 노드. */
  instance_id: string;
  /** 발행한 도메인. */
  domain: string;
  /** 발행한 액션. */
  action: string;
  /** 노드가 반환한 결과 (도메인/액션별 임의 페이로드). null 가능. */
  result: unknown;
}

// ---- 원격 자원 편집 (M7, 그룹 I, REQ-I01~I06/I08~I11) ----

/**
 * 노드 어댑터가 반환하는 자원 결과 표현.
 * Go `nodeResult` (internal/api/handler/remote_editing.go) 와 1:1 매핑된다.
 *
 * create 응답에서 `id` 는 노드가 채번한 식별자이다(node-assigned — §5.9-8).
 * `config` 는 노드가 redaction(F06)한 정의이므로 시크릿이 없다.
 */
export interface RemoteResourceResult {
  /** 노드가 채번/확정한 자원 식별자. */
  id: string;
  /** 자원 이름. */
  name: string;
  /** 자원 상태 (running/stopped 등). */
  status: string;
  /** redaction 된 config/definition. 선택적. */
  config?: Record<string, unknown>;
}

/**
 * 원격 플로우 생성 요청 본문 (REQ-I01).
 * Go `dto.FlowCreateRequest` 와 1:1 매핑된다
 * (POST /remote/nodes/{instance_id}/flows → command{domain:flow, action:create}).
 */
export interface RemoteFlowCreateRequest {
  /** 플로우 이름 (필수). */
  name: string;
  /** 플로우 정의 (nodes/wires 등 JSON, 필수). */
  definition: Record<string, unknown>;
  /** 플로우 설명 (선택적). */
  description?: string;
}

/**
 * 원격 플로우 수정 요청 본문 (REQ-I02).
 * Go `dto.FlowUpdateRequest` 와 1:1 매핑된다
 * (PATCH /remote/nodes/{instance_id}/flows/{flow_id}).
 *
 * `definition` 의 마스킹/미변경 시크릿 필드는 호출자가 생략해야 하며(REQ-I07),
 * 노드가 기존값으로 backfill 한다. 마스킹 자리표시자를 그대로 전송하면 안 된다.
 */
export interface RemoteFlowUpdateRequest {
  /** 갱신 플로우 정의 (시크릿 생략됨). */
  definition: Record<string, unknown>;
  /** 플로우 이름 (선택적). */
  name?: string;
  /** 플로우 설명 (선택적). */
  description?: string;
}

/**
 * 원격 에이전트 생성 요청 본문 (REQ-I04).
 * Go `dto.AgentCreateRequest` 와 1:1 매핑된다
 * (POST /remote/nodes/{instance_id}/agents → command{domain:agent, action:create}).
 */
export interface RemoteAgentCreateRequest {
  /** 에이전트 이름 (필수). */
  name: string;
  /** 에이전트 종류 (필수, 예: mqtt/socket/hvac). */
  type: string;
  /** 에이전트 설정 (시크릿 포함 가능, 선택적). */
  config?: Record<string, unknown>;
}

/**
 * 원격 에이전트 수정 요청 본문 (REQ-I04).
 * Go `dto.AgentUpdateRequest` 와 1:1 매핑된다
 * (PATCH /remote/nodes/{instance_id}/agents/{agent_id}).
 *
 * `config` 의 마스킹/미변경 시크릿 필드는 호출자가 생략해야 하며(REQ-I07),
 * 노드가 기존값으로 backfill 한다.
 */
export interface RemoteAgentUpdateRequest {
  /** 갱신 에이전트 설정 (시크릿 생략됨). 선택적. */
  config?: Record<string, unknown>;
  /** 에이전트 이름 (선택적). */
  name?: string;
  /** 로그 레벨 (선택적, debug/info/warn/error). */
  log_level?: string;
}
