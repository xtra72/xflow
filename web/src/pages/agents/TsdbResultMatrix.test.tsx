// SeriesResultMatrix 단위 테스트 — 매트릭스 렌더링, 누락 셀 em-dash, 타임스탬프 포맷.
//
// SPEC-WEB-005 v0.2.0: 컴포넌트가 pivot 된 `SeriesMatrix` 를 직접 받도록 변경되었다.
// SPEC-WEB-005 v0.3.0 Wave 2: CSV 내보내기 버튼 + 가상 스크롤(≥500행) 커버리지 추가.
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

// ---- v0.3.0 Wave 2: 가상 스크롤 (react-window) ----

describe('SeriesResultMatrix: 가상 스크롤', () => {
  /** 지정된 행 수만큼 단일 컬럼 매트릭스를 만든다. */
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

  it('500 행 미만: 기존 HTML 테이블 렌더링, 가상 스크롤 wrapper 없음', () => {
    render(<SeriesResultMatrix matrix={makeLargeMatrix(499)} />);
    // 기존 경로는 `<table>` 이 존재하며, 가상 스크롤 wrapper 는 없다.
    expect(document.querySelector('table')).not.toBeNull();
    expect(screen.queryByTestId('tsdb-result-virtual-wrapper')).toBeNull();
  });

  it('500 행 이상: 가상 스크롤 wrapper 로 전환, 바닐라 table 미사용', () => {
    render(<SeriesResultMatrix matrix={makeLargeMatrix(500)} />);
    expect(screen.getByTestId('tsdb-result-virtual-wrapper')).toBeInTheDocument();
    // 가상 스크롤 경로는 `<table>` 을 사용하지 않는다.
    expect(document.querySelector('table')).toBeNull();
    // 헤더는 여전히 컬럼명을 columnheader role 로 노출.
    expect(screen.getByRole('columnheader', { name: /타임스탬프/ })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'v' })).toBeInTheDocument();
  });

  it('가상 스크롤 경로에서도 CSV 내보내기 버튼은 노출된다', () => {
    render(<SeriesResultMatrix matrix={makeLargeMatrix(600)} />);
    expect(screen.getByTestId('tsdb-result-csv-export')).toBeInTheDocument();
  });

  it('가상 스크롤 경로에 "가상 스크롤" 라벨이 표시된다', () => {
    render(<SeriesResultMatrix matrix={makeLargeMatrix(500)} />);
    expect(screen.getByText(/가상 스크롤/)).toBeInTheDocument();
  });
});
