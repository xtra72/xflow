// SeriesListPanel 단위 테스트.
// SPEC-WEB-005 v0.2.0: dataSource prop 을 받아 useKeys() 를 호출한다.
// SPEC-WEB-005 v0.3.0: 행별 "데이터 보기" 버튼이 제거되었고,
//                      테이블은 정보 표시 전용으로 전환되었다.
//
// @spec SPEC-WEB-005

import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

import type {
  SeriesDataSource,
  SeriesKeysPage,
  SeriesKeysQueryResult,
} from '@/services/api/seriesDataSource';

import TsdbSeriesListPanel from './TsdbSeriesListPanel';

// i18n: ko.json 을 점 표기 키로 해석하는 mock. 컴포넌트가 useTranslation 을
// 쓰지만 I18nProvider 로 감싸지 않으므로 키를 한국어로 해석해 단언을 통과시킨다.
vi.mock('@/lib/i18n', async () => {
  const ko = (await import('@/lib/i18n/ko.json')).default as Record<string, unknown>;
  const resolve = (key: string): string => {
    const v = key.split('.').reduce<unknown>(
      (o, p) => (o && typeof o === 'object' ? (o as Record<string, unknown>)[p] : undefined),
      ko,
    );
    return typeof v === 'string' ? v : key;
  };
  return {
    useTranslation: () => ({ t: resolve, locale: 'ko' as const, setLocale: () => {} }),
  };
});

/**
 * useKeys 를 수동으로 스텁할 수 있는 가벼운 SeriesDataSource 페이크.
 * 테스트마다 서로 다른 결과를 반환하고 호출 인자를 검사할 수 있다.
 */
function makeFakeDataSource(
  useKeysImpl: (params: { page: number; size: number }) => SeriesKeysQueryResult,
): SeriesDataSource {
  return {
    kind: 'memtsdb',
    useKeys: useKeysImpl,
    queryMatrix: vi.fn(),
  };
}

function makePage(
  keys: string[],
  total: number,
  page = 1,
  size = 10,
): SeriesKeysPage {
  return {
    keys,
    pagination: {
      page,
      size,
      total,
      totalPages: size > 0 ? Math.ceil(total / size) : 0,
    },
  };
}

let lastUseKeysArgs: { page: number; size: number } | undefined;

beforeEach(() => {
  lastUseKeysArgs = undefined;
});

describe('SeriesListPanel', () => {
  it('로딩 중에는 스켈레톤을 표시', () => {
    const ds = makeFakeDataSource((args) => {
      lastUseKeysArgs = args;
      return {
        data: undefined,
        isLoading: true,
        isError: false,
        error: null,
        refetch: vi.fn(),
      };
    });
    render(<TsdbSeriesListPanel dataSource={ds} />);
    expect(screen.getAllByTestId('tsdb-series-skeleton').length).toBeGreaterThan(0);
  });

  it('시리즈 목록을 렌더링하고 총 개수를 표시', () => {
    const ds = makeFakeDataSource(() => ({
      data: makePage(['temp,room=1', 'temp,room=2'], 120, 1, 10),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    }));
    render(<TsdbSeriesListPanel dataSource={ds} />);
    expect(screen.getByText('temp,room=1')).toBeInTheDocument();
    expect(screen.getByText('temp,room=2')).toBeInTheDocument();
    expect(screen.getByText('120')).toBeInTheDocument();
  });

  it('테이블에 행별 "데이터 보기" 버튼이 존재하지 않는다 (v0.3.0 개선)', () => {
    const ds = makeFakeDataSource(() => ({
      data: makePage(['temp,room=1', 'temp,room=2'], 2, 1, 10),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    }));
    render(<TsdbSeriesListPanel dataSource={ds} />);
    expect(screen.queryByRole('button', { name: /데이터 보기/ })).toBeNull();
  });

  it('페이지 크기 셀렉터 변경 시 page=1/size=변경값으로 재조회', () => {
    const useKeysSpy = vi.fn((args: { page: number; size: number }) => {
      lastUseKeysArgs = args;
      return {
        data: makePage(['a'], 1, args.page, args.size),
        isLoading: false,
        isError: false,
        error: null as Error | null,
        refetch: vi.fn(),
      } satisfies SeriesKeysQueryResult;
    });
    const ds = makeFakeDataSource(useKeysSpy);
    render(<TsdbSeriesListPanel dataSource={ds} />);
    // 초기 호출: page=1, size=10
    expect(lastUseKeysArgs).toEqual({ page: 1, size: 10 });

    const select = screen.getByLabelText('페이지당') as HTMLSelectElement;
    fireEvent.change(select, { target: { value: '25' } });
    // 재호출: page=1 (리셋), size=25
    expect(lastUseKeysArgs).toEqual({ page: 1, size: 25 });
  });

  it('첫 페이지에서 "이전" 버튼 비활성, 마지막 페이지에서 "다음" 버튼 비활성', () => {
    const ds = makeFakeDataSource(() => ({
      data: makePage(['a'], 1, 1, 10),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    }));
    render(<TsdbSeriesListPanel dataSource={ds} />);
    const prev = screen.getByRole('button', { name: '이전 페이지' }) as HTMLButtonElement;
    const next = screen.getByRole('button', { name: '다음 페이지' }) as HTMLButtonElement;
    expect(prev.disabled).toBe(true);
    expect(next.disabled).toBe(true);
  });

  it('빈 상태일 때 "저장된 시리즈가 없습니다" 표시', () => {
    const ds = makeFakeDataSource(() => ({
      data: makePage([], 0, 1, 10),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    }));
    render(<TsdbSeriesListPanel dataSource={ds} />);
    expect(screen.getByText(/저장된 시리즈가 없습니다/)).toBeInTheDocument();
    const select = screen.getByLabelText('페이지당') as HTMLSelectElement;
    expect(select.disabled).toBe(true);
  });

  it('에러 상태에서 "다시 시도" 버튼 노출', () => {
    const refetch = vi.fn();
    const ds = makeFakeDataSource(() => ({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error('oops'),
      refetch,
    }));
    render(<TsdbSeriesListPanel dataSource={ds} />);
    const retry = screen.getByRole('button', { name: /다시 시도/ });
    fireEvent.click(retry);
    expect(refetch).toHaveBeenCalled();
  });
});
