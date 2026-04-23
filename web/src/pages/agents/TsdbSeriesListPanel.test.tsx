// SeriesListPanel 단위 테스트.
// SPEC-WEB-005 v0.2.0: dataSource prop 을 받아 useKeys() 를 호출한다.
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

/**
 * useKeys 를 수동으로 스텁할 수 있는 가벼운 SeriesDataSource 페이크.
 * 테스트마다 서로 다른 결과를 반환하고 호출 인자를 검사할 수 있다.
 */
function makeFakeDataSource(
  useKeysImpl: (params: { page: number; size: number }) => SeriesKeysQueryResult,
): SeriesDataSource {
  return {
    kind: 'tsdb',
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
    render(<TsdbSeriesListPanel dataSource={ds} onViewData={vi.fn()} />);
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
    render(<TsdbSeriesListPanel dataSource={ds} onViewData={vi.fn()} />);
    expect(screen.getByText('temp,room=1')).toBeInTheDocument();
    expect(screen.getByText('temp,room=2')).toBeInTheDocument();
    expect(screen.getByText('120')).toBeInTheDocument();
  });

  it('"데이터 보기" 버튼 클릭 시 onViewData 콜백 호출', () => {
    const onViewData = vi.fn();
    const ds = makeFakeDataSource(() => ({
      data: makePage(['temp,room=1'], 1, 1, 10),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    }));
    render(<TsdbSeriesListPanel dataSource={ds} onViewData={onViewData} />);
    fireEvent.click(screen.getByRole('button', { name: /데이터 보기/ }));
    expect(onViewData).toHaveBeenCalledWith('temp,room=1');
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
    render(<TsdbSeriesListPanel dataSource={ds} onViewData={vi.fn()} />);
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
    render(<TsdbSeriesListPanel dataSource={ds} onViewData={vi.fn()} />);
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
    render(<TsdbSeriesListPanel dataSource={ds} onViewData={vi.fn()} />);
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
    render(<TsdbSeriesListPanel dataSource={ds} onViewData={vi.fn()} />);
    const retry = screen.getByRole('button', { name: /다시 시도/ });
    fireEvent.click(retry);
    expect(refetch).toHaveBeenCalled();
  });
});
