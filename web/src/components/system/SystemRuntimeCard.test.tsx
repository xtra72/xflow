// SPEC-WEB-007 v0.1.0 (M3, M7, M9) — SystemRuntimeCard 컴포넌트 테스트.
//
// 런타임 메트릭(uptime, CPU%, 메모리%, goroutines, Go heap) 카드. useSystemMetrics
// 훅을 mock 으로 주입하여 필드 렌더링·단위/소수점·uptime 가독 형식·CPU 0% 안내·
// 로딩/에러 독립을 검증한다.
//
// 커버 AC:
//   - AC-6  Runtime 필드 렌더 + 단위/소수점, uptime 가독 형식
//   - AC-7  CPU 0% "측정 미지원" 안내
//   - AC-11 영역별 독립 로딩/에러 ("다시 시도" refetch)
//
// @spec SPEC-WEB-007 v0.1.0 (M3, M7, M9)

import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { SystemMetrics } from '@/services/api/monitorService';

// useSystemMetrics 훅 mock — 상태를 테스트별로 제어한다.
const useSystemMetricsMock = vi.hoisted(() => vi.fn());
vi.mock('@/services/api/monitorService', async () => {
  const actual = await vi.importActual<
    typeof import('@/services/api/monitorService')
  >('@/services/api/monitorService');
  return {
    ...actual,
    useSystemMetrics: useSystemMetricsMock,
  };
});

// i18n: t() 를 ko.json 키 해석으로 모킹해 한국어 단언을 유지한다.
vi.mock('@/lib/i18n', async () => {
  const ko = (await import('@/lib/i18n/ko.json')).default as Record<
    string,
    unknown
  >;
  const resolve = (key: string): string => {
    const v = key.split('.').reduce<unknown>(
      (o, p) =>
        o && typeof o === 'object' ? (o as Record<string, unknown>)[p] : undefined,
      ko,
    );
    return typeof v === 'string' ? v : key;
  };
  return {
    useTranslation: () => ({
      t: resolve,
      locale: 'ko' as const,
      setLocale: () => {},
    }),
  };
});

import { SystemRuntimeCard } from './SystemRuntimeCard';

// ─────────────────────────────────────────────────────────────────────
// Test fixtures
// ─────────────────────────────────────────────────────────────────────

function makeMetrics(overrides: Partial<SystemMetrics> = {}): SystemMetrics {
  return {
    cpu_usage_percent: 12.3,
    memory_usage_percent: 45.6,
    go_routines: 42,
    go_mem_alloc_mb: 12.34,
    go_mem_sys_mb: 56.78,
    uptime_seconds: 274320, // 3d 4h 12m
    ...overrides,
  };
}

interface QueryState {
  data?: SystemMetrics;
  isLoading?: boolean;
  isError?: boolean;
  error?: Error;
  refetch?: () => void;
}

function setQueryState(state: QueryState) {
  useSystemMetricsMock.mockReturnValue({
    data: state.data,
    isLoading: state.isLoading ?? false,
    isError: state.isError ?? false,
    error: state.error,
    refetch: state.refetch ?? vi.fn(),
  });
}

beforeEach(() => {
  useSystemMetricsMock.mockReset();
});

// ─────────────────────────────────────────────────────────────────────
// AC-6 — Runtime 필드 렌더 + 단위/소수점 + uptime 가독 형식
// ─────────────────────────────────────────────────────────────────────

describe('SystemRuntimeCard — Runtime 필드 (AC-6)', () => {
  it('uptime 을 "3d 4h 12m" 가독 형식으로 표시한다', () => {
    setQueryState({ data: makeMetrics({ uptime_seconds: 274320 }) });
    render(<SystemRuntimeCard />);
    expect(screen.getByTestId('runtime-uptime')).toHaveTextContent('3d 4h 12m');
  });

  it('CPU% 를 소수점 1자리 + % 로 표시한다', () => {
    setQueryState({ data: makeMetrics({ cpu_usage_percent: 12.34 }) });
    render(<SystemRuntimeCard />);
    expect(screen.getByTestId('runtime-cpu')).toHaveTextContent('12.3%');
  });

  it('메모리% 를 소수점 1자리 + % 로 표시한다', () => {
    setQueryState({ data: makeMetrics({ memory_usage_percent: 45.67 }) });
    render(<SystemRuntimeCard />);
    expect(screen.getByTestId('runtime-memory')).toHaveTextContent('45.7%');
  });

  it('goroutines 수를 표시한다', () => {
    setQueryState({ data: makeMetrics({ go_routines: 42 }) });
    render(<SystemRuntimeCard />);
    expect(screen.getByTestId('runtime-goroutines')).toHaveTextContent('42');
  });

  it('Go heap 을 alloc/sys MB (소수점 1자리) 로 표시한다', () => {
    setQueryState({
      data: makeMetrics({ go_mem_alloc_mb: 12.34, go_mem_sys_mb: 56.78 }),
    });
    render(<SystemRuntimeCard />);
    const heap = screen.getByTestId('runtime-heap');
    expect(heap).toHaveTextContent('12.3');
    expect(heap).toHaveTextContent('56.8');
    expect(heap).toHaveTextContent(/MB/);
  });
});

// ─────────────────────────────────────────────────────────────────────
// AC-7 — CPU 0% "측정 미지원" 안내
// ─────────────────────────────────────────────────────────────────────

describe('SystemRuntimeCard — CPU 미지원 안내 (AC-7)', () => {
  it('cpu_usage_percent=0 → "측정 미지원" 보조 안내를 함께 표시한다', () => {
    setQueryState({ data: makeMetrics({ cpu_usage_percent: 0 }) });
    render(<SystemRuntimeCard />);
    expect(screen.getByTestId('runtime-cpu-unsupported')).toHaveTextContent(
      /측정 미지원/,
    );
  });

  it('cpu_usage_percent>0 → "측정 미지원" 안내를 표시하지 않는다', () => {
    setQueryState({ data: makeMetrics({ cpu_usage_percent: 5.5 }) });
    render(<SystemRuntimeCard />);
    expect(
      screen.queryByTestId('runtime-cpu-unsupported'),
    ).not.toBeInTheDocument();
  });
});

// ─────────────────────────────────────────────────────────────────────
// AC-11 — 영역별 독립 로딩/에러
// ─────────────────────────────────────────────────────────────────────

describe('SystemRuntimeCard — 로딩/에러 독립 (AC-11)', () => {
  it('isLoading=true → 스켈레톤을 표시한다', () => {
    setQueryState({ isLoading: true });
    render(<SystemRuntimeCard />);
    expect(screen.getByTestId('runtime-loading')).toBeInTheDocument();
  });

  it('isError=true → 에러 + "다시 시도" 버튼을 표시한다', () => {
    setQueryState({ isError: true, error: new Error('boom') });
    render(<SystemRuntimeCard />);
    expect(screen.getByTestId('runtime-error')).toBeInTheDocument();
    expect(screen.getByTestId('runtime-retry')).toHaveTextContent(/다시 시도/);
  });

  it('"다시 시도" 클릭 시 refetch 가 호출된다', () => {
    const refetch = vi.fn();
    setQueryState({ isError: true, error: new Error('boom'), refetch });
    render(<SystemRuntimeCard />);
    fireEvent.click(screen.getByTestId('runtime-retry'));
    expect(refetch).toHaveBeenCalledTimes(1);
  });
});
