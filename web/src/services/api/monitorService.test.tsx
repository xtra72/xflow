// SPEC-WEB-007 — 시스템 메트릭 API + 폴링 훅 테스트.
//
// 모든 HTTP 호출은 `./client` 의 `get` 헬퍼를 통해 envelope 가 이미 풀린 형태로
// 들어온다(인터셉터가 4xx/5xx 를 APIError 로 변환). 프로젝트 컨벤션상 msw 대신
// `./client` 모킹으로 검증한다(systemUpdate.test.tsx 와 동일 전략, 외부 의존성 0).
//
// 검증 범위:
//   - getMetrics: GET /monitor/metrics 호출 + SystemMetrics 타입 매핑 정확성.
//   - useSystemMetrics: 마운트 시 1회 호출, 1초 미만 간격엔 재호출 없음(설정값
//     5000 검증), fake timer 로 5초 경과 시 재호출, refetchIntervalInBackground
//     false 설정.
//
// @spec SPEC-WEB-007

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import { type PropsWithChildren } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  put: vi.fn(),
  del: vi.fn(),
}));

import { getMetrics, useSystemMetrics, type SystemMetrics } from './monitorService';

// 백엔드 MetricsResponse 직렬화 태그와 1:1 매핑되는 샘플 payload.
const SAMPLE: SystemMetrics = {
  cpu_usage_percent: 0,
  memory_usage_percent: 42.5,
  go_routines: 87,
  go_mem_alloc_mb: 12.34,
  go_mem_sys_mb: 56.78,
  uptime_seconds: 3600,
};

// ─────────────────────────────────────────────────────────────────────
// Pure API function
// ─────────────────────────────────────────────────────────────────────

describe('getMetrics', () => {
  beforeEach(() => {
    getMock.mockReset();
  });

  it('GET /monitor/metrics 를 호출하고 SystemMetrics 를 그대로 반환한다', async () => {
    getMock.mockResolvedValueOnce(SAMPLE);

    const result = await getMetrics();

    expect(getMock).toHaveBeenCalledWith('/monitor/metrics');
    expect(result).toEqual(SAMPLE);
  });

  it('6개 필드를 백엔드 직렬화 키 그대로 매핑한다 (타입 정확성)', async () => {
    getMock.mockResolvedValueOnce(SAMPLE);

    const result = await getMetrics();

    // 컴파일 단계에서 SystemMetrics 형상이 강제되며, 런타임에서도 키/값을 검증.
    expect(result.cpu_usage_percent).toBe(0);
    expect(result.memory_usage_percent).toBe(42.5);
    expect(result.go_routines).toBe(87);
    expect(result.go_mem_alloc_mb).toBe(12.34);
    expect(result.go_mem_sys_mb).toBe(56.78);
    expect(result.uptime_seconds).toBe(3600);
  });
});

// ─────────────────────────────────────────────────────────────────────
// useSystemMetrics 폴링 훅
// ─────────────────────────────────────────────────────────────────────

function buildWrapper(client?: QueryClient) {
  const queryClient =
    client ??
    new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
          // 백그라운드 폴링이 테스트 종료 후에도 계속되는 것을 방지.
          refetchOnWindowFocus: false,
        },
      },
    });

  function Wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  }

  return { Wrapper, queryClient };
}

describe('useSystemMetrics', () => {
  beforeEach(() => {
    getMock.mockReset();
  });

  it('마운트 시 GET /monitor/metrics 를 1회 호출하고 data 로 노출한다', async () => {
    getMock.mockResolvedValue(SAMPLE);

    const { Wrapper } = buildWrapper();
    const { result } = renderHook(() => useSystemMetrics(), { wrapper: Wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(getMock).toHaveBeenCalledWith('/monitor/metrics');
    expect(getMock).toHaveBeenCalledTimes(1);
    expect(result.current.data).toEqual(SAMPLE);
  });

  it('refetchInterval 이 5_000ms 로 설정되어 있다 (5초 경과 시 재호출)', async () => {
    vi.useFakeTimers();
    getMock.mockResolvedValue(SAMPLE);

    const { Wrapper } = buildWrapper();
    renderHook(() => useSystemMetrics(), { wrapper: Wrapper });

    // 첫 호출은 마운트 직후 즉시.
    await vi.waitFor(() => expect(getMock).toHaveBeenCalledTimes(1));

    // 5초 경과 → 두 번째 호출.
    await vi.advanceTimersByTimeAsync(5_100);
    await vi.waitFor(() => expect(getMock).toHaveBeenCalledTimes(2));

    vi.useRealTimers();
  });

  it('1초 미만(및 5초 미만) 간격에는 자동 폴링하지 않는다 (설정값 5000 검증)', async () => {
    vi.useFakeTimers();
    getMock.mockResolvedValue(SAMPLE);

    const { Wrapper } = buildWrapper();
    renderHook(() => useSystemMetrics(), { wrapper: Wrapper });

    await vi.waitFor(() => expect(getMock).toHaveBeenCalledTimes(1));

    // 1초 경과: 추가 호출 없음 (1s 미만 간격 자동 폴링 안 함).
    await vi.advanceTimersByTimeAsync(1_000);
    expect(getMock).toHaveBeenCalledTimes(1);

    // 4.9초 경과(누적 4.9s): 아직 5s 미경과 → 추가 호출 없음.
    await vi.advanceTimersByTimeAsync(3_900);
    expect(getMock).toHaveBeenCalledTimes(1);

    vi.useRealTimers();
  });

  it('refetchIntervalInBackground:false — document 가 hidden 이면 폴링하지 않는다', async () => {
    vi.useFakeTimers();
    getMock.mockResolvedValue(SAMPLE);

    // 탭이 백그라운드(hidden)인 상황을 시뮬레이션.
    const visibilitySpy = vi
      .spyOn(document, 'visibilityState', 'get')
      .mockReturnValue('hidden');

    const { Wrapper } = buildWrapper();
    renderHook(() => useSystemMetrics(), { wrapper: Wrapper });

    // 마운트 시 1회 호출(초기 fetch 는 가시성과 무관).
    await vi.waitFor(() => expect(getMock).toHaveBeenCalledTimes(1));

    // 5초 경과해도 hidden 이므로 refetchIntervalInBackground:false 에 의해
    // 배경 폴링이 일어나지 않는다.
    await vi.advanceTimersByTimeAsync(5_500);
    expect(getMock).toHaveBeenCalledTimes(1);

    visibilitySpy.mockRestore();
    vi.useRealTimers();
  });
});
