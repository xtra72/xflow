// SPEC-WEB-006 v0.1.0 (M2, M3, M5, M6, M7) — System Update API 클라이언트.
//
// SPEC-UPDATE-001 v0.1.0 의 5 REST endpoint 를 소비하는 pure-API 함수와
// React Query 훅을 제공한다.
//
// 엔드포인트 (envelope 는 client 인터셉터가 풀어준다):
//   GET    /api/v1/system/version           — 현재 + 채널 최신 버전
//   POST   /api/v1/system/update/check      — 채널 즉시 폴링
//   POST   /api/v1/system/update/apply      — 업데이트 시작 (operation_id 발급)
//   POST   /api/v1/system/update/rollback   — 백업 바이너리로 복원
//   GET    /api/v1/system/update/status     — 활성 작업 상태 (1s 폴링용)
//
// @spec SPEC-WEB-006 v0.1.0
// @spec SPEC-UPDATE-001 v0.1.0

import {
  useMutation,
  useQuery,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import { get, post } from './client';

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
 * 업데이트 작업의 9-state machine (SPEC-UPDATE-001 M6 / SPEC-WEB-006 M6).
 *
 * Active operation: `starting | checking | downloading | verifying | applying | ready_to_restart`.
 * Terminal: `completed | failed`. UI 는 terminal 상태에서 status 폴링을 중단한다.
 */
export type OperationStatus =
  | 'idle'
  | 'starting'
  | 'checking'
  | 'downloading'
  | 'verifying'
  | 'applying'
  | 'ready_to_restart'
  | 'completed'
  | 'failed';

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
 * - `version` 미지정 → 백엔드가 채널 최신 버전 사용.
 * - `force=true`     → 다운그레이드 허용 (M8 anti-downgrade 우회).
 */
export interface ApplyRequest {
  version?: string;
  force?: boolean;
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
