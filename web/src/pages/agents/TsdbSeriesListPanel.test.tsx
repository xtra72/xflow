// TsdbSeriesListPanel 단위 테스트.
// useTsdbSeries 훅을 vi.mock 으로 교체해 API 호출 없이 UI 상호작용을 검증한다.
//
// @spec SPEC-WEB-005

import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

import type { TsdbSeriesListResponse } from '@/services/api/tsdb';

// useTsdbSeries 훅 모킹
type UseTsdbSeriesArgs = { page: number; size: number; agentId?: string };
type UseTsdbSeriesResult = {
  data: TsdbSeriesListResponse | undefined;
  isLoading: boolean;
  error: Error | null;
  refetch: () => void;
};

const useTsdbSeriesMock = vi.hoisted(() =>
  vi.fn<(args: UseTsdbSeriesArgs) => UseTsdbSeriesResult>(),
);

vi.mock('@/hooks/useTsdb', () => ({
  useTsdbSeries: useTsdbSeriesMock,
}));

import TsdbSeriesListPanel from './TsdbSeriesListPanel';

beforeEach(() => {
  useTsdbSeriesMock.mockReset();
});

function makeResponse(
  series: string[],
  total: number,
  page = 1,
  size = 10,
): TsdbSeriesListResponse {
  return {
    series,
    count: series.length,
    pagination: {
      page,
      size,
      total,
      total_pages: size > 0 ? Math.ceil(total / size) : 0,
    },
  };
}

describe('TsdbSeriesListPanel', () => {
  it('로딩 중에는 스켈레톤을 표시', () => {
    useTsdbSeriesMock.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
      refetch: vi.fn(),
    });
    render(<TsdbSeriesListPanel onViewData={vi.fn()} />);
    expect(screen.getAllByTestId('tsdb-series-skeleton').length).toBeGreaterThan(0);
  });

  it('시리즈 목록을 렌더링하고 총 개수를 표시', () => {
    useTsdbSeriesMock.mockReturnValue({
      data: makeResponse(['temp,room=1', 'temp,room=2'], 120, 1, 10),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    render(<TsdbSeriesListPanel onViewData={vi.fn()} />);
    expect(screen.getByText('temp,room=1')).toBeInTheDocument();
    expect(screen.getByText('temp,room=2')).toBeInTheDocument();
    expect(screen.getByText('120')).toBeInTheDocument();
  });

  it('"데이터 보기" 버튼 클릭 시 onViewData 콜백 호출', () => {
    const onViewData = vi.fn();
    useTsdbSeriesMock.mockReturnValue({
      data: makeResponse(['temp,room=1'], 1, 1, 10),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    render(<TsdbSeriesListPanel onViewData={onViewData} />);
    fireEvent.click(screen.getByRole('button', { name: /데이터 보기/ }));
    expect(onViewData).toHaveBeenCalledWith('temp,room=1');
  });

  it('페이지 크기 셀렉터 변경 시 page=1/size=변경값으로 재조회', () => {
    useTsdbSeriesMock.mockReturnValue({
      data: makeResponse(['a'], 1, 1, 10),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    const { rerender } = render(<TsdbSeriesListPanel onViewData={vi.fn()} />);
    // 초기 호출: page=1, size=10
    expect(useTsdbSeriesMock).toHaveBeenCalledWith({ page: 1, size: 10, agentId: undefined });

    useTsdbSeriesMock.mockClear();
    useTsdbSeriesMock.mockReturnValue({
      data: makeResponse(['a'], 1, 1, 25),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const select = screen.getByLabelText('페이지당') as HTMLSelectElement;
    fireEvent.change(select, { target: { value: '25' } });
    rerender(<TsdbSeriesListPanel onViewData={vi.fn()} />);
    // 재호출: page=1 (리셋), size=25
    expect(useTsdbSeriesMock).toHaveBeenCalledWith({ page: 1, size: 25, agentId: undefined });
  });

  it('첫 페이지에서 "이전" 버튼 비활성, 마지막 페이지에서 "다음" 버튼 비활성', () => {
    useTsdbSeriesMock.mockReturnValue({
      data: makeResponse(['a'], 1, 1, 10),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    render(<TsdbSeriesListPanel onViewData={vi.fn()} />);
    const prev = screen.getByRole('button', { name: '이전 페이지' }) as HTMLButtonElement;
    const next = screen.getByRole('button', { name: '다음 페이지' }) as HTMLButtonElement;
    expect(prev.disabled).toBe(true);
    expect(next.disabled).toBe(true);
  });

  it('빈 상태일 때 "저장된 시리즈가 없습니다" 표시', () => {
    useTsdbSeriesMock.mockReturnValue({
      data: makeResponse([], 0, 1, 10),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    render(<TsdbSeriesListPanel onViewData={vi.fn()} />);
    expect(screen.getByText(/저장된 시리즈가 없습니다/)).toBeInTheDocument();
    const select = screen.getByLabelText('페이지당') as HTMLSelectElement;
    expect(select.disabled).toBe(true);
  });

  it('에러 상태에서 "다시 시도" 버튼 노출', () => {
    const refetch = vi.fn();
    useTsdbSeriesMock.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('oops'),
      refetch,
    });
    render(<TsdbSeriesListPanel onViewData={vi.fn()} />);
    const retry = screen.getByRole('button', { name: /다시 시도/ });
    fireEvent.click(retry);
    expect(refetch).toHaveBeenCalled();
  });
});
