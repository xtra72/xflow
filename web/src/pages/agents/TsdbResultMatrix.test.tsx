// SeriesResultMatrix 단위 테스트 — 매트릭스 렌더링, 누락 셀 em-dash, 타임스탬프 포맷.
//
// SPEC-WEB-005 v0.2.0: 컴포넌트가 pivot 된 `SeriesMatrix` 를 직접 받도록 변경되었다.
// SPEC-WEB-005 v0.3.0 Wave 2: CSV 내보내기 버튼 추가.
// SPEC-WEB-005 v0.4.0: 평균 자릿수 + 페이지네이션 (가상 스크롤 제거).
//
// @spec SPEC-WEB-005

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/react';

import SeriesResultMatrix from './TsdbResultMatrix';
import type { SeriesMatrix } from '@/services/api/seriesDataSource';

function makeMatrix(): SeriesMatrix {
  const t0 = new Date(2026, 3, 23, 0, 0, 0).getTime();
  const t1 = new Date(2026, 3, 23, 1, 0, 0).getTime();
  return {
    columns: ['temp,room=1', 'temp,room=2'],
    rows: [
      {
        bucketStartMs: t0,
        values: [21.5, 18.3],
      },
      {
        bucketStartMs: t1,
        // temp,room=2 는 이 버킷에 데이터가 없으므로 null.
        values: [22.0, null],
      },
    ],
  };
}

describe('SeriesResultMatrix', () => {
  it('선택된 시리즈 키를 컬럼 헤더로 표시', () => {
    render(<SeriesResultMatrix matrix={makeMatrix()} />);
    expect(screen.getByRole('columnheader', { name: 'temp,room=1' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'temp,room=2' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: /타임스탬프/ })).toBeInTheDocument();
  });

  it('시간 버킷이 매트릭스 rows 순서대로 렌더링', () => {
    render(<SeriesResultMatrix matrix={makeMatrix()} />);
    expect(screen.getByText('2026-04-23 00:00:00')).toBeInTheDocument();
    expect(screen.getByText('2026-04-23 01:00:00')).toBeInTheDocument();
  });

  it('누락된 버킷×시리즈 교차점은 em-dash(—) 로 표시', () => {
    render(<SeriesResultMatrix matrix={makeMatrix()} />);
    // t1 행에서 temp,room=2 셀은 — 여야 함.
    const row = screen.getByText('2026-04-23 01:00:00').closest('tr')!;
    expect(within(row).getByText('—')).toBeInTheDocument();
  });

  it('값이 존재하는 셀은 포맷된 숫자 렌더링', () => {
    render(<SeriesResultMatrix matrix={makeMatrix()} />);
    expect(screen.getByText('21.5')).toBeInTheDocument();
    // 22.0 은 Integer 로 인식되어 "22" 로 출력.
    expect(screen.getByText('22')).toBeInTheDocument();
  });

  it('단일 시리즈도 정상 렌더링 (1개 데이터 컬럼)', () => {
    const matrix: SeriesMatrix = {
      columns: ['only'],
      rows: [{ bucketStartMs: new Date(2026, 3, 23).getTime(), values: [1] }],
    };
    render(<SeriesResultMatrix matrix={matrix} />);
    expect(screen.getByRole('columnheader', { name: 'only' })).toBeInTheDocument();
  });

  it('emptyCellPlaceholder 커스텀 문자를 사용', () => {
    const matrix: SeriesMatrix = {
      columns: ['a', 'b'],
      rows: [
        {
          bucketStartMs: new Date(2026, 3, 23).getTime(),
          values: [1, null],
        },
      ],
    };
    render(<SeriesResultMatrix matrix={matrix} emptyCellPlaceholder="N/A" />);
    expect(screen.getByText('N/A')).toBeInTheDocument();
  });

  it('columns 가 없으면 안내 메시지 렌더링', () => {
    render(<SeriesResultMatrix matrix={{ columns: [], rows: [] }} />);
    expect(screen.getByText(/선택된 시리즈가 없습니다/)).toBeInTheDocument();
  });

  it('rows 가 비어있으면 안내 메시지 렌더링', () => {
    render(<SeriesResultMatrix matrix={{ columns: ['a'], rows: [] }} />);
    expect(screen.getByText(/쿼리 결과가 비어 있습니다/)).toBeInTheDocument();
  });
});

// ---- v0.3.0 Wave 2: CSV 내보내기 ----

describe('SeriesResultMatrix: CSV 내보내기', () => {
  let createObjectURLMock: ReturnType<typeof vi.fn>;
  let clickMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    createObjectURLMock = vi.fn(() => 'blob:fake');
    (URL as unknown as { createObjectURL: () => string }).createObjectURL =
      createObjectURLMock;
    (URL as unknown as { revokeObjectURL: (u: string) => void }).revokeObjectURL =
      vi.fn();

    // createElement('a') 의 click 만 가로채 네비게이션 없이 실행되도록.
    const origCreate = document.createElement.bind(document);
    clickMock = vi.fn();
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      const el = origCreate(tag);
      if (tag === 'a') {
        Object.defineProperty(el, 'click', {
          configurable: true,
          value: clickMock,
        });
      }
      return el;
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('rows 가 있을 때 CSV 내보내기 버튼 렌더링', () => {
    render(<SeriesResultMatrix matrix={makeMatrix()} />);
    expect(screen.getByTestId('tsdb-result-csv-export')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /CSV 내보내기/ })).toBeInTheDocument();
  });

  it('rows 가 비어있으면 CSV 버튼을 렌더링하지 않음', () => {
    render(<SeriesResultMatrix matrix={{ columns: ['a'], rows: [] }} />);
    expect(screen.queryByTestId('tsdb-result-csv-export')).toBeNull();
  });

  it('버튼 클릭 시 Blob URL 생성 + <a> 클릭 트리거', () => {
    render(
      <SeriesResultMatrix
        matrix={makeMatrix()}
        agentName="agent1"
        exportStartMs={new Date(2026, 3, 23, 0, 0, 0).getTime()}
        exportEndMs={new Date(2026, 3, 23, 1, 0, 0).getTime()}
      />,
    );
    fireEvent.click(screen.getByTestId('tsdb-result-csv-export'));
    expect(createObjectURLMock).toHaveBeenCalledTimes(1);
    expect(clickMock).toHaveBeenCalledTimes(1);
  });
});

// ---- v0.4.0: 평균 자릿수 (decimalPrecision) ----

describe('SeriesResultMatrix: 평균 자릿수', () => {
  function precisionMatrix(values: Array<number | null>): SeriesMatrix {
    const base = new Date(2026, 3, 23, 0, 0, 0).getTime();
    return {
      columns: ['v'],
      rows: values.map((v, i) => ({
        bucketStartMs: base + i * 60_000,
        values: [v],
      })),
    };
  }

  it('평균 + precision=1 → 10.123 은 "10.1" 로 표시', () => {
    render(
      <SeriesResultMatrix
        matrix={precisionMatrix([10.123])}
        aggregation="average"
        decimalPrecision={1}
      />,
    );
    expect(screen.getByText('10.1')).toBeInTheDocument();
  });

  it('평균 + precision=3 → 10.123 은 "10.123" 으로 표시', () => {
    render(
      <SeriesResultMatrix
        matrix={precisionMatrix([10.123])}
        aggregation="average"
        decimalPrecision={3}
      />,
    );
    expect(screen.getByText('10.123')).toBeInTheDocument();
  });

  it('평균 + precision=0 → 10.7 은 반올림되어 "11" 로 표시', () => {
    render(
      <SeriesResultMatrix
        matrix={precisionMatrix([10.7])}
        aggregation="average"
        decimalPrecision={0}
      />,
    );
    expect(screen.getByText('11')).toBeInTheDocument();
  });

  it('min 집계는 precision 을 무시하고 원본 값 표시', () => {
    render(
      <SeriesResultMatrix
        matrix={precisionMatrix([10.123])}
        aggregation="min"
        decimalPrecision={1}
      />,
    );
    // 1자리 반올림이 아닌 최대 4자리 이하 원본 표시.
    expect(screen.getByText('10.123')).toBeInTheDocument();
  });

  it('aggregation 미지정 시 precision 미적용 (기본 동작)', () => {
    render(<SeriesResultMatrix matrix={precisionMatrix([10.5])} />);
    expect(screen.getByText('10.5')).toBeInTheDocument();
  });
});

// ---- v0.4.0: 페이지네이션 ----

describe('SeriesResultMatrix: 페이지네이션', () => {
  function makeLargeMatrix(rowCount: number): SeriesMatrix {
    const base = new Date(2026, 3, 23, 0, 0, 0).getTime();
    return {
      columns: ['v'],
      rows: Array.from({ length: rowCount }, (_, i) => ({
        bucketStartMs: base + i * 60_000,
        values: [i],
      })),
    };
  }

  it('기본 페이지 크기 = 25 — 100행 매트릭스에 4 페이지로 표시', () => {
    render(<SeriesResultMatrix matrix={makeLargeMatrix(100)} />);
    const indicator = screen.getByTestId('tsdb-page-indicator');
    expect(indicator.textContent).toBe('1 / 4');
    expect(screen.getByTestId('tsdb-page-range').textContent).toBe('1-25 / 100 행');
  });

  it('페이지 크기 변경 시 페이지가 1로 리셋된다', () => {
    render(<SeriesResultMatrix matrix={makeLargeMatrix(100)} />);
    fireEvent.click(screen.getByTestId('tsdb-page-next'));
    expect(screen.getByTestId('tsdb-page-indicator').textContent).toBe('2 / 4');

    fireEvent.change(screen.getByTestId('tsdb-page-size'), { target: { value: '50' } });
    expect(screen.getByTestId('tsdb-page-indicator').textContent).toBe('1 / 2');
    expect(screen.getByTestId('tsdb-page-range').textContent).toBe('1-50 / 100 행');
  });

  it('첫 페이지에서 "이전" 버튼은 비활성', () => {
    render(<SeriesResultMatrix matrix={makeLargeMatrix(100)} />);
    const prev = screen.getByTestId('tsdb-page-prev') as HTMLButtonElement;
    expect(prev.disabled).toBe(true);
  });

  it('마지막 페이지에서 "다음" 버튼은 비활성', () => {
    render(<SeriesResultMatrix matrix={makeLargeMatrix(100)} />);
    const next = screen.getByTestId('tsdb-page-next') as HTMLButtonElement;
    // 4 페이지로 이동.
    fireEvent.click(next);
    fireEvent.click(next);
    fireEvent.click(next);
    expect(screen.getByTestId('tsdb-page-indicator').textContent).toBe('4 / 4');
    expect(next.disabled).toBe(true);
  });

  it('마지막 페이지의 "X-Y / Z" 표시는 Y = min(page*size, total)', () => {
    // 90 행 / 25 페이지 → 4 페이지에서 76-90 / 90.
    render(<SeriesResultMatrix matrix={makeLargeMatrix(90)} />);
    const next = screen.getByTestId('tsdb-page-next');
    fireEvent.click(next);
    fireEvent.click(next);
    fireEvent.click(next);
    expect(screen.getByTestId('tsdb-page-range').textContent).toBe('76-90 / 90 행');
  });

  it('빈 매트릭스: 페이지네이션 컨트롤이 렌더링되지 않는다', () => {
    render(<SeriesResultMatrix matrix={{ columns: ['a'], rows: [] }} />);
    expect(screen.queryByTestId('tsdb-result-pagination')).toBeNull();
  });

  it('페이지 크기 100 으로 99 행 매트릭스는 "1-99 / 99 행"', () => {
    render(<SeriesResultMatrix matrix={makeLargeMatrix(99)} />);
    fireEvent.change(screen.getByTestId('tsdb-page-size'), {
      target: { value: '100' },
    });
    expect(screen.getByTestId('tsdb-page-range').textContent).toBe('1-99 / 99 행');
    expect(screen.getByTestId('tsdb-page-indicator').textContent).toBe('1 / 1');
  });

  // ---- v0.5.0: 결과 뷰 모드 토글 (테이블/차트) ----

  it('헤더에 테이블/차트 토글이 노출되며 기본값은 테이블', () => {
    render(<SeriesResultMatrix matrix={makeMatrix()} />);
    const tableTab = screen.getByTestId('tsdb-result-view-table');
    const chartTab = screen.getByTestId('tsdb-result-view-chart');
    expect(tableTab.getAttribute('aria-selected')).toBe('true');
    expect(chartTab.getAttribute('aria-selected')).toBe('false');
    // 테이블 모드: 매트릭스 테이블이 보이고 차트는 숨김.
    expect(screen.getByRole('table')).toBeInTheDocument();
    expect(screen.queryByTestId('tsdb-result-chart')).toBeNull();
  });

  it('차트 탭 클릭 시 차트가 노출되고 테이블/페이지네이션은 사라진다', () => {
    render(<SeriesResultMatrix matrix={makeMatrix()} />);
    fireEvent.click(screen.getByTestId('tsdb-result-view-chart'));
    expect(
      screen.getByTestId('tsdb-result-view-chart').getAttribute('aria-selected'),
    ).toBe('true');
    expect(screen.getByTestId('tsdb-result-chart')).toBeInTheDocument();
    expect(screen.queryByRole('table')).toBeNull();
    expect(screen.queryByTestId('tsdb-result-pagination')).toBeNull();
  });

  it('차트 모드에서 다시 테이블 탭 클릭 시 테이블이 복원된다', () => {
    render(<SeriesResultMatrix matrix={makeMatrix()} />);
    fireEvent.click(screen.getByTestId('tsdb-result-view-chart'));
    fireEvent.click(screen.getByTestId('tsdb-result-view-table'));
    expect(screen.getByRole('table')).toBeInTheDocument();
    expect(screen.queryByTestId('tsdb-result-chart')).toBeNull();
  });

  it('빈 매트릭스에서도 뷰 토글은 노출된다 (모드 사전 선택 가능)', () => {
    render(<SeriesResultMatrix matrix={{ columns: ['a'], rows: [] }} />);
    expect(screen.getByTestId('tsdb-result-view-table')).toBeInTheDocument();
    expect(screen.getByTestId('tsdb-result-view-chart')).toBeInTheDocument();
  });

  // ---- v0.5.0: 차트 결측값 처리 ----

  it('차트 모드에서 결측값 처리 select 가 노출되며 기본은 "gap"', () => {
    render(<SeriesResultMatrix matrix={makeMatrix()} />);
    fireEvent.click(screen.getByTestId('tsdb-result-view-chart'));
    const select = screen.getByTestId('tsdb-chart-null-mode') as HTMLSelectElement;
    expect(select).toBeInTheDocument();
    expect(select.value).toBe('gap');
    // gap 모드에서는 fillValue 입력이 노출되지 않는다.
    expect(screen.queryByTestId('tsdb-chart-null-fill-value')).toBeNull();
  });

  it('"지정값" 선택 시 대체 값 number 입력이 노출된다', () => {
    render(<SeriesResultMatrix matrix={makeMatrix()} />);
    fireEvent.click(screen.getByTestId('tsdb-result-view-chart'));
    fireEvent.change(screen.getByTestId('tsdb-chart-null-mode'), {
      target: { value: 'value' },
    });
    expect(screen.getByTestId('tsdb-chart-null-fill-value')).toBeInTheDocument();
  });

  it('controlled viewMode prop 으로 외부에서 모드 강제 가능 + onChange 콜백', () => {
    const onChange = vi.fn();
    const { rerender } = render(
      <SeriesResultMatrix
        matrix={makeMatrix()}
        viewMode="chart"
        onViewModeChange={onChange}
      />,
    );
    // 외부 prop 으로 차트 모드 강제됨.
    expect(
      screen.getByTestId('tsdb-result-view-chart').getAttribute('aria-selected'),
    ).toBe('true');
    expect(screen.getByTestId('tsdb-result-chart')).toBeInTheDocument();

    // 토글 클릭 시 onViewModeChange 콜백만 호출되고 내부 state 는 변경되지 않는다.
    fireEvent.click(screen.getByTestId('tsdb-result-view-table'));
    expect(onChange).toHaveBeenCalledWith('table');
    // prop 이 그대로 'chart' 이므로 화면도 그대로.
    expect(screen.getByTestId('tsdb-result-chart')).toBeInTheDocument();

    // 부모가 prop 을 'table' 로 갱신하면 화면이 따라온다 ("다시 실행" 후 동일 모드 유지 시뮬).
    rerender(
      <SeriesResultMatrix
        matrix={makeMatrix()}
        viewMode="table"
        onViewModeChange={onChange}
      />,
    );
    expect(screen.getByRole('table')).toBeInTheDocument();
    expect(screen.queryByTestId('tsdb-result-chart')).toBeNull();
  });

  it('다른 모드(previous/interpolate)에서는 대체 값 입력이 숨겨진다', () => {
    render(<SeriesResultMatrix matrix={makeMatrix()} />);
    fireEvent.click(screen.getByTestId('tsdb-result-view-chart'));
    fireEvent.change(screen.getByTestId('tsdb-chart-null-mode'), {
      target: { value: 'previous' },
    });
    expect(screen.queryByTestId('tsdb-chart-null-fill-value')).toBeNull();
    fireEvent.change(screen.getByTestId('tsdb-chart-null-mode'), {
      target: { value: 'interpolate' },
    });
    expect(screen.queryByTestId('tsdb-chart-null-fill-value')).toBeNull();
  });
});
