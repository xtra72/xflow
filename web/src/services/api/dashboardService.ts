// SPEC-DASHBOARD-001 v0.2.0 — 대시보드 snapshot REST 클라이언트.
//
// 6 endpoints over `/dashboards/{shared,mine}` GET/PUT/DELETE.
// `If-Match: <version>` 헤더로 last-write-wins 충돌 처리. 404 는 null 로 반환하여
// 호출자가 빌트인 기본 대시보드로 fallback 할 수 있게 한다. 409 는
// `ConflictResult` 로 반환 (Promise resolve), 401/403/413/500 은 타입화된 Error
// 로 throw.
//
// baseURL 은 `client.ts` 의 axios instance(`/api/v1`) 를 그대로 사용한다 —
// 백엔드 라우트는 `/api/v1/dashboards/{shared,mine}` 에 등록될 것으로 전제.
// (interceptor envelope 처리는 통과하므로 axios 응답 unwrap 이 자동 적용된다.)
//
// @spec SPEC-DASHBOARD-001 v0.2.0

import axios from 'axios';
import { apiClient } from './client';
import type { DashboardPayload, DashboardSnapshot } from '@/types/dashboard';

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

/** 권한 부족 (403). 예: editor/viewer 가 공유 PUT 시도. */
export class DashboardForbiddenError extends Error {
  constructor(message = 'forbidden') {
    super(message);
    this.name = 'DashboardForbiddenError';
  }
}

/** 페이로드 크기 초과 (413). */
export class DashboardPayloadTooLargeError extends Error {
  constructor(message = 'payload too large') {
    super(message);
    this.name = 'DashboardPayloadTooLargeError';
  }
}

/** 잘못된 요청 (400). schema/owner spoofing/scope mismatch. */
export class DashboardBadRequestError extends Error {
  constructor(message = 'bad request') {
    super(message);
    this.name = 'DashboardBadRequestError';
  }
}

/** 서버 오류 (500). */
export class DashboardServerError extends Error {
  constructor(message = 'internal server error') {
    super(message);
    this.name = 'DashboardServerError';
  }
}

/** PUT 409 응답 — 서버측 최신 snapshot 을 포함하여 last-write-wins 재시도에 사용. */
export interface ConflictResult {
  conflict: true;
  serverSnapshot: DashboardSnapshot;
}

/** PUT 성공/충돌 union — `'conflict' in result` 로 분기. */
export type PutResult = DashboardSnapshot | ConflictResult;

// ---------------------------------------------------------------------------
// 내부 헬퍼
// ---------------------------------------------------------------------------

/** API envelope `{success, data, error, meta}` 를 가정한 응답 본문 형태. */
interface ApiEnvelope<T> {
  success: boolean;
  data?: T;
  error?: { code: string; message: string; details?: unknown };
}

/** envelope 응답에서 data 를 꺼낸다 — 백엔드 응답 unwrap. */
function unwrapEnvelope<T>(body: unknown): T {
  // 백엔드가 envelope 으로 감싸지 않은 raw snapshot 을 보낼 수도 있으므로
  // 형태에 따라 분기한다 (SPEC 의 DTO 가 직접 노출되는 경우를 허용).
  if (body && typeof body === 'object' && 'success' in (body as Record<string, unknown>)) {
    const env = body as ApiEnvelope<T>;
    if (env.success && env.data !== undefined) return env.data;
    if (env.data !== undefined) return env.data;
  }
  return body as T;
}

/** 응답 상태를 보고 알맞은 에러를 throw 하거나 결과를 반환한다. */
function throwForStatus(status: number, body?: unknown): never {
  switch (status) {
    case 401:
      throw new DashboardUnauthorizedError();
    case 403:
      throw new DashboardForbiddenError();
    case 413:
      throw new DashboardPayloadTooLargeError();
    case 400: {
      const msg =
        body && typeof body === 'object' && 'error' in (body as Record<string, unknown>)
          ? (body as ApiEnvelope<unknown>).error?.message ?? 'bad request'
          : 'bad request';
      throw new DashboardBadRequestError(msg);
    }
    case 500:
    default:
      throw new DashboardServerError(`unexpected status ${status}`);
  }
}

/** If-Match 헤더를 조립한다 — version <= 0 이면 (서버에 아직 없음) 헤더 생략. */
function ifMatchHeader(ifMatch?: number): Record<string, string> {
  if (ifMatch === undefined || ifMatch === null) return {};
  if (ifMatch <= 0) return {};
  return { 'If-Match': String(ifMatch) };
}

// ---------------------------------------------------------------------------
// GET — 단일 snapshot 조회 (404 → null)
// ---------------------------------------------------------------------------

async function getSnapshot(path: string): Promise<DashboardSnapshot | null> {
  try {
    const response = await apiClient.get<unknown>(path, {
      // interceptor 가 200 만 통과시키지만, 404 같은 오류 응답은 catch 절에서 처리.
      validateStatus: (status) => status === 200 || status === 404,
    });
    if (response.status === 404) return null;
    // interceptor 가 response.data 를 unwrap 했지만, validateStatus 로 흐름이 바뀌면
    // envelope 이 그대로일 수 있으므로 한 번 더 안전하게 unwrap 한다.
    return unwrapEnvelope<DashboardSnapshot>(response.data);
  } catch (err) {
    if (axios.isAxiosError(err) && err.response) {
      if (err.response.status === 404) return null;
      throwForStatus(err.response.status, err.response.data);
    }
    throw err;
  }
}

/** 공유(global) 대시보드 snapshot 을 조회한다. 없으면 null. */
export async function getSharedDashboard(): Promise<DashboardSnapshot | null> {
  return getSnapshot('/dashboards/shared');
}

/** 본인 개인(user) 대시보드 snapshot 을 조회한다. 없으면 null. */
export async function getMyDashboard(): Promise<DashboardSnapshot | null> {
  return getSnapshot('/dashboards/mine');
}

// ---------------------------------------------------------------------------
// PUT — snapshot 저장 (200 / 409 분기)
// ---------------------------------------------------------------------------

async function putSnapshot(
  path: string,
  payload: DashboardPayload,
  ifMatch?: number,
): Promise<PutResult> {
  try {
    const response = await apiClient.put<unknown>(
      path,
      { payload },
      {
        headers: ifMatchHeader(ifMatch),
        validateStatus: (status) => status === 200 || status === 409,
      },
    );
    if (response.status === 409) {
      const serverSnapshot = unwrapEnvelope<DashboardSnapshot>(response.data);
      return { conflict: true, serverSnapshot };
    }
    return unwrapEnvelope<DashboardSnapshot>(response.data);
  } catch (err) {
    if (axios.isAxiosError(err) && err.response) {
      const { status, data } = err.response;
      if (status === 409) {
        const serverSnapshot = unwrapEnvelope<DashboardSnapshot>(data);
        return { conflict: true, serverSnapshot };
      }
      throwForStatus(status, data);
    }
    throw err;
  }
}

/**
 * 공유 대시보드 snapshot 을 저장한다 (admin only — 비 admin 은 403).
 *
 * @param payload 클라이언트 메모리 상태 (scope/owner/version 은 서버가 부여).
 * @param ifMatch 최종 관측한 server version. 없으면 unconditional 최초 생성.
 */
export async function putSharedDashboard(
  payload: DashboardPayload,
  ifMatch?: number,
): Promise<PutResult> {
  return putSnapshot('/dashboards/shared', payload, ifMatch);
}

/**
 * 본인 개인 대시보드 snapshot 을 저장한다. JWT 의 username 으로 owner 결정.
 */
export async function putMyDashboard(
  payload: DashboardPayload,
  ifMatch?: number,
): Promise<PutResult> {
  return putSnapshot('/dashboards/mine', payload, ifMatch);
}

// ---------------------------------------------------------------------------
// DELETE — snapshot 삭제 (204 정상, 401/403 등)
// ---------------------------------------------------------------------------

async function deleteSnapshot(path: string): Promise<void> {
  try {
    await apiClient.delete(path, {
      validateStatus: (status) => status === 204 || status === 404,
    });
  } catch (err) {
    if (axios.isAxiosError(err) && err.response) {
      throwForStatus(err.response.status, err.response.data);
    }
    throw err;
  }
}

/** 공유 대시보드를 삭제한다 (admin only). */
export async function deleteSharedDashboard(): Promise<void> {
  return deleteSnapshot('/dashboards/shared');
}

/** 본인 개인 대시보드를 삭제한다. */
export async function deleteMyDashboard(): Promise<void> {
  return deleteSnapshot('/dashboards/mine');
}
