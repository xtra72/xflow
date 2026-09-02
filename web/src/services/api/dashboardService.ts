// SPEC-DASHBOARD-004 — 대시보드 1급 엔티티 REST 클라이언트.
//
// 계약은 spec.md §2.3 라우트 표를 그대로 따른다.
//
//   GET    /dashboards             목록 (payload 미포함)
//   POST   /dashboards             생성 → 201
//   GET    /dashboards/{uid}       단건 (payload 포함)
//   PUT    /dashboards/{uid}       본문 저장 (If-Match)
//   PATCH  /dashboards/{uid}       메타 변경 (이름·공개범위·기본·정렬)
//   DELETE /dashboards/{uid}       삭제 → 204
//   GET    /dashboards/{uid}/acl   권한 목록
//   PUT    /dashboards/{uid}/acl   권한 전량 치환
//   GET    /dashboard-state        본인 UI 상태
//   PUT    /dashboard-state        본인 UI 상태 저장
//
// 구 모델의 묶음 단위 엔드포인트(`/dashboards/{shared,mine}` GET/PUT/DELETE)는
// 더 이상 이 클라이언트에서 호출하지 않는다. 서버의 GET shim 은 원격 노드 프록시
// 전용으로만 남아 있다(spec.md §4.4) — 그 경로는 remoteService 가 담당한다.
//
// 상태 코드 매핑:
//   - 404: 조회는 `null`, 저장은 `DashboardNotFoundError` (호출자가 폴백 판단).
//   - 409: `ConflictResult` 로 **resolve** 한다 (throw 아님) — 호출자가 서버
//          version 으로 재시도할 수 있어야 하기 때문.
//   - 401/403/400/413/500: 타입화된 Error 로 throw.
//
// @spec SPEC-DASHBOARD-004 v0.1.0 (§2.3, §2.8)

import axios from 'axios';

import { apiClient } from './client';
import { APIError } from '@/types/api';
import type {
  Dashboard,
  DashboardAclEntry,
  DashboardContent,
  DashboardDetail,
  DashboardPatch,
  DashboardUserState,
} from '@/types/dashboard';

// ---------------------------------------------------------------------------
// 타입화된 에러
// ---------------------------------------------------------------------------

/** 인증 만료/누락 (401). 호출자가 interceptor 의 refresh 흐름에 위임할 수 있다. */
export class DashboardUnauthorizedError extends Error {
  constructor(message = 'unauthorized') {
    super(message);
    this.name = 'DashboardUnauthorizedError';
  }
}

/** 권한 부족 (403). 예: view 만 가진 사용자의 저장 시도. */
export class DashboardForbiddenError extends Error {
  constructor(message = 'forbidden') {
    super(message);
    this.name = 'DashboardForbiddenError';
  }
}

/** 대상 없음 (404). 저장 경로에서 "타 세션이 삭제함" 을 뜻한다. */
export class DashboardNotFoundError extends Error {
  constructor(message = 'not found') {
    super(message);
    this.name = 'DashboardNotFoundError';
  }
}

/** 페이로드 크기 초과 (413). */
export class DashboardPayloadTooLargeError extends Error {
  constructor(message = 'payload too large') {
    super(message);
    this.name = 'DashboardPayloadTooLargeError';
  }
}

/** 잘못된 요청 (400). 이름 규칙 위반, ACL subject 검증 실패 등. */
export class DashboardBadRequestError extends Error {
  constructor(message = 'bad request') {
    super(message);
    this.name = 'DashboardBadRequestError';
  }
}

/** 서버 오류 (500 및 그 밖의 예상하지 못한 상태). */
export class DashboardServerError extends Error {
  constructor(message = 'internal server error') {
    super(message);
    this.name = 'DashboardServerError';
  }
}

/** PUT 409 응답 — 서버측 최신 대시보드를 포함하여 재시도에 사용한다. */
export interface ConflictResult {
  conflict: true;
  serverDashboard: DashboardDetail;
}

/** PUT 성공/충돌 union — `'conflict' in result` 로 분기. */
export type DashboardPutResult = DashboardDetail | ConflictResult;

// ---------------------------------------------------------------------------
// 내부 헬퍼
// ---------------------------------------------------------------------------

/** API envelope `{success, data, error, meta}` 를 가정한 응답 본문 형태. */
interface ApiEnvelope<T> {
  success: boolean;
  data?: T;
  error?: { code: string; message: string; details?: unknown };
}

/** envelope 응답에서 data 를 꺼낸다.
 *
 * client.ts 의 성공 interceptor 가 이미 unwrap 하지만, `validateStatus` 로 흐름이
 * 바뀌거나 에러 경로로 들어오면 envelope 이 그대로 남는다. 형태를 보고 분기한다.
 */
function unwrapEnvelope<T>(body: unknown): T {
  if (body && typeof body === 'object' && 'success' in (body as Record<string, unknown>)) {
    const env = body as ApiEnvelope<T>;
    if (env.data !== undefined) return env.data;
  }
  return body as T;
}

/** 응답 상태를 알맞은 타입화된 에러로 바꾼다. */
function throwForStatus(status: number, body?: unknown): never {
  const message =
    body && typeof body === 'object' && 'error' in (body as Record<string, unknown>)
      ? ((body as ApiEnvelope<unknown>).error?.message ?? '')
      : '';
  switch (status) {
    case 401:
      throw new DashboardUnauthorizedError(message || 'unauthorized');
    case 403:
      throw new DashboardForbiddenError(message || 'forbidden');
    case 404:
      throw new DashboardNotFoundError(message || 'not found');
    case 413:
      throw new DashboardPayloadTooLargeError(message || 'payload too large');
    case 400:
      throw new DashboardBadRequestError(message || 'bad request');
    default:
      throw new DashboardServerError(message || `unexpected status ${status}`);
  }
}

/**
 * catch 절의 알 수 없는 에러를 타입화된 에러로 정규화한다.
 *
 * client.ts 의 response error interceptor 는 envelope 의 `error` 필드를 보고
 * APIError 를 던지므로 AxiosError 가 아닐 수 있다. 두 경로를 모두 다루지 않으면
 * 401/403 이 일반 Error 로 흘러가 호출자의 무한 재시도 가드를 우회한다.
 */
function rethrowDashboardError(err: unknown): never {
  if (err instanceof APIError) {
    throwForStatus(err.status, { error: { code: err.code, message: err.message } });
  }
  if (axios.isAxiosError(err) && err.response) {
    throwForStatus(err.response.status, err.response.data);
  }
  throw err;
}

/** 에러에서 HTTP 상태를 뽑는다. 알 수 없으면 0. */
function statusOf(err: unknown): number {
  if (err instanceof APIError) return err.status;
  if (axios.isAxiosError(err) && err.response) return err.response.status;
  return 0;
}

/** If-Match 헤더를 조립한다 — version <= 0 이면 (서버에 아직 없음) 헤더 생략. */
function ifMatchHeader(ifMatch?: number): Record<string, string> {
  if (ifMatch === undefined || ifMatch === null || ifMatch <= 0) return {};
  return { 'If-Match': String(ifMatch) };
}

/** uid 를 경로 세그먼트로 안전하게 인코딩한다. */
function uidPath(uid: string): string {
  return `/dashboards/${encodeURIComponent(uid)}`;
}

// ---------------------------------------------------------------------------
// 대시보드 CRUD
// ---------------------------------------------------------------------------

/**
 * 요청자가 view 가능한 대시보드 목록을 조회한다 (payload 미포함).
 *
 * 부팅 시 이 호출 **1회**로 접근 가능한 전체 목록이 확정된다(spec.md §2.14 UB2 #1).
 */
export async function listDashboards(): Promise<Dashboard[]> {
  try {
    const response = await apiClient.get<unknown>('/dashboards');
    return unwrapEnvelope<Dashboard[]>(response.data) ?? [];
  } catch (err) {
    rethrowDashboardError(err);
  }
}

/** 대시보드 1장을 payload 와 함께 조회한다. 없으면 `null`. */
export async function getDashboard(uid: string): Promise<DashboardDetail | null> {
  try {
    const response = await apiClient.get<unknown>(uidPath(uid));
    return unwrapEnvelope<DashboardDetail>(response.data);
  } catch (err) {
    if (statusOf(err) === 404) return null;
    rethrowDashboardError(err);
  }
}

/**
 * 대시보드를 생성한다 (spec.md §2.7 E1).
 *
 * `owner` · `visibility` · `version` · `uid` 는 서버가 결정하므로 보내지 않는다.
 * 생성 응답을 목록에 그대로 삽입할 수 있도록 detail 을 반환한다.
 */
export async function createDashboard(
  name: string,
  payload?: DashboardContent,
): Promise<DashboardDetail> {
  try {
    const body: Record<string, unknown> = { name };
    if (payload !== undefined) body.payload = payload;
    const response = await apiClient.post<unknown>('/dashboards', body);
    return unwrapEnvelope<DashboardDetail>(response.data);
  } catch (err) {
    rethrowDashboardError(err);
  }
}

/**
 * 대시보드 본문을 저장한다 (spec.md §2.8 E2).
 *
 * 409 는 throw 하지 않고 `ConflictResult` 로 resolve 한다 — 호출자가 서버
 * version 으로 1회 재시도할 수 있어야 하기 때문이다.
 *
 * @param ifMatch 최종 관측한 서버 version. 0 이하면 헤더를 생략한다.
 */
export async function updateDashboard(
  uid: string,
  payload: DashboardContent,
  ifMatch?: number,
): Promise<DashboardPutResult> {
  try {
    const response = await apiClient.put<unknown>(
      uidPath(uid),
      { payload },
      {
        headers: ifMatchHeader(ifMatch),
        // 409 는 서버가 `success:true` envelope 에 최신 대시보드를 실어 보낸다.
        // 성공 interceptor 를 태워야 unwrap 이 일관되게 적용된다.
        validateStatus: (status) => status === 200 || status === 409,
      },
    );
    const detail = unwrapEnvelope<DashboardDetail>(response.data);
    if (response.status === 409) {
      return { conflict: true, serverDashboard: detail };
    }
    return detail;
  } catch (err) {
    // interceptor 가 409 envelope 을 먼저 가로챈 경우를 대비한 방어 경로.
    if (statusOf(err) === 409 && err instanceof APIError && err.details) {
      const detail = err.details as DashboardDetail;
      if (typeof detail.uid === 'string') {
        return { conflict: true, serverDashboard: detail };
      }
    }
    rethrowDashboardError(err);
  }
}

/** 대시보드 메타(이름·공개범위·기본 여부·정렬)를 변경한다. */
export async function patchDashboard(
  uid: string,
  patch: DashboardPatch,
  ifMatch?: number,
): Promise<DashboardDetail> {
  try {
    const response = await apiClient.patch<unknown>(uidPath(uid), patch, {
      headers: ifMatchHeader(ifMatch),
    });
    return unwrapEnvelope<DashboardDetail>(response.data);
  } catch (err) {
    rethrowDashboardError(err);
  }
}

/** 대시보드를 삭제한다 (204). 이미 없으면 성공으로 간주한다. */
export async function deleteDashboard(uid: string): Promise<void> {
  try {
    await apiClient.delete(uidPath(uid));
  } catch (err) {
    if (statusOf(err) === 404) return;
    rethrowDashboardError(err);
  }
}

// ---------------------------------------------------------------------------
// 권한 부여 (ACL)
// ---------------------------------------------------------------------------

/** 대시보드의 권한 목록을 조회한다 (grant 인가 필요). */
export async function getAcl(uid: string): Promise<DashboardAclEntry[]> {
  try {
    const response = await apiClient.get<unknown>(`${uidPath(uid)}/acl`);
    return unwrapEnvelope<DashboardAclEntry[]>(response.data) ?? [];
  } catch (err) {
    rethrowDashboardError(err);
  }
}

/**
 * 대시보드의 권한 목록을 **전량 치환**한다 (spec.md §4.5 — 부분 갱신 없음).
 *
 * 하나라도 유효하지 않으면 서버가 400 으로 전체를 거부하며 기존 ACL 은 변경되지
 * 않는다(spec.md §2.9 E3).
 */
export async function putAcl(
  uid: string,
  entries: DashboardAclEntry[],
): Promise<DashboardAclEntry[]> {
  try {
    // 서버는 최상위 배열과 `{entries:[...]}` 를 모두 받는다. AC-17 이 배열을 쓴다.
    const response = await apiClient.put<unknown>(`${uidPath(uid)}/acl`, entries);
    return unwrapEnvelope<DashboardAclEntry[]>(response.data) ?? [];
  } catch (err) {
    rethrowDashboardError(err);
  }
}

// ---------------------------------------------------------------------------
// 사용자 UI 상태 (/dashboard-state)
// ---------------------------------------------------------------------------

/** PUT /dashboard-state 요청 본문. username 은 세션 사용자로 고정된다. */
export interface DashboardUserStateInput {
  active_dashboard_uid: string;
  device_grid_layout: Record<string, import('@/stores/uiStore').DashboardLayoutItem>;
}

/** 빈 UI 상태 — 서버가 행을 갖고 있지 않을 때의 형상과 동일하다. */
function emptyUserState(): DashboardUserState {
  return {
    active_dashboard_uid: '',
    device_grid_layout: {},
    version: 0,
    updated_at: 0,
  };
}

/**
 * 본인의 대시보드 UI 상태를 조회한다.
 *
 * 서버는 행이 없어도 404 가 아니라 기본값을 반환한다. 그럼에도 방어적으로 404 를
 * 빈 상태로 접는다 — 최초 로그인 사용자가 폴백 분기를 타야 할 이유가 없다.
 */
export async function getState(): Promise<DashboardUserState> {
  try {
    const response = await apiClient.get<unknown>('/dashboard-state');
    const st = unwrapEnvelope<DashboardUserState>(response.data);
    return st ?? emptyUserState();
  } catch (err) {
    if (statusOf(err) === 404) return emptyUserState();
    rethrowDashboardError(err);
  }
}

/**
 * 본인의 대시보드 UI 상태를 저장한다.
 *
 * `If-Match` 를 보내지 않는다 — 이 리소스는 사용자 1인 소유이고 서버가 헤더
 * 부재를 무조건 저장으로 해석한다. 헤더를 붙이면 폴백 정정 저장(spec.md §2.13
 * UB1 #11)이 409 로 막혀 "정정할 수 없는 잘못된 활성 uid" 상태가 고착된다.
 */
export async function putState(input: DashboardUserStateInput): Promise<DashboardUserState> {
  try {
    const response = await apiClient.put<unknown>('/dashboard-state', input);
    const st = unwrapEnvelope<DashboardUserState>(response.data);
    return st ?? emptyUserState();
  } catch (err) {
    rethrowDashboardError(err);
  }
}
