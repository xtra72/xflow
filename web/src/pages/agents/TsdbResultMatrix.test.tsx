// SeriesResultMatrix 단위 테스트 — 매트릭스 렌더링, 누락 셀 em-dash, 타임스탬프 포맷.
//
// SPEC-WEB-005 v0.2.0: 컴포넌트가 pivot 된 `SeriesMatrix` 를 직접 받도록 변경되었다.
//
// @spec SPEC-WEB-005

import { describe, expect, it } from 'vitest';
import { render, screen, within } from '@testing-library/react';

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
