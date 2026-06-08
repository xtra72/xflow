// useDashboardConfigTarget / useMetricsTarget 테스트 (SPEC-REMOTE-001 M10, 그룹 L).
//
// 검증:
//   - 로컬 target: 원격 config 를 fetch 하지 않는다(payload undefined, sync 없음).
//   - 원격 target: getRemoteDashboard(scope) 로 READ-ONLY config 를 취득한다.
//   - useMetricsTarget: 로컬은 getMetrics, 원격은 getRemoteMetrics 를 소스로 쓴다.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const getRemoteDashboardMock = vi.hoisted(() => vi.fn());
const getRemoteMetricsMock = vi.hoisted(() => vi.fn());
const getMetricsMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/remoteService', () => ({
  getRemoteDashboard: getRemoteDashboardMock,
  getRemoteMetrics: getRemoteMetricsMock,
}));
vi.mock('@/services/api/monitorService', () => ({
  getMetrics: getMetricsMock,
}));

import { LOCAL_TARGET } from '@/lib/remote/target';

import {
  normalizeDashboardSnapshot,
  useDashboardConfigTarget,
} from './useDashboardConfigTarget';
import { useMetricsTarget } from './useMetricsTarget';

/** 테스트 픽스처: 최소 식별 필드(dashboardPages)를 갖는 페이로드. */
const SAMPLE_PAYLOAD = {
  dashboardPages: [
    {
      id: 'p1',
      name: 'P1',
      isDefault: true,
      panels: [{ id: 'flows-1', type: 'flows', title: 'F', config: {} }],
      layout: [{ i: 'flows-1', x: 0, y: 0, w: 4, h: 3 }],
    },
  ],
  activeDashboardId: 'p1',
  dashboardGridCols: 10,
  dashboardShowGridLines: false,
  dashboardRefreshInterval: 5,
  deviceGridLayout: {},
};

/** 유니코드 안전 base64 인코딩(테스트용 — 구현부와 대칭 경로). */
function toBase64Json(value: unknown): string {
  const json = JSON.stringify(value);
  const bytes = new TextEncoder().encode(json);
  let binary = '';
  for (const b of bytes) binary += String.fromCharCode(b);
  return btoa(binary);
}

function wrapper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

beforeEach(() => {
  getRemoteDashboardMock.mockReset();
  getRemoteMetricsMock.mockReset();
  getMetricsMock.mockReset();
});

describe('normalizeDashboardSnapshot', () => {
  it('(a) DTO 형태: payload 객체를 그대로 반환한다', () => {
    const result = normalizeDashboardSnapshot({
      scope: 'global',
      owner: null,
      version: 3,
      updatedAt: 1,
      payload: SAMPLE_PAYLOAD,
    });
    expect(result).toEqual(SAMPLE_PAYLOAD);
  });

  it('(b) RAW 노드 형태: Payload(base64 문자열)를 디코드해 객체로 반환한다', () => {
    const result = normalizeDashboardSnapshot({
      Scope: 'global',
      Owner: '',
      Version: 3,
      UpdatedAt: 1,
      Payload: toBase64Json(SAMPLE_PAYLOAD),
    });
    expect(result).toEqual(SAMPLE_PAYLOAD);
  });

  it('소문자 payload 가 base64 문자열이어도(방어적) 디코드한다', () => {
    const result = normalizeDashboardSnapshot({
      payload: toBase64Json(SAMPLE_PAYLOAD),
    });
    expect(result).toEqual(SAMPLE_PAYLOAD);
  });

  it('유니코드(멀티바이트)가 섞인 페이로드도 안전하게 디코드한다', () => {
    const unicodePayload = {
      ...SAMPLE_PAYLOAD,
      dashboardPages: [{ ...SAMPLE_PAYLOAD.dashboardPages[0], name: '대시보드 ✓' }],
    };
    const result = normalizeDashboardSnapshot({
      Payload: toBase64Json(unicodePayload),
    });
    expect(result).toEqual(unicodePayload);
  });

  it('(c) garbage 입력은 undefined 를 반환한다(크래시 X)', () => {
    expect(normalizeDashboardSnapshot(null)).toBeUndefined();
    expect(normalizeDashboardSnapshot(undefined)).toBeUndefined();
    expect(normalizeDashboardSnapshot(42)).toBeUndefined();
    expect(normalizeDashboardSnapshot('plain-string')).toBeUndefined();
    expect(normalizeDashboardSnapshot({})).toBeUndefined();
    // base64 가 아닌 문자열 / base64 지만 JSON 아님 / JSON 이지만 payload 아님.
    expect(normalizeDashboardSnapshot({ Payload: '@@not-base64@@' })).toBeUndefined();
    expect(normalizeDashboardSnapshot({ Payload: btoa('not json') })).toBeUndefined();
    expect(
      normalizeDashboardSnapshot({ Payload: btoa('{"foo":1}') }),
    ).toBeUndefined();
    // payload 가 dashboardPages 없는 객체.
    expect(normalizeDashboardSnapshot({ payload: { foo: 1 } })).toBeUndefined();
  });
});

describe('useDashboardConfigTarget', () => {
  it('로컬 target 이면 원격 config 를 fetch 하지 않는다(READ-ONLY 없음)', () => {
    const { result } = renderHook(
      () => useDashboardConfigTarget(LOCAL_TARGET, 'shared'),
      { wrapper: wrapper() },
    );
    expect(getRemoteDashboardMock).not.toHaveBeenCalled();
    expect(result.current.payload).toBeUndefined();
  });

  it('원격 target 이면 getRemoteDashboard(scope) 로 config 를 READ-ONLY 취득한다', async () => {
    const payload = { dashboardPages: [], activeDashboardId: '', dashboardGridCols: 10 };
    getRemoteDashboardMock.mockResolvedValueOnce({ payload });

    const { result } = renderHook(
      () =>
        useDashboardConfigTarget({ type: 'remote', instanceId: 'node-1' }, 'mine', true),
      { wrapper: wrapper() },
    );

    await waitFor(() => expect(result.current.payload).toEqual(payload));
    expect(getRemoteDashboardMock).toHaveBeenCalledWith('node-1', 'mine');
  });

  it('원격 노드가 RAW base64 형태를 반환해도 payload 를 디코드해 노출한다', async () => {
    // 현행 노드 프록시는 대문자 키 + Payload(base64) 형태를 반환한다.
    getRemoteDashboardMock.mockResolvedValueOnce({
      Scope: 'global',
      Owner: '',
      Version: 1,
      UpdatedAt: 0,
      Payload: toBase64Json(SAMPLE_PAYLOAD),
    });

    const { result } = renderHook(
      () =>
        useDashboardConfigTarget({ type: 'remote', instanceId: 'node-1' }, 'shared', true),
      { wrapper: wrapper() },
    );

    await waitFor(() => expect(result.current.payload).toEqual(SAMPLE_PAYLOAD));
  });

  it('원격 노드가 garbage 를 반환하면 payload 는 undefined(빈 상태, 크래시 X)', async () => {
    getRemoteDashboardMock.mockResolvedValueOnce({ Payload: '@@garbage@@' });

    const { result } = renderHook(
      () =>
        useDashboardConfigTarget({ type: 'remote', instanceId: 'node-1' }, 'shared', true),
      { wrapper: wrapper() },
    );

    await waitFor(() => expect(getRemoteDashboardMock).toHaveBeenCalled());
    expect(result.current.payload).toBeUndefined();
  });

  it('enabled=false 면 원격 config 를 fetch 하지 않는다(게이팅)', () => {
    renderHook(
      () =>
        useDashboardConfigTarget({ type: 'remote', instanceId: 'node-1' }, 'shared', false),
      { wrapper: wrapper() },
    );
    expect(getRemoteDashboardMock).not.toHaveBeenCalled();
  });
});

describe('useMetricsTarget', () => {
  it('로컬 target 이면 getMetrics 를 소스로 쓴다(getRemoteMetrics 미호출)', async () => {
    getMetricsMock.mockResolvedValueOnce({ cpu: 5 });
    const { result } = renderHook(() => useMetricsTarget(LOCAL_TARGET, 1000), {
      wrapper: wrapper(),
    });
    await waitFor(() => expect(result.current.metrics).toEqual({ cpu: 5 }));
    expect(getRemoteMetricsMock).not.toHaveBeenCalled();
  });

  it('원격 target 이면 getRemoteMetrics 를 소스로 쓴다(getMetrics 미호출)', async () => {
    getRemoteMetricsMock.mockResolvedValueOnce({ cpu: 9 });
    const { result } = renderHook(
      () => useMetricsTarget({ type: 'remote', instanceId: 'node-1' }, 1000, true),
      { wrapper: wrapper() },
    );
    await waitFor(() => expect(result.current.metrics).toEqual({ cpu: 9 }));
    expect(getRemoteMetricsMock).toHaveBeenCalledWith('node-1');
    expect(getMetricsMock).not.toHaveBeenCalled();
  });
});
