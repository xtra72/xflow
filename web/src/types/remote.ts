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
  /**
   * 단일 그룹 라벨 (v1.4 M9, 그룹 K, REQ-K01/K04). 빈 문자열/미지정은 가상
   * "전체"(All) 버킷을 의미한다. 구버전 백엔드 응답에는 없을 수 있으므로 선택적.
   */
  group_name?: string;
  /** 마지막 수신 시각 (epoch ms, 0 = 미수신). */
  last_seen: number;
  /**
   * 관리자 지정 목표 버전 대비 구버전 여부 (버전 관리 Phase 1). 목표 버전 미설정/
   * 비-semver 면 false. 구버전 백엔드 응답에는 없을 수 있으므로 선택적.
   */
  outdated?: boolean;
}

/** 서버 전역 목표 버전 (버전 관리 Phase 1). GET/PUT /remote/target-version */
export interface TargetVersion {
  /** 목표 버전 문자열 (vMAJOR.MINOR.PATCH). 빈 문자열 = 미설정/해제. */
  version: string;
}

/**
 * 서버 저장 업데이트 소스 (GitHub/자체 호스팅). GET/PUT /remote/update-source
 * 원격 업데이트 명령에 자동 주입된다. 공개키는 노드 로컬 신뢰 앵커이므로 서버가 저장/전달하지
 * 않는다(무결성은 각 노드가 자기 로컬 공개키로 서명 검증).
 */
export interface UpdateSource {
  /** 릴리스 API 베이스 URL. 빈 문자열 = 미설정(노드 로컬 설정으로 폴백). https:// 필수. */
  update_url: string;
  /** 채널(stable/beta/nightly). 빈 문자열 = 노드 기본. */
  channel?: string;
}

// ---- 릴리스 저장소 (관리 서버 호스팅 프로그램 이미지) ----
//
// 관리 서버가 아키텍처별 `xflowd` 바이너리 + Ed25519 서명을 저장하는 릴리스 저장소이다.
// 노드는 런타임 GOOS/GOARCH 를 보고하므로(RPi armv6/armv7 은 모두 arm), UI 는
// linux/amd64 · linux/arm64 · linux/arm · darwin/amd64 · darwin/arm64 5개 슬롯을
// 행렬로 표시하고 업로드 여부를 표시한다. 업데이트 소스를 이 서버로 지정하면 각 노드가
// 자신의 아키텍처에 맞는 바이너리를 자동 다운로드한다.
//
// Go DTO 매핑: internal/api/handler/remote_admin.go (ReleaseRecord/ReleaseAsset).
// 모든 epoch 시각은 밀리초(int64 UnixMilli)이다.

/**
 * 릴리스 자산 한 개(아키텍처별 바이너리 + 서명 메타데이터).
 * Go `ReleaseAsset` 와 1:1 매핑된다.
 */
export interface ReleaseAsset {
  /** 운영체제 (GOOS). 예: linux, darwin. */
  os: string;
  /** 아키텍처 (GOARCH). 예: amd64, arm64, arm. */
  arch: string;
  /** 저장된 바이너리 파일명. */
  filename: string;
  /** 바이너리 크기 (바이트). */
  size: number;
  /** 바이너리 SHA-256 해시 (hex 문자열). */
  sha256: string;
  /** Ed25519 서명(.sig) 동반 여부. */
  has_sig: boolean;
  /** 업로드 시각 (epoch ms). */
  uploaded_at: number;
}

/**
 * 릴리스 버전 한 개(버전 + 채널 + 노트 + 아키텍처별 자산 목록).
 * Go `ReleaseRecord` 와 1:1 매핑된다.
 */
export interface ReleaseRecord {
  /** semver 버전 문자열 (vMAJOR.MINOR.PATCH). */
  version: string;
  /** 릴리스 채널 (stable/beta/nightly). */
  channel: string;
  /** 릴리스 노트(자유 텍스트). */
  notes: string;
  /** 게시 시각 (epoch ms). */
  published_at: number;
  /** 아키텍처별 업로드된 자산 목록. */
  assets: ReleaseAsset[];
}

/** 릴리스 버전 생성/갱신 요청. POST /remote/releases */
export interface ReleaseCreateRequest {
  /** semver 버전 문자열 (vMAJOR.MINOR.PATCH). */
  version: string;
  /** 릴리스 채널 (stable/beta/nightly). 미지정 시 서버 기본(stable). */
  channel?: string;
  /** 릴리스 노트(자유 텍스트). */
  notes?: string;
}

/** 노드 버전 변경 이력 한 줄 (버전 관리 Phase 1). */
export interface NodeVersionHistoryEntry {
  /** 변경 후 버전 문자열. */
  version: string;
  /** 변경 감지 시각 (epoch ms). */
  changed_at: number;
}

/** 그룹 일괄 명령/업데이트의 노드별 결과 (그룹 관리). */
export interface GroupDispatchResult {
  instance_id: string;
  ok: boolean;
  result?: unknown;
  error?: string;
}

/** 노드 원격 업데이트 요청 (버전 관리 Phase 2). POST /remote/nodes/{id}/update */
export interface NodeUpdateRequest {
  /** 목표 버전 (vMAJOR.MINOR.PATCH). 빈 값 = 채널 최신. */
  version?: string;
  /** 릴리스 채널 (stable/beta/nightly). 빈 값 = 노드 기본. */
  channel?: string;
  /** true 면 바이너리 교체 후 노드 graceful 재시작. */
  restart?: boolean;
}

// ---- 노드 그룹핑 + 시스템 정보 + 운영 요약 (v1.4 M9, 그룹 K, REQ-K01~K10) ----

/**
 * distinct 그룹 + 노드 수 (REQ-K03).
 * Go `NodeGroupDTO` (internal/api/handler/remote_grouping.go) 와 1:1 매핑된다.
 *
 * `group_name === ''` 은 그룹 미지정 노드를 묶는 가상 "전체"(All) 버킷이다
 * (예약 라벨 비영속 — OQ-K5). UI 는 빈 라벨을 "전체"로 표시한다.
 */
export interface NodeGroup {
  /** 그룹 라벨 (빈 문자열 = "전체" 가상 버킷). */
  group_name: string;
  /** 해당 그룹에 속한 노드 수. */
  node_count: number;
}

/**
 * 노드별 운영 요약 (미러 파생 — REQ-K10).
 * Go `NodeSummaryDTO` 와 1:1 매핑된다. 오프라인 시에도 last-known 으로 제공된다.
 */
export interface NodeOperationalSummary {
  /** 플로우 요약 (카운트 + running/stopped 분해). */
  flows: { total: number; running: number; stopped: number };
  /** 에이전트 요약 (카운트 + connected 분해). */
  agents: { total: number; connected: number };
  /** 디바이스 요약 (카운트 + online 분해). */
  devices: { total: number; online: number };
}

/**
 * 노드 상세 응답 (메타 + BASIC 시스템 정보 + uptime + 운영 요약, REQ-K08/K10).
 * Go `NodeDetailDTO` (internal/api/handler/remote_grouping.go) 와 1:1 매핑된다.
 *
 * - `uptime` 은 `started_at > 0` 일 때만 채워지며(서버 파생 = now − started_at),
 *   미보고(구버전) 노드는 `uptime: null` + `started_at: 0` 으로 표현된다. UI 는
 *   이 경우 uptime 을 "미보고"로 표시한다(하위 호환 — REQ-K09).
 * - 시크릿(토큰 식별자 등)은 포함되지 않는다(REQ-F06).
 */
export interface NodeDetail {
  /** 노드 인스턴스 식별자. */
  instance_id: string;
  /** 호스트명. */
  hostname: string;
  /** xflowd 버전 문자열. */
  version: string;
  /** 등록 상태. */
  status: string;
  /** 라이브 연결 상태. */
  online: boolean;
  /** 단일 그룹 라벨 (빈 문자열 = "전체"). */
  group_name: string;
  /** OS (runtime.GOOS). 미보고 시 빈 문자열. */
  os: string;
  /** 아키텍처 (runtime.GOARCH). 미보고 시 빈 문자열. */
  arch: string;
  /** 프로세스 시작 시각 (epoch ms, 0 = 미보고). */
  started_at: number;
  /** uptime (ms). started_at > 0 일 때만, 미보고 시 null. */
  uptime: number | null;
  /** 마지막 수신 시각 (epoch ms). */
  last_seen: number;
  /**
   * EFFECTIVE 가로 해상도 (px, v1.6 M11/M12, REQ-M01/M02). 관리자 오버라이드가
   * 설정되어 있으면 그 값, 아니면 노드 보고값이다(둘 다 없으면 0 = 미보고/미설정).
   * 고정 캔버스(FixedCanvasScaler)는 이 EFFECTIVE 값을 사용하므로, 오버라이드를
   * 설정/해제하면 노드-상세 쿼리 무효화 → 캔버스가 새 해상도로 재렌더된다(REQ-M03).
   */
  display_width: number;
  /**
   * EFFECTIVE 세로 해상도 (px, v1.6 M11/M12). 0 = 미보고/미설정.
   */
  display_height: number;
  /**
   * 관리자 오버라이드 가로 해상도 (px, v1.6 M12). 0 = 오버라이드 없음.
   * 설정되면 EFFECTIVE 해상도의 출처가 "오버라이드"가 된다.
   */
  display_override_width: number;
  /**
   * 관리자 오버라이드 세로 해상도 (px, v1.6 M12). 0 = 오버라이드 없음.
   */
  display_override_height: number;
  /**
   * 노드가 보고한 가로 해상도 (px, v1.6 M12). 0 = 미보고.
   * 오버라이드가 없을 때 EFFECTIVE 해상도의 출처가 "노드 보고"가 된다.
   */
  display_reported_width: number;
  /**
   * 노드가 보고한 세로 해상도 (px, v1.6 M12). 0 = 미보고.
   */
  display_reported_height: number;
  /** 운영 요약 (미러 파생). */
  summary: NodeOperationalSummary;
}

/**
 * 노드 그룹 배정 요청 본문 (REQ-K02). PUT /remote/nodes/{id}/group.
 */
export interface SetNodeGroupRequest {
  /** 배정할 그룹 라벨. 빈 문자열은 해제("전체" 환원)와 동일하다(REQ-K05). */
  group_name: string;
}

/**
 * 노드 디스플레이 해상도 오버라이드 설정 요청 본문 (v1.6 M12).
 * PUT /remote/nodes/{instance_id}/display.
 *
 * width/height 는 양의 정수여야 하며, 비양수 값은 백엔드가 400 으로 거부한다.
 * 설정 시 EFFECTIVE 해상도가 이 값으로 고정되어 고정 캔버스가 새 크기로 재렌더된다.
 * 노드 config 편집/재시작 없이 서버 메타데이터만 갱신한다(노드로 명령 전파 없음).
 */
export interface SetNodeDisplayRequest {
  /** 오버라이드 가로 해상도 (px, 양의 정수). */
  width: number;
  /** 오버라이드 세로 해상도 (px, 양의 정수). */
  height: number;
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
