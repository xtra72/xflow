// SPEC-WEB-006 v0.1.0 (M2, M3, M5, M6, M7) — System Update API 클라이언트.
// SPEC-UPDATE-002 v0.1.0 (M6, M7, M8, M9, M10, M14) — Channel REST API + Multi-Binary + Auto-Restart 추가.
//
// SPEC-UPDATE-001 v0.1.0 의 5 REST endpoint + SPEC-UPDATE-002 v0.1.0 의 채널
// 조회/변경 endpoint 2종을 소비하는 pure-API 함수와 React Query 훅을 제공한다.
//
// 엔드포인트 (envelope 는 client 인터셉터가 풀어준다):
//   GET    /api/v1/system/version           — 현재 + 채널 최신 버전
//   POST   /api/v1/system/update/check      — 채널 즉시 폴링
//   POST   /api/v1/system/update/apply      — 업데이트 시작 (operation_id 발급)
//   POST   /api/v1/system/update/rollback   — 백업 바이너리로 복원
//   GET    /api/v1/system/update/status     — 활성 작업 상태 (1s 폴링용)
//   GET    /api/v1/system/update/channel    — 현재 + 사용 가능 채널 목록 (M6)
//   PUT    /api/v1/system/update/channel    — 채널 변경 + 즉시 check (M7)
//
// SPEC-UPDATE-002 v0.1.0 변경점:
//   - OperationStatus: 9 → 11 state (restarting + health_checking 추가)
//   - ApplyRequest: auto_restart? + target? 신규 필드
//   - Target enum: xflowd | xflow-agent | xflow
//
// @spec SPEC-WEB-006 v0.1.0
// @spec SPEC-UPDATE-001 v0.1.0
// @spec SPEC-UPDATE-002 v0.1.0 (M6, M7, M8, M9, M10, M14)

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import { get, post, put } from './client';

// ─────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────

/**
 * 업데이트 채널 enum (SPEC-UPDATE-001 M1).
 *
 * - `stable`:  releases/latest (default)
 * - `beta`:    *-beta.* prerelease
 * - `nightly`: *-nightly.* prerelease
 */
export type Channel = 'stable' | 'beta' | 'nightly';

/**
 * 업데이트 작업의 11-state machine.
 *
 * v0.1.0 (SPEC-UPDATE-001 M6 / SPEC-WEB-006 M6) 의 9-state 에 v0.2.0
 * (SPEC-UPDATE-002 M-1) 가 추가한 in-process restart 흐름의 두 단계를 포함한다.
 *
 * Active operation:
 *   - 기존: `starting | checking | downloading | verifying | applying | ready_to_restart`.
 *   - 신규 (auto_restart=true): `restarting | health_checking`.
 *     - `restarting`: atomic replace 완료 → graceful drain → syscall.Exec 까지의 단계.
 *     - `health_checking`: 새 프로세스 부팅 후 자가 health probe 진행 중.
 *
 * Terminal: `completed | failed`. UI 는 terminal 상태에서 status 폴링을 중단한다.
 *
 * v0.1.0 backward compat (Scenario 14): `auto_restart` 미지정 → 기존 흐름
 * (`ready_to_restart` 종료) 을 유지하며 `restarting` / `health_checking` 단계는
 * 발생하지 않는다.
 *
 * @spec SPEC-UPDATE-002 v0.1.0 (M-1, M14)
 */
export type OperationStatus =
  | 'idle'
  | 'starting'
  | 'checking'
  | 'downloading'
  | 'verifying'
  | 'applying'
  | 'ready_to_restart'
  | 'restarting'
  | 'health_checking'
  | 'completed'
  | 'failed';

/**
 * 업데이트 대상 바이너리 enum (SPEC-UPDATE-002 M9, M10).
 *
 * - `xflowd`:      메인 데몬 (default, in-process restart 권장).
 * - `xflow-agent`: 에이전트 런타임 (별도 프로세스, supervisor/syscall.Exec 모두 지원).
 * - `xflow`:       CLI 도구 (재시작 불필요, atomic replace 후 즉시 completed).
 *
 * 백엔드 whitelist 검증 대상이며 그 외 값은 400 거부.
 *
 * @spec SPEC-UPDATE-002 v0.1.0 (M9, M10)
 */
export type Target = 'xflowd' | 'xflow-agent' | 'xflow';

/**
 * `GET /api/v1/system/version` 응답 (envelope 풀린 후).
 *
 * SPEC-UPDATE-001 v0.1.0 백엔드는 빌드 시점 정보(version/commit/build_date/go_version)
 * 와 채널 최신 버전 정보(latest_version/update_available)를 함께 내려준다.
 */
export interface VersionInfo {
  version: string;
  commit: string;
  build_date: string;
  go_version: string;
  channel: Channel;
  update_available: boolean;
  latest_version: string | null;
}

/**
 * `POST /api/v1/system/update/check` 응답.
 */
export interface CheckResult {
  current: string;
  latest: string;
  available: boolean;
  channel: Channel;
  release_url: string;
  published_at: string;
}

/**
 * `POST /api/v1/system/update/apply` 요청 본문.
 *
 * v0.1.0 필드:
 *   - `version` 미지정 → 백엔드가 채널 최신 버전 사용.
 *   - `force=true`     → 다운그레이드 허용 (M8 anti-downgrade 우회).
 *
 * v0.2.0 신규 필드 (SPEC-UPDATE-002 M-1, M9, M14):
 *   - `auto_restart=true` → 적용 후 graceful drain → syscall.Exec → 자가 health
 *     check → 실패 시 자동 rollback. 미지정 시 v0.1.0 기본 흐름 (operator 가
 *     수동 재시작).
 *   - `target` 미지정    → "xflowd" 기본값. xflow-agent / xflow 는 multi-binary
 *     업데이트 대상.
 *
 * v0.1.0 backward compat: 신규 필드는 모두 optional 이며 미지정 시 v0.1.0 동작
 * 그대로 (Scenario 14).
 *
 * @spec SPEC-UPDATE-002 v0.1.0 (M-1, M9, M14)
 */
export interface ApplyRequest {
  version?: string;
  force?: boolean;
  /** in-process auto-restart 활성화 여부 (default: false). */
  auto_restart?: boolean;
  /** 업데이트 대상 바이너리 (default: "xflowd"). */
  target?: Target;
}

/**
 * `POST /api/v1/system/update/apply` 응답.
 *
 * 비동기 트리거이므로 백엔드는 즉시 `operation_id` 를 발급하고 작업은 백그라운드로
 * 진행된다. UI 는 이후 `useUpdateStatus({ enabled: true })` 로 상태를 폴링.
 */
export interface ApplyResponse {
  operation_id: string;
  status: OperationStatus;
  from_version: string;
  to_version: string;
}

/**
 * `GET /api/v1/system/update/status` 응답.
 *
 * Terminal 상태에서는 `completed_at` 또는 `error` 가 채워진다.
 */
export interface UpdateOperation {
  operation_id: string;
  status: OperationStatus;
  from_version: string;
  to_version: string;
  started_at: string;
  completed_at: string | null;
  error: string | null;
}

/**
 * `POST /api/v1/system/update/rollback` 응답.
 *
 * 성공 시 `from_version` (롤백 직전) → `to_version` (백업 바이너리 버전) 정보를
 * 운영자에게 제공한다.
 */
export interface RollbackResponse {
  from_version: string;
  to_version: string;
  rolled_back_at: string;
}

/**
 * `GET /api/v1/system/update/channel` 응답 (SPEC-UPDATE-002 M6).
 *
 * 현재 활성 채널과 시스템이 지원하는 모든 채널 enum 을 함께 반환한다.
 * Web UI 의 채널 변경 dropdown 에서 사용된다.
 */
export interface ChannelInfo {
  /** 현재 cfg 에 설정된 채널. */
  current: Channel;
  /** 시스템이 지원하는 모든 채널 enum (정렬: stable, beta, nightly). */
  available: Channel[];
}

/**
 * `PUT /api/v1/system/update/channel` 요청 본문 (SPEC-UPDATE-002 M7).
 */
export interface ChangeChannelRequest {
  /** 새로 적용할 채널. */
  channel: Channel;
}

/**
 * `PUT /api/v1/system/update/channel` 응답 (SPEC-UPDATE-002 M7).
 *
 * 채널 변경은 in-memory 만 적용된다. 영구 저장은 `xflowd update channel <name>`
 * CLI 또는 yaml 직접 수정이 필요하며, 응답의 `message` 필드로 운영자에게
 * 안내된다.
 *
 * 채널 변경 직후 새 채널로 즉시 Check 가 실행되며 결과는 `check_result` 에 포함.
 * 네트워크 에러 등으로 Check 실패 시 `check_result` 는 `null` 이며 채널 변경
 * 자체는 적용된다 (`previous != current`).
 */
export interface ChangeChannelResponse {
  /** 변경 전 채널. */
  previous: Channel;
  /** 변경 후 채널 (요청과 동일). */
  current: Channel;
  /** 새 채널로 즉시 실행한 Check 결과 (실패 시 null). */
  check_result: CheckResult | null;
  /** 영구 저장 안내 또는 check 실패 경고 메시지 (optional). */
  message?: string;
}

// ─────────────────────────────────────────────────────────────────────
// Pure API functions (envelope 인터셉터가 4xx/5xx → APIError 로 변환)
// ─────────────────────────────────────────────────────────────────────

/**
 * `GET /api/v1/system/version` 호출. 인증 필수 (401 → APIError).
 */
export async function fetchSystemVersion(): Promise<VersionInfo> {
  return get<VersionInfo>('/system/version');
}

/**
 * `POST /api/v1/system/update/check` 호출. 본문 없음.
 */
export async function postUpdateCheck(): Promise<CheckResult> {
  return post<CheckResult>('/system/update/check');
}

/**
 * `POST /api/v1/system/update/apply` 호출. 빈 객체 가능 (백엔드 기본값 적용).
 */
export async function postUpdateApply(req: ApplyRequest): Promise<ApplyResponse> {
  return post<ApplyResponse>('/system/update/apply', req);
}

/**
 * `POST /api/v1/system/update/rollback` 호출. 본문 없음.
 */
export async function postUpdateRollback(): Promise<RollbackResponse> {
  return post<RollbackResponse>('/system/update/rollback');
}

/**
 * `GET /api/v1/system/update/status` 호출. 활성 작업이 없으면 status='idle'.
 */
export async function fetchUpdateStatus(): Promise<UpdateOperation> {
  return get<UpdateOperation>('/system/update/status');
}

// ─────────────────────────────────────────────────────────────────────
// React Query hooks
// ─────────────────────────────────────────────────────────────────────

/**
 * 시스템 버전 정보 폴링 훅 (60초 간격).
 *
 * SPEC-WEB-006 M3:
 *   - `refetchInterval: 60_000` (60초) — 버전은 자주 바뀌지 않으므로 보수적 간격.
 *   - `staleTime: 30_000` — 1 사이클의 절반 동안 fresh 로 간주.
 *   - `refetchIntervalInBackground: false` — 다른 탭으로 전환 시 폴링 일시 중지
 *     (네트워크 부담 절감).
 */
export function useSystemVersion(): UseQueryResult<VersionInfo, Error> {
  return useQuery<VersionInfo, Error>({
    queryKey: ['system', 'version'],
    queryFn: fetchSystemVersion,
    refetchInterval: 60_000,
    staleTime: 30_000,
    refetchIntervalInBackground: false,
  });
}

/**
 * 명시적 "Check for updates" 트리거 훅 (mutation 형태).
 *
 * SPEC-WEB-006 M3 의 사용자 클릭 액션 — 자동 폴링과 별개로 즉시 채널을 폴링한다.
 */
export function useUpdateCheck(): UseMutationResult<CheckResult, Error, void> {
  return useMutation<CheckResult, Error, void>({
    mutationFn: postUpdateCheck,
  });
}

/**
 * 업데이트 적용 트리거 훅 (mutation).
 *
 * Variables 는 `ApplyRequest` (version/force optional). 응답의 `operation_id` 는
 * UI 가 status 폴링을 활성화할 때 사용된다.
 */
export function useUpdateApply(): UseMutationResult<
  ApplyResponse,
  Error,
  ApplyRequest
> {
  return useMutation<ApplyResponse, Error, ApplyRequest>({
    mutationFn: postUpdateApply,
  });
}

/**
 * 백업 바이너리로 롤백하는 트리거 훅 (mutation).
 */
export function useUpdateRollback(): UseMutationResult<
  RollbackResponse,
  Error,
  void
> {
  return useMutation<RollbackResponse, Error, void>({
    mutationFn: postUpdateRollback,
  });
}

/**
 * 활성 작업 상태 1초 폴링 훅 (Progress step 시각화 용).
 *
 * SPEC-WEB-006 M6:
 *   - `refetchInterval: 1_000` — 9-state machine 시각화에 충분히 매끄러운 간격.
 *   - `enabled` 옵션 — `operation_id` 가 있는 동안만 활성화.
 *
 * 호출자는 응답의 `status` 가 terminal (`completed | failed | idle`) 이 되면
 * `enabled: false` 로 토글해 폴링을 중단해야 한다.
 */
export function useUpdateStatus(opts: {
  enabled: boolean;
}): UseQueryResult<UpdateOperation, Error> {
  return useQuery<UpdateOperation, Error>({
    queryKey: ['system', 'update', 'status'],
    queryFn: fetchUpdateStatus,
    enabled: opts.enabled,
    refetchInterval: 1_000,
    staleTime: 0,
    refetchIntervalInBackground: false,
  });
}

// ─────────────────────────────────────────────────────────────────────
// SPEC-UPDATE-002 v0.1.0 (M6, M7) — Channel REST API
// ─────────────────────────────────────────────────────────────────────

/**
 * `GET /api/v1/system/update/channel` 호출 (SPEC-UPDATE-002 M6).
 *
 * 인증 필수 (401 → APIError). admin role 검증은 백엔드가 수행한다.
 */
export async function fetchChannelInfo(): Promise<ChannelInfo> {
  return get<ChannelInfo>('/system/update/channel');
}

/**
 * `PUT /api/v1/system/update/channel` 호출 (SPEC-UPDATE-002 M7).
 *
 * - 잘못된 채널 enum → 400 (CHANNEL_INVALID).
 * - 비-admin 호출 → 401 (UNAUTHORIZED).
 */
export async function putChannel(
  req: ChangeChannelRequest,
): Promise<ChangeChannelResponse> {
  return put<ChangeChannelResponse>('/system/update/channel', req);
}

/**
 * 채널 정보 조회 훅 (SPEC-UPDATE-002 M6).
 *
 * 채널 목록은 자주 바뀌지 않으므로 1분 staleTime 으로 충분하다. 자동 폴링은
 * 사용하지 않으며, 채널 변경 mutation 성공 시 무효화로 갱신한다.
 */
export function useChannelInfo(): UseQueryResult<ChannelInfo, Error> {
  return useQuery<ChannelInfo, Error>({
    queryKey: ['system', 'update', 'channel'],
    queryFn: fetchChannelInfo,
    staleTime: 60_000,
  });
}

/**
 * 채널 변경 트리거 훅 (SPEC-UPDATE-002 M7).
 *
 * 성공 시 두 query 를 무효화하여 UI 가 즉시 반영되도록 한다:
 *   - `['system', 'version']`           — 새 채널의 latest_version 반영
 *   - `['system', 'update', 'channel']` — 현재 채널 표시 갱신
 *
 * 응답의 `check_result` 가 null 인 경우 (네트워크 에러로 즉시 check 실패) 에도
 * 채널 변경 자체는 적용되었으므로 query 무효화는 동일하게 수행한다.
 */
export function useChangeChannel(): UseMutationResult<
  ChangeChannelResponse,
  Error,
  ChangeChannelRequest
> {
  const queryClient = useQueryClient();
  return useMutation<ChangeChannelResponse, Error, ChangeChannelRequest>({
    mutationFn: putChannel,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['system', 'version'] });
      queryClient.invalidateQueries({
        queryKey: ['system', 'update', 'channel'],
      });
    },
  });
}
